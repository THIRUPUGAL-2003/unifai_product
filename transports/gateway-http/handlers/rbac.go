package handlers

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/gateway/gateway/core/schemas"
	"github.com/gateway/gateway/framework/configstore"
	"github.com/gateway/gateway/framework/configstore/tables"
	"github.com/gateway/gateway/framework/rbac"
	"github.com/valyala/fasthttp"
)

type rbacRolePayload struct {
	ID            uint      `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	IsSystemRole  bool      `json:"is_system_role"`
	DAC           string    `json:"dac"`
	PermissionIDs []uint    `json:"permission_ids,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func roleFromRow(row tables.TableRBACRole) rbacRolePayload {
	return rbacRolePayload{
		ID: row.ID, Name: row.Name, Description: row.Description, IsSystemRole: row.IsSystemRole,
		DAC: row.DAC, PermissionIDs: row.ParsedPermissionIDs, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func (h *WorkspaceHandler) listRoles(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	if err := store.EnsureRBACRoles(ctx); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to seed roles")
		return
	}
	rows, err := store.ListRBACRoles(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to list roles")
		return
	}
	roles := make([]rbacRolePayload, 0, len(rows))
	for _, row := range rows {
		roles = append(roles, roleFromRow(row))
	}
	SendJSON(ctx, map[string]any{"roles": roles})
}

func (h *WorkspaceHandler) createRole(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	if err := store.EnsureRBACRoles(ctx); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to seed roles")
		return
	}
	var payload rbacRolePayload
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	payload.Name = strings.ToLower(strings.TrimSpace(payload.Name))
	if payload.Name == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "name is required")
		return
	}
	if payload.DAC == "" {
		payload.DAC = "all-data"
	}
	existingRoles, err := store.ListRBACRoles(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to list roles")
		return
	}
	for _, r := range existingRoles {
		if strings.EqualFold(r.Name, payload.Name) {
			SendError(ctx, fasthttp.StatusConflict, "a role with this name already exists")
			return
		}
	}
	now := time.Now().UTC()
	row := tables.TableRBACRole{
		Name: payload.Name, Description: payload.Description, IsSystemRole: false,
		DAC: payload.DAC, ParsedPermissionIDs: payload.PermissionIDs, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreateRBACRole(ctx, &row); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to save role")
		return
	}
	SendJSON(ctx, map[string]any{"role": roleFromRow(row)})
}

func (h *WorkspaceHandler) getRole(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	if err := store.EnsureRBACRoles(ctx); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to seed roles")
		return
	}
	id, ok := pathUint(ctx, "id")
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid role id")
		return
	}
	row, err := store.GetRBACRole(ctx, id)
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "role not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load role")
		return
	}
	SendJSON(ctx, map[string]any{"role": roleFromRow(*row)})
}

func (h *WorkspaceHandler) updateRole(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	id, ok := pathUint(ctx, "id")
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid role id")
		return
	}
	existing, err := store.GetRBACRole(ctx, id)
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "role not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load role")
		return
	}
	var patch rbacRolePayload
	if err := json.Unmarshal(ctx.PostBody(), &patch); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	prevName := existing.Name
	if newName := strings.ToLower(strings.TrimSpace(patch.Name)); newName != "" && !strings.EqualFold(newName, prevName) {
		// Users and sessions reference roles by name; admin/sub_admin/user are wired into auth.
		if existing.IsSystemRole || isBuiltinRoleName(prevName) {
			SendError(ctx, fasthttp.StatusForbidden, "system roles cannot be renamed")
			return
		}
		if isBuiltinRoleName(newName) {
			SendError(ctx, fasthttp.StatusConflict, "a role with this name already exists")
			return
		}
		others, err := store.ListRBACRoles(ctx)
		if err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, "failed to list roles")
			return
		}
		for _, r := range others {
			if r.ID != existing.ID && strings.EqualFold(r.Name, newName) {
				SendError(ctx, fasthttp.StatusConflict, "a role with this name already exists")
				return
			}
		}
		existing.Name = newName
	}
	if strings.Contains(string(ctx.PostBody()), `"description"`) {
		existing.Description = patch.Description
	}
	if patch.DAC != "" {
		existing.DAC = patch.DAC
	}
	existing.UpdatedAt = time.Now().UTC()
	if err := store.UpdateRBACRole(ctx, existing); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to update role")
		return
	}
	if existing.Name != prevName {
		if err := h.reassignUsersRole(ctx, prevName, existing.Name); err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, "role renamed but failed to update its users: "+err.Error())
			return
		}
	}
	SendJSON(ctx, map[string]any{"role": roleFromRow(*existing)})
}

