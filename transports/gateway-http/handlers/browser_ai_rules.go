package handlers

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gateway/gateway/framework/logstore"
	"github.com/valyala/fasthttp"
)

func (h *BrowserAIHandler) getRules(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	rules, err := h.manager.GetRules(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	forAgent := strings.EqualFold(string(ctx.QueryArgs().Peek("for")), "agent")
	if forAgent {
		out := make([]map[string]any, 0, len(rules))
		hasAIBot := false
		for _, r := range rules {
			if !r.Active {
				continue
			}
			rt := strings.ToLower(strings.TrimSpace(r.RuleType))
			if rt == "ai_bot" {
				hasAIBot = true
			}
			// Lite payload: pattern + action for local regex. Skip huge bot_prompt blobs.
			row := map[string]any{
				"id":              r.ID,
				"name":            r.Name,
				"rule_type":       r.RuleType,
				"pattern":         r.Pattern,
				"action":          r.Action,
				"severity":        r.Severity,
				"warning_message": r.WarningMessage,
				"active":          true,
			}
			if rt == "ai_bot" {
				row["bot_provider"] = r.BotProvider
				row["bot_model"] = r.BotModel
				// Intentionally omit bot_prompt / reference image — agent only needs has_ai_bot.
			}
			out = append(out, row)
		}
		SendJSON(ctx, map[string]any{"rules": out, "has_ai_bot": hasAIBot})
		return
	}
	SendJSON(ctx, map[string]any{"rules": rules})
}

func (h *BrowserAIHandler) getOllamaModels(ctx *fasthttp.RequestCtx) {
	models, baseURL, err := listOllamaInstalledModels(10 * time.Second)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadGateway, err.Error())
		return
	}
	SendJSON(ctx, map[string]any{
		"models":   models,
		"base_url": baseURL,
	})
}

func (h *BrowserAIHandler) createRule(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	// Bulk import payload {"rules":[...],"overwrite":bool} on POST /rules — same handler as /rules/import.
	// Lets import work even when a reverse proxy or older route table only exposes POST /rules.
	var peek map[string]any
	if err := sonic.Unmarshal(ctx.PostBody(), &peek); err == nil {
		if _, ok := peek["rules"]; ok {
			h.importRules(ctx)
			return
		}
	}
	var rule logstore.BrowserGuardRule
	if err := sonic.Unmarshal(ctx.PostBody(), &rule); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if strings.TrimSpace(rule.Name) == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Rule name is required")
		return
	}
	if strings.ToLower(rule.RuleType) == "ai_bot" {
		applyAIBotDefaults(&rule)
		if err := validateAIBotRuleFields(&rule); err != nil {
			SendError(ctx, fasthttp.StatusBadRequest, err.Error())
			return
		}
	} else {
		rule.RuleType = "regex"
		if strings.TrimSpace(rule.Pattern) == "" {
			SendError(ctx, fasthttp.StatusBadRequest, "Regex pattern is required for Regex rule")
			return
		}
		if err := validateGuardRegexPattern(rule.Pattern); err != nil {
			SendError(ctx, fasthttp.StatusBadRequest, err.Error())
			return
		}
	}
	if err := h.manager.CreateRule(ctx, &rule); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	h.invalidateRulesCache()
	SendJSON(ctx, map[string]any{"status": "success", "rule": rule})
}

func isReservedBrowserAIRulePathID(id string) bool {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "import", "test-bot", "generate-regex":
		return true
	default:
		return false
	}
}

