// Package providers implements various LLM providers and their utility functions.
// This file contains the SGL provider implementation.
package sgl

import (
	"context"
	"strings"
	"time"

	"github.com/unifai/unifai/core/providers/openai"
	providerUtils "github.com/unifai/unifai/core/providers/utils"
	schemas "github.com/unifai/unifai/core/schemas"
	"github.com/valyala/fasthttp"
)

// SGLProvider implements the Provider interface for SGL's API.
type SGLProvider struct {
	logger              schemas.Logger        // Logger for provider operations
	client              *fasthttp.Client      // HTTP client for unary API requests (ReadTimeout bounds overall response)
	streamingClient     *fasthttp.Client      // HTTP client for streaming API requests (no ReadTimeout; idle governed by NewIdleTimeoutReader)
	networkConfig       schemas.NetworkConfig // Network configuration including extra headers
	sendBackRawRequest  bool                  // Whether to include raw request in UnifAIResponse
	sendBackRawResponse bool                  // Whether to include raw response in UnifAIResponse
}

// NewSGLProvider creates a new SGL provider instance.
// It initializes the HTTP client with the provided configuration and sets up response pools.
// The client is configured with timeouts, concurrency limits, and optional proxy settings.
func NewSGLProvider(config *schemas.ProviderConfig, logger schemas.Logger) (*SGLProvider, error) {
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
	// 	sglResponsePool.Put(&schemas.UnifAIResponse{})
	// }

	// Configure proxy and retry policy
	client = providerUtils.ConfigureProxy(client, config.ProxyConfig, logger)
	client = providerUtils.ConfigureDialer(client, config.NetworkConfig.AllowPrivateNetwork)
	client = providerUtils.ConfigureTLS(client, config.NetworkConfig, logger)
	streamingClient := providerUtils.BuildStreamingClient(client)
	config.NetworkConfig.BaseURL = strings.TrimRight(config.NetworkConfig.BaseURL, "/")

	// BaseURL is optional when keys have sgl_key_config with per-key URLs
	return &SGLProvider{
		logger:              logger,
		client:              client,
		streamingClient:     streamingClient,
		networkConfig:       config.NetworkConfig,
		sendBackRawRequest:  config.SendBackRawRequest,
		sendBackRawResponse: config.SendBackRawResponse,
	}, nil
}

// GetProviderKey returns the provider identifier for SGL.
func (provider *SGLProvider) GetProviderKey() schemas.ModelProvider {
	return schemas.SGL
}

// getBaseURL resolves the base URL for a request from the per-key sgl_key_config.
// Each SGL key must have its own URL configured and falls back to provider-level base_url if not configured.
func (provider *SGLProvider) getBaseURL(key schemas.Key) string {
	if key.SGLKeyConfig != nil && key.SGLKeyConfig.URL.GetValue() != "" {
		return strings.TrimRight(key.SGLKeyConfig.URL.GetValue(), "/")
	}
	if provider.networkConfig.BaseURL != "" {
		return strings.TrimRight(provider.networkConfig.BaseURL, "/")
	}
	return ""
}

// baseURLOrError returns the resolved base URL or a UnifAIError when none is configured.
func (provider *SGLProvider) baseURLOrError(key schemas.Key) (string, *schemas.UnifAIError) {
	u := provider.getBaseURL(key)
	if u == "" {
		return "", providerUtils.NewUnifAIOperationError(
			"no base URL configured: either set sgl_key_config.url on the key or set network_config.base_url",
			nil)
	}
	return u, nil
}

// listModelsByKey performs a list models request for a single SGL key,
// resolving the per-key URL so each backend is queried individually.
func (provider *SGLProvider) listModelsByKey(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIListModelsRequest) (*schemas.UnifAIListModelsResponse, *schemas.UnifAIError) {
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

// ListModels performs a list models request to SGL's API.
// Requests are made concurrently per key so that each backend is queried
// with its own URL (from sgl_key_config).
func (provider *SGLProvider) ListModels(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAIListModelsRequest) (*schemas.UnifAIListModelsResponse, *schemas.UnifAIError) {
	return providerUtils.HandleMultipleListModelsRequests(
		ctx,
		keys,
		request,
		provider.listModelsByKey,
	)
}

// TextCompletion performs a text completion request to the SGL API.
func (provider *SGLProvider) TextCompletion(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAITextCompletionRequest) (*schemas.UnifAITextCompletionResponse, *schemas.UnifAIError) {
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
		nil,
		nil,
		provider.logger,
	)
}

