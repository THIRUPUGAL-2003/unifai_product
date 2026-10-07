package openaicompat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gateway/gateway/core/providers/openai"
	providerUtils "github.com/gateway/gateway/core/providers/utils"
	schemas "github.com/gateway/gateway/core/schemas"
	"github.com/valyala/fasthttp"
)

// Provider is a thin OpenAI-compatible chat/embeddings provider with a branded key.
type Provider struct {
	spec                Spec
	logger              schemas.Logger
	client              *fasthttp.Client
	streamingClient     *fasthttp.Client
	networkConfig       schemas.NetworkConfig
	sendBackRawRequest  bool
	sendBackRawResponse bool
}

// New creates an OpenAI-compatible provider for the given built-in key.
func New(providerKey schemas.ModelProvider, config *schemas.ProviderConfig, logger schemas.Logger) (*Provider, error) {
	spec, ok := Registry[providerKey]
	if !ok {
		return nil, fmt.Errorf("unknown openai-compat provider: %s", providerKey)
	}
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
		config.NetworkConfig.BaseURL = spec.DefaultBaseURL
	}
	config.NetworkConfig.BaseURL = providerUtils.NormalizeOpenAICompatibleBaseURL(config.NetworkConfig.BaseURL)
	config.NetworkConfig.BaseURL = strings.TrimRight(config.NetworkConfig.BaseURL, "/")

	return &Provider{
		spec:                spec,
		logger:              logger,
		client:              client,
		streamingClient:     streamingClient,
		networkConfig:       config.NetworkConfig,
		sendBackRawRequest:  config.SendBackRawRequest,
		sendBackRawResponse: config.SendBackRawResponse,
	}, nil
}

func (p *Provider) path(resource string) string {
	resource = strings.TrimLeft(resource, "/")
	if p.spec.PathPrefix == "" {
		return "/" + resource
	}
	return "/" + strings.Trim(p.spec.PathPrefix, "/") + "/" + resource
}

func (p *Provider) url(resource string) string {
	return p.networkConfig.BaseURL + p.path(resource)
}

func (p *Provider) GetProviderKey() schemas.ModelProvider {
	return p.spec.Key
}

func (p *Provider) ListModels(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayListModelsRequest) (*schemas.GatewayListModelsResponse, *schemas.GatewayError) {
	return openai.HandleOpenAIListModelsRequest(
		ctx, p.client, request,
		p.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, p.path("models")),
		keys, p.networkConfig.ExtraHeaders, p.GetProviderKey(),
		providerUtils.ShouldSendBackRawRequest(ctx, p.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, p.sendBackRawResponse),
	)
}

func (p *Provider) TextCompletion(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayTextCompletionRequest) (*schemas.GatewayTextCompletionResponse, *schemas.GatewayError) {
	return openai.HandleOpenAITextCompletionRequest(
		ctx, p.client,
		p.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, p.path("completions")),
		request, openai.BearerAuthHeader(key), p.networkConfig.ExtraHeaders, p.GetProviderKey(),
		providerUtils.ShouldSendBackRawRequest(ctx, p.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, p.sendBackRawResponse),
		nil, nil, p.logger,
	)
}

func (p *Provider) TextCompletionStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayTextCompletionRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return openai.HandleOpenAITextCompletionStreaming(
		ctx, p.streamingClient, p.url("completions"), request, openai.BearerAuthHeader(key),
		p.networkConfig.ExtraHeaders, p.networkConfig.StreamIdleTimeoutInSeconds,
		providerUtils.ShouldSendBackRawRequest(ctx, p.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, p.sendBackRawResponse),
		p.GetProviderKey(), nil, postHookRunner, nil, nil, p.logger, postHookSpanFinalizer,
	)
}

func (p *Provider) ChatCompletion(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayChatRequest) (*schemas.GatewayChatResponse, *schemas.GatewayError) {
	return openai.HandleOpenAIChatCompletionRequest(
		ctx, p.client,
		p.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, p.path("chat/completions")),
		request, openai.BearerAuthHeader(key), p.networkConfig.ExtraHeaders,
		providerUtils.ShouldSendBackRawRequest(ctx, p.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, p.sendBackRawResponse),
		p.GetProviderKey(), nil, nil, nil, p.logger,
	)
}

