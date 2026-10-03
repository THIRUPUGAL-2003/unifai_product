// Package openai provides the OpenAI provider implementation for the Raksha framework.
package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bytedance/sonic"

	providerUtils "github.com/raksha/raksha/core/providers/utils"
	schemas "github.com/raksha/raksha/core/schemas"
	"github.com/valyala/fasthttp"
)

// OpenAIProvider implements the Provider interface for OpenAI's GPT API.
type OpenAIProvider struct {
	logger               schemas.Logger                // Logger for provider operations
	client               *fasthttp.Client              // HTTP client for unary API requests (ReadTimeout bounds overall response)
	streamingClient      *fasthttp.Client              // HTTP client for streaming API requests (no ReadTimeout; idle governed by NewIdleTimeoutReader)
	networkConfig        schemas.NetworkConfig         // Network configuration including extra headers
	sendBackRawRequest   bool                          // Whether to include raw request in RakshaResponse
	sendBackRawResponse  bool                          // Whether to include raw response in RakshaResponse
	customProviderConfig *schemas.CustomProviderConfig // Custom provider config
	disableStore         bool                          // Whether to force store=false on outgoing requests
}

// NewOpenAIProvider creates a new OpenAI provider instance.
// It initializes the HTTP client with the provided configuration and sets up response pools.
// The client is configured with timeouts, concurrency limits, and optional proxy settings.
func NewOpenAIProvider(config *schemas.ProviderConfig, logger schemas.Logger) *OpenAIProvider {
	config.CheckAndSetDefaults()

	requestTimeout := time.Second * time.Duration(config.NetworkConfig.DefaultRequestTimeoutInSeconds)
	client := &fasthttp.Client{
		ReadTimeout:         requestTimeout,
		WriteTimeout:        requestTimeout,
		MaxConnsPerHost:     config.NetworkConfig.MaxConnsPerHost,
		MaxIdleConnDuration: 30 * time.Second,
		MaxConnWaitTimeout:  requestTimeout,
		MaxConnDuration:     time.Second * time.Duration(schemas.DefaultMaxConnDurationInSeconds),
		ConnPoolStrategy:    fasthttp.FIFO,
	}

	// // Pre-warm response pools
	// for range config.ConcurrencyAndBufferSize.Concurrency {
	// 	openAIResponsePool.Put(&schemas.RakshaResponse{})
	// }

	// Configure proxy and retry policy
	client = providerUtils.ConfigureProxy(client, config.ProxyConfig, logger)
	client = providerUtils.ConfigureDialer(client, config.NetworkConfig.AllowPrivateNetwork)
	client = providerUtils.ConfigureTLS(client, config.NetworkConfig, logger)
	streamingClient := providerUtils.BuildStreamingClient(client)
	// Set default BaseURL if not provided
	if config.NetworkConfig.BaseURL == "" {
		config.NetworkConfig.BaseURL = "https://api.openai.com"
	}
	// Strip trailing /v1 — paths already include /v1/... (avoids .../v1/v1/models → 404)
	config.NetworkConfig.BaseURL = providerUtils.NormalizeOpenAICompatibleBaseURL(config.NetworkConfig.BaseURL)

	return &OpenAIProvider{
		logger:               logger,
		client:               client,
		streamingClient:      streamingClient,
		networkConfig:        config.NetworkConfig,
		sendBackRawRequest:   config.SendBackRawRequest,
		sendBackRawResponse:  config.SendBackRawResponse,
		customProviderConfig: config.CustomProviderConfig,
		disableStore:         config.OpenAIConfig != nil && config.OpenAIConfig.DisableStore,
	}
}

// GetProviderKey returns the provider identifier for OpenAI.
func (provider *OpenAIProvider) GetProviderKey() schemas.ModelProvider {
	return providerUtils.GetProviderName(schemas.OpenAI, provider.customProviderConfig)
}

// buildRequestURL constructs the full request URL using the provider's configuration.
func (provider *OpenAIProvider) buildRequestURL(ctx *schemas.RakshaContext, defaultPath string, requestType schemas.RequestType) string {
	path, isCompleteURL := providerUtils.GetRequestPath(ctx, defaultPath, provider.customProviderConfig, requestType)
	if isCompleteURL {
		return path
	}
	return provider.networkConfig.BaseURL + path
}

func (provider *OpenAIProvider) ListModels(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaListModelsRequest) (*schemas.RakshaListModelsResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.ListModelsRequest); err != nil {
		return nil, err
	}
	providerName := provider.GetProviderKey()

	if provider.customProviderConfig != nil && provider.customProviderConfig.IsKeyLess {
		return providerUtils.HandleKeylessListModelsRequest(providerName, func() (*schemas.RakshaListModelsResponse, *schemas.RakshaError) {
			return ListModelsByKey(
				ctx,
				provider.client,
				provider.buildRequestURL(ctx, "/v1/models", schemas.ListModelsRequest),
				schemas.Key{Models: schemas.WhiteList{"*"}},
				request.Unfiltered,
				provider.networkConfig.ExtraHeaders,
				providerName,
				providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
				providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
			)
		})
	}

	return HandleOpenAIListModelsRequest(ctx,
		provider.client,
		request,
		provider.buildRequestURL(ctx, "/v1/models", schemas.ListModelsRequest),
		keys,
		provider.networkConfig.ExtraHeaders,
		providerName,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
	)
}

// ListModelsByKey performs a list models request for a single key.
// Returns the list-models response, or an error if the request fails.
func ListModelsByKey(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	url string,
	key schemas.Key,
	unfiltered bool,
	extraHeaders map[string]string,
	providerName schemas.ModelProvider,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
) (*schemas.RakshaListModelsResponse, *schemas.RakshaError) {
	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)

	req.SetRequestURI(url)
	req.Header.SetMethod(http.MethodGet)
	req.Header.SetContentType("application/json")

	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	// Make request
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}
	// Extract provider response headers early so they're available on error paths too
	providerResponseHeaders := providerUtils.ExtractProviderResponseHeaders(resp)
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		rakshaErr := ParseOpenAIError(resp)
		return nil, providerUtils.SetErrorLatency(rakshaErr, latency)
	}

	// Copy response body before releasing
	responseBody := append([]byte(nil), resp.Body()...)

	openaiResponse, parseErr := parseOpenAIListModelsBody(responseBody)
	if parseErr != nil {
		return nil, providerUtils.SetErrorLatency(providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseUnmarshal, parseErr), latency)
	}

	response := openaiResponse.ToRakshaListModelsResponse(providerName, key.Models, key.BlacklistedModels, key.Aliases, unfiltered)

	response.ExtraFields.Latency = latency.Milliseconds()
	response.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	// Set raw response if enabled (GET has no request body).
	if sendBackRawResponse {
		response.ExtraFields.RawResponse = responseBody
	}

	return response, nil
}

// parseOpenAIListModelsBody accepts both OpenAI's {object,data} envelope and a bare
// model array (Together AI and several other OpenAI-compat vendors).
func parseOpenAIListModelsBody(responseBody []byte) (*OpenAIListModelsResponse, error) {
	trimmed := bytes.TrimSpace(responseBody)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty list models response")
	}
	if trimmed[0] == '[' {
		var models []OpenAIModel
		if err := sonic.Unmarshal(trimmed, &models); err != nil {
			return nil, err
		}
		return &OpenAIListModelsResponse{Object: "list", Data: models}, nil
	}
	out := &OpenAIListModelsResponse{}
	if err := sonic.Unmarshal(trimmed, out); err != nil {
		return nil, err
	}
	return out, nil
}

// BearerAuthHeader builds the auth header map for OpenAI-compatible providers that authenticate
// with a bearer token. It returns an empty (non-nil) map when the key carries no value (e.g.
// SigV4 / header-based auth supplied via extraHeaders), so callers can pass it directly to the
// Handle*Request functions that take an authHeader map.
func BearerAuthHeader(key schemas.Key) map[string]string {
	headers := map[string]string{}
	if key.Value.GetValue() != "" {
		headers["Authorization"] = "Bearer " + key.Value.GetValue()
	}
	return headers
}

// HandleOpenAIListModelsRequest handles a list models request to OpenAI's API.
func HandleOpenAIListModelsRequest(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	request *schemas.RakshaListModelsRequest,
	url string,
	keys []schemas.Key,
	extraHeaders map[string]string,
	providerName schemas.ModelProvider,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
) (*schemas.RakshaListModelsResponse, *schemas.RakshaError) {
	if len(keys) == 0 {
		return ListModelsByKey(ctx, client, url, schemas.Key{}, request.Unfiltered, extraHeaders, providerName, sendBackRawRequest, sendBackRawResponse)
	}
	listModelsByKeyWrapper := func(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaListModelsRequest) (*schemas.RakshaListModelsResponse, *schemas.RakshaError) {
		return ListModelsByKey(ctx, client, url, key, request.Unfiltered, extraHeaders, providerName, sendBackRawRequest, sendBackRawResponse)
	}
	return providerUtils.HandleMultipleListModelsRequests(
		ctx,
		keys,
		request,
		listModelsByKeyWrapper,
	)
}

// TextCompletion is not supported by the OpenAI provider.
// Returns an error indicating that text completion is not available.
func (provider *OpenAIProvider) TextCompletion(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaTextCompletionRequest) (*schemas.RakshaTextCompletionResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.TextCompletionRequest); err != nil {
		return nil, err
	}
	return HandleOpenAITextCompletionRequest(
		ctx,
		provider.client,
		provider.buildRequestURL(ctx, "/v1/completions", schemas.TextCompletionRequest),
		request,
		BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		provider.GetProviderKey(),
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		nil,
		nil,
		provider.logger,
	)
}

// HandleOpenAITextCompletionRequest handles a text completion request to OpenAI's API.
func HandleOpenAITextCompletionRequest(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	url string,
	request *schemas.RakshaTextCompletionRequest,
	authHeader map[string]string,
	extraHeaders map[string]string,
	providerName schemas.ModelProvider,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
	customResponseHandler responseHandler[schemas.RakshaTextCompletionResponse],
	customErrorConverter ErrorConverter,
	logger schemas.Logger,
) (*schemas.RakshaTextCompletionResponse, *schemas.RakshaError) {
	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	// resp lifecycle: managed by finalizeOpenAIResponse or released on error paths
	respOwned := true
	defer func() {
		if respOwned {
			fasthttp.ReleaseResponse(resp)
		}
	}()
	activeClient := providerUtils.PrepareResponseStreaming(ctx, client, resp)

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)

	req.SetRequestURI(url)
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")

	for k, v := range authHeader {
		req.Header.Set(k, v)
	}

	// Large payload passthrough: stream body directly without JSON marshaling
	if lpResult, lpErr, handled := handleOpenAILargePayloadPassthrough(ctx, client, url, authHeader, extraHeaders, providerName, logger); handled {
		if lpErr != nil {
			return nil, lpErr
		}
		if len(lpResult.ResponseBody) > 0 {
			response := &schemas.RakshaTextCompletionResponse{}
			if err := sonic.Unmarshal(lpResult.ResponseBody, response); err != nil {
				return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseUnmarshal, err)
			}
			response.ExtraFields = schemas.RakshaResponseExtraFields{Latency: lpResult.Latency}
			return response, nil
		}
		return &schemas.RakshaTextCompletionResponse{
			Model:       request.Model,
			Usage:       lpResult.Usage,
			ExtraFields: schemas.RakshaResponseExtraFields{Latency: lpResult.Latency},
		}, nil
	}

	jsonData, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToOpenAITextCompletionRequest(request), nil
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	req.SetBody(jsonData)

	// Make request
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}
	// Extract provider response headers early so they're available on error paths too
	providerResponseHeaders := providerUtils.ExtractProviderResponseHeaders(resp)
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		if customErrorConverter != nil {
			return nil, providerUtils.EnrichError(ctx, customErrorConverter(resp), jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
		}
		return nil, providerUtils.EnrichError(ctx, ParseOpenAIError(resp), jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	body, lpResult, finalErr := finalizeOpenAIResponse(ctx, resp, latency, providerName, logger)
	respOwned = false // ownership transferred
	if finalErr != nil {
		return nil, providerUtils.EnrichError(ctx, finalErr, jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}
	if lpResult != nil {
		return &schemas.RakshaTextCompletionResponse{
			Model:       request.Model,
			Usage:       lpResult.Usage,
			ExtraFields: schemas.RakshaResponseExtraFields{Latency: lpResult.Latency},
		}, nil
	}

	response := &schemas.RakshaTextCompletionResponse{}

	var rawRequest, rawResponse interface{}

	if customResponseHandler != nil {
		rawRequest, rawResponse, rakshaErr = customResponseHandler(body, response, jsonData, sendBackRawRequest, sendBackRawResponse)
	} else {
		rawRequest, rawResponse, rakshaErr = providerUtils.HandleProviderResponse(body, response, jsonData, sendBackRawRequest, sendBackRawResponse)
	}

	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, body, sendBackRawRequest, sendBackRawResponse, latency)
	}

	response.ExtraFields.Latency = latency.Milliseconds()
	response.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	// Set raw request if enabled
	if providerUtils.ShouldSendBackRawRequest(ctx, sendBackRawRequest) {
		response.ExtraFields.RawRequest = rawRequest
	}

	// Set raw response if enabled
	if sendBackRawResponse {
		response.ExtraFields.RawResponse = rawResponse
	}

	return response, nil
}

// TextCompletionStream performs a streaming text completion request to OpenAI's API.
// It formats the request, sends it to OpenAI, and processes the response.
// Returns a channel of RakshaStreamChunk objects or an error if the request fails.
func (provider *OpenAIProvider) TextCompletionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaTextCompletionRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.TextCompletionStreamRequest); err != nil {
		return nil, err
	}
	return HandleOpenAITextCompletionStreaming(
		ctx,
		provider.streamingClient,
		provider.buildRequestURL(ctx, "/v1/completions", schemas.TextCompletionStreamRequest),
		request,
		BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		provider.networkConfig.StreamIdleTimeoutInSeconds,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		nil,
		postHookRunner,
		nil,
		nil,
		provider.logger,
		postHookSpanFinalizer,
	)
}

// HandleOpenAITextCompletionStreaming handles text completion streaming for OpenAI-compatible APIs.
// This shared function reduces code duplication between providers that use the same SSE format.
func HandleOpenAITextCompletionStreaming(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	url string,
	request *schemas.RakshaTextCompletionRequest,
	authHeader map[string]string,
	extraHeaders map[string]string,
	streamIdleTimeoutInSeconds int,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
	providerName schemas.ModelProvider,
	customErrorConverter ErrorConverter,
	postHookRunner schemas.PostHookRunner,
	customResponseHandler responseHandler[schemas.RakshaTextCompletionResponse],
	postResponseConverter func(*schemas.RakshaTextCompletionResponse) *schemas.RakshaTextCompletionResponse,
	logger schemas.Logger,
	postHookSpanFinalizer func(context.Context),
) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	providerUtils.SetStreamIdleTimeoutIfEmpty(ctx, streamIdleTimeoutInSeconds)
	headers := map[string]string{
		"Content-Type":  "application/json",
		"Accept":        "text/event-stream",
		"Cache-Control": "no-cache",
	}

	if authHeader != nil {
		maps.Copy(headers, authHeader)
	}

	jsonBody, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			reqBody := ToOpenAITextCompletionRequest(request)
			if reqBody != nil {
				reqBody.Stream = schemas.Ptr(true)
				reqBody.StreamOptions = &schemas.ChatStreamOptions{
					IncludeUsage: schemas.Ptr(true),
				}
			}
			return reqBody, nil
		})

	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Create HTTP request for streaming
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	resp.StreamBody = true
	defer fasthttp.ReleaseRequest(req)

	req.Header.SetMethod(http.MethodPost)
	req.SetRequestURI(url)
	req.Header.SetContentType("application/json")

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)

	// Set headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	setStreamingRequestBody(ctx, req, jsonBody, providerName)

	// Use streaming-aware client when large payload optimization is active — ensures
	// MaxResponseBodySize > 0 so ErrBodyTooLarge triggers StreamBody for Content-Length responses.
	activeClient := providerUtils.PrepareResponseStreaming(ctx, client, resp)

	startTime := time.Now()
	// Make the request
	err := activeClient.Do(req, resp)
	if err != nil {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		latency := time.Since(startTime)
		if errors.Is(err, context.Canceled) {
			return nil, providerUtils.EnrichError(ctx, &schemas.RakshaError{
				IsRakshaError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   err,
				},
			}, jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
		}
		if errors.Is(err, fasthttp.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
			return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaTimeoutError(schemas.ErrProviderRequestTimedOut, err), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
		}
		// The request failed before the first response byte (connection refused, server
		// closed an idle/pooled connection, broken pipe, DNS failure, etc.). Mirror the
		// non-streaming path (makeRequestWithDoFunc) and surface this as a retriable upstream
		// connection error (502, IsRakshaError=false) rather than NewRakshaOperationError
		// (500, IsRakshaError=true). The latter caused the retry loop in executeRequestWithRetries
		// to break early on IsRakshaError, so max_retries never applied to streaming connection
		// failures - see https://github.com/raksha/raksha/issues/4496.
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaUpstreamConnectionError(schemas.ErrProviderDoRequest, err), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Store provider response headers in context before status check so error responses also forward them
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	// Check for HTTP errors
	if resp.StatusCode() != fasthttp.StatusOK {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		latency := time.Since(startTime)
		if customErrorConverter != nil {
			return nil, providerUtils.EnrichError(ctx, customErrorConverter(resp), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
		}
		return nil, providerUtils.EnrichError(ctx, ParseOpenAIError(resp), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	latency := time.Since(startTime)

	// Large payload streaming passthrough — pipe raw upstream SSE to client
	if providerUtils.SetupStreamingPassthrough(ctx, resp) {
		responseChan := make(chan *schemas.RakshaStreamChunk)
		providerUtils.CloseStream(ctx, responseChan)
		return responseChan, nil
	}

	// Create response channel
	responseChan := make(chan *schemas.RakshaStreamChunk, schemas.DefaultStreamBufferSize)

	// Start streaming in a goroutine
	go func() {
		defer providerUtils.EnsureStreamFinalizerCalled(ctx, postHookSpanFinalizer)
		defer func() {
			if ctx.Err() == context.Canceled {
				providerUtils.HandleStreamCancellation(ctx, postHookRunner, responseChan, logger, postHookSpanFinalizer, jsonBody)
			} else if ctx.Err() == context.DeadlineExceeded {
				providerUtils.HandleStreamTimeout(ctx, postHookRunner, responseChan, logger, postHookSpanFinalizer, jsonBody)
			}
			providerUtils.CloseStream(ctx, responseChan)
		}()
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		// Decompress gzip-encoded streams transparently (no-op for non-gzip)
		reader, releaseGzip := providerUtils.DecompressStreamBody(resp)
		defer releaseGzip()

		// Wrap reader with idle timeout to detect stalled streams.
		reader, stopIdleTimeout := providerUtils.NewIdleTimeoutReader(reader, resp.BodyStream(), providerUtils.GetStreamIdleTimeout(ctx), ctx)
		defer stopIdleTimeout()

		// Setup cancellation handler to close the raw network stream on ctx cancellation,
		// which immediately unblocks any in-progress read (including reads blocked inside a gzip decompression layer).
		stopCancellation := providerUtils.SetupStreamCancellation(ctx, resp.BodyStream(), logger)
		defer stopCancellation()

		// Skip scanner for non-SSE responses — avoids bufio.Scanner buffer bloat
		// on non-line-delimited data (e.g. provider returned JSON instead of SSE).
		reader, drained := providerUtils.DrainNonSSEStreamReader(resp, reader)
		if drained {
			ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
			providerUtils.ProcessAndSendError(ctx, postHookRunner, errors.New("provider returned non-SSE response for streaming request"), responseChan, logger, postHookSpanFinalizer)
			return
		}

		sseReader := providerUtils.GetSSEDataReader(ctx, reader)

		chunkIndex := -1
		usage := &schemas.RakshaLLMUsage{}
		// Register the accumulating usage handle so a mid-stream
		// cancel/timeout can bill for tokens the provider already processed.
		ctx.SetValue(schemas.RakshaContextKeyStreamAccumulatedUsage, usage)

		var finishReason *string
		var messageID string
		lastChunkTime := startTime

		for {
			// If context was cancelled/timed out, let defer handle it
			if ctx.Err() != nil {
				return
			}
			data, readErr := sseReader.ReadDataLine()
			if readErr != nil {
				if ctx.Err() != nil {
					return
				}
				if readErr != io.EOF {
					ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
					logger.Warn("Error reading stream: %v", readErr)
					providerUtils.ProcessAndSendError(ctx, postHookRunner, readErr, responseChan, logger, postHookSpanFinalizer)
					return
				}
				break
			}
			jsonData := string(data)
			var response schemas.RakshaTextCompletionResponse
			if customResponseHandler != nil {
				rawRequest, rawResponse, handlerErr := customResponseHandler([]byte(jsonData), &response, nil, sendBackRawRequest, sendBackRawResponse)
				if handlerErr != nil {
					// TODO fix this
					if sendBackRawRequest {
						handlerErr.ExtraFields.RawRequest = rawRequest
					}
					if sendBackRawResponse {
						handlerErr.ExtraFields.RawResponse = rawResponse
					}
					ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
					providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, providerUtils.EnrichError(ctx, handlerErr, jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency), responseChan, logger, postHookSpanFinalizer)
					return
				}
			} else {

				// Quick check for error field (allocation-free using sonic.GetFromString)
				if errorNode, _ := sonic.GetFromString(jsonData, "error"); errorNode.Exists() {
					// Only unmarshal when we know there's an error
					var rakshaErr schemas.RakshaError
					if err := sonic.UnmarshalString(jsonData, &rakshaErr); err == nil {
						if rakshaErr.Error != nil && rakshaErr.Error.Message != "" {
							ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
							providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, providerUtils.EnrichError(ctx, &rakshaErr, jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency), responseChan, logger, postHookSpanFinalizer)
							return
						}
					}
				}

				// Parse into raksha response
				if err := sonic.UnmarshalString(jsonData, &response); err != nil {
					logger.Warn("Failed to parse stream response: %v", err)
					continue
				}
			}

			// choices be array if nil
			if response.Choices == nil {
				response.Choices = []schemas.RakshaResponseChoice{}
			}

			if postResponseConverter != nil {
				if converted := postResponseConverter(&response); converted != nil {
					response = *converted
				} else {
					logger.Warn("postResponseConverter returned nil; leaving chunk unmodified")
				}
			}

			// Handle usage-only chunks (when stream_options include_usage is true)
			if response.Usage != nil {
				// Collect usage information and send at the end of the stream
				// Here in some cases usage comes before final message
				// So we need to check if the response.Usage is nil and then if usage != nil
				// then add up all tokens
				if response.Usage.PromptTokens > usage.PromptTokens {
					usage.PromptTokens = response.Usage.PromptTokens
				}
				if response.Usage.CompletionTokens > usage.CompletionTokens {
					usage.CompletionTokens = response.Usage.CompletionTokens
				}
				if response.Usage.TotalTokens > usage.TotalTokens {
					usage.TotalTokens = response.Usage.TotalTokens
				}
				calculatedTotal := usage.PromptTokens + usage.CompletionTokens
				if calculatedTotal > usage.TotalTokens {
					usage.TotalTokens = calculatedTotal
				}
				if response.Usage.CompletionTokensDetails != nil {
					usage.CompletionTokensDetails = response.Usage.CompletionTokensDetails
				}
				if response.Usage.PromptTokensDetails != nil {
					usage.PromptTokensDetails = response.Usage.PromptTokensDetails
				}
				response.Usage = nil
			}

			// Skip empty responses or responses without choices
			if len(response.Choices) == 0 {
				continue
			}

			// Handle finish reason, usually in the final chunk
			choice := response.Choices[0]
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				// Collect finish reason and send at the end of the stream
				finishReason = choice.FinishReason
				response.Choices[0].FinishReason = nil
			}

			if response.ID != "" && messageID == "" {
				messageID = response.ID
			}

			// Handle regular content chunks
			if choice.TextCompletionResponseChoice != nil && choice.TextCompletionResponseChoice.Text != nil {
				chunkIndex++

				response.ExtraFields.ChunkIndex = chunkIndex
				response.ExtraFields.Latency = time.Since(lastChunkTime).Milliseconds()
				lastChunkTime = time.Now()

				if sendBackRawResponse {
					response.ExtraFields.RawResponse = jsonData
				}

				providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(&response, nil, nil, nil, nil, nil), responseChan, postHookSpanFinalizer)
			}

			// For providers that don't send [DONE] marker break on finish_reason
			if !providerUtils.ProviderSendsDoneMarker(providerName) && finishReason != nil {
				break
			}
		}

		response := providerUtils.CreateRakshaTextCompletionChunkResponse(messageID, usage, finishReason, chunkIndex, schemas.TextCompletionStreamRequest, request.Model)
		if postResponseConverter != nil {
			response = postResponseConverter(response)
			if response == nil {
				logger.Warn("postResponseConverter returned nil; leaving chunk unmodified")
				return
			}
		}
		// Set raw request if enabled
		if sendBackRawRequest {
			providerUtils.ParseAndSetRawRequest(&response.ExtraFields, jsonBody)
		}
		response.ExtraFields.Latency = time.Since(startTime).Milliseconds()
		ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
		providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(response, nil, nil, nil, nil, nil), responseChan, postHookSpanFinalizer)
	}()

	return responseChan, nil
}

// ChatCompletion performs a chat completion request to the OpenAI API.
// It supports both text and image content in messages.
// Returns a RakshaResponse containing the completion results or an error if the request fails.
func (provider *OpenAIProvider) ChatCompletion(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaChatRequest) (*schemas.RakshaChatResponse, *schemas.RakshaError) {
	// Check if chat completion is allowed for this provider
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.ChatCompletionRequest); err != nil {
		return nil, err
	}

	if provider.disableStore {
		if request.Params == nil {
			request.Params = &schemas.ChatParameters{}
		}
		request.Params.Store = schemas.Ptr(false)
	}

	return HandleOpenAIChatCompletionRequest(
		ctx,
		provider.client,
		provider.buildRequestURL(ctx, "/v1/chat/completions", schemas.ChatCompletionRequest),
		request,
		BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		nil,
		nil,
		nil,
		provider.logger,
	)
}

