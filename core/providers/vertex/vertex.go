package vertex

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/gateway/gateway/core/providers/anthropic"
	"github.com/gateway/gateway/core/providers/gemini"
	"github.com/gateway/gateway/core/providers/openai"
	providerUtils "github.com/gateway/gateway/core/providers/utils"
	schemas "github.com/gateway/gateway/core/schemas"
	"github.com/tidwall/gjson"
)

type VertexError struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// vertexTokenSourcePool caches oauth2.TokenSource instances keyed by a hash of
// the auth credentials. The Google TokenSource internally handles token refresh
// and expiry, so caching the source avoids re-parsing credentials JSON and
// re-creating the credentials object on every request.
// Entries are evicted by removeVertexClient on 401/403 or token-acquisition errors.
var vertexTokenSourcePool sync.Map

// vertexLocationsPathRe matches /locations/{region} in Vertex API paths for region replacement.
var vertexLocationsPathRe = regexp.MustCompile(`/locations/[^/]+`)

var vertexProjectsPathRe = regexp.MustCompile(`/projects/[^/]+`)

// vertexBodyProjectsRe matches projects/{project} in body JSON values,
// where the path may appear as "projects/X (after a JSON quote) or /projects/X (mid-path).
var vertexBodyProjectsRe = regexp.MustCompile(`(["/])projects/[^/"]+`)

// vertexShortModelRe matches short-form model names like "models/X" in JSON bodies
// that need expanding to the full Vertex resource path.
var vertexShortModelRe = regexp.MustCompile(`"(models/[^/"]+)"`)

// defaultCredentialsCacheKey is the sentinel pool key used when AuthCredentials
// is empty and we fall back to google.FindDefaultCredentials.
const defaultCredentialsCacheKey = "__default_credentials__"

// geminiImageURLSchemes is the image URL scheme allowlist Vertex applies when it
// routes a request through the Gemini converter. Vertex natively accepts gs://
// FileData URIs (in addition to http(s)), so we extend the Gemini-default list
// with "gs".
var geminiImageURLSchemes = []string{"http", "https", "gs"}

// getClientKey generates a unique key for caching token sources.
// It uses a hash of the auth credentials for security.
func getClientKey(authCredentials string) string {
	if authCredentials == "" {
		return defaultCredentialsCacheKey
	}
	hash := sha256.Sum256([]byte(authCredentials))
	return hex.EncodeToString(hash[:])
}

// removeVertexClient evicts a cached token source from the pool.
// This should be called when:
// - API returns authentication/authorization errors (401, 403)
// - Token acquisition fails (tokenSource.Token() error)
// This forces the next request to re-create the token source from scratch.
func removeVertexClient(authCredentials string) {
	clientKey := getClientKey(authCredentials)
	vertexTokenSourcePool.Delete(clientKey)
}

// VertexProvider implements the Provider interface for Google's Vertex AI API.
type VertexProvider struct {
	logger              schemas.Logger        // Logger for provider operations
	client              *fasthttp.Client      // HTTP client for unary API requests (ReadTimeout bounds overall response)
	streamingClient     *fasthttp.Client      // HTTP client for streaming API requests (no ReadTimeout; idle governed by NewIdleTimeoutReader)
	networkConfig       schemas.NetworkConfig // Network configuration including extra headers
	sendBackRawRequest  bool                  // Whether to include raw request in GatewayResponse
	sendBackRawResponse bool                  // Whether to include raw response in GatewayResponse
}

// NewVertexProvider creates a new Vertex provider instance.
// It initializes the HTTP client with the provided configuration and sets up response pools.
// The client is configured with timeouts, concurrency limits, and optional proxy settings.
func NewVertexProvider(config *schemas.ProviderConfig, logger schemas.Logger) (*VertexProvider, error) {
	config.CheckAndSetDefaults()
	requestTimeout := time.Second * time.Duration(config.NetworkConfig.DefaultRequestTimeoutInSeconds)
	client := &fasthttp.Client{
		ReadTimeout:            requestTimeout,
		WriteTimeout:           requestTimeout,
		MaxConnsPerHost:        config.NetworkConfig.MaxConnsPerHost,
		MaxIdleConnDuration:    30 * time.Second,
		MaxConnWaitTimeout:     requestTimeout,
		MaxConnDuration:        time.Second * time.Duration(schemas.DefaultMaxConnDurationInSeconds),
		ConnPoolStrategy:       fasthttp.FIFO,
		DisablePathNormalizing: true,
	}
	client = providerUtils.ConfigureProxy(client, config.ProxyConfig, logger)
	client = providerUtils.ConfigureDialer(client, config.NetworkConfig.AllowPrivateNetwork)
	client = providerUtils.ConfigureTLS(client, config.NetworkConfig, logger)
	streamingClient := providerUtils.BuildStreamingClient(client)
	return &VertexProvider{
		logger:              logger,
		client:              client,
		streamingClient:     streamingClient,
		networkConfig:       config.NetworkConfig,
		sendBackRawRequest:  config.SendBackRawRequest,
		sendBackRawResponse: config.SendBackRawResponse,
	}, nil
}

const cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

// getAuthTokenSource returns an authenticated token source for Vertex AI API requests.
// Token sources are cached in vertexTokenSourcePool keyed by a hash of the auth
// credentials. The Google oauth2.TokenSource handles token refresh and expiry
// internally, so caching the source is safe and avoids re-parsing credentials
// on every request.
func getAuthTokenSource(key schemas.Key) (oauth2.TokenSource, error) {
	authCredentials := key.VertexKeyConfig.AuthCredentials
	clientKey := getClientKey(authCredentials.GetValue())

	// Fast path: return cached token source.
	if cached, ok := vertexTokenSourcePool.Load(clientKey); ok {
		return cached.(oauth2.TokenSource), nil
	}

	// Slow path: create a new token source and cache it.
	var tokenSource oauth2.TokenSource
	if authCredentials.GetValue() == "" {
		creds, err := google.FindDefaultCredentials(context.Background(), cloudPlatformScope)
		if err != nil {
			return nil, fmt.Errorf("failed to find default credentials in environment: %w", err)
		}
		tokenSource = creds.TokenSource
	} else {
		jsonData := []byte(authCredentials.GetValue())

		// Peek at the JSON to detect the "type" field
		var meta struct {
			Type string `json:"type"`
		}
		if err := sonic.Unmarshal(jsonData, &meta); err != nil {
			return nil, fmt.Errorf("failed to parse auth credentials JSON: %w", err)
		}

		// Map string to google.CredentialsType with a security whitelist
		var credType google.CredentialsType
		switch meta.Type {
		case string(google.ServiceAccount):
			credType = google.ServiceAccount
		case string(google.ImpersonatedServiceAccount):
			credType = google.ImpersonatedServiceAccount
		case string(google.AuthorizedUser):
			credType = google.AuthorizedUser
		case string(google.ExternalAccount):
			credType = google.ExternalAccount
		case string(google.ExternalAccountAuthorizedUser):
			credType = google.ExternalAccountAuthorizedUser
		case "":
			return nil, fmt.Errorf("invalid google auth credentials: missing 'type'")
		default:
			return nil, fmt.Errorf("unsupported or restricted credential type: %s", meta.Type)
		}

		conf, err := google.CredentialsFromJSONWithType(context.Background(), jsonData, credType, cloudPlatformScope)
		if err != nil {
			return nil, fmt.Errorf("failed to create credentials from auth credentials JSON: %w", err)
		}
		tokenSource = conf.TokenSource
	}

	// Cache the token source. If another goroutine raced and stored first, use
	// that one — both are equally valid, but sharing maximises token reuse.
	actual, _ := vertexTokenSourcePool.LoadOrStore(clientKey, tokenSource)
	return actual.(oauth2.TokenSource), nil
}

// GetProviderKey returns the provider identifier for Vertex.
func (provider *VertexProvider) GetProviderKey() schemas.ModelProvider {
	return schemas.Vertex
}

// listModelsByKey performs a list models request for a single key.
// Returns the response and latency, or an error if the request fails.
//
// The logic is:
// 1. If deployments or allowedModels are configured, return those (no API call needed)
// 2. Otherwise, fetch from the publishers.models.list API endpoint (Model Garden)
func (provider *VertexProvider) listModelsByKey(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayListModelsRequest) (*schemas.GatewayListModelsResponse, *schemas.GatewayError) {
	region := resolveVertexRegion(ctx, key)
	if region == "" {
		return nil, providerUtils.NewConfigurationError("region is not set in key config")
	}

	deployments := key.Aliases
	allowedModels := key.Models

	if !request.Unfiltered && (allowedModels.IsEmpty() && len(deployments) == 0 || key.BlacklistedModels.IsBlockAll()) {
		return &schemas.GatewayListModelsResponse{Data: make([]schemas.Model, 0)}, nil
	}

	// If deployments or allowedModels are configured, return those directly without API call
	// Skip this fast path when Unfiltered is set so the full Vertex catalog can be retrieved
	if !request.Unfiltered && (len(deployments) > 0 || allowedModels.IsRestricted()) {
		return buildResponseFromConfig(deployments, allowedModels, key.BlacklistedModels), nil
	}

	// No deployments configured - fetch from Model Garden API
	host := getVertexModelListingAPIHost(region)

	// Accumulate all publisher models from paginated requests
	var allPublisherModels []VertexPublisherModel
	var rawRequests []interface{}
	var rawResponses []interface{}
	pageToken := ""

	// Getting oauth2 token
	tokenSource, err := getAuthTokenSource(key)
	if err != nil {
		return nil, providerUtils.NewGatewayOperationError("error creating auth token source (api key auth not supported for list models)", err)
	}
	token, err := tokenSource.Token()
	if err != nil {
		return nil, providerUtils.NewGatewayOperationError("error getting token (api key auth not supported for list models)", err)
	}

	// Iterate over all supported Vertex publishers to include Google, Anthropic, and Mistral models
	publishers := []string{"google", "anthropic", "mistralai"}
	for _, publisher := range publishers {
		pageToken = ""
		// Loop through all pages until no nextPageToken is returned
		for {
			// Build URL for publishers.models.list endpoint (Model Garden)
			// Format: https://{vertex-api-host}/v1beta1/publishers/{publisher}/models
			requestURL := fmt.Sprintf("https://%s/v1beta1/publishers/%s/models?pageSize=%d", host, publisher, MaxPageSize)
			if pageToken != "" {
				requestURL = fmt.Sprintf("%s&pageToken=%s", requestURL, url.QueryEscape(pageToken))
			}

			// Create HTTP request for listing models
			req := fasthttp.AcquireRequest()
			resp := fasthttp.AcquireResponse()

			req.Header.SetMethod(http.MethodGet)
			req.SetRequestURI(requestURL)
			req.Header.SetContentType("application/json")
			providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
			req.Header.Set("Authorization", "Bearer "+token.AccessToken)

			latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
			if gatewayErr != nil {
				wait()
				respBody := append([]byte(nil), resp.Body()...)
				fasthttp.ReleaseRequest(req)
				fasthttp.ReleaseResponse(resp)
				// Non-Google publishers may not be available in all regions; skip on error
				if publisher != "google" {
					break
				}
				return nil, providerUtils.EnrichError(ctx, gatewayErr, nil, respBody, provider.sendBackRawRequest, provider.sendBackRawResponse)
			}
			ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

			// Handle error response
			if resp.StatusCode() != fasthttp.StatusOK {
				if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
					removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
				}

				// Non-Google publishers may not be available in all regions;
				// skip only on 403/404 which indicate regional unavailability.
				// Surface other errors (401, 429, 5xx) so they aren't silently swallowed.
				if publisher != "google" && (resp.StatusCode() == fasthttp.StatusForbidden || resp.StatusCode() == fasthttp.StatusNotFound) {
					wait()
					fasthttp.ReleaseRequest(req)
					fasthttp.ReleaseResponse(resp)
					break
				}

				respBody := append([]byte(nil), resp.Body()...)
				statusCode := resp.StatusCode()
				wait()
				fasthttp.ReleaseRequest(req)
				fasthttp.ReleaseResponse(resp)

				var errorResp VertexError
				if err := sonic.Unmarshal(respBody, &errorResp); err != nil {
					return nil, providerUtils.EnrichError(ctx, providerUtils.NewGatewayOperationError(schemas.ErrProviderResponseUnmarshal, err), nil, respBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
				}
				return nil, providerUtils.EnrichError(ctx, providerUtils.NewProviderAPIError(errorResp.Error.Message, nil, statusCode, nil, nil), nil, respBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
			}

			// Parse Vertex's publisher models response
			var vertexResponse VertexListPublisherModelsResponse
			rawRequest, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(resp.Body(), &vertexResponse, nil, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
			if gatewayErr != nil {
				respBody := append([]byte(nil), resp.Body()...)
				wait()
				fasthttp.ReleaseRequest(req)
				fasthttp.ReleaseResponse(resp)
				return nil, providerUtils.EnrichError(ctx, gatewayErr, nil, respBody, provider.sendBackRawRequest, provider.sendBackRawResponse)
			}
			if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
				rawRequests = append(rawRequests, rawRequest)
			}
			if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
				rawResponses = append(rawResponses, rawResponse)
			}

			// Accumulate models from this page
			allPublisherModels = append(allPublisherModels, vertexResponse.PublisherModels...)

			wait()
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)

			// Check if there are more pages
			if vertexResponse.NextPageToken == "" {
				break
			}
			pageToken = vertexResponse.NextPageToken
		}
	}

	// Create aggregated response from all pages
	aggregatedResponse := &VertexListPublisherModelsResponse{
		PublisherModels: allPublisherModels,
	}

	response := aggregatedResponse.ToGatewayListModelsResponse(key.Models, key.BlacklistedModels, key.Aliases, request.Unfiltered)

	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		response.ExtraFields.RawRequest = rawRequests
	}

	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		response.ExtraFields.RawResponse = rawResponses
	}

	return response, nil
}

// ListModels performs a list models request to Vertex's API.
// Requests are made concurrently for improved performance.
func (provider *VertexProvider) ListModels(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayListModelsRequest) (*schemas.GatewayListModelsResponse, *schemas.GatewayError) {
	finalResponse, gatewayErr := providerUtils.HandleMultipleListModelsRequests(
		ctx,
		keys,
		request,
		provider.listModelsByKey,
	)
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	return finalResponse, nil
}

// TextCompletion is not supported by the Vertex provider.
// Returns an error indicating that text completion is not available.
func (provider *VertexProvider) TextCompletion(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayTextCompletionRequest) (*schemas.GatewayTextCompletionResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionRequest, provider.GetProviderKey())
}

// TextCompletionStream performs a streaming text completion request to Vertex's API.
// It formats the request, sends it to Vertex, and processes the response.
// Returns a channel of GatewayStreamChunk objects or an error if the request fails.
func (provider *VertexProvider) TextCompletionStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayTextCompletionRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionStreamRequest, provider.GetProviderKey())
}

// inlineRemoteURLSources replaces document AND image content blocks carrying a
// remote URL source with inline base64 bytes by fetching each URL. Required
// because Anthropic-on-Vertex does not accept URL-source documents or images
// (unlike direct Anthropic, which accepts source.type "url"). Mutates the
// request in place; safe to call when no such blocks are present. The ctx is
// propagated to each fetch so request cancellation/deadlines abort in-flight
// downloads.
func inlineRemoteURLSources(ctx context.Context, request *schemas.GatewayChatRequest) error {
	if request == nil || request.Input == nil {
		return nil
	}
	// When the caller is bypassing the converter via a pre-built raw body,
	// the request struct isn't what gets sent — skip the fetch.
	if useRawBody, ok := ctx.Value(schemas.GatewayContextKeyUseRawRequestBody).(bool); ok && useRawBody {
		return nil
	}
	for mi := range request.Input {
		msg := &request.Input[mi]
		if msg.Content == nil || msg.Content.ContentBlocks == nil {
			continue
		}
		for bi := range msg.Content.ContentBlocks {
			block := &msg.Content.ContentBlocks[bi]

			// Inline url-source documents.
			if block.File != nil && block.File.FileURL != nil && *block.File.FileURL != "" {
				mediaType, encoded, err := providerUtils.FetchAndEncodeURL(ctx, *block.File.FileURL)
				if err != nil {
					return err
				}
				block.File.FileData = &encoded
				if mediaType != "" && block.File.FileType == nil {
					block.File.FileType = &mediaType
				}
				block.File.FileURL = nil
			}

			// Inline url-source images to a base64 data URI; Anthropic-on-Vertex
			// accepts base64 image sources only. Skip data: URIs (already inline).
			if img := block.ImageURLStruct; img != nil && img.URL != "" && !strings.HasPrefix(img.URL, "data:") {
				mediaType, encoded, err := providerUtils.FetchAndEncodeURL(ctx, img.URL)
				if err != nil {
					return err
				}
				if mediaType != "" {
					img.URL = "data:" + mediaType + ";base64," + encoded
				} else {
					// Content-Type header absent; sniff the media type from the
					// fetched bytes so we never emit a malformed "data:;base64,..."
					// URI, which Anthropic-on-Vertex rejects.
					sanitized, sErr := schemas.SanitizeImageURL(encoded)
					if sErr != nil {
						return sErr
					}
					img.URL = sanitized
				}
			}
		}
	}
	return nil
}

// inlineDocumentURLsResponses is the Responses-API analogue of inlineDocumentURLs.
// File blocks live on ResponsesMessageContentBlock.ResponsesInputMessageContentBlockFile
// rather than the chat ContentBlock.File, so this walks the responses-shape input.
func inlineDocumentURLsResponses(ctx *schemas.GatewayContext, request *schemas.GatewayResponsesRequest) error {
	if request == nil || request.Input == nil {
		return nil
	}
	if useRawBody, ok := ctx.Value(schemas.GatewayContextKeyUseRawRequestBody).(bool); ok && useRawBody {
		return nil
	}
	for mi := range request.Input {
		msg := &request.Input[mi]
		if msg.Content == nil || msg.Content.ContentBlocks == nil {
			continue
		}
		for bi := range msg.Content.ContentBlocks {
			block := &msg.Content.ContentBlocks[bi]

			// Inline url-source files.
			if f := block.ResponsesInputMessageContentBlockFile; f != nil && f.FileURL != nil && *f.FileURL != "" {
				mediaType, encoded, err := providerUtils.FetchAndEncodeURL(ctx, *f.FileURL)
				if err != nil {
					return err
				}
				f.FileData = &encoded
				if mediaType != "" && f.FileType == nil {
					f.FileType = &mediaType
				}
				f.FileURL = nil
			}

			// Inline url-source images to a base64 data URI; Anthropic-on-Vertex
			// accepts base64 image sources only. Skip data: URIs (already inline).
			if img := block.ResponsesInputMessageContentBlockImage; img != nil && img.ImageURL != nil && *img.ImageURL != "" && !strings.HasPrefix(*img.ImageURL, "data:") {
				mediaType, encoded, err := providerUtils.FetchAndEncodeURL(ctx, *img.ImageURL)
				if err != nil {
					return err
				}
				if mediaType != "" {
					dataURI := "data:" + mediaType + ";base64," + encoded
					img.ImageURL = &dataURI
				} else {
					// Content-Type header absent; sniff the media type from the
					// fetched bytes so we never emit a malformed "data:;base64,..."
					// URI, which Anthropic-on-Vertex rejects.
					sanitized, sErr := schemas.SanitizeImageURL(encoded)
					if sErr != nil {
						return sErr
					}
					img.ImageURL = &sanitized
				}
			}
		}
	}
	return nil
}

