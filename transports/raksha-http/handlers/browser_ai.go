package handlers

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/router"
	raksha "github.com/raksha/raksha/core"
	"github.com/raksha/raksha/core/schemas"
	"github.com/raksha/raksha/framework/configstore"
	"github.com/raksha/raksha/framework/logstore"
	"github.com/raksha/raksha/transports/raksha-http/lib"
	"github.com/valyala/fasthttp"
)

type BrowserAIHandler struct {
	configStore configstore.ConfigStore
	config      *lib.Config
	client      *raksha.Raksha
	manager     *logstore.BrowserAIManager

	// Short TTL cache — 1000+ employees hammer GetRules on every AI-bot prompt.
	rulesCacheMu sync.RWMutex
	rulesCache   []logstore.BrowserGuardRule
	rulesCacheAt time.Time
}

func NewBrowserAIHandler(configStore configstore.ConfigStore, config *lib.Config, client *raksha.Raksha) *BrowserAIHandler {
	manager := logstore.NewBrowserAIManager(nil)
	h := &BrowserAIHandler{
		configStore: configStore,
		config:      config,
		client:      client,
		manager:     manager,
	}
	h.initDB()
	startBrowserAIAttachmentCleanup(manager)
	startBrowserAILogRetentionCleanup(manager)
	startBrowserAIAgentKeyDailyRotation(manager)
	return h
}

// getRulesCached returns active rules with a 2s in-memory cache (hot intercept path).
func (h *BrowserAIHandler) getRulesCached(ctx context.Context) ([]logstore.BrowserGuardRule, error) {
	h.rulesCacheMu.RLock()
	if h.rulesCache != nil && time.Since(h.rulesCacheAt) < 2*time.Second {
		out := h.rulesCache
		h.rulesCacheMu.RUnlock()
		return out, nil
	}
	h.rulesCacheMu.RUnlock()

	rules, err := h.manager.GetRules(ctx)
	if err != nil {
		return rules, err
	}
	h.rulesCacheMu.Lock()
	h.rulesCache = rules
	h.rulesCacheAt = time.Now()
	h.rulesCacheMu.Unlock()
	return rules, nil
}

// invalidateRulesCache clears the short TTL after admin rule writes.
func (h *BrowserAIHandler) invalidateRulesCache() {
	h.rulesCacheMu.Lock()
	h.rulesCache = nil
	h.rulesCacheAt = time.Time{}
	h.rulesCacheMu.Unlock()
}
func (h *BrowserAIHandler) initDB() {
	if h.configStore != nil {
		if db := h.configStore.DB(); db != nil {
			h.manager.SetDB(db)
		}
	}
}

func (h *BrowserAIHandler) ensureDB(ctx *fasthttp.RequestCtx) {
	if h.manager == nil || h.manager.GetDB() == nil {
		h.initDB()
	}
}

