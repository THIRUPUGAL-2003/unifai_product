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
	"github.com/raksha/raksha/core/providers/openai"
	providerUtils "github.com/raksha/raksha/core/providers/utils"
	schemas "github.com/raksha/raksha/core/schemas"
	"github.com/valyala/fasthttp"
)

// MistralProvider implements the Provider interface for Mistral's API.
type MistralProvider struct {
	logger               schemas.Logger        // Logger for provider operations
	client               *fasthttp.Client      // HTTP client for unary API requests (ReadTimeout bounds overall response)
	streamingClient      *fasthttp.Client      // HTTP client for streaming API requests (no ReadTimeout; idle governed by NewIdleTimeoutReader)
	networkConfig        schemas.NetworkConfig // Network configuration including extra headers
	customProviderConfig *schemas.CustomProviderConfig
	sendBackRawRequest   bool // Whether to include raw request in RakshaResponse
	sendBackRawResponse  bool // Whether to include raw response in RakshaResponse
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
	// 	mistralResponsePool.Put(&schemas.RakshaResponse{})
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
func (provider *MistralProvider) listModelsByKey(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaListModelsRequest) (*schemas.RakshaListModelsResponse, *schemas.RakshaError) {
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
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		rakshaErr := providerUtils.SetErrorLatency(ParseMistralError(resp), latency)
		return nil, rakshaErr
	}

	// Copy response body before releasing
	responseBody := append([]byte(nil), resp.Body()...)

	// Parse Mistral's response
	var mistralResponse MistralListModelsResponse
	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(responseBody, &mistralResponse, nil, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Create final response
	response := mistralResponse.ToRakshaListModelsResponse(key.Models, key.BlacklistedModels, key.Aliases, request.Unfiltered)

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
func (provider *MistralProvider) ListModels(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaListModelsRequest) (*schemas.RakshaListModelsResponse, *schemas.RakshaError) {
	return providerUtils.HandleMultipleListModelsRequests(
		ctx,
		keys,
		request,
		provider.listModelsByKey,
	)
}

// TextCompletion is not supported by the Mistral provider.
func (provider *MistralProvider) TextCompletion(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaTextCompletionRequest) (*schemas.RakshaTextCompletionResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionRequest, provider.GetProviderKey())
}

// TextCompletionStream performs a streaming text completion request to Mistral's API.
// It formats the request, sends it to Mistral, and processes the response.
// Returns a channel of RakshaStreamChunk objects or an error if the request fails.
func (provider *MistralProvider) TextCompletionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaTextCompletionRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionStreamRequest, provider.GetProviderKey())
}

// normalizeChatRequestForConversion returns the request unchanged for the stock Mistral
// provider. For custom aliases (e.g. a provider registered as "custom-mistral" with
// BaseProviderType=Mistral), it returns a shallow copy with Provider set to schemas.Mistral
// so the shared OpenAI converter applies Mistral-specific compatibility (max_completion_tokens
// → max_tokens, tool_choice struct → "any"). The caller's request is never mutated.
func (provider *MistralProvider) normalizeChatRequestForConversion(request *schemas.RakshaChatRequest) *schemas.RakshaChatRequest {
	if request == nil || provider.customProviderConfig == nil || request.Provider == schemas.Mistral {
		return request
	}
	normalized := *request
	normalized.Provider = schemas.Mistral
	return &normalized
}

// ChatCompletion performs a chat completion request to the Mistral API.
func (provider *MistralProvider) ChatCompletion(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaChatRequest) (*schemas.RakshaChatResponse, *schemas.RakshaError) {
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
// Returns a channel containing RakshaStreamChunk objects representing the stream or an error if the request fails.
func (provider *MistralProvider) ChatCompletionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaChatRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
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
func (provider *MistralProvider) Responses(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaResponsesRequest) (*schemas.RakshaResponsesResponse, *schemas.RakshaError) {
	chatResponse, err := provider.ChatCompletion(ctx, key, request.ToChatRequest())
	if err != nil {
		return nil, err
	}

	response := chatResponse.ToRakshaResponsesResponse()

	return response, nil
}

// ResponsesStream performs a streaming responses request to the Mistral API.
func (provider *MistralProvider) ResponsesStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaResponsesRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	ctx.SetValue(schemas.RakshaContextKeyIsResponsesToChatCompletionFallback, true)
	return provider.ChatCompletionStream(
		ctx,
		postHookRunner,
		postHookSpanFinalizer,
		key,
		request.ToChatRequest(),
	)
}

