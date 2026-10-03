package cohere

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"net/http"
	"net/url"

	"github.com/bytedance/sonic"

	providerUtils "github.com/raksha/raksha/core/providers/utils"
	schemas "github.com/raksha/raksha/core/schemas"

	"github.com/valyala/fasthttp"
)

// cohereResponsePool provides a pool for Cohere v2 response objects.
var cohereResponsePool = sync.Pool{
	New: func() interface{} {
		return &CohereChatResponse{}
	},
}

// cohereEmbeddingResponsePool provides a pool for Cohere embedding response objects.
var cohereEmbeddingResponsePool = sync.Pool{
	New: func() interface{} {
		return &CohereEmbeddingResponse{}
	},
}

// acquireCohereEmbeddingResponse gets a Cohere embedding response from the pool and resets it.
func acquireCohereEmbeddingResponse() *CohereEmbeddingResponse {
	resp := cohereEmbeddingResponsePool.Get().(*CohereEmbeddingResponse)
	*resp = CohereEmbeddingResponse{} // Reset the struct
	return resp
}

// releaseCohereEmbeddingResponse returns a Cohere embedding response to the pool.
func releaseCohereEmbeddingResponse(resp *CohereEmbeddingResponse) {
	if resp != nil {
		cohereEmbeddingResponsePool.Put(resp)
	}
}

// cohereRerankResponsePool provides a pool for Cohere rerank response objects.
var cohereRerankResponsePool = sync.Pool{
	New: func() interface{} {
		return &CohereRerankResponse{}
	},
}

// acquireCohereRerankResponse gets a Cohere rerank response from the pool and resets it.
func acquireCohereRerankResponse() *CohereRerankResponse {
	resp := cohereRerankResponsePool.Get().(*CohereRerankResponse)
	*resp = CohereRerankResponse{} // Reset the struct
	return resp
}

// releaseCohereRerankResponse returns a Cohere rerank response to the pool.
func releaseCohereRerankResponse(resp *CohereRerankResponse) {
	if resp != nil {
		cohereRerankResponsePool.Put(resp)
	}
}

// acquireCohereResponse gets a Cohere v2 response from the pool and resets it.
func acquireCohereResponse() *CohereChatResponse {
	resp := cohereResponsePool.Get().(*CohereChatResponse)
	*resp = CohereChatResponse{} // Reset the struct
	return resp
}

// releaseCohereResponse returns a Cohere v2 response to the pool.
func releaseCohereResponse(resp *CohereChatResponse) {
	if resp != nil {
		cohereResponsePool.Put(resp)
	}
}

// CohereProvider implements the Provider interface for Cohere.
type CohereProvider struct {
	logger               schemas.Logger                // Logger for provider operations
	client               *fasthttp.Client              // HTTP client for unary API requests (ReadTimeout bounds overall response)
	streamingClient      *fasthttp.Client              // HTTP client for streaming API requests (no ReadTimeout; idle governed by NewIdleTimeoutReader)
	networkConfig        schemas.NetworkConfig         // Network configuration including extra headers
	sendBackRawRequest   bool                          // Whether to include raw request in RakshaResponse
	sendBackRawResponse  bool                          // Whether to include raw response in RakshaResponse
	customProviderConfig *schemas.CustomProviderConfig // Custom provider config
}

// NewCohereProvider creates a new Cohere provider instance.
// It initializes the HTTP client with the provided configuration and sets up response pools.
// The client is configured with timeouts and connection limits.
func NewCohereProvider(config *schemas.ProviderConfig, logger schemas.Logger) (*CohereProvider, error) {
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

	// Setting proxy and retry policy
	client = providerUtils.ConfigureProxy(client, config.ProxyConfig, logger)
	client = providerUtils.ConfigureDialer(client, config.NetworkConfig.AllowPrivateNetwork)
	client = providerUtils.ConfigureTLS(client, config.NetworkConfig, logger)
	// Pre-warm response pools
	for i := 0; i < config.ConcurrencyAndBufferSize.Concurrency; i++ {
		cohereResponsePool.Put(&CohereChatResponse{})
		cohereEmbeddingResponsePool.Put(&CohereEmbeddingResponse{})
		cohereRerankResponsePool.Put(&CohereRerankResponse{})
	}

	streamingClient := providerUtils.BuildStreamingClient(client)

	// Set default BaseURL if not provided
	if config.NetworkConfig.BaseURL == "" {
		config.NetworkConfig.BaseURL = "https://api.cohere.ai"
	}
	config.NetworkConfig.BaseURL = strings.TrimRight(config.NetworkConfig.BaseURL, "/")

	return &CohereProvider{
		logger:               logger,
		client:               client,
		streamingClient:      streamingClient,
		networkConfig:        config.NetworkConfig,
		customProviderConfig: config.CustomProviderConfig,
		sendBackRawRequest:   config.SendBackRawRequest,
		sendBackRawResponse:  config.SendBackRawResponse,
	}, nil
}