func (p *Provider) ChatCompletionStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayChatRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return openai.HandleOpenAIChatCompletionStreaming(
		ctx, p.streamingClient, p.url("chat/completions"), request, openai.BearerAuthHeader(key),
		p.networkConfig.ExtraHeaders, p.networkConfig.StreamIdleTimeoutInSeconds,
		providerUtils.ShouldSendBackRawRequest(ctx, p.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, p.sendBackRawResponse),
		p.GetProviderKey(), postHookRunner, nil, nil, nil, nil, nil, nil, p.logger, postHookSpanFinalizer,
	)
}

func (p *Provider) Responses(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayResponsesRequest) (*schemas.GatewayResponsesResponse, *schemas.GatewayError) {
	chatResponse, err := p.ChatCompletion(ctx, key, request.ToChatRequest())
	if err != nil {
		return nil, err
	}
	return chatResponse.ToGatewayResponsesResponse(), nil
}

func (p *Provider) ResponsesStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayResponsesRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	ctx.SetValue(schemas.GatewayContextKeyIsResponsesToChatCompletionFallback, true)
	return p.ChatCompletionStream(ctx, postHookRunner, postHookSpanFinalizer, key, request.ToChatRequest())
}

func (p *Provider) Embedding(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayEmbeddingRequest) (*schemas.GatewayEmbeddingResponse, *schemas.GatewayError) {
	if !p.spec.Embedding {
		return nil, providerUtils.NewUnsupportedOperationError(schemas.EmbeddingRequest, p.GetProviderKey())
	}
	return openai.HandleOpenAIEmbeddingRequest(
		ctx, p.client,
		p.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, p.path("embeddings")),
		request, openai.BearerAuthHeader(key), p.networkConfig.ExtraHeaders, p.GetProviderKey(),
		providerUtils.ShouldSendBackRawRequest(ctx, p.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, p.sendBackRawResponse),
		nil, p.logger,
	)
}

