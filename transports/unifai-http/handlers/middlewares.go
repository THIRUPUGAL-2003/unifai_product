package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	providerUtils "github.com/unifai/unifai/core/providers/utils"
	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/rbac"
	"github.com/unifai/unifai/framework/temptoken"
	"github.com/unifai/unifai/framework/tracing"
	"github.com/unifai/unifai/plugins/governance"
	"github.com/unifai/unifai/transports/unifai-http/integrations"
	"github.com/unifai/unifai/transports/unifai-http/lib"
	"github.com/valyala/fasthttp"
)

var loggingSkipPaths = []string{"/health", "/_next", "/api/dev"}
var realtimeTransportPaths = buildRealtimeTransportPathSet()

// SecurityHeadersMiddleware sets security-related HTTP headers on every response.
// This should wrap the outermost handler so all responses (API, UI, errors) include these headers.
func SecurityHeadersMiddleware() schemas.UnifAIHTTPMiddleware {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			ctx.Response.Header.Set("X-Frame-Options", "DENY")
			ctx.Response.Header.Set("X-Content-Type-Options", "nosniff")
			ctx.Response.Header.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			ctx.Response.Header.Set("Content-Security-Policy", "default-src 'self' https://*.google.com https://*.googleapis.com; script-src 'self' 'unsafe-inline' 'unsafe-eval' https://*.google.com https://*.googleapis.com https://*.gstatic.com; style-src 'self' 'unsafe-inline' https://*.google.com https://*.googleapis.com https://*.gstatic.com; img-src 'self' data: https: https://*.google.com https://*.gstatic.com; font-src 'self' data: https://fonts.gstatic.com; connect-src 'self' https: wss: https://*.google.com https://*.googleapis.com; frame-src 'self' https://*.google.com https://*.googleapis.com; frame-ancestors 'none'; object-src 'none'; base-uri 'self'")
			ctx.Response.Header.Set("Permissions-Policy", "camera=(), microphone=(self), geolocation=()")
			// Only set HSTS when serving over HTTPS (detected via reverse proxy header or direct TLS)
			if string(ctx.Request.Header.Peek("X-Forwarded-Proto")) == "https" || ctx.IsTLS() {
				ctx.Response.Header.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next(ctx)
		}
	}
}

// clientForwardedIP returns the client-supplied originating IP from reverse-proxy
// headers, or "" if none are present. X-Forwarded-For may be a comma-separated list
// (client, proxy1, proxy2); the leftmost entry is the original client.
//
// These headers are caller-controlled and unauthenticated unless UnifAI sits behind
// a trusted proxy that overwrites them. The value is logged as http.forwarded_for —
// separate from the authoritative http.remote_addr (the real TCP peer) — so a forged
// header cannot mask the true peer in the access log.
func clientForwardedIP(ctx *fasthttp.RequestCtx) string {
	if xff := strings.TrimSpace(string(ctx.Request.Header.Peek("X-Forwarded-For"))); xff != "" {
		if first, _, found := strings.Cut(xff, ","); found {
			return strings.TrimSpace(first)
		}
		return xff
	}
	if xrip := strings.TrimSpace(string(ctx.Request.Header.Peek("X-Real-IP"))); xrip != "" {
		return xrip
	}
	return ""
}

// corsMiddlewareConfig is an immutable snapshot of the CORS-relevant client config.
// The slices are cloned at construction so a hot reload mutating the source
// ClientConfig in place cannot race with in-flight requests reading these fields.
type corsMiddlewareConfig struct {
	dumpErrorsInConsoleLogs bool
	allowedOrigins          []string
	allowedHeaders          []string
}

// newCorsMiddlewareConfig builds an immutable snapshot from the live config,
// cloning the slices so the snapshot never aliases the shared ClientConfig.
func newCorsMiddlewareConfig(config *lib.Config) *corsMiddlewareConfig {
	if config == nil || config.ClientConfig == nil {
		return nil
	}
	return &corsMiddlewareConfig{
		dumpErrorsInConsoleLogs: config.ClientConfig.DumpErrorsInConsoleLogs,
		allowedOrigins:          slices.Clone(config.ClientConfig.AllowedOrigins),
		allowedHeaders:          slices.Clone(config.ClientConfig.AllowedHeaders),
	}
}

// CorsMiddleware handles CORS headers for localhost and configured allowed origins.
// The snapshot is held in an atomic.Pointer so UpdateConfig can swap it at runtime
// without racing in-flight requests, which read the pointer concurrently. Because the
// snapshot is immutable (slices cloned), readers never observe a torn or half-updated
// config even while a reload swaps in a new one.
type CorsMiddleware struct {
	config atomic.Pointer[corsMiddlewareConfig]
}

func NewCorsMiddleware(config *lib.Config) *CorsMiddleware {
	c := &CorsMiddleware{}
	c.config.Store(newCorsMiddlewareConfig(config))
	return c
}

// UpdateConfig atomically swaps in a fresh immutable snapshot of the configuration.
// In-flight requests reading the pointer observe either the old or the new snapshot,
// never a torn value. ReloadClientConfigFromConfigStore must call this whenever the
// client config is refreshed, mirroring how AuthMiddleware is updated.
func (c *CorsMiddleware) UpdateConfig(config *lib.Config) {
	c.config.Store(newCorsMiddlewareConfig(config))
}

// CorsMiddleware handles CORS headers for localhost and configured allowed origins
func (c *CorsMiddleware) Middleware() schemas.UnifAIHTTPMiddleware {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			// Snapshot the config once per request so a concurrent UpdateConfig swap
			// cannot apply two different configs within a single response.
			cfg := c.config.Load()
			if cfg == nil {
				SendError(ctx, fasthttp.StatusInternalServerError, "CORS middleware configuration not loaded")
				return
			}
			shouldLog := slices.IndexFunc(loggingSkipPaths, func(path string) bool {
				return strings.HasPrefix(string(ctx.RequestURI()), path)
			}) == -1
			if shouldLog {
				startTime := time.Now()
				defer func() {
					statusCode := ctx.Response.Header.StatusCode()
					level := schemas.LogLevelInfo
					if statusCode >= 500 {
						level = schemas.LogLevelError
					} else if statusCode >= 400 {
						level = schemas.LogLevelWarn
					}
					logBuilder := logger.LogHTTPRequest(level, "request completed").
						Str("http.method", string(ctx.Method())).
						Str("http.target", string(ctx.RequestURI())).
						Int("http.status_code", statusCode).
						Int64("http.request_duration_ms", time.Since(startTime).Milliseconds()).
						Str("http.remote_addr", ctx.RemoteAddr().String()).
						Str("http.user_agent", string(ctx.Request.Header.UserAgent()))
					if forwarded := clientForwardedIP(ctx); forwarded != "" {
						logBuilder = logBuilder.Str("http.forwarded_for", forwarded)
					}
					if traceID, ok := ctx.UserValue(schemas.UnifAIContextKeyTraceID).(string); ok && traceID != "" {
						logBuilder = logBuilder.Str("trace_id", traceID)
					}
					if cfg.dumpErrorsInConsoleLogs {
						if statusCode >= 400 && !ctx.Response.IsBodyStream() {
							if body := ctx.Response.Body(); len(body) > 0 {
								logBuilder = logBuilder.Str("http.error", string(body))
							}
						}
					}
					logBuilder.Send()
				}()
			}
			origin := string(ctx.Request.Header.Peek("Origin"))
			allowed := IsOriginAllowed(origin, cfg.allowedOrigins)
			// Credentialed CORS only for exact origins (never for "*" or subdomain wildcards).
			credentialed := originAllowsCredentials(origin, cfg.allowedOrigins, string(ctx.Host()))

			allowedHeaders := []string{
				"Content-Type",
				"Authorization",
				"X-Requested-With",
				"X-Stainless-Timeout",
				"X-Api-Key",
				"X-OpenAI-Agents-SDK",
				"X-Operation-ID",
				// Prompt Repo / MCP playground headers (case-insensitive match in browsers)
				"x-uf-mcp-include-clients",
				"x-uf-mcp-include-tools",
				"x-uf-api-key-id",
				"x-uf-skill-id",
				"x-uf-prompt-id",
				"x-uf-prompt-version",
				"x-uf-prompt-environment",
			}
			if slices.Contains(cfg.allowedHeaders, "*") {
				if credentialed {
					// Per the Fetch spec, Access-Control-Allow-Headers: * is NOT treated as a
					// wildcard when Access-Control-Allow-Credentials: true is set — browsers
					// interpret it as a literal header name. For credentialed preflight requests,
					// reflect back the requested headers instead.
					if requestedHeaders := string(ctx.Request.Header.Peek("Access-Control-Request-Headers")); requestedHeaders != "" {
						allowedHeaders = []string{requestedHeaders}
					}
					// For non-preflight requests (no Access-Control-Request-Headers), keep defaults.
				} else {
					allowedHeaders = []string{"*"}
				}
			} else if len(cfg.allowedHeaders) > 0 {
				// append allowed headers from config to the default headers
				for _, header := range cfg.allowedHeaders {
					if !slices.Contains(allowedHeaders, header) {
						allowedHeaders = append(allowedHeaders, header)
					}
				}
			}
			// Check if origin is allowed (localhost always allowed + configured origins)
			if allowed {
				ctx.Response.Header.Set("Access-Control-Allow-Origin", origin)
				ctx.Response.Header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS, HEAD")
				ctx.Response.Header.Set("Access-Control-Allow-Headers", strings.Join(allowedHeaders, ", "))
				if credentialed {
					ctx.Response.Header.Set("Access-Control-Allow-Credentials", "true")
				}
				ctx.Response.Header.Set("Access-Control-Max-Age", "86400")
				// Vary: Origin tells caches that the response varies based on the Origin
				// request header, preventing incorrect CORS headers from being served.
				ctx.Response.Header.Set("Vary", "Origin")
			}
			// Handle preflight OPTIONS requests
			if string(ctx.Method()) == "OPTIONS" {
				if allowed {
					ctx.SetStatusCode(fasthttp.StatusOK)
				} else {
					ctx.SetStatusCode(fasthttp.StatusForbidden)
				}
				return
			}
			next(ctx)
		}
	}
}