func (h *BrowserAIHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.RakshaHTTPMiddleware) {
	r.GET("/api/browser-ai/logs", lib.ChainMiddlewares(h.getLogs, middlewares...))
	r.GET("/api/browser-ai/logs/stats", lib.ChainMiddlewares(h.getLogStats, middlewares...))
	r.DELETE("/api/browser-ai/logs", lib.ChainMiddlewares(h.deleteLogs, middlewares...))
	r.POST("/api/browser-ai/logs/bulk-delete", lib.ChainMiddlewares(h.bulkDeleteLogs, middlewares...))
	r.DELETE("/api/browser-ai/logs/{id}", lib.ChainMiddlewares(h.deleteLog, middlewares...))

	r.GET("/api/browser-ai/search-logs", lib.ChainMiddlewares(h.getSearchLogs, middlewares...))
	r.POST("/api/browser-ai/search-logs", lib.ChainMiddlewares(h.recordSearchLog, middlewares...))
	r.DELETE("/api/browser-ai/search-logs", lib.ChainMiddlewares(h.deleteSearchLogs, middlewares...))
	r.POST("/api/browser-ai/search-logs/bulk-delete", lib.ChainMiddlewares(h.bulkDeleteSearchLogs, middlewares...))
	r.DELETE("/api/browser-ai/search-logs/{id}", lib.ChainMiddlewares(h.deleteSearchLog, middlewares...))

	r.GET("/api/browser-ai/rules", lib.ChainMiddlewares(h.getRules, middlewares...))
	r.POST("/api/browser-ai/rules", lib.ChainMiddlewares(h.createRule, middlewares...))
	r.POST("/api/browser-ai/rules/import", lib.ChainMiddlewares(h.importRules, middlewares...))
	r.PUT("/api/browser-ai/rules/{id}", lib.ChainMiddlewares(h.updateRule, middlewares...))
	r.DELETE("/api/browser-ai/rules/{id}", lib.ChainMiddlewares(h.deleteRule, middlewares...))

	r.GET("/api/browser-ai/controls", lib.ChainMiddlewares(h.getControls, middlewares...))
	r.PUT("/api/browser-ai/controls", lib.ChainMiddlewares(h.updateControls, middlewares...))

	r.GET("/api/browser-ai/targets", lib.ChainMiddlewares(h.getTargets, middlewares...))
	r.POST("/api/browser-ai/targets/import", lib.ChainMiddlewares(h.importTargets, middlewares...))
	r.POST("/api/browser-ai/targets", lib.ChainMiddlewares(h.createTarget, middlewares...))
	r.PUT("/api/browser-ai/targets/{id}", lib.ChainMiddlewares(h.updateTarget, middlewares...))
	r.DELETE("/api/browser-ai/targets/{id}", lib.ChainMiddlewares(h.deleteTarget, middlewares...))
	r.GET("/api/browser-ai/proxy.pac", lib.ChainMiddlewares(h.getProxyPAC, middlewares...))
	r.GET("/api/browser-ai/pac", lib.ChainMiddlewares(h.getProxyPAC, middlewares...))
	r.GET("/api/browser-ai/setup/download.zip", lib.ChainMiddlewares(h.downloadSetupPackage, middlewares...))
	r.HEAD("/api/browser-ai/setup/download.zip", lib.ChainMiddlewares(h.downloadSetupPackage, middlewares...))
	r.GET("/api/browser-ai/setup/download-windows.zip", lib.ChainMiddlewares(h.downloadSetupPackage, middlewares...))
	r.HEAD("/api/browser-ai/setup/download-windows.zip", lib.ChainMiddlewares(h.downloadSetupPackage, middlewares...))
	r.GET("/api/browser-ai/setup/download-mac.zip", lib.ChainMiddlewares(h.downloadSetupPackage, middlewares...))
	r.HEAD("/api/browser-ai/setup/download-mac.zip", lib.ChainMiddlewares(h.downloadSetupPackage, middlewares...))
	r.POST("/api/browser-ai/setup/rebuild", lib.ChainMiddlewares(h.rebuildSetupPackages, middlewares...))
	r.GET("/api/browser-ai/setup/info", lib.ChainMiddlewares(h.getSetupInfo, middlewares...))
	r.GET("/api/browser-ai/setup/rebuild-history", lib.ChainMiddlewares(h.getRebuildHistory, middlewares...))
	r.GET("/api/browser-ai/setup/proxy-bundle.json", lib.ChainMiddlewares(h.getProxyBundleInfo, middlewares...))
	r.GET("/api/browser-ai/setup/proxy-bundle.zip", lib.ChainMiddlewares(h.downloadProxyBundle, middlewares...))

	r.GET("/api/browser-ai/setup/Raksha_Guard_Setup.exe", lib.ChainMiddlewares(h.downloadSetupExe, middlewares...))
	r.HEAD("/api/browser-ai/setup/Raksha_Guard_Setup.exe", lib.ChainMiddlewares(h.downloadSetupExe, middlewares...))

	r.GET("/api/browser-ai/agents", lib.ChainMiddlewares(h.listAgents, middlewares...))
	r.POST("/api/browser-ai/agents/heartbeat", lib.ChainMiddlewares(h.agentHeartbeat, middlewares...))
	r.POST("/api/browser-ai/agents/wait-command", lib.ChainMiddlewares(h.agentWaitCommand, middlewares...))
	r.GET("/api/browser-ai/fleet-config", lib.ChainMiddlewares(h.getFleetConfig, middlewares...))
	r.PUT("/api/browser-ai/fleet-config", lib.ChainMiddlewares(h.putFleetConfig, middlewares...))
	r.GET("/api/browser-ai/agents/settings", lib.ChainMiddlewares(h.getAgentSettings, middlewares...))
	r.GET("/api/browser-ai/agents/uninstall-key", lib.ChainMiddlewares(h.getCompanyUninstallKey, middlewares...))
	r.PUT("/api/browser-ai/agents/uninstall-key", lib.ChainMiddlewares(h.saveUninstallKey, middlewares...))
	r.POST("/api/browser-ai/agents/uninstall-verify", lib.ChainMiddlewares(h.verifyUninstall, middlewares...))
	r.POST("/api/browser-ai/agents/uninstall", lib.ChainMiddlewares(h.uninstallAgent, middlewares...))
	r.POST("/api/browser-ai/agents/uninstall-ack", lib.ChainMiddlewares(h.ackRemoteUninstall, middlewares...))
	r.POST("/api/browser-ai/agents/uninstall-status", lib.ChainMiddlewares(h.uninstallStatus, middlewares...))
	r.POST("/api/browser-ai/agents/{id}/remote-uninstall", lib.ChainMiddlewares(h.remoteUninstallAgent, middlewares...))
	r.POST("/api/browser-ai/agents/{id}/pause", lib.ChainMiddlewares(h.pauseAgent, middlewares...))
	r.POST("/api/browser-ai/agents/{id}/resume", lib.ChainMiddlewares(h.resumeAgent, middlewares...))
	r.POST("/api/browser-ai/agents/{id}/allow-reinstall", lib.ChainMiddlewares(h.allowReinstallAgent, middlewares...))
	r.GET("/api/browser-ai/agents/{id}/uninstall-key", lib.ChainMiddlewares(h.getAgentUninstallKey, middlewares...))
	r.POST("/api/browser-ai/agents/{id}/uninstall-key/rotate", lib.ChainMiddlewares(h.rotateAgentUninstallKey, middlewares...))
	r.POST("/api/browser-ai/agents/bulk-delete", lib.ChainMiddlewares(h.bulkDeleteAgents, middlewares...))
	r.DELETE("/api/browser-ai/agents/{id}", lib.ChainMiddlewares(h.deleteAgent, middlewares...))
	r.POST("/api/browser-ai/send-warning-email", lib.ChainMiddlewares(h.sendWarningEmail, middlewares...))
	r.GET("/api/browser-ai/insights/stats", lib.ChainMiddlewares(h.getInsightStats, middlewares...))
	r.PUT("/api/browser-ai/agents/{id}/contact-email", lib.ChainMiddlewares(h.updateAgentContactEmail, middlewares...))

	r.POST("/api/browser-ai/intercept", lib.ChainMiddlewares(h.intercept, middlewares...))
	r.POST("/api/browser-ai/intercept-file", lib.ChainMiddlewares(h.interceptFile, middlewares...))
	r.GET("/api/browser-ai/attachments/{id}", lib.ChainMiddlewares(h.getAttachment, middlewares...))
	r.POST("/api/browser-ai/rules/test-bot", lib.ChainMiddlewares(h.testAIGuardBot, middlewares...))
	r.POST("/api/browser-ai/rules/generate-regex", lib.ChainMiddlewares(h.generateRegexFromPolicy, middlewares...))
	r.GET("/api/browser-ai/ollama-models", lib.ChainMiddlewares(h.getOllamaModels, middlewares...))
}

