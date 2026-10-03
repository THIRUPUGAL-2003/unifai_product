package handlers

import (
	"encoding/json"
	"strings"

	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/framework/logstore"
	"github.com/unifai/unifai/framework/queryscope"
	"github.com/valyala/fasthttp"
	"gorm.io/gorm"
)

// noVirtualKeyAccess is a VK ID filter value that matches no log row.
const noVirtualKeyAccess = "__no_virtual_key_access__"

// Non-admin dashboard sessions only see logs of the virtual keys they can access (the same
// set the Virtual Keys page shows them). Admin / sub-admin sessions and API callers without
// a session are unaffected.
func (h *LoggingHandler) allowedVKs(ctx *fasthttp.RequestCtx) (map[string]bool, bool) {
	if h == nil || h.config == nil {
		return nil, false
	}
	return allowedVKIDsForRequest(ctx, h.config.ConfigStore)
}

// scopeVKQuery narrows the virtual_key_ids filter to the caller's keys before the handler parses it.
func (h *LoggingHandler) scopeVKQuery(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		if allowed, filter := h.allowedVKs(ctx); filter {
			ctx.QueryArgs().Set("virtual_key_ids", scopedVKFilter(string(ctx.QueryArgs().Peek("virtual_key_ids")), allowed))
		}
		next(ctx)
	}
}

func scopedVKFilter(requested string, allowed map[string]bool) string {
	var out []string
	if strings.TrimSpace(requested) == "" {
		for id := range allowed {
			out = append(out, id)
		}
	} else {
		for _, id := range parseCommaSeparated(requested) {
			if allowed[id] {
				out = append(out, id)
			}
		}
	}
	if len(out) == 0 {
		return noVirtualKeyAccess
	}
	return strings.Join(out, ",")
}

// scopeSingleLog hides a single log row (LLM or MCP) whose virtual key the caller cannot access.
func (h *LoggingHandler) scopeSingleLog(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		allowed, filter := h.allowedVKs(ctx)
		next(ctx)
		if !filter || ctx.Response.StatusCode() != fasthttp.StatusOK {
			return
		}
		var row struct {
			VirtualKeyID *string `json:"virtual_key_id"`
		}
		if err := json.Unmarshal(ctx.Response.Body(), &row); err != nil || row.VirtualKeyID == nil || !allowed[*row.VirtualKeyID] {
			ctx.Response.ResetBody()
			SendError(ctx, fasthttp.StatusNotFound, "log not found")
		}
	}
}

// scopeSessionLogs drops rows of other virtual keys from a session's log list.
func (h *LoggingHandler) scopeSessionLogs(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		allowed, filter := h.allowedVKs(ctx)
		next(ctx)
		if !filter || ctx.Response.StatusCode() != fasthttp.StatusOK {
			return
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
			return
		}
		var logs []json.RawMessage
		if err := json.Unmarshal(body["logs"], &logs); err != nil {
			return
		}
		kept := make([]json.RawMessage, 0, len(logs))
		for _, raw := range logs {
			var row struct {
				VirtualKeyID *string `json:"virtual_key_id"`
			}
			if json.Unmarshal(raw, &row) == nil && row.VirtualKeyID != nil && allowed[*row.VirtualKeyID] {
				kept = append(kept, raw)
			}
		}
		if len(kept) == 0 {
			ctx.Response.ResetBody()
			SendError(ctx, fasthttp.StatusNotFound, "session not found")
			return
		}
		body["logs"], _ = json.Marshal(kept)
		out, err := json.Marshal(body)
		if err != nil {
			return
		}
		ctx.Response.SetBody(out)
	}
}

// scopeSessionSummary only returns a session's totals when every row in it belongs to
// one of the caller's virtual keys.
func (h *LoggingHandler) scopeSessionSummary(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		allowed, filter := h.allowedVKs(ctx)
		if filter {
			sessionID, _ := ctx.UserValue("session_id").(string)
			result, err := h.logManager.GetSessionLogs(ctx, sessionID, &logstore.PaginationOptions{Limit: sessionLogPageLimit, SortBy: "timestamp", Order: "asc"})
			if err != nil || result == nil || len(result.Logs) == 0 {
				SendError(ctx, fasthttp.StatusNotFound, "session not found")
				return
			}
			for _, l := range result.Logs {
				if l.VirtualKeyID == nil || !allowed[*l.VirtualKeyID] {
					SendError(ctx, fasthttp.StatusNotFound, "session not found")
					return
				}
			}
		}
		next(ctx)
	}
}

// vkQueryScope restricts every log-store read on ctx (raw tables and filter matviews, which
// all carry virtual_key_id) to the caller's virtual keys.
func vkQueryScope(allowed map[string]bool) queryscope.QueryScope {
	ids := make([]string, 0, len(allowed))
	for id := range allowed {
		ids = append(ids, id)
	}
	return func(db *gorm.DB) *gorm.DB {
		if len(ids) == 0 {
			return db.Where("1 = 0")
		}
		return db.Where("virtual_key_id IN ?", ids)
	}
}

// scopeFilterData limits every filter dropdown (models, teams, users, metadata, ...) to values
// seen on the caller's own virtual keys, so non-admins can't enumerate other tenants.
func (h *LoggingHandler) scopeFilterData(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		allowed, filter := h.allowedVKs(ctx)
		if filter {
			ctx.SetUserValue(schemas.UnifAIContextKeyQueryScope, vkQueryScope(allowed))
		}
		next(ctx)
		if !filter || ctx.Response.StatusCode() != fasthttp.StatusOK {
			return
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
			return
		}
		raw, ok := body["virtual_keys"]
		if !ok {
			return
		}
		var keys []map[string]any
		if err := json.Unmarshal(raw, &keys); err != nil {
			return
		}
		kept := make([]map[string]any, 0, len(keys))
		for _, k := range keys {
			if id, _ := k["id"].(string); allowed[id] {
				kept = append(kept, k)
			}
		}
		body["virtual_keys"], _ = json.Marshal(kept)
		out, err := json.Marshal(body)
		if err != nil {
			return
		}
		ctx.Response.SetBody(out)
	}
}

// adminOnlyLogs blocks workspace-wide log operations (delete, cost recalculation) for
// callers limited to their own virtual keys.
func (h *LoggingHandler) adminOnlyLogs(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		if _, filter := h.allowedVKs(ctx); filter {
			SendError(ctx, fasthttp.StatusForbidden, "only workspace admins can delete or recalculate logs")
			return
		}
		next(ctx)
	}
}