// GetProviderKey returns the provider identifier for Cohere.
func (provider *CohereProvider) GetProviderKey() schemas.ModelProvider {
	return providerUtils.GetProviderName(schemas.Cohere, provider.customProviderConfig)
}

// buildRequestURL constructs the full request URL using the provider's configuration.
func (provider *CohereProvider) buildRequestURL(ctx *schemas.RakshaContext, defaultPath string, requestType schemas.RequestType) string {
	path, isCompleteURL := providerUtils.GetRequestPath(ctx, defaultPath, provider.customProviderConfig, requestType)
	if isCompleteURL {
		return path
	}
	return provider.networkConfig.BaseURL + path
}

// completeRequest sends a request to Cohere's API and handles the response.
// It constructs the API URL, sets up authentication, and processes the response.
// Returns the response body or an error if the request fails.
func (provider *CohereProvider) completeRequest(ctx *schemas.RakshaContext, jsonData []byte, url string, key string) ([]byte, time.Duration, map[string]string, *schemas.RakshaError) {
	// Create the request with the JSON body
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

	req.SetRequestURI(url)
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	usedLargePayloadBody := providerUtils.ApplyLargePayloadRequestBodyWithModelNormalization(ctx, req, schemas.Cohere)
	if !usedLargePayloadBody {
		req.SetBody(jsonData)
	}

	// Send the request with optional large response streaming
	activeClient := providerUtils.PrepareResponseStreaming(ctx, provider.client, resp)
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
	defer wait()
	if usedLargePayloadBody {
		providerUtils.DrainLargePayloadRemainder(ctx)
	}
	if rakshaErr != nil {
		return nil, latency, nil, rakshaErr
	}

	// Extract provider response headers before status check so error responses also forward them
	providerResponseHeaders := providerUtils.ExtractProviderResponseHeaders(resp)

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		return nil, latency, providerResponseHeaders, providerUtils.SetErrorLatency(parseCohereError(resp), latency)
	}

	body, isLargeResp, decodeErr := providerUtils.FinalizeResponseWithLargeDetection(ctx, resp, provider.logger)
	if decodeErr != nil {
		return nil, latency, providerResponseHeaders, decodeErr
	}
	if isLargeResp {
		respOwned = false
		return nil, latency, providerResponseHeaders, nil
	}

	return body, latency, providerResponseHeaders, nil
}

