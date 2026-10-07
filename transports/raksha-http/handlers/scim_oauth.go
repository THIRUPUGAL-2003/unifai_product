package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/raksha/raksha/framework/configstore"
	"github.com/valyala/fasthttp"
)

func loadSCIMConfig(ctx *fasthttp.RequestCtx, store configstore.WorkspaceStore) scimConfigPayload {
	cfg := scimConfigPayload{Enabled: false, Config: map[string]any{}}
	row, err := store.GetWorkspaceSetting(ctx, configstore.WorkspaceSettingSCIM)
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal([]byte(row.Data), &cfg)
	if cfg.Config == nil {
		cfg.Config = map[string]any{}
	}
	return cfg
}

func saveSCIMConfig(ctx *fasthttp.RequestCtx, store configstore.WorkspaceStore, cfg scimConfigPayload) error {
	if cfg.Config == nil {
		cfg.Config = map[string]any{}
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return store.UpsertWorkspaceSetting(ctx, configstore.WorkspaceSettingSCIM, string(raw))
}

// SCIM OAuth routes are allowlisted for browser redirects / logout.
// Callback exchanges the authorization code for tokens when oauth.token_url
// + client credentials are configured; otherwise persists a clear error state.

func (h *WorkspaceHandler) getSCIMOAuthConfig(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	cfg := loadSCIMConfig(ctx, store)
	oauth, _ := cfg.Config["oauth"].(map[string]any)
	if oauth == nil {
		oauth = map[string]any{}
	}
	SendJSON(ctx, map[string]any{
		"enabled":  cfg.Enabled,
		"provider": cfg.Provider,
		"oauth":    redactSCIMOAuthPublic(oauth),
	})
}

// redactSCIMOAuthPublic strips secrets from the public SCIM OAuth bootstrap payload.
func redactSCIMOAuthPublic(oauth map[string]any) map[string]any {
	out := map[string]any{}
	safeKeys := []string{
		"authorize_url", "authorization_url", "authorizationUrl", "auth_url",
		"client_id", "clientId",
		"redirect_uri", "redirectUri",
		"scopes", "scope",
		"token_url", "tokenUrl", "token_endpoint",
		"provider",
	}
	for _, k := range safeKeys {
		if v, ok := oauth[k]; ok {
			out[k] = v
		}
	}
	if oauthString(oauth, "client_secret", "clientSecret") != "" {
		out["client_secret_configured"] = true
	}
	if oauthString(oauth, "access_token", "accessToken", "bearer_token", "bearerToken", "refresh_token", "refreshToken") != "" {
		out["token_configured"] = true
	}
	return out
}

func oauthString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func exchangeSCIMOAuthCode(oauth map[string]any, code string) (map[string]any, error) {
	tokenURL := oauthString(oauth, "token_url", "tokenUrl", "token_endpoint")
	clientID := oauthString(oauth, "client_id", "clientId")
	clientSecret := oauthString(oauth, "client_secret", "clientSecret")
	redirectURI := oauthString(oauth, "redirect_uri", "redirectUri")
	if tokenURL == "" || clientID == "" {
		return nil, fmt.Errorf("SCIM OAuth is incomplete: set token_url and client_id (and client_secret) under User Provisioning, or paste a bearer token instead")
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", clientID)
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}
	if redirectURI != "" {
		form.Set("redirect_uri", redirectURI)
	}
	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token endpoint request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 300 {
			msg = msg[:300] + "…"
		}
		return nil, fmt.Errorf("token endpoint HTTP %d: %s", resp.StatusCode, msg)
	}
	var tok map[string]any
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("invalid token JSON: %w", err)
	}
	access, _ := tok["access_token"].(string)
	if strings.TrimSpace(access) == "" {
		return nil, fmt.Errorf("token response missing access_token")
	}
	return tok, nil
}

