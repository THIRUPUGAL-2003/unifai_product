// Package providers implements various LLM providers and their utility functions.
// This file contains the Runway provider implementation.
package runway

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	providerUtils "github.com/gateway/gateway/core/providers/utils"
	schemas "github.com/gateway/gateway/core/schemas"
	"github.com/valyala/fasthttp"
)

// RunwayProvider implements the Provider interface for Runway's API.
type RunwayProvider struct {
	logger              schemas.Logger        // Logger for provider operations
	client              *fasthttp.Client      // HTTP client for API requests
	networkConfig       schemas.NetworkConfig // Network configuration including extra headers
	sendBackRawRequest  bool                  // Whether to include raw request in GatewayResponse
	sendBackRawResponse bool                  // Whether to include raw response in GatewayResponse
}

// NewRunwayProvider creates a new Runway provider instance.
// It initializes the HTTP client with the provided configuration and sets up response pools.
// The client is configured with timeouts, concurrency limits, and optional proxy settings.
func NewRunwayProvider(config *schemas.ProviderConfig, logger schemas.Logger) (*RunwayProvider, error) {
	config.CheckAndSetDefaults()

	requestTimeout := time.Second * time.Duration(config.NetworkConfig.DefaultRequestTimeoutInSeconds)
	client := &fasthttp.Client{
		ReadTimeout:         requestTimeout,
		WriteTimeout:        requestTimeout,
		MaxConnsPerHost:     config.NetworkConfig.MaxConnsPerHost,
		MaxIdleConnDuration: 60 * time.Second, // Video provider — longer idle duration to accommodate slower video generation responses
		MaxConnWaitTimeout:  requestTimeout,
		MaxConnDuration:     time.Second * time.Duration(schemas.DefaultMaxConnDurationInSeconds),
		ConnPoolStrategy:    fasthttp.FIFO,
	}

	// Configure proxy if provided
	client = providerUtils.ConfigureProxy(client, config.ProxyConfig, logger)
	client = providerUtils.ConfigureDialer(client, config.NetworkConfig.AllowPrivateNetwork)
	client = providerUtils.ConfigureTLS(client, config.NetworkConfig, logger)

	// Set default BaseURL if not provided
	if config.NetworkConfig.BaseURL == "" {
		config.NetworkConfig.BaseURL = "https://api.dev.runwayml.com"
	}
	config.NetworkConfig.BaseURL = strings.TrimRight(config.NetworkConfig.BaseURL, "/")

	return &RunwayProvider{
		logger:              logger,
		client:              client,
		networkConfig:       config.NetworkConfig,
		sendBackRawRequest:  config.SendBackRawRequest,
		sendBackRawResponse: config.SendBackRawResponse,
	}, nil
}

// GetProviderKey returns the provider identifier for Runway.
func (provider *RunwayProvider) GetProviderKey() schemas.ModelProvider {
	return schemas.Runway
}

// ListModels is not supported by the Runway provider.
func (provider *RunwayProvider) ListModels(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayListModelsRequest) (*schemas.GatewayListModelsResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ListModelsRequest, provider.GetProviderKey())
}

// TextCompletion is not supported by the Runway provider.
func (provider *RunwayProvider) TextCompletion(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayTextCompletionRequest) (*schemas.GatewayTextCompletionResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionRequest, provider.GetProviderKey())
}

// TextCompletionStream is not supported by the Runway provider.
func (provider *RunwayProvider) TextCompletionStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayTextCompletionRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionStreamRequest, provider.GetProviderKey())
}

// ChatCompletion is not supported by the Runway provider.
func (provider *RunwayProvider) ChatCompletion(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayChatRequest) (*schemas.GatewayChatResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ChatCompletionRequest, provider.GetProviderKey())
}

// ChatCompletionStream is not supported by the Runway provider.
func (provider *RunwayProvider) ChatCompletionStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayChatRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ChatCompletionStreamRequest, provider.GetProviderKey())
}

// Responses is not supported by the Runway provider.
func (provider *RunwayProvider) Responses(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayResponsesRequest) (*schemas.GatewayResponsesResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ResponsesRequest, provider.GetProviderKey())
}

// ResponsesStream is not supported by the Runway provider.
func (provider *RunwayProvider) ResponsesStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayResponsesRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ResponsesStreamRequest, provider.GetProviderKey())
}

