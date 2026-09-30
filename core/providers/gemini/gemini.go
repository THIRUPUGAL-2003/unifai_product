package gemini

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	providerUtils "github.com/unifai/unifai/core/providers/utils"
	schemas "github.com/unifai/unifai/core/schemas"
	"github.com/valyala/fasthttp"
)

const (
	UnifAIContextKeyResponseFormat schemas.UnifAIContextKey = "unifai_context_key_response_format"
)

type GeminiProvider struct {
	logger               schemas.Logger                // Logger for provider operations
	client               *fasthttp.Client              // HTTP client for unary API requests (ReadTimeout bounds overall response)
	streamingClient      *fasthttp.Client              // HTTP client for streaming API requests (no ReadTimeout; idle governed by NewIdleTimeoutReader)
	networkConfig        schemas.NetworkConfig         // Network configuration including extra headers
	sendBackRawRequest   bool                          // Whether to include raw request in UnifAIResponse
	sendBackRawResponse  bool                          // Whether to include raw response in UnifAIResponse
	customProviderConfig *schemas.CustomProviderConfig // Custom provider config
}

func setGeminiRequestBody(req *fasthttp.Request, bodyReader io.Reader, bodySize int, jsonData []byte) {
	// Large payload mode streams request bytes directly from the ingress reader.
	// Normal mode sends marshaled JSON as before.
	// Example failure prevented: materializing giant request payloads again here
	// after transport already selected streaming mode.
	if bodyReader != nil {
		req.SetBodyStream(bodyReader, bodySize)
		return
	}
	req.SetBody(jsonData)
}

func normalizeRawGenerateContentBody(ctx *schemas.UnifAIContext, body []byte) []byte {
	if rawBody, ok := ctx.Value(schemas.UnifAIContextKeyUseRawRequestBody).(bool); ok && rawBody {
		body = NormalizeRawGenerateContentRequestForCompatibility(body)
	}
	if updated, err := providerUtils.DeleteJSONField(body, "fallbacks"); err == nil {
		body = updated
	}
	return body
}

// NewGeminiProvider creates a new Gemini provider instance.
// It initializes the HTTP client with the provided configuration.
// The client is configured with timeouts, concurrency limits, and optional proxy settings.
func NewGeminiProvider(config *schemas.ProviderConfig, logger schemas.Logger) *GeminiProvider {
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

	// Configure proxy and retry policy
	client = providerUtils.ConfigureProxy(client, config.ProxyConfig, logger)
	client = providerUtils.ConfigureDialer(client, config.NetworkConfig.AllowPrivateNetwork)
	client = providerUtils.ConfigureTLS(client, config.NetworkConfig, logger)
	streamingClient := providerUtils.BuildStreamingClient(client)

	// Set default BaseURL if not provided
	if config.NetworkConfig.BaseURL == "" {
		config.NetworkConfig.BaseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	config.NetworkConfig.BaseURL = strings.TrimRight(config.NetworkConfig.BaseURL, "/")

	return &GeminiProvider{
		logger:               logger,
		client:               client,
		streamingClient:      streamingClient,
		networkConfig:        config.NetworkConfig,
		customProviderConfig: config.CustomProviderConfig,
		sendBackRawRequest:   config.SendBackRawRequest,
		sendBackRawResponse:  config.SendBackRawResponse,
	}
}

// GetProviderKey returns the provider identifier for Gemini.
func (provider *GeminiProvider) GetProviderKey() schemas.ModelProvider {
	return providerUtils.GetProviderName(schemas.Gemini, provider.customProviderConfig)
}

// completeRequest handles the common HTTP request pattern for Gemini API calls.
// When large response streaming is activated (UnifAIContextKeyLargeResponseMode set in ctx),
// returns (nil, nil, latency, nil) — callers must check the context flag.
func (provider *GeminiProvider) completeRequest(ctx *schemas.UnifAIContext, model string, key schemas.Key, jsonBody []byte, endpoint string) (*GenerateContentResponse, interface{}, time.Duration, map[string]string, *schemas.UnifAIError) {
	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	respOwned := true
	defer func() {
		if respOwned {
			fasthttp.ReleaseResponse(resp)
		}
	}()

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	// Use Gemini's generateContent endpoint
	req.SetRequestURI(provider.networkConfig.BaseURL + providerUtils.GetPathFromContext(ctx, "/models/"+model+endpoint))
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}

	// Large payload mode: stream original request bytes directly from ingress.
	// Normal mode: use converted JSON body.
	usedLargePayloadBody := providerUtils.ApplyLargePayloadRequestBody(ctx, req)
	if !usedLargePayloadBody {
		jsonBody = normalizeRawGenerateContentBody(ctx, jsonBody)
		req.SetBody(jsonBody)
	}

	// Send the request with optional large response streaming
	activeClient := providerUtils.PrepareResponseStreaming(ctx, provider.client, resp)
	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, nil, latency, nil, unifaiErr
	}
	if usedLargePayloadBody {
		providerUtils.DrainLargePayloadRemainder(ctx)
	}

	// Extract provider response headers before status check so error responses also forward them
	providerResponseHeaders := providerUtils.ExtractProviderResponseHeaders(resp)

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		return nil, nil, latency, providerResponseHeaders, providerUtils.SetErrorLatency(parseGeminiError(resp), latency)
	}

	body, isLargeResp, decodeErr := providerUtils.FinalizeResponseWithLargeDetection(ctx, resp, provider.logger)
	if decodeErr != nil {
		return nil, nil, latency, providerResponseHeaders, decodeErr
	}
	if isLargeResp {
		respOwned = false
		return nil, nil, latency, providerResponseHeaders, nil
	}

	// Parse Gemini's response
	var geminiResponse GenerateContentResponse
	if err := sonic.Unmarshal(body, &geminiResponse); err != nil {
		return nil, nil, latency, providerResponseHeaders, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseUnmarshal, err)
	}

	var rawResponse interface{}
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		if err := sonic.Unmarshal(body, &rawResponse); err != nil {
			return nil, nil, latency, providerResponseHeaders, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseUnmarshal, err)
		}
	}

	return &geminiResponse, rawResponse, latency, providerResponseHeaders, nil
}

// listModelsByKey performs a list models request for a single key.
// Returns the response and latency, or an error if the request fails.
func (provider *GeminiProvider) listModelsByKey(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIListModelsRequest) (*schemas.UnifAIListModelsResponse, *schemas.UnifAIError) {
	providerName := provider.GetProviderKey()
	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	// Build URL using centralized URL construction
	req.SetRequestURI(provider.networkConfig.BaseURL + providerUtils.GetPathFromContext(ctx, fmt.Sprintf("/models?pageSize=%d", schemas.DefaultPageSize)))
	req.Header.SetMethod(http.MethodGet)
	req.Header.SetContentType("application/json")
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}

	// Make request
	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Store provider response headers in context before status check so error responses also forward them
	ctx.SetValue(schemas.UnifAIContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.SetErrorLatency(parseGeminiError(resp), latency)
	}

	// Parse Gemini's response
	var geminiResponse GeminiListModelsResponse
	rawRequest, rawResponse, unifaiErr := providerUtils.HandleProviderResponse(resp.Body(), &geminiResponse, nil, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if unifaiErr != nil {
		return nil, unifaiErr
	}
	if len(geminiResponse.Models) == 0 {
		var singleModel GeminiModel
		if err := sonic.Unmarshal(resp.Body(), &singleModel); err == nil && singleModel.Name != "" {
			geminiResponse.Models = []GeminiModel{singleModel}
		}
	}

	response := geminiResponse.ToUnifAIListModelsResponse(providerName, key.Models, key.BlacklistedModels, key.Aliases, request.Unfiltered)

	response.ExtraFields.Latency = latency.Milliseconds()

	// Set raw request if enabled
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		response.ExtraFields.RawRequest = rawRequest
	}

	// Set raw response if enabled
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		response.ExtraFields.RawResponse = rawResponse
	}

	return response, nil
}

// ListModels performs a list models request to Gemini's API.
// Requests are made concurrently for improved performance.
func (provider *GeminiProvider) ListModels(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAIListModelsRequest) (*schemas.UnifAIListModelsResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.ListModelsRequest); err != nil {
		return nil, err
	}
	if provider.customProviderConfig != nil && provider.customProviderConfig.IsKeyLess {
		return providerUtils.HandleKeylessListModelsRequest(provider.GetProviderKey(), func() (*schemas.UnifAIListModelsResponse, *schemas.UnifAIError) {
			return provider.listModelsByKey(ctx, schemas.Key{Models: schemas.WhiteList{"*"}}, request)
		})
	}
	return providerUtils.HandleMultipleListModelsRequests(
		ctx,
		keys,
		request,
		provider.listModelsByKey,
	)
}

// TextCompletion is not supported by the Gemini provider.
func (provider *GeminiProvider) TextCompletion(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAITextCompletionRequest) (*schemas.UnifAITextCompletionResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionRequest, provider.GetProviderKey())
}

// TextCompletionStream performs a streaming text completion request to Gemini's API.
// It formats the request, sends it to Gemini, and processes the response.
// Returns a channel of UnifAIStreamChunk objects or an error if the request fails.
func (provider *GeminiProvider) TextCompletionStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAITextCompletionRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionStreamRequest, provider.GetProviderKey())
}

// ChatCompletion performs a chat completion request to the Gemini API.
func (provider *GeminiProvider) ChatCompletion(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIChatRequest) (*schemas.UnifAIChatResponse, *schemas.UnifAIError) {
	// Check if chat completion is allowed for this provider
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.ChatCompletionRequest); err != nil {
		return nil, err
	}

	jsonData, err := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToGeminiChatCompletionRequest(ctx, request)
		})
	if err != nil {
		return nil, err
	}

	geminiResponse, rawResponse, latency, providerResponseHeaders, unifaiErr := provider.completeRequest(ctx, request.Model, key, jsonData, ":generateContent")
	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.UnifAIContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if unifaiErr != nil {
		return nil, providerUtils.EnrichError(ctx, unifaiErr, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Large response mode: return lightweight response with metadata only
	if isLargeResp, _ := ctx.Value(schemas.UnifAIContextKeyLargeResponseMode).(bool); isLargeResp {
		return &schemas.UnifAIChatResponse{
			Model: request.Model,
			ExtraFields: schemas.UnifAIResponseExtraFields{
				Latency:                 latency.Milliseconds(),
				ProviderResponseHeaders: providerResponseHeaders,
			},
		}, nil
	}

	unifaiResponse := geminiResponse.ToUnifAIChatResponse()

	unifaiResponse.ExtraFields.Latency = latency.Milliseconds()
	unifaiResponse.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	// Set raw request if enabled
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		providerUtils.ParseAndSetRawRequest(&unifaiResponse.ExtraFields, jsonData)
	}

	// Set raw response if enabled
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		unifaiResponse.ExtraFields.RawResponse = rawResponse
	}

	return unifaiResponse, nil
}

// ChatCompletionStream performs a streaming chat completion request to the Gemini API.
// It supports real-time streaming of responses using Server-Sent Events (SSE).
// Returns a channel containing UnifAIStreamChunk objects representing the stream or an error if the request fails.
func (provider *GeminiProvider) ChatCompletionStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAIChatRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	// Check if chat completion stream is allowed for this provider
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.ChatCompletionStreamRequest); err != nil {
		return nil, err
	}

	jsonData, err := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			reqBody, err := ToGeminiChatCompletionRequest(ctx, request)
			if err != nil {
				return nil, err
			}
			if reqBody == nil {
				return nil, fmt.Errorf("chat completion request is not provided or could not be converted to gemini format")
			}
			return reqBody, nil
		})
	if err != nil {
		return nil, err
	}

	// Prepare Gemini headers
	headers := map[string]string{
		"Accept":        "text/event-stream",
		"Cache-Control": "no-cache",
	}
	if key.Value.GetValue() != "" {
		headers["x-goog-api-key"] = key.Value.GetValue()
	}

	providerUtils.SetStreamIdleTimeoutIfEmpty(ctx, provider.networkConfig.StreamIdleTimeoutInSeconds)

	// Use shared Gemini streaming logic
	return HandleGeminiChatCompletionStream(
		ctx,
		provider.streamingClient,
		provider.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/models/"+request.Model+":streamGenerateContent?alt=sse"),
		jsonData,
		headers,
		provider.networkConfig.ExtraHeaders,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		request.Model,
		postHookRunner,
		nil,
		provider.logger,
		postHookSpanFinalizer,
	)
}