// listModelsByKey performs a list models request for a single key.
// Returns the response and latency, or an error if the request fails.
func (provider *CohereProvider) listModelsByKey(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaListModelsRequest) (*schemas.RakshaListModelsResponse, *schemas.RakshaError) {
	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	// Build base URL first
	baseURL := provider.buildRequestURL(ctx, "/v1/models", schemas.ListModelsRequest)

	// Parse and add query parameters
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError("failed to parse request url", err)
	}

	q := u.Query()
	q.Set("page_size", strconv.Itoa(schemas.DefaultPageSize))
	if request.ExtraParams != nil {
		if endpoint, ok := request.ExtraParams["endpoint"].(string); ok && endpoint != "" {
			q.Set("endpoint", endpoint)
		}
		if defaultOnly, ok := request.ExtraParams["default_only"].(bool); ok && defaultOnly {
			q.Set("default_only", "true")
		}
	}
	u.RawQuery = q.Encode()

	// Set the final URL
	req.SetRequestURI(u.String())
	req.Header.SetMethod(http.MethodGet)
	req.Header.SetContentType("application/json")
	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key.Value.GetValue()))
	}

	// Make request
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Store provider response headers in context before status check so error responses also forward them
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.SetErrorLatency(parseCohereError(resp), latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err)
	}

	// Parse Cohere list models response
	var cohereResponse CohereListModelsResponse
	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(body, &cohereResponse, nil, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Convert Cohere v2 response to Raksha response
	response := cohereResponse.ToRakshaListModelsResponse(provider.GetProviderKey(), key.Models, key.BlacklistedModels, key.Aliases, request.Unfiltered)

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

// ListModels performs a list models request to Cohere's API.
// Requests are made concurrently for improved performance.
func (provider *CohereProvider) ListModels(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaListModelsRequest) (*schemas.RakshaListModelsResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Cohere, provider.customProviderConfig, schemas.ListModelsRequest); err != nil {
		return nil, err
	}
	if provider.customProviderConfig != nil && provider.customProviderConfig.IsKeyLess {
		return providerUtils.HandleKeylessListModelsRequest(provider.GetProviderKey(), func() (*schemas.RakshaListModelsResponse, *schemas.RakshaError) {
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

// TextCompletion is not supported by the Cohere provider.
// Returns an error indicating that text completion is not supported.
func (provider *CohereProvider) TextCompletion(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaTextCompletionRequest) (*schemas.RakshaTextCompletionResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionRequest, provider.GetProviderKey())
}

// TextCompletionStream performs a streaming text completion request to Cohere's API.
// It formats the request, sends it to Cohere, and processes the response.
// Returns a channel of RakshaStreamChunk objects or an error if the request fails.
func (provider *CohereProvider) TextCompletionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaTextCompletionRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionStreamRequest, provider.GetProviderKey())
}

// ChatCompletion performs a chat completion request to the Cohere API using v2 converter.
// It formats the request, sends it to Cohere, and processes the response.
// Returns a RakshaResponse containing the completion results or an error if the request fails.
func (provider *CohereProvider) ChatCompletion(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaChatRequest) (*schemas.RakshaChatResponse, *schemas.RakshaError) {
	// Check if chat completion is allowed
	if err := providerUtils.CheckOperationAllowed(schemas.Cohere, provider.customProviderConfig, schemas.ChatCompletionRequest); err != nil {
		return nil, err
	}

	// Convert to Cohere v2 request
	jsonBody, err := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToCohereChatCompletionRequest(request)
		})
	if err != nil {
		return nil, err
	}

	responseBody, latency, providerResponseHeaders, err := provider.completeRequest(ctx, jsonBody, provider.buildRequestURL(ctx, "/v2/chat", schemas.ChatCompletionRequest), key.Value.GetValue())
	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if err != nil {
		return nil, providerUtils.EnrichError(ctx, err, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Large response mode: return lightweight response with metadata only
	if isLargeResp, _ := ctx.Value(schemas.RakshaContextKeyLargeResponseMode).(bool); isLargeResp {
		return &schemas.RakshaChatResponse{
			Model: request.Model,
			ExtraFields: schemas.RakshaResponseExtraFields{
				Latency:                 latency.Milliseconds(),
				ProviderResponseHeaders: providerResponseHeaders,
			},
		}, nil
	}

	// Create response object from pool
	response := acquireCohereResponse()
	defer releaseCohereResponse(response)

	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(responseBody, response, jsonBody, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	rakshaResponse := response.ToRakshaChatResponse(request.Model)

	// Set ExtraFields
	rakshaResponse.ExtraFields.Latency = latency.Milliseconds()
	rakshaResponse.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	// Set raw request if enabled
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		rakshaResponse.ExtraFields.RawRequest = rawRequest
	}

	// Set raw response if enabled
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		rakshaResponse.ExtraFields.RawResponse = rawResponse
	}

	return rakshaResponse, nil
}