// TextCompletionStream performs a streaming text completion request to SGL's API.
// It formats the request, sends it to SGL, and processes the response.
// Returns a channel of UnifAIStreamChunk objects or an error if the request fails.
func (provider *SGLProvider) TextCompletionStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAITextCompletionRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
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
		nil,
		nil,
		provider.logger,
		postHookSpanFinalizer,
	)
}

// ChatCompletion performs a chat completion request to the SGL API.
func (provider *SGLProvider) ChatCompletion(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIChatRequest) (*schemas.UnifAIChatResponse, *schemas.UnifAIError) {
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
		nil,
		nil,
		nil,
		provider.logger,
	)
}

// ChatCompletionStream performs a streaming chat completion request to the SGL API.
// It supports real-time streaming of responses using Server-Sent Events (SSE).
// Uses SGL's OpenAI-compatible streaming format.
// Returns a channel containing UnifAIResponse objects representing the stream or an error if the request fails.
func (provider *SGLProvider) ChatCompletionStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAIChatRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
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
		schemas.SGL,
		postHookRunner,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		provider.logger,
		postHookSpanFinalizer,
	)
}

// Responses performs a responses request to the SGL API.
func (provider *SGLProvider) Responses(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIResponsesRequest) (*schemas.UnifAIResponsesResponse, *schemas.UnifAIError) {
	chatResponse, err := provider.ChatCompletion(ctx, key, request.ToChatRequest())
	if err != nil {
		return nil, err
	}

	response := chatResponse.ToUnifAIResponsesResponse()

	return response, nil
}

// ResponsesStream performs a streaming responses request to the SGL API.
func (provider *SGLProvider) ResponsesStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAIResponsesRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	ctx.SetValue(schemas.UnifAIContextKeyIsResponsesToChatCompletionFallback, true)
	return provider.ChatCompletionStream(
		ctx,
		postHookRunner,
		postHookSpanFinalizer,
		key,
		request.ToChatRequest(),
	)
}

// Embedding performs an embedding request to the SGL API.
func (provider *SGLProvider) Embedding(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIEmbeddingRequest) (*schemas.UnifAIEmbeddingResponse, *schemas.UnifAIError) {
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
		nil,
		provider.logger,
	)
}

// Speech is not supported by the SGL provider.
func (provider *SGLProvider) Speech(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAISpeechRequest) (*schemas.UnifAISpeechResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechRequest, provider.GetProviderKey())
}

// Rerank is not supported by the SGL provider.
func (provider *SGLProvider) Rerank(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIRerankRequest) (*schemas.UnifAIRerankResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.RerankRequest, provider.GetProviderKey())
}

// OCR is not supported by the Sgl provider.
func (provider *SGLProvider) OCR(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIOCRRequest) (*schemas.UnifAIOCRResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.OCRRequest, provider.GetProviderKey())
}

// SpeechStream is not supported by the SGL provider.
func (provider *SGLProvider) SpeechStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAISpeechRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechStreamRequest, provider.GetProviderKey())
}

// Transcription is not supported by the SGL provider.
func (provider *SGLProvider) Transcription(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAITranscriptionRequest) (*schemas.UnifAITranscriptionResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionRequest, provider.GetProviderKey())
}

// TranscriptionStream is not supported by the SGL provider.
func (provider *SGLProvider) TranscriptionStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAITranscriptionRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionStreamRequest, provider.GetProviderKey())
}

// ImageGeneration is not supported by the SGL provider.
func (provider *SGLProvider) ImageGeneration(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIImageGenerationRequest) (*schemas.UnifAIImageGenerationResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationRequest, provider.GetProviderKey())
}