// HandleGeminiChatCompletionStream handles streaming for Gemini-compatible APIs.
func HandleGeminiChatCompletionStream(
	ctx *schemas.UnifAIContext,
	client *fasthttp.Client,
	url string,
	jsonBody []byte,
	headers map[string]string,
	extraHeaders map[string]string,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
	providerName schemas.ModelProvider,
	model string,
	postHookRunner schemas.PostHookRunner,
	postResponseConverter func(*schemas.UnifAIChatResponse) *schemas.UnifAIChatResponse,
	logger schemas.Logger,
	postHookSpanFinalizer func(context.Context),
) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	resp.StreamBody = true
	defer fasthttp.ReleaseRequest(req)

	req.Header.SetMethod(http.MethodPost)
	req.SetRequestURI(url)
	req.Header.SetContentType("application/json")
	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)

	// Set headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Large payload mode: stream original request bytes directly from ingress.
	if !providerUtils.ApplyLargePayloadRequestBody(ctx, req) {
		jsonBody = normalizeRawGenerateContentBody(ctx, jsonBody)
		req.SetBody(jsonBody)
	}

	startTime := time.Now()
	// Make the request — caller is responsible for passing a streaming-configured client.
	doErr := client.Do(req, resp)
	latency := time.Since(startTime)
	if doErr != nil {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		if errors.Is(doErr, context.Canceled) {
			return nil, providerUtils.EnrichError(ctx, &schemas.UnifAIError{
				IsUnifAIError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   doErr,
				},
			}, jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
		}
		if errors.Is(doErr, fasthttp.ErrTimeout) || errors.Is(doErr, context.DeadlineExceeded) {
			return nil, providerUtils.EnrichError(ctx, providerUtils.NewUnifAITimeoutError(schemas.ErrProviderRequestTimedOut, doErr), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
		}
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewUnifAIOperationError(schemas.ErrProviderDoRequest, doErr), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Extract provider response headers before status check so error responses also forward them
	ctx.SetValue(schemas.UnifAIContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	// Check for HTTP errors — use parseGeminiError to preserve upstream error details
	if resp.StatusCode() != fasthttp.StatusOK {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		respBody := append([]byte(nil), resp.Body()...)
		return nil, providerUtils.EnrichError(ctx, parseGeminiError(resp), jsonBody, respBody, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Large payload streaming passthrough — pipe raw upstream SSE to client
	if providerUtils.SetupStreamingPassthrough(ctx, resp) {
		responseChan := make(chan *schemas.UnifAIStreamChunk)
		providerUtils.CloseStream(ctx, responseChan)
		return responseChan, nil
	}

	// Create response channel
	responseChan := make(chan *schemas.UnifAIStreamChunk, schemas.DefaultStreamBufferSize)

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

		if resp.BodyStream() == nil {
			unifaiErr := providerUtils.NewUnifAIOperationError(
				"Provider returned an empty response",
				fmt.Errorf("provider returned an empty response"),
			)
			ctx.SetValue(schemas.UnifAIContextKeyStreamEndIndicator, true)
			providerUtils.ProcessAndSendUnifAIError(ctx, postHookRunner, providerUtils.EnrichError(ctx, unifaiErr, jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency), responseChan, logger, postHookSpanFinalizer)
			return
		}

		// Decompress gzip-encoded streams transparently (no-op for non-gzip)
		decompressedReader, releaseGzip := providerUtils.DecompressStreamBody(resp)
		defer releaseGzip()

		// Wrap reader with idle timeout to detect stalled streams.
		decompressedReader, stopIdleTimeout := providerUtils.NewIdleTimeoutReader(decompressedReader, resp.BodyStream(), providerUtils.GetStreamIdleTimeout(ctx), ctx)
		defer stopIdleTimeout()

		// Setup cancellation handler to close the raw network stream on ctx cancellation,
		// which immediately unblocks any in-progress read (including reads blocked inside a gzip decompression layer).
		stopCancellation := providerUtils.SetupStreamCancellation(ctx, resp.BodyStream(), logger)
		defer stopCancellation()

		skipInlineData := shouldSkipInlineDataForStreamingContext(ctx)
		var lineReader *bufio.Reader
		var sseReader providerUtils.SSEDataReader
		if skipInlineData {
			lineReader = bufio.NewReaderSize(decompressedReader, 64*1024)
		} else {
			sseReader = providerUtils.GetSSEDataReader(ctx, decompressedReader)
		}

		chunkIndex := 0
		lastChunkTime := startTime

		var responseID string
		var modelName string

		streamState := NewGeminiStreamState()

		// Running usage handle so a mid-stream cancel/timeout can bill for
		// tokens already processed. Gemini's usageMetadata is cumulative, so the
		// latest non-nil copy below is the running total.
		streamUsage := &schemas.UnifAILLMUsage{}
		ctx.SetValue(schemas.UnifAIContextKeyStreamAccumulatedUsage, streamUsage)

		for {
			// If context was cancelled/timed out, let defer handle it
			if ctx.Err() != nil {
				return
			}
			var (
				eventData []byte
				readErr   error
			)
			if skipInlineData {
				eventData, readErr = readNextSSEDataLine(lineReader, true)
			} else {
				eventData, readErr = sseReader.ReadDataLine()
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				// If context was cancelled/timed out, let defer handle it
				if ctx.Err() != nil {
					return
				}
				ctx.SetValue(schemas.UnifAIContextKeyStreamEndIndicator, true)
				logger.Warn("Error reading stream: %v", readErr)
				providerUtils.ProcessAndSendError(ctx, postHookRunner, readErr, responseChan, logger, postHookSpanFinalizer)
				return
			}
			// Process chunk using shared function
			geminiResponse, err := processGeminiStreamChunk(eventData)
			if err != nil {
				if strings.Contains(err.Error(), "gemini api error") {
					// Handle API error
					unifaiErr := &schemas.UnifAIError{
						Type:           schemas.Ptr("gemini_api_error"),
						IsUnifAIError: false,
						Error: &schemas.ErrorField{
							Message: err.Error(),
							Error:   err,
						},
					}
					ctx.SetValue(schemas.UnifAIContextKeyStreamEndIndicator, true)
					providerUtils.ProcessAndSendUnifAIError(ctx, postHookRunner, providerUtils.EnrichError(ctx, unifaiErr, jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency), responseChan, logger, postHookSpanFinalizer)
					return
				}
				logger.Warn("Failed to process chunk: %v", err)
				continue
			}

			// Track response ID and model
			if geminiResponse.ResponseID != "" && responseID == "" {
				responseID = geminiResponse.ResponseID
			}
			if geminiResponse.ModelVersion != "" && modelName == "" {
				modelName = geminiResponse.ModelVersion
			}

			// Convert to UnifAI stream response
			response, unifaiErr, isLastChunk := geminiResponse.ToUnifAIChatCompletionStream(streamState)
			if unifaiErr != nil {
				ctx.SetValue(schemas.UnifAIContextKeyStreamEndIndicator, true)
				providerUtils.ProcessAndSendUnifAIError(ctx, postHookRunner, providerUtils.EnrichError(ctx, unifaiErr, jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency), responseChan, logger, postHookSpanFinalizer)
				return
			}

			if response != nil {
				response.ID = responseID
				if modelName != "" {
					response.Model = modelName
				}
				// keep the running usage handle current (cumulative).
				if response.Usage != nil {
					*streamUsage = *response.Usage
				}
				response.ExtraFields = schemas.UnifAIResponseExtraFields{
					ChunkIndex: chunkIndex,
					Latency:    time.Since(lastChunkTime).Milliseconds(),
				}

				if postResponseConverter != nil {
					response = postResponseConverter(response)
					if response == nil {
						logger.Warn("postResponseConverter returned nil; skipping chunk")
						continue
					}
				}

				if sendBackRawResponse {
					response.ExtraFields.RawResponse = string(eventData)
				}

				lastChunkTime = time.Now()
				chunkIndex++

				if isLastChunk {
					if sendBackRawRequest {
						providerUtils.ParseAndSetRawRequest(&response.ExtraFields, jsonBody)
					}
					response.ExtraFields.Latency = time.Since(startTime).Milliseconds()
					ctx.SetValue(schemas.UnifAIContextKeyStreamEndIndicator, true)
					providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetUnifAIResponseForStreamResponse(nil, response, nil, nil, nil, nil), responseChan, postHookSpanFinalizer)
					break
				}

				// Process response through post-hooks and send to channel
				providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetUnifAIResponseForStreamResponse(nil, response, nil, nil, nil, nil), responseChan, postHookSpanFinalizer)
			}
		}
	}()

	return responseChan, nil
}

// Responses performs a chat completion request to Gemini's API.
// It formats the request, sends it to Gemini, and processes the response.
// Returns a UnifAIResponse containing the completion results or an error if the request fails.
func (provider *GeminiProvider) Responses(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIResponsesRequest) (*schemas.UnifAIResponsesResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.ResponsesRequest); err != nil {
		return nil, err
	}

	// Check for large payload streaming mode (enterprise-only feature)
	// In large payload mode, the request body streams directly from the client — skip body conversion
	var bodyReader io.Reader
	bodySize := -1
	var jsonData []byte

	if isLargePayload, ok := ctx.Value(schemas.UnifAIContextKeyLargePayloadMode).(bool); ok && isLargePayload {
		if reader, readerOk := ctx.Value(schemas.UnifAIContextKeyLargePayloadReader).(io.Reader); readerOk && reader != nil {
			bodyReader = reader
			if contentLength, lenOk := ctx.Value(schemas.UnifAIContextKeyLargePayloadContentLength).(int); lenOk {
				bodySize = contentLength
			}
		}
	}

	// For normal path (no large payload body reader), convert request to bytes
	if bodyReader == nil {
		var err *schemas.UnifAIError
		jsonData, err = providerUtils.CheckContextAndGetRequestBody(
			ctx,
			request,
			func() (providerUtils.RequestBodyWithExtraParams, error) {
				reqBody, err := ToGeminiResponsesRequest(ctx, request)
				if err != nil {
					return nil, err
				}
				if reqBody == nil {
					return nil, fmt.Errorf("responses input is not provided or could not be converted to gemini format")
				}
				return reqBody, nil
			})
		if err != nil {
			return nil, err
		}
	}

	// Check if enterprise large response detection is enabled
	if responseThreshold, ok := ctx.Value(schemas.UnifAIContextKeyLargeResponseThreshold).(int64); ok && responseThreshold > 0 {
		return provider.responsesWithLargeResponseDetection(ctx, key, request, jsonData, responseThreshold, bodyReader, bodySize)
	}

	// Use struct directly for JSON marshaling
	geminiResponse, rawResponse, latency, providerResponseHeaders, unifaiErr := provider.completeRequest(ctx, request.Model, key, jsonData, ":generateContent")
	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.UnifAIContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if unifaiErr != nil {
		return nil, providerUtils.EnrichError(ctx, unifaiErr, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Large response mode: return lightweight response with metadata only
	if isLargeResp, _ := ctx.Value(schemas.UnifAIContextKeyLargeResponseMode).(bool); isLargeResp {
		return &schemas.UnifAIResponsesResponse{
			Model: request.Model,
			ExtraFields: schemas.UnifAIResponseExtraFields{
				Latency:                 latency.Milliseconds(),
				ProviderResponseHeaders: providerResponseHeaders,
			},
		}, nil
	}

	// Create final response
	unifaiResponse := geminiResponse.ToResponsesUnifAIResponsesResponse()

	// Set ExtraFields
	unifaiResponse.ExtraFields.Latency = latency.Milliseconds()
	unifaiResponse.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	// Set raw request if enabled
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		providerUtils.ParseAndSetRawRequest(&unifaiResponse.ExtraFields, jsonData)
	}

	// Set raw response if enabled
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		unifaiResponse.ExtraFields.RawResponse = rawResponse
	}

	return unifaiResponse, nil
}

// responsesWithLargeResponseDetection makes the upstream request with response body streaming
// enabled. If the response Content-Length exceeds the threshold, it sets context flags for
// the router to stream the body directly to the client without full materialization.
// If the response is small, it falls through to the normal parse-and-convert path.
func (provider *GeminiProvider) responsesWithLargeResponseDetection(
	ctx *schemas.UnifAIContext,
	key schemas.Key,
	request *schemas.UnifAIResponsesRequest,
	jsonData []byte,
	responseThreshold int64,
	bodyReader io.Reader, // Optional: for large payload request streaming (pass nil for normal path)
	bodySize int, // Required if bodyReader is non-nil
) (*schemas.UnifAIResponsesResponse, *schemas.UnifAIError) {
	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	// Note: resp is NOT deferred — lifecycle managed manually for large responses

	// Enable response body streaming so fasthttp doesn't buffer the entire body
	resp.StreamBody = true

	// Set up request (same as completeRequest)
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.SetRequestURI(provider.networkConfig.BaseURL + providerUtils.GetPathFromContext(ctx, "/models/"+request.Model+":generateContent"))
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}

	// Large payload mode streams request bytes directly to upstream; normal mode sends marshaled bytes.
	if bodyReader == nil {
		jsonData = normalizeRawGenerateContentBody(ctx, jsonData)
	}
	setGeminiRequestBody(req, bodyReader, bodySize, jsonData)

	// Make request
	streamingClient := providerUtils.BuildLargeResponseClient(provider.client, responseThreshold)
	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, streamingClient, req, resp)
	if unifaiErr != nil {
		wait()
		fasthttp.ReleaseResponse(resp)
		return nil, unifaiErr
	}

	// Handle error response — materialize stream body for error parsing
	if resp.StatusCode() != fasthttp.StatusOK {
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		unifaiErr := parseGeminiError(resp)
		wait()
		fasthttp.ReleaseResponse(resp)
		return nil, providerUtils.EnrichError(ctx, unifaiErr, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Delegate large response detection + normal buffered path to shared utility
	responseBody, isLarge, respErr := providerUtils.FinalizeResponseWithLargeDetection(ctx, resp, provider.logger)
	if respErr != nil {
		wait()
		fasthttp.ReleaseResponse(resp)
		return nil, providerUtils.EnrichError(ctx, respErr, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}
	if isLarge {
		// Build lightweight response with usage from preview for plugin pipeline
		preview, _ := ctx.Value(schemas.UnifAIContextKeyLargePayloadResponsePreview).(string)
		usage := extractUsageFromResponsePrefetch([]byte(preview))
		unifaiResponse := &schemas.UnifAIResponsesResponse{
			ID:        schemas.Ptr("resp_" + providerUtils.GetRandomString(50)),
			CreatedAt: int(time.Now().Unix()),
			Model:     request.Model,
			Usage:     usage,
		}
		unifaiResponse.ExtraFields.Latency = latency.Milliseconds()
		// resp owned by reader in context — don't release
		wait()
		return unifaiResponse, nil
	}
	wait()
	fasthttp.ReleaseResponse(resp)

	// Normal parse-and-convert path
	var geminiResponse GenerateContentResponse
	if unmarshalErr := sonic.Unmarshal(responseBody, &geminiResponse); unmarshalErr != nil {
		return nil, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseUnmarshal, unmarshalErr)
	}
	unifaiResponse := geminiResponse.ToResponsesUnifAIResponsesResponse()
	unifaiResponse.ExtraFields.Latency = latency.Milliseconds()
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		providerUtils.ParseAndSetRawRequest(&unifaiResponse.ExtraFields, jsonData)
	}
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		var rawResponse interface{}
		sonic.Unmarshal(responseBody, &rawResponse) //nolint:errcheck
		unifaiResponse.ExtraFields.RawResponse = rawResponse
	}
	return unifaiResponse, nil
}

// extractUsageFromResponsePrefetch extracts usage metadata from the response prefetch buffer.
// Uses sonic.Get for O(1) extraction without parsing the full response.
func extractUsageFromResponsePrefetch(data []byte) *schemas.ResponsesResponseUsage {
	node, err := sonic.Get(data, "usageMetadata")
	if err != nil {
		return nil
	}
	raw, _ := node.Raw()
	if raw == "" {
		return nil
	}

	var usageMeta GenerateContentResponseUsageMetadata
	if err := sonic.UnmarshalString(raw, &usageMeta); err != nil {
		return nil
	}

	return ConvertGeminiUsageMetadataToResponsesUsage(&usageMeta)
}

// ResponsesStream performs a streaming responses request to the Gemini API.
func (provider *GeminiProvider) ResponsesStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAIResponsesRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	// Check if responses stream is allowed for this provider
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.ResponsesStreamRequest); err != nil {
		return nil, err
	}

	jsonData, err := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			reqBody, err := ToGeminiResponsesRequest(ctx, request)
			if err != nil {
				return nil, err
			}
			if reqBody == nil {
				return nil, fmt.Errorf("responses input is not provided or could not be converted to gemini format")
			}
			return reqBody, nil
		})
	if err != nil {
		return nil, err
	}

	// Prepare Gemini headers
	headers := map[string]string{
		"Accept":        "text/event-stream",
		"Cache-Control": "no-cache",
	}
	if key.Value.GetValue() != "" {
		headers["x-goog-api-key"] = key.Value.GetValue()
	}

	providerUtils.SetStreamIdleTimeoutIfEmpty(ctx, provider.networkConfig.StreamIdleTimeoutInSeconds)

	return HandleGeminiResponsesStream(
		ctx,
		provider.streamingClient,
		provider.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/models/"+request.Model+":streamGenerateContent?alt=sse"),
		jsonData,
		headers,
		provider.networkConfig.ExtraHeaders,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		request.Model,
		postHookRunner,
		nil,
		provider.logger,
		postHookSpanFinalizer,
	)
}

