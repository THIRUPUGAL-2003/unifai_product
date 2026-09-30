// Package providers implements various LLM providers and their utility functions.
// This file contains the Runway provider implementation.
package runway

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	providerUtils "github.com/unifai/unifai/core/providers/utils"
	schemas "github.com/unifai/unifai/core/schemas"
	"github.com/valyala/fasthttp"
)

// RunwayProvider implements the Provider interface for Runway's API.
type RunwayProvider struct {
	logger              schemas.Logger        // Logger for provider operations
	client              *fasthttp.Client      // HTTP client for API requests
	networkConfig       schemas.NetworkConfig // Network configuration including extra headers
	sendBackRawRequest  bool                  // Whether to include raw request in UnifAIResponse
	sendBackRawResponse bool                  // Whether to include raw response in UnifAIResponse
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
func (provider *RunwayProvider) ListModels(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAIListModelsRequest) (*schemas.UnifAIListModelsResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ListModelsRequest, provider.GetProviderKey())
}

// TextCompletion is not supported by the Runway provider.
func (provider *RunwayProvider) TextCompletion(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAITextCompletionRequest) (*schemas.UnifAITextCompletionResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionRequest, provider.GetProviderKey())
}

// TextCompletionStream is not supported by the Runway provider.
func (provider *RunwayProvider) TextCompletionStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAITextCompletionRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionStreamRequest, provider.GetProviderKey())
}

// ChatCompletion is not supported by the Runway provider.
func (provider *RunwayProvider) ChatCompletion(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIChatRequest) (*schemas.UnifAIChatResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ChatCompletionRequest, provider.GetProviderKey())
}

// ChatCompletionStream is not supported by the Runway provider.
func (provider *RunwayProvider) ChatCompletionStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAIChatRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ChatCompletionStreamRequest, provider.GetProviderKey())
}

// Responses is not supported by the Runway provider.
func (provider *RunwayProvider) Responses(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIResponsesRequest) (*schemas.UnifAIResponsesResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ResponsesRequest, provider.GetProviderKey())
}

// ResponsesStream is not supported by the Runway provider.
func (provider *RunwayProvider) ResponsesStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAIResponsesRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ResponsesStreamRequest, provider.GetProviderKey())
}

// Embedding is not supported by the Runway provider.
func (provider *RunwayProvider) Embedding(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIEmbeddingRequest) (*schemas.UnifAIEmbeddingResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.EmbeddingRequest, provider.GetProviderKey())
}

// Speech is not supported by the Runway provider.
func (provider *RunwayProvider) Speech(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAISpeechRequest) (*schemas.UnifAISpeechResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechRequest, provider.GetProviderKey())
}

// SpeechStream is not supported by the Runway provider.
func (provider *RunwayProvider) SpeechStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAISpeechRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechStreamRequest, provider.GetProviderKey())
}

// Transcription is not supported by the Runway provider.
func (provider *RunwayProvider) Transcription(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAITranscriptionRequest) (*schemas.UnifAITranscriptionResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionRequest, provider.GetProviderKey())
}

// TranscriptionStream is not supported by the Runway provider.
func (provider *RunwayProvider) TranscriptionStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAITranscriptionRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionStreamRequest, provider.GetProviderKey())
}

// Rerank is not supported by the Runway provider.
func (provider *RunwayProvider) Rerank(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIRerankRequest) (*schemas.UnifAIRerankResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.RerankRequest, provider.GetProviderKey())
}

// OCR is not supported by the Runway provider.
func (provider *RunwayProvider) OCR(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIOCRRequest) (*schemas.UnifAIOCRResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.OCRRequest, provider.GetProviderKey())
}

// ImageGeneration performs a text-to-image generation request to Runway's API.
// Runway image generation is task-based: a task is created and then polled until it
// reaches a terminal state, after which the generated image URLs are returned.
func (provider *RunwayProvider) ImageGeneration(ctx *schemas.UnifAIContext, key schemas.Key, unifaiReq *schemas.UnifAIImageGenerationRequest) (*schemas.UnifAIImageGenerationResponse, *schemas.UnifAIError) {
	// Convert UnifAI request to Runway format
	jsonData, unifaiErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		unifaiReq,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToRunwayImageGenerationRequest(unifaiReq)
		})
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	return provider.HandleRunwayImageTask(ctx, key, unifaiReq.Model, jsonData)
}

