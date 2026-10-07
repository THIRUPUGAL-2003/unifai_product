package handlers

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/gateway/gateway/framework/configstore"
	"github.com/gateway/gateway/framework/configstore/tables"
	"github.com/gateway/gateway/framework/rbac"
	"github.com/valyala/fasthttp"
)

const roleBeyondCallerMessage = "You can only assign or manage roles whose permissions you also have"

// adminCredentialsFromStore returns the bootstrap super admin username and email.
func adminCredentialsFromStore(ctx context.Context, store configstore.ConfigStore) (adminUsername, adminEmail string) {
	if store != nil {
		if authConfig, err := store.GetAuthConfig(ctx); err == nil && authConfig != nil {
			if authConfig.AdminUserName != nil {
				adminUsername = strings.TrimSpace(authConfig.AdminUserName.GetValue())
			}
			if authConfig.AdminEmail != nil {
				adminEmail = strings.TrimSpace(authConfig.AdminEmail.GetValue())
			}
		}
	}
	if adminUsername == "" {
		adminUsername = "admin"
	}
	if adminEmail == "" && strings.Contains(adminUsername, "@") && isValidEmail(adminUsername) {
		adminEmail = adminUsername
	}
	if adminEmail == "" {
		adminEmail = strings.TrimSpace(os.Getenv("ADMIN_EMAIL"))
	}
	return adminUsername, adminEmail
}

// isBuiltinAdminIdentity reports whether a username or email is one of the built-in
// admin's identifiers. Login resolves those to the bootstrap admin, so a stored user
// carrying one could never sign in and would share the admin's lockout counter.
func isBuiltinAdminIdentity(ctx context.Context, store configstore.ConfigStore, identifiers ...string) bool {
	adminName, adminEmail := adminCredentialsFromStore(ctx, store)
	for _, id := range identifiers {
		if isMatchingAdminIdentity(id, adminName, adminEmail) {
			return true
		}
	}
	return false
}

// callerMayManageRole reports whether the caller may give a user this role, or change
// the password / role of a user who has it. The super admin may manage any role; a
// sub_admin only roles whose permissions are a subset of its own, so it cannot mint
// accounts more powerful than itself.
func (h *SessionHandler) callerMayManageRole(ctx *fasthttp.RequestCtx, role string) bool {
	return callerMayManageRole(ctx, h.configStore, role)
}

// callerSession returns the caller's live session, or nil when the request was admitted
// without one (auth disabled, local admin) or the session is gone.
func callerSession(ctx *fasthttp.RequestCtx, store configstore.ConfigStore) *tables.SessionsTable {
	token := sessionToken(ctx)
	if token == "" || store == nil {
		return nil
	}
	session, err := store.GetSession(ctx, token)
	if err != nil || session == nil || session.ExpiresAt.Before(time.Now()) {
		return nil
	}
	return session
}

// callerIsScopedAdmin reports whether the caller manages users without being the super
// admin (sub_admin or a custom role); such callers cannot exceed their own grants.
func callerIsScopedAdmin(ctx *fasthttp.RequestCtx, store configstore.ConfigStore) (*tables.SessionsTable, bool) {
	session := callerSession(ctx, store)
	if session == nil {
		return nil, false
	}
	return session, !strings.EqualFold(strings.TrimSpace(session.Role), "admin")
}

// callerSectionsCover reports whether every requested sidebar section is one the caller
// itself holds, so a scoped admin can never hand out (or take) more access than it has.
func callerSectionsCover(ctx *fasthttp.RequestCtx, store configstore.ConfigStore, requested string) bool {
	session, scoped := callerIsScopedAdmin(ctx, store)
	if !scoped {
		return true
	}
	caller, err := store.GetUserByUsername(ctx, session.Username)
	if err != nil || caller == nil {
		return false
	}
	have := effectiveAllowedSections(ctx, store, caller)
	for _, part := range strings.Split(requested, ",") {
		key := strings.ToLower(strings.TrimSpace(part))
		if key == "" {
			continue
		}
		if !rbac.SectionsAllow(have, []string{key}) {
			return false
		}
	}
	return true
}

// scopedAdminTargetGuard blocks a scoped admin from editing its own grants or managing
// another sub_admin account. Returns an error message, or "" when allowed.
func scopedAdminTargetGuard(ctx *fasthttp.RequestCtx, store configstore.ConfigStore, targetUsername, targetRole string) string {
	session, scoped := callerIsScopedAdmin(ctx, store)
	if !scoped {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(targetRole), "sub_admin") && !strings.EqualFold(session.Username, targetUsername) {
		return "Only the super admin can manage sub-admin accounts"
	}
	return ""
}

func callerMayManageRole(ctx *fasthttp.RequestCtx, store configstore.ConfigStore, role string) bool {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" || role == "user" {
		return true
	}
	token := sessionToken(ctx)
	if token == "" || store == nil {
		// The route's admin check already admitted this caller without a session
		// (auth disabled, local admin).
		return true
	}
	session, err := store.GetSession(ctx, token)
	if err != nil || session == nil || session.ExpiresAt.Before(time.Now()) {
		return false
	}
	callerRole := strings.ToLower(strings.TrimSpace(session.Role))
	if callerRole == "admin" {
		return true
	}
	if role == "admin" {
		return false
	}
	if callerRole == role {
		return true
	}
	ws, ok := configstore.AsWorkspaceStore(store)
	if !ok || ws == nil {
		return false
	}
	callerPerms, err := rbac.ResolvePermissions(ctx, ws, callerRole)
	if err != nil {
		return false
	}
	targetPerms, err := rbac.ResolvePermissions(ctx, ws, role)
	if err != nil {
		return false
	}
	for resource, ops := range targetPerms {
		for op, granted := range ops {
			if granted && !rbac.HasPermission(callerPerms, resource, op) {
				return false
			}
		}
	}
	return true
}