// ChatCompletion performs a chat completion request to the Vertex API.
// It supports both text and image content in messages.
// Returns a GatewayResponse containing the completion results or an error if the request fails.
func (provider *VertexProvider) ChatCompletion(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayChatRequest) (*schemas.GatewayChatResponse, *schemas.GatewayError) {
	var jsonBody []byte
	var gatewayErr *schemas.GatewayError
	if schemas.IsAnthropicModelFamily(ctx, request.Model) {
		// Anthropic-on-Vertex doesn't accept URL-source document or image blocks.
		// Inline any URL documents/images to base64 before the converter runs.
		if err := inlineRemoteURLSources(ctx, request); err != nil {
			return nil, providerUtils.NewGatewayOperationError("failed to inline remote URL sources for vertex/claude", err)
		}
		jsonBody, gatewayErr = anthropic.BuildAnthropicChatRequestBody(ctx, request, anthropic.AnthropicRequestBuildConfig{
			Provider:                  schemas.Vertex,
			Model:                     request.Model,
			BetaHeaderOverrides:       provider.networkConfig.BetaHeaderOverrides,
			ProviderExtraHeaders:      provider.networkConfig.ExtraHeaders,
			ShouldSendBackRawRequest:  provider.sendBackRawRequest,
			ShouldSendBackRawResponse: provider.sendBackRawResponse,
		})
	} else {
		jsonBody, gatewayErr = providerUtils.CheckContextAndGetRequestBody(
			ctx,
			request,
			func() (providerUtils.RequestBodyWithExtraParams, error) {
				// Format messages for Vertex API, preserving key order for prompt caching
				var rawBody []byte
				var extraParams map[string]interface{}
				var err error

				if schemas.IsGeminiModelFamily(ctx, request.Model) || schemas.IsAllDigitsASCII(request.Model) || schemas.IsGemmaModelFamily(ctx, request.Model) {
					reqBody, err := gemini.ToGeminiChatCompletionRequestWithImageURLSchemes(ctx, request, geminiImageURLSchemes...)
					if err != nil {
						return nil, err
					}
					if reqBody == nil {
						return nil, fmt.Errorf("chat completion input is not provided")
					}
					extraParams = reqBody.GetExtraParams()
					// Strip unsupported fields for Vertex Gemini
					stripVertexGeminiUnsupportedFields(reqBody)
					// Marshal to JSON bytes
					rawBody, err = providerUtils.MarshalSorted(reqBody)
					if err != nil {
						return nil, fmt.Errorf("failed to marshal request body: %w", err)
					}
				} else {
					// Use centralized OpenAI converter for non-Claude models
					reqBody := openai.ToOpenAIChatRequest(ctx, request)
					if reqBody == nil {
						return nil, fmt.Errorf("chat completion input is not provided")
					}
					extraParams = reqBody.GetExtraParams()
					// Marshal to JSON bytes
					rawBody, err = providerUtils.MarshalSorted(reqBody)
					if err != nil {
						return nil, fmt.Errorf("failed to marshal request body: %w", err)
					}
				}
				// Remove region field if present
				rawBody, err = providerUtils.DeleteJSONField(rawBody, "region")
				if err != nil {
					return nil, fmt.Errorf("failed to delete region field: %w", err)
				}
				return &VertexRawRequestBody{RawBody: rawBody, ExtraParams: extraParams}, nil
			},
		)
	}
	if gatewayErr != nil {
		return nil, gatewayErr
	}
	if schemas.IsGeminiModelFamily(ctx, request.Model) || schemas.IsAllDigitsASCII(request.Model) || schemas.IsGemmaModelFamily(ctx, request.Model) {
		if rawBody, ok := ctx.Value(schemas.GatewayContextKeyUseRawRequestBody).(bool); ok && rawBody {
			jsonBody = gemini.NormalizeRawGenerateContentRequestForCompatibility(jsonBody)
		}
		jsonBody = stripVertexGeminiUnsupportedFieldsRaw(jsonBody)
	}

	projectID := resolveVertexProjectID(ctx, key)
	if projectID == "" {
		return nil, providerUtils.NewConfigurationError("project ID is not set")
	}

	region := resolveVertexRegion(ctx, key)
	if region == "" {
		return nil, providerUtils.NewConfigurationError("region is not set in key config")
	}

	// Remap unsupported tool versions for Vertex (handles raw passthrough bodies)
	if schemas.IsAnthropicModelFamily(ctx, request.Model) && jsonBody != nil {
		capModel := schemas.ResolveCanonicalModel(ctx, request.Model)
		remappedBody, remapErr := anthropic.RemapRawToolVersionsForProvider(jsonBody, schemas.Vertex, capModel)
		if remapErr != nil {
			return nil, providerUtils.NewGatewayOperationError(remapErr.Error(), nil)
		}
		jsonBody = remappedBody

		// Strip unsupported body fields for Vertex — covers both structured and raw passthrough paths.
		var stripErr error
		jsonBody, stripErr = anthropic.StripUnsupportedFieldsFromRawBody(jsonBody, schemas.Vertex, capModel)
		if stripErr != nil {
			return nil, providerUtils.NewGatewayOperationError(stripErr.Error(), nil)
		}
	}

	// Auth query is used for fine-tuned models to pass the API key in the query string
	authQuery := ""
	// Determine the URL based on model type
	var completeURL string
	if schemas.IsAllDigitsASCII(request.Model) {
		// Custom Fine-tuned models use OpenAPI endpoint
		projectNumber := resolveVertexProjectNumber(ctx, key)
		if projectNumber == "" {
			return nil, providerUtils.NewConfigurationError("project number is not set for fine-tuned models")
		}
		if key.Value.GetValue() != "" {
			authQuery = fmt.Sprintf("key=%s", url.QueryEscape(key.Value.GetValue()))
		}
		completeURL = getVertexEndpointURL(region, "v1beta1", projectNumber, request.Model, ":generateContent")
	} else if schemas.IsAnthropicModelFamily(ctx, request.Model) {
		// Claude models use Anthropic publisher — model-aware host for multi-region support
		completeURL = getVertexModelAwarePublisherModelURL(region, "v1", projectID, "anthropic", request.Model, ":rawPredict")
	} else if schemas.IsMistralModelFamily(ctx, request.Model) {
		// Mistral models use mistralai publisher with rawPredict
		completeURL = getVertexPublisherModelURL(region, "v1", projectID, "mistralai", request.Model, ":rawPredict")
	} else if schemas.IsGeminiModelFamily(ctx, request.Model) || schemas.IsGemmaModelFamily(ctx, request.Model) {
		// Gemini models support api key
		if key.Value.GetValue() != "" {
			authQuery = fmt.Sprintf("key=%s", url.QueryEscape(key.Value.GetValue()))
		}
		completeURL = getVertexPublisherModelURL(region, "v1", projectID, "google", gemini.NormalizeModelName(request.Model), ":generateContent")
	} else {
		completeURL = getVertexEndpointURL(region, "v1beta1", projectID, "openapi/chat/completions", "")
	}

	// Create HTTP request for streaming
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	respOwned := true
	defer func() {
		if respOwned {
			fasthttp.ReleaseResponse(resp)
		}
	}()

	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	if (schemas.IsGeminiModelFamily(ctx, request.Model) || schemas.IsAllDigitsASCII(request.Model)) &&
		request.Params != nil && request.Params.ServiceTier != nil {
		if v := vertexServiceTierHeaderValue(region, request.Model, *request.Params.ServiceTier); v != "" {
			req.Header.Set(VertexServiceTierHeader, v)
		}
	}
	// Skip anthropic-beta from context headers — Anthropic models on Vertex use the
	// anthropic_beta body field instead, and other model families don't use it.
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, []string{anthropic.AnthropicBetaHeader})

	// If auth query is set, add it to the URL
	// Otherwise, get the oauth2 token and set the Authorization header
	if authQuery != "" {
		completeURL = fmt.Sprintf("%s?%s", completeURL, authQuery)
	} else {
		// Getting oauth2 token
		tokenSource, err := getAuthTokenSource(key)
		if err != nil {
			return nil, providerUtils.NewGatewayOperationError("error creating auth token source", err)
		}
		token, err := tokenSource.Token()
		if err != nil {
			return nil, providerUtils.NewGatewayOperationError("error getting token", err)
		}
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	}

	req.SetRequestURI(completeURL)
	usedLargePayloadBody := providerUtils.ApplyLargePayloadRequestBody(ctx, req)
	if !usedLargePayloadBody {
		req.SetBody(jsonBody)
	}

	// Make the request with optional large response streaming
	activeClient := providerUtils.PrepareResponseStreaming(ctx, provider.client, resp)
	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}
	if usedLargePayloadBody {
		providerUtils.DrainLargePayloadRemainder(ctx)
	}
	ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	if resp.StatusCode() != fasthttp.StatusOK {
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		// Remove client from pool for authentication/authorization errors
		if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
			removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
		}
		return nil, providerUtils.EnrichError(ctx, parseVertexError(resp), jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	responseBody, isLargeResp, decodeErr := providerUtils.FinalizeResponseWithLargeDetection(ctx, resp, provider.logger)
	if decodeErr != nil {
		return nil, providerUtils.EnrichError(ctx, decodeErr, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}
	if isLargeResp {
		respOwned = false
		return &schemas.GatewayChatResponse{
			Model: request.Model,
			ExtraFields: schemas.GatewayResponseExtraFields{
				Latency:                 latency.Milliseconds(),
				ProviderResponseHeaders: providerUtils.ExtractProviderResponseHeaders(resp),
			},
		}, nil
	}

	if schemas.IsAnthropicModelFamily(ctx, request.Model) {
		// Create response object from pool
		anthropicResponse := anthropic.AcquireAnthropicMessageResponse()
		defer anthropic.ReleaseAnthropicMessageResponse(anthropicResponse)

		rawRequest, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(responseBody, anthropicResponse, jsonBody, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
		if gatewayErr != nil {
			return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}

		// Create final response
		response := anthropicResponse.ToGatewayChatResponse(ctx)

		response.ExtraFields = schemas.GatewayResponseExtraFields{
			Latency:                 latency.Milliseconds(),
			ProviderResponseHeaders: providerUtils.ExtractProviderResponseHeaders(resp),
		}

		// Set raw request if enabled
		if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
			response.ExtraFields.RawRequest = rawRequest
		}

		// Set raw response if enabled
		if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
			response.ExtraFields.RawResponse = rawResponse
		}

		return response, nil
	} else if schemas.IsGeminiModelFamily(ctx, request.Model) || schemas.IsAllDigitsASCII(request.Model) || schemas.IsGemmaModelFamily(ctx, request.Model) {
		geminiResponse := gemini.GenerateContentResponse{}

		rawRequest, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(responseBody, &geminiResponse, jsonBody, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
		if gatewayErr != nil {
			return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}

		response := geminiResponse.ToGatewayChatResponse()
		response.ExtraFields.Latency = latency.Milliseconds()
		response.ExtraFields.ProviderResponseHeaders = providerUtils.ExtractProviderResponseHeaders(resp)

		if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
			response.ExtraFields.RawRequest = rawRequest
		}

		if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
			response.ExtraFields.RawResponse = rawResponse
		}

		return response, nil
	} else {
		response := &schemas.GatewayChatResponse{}

		// Use enhanced response handler with pre-allocated response
		rawRequest, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(responseBody, response, jsonBody, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
		if gatewayErr != nil {
			return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}

		response.ExtraFields.Latency = latency.Milliseconds()
		response.ExtraFields.ProviderResponseHeaders = providerUtils.ExtractProviderResponseHeaders(resp)

		// Set raw request if enabled
		if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
			response.ExtraFields.RawRequest = rawRequest
		}

		// Set raw response if enabled
		if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
			response.ExtraFields.RawResponse = rawResponse
		}

		return response, nil
	}
}

// ChatCompletionStream performs a streaming chat completion request to the Vertex API.
// It supports both OpenAI-style streaming (for non-Claude models) and Anthropic-style streaming (for Claude models).
// Returns a channel of GatewayStreamChunk objects for streaming results or an error if the request fails.
func (provider *VertexProvider) ChatCompletionStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayChatRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	providerName := provider.GetProviderKey()
	projectID := resolveVertexProjectID(ctx, key)
	if projectID == "" {
		return nil, providerUtils.NewConfigurationError("project ID is not set")
	}

	region := resolveVertexRegion(ctx, key)
	if region == "" {
		return nil, providerUtils.NewConfigurationError("region is not set in key config")
	}

	if schemas.IsAnthropicModelFamily(ctx, request.Model) {
		// Use Anthropic-style streaming for Claude models.
		// Anthropic-on-Vertex doesn't accept URL-source document or image blocks; inline first.
		if err := inlineRemoteURLSources(ctx, request); err != nil {
			return nil, providerUtils.NewGatewayOperationError("failed to inline remote URL sources for vertex/claude", err)
		}
		jsonData, gatewayErr := anthropic.BuildAnthropicChatRequestBody(ctx, request, anthropic.AnthropicRequestBuildConfig{
			Provider:                  schemas.Vertex,
			Model:                     request.Model,
			IsStreaming:               true,
			BetaHeaderOverrides:       provider.networkConfig.BetaHeaderOverrides,
			ProviderExtraHeaders:      provider.networkConfig.ExtraHeaders,
			ShouldSendBackRawRequest:  provider.sendBackRawRequest,
			ShouldSendBackRawResponse: provider.sendBackRawResponse,
		})
		if gatewayErr != nil {
			return nil, gatewayErr
		}

		// Remap unsupported tool versions for Vertex streaming (handles raw passthrough bodies)
		if jsonData != nil {
			capModel := schemas.ResolveCanonicalModel(ctx, request.Model)
			var remapErr error
			jsonData, remapErr = anthropic.RemapRawToolVersionsForProvider(jsonData, schemas.Vertex, capModel)
			if remapErr != nil {
				return nil, providerUtils.NewGatewayOperationError(remapErr.Error(), nil)
			}

			// Strip unsupported body fields for Vertex — covers both structured and raw passthrough paths.
			var stripErr error
			jsonData, stripErr = anthropic.StripUnsupportedFieldsFromRawBody(jsonData, schemas.Vertex, capModel)
			if stripErr != nil {
				return nil, providerUtils.NewGatewayOperationError(stripErr.Error(), nil)
			}
		}

		completeURL := getVertexModelAwarePublisherModelURL(region, "v1", projectID, "anthropic", request.Model, ":streamRawPredict")

		// Prepare headers for Vertex Anthropic
		headers := map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "text/event-stream",
			"Cache-Control": "no-cache",
		}

		// Adding authorization header
		tokenSource, err := getAuthTokenSource(key)
		if err != nil {
			return nil, providerUtils.NewGatewayOperationError("error creating auth token source", err)
		}
		token, err := tokenSource.Token()
		if err != nil {
			return nil, providerUtils.NewGatewayOperationError("error getting token", err)
		}
		headers["Authorization"] = "Bearer " + token.AccessToken

		// Use shared Anthropic streaming logic
		return anthropic.HandleAnthropicChatCompletionStreaming(
			ctx,
			provider.streamingClient,
			completeURL,
			jsonData,
			headers,
			provider.networkConfig.ExtraHeaders,
			provider.networkConfig.StreamIdleTimeoutInSeconds,
			provider.networkConfig.BetaHeaderOverrides,
			providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
			providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
			providerName,
			postHookRunner,
			nil,
			nil,
			provider.logger,
			postHookSpanFinalizer,
		)
	} else if schemas.IsGeminiModelFamily(ctx, request.Model) || schemas.IsAllDigitsASCII(request.Model) || schemas.IsGemmaModelFamily(ctx, request.Model) {
		// Use Gemini-style streaming for Gemini models
		jsonData, gatewayErr := providerUtils.CheckContextAndGetRequestBody(
			ctx,
			request,
			func() (providerUtils.RequestBodyWithExtraParams, error) {
				reqBody, err := gemini.ToGeminiChatCompletionRequestWithImageURLSchemes(ctx, request, geminiImageURLSchemes...)
				if err != nil {
					return nil, err
				}
				if reqBody == nil {
					return nil, fmt.Errorf("chat completion input is not provided")
				}
				// Strip unsupported fields for Vertex Gemini
				stripVertexGeminiUnsupportedFields(reqBody)
				return reqBody, nil
			},
		)
		if gatewayErr != nil {
			return nil, gatewayErr
		}

		// Auth query is used to pass the API key in the query string
		authQuery := ""
		if key.Value.GetValue() != "" {
			authQuery = fmt.Sprintf("key=%s", url.QueryEscape(key.Value.GetValue()))
		}

		// For custom/fine-tuned models, validate projectNumber is set
		projectNumber := resolveVertexProjectNumber(ctx, key)
		if schemas.IsAllDigitsASCII(request.Model) && projectNumber == "" {
			return nil, providerUtils.NewConfigurationError("project number is not set for fine-tuned models")
		}

		// Construct the URL for Gemini streaming
		completeURL := getCompleteURLForGeminiEndpoint(request.Model, region, projectID, projectNumber, ":streamGenerateContent")

		// Add alt=sse parameter
		if authQuery != "" {
			completeURL = fmt.Sprintf("%s?alt=sse&%s", completeURL, authQuery)
		} else {
			completeURL = fmt.Sprintf("%s?alt=sse", completeURL)
		}

		// Prepare headers for Vertex Gemini
		headers := map[string]string{
			"Accept":        "text/event-stream",
			"Cache-Control": "no-cache",
		}

		if schemas.IsGeminiModelFamily(ctx, request.Model) || schemas.IsAllDigitsASCII(request.Model) {
			if _, overridden := provider.networkConfig.ExtraHeaders[VertexServiceTierHeader]; !overridden {
				if request.Params != nil && request.Params.ServiceTier != nil {
					if v := vertexServiceTierHeaderValue(region, request.Model, *request.Params.ServiceTier); v != "" {
						headers[VertexServiceTierHeader] = v
					}
				}
			}
		}

		// If no auth query, use OAuth2 token
		if authQuery == "" {
			tokenSource, err := getAuthTokenSource(key)
			if err != nil {
				return nil, providerUtils.NewGatewayOperationError("error creating auth token source", err)
			}
			token, err := tokenSource.Token()
			if err != nil {
				return nil, providerUtils.NewGatewayOperationError("error getting token", err)
			}
			headers["Authorization"] = "Bearer " + token.AccessToken
		}

		// Use shared streaming logic from Gemini
		return gemini.HandleGeminiChatCompletionStream(
			ctx,
			provider.streamingClient,
			completeURL,
			jsonData,
			headers,
			provider.networkConfig.ExtraHeaders,
			providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
			providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
			provider.GetProviderKey(),
			request.Model,
			postHookRunner,
			nil,
			provider.logger,
			postHookSpanFinalizer,
		)
	} else {
		var authHeader map[string]string
		// Auth query is used for fine-tuned models to pass the API key in the query string
		authQuery := ""
		// Determine the URL based on model type
		var completeURL string
		if schemas.IsMistralModelFamily(ctx, request.Model) {
			// Mistral models use mistralai publisher with streamRawPredict
			completeURL = getVertexPublisherModelURL(region, "v1", projectID, "mistralai", request.Model, ":streamRawPredict")
		} else {
			// Other models use OpenAPI endpoint for gemini models
			if key.Value.GetValue() != "" {
				authQuery = fmt.Sprintf("key=%s", url.QueryEscape(key.Value.GetValue()))
			}
			completeURL = getVertexEndpointURL(region, "v1beta1", projectID, "openapi/chat/completions", "")
		}

		if authQuery != "" {
			completeURL = fmt.Sprintf("%s?%s", completeURL, authQuery)
		} else {
			// Getting oauth2 token
			tokenSource, err := getAuthTokenSource(key)
			if err != nil {
				return nil, providerUtils.NewGatewayOperationError("error creating auth token source", err)
			}
			token, err := tokenSource.Token()
			if err != nil {
				return nil, providerUtils.NewGatewayOperationError("error getting token", err)
			}
			authHeader = map[string]string{
				"Authorization": "Bearer " + token.AccessToken,
			}
		}

		// Use shared OpenAI streaming logic
		return openai.HandleOpenAIChatCompletionStreaming(
			ctx,
			provider.streamingClient,
			completeURL,
			request,
			authHeader,
			provider.networkConfig.ExtraHeaders,
			provider.networkConfig.StreamIdleTimeoutInSeconds,
			providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
			providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
			providerName,
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
}

// Responses performs a responses request to the Vertex API.
func (provider *VertexProvider) Responses(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayResponsesRequest) (*schemas.GatewayResponsesResponse, *schemas.GatewayError) {
	if schemas.IsAnthropicModelFamily(ctx, request.Model) {
		// Anthropic-on-Vertex doesn't accept URL-source document blocks.
		// Inline any URL documents to base64 before the converter runs.
		if err := inlineDocumentURLsResponses(ctx, request); err != nil {
			return nil, providerUtils.NewGatewayOperationError("failed to inline document URLs for vertex/claude", err)
		}
		jsonBody, gatewayErr := anthropic.BuildAnthropicResponsesRequestBody(ctx, request, anthropic.AnthropicRequestBuildConfig{
			Provider:                  schemas.Vertex,
			Model:                     request.Model,
			BetaHeaderOverrides:       provider.networkConfig.BetaHeaderOverrides,
			ProviderExtraHeaders:      provider.networkConfig.ExtraHeaders,
			ValidateTools:             true,
			ShouldSendBackRawRequest:  provider.sendBackRawRequest,
			ShouldSendBackRawResponse: provider.sendBackRawResponse,
		})
		if gatewayErr != nil {
			return nil, gatewayErr
		}
		projectID := resolveVertexProjectID(ctx, key)
		if projectID == "" {
			return nil, providerUtils.NewConfigurationError("project ID is not set")
		}

		region := resolveVertexRegion(ctx, key)
		if region == "" {
			return nil, providerUtils.NewConfigurationError("region is not set in key config")
		}

		// Claude models use Anthropic publisher — model-aware host for multi-region support
		url := getVertexModelAwarePublisherModelURL(region, "v1beta1", projectID, "anthropic", request.Model, ":rawPredict")

		// Create HTTP request for streaming
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()
		defer fasthttp.ReleaseRequest(req)
		respOwned := true
		defer func() {
			if respOwned {
				fasthttp.ReleaseResponse(resp)
			}
		}()

		req.Header.SetMethod(http.MethodPost)
		req.Header.SetContentType("application/json")
		providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, []string{anthropic.AnthropicBetaHeader})

		if betaHeaders := anthropic.FilterBetaHeadersForProvider(anthropic.MergeBetaHeaders(ctx, provider.networkConfig.ExtraHeaders), schemas.Vertex, provider.networkConfig.BetaHeaderOverrides); len(betaHeaders) > 0 {
			req.Header.Set(anthropic.AnthropicBetaHeader, strings.Join(betaHeaders, ","))
		} else {
			req.Header.Del(anthropic.AnthropicBetaHeader)
		}

		// Getting oauth2 token
		tokenSource, err := getAuthTokenSource(key)
		if err != nil {
			return nil, providerUtils.NewGatewayOperationError("error creating auth token source", err)
		}
		token, err := tokenSource.Token()
		if err != nil {
			return nil, providerUtils.NewGatewayOperationError("error getting token", err)
		}
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)

		req.SetRequestURI(url)
		usedLargePayloadBody := providerUtils.ApplyLargePayloadRequestBody(ctx, req)
		if !usedLargePayloadBody {
			req.SetBody(jsonBody)
		}

		// Make the request with optional large response streaming
		activeClient := providerUtils.PrepareResponseStreaming(ctx, provider.client, resp)
		latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
		defer wait()
		if gatewayErr != nil {
			return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}
		if usedLargePayloadBody {
			providerUtils.DrainLargePayloadRemainder(ctx)
		}
		ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

		if resp.StatusCode() != fasthttp.StatusOK {
			providerUtils.MaterializeStreamErrorBody(ctx, resp)
			// Remove client from pool for authentication/authorization errors
			if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
				removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
			}
			return nil, providerUtils.EnrichError(ctx, parseVertexError(resp), jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}

		responseBody, isLargeResp, decodeErr := providerUtils.FinalizeResponseWithLargeDetection(ctx, resp, provider.logger)
		if decodeErr != nil {
			return nil, providerUtils.EnrichError(ctx, decodeErr, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}
		if isLargeResp {
			respOwned = false
			return &schemas.GatewayResponsesResponse{
				ExtraFields: schemas.GatewayResponseExtraFields{
					Latency:                 latency.Milliseconds(),
					ProviderResponseHeaders: providerUtils.ExtractProviderResponseHeaders(resp),
				},
			}, nil
		}

		// Create response object from pool
		anthropicResponse := anthropic.AcquireAnthropicMessageResponse()
		defer anthropic.ReleaseAnthropicMessageResponse(anthropicResponse)

		rawRequest, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(responseBody, anthropicResponse, jsonBody, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
		if gatewayErr != nil {
			return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}

		// Create final response
		response := anthropicResponse.ToGatewayResponsesResponse(ctx)

		response.ExtraFields = schemas.GatewayResponseExtraFields{
			Latency: latency.Milliseconds(),
		}

		response.ExtraFields.ProviderResponseHeaders = providerUtils.ExtractProviderResponseHeaders(resp)
		// Set raw request if enabled
		if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
			response.ExtraFields.RawRequest = rawRequest
		}

		// Set raw response if enabled
		if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
			response.ExtraFields.RawResponse = rawResponse
		}

		return response, nil
	} else if schemas.IsGeminiModelFamily(ctx, request.Model) || schemas.IsAllDigitsASCII(request.Model) || schemas.IsGemmaModelFamily(ctx, request.Model) {
		jsonBody, gatewayErr := providerUtils.CheckContextAndGetRequestBody(
			ctx,
			request,
			func() (providerUtils.RequestBodyWithExtraParams, error) {
				reqBody, err := gemini.ToGeminiResponsesRequestWithImageURLSchemes(ctx, request, geminiImageURLSchemes...)
				if err != nil {
					return nil, err
				}
				if reqBody == nil {
					return nil, fmt.Errorf("responses input is not provided")
				}
				// Strip unsupported fields for Vertex Gemini
				stripVertexGeminiUnsupportedFields(reqBody)
				return reqBody, nil
			},
		)
		if gatewayErr != nil {
			return nil, gatewayErr
		}
		if rawBody, ok := ctx.Value(schemas.GatewayContextKeyUseRawRequestBody).(bool); ok && rawBody {
			jsonBody = gemini.NormalizeRawGenerateContentRequestForCompatibility(jsonBody)
		}
		jsonBody = stripVertexGeminiUnsupportedFieldsRaw(jsonBody)

		projectID := resolveVertexProjectID(ctx, key)
		if projectID == "" {
			return nil, providerUtils.NewConfigurationError("project ID is not set")
		}

		region := resolveVertexRegion(ctx, key)
		if region == "" {
			return nil, providerUtils.NewConfigurationError("region is not set in key config")
		}

		authQuery := ""
		if key.Value.GetValue() != "" {
			authQuery = fmt.Sprintf("key=%s", url.QueryEscape(key.Value.GetValue()))
		}

		// For custom/fine-tuned models, validate projectNumber is set
		projectNumber := resolveVertexProjectNumber(ctx, key)
		if schemas.IsAllDigitsASCII(request.Model) && projectNumber == "" {
			return nil, providerUtils.NewConfigurationError("project number is not set for fine-tuned models")
		}

		url := getCompleteURLForGeminiEndpoint(request.Model, region, projectID, projectNumber, ":generateContent")

		// Create HTTP request for streaming
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()
		defer fasthttp.ReleaseRequest(req)
		respOwned := true
		defer func() {
			if respOwned {
				fasthttp.ReleaseResponse(resp)
			}
		}()

		req.Header.SetMethod(http.MethodPost)
		req.Header.SetContentType("application/json")
		if (schemas.IsGeminiModelFamily(ctx, request.Model) || schemas.IsAllDigitsASCII(request.Model)) &&
			request.Params != nil && request.Params.ServiceTier != nil {
			if v := vertexServiceTierHeaderValue(region, request.Model, *request.Params.ServiceTier); v != "" {
				req.Header.Set(VertexServiceTierHeader, v)
			}
		}

		providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

		// If auth query is set, add it to the URL
		// Otherwise, get the oauth2 token and set the Authorization header
		if authQuery != "" {
			url = fmt.Sprintf("%s?%s", url, authQuery)
		} else {
			// Getting oauth2 token
			tokenSource, err := getAuthTokenSource(key)
			if err != nil {
				return nil, providerUtils.NewGatewayOperationError("error creating auth token source", err)
			}
			token, err := tokenSource.Token()
			if err != nil {
				return nil, providerUtils.NewGatewayOperationError("error getting token", err)
			}
			req.Header.Set("Authorization", "Bearer "+token.AccessToken)
		}

		req.SetRequestURI(url)
		usedLargePayloadBody := providerUtils.ApplyLargePayloadRequestBody(ctx, req)
		if !usedLargePayloadBody {
			req.SetBody(jsonBody)
		}

		// Make the request with optional large response streaming
		activeClient := providerUtils.PrepareResponseStreaming(ctx, provider.client, resp)
		latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
		defer wait()
		if gatewayErr != nil {
			return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}
		if usedLargePayloadBody {
			providerUtils.DrainLargePayloadRemainder(ctx)
		}
		ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

		if resp.StatusCode() != fasthttp.StatusOK {
			providerUtils.MaterializeStreamErrorBody(ctx, resp)
			// Remove client from pool for authentication/authorization errors
			if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
				removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
			}
			return nil, providerUtils.EnrichError(ctx, parseVertexError(resp), jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}

		responseBody, isLargeResp, decodeErr := providerUtils.FinalizeResponseWithLargeDetection(ctx, resp, provider.logger)
		if decodeErr != nil {
			return nil, providerUtils.EnrichError(ctx, decodeErr, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}
		if isLargeResp {
			respOwned = false
			return &schemas.GatewayResponsesResponse{
				ExtraFields: schemas.GatewayResponseExtraFields{
					Latency:                 latency.Milliseconds(),
					ProviderResponseHeaders: providerUtils.ExtractProviderResponseHeaders(resp),
				},
			}, nil
		}

		geminiResponse := &gemini.GenerateContentResponse{}

		rawRequest, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(responseBody, geminiResponse, jsonBody, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
		if gatewayErr != nil {
			return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}

		response := geminiResponse.ToResponsesGatewayResponsesResponse()
		response.ExtraFields.Latency = latency.Milliseconds()
		response.ExtraFields.ProviderResponseHeaders = providerUtils.ExtractProviderResponseHeaders(resp)

		// Set raw response if enabled
		if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
			response.ExtraFields.RawResponse = rawResponse
		}

		if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
			response.ExtraFields.RawRequest = rawRequest
		}

		return response, nil
	} else {
		chatResponse, err := provider.ChatCompletion(ctx, key, request.ToChatRequest())
		if err != nil {
			return nil, err
		}

		response := chatResponse.ToGatewayResponsesResponse()
		return response, nil
	}
}

