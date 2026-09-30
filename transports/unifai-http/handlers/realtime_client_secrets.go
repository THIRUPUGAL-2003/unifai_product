package handlers

import (
	"encoding/json"
	"fmt"
	"mime"
	"strings"
	"time"

	"github.com/fasthttp/router"
	unifai "github.com/unifai/unifai/core"
	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/plugins/governance"
	"github.com/unifai/unifai/plugins/modelcatalogresolver"
	"github.com/unifai/unifai/transports/unifai-http/integrations"
	"github.com/unifai/unifai/transports/unifai-http/lib"
	"github.com/valyala/fasthttp"
)

// RealtimeClientSecretsHandler exposes OpenAI-compatible HTTP routes for
// minting short-lived Realtime client secrets.
type RealtimeClientSecretsHandler struct {
	client       *unifai.UnifAI
	config       *lib.Config
	handlerStore lib.HandlerStore
	routeSpecs   map[string]schemas.RealtimeSessionRoute
}

func NewRealtimeClientSecretsHandler(client *unifai.UnifAI, config *lib.Config) *RealtimeClientSecretsHandler {
	return &RealtimeClientSecretsHandler{
		client:       client,
		config:       config,
		handlerStore: config,
		routeSpecs:   make(map[string]schemas.RealtimeSessionRoute),
	}
}

func (h *RealtimeClientSecretsHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.UnifAIHTTPMiddleware) {
	handler := lib.ChainMiddlewares(h.handleRequest, middlewares...)
	for _, route := range h.realtimeSessionRoutes() {
		h.routeSpecs[route.Path] = route
		r.POST(route.Path, handler)
	}
}

func (h *RealtimeClientSecretsHandler) findGovernancePlugin() governance.BaseGovernancePlugin {
	basePlugins := h.config.BasePlugins.Load()
	if basePlugins == nil {
		return nil
	}

	for _, plugin := range *basePlugins {
		if governancePlugin, ok := plugin.(governance.BaseGovernancePlugin); ok {
			return governancePlugin
		}
	}

	return nil
}