// HandleOpenAIChatCompletionRequest handles a chat completion request to OpenAI's API.
func HandleOpenAIChatCompletionRequest(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	url string,
	request *schemas.RakshaChatRequest,
	authHeader map[string]string,
	extraHeaders map[string]string,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
	providerName schemas.ModelProvider,
	customResponseHandler responseHandler[schemas.RakshaChatResponse],
	customErrorConverter ErrorConverter,
	signer providerUtils.BodySigner,
	logger schemas.Logger,
) (*schemas.RakshaChatResponse, *schemas.RakshaError) {
	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	// resp lifecycle: managed by finalizeOpenAIResponse or released on error paths
	respOwned := true
	defer func() {
		if respOwned {
			fasthttp.ReleaseResponse(resp)
		}
	}()
	activeClient := providerUtils.PrepareResponseStreaming(ctx, client, resp)

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)

	req.SetRequestURI(url)
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")

	for k, v := range authHeader {
		req.Header.Set(k, v)
	}

	// Large payload passthrough: stream body directly without JSON marshaling
	if lpResult, lpErr, handled := handleOpenAILargePayloadPassthrough(ctx, client, url, authHeader, extraHeaders, providerName, logger); handled {
		if lpErr != nil {
			return nil, lpErr
		}
		if len(lpResult.ResponseBody) > 0 {
			response := &schemas.RakshaChatResponse{}
			if err := sonic.Unmarshal(lpResult.ResponseBody, response); err != nil {
				return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseUnmarshal, err)
			}
			response.ExtraFields = schemas.RakshaResponseExtraFields{Latency: lpResult.Latency}
			return response, nil
		}
		return &schemas.RakshaChatResponse{
			Model:       request.Model,
			Usage:       lpResult.Usage,
			ExtraFields: schemas.RakshaResponseExtraFields{Latency: lpResult.Latency},
		}, nil
	}

	jsonData, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToOpenAIChatRequest(ctx, request), nil
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	if signer != nil {
		sigHeaders, signErr := signer(jsonData)
		if signErr != nil {
			return nil, signErr
		}
		for k, v := range sigHeaders {
			req.Header.Set(k, v)
		}
	}

	req.SetBody(jsonData)

	// Make request
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}
	// Extract provider response headers early so they're available on error paths too
	providerResponseHeaders := providerUtils.ExtractProviderResponseHeaders(resp)
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		logger.Debug("error from %s provider: %s", providerName, string(resp.Body()))
		if customErrorConverter != nil {
			return nil, providerUtils.EnrichError(ctx, customErrorConverter(resp), jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
		}
		return nil, providerUtils.EnrichError(ctx, ParseOpenAIError(resp), jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	body, lpResult, finalErr := finalizeOpenAIResponse(ctx, resp, latency, providerName, logger)
	respOwned = false // ownership transferred
	if finalErr != nil {
		return nil, providerUtils.EnrichError(ctx, finalErr, jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}
	if lpResult != nil {
		return &schemas.RakshaChatResponse{
			Model:       request.Model,
			Usage:       lpResult.Usage,
			ExtraFields: schemas.RakshaResponseExtraFields{Latency: lpResult.Latency},
		}, nil
	}
	response := &schemas.RakshaChatResponse{}
	response.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	var rawRequest, rawResponse interface{}

	if customResponseHandler != nil {
		rawRequest, rawResponse, rakshaErr = customResponseHandler(body, response, jsonData, sendBackRawRequest, sendBackRawResponse)
	} else {
		rawRequest, rawResponse, rakshaErr = providerUtils.HandleProviderResponse(body, response, jsonData, sendBackRawRequest, sendBackRawResponse)
	}

	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, body, sendBackRawRequest, sendBackRawResponse, latency)
	}

	response.ExtraFields.Latency = latency.Milliseconds()

	// Set raw request if enabled
	if providerUtils.ShouldSendBackRawRequest(ctx, sendBackRawRequest) {
		response.ExtraFields.RawRequest = rawRequest
	}

	// Set raw response if enabled
	if providerUtils.ShouldSendBackRawResponse(ctx, sendBackRawResponse) {
		response.ExtraFields.RawResponse = rawResponse
	}

	return response, nil
}

// ChatCompletionStream handles streaming for OpenAI chat completions.
// It formats messages, prepares request body, and uses shared streaming logic.
// Returns a channel for streaming responses and any error that occurred.
func (provider *OpenAIProvider) ChatCompletionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaChatRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	// Check if chat completion stream is allowed for this provider
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.ChatCompletionStreamRequest); err != nil {
		return nil, err
	}
	if provider.disableStore {
		if request.Params == nil {
			request.Params = &schemas.ChatParameters{}
		}
		request.Params.Store = schemas.Ptr(false)
	}

	// Use shared streaming logic
	return HandleOpenAIChatCompletionStreaming(
		ctx,
		provider.streamingClient,
		provider.buildRequestURL(ctx, "/v1/chat/completions", schemas.ChatCompletionStreamRequest),
		request,
		BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		provider.networkConfig.StreamIdleTimeoutInSeconds,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		postHookRunner,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		provider.logger,
		postHookSpanFinalizer,
	)
}

// HandleOpenAIChatCompletionStreaming handles streaming for OpenAI-compatible APIs.
// This shared function reduces code duplication between providers that use the same SSE format.
func HandleOpenAIChatCompletionStreaming(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	url string,
	request *schemas.RakshaChatRequest,
	authHeader map[string]string,
	extraHeaders map[string]string,
	streamIdleTimeoutInSeconds int,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
	providerName schemas.ModelProvider,
	postHookRunner schemas.PostHookRunner,
	customRequestConverter func(*schemas.RakshaChatRequest) (providerUtils.RequestBodyWithExtraParams, error),
	customResponseHandler responseHandler[schemas.RakshaChatResponse],
	customErrorConverter ErrorConverter,
	postRequestConverter func(*OpenAIChatRequest) *OpenAIChatRequest,
	postResponseConverter func(*schemas.RakshaChatResponse) *schemas.RakshaChatResponse,
	signer providerUtils.BodySigner,
	logger schemas.Logger,
	postHookSpanFinalizer func(context.Context),
) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	providerUtils.SetStreamIdleTimeoutIfEmpty(ctx, streamIdleTimeoutInSeconds)
	// Check if the request is a redirect from ResponsesStream to ChatCompletionStream
	isResponsesToChatCompletionsFallback := false
	var responsesStreamState *schemas.ChatToResponsesStreamState
	if ctx.Value(schemas.RakshaContextKeyIsResponsesToChatCompletionFallback) != nil {
		isResponsesToChatCompletionsFallbackValue, ok := ctx.Value(schemas.RakshaContextKeyIsResponsesToChatCompletionFallback).(bool)
		if ok && isResponsesToChatCompletionsFallbackValue {
			isResponsesToChatCompletionsFallback = true
			responsesStreamState = schemas.AcquireChatToResponsesStreamState()
		}
	}

	headers := map[string]string{
		"Content-Type":  "application/json",
		"Accept":        "text/event-stream",
		"Cache-Control": "no-cache",
	}

	if authHeader != nil {
		// Copy auth header to headers
		maps.Copy(headers, authHeader)
	}

	jsonBody, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			if customRequestConverter != nil {
				return customRequestConverter(request)
			}
			reqBody := ToOpenAIChatRequest(ctx, request)
			if reqBody != nil {
				reqBody.Stream = new(true)
				reqBody.StreamOptions = &schemas.ChatStreamOptions{
					IncludeUsage: new(true),
				}
				if postRequestConverter != nil {
					reqBody = postRequestConverter(reqBody)
				}
			}
			return reqBody, nil
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Create HTTP request for streaming
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	resp.StreamBody = true
	defer fasthttp.ReleaseRequest(req)

	// Updating request
	req.Header.SetMethod(http.MethodPost)
	req.SetRequestURI(url)
	req.Header.SetContentType("application/json")

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)

	// Set headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	if signer != nil {
		sigHeaders, signErr := signer(jsonBody)
		if signErr != nil {
			defer providerUtils.ReleaseStreamingResponse(ctx, resp)
			return nil, signErr
		}
		for k, v := range sigHeaders {
			req.Header.Set(k, v)
		}
	}

	setStreamingRequestBody(ctx, req, jsonBody, providerName)

	// Use streaming-aware client when large payload optimization is active — ensures
	// MaxResponseBodySize > 0 so ErrBodyTooLarge triggers StreamBody for Content-Length responses.
	activeClient := providerUtils.PrepareResponseStreaming(ctx, client, resp)

	startTime := time.Now()
	// Make the request
	err := activeClient.Do(req, resp)
	latency := time.Since(startTime)
	if err != nil {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		if errors.Is(err, context.Canceled) {
			return nil, providerUtils.EnrichError(ctx, &schemas.RakshaError{
				IsRakshaError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   err,
				},
			}, jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
		}
		if errors.Is(err, fasthttp.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
			return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaTimeoutError(schemas.ErrProviderRequestTimedOut, err), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
		}
		// The request failed before the first response byte (connection refused, server
		// closed an idle/pooled connection, broken pipe, DNS failure, etc.). Mirror the
		// non-streaming path (makeRequestWithDoFunc) and surface this as a retriable upstream
		// connection error (502, IsRakshaError=false) rather than NewRakshaOperationError
		// (500, IsRakshaError=true). The latter caused the retry loop in executeRequestWithRetries
		// to break early on IsRakshaError, so max_retries never applied to streaming connection
		// failures - see https://github.com/raksha/raksha/issues/4496.
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaUpstreamConnectionError(schemas.ErrProviderDoRequest, err), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Store provider response headers in context before status check so error responses also forward them
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	// Check for HTTP errors
	if resp.StatusCode() != fasthttp.StatusOK {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		if customErrorConverter != nil {
			return nil, providerUtils.EnrichError(ctx, customErrorConverter(resp), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
		}
		return nil, providerUtils.EnrichError(ctx, ParseOpenAIError(resp), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Large payload streaming passthrough — pipe raw upstream SSE to client
	if providerUtils.SetupStreamingPassthrough(ctx, resp) {
		responseChan := make(chan *schemas.RakshaStreamChunk)
		providerUtils.CloseStream(ctx, responseChan)
		return responseChan, nil
	}

	// Create response channel
	responseChan := make(chan *schemas.RakshaStreamChunk, schemas.DefaultStreamBufferSize)

	// Start streaming in a goroutine
	go func() {
		defer providerUtils.EnsureStreamFinalizerCalled(ctx, postHookSpanFinalizer)
		defer func() {
			if ctx.Err() == context.Canceled {
				providerUtils.HandleStreamCancellation(ctx, postHookRunner, responseChan, logger, postHookSpanFinalizer, jsonBody)
			} else if ctx.Err() == context.DeadlineExceeded {
				providerUtils.HandleStreamTimeout(ctx, postHookRunner, responseChan, logger, postHookSpanFinalizer, jsonBody)
			}
			// Release the responses stream state if it was acquired (for ResponsesToChatCompletions fallback)
			schemas.ReleaseChatToResponsesStreamState(responsesStreamState)
			providerUtils.CloseStream(ctx, responseChan)
		}()
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		// Decompress gzip-encoded streams transparently (no-op for non-gzip)
		reader, releaseGzip := providerUtils.DecompressStreamBody(resp)
		defer releaseGzip()

		// Wrap reader with idle timeout to detect stalled streams.
		reader, stopIdleTimeout := providerUtils.NewIdleTimeoutReader(reader, resp.BodyStream(), providerUtils.GetStreamIdleTimeout(ctx), ctx)
		defer stopIdleTimeout()

		// Setup cancellation handler to close the raw network stream on ctx cancellation,
		// which immediately unblocks any in-progress read (including reads blocked inside a gzip decompression layer).
		stopCancellation := providerUtils.SetupStreamCancellation(ctx, resp.BodyStream(), logger)
		defer stopCancellation()

		// Skip scanner for non-SSE responses — avoids bufio.Scanner buffer bloat
		// on non-line-delimited data (e.g. provider returned JSON instead of SSE).
		reader, drained := providerUtils.DrainNonSSEStreamReader(resp, reader)
		if drained {
			ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
			providerUtils.ProcessAndSendError(ctx, postHookRunner, errors.New("provider returned non-SSE response for streaming request"), responseChan, logger, postHookSpanFinalizer)
			return
		}

		sseReader := providerUtils.GetSSEDataReader(ctx, reader)

		chunkIndex := -1
		usage := &schemas.RakshaLLMUsage{}
		// Register the accumulating usage handle so a mid-stream
		// cancel/timeout can bill for tokens the provider already processed.
		ctx.SetValue(schemas.RakshaContextKeyStreamAccumulatedUsage, usage)

		lastChunkTime := startTime

		var finishReason *string
		var messageID string
		var modelName string
		var created int
		forwardedTerminalFinishReason := false
		// Defer final completed/incomplete event until usage chunk arrives (fallback path only).
		var pendingFinalEvent *schemas.RakshaResponsesStreamResponse
		usageSeen := false

		for {
			// If context was cancelled/timed out, let defer handle it
			if ctx.Err() != nil {
				return
			}
			data, readErr := sseReader.ReadDataLine()
			if readErr != nil {
				if ctx.Err() != nil {
					return
				}
				if readErr != io.EOF {
					ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
					logger.Warn("Error reading stream: %v", readErr)
					providerUtils.ProcessAndSendError(ctx, postHookRunner, readErr, responseChan, logger, postHookSpanFinalizer)
					return
				}
				break
			}
			jsonData := string(data)

			// Quick check for error field (allocation-free using sonic.GetFromString)
			if errorNode, _ := sonic.GetFromString(jsonData, "error"); errorNode.Exists() {
				// Only unmarshal when we know there's an error
				var rakshaErr schemas.RakshaError
				if err := sonic.UnmarshalString(jsonData, &rakshaErr); err == nil {
					if rakshaErr.Error != nil && rakshaErr.Error.Message != "" {
						ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
						providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, providerUtils.EnrichError(ctx, &rakshaErr, jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency), responseChan, logger, postHookSpanFinalizer)
						return
					}
				}
			}

			// Parse into raksha response
			var response schemas.RakshaChatResponse
			if customResponseHandler != nil {
				rawRequest, rawResponse, handlerErr := customResponseHandler([]byte(jsonData), &response, nil, sendBackRawRequest, sendBackRawResponse)
				if handlerErr != nil {
					if sendBackRawRequest {
						handlerErr.ExtraFields.RawRequest = rawRequest
					}
					if sendBackRawResponse {
						handlerErr.ExtraFields.RawResponse = rawResponse
					}
					ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
					providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, providerUtils.EnrichError(ctx, handlerErr, jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency), responseChan, logger, postHookSpanFinalizer)
					return
				}
			} else {
				if err := sonic.UnmarshalString(jsonData, &response); err != nil {
					logger.Warn("Failed to parse stream response: %v", err)
					continue
				}
			}

			// choices be array if nil
			if response.Choices == nil {
				response.Choices = []schemas.RakshaResponseChoice{}
			}

			if isResponsesToChatCompletionsFallback {
				// Accumulate usage across chunks; attached to final event below.
				if response.Usage != nil {
					usageSeen = true
					if response.Usage.PromptTokens > usage.PromptTokens {
						usage.PromptTokens = response.Usage.PromptTokens
					}
					if response.Usage.CompletionTokens > usage.CompletionTokens {
						usage.CompletionTokens = response.Usage.CompletionTokens
					}
					if response.Usage.TotalTokens > usage.TotalTokens {
						usage.TotalTokens = response.Usage.TotalTokens
					}
					if calculatedTotal := usage.PromptTokens + usage.CompletionTokens; calculatedTotal > usage.TotalTokens {
						usage.TotalTokens = calculatedTotal
					}
					if response.Usage.PromptTokensDetails != nil {
						usage.PromptTokensDetails = response.Usage.PromptTokensDetails
					}
					if response.Usage.CompletionTokensDetails != nil {
						usage.CompletionTokensDetails = response.Usage.CompletionTokensDetails
					}
					if response.Usage.Cost != nil {
						usage.Cost = response.Usage.Cost
					}
				}

				spreadResponses := response.ToRakshaResponsesStreamResponse(responsesStreamState)
				for _, response := range spreadResponses {
					if response.Type == schemas.ResponsesStreamResponseTypeError {
						rakshaErr := &schemas.RakshaError{
							Type:           schemas.Ptr(string(schemas.ResponsesStreamResponseTypeError)),
							IsRakshaError: false,
							Error:          &schemas.ErrorField{},
						}

						if response.Message != nil {
							rakshaErr.Error.Message = *response.Message
						}
						if response.Param != nil {
							rakshaErr.Error.Param = *response.Param
						}
						if response.Code != nil {
							rakshaErr.Error.Code = response.Code
						}

						ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
						providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, providerUtils.EnrichError(ctx, rakshaErr, jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency), responseChan, logger, postHookSpanFinalizer)
						return
					}

					response.ExtraFields.ChunkIndex = response.SequenceNumber

					if sendBackRawResponse {
						response.ExtraFields.RawResponse = jsonData
					}

					if response.Type == schemas.ResponsesStreamResponseTypeCompleted || response.Type == schemas.ResponsesStreamResponseTypeIncomplete {
						// Defer sending until stream end so usage can be attached.
						pendingFinalEvent = response
						continue
					}

					response.ExtraFields.Latency = time.Since(lastChunkTime).Milliseconds()
					lastChunkTime = time.Now()

					providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(nil, nil, response, nil, nil, nil), responseChan, postHookSpanFinalizer)
				}
			} else {
				if postResponseConverter != nil {
					if converted := postResponseConverter(&response); converted != nil {
						response = *converted
					} else {
						logger.Warn("postResponseConverter returned nil; leaving chunk unmodified")
					}
				}

				// Handle usage-only chunks (when stream_options include_usage is true)
				if response.Usage != nil {
					// Collect usage information and send at the end of the stream
					// Here in some cases usage comes before final message
					// So we need to check if the response.Usage is nil and then if usage != nil
					// then add up all tokens
					if response.Usage.PromptTokens > usage.PromptTokens {
						usage.PromptTokens = response.Usage.PromptTokens
					}
					if response.Usage.CompletionTokens > usage.CompletionTokens {
						usage.CompletionTokens = response.Usage.CompletionTokens
					}
					if response.Usage.TotalTokens > usage.TotalTokens {
						usage.TotalTokens = response.Usage.TotalTokens
					}
					calculatedTotal := usage.PromptTokens + usage.CompletionTokens
					if calculatedTotal > usage.TotalTokens {
						usage.TotalTokens = calculatedTotal
					}
					if response.Usage.PromptTokensDetails != nil {
						usage.PromptTokensDetails = response.Usage.PromptTokensDetails
					}
					if response.Usage.CompletionTokensDetails != nil {
						usage.CompletionTokensDetails = response.Usage.CompletionTokensDetails
					}
					if response.Usage.Cost != nil {
						usage.Cost = response.Usage.Cost
					}
					response.Usage = nil
				}

				if response.Model != "" {
					modelName = response.Model
				}

				// Skip empty responses or responses without choices
				if len(response.Choices) == 0 {
					continue
				}

				// Handle finish reason, usually in the final chunk
				choice := response.Choices[0]
				if choice.FinishReason != nil && *choice.FinishReason != "" {
					// Collect finish reason and send at the end of the stream
					finishReason = choice.FinishReason
				}

				if response.ID != "" && messageID == "" {
					messageID = response.ID
				}
				if response.Created != 0 && created == 0 {
					created = response.Created
				}

				// Handle regular content chunks, including reasoning
				if choice.ChatStreamResponseChoice != nil &&
					choice.ChatStreamResponseChoice.Delta != nil &&
					((choice.ChatStreamResponseChoice.Delta.Content != nil && *choice.ChatStreamResponseChoice.Delta.Content != "") ||
						choice.ChatStreamResponseChoice.Delta.Reasoning != nil ||
						len(choice.ChatStreamResponseChoice.Delta.ReasoningDetails) > 0 ||
						choice.ChatStreamResponseChoice.Delta.Audio != nil ||
						len(choice.ChatStreamResponseChoice.Delta.ToolCalls) > 0) {
					if choice.FinishReason != nil && *choice.FinishReason != "" {
						forwardedTerminalFinishReason = true
					}
					chunkIndex++

					response.ExtraFields.ChunkIndex = chunkIndex
					response.ExtraFields.Latency = time.Since(lastChunkTime).Milliseconds()
					lastChunkTime = time.Now()

					if sendBackRawResponse {
						response.ExtraFields.RawResponse = jsonData
					}

					providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(nil, &response, nil, nil, nil, nil), responseChan, postHookSpanFinalizer)
				}

				// For providers that don't send [DONE] marker break on finish_reason
				if !providerUtils.ProviderSendsDoneMarker(providerName) && finishReason != nil {
					break
				}
			}
		}

		if isResponsesToChatCompletionsFallback {
			if pendingFinalEvent != nil {
				if usageSeen && pendingFinalEvent.Response != nil {
					pendingFinalEvent.Response.Usage = usage.ToResponsesResponseUsage()
				}
				if sendBackRawRequest {
					providerUtils.ParseAndSetRawRequest(&pendingFinalEvent.ExtraFields, jsonBody)
				}
				pendingFinalEvent.ExtraFields.Latency = time.Since(startTime).Milliseconds()
				ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
				providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(nil, nil, pendingFinalEvent, nil, nil, nil), responseChan, postHookSpanFinalizer)
			}
		} else {
			finalFinishReason := finishReason
			if forwardedTerminalFinishReason {
				finalFinishReason = nil
			}
			response := providerUtils.CreateRakshaChatCompletionChunkResponse(messageID, usage, finalFinishReason, chunkIndex, modelName, created)
			if postResponseConverter != nil {
				response = postResponseConverter(response)
			}
			// Set raw request if enabled
			if sendBackRawRequest {
				providerUtils.ParseAndSetRawRequest(&response.ExtraFields, jsonBody)
			}
			response.ExtraFields.Latency = time.Since(startTime).Milliseconds()
			ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
			providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(nil, response, nil, nil, nil, nil), responseChan, postHookSpanFinalizer)
		}
	}()

	return responseChan, nil
}

// Responses performs a responses request to the OpenAI API.
func (provider *OpenAIProvider) Responses(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaResponsesRequest) (*schemas.RakshaResponsesResponse, *schemas.RakshaError) {
	if provider.shouldFallbackResponsesToChat(schemas.ResponsesRequest, schemas.ChatCompletionRequest) {
		chatResponse, err := provider.ChatCompletion(ctx, key, request.ToChatRequest())
		if err != nil {
			return nil, err
		}
		return chatResponse.ToRakshaResponsesResponse(), nil
	}

	// Check if chat completion is allowed for this provider
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.ResponsesRequest); err != nil {
		return nil, err
	}

	if provider.disableStore {
		if request.Params == nil {
			request.Params = &schemas.ResponsesParameters{}
		}
		request.Params.Store = schemas.Ptr(false)
	}

	return HandleOpenAIResponsesRequest(
		ctx,
		provider.client,
		provider.buildRequestURL(ctx, "/v1/responses", schemas.ResponsesRequest),
		request,
		BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		nil,
		nil,
		nil,
		provider.logger,
	)
}

// HandleOpenAIResponsesRequest handles a responses request to OpenAI's API.
func HandleOpenAIResponsesRequest(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	url string,
	request *schemas.RakshaResponsesRequest,
	authHeader map[string]string,
	extraHeaders map[string]string,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
	providerName schemas.ModelProvider,
	customResponseHandler responseHandler[schemas.RakshaResponsesResponse],
	customErrorConverter ErrorConverter,
	signer providerUtils.BodySigner,
	logger schemas.Logger,
) (*schemas.RakshaResponsesResponse, *schemas.RakshaError) {
	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	// resp lifecycle: managed by finalizeOpenAIResponse or released on error paths
	respOwned := true
	defer func() {
		if respOwned {
			fasthttp.ReleaseResponse(resp)
		}
	}()
	activeClient := providerUtils.PrepareResponseStreaming(ctx, client, resp)

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)

	req.SetRequestURI(url)
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")

	for k, v := range authHeader {
		req.Header.Set(k, v)
	}

	// Large payload passthrough: stream body directly without JSON marshaling
	if lpResult, lpErr, handled := handleOpenAILargePayloadPassthrough(ctx, client, url, authHeader, extraHeaders, providerName, logger); handled {
		if lpErr != nil {
			return nil, lpErr
		}
		if len(lpResult.ResponseBody) > 0 {
			response := &schemas.RakshaResponsesResponse{}
			if err := sonic.Unmarshal(lpResult.ResponseBody, response); err != nil {
				return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseUnmarshal, err)
			}
			response.ExtraFields = schemas.RakshaResponseExtraFields{Latency: lpResult.Latency}
			return response, nil
		}
		return &schemas.RakshaResponsesResponse{
			Model:       request.Model,
			ExtraFields: schemas.RakshaResponseExtraFields{Latency: lpResult.Latency},
		}, nil
	}

	// Use centralized converter
	jsonData, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToOpenAIResponsesRequest(ctx, request), nil
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	if signer != nil {
		sigHeaders, signErr := signer(jsonData)
		if signErr != nil {
			return nil, signErr
		}
		for k, v := range sigHeaders {
			req.Header.Set(k, v)
		}
	}

	req.SetBody(jsonData)

	// Make request
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}
	// Extract provider response headers early so they're available on error paths too
	providerResponseHeaders := providerUtils.ExtractProviderResponseHeaders(resp)
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		logger.Debug("error from %s provider: %s", providerName, string(resp.Body()))
		if customErrorConverter != nil {
			return nil, providerUtils.EnrichError(ctx, customErrorConverter(resp), jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
		}
		return nil, providerUtils.EnrichError(ctx, ParseOpenAIError(resp), jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	body, lpResult, finalErr := finalizeOpenAIResponse(ctx, resp, latency, providerName, logger)
	respOwned = false // ownership transferred
	if finalErr != nil {
		return nil, providerUtils.EnrichError(ctx, finalErr, jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}
	if lpResult != nil {
		return &schemas.RakshaResponsesResponse{
			Model:       request.Model,
			ExtraFields: schemas.RakshaResponseExtraFields{Latency: lpResult.Latency},
		}, nil
	}

	response := &schemas.RakshaResponsesResponse{}

	var rawRequest, rawResponse interface{}

	if customResponseHandler != nil {
		rawRequest, rawResponse, rakshaErr = customResponseHandler(body, response, jsonData, sendBackRawRequest, sendBackRawResponse)
	} else {
		rawRequest, rawResponse, rakshaErr = providerUtils.HandleProviderResponse(body, response, jsonData, sendBackRawRequest, sendBackRawResponse)
	}

	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, body, sendBackRawRequest, sendBackRawResponse, latency)
	}

	response.ExtraFields.Latency = latency.Milliseconds()
	response.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	// Set raw request if enabled
	if sendBackRawRequest {
		response.ExtraFields.RawRequest = rawRequest
	}

	// Set raw response if enabled
	if sendBackRawResponse {
		response.ExtraFields.RawResponse = rawResponse
	}

	return response, nil
}