// Embedding is not supported by the Runway provider.
func (provider *RunwayProvider) Embedding(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayEmbeddingRequest) (*schemas.GatewayEmbeddingResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.EmbeddingRequest, provider.GetProviderKey())
}

// Speech is not supported by the Runway provider.
func (provider *RunwayProvider) Speech(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewaySpeechRequest) (*schemas.GatewaySpeechResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechRequest, provider.GetProviderKey())
}

// SpeechStream is not supported by the Runway provider.
func (provider *RunwayProvider) SpeechStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewaySpeechRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechStreamRequest, provider.GetProviderKey())
}

// Transcription is not supported by the Runway provider.
func (provider *RunwayProvider) Transcription(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayTranscriptionRequest) (*schemas.GatewayTranscriptionResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionRequest, provider.GetProviderKey())
}

// TranscriptionStream is not supported by the Runway provider.
func (provider *RunwayProvider) TranscriptionStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayTranscriptionRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionStreamRequest, provider.GetProviderKey())
}

// Rerank is not supported by the Runway provider.
func (provider *RunwayProvider) Rerank(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayRerankRequest) (*schemas.GatewayRerankResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.RerankRequest, provider.GetProviderKey())
}

// OCR is not supported by the Runway provider.
func (provider *RunwayProvider) OCR(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayOCRRequest) (*schemas.GatewayOCRResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.OCRRequest, provider.GetProviderKey())
}

// ImageGeneration performs a text-to-image generation request to Runway's API.
// Runway image generation is task-based: a task is created and then polled until it
// reaches a terminal state, after which the generated image URLs are returned.
func (provider *RunwayProvider) ImageGeneration(ctx *schemas.GatewayContext, key schemas.Key, gatewayReq *schemas.GatewayImageGenerationRequest) (*schemas.GatewayImageGenerationResponse, *schemas.GatewayError) {
	// Convert Gateway request to Runway format
	jsonData, gatewayErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		gatewayReq,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToRunwayImageGenerationRequest(gatewayReq)
		})
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	return provider.HandleRunwayImageTask(ctx, key, gatewayReq.Model, jsonData)
}

