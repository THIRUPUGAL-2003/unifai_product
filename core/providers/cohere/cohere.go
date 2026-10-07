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

	providerUtils "github.com/gateway/gateway/core/providers/utils"
	schemas "github.com/gateway/gateway/core/schemas"

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
	sendBackRawRequest   bool                          // Whether to include raw request in GatewayResponse
	sendBackRawResponse  bool                          // Whether to include raw response in GatewayResponse
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
func (provider *CohereProvider) buildRequestURL(ctx *schemas.GatewayContext, defaultPath string, requestType schemas.RequestType) string {
	path, isCompleteURL := providerUtils.GetRequestPath(ctx, defaultPath, provider.customProviderConfig, requestType)
	if isCompleteURL {
		return path
	}
	return provider.networkConfig.BaseURL + path
}

// completeRequest sends a request to Cohere's API and handles the response.
// It constructs the API URL, sets up authentication, and processes the response.
// Returns the response body or an error if the request fails.
func (provider *CohereProvider) completeRequest(ctx *schemas.GatewayContext, jsonData []byte, url string, key string) ([]byte, time.Duration, map[string]string, *schemas.GatewayError) {
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
	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
	defer wait()
	if usedLargePayloadBody {
		providerUtils.DrainLargePayloadRemainder(ctx)
	}
	if gatewayErr != nil {
		return nil, latency, nil, gatewayErr
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
func (provider *CohereProvider) listModelsByKey(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayListModelsRequest) (*schemas.GatewayListModelsResponse, *schemas.GatewayError) {
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
		return nil, providerUtils.NewGatewayOperationError("failed to parse request url", err)
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
	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	// Store provider response headers in context before status check so error responses also forward them
	ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.SetErrorLatency(parseCohereError(resp), latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewGatewayOperationError(schemas.ErrProviderResponseDecode, err)
	}

	// Parse Cohere list models response
	var cohereResponse CohereListModelsResponse
	rawRequest, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(body, &cohereResponse, nil, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	// Convert Cohere v2 response to Gateway response
	response := cohereResponse.ToGatewayListModelsResponse(provider.GetProviderKey(), key.Models, key.BlacklistedModels, key.Aliases, request.Unfiltered)

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
func (provider *CohereProvider) ListModels(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayListModelsRequest) (*schemas.GatewayListModelsResponse, *schemas.GatewayError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Cohere, provider.customProviderConfig, schemas.ListModelsRequest); err != nil {
		return nil, err
	}
	if provider.customProviderConfig != nil && provider.customProviderConfig.IsKeyLess {
		return providerUtils.HandleKeylessListModelsRequest(provider.GetProviderKey(), func() (*schemas.GatewayListModelsResponse, *schemas.GatewayError) {
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
func (provider *CohereProvider) TextCompletion(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayTextCompletionRequest) (*schemas.GatewayTextCompletionResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionRequest, provider.GetProviderKey())
}

// TextCompletionStream performs a streaming text completion request to Cohere's API.
// It formats the request, sends it to Cohere, and processes the response.
// Returns a channel of GatewayStreamChunk objects or an error if the request fails.
func (provider *CohereProvider) TextCompletionStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayTextCompletionRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionStreamRequest, provider.GetProviderKey())
}

// ChatCompletion performs a chat completion request to the Cohere API using v2 converter.
// It formats the request, sends it to Cohere, and processes the response.
// Returns a GatewayResponse containing the completion results or an error if the request fails.
func (provider *CohereProvider) ChatCompletion(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayChatRequest) (*schemas.GatewayChatResponse, *schemas.GatewayError) {
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
		ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if err != nil {
		return nil, providerUtils.EnrichError(ctx, err, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Large response mode: return lightweight response with metadata only
	if isLargeResp, _ := ctx.Value(schemas.GatewayContextKeyLargeResponseMode).(bool); isLargeResp {
		return &schemas.GatewayChatResponse{
			Model: request.Model,
			ExtraFields: schemas.GatewayResponseExtraFields{
				Latency:                 latency.Milliseconds(),
				ProviderResponseHeaders: providerResponseHeaders,
			},
		}, nil
	}

	// Create response object from pool
	response := acquireCohereResponse()
	defer releaseCohereResponse(response)

	rawRequest, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(responseBody, response, jsonBody, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if gatewayErr != nil {
		return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	gatewayResponse := response.ToGatewayChatResponse(request.Model)

	// Set ExtraFields
	gatewayResponse.ExtraFields.Latency = latency.Milliseconds()
	gatewayResponse.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	// Set raw request if enabled
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		gatewayResponse.ExtraFields.RawRequest = rawRequest
	}

	// Set raw response if enabled
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		gatewayResponse.ExtraFields.RawResponse = rawResponse
	}

	return gatewayResponse, nil
}

// ChatCompletionStream performs a streaming chat completion request to the Cohere API.
// It supports real-time streaming of responses using Server-Sent Events (SSE).
// Returns a channel containing GatewayResponse objects representing the stream or an error if the request fails.
func (provider *CohereProvider) ChatCompletionStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayChatRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	// Check if chat completion stream is allowed
	if err := providerUtils.CheckOperationAllowed(schemas.Cohere, provider.customProviderConfig, schemas.ChatCompletionStreamRequest); err != nil {
		return nil, err
	}

	jsonBody, gatewayErr := providerUtils.CheckContextAndGetRequestBody(
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
	if gatewayErr != nil {
		return nil, gatewayErr
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
			return nil, providerUtils.EnrichError(ctx, &schemas.GatewayError{
				IsGatewayError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   err,
				},
			}, jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
		}
		if errors.Is(err, fasthttp.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
			return nil, providerUtils.EnrichError(ctx, providerUtils.NewGatewayTimeoutError(schemas.ErrProviderRequestTimedOut, err), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
		}
		// Request failed before the first response byte (server closed an idle/pooled connection,
		// broken pipe, connection refused, DNS failure, etc.). Surface as a retriable upstream
		// connection error (502) so executeRequestWithRetries honors max_retries, matching the
		// non-streaming path - see https://github.com/gateway/gateway/issues/4496.
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewGatewayUpstreamConnectionError(schemas.ErrProviderDoRequest, err), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Extract provider response headers before status check so error responses also forward them
	ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	// Check for HTTP errors
	if resp.StatusCode() != fasthttp.StatusOK {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		return nil, providerUtils.EnrichError(ctx, parseCohereError(resp), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
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
		streamUsage := &schemas.GatewayLLMUsage{}
		ctx.SetValue(schemas.GatewayContextKeyStreamAccumulatedUsage, streamUsage)

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
					ctx.SetValue(schemas.GatewayContextKeyStreamEndIndicator, true)
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

			response, gatewayErr, isLastChunk := event.ToGatewayChatCompletionStream()
			if gatewayErr != nil {
				ctx.SetValue(schemas.GatewayContextKeyStreamEndIndicator, true)
				providerUtils.ProcessAndSendGatewayError(ctx, postHookRunner, gatewayErr, responseChan, provider.logger, postHookSpanFinalizer)
				break
			}
			if response != nil {
				response.ID = responseID
				// keep the running usage handle current.
				if response.Usage != nil {
					*streamUsage = *response.Usage
				}
				response.ExtraFields = schemas.GatewayResponseExtraFields{
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
					ctx.SetValue(schemas.GatewayContextKeyStreamEndIndicator, true)
					providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetGatewayResponseForStreamResponse(nil, response, nil, nil, nil, nil), responseChan, postHookSpanFinalizer)
					break
				}
				providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetGatewayResponseForStreamResponse(nil, response, nil, nil, nil, nil), responseChan, postHookSpanFinalizer)
			}
		}
	}()

	return responseChan, nil
}

// Responses performs a responses request to the Cohere API using v2 converter.
func (provider *CohereProvider) Responses(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayResponsesRequest) (*schemas.GatewayResponsesResponse, *schemas.GatewayError) {
	// Check if chat completion is allowed
	if err := providerUtils.CheckOperationAllowed(schemas.Cohere, provider.customProviderConfig, schemas.ResponsesRequest); err != nil {
		return nil, err
	}

	jsonBody, gatewayErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToCohereResponsesRequest(request)
		})
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	// Convert to Cohere v2 request
	responseBody, latency, providerResponseHeaders, err := provider.completeRequest(ctx, jsonBody, provider.buildRequestURL(ctx, "/v2/chat", schemas.ResponsesRequest), key.Value.GetValue())
	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if err != nil {
		return nil, providerUtils.EnrichError(ctx, err, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Large response mode: return lightweight response with metadata only
	if isLargeResp, _ := ctx.Value(schemas.GatewayContextKeyLargeResponseMode).(bool); isLargeResp {
		return &schemas.GatewayResponsesResponse{
			Model: request.Model,
			ExtraFields: schemas.GatewayResponseExtraFields{
				Latency:                 latency.Milliseconds(),
				ProviderResponseHeaders: providerResponseHeaders,
			},
		}, nil
	}

	// Create response object from pool
	response := acquireCohereResponse()
	defer releaseCohereResponse(response)

	rawRequest, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(responseBody, response, jsonBody, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if gatewayErr != nil {
		return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	gatewayResponse := response.ToGatewayResponsesResponse()

	gatewayResponse.Model = request.Model

	// Set ExtraFields
	gatewayResponse.ExtraFields.Latency = latency.Milliseconds()
	gatewayResponse.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	// Set raw request if enabled
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		gatewayResponse.ExtraFields.RawRequest = rawRequest
	}

	// Set raw response if enabled
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		gatewayResponse.ExtraFields.RawResponse = rawResponse
	}

	return gatewayResponse, nil
}

// ResponsesStream performs a streaming responses request to the Cohere API.
func (provider *CohereProvider) ResponsesStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayResponsesRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	// Check if responses stream is allowed
	if err := providerUtils.CheckOperationAllowed(schemas.Cohere, provider.customProviderConfig, schemas.ResponsesStreamRequest); err != nil {
		return nil, err
	}

	// Convert to Cohere v2 request and add streaming
	jsonBody, gatewayErr := providerUtils.CheckContextAndGetRequestBody(
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
	if gatewayErr != nil {
		return nil, gatewayErr
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
			return nil, providerUtils.EnrichError(ctx, &schemas.GatewayError{
				IsGatewayError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   err,
				},
			}, jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
		}
		if errors.Is(err, fasthttp.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
			return nil, providerUtils.EnrichError(ctx, providerUtils.NewGatewayTimeoutError(schemas.ErrProviderRequestTimedOut, err), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
		}
		// Request failed before the first response byte (server closed an idle/pooled connection,
		// broken pipe, connection refused, DNS failure, etc.). Surface as a retriable upstream
		// connection error (502) so executeRequestWithRetries honors max_retries, matching the
		// non-streaming path - see https://github.com/gateway/gateway/issues/4496.
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewGatewayUpstreamConnectionError(schemas.ErrProviderDoRequest, err), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Extract provider response headers before status check so error responses also forward them
	ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	// Check for HTTP errors
	if resp.StatusCode() != fasthttp.StatusOK {
		defer providerUtils.ReleaseStreamingResponse(ctx, resp)
		return nil, providerUtils.EnrichError(ctx, parseCohereError(resp), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
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
					ctx.SetValue(schemas.GatewayContextKeyStreamEndIndicator, true)
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

			// Note: response.created and response.in_progress are now emitted by ToGatewayResponsesStream
			// from the message_start event, so we don't need to call them manually here

			responses, gatewayErr, isLastChunk := event.ToGatewayResponsesStream(chunkIndex, streamState)
			if gatewayErr != nil {
				ctx.SetValue(schemas.GatewayContextKeyStreamEndIndicator, true)
				providerUtils.ProcessAndSendGatewayError(ctx, postHookRunner, gatewayErr, responseChan, provider.logger, postHookSpanFinalizer)
				break
			}
			// Handle each response in the slice
			for i, response := range responses {
				if response != nil {
					response.ExtraFields = schemas.GatewayResponseExtraFields{
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
							response.Response = &schemas.GatewayResponsesResponse{}
						}
						// Set raw request if enabled
						if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
							providerUtils.ParseAndSetRawRequest(&response.ExtraFields, jsonBody)
						}
						response.ExtraFields.Latency = time.Since(startTime).Milliseconds()
						ctx.SetValue(schemas.GatewayContextKeyStreamEndIndicator, true)
						providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetGatewayResponseForStreamResponse(nil, nil, response, nil, nil, nil), responseChan, postHookSpanFinalizer)
						return
					}
					providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetGatewayResponseForStreamResponse(nil, nil, response, nil, nil, nil), responseChan, postHookSpanFinalizer)
				}
			}
		}
	}()

	return responseChan, nil
}

// Embedding generates embeddings for the given input text(s) using the Cohere API.
// Supports Cohere's embedding models and returns a GatewayResponse containing the embedding(s).
func (provider *CohereProvider) Embedding(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayEmbeddingRequest) (*schemas.GatewayEmbeddingResponse, *schemas.GatewayError) {
	// Check if embedding is allowed
	if err := providerUtils.CheckOperationAllowed(schemas.Cohere, provider.customProviderConfig, schemas.EmbeddingRequest); err != nil {
		return nil, err
	}

	jsonBody, gatewayErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToCohereEmbeddingRequest(request), nil
		})
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	// Create Gateway request for conversion
	responseBody, latency, providerResponseHeaders, err := provider.completeRequest(ctx, jsonBody, provider.buildRequestURL(ctx, "/v2/embed", schemas.EmbeddingRequest), key.Value.GetValue())
	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if err != nil {
		return nil, providerUtils.EnrichError(ctx, err, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Large response mode: return lightweight response with metadata only
	if isLargeResp, _ := ctx.Value(schemas.GatewayContextKeyLargeResponseMode).(bool); isLargeResp {
		return &schemas.GatewayEmbeddingResponse{
			Model: request.Model,
			ExtraFields: schemas.GatewayResponseExtraFields{
				Latency:                 latency.Milliseconds(),
				ProviderResponseHeaders: providerResponseHeaders,
			},
		}, nil
	}

	// Create response object from pool
	response := acquireCohereEmbeddingResponse()
	defer releaseCohereEmbeddingResponse(response)

	rawRequest, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(responseBody, response, jsonBody, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if gatewayErr != nil {
		return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	gatewayResponse := response.ToGatewayEmbeddingResponse()

	// Set ExtraFields
	gatewayResponse.ExtraFields.Latency = latency.Milliseconds()
	gatewayResponse.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	// Set raw request if enabled
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		gatewayResponse.ExtraFields.RawRequest = rawRequest
	}

	// Set raw response if enabled
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		gatewayResponse.ExtraFields.RawResponse = rawResponse
	}

	return gatewayResponse, nil
}

// Rerank performs a rerank request using the Cohere /v2/rerank API.
func (provider *CohereProvider) Rerank(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayRerankRequest) (*schemas.GatewayRerankResponse, *schemas.GatewayError) {
	// Check if rerank is allowed
	if err := providerUtils.CheckOperationAllowed(schemas.Cohere, provider.customProviderConfig, schemas.RerankRequest); err != nil {
		return nil, err
	}

	jsonBody, gatewayErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToCohereRerankRequest(request), nil
		})
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	responseBody, latency, providerResponseHeaders, err := provider.completeRequest(ctx, jsonBody, provider.buildRequestURL(ctx, "/v2/rerank", schemas.RerankRequest), key.Value.GetValue())
	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if err != nil {
		return nil, providerUtils.EnrichError(ctx, err, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Large response mode: return lightweight response with metadata only
	if isLargeResp, _ := ctx.Value(schemas.GatewayContextKeyLargeResponseMode).(bool); isLargeResp {
		return &schemas.GatewayRerankResponse{
			Model: request.Model,
			ExtraFields: schemas.GatewayResponseExtraFields{
				Latency:                 latency.Milliseconds(),
				ProviderResponseHeaders: providerResponseHeaders,
			},
		}, nil
	}

	// Create response object from pool
	response := acquireCohereRerankResponse()
	defer releaseCohereRerankResponse(response)

	rawRequest, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(responseBody, response, jsonBody, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if gatewayErr != nil {
		return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	returnDocuments := request.Params != nil && request.Params.ReturnDocuments != nil && *request.Params.ReturnDocuments
	gatewayResponse := response.ToGatewayRerankResponse(request.Documents, returnDocuments)
	gatewayResponse.Model = request.Model

	// Set ExtraFields
	gatewayResponse.ExtraFields.Latency = latency.Milliseconds()
	gatewayResponse.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	// Set raw request if enabled
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		gatewayResponse.ExtraFields.RawRequest = rawRequest
	}

	// Set raw response if enabled
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		gatewayResponse.ExtraFields.RawResponse = rawResponse
	}

	return gatewayResponse, nil
}

// OCR is not supported by the Cohere provider.
func (provider *CohereProvider) OCR(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayOCRRequest) (*schemas.GatewayOCRResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.OCRRequest, provider.GetProviderKey())
}

// Speech is not supported by the Cohere provider.
func (provider *CohereProvider) Speech(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewaySpeechRequest) (*schemas.GatewaySpeechResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechRequest, provider.GetProviderKey())
}