// ResponsesStream performs a streaming responses request to the OpenAI API.
func (provider *OpenAIProvider) ResponsesStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaResponsesRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	if provider.shouldFallbackResponsesToChat(schemas.ResponsesStreamRequest, schemas.ChatCompletionStreamRequest) {
		ctx.SetValue(schemas.RakshaContextKeyIsResponsesToChatCompletionFallback, true)
		return provider.ChatCompletionStream(ctx, postHookRunner, postHookSpanFinalizer, key, request.ToChatRequest())
	}

	// Check if chat completion stream is allowed for this provider
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.ResponsesStreamRequest); err != nil {
		return nil, err
	}
	if provider.disableStore {
		if request.Params == nil {
			request.Params = &schemas.ResponsesParameters{}
		}
		request.Params.Store = schemas.Ptr(false)
	}

	// Use shared streaming logic
	return HandleOpenAIResponsesStreaming(
		ctx,
		provider.streamingClient,
		provider.buildRequestURL(ctx, "/v1/responses", schemas.ResponsesStreamRequest),
		request,
		BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		provider.networkConfig.StreamIdleTimeoutInSeconds,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		postHookRunner,
		nil,
		nil,
		nil,
		nil,
		nil,
		provider.logger,
		postHookSpanFinalizer,
	)
}

// HandleOpenAIResponsesStreaming handles streaming for OpenAI-compatible APIs.
// This shared function reduces code duplication between providers that use the same SSE format.
func HandleOpenAIResponsesStreaming(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	url string,
	request *schemas.RakshaResponsesRequest,
	authHeader map[string]string,
	extraHeaders map[string]string,
	streamIdleTimeoutInSeconds int,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
	providerName schemas.ModelProvider,
	postHookRunner schemas.PostHookRunner,
	customResponseHandler responseHandler[schemas.RakshaResponsesStreamResponse],
	customErrorConverter ErrorConverter,
	postRequestConverter func(*OpenAIResponsesRequest) *OpenAIResponsesRequest,
	postResponseConverter func(*schemas.RakshaResponsesStreamResponse) *schemas.RakshaResponsesStreamResponse,
	signer providerUtils.BodySigner,
	logger schemas.Logger,
	postHookSpanFinalizer func(context.Context),
) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	providerUtils.SetStreamIdleTimeoutIfEmpty(ctx, streamIdleTimeoutInSeconds)
	// Prepare SGL headers (SGL typically doesn't require authorization, but we include it if provided)
	headers := map[string]string{
		"Content-Type":  "application/json",
		"Accept":        "text/event-stream",
		"Cache-Control": "no-cache",
	}

	if authHeader != nil {
		// Copy auth header to headers
		maps.Copy(headers, authHeader)
	}

	jsonBody, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			reqBody := ToOpenAIResponsesRequest(ctx, request)
			if reqBody != nil {
				reqBody.Stream = schemas.Ptr(true)
				if postRequestConverter != nil {
					reqBody = postRequestConverter(reqBody)
				}
			}
			return reqBody, nil
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Create HTTP request for streaming
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	resp.StreamBody = true
	defer fasthttp.ReleaseRequest(req)

	req.Header.SetMethod(http.MethodPost)
	req.SetRequestURI(url)
	req.Header.SetContentType("application/json")

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)

	// Set headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	if signer != nil {
		sigHeaders, signErr := signer(jsonBody)
		if signErr != nil {
			defer providerUtils.ReleaseStreamingResponse(ctx, resp)
			return nil, signErr
		}
		for k, v := range sigHeaders {
			req.Header.Set(k, v)
		}
	}

	setStreamingRequestBody(ctx, req, jsonBody, providerName)

	// Use streaming-aware client when large payload optimization is active — ensures
	// MaxResponseBodySize > 0 so ErrBodyTooLarge triggers StreamBody for Content-Length responses.
	activeClient := providerUtils.PrepareResponseStreaming(ctx, client, resp)

	startTime := time.Now()
	// Make the request
	err := activeClient.Do(req, resp)
	latency := time.Since(startTime)
	if err != nil {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		if errors.Is(err, context.Canceled) {
			return nil, providerUtils.EnrichError(ctx, &schemas.RakshaError{
				IsRakshaError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   err,
				},
			}, jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
		}
		if errors.Is(err, fasthttp.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
			return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaTimeoutError(schemas.ErrProviderRequestTimedOut, err), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
		}
		// The request failed before the first response byte (connection refused, server
		// closed an idle/pooled connection, broken pipe, DNS failure, etc.). Mirror the
		// non-streaming path (makeRequestWithDoFunc) and surface this as a retriable upstream
		// connection error (502, IsRakshaError=false) rather than NewRakshaOperationError
		// (500, IsRakshaError=true). The latter caused the retry loop in executeRequestWithRetries
		// to break early on IsRakshaError, so max_retries never applied to streaming connection
		// failures - see https://github.com/raksha/raksha/issues/4496.
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaUpstreamConnectionError(schemas.ErrProviderDoRequest, err), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Store provider response headers in context before status check so error responses also forward them
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	// Check for HTTP errors
	if resp.StatusCode() != fasthttp.StatusOK {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		if customErrorConverter != nil {
			return nil, providerUtils.EnrichError(ctx, customErrorConverter(resp), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
		}
		return nil, providerUtils.EnrichError(ctx, ParseOpenAIError(resp), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Large payload streaming passthrough — pipe raw upstream SSE to client
	if providerUtils.SetupStreamingPassthrough(ctx, resp) {
		responseChan := make(chan *schemas.RakshaStreamChunk)
		providerUtils.CloseStream(ctx, responseChan)
		return responseChan, nil
	}

	// Create response channel
	responseChan := make(chan *schemas.RakshaStreamChunk, schemas.DefaultStreamBufferSize)

	// Start streaming in a goroutine
	go func() {
		defer providerUtils.EnsureStreamFinalizerCalled(ctx, postHookSpanFinalizer)
		defer func() {
			if ctx.Err() == context.Canceled {
				providerUtils.HandleStreamCancellation(ctx, postHookRunner, responseChan, logger, postHookSpanFinalizer, jsonBody)
			} else if ctx.Err() == context.DeadlineExceeded {
				providerUtils.HandleStreamTimeout(ctx, postHookRunner, responseChan, logger, postHookSpanFinalizer, jsonBody)
			}
			providerUtils.CloseStream(ctx, responseChan)
		}()
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		// Decompress gzip-encoded streams transparently (no-op for non-gzip)
		reader, releaseGzip := providerUtils.DecompressStreamBody(resp)
		defer releaseGzip()

		// Wrap reader with idle timeout to detect stalled streams.
		reader, stopIdleTimeout := providerUtils.NewIdleTimeoutReader(reader, resp.BodyStream(), providerUtils.GetStreamIdleTimeout(ctx), ctx)
		defer stopIdleTimeout()

		// Setup cancellation handler to close the raw network stream on ctx cancellation,
		// which immediately unblocks any in-progress read (including reads blocked inside a gzip decompression layer).
		stopCancellation := providerUtils.SetupStreamCancellation(ctx, resp.BodyStream(), logger)
		defer stopCancellation()

		// Skip scanner for non-SSE responses — avoids bufio.Scanner buffer bloat
		// on non-line-delimited data (e.g. provider returned JSON instead of SSE).
		reader, drained := providerUtils.DrainNonSSEStreamReader(resp, reader)
		if drained {
			ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
			providerUtils.ProcessAndSendError(ctx, postHookRunner, errors.New("provider returned non-SSE response for streaming request"), responseChan, logger, postHookSpanFinalizer)
			return
		}

		sseReader := providerUtils.GetSSEDataReader(ctx, reader)

		lastChunkTime := startTime

		for {
			// If context was cancelled/timed out, let defer handle it
			if ctx.Err() != nil {
				return
			}
			data, readErr := sseReader.ReadDataLine()
			if readErr != nil {
				if ctx.Err() != nil {
					return
				}
				if readErr != io.EOF {
					ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
					logger.Warn("Error reading stream: %v", readErr)
					providerUtils.ProcessAndSendError(ctx, postHookRunner, readErr, responseChan, logger, postHookSpanFinalizer)
				}
				break
			}
			jsonData := string(data)

			// Parse into raksha response
			var response schemas.RakshaResponsesStreamResponse
			// TODO fix this
			if customResponseHandler != nil {
				rawRequest, rawResponse, rakshaErr := customResponseHandler([]byte(jsonData), &response, nil, false, false)
				if rakshaErr != nil {
					if sendBackRawRequest {
						rakshaErr.ExtraFields.RawRequest = rawRequest
					}
					if sendBackRawResponse {
						rakshaErr.ExtraFields.RawResponse = rawResponse
					}
					ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
					providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, providerUtils.EnrichError(ctx, rakshaErr, jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency), responseChan, logger, postHookSpanFinalizer)
					return
				}
			} else {
				if err := sonic.UnmarshalString(jsonData, &response); err != nil {
					logger.Warn("Failed to parse stream response: %v", err)
					continue
				}

				if postResponseConverter != nil {
					if converted := postResponseConverter(&response); converted != nil {
						response = *converted
					} else {
						logger.Warn("postResponseConverter returned nil; leaving chunk unmodified")
					}
				}

				if sendBackRawResponse {
					response.ExtraFields.RawResponse = jsonData
				}

				if response.Type == schemas.ResponsesStreamResponseTypeError {
					rakshaErr := &schemas.RakshaError{
						Type:           schemas.Ptr(string(schemas.ResponsesStreamResponseTypeError)),
						IsRakshaError: false,
						Error:          &schemas.ErrorField{},
					}

					if response.Message != nil {
						rakshaErr.Error.Message = *response.Message
					}
					if response.Param != nil {
						rakshaErr.Error.Param = *response.Param
					}
					if response.Code != nil {
						rakshaErr.Error.Code = response.Code
					}
					if response.Error != nil {
						if response.Error.Message != "" && rakshaErr.Error.Message == "" {
							rakshaErr.Error.Message = response.Error.Message
						}
						if response.Error.Code != "" && (rakshaErr.Error.Code == nil || *rakshaErr.Error.Code == "") {
							rakshaErr.Error.Code = &response.Error.Code
						}
					}
					if response.Response != nil && response.Response.Error != nil {
						if response.Response.Error.Message != "" && rakshaErr.Error.Message == "" {
							rakshaErr.Error.Message = response.Response.Error.Message
						}
						if response.Response.Error.Code != "" && (rakshaErr.Error.Code == nil || *rakshaErr.Error.Code == "") {
							rakshaErr.Error.Code = schemas.Ptr(response.Response.Error.Code)
						}
					}

					ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
					providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, providerUtils.EnrichError(ctx, rakshaErr, jsonBody, []byte(jsonData), sendBackRawRequest, sendBackRawResponse, latency), responseChan, logger, postHookSpanFinalizer)
					return
				}

				// Some providers (e.g. Fireworks) send response.failed on HTTP 200 streams
				// instead of a pre-stream 4xx. Convert to RakshaError for consistent handling.
				if response.Type == schemas.ResponsesStreamResponseTypeFailed {
					rakshaErr := &schemas.RakshaError{
						Type:           schemas.Ptr(string(schemas.ResponsesStreamResponseTypeFailed)),
						IsRakshaError: false,
						Error:          &schemas.ErrorField{},
					}
					if response.Response != nil && response.Response.Error != nil {
						rakshaErr.Error.Message = response.Response.Error.Message
						rakshaErr.Error.Code = &response.Response.Error.Code
					}
					ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
					providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, providerUtils.EnrichError(ctx, rakshaErr, jsonBody, []byte(jsonData), sendBackRawRequest, sendBackRawResponse, latency), responseChan, logger, postHookSpanFinalizer)
					return
				}

				response.ExtraFields.ChunkIndex = response.SequenceNumber
				if response.Type == schemas.ResponsesStreamResponseTypeCompleted || response.Type == schemas.ResponsesStreamResponseTypeIncomplete {
					// Set raw request if enabled
					if sendBackRawRequest {
						providerUtils.ParseAndSetRawRequest(&response.ExtraFields, jsonBody)
					}
					response.ExtraFields.Latency = time.Since(startTime).Milliseconds()
					ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
					providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(nil, nil, &response, nil, nil, nil), responseChan, postHookSpanFinalizer)
					return
				}

				response.ExtraFields.Latency = time.Since(lastChunkTime).Milliseconds()
				lastChunkTime = time.Now()

				providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(nil, nil, &response, nil, nil, nil), responseChan, postHookSpanFinalizer)
			}
		}
	}()

	return responseChan, nil
}

// Embedding generates embeddings for the given input text(s).
// The input can be either a single string or a slice of strings for batch embedding.
// Returns a RakshaResponse containing the embedding(s) and any error that occurred.
func (provider *OpenAIProvider) Embedding(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaEmbeddingRequest) (*schemas.RakshaEmbeddingResponse, *schemas.RakshaError) {
	// Check if embedding is allowed for this provider
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.EmbeddingRequest); err != nil {
		return nil, err
	}

	// Use the shared embedding request handler
	return HandleOpenAIEmbeddingRequest(
		ctx,
		provider.client,
		provider.buildRequestURL(ctx, "/v1/embeddings", schemas.EmbeddingRequest),
		request,
		BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		provider.GetProviderKey(),
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		nil,
		provider.logger,
	)
}

// HandleOpenAIEmbeddingRequest handles embedding requests for OpenAI-compatible APIs.
// This shared function reduces code duplication between providers that use the same embedding request format.
func HandleOpenAIEmbeddingRequest(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	url string,
	request *schemas.RakshaEmbeddingRequest,
	authHeader map[string]string,
	extraHeaders map[string]string,
	providerName schemas.ModelProvider,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
	customResponseHandler responseHandler[schemas.RakshaEmbeddingResponse],
	logger schemas.Logger,
) (*schemas.RakshaEmbeddingResponse, *schemas.RakshaError) {
	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	// resp lifecycle: managed by finalizeOpenAIResponse or released on error paths
	respOwned := true
	defer func() {
		if respOwned {
			fasthttp.ReleaseResponse(resp)
		}
	}()
	activeClient := providerUtils.PrepareResponseStreaming(ctx, client, resp)

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)

	req.SetRequestURI(url)
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")

	for k, v := range authHeader {
		req.Header.Set(k, v)
	}

	// Large payload passthrough: stream body directly without JSON marshaling
	if lpResult, lpErr, handled := handleOpenAILargePayloadPassthrough(ctx, client, url, authHeader, extraHeaders, providerName, logger); handled {
		if lpErr != nil {
			return nil, lpErr
		}
		if len(lpResult.ResponseBody) > 0 {
			response := &schemas.RakshaEmbeddingResponse{}
			if err := sonic.Unmarshal(lpResult.ResponseBody, response); err != nil {
				return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseUnmarshal, err)
			}
			response.ExtraFields = schemas.RakshaResponseExtraFields{Latency: lpResult.Latency}
			return response, nil
		}
		return &schemas.RakshaEmbeddingResponse{
			Model:       request.Model,
			Usage:       lpResult.Usage,
			ExtraFields: schemas.RakshaResponseExtraFields{Latency: lpResult.Latency},
		}, nil
	}

	// Use centralized converter
	jsonData, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToOpenAIEmbeddingRequest(request), nil
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	req.SetBody(jsonData)

	// Make request
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}
	// Extract provider response headers early so they're available on error paths too
	providerResponseHeaders := providerUtils.ExtractProviderResponseHeaders(resp)
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		logger.Debug(fmt.Sprintf("error from %s provider: %s", providerName, string(resp.Body())))
		return nil, providerUtils.EnrichError(ctx, ParseOpenAIError(resp), jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	body, lpResult, finalErr := finalizeOpenAIResponse(ctx, resp, latency, providerName, logger)
	respOwned = false // ownership transferred
	if finalErr != nil {
		return nil, providerUtils.EnrichError(ctx, finalErr, jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}
	if lpResult != nil {
		return &schemas.RakshaEmbeddingResponse{
			Model:       request.Model,
			Usage:       lpResult.Usage,
			ExtraFields: schemas.RakshaResponseExtraFields{Latency: lpResult.Latency},
		}, nil
	}

	response := &schemas.RakshaEmbeddingResponse{}

	var rawRequest, rawResponse interface{}

	if customResponseHandler != nil {
		rawRequest, rawResponse, rakshaErr = customResponseHandler(body, response, jsonData, sendBackRawRequest, sendBackRawResponse)
	} else {
		rawRequest, rawResponse, rakshaErr = providerUtils.HandleProviderResponse(body, response, jsonData, sendBackRawRequest, sendBackRawResponse)
	}

	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, body, sendBackRawRequest, sendBackRawResponse, latency)
	}

	response.ExtraFields.Latency = latency.Milliseconds()
	response.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	// Set raw request if enabled
	if sendBackRawRequest {
		response.ExtraFields.RawRequest = rawRequest
	}

	// Set raw response if enabled
	if sendBackRawResponse {
		response.ExtraFields.RawResponse = rawResponse
	}

	return response, nil
}

// shouldFallbackResponsesToChat reports whether a Responses call should be
// transparently translated into Chat Completions. This applies when a custom
// provider disables the Responses operation but still allows Chat Completions.
func (provider *OpenAIProvider) shouldFallbackResponsesToChat(responsesOp, chatOp schemas.RequestType) bool {
	cfg := provider.customProviderConfig
	if cfg == nil || cfg.AllowedRequests == nil {
		return false
	}
	return !cfg.IsOperationAllowed(responsesOp) && cfg.IsOperationAllowed(chatOp)
}

// Speech handles non-streaming speech synthesis requests.
// It formats the request body, makes the API call, and returns the response.
// Returns the response and any error that occurred.
func (provider *OpenAIProvider) Speech(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaSpeechRequest) (*schemas.RakshaSpeechResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.SpeechRequest); err != nil {
		return nil, err
	}

	return HandleOpenAISpeechRequest(
		ctx,
		provider.client,
		provider.buildRequestURL(ctx, "/v1/audio/speech", schemas.SpeechRequest),
		request,
		key,
		provider.networkConfig.ExtraHeaders,
		provider.GetProviderKey(),
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		nil,
		provider.logger,
	)
}

// HandleOpenAISpeechRequest handles speech requests for OpenAI-compatible APIs.
// This shared function reduces code duplication between providers that use the same speech request format.
func HandleOpenAISpeechRequest(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	url string,
	request *schemas.RakshaSpeechRequest,
	key schemas.Key,
	extraHeaders map[string]string,
	providerName schemas.ModelProvider,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
	customResponseHandler responseHandler[schemas.RakshaSpeechResponse],
	logger schemas.Logger,
) (*schemas.RakshaSpeechResponse, *schemas.RakshaError) {
	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	// resp lifecycle: managed by finalizeOpenAIResponse or released on error paths
	respOwned := true
	defer func() {
		if respOwned {
			fasthttp.ReleaseResponse(resp)
		}
	}()
	activeClient := providerUtils.PrepareResponseStreaming(ctx, client, resp)

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)

	req.SetRequestURI(url)
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	// Large payload passthrough: stream body directly without JSON marshaling
	if lpResult, lpErr, handled := handleOpenAILargePayloadPassthrough(ctx, client, url, BearerAuthHeader(key), extraHeaders, providerName, logger); handled {
		if lpErr != nil {
			return nil, lpErr
		}
		// Speech response is raw audio bytes (MP3/WAV), not JSON
		return &schemas.RakshaSpeechResponse{
			Audio:       lpResult.ResponseBody,
			ExtraFields: schemas.RakshaResponseExtraFields{Latency: lpResult.Latency},
		}, nil
	}

	jsonData, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) { return ToOpenAISpeechRequest(request), nil })
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	req.SetBody(jsonData)

	// Make request
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}
	// Extract provider response headers early so they're available on error paths too
	providerResponseHeaders := providerUtils.ExtractProviderResponseHeaders(resp)
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		logger.Debug(fmt.Sprintf("error from %s provider: %s", providerName, string(resp.Body())))
		return nil, providerUtils.EnrichError(ctx, ParseOpenAIError(resp), jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Get the binary audio data from the response body
	body, lpResult, finalErr := finalizeOpenAIResponse(ctx, resp, latency, providerName, logger)
	respOwned = false // ownership transferred
	if finalErr != nil {
		return nil, providerUtils.EnrichError(ctx, finalErr, jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}
	if lpResult != nil {
		return &schemas.RakshaSpeechResponse{
			ExtraFields: schemas.RakshaResponseExtraFields{Latency: lpResult.Latency},
		}, nil
	}

	// Create final response with the audio data
	// Note: For speech synthesis, we return the binary audio data in the raw response
	// The audio data is typically in MP3, WAV, or other audio formats as specified by response_format
	rakshaResponse := &schemas.RakshaSpeechResponse{
		Audio: body,
		ExtraFields: schemas.RakshaResponseExtraFields{
			Latency:                 latency.Milliseconds(),
			ProviderResponseHeaders: providerResponseHeaders,
		},
	}

	if sendBackRawRequest {
		providerUtils.ParseAndSetRawRequest(&rakshaResponse.ExtraFields, jsonData)
	}

	return rakshaResponse, nil
}

// SpeechStream handles streaming for speech synthesis.
// It formats the request body, creates HTTP request, and uses shared streaming logic.
// Returns a channel for streaming responses and any error that occurred.
func (provider *OpenAIProvider) SpeechStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaSpeechRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.SpeechStreamRequest); err != nil {
		return nil, err
	}

	for _, model := range providerUtils.UnsupportedSpeechStreamModels {
		if model == request.Model {
			return nil, providerUtils.NewRakshaOperationError(fmt.Sprintf("model %s is not supported for streaming speech synthesis", model), nil)
		}
	}

	return HandleOpenAISpeechStreamRequest(
		ctx,
		provider.streamingClient,
		provider.buildRequestURL(ctx, "/v1/audio/speech", schemas.SpeechStreamRequest),
		request,
		BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		provider.networkConfig.StreamIdleTimeoutInSeconds,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		postHookRunner,
		nil,
		nil,
		provider.logger,
		postHookSpanFinalizer,
	)
}

// HandleOpenAISpeechStreamRequest handles speech stream requests for OpenAI-compatible APIs.
// This shared function reduces code duplication between providers that use the same speech stream request format.
func HandleOpenAISpeechStreamRequest(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	url string,
	request *schemas.RakshaSpeechRequest,
	authHeader map[string]string,
	extraHeaders map[string]string,
	streamIdleTimeoutInSeconds int,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
	providerName schemas.ModelProvider,
	postHookRunner schemas.PostHookRunner,
	postRequestConverter func(*OpenAISpeechRequest) *OpenAISpeechRequest,
	postResponseConverter func(*schemas.RakshaSpeechStreamResponse) *schemas.RakshaSpeechStreamResponse,
	logger schemas.Logger,
	postHookSpanFinalizer func(context.Context),
) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	providerUtils.SetStreamIdleTimeoutIfEmpty(ctx, streamIdleTimeoutInSeconds)
	// Create HTTP request for streaming
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	resp.StreamBody = true
	defer fasthttp.ReleaseRequest(req)

	// Prepare OpenAI headers
	headers := map[string]string{
		"Content-Type":  "application/json",
		"Accept":        "text/event-stream",
		"Cache-Control": "no-cache",
	}

	if authHeader != nil {
		maps.Copy(headers, authHeader)
	}

	req.Header.SetMethod(http.MethodPost)
	req.SetRequestURI(url)
	req.Header.SetContentType("application/json")

	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)

	// Set any extra headers from network config
	// Set headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Use centralized converter
	jsonBody, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			reqBody := ToOpenAISpeechRequest(request)
			if reqBody != nil {
				reqBody.StreamFormat = schemas.Ptr("sse")
				if postRequestConverter != nil {
					reqBody = postRequestConverter(reqBody)
				}
			}
			return reqBody, nil
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	setStreamingRequestBody(ctx, req, jsonBody, providerName)

	// Use streaming-aware client when large payload optimization is active — ensures
	// MaxResponseBodySize > 0 so ErrBodyTooLarge triggers StreamBody for Content-Length responses.
	activeClient := providerUtils.PrepareResponseStreaming(ctx, client, resp)

	startTime := time.Now()
	// Make the request
	err := activeClient.Do(req, resp)
	latency := time.Since(startTime)
	if err != nil {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		if errors.Is(err, context.Canceled) {
			return nil, providerUtils.EnrichError(ctx, &schemas.RakshaError{
				IsRakshaError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   err,
				},
			}, jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
		}
		if errors.Is(err, fasthttp.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
			return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaTimeoutError(schemas.ErrProviderRequestTimedOut, err), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
		}
		// The request failed before the first response byte (connection refused, server
		// closed an idle/pooled connection, broken pipe, DNS failure, etc.). Mirror the
		// non-streaming path (makeRequestWithDoFunc) and surface this as a retriable upstream
		// connection error (502, IsRakshaError=false) rather than NewRakshaOperationError
		// (500, IsRakshaError=true). The latter caused the retry loop in executeRequestWithRetries
		// to break early on IsRakshaError, so max_retries never applied to streaming connection
		// failures - see https://github.com/raksha/raksha/issues/4496.
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaUpstreamConnectionError(schemas.ErrProviderDoRequest, err), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Store provider response headers in context before status check so error responses also forward them
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	// Check for HTTP errors
	if resp.StatusCode() != fasthttp.StatusOK {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		return nil, providerUtils.EnrichError(ctx, ParseOpenAIError(resp), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Large payload streaming passthrough — pipe raw upstream SSE to client
	if providerUtils.SetupStreamingPassthrough(ctx, resp) {
		responseChan := make(chan *schemas.RakshaStreamChunk)
		providerUtils.CloseStream(ctx, responseChan)
		return responseChan, nil
	}

	// Create response channel
	responseChan := make(chan *schemas.RakshaStreamChunk, schemas.DefaultStreamBufferSize)

	// Start streaming in a goroutine
	go func() {
		defer providerUtils.EnsureStreamFinalizerCalled(ctx, postHookSpanFinalizer)
		defer func() {
			if ctx.Err() == context.Canceled {
				providerUtils.HandleStreamCancellation(ctx, postHookRunner, responseChan, logger, postHookSpanFinalizer, jsonBody)
			} else if ctx.Err() == context.DeadlineExceeded {
				providerUtils.HandleStreamTimeout(ctx, postHookRunner, responseChan, logger, postHookSpanFinalizer, jsonBody)
			}
			providerUtils.CloseStream(ctx, responseChan)
		}()
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		// Decompress gzip-encoded streams transparently (no-op for non-gzip)
		reader, releaseGzip := providerUtils.DecompressStreamBody(resp)
		defer releaseGzip()

		// Wrap reader with idle timeout to detect stalled streams.
		reader, stopIdleTimeout := providerUtils.NewIdleTimeoutReader(reader, resp.BodyStream(), providerUtils.GetStreamIdleTimeout(ctx), ctx)
		defer stopIdleTimeout()

		// Setup cancellation handler to close the raw network stream on ctx cancellation,
		// which immediately unblocks any in-progress read (including reads blocked inside a gzip decompression layer).
		stopCancellation := providerUtils.SetupStreamCancellation(ctx, resp.BodyStream(), logger)
		defer stopCancellation()

		// Skip scanner for non-SSE responses — avoids bufio.Scanner buffer bloat
		// on non-line-delimited data (e.g. provider returned JSON instead of SSE).
		reader, drained := providerUtils.DrainNonSSEStreamReader(resp, reader)
		if drained {
			ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
			providerUtils.ProcessAndSendError(ctx, postHookRunner, errors.New("provider returned non-SSE response for streaming request"), responseChan, logger, postHookSpanFinalizer)
			return
		}

		sseReader := providerUtils.GetSSEDataReader(ctx, reader)
		chunkIndex := -1

		lastChunkTime := startTime

		for {
			// If context was cancelled/timed out, let defer handle it
			if ctx.Err() != nil {
				return
			}

			data, readErr := sseReader.ReadDataLine()
			if readErr != nil {
				if ctx.Err() != nil {
					return
				}
				if readErr != io.EOF {
					ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
					logger.Warn("Error reading stream: %v", readErr)
					providerUtils.ProcessAndSendError(ctx, postHookRunner, readErr, responseChan, logger, postHookSpanFinalizer)
				}
				break
			}
			jsonData := string(data)

			// Quick check for error field (allocation-free using sonic.GetFromString)
			if errorNode, _ := sonic.GetFromString(jsonData, "error"); errorNode.Exists() {
				// Only unmarshal when we know there's an error
				var rakshaErr schemas.RakshaError
				if err := sonic.UnmarshalString(jsonData, &rakshaErr); err == nil {
					if rakshaErr.Error != nil && rakshaErr.Error.Message != "" {
						ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
						providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, providerUtils.EnrichError(ctx, &rakshaErr, jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency), responseChan, logger, postHookSpanFinalizer)
						return
					}
				}
			}

			// Parse into raksha response
			var response schemas.RakshaSpeechStreamResponse
			if err := sonic.UnmarshalString(jsonData, &response); err != nil {
				logger.Warn("Failed to parse stream response: %v", err)
				continue
			}

			if postResponseConverter != nil {
				if converted := postResponseConverter(&response); converted != nil {
					response = *converted
				} else {
					logger.Warn("postResponseConverter returned nil; leaving chunk unmodified")
				}
			}

			chunkIndex++

			response.ExtraFields = schemas.RakshaResponseExtraFields{
				ChunkIndex: chunkIndex,
				Latency:    time.Since(lastChunkTime).Milliseconds(),
			}
			lastChunkTime = time.Now()

			if sendBackRawResponse {
				response.ExtraFields.RawResponse = jsonData
			}

			if response.Usage != nil {
				response.ExtraFields.Latency = time.Since(startTime).Milliseconds()
				if sendBackRawRequest {
					providerUtils.ParseAndSetRawRequest(&response.ExtraFields, jsonBody)
				}
				response.BackfillParams(request)
				ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
				providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(nil, nil, nil, &response, nil, nil), responseChan, postHookSpanFinalizer)
				return
			}

			providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(nil, nil, nil, &response, nil, nil), responseChan, postHookSpanFinalizer)
		}
	}()

	return responseChan, nil
}

