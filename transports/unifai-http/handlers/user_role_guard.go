package handlers

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/rbac"
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
