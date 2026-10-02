package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"

	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/rbac"
	"github.com/valyala/fasthttp"
)

var auditDisabled atomic.Bool

// ReloadAuditSettingsFromStore caches whether audit logging is disabled.
func ReloadAuditSettingsFromStore(store configstore.WorkspaceStore) {
	if store == nil {
		auditDisabled.Store(false)
		return
	}
	row, err := store.GetWorkspaceSetting(context.Background(), configstore.WorkspaceSettingAudit)
	if err != nil {
		auditDisabled.Store(false)
		return
	}
	var payload auditSettingsPayload
	if err := json.Unmarshal([]byte(row.Data), &payload); err != nil {
		auditDisabled.Store(false)
		return
	}
	auditDisabled.Store(payload.Disabled)
}

func isAuditDisabled() bool {
	return auditDisabled.Load()
}

// RBACMiddleware enforces workspace RBAC for dashboard session requests.
func RBACMiddleware(store configstore.ConfigStore) func(fasthttp.RequestHandler) fasthttp.RequestHandler {
	ws, _ := configstore.AsWorkspaceStore(store)
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			if ws == nil {
				next(ctx)
				return
			}
			path := string(ctx.Path())
			if !strings.HasPrefix(path, "/api/") {
				next(ctx)
				return
			}
			req := rbac.PathRequirementFor(string(ctx.Method()), path)
			if req == nil {
				next(ctx)
				return
			}
			token := sessionToken(ctx)
			if token == "" {
				next(ctx)
				return
			}
			session, err := store.GetSession(ctx, token)
			if err != nil || session == nil {
				next(ctx)
				return
			}
			if session.Role == "admin" {
				next(ctx)
				return
			}
			perms, err := rbac.ResolvePermissions(ctx, ws, session.Role)
			if err != nil {
				SendError(ctx, fasthttp.StatusInternalServerError, "failed to resolve permissions")
				return
			}
			allowed := false
			if len(req.AnyOfResources) > 0 {
				for _, resource := range req.AnyOfResources {
					if rbac.HasPermission(perms, resource, req.Operation) {
						allowed = true
						break
					}
				}
			} else {
				allowed = rbac.HasPermission(perms, req.Resource, req.Operation)
			}
			if !allowed {
				SendError(ctx, fasthttp.StatusForbidden, "insufficient permissions")
				return
			}
			next(ctx)
		}
	}
}

func sessionToken(ctx *fasthttp.RequestCtx) string {
	if authHeader := string(ctx.Request.Header.Peek("Authorization")); strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return string(ctx.Request.Header.Cookie("token"))
}

func (h *WorkspaceHandler) getMyRBACPermissions(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	// Never default to admin — unauthenticated / missing-session callers must not
	// receive the full permission catalog (privilege-inflation risk).
	role := ""
	if h.store != nil && h.store.ConfigStore != nil {
		if token := sessionToken(ctx); token != "" {
			if session, err := h.store.ConfigStore.GetSession(ctx, token); err == nil && session != nil && session.Role != "" {
				role = session.Role
			}
		}
	}
	if role == "" {
		// Auth disabled (loopback / ALLOW_OPEN_AUTH): the auth middleware marks the
		// request as local admin and there is no session to read a role from.
		if isLocalAdmin, _ := ctx.UserValue(schemas.IsLocalAdminContextKey).(bool); isLocalAdmin {
			role = "admin"
		}
	}
	if role == "" {
		SendError(ctx, fasthttp.StatusUnauthorized, "Unauthorized")
		return
	}
	perms, err := rbac.ResolvePermissions(ctx, store, role)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to resolve permissions")
		return
	}
	SendJSON(ctx, map[string]any{"role": role, "permissions": perms})
}
