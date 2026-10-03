package handlers

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/raksha/raksha/framework/logstore"
	"github.com/raksha/raksha/framework/mailer"
	"github.com/valyala/fasthttp"
)

var (
	uninstallAttemptsMu sync.Mutex
	uninstallAttempts   = make(map[string]struct {
		count int
		last  time.Time
	})
	browserAIAgentKeyRotateOnce sync.Once
	browserAIAgentKeyRotateStop chan struct{}
)

func startBrowserAIAgentKeyDailyRotation(manager *logstore.BrowserAIManager) {
	if manager == nil {
		return
	}
	browserAIAgentKeyRotateOnce.Do(func() {
		browserAIAgentKeyRotateStop = make(chan struct{})
		go func() {
			_, _ = manager.AutoRotateDailyAgentUninstallKeys(context.Background())
			ticker := time.NewTicker(1 * time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					_, _ = manager.AutoRotateDailyAgentUninstallKeys(context.Background())
				case <-browserAIAgentKeyRotateStop:
					return
				}
			}
		}()
	})
}

const (
	maxUninstallAttempts   = 5
	uninstallLockoutPeriod = 15 * time.Minute
)

func (h *BrowserAIHandler) listAgents(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	status := string(ctx.QueryArgs().Peek("status"))
	search := string(ctx.QueryArgs().Peek("search"))
	agentType := string(ctx.QueryArgs().Peek("agent_type"))
	limit, _ := strconv.Atoi(string(ctx.QueryArgs().Peek("limit")))
	offset, _ := strconv.Atoi(string(ctx.QueryArgs().Peek("offset")))
	if limit <= 0 {
		limit = 50
	}
	agents, total, err := h.manager.ListAgents(ctx, status, search, limit, offset, agentType)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	activeCount, uninstalledCount, _ := h.manager.CountAgentsStatus(ctx, search, agentType)
	SendJSON(ctx, map[string]any{
		"agents":             agents,
		"total":              total,
		"active_count":       activeCount,
		"uninstalled_count":  uninstalledCount,
		"limit":              limit,
		"offset":             offset,
		"latest_version":     readGuardReleaseVersion(),
		"latest_mac_version": readGuardMacReleaseVersion(),
	})
}

func (h *BrowserAIHandler) agentHeartbeat(ctx *fasthttp.RequestCtx) {
	if !h.verifyGuardSecurity(ctx) {
		return
	}
	h.ensureDB(ctx)
	var body logstore.BrowserAIAgent
	if err := sonic.Unmarshal(ctx.PostBody(), &body); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if strings.TrimSpace(body.ID) == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "agent id is required")
		return
	}
	body.AgentType = logstore.NormalizeBrowserAIAgentType(body.AgentType)
	host := strings.ToLower(strings.TrimSpace(body.Hostname))
	// Laptop-only setups: drop shared network proxy heartbeats (corp-network-proxy)
	// so they cannot reappear after admin delete. Opt back in with BROWSER_AI_ALLOW_NETWORK_AGENTS=1.
	allowNetwork := strings.TrimSpace(os.Getenv("BROWSER_AI_ALLOW_NETWORK_AGENTS"))
	allowNetworkOn := allowNetwork == "1" || strings.EqualFold(allowNetwork, "true") || strings.EqualFold(allowNetwork, "yes")
	if !allowNetworkOn && (body.AgentType == "network" || host == "corp-network-proxy" || strings.HasPrefix(strings.ToLower(strings.TrimSpace(body.ID)), "network-")) {
		SendJSON(ctx, map[string]any{
			"status":  "ignored",
			"reason":  "network agents disabled",
			"command": "",
		})
		return
	}
	if strings.TrimSpace(body.IPAddress) == "" {
		body.IPAddress = ctx.RemoteIP().String()
	}
	agent, err := h.manager.UpsertAgentHeartbeat(ctx, &body)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	settings, _ := h.manager.GetAgentSettings(ctx)
	fleet, _ := h.manager.GetFleetConfig(ctx)
	command := ""
	if agent != nil && (agent.UninstallRequested || agent.Status == logstore.AgentStatusUninstalled || agent.Status == logstore.AgentStatusUninstallPending) {
		command = "uninstall"
	}
	resp := map[string]any{
		"status":       "success",
		"agent":        agent,
		"settings":     settings,
		"fleet_config": fleet,
		"command":      command,
	}
	if command == "" {
		resp["latest_guard_version"] = readGuardReleaseVersion()
		resp["latest_mac_guard_version"] = readGuardMacReleaseVersion()
		if bundle := heartbeatProxyBundle(); bundle != nil {
			resp["proxy_bundle"] = bundle
		}
	}
	SendJSON(ctx, resp)
}

