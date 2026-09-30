// Package handlers provides HTTP request handlers for the UnifAI HTTP transport.
// This file contains common utility functions used across all handlers.
package handlers

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	unifai "github.com/unifai/unifai/core"
	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/transports/unifai-http/lib"
	"github.com/valyala/fasthttp"
)

// ResolvePeriod converts a relative period string ("1h","6h","24h","7d","30d") to concrete
// start/end time pointers computed from the current server time. Returns nil, nil for
// unrecognised values. When used in filter parsing, period takes precedence over any explicit
// start_time/end_time query parameters so every poll always covers the intended window.
func ResolvePeriod(period string) (start, end *time.Time) {
	now := time.Now().UTC()
	var from time.Time
	switch period {
	case "1h":
		from = now.Add(-time.Hour)
	case "6h":
		from = now.Add(-6 * time.Hour)
	case "24h":
		from = now.Add(-24 * time.Hour)
	case "7d":
		from = now.AddDate(0, 0, -7)
	case "30d":
		from = now.AddDate(0, 0, -30)
	default:
		return nil, nil
	}
	return &from, &now
}

// pluginDisabledKey is a dedicated context key type for marking a plugin as disabled
// rather than removed. Using a named type instead of a raw string follows Go best practices.
type pluginDisabledKey struct{}

// PluginDisabledKey is the context key used to indicate a plugin is being disabled.
var PluginDisabledKey pluginDisabledKey

// badRequestError wraps a client input validation error so that outer handlers
// can distinguish it from internal server errors and return HTTP 400.
type badRequestError struct{ err error }

func (e *badRequestError) Error() string { return e.err.Error() }
func (e *badRequestError) Unwrap() error { return e.err }

// IsUniqueConstraintError reports whether err looks like a DB unique-constraint violation.
func IsUniqueConstraintError(err error, identifiers ...string) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	hasUniqueSignal := strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "unique index") ||
		strings.Contains(msg, "unique_violation") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "duplicate entry") ||
		strings.Contains(msg, "duplicated key")
	if !hasUniqueSignal {
		return false
	}
	if len(identifiers) == 0 {
		return true
	}
	for _, identifier := range identifiers {
		if strings.Contains(msg, strings.ToLower(identifier)) {
			return true
		}
	}
	return false
}

// SendJSON sends a JSON response with 200 OK status
func SendJSON(ctx *fasthttp.RequestCtx, data interface{}) {
	ctx.SetContentType("application/json")
	if err := json.NewEncoder(ctx).Encode(data); err != nil {
		logger.Warn(fmt.Sprintf("Failed to encode JSON response: %v", err))
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to encode response: %v", err))
	}
}

// SendJSONWithStatus sends a JSON response with a custom status code
func SendJSONWithStatus(ctx *fasthttp.RequestCtx, data interface{}, statusCode int) {
	ctx.SetContentType("application/json")
	ctx.SetStatusCode(statusCode)
	if err := json.NewEncoder(ctx).Encode(data); err != nil {
		logger.Warn(fmt.Sprintf("Failed to encode JSON response: %v", err))
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to encode response: %v", err))
	}
}

// SendError sends a UnifAIError response
func SendError(ctx *fasthttp.RequestCtx, statusCode int, message string) {
	unifaiErr := &schemas.UnifAIError{
		IsUnifAIError: false,
		StatusCode:    &statusCode,
		Error: &schemas.ErrorField{
			Message: message,
		},
	}
	SendUnifAIError(ctx, unifaiErr)
}

// SendUnifAIError sends a UnifAIError response
func SendUnifAIError(ctx *fasthttp.RequestCtx, unifaiErr *schemas.UnifAIError) {
	unifaiErr = lib.SanitizeUnifAIErrorForClient(unifaiErr)
	if unifaiErr == nil {
		SendError(ctx, fasthttp.StatusInternalServerError, lib.ClientSafeInternalErrorMessage)
		return
	}

	if unifaiErr.StatusCode != nil {
		ctx.SetStatusCode(*unifaiErr.StatusCode)
	} else if !unifaiErr.IsUnifAIError {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
	} else {
		if unifaiErr.Error != nil &&
			(unifaiErr.Error.Message == unifai.ProviderAutoResolveErrorMessage ||
				unifaiErr.Error.Message == unifai.ModelAutoResolveErrorMessage) {
			ctx.SetStatusCode(fasthttp.StatusBadRequest)
		} else {
			ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		}
	}

	ctx.SetContentType("application/json")
	if encodeErr := json.NewEncoder(ctx).Encode(unifaiErr); encodeErr != nil {
		logger.Warn(fmt.Sprintf("Failed to encode error response: %v", encodeErr))
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		ctx.SetBodyString(fmt.Sprintf("Failed to encode error response: %v", encodeErr))
	}
}

