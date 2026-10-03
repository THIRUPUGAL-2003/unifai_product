// Package deepseek implements the DeepSeek LLM provider.
package deepseek

import (
	"context"
	"strings"
	"time"

	"github.com/raksha/raksha/core/providers/openai"
	providerUtils "github.com/raksha/raksha/core/providers/utils"
	schemas "github.com/raksha/raksha/core/schemas"
	"github.com/valyala/fasthttp"
)

// DeepSeekProvider implements the Provider interface for DeepSeek's API.
type DeepSeekProvider struct {
	logger              schemas.Logger        // Logger for provider operations
	client              *fasthttp.Client      // HTTP client for unary API requests (ReadTimeout bounds overall response)
	streamingClient     *fasthttp.Client      // HTTP client for streaming API requests (no ReadTimeout; idle governed by NewIdleTimeoutReader)
	networkConfig       schemas.NetworkConfig // Network configuration including extra headers
	sendBackRawRequest  bool                  // Whether to include raw request in RakshaResponse
	sendBackRawResponse bool                  // Whether to include raw response in RakshaResponse
}

// NewDeepSeekProvider creates a new DeepSeek provider instance.
// It initializes the HTTP client with the provided configuration and sets up response pools.
// The client is configured with timeouts, concurrency limits, and optional proxy settings.
func NewDeepSeekProvider(config *schemas.ProviderConfig, logger schemas.Logger) (*DeepSeekProvider, error) {
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
	// Set default BaseURL if not provided
	if config.NetworkConfig.BaseURL == "" {
		config.NetworkConfig.BaseURL = "https://api.deepseek.com"
	}
	config.NetworkConfig.BaseURL = strings.TrimRight(config.NetworkConfig.BaseURL, "/")

	return &DeepSeekProvider{
		logger:              logger,
		client:              client,
		streamingClient:     streamingClient,
		networkConfig:       config.NetworkConfig,
		sendBackRawRequest:  config.SendBackRawRequest,
		sendBackRawResponse: config.SendBackRawResponse,
	}, nil
}

// GetProviderKey returns the provider identifier for DeepSeek.
func (provider *DeepSeekProvider) GetProviderKey() schemas.ModelProvider {
	return schemas.DeepSeek
}

// ListModels performs a list models request to DeepSeek's API.
func (provider *DeepSeekProvider) ListModels(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaListModelsRequest) (*schemas.RakshaListModelsResponse, *schemas.RakshaError) {
	return openai.HandleOpenAIListModelsRequest(
		ctx,
		provider.client,
		request,
		provider.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/models"),
		keys,
		provider.networkConfig.ExtraHeaders,
		provider.GetProviderKey(),
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
	)
}