// HandleRunwayImageTask posts a prebuilt text_to_image body, polls the task to completion,
// and builds the Gateway image response. Shared by ImageGeneration and ImageEdit since both
// use the same /v1/text_to_image endpoint and response shape.
func (provider *RunwayProvider) HandleRunwayImageTask(ctx *schemas.GatewayContext, key schemas.Key, model string, jsonData []byte) (*schemas.GatewayImageGenerationResponse, *schemas.GatewayError) {
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)
	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)

	// Create HTTP request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	req.SetRequestURI(provider.networkConfig.BaseURL + providerUtils.GetPathFromContext(ctx, "/v1/text_to_image"))
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	req.Header.Set("X-Runway-Version", "2024-11-06")
	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	req.SetBody(jsonData)

	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.EnrichError(ctx, parseRunwayError(resp), jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Decode response body
	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		rawErrBody := append([]byte(nil), resp.Body()...)
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewGatewayOperationError(schemas.ErrProviderResponseDecode, err), jsonData, rawErrBody, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Parse task creation response
	var taskResp RunwayTaskCreationResponse
	rawRequest, _, gatewayErr := providerUtils.HandleProviderResponse(body, &taskResp, jsonData, sendBackRawRequest, sendBackRawResponse)
	if gatewayErr != nil {
		return nil, providerUtils.SetErrorLatency(gatewayErr, latency)
	}

	// Poll the task until it reaches a terminal state
	taskDetails, rawResponse, gatewayErr := provider.pollRunwayTask(ctx, key, taskResp.ID, sendBackRawResponse)
	if gatewayErr != nil {
		return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Convert to Gateway response
	gatewayResp, gatewayErr := ToGatewayImageGenerationResponse(taskDetails)
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	gatewayResp.Model = model
	gatewayResp.ExtraFields.Latency = latency.Milliseconds()

	if sendBackRawRequest {
		gatewayResp.ExtraFields.RawRequest = rawRequest
	}
	if sendBackRawResponse {
		gatewayResp.ExtraFields.RawResponse = rawResponse
	}

	return gatewayResp, nil
}

// runwayImagePollingInterval is the interval between Runway task status polls for image generation.
const runwayImagePollingInterval = 2 * time.Second

// pollRunwayTask polls a Runway task until it reaches a terminal state or the context times out.
func (provider *RunwayProvider) pollRunwayTask(ctx *schemas.GatewayContext, key schemas.Key, taskID string, sendBackRawResponse bool) (*RunwayTaskDetailsResponse, interface{}, *schemas.GatewayError) {
	pollCtx, cancel := schemas.NewGatewayContextWithTimeout(ctx, time.Duration(provider.networkConfig.DefaultRequestTimeoutInSeconds)*time.Second)
	defer cancel()

	ticker := time.NewTicker(runwayImagePollingInterval)
	defer ticker.Stop()

	for {
		taskDetails, rawResponse, gatewayErr := provider.retrieveRunwayTask(pollCtx, key, taskID, sendBackRawResponse)
		if gatewayErr != nil {
			return nil, nil, gatewayErr
		}

		switch taskDetails.Status {
		case RunwayTaskStatusSucceeded:
			return taskDetails, rawResponse, nil
		case RunwayTaskStatusFailed, RunwayTaskStatusCancelled:
			return nil, nil, providerUtils.NewGatewayOperationError(fmt.Sprintf("runway task %s", taskDetails.Status), nil)
		}

		select {
		case <-pollCtx.Done():
			return nil, nil, providerUtils.NewGatewayOperationError(schemas.ErrProviderRequestTimedOut, fmt.Errorf("runway task polling timed out"))
		case <-ticker.C:
		}
	}
}

// retrieveRunwayTask fetches the current state of a Runway task.
func (provider *RunwayProvider) retrieveRunwayTask(ctx *schemas.GatewayContext, key schemas.Key, taskID string, sendBackRawResponse bool) (*RunwayTaskDetailsResponse, interface{}, *schemas.GatewayError) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	req.SetRequestURI(provider.networkConfig.BaseURL + providerUtils.GetPathFromContext(ctx, "/v1/tasks/"+taskID))
	req.Header.SetMethod(http.MethodGet)
	req.Header.Set("X-Runway-Version", "2024-11-06")
	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, nil, gatewayErr
	}

	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, nil, providerUtils.SetErrorLatency(parseRunwayError(resp), latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, nil, providerUtils.SetErrorLatency(providerUtils.NewGatewayOperationError(schemas.ErrProviderResponseDecode, err), latency)
	}

	var taskDetails RunwayTaskDetailsResponse
	_, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(body, &taskDetails, nil, false, sendBackRawResponse)
	if gatewayErr != nil {
		return nil, nil, providerUtils.SetErrorLatency(gatewayErr, latency)
	}

	return &taskDetails, rawResponse, nil
}

// ImageGenerationStream is not supported by the Runway provider.
func (provider *RunwayProvider) ImageGenerationStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayImageGenerationRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationStreamRequest, provider.GetProviderKey())
}

// ImageEdit performs an image edit request to Runway's API. Runway has no dedicated edit
// endpoint, so the input images are passed as reference images to /v1/text_to_image.
func (provider *RunwayProvider) ImageEdit(ctx *schemas.GatewayContext, key schemas.Key, gatewayReq *schemas.GatewayImageEditRequest) (*schemas.GatewayImageGenerationResponse, *schemas.GatewayError) {
	// Convert Gateway request to Runway format
	jsonData, gatewayErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		gatewayReq,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToRunwayImageEditRequest(gatewayReq)
		})
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	return provider.HandleRunwayImageTask(ctx, key, gatewayReq.Model, jsonData)
}

// ImageEditStream is not supported by the Runway provider.
func (provider *RunwayProvider) ImageEditStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayImageEditRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditStreamRequest, provider.GetProviderKey())
}

// ImageVariation is not supported by the Runway provider.
func (provider *RunwayProvider) ImageVariation(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayImageVariationRequest) (*schemas.GatewayImageGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageVariationRequest, provider.GetProviderKey())
}