func (h *BrowserAIHandler) updateRule(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	id, ok := ctx.UserValue("id").(string)
	if !ok || id == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Missing rule ID")
		return
	}
	if isReservedBrowserAIRulePathID(id) {
		SendError(ctx, fasthttp.StatusMethodNotAllowed, "Use POST /api/browser-ai/rules/import for bulk import")
		return
	}
	var updates map[string]any
	if err := sonic.Unmarshal(ctx.PostBody(), &updates); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid JSON payload")
		return
	}

	// Validate AI Guard Bot fields for the resulting rule (including partial updates).
	ruleType := ""
	if v, ok := updates["rule_type"].(string); ok {
		ruleType = strings.ToLower(strings.TrimSpace(v))
	}
	_, hasBotPrompt := updates["bot_prompt"]
	_, hasBotProvider := updates["bot_provider"]
	_, hasBotModel := updates["bot_model"]
	_, hasBotReferenceImage := updates["bot_reference_image"]
	_, hasBotReferenceImageType := updates["bot_reference_image_type"]
	needsAIBotCheck := ruleType == "ai_bot" || hasBotPrompt || hasBotProvider || hasBotModel || hasBotReferenceImage || hasBotReferenceImageType
	if needsAIBotCheck {
		if existingRules, getErr := h.manager.GetRules(ctx); getErr == nil {
			for i := range existingRules {
				if existingRules[i].ID != id {
					continue
				}
				existing := existingRules[i]
				if ruleType == "" {
					ruleType = strings.ToLower(strings.TrimSpace(existing.RuleType))
				}
				if !hasBotPrompt {
					updates["bot_prompt"] = existing.BotPrompt
				}
				if !hasBotProvider {
					updates["bot_provider"] = existing.BotProvider
				}
				if !hasBotModel {
					updates["bot_model"] = existing.BotModel
				}
				if !hasBotReferenceImage {
					updates["bot_reference_image"] = existing.BotReferenceImage
				}
				if !hasBotReferenceImageType {
					updates["bot_reference_image_type"] = existing.BotReferenceImageType
				}
				break
			}
		}
	}
	if ruleType == "ai_bot" {
		tmp := logstore.BrowserGuardRule{
			RuleType:              "ai_bot",
			BotProvider:           stringFromUpdate(updates, "bot_provider"),
			BotModel:              stringFromUpdate(updates, "bot_model"),
			BotPrompt:             stringFromUpdate(updates, "bot_prompt"),
			BotReferenceImage:     stringFromUpdate(updates, "bot_reference_image"),
			BotReferenceImageType: stringFromUpdate(updates, "bot_reference_image_type"),
		}
		if err := validateAIBotRuleFields(&tmp); err != nil {
			SendError(ctx, fasthttp.StatusBadRequest, err.Error())
			return
		}
		updates["bot_provider"] = tmp.BotProvider
		updates["bot_model"] = tmp.BotModel
		updates["bot_prompt"] = tmp.BotPrompt
		updates["bot_reference_image"] = tmp.BotReferenceImage
		updates["bot_reference_image_type"] = tmp.BotReferenceImageType
	}

	if raw, ok := updates["pattern"]; ok && ruleType != "ai_bot" {
		pattern, _ := raw.(string)
		if strings.TrimSpace(pattern) == "" {
			SendError(ctx, fasthttp.StatusBadRequest, "Regex pattern is required for Regex rule")
			return
		}
		if err := validateGuardRegexPattern(pattern); err != nil {
			SendError(ctx, fasthttp.StatusBadRequest, err.Error())
			return
		}
	}

	if err := h.manager.UpdateRule(ctx, id, updates); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	h.invalidateRulesCache()
	SendJSON(ctx, map[string]any{"status": "success"})
}

// validateGuardRegexPattern rejects patterns the gateway (Go RE2) cannot compile, and
// RE2-only constructs the browser proxy's Python engine reads differently, so a saved
// rule behaves the same at the gateway, in the proxy and in the extension.
func validateGuardRegexPattern(pattern string) error {
	if _, err := logstore.CompileGuardRegex(pattern); err != nil {
		return fmt.Errorf("invalid regex (RE2 syntax — lookahead/lookbehind and backreferences are not supported): %v", err)
	}
	for _, construct := range []string{`\p{`, `\P{`, `[[:`} {
		if strings.Contains(pattern, construct) {
			return fmt.Errorf("pattern uses %q, which the browser proxy cannot evaluate; use explicit character classes such as [A-Za-z] or [0-9]", construct)
		}
	}
	return nil
}

