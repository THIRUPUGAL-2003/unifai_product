package handlers

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/configstore/tables"
	"github.com/valyala/fasthttp"
)

// Paths that must not flood Audit Logs (agent heartbeats, intercept stream, etc.).
// Login/logout keep dedicated action names via SessionHandler.recordAuthAudit.
func shouldSkipWorkspaceAudit(path string) bool {
	p := strings.ToLower(path)
	switch {
	case strings.HasPrefix(p, "/api/browser-ai/intercept"):
		return true
	case p == "/api/browser-ai/agents/heartbeat",
		p == "/api/browser-ai/agents/uninstall-ack",
		p == "/api/browser-ai/agents/uninstall-verify",
		p == "/api/browser-ai/agents/uninstall-status",
		p == "/api/browser-ai/setup/proxy-bundle.json",
		p == "/api/browser-ai/setup/proxy-bundle.zip",
		p == "/api/browser-ai/search-logs",
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

func auditActionForMethod(method string) string {
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

// WorkspaceAuditMiddleware records every mutating /api/* dashboard action
// (create / update / delete) into Audit Logs — governance, Browser AI, plugins,
// session users, etc. Skips high-volume agent/intercept noise and auth events
// that already use dedicated login/logout actions.
func WorkspaceAuditMiddleware(store configstore.ConfigStore) schemas.UnifAIHTTPMiddleware {
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
			if shouldSkipWorkspaceAudit(path) {
				return
			}
			status := ctx.Response.StatusCode()
			outcome := "success"
			if status >= 400 {
				outcome = "failure"
			}

			// Sanitize request body if JSON to record non-sensitive audit details (SOC-2 compliance)
			var detail string
			if body := ctx.PostBody(); len(body) > 0 {
				var payload map[string]any
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

			_ = ws.CreateAuditLog(ctx, &tables.TableAuditLog{
				Action:     auditActionForMethod(method),
				Outcome:    outcome,
				Initiator:  auditInitiator(store, ctx),
				Target:     path,
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
