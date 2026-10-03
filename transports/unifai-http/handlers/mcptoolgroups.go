package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/configstore/tables"
	"github.com/unifai/unifai/framework/mcptoolgroups"
	"github.com/valyala/fasthttp"
)

type mcpToolGroupPayload struct {
	ID            uint             `json:"id"`
	Name          string           `json:"name"`
	Description   string           `json:"description,omitempty"`
	Enabled       bool             `json:"enabled"`
	Tools         []map[string]any `json:"tools"`
	VirtualKeyIDs []string         `json:"virtual_key_ids,omitempty"`
	TeamIDs       []string         `json:"team_ids,omitempty"`
	CustomerIDs   []string         `json:"customer_ids,omitempty"`
	UserIDs       []string         `json:"user_ids,omitempty"`
	ProviderNames []string         `json:"provider_names,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

func mcpToolGroupFromRow(row tables.TableMCPToolGroup) mcpToolGroupPayload {
	spec := row.ParsedSpec
	if spec == nil {
		spec = map[string]any{}
	}
	return mcpToolGroupPayload{
		ID: row.ID, Name: row.Name, Description: row.Description, Enabled: row.Enabled,
		Tools: specMapSlice(spec, "tools"), VirtualKeyIDs: specStringSlice(spec, "virtual_key_ids"),
		TeamIDs: specStringSlice(spec, "team_ids"), CustomerIDs: specStringSlice(spec, "customer_ids"),
		UserIDs: specStringSlice(spec, "user_ids"), ProviderNames: specStringSlice(spec, "provider_names"),
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func (p mcpToolGroupPayload) toRow() tables.TableMCPToolGroup {
	return tables.TableMCPToolGroup{
		ID: p.ID, Name: p.Name, Description: p.Description, Enabled: p.Enabled,
		ParsedSpec: map[string]any{
			"tools": p.Tools, "virtual_key_ids": p.VirtualKeyIDs, "team_ids": p.TeamIDs,
			"customer_ids": p.CustomerIDs, "user_ids": p.UserIDs, "provider_names": p.ProviderNames,
		},
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func (h *WorkspaceHandler) listMCPToolGroups(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	rows, err := store.ListMCPToolGroups(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to list tool groups")
		return
	}
	search := strings.ToLower(string(ctx.QueryArgs().Peek("search")))
	items := make([]mcpToolGroupPayload, 0, len(rows))
	for _, row := range rows {
		item := mcpToolGroupFromRow(row)
		if search != "" && !strings.Contains(strings.ToLower(item.Name), search) {
			continue
		}
		items = append(items, item)
	}
	SendJSON(ctx, map[string]any{"tool_groups": items, "count": len(items), "total_count": len(items)})
}

func (h *WorkspaceHandler) createMCPToolGroup(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	var payload mcpToolGroupPayload
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	if strings.TrimSpace(payload.Name) == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "name is required")
		return
	}
	if len(payload.VirtualKeyIDs) == 0 {
		// Empty virtual_key_ids matches every VK at runtime — reject to avoid
		// accidental global tool lockdown via API.
		SendError(ctx, fasthttp.StatusBadRequest, "at least one virtual_key_id is required")
		return
	}
	now := time.Now().UTC()
	if payload.Tools == nil {
		payload.Tools = []map[string]any{}
	}
	row := payload.toRow()
	row.ID = 0
	row.CreatedAt = now
	row.UpdatedAt = now
	if err := store.CreateMCPToolGroup(ctx, &row); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to save tool group")
		return
	}
	_ = mcptoolgroups.ReloadFromStore(ctx, store)
	SendJSONWithStatus(ctx, mcpToolGroupFromRow(row), fasthttp.StatusCreated)
}

func (h *WorkspaceHandler) getMCPToolGroup(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	id, ok := pathUint(ctx, "id")
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid tool group id")
		return
	}
	row, err := store.GetMCPToolGroup(ctx, id)
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "mcp tool group not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load tool group")
		return
	}
	SendJSON(ctx, mcpToolGroupFromRow(*row))
}

func (h *WorkspaceHandler) updateMCPToolGroup(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	id, ok := pathUint(ctx, "id")
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid tool group id")
		return
	}
	existing, err := store.GetMCPToolGroup(ctx, id)
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "mcp tool group not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load tool group")
		return
	}
	current := mcpToolGroupFromRow(*existing)
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
	var updated mcpToolGroupPayload
	if err := json.Unmarshal(updatedRaw, &updated); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	if len(updated.VirtualKeyIDs) == 0 {
		SendError(ctx, fasthttp.StatusBadRequest, "at least one virtual_key_id is required")
		return
	}
	row := updated.toRow()
	row.ID = existing.ID
	row.CreatedAt = existing.CreatedAt
	row.UpdatedAt = time.Now().UTC()
	if err := store.UpdateMCPToolGroup(ctx, &row); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to update tool group")
		return
	}
	_ = mcptoolgroups.ReloadFromStore(ctx, store)
	SendJSON(ctx, mcpToolGroupFromRow(row))
}

func (h *WorkspaceHandler) deleteMCPToolGroup(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	id, ok := pathUint(ctx, "id")
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid tool group id")
		return
	}
	if err := store.DeleteMCPToolGroup(ctx, id); isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "mcp tool group not found")
		return
	} else if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to delete tool group")
		return
	}
	_ = mcptoolgroups.ReloadFromStore(ctx, store)
	SendJSON(ctx, map[string]string{"message": "deleted"})
}

// renameClientInToolGroups rewrites tool group entries keyed by an MCP client's name
// ("oldName-tool", "oldName-*", or {mcp_client_name: oldName}) after the client is renamed,
// so existing groups keep granting the same tools.
func renameClientInToolGroups(ctx context.Context, cs configstore.ConfigStore, oldName, newName string) {
	ws, ok := configstore.AsWorkspaceStore(cs)
	if !ok || ws == nil {
		return
	}
	rows, err := ws.ListMCPToolGroups(ctx)
	if err != nil {
		return
	}
	changed := false
	for i := range rows {
		row := rows[i]
		tools, ok := row.ParsedSpec["tools"]
		if !ok {
			continue
		}
		next, rowChanged := renameClientInToolList(tools, oldName, newName)
		if !rowChanged {
			continue
		}
		row.ParsedSpec["tools"] = next
		if err := ws.UpdateMCPToolGroup(ctx, &row); err == nil {
			changed = true
		}
	}
	if changed {
		_ = mcptoolgroups.ReloadFromStore(ctx, ws)
	}
}

func renameClientInToolList(tools any, oldName, newName string) (any, bool) {
	oldPrefix := oldName + "-"
	renameID := func(v string) (string, bool) {
		trimmed := strings.TrimSpace(v)
		if strings.HasPrefix(trimmed, oldPrefix) {
			return newName + "-" + strings.TrimPrefix(trimmed, oldPrefix), true
		}
		return v, false
	}
	renameMap := func(m map[string]any) bool {
		changed := false
		for _, key := range []string{"name", "tool_name", "id"} {
			if s, ok := m[key].(string); ok {
				if renamed, did := renameID(s); did {
					m[key] = renamed
					changed = true
				}
			}
		}
		if s, ok := m["mcp_client_name"].(string); ok && strings.TrimSpace(s) == oldName {
			m["mcp_client_name"] = newName
			changed = true
		}
		return changed
	}
	changed := false
	switch list := tools.(type) {
	case []any:
		for i, item := range list {
			switch v := item.(type) {
			case string:
				if renamed, did := renameID(v); did {
					list[i] = renamed
					changed = true
				}
			case map[string]any:
				if renameMap(v) {
					changed = true
				}
			}
		}
		return list, changed
	case []map[string]any:
		for _, m := range list {
			if renameMap(m) {
				changed = true
			}
		}
		return list, changed
	case []string:
		for i, v := range list {
			if renamed, did := renameID(v); did {
				list[i] = renamed
				changed = true
			}
		}
		return list, changed
	}
	return tools, false
}