// Embedding generates embeddings for the given input text(s) using the Mistral API.
// Supports Mistral's embedding models and returns a RakshaResponse containing the embedding(s).
func (provider *MistralProvider) Embedding(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaEmbeddingRequest) (*schemas.RakshaEmbeddingResponse, *schemas.RakshaError) {
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
func (provider *MistralProvider) Speech(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaSpeechRequest) (*schemas.RakshaSpeechResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechRequest, provider.GetProviderKey())
}

// SpeechStream is not supported by the Mistral provider.
func (provider *MistralProvider) SpeechStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaSpeechRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechStreamRequest, provider.GetProviderKey())
}

// Transcription performs an audio transcription request to the Mistral API.
// It creates a multipart form with the audio file and sends it to Mistral's transcription endpoint.
// Returns the transcribed text and metadata, or an error if the request fails.
func (provider *MistralProvider) Transcription(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaTranscriptionRequest) (*schemas.RakshaTranscriptionResponse, *schemas.RakshaError) {
	// Convert Raksha request to Mistral format
	mistralReq := ToMistralTranscriptionRequest(request)
	if mistralReq == nil {
		return nil, providerUtils.NewRakshaOperationError("transcription input is not provided", nil)
	}

	// Create multipart form body
	body, contentType, rakshaErr := createMistralTranscriptionMultipartBody(mistralReq, provider.GetProviderKey())
	if rakshaErr != nil {
		return nil, rakshaErr
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
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.SetErrorLatency(ParseMistralError(resp), latency)
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

	copiedResponseBody := append([]byte(nil), responseBody...)

	// Parse Mistral's transcription response
	var mistralResponse MistralTranscriptionResponse
	if err := sonic.Unmarshal(copiedResponseBody, &mistralResponse); err != nil {
		if providerUtils.IsHTMLResponse(resp, copiedResponseBody) {
			return nil, &schemas.RakshaError{
				IsRakshaError: false,
				Error: &schemas.ErrorField{
					Message: schemas.ErrProviderResponseHTML,
					Error:   errors.New(string(copiedResponseBody)),
				},
			}
		}
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseUnmarshal, err)
	}

	// Convert to Raksha format
	response := mistralResponse.ToRakshaTranscriptionResponse()
	if response == nil {
		return nil, providerUtils.NewRakshaOperationError("failed to convert transcription response", nil)
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
// Returns a channel of RakshaStreamChunk objects containing transcription deltas.
func (provider *MistralProvider) TranscriptionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaTranscriptionRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	providerName := provider.GetProviderKey()

	// Convert Raksha request to Mistral format
	mistralReq := ToMistralTranscriptionRequest(request)
	if mistralReq == nil {
		return nil, providerUtils.NewRakshaOperationError("transcription input is not provided", nil)
	}
	mistralReq.Stream = schemas.Ptr(true)

	// Create multipart form body with stream=true
	body, contentType, rakshaErr := createMistralTranscriptionMultipartBody(mistralReq, providerName)
	if rakshaErr != nil {
		return nil, rakshaErr
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
		// Request failed before the first response byte (server closed an idle/pooled connection,
		// broken pipe, connection refused, DNS failure, etc.). Surface as a retriable upstream
		// connection error (502) so executeRequestWithRetries honors max_retries, matching the
		// non-streaming path - see https://github.com/raksha/raksha/issues/4496.
		return nil, providerUtils.SetErrorLatency(providerUtils.NewRakshaUpstreamConnectionError(schemas.ErrProviderDoRequest, err), latency)
	}

	// Store provider response headers in context before status check so error responses also forward them
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	// Check for HTTP errors
	if resp.StatusCode() != fasthttp.StatusOK {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		return nil, providerUtils.SetErrorLatency(ParseMistralError(resp), latency)
	}

	// Large payload streaming passthrough — pipe raw upstream SSE to client
	if providerUtils.SetupStreamingPassthrough(ctx, resp) {
		responseChan := make(chan *schemas.RakshaStreamChunk)
		providerUtils.CloseStream(ctx, responseChan)
		return responseChan, nil
	}

	// Create response channel
	responseChan := make(chan *schemas.RakshaStreamChunk, schemas.DefaultStreamBufferSize)

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
					ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
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
			if ended, _ := ctx.Value(schemas.RakshaContextKeyStreamEndIndicator).(bool); ended {
				break
			}
		}
	}()

	return responseChan, nil
}