func (h *BrowserAIHandler) getAgentSettings(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	settings, err := h.manager.GetAgentSettings(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]any{"settings": settings}
	if plainKey, err := h.manager.GetCompanyUninstallKeyReveal(ctx); err == nil && plainKey != "" {
		resp["uninstall_key"] = plainKey
	}
	SendJSON(ctx, resp)
}

func (h *BrowserAIHandler) getCompanyUninstallKey(ctx *fasthttp.RequestCtx) {
	if !h.requireGuardAdmin(ctx, "Admin role required to view the company uninstall key") {
		return
	}
	h.ensureDB(ctx)
	plainKey, err := h.manager.GetCompanyUninstallKeyReveal(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusNotFound, err.Error())
		return
	}
	SendJSON(ctx, map[string]any{
		"uninstall_key":  plainKey,
		"key_configured": true,
	})
}

func (h *BrowserAIHandler) saveUninstallKey(ctx *fasthttp.RequestCtx) {
	if !h.requireGuardAdmin(ctx, "Admin role required to change the company uninstall key") {
		return
	}
	h.ensureDB(ctx)
	var req struct {
		Key                 string `json:"key"`
		RequireUninstallKey *bool  `json:"require_uninstall_key"`
		UpdatedBy           string `json:"updated_by"`
	}
	if err := sonic.Unmarshal(ctx.PostBody(), &req); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if strings.TrimSpace(req.Key) == "" && req.RequireUninstallKey == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Provide uninstall key and/or require_uninstall_key")
		return
	}
	settings, err := h.manager.SaveUninstallKey(ctx, req.Key, req.UpdatedBy, req.RequireUninstallKey)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]any{"status": "success", "settings": settings}
	if plainKey, err := h.manager.GetCompanyUninstallKeyReveal(ctx); err == nil && plainKey != "" {
		resp["uninstall_key"] = plainKey
	}
	SendJSON(ctx, resp)
}

func (h *BrowserAIHandler) verifyUninstall(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	clientIP := clientIPAddress(ctx)

	uninstallAttemptsMu.Lock()
	if state, exists := uninstallAttempts[clientIP]; exists && state.count >= maxUninstallAttempts && time.Since(state.last) <= uninstallLockoutPeriod {
		uninstallAttemptsMu.Unlock()
		SendError(ctx, fasthttp.StatusTooManyRequests, "Too many failed uninstall attempts. Locked out for 15 minutes.")
		return
	}
	uninstallAttemptsMu.Unlock()

	var req struct {
		Key     string `json:"key"`
		AgentID string `json:"agent_id"`
	}
	if err := sonic.Unmarshal(ctx.PostBody(), &req); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid JSON payload")
		return
	}
	ok, settings, err := h.manager.VerifyAgentUninstallKey(ctx, req.AgentID, req.Key)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		uninstallAttemptsMu.Lock()
		state := uninstallAttempts[clientIP]
		if time.Since(state.last) > uninstallLockoutPeriod {
			state.count = 0
		}
		state.count++
		state.last = time.Now()
		uninstallAttempts[clientIP] = state
		fails := state.count
		uninstallAttemptsMu.Unlock()

		if fails >= maxUninstallAttempts {
			SendError(ctx, fasthttp.StatusTooManyRequests, "Too many failed uninstall attempts. Locked out for 15 minutes.")
			return
		}
	} else {
		uninstallAttemptsMu.Lock()
		delete(uninstallAttempts, clientIP)
		uninstallAttemptsMu.Unlock()
	}

	SendJSON(ctx, map[string]any{
		"valid":    ok,
		"settings": settings,
	})
}