func (h *BrowserAIHandler) deleteRule(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	id, ok := ctx.UserValue("id").(string)
	if !ok || id == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Missing rule ID")
		return
	}
	if isReservedBrowserAIRulePathID(id) {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid rule ID")
		return
	}
	if err := h.manager.DeleteRule(ctx, id); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	h.invalidateRulesCache()
	SendJSON(ctx, map[string]any{"status": "success"})
}

type importRulesPayload struct {
	Rules     []importRuleItem `json:"rules"`
	Overwrite bool             `json:"overwrite"`
}

// importRuleItem accepts spreadsheet / API rows. Active defaults to true when omitted.
type importRuleItem struct {
	Name           string `json:"name"`
	RuleType       string `json:"rule_type"`
	Pattern        string `json:"pattern"`
	Severity       string `json:"severity"`
	Action         string `json:"action"`
	WarningMessage string `json:"warning_message"`
	Description    string `json:"description"`
	Active         *bool  `json:"active"`
}

func normalizeImportedRulePattern(raw string) string {
	p := strings.TrimSpace(raw)
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "\\b") || strings.HasPrefix(p, "^") || strings.HasPrefix(p, "(?") {
		return p
	}
	// Real regexes (quantifiers like {3,5}, classes, alternation of tokens) must not be
	// split on , or | into escaped literals. $ ^ . are excluded: they appear in keywords.
	if strings.ContainsAny(p, `\[]{}()*+?`) {
		if _, err := regexp.Compile(p); err == nil {
			return p
		}
	}
	if strings.ContainsAny(p, ",\n\r;|") {
		parts := strings.FieldsFunc(p, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r' || r == ';' || r == '|'
		})
		isWordChar := func(c rune) bool {
			return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
		}
		var clean []string
		var bounded []string
		allWordBounded := true
		for _, part := range parts {
			t := strings.TrimSpace(part)
			if t == "" {
				continue
			}
			runes := []rune(t)
			escaped := regexp.QuoteMeta(t)
			clean = append(clean, escaped)
			hasWordStart := isWordChar(runes[0])
			hasWordEnd := isWordChar(runes[len(runes)-1])
			if !hasWordStart || !hasWordEnd {
				allWordBounded = false
			}
			prefix := ""
			suffix := ""
			if hasWordStart {
				prefix = `\b`
			}
			if hasWordEnd {
				suffix = `\b`
			}
			bounded = append(bounded, prefix+escaped+suffix)
		}
		if len(clean) > 1 {
			if allWordBounded {
				return `\b(?:` + strings.Join(clean, "|") + `)\b`
			}
			return `(?:` + strings.Join(bounded, "|") + `)`
		} else if len(clean) == 1 {
			return bounded[0]
		}
	}
	return p
}