func (h *WorkspaceHandler) scimOAuthCallback(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	var body struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &body); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	if strings.TrimSpace(body.Code) == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "authorization code is required")
		return
	}
	cfg := loadSCIMConfig(ctx, store)
	if cfg.Config == nil {
		cfg.Config = map[string]any{}
	}
	oauth, _ := cfg.Config["oauth"].(map[string]any)
	if oauth == nil {
		oauth = map[string]any{}
	}
	oauth["last_callback_at"] = time.Now().UTC().Format(time.RFC3339)
	oauth["last_state"] = body.State
	delete(oauth, "last_error")

	tok, err := exchangeSCIMOAuthCode(oauth, body.Code)
	if err != nil {
		oauth["discovery_complete"] = false
		oauth["pending_code"] = body.Code
		oauth["last_error"] = err.Error()
		cfg.Config["oauth"] = oauth
		_ = saveSCIMConfig(ctx, store, cfg)
		SendError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}

	if access, ok := tok["access_token"].(string); ok && access != "" {
		// Outbound OAuth token for calling the IdP — never overwrite the inbound SCIM
		// bearer the IdP must send on /scim/v2 (that secret is configured separately).
		oauth["access_token"] = access
	}
	if refresh, ok := tok["refresh_token"].(string); ok && refresh != "" {
		oauth["refresh_token"] = refresh
	}
	if exp, ok := tok["expires_in"]; ok {
		oauth["expires_in"] = exp
	}
	oauth["discovery_complete"] = true
	oauth["connected_at"] = time.Now().UTC().Format(time.RFC3339)
	delete(oauth, "pending_code")
	cfg.Config["oauth"] = oauth
	if err := saveSCIMConfig(ctx, store, cfg); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to persist SCIM OAuth tokens")
		return
	}
	SendJSON(ctx, map[string]any{"status": "ok", "message": "SCIM OAuth connected — access token saved"})
}

func (h *WorkspaceHandler) scimOAuthRefresh(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	cfg := loadSCIMConfig(ctx, store)
	oauth, _ := cfg.Config["oauth"].(map[string]any)
	if oauth == nil {
		SendJSON(ctx, map[string]any{"status": "ok", "refreshed": false})
		return
	}
	refresh := oauthString(oauth, "refresh_token")
	tokenURL := oauthString(oauth, "token_url", "tokenUrl", "token_endpoint")
	clientID := oauthString(oauth, "client_id", "clientId")
	clientSecret := oauthString(oauth, "client_secret", "clientSecret")
	if refresh == "" || tokenURL == "" || clientID == "" {
		SendJSON(ctx, map[string]any{"status": "ok", "refreshed": false, "reason": "missing refresh_token or oauth client config"})
		return
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refresh)
	form.Set("client_id", clientID)
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}
	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadGateway, "refresh request failed: "+err.Error())
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		SendError(ctx, fasthttp.StatusBadGateway, fmt.Sprintf("refresh HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
		return
	}
	var tok map[string]any
	if err := json.Unmarshal(body, &tok); err != nil {
		SendError(ctx, fasthttp.StatusBadGateway, "invalid refresh JSON")
		return
	}
	if access, ok := tok["access_token"].(string); ok && access != "" {
		// The inbound /scim/v2 bearer is what the IdP was configured to send; rotating the
		// OAuth access token must not change it.
		oauth["access_token"] = access
	}
	if newRefresh, ok := tok["refresh_token"].(string); ok && newRefresh != "" {
		oauth["refresh_token"] = newRefresh
	}
	oauth["refreshed_at"] = time.Now().UTC().Format(time.RFC3339)
	cfg.Config["oauth"] = oauth
	if err := saveSCIMConfig(ctx, store, cfg); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to persist refreshed tokens")
		return
	}
	SendJSON(ctx, map[string]any{"status": "ok", "refreshed": true})
}

func (h *WorkspaceHandler) scimOAuthLogout(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	cfg := loadSCIMConfig(ctx, store)
	oauth, _ := cfg.Config["oauth"].(map[string]any)
	if oauth == nil {
		oauth = map[string]any{}
	}
	delete(oauth, "access_token")
	delete(oauth, "refresh_token")
	delete(oauth, "pending_code")
	delete(oauth, "discovery_complete")
	delete(oauth, "connected_at")
	oauth["logged_out_at"] = time.Now().UTC().Format(time.RFC3339)
	cfg.Config["oauth"] = oauth
	_ = saveSCIMConfig(ctx, store, cfg)
	SendJSON(ctx, map[string]any{"status": "ok"})
}