// SpeechStream is not supported by the Cohere provider.
func (provider *CohereProvider) SpeechStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewaySpeechRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechStreamRequest, provider.GetProviderKey())
}

// Transcription is not supported by the Cohere provider.
func (provider *CohereProvider) Transcription(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayTranscriptionRequest) (*schemas.GatewayTranscriptionResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionRequest, provider.GetProviderKey())
}

// TranscriptionStream is not supported by the Cohere provider.
func (provider *CohereProvider) TranscriptionStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayTranscriptionRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionStreamRequest, provider.GetProviderKey())
}

// ImageGeneration is not supported by the Cohere provider.
func (provider *CohereProvider) ImageGeneration(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayImageGenerationRequest) (*schemas.GatewayImageGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationRequest, provider.GetProviderKey())
}

// ImageGenerationStream is not supported by the Cohere provider.
func (provider *CohereProvider) ImageGenerationStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayImageGenerationRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationStreamRequest, provider.GetProviderKey())
}

// ImageEdit is not supported by the Cohere provider.
func (provider *CohereProvider) ImageEdit(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayImageEditRequest) (*schemas.GatewayImageGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditRequest, provider.GetProviderKey())
}

// ImageEditStream is not supported by the Cohere provider.
func (provider *CohereProvider) ImageEditStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayImageEditRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditStreamRequest, provider.GetProviderKey())
}

