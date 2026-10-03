// Package providers implements various LLM providers and their utility functions.
// This file contains the Runway provider implementation.
package runway

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	providerUtils "github.com/raksha/raksha/core/providers/utils"
	schemas "github.com/raksha/raksha/core/schemas"
	"github.com/valyala/fasthttp"
)

// RunwayProvider implements the Provider interface for Runway's API.
type RunwayProvider struct {
	logger              schemas.Logger        // Logger for provider operations
	client              *fasthttp.Client      // HTTP client for API requests
	networkConfig       schemas.NetworkConfig // Network configuration including extra headers
	sendBackRawRequest  bool                  // Whether to include raw request in RakshaResponse
	sendBackRawResponse bool                  // Whether to include raw response in RakshaResponse
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
func (provider *RunwayProvider) ListModels(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaListModelsRequest) (*schemas.RakshaListModelsResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ListModelsRequest, provider.GetProviderKey())
}

// TextCompletion is not supported by the Runway provider.
func (provider *RunwayProvider) TextCompletion(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaTextCompletionRequest) (*schemas.RakshaTextCompletionResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionRequest, provider.GetProviderKey())
}

// TextCompletionStream is not supported by the Runway provider.
func (provider *RunwayProvider) TextCompletionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaTextCompletionRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionStreamRequest, provider.GetProviderKey())
}

// ChatCompletion is not supported by the Runway provider.
func (provider *RunwayProvider) ChatCompletion(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaChatRequest) (*schemas.RakshaChatResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ChatCompletionRequest, provider.GetProviderKey())
}

// ChatCompletionStream is not supported by the Runway provider.
func (provider *RunwayProvider) ChatCompletionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaChatRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ChatCompletionStreamRequest, provider.GetProviderKey())
}

// Responses is not supported by the Runway provider.
func (provider *RunwayProvider) Responses(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaResponsesRequest) (*schemas.RakshaResponsesResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ResponsesRequest, provider.GetProviderKey())
}

// ResponsesStream is not supported by the Runway provider.
func (provider *RunwayProvider) ResponsesStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaResponsesRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ResponsesStreamRequest, provider.GetProviderKey())
}

// Embedding is not supported by the Runway provider.
func (provider *RunwayProvider) Embedding(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaEmbeddingRequest) (*schemas.RakshaEmbeddingResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.EmbeddingRequest, provider.GetProviderKey())
}

// Speech is not supported by the Runway provider.
func (provider *RunwayProvider) Speech(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaSpeechRequest) (*schemas.RakshaSpeechResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechRequest, provider.GetProviderKey())
}

// SpeechStream is not supported by the Runway provider.
func (provider *RunwayProvider) SpeechStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaSpeechRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechStreamRequest, provider.GetProviderKey())
}

// Transcription is not supported by the Runway provider.
func (provider *RunwayProvider) Transcription(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaTranscriptionRequest) (*schemas.RakshaTranscriptionResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionRequest, provider.GetProviderKey())
}

// TranscriptionStream is not supported by the Runway provider.
func (provider *RunwayProvider) TranscriptionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaTranscriptionRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionStreamRequest, provider.GetProviderKey())
}

// Rerank is not supported by the Runway provider.
func (provider *RunwayProvider) Rerank(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaRerankRequest) (*schemas.RakshaRerankResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.RerankRequest, provider.GetProviderKey())
}

// OCR is not supported by the Runway provider.
func (provider *RunwayProvider) OCR(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaOCRRequest) (*schemas.RakshaOCRResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.OCRRequest, provider.GetProviderKey())
}

// ImageGeneration performs a text-to-image generation request to Runway's API.
// Runway image generation is task-based: a task is created and then polled until it
// reaches a terminal state, after which the generated image URLs are returned.
func (provider *RunwayProvider) ImageGeneration(ctx *schemas.RakshaContext, key schemas.Key, rakshaReq *schemas.RakshaImageGenerationRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	// Convert Raksha request to Runway format
	jsonData, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		rakshaReq,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToRunwayImageGenerationRequest(rakshaReq)
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	return provider.HandleRunwayImageTask(ctx, key, rakshaReq.Model, jsonData)
}