// HandleRunwayImageTask posts a prebuilt text_to_image body, polls the task to completion,
// and builds the UnifAI image response. Shared by ImageGeneration and ImageEdit since both
// use the same /v1/text_to_image endpoint and response shape.
func (provider *RunwayProvider) HandleRunwayImageTask(ctx *schemas.UnifAIContext, key schemas.Key, model string, jsonData []byte) (*schemas.UnifAIImageGenerationResponse, *schemas.UnifAIError) {
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

	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.EnrichError(ctx, parseRunwayError(resp), jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Decode response body
	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		rawErrBody := append([]byte(nil), resp.Body()...)
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, err), jsonData, rawErrBody, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Parse task creation response
	var taskResp RunwayTaskCreationResponse
	rawRequest, _, unifaiErr := providerUtils.HandleProviderResponse(body, &taskResp, jsonData, sendBackRawRequest, sendBackRawResponse)
	if unifaiErr != nil {
		return nil, providerUtils.SetErrorLatency(unifaiErr, latency)
	}

	// Poll the task until it reaches a terminal state
	taskDetails, rawResponse, unifaiErr := provider.pollRunwayTask(ctx, key, taskResp.ID, sendBackRawResponse)
	if unifaiErr != nil {
		return nil, providerUtils.EnrichError(ctx, unifaiErr, jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Convert to UnifAI response
	unifaiResp, unifaiErr := ToUnifAIImageGenerationResponse(taskDetails)
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	unifaiResp.Model = model
	unifaiResp.ExtraFields.Latency = latency.Milliseconds()

	if sendBackRawRequest {
		unifaiResp.ExtraFields.RawRequest = rawRequest
	}
	if sendBackRawResponse {
		unifaiResp.ExtraFields.RawResponse = rawResponse
	}

	return unifaiResp, nil
}

// runwayImagePollingInterval is the interval between Runway task status polls for image generation.
const runwayImagePollingInterval = 2 * time.Second

// pollRunwayTask polls a Runway task until it reaches a terminal state or the context times out.
func (provider *RunwayProvider) pollRunwayTask(ctx *schemas.UnifAIContext, key schemas.Key, taskID string, sendBackRawResponse bool) (*RunwayTaskDetailsResponse, interface{}, *schemas.UnifAIError) {
	pollCtx, cancel := schemas.NewUnifAIContextWithTimeout(ctx, time.Duration(provider.networkConfig.DefaultRequestTimeoutInSeconds)*time.Second)
	defer cancel()

	ticker := time.NewTicker(runwayImagePollingInterval)
	defer ticker.Stop()

	for {
		taskDetails, rawResponse, unifaiErr := provider.retrieveRunwayTask(pollCtx, key, taskID, sendBackRawResponse)
		if unifaiErr != nil {
			return nil, nil, unifaiErr
		}

		switch taskDetails.Status {
		case RunwayTaskStatusSucceeded:
			return taskDetails, rawResponse, nil
		case RunwayTaskStatusFailed, RunwayTaskStatusCancelled:
			return nil, nil, providerUtils.NewUnifAIOperationError(fmt.Sprintf("runway task %s", taskDetails.Status), nil)
		}

		select {
		case <-pollCtx.Done():
			return nil, nil, providerUtils.NewUnifAIOperationError(schemas.ErrProviderRequestTimedOut, fmt.Errorf("runway task polling timed out"))
		case <-ticker.C:
		}
	}
}

// retrieveRunwayTask fetches the current state of a Runway task.
func (provider *RunwayProvider) retrieveRunwayTask(ctx *schemas.UnifAIContext, key schemas.Key, taskID string, sendBackRawResponse bool) (*RunwayTaskDetailsResponse, interface{}, *schemas.UnifAIError) {
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

	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, nil, unifaiErr
	}

	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, nil, providerUtils.SetErrorLatency(parseRunwayError(resp), latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, nil, providerUtils.SetErrorLatency(providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, err), latency)
	}

	var taskDetails RunwayTaskDetailsResponse
	_, rawResponse, unifaiErr := providerUtils.HandleProviderResponse(body, &taskDetails, nil, false, sendBackRawResponse)
	if unifaiErr != nil {
		return nil, nil, providerUtils.SetErrorLatency(unifaiErr, latency)
	}

	return &taskDetails, rawResponse, nil
}

// ImageGenerationStream is not supported by the Runway provider.
func (provider *RunwayProvider) ImageGenerationStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAIImageGenerationRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationStreamRequest, provider.GetProviderKey())
}