// ImageVariation is not supported by the Cohere provider.
func (provider *CohereProvider) ImageVariation(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayImageVariationRequest) (*schemas.GatewayImageGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageVariationRequest, provider.GetProviderKey())
}

// VideoGeneration is not supported by the Cohere provider.
func (provider *CohereProvider) VideoGeneration(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoGenerationRequest) (*schemas.GatewayVideoGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoGenerationRequest, provider.GetProviderKey())
}

// VideoRetrieve is not supported by the Cohere provider.
func (provider *CohereProvider) VideoRetrieve(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoRetrieveRequest) (*schemas.GatewayVideoGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRetrieveRequest, provider.GetProviderKey())
}

// VideoDownload is not supported by the Cohere provider.
func (provider *CohereProvider) VideoDownload(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoDownloadRequest) (*schemas.GatewayVideoDownloadResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDownloadRequest, provider.GetProviderKey())
}

// VideoDelete is not supported by Cohere provider.
func (provider *CohereProvider) VideoDelete(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoDeleteRequest) (*schemas.GatewayVideoDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDeleteRequest, provider.GetProviderKey())
}

// VideoList is not supported by Cohere provider.
func (provider *CohereProvider) VideoList(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoListRequest) (*schemas.GatewayVideoListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoListRequest, provider.GetProviderKey())
}