func (h *RealtimeClientSecretsHandler) handleRequest(ctx *fasthttp.RequestCtx) {
	if !isJSONContentType(string(ctx.Request.Header.ContentType())) {
		SendUnifAIError(ctx, newRealtimeClientSecretHandlerError(
			fasthttp.StatusBadRequest,
			"invalid_request_error",
			"Content-Type must be application/json",
			nil,
		))
		return
	}

	body := append([]byte(nil), ctx.Request.Body()...)
	route, ok := h.routeSpecs[string(ctx.Path())]
	if !ok {
		SendUnifAIError(ctx, newRealtimeClientSecretHandlerError(
			fasthttp.StatusNotFound,
			"invalid_request_error",
			"unsupported realtime client secret route",
			nil,
		))
		return
	}

	providerKey, model, normalizedBody, err := resolveRealtimeClientSecretTarget(ctx, h.config, route, body)
	if err != nil {
		SendUnifAIError(ctx, err)
		return
	}

	logger.Info("[realtime-client-secrets] request: path=%s provider=%s model=%s endpoint_type=%s",
		string(ctx.Path()), providerKey, model, route.EndpointType)

	unifaiCtx, cancel := lib.ConvertToUnifAIContext(ctx, h.handlerStore)
	defer cancel()
	unifaiCtx.SetValue(schemas.UnifAIContextKeyHTTPRequestType, schemas.RealtimeRequest)
	if route.DefaultProvider == schemas.OpenAI {
		unifaiCtx.SetValue(schemas.UnifAIContextKeyIntegrationType, "openai")
	}
	if governanceUserID, ok := ctx.UserValue(schemas.UnifAIContextKeyUserID).(string); ok && governanceUserID != "" {
		unifaiCtx.SetValue(schemas.UnifAIContextKeyUserID, governanceUserID)
	}
	if userName, ok := ctx.UserValue(schemas.UnifAIContextKeyUserName).(string); ok && userName != "" {
		unifaiCtx.SetValue(schemas.UnifAIContextKeyUserName, userName)
	}
	if unifaiErr := h.evaluateMintingGovernance(unifaiCtx, providerKey, model); unifaiErr != nil {
		SendUnifAIError(ctx, unifaiErr)
		return
	}

	provider := h.client.GetProviderByKey(providerKey)
	if provider == nil {
		SendUnifAIError(ctx, newRealtimeClientSecretHandlerError(
			fasthttp.StatusBadRequest,
			"invalid_request_error",
			"provider not found: "+string(providerKey),
			nil,
		))
		return
	}

	key, keyErr := h.client.SelectKeyForProviderRequestType(unifaiCtx, schemas.RealtimeRequest, providerKey, model)
	if keyErr != nil {
		SendUnifAIError(ctx, newRealtimeClientSecretHandlerError(
			fasthttp.StatusBadRequest,
			"invalid_request_error",
			keyErr.Error(),
			keyErr,
		))
		return
	}

	// Resolve model aliases now that the key is selected so the forwarded body
	// carries the provider's canonical model, matching wsrealtime/webrtc flows.
	if resolved := key.Aliases.Resolve(model); resolved != "" && resolved != model {
		model = resolved
		reparsed, parseErr := schemas.ParseRealtimeClientSecretBody(normalizedBody)
		if parseErr != nil {
			SendUnifAIError(ctx, parseErr)
			return
		}
		rewritten, normalizeErr := normalizeRealtimeClientSecretBody(reparsed, model)
		if normalizeErr != nil {
			SendUnifAIError(ctx, normalizeErr)
			return
		}
		normalizedBody = rewritten
	}

	sessionProvider, ok := provider.(schemas.RealtimeSessionProvider)
	if !ok {
		SendUnifAIError(ctx, realtimeSessionNotSupportedError(providerKey, provider))
		return
	}

	resp, unifaiErr := sessionProvider.CreateRealtimeClientSecret(unifaiCtx, key, route.EndpointType, normalizedBody)
	if unifaiErr != nil {
		logger.Error("[realtime-client-secrets] upstream error: provider=%s model=%s error=%s",
			providerKey, model, unifaiErr.Error)
		SendUnifAIError(ctx, unifaiErr)
		return
	}

	logger.Info("[realtime-client-secrets] upstream success: provider=%s model=%s status=%d",
		providerKey, model, resp.StatusCode)
	cacheRealtimeEphemeralKeyMapping(
		h.handlerStore.GetKVStore(),
		resp.Body,
		key.ID,
		unifai.GetStringFromContext(unifaiCtx, schemas.UnifAIContextKeyVirtualKey),
	)

	writeRealtimeClientSecretResponse(ctx, resp)
}

func (h *RealtimeClientSecretsHandler) evaluateMintingGovernance(
	unifaiCtx *schemas.UnifAIContext,
	providerKey schemas.ModelProvider,
	model string,
) *schemas.UnifAIError {
	governancePlugin := h.findGovernancePlugin()
	if governancePlugin == nil {
		return nil
	}

	_, unifaiErr := governancePlugin.EvaluateGovernanceRequest(unifaiCtx, &governance.EvaluationRequest{
		VirtualKey: unifai.GetStringFromContext(unifaiCtx, schemas.UnifAIContextKeyVirtualKey),
		Provider:   providerKey,
		Model:      model,
		UserID:     unifai.GetStringFromContext(unifaiCtx, schemas.UnifAIContextKeyUserID),
	}, schemas.RealtimeRequest)
	return unifaiErr
}

func (h *RealtimeClientSecretsHandler) realtimeSessionRoutes() []schemas.RealtimeSessionRoute {
	routes := []schemas.RealtimeSessionRoute{
		{
			Path:         "/v1/realtime/client_secrets",
			EndpointType: schemas.RealtimeSessionEndpointClientSecrets,
		},
		{
			Path:         "/v1/realtime/sessions",
			EndpointType: schemas.RealtimeSessionEndpointSessions,
		},
	}

	for _, path := range integrations.OpenAIRealtimeClientSecretPaths("/openai") {
		endpointType := schemas.RealtimeSessionEndpointClientSecrets
		if strings.HasSuffix(path, "/realtime/sessions") {
			endpointType = schemas.RealtimeSessionEndpointSessions
		}
		routes = append(routes, schemas.RealtimeSessionRoute{
			Path:            path,
			EndpointType:    endpointType,
			DefaultProvider: schemas.OpenAI,
		})
	}
	return routes
}

