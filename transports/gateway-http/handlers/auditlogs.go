package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gateway/gateway/framework/configstore"
	"github.com/gateway/gateway/framework/configstore/tables"
	"github.com/valyala/fasthttp"
)

type auditSettingsPayload struct {
	Disabled        bool   `json:"disabled"`
	RetentionDays   int    `json:"retention_days"`
	AutoDelete      bool   `json:"auto_delete"`
	RetentionPeriod string `json:"retention_period"`
	HMACKey         string `json:"hmac_key,omitempty"`
}

var (
	auditLogRetentionOnce sync.Once
	auditLogRetentionStop chan struct{}
)

// StartAuditLogRetentionCleanup runs background auto-delete for Audit Logs
// when auto_delete is enabled (e.g. 1d|7d|30d|90d|180d|365d).
func StartAuditLogRetentionCleanup(store configstore.WorkspaceStore) {
	if store == nil {
		return
	}
	auditLogRetentionOnce.Do(func() {
		auditLogRetentionStop = make(chan struct{})
		go func() {
			initCtx, initCancel := context.WithTimeout(context.Background(), 2*time.Minute)
			_ = RunAuditLogRetentionSweep(initCtx, store)
			initCancel()

			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					sweepCtx, sweepCancel := context.WithTimeout(context.Background(), 2*time.Minute)
					_ = RunAuditLogRetentionSweep(sweepCtx, store)
					sweepCancel()
				case <-auditLogRetentionStop:
					return
				}
			}
		}()
	})
}

func parseAuditRetentionDuration(period string, days int) time.Duration {
	p := strings.ToLower(strings.TrimSpace(period))
	switch p {
	case "24h", "1d":
		return 24 * time.Hour
	case "7d", "1w":
		return 7 * 24 * time.Hour
	case "30d", "1m":
		return 30 * 24 * time.Hour
	case "90d", "3m":
		return 90 * 24 * time.Hour
	case "180d", "6m":
		return 180 * 24 * time.Hour
	case "365d", "1y":
		return 365 * 24 * time.Hour
	}
	if days > 0 {
		return time.Duration(days) * 24 * time.Hour
	}
	return 30 * 24 * time.Hour
}

func RunAuditLogRetentionSweep(ctx context.Context, store configstore.WorkspaceStore) error {
	if store == nil {
		return nil
	}
	row, err := store.GetWorkspaceSetting(ctx, configstore.WorkspaceSettingAudit)
	if err != nil || row == nil || row.Data == "" {
		return nil
	}
	var settings auditSettingsPayload
	if err := json.Unmarshal([]byte(row.Data), &settings); err != nil {
		return nil
	}
	if !settings.AutoDelete {
		return nil
	}
	dur := parseAuditRetentionDuration(settings.RetentionPeriod, settings.RetentionDays)
	if dur <= 0 {
		return nil
	}
	cutoff := time.Now().UTC().Add(-dur)
	return store.ClearAuditLogsInRange(ctx, nil, &cutoff)
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
		SendJSON(ctx, auditSettingsPayload{
			Disabled:        false,
			AutoDelete:      false,
			RetentionPeriod: "30d",
			RetentionDays:   30,
		})
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
	if settings.RetentionPeriod == "" {
		if settings.RetentionDays > 0 {
			settings.RetentionPeriod = fmt.Sprintf("%dd", settings.RetentionDays)
		} else {
			settings.RetentionPeriod = "30d"
		}
	}
	if settings.AutoDelete {
		_ = RunAuditLogRetentionSweep(ctx, store)
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
	if payload.RetentionPeriod != "" {
		switch strings.ToLower(strings.TrimSpace(payload.RetentionPeriod)) {
		case "1d", "24h":
			payload.RetentionDays = 1
		case "7d", "1w":
			payload.RetentionDays = 7
		case "30d", "1m":
			payload.RetentionDays = 30
		case "90d", "3m":
			payload.RetentionDays = 90
		case "180d", "6m":
			payload.RetentionDays = 180
		case "365d", "1y":
			payload.RetentionDays = 365
		}
	} else if payload.RetentionDays > 0 {
		payload.RetentionPeriod = fmt.Sprintf("%dd", payload.RetentionDays)
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
	if payload.AutoDelete {
		_ = RunAuditLogRetentionSweep(ctx, store)
	}
	SendJSON(ctx, payload)
}
