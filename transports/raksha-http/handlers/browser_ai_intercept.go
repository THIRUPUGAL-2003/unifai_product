package handlers

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/raksha/raksha/framework/logstore"
	"github.com/valyala/fasthttp"
)

func (h *BrowserAIHandler) intercept(ctx *fasthttp.RequestCtx) {
	if !h.verifyGuardSecurity(ctx) {
		return
	}
	h.ensureDB(ctx)

	var payload struct {
		Platform      string         `json:"platform"`
		Prompt        string         `json:"prompt"`
		ClientIP      string         `json:"client_ip"`
		AgentID       string         `json:"agent_id"`
		AgentHostname string         `json:"agent_hostname"`
		AgentType     string         `json:"agent_type"`
		UploadImages  []string       `json:"upload_images"`
		Metadata      map[string]any `json:"metadata"`
	}

	if err := sonic.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if len(payload.Prompt) > browserAIMaxPromptChars {
		SendError(ctx, fasthttp.StatusBadRequest, "Prompt payload exceeds maximum allowed size")
		return
	}

	if payload.ClientIP == "" {
		payload.ClientIP = string(ctx.RemoteIP().String())
	}
	if payload.Platform == "" {
		payload.Platform = "Browser AI"
	}
	if payload.Metadata == nil {
		payload.Metadata = make(map[string]any)
	}
	if strings.TrimSpace(payload.AgentID) != "" {
		payload.Metadata["agent_id"] = strings.TrimSpace(payload.AgentID)
	}
	if strings.TrimSpace(payload.AgentHostname) != "" {
		payload.Metadata["agent_hostname"] = strings.TrimSpace(payload.AgentHostname)
	}
	agentType := strings.TrimSpace(payload.AgentType)
	if agentType == "" {
		if v, ok := payload.Metadata["agent_type"].(string); ok {
			agentType = v
		}
	}
	if agentType != "" {
		payload.Metadata["agent_type"] = logstore.NormalizeBrowserAIAgentType(agentType)
	}

	evalOnly := metadataBool(payload.Metadata, "evaluation_only")
	if evalOnly {
		allowed, action, ruleTriggered, ruleWarning, evalError, securityVerdict := h.evaluateGuardOnly(ctx, payload.Prompt, payload.UploadImages)
		forwardPrompt := payload.Prompt
		if action == "Redacted" || action == "Warned" {
			forwardPrompt = logstore.FormatWarnedForwardPrompt(payload.Prompt, ruleWarning)
		}
		SendJSON(ctx, map[string]any{
			"status":             "success",
			"allowed":            allowed,
			"action":             action,
			"rule_triggered":     ruleTriggered,
			"warning_message":    ruleWarning,
			"forward_prompt":     forwardPrompt,
			"redacted_prompt":    forwardPrompt,
			"eval_error":         evalError,
			"security_verdict":   securityVerdict,
			"security_message":   securityVerdictMessage(securityVerdict, ruleTriggered),
			"predicted_category": securityVerdictCategory(securityVerdict),
		})
		return
	}

	logEntry, ruleWarning, err := h.manager.InterceptPrompt(ctx, payload.Platform, payload.Prompt, payload.ClientIP, payload.Metadata)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	allowed := logEntry.Action == "Allowed" || logEntry.Action == "Redacted" || logEntry.Action == "Warned"
	isViolationBlock := !allowed

	// File-upload audit logs: proxy already decided allow/block. Do not re-run AI Guard Bot
	// or Reply Bot on "[FILE UPLOAD] …" — that corrupts Allowed upload rows in Prompt Logs.
	uploadScan := metadataBool(payload.Metadata, "upload_scan")
	if strings.HasPrefix(strings.TrimSpace(payload.Prompt), "[FILE UPLOAD]") ||
		strings.HasPrefix(strings.TrimSpace(payload.Prompt), "[VOICE UPLOAD]") {
		uploadScan = true
	}
	if uploadScan {
		SendJSON(ctx, map[string]any{
			"status":             "success",
			"allowed":            allowed,
			"action":             logEntry.Action,
			"rule_triggered":     logEntry.RuleTriggered,
			"warning_message":    ruleWarning,
			"redacted_prompt":    payload.Prompt,
			"forward_prompt":     payload.Prompt,
			"risk_score":         logEntry.RiskScore,
			"predictive_risk":    logEntry.PredictiveRisk,
			"predicted_category": logEntry.PredictedCategory,
			"reply_text":         "",
			"reply_bot_provider": "",
			"reply_bot_model":    "",
			"eval_error":         "",
			"log":                logEntry,
		})
		return
	}

	evalError := ""
	securityVerdict := "not_evaluated" // clear | violation | warning | eval_failed | misconfigured | not_evaluated
	aiBotCheckedOK := false
	// Evaluate AI Guard Bot whenever the prompt is still allowed (Allowed or Warned).
	// A regex WARN must not short-circuit a stronger AI Guard Bot BLOCK policy.
	if allowed {
		// Never run AI bots on IDE/wire junk — avoids false CRITICAL blocks and multi-second LLM waits.
		if logstore.IsOpaqueOrWirePrompt(payload.Prompt) {
			securityVerdict = "not_evaluated"
		} else {
			rules, _ := h.getRulesCached(ctx)
			evalContent := buildGuardEvalContent(payload.Prompt, getMetadataString(payload.Metadata, "extracted_text"))
			readyBots, misconfigBlock := collectAIBotRulesForEval(rules, payload.UploadImages)
			if len(misconfigBlock) > 0 {
				rule := misconfigBlock[0]
				logEntry.Action = "Blocked"
				logEntry.Status = fmt.Sprintf("Blocked (%s — AI Guard Bot misconfigured)", rule.Name)
				logEntry.RiskScore = 95
				logEntry.PredictiveRisk = "CRITICAL"
				logEntry.PredictedCategory = "AI_GUARD_BOT_MISCONFIGURED"
				logEntry.RuleTriggered = rule.Name
				ruleWarning = strings.TrimSpace(rule.WarningMessage)
				if ruleWarning == "" {
					ruleWarning = "AI Guard Bot rule is incomplete (evaluation prompt required)."
				}
				allowed = false
				isViolationBlock = true
				evalError = "ai guard bot misconfigured: evaluation prompt is required"
				securityVerdict = "misconfigured"
				_ = h.manager.UpdateLogRuleViolation(ctx, logEntry.ID, logEntry.Action, logEntry.Status, logEntry.RuleTriggered, logEntry.RiskScore, logEntry.PredictiveRisk, logEntry.PredictedCategory)
			} else if len(readyBots) > 0 {
				// Parallel bots: wall time ≈ slowest bot (not sum). Cap + shared budget for enterprise scale.
				for _, res := range h.evalAIBotRulesParallel(evalContent, payload.UploadImages, readyBots) {
					if res.skipped || (!res.violated && res.evalErr == "") {
						if !res.skipped && res.evalErr == "" && !res.violated {
							aiBotCheckedOK = true
							if securityVerdict == "not_evaluated" || securityVerdict == "clear" || securityVerdict == "eval_failed" {
								securityVerdict = "clear"
								evalError = ""
							}
						}
						continue
					}
					if res.evalErr != "" {
						evalError = res.evalErr
						ruleAction := logstore.NormalizeGuardRuleAction(res.rule.Action)
						logEntry.RuleTriggered = res.rule.Name
						if ruleAction == "BLOCK" {
							// Fail closed for BLOCK bots: unreachable Ollama must not silently allow.
							logEntry.Action = "Blocked"
							logEntry.Status = fmt.Sprintf("Blocked (%s — AI Guard Bot eval failed)", res.rule.Name)
							logEntry.PredictedCategory = "AI_GUARD_BOT_EVAL_ERROR"
							logEntry.RiskScore = 90
							logEntry.PredictiveRisk = "HIGH"
							securityVerdict = "eval_failed_blocked"
							evalError = res.evalErr + " (blocked — fail-closed on eval error)"
							_ = h.manager.UpdateLogRuleViolation(ctx, logEntry.ID, logEntry.Action, logEntry.Status, logEntry.RuleTriggered, logEntry.RiskScore, logEntry.PredictiveRisk, logEntry.PredictedCategory)
							continue
						}
						logEntry.Status = fmt.Sprintf("Allowed (%s — AI Guard Bot eval failed)", res.rule.Name)
						logEntry.PredictedCategory = "AI_GUARD_BOT_EVAL_ERROR"
						securityVerdict = "eval_failed"
						_ = h.manager.UpdateLogRuleViolation(ctx, logEntry.ID, logEntry.Action, logEntry.Status, logEntry.RuleTriggered, logEntry.RiskScore, logEntry.PredictiveRisk, logEntry.PredictedCategory)
						continue
					}
					if !res.violated {
						continue
					}
					ruleAction := logstore.NormalizeGuardRuleAction(res.rule.Action)
					sevScore, sevLabel := logstore.GuardSeverityScore(res.rule.Severity)
					if ruleAction == "BLOCK" {
						logEntry.Action = "Blocked"
						logEntry.Status = fmt.Sprintf("Blocked (%s)", res.rule.Name)
						logEntry.RiskScore = sevScore
						if logEntry.RiskScore < 80 {
							logEntry.RiskScore = 90
						}
						logEntry.PredictiveRisk = sevLabel
						if logEntry.PredictiveRisk == "LOW" || logEntry.PredictiveRisk == "MEDIUM" {
							logEntry.PredictiveRisk = "HIGH"
						}
						logEntry.PredictedCategory = "AI_GUARD_BOT_VIOLATION"
						logEntry.RuleTriggered = res.rule.Name
						ruleWarning = strings.TrimSpace(res.rule.WarningMessage)
						if strings.Contains(strings.ToLower(ruleWarning), "evaluation failed") {
							ruleWarning = ""
						}
						if ruleWarning == "" {
							ruleWarning = "This request was blocked by Raksha Guard."
						}
						allowed = false
						isViolationBlock = true
						evalError = ""
						securityVerdict = "violation"
						_ = h.manager.UpdateLogRuleViolation(ctx, logEntry.ID, logEntry.Action, logEntry.Status, logEntry.RuleTriggered, logEntry.RiskScore, logEntry.PredictiveRisk, logEntry.PredictedCategory)
						break
					} else if ruleAction == "REDACT" {
						logEntry.Action = "Redacted"
						logEntry.Status = fmt.Sprintf("Redacted (%s)", res.rule.Name)
						logEntry.RiskScore = sevScore
						if logEntry.RiskScore > 70 {
							logEntry.RiskScore = 65
						}
						if logEntry.RiskScore < 40 {
							logEntry.RiskScore = 50
						}
						logEntry.PredictiveRisk = "MEDIUM"
						if sevLabel == "CRITICAL" || sevLabel == "HIGH" {
							logEntry.PredictiveRisk = "HIGH"
						}
						logEntry.PredictedCategory = "AI_GUARD_BOT_REDACT"
						logEntry.RuleTriggered = res.rule.Name
						ruleWarning = strings.TrimSpace(res.rule.WarningMessage)
						evalError = ""
						securityVerdict = "warning"
						_ = h.manager.UpdateLogRuleViolation(ctx, logEntry.ID, logEntry.Action, logEntry.Status, logEntry.RuleTriggered, logEntry.RiskScore, logEntry.PredictiveRisk, logEntry.PredictedCategory)
					} else if ruleAction == "WARN" {
						logEntry.Action = "Warned"
						logEntry.Status = fmt.Sprintf("Warned (%s)", res.rule.Name)
						logEntry.RiskScore = sevScore
						if logEntry.RiskScore > 50 {
							logEntry.RiskScore = 40
						}
						if logEntry.RiskScore < 20 {
							logEntry.RiskScore = 25
						}
						logEntry.PredictiveRisk = "LOW"
						if sevLabel == "CRITICAL" || sevLabel == "HIGH" {
							logEntry.PredictiveRisk = "MEDIUM"
						}
						logEntry.PredictedCategory = "AI_GUARD_BOT_WARNING"
						logEntry.RuleTriggered = res.rule.Name
						ruleWarning = strings.TrimSpace(res.rule.WarningMessage)
						evalError = ""
						securityVerdict = "warning"
						_ = h.manager.UpdateLogRuleViolation(ctx, logEntry.ID, logEntry.Action, logEntry.Status, logEntry.RuleTriggered, logEntry.RiskScore, logEntry.PredictiveRisk, logEntry.PredictedCategory)
					}
				}
			}
			if allowed && aiBotCheckedOK && securityVerdict == "clear" && logEntry.Action == "Allowed" {
				logEntry.Status = "Allowed (AI Guard Bot: security OK)"
				logEntry.PredictedCategory = "AI_GUARD_BOT_CLEAR"
				logEntry.RuleTriggered = ""
				_ = h.manager.UpdateLogRuleViolation(ctx, logEntry.ID, logEntry.Action, logEntry.Status, logEntry.RuleTriggered, logEntry.RiskScore, logEntry.PredictiveRisk, logEntry.PredictedCategory)
			} else if allowed && securityVerdict == "eval_failed" && logEntry.PredictedCategory != "AI_GUARD_BOT_EVAL_ERROR" {
				logEntry.Status = fmt.Sprintf("Allowed (%s — AI Guard Bot eval failed)", logEntry.RuleTriggered)
				if strings.TrimSpace(logEntry.RuleTriggered) == "" {
					logEntry.Status = "Allowed (AI Guard Bot eval failed)"
				}
				logEntry.PredictedCategory = "AI_GUARD_BOT_EVAL_ERROR"
				_ = h.manager.UpdateLogRuleViolation(ctx, logEntry.ID, logEntry.Action, logEntry.Status, logEntry.RuleTriggered, logEntry.RiskScore, logEntry.PredictiveRisk, logEntry.PredictedCategory)
			}
		} // end non-opaque AI bot evaluation
	}

	// What the browser AI receives: REDACT → [RAKSHA REDACTED]; WARN → [RAKSHA WARNING]. Logs keep original only.
	forwardPrompt := payload.Prompt
	if logEntry.Action == "Redacted" {
		forwardPrompt = logstore.FormatWarnedForwardPrompt(payload.Prompt, ruleWarning)
	} else if logEntry.Action == "Warned" {
		forwardPrompt = logstore.FormatWarningForwardPrompt(payload.Prompt, ruleWarning)
	}

	replyText := ""
	replyProvider := ""
	replyModel := ""

	domainHint := ""
	if d, ok := payload.Metadata["domain"].(string); ok {
		domainHint = d
	}
	target, _ := h.manager.GetTargetByDomain(ctx, domainHint)
	replyMode := "violations"
	if target != nil {
		replyMode = strings.ToLower(strings.TrimSpace(target.ReplyBotMode))
		if replyMode != "all" {
			replyMode = "violations"
		}
	}
	botReady := target != nil &&
		target.ReplyBotEnabled &&
		strings.TrimSpace(target.ReplyBotProvider) != "" &&
		strings.TrimSpace(target.ReplyBotModel) != ""

	// "all" mode: Answer every prompt with Reply Bot (do not forward to the site AI).
	if allowed && botReady && replyMode == "all" {
		allowed = false
		logEntry.Action = "Bot Answered"
		logEntry.Status = "REPLIED"
	}

	if !allowed {
		if isViolationBlock {
			// Only the admin-authored rule warning — never a built-in template or Reply Bot rewrite.
			replyText = logstore.SecurityReplyForRule(logEntry.RuleTriggered, ruleWarning)
		} else if botReady {
			replyProvider = strings.TrimSpace(target.ReplyBotProvider)
			replyModel = strings.TrimSpace(target.ReplyBotModel)
			if generated, genErr := h.generateReplyBotText(ctx, replyProvider, replyModel, payload.Platform, logEntry.RuleTriggered, payload.Prompt, "answer"); genErr == nil && strings.TrimSpace(generated) != "" {
				replyText = strings.TrimSpace(generated)
			}
		}
		if strings.TrimSpace(replyText) == "" && !isViolationBlock {
			replyText = "Raksha Reply Bot is enabled for this site, but no model response was returned. Please try again or check provider/model settings."
		}
		_ = h.manager.UpdateLogReplyBot(ctx, logEntry.ID, replyProvider, replyModel, replyText)
		logEntry.ReplyBotProvider = replyProvider
		logEntry.ReplyBotModel = replyModel
		logEntry.ReplyBotText = replyText
		if logEntry.Action == "Bot Answered" {
			_ = h.manager.UpdateLogActionStatus(ctx, logEntry.ID, logEntry.Action, logEntry.Status)
		}
	}

	SendJSON(ctx, map[string]any{
		"status":             "success",
		"allowed":            allowed,
		"action":             logEntry.Action,
		"rule_triggered":     logEntry.RuleTriggered,
		"warning_message":    ruleWarning,
		"redacted_prompt":    forwardPrompt, // proxy injects this into the chat request body
		"forward_prompt":     forwardPrompt,
		"risk_score":         logEntry.RiskScore,
		"predictive_risk":    logEntry.PredictiveRisk,
		"predicted_category": logEntry.PredictedCategory,
		"security_verdict":   securityVerdict,
		"security_message":   securityVerdictMessage(securityVerdict, logEntry.RuleTriggered),
		"reply_text":         replyText,
		"reply_bot_provider": replyProvider,
		"reply_bot_model":    replyModel,
		"eval_error":         evalError,
		"log":                logEntry,
	})
}

func getMetadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	v, ok := metadata[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(v)
}

func parseUploadImagesMetadata(metadata map[string]any) []string {
	if metadata == nil {
		return nil
	}
	raw, ok := metadata["upload_images"]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if t := strings.TrimSpace(s); t != "" {
				out = append(out, t)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		return nil
	}
}

func rulePatternMatches(rule logstore.BrowserGuardRule, text string) bool {
	pattern := strings.TrimSpace(rule.Pattern)
	text = strings.TrimSpace(text)
	if pattern == "" || text == "" {
		return false
	}
	re, err := logstore.CompileGuardRegex(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(text)
}

// aiBotViolationLikelyFalsePositive only drops non-prompt wire/IDE junk.
// Never drop code or valid file content — code characters like {}, ;, operators must not be treated as binary noise.
func aiBotViolationLikelyFalsePositive(_ logstore.BrowserGuardRule, content string) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return true
	}
	low := strings.ToLower(content)
	if strings.Contains(low, "cursor.exe") && strings.Contains(content, "@") {
		return true
	}
	return false
}

// evaluateGuardOnly runs regex + AI Guard Bot rules without persisting a log row (file-scan pre-check).
// Returns evalError/securityVerdict so the proxy never stamps a false "security OK".
func (h *BrowserAIHandler) evaluateGuardOnly(ctx *fasthttp.RequestCtx, prompt string, uploadImages []string) (allowed bool, action, ruleTriggered, ruleWarning, evalError, securityVerdict string) {
	allowed = true
	action = "Allowed"
	securityVerdict = "not_evaluated"
	prompt = strings.TrimSpace(prompt)
	rules, _ := h.getRulesCached(ctx)

	// BLOCK regex before REDACT so duplicate patterns take the stricter action.
	sort.SliceStable(rules, func(i, j int) bool {
		ai := logstore.NormalizeGuardRuleAction(rules[i].Action) == "BLOCK"
		aj := logstore.NormalizeGuardRuleAction(rules[j].Action) == "BLOCK"
		if ai != aj {
			return ai
		}
		return false
	})

	for _, rule := range rules {
		if !rule.Active || strings.ToLower(rule.RuleType) == "ai_bot" || rule.Pattern == "" {
			continue
		}
		re, err := logstore.CompileGuardRegex(rule.Pattern)
		if err != nil || !re.MatchString(prompt) {
			continue
		}
		ruleTriggered = rule.Name
		ruleWarning = strings.TrimSpace(rule.WarningMessage)
		ruleAction := logstore.NormalizeGuardRuleAction(rule.Action)
		if ruleAction == "BLOCK" {
			return false, "Blocked", ruleTriggered, ruleWarning, "", "violation"
		}
		if ruleAction == "REDACT" {
			allowed = true
			action = "Redacted"
			securityVerdict = "warning"
		}
		if ruleAction == "WARN" {
			allowed = true
			action = "Warned"
			securityVerdict = "warning"
		}
	}
	botCheckedOK := false
	readyBots, misconfigBlock := collectAIBotRulesForEval(rules, uploadImages)
	if len(misconfigBlock) > 0 {
		rule := misconfigBlock[0]
		warn := strings.TrimSpace(rule.WarningMessage)
		if warn == "" {
			warn = "AI Guard Bot rule is incomplete (evaluation prompt required)."
		}
		return false, "Blocked", rule.Name, warn, "ai guard bot misconfigured: evaluation prompt is required", "misconfigured"
	}
	for _, res := range h.evalAIBotRulesParallel(prompt, uploadImages, readyBots) {
		if res.skipped {
			continue
		}
		if res.evalErr != "" {
			evalError = res.evalErr
			securityVerdict = "eval_failed"
			if ruleTriggered == "" {
				ruleTriggered = res.rule.Name
			}
			continue
		}
		if !res.violated {
			botCheckedOK = true
			if securityVerdict == "not_evaluated" || securityVerdict == "clear" || securityVerdict == "eval_failed" {
				securityVerdict = "clear"
				evalError = ""
			}
			continue
		}
		ruleTriggered = res.rule.Name
		ruleWarning = strings.TrimSpace(res.rule.WarningMessage)
		ruleAction := logstore.NormalizeGuardRuleAction(res.rule.Action)
		evalError = ""
		if ruleAction == "BLOCK" {
			return false, "Blocked", ruleTriggered, ruleWarning, "", "violation"
		}
		if ruleAction == "REDACT" {
			allowed = true
			action = "Redacted"
			securityVerdict = "warning"
			botCheckedOK = true
		}
		if ruleAction == "WARN" {
			allowed = true
			action = "Warned"
			securityVerdict = "warning"
			botCheckedOK = true
		}
	}
	if botCheckedOK && securityVerdict == "eval_failed" {
		securityVerdict = "clear"
		evalError = ""
	}
	return allowed, action, ruleTriggered, ruleWarning, evalError, securityVerdict
}

