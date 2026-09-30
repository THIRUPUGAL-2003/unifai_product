// Package runware implements the Runware provider for UnifAI.
// Runware exposes a single synchronous endpoint that accepts an array of tasks; this
// provider supports its image operations (text-to-image, image-to-image, inpainting, outpainting),
// all of which use the "imageInference" task type.
package runware

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

// RunwareProvider implements the Provider interface for Runware's API.
type RunwareProvider struct {
	logger              schemas.Logger        // Logger for provider operations
	client              *fasthttp.Client      // HTTP client for API requests
	networkConfig       schemas.NetworkConfig // Network configuration including extra headers
	sendBackRawRequest  bool                  // Whether to include raw request in UnifAIResponse
	sendBackRawResponse bool                  // Whether to include raw response in UnifAIResponse
}

// NewRunwareProvider creates a new Runware provider instance.
func NewRunwareProvider(config *schemas.ProviderConfig, logger schemas.Logger) (*RunwareProvider, error) {
	config.CheckAndSetDefaults()

	requestTimeout := time.Second * time.Duration(config.NetworkConfig.DefaultRequestTimeoutInSeconds)
	client := &fasthttp.Client{
		ReadTimeout:         requestTimeout,
		WriteTimeout:        requestTimeout,
		MaxConnsPerHost:     config.NetworkConfig.MaxConnsPerHost,
		MaxIdleConnDuration: 60 * time.Second, // Image generation can be slow; keep connections warm longer.
		MaxConnWaitTimeout:  requestTimeout,
		MaxConnDuration:     time.Second * time.Duration(schemas.DefaultMaxConnDurationInSeconds),
		ConnPoolStrategy:    fasthttp.FIFO,
	}

	// Configure proxy if provided
	client = providerUtils.ConfigureProxy(client, config.ProxyConfig, logger)
	client = providerUtils.ConfigureDialer(client, config.NetworkConfig.AllowPrivateNetwork)
	client = providerUtils.ConfigureTLS(client, config.NetworkConfig, logger)

	// Set default BaseURL if not provided. Runware's single endpoint already includes /v1.
	if config.NetworkConfig.BaseURL == "" {
		config.NetworkConfig.BaseURL = "https://api.runware.ai/v1"
	}
	config.NetworkConfig.BaseURL = strings.TrimRight(config.NetworkConfig.BaseURL, "/")

	return &RunwareProvider{
		logger:              logger,
		client:              client,
		networkConfig:       config.NetworkConfig,
		sendBackRawRequest:  config.SendBackRawRequest,
		sendBackRawResponse: config.SendBackRawResponse,
	}, nil
}

// GetProviderKey returns the provider identifier for Runware.
func (provider *RunwareProvider) GetProviderKey() schemas.ModelProvider {
	return schemas.Runware
}

// ListModels is not supported by the Runware provider.
func (provider *RunwareProvider) ListModels(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAIListModelsRequest) (*schemas.UnifAIListModelsResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ListModelsRequest, provider.GetProviderKey())
}

// TextCompletion is not supported by the Runware provider.
func (provider *RunwareProvider) TextCompletion(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAITextCompletionRequest) (*schemas.UnifAITextCompletionResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionRequest, provider.GetProviderKey())
}

// TextCompletionStream is not supported by the Runware provider.
func (provider *RunwareProvider) TextCompletionStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAITextCompletionRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionStreamRequest, provider.GetProviderKey())
}

// ChatCompletion is not supported by the Runware provider.
func (provider *RunwareProvider) ChatCompletion(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIChatRequest) (*schemas.UnifAIChatResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ChatCompletionRequest, provider.GetProviderKey())
}

// ChatCompletionStream is not supported by the Runware provider.
func (provider *RunwareProvider) ChatCompletionStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAIChatRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ChatCompletionStreamRequest, provider.GetProviderKey())
}

// Responses is not supported by the Runware provider.
func (provider *RunwareProvider) Responses(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIResponsesRequest) (*schemas.UnifAIResponsesResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ResponsesRequest, provider.GetProviderKey())
}

// ResponsesStream is not supported by the Runware provider.
func (provider *RunwareProvider) ResponsesStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAIResponsesRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ResponsesStreamRequest, provider.GetProviderKey())
}

// Embedding is not supported by the Runware provider.
func (provider *RunwareProvider) Embedding(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIEmbeddingRequest) (*schemas.UnifAIEmbeddingResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.EmbeddingRequest, provider.GetProviderKey())
}

