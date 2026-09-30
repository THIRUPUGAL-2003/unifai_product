// Package vllm implements the vLLM LLM provider (OpenAI-compatible).
package vllm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/unifai/unifai/core/providers/openai"
	providerUtils "github.com/unifai/unifai/core/providers/utils"
	schemas "github.com/unifai/unifai/core/schemas"
	"github.com/valyala/fasthttp"
)

// VLLMProvider implements the Provider interface for vLLM's OpenAI-compatible API.
type VLLMProvider struct {
	logger              schemas.Logger        // Logger for provider operations
	client              *fasthttp.Client      // HTTP client for unary API requests (ReadTimeout bounds overall response)
	streamingClient     *fasthttp.Client      // HTTP client for streaming API requests (no ReadTimeout; idle governed by NewIdleTimeoutReader)
	networkConfig       schemas.NetworkConfig // Network configuration including extra headers
	sendBackRawRequest  bool                  // Whether to include raw request in UnifAIResponse
	sendBackRawResponse bool                  // Whether to include raw response in UnifAIResponse
}

// NewVLLMProvider creates a new vLLM provider instance.
func NewVLLMProvider(config *schemas.ProviderConfig, logger schemas.Logger) (*VLLMProvider, error) {
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

	client = providerUtils.ConfigureProxy(client, config.ProxyConfig, logger)
	client = providerUtils.ConfigureDialer(client, config.NetworkConfig.AllowPrivateNetwork)
	client = providerUtils.ConfigureTLS(client, config.NetworkConfig, logger)
	streamingClient := providerUtils.BuildStreamingClient(client)
	config.NetworkConfig.BaseURL = strings.TrimRight(config.NetworkConfig.BaseURL, "/")

	// BaseURL is optional when keys have vllm_key_config with per-key URLs
	return &VLLMProvider{
		logger:              logger,
		client:              client,
		streamingClient:     streamingClient,
		networkConfig:       config.NetworkConfig,
		sendBackRawRequest:  config.SendBackRawRequest,
		sendBackRawResponse: config.SendBackRawResponse,
	}, nil
}

// GetProviderKey returns the provider identifier for vLLM.
func (provider *VLLMProvider) GetProviderKey() schemas.ModelProvider {
	return schemas.VLLM
}

// getBaseURL resolves the base URL for a request from the per-key vllm_key_config.
// Each vLLM key must have its own URL configured — there is no provider-level fallback.
func (provider *VLLMProvider) getBaseURL(key schemas.Key) string {
	if key.VLLMKeyConfig != nil && key.VLLMKeyConfig.URL.GetValue() != "" {
		return strings.TrimRight(key.VLLMKeyConfig.URL.GetValue(), "/")
	}
	return ""
}

// baseURLOrError returns the resolved base URL or a UnifAIError when none is configured.
func (provider *VLLMProvider) baseURLOrError(key schemas.Key) (string, *schemas.UnifAIError) {
	u := provider.getBaseURL(key)
	if u == "" {
		return "", providerUtils.NewUnifAIOperationError(
			"no base URL configured: set vllm_key_config.url on the key",
			nil)
	}
	return u, nil
}

// listModelsByKey performs a list models request for a single vLLM key,
// resolving the per-key URL so each backend is queried individually.
func (provider *VLLMProvider) listModelsByKey(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIListModelsRequest) (*schemas.UnifAIListModelsResponse, *schemas.UnifAIError) {
	baseURL, unifaiErr := provider.baseURLOrError(key)
	if unifaiErr != nil {
		return nil, unifaiErr
	}
	url := baseURL + providerUtils.GetPathFromContext(ctx, "/v1/models")
	return openai.ListModelsByKey(
		ctx,
		provider.client,
		url,
		key,
		request.Unfiltered,
		provider.networkConfig.ExtraHeaders,
		provider.GetProviderKey(),
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
	)
}

