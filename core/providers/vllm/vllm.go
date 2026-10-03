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

	"github.com/raksha/raksha/core/providers/openai"
	providerUtils "github.com/raksha/raksha/core/providers/utils"
	schemas "github.com/raksha/raksha/core/schemas"
	"github.com/valyala/fasthttp"
)

// VLLMProvider implements the Provider interface for vLLM's OpenAI-compatible API.
type VLLMProvider struct {
	logger              schemas.Logger        // Logger for provider operations
	client              *fasthttp.Client      // HTTP client for unary API requests (ReadTimeout bounds overall response)
	streamingClient     *fasthttp.Client      // HTTP client for streaming API requests (no ReadTimeout; idle governed by NewIdleTimeoutReader)
	networkConfig       schemas.NetworkConfig // Network configuration including extra headers
	sendBackRawRequest  bool                  // Whether to include raw request in RakshaResponse
	sendBackRawResponse bool                  // Whether to include raw response in RakshaResponse
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

// baseURLOrError returns the resolved base URL or a RakshaError when none is configured.
func (provider *VLLMProvider) baseURLOrError(key schemas.Key) (string, *schemas.RakshaError) {
	u := provider.getBaseURL(key)
	if u == "" {
		return "", providerUtils.NewRakshaOperationError(
			"no base URL configured: set vllm_key_config.url on the key",
			nil)
	}
	return u, nil
}

// listModelsByKey performs a list models request for a single vLLM key,
// resolving the per-key URL so each backend is queried individually.
func (provider *VLLMProvider) listModelsByKey(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaListModelsRequest) (*schemas.RakshaListModelsResponse, *schemas.RakshaError) {
	baseURL, rakshaErr := provider.baseURLOrError(key)
	if rakshaErr != nil {
		return nil, rakshaErr
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
func (provider *VLLMProvider) ListModels(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaListModelsRequest) (*schemas.RakshaListModelsResponse, *schemas.RakshaError) {
	return providerUtils.HandleMultipleListModelsRequests(
		ctx,
		keys,
		request,
		provider.listModelsByKey,
	)
}

// TextCompletion performs a text completion request to vLLM's API.
func (provider *VLLMProvider) TextCompletion(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaTextCompletionRequest) (*schemas.RakshaTextCompletionResponse, *schemas.RakshaError) {
	ctx.SetValue(schemas.RakshaContextKeyPassthroughExtraParams, true)
	baseURL, rakshaErr := provider.baseURLOrError(key)
	if rakshaErr != nil {
		return nil, rakshaErr
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
func (provider *VLLMProvider) TextCompletionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaTextCompletionRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	ctx.SetValue(schemas.RakshaContextKeyPassthroughExtraParams, true)
	baseURL, rakshaErr := provider.baseURLOrError(key)
	if rakshaErr != nil {
		return nil, rakshaErr
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
func (provider *VLLMProvider) ChatCompletion(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaChatRequest) (*schemas.RakshaChatResponse, *schemas.RakshaError) {
	ctx.SetValue(schemas.RakshaContextKeyPassthroughExtraParams, true)
	baseURL, rakshaErr := provider.baseURLOrError(key)
	if rakshaErr != nil {
		return nil, rakshaErr
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
func (provider *VLLMProvider) ChatCompletionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaChatRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	ctx.SetValue(schemas.RakshaContextKeyPassthroughExtraParams, true)
	baseURL, rakshaErr := provider.baseURLOrError(key)
	if rakshaErr != nil {
		return nil, rakshaErr
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
func (provider *VLLMProvider) Embedding(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaEmbeddingRequest) (*schemas.RakshaEmbeddingResponse, *schemas.RakshaError) {
	baseURL, rakshaErr := provider.baseURLOrError(key)
	if rakshaErr != nil {
		return nil, rakshaErr
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
func (provider *VLLMProvider) Responses(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaResponsesRequest) (*schemas.RakshaResponsesResponse, *schemas.RakshaError) {
	chatResponse, err := provider.ChatCompletion(ctx, key, request.ToChatRequest())
	if err != nil {
		return nil, err
	}
	response := chatResponse.ToRakshaResponsesResponse()
	return response, nil
}

// ResponsesStream performs a streaming responses request to vLLM's API (via chat completion stream).
func (provider *VLLMProvider) ResponsesStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaResponsesRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	ctx.SetValue(schemas.RakshaContextKeyIsResponsesToChatCompletionFallback, true)
	return provider.ChatCompletionStream(
		ctx,
		postHookRunner,
		postHookSpanFinalizer,
		key,
		request.ToChatRequest(),
	)
}

// Speech is not supported by the vLLM provider.
func (provider *VLLMProvider) Speech(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaSpeechRequest) (*schemas.RakshaSpeechResponse, *schemas.RakshaError) {
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
	ctx *schemas.RakshaContext,
	key schemas.Key,
	request *schemas.RakshaRerankRequest,
	endpointPath string,
	jsonData []byte,
) (map[string]interface{}, interface{}, interface{}, []byte, int, time.Duration, *schemas.RakshaError) {
	baseURL, rakshaErr := provider.baseURLOrError(key)
	if rakshaErr != nil {
		return nil, nil, nil, nil, 0, 0, rakshaErr
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

	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, nil, nil, nil, 0, latency, rakshaErr
	}

	statusCode := resp.StatusCode()
	if statusCode != fasthttp.StatusOK {
		rawErrBody := append([]byte(nil), resp.Body()...)
		return nil, nil, nil, rawErrBody, statusCode, latency, providerUtils.SetErrorLatency(openai.ParseOpenAIError(resp), latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		rawErrBody := append([]byte(nil), resp.Body()...)
		return nil, nil, nil, rawErrBody, statusCode, latency, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err)
	}

	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)

	responsePayload := make(map[string]interface{})
	rawRequest, rawResponse, rakshaErr := HandleVLLMResponse(body, &responsePayload, jsonData, sendBackRawRequest, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, nil, nil, body, statusCode, latency, rakshaErr
	}

	return responsePayload, rawRequest, rawResponse, body, statusCode, latency, nil
}

// Rerank performs a rerank request to vLLM's API.
func (provider *VLLMProvider) Rerank(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaRerankRequest) (*schemas.RakshaRerankResponse, *schemas.RakshaError) {
	jsonData, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToVLLMRerankRequest(request), nil
		})
	if rakshaErr != nil {
		return nil, rakshaErr
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

	responsePayload, rawRequest, rawResponse, responseBody, statusCode, latency, rakshaErr := provider.callVLLMRerankEndpoint(ctx, key, request, resolvedPath, jsonData)
	if rakshaErr != nil && !hasPathOverride && isRerankFallbackStatus(statusCode) {
		var fallbackLatency time.Duration
		responsePayload, rawRequest, rawResponse, responseBody, statusCode, fallbackLatency, rakshaErr = provider.callVLLMRerankEndpoint(ctx, key, request, "/rerank", jsonData)
		latency += fallbackLatency
	}
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, responseBody, sendBackRawRequest, sendBackRawResponse, latency)
	}

	returnDocuments := request.Params != nil && request.Params.ReturnDocuments != nil && *request.Params.ReturnDocuments
	rakshaResponse, err := ToRakshaRerankResponse(responsePayload, request.Documents, returnDocuments)
	if err != nil {
		return nil, providerUtils.EnrichError(
			ctx,
			providerUtils.NewRakshaOperationError("error converting rerank response", err),
			jsonData,
			responseBody,
			sendBackRawRequest,
			sendBackRawResponse,
			latency,
		)
	}

	// Keep requested model as the canonical model in Raksha response.
	rakshaResponse.Model = request.Model
	rakshaResponse.ExtraFields.Latency = latency.Milliseconds()

	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		rakshaResponse.ExtraFields.RawRequest = rawRequest
	}
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		rakshaResponse.ExtraFields.RawResponse = rawResponse
	}

	return rakshaResponse, nil
}

// OCR is not supported by the Vllm provider.
func (provider *VLLMProvider) OCR(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaOCRRequest) (*schemas.RakshaOCRResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.OCRRequest, provider.GetProviderKey())
}

// SpeechStream is not supported by the vLLM provider.
func (provider *VLLMProvider) SpeechStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaSpeechRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechStreamRequest, provider.GetProviderKey())
}

// Transcription performs a transcription request to vLLM's API.
func (provider *VLLMProvider) Transcription(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaTranscriptionRequest) (*schemas.RakshaTranscriptionResponse, *schemas.RakshaError) {
	baseURL, rakshaErr := provider.baseURLOrError(key)
	if rakshaErr != nil {
		return nil, rakshaErr
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
func (provider *VLLMProvider) TranscriptionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaTranscriptionRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	baseURL, rakshaErr := provider.baseURLOrError(key)
	if rakshaErr != nil {
		return nil, rakshaErr
	}
	{
		logger := provider.logger
		providerName := provider.GetProviderKey()
		// Use centralized converter
		reqBody := openai.ToOpenAITranscriptionRequest(request)
		if reqBody == nil {
			return nil, providerUtils.NewRakshaOperationError("transcription input is not provided", nil)
		}
		reqBody.Stream = schemas.Ptr(true)

		// Create multipart form
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)

		if rakshaErr := openai.ParseTranscriptionFormDataBodyFromRequest(writer, reqBody, providerName); rakshaErr != nil {
			return nil, rakshaErr
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
			return nil, providerUtils.SetErrorLatency(openai.ParseOpenAIError(resp), latency)
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
						ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
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

				var response schemas.RakshaTranscriptionStreamResponse
				var rakshaErr *schemas.RakshaError

				_, _, rakshaErr = HandleVLLMResponse(dataBytes, &response, nil, false, false)
				if rakshaErr != nil {
					ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
					providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, providerUtils.EnrichError(ctx, rakshaErr, body.Bytes(), dataBytes, false, providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse), latency), responseChan, logger, postHookSpanFinalizer)
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

				response.ExtraFields = schemas.RakshaResponseExtraFields{
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
					ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
					providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(nil, nil, nil, nil, &response, nil), responseChan, postHookSpanFinalizer)
					return
				}

				providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(nil, nil, nil, nil, &response, nil), responseChan, postHookSpanFinalizer)
			}
		}()

		return responseChan, nil
	}
}