func metadataBool(metadata map[string]any, key string) bool {
	if metadata == nil {
		return false
	}
	v, ok := metadata[key]
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "1" || s == "true" || s == "yes"
	case float64:
		return t != 0
	case int:
		return t != 0
	default:
		return false
	}
}

func securityVerdictCategory(verdict string) string {
	switch verdict {
	case "clear":
		return "AI_GUARD_BOT_CLEAR"
	case "violation":
		return "AI_GUARD_BOT_VIOLATION"
	case "warning":
		return "AI_GUARD_BOT_REDACT"
	case "eval_failed":
		return "AI_GUARD_BOT_EVAL_ERROR"
	case "misconfigured":
		return "AI_GUARD_BOT_MISCONFIGURED"
	default:
		return ""
	}
}

func guardActionRank(action string) int {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "blocked", "block":
		return 3
	case "redacted", "redact", "warned", "warn":
		return 2
	default:
		return 1
	}
}

// applyScanGuardFromMetadata uses the proxy's guard scan result (regex + bot already evaluated on Send).
func (h *BrowserAIHandler) applyScanGuardFromMetadata(ctx *fasthttp.RequestCtx, logEntry *logstore.BrowserAILog, metadata map[string]any, ruleWarning *string) bool {
	if logEntry == nil || metadata == nil {
		return false
	}
	decided := metadataBool(metadata, "scan_guard_decided")
	if !decided {
		return false
	}
	// Proxy marked "evaluated" but bot actually failed — do not stamp security OK; re-run below.
	if evalErr := getMetadataString(metadata, "scan_guard_eval_error"); evalErr != "" {
		return false
	}
	action := strings.TrimSpace(getMetadataString(metadata, "scan_guard_action"))
	if action == "" {
		return false
	}
	// Keep stricter InterceptPrompt regex result — never downgrade Blocked/Redacted to Allowed.
	if guardActionRank(logEntry.Action) > guardActionRank(action) {
		return true
	}
	if logEntry.Action == "Blocked" {
		return true
	}
	ruleName := getMetadataString(metadata, "scan_rule_triggered")
	warn := getMetadataString(metadata, "scan_warning_message")

	switch strings.ToLower(action) {
	case "blocked", "block":
		logEntry.Action = "Blocked"
		if ruleName != "" {
			logEntry.Status = fmt.Sprintf("Blocked (%s)", ruleName)
		} else {
			logEntry.Status = "Blocked (Guard Rule)"
		}
		logEntry.RuleTriggered = ruleName
		logEntry.RiskScore = 90
		logEntry.PredictiveRisk = "HIGH"
		logEntry.PredictedCategory = "AI_GUARD_BOT_VIOLATION"
	case "redacted", "redact", "warned", "warn":
		logEntry.Action = "Redacted"
		if ruleName != "" {
			logEntry.Status = fmt.Sprintf("Redacted (%s)", ruleName)
		} else {
			logEntry.Status = "Redacted (Guard Rule)"
		}
		logEntry.RuleTriggered = ruleName
		logEntry.RiskScore = 65
		logEntry.PredictiveRisk = "MEDIUM"
		logEntry.PredictedCategory = "AI_GUARD_BOT_REDACT"
	default:
		logEntry.Action = "Allowed"
		logEntry.Status = "Allowed (AI Guard Bot: security OK)"
		logEntry.RuleTriggered = ""
		logEntry.PredictedCategory = "AI_GUARD_BOT_CLEAR"
	}
	if warn != "" && ruleWarning != nil {
		*ruleWarning = warn
	}
	_ = h.manager.UpdateLogRuleViolation(ctx, logEntry.ID, logEntry.Action, logEntry.Status, logEntry.RuleTriggered, logEntry.RiskScore, logEntry.PredictiveRisk, logEntry.PredictedCategory)
	return true
}