// ListModels performs a list models request to vLLM's API.
// Requests are made concurrently per key so that each backend is queried
// with its own URL (from vllm_key_config).
func (provider *VLLMProvider) ListModels(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAIListModelsRequest) (*schemas.UnifAIListModelsResponse, *schemas.UnifAIError) {
	return providerUtils.HandleMultipleListModelsRequests(
		ctx,
		keys,
		request,
		provider.listModelsByKey,
	)
}

// TextCompletion performs a text completion request to vLLM's API.
func (provider *VLLMProvider) TextCompletion(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAITextCompletionRequest) (*schemas.UnifAITextCompletionResponse, *schemas.UnifAIError) {
	ctx.SetValue(schemas.UnifAIContextKeyPassthroughExtraParams, true)
	baseURL, unifaiErr := provider.baseURLOrError(key)
	if unifaiErr != nil {
		return nil, unifaiErr
	}
	return openai.HandleOpenAITextCompletionRequest(
		ctx,
		provider.client,
		baseURL+providerUtils.GetPathFromContext(ctx, "/v1/completions"),
		request,
		openai.BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		provider.GetProviderKey(),
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		HandleVLLMResponse,
		nil,
		provider.logger,
	)
}

// TextCompletionStream performs a streaming text completion request to vLLM's API.
func (provider *VLLMProvider) TextCompletionStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAITextCompletionRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	ctx.SetValue(schemas.UnifAIContextKeyPassthroughExtraParams, true)
	baseURL, unifaiErr := provider.baseURLOrError(key)
	if unifaiErr != nil {
		return nil, unifaiErr
	}
	return openai.HandleOpenAITextCompletionStreaming(
		ctx,
		provider.streamingClient,
		baseURL+providerUtils.GetPathFromContext(ctx, "/v1/completions"),
		request,
		openai.BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		provider.networkConfig.StreamIdleTimeoutInSeconds,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		nil,
		postHookRunner,
		HandleVLLMResponse,
		nil,
		provider.logger,
		postHookSpanFinalizer,
	)
}

// ChatCompletion performs a chat completion request to vLLM's API.
func (provider *VLLMProvider) ChatCompletion(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIChatRequest) (*schemas.UnifAIChatResponse, *schemas.UnifAIError) {
	ctx.SetValue(schemas.UnifAIContextKeyPassthroughExtraParams, true)
	baseURL, unifaiErr := provider.baseURLOrError(key)
	if unifaiErr != nil {
		return nil, unifaiErr
	}
	return openai.HandleOpenAIChatCompletionRequest(
		ctx,
		provider.client,
		baseURL+providerUtils.GetPathFromContext(ctx, "/v1/chat/completions"),
		request,
		openai.BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		HandleVLLMResponse,
		nil,
		nil,
		provider.logger,
	)
}

// ChatCompletionStream performs a streaming chat completion request to vLLM's API.
func (provider *VLLMProvider) ChatCompletionStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAIChatRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	ctx.SetValue(schemas.UnifAIContextKeyPassthroughExtraParams, true)
	baseURL, unifaiErr := provider.baseURLOrError(key)
	if unifaiErr != nil {
		return nil, unifaiErr
	}
	return openai.HandleOpenAIChatCompletionStreaming(
		ctx,
		provider.streamingClient,
		baseURL+providerUtils.GetPathFromContext(ctx, "/v1/chat/completions"),
		request,
		openai.BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		provider.networkConfig.StreamIdleTimeoutInSeconds,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		postHookRunner,
		nil,
		HandleVLLMResponse,
		nil,
		nil,
		nil,
		nil,
		provider.logger,
		postHookSpanFinalizer,
	)
}

// Embedding performs an embedding request to vLLM's API.
func (provider *VLLMProvider) Embedding(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIEmbeddingRequest) (*schemas.UnifAIEmbeddingResponse, *schemas.UnifAIError) {
	baseURL, unifaiErr := provider.baseURLOrError(key)
	if unifaiErr != nil {
		return nil, unifaiErr
	}
	return openai.HandleOpenAIEmbeddingRequest(
		ctx,
		provider.client,
		baseURL+providerUtils.GetPathFromContext(ctx, "/v1/embeddings"),
		request,
		openai.BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		provider.GetProviderKey(),
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		HandleVLLMResponse,
		provider.logger,
	)
}

