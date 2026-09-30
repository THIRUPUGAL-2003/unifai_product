package handlers

import (
	"encoding/json"

	"github.com/fasthttp/router"
	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/plugins/guardrails"
	"github.com/unifai/unifai/transports/unifai-http/lib"
	"github.com/valyala/fasthttp"
)

// GuardrailsHandler manages runtime configuration updates for Guardrails.
type GuardrailsHandler struct {
	store         *lib.Config
	configManager ConfigManager
}

// NewGuardrailsHandler creates a new handler for guardrails configuration management.
func NewGuardrailsHandler(configManager ConfigManager, store *lib.Config) *GuardrailsHandler {
	return &GuardrailsHandler{
		configManager: configManager,
		store:         store,
	}
}

// RegisterRoutes registers the configuration-related routes.
func (h *GuardrailsHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.UnifAIHTTPMiddleware) {
	r.GET("/api/guardrails/config", lib.ChainMiddlewares(h.getConfig, middlewares...))
	r.PUT("/api/guardrails/config", lib.ChainMiddlewares(h.updateConfig, middlewares...))
}

// getConfig handles GET /api/guardrails/config - Get the current guardrails configuration
func (h *GuardrailsHandler) getConfig(ctx *fasthttp.RequestCtx) {
	h.store.Mu.RLock()
	defer h.store.Mu.RUnlock()

	if h.store.GuardrailsConfig == nil {
		SendJSON(ctx, &lib.GuardrailsConfig{})
		return
	}
	SendJSON(ctx, h.store.GuardrailsConfig)
}

// updateConfig handles PUT /api/guardrails/config - Updates the guardrails configuration
// and reloads the guardrails plugin so rules take effect immediately.
func (h *GuardrailsHandler) updateConfig(ctx *fasthttp.RequestCtx) {
	var payload lib.GuardrailsConfig

	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}

	pluginCfg := &guardrails.Config{}
	cfgBytes, err := json.Marshal(payload)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid guardrails configuration")
		return
	}
	if err := json.Unmarshal(cfgBytes, pluginCfg); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid guardrails configuration")
		return
	}
	if err := guardrails.ValidateConfig(pluginCfg); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}

	h.store.Mu.Lock()
	h.store.GuardrailsConfig = &payload
	h.store.Mu.Unlock()

	if err := h.store.PersistGuardrailsConfig(&payload); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Guardrails config updated in memory but failed to persist to config file: "+err.Error())
		return
	}

	// InstantiatePlugin(guardrails) reads unifaiConfig.GuardrailsConfig — reload so CEL/providers apply now.
	if h.configManager != nil {
		if err := h.configManager.ReloadPlugin(ctx, guardrails.PluginName, nil, nil, nil, nil); err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, "Guardrails config saved but plugin reload failed: "+err.Error())
			return
		}
	}

	SendJSON(ctx, map[string]any{"success": true, "reloaded": true})
}