// Speech is not supported by the Runware provider.
func (provider *RunwareProvider) Speech(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAISpeechRequest) (*schemas.UnifAISpeechResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechRequest, provider.GetProviderKey())
}

// SpeechStream is not supported by the Runware provider.
func (provider *RunwareProvider) SpeechStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAISpeechRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechStreamRequest, provider.GetProviderKey())
}

// Transcription is not supported by the Runware provider.
func (provider *RunwareProvider) Transcription(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAITranscriptionRequest) (*schemas.UnifAITranscriptionResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionRequest, provider.GetProviderKey())
}

// TranscriptionStream is not supported by the Runware provider.
func (provider *RunwareProvider) TranscriptionStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAITranscriptionRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionStreamRequest, provider.GetProviderKey())
}

// ImageGeneration performs a text-to-image (or image-to-image) request to Runware's API.
func (provider *RunwareProvider) ImageGeneration(ctx *schemas.UnifAIContext, key schemas.Key, unifaiReq *schemas.UnifAIImageGenerationRequest) (*schemas.UnifAIImageGenerationResponse, *schemas.UnifAIError) {
	jsonData, unifaiErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		unifaiReq,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToRunwareImageGenerationRequest(unifaiReq)
		})
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	return provider.handleImageInference(ctx, key, unifaiReq.Model, jsonData)
}

// ImageEdit performs an image edit (image-to-image, inpainting, or outpainting) request to Runware's API.
func (provider *RunwareProvider) ImageEdit(ctx *schemas.UnifAIContext, key schemas.Key, unifaiReq *schemas.UnifAIImageEditRequest) (*schemas.UnifAIImageGenerationResponse, *schemas.UnifAIError) {
	jsonData, unifaiErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		unifaiReq,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToRunwareImageEditRequest(unifaiReq)
		})
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	return provider.handleImageInference(ctx, key, unifaiReq.Model, jsonData)
}

// handleImageInference wraps a single imageInference task in the array Runware expects, posts it
// to the unified endpoint, and converts the synchronous response into a UnifAI image response.
func (provider *RunwareProvider) handleImageInference(ctx *schemas.UnifAIContext, key schemas.Key, model string, jsonData []byte) (*schemas.UnifAIImageGenerationResponse, *schemas.UnifAIError) {
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)
	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)

	// Runware expects an array of tasks; wrap the single marshalled task object.
	body := make([]byte, 0, len(jsonData)+2)
	body = append(body, '[')
	body = append(body, jsonData...)
	body = append(body, ']')

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	req.SetRequestURI(provider.networkConfig.BaseURL + providerUtils.GetPathFromContext(ctx, ""))
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	req.SetBody(body)

	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.EnrichError(ctx, parseRunwareError(resp), body, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Decode response body
	respBody, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		rawErrBody := append([]byte(nil), resp.Body()...)
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, err), body, rawErrBody, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Parse response envelope
	var runwareResp RunwareResponse
	rawRequest, rawResponse, unifaiErr := providerUtils.HandleProviderResponse(respBody, &runwareResp, body, sendBackRawRequest, sendBackRawResponse)
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	// Convert to UnifAI response
	unifaiResp, unifaiErr := ToUnifAIImageGenerationResponse(&runwareResp)
	if unifaiErr != nil {
		return nil, providerUtils.EnrichError(ctx, unifaiErr, body, respBody, sendBackRawRequest, sendBackRawResponse, latency)
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

// Rerank is not supported by the Runware provider.
func (provider *RunwareProvider) Rerank(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIRerankRequest) (*schemas.UnifAIRerankResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.RerankRequest, provider.GetProviderKey())
}

// OCR is not supported by the Runware provider.
func (provider *RunwareProvider) OCR(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIOCRRequest) (*schemas.UnifAIOCRResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.OCRRequest, provider.GetProviderKey())
}

// ImageGenerationStream is not supported by the Runware provider.
func (provider *RunwareProvider) ImageGenerationStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAIImageGenerationRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationStreamRequest, provider.GetProviderKey())
}

// ImageEditStream is not supported by the Runware provider.
func (provider *RunwareProvider) ImageEditStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAIImageEditRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditStreamRequest, provider.GetProviderKey())
}

// ImageVariation is not supported by the Runware provider.
func (provider *RunwareProvider) ImageVariation(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIImageVariationRequest) (*schemas.UnifAIImageGenerationResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageVariationRequest, provider.GetProviderKey())
}