// processTranscriptionStreamEvent processes a single SSE event and sends it to the response channel.
func (provider *MistralProvider) processTranscriptionStreamEvent(
	ctx *schemas.RakshaContext,
	postHookRunner schemas.PostHookRunner,
	eventType string,
	jsonData string,
	chunkIndex int,
	startTime time.Time,
	lastChunkTime *time.Time,
	responseChan chan *schemas.RakshaStreamChunk,
	postHookSpanFinalizer func(context.Context),
) {
	// Skip empty data
	if strings.TrimSpace(jsonData) == "" {
		return
	}

	// Quick check for error field (allocation-free using sonic.GetFromString)
	if errorNode, _ := sonic.GetFromString(jsonData, "error"); errorNode.Exists() {
		// Only unmarshal when we know there's an error
		var rakshaErr schemas.RakshaError
		if err := sonic.UnmarshalString(jsonData, &rakshaErr); err == nil {
			if rakshaErr.Error != nil && rakshaErr.Error.Message != "" {
				ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
				providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, &rakshaErr, responseChan, provider.logger, postHookSpanFinalizer)
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

	// Convert to Raksha format
	response := streamEvent.ToRakshaTranscriptionStreamResponse()
	if response == nil {
		return
	}

	// Set extra fields
	response.ExtraFields = schemas.RakshaResponseExtraFields{
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
		ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
		// Ensure response type is set to Done
		response.Type = schemas.TranscriptionStreamResponseTypeDone
	}

	providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(nil, nil, nil, nil, response, nil), responseChan, postHookSpanFinalizer)
}

// Rerank is not supported by the Mistral provider.
func (provider *MistralProvider) Rerank(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaRerankRequest) (*schemas.RakshaRerankResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.RerankRequest, provider.GetProviderKey())
}

// OCR performs an OCR request to the Mistral API.
// It sends a JSON request to Mistral's OCR endpoint and returns the extracted content.
func (provider *MistralProvider) OCR(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaOCRRequest) (*schemas.RakshaOCRResponse, *schemas.RakshaError) {
	// Convert Raksha request to Mistral format
	mistralReq := ToMistralOCRRequest(request)
	if mistralReq == nil {
		return nil, providerUtils.NewRakshaOperationError("ocr request input is not provided", nil)
	}

	// Marshal request body
	requestBody, err := sonic.Marshal(mistralReq)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderRequestMarshal, err)
	}

	// Merge extra params into JSON payload
	if len(mistralReq.ExtraParams) > 0 {
		requestBody, err = providerUtils.MergeExtraParamsIntoJSON(requestBody, mistralReq.ExtraParams)
		if err != nil {
			return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderRequestMarshal, err)
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
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.SetErrorLatency(ParseMistralError(resp), latency)
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

	copiedResponseBody := append([]byte(nil), responseBody...)

	// Parse Mistral's OCR response
	var mistralResponse MistralOCRResponse
	if err := sonic.Unmarshal(copiedResponseBody, &mistralResponse); err != nil {
		if providerUtils.IsHTMLResponse(resp, copiedResponseBody) {
			return nil, &schemas.RakshaError{
				IsRakshaError: false,
				Error: &schemas.ErrorField{
					Message: schemas.ErrProviderResponseHTML,
					Error:   errors.New(string(copiedResponseBody)),
				},
			}
		}
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseUnmarshal, err)
	}

	// Convert to Raksha format
	response := mistralResponse.ToRakshaOCRResponse()
	if response == nil {
		return nil, providerUtils.NewRakshaOperationError("failed to convert ocr response", nil)
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
func (provider *MistralProvider) BatchCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaBatchCreateRequest) (*schemas.RakshaBatchCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCreateRequest, provider.GetProviderKey())
}