// runAIBotOnLogEntry evaluates active AI Guard Bot rules against file/voice extracted content.
func (h *BrowserAIHandler) runAIBotOnLogEntry(ctx *fasthttp.RequestCtx, logEntry *logstore.BrowserAILog, content string, uploadImages []string, ruleWarning *string) {
	if logEntry == nil || logEntry.Action == "Blocked" {
		return
	}
	content = strings.TrimSpace(content)
	if content == "" && len(uploadImages) == 0 {
		return
	}
	rules, _ := h.getRulesCached(ctx)
	anyClear := false
	readyBots, _ := collectAIBotRulesForEval(rules, uploadImages)
	for _, res := range h.evalAIBotRulesParallel(content, uploadImages, readyBots) {
		if res.skipped {
			continue
		}
		rule := res.rule
		if res.evalErr != "" {
			if logEntry.Action == "Allowed" {
				logEntry.Status = fmt.Sprintf("Allowed (%s — AI Guard Bot eval failed)", rule.Name)
				logEntry.PredictedCategory = "AI_GUARD_BOT_EVAL_ERROR"
				logEntry.RuleTriggered = rule.Name
				_ = h.manager.UpdateLogRuleViolation(ctx, logEntry.ID, logEntry.Action, logEntry.Status, logEntry.RuleTriggered, logEntry.RiskScore, logEntry.PredictiveRisk, logEntry.PredictedCategory)
			}
			continue
		}
		if !res.violated {
			if logEntry.Action == "Allowed" {
				anyClear = true
			}
			continue
		}
		ruleAction := logstore.NormalizeGuardRuleAction(rule.Action)
		sevScore, sevLabel := logstore.GuardSeverityScore(rule.Severity)
		if ruleAction == "BLOCK" {
			logEntry.Action = "Blocked"
			logEntry.Status = fmt.Sprintf("Blocked (%s)", rule.Name)
			logEntry.RiskScore = sevScore
			if logEntry.RiskScore < 80 {
				logEntry.RiskScore = 90
			}
			logEntry.PredictiveRisk = sevLabel
			logEntry.PredictedCategory = "AI_GUARD_BOT_VIOLATION"
			logEntry.RuleTriggered = rule.Name
			if ruleWarning != nil {
				*ruleWarning = strings.TrimSpace(rule.WarningMessage)
			}
			_ = h.manager.UpdateLogRuleViolation(ctx, logEntry.ID, logEntry.Action, logEntry.Status, logEntry.RuleTriggered, logEntry.RiskScore, logEntry.PredictiveRisk, logEntry.PredictedCategory)
			return
		}
		if ruleAction == "REDACT" {
			logEntry.Action = "Redacted"
			logEntry.Status = fmt.Sprintf("Redacted (%s)", rule.Name)
			logEntry.RiskScore = sevScore
			logEntry.PredictedCategory = "AI_GUARD_BOT_REDACT"
			logEntry.RuleTriggered = rule.Name
			if ruleWarning != nil {
				*ruleWarning = strings.TrimSpace(rule.WarningMessage)
			}
			_ = h.manager.UpdateLogRuleViolation(ctx, logEntry.ID, logEntry.Action, logEntry.Status, logEntry.RuleTriggered, logEntry.RiskScore, logEntry.PredictiveRisk, logEntry.PredictedCategory)
		}
		if ruleAction == "WARN" {
			logEntry.Action = "Warned"
			logEntry.Status = fmt.Sprintf("Warned (%s)", rule.Name)
			logEntry.RiskScore = sevScore
			logEntry.PredictedCategory = "AI_GUARD_BOT_WARNING"
			logEntry.RuleTriggered = rule.Name
			if ruleWarning != nil {
				*ruleWarning = strings.TrimSpace(rule.WarningMessage)
			}
			_ = h.manager.UpdateLogRuleViolation(ctx, logEntry.ID, logEntry.Action, logEntry.Status, logEntry.RuleTriggered, logEntry.RiskScore, logEntry.PredictiveRisk, logEntry.PredictedCategory)
		}
	}
	if anyClear && logEntry.Action == "Allowed" {
		logEntry.Status = "Allowed (AI Guard Bot: security OK)"
		logEntry.PredictedCategory = "AI_GUARD_BOT_CLEAR"
		logEntry.RuleTriggered = ""
		_ = h.manager.UpdateLogRuleViolation(ctx, logEntry.ID, logEntry.Action, logEntry.Status, logEntry.RuleTriggered, logEntry.RiskScore, logEntry.PredictiveRisk, logEntry.PredictedCategory)
	}
}