// ChatCompletionStream performs a streaming chat completion request to the Cohere API.
// It supports real-time streaming of responses using Server-Sent Events (SSE).
// Returns a channel containing RakshaResponse objects representing the stream or an error if the request fails.
func (provider *CohereProvider) ChatCompletionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaChatRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	// Check if chat completion stream is allowed
	if err := providerUtils.CheckOperationAllowed(schemas.Cohere, provider.customProviderConfig, schemas.ChatCompletionStreamRequest); err != nil {
		return nil, err
	}

	jsonBody, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			reqBody, err := ToCohereChatCompletionRequest(request)
			if err != nil {
				return nil, err
			}
			reqBody.Stream = schemas.Ptr(true)
			return reqBody, nil
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	sendBackRawRequest := provider.sendBackRawRequest
	sendBackRawResponse := provider.sendBackRawResponse

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	resp.StreamBody = true
	defer fasthttp.ReleaseRequest(req)

	req.Header.SetMethod(http.MethodPost)
	req.SetRequestURI(provider.buildRequestURL(ctx, "/v2/chat", schemas.ChatCompletionStreamRequest))
	req.Header.SetContentType("application/json")

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	// Set headers
	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	usedLargePayloadBody := providerUtils.ApplyLargePayloadRequestBodyWithModelNormalization(ctx, req, schemas.Cohere)
	if !usedLargePayloadBody {
		req.SetBody(jsonBody)
	}

	startTime := time.Now()
	// Make the request
	err := provider.streamingClient.Do(req, resp)
	latency := time.Since(startTime)
	if usedLargePayloadBody {
		providerUtils.DrainLargePayloadRemainder(ctx)
	}
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
		// Request failed before the first response byte (server closed an idle/pooled connection,
		// broken pipe, connection refused, DNS failure, etc.). Surface as a retriable upstream
		// connection error (502) so executeRequestWithRetries honors max_retries, matching the
		// non-streaming path - see https://github.com/raksha/raksha/issues/4496.
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaUpstreamConnectionError(schemas.ErrProviderDoRequest, err), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Extract provider response headers before status check so error responses also forward them
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	// Check for HTTP errors
	if resp.StatusCode() != fasthttp.StatusOK {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		return nil, providerUtils.EnrichError(ctx, parseCohereError(resp), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
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
		chunkIndex := 0
		lastChunkTime := startTime

		var responseID string

		// Running usage handle so a cancel/timeout can bill for tokens
		// already processed. Cohere only reports usage at message-end, so a true
		// mid-stream cancel captures nothing (the API emits no incremental usage);
		// this still bills correctly when usage has arrived before teardown.
		streamUsage := &schemas.RakshaLLMUsage{}
		ctx.SetValue(schemas.RakshaContextKeyStreamAccumulatedUsage, streamUsage)

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
					ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
					provider.logger.Warn("Error reading stream: %v", readErr)
					providerUtils.ProcessAndSendError(ctx, postHookRunner, readErr, responseChan, provider.logger, postHookSpanFinalizer)
					return
				}
				break
			}

			eventData := string(data)

			// Parse the unified streaming event
			var event CohereStreamEvent
			if err := sonic.Unmarshal(data, &event); err != nil {
				provider.logger.Warn("Failed to parse stream event: %v", err)
				continue
			}

			// Extract response ID from message-start events
			if event.Type == StreamEventMessageStart && event.ID != nil {
				responseID = *event.ID
			}

			response, rakshaErr, isLastChunk := event.ToRakshaChatCompletionStream()
			if rakshaErr != nil {
				ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
				providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, rakshaErr, responseChan, provider.logger, postHookSpanFinalizer)
				break
			}
			if response != nil {
				response.ID = responseID
				// keep the running usage handle current.
				if response.Usage != nil {
					*streamUsage = *response.Usage
				}
				response.ExtraFields = schemas.RakshaResponseExtraFields{
					ChunkIndex: chunkIndex,
					Latency:    time.Since(lastChunkTime).Milliseconds(),
				}

				lastChunkTime = time.Now()
				chunkIndex++

				if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
					response.ExtraFields.RawResponse = eventData
				}

				if isLastChunk {
					// Set raw request if enabled
					if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
						providerUtils.ParseAndSetRawRequest(&response.ExtraFields, jsonBody)
					}
					response.ExtraFields.Latency = time.Since(startTime).Milliseconds()
					ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
					providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(nil, response, nil, nil, nil, nil), responseChan, postHookSpanFinalizer)
					break
				}
				providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(nil, response, nil, nil, nil, nil), responseChan, postHookSpanFinalizer)
			}
		}
	}()

	return responseChan, nil
}