// ResponsesStream performs a streaming responses request to the Vertex API.
func (provider *VertexProvider) ResponsesStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayResponsesRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	if schemas.IsAnthropicModelFamily(ctx, request.Model) {
		region := resolveVertexRegion(ctx, key)
		if region == "" {
			return nil, providerUtils.NewConfigurationError("region is not set in key config")
		}

		projectID := resolveVertexProjectID(ctx, key)
		if projectID == "" {
			return nil, providerUtils.NewConfigurationError("project ID is not set")
		}

		// Anthropic-on-Vertex doesn't accept URL-source document blocks.
		// Inline any URL documents to base64 before the converter runs.
		if err := inlineDocumentURLsResponses(ctx, request); err != nil {
			return nil, providerUtils.NewGatewayOperationError("failed to inline document URLs for vertex/claude", err)
		}
		jsonBody, gatewayErr := anthropic.BuildAnthropicResponsesRequestBody(ctx, request, anthropic.AnthropicRequestBuildConfig{
			Provider:                  schemas.Vertex,
			Model:                     request.Model,
			IsStreaming:               true,
			BetaHeaderOverrides:       provider.networkConfig.BetaHeaderOverrides,
			ProviderExtraHeaders:      provider.networkConfig.ExtraHeaders,
			ValidateTools:             true,
			ShouldSendBackRawRequest:  provider.sendBackRawRequest,
			ShouldSendBackRawResponse: provider.sendBackRawResponse,
		})
		if gatewayErr != nil {
			return nil, gatewayErr
		}

		url := getVertexModelAwarePublisherModelURL(region, "v1", projectID, "anthropic", request.Model, ":streamRawPredict")

		// Prepare headers for Vertex Anthropic
		headers := map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "text/event-stream",
			"Cache-Control": "no-cache",
		}

		// Adding authorization header
		tokenSource, err := getAuthTokenSource(key)
		if err != nil {
			return nil, providerUtils.NewGatewayOperationError("error creating auth token source", err)
		}
		token, err := tokenSource.Token()
		if err != nil {
			return nil, providerUtils.NewGatewayOperationError("error getting token", err)
		}
		headers["Authorization"] = "Bearer " + token.AccessToken

		// Use shared streaming logic from Anthropic
		return anthropic.HandleAnthropicResponsesStream(
			ctx,
			provider.streamingClient,
			url,
			jsonBody,
			headers,
			provider.networkConfig.ExtraHeaders,
			provider.networkConfig.StreamIdleTimeoutInSeconds,
			provider.networkConfig.BetaHeaderOverrides,
			providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
			providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
			provider.GetProviderKey(),
			postHookRunner,
			nil,
			nil,
			provider.logger,
			postHookSpanFinalizer,
		)
	} else if schemas.IsGeminiModelFamily(ctx, request.Model) || schemas.IsAllDigitsASCII(request.Model) || schemas.IsGemmaModelFamily(ctx, request.Model) {
		region := resolveVertexRegion(ctx, key)
		if region == "" {
			return nil, providerUtils.NewConfigurationError("region is not set in key config")
		}

		projectID := resolveVertexProjectID(ctx, key)
		if projectID == "" {
			return nil, providerUtils.NewConfigurationError("project ID is not set")
		}

		// Use Gemini-style streaming for Gemini models
		jsonData, gatewayErr := providerUtils.CheckContextAndGetRequestBody(
			ctx,
			request,
			func() (providerUtils.RequestBodyWithExtraParams, error) {
				reqBody, err := gemini.ToGeminiResponsesRequestWithImageURLSchemes(ctx, request, geminiImageURLSchemes...)
				if err != nil {
					return nil, err
				}
				if reqBody == nil {
					return nil, fmt.Errorf("responses input is not provided")
				}
				// Strip unsupported fields for Vertex Gemini
				stripVertexGeminiUnsupportedFields(reqBody)
				return reqBody, nil
			},
		)
		if gatewayErr != nil {
			return nil, gatewayErr
		}
		if rawBody, ok := ctx.Value(schemas.GatewayContextKeyUseRawRequestBody).(bool); ok && rawBody {
			jsonData = gemini.NormalizeRawGenerateContentRequestForCompatibility(jsonData)
		}
		jsonData = stripVertexGeminiUnsupportedFieldsRaw(jsonData)

		// Auth query is used to pass the API key in the query string
		authQuery := ""
		if key.Value.GetValue() != "" {
			authQuery = fmt.Sprintf("key=%s", url.QueryEscape(key.Value.GetValue()))
		}

		// For custom/fine-tuned models, validate projectNumber is set
		projectNumber := resolveVertexProjectNumber(ctx, key)
		if schemas.IsAllDigitsASCII(request.Model) && projectNumber == "" {
			return nil, providerUtils.NewConfigurationError("project number is not set for fine-tuned models")
		}

		// Construct the URL for Gemini streaming
		completeURL := getCompleteURLForGeminiEndpoint(request.Model, region, projectID, projectNumber, ":streamGenerateContent")
		// Add alt=sse parameter
		if authQuery != "" {
			completeURL = fmt.Sprintf("%s?alt=sse&%s", completeURL, authQuery)
		} else {
			completeURL = fmt.Sprintf("%s?alt=sse", completeURL)
		}

		// Prepare headers for Vertex Gemini
		headers := map[string]string{
			"Accept":        "text/event-stream",
			"Cache-Control": "no-cache",
		}

		if schemas.IsGeminiModelFamily(ctx, request.Model) || schemas.IsAllDigitsASCII(request.Model) {
			if _, overridden := provider.networkConfig.ExtraHeaders[VertexServiceTierHeader]; !overridden {
				if request.Params != nil && request.Params.ServiceTier != nil {
					if v := vertexServiceTierHeaderValue(region, request.Model, *request.Params.ServiceTier); v != "" {
						headers[VertexServiceTierHeader] = v
					}
				}
			}
		}

		// If no auth query, use OAuth2 token
		if authQuery == "" {
			tokenSource, err := getAuthTokenSource(key)
			if err != nil {
				return nil, providerUtils.NewGatewayOperationError("error creating auth token source", err)
			}
			token, err := tokenSource.Token()
			if err != nil {
				return nil, providerUtils.NewGatewayOperationError("error getting token", err)
			}
			headers["Authorization"] = "Bearer " + token.AccessToken
		}

		// Use shared streaming logic from Gemini
		return gemini.HandleGeminiResponsesStream(
			ctx,
			provider.streamingClient,
			completeURL,
			jsonData,
			headers,
			provider.networkConfig.ExtraHeaders,
			providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
			providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
			provider.GetProviderKey(),
			request.Model,
			postHookRunner,
			nil,
			provider.logger,
			postHookSpanFinalizer,
		)
	} else {
		ctx.SetValue(schemas.GatewayContextKeyIsResponsesToChatCompletionFallback, true)
		return provider.ChatCompletionStream(
			ctx,
			postHookRunner,
			postHookSpanFinalizer,
			key,
			request.ToChatRequest(),
		)
	}
}

// Embedding generates embeddings for the given input text(s) using Vertex AI.
// All Vertex AI embedding models use the same response format regardless of the model type.
// Returns a GatewayResponse containing the embedding(s) and any error that occurred.
func (provider *VertexProvider) Embedding(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayEmbeddingRequest) (*schemas.GatewayEmbeddingResponse, *schemas.GatewayError) {
	projectID := resolveVertexProjectID(ctx, key)
	if projectID == "" {
		return nil, providerUtils.NewConfigurationError("project ID is not set")
	}

	region := resolveVertexRegion(ctx, key)
	if region == "" {
		return nil, providerUtils.NewConfigurationError("region is not set in key config")
	}

	jsonBody, gatewayErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToVertexEmbeddingRequest(request), nil
		},
	)
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	// For custom/fine-tuned models, validate projectNumber is set
	projectNumber := resolveVertexProjectNumber(ctx, key)
	if schemas.IsAllDigitsASCII(request.Model) && projectNumber == "" {
		return nil, providerUtils.NewConfigurationError("project number is not set for fine-tuned models")
	}

	// Build the native Vertex embedding API endpoint
	authQuery := ""
	if key.Value.GetValue() != "" {
		authQuery = fmt.Sprintf("key=%s", url.QueryEscape(key.Value.GetValue()))
	}
	completeURL := getCompleteURLForGeminiEndpoint(request.Model, region, projectID, projectNumber, ":predict")

	// Create HTTP request for streaming
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	respOwned := true
	defer func() {
		if respOwned {
			fasthttp.ReleaseResponse(resp)
		}
	}()

	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")

	// Set any extra headers from network config
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	// If auth query is set, add it to the URL
	// Otherwise, get the oauth2 token and set the Authorization header
	if authQuery != "" {
		completeURL = fmt.Sprintf("%s?%s", completeURL, authQuery)
	} else {
		tokenSource, err := getAuthTokenSource(key)
		if err != nil {
			return nil, providerUtils.NewGatewayOperationError("error creating auth token source", err)
		}
		token, err := tokenSource.Token()
		if err != nil {
			return nil, providerUtils.NewGatewayOperationError("error getting token", err)
		}
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	}

	req.SetRequestURI(completeURL)

	usedLargePayloadBody := providerUtils.ApplyLargePayloadRequestBody(ctx, req)
	if !usedLargePayloadBody {
		req.SetBody(jsonBody)
	}

	// Make the request with optional large response streaming
	activeClient := providerUtils.PrepareResponseStreaming(ctx, provider.client, resp)
	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}
	if usedLargePayloadBody {
		providerUtils.DrainLargePayloadRemainder(ctx)
	}
	ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	if resp.StatusCode() != fasthttp.StatusOK {
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		// Remove client from pool for authentication/authorization errors
		if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
			removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
		}

		errBody := resp.Body()

		// Extract error message from Vertex's error format
		errorMessage := "Unknown error"
		if len(errBody) > 0 {
			// Try to parse Vertex's error format
			var vertexError map[string]interface{}
			if err := sonic.Unmarshal(errBody, &vertexError); err != nil {
				return nil, providerUtils.EnrichError(ctx, providerUtils.NewGatewayOperationError(schemas.ErrProviderResponseUnmarshal, err), jsonBody, errBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
			}

			if errorObj, exists := vertexError["error"]; exists {
				if errorMap, ok := errorObj.(map[string]interface{}); ok {
					if message, exists := errorMap["message"]; exists {
						if msgStr, ok := message.(string); ok {
							errorMessage = msgStr
						}
					}
				}
			}
		}

		return nil, providerUtils.EnrichError(ctx, providerUtils.NewProviderAPIError(errorMessage, nil, resp.StatusCode(), nil, nil), jsonBody, errBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	responseBody, isLargeResp, decodeErr := providerUtils.FinalizeResponseWithLargeDetection(ctx, resp, provider.logger)
	if decodeErr != nil {
		return nil, providerUtils.EnrichError(ctx, decodeErr, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse)
	}
	if isLargeResp {
		respOwned = false
		return &schemas.GatewayEmbeddingResponse{
			ExtraFields: schemas.GatewayResponseExtraFields{
				Latency:                 latency.Milliseconds(),
				ProviderResponseHeaders: providerUtils.ExtractProviderResponseHeaders(resp),
			},
		}, nil
	}

	// Parse Vertex's native embedding response using typed response
	var vertexResponse VertexEmbeddingResponse
	if err := sonic.Unmarshal(responseBody, &vertexResponse); err != nil {
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewGatewayOperationError(schemas.ErrProviderResponseUnmarshal, err), jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse)
	}

	// Use centralized Vertex converter
	gatewayResponse := vertexResponse.ToGatewayEmbeddingResponse()

	// Set ExtraFields
	gatewayResponse.ExtraFields.Latency = latency.Milliseconds()
	gatewayResponse.ExtraFields.ProviderResponseHeaders = providerUtils.ExtractProviderResponseHeaders(resp)

	// Set raw response if enabled
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		var rawResponseMap map[string]interface{}
		if err := sonic.Unmarshal(resp.Body(), &rawResponseMap); err != nil {
			return nil, providerUtils.EnrichError(ctx, providerUtils.NewGatewayOperationError(schemas.ErrProviderRawResponseUnmarshal, err), jsonBody, resp.Body(), provider.sendBackRawRequest, provider.sendBackRawResponse)
		}
		gatewayResponse.ExtraFields.RawResponse = rawResponseMap
	}

	return gatewayResponse, nil
}

