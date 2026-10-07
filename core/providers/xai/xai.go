// Package providers implements various LLM providers and their utility functions.
// This file contains the xAI provider implementation.
package xai

import (
	"context"
	"strings"
	"time"

	"github.com/gateway/gateway/core/providers/openai"
	providerUtils "github.com/gateway/gateway/core/providers/utils"
	schemas "github.com/gateway/gateway/core/schemas"
	"github.com/valyala/fasthttp"
)

// xAIProvider implements the Provider interface for xAI's API.
type XAIProvider struct {
	logger              schemas.Logger        // Logger for provider operations
	client              *fasthttp.Client      // HTTP client for unary API requests (ReadTimeout bounds overall response)
	streamingClient     *fasthttp.Client      // HTTP client for streaming API requests (no ReadTimeout; idle governed by NewIdleTimeoutReader)
	networkConfig       schemas.NetworkConfig // Network configuration including extra headers
	sendBackRawRequest  bool                  // Whether to include raw request in GatewayResponse
	sendBackRawResponse bool                  // Whether to include raw response in GatewayResponse
}

// NewXAIProvider creates a new xAI provider instance.
// It initializes the HTTP client with the provided configuration and sets up response pools.
// The client is configured with timeouts, concurrency limits, and optional proxy settings.
func NewXAIProvider(config *schemas.ProviderConfig, logger schemas.Logger) (*XAIProvider, error) {
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

	// Configure proxy and retry policy
	client = providerUtils.ConfigureProxy(client, config.ProxyConfig, logger)
	client = providerUtils.ConfigureDialer(client, config.NetworkConfig.AllowPrivateNetwork)
	client = providerUtils.ConfigureTLS(client, config.NetworkConfig, logger)
	streamingClient := providerUtils.BuildStreamingClient(client)
	config.NetworkConfig.BaseURL = strings.TrimRight(config.NetworkConfig.BaseURL, "/")

	if config.NetworkConfig.BaseURL == "" {
		config.NetworkConfig.BaseURL = "https://api.x.ai"
	}

	return &XAIProvider{
		logger:              logger,
		client:              client,
		streamingClient:     streamingClient,
		networkConfig:       config.NetworkConfig,
		sendBackRawRequest:  config.SendBackRawRequest,
		sendBackRawResponse: config.SendBackRawResponse,
	}, nil
}

// GetProviderKey returns the provider identifier for xAI.
func (provider *XAIProvider) GetProviderKey() schemas.ModelProvider {
	return schemas.XAI
}

// ListModels performs a list models request to xAI's API.
func (provider *XAIProvider) ListModels(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayListModelsRequest) (*schemas.GatewayListModelsResponse, *schemas.GatewayError) {
	if provider.networkConfig.BaseURL == "" {
		return nil, providerUtils.NewConfigurationError("base_url is not set")
	}
	return openai.HandleOpenAIListModelsRequest(
		ctx,
		provider.client,
		request,
		provider.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/v1/models"),
		keys,
		provider.networkConfig.ExtraHeaders,
		provider.GetProviderKey(),
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
	)
}

// TextCompletion performs a text completion request to the xAI API.
func (provider *XAIProvider) TextCompletion(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayTextCompletionRequest) (*schemas.GatewayTextCompletionResponse, *schemas.GatewayError) {
	return openai.HandleOpenAITextCompletionRequest(
		ctx,
		provider.client,
		provider.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/v1/completions"),
		request,
		openai.BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		provider.GetProviderKey(),
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		nil,
		ParseXAIError,
		provider.logger,
	)
}

// TextCompletionStream performs a streaming text completion request to xAI's API.
// It formats the request, sends it to xAI, and processes the response.
// Returns a channel of GatewayStreamChunk objects or an error if the request fails.
func (provider *XAIProvider) TextCompletionStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayTextCompletionRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return openai.HandleOpenAITextCompletionStreaming(
		ctx,
		provider.streamingClient,
		provider.networkConfig.BaseURL+"/v1/completions",
		request,
		nil,
		provider.networkConfig.ExtraHeaders,
		provider.networkConfig.StreamIdleTimeoutInSeconds,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		ParseXAIError,
		postHookRunner,
		nil,
		nil,
		provider.logger,
		postHookSpanFinalizer,
	)
}

