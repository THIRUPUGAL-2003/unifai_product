// Package mistral implements the Mistral provider.
package mistral

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gateway/gateway/core/providers/openai"
	providerUtils "github.com/gateway/gateway/core/providers/utils"
	schemas "github.com/gateway/gateway/core/schemas"
	"github.com/valyala/fasthttp"
)

// MistralProvider implements the Provider interface for Mistral's API.
type MistralProvider struct {
	logger               schemas.Logger        // Logger for provider operations
	client               *fasthttp.Client      // HTTP client for unary API requests (ReadTimeout bounds overall response)
	streamingClient      *fasthttp.Client      // HTTP client for streaming API requests (no ReadTimeout; idle governed by NewIdleTimeoutReader)
	networkConfig        schemas.NetworkConfig // Network configuration including extra headers
	customProviderConfig *schemas.CustomProviderConfig
	sendBackRawRequest   bool // Whether to include raw request in GatewayResponse
	sendBackRawResponse  bool // Whether to include raw response in GatewayResponse
}

// NewMistralProvider creates a new Mistral provider instance.
// It initializes the HTTP client with the provided configuration and sets up response pools.
// The client is configured with timeouts, concurrency limits, and optional proxy settings.
func NewMistralProvider(config *schemas.ProviderConfig, logger schemas.Logger) *MistralProvider {
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

	// Pre-warm response pools
	// for range config.ConcurrencyAndBufferSize.Concurrency {
	// 	mistralResponsePool.Put(&schemas.GatewayResponse{})
	// }

	// Configure proxy and retry policy
	client = providerUtils.ConfigureProxy(client, config.ProxyConfig, logger)
	client = providerUtils.ConfigureDialer(client, config.NetworkConfig.AllowPrivateNetwork)
	client = providerUtils.ConfigureTLS(client, config.NetworkConfig, logger)
	streamingClient := providerUtils.BuildStreamingClient(client)
	// Set default BaseURL if not provided
	if config.NetworkConfig.BaseURL == "" {
		config.NetworkConfig.BaseURL = "https://api.mistral.ai"
	}
	config.NetworkConfig.BaseURL = strings.TrimRight(config.NetworkConfig.BaseURL, "/")

	return &MistralProvider{
		logger:               logger,
		client:               client,
		streamingClient:      streamingClient,
		networkConfig:        config.NetworkConfig,
		customProviderConfig: config.CustomProviderConfig,
		sendBackRawRequest:   config.SendBackRawRequest,
		sendBackRawResponse:  config.SendBackRawResponse,
	}
}

// GetProviderKey returns the provider identifier for Mistral.
func (provider *MistralProvider) GetProviderKey() schemas.ModelProvider {
	return providerUtils.GetProviderName(schemas.Mistral, provider.customProviderConfig)
}

// listModelsByKey performs a list models request for a single key.
// Returns the response and latency, or an error if the request fails.
func (provider *MistralProvider) listModelsByKey(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayListModelsRequest) (*schemas.GatewayListModelsResponse, *schemas.GatewayError) {
	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	req.SetRequestURI(provider.networkConfig.BaseURL + providerUtils.GetPathFromContext(ctx, "/v1/models"))
	req.Header.SetMethod(http.MethodGet)
	req.Header.SetContentType("application/json")
	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	// Make request
	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		gatewayErr := providerUtils.SetErrorLatency(ParseMistralError(resp), latency)
		return nil, gatewayErr
	}

	// Copy response body before releasing
	responseBody := append([]byte(nil), resp.Body()...)

	// Parse Mistral's response
	var mistralResponse MistralListModelsResponse
	rawRequest, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(responseBody, &mistralResponse, nil, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	// Create final response
	response := mistralResponse.ToGatewayListModelsResponse(key.Models, key.BlacklistedModels, key.Aliases, request.Unfiltered)

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

// ListModels performs a list models request to Mistral's API.
// Requests are made concurrently for improved performance.
func (provider *MistralProvider) ListModels(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayListModelsRequest) (*schemas.GatewayListModelsResponse, *schemas.GatewayError) {
	return providerUtils.HandleMultipleListModelsRequests(
		ctx,
		keys,
		request,
		provider.listModelsByKey,
	)
}

// TextCompletion is not supported by the Mistral provider.
func (provider *MistralProvider) TextCompletion(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayTextCompletionRequest) (*schemas.GatewayTextCompletionResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionRequest, provider.GetProviderKey())
}

