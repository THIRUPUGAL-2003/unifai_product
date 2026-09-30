package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/configstore/tables"
	"github.com/valyala/fasthttp"
)

type accessProfilePayload struct {
	ID               uint             `json:"id"`
	Name             string           `json:"name"`
	Description      string           `json:"description,omitempty"`
	IsActive         bool             `json:"is_active"`
	Version          int              `json:"version"`
	Tags             []string         `json:"tags,omitempty"`
	ProviderConfigs  []map[string]any `json:"provider_configs,omitempty"`
	Budgets          []map[string]any `json:"budgets,omitempty"`
	RateLimit        map[string]any   `json:"rate_limit,omitempty"`
	CalendarAligned  bool             `json:"calendar_aligned"`
	MCPToolGroups    []map[string]any `json:"mcp_tool_groups,omitempty"`
	MCPServers       []map[string]any `json:"mcp_servers,omitempty"`
	MCPToolOverrides []map[string]any `json:"mcp_tool_overrides,omitempty"`
	VirtualKeyIDs    []string         `json:"virtual_key_ids,omitempty"`
	RoleIDs          []uint           `json:"role_ids,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

func accessProfileFromRow(row tables.TableAccessProfile) accessProfilePayload {
	spec := row.Spec()
	return accessProfilePayload{
		ID: row.ID, Name: row.Name, Description: row.Description, IsActive: row.IsActive,
		Version: row.Version, Tags: row.ParsedTags, CalendarAligned: row.CalendarAligned,
		ProviderConfigs:  specMapSlice(spec, "provider_configs"),
		Budgets:          specMapSlice(spec, "budgets"),
		RateLimit:        specMap(spec, "rate_limit"),
		MCPToolGroups:    specMapSlice(spec, "mcp_tool_groups"),
		MCPServers:       specMapSlice(spec, "mcp_servers"),
		MCPToolOverrides: specMapSlice(spec, "mcp_tool_overrides"),
		VirtualKeyIDs:    specStringSlice(spec, "virtual_key_ids"),
		RoleIDs:          specUintSlice(spec, "role_ids"),
		CreatedAt:        row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func (p accessProfilePayload) toRow() tables.TableAccessProfile {
	return tables.TableAccessProfile{
		ID: p.ID, Name: p.Name, Description: p.Description, IsActive: p.IsActive,
		Version: p.Version, CalendarAligned: p.CalendarAligned, ParsedTags: p.Tags,
		ParsedSpec: map[string]any{
			"provider_configs":   p.ProviderConfigs,
			"budgets":            p.Budgets,
			"rate_limit":         p.RateLimit,
			"mcp_tool_groups":    p.MCPToolGroups,
			"mcp_servers":        p.MCPServers,
			"mcp_tool_overrides": p.MCPToolOverrides,
			"virtual_key_ids":    p.VirtualKeyIDs,
			"role_ids":           p.RoleIDs,
		},
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

// applyAccessProfile writes profile grants onto VKs in DB, then reloads those
// VKs into governance memory so inference/MCP see the change immediately.
func (h *WorkspaceHandler) applyAccessProfile(ctx context.Context, row tables.TableAccessProfile) error {
	if err := propagateAccessProfile(ctx, h.store.ConfigStore, row); err != nil {
		return err
	}
	if h.governanceManager == nil {
		return nil
	}
	for _, vkID := range specStringSlice(row.Spec(), "virtual_key_ids") {
		if _, err := h.governanceManager.ReloadVirtualKey(ctx, vkID); err != nil {
			return fmt.Errorf("reload virtual key %s: %w", vkID, err)
		}
	}
	return nil
}

func (h *WorkspaceHandler) listAccessProfiles(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	rows, err := store.ListAccessProfiles(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to list access profiles")
		return
	}
	search := strings.ToLower(string(ctx.QueryArgs().Peek("search")))
	tagFilter := string(ctx.QueryArgs().Peek("tags"))
	activeFilter := string(ctx.QueryArgs().Peek("is_active"))
	items := make([]accessProfilePayload, 0, len(rows))
	for _, row := range rows {
		item := accessProfileFromRow(row)
		if search != "" && !strings.Contains(strings.ToLower(item.Name), search) && !strings.Contains(strings.ToLower(item.Description), search) {
			continue
		}
		if tagFilter != "" {
			wanted := map[string]bool{}
			for _, tag := range strings.Split(tagFilter, ",") {
				wanted[strings.TrimSpace(tag)] = true
			}
			matched := false
			for _, tag := range item.Tags {
				if wanted[tag] {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if activeFilter == "true" && !item.IsActive {
			continue
		}
		if activeFilter == "false" && item.IsActive {
			continue
		}
		items = append(items, item)
	}
	limit := queryInt(ctx, "limit", 0)
	offset := queryInt(ctx, "offset", 0)
	total := len(items)
	if offset > total {
		offset = total
	}
	end := total
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	if limit > 0 {
		items = items[offset:end]
	}
	SendJSON(ctx, map[string]any{"access_profiles": items, "count": len(items), "total_count": total})
}

func (h *WorkspaceHandler) createAccessProfile(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	var payload accessProfilePayload
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.Name == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "name is required")
		return
	}
	existing, err := store.ListAccessProfiles(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to list access profiles")
		return
	}
	for _, row := range existing {
		if strings.EqualFold(row.Name, payload.Name) {
			SendError(ctx, fasthttp.StatusConflict, "a profile with this name already exists")
			return
		}
	}
	now := time.Now().UTC()
	row := payload.toRow()
	row.ID = 0
	row.Version = 1
	row.CreatedAt = now
	row.UpdatedAt = now
	if err := store.CreateAccessProfile(ctx, &row); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to save access profile")
		return
	}
	if err := h.applyAccessProfile(ctx, row); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("profile saved but failed to apply to virtual keys: %v", err))
		return
	}
	SendJSONWithStatus(ctx, map[string]any{"access_profile": accessProfileFromRow(row)}, fasthttp.StatusCreated)
}

func (h *WorkspaceHandler) getAccessProfile(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	id, ok := pathUint(ctx, "id")
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid profile id")
		return
	}
	row, err := store.GetAccessProfile(ctx, id)
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "profile not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load access profile")
		return
	}
	SendJSON(ctx, map[string]any{
		"access_profile":   accessProfileFromRow(*row),
		"role_attachments": h.buildAccessProfileRoleAttachments(ctx, store, specUintSlice(row.Spec(), "role_ids")),
		"user_copy_count":  countAccessProfileCopies(rowsForCopyCount(store, ctx), row.ID),
	})
}

func (h *WorkspaceHandler) buildAccessProfileRoleAttachments(ctx *fasthttp.RequestCtx, store configstore.WorkspaceStore, roleIDs []uint) []map[string]any {
	if len(roleIDs) == 0 {
		return []map[string]any{}
	}
	roles, err := store.ListRBACRoles(ctx)
	if err != nil {
		return []map[string]any{}
	}
	wanted := map[uint]bool{}
	for _, id := range roleIDs {
		wanted[id] = true
	}
	out := make([]map[string]any, 0)
	for _, role := range roles {
		if wanted[role.ID] {
			out = append(out, map[string]any{"role_id": role.ID, "role_name": role.Name})
		}
	}
	return out
}

func rowsForCopyCount(store configstore.WorkspaceStore, ctx *fasthttp.RequestCtx) []tables.TableAccessProfile {
	rows, err := store.ListAccessProfiles(ctx)
	if err != nil {
		return nil
	}
	return rows
}

func countAccessProfileCopies(rows []tables.TableAccessProfile, profileID uint) int {
	count := 0
	for _, row := range rows {
		spec := row.Spec()
		if parent, ok := spec["parent_profile_id"].(float64); ok && uint(parent) == profileID {
			count++
		}
	}
	return count
}

func (h *WorkspaceHandler) updateAccessProfile(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	id, ok := pathUint(ctx, "id")
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid profile id")
		return
	}
	existing, err := store.GetAccessProfile(ctx, id)
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "profile not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load access profile")
		return
	}
	current := accessProfileFromRow(*existing)
	var patch map[string]any
	if err := json.Unmarshal(ctx.PostBody(), &patch); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	raw, _ := json.Marshal(current)
	merged := map[string]any{}
	_ = json.Unmarshal(raw, &merged)
	for key, value := range patch {
		merged[key] = value
	}
	updatedRaw, _ := json.Marshal(merged)
	var updated accessProfilePayload
	if err := json.Unmarshal(updatedRaw, &updated); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	row := updated.toRow()
	row.ID = existing.ID
	row.CreatedAt = existing.CreatedAt
	row.UpdatedAt = time.Now().UTC()
	row.Version = existing.Version + 1
	if err := store.UpdateAccessProfile(ctx, &row); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to update access profile")
		return
	}
	if err := h.applyAccessProfile(ctx, row); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("profile updated but failed to apply to virtual keys: %v", err))
		return
	}
	SendJSON(ctx, map[string]any{"access_profile": accessProfileFromRow(row)})
}

func (h *WorkspaceHandler) deleteAccessProfile(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	id, ok := pathUint(ctx, "id")
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid profile id")
		return
	}
	row, err := store.GetAccessProfile(ctx, id)
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "profile not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load access profile")
		return
	}
	if err := store.DeleteAccessProfile(ctx, id); isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "profile not found")
		return
	} else if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to delete access profile")
		return
	}
	if h.store != nil && h.store.ConfigStore != nil {
		if err := rollbackAccessProfileMCP(ctx, h.store.ConfigStore, *row); err != nil {
			SendError(ctx, fasthttp.StatusBadGateway, "profile deleted but failed to roll back MCP grants: "+err.Error())
			return
		}
		if h.governanceManager != nil {
			for _, vkID := range specStringSlice(row.Spec(), "virtual_key_ids") {
				_, _ = h.governanceManager.ReloadVirtualKey(ctx, vkID)
			}
		}
	}
	SendJSON(ctx, map[string]string{"message": "deleted"})
}

func (h *WorkspaceHandler) setAccessProfileActive(ctx *fasthttp.RequestCtx, active bool) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	id, ok := pathUint(ctx, "id")
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid profile id")
		return
	}
	row, err := store.GetAccessProfile(ctx, id)
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "profile not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load access profile")
		return
	}
	row.IsActive = active
	row.UpdatedAt = time.Now().UTC()
	if err := store.UpdateAccessProfile(ctx, row); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to update access profile")
		return
	}
	if active && h.store != nil && h.store.ConfigStore != nil {
		if err := h.applyAccessProfile(ctx, *row); err != nil {
			SendError(ctx, fasthttp.StatusBadGateway, "profile activated but failed to propagate to virtual keys: "+err.Error())
			return
		}
	}
	SendJSON(ctx, map[string]any{"access_profile": accessProfileFromRow(*row)})
}

func (h *WorkspaceHandler) activateAccessProfile(ctx *fasthttp.RequestCtx) {
	h.setAccessProfileActive(ctx, true)
}

func (h *WorkspaceHandler) deactivateAccessProfile(ctx *fasthttp.RequestCtx) {
	h.setAccessProfileActive(ctx, false)
}

func (h *WorkspaceHandler) cloneAccessProfile(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	id, ok := pathUint(ctx, "id")
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid profile id")
		return
	}
	existing, err := store.GetAccessProfile(ctx, id)
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "profile not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load access profile")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(ctx.PostBody(), &body)
	clone := *existing
	clone.ID = 0
	now := time.Now().UTC()
	if body.Name != "" {
		clone.Name = body.Name
	} else {
		targetName := existing.Name + " copy"
		profiles, _ := store.ListAccessProfiles(ctx)
		nameExists := false
		for _, p := range profiles {
			if strings.EqualFold(p.Name, targetName) {
				nameExists = true
				break
			}
		}
		if nameExists {
			clone.Name = fmt.Sprintf("%s (%s)", targetName, now.Format("15:04:05"))
		} else {
			clone.Name = targetName
		}
	}
	clone.IsActive = false
	clone.Version = 1
	clone.CreatedAt = now
	clone.UpdatedAt = now
	if err := store.CreateAccessProfile(ctx, &clone); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to clone access profile: "+err.Error())
		return
	}
	SendJSONWithStatus(ctx, map[string]any{"access_profile": accessProfileFromRow(clone)}, fasthttp.StatusCreated)
}

func (h *WorkspaceHandler) listUserAccessProfiles(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	targetUserID := pathID(ctx, "target_user_id")
	userLinks, _ := store.ListVirtualKeysForUser(ctx, targetUserID)
	userVKs := map[string]bool{}
	for _, link := range userLinks {
		userVKs[link.VirtualKeyID] = true
	}
	rows, err := store.ListAccessProfiles(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to list access profiles")
		return
	}
	items := make([]map[string]any, 0)
	for _, row := range rows {
		if !row.IsActive {
			continue
		}
		spec := row.Spec()
		if specUserID, _ := spec["user_id"].(string); specUserID != "" && specUserID != targetUserID {
			continue
		}
		vkIDs := specStringSlice(spec, "virtual_key_ids")
		matched := specUserIDMatches(spec, targetUserID)
		if !matched {
			for _, vkID := range vkIDs {
				if userVKs[vkID] {
					matched = true
					break
				}
			}
		}
		if !matched && len(vkIDs) > 0 {
			continue
		}
		parentID := row.ID
		if p, ok := spec["parent_profile_id"].(float64); ok {
			parentID = uint(p)
		}
		item := map[string]any{
			"id": row.ID, "name": row.Name, "is_active": row.IsActive,
			"user_id": targetUserID, "parent_profile_id": parentID,
			"provider_configs": specMapSlice(spec, "provider_configs"),
			"budgets":          specMapSlice(spec, "budgets"),
			"rate_limit":       specMap(spec, "rate_limit"),
			"virtual_key_ids":  vkIDs,
			"created_at":       row.CreatedAt, "updated_at": row.UpdatedAt,
		}
		items = append(items, item)
	}
	SendJSON(ctx, map[string]any{"access_profiles": items})
}

func specUserIDMatches(spec map[string]any, userID string) bool {
	if spec == nil || userID == "" {
		return false
	}
	if v, ok := spec["user_id"].(string); ok && v == userID {
		return true
	}
	return false
}