// Speech is not supported by the Vertex provider.
func (provider *VertexProvider) Speech(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewaySpeechRequest) (*schemas.GatewaySpeechResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechRequest, provider.GetProviderKey())
}

// Rerank performs a rerank request using Vertex Discovery Engine ranking API.
func (provider *VertexProvider) Rerank(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayRerankRequest) (*schemas.GatewayRerankResponse, *schemas.GatewayError) {
	projectID := strings.TrimSpace(resolveVertexProjectID(ctx, key))
	if projectID == "" {
		return nil, providerUtils.NewConfigurationError("project ID is not set")
	}

	options, err := getVertexRerankOptions(projectID, request.Params)
	if err != nil {
		return nil, providerUtils.NewConfigurationError(err.Error())
	}

	jsonBody, gatewayErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToVertexRankRequest(request, options)
		},
	)
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	completeURL := fmt.Sprintf("https://discoveryengine.googleapis.com/v1/%s:rank", options.RankingConfig)

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	respOwned := true
	defer func() {
		if respOwned {
			fasthttp.ReleaseResponse(resp)
		}
	}()

	req.Header.SetMethod(http.MethodPost)
	req.SetRequestURI(completeURL)
	req.Header.SetContentType("application/json")
	req.Header.Set("X-Goog-User-Project", projectID)

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	tokenSource, err := getAuthTokenSource(key)
	if err != nil {
		return nil, providerUtils.NewGatewayOperationError("error creating auth token source", err)
	}
	token, err := tokenSource.Token()
	if err != nil {
		return nil, providerUtils.NewGatewayOperationError("error getting token", err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	usedLargePayloadBody := providerUtils.ApplyLargePayloadRequestBody(ctx, req)
	if !usedLargePayloadBody {
		req.SetBody(jsonBody)
	}

	// Make the request with optional large response streaming
	activeClient := providerUtils.PrepareResponseStreaming(ctx, provider.client, resp)
	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}
	if usedLargePayloadBody {
		providerUtils.DrainLargePayloadRemainder(ctx)
	}
	ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	if resp.StatusCode() != fasthttp.StatusOK {
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
			removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
		}

		errorMessage := parseDiscoveryEngineErrorMessage(resp.Body())
		parsedError := parseVertexError(resp)

		if strings.TrimSpace(errorMessage) != "" {
			shouldOverride := parsedError == nil ||
				parsedError.Error == nil ||
				strings.TrimSpace(parsedError.Error.Message) == "" ||
				parsedError.Error.Message == "Unknown error" ||
				parsedError.Error.Message == schemas.ErrProviderResponseUnmarshal

			if shouldOverride {
				parsedError = providerUtils.NewProviderAPIError(errorMessage, nil, resp.StatusCode(), nil, nil)
			}
		}

		return nil, providerUtils.EnrichError(ctx, parsedError, jsonBody, resp.Body(), provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	responseBody, isLargeResp, decodeErr := providerUtils.FinalizeResponseWithLargeDetection(ctx, resp, provider.logger)
	if decodeErr != nil {
		return nil, providerUtils.EnrichError(ctx, decodeErr, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}
	if isLargeResp {
		respOwned = false
		return &schemas.GatewayRerankResponse{
			Model: request.Model,
			ExtraFields: schemas.GatewayResponseExtraFields{
				Latency:                 latency.Milliseconds(),
				ProviderResponseHeaders: providerUtils.ExtractProviderResponseHeaders(resp),
			},
		}, nil
	}

	vertexResponse := &VertexRankResponse{}
	rawRequest, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(responseBody, vertexResponse, jsonBody, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if gatewayErr != nil {
		return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	returnDocuments := request.Params != nil && request.Params.ReturnDocuments != nil && *request.Params.ReturnDocuments
	gatewayResponse, err := vertexResponse.ToGatewayRerankResponse(request.Documents, returnDocuments)
	if err != nil {
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewGatewayOperationError("error converting rerank response", err), jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	gatewayResponse.ExtraFields.Latency = latency.Milliseconds()
	gatewayResponse.ExtraFields.ProviderResponseHeaders = providerUtils.ExtractProviderResponseHeaders(resp)

	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		gatewayResponse.ExtraFields.RawRequest = rawRequest
	}

	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		gatewayResponse.ExtraFields.RawResponse = rawResponse
	}

	return gatewayResponse, nil
}

// OCR is not supported by the Vertex provider.
func (provider *VertexProvider) OCR(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayOCRRequest) (*schemas.GatewayOCRResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.OCRRequest, provider.GetProviderKey())
}

// SpeechStream is not supported by the Vertex provider.
func (provider *VertexProvider) SpeechStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewaySpeechRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechStreamRequest, provider.GetProviderKey())
}

// Transcription is not supported by the Vertex provider.
func (provider *VertexProvider) Transcription(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayTranscriptionRequest) (*schemas.GatewayTranscriptionResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionRequest, provider.GetProviderKey())
}

// TranscriptionStream is not supported by the Vertex provider.
func (provider *VertexProvider) TranscriptionStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayTranscriptionRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionStreamRequest, provider.GetProviderKey())
}

func (provider *VertexProvider) ImageGeneration(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayImageGenerationRequest) (*schemas.GatewayImageGenerationResponse, *schemas.GatewayError) {
	// Validate model type before processing
	if !schemas.IsGeminiModelFamily(ctx, request.Model) && !schemas.IsAllDigitsASCII(request.Model) && !schemas.IsImagenModelFamily(ctx, request.Model) {
		return nil, providerUtils.NewConfigurationError(fmt.Sprintf("image generation is only supported for Gemini and Imagen models, got: %s", request.Model))
	}

	jsonBody, gatewayErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			var rawBody []byte
			var extraParams map[string]interface{}
			var err error

			if schemas.IsGeminiModelFamily(ctx, request.Model) || schemas.IsAllDigitsASCII(request.Model) {
				reqBody := gemini.ToGeminiImageGenerationRequest(request)
				if reqBody == nil {
					return nil, fmt.Errorf("image generation input is not provided")
				}
				extraParams = reqBody.GetExtraParams()
				// Strip unsupported fields for Vertex Gemini
				stripVertexGeminiUnsupportedFields(reqBody)
				// Marshal to JSON bytes, preserving key order
				rawBody, err = providerUtils.MarshalSorted(reqBody)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal request body: %w", err)
				}
			} else if schemas.IsImagenModelFamily(ctx, request.Model) {
				reqBody := gemini.ToImagenImageGenerationRequest(request)
				if reqBody == nil {
					return nil, fmt.Errorf("image generation input is not provided")
				}
				extraParams = reqBody.GetExtraParams()
				// Marshal to JSON bytes, preserving key order
				rawBody, err = providerUtils.MarshalSorted(reqBody)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal request body: %w", err)
				}
			}

			// Remove region field if present
			rawBody, err = providerUtils.DeleteJSONField(rawBody, "region")
			if err != nil {
				return nil, fmt.Errorf("failed to delete region field: %w", err)
			}
			return &VertexRawRequestBody{RawBody: rawBody, ExtraParams: extraParams}, nil
		},
	)
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	projectID := resolveVertexProjectID(ctx, key)
	if projectID == "" {
		return nil, providerUtils.NewConfigurationError("project ID is not set")
	}

	region := resolveVertexRegion(ctx, key)
	if region == "" {
		return nil, providerUtils.NewConfigurationError("region is not set in key config")
	}

	// Auth query is used for fine-tuned models to pass the API key in the query string
	authQuery := ""
	// Determine the URL based on model type
	var completeURL string
	if schemas.IsAllDigitsASCII(request.Model) {
		// Custom Fine-tuned models use OpenAPI endpoint
		projectNumber := resolveVertexProjectNumber(ctx, key)
		if projectNumber == "" {
			return nil, providerUtils.NewConfigurationError("project number is not set for fine-tuned models")
		}
		if value := key.Value.GetValue(); value != "" {
			authQuery = fmt.Sprintf("key=%s", url.QueryEscape(value))
		}
		completeURL = getVertexEndpointURL(region, "v1beta1", projectNumber, request.Model, ":generateContent")

	} else if schemas.IsImagenModelFamily(ctx, request.Model) {
		// Imagen models are published models, use publishers/google/models path
		if value := key.Value.GetValue(); value != "" {
			authQuery = fmt.Sprintf("key=%s", url.QueryEscape(value))
		}
		completeURL = getVertexPublisherModelURL(region, "v1", projectID, "google", gemini.NormalizeModelName(request.Model), ":predict")
	} else if schemas.IsGeminiModelFamily(ctx, request.Model) {
		if value := key.Value.GetValue(); value != "" {
			authQuery = fmt.Sprintf("key=%s", url.QueryEscape(value))
		}
		completeURL = getVertexPublisherModelURL(region, "v1", projectID, "google", gemini.NormalizeModelName(request.Model), ":generateContent")
	}

	// Create HTTP request for image generation
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	respOwned := true
	defer func() {
		if respOwned {
			fasthttp.ReleaseResponse(resp)
		}
	}()

	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	// If auth query is set, add it to the URL
	// Otherwise, get the oauth2 token and set the Authorization header
	if authQuery != "" {
		completeURL = fmt.Sprintf("%s?%s", completeURL, authQuery)
	} else {
		// Getting oauth2 token
		tokenSource, err := getAuthTokenSource(key)
		if err != nil {
			return nil, providerUtils.NewGatewayOperationError("error creating auth token source", err)
		}
		token, err := tokenSource.Token()
		if err != nil {
			return nil, providerUtils.NewGatewayOperationError("error getting token", err)
		}
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	}

	req.SetRequestURI(completeURL)
	usedLargePayloadBody := providerUtils.ApplyLargePayloadRequestBody(ctx, req)
	if !usedLargePayloadBody {
		req.SetBody(jsonBody)
	}

	// Make the request with optional large response streaming
	activeClient := providerUtils.PrepareResponseStreaming(ctx, provider.client, resp)
	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}
	if usedLargePayloadBody {
		providerUtils.DrainLargePayloadRemainder(ctx)
	}
	ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	if resp.StatusCode() != fasthttp.StatusOK {
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		// Remove client from pool for authentication/authorization errors
		if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
			removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
		}
		return nil, providerUtils.EnrichError(ctx, parseVertexError(resp), jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	responseBody, isLargeResp, decodeErr := providerUtils.FinalizeResponseWithLargeDetection(ctx, resp, provider.logger)
	if decodeErr != nil {
		return nil, providerUtils.EnrichError(ctx, decodeErr, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}
	if isLargeResp {
		respOwned = false
		return &schemas.GatewayImageGenerationResponse{
			ExtraFields: schemas.GatewayResponseExtraFields{
				Latency:                 latency.Milliseconds(),
				ProviderResponseHeaders: providerUtils.ExtractProviderResponseHeaders(resp),
			},
		}, nil
	}

	if schemas.IsGeminiModelFamily(ctx, request.Model) || schemas.IsAllDigitsASCII(request.Model) {
		geminiResponse := gemini.GenerateContentResponse{}

		rawRequest, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(responseBody, &geminiResponse, jsonBody, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
		if gatewayErr != nil {
			return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}

		response, err := geminiResponse.ToGatewayImageGenerationResponse()
		if err != nil {
			return nil, providerUtils.EnrichError(ctx, err, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}

		response.ExtraFields.Latency = latency.Milliseconds()
		response.ExtraFields.ProviderResponseHeaders = providerUtils.ExtractProviderResponseHeaders(resp)

		if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
			response.ExtraFields.RawRequest = rawRequest
		}

		if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
			response.ExtraFields.RawResponse = rawResponse
		}

		return response, nil
	} else {
		// Handle Imagen responses
		imagenResponse := gemini.GeminiImagenResponse{}

		rawRequest, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(responseBody, &imagenResponse, jsonBody, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
		if gatewayErr != nil {
			return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}

		response := imagenResponse.ToGatewayImageGenerationResponse()
		response.ExtraFields.Latency = latency.Milliseconds()
		response.ExtraFields.ProviderResponseHeaders = providerUtils.ExtractProviderResponseHeaders(resp)

		if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
			response.ExtraFields.RawRequest = rawRequest
		}

		if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
			response.ExtraFields.RawResponse = rawResponse
		}

		return response, nil
	}
}

// ImageGenerationStream is not supported by the Vertex provider.
func (provider *VertexProvider) ImageGenerationStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayImageGenerationRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationStreamRequest, provider.GetProviderKey())
}

// ImageEdit edits images for the given input text(s) using Vertex AI.
// Returns a GatewayResponse containing the images and any error that occurred.
func (provider *VertexProvider) ImageEdit(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayImageEditRequest) (*schemas.GatewayImageGenerationResponse, *schemas.GatewayError) {
	// Validate model type before processing
	if !schemas.IsGeminiModelFamily(ctx, request.Model) && !schemas.IsAllDigitsASCII(request.Model) && !schemas.IsImagenModelFamily(ctx, request.Model) {
		return nil, providerUtils.NewConfigurationError(fmt.Sprintf("image edit is only supported for Gemini and Imagen models, got: %s", request.Model))
	}

	jsonBody, gatewayErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			var rawBody []byte
			var extraParams map[string]interface{}
			var err error

			if schemas.IsGeminiModelFamily(ctx, request.Model) || schemas.IsAllDigitsASCII(request.Model) {
				reqBody := gemini.ToGeminiImageEditRequest(request)
				if reqBody == nil {
					return nil, fmt.Errorf("image edit input is not provided")
				}
				extraParams = reqBody.GetExtraParams()
				// Strip unsupported fields for Vertex Gemini
				stripVertexGeminiUnsupportedFields(reqBody)
				// Marshal to JSON bytes, preserving key order
				rawBody, err = providerUtils.MarshalSorted(reqBody)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal request body: %w", err)
				}
			} else if schemas.IsImagenModelFamily(ctx, request.Model) {
				reqBody := gemini.ToImagenImageEditRequest(request)
				if reqBody == nil {
					return nil, fmt.Errorf("image edit input is not provided")
				}
				extraParams = reqBody.GetExtraParams()
				// Marshal to JSON bytes, preserving key order
				rawBody, err = providerUtils.MarshalSorted(reqBody)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal request body: %w", err)
				}
			}

			// Remove region field if present
			rawBody, err = providerUtils.DeleteJSONField(rawBody, "region")
			if err != nil {
				return nil, fmt.Errorf("failed to delete region field: %w", err)
			}
			return &VertexRawRequestBody{RawBody: rawBody, ExtraParams: extraParams}, nil
		},
	)
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	projectID := resolveVertexProjectID(ctx, key)
	if projectID == "" {
		return nil, providerUtils.NewConfigurationError("project ID is not set")
	}

	region := resolveVertexRegion(ctx, key)
	if region == "" {
		return nil, providerUtils.NewConfigurationError("region is not set in key config")
	}

	authQuery := ""
	if value := key.Value.GetValue(); value != "" {
		authQuery = fmt.Sprintf("key=%s", url.QueryEscape(value))
	}

	var completeURL string
	if schemas.IsAllDigitsASCII(request.Model) {
		projectNumber := resolveVertexProjectNumber(ctx, key)
		if projectNumber == "" {
			return nil, providerUtils.NewConfigurationError("project number is not set for fine-tuned models")
		}
		completeURL = getVertexEndpointURL(region, "v1beta1", projectNumber, gemini.NormalizeModelName(request.Model), ":generateContent")
	} else if schemas.IsImagenModelFamily(ctx, request.Model) {
		completeURL = getVertexPublisherModelURL(region, "v1", projectID, "google", gemini.NormalizeModelName(request.Model), ":predict")
	} else if schemas.IsGeminiModelFamily(ctx, request.Model) {
		completeURL = getVertexPublisherModelURL(region, "v1", projectID, "google", gemini.NormalizeModelName(request.Model), ":generateContent")
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	respOwned := true
	defer func() {
		if respOwned {
			fasthttp.ReleaseResponse(resp)
		}
	}()

	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	// If auth query is set, add it to the URL
	// Otherwise, get the oauth2 token and set the Authorization header
	if authQuery != "" {
		completeURL = fmt.Sprintf("%s?%s", completeURL, authQuery)
	} else {
		// Getting oauth2 token
		tokenSource, err := getAuthTokenSource(key)
		if err != nil {
			return nil, providerUtils.NewGatewayOperationError("error creating auth token source", err)
		}
		token, err := tokenSource.Token()
		if err != nil {
			return nil, providerUtils.NewGatewayOperationError("error getting token", err)
		}
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	}

	req.SetRequestURI(completeURL)
	usedLargePayloadBody := providerUtils.ApplyLargePayloadRequestBody(ctx, req)
	if !usedLargePayloadBody {
		req.SetBody(jsonBody)
	}

	// Make the request with optional large response streaming
	activeClient := providerUtils.PrepareResponseStreaming(ctx, provider.client, resp)
	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}
	if usedLargePayloadBody {
		providerUtils.DrainLargePayloadRemainder(ctx)
	}
	ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	if resp.StatusCode() != fasthttp.StatusOK {
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
			removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
		}
		return nil, providerUtils.EnrichError(ctx, parseVertexError(resp), jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	responseBody, isLargeResp, decodeErr := providerUtils.FinalizeResponseWithLargeDetection(ctx, resp, provider.logger)
	if decodeErr != nil {
		return nil, providerUtils.EnrichError(ctx, decodeErr, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}
	if isLargeResp {
		respOwned = false
		return &schemas.GatewayImageGenerationResponse{
			ExtraFields: schemas.GatewayResponseExtraFields{
				Latency:                 latency.Milliseconds(),
				ProviderResponseHeaders: providerUtils.ExtractProviderResponseHeaders(resp),
			},
		}, nil
	}

	if schemas.IsGeminiModelFamily(ctx, request.Model) || schemas.IsAllDigitsASCII(request.Model) {
		geminiResponse := gemini.GenerateContentResponse{}

		rawRequest, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(responseBody, &geminiResponse, jsonBody, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
		if gatewayErr != nil {
			return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}

		response, err := geminiResponse.ToGatewayImageGenerationResponse()
		if err != nil {
			return nil, providerUtils.EnrichError(ctx, err, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}

		response.ExtraFields.Latency = latency.Milliseconds()
		response.ExtraFields.ProviderResponseHeaders = providerUtils.ExtractProviderResponseHeaders(resp)

		if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
			response.ExtraFields.RawRequest = rawRequest
		}

		if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
			response.ExtraFields.RawResponse = rawResponse
		}

		return response, nil
	} else {
		// Handle Imagen responses
		imagenResponse := gemini.GeminiImagenResponse{}

		rawRequest, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(responseBody, &imagenResponse, jsonBody, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
		if gatewayErr != nil {
			return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}

		response := imagenResponse.ToGatewayImageGenerationResponse()
		response.ExtraFields.Latency = latency.Milliseconds()
		response.ExtraFields.ProviderResponseHeaders = providerUtils.ExtractProviderResponseHeaders(resp)

		if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
			response.ExtraFields.RawRequest = rawRequest
		}
		if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
			response.ExtraFields.RawResponse = rawResponse
		}

		return response, nil
	}
}

// ImageEditStream is not supported by the Vertex provider.
func (provider *VertexProvider) ImageEditStream(ctx *schemas.GatewayContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.GatewayImageEditRequest) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditStreamRequest, provider.GetProviderKey())
}