func (p *Provider) Speech(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewaySpeechRequest) (*schemas.GatewaySpeechResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechRequest, p.GetProviderKey())
}
func (p *Provider) Rerank(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayRerankRequest) (*schemas.GatewayRerankResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.RerankRequest, p.GetProviderKey())
}
func (p *Provider) OCR(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayOCRRequest) (*schemas.GatewayOCRResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.OCRRequest, p.GetProviderKey())
}
func (p *Provider) SpeechStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewaySpeechRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechStreamRequest, p.GetProviderKey())
}
func (p *Provider) Transcription(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayTranscriptionRequest) (*schemas.GatewayTranscriptionResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionRequest, p.GetProviderKey())
}
func (p *Provider) TranscriptionStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayTranscriptionRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionStreamRequest, p.GetProviderKey())
}
func (p *Provider) ImageGeneration(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayImageGenerationRequest) (*schemas.GatewayImageGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationRequest, p.GetProviderKey())
}
func (p *Provider) ImageGenerationStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayImageGenerationRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationStreamRequest, p.GetProviderKey())
}
func (p *Provider) ImageEdit(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayImageEditRequest) (*schemas.GatewayImageGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditRequest, p.GetProviderKey())
}
func (p *Provider) ImageEditStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayImageEditRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditStreamRequest, p.GetProviderKey())
}
func (p *Provider) ImageVariation(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayImageVariationRequest) (*schemas.GatewayImageGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageVariationRequest, p.GetProviderKey())
}
func (p *Provider) VideoGeneration(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoGenerationRequest) (*schemas.GatewayVideoGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoGenerationRequest, p.GetProviderKey())
}
func (p *Provider) VideoRetrieve(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoRetrieveRequest) (*schemas.GatewayVideoGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRetrieveRequest, p.GetProviderKey())
}
func (p *Provider) VideoDownload(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoDownloadRequest) (*schemas.GatewayVideoDownloadResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDownloadRequest, p.GetProviderKey())
}
func (p *Provider) VideoDelete(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoDeleteRequest) (*schemas.GatewayVideoDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDeleteRequest, p.GetProviderKey())
}
func (p *Provider) VideoList(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoListRequest) (*schemas.GatewayVideoListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoListRequest, p.GetProviderKey())
}
func (p *Provider) VideoRemix(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoRemixRequest) (*schemas.GatewayVideoGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRemixRequest, p.GetProviderKey())
}
func (p *Provider) FileUpload(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayFileUploadRequest) (*schemas.GatewayFileUploadResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileUploadRequest, p.GetProviderKey())
}
func (p *Provider) FileList(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayFileListRequest) (*schemas.GatewayFileListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileListRequest, p.GetProviderKey())
}
func (p *Provider) FileRetrieve(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayFileRetrieveRequest) (*schemas.GatewayFileRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileRetrieveRequest, p.GetProviderKey())
}
func (p *Provider) FileDelete(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayFileDeleteRequest) (*schemas.GatewayFileDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileDeleteRequest, p.GetProviderKey())
}
func (p *Provider) FileContent(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayFileContentRequest) (*schemas.GatewayFileContentResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileContentRequest, p.GetProviderKey())
}
func (p *Provider) BatchCreate(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayBatchCreateRequest) (*schemas.GatewayBatchCreateResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCreateRequest, p.GetProviderKey())
}
func (p *Provider) BatchList(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchListRequest) (*schemas.GatewayBatchListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchListRequest, p.GetProviderKey())
}
func (p *Provider) BatchRetrieve(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchRetrieveRequest) (*schemas.GatewayBatchRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchRetrieveRequest, p.GetProviderKey())
}
func (p *Provider) BatchCancel(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchCancelRequest) (*schemas.GatewayBatchCancelResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCancelRequest, p.GetProviderKey())
}
func (p *Provider) BatchDelete(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchDeleteRequest) (*schemas.GatewayBatchDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchDeleteRequest, p.GetProviderKey())
}
func (p *Provider) BatchResults(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayBatchResultsRequest) (*schemas.GatewayBatchResultsResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchResultsRequest, p.GetProviderKey())
}
func (p *Provider) CountTokens(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayResponsesRequest) (*schemas.GatewayCountTokensResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CountTokensRequest, p.GetProviderKey())
}
func (p *Provider) Compaction(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayCompactionRequest) (*schemas.GatewayCompactionResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CompactionRequest, p.GetProviderKey())
}
func (p *Provider) ContainerCreate(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayContainerCreateRequest) (*schemas.GatewayContainerCreateResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerCreateRequest, p.GetProviderKey())
}
func (p *Provider) ContainerList(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerListRequest) (*schemas.GatewayContainerListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerListRequest, p.GetProviderKey())
}
func (p *Provider) ContainerRetrieve(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerRetrieveRequest) (*schemas.GatewayContainerRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerRetrieveRequest, p.GetProviderKey())
}
func (p *Provider) ContainerDelete(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerDeleteRequest) (*schemas.GatewayContainerDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerDeleteRequest, p.GetProviderKey())
}
func (p *Provider) ContainerFileCreate(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayContainerFileCreateRequest) (*schemas.GatewayContainerFileCreateResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileCreateRequest, p.GetProviderKey())
}
func (p *Provider) ContainerFileList(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileListRequest) (*schemas.GatewayContainerFileListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileListRequest, p.GetProviderKey())
}
func (p *Provider) ContainerFileRetrieve(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileRetrieveRequest) (*schemas.GatewayContainerFileRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileRetrieveRequest, p.GetProviderKey())
}
func (p *Provider) ContainerFileContent(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileContentRequest) (*schemas.GatewayContainerFileContentResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileContentRequest, p.GetProviderKey())
}
func (p *Provider) ContainerFileDelete(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileDeleteRequest) (*schemas.GatewayContainerFileDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileDeleteRequest, p.GetProviderKey())
}
func (p *Provider) Passthrough(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayPassthroughRequest) (*schemas.GatewayPassthroughResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughRequest, p.GetProviderKey())
}
func (p *Provider) PassthroughStream(_ *schemas.GatewayContext, _ schemas.PostHookRunner, _ func(context.Context), _ schemas.Key, _ *schemas.GatewayPassthroughRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughStreamRequest, p.GetProviderKey())
}