func (h *BrowserAIHandler) uninstallAgent(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	clientIP := clientIPAddress(ctx)

	uninstallAttemptsMu.Lock()
	if state, exists := uninstallAttempts[clientIP]; exists && state.count >= maxUninstallAttempts && time.Since(state.last) <= uninstallLockoutPeriod {
		uninstallAttemptsMu.Unlock()
		SendError(ctx, fasthttp.StatusTooManyRequests, "Too many failed uninstall attempts. Locked out for 15 minutes.")
		return
	}
	uninstallAttemptsMu.Unlock()

	var req struct {
		AgentID string `json:"agent_id"`
		Key     string `json:"key"`
	}
	if err := sonic.Unmarshal(ctx.PostBody(), &req); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid JSON payload")
		return
	}
	ok, settings, err := h.manager.VerifyAgentUninstallKey(ctx, req.AgentID, req.Key)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		// If neither company nor Guard key works — distinguish "no company key and no agent key"
		agentOK := strings.TrimSpace(req.AgentID) != ""
		if settings != nil && !settings.KeyConfigured && !agentOK {
			SendError(ctx, fasthttp.StatusForbidden, "Set a company uninstall key in Browser AI → Setup, or use this Guard's uninstall key")
			return
		}
		uninstallAttemptsMu.Lock()
		state := uninstallAttempts[clientIP]
		if time.Since(state.last) > uninstallLockoutPeriod {
			state.count = 0
		}
		state.count++
		state.last = time.Now()
		uninstallAttempts[clientIP] = state
		fails := state.count
		uninstallAttemptsMu.Unlock()

		if fails >= maxUninstallAttempts {
			SendError(ctx, fasthttp.StatusTooManyRequests, "Too many failed uninstall attempts. Locked out for 15 minutes.")
			return
		}
		SendError(ctx, fasthttp.StatusForbidden, "Invalid uninstall key")
		return
	}

	uninstallAttemptsMu.Lock()
	delete(uninstallAttempts, clientIP)
	uninstallAttemptsMu.Unlock()
	agent, err := h.manager.MarkAgentUninstalled(ctx, req.AgentID)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	SendJSON(ctx, map[string]any{
		"status":   "success",
		"agent":    agent,
		"settings": settings,
	})
}

func (h *BrowserAIHandler) remoteUninstallAgent(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	id, ok := ctx.UserValue("id").(string)
	if !ok || id == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Missing agent ID")
		return
	}
	var req struct {
		Key string `json:"key"`
	}
	if len(ctx.PostBody()) > 0 {
		if err := sonic.Unmarshal(ctx.PostBody(), &req); err != nil {
			SendError(ctx, fasthttp.StatusBadRequest, "Invalid JSON payload")
			return
		}
	}
	valid, settings, err := h.manager.VerifyAgentUninstallKey(ctx, id, req.Key)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	if !valid {
		_ = settings
		SendError(ctx, fasthttp.StatusForbidden, "Invalid uninstall key — use this Guard's key or the company uninstall key")
		return
	}
	agent, err := h.manager.RequestRemoteUninstall(ctx, id)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}
	guardEvents.notify(id, guardEventUninstall)
	SendJSON(ctx, map[string]any{"status": "success", "agent": agent, "command": "uninstall"})
}

func (h *BrowserAIHandler) getAgentUninstallKey(ctx *fasthttp.RequestCtx) {
	if !h.requireGuardAdmin(ctx, "Admin role required to view Guard uninstall keys") {
		return
	}
	h.ensureDB(ctx)
	id, ok := ctx.UserValue("id").(string)
	if !ok || id == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Missing agent ID")
		return
	}
	plain, agent, err := h.manager.GetAgentUninstallKeyReveal(ctx, id)
	if err != nil {
		SendError(ctx, fasthttp.StatusNotFound, err.Error())
		return
	}
	SendJSON(ctx, map[string]any{
		"agent_id":                 agent.ID,
		"hostname":                 agent.Hostname,
		"uninstall_key":            plain,
		"has_key":                  true,
		"uninstall_key_rotated_at": agent.UninstallKeyRotatedAt,
	})
}

func (h *BrowserAIHandler) rotateAgentUninstallKey(ctx *fasthttp.RequestCtx) {
	if !h.requireGuardAdmin(ctx, "Admin role required to rotate Guard uninstall keys") {
		return
	}
	h.ensureDB(ctx)
	id, ok := ctx.UserValue("id").(string)
	if !ok || id == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Missing agent ID")
		return
	}
	plain, agent, err := h.manager.RotateAgentUninstallKey(ctx, id)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}
	SendJSON(ctx, map[string]any{
		"status":                   "success",
		"agent_id":                 agent.ID,
		"hostname":                 agent.Hostname,
		"uninstall_key":            plain,
		"uninstall_key_rotated_at": agent.UninstallKeyRotatedAt,
	})
}

func (h *BrowserAIHandler) deleteAgent(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	id, ok := ctx.UserValue("id").(string)
	if !ok || id == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Missing agent ID")
		return
	}
	deleted, err := h.manager.DeleteAgents(ctx, []string{id})
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	if deleted == 0 {
		SendError(ctx, fasthttp.StatusNotFound, "Agent not found")
		return
	}
	SendJSON(ctx, map[string]any{"status": "success", "deleted": deleted})
}

