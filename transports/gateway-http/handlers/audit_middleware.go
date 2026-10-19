package handlers

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/gateway/gateway/core/schemas"
	"github.com/gateway/gateway/framework/configstore"
	"github.com/gateway/gateway/framework/configstore/tables"
	"github.com/valyala/fasthttp"
)

// Paths that must not flood Audit Logs (agent heartbeats, intercept stream, etc.).
// Login/logout keep dedicated action names via SessionHandler.recordAuthAudit.
func shouldSkipWorkspaceAudit(method, path string) bool {
	p := strings.ToLower(path)
	switch {
	case strings.HasPrefix(p, "/api/browser-ai/intercept"):
		return true
	case p == "/api/browser-ai/search-logs":
		// Guard records every search via POST; admin clears (DELETE) must stay audited.
		return method == fasthttp.MethodPost
	case p == "/api/browser-ai/agents/heartbeat",
		p == "/api/browser-ai/agents/wait-command",
		p == "/api/browser-ai/agents/uninstall-ack",
		p == "/api/browser-ai/agents/uninstall-verify",
		p == "/api/browser-ai/agents/uninstall-status",
		p == "/api/browser-ai/setup/proxy-bundle.json",
		p == "/api/browser-ai/setup/proxy-bundle.zip",
		p == "/api/session/login",
		p == "/api/session/logout",
		p == "/api/session/ws-ticket":
		return true
	case strings.HasPrefix(p, "/api/session/is-auth"):
		return true
	default:
		return false
	}
}

func auditActionForMethod(method, path string) string {
	p := strings.ToLower(path)
	if strings.HasSuffix(p, "/activate") || strings.HasSuffix(p, "/deactivate") {
		return "update"
	}
	if strings.HasSuffix(p, "/clone") {
		return "create"
	}
	switch method {
	case fasthttp.MethodPost:
		return "create"
	case fasthttp.MethodDelete:
		return "delete"
	case fasthttp.MethodPut, fasthttp.MethodPatch:
		return "update"
	default:
		return "update"
	}
}

func auditInitiator(store configstore.ConfigStore, ctx *fasthttp.RequestCtx) string {
	if strings.HasPrefix(string(ctx.Path()), "/scim/") {
		return "scim:idp"
	}
	// Prefer context user value if set upstream by auth middleware
	if val := ctx.UserValue(schemas.GatewayContextKeyUserName); val != nil {
		if u, ok := val.(string); ok && u != "" {
			return u
		}
	}
	if isLocal, ok := ctx.UserValue(schemas.IsLocalAdminContextKey).(bool); ok && isLocal {
		return "admin"
	}
	if vkID, ok := ctx.UserValue(schemas.GatewayContextKeyGovernanceVirtualKeyID).(string); ok && vkID != "" {
		return "vk:" + vkID
	}
	if store == nil {
		return "system"
	}
	token := ""
	if authHeader := string(ctx.Request.Header.Peek("Authorization")); strings.HasPrefix(authHeader, "Bearer ") {
		token = strings.TrimPrefix(authHeader, "Bearer ")
	}
	if token == "" {
		token = string(ctx.Request.Header.Cookie("token"))
	}
	if token == "" {
		return "system"
	}
	session, err := store.GetSession(ctx, token)
	if err != nil || session == nil {
		return "system"
	}
	if session.Username != "" {
		return session.Username
	}
	return "admin"
}

