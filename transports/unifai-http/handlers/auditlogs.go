package handlers

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/unifai/unifai/framework/configstore"
	"github.com/valyala/fasthttp"
)

type auditSettingsPayload struct {
	Disabled      bool   `json:"disabled"`
	RetentionDays int    `json:"retention_days"`
	HMACKey       string `json:"hmac_key,omitempty"`
}

func (h *WorkspaceHandler) listAuditLogs(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	query := configstore.AuditLogQuery{
		Search:  strings.ToLower(string(ctx.QueryArgs().Peek("search"))),
		Action:  string(ctx.QueryArgs().Peek("action")),
		Outcome: string(ctx.QueryArgs().Peek("outcome")),
		Limit:   queryInt(ctx, "limit", 50),
		Offset:  queryInt(ctx, "offset", 0),
	}
	if start := string(ctx.QueryArgs().Peek("start")); start != "" {
		if parsed, err := time.Parse(time.RFC3339, start); err == nil {
			query.Start = &parsed
		}
	}
	if end := string(ctx.QueryArgs().Peek("end")); end != "" {
		if parsed, err := time.Parse(time.RFC3339, end); err == nil {
			query.End = &parsed
		}
	}
	rows, total, err := store.ListAuditLogs(ctx, query)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to list audit logs")
		return
	}
	SendJSON(ctx, map[string]any{"logs": rows, "count": len(rows), "total_count": total})
}

func (h *WorkspaceHandler) exportAuditLogs(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	limit := 10000
	if l := ctx.QueryArgs().GetUintOrZero("limit"); l > 0 {
		limit = l
	}
	query := configstore.AuditLogQuery{Limit: limit, Offset: 0}
	if start := string(ctx.QueryArgs().Peek("start")); start != "" {
		if parsed, err := time.Parse(time.RFC3339, start); err == nil {
			query.Start = &parsed
		}
	}
	if end := string(ctx.QueryArgs().Peek("end")); end != "" {
		if parsed, err := time.Parse(time.RFC3339, end); err == nil {
			query.End = &parsed
		}
	}
	rows, _, err := store.ListAuditLogs(ctx, query)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to export audit logs")
		return
	}
	format := string(ctx.QueryArgs().Peek("format"))
	if format == "jsonl" {
		ctx.SetContentType("application/x-ndjson")
		enc := json.NewEncoder(ctx)
		for _, row := range rows {
			_ = enc.Encode(row)
		}
		return
	}
	SendJSON(ctx, map[string]any{"logs": rows, "count": len(rows)})
}

func (h *WorkspaceHandler) getAuditSettings(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	row, err := store.GetWorkspaceSetting(ctx, configstore.WorkspaceSettingAudit)
	if isStoreNotFound(err) {
		SendJSON(ctx, auditSettingsPayload{RetentionDays: 365})
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load audit settings")
		return
	}
	var settings auditSettingsPayload
	if err := json.Unmarshal([]byte(row.Data), &settings); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to parse audit settings")
		return
	}
	SendJSON(ctx, settings)
}

func (h *WorkspaceHandler) updateAuditSettings(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	var payload auditSettingsPayload
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	if payload.RetentionDays < 0 {
		SendError(ctx, fasthttp.StatusBadRequest, "retention_days cannot be negative")
		return
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to save audit settings")
		return
	}
	if err := store.UpsertWorkspaceSetting(ctx, configstore.WorkspaceSettingAudit, string(raw)); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to save audit settings")
		return
	}
	ReloadAuditSettingsFromStore(store)
	SendJSON(ctx, payload)
}