// VideoGeneration performs a video generation request to Runway's API.
func (provider *RunwayProvider) VideoGeneration(ctx *schemas.GatewayContext, key schemas.Key, gatewayReq *schemas.GatewayVideoGenerationRequest) (*schemas.GatewayVideoGenerationResponse, *schemas.GatewayError) {
	providerName := provider.GetProviderKey()
	model := gatewayReq.Model

	// Convert Gateway request to Runway format
	jsonData, gatewayErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		gatewayReq,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToRunwayVideoGenerationRequest(gatewayReq)
		})
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	// Determine the endpoint based on request type
	endpoint := getRunwayEndpoint(gatewayReq)

	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)
	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)

	// Create HTTP request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	// Set request URI and headers
	req.SetRequestURI(provider.networkConfig.BaseURL + providerUtils.GetPathFromContext(ctx, endpoint))
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	req.Header.Set("X-Runway-Version", "2024-11-06")
	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	req.SetBody(jsonData)

	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.EnrichError(ctx, parseRunwayError(resp), jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Decode response body
	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		rawErrBody := append([]byte(nil), resp.Body()...)
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewGatewayOperationError(schemas.ErrProviderResponseDecode, err), jsonData, rawErrBody, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Parse response
	var taskResp RunwayTaskCreationResponse
	rawRequest, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(body, &taskResp, jsonData, sendBackRawRequest, sendBackRawResponse)
	if gatewayErr != nil {
		return nil, providerUtils.SetErrorLatency(gatewayErr, latency)
	}

	// Convert to Gateway response
	gatewayResp := &schemas.GatewayVideoGenerationResponse{
		ID:     providerUtils.AddVideoIDProviderSuffix(taskResp.ID, providerName),
		Model:  model,
		Object: "video",
		Status: schemas.VideoStatusQueued,
		ExtraFields: schemas.GatewayResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}

	if sendBackRawRequest {
		gatewayResp.ExtraFields.RawRequest = rawRequest
	}
	if sendBackRawResponse {
		gatewayResp.ExtraFields.RawResponse = rawResponse
	}

	return gatewayResp, nil
}

// VideoRetrieve retrieves the status of a video generation task from Runway's API.
func (provider *RunwayProvider) VideoRetrieve(ctx *schemas.GatewayContext, key schemas.Key, gatewayReq *schemas.GatewayVideoRetrieveRequest) (*schemas.GatewayVideoGenerationResponse, *schemas.GatewayError) {
	providerName := provider.GetProviderKey()
	taskID := providerUtils.StripVideoIDProviderSuffix(gatewayReq.ID, providerName)

	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)
	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)

	// Create HTTP request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	// Set request URI and headers
	req.SetRequestURI(provider.networkConfig.BaseURL + providerUtils.GetPathFromContext(ctx, "/v1/tasks/"+taskID))
	req.Header.SetMethod("GET")
	req.Header.Set("X-Runway-Version", "2024-11-06")
	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.EnrichError(ctx, parseRunwayError(resp), nil, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Decode response body
	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		rawErrBody := append([]byte(nil), resp.Body()...)
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewGatewayOperationError(schemas.ErrProviderResponseDecode, err), nil, rawErrBody, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Parse response
	var taskDetails RunwayTaskDetailsResponse
	rawRequest, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(body, &taskDetails, nil, sendBackRawRequest, sendBackRawResponse)
	if gatewayErr != nil {
		return nil, providerUtils.SetErrorLatency(gatewayErr, latency)
	}

	// Convert to Gateway response
	gatewayResp, gatewayErr := ToGatewayVideoGenerationResponse(&taskDetails)
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	gatewayResp.ID = providerUtils.AddVideoIDProviderSuffix(gatewayResp.ID, providerName)
	gatewayResp.ExtraFields.Latency = latency.Milliseconds()

	if sendBackRawRequest {
		gatewayResp.ExtraFields.RawRequest = rawRequest
	}
	if sendBackRawResponse {
		gatewayResp.ExtraFields.RawResponse = rawResponse
	}

	return gatewayResp, nil
}