func (h *BrowserAIHandler) bulkDeleteAgents(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := sonic.Unmarshal(ctx.PostBody(), &req); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid JSON payload")
		return
	}
	deleted, err := h.manager.DeleteAgents(ctx, req.IDs)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}
	SendJSON(ctx, map[string]any{"status": "success", "deleted": deleted})
}

func (h *BrowserAIHandler) ackRemoteUninstall(ctx *fasthttp.RequestCtx) {
	if !h.verifyGuardSecurity(ctx) {
		return
	}
	h.ensureDB(ctx)
	var req struct {
		AgentID string `json:"agent_id"`
	}
	if err := sonic.Unmarshal(ctx.PostBody(), &req); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid JSON payload")
		return
	}
	agent, err := h.manager.AckRemoteUninstall(ctx, req.AgentID)
	if err != nil {
		if strings.Contains(err.Error(), "not requested") || strings.Contains(err.Error(), "not found") {
			SendError(ctx, fasthttp.StatusForbidden, err.Error())
			return
		}
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	SendJSON(ctx, map[string]any{"status": "success", "agent": agent})
}

// remoteUninstallGrace is how long after an admin-approved uninstall the laptop's
// Windows uninstaller may skip the key prompt.
const remoteUninstallGrace = 15 * time.Minute

// uninstallStatus lets the Guard uninstaller skip the key prompt only when an admin
// already approved removal of this agent from the dashboard.
func (h *BrowserAIHandler) uninstallStatus(ctx *fasthttp.RequestCtx) {
	if !h.verifyGuardSecurity(ctx) {
		return
	}
	h.ensureDB(ctx)
	var req struct {
		AgentID string `json:"agent_id"`
	}
	if err := sonic.Unmarshal(ctx.PostBody(), &req); err != nil || strings.TrimSpace(req.AgentID) == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "agent_id is required")
		return
	}
	agent, err := h.manager.GetAgent(ctx, strings.TrimSpace(req.AgentID))
	if err != nil || agent == nil {
		SendJSON(ctx, map[string]any{"authorized": false})
		return
	}
	SendJSON(ctx, map[string]any{"authorized": remoteUninstallAuthorized(agent, time.Now()), "status": agent.Status})
}

func remoteUninstallAuthorized(agent *logstore.BrowserAIAgent, now time.Time) bool {
	if agent == nil {
		return false
	}
	if agent.UninstallRequested || agent.Status == logstore.AgentStatusUninstallPending {
		return true
	}
	return agent.Status == logstore.AgentStatusUninstalled && agent.UninstalledAt != nil &&
		now.Sub(*agent.UninstalledAt) <= remoteUninstallGrace
}

type warningEmailPayload struct {
	To            string `json:"to"`
	Subject       string `json:"subject"`
	Message       string `json:"message"`
	AgentID       string `json:"agent_id"`
	AgentHostname string `json:"agent_hostname"`
	AllowedCount  *int64 `json:"allowed_count"`
	BlockedCount  *int64 `json:"blocked_count"`
	WarnCount     *int64 `json:"warn_count"`
	RedactCount   *int64 `json:"redact_count"`
}