// Transcription handles non-streaming transcription requests.
// It creates a multipart form, adds fields, makes the API call, and returns the response.
// Returns the response and any error that occurred.
func (provider *OpenAIProvider) Transcription(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaTranscriptionRequest) (*schemas.RakshaTranscriptionResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.TranscriptionRequest); err != nil {
		return nil, err
	}

	return HandleOpenAITranscriptionRequest(
		ctx,
		provider.client,
		provider.buildRequestURL(ctx, "/v1/audio/transcriptions", schemas.TranscriptionRequest),
		request,
		key,
		provider.networkConfig.ExtraHeaders,
		provider.GetProviderKey(),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		nil,
		provider.logger,
	)
}

func HandleOpenAITranscriptionRequest(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	url string,
	request *schemas.RakshaTranscriptionRequest,
	key schemas.Key,
	extraHeaders map[string]string,
	providerName schemas.ModelProvider,
	sendBackRawResponse bool,
	customResponseHandler responseHandler[schemas.RakshaTranscriptionResponse],
	logger schemas.Logger,
) (*schemas.RakshaTranscriptionResponse, *schemas.RakshaError) {
	// Large payload passthrough: stream multipart body directly without parsing
	if lpResult, lpErr, handled := handleOpenAILargePayloadPassthrough(ctx, client, url, BearerAuthHeader(key), extraHeaders, providerName, logger); handled {
		if lpErr != nil {
			return nil, lpErr
		}
		// Unmarshal the upstream response body to preserve transcription text and fields
		if len(lpResult.ResponseBody) > 0 {
			response := &schemas.RakshaTranscriptionResponse{}
			if err := sonic.Unmarshal(lpResult.ResponseBody, response); err != nil {
				return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseUnmarshal, err)
			}
			response.ExtraFields = schemas.RakshaResponseExtraFields{Latency: lpResult.Latency}
			return response, nil
		}
		return &schemas.RakshaTranscriptionResponse{
			ExtraFields: schemas.RakshaResponseExtraFields{Latency: lpResult.Latency},
		}, nil
	}

	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	// resp lifecycle: managed by finalizeOpenAIResponse or released on error paths
	respOwned := true
	defer func() {
		if respOwned {
			fasthttp.ReleaseResponse(resp)
		}
	}()
	activeClient := providerUtils.PrepareResponseStreaming(ctx, client, resp)

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)

	req.SetRequestURI(url)
	req.Header.SetMethod(http.MethodPost)
	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	// Use centralized converter
	reqBody := ToOpenAITranscriptionRequest(request)
	if reqBody == nil {
		return nil, providerUtils.NewRakshaOperationError("transcription input is not provided", nil)
	}

	// Create multipart form
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := ParseTranscriptionFormDataBodyFromRequest(writer, reqBody, providerName); err != nil {
		return nil, err
	}

	req.Header.SetContentType(writer.FormDataContentType()) // This sets multipart/form-data with boundary
	req.SetBody(body.Bytes())

	// Make request
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, providerUtils.SetErrorLatency(rakshaErr, latency)
	}
	// Extract provider response headers early so they're available on error paths too
	providerResponseHeaders := providerUtils.ExtractProviderResponseHeaders(resp)
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		logger.Debug("error from %s provider: %s", providerName, string(resp.Body()))
		return nil, providerUtils.SetErrorLatency(ParseOpenAIError(resp), latency)
	}

	responseBody, lpResult, finalErr := finalizeOpenAIResponse(ctx, resp, latency, providerName, logger)
	respOwned = false // ownership transferred
	if finalErr != nil {
		return nil, providerUtils.SetErrorLatency(finalErr, latency)
	}
	if lpResult != nil {
		return &schemas.RakshaTranscriptionResponse{
			ExtraFields: schemas.RakshaResponseExtraFields{Latency: lpResult.Latency},
		}, nil
	}

	// Check for empty response
	trimmed := strings.TrimSpace(string(responseBody))
	if len(trimmed) == 0 {
		return nil, providerUtils.SetErrorLatency(&schemas.RakshaError{
			IsRakshaError: true,
			Error: &schemas.ErrorField{
				Message: schemas.ErrProviderResponseEmpty,
			},
		}, latency)
	}

	copiedResponseBody := append([]byte(nil), responseBody...)

	// Parse OpenAI's transcription response directly into RakshaTranscribe
	response := &schemas.RakshaTranscriptionResponse{}
	var rawResponse interface{}
	if request.Params != nil && schemas.IsPlainTextTranscriptionFormat(request.Params.ResponseFormat) {
		response.Text = string(copiedResponseBody)
		if sendBackRawResponse {
			rawResponse = string(copiedResponseBody)
		}
	} else if customResponseHandler != nil {
		_, rawResponse, rakshaErr = customResponseHandler(copiedResponseBody, response, nil, false, sendBackRawResponse)
	} else {
		if err := sonic.Unmarshal(copiedResponseBody, response); err != nil {
			// Check if it's an HTML response
			if providerUtils.IsHTMLResponse(resp, copiedResponseBody) {
				return nil, providerUtils.SetErrorLatency(&schemas.RakshaError{
					IsRakshaError: false,
					Error: &schemas.ErrorField{
						Message: schemas.ErrProviderResponseHTML,
						Error:   errors.New(string(copiedResponseBody)),
					},
				}, latency)
			}
			return nil, providerUtils.SetErrorLatency(providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseUnmarshal, err), latency)
		}

		// TODO: add HandleProviderResponse here

		// Parse raw response for RawResponse field
		if sendBackRawResponse {
			if err := sonic.Unmarshal(copiedResponseBody, &rawResponse); err != nil {
				return nil, providerUtils.SetErrorLatency(providerUtils.NewRakshaOperationError(schemas.ErrProviderRawResponseUnmarshal, err), latency)
			}
		}
	}

	if rakshaErr != nil {
		return nil, providerUtils.SetErrorLatency(rakshaErr, latency)
	}

	response.ExtraFields = schemas.RakshaResponseExtraFields{
		Latency:                 latency.Milliseconds(),
		ProviderResponseHeaders: providerResponseHeaders,
	}

	if sendBackRawResponse {
		response.ExtraFields.RawResponse = rawResponse
	}

	return response, nil
}

// TranscriptionStream performs a streaming transcription request to the OpenAI API.
func (provider *OpenAIProvider) TranscriptionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaTranscriptionRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.TranscriptionStreamRequest); err != nil {
		return nil, err
	}

	return HandleOpenAITranscriptionStreamRequest(
		ctx,
		provider.streamingClient,
		provider.buildRequestURL(ctx, "/v1/audio/transcriptions", schemas.TranscriptionStreamRequest),
		request,
		BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		provider.networkConfig.StreamIdleTimeoutInSeconds,
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		false,
		provider.GetProviderKey(),
		postHookRunner,
		nil,
		nil,
		nil,
		provider.logger,
		postHookSpanFinalizer,
	)
}

// HandleOpenAITranscriptionStreamRequest handles transcription stream requests for OpenAI-compatible APIs.
// This shared function reduces code duplication between providers that use the same transcription stream request format.
func HandleOpenAITranscriptionStreamRequest(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	url string,
	request *schemas.RakshaTranscriptionRequest,
	authHeader map[string]string,
	extraHeaders map[string]string,
	streamIdleTimeoutInSeconds int,
	sendBackRawResponse bool,
	accumulateText bool,
	providerName schemas.ModelProvider,
	postHookRunner schemas.PostHookRunner,
	customResponseHandler responseHandler[schemas.RakshaTranscriptionStreamResponse],
	postRequestConverter func(*OpenAITranscriptionRequest) *OpenAITranscriptionRequest,
	postResponseConverter func(*schemas.RakshaTranscriptionStreamResponse) *schemas.RakshaTranscriptionStreamResponse,
	logger schemas.Logger,
	postHookSpanFinalizer func(context.Context),
) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	providerUtils.SetStreamIdleTimeoutIfEmpty(ctx, streamIdleTimeoutInSeconds)
	// Use centralized converter
	reqBody := ToOpenAITranscriptionRequest(request)
	if reqBody == nil {
		return nil, providerUtils.NewRakshaOperationError("transcription input is not provided", nil)
	}
	reqBody.Stream = schemas.Ptr(true)
	if postRequestConverter != nil {
		reqBody = postRequestConverter(reqBody)
	}

	// Create multipart form
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if rakshaErr := ParseTranscriptionFormDataBodyFromRequest(writer, reqBody, providerName); rakshaErr != nil {
		return nil, rakshaErr
	}

	// Prepare OpenAI headers
	headers := map[string]string{
		"Content-Type":  writer.FormDataContentType(),
		"Accept":        "text/event-stream",
		"Cache-Control": "no-cache",
	}

	if authHeader != nil {
		maps.Copy(headers, authHeader)
	}

	// Create HTTP request for streaming
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	resp.StreamBody = true
	defer fasthttp.ReleaseRequest(req)

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)

	req.Header.SetMethod(http.MethodPost)
	req.SetRequestURI(url)
	req.Header.SetContentType("application/json")

	// Set headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	req.SetBody(body.Bytes())

	startTime := time.Now()
	// Make the request
	err := client.Do(req, resp)
	latency := time.Since(startTime)
	if err != nil {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		if errors.Is(err, context.Canceled) {
			return nil, providerUtils.SetErrorLatency(&schemas.RakshaError{
				IsRakshaError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   err,
				},
			}, latency)
		}
		if errors.Is(err, fasthttp.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
			return nil, providerUtils.SetErrorLatency(providerUtils.NewRakshaTimeoutError(schemas.ErrProviderRequestTimedOut, err), latency)
		}
		return nil, providerUtils.SetErrorLatency(providerUtils.NewRakshaOperationError(schemas.ErrProviderDoRequest, err), latency)
	}

	// Store provider response headers in context before status check so error responses also forward them
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	// Check for HTTP errors
	if resp.StatusCode() != fasthttp.StatusOK {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		return nil, providerUtils.SetErrorLatency(ParseOpenAIError(resp), latency)
	}

	// Large payload streaming passthrough — pipe raw upstream SSE to client
	if providerUtils.SetupStreamingPassthrough(ctx, resp) {
		responseChan := make(chan *schemas.RakshaStreamChunk)
		providerUtils.CloseStream(ctx, responseChan)
		return responseChan, nil
	}

	// Create response channel
	responseChan := make(chan *schemas.RakshaStreamChunk, schemas.DefaultStreamBufferSize)

	// Start streaming in a goroutine
	go func() {
		defer providerUtils.EnsureStreamFinalizerCalled(ctx, postHookSpanFinalizer)
		defer func() {
			if ctx.Err() == context.Canceled {
				providerUtils.HandleStreamCancellation(ctx, postHookRunner, responseChan, logger, postHookSpanFinalizer, nil)
			} else if ctx.Err() == context.DeadlineExceeded {
				providerUtils.HandleStreamTimeout(ctx, postHookRunner, responseChan, logger, postHookSpanFinalizer, nil)
			}
			providerUtils.CloseStream(ctx, responseChan)
		}()
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		// Decompress gzip-encoded streams transparently (no-op for non-gzip)
		reader, releaseGzip := providerUtils.DecompressStreamBody(resp)
		defer releaseGzip()

		// Wrap reader with idle timeout to detect stalled streams.
		reader, stopIdleTimeout := providerUtils.NewIdleTimeoutReader(reader, resp.BodyStream(), providerUtils.GetStreamIdleTimeout(ctx), ctx)
		defer stopIdleTimeout()

		// Setup cancellation handler to close the raw network stream on ctx cancellation,
		// which immediately unblocks any in-progress read (including reads blocked inside a gzip decompression layer).
		stopCancellation := providerUtils.SetupStreamCancellation(ctx, resp.BodyStream(), logger)
		defer stopCancellation()

		// Skip scanner for non-SSE responses — avoids bufio.Scanner buffer bloat
		// on non-line-delimited data (e.g. provider returned JSON instead of SSE).
		reader, drained := providerUtils.DrainNonSSEStreamReader(resp, reader)
		if drained {
			ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
			providerUtils.ProcessAndSendError(ctx, postHookRunner, errors.New("provider returned non-SSE response for streaming request"), responseChan, logger, postHookSpanFinalizer)
			return
		}

		sseReader := providerUtils.GetSSEDataReader(ctx, reader)
		chunkIndex := -1

		lastChunkTime := startTime
		var fullTranscriptionText string

		for {
			// If context was cancelled/timed out, let defer handle it
			if ctx.Err() != nil {
				return
			}

			data, readErr := sseReader.ReadDataLine()
			if readErr != nil {
				if ctx.Err() != nil {
					return
				}
				if readErr != io.EOF {
					ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
					logger.Warn("Error reading stream: %v", readErr)
					providerUtils.ProcessAndSendError(ctx, postHookRunner, readErr, responseChan, logger, postHookSpanFinalizer)
				}
				break
			}
			jsonData := string(data)
			// TODo fix this
			response := &schemas.RakshaTranscriptionStreamResponse{}
			var rakshaErr *schemas.RakshaError
			if customResponseHandler != nil {
				_, _, rakshaErr = customResponseHandler([]byte(jsonData), response, nil, false, false)
				if rakshaErr != nil {
					if sendBackRawResponse {
						rakshaErr.ExtraFields.RawResponse = jsonData
					}
					ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
					providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, providerUtils.EnrichError(ctx, rakshaErr, body.Bytes(), []byte(jsonData), false, sendBackRawResponse, latency), responseChan, logger, postHookSpanFinalizer)
					return
				}
			} else {
				// Quick check for error field (allocation-free using sonic.GetFromString)
				if errorNode, _ := sonic.GetFromString(jsonData, "error"); errorNode.Exists() {
					// Only unmarshal when we know there's an error
					var rakshaErrVal schemas.RakshaError
					if err := sonic.UnmarshalString(jsonData, &rakshaErrVal); err == nil {
						if rakshaErrVal.Error != nil && rakshaErrVal.Error.Message != "" {
							ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
							respBody := append([]byte(nil), resp.Body()...)
							providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, providerUtils.EnrichError(ctx, &rakshaErrVal, body.Bytes(), respBody, false, sendBackRawResponse, latency), responseChan, logger, postHookSpanFinalizer)
							return
						}
					}
				}

				if err := sonic.UnmarshalString(jsonData, response); err != nil {
					logger.Warn("Failed to parse stream response: %v", err)
					continue

				}
			}

			if postResponseConverter != nil {
				if converted := postResponseConverter(response); converted != nil {
					response = converted
				} else {
					logger.Warn("postResponseConverter returned nil; leaving chunk unmodified")
				}
			}

			chunkIndex++

			response.ExtraFields = schemas.RakshaResponseExtraFields{
				ChunkIndex: chunkIndex,
				Latency:    time.Since(lastChunkTime).Milliseconds(),
			}
			lastChunkTime = time.Now()

			if sendBackRawResponse {
				response.ExtraFields.RawResponse = jsonData
			}

			if response.Usage != nil || response.Type == schemas.TranscriptionStreamResponseTypeDone {
				response.ExtraFields.Latency = time.Since(startTime).Milliseconds()
				ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)

				if accumulateText {
					response.Text = fullTranscriptionText
				}

				providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(nil, nil, nil, nil, response, nil), responseChan, postHookSpanFinalizer)
				return
			}

			providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(nil, nil, nil, nil, response, nil), responseChan, postHookSpanFinalizer)
		}
	}()

	return responseChan, nil
}

// ImageGeneration performs an Image Generation request to OpenAI's API.
// It formats the request, sends it to OpenAI, and processes the response.
// Returns a RakshaResponse containing the raksha response or an error if the request fails.
func (provider *OpenAIProvider) ImageGeneration(ctx *schemas.RakshaContext, key schemas.Key,
	req *schemas.RakshaImageGenerationRequest,
) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.ImageGenerationRequest); err != nil {
		return nil, err
	}

	return HandleOpenAIImageGenerationRequest(
		ctx,
		provider.client,
		provider.buildRequestURL(ctx, "/v1/images/generations", schemas.ImageGenerationRequest),
		req,
		key,
		provider.networkConfig.ExtraHeaders,
		provider.GetProviderKey(),
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.logger,
	)
}

// HandleOpenAIImageGenerationRequest handles image generation requests for OpenAI-compatible APIs.
// This shared function reduces code duplication between providers that use the same image generation request format.
func HandleOpenAIImageGenerationRequest(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	url string,
	request *schemas.RakshaImageGenerationRequest,
	key schemas.Key,
	extraHeaders map[string]string,
	providerName schemas.ModelProvider,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
	logger schemas.Logger,
) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	// resp lifecycle: managed by finalizeOpenAIResponse or released on error paths
	respOwned := true
	defer func() {
		if respOwned {
			fasthttp.ReleaseResponse(resp)
		}
	}()
	activeClient := providerUtils.PrepareResponseStreaming(ctx, client, resp)

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)

	req.SetRequestURI(url)
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")

	if value := key.Value.GetValue(); value != "" {
		req.Header.Set("Authorization", "Bearer "+value)
	}

	// Large payload passthrough: stream body directly without JSON marshaling
	if lpResult, lpErr, handled := handleOpenAILargePayloadPassthrough(ctx, client, url, BearerAuthHeader(key), extraHeaders, providerName, logger); handled {
		if lpErr != nil {
			return nil, lpErr
		}
		if len(lpResult.ResponseBody) > 0 {
			response := &schemas.RakshaImageGenerationResponse{}
			if err := sonic.Unmarshal(lpResult.ResponseBody, response); err != nil {
				return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseUnmarshal, err)
			}
			response.ExtraFields = schemas.RakshaResponseExtraFields{Latency: lpResult.Latency}
			return response, nil
		}
		return &schemas.RakshaImageGenerationResponse{
			ExtraFields: schemas.RakshaResponseExtraFields{Latency: lpResult.Latency},
		}, nil
	}

	// Use centralized converter
	jsonData, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToOpenAIImageGenerationRequest(request), nil
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	req.SetBody(jsonData)

	// Make request
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}
	// Extract provider response headers early so they're available on error paths too
	providerResponseHeaders := providerUtils.ExtractProviderResponseHeaders(resp)
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		logger.Debug(fmt.Sprintf("error from %s provider: %s", providerName, string(resp.Body())))
		return nil, providerUtils.EnrichError(ctx, ParseOpenAIError(resp), jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	body, lpResult, finalErr := finalizeOpenAIResponse(ctx, resp, latency, providerName, logger)
	respOwned = false // ownership transferred
	if finalErr != nil {
		return nil, finalErr
	}
	if lpResult != nil {
		return &schemas.RakshaImageGenerationResponse{
			ExtraFields: schemas.RakshaResponseExtraFields{Latency: lpResult.Latency},
		}, nil
	}

	response := &schemas.RakshaImageGenerationResponse{}

	// Use enhanced response handler with pre-allocated response
	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(body, response, jsonData, sendBackRawRequest, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	response.ExtraFields.Latency = latency.Milliseconds()
	response.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	// Set raw request if enabled
	if sendBackRawRequest {
		response.ExtraFields.RawRequest = rawRequest
	}

	// Set raw response if enabled
	if sendBackRawResponse {
		response.ExtraFields.RawResponse = rawResponse
	}

	return response, nil
}

// ImageGenerationStream handles streaming for image generation.
// It formats the request body, creates HTTP request, and uses shared streaming logic.
// Returns a channel for streaming responses and any error that occurred.
func (provider *OpenAIProvider) ImageGenerationStream(
	ctx *schemas.RakshaContext,
	postHookRunner schemas.PostHookRunner,
	postHookSpanFinalizer func(context.Context),
	key schemas.Key,
	request *schemas.RakshaImageGenerationRequest,
) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	if request == nil {
		return nil, providerUtils.NewRakshaOperationError("invalid request: nil", nil)
	}

	// Check if image generation stream is allowed for this provider
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.ImageGenerationStreamRequest); err != nil {
		return nil, err
	}

	// Use shared streaming logic
	return HandleOpenAIImageGenerationStreaming(
		ctx,
		provider.streamingClient,
		provider.buildRequestURL(ctx, "/v1/images/generations", schemas.ImageGenerationStreamRequest),
		request,
		BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		provider.networkConfig.StreamIdleTimeoutInSeconds,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		postHookRunner,
		nil,
		nil,
		nil,
		provider.logger,
		postHookSpanFinalizer,
	)
}

