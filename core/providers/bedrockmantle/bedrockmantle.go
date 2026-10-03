// Package bedrockmantle implements the Bedrock Mantle LLM provider. It owns the Bedrock Mantle
// surface served on the bedrock-mantle.{region}.api.aws host: Claude models via the native
// Anthropic Messages API (/anthropic/v1/messages), and OpenAI-family (gpt-*) and Gemma models
// via the OpenAI-compatible API (/v1 or /openai/v1). The model id is sent verbatim, save for an
// optional leading "region/" addressing prefix. Authentication is either a Bedrock Mantle API key
// (Authorization: Bearer) or AWS SigV4 for the bedrock-mantle service.
package bedrockmantle

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/raksha/raksha/core/providers/anthropic"
	"github.com/raksha/raksha/core/providers/bedrock"
	openai "github.com/raksha/raksha/core/providers/openai"
	providerUtils "github.com/raksha/raksha/core/providers/utils"
	schemas "github.com/raksha/raksha/core/schemas"
	"github.com/valyala/fasthttp"
)

// BedrockMantleProvider implements the Provider interface for the Bedrock Mantle endpoint.
type BedrockMantleProvider struct {
	logger                schemas.Logger        // Logger for provider operations
	mantleClient          *fasthttp.Client      // fasthttp client for unary requests (OpenAI-compatible and native-Anthropic paths)
	mantleStreamingClient *fasthttp.Client      // fasthttp streaming client for streaming requests
	networkConfig         schemas.NetworkConfig // Network configuration including extra headers
	sendBackRawRequest    bool                  // Whether to include raw request in RakshaResponse
	sendBackRawResponse   bool                  // Whether to include raw response in RakshaResponse
}

// NewBedrockMantleProvider creates a new Bedrock Mantle provider instance.
// It initializes the fasthttp unary and streaming clients with the provided configuration.
// There is no default BaseURL: mantle requests target computed bedrock-mantle.{region}.api.aws hosts.
func NewBedrockMantleProvider(config *schemas.ProviderConfig, logger schemas.Logger) (*BedrockMantleProvider, error) {
	config.CheckAndSetDefaults()

	requestTimeout := time.Second * time.Duration(config.NetworkConfig.DefaultRequestTimeoutInSeconds)

	// fasthttp clients for Bedrock Mantle (shared by OpenAI-compatible and native-Anthropic paths).
	// ReadTimeout is the shared provider request timeout; oversized Anthropic responses are handled
	// by PrepareResponseStreaming, not by these static settings.
	mantleFasthttpClient := &fasthttp.Client{
		ReadTimeout:         requestTimeout,
		WriteTimeout:        requestTimeout,
		MaxConnsPerHost:     config.NetworkConfig.MaxConnsPerHost,
		MaxIdleConnDuration: 30 * time.Second,
		MaxConnWaitTimeout:  requestTimeout,
		MaxConnDuration:     time.Second * time.Duration(schemas.DefaultMaxConnDurationInSeconds),
		ConnPoolStrategy:    fasthttp.FIFO,
	}
	mantleFasthttpClient = providerUtils.ConfigureProxy(mantleFasthttpClient, config.ProxyConfig, logger)
	mantleFasthttpClient = providerUtils.ConfigureDialer(mantleFasthttpClient, config.NetworkConfig.AllowPrivateNetwork)
	mantleFasthttpClient = providerUtils.ConfigureTLS(mantleFasthttpClient, config.NetworkConfig, logger)
	mantleStreamingFasthttpClient := providerUtils.BuildStreamingClient(mantleFasthttpClient)

	return &BedrockMantleProvider{
		logger:                logger,
		mantleClient:          mantleFasthttpClient,
		mantleStreamingClient: mantleStreamingFasthttpClient,
		networkConfig:         config.NetworkConfig,
		sendBackRawRequest:    config.SendBackRawRequest,
		sendBackRawResponse:   config.SendBackRawResponse,
	}, nil
}

// mantleAnthropicVersion is the Anthropic API version sent as an HTTP header on the
// Bedrock Mantle native-Anthropic endpoint (unlike bedrock-runtime, which carries the
// version as an "anthropic_version" body field).
const mantleAnthropicVersion = "2023-06-01"