// RequestDecompressionMiddleware transparently decompresses compressed request bodies.
// Two paths based on compressed Content-Length:
//   - Large or chunked (CL > threshold or CL unknown): streaming decompression via
//     SetBodyStream, avoiding full body materialization. Uses pooled gzip readers
//     matching the response-side pattern in core/providers/utils.
//   - Small (CL ≤ threshold): buffered decompression via io.ReadAll + SetBodyRaw,
//     with decompression bomb protection via MaxRequestBodySizeMB.
func RequestDecompressionMiddleware(config *lib.Config) schemas.UnifAIHTTPMiddleware {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			if len(ctx.Request.Header.ContentEncoding()) == 0 {
				next(ctx)
				return
			}

			if shouldStreamDecompress(config, ctx) {
				cleanup, applied, err := streamingDecompress(ctx)
				if err != nil {
					SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("invalid compressed request body: %v", err))
					return
				}
				if applied {
					next(ctx)
					cleanup()
					return
				}
				// No body stream available (StreamRequestBody not enabled) — fall
				// through to the buffered decompression path below.
			}

			// Buffered path: small compressed request — materialize fully.
			maxRequestBodyBytes := 100 * 1024 * 1024 // default 100 MB (matches decodeRequestBodyWithLimit fallback)
			if config != nil && config.ClientConfig.MaxRequestBodySizeMB > 0 {
				maxRequestBodyBytes = config.ClientConfig.MaxRequestBodySizeMB * 1024 * 1024
			}

			body, err := decodeRequestBodyWithLimit(&ctx.Request, maxRequestBodyBytes)
			if errors.Is(err, errRequestBodyTooLarge) {
				SendError(ctx, fasthttp.StatusRequestEntityTooLarge, fmt.Sprintf("decompressed request body exceeds max allowed size of %d bytes", maxRequestBodyBytes))
				return
			}
			if err != nil {
				SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("invalid compressed request body: %v", err))
				return
			}

			ctx.Request.SetBodyRaw(body)
			ctx.Request.Header.Del(fasthttp.HeaderContentEncoding)
			ctx.Request.Header.Del(fasthttp.HeaderContentLength)
			next(ctx)
		}
	}
}

// shouldStreamDecompress returns true when the compressed request body should
// use streaming decompression rather than full materialization. Uses the
// config threshold (set by enterprise from LargePayloadConfig.RequestThresholdBytes)
// or falls back to DefaultLargePayloadRequestThresholdBytes.
// Chunked requests (unknown size) always stream to be safe.
func shouldStreamDecompress(config *lib.Config, ctx *fasthttp.RequestCtx) bool {
	contentLength := ctx.Request.Header.ContentLength()
	// Chunked transfer encoding: fasthttp reports -1. Size unknown, stream to be safe.
	if contentLength < 0 {
		return true
	}
	var threshold int64 = schemas.DefaultLargePayloadRequestThresholdBytes
	if config != nil && config.StreamingDecompressThreshold > 0 {
		threshold = config.StreamingDecompressThreshold
	}
	return int64(contentLength) > threshold
}

// streamingDecompress wraps the request body stream with a streaming decompression
// reader, avoiding full body materialization for large compressed requests.
// Returns (cleanup, applied, err):
//   - applied=true: body stream was wrapped; caller must invoke cleanup after the
//     handler chain completes and the body is fully consumed.
//   - applied=false: no body stream available (StreamRequestBody not enabled on the
//     server). Caller should fall back to the buffered decompression path.
func streamingDecompress(ctx *fasthttp.RequestCtx) (cleanup func(), applied bool, err error) {
	bodyStream := ctx.RequestBodyStream()
	if bodyStream == nil {
		return func() {}, false, nil
	}

	encoding := strings.ToLower(strings.TrimSpace(
		string(ctx.Request.Header.ContentEncoding()),
	))

	decompReader, cleanup, err := newDecompressReader(bodyStream, encoding)
	if err != nil {
		return nil, false, err
	}

	ctx.Request.SetBodyStream(decompReader, -1)
	ctx.Request.Header.Del(fasthttp.HeaderContentEncoding)
	ctx.Request.Header.Del(fasthttp.HeaderContentLength)

	return cleanup, true, nil
}

var errRequestBodyTooLarge = errors.New("decompressed request body exceeds max allowed size")

// decodeRequestBodyWithLimit decodes the request body with a limit on the size of the body.
func decodeRequestBodyWithLimit(req *fasthttp.Request, maxRequestBodyBytes int) ([]byte, error) {
	encoding := strings.ToLower(strings.TrimSpace(string(req.Header.ContentEncoding())))
	bodyReader := bytes.NewReader(req.Body())

	var reader io.Reader = bodyReader
	cleanup := func() {}
	if encoding != "" {
		var err error
		reader, cleanup, err = newDecompressReader(bodyReader, encoding)
		if err != nil {
			return nil, err
		}
	}
	defer cleanup()

	if maxRequestBodyBytes <= 0 {
		maxRequestBodyBytes = 100 * 1024 * 1024 // 100 MB hard cap
	}

	limitedReader := &io.LimitedReader{R: reader, N: int64(maxRequestBodyBytes + 1)}
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, err
	}
	if len(body) > maxRequestBodyBytes {
		return nil, errRequestBodyTooLarge
	}
	return body, nil
}

// newDecompressReader wraps r with a decompression reader for the given encoding.
// All encodings use pooled readers from core/providers/utils. The returned cleanup
// function must be called when the reader is no longer needed.
func newDecompressReader(r io.Reader, encoding string) (io.Reader, func(), error) {
	switch encoding {
	case "gzip":
		gz, err := providerUtils.AcquireGzipReader(r)
		if err != nil {
			return nil, nil, err
		}
		return gz, func() { providerUtils.ReleaseGzipReader(gz) }, nil
	case "deflate":
		fr, err := providerUtils.AcquireFlateReader(r)
		if err != nil {
			return nil, nil, err
		}
		return fr, func() { providerUtils.ReleaseFlateReader(fr) }, nil
	case "br":
		br := providerUtils.AcquireBrotliReader(r)
		return br, func() { providerUtils.ReleaseBrotliReader(br) }, nil
	case "zstd":
		dec, err := providerUtils.AcquireZstdDecoder(r)
		if err != nil {
			return nil, nil, err
		}
		return dec, func() { providerUtils.ReleaseZstdDecoder(dec) }, nil
	default:
		return nil, nil, fmt.Errorf("%w: %q", fasthttp.ErrContentEncodingUnsupported, encoding)
	}
}

