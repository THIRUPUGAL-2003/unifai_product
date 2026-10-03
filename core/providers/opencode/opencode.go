// Package opencode implements the Opencode Zen and Go AI gateway providers.
// Both gateways expose an OpenAI-compatible API and share the same implementation,
// differing only in default base URL and provider key.
package opencode

import (
	"context"
	"strings"
	"time"

	"github.com/raksha/raksha/core/providers/openai"
	providerUtils "github.com/raksha/raksha/core/providers/utils"
	schemas "github.com/raksha/raksha/core/schemas"
	"github.com/valyala/fasthttp"
)

// opencodeProvider implements the Provider interface for Opencode Zen and Go gateways.
type opencodeProvider struct {
	providerKey         schemas.ModelProvider
	logger              schemas.Logger
	client              *fasthttp.Client
	streamingClient     *fasthttp.Client
	networkConfig       schemas.NetworkConfig
	sendBackRawRequest  bool
	sendBackRawResponse bool
}

// NewOpencodeZenProvider creates a new Opencode Zen provider instance.
// Zen is the pay-as-you-go gateway at https://opencode.ai/zen/v1.
func NewOpencodeZenProvider(config *schemas.ProviderConfig, logger schemas.Logger) (*opencodeProvider, error) {
	return newOpencodeProvider(config, schemas.OpencodeZen, "https://opencode.ai/zen", logger)
}

// NewOpencodeGoProvider creates a new Opencode Go provider instance.
// Go is the subscription-based gateway at https://opencode.ai/zen/go/v1.
func NewOpencodeGoProvider(config *schemas.ProviderConfig, logger schemas.Logger) (*opencodeProvider, error) {
	return newOpencodeProvider(config, schemas.OpencodeGo, "https://opencode.ai/zen/go", logger)
}

// newOpencodeProvider initializes the shared provider infrastructure.
func newOpencodeProvider(
	config *schemas.ProviderConfig,
	providerKey schemas.ModelProvider,
	defaultBaseURL string,
	logger schemas.Logger,
) (*opencodeProvider, error) {
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

	client = providerUtils.ConfigureProxy(client, config.ProxyConfig, logger)
	client = providerUtils.ConfigureDialer(client, config.NetworkConfig.AllowPrivateNetwork)
	client = providerUtils.ConfigureTLS(client, config.NetworkConfig, logger)
	streamingClient := providerUtils.BuildStreamingClient(client)

	if config.NetworkConfig.BaseURL == "" {
		config.NetworkConfig.BaseURL = defaultBaseURL
	}
	config.NetworkConfig.BaseURL = strings.TrimRight(config.NetworkConfig.BaseURL, "/")

	return &opencodeProvider{
		providerKey:         providerKey,
		logger:              logger,
		client:              client,
		streamingClient:     streamingClient,
		networkConfig:       config.NetworkConfig,
		sendBackRawRequest:  config.SendBackRawRequest,
		sendBackRawResponse: config.SendBackRawResponse,
	}, nil
}

// GetProviderKey returns the provider identifier stored at construction time.
func (p *opencodeProvider) GetProviderKey() schemas.ModelProvider {
	return p.providerKey
}

// ListModels performs a list models request to the Opencode API.
func (p *opencodeProvider) ListModels(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaListModelsRequest) (*schemas.RakshaListModelsResponse, *schemas.RakshaError) {
	return openai.HandleOpenAIListModelsRequest(
		ctx,
		p.client,
		request,
		p.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/v1/models"),
		keys,
		p.networkConfig.ExtraHeaders,
		p.providerKey,
		providerUtils.ShouldSendBackRawRequest(ctx, p.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, p.sendBackRawResponse),
	)
}

// TextCompletion is not supported by Opencode.
func (p *opencodeProvider) TextCompletion(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaTextCompletionRequest) (*schemas.RakshaTextCompletionResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionRequest, p.GetProviderKey())
}

// TextCompletionStream is not supported by Opencode.
func (p *opencodeProvider) TextCompletionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaTextCompletionRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionStreamRequest, p.GetProviderKey())
}

// ChatCompletion performs a chat completion request to the Opencode API.
func (p *opencodeProvider) ChatCompletion(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaChatRequest) (*schemas.RakshaChatResponse, *schemas.RakshaError) {
	return openai.HandleOpenAIChatCompletionRequest(
		ctx,
		p.client,
		p.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/v1/chat/completions"),
		request,
		openai.BearerAuthHeader(key),
		p.networkConfig.ExtraHeaders,
		providerUtils.ShouldSendBackRawRequest(ctx, p.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, p.sendBackRawResponse),
		p.GetProviderKey(),
		nil,
		parseOpencodeError,
		nil,
		p.logger,
	)
}