func (h *BrowserAIHandler) getLogs(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	if h.manager != nil {
		h.manager.ApplyPromptLogAutoDelete(ctx)
	}

	platform := string(ctx.QueryArgs().Peek("platform"))
	status := string(ctx.QueryArgs().Peek("status"))
	action := string(ctx.QueryArgs().Peek("action"))
	search := string(ctx.QueryArgs().Peek("search"))
	limitStr := string(ctx.QueryArgs().Peek("limit"))
	offsetStr := string(ctx.QueryArgs().Peek("offset"))

	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	logs, total, err := h.manager.GetLogs(ctx, platform, status, action, search, limit, offset)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	SendJSON(ctx, map[string]any{
		"logs":   logs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *BrowserAIHandler) getLogStats(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	if h.manager == nil {
		SendJSON(ctx, logstore.BrowserAILogStats{})
		return
	}
	stats, err := h.manager.GetLogStats(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	SendJSON(ctx, stats)
}

// logDeleteWindow resolves the delete range shared by prompt-log and search-log clears.
//
//	period=1d|7d|30d  — delete logs OLDER than N days (keeps the most recent N days)
//	period=all or ""  — delete everything
//	date=YYYY-MM-DD   — delete logs for that calendar day (server local day)
//
// label describes what was removed, e.g. "older than 7 days".
func logDeleteWindow(ctx *fasthttp.RequestCtx) (since, until *time.Time, label string, ok bool) {
	period := strings.ToLower(strings.TrimSpace(string(ctx.QueryArgs().Peek("period"))))
	dateStr := strings.TrimSpace(string(ctx.QueryArgs().Peek("date")))
	now := time.Now()

	olderThan := func(days int, text string) (*time.Time, *time.Time, string, bool) {
		cutoff := now.Add(-time.Duration(days) * 24 * time.Hour)
		return nil, &cutoff, text, true
	}

	switch {
	case dateStr != "":
		day, err := time.ParseInLocation("2006-01-02", dateStr, now.Location())
		if err != nil {
			SendError(ctx, fasthttp.StatusBadRequest, "Invalid date (use YYYY-MM-DD)")
			return nil, nil, "", false
		}
		start := day
		end := day.Add(24 * time.Hour)
		return &start, &end, "for " + dateStr, true
	case period == "1d" || period == "day":
		return olderThan(1, "older than 1 day")
	case period == "7d" || period == "week" || period == "weekly":
		return olderThan(7, "older than 7 days")
	case period == "30d" || period == "month" || period == "monthly":
		return olderThan(30, "older than 30 days")
	case period == "" || period == "all":
		return nil, nil, "", true
	default:
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid period (use 1d, 7d, 30d, or all)")
		return nil, nil, "", false
	}
}

// parseLogIDs reads {"ids": [...]} and returns the trimmed, de-duplicated IDs.
func parseLogIDs(ctx *fasthttp.RequestCtx) ([]string, bool) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := sonic.Unmarshal(ctx.PostBody(), &body); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid JSON payload")
		return nil, false
	}
	seen := make(map[string]struct{}, len(body.IDs))
	ids := make([]string, 0, len(body.IDs))
	for _, id := range body.IDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		SendError(ctx, fasthttp.StatusBadRequest, "ids is required")
		return nil, false
	}
	if len(ids) > 1000 {
		SendError(ctx, fasthttp.StatusBadRequest, "Too many ids (max 1000 per request)")
		return nil, false
	}
	return ids, true
}