// ChatCompletion performs a chat completion request to the xAI API.
func (provider *XAIProvider) ChatCompletion(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayChatRequest) (*schemas.GatewayChatResponse, *schemas.GatewayError) {
	return openai.HandleOpenAIChatCompletionRequest(
		ctx,
		provider.client,
		provider.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/v1/chat/completions"),
		request,
		openai.BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		nil,
		ParseXAIError,
		nil,
		provider.logger,
	)
}

// ChatCompletionStream performs a streaming chat completion request to the xAI API.
// It supports real-time streaming of responses using Server-Sent Events (SSE).
// Uses xAI's OpenAI-compatible streaming format.
// Returns a channel containing GatewayStreamChunk objects representing the stream or an error if the request fails.
func (provider *XAIProvider) ChatCompletionStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayChatRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return openai.HandleOpenAIChatCompletionStreaming(
		ctx,
		provider.streamingClient,
		provider.networkConfig.BaseURL+"/v1/chat/completions",
		request,
		openai.BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		provider.networkConfig.StreamIdleTimeoutInSeconds,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		schemas.XAI,
		postHookRunner,
		nil,
		nil,
		ParseXAIError,
		nil,
		nil,
		nil,
		provider.logger,
		postHookSpanFinalizer,
	)
}

// Responses performs a responses request to the xAI API.
func (provider *XAIProvider) Responses(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayResponsesRequest) (*schemas.GatewayResponsesResponse, *schemas.GatewayError) {
	return openai.HandleOpenAIResponsesRequest(
		ctx,
		provider.client,
		provider.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/v1/responses"),
		request,
		openai.BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		nil,
		ParseXAIError,
		nil,
		provider.logger,
	)
}

// ResponsesStream performs a streaming responses request to the xAI API.
func (provider *XAIProvider) ResponsesStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayResponsesRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return openai.HandleOpenAIResponsesStreaming(
		ctx,
		provider.streamingClient,
		provider.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/v1/responses"),
		request,
		openai.BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		provider.networkConfig.StreamIdleTimeoutInSeconds,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		postHookRunner,
		nil,
		ParseXAIError,
		nil,
		nil,
		nil,
		provider.logger,
		postHookSpanFinalizer,
	)
}

// Embedding is not supported by the xAI provider.
func (provider *XAIProvider) Embedding(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayEmbeddingRequest) (*schemas.GatewayEmbeddingResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.EmbeddingRequest, provider.GetProviderKey())
}

// Speech is not supported by the xAI provider.
func (provider *XAIProvider) Speech(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewaySpeechRequest) (*schemas.GatewaySpeechResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechRequest, provider.GetProviderKey())
}

// Rerank is not supported by the XAI provider.
func (provider *XAIProvider) Rerank(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayRerankRequest) (*schemas.GatewayRerankResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.RerankRequest, provider.GetProviderKey())
}

// OCR is not supported by the Xai provider.
func (provider *XAIProvider) OCR(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayOCRRequest) (*schemas.GatewayOCRResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.OCRRequest, provider.GetProviderKey())
}

// SpeechStream is not supported by the xAI provider.
func (provider *XAIProvider) SpeechStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewaySpeechRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechStreamRequest, provider.GetProviderKey())
}

// Transcription is not supported by the xAI provider.
func (provider *XAIProvider) Transcription(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayTranscriptionRequest) (*schemas.GatewayTranscriptionResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionRequest, provider.GetProviderKey())
}

// TranscriptionStream is not supported by the xAI provider.
func (provider *XAIProvider) TranscriptionStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayTranscriptionRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionStreamRequest, provider.GetProviderKey())
}