// Responses performs a responses request to vLLM's API (via chat completion).
func (provider *VLLMProvider) Responses(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIResponsesRequest) (*schemas.UnifAIResponsesResponse, *schemas.UnifAIError) {
	chatResponse, err := provider.ChatCompletion(ctx, key, request.ToChatRequest())
	if err != nil {
		return nil, err
	}
	response := chatResponse.ToUnifAIResponsesResponse()
	return response, nil
}

// ResponsesStream performs a streaming responses request to vLLM's API (via chat completion stream).
func (provider *VLLMProvider) ResponsesStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAIResponsesRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	ctx.SetValue(schemas.UnifAIContextKeyIsResponsesToChatCompletionFallback, true)
	return provider.ChatCompletionStream(
		ctx,
		postHookRunner,
		postHookSpanFinalizer,
		key,
		request.ToChatRequest(),
	)
}

// Speech is not supported by the vLLM provider.
func (provider *VLLMProvider) Speech(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAISpeechRequest) (*schemas.UnifAISpeechResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechRequest, provider.GetProviderKey())
}

func isRerankFallbackStatus(statusCode int) bool {
	// vLLM deployments may return 501 for unimplemented routes.
	// We fallback on 501 in addition to 404/405 for compatibility.
	return statusCode == fasthttp.StatusNotFound ||
		statusCode == fasthttp.StatusMethodNotAllowed ||
		statusCode == fasthttp.StatusNotImplemented
}

func (provider *VLLMProvider) callVLLMRerankEndpoint(
	ctx *schemas.UnifAIContext,
	key schemas.Key,
	request *schemas.UnifAIRerankRequest,
	endpointPath string,
	jsonData []byte,
) (map[string]interface{}, interface{}, interface{}, []byte, int, time.Duration, *schemas.UnifAIError) {
	baseURL, unifaiErr := provider.baseURLOrError(key)
	if unifaiErr != nil {
		return nil, nil, nil, nil, 0, 0, unifaiErr
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	req.SetRequestURI(baseURL + endpointPath)
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")

	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}
	if !providerUtils.ApplyLargePayloadRequestBodyWithModelNormalization(ctx, req, schemas.VLLM) {
		req.SetBody(jsonData)
	}

	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, nil, nil, nil, 0, latency, unifaiErr
	}

	statusCode := resp.StatusCode()
	if statusCode != fasthttp.StatusOK {
		rawErrBody := append([]byte(nil), resp.Body()...)
		return nil, nil, nil, rawErrBody, statusCode, latency, providerUtils.SetErrorLatency(openai.ParseOpenAIError(resp), latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		rawErrBody := append([]byte(nil), resp.Body()...)
		return nil, nil, nil, rawErrBody, statusCode, latency, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, err)
	}

	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)

	responsePayload := make(map[string]interface{})
	rawRequest, rawResponse, unifaiErr := HandleVLLMResponse(body, &responsePayload, jsonData, sendBackRawRequest, sendBackRawResponse)
	if unifaiErr != nil {
		return nil, nil, nil, body, statusCode, latency, unifaiErr
	}

	return responsePayload, rawRequest, rawResponse, body, statusCode, latency, nil
}