// defaultMantleRegion is the fallback AWS region used to build the bedrock-mantle host when
// no region is supplied by the model prefix, the resolved alias, or the key config.
const defaultMantleRegion = "us-east-1"

// mantleOpenAIURL builds the Bedrock Mantle OpenAI-compatible endpoint URL for the given
// region, model, and API path (e.g. "chat/completions", "responses"). The native-Anthropic
// path is built separately by mantleAnthropicURL. Pass the canonical (capability-resolved)
// model for correct path gating; the request body still carries the wire request.Model.
// Frontier families (closed gpt-5.x, Gemma 4) live under the "openai/v1" base path; gpt-oss
// uses the bare "v1" path.
func mantleOpenAIURL(region, model, path string) string {
	base := "v1"
	if strings.Contains(model, "gpt-5") || strings.Contains(model, "gemma-4") {
		base = "openai/v1"
	}
	return fmt.Sprintf("https://bedrock-mantle.%s.api.aws/%s/%s", region, base, path)
}

// mantleAnthropicURL builds the Bedrock Mantle native-Anthropic Messages endpoint URL.
func mantleAnthropicURL(region string) string {
	return fmt.Sprintf("https://bedrock-mantle.%s.api.aws/anthropic/v1/messages", region)
}

// mantleSigner returns a BodySigner that SigV4-signs the request body for the bedrock-mantle
// service, or nil when a Bedrock Mantle API key is present (auth then flows through the
// Authorization: Bearer header instead). The handler invokes the signer on the exact body it
// builds, so the signature always covers what is actually sent.
func (provider *BedrockMantleProvider) mantleSigner(ctx *schemas.RakshaContext, key schemas.Key, url, accept, region string) providerUtils.BodySigner {
	if key.Value.GetValue() != "" {
		return nil
	}
	return func(body []byte) (map[string]string, *schemas.RakshaError) {
		return bedrock.SignMantleV4Headers(ctx, body, url, accept, key, region, provider.networkConfig.ExtraHeaders)
	}
}

// GetProviderKey returns the provider identifier for Bedrock Mantle.
func (provider *BedrockMantleProvider) GetProviderKey() schemas.ModelProvider {
	return schemas.BedrockMantle
}

// listModelsByKey lists models from the Bedrock Mantle (OpenAI-compatible) /v1/models
// endpoint for a single key, converted to a Raksha response with the key's allow/blacklist/
// alias gating. The request is signed as it is sent (the GET cannot reuse the POST signer);
// a Bedrock Mantle API key authenticates via Authorization: Bearer, otherwise the request is
// SigV4-signed for the bedrock-mantle service and the signed headers are merged into the
// per-request extra headers consumed by the shared OpenAI list-models path.
func (provider *BedrockMantleProvider) listModelsByKey(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaListModelsRequest) (*schemas.RakshaListModelsResponse, *schemas.RakshaError) {
	region := provider.resolveRegion(ctx, key, "")
	mURL := mantleOpenAIURL(region, "", "models")

	extraHeaders := provider.networkConfig.ExtraHeaders
	if key.Value.GetValue() == "" {
		// SigV4: sign the GET and overlay the signed headers; OpenAI's ListModelsByKey only sets
		// a Bearer header when the key carries a value, so the SigV4 Authorization wins here.
		sigHeaders, rakshaErr := bedrock.SignMantleV4Headers(ctx, nil, mURL, "", key, region, provider.networkConfig.ExtraHeaders)
		if rakshaErr != nil {
			return nil, rakshaErr
		}
		merged := make(map[string]string, len(provider.networkConfig.ExtraHeaders)+len(sigHeaders))
		maps.Copy(merged, provider.networkConfig.ExtraHeaders)
		maps.Copy(merged, sigHeaders)
		extraHeaders = merged
	}

	return openai.ListModelsByKey(
		ctx,
		provider.mantleClient,
		mURL,
		key,
		request.Unfiltered,
		extraHeaders,
		provider.GetProviderKey(),
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
	)
}

// ListModels lists models from the Bedrock Mantle OpenAI-compatible /v1/models endpoint,
// aggregating across the supplied keys.
func (provider *BedrockMantleProvider) ListModels(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaListModelsRequest) (*schemas.RakshaListModelsResponse, *schemas.RakshaError) {
	return providerUtils.HandleMultipleListModelsRequests(
		ctx,
		keys,
		request,
		provider.listModelsByKey,
	)
}