// HandleRunwayImageTask posts a prebuilt text_to_image body, polls the task to completion,
// and builds the Raksha image response. Shared by ImageGeneration and ImageEdit since both
// use the same /v1/text_to_image endpoint and response shape.
func (provider *RunwayProvider) HandleRunwayImageTask(ctx *schemas.RakshaContext, key schemas.Key, model string, jsonData []byte) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
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

	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.EnrichError(ctx, parseRunwayError(resp), jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Decode response body
	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		rawErrBody := append([]byte(nil), resp.Body()...)
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err), jsonData, rawErrBody, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Parse task creation response
	var taskResp RunwayTaskCreationResponse
	rawRequest, _, rakshaErr := providerUtils.HandleProviderResponse(body, &taskResp, jsonData, sendBackRawRequest, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, providerUtils.SetErrorLatency(rakshaErr, latency)
	}

	// Poll the task until it reaches a terminal state
	taskDetails, rawResponse, rakshaErr := provider.pollRunwayTask(ctx, key, taskResp.ID, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Convert to Raksha response
	rakshaResp, rakshaErr := ToRakshaImageGenerationResponse(taskDetails)
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	rakshaResp.Model = model
	rakshaResp.ExtraFields.Latency = latency.Milliseconds()

	if sendBackRawRequest {
		rakshaResp.ExtraFields.RawRequest = rawRequest
	}
	if sendBackRawResponse {
		rakshaResp.ExtraFields.RawResponse = rawResponse
	}

	return rakshaResp, nil
}

// runwayImagePollingInterval is the interval between Runway task status polls for image generation.
const runwayImagePollingInterval = 2 * time.Second

// pollRunwayTask polls a Runway task until it reaches a terminal state or the context times out.
func (provider *RunwayProvider) pollRunwayTask(ctx *schemas.RakshaContext, key schemas.Key, taskID string, sendBackRawResponse bool) (*RunwayTaskDetailsResponse, interface{}, *schemas.RakshaError) {
	pollCtx, cancel := schemas.NewRakshaContextWithTimeout(ctx, time.Duration(provider.networkConfig.DefaultRequestTimeoutInSeconds)*time.Second)
	defer cancel()

	ticker := time.NewTicker(runwayImagePollingInterval)
	defer ticker.Stop()

	for {
		taskDetails, rawResponse, rakshaErr := provider.retrieveRunwayTask(pollCtx, key, taskID, sendBackRawResponse)
		if rakshaErr != nil {
			return nil, nil, rakshaErr
		}

		switch taskDetails.Status {
		case RunwayTaskStatusSucceeded:
			return taskDetails, rawResponse, nil
		case RunwayTaskStatusFailed, RunwayTaskStatusCancelled:
			return nil, nil, providerUtils.NewRakshaOperationError(fmt.Sprintf("runway task %s", taskDetails.Status), nil)
		}

		select {
		case <-pollCtx.Done():
			return nil, nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderRequestTimedOut, fmt.Errorf("runway task polling timed out"))
		case <-ticker.C:
		}
	}
}

// retrieveRunwayTask fetches the current state of a Runway task.
func (provider *RunwayProvider) retrieveRunwayTask(ctx *schemas.RakshaContext, key schemas.Key, taskID string, sendBackRawResponse bool) (*RunwayTaskDetailsResponse, interface{}, *schemas.RakshaError) {
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

	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, nil, rakshaErr
	}

	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, nil, providerUtils.SetErrorLatency(parseRunwayError(resp), latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, nil, providerUtils.SetErrorLatency(providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err), latency)
	}

	var taskDetails RunwayTaskDetailsResponse
	_, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(body, &taskDetails, nil, false, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, nil, providerUtils.SetErrorLatency(rakshaErr, latency)
	}

	return &taskDetails, rawResponse, nil
}