func (h *BrowserAIHandler) importRules(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	var payload importRulesPayload
	if err := sonic.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if len(payload.Rules) == 0 {
		SendError(ctx, fasthttp.StatusBadRequest, "No rules provided for import")
		return
	}

	existingRules, err := h.manager.GetRules(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	existingByName := make(map[string]logstore.BrowserGuardRule, len(existingRules))
	existingByPattern := make(map[string]logstore.BrowserGuardRule, len(existingRules))
	for _, r := range existingRules {
		existingByName[strings.ToLower(strings.TrimSpace(r.Name))] = r
		if r.RuleType != "ai_bot" && strings.TrimSpace(r.Pattern) != "" {
			existingByPattern[strings.TrimSpace(r.Pattern)] = r
		}
	}

	imported := 0
	updated := 0
	skipped := 0
	alreadyExists := 0
	duplicates := 0
	failed := 0
	var errorDetails []string
	seenNames := map[string]bool{}
	seenPatterns := map[string]bool{}

	for i := range payload.Rules {
		item := payload.Rules[i]
		name := strings.TrimSpace(item.Name)
		if name == "" {
			skipped++
			errorDetails = append(errorDetails, fmt.Sprintf("Row %d: Missing rule name", i+1))
			continue
		}

		action := logstore.NormalizeGuardRuleAction(item.Action)
		if action == "" {
			action = "BLOCK"
		}

		sev := logstore.NormalizeGuardRuleSeverity(item.Severity)

		rt := strings.ToLower(strings.TrimSpace(item.RuleType))
		if rt != "ai_bot" {
			rt = "regex"
		}

		pattern := normalizeImportedRulePattern(item.Pattern)
		warning := strings.TrimSpace(item.WarningMessage)
		description := strings.TrimSpace(item.Description)
		if warning == "" && description != "" {
			warning = description
		}

		active := true
		if item.Active != nil {
			active = *item.Active
		}

		if rt == "ai_bot" {
			skipped++
			errorDetails = append(errorDetails, fmt.Sprintf("Row %d (%s): AI Guard Bot rules cannot be imported from Excel — create them in the UI", i+1, name))
			continue
		}
		if pattern == "" {
			skipped++
			errorDetails = append(errorDetails, fmt.Sprintf("Row %d (%s): Missing regex pattern", i+1, name))
			continue
		}
		if compileErr := validateGuardRegexPattern(pattern); compileErr != nil {
			skipped++
			errorDetails = append(errorDetails, fmt.Sprintf("Row %d (%s): %v", i+1, name, compileErr))
			continue
		}

		nameKey := strings.ToLower(name)
		if seenNames[nameKey] || seenPatterns[pattern] {
			skipped++
			duplicates++
			errorDetails = append(errorDetails, fmt.Sprintf("Row %d (%s): Duplicate rule in the file (same name or pattern) — skipped", i+1, name))
			continue
		}
		seenNames[nameKey] = true
		seenPatterns[pattern] = true

		if same, exists := existingByPattern[pattern]; exists && !strings.EqualFold(strings.TrimSpace(same.Name), name) {
			skipped++
			alreadyExists++
			errorDetails = append(errorDetails, fmt.Sprintf("Row %d (%s): Same pattern already exists as rule '%s' — skipped", i+1, name, same.Name))
			continue
		}

		if existing, exists := existingByName[nameKey]; exists {
			if payload.Overwrite {
				updateFields := map[string]any{
					"name":            name,
					"rule_type":       "regex",
					"pattern":         pattern,
					"action":          action,
					"severity":        sev,
					"warning_message": warning,
					"description":     description,
					"active":          active,
				}
				if err := h.manager.UpdateRule(ctx, existing.ID, updateFields); err != nil {
					failed++
					errorDetails = append(errorDetails, fmt.Sprintf("Failed to update '%s': %v", name, err))
					continue
				}
				delete(existingByPattern, strings.TrimSpace(existing.Pattern))
				existing.Pattern = pattern
				existingByPattern[pattern] = existing
				updated++
			} else {
				skipped++
				alreadyExists++
			}
			continue
		}

		rule := logstore.BrowserGuardRule{
			Name:           name,
			RuleType:       "regex",
			Pattern:        pattern,
			Action:         action,
			Severity:       sev,
			WarningMessage: warning,
			Description:    description,
			Active:         active,
		}
		if err := h.manager.CreateRule(ctx, &rule); err != nil {
			failed++
			errorDetails = append(errorDetails, fmt.Sprintf("Failed to create '%s': %v", name, err))
			continue
		}
		existingByName[nameKey] = rule
		existingByPattern[pattern] = rule
		imported++
	}

	h.invalidateRulesCache()
	SendJSON(ctx, map[string]any{
		"status":             "success",
		"imported":           imported,
		"updated":            updated,
		"skipped":            skipped,
		"already_exists":     alreadyExists,
		"duplicates_in_file": duplicates,
		"failed":             failed,
		"errors":             errorDetails,
		"total":              len(payload.Rules),
	})
}