// ImageEdit performs an image edit request to Runway's API. Runway has no dedicated edit
// endpoint, so the input images are passed as reference images to /v1/text_to_image.
func (provider *RunwayProvider) ImageEdit(ctx *schemas.UnifAIContext, key schemas.Key, unifaiReq *schemas.UnifAIImageEditRequest) (*schemas.UnifAIImageGenerationResponse, *schemas.UnifAIError) {
	// Convert UnifAI request to Runway format
	jsonData, unifaiErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		unifaiReq,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToRunwayImageEditRequest(unifaiReq)
		})
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	return provider.HandleRunwayImageTask(ctx, key, unifaiReq.Model, jsonData)
}

// ImageEditStream is not supported by the Runway provider.
func (provider *RunwayProvider) ImageEditStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAIImageEditRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditStreamRequest, provider.GetProviderKey())
}

// ImageVariation is not supported by the Runway provider.
func (provider *RunwayProvider) ImageVariation(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIImageVariationRequest) (*schemas.UnifAIImageGenerationResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageVariationRequest, provider.GetProviderKey())
}

// VideoGeneration performs a video generation request to Runway's API.
func (provider *RunwayProvider) VideoGeneration(ctx *schemas.UnifAIContext, key schemas.Key, unifaiReq *schemas.UnifAIVideoGenerationRequest) (*schemas.UnifAIVideoGenerationResponse, *schemas.UnifAIError) {
	providerName := provider.GetProviderKey()
	model := unifaiReq.Model

	// Convert UnifAI request to Runway format
	jsonData, unifaiErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		unifaiReq,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToRunwayVideoGenerationRequest(unifaiReq)
		})
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Determine the endpoint based on request type
	endpoint := getRunwayEndpoint(unifaiReq)

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

	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.EnrichError(ctx, parseRunwayError(resp), jsonData, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Decode response body
	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		rawErrBody := append([]byte(nil), resp.Body()...)
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, err), jsonData, rawErrBody, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Parse response
	var taskResp RunwayTaskCreationResponse
	rawRequest, rawResponse, unifaiErr := providerUtils.HandleProviderResponse(body, &taskResp, jsonData, sendBackRawRequest, sendBackRawResponse)
	if unifaiErr != nil {
		return nil, providerUtils.SetErrorLatency(unifaiErr, latency)
	}

	// Convert to UnifAI response
	unifaiResp := &schemas.UnifAIVideoGenerationResponse{
		ID:     providerUtils.AddVideoIDProviderSuffix(taskResp.ID, providerName),
		Model:  model,
		Object: "video",
		Status: schemas.VideoStatusQueued,
		ExtraFields: schemas.UnifAIResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}

	if sendBackRawRequest {
		unifaiResp.ExtraFields.RawRequest = rawRequest
	}
	if sendBackRawResponse {
		unifaiResp.ExtraFields.RawResponse = rawResponse
	}

	return unifaiResp, nil
}

// VideoRetrieve retrieves the status of a video generation task from Runway's API.
func (provider *RunwayProvider) VideoRetrieve(ctx *schemas.UnifAIContext, key schemas.Key, unifaiReq *schemas.UnifAIVideoRetrieveRequest) (*schemas.UnifAIVideoGenerationResponse, *schemas.UnifAIError) {
	providerName := provider.GetProviderKey()
	taskID := providerUtils.StripVideoIDProviderSuffix(unifaiReq.ID, providerName)

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

	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.EnrichError(ctx, parseRunwayError(resp), nil, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Decode response body
	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		rawErrBody := append([]byte(nil), resp.Body()...)
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, err), nil, rawErrBody, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Parse response
	var taskDetails RunwayTaskDetailsResponse
	rawRequest, rawResponse, unifaiErr := providerUtils.HandleProviderResponse(body, &taskDetails, nil, sendBackRawRequest, sendBackRawResponse)
	if unifaiErr != nil {
		return nil, providerUtils.SetErrorLatency(unifaiErr, latency)
	}

	// Convert to UnifAI response
	unifaiResp, unifaiErr := ToUnifAIVideoGenerationResponse(&taskDetails)
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	unifaiResp.ID = providerUtils.AddVideoIDProviderSuffix(unifaiResp.ID, providerName)
	unifaiResp.ExtraFields.Latency = latency.Milliseconds()

	if sendBackRawRequest {
		unifaiResp.ExtraFields.RawRequest = rawRequest
	}
	if sendBackRawResponse {
		unifaiResp.ExtraFields.RawResponse = rawResponse
	}

	return unifaiResp, nil
}