// ChatCompletionStream performs a streaming chat completion request to the Opencode API.
func (p *opencodeProvider) ChatCompletionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaChatRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return openai.HandleOpenAIChatCompletionStreaming(
		ctx,
		p.streamingClient,
		p.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/v1/chat/completions"),
		request,
		openai.BearerAuthHeader(key),
		p.networkConfig.ExtraHeaders,
		p.networkConfig.StreamIdleTimeoutInSeconds,
		providerUtils.ShouldSendBackRawRequest(ctx, p.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, p.sendBackRawResponse),
		p.providerKey,
		postHookRunner,
		nil,
		nil,
		parseOpencodeError,
		nil,
		nil,
		nil,
		p.logger,
		postHookSpanFinalizer,
	)
}

// Responses performs a responses request to the Opencode API.
func (p *opencodeProvider) Responses(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaResponsesRequest) (*schemas.RakshaResponsesResponse, *schemas.RakshaError) {
	chatResponse, err := p.ChatCompletion(ctx, key, request.ToChatRequest())
	if err != nil {
		return nil, err
	}
	return chatResponse.ToRakshaResponsesResponse(), nil
}

// ResponsesStream performs a streaming responses request to the Opencode API.
func (p *opencodeProvider) ResponsesStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaResponsesRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	ctx.SetValue(schemas.RakshaContextKeyIsResponsesToChatCompletionFallback, true)
	return p.ChatCompletionStream(ctx, postHookRunner, postHookSpanFinalizer, key, request.ToChatRequest())
}

// Embedding is not supported by Opencode.
func (p *opencodeProvider) Embedding(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaEmbeddingRequest) (*schemas.RakshaEmbeddingResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.EmbeddingRequest, p.GetProviderKey())
}

// Rerank is not supported by Opencode.
func (p *opencodeProvider) Rerank(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaRerankRequest) (*schemas.RakshaRerankResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.RerankRequest, p.GetProviderKey())
}

// OCR is not supported by Opencode.
func (p *opencodeProvider) OCR(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaOCRRequest) (*schemas.RakshaOCRResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.OCRRequest, p.GetProviderKey())
}

// Speech is not supported by Opencode.
func (p *opencodeProvider) Speech(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaSpeechRequest) (*schemas.RakshaSpeechResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechRequest, p.GetProviderKey())
}

// SpeechStream is not supported by Opencode.
func (p *opencodeProvider) SpeechStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaSpeechRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechStreamRequest, p.GetProviderKey())
}

// Transcription is not supported by Opencode.
func (p *opencodeProvider) Transcription(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaTranscriptionRequest) (*schemas.RakshaTranscriptionResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionRequest, p.GetProviderKey())
}

// TranscriptionStream is not supported by Opencode.
func (p *opencodeProvider) TranscriptionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaTranscriptionRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionStreamRequest, p.GetProviderKey())
}

// ImageGeneration is not supported by Opencode.
func (p *opencodeProvider) ImageGeneration(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageGenerationRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationRequest, p.GetProviderKey())
}

// ImageGenerationStream is not supported by Opencode.
func (p *opencodeProvider) ImageGenerationStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaImageGenerationRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationStreamRequest, p.GetProviderKey())
}

// ImageEdit is not supported by Opencode.
func (p *opencodeProvider) ImageEdit(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageEditRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditRequest, p.GetProviderKey())
}

// ImageEditStream is not supported by Opencode.
func (p *opencodeProvider) ImageEditStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaImageEditRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditStreamRequest, p.GetProviderKey())
}

// ImageVariation is not supported by Opencode.
func (p *opencodeProvider) ImageVariation(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageVariationRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageVariationRequest, p.GetProviderKey())
}

// VideoGeneration is not supported by Opencode.
func (p *opencodeProvider) VideoGeneration(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoGenerationRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoGenerationRequest, p.GetProviderKey())
}

// VideoRetrieve is not supported by Opencode.
func (p *opencodeProvider) VideoRetrieve(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoRetrieveRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRetrieveRequest, p.GetProviderKey())
}

// VideoDownload is not supported by Opencode.
func (p *opencodeProvider) VideoDownload(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoDownloadRequest) (*schemas.RakshaVideoDownloadResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDownloadRequest, p.GetProviderKey())
}

// VideoDelete is not supported by Opencode.
func (p *opencodeProvider) VideoDelete(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoDeleteRequest) (*schemas.RakshaVideoDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDeleteRequest, p.GetProviderKey())
}

// VideoList is not supported by Opencode.
func (p *opencodeProvider) VideoList(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoListRequest) (*schemas.RakshaVideoListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoListRequest, p.GetProviderKey())
}

// VideoRemix is not supported by Opencode.
func (p *opencodeProvider) VideoRemix(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoRemixRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRemixRequest, p.GetProviderKey())
}

// BatchCreate is not supported by Opencode.
func (p *opencodeProvider) BatchCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaBatchCreateRequest) (*schemas.RakshaBatchCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCreateRequest, p.GetProviderKey())
}