// ImageVariation is not supported by the Vertex provider.
func (provider *VertexProvider) ImageVariation(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayImageVariationRequest) (*schemas.GatewayImageGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageVariationRequest, provider.GetProviderKey())
}

// VideoGeneration generates a video using Vertex AI's Gemini models.
// Only Gemini models support video generation in Vertex AI.
// Uses the predictLongRunning endpoint for async video generation.
func (provider *VertexProvider) VideoGeneration(ctx *schemas.GatewayContext, key schemas.Key, gatewayReq *schemas.GatewayVideoGenerationRequest) (*schemas.GatewayVideoGenerationResponse, *schemas.GatewayError) {
	providerName := provider.GetProviderKey()

	// Only Gemini models support video generation in Vertex
	if !schemas.IsVeoModelFamily(ctx, gatewayReq.Model) && !schemas.IsAllDigitsASCII(gatewayReq.Model) {
		return nil, providerUtils.NewConfigurationError(fmt.Sprintf("video generation is only supported for Veo models in Vertex, got: %s", gatewayReq.Model))
	}

	// Convert Gateway request to Gemini format (reusing Gemini converters)
	jsonData, gatewayErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		gatewayReq,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return gemini.ToGeminiVideoGenerationRequest(gatewayReq)
		},
	)
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	projectID := resolveVertexProjectID(ctx, key)
	if projectID == "" {
		return nil, providerUtils.NewConfigurationError("project ID is not set")
	}

	region := resolveVertexRegion(ctx, key)
	if region == "" {
		return nil, providerUtils.NewConfigurationError("region is not set in key config")
	}

	// Auth query is used to pass the API key in the query string
	authQuery := ""
	if key.Value.GetValue() != "" {
		authQuery = fmt.Sprintf("key=%s", url.QueryEscape(key.Value.GetValue()))
	}

	// For custom/fine-tuned models, validate projectNumber is set
	projectNumber := resolveVertexProjectNumber(ctx, key)
	if schemas.IsAllDigitsASCII(gatewayReq.Model) && projectNumber == "" {
		return nil, providerUtils.NewConfigurationError("project number is not set for fine-tuned models")
	}

	// Construct the URL for Gemini video generation using predictLongRunning
	completeURL := getCompleteURLForGeminiEndpoint(gatewayReq.Model, region, projectID, projectNumber, ":predictLongRunning")

	// Create HTTP request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	// If auth query is set, add it to the URL
	// Otherwise, get the oauth2 token and set the Authorization header
	if authQuery != "" {
		completeURL = fmt.Sprintf("%s?%s", completeURL, authQuery)
	} else {
		tokenSource, err := getAuthTokenSource(key)
		if err != nil {
			return nil, providerUtils.NewGatewayOperationError("error creating auth token source", err)
		}
		token, err := tokenSource.Token()
		if err != nil {
			return nil, providerUtils.NewGatewayOperationError("error getting token", err)
		}
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	}

	req.SetRequestURI(completeURL)
	req.SetBody(jsonData)

	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}
	ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
			removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
		}
		return nil, providerUtils.EnrichError(ctx, parseVertexError(resp), jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Parse response
	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewGatewayOperationError(schemas.ErrProviderResponseDecode, err)
	}

	var operation gemini.GenerateVideosOperation
	rawRequest, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(body, &operation, jsonData, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if gatewayErr != nil {
		return nil, gatewayErr
	}
	// Convert to Gateway response using Gemini converter
	gatewayResp, gatewayErr := gemini.ToGatewayVideoGenerationResponse(&operation, gatewayReq.Model)
	if gatewayErr != nil {
		return nil, gatewayErr
	}
	gatewayResp.ID = providerUtils.AddVideoIDProviderSuffix(gatewayResp.ID, providerName)

	gatewayResp.ExtraFields.Latency = latency.Milliseconds()
	gatewayResp.ExtraFields.ProviderResponseHeaders = providerUtils.ExtractProviderResponseHeaders(resp)

	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		gatewayResp.ExtraFields.RawRequest = rawRequest
	}
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		gatewayResp.ExtraFields.RawResponse = rawResponse
	}

	return gatewayResp, nil
}

// VideoRetrieve retrieves the status of a video generation operation.
// Uses the fetchPredictOperation endpoint for Vertex AI.
func (provider *VertexProvider) VideoRetrieve(ctx *schemas.GatewayContext, key schemas.Key, gatewayReq *schemas.GatewayVideoRetrieveRequest) (*schemas.GatewayVideoGenerationResponse, *schemas.GatewayError) {
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)
	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)

	region := resolveVertexRegion(ctx, key)
	if region == "" {
		return nil, providerUtils.NewConfigurationError("region is not set in key config")
	}

	baseURL := getVertexAPIBaseURL(region, "v1")

	// Construct the URL for fetching the operation status
	// The operation name (gatewayReq.ID) already contains the full path:
	// projects/PROJECT_ID/locations/REGION/publishers/google/models/MODEL_ID/operations/OPERATION_ID
	// We need to extract the model path from it to construct the fetchPredictOperation endpoint
	// Extract: projects/.../models/MODEL_ID from the operation name
	taskID := providerUtils.StripVideoIDProviderSuffix(gatewayReq.ID, provider.GetProviderKey())
	var modelPath string
	if idx := strings.Index(taskID, "/operations/"); idx != -1 {
		modelPath = taskID[:idx]
	} else {
		return nil, providerUtils.NewGatewayOperationError("invalid operation ID format", nil)
	}

	// Construct the URL: https://{vertex-api-host}/v1/{modelPath}:fetchPredictOperation
	completeURL := fmt.Sprintf("%s/%s:fetchPredictOperation", baseURL, modelPath)

	// Auth query is used to pass the API key in the query string
	authQuery := ""
	if key.Value.GetValue() != "" {
		authQuery = fmt.Sprintf("key=%s", url.QueryEscape(key.Value.GetValue()))
	}

	// Create request body with operation name (using sjson to avoid map marshaling)
	jsonBody, err := providerUtils.SetJSONField([]byte(`{}`), "operationName", taskID)
	if err != nil {
		return nil, providerUtils.NewGatewayOperationError("failed to marshal request", err)
	}

	// Create HTTP request
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	// If auth query is set, add it to the URL
	// Otherwise, get the oauth2 token and set the Authorization header
	if authQuery != "" {
		completeURL = fmt.Sprintf("%s?%s", completeURL, authQuery)
	} else {
		tokenSource, err := getAuthTokenSource(key)
		if err != nil {
			return nil, providerUtils.NewGatewayOperationError("error creating auth token source", err)
		}
		token, err := tokenSource.Token()
		if err != nil {
			return nil, providerUtils.NewGatewayOperationError("error getting token", err)
		}
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	}

	req.SetRequestURI(completeURL)
	req.SetBody(jsonBody)

	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}
	ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	// Handle error response
	if resp.StatusCode() != fasthttp.StatusOK {
		if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
			removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
		}
		return nil, providerUtils.EnrichError(ctx, parseVertexError(resp), jsonBody, nil, sendBackRawRequest, sendBackRawResponse, latency)
	}

	// Parse response
	var operation gemini.GenerateVideosOperation
	_, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(resp.Body(), &operation, jsonBody, sendBackRawRequest, sendBackRawResponse)
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	gatewayResp, gatewayErr := gemini.ToGatewayVideoGenerationResponse(&operation, "")
	if gatewayErr != nil {
		return nil, gatewayErr
	}
	gatewayResp.ID = providerUtils.AddVideoIDProviderSuffix(gatewayResp.ID, provider.GetProviderKey())
	gatewayResp.ExtraFields.Latency = latency.Milliseconds()
	gatewayResp.ExtraFields.ProviderResponseHeaders = providerUtils.ExtractProviderResponseHeaders(resp)

	if sendBackRawResponse {
		gatewayResp.ExtraFields.RawResponse = rawResponse
	}

	return gatewayResp, nil
}

// VideoDownload downloads the generated video content.
// First retrieves the video status to get the URL, then downloads the content.
// Handles both regular URLs and data URLs (base64-encoded videos).
func (provider *VertexProvider) VideoDownload(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayVideoDownloadRequest) (*schemas.GatewayVideoDownloadResponse, *schemas.GatewayError) {
	if request == nil || request.ID == "" {
		return nil, providerUtils.NewGatewayOperationError("video_id is required", nil)
	}
	// Retrieve operation first to get the video URL
	gatewayVideoRetrieveRequest := &schemas.GatewayVideoRetrieveRequest{
		Provider: request.Provider,
		ID:       request.ID,
	}
	videoResp, gatewayErr := provider.VideoRetrieve(ctx, key, gatewayVideoRetrieveRequest)
	if gatewayErr != nil {
		return nil, gatewayErr
	}
	if videoResp.Status != schemas.VideoStatusCompleted {
		return nil, providerUtils.NewGatewayOperationError(
			fmt.Sprintf("video not ready, current status: %s", videoResp.Status),
			nil)
	}
	if len(videoResp.Videos) == 0 {
		return nil, providerUtils.NewGatewayOperationError("video URL not available", nil)
	}
	var content []byte
	var latency time.Duration
	var providerResponseHeaders map[string]string
	contentType := "video/mp4"
	// Check if it's a data URL (base64-encoded video)
	if videoResp.Videos[0].Type == schemas.VideoOutputTypeBase64 && videoResp.Videos[0].Base64Data != nil {
		// Decode base64 content
		startTime := time.Now()
		decoded, err := base64.StdEncoding.DecodeString(*videoResp.Videos[0].Base64Data)
		if err != nil {
			return nil, providerUtils.NewGatewayOperationError("failed to decode base64 video data", err)
		}
		content = decoded
		contentType = videoResp.Videos[0].ContentType
		latency = time.Since(startTime)
	} else if videoResp.Videos[0].Type == schemas.VideoOutputTypeURL && videoResp.Videos[0].URL != nil {
		// Regular URL - fetch from HTTP endpoint
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()
		defer fasthttp.ReleaseRequest(req)
		defer fasthttp.ReleaseResponse(resp)
		providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
		req.SetRequestURI(*videoResp.Videos[0].URL)
		req.Header.SetMethod(http.MethodGet)
		// Add authentication for Vertex video downloads
		authQuery := ""
		if key.Value.GetValue() != "" {
			authQuery = fmt.Sprintf("key=%s", url.QueryEscape(key.Value.GetValue()))
		}
		if authQuery != "" {
			uri := *videoResp.Videos[0].URL
			if strings.Contains(uri, "?") {
				uri += "&" + authQuery
			} else {
				uri += "?" + authQuery
			}
			req.SetRequestURI(uri)
		} else {
			tokenSource, err := getAuthTokenSource(key)
			if err != nil {
				return nil, providerUtils.NewGatewayOperationError("error creating auth token source", err)
			}
			token, err := tokenSource.Token()
			if err != nil {
				return nil, providerUtils.NewGatewayOperationError("error getting token", err)
			}
			req.Header.Set("Authorization", "Bearer "+token.AccessToken)
		}
		var gatewayErr *schemas.GatewayError
		var wait func()
		latency, gatewayErr, wait = providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
		defer wait()
		if gatewayErr != nil {
			return nil, gatewayErr
		}
		ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))
		if resp.StatusCode() != fasthttp.StatusOK {
			return nil, providerUtils.SetErrorLatency(providerUtils.NewGatewayOperationError(
				fmt.Sprintf("failed to download video: HTTP %d", resp.StatusCode()),
				nil), latency)
		}
		body, err := providerUtils.CheckAndDecodeBody(resp)
		if err != nil {
			return nil, providerUtils.NewGatewayOperationError(schemas.ErrProviderResponseDecode, err)
		}
		contentType = string(resp.Header.ContentType())
		content = append([]byte(nil), body...)
		providerResponseHeaders = providerUtils.ExtractProviderResponseHeaders(resp)
	} else {
		return nil, providerUtils.NewGatewayOperationError("invalid video output type", nil)
	}

	gatewayResp := &schemas.GatewayVideoDownloadResponse{
		VideoID:     request.ID,
		Content:     content,
		ContentType: contentType,
	}

	gatewayResp.ExtraFields.Latency = latency.Milliseconds()
	gatewayResp.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	return gatewayResp, nil
}

// VideoDelete is not supported by the Vertex provider.
func (provider *VertexProvider) VideoDelete(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoDeleteRequest) (*schemas.GatewayVideoDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDeleteRequest, provider.GetProviderKey())
}

// VideoList is not supported by the Vertex provider.
func (provider *VertexProvider) VideoList(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoListRequest) (*schemas.GatewayVideoListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoListRequest, provider.GetProviderKey())
}

// VideoRemix is not supported by the Vertex provider.
func (provider *VertexProvider) VideoRemix(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayVideoRemixRequest) (*schemas.GatewayVideoGenerationResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRemixRequest, provider.GetProviderKey())
}

// stripVertexGeminiUnsupportedFields removes fields that are not supported by Vertex AI's Gemini API.
// Specifically, it removes the "id" field from function_call and function_response objects in contents.
func stripVertexGeminiUnsupportedFields(requestBody *gemini.GeminiGenerationRequest) {
	// Strip service tier — Vertex uses HTTP headers for this, not the request body.
	requestBody.ServiceTier = ""
	for _, content := range requestBody.Contents {
		for _, part := range content.Parts {
			// Remove id from function_call
			if part.FunctionCall != nil {
				part.FunctionCall.ID = ""
			}
			// Remove id from function_response
			if part.FunctionResponse != nil {
				part.FunctionResponse.ID = ""
			}
		}
	}
}

func stripVertexGeminiUnsupportedFieldsRaw(jsonBody []byte) []byte {
	if len(jsonBody) == 0 {
		return jsonBody
	}

	contents := gjson.GetBytes(jsonBody, "contents")
	if !contents.IsArray() {
		return jsonBody
	}

	out := jsonBody
	contentIndex := 0
	contents.ForEach(func(_, content gjson.Result) bool {
		parts := content.Get("parts")
		if !parts.IsArray() {
			contentIndex++
			return true
		}
		partIndex := 0
		parts.ForEach(func(_, part gjson.Result) bool {
			if part.Get("functionCall.id").Exists() {
				if updated, err := providerUtils.DeleteJSONField(out, fmt.Sprintf("contents.%d.parts.%d.functionCall.id", contentIndex, partIndex)); err == nil {
					out = updated
				}
			}
			if part.Get("functionResponse.id").Exists() {
				if updated, err := providerUtils.DeleteJSONField(out, fmt.Sprintf("contents.%d.parts.%d.functionResponse.id", contentIndex, partIndex)); err == nil {
					out = updated
				}
			}
			partIndex++
			return true
		})
		contentIndex++
		return true
	})

	// Strip top-level serviceTier — Vertex uses HTTP headers for this, not the request body.
	if providerUtils.JSONFieldExists(out, "serviceTier") {
		if updated, err := providerUtils.DeleteJSONField(out, "serviceTier"); err == nil {
			out = updated
		}
	}

	return out
}

