package handlers

import (
	"encoding/json"
	"strings"

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
	// The bearer token lets its holder create admin users over SCIM; only admins may read it.
	if h.callerRole(ctx) != "admin" {
		redactSCIMSecrets(&cfg)
	}
	SendJSON(ctx, cfg)
}

const scimRedacted = "********"

func redactSCIMSecrets(cfg *scimConfigPayload) {
	if cfg.BearerToken != "" {
		cfg.BearerToken = scimRedacted
	}
	redactSecretKeys(cfg.Config)
}

func restoreRedactedKeys(dst, stored map[string]any) {
	for key, value := range dst {
		switch v := value.(type) {
		case map[string]any:
			if nested, ok := stored[key].(map[string]any); ok {
				restoreRedactedKeys(v, nested)
			}
		case string:
			if v == scimRedacted {
				dst[key] = stored[key]
			}
		}
	}
}

func isSCIMSecretKey(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password")
}

func redactSecretKeys(m map[string]any) {
	for key, value := range m {
		switch v := value.(type) {
		case map[string]any:
			redactSecretKeys(v)
		case string:
			if v != "" && isSCIMSecretKey(key) {
				m[key] = scimRedacted
			}
		}
	}
}

// keepStoredSecretKeys overwrites every secret-like key in dst with the stored value
// (or drops it when nothing is stored), so the caller cannot set or change secrets.
func keepStoredSecretKeys(dst, stored map[string]any) {
	for key, value := range dst {
		if nested, ok := value.(map[string]any); ok {
			storedNested, _ := stored[key].(map[string]any)
			keepStoredSecretKeys(nested, storedNested)
			continue
		}
		if !isSCIMSecretKey(key) {
			continue
		}
		if prev, ok := stored[key]; ok {
			dst[key] = prev
		} else {
			delete(dst, key)
		}
	}
	for key, value := range stored {
		if _, present := dst[key]; !present && isSCIMSecretKey(key) {
			dst[key] = value
		}
	}
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
	isAdmin := h.callerRole(ctx) == "admin"
	var stored scimConfigPayload
	if row, err := store.GetWorkspaceSetting(ctx, configstore.WorkspaceSettingSCIM); err == nil && row != nil {
		_ = json.Unmarshal([]byte(row.Data), &stored)
	}
	if isAdmin {
		// A config loaded by a non-admin comes back with redacted secrets: keep the stored values.
		if payload.BearerToken == scimRedacted {
			payload.BearerToken = stored.BearerToken
		}
		restoreRedactedKeys(payload.Config, stored.Config)
	} else {
		// The bearer token can create admin users over SCIM, so only admins may set secrets.
		payload.BearerToken = stored.BearerToken
		keepStoredSecretKeys(payload.Config, stored.Config)
		// The default role is granted to every provisioned user without a role, so a
		// non-admin may only pick roles it could assign by hand.
		for _, key := range []string{"defaultRole", "default_role"} {
			requested, _ := payload.Config[key].(string)
			if strings.TrimSpace(requested) == "" {
				continue
			}
			previous, _ := stored.Config[key].(string)
			if strings.EqualFold(strings.TrimSpace(requested), strings.TrimSpace(previous)) {
				continue
			}
			r := strings.ToLower(strings.TrimSpace(requested))
			if r == "admin" || r == "sub_admin" || (h.store != nil && !callerMayManageRole(ctx, h.store.ConfigStore, r)) {
				SendError(ctx, fasthttp.StatusForbidden, "Only the super admin can set the SCIM default role to "+requested)
				return
			}
		}
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
	if !isAdmin {
		redactSCIMSecrets(&payload)
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

const connectorSecretPlaceholder = "<redacted>"

func isConnectorSecretKey(key string) bool {
	k := strings.ToLower(key)
	switch k {
	case "api_key", "password", "credentials_json":
		return true
	}
	return strings.Contains(k, "secret") || strings.Contains(k, "token")
}

// redactConnectorPayload masks secret config values before a connector is returned to a client.
func redactConnectorPayload(payload map[string]any) {
	cfg, ok := payload["config"].(map[string]any)
	if !ok {
		return
	}
	redacted := make(map[string]any, len(cfg))
	for k, v := range cfg {
		if s, isStr := v.(string); isStr && s != "" && isConnectorSecretKey(k) {
			redacted[k] = connectorSecretPlaceholder
			continue
		}
		redacted[k] = v
	}
	payload["config"] = redacted
}

// restoreConnectorSecrets swaps redaction placeholders sent back by the UI for the stored values.
func restoreConnectorSecrets(incoming map[string]any, stored map[string]any) {
	inCfg, ok := incoming["config"].(map[string]any)
	if !ok {
		return
	}
	storedCfg, _ := stored["config"].(map[string]any)
	for k, v := range inCfg {
		if s, isStr := v.(string); isStr && s == connectorSecretPlaceholder {
			if prev, ok := storedCfg[k]; ok {
				inCfg[k] = prev
			} else {
				delete(inCfg, k)
			}
		}
	}
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
		redactConnectorPayload(payload)
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
	redactConnectorPayload(payload)
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
	if row, err := store.GetWorkspaceSetting(ctx, configstore.WorkspaceSettingConnector(name)); err == nil && row != nil {
		var stored map[string]any
		if json.Unmarshal([]byte(row.Data), &stored) == nil {
			restoreConnectorSecrets(payload, stored)
		}
	} else {
		restoreConnectorSecrets(payload, nil)
	}
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
	redactConnectorPayload(payload)
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