func HandleOpenAIImageGenerationStreaming(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	url string,
	request *schemas.RakshaImageGenerationRequest,
	authHeader map[string]string,
	extraHeaders map[string]string,
	streamIdleTimeoutInSeconds int,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
	providerName schemas.ModelProvider,
	postHookRunner schemas.PostHookRunner,
	customRequestConverter func(*schemas.RakshaImageGenerationRequest) (providerUtils.RequestBodyWithExtraParams, error),
	postRequestConverter func(*OpenAIImageGenerationRequest) *OpenAIImageGenerationRequest,
	postResponseConverter func(*schemas.RakshaImageGenerationStreamResponse) *schemas.RakshaImageGenerationStreamResponse,
	logger schemas.Logger,
	postHookSpanFinalizer func(context.Context),
) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	providerUtils.SetStreamIdleTimeoutIfEmpty(ctx, streamIdleTimeoutInSeconds)
	// Set headers
	headers := map[string]string{
		"Content-Type":  "application/json",
		"Accept":        "text/event-stream",
		"Cache-Control": "no-cache",
	}

	if authHeader != nil {
		// Copy auth header to headers
		maps.Copy(headers, authHeader)
	}

	jsonBody, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			if customRequestConverter != nil {
				return customRequestConverter(request)
			}
			reqBody := ToOpenAIImageGenerationRequest(request)
			if reqBody != nil {
				reqBody.Stream = schemas.Ptr(true)
				if postRequestConverter != nil {
					reqBody = postRequestConverter(reqBody)
				}
			}
			return reqBody, nil
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Create HTTP request for streaming
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	resp.StreamBody = true
	defer fasthttp.ReleaseRequest(req)

	// Updating request
	req.Header.SetMethod(http.MethodPost)
	req.SetRequestURI(url)
	req.Header.SetContentType("application/json")

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)

	// Set headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	setStreamingRequestBody(ctx, req, jsonBody, providerName)

	// Use streaming-aware client when large payload optimization is active — ensures
	// MaxResponseBodySize > 0 so ErrBodyTooLarge triggers StreamBody for Content-Length responses.
	activeClient := providerUtils.PrepareResponseStreaming(ctx, client, resp)

	startTime := time.Now()
	// Make the request
	err := activeClient.Do(req, resp)
	latency := time.Since(startTime)
	if err != nil {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		if errors.Is(err, context.Canceled) {
			return nil, providerUtils.SetErrorLatency(&schemas.RakshaError{
				IsRakshaError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   err,
				},
			}, latency)
		}
		if errors.Is(err, fasthttp.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
			return nil, providerUtils.SetErrorLatency(providerUtils.NewRakshaTimeoutError(schemas.ErrProviderRequestTimedOut, err), latency)
		}
		return nil, providerUtils.SetErrorLatency(providerUtils.NewRakshaOperationError(schemas.ErrProviderDoRequest, err), latency)
	}

	// Store provider response headers in context before status check so error responses also forward them
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	// Check for HTTP errors
	if resp.StatusCode() != fasthttp.StatusOK {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		return nil, providerUtils.EnrichError(ctx, ParseOpenAIError(resp), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Large payload streaming passthrough — pipe raw upstream SSE to client
	if providerUtils.SetupStreamingPassthrough(ctx, resp) {
		responseChan := make(chan *schemas.RakshaStreamChunk)
		providerUtils.CloseStream(ctx, responseChan)
		return responseChan, nil
	}

	// Create response channel
	responseChan := make(chan *schemas.RakshaStreamChunk, schemas.DefaultStreamBufferSize)

	// Start streaming in a goroutine
	go func() {
		defer providerUtils.EnsureStreamFinalizerCalled(ctx, postHookSpanFinalizer)
		defer func() {
			if ctx.Err() == context.Canceled {
				providerUtils.HandleStreamCancellation(ctx, postHookRunner, responseChan, logger, postHookSpanFinalizer, jsonBody)
			} else if ctx.Err() == context.DeadlineExceeded {
				providerUtils.HandleStreamTimeout(ctx, postHookRunner, responseChan, logger, postHookSpanFinalizer, jsonBody)
			}
			providerUtils.CloseStream(ctx, responseChan)
		}()
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		// Decompress gzip-encoded streams transparently (no-op for non-gzip)
		reader, releaseGzip := providerUtils.DecompressStreamBody(resp)
		defer releaseGzip()

		// Wrap reader with idle timeout to detect stalled streams.
		reader, stopIdleTimeout := providerUtils.NewIdleTimeoutReader(reader, resp.BodyStream(), providerUtils.GetStreamIdleTimeout(ctx), ctx)
		defer stopIdleTimeout()

		// Setup cancellation handler to close the raw network stream on ctx cancellation,
		// which immediately unblocks any in-progress read (including reads blocked inside a gzip decompression layer).
		stopCancellation := providerUtils.SetupStreamCancellation(ctx, resp.BodyStream(), logger)
		defer stopCancellation()

		// Skip scanner for non-SSE responses — avoids bufio.Scanner buffer bloat
		// on non-line-delimited data (e.g. provider returned JSON instead of SSE).
		reader, drained := providerUtils.DrainNonSSEStreamReader(resp, reader)
		if drained {
			ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
			providerUtils.ProcessAndSendError(ctx, postHookRunner, errors.New("provider returned non-SSE response for streaming request"), responseChan, logger, postHookSpanFinalizer)
			return
		}

		sseReader := providerUtils.GetSSEDataReader(ctx, reader)

		lastChunkTime := startTime
		// Track chunk indices per image - similar to how speech/transcription track chunkIndex
		imageChunkIndices := make(map[int]int) // image index -> chunk index
		// Track images that have started (via partial chunks) but not yet completed
		// This allows us to correctly match completed events to images even if chunks are interleaved
		incompleteImages := make(map[int]bool)
		maxImageIndex := -1 // Track maximum image index for NImages calculation

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			data, readErr := sseReader.ReadDataLine()
			if readErr != nil {
				if ctx.Err() != nil {
					return
				}
				if readErr != io.EOF {
					providerUtils.ProcessAndSendError(ctx, postHookRunner, readErr, responseChan, logger, postHookSpanFinalizer)
				}
				break
			}
			jsonData := string(data)

			// Quick check for error field (allocation-free using sonic.GetFromString)
			if errorNode, _ := sonic.GetFromString(jsonData, "error"); errorNode.Exists() {
				// Only unmarshal when we know there's an error
				var rakshaErr schemas.RakshaError
				if err := sonic.UnmarshalString(jsonData, &rakshaErr); err == nil {
					if rakshaErr.Error != nil && rakshaErr.Error.Message != "" {
						ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
						providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, providerUtils.EnrichError(ctx, &rakshaErr, jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency), responseChan, logger, postHookSpanFinalizer)
						return
					}
				}
			}

			// Parse minimally to extract usage and check for errors
			var response OpenAIImageStreamResponse
			if err := sonic.UnmarshalString(jsonData, &response); err != nil {
				logger.Warn("Failed to parse stream response: %v", err)
				continue
			}

			// Check if response type indicates an error
			if response.Type == "error" {
				rakshaErr := &schemas.RakshaError{
					IsRakshaError: false,
					Error:          &schemas.ErrorField{},
				}
				// Guard access to response.Error fields
				if response.Error != nil {
					rakshaErr.Error.Message = response.Error.Message
					if response.Error.Code != nil {
						rakshaErr.Error.Code = response.Error.Code
					}
					if response.Error.Param != nil {
						rakshaErr.Error.Param = response.Error.Param
					}
					if response.Error.Type != nil {
						rakshaErr.Error.Type = response.Error.Type
					}
				}
				ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
				providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, rakshaErr, responseChan, logger, postHookSpanFinalizer)
				return
			}

			// Determine if this is the final chunk
			isCompleted := response.Type == schemas.ImageGenerationEventTypeCompleted

			// Determine image index with robust tracking for interleaved chunks
			// Both partial and completed chunks should use PartialImageIndex when available
			var imageIndex int
			if response.PartialImageIndex != nil {
				// Use explicit index from response
				imageIndex = *response.PartialImageIndex
				if isCompleted {
					// Mark this image as completed
					delete(incompleteImages, imageIndex)
				} else {
					// Mark this image as started (incomplete)
					incompleteImages[imageIndex] = true
				}
			} else {
				// Fallback: PartialImageIndex is nil, use tracked state
				if isCompleted {
					// For completed chunks, match to the oldest incomplete image
					// This handles interleaved chunks correctly
					if len(incompleteImages) == 0 {
						// Fallback: if no incomplete images tracked, this shouldn't happen in normal flow
						// but we'll default to 0 to prevent panics
						imageIndex = 0
						logger.Warn("Received completed event but no incomplete images tracked, defaulting to index 0")
					} else {
						// Find the minimum (oldest) incomplete image index
						// Completed events should match the oldest image that was started
						minIndex := -1
						for idx := range incompleteImages {
							if minIndex == -1 || idx < minIndex {
								minIndex = idx
							}
						}
						imageIndex = minIndex
						// Mark this image as completed
						delete(incompleteImages, imageIndex)
						logger.Warn("Completed event missing PartialImageIndex, using oldest incomplete image index %d", imageIndex)
					}
				} else {
					// For partial chunks without PartialImageIndex, allocate a new unique index
					// Use maxImageIndex + 1 to ensure uniqueness
					imageIndex = maxImageIndex + 1
					// Mark this image as started (incomplete)
					incompleteImages[imageIndex] = true
				}
			}

			// Update maximum image index for NImages calculation
			if imageIndex > maxImageIndex {
				maxImageIndex = imageIndex
			}

			// Increment chunk index for this image
			if _, exists := imageChunkIndices[imageIndex]; !exists {
				imageChunkIndices[imageIndex] = 0
			} else {
				imageChunkIndices[imageIndex]++
			}
			chunkIndex := imageChunkIndices[imageIndex]
			// Build chunk with all OpenAI fields
			chunk := &schemas.RakshaImageGenerationStreamResponse{
				Type:         response.Type,
				Index:        imageIndex, // Which image (0-N)
				ChunkIndex:   chunkIndex, // Chunk order within this image (top-level)
				CreatedAt:    response.CreatedAt,
				Size:         response.Size,
				Quality:      response.Quality,
				Background:   response.Background,
				OutputFormat: response.OutputFormat,
				ExtraFields: schemas.RakshaResponseExtraFields{
					ChunkIndex: chunkIndex, // Chunk order within this image
					Latency:    time.Since(lastChunkTime).Milliseconds(),
				},
			}

			if postResponseConverter != nil {
				if converted := postResponseConverter(chunk); converted != nil {
					chunk = converted
				} else {
					logger.Warn("postResponseConverter returned nil; leaving chunk unmodified")
				}
			}

			// Only set PartialImageIndex for partial images, not for completed events
			if !isCompleted {
				chunk.PartialImageIndex = response.PartialImageIndex
			}
			// Set SequenceNumber if present
			if response.SequenceNumber != nil {
				chunk.SequenceNumber = *response.SequenceNumber
			}
			lastChunkTime = time.Now()

			// Copy b64_json if present
			if response.B64JSON != nil {
				chunk.B64JSON = *response.B64JSON
			}

			// Set raw response on every chunk if enabled
			if sendBackRawResponse {
				chunk.ExtraFields.RawResponse = jsonData
			}

			if isCompleted {
				if response.Usage != nil && maxImageIndex >= 0 {
					if response.Usage.OutputTokensDetails == nil {
						response.Usage.OutputTokensDetails = &schemas.ImageTokenDetails{}
					}
					if response.Usage.OutputTokensDetails.NImages == 0 {
						response.Usage.OutputTokensDetails.NImages = maxImageIndex + 1
					}
				}
				chunk.Usage = response.Usage
				// For completed chunk, use total latency from start
				chunk.ExtraFields.Latency = time.Since(startTime).Milliseconds()
				chunk.BackfillParams(&schemas.RakshaRequest{
					ImageGenerationRequest: request,
				})
				// Set raw request only on final chunk if enabled
				if sendBackRawRequest {
					providerUtils.ParseAndSetRawRequest(&chunk.ExtraFields, jsonBody)
				}
				ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
			}

			providerUtils.ProcessAndSendResponse(ctx, postHookRunner,
				providerUtils.GetRakshaResponseForStreamResponse(nil, nil, nil, nil, nil, chunk),
				responseChan, postHookSpanFinalizer)

			if isCompleted {
				return
			}
		}
	}()

	return responseChan, nil
}

// Rerank is not supported by the OpenAI provider.
func (provider *OpenAIProvider) Rerank(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaRerankRequest) (*schemas.RakshaRerankResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.RerankRequest, provider.GetProviderKey())
}

// OCR is not supported by the Openai provider.
func (provider *OpenAIProvider) OCR(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaOCRRequest) (*schemas.RakshaOCRResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.OCRRequest, provider.GetProviderKey())
}

// VideoGeneration performs a video generation request via the OpenAI API.
func (provider *OpenAIProvider) VideoGeneration(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaVideoGenerationRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.VideoGenerationRequest); err != nil {
		return nil, err
	}

	return HandleOpenAIVideoGenerationRequest(
		ctx,
		provider.client,
		provider.buildRequestURL(ctx, "/v1/videos", schemas.VideoGenerationRequest),
		request,
		key,
		provider.networkConfig.ExtraHeaders,
		provider.GetProviderKey(),
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.logger,
	)
}

// VideoRetrieve retrieves a video generation job from the OpenAI API.
func (provider *OpenAIProvider) VideoRetrieve(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaVideoRetrieveRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.VideoRetrieveRequest); err != nil {
		return nil, err
	}

	providerName := provider.GetProviderKey()
	if request.ID == "" {
		return nil, providerUtils.NewRakshaOperationError("video_id is required", nil)
	}
	videoID := providerUtils.StripVideoIDProviderSuffix(request.ID, providerName)

	return HandleOpenAIVideoRetrieveRequest(
		ctx,
		provider.client,
		provider.buildRequestURL(ctx, "/v1/videos/"+videoID, schemas.VideoRetrieveRequest),
		request,
		key,
		provider.networkConfig.ExtraHeaders,
		nil, // OpenAI uses Bearer from key
		providerName,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.VideoDownload,
		provider.logger,
	)
}

// VideoDownload downloads video content from OpenAI.
func (provider *OpenAIProvider) VideoDownload(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaVideoDownloadRequest) (*schemas.RakshaVideoDownloadResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.VideoDownloadRequest); err != nil {
		return nil, err
	}

	providerName := provider.GetProviderKey()

	if request.ID == "" {
		return nil, providerUtils.NewRakshaOperationError("video_id is required", nil)
	}
	videoID := providerUtils.StripVideoIDProviderSuffix(request.ID, providerName)

	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Set headers
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	// Build URL: /v1/videos/{video_id}/content
	requestURL := provider.buildRequestURL(ctx, "/v1/videos/"+videoID+"/content", schemas.VideoDownloadRequest)

	if request.Variant != nil && *request.Variant != "" {
		// attach variant to url if present
		requestURL = fmt.Sprintf("%s?variant=%s", requestURL, url.QueryEscape(string(*request.Variant)))
	}

	req.SetRequestURI(requestURL)
	req.Header.SetMethod(http.MethodGet)

	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	// Make request
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}
	// Extract provider response headers early so they're available on error paths too
	providerResponseHeaders := providerUtils.ExtractProviderResponseHeaders(resp)
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		provider.logger.Debug("error from %s provider: %s", providerName, string(resp.Body()))
		return nil, providerUtils.SetErrorLatency(ParseOpenAIError(resp), latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err)
	}

	// Get content type from response
	contentType := string(resp.Header.ContentType())
	if contentType == "" {
		// Default to video/mp4 if not specified
		contentType = "video/mp4"
	}

	// Copy the binary content
	content := append([]byte(nil), body...)

	return &schemas.RakshaVideoDownloadResponse{
		VideoID:     providerUtils.AddVideoIDProviderSuffix(videoID, providerName),
		Content:     content,
		ContentType: contentType,
		ExtraFields: schemas.RakshaResponseExtraFields{
			Latency:                 latency.Milliseconds(),
			ProviderResponseHeaders: providerResponseHeaders,
		},
	}, nil
}

// VideoDelete deletes a video generation job from the OpenAI API.
func (provider *OpenAIProvider) VideoDelete(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaVideoDeleteRequest) (*schemas.RakshaVideoDeleteResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.VideoDeleteRequest); err != nil {
		return nil, err
	}

	providerName := provider.GetProviderKey()

	if request.ID == "" {
		return nil, providerUtils.NewRakshaOperationError("video_id is required", nil)
	}
	videoID := providerUtils.StripVideoIDProviderSuffix(request.ID, providerName)

	return HandleOpenAIVideoDeleteRequest(
		ctx,
		provider.client,
		provider.buildRequestURL(ctx, "/v1/videos/"+videoID, schemas.VideoDeleteRequest),
		videoID,
		key,
		provider.networkConfig.ExtraHeaders,
		providerName,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.logger,
	)
}

// VideoList lists videos from OpenAI.
func (provider *OpenAIProvider) VideoList(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaVideoListRequest) (*schemas.RakshaVideoListResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.VideoListRequest); err != nil {
		return nil, err
	}

	return HandleOpenAIVideoListRequest(
		ctx,
		provider.client,
		provider.buildRequestURL(ctx, "/v1/videos", schemas.VideoListRequest),
		request,
		key,
		provider.networkConfig.ExtraHeaders,
		provider.GetProviderKey(),
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.logger,
	)
}

// HandleOpenAIVideoGenerationRequest handles video generation requests for OpenAI-compatible APIs.
// It creates a multipart form, adds fields, makes the API call, and returns the response.
func HandleOpenAIVideoGenerationRequest(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	url string,
	request *schemas.RakshaVideoGenerationRequest,
	key schemas.Key,
	extraHeaders map[string]string,
	providerName schemas.ModelProvider,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
	logger schemas.Logger,
) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)

	req.SetRequestURI(url)
	req.Header.SetMethod(http.MethodPost)
	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	// Use centralized converter
	reqBody, err := ToOpenAIVideoGenerationRequest(request)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError("failed to convert video generation request to openai format", err)
	}
	if reqBody == nil {
		return nil, providerUtils.NewRakshaOperationError("video generation input is not provided", nil)
	}

	// Create multipart form
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := parseVideoGenerationFormDataBodyFromRequest(writer, reqBody, providerName); err != nil {
		return nil, err
	}

	req.Header.SetContentType(writer.FormDataContentType()) // This sets multipart/form-data with boundary
	req.SetBody(body.Bytes())

	// Make request
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}
	// Extract provider response headers early so they're available on error paths too
	providerResponseHeaders := providerUtils.ExtractProviderResponseHeaders(resp)
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		logger.Debug("error from %s provider: %s", providerName, string(resp.Body()))
		return nil, providerUtils.SetErrorLatency(ParseOpenAIError(resp), latency)
	}

	responseBody, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err)
	}

	// Check for empty response
	trimmed := strings.TrimSpace(string(responseBody))
	if len(trimmed) == 0 {
		return nil, &schemas.RakshaError{
			IsRakshaError: true,
			Error: &schemas.ErrorField{
				Message: schemas.ErrProviderResponseEmpty,
			},
		}
	}

	// Parse OpenAI's video generation response
	response := &schemas.RakshaVideoGenerationResponse{}
	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(responseBody, response, nil, sendBackRawRequest, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	if response.ID != "" {
		response.ID = providerUtils.AddVideoIDProviderSuffix(response.ID, providerName)
	}

	response.ExtraFields = schemas.RakshaResponseExtraFields{
		Latency:                 latency.Milliseconds(),
		ProviderResponseHeaders: providerResponseHeaders,
	}

	if sendBackRawResponse {
		response.ExtraFields.RawResponse = rawResponse
	}

	if sendBackRawRequest {
		response.ExtraFields.RawRequest = rawRequest
	}

	return response, nil
}

// VideoDownloadFunc downloads video content. Used by HandleOpenAIVideoRetrieveRequest for enrichment.
type VideoDownloadHandler func(ctx *schemas.RakshaContext, key schemas.Key, req *schemas.RakshaVideoDownloadRequest) (*schemas.RakshaVideoDownloadResponse, *schemas.RakshaError)

// HandleOpenAIVideoRetrieveRequest handles video retrieve requests for OpenAI-compatible APIs.
// When authHeaders is non-nil, they are applied for authentication (e.g. Azure api-key); otherwise Bearer from key is used.
// When videoDownloadFunc is non-nil and ctx has VideoOutputRequested with status completed, the handler fetches video content and appends to response.
func HandleOpenAIVideoRetrieveRequest(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	url string,
	request *schemas.RakshaVideoRetrieveRequest,
	key schemas.Key,
	extraHeaders map[string]string,
	authHeaders map[string]string,
	providerName schemas.ModelProvider,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
	videoDownloaddHandler VideoDownloadHandler,
	logger schemas.Logger,
) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)
	req.SetRequestURI(url)
	req.Header.SetMethod(http.MethodGet)
	req.Header.SetContentType("application/json")

	if len(authHeaders) > 0 {
		for k, v := range authHeaders {
			req.Header.Set(k, v)
		}
	} else if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}
	// Extract provider response headers early so they're available on error paths too
	providerResponseHeaders := providerUtils.ExtractProviderResponseHeaders(resp)
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)

	if resp.StatusCode() != fasthttp.StatusOK {
		logger.Debug("error from %s provider: %s", providerName, string(resp.Body()))
		return nil, providerUtils.SetErrorLatency(ParseOpenAIError(resp), latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err)
	}

	response := &schemas.RakshaVideoGenerationResponse{}
	_, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(body, response, nil, sendBackRawRequest, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, rakshaErr
	}
	if response.ID != "" {
		response.ID = providerUtils.AddVideoIDProviderSuffix(response.ID, providerName)
	}
	if response.RemixedFromVideoID != nil && *response.RemixedFromVideoID != "" {
		remixID := providerUtils.AddVideoIDProviderSuffix(*response.RemixedFromVideoID, providerName)
		response.RemixedFromVideoID = &remixID
	}

	if videoDownloaddHandler != nil {
		downloadVideo, ok := ctx.Value(schemas.RakshaContextKeyVideoOutputRequested).(bool)
		if ok && downloadVideo && response.Status == schemas.VideoStatusCompleted {
			videoDownloadRequest := &schemas.RakshaVideoDownloadRequest{
				Provider: providerName,
				ID:       response.ID,
			}
			videoDownloadResponse, rakshaErr := videoDownloaddHandler(ctx, key, videoDownloadRequest)
			if rakshaErr != nil {
				return nil, rakshaErr
			}
			if len(videoDownloadResponse.Content) > 0 {
				output := schemas.VideoOutput{
					Type:        schemas.VideoOutputTypeBase64,
					ContentType: videoDownloadResponse.ContentType,
				}
				base64Data := base64.StdEncoding.EncodeToString(videoDownloadResponse.Content)
				output.Base64Data = &base64Data
				response.Videos = append(response.Videos, output)
			} else {
				logger.Warn("no content found for video download request for %s video retrieve request", providerName)
			}
		}
	}

	response.ExtraFields = schemas.RakshaResponseExtraFields{
		Latency:                 latency.Milliseconds(),
		ProviderResponseHeaders: providerResponseHeaders,
	}
	if sendBackRawResponse {
		response.ExtraFields.RawResponse = rawResponse
	}
	return response, nil
}

// HandleOpenAIVideoDeleteRequest handles video deletion requests for OpenAI-compatible APIs.
func HandleOpenAIVideoDeleteRequest(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	url string,
	videoID string,
	key schemas.Key,
	extraHeaders map[string]string,
	providerName schemas.ModelProvider,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
	logger schemas.Logger,
) (*schemas.RakshaVideoDeleteResponse, *schemas.RakshaError) {
	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Set headers
	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)
	req.SetRequestURI(url)
	req.Header.SetMethod(http.MethodDelete)
	req.Header.SetContentType("application/json")

	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	// Make request
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}
	// Extract provider response headers early so they're available on error paths too
	providerResponseHeaders := providerUtils.ExtractProviderResponseHeaders(resp)
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		logger.Debug("error from %s provider: %s", providerName, string(resp.Body()))
		return nil, providerUtils.SetErrorLatency(ParseOpenAIError(resp), latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err)
	}

	// Parse OpenAI's video response
	response := &schemas.RakshaVideoDeleteResponse{}
	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(body, response, nil, sendBackRawRequest, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, rakshaErr
	}
	if response.ID != "" {
		response.ID = providerUtils.AddVideoIDProviderSuffix(response.ID, providerName)
	}

	response.ExtraFields = schemas.RakshaResponseExtraFields{
		Latency:                 latency.Milliseconds(),
		ProviderResponseHeaders: providerResponseHeaders,
	}

	if sendBackRawResponse {
		response.ExtraFields.RawResponse = rawResponse
	}
	if sendBackRawRequest {
		response.ExtraFields.RawRequest = rawRequest
	}

	return response, nil
}

// HandleOpenAIVideoListRequest handles video list requests for OpenAI-compatible APIs.
func HandleOpenAIVideoListRequest(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	baseURL string,
	request *schemas.RakshaVideoListRequest,
	key schemas.Key,
	extraHeaders map[string]string,
	providerName schemas.ModelProvider,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
	logger schemas.Logger,
) (*schemas.RakshaVideoListResponse, *schemas.RakshaError) {
	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Build URL with query parameters
	values := url.Values{}
	if request.After != nil && *request.After != "" {
		values.Set("after", providerUtils.StripVideoIDProviderSuffix(*request.After, providerName))
	}
	if request.Limit != nil {
		values.Set("limit", fmt.Sprintf("%d", *request.Limit))
	}
	if request.Order != nil && *request.Order != "" {
		values.Set("order", *request.Order)
	}
	finalURL := baseURL
	if encoded := values.Encode(); encoded != "" {
		finalURL = baseURL + "?" + encoded
	}

	// Set headers
	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)
	req.SetRequestURI(finalURL)
	req.Header.SetMethod(http.MethodGet)
	req.Header.SetContentType("application/json")

	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	// Make request
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}
	// Extract provider response headers early so they're available on error paths too
	providerResponseHeaders := providerUtils.ExtractProviderResponseHeaders(resp)
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		logger.Debug("error from %s provider: %s", providerName, string(resp.Body()))
		return nil, providerUtils.SetErrorLatency(ParseOpenAIError(resp), latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err)
	}

	response := &schemas.RakshaVideoListResponse{}
	_, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(body, response, nil, sendBackRawRequest, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, rakshaErr
	}
	for i := range response.Data {
		if response.Data[i].ID != "" {
			response.Data[i].ID = providerUtils.AddVideoIDProviderSuffix(response.Data[i].ID, providerName)
		}
		if response.Data[i].RemixedFromVideoID != nil && *response.Data[i].RemixedFromVideoID != "" {
			remixID := providerUtils.AddVideoIDProviderSuffix(*response.Data[i].RemixedFromVideoID, providerName)
			response.Data[i].RemixedFromVideoID = &remixID
		}
	}
	if response.FirstID != nil && *response.FirstID != "" {
		firstID := providerUtils.AddVideoIDProviderSuffix(*response.FirstID, providerName)
		response.FirstID = &firstID
	}
	if response.LastID != nil && *response.LastID != "" {
		lastID := providerUtils.AddVideoIDProviderSuffix(*response.LastID, providerName)
		response.LastID = &lastID
	}

	response.ExtraFields = schemas.RakshaResponseExtraFields{
		Latency:                 latency.Milliseconds(),
		ProviderResponseHeaders: providerResponseHeaders,
	}

	if sendBackRawResponse {
		response.ExtraFields.RawResponse = rawResponse
	}

	return response, nil
}

// CountTokens performs a count tokens request to the OpenAI API.
func (provider *OpenAIProvider) CountTokens(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaResponsesRequest) (*schemas.RakshaCountTokensResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.CountTokensRequest); err != nil {
		return nil, err
	}

	return HandleOpenAICountTokensRequest(
		ctx,
		provider.client,
		provider.buildRequestURL(ctx, "/v1/responses/input_tokens", schemas.CountTokensRequest),
		request,
		key,
		provider.networkConfig.ExtraHeaders,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		provider.logger,
	)
}

// Compaction compacts a conversation context window using OpenAI's /v1/responses/compact endpoint.
func (provider *OpenAIProvider) Compaction(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaCompactionRequest) (*schemas.RakshaCompactionResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.CompactionRequest); err != nil {
		return nil, err
	}

	return HandleOpenAICompactionRequest(
		ctx,
		provider.client,
		provider.buildRequestURL(ctx, "/v1/responses/compact", schemas.CompactionRequest),
		request,
		BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		provider.logger,
	)
}

