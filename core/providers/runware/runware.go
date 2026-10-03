// Package runware implements the Runware provider for Raksha.
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

	providerUtils "github.com/raksha/raksha/core/providers/utils"
	schemas "github.com/raksha/raksha/core/schemas"
	"github.com/valyala/fasthttp"
)

// RunwareProvider implements the Provider interface for Runware's API.
type RunwareProvider struct {
	logger              schemas.Logger        // Logger for provider operations
	client              *fasthttp.Client      // HTTP client for API requests
	networkConfig       schemas.NetworkConfig // Network configuration including extra headers
	sendBackRawRequest  bool                  // Whether to include raw request in RakshaResponse
	sendBackRawResponse bool                  // Whether to include raw response in RakshaResponse
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
func (provider *RunwareProvider) ListModels(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaListModelsRequest) (*schemas.RakshaListModelsResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ListModelsRequest, provider.GetProviderKey())
}

// TextCompletion is not supported by the Runware provider.
func (provider *RunwareProvider) TextCompletion(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaTextCompletionRequest) (*schemas.RakshaTextCompletionResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionRequest, provider.GetProviderKey())
}

// TextCompletionStream is not supported by the Runware provider.
func (provider *RunwareProvider) TextCompletionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaTextCompletionRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionStreamRequest, provider.GetProviderKey())
}

// ChatCompletion is not supported by the Runware provider.
func (provider *RunwareProvider) ChatCompletion(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaChatRequest) (*schemas.RakshaChatResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ChatCompletionRequest, provider.GetProviderKey())
}

// ChatCompletionStream is not supported by the Runware provider.
func (provider *RunwareProvider) ChatCompletionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaChatRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ChatCompletionStreamRequest, provider.GetProviderKey())
}

// Responses is not supported by the Runware provider.
func (provider *RunwareProvider) Responses(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaResponsesRequest) (*schemas.RakshaResponsesResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ResponsesRequest, provider.GetProviderKey())
}

// ResponsesStream is not supported by the Runware provider.
func (provider *RunwareProvider) ResponsesStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaResponsesRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ResponsesStreamRequest, provider.GetProviderKey())
}

// Embedding is not supported by the Runware provider.
func (provider *RunwareProvider) Embedding(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaEmbeddingRequest) (*schemas.RakshaEmbeddingResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.EmbeddingRequest, provider.GetProviderKey())
}

// Speech is not supported by the Runware provider.
func (provider *RunwareProvider) Speech(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaSpeechRequest) (*schemas.RakshaSpeechResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechRequest, provider.GetProviderKey())
}

// SpeechStream is not supported by the Runware provider.
func (provider *RunwareProvider) SpeechStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaSpeechRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechStreamRequest, provider.GetProviderKey())
}

// Transcription is not supported by the Runware provider.
func (provider *RunwareProvider) Transcription(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaTranscriptionRequest) (*schemas.RakshaTranscriptionResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionRequest, provider.GetProviderKey())
}

// TranscriptionStream is not supported by the Runware provider.
func (provider *RunwareProvider) TranscriptionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaTranscriptionRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionStreamRequest, provider.GetProviderKey())
}

// ImageGeneration performs a text-to-image (or image-to-image) request to Runware's API.
func (provider *RunwareProvider) ImageGeneration(ctx *schemas.RakshaContext, key schemas.Key, rakshaReq *schemas.RakshaImageGenerationRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	jsonData, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		rakshaReq,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToRunwareImageGenerationRequest(rakshaReq)
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	return provider.handleImageInference(ctx, key, rakshaReq.Model, jsonData)
}

// ImageEdit performs an image edit (image-to-image, inpainting, or outpainting) request to Runware's API.
func (provider *RunwareProvider) ImageEdit(ctx *schemas.RakshaContext, key schemas.Key, rakshaReq *schemas.RakshaImageEditRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	jsonData, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		rakshaReq,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToRunwareImageEditRequest(rakshaReq)
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	return provider.handleImageInference(ctx, key, rakshaReq.Model, jsonData)
}