func isBuiltinRoleName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "admin", "sub_admin", "user":
		return true
	}
	return false
}

// reassignUsersRole moves every user (and their live sessions) from one role name to another.
func (h *WorkspaceHandler) reassignUsersRole(ctx *fasthttp.RequestCtx, from, to string) error {
	if h.store == nil || h.store.ConfigStore == nil {
		return nil
	}
	users, err := h.store.ConfigStore.GetUsers(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		if u == nil || !strings.EqualFold(u.Role, from) {
			continue
		}
		u.Role = to
		u.UpdatedAt = time.Now()
		if err := h.store.ConfigStore.UpdateUser(ctx, u); err != nil {
			return err
		}
		_ = h.store.ConfigStore.UpdateSessionsRoleByUsername(ctx, u.Username, to)
	}
	return nil
}

// callerRole returns the dashboard session role of the request, "admin" when auth is disabled.
func (h *WorkspaceHandler) callerRole(ctx *fasthttp.RequestCtx) string {
	if h.store != nil && h.store.ConfigStore != nil {
		if token := sessionToken(ctx); token != "" {
			if session, err := h.store.ConfigStore.GetSession(ctx, token); err == nil && session != nil {
				return session.Role
			}
		}
	}
	if isLocalAdmin, _ := ctx.UserValue(schemas.IsLocalAdminContextKey).(bool); isLocalAdmin {
		return "admin"
	}
	return ""
}

func (h *WorkspaceHandler) deleteRole(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	id, ok := pathUint(ctx, "id")
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid role id")
		return
	}
	role, err := store.GetRBACRole(ctx, id)
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "role not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load role")
		return
	}
	if role.IsSystemRole || isBuiltinRoleName(role.Name) {
		SendError(ctx, fasthttp.StatusForbidden, "cannot delete system role")
		return
	}
	if err := store.DeleteRBACRole(ctx, id); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to delete role")
		return
	}
	// Users left on a deleted role would resolve to no permissions at all.
	if err := h.reassignUsersRole(ctx, role.Name, "user"); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "role deleted but failed to move its users to the user role: "+err.Error())
		return
	}
	SendJSON(ctx, map[string]string{"message": "deleted"})
}

func (h *WorkspaceHandler) getRolePermissions(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	if err := store.EnsureRBACRoles(ctx); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to seed roles")
		return
	}
	id, ok := pathUint(ctx, "id")
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid role id")
		return
	}
	role, err := store.GetRBACRole(ctx, id)
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "role not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load role")
		return
	}
	wanted := map[uint]bool{}
	for _, permID := range role.ParsedPermissionIDs {
		wanted[permID] = true
	}
	matched := []configstore.RBACPermission{}
	for _, perm := range configstore.RBACPermissions() {
		if wanted[perm.ID] {
			matched = append(matched, perm)
		}
	}
	SendJSON(ctx, map[string]any{"permissions": matched})
}