// deleteLogs clears prompt logs in PostgreSQL. See logDeleteWindow for query params.
func (h *BrowserAIHandler) deleteLogs(ctx *fasthttp.RequestCtx) {
	since, until, label, ok := logDeleteWindow(ctx)
	if !ok {
		return
	}
	h.ensureDB(ctx)
	if h.manager == nil || h.manager.GetDB() == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "Log database is not available")
		return
	}

	if err := h.manager.ClearLogsInRange(ctx, since, until); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	msg := "Browser AI logs cleared"
	if label != "" {
		msg = "Prompt logs deleted " + label
	}
	SendJSON(ctx, map[string]any{"status": "success", "message": msg})
}

// bulkDeleteLogs deletes selected prompt logs by ID. Body: {"ids": ["..."]}.
func (h *BrowserAIHandler) bulkDeleteLogs(ctx *fasthttp.RequestCtx) {
	ids, ok := parseLogIDs(ctx)
	if !ok {
		return
	}
	h.deletePromptLogIDs(ctx, ids)
}

// deleteLog deletes a single prompt log.
func (h *BrowserAIHandler) deleteLog(ctx *fasthttp.RequestCtx) {
	id, _ := ctx.UserValue("id").(string)
	id = strings.TrimSpace(id)
	if id == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "id is required")
		return
	}
	h.deletePromptLogIDs(ctx, []string{id})
}

func (h *BrowserAIHandler) deletePromptLogIDs(ctx *fasthttp.RequestCtx, ids []string) {
	h.ensureDB(ctx)
	if h.manager == nil || h.manager.GetDB() == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "Log database is not available")
		return
	}
	deleted, err := h.manager.DeleteLogsByIDs(ctx, ids)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	if deleted == 0 && len(ids) == 1 {
		SendError(ctx, fasthttp.StatusNotFound, "Prompt log not found")
		return
	}
	SendJSON(ctx, map[string]any{"status": "success", "deleted": deleted})
}