// handleImageInference wraps a single imageInference task in the array Runware expects, posts it
// to the unified endpoint, and converts the synchronous response into a Raksha image response.
func (provider *RunwareProvider) handleImageInference(ctx *schemas.RakshaContext, key schemas.Key, model string, jsonData []byte) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
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

	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.EnrichError(ctx, parseRunwareError(resp), body, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Decode response body
	respBody, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		rawErrBody := append([]byte(nil), resp.Body()...)
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err), body, rawErrBody, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Parse response envelope
	var runwareResp RunwareResponse
	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(respBody, &runwareResp, body, sendBackRawRequest, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Convert to Raksha response
	rakshaResp, rakshaErr := ToRakshaImageGenerationResponse(&runwareResp)
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, body, respBody, sendBackRawRequest, sendBackRawResponse, latency)
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

// Rerank is not supported by the Runware provider.
func (provider *RunwareProvider) Rerank(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaRerankRequest) (*schemas.RakshaRerankResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.RerankRequest, provider.GetProviderKey())
}

// OCR is not supported by the Runware provider.
func (provider *RunwareProvider) OCR(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaOCRRequest) (*schemas.RakshaOCRResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.OCRRequest, provider.GetProviderKey())
}

// ImageGenerationStream is not supported by the Runware provider.
func (provider *RunwareProvider) ImageGenerationStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaImageGenerationRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationStreamRequest, provider.GetProviderKey())
}

// ImageEditStream is not supported by the Runware provider.
func (provider *RunwareProvider) ImageEditStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaImageEditRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditStreamRequest, provider.GetProviderKey())
}

// ImageVariation is not supported by the Runware provider.
func (provider *RunwareProvider) ImageVariation(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageVariationRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageVariationRequest, provider.GetProviderKey())
}

// sendTaskArray wraps a single task object in the Runware array envelope, posts it to the
// unified endpoint, and returns the wrapped request body, decoded response body, and latency.
func (provider *RunwareProvider) sendTaskArray(ctx *schemas.RakshaContext, key schemas.Key, jsonData []byte) (reqBody []byte, respBody []byte, latency time.Duration, rakshaErr *schemas.RakshaError) {
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

	lat, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return reqBody, nil, lat, rakshaErr
	}
	if resp.StatusCode() != fasthttp.StatusOK {
		return reqBody, nil, lat, providerUtils.SetErrorLatency(parseRunwareError(resp), lat)
	}
	decoded, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return reqBody, nil, lat, providerUtils.SetErrorLatency(providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err), lat)
	}
	// Copy out: the fasthttp response buffer is released when this function returns.
	return reqBody, append([]byte(nil), decoded...), lat, nil
}

// VideoGeneration submits an async videoInference task and returns the queued job.
// The caller polls VideoRetrieve to fetch the finished video.
func (provider *RunwareProvider) VideoGeneration(ctx *schemas.RakshaContext, key schemas.Key, rakshaReq *schemas.RakshaVideoGenerationRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	providerName := provider.GetProviderKey()
	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)

	jsonData, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		rakshaReq,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToRunwareVideoGenerationRequest(rakshaReq)
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	reqBody, respBody, latency, rakshaErr := provider.sendTaskArray(ctx, key, jsonData)
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, reqBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	var videoResp RunwareResponse
	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(respBody, &videoResp, reqBody, sendBackRawRequest, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	result, rakshaErr := firstVideoResult(&videoResp)
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, reqBody, respBody, sendBackRawRequest, sendBackRawResponse, latency)
	}

	rakshaResp := ToRakshaVideoGenerationResponse(result)
	rakshaResp.ID = providerUtils.AddVideoIDProviderSuffix(result.TaskUUID, providerName)
	rakshaResp.Model = rakshaReq.Model
	rakshaResp.ExtraFields.Latency = latency.Milliseconds()
	if sendBackRawRequest {
		rakshaResp.ExtraFields.RawRequest = rawRequest
	}
	if sendBackRawResponse {
		rakshaResp.ExtraFields.RawResponse = rawResponse
	}

	return rakshaResp, nil
}

// VideoRetrieve polls a previously submitted videoInference task via a getResponse task.
func (provider *RunwareProvider) VideoRetrieve(ctx *schemas.RakshaContext, key schemas.Key, rakshaReq *schemas.RakshaVideoRetrieveRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	providerName := provider.GetProviderKey()
	taskID := providerUtils.StripVideoIDProviderSuffix(rakshaReq.ID, providerName)
	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)

	jsonData, err := providerUtils.MarshalSorted(RunwareGetResponseRequest{TaskType: taskTypeGetResponse, TaskUUID: taskID})
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderRequestMarshal, err)
	}

	reqBody, respBody, latency, rakshaErr := provider.sendTaskArray(ctx, key, jsonData)
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, reqBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	var videoResp RunwareResponse
	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(respBody, &videoResp, reqBody, sendBackRawRequest, sendBackRawResponse)
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	result, rakshaErr := firstVideoResult(&videoResp)
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, reqBody, respBody, sendBackRawRequest, sendBackRawResponse, latency)
	}

	rakshaResp := ToRakshaVideoGenerationResponse(result)
	rakshaResp.ID = providerUtils.AddVideoIDProviderSuffix(taskID, providerName)
	rakshaResp.ExtraFields.Latency = latency.Milliseconds()
	if sendBackRawRequest {
		rakshaResp.ExtraFields.RawRequest = rawRequest
	}
	if sendBackRawResponse {
		rakshaResp.ExtraFields.RawResponse = rawResponse
	}

	return rakshaResp, nil
}