func (h *WorkspaceHandler) updateRolePermissions(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	id, ok := pathUint(ctx, "id")
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid role id")
		return
	}
	role, err := store.GetRBACRole(ctx, id)
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "role not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load role")
		return
	}
	if strings.EqualFold(role.Name, "admin") {
		SendError(ctx, fasthttp.StatusForbidden, "admin role always has full permissions")
		return
	}
	var body struct {
		PermissionIDs []uint `json:"permission_ids"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &body); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	known := map[uint]configstore.RBACPermission{}
	for _, perm := range configstore.RBACPermissions() {
		known[perm.ID] = perm
	}
	ids := make([]uint, 0, len(body.PermissionIDs))
	seen := map[uint]bool{}
	for _, id := range body.PermissionIDs {
		if _, ok := known[id]; ok && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	// Only the super admin edits roles freely. Anyone else may not touch their own role or
	// grant a permission they do not hold, or RBAC:Update becomes a path to full access.
	if h.store != nil && h.store.ConfigStore != nil {
		if session, scoped := callerIsScopedAdmin(ctx, h.store.ConfigStore); scoped {
			if strings.EqualFold(strings.TrimSpace(role.Name), strings.TrimSpace(session.Role)) {
				SendError(ctx, fasthttp.StatusForbidden, "You cannot change the permissions of your own role")
				return
			}
			callerPerms, err := rbac.ResolvePermissions(ctx, store, session.Role)
			if err != nil {
				SendError(ctx, fasthttp.StatusInternalServerError, "failed to resolve your permissions")
				return
			}
			for _, id := range ids {
				perm := known[id]
				if !rbac.HasPermission(callerPerms, perm.Resource, perm.Operation) {
					SendError(ctx, fasthttp.StatusForbidden, roleBeyondCallerMessage)
					return
				}
			}
		}
	}
	role.ParsedPermissionIDs = ids
	role.UpdatedAt = time.Now().UTC()
	if err := store.UpdateRBACRole(ctx, role); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to update permissions")
		return
	}
	if role.IsSystemRole {
		if err := configstore.MarkRBACRoleCustomized(ctx, store, role.Name); err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, "permissions saved but defaults may be re-applied: "+err.Error())
			return
		}
	}
	SendJSON(ctx, map[string]string{"message": "updated"})
}

func (h *WorkspaceHandler) listRBACResources(ctx *fasthttp.RequestCtx) {
	items := make([]map[string]any, 0, len(configstore.RBACResourceNames))
	for i, name := range configstore.RBACResourceNames {
		items = append(items, map[string]any{"id": i + 1, "name": name})
	}
	SendJSON(ctx, map[string]any{"resources": items})
}

func (h *WorkspaceHandler) listRBACOperations(ctx *fasthttp.RequestCtx) {
	items := make([]map[string]any, 0, len(configstore.RBACOperationNames))
	for i, name := range configstore.RBACOperationNames {
		items = append(items, map[string]any{"id": i + 1, "name": name})
	}
	SendJSON(ctx, map[string]any{"operations": items})
}

func (h *WorkspaceHandler) listRBACPermissions(ctx *fasthttp.RequestCtx) {
	SendJSON(ctx, map[string]any{"permissions": configstore.RBACPermissions()})
}

func (h *WorkspaceHandler) assignUserRole(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.store.ConfigStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	userID := pathID(ctx, "id")
	var body struct {
		RoleID   uint   `json:"role_id"`
		RoleName string `json:"role_name"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &body); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	roleName := strings.TrimSpace(body.RoleName)
	if roleName == "" && body.RoleID == 0 {
		SendError(ctx, fasthttp.StatusBadRequest, "role_id or role_name is required")
		return
	}
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	if err := store.EnsureRBACRoles(ctx); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to seed roles")
		return
	}
	if roleName == "" {
		role, err := store.GetRBACRole(ctx, body.RoleID)
		if err != nil || role == nil {
			SendError(ctx, fasthttp.StatusNotFound, "role not found")
			return
		}
		roleName = role.Name
	} else {
		roles, err := store.ListRBACRoles(ctx)
		if err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, "failed to list roles")
			return
		}
		found := false
		for _, r := range roles {
			if strings.EqualFold(r.Name, roleName) {
				roleName = r.Name
				found = true
				break
			}
		}
		if !found {
			SendError(ctx, fasthttp.StatusNotFound, "role not found")
			return
		}
	}
	user, err := h.store.ConfigStore.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		SendError(ctx, fasthttp.StatusNotFound, "user not found")
		return
	}
	// RBAC:Update alone must not allow granting or removing admin (privilege escalation).
	if (strings.EqualFold(roleName, "admin") || strings.EqualFold(user.Role, "admin")) && h.callerRole(ctx) != "admin" {
		SendError(ctx, fasthttp.StatusForbidden, "only an admin can change admin role assignments")
		return
	}
	if !callerMayManageRole(ctx, h.store.ConfigStore, roleName) || !callerMayManageRole(ctx, h.store.ConfigStore, user.Role) {
		SendError(ctx, fasthttp.StatusForbidden, roleBeyondCallerMessage)
		return
	}
	user.Role = roleName
	if err := h.store.ConfigStore.UpdateUser(ctx, user); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to assign role")
		return
	}
	if err := h.store.ConfigStore.UpdateSessionsRoleByUsername(ctx, user.Username, roleName); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "role assigned but failed to refresh live sessions")
		return
	}
	SendJSON(ctx, map[string]string{"message": "role assigned"})
}