// VideoDownload retrieves a video from Runway's API.
func (provider *RunwayProvider) VideoDownload(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIVideoDownloadRequest) (*schemas.UnifAIVideoDownloadResponse, *schemas.UnifAIError) {
	// Retrieve task status to get the video URL
	unifaiVideoRetrieveRequest := &schemas.UnifAIVideoRetrieveRequest{
		Provider: request.Provider,
		ID:       request.ID,
	}
	taskDetails, unifaiErr := provider.VideoRetrieve(ctx, key, unifaiVideoRetrieveRequest)
	if unifaiErr != nil {
		return nil, unifaiErr
	}
	// Check if video is ready
	if taskDetails.Status != schemas.VideoStatusCompleted {
		return nil, providerUtils.NewUnifAIOperationError(
			fmt.Sprintf("video not ready, current status: %s", taskDetails.Status),
			nil)
	}
	if len(taskDetails.Videos) == 0 {
		return nil, providerUtils.NewUnifAIOperationError("video URL not available", nil)
	}
	var videoUrl string
	if taskDetails.Videos[0].URL != nil {
		videoUrl = *taskDetails.Videos[0].URL
	}
	if videoUrl == "" {
		return nil, providerUtils.NewUnifAIOperationError("invalid video output type", nil)
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
	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, unifaiErr
	}
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.SetErrorLatency(providerUtils.NewUnifAIOperationError(
			fmt.Sprintf("failed to download video: HTTP %d", resp.StatusCode()),
			nil), latency)
	}
	// Get content and content type
	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		rawErrBody := append([]byte(nil), resp.Body()...)
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, err), nil, rawErrBody, sendBackRawRequest, sendBackRawResponse, latency)
	}
	contentType := string(resp.Header.ContentType())
	if contentType == "" {
		contentType = "video/mp4" // Default for Runway
	}
	// Copy the binary content
	content := append([]byte(nil), body...)
	unifaiResp := &schemas.UnifAIVideoDownloadResponse{
		VideoID:     request.ID,
		Content:     content,
		ContentType: contentType,
	}

	unifaiResp.ExtraFields.Latency = latency.Milliseconds()

	return unifaiResp, nil
}

// VideoDelete cancels or deletes a task in Runway.
// Tasks that are running, pending, or throttled can be canceled by invoking this method.
// Invoking this method for other tasks will delete them.
func (provider *RunwayProvider) VideoDelete(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIVideoDeleteRequest) (*schemas.UnifAIVideoDeleteResponse, *schemas.UnifAIError) {
	providerName := provider.GetProviderKey()

	if request.ID == "" {
		return nil, providerUtils.NewUnifAIOperationError("task_id is required", nil)
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
	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Handle error response - Runway returns 204 No Content on success
	if resp.StatusCode() != fasthttp.StatusNoContent {
		return nil, providerUtils.EnrichError(ctx, parseRunwayError(resp), nil, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Build response - Runway returns empty body on 204
	response := &schemas.UnifAIVideoDeleteResponse{
		ID:      request.ID, // Return with provider prefix
		Object:  "video.deleted",
		Deleted: true,
	}

	response.ExtraFields.Latency = latency.Milliseconds()

	return response, nil
}

// VideoList is not supported by Runway provider.
func (provider *RunwayProvider) VideoList(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIVideoListRequest) (*schemas.UnifAIVideoListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoListRequest, provider.GetProviderKey())
}

// VideoRemix is not supported by Runway provider.
func (provider *RunwayProvider) VideoRemix(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIVideoRemixRequest) (*schemas.UnifAIVideoGenerationResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRemixRequest, provider.GetProviderKey())
}

// FileUpload is not supported by Runway provider.
func (provider *RunwayProvider) FileUpload(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIFileUploadRequest) (*schemas.UnifAIFileUploadResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileUploadRequest, provider.GetProviderKey())
}

// FileList is not supported by Runway provider.
func (provider *RunwayProvider) FileList(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIFileListRequest) (*schemas.UnifAIFileListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileListRequest, provider.GetProviderKey())
}

// FileRetrieve is not supported by Runway provider.
func (provider *RunwayProvider) FileRetrieve(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIFileRetrieveRequest) (*schemas.UnifAIFileRetrieveResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileRetrieveRequest, provider.GetProviderKey())
}