// Responses performs a responses request to the Cohere API using v2 converter.
func (provider *CohereProvider) Responses(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaResponsesRequest) (*schemas.RakshaResponsesResponse, *schemas.RakshaError) {
	// Check if chat completion is allowed
	if err := providerUtils.CheckOperationAllowed(schemas.Cohere, provider.customProviderConfig, schemas.ResponsesRequest); err != nil {
		return nil, err
	}

	jsonBody, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToCohereResponsesRequest(request)
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Convert to Cohere v2 request
	responseBody, latency, providerResponseHeaders, err := provider.completeRequest(ctx, jsonBody, provider.buildRequestURL(ctx, "/v2/chat", schemas.ResponsesRequest), key.Value.GetValue())
	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if err != nil {
		return nil, providerUtils.EnrichError(ctx, err, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Large response mode: return lightweight response with metadata only
	if isLargeResp, _ := ctx.Value(schemas.RakshaContextKeyLargeResponseMode).(bool); isLargeResp {
		return &schemas.RakshaResponsesResponse{
			Model: request.Model,
			ExtraFields: schemas.RakshaResponseExtraFields{
				Latency:                 latency.Milliseconds(),
				ProviderResponseHeaders: providerResponseHeaders,
			},
		}, nil
	}

	// Create response object from pool
	response := acquireCohereResponse()
	defer releaseCohereResponse(response)

	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(responseBody, response, jsonBody, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	rakshaResponse := response.ToRakshaResponsesResponse()

	rakshaResponse.Model = request.Model

	// Set ExtraFields
	rakshaResponse.ExtraFields.Latency = latency.Milliseconds()
	rakshaResponse.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	// Set raw request if enabled
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		rakshaResponse.ExtraFields.RawRequest = rawRequest
	}

	// Set raw response if enabled
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		rakshaResponse.ExtraFields.RawResponse = rawResponse
	}

	return rakshaResponse, nil
}

// ResponsesStream performs a streaming responses request to the Cohere API.
func (provider *CohereProvider) ResponsesStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaResponsesRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	// Check if responses stream is allowed
	if err := providerUtils.CheckOperationAllowed(schemas.Cohere, provider.customProviderConfig, schemas.ResponsesStreamRequest); err != nil {
		return nil, err
	}

	// Convert to Cohere v2 request and add streaming
	jsonBody, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			reqBody, err := ToCohereResponsesRequest(request)
			if err != nil {
				return nil, err
			}
			if reqBody != nil {
				reqBody.Stream = schemas.Ptr(true)
			}
			return reqBody, nil
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	sendBackRawRequest := provider.sendBackRawRequest
	sendBackRawResponse := provider.sendBackRawResponse

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	resp.StreamBody = true
	defer fasthttp.ReleaseRequest(req)

	req.Header.SetMethod(http.MethodPost)
	req.SetRequestURI(provider.buildRequestURL(ctx, "/v2/chat", schemas.ResponsesStreamRequest))
	req.Header.SetContentType("application/json")
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	// Set headers
	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	usedLargePayloadBody := providerUtils.ApplyLargePayloadRequestBodyWithModelNormalization(ctx, req, schemas.Cohere)
	if !usedLargePayloadBody {
		req.SetBody(jsonBody)
	}

	startTime := time.Now()
	// Make the request
	err := provider.streamingClient.Do(req, resp)
	latency := time.Since(startTime)
	if usedLargePayloadBody {
		providerUtils.DrainLargePayloadRemainder(ctx)
	}
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
		// Request failed before the first response byte (server closed an idle/pooled connection,
		// broken pipe, connection refused, DNS failure, etc.). Surface as a retriable upstream
		// connection error (502) so executeRequestWithRetries honors max_retries, matching the
		// non-streaming path - see https://github.com/raksha/raksha/issues/4496.
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaUpstreamConnectionError(schemas.ErrProviderDoRequest, err), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Extract provider response headers before status check so error responses also forward them
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	// Check for HTTP errors
	if resp.StatusCode() != fasthttp.StatusOK {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		return nil, providerUtils.EnrichError(ctx, parseCohereError(resp), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
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

		chunkIndex := 0

		lastChunkTime := startTime

		// Create stream state for stateful conversions (outside loop to persist across events)
		streamState := acquireCohereResponsesStreamState()
		streamState.Model = &request.Model
		defer releaseCohereResponsesStreamState(streamState)

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
					ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
					provider.logger.Warn("Error reading stream: %v", readErr)
					providerUtils.ProcessAndSendError(ctx, postHookRunner, readErr, responseChan, provider.logger, postHookSpanFinalizer)
					return
				}
				break
			}

			eventData := string(data)

			// Parse the unified streaming event
			var event CohereStreamEvent
			if err := sonic.Unmarshal(data, &event); err != nil {
				provider.logger.Warn("Failed to parse stream event: %v", err)
				continue
			}

			// Note: response.created and response.in_progress are now emitted by ToRakshaResponsesStream
			// from the message_start event, so we don't need to call them manually here

			responses, rakshaErr, isLastChunk := event.ToRakshaResponsesStream(chunkIndex, streamState)
			if rakshaErr != nil {
				ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
				providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, rakshaErr, responseChan, provider.logger, postHookSpanFinalizer)
				break
			}
			// Handle each response in the slice
			for i, response := range responses {
				if response != nil {
					response.ExtraFields = schemas.RakshaResponseExtraFields{
						ChunkIndex: chunkIndex,
						Latency:    time.Since(lastChunkTime).Milliseconds(),
					}
					lastChunkTime = time.Now()
					chunkIndex++

					if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
						response.ExtraFields.RawResponse = eventData
					}

					if isLastChunk && i == len(responses)-1 {
						if response.Response == nil {
							response.Response = &schemas.RakshaResponsesResponse{}
						}
						// Set raw request if enabled
						if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
							providerUtils.ParseAndSetRawRequest(&response.ExtraFields, jsonBody)
						}
						response.ExtraFields.Latency = time.Since(startTime).Milliseconds()
						ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
						providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(nil, nil, response, nil, nil, nil), responseChan, postHookSpanFinalizer)
						return
					}
					providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(nil, nil, response, nil, nil, nil), responseChan, postHookSpanFinalizer)
				}
			}
		}
	}()

	return responseChan, nil
}