// TextCompletionStream performs a streaming text completion request to Mistral's API.
// It formats the request, sends it to Mistral, and processes the response.
// Returns a channel of GatewayStreamChunk objects or an error if the request fails.
func (provider *MistralProvider) TextCompletionStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayTextCompletionRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionStreamRequest, provider.GetProviderKey())
}

// normalizeChatRequestForConversion returns the request unchanged for the stock Mistral
// provider. For custom aliases (e.g. a provider registered as "custom-mistral" with
// BaseProviderType=Mistral), it returns a shallow copy with Provider set to schemas.Mistral
// so the shared OpenAI converter applies Mistral-specific compatibility (max_completion_tokens
// → max_tokens, tool_choice struct → "any"). The caller's request is never mutated.
func (provider *MistralProvider) normalizeChatRequestForConversion(request *schemas.GatewayChatRequest) *schemas.GatewayChatRequest {
	if request == nil || provider.customProviderConfig == nil || request.Provider == schemas.Mistral {
		return request
	}
	normalized := *request
	normalized.Provider = schemas.Mistral
	return &normalized
}

// ChatCompletion performs a chat completion request to the Mistral API.
func (provider *MistralProvider) ChatCompletion(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayChatRequest) (*schemas.GatewayChatResponse, *schemas.GatewayError) {
	return openai.HandleOpenAIChatCompletionRequest(
		ctx,
		provider.client,
		provider.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/v1/chat/completions"),
		provider.normalizeChatRequestForConversion(request),
		openai.BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		nil,
		ParseMistralError,
		nil,
		provider.logger,
	)
}

// ChatCompletionStream performs a streaming chat completion request to the Mistral API.
// It supports real-time streaming of responses using Server-Sent Events (SSE).
// Uses Mistral's OpenAI-compatible streaming format.
// Returns a channel containing GatewayStreamChunk objects representing the stream or an error if the request fails.
func (provider *MistralProvider) ChatCompletionStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayChatRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return openai.HandleOpenAIChatCompletionStreaming(
		ctx,
		provider.streamingClient,
		provider.networkConfig.BaseURL+"/v1/chat/completions",
		provider.normalizeChatRequestForConversion(request),
		openai.BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		provider.networkConfig.StreamIdleTimeoutInSeconds,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		postHookRunner,
		nil,
		nil,
		ParseMistralError,
		nil,
		nil,
		nil,
		provider.logger,
		postHookSpanFinalizer,
	)
}

// Responses performs a responses request to the Mistral API.
func (provider *MistralProvider) Responses(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayResponsesRequest) (*schemas.GatewayResponsesResponse, *schemas.GatewayError) {
	chatResponse, err := provider.ChatCompletion(ctx, key, request.ToChatRequest())
	if err != nil {
		return nil, err
	}

	response := chatResponse.ToGatewayResponsesResponse()

	return response, nil
}

// ResponsesStream performs a streaming responses request to the Mistral API.
func (provider *MistralProvider) ResponsesStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayResponsesRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	ctx.SetValue(schemas.GatewayContextKeyIsResponsesToChatCompletionFallback, true)
	return provider.ChatCompletionStream(
		ctx,
		postHookRunner,
		postHookSpanFinalizer,
		key,
		request.ToChatRequest(),
	)
}

// Embedding generates embeddings for the given input text(s) using the Mistral API.
// Supports Mistral's embedding models and returns a GatewayResponse containing the embedding(s).
func (provider *MistralProvider) Embedding(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayEmbeddingRequest) (*schemas.GatewayEmbeddingResponse, *schemas.GatewayError) {
	// Use the shared embedding request handler
	return openai.HandleOpenAIEmbeddingRequest(
		ctx,
		provider.client,
		provider.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/v1/embeddings"),
		request,
		openai.BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		provider.GetProviderKey(),
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		nil,
		provider.logger,
	)
}

// Speech is not supported by the Mistral provider.
func (provider *MistralProvider) Speech(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewaySpeechRequest) (*schemas.GatewaySpeechResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechRequest, provider.GetProviderKey())
}

// SpeechStream is not supported by the Mistral provider.
func (provider *MistralProvider) SpeechStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewaySpeechRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechStreamRequest, provider.GetProviderKey())
}