// ImageGeneration is not supported by the vLLM provider.
func (provider *VLLMProvider) ImageGeneration(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageGenerationRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationRequest, provider.GetProviderKey())
}

// ImageGenerationStream is not supported by the vLLM provider.
func (provider *VLLMProvider) ImageGenerationStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaImageGenerationRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationStreamRequest, provider.GetProviderKey())
}

// ImageEdit is not supported by the vLLM provider.
func (provider *VLLMProvider) ImageEdit(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageEditRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditRequest, provider.GetProviderKey())
}

// ImageEditStream is not supported by the vLLM provider.
func (provider *VLLMProvider) ImageEditStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaImageEditRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditStreamRequest, provider.GetProviderKey())
}

// ImageVariation is not supported by the vLLM provider.
func (provider *VLLMProvider) ImageVariation(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageVariationRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageVariationRequest, provider.GetProviderKey())
}

// VideoGeneration is not supported by the vLLM provider.
func (provider *VLLMProvider) VideoGeneration(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoGenerationRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoGenerationRequest, provider.GetProviderKey())
}

// VideoRetrieve is not supported by the vLLM provider.
func (provider *VLLMProvider) VideoRetrieve(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoRetrieveRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRetrieveRequest, provider.GetProviderKey())
}