// ChatCompletion performs a chat completion request to the Bedrock Mantle endpoint, dispatching
// by model family: Anthropic-family (Claude) models use the native Anthropic Messages surface;
// all other (OpenAI-family / Gemma) models use the OpenAI-compatible surface.
func (provider *BedrockMantleProvider) ChatCompletion(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaChatRequest) (*schemas.RakshaChatResponse, *schemas.RakshaError) {
	region := provider.resolveRegion(ctx, key, request.Model)

	// Anthropic-family models (Claude) use the native Anthropic Messages surface; all other
	// (OpenAI-family / Gemma) models use the OpenAI-compatible surface.
	if schemas.IsAnthropicModelFamily(ctx, request.Model) {
		url := mantleAnthropicURL(region)
		_, bareModel := parseBedrockRegionAndModel(request.Model)
		return anthropic.HandleAnthropicChatCompletionRequest(
			ctx,
			provider.mantleClient,
			url,
			request,
			anthropic.AnthropicRequestBuildConfig{
				Provider:                  schemas.BedrockMantle,
				Model:                     bareModel,
				BetaHeaderOverrides:       provider.networkConfig.BetaHeaderOverrides,
				ShouldSendBackRawRequest:  provider.sendBackRawRequest,
				ShouldSendBackRawResponse: provider.sendBackRawResponse,
			},
			openai.BearerAuthHeader(key),
			addAnthropicHeaders(provider.networkConfig.ExtraHeaders),
			provider.mantleSigner(ctx, key, url, "application/json", region),
			provider.logger,
		)
	}

	url := mantleOpenAIURL(region, schemas.ResolveCanonicalModel(ctx, request.Model), "chat/completions")
	return openai.HandleOpenAIChatCompletionRequest(
		ctx,
		provider.mantleClient,
		url,
		request,
		openai.BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		nil,
		nil,
		provider.mantleSigner(ctx, key, url, "application/json", region),
		provider.logger,
	)
}

// ChatCompletionStream performs a streaming chat completion request to the Bedrock Mantle
// endpoint, dispatching by model family (native Anthropic vs OpenAI-compatible).
func (provider *BedrockMantleProvider) ChatCompletionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaChatRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	region := provider.resolveRegion(ctx, key, request.Model)

	// Anthropic-family models (Claude) use the native Anthropic Messages surface; all other
	// (OpenAI-family / Gemma) models use the OpenAI-compatible surface.
	if schemas.IsAnthropicModelFamily(ctx, request.Model) {
		url := mantleAnthropicURL(region)

		_, bareModel := parseBedrockRegionAndModel(request.Model)
		jsonData, rakshaErr := anthropic.BuildAnthropicChatRequestBody(ctx, request, anthropic.AnthropicRequestBuildConfig{
			Provider:                  schemas.BedrockMantle,
			Model:                     bareModel,
			IsStreaming:               true,
			BetaHeaderOverrides:       provider.networkConfig.BetaHeaderOverrides,
			ShouldSendBackRawRequest:  provider.sendBackRawRequest,
			ShouldSendBackRawResponse: provider.sendBackRawResponse,
		})
		if rakshaErr != nil {
			return nil, rakshaErr
		}

		return anthropic.HandleAnthropicChatCompletionStreaming(
			ctx,
			provider.mantleStreamingClient,
			url,
			jsonData,
			openai.BearerAuthHeader(key),
			addAnthropicHeaders(provider.networkConfig.ExtraHeaders),
			provider.networkConfig.StreamIdleTimeoutInSeconds,
			provider.networkConfig.BetaHeaderOverrides,
			providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
			providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
			provider.GetProviderKey(),
			postHookRunner,
			nil,
			provider.mantleSigner(ctx, key, url, "text/event-stream", region),
			provider.logger,
			postHookSpanFinalizer,
		)
	}

	url := mantleOpenAIURL(region, schemas.ResolveCanonicalModel(ctx, request.Model), "chat/completions")
	return openai.HandleOpenAIChatCompletionStreaming(
		ctx, provider.mantleStreamingClient, url, request,
		openai.BearerAuthHeader(key), provider.networkConfig.ExtraHeaders,
		provider.networkConfig.StreamIdleTimeoutInSeconds,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(), postHookRunner,
		nil, nil, nil, nil, nil,
		provider.mantleSigner(ctx, key, url, "text/event-stream", region),
		provider.logger, postHookSpanFinalizer,
	)
}