func (h *BrowserAIHandler) sendWarningEmail(ctx *fasthttp.RequestCtx) {
	if h.configStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "Configuration store not available")
		return
	}
	h.ensureDB(ctx)
	var payload warningEmailPayload
	if err := sonic.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid JSON payload")
		return
	}
	to := strings.TrimSpace(payload.To)
	if to == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Recipient email address ('to') is required")
		return
	}
	if !strings.Contains(to, "@") || !strings.Contains(to, ".") {
		SendError(ctx, fasthttp.StatusBadRequest, "Recipient email address looks invalid")
		return
	}
	subject := strings.TrimSpace(payload.Subject)
	if subject == "" {
		subject = "Raksha Security Policy Warning"
	}
	message := strings.TrimSpace(payload.Message)
	if message == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Warning message cannot be empty")
		return
	}

	sentBy := ""
	if u := ctx.UserValue("user"); u != nil {
		if m, ok := u.(map[string]any); ok {
			if e, ok := m["email"].(string); ok {
				sentBy = e
			} else if n, ok := m["username"].(string); ok {
				sentBy = n
			}
		}
	}
	if sentBy == "" {
		sentBy = auditInitiator(h.configStore, ctx)
	}
	recordEmail := func(status, detail string) {
		if h.manager == nil {
			return
		}
		_ = h.manager.RecordWarningEmail(ctx, &logstore.BrowserAIWarningEmailLog{
			ToEmail:       to,
			Subject:       subject,
			Message:       message,
			AgentID:       strings.TrimSpace(payload.AgentID),
			AgentHostname: strings.TrimSpace(payload.AgentHostname),
			SentBy:        sentBy,
			Status:        status,
			ErrorDetail:   detail,
		})
	}
	failBeforeSend := func(code int, msg string) {
		recordEmail("failed", msg)
		SendError(ctx, code, msg)
	}

	smtpConfig, err := h.configStore.GetSMTPConfig(ctx)
	if err != nil {
		failBeforeSend(fasthttp.StatusInternalServerError, "Failed to retrieve SMTP configuration: "+err.Error())
		return
	}
	if smtpConfig == nil || !smtpConfig.Enabled {
		failBeforeSend(fasthttp.StatusBadRequest, "SMTP is not enabled. Please configure and enable SMTP in Settings → Security before sending security warning emails.")
		return
	}
	if strings.TrimSpace(smtpConfig.Host) == "" {
		failBeforeSend(fasthttp.StatusBadRequest, "SMTP host is empty. Set Host in Settings → Security SMTP settings.")
		return
	}
	if strings.TrimSpace(smtpConfig.FromEmail) == "" && strings.TrimSpace(smtpConfig.Username) == "" {
		failBeforeSend(fasthttp.StatusBadRequest, "SMTP From Email is empty. Set From Email in Settings → Security SMTP settings.")
		return
	}

	deviceInfo := ""
	if !strings.Contains(message, "DEVICE REPORT") && (payload.AgentHostname != "" || payload.AgentID != "") {
		deviceInfo = fmt.Sprintf("\n\n--- TARGET DEVICE ---\nHostname: %s\nAgent ID: %s", payload.AgentHostname, payload.AgentID)
	}

	// Avoid duplicating stats when the UI message already embeds the report summary.
	statsBlock := ""
	if !strings.Contains(message, "VIOLATION SUMMARY") &&
		(payload.AllowedCount != nil || payload.BlockedCount != nil || payload.WarnCount != nil || payload.RedactCount != nil) {
		var allowed, blocked, warn, redact int64
		if payload.AllowedCount != nil {
			allowed = *payload.AllowedCount
		}
		if payload.BlockedCount != nil {
			blocked = *payload.BlockedCount
		}
		if payload.WarnCount != nil {
			warn = *payload.WarnCount
		}
		if payload.RedactCount != nil {
			redact = *payload.RedactCount
		}
		statsBlock = fmt.Sprintf(
			"\n\n--- VIOLATION SUMMARY ---\nAllowed: %d\nBlocked: %d\nWarned: %d\nRedacted: %d",
			allowed, blocked, warn, redact,
		)
	}

	body := fmt.Sprintf(
		"Raksha Browser Guard — Security Warning Report\n"+
			"==================================================\n\n"+
			"%s"+
			"%s"+
			"%s\n\n"+
			"==================================================\n"+
			"Sent via Raksha Enterprise Security Console\n"+
			"(SMTP: Settings → Security)\n",
		message,
		deviceInfo,
		statsBlock,
	)

	if err := mailer.Send(smtpToMailer(smtpConfig), mailer.Message{
		To:      to,
		Subject: subject,
		Body:    body,
	}); err != nil {
		recordEmail("failed", err.Error())
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to send warning email: %v", err))
		return
	}

	recordEmail("sent", "")
	if h.manager != nil {
		// Remember contact email on the agent for next Insights warning (never over an admin-saved one).
		if aid := strings.TrimSpace(payload.AgentID); aid != "" {
			_, _ = h.manager.RememberAgentContactEmail(ctx, aid, to)
		}
	}

	SendJSON(ctx, map[string]any{
		"status":  "success",
		"message": "Security warning email sent successfully",
		"to":      to,
	})
}

func (h *BrowserAIHandler) getInsightStats(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	if h.manager == nil {
		SendJSON(ctx, map[string]any{"stats": []any{}, "totals": map[string]int64{}})
		return
	}
	stats, totals, err := h.manager.GetAgentInsightStats(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	SendJSON(ctx, map[string]any{
		"stats":  stats,
		"totals": totals,
	})
}

func (h *BrowserAIHandler) updateAgentContactEmail(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	id, ok := ctx.UserValue("id").(string)
	if !ok || strings.TrimSpace(id) == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "agent id is required")
		return
	}
	var body struct {
		ContactEmail string `json:"contact_email"`
	}
	if err := sonic.Unmarshal(ctx.PostBody(), &body); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid JSON payload")
		return
	}
	agent, err := h.manager.UpdateAgentContactEmail(ctx, id, body.ContactEmail)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}
	SendJSON(ctx, map[string]any{"status": "success", "agent": agent})
}