// HandleOpenAICompactionRequest handles a compaction request to OpenAI's /v1/responses/compact endpoint.
func HandleOpenAICompactionRequest(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	url string,
	request *schemas.RakshaCompactionRequest,
	authHeader map[string]string,
	extraHeaders map[string]string,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
	providerName schemas.ModelProvider,
	logger schemas.Logger,
) (*schemas.RakshaCompactionResponse, *schemas.RakshaError) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	respOwned := true
	defer func() {
		if respOwned {
			fasthttp.ReleaseResponse(resp)
		}
	}()
	activeClient := providerUtils.PrepareResponseStreaming(ctx, client, resp)

	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)
	req.SetRequestURI(url)
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")

	for k, v := range authHeader {
		req.Header.Set(k, v)
	}

	if lpResult, lpErr, handled := handleOpenAILargePayloadPassthrough(ctx, client, url, authHeader, extraHeaders, providerName, logger); handled {
		if lpErr != nil {
			return nil, lpErr
		}
		if len(lpResult.ResponseBody) > 0 {
			response := &schemas.RakshaCompactionResponse{}
			if err := sonic.Unmarshal(lpResult.ResponseBody, response); err != nil {
				return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseUnmarshal, err)
			}
			response.ExtraFields = schemas.RakshaResponseExtraFields{Latency: lpResult.Latency}
			return response, nil
		}
		return &schemas.RakshaCompactionResponse{
			ExtraFields: schemas.RakshaResponseExtraFields{Latency: lpResult.Latency},
		}, nil
	}

	jsonData, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToOpenAICompactionRequest(ctx, request), nil
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	req.SetBody(jsonData)

	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	providerResponseHeaders := providerUtils.ExtractProviderResponseHeaders(resp)
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)

	if resp.StatusCode() != fasthttp.StatusOK {
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		logger.Debug("error from %s provider with status %d", providerName, resp.StatusCode())
		return nil, providerUtils.EnrichError(ctx, ParseOpenAIError(resp), jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	body, lpResult, finalErr := finalizeOpenAIResponse(ctx, resp, latency, providerName, logger)
	respOwned = false
	if finalErr != nil {
		return nil, providerUtils.EnrichError(ctx, finalErr, jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}
	if lpResult != nil {
		return &schemas.RakshaCompactionResponse{
			ExtraFields: schemas.RakshaResponseExtraFields{Latency: lpResult.Latency},
		}, nil
	}

	response := &schemas.RakshaCompactionResponse{}
	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(body, response, jsonData, sendBackRawRequest, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, body, sendBackRawRequest, sendBackRawResponse, latency)
	}

	response.ExtraFields.Latency = latency.Milliseconds()
	response.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	if providerUtils.ShouldSendBackRawRequest(ctx, sendBackRawRequest) {
		response.ExtraFields.RawRequest = rawRequest
	}
	if providerUtils.ShouldSendBackRawResponse(ctx, sendBackRawResponse) {
		response.ExtraFields.RawResponse = rawResponse
	}

	return response, nil
}

// HandleOpenAICountTokensRequest handles a count tokens request to OpenAI's API.
func HandleOpenAICountTokensRequest(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	url string,
	request *schemas.RakshaResponsesRequest,
	key schemas.Key,
	extraHeaders map[string]string,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
	providerName schemas.ModelProvider,
	logger schemas.Logger,
) (*schemas.RakshaCountTokensResponse, *schemas.RakshaError) {
	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	// resp lifecycle: managed by finalizeOpenAIResponse or released on error paths
	respOwned := true
	defer func() {
		if respOwned {
			fasthttp.ReleaseResponse(resp)
		}
	}()
	activeClient := providerUtils.PrepareResponseStreaming(ctx, client, resp)

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)

	req.SetRequestURI(url)
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")

	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	// Large payload passthrough: stream body directly without JSON marshaling
	if lpResult, lpErr, handled := handleOpenAILargePayloadPassthrough(ctx, client, url, BearerAuthHeader(key), extraHeaders, providerName, logger); handled {
		if lpErr != nil {
			return nil, lpErr
		}
		if len(lpResult.ResponseBody) > 0 {
			response := &schemas.RakshaCountTokensResponse{}
			if err := sonic.Unmarshal(lpResult.ResponseBody, response); err != nil {
				return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseUnmarshal, err)
			}
			response.ExtraFields = schemas.RakshaResponseExtraFields{Latency: lpResult.Latency}
			return response, nil
		}
		return &schemas.RakshaCountTokensResponse{
			ExtraFields: schemas.RakshaResponseExtraFields{Latency: lpResult.Latency},
		}, nil
	}

	jsonData, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToOpenAIResponsesRequest(ctx, request), nil
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	req.SetBody(jsonData)

	// Make request
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}
	// Extract provider response headers early so they're available on error paths too
	providerResponseHeaders := providerUtils.ExtractProviderResponseHeaders(resp)
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		logger.Debug(fmt.Sprintf("error from %s provider: %s", providerName, string(resp.Body())))
		return nil, providerUtils.EnrichError(ctx, ParseOpenAIError(resp), jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	body, lpResult, finalErr := finalizeOpenAIResponse(ctx, resp, latency, providerName, logger)
	respOwned = false // ownership transferred
	if finalErr != nil {
		return nil, finalErr
	}
	if lpResult != nil {
		return &schemas.RakshaCountTokensResponse{
			ExtraFields: schemas.RakshaResponseExtraFields{Latency: lpResult.Latency},
		}, nil
	}

	response := &schemas.RakshaCountTokensResponse{}

	// Use enhanced response handler with pre-allocated response
	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(body, response, jsonData, sendBackRawRequest, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	response.Model = request.Model
	response.ExtraFields.Latency = latency.Milliseconds()
	response.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	if providerUtils.ShouldSendBackRawRequest(ctx, sendBackRawRequest) {
		response.ExtraFields.RawRequest = rawRequest
	}

	if providerUtils.ShouldSendBackRawResponse(ctx, sendBackRawResponse) {
		response.ExtraFields.RawResponse = rawResponse
	}

	return response, nil
}

// ImageEdit performs image editing via the OpenAI Images API.
func (provider *OpenAIProvider) ImageEdit(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageEditRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.ImageEditRequest); err != nil {
		return nil, err
	}

	return HandleOpenAIImageEditRequest(
		ctx,
		provider.client,
		provider.buildRequestURL(ctx, "/v1/images/edits", schemas.ImageEditRequest),
		request,
		key,
		provider.networkConfig.ExtraHeaders,
		false,
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		provider.logger,
	)
}

func HandleOpenAIImageEditRequest(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	url string,
	request *schemas.RakshaImageEditRequest,
	key schemas.Key,
	extraHeaders map[string]string,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
	providerName schemas.ModelProvider,
	logger schemas.Logger,
) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	// Large payload passthrough: stream multipart body directly without parsing
	if lpResult, lpErr, handled := handleOpenAILargePayloadPassthrough(ctx, client, url, BearerAuthHeader(key), extraHeaders, providerName, logger); handled {
		if lpErr != nil {
			return nil, lpErr
		}
		if len(lpResult.ResponseBody) > 0 {
			response := &schemas.RakshaImageGenerationResponse{}
			if err := sonic.Unmarshal(lpResult.ResponseBody, response); err != nil {
				return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseUnmarshal, err)
			}
			response.ExtraFields = schemas.RakshaResponseExtraFields{Latency: lpResult.Latency}
			return response, nil
		}
		return &schemas.RakshaImageGenerationResponse{
			ExtraFields: schemas.RakshaResponseExtraFields{Latency: lpResult.Latency},
		}, nil
	}

	openaiReq := ToOpenAIImageEditRequest(request)
	if openaiReq == nil {
		return nil, providerUtils.NewRakshaOperationError("failed to convert request to OpenAI format", nil)
	}

	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	// resp lifecycle: managed by finalizeOpenAIResponse or released on error paths
	respOwned := true
	defer func() {
		if respOwned {
			fasthttp.ReleaseResponse(resp)
		}
	}()
	activeClient := providerUtils.PrepareResponseStreaming(ctx, client, resp)

	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)
	req.SetRequestURI(url)
	req.Header.SetMethod(http.MethodPost)

	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}
	req.Header.Set("Content-Type", "multipart/form-data")

	// Create multipart form
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := parseImageEditFormDataBodyFromRequest(writer, openaiReq, providerName); err != nil {
		return nil, err
	}

	req.Header.SetContentType(writer.FormDataContentType())
	bodyData := body.Bytes()
	req.SetBody(bodyData)

	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, nil, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}
	// Extract provider response headers early so they're available on error paths too
	providerResponseHeaders := providerUtils.ExtractProviderResponseHeaders(resp)
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)

	if resp.StatusCode() != fasthttp.StatusOK {
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		return nil, providerUtils.EnrichError(ctx, ParseOpenAIError(resp), nil, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	bodyBytes, lpResult, finalErr := finalizeOpenAIResponse(ctx, resp, latency, providerName, logger)
	respOwned = false // ownership transferred
	if finalErr != nil {
		return nil, finalErr
	}
	if lpResult != nil {
		return &schemas.RakshaImageGenerationResponse{
			ExtraFields: schemas.RakshaResponseExtraFields{Latency: lpResult.Latency},
		}, nil
	}

	response := &schemas.RakshaImageGenerationResponse{}
	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(bodyBytes, response, nil, false, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, rakshaErr
	}
	response.ExtraFields.Latency = latency.Milliseconds()
	response.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	// Set raw request if enabled
	if sendBackRawRequest {
		response.ExtraFields.RawRequest = rawRequest
	}

	// Set raw response if enabled
	if sendBackRawResponse {
		response.ExtraFields.RawResponse = rawResponse
	}
	return response, nil
}

// ImageEditStream streams image edits via the OpenAI Images API.
func (provider *OpenAIProvider) ImageEditStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaImageEditRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	// Check if image generation stream is allowed for this provider
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.ImageEditStreamRequest); err != nil {
		return nil, err
	}

	return HandleOpenAIImageEditStreamRequest(
		ctx,
		provider.streamingClient,
		provider.buildRequestURL(ctx, "/v1/images/edits", schemas.ImageEditStreamRequest),
		request,
		BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		provider.networkConfig.StreamIdleTimeoutInSeconds,
		false,
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		postHookRunner,
		nil,
		nil,
		nil,
		provider.logger,
		postHookSpanFinalizer,
	)
}

func HandleOpenAIImageEditStreamRequest(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	url string,
	request *schemas.RakshaImageEditRequest,
	authHeader map[string]string,
	extraHeaders map[string]string,
	streamIdleTimeoutInSeconds int,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
	providerName schemas.ModelProvider,
	postHookRunner schemas.PostHookRunner,
	customRequestConverter func(*schemas.RakshaImageEditRequest) (providerUtils.RequestBodyWithExtraParams, error),
	postRequestConverter func(*OpenAIImageEditRequest) *OpenAIImageEditRequest,
	postResponseConverter func(*schemas.RakshaImageGenerationStreamResponse) *schemas.RakshaImageGenerationStreamResponse,
	logger schemas.Logger,
	postHookSpanFinalizer func(context.Context),
) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	providerUtils.SetStreamIdleTimeoutIfEmpty(ctx, streamIdleTimeoutInSeconds)
	reqBody := ToOpenAIImageEditRequest(request)
	if reqBody == nil {
		return nil, providerUtils.NewRakshaOperationError("image edit input is not provided", nil)
	}

	reqBody.Stream = schemas.Ptr(true)
	if postRequestConverter != nil {
		reqBody = postRequestConverter(reqBody)
	}
	// Create multipart form
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if rakshaErr := parseImageEditFormDataBodyFromRequest(writer, reqBody, providerName); rakshaErr != nil {
		return nil, rakshaErr
	}

	// Prepare OpenAI headers
	headers := map[string]string{
		"Content-Type":  writer.FormDataContentType(),
		"Accept":        "text/event-stream",
		"Cache-Control": "no-cache",
	}

	if authHeader != nil {
		maps.Copy(headers, authHeader)
	}

	// Create HTTP request for streaming
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	resp.StreamBody = true
	defer fasthttp.ReleaseRequest(req)

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)

	req.Header.SetMethod(http.MethodPost)
	req.SetRequestURI(url)

	// Set headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	req.SetBody(body.Bytes())

	startTime := time.Now()
	// Make the request
	err := client.Do(req, resp)
	latency := time.Since(startTime)
	if err != nil {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		if errors.Is(err, context.Canceled) {
			return nil, providerUtils.SetErrorLatency(&schemas.RakshaError{
				IsRakshaError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   err,
				},
			}, latency)
		}
		if errors.Is(err, fasthttp.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
			return nil, providerUtils.SetErrorLatency(providerUtils.NewRakshaTimeoutError(schemas.ErrProviderRequestTimedOut, err), latency)
		}
		return nil, providerUtils.SetErrorLatency(providerUtils.NewRakshaOperationError(schemas.ErrProviderDoRequest, err), latency)
	}
	// Store provider response headers in context before status check so error responses also forward them
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	// Check for HTTP errors
	if resp.StatusCode() != fasthttp.StatusOK {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		return nil, providerUtils.EnrichError(ctx, ParseOpenAIError(resp), nil, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Large payload streaming passthrough — pipe raw upstream SSE to client
	if providerUtils.SetupStreamingPassthrough(ctx, resp) {
		responseChan := make(chan *schemas.RakshaStreamChunk)
		providerUtils.CloseStream(ctx, responseChan)
		return responseChan, nil
	}

	// Create response channel
	responseChan := make(chan *schemas.RakshaStreamChunk, schemas.DefaultStreamBufferSize)

	// Start streaming in a goroutine
	go func() {
		defer providerUtils.EnsureStreamFinalizerCalled(ctx, postHookSpanFinalizer)
		defer func() {
			if ctx.Err() == context.Canceled {
				providerUtils.HandleStreamCancellation(ctx, postHookRunner, responseChan, logger, postHookSpanFinalizer, nil)
			} else if ctx.Err() == context.DeadlineExceeded {
				providerUtils.HandleStreamTimeout(ctx, postHookRunner, responseChan, logger, postHookSpanFinalizer, nil)
			}
			providerUtils.CloseStream(ctx, responseChan)
		}()
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		// Decompress gzip-encoded streams transparently (no-op for non-gzip)
		reader, releaseGzip := providerUtils.DecompressStreamBody(resp)
		defer releaseGzip()

		// Wrap reader with idle timeout to detect stalled streams.
		reader, stopIdleTimeout := providerUtils.NewIdleTimeoutReader(reader, resp.BodyStream(), providerUtils.GetStreamIdleTimeout(ctx), ctx)
		defer stopIdleTimeout()

		// Setup cancellation handler to close the raw network stream on ctx cancellation,
		// which immediately unblocks any in-progress read (including reads blocked inside a gzip decompression layer).
		stopCancellation := providerUtils.SetupStreamCancellation(ctx, resp.BodyStream(), logger)
		defer stopCancellation()

		// Skip scanner for non-SSE responses — avoids bufio.Scanner buffer bloat
		// on non-line-delimited data (e.g. provider returned JSON instead of SSE).
		reader, drained := providerUtils.DrainNonSSEStreamReader(resp, reader)
		if drained {
			ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
			providerUtils.ProcessAndSendError(ctx, postHookRunner, errors.New("provider returned non-SSE response for streaming request"), responseChan, logger, postHookSpanFinalizer)
			return
		}

		sseReader := providerUtils.GetSSEDataReader(ctx, reader)

		lastChunkTime := startTime
		// Track chunk indices per image - similar to how speech/transcription track chunkIndex
		imageChunkIndices := make(map[int]int) // image index -> chunk index
		// Track images that have started (via partial chunks) but not yet completed
		// This allows us to correctly match completed events to images even if chunks are interleaved
		incompleteImages := make(map[int]bool)
		maxImageIndex := -1 // Track maximum image index for NImages calculation

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			data, readErr := sseReader.ReadDataLine()
			if readErr != nil {
				if ctx.Err() != nil {
					return
				}
				if readErr != io.EOF {
					logger.Warn(fmt.Sprintf("Error reading stream: %v", readErr))
					providerUtils.ProcessAndSendError(ctx, postHookRunner, readErr, responseChan, logger, postHookSpanFinalizer)
				}
				break
			}
			jsonData := string(data)

			// Quick check for error field (allocation-free using sonic.GetFromString)
			if errorNode, _ := sonic.GetFromString(jsonData, "error"); errorNode.Exists() {
				// Only unmarshal when we know there's an error
				var rakshaErr schemas.RakshaError
				if err := sonic.UnmarshalString(jsonData, &rakshaErr); err == nil {
					if rakshaErr.Error != nil && rakshaErr.Error.Message != "" {
						ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
						providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, providerUtils.EnrichError(ctx, &rakshaErr, nil, nil, sendBackRawRequest, sendBackRawResponse, latency), responseChan, logger, postHookSpanFinalizer)
						return
					}
				}
			}

			// Parse minimally to extract usage and check for errors
			var response OpenAIImageStreamResponse
			if err := sonic.UnmarshalString(jsonData, &response); err != nil {
				logger.Warn("Failed to parse stream response: %v", err)
				continue
			}

			// Check if response type indicates an error
			if response.Type == "error" {
				rakshaErr := &schemas.RakshaError{
					IsRakshaError: false,
					Error:          &schemas.ErrorField{},
				}
				// Guard access to response.Error fields
				if response.Error != nil {
					rakshaErr.Error.Message = response.Error.Message
					if response.Error.Code != nil {
						rakshaErr.Error.Code = response.Error.Code
					}
					if response.Error.Param != nil {
						rakshaErr.Error.Param = response.Error.Param
					}
					if response.Error.Type != nil {
						rakshaErr.Error.Type = response.Error.Type
					}
				}
				ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
				providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, rakshaErr, responseChan, logger, postHookSpanFinalizer)
				return
			}

			// Determine if this is the final chunk
			isCompleted := response.Type == schemas.ImageGenerationEventTypeCompleted || response.Type == schemas.ImageEditEventTypeCompleted

			// Determine image index with robust tracking for interleaved chunks
			// Both partial and completed chunks should use PartialImageIndex when available
			var imageIndex int
			if response.PartialImageIndex != nil {
				// Use explicit index from response
				imageIndex = *response.PartialImageIndex
				if isCompleted {
					// Mark this image as completed
					delete(incompleteImages, imageIndex)
				} else {
					// Mark this image as started (incomplete)
					incompleteImages[imageIndex] = true
				}
			} else {
				// Fallback: PartialImageIndex is nil, use tracked state
				if isCompleted {
					// For completed chunks, match to the oldest incomplete image
					// This handles interleaved chunks correctly
					if len(incompleteImages) == 0 {
						// Fallback: if no incomplete images tracked, this shouldn't happen in normal flow
						// but we'll default to 0 to prevent panics
						imageIndex = 0
						logger.Warn("Received completed event but no incomplete images tracked, defaulting to index 0")
					} else {
						// Find the minimum (oldest) incomplete image index
						// Completed events should match the oldest image that was started
						minIndex := -1
						for idx := range incompleteImages {
							if minIndex == -1 || idx < minIndex {
								minIndex = idx
							}
						}
						imageIndex = minIndex
						// Mark this image as completed
						delete(incompleteImages, imageIndex)
						logger.Warn(fmt.Sprintf("Completed event missing PartialImageIndex, using oldest incomplete image index %d", imageIndex))
					}
				} else {
					// For partial chunks without PartialImageIndex, allocate a new unique index
					// Use maxImageIndex + 1 to ensure uniqueness
					imageIndex = maxImageIndex + 1
					// Mark this image as started (incomplete)
					incompleteImages[imageIndex] = true
				}
			}

			// Update maximum image index for NImages calculation
			if imageIndex > maxImageIndex {
				maxImageIndex = imageIndex
			}

			// Increment chunk index for this image
			if _, exists := imageChunkIndices[imageIndex]; !exists {
				imageChunkIndices[imageIndex] = 0
			} else {
				imageChunkIndices[imageIndex]++
			}
			chunkIndex := imageChunkIndices[imageIndex]
			// Build chunk with all OpenAI fields
			chunk := &schemas.RakshaImageGenerationStreamResponse{
				Type:         response.Type,
				Index:        imageIndex, // Which image (0-N)
				ChunkIndex:   chunkIndex, // Chunk order within this image (top-level)
				CreatedAt:    response.CreatedAt,
				Size:         response.Size,
				Quality:      response.Quality,
				Background:   response.Background,
				OutputFormat: response.OutputFormat,
				ExtraFields: schemas.RakshaResponseExtraFields{
					ChunkIndex: chunkIndex, // Chunk order within this image
					Latency:    time.Since(lastChunkTime).Milliseconds(),
				},
			}

			if postResponseConverter != nil {
				if converted := postResponseConverter(chunk); converted != nil {
					chunk = converted
				} else {
					logger.Warn("postResponseConverter returned nil; leaving chunk unmodified")
				}
			}

			// Only set PartialImageIndex for partial images, not for completed events
			if !isCompleted {
				chunk.PartialImageIndex = response.PartialImageIndex
			}
			// Set SequenceNumber if present
			if response.SequenceNumber != nil {
				chunk.SequenceNumber = *response.SequenceNumber
			}
			lastChunkTime = time.Now()

			// Copy b64_json if present
			if response.B64JSON != nil {
				chunk.B64JSON = *response.B64JSON
			}

			// Set raw response on every chunk if enabled
			if sendBackRawResponse {
				chunk.ExtraFields.RawResponse = jsonData
			}

			if isCompleted {
				if response.Usage != nil && maxImageIndex >= 0 {
					if response.Usage.OutputTokensDetails == nil {
						response.Usage.OutputTokensDetails = &schemas.ImageTokenDetails{}
					}
					if response.Usage.OutputTokensDetails.NImages == 0 {
						response.Usage.OutputTokensDetails.NImages = maxImageIndex + 1
					}
				}
				chunk.Usage = response.Usage
				// For completed chunk, use total latency from start
				chunk.ExtraFields.Latency = time.Since(startTime).Milliseconds()
				chunk.BackfillParams(&schemas.RakshaRequest{
					ImageEditRequest: request,
				})
				ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
			}

			providerUtils.ProcessAndSendResponse(ctx, postHookRunner,
				providerUtils.GetRakshaResponseForStreamResponse(nil, nil, nil, nil, nil, chunk),
				responseChan, postHookSpanFinalizer)

			if isCompleted {
				return
			}
		}
	}()

	return responseChan, nil
}

// ImageVariation performs an image variation request to openai's images api.
func (provider *OpenAIProvider) ImageVariation(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageVariationRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.ImageVariationRequest); err != nil {
		return nil, err
	}

	response, err := HandleOpenAIImageVariationRequest(
		ctx,
		provider.client,
		provider.buildRequestURL(ctx, "/v1/images/variations", schemas.ImageVariationRequest),
		request,
		key,
		provider.networkConfig.ExtraHeaders,
		false,
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		provider.logger,
	)
	return response, err
}

// ImageVariation performs an image variation request
// HandleOpenAIImageVariationRequest handles image variation requests for OpenAI-compatible providers
func HandleOpenAIImageVariationRequest(
	ctx *schemas.RakshaContext,
	client *fasthttp.Client,
	url string,
	request *schemas.RakshaImageVariationRequest,
	key schemas.Key,
	extraHeaders map[string]string,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
	providerName schemas.ModelProvider,
	logger schemas.Logger,
) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	// Large payload passthrough: stream multipart body directly without parsing
	if lpResult, lpErr, handled := handleOpenAILargePayloadPassthrough(ctx, client, url, BearerAuthHeader(key), extraHeaders, providerName, logger); handled {
		if lpErr != nil {
			return nil, lpErr
		}
		if len(lpResult.ResponseBody) > 0 {
			response := &schemas.RakshaImageGenerationResponse{}
			if err := sonic.Unmarshal(lpResult.ResponseBody, response); err != nil {
				return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseUnmarshal, err)
			}
			response.ExtraFields = schemas.RakshaResponseExtraFields{Latency: lpResult.Latency}
			return response, nil
		}
		return &schemas.RakshaImageGenerationResponse{
			ExtraFields: schemas.RakshaResponseExtraFields{Latency: lpResult.Latency},
		}, nil
	}

	openaiReq := ToOpenAIImageVariationRequest(request)
	if openaiReq == nil {
		return nil, providerUtils.NewRakshaOperationError("failed to convert request to OpenAI format", nil)
	}

	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	// resp lifecycle: managed by finalizeOpenAIResponse or released on error paths
	respOwned := true
	defer func() {
		if respOwned {
			fasthttp.ReleaseResponse(resp)
		}
	}()
	activeClient := providerUtils.PrepareResponseStreaming(ctx, client, resp)

	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)
	req.SetRequestURI(url)
	req.Header.SetMethod(http.MethodPost)

	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	// Create multipart form
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := parseImageVariationFormDataBodyFromRequest(writer, openaiReq, providerName); err != nil {
		return nil, err
	}

	req.Header.SetContentType(writer.FormDataContentType())
	bodyData := body.Bytes()
	req.SetBody(bodyData)

	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, nil, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}
	// Extract provider response headers early so they're available on error paths too
	providerResponseHeaders := providerUtils.ExtractProviderResponseHeaders(resp)
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)

	if resp.StatusCode() != fasthttp.StatusOK {
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		return nil, providerUtils.EnrichError(ctx, ParseOpenAIError(resp), nil, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	bodyBytes, lpResult, finalErr := finalizeOpenAIResponse(ctx, resp, latency, providerName, logger)
	respOwned = false // ownership transferred
	if finalErr != nil {
		return nil, finalErr
	}
	if lpResult != nil {
		return &schemas.RakshaImageGenerationResponse{
			ExtraFields: schemas.RakshaResponseExtraFields{Latency: lpResult.Latency},
		}, nil
	}

	response := &schemas.RakshaImageGenerationResponse{}
	_, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(bodyBytes, response, nil, false, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, rakshaErr
	}
	response.ExtraFields.Latency = latency.Milliseconds()
	response.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	// Set raw response if enabled
	if sendBackRawResponse {
		response.ExtraFields.RawResponse = rawResponse
	}
	return response, nil
}

// FileUpload uploads a file to OpenAI.
func (provider *OpenAIProvider) FileUpload(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaFileUploadRequest) (*schemas.RakshaFileUploadResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.FileUploadRequest); err != nil {
		return nil, err
	}

	if len(request.File) == 0 {
		return nil, providerUtils.NewRakshaOperationError("file content is required", nil)
	}

	if request.Purpose == "" {
		return nil, providerUtils.NewRakshaOperationError("purpose is required", nil)
	}

	// Create multipart form data
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add purpose field
	if err := writer.WriteField("purpose", string(request.Purpose)); err != nil {
		return nil, providerUtils.NewRakshaOperationError("failed to write purpose field", err)
	}

	// Add expires_after fields if provided
	if request.ExpiresAfter != nil {
		if err := writer.WriteField("expires_after[anchor]", request.ExpiresAfter.Anchor); err != nil {
			return nil, providerUtils.NewRakshaOperationError("failed to write expires_after[anchor] field", err)
		}
		if err := writer.WriteField("expires_after[seconds]", fmt.Sprintf("%d", request.ExpiresAfter.Seconds)); err != nil {
			return nil, providerUtils.NewRakshaOperationError("failed to write expires_after[seconds] field", err)
		}
	}

	// Add file field
	filename := request.Filename
	if filename == "" {
		filename = "file.jsonl"
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError("failed to create form file", err)
	}
	if _, err := part.Write(request.File); err != nil {
		return nil, providerUtils.NewRakshaOperationError("failed to write file content", err)
	}

	if err := writer.Close(); err != nil {
		return nil, providerUtils.NewRakshaOperationError("failed to close multipart writer", err)
	}

	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Set headers
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.SetRequestURI(provider.buildRequestURL(ctx, "/v1/files", schemas.FileUploadRequest))
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType(writer.FormDataContentType())

	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	req.SetBody(buf.Bytes())

	// Make request
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		provider.logger.Debug("error from %s provider: %s", provider.GetProviderKey(), string(resp.Body()))
		return nil, providerUtils.SetErrorLatency(ParseOpenAIError(resp), latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err)
	}

	var openAIResp OpenAIFileResponse
	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)
	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(body, &openAIResp, nil, sendBackRawRequest, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	fileResponse := openAIResp.ToRakshaFileUploadResponse(latency, sendBackRawRequest, sendBackRawResponse, rawRequest, rawResponse)
	fileResponse.ExtraFields.ProviderResponseHeaders = providerUtils.ExtractProviderResponseHeaders(resp)
	return fileResponse, nil
}