// interceptFile accepts multipart metadata + optional file bytes from Guard/proxy.
// Filename + extracted text are permanent in Prompt Logs.
// File bytes are stored temporarily (~10 minutes) for View/Download, then deleted.
func (h *BrowserAIHandler) interceptFile(ctx *fasthttp.RequestCtx) {
	if !h.verifyGuardSecurity(ctx) {
		return
	}
	h.ensureDB(ctx)

	form, err := ctx.MultipartForm()
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "expected multipart form")
		return
	}

	getForm := func(key string) string {
		if form.Value == nil {
			return ""
		}
		vals := form.Value[key]
		if len(vals) == 0 {
			return ""
		}
		return strings.TrimSpace(vals[0])
	}

	platform := getForm("platform")
	prompt := getForm("prompt")
	clientIP := getForm("client_ip")
	agentID := getForm("agent_id")
	agentHostname := getForm("agent_hostname")
	agentType := getForm("agent_type")
	metaRaw := getForm("metadata")
	contentTypeHint := getForm("content_type")

	metadata := map[string]any{}
	if metaRaw != "" {
		_ = sonic.Unmarshal([]byte(metaRaw), &metadata)
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	if clientIP == "" {
		clientIP = ctx.RemoteIP().String()
	}
	if platform == "" {
		platform = "Browser AI"
	}
	if agentID != "" {
		metadata["agent_id"] = agentID
	}
	if agentHostname != "" {
		metadata["agent_hostname"] = agentHostname
	}
	if agentType == "" {
		if v, ok := metadata["agent_type"].(string); ok {
			agentType = v
		}
	}
	if agentType != "" {
		metadata["agent_type"] = logstore.NormalizeBrowserAIAgentType(agentType)
	}
	metadata["upload_scan"] = true

	fileName := getForm("file_name")
	if fileName != "" {
		metadata["file_name"] = fileName
	}
	if prompt == "" {
		if fileName != "" {
			prompt = "[FILE UPLOAD] " + fileName
		} else {
			prompt = "[FILE UPLOAD] attachment"
		}
	}

	// Optional file part for temp View (does not affect extract/predict — already done on proxy).
	uploadName, uploadBytes, uploadErr := readMultipartUploadFile(form.File)
	if uploadErr != nil {
		// Keep logging even if temp file store is rejected (size etc.)
		uploadBytes = nil
	}
	if fileName == "" && uploadName != "" {
		fileName = uploadName
		metadata["file_name"] = fileName
	}

	logEntry, ruleWarning, err := h.manager.InterceptPrompt(ctx, platform, prompt, clientIP, metadata)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	extractedText := strings.TrimSpace(getMetadataString(metadata, "extracted_text"))
	uploadImages := parseUploadImagesMetadata(metadata)
	scanApplied := h.applyScanGuardFromMetadata(ctx, logEntry, metadata, &ruleWarning)
	// Never stamp AI Guard Bot "security OK" on filename-only prefixes with no extract/images.
	if !scanApplied && logEntry.Action != "Blocked" && (extractedText != "" || len(uploadImages) > 0) {
		evalContent := buildGuardEvalContent(prompt, extractedText)
		h.runAIBotOnLogEntry(ctx, logEntry, evalContent, uploadImages, &ruleWarning)
	} else if !scanApplied && logEntry.Action == "Allowed" && extractedText == "" && len(uploadImages) == 0 {
		logEntry.Status = "Allowed (no extractable content)"
		logEntry.PredictedCategory = "SAFE"
		logEntry.RuleTriggered = ""
		_ = h.manager.UpdateLogRuleViolation(ctx, logEntry.ID, logEntry.Action, logEntry.Status, logEntry.RuleTriggered, logEntry.RiskScore, logEntry.PredictiveRisk, logEntry.PredictedCategory)
	}

	safeName := sanitizeAttachmentFileName(fileName)
	if safeName != "" {
		logEntry.AttachmentName = safeName
		// Permanent filename on the log (even if temp file store is skipped).
		_ = h.manager.UpdateLogAttachment(ctx, logEntry.ID, safeName, "", contentTypeHint)
	}

	if len(uploadBytes) >= 32 {
		stored, ctype, storeErr := storeBrowserAIAttachment(logEntry.ID, safeName, uploadBytes, contentTypeHint)
		if storeErr == nil && stored != "" {
			exp := time.Now().Add(browserAIAttachmentTTL)
			rel := "attachments/" + stored
			if err := h.manager.UpdateLogAttachmentMeta(ctx, logEntry.ID, safeName, stored, ctype, int64(len(uploadBytes)), rel, &exp); err == nil {
				logEntry.AttachmentStoredName = stored
				logEntry.AttachmentContentType = ctype
				logEntry.AttachmentName = safeName
				logEntry.AttachmentSizeBytes = int64(len(uploadBytes))
				logEntry.AttachmentPath = rel
				logEntry.AttachmentExpiresAt = &exp
			}
		}
	}

	// Opportunistic cleanup of expired temp files
	go purgeExpiredBrowserAIAttachments(h.manager)

	SendJSON(ctx, map[string]any{
		"status":          "success",
		"allowed":         logEntry.Action == "Allowed" || logEntry.Action == "Warned" || logEntry.Action == "Redacted",
		"action":          logEntry.Action,
		"rule_triggered":  logEntry.RuleTriggered,
		"warning_message": ruleWarning,
		"log":             logEntry,
	})
}