// VideoRemix is not supported by Cohere provider.
func (provider *CohereProvider) VideoRemix(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoRemixRequest) (*schemas.GatewayVideoGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRemixRequest, provider.GetProviderKey())
}

// BatchCreate is not supported by Cohere provider.
func (provider *CohereProvider) BatchCreate(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayBatchCreateRequest) (*schemas.GatewayBatchCreateResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCreateRequest, provider.GetProviderKey())
}

// BatchList is not supported by Cohere provider.
func (provider *CohereProvider) BatchList(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchListRequest) (*schemas.GatewayBatchListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchListRequest, provider.GetProviderKey())
}

// BatchRetrieve is not supported by Cohere provider.
func (provider *CohereProvider) BatchRetrieve(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchRetrieveRequest) (*schemas.GatewayBatchRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchRetrieveRequest, provider.GetProviderKey())
}

// BatchCancel is not supported by Cohere provider.
func (provider *CohereProvider) BatchCancel(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchCancelRequest) (*schemas.GatewayBatchCancelResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCancelRequest, provider.GetProviderKey())
}

// BatchDelete is not supported by Cohere provider.
func (provider *CohereProvider) BatchDelete(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchDeleteRequest) (*schemas.GatewayBatchDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchDeleteRequest, provider.GetProviderKey())
}