func resolveRealtimeClientSecretTarget(ctx *fasthttp.RequestCtx, config *lib.Config, route schemas.RealtimeSessionRoute, body []byte) (schemas.ModelProvider, string, []byte, *schemas.UnifAIError) {
	root, err := schemas.ParseRealtimeClientSecretBody(body)
	if err != nil {
		return "", "", nil, err
	}

	rawModel, err := schemas.ExtractRealtimeClientSecretModel(root)
	if err != nil {
		return "", "", nil, err
	}

	defaultProvider := route.DefaultProvider
	providerKey, model := schemas.ParseModelString(rawModel, defaultProvider)
	// Model catalog auto-resolution for bare model names on /v1 client secret routes
	if defaultProvider == "" && providerKey == "" && model != "" {
		selected, candidates := modelcatalogresolver.ResolveProviderFromCatalog(nil, config.ModelCatalog, model)
		if selected != "" {
			ctx.SetUserValue(lib.FastHTTPUserValueModelCatalogResolution, &lib.ModelCatalogResolution{
				Model:            model,
				ResolvedProvider: selected,
				AllProviders:     candidates,
			})
			providerKey = selected
		}
	}
	if defaultProvider == "" && providerKey == "" {
		return "", "", nil, newRealtimeClientSecretHandlerError(
			fasthttp.StatusBadRequest,
			"invalid_request_error",
			"session.model must use provider/model on /v1 realtime client secret routes",
			nil,
		)
	}
	if providerKey == "" || model == "" {
		return "", "", nil, newRealtimeClientSecretHandlerError(
			fasthttp.StatusBadRequest,
			"invalid_request_error",
			"session.model is required",
			nil,
		)
	}

	// Normalize the forwarded body so the upstream provider sees the bare model
	// (strip provider prefix). Mirrors resolveRealtimeSDPTarget normalization.
	normalizedBody, normalizeErr := normalizeRealtimeClientSecretBody(root, model)
	if normalizeErr != nil {
		return "", "", nil, normalizeErr
	}

	return providerKey, model, normalizedBody, nil
}

func normalizeRealtimeClientSecretBody(root map[string]json.RawMessage, bareModel string) ([]byte, *schemas.UnifAIError) {
	normalizedModel, marshalErr := json.Marshal(bareModel)
	if marshalErr != nil {
		return nil, newRealtimeClientSecretHandlerError(fasthttp.StatusInternalServerError, "server_error", "failed to encode normalized model", marshalErr)
	}

	// Normalize session.model if present
	if sessionJSON, ok := root["session"]; ok && len(sessionJSON) > 0 {
		var session map[string]json.RawMessage
		if err := json.Unmarshal(sessionJSON, &session); err == nil {
			if _, hasModel := session["model"]; hasModel {
				session["model"] = normalizedModel
				rewritten, err := json.Marshal(session)
				if err != nil {
					return nil, newRealtimeClientSecretHandlerError(fasthttp.StatusInternalServerError, "server_error", "failed to re-encode session", err)
				}
				root["session"] = rewritten
			}
		}
	}
	// Normalize top-level model if present
	if _, ok := root["model"]; ok {
		root["model"] = normalizedModel
	}

	normalized, marshalErr := json.Marshal(root)
	if marshalErr != nil {
		return nil, newRealtimeClientSecretHandlerError(fasthttp.StatusInternalServerError, "server_error", "failed to re-encode body", marshalErr)
	}
	return normalized, nil
}

const realtimeEphemeralKeyMappingPrefix = "realtime:ephemeral-key:"

type realtimeEphemeralKeyMapping struct {
	KeyID      string `json:"key_id,omitempty"`
	VirtualKey string `json:"virtual_key,omitempty"`
}