// Transcription performs an audio transcription request to the Mistral API.
// It creates a multipart form with the audio file and sends it to Mistral's transcription endpoint.
// Returns the transcribed text and metadata, or an error if the request fails.
func (provider *MistralProvider) Transcription(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayTranscriptionRequest) (*schemas.GatewayTranscriptionResponse, *schemas.GatewayError) {
	// Convert Gateway request to Mistral format
	mistralReq := ToMistralTranscriptionRequest(request)
	if mistralReq == nil {
		return nil, providerUtils.NewGatewayOperationError("transcription input is not provided", nil)
	}

	// Create multipart form body
	body, contentType, gatewayErr := createMistralTranscriptionMultipartBody(mistralReq, provider.GetProviderKey())
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	// Create HTTP request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Set extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	req.SetRequestURI(provider.networkConfig.BaseURL + providerUtils.GetPathFromContext(ctx, "/v1/audio/transcriptions"))
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType(contentType)
	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	req.SetBody(body.Bytes())

	// Make request
	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.SetErrorLatency(ParseMistralError(resp), latency)
	}

	responseBody, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewGatewayOperationError(schemas.ErrProviderResponseDecode, err)
	}

	// Check for empty response
	trimmed := strings.TrimSpace(string(responseBody))
	if len(trimmed) == 0 {
		return nil, &schemas.GatewayError{
			IsGatewayError: true,
			Error: &schemas.ErrorField{
				Message: schemas.ErrProviderResponseEmpty,
			},
		}
	}

	copiedResponseBody := append([]byte(nil), responseBody...)

	// Parse Mistral's transcription response
	var mistralResponse MistralTranscriptionResponse
	if err := sonic.Unmarshal(copiedResponseBody, &mistralResponse); err != nil {
		if providerUtils.IsHTMLResponse(resp, copiedResponseBody) {
			return nil, &schemas.GatewayError{
				IsGatewayError: false,
				Error: &schemas.ErrorField{
					Message: schemas.ErrProviderResponseHTML,
					Error:   errors.New(string(copiedResponseBody)),
				},
			}
		}
		return nil, providerUtils.NewGatewayOperationError(schemas.ErrProviderResponseUnmarshal, err)
	}

	// Convert to Gateway format
	response := mistralResponse.ToGatewayTranscriptionResponse()
	if response == nil {
		return nil, providerUtils.NewGatewayOperationError("failed to convert transcription response", nil)
	}

	// Set extra fields
	response.ExtraFields.Latency = latency.Milliseconds()

	// Set raw response if enabled
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		var rawResponse interface{}
		if err := sonic.Unmarshal(copiedResponseBody, &rawResponse); err == nil {
			response.ExtraFields.RawResponse = rawResponse
		}
	}

	return response, nil
}

