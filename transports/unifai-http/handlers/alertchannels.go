package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/unifai/unifai/framework/configstore/tables"
	"github.com/valyala/fasthttp"
)

type alertChannelPayload struct {
	ID        uint           `json:"id"`
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Enabled   bool           `json:"enabled"`
	Config    map[string]any `json:"config"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

func alertChannelFromRow(row tables.TableAlertChannel) alertChannelPayload {
	return alertChannelPayload{
		ID: row.ID, Name: row.Name, Type: row.Type, Enabled: row.Enabled,
		Config: row.ParsedConfig, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func (h *WorkspaceHandler) listAlertChannels(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	rows, err := store.ListAlertChannels(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to list alert channels")
		return
	}
	items := make([]alertChannelPayload, 0, len(rows))
	for _, row := range rows {
		items = append(items, alertChannelFromRow(row))
	}
	SendJSON(ctx, map[string]any{"channels": items, "count": len(items)})
}

func (h *WorkspaceHandler) createAlertChannel(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	var payload alertChannelPayload
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	if strings.TrimSpace(payload.Name) == "" || strings.TrimSpace(payload.Type) == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "name and type are required")
		return
	}
	switch payload.Type {
	case "webhook", "slack", "email", "pagerduty":
	default:
		SendError(ctx, fasthttp.StatusBadRequest, "type must be webhook, slack, email, or pagerduty")
		return
	}
	now := time.Now().UTC()
	if payload.Config == nil {
		payload.Config = map[string]any{}
	}
	row := tables.TableAlertChannel{
		Name: payload.Name, Type: payload.Type, Enabled: payload.Enabled,
		ParsedConfig: payload.Config, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreateAlertChannel(ctx, &row); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to save alert channel")
		return
	}
	SendJSONWithStatus(ctx, alertChannelFromRow(row), fasthttp.StatusCreated)
}

func (h *WorkspaceHandler) updateAlertChannel(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	id, ok := pathUint(ctx, "id")
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid alert channel id")
		return
	}
	existing, err := store.GetAlertChannel(ctx, id)
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "alert channel not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load alert channel")
		return
	}
	var patch alertChannelPayload
	if err := json.Unmarshal(ctx.PostBody(), &patch); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	if patch.Name != "" {
		existing.Name = patch.Name
	}
	if patch.Type != "" {
		existing.Type = patch.Type
	}
	if patch.Config != nil {
		existing.ParsedConfig = patch.Config
	}
	existing.Enabled = patch.Enabled
	existing.UpdatedAt = time.Now().UTC()
	if err := store.UpdateAlertChannel(ctx, existing); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to update alert channel")
		return
	}
	SendJSON(ctx, alertChannelFromRow(*existing))
}

func (h *WorkspaceHandler) deleteAlertChannel(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	id, ok := pathUint(ctx, "id")
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid alert channel id")
		return
	}
	if err := store.DeleteAlertChannel(ctx, id); isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "alert channel not found")
		return
	} else if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to delete alert channel")
		return
	}
	SendJSON(ctx, map[string]string{"message": "deleted"})
}

func (h *WorkspaceHandler) testAlertChannel(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	id, ok := pathUint(ctx, "id")
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid alert channel id")
		return
	}
	row, err := store.GetAlertChannel(ctx, id)
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "alert channel not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load alert channel")
		return
	}
	SendJSON(ctx, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("test event accepted for %s channel %q", row.Type, row.Name),
	})
}