// VideoDownload retrieves a video from Runway's API.
func (provider *RunwayProvider) VideoDownload(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayVideoDownloadRequest) (*schemas.GatewayVideoDownloadResponse, *schemas.GatewayError) {
	// Retrieve task status to get the video URL
	gatewayVideoRetrieveRequest := &schemas.GatewayVideoRetrieveRequest{
		Provider: request.Provider,
		ID:       request.ID,
	}
	taskDetails, gatewayErr := provider.VideoRetrieve(ctx, key, gatewayVideoRetrieveRequest)
	if gatewayErr != nil {
		return nil, gatewayErr
	}
	// Check if video is ready
	if taskDetails.Status != schemas.VideoStatusCompleted {
		return nil, providerUtils.NewGatewayOperationError(
			fmt.Sprintf("video not ready, current status: %s", taskDetails.Status),
			nil)
	}
	if len(taskDetails.Videos) == 0 {
		return nil, providerUtils.NewGatewayOperationError("video URL not available", nil)
	}
	var videoUrl string
	if taskDetails.Videos[0].URL != nil {
		videoUrl = *taskDetails.Videos[0].URL
	}
	if videoUrl == "" {
		return nil, providerUtils.NewGatewayOperationError("invalid video output type", nil)
	}
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)
	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)

	// Download video from Runway's URL
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)
	req.SetRequestURI(videoUrl)
	req.Header.SetMethod("GET")
	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, gatewayErr
	}
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.SetErrorLatency(providerUtils.NewGatewayOperationError(
			fmt.Sprintf("failed to download video: HTTP %d", resp.StatusCode()),
			nil), latency)
	}
	// Get content and content type
	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		rawErrBody := append([]byte(nil), resp.Body()...)
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewGatewayOperationError(schemas.ErrProviderResponseDecode, err), nil, rawErrBody, sendBackRawRequest, sendBackRawResponse, latency)
	}
	contentType := string(resp.Header.ContentType())
	if contentType == "" {
		contentType = "video/mp4" // Default for Runway
	}
	// Copy the binary content
	content := append([]byte(nil), body...)
	gatewayResp := &schemas.GatewayVideoDownloadResponse{
		VideoID:     request.ID,
		Content:     content,
		ContentType: contentType,
	}

	gatewayResp.ExtraFields.Latency = latency.Milliseconds()

	return gatewayResp, nil
}

// VideoDelete cancels or deletes a task in Runway.
// Tasks that are running, pending, or throttled can be canceled by invoking this method.
// Invoking this method for other tasks will delete them.
func (provider *RunwayProvider) VideoDelete(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayVideoDeleteRequest) (*schemas.GatewayVideoDeleteResponse, *schemas.GatewayError) {
	providerName := provider.GetProviderKey()

	if request.ID == "" {
		return nil, providerUtils.NewGatewayOperationError("task_id is required", nil)
	}

	taskID := providerUtils.StripVideoIDProviderSuffix(request.ID, providerName)

	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)
	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)

	// Create request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	req.SetRequestURI(provider.networkConfig.BaseURL + providerUtils.GetPathFromContext(ctx, "/v1/tasks/"+taskID))
	req.Header.SetMethod(http.MethodDelete)
	req.Header.Set("X-Runway-Version", "2024-11-06")
	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	// Make request
	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	// Handle error response - Runway returns 204 No Content on success
	if resp.StatusCode() != fasthttp.StatusNoContent {
		return nil, providerUtils.EnrichError(ctx, parseRunwayError(resp), nil, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Build response - Runway returns empty body on 204
	response := &schemas.GatewayVideoDeleteResponse{
		ID:      request.ID, // Return with provider prefix
		Object:  "video.deleted",
		Deleted: true,
	}

	response.ExtraFields.Latency = latency.Milliseconds()

	return response, nil
}

// VideoList is not supported by Runway provider.
func (provider *RunwayProvider) VideoList(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoListRequest) (*schemas.GatewayVideoListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoListRequest, provider.GetProviderKey())
}

// VideoRemix is not supported by Runway provider.
func (provider *RunwayProvider) VideoRemix(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoRemixRequest) (*schemas.GatewayVideoGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRemixRequest, provider.GetProviderKey())
}

// FileUpload is not supported by Runway provider.
func (provider *RunwayProvider) FileUpload(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayFileUploadRequest) (*schemas.GatewayFileUploadResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileUploadRequest, provider.GetProviderKey())
}

// FileList is not supported by Runway provider.
func (provider *RunwayProvider) FileList(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayFileListRequest) (*schemas.GatewayFileListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileListRequest, provider.GetProviderKey())
}

// FileRetrieve is not supported by Runway provider.
func (provider *RunwayProvider) FileRetrieve(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayFileRetrieveRequest) (*schemas.GatewayFileRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileRetrieveRequest, provider.GetProviderKey())
}