// sendTaskArray wraps a single task object in the Runware array envelope, posts it to the
// unified endpoint, and returns the wrapped request body, decoded response body, and latency.
func (provider *RunwareProvider) sendTaskArray(ctx *schemas.UnifAIContext, key schemas.Key, jsonData []byte) (reqBody []byte, respBody []byte, latency time.Duration, unifaiErr *schemas.UnifAIError) {
	reqBody = make([]byte, 0, len(jsonData)+2)
	reqBody = append(reqBody, '[')
	reqBody = append(reqBody, jsonData...)
	reqBody = append(reqBody, ']')

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.SetRequestURI(provider.networkConfig.BaseURL + providerUtils.GetPathFromContext(ctx, ""))
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}
	req.SetBody(reqBody)

	lat, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return reqBody, nil, lat, unifaiErr
	}
	if resp.StatusCode() != fasthttp.StatusOK {
		return reqBody, nil, lat, providerUtils.SetErrorLatency(parseRunwareError(resp), lat)
	}
	decoded, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return reqBody, nil, lat, providerUtils.SetErrorLatency(providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, err), lat)
	}
	// Copy out: the fasthttp response buffer is released when this function returns.
	return reqBody, append([]byte(nil), decoded...), lat, nil
}

// VideoGeneration submits an async videoInference task and returns the queued job.
// The caller polls VideoRetrieve to fetch the finished video.
func (provider *RunwareProvider) VideoGeneration(ctx *schemas.UnifAIContext, key schemas.Key, unifaiReq *schemas.UnifAIVideoGenerationRequest) (*schemas.UnifAIVideoGenerationResponse, *schemas.UnifAIError) {
	providerName := provider.GetProviderKey()
	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)

	jsonData, unifaiErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		unifaiReq,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToRunwareVideoGenerationRequest(unifaiReq)
		})
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	reqBody, respBody, latency, unifaiErr := provider.sendTaskArray(ctx, key, jsonData)
	if unifaiErr != nil {
		return nil, providerUtils.EnrichError(ctx, unifaiErr, reqBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	var videoResp RunwareResponse
	rawRequest, rawResponse, unifaiErr := providerUtils.HandleProviderResponse(respBody, &videoResp, reqBody, sendBackRawRequest, sendBackRawResponse)
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	result, unifaiErr := firstVideoResult(&videoResp)
	if unifaiErr != nil {
		return nil, providerUtils.EnrichError(ctx, unifaiErr, reqBody, respBody, sendBackRawRequest, sendBackRawResponse, latency)
	}

	unifaiResp := ToUnifAIVideoGenerationResponse(result)
	unifaiResp.ID = providerUtils.AddVideoIDProviderSuffix(result.TaskUUID, providerName)
	unifaiResp.Model = unifaiReq.Model
	unifaiResp.ExtraFields.Latency = latency.Milliseconds()
	if sendBackRawRequest {
		unifaiResp.ExtraFields.RawRequest = rawRequest
	}
	if sendBackRawResponse {
		unifaiResp.ExtraFields.RawResponse = rawResponse
	}

	return unifaiResp, nil
}

// VideoRetrieve polls a previously submitted videoInference task via a getResponse task.
func (provider *RunwareProvider) VideoRetrieve(ctx *schemas.UnifAIContext, key schemas.Key, unifaiReq *schemas.UnifAIVideoRetrieveRequest) (*schemas.UnifAIVideoGenerationResponse, *schemas.UnifAIError) {
	providerName := provider.GetProviderKey()
	taskID := providerUtils.StripVideoIDProviderSuffix(unifaiReq.ID, providerName)
	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)

	jsonData, err := providerUtils.MarshalSorted(RunwareGetResponseRequest{TaskType: taskTypeGetResponse, TaskUUID: taskID})
	if err != nil {
		return nil, providerUtils.NewUnifAIOperationError(schemas.ErrProviderRequestMarshal, err)
	}

	reqBody, respBody, latency, unifaiErr := provider.sendTaskArray(ctx, key, jsonData)
	if unifaiErr != nil {
		return nil, providerUtils.EnrichError(ctx, unifaiErr, reqBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	var videoResp RunwareResponse
	rawRequest, rawResponse, unifaiErr := providerUtils.HandleProviderResponse(respBody, &videoResp, reqBody, sendBackRawRequest, sendBackRawResponse)
	if unifaiErr != nil {
		return nil, unifaiErr
	}

	result, unifaiErr := firstVideoResult(&videoResp)
	if unifaiErr != nil {
		return nil, providerUtils.EnrichError(ctx, unifaiErr, reqBody, respBody, sendBackRawRequest, sendBackRawResponse, latency)
	}

	unifaiResp := ToUnifAIVideoGenerationResponse(result)
	unifaiResp.ID = providerUtils.AddVideoIDProviderSuffix(taskID, providerName)
	unifaiResp.ExtraFields.Latency = latency.Milliseconds()
	if sendBackRawRequest {
		unifaiResp.ExtraFields.RawRequest = rawRequest
	}
	if sendBackRawResponse {
		unifaiResp.ExtraFields.RawResponse = rawResponse
	}

	return unifaiResp, nil
}

// VideoDownload retrieves the task, then downloads the finished video from its URL.
func (provider *RunwareProvider) VideoDownload(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIVideoDownloadRequest) (*schemas.UnifAIVideoDownloadResponse, *schemas.UnifAIError) {
	taskDetails, unifaiErr := provider.VideoRetrieve(ctx, key, &schemas.UnifAIVideoRetrieveRequest{Provider: request.Provider, ID: request.ID})
	if unifaiErr != nil {
		return nil, unifaiErr
	}
	if taskDetails.Status != schemas.VideoStatusCompleted {
		return nil, providerUtils.NewUnifAIOperationError(fmt.Sprintf("video not ready, current status: %s", taskDetails.Status), nil)
	}
	if len(taskDetails.Videos) == 0 || taskDetails.Videos[0].URL == nil || *taskDetails.Videos[0].URL == "" {
		return nil, providerUtils.NewUnifAIOperationError("video URL not available", nil)
	}
	videoURL := *taskDetails.Videos[0].URL

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)
	req.SetRequestURI(videoURL)
	req.Header.SetMethod(http.MethodGet)

	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, unifaiErr
	}
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.SetErrorLatency(providerUtils.NewUnifAIOperationError(fmt.Sprintf("failed to download video: HTTP %d", resp.StatusCode()), nil), latency)
	}
	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, err)
	}
	contentType := string(resp.Header.ContentType())
	if contentType == "" {
		contentType = "video/mp4"
	}

	unifaiResp := &schemas.UnifAIVideoDownloadResponse{
		VideoID:     request.ID,
		Content:     append([]byte(nil), body...),
		ContentType: contentType,
	}
	unifaiResp.ExtraFields.Latency = latency.Milliseconds()

	return unifaiResp, nil
}