// ImageGenerationStream is not supported by the Runway provider.
func (provider *RunwayProvider) ImageGenerationStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaImageGenerationRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationStreamRequest, provider.GetProviderKey())
}

// ImageEdit performs an image edit request to Runway's API. Runway has no dedicated edit
// endpoint, so the input images are passed as reference images to /v1/text_to_image.
func (provider *RunwayProvider) ImageEdit(ctx *schemas.RakshaContext, key schemas.Key, rakshaReq *schemas.RakshaImageEditRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	// Convert Raksha request to Runway format
	jsonData, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		rakshaReq,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToRunwayImageEditRequest(rakshaReq)
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	return provider.HandleRunwayImageTask(ctx, key, rakshaReq.Model, jsonData)
}

// ImageEditStream is not supported by the Runway provider.
func (provider *RunwayProvider) ImageEditStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaImageEditRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditStreamRequest, provider.GetProviderKey())
}

// ImageVariation is not supported by the Runway provider.
func (provider *RunwayProvider) ImageVariation(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageVariationRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageVariationRequest, provider.GetProviderKey())
}

// VideoGeneration performs a video generation request to Runway's API.
func (provider *RunwayProvider) VideoGeneration(ctx *schemas.RakshaContext, key schemas.Key, rakshaReq *schemas.RakshaVideoGenerationRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	providerName := provider.GetProviderKey()
	model := rakshaReq.Model

	// Convert Raksha request to Runway format
	jsonData, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		rakshaReq,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToRunwayVideoGenerationRequest(rakshaReq)
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Determine the endpoint based on request type
	endpoint := getRunwayEndpoint(rakshaReq)

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

	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.EnrichError(ctx, parseRunwayError(resp), jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Decode response body
	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		rawErrBody := append([]byte(nil), resp.Body()...)
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err), jsonData, rawErrBody, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Parse response
	var taskResp RunwayTaskCreationResponse
	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(body, &taskResp, jsonData, sendBackRawRequest, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, providerUtils.SetErrorLatency(rakshaErr, latency)
	}

	// Convert to Raksha response
	rakshaResp := &schemas.RakshaVideoGenerationResponse{
		ID:     providerUtils.AddVideoIDProviderSuffix(taskResp.ID, providerName),
		Model:  model,
		Object: "video",
		Status: schemas.VideoStatusQueued,
		ExtraFields: schemas.RakshaResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}

	if sendBackRawRequest {
		rakshaResp.ExtraFields.RawRequest = rawRequest
	}
	if sendBackRawResponse {
		rakshaResp.ExtraFields.RawResponse = rawResponse
	}

	return rakshaResp, nil
}

// VideoRetrieve retrieves the status of a video generation task from Runway's API.
func (provider *RunwayProvider) VideoRetrieve(ctx *schemas.RakshaContext, key schemas.Key, rakshaReq *schemas.RakshaVideoRetrieveRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	providerName := provider.GetProviderKey()
	taskID := providerUtils.StripVideoIDProviderSuffix(rakshaReq.ID, providerName)

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

	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.EnrichError(ctx, parseRunwayError(resp), nil, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Decode response body
	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		rawErrBody := append([]byte(nil), resp.Body()...)
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err), nil, rawErrBody, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Parse response
	var taskDetails RunwayTaskDetailsResponse
	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(body, &taskDetails, nil, sendBackRawRequest, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, providerUtils.SetErrorLatency(rakshaErr, latency)
	}

	// Convert to Raksha response
	rakshaResp, rakshaErr := ToRakshaVideoGenerationResponse(&taskDetails)
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	rakshaResp.ID = providerUtils.AddVideoIDProviderSuffix(rakshaResp.ID, providerName)
	rakshaResp.ExtraFields.Latency = latency.Milliseconds()

	if sendBackRawRequest {
		rakshaResp.ExtraFields.RawRequest = rawRequest
	}
	if sendBackRawResponse {
		rakshaResp.ExtraFields.RawResponse = rawResponse
	}

	return rakshaResp, nil
}