// TextCompletion performs a text completion request to DeepSeek's API.
// It formats the request, sends it to DeepSeek, and processes the response.
// Returns a RakshaResponse containing the completion results or an error if the request fails.
func (provider *DeepSeekProvider) TextCompletion(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaTextCompletionRequest) (*schemas.RakshaTextCompletionResponse, *schemas.RakshaError) {
	ctx.SetValue(schemas.RakshaContextKeyPassthroughExtraParams, true)
	return openai.HandleOpenAITextCompletionRequest(
		ctx,
		provider.client,
		provider.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/beta/completions"),
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

// TextCompletionStream performs a streaming text completion request to DeepSeek's API.
// It formats the request, sends it to DeepSeek, and processes the response.
// Returns a channel of RakshaStreamChunk objects or an error if the request fails.
func (provider *DeepSeekProvider) TextCompletionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaTextCompletionRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	ctx.SetValue(schemas.RakshaContextKeyPassthroughExtraParams, true)
	return openai.HandleOpenAITextCompletionStreaming(
		ctx,
		provider.streamingClient,
		provider.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/beta/completions"),
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

// ChatCompletion performs a chat completion request to the DeepSeek API.
func (provider *DeepSeekProvider) ChatCompletion(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaChatRequest) (*schemas.RakshaChatResponse, *schemas.RakshaError) {
	ctx.SetValue(schemas.RakshaContextKeyPassthroughExtraParams, true)
	return openai.HandleOpenAIChatCompletionRequest(
		ctx,
		provider.client,
		provider.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/chat/completions"),
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

// ChatCompletionStream performs a streaming chat completion request to the DeepSeek API.
// It supports real-time streaming of responses using Server-Sent Events (SSE).
// Uses DeepSeek's OpenAI-compatible streaming format.
// Returns a channel containing RakshaStreamChunk objects representing the stream or an error if the request fails.
func (provider *DeepSeekProvider) ChatCompletionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaChatRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	ctx.SetValue(schemas.RakshaContextKeyPassthroughExtraParams, true)
	return openai.HandleOpenAIChatCompletionStreaming(
		ctx,
		provider.streamingClient,
		provider.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/chat/completions"),
		request,
		openai.BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		provider.networkConfig.StreamIdleTimeoutInSeconds,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		schemas.DeepSeek,
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

func (provider *DeepSeekProvider) Responses(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaResponsesRequest) (*schemas.RakshaResponsesResponse, *schemas.RakshaError) {
	chatResponse, err := provider.ChatCompletion(ctx, key, request.ToChatRequest())
	if err != nil {
		return nil, err
	}

	response := chatResponse.ToRakshaResponsesResponse()

	return response, nil
}

// ResponsesStream performs a streaming responses request to the DeepSeek API.
func (provider *DeepSeekProvider) ResponsesStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaResponsesRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	ctx.SetValue(schemas.RakshaContextKeyIsResponsesToChatCompletionFallback, true)
	return provider.ChatCompletionStream(
		ctx,
		postHookRunner,
		postHookSpanFinalizer,
		key,
		request.ToChatRequest(),
	)
}

// Embedding is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) Embedding(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaEmbeddingRequest) (*schemas.RakshaEmbeddingResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.EmbeddingRequest, provider.GetProviderKey())
}

// Speech is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) Speech(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaSpeechRequest) (*schemas.RakshaSpeechResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechRequest, provider.GetProviderKey())
}

// Rerank is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) Rerank(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaRerankRequest) (*schemas.RakshaRerankResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.RerankRequest, provider.GetProviderKey())
}

// OCR is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) OCR(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaOCRRequest) (*schemas.RakshaOCRResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.OCRRequest, provider.GetProviderKey())
}

// SpeechStream is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) SpeechStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaSpeechRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechStreamRequest, provider.GetProviderKey())
}

// Transcription is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) Transcription(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaTranscriptionRequest) (*schemas.RakshaTranscriptionResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionRequest, provider.GetProviderKey())
}

// TranscriptionStream is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) TranscriptionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaTranscriptionRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionStreamRequest, provider.GetProviderKey())
}

// ImageGeneration is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) ImageGeneration(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageGenerationRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationRequest, provider.GetProviderKey())
}

// ImageGenerationStream is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) ImageGenerationStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaImageGenerationRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationStreamRequest, provider.GetProviderKey())
}

// ImageEdit is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) ImageEdit(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageEditRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditRequest, provider.GetProviderKey())
}

// ImageEditStream is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) ImageEditStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaImageEditRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditStreamRequest, provider.GetProviderKey())
}

// ImageVariation is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) ImageVariation(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageVariationRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageVariationRequest, provider.GetProviderKey())
}

// VideoGeneration is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) VideoGeneration(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoGenerationRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoGenerationRequest, provider.GetProviderKey())
}

// VideoRetrieve is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) VideoRetrieve(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoRetrieveRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRetrieveRequest, provider.GetProviderKey())
}

// VideoDownload is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) VideoDownload(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoDownloadRequest) (*schemas.RakshaVideoDownloadResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDownloadRequest, provider.GetProviderKey())
}

// VideoDelete is not supported by DeepSeek provider.
func (provider *DeepSeekProvider) VideoDelete(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoDeleteRequest) (*schemas.RakshaVideoDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDeleteRequest, provider.GetProviderKey())
}

// VideoList is not supported by DeepSeek provider.
func (provider *DeepSeekProvider) VideoList(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoListRequest) (*schemas.RakshaVideoListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoListRequest, provider.GetProviderKey())
}

// VideoRemix is not supported by DeepSeek provider.
func (provider *DeepSeekProvider) VideoRemix(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoRemixRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRemixRequest, provider.GetProviderKey())
}

// FileUpload is not supported by DeepSeek provider.
func (provider *DeepSeekProvider) FileUpload(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaFileUploadRequest) (*schemas.RakshaFileUploadResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileUploadRequest, provider.GetProviderKey())
}

// FileList is not supported by DeepSeek provider.
func (provider *DeepSeekProvider) FileList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileListRequest) (*schemas.RakshaFileListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileListRequest, provider.GetProviderKey())
}