// TranscriptionStream performs a streaming transcription request to Mistral's API.
// It creates a multipart form with the audio file and streams transcription events.
// Returns a channel of GatewayStreamChunk objects containing transcription deltas.
func (provider *MistralProvider) TranscriptionStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayTranscriptionRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	providerName := provider.GetProviderKey()

	// Convert Gateway request to Mistral format
	mistralReq := ToMistralTranscriptionRequest(request)
	if mistralReq == nil {
		return nil, providerUtils.NewGatewayOperationError("transcription input is not provided", nil)
	}
	mistralReq.Stream = schemas.Ptr(true)

	// Create multipart form body with stream=true
	body, contentType, gatewayErr := createMistralTranscriptionMultipartBody(mistralReq, providerName)
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	// Prepare headers for streaming
	headers := map[string]string{
		"Content-Type":  contentType,
		"Accept":        "text/event-stream",
		"Cache-Control": "no-cache",
	}

	if key.Value.GetValue() != "" {
		headers["Authorization"] = "Bearer " + key.Value.GetValue()
	}

	// Create HTTP request for streaming
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	resp.StreamBody = true
	defer fasthttp.ReleaseRequest(req)

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	req.Header.SetMethod(http.MethodPost)
	req.SetRequestURI(provider.networkConfig.BaseURL + providerUtils.GetPathFromContext(ctx, "/v1/audio/transcriptions"))

	// Set headers
	for headerKey, value := range headers {
		req.Header.Set(headerKey, value)
	}

	req.SetBody(body.Bytes())

	startTime := time.Now()
	// Make the request
	err := provider.streamingClient.Do(req, resp)
	latency := time.Since(startTime)
	if err != nil {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		if errors.Is(err, context.Canceled) {
			return nil, providerUtils.SetErrorLatency(&schemas.GatewayError{
				IsGatewayError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   err,
				},
			}, latency)
		}
		if errors.Is(err, fasthttp.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
			return nil, providerUtils.SetErrorLatency(providerUtils.NewGatewayTimeoutError(schemas.ErrProviderRequestTimedOut, err), latency)
		}
		// Request failed before the first response byte (server closed an idle/pooled connection,
		// broken pipe, connection refused, DNS failure, etc.). Surface as a retriable upstream
		// connection error (502) so executeRequestWithRetries honors max_retries, matching the
		// non-streaming path - see https://github.com/gateway/gateway/issues/4496.
		return nil, providerUtils.SetErrorLatency(providerUtils.NewGatewayUpstreamConnectionError(schemas.ErrProviderDoRequest, err), latency)
	}

	// Store provider response headers in context before status check so error responses also forward them
	ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	// Check for HTTP errors
	if resp.StatusCode() != fasthttp.StatusOK {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		return nil, providerUtils.SetErrorLatency(ParseMistralError(resp), latency)
	}

	// Large payload streaming passthrough — pipe raw upstream SSE to client
	if providerUtils.SetupStreamingPassthrough(ctx, resp) {
		responseChan := make(chan *schemas.GatewayStreamChunk)
		providerUtils.CloseStream(ctx, responseChan)
		return responseChan, nil
	}

	// Create response channel
	responseChan := make(chan *schemas.GatewayStreamChunk, schemas.DefaultStreamBufferSize)

	providerUtils.SetStreamIdleTimeoutIfEmpty(ctx, provider.networkConfig.StreamIdleTimeoutInSeconds)

	// Start streaming in a goroutine
	go func() {
		defer func() {
			if ctx.Err() == context.Canceled {
				providerUtils.HandleStreamCancellation(ctx, postHookRunner, responseChan, provider.logger, postHookSpanFinalizer, nil)
			} else if ctx.Err() == context.DeadlineExceeded {
				providerUtils.HandleStreamTimeout(ctx, postHookRunner, responseChan, provider.logger, postHookSpanFinalizer, nil)
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
		defer providerUtils.EnsureStreamFinalizerCalled(ctx, postHookSpanFinalizer)

		sseReader := providerUtils.GetSSEEventReader(ctx, reader)
		chunkIndex := -1

		lastChunkTime := startTime

		for {
			// If context was cancelled/timed out, let defer handle it
			if ctx.Err() != nil {
				return
			}

			eventType, eventDataBytes, readErr := sseReader.ReadEvent()
			if readErr != nil {
				// If context was cancelled/timed out, let defer handle it
				if ctx.Err() != nil {
					return
				}
				if readErr != io.EOF {
					ctx.SetValue(schemas.GatewayContextKeyStreamEndIndicator, true)
					provider.logger.Warn("Error reading stream: %v", readErr)
					providerUtils.ProcessAndSendError(ctx, postHookRunner, readErr, responseChan, provider.logger, postHookSpanFinalizer)
				}
				break
			}

			currentEvent := eventType
			currentData := string(eventDataBytes)
			if currentEvent == "" || currentData == "" {
				continue
			}

			chunkIndex++
			provider.processTranscriptionStreamEvent(ctx, postHookRunner, currentEvent, currentData, chunkIndex, startTime, &lastChunkTime, responseChan, postHookSpanFinalizer)
			// Break on terminal stream indicator (covers both done events and error events
			// that processTranscriptionStreamEvent signals via context).
			if ended, _ := ctx.Value(schemas.GatewayContextKeyStreamEndIndicator).(bool); ended {
				break
			}
		}
	}()

	return responseChan, nil
}

// processTranscriptionStreamEvent processes a single SSE event and sends it to the response channel.
func (provider *MistralProvider) processTranscriptionStreamEvent(
	ctx *schemas.GatewayContext,
	postHookRunner schemas.PostHookRunner,
	eventType string,
	jsonData string,
	chunkIndex int,
	startTime time.Time,
	lastChunkTime *time.Time,
	responseChan chan *schemas.GatewayStreamChunk,
	postHookSpanFinalizer func(context.Context),
) {
	// Skip empty data
	if strings.TrimSpace(jsonData) == "" {
		return
	}

	// Quick check for error field (allocation-free using sonic.GetFromString)
	if errorNode, _ := sonic.GetFromString(jsonData, "error"); errorNode.Exists() {
		// Only unmarshal when we know there's an error
		var gatewayErr schemas.GatewayError
		if err := sonic.UnmarshalString(jsonData, &gatewayErr); err == nil {
			if gatewayErr.Error != nil && gatewayErr.Error.Message != "" {
				ctx.SetValue(schemas.GatewayContextKeyStreamEndIndicator, true)
				providerUtils.ProcessAndSendGatewayError(ctx, postHookRunner, &gatewayErr, responseChan, provider.logger, postHookSpanFinalizer)
				return
			}
		}
	}

	// Parse the event data
	var eventData MistralTranscriptionStreamData
	if err := sonic.UnmarshalString(jsonData, &eventData); err != nil {
		provider.logger.Warn("Failed to parse stream event data: %v", err)
		return
	}

	// Create the stream event
	streamEvent := &MistralTranscriptionStreamEvent{
		Event: eventType,
		Data:  &eventData,
	}

	// Convert to Gateway format
	response := streamEvent.ToGatewayTranscriptionStreamResponse()
	if response == nil {
		return
	}

	// Set extra fields
	response.ExtraFields = schemas.GatewayResponseExtraFields{
		ChunkIndex: chunkIndex,
		Latency:    time.Since(*lastChunkTime).Milliseconds(),
	}
	*lastChunkTime = time.Now()

	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		response.ExtraFields.RawResponse = jsonData
	}

	// Check for done event (handle both "transcription.done" and "transcript.text.done")
	if MistralTranscriptionStreamEventType(eventType) == MistralTranscriptionStreamEventDone || eventType == "transcript.text.done" {
		response.ExtraFields.Latency = time.Since(startTime).Milliseconds()
		ctx.SetValue(schemas.GatewayContextKeyStreamEndIndicator, true)
		// Ensure response type is set to Done
		response.Type = schemas.TranscriptionStreamResponseTypeDone
	}

	providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetGatewayResponseForStreamResponse(nil, nil, nil, nil, response, nil), responseChan, postHookSpanFinalizer)
}