// TransportInterceptorMiddleware runs all plugin HTTP transport interceptors.
// It converts the fasthttp request to a serializable HTTPRequest, runs all plugin interceptors,
// and applies any modifications back to the fasthttp context.
func TransportInterceptorMiddleware(config *lib.Config) schemas.UnifAIHTTPMiddleware {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			plugins := config.GetLoadedHTTPTransportPlugins()
			if len(plugins) == 0 {
				next(ctx)
				return
			}
			// Get or create UnifAIContext from fasthttp context
			unifaiCtx := getUnifAIContextFromFastHTTP(ctx)
			// Acquire pooled request
			req := schemas.AcquireHTTPRequest()
			defer schemas.ReleaseHTTPRequest(req)
			fasthttpToHTTPRequest(ctx, req)
			// Run plugin interceptors
			for _, plugin := range plugins {
				pluginName := plugin.GetName()
				pluginCtx := unifaiCtx.WithPluginScope(&pluginName)
				resp, err := plugin.HTTPTransportPreHook(pluginCtx, req)
				pluginCtx.ReleasePluginScope()
				if err != nil {
					// Short-circuit with error — drain plugin logs before returning
					if logs := unifaiCtx.DrainPluginLogs(); len(logs) > 0 {
						ctx.SetUserValue(schemas.UnifAIContextKeyTransportPluginLogs, logs)
					}
					ctx.SetStatusCode(fasthttp.StatusInternalServerError)
					ctx.SetBodyString(err.Error())
					return
				}
				if resp != nil {
					// Short-circuit with response — drain plugin logs before returning
					if logs := unifaiCtx.DrainPluginLogs(); len(logs) > 0 {
						ctx.SetUserValue(schemas.UnifAIContextKeyTransportPluginLogs, logs)
					}
					applyHTTPResponseToCtx(ctx, resp)
					return
				}
				// If we got here, the plugin may have modified req in-place
			}
			// Drain pre-hook plugin logs and store on fasthttp context for trace attachment
			if preHookLogs := unifaiCtx.DrainPluginLogs(); len(preHookLogs) > 0 {
				ctx.SetUserValue(schemas.UnifAIContextKeyTransportPluginLogs, preHookLogs)
			}
			// Apply modifications back to fasthttp context
			applyHTTPRequestToCtx(ctx, req)
			// Adding user values
			for key, value := range unifaiCtx.GetUserValues() {
				ctx.SetUserValue(key, value)
			}
			next(ctx)

			// For streaming responses, store a callback to run post-hooks after the stream ends.
			// The streaming handler calls this BEFORE reader.Done() so that errors can
			// still be sent as SSE events. applyResponse=false because the response is
			// already on the wire and mutating ctx.Response would corrupt the chunked stream.
			//
			// IMPORTANT: The callback must NOT access ctx — fasthttp recycles RequestCtx
			// after the response body stream completes. All needed data is eagerly captured
			// here (while ctx is still valid) and passed through the closure.
			if deferred, ok := ctx.UserValue(schemas.UnifAIContextKeyDeferTraceCompletion).(bool); ok && deferred {
				// Verify the completer slot exists before allocating pooled snapshots.
				// The streaming handler pre-allocates this *atomic.Value; if absent,
				// skip work to avoid leaking pooled HTTPRequest/HTTPResponse objects.
				slot, ok := ctx.UserValue(schemas.UnifAIContextKeyTransportPostHookCompleter).(*atomic.Value)
				if !ok {
					return
				}

				// Eagerly snapshot request/response from ctx before it can be recycled.
				capturedReq := lib.BuildHTTPRequestFromFastHTTP(ctx)
				capturedResp := lib.BuildHTTPResponseFromFastHTTP(ctx)
				// Snapshot pre-hook transport plugin logs already accumulated on ctx.
				var preHookLogs []schemas.PluginLogEntry
				if logs, ok := ctx.UserValue(schemas.UnifAIContextKeyTransportPluginLogs).([]schemas.PluginLogEntry); ok {
					preHookLogs = logs
				}

				completer := func() ([]schemas.PluginLogEntry, error) {
					defer schemas.ReleaseHTTPRequest(capturedReq)
					defer schemas.ReleaseHTTPResponse(capturedResp)
					postHookLogs, err := runTransportPostHooksCaptured(capturedReq, capturedResp, plugins, unifaiCtx)
					allLogs := preHookLogs
					if len(postHookLogs) > 0 {
						allLogs = append(allLogs, postHookLogs...)
					}
					return allLogs, err
				}

				// Store the completer in the atomic.Value slot that the streaming handler
				// placed on ctx. The goroutine reads from its closure-captured copy of
				// the slot, avoiding any ctx access after the handler returns.
				slot.Store(completer)
				return
			}

			_ = runTransportPostHooks(ctx, plugins, unifaiCtx, true)
		}
	}
}

// runTransportPostHooks runs HTTPTransportPostHook for all plugins in reverse order,
// drains plugin logs, and applies the response back to the fasthttp context.
// Used for both non-streaming (inline) and streaming (deferred callback) paths.
//
// Transport-level plugin logs are stored in fasthttp UserValues (keyed by
// UnifAIContextKeyTransportPluginLogs) rather than directly on UnifAIContext,
// because transport hooks operate at the fasthttp layer before/after the core
// UnifAIContext lifecycle. These logs are merged into the trace by the
// TracingMiddleware at trace completion, alongside core-level plugin logs
// which travel through UnifAIContext → Trace → AttachPluginLogs.
func runTransportPostHooks(ctx *fasthttp.RequestCtx, plugins []schemas.HTTPTransportPlugin, unifaiCtx *schemas.UnifAIContext, applyResponse bool) error {
	shouldApplyShortCircuit := applyResponse
	httpResp := schemas.AcquireHTTPResponse()
	defer schemas.ReleaseHTTPResponse(httpResp)
	fasthttpResponseToHTTPResponse(ctx, httpResp)

	// Build request from current fasthttp state (original pooled req may have been released)
	req := schemas.AcquireHTTPRequest()
	defer schemas.ReleaseHTTPRequest(req)
	fasthttpToHTTPRequest(ctx, req)

	// Run http post-hooks in reverse order
	for i := len(plugins) - 1; i >= 0; i-- {
		plugin := plugins[i]
		pluginName := plugin.GetName()
		pluginCtx := unifaiCtx.WithPluginScope(&pluginName)
		err := plugin.HTTPTransportPostHook(pluginCtx, req, httpResp)
		pluginCtx.ReleasePluginScope()
		if err != nil {
			logger.Warn("error in HTTPTransportPostHook for plugin %s: %s", pluginName, err.Error())
			// Drain plugin logs before returning on error
			if postHookLogs := unifaiCtx.DrainPluginLogs(); len(postHookLogs) > 0 {
				if existing, ok := ctx.UserValue(schemas.UnifAIContextKeyTransportPluginLogs).([]schemas.PluginLogEntry); ok {
					ctx.SetUserValue(schemas.UnifAIContextKeyTransportPluginLogs, append(existing, postHookLogs...))
				} else {
					ctx.SetUserValue(schemas.UnifAIContextKeyTransportPluginLogs, postHookLogs)
				}
			}
			if shouldApplyShortCircuit {
				applyHTTPResponseToCtx(ctx, httpResp)
			}
			return fmt.Errorf("transport post-hook plugin %s: %w", pluginName, err)
		}
	}
	// Drain post-hook plugin logs and merge with pre-hook logs
	if postHookLogs := unifaiCtx.DrainPluginLogs(); len(postHookLogs) > 0 {
		if existing, ok := ctx.UserValue(schemas.UnifAIContextKeyTransportPluginLogs).([]schemas.PluginLogEntry); ok {
			ctx.SetUserValue(schemas.UnifAIContextKeyTransportPluginLogs, append(existing, postHookLogs...))
		} else {
			ctx.SetUserValue(schemas.UnifAIContextKeyTransportPluginLogs, postHookLogs)
		}
	}
	if shouldApplyShortCircuit {
		applyHTTPResponseToCtx(ctx, httpResp)
	}
	return nil
}

// runTransportPostHooksCaptured is the goroutine-safe variant of runTransportPostHooks.
// It uses pre-captured HTTPRequest and HTTPResponse snapshots instead of reading from
// a fasthttp RequestCtx, which may have been recycled by the time this runs in a
// streaming goroutine. Returns accumulated plugin logs (instead of writing them to
// ctx.UserValue) so the caller can forward them to the trace completer.
func runTransportPostHooksCaptured(capturedReq *schemas.HTTPRequest, capturedResp *schemas.HTTPResponse, plugins []schemas.HTTPTransportPlugin, unifaiCtx *schemas.UnifAIContext) ([]schemas.PluginLogEntry, error) {
	// Clone into fresh pooled objects so plugins can mutate without affecting the snapshots.
	req := schemas.AcquireHTTPRequest()
	defer schemas.ReleaseHTTPRequest(req)
	req.Method = capturedReq.Method
	req.Path = capturedReq.Path
	for k, v := range capturedReq.Headers {
		req.Headers[k] = v
	}
	for k, v := range capturedReq.Query {
		req.Query[k] = v
	}
	for k, v := range capturedReq.PathParams {
		req.PathParams[k] = v
	}

	httpResp := schemas.AcquireHTTPResponse()
	defer schemas.ReleaseHTTPResponse(httpResp)
	httpResp.StatusCode = capturedResp.StatusCode
	for k, v := range capturedResp.Headers {
		httpResp.Headers[k] = v
	}

	var allLogs []schemas.PluginLogEntry

	// Run http post-hooks in reverse order
	for i := len(plugins) - 1; i >= 0; i-- {
		plugin := plugins[i]
		pluginName := plugin.GetName()
		pluginCtx := unifaiCtx.WithPluginScope(&pluginName)
		err := plugin.HTTPTransportPostHook(pluginCtx, req, httpResp)
		pluginCtx.ReleasePluginScope()
		if err != nil {
			logger.Warn("error in HTTPTransportPostHook for plugin %s: %s", pluginName, err.Error())
			if postHookLogs := unifaiCtx.DrainPluginLogs(); len(postHookLogs) > 0 {
				allLogs = append(allLogs, postHookLogs...)
			}
			return allLogs, fmt.Errorf("transport post-hook plugin %s: %w", pluginName, err)
		}
	}
	// Drain post-hook plugin logs
	if postHookLogs := unifaiCtx.DrainPluginLogs(); len(postHookLogs) > 0 {
		allLogs = append(allLogs, postHookLogs...)
	}
	return allLogs, nil
}