// FileDelete is not supported by Runway provider.
func (provider *RunwayProvider) FileDelete(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayFileDeleteRequest) (*schemas.GatewayFileDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileDeleteRequest, provider.GetProviderKey())
}

// FileContent is not supported by Runway provider.
func (provider *RunwayProvider) FileContent(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayFileContentRequest) (*schemas.GatewayFileContentResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileContentRequest, provider.GetProviderKey())
}

// BatchCreate is not supported by Runway provider.
func (provider *RunwayProvider) BatchCreate(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayBatchCreateRequest) (*schemas.GatewayBatchCreateResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCreateRequest, provider.GetProviderKey())
}

// BatchList is not supported by Runway provider.
func (provider *RunwayProvider) BatchList(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchListRequest) (*schemas.GatewayBatchListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchListRequest, provider.GetProviderKey())
}

// BatchRetrieve is not supported by Runway provider.
func (provider *RunwayProvider) BatchRetrieve(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchRetrieveRequest) (*schemas.GatewayBatchRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchRetrieveRequest, provider.GetProviderKey())
}

// BatchCancel is not supported by Runway provider.
func (provider *RunwayProvider) BatchCancel(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchCancelRequest) (*schemas.GatewayBatchCancelResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCancelRequest, provider.GetProviderKey())
}

// BatchDelete is not supported by Runway provider.
func (provider *RunwayProvider) BatchDelete(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchDeleteRequest) (*schemas.GatewayBatchDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchDeleteRequest, provider.GetProviderKey())
}

// BatchResults is not supported by Runway provider.
func (provider *RunwayProvider) BatchResults(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchResultsRequest) (*schemas.GatewayBatchResultsResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchResultsRequest, provider.GetProviderKey())
}

// CountTokens is not supported by the Runway provider.
func (provider *RunwayProvider) CountTokens(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayResponsesRequest) (*schemas.GatewayCountTokensResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CountTokensRequest, provider.GetProviderKey())
}

// Compaction is not supported by the Runway provider.
func (provider *RunwayProvider) Compaction(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayCompactionRequest) (*schemas.GatewayCompactionResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CompactionRequest, provider.GetProviderKey())
}

// ContainerCreate is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerCreate(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayContainerCreateRequest) (*schemas.GatewayContainerCreateResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerCreateRequest, provider.GetProviderKey())
}

// ContainerList is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerList(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerListRequest) (*schemas.GatewayContainerListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerListRequest, provider.GetProviderKey())
}

// ContainerRetrieve is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerRetrieve(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerRetrieveRequest) (*schemas.GatewayContainerRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerRetrieveRequest, provider.GetProviderKey())
}

// ContainerDelete is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerDelete(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerDeleteRequest) (*schemas.GatewayContainerDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerDeleteRequest, provider.GetProviderKey())
}

// ContainerFileCreate is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerFileCreate(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayContainerFileCreateRequest) (*schemas.GatewayContainerFileCreateResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileCreateRequest, provider.GetProviderKey())
}

// ContainerFileList is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerFileList(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileListRequest) (*schemas.GatewayContainerFileListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileListRequest, provider.GetProviderKey())
}

// ContainerFileRetrieve is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerFileRetrieve(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileRetrieveRequest) (*schemas.GatewayContainerFileRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileRetrieveRequest, provider.GetProviderKey())
}

// ContainerFileContent is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerFileContent(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileContentRequest) (*schemas.GatewayContainerFileContentResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileContentRequest, provider.GetProviderKey())
}

// ContainerFileDelete is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerFileDelete(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileDeleteRequest) (*schemas.GatewayContainerFileDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileDeleteRequest, provider.GetProviderKey())
}

// Passthrough is not supported by the Runway provider.
func (provider *RunwayProvider) Passthrough(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayPassthroughRequest) (*schemas.GatewayPassthroughResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughRequest, provider.GetProviderKey())
}

func (provider *RunwayProvider) PassthroughStream(_ *schemas.GatewayContext, _ schemas.PostHookRunner, _ func(context.Context), _ schemas.Key, _ *schemas.GatewayPassthroughRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughStreamRequest, provider.GetProviderKey())
}