// BatchResults is not supported by Cohere provider.
func (provider *CohereProvider) BatchResults(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchResultsRequest) (*schemas.GatewayBatchResultsResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchResultsRequest, provider.GetProviderKey())
}

// FileUpload is not supported by Cohere provider.
func (provider *CohereProvider) FileUpload(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayFileUploadRequest) (*schemas.GatewayFileUploadResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileUploadRequest, provider.GetProviderKey())
}

// FileList is not supported by Cohere provider.
func (provider *CohereProvider) FileList(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayFileListRequest) (*schemas.GatewayFileListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileListRequest, provider.GetProviderKey())
}

// FileRetrieve is not supported by Cohere provider.
func (provider *CohereProvider) FileRetrieve(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayFileRetrieveRequest) (*schemas.GatewayFileRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileRetrieveRequest, provider.GetProviderKey())
}

// FileDelete is not supported by Cohere provider.
func (provider *CohereProvider) FileDelete(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayFileDeleteRequest) (*schemas.GatewayFileDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileDeleteRequest, provider.GetProviderKey())
}

// FileContent is not supported by Cohere provider.
func (provider *CohereProvider) FileContent(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayFileContentRequest) (*schemas.GatewayFileContentResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileContentRequest, provider.GetProviderKey())
}