// Rerank performs a rerank request to vLLM's API.
func (provider *VLLMProvider) Rerank(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIRerankRequest) (*schemas.UnifAIRerankResponse, *schemas.UnifAIError) {
	jsonData, unifaiErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToVLLMRerankRequest(request), nil
		})
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	resolvedPath := providerUtils.GetPathFromContext(ctx, "")
	hasPathOverride := resolvedPath != ""
	if !hasPathOverride {
		resolvedPath = "/v1/rerank"
	} else if !strings.HasPrefix(resolvedPath, "/") {
		resolvedPath = "/" + resolvedPath
	}

	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)

	responsePayload, rawRequest, rawResponse, responseBody, statusCode, latency, unifaiErr := provider.callVLLMRerankEndpoint(ctx, key, request, resolvedPath, jsonData)
	if unifaiErr != nil && !hasPathOverride && isRerankFallbackStatus(statusCode) {
		var fallbackLatency time.Duration
		responsePayload, rawRequest, rawResponse, responseBody, statusCode, fallbackLatency, unifaiErr = provider.callVLLMRerankEndpoint(ctx, key, request, "/rerank", jsonData)
		latency += fallbackLatency
	}
	if unifaiErr != nil {
		return nil, providerUtils.EnrichError(ctx, unifaiErr, jsonData, responseBody, sendBackRawRequest, sendBackRawResponse, latency)
	}

	returnDocuments := request.Params != nil && request.Params.ReturnDocuments != nil && *request.Params.ReturnDocuments
	unifaiResponse, err := ToUnifAIRerankResponse(responsePayload, request.Documents, returnDocuments)
	if err != nil {
		return nil, providerUtils.EnrichError(
			ctx,
			providerUtils.NewUnifAIOperationError("error converting rerank response", err),
			jsonData,
			responseBody,
			sendBackRawRequest,
			sendBackRawResponse,
			latency,
		)
	}

	// Keep requested model as the canonical model in UnifAI response.
	unifaiResponse.Model = request.Model
	unifaiResponse.ExtraFields.Latency = latency.Milliseconds()

	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		unifaiResponse.ExtraFields.RawRequest = rawRequest
	}
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		unifaiResponse.ExtraFields.RawResponse = rawResponse
	}

	return unifaiResponse, nil
}

// OCR is not supported by the Vllm provider.
func (provider *VLLMProvider) OCR(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIOCRRequest) (*schemas.UnifAIOCRResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.OCRRequest, provider.GetProviderKey())
}

// SpeechStream is not supported by the vLLM provider.
func (provider *VLLMProvider) SpeechStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAISpeechRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechStreamRequest, provider.GetProviderKey())
}

// Transcription performs a transcription request to vLLM's API.
func (provider *VLLMProvider) Transcription(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAITranscriptionRequest) (*schemas.UnifAITranscriptionResponse, *schemas.UnifAIError) {
	baseURL, unifaiErr := provider.baseURLOrError(key)
	if unifaiErr != nil {
		return nil, unifaiErr
	}
	return openai.HandleOpenAITranscriptionRequest(
		ctx,
		provider.client,
		baseURL+providerUtils.GetPathFromContext(ctx, "/v1/audio/transcriptions"),
		request,
		key,
		provider.networkConfig.ExtraHeaders,
		provider.GetProviderKey(),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		HandleVLLMResponse,
		provider.logger,
	)
}

