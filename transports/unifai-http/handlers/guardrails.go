package handlers

import (
	"encoding/json"
	"sync"

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
	saveMu        sync.Mutex
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
	r.PUT("/api/guardrails/rules", lib.ChainMiddlewares(h.updateRules, middlewares...))
	r.PUT("/api/guardrails/providers", lib.ChainMiddlewares(h.updateProviders, middlewares...))
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
	h.saveMu.Lock()
	defer h.saveMu.Unlock()
	h.saveConfig(ctx, payload)
}

// updateRules handles PUT /api/guardrails/rules - replaces only the rules, keeping providers.
func (h *GuardrailsHandler) updateRules(ctx *fasthttp.RequestCtx) {
	var body struct {
		GuardrailRules []lib.GuardrailRule `json:"guardrail_rules"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &body); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}
	h.saveMu.Lock()
	defer h.saveMu.Unlock()
	next := h.currentConfig()
	next.GuardrailRules = body.GuardrailRules
	h.saveConfig(ctx, next)
}

// updateProviders handles PUT /api/guardrails/providers - replaces only the providers, keeping rules.
func (h *GuardrailsHandler) updateProviders(ctx *fasthttp.RequestCtx) {
	var body struct {
		GuardrailProviders []lib.GuardrailProvider `json:"guardrail_providers"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &body); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}
	h.saveMu.Lock()
	defer h.saveMu.Unlock()
	next := h.currentConfig()
	next.GuardrailProviders = body.GuardrailProviders
	h.saveConfig(ctx, next)
}

func (h *GuardrailsHandler) currentConfig() lib.GuardrailsConfig {
	h.store.Mu.RLock()
	defer h.store.Mu.RUnlock()
	if h.store.GuardrailsConfig == nil {
		return lib.GuardrailsConfig{}
	}
	return lib.GuardrailsConfig{
		GuardrailRules:     append([]lib.GuardrailRule(nil), h.store.GuardrailsConfig.GuardrailRules...),
		GuardrailProviders: append([]lib.GuardrailProvider(nil), h.store.GuardrailsConfig.GuardrailProviders...),
	}
}

// saveConfig validates, persists and applies a full guardrails config. Callers hold saveMu.
func (h *GuardrailsHandler) saveConfig(ctx *fasthttp.RequestCtx, payload lib.GuardrailsConfig) {
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

	if err := h.store.PersistGuardrailsConfig(&payload); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to save guardrails config: "+err.Error())
		return
	}

	h.store.Mu.Lock()
	h.store.GuardrailsConfig = &payload
	h.store.Mu.Unlock()

	// InstantiatePlugin(guardrails) reads unifaiConfig.GuardrailsConfig — reload so CEL/providers apply now.
	if h.configManager != nil {
		if err := h.configManager.ReloadPlugin(ctx, guardrails.PluginName, nil, nil, nil, nil); err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, "Guardrails config saved but plugin reload failed: "+err.Error())
			return
		}
	}

	SendJSON(ctx, map[string]any{"success": true, "reloaded": true})
}