// Rerank is not supported by the Mistral provider.
func (provider *MistralProvider) Rerank(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayRerankRequest) (*schemas.GatewayRerankResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.RerankRequest, provider.GetProviderKey())
}

// OCR performs an OCR request to the Mistral API.
// It sends a JSON request to Mistral's OCR endpoint and returns the extracted content.
func (provider *MistralProvider) OCR(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayOCRRequest) (*schemas.GatewayOCRResponse, *schemas.GatewayError) {
	// Convert Gateway request to Mistral format
	mistralReq := ToMistralOCRRequest(request)
	if mistralReq == nil {
		return nil, providerUtils.NewGatewayOperationError("ocr request input is not provided", nil)
	}

	// Marshal request body
	requestBody, err := sonic.Marshal(mistralReq)
	if err != nil {
		return nil, providerUtils.NewGatewayOperationError(schemas.ErrProviderRequestMarshal, err)
	}

	// Merge extra params into JSON payload
	if len(mistralReq.ExtraParams) > 0 {
		requestBody, err = providerUtils.MergeExtraParamsIntoJSON(requestBody, mistralReq.ExtraParams)
		if err != nil {
			return nil, providerUtils.NewGatewayOperationError(schemas.ErrProviderRequestMarshal, err)
		}
	}

	// Create HTTP request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Set extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	req.SetRequestURI(provider.networkConfig.BaseURL + providerUtils.GetPathFromContext(ctx, "/v1/ocr"))
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	req.SetBody(requestBody)

	// Set raw request if enabled
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		request.RawRequestBody = requestBody
	}

	// Make request
	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.SetErrorLatency(ParseMistralError(resp), latency)
	}

	responseBody, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewGatewayOperationError(schemas.ErrProviderResponseDecode, err)
	}

	// Check for empty response
	trimmed := strings.TrimSpace(string(responseBody))
	if len(trimmed) == 0 {
		return nil, &schemas.GatewayError{
			IsGatewayError: true,
			Error: &schemas.ErrorField{
				Message: schemas.ErrProviderResponseEmpty,
			},
		}
	}

	copiedResponseBody := append([]byte(nil), responseBody...)

	// Parse Mistral's OCR response
	var mistralResponse MistralOCRResponse
	if err := sonic.Unmarshal(copiedResponseBody, &mistralResponse); err != nil {
		if providerUtils.IsHTMLResponse(resp, copiedResponseBody) {
			return nil, &schemas.GatewayError{
				IsGatewayError: false,
				Error: &schemas.ErrorField{
					Message: schemas.ErrProviderResponseHTML,
					Error:   errors.New(string(copiedResponseBody)),
				},
			}
		}
		return nil, providerUtils.NewGatewayOperationError(schemas.ErrProviderResponseUnmarshal, err)
	}

	// Convert to Gateway format
	response := mistralResponse.ToGatewayOCRResponse()
	if response == nil {
		return nil, providerUtils.NewGatewayOperationError("failed to convert ocr response", nil)
	}

	// Set extra fields
	response.ExtraFields.Latency = latency.Milliseconds()

	// Set raw response if enabled
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		var rawResponse interface{}
		if err := sonic.Unmarshal(copiedResponseBody, &rawResponse); err == nil {
			response.ExtraFields.RawResponse = rawResponse
		}
	}

	return response, nil
}