// TranscriptionStream performs a streaming transcription request to vLLM's API.
func (provider *VLLMProvider) TranscriptionStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAITranscriptionRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	baseURL, unifaiErr := provider.baseURLOrError(key)
	if unifaiErr != nil {
		return nil, unifaiErr
	}
	{
		logger := provider.logger
		providerName := provider.GetProviderKey()
		// Use centralized converter
		reqBody := openai.ToOpenAITranscriptionRequest(request)
		if reqBody == nil {
			return nil, providerUtils.NewUnifAIOperationError("transcription input is not provided", nil)
		}
		reqBody.Stream = schemas.Ptr(true)

		// Create multipart form
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)

		if unifaiErr := openai.ParseTranscriptionFormDataBodyFromRequest(writer, reqBody, providerName); unifaiErr != nil {
			return nil, unifaiErr
		}

		// Prepare OpenAI headers
		headers := map[string]string{
			"Content-Type":  writer.FormDataContentType(),
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
		req.SetRequestURI(baseURL + providerUtils.GetPathFromContext(ctx, "/v1/audio/transcriptions"))

		// Set headers
		for key, value := range headers {
			req.Header.Set(key, value)
		}

		req.SetBody(body.Bytes())

		startTime := time.Now()
		// Make the request
		err := provider.streamingClient.Do(req, resp)
		latency := time.Since(startTime)
		if err != nil {
			defer providerUtils.ReleaseStreamingResponse(ctx, resp)
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
			return nil, providerUtils.SetErrorLatency(providerUtils.NewUnifAIOperationError(schemas.ErrProviderDoRequest, err), latency)
		}

		// Store provider response headers in context before status check so error responses also forward them
		ctx.SetValue(schemas.UnifAIContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

		// Check for HTTP errors
		if resp.StatusCode() != fasthttp.StatusOK {
			defer providerUtils.ReleaseStreamingResponse(ctx, resp)
			return nil, providerUtils.SetErrorLatency(openai.ParseOpenAIError(resp), latency)
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

			sseReader := providerUtils.GetSSEDataReader(ctx, reader)
			chunkIndex := -1

			lastChunkTime := startTime
			var fullTranscriptionText strings.Builder

			for {
				// If context was cancelled/timed out, let defer handle it
				if ctx.Err() != nil {
					return
				}

				dataBytes, readErr := sseReader.ReadDataLine()
				if readErr != nil {
					// If context was cancelled/timed out, let defer handle it
					if ctx.Err() != nil {
						return
					}
					if readErr != io.EOF {
						ctx.SetValue(schemas.UnifAIContextKeyStreamEndIndicator, true)
						logger.Warn("Error reading stream: %v", readErr)
						providerUtils.ProcessAndSendError(ctx, postHookRunner, readErr, responseChan, logger, postHookSpanFinalizer)
					}
					break
				}

				jsonData := string(dataBytes)

				// Skip empty data
				if strings.TrimSpace(jsonData) == "" {
					continue
				}

				var response schemas.UnifAITranscriptionStreamResponse
				var unifaiErr *schemas.UnifAIError

				_, _, unifaiErr = HandleVLLMResponse(dataBytes, &response, nil, false, false)
				if unifaiErr != nil {
					ctx.SetValue(schemas.UnifAIContextKeyStreamEndIndicator, true)
					providerUtils.ProcessAndSendUnifAIError(ctx, postHookRunner, providerUtils.EnrichError(ctx, unifaiErr, body.Bytes(), dataBytes, false, providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse), latency), responseChan, logger, postHookSpanFinalizer)
					return
				}

				customChunk, ok := parseVLLMTranscriptionStreamChunk(dataBytes)
				if !ok || customChunk == nil {
					logger.Warn("customChunkParser returned no chunk")
					continue
				}
				response = *customChunk

				chunkIndex++
				if response.Delta != nil {
					fullTranscriptionText.WriteString(*response.Delta)
				}

				response.ExtraFields = schemas.UnifAIResponseExtraFields{
					ChunkIndex: chunkIndex,
					Latency:    time.Since(lastChunkTime).Milliseconds(),
				}
				lastChunkTime = time.Now()

				if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
					response.ExtraFields.RawResponse = jsonData
				}
				if response.Usage != nil || response.Type == schemas.TranscriptionStreamResponseTypeDone {
					response.ExtraFields.Latency = time.Since(startTime).Milliseconds()
					response.Text = fullTranscriptionText.String()
					ctx.SetValue(schemas.UnifAIContextKeyStreamEndIndicator, true)
					providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetUnifAIResponseForStreamResponse(nil, nil, nil, nil, &response, nil), responseChan, postHookSpanFinalizer)
					return
				}

				providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetUnifAIResponseForStreamResponse(nil, nil, nil, nil, &response, nil), responseChan, postHookSpanFinalizer)
			}
		}()

		return responseChan, nil
	}
}

// ImageGeneration is not supported by the vLLM provider.
func (provider *VLLMProvider) ImageGeneration(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIImageGenerationRequest) (*schemas.UnifAIImageGenerationResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationRequest, provider.GetProviderKey())
}