// firstVideoResult returns the first video task result, surfacing task-level errors.
func firstVideoResult(resp *RunwareResponse) (*RunwareResult, *schemas.UnifAIError) {
	if len(resp.Data) == 0 {
		if msg := firstRunwareErrorMessage(resp.Errors); msg != "" {
			return nil, providerUtils.NewUnifAIOperationError(msg, nil)
		}
		return nil, providerUtils.NewUnifAIOperationError("runware returned no video task", nil)
	}
	return &resp.Data[0], nil
}

// VideoDelete is not supported by the Runware provider.
func (provider *RunwareProvider) VideoDelete(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIVideoDeleteRequest) (*schemas.UnifAIVideoDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDeleteRequest, provider.GetProviderKey())
}

// VideoList is not supported by the Runware provider.
func (provider *RunwareProvider) VideoList(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIVideoListRequest) (*schemas.UnifAIVideoListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoListRequest, provider.GetProviderKey())
}

// VideoRemix is not supported by the Runware provider.
func (provider *RunwareProvider) VideoRemix(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIVideoRemixRequest) (*schemas.UnifAIVideoGenerationResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRemixRequest, provider.GetProviderKey())
}

// FileUpload is not supported by Runware provider.
func (provider *RunwareProvider) FileUpload(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIFileUploadRequest) (*schemas.UnifAIFileUploadResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileUploadRequest, provider.GetProviderKey())
}

// FileList is not supported by Runware provider.
func (provider *RunwareProvider) FileList(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIFileListRequest) (*schemas.UnifAIFileListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileListRequest, provider.GetProviderKey())
}

// FileRetrieve is not supported by Runware provider.
func (provider *RunwareProvider) FileRetrieve(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIFileRetrieveRequest) (*schemas.UnifAIFileRetrieveResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileRetrieveRequest, provider.GetProviderKey())
}

// FileDelete is not supported by Runware provider.
func (provider *RunwareProvider) FileDelete(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIFileDeleteRequest) (*schemas.UnifAIFileDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileDeleteRequest, provider.GetProviderKey())
}

// FileContent is not supported by Runware provider.
func (provider *RunwareProvider) FileContent(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIFileContentRequest) (*schemas.UnifAIFileContentResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileContentRequest, provider.GetProviderKey())
}