// getUnifAIContextFromFastHTTP gets or creates a UnifAIContext from fasthttp context.
func getUnifAIContextFromFastHTTP(ctx *fasthttp.RequestCtx) *schemas.UnifAIContext {
	return schemas.NewUnifAIContext(ctx, schemas.NoDeadline)
}

// fasthttpToHTTPRequest populates a pooled HTTPRequest from fasthttp context.
func fasthttpToHTTPRequest(ctx *fasthttp.RequestCtx, req *schemas.HTTPRequest) {
	req.Method = string(ctx.Method())
	req.Path = string(ctx.Path())

	// Copy headers
	for key, value := range ctx.Request.Header.All() {
		req.Headers[string(key)] = string(value)
	}

	// Copy query params
	for key, value := range ctx.Request.URI().QueryArgs().All() {
		req.Query[string(key)] = string(value)
	}

	// Copy path parameters from user values
	// The fasthttp router stores path variables (like {file_id}, {model}) as user values
	// We extract all string user values that are likely path parameters
	ctx.VisitUserValuesAll(func(key, value any) {
		// Only process string keys and string values
		keyStr, keyIsString := key.(string)
		valueStr, valueIsString := value.(string)
		if !keyIsString || !valueIsString {
			return
		}
		// Skip internal UnifAI system keys and tracing keys
		if strings.HasPrefix(keyStr, "unifai-") ||
			keyStr == "UnifAIContextKeyRequestID" ||
			keyStr == "trace_id" ||
			keyStr == "span_id" {
			return
		}
		// Store as path parameter
		req.PathParams[keyStr] = valueStr
	})

	// Skip body copy for large payloads.
	// Check threshold first (set by RequestThresholdMiddleware before this middleware runs)
	// because the large-payload-mode flag is only set later inside the handler hook.
	if threshold, ok := ctx.UserValue(schemas.UnifAIContextKeyLargePayloadRequestThreshold).(int64); ok && threshold > 0 {
		cl := int64(ctx.Request.Header.ContentLength())
		// Skip body copy when CL exceeds threshold OR CL is unknown (streaming/
		// chunked, e.g. after streaming decompression deletes the header).
		if cl > threshold || cl < 0 {
			return
		}
	}
	if isLargePayload, ok := ctx.UserValue(schemas.UnifAIContextKeyLargePayloadMode).(bool); ok && isLargePayload {
		return
	}
	body := ctx.Request.Body()
	if len(body) > 0 {
		req.Body = make([]byte, len(body))
		copy(req.Body, body)
	}
}

// applyHTTPRequestToCtx applies modifications from HTTPRequest back to fasthttp context.
func applyHTTPRequestToCtx(ctx *fasthttp.RequestCtx, req *schemas.HTTPRequest) {
	// If path/method is different, throw error
	if req.Method != string(ctx.Method()) || req.Path != string(ctx.Path()) {
		logger.Error("request method/path mismatch: %s %s != %s %s", req.Method, req.Path, string(ctx.Method()), string(ctx.Path()))
		SendError(ctx, fasthttp.StatusConflict, "request method/path was modified by a plugin, this is not allowed")
		return
	}
	// Apply headers
	for key, value := range req.Headers {
		ctx.Request.Header.Set(key, value)
	}
	// Apply query params
	for key, value := range req.Query {
		ctx.Request.URI().QueryArgs().Set(key, value)
	}
	// Apply body if set
	if req.Body != nil {
		ctx.Request.SetBody(req.Body)
	}
}

// applyHTTPResponseToCtx writes a short-circuit response to fasthttp context.
func applyHTTPResponseToCtx(ctx *fasthttp.RequestCtx, resp *schemas.HTTPResponse) {
	ctx.SetStatusCode(resp.StatusCode)
	for key, value := range resp.Headers {
		ctx.Response.Header.Set(key, value)
	}
	if resp.Body != nil {
		ctx.SetBody(resp.Body)
	}
}

// fasthttpResponseToHTTPResponse populates a pooled HTTPResponse from fasthttp context.
func fasthttpResponseToHTTPResponse(ctx *fasthttp.RequestCtx, resp *schemas.HTTPResponse) {
	resp.StatusCode = ctx.Response.StatusCode()
	for key, value := range ctx.Response.Header.All() {
		resp.Headers[string(key)] = string(value)
	}
	// Skip response body copy for streaming (SSE) responses — the body is an active
	// io.Reader consumed by fasthttp's writeBodyChunked. Calling Body() would race
	// with the chunked writer (Body() drains and closes the bodyStream).
	if deferred, ok := ctx.UserValue(schemas.UnifAIContextKeyDeferTraceCompletion).(bool); ok && deferred {
		return
	}
	// Skip response body copy when large payload/response mode is active — the response is
	// streamed directly to the client and materializing it here would spike memory.
	if isLargePayload, ok := ctx.UserValue(schemas.UnifAIContextKeyLargePayloadMode).(bool); ok && isLargePayload {
		return
	}
	if isLargeResponse, ok := ctx.UserValue(lib.FastHTTPUserValueLargeResponseMode).(bool); ok && isLargeResponse {
		return
	}
	// Also skip if response Content-Length exceeds the configured response threshold.
	if threshold, ok := ctx.UserValue(schemas.UnifAIContextKeyLargeResponseThreshold).(int64); ok && threshold > 0 {
		if int64(ctx.Response.Header.ContentLength()) > threshold {
			return
		}
	}
	body := ctx.Response.Body()
	if len(body) > 0 {
		resp.Body = make([]byte, len(body))
		copy(resp.Body, body)
	}
}

// isWorkspaceAdminRole is true for dashboard super-admin and sub-admin.
// Every other role (user, developer, custom RBAC) is treated as a Prompt Repo member.
func isWorkspaceAdminRole(role string) bool {
	role = strings.ToLower(strings.TrimSpace(role))
	return role == "admin" || role == "sub_admin"
}

// markLocalAdminIfSessionAdmin sets IsLocalAdmin only for admin/sub_admin sessions.
func markLocalAdminIfSessionAdmin(ctx *fasthttp.RequestCtx, store configstore.ConfigStore, token string) {
	if store == nil || token == "" {
		return
	}
	session, err := store.GetSession(context.Background(), token)
	if err != nil || session == nil {
		return
	}
	if isWorkspaceAdminRole(session.Role) {
		ctx.SetUserValue(schemas.IsLocalAdminContextKey, true)
	}
}