// Responses performs a Responses API request to the Bedrock Mantle endpoint, dispatching by
// model family (native Anthropic vs OpenAI-compatible).
func (provider *BedrockMantleProvider) Responses(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaResponsesRequest) (*schemas.RakshaResponsesResponse, *schemas.RakshaError) {
	region := provider.resolveRegion(ctx, key, request.Model)

	// Anthropic-family models (Claude) use the native Anthropic Messages surface; all other
	// (OpenAI-family / Gemma) models use the OpenAI-compatible surface.
	if schemas.IsAnthropicModelFamily(ctx, request.Model) {
		url := mantleAnthropicURL(region)

		_, bareModel := parseBedrockRegionAndModel(request.Model)
		return anthropic.HandleAnthropicResponsesRequest(
			ctx,
			provider.mantleClient,
			url,
			request,
			anthropic.AnthropicRequestBuildConfig{
				Provider:                  schemas.BedrockMantle,
				Model:                     bareModel,
				ValidateTools:             true,
				BetaHeaderOverrides:       provider.networkConfig.BetaHeaderOverrides,
				ShouldSendBackRawRequest:  provider.sendBackRawRequest,
				ShouldSendBackRawResponse: provider.sendBackRawResponse,
			},
			openai.BearerAuthHeader(key),
			addAnthropicHeaders(provider.networkConfig.ExtraHeaders),
			provider.mantleSigner(ctx, key, url, "application/json", region),
			provider.logger,
		)
	}

	url := mantleOpenAIURL(region, schemas.ResolveCanonicalModel(ctx, request.Model), "responses")
	return openai.HandleOpenAIResponsesRequest(
		ctx,
		provider.mantleClient,
		url,
		request,
		openai.BearerAuthHeader(key),
		provider.networkConfig.ExtraHeaders,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		nil, nil,
		provider.mantleSigner(ctx, key, url, "application/json", region),
		provider.logger,
	)
}

// ResponsesStream performs a streaming Responses API request to the Bedrock Mantle endpoint,
// dispatching by model family (native Anthropic vs OpenAI-compatible).
func (provider *BedrockMantleProvider) ResponsesStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaResponsesRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	region := provider.resolveRegion(ctx, key, request.Model)

	// Anthropic-family models (Claude) use the native Anthropic Messages surface; all other
	// (OpenAI-family / Gemma) models use the OpenAI-compatible surface.
	if schemas.IsAnthropicModelFamily(ctx, request.Model) {
		url := mantleAnthropicURL(region)

		_, bareModel := parseBedrockRegionAndModel(request.Model)
		jsonData, rakshaErr := anthropic.BuildAnthropicResponsesRequestBody(ctx, request, anthropic.AnthropicRequestBuildConfig{
			Provider:                  schemas.BedrockMantle,
			Model:                     bareModel,
			IsStreaming:               true,
			ValidateTools:             true,
			BetaHeaderOverrides:       provider.networkConfig.BetaHeaderOverrides,
			ShouldSendBackRawRequest:  provider.sendBackRawRequest,
			ShouldSendBackRawResponse: provider.sendBackRawResponse,
		})
		if rakshaErr != nil {
			return nil, rakshaErr
		}

		return anthropic.HandleAnthropicResponsesStream(
			ctx,
			provider.mantleStreamingClient,
			url,
			jsonData,
			openai.BearerAuthHeader(key),
			addAnthropicHeaders(provider.networkConfig.ExtraHeaders),
			provider.networkConfig.StreamIdleTimeoutInSeconds,
			provider.networkConfig.BetaHeaderOverrides,
			providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
			providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
			provider.GetProviderKey(),
			postHookRunner,
			nil,
			provider.mantleSigner(ctx, key, url, "text/event-stream", region),
			provider.logger,
			postHookSpanFinalizer,
		)
	}

	url := mantleOpenAIURL(region, schemas.ResolveCanonicalModel(ctx, request.Model), "responses")
	return openai.HandleOpenAIResponsesStreaming(
		ctx, provider.mantleStreamingClient, url, request,
		openai.BearerAuthHeader(key), provider.networkConfig.ExtraHeaders,
		provider.networkConfig.StreamIdleTimeoutInSeconds,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(), postHookRunner,
		nil, nil, nil, nil,
		provider.mantleSigner(ctx, key, url, "text/event-stream", region),
		provider.logger, postHookSpanFinalizer,
	)
}