// auditTargetForRequest produces a human-friendly entity target (e.g. "Role: admin",
// "Access Profile: Tier 1", "User: alice (Role)", etc.) instead of a raw URL path.
func auditTargetForRequest(path, method string, payload map[string]any) string {
	p := strings.Trim(path, "/")
	parts := strings.Split(p, "/")
	if len(parts) >= 2 && parts[0] == "api" {
		resource := strings.ToLower(parts[1])
		switch resource {
		case "roles":
			if len(parts) >= 4 && parts[3] == "permissions" {
				return "Role: #" + parts[2] + " (Permissions)"
			}
			if len(parts) >= 3 {
				return "Role: #" + parts[2]
			}
			if name, ok := payload["name"].(string); ok && strings.TrimSpace(name) != "" {
				return "Role: " + strings.TrimSpace(name)
			}
			return "Role"
		case "access-profiles":
			if len(parts) >= 4 {
				return "Access Profile: #" + parts[2] + " (" + parts[3] + ")"
			}
			if len(parts) >= 3 {
				return "Access Profile: #" + parts[2]
			}
			if name, ok := payload["name"].(string); ok && strings.TrimSpace(name) != "" {
				return "Access Profile: " + strings.TrimSpace(name)
			}
			return "Access Profile"
		case "users":
			if len(parts) >= 4 && parts[3] == "role" {
				return "User: " + parts[2] + " (Role)"
			}
			if len(parts) >= 4 && parts[3] == "access-profiles" {
				return "User: " + parts[2] + " (Access Profiles)"
			}
			if len(parts) >= 3 {
				return "User: " + parts[2]
			}
			if u, ok := payload["username"].(string); ok && strings.TrimSpace(u) != "" {
				return "User: " + strings.TrimSpace(u)
			}
			return "User"
		case "rbac":
			if len(parts) >= 3 && parts[2] == "scope-grants" {
				if st, ok := payload["scope_type"].(string); ok && strings.TrimSpace(st) != "" {
					return "RBAC Scope: " + strings.TrimSpace(st)
				}
				return "RBAC Scope Grants"
			}
			return "RBAC"
		case "virtual-keys":
			if len(parts) >= 3 {
				return "Virtual Key: " + parts[2]
			}
			if name, ok := payload["name"].(string); ok && strings.TrimSpace(name) != "" {
				return "Virtual Key: " + strings.TrimSpace(name)
			}
			return "Virtual Key"
		case "teams":
			if len(parts) >= 3 {
				return "Team: " + parts[2]
			}
			if name, ok := payload["name"].(string); ok && strings.TrimSpace(name) != "" {
				return "Team: " + strings.TrimSpace(name)
			}
			return "Team"
		case "customers":
			if len(parts) >= 3 {
				return "Customer: " + parts[2]
			}
			if name, ok := payload["name"].(string); ok && strings.TrimSpace(name) != "" {
				return "Customer: " + strings.TrimSpace(name)
			}
			return "Customer"
		case "business-units":
			if len(parts) >= 3 {
				return "Business Unit: " + parts[2]
			}
			if name, ok := payload["name"].(string); ok && strings.TrimSpace(name) != "" {
				return "Business Unit: " + strings.TrimSpace(name)
			}
			return "Business Unit"
		case "guardrails":
			if len(parts) >= 3 {
				return "Guardrail: " + parts[2]
			}
			return "Guardrails"
		case "plugins":
			if len(parts) >= 3 {
				return "Plugin: " + parts[2]
			}
			return "Plugins"
		case "mcp":
			return "MCP Gateway"
		case "adaptive-routing":
			return "Adaptive Routing"
		case "config":
			if len(parts) >= 3 {
				return "Config: " + parts[2]
			}
			return "Configuration"
		case "feature-flags":
			if len(parts) >= 3 {
				return "Feature Flag: " + parts[2]
			}
			if id, ok := payload["id"].(string); ok && strings.TrimSpace(id) != "" {
				return "Feature Flag: " + strings.TrimSpace(id)
			}
			return "Feature Flags"
		case "vector-store-config":
			return "Vector Store Config"
		case "proxy-config":
			return "Proxy Config"
		case "pricing":
			return "Pricing Config"
		case "smtp":
			return "SMTP Config"
		case "session":
			if len(parts) >= 3 {
				return "Session: " + parts[2]
			}
			return "Session"
		}
	} else if strings.HasPrefix(path, "/scim/") {
		return "SCIM Directory"
	}
	return path
}

// WorkspaceAuditMiddleware records every mutating /api/* dashboard action
// (create / update / delete) into Audit Logs — governance, Browser AI, plugins,
// session users, etc. Skips high-volume agent/intercept noise and auth events
// that already use dedicated login/logout actions.
func WorkspaceAuditMiddleware(store configstore.ConfigStore) schemas.GatewayHTTPMiddleware {
	ws, _ := configstore.AsWorkspaceStore(store)
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			start := time.Now()
			next(ctx)
			if ws == nil || store == nil || isAuditDisabled() {
				return
			}
			method := string(ctx.Method())
			if method == fasthttp.MethodGet || method == fasthttp.MethodHead || method == fasthttp.MethodOptions {
				return
			}
			path := string(ctx.Path())
			if !strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/scim/") {
				return
			}
			if shouldSkipWorkspaceAudit(method, path) {
				return
			}
			status := ctx.Response.StatusCode()
			outcome := "success"
			if status >= 400 {
				outcome = "failure"
			}

			// Sanitize request body if JSON to record non-sensitive audit details (SOC-2 compliance)
			var detail string
			var payload map[string]any
			if body := ctx.PostBody(); len(body) > 0 {
				if err := json.Unmarshal(body, &payload); err == nil {
					delete(payload, "password")
					delete(payload, "value")
					delete(payload, "secret")
					delete(payload, "token")
					delete(payload, "admin_password")
					delete(payload, "api_key")
					if sanitizedBytes, err := json.Marshal(payload); err == nil && len(sanitizedBytes) > 2 {
						detail = string(sanitizedBytes)
						if len(detail) > 1024 {
							detail = detail[:1024]
						}
					}
				}
			}

			target := auditTargetForRequest(path, method, payload)

			_ = ws.CreateAuditLog(ctx, &tables.TableAuditLog{
				Action:     auditActionForMethod(method, path),
				Outcome:    outcome,
				Initiator:  auditInitiator(store, ctx),
				Target:     target,
				Method:     method,
				Path:       path,
				IP:         ctx.RemoteIP().String(),
				DurationMs: time.Since(start).Milliseconds(),
				Detail:     detail,
				CreatedAt:  time.Now().UTC(),
			})
		}
	}
}