// enrichInferenceFromDashboardSession stamps user_id/user_name and, when a member
// has an assigned Virtual Key, binds it onto /v1 requests that arrive with a
// dashboard session cookie. External API callers without a session cookie are unchanged.
// Members with no assigned VK are allowed through (Auto / configured provider keys).
// Returns a non-empty error message when the member must be blocked.
func (m *AuthMiddleware) enrichInferenceFromDashboardSession(ctx *fasthttp.RequestCtx) string {
	if m == nil || m.store == nil {
		return ""
	}
	token := string(ctx.Request.Header.Cookie("token"))
	if token == "" {
		if existing, ok := ctx.UserValue(schemas.UnifAIContextKeySessionToken).(string); ok {
			token = existing
		}
	}
	if token == "" {
		return ""
	}
	session, err := m.store.GetSession(context.Background(), token)
	if err != nil || session == nil || session.ExpiresAt.Before(time.Now()) {
		return ""
	}
	ctx.SetUserValue(schemas.UnifAIContextKeySessionToken, token)

	dbUser, err := m.store.GetUserByUsername(context.Background(), session.Username)
	if err != nil || dbUser == nil {
		// Bootstrap env admin may have no governance_users row — allow through.
		if isWorkspaceAdminRole(session.Role) {
			if session.Username != "" {
				ctx.SetUserValue(schemas.UnifAIContextKeyUserName, session.Username)
			}
			return ""
		}
		return "User account not found. Contact your admin."
	}
	if dbUser.ID != "" {
		ctx.SetUserValue(schemas.UnifAIContextKeyUserID, dbUser.ID)
	}
	if session.Username != "" {
		ctx.SetUserValue(schemas.UnifAIContextKeyUserName, session.Username)
	}

	// Admins may pick any VK / Auto from the playground — do not force-bind.
	if isWorkspaceAdminRole(session.Role) {
		return ""
	}

	// Double-lock: enforce user personal budget limit at the gateway level
	if dbUser.Budget > 0 {
		var currentUsage float64
		hasBudgetRow := false
		if dbUser.BudgetID != nil && *dbUser.BudgetID != "" {
			if b, bErr := m.store.GetBudget(context.Background(), *dbUser.BudgetID); bErr == nil && b != nil {
				currentUsage = b.CurrentUsage
				hasBudgetRow = true
			}
		}
		if hasBudgetRow && currentUsage >= dbUser.Budget {
			return fmt.Sprintf("User personal budget limit reached ($%.2f / $%.2f). Prompt execution blocked.", currentUsage, dbUser.Budget)
		}
	}

	allowedIDs, err := ResolveAllowedVirtualKeyIDsForUser(context.Background(), m.store, dbUser.ID)
	if err != nil {
		return "Failed to resolve assigned Virtual Key. Contact your admin."
	}
	existingVK := governance.ParseVirtualKeyFromFastHTTPRequest(ctx)
	allowedValues := make(map[string]string, len(allowedIDs)) // value → id
	var firstValue string
	assignedKeys := 0
	for vkID := range allowedIDs {
		vk, gerr := m.store.GetVirtualKey(context.Background(), vkID)
		if errors.Is(gerr, configstore.ErrNotFound) {
			continue
		}
		assignedKeys++
		if gerr != nil || vk == nil || !vk.IsActiveValue() {
			continue
		}
		val := strings.TrimSpace(vk.Value.GetValue())
		if val == "" || !strings.HasPrefix(strings.ToLower(val), governance.VirtualKeyPrefix) {
			continue
		}
		allowedValues[val] = vk.ID
		if firstValue == "" {
			firstValue = val
		}
	}
	// No VK assigned (or every assigned VK was deleted): allow Prompt Repo via
	// Auto / configured provider keys. Strip any client-supplied VK so members
	// cannot use a foreign key.
	if assignedKeys == 0 {
		if existingVK != nil && *existingVK != "" {
			ctx.Request.Header.Del("Authorization")
			ctx.Request.Header.Del("x-uf-vk")
			ctx.Request.Header.Del("x-api-key")
		}
		return ""
	}
	if firstValue == "" {
		return "Assigned Virtual Key is inactive or invalid. Contact your admin."
	}

	if existingVK != nil && *existingVK != "" {
		if _, ok := allowedValues[*existingVK]; !ok {
			// Non-admin tried a foreign VK — replace with their assigned key.
			ctx.Request.Header.Set("Authorization", "Bearer "+firstValue)
			ctx.Request.Header.Del("x-uf-vk")
			ctx.Request.Header.Del("x-api-key")
		}
		return ""
	}

	// Auto / no VK header: force assigned Virtual Key so budget ticks on the right meter.
	ctx.Request.Header.Set("Authorization", "Bearer "+firstValue)
	return ""
}

// enforceCommittedPromptModelForMember locks non-admin Prompt Repo chat to the
// prompt's latest committed provider/model. Members cannot switch models via
// a modified client — the request body model is rewritten when x-uf-prompt-id is set.
func (m *AuthMiddleware) enforceCommittedPromptModelForMember(ctx *fasthttp.RequestCtx) string {
	if m == nil || m.store == nil {
		return ""
	}
	token := string(ctx.Request.Header.Cookie("token"))
	if token == "" {
		if existing, ok := ctx.UserValue(schemas.UnifAIContextKeySessionToken).(string); ok {
			token = existing
		}
	}
	if token == "" {
		return ""
	}
	session, err := m.store.GetSession(context.Background(), token)
	if err != nil || session == nil || session.ExpiresAt.Before(time.Now()) {
		return ""
	}
	if isWorkspaceAdminRole(session.Role) {
		return ""
	}
	promptID := strings.TrimSpace(string(ctx.Request.Header.Peek("x-uf-prompt-id")))
	if promptID == "" {
		return ""
	}
	prompt, err := m.store.GetPromptByID(context.Background(), promptID)
	if err != nil || prompt == nil {
		return "Prompt not found for this chat."
	}
	// Members may only run prompts in their allowlist.
	dbUser, err := m.store.GetUserByUsername(context.Background(), session.Username)
	if err != nil || dbUser == nil {
		return "User account not found. Contact your admin."
	}
	if dbUser.AllowedPromptRepos == "" {
		return "No Prompt Repositories assigned. Ask your admin to allow prompts for your user."
	}
	allowed := false
	for _, id := range strings.Split(dbUser.AllowedPromptRepos, ",") {
		if strings.TrimSpace(id) == promptID {
			allowed = true
			break
		}
	}
	if !allowed {
		return "You do not have access to this prompt."
	}
	version := prompt.LatestVersion
	if version == nil || strings.TrimSpace(version.Provider) == "" || strings.TrimSpace(version.Model) == "" {
		return "This prompt has no committed model. Ask your admin to commit a provider/model on the prompt."
	}
	expectedModel := strings.TrimSpace(version.Provider) + "/" + strings.TrimSpace(version.Model)

	body := ctx.PostBody()
	if len(body) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	payload["model"] = expectedModel
	rewritten, err := json.Marshal(payload)
	if err != nil {
		return "Failed to lock prompt model."
	}
	ctx.Request.SetBody(rewritten)
	ctx.Request.Header.SetContentLength(len(rewritten))
	return ""
}

// validateSession checks if a session token is valid and user is actively approved
func validateSession(ctx *fasthttp.RequestCtx, store configstore.ConfigStore, token string) bool {
	session, err := store.GetSession(context.Background(), token)
	if err != nil || session == nil {
		return false
	}
	if session.ExpiresAt.Before(time.Now()) {
		return false
	}

	// Verify user account still exists and is approved (unless bootstrap admin)
	authConfig, _ := store.GetAuthConfig(context.Background())
	isBootstrapAdmin := authConfig != nil && authConfig.AdminUserName.GetValue() != "" && session.Username == authConfig.AdminUserName.GetValue()
	if !isBootstrapAdmin {
		dbUser, err := store.GetUserByUsername(context.Background(), session.Username)
		if err != nil || dbUser == nil || !dbUser.IsApproved() {
			// Invalidate ghost session immediately (CWE-613)
			_ = store.DeleteSession(context.Background(), token)
			return false
		}
	}

	// Non-admin: Prompt Repository + inference + read-only config needed for playground.
	if !isWorkspaceAdminRole(session.Role) {
		path := string(ctx.Path())
		isAllowed := strings.HasPrefix(path, "/v1/") ||
			strings.HasPrefix(path, "/api/prompt-repo/") ||
			path == "/api/session/is-auth-enabled" ||
			path == "/api/session/logout" ||
			path == "/api/session/login" ||
			path == "/api/session/ws-ticket" ||
			path == "/ws" ||
			(path == "/api/config" && string(ctx.Method()) == "GET") ||
			(strings.HasPrefix(path, "/api/providers") && string(ctx.Method()) == "GET") ||
			(strings.HasPrefix(path, "/api/governance/virtual-keys") && string(ctx.Method()) == "GET") ||
			(strings.HasPrefix(path, "/api/governance/providers") && string(ctx.Method()) == "GET") ||
			(strings.HasPrefix(path, "/api/models") && string(ctx.Method()) == "GET") ||
			(path == "/api/rbac/me/permissions" && string(ctx.Method()) == "GET")
		if !isAllowed && !customRoleMayReach(store, session.Role, string(ctx.Method()), path) {
			return false
		}
	}
	return true
}

// customRoleMayReach lets custom-role sessions through to RBAC-gated routes, where
// RBACMiddleware enforces the role's permissions. Writes that manage identity, roles
// or auth settings stay admin-only so a delegated permission cannot self-escalate.
// The built-in "user" role is locked to Prompt Repository + inference: its seeded
// read permissions exist for the playground, not for browsing workspace logs.
func customRoleMayReach(store configstore.ConfigStore, role, method, path string) bool {
	if !roleUsesRBACDelegation(role) {
		return false
	}
	if _, ok := configstore.AsWorkspaceStore(store); !ok {
		return false
	}
	return customRolePathDelegable(method, path)
}

func roleUsesRBACDelegation(role string) bool {
	role = strings.ToLower(strings.TrimSpace(role))
	return role != "" && role != "user" && !isWorkspaceAdminRole(role)
}

func customRolePathDelegable(method, path string) bool {
	if rbac.PathRequirementFor(method, path) == nil {
		return false
	}
	switch strings.ToUpper(method) {
	case fasthttp.MethodGet, fasthttp.MethodHead, fasthttp.MethodOptions:
		return true
	}
	for _, prefix := range []string{"/api/roles", "/api/permissions", "/api/rbac", "/api/users", "/api/config", "/api/scim"} {
		if strings.HasPrefix(path, prefix) {
			return false
		}
	}
	return true
}