// FileDelete is not supported by Runway provider.
func (provider *RunwayProvider) FileDelete(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIFileDeleteRequest) (*schemas.UnifAIFileDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileDeleteRequest, provider.GetProviderKey())
}

// FileContent is not supported by Runway provider.
func (provider *RunwayProvider) FileContent(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIFileContentRequest) (*schemas.UnifAIFileContentResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileContentRequest, provider.GetProviderKey())
}

// BatchCreate is not supported by Runway provider.
func (provider *RunwayProvider) BatchCreate(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIBatchCreateRequest) (*schemas.UnifAIBatchCreateResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCreateRequest, provider.GetProviderKey())
}

// BatchList is not supported by Runway provider.
func (provider *RunwayProvider) BatchList(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIBatchListRequest) (*schemas.UnifAIBatchListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchListRequest, provider.GetProviderKey())
}

// BatchRetrieve is not supported by Runway provider.
func (provider *RunwayProvider) BatchRetrieve(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIBatchRetrieveRequest) (*schemas.UnifAIBatchRetrieveResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchRetrieveRequest, provider.GetProviderKey())
}

// BatchCancel is not supported by Runway provider.
func (provider *RunwayProvider) BatchCancel(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIBatchCancelRequest) (*schemas.UnifAIBatchCancelResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCancelRequest, provider.GetProviderKey())
}

// BatchDelete is not supported by Runway provider.
func (provider *RunwayProvider) BatchDelete(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIBatchDeleteRequest) (*schemas.UnifAIBatchDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchDeleteRequest, provider.GetProviderKey())
}

// BatchResults is not supported by Runway provider.
func (provider *RunwayProvider) BatchResults(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIBatchResultsRequest) (*schemas.UnifAIBatchResultsResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchResultsRequest, provider.GetProviderKey())
}

// CountTokens is not supported by the Runway provider.
func (provider *RunwayProvider) CountTokens(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIResponsesRequest) (*schemas.UnifAICountTokensResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CountTokensRequest, provider.GetProviderKey())
}

// Compaction is not supported by the Runway provider.
func (provider *RunwayProvider) Compaction(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAICompactionRequest) (*schemas.UnifAICompactionResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CompactionRequest, provider.GetProviderKey())
}

// ContainerCreate is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerCreate(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIContainerCreateRequest) (*schemas.UnifAIContainerCreateResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerCreateRequest, provider.GetProviderKey())
}

// ContainerList is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerList(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerListRequest) (*schemas.UnifAIContainerListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerListRequest, provider.GetProviderKey())
}

// ContainerRetrieve is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerRetrieve(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerRetrieveRequest) (*schemas.UnifAIContainerRetrieveResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerRetrieveRequest, provider.GetProviderKey())
}

// ContainerDelete is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerDelete(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerDeleteRequest) (*schemas.UnifAIContainerDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerDeleteRequest, provider.GetProviderKey())
}

// ContainerFileCreate is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerFileCreate(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIContainerFileCreateRequest) (*schemas.UnifAIContainerFileCreateResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileCreateRequest, provider.GetProviderKey())
}

// ContainerFileList is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerFileList(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerFileListRequest) (*schemas.UnifAIContainerFileListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileListRequest, provider.GetProviderKey())
}

// ContainerFileRetrieve is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerFileRetrieve(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerFileRetrieveRequest) (*schemas.UnifAIContainerFileRetrieveResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileRetrieveRequest, provider.GetProviderKey())
}

// ContainerFileContent is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerFileContent(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerFileContentRequest) (*schemas.UnifAIContainerFileContentResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileContentRequest, provider.GetProviderKey())
}

// ContainerFileDelete is not supported by the Runway provider.
func (provider *RunwayProvider) ContainerFileDelete(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerFileDeleteRequest) (*schemas.UnifAIContainerFileDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileDeleteRequest, provider.GetProviderKey())
}

// Passthrough is not supported by the Runway provider.
func (provider *RunwayProvider) Passthrough(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIPassthroughRequest) (*schemas.UnifAIPassthroughResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughRequest, provider.GetProviderKey())
}

func (provider *RunwayProvider) PassthroughStream(_ *schemas.UnifAIContext, _ schemas.PostHookRunner, _ func(context.Context), _ schemas.Key, _ *schemas.UnifAIPassthroughRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughStreamRequest, provider.GetProviderKey())
}