// BatchList is not supported by Mistral provider.
func (provider *MistralProvider) BatchList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchListRequest) (*schemas.RakshaBatchListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchListRequest, provider.GetProviderKey())
}

// BatchRetrieve is not supported by Mistral provider.
func (provider *MistralProvider) BatchRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchRetrieveRequest) (*schemas.RakshaBatchRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchRetrieveRequest, provider.GetProviderKey())
}

// BatchCancel is not supported by Mistral provider.
func (provider *MistralProvider) BatchCancel(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchCancelRequest) (*schemas.RakshaBatchCancelResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCancelRequest, provider.GetProviderKey())
}

// BatchDelete is not supported by Mistral provider.
func (provider *MistralProvider) BatchDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchDeleteRequest) (*schemas.RakshaBatchDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchDeleteRequest, provider.GetProviderKey())
}

// BatchResults is not supported by Mistral provider.
func (provider *MistralProvider) BatchResults(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchResultsRequest) (*schemas.RakshaBatchResultsResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchResultsRequest, provider.GetProviderKey())
}

// FileUpload is not supported by Mistral provider.
func (provider *MistralProvider) FileUpload(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaFileUploadRequest) (*schemas.RakshaFileUploadResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileUploadRequest, provider.GetProviderKey())
}

// FileList is not supported by Mistral provider.
func (provider *MistralProvider) FileList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileListRequest) (*schemas.RakshaFileListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileListRequest, provider.GetProviderKey())
}

// FileRetrieve is not supported by Mistral provider.
func (provider *MistralProvider) FileRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileRetrieveRequest) (*schemas.RakshaFileRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileRetrieveRequest, provider.GetProviderKey())
}

// FileDelete is not supported by Mistral provider.
func (provider *MistralProvider) FileDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileDeleteRequest) (*schemas.RakshaFileDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileDeleteRequest, provider.GetProviderKey())
}

// FileContent is not supported by Mistral provider.
func (provider *MistralProvider) FileContent(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileContentRequest) (*schemas.RakshaFileContentResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileContentRequest, provider.GetProviderKey())
}

// CountTokens is not supported by the Mistral provider.
func (provider *MistralProvider) CountTokens(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaResponsesRequest) (*schemas.RakshaCountTokensResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CountTokensRequest, provider.GetProviderKey())
}

// Compaction is not supported by the Mistral provider.
func (provider *MistralProvider) Compaction(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaCompactionRequest) (*schemas.RakshaCompactionResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CompactionRequest, provider.GetProviderKey())
}

// ImageGeneration is not supported by the Mistral provider.
func (provider *MistralProvider) ImageGeneration(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageGenerationRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationRequest, provider.GetProviderKey())
}

// ImageGenerationStream is not supported by the Mistral provider.
func (provider *MistralProvider) ImageGenerationStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaImageGenerationRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationStreamRequest, provider.GetProviderKey())
}

// ImageEdit is not supported by the Mistral provider.
func (provider *MistralProvider) ImageEdit(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageEditRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditRequest, provider.GetProviderKey())
}

// ImageEditStream is not supported by the Mistral provider.
func (provider *MistralProvider) ImageEditStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaImageEditRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditStreamRequest, provider.GetProviderKey())
}