// isInferenceWSEndpoint returns true for WebSocket endpoints that should use
// standard inference auth (Bearer/Basic/VK) rather than dashboard session tokens.
func isInferenceWSEndpoint(path string) bool {
	for strings.HasPrefix(path, "/openai/") {
		path = strings.TrimPrefix(path, "/openai")
	}

	switch path {
	case "/v1/responses",
		"/responses",
		"/v1/realtime",
		"/realtime":
		return true
	default:
		return false
	}
}

func buildRealtimeTransportPathSet() map[string]struct{} {
	paths := map[string]struct{}{}
	for _, path := range integrations.OpenAIRealtimePaths("") {
		paths[path] = struct{}{}
	}
	for _, path := range integrations.OpenAIRealtimePaths("/openai") {
		paths[path] = struct{}{}
	}
	for _, path := range integrations.OpenAIRealtimeWebRTCCallsPaths("") {
		paths[path] = struct{}{}
	}
	for _, path := range integrations.OpenAIRealtimeWebRTCCallsPaths("/openai") {
		paths[path] = struct{}{}
	}
	return paths
}

func isRealtimeTransportEndpoint(path string) bool {
	_, ok := realtimeTransportPaths[path]
	return ok
}

// AuthMiddleware is a middleware that handles authentication for the API.
type AuthMiddleware struct {
	store             configstore.ConfigStore
	whitelistedRoutes atomic.Pointer[[]string]
	authConfig        atomic.Pointer[configstore.AuthConfig]
	wsTicketStore     *WSTicketStore
	tempTokensService *temptoken.Service // optional; when nil, temp-token fallback is disabled
	tempTokensEnabled atomic.Bool
}

// InitAuthMiddleware initializes the auth middleware. The tempTokens service
// is optional and still gated by client config — when nil or disabled, the
// temp-token fallback path is skipped.
func InitAuthMiddleware(store configstore.ConfigStore, wsTicketStore *WSTicketStore, tempTokensService *temptoken.Service) (*AuthMiddleware, error) {
	if store == nil {
		return nil, fmt.Errorf("store is not present")
	}
	authConfig, err := store.GetAuthConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get auth config from store: %v", err)
	}
	am := &AuthMiddleware{
		store:             store,
		authConfig:        atomic.Pointer[configstore.AuthConfig]{},
		wsTicketStore:     wsTicketStore,
		tempTokensService: tempTokensService,
	}

	am.authConfig.Store(authConfig)

	// Load whitelisted routes from client config
	clientConfig, err := store.GetClientConfig(context.Background())
	if err == nil && clientConfig != nil {
		am.whitelistedRoutes.Store(&clientConfig.WhitelistedRoutes)
		am.tempTokensEnabled.Store(clientConfig.MCPEnableTempTokenAuth)
	} else {
		emptyRoutes := []string{}
		am.whitelistedRoutes.Store(&emptyRoutes)
		am.tempTokensEnabled.Store(false)
	}

	return am, nil
}

func (m *AuthMiddleware) UpdateAuthConfig(authConfig *configstore.AuthConfig) {
	m.authConfig.Store(authConfig)
}

// UpdateWhitelistedRoutes updates the configured whitelisted routes that bypass auth middleware.
func (m *AuthMiddleware) UpdateWhitelistedRoutes(routes []string) {
	m.whitelistedRoutes.Store(&routes)
}

// UpdateTempTokenAuthEnabled updates whether scoped temp-token fallback auth is accepted.
func (m *AuthMiddleware) UpdateTempTokenAuthEnabled(enabled bool) {
	m.tempTokensEnabled.Store(enabled)
}

// tryTempTokenOrUnauthorized is the last-resort auth path: a request that
// failed every conventional credential check (no Authorization header, no
// valid cookie) is given one more chance to present an X-UnifAI-Temp-Token
// header that authorizes the specific (method, path) being requested. On
// success the validated scope and resource_id are attached to ctx for
// handler-side defense-in-depth checks, and the next handler runs. On
// failure (no header, expired, route-mismatch, etc.) a 401 is written.
//
// Temp-token validation is intentionally *not* attempted when an
// Authorization header or session cookie is present — those paths have
// their own success/failure semantics and silently rescuing a bad password
// with a temp token would be surprising.
func (m *AuthMiddleware) tryTempTokenOrUnauthorized(ctx *fasthttp.RequestCtx, next fasthttp.RequestHandler) {
	if m.tempTokensService != nil && m.tempTokensEnabled.Load() {
		token := string(ctx.Request.Header.Peek("X-UnifAI-Temp-Token"))
		if token != "" {
			validated, err := m.tempTokensService.Validate(ctx, token, string(ctx.Method()), string(ctx.Path()))
			if err == nil && validated != nil {
				ctx.SetUserValue(schemas.UnifAIContextKeyTempTokenScope, validated.Scope)
				ctx.SetUserValue(schemas.UnifAIContextKeyTempTokenResourceID, validated.ResourceID)
				next(ctx)
				return
			}
		}
	}
	SendError(ctx, fasthttp.StatusUnauthorized, "Unauthorized")
}

// InferenceMiddleware is for inference requests (including MCP routes).
// It does not require dashboard auth, but when a session cookie is present it
// stamps user_id and, if the member has an assigned Virtual Key, auto-binds it
// so Prompt Repo usage hits the correct VK → Team → Customer budget chain.
// Members with a session but no assigned VK may still run via Auto / provider keys.
func (m *AuthMiddleware) InferenceMiddleware() schemas.UnifAIHTTPMiddleware {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			if errMsg := m.enrichInferenceFromDashboardSession(ctx); errMsg != "" {
				SendError(ctx, fasthttp.StatusForbidden, errMsg)
				return
			}
			if errMsg := m.enforceCommittedPromptModelForMember(ctx); errMsg != "" {
				SendError(ctx, fasthttp.StatusForbidden, errMsg)
				return
			}
			next(ctx)
		}
	}
}

// APIMiddleware is for API requests if authConfig is set, it will verify authentication based on the request type.
// Authentication methods:
//   - Bearer token: session validation via validateSession(). Used for dashboard calls.
//   - Cookie / WebSocket ticket: session validation for the dashboard UI.
//
// Basic auth (admin username/password) is intentionally rejected on dashboard
// API routes — it bypassed session revoke/logout and put long-lived credentials
// on every request. Inference uses virtual keys via InferenceMiddleware.
func (m *AuthMiddleware) APIMiddleware() schemas.UnifAIHTTPMiddleware {
	systemWhitelistedRoutes := []string{
		"/api/session/is-auth-enabled",
		"/api/session/login",
		"/api/session/register",
		"/api/session/register/verify",
		"/api/session/register/resend",
		"/api/session/forgot-password",
		"/api/session/verify-otp",
		"/api/session/reset-password",
		"/api/session/forgot-username",
		"/api/oauth/callback",
		"/health",
		"/login",
		"/favicon.ico",
		"/assets/*",
		"/api/scim/oauth/config",
		"/api/scim/oauth/callback",
		"/api/scim/oauth/refresh",
		"/api/scim/oauth/logout",
		"/health",
		"/api/version",
	}
	whitelistedPrefixes := []string{
		// "/api/oauth/callback" is also in systemWhitelistedRoutes above as an
		// exact match — that's the only OAuth route that must be public (it's
		// hit by the browser after the upstream provider redirects back, with
		// no cookie context). DO NOT add a broad "/api/oauth" prefix here:
		// it would whitelist /api/oauth/per-user/* (auth-via-temp-token) and
		// /api/oauth/config/* (admin-only) and bypass the temp-token fallback
		// Skills serving endpoints are public — marketplace URLs cannot carry
		// credentials securely. Management endpoints under /api/skills (without
		// /serve/) remain authenticated.
		"/api/skills/serve/",
		// OAuth2 discovery endpoints (RFC 8414 AS metadata, RFC 9728 protected
		// resource metadata, RFC 7517 JWKS) must be reachable without auth so
		// clients can bootstrap the flow. Each handler still gates availability
		// behind discoveryEnabled() and serves 404 when OAuth mode is off.
		"/.well-known/",
	}
	return m.middleware(func(authConfig *configstore.AuthConfig, url string) bool {
		if slices.Contains(systemWhitelistedRoutes, url) ||
			slices.IndexFunc(whitelistedPrefixes, func(prefix string) bool {
				return strings.HasPrefix(url, prefix)
			}) != -1 {
			return true
		}
		// Check user-configured whitelisted routes
		if configuredRoutes := m.whitelistedRoutes.Load(); configuredRoutes != nil {
			if slices.Contains(*configuredRoutes, url) || slices.IndexFunc(*configuredRoutes, func(route string) bool {
				if before, ok := strings.CutSuffix(route, "*"); ok {
					return strings.HasPrefix(url, before)
				}
				return false
			}) != -1 {
				return true
			}
		}
		return false
	})
}