// BatchCreate is not supported by Mistral provider.
func (provider *MistralProvider) BatchCreate(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayBatchCreateRequest) (*schemas.GatewayBatchCreateResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCreateRequest, provider.GetProviderKey())
}

// BatchList is not supported by Mistral provider.
func (provider *MistralProvider) BatchList(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchListRequest) (*schemas.GatewayBatchListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchListRequest, provider.GetProviderKey())
}

// BatchRetrieve is not supported by Mistral provider.
func (provider *MistralProvider) BatchRetrieve(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchRetrieveRequest) (*schemas.GatewayBatchRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchRetrieveRequest, provider.GetProviderKey())
}

// BatchCancel is not supported by Mistral provider.
func (provider *MistralProvider) BatchCancel(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchCancelRequest) (*schemas.GatewayBatchCancelResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCancelRequest, provider.GetProviderKey())
}

// BatchDelete is not supported by Mistral provider.
func (provider *MistralProvider) BatchDelete(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchDeleteRequest) (*schemas.GatewayBatchDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchDeleteRequest, provider.GetProviderKey())
}

// BatchResults is not supported by Mistral provider.
func (provider *MistralProvider) BatchResults(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchResultsRequest) (*schemas.GatewayBatchResultsResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchResultsRequest, provider.GetProviderKey())
}

// FileUpload is not supported by Mistral provider.
func (provider *MistralProvider) FileUpload(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayFileUploadRequest) (*schemas.GatewayFileUploadResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileUploadRequest, provider.GetProviderKey())
}

// FileList is not supported by Mistral provider.
func (provider *MistralProvider) FileList(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayFileListRequest) (*schemas.GatewayFileListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileListRequest, provider.GetProviderKey())
}

// FileRetrieve is not supported by Mistral provider.
func (provider *MistralProvider) FileRetrieve(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayFileRetrieveRequest) (*schemas.GatewayFileRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileRetrieveRequest, provider.GetProviderKey())
}

// FileDelete is not supported by Mistral provider.
func (provider *MistralProvider) FileDelete(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayFileDeleteRequest) (*schemas.GatewayFileDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileDeleteRequest, provider.GetProviderKey())
}

// FileContent is not supported by Mistral provider.
func (provider *MistralProvider) FileContent(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayFileContentRequest) (*schemas.GatewayFileContentResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileContentRequest, provider.GetProviderKey())
}

// CountTokens is not supported by the Mistral provider.
func (provider *MistralProvider) CountTokens(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayResponsesRequest) (*schemas.GatewayCountTokensResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CountTokensRequest, provider.GetProviderKey())
}

// Compaction is not supported by the Mistral provider.
func (provider *MistralProvider) Compaction(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayCompactionRequest) (*schemas.GatewayCompactionResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CompactionRequest, provider.GetProviderKey())
}

// ImageGeneration is not supported by the Mistral provider.
func (provider *MistralProvider) ImageGeneration(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayImageGenerationRequest) (*schemas.GatewayImageGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationRequest, provider.GetProviderKey())
}

// ImageGenerationStream is not supported by the Mistral provider.
func (provider *MistralProvider) ImageGenerationStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayImageGenerationRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationStreamRequest, provider.GetProviderKey())
}

// ImageEdit is not supported by the Mistral provider.
func (provider *MistralProvider) ImageEdit(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayImageEditRequest) (*schemas.GatewayImageGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditRequest, provider.GetProviderKey())
}