// Embedding generates embeddings for the given input text(s) using the Cohere API.
// Supports Cohere's embedding models and returns a RakshaResponse containing the embedding(s).
func (provider *CohereProvider) Embedding(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaEmbeddingRequest) (*schemas.RakshaEmbeddingResponse, *schemas.RakshaError) {
	// Check if embedding is allowed
	if err := providerUtils.CheckOperationAllowed(schemas.Cohere, provider.customProviderConfig, schemas.EmbeddingRequest); err != nil {
		return nil, err
	}

	jsonBody, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToCohereEmbeddingRequest(request), nil
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Create Raksha request for conversion
	responseBody, latency, providerResponseHeaders, err := provider.completeRequest(ctx, jsonBody, provider.buildRequestURL(ctx, "/v2/embed", schemas.EmbeddingRequest), key.Value.GetValue())
	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if err != nil {
		return nil, providerUtils.EnrichError(ctx, err, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Large response mode: return lightweight response with metadata only
	if isLargeResp, _ := ctx.Value(schemas.RakshaContextKeyLargeResponseMode).(bool); isLargeResp {
		return &schemas.RakshaEmbeddingResponse{
			Model: request.Model,
			ExtraFields: schemas.RakshaResponseExtraFields{
				Latency:                 latency.Milliseconds(),
				ProviderResponseHeaders: providerResponseHeaders,
			},
		}, nil
	}

	// Create response object from pool
	response := acquireCohereEmbeddingResponse()
	defer releaseCohereEmbeddingResponse(response)

	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(responseBody, response, jsonBody, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	rakshaResponse := response.ToRakshaEmbeddingResponse()

	// Set ExtraFields
	rakshaResponse.ExtraFields.Latency = latency.Milliseconds()
	rakshaResponse.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	// Set raw request if enabled
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		rakshaResponse.ExtraFields.RawRequest = rawRequest
	}

	// Set raw response if enabled
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		rakshaResponse.ExtraFields.RawResponse = rawResponse
	}

	return rakshaResponse, nil
}

// Rerank performs a rerank request using the Cohere /v2/rerank API.
func (provider *CohereProvider) Rerank(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaRerankRequest) (*schemas.RakshaRerankResponse, *schemas.RakshaError) {
	// Check if rerank is allowed
	if err := providerUtils.CheckOperationAllowed(schemas.Cohere, provider.customProviderConfig, schemas.RerankRequest); err != nil {
		return nil, err
	}

	jsonBody, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToCohereRerankRequest(request), nil
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	responseBody, latency, providerResponseHeaders, err := provider.completeRequest(ctx, jsonBody, provider.buildRequestURL(ctx, "/v2/rerank", schemas.RerankRequest), key.Value.GetValue())
	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if err != nil {
		return nil, providerUtils.EnrichError(ctx, err, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Large response mode: return lightweight response with metadata only
	if isLargeResp, _ := ctx.Value(schemas.RakshaContextKeyLargeResponseMode).(bool); isLargeResp {
		return &schemas.RakshaRerankResponse{
			Model: request.Model,
			ExtraFields: schemas.RakshaResponseExtraFields{
				Latency:                 latency.Milliseconds(),
				ProviderResponseHeaders: providerResponseHeaders,
			},
		}, nil
	}

	// Create response object from pool
	response := acquireCohereRerankResponse()
	defer releaseCohereRerankResponse(response)

	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(responseBody, response, jsonBody, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	returnDocuments := request.Params != nil && request.Params.ReturnDocuments != nil && *request.Params.ReturnDocuments
	rakshaResponse := response.ToRakshaRerankResponse(request.Documents, returnDocuments)
	rakshaResponse.Model = request.Model

	// Set ExtraFields
	rakshaResponse.ExtraFields.Latency = latency.Milliseconds()
	rakshaResponse.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	// Set raw request if enabled
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		rakshaResponse.ExtraFields.RawRequest = rawRequest
	}

	// Set raw response if enabled
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		rakshaResponse.ExtraFields.RawResponse = rawResponse
	}

	return rakshaResponse, nil
}

// OCR is not supported by the Cohere provider.
func (provider *CohereProvider) OCR(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaOCRRequest) (*schemas.RakshaOCRResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.OCRRequest, provider.GetProviderKey())
}

// Speech is not supported by the Cohere provider.
func (provider *CohereProvider) Speech(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaSpeechRequest) (*schemas.RakshaSpeechResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechRequest, provider.GetProviderKey())
}

// SpeechStream is not supported by the Cohere provider.
func (provider *CohereProvider) SpeechStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaSpeechRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechStreamRequest, provider.GetProviderKey())
}

