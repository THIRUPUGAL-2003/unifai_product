package handlers

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gateway/gateway/framework/configstore"
	"github.com/gateway/gateway/framework/configstore/tables"
	"github.com/valyala/fasthttp"
)

type auditSettingsPayload struct {
	Disabled      bool   `json:"disabled"`
	RetentionDays int    `json:"retention_days"`
	HMACKey       string `json:"hmac_key,omitempty"`
}

const (
	auditExportPageSize = 500
	auditExportMaxRows  = 50000
)

func auditLogQueryFromArgs(ctx *fasthttp.RequestCtx) configstore.AuditLogQuery {
	query := configstore.AuditLogQuery{
		Search:  strings.ToLower(strings.TrimSpace(string(ctx.QueryArgs().Peek("search")))),
		Action:  strings.TrimSpace(string(ctx.QueryArgs().Peek("action"))),
		Outcome: strings.TrimSpace(string(ctx.QueryArgs().Peek("outcome"))),
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
	return query
}

func (h *WorkspaceHandler) listAuditLogs(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	query := auditLogQueryFromArgs(ctx)
	query.Limit = queryInt(ctx, "limit", 50)
	query.Offset = queryInt(ctx, "offset", 0)
	rows, total, err := store.ListAuditLogs(ctx, query)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to list audit logs")
		return
	}
	SendJSON(ctx, map[string]any{"logs": rows, "count": len(rows), "total_count": total})
}

func (h *WorkspaceHandler) deleteAuditLogs(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	since, until, label, ok := logDeleteWindow(ctx)
	if !ok {
		return
	}
	if err := store.ClearAuditLogsInRange(ctx, since, until); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to clear audit logs: "+err.Error())
		return
	}
	msg := "All audit logs cleared"
	if label != "" {
		msg = "Audit logs deleted " + label
	}
	SendJSON(ctx, map[string]any{"status": "success", "message": msg})
}

func (h *WorkspaceHandler) bulkDeleteAuditLogs(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	var payload struct {
		IDs []any `json:"ids"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request body")
		return
	}
	if len(payload.IDs) == 0 {
		SendError(ctx, fasthttp.StatusBadRequest, "ids array is required")
		return
	}
	var uintIDs []uint
	for _, raw := range payload.IDs {
		switch v := raw.(type) {
		case float64:
			uintIDs = append(uintIDs, uint(v))
		case string:
			if n, err := strconv.ParseUint(strings.TrimSpace(v), 10, 64); err == nil {
				uintIDs = append(uintIDs, uint(n))
			}
		}
	}
	if len(uintIDs) == 0 {
		SendError(ctx, fasthttp.StatusBadRequest, "no valid IDs provided")
		return
	}
	deleted, err := store.DeleteAuditLogsByIDs(ctx, uintIDs)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to delete audit logs: "+err.Error())
		return
	}
	SendJSON(ctx, map[string]any{"status": "success", "deleted": deleted})
}

func (h *WorkspaceHandler) deleteAuditLog(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	idStr, _ := ctx.UserValue("id").(string)
	id, err := strconv.ParseUint(strings.TrimSpace(idStr), 10, 64)
	if err != nil || id == 0 {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid audit log id")
		return
	}
	deleted, err := store.DeleteAuditLogsByIDs(ctx, []uint{uint(id)})
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to delete audit log: "+err.Error())
		return
	}
	if deleted == 0 {
		SendError(ctx, fasthttp.StatusNotFound, "audit log not found")
		return
	}
	SendJSON(ctx, map[string]any{"status": "success", "deleted": deleted})
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
	if limit > auditExportMaxRows {
		limit = auditExportMaxRows
	}
	query := auditLogQueryFromArgs(ctx)
	if query.End == nil {
		// Pin the window so rows written during paging don't shift offsets.
		now := time.Now().UTC()
		query.End = &now
	}
	rows := make([]tables.TableAuditLog, 0, min(limit, auditExportPageSize))
	for len(rows) < limit {
		query.Limit = min(auditExportPageSize, limit-len(rows))
		query.Offset = len(rows)
		page, _, err := store.ListAuditLogs(ctx, query)
		if err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, "failed to export audit logs")
			return
		}
		rows = append(rows, page...)
		if len(page) < query.Limit {
			break
		}
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