// HandleGeminiResponsesStream handles streaming for Gemini-compatible APIs.
func HandleGeminiResponsesStream(
	ctx *schemas.UnifAIContext,
	client *fasthttp.Client,
	url string,
	jsonBody []byte,
	headers map[string]string,
	extraHeaders map[string]string,
	sendBackRawRequest bool,
	sendBackRawResponse bool,
	providerName schemas.ModelProvider,
	model string,
	postHookRunner schemas.PostHookRunner,
	postResponseConverter func(*schemas.UnifAIResponsesStreamResponse) *schemas.UnifAIResponsesStreamResponse,
	logger schemas.Logger,
	postHookSpanFinalizer func(context.Context),
) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	resp.StreamBody = true
	defer fasthttp.ReleaseRequest(req)

	req.Header.SetMethod(http.MethodPost)
	req.SetRequestURI(url)
	req.Header.SetContentType("application/json")
	providerUtils.SetExtraHeaders(ctx, req, extraHeaders, nil)

	// Set headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Large payload mode: stream original request body to upstream.
	if !providerUtils.ApplyLargePayloadRequestBody(ctx, req) {
		jsonBody = normalizeRawGenerateContentBody(ctx, jsonBody)
		req.SetBody(jsonBody)
	}

	startTime := time.Now()
	// Make the request — caller is responsible for passing a streaming-configured client.
	doErr := client.Do(req, resp)
	latency := time.Since(startTime)
	if doErr != nil {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		if errors.Is(doErr, context.Canceled) {
			return nil, providerUtils.EnrichError(ctx, &schemas.UnifAIError{
				IsUnifAIError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   doErr,
				},
			}, jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
		}
		if errors.Is(doErr, fasthttp.ErrTimeout) || errors.Is(doErr, context.DeadlineExceeded) {
			return nil, providerUtils.EnrichError(ctx, providerUtils.NewUnifAITimeoutError(schemas.ErrProviderRequestTimedOut, doErr), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
		}
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewUnifAIOperationError(schemas.ErrProviderDoRequest, doErr), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Extract provider response headers before status check so error responses also forward them
	ctx.SetValue(schemas.UnifAIContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	// Check for HTTP errors — use parseGeminiError to preserve upstream error details
	if resp.StatusCode() != fasthttp.StatusOK {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		return nil, providerUtils.EnrichError(ctx, parseGeminiError(resp), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Large payload streaming passthrough — pipe raw upstream SSE to client
	if providerUtils.SetupStreamingPassthrough(ctx, resp) {
		responseChan := make(chan *schemas.UnifAIStreamChunk)
		providerUtils.CloseStream(ctx, responseChan)
		return responseChan, nil
	}

	// Create response channel
	responseChan := make(chan *schemas.UnifAIStreamChunk, schemas.DefaultStreamBufferSize)

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

		if resp.BodyStream() == nil {
			unifaiErr := providerUtils.NewUnifAIOperationError(
				"Provider returned an empty response",
				fmt.Errorf("provider returned an empty response"),
			)
			ctx.SetValue(schemas.UnifAIContextKeyStreamEndIndicator, true)
			providerUtils.ProcessAndSendUnifAIError(
				ctx,
				postHookRunner,
				providerUtils.EnrichError(ctx, unifaiErr, jsonBody, nil, sendBackRawRequest, sendBackRawResponse),
				responseChan,
				logger,
				postHookSpanFinalizer,
			)
			return
		}

		// Decompress gzip-encoded streams transparently (no-op for non-gzip)
		decompressedReader, releaseGzip := providerUtils.DecompressStreamBody(resp)
		defer releaseGzip()

		// Wrap reader with idle timeout to detect stalled streams.
		decompressedReader, stopIdleTimeout := providerUtils.NewIdleTimeoutReader(decompressedReader, resp.BodyStream(), providerUtils.GetStreamIdleTimeout(ctx), ctx)
		defer stopIdleTimeout()

		// Setup cancellation handler to close the raw network stream on ctx cancellation,
		// which immediately unblocks any in-progress read (including reads blocked inside a gzip decompression layer).
		stopCancellation := providerUtils.SetupStreamCancellation(ctx, resp.BodyStream(), logger)
		defer stopCancellation()

		skipInlineData := shouldSkipInlineDataForStreamingContext(ctx)
		var lineReader *bufio.Reader
		var sseReader providerUtils.SSEDataReader
		if skipInlineData {
			lineReader = bufio.NewReaderSize(decompressedReader, 64*1024)
		} else {
			sseReader = providerUtils.GetSSEDataReader(ctx, decompressedReader)
		}

		chunkIndex := 0
		sequenceNumber := 0 // Track sequence across all events
		lastChunkTime := startTime

		// Initialize stream state for responses lifecycle management
		streamState := acquireGeminiResponsesStreamState()
		defer releaseGeminiResponsesStreamState(streamState)

		var lastUsageMetadata *GenerateContentResponseUsageMetadata

		// Running usage handle so a mid-stream cancel/timeout can bill for
		// tokens already processed. Gemini's usageMetadata is cumulative.
		streamUsage := &schemas.UnifAILLMUsage{}
		ctx.SetValue(schemas.UnifAIContextKeyStreamAccumulatedUsage, streamUsage)

		for {
			// If context was cancelled/timed out, let defer handle it
			if ctx.Err() != nil {
				return
			}
			var (
				eventData []byte
				readErr   error
			)
			if skipInlineData {
				eventData, readErr = readNextSSEDataLine(lineReader, true)
			} else {
				eventData, readErr = sseReader.ReadDataLine()
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				if ctx.Err() != nil {
					return
				}
				ctx.SetValue(schemas.UnifAIContextKeyStreamEndIndicator, true)
				logger.Warn("Error reading stream: %v", readErr)
				providerUtils.ProcessAndSendError(ctx, postHookRunner, readErr, responseChan, logger, postHookSpanFinalizer)
				return
			}

			// Process chunk using shared function
			geminiResponse, err := processGeminiStreamChunk(eventData)
			if err != nil {
				if strings.Contains(err.Error(), "gemini api error") {
					// Handle API error
					unifaiErr := &schemas.UnifAIError{
						Type:           schemas.Ptr("gemini_api_error"),
						IsUnifAIError: false,
						Error: &schemas.ErrorField{
							Message: err.Error(),
							Error:   err,
						},
					}
					ctx.SetValue(schemas.UnifAIContextKeyStreamEndIndicator, true)
					providerUtils.ProcessAndSendUnifAIError(ctx, postHookRunner, providerUtils.EnrichError(ctx, unifaiErr, jsonBody, nil, sendBackRawRequest, sendBackRawResponse), responseChan, logger, postHookSpanFinalizer)
					return
				}
				logger.Warn("Failed to process chunk: %v", err)
				continue
			}

			// Track usage metadata from the latest chunk
			if geminiResponse.UsageMetadata != nil {
				lastUsageMetadata = geminiResponse.UsageMetadata
				// keep the running usage handle current (cumulative).
				if converted := ConvertGeminiUsageMetadataToChatUsage(lastUsageMetadata); converted != nil {
					*streamUsage = *converted
				}
			}

			// Convert to UnifAI responses stream response
			responses, unifaiErr := geminiResponse.ToUnifAIResponsesStream(sequenceNumber, streamState)
			if unifaiErr != nil {
				ctx.SetValue(schemas.UnifAIContextKeyStreamEndIndicator, true)
				providerUtils.ProcessAndSendUnifAIError(ctx, postHookRunner, providerUtils.EnrichError(ctx, unifaiErr, jsonBody, nil, sendBackRawRequest, sendBackRawResponse), responseChan, logger, postHookSpanFinalizer)
				return
			}

			for i, response := range responses {
				if response != nil {
					response.ExtraFields = schemas.UnifAIResponseExtraFields{
						ChunkIndex: chunkIndex,
						Latency:    time.Since(lastChunkTime).Milliseconds(),
					}

					if postResponseConverter != nil {
						response = postResponseConverter(response)
						if response == nil {
							logger.Warn("postResponseConverter returned nil; skipping chunk")
							continue
						}
					}

					// Only add raw response to the LAST response in the array
					if sendBackRawResponse && i == len(responses)-1 {
						response.ExtraFields.RawResponse = string(eventData)
					}

					chunkIndex++
					sequenceNumber++ // Increment sequence number for each response

					// Check if this is the last chunk
					isLastChunk := false
					if response.Type == schemas.ResponsesStreamResponseTypeCompleted {
						isLastChunk = true
					}

					if isLastChunk {
						if sendBackRawRequest {
							providerUtils.ParseAndSetRawRequest(&response.ExtraFields, jsonBody)
						}
						response.ExtraFields.Latency = time.Since(startTime).Milliseconds()
						ctx.SetValue(schemas.UnifAIContextKeyStreamEndIndicator, true)
						providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetUnifAIResponseForStreamResponse(nil, nil, response, nil, nil, nil), responseChan, postHookSpanFinalizer)
						return
					}

					// For multiple responses in one event, only update timing on the last one
					if i == len(responses)-1 {
						lastChunkTime = time.Now()
					}

					// Process response through post-hooks and send to channel
					providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetUnifAIResponseForStreamResponse(nil, nil, response, nil, nil, nil), responseChan, postHookSpanFinalizer)
				}
			}
		}
		// Finalize the stream by closing any open items
		finalResponses := FinalizeGeminiResponsesStream(streamState, lastUsageMetadata, sequenceNumber)
		for i, finalResponse := range finalResponses {
			if finalResponse == nil {
				logger.Warn("FinalizeGeminiResponsesStream returned nil; skipping final response")
				continue
			}
			finalResponse.ExtraFields = schemas.UnifAIResponseExtraFields{
				ChunkIndex: chunkIndex,
				Latency:    time.Since(lastChunkTime).Milliseconds(),
			}

			if postResponseConverter != nil {
				finalResponse = postResponseConverter(finalResponse)
				if finalResponse == nil {
					logger.Warn("postResponseConverter returned nil; skipping final response")
					continue
				}
			}

			chunkIndex++
			sequenceNumber++

			if sendBackRawResponse {
				finalResponse.ExtraFields.RawResponse = "{}" // Final event has no payload
			}
			isLast := i == len(finalResponses)-1
			// Set final latency on the last response (completed event)
			if isLast {
				ctx.SetValue(schemas.UnifAIContextKeyStreamEndIndicator, true)
				finalResponse.ExtraFields.Latency = time.Since(startTime).Milliseconds()
			}
			providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetUnifAIResponseForStreamResponse(nil, nil, finalResponse, nil, nil, nil), responseChan, postHookSpanFinalizer)
		}
	}()

	return responseChan, nil
}

// Embedding performs an embedding request to the Gemini API.
func (provider *GeminiProvider) Embedding(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIEmbeddingRequest) (*schemas.UnifAIEmbeddingResponse, *schemas.UnifAIError) {
	// Check if embedding is allowed for this provider
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.EmbeddingRequest); err != nil {
		return nil, err
	}

	providerName := provider.GetProviderKey()

	// Convert UnifAI request to Gemini batch embedding request format
	jsonData, err := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToGeminiEmbeddingRequest(request), nil
		})
	if err != nil {
		return nil, err
	}

	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	// resp lifecycle managed manually for large response streaming

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	// Use Gemini's batchEmbedContents endpoint
	req.SetRequestURI(provider.networkConfig.BaseURL + providerUtils.GetPathFromContext(ctx, "/models/"+request.Model+":batchEmbedContents"))
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}

	// Large payload mode: stream original request bytes directly from ingress.
	// Normal mode: use converted JSON body.
	usedLargePayloadBody := providerUtils.ApplyLargePayloadRequestBody(ctx, req)
	if !usedLargePayloadBody {
		req.SetBody(jsonData)
	}

	// Send the request with optional large response streaming
	activeClient := providerUtils.PrepareResponseStreaming(ctx, provider.client, resp)
	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
	if unifaiErr != nil {
		wait()
		fasthttp.ReleaseResponse(resp)
		return nil, providerUtils.EnrichError(ctx, unifaiErr, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}
	// When upstream responds before consuming full upload, drain remaining bytes from
	// ingress reader so proxy hops (e.g., Caddy) don't surface broken-pipe 502s.
	if usedLargePayloadBody {
		providerUtils.DrainLargePayloadRemainder(ctx)
	}

	// Extract provider response headers before status check so error responses also forward them
	providerResponseHeaders := providerUtils.ExtractProviderResponseHeaders(resp)
	ctx.SetValue(schemas.UnifAIContextKeyProviderResponseHeaders, providerResponseHeaders)

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		provider.logger.Debug(fmt.Sprintf("error from %s provider: %s", providerName, string(resp.Body())))
		parsedErr := providerUtils.EnrichError(ctx, parseGeminiError(resp), jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		wait()
		fasthttp.ReleaseResponse(resp)
		return nil, parsedErr
	}

	body, isLargeResp, decodeErr := providerUtils.FinalizeResponseWithLargeDetection(ctx, resp, provider.logger)
	if decodeErr != nil {
		wait()
		fasthttp.ReleaseResponse(resp)
		return nil, providerUtils.EnrichError(ctx, decodeErr, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}
	if isLargeResp {
		// Large response detected — return lightweight response with metadata only;
		// resp owned by LargeResponseReader in context, don't release.
		wait()
		return &schemas.UnifAIEmbeddingResponse{
			Model: request.Model,
			ExtraFields: schemas.UnifAIResponseExtraFields{
				Latency:                 latency.Milliseconds(),
				ProviderResponseHeaders: providerResponseHeaders,
			},
		}, nil
	}
	wait()
	fasthttp.ReleaseResponse(resp)

	// Parse Gemini's batch embedding response
	var geminiResponse GeminiEmbeddingResponse
	rawRequest, rawResponse, unifaiErr := providerUtils.HandleProviderResponse(body, &geminiResponse, jsonData,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if unifaiErr != nil {
		return nil, providerUtils.EnrichError(ctx, unifaiErr, jsonData, body, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Convert to UnifAI format
	unifaiResponse := ToUnifAIEmbeddingResponse(&geminiResponse, request.Model)
	if unifaiResponse == nil {
		return nil, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseUnmarshal,
			fmt.Errorf("failed to convert Gemini embedding response to UnifAI format"))
	}

	unifaiResponse.ExtraFields.Latency = latency.Milliseconds()

	// Set raw request if enabled
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		unifaiResponse.ExtraFields.RawRequest = rawRequest
	}

	// Set raw response if enabled
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		unifaiResponse.ExtraFields.RawResponse = rawResponse
	}

	return unifaiResponse, nil
}

// Speech performs a speech synthesis request to the Gemini API.
func (provider *GeminiProvider) Speech(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAISpeechRequest) (*schemas.UnifAISpeechResponse, *schemas.UnifAIError) {
	// Check if speech is allowed for this provider
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.SpeechRequest); err != nil {
		return nil, err
	}

	// Prepare request body using speech-specific function
	jsonData, err := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToGeminiSpeechRequest(request)
		})
	if err != nil {
		return nil, err
	}

	// Use common request function
	geminiResponse, rawResponse, latency, providerResponseHeaders, unifaiErr := provider.completeRequest(ctx, request.Model, key, jsonData, ":generateContent")
	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.UnifAIContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if unifaiErr != nil {
		return nil, providerUtils.EnrichError(ctx, unifaiErr, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Large response mode: return lightweight response with metadata only
	if isLargeResp, _ := ctx.Value(schemas.UnifAIContextKeyLargeResponseMode).(bool); isLargeResp {
		return &schemas.UnifAISpeechResponse{
			ExtraFields: schemas.UnifAIResponseExtraFields{
				Latency:                 latency.Milliseconds(),
				ProviderResponseHeaders: providerResponseHeaders,
			},
		}, nil
	}

	if request.Params != nil {
		ctx.SetValue(UnifAIContextKeyResponseFormat, request.Params.ResponseFormat)
	}
	response, convErr := geminiResponse.ToUnifAISpeechResponse(ctx)
	if convErr != nil {
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, convErr), jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Set ExtraFields
	response.ExtraFields.Latency = latency.Milliseconds()
	response.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		providerUtils.ParseAndSetRawRequest(&response.ExtraFields, jsonData)
	}

	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		response.ExtraFields.RawResponse = rawResponse
	}

	return response, nil
}

// Rerank is not supported by the Gemini provider.
func (provider *GeminiProvider) Rerank(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIRerankRequest) (*schemas.UnifAIRerankResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.RerankRequest, provider.GetProviderKey())
}

// OCR is not supported by the Gemini provider.
func (provider *GeminiProvider) OCR(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIOCRRequest) (*schemas.UnifAIOCRResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.OCRRequest, provider.GetProviderKey())
}

