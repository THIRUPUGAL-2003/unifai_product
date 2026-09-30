package handlers

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/configstore/tables"
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
	if patch.Name != "" {
		existing.Name = strings.ToLower(strings.TrimSpace(patch.Name))
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
	SendJSON(ctx, map[string]any{"role": roleFromRow(*existing)})
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
	if role.IsSystemRole || strings.EqualFold(role.Name, "admin") || strings.EqualFold(role.Name, "user") {
		SendError(ctx, fasthttp.StatusForbidden, "cannot delete system role")
		return
	}
	if err := store.DeleteRBACRole(ctx, id); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to delete role")
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
	var body struct {
		PermissionIDs []uint `json:"permission_ids"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &body); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	role.ParsedPermissionIDs = body.PermissionIDs
	role.UpdatedAt = time.Now().UTC()
	if err := store.UpdateRBACRole(ctx, role); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to update permissions")
		return
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
	if roleName == "" && body.RoleID != 0 {
		store := h.requireStore(ctx)
		if store == nil {
			return
		}
		role, err := store.GetRBACRole(ctx, body.RoleID)
		if err != nil {
			SendError(ctx, fasthttp.StatusNotFound, "role not found")
			return
		}
		roleName = role.Name
	}
	if roleName == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "role_id or role_name is required")
		return
	}
	user, err := h.store.ConfigStore.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		SendError(ctx, fasthttp.StatusNotFound, "user not found")
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