// VideoDownload retrieves the task, then downloads the finished video from its URL.
func (provider *RunwareProvider) VideoDownload(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaVideoDownloadRequest) (*schemas.RakshaVideoDownloadResponse, *schemas.RakshaError) {
	taskDetails, rakshaErr := provider.VideoRetrieve(ctx, key, &schemas.RakshaVideoRetrieveRequest{Provider: request.Provider, ID: request.ID})
	if rakshaErr != nil {
		return nil, rakshaErr
	}
	if taskDetails.Status != schemas.VideoStatusCompleted {
		return nil, providerUtils.NewRakshaOperationError(fmt.Sprintf("video not ready, current status: %s", taskDetails.Status), nil)
	}
	if len(taskDetails.Videos) == 0 || taskDetails.Videos[0].URL == nil || *taskDetails.Videos[0].URL == "" {
		return nil, providerUtils.NewRakshaOperationError("video URL not available", nil)
	}
	videoURL := *taskDetails.Videos[0].URL

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)
	req.SetRequestURI(videoURL)
	req.Header.SetMethod(http.MethodGet)

	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.SetErrorLatency(providerUtils.NewRakshaOperationError(fmt.Sprintf("failed to download video: HTTP %d", resp.StatusCode()), nil), latency)
	}
	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err)
	}
	contentType := string(resp.Header.ContentType())
	if contentType == "" {
		contentType = "video/mp4"
	}

	rakshaResp := &schemas.RakshaVideoDownloadResponse{
		VideoID:     request.ID,
		Content:     append([]byte(nil), body...),
		ContentType: contentType,
	}
	rakshaResp.ExtraFields.Latency = latency.Milliseconds()

	return rakshaResp, nil
}

// firstVideoResult returns the first video task result, surfacing task-level errors.
func firstVideoResult(resp *RunwareResponse) (*RunwareResult, *schemas.RakshaError) {
	if len(resp.Data) == 0 {
		if msg := firstRunwareErrorMessage(resp.Errors); msg != "" {
			return nil, providerUtils.NewRakshaOperationError(msg, nil)
		}
		return nil, providerUtils.NewRakshaOperationError("runware returned no video task", nil)
	}
	return &resp.Data[0], nil
}

// VideoDelete is not supported by the Runware provider.
func (provider *RunwareProvider) VideoDelete(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoDeleteRequest) (*schemas.RakshaVideoDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDeleteRequest, provider.GetProviderKey())
}

// VideoList is not supported by the Runware provider.
func (provider *RunwareProvider) VideoList(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoListRequest) (*schemas.RakshaVideoListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoListRequest, provider.GetProviderKey())
}

// VideoRemix is not supported by the Runware provider.
func (provider *RunwareProvider) VideoRemix(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoRemixRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRemixRequest, provider.GetProviderKey())
}

// FileUpload is not supported by Runware provider.
func (provider *RunwareProvider) FileUpload(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaFileUploadRequest) (*schemas.RakshaFileUploadResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileUploadRequest, provider.GetProviderKey())
}

// FileList is not supported by Runware provider.
func (provider *RunwareProvider) FileList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileListRequest) (*schemas.RakshaFileListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileListRequest, provider.GetProviderKey())
}

// FileRetrieve is not supported by Runware provider.
func (provider *RunwareProvider) FileRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileRetrieveRequest) (*schemas.RakshaFileRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileRetrieveRequest, provider.GetProviderKey())
}

// FileDelete is not supported by Runware provider.
func (provider *RunwareProvider) FileDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileDeleteRequest) (*schemas.RakshaFileDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileDeleteRequest, provider.GetProviderKey())
}