// SpeechStream performs a streaming speech synthesis request to the Gemini API.
func (provider *GeminiProvider) SpeechStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAISpeechRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	// Check if speech stream is allowed for this provider
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.SpeechStreamRequest); err != nil {
		return nil, err
	}

	// Prepare request body using speech-specific function
	jsonBody, unifaiErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToGeminiSpeechRequest(request)
		})
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Create HTTP request for streaming
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	resp.StreamBody = true
	defer fasthttp.ReleaseRequest(req)

	req.Header.SetMethod(http.MethodPost)
	req.SetRequestURI(provider.networkConfig.BaseURL + providerUtils.GetPathFromContext(ctx, "/models/"+request.Model+":streamGenerateContent?alt=sse"))
	req.Header.SetContentType("application/json")

	// Set headers for streaming
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	// Large payload mode: stream original request body to upstream.
	if !providerUtils.ApplyLargePayloadRequestBody(ctx, req) {
		req.SetBody(jsonBody)
	}

	startTime := time.Now()
	// Make the request
	err := provider.streamingClient.Do(req, resp)
	latency := time.Since(startTime)
	if err != nil {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		if errors.Is(err, context.Canceled) {
			return nil, providerUtils.EnrichError(ctx, &schemas.UnifAIError{
				IsUnifAIError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   err,
				},
			}, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}
		if errors.Is(err, fasthttp.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
			return nil, providerUtils.EnrichError(ctx, providerUtils.NewUnifAITimeoutError(schemas.ErrProviderRequestTimedOut, err), jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}
		// Request failed before the first response byte (server closed an idle/pooled connection,
		// broken pipe, connection refused, DNS failure, etc.). Surface as a retriable upstream
		// connection error (502) so executeRequestWithRetries honors max_retries, matching the
		// non-streaming path - see https://github.com/unifai/unifai/issues/4496.
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewUnifAIUpstreamConnectionError(schemas.ErrProviderDoRequest, err), jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Extract provider response headers before status check so error responses also forward them
	ctx.SetValue(schemas.UnifAIContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	// Check for HTTP errors
	if resp.StatusCode() != fasthttp.StatusOK {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		return nil, providerUtils.EnrichError(ctx, parseGeminiError(resp), jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Large payload streaming passthrough — pipe raw upstream SSE to client
	if providerUtils.SetupStreamingPassthrough(ctx, resp) {
		responseChan := make(chan *schemas.UnifAIStreamChunk)
		providerUtils.CloseStream(ctx, responseChan)
		return responseChan, nil
	}

	// Create response channel
	responseChan := make(chan *schemas.UnifAIStreamChunk, schemas.DefaultStreamBufferSize)

	providerUtils.SetStreamIdleTimeoutIfEmpty(ctx, provider.networkConfig.StreamIdleTimeoutInSeconds)

	// Start streaming in a goroutine
	go func() {
		defer providerUtils.EnsureStreamFinalizerCalled(ctx, postHookSpanFinalizer)
		defer func() {
			if ctx.Err() == context.Canceled {
				providerUtils.HandleStreamCancellation(ctx, postHookRunner, responseChan, provider.logger, postHookSpanFinalizer, jsonBody)
			} else if ctx.Err() == context.DeadlineExceeded {
				providerUtils.HandleStreamTimeout(ctx, postHookRunner, responseChan, provider.logger, postHookSpanFinalizer, jsonBody)
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
		stopCancellation := providerUtils.SetupStreamCancellation(ctx, resp.BodyStream(), provider.logger)
		defer stopCancellation()

		sseReader := providerUtils.GetSSEDataReader(ctx, reader)
		chunkIndex := -1
		usage := &schemas.SpeechUsage{}
		lastChunkTime := startTime

		for {
			// If context was cancelled/timed out, let defer handle it
			if ctx.Err() != nil {
				return
			}

			data, readErr := sseReader.ReadDataLine()
			if readErr != nil {
				// Recheck context cancellation
				if ctx.Err() != nil {
					return
				}
				if readErr != io.EOF {
					ctx.SetValue(schemas.UnifAIContextKeyStreamEndIndicator, true)
					provider.logger.Warn("Error reading stream: %v", readErr)
					providerUtils.ProcessAndSendError(ctx, postHookRunner, readErr, responseChan, provider.logger, postHookSpanFinalizer)
					return
				}
				break
			}

			jsonData := data

			// Process chunk using shared function
			geminiResponse, err := processGeminiStreamChunk(jsonData)
			if err != nil {
				if strings.Contains(err.Error(), "gemini api error") {
					// Handle API error
					unifaiErr := &schemas.UnifAIError{
						Type:           schemas.Ptr("gemini_api_error"),
						IsUnifAIError: false,
						Error: &schemas.ErrorField{
							Message: err.Error(),
							Error:   err,
						},
					}
					ctx.SetValue(schemas.UnifAIContextKeyStreamEndIndicator, true)
					providerUtils.ProcessAndSendUnifAIError(ctx, postHookRunner, unifaiErr, responseChan, provider.logger, postHookSpanFinalizer)
					return
				}
				provider.logger.Warn("Failed to process chunk: %v", err)
				continue
			}

			// Extract audio data from Gemini response for regular chunks
			var audioChunk []byte
			if len(geminiResponse.Candidates) > 0 {
				candidate := geminiResponse.Candidates[0]
				if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
					var buf []byte
					for _, part := range candidate.Content.Parts {
						if part.InlineData != nil && len(part.InlineData.Data) > 0 {
							// Decode base64-encoded audio data
							decodedData, err := decodeBase64StringToBytes(part.InlineData.Data)
							if err != nil {
								provider.logger.Warn("Failed to decode base64 audio data: %v", err)
								continue
							}
							buf = append(buf, decodedData...)
						}
					}
					if len(buf) > 0 {
						audioChunk = buf
					}
				}
			}

			// Check if this is the final chunk (has finishReason)
			if len(geminiResponse.Candidates) > 0 && (geminiResponse.Candidates[0].FinishReason != "" || geminiResponse.UsageMetadata != nil) {
				// Extract usage metadata using shared function
				inputTokens, outputTokens, totalTokens := extractGeminiUsageMetadata(geminiResponse)
				usage.InputTokens = inputTokens
				usage.OutputTokens = outputTokens
				usage.TotalTokens = totalTokens
			}

			// Only send response if we have actual audio content
			if len(audioChunk) > 0 {
				chunkIndex++

				// Create UnifAI speech response for streaming
				response := &schemas.UnifAISpeechStreamResponse{
					Type:  schemas.SpeechStreamResponseTypeDelta,
					Audio: audioChunk,
					ExtraFields: schemas.UnifAIResponseExtraFields{
						ChunkIndex: chunkIndex,
						Latency:    time.Since(lastChunkTime).Milliseconds(),
					},
				}
				lastChunkTime = time.Now()

				if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
					response.ExtraFields.RawResponse = jsonData
				}

				// Process response through post-hooks and send to channel
				providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetUnifAIResponseForStreamResponse(nil, nil, nil, response, nil, nil), responseChan, postHookSpanFinalizer)
			}
		}
		response := &schemas.UnifAISpeechStreamResponse{
			Type:  schemas.SpeechStreamResponseTypeDone,
			Usage: usage,
			ExtraFields: schemas.UnifAIResponseExtraFields{
				ChunkIndex: chunkIndex + 1,
				Latency:    time.Since(startTime).Milliseconds(),
			},
		}
		response.BackfillParams(request)
		// Set raw request if enabled
		if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
			providerUtils.ParseAndSetRawRequest(&response.ExtraFields, jsonBody)
		}
		ctx.SetValue(schemas.UnifAIContextKeyStreamEndIndicator, true)
		providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetUnifAIResponseForStreamResponse(nil, nil, nil, response, nil, nil), responseChan, postHookSpanFinalizer)
	}()

	return responseChan, nil
}

// Transcription performs a speech-to-text request to the Gemini API.
func (provider *GeminiProvider) Transcription(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAITranscriptionRequest) (*schemas.UnifAITranscriptionResponse, *schemas.UnifAIError) {
	// Check if transcription is allowed for this provider
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.TranscriptionRequest); err != nil {
		return nil, err
	}

	// Prepare request body using transcription-specific function
	jsonData, unifaiErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToGeminiTranscriptionRequest(request), nil
		})
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Use common request function
	geminiResponse, rawResponse, latency, providerResponseHeaders, unifaiErr := provider.completeRequest(ctx, request.Model, key, jsonData, ":generateContent")
	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.UnifAIContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if unifaiErr != nil {
		return nil, providerUtils.EnrichError(ctx, unifaiErr, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Large response mode: return lightweight response with metadata only
	if isLargeResp, _ := ctx.Value(schemas.UnifAIContextKeyLargeResponseMode).(bool); isLargeResp {
		return &schemas.UnifAITranscriptionResponse{
			ExtraFields: schemas.UnifAIResponseExtraFields{
				Latency:                 latency.Milliseconds(),
				ProviderResponseHeaders: providerResponseHeaders,
			},
		}, nil
	}

	response := geminiResponse.ToUnifAITranscriptionResponse()

	// Set ExtraFields
	response.ExtraFields.Latency = latency.Milliseconds()
	response.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		providerUtils.ParseAndSetRawRequest(&response.ExtraFields, jsonData)
	}

	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		response.ExtraFields.RawResponse = rawResponse
	}

	return response, nil
}

// TranscriptionStream performs a streaming speech-to-text request to the Gemini API.
func (provider *GeminiProvider) TranscriptionStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAITranscriptionRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	// Check if transcription stream is allowed for this provider
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.TranscriptionStreamRequest); err != nil {
		return nil, err
	}

	// Prepare request body using transcription-specific function
	jsonBody, unifaiErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToGeminiTranscriptionRequest(request), nil
		})
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Create HTTP request for streaming
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	resp.StreamBody = true
	defer fasthttp.ReleaseRequest(req)

	req.Header.SetMethod(http.MethodPost)
	req.SetRequestURI(provider.networkConfig.BaseURL + providerUtils.GetPathFromContext(ctx, "/models/"+request.Model+":streamGenerateContent?alt=sse"))
	req.Header.SetContentType("application/json")

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	// Set headers for streaming
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	// Large payload mode: stream original request body to upstream.
	if !providerUtils.ApplyLargePayloadRequestBody(ctx, req) {
		req.SetBody(jsonBody)
	}

	startTime := time.Now()
	// Make the request
	err := provider.streamingClient.Do(req, resp)
	latency := time.Since(startTime)
	if err != nil {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		if errors.Is(err, context.Canceled) {
			return nil, providerUtils.EnrichError(ctx, &schemas.UnifAIError{
				IsUnifAIError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   err,
				},
			}, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}
		if errors.Is(err, fasthttp.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
			return nil, providerUtils.EnrichError(ctx, providerUtils.NewUnifAITimeoutError(schemas.ErrProviderRequestTimedOut, err), jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}
		// Request failed before the first response byte (server closed an idle/pooled connection,
		// broken pipe, connection refused, DNS failure, etc.). Surface as a retriable upstream
		// connection error (502) so executeRequestWithRetries honors max_retries, matching the
		// non-streaming path - see https://github.com/unifai/unifai/issues/4496.
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewUnifAIUpstreamConnectionError(schemas.ErrProviderDoRequest, err), jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Extract provider response headers before status check so error responses also forward them
	ctx.SetValue(schemas.UnifAIContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	// Check for HTTP errors
	if resp.StatusCode() != fasthttp.StatusOK {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		return nil, providerUtils.EnrichError(ctx, parseGeminiError(resp), jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Large payload streaming passthrough — pipe raw upstream SSE to client
	if providerUtils.SetupStreamingPassthrough(ctx, resp) {
		responseChan := make(chan *schemas.UnifAIStreamChunk)
		providerUtils.CloseStream(ctx, responseChan)
		return responseChan, nil
	}

	// Create response channel
	responseChan := make(chan *schemas.UnifAIStreamChunk, schemas.DefaultStreamBufferSize)

	providerUtils.SetStreamIdleTimeoutIfEmpty(ctx, provider.networkConfig.StreamIdleTimeoutInSeconds)

	// Start streaming in a goroutine
	go func() {
		defer providerUtils.EnsureStreamFinalizerCalled(ctx, postHookSpanFinalizer)
		defer func() {
			if ctx.Err() == context.Canceled {
				providerUtils.HandleStreamCancellation(ctx, postHookRunner, responseChan, provider.logger, postHookSpanFinalizer, jsonBody)
			} else if ctx.Err() == context.DeadlineExceeded {
				providerUtils.HandleStreamTimeout(ctx, postHookRunner, responseChan, provider.logger, postHookSpanFinalizer, jsonBody)
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
		stopCancellation := providerUtils.SetupStreamCancellation(ctx, resp.BodyStream(), provider.logger)
		defer stopCancellation()

		sseReader := providerUtils.GetSSEDataReader(ctx, reader)
		chunkIndex := -1
		usage := &schemas.TranscriptionUsage{}
		lastChunkTime := startTime

		var fullTranscriptionText string

		for {
			// If context was cancelled/timed out, let defer handle it
			if ctx.Err() != nil {
				return
			}

			data, readErr := sseReader.ReadDataLine()
			if readErr != nil {
				// Recheck context cancellation
				if ctx.Err() != nil {
					return
				}
				if readErr != io.EOF {
					ctx.SetValue(schemas.UnifAIContextKeyStreamEndIndicator, true)
					provider.logger.Warn("Error reading stream: %v", readErr)
					providerUtils.ProcessAndSendError(ctx, postHookRunner, readErr, responseChan, provider.logger, postHookSpanFinalizer)
					return
				}
				break
			}

			jsonData := data

			// Process chunk using shared function.
			geminiResponse, err := processGeminiStreamChunk(jsonData)
			if err != nil {
				if strings.Contains(err.Error(), "gemini api error") {
					unifaiErr := &schemas.UnifAIError{
						Type:           schemas.Ptr("gemini_api_error"),
						IsUnifAIError: false,
						Error: &schemas.ErrorField{
							Message: err.Error(),
							Error:   err,
						},
					}
					ctx.SetValue(schemas.UnifAIContextKeyStreamEndIndicator, true)
					providerUtils.ProcessAndSendUnifAIError(ctx, postHookRunner, unifaiErr, responseChan, provider.logger, postHookSpanFinalizer)
					return
				}
				provider.logger.Warn("Failed to process chunk: %v", err)
				continue
			}

			// Extract text from Gemini response for regular chunks
			var deltaText string
			if len(geminiResponse.Candidates) > 0 && geminiResponse.Candidates[0].Content != nil {
				if len(geminiResponse.Candidates[0].Content.Parts) > 0 {
					var sb strings.Builder
					for _, p := range geminiResponse.Candidates[0].Content.Parts {
						if p.Text != "" {
							sb.WriteString(p.Text)
						}
					}
					if sb.Len() > 0 {
						deltaText = sb.String()
						fullTranscriptionText += deltaText
					}
				}
			}

			// Check if this is the final chunk (has finishReason)
			if len(geminiResponse.Candidates) > 0 && (geminiResponse.Candidates[0].FinishReason != "" || geminiResponse.UsageMetadata != nil) {
				// Extract usage metadata from Gemini response
				inputTokens, outputTokens, totalTokens := extractGeminiUsageMetadata(geminiResponse)
				usage.InputTokens = schemas.Ptr(inputTokens)
				usage.OutputTokens = schemas.Ptr(outputTokens)
				usage.TotalTokens = schemas.Ptr(totalTokens)
			}

			// Only send response if we have actual text content
			if deltaText != "" {
				chunkIndex++

				// Create UnifAI transcription response for streaming
				response := &schemas.UnifAITranscriptionStreamResponse{
					Type:  schemas.TranscriptionStreamResponseTypeDelta,
					Delta: &deltaText, // Delta text for this chunk
					ExtraFields: schemas.UnifAIResponseExtraFields{
						ChunkIndex: chunkIndex,
						Latency:    time.Since(lastChunkTime).Milliseconds(),
					},
				}
				lastChunkTime = time.Now()

				if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
					response.ExtraFields.RawResponse = jsonData
				}

				// Process response through post-hooks and send to channel
				providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetUnifAIResponseForStreamResponse(nil, nil, nil, nil, response, nil), responseChan, postHookSpanFinalizer)
			}
		}
		response := &schemas.UnifAITranscriptionStreamResponse{
			Type: schemas.TranscriptionStreamResponseTypeDone,
			Text: fullTranscriptionText,
			Usage: &schemas.TranscriptionUsage{
				Type:         "tokens",
				InputTokens:  usage.InputTokens,
				OutputTokens: usage.OutputTokens,
				TotalTokens:  usage.TotalTokens,
			},
			ExtraFields: schemas.UnifAIResponseExtraFields{
				ChunkIndex: chunkIndex + 1,
				Latency:    time.Since(startTime).Milliseconds(),
			},
		}

		// Set raw request if enabled
		if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
			providerUtils.ParseAndSetRawRequest(&response.ExtraFields, jsonBody)
		}
		ctx.SetValue(schemas.UnifAIContextKeyStreamEndIndicator, true)
		providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetUnifAIResponseForStreamResponse(nil, nil, nil, nil, response, nil), responseChan, postHookSpanFinalizer)

	}()

	return responseChan, nil
}

// ImageGeneration performs an image generation request to the Gemini API.
func (provider *GeminiProvider) ImageGeneration(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIImageGenerationRequest) (*schemas.UnifAIImageGenerationResponse, *schemas.UnifAIError) {
	// Check if image gen is allowed for this provider
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.ImageGenerationRequest); err != nil {
		return nil, err
	}

	// check for imagen models
	if schemas.IsImagenModel(request.Model) {
		return provider.handleImagenImageGeneration(ctx, key, request)
	}
	// Prepare body
	jsonData, unifaiErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToGeminiImageGenerationRequest(request), nil
		})
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Use common request function
	geminiResponse, rawResponse, latency, providerResponseHeaders, unifaiErr := provider.completeRequest(ctx, request.Model, key, jsonData, ":generateContent")
	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.UnifAIContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if unifaiErr != nil {
		return nil, providerUtils.EnrichError(ctx, unifaiErr, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Large response mode: return lightweight response with metadata only
	if isLargeResp, _ := ctx.Value(schemas.UnifAIContextKeyLargeResponseMode).(bool); isLargeResp {
		return &schemas.UnifAIImageGenerationResponse{
			ExtraFields: schemas.UnifAIResponseExtraFields{
				Latency:                 latency.Milliseconds(),
				ProviderResponseHeaders: providerResponseHeaders,
			},
		}, nil
	}

	response, unifaiErr := geminiResponse.ToUnifAIImageGenerationResponse()
	if unifaiErr != nil {
		return nil, unifaiErr
	}
	if response == nil {
		return nil, providerUtils.NewUnifAIOperationError(
			"failed to convert Gemini image generation response",
			fmt.Errorf("ToUnifAIImageGenerationResponse returned nil response"),
		)
	}

	// Set ExtraFields
	response.ExtraFields.Latency = latency.Milliseconds()
	response.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		providerUtils.ParseAndSetRawRequest(&response.ExtraFields, jsonData)
	}

	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		response.ExtraFields.RawResponse = rawResponse
	}

	return response, nil
}