// Transcription is not supported by the Cohere provider.
func (provider *CohereProvider) Transcription(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaTranscriptionRequest) (*schemas.RakshaTranscriptionResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionRequest, provider.GetProviderKey())
}

// TranscriptionStream is not supported by the Cohere provider.
func (provider *CohereProvider) TranscriptionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaTranscriptionRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionStreamRequest, provider.GetProviderKey())
}

// ImageGeneration is not supported by the Cohere provider.
func (provider *CohereProvider) ImageGeneration(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageGenerationRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationRequest, provider.GetProviderKey())
}

// ImageGenerationStream is not supported by the Cohere provider.
func (provider *CohereProvider) ImageGenerationStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaImageGenerationRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationStreamRequest, provider.GetProviderKey())
}

// ImageEdit is not supported by the Cohere provider.
func (provider *CohereProvider) ImageEdit(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageEditRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditRequest, provider.GetProviderKey())
}

// ImageEditStream is not supported by the Cohere provider.
func (provider *CohereProvider) ImageEditStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaImageEditRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditStreamRequest, provider.GetProviderKey())
}

// ImageVariation is not supported by the Cohere provider.
func (provider *CohereProvider) ImageVariation(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageVariationRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageVariationRequest, provider.GetProviderKey())
}

// VideoGeneration is not supported by the Cohere provider.
func (provider *CohereProvider) VideoGeneration(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoGenerationRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoGenerationRequest, provider.GetProviderKey())
}

// VideoRetrieve is not supported by the Cohere provider.
func (provider *CohereProvider) VideoRetrieve(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoRetrieveRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRetrieveRequest, provider.GetProviderKey())
}

// VideoDownload is not supported by the Cohere provider.
func (provider *CohereProvider) VideoDownload(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoDownloadRequest) (*schemas.RakshaVideoDownloadResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDownloadRequest, provider.GetProviderKey())
}

// VideoDelete is not supported by Cohere provider.
func (provider *CohereProvider) VideoDelete(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoDeleteRequest) (*schemas.RakshaVideoDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDeleteRequest, provider.GetProviderKey())
}

// VideoList is not supported by Cohere provider.
func (provider *CohereProvider) VideoList(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoListRequest) (*schemas.RakshaVideoListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoListRequest, provider.GetProviderKey())
}

// VideoRemix is not supported by Cohere provider.
func (provider *CohereProvider) VideoRemix(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoRemixRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRemixRequest, provider.GetProviderKey())
}

// BatchCreate is not supported by Cohere provider.
func (provider *CohereProvider) BatchCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaBatchCreateRequest) (*schemas.RakshaBatchCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCreateRequest, provider.GetProviderKey())
}

// BatchList is not supported by Cohere provider.
func (provider *CohereProvider) BatchList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchListRequest) (*schemas.RakshaBatchListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchListRequest, provider.GetProviderKey())
}

// BatchRetrieve is not supported by Cohere provider.
func (provider *CohereProvider) BatchRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchRetrieveRequest) (*schemas.RakshaBatchRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchRetrieveRequest, provider.GetProviderKey())
}

// BatchCancel is not supported by Cohere provider.
func (provider *CohereProvider) BatchCancel(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchCancelRequest) (*schemas.RakshaBatchCancelResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCancelRequest, provider.GetProviderKey())
}

// BatchDelete is not supported by Cohere provider.
func (provider *CohereProvider) BatchDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchDeleteRequest) (*schemas.RakshaBatchDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchDeleteRequest, provider.GetProviderKey())
}

// BatchResults is not supported by Cohere provider.
func (provider *CohereProvider) BatchResults(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchResultsRequest) (*schemas.RakshaBatchResultsResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchResultsRequest, provider.GetProviderKey())
}

// FileUpload is not supported by Cohere provider.
func (provider *CohereProvider) FileUpload(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaFileUploadRequest) (*schemas.RakshaFileUploadResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileUploadRequest, provider.GetProviderKey())
}

// FileList is not supported by Cohere provider.
func (provider *CohereProvider) FileList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileListRequest) (*schemas.RakshaFileListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileListRequest, provider.GetProviderKey())
}

// FileRetrieve is not supported by Cohere provider.
func (provider *CohereProvider) FileRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileRetrieveRequest) (*schemas.RakshaFileRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileRetrieveRequest, provider.GetProviderKey())
}

// FileDelete is not supported by Cohere provider.
func (provider *CohereProvider) FileDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileDeleteRequest) (*schemas.RakshaFileDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileDeleteRequest, provider.GetProviderKey())
}