// BatchCreate is not supported by Runware provider.
func (provider *RunwareProvider) BatchCreate(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIBatchCreateRequest) (*schemas.UnifAIBatchCreateResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCreateRequest, provider.GetProviderKey())
}

// BatchList is not supported by Runware provider.
func (provider *RunwareProvider) BatchList(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIBatchListRequest) (*schemas.UnifAIBatchListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchListRequest, provider.GetProviderKey())
}

// BatchRetrieve is not supported by Runware provider.
func (provider *RunwareProvider) BatchRetrieve(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIBatchRetrieveRequest) (*schemas.UnifAIBatchRetrieveResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchRetrieveRequest, provider.GetProviderKey())
}

// BatchCancel is not supported by Runware provider.
func (provider *RunwareProvider) BatchCancel(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIBatchCancelRequest) (*schemas.UnifAIBatchCancelResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCancelRequest, provider.GetProviderKey())
}

// BatchDelete is not supported by Runware provider.
func (provider *RunwareProvider) BatchDelete(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIBatchDeleteRequest) (*schemas.UnifAIBatchDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchDeleteRequest, provider.GetProviderKey())
}

// BatchResults is not supported by Runware provider.
func (provider *RunwareProvider) BatchResults(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIBatchResultsRequest) (*schemas.UnifAIBatchResultsResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchResultsRequest, provider.GetProviderKey())
}

// CountTokens is not supported by the Runware provider.
func (provider *RunwareProvider) CountTokens(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIResponsesRequest) (*schemas.UnifAICountTokensResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CountTokensRequest, provider.GetProviderKey())
}

// Compaction is not supported by the Runware provider.
func (provider *RunwareProvider) Compaction(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAICompactionRequest) (*schemas.UnifAICompactionResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CompactionRequest, provider.GetProviderKey())
}

// ContainerCreate is not supported by the Runware provider.
func (provider *RunwareProvider) ContainerCreate(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIContainerCreateRequest) (*schemas.UnifAIContainerCreateResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerCreateRequest, provider.GetProviderKey())
}

// ContainerList is not supported by the Runware provider.
func (provider *RunwareProvider) ContainerList(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerListRequest) (*schemas.UnifAIContainerListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerListRequest, provider.GetProviderKey())
}

// ContainerRetrieve is not supported by the Runware provider.
func (provider *RunwareProvider) ContainerRetrieve(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerRetrieveRequest) (*schemas.UnifAIContainerRetrieveResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerRetrieveRequest, provider.GetProviderKey())
}

// ContainerDelete is not supported by the Runware provider.
func (provider *RunwareProvider) ContainerDelete(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerDeleteRequest) (*schemas.UnifAIContainerDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerDeleteRequest, provider.GetProviderKey())
}

// ContainerFileCreate is not supported by the Runware provider.
func (provider *RunwareProvider) ContainerFileCreate(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIContainerFileCreateRequest) (*schemas.UnifAIContainerFileCreateResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileCreateRequest, provider.GetProviderKey())
}

// ContainerFileList is not supported by the Runware provider.
func (provider *RunwareProvider) ContainerFileList(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerFileListRequest) (*schemas.UnifAIContainerFileListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileListRequest, provider.GetProviderKey())
}

// ContainerFileRetrieve is not supported by the Runware provider.
func (provider *RunwareProvider) ContainerFileRetrieve(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerFileRetrieveRequest) (*schemas.UnifAIContainerFileRetrieveResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileRetrieveRequest, provider.GetProviderKey())
}

// ContainerFileContent is not supported by the Runware provider.
func (provider *RunwareProvider) ContainerFileContent(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerFileContentRequest) (*schemas.UnifAIContainerFileContentResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileContentRequest, provider.GetProviderKey())
}

// ContainerFileDelete is not supported by the Runware provider.
func (provider *RunwareProvider) ContainerFileDelete(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerFileDeleteRequest) (*schemas.UnifAIContainerFileDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileDeleteRequest, provider.GetProviderKey())
}

// Passthrough is not supported by the Runware provider.
func (provider *RunwareProvider) Passthrough(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIPassthroughRequest) (*schemas.UnifAIPassthroughResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughRequest, provider.GetProviderKey())
}

// PassthroughStream is not supported by the Runware provider.
func (provider *RunwareProvider) PassthroughStream(_ *schemas.UnifAIContext, _ schemas.PostHookRunner, _ func(context.Context), _ schemas.Key, _ *schemas.UnifAIPassthroughRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughStreamRequest, provider.GetProviderKey())
}