// VideoDownload is not supported by the vLLM provider.
func (provider *VLLMProvider) VideoDownload(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoDownloadRequest) (*schemas.RakshaVideoDownloadResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDownloadRequest, provider.GetProviderKey())
}

// VideoDelete is not supported by the vLLM provider.
func (provider *VLLMProvider) VideoDelete(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoDeleteRequest) (*schemas.RakshaVideoDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDeleteRequest, provider.GetProviderKey())
}

// VideoList is not supported by the vLLM provider.
func (provider *VLLMProvider) VideoList(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoListRequest) (*schemas.RakshaVideoListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoListRequest, provider.GetProviderKey())
}

// VideoRemix is not supported by the vLLM provider.
func (provider *VLLMProvider) VideoRemix(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoRemixRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRemixRequest, provider.GetProviderKey())
}

// FileUpload is not supported by the vLLM provider.
func (provider *VLLMProvider) FileUpload(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaFileUploadRequest) (*schemas.RakshaFileUploadResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileUploadRequest, provider.GetProviderKey())
}

// FileList is not supported by the vLLM provider.
func (provider *VLLMProvider) FileList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileListRequest) (*schemas.RakshaFileListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileListRequest, provider.GetProviderKey())
}

// FileRetrieve is not supported by the vLLM provider.
func (provider *VLLMProvider) FileRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileRetrieveRequest) (*schemas.RakshaFileRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileRetrieveRequest, provider.GetProviderKey())
}

// FileDelete is not supported by the vLLM provider.
func (provider *VLLMProvider) FileDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileDeleteRequest) (*schemas.RakshaFileDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileDeleteRequest, provider.GetProviderKey())
}