// isPublicBrowserAIRoute allows Guard EXE (no session cookie) only for the
// exact method+path pairs it needs. Admin mutations stay auth-protected.
// Policy GETs (targets/rules/controls/fleet) are NOT public — Guard key or session required.
func isPublicBrowserAIRoute(method, path string) bool {
	method = strings.ToUpper(strings.TrimSpace(method))
	switch path {
	case "/api/browser-ai/intercept", "/api/browser-ai/intercept-file":
		return method == fasthttp.MethodPost
	case "/api/browser-ai/search-logs":
		return method == fasthttp.MethodPost
	case "/api/browser-ai/proxy.pac", "/api/browser-ai/pac":
		return method == fasthttp.MethodGet
	case "/api/browser-ai/agents/heartbeat",
		"/api/browser-ai/agents/uninstall-verify",
		"/api/browser-ai/agents/uninstall",
		"/api/browser-ai/agents/uninstall-ack",
		"/api/browser-ai/agents/uninstall-status":
		return method == fasthttp.MethodPost
	default:
		return false
	}
}

// isGuardKeyBrowserAIRoute is reachable with a valid X-UnifAI-Guard-Key (or session auth).
func isGuardKeyBrowserAIRoute(method, path string) bool {
	method = strings.ToUpper(strings.TrimSpace(method))
	switch path {
	case "/api/browser-ai/targets",
		"/api/browser-ai/rules",
		"/api/browser-ai/controls",
		"/api/browser-ai/fleet-config":
		return method == fasthttp.MethodGet
	// Guard auto-update + admin Setup download: Guard secret OR dashboard session.
	case "/api/browser-ai/setup/download.zip",
		"/api/browser-ai/setup/download-windows.zip",
		"/api/browser-ai/setup/download-mac.zip":
		return method == fasthttp.MethodGet || method == fasthttp.MethodHead
	case "/api/browser-ai/setup/proxy-bundle.json",
		"/api/browser-ai/setup/proxy-bundle.zip":
		return method == fasthttp.MethodGet
	default:
		return false
	}
}

// middleware is the core authentication middleware that checks if the request should be authenticated or not.
func (m *AuthMiddleware) middleware(shouldSkip func(*configstore.AuthConfig, string) bool) schemas.UnifAIHTTPMiddleware {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			// We will first check if its API key auth
			// If yes; we will skip this middleware
			if isAPIKeyAuth, ok := ctx.UserValue(schemas.IsAPIKeyAuthContextKey).(bool); ok && isAPIKeyAuth {
				next(ctx)
				return
			}
			authConfig := m.authConfig.Load()
			if authConfig == nil || !authConfig.IsEnabled {
				if !allowOpenAuthWhenDisabled(ctx) {
					SendError(ctx, fasthttp.StatusUnauthorized, "Unauthorized")
					return
				}
				ctx.SetUserValue(schemas.UnifAIContextKeySessionToken, "")
				// Mark as local admin so downstream RBAC bypasses cleanly when
				// auth is fully disabled on loopback / ALLOW_OPEN_AUTH bootstrap.
				ctx.SetUserValue(schemas.IsLocalAdminContextKey, true)
				next(ctx)
				return
			}
			// Match the whitelist against the path only
			url := string(ctx.Path())
			method := string(ctx.Method())
			// We skip authorization for the login route
			if shouldSkip(authConfig, url) || isPublicBrowserAIRoute(method, url) {
				next(ctx)
				return
			}
			// Guard agent policy sync: valid shared secret OR fall through to session auth
			if isGuardKeyBrowserAIRoute(method, url) {
				secret := configuredGuardSecret()
				if secret != "" && guardSecretMatches(extractGuardKey(ctx), secret) {
					if isGuardSyncRateLimited(clientIPAddress(ctx)) {
						SendError(ctx, fasthttp.StatusTooManyRequests, "Too many requests: Browser AI rate limit exceeded. Please wait a minute.")
						return
					}
					next(ctx)
					return
				}
				if guardSecretRequired() && secret == "" {
					SendError(ctx, fasthttp.StatusServiceUnavailable, "Guard agent secret not configured — set UNIFAI_GUARD_SECRET in .env")
					return
				}
				// No/invalid Guard key → require admin session below
			}
			if isRealtimeTransportEndpoint(url) {
				next(ctx)
				return
			}
			// If inference is disabled, we skip authorization
			// Get the authorization header
			authorization := string(ctx.Request.Header.Peek("Authorization"))
			if authorization == "" {
				if string(ctx.Request.Header.Peek("Upgrade")) == "websocket" {
					path := string(ctx.Path())
					if isInferenceWSEndpoint(path) {
						// Inference WS endpoints (/v1/responses, /v1/realtime) use the same
						// auth as HTTP inference: Bearer/Basic headers or governance VK validation.
						// If no Authorization header, fall through to return 401 below
						// (or the shouldSkip check above already passed them through).
					} else {
						// Prefer short-lived ticket-based auth (from POST /api/session/ws-ticket)
						ticket := string(ctx.Request.URI().QueryArgs().Peek("ticket"))
						if ticket != "" && m.wsTicketStore != nil {
							sessionToken := m.wsTicketStore.Consume(ticket)
							if sessionToken != "" && validateSession(ctx, m.store, sessionToken) {
								ctx.SetUserValue(schemas.UnifAIContextKeySessionToken, sessionToken)
								markLocalAdminIfSessionAdmin(ctx, m.store, sessionToken)
								next(ctx)
								return
							}
							SendError(ctx, fasthttp.StatusUnauthorized, "Unauthorized")
							return
						}
						// Cookie-based WS auth only (legacy ?token= query auth removed — token leak risk).
						cookieToken := string(ctx.Request.Header.Cookie("token"))
						if cookieToken != "" && validateSession(ctx, m.store, cookieToken) {
							ctx.SetUserValue(schemas.UnifAIContextKeySessionToken, cookieToken)
							markLocalAdminIfSessionAdmin(ctx, m.store, cookieToken)
							next(ctx)
							return
						}
						SendError(ctx, fasthttp.StatusUnauthorized, "Unauthorized")
						return
					}
				}
				// Cookie-based auth fallback: if no Authorization header, check for the HTTPOnly session cookie.
				// This supports the dashboard which relies on cookies instead of localStorage tokens.
				cookieToken := string(ctx.Request.Header.Cookie("token"))
				if cookieToken != "" && validateSession(ctx, m.store, cookieToken) {
					ctx.SetUserValue(schemas.UnifAIContextKeySessionToken, cookieToken)
					markLocalAdminIfSessionAdmin(ctx, m.store, cookieToken)
					next(ctx)
					return
				}
				// Last-resort: a scoped temp token (e.g. for the MCP per-user
				// OAuth auth page accessed by a non-admin browser) can rescue
				// this request when it targets a route the token authorizes.
				m.tryTempTokenOrUnauthorized(ctx, next)
				return
			}
			// Split the authorization header into the scheme and the token
			scheme, token, ok := strings.Cut(authorization, " ")
			if !ok {
				SendError(ctx, fasthttp.StatusUnauthorized, "Unauthorized")
				return
			}
			// Checking basic auth — rejected on dashboard API routes.
			// Admin username/password must not authenticate /api/* (no session
			// revoke/logout). Use cookie/Bearer session instead. Inference uses VK.
			if scheme == "Basic" {
				SendError(ctx, fasthttp.StatusUnauthorized, "Unauthorized")
				return
			}
			// Checking bearer auth for dashboard calls — session tokens only.
			// Password-as-Bearer (base64 user:pass) is intentionally removed: it
			// bypassed session revoke/logout and put credentials on every request.
			if scheme == "Bearer" {
				if !validateSession(ctx, m.store, token) {
					SendError(ctx, fasthttp.StatusUnauthorized, "Unauthorized")
					return
				}
				ctx.SetUserValue(schemas.UnifAIContextKeySessionToken, token)
				markLocalAdminIfSessionAdmin(ctx, m.store, token)
				next(ctx)
				return
			}
			SendError(ctx, fasthttp.StatusUnauthorized, "Unauthorized")
		}
	}
}

// TracingMiddleware creates distributed traces for requests and forwards completed traces
// to observability plugins after the response has been written.
//
// The middleware:
// 1. Extracts parent trace ID from incoming W3C traceparent header (if present)
// 2. Creates a new trace in the store (only the lightweight trace ID is stored in context)
// 3. Calls the next handler to process the request
// 4. After response is written, asynchronously completes the trace and forwards it to observability plugins
//
// This middleware should be placed early in the middleware chain to capture the full request lifecycle.
type TracingMiddleware struct {
	tracer atomic.Pointer[tracing.Tracer]
}

// collectDimensionHeaders gathers x-uf-dim-* request headers into a map keyed by
// the bare dimension name. TracingMiddleware runs before ConvertToUnifAIContext,
// so it reads the headers directly rather than UnifAIContextKeyDimensions.
func collectDimensionHeaders(ctx *fasthttp.RequestCtx) map[string]string {
	if ctx == nil {
		return nil
	}
	var dims map[string]string
	ctx.Request.Header.All()(func(key, value []byte) bool {
		keyStr := strings.ToLower(string(key))
		if labelName, ok := strings.CutPrefix(keyStr, "x-uf-dim-"); ok && labelName != "" {
			if dims == nil {
				dims = make(map[string]string)
			}
			dims[labelName] = string(value)
		}
		return true
	})
	return dims
}