// CountTokens performs a token counting request via Cohere's /v1/tokenize API.
func (provider *CohereProvider) CountTokens(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayResponsesRequest) (*schemas.GatewayCountTokensResponse, *schemas.GatewayError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Cohere, provider.customProviderConfig, schemas.CountTokensRequest); err != nil {
		return nil, err
	}

	jsonBody, gatewayErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToCohereCountTokensRequest(request)
		})
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	responseBody, latency, providerResponseHeaders, gatewayErr := provider.completeRequest(
		ctx,
		jsonBody,
		provider.buildRequestURL(ctx, "/v1/tokenize", schemas.CountTokensRequest),
		key.Value.GetValue(),
	)
	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if gatewayErr != nil {
		return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Large response mode: return lightweight response with metadata only
	if isLargeResp, _ := ctx.Value(schemas.GatewayContextKeyLargeResponseMode).(bool); isLargeResp {
		return &schemas.GatewayCountTokensResponse{
			Model: request.Model,
			ExtraFields: schemas.GatewayResponseExtraFields{
				Latency:                 latency.Milliseconds(),
				ProviderResponseHeaders: providerResponseHeaders,
			},
		}, nil
	}

	cohereResponse := &CohereCountTokensResponse{}

	rawRequest, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(
		responseBody,
		cohereResponse,
		jsonBody,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
	)
	if gatewayErr != nil {
		return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	gatewayResponse := cohereResponse.ToGatewayCountTokensResponse(request.Model)
	if gatewayResponse == nil {
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewGatewayOperationError(schemas.ErrProviderResponseDecode, fmt.Errorf("nil cohere count tokens response")), jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	gatewayResponse.ExtraFields.Latency = latency.Milliseconds()
	gatewayResponse.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		gatewayResponse.ExtraFields.RawRequest = rawRequest
	}

	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		gatewayResponse.ExtraFields.RawResponse = rawResponse
	}

	return gatewayResponse, nil
}