// ImageGeneration performs an image generation request to the xAI API.
func (provider *XAIProvider) ImageGeneration(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayImageGenerationRequest) (*schemas.GatewayImageGenerationResponse, *schemas.GatewayError) {
	return openai.HandleOpenAIImageGenerationRequest(
		ctx,
		provider.client,
		provider.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/v1/images/generations"),
		request,
		key,
		provider.networkConfig.ExtraHeaders,
		provider.GetProviderKey(),
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.logger,
	)
}

// ImageGenerationStream is not supported by the xAI provider.
func (provider *XAIProvider) ImageGenerationStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayImageGenerationRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationStreamRequest, provider.GetProviderKey())
}

// ImageEdit is not supported by the xAI provider.
func (provider *XAIProvider) ImageEdit(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayImageEditRequest) (*schemas.GatewayImageGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditRequest, provider.GetProviderKey())
}

// ImageEditStream is not supported by the xAI provider.
func (provider *XAIProvider) ImageEditStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayImageEditRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditStreamRequest, provider.GetProviderKey())
}

// ImageVariation is not supported by the XAI provider.
func (provider *XAIProvider) ImageVariation(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayImageVariationRequest) (*schemas.GatewayImageGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageVariationRequest, provider.GetProviderKey())
}

// VideoGeneration is not supported by the xAI provider.
func (provider *XAIProvider) VideoGeneration(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoGenerationRequest) (*schemas.GatewayVideoGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoGenerationRequest, provider.GetProviderKey())
}

// VideoRetrieve is not supported by the xAI provider.
func (provider *XAIProvider) VideoRetrieve(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoRetrieveRequest) (*schemas.GatewayVideoGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRetrieveRequest, provider.GetProviderKey())
}

// VideoDownload is not supported by the xAI provider.
func (provider *XAIProvider) VideoDownload(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoDownloadRequest) (*schemas.GatewayVideoDownloadResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDownloadRequest, provider.GetProviderKey())
}

// VideoDelete is not supported by the xAI provider.
func (provider *XAIProvider) VideoDelete(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoDeleteRequest) (*schemas.GatewayVideoDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDeleteRequest, provider.GetProviderKey())
}

// VideoList is not supported by the xAI provider.
func (provider *XAIProvider) VideoList(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoListRequest) (*schemas.GatewayVideoListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoListRequest, provider.GetProviderKey())
}

// VideoRemix is not supported by the xAI provider.
func (provider *XAIProvider) VideoRemix(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoRemixRequest) (*schemas.GatewayVideoGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRemixRequest, provider.GetProviderKey())
}

// BatchCreate is not supported by xAI provider.
func (provider *XAIProvider) BatchCreate(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayBatchCreateRequest) (*schemas.GatewayBatchCreateResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCreateRequest, provider.GetProviderKey())
}

// BatchList is not supported by xAI provider.
func (provider *XAIProvider) BatchList(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchListRequest) (*schemas.GatewayBatchListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchListRequest, provider.GetProviderKey())
}

// BatchRetrieve is not supported by xAI provider.
func (provider *XAIProvider) BatchRetrieve(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchRetrieveRequest) (*schemas.GatewayBatchRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchRetrieveRequest, provider.GetProviderKey())
}

// BatchCancel is not supported by xAI provider.
func (provider *XAIProvider) BatchCancel(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchCancelRequest) (*schemas.GatewayBatchCancelResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCancelRequest, provider.GetProviderKey())
}

// BatchDelete is not supported by xAI provider.
func (provider *XAIProvider) BatchDelete(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchDeleteRequest) (*schemas.GatewayBatchDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchDeleteRequest, provider.GetProviderKey())
}

// BatchResults is not supported by xAI provider.
func (provider *XAIProvider) BatchResults(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchResultsRequest) (*schemas.GatewayBatchResultsResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchResultsRequest, provider.GetProviderKey())
}

// FileUpload is not supported by xAI provider.
func (provider *XAIProvider) FileUpload(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayFileUploadRequest) (*schemas.GatewayFileUploadResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileUploadRequest, provider.GetProviderKey())
}