// TextCompletion is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) TextCompletion(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaTextCompletionRequest) (*schemas.RakshaTextCompletionResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionRequest, provider.GetProviderKey())
}

// TextCompletionStream is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) TextCompletionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaTextCompletionRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionStreamRequest, provider.GetProviderKey())
}

// Embedding is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) Embedding(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaEmbeddingRequest) (*schemas.RakshaEmbeddingResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.EmbeddingRequest, provider.GetProviderKey())
}

// Speech is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) Speech(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaSpeechRequest) (*schemas.RakshaSpeechResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechRequest, provider.GetProviderKey())
}

// Rerank is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) Rerank(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaRerankRequest) (*schemas.RakshaRerankResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.RerankRequest, provider.GetProviderKey())
}

// OCR is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) OCR(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaOCRRequest) (*schemas.RakshaOCRResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.OCRRequest, provider.GetProviderKey())
}

// SpeechStream is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) SpeechStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaSpeechRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechStreamRequest, provider.GetProviderKey())
}

// Transcription is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) Transcription(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaTranscriptionRequest) (*schemas.RakshaTranscriptionResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionRequest, provider.GetProviderKey())
}

// TranscriptionStream is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) TranscriptionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaTranscriptionRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionStreamRequest, provider.GetProviderKey())
}

// ImageGeneration is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) ImageGeneration(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageGenerationRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationRequest, provider.GetProviderKey())
}

// ImageGenerationStream is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) ImageGenerationStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaImageGenerationRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationStreamRequest, provider.GetProviderKey())
}

// ImageEdit is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) ImageEdit(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageEditRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditRequest, provider.GetProviderKey())
}

// ImageEditStream is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) ImageEditStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaImageEditRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditStreamRequest, provider.GetProviderKey())
}

// ImageVariation is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) ImageVariation(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageVariationRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageVariationRequest, provider.GetProviderKey())
}

// VideoGeneration is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) VideoGeneration(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoGenerationRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoGenerationRequest, provider.GetProviderKey())
}

// VideoRetrieve is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) VideoRetrieve(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoRetrieveRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRetrieveRequest, provider.GetProviderKey())
}

// VideoDownload is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) VideoDownload(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoDownloadRequest) (*schemas.RakshaVideoDownloadResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDownloadRequest, provider.GetProviderKey())
}

// VideoDelete is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) VideoDelete(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoDeleteRequest) (*schemas.RakshaVideoDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDeleteRequest, provider.GetProviderKey())
}

// VideoList is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) VideoList(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoListRequest) (*schemas.RakshaVideoListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoListRequest, provider.GetProviderKey())
}

// VideoRemix is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) VideoRemix(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoRemixRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRemixRequest, provider.GetProviderKey())
}

// FileUpload is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) FileUpload(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaFileUploadRequest) (*schemas.RakshaFileUploadResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileUploadRequest, provider.GetProviderKey())
}

// FileList is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) FileList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileListRequest) (*schemas.RakshaFileListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileListRequest, provider.GetProviderKey())
}

// FileRetrieve is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) FileRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileRetrieveRequest) (*schemas.RakshaFileRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileRetrieveRequest, provider.GetProviderKey())
}

// FileDelete is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) FileDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileDeleteRequest) (*schemas.RakshaFileDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileDeleteRequest, provider.GetProviderKey())
}

// FileContent is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) FileContent(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaFileContentRequest) (*schemas.RakshaFileContentResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileContentRequest, provider.GetProviderKey())
}