// BatchCreate creates a Vertex AI batch prediction job.
//
// Input modes (mutually exclusive, mirroring the Gemini provider):
//   - InputFileID: a gs:// URI of an existing Vertex-format JSONL file.
//   - Requests: inline items converted to JSONL and uploaded to GCS via FileUpload.
//
// The output destination is taken from the typed output_folder.url (a gs:// prefix).
func (provider *VertexProvider) BatchCreate(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayBatchCreateRequest) (*schemas.GatewayBatchCreateResponse, *schemas.GatewayError) {
	baseURL, cfgErr := vertexBatchJobsBaseURL(key)
	if cfgErr != nil {
		return nil, cfgErr
	}

	rawBody, hasRawBody := providerUtils.CheckAndGetRawRequestBody(ctx, request)
	hasRawBody = hasRawBody && len(rawBody) > 0

	inputFileID := request.InputFileID
	jobName := ""
	outputURI := ""
	if !hasRawBody {
		if request.Model == nil || *request.Model == "" {
			return nil, providerUtils.NewGatewayOperationError("model is required for Vertex batch API", nil)
		}
		hasFileInput := request.InputFileID != ""
		hasInlineRequests := len(request.Requests) > 0
		if hasFileInput && hasInlineRequests {
			return nil, providerUtils.NewGatewayOperationError("cannot specify both input_file_id and requests", nil)
		}
		if !hasFileInput && !hasInlineRequests {
			return nil, providerUtils.NewGatewayOperationError("either input_file_id (gs:// JSONL URI) or requests is required for Vertex batch API", nil)
		}

		// Output destination is the typed output_folder.url (a gs:// prefix). Vertex writes
		// results into its own subdirectory under this prefix.
		if request.OutputFolder != nil {
			outputURI = strings.TrimSpace(request.OutputFolder.URL)
		}
		if outputURI == "" {
			return nil, providerUtils.NewGatewayOperationError("output_folder.url (gs:// prefix) is required for Vertex batch API", nil)
		}

		jobName = fmt.Sprintf("gateway-batch-%d", time.Now().Unix())
		if request.DisplayName != nil && *request.DisplayName != "" {
			jobName = *request.DisplayName
		} else if request.Metadata != nil {
			// Back-compat: OpenAI-compatible clients may pass the job name via metadata.
			if name, ok := request.Metadata["job_name"]; ok && name != "" {
				jobName = name
			}
		}

		// Inline mode: convert to JSONL and upload next to the output location (Bedrock pattern).
		if inputFileID == "" {
			jsonlData, err := vertexConvertRequestsToJSONL(request.Requests)
			if err != nil {
				return nil, providerUtils.NewGatewayOperationError("failed to convert requests to Vertex JSONL", err)
			}
			outBucket, outKey, parseErr := parseGCSURI(outputURI)
			if parseErr != nil {
				return nil, providerUtils.NewGatewayOperationError(parseErr.Error(), nil)
			}
			// Place the input alongside the output directory (sibling, not child) so the
			// generated JSONL does not live inside the directory Vertex writes results to.
			inputPrefix := "vertex-batches-input"
			if trimmed := strings.Trim(outKey, "/"); trimmed != "" {
				if idx := strings.LastIndexByte(trimmed, '/'); idx >= 0 {
					inputPrefix = trimmed[:idx] + "/input"
				} else {
					inputPrefix = "input"
				}
			}
			uploadResp, uploadErr := provider.FileUpload(ctx, key, &schemas.GatewayFileUploadRequest{
				Provider:    schemas.Vertex,
				File:        jsonlData,
				Filename:    jobName + "-input.jsonl",
				Purpose:     schemas.FilePurposeBatch,
				ContentType: schemas.Ptr("application/jsonl"),
				StorageConfig: &schemas.FileStorageConfig{
					GCS: &schemas.GCSStorageConfig{Bucket: outBucket, Prefix: inputPrefix},
				},
			})
			if uploadErr != nil {
				return nil, uploadErr
			}
			inputFileID = uploadResp.ID
		}
	}

	jsonData, bodyErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToVertexBatchCreateRequest(request, jobName, inputFileID, outputURI), nil
		},
	)
	if bodyErr != nil {
		return nil, bodyErr
	}

	authHeader, authErr := gcsGetAuthHeader(key)
	if authErr != nil {
		return nil, providerUtils.NewGatewayOperationError(authErr.Error(), nil)
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(baseURL + "/batchPredictionJobs")
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.Header.Set("Authorization", authHeader)
	req.SetBody(jsonData)

	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)

	startTime := time.Now()
	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	if resp.StatusCode() != fasthttp.StatusOK {
		if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
			removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
		}
		return nil, providerUtils.EnrichError(ctx, parseVertexJobAPIError(resp.Body(), resp.StatusCode(), "batch create"), jsonData, resp.Body(), provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	var created VertexBatchPredictionJob
	rawRequest, rawResponse, parseErr := providerUtils.HandleProviderResponse(resp.Body(), &created, jsonData, sendBackRawRequest, sendBackRawResponse)
	if parseErr != nil {
		return nil, providerUtils.EnrichError(ctx, parseErr, jsonData, resp.Body(), provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// In raw-passthrough mode inputFileID may be empty (e.g. BigQuery or multi-URI inputs the
	// typed path never sees); fall back to the GCS source the created job echoes back.
	if inputFileID == "" && created.InputConfig.GcsSource != nil && len(created.InputConfig.GcsSource.Uris) > 0 {
		inputFileID = created.InputConfig.GcsSource.Uris[0]
	}

	result := &schemas.GatewayBatchCreateResponse{
		ID:          created.Name,
		Object:      "batch",
		InputFileID: inputFileID,
		Status:      vertexJobStateToBatchStatus(created.State),
		CreatedAt:   gcsParseTime(created.CreateTime),
		Metadata:    request.Metadata,
		ExtraFields: schemas.GatewayResponseExtraFields{
			Latency: time.Since(startTime).Milliseconds(),
		},
	}
	if created.DisplayName != "" {
		result.DisplayName = schemas.Ptr(created.DisplayName)
	}
	if sendBackRawRequest {
		result.ExtraFields.RawRequest = rawRequest
	}
	if sendBackRawResponse {
		result.ExtraFields.RawResponse = rawResponse
	}
	return result, nil
}

// BatchList lists Vertex AI batch prediction jobs across all keys, paginating one key
// at a time. Each Vertex key carries its own project/region, and batch jobs are scoped to
// that project/region, so the serial helper walks every key (exhausting all of its pages
// before advancing) to avoid hiding jobs created under any key but the first.
func (provider *VertexProvider) BatchList(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayBatchListRequest) (*schemas.GatewayBatchListResponse, *schemas.GatewayError) {
	if len(keys) == 0 {
		return nil, providerUtils.NewGatewayOperationError("no keys provided for Vertex BatchList", nil)
	}

	// The OpenAI-compatible /v1/batches route feeds the cursor back via After.
	helper, err := providerUtils.NewSerialListHelper(keys, request.After, provider.logger, true)
	if err != nil {
		return nil, providerUtils.NewGatewayOperationError("invalid pagination cursor", err)
	}

	key, nativeCursor, ok := helper.GetCurrentKey()
	if !ok {
		// All keys exhausted.
		return &schemas.GatewayBatchListResponse{
			Object: "list",
			Data:   []schemas.GatewayBatchRetrieveResponse{},
		}, nil
	}

	// Query the current key with its native Vertex page token.
	modifiedRequest := *request
	if nativeCursor != "" {
		modifiedRequest.PageToken = &nativeCursor
	} else {
		modifiedRequest.PageToken = nil
	}

	resp, latency, gatewayErr := provider.batchListByKey(ctx, key, &modifiedRequest)
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	nativeNextCursor := ""
	if resp.NextCursor != nil {
		nativeNextCursor = *resp.NextCursor
	}
	nextCursor, hasMore := helper.BuildNextCursor(resp.HasMore, nativeNextCursor)

	resp.HasMore = hasMore
	if nextCursor != "" {
		resp.NextCursor = &nextCursor
	} else {
		resp.NextCursor = nil
	}
	resp.ExtraFields.Latency = latency.Milliseconds()
	return resp, nil
}

// batchListByKey lists batch prediction jobs for a single Vertex key/project/region.
// The native Vertex page token (if any) is taken from request.PageToken; the returned
// NextCursor carries Vertex's nextPageToken verbatim for the caller to re-encode.
func (provider *VertexProvider) batchListByKey(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayBatchListRequest) (*schemas.GatewayBatchListResponse, time.Duration, *schemas.GatewayError) {
	baseURL, cfgErr := vertexBatchJobsBaseURL(key)
	if cfgErr != nil {
		return nil, 0, cfgErr
	}

	params := url.Values{}
	pageSize := request.PageSize
	if pageSize <= 0 {
		pageSize = request.Limit
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	params.Set("pageSize", fmt.Sprintf("%d", pageSize))
	if request.PageToken != nil && *request.PageToken != "" {
		params.Set("pageToken", *request.PageToken)
	}

	authHeader, authErr := gcsGetAuthHeader(key)
	if authErr != nil {
		return nil, 0, providerUtils.NewGatewayOperationError(authErr.Error(), nil)
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(baseURL + "/batchPredictionJobs?" + params.Encode())
	req.Header.SetMethod(http.MethodGet)
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.Header.Set("Authorization", authHeader)

	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)

	startTime := time.Now()
	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, 0, providerUtils.EnrichError(ctx, gatewayErr, nil, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	if resp.StatusCode() != fasthttp.StatusOK {
		if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
			removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
		}
		return nil, 0, providerUtils.EnrichError(ctx, parseVertexJobAPIError(resp.Body(), resp.StatusCode(), "batch list"), nil, resp.Body(), provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// GET request: no request body, so raw request capture is skipped by HandleProviderResponse.
	var listResp VertexBatchJobListResponse
	_, rawResponse, parseErr := providerUtils.HandleProviderResponse(resp.Body(), &listResp, nil, false, sendBackRawResponse)
	if parseErr != nil {
		return nil, 0, providerUtils.EnrichError(ctx, parseErr, nil, resp.Body(), provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	data := make([]schemas.GatewayBatchRetrieveResponse, 0, len(listResp.BatchPredictionJobs))
	for i := range listResp.BatchPredictionJobs {
		data = append(data, vertexBatchJobToGateway(&listResp.BatchPredictionJobs[i]))
	}

	var nextCursor *string
	if listResp.NextPageToken != "" {
		nextCursor = &listResp.NextPageToken
	}

	result := &schemas.GatewayBatchListResponse{
		Object:     "list",
		Data:       data,
		HasMore:    listResp.NextPageToken != "",
		NextCursor: nextCursor,
	}
	if sendBackRawResponse {
		result.ExtraFields.RawResponse = rawResponse
	}
	return result, time.Since(startTime), nil
}

// BatchRetrieve fetches a Vertex AI batch prediction job by ID (bare or full resource name).
func (provider *VertexProvider) BatchRetrieve(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayBatchRetrieveRequest) (*schemas.GatewayBatchRetrieveResponse, *schemas.GatewayError) {
	if len(keys) == 0 {
		return nil, providerUtils.NewGatewayOperationError("no keys provided for Vertex BatchRetrieve", nil)
	}

	// A job ID is scoped to the project/region of the key that created it, so try each key
	// until one resolves the job; return the last error only if all keys fail.
	var lastErr *schemas.GatewayError
	for _, key := range keys {
		startTime := time.Now()
		job, rawResponse, gatewayErr := provider.vertexGetBatchJob(ctx, key, request.BatchID)
		if gatewayErr != nil {
			lastErr = gatewayErr
			continue
		}

		result := vertexBatchJobToGateway(job)
		result.ExtraFields = schemas.GatewayResponseExtraFields{
			Latency: time.Since(startTime).Milliseconds(),
		}
		if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
			result.ExtraFields.RawResponse = rawResponse
		}
		return &result, nil
	}

	return nil, lastErr
}

// vertexGetBatchJob fetches a BatchPredictionJob resource. The returned rawResponse is
// the raw response payload when raw-response capture is enabled (nil otherwise); it is a
// GET, so there is no raw request to capture.
func (provider *VertexProvider) vertexGetBatchJob(ctx *schemas.GatewayContext, key schemas.Key, batchID string) (*VertexBatchPredictionJob, interface{}, *schemas.GatewayError) {
	jobURL, cfgErr := vertexBatchJobURL(key, batchID)
	if cfgErr != nil {
		return nil, nil, cfgErr
	}

	authHeader, authErr := gcsGetAuthHeader(key)
	if authErr != nil {
		return nil, nil, providerUtils.NewGatewayOperationError(authErr.Error(), nil)
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(jobURL)
	req.Header.SetMethod(http.MethodGet)
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.Header.Set("Authorization", authHeader)

	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, nil, providerUtils.EnrichError(ctx, gatewayErr, nil, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	if resp.StatusCode() != fasthttp.StatusOK {
		if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
			removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
		}
		return nil, nil, providerUtils.EnrichError(ctx, parseVertexJobAPIError(resp.Body(), resp.StatusCode(), "batch retrieve"), nil, resp.Body(), provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	var job VertexBatchPredictionJob
	_, rawResponse, parseErr := providerUtils.HandleProviderResponse(resp.Body(), &job, nil, false, providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if parseErr != nil {
		return nil, nil, providerUtils.EnrichError(ctx, parseErr, nil, resp.Body(), provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}
	return &job, rawResponse, nil
}

// BatchCancel cancels a running Vertex AI batch prediction job.
func (provider *VertexProvider) BatchCancel(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayBatchCancelRequest) (*schemas.GatewayBatchCancelResponse, *schemas.GatewayError) {
	if len(keys) == 0 {
		return nil, providerUtils.NewGatewayOperationError("no keys provided for Vertex BatchCancel", nil)
	}

	// A job ID is scoped to the project/region of the key that created it, so try each key
	// until the cancel succeeds; return the last error only if all keys fail.
	var lastErr *schemas.GatewayError
	for _, key := range keys {
		resp, gatewayErr := provider.batchCancelByKey(ctx, key, request)
		if gatewayErr == nil {
			return resp, nil
		}
		lastErr = gatewayErr
	}
	return nil, lastErr
}

// batchCancelByKey cancels a batch prediction job using a single Vertex key.
func (provider *VertexProvider) batchCancelByKey(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayBatchCancelRequest) (*schemas.GatewayBatchCancelResponse, *schemas.GatewayError) {
	jobURL, cfgErr := vertexBatchJobURL(key, request.BatchID)
	if cfgErr != nil {
		return nil, cfgErr
	}

	authHeader, authErr := gcsGetAuthHeader(key)
	if authErr != nil {
		return nil, providerUtils.NewGatewayOperationError(authErr.Error(), nil)
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(jobURL + ":cancel")
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.Header.Set("Authorization", authHeader)

	startTime := time.Now()
	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, providerUtils.EnrichError(ctx, gatewayErr, nil, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	if resp.StatusCode() != fasthttp.StatusOK {
		if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
			removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
		}
		return nil, providerUtils.EnrichError(ctx, parseVertexJobAPIError(resp.Body(), resp.StatusCode(), "batch cancel"), nil, resp.Body(), provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	return &schemas.GatewayBatchCancelResponse{
		// Echo the caller's id so it stays stable across create/retrieve/cancel.
		ID:           request.BatchID,
		Object:       "batch",
		Status:       schemas.BatchStatusCancelling,
		CancellingAt: schemas.Ptr(startTime.Unix()),
		ExtraFields: schemas.GatewayResponseExtraFields{
			Latency: time.Since(startTime).Milliseconds(),
		},
	}, nil
}

// BatchDelete deletes a finished Vertex AI batch prediction job.
func (provider *VertexProvider) BatchDelete(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayBatchDeleteRequest) (*schemas.GatewayBatchDeleteResponse, *schemas.GatewayError) {
	if len(keys) == 0 {
		return nil, providerUtils.NewGatewayOperationError("no keys provided for Vertex BatchDelete", nil)
	}

	// A job ID is scoped to the project/region of the key that created it, so try each key
	// until the delete succeeds; return the last error only if all keys fail.
	var lastErr *schemas.GatewayError
	for _, key := range keys {
		resp, gatewayErr := provider.batchDeleteByKey(ctx, key, request)
		if gatewayErr == nil {
			return resp, nil
		}
		lastErr = gatewayErr
	}
	return nil, lastErr
}

// batchDeleteByKey deletes a batch prediction job using a single Vertex key.
func (provider *VertexProvider) batchDeleteByKey(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayBatchDeleteRequest) (*schemas.GatewayBatchDeleteResponse, *schemas.GatewayError) {
	jobURL, cfgErr := vertexBatchJobURL(key, request.BatchID)
	if cfgErr != nil {
		return nil, cfgErr
	}

	authHeader, authErr := gcsGetAuthHeader(key)
	if authErr != nil {
		return nil, providerUtils.NewGatewayOperationError(authErr.Error(), nil)
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(jobURL)
	req.Header.SetMethod(http.MethodDelete)
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.Header.Set("Authorization", authHeader)

	startTime := time.Now()
	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, providerUtils.EnrichError(ctx, gatewayErr, nil, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	if resp.StatusCode() != fasthttp.StatusOK {
		if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
			removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
		}
		return nil, providerUtils.EnrichError(ctx, parseVertexJobAPIError(resp.Body(), resp.StatusCode(), "batch delete"), nil, resp.Body(), provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	return &schemas.GatewayBatchDeleteResponse{
		// Echo the caller's id so it stays stable across create/retrieve/delete.
		ID:     request.BatchID,
		Object: "batch",
		Status: schemas.BatchStatusDeleted,
		ExtraFields: schemas.GatewayResponseExtraFields{
			Latency: time.Since(startTime).Milliseconds(),
		},
	}, nil
}

// BatchResults reads the predictions-*.jsonl files a finished job wrote to its GCS
// output directory and maps each line to a Gateway batch result item. The custom_id
// is recovered from the echoed request labels.
func (provider *VertexProvider) BatchResults(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayBatchResultsRequest) (*schemas.GatewayBatchResultsResponse, *schemas.GatewayError) {
	if len(keys) == 0 {
		return nil, providerUtils.NewGatewayOperationError("no keys provided for Vertex BatchResults", nil)
	}

	// A job ID is scoped to the project/region of the key that created it, so try each key
	// until one resolves the job and reads its results; return the last error if all fail.
	var lastErr *schemas.GatewayError
	for _, key := range keys {
		resp, gatewayErr := provider.batchResultsByKey(ctx, key, request)
		if gatewayErr == nil {
			return resp, nil
		}
		lastErr = gatewayErr
	}
	return nil, lastErr
}

// batchResultsByKey reads a finished job's GCS output using a single Vertex key.
func (provider *VertexProvider) batchResultsByKey(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayBatchResultsRequest) (*schemas.GatewayBatchResultsResponse, *schemas.GatewayError) {
	startTime := time.Now()
	job, _, gatewayErr := provider.vertexGetBatchJob(ctx, key, request.BatchID)
	if gatewayErr != nil {
		return nil, gatewayErr
	}
	if job.OutputInfo == nil || job.OutputInfo.GcsOutputDirectory == "" {
		return nil, providerUtils.NewGatewayOperationError(fmt.Sprintf("batch output is not available yet (job state: %s)", job.State), nil)
	}

	bucket, dirKey, parseErr := parseGCSURI(job.OutputInfo.GcsOutputDirectory)
	if parseErr != nil {
		return nil, providerUtils.NewGatewayOperationError(parseErr.Error(), nil)
	}

	authHeader, authErr := gcsGetAuthHeader(key)
	if authErr != nil {
		return nil, providerUtils.NewGatewayOperationError(authErr.Error(), nil)
	}

	objects, listErr := provider.gcsListAllObjects(ctx, authHeader, bucket, strings.Trim(dirKey, "/")+"/")
	if listErr != nil {
		return nil, listErr
	}

	results := []schemas.BatchResultItem{}
	for _, obj := range objects {
		name := obj.Name
		if idx := strings.LastIndexByte(name, '/'); idx >= 0 {
			name = name[idx+1:]
		}
		if !strings.HasPrefix(name, "predictions") {
			continue
		}

		content, downloadErr := provider.gcsDownloadObject(ctx, authHeader, bucket, obj.Name)
		if downloadErr != nil {
			return nil, downloadErr
		}

		for _, rawLine := range bytes.Split(content, []byte("\n")) {
			if len(bytes.TrimSpace(rawLine)) == 0 {
				continue
			}
			var line VertexBatchOutputLine
			if err := sonic.Unmarshal(rawLine, &line); err != nil {
				continue // skip malformed lines rather than failing the whole result set
			}
			item := schemas.BatchResultItem{
				CustomID: line.Request.Labels[vertexBatchCustomIDLabel],
			}
			if line.Response != nil {
				item.Response = &schemas.BatchResultResponse{
					StatusCode: 200,
					Body:       line.Response,
				}
			} else {
				item.Error = &schemas.BatchResultError{Message: line.Status}
			}
			results = append(results, item)
		}
	}

	return &schemas.GatewayBatchResultsResponse{
		BatchID: request.BatchID,
		Results: results,
		ExtraFields: schemas.GatewayResponseExtraFields{
			Latency: time.Since(startTime).Milliseconds(),
		},
	}, nil
}

// gcsListAllObjects lists every object under a prefix, following pagination.
func (provider *VertexProvider) gcsListAllObjects(ctx *schemas.GatewayContext, authHeader, bucket, prefix string) ([]gcsObjectMetadata, *schemas.GatewayError) {
	var objects []gcsObjectMetadata
	pageToken := ""
	for {
		params := url.Values{}
		params.Set("prefix", prefix)
		params.Set("maxResults", "1000")
		if pageToken != "" {
			params.Set("pageToken", pageToken)
		}

		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()

		req.SetRequestURI(fmt.Sprintf("%s/b/%s/o?%s", gcsStorageBase, url.PathEscape(bucket), params.Encode()))
		req.Header.SetMethod(http.MethodGet)
		providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
		req.Header.Set("Authorization", authHeader)

		_, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
		if gatewayErr != nil {
			wait()
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			return nil, gatewayErr
		}

		statusCode := resp.StatusCode()
		var listResp gcsObjectListResponse
		var unmarshalErr error
		if statusCode == fasthttp.StatusOK {
			unmarshalErr = sonic.Unmarshal(resp.Body(), &listResp)
		}
		var apiErr *schemas.GatewayError
		if statusCode != fasthttp.StatusOK {
			apiErr = parseGCSAPIError(resp.Body(), statusCode, "list")
		}
		wait()
		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(resp)

		if apiErr != nil {
			return nil, apiErr
		}
		if unmarshalErr != nil {
			return nil, providerUtils.NewGatewayOperationError("failed to parse GCS list response", unmarshalErr)
		}

		objects = append(objects, listResp.Items...)
		if listResp.NextPageToken == "" {
			return objects, nil
		}
		pageToken = listResp.NextPageToken
	}
}

// gcsDownloadObject downloads the raw bytes of a GCS object.
func (provider *VertexProvider) gcsDownloadObject(ctx *schemas.GatewayContext, authHeader, bucket, objectKey string) ([]byte, *schemas.GatewayError) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(fmt.Sprintf("%s/b/%s/o/%s?alt=media", gcsStorageBase, url.PathEscape(bucket), gcsEncodeObjectName(objectKey)))
	req.Header.SetMethod(http.MethodGet)
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.Header.Set("Authorization", authHeader)

	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.SetErrorLatency(parseGCSAPIError(resp.Body(), resp.StatusCode(), "content download"), latency)
	}

	content := make([]byte, len(resp.Body()))
	copy(content, resp.Body())
	return content, nil
}

const (
	gcsStorageBase = "https://storage.googleapis.com/storage/v1"
	gcsUploadBase  = "https://storage.googleapis.com/upload/storage/v1"
)

// --- GCS helpers ---

func gcsObjectKey(prefix, filename string) string {
	key := "vertex-files/" + uuid.New().String() + "/" + filename
	if prefix != "" {
		key = strings.Trim(prefix, "/") + "/" + key
	}
	return key
}

// gcsEncodeObjectName percent-encodes a GCS object name for the URL path,
// encoding slashes as %2F (url.PathEscape preserves them).
func gcsEncodeObjectName(name string) string {
	return strings.ReplaceAll(url.PathEscape(name), "/", "%2F")
}

func parseGCSURI(uri string) (bucket, objectKey string, err error) {
	if !strings.HasPrefix(uri, "gs://") {
		return "", "", fmt.Errorf("invalid GCS URI %q: must start with gs://", uri)
	}
	rest := strings.TrimPrefix(uri, "gs://")
	idx := strings.IndexByte(rest, '/')
	if idx < 0 {
		return rest, "", nil
	}
	return rest[:idx], rest[idx+1:], nil
}

func gcsParseTime(s string) int64 {
	if s == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return 0
	}
	return t.Unix()
}

func gcsParseSize(s string) int64 {
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n
}

func gcsMetadataToFileObject(bucket string, obj gcsObjectMetadata) schemas.FileObject {
	filename := obj.Metadata["gateway_filename"]
	if filename == "" {
		// Fall back to last path segment of the object key.
		if idx := strings.LastIndexByte(obj.Name, '/'); idx >= 0 {
			filename = obj.Name[idx+1:]
		} else {
			filename = obj.Name
		}
	}
	return schemas.FileObject{
		ID:        "gs://" + bucket + "/" + obj.Name,
		Object:    "file",
		Bytes:     gcsParseSize(obj.Size),
		CreatedAt: gcsParseTime(obj.TimeCreated),
		UpdatedAt: gcsParseTime(obj.Updated),
		Filename:  filename,
		Purpose:   schemas.FilePurpose(obj.Metadata["gateway_purpose"]),
		Status:    schemas.FileStatusProcessed,
	}
}

func gcsGetAuthHeader(key schemas.Key) (string, error) {
	tokenSrc, err := getAuthTokenSource(key)
	if err != nil {
		return "", fmt.Errorf("failed to get GCS auth token source: %w", err)
	}
	tok, err := tokenSrc.Token()
	if err != nil {
		removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
		return "", fmt.Errorf("failed to acquire GCS access token: %w", err)
	}
	return "Bearer " + tok.AccessToken, nil
}

func parseGCSAPIError(body []byte, statusCode int, op string) *schemas.GatewayError {
	var gcsErr gcsErrorBody
	_ = sonic.Unmarshal(body, &gcsErr)
	msg := gcsErr.Error.Message
	if msg == "" {
		msg = fmt.Sprintf("GCS %s failed with HTTP %d", op, statusCode)
	}
	return providerUtils.NewProviderAPIError(msg, nil, statusCode, nil, nil)
}

// FileUpload uploads a file to GCS for use with Vertex AI inference.
//
// Two modes based on whether file bytes are provided:
//   - Direct (request.File non-empty): uploads bytes via GCS multipart upload.
//   - Resumable (request.File empty): mints a GCS resumable upload session URL.
//     The client uploads bytes directly to GCS; Gateway stays out of the data path.
func (provider *VertexProvider) FileUpload(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayFileUploadRequest) (*schemas.GatewayFileUploadResponse, *schemas.GatewayError) {
	var bucket, prefix string
	if request.StorageConfig != nil && request.StorageConfig.GCS != nil {
		bucket = request.StorageConfig.GCS.Bucket
		prefix = request.StorageConfig.GCS.Prefix
	}
	if bucket == "" {
		return nil, providerUtils.NewGatewayOperationError("gcs_bucket is required for Vertex FileUpload (provide in storage_config.gcs)", nil)
	}

	filename := request.Filename
	if filename == "" {
		filename = "file-" + uuid.New().String()
	}

	objectKey := gcsObjectKey(prefix, filename)
	gcsURI := "gs://" + bucket + "/" + objectKey

	contentType := "application/octet-stream"
	if request.ContentType != nil && *request.ContentType != "" {
		contentType = *request.ContentType
	}

	authHeader, authErr := gcsGetAuthHeader(key)
	if authErr != nil {
		return nil, providerUtils.NewGatewayOperationError(authErr.Error(), nil)
	}

	gcsMeta := map[string]string{
		"gateway_filename":     filename,
		"gateway_purpose":      string(request.Purpose),
		"gateway_content_type": contentType,
	}

	// GCS object metadata JSON, shared by both upload modes (multipart part 1
	// for direct uploads, session body for resumable uploads).
	metaJSON, err := sonic.Marshal(map[string]interface{}{
		"name":        objectKey,
		"contentType": contentType,
		"metadata":    gcsMeta,
	})
	if err != nil {
		return nil, providerUtils.NewGatewayOperationError("failed to marshal GCS object metadata", err)
	}

	startTime := time.Now()

	if len(request.File) == 0 {
		return provider.gcsFileUploadResumable(ctx, key, authHeader, bucket, contentType, gcsURI, metaJSON, request, filename, startTime)
	}
	return provider.gcsFileUploadDirect(ctx, key, authHeader, bucket, contentType, gcsURI, metaJSON, request, filename, startTime)
}

func (provider *VertexProvider) gcsFileUploadDirect(
	ctx *schemas.GatewayContext,
	key schemas.Key,
	authHeader, bucket, contentType, gcsURI string,
	metaJSON []byte,
	request *schemas.GatewayFileUploadRequest,
	filename string,
	startTime time.Time,
) (*schemas.GatewayFileUploadResponse, *schemas.GatewayError) {
	// Build GCS multipart/related body: part 1 = JSON object metadata, part 2 = file bytes.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	metaPartHeader := textproto.MIMEHeader{}
	metaPartHeader.Set("Content-Type", "application/json; charset=UTF-8")
	metaPart, err := mw.CreatePart(metaPartHeader)
	if err != nil {
		return nil, providerUtils.NewGatewayOperationError("failed to create GCS metadata part", err)
	}
	if _, err := metaPart.Write(metaJSON); err != nil {
		return nil, providerUtils.NewGatewayOperationError("failed to write GCS metadata part", err)
	}

	filePartHeader := textproto.MIMEHeader{}
	filePartHeader.Set("Content-Type", contentType)
	filePart, err := mw.CreatePart(filePartHeader)
	if err != nil {
		return nil, providerUtils.NewGatewayOperationError("failed to create GCS file part", err)
	}
	if _, err := filePart.Write(request.File); err != nil {
		return nil, providerUtils.NewGatewayOperationError("failed to write file bytes", err)
	}
	if err := mw.Close(); err != nil {
		return nil, providerUtils.NewGatewayOperationError("failed to finalise GCS multipart body", err)
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(fmt.Sprintf("%s/b/%s/o?uploadType=multipart", gcsUploadBase, url.PathEscape(bucket)))
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("multipart/related; boundary=" + mw.Boundary())
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.Header.Set("Authorization", authHeader)
	req.SetBody(buf.Bytes())

	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	if resp.StatusCode() != fasthttp.StatusOK && resp.StatusCode() != fasthttp.StatusCreated {
		if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
			removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
		}
		return nil, providerUtils.SetErrorLatency(parseGCSAPIError(resp.Body(), resp.StatusCode(), "upload"), latency)
	}

	return &schemas.GatewayFileUploadResponse{
		ID:             gcsURI,
		Object:         "file",
		Bytes:          int64(len(request.File)),
		CreatedAt:      startTime.Unix(),
		Filename:       filename,
		Purpose:        request.Purpose,
		Status:         schemas.FileStatusProcessed,
		StorageBackend: schemas.FileStorageGCS,
		StorageURI:     gcsURI,
		ExtraFields: schemas.GatewayResponseExtraFields{
			Latency:                 time.Since(startTime).Milliseconds(),
			ProviderResponseHeaders: providerUtils.ExtractProviderResponseHeaders(resp),
		},
	}, nil
}

func (provider *VertexProvider) gcsFileUploadResumable(
	ctx *schemas.GatewayContext,
	key schemas.Key,
	authHeader, bucket, contentType, gcsURI string,
	metaJSON []byte,
	request *schemas.GatewayFileUploadRequest,
	filename string,
	startTime time.Time,
) (*schemas.GatewayFileUploadResponse, *schemas.GatewayError) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(fmt.Sprintf("%s/b/%s/o?uploadType=resumable", gcsUploadBase, url.PathEscape(bucket)))
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json; charset=UTF-8")
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("X-Upload-Content-Type", contentType)
	req.SetBody(metaJSON)

	// content_length is optional but helps GCS validate the upload size. Depending
	// on the transport it may arrive as a JSON number (float64) or a form-field
	// string, so accept both numeric and string forms.
	if request.ExtraParams != nil {
		var contentLength int64
		switch cl := request.ExtraParams["content_length"].(type) {
		case float64:
			contentLength = int64(cl)
		case int:
			contentLength = int64(cl)
		case int64:
			contentLength = cl
		case string:
			if parsed, err := strconv.ParseInt(strings.TrimSpace(cl), 10, 64); err == nil {
				contentLength = parsed
			}
		}
		if contentLength > 0 {
			req.Header.Set("X-Upload-Content-Length", fmt.Sprintf("%d", contentLength))
		}
	}

	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	if resp.StatusCode() != fasthttp.StatusOK {
		if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
			removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
		}
		return nil, providerUtils.SetErrorLatency(parseGCSAPIError(resp.Body(), resp.StatusCode(), "resumable session initiation"), latency)
	}

	sessionURL := string(resp.Header.Peek("Location"))
	if sessionURL == "" {
		return nil, providerUtils.NewGatewayOperationError("GCS did not return a Location header for the resumable session", nil)
	}

	return &schemas.GatewayFileUploadResponse{
		ID:             gcsURI,
		Object:         "file",
		Bytes:          0,
		CreatedAt:      startTime.Unix(),
		Filename:       filename,
		Purpose:        request.Purpose,
		Status:         schemas.FileStatusPendingUpload,
		StorageBackend: schemas.FileStorageGCS,
		StorageURI:     gcsURI,
		UploadURL:      &sessionURL,
		ExtraFields: schemas.GatewayResponseExtraFields{
			Latency:                 time.Since(startTime).Milliseconds(),
			ProviderResponseHeaders: providerUtils.ExtractProviderResponseHeaders(resp),
		},
	}, nil
}

// FileList lists GCS objects under the configured prefix.
// Bucket must be provided via storage_config.gcs.
// Pagination is serial across keys: each key's GCS pages are exhausted (via the
// native pageToken) before moving to the next key.
func (provider *VertexProvider) FileList(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayFileListRequest) (*schemas.GatewayFileListResponse, *schemas.GatewayError) {
	if len(keys) == 0 {
		return nil, providerUtils.NewGatewayOperationError("no keys provided for Vertex FileList", nil)
	}
	var bucket, prefix string
	if request.StorageConfig != nil && request.StorageConfig.GCS != nil {
		bucket = request.StorageConfig.GCS.Bucket
		prefix = request.StorageConfig.GCS.Prefix
	}
	if bucket == "" {
		return nil, providerUtils.NewGatewayOperationError("gcs_bucket is required for Vertex FileList (provide in storage_config.gcs)", nil)
	}

	// Serial pagination across keys: exhaust one key's pages before moving to the next.
	helper, err := providerUtils.NewSerialListHelper(keys, request.After, provider.logger, true)
	if err != nil {
		return nil, providerUtils.NewGatewayOperationError("invalid pagination cursor", err)
	}

	key, nativeCursor, ok := helper.GetCurrentKey()
	if !ok {
		// All keys exhausted
		return &schemas.GatewayFileListResponse{
			Object:  "list",
			Data:    []schemas.FileObject{},
			HasMore: false,
		}, nil
	}

	authHeader, authErr := gcsGetAuthHeader(key)
	if authErr != nil {
		return nil, providerUtils.NewGatewayOperationError(authErr.Error(), nil)
	}

	params := url.Values{}
	if prefix != "" {
		params.Set("prefix", prefix)
	}
	limit := request.Limit
	if limit <= 0 {
		limit = 20
	}
	params.Set("maxResults", fmt.Sprintf("%d", limit))
	if nativeCursor != "" {
		params.Set("pageToken", nativeCursor)
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(fmt.Sprintf("%s/b/%s/o?%s", gcsStorageBase, url.PathEscape(bucket), params.Encode()))
	req.Header.SetMethod(http.MethodGet)
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.Header.Set("Authorization", authHeader)

	startTime := time.Now()
	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	if resp.StatusCode() != fasthttp.StatusOK {
		if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
			removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
		}
		return nil, providerUtils.SetErrorLatency(parseGCSAPIError(resp.Body(), resp.StatusCode(), "list"), latency)
	}

	var listResp gcsObjectListResponse
	if err := sonic.Unmarshal(resp.Body(), &listResp); err != nil {
		return nil, providerUtils.NewGatewayOperationError("failed to parse GCS list response", err)
	}

	files := make([]schemas.FileObject, 0, len(listResp.Items))
	for _, item := range listResp.Items {
		files = append(files, gcsMetadataToFileObject(bucket, item))
	}

	// Build cursor for next request: stay on this key while it has more pages,
	// then advance to the next key.
	nextCursor, hasMore := helper.BuildNextCursor(listResp.NextPageToken != "", listResp.NextPageToken)

	result := &schemas.GatewayFileListResponse{
		Object:  "list",
		Data:    files,
		HasMore: hasMore,
		ExtraFields: schemas.GatewayResponseExtraFields{
			Latency: time.Since(startTime).Milliseconds(),
		},
	}
	if nextCursor != "" {
		result.After = &nextCursor
	}
	return result, nil
}

// FileRetrieve fetches GCS object metadata, trying each key until one succeeds.
// FileID must be a gs:// URI.
func (provider *VertexProvider) FileRetrieve(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayFileRetrieveRequest) (*schemas.GatewayFileRetrieveResponse, *schemas.GatewayError) {
	if len(keys) == 0 {
		return nil, providerUtils.NewGatewayOperationError("no keys provided for Vertex FileRetrieve", nil)
	}

	var lastErr *schemas.GatewayError
	for _, key := range keys {
		resp, err := provider.fileRetrieveByKey(ctx, key, request)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		provider.logger.Debug("Vertex FileRetrieve failed for key %s: %v", key.Name, err.Error)
	}
	return nil, lastErr
}

// fileRetrieveByKey fetches GCS object metadata for a single key.
func (provider *VertexProvider) fileRetrieveByKey(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayFileRetrieveRequest) (*schemas.GatewayFileRetrieveResponse, *schemas.GatewayError) {
	bucket, objectKey, parseErr := parseGCSURI(request.FileID)
	if parseErr != nil {
		return nil, providerUtils.NewGatewayOperationError(parseErr.Error(), nil)
	}

	authHeader, authErr := gcsGetAuthHeader(key)
	if authErr != nil {
		return nil, providerUtils.NewGatewayOperationError(authErr.Error(), nil)
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(fmt.Sprintf("%s/b/%s/o/%s", gcsStorageBase, url.PathEscape(bucket), gcsEncodeObjectName(objectKey)))
	req.Header.SetMethod(http.MethodGet)
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.Header.Set("Authorization", authHeader)

	startTime := time.Now()
	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	if resp.StatusCode() != fasthttp.StatusOK {
		if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
			removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
		}
		return nil, providerUtils.SetErrorLatency(parseGCSAPIError(resp.Body(), resp.StatusCode(), "retrieve"), latency)
	}

	var obj gcsObjectMetadata
	if err := sonic.Unmarshal(resp.Body(), &obj); err != nil {
		return nil, providerUtils.NewGatewayOperationError("failed to parse GCS object metadata", err)
	}

	filename := obj.Metadata["gateway_filename"]
	if filename == "" {
		if idx := strings.LastIndexByte(obj.Name, '/'); idx >= 0 {
			filename = obj.Name[idx+1:]
		} else {
			filename = obj.Name
		}
	}

	return &schemas.GatewayFileRetrieveResponse{
		ID:             request.FileID,
		Object:         "file",
		Bytes:          gcsParseSize(obj.Size),
		CreatedAt:      gcsParseTime(obj.TimeCreated),
		UpdatedAt:      gcsParseTime(obj.Updated),
		Filename:       filename,
		Purpose:        schemas.FilePurpose(obj.Metadata["gateway_purpose"]),
		Status:         schemas.FileStatusProcessed,
		StorageBackend: schemas.FileStorageGCS,
		StorageURI:     request.FileID,
		ExtraFields: schemas.GatewayResponseExtraFields{
			Latency: time.Since(startTime).Milliseconds(),
		},
	}, nil
}

// FileDelete deletes a GCS object, trying each key until one succeeds.
// FileID must be a gs:// URI. Deleting a non-existent object is treated as
// success (idempotent).
func (provider *VertexProvider) FileDelete(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayFileDeleteRequest) (*schemas.GatewayFileDeleteResponse, *schemas.GatewayError) {
	if len(keys) == 0 {
		return nil, providerUtils.NewGatewayOperationError("no keys provided for Vertex FileDelete", nil)
	}

	var lastErr *schemas.GatewayError
	for _, key := range keys {
		resp, err := provider.fileDeleteByKey(ctx, key, request)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		provider.logger.Debug("Vertex FileDelete failed for key %s: %v", key.Name, err.Error)
	}
	return nil, lastErr
}

// fileDeleteByKey deletes a GCS object for a single key.
func (provider *VertexProvider) fileDeleteByKey(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayFileDeleteRequest) (*schemas.GatewayFileDeleteResponse, *schemas.GatewayError) {
	bucket, objectKey, parseErr := parseGCSURI(request.FileID)
	if parseErr != nil {
		return nil, providerUtils.NewGatewayOperationError(parseErr.Error(), nil)
	}

	authHeader, authErr := gcsGetAuthHeader(key)
	if authErr != nil {
		return nil, providerUtils.NewGatewayOperationError(authErr.Error(), nil)
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(fmt.Sprintf("%s/b/%s/o/%s", gcsStorageBase, url.PathEscape(bucket), gcsEncodeObjectName(objectKey)))
	req.Header.SetMethod(http.MethodDelete)
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.Header.Set("Authorization", authHeader)

	startTime := time.Now()
	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	// 204 = deleted; 404 = already gone — both succeed for an idempotent delete.
	if resp.StatusCode() != fasthttp.StatusNoContent && resp.StatusCode() != fasthttp.StatusNotFound {
		if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
			removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
		}
		return nil, providerUtils.SetErrorLatency(parseGCSAPIError(resp.Body(), resp.StatusCode(), "delete"), latency)
	}

	return &schemas.GatewayFileDeleteResponse{
		ID:      request.FileID,
		Object:  "file",
		Deleted: true,
		ExtraFields: schemas.GatewayResponseExtraFields{
			Latency: time.Since(startTime).Milliseconds(),
		},
	}, nil
}

// FileContent downloads the raw bytes of a GCS object, trying each key until one
// succeeds. FileID must be a gs:// URI.
func (provider *VertexProvider) FileContent(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayFileContentRequest) (*schemas.GatewayFileContentResponse, *schemas.GatewayError) {
	if len(keys) == 0 {
		return nil, providerUtils.NewGatewayOperationError("no keys provided for Vertex FileContent", nil)
	}

	var lastErr *schemas.GatewayError
	for _, key := range keys {
		resp, err := provider.fileContentByKey(ctx, key, request)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		provider.logger.Debug("Vertex FileContent failed for key %s: %v", key.Name, err.Error)
	}
	return nil, lastErr
}

// fileContentByKey downloads the raw bytes of a GCS object for a single key.
func (provider *VertexProvider) fileContentByKey(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayFileContentRequest) (*schemas.GatewayFileContentResponse, *schemas.GatewayError) {
	bucket, objectKey, parseErr := parseGCSURI(request.FileID)
	if parseErr != nil {
		return nil, providerUtils.NewGatewayOperationError(parseErr.Error(), nil)
	}

	authHeader, authErr := gcsGetAuthHeader(key)
	if authErr != nil {
		return nil, providerUtils.NewGatewayOperationError(authErr.Error(), nil)
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(fmt.Sprintf("%s/b/%s/o/%s?alt=media", gcsStorageBase, url.PathEscape(bucket), gcsEncodeObjectName(objectKey)))
	req.Header.SetMethod(http.MethodGet)
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.Header.Set("Authorization", authHeader)

	startTime := time.Now()
	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	if resp.StatusCode() != fasthttp.StatusOK {
		if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
			removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
		}
		return nil, providerUtils.SetErrorLatency(parseGCSAPIError(resp.Body(), resp.StatusCode(), "content download"), latency)
	}

	// Copy body before deferred ReleaseResponse invalidates the buffer.
	content := make([]byte, len(resp.Body()))
	copy(content, resp.Body())

	contentType := string(resp.Header.Peek("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return &schemas.GatewayFileContentResponse{
		FileID:      request.FileID,
		Content:     content,
		ContentType: contentType,
		ExtraFields: schemas.GatewayResponseExtraFields{
			Latency: time.Since(startTime).Milliseconds(),
		},
	}, nil
}

// CountTokens counts the number of tokens in the provided content using Vertex AI's countTokens endpoint.
// Supports Gemini models with both text and image content.
func (provider *VertexProvider) CountTokens(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayResponsesRequest) (*schemas.GatewayCountTokensResponse, *schemas.GatewayError) {
	var (
		jsonBody   []byte
		gatewayErr *schemas.GatewayError
	)

	if schemas.IsAnthropicModelFamily(ctx, request.Model) {
		// Anthropic-on-Vertex doesn't accept URL-source document blocks.
		// Inline any URL documents to base64 before the converter runs.
		if err := inlineDocumentURLsResponses(ctx, request); err != nil {
			return nil, providerUtils.NewGatewayOperationError("failed to inline document URLs for vertex/claude", err)
		}
		jsonBody, gatewayErr = anthropic.BuildAnthropicResponsesRequestBody(ctx, request, anthropic.AnthropicRequestBuildConfig{
			Provider:                  schemas.Vertex,
			Model:                     request.Model,
			IsCountTokens:             true,
			BetaHeaderOverrides:       provider.networkConfig.BetaHeaderOverrides,
			ProviderExtraHeaders:      provider.networkConfig.ExtraHeaders,
			ValidateTools:             true,
			ShouldSendBackRawRequest:  provider.sendBackRawRequest,
			ShouldSendBackRawResponse: provider.sendBackRawResponse,
		})
		if gatewayErr != nil {
			return nil, gatewayErr
		}
	} else {
		jsonBody, gatewayErr = providerUtils.CheckContextAndGetRequestBody(
			ctx,
			request,
			func() (providerUtils.RequestBodyWithExtraParams, error) {
				return gemini.ToGeminiResponsesRequestWithImageURLSchemes(ctx, request, geminiImageURLSchemes...)
			},
		)
		if gatewayErr != nil {
			return nil, gatewayErr
		}

		if rawBody, ok := ctx.Value(schemas.GatewayContextKeyUseRawRequestBody).(bool); ok && rawBody {
			jsonBody = gemini.NormalizeRawGenerateContentRequestForCompatibility(jsonBody)
		}

		// Skip field-stripping when large payload mode is active — jsonBody is nil
		// and the raw body will stream directly from the ingress reader.
		if jsonBody != nil {
			// Use sjson to delete fields directly from JSON bytes, preserving key ordering
			jsonBody, _ = providerUtils.DeleteJSONField(jsonBody, "toolConfig")
			jsonBody, _ = providerUtils.DeleteJSONField(jsonBody, "generationConfig")
			jsonBody, _ = providerUtils.DeleteJSONField(jsonBody, "systemInstruction")
		}
	}

	projectID := resolveVertexProjectID(ctx, key)
	if projectID == "" {
		return nil, providerUtils.NewConfigurationError("project ID is not set")
	}

	region := resolveVertexRegion(ctx, key)
	if region == "" {
		return nil, providerUtils.NewConfigurationError("region is not set in key config")
	}

	authQuery := ""
	var completeURL string

	if schemas.IsAnthropicModelFamily(ctx, request.Model) {
		// Use model-aware host based on request.Model, but URL path uses "count-tokens"
		effectiveRegion := getVertexEffectiveRegion(region, request.Model)
		baseURL := getVertexModelAwareAPIBaseURL(region, "v1", request.Model)
		completeURL = fmt.Sprintf("%s/projects/%s/locations/%s/publishers/%s/models/%s%s", baseURL, projectID, effectiveRegion, "anthropic", "count-tokens", ":rawPredict")
	} else if schemas.IsGeminiModelFamily(ctx, request.Model) || schemas.IsAllDigitsASCII(request.Model) || schemas.IsGemmaModelFamily(ctx, request.Model) {
		if key.Value.GetValue() != "" {
			authQuery = fmt.Sprintf("key=%s", url.QueryEscape(key.Value.GetValue()))
		}

		projectNumber := resolveVertexProjectNumber(ctx, key)
		if schemas.IsAllDigitsASCII(request.Model) && projectNumber == "" {
			return nil, providerUtils.NewConfigurationError("project number is not set for fine-tuned models")
		}

		completeURL = getCompleteURLForGeminiEndpoint(request.Model, region, projectID, projectNumber, ":countTokens")
	}

	if completeURL == "" {
		return nil, providerUtils.NewConfigurationError(fmt.Sprintf("count tokens is not supported for model: %s", request.Model))
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	respOwned := true
	defer func() {
		if respOwned {
			fasthttp.ReleaseResponse(resp)
		}
	}()

	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, []string{anthropic.AnthropicBetaHeader})

	if authQuery != "" {
		completeURL = fmt.Sprintf("%s?%s", completeURL, authQuery)
	} else {
		tokenSource, err := getAuthTokenSource(key)
		if err != nil {
			return nil, providerUtils.NewGatewayOperationError("error creating auth token source", err)
		}
		token, err := tokenSource.Token()
		if err != nil {
			return nil, providerUtils.NewGatewayOperationError("error getting token", err)
		}
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	}

	req.SetRequestURI(completeURL)
	usedLargePayloadBody := providerUtils.ApplyLargePayloadRequestBody(ctx, req)
	if !usedLargePayloadBody {
		req.SetBody(jsonBody)
	}

	// Make the request with optional large response streaming
	activeClient := providerUtils.PrepareResponseStreaming(ctx, provider.client, resp)
	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse)
	}
	if usedLargePayloadBody {
		providerUtils.DrainLargePayloadRemainder(ctx)
	}
	ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeaders(resp))

	if resp.StatusCode() != fasthttp.StatusOK {
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
			removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
		}
		return nil, providerUtils.EnrichError(ctx, parseVertexError(resp), jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	responseBody, isLargeResp, decodeErr := providerUtils.FinalizeResponseWithLargeDetection(ctx, resp, provider.logger)
	if decodeErr != nil {
		return nil, providerUtils.EnrichError(ctx, decodeErr, jsonBody, nil, provider.sendBackRawRequest, provider.sendBackRawResponse)
	}
	if isLargeResp {
		respOwned = false
		return &schemas.GatewayCountTokensResponse{
			ExtraFields: schemas.GatewayResponseExtraFields{
				Latency:                 latency.Milliseconds(),
				ProviderResponseHeaders: providerUtils.ExtractProviderResponseHeaders(resp),
			},
		}, nil
	}

	if schemas.IsAnthropicModelFamily(ctx, request.Model) {
		anthropicResponse := &anthropic.AnthropicCountTokensResponse{}

		rawRequest, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(responseBody, anthropicResponse, jsonBody, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
		if gatewayErr != nil {
			return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse)
		}

		response := anthropicResponse.ToGatewayCountTokensResponse(request.Model)
		response.ExtraFields.Latency = latency.Milliseconds()
		response.ExtraFields.ProviderResponseHeaders = providerUtils.ExtractProviderResponseHeaders(resp)

		if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
			response.ExtraFields.RawRequest = rawRequest
		}

		if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
			response.ExtraFields.RawResponse = rawResponse
		}

		return response, nil
	}

	vertexResponse := VertexCountTokensResponse{}

	rawRequest, rawResponse, gatewayErr := providerUtils.HandleProviderResponse(responseBody, &vertexResponse, jsonBody, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if gatewayErr != nil {
		return nil, providerUtils.EnrichError(ctx, gatewayErr, jsonBody, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse)
	}

	response := vertexResponse.ToGatewayCountTokensResponse(request.Model)
	response.ExtraFields.Latency = latency.Milliseconds()
	response.ExtraFields.ProviderResponseHeaders = providerUtils.ExtractProviderResponseHeaders(resp)

	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		response.ExtraFields.RawRequest = rawRequest
	}

	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		response.ExtraFields.RawResponse = rawResponse
	}

	return response, nil
}

// Compaction is not supported by the Vertex provider.
func (provider *VertexProvider) Compaction(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayCompactionRequest) (*schemas.GatewayCompactionResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CompactionRequest, provider.GetProviderKey())
}

// ContainerCreate is not supported by the Vertex provider.
func (provider *VertexProvider) ContainerCreate(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayContainerCreateRequest) (*schemas.GatewayContainerCreateResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerCreateRequest, provider.GetProviderKey())
}

// ContainerList is not supported by the Vertex provider.
func (provider *VertexProvider) ContainerList(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerListRequest) (*schemas.GatewayContainerListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerListRequest, provider.GetProviderKey())
}

// ContainerRetrieve is not supported by the Vertex provider.
func (provider *VertexProvider) ContainerRetrieve(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerRetrieveRequest) (*schemas.GatewayContainerRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerRetrieveRequest, provider.GetProviderKey())
}

// ContainerDelete is not supported by the Vertex provider.
func (provider *VertexProvider) ContainerDelete(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerDeleteRequest) (*schemas.GatewayContainerDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerDeleteRequest, provider.GetProviderKey())
}

// ContainerFileCreate is not supported by the Vertex provider.
func (provider *VertexProvider) ContainerFileCreate(_ *schemas.GatewayContext, _ schemas.Key, _ *schemas.GatewayContainerFileCreateRequest) (*schemas.GatewayContainerFileCreateResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileCreateRequest, provider.GetProviderKey())
}

// ContainerFileList is not supported by the Vertex provider.
func (provider *VertexProvider) ContainerFileList(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileListRequest) (*schemas.GatewayContainerFileListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileListRequest, provider.GetProviderKey())
}

// ContainerFileRetrieve is not supported by the Vertex provider.
func (provider *VertexProvider) ContainerFileRetrieve(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileRetrieveRequest) (*schemas.GatewayContainerFileRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileRetrieveRequest, provider.GetProviderKey())
}