// BatchList is not supported by Opencode.
func (p *opencodeProvider) BatchList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchListRequest) (*schemas.RakshaBatchListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchListRequest, p.GetProviderKey())
}

// BatchRetrieve is not supported by Opencode.
func (p *opencodeProvider) BatchRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchRetrieveRequest) (*schemas.RakshaBatchRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchRetrieveRequest, p.GetProviderKey())
}

// BatchCancel is not supported by Opencode.
func (p *opencodeProvider) BatchCancel(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchCancelRequest) (*schemas.RakshaBatchCancelResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCancelRequest, p.GetProviderKey())
}

// BatchDelete is not supported by Opencode.
func (p *opencodeProvider) BatchDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchDeleteRequest) (*schemas.RakshaBatchDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchDeleteRequest, p.GetProviderKey())
}

// BatchResults is not supported by Opencode.
func (p *opencodeProvider) BatchResults(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchResultsRequest) (*schemas.RakshaBatchResultsResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchResultsRequest, p.GetProviderKey())
}

// FileUpload is not supported by Opencode.
func (p *opencodeProvider) FileUpload(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaFileUploadRequest) (*schemas.RakshaFileUploadResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileUploadRequest, p.GetProviderKey())
}

// FileList is not supported by Opencode.
func (p *opencodeProvider) FileList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileListRequest) (*schemas.RakshaFileListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileListRequest, p.GetProviderKey())
}

// FileRetrieve is not supported by Opencode.
func (p *opencodeProvider) FileRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileRetrieveRequest) (*schemas.RakshaFileRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileRetrieveRequest, p.GetProviderKey())
}

// FileDelete is not supported by Opencode.
func (p *opencodeProvider) FileDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileDeleteRequest) (*schemas.RakshaFileDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileDeleteRequest, p.GetProviderKey())
}

// FileContent is not supported by Opencode.
func (p *opencodeProvider) FileContent(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileContentRequest) (*schemas.RakshaFileContentResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileContentRequest, p.GetProviderKey())
}

// CountTokens is not supported by Opencode.
func (p *opencodeProvider) CountTokens(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaResponsesRequest) (*schemas.RakshaCountTokensResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CountTokensRequest, p.GetProviderKey())
}

// Compaction is not supported by Opencode.
func (p *opencodeProvider) Compaction(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaCompactionRequest) (*schemas.RakshaCompactionResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CompactionRequest, p.GetProviderKey())
}

// ContainerCreate is not supported by Opencode.
func (p *opencodeProvider) ContainerCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaContainerCreateRequest) (*schemas.RakshaContainerCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerCreateRequest, p.GetProviderKey())
}

// ContainerList is not supported by Opencode.
func (p *opencodeProvider) ContainerList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerListRequest) (*schemas.RakshaContainerListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerListRequest, p.GetProviderKey())
}

// ContainerRetrieve is not supported by Opencode.
func (p *opencodeProvider) ContainerRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerRetrieveRequest) (*schemas.RakshaContainerRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerRetrieveRequest, p.GetProviderKey())
}

// ContainerDelete is not supported by Opencode.
func (p *opencodeProvider) ContainerDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerDeleteRequest) (*schemas.RakshaContainerDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerDeleteRequest, p.GetProviderKey())
}

// ContainerFileCreate is not supported by Opencode.
func (p *opencodeProvider) ContainerFileCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaContainerFileCreateRequest) (*schemas.RakshaContainerFileCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileCreateRequest, p.GetProviderKey())
}

// ContainerFileList is not supported by Opencode.
func (p *opencodeProvider) ContainerFileList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileListRequest) (*schemas.RakshaContainerFileListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileListRequest, p.GetProviderKey())
}

// ContainerFileRetrieve is not supported by Opencode.
func (p *opencodeProvider) ContainerFileRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileRetrieveRequest) (*schemas.RakshaContainerFileRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileRetrieveRequest, p.GetProviderKey())
}

// ContainerFileContent is not supported by Opencode.
func (p *opencodeProvider) ContainerFileContent(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileContentRequest) (*schemas.RakshaContainerFileContentResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileContentRequest, p.GetProviderKey())
}

// ContainerFileDelete is not supported by Opencode.
func (p *opencodeProvider) ContainerFileDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileDeleteRequest) (*schemas.RakshaContainerFileDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileDeleteRequest, p.GetProviderKey())
}

// Passthrough is not supported by Opencode.
func (p *opencodeProvider) Passthrough(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaPassthroughRequest) (*schemas.RakshaPassthroughResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughRequest, p.GetProviderKey())
}

// PassthroughStream is not supported by Opencode.
func (p *opencodeProvider) PassthroughStream(_ *schemas.RakshaContext, _ schemas.PostHookRunner, _ func(context.Context), _ schemas.Key, _ *schemas.RakshaPassthroughRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughStreamRequest, p.GetProviderKey())
}