// BatchCreate is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) BatchCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaBatchCreateRequest) (*schemas.RakshaBatchCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCreateRequest, provider.GetProviderKey())
}

// BatchList is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) BatchList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchListRequest) (*schemas.RakshaBatchListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchListRequest, provider.GetProviderKey())
}

// BatchRetrieve is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) BatchRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchRetrieveRequest) (*schemas.RakshaBatchRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchRetrieveRequest, provider.GetProviderKey())
}

// BatchCancel is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) BatchCancel(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchCancelRequest) (*schemas.RakshaBatchCancelResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCancelRequest, provider.GetProviderKey())
}

// BatchDelete is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) BatchDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchDeleteRequest) (*schemas.RakshaBatchDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchDeleteRequest, provider.GetProviderKey())
}

// BatchResults is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) BatchResults(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaBatchResultsRequest) (*schemas.RakshaBatchResultsResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchResultsRequest, provider.GetProviderKey())
}

// CountTokens is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) CountTokens(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaResponsesRequest) (*schemas.RakshaCountTokensResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CountTokensRequest, provider.GetProviderKey())
}

// Compaction is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) Compaction(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaCompactionRequest) (*schemas.RakshaCompactionResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CompactionRequest, provider.GetProviderKey())
}

// CachedContentCreate is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) CachedContentCreate(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaCachedContentCreateRequest) (*schemas.RakshaCachedContentCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentCreateRequest, provider.GetProviderKey())
}

// CachedContentList is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) CachedContentList(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaCachedContentListRequest) (*schemas.RakshaCachedContentListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentListRequest, provider.GetProviderKey())
}

// CachedContentRetrieve is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) CachedContentRetrieve(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaCachedContentRetrieveRequest) (*schemas.RakshaCachedContentRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentRetrieveRequest, provider.GetProviderKey())
}

// CachedContentUpdate is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) CachedContentUpdate(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaCachedContentUpdateRequest) (*schemas.RakshaCachedContentUpdateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentUpdateRequest, provider.GetProviderKey())
}

// CachedContentDelete is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) CachedContentDelete(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaCachedContentDeleteRequest) (*schemas.RakshaCachedContentDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentDeleteRequest, provider.GetProviderKey())
}

// ContainerCreate is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) ContainerCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaContainerCreateRequest) (*schemas.RakshaContainerCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerCreateRequest, provider.GetProviderKey())
}

// ContainerList is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) ContainerList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerListRequest) (*schemas.RakshaContainerListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerListRequest, provider.GetProviderKey())
}

// ContainerRetrieve is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) ContainerRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerRetrieveRequest) (*schemas.RakshaContainerRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerRetrieveRequest, provider.GetProviderKey())
}

// ContainerDelete is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) ContainerDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerDeleteRequest) (*schemas.RakshaContainerDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerDeleteRequest, provider.GetProviderKey())
}

// ContainerFileCreate is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) ContainerFileCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaContainerFileCreateRequest) (*schemas.RakshaContainerFileCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileCreateRequest, provider.GetProviderKey())
}

// ContainerFileList is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) ContainerFileList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileListRequest) (*schemas.RakshaContainerFileListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileListRequest, provider.GetProviderKey())
}

// ContainerFileRetrieve is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) ContainerFileRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileRetrieveRequest) (*schemas.RakshaContainerFileRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileRetrieveRequest, provider.GetProviderKey())
}

// ContainerFileContent is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) ContainerFileContent(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileContentRequest) (*schemas.RakshaContainerFileContentResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileContentRequest, provider.GetProviderKey())
}

// ContainerFileDelete is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) ContainerFileDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileDeleteRequest) (*schemas.RakshaContainerFileDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileDeleteRequest, provider.GetProviderKey())
}

// Passthrough is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) Passthrough(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaPassthroughRequest) (*schemas.RakshaPassthroughResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughRequest, provider.GetProviderKey())
}

// PassthroughStream is not supported by the Bedrock Mantle provider.
func (provider *BedrockMantleProvider) PassthroughStream(_ *schemas.RakshaContext, _ schemas.PostHookRunner, _ func(context.Context), _ schemas.Key, _ *schemas.RakshaPassthroughRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughStreamRequest, provider.GetProviderKey())
}