// handleImagenImageGeneration handles Imagen model requests using Vertex AI endpoint with API key auth
func (provider *GeminiProvider) handleImagenImageGeneration(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIImageGenerationRequest) (*schemas.UnifAIImageGenerationResponse, *schemas.UnifAIError) {
	// Prepare Imagen request body
	jsonData, unifaiErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToImagenImageGenerationRequest(request), nil
		})
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	baseURL := provider.networkConfig.BaseURL + providerUtils.GetPathFromContext(ctx, "/models/"+request.Model+":predict")
	// Create HTTP request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	respOwned := true
	defer func() {
		if respOwned {
			fasthttp.ReleaseResponse(resp)
		}
	}()

	// Set headers
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.SetRequestURI(baseURL)
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	req.SetBody(jsonData)

	value := key.Value.GetValue()
	if value != "" {
		req.Header.Set("x-goog-api-key", value)
	}

	// Send the request with optional large response streaming
	activeClient := providerUtils.PrepareResponseStreaming(ctx, provider.client, resp)
	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		return nil, providerUtils.EnrichError(ctx, parseGeminiError(resp), jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Parse Imagen response
	body, isLargeResp, decodeErr := providerUtils.FinalizeResponseWithLargeDetection(ctx, resp, provider.logger)
	if decodeErr != nil {
		return nil, decodeErr
	}
	if isLargeResp {
		respOwned = false
		return &schemas.UnifAIImageGenerationResponse{
			ExtraFields: schemas.UnifAIResponseExtraFields{
				Latency: latency.Milliseconds(),
			},
		}, nil
	}

	imagenResponse := GeminiImagenResponse{}
	rawRequest, rawResponse, unifaiErr := providerUtils.HandleProviderResponse(body, &imagenResponse, jsonData, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if unifaiErr != nil {
		return nil, unifaiErr
	}
	// Convert to UnifAI format
	response := imagenResponse.ToUnifAIImageGenerationResponse()
	response.ExtraFields.Latency = latency.Milliseconds()

	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		response.ExtraFields.RawRequest = rawRequest
	}

	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		response.ExtraFields.RawResponse = rawResponse
	}

	return response, nil
}

// ImageGenerationStream is not supported by the Gemini provider.
func (provider *GeminiProvider) ImageGenerationStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAIImageGenerationRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationStreamRequest, provider.GetProviderKey())
}

// ImageEdit handles image edit requests. For Imagen models, uses the Imagen edit API; otherwise uses Gemini generateContent.
func (provider *GeminiProvider) ImageEdit(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIImageEditRequest) (*schemas.UnifAIImageGenerationResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.ImageEditRequest); err != nil {
		return nil, err
	}

	// Handle Imagen models using :predict endpoint
	if schemas.IsImagenModel(request.Model) {
		jsonData, unifaiErr := providerUtils.CheckContextAndGetRequestBody(
			ctx,
			request,
			func() (providerUtils.RequestBodyWithExtraParams, error) {
				return ToImagenImageEditRequest(request), nil
			})
		if unifaiErr != nil {
			return nil, unifaiErr
		}

		baseURL := provider.networkConfig.BaseURL + providerUtils.GetPathFromContext(ctx, "/models/"+request.Model+":predict")
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()
		defer fasthttp.ReleaseRequest(req)
		imagenRespOwned := true
		defer func() {
			if imagenRespOwned {
				fasthttp.ReleaseResponse(resp)
			}
		}()

		providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
		req.SetRequestURI(baseURL)
		req.Header.SetMethod(http.MethodPost)
		req.Header.SetContentType("application/json")
		req.SetBody(jsonData)

		if value := key.Value.GetValue(); value != "" {
			req.Header.Set("x-goog-api-key", value)
		}

		activeClient := providerUtils.PrepareResponseStreaming(ctx, provider.client, resp)
		latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
		defer wait()
		if unifaiErr != nil {
			return nil, unifaiErr
		}

		if resp.StatusCode() != fasthttp.StatusOK {
			providerUtils.MaterializeStreamErrorBody(ctx, resp)
			return nil, providerUtils.EnrichError(ctx, parseGeminiError(resp), jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}

		body, isLargeResp, decodeErr := providerUtils.FinalizeResponseWithLargeDetection(ctx, resp, provider.logger)
		if decodeErr != nil {
			return nil, decodeErr
		}
		if isLargeResp {
			imagenRespOwned = false
			return &schemas.UnifAIImageGenerationResponse{
				ExtraFields: schemas.UnifAIResponseExtraFields{
					Latency: latency.Milliseconds(),
				},
			}, nil
		}

		imagenResponse := GeminiImagenResponse{}
		rawRequest, rawResponse, unifaiErr := providerUtils.HandleProviderResponse(body, &imagenResponse, jsonData, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
		if unifaiErr != nil {
			return nil, unifaiErr
		}

		response := imagenResponse.ToUnifAIImageGenerationResponse()
		response.ExtraFields.Latency = latency.Milliseconds()

		if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
			response.ExtraFields.RawRequest = rawRequest
		}
		if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
			response.ExtraFields.RawResponse = rawResponse
		}

		return response, nil
	}

	// Prepare body for non-Imagen models
	jsonData, unifaiErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToGeminiImageEditRequest(request), nil
		})
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Use common request function
	geminiResponse, rawResponse, latency, providerResponseHeaders, unifaiErr := provider.completeRequest(ctx, request.Model, key, jsonData, ":generateContent")
	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.UnifAIContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if unifaiErr != nil {
		return nil, providerUtils.EnrichError(ctx, unifaiErr, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Large response mode: return lightweight response with metadata only
	if isLargeResp, _ := ctx.Value(schemas.UnifAIContextKeyLargeResponseMode).(bool); isLargeResp {
		return &schemas.UnifAIImageGenerationResponse{
			ExtraFields: schemas.UnifAIResponseExtraFields{
				Latency:                 latency.Milliseconds(),
				ProviderResponseHeaders: providerResponseHeaders,
			},
		}, nil
	}

	response, unifaiErr := geminiResponse.ToUnifAIImageGenerationResponse()
	if unifaiErr != nil {
		return nil, unifaiErr
	}
	if response == nil {
		return nil, providerUtils.NewUnifAIOperationError(
			"failed to convert Gemini image edit response",
			fmt.Errorf("ToUnifAIImageGenerationResponse returned nil response"),
		)
	}

	// Set ExtraFields
	response.ExtraFields.Latency = latency.Milliseconds()
	response.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		providerUtils.ParseAndSetRawRequest(&response.ExtraFields, jsonData)
	}

	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		response.ExtraFields.RawResponse = rawResponse
	}

	return response, nil
}

// ImageEditStream is not supported by the Gemini provider.
func (provider *GeminiProvider) ImageEditStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAIImageEditRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditStreamRequest, provider.GetProviderKey())
}

// ImageVariation is not supported by the Gemini provider.
func (provider *GeminiProvider) ImageVariation(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIImageVariationRequest) (*schemas.UnifAIImageGenerationResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageVariationRequest, provider.GetProviderKey())
}

// VideoGeneration creates a video generation operation using Gemini's Veo models.
// Uses the POST /models/{model}:predictLongRunning endpoint.
func (provider *GeminiProvider) VideoGeneration(ctx *schemas.UnifAIContext, key schemas.Key, unifaiReq *schemas.UnifAIVideoGenerationRequest) (*schemas.UnifAIVideoGenerationResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.VideoGenerationRequest); err != nil {
		return nil, err
	}

	model := unifaiReq.Model

	jsonData, unifaiErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		unifaiReq,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToGeminiVideoGenerationRequest(unifaiReq)
		},
	)
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	jsonData = normalizeRawGenerateContentBody(ctx, jsonData)

	// Create HTTP request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	// Use Gemini's predictLongRunning endpoint for video generation
	req.SetRequestURI(provider.networkConfig.BaseURL + providerUtils.GetPathFromContext(ctx, "/models/"+model+":predictLongRunning"))
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}

	req.SetBody(jsonData)

	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.EnrichError(ctx, parseGeminiError(resp), jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// use handle provider response
	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, err), jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Parse response
	var operation GenerateVideosOperation
	rawRequest, rawResponse, unifaiErr := providerUtils.HandleProviderResponse(body, &operation, jsonData, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Convert to UnifAI response
	unifaiResp, unifaiErr := ToUnifAIVideoGenerationResponse(&operation, model)
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	unifaiResp.ID = providerUtils.AddVideoIDProviderSuffix(unifaiResp.ID, provider.GetProviderKey())

	unifaiResp.ExtraFields.Latency = latency.Milliseconds()

	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		unifaiResp.ExtraFields.RawRequest = rawRequest
	}
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		unifaiResp.ExtraFields.RawResponse = rawResponse
	}
	return unifaiResp, nil
}

// VideoRetrieve retrieves the status of a video generation operation.
// Uses the GET /operations/{operationName} endpoint.
func (provider *GeminiProvider) VideoRetrieve(ctx *schemas.UnifAIContext, key schemas.Key, unifaiReq *schemas.UnifAIVideoRetrieveRequest) (*schemas.UnifAIVideoGenerationResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.VideoRetrieveRequest); err != nil {
		return nil, err
	}

	operationID := unifaiReq.ID

	operationID = providerUtils.StripVideoIDProviderSuffix(operationID, provider.GetProviderKey())

	// Create HTTP request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	req.SetRequestURI(provider.networkConfig.BaseURL + providerUtils.GetPathFromContext(ctx, "/"+operationID))
	req.Header.SetMethod(http.MethodGet)
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}

	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		respBody := append([]byte(nil), resp.Body()...)
		return nil, providerUtils.EnrichError(ctx, parseGeminiError(resp), nil, respBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Parse response
	var operation GenerateVideosOperation
	_, rawResponse, unifaiErr := providerUtils.HandleProviderResponse(resp.Body(), &operation, nil, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	unifaiResp, unifaiErr := ToUnifAIVideoGenerationResponse(&operation, "")
	if unifaiErr != nil {
		return nil, unifaiErr
	}
	unifaiResp.ID = providerUtils.AddVideoIDProviderSuffix(unifaiResp.ID, provider.GetProviderKey())

	// Add extra fields
	unifaiResp.ExtraFields.Latency = latency.Milliseconds()

	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		unifaiResp.ExtraFields.RawResponse = rawResponse
	}

	return unifaiResp, nil
}

// VideoDownload downloads a video from Gemini.
func (provider *GeminiProvider) VideoDownload(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIVideoDownloadRequest) (*schemas.UnifAIVideoDownloadResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.VideoDownloadRequest); err != nil {
		return nil, err
	}
	if request == nil || request.ID == "" {
		return nil, providerUtils.NewUnifAIOperationError("video_id is required", nil)
	}
	// Retrieve operation first so download behavior follows retrieve status.
	unifaiVideoRetrieveRequest := &schemas.UnifAIVideoRetrieveRequest{
		Provider: request.Provider,
		ID:       request.ID,
	}
	videoResp, unifaiErr := provider.VideoRetrieve(ctx, key, unifaiVideoRetrieveRequest)
	if unifaiErr != nil {
		return nil, unifaiErr
	}
	if videoResp.Status != schemas.VideoStatusCompleted {
		return nil, providerUtils.NewUnifAIOperationError(
			fmt.Sprintf("video not ready, current status: %s", videoResp.Status),
			nil,
		)
	}
	if len(videoResp.Videos) == 0 {
		return nil, providerUtils.NewUnifAIOperationError("video URL not available", nil)
	}
	var content []byte
	contentType := "video/mp4"
	var latency time.Duration
	// Check if it's a data URL (base64-encoded video)
	if videoResp.Videos[0].Type == schemas.VideoOutputTypeBase64 && videoResp.Videos[0].Base64Data != nil {
		// Decode base64 content
		startTime := time.Now()
		decoded, err := base64.StdEncoding.DecodeString(*videoResp.Videos[0].Base64Data)
		if err != nil {
			return nil, providerUtils.NewUnifAIOperationError("failed to decode base64 video data", err)
		}
		content = decoded
		latency = time.Since(startTime)
		contentType = videoResp.Videos[0].ContentType
	} else if videoResp.Videos[0].Type == schemas.VideoOutputTypeURL && videoResp.Videos[0].URL != nil {
		// Regular URL - fetch from HTTP endpoint
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()
		defer fasthttp.ReleaseRequest(req)
		defer fasthttp.ReleaseResponse(resp)
		// Preserve custom headers and add API key for Gemini file download endpoint.
		providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
		req.SetRequestURI(*videoResp.Videos[0].URL)
		req.Header.SetMethod(http.MethodGet)
		if key.Value.GetValue() != "" {
			req.Header.Set("x-goog-api-key", key.Value.GetValue())
		}
		var unifaiErr *schemas.UnifAIError
		var wait func()
		latency, unifaiErr, wait = providerUtils.MakeRequestWithContextFollowRedirects(ctx, provider.client, req, resp, 5)
		defer wait()
		if unifaiErr != nil {
			return nil, unifaiErr
		}
		if resp.StatusCode() != fasthttp.StatusOK {
			// log full error
			provider.logger.Error("failed to download video: " + string(resp.Body()))
			return nil, providerUtils.SetErrorLatency(providerUtils.NewUnifAIOperationError(
				fmt.Sprintf("failed to download video: HTTP %d", resp.StatusCode()),
				nil,
			), latency)
		}
		body, err := providerUtils.CheckAndDecodeBody(resp)
		if err != nil {
			return nil, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, err)
		}
		contentType = string(resp.Header.ContentType())
		content = append([]byte(nil), body...)
	} else {
		return nil, providerUtils.NewUnifAIOperationError("invalid video output type", nil)
	}
	unifaiResp := &schemas.UnifAIVideoDownloadResponse{
		VideoID:     request.ID,
		Content:     content,
		ContentType: contentType,
	}

	unifaiResp.ExtraFields.Latency = latency.Milliseconds()

	return unifaiResp, nil
}

// VideoDelete is not supported by the Gemini provider.
func (provider *GeminiProvider) VideoDelete(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIVideoDeleteRequest) (*schemas.UnifAIVideoDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDeleteRequest, provider.GetProviderKey())
}

// VideoList is not supported by the Gemini provider.
func (provider *GeminiProvider) VideoList(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIVideoListRequest) (*schemas.UnifAIVideoListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoListRequest, provider.GetProviderKey())
}

// VideoRemix is not supported by the Gemini provider.
func (provider *GeminiProvider) VideoRemix(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIVideoRemixRequest) (*schemas.UnifAIVideoGenerationResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRemixRequest, provider.GetProviderKey())
}

// ==================== BATCH OPERATIONS ====================