// streamLargeResponseIfActive checks if large response mode was activated by the provider
// and streams the response directly to the client. Returns true if the response was handled
// (caller should return), false if normal response handling should continue.
func streamLargeResponseIfActive(ctx *fasthttp.RequestCtx, unifaiCtx *schemas.UnifAIContext) bool {
	isLargeResponse, ok := unifaiCtx.Value(schemas.UnifAIContextKeyLargeResponseMode).(bool)
	if !ok || !isLargeResponse {
		return false
	}
	if !lib.StreamLargeResponseBody(ctx, unifaiCtx) {
		SendError(ctx, fasthttp.StatusInternalServerError, "Large response reader not available")
	}
	return true
}

// SendSSEError sends an error in Server-Sent Events format
func SendSSEError(ctx *fasthttp.RequestCtx, unifaiErr *schemas.UnifAIError) {
	errorJSON, err := json.Marshal(map[string]interface{}{
		"error": unifaiErr,
	})
	if err != nil {
		logger.Error("failed to marshal error for SSE: %v", err)
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}

	if _, err := fmt.Fprintf(ctx, "data: %s\n\n", errorJSON); err != nil {
		logger.Warn(fmt.Sprintf("Failed to write SSE error: %v", err))
	}
}

// IsOriginAllowed checks if the given origin is allowed based on localhost rules and configured allowed origins.
// Localhost origins are always allowed. Additional origins can be configured in allowedOrigins.
// Supports wildcard patterns like *.example.com to match any subdomain.
func IsOriginAllowed(origin string, allowedOrigins []string) bool {
	// Always allow localhost origins
	if isLocalhostOrigin(origin) {
		return true
	}

	normOrigin := strings.TrimRight(strings.TrimSpace(origin), "/")

	// Dynamic match against environment SERVER_DOMAIN
	if serverDomain := strings.TrimRight(strings.TrimSpace(os.Getenv("SERVER_DOMAIN")), "/"); serverDomain != "" {
		if normOrigin == serverDomain {
			return true
		}
	}

	// Check configured allowed origins
	for _, allowedOrigin := range allowedOrigins {
		allowedOrigin = strings.TrimSpace(allowedOrigin)

		// Expand env references like "env.SERVER_DOMAIN"
		if strings.HasPrefix(allowedOrigin, "env.") {
			envVar := strings.TrimPrefix(allowedOrigin, "env.")
			allowedOrigin = strings.TrimRight(strings.TrimSpace(os.Getenv(envVar)), "/")
			if allowedOrigin == "" {
				continue
			}
		}

		// Check for exact match first (ignoring trailing slash)
		if allowedOrigin == origin || strings.TrimRight(allowedOrigin, "/") == normOrigin {
			return true
		}

		if allowedOrigin == "*" {
			return true
		}

		// Check for wildcard pattern
		if strings.Contains(allowedOrigin, "*") {
			if matchesWildcardPattern(origin, allowedOrigin) {
				return true
			}
		}
	}

	return false
}