// FileList lists files using serial pagination across keys.
// Exhausts all pages from one key before moving to the next.
func (provider *OpenAIProvider) FileList(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaFileListRequest) (*schemas.RakshaFileListResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.FileListRequest); err != nil {
		return nil, err
	}

	providerName := provider.GetProviderKey()
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)
	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)

	// Initialize serial pagination helper
	helper, err := providerUtils.NewSerialListHelper(keys, request.After, provider.logger, true)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError("invalid pagination cursor", err)
	}

	// Get current key to query
	key, nativeCursor, ok := helper.GetCurrentKey()
	if !ok {
		// All keys exhausted
		return &schemas.RakshaFileListResponse{
			Object:  "list",
			Data:    []schemas.FileObject{},
			HasMore: false,
		}, nil
	}

	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Build URL with query params
	requestURL := provider.buildRequestURL(ctx, "/v1/files", schemas.FileListRequest)
	values := url.Values{}
	if request.Purpose != "" {
		values.Set("purpose", string(request.Purpose))
	}
	if request.Limit > 0 {
		values.Set("limit", fmt.Sprintf("%d", request.Limit))
	}
	// Use native cursor from serial helper instead of request.After
	if nativeCursor != "" {
		values.Set("after", nativeCursor)
	}
	if request.Order != nil && *request.Order != "" {
		values.Set("order", *request.Order)
	}
	if encoded := values.Encode(); encoded != "" {
		requestURL += "?" + encoded
	}

	// Set headers
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.SetRequestURI(requestURL)
	req.Header.SetMethod(http.MethodGet)
	req.Header.SetContentType("application/json")

	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	// Make request
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		provider.logger.Debug("error from %s provider: %s", providerName, string(resp.Body()))
		return nil, providerUtils.SetErrorLatency(ParseOpenAIError(resp), latency)
	}

	body, decodeErr := providerUtils.CheckAndDecodeBody(resp)
	if decodeErr != nil {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, decodeErr)
	}

	var openAIResp OpenAIFileListResponse
	_, _, rakshaErr = providerUtils.HandleProviderResponse(body, &openAIResp, nil, sendBackRawRequest, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Convert files to Raksha format
	files := make([]schemas.FileObject, 0, len(openAIResp.Data))
	var lastFileID string
	for _, file := range openAIResp.Data {
		files = append(files, schemas.FileObject{
			ID:            file.ID,
			Object:        file.Object,
			Bytes:         file.Bytes,
			CreatedAt:     file.CreatedAt,
			Filename:      file.Filename,
			Purpose:       schemas.FilePurpose(file.Purpose),
			Status:        ToRakshaFileStatus(file.Status),
			StatusDetails: file.StatusDetails,
		})
		lastFileID = file.ID
	}

	// Build cursor for next request
	// OpenAI uses LastID as the cursor for pagination
	nextCursor, hasMore := helper.BuildNextCursor(openAIResp.HasMore, lastFileID)

	// Convert to Raksha response
	rakshaResp := &schemas.RakshaFileListResponse{
		Object:  "list",
		Data:    files,
		HasMore: hasMore,
		ExtraFields: schemas.RakshaResponseExtraFields{
			Latency:                 latency.Milliseconds(),
			ProviderResponseHeaders: providerUtils.ExtractProviderResponseHeaders(resp),
		},
	}
	if nextCursor != "" {
		rakshaResp.After = &nextCursor
	}

	return rakshaResp, nil
}

// FileRetrieve retrieves file metadata from OpenAI by trying each key until found.
func (provider *OpenAIProvider) FileRetrieve(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaFileRetrieveRequest) (*schemas.RakshaFileRetrieveResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.FileRetrieveRequest); err != nil {
		return nil, err
	}

	providerName := provider.GetProviderKey()

	if request.FileID == "" {
		return nil, providerUtils.NewRakshaOperationError("file_id is required", nil)
	}

	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)

	var lastErr *schemas.RakshaError
	for _, key := range keys {
		// Create request
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()

		// Set headers
		providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
		req.SetRequestURI(provider.networkConfig.BaseURL + "/v1/files/" + request.FileID)
		req.Header.SetMethod(http.MethodGet)
		req.Header.SetContentType("application/json")

		if key.Value.GetValue() != "" {
			req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
		}

		// Make request
		latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
		wait()
		if rakshaErr != nil {
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			lastErr = rakshaErr
			continue
		}

		// Handle error response
		if resp.StatusCode() != fasthttp.StatusOK {
			provider.logger.Debug("error from %s provider: %s", providerName, string(resp.Body()))
			lastErr = ParseOpenAIError(resp)
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			continue
		}

		body, err := providerUtils.CheckAndDecodeBody(resp)
		if err != nil {
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			lastErr = providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err)
			continue
		}

		var openAIResp OpenAIFileResponse
		rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(body, &openAIResp, nil, sendBackRawRequest, sendBackRawResponse)
		if rakshaErr != nil {
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			lastErr = rakshaErr
			continue
		}

		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(resp)

		return openAIResp.ToRakshaFileRetrieveResponse(providerName, latency, sendBackRawRequest, sendBackRawResponse, rawRequest, rawResponse), nil
	}

	return nil, lastErr
}

// FileDelete deletes a file from OpenAI by trying each key until successful.
func (provider *OpenAIProvider) FileDelete(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaFileDeleteRequest) (*schemas.RakshaFileDeleteResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.FileDeleteRequest); err != nil {
		return nil, err
	}

	providerName := provider.GetProviderKey()

	if request.FileID == "" {
		return nil, providerUtils.NewRakshaOperationError("file_id is required", nil)
	}

	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)

	var lastErr *schemas.RakshaError
	for _, key := range keys {
		// Create request
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()

		// Set headers
		providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
		req.SetRequestURI(provider.networkConfig.BaseURL + "/v1/files/" + request.FileID)
		req.Header.SetMethod(http.MethodDelete)
		req.Header.SetContentType("application/json")

		if key.Value.GetValue() != "" {
			req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
		}

		// Make request
		latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
		wait()
		if rakshaErr != nil {
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			lastErr = rakshaErr
			continue
		}

		// Handle error response
		if resp.StatusCode() != fasthttp.StatusOK {
			provider.logger.Debug("error from %s provider: %s", providerName, string(resp.Body()))
			lastErr = ParseOpenAIError(resp)
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			continue
		}

		body, err := providerUtils.CheckAndDecodeBody(resp)
		if err != nil {
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			lastErr = providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err)
			continue
		}

		var openAIResp OpenAIFileDeleteResponse
		rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(body, &openAIResp, nil, sendBackRawRequest, sendBackRawResponse)
		if rakshaErr != nil {
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			lastErr = rakshaErr
			continue
		}

		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(resp)

		result := &schemas.RakshaFileDeleteResponse{
			ID:      openAIResp.ID,
			Object:  openAIResp.Object,
			Deleted: openAIResp.Deleted,
			ExtraFields: schemas.RakshaResponseExtraFields{
				Latency: latency.Milliseconds(),
			},
		}

		if sendBackRawRequest {
			result.ExtraFields.RawRequest = rawRequest
		}

		if sendBackRawResponse {
			result.ExtraFields.RawResponse = rawResponse
		}

		return result, nil
	}

	return nil, lastErr
}

// FileContent downloads file content from OpenAI by trying each key until found.
func (provider *OpenAIProvider) FileContent(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaFileContentRequest) (*schemas.RakshaFileContentResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.FileContentRequest); err != nil {
		return nil, err
	}

	providerName := provider.GetProviderKey()

	if request.FileID == "" {
		return nil, providerUtils.NewRakshaOperationError("file_id is required", nil)
	}

	var lastErr *schemas.RakshaError
	for _, key := range keys {
		// Create request
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()

		// Set headers
		providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
		req.SetRequestURI(provider.networkConfig.BaseURL + "/v1/files/" + request.FileID + "/content")
		req.Header.SetMethod(http.MethodGet)

		if key.Value.GetValue() != "" {
			req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
		}

		// Make request
		latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
		wait()
		if rakshaErr != nil {
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			lastErr = rakshaErr
			continue
		}

		// Handle error response
		if resp.StatusCode() != fasthttp.StatusOK {
			provider.logger.Debug("error from %s provider: %s", providerName, string(resp.Body()))
			lastErr = ParseOpenAIError(resp)
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			continue
		}

		body, err := providerUtils.CheckAndDecodeBody(resp)
		if err != nil {
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			lastErr = providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err)
			continue
		}

		// Get content type from response
		contentType := string(resp.Header.ContentType())
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		content := append([]byte(nil), body...)

		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(resp)

		return &schemas.RakshaFileContentResponse{
			FileID:      request.FileID,
			Content:     content,
			ContentType: contentType,
			ExtraFields: schemas.RakshaResponseExtraFields{
				Latency: latency.Milliseconds(),
			},
		}, nil
	}

	return nil, lastErr
}

// VideoRemix remixes an existing video from the OpenAI provider.
func (provider *OpenAIProvider) VideoRemix(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaVideoRemixRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.VideoRemixRequest); err != nil {
		return nil, err
	}

	providerName := provider.GetProviderKey()

	if request.ID == "" {
		return nil, providerUtils.NewRakshaOperationError("video_id is required", nil)
	}
	if request.Input == nil || request.Input.Prompt == "" {
		return nil, providerUtils.NewRakshaOperationError("prompt is required", nil)
	}

	jsonData, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToOpenAIVideoRemixRequest(request)
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	videoID := providerUtils.StripVideoIDProviderSuffix(request.ID, providerName)

	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)
	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)

	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Set headers
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.SetRequestURI(provider.buildRequestURL(ctx, "/v1/videos/"+videoID+"/remix", schemas.VideoRemixRequest))
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")

	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	req.SetBody(jsonData)

	// Make request
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		provider.logger.Debug("error from %s provider: %s", providerName, string(resp.Body()))
		return nil, providerUtils.SetErrorLatency(ParseOpenAIError(resp), latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err), jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Parse OpenAI's video response
	response := &schemas.RakshaVideoGenerationResponse{}
	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(body, response, jsonData, sendBackRawRequest, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, rakshaErr
	}
	if response.ID != "" {
		response.ID = providerUtils.AddVideoIDProviderSuffix(response.ID, providerName)
	}
	if response.RemixedFromVideoID != nil && *response.RemixedFromVideoID != "" {
		remixID := providerUtils.AddVideoIDProviderSuffix(*response.RemixedFromVideoID, providerName)
		response.RemixedFromVideoID = &remixID
	}

	response.ExtraFields = schemas.RakshaResponseExtraFields{
		Latency: latency.Milliseconds(),
	}

	if sendBackRawResponse {
		response.ExtraFields.RawResponse = rawResponse
	}
	if sendBackRawRequest {
		response.ExtraFields.RawRequest = rawRequest
	}

	return response, nil
}

// BatchCreate creates a new batch job.
func (provider *OpenAIProvider) BatchCreate(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaBatchCreateRequest) (*schemas.RakshaBatchCreateResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.BatchCreateRequest); err != nil {
		return nil, err
	}

	inputFileID := request.InputFileID

	// If no file_id provided but inline requests are available, upload them first
	if inputFileID == "" && len(request.Requests) > 0 {
		// Convert inline requests to JSONL format
		jsonlData, err := ConvertRequestsToJSONL(request.Requests)
		if err != nil {
			return nil, providerUtils.NewRakshaOperationError("failed to convert requests to JSONL", err)
		}

		// Upload the file with purpose "batch"
		uploadResp, rakshaErr := provider.FileUpload(ctx, key, &schemas.RakshaFileUploadRequest{
			Provider: schemas.OpenAI,
			File:     jsonlData,
			Filename: "batch_requests.jsonl",
			Purpose:  "batch",
		})
		if rakshaErr != nil {
			return nil, rakshaErr
		}

		inputFileID = uploadResp.ID
	}

	// Validate that we have a file ID (either provided or uploaded)
	if inputFileID == "" {
		return nil, providerUtils.NewRakshaOperationError("either input_file_id or requests array is required for OpenAI batch API", nil)
	}

	// Validate that we have an endpoint
	if request.Endpoint == "" {
		return nil, providerUtils.NewRakshaOperationError("endpoint is required for OpenAI batch API", nil)
	}

	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Set headers
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.SetRequestURI(provider.buildRequestURL(ctx, "/v1/batches", schemas.BatchCreateRequest))
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")

	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	// Build request body
	openAIReq := &OpenAIBatchRequest{
		InputFileID:        schemas.Ptr(inputFileID),
		Endpoint:           string(request.Endpoint),
		CompletionWindow:   request.CompletionWindow,
		Metadata:           request.Metadata,
		OutputExpiresAfter: request.OutputExpiresAfter,
	}

	// Set default completion window if not provided
	if openAIReq.CompletionWindow == "" {
		openAIReq.CompletionWindow = "24h"
	}

	jsonData, err := providerUtils.MarshalSorted(openAIReq)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderRequestMarshal, err)
	}
	req.SetBody(jsonData)

	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)

	// Make request
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.EnrichError(ctx, ParseOpenAIError(resp), jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err), jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	var openAIResp OpenAIBatchResponse
	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(body, &openAIResp, jsonData, sendBackRawRequest, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, body, sendBackRawRequest, sendBackRawResponse, latency)
	}

	return openAIResp.ToRakshaBatchCreateResponse(latency, sendBackRawRequest, sendBackRawResponse, rawRequest, rawResponse), nil
}

// BatchList lists batch jobs using serial pagination across keys.
// Exhausts all pages from one key before moving to the next.
func (provider *OpenAIProvider) BatchList(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaBatchListRequest) (*schemas.RakshaBatchListResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.BatchListRequest); err != nil {
		return nil, err
	}

	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)

	// Initialize serial pagination helper
	helper, err := providerUtils.NewSerialListHelper(keys, request.After, provider.logger, true)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError("invalid pagination cursor", err)
	}

	// Get current key to query
	key, nativeCursor, ok := helper.GetCurrentKey()
	if !ok {
		// All keys exhausted
		return &schemas.RakshaBatchListResponse{
			Object:  "list",
			Data:    []schemas.RakshaBatchRetrieveResponse{},
			HasMore: false,
		}, nil
	}

	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Build URL with query params
	baseURL := provider.buildRequestURL(ctx, "/v1/batches", schemas.BatchListRequest)
	values := url.Values{}
	if request.Limit > 0 {
		values.Set("limit", fmt.Sprintf("%d", request.Limit))
	}
	// Use native cursor from serial helper instead of request.After
	if nativeCursor != "" {
		values.Set("after", nativeCursor)
	}
	requestURL := baseURL
	if encodedValues := values.Encode(); encodedValues != "" {
		requestURL += "?" + encodedValues
	}

	// Set headers
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.SetRequestURI(requestURL)
	req.Header.SetMethod(http.MethodGet)
	req.Header.SetContentType("application/json")

	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	// Make request
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.SetErrorLatency(ParseOpenAIError(resp), latency)
	}

	body, decodeErr := providerUtils.CheckAndDecodeBody(resp)
	if decodeErr != nil {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, decodeErr)
	}

	var openAIResp OpenAIBatchListResponse
	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(body, &openAIResp, nil, sendBackRawRequest, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Convert batches to Raksha format
	batches := make([]schemas.RakshaBatchRetrieveResponse, 0, len(openAIResp.Data))
	var lastBatchID string
	for _, batch := range openAIResp.Data {
		batches = append(batches, *batch.ToRakshaBatchRetrieveResponse(latency, sendBackRawRequest, sendBackRawResponse, rawRequest, rawResponse))
		lastBatchID = batch.ID
	}

	// Build cursor for next request
	// OpenAI uses LastID as the cursor for pagination
	nextCursor, hasMore := helper.BuildNextCursor(openAIResp.HasMore, lastBatchID)

	// Convert to Raksha response
	rakshaResp := &schemas.RakshaBatchListResponse{
		Object:  "list",
		Data:    batches,
		HasMore: hasMore,
		ExtraFields: schemas.RakshaResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}
	if nextCursor != "" {
		rakshaResp.NextCursor = &nextCursor
	}

	return rakshaResp, nil
}

// BatchRetrieve retrieves a specific batch job by trying each key until found.
func (provider *OpenAIProvider) BatchRetrieve(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaBatchRetrieveRequest) (*schemas.RakshaBatchRetrieveResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.BatchRetrieveRequest); err != nil {
		return nil, err
	}

	if request.BatchID == "" {
		return nil, providerUtils.NewRakshaOperationError("batch_id is required", nil)
	}

	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)

	var lastErr *schemas.RakshaError
	for _, key := range keys {
		// Create request
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()

		// Set headers
		providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
		req.SetRequestURI(provider.networkConfig.BaseURL + "/v1/batches/" + request.BatchID)
		req.Header.SetMethod(http.MethodGet)
		req.Header.SetContentType("application/json")

		if key.Value.GetValue() != "" {
			req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
		}

		// Make request
		latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
		wait()
		if rakshaErr != nil {
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			lastErr = rakshaErr
			continue
		}

		// Handle error response
		if resp.StatusCode() != fasthttp.StatusOK {
			lastErr = ParseOpenAIError(resp)
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			continue
		}

		body, err := providerUtils.CheckAndDecodeBody(resp)
		if err != nil {
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			lastErr = providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err)
			continue
		}

		var openAIResp OpenAIBatchResponse
		rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(body, &openAIResp, nil, sendBackRawRequest, sendBackRawResponse)
		if rakshaErr != nil {
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			lastErr = rakshaErr
			continue
		}

		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(resp)

		result := openAIResp.ToRakshaBatchRetrieveResponse(latency, sendBackRawRequest, sendBackRawResponse, rawRequest, rawResponse)
		return result, nil
	}

	return nil, lastErr
}

// BatchCancel cancels a batch job by trying each key until successful.
func (provider *OpenAIProvider) BatchCancel(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaBatchCancelRequest) (*schemas.RakshaBatchCancelResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.BatchCancelRequest); err != nil {
		return nil, err
	}

	if request.BatchID == "" {
		return nil, providerUtils.NewRakshaOperationError("batch_id is required", nil)
	}

	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)

	var lastErr *schemas.RakshaError
	for _, key := range keys {
		// Create request
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()

		// Set headers
		providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
		req.SetRequestURI(provider.networkConfig.BaseURL + "/v1/batches/" + request.BatchID + "/cancel")
		req.Header.SetMethod(http.MethodPost)
		req.Header.SetContentType("application/json")

		if key.Value.GetValue() != "" {
			req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
		}

		// Make request
		latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
		wait()
		if rakshaErr != nil {
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			lastErr = rakshaErr
			continue
		}

		// Handle error response
		if resp.StatusCode() != fasthttp.StatusOK {
			lastErr = ParseOpenAIError(resp)
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			continue
		}

		body, err := providerUtils.CheckAndDecodeBody(resp)
		if err != nil {
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			lastErr = providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err)
			continue
		}

		var openAIResp OpenAIBatchResponse
		rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(body, &openAIResp, nil, sendBackRawRequest, sendBackRawResponse)
		if rakshaErr != nil {
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			lastErr = rakshaErr
			continue
		}

		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(resp)

		result := &schemas.RakshaBatchCancelResponse{
			ID:           openAIResp.ID,
			Object:       openAIResp.Object,
			Status:       ToRakshaBatchStatus(openAIResp.Status),
			CancellingAt: openAIResp.CancellingAt,
			CancelledAt:  openAIResp.CancelledAt,
			ExtraFields: schemas.RakshaResponseExtraFields{
				Latency: latency.Milliseconds(),
			},
		}

		if openAIResp.RequestCounts != nil {
			result.RequestCounts = schemas.BatchRequestCounts{
				Total:     openAIResp.RequestCounts.Total,
				Completed: openAIResp.RequestCounts.Completed,
				Failed:    openAIResp.RequestCounts.Failed,
			}
		}

		if sendBackRawRequest {
			result.ExtraFields.RawRequest = rawRequest
		}

		if sendBackRawResponse {
			result.ExtraFields.RawResponse = rawResponse
		}

		return result, nil
	}

	return nil, lastErr
}

// BatchDelete is not supported by the OpenAI provider.
func (provider *OpenAIProvider) BatchDelete(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaBatchDeleteRequest) (*schemas.RakshaBatchDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchDeleteRequest, provider.GetProviderKey())
}

// BatchResults retrieves batch results by trying each key until successful.
// Note: For OpenAI, batch results are obtained by downloading the output_file_id.
// This method returns the file content parsed as batch results.
func (provider *OpenAIProvider) BatchResults(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaBatchResultsRequest) (*schemas.RakshaBatchResultsResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.BatchResultsRequest); err != nil {
		return nil, err
	}

	if request.BatchID == "" {
		return nil, providerUtils.NewRakshaOperationError("batch_id is required", nil)
	}

	// First, retrieve the batch to get the output_file_id (this already iterates over keys)
	batchResp, rakshaErr := provider.BatchRetrieve(ctx, keys, &schemas.RakshaBatchRetrieveRequest{
		Provider: request.Provider,
		BatchID:  request.BatchID,
	})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	if batchResp.OutputFileID == nil || *batchResp.OutputFileID == "" {
		return nil, providerUtils.NewRakshaOperationError("batch results not available: output_file_id is empty (batch may not be completed)", nil)
	}

	// Download the output file - try each key
	var lastErr *schemas.RakshaError
	for _, key := range keys {
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()

		// Set headers
		providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
		req.SetRequestURI(provider.networkConfig.BaseURL + "/v1/files/" + *batchResp.OutputFileID + "/content")
		req.Header.SetMethod(http.MethodGet)

		if key.Value.GetValue() != "" {
			req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
		}

		// Make request
		latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
		wait()
		if rakshaErr != nil {
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			lastErr = rakshaErr
			continue
		}

		// Handle error response
		if resp.StatusCode() != fasthttp.StatusOK {
			lastErr = ParseOpenAIError(resp)
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			continue
		}

		body, err := providerUtils.CheckAndDecodeBody(resp)
		if err != nil {
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			lastErr = providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err)
			continue
		}

		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(resp)

		// Parse JSONL content - each line is a separate result
		var results []schemas.BatchResultItem

		parseResult := providerUtils.ParseJSONL(body, func(line []byte) error {
			var resultItem schemas.BatchResultItem
			if err := sonic.Unmarshal(line, &resultItem); err != nil {
				provider.logger.Warn("failed to parse batch result line: %v", err)
				return err
			}
			results = append(results, resultItem)
			return nil
		})

		batchResultsResp := &schemas.RakshaBatchResultsResponse{
			BatchID: request.BatchID,
			Results: results,
			ExtraFields: schemas.RakshaResponseExtraFields{
				Latency: latency.Milliseconds(),
			},
		}

		if len(parseResult.Errors) > 0 {
			batchResultsResp.ExtraFields.ParseErrors = parseResult.Errors
		}

		return batchResultsResp, nil
	}

	return nil, lastErr
}

// ContainerCreate creates a new container via OpenAI's API.
func (provider *OpenAIProvider) ContainerCreate(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaContainerCreateRequest) (*schemas.RakshaContainerCreateResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.ContainerCreateRequest); err != nil {
		return nil, err
	}

	if request == nil {
		return nil, providerUtils.NewRakshaOperationError("invalid request: nil", nil)
	}

	if request.Name == "" {
		return nil, providerUtils.NewRakshaOperationError("invalid request: name is required", nil)
	}

	// Build request body
	reqBody := map[string]interface{}{
		"name": request.Name,
	}

	if request.ExpiresAfter != nil {
		reqBody["expires_after"] = map[string]interface{}{
			"anchor":  request.ExpiresAfter.Anchor,
			"minutes": request.ExpiresAfter.Minutes,
		}
	}

	if len(request.FileIDs) > 0 {
		reqBody["file_ids"] = request.FileIDs
	}

	if request.MemoryLimit != "" {
		reqBody["memory_limit"] = request.MemoryLimit
	}

	if len(request.Metadata) > 0 {
		reqBody["metadata"] = request.Metadata
	}

	// Merge ExtraParams into reqBody (do not overwrite mandatory keys)
	for k, v := range request.ExtraParams {
		if _, exists := reqBody[k]; !exists {
			reqBody[k] = v
		}
	}

	jsonBody, err := providerUtils.MarshalSorted(reqBody)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderRequestMarshal, err)
	}

	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	req.SetRequestURI(provider.buildRequestURL(ctx, "/v1/containers", schemas.ContainerCreateRequest))
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	req.SetBody(jsonBody)

	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	// Make request
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK && resp.StatusCode() != fasthttp.StatusCreated {
		return nil, providerUtils.SetErrorLatency(ParseOpenAIError(resp), latency)
	}

	// Parse response
	responseBody := append([]byte(nil), resp.Body()...)

	var containerResp struct {
		ID           string                         `json:"id"`
		Object       string                         `json:"object"`
		Name         string                         `json:"name"`
		CreatedAt    int64                          `json:"created_at"`
		Status       schemas.ContainerStatus        `json:"status"`
		ExpiresAfter *schemas.ContainerExpiresAfter `json:"expires_after"`
		LastActiveAt *int64                         `json:"last_active_at"`
		MemoryLimit  string                         `json:"memory_limit"`
		Metadata     map[string]string              `json:"metadata"`
	}

	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(responseBody, &containerResp, jsonBody, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	response := &schemas.RakshaContainerCreateResponse{
		ID:           containerResp.ID,
		Object:       containerResp.Object,
		Name:         containerResp.Name,
		CreatedAt:    containerResp.CreatedAt,
		Status:       containerResp.Status,
		ExpiresAfter: containerResp.ExpiresAfter,
		LastActiveAt: containerResp.LastActiveAt,
		MemoryLimit:  containerResp.MemoryLimit,
		Metadata:     containerResp.Metadata,
		ExtraFields: schemas.RakshaResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}

	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		response.ExtraFields.RawRequest = rawRequest
	}
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		response.ExtraFields.RawResponse = rawResponse
	}

	return response, nil
}

// ContainerList lists containers via OpenAI's API.
// Uses SerialListHelper for multi-key pagination - exhausts all pages from one key before moving to next.
func (provider *OpenAIProvider) ContainerList(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaContainerListRequest) (*schemas.RakshaContainerListResponse, *schemas.RakshaError) {
	if request == nil {
		return nil, providerUtils.NewRakshaOperationError("invalid request: nil", nil)
	}
	if len(keys) == 0 {
		if provider.customProviderConfig != nil && provider.customProviderConfig.IsKeyLess {
			keys = []schemas.Key{{}}
		} else {
			return nil, providerUtils.NewRakshaOperationError("provider config not found", nil)
		}
	}

	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.ContainerListRequest); err != nil {
		return nil, err
	}

	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)

	// Initialize serial pagination helper for multi-key support
	helper, err := providerUtils.NewSerialListHelper(keys, request.After, provider.logger, true)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError("invalid pagination cursor", err)
	}

	// Get current key to query
	key, nativeCursor, ok := helper.GetCurrentKey()
	if !ok {
		// All keys exhausted
		return &schemas.RakshaContainerListResponse{
			Object:  "list",
			Data:    []schemas.ContainerObject{},
			HasMore: false,
		}, nil
	}

	// Build query string
	queryParams := url.Values{}
	if request.Limit > 0 {
		queryParams.Set("limit", fmt.Sprintf("%d", request.Limit))
	}
	// Use native cursor from helper instead of request.After
	if nativeCursor != "" {
		queryParams.Set("after", nativeCursor)
	}
	if request.Order != nil {
		queryParams.Set("order", *request.Order)
	}

	requestURL := provider.buildRequestURL(ctx, "/v1/containers", schemas.ContainerListRequest)
	if len(queryParams) > 0 {
		requestURL += "?" + queryParams.Encode()
	}

	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	req.SetRequestURI(requestURL)
	req.Header.SetMethod(http.MethodGet)
	req.Header.SetContentType("application/json")

	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	// Make request
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.SetErrorLatency(ParseOpenAIError(resp), latency)
	}

	// Parse response
	responseBody := append([]byte(nil), resp.Body()...)

	var listResp struct {
		Object  string                    `json:"object"`
		Data    []schemas.ContainerObject `json:"data"`
		FirstID *string                   `json:"first_id"`
		LastID  *string                   `json:"last_id"`
		HasMore bool                      `json:"has_more"`
	}

	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(responseBody, &listResp, nil, sendBackRawRequest, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Track last container ID for pagination cursor
	var lastContainerID string
	for _, container := range listResp.Data {
		lastContainerID = container.ID
	}

	// Build cursor for next request (handles cross-key pagination)
	nextCursor, hasMore := helper.BuildNextCursor(listResp.HasMore, lastContainerID)

	response := &schemas.RakshaContainerListResponse{
		Object:  listResp.Object,
		Data:    listResp.Data,
		FirstID: listResp.FirstID,
		LastID:  listResp.LastID,
		HasMore: hasMore,
		ExtraFields: schemas.RakshaResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}

	// Set encoded cursor for next page
	if nextCursor != "" {
		response.After = &nextCursor
	}

	if sendBackRawRequest {
		response.ExtraFields.RawRequest = rawRequest
	}
	if sendBackRawResponse {
		response.ExtraFields.RawResponse = rawResponse
	}

	return response, nil
}

