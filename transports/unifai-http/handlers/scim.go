package handlers

import (
	"encoding/json"

	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/connectors"
	"github.com/valyala/fasthttp"
)

type scimConfigPayload struct {
	Enabled     bool           `json:"enabled"`
	Provider    string         `json:"provider"`
	BearerToken string         `json:"bearer_token,omitempty"`
	Config      map[string]any `json:"config"`
}

func (h *WorkspaceHandler) getSCIMConfig(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	row, err := store.GetWorkspaceSetting(ctx, configstore.WorkspaceSettingSCIM)
	if isStoreNotFound(err) {
		SendJSON(ctx, scimConfigPayload{Enabled: false, Config: map[string]any{}})
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load scim config")
		return
	}
	var cfg scimConfigPayload
	if err := json.Unmarshal([]byte(row.Data), &cfg); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to parse scim config")
		return
	}
	if cfg.Config == nil {
		cfg.Config = map[string]any{}
	}
	SendJSON(ctx, cfg)
}

func (h *WorkspaceHandler) updateSCIMConfig(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	var payload scimConfigPayload
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	if payload.Enabled && payload.Provider == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "provider is required when scim is enabled")
		return
	}
	switch payload.Provider {
	case "", "okta", "entra", "keycloak":
	default:
		SendError(ctx, fasthttp.StatusBadRequest, "provider must be okta, entra, or keycloak")
		return
	}
	if payload.Config == nil {
		payload.Config = map[string]any{}
	}
	ensureSCIMBearerToken(&payload)
	raw, err := json.Marshal(payload)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to save scim config")
		return
	}
	if err := store.UpsertWorkspaceSetting(ctx, configstore.WorkspaceSettingSCIM, string(raw)); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to save scim config")
		return
	}
	SendJSON(ctx, payload)
}

func (h *WorkspaceHandler) listSCIMProviders(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	row, err := store.GetWorkspaceSetting(ctx, configstore.WorkspaceSettingSCIM)
	if err != nil {
		SendJSON(ctx, []any{})
		return
	}
	var cfg scimConfigPayload
	if err := json.Unmarshal([]byte(row.Data), &cfg); err != nil || !cfg.Enabled || cfg.Provider == "" {
		SendJSON(ctx, []any{})
		return
	}
	SendJSON(ctx, []map[string]any{{"provider": cfg.Provider, "enabled": cfg.Enabled}})
}

func (h *WorkspaceHandler) listConnectors(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	out := make([]map[string]any, 0, len(connectors.ConnectorNames))
	for _, name := range connectors.ConnectorNames {
		row, err := store.GetWorkspaceSetting(ctx, configstore.WorkspaceSettingConnector(name))
		if isStoreNotFound(err) {
			continue
		}
		if err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, "failed to load connectors")
			return
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(row.Data), &payload); err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, "failed to parse connector")
			return
		}
		payload["name"] = name
		out = append(out, payload)
	}
	SendJSON(ctx, map[string]any{"connectors": out, "count": len(out)})
}

func (h *WorkspaceHandler) getConnector(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	name := pathID(ctx, "name")
	if !connectors.IsKnown(name) {
		SendError(ctx, fasthttp.StatusNotFound, "unknown connector")
		return
	}
	row, err := store.GetWorkspaceSetting(ctx, configstore.WorkspaceSettingConnector(name))
	if isStoreNotFound(err) {
		SendJSON(ctx, map[string]any{"name": name, "enabled": false, "config": map[string]any{}})
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load connector")
		return
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(row.Data), &payload); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to parse connector")
		return
	}
	payload["name"] = name
	SendJSON(ctx, payload)
}

func (h *WorkspaceHandler) deleteConnector(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	name := pathID(ctx, "name")
	if !connectors.IsKnown(name) {
		SendError(ctx, fasthttp.StatusNotFound, "unknown connector")
		return
	}
	if err := store.DeleteWorkspaceSetting(ctx, configstore.WorkspaceSettingConnector(name)); err != nil {
		if isStoreNotFound(err) {
			SendError(ctx, fasthttp.StatusNotFound, "connector not configured")
			return
		}
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to delete connector")
		return
	}
	connectors.Default.Remove(name)
	ReloadEnterpriseRuntimeFromStore(store, h.store)
	SendJSON(ctx, map[string]any{"name": name, "deleted": true})
}

func (h *WorkspaceHandler) updateConnector(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	name := pathID(ctx, "name")
	if !connectors.IsKnown(name) {
		SendError(ctx, fasthttp.StatusNotFound, "unknown connector")
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	payload["name"] = name
	raw, err := json.Marshal(payload)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to save connector")
		return
	}
	if err := store.UpsertWorkspaceSetting(ctx, configstore.WorkspaceSettingConnector(name), string(raw)); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to save connector")
		return
	}
	ReloadEnterpriseRuntimeFromStore(store, h.store)
	settings := connectors.Settings{Name: name, Enabled: false, Config: map[string]string{}}
	if enabled, ok := payload["enabled"].(bool); ok {
		settings.Enabled = enabled
	}
	if cfgMap, ok := payload["config"].(map[string]any); ok {
		for k, v := range cfgMap {
			if s, ok := v.(string); ok {
				settings.Config[k] = s
			}
		}
	}
	connectors.Default.ApplySettings(settings)
	test := connectors.Default.Test(ctx, name, settings)
	payload["connection"] = test
	// Always 200 after a successful save. A failed connectivity probe must not look like
	// an HTTP failure to the UI — the form shows connection.ok / connection.error instead.
	SendJSON(ctx, payload)
}

func (h *WorkspaceHandler) testConnector(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	name := pathID(ctx, "name")
	if !connectors.IsKnown(name) {
		SendError(ctx, fasthttp.StatusNotFound, "unknown connector")
		return
	}
	row, err := store.GetWorkspaceSetting(ctx, configstore.WorkspaceSettingConnector(name))
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "connector not configured")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load connector")
		return
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(row.Data), &payload); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to parse connector")
		return
	}
	settings := connectors.Settings{Name: name, Config: map[string]string{}}
	if enabled, ok := payload["enabled"].(bool); ok {
		settings.Enabled = enabled
	}
	if cfgMap, ok := payload["config"].(map[string]any); ok {
		for k, v := range cfgMap {
			if s, ok := v.(string); ok {
				settings.Config[k] = s
			}
		}
	}
	result := connectors.Default.Test(ctx, name, settings)
	if !result.OK {
		SendJSONWithStatus(ctx, map[string]any{"connection": result}, fasthttp.StatusBadGateway)
		return
	}
	SendJSON(ctx, map[string]any{"connection": result})
}