// originAllowsCredentials is true only for an exact (non-wildcard) allowlist
// entry, SERVER_DOMAIN, or localhost when the API itself is on loopback / when
// ALLOW_LOCALHOST_CORS=1. Wildcard "*" / "*.example.com" must never get
// Access-Control-Allow-Credentials with cookie auth.
func originAllowsCredentials(origin string, allowedOrigins []string, requestHost string) bool {
	if origin == "" {
		return false
	}
	if isLocalhostOrigin(origin) {
		if allowLocalhostCORSCredentials() || isLocalhostHost(requestHost) {
			return true
		}
		return false
	}
	normOrigin := strings.TrimRight(strings.TrimSpace(origin), "/")
	if serverDomain := strings.TrimRight(strings.TrimSpace(os.Getenv("SERVER_DOMAIN")), "/"); serverDomain != "" {
		if normOrigin == serverDomain {
			return true
		}
	}
	for _, allowedOrigin := range allowedOrigins {
		allowedOrigin = strings.TrimSpace(allowedOrigin)
		if strings.HasPrefix(allowedOrigin, "env.") {
			envVar := strings.TrimPrefix(allowedOrigin, "env.")
			allowedOrigin = strings.TrimRight(strings.TrimSpace(os.Getenv(envVar)), "/")
			if allowedOrigin == "" {
				continue
			}
		}
		if allowedOrigin == "*" || strings.Contains(allowedOrigin, "*") {
			continue
		}
		if allowedOrigin == origin || strings.TrimRight(allowedOrigin, "/") == normOrigin {
			return true
		}
	}
	return false
}

func allowLocalhostCORSCredentials() bool {
	v := strings.TrimSpace(os.Getenv("ALLOW_LOCALHOST_CORS"))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

func isLocalhostHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// isLocalhostOrigin checks if the given origin is a localhost origin.
// Covers hostname "localhost" plus IPv4/IPv6 loopback and unspecified
// literals (127.0.0.1, ::1, 0.0.0.0, ::), bracketed or not.
func isLocalhostOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	host := parsed.Hostname() // unwraps IPv6 brackets
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified())
}

// wildcardRegexpCache caches compiled regexps for wildcard origin patterns.
// The set of patterns is small (typically 1-5) and append-only, making sync.Map
// ideal: lock-free reads on the hot path, no coordination with config reloads.
var wildcardRegexpCache sync.Map // map[string]*regexp.Regexp

// matchesWildcardPattern checks if an origin matches a wildcard pattern.
// Supports patterns like *.example.com, https://*.example.com, or http://*.example.com
func matchesWildcardPattern(origin string, pattern string) bool {
	if v, ok := wildcardRegexpCache.Load(pattern); ok {
		return v.(*regexp.Regexp).MatchString(origin)
	}

	// Convert wildcard pattern to regex pattern
	// Escape special regex characters except *
	regexPattern := regexp.QuoteMeta(pattern)
	// Replace escaped \* with regex pattern for subdomain matching
	// \* should match one or more characters that are not dots (to match a subdomain)
	regexPattern = strings.ReplaceAll(regexPattern, `\*`, `[^/.]+`)
	// Anchor the pattern to match the entire origin
	regexPattern = "^" + regexPattern + "$"

	re, err := regexp.Compile(regexPattern)
	if err != nil {
		return false
	}

	actual, _ := wildcardRegexpCache.LoadOrStore(pattern, re)
	return actual.(*regexp.Regexp).MatchString(origin)
}

// ParseModel parses a model string in the format "provider/model" or "provider/nested/model"
// Returns the provider and full model name after the first slash
func ParseModel(model string) (string, string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "", "", fmt.Errorf("model cannot be empty")
	}

	parts := strings.SplitN(model, "/", 2)
	if len(parts) < 2 {
		return "", "", fmt.Errorf("model must be in the format 'provider/model'")
	}

	provider := strings.TrimSpace(parts[0])
	name := strings.TrimSpace(parts[1])
	if provider == "" || name == "" {
		return "", "", fmt.Errorf("model must be in the format 'provider/model' with non-empty provider and model")
	}
	return provider, name, nil
}

// ClampPaginationParams applies default/max bounds to limit and offset so that
// the handler response matches the values the store actually uses.
func ClampPaginationParams(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 25
	} else if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// fuzzyMatch checks if all characters in query appear in text in order (case-insensitive)
// Example: "gpt4" matches "gpt-4", "gpt-4-turbo", etc.
func fuzzyMatch(text, query string) bool {
	if query == "" {
		return true
	}

	text = strings.ToLower(text)
	query = strings.ToLower(query)

	queryIndex := 0
	queryRunes := []rune(query)

	for _, textChar := range text {
		if queryIndex < len(queryRunes) && textChar == queryRunes[queryIndex] {
			queryIndex++
		}
	}

	return queryIndex == len(queryRunes)
}