// BatchCreate creates a new batch job for Gemini.
// Uses the asynchronous batchGenerateContent endpoint as per official documentation.
// Supports both inline requests and file-based input (via InputFileID).
func (provider *GeminiProvider) BatchCreate(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIBatchCreateRequest) (*schemas.UnifAIBatchCreateResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.BatchCreateRequest); err != nil {
		return nil, err
	}

	var jsonData []byte
	if rawBody, ok := providerUtils.CheckAndGetRawRequestBody(ctx, request); ok && len(rawBody) > 0 {
		jsonData = NormalizeRawGenerateContentRequestForCompatibility(rawBody)
	}

	// Validate that either InputFileID or Requests is provided, but not both
	hasFileInput := request.InputFileID != ""
	hasInlineRequests := len(request.Requests) > 0

	if len(jsonData) == 0 && !hasFileInput && !hasInlineRequests {
		return nil, providerUtils.NewUnifAIOperationError("either input_file_id or requests must be provided", nil)
	}

	if hasFileInput && hasInlineRequests {
		return nil, providerUtils.NewUnifAIOperationError("cannot specify both input_file_id and requests", nil)
	}

	if len(jsonData) == 0 {
		// Build the batch request with proper nested structure
		batchReq := &GeminiBatchCreateRequest{
			Batch: GeminiBatchConfig{
				DisplayName: fmt.Sprintf("unifai-batch-%d", time.Now().UnixNano()),
			},
		}

		if hasFileInput {
			// File-based input: use file_name in input_config
			fileID := request.InputFileID
			// Ensure file ID has the "files/" prefix
			if !strings.HasPrefix(fileID, "files/") {
				fileID = "files/" + fileID
			}
			batchReq.Batch.InputConfig = GeminiBatchInputConfig{
				FileName: fileID,
			}
		} else {
			// Inline requests: convert UnifAI requests to Gemini format
			geminiRequests := make([]GeminiBatchRequestItem, len(request.Requests))
			for i, unifaiItem := range request.Requests {
				geminiReq, err := ToGeminiBatchGenerateContentRequest(unifaiItem.Body)
				if err != nil {
					return nil, providerUtils.NewUnifAIOperationError("failed to convert batch request to gemini format", err)
				}

				geminiRequests[i] = GeminiBatchRequestItem{
					Request: geminiReq,
				}
				// Set metadata with custom_id
				if unifaiItem.CustomID != "" {
					geminiRequests[i].Metadata = &GeminiBatchMetadata{
						Key: unifaiItem.CustomID,
					}
				}
			}

			batchReq.Batch.InputConfig = GeminiBatchInputConfig{
				Requests: &GeminiBatchRequestsWrapper{
					Requests: geminiRequests,
				},
			}
		}

		var err error
		jsonData, err = providerUtils.MarshalSorted(batchReq)
		if err != nil {
			return nil, providerUtils.NewUnifAIOperationError(schemas.ErrProviderRequestMarshal, err)
		}
	}

	if updated, err := providerUtils.DeleteJSONField(jsonData, "fallbacks"); err == nil {
		jsonData = updated
	}

	// Create HTTP request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Build URL - use batchGenerateContent endpoint
	var model string
	if request.Model != nil {
		_, model = schemas.ParseModelString(*request.Model, schemas.Gemini)
	}
	// We default gemini 2.5 flash
	if model == "" {
		model = "gemini-2.5-flash"
	}
	url := fmt.Sprintf("%s/models/%s:batchGenerateContent", provider.networkConfig.BaseURL, model)

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.SetRequestURI(url)
	req.Header.SetMethod(http.MethodPost)
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}
	req.Header.SetContentType("application/json")
	req.SetBody(jsonData)

	// Make request
	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, providerUtils.EnrichError(ctx, unifaiErr, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.EnrichError(ctx, parseGeminiError(resp), jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, err), jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Parse the batch job response
	var geminiResp GeminiBatchJobResponse
	if err := sonic.Unmarshal(body, &geminiResp); err != nil {
		provider.logger.Error("gemini batch create unmarshal error: " + err.Error())
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseUnmarshal, err), jsonData, body, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}
	// Check for metadata
	if geminiResp.Metadata == nil {
		return nil, providerUtils.NewUnifAIOperationError("gemini batch response missing metadata", nil)
	}
	// Check for batch stats
	if geminiResp.Metadata.BatchStats == nil {
		return nil, providerUtils.NewUnifAIOperationError("gemini batch response missing batch stats", nil)
	}
	// Calculate request counts based on response
	totalRequests := geminiResp.Metadata.BatchStats.RequestCount
	completedCount := 0
	failedCount := 0

	// If results are already available (fast completion), count them
	outputFile, inlinedResponses := geminiBatchOutput(&geminiResp)
	if len(inlinedResponses) > 0 {
		for _, inlineResp := range inlinedResponses {
			if inlineResp.Error != nil {
				failedCount++
			} else if inlineResp.Response != nil {
				completedCount++
			}
		}
	} else {
		completedCount = geminiResp.Metadata.BatchStats.RequestCount - geminiResp.Metadata.BatchStats.PendingRequestCount
	}

	// Determine status
	status := ToUnifAIBatchStatus(geminiResp.Metadata.State)

	// If state is empty but we have results, it's completed
	if geminiResp.Metadata.State == "" && len(inlinedResponses) > 0 {
		status = schemas.BatchStatusCompleted
		completedCount = len(inlinedResponses) - failedCount
	}

	// Build response
	result := &schemas.UnifAIBatchCreateResponse{
		ID:            geminiResp.Metadata.Name,
		Object:        "batch",
		Endpoint:      string(request.Endpoint),
		Status:        status,
		CreatedAt:     parseGeminiTimestamp(geminiResp.Metadata.CreateTime),
		OperationName: &geminiResp.Metadata.Name,
		RequestCounts: schemas.BatchRequestCounts{
			Total:     totalRequests,
			Completed: completedCount,
			Failed:    failedCount,
		},
		ExtraFields: schemas.UnifAIResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}

	// Include InputFileID if file-based input was used
	if hasFileInput {
		result.InputFileID = request.InputFileID
	}

	// Include output file ID if results are in a file
	if outputFile != "" {
		result.OutputFileID = &outputFile
	}

	return result, nil
}

// batchListByKey lists batch jobs for Gemini for a single key.
func (provider *GeminiProvider) batchListByKey(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIBatchListRequest) (*schemas.UnifAIBatchListResponse, time.Duration, *schemas.UnifAIError) {
	// Create HTTP request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Build URL for listing batches
	baseURL := fmt.Sprintf("%s/batches", provider.networkConfig.BaseURL)
	values := url.Values{}
	if request.PageSize > 0 {
		values.Set("pageSize", fmt.Sprintf("%d", request.PageSize))
	} else if request.Limit > 0 {
		values.Set("pageSize", fmt.Sprintf("%d", request.Limit))
	}
	if request.PageToken != nil && *request.PageToken != "" {
		values.Set("pageToken", *request.PageToken)
	}
	requestURL := baseURL
	if encodedValues := values.Encode(); encodedValues != "" {
		requestURL += "?" + encodedValues
	}

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.SetRequestURI(requestURL)
	req.Header.SetMethod(http.MethodGet)
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}
	req.Header.SetContentType("application/json")

	// Make request
	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, latency, unifaiErr
	}

	// Handle error response - if listing is not supported, return empty list
	if resp.StatusCode() != fasthttp.StatusOK {
		// If 404 or method not allowed, batch listing may not be available
		if resp.StatusCode() == fasthttp.StatusNotFound || resp.StatusCode() == fasthttp.StatusMethodNotAllowed {
			provider.logger.Debug("gemini batch list not available, returning empty list")
			return &schemas.UnifAIBatchListResponse{
				Object:  "list",
				Data:    []schemas.UnifAIBatchRetrieveResponse{},
				HasMore: false,
				ExtraFields: schemas.UnifAIResponseExtraFields{
					Latency: latency.Milliseconds(),
				},
			}, latency, nil
		}
		return nil, latency, providerUtils.SetErrorLatency(parseGeminiError(resp), latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, latency, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, err)
	}

	var geminiResp GeminiBatchListResponse
	if err := sonic.Unmarshal(body, &geminiResp); err != nil {
		return nil, latency, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseUnmarshal, err)
	}

	// Convert to UnifAI format
	data := make([]schemas.UnifAIBatchRetrieveResponse, 0, len(geminiResp.Operations))
	for _, batch := range geminiResp.Operations {
		data = append(data, schemas.UnifAIBatchRetrieveResponse{
			// Full name (batches/<id>), matching create/retrieve so the id is stable.
			ID:            batch.Name,
			Object:        "batch",
			Status:        ToUnifAIBatchStatus(batch.Metadata.State),
			CreatedAt:     parseGeminiTimestamp(batch.Metadata.CreateTime),
			OperationName: &batch.Name,
			ExtraFields:   schemas.UnifAIResponseExtraFields{},
		})
	}

	hasMore := geminiResp.NextPageToken != ""
	var nextCursor *string
	if hasMore {
		nextCursor = &geminiResp.NextPageToken
	}

	return &schemas.UnifAIBatchListResponse{
		Object:     "list",
		Data:       data,
		HasMore:    hasMore,
		NextCursor: nextCursor,
		ExtraFields: schemas.UnifAIResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}, latency, nil
}

// BatchList lists batch jobs for Gemini across all provided keys.
// Note: The consumer API may have limited list functionality.
// BatchList lists batch jobs using serial pagination across keys.
// Exhausts all pages from one key before moving to the next.
func (provider *GeminiProvider) BatchList(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAIBatchListRequest) (*schemas.UnifAIBatchListResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.BatchListRequest); err != nil {
		return nil, err
	}

	if len(keys) == 0 {
		return nil, providerUtils.NewUnifAIOperationError("no keys provided for batch list", nil)
	}

	// Initialize serial pagination helper (Gemini uses PageToken for pagination)
	helper, err := providerUtils.NewSerialListHelper(keys, request.PageToken, provider.logger, true)
	if err != nil {
		return nil, providerUtils.NewUnifAIOperationError("invalid pagination cursor", err)
	}

	// Get current key to query
	key, nativeCursor, ok := helper.GetCurrentKey()
	if !ok {
		// All keys exhausted
		return &schemas.UnifAIBatchListResponse{
			Object:  "list",
			Data:    []schemas.UnifAIBatchRetrieveResponse{},
			HasMore: false,
		}, nil
	}

	// Create a modified request with the native cursor
	modifiedRequest := *request
	if nativeCursor != "" {
		modifiedRequest.PageToken = &nativeCursor
	} else {
		modifiedRequest.PageToken = nil
	}

	// Call the single-key helper
	resp, latency, unifaiErr := provider.batchListByKey(ctx, key, &modifiedRequest)
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Determine native cursor for next page
	nativeNextCursor := ""
	if resp.NextCursor != nil {
		nativeNextCursor = *resp.NextCursor
	}

	// Build cursor for next request
	nextCursor, hasMore := helper.BuildNextCursor(resp.HasMore, nativeNextCursor)

	result := &schemas.UnifAIBatchListResponse{
		Object:  "list",
		Data:    resp.Data,
		HasMore: hasMore,
		ExtraFields: schemas.UnifAIResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}
	if nextCursor != "" {
		result.NextCursor = &nextCursor
	}

	return result, nil
}

// batchRetrieveByKey retrieves a specific batch job for Gemini for a single key.
func (provider *GeminiProvider) batchRetrieveByKey(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIBatchRetrieveRequest) (*schemas.UnifAIBatchRetrieveResponse, *schemas.UnifAIError) {
	// Create HTTP request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Build URL - batch ID might be full resource name or just the ID
	batchID := request.BatchID
	var requestURL string
	if strings.HasPrefix(batchID, "batches/") {
		requestURL = fmt.Sprintf("%s/%s", provider.networkConfig.BaseURL, batchID)
	} else {
		requestURL = fmt.Sprintf("%s/batches/%s", provider.networkConfig.BaseURL, batchID)
	}

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.SetRequestURI(requestURL)
	req.Header.SetMethod(http.MethodGet)
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}
	req.Header.SetContentType("application/json")

	// Make request
	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.SetErrorLatency(parseGeminiError(resp), latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, err)
	}

	var geminiResp GeminiBatchJobResponse
	if err := sonic.Unmarshal(body, &geminiResp); err != nil {
		return nil, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseUnmarshal, err)
	}

	var completedCount, failedCount int

	completedCount = geminiResp.Metadata.BatchStats.RequestCount - geminiResp.Metadata.BatchStats.PendingRequestCount
	failedCount = completedCount - geminiResp.Metadata.BatchStats.SuccessfulRequestCount

	// Determine if job is done
	isDone := geminiResp.Metadata.State == GeminiBatchStateSucceeded ||
		geminiResp.Metadata.State == GeminiBatchStateFailed ||
		geminiResp.Metadata.State == GeminiBatchStateCancelled ||
		geminiResp.Metadata.State == GeminiBatchStateExpired

	result := &schemas.UnifAIBatchRetrieveResponse{
		ID:            geminiResp.Metadata.Name,
		Object:        "batch",
		Status:        ToUnifAIBatchStatus(geminiResp.Metadata.State),
		CreatedAt:     parseGeminiTimestamp(geminiResp.Metadata.CreateTime),
		OperationName: &geminiResp.Metadata.Name,
		Done:          &isDone,
		RequestCounts: schemas.BatchRequestCounts{
			Completed: completedCount,
			Total:     geminiResp.Metadata.BatchStats.RequestCount,
			Succeeded: geminiResp.Metadata.BatchStats.SuccessfulRequestCount,
			Pending:   geminiResp.Metadata.BatchStats.PendingRequestCount,
			Failed:    failedCount,
		},
		ExtraFields: schemas.UnifAIResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}

	// Surface the output file id for file-based batches so callers can download the
	// results via files.content. Inline batches carry no file; their responses are
	// exposed through BatchResults instead.
	if outputFile, _ := geminiBatchOutput(&geminiResp); outputFile != "" {
		result.OutputFileID = &outputFile
	}

	return result, nil
}

// BatchRetrieve retrieves a specific batch job for Gemini, trying each key until successful.
func (provider *GeminiProvider) BatchRetrieve(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAIBatchRetrieveRequest) (*schemas.UnifAIBatchRetrieveResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.BatchRetrieveRequest); err != nil {
		return nil, err
	}

	if request.BatchID == "" {
		return nil, providerUtils.NewUnifAIOperationError("batch_id is required", nil)
	}

	if len(keys) == 0 {
		return nil, providerUtils.NewUnifAIOperationError("no keys provided for batch retrieve", nil)
	}

	// Try each key until we find the batch
	var lastError *schemas.UnifAIError
	for _, key := range keys {
		resp, err := provider.batchRetrieveByKey(ctx, key, request)
		if err == nil {
			return resp, nil
		}
		lastError = err
	}

	// All keys failed, return the last error
	return nil, lastError
}

// batchCancelByKey cancels a batch job for Gemini for a single key.
func (provider *GeminiProvider) batchCancelByKey(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIBatchCancelRequest) (*schemas.UnifAIBatchCancelResponse, *schemas.UnifAIError) {
	// Create HTTP request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Build URL for cancel operation
	batchID := request.BatchID
	var requestURL string
	if strings.HasPrefix(batchID, "batches/") {
		requestURL = fmt.Sprintf("%s/%s:cancel", provider.networkConfig.BaseURL, batchID)
	} else {
		requestURL = fmt.Sprintf("%s/batches/%s:cancel", provider.networkConfig.BaseURL, batchID)
	}

	provider.logger.Debug("gemini batch cancel url: " + requestURL)
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.SetRequestURI(requestURL)
	req.Header.SetMethod(http.MethodPost)
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}
	req.Header.SetContentType("application/json")

	// Make request
	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Handle response
	if resp.StatusCode() != fasthttp.StatusOK {
		// If cancel is not supported, return appropriate status
		if resp.StatusCode() == fasthttp.StatusNotFound || resp.StatusCode() == fasthttp.StatusMethodNotAllowed {
			// 404 could mean batch not found or cancel not supported
			// Return the error instead of assuming completed
			return nil, providerUtils.SetErrorLatency(parseGeminiError(resp), latency)
		}
		return nil, providerUtils.SetErrorLatency(parseGeminiError(resp), latency)
	}

	now := time.Now().Unix()
	return &schemas.UnifAIBatchCancelResponse{
		ID:           request.BatchID,
		Object:       "batch",
		Status:       schemas.BatchStatusCancelling,
		CancellingAt: &now,
		ExtraFields: schemas.UnifAIResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}, nil
}

// BatchCancel cancels a batch job for Gemini, trying each key until successful.
// Note: Cancellation support depends on the API version and batch state.
func (provider *GeminiProvider) BatchCancel(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAIBatchCancelRequest) (*schemas.UnifAIBatchCancelResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.BatchCancelRequest); err != nil {
		return nil, err
	}

	if request.BatchID == "" {
		return nil, providerUtils.NewUnifAIOperationError("batch_id is required", nil)
	}

	if len(keys) == 0 {
		return nil, providerUtils.NewUnifAIOperationError("no keys provided for batch cancel", nil)
	}

	// Try each key until cancellation succeeds
	var lastError *schemas.UnifAIError
	for _, key := range keys {
		resp, err := provider.batchCancelByKey(ctx, key, request)
		if err == nil {
			return resp, nil
		}
		lastError = err
		provider.logger.Debug("BatchCancel failed for key %s: %v", key.Name, err.Error)
	}

	// All keys failed, return the last error
	return nil, lastError
}

// batchDeleteByKey deletes a batch job for Gemini for a single key.
// batches.delete indicates the client is no longer interested in the operation result.
// It does not cancel the operation. If the server doesn't support this method, it returns UNIMPLEMENTED.
func (provider *GeminiProvider) batchDeleteByKey(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIBatchDeleteRequest) (*schemas.UnifAIBatchDeleteResponse, *schemas.UnifAIError) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	batchID := request.BatchID
	var requestURL string
	if strings.HasPrefix(batchID, "batches/") {
		requestURL = fmt.Sprintf("%s/%s", provider.networkConfig.BaseURL, batchID)
	} else {
		requestURL = fmt.Sprintf("%s/batches/%s", provider.networkConfig.BaseURL, batchID)
	}

	provider.logger.Debug("gemini batch delete url: " + requestURL)
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.SetRequestURI(requestURL)
	req.Header.SetMethod(http.MethodDelete)
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}

	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	if resp.StatusCode() != fasthttp.StatusOK && resp.StatusCode() != fasthttp.StatusNoContent {
		return nil, providerUtils.SetErrorLatency(parseGeminiError(resp), latency)
	}

	return &schemas.UnifAIBatchDeleteResponse{
		ID:     request.BatchID,
		Object: "batch",
		Status: schemas.BatchStatusDeleted,
		ExtraFields: schemas.UnifAIResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}, nil
}

// BatchDelete deletes a batch job for Gemini, trying each key until successful.
// This indicates the client is no longer interested in the operation result.
// It does not cancel the operation. If the server doesn't support this method, it returns UNIMPLEMENTED.
func (provider *GeminiProvider) BatchDelete(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAIBatchDeleteRequest) (*schemas.UnifAIBatchDeleteResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.BatchDeleteRequest); err != nil {
		return nil, err
	}

	if request.BatchID == "" {
		return nil, providerUtils.NewUnifAIOperationError("batch_id is required", nil)
	}

	if len(keys) == 0 {
		return nil, providerUtils.NewUnifAIOperationError("no keys provided for batch delete", nil)
	}

	var lastError *schemas.UnifAIError
	for _, key := range keys {
		resp, err := provider.batchDeleteByKey(ctx, key, request)
		if err == nil {
			return resp, nil
		}
		lastError = err
		provider.logger.Debug("BatchDelete failed for key %s: %v", key.Name, err.Error)
	}

	return nil, lastError
}

