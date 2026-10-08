package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"

	"github.com/gateway/gateway/core/schemas"
	"github.com/gateway/gateway/framework/configstore"
	"github.com/gateway/gateway/framework/rbac"
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
			method := string(ctx.Method())
			req := rbac.PathRequirementFor(method, path)
			sections := rbac.SectionRequirementFor(method, path)
			if req == nil && len(sections) == 0 {
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
			if session.Role == "user" {
				// Built-in "user" role is scoped to Prompt Repository playground.
				// SessionMiddleware already verifies allowed paths for non-admin sessions.
				// Allow read-only access to virtual keys (which are row-filtered by allowedVKIDsForCaller
				// to only the keys assigned to the user/team/customer), providers, models, and billing blocks
				// needed to run prompts in the playground.
				if (method == "GET" || method == "HEAD") && (strings.HasPrefix(path, "/api/governance/virtual-keys") ||
					strings.HasPrefix(path, "/api/governance/providers") ||
					strings.HasPrefix(path, "/api/providers") ||
					strings.HasPrefix(path, "/api/models") ||
					strings.HasPrefix(path, "/api/mcp/clients") ||
					strings.HasPrefix(path, "/api/skills")) {
					next(ctx)
					return
				}
			}
			if req != nil {
				perms, err := rbac.ResolvePermissions(ctx, ws, session.Role)
				if err != nil {
					SendError(ctx, fasthttp.StatusInternalServerError, "failed to resolve permissions")
					return
				}
				if !req.Allowed(perms) {
					SendError(ctx, fasthttp.StatusForbidden, "insufficient permissions")
					return
				}
			}
			if len(sections) > 0 && !sessionSectionsAllow(ctx, store, session.Role, session.Username, sections) {
				SendError(ctx, fasthttp.StatusForbidden, "this section is not enabled for your account")
				return
			}
			next(ctx)
		}
	}
}

// sessionSectionsAllow applies sidebar / Workspace Access section grants to the API.
// An admin with no saved section list (and the bootstrap admin, who has no user row)
// stays unrestricted. A specific admin whose allowed_sections were customized is
// limited to that list. Built-in "user", sub_admin, and custom roles are always scoped.
func sessionSectionsAllow(ctx context.Context, store configstore.ConfigStore, role, username string, required []string) bool {
	r := strings.ToLower(strings.TrimSpace(role))
	if r == "" {
		return true
	}
	if r == "admin" {
		if username == "" {
			return true
		}
		user, err := store.GetUserByUsername(ctx, username)
		if err != nil || user == nil || strings.TrimSpace(user.AllowedSections) == "" {
			return true
		}
		return rbac.SectionsAllow(user.AllowedSections, required)
	}
	if username == "" {
		return false
	}
	user, err := store.GetUserByUsername(ctx, username)
	if err != nil || user == nil {
		return false
	}
	granted := effectiveAllowedSections(ctx, store, user)
	// Empty grants: prompt-repository only (matches UI workspaceAccess for role=user).
	if strings.TrimSpace(granted) == "" && r == "user" {
		granted = "prompt-repository"
	}
	return rbac.SectionsAllow(granted, required)
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
