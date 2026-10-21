package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"

	"github.com/gateway/gateway/core/schemas"
	"github.com/gateway/gateway/framework/configstore"
	"github.com/gateway/gateway/framework/configstore/tables"
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
			// Public auth endpoints never enforce RBAC or previous temporary-password restrictions:
			if path == "/api/session/login" ||
				path == "/api/session/register" ||
				path == "/api/session/register/verify" ||
				path == "/api/session/register/resend" ||
				path == "/api/session/forgot-password" ||
				path == "/api/session/verify-otp" ||
				path == "/api/session/reset-password" ||
				path == "/api/session/forgot-username" ||
				path == "/api/session/is-auth-enabled" ||
				path == "/api/session/schedule-demo" ||
				path == "/api/session/teams" ||
				path == "/api/session/logout" {
				next(ctx)
				return
			}
			token := sessionToken(ctx)
			var session *tables.SessionsTable
			if token != "" {
				if sess, err := store.GetSession(ctx, token); err == nil && sess != nil {
					session = sess
					if session.Username != "" {
						if user, err := store.GetUserByUsername(ctx, session.Username); err == nil && user != nil {
							if !user.IsApproved() {
								SendError(ctx, fasthttp.StatusForbidden, "Your account is not approved or is disabled.")
								return
							}
							if user.MustChangePassword {
								allowedForPasswordChange := path == "/api/session/change-temporary-password" ||
									path == "/api/session/is-auth-enabled" ||
									path == "/api/session/logout"
								if !allowedForPasswordChange {
									SendError(ctx, fasthttp.StatusForbidden, "You must change your temporary password before accessing the dashboard.")
									return
								}
							}
						}
					}
				}
			}

			method := string(ctx.Method())
			req := rbac.PathRequirementFor(method, path)
			sections := rbac.SectionRequirementFor(method, path)
			if req == nil && len(sections) == 0 {
				next(ctx)
				return
			}
			if token == "" || session == nil {
				next(ctx)
				return
			}
			isRead := method == "GET" || method == "HEAD" || method == "OPTIONS"
			var grantedSections string
			var userRbacIDs string
			if session.Username != "" {
				if user, err := store.GetUserByUsername(ctx, session.Username); err == nil && user != nil {
					grantedSections = effectiveAllowedSections(ctx, store, user)
					userRbacIDs = user.RbacPermissionIDs
				}
			}

			// Built-in Super Admin with no custom RBAC and no section restrictions stays unrestricted.
			if session.Role == "admin" && strings.TrimSpace(userRbacIDs) == "" && strings.TrimSpace(grantedSections) == "" {
				next(ctx)
				return
			}

			// Built-in users always keep Prompt Repository (chat / folders / sessions / prompts).
			if strings.EqualFold(session.Role, "user") && (path == "/api/prompt-repo" || strings.HasPrefix(path, "/api/prompt-repo/")) {
				next(ctx)
				return
			}

			// Playground helpers: VKs are row-filtered; providers/models/skills feed the chat UI.
			if strings.EqualFold(session.Role, "user") {
				if isRead && (strings.HasPrefix(path, "/api/governance/virtual-keys") ||
					strings.HasPrefix(path, "/api/governance/providers") ||
					strings.HasPrefix(path, "/api/governance/users") ||
					strings.HasPrefix(path, "/api/providers") ||
					strings.HasPrefix(path, "/api/keys") ||
					strings.HasPrefix(path, "/api/models") ||
					strings.HasPrefix(path, "/api/mcp/clients") ||
					strings.HasPrefix(path, "/api/skills")) {
					next(ctx)
					return
				}
			}

			// When no custom RBAC is configured, uncustomized admin and sub_admin get section-based access.
			if strings.TrimSpace(userRbacIDs) == "" && (session.Role == "admin" || session.Role == "sub_admin") {
				if isRead && isSectionGrantedForPath(grantedSections, path) {
					next(ctx)
					return
				}
				if len(sections) > 0 {
					if !rbac.SectionsAllow(grantedSections, sections) {
						SendError(ctx, fasthttp.StatusForbidden, "this section is not enabled for your account")
						return
					}
					if req == nil || isRead {
						next(ctx)
						return
					}
				}
				if req == nil {
					next(ctx)
					return
				}
				perms := rbac.PermissionSet{}
				applySectionFullPermissions(perms, grantedSections)
				if !req.Allowed(perms) {
					SendError(ctx, fasthttp.StatusForbidden, "insufficient permissions")
					return
				}
				next(ctx)
				return
			}

			// For custom RBAC users or user role, check Section gating first if applicable.
			if len(sections) > 0 {
				if strings.TrimSpace(grantedSections) != "" {
					if !rbac.SectionsAllow(grantedSections, sections) {
						SendError(ctx, fasthttp.StatusForbidden, "this section is not enabled for your account")
						return
					}
				} else if strings.EqualFold(session.Role, "user") || strings.EqualFold(session.Role, "sub_admin") {
					if !rbac.SectionsAllow("prompt-repository", sections) {
						SendError(ctx, fasthttp.StatusForbidden, "this section is not enabled for your account")
						return
					}
				}
			}

			// Evaluate granular RBAC permissions.
			if req != nil {
				perms, err := rbac.ResolvePermissions(ctx, ws, session.Role)
				if err != nil {
					SendError(ctx, fasthttp.StatusInternalServerError, "failed to resolve permissions")
					return
				}
				if strings.TrimSpace(userRbacIDs) != "" {
					perms = rbac.PermissionSet{}
					applyRbacPermissionIDs(perms, parseRbacPermissionIDs(userRbacIDs))
					if strings.EqualFold(session.Role, "user") {
						ensureUserPromptRepositoryAccess(perms)
					}
				} else if strings.EqualFold(session.Role, "user") {
					applyUserRolePermissions(perms, grantedSections, userRbacIDs)
				} else if strings.EqualFold(session.Role, "sub_admin") {
					perms = rbac.PermissionSet{}
					applySectionFullPermissions(perms, grantedSections)
				} else if grantedSections != "" && strings.EqualFold(session.Role, "admin") {
					perms = rbac.PermissionSet{}
					applySectionFullPermissions(perms, grantedSections)
				}
				if !req.Allowed(perms) {
					SendError(ctx, fasthttp.StatusForbidden, "insufficient permissions")
					return
				}
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
	if username == "" {
		return r == "admin"
	}
	user, err := store.GetUserByUsername(ctx, username)
	if err != nil || user == nil {
		return r == "admin"
	}
	granted := effectiveAllowedSections(ctx, store, user)
	if r == "admin" && strings.TrimSpace(granted) == "" {
		return true
	}
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
	username := ""
	if h.store != nil && h.store.ConfigStore != nil {
		if token := sessionToken(ctx); token != "" {
			if session, err := h.store.ConfigStore.GetSession(ctx, token); err == nil && session != nil && session.Role != "" {
				role = session.Role
				username = session.Username
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
	granted := ""
	userRbacIDs := ""
	if username != "" && h.store != nil && h.store.ConfigStore != nil {
		if user, err := h.store.ConfigStore.GetUserByUsername(ctx, username); err == nil && user != nil {
			granted = effectiveAllowedSections(ctx, h.store.ConfigStore, user)
			userRbacIDs = user.RbacPermissionIDs
		}
	}
	switch role {
	case "admin":
		if strings.TrimSpace(userRbacIDs) != "" {
			perms = rbac.PermissionSet{}
			applyRbacPermissionIDs(perms, parseRbacPermissionIDs(userRbacIDs))
		} else if strings.TrimSpace(granted) != "" {
			perms = rbac.PermissionSet{}
			applySectionFullPermissions(perms, granted)
		}
	case "sub_admin":
		if strings.TrimSpace(userRbacIDs) != "" {
			perms = rbac.PermissionSet{}
			applyRbacPermissionIDs(perms, parseRbacPermissionIDs(userRbacIDs))
		} else {
			perms = rbac.PermissionSet{}
			applySectionFullPermissions(perms, granted)
		}
	case "user":
		applyUserRolePermissions(perms, granted, userRbacIDs)
	default:
		if strings.TrimSpace(userRbacIDs) != "" {
			perms = rbac.PermissionSet{}
			applyRbacPermissionIDs(perms, parseRbacPermissionIDs(userRbacIDs))
		} else if username != "" && granted != "" {
			applySectionPermissions(perms, granted)
		}
	}
	SendJSON(ctx, map[string]any{"role": role, "permissions": perms})
}