// ContainerRetrieve retrieves a specific container via OpenAI's API.
func (provider *OpenAIProvider) ContainerRetrieve(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaContainerRetrieveRequest) (*schemas.RakshaContainerRetrieveResponse, *schemas.RakshaError) {
	if request == nil {
		return nil, providerUtils.NewRakshaOperationError("invalid request: nil", nil)
	}
	if len(keys) == 0 {
		if provider.customProviderConfig != nil && provider.customProviderConfig.IsKeyLess {
			keys = []schemas.Key{{}}
		} else {
			return nil, providerUtils.NewRakshaOperationError("provider config not found", nil)
		}
	}
	if request.ContainerID == "" {
		return nil, providerUtils.NewRakshaOperationError("container_id is required", nil)
	}

	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.ContainerRetrieveRequest); err != nil {
		return nil, err
	}

	var lastErr *schemas.RakshaError
	for _, key := range keys {
		// Create request
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()

		providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

		req.SetRequestURI(provider.buildRequestURL(ctx, "/v1/containers/"+request.ContainerID, schemas.ContainerRetrieveRequest))
		req.Header.SetMethod(http.MethodGet)
		req.Header.SetContentType("application/json")

		if key.Value.GetValue() != "" {
			req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
		}

		// Make request
		latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
		wait()
		if rakshaErr != nil {
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			lastErr = rakshaErr
			continue
		}

		// Handle error response
		if resp.StatusCode() != fasthttp.StatusOK {
			lastErr = ParseOpenAIError(resp)
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			continue
		}

		// Parse response
		responseBody := append([]byte(nil), resp.Body()...)

		var containerResp struct {
			ID           string                         `json:"id"`
			Object       string                         `json:"object"`
			Name         string                         `json:"name"`
			CreatedAt    int64                          `json:"created_at"`
			Status       schemas.ContainerStatus        `json:"status"`
			ExpiresAfter *schemas.ContainerExpiresAfter `json:"expires_after"`
			LastActiveAt *int64                         `json:"last_active_at"`
			MemoryLimit  string                         `json:"memory_limit"`
			Metadata     map[string]string              `json:"metadata"`
		}

		rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(responseBody, &containerResp, nil, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
		if rakshaErr != nil {
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			lastErr = rakshaErr
			continue
		}

		response := &schemas.RakshaContainerRetrieveResponse{
			ID:           containerResp.ID,
			Object:       containerResp.Object,
			Name:         containerResp.Name,
			CreatedAt:    containerResp.CreatedAt,
			Status:       containerResp.Status,
			ExpiresAfter: containerResp.ExpiresAfter,
			LastActiveAt: containerResp.LastActiveAt,
			MemoryLimit:  containerResp.MemoryLimit,
			Metadata:     containerResp.Metadata,
			ExtraFields: schemas.RakshaResponseExtraFields{
				Latency: latency.Milliseconds(),
			},
		}

		if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
			response.ExtraFields.RawRequest = rawRequest
		}
		if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
			response.ExtraFields.RawResponse = rawResponse
		}

		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(resp)
		return response, nil
	}

	return nil, lastErr
}

// ContainerDelete deletes a container via OpenAI's API.
func (provider *OpenAIProvider) ContainerDelete(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaContainerDeleteRequest) (*schemas.RakshaContainerDeleteResponse, *schemas.RakshaError) {
	if request == nil {
		return nil, providerUtils.NewRakshaOperationError("invalid request: nil", nil)
	}
	if len(keys) == 0 {
		if provider.customProviderConfig != nil && provider.customProviderConfig.IsKeyLess {
			keys = []schemas.Key{{}}
		} else {
			return nil, providerUtils.NewRakshaOperationError("provider config not found", nil)
		}
	}
	if request.ContainerID == "" {
		return nil, providerUtils.NewRakshaOperationError("container_id is required", nil)
	}

	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.ContainerDeleteRequest); err != nil {
		return nil, err
	}

	var lastErr *schemas.RakshaError
	for _, key := range keys {
		// Create request
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()

		providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

		req.SetRequestURI(provider.buildRequestURL(ctx, "/v1/containers/"+request.ContainerID, schemas.ContainerDeleteRequest))
		req.Header.SetMethod(http.MethodDelete)
		req.Header.SetContentType("application/json")

		if key.Value.GetValue() != "" {
			req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
		}

		// Make request
		latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
		wait()
		if rakshaErr != nil {
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			lastErr = rakshaErr
			continue
		}

		// Handle error response
		if resp.StatusCode() != fasthttp.StatusOK {
			lastErr = ParseOpenAIError(resp)
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			continue
		}

		// Parse response
		responseBody := append([]byte(nil), resp.Body()...)

		var deleteResp struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Deleted bool   `json:"deleted"`
		}

		rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(responseBody, &deleteResp, nil, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
		if rakshaErr != nil {
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			lastErr = rakshaErr
			continue
		}

		response := &schemas.RakshaContainerDeleteResponse{
			ID:      deleteResp.ID,
			Object:  deleteResp.Object,
			Deleted: deleteResp.Deleted,
			ExtraFields: schemas.RakshaResponseExtraFields{
				Latency: latency.Milliseconds(),
			},
		}

		if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
			response.ExtraFields.RawRequest = rawRequest
		}
		if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
			response.ExtraFields.RawResponse = rawResponse
		}

		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(resp)
		return response, nil
	}

	return nil, lastErr
}

// =============================================================================
// CONTAINER FILES API
// =============================================================================

// ContainerFileCreate creates a file in a container via OpenAI's API.
func (provider *OpenAIProvider) ContainerFileCreate(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaContainerFileCreateRequest) (*schemas.RakshaContainerFileCreateResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.ContainerFileCreateRequest); err != nil {
		return nil, err
	}

	if request == nil {
		return nil, providerUtils.NewRakshaOperationError("invalid request: nil", nil)
	}

	if request.ContainerID == "" {
		return nil, providerUtils.NewRakshaOperationError("invalid request: container_id is required", nil)
	}

	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	endpoint := fmt.Sprintf("/v1/containers/%s/files", request.ContainerID)
	req.SetRequestURI(provider.buildRequestURL(ctx, endpoint, schemas.ContainerFileCreateRequest))
	req.Header.SetMethod(http.MethodPost)

	// Handle file upload (multipart only)
	if len(request.File) == 0 {
		return nil, providerUtils.NewRakshaOperationError("invalid request: file is required", nil)
	}

	// Multipart file upload
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Add file
	part, err := writer.CreateFormFile("file", "file")
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError("failed to create multipart form", err)
	}
	if _, err = part.Write(request.File); err != nil {
		return nil, providerUtils.NewRakshaOperationError("failed to write file to multipart form", err)
	}
	if err := writer.Close(); err != nil {
		return nil, providerUtils.NewRakshaOperationError("failed to close multipart form", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.SetBody(body.Bytes())

	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	// Make request
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Handle error response
	if resp.StatusCode() >= 400 {
		return nil, providerUtils.SetErrorLatency(ParseOpenAIError(resp), latency)
	}

	// Decode response body (handles content-encoding like gzip)
	responseBody, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err)
	}
	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)

	var fileResp struct {
		ID          string `json:"id"`
		Object      string `json:"object"`
		Bytes       int64  `json:"bytes"`
		CreatedAt   int64  `json:"created_at"`
		ContainerID string `json:"container_id"`
		Path        string `json:"path"`
		Source      string `json:"source"`
	}

	_, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(responseBody, &fileResp, nil, false, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	containerFileCreateResponse := &schemas.RakshaContainerFileCreateResponse{
		ID:          fileResp.ID,
		Object:      fileResp.Object,
		Bytes:       fileResp.Bytes,
		CreatedAt:   fileResp.CreatedAt,
		ContainerID: fileResp.ContainerID,
		Path:        fileResp.Path,
		Source:      fileResp.Source,
		ExtraFields: schemas.RakshaResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}

	// We don't capture payload for security reasons
	if sendBackRawRequest {
		containerFileCreateResponse.ExtraFields.RawRequest = "<REDACTED>"
	}
	if sendBackRawResponse {
		containerFileCreateResponse.ExtraFields.RawResponse = rawResponse
	}

	return containerFileCreateResponse, nil
}

// ContainerFileList lists files in a container via OpenAI's API.
// Uses SerialListHelper for multi-key pagination - exhausts all pages from one key before moving to next.
func (provider *OpenAIProvider) ContainerFileList(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaContainerFileListRequest) (*schemas.RakshaContainerFileListResponse, *schemas.RakshaError) {
	if request == nil {
		return nil, providerUtils.NewRakshaOperationError("invalid request: nil", nil)
	}

	if request.ContainerID == "" {
		return nil, providerUtils.NewRakshaOperationError("invalid request: container_id is required", nil)
	}

	if len(keys) == 0 {
		if provider.customProviderConfig != nil && provider.customProviderConfig.IsKeyLess {
			keys = []schemas.Key{{}}
		} else {
			return nil, providerUtils.NewRakshaOperationError("no keys provided", nil)
		}
	}

	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.ContainerFileListRequest); err != nil {
		return nil, err
	}

	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)

	// Initialize serial pagination helper for multi-key support
	helper, err := providerUtils.NewSerialListHelper(keys, request.After, provider.logger, true)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError("invalid pagination cursor", err)
	}

	// Get current key to query
	key, nativeCursor, ok := helper.GetCurrentKey()
	if !ok {
		// All keys exhausted
		return &schemas.RakshaContainerFileListResponse{
			Object:  "list",
			Data:    []schemas.ContainerFileObject{},
			HasMore: false,
		}, nil
	}

	// Build URL with query parameters
	endpoint := fmt.Sprintf("/v1/containers/%s/files", request.ContainerID)
	requestURL := provider.buildRequestURL(ctx, endpoint, schemas.ContainerFileListRequest)

	// Add query parameters
	queryParams := url.Values{}
	if request.Limit > 0 {
		queryParams.Set("limit", fmt.Sprintf("%d", request.Limit))
	}
	// Use native cursor from helper instead of request.After
	if nativeCursor != "" {
		queryParams.Set("after", nativeCursor)
	}
	if request.Order != nil {
		queryParams.Set("order", *request.Order)
	}
	if len(queryParams) > 0 {
		requestURL = requestURL + "?" + queryParams.Encode()
	}

	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	req.SetRequestURI(requestURL)
	req.Header.SetMethod(http.MethodGet)

	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	if resp.StatusCode() >= 400 {
		return nil, providerUtils.SetErrorLatency(ParseOpenAIError(resp), latency)
	}

	// Decode response body (handles content-encoding like gzip)
	responseBody, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err)
	}

	var listResp struct {
		Object  string                        `json:"object"`
		Data    []schemas.ContainerFileObject `json:"data"`
		FirstID *string                       `json:"first_id"`
		LastID  *string                       `json:"last_id"`
		HasMore bool                          `json:"has_more"`
	}

	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(responseBody, &listResp, nil, sendBackRawRequest, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Track last file ID for pagination cursor
	var lastFileID string
	for _, file := range listResp.Data {
		lastFileID = file.ID
	}

	// Build cursor for next request (handles cross-key pagination)
	nextCursor, hasMore := helper.BuildNextCursor(listResp.HasMore, lastFileID)

	containerFileListResponse := &schemas.RakshaContainerFileListResponse{
		Object:  listResp.Object,
		Data:    listResp.Data,
		FirstID: listResp.FirstID,
		LastID:  listResp.LastID,
		HasMore: hasMore,
		ExtraFields: schemas.RakshaResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}

	// Set encoded cursor for next page
	if nextCursor != "" {
		containerFileListResponse.After = &nextCursor
	}

	if sendBackRawRequest {
		containerFileListResponse.ExtraFields.RawRequest = rawRequest
	}
	if sendBackRawResponse {
		containerFileListResponse.ExtraFields.RawResponse = rawResponse
	}

	return containerFileListResponse, nil
}

// ContainerFileRetrieve retrieves a file from a container via OpenAI's API.
func (provider *OpenAIProvider) ContainerFileRetrieve(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaContainerFileRetrieveRequest) (*schemas.RakshaContainerFileRetrieveResponse, *schemas.RakshaError) {
	if len(keys) == 0 {
		if provider.customProviderConfig != nil && provider.customProviderConfig.IsKeyLess {
			keys = []schemas.Key{{}}
		} else {
			return nil, providerUtils.NewRakshaOperationError("no keys provided", nil)
		}
	}

	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.ContainerFileRetrieveRequest); err != nil {
		return nil, err
	}

	if request == nil {
		return nil, providerUtils.NewRakshaOperationError("invalid request: nil", nil)
	}

	if request.ContainerID == "" {
		return nil, providerUtils.NewRakshaOperationError("invalid request: container_id is required", nil)
	}

	if request.FileID == "" {
		return nil, providerUtils.NewRakshaOperationError("invalid request: file_id is required", nil)
	}

	var lastErr *schemas.RakshaError
	for _, key := range keys {
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()

		providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

		endpoint := fmt.Sprintf("/v1/containers/%s/files/%s", request.ContainerID, request.FileID)
		req.SetRequestURI(provider.buildRequestURL(ctx, endpoint, schemas.ContainerFileRetrieveRequest))
		req.Header.SetMethod(http.MethodGet)

		if key.Value.GetValue() != "" {
			req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
		}

		latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
		wait()
		if rakshaErr != nil {
			lastErr = rakshaErr
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			continue
		}

		if resp.StatusCode() >= 400 {
			lastErr = ParseOpenAIError(resp)
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			continue
		}

		// Decode response body (handles content-encoding like gzip)
		responseBody, err := providerUtils.CheckAndDecodeBody(resp)
		if err != nil {
			lastErr = providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err)
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			continue
		}
		sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
		sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)

		var fileResp struct {
			ID          string `json:"id"`
			Object      string `json:"object"`
			Bytes       int64  `json:"bytes"`
			CreatedAt   int64  `json:"created_at"`
			ContainerID string `json:"container_id"`
			Path        string `json:"path"`
			Source      string `json:"source"`
		}

		rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(responseBody, &fileResp, nil, sendBackRawRequest, sendBackRawResponse)
		if rakshaErr != nil {
			lastErr = rakshaErr
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			continue
		}

		containerFileRetrieveResponse := &schemas.RakshaContainerFileRetrieveResponse{
			ID:          fileResp.ID,
			Object:      fileResp.Object,
			Bytes:       fileResp.Bytes,
			CreatedAt:   fileResp.CreatedAt,
			ContainerID: fileResp.ContainerID,
			Path:        fileResp.Path,
			Source:      fileResp.Source,
			ExtraFields: schemas.RakshaResponseExtraFields{
				Latency: latency.Milliseconds(),
			},
		}

		if sendBackRawRequest {
			containerFileRetrieveResponse.ExtraFields.RawRequest = rawRequest
		}
		if sendBackRawResponse {
			containerFileRetrieveResponse.ExtraFields.RawResponse = rawResponse
		}

		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(resp)
		return containerFileRetrieveResponse, nil
	}

	return nil, lastErr
}

// ContainerFileContent retrieves the content of a file from a container via OpenAI's API.
func (provider *OpenAIProvider) ContainerFileContent(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaContainerFileContentRequest) (*schemas.RakshaContainerFileContentResponse, *schemas.RakshaError) {
	if len(keys) == 0 {
		if provider.customProviderConfig != nil && provider.customProviderConfig.IsKeyLess {
			keys = []schemas.Key{{}}
		} else {
			return nil, providerUtils.NewRakshaOperationError("no keys provided", nil)
		}
	}

	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.ContainerFileContentRequest); err != nil {
		return nil, err
	}

	if request == nil {
		return nil, providerUtils.NewRakshaOperationError("invalid request: nil", nil)
	}

	if request.ContainerID == "" {
		return nil, providerUtils.NewRakshaOperationError("invalid request: container_id is required", nil)
	}

	if request.FileID == "" {
		return nil, providerUtils.NewRakshaOperationError("invalid request: file_id is required", nil)
	}

	var lastErr *schemas.RakshaError
	for _, key := range keys {
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()

		providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

		endpoint := fmt.Sprintf("/v1/containers/%s/files/%s/content", request.ContainerID, request.FileID)
		req.SetRequestURI(provider.buildRequestURL(ctx, endpoint, schemas.ContainerFileContentRequest))
		req.Header.SetMethod(http.MethodGet)

		if key.Value.GetValue() != "" {
			req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
		}

		latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
		wait()
		if rakshaErr != nil {
			lastErr = rakshaErr
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			continue
		}

		if resp.StatusCode() >= 400 {
			lastErr = ParseOpenAIError(resp)
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			continue
		}

		// Get content type from response header
		contentType := string(resp.Header.ContentType())
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		// Decode response body (handles content-encoding like gzip)
		body, err := providerUtils.CheckAndDecodeBody(resp)
		if err != nil {
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			lastErr = providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err)
			continue
		}
		content := append([]byte(nil), body...)

		containerFileContentResponse := &schemas.RakshaContainerFileContentResponse{
			Content:     content,
			ContentType: contentType,
			ExtraFields: schemas.RakshaResponseExtraFields{
				Latency: latency.Milliseconds(),
			},
		}

		if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
			containerFileContentResponse.ExtraFields.RawRequest = map[string]string{
				"container_id": request.ContainerID,
				"file_id":      request.FileID,
			}
		}
		if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
			containerFileContentResponse.ExtraFields.RawResponse = "<REDACTED>"
		}

		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(resp)
		return containerFileContentResponse, nil
	}

	return nil, lastErr
}

// ContainerFileDelete deletes a file from a container via OpenAI's API.
func (provider *OpenAIProvider) ContainerFileDelete(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaContainerFileDeleteRequest) (*schemas.RakshaContainerFileDeleteResponse, *schemas.RakshaError) {
	if len(keys) == 0 {
		if provider.customProviderConfig != nil && provider.customProviderConfig.IsKeyLess {
			keys = []schemas.Key{{}}
		} else {
			return nil, providerUtils.NewRakshaOperationError("no keys provided", nil)
		}
	}

	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.ContainerFileDeleteRequest); err != nil {
		return nil, err
	}

	if request == nil {
		return nil, providerUtils.NewRakshaOperationError("invalid request: nil", nil)
	}

	if request.ContainerID == "" {
		return nil, providerUtils.NewRakshaOperationError("invalid request: container_id is required", nil)
	}

	if request.FileID == "" {
		return nil, providerUtils.NewRakshaOperationError("invalid request: file_id is required", nil)
	}

	var lastErr *schemas.RakshaError
	for _, key := range keys {
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()

		providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

		endpoint := fmt.Sprintf("/v1/containers/%s/files/%s", request.ContainerID, request.FileID)
		req.SetRequestURI(provider.buildRequestURL(ctx, endpoint, schemas.ContainerFileDeleteRequest))
		req.Header.SetMethod(http.MethodDelete)
		req.Header.SetContentType("application/json")

		if key.Value.GetValue() != "" {
			req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
		}

		latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
		wait()
		if rakshaErr != nil {
			lastErr = rakshaErr
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			continue
		}

		if resp.StatusCode() >= 400 {
			lastErr = ParseOpenAIError(resp)
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			continue
		}

		// Decode response body (handles content-encoding like gzip)
		responseBody, err := providerUtils.CheckAndDecodeBody(resp)
		if err != nil {
			lastErr = providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err)
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			continue
		}
		sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
		sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)

		var deleteResp struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Deleted bool   `json:"deleted"`
		}

		rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(responseBody, &deleteResp, nil, sendBackRawRequest, sendBackRawResponse)
		if rakshaErr != nil {
			lastErr = rakshaErr
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			continue
		}

		containerFileDeleteResponse := &schemas.RakshaContainerFileDeleteResponse{
			ID:      deleteResp.ID,
			Object:  deleteResp.Object,
			Deleted: deleteResp.Deleted,
			ExtraFields: schemas.RakshaResponseExtraFields{
				Latency: latency.Milliseconds(),
			},
		}

		if sendBackRawRequest {
			containerFileDeleteResponse.ExtraFields.RawRequest = rawRequest
		}
		if sendBackRawResponse {
			containerFileDeleteResponse.ExtraFields.RawResponse = rawResponse
		}

		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(resp)
		return containerFileDeleteResponse, nil
	}

	return nil, lastErr
}

func (provider *OpenAIProvider) Passthrough(
	ctx *schemas.RakshaContext,
	key schemas.Key,
	req *schemas.RakshaPassthroughRequest,
) (*schemas.RakshaPassthroughResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.PassthroughRequest); err != nil {
		return nil, err
	}

	path := req.Path
	// if path has v1 or v1/ remove it
	if after, ok := strings.CutPrefix(path, "/v1"); ok {
		path = after
	}

	url := provider.networkConfig.BaseURL + "/v1" + path
	if req.RawQuery != "" {
		url += "?" + req.RawQuery
	}

	fasthttpReq := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)
	defer fasthttp.ReleaseRequest(fasthttpReq)

	fasthttpReq.Header.SetMethod(req.Method)
	fasthttpReq.SetRequestURI(url)

	providerUtils.SetExtraHeaders(ctx, fasthttpReq, provider.networkConfig.ExtraHeaders, nil)

	for k, v := range req.SafeHeaders {
		fasthttpReq.Header.Set(k, v)
	}

	if key.Value.GetValue() != "" {
		fasthttpReq.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	fasthttpReq.SetBody(req.Body)

	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, fasthttpReq, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	headers := providerUtils.ExtractPassthroughProviderResponseHeaders(resp)
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, headers)

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError("failed to decode response body", err)
	}

	var passthroughUsage *schemas.RakshaPassthroughUsage
	if resp.StatusCode() >= 200 && resp.StatusCode() < 300 {
		passthroughUsage = ExtractOpenAIPassthroughUsage(req.Method, req.Path, req.Body, body)
	}

	rakshaResponse := &schemas.RakshaPassthroughResponse{
		StatusCode: resp.StatusCode(),
		Headers:    headers,
		Body:       body,
		ExtraFields: schemas.RakshaResponseExtraFields{
			Latency:                 latency.Milliseconds(),
			ProviderResponseHeaders: headers,
			PassthroughPath:         req.Path,
		},
		PassthroughUsage: passthroughUsage,
	}

	return rakshaResponse, nil
}

func (provider *OpenAIProvider) PassthroughStream(
	ctx *schemas.RakshaContext,
	postHookRunner schemas.PostHookRunner,
	postHookSpanFinalizer func(context.Context),
	key schemas.Key,
	req *schemas.RakshaPassthroughRequest,
) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.PassthroughStreamRequest); err != nil {
		return nil, err
	}

	providerUtils.SetStreamIdleTimeoutIfEmpty(ctx, provider.networkConfig.StreamIdleTimeoutInSeconds)
	path := req.Path
	if after, ok := strings.CutPrefix(path, "/v1"); ok {
		path = after
	}
	url := provider.networkConfig.BaseURL + "/v1" + path
	if req.RawQuery != "" {
		url += "?" + req.RawQuery
	}

	fasthttpReq := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	resp.StreamBody = true
	defer fasthttp.ReleaseRequest(fasthttpReq)

	fasthttpReq.Header.SetMethod(req.Method)
	fasthttpReq.SetRequestURI(url)

	providerUtils.SetExtraHeaders(ctx, fasthttpReq, provider.networkConfig.ExtraHeaders, nil)

	for k, v := range req.SafeHeaders {
		fasthttpReq.Header.Set(k, v)
	}

	fasthttpReq.Header.Set("Connection", "close")

	if key.Value.GetValue() != "" {
		fasthttpReq.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	fasthttpReq.SetBody(req.Body)

	activeClient := providerUtils.PrepareResponseStreaming(ctx, provider.streamingClient, resp)

	startTime := time.Now()

	err := activeClient.Do(fasthttpReq, resp)
	latency := time.Since(startTime)
	if err != nil {
		providerUtils.ReleaseStreamingResponse(ctx, resp)
		if errors.Is(err, context.Canceled) {
			return nil, providerUtils.SetErrorLatency(&schemas.RakshaError{
				IsRakshaError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   err,
				},
			}, latency)
		}
		if errors.Is(err, fasthttp.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
			return nil, providerUtils.SetErrorLatency(providerUtils.NewRakshaTimeoutError(schemas.ErrProviderRequestTimedOut, err), latency)
		}
		return nil, providerUtils.SetErrorLatency(providerUtils.NewRakshaOperationError(schemas.ErrProviderDoRequest, err), latency)
	}

	headers := providerUtils.ExtractPassthroughProviderResponseHeaders(resp)
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, headers)

	rawBodyStream := resp.BodyStream()
	if rawBodyStream == nil {
		providerUtils.ReleaseStreamingResponse(ctx, resp)
		return nil, providerUtils.NewRakshaOperationError(
			"provider returned an empty stream body",
			fmt.Errorf("provider returned an empty stream body"))
	}

	// Forward raw chunks to the client and extract usage incrementally per SSE event —
	return providerUtils.StreamPassthrough(
		ctx, postHookRunner, postHookSpanFinalizer, resp, rawBodyStream,
		providerUtils.PassthroughStreamParams{
			StatusCode:       resp.StatusCode(),
			Headers:          headers,
			Path:             req.Path,
			RawRequest:       req.Body,
			CancellationBody: providerUtils.PassthroughJSONBody(fasthttpReq, req.Body),
			StartTime:        startTime,
			Logger:           provider.logger,
			HasUsage:         HasOpenAIPassthroughUsage,
			Observe: func(event []byte) *schemas.RakshaPassthroughUsage {
				return ExtractOpenAIPassthroughUsage(req.Method, req.Path, req.Body, event)
			},
		},
	), nil
}