func cacheRealtimeEphemeralKeyMapping(kv schemas.KVStore, body []byte, keyID string, virtualKey string) {
	if kv == nil || len(body) == 0 || strings.TrimSpace(keyID) == "" {
		return
	}

	token, ttl, ok := parseRealtimeEphemeralKeyMapping(body)
	if !ok || strings.TrimSpace(token) == "" || ttl <= 0 {
		return
	}

	payload, err := json.Marshal(realtimeEphemeralKeyMapping{
		KeyID:      strings.TrimSpace(keyID),
		VirtualKey: strings.TrimSpace(virtualKey),
	})
	if err != nil {
		logger.Warn("failed to encode realtime ephemeral key mapping for key_id=%s: %v", keyID, err)
		return
	}

	if err := kv.SetWithTTL(buildRealtimeEphemeralKeyMappingKey(token), payload, ttl); err != nil {
		logger.Warn("failed to cache realtime ephemeral key mapping for key_id=%s: %v", keyID, err)
	}
}

func parseRealtimeEphemeralKeyMapping(body []byte) (string, time.Duration, bool) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return "", 0, false
	}

	var clientSecret struct {
		Value     string `json:"value"`
		ExpiresAt int64  `json:"expires_at"`
	}

	// OpenAI client_secrets responses expose the ephemeral token at the top level.
	// Keep accepting the nested shape too so the mapping logic stays compatible
	// with any provider/session endpoint variants that wrap the secret object.
	if err := json.Unmarshal(body, &clientSecret); err != nil || strings.TrimSpace(clientSecret.Value) == "" || clientSecret.ExpiresAt <= 0 {
		clientSecretRaw, ok := root["client_secret"]
		if !ok || len(clientSecretRaw) == 0 || string(clientSecretRaw) == "null" {
			return "", 0, false
		}
		if err := json.Unmarshal(clientSecretRaw, &clientSecret); err != nil {
			return "", 0, false
		}
	}
	if strings.TrimSpace(clientSecret.Value) == "" || clientSecret.ExpiresAt <= 0 {
		return "", 0, false
	}

	ttl := time.Until(time.Unix(clientSecret.ExpiresAt, 0))
	if ttl <= 0 {
		return "", 0, false
	}

	return clientSecret.Value, ttl, true
}

func buildRealtimeEphemeralKeyMappingKey(token string) string {
	return realtimeEphemeralKeyMappingPrefix + strings.TrimSpace(token)
}

func realtimeSessionNotSupportedError(providerKey schemas.ModelProvider, provider schemas.Provider) *schemas.UnifAIError {
	if rtProvider, ok := provider.(schemas.RealtimeProvider); ok && rtProvider.SupportsRealtimeAPI() {
		return newRealtimeClientSecretHandlerError(
			fasthttp.StatusBadRequest,
			"invalid_request_error",
			fmt.Sprintf("provider %s supports realtime websocket connections but not realtime client secret creation", providerKey),
			nil,
		)
	}

	return newRealtimeClientSecretHandlerError(
		fasthttp.StatusBadRequest,
		"invalid_request_error",
		fmt.Sprintf("provider %s does not support realtime client secret creation", providerKey),
		nil,
	)
}

func newRealtimeClientSecretHandlerError(status int, errorType, message string, err error) *schemas.UnifAIError {
	return &schemas.UnifAIError{
		IsUnifAIError: false,
		StatusCode:    schemas.Ptr(status),
		Error: &schemas.ErrorField{
			Type:    schemas.Ptr(errorType),
			Message: message,
			Error:   err,
		},
		ExtraFields: schemas.UnifAIErrorExtraFields{
			RequestType: schemas.RealtimeRequest,
		},
	}
}

func writeRealtimeClientSecretResponse(ctx *fasthttp.RequestCtx, resp *schemas.UnifAIPassthroughResponse) {
	if resp == nil {
		SendUnifAIError(ctx, newRealtimeClientSecretHandlerError(
			fasthttp.StatusInternalServerError,
			"server_error",
			"provider returned an empty realtime client secret response",
			nil,
		))
		return
	}

	for key, value := range resp.Headers {
		ctx.Response.Header.Set(key, value)
	}
	if len(ctx.Response.Header.ContentType()) == 0 {
		ctx.SetContentType("application/json")
	}
	ctx.SetStatusCode(resp.StatusCode)
	ctx.SetBody(resp.Body)
}

func isJSONContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	mediaType = strings.ToLower(mediaType)
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}