// ImageGenerationStream is not supported by the SGL provider.
func (provider *SGLProvider) ImageGenerationStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAIImageGenerationRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationStreamRequest, provider.GetProviderKey())
}

// ImageEdit is not supported by the SGL provider.
func (provider *SGLProvider) ImageEdit(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIImageEditRequest) (*schemas.UnifAIImageGenerationResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditRequest, provider.GetProviderKey())
}

// ImageEditStream is not supported by the SGL provider.
func (provider *SGLProvider) ImageEditStream(ctx *schemas.UnifAIContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.UnifAIImageEditRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditStreamRequest, provider.GetProviderKey())
}

// ImageVariation is not supported by the SGL provider.
func (provider *SGLProvider) ImageVariation(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAIImageVariationRequest) (*schemas.UnifAIImageGenerationResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageVariationRequest, provider.GetProviderKey())
}

// VideoGeneration is not supported by the SGL provider.
func (provider *SGLProvider) VideoGeneration(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIVideoGenerationRequest) (*schemas.UnifAIVideoGenerationResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoGenerationRequest, provider.GetProviderKey())
}

// VideoRetrieve is not supported by the SGL provider.
func (provider *SGLProvider) VideoRetrieve(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIVideoRetrieveRequest) (*schemas.UnifAIVideoGenerationResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRetrieveRequest, provider.GetProviderKey())
}

// VideoDownload is not supported by the SGL provider.
func (provider *SGLProvider) VideoDownload(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIVideoDownloadRequest) (*schemas.UnifAIVideoDownloadResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDownloadRequest, provider.GetProviderKey())
}

// VideoDelete is not supported by the SGL provider.
func (provider *SGLProvider) VideoDelete(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIVideoDeleteRequest) (*schemas.UnifAIVideoDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDeleteRequest, provider.GetProviderKey())
}

// VideoList is not supported by the SGL provider.
func (provider *SGLProvider) VideoList(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIVideoListRequest) (*schemas.UnifAIVideoListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoListRequest, provider.GetProviderKey())
}

// VideoRemix is not supported by the SGL provider.
func (provider *SGLProvider) VideoRemix(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIVideoRemixRequest) (*schemas.UnifAIVideoGenerationResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRemixRequest, provider.GetProviderKey())
}

// FileUpload is not supported by SGL provider.
func (provider *SGLProvider) FileUpload(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIFileUploadRequest) (*schemas.UnifAIFileUploadResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileUploadRequest, provider.GetProviderKey())
}

// FileList is not supported by SGL provider.
func (provider *SGLProvider) FileList(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIFileListRequest) (*schemas.UnifAIFileListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileListRequest, provider.GetProviderKey())
}

// FileRetrieve is not supported by SGL provider.
func (provider *SGLProvider) FileRetrieve(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIFileRetrieveRequest) (*schemas.UnifAIFileRetrieveResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileRetrieveRequest, provider.GetProviderKey())
}

// FileDelete is not supported by SGL provider.
func (provider *SGLProvider) FileDelete(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIFileDeleteRequest) (*schemas.UnifAIFileDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileDeleteRequest, provider.GetProviderKey())
}

// FileContent is not supported by SGL provider.
func (provider *SGLProvider) FileContent(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIFileContentRequest) (*schemas.UnifAIFileContentResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileContentRequest, provider.GetProviderKey())
}

// BatchCreate is not supported by SGL provider.
func (provider *SGLProvider) BatchCreate(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIBatchCreateRequest) (*schemas.UnifAIBatchCreateResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCreateRequest, provider.GetProviderKey())
}

// BatchList is not supported by SGL provider.
func (provider *SGLProvider) BatchList(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIBatchListRequest) (*schemas.UnifAIBatchListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchListRequest, provider.GetProviderKey())
}

// BatchRetrieve is not supported by SGL provider.
func (provider *SGLProvider) BatchRetrieve(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIBatchRetrieveRequest) (*schemas.UnifAIBatchRetrieveResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchRetrieveRequest, provider.GetProviderKey())
}