// FileContent is not supported by Cohere provider.
func (provider *CohereProvider) FileContent(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileContentRequest) (*schemas.RakshaFileContentResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileContentRequest, provider.GetProviderKey())
}

// CountTokens performs a token counting request via Cohere's /v1/tokenize API.
func (provider *CohereProvider) CountTokens(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaResponsesRequest) (*schemas.RakshaCountTokensResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Cohere, provider.customProviderConfig, schemas.CountTokensRequest); err != nil {
		return nil, err
	}

	jsonBody, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToCohereCountTokensRequest(request)
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	responseBody, latency, providerResponseHeaders, rakshaErr := provider.completeRequest(
		ctx,
		jsonBody,
		provider.buildRequestURL(ctx, "/v1/tokenize", schemas.CountTokensRequest),
		key.Value.GetValue(),
	)
	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Large response mode: return lightweight response with metadata only
	if isLargeResp, _ := ctx.Value(schemas.RakshaContextKeyLargeResponseMode).(bool); isLargeResp {
		return &schemas.RakshaCountTokensResponse{
			Model: request.Model,
			ExtraFields: schemas.RakshaResponseExtraFields{
				Latency:                 latency.Milliseconds(),
				ProviderResponseHeaders: providerResponseHeaders,
			},
		}, nil
	}

	cohereResponse := &CohereCountTokensResponse{}

	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(
		responseBody,
		cohereResponse,
		jsonBody,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
	)
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	rakshaResponse := cohereResponse.ToRakshaCountTokensResponse(request.Model)
	if rakshaResponse == nil {
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, fmt.Errorf("nil cohere count tokens response")), jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	rakshaResponse.ExtraFields.Latency = latency.Milliseconds()
	rakshaResponse.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		rakshaResponse.ExtraFields.RawRequest = rawRequest
	}

	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		rakshaResponse.ExtraFields.RawResponse = rawResponse
	}

	return rakshaResponse, nil
}

// Compaction is not supported by the Cohere provider.
func (provider *CohereProvider) Compaction(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaCompactionRequest) (*schemas.RakshaCompactionResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CompactionRequest, provider.GetProviderKey())
}

// ContainerCreate is not supported by the Cohere provider.
func (provider *CohereProvider) ContainerCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaContainerCreateRequest) (*schemas.RakshaContainerCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerCreateRequest, provider.GetProviderKey())
}

// ContainerList is not supported by the Cohere provider.
func (provider *CohereProvider) ContainerList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerListRequest) (*schemas.RakshaContainerListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerListRequest, provider.GetProviderKey())
}

// ContainerRetrieve is not supported by the Cohere provider.
func (provider *CohereProvider) ContainerRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerRetrieveRequest) (*schemas.RakshaContainerRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerRetrieveRequest, provider.GetProviderKey())
}

// ContainerDelete is not supported by the Cohere provider.
func (provider *CohereProvider) ContainerDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerDeleteRequest) (*schemas.RakshaContainerDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerDeleteRequest, provider.GetProviderKey())
}

// ContainerFileCreate is not supported by the Cohere provider.
func (provider *CohereProvider) ContainerFileCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaContainerFileCreateRequest) (*schemas.RakshaContainerFileCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileCreateRequest, provider.GetProviderKey())
}

// ContainerFileList is not supported by the Cohere provider.
func (provider *CohereProvider) ContainerFileList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileListRequest) (*schemas.RakshaContainerFileListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileListRequest, provider.GetProviderKey())
}

// ContainerFileRetrieve is not supported by the Cohere provider.
func (provider *CohereProvider) ContainerFileRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileRetrieveRequest) (*schemas.RakshaContainerFileRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileRetrieveRequest, provider.GetProviderKey())
}

// ContainerFileContent is not supported by the Cohere provider.
func (provider *CohereProvider) ContainerFileContent(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileContentRequest) (*schemas.RakshaContainerFileContentResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileContentRequest, provider.GetProviderKey())
}

// ContainerFileDelete is not supported by the Cohere provider.
func (provider *CohereProvider) ContainerFileDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileDeleteRequest) (*schemas.RakshaContainerFileDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileDeleteRequest, provider.GetProviderKey())
}

// Passthrough is not supported by the Cohere provider.
func (provider *CohereProvider) Passthrough(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaPassthroughRequest) (*schemas.RakshaPassthroughResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughRequest, provider.GetProviderKey())
}

func (provider *CohereProvider) PassthroughStream(_ *schemas.RakshaContext, _ schemas.PostHookRunner, _ func(context.Context), _ schemas.Key, _ *schemas.RakshaPassthroughRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughStreamRequest, provider.GetProviderKey())
}