// Compaction is not supported by the Cohere provider.
func (provider *CohereProvider) Compaction(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayCompactionRequest) (*schemas.GatewayCompactionResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CompactionRequest, provider.GetProviderKey())
}

// ContainerCreate is not supported by the Cohere provider.
func (provider *CohereProvider) ContainerCreate(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayContainerCreateRequest) (*schemas.GatewayContainerCreateResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerCreateRequest, provider.GetProviderKey())
}

// ContainerList is not supported by the Cohere provider.
func (provider *CohereProvider) ContainerList(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerListRequest) (*schemas.GatewayContainerListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerListRequest, provider.GetProviderKey())
}

// ContainerRetrieve is not supported by the Cohere provider.
func (provider *CohereProvider) ContainerRetrieve(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerRetrieveRequest) (*schemas.GatewayContainerRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerRetrieveRequest, provider.GetProviderKey())
}

// ContainerDelete is not supported by the Cohere provider.
func (provider *CohereProvider) ContainerDelete(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerDeleteRequest) (*schemas.GatewayContainerDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerDeleteRequest, provider.GetProviderKey())
}

// ContainerFileCreate is not supported by the Cohere provider.
func (provider *CohereProvider) ContainerFileCreate(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayContainerFileCreateRequest) (*schemas.GatewayContainerFileCreateResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileCreateRequest, provider.GetProviderKey())
}

// ContainerFileList is not supported by the Cohere provider.
func (provider *CohereProvider) ContainerFileList(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileListRequest) (*schemas.GatewayContainerFileListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileListRequest, provider.GetProviderKey())
}

// ContainerFileRetrieve is not supported by the Cohere provider.
func (provider *CohereProvider) ContainerFileRetrieve(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileRetrieveRequest) (*schemas.GatewayContainerFileRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileRetrieveRequest, provider.GetProviderKey())
}

// ContainerFileContent is not supported by the Cohere provider.
func (provider *CohereProvider) ContainerFileContent(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileContentRequest) (*schemas.GatewayContainerFileContentResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileContentRequest, provider.GetProviderKey())
}

// ContainerFileDelete is not supported by the Cohere provider.
func (provider *CohereProvider) ContainerFileDelete(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileDeleteRequest) (*schemas.GatewayContainerFileDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileDeleteRequest, provider.GetProviderKey())
}

// Passthrough is not supported by the Cohere provider.
func (provider *CohereProvider) Passthrough(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayPassthroughRequest) (*schemas.GatewayPassthroughResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughRequest, provider.GetProviderKey())
}

func (provider *CohereProvider) PassthroughStream(_ *schemas.GatewayContext, _ schemas.PostHookRunner, _ func(context.Context), _ schemas.Key, _ *schemas.GatewayPassthroughRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughStreamRequest, provider.GetProviderKey())
}