// BatchCancel is not supported by SGL provider.
func (provider *SGLProvider) BatchCancel(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIBatchCancelRequest) (*schemas.UnifAIBatchCancelResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCancelRequest, provider.GetProviderKey())
}

// BatchDelete is not supported by SGL provider.
func (provider *SGLProvider) BatchDelete(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIBatchDeleteRequest) (*schemas.UnifAIBatchDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchDeleteRequest, provider.GetProviderKey())
}

// BatchResults is not supported by SGL provider.
func (provider *SGLProvider) BatchResults(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIBatchResultsRequest) (*schemas.UnifAIBatchResultsResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchResultsRequest, provider.GetProviderKey())
}

// CountTokens is not supported by the SGL provider.
func (provider *SGLProvider) CountTokens(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIResponsesRequest) (*schemas.UnifAICountTokensResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CountTokensRequest, provider.GetProviderKey())
}

// Compaction is not supported by the SGL provider.
func (provider *SGLProvider) Compaction(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAICompactionRequest) (*schemas.UnifAICompactionResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CompactionRequest, provider.GetProviderKey())
}

// ContainerCreate is not supported by the SGL provider.
func (provider *SGLProvider) ContainerCreate(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIContainerCreateRequest) (*schemas.UnifAIContainerCreateResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerCreateRequest, provider.GetProviderKey())
}

// ContainerList is not supported by the SGL provider.
func (provider *SGLProvider) ContainerList(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerListRequest) (*schemas.UnifAIContainerListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerListRequest, provider.GetProviderKey())
}

// ContainerRetrieve is not supported by the SGL provider.
func (provider *SGLProvider) ContainerRetrieve(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerRetrieveRequest) (*schemas.UnifAIContainerRetrieveResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerRetrieveRequest, provider.GetProviderKey())
}

// ContainerDelete is not supported by the SGL provider.
func (provider *SGLProvider) ContainerDelete(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerDeleteRequest) (*schemas.UnifAIContainerDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerDeleteRequest, provider.GetProviderKey())
}

// ContainerFileCreate is not supported by the SGL provider.
func (provider *SGLProvider) ContainerFileCreate(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIContainerFileCreateRequest) (*schemas.UnifAIContainerFileCreateResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileCreateRequest, provider.GetProviderKey())
}

// ContainerFileList is not supported by the SGL provider.
func (provider *SGLProvider) ContainerFileList(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerFileListRequest) (*schemas.UnifAIContainerFileListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileListRequest, provider.GetProviderKey())
}

// ContainerFileRetrieve is not supported by the SGL provider.
func (provider *SGLProvider) ContainerFileRetrieve(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerFileRetrieveRequest) (*schemas.UnifAIContainerFileRetrieveResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileRetrieveRequest, provider.GetProviderKey())
}

// ContainerFileContent is not supported by the SGL provider.
func (provider *SGLProvider) ContainerFileContent(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerFileContentRequest) (*schemas.UnifAIContainerFileContentResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileContentRequest, provider.GetProviderKey())
}

// ContainerFileDelete is not supported by the SGL provider.
func (provider *SGLProvider) ContainerFileDelete(_ *schemas.UnifAIContext, _ []schemas.Key, _ *schemas.UnifAIContainerFileDeleteRequest) (*schemas.UnifAIContainerFileDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileDeleteRequest, provider.GetProviderKey())
}

// Passthrough is not supported by the SGL provider.
func (provider *SGLProvider) Passthrough(_ *schemas.UnifAIContext, _ schemas.Key, _ *schemas.UnifAIPassthroughRequest) (*schemas.UnifAIPassthroughResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughRequest, provider.GetProviderKey())
}

func (provider *SGLProvider) PassthroughStream(_ *schemas.UnifAIContext, _ schemas.PostHookRunner, _ func(context.Context), _ schemas.Key, _ *schemas.UnifAIPassthroughRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughStreamRequest, provider.GetProviderKey())
}