// ImageEditStream is not supported by the Mistral provider.
func (provider *MistralProvider) ImageEditStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayImageEditRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditStreamRequest, provider.GetProviderKey())
}

// ImageVariation is not supported by the Mistral provider.
func (provider *MistralProvider) ImageVariation(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayImageVariationRequest) (*schemas.GatewayImageGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageVariationRequest, provider.GetProviderKey())
}

// VideoGeneration is not supported by the Mistral provider.
func (provider *MistralProvider) VideoGeneration(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoGenerationRequest) (*schemas.GatewayVideoGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoGenerationRequest, provider.GetProviderKey())
}

// VideoRetrieve is not supported by the Mistral provider.
func (provider *MistralProvider) VideoRetrieve(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoRetrieveRequest) (*schemas.GatewayVideoGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRetrieveRequest, provider.GetProviderKey())
}

// VideoDownload is not supported by the Mistral provider.
func (provider *MistralProvider) VideoDownload(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoDownloadRequest) (*schemas.GatewayVideoDownloadResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDownloadRequest, provider.GetProviderKey())
}

// VideoDelete is not supported by the Mistral provider.
func (provider *MistralProvider) VideoDelete(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoDeleteRequest) (*schemas.GatewayVideoDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDeleteRequest, provider.GetProviderKey())
}

// VideoList is not supported by the Mistral provider.
func (provider *MistralProvider) VideoList(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoListRequest) (*schemas.GatewayVideoListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoListRequest, provider.GetProviderKey())
}

// VideoRemix is not supported by the Mistral provider.
func (provider *MistralProvider) VideoRemix(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoRemixRequest) (*schemas.GatewayVideoGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRemixRequest, provider.GetProviderKey())
}

// ContainerCreate is not supported by the Mistral provider.
func (provider *MistralProvider) ContainerCreate(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayContainerCreateRequest) (*schemas.GatewayContainerCreateResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerCreateRequest, provider.GetProviderKey())
}

// ContainerList is not supported by the Mistral provider.
func (provider *MistralProvider) ContainerList(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerListRequest) (*schemas.GatewayContainerListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerListRequest, provider.GetProviderKey())
}

// ContainerRetrieve is not supported by the Mistral provider.
func (provider *MistralProvider) ContainerRetrieve(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerRetrieveRequest) (*schemas.GatewayContainerRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerRetrieveRequest, provider.GetProviderKey())
}

// ContainerDelete is not supported by the Mistral provider.
func (provider *MistralProvider) ContainerDelete(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerDeleteRequest) (*schemas.GatewayContainerDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerDeleteRequest, provider.GetProviderKey())
}

// ContainerFileCreate is not supported by the Mistral provider.
func (provider *MistralProvider) ContainerFileCreate(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayContainerFileCreateRequest) (*schemas.GatewayContainerFileCreateResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileCreateRequest, provider.GetProviderKey())
}

// ContainerFileList is not supported by the Mistral provider.
func (provider *MistralProvider) ContainerFileList(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileListRequest) (*schemas.GatewayContainerFileListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileListRequest, provider.GetProviderKey())
}

// ContainerFileRetrieve is not supported by the Mistral provider.
func (provider *MistralProvider) ContainerFileRetrieve(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileRetrieveRequest) (*schemas.GatewayContainerFileRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileRetrieveRequest, provider.GetProviderKey())
}

// ContainerFileContent is not supported by the Mistral provider.
func (provider *MistralProvider) ContainerFileContent(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileContentRequest) (*schemas.GatewayContainerFileContentResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileContentRequest, provider.GetProviderKey())
}

// ContainerFileDelete is not supported by the Mistral provider.
func (provider *MistralProvider) ContainerFileDelete(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileDeleteRequest) (*schemas.GatewayContainerFileDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileDeleteRequest, provider.GetProviderKey())
}

// Passthrough is not supported by the Mistral provider.
func (provider *MistralProvider) Passthrough(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayPassthroughRequest) (*schemas.GatewayPassthroughResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughRequest, provider.GetProviderKey())
}

func (provider *MistralProvider) PassthroughStream(_ *schemas.GatewayContext, _ schemas.PostHookRunner, _ func(context.Context), _ schemas.Key, _ *schemas.GatewayPassthroughRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughStreamRequest, provider.GetProviderKey())
}