// processGeminiStreamChunk processes a single chunk from Gemini streaming response
func processGeminiStreamChunk(jsonData []byte) (*GenerateContentResponse, error) {
	// Error chunks are rare; avoid a second decode in the common path.
	if bytes.Contains(jsonData, []byte(`"error"`)) {
		var errorCheck map[string]interface{}
		if err := sonic.Unmarshal(jsonData, &errorCheck); err == nil {
			if errValue, hasError := errorCheck["error"]; hasError {
				return nil, fmt.Errorf("gemini api error: %v", errValue)
			}
		}
	}

	var geminiResponse GenerateContentResponse
	if err := sonic.Unmarshal(jsonData, &geminiResponse); err != nil {
		return nil, fmt.Errorf("failed to parse Gemini stream response: %v", err)
	}

	return &geminiResponse, nil
}

func shouldSkipInlineDataForStreamingContext(ctx *schemas.UnifAIContext) bool {
	if ctx == nil {
		return false
	}
	if isLargePayload, ok := ctx.Value(schemas.UnifAIContextKeyLargePayloadMode).(bool); ok && isLargePayload {
		return true
	}
	if responseThreshold, ok := ctx.Value(schemas.UnifAIContextKeyLargeResponseThreshold).(int64); ok && responseThreshold > 0 {
		return true
	}
	return false
}

// extractSSEJSONData returns the JSON payload for SSE "data:" lines.
// It skips comments, control fields (event/id/retry), empty lines, and [DONE].
func extractSSEJSONData(line []byte) ([]byte, bool) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 || line[0] == ':' {
		return nil, false
	}
	if !bytes.HasPrefix(line, []byte("data:")) {
		return nil, false
	}
	data := bytes.TrimSpace(line[len("data:"):])
	if len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) {
		return nil, false
	}
	return data, true
}

// readNextSSEDataLine reads the next SSE `data:` line from a streaming response.
// It avoids scanner growth on oversized lines by reading fragments and discarding
// inlineData lines when skipInlineData is enabled.
func readNextSSEDataLine(reader *bufio.Reader, skipInlineData bool) ([]byte, error) {
	for {
		fragment, isPrefix, err := reader.ReadLine()
		if err != nil {
			return nil, err
		}

		trimmed := bytes.TrimSpace(fragment)
		if len(trimmed) == 0 || trimmed[0] == ':' || !bytes.HasPrefix(trimmed, []byte("data:")) {
			for isPrefix {
				_, isPrefix, err = reader.ReadLine()
				if err != nil {
					return nil, err
				}
			}
			continue
		}

		data := bytes.TrimLeft(trimmed[len("data:"):], " \t")
		if len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) {
			for isPrefix {
				_, isPrefix, err = reader.ReadLine()
				if err != nil {
					return nil, err
				}
			}
			continue
		}

		// Large mocked stream chunks encode binary inlineData; skip those lines entirely.
		if skipInlineData && bytes.Contains(data, []byte(`"inlineData"`)) {
			for isPrefix {
				_, isPrefix, err = reader.ReadLine()
				if err != nil {
					return nil, err
				}
			}
			continue
		}

		if !isPrefix {
			return append([]byte(nil), data...), nil
		}

		// For continued lines, bound accumulation to avoid unbounded memory growth.
		const maxJSONLineBytes = 512 * 1024
		collected := make([]byte, 0, len(data)+1024)
		collected = append(collected, data...)
		dropLine := false

		for isPrefix {
			fragment, isPrefix, err = reader.ReadLine()
			if err != nil {
				return nil, err
			}
			if skipInlineData && bytes.Contains(fragment, []byte(`"inlineData"`)) {
				dropLine = true
				continue
			}
			if dropLine || len(collected)+len(fragment) > maxJSONLineBytes {
				dropLine = true
				continue
			}
			collected = append(collected, fragment...)
		}

		if dropLine {
			continue
		}
		collected = bytes.TrimSpace(collected)
		if len(collected) == 0 || bytes.Equal(collected, []byte("[DONE]")) {
			continue
		}
		return collected, nil
	}
}

// batchResultsByKey retrieves batch results for Gemini for a single key.
func (provider *GeminiProvider) batchResultsByKey(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIBatchResultsRequest) (*schemas.UnifAIBatchResultsResponse, *schemas.UnifAIError) {
	// We need to get the full batch response with results, so make the API call directly
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Build URL
	batchID := request.BatchID
	var requestURL string
	if strings.HasPrefix(batchID, "batches/") {
		requestURL = fmt.Sprintf("%s/%s", provider.networkConfig.BaseURL, batchID)
	} else {
		requestURL = fmt.Sprintf("%s/batches/%s", provider.networkConfig.BaseURL, batchID)
	}

	provider.logger.Debug("gemini batch results url: " + requestURL)
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.SetRequestURI(requestURL)
	req.Header.SetMethod(http.MethodGet)
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}
	req.Header.SetContentType("application/json")

	// Make request
	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.SetErrorLatency(parseGeminiError(resp), latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, err)
	}

	var geminiResp GeminiBatchJobResponse
	if err := sonic.Unmarshal(body, &geminiResp); err != nil {
		return nil, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseUnmarshal, err)
	}

	// Check if batch is still processing
	if geminiResp.Metadata.State == GeminiBatchStatePending || geminiResp.Metadata.State == GeminiBatchStateRunning {
		return nil, providerUtils.NewUnifAIOperationError(
			fmt.Sprintf("batch %s is still processing (state: %s), results not yet available", request.BatchID, geminiResp.Metadata.State),
			nil,
		)
	}

	// Extract results - check for file-based results first, then inline responses
	var results []schemas.BatchResultItem
	var parseErrors []schemas.BatchError

	outputFile, inlinedResponses := geminiBatchOutput(&geminiResp)
	if outputFile != "" {
		// File-based results: download and parse the results file
		provider.logger.Debug("gemini batch results in file: " + outputFile)
		fileResults, fileParseErrors, unifaiErr := provider.downloadBatchResultsFile(ctx, key, outputFile)
		if unifaiErr != nil {
			return nil, unifaiErr
		}
		results = fileResults
		parseErrors = fileParseErrors
	} else if len(inlinedResponses) > 0 {
		// Inline results: extract from response.inlinedResponses
		results = make([]schemas.BatchResultItem, 0, len(inlinedResponses))
		for i, inlineResp := range inlinedResponses {
			results = append(results, geminiInlineResponseToBatchResultItem(inlineResp, fmt.Sprintf("request-%d", i)))
		}
	}

	// If no results found but job is complete, return info message
	if len(results) == 0 && (geminiResp.Metadata.State == GeminiBatchStateSucceeded || geminiResp.Metadata.State == GeminiBatchStateFailed) {
		results = []schemas.BatchResultItem{{
			CustomID: "info",
			Response: &schemas.BatchResultResponse{
				StatusCode: 200,
				Body: map[string]interface{}{
					"message": fmt.Sprintf("Batch completed with state: %s. No results available.", geminiResp.Metadata.State),
				},
			},
		}}
	}

	batchResultsResp := &schemas.UnifAIBatchResultsResponse{
		BatchID: request.BatchID,
		Results: results,
		ExtraFields: schemas.UnifAIResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}

	if len(parseErrors) > 0 {
		batchResultsResp.ExtraFields.ParseErrors = parseErrors
	}

	return batchResultsResp, nil
}

// BatchResults retrieves batch results for Gemini, trying each key until successful.
// Results are extracted from the batch response's inline responses
// (response.inlinedResponses) for inline batches, or downloaded from the responses
// file (response.responsesFile) for file-based batches.
func (provider *GeminiProvider) BatchResults(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAIBatchResultsRequest) (*schemas.UnifAIBatchResultsResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.BatchResultsRequest); err != nil {
		return nil, err
	}

	if request.BatchID == "" {
		return nil, providerUtils.NewUnifAIOperationError("batch_id is required", nil)
	}

	if len(keys) == 0 {
		return nil, providerUtils.NewUnifAIOperationError("no keys provided for batch results", nil)
	}

	// Try each key until we get results
	var lastError *schemas.UnifAIError
	for _, key := range keys {
		resp, err := provider.batchResultsByKey(ctx, key, request)
		if err == nil {
			return resp, nil
		}
		lastError = err
		provider.logger.Debug("BatchResults failed for key %s: %v", key.Name, err.Error.Message)
	}

	// All keys failed, return the last error
	return nil, lastError
}

// FileUpload uploads a file to Gemini.
func (provider *GeminiProvider) FileUpload(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIFileUploadRequest) (*schemas.UnifAIFileUploadResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.FileUploadRequest); err != nil {
		return nil, err
	}

	if len(request.File) == 0 {
		return nil, providerUtils.NewUnifAIOperationError("file content is required", nil)
	}

	// Create multipart request
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add file metadata as JSON
	metadataField, err := writer.CreateFormField("metadata")
	if err != nil {
		return nil, providerUtils.NewUnifAIOperationError("failed to create metadata field", err)
	}
	metadataJSON, err := providerUtils.SetJSONField([]byte(`{}`), "file.displayName", request.Filename)
	if err != nil {
		return nil, providerUtils.NewUnifAIOperationError("failed to marshal metadata", err)
	}
	contentType := ""
	if request.ContentType != nil {
		contentType = strings.TrimSpace(*request.ContentType)
	}
	if contentType != "" {
		metadataJSON, err = providerUtils.SetJSONField(metadataJSON, "file.mimeType", contentType)
		if err != nil {
			return nil, providerUtils.NewUnifAIOperationError("failed to marshal metadata", err)
		}
	}
	if _, err := metadataField.Write(metadataJSON); err != nil {
		return nil, providerUtils.NewUnifAIOperationError("failed to write metadata", err)
	}

	// Add file content
	filename := request.Filename
	if filename == "" {
		filename = "file.bin"
	}
	var part io.Writer
	if contentType != "" {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", multipart.FileContentDisposition("file", filename))
		header.Set("Content-Type", contentType)
		part, err = writer.CreatePart(header)
	} else {
		part, err = writer.CreateFormFile("file", filename)
	}
	if err != nil {
		return nil, providerUtils.NewUnifAIOperationError("failed to create form file", err)
	}
	if _, err := part.Write(request.File); err != nil {
		return nil, providerUtils.NewUnifAIOperationError("failed to write file content", err)
	}

	if err := writer.Close(); err != nil {
		return nil, providerUtils.NewUnifAIOperationError("failed to close multipart writer", err)
	}

	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Build URL - use upload endpoint
	baseURL := strings.Replace(provider.networkConfig.BaseURL, "/v1beta", "/upload/v1beta", 1)
	requestURL := fmt.Sprintf("%s/files", baseURL)

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.SetRequestURI(requestURL)
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType(writer.FormDataContentType())
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}
	req.SetBody(buf.Bytes())

	// Make request
	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK && resp.StatusCode() != fasthttp.StatusCreated {
		return nil, providerUtils.SetErrorLatency(parseGeminiError(resp), latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, err)
	}

	// Parse response - wrapped in "file" object
	var responseWrapper struct {
		File GeminiFileResponse `json:"file"`
	}
	if err := sonic.Unmarshal(body, &responseWrapper); err != nil {
		return nil, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseUnmarshal, err)
	}

	geminiResp := responseWrapper.File

	// Parse size
	var sizeBytes int64
	fmt.Sscanf(geminiResp.SizeBytes, "%d", &sizeBytes)

	// Parse creation time
	var createdAt int64
	if t, err := time.Parse(time.RFC3339, geminiResp.CreateTime); err == nil {
		createdAt = t.Unix()
	}

	// Parse expiration time
	var expiresAt *int64
	if geminiResp.ExpirationTime != "" {
		if t, err := time.Parse(time.RFC3339, geminiResp.ExpirationTime); err == nil {
			exp := t.Unix()
			expiresAt = &exp
		}
	}

	return &schemas.UnifAIFileUploadResponse{
		ID:             geminiResp.Name,
		Object:         "file",
		Bytes:          sizeBytes,
		CreatedAt:      createdAt,
		Filename:       geminiResp.DisplayName,
		Purpose:        request.Purpose,
		Status:         ToUnifAIFileStatus(geminiResp.State),
		StorageBackend: schemas.FileStorageAPI,
		StorageURI:     geminiResp.URI,
		ExpiresAt:      expiresAt,
		ExtraFields: schemas.UnifAIResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}, nil
}

// fileListByKey lists files from Gemini for a single key.
func (provider *GeminiProvider) fileListByKey(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIFileListRequest) (*schemas.UnifAIFileListResponse, time.Duration, *schemas.UnifAIError) {
	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Build URL with pagination
	requestURL := fmt.Sprintf("%s/files", provider.networkConfig.BaseURL)
	values := url.Values{}
	if request.Limit > 0 {
		values.Set("pageSize", fmt.Sprintf("%d", request.Limit))
	}
	if request.After != nil && *request.After != "" {
		values.Set("pageToken", *request.After)
	}
	if encodedValues := values.Encode(); encodedValues != "" {
		requestURL += "?" + encodedValues
	}

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.SetRequestURI(requestURL)
	req.Header.SetMethod(http.MethodGet)
	req.Header.SetContentType("application/json")
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}

	// Make request
	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, latency, unifaiErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, latency, providerUtils.SetErrorLatency(parseGeminiError(resp), latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, latency, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, err)
	}

	var geminiResp GeminiFileListResponse
	if err := sonic.Unmarshal(body, &geminiResp); err != nil {
		return nil, latency, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseUnmarshal, err)
	}

	// Convert to UnifAI response
	unifaiResp := &schemas.UnifAIFileListResponse{
		Object:  "list",
		Data:    make([]schemas.FileObject, len(geminiResp.Files)),
		HasMore: geminiResp.NextPageToken != "",
		ExtraFields: schemas.UnifAIResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}

	if geminiResp.NextPageToken != "" {
		unifaiResp.After = &geminiResp.NextPageToken
	}

	for i, file := range geminiResp.Files {
		var sizeBytes int64
		fmt.Sscanf(file.SizeBytes, "%d", &sizeBytes)

		var createdAt int64
		if t, err := time.Parse(time.RFC3339, file.CreateTime); err == nil {
			createdAt = t.Unix()
		}
		var updatedAt int64
		if t, err := time.Parse(time.RFC3339, file.UpdateTime); err == nil {
			updatedAt = t.Unix()
		}

		var expiresAt *int64
		if file.ExpirationTime != "" {
			if t, err := time.Parse(time.RFC3339, file.ExpirationTime); err == nil {
				exp := t.Unix()
				expiresAt = &exp
			}
		}

		unifaiResp.Data[i] = schemas.FileObject{
			ID:        file.Name,
			Object:    "file",
			Bytes:     sizeBytes,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
			Filename:  file.DisplayName,
			Purpose:   schemas.FilePurposeVision,
			Status:    ToUnifAIFileStatus(file.State),
			ExpiresAt: expiresAt,
		}
	}

	return unifaiResp, latency, nil
}

// FileList lists files from Gemini across all provided keys.
// FileList lists files using serial pagination across keys.
// Exhausts all pages from one key before moving to the next.
func (provider *GeminiProvider) FileList(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAIFileListRequest) (*schemas.UnifAIFileListResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.FileListRequest); err != nil {
		return nil, err
	}

	if len(keys) == 0 {
		return nil, providerUtils.NewUnifAIOperationError("no keys provided for file list", nil)
	}

	// Initialize serial pagination helper
	helper, err := providerUtils.NewSerialListHelper(keys, request.After, provider.logger, true)
	if err != nil {
		return nil, providerUtils.NewUnifAIOperationError("invalid pagination cursor", err)
	}

	// Get current key to query
	key, nativeCursor, ok := helper.GetCurrentKey()
	if !ok {
		// All keys exhausted
		return &schemas.UnifAIFileListResponse{
			Object:  "list",
			Data:    []schemas.FileObject{},
			HasMore: false,
		}, nil
	}

	// Create a modified request with the native cursor
	modifiedRequest := *request
	if nativeCursor != "" {
		modifiedRequest.After = &nativeCursor
	} else {
		modifiedRequest.After = nil
	}

	// Call the single-key helper
	resp, latency, unifaiErr := provider.fileListByKey(ctx, key, &modifiedRequest)
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Determine native cursor for next page
	nativeNextCursor := ""
	if resp.After != nil {
		nativeNextCursor = *resp.After
	}

	// Build cursor for next request
	nextCursor, hasMore := helper.BuildNextCursor(resp.HasMore, nativeNextCursor)

	result := &schemas.UnifAIFileListResponse{
		Object:  "list",
		Data:    resp.Data,
		HasMore: hasMore,
		ExtraFields: schemas.UnifAIResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}
	if nextCursor != "" {
		result.After = &nextCursor
	}

	return result, nil
}