// ImageGenerationStream is not supported by the vLLM provider.
func (provider *VLLMProvider) ImageGenerationStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAIImageGenerationRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationStreamRequest, provider.GetProviderKey())
}

// ImageEdit is not supported by the vLLM provider.
func (provider *VLLMProvider) ImageEdit(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIImageEditRequest) (*schemas.UnifAIImageGenerationResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditRequest, provider.GetProviderKey())
}

// ImageEditStream is not supported by the vLLM provider.
func (provider *VLLMProvider) ImageEditStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAIImageEditRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditStreamRequest, provider.GetProviderKey())
}

// ImageVariation is not supported by the vLLM provider.
func (provider *VLLMProvider) ImageVariation(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIImageVariationRequest) (*schemas.UnifAIImageGenerationResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageVariationRequest, provider.GetProviderKey())
}

// VideoGeneration is not supported by the vLLM provider.
func (provider *VLLMProvider) VideoGeneration(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIVideoGenerationRequest) (*schemas.UnifAIVideoGenerationResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoGenerationRequest, provider.GetProviderKey())
}

// VideoRetrieve is not supported by the vLLM provider.
func (provider *VLLMProvider) VideoRetrieve(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIVideoRetrieveRequest) (*schemas.UnifAIVideoGenerationResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRetrieveRequest, provider.GetProviderKey())
}

// VideoDownload is not supported by the vLLM provider.
func (provider *VLLMProvider) VideoDownload(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIVideoDownloadRequest) (*schemas.UnifAIVideoDownloadResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDownloadRequest, provider.GetProviderKey())
}

// VideoDelete is not supported by the vLLM provider.
func (provider *VLLMProvider) VideoDelete(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIVideoDeleteRequest) (*schemas.UnifAIVideoDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDeleteRequest, provider.GetProviderKey())
}

// VideoList is not supported by the vLLM provider.
func (provider *VLLMProvider) VideoList(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIVideoListRequest) (*schemas.UnifAIVideoListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoListRequest, provider.GetProviderKey())
}

// VideoRemix is not supported by the vLLM provider.
func (provider *VLLMProvider) VideoRemix(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIVideoRemixRequest) (*schemas.UnifAIVideoGenerationResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRemixRequest, provider.GetProviderKey())
}

// FileUpload is not supported by the vLLM provider.
func (provider *VLLMProvider) FileUpload(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIFileUploadRequest) (*schemas.UnifAIFileUploadResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileUploadRequest, provider.GetProviderKey())
}

// FileList is not supported by the vLLM provider.
func (provider *VLLMProvider) FileList(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIFileListRequest) (*schemas.UnifAIFileListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileListRequest, provider.GetProviderKey())
}

// FileRetrieve is not supported by the vLLM provider.
func (provider *VLLMProvider) FileRetrieve(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIFileRetrieveRequest) (*schemas.UnifAIFileRetrieveResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileRetrieveRequest, provider.GetProviderKey())
}

// FileDelete is not supported by the vLLM provider.
func (provider *VLLMProvider) FileDelete(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIFileDeleteRequest) (*schemas.UnifAIFileDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileDeleteRequest, provider.GetProviderKey())
}

// FileContent is not supported by the vLLM provider.
func (provider *VLLMProvider) FileContent(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIFileContentRequest) (*schemas.UnifAIFileContentResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileContentRequest, provider.GetProviderKey())
}

// BatchCreate is not supported by the vLLM provider.
func (provider *VLLMProvider) BatchCreate(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIBatchCreateRequest) (*schemas.UnifAIBatchCreateResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCreateRequest, provider.GetProviderKey())
}

// BatchList is not supported by the vLLM provider.
func (provider *VLLMProvider) BatchList(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIBatchListRequest) (*schemas.UnifAIBatchListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchListRequest, provider.GetProviderKey())
}

// BatchRetrieve is not supported by the vLLM provider.
func (provider *VLLMProvider) BatchRetrieve(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIBatchRetrieveRequest) (*schemas.UnifAIBatchRetrieveResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchRetrieveRequest, provider.GetProviderKey())
}