// FileContent is not supported by the vLLM provider.
func (provider *VLLMProvider) FileContent(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileContentRequest) (*schemas.RakshaFileContentResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileContentRequest, provider.GetProviderKey())
}

// BatchCreate is not supported by the vLLM provider.
func (provider *VLLMProvider) BatchCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaBatchCreateRequest) (*schemas.RakshaBatchCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCreateRequest, provider.GetProviderKey())
}

// BatchList is not supported by the vLLM provider.
func (provider *VLLMProvider) BatchList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchListRequest) (*schemas.RakshaBatchListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchListRequest, provider.GetProviderKey())
}

// BatchRetrieve is not supported by the vLLM provider.
func (provider *VLLMProvider) BatchRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchRetrieveRequest) (*schemas.RakshaBatchRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchRetrieveRequest, provider.GetProviderKey())
}

// BatchCancel is not supported by the vLLM provider.
func (provider *VLLMProvider) BatchCancel(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchCancelRequest) (*schemas.RakshaBatchCancelResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCancelRequest, provider.GetProviderKey())
}

// BatchDelete is not supported by the vLLM provider.
func (provider *VLLMProvider) BatchDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchDeleteRequest) (*schemas.RakshaBatchDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchDeleteRequest, provider.GetProviderKey())
}

// BatchResults is not supported by the vLLM provider.
func (provider *VLLMProvider) BatchResults(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchResultsRequest) (*schemas.RakshaBatchResultsResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchResultsRequest, provider.GetProviderKey())
}

// CountTokens is not supported by the vLLM provider.
func (provider *VLLMProvider) CountTokens(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaResponsesRequest) (*schemas.RakshaCountTokensResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CountTokensRequest, provider.GetProviderKey())
}

// Compaction is not supported by the vLLM provider.
func (provider *VLLMProvider) Compaction(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaCompactionRequest) (*schemas.RakshaCompactionResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CompactionRequest, provider.GetProviderKey())
}

// ContainerCreate is not supported by the vLLM provider.
func (provider *VLLMProvider) ContainerCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaContainerCreateRequest) (*schemas.RakshaContainerCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerCreateRequest, provider.GetProviderKey())
}

// ContainerList is not supported by the vLLM provider.
func (provider *VLLMProvider) ContainerList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerListRequest) (*schemas.RakshaContainerListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerListRequest, provider.GetProviderKey())
}

// ContainerRetrieve is not supported by the vLLM provider.
func (provider *VLLMProvider) ContainerRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerRetrieveRequest) (*schemas.RakshaContainerRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerRetrieveRequest, provider.GetProviderKey())
}

// ContainerDelete is not supported by the vLLM provider.
func (provider *VLLMProvider) ContainerDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerDeleteRequest) (*schemas.RakshaContainerDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerDeleteRequest, provider.GetProviderKey())
}

// ContainerFileCreate is not supported by the vLLM provider.
func (provider *VLLMProvider) ContainerFileCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaContainerFileCreateRequest) (*schemas.RakshaContainerFileCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileCreateRequest, provider.GetProviderKey())
}

// ContainerFileList is not supported by the vLLM provider.
func (provider *VLLMProvider) ContainerFileList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileListRequest) (*schemas.RakshaContainerFileListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileListRequest, provider.GetProviderKey())
}

// ContainerFileRetrieve is not supported by the vLLM provider.
func (provider *VLLMProvider) ContainerFileRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileRetrieveRequest) (*schemas.RakshaContainerFileRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileRetrieveRequest, provider.GetProviderKey())
}

// ContainerFileContent is not supported by the vLLM provider.
func (provider *VLLMProvider) ContainerFileContent(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileContentRequest) (*schemas.RakshaContainerFileContentResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileContentRequest, provider.GetProviderKey())
}

// ContainerFileDelete is not supported by the vLLM provider.
func (provider *VLLMProvider) ContainerFileDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileDeleteRequest) (*schemas.RakshaContainerFileDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileDeleteRequest, provider.GetProviderKey())
}

// Passthrough is not supported by the vLLM provider.
func (provider *VLLMProvider) Passthrough(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaPassthroughRequest) (*schemas.RakshaPassthroughResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughRequest, provider.GetProviderKey())
}

func (provider *VLLMProvider) PassthroughStream(_ *schemas.RakshaContext, _ schemas.PostHookRunner, _ func(context.Context), _ schemas.Key, _ *schemas.RakshaPassthroughRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughStreamRequest, provider.GetProviderKey())
}