// NewTracingMiddleware creates a new tracing middleware
func NewTracingMiddleware(tracer *tracing.Tracer) *TracingMiddleware {
	tm := &TracingMiddleware{
		tracer: atomic.Pointer[tracing.Tracer]{},
	}
	tm.tracer.Store(tracer)
	return tm
}

// SetObservabilityPlugins sets the observability plugins for the tracing middleware
func (m *TracingMiddleware) SetObservabilityPlugins(obsPlugins []schemas.ObservabilityPlugin) {
	if tracer := m.tracer.Load(); tracer != nil {
		tracer.SetObservabilityPlugins(obsPlugins)
	}
}

// SetTracer sets the tracer for the tracing middleware
func (m *TracingMiddleware) SetTracer(tracer *tracing.Tracer) {
	m.tracer.Store(tracer)
}

// Middleware returns the middleware function that creates distributed traces for requests and forwards completed traces
func (m *TracingMiddleware) Middleware() schemas.UnifAIHTTPMiddleware {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			// Pin the tracer for the lifetime of this request so that a concurrent
			// SetTracer() swap cannot split a trace across two instances.
			tracer := m.tracer.Load()
			if tracer == nil {
				next(ctx)
				return
			}
			requestID := string(ctx.Request.Header.Peek("x-request-id"))
			if requestID == "" {
				requestID = uuid.New().String()
				// Injecting this back to be picked up by the next middleware
				ctx.Request.Header.Set("x-request-id", requestID)
			}
			// Extract trace ID from W3C traceparent header (if present)
			// This is the 32-char trace ID that links all spans in a distributed trace
			inheritedTraceID := tracing.ExtractParentID(&ctx.Request.Header)
			// Create trace in store - only ID returned (trace data stays in store)
			traceID := tracer.CreateTrace(inheritedTraceID, requestID)
			// Store dimensions and session ID at the trace level (not as span
			// attributes) so connectors like BigQuery can export them without
			// changing the OTEL/Datadog span payloads.
			dimensions := collectDimensionHeaders(ctx)
			if len(dimensions) > 0 {
				// Hand the trace its own copy — connectors read the attribute
				// asynchronously and must never observe later mutations.
				tracer.SetTraceAttribute(traceID, schemas.TraceAttrDimensions, maps.Clone(dimensions))
			}
			if sessionID := strings.TrimSpace(string(ctx.Request.Header.Peek("x-uf-session-id"))); sessionID != "" {
				tracer.SetTraceAttribute(traceID, schemas.TraceAttrSessionID, sessionID)
			}
			// Only trace ID goes into context (lightweight, no bloat)
			ctx.SetUserValue(schemas.UnifAIContextKeyTraceID, traceID)
			// Extract parent span ID from W3C traceparent header (if present)
			// This is the 16-char span ID from the upstream service that should be
			// set as the ParentID of our root span for proper trace linking in Datadog/etc.
			parentSpanID := tracing.ExtractTraceParentSpanID(&ctx.Request.Header)
			if parentSpanID != "" {
				ctx.SetUserValue(schemas.UnifAIContextKeyParentSpanID, parentSpanID)
			}
			// Store a trace completion callback for streaming handlers to use.
			// Accepts transport plugin logs as a parameter so it never reads from
			// ctx.UserValue — ctx may be recycled by the time this runs in a goroutine.
			ctx.SetUserValue(schemas.UnifAIContextKeyTraceCompleter, func(transportLogs []schemas.PluginLogEntry) {
				if len(transportLogs) > 0 {
					tracer.AttachPluginLogs(traceID, transportLogs)
				}
				// End the root HTTP span now that the stream has fully drained, so its
				// latency covers the entire streamed response. For deferred (streaming)
				// requests the TracingMiddleware defer below intentionally leaves the root
				// span open; ending it here keeps the parent from closing before its child
				// llm.call span (which is ended by completeDeferredSpan on the final chunk).
				// Status is always Ok: deferral is only set after the stream was set up with
				// HTTP 200, and mid-stream failures surface as SSE error frames / on the
				// llm.call span, not as an HTTP error on the root.
				if rootHandle := tracer.GetSpanHandleByID(traceID, nil); rootHandle != nil {
					tracer.EndSpan(rootHandle, schemas.SpanStatusOk, "")
				}
				tracer.CompleteAndFlushTrace(traceID)
				// Guaranteed end-of-stream backstop: force-reap the stream accumulator
				// now that the stream has fully drained and the trace is flushed. This
				// covers streams that ended without a clean terminal chunk (client abort,
				// broken SSE write, or a multi-plugin refcount imbalance), which would
				// otherwise leak their accumulated (deep-copied) chunks until the TTL
				// sweep. Safe after CompleteAndFlushTrace: span completion reads the
				// separately stored accumulated response, not the live accumulator.
				tracer.ForceCleanupStreamAccumulator(traceID)
			})
			// Create root span for the HTTP request
			spanCtx, rootSpan := tracer.StartSpan(ctx, string(ctx.RequestURI()), schemas.SpanKindHTTPRequest)
			if rootSpan != nil {
				for name, value := range dimensions {
					// "path" and "method" stay reserved for the standard http.* attributes.
					if name != "path" && name != "method" {
						tracer.SetAttribute(rootSpan, name, value)
					}
				}
				tracer.SetAttribute(rootSpan, "http.method", string(ctx.Method()))
				tracer.SetAttribute(rootSpan, "http.url", string(ctx.RequestURI()))
				tracer.SetAttribute(rootSpan, "http.user_agent", string(ctx.Request.Header.UserAgent()))
				// Set root span ID in context for child span creation
				if spanID, ok := spanCtx.Value(schemas.UnifAIContextKeySpanID).(string); ok {
					ctx.SetUserValue(schemas.UnifAIContextKeySpanID, spanID)
				}
			}
			// Capture request headers onto the trace when a connector has opted in.
			// Gated so there is no overhead when no observability plugin wants headers.
			if tracer.ShouldCaptureRequestHeaders() {
				headers := make(map[string]string)
				ctx.Request.Header.All()(func(key, value []byte) bool {
					headers[strings.ToLower(string(key))] = string(value)
					return true
				})
				tracer.SetTraceRequestHeaders(traceID, headers)
			}
			defer func() {
				deferred, _ := ctx.UserValue(schemas.UnifAIContextKeyDeferTraceCompletion).(bool)
				// Record response status on the root span
				if rootSpan != nil {
					tracer.SetAttribute(rootSpan, "http.status_code", ctx.Response.StatusCode())
					// For deferred (streaming) requests, the trace completer ends the root
					// span after the stream fully drains, so its latency reflects the whole
					// streamed response. Ending it here (at handler return) would close the
					// parent before the deferred llm.call span finishes, making the child
					// span appear longer than its parent in trace viewers.
					if !deferred {
						if ctx.Response.StatusCode() >= 400 {
							tracer.EndSpan(rootSpan, schemas.SpanStatusError, fmt.Sprintf("HTTP %d", ctx.Response.StatusCode()))
						} else {
							tracer.EndSpan(rootSpan, schemas.SpanStatusOk, "")
						}
					}
				}
				// Check if trace completion is deferred (for streaming requests)
				// If deferred, the streaming handler will complete the trace (and end the
				// root span via the trace completer) after the stream ends.
				if deferred {
					return
				}
				// Attach transport plugin logs to trace before completion
				if transportLogs, ok := ctx.UserValue(schemas.UnifAIContextKeyTransportPluginLogs).([]schemas.PluginLogEntry); ok && len(transportLogs) > 0 {
					tracer.AttachPluginLogs(traceID, transportLogs)
				}
				// After response written - async flush
				tracer.CompleteAndFlushTrace(traceID)
			}()

			next(ctx)
		}
	}
}

// GetTracer returns the tracer instance for use by streaming handlers
func (m *TracingMiddleware) GetTracer() *tracing.Tracer {
	return m.tracer.Load()
}

// GetObservabilityPlugins filters and returns only observability plugins from a list of plugins.
// Uses Go type assertion to identify plugins implementing the ObservabilityPlugin interface.
func GetObservabilityPlugins(plugins []schemas.BasePlugin) []schemas.ObservabilityPlugin {
	if len(plugins) == 0 {
		return nil
	}

	obsPlugins := make([]schemas.ObservabilityPlugin, 0)
	for _, plugin := range plugins {
		if obsPlugin, ok := plugin.(schemas.ObservabilityPlugin); ok {
			obsPlugins = append(obsPlugins, obsPlugin)
		}
	}

	return obsPlugins
}