// BatchCancel is not supported by the vLLM provider.
func (provider *VLLMProvider) BatchCancel(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIBatchCancelRequest) (*schemas.UnifAIBatchCancelResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCancelRequest, provider.GetProviderKey())
}

// BatchDelete is not supported by the vLLM provider.
func (provider *VLLMProvider) BatchDelete(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIBatchDeleteRequest) (*schemas.UnifAIBatchDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchDeleteRequest, provider.GetProviderKey())
}

// BatchResults is not supported by the vLLM provider.
func (provider *VLLMProvider) BatchResults(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIBatchResultsRequest) (*schemas.UnifAIBatchResultsResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchResultsRequest, provider.GetProviderKey())
}

// CountTokens is not supported by the vLLM provider.
func (provider *VLLMProvider) CountTokens(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIResponsesRequest) (*schemas.UnifAICountTokensResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CountTokensRequest, provider.GetProviderKey())
}

// Compaction is not supported by the vLLM provider.
func (provider *VLLMProvider) Compaction(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAICompactionRequest) (*schemas.UnifAICompactionResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CompactionRequest, provider.GetProviderKey())
}

// ContainerCreate is not supported by the vLLM provider.
func (provider *VLLMProvider) ContainerCreate(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIContainerCreateRequest) (*schemas.UnifAIContainerCreateResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerCreateRequest, provider.GetProviderKey())
}

// ContainerList is not supported by the vLLM provider.
func (provider *VLLMProvider) ContainerList(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerListRequest) (*schemas.UnifAIContainerListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerListRequest, provider.GetProviderKey())
}

// ContainerRetrieve is not supported by the vLLM provider.
func (provider *VLLMProvider) ContainerRetrieve(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerRetrieveRequest) (*schemas.UnifAIContainerRetrieveResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerRetrieveRequest, provider.GetProviderKey())
}

// ContainerDelete is not supported by the vLLM provider.
func (provider *VLLMProvider) ContainerDelete(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerDeleteRequest) (*schemas.UnifAIContainerDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerDeleteRequest, provider.GetProviderKey())
}

// ContainerFileCreate is not supported by the vLLM provider.
func (provider *VLLMProvider) ContainerFileCreate(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIContainerFileCreateRequest) (*schemas.UnifAIContainerFileCreateResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileCreateRequest, provider.GetProviderKey())
}

// ContainerFileList is not supported by the vLLM provider.
func (provider *VLLMProvider) ContainerFileList(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerFileListRequest) (*schemas.UnifAIContainerFileListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileListRequest, provider.GetProviderKey())
}

// ContainerFileRetrieve is not supported by the vLLM provider.
func (provider *VLLMProvider) ContainerFileRetrieve(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerFileRetrieveRequest) (*schemas.UnifAIContainerFileRetrieveResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileRetrieveRequest, provider.GetProviderKey())
}

// ContainerFileContent is not supported by the vLLM provider.
func (provider *VLLMProvider) ContainerFileContent(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerFileContentRequest) (*schemas.UnifAIContainerFileContentResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileContentRequest, provider.GetProviderKey())
}

// ContainerFileDelete is not supported by the vLLM provider.
func (provider *VLLMProvider) ContainerFileDelete(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerFileDeleteRequest) (*schemas.UnifAIContainerFileDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileDeleteRequest, provider.GetProviderKey())
}

// Passthrough is not supported by the vLLM provider.
func (provider *VLLMProvider) Passthrough(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIPassthroughRequest) (*schemas.UnifAIPassthroughResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughRequest, provider.GetProviderKey())
}

func (provider *VLLMProvider) PassthroughStream(_ *schemas.UnifAIContext, _ schemas.PostHookRunner, _ func(context.Context), _ schemas.Key, _ *schemas.UnifAIPassthroughRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughStreamRequest, provider.GetProviderKey())
}