// VideoDownload retrieves a video from Runway's API.
func (provider *RunwayProvider) VideoDownload(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaVideoDownloadRequest) (*schemas.RakshaVideoDownloadResponse, *schemas.RakshaError) {
	// Retrieve task status to get the video URL
	rakshaVideoRetrieveRequest := &schemas.RakshaVideoRetrieveRequest{
		Provider: request.Provider,
		ID:       request.ID,
	}
	taskDetails, rakshaErr := provider.VideoRetrieve(ctx, key, rakshaVideoRetrieveRequest)
	if rakshaErr != nil {
		return nil, rakshaErr
	}
	// Check if video is ready
	if taskDetails.Status != schemas.VideoStatusCompleted {
		return nil, providerUtils.NewRakshaOperationError(
			fmt.Sprintf("video not ready, current status: %s", taskDetails.Status),
			nil)
	}
	if len(taskDetails.Videos) == 0 {
		return nil, providerUtils.NewRakshaOperationError("video URL not available", nil)
	}
	var videoUrl string
	if taskDetails.Videos[0].URL != nil {
		videoUrl = *taskDetails.Videos[0].URL
	}
	if videoUrl == "" {
		return nil, providerUtils.NewRakshaOperationError("invalid video output type", nil)
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
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.SetErrorLatency(providerUtils.NewRakshaOperationError(
			fmt.Sprintf("failed to download video: HTTP %d", resp.StatusCode()),
			nil), latency)
	}
	// Get content and content type
	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		rawErrBody := append([]byte(nil), resp.Body()...)
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err), nil, rawErrBody, sendBackRawRequest, sendBackRawResponse, latency)
	}
	contentType := string(resp.Header.ContentType())
	if contentType == "" {
		contentType = "video/mp4" // Default for Runway
	}
	// Copy the binary content
	content := append([]byte(nil), body...)
	rakshaResp := &schemas.RakshaVideoDownloadResponse{
		VideoID:     request.ID,
		Content:     content,
		ContentType: contentType,
	}

	rakshaResp.ExtraFields.Latency = latency.Milliseconds()

	return rakshaResp, nil
}

// VideoDelete cancels or deletes a task in Runway.
// Tasks that are running, pending, or throttled can be canceled by invoking this method.
// Invoking this method for other tasks will delete them.
func (provider *RunwayProvider) VideoDelete(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaVideoDeleteRequest) (*schemas.RakshaVideoDeleteResponse, *schemas.RakshaError) {
	providerName := provider.GetProviderKey()

	if request.ID == "" {
		return nil, providerUtils.NewRakshaOperationError("task_id is required", nil)
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
	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Handle error response - Runway returns 204 No Content on success
	if resp.StatusCode() != fasthttp.StatusNoContent {
		return nil, providerUtils.EnrichError(ctx, parseRunwayError(resp), nil, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Build response - Runway returns empty body on 204
	response := &schemas.RakshaVideoDeleteResponse{
		ID:      request.ID, // Return with provider prefix
		Object:  "video.deleted",
		Deleted: true,
	}

	response.ExtraFields.Latency = latency.Milliseconds()

	return response, nil
}

// VideoList is not supported by Runway provider.
func (provider *RunwayProvider) VideoList(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoListRequest) (*schemas.RakshaVideoListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoListRequest, provider.GetProviderKey())
}

// VideoRemix is not supported by Runway provider.
func (provider *RunwayProvider) VideoRemix(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoRemixRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRemixRequest, provider.GetProviderKey())
}

// FileUpload is not supported by Runway provider.
func (provider *RunwayProvider) FileUpload(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaFileUploadRequest) (*schemas.RakshaFileUploadResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileUploadRequest, provider.GetProviderKey())
}

// FileList is not supported by Runway provider.
func (provider *RunwayProvider) FileList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileListRequest) (*schemas.RakshaFileListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileListRequest, provider.GetProviderKey())
}

// FileRetrieve is not supported by Runway provider.
func (provider *RunwayProvider) FileRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileRetrieveRequest) (*schemas.RakshaFileRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileRetrieveRequest, provider.GetProviderKey())
}