// ContainerFileContent is not supported by the Vertex provider.
func (provider *VertexProvider) ContainerFileContent(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileContentRequest) (*schemas.GatewayContainerFileContentResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileContentRequest, provider.GetProviderKey())
}

// ContainerFileDelete is not supported by the Vertex provider.
func (provider *VertexProvider) ContainerFileDelete(_ *schemas.GatewayContext, _ []schemas.Key, _ *schemas.GatewayContainerFileDeleteRequest) (*schemas.GatewayContainerFileDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileDeleteRequest, provider.GetProviderKey())
}

func (provider *VertexProvider) Passthrough(
	ctx *schemas.GatewayContext,
	key schemas.Key,
	req *schemas.GatewayPassthroughRequest,
) (*schemas.GatewayPassthroughResponse, *schemas.GatewayError) {
	projectID := strings.TrimSpace(resolveVertexProjectID(ctx, key))
	if projectID == "" {
		return nil, providerUtils.NewConfigurationError("project ID is not set")
	}

	keyRegion := resolveVertexRegion(ctx, key)
	if keyRegion == "" {
		keyRegion = "global"
	}

	baseURL := getVertexAPIBaseURL(keyRegion, "v1")

	// Normalize path: remove leading /v1 or /v1/ to avoid duplicate version segments (e.g. /v1/v1/...)
	path := req.Path
	for strings.HasPrefix(path, "/v1/") || path == "/v1" {
		path = strings.TrimPrefix(path, "/v1/")
		path = strings.TrimPrefix(path, "/v1")
		if path != "" && !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
	}

	// Replace region in path with key's configured region (client path may have different region)
	if strings.Contains(path, "/locations/") {
		path = vertexLocationsPathRe.ReplaceAllString(path, "/locations/"+keyRegion)
		if strings.Contains(path, "/projects/") {
			path = vertexProjectsPathRe.ReplaceAllString(path, "/projects/"+projectID)
		}
	} else {
		// add projects/%s/locations/%s/publishers/google to path
		path = fmt.Sprintf("/projects/%s/locations/%s%s", projectID, keyRegion, path)
	}

	requestURL := baseURL + path
	if req.RawQuery != "" {
		requestURL += "?" + req.RawQuery
	}

	// Only use API key for Google publisher endpoints; Anthropic/Mistral/OpenAPI-style paths require OAuth.
	authQuery := ""
	if key.Value.GetValue() != "" && strings.Contains(path, "publishers/google") {
		authQuery = fmt.Sprintf("key=%s", url.QueryEscape(key.Value.GetValue()))
	}

	// Prepare fasthttp request
	fasthttpReq := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)
	defer fasthttp.ReleaseRequest(fasthttpReq)

	fasthttpReq.Header.SetMethod(req.Method)

	// If auth query is set, add it to the URL; otherwise use OAuth2
	if authQuery != "" {
		if strings.Contains(requestURL, "?") {
			requestURL += "&" + authQuery
		} else {
			requestURL += "?" + authQuery
		}
	} else {
		tokenSource, err := getAuthTokenSource(key)
		if err != nil {
			removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
			return nil, providerUtils.NewGatewayOperationError("error creating auth token source", err)
		}
		token, err := tokenSource.Token()
		if err != nil {
			removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
			return nil, providerUtils.NewGatewayOperationError("error getting token", err)
		}
		fasthttpReq.Header.Set("Authorization", "Bearer "+token.AccessToken)
	}

	fasthttpReq.SetRequestURI(requestURL)

	// Set extra headers from provider network config
	providerUtils.SetExtraHeaders(ctx, fasthttpReq, provider.networkConfig.ExtraHeaders, nil)

	// Set safe headers from client request
	for k, v := range req.SafeHeaders {
		if strings.EqualFold(k, "authorization") || strings.EqualFold(k, "proxy-authorization") {
			continue
		}
		fasthttpReq.Header.Set(k, v)
	}

	if len(req.Body) > 0 && strings.Contains(strings.ToLower(string(fasthttpReq.Header.ContentType())), "application/json") {
		region := keyRegion
		// Replace fully-qualified model paths that have placeholder project/location
		// e.g. "projects/None/locations/None/publishers/..." -> "projects/real-id/locations/real-region/..."
		body := req.Body
		bodyStr := vertexBodyProjectsRe.ReplaceAllString(string(body), "${1}projects/"+projectID)
		bodyStr = vertexLocationsPathRe.ReplaceAllString(bodyStr, "/locations/"+region)
		// Expand short-form model names: "models/X" -> "projects/P/locations/L/publishers/google/models/X"
		bodyStr = vertexShortModelRe.ReplaceAllString(bodyStr,
			fmt.Sprintf(`"projects/%s/locations/%s/publishers/google/$1"`, projectID, keyRegion))
		fasthttpReq.SetBodyString(bodyStr)
	} else if len(req.Body) > 0 {
		fasthttpReq.SetBody(req.Body)
	}

	// Execute request
	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, fasthttpReq, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	// Remove client from pool for authentication/authorization errors
	if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
		removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
	}

	headers := providerUtils.ExtractPassthroughProviderResponseHeaders(resp)
	ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, headers)

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewGatewayOperationError("failed to decode response body", err)
	}

	var passthroughUsage *schemas.GatewayPassthroughUsage
	if resp.StatusCode() >= 200 && resp.StatusCode() < 300 {
		passthroughUsage = gemini.ExtractGeminiPassthroughUsage(req.Path, req.Body, body)
	}

	gatewayResponse := &schemas.GatewayPassthroughResponse{
		StatusCode: resp.StatusCode(),
		Headers:    headers,
		Body:       body,
		ExtraFields: schemas.GatewayResponseExtraFields{
			Latency:                 latency.Milliseconds(),
			ProviderResponseHeaders: headers,
			PassthroughPath:         req.Path,
			RawRequest:              req.Body,
		},
		PassthroughUsage: passthroughUsage,
	}

	return gatewayResponse, nil
}