// FileList is not supported by xAI provider.
func (provider *XAIProvider) FileList(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayFileListRequest) (*schemas.GatewayFileListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileListRequest, provider.GetProviderKey())
}

// FileRetrieve is not supported by xAI provider.
func (provider *XAIProvider) FileRetrieve(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayFileRetrieveRequest) (*schemas.GatewayFileRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileRetrieveRequest, provider.GetProviderKey())
}

// FileDelete is not supported by xAI provider.
func (provider *XAIProvider) FileDelete(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayFileDeleteRequest) (*schemas.GatewayFileDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileDeleteRequest, provider.GetProviderKey())
}

// FileContent is not supported by xAI provider.
func (provider *XAIProvider) FileContent(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayFileContentRequest) (*schemas.GatewayFileContentResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileContentRequest, provider.GetProviderKey())
}

func (provider *XAIProvider) CountTokens(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayResponsesRequest) (*schemas.GatewayCountTokensResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CountTokensRequest, provider.GetProviderKey())
}

// Compaction compacts a conversation context window using xAI's /v1/responses/compact endpoint.
func (provider *XAIProvider) Compaction(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayCompactionRequest) (*schemas.GatewayCompactionResponse, *schemas.GatewayError) {
	return openai.HandleOpenAICompactionRequest(
		ctx,
		provider.client,
		provider.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/v1/responses/compact"),
		request,
		openai.BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		provider.logger,
	)
}

// ContainerCreate is not supported by the xAI provider.
func (provider *XAIProvider) ContainerCreate(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayContainerCreateRequest) (*schemas.GatewayContainerCreateResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerCreateRequest, provider.GetProviderKey())
}

// ContainerList is not supported by the xAI provider.
func (provider *XAIProvider) ContainerList(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerListRequest) (*schemas.GatewayContainerListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerListRequest, provider.GetProviderKey())
}

// ContainerRetrieve is not supported by the xAI provider.
func (provider *XAIProvider) ContainerRetrieve(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerRetrieveRequest) (*schemas.GatewayContainerRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerRetrieveRequest, provider.GetProviderKey())
}

// ContainerDelete is not supported by the xAI provider.
func (provider *XAIProvider) ContainerDelete(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerDeleteRequest) (*schemas.GatewayContainerDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerDeleteRequest, provider.GetProviderKey())
}

// ContainerFileCreate is not supported by the xAI provider.
func (provider *XAIProvider) ContainerFileCreate(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayContainerFileCreateRequest) (*schemas.GatewayContainerFileCreateResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileCreateRequest, provider.GetProviderKey())
}

// ContainerFileList is not supported by the xAI provider.
func (provider *XAIProvider) ContainerFileList(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileListRequest) (*schemas.GatewayContainerFileListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileListRequest, provider.GetProviderKey())
}

// ContainerFileRetrieve is not supported by the xAI provider.
func (provider *XAIProvider) ContainerFileRetrieve(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileRetrieveRequest) (*schemas.GatewayContainerFileRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileRetrieveRequest, provider.GetProviderKey())
}

// ContainerFileContent is not supported by the xAI provider.
func (provider *XAIProvider) ContainerFileContent(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileContentRequest) (*schemas.GatewayContainerFileContentResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileContentRequest, provider.GetProviderKey())
}

// ContainerFileDelete is not supported by the xAI provider.
func (provider *XAIProvider) ContainerFileDelete(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileDeleteRequest) (*schemas.GatewayContainerFileDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileDeleteRequest, provider.GetProviderKey())
}

// Passthrough is not supported by the xAI provider.
func (provider *XAIProvider) Passthrough(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayPassthroughRequest) (*schemas.GatewayPassthroughResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughRequest, provider.GetProviderKey())
}

func (provider *XAIProvider) PassthroughStream(_ *schemas.GatewayContext, _ schemas.PostHookRunner, _ func(context.Context), _ schemas.Key, _ *schemas.GatewayPassthroughRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughStreamRequest, provider.GetProviderKey())
}