// FileDelete is not supported by Runway provider.
func (provider *RunwayProvider) FileDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileDeleteRequest) (*schemas.RakshaFileDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileDeleteRequest, provider.GetProviderKey())
}

// FileContent is not supported by Runway provider.
func (provider *RunwayProvider) FileContent(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileContentRequest) (*schemas.RakshaFileContentResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileContentRequest, provider.GetProviderKey())
}

// BatchCreate is not supported by Runway provider.
func (provider *RunwayProvider) BatchCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaBatchCreateRequest) (*schemas.RakshaBatchCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCreateRequest, provider.GetProviderKey())
}

// BatchList is not supported by Runway provider.
func (provider *RunwayProvider) BatchList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchListRequest) (*schemas.RakshaBatchListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchListRequest, provider.GetProviderKey())
}

// BatchRetrieve is not supported by Runway provider.
func (provider *RunwayProvider) BatchRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchRetrieveRequest) (*schemas.RakshaBatchRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchRetrieveRequest, provider.GetProviderKey())
}

// BatchCancel is not supported by Runway provider.
func (provider *RunwayProvider) BatchCancel(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchCancelRequest) (*schemas.RakshaBatchCancelResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCancelRequest, provider.GetProviderKey())
}

// BatchDelete is not supported by Runway provider.
func (provider *RunwayProvider) BatchDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchDeleteRequest) (*schemas.RakshaBatchDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchDeleteRequest, provider.GetProviderKey())
}

// BatchResults is not supported by Runway provider.
func (provider *RunwayProvider) BatchResults(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchResultsRequest) (*schemas.RakshaBatchResultsResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchResultsRequest, provider.GetProviderKey())
}

// CountTokens is not supported by the Runway provider.
func (provider *RunwayProvider) CountTokens(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaResponsesRequest) (*schemas.RakshaCountTokensResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CountTokensRequest, provider.GetProviderKey())
}

// Compaction is not supported by the Runway provider.
func (provider *RunwayProvider) Compaction(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaCompactionRequest) (*schemas.RakshaCompactionResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CompactionRequest, provider.GetProviderKey())
}

// ContainerCreate is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaContainerCreateRequest) (*schemas.RakshaContainerCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerCreateRequest, provider.GetProviderKey())
}

// ContainerList is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerListRequest) (*schemas.RakshaContainerListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerListRequest, provider.GetProviderKey())
}

// ContainerRetrieve is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerRetrieveRequest) (*schemas.RakshaContainerRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerRetrieveRequest, provider.GetProviderKey())
}

// ContainerDelete is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerDeleteRequest) (*schemas.RakshaContainerDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerDeleteRequest, provider.GetProviderKey())
}

// ContainerFileCreate is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerFileCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaContainerFileCreateRequest) (*schemas.RakshaContainerFileCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileCreateRequest, provider.GetProviderKey())
}

// ContainerFileList is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerFileList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileListRequest) (*schemas.RakshaContainerFileListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileListRequest, provider.GetProviderKey())
}

// ContainerFileRetrieve is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerFileRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileRetrieveRequest) (*schemas.RakshaContainerFileRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileRetrieveRequest, provider.GetProviderKey())
}

// ContainerFileContent is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerFileContent(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileContentRequest) (*schemas.RakshaContainerFileContentResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileContentRequest, provider.GetProviderKey())
}

// ContainerFileDelete is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerFileDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileDeleteRequest) (*schemas.RakshaContainerFileDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileDeleteRequest, provider.GetProviderKey())
}

// Passthrough is not supported by the Runway provider.
func (provider *RunwayProvider) Passthrough(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaPassthroughRequest) (*schemas.RakshaPassthroughResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughRequest, provider.GetProviderKey())
}

func (provider *RunwayProvider) PassthroughStream(_ *schemas.RakshaContext, _ schemas.PostHookRunner, _ func(context.Context), _ schemas.Key, _ *schemas.RakshaPassthroughRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughStreamRequest, provider.GetProviderKey())
}