// FileContent is not supported by Runware provider.
func (provider *RunwareProvider) FileContent(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileContentRequest) (*schemas.RakshaFileContentResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileContentRequest, provider.GetProviderKey())
}

// BatchCreate is not supported by Runware provider.
func (provider *RunwareProvider) BatchCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaBatchCreateRequest) (*schemas.RakshaBatchCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCreateRequest, provider.GetProviderKey())
}

// BatchList is not supported by Runware provider.
func (provider *RunwareProvider) BatchList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchListRequest) (*schemas.RakshaBatchListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchListRequest, provider.GetProviderKey())
}

// BatchRetrieve is not supported by Runware provider.
func (provider *RunwareProvider) BatchRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchRetrieveRequest) (*schemas.RakshaBatchRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchRetrieveRequest, provider.GetProviderKey())
}

// BatchCancel is not supported by Runware provider.
func (provider *RunwareProvider) BatchCancel(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchCancelRequest) (*schemas.RakshaBatchCancelResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCancelRequest, provider.GetProviderKey())
}

// BatchDelete is not supported by Runware provider.
func (provider *RunwareProvider) BatchDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchDeleteRequest) (*schemas.RakshaBatchDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchDeleteRequest, provider.GetProviderKey())
}

// BatchResults is not supported by Runware provider.
func (provider *RunwareProvider) BatchResults(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchResultsRequest) (*schemas.RakshaBatchResultsResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchResultsRequest, provider.GetProviderKey())
}

// CountTokens is not supported by the Runware provider.
func (provider *RunwareProvider) CountTokens(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaResponsesRequest) (*schemas.RakshaCountTokensResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CountTokensRequest, provider.GetProviderKey())
}

// Compaction is not supported by the Runware provider.
func (provider *RunwareProvider) Compaction(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaCompactionRequest) (*schemas.RakshaCompactionResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CompactionRequest, provider.GetProviderKey())
}

// ContainerCreate is not supported by the Runware provider.
func (provider *RunwareProvider) ContainerCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaContainerCreateRequest) (*schemas.RakshaContainerCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerCreateRequest, provider.GetProviderKey())
}

// ContainerList is not supported by the Runware provider.
func (provider *RunwareProvider) ContainerList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerListRequest) (*schemas.RakshaContainerListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerListRequest, provider.GetProviderKey())
}

// ContainerRetrieve is not supported by the Runware provider.
func (provider *RunwareProvider) ContainerRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerRetrieveRequest) (*schemas.RakshaContainerRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerRetrieveRequest, provider.GetProviderKey())
}

// ContainerDelete is not supported by the Runware provider.
func (provider *RunwareProvider) ContainerDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerDeleteRequest) (*schemas.RakshaContainerDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerDeleteRequest, provider.GetProviderKey())
}

// ContainerFileCreate is not supported by the Runware provider.
func (provider *RunwareProvider) ContainerFileCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaContainerFileCreateRequest) (*schemas.RakshaContainerFileCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileCreateRequest, provider.GetProviderKey())
}

// ContainerFileList is not supported by the Runware provider.
func (provider *RunwareProvider) ContainerFileList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileListRequest) (*schemas.RakshaContainerFileListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileListRequest, provider.GetProviderKey())
}

// ContainerFileRetrieve is not supported by the Runware provider.
func (provider *RunwareProvider) ContainerFileRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileRetrieveRequest) (*schemas.RakshaContainerFileRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileRetrieveRequest, provider.GetProviderKey())
}

// ContainerFileContent is not supported by the Runware provider.
func (provider *RunwareProvider) ContainerFileContent(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileContentRequest) (*schemas.RakshaContainerFileContentResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileContentRequest, provider.GetProviderKey())
}

// ContainerFileDelete is not supported by the Runware provider.
func (provider *RunwareProvider) ContainerFileDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileDeleteRequest) (*schemas.RakshaContainerFileDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileDeleteRequest, provider.GetProviderKey())
}

// Passthrough is not supported by the Runware provider.
func (provider *RunwareProvider) Passthrough(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaPassthroughRequest) (*schemas.RakshaPassthroughResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughRequest, provider.GetProviderKey())
}

// PassthroughStream is not supported by the Runware provider.
func (provider *RunwareProvider) PassthroughStream(_ *schemas.RakshaContext, _ schemas.PostHookRunner, _ func(context.Context), _ schemas.Key, _ *schemas.RakshaPassthroughRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughStreamRequest, provider.GetProviderKey())
}