func (provider *VertexProvider) PassthroughStream(
	ctx *schemas.GatewayContext,
	postHookRunner schemas.PostHookRunner,
	postHookSpanFinalizer func(context.Context),
	key schemas.Key,
	req *schemas.GatewayPassthroughRequest,
) (chan *schemas.GatewayStreamChunk, *schemas.GatewayError) {
	projectID := strings.TrimSpace(resolveVertexProjectID(ctx, key))
	if projectID == "" {
		return nil, providerUtils.NewConfigurationError("project ID is not set")
	}

	keyRegion := resolveVertexRegion(ctx, key)
	if keyRegion == "" {
		keyRegion = "global"
	}

	baseURL := getVertexAPIBaseURL(keyRegion, "v1")

	// Normalize path: remove leading /v1 or /v1/ to avoid duplicate version segments.
	path := req.Path
	for strings.HasPrefix(path, "/v1/") || path == "/v1" {
		path = strings.TrimPrefix(path, "/v1/")
		path = strings.TrimPrefix(path, "/v1")
		if path != "" && !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
	}

	// Replace region and project in path with key's configured values.
	if strings.Contains(path, "/locations/") {
		path = vertexLocationsPathRe.ReplaceAllString(path, "/locations/"+keyRegion)
		if strings.Contains(path, "/projects/") {
			path = vertexProjectsPathRe.ReplaceAllString(path, "/projects/"+projectID)
		}
	} else {
		path = fmt.Sprintf("/projects/%s/locations/%s%s", projectID, keyRegion, path)
	}

	requestURL := baseURL + path
	if req.RawQuery != "" {
		requestURL += "?" + req.RawQuery
	}

	fasthttpReq := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	resp.StreamBody = true
	defer fasthttp.ReleaseRequest(fasthttpReq)

	fasthttpReq.Header.SetMethod(req.Method)

	// Only use API key for Google publisher endpoints; Anthropic/Mistral/OpenAPI-style paths require OAuth.
	authQuery := ""
	if key.Value.GetValue() != "" && strings.Contains(path, "publishers/google") {
		authQuery = fmt.Sprintf("key=%s", url.QueryEscape(key.Value.GetValue()))
	}

	if authQuery != "" {
		if strings.Contains(requestURL, "?") {
			requestURL += "&" + authQuery
		} else {
			requestURL += "?" + authQuery
		}
	} else {
		tokenSource, err := getAuthTokenSource(key)
		if err != nil {
			removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
			providerUtils.ReleaseStreamingResponse(ctx, resp)
			return nil, providerUtils.NewGatewayOperationError("error creating auth token source", err)
		}
		token, err := tokenSource.Token()
		if err != nil {
			removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
			providerUtils.ReleaseStreamingResponse(ctx, resp)
			return nil, providerUtils.NewGatewayOperationError("error getting token", err)
		}
		fasthttpReq.Header.Set("Authorization", "Bearer "+token.AccessToken)
	}

	fasthttpReq.SetRequestURI(requestURL)

	providerUtils.SetExtraHeaders(ctx, fasthttpReq, provider.networkConfig.ExtraHeaders, nil)

	for k, v := range req.SafeHeaders {
		if strings.EqualFold(k, "authorization") || strings.EqualFold(k, "proxy-authorization") {
			continue
		}
		fasthttpReq.Header.Set(k, v)
	}

	if len(req.Body) > 0 && strings.Contains(strings.ToLower(string(fasthttpReq.Header.ContentType())), "application/json") {
		bodyStr := vertexBodyProjectsRe.ReplaceAllString(string(req.Body), "${1}projects/"+projectID)
		bodyStr = vertexLocationsPathRe.ReplaceAllString(bodyStr, "/locations/"+keyRegion)
		bodyStr = vertexShortModelRe.ReplaceAllString(bodyStr,
			fmt.Sprintf(`"projects/%s/locations/%s/publishers/google/$1"`, projectID, keyRegion))
		fasthttpReq.SetBodyString(bodyStr)
	} else if len(req.Body) > 0 {
		fasthttpReq.SetBody(req.Body)
	}

	activeClient := providerUtils.PrepareResponseStreaming(ctx, provider.streamingClient, resp)
	startTime := time.Now()
	err := activeClient.Do(fasthttpReq, resp)
	latency := time.Since(startTime)
	if err != nil {
		providerUtils.ReleaseStreamingResponse(ctx, resp)
		if errors.Is(err, context.Canceled) {
			return nil, providerUtils.SetErrorLatency(&schemas.GatewayError{
				IsGatewayError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   err,
				},
			}, latency)
		}
		if errors.Is(err, fasthttp.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
			return nil, providerUtils.SetErrorLatency(providerUtils.NewGatewayTimeoutError(schemas.ErrProviderRequestTimedOut, err), latency)
		}
		// Request failed before the first response byte (server closed an idle/pooled connection,
		// broken pipe, connection refused, DNS failure, etc.). Surface as a retriable upstream
		// connection error (502) so executeRequestWithRetries honors max_retries, matching the
		// non-streaming path - see https://github.com/gateway/gateway/issues/4496.
		return nil, providerUtils.SetErrorLatency(providerUtils.NewGatewayUpstreamConnectionError(schemas.ErrProviderDoRequest, err), latency)
	}

	if resp.StatusCode() == fasthttp.StatusUnauthorized || resp.StatusCode() == fasthttp.StatusForbidden {
		removeVertexClient(key.VertexKeyConfig.AuthCredentials.GetValue())
	}

	headers := providerUtils.ExtractPassthroughProviderResponseHeaders(resp)
	ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, headers)

	bodyStream := resp.BodyStream()
	if bodyStream == nil {
		providerUtils.ReleaseStreamingResponse(ctx, resp)
		return nil, providerUtils.NewGatewayOperationError(
			"provider returned an empty stream body",
			fmt.Errorf("provider returned an empty stream body"))
	}

	providerUtils.SetStreamIdleTimeoutIfEmpty(ctx, provider.networkConfig.StreamIdleTimeoutInSeconds)
	return providerUtils.StreamPassthrough(
		ctx, postHookRunner, postHookSpanFinalizer, resp, bodyStream,
		providerUtils.PassthroughStreamParams{
			StatusCode:          resp.StatusCode(),
			Headers:             headers,
			Path:                req.Path,
			RawRequest:          req.Body,
			CancellationBody:    providerUtils.PassthroughJSONBody(fasthttpReq, req.Body),
			StartTime:           time.Now(),
			UseTerminalDetector: true,
			Logger:              provider.logger,
			HasUsage:            gemini.HasGeminiPassthroughUsage,
			Observe: func(event []byte) *schemas.GatewayPassthroughUsage {
				return gemini.ExtractGeminiPassthroughUsage(req.Path, req.Body, event)
			},
		},
	), nil
}