func (h *BrowserAIHandler) getAttachment(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	id := ctx.UserValue("id")
	idStr, _ := id.(string)
	idStr = strings.TrimSpace(idStr)
	if idStr == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "missing attachment id")
		return
	}
	logEntry, err := h.manager.GetLogByID(ctx, idStr)
	if err != nil || logEntry == nil {
		SendError(ctx, fasthttp.StatusNotFound, "attachment not found")
		return
	}
	name := strings.TrimSpace(logEntry.AttachmentName)
	if name == "" {
		name = "attachment"
	}
	stored := strings.TrimSpace(logEntry.AttachmentStoredName)
	if stored == "" {
		SendError(ctx, fasthttp.StatusGone, "file expired (available for 10 minutes only); log and filename remain")
		return
	}
	path, err := resolveBrowserAIAttachmentPath(stored)
	if err != nil {
		_ = h.manager.ClearLogAttachmentFile(ctx, logEntry.ID)
		SendError(ctx, fasthttp.StatusGone, "file expired (available for 10 minutes only); log and filename remain")
		return
	}
	if browserAIAttachmentExpired(path) {
		_ = os.Remove(path)
		_ = h.manager.ClearLogAttachmentFile(ctx, logEntry.ID)
		SendError(ctx, fasthttp.StatusGone, "file expired (available for 10 minutes only); log and filename remain")
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		_ = h.manager.ClearLogAttachmentFile(ctx, logEntry.ID)
		SendError(ctx, fasthttp.StatusGone, "file expired (available for 10 minutes only); log and filename remain")
		return
	}
	ctype := sniffAttachmentContentType(data, name, logEntry.AttachmentContentType)
	if len(data) >= 5 && string(data[:5]) == "%PDF-" {
		ctype = "application/pdf"
	} else if len(data) > 0 && (data[0] == '{' || data[0] == '[') {
		ctype = "application/json; charset=utf-8"
	}
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	download := string(ctx.QueryArgs().Peek("download")) == "1" ||
		strings.EqualFold(string(ctx.QueryArgs().Peek("download")), "true")

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetContentType(ctype)
	ctx.Response.Header.Set("Cache-Control", "private, max-age=60")
	if download {
		ctx.Response.Header.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, strings.ReplaceAll(name, `"`, "")))
	} else {
		ctx.Response.Header.Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, strings.ReplaceAll(name, `"`, "")))
	}
	ctx.SetBody(data)
}