// fileRetrieveByKey retrieves file metadata from Gemini for a single key.
func (provider *GeminiProvider) fileRetrieveByKey(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIFileRetrieveRequest) (*schemas.UnifAIFileRetrieveResponse, *schemas.UnifAIError) {
	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Build URL - file ID is the full resource name (e.g., "files/abc123")
	fileID := request.FileID
	if !strings.HasPrefix(fileID, "files/") {
		fileID = "files/" + fileID
	}
	requestURL := fmt.Sprintf("%s/%s", provider.networkConfig.BaseURL, fileID)

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.SetRequestURI(requestURL)
	req.Header.SetMethod(http.MethodGet)
	req.Header.SetContentType("application/json")
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}

	// Make request
	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.SetErrorLatency(parseGeminiError(resp), latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, err)
	}

	var geminiResp GeminiFileResponse
	if err := sonic.Unmarshal(body, &geminiResp); err != nil {
		return nil, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseUnmarshal, err)
	}

	var sizeBytes int64
	fmt.Sscanf(geminiResp.SizeBytes, "%d", &sizeBytes)

	var createdAt int64
	if t, err := time.Parse(time.RFC3339, geminiResp.CreateTime); err == nil {
		createdAt = t.Unix()
	}

	var updatedAt int64
	if t, err := time.Parse(time.RFC3339, geminiResp.UpdateTime); err == nil {
		updatedAt = t.Unix()
	}

	var expiresAt *int64
	if geminiResp.ExpirationTime != "" {
		if t, err := time.Parse(time.RFC3339, geminiResp.ExpirationTime); err == nil {
			exp := t.Unix()
			expiresAt = &exp
		}
	}

	return &schemas.UnifAIFileRetrieveResponse{
		ID:             geminiResp.Name,
		Object:         "file",
		Bytes:          sizeBytes,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
		Filename:       geminiResp.DisplayName,
		Purpose:        schemas.FilePurposeVision,
		Status:         ToUnifAIFileStatus(geminiResp.State),
		StorageBackend: schemas.FileStorageAPI,
		StorageURI:     geminiResp.URI,
		ExpiresAt:      expiresAt,
		ExtraFields: schemas.UnifAIResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}, nil
}

// FileRetrieve retrieves file metadata from Gemini, trying each key until successful.
func (provider *GeminiProvider) FileRetrieve(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAIFileRetrieveRequest) (*schemas.UnifAIFileRetrieveResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.FileRetrieveRequest); err != nil {
		return nil, err
	}

	if request.FileID == "" {
		return nil, providerUtils.NewUnifAIOperationError("file_id is required", nil)
	}

	if len(keys) == 0 {
		return nil, providerUtils.NewUnifAIOperationError("no keys provided for file retrieve", nil)
	}

	// Try each key until we find the file
	var lastError *schemas.UnifAIError
	for _, key := range keys {
		resp, err := provider.fileRetrieveByKey(ctx, key, request)
		if err == nil {
			return resp, nil
		}
		lastError = err
		provider.logger.Debug("FileRetrieve failed for key %s: %v", key.Name, err.Error)
	}

	// All keys failed, return the last error
	return nil, lastError
}

// fileDeleteByKey deletes a file from Gemini for a single key.
func (provider *GeminiProvider) fileDeleteByKey(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIFileDeleteRequest) (*schemas.UnifAIFileDeleteResponse, *schemas.UnifAIError) {
	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Build URL
	fileID := request.FileID
	if !strings.HasPrefix(fileID, "files/") {
		fileID = "files/" + fileID
	}
	requestURL := fmt.Sprintf("%s/%s", provider.networkConfig.BaseURL, fileID)

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.SetRequestURI(requestURL)
	req.Header.SetMethod(http.MethodDelete)
	req.Header.SetContentType("application/json")
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}

	// Make request
	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Handle error response - DELETE returns 200 with empty body on success
	if resp.StatusCode() != fasthttp.StatusOK && resp.StatusCode() != fasthttp.StatusNoContent {
		return nil, providerUtils.SetErrorLatency(parseGeminiError(resp), latency)
	}

	return &schemas.UnifAIFileDeleteResponse{
		ID:      request.FileID,
		Object:  "file",
		Deleted: true,
		ExtraFields: schemas.UnifAIResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}, nil
}

// FileDelete deletes a file from Gemini, trying each key until successful.
func (provider *GeminiProvider) FileDelete(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAIFileDeleteRequest) (*schemas.UnifAIFileDeleteResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.FileDeleteRequest); err != nil {
		return nil, err
	}

	if request.FileID == "" {
		return nil, providerUtils.NewUnifAIOperationError("file_id is required", nil)
	}

	if len(keys) == 0 {
		return nil, providerUtils.NewUnifAIOperationError("no keys provided for file delete", nil)
	}

	// Try each key until deletion succeeds
	var lastError *schemas.UnifAIError
	for _, key := range keys {
		resp, err := provider.fileDeleteByKey(ctx, key, request)
		if err == nil {
			return resp, nil
		}
		lastError = err
		provider.logger.Debug("FileDelete failed for key %s: %v", key.Name, err.Error)
	}

	// All keys failed, return the last error
	return nil, lastError
}

// FileContent downloads file content from Gemini.
// Note: Gemini Files API doesn't support direct content download.
// Files are accessed via their URI in API requests.
func (provider *GeminiProvider) FileContent(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAIFileContentRequest) (*schemas.UnifAIFileContentResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.FileContentRequest); err != nil {
		return nil, err
	}

	// Gemini doesn't support direct file content download
	// Files are referenced by their URI in requests
	return nil, providerUtils.NewUnifAIOperationError(
		"Gemini Files API doesn't support direct content download. Use the file URI in your requests instead.",
		nil,
	)
}

// CountTokens performs a token counting request to Gemini's countTokens endpoint.
func (provider *GeminiProvider) CountTokens(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIResponsesRequest) (*schemas.UnifAICountTokensResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.CountTokensRequest); err != nil {
		return nil, err
	}

	// Large payload mode only: stream original request bytes directly to upstream.
	// Feature-off behavior stays on the normal buffered request construction path.
	// This avoids early upstream completion before body upload (which can trigger
	// proxy broken-pipe 502s when a fronting LB is still forwarding a large body).
	isLargePayload := false
	if v, ok := ctx.Value(schemas.UnifAIContextKeyLargePayloadMode).(bool); ok && v {
		isLargePayload = true
	}

	var (
		jsonData   []byte
		unifaiErr *schemas.UnifAIError
	)
	if !isLargePayload {
		// Build JSON body from UnifAI request for normal path.
		jsonData, unifaiErr = providerUtils.CheckContextAndGetRequestBody(
			ctx,
			request,
			func() (providerUtils.RequestBodyWithExtraParams, error) {
				return ToGeminiResponsesRequest(ctx, request)
			},
		)
		if unifaiErr != nil {
			return nil, unifaiErr
		}

		jsonData = normalizeRawGenerateContentBody(ctx, jsonData)

		// Use sjson to delete fields directly from JSON bytes, preserving key ordering
		jsonData, _ = providerUtils.DeleteJSONField(jsonData, "toolConfig")
		jsonData, _ = providerUtils.DeleteJSONField(jsonData, "generationConfig")
		jsonData, _ = providerUtils.DeleteJSONField(jsonData, "systemInstruction")
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	if strings.TrimSpace(request.Model) == "" {
		return nil, providerUtils.NewUnifAIOperationError("model is required for Gemini count tokens request", fmt.Errorf("missing model"))
	}

	// Determine native model name (e.g., parse any provider prefix)
	_, model := schemas.ParseModelString(request.Model, schemas.Gemini)

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	path := fmt.Sprintf("/models/%s:countTokens", model)
	req.SetRequestURI(provider.networkConfig.BaseURL + providerUtils.GetPathFromContext(ctx, path))
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}
	usedLargePayloadBody := providerUtils.ApplyLargePayloadRequestBody(ctx, req)
	if !usedLargePayloadBody {
		req.SetBody(jsonData)
	}

	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, providerUtils.EnrichError(ctx, unifaiErr, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}
	// Keep passthrough request mode for countTokens, but fully drain the remaining
	// client upload before returning. This avoids proxy-layer broken-pipe 502s when
	// upstream responds early to lightweight count endpoints.
	// Example: under pressure, Caddy returned intermittent 502 with broken pipe.
	if usedLargePayloadBody {
		providerUtils.DrainLargePayloadRemainder(ctx)
	}

	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.EnrichError(ctx, parseGeminiError(resp), jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, err), jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	responseBody := append([]byte(nil), body...)

	geminiResponse := &GeminiCountTokensResponse{}
	rawRequest, rawResponse, unifaiErr := providerUtils.HandleProviderResponse(
		responseBody,
		geminiResponse,
		jsonData,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
	)
	if unifaiErr != nil {
		return nil, providerUtils.EnrichError(ctx, unifaiErr, jsonData, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	response := geminiResponse.ToUnifAICountTokensResponse(request.Model)

	// Set ExtraFields
	response.ExtraFields.Latency = latency.Milliseconds()

	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		response.ExtraFields.RawResponse = rawResponse
	}

	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		response.ExtraFields.RawRequest = rawRequest
	}

	return response, nil
}

// Compaction is not supported by the Gemini provider.
func (provider *GeminiProvider) Compaction(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAICompactionRequest) (*schemas.UnifAICompactionResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CompactionRequest, provider.GetProviderKey())
}

// ContainerCreate is not supported by the Gemini provider.
func (provider *GeminiProvider) ContainerCreate(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIContainerCreateRequest) (*schemas.UnifAIContainerCreateResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerCreateRequest, provider.GetProviderKey())
}

// ContainerList is not supported by the Gemini provider.
func (provider *GeminiProvider) ContainerList(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerListRequest) (*schemas.UnifAIContainerListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerListRequest, provider.GetProviderKey())
}

// ContainerRetrieve is not supported by the Gemini provider.
func (provider *GeminiProvider) ContainerRetrieve(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerRetrieveRequest) (*schemas.UnifAIContainerRetrieveResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerRetrieveRequest, provider.GetProviderKey())
}

// ContainerDelete is not supported by the Gemini provider.
func (provider *GeminiProvider) ContainerDelete(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerDeleteRequest) (*schemas.UnifAIContainerDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerDeleteRequest, provider.GetProviderKey())
}

// ContainerFileCreate is not supported by the Gemini provider.
func (provider *GeminiProvider) ContainerFileCreate(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIContainerFileCreateRequest) (*schemas.UnifAIContainerFileCreateResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileCreateRequest, provider.GetProviderKey())
}

// ContainerFileList is not supported by the Gemini provider.
func (provider *GeminiProvider) ContainerFileList(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerFileListRequest) (*schemas.UnifAIContainerFileListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileListRequest, provider.GetProviderKey())
}

// ContainerFileRetrieve is not supported by the Gemini provider.
func (provider *GeminiProvider) ContainerFileRetrieve(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerFileRetrieveRequest) (*schemas.UnifAIContainerFileRetrieveResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileRetrieveRequest, provider.GetProviderKey())
}

// ContainerFileContent is not supported by the Gemini provider.
func (provider *GeminiProvider) ContainerFileContent(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerFileContentRequest) (*schemas.UnifAIContainerFileContentResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileContentRequest, provider.GetProviderKey())
}

// ContainerFileDelete is not supported by the Gemini provider.
func (provider *GeminiProvider) ContainerFileDelete(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerFileDeleteRequest) (*schemas.UnifAIContainerFileDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileDeleteRequest, provider.GetProviderKey())
}

func (provider *GeminiProvider) Passthrough(
	ctx *schemas.UnifAIContext,
	key schemas.Key,
	req *schemas.UnifAIPassthroughRequest,
) (*schemas.UnifAIPassthroughResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.PassthroughRequest); err != nil {
		return nil, err
	}
	url := provider.networkConfig.BaseURL + req.Path
	if req.RawQuery != "" {
		url += "?" + req.RawQuery
	}

	url = strings.Replace(url, "v1beta/upload/v1beta", "upload/v1beta", 1)

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
		fasthttpReq.Header.Set("x-goog-api-key", key.Value.GetValue())
	}

	fasthttpReq.SetBody(req.Body)

	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, fasthttpReq, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	headers := providerUtils.ExtractPassthroughProviderResponseHeaders(resp)
	ctx.SetValue(schemas.UnifAIContextKeyProviderResponseHeaders, headers)

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewUnifAIOperationError("failed to decode response body", err)
	}

	var passthroughUsage *schemas.UnifAIPassthroughUsage
	if resp.StatusCode() >= 200 && resp.StatusCode() < 300 {
		passthroughUsage = ExtractGeminiPassthroughUsage(req.Path, req.Body, body)
	}

	unifaiResponse := &schemas.UnifAIPassthroughResponse{
		StatusCode: resp.StatusCode(),
		Headers:    headers,
		Body:       body,
		ExtraFields: schemas.UnifAIResponseExtraFields{
			Latency:                 latency.Milliseconds(),
			ProviderResponseHeaders: headers,
			PassthroughPath:         req.Path,
		},
		PassthroughUsage: passthroughUsage,
	}

	return unifaiResponse, nil
}

func (provider *GeminiProvider) PassthroughStream(
	ctx *schemas.UnifAIContext,
	postHookRunner schemas.PostHookRunner,
	postHookSpanFinalizer func(context.Context),
	key schemas.Key,
	req *schemas.UnifAIPassthroughRequest,
) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.PassthroughStreamRequest); err != nil {
		return nil, err
	}

	url := provider.networkConfig.BaseURL + req.Path
	if req.RawQuery != "" {
		url += "?" + req.RawQuery
	}

	url = strings.Replace(url, "v1beta/upload/v1beta", "upload/v1beta", 1)

	startTime := time.Now()

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

	if key.Value.GetValue() != "" {
		fasthttpReq.Header.Set("x-goog-api-key", key.Value.GetValue())
	}

	fasthttpReq.Header.Set("Connection", "close")

	fasthttpReq.SetBody(req.Body)

	activeClient := providerUtils.PrepareResponseStreaming(ctx, provider.streamingClient, resp)
	err := activeClient.Do(fasthttpReq, resp)
	latency := time.Since(startTime)
	if err != nil {
		providerUtils.ReleaseStreamingResponse(ctx, resp)
		if errors.Is(err, context.Canceled) {
			return nil, providerUtils.SetErrorLatency(&schemas.UnifAIError{
				IsUnifAIError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   err,
				},
			}, latency)
		}
		if errors.Is(err, fasthttp.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
			return nil, providerUtils.SetErrorLatency(providerUtils.NewUnifAITimeoutError(schemas.ErrProviderRequestTimedOut, err), latency)
		}
		return nil, providerUtils.SetErrorLatency(providerUtils.NewUnifAIUpstreamConnectionError(schemas.ErrProviderDoRequest, err), latency)
	}

	headers := providerUtils.ExtractPassthroughProviderResponseHeaders(resp)
	ctx.SetValue(schemas.UnifAIContextKeyProviderResponseHeaders, headers)

	bodyStream := resp.BodyStream()
	if bodyStream == nil {
		providerUtils.ReleaseStreamingResponse(ctx, resp)
		return nil, providerUtils.NewUnifAIOperationError(
			"provider returned an empty stream body",
			fmt.Errorf("provider returned an empty stream body"),
		)
	}

	providerUtils.SetStreamIdleTimeoutIfEmpty(ctx, provider.networkConfig.StreamIdleTimeoutInSeconds)
	return providerUtils.StreamPassthrough(
		ctx, postHookRunner, postHookSpanFinalizer, resp, bodyStream,
		providerUtils.PassthroughStreamParams{
			StatusCode:          resp.StatusCode(),
			Headers:             headers,
			Path:                req.Path,
			RawRequest:          req.Body,
			CancellationBody:    providerUtils.PassthroughJSONBody(fasthttpReq, req.Body),
			StartTime:           startTime,
			UseTerminalDetector: true,
			Logger:              provider.logger,
			HasUsage:            HasGeminiPassthroughUsage,
			Observe: func(event []byte) *schemas.UnifAIPassthroughUsage {
				return ExtractGeminiPassthroughUsage(req.Path, req.Body, event)
			},
		},
	), nil
}