// FileRetrieve is not supported by DeepSeek provider.
func (provider *DeepSeekProvider) FileRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileRetrieveRequest) (*schemas.RakshaFileRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileRetrieveRequest, provider.GetProviderKey())
}

// FileDelete is not supported by DeepSeek provider.
func (provider *DeepSeekProvider) FileDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileDeleteRequest) (*schemas.RakshaFileDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileDeleteRequest, provider.GetProviderKey())
}

// FileContent is not supported by DeepSeek provider.
func (provider *DeepSeekProvider) FileContent(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileContentRequest) (*schemas.RakshaFileContentResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileContentRequest, provider.GetProviderKey())
}

// BatchCreate is not supported by DeepSeek provider.
func (provider *DeepSeekProvider) BatchCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaBatchCreateRequest) (*schemas.RakshaBatchCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCreateRequest, provider.GetProviderKey())
}

// BatchList is not supported by DeepSeek provider.
func (provider *DeepSeekProvider) BatchList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchListRequest) (*schemas.RakshaBatchListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchListRequest, provider.GetProviderKey())
}

// BatchRetrieve is not supported by DeepSeek provider.
func (provider *DeepSeekProvider) BatchRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchRetrieveRequest) (*schemas.RakshaBatchRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchRetrieveRequest, provider.GetProviderKey())
}

// BatchCancel is not supported by DeepSeek provider.
func (provider *DeepSeekProvider) BatchCancel(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchCancelRequest) (*schemas.RakshaBatchCancelResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCancelRequest, provider.GetProviderKey())
}

// BatchDelete is not supported by DeepSeek provider.
func (provider *DeepSeekProvider) BatchDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchDeleteRequest) (*schemas.RakshaBatchDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchDeleteRequest, provider.GetProviderKey())
}

// BatchResults is not supported by DeepSeek provider.
func (provider *DeepSeekProvider) BatchResults(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchResultsRequest) (*schemas.RakshaBatchResultsResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchResultsRequest, provider.GetProviderKey())
}

// CountTokens is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) CountTokens(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaResponsesRequest) (*schemas.RakshaCountTokensResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CountTokensRequest, provider.GetProviderKey())
}

// Compaction is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) Compaction(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaCompactionRequest) (*schemas.RakshaCompactionResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CompactionRequest, provider.GetProviderKey())
}

// ContainerCreate is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) ContainerCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaContainerCreateRequest) (*schemas.RakshaContainerCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerCreateRequest, provider.GetProviderKey())
}

// ContainerList is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) ContainerList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerListRequest) (*schemas.RakshaContainerListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerListRequest, provider.GetProviderKey())
}

// ContainerRetrieve is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) ContainerRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerRetrieveRequest) (*schemas.RakshaContainerRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerRetrieveRequest, provider.GetProviderKey())
}

// ContainerDelete is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) ContainerDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerDeleteRequest) (*schemas.RakshaContainerDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerDeleteRequest, provider.GetProviderKey())
}

// ContainerFileCreate is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) ContainerFileCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaContainerFileCreateRequest) (*schemas.RakshaContainerFileCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileCreateRequest, provider.GetProviderKey())
}

// ContainerFileList is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) ContainerFileList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileListRequest) (*schemas.RakshaContainerFileListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileListRequest, provider.GetProviderKey())
}

// ContainerFileRetrieve is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) ContainerFileRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileRetrieveRequest) (*schemas.RakshaContainerFileRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileRetrieveRequest, provider.GetProviderKey())
}

// ContainerFileContent is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) ContainerFileContent(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileContentRequest) (*schemas.RakshaContainerFileContentResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileContentRequest, provider.GetProviderKey())
}

// ContainerFileDelete is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) ContainerFileDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileDeleteRequest) (*schemas.RakshaContainerFileDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileDeleteRequest, provider.GetProviderKey())
}

// Passthrough is not supported by the DeepSeek provider.
func (provider *DeepSeekProvider) Passthrough(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaPassthroughRequest) (*schemas.RakshaPassthroughResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughRequest, provider.GetProviderKey())
}

func (provider *DeepSeekProvider) PassthroughStream(_ *schemas.RakshaContext, _ schemas.PostHookRunner, _ func(context.Context), _ schemas.Key, _ *schemas.RakshaPassthroughRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughStreamRequest, provider.GetProviderKey())
}