// ImageVariation is not supported by the Mistral provider.
func (provider *MistralProvider) ImageVariation(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageVariationRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageVariationRequest, provider.GetProviderKey())
}

// VideoGeneration is not supported by the Mistral provider.
func (provider *MistralProvider) VideoGeneration(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoGenerationRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoGenerationRequest, provider.GetProviderKey())
}

// VideoRetrieve is not supported by the Mistral provider.
func (provider *MistralProvider) VideoRetrieve(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoRetrieveRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRetrieveRequest, provider.GetProviderKey())
}

// VideoDownload is not supported by the Mistral provider.
func (provider *MistralProvider) VideoDownload(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoDownloadRequest) (*schemas.RakshaVideoDownloadResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDownloadRequest, provider.GetProviderKey())
}

// VideoDelete is not supported by the Mistral provider.
func (provider *MistralProvider) VideoDelete(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoDeleteRequest) (*schemas.RakshaVideoDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDeleteRequest, provider.GetProviderKey())
}

// VideoList is not supported by the Mistral provider.
func (provider *MistralProvider) VideoList(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoListRequest) (*schemas.RakshaVideoListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoListRequest, provider.GetProviderKey())
}

// VideoRemix is not supported by the Mistral provider.
func (provider *MistralProvider) VideoRemix(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoRemixRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRemixRequest, provider.GetProviderKey())
}

// ContainerCreate is not supported by the Mistral provider.
func (provider *MistralProvider) ContainerCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaContainerCreateRequest) (*schemas.RakshaContainerCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerCreateRequest, provider.GetProviderKey())
}

// ContainerList is not supported by the Mistral provider.
func (provider *MistralProvider) ContainerList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerListRequest) (*schemas.RakshaContainerListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerListRequest, provider.GetProviderKey())
}

// ContainerRetrieve is not supported by the Mistral provider.
func (provider *MistralProvider) ContainerRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerRetrieveRequest) (*schemas.RakshaContainerRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerRetrieveRequest, provider.GetProviderKey())
}

// ContainerDelete is not supported by the Mistral provider.
func (provider *MistralProvider) ContainerDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerDeleteRequest) (*schemas.RakshaContainerDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerDeleteRequest, provider.GetProviderKey())
}

// ContainerFileCreate is not supported by the Mistral provider.
func (provider *MistralProvider) ContainerFileCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaContainerFileCreateRequest) (*schemas.RakshaContainerFileCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileCreateRequest, provider.GetProviderKey())
}

// ContainerFileList is not supported by the Mistral provider.
func (provider *MistralProvider) ContainerFileList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileListRequest) (*schemas.RakshaContainerFileListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileListRequest, provider.GetProviderKey())
}

// ContainerFileRetrieve is not supported by the Mistral provider.
func (provider *MistralProvider) ContainerFileRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileRetrieveRequest) (*schemas.RakshaContainerFileRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileRetrieveRequest, provider.GetProviderKey())
}

// ContainerFileContent is not supported by the Mistral provider.
func (provider *MistralProvider) ContainerFileContent(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileContentRequest) (*schemas.RakshaContainerFileContentResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileContentRequest, provider.GetProviderKey())
}

// ContainerFileDelete is not supported by the Mistral provider.
func (provider *MistralProvider) ContainerFileDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileDeleteRequest) (*schemas.RakshaContainerFileDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileDeleteRequest, provider.GetProviderKey())
}

// Passthrough is not supported by the Mistral provider.
func (provider *MistralProvider) Passthrough(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaPassthroughRequest) (*schemas.RakshaPassthroughResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughRequest, provider.GetProviderKey())
}

func (provider *MistralProvider) PassthroughStream(_ *schemas.RakshaContext, _ schemas.PostHookRunner, _ func(context.Context), _ schemas.Key, _ *schemas.RakshaPassthroughRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughStreamRequest, provider.GetProviderKey())
}
