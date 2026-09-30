package handlers

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/framework/logstore"
	"github.com/valyala/fasthttp"
)

func guardBotEvalBudget() time.Duration {
	// Per-bot LLM budget. Agent wait is ~18–28s; keep bots under that (parallel = max, not sum).
	sec := 12
	if v := strings.TrimSpace(os.Getenv("UNIFAI_GUARD_BOT_TIMEOUT_SEC")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 3 && n <= 60 {
			sec = n
		}
	}
	return time.Duration(sec) * time.Second
}

const maxAIBotsPerPrompt = 8
const maxAIBotParallel = 4

type aiBotRuleResult struct {
	rule          logstore.BrowserGuardRule
	violated      bool
	evalErr       string
	misconfigured bool
	skipped       bool
}

func collectAIBotRulesForEval(rules []logstore.BrowserGuardRule, uploadImages []string) (ready []logstore.BrowserGuardRule, misconfigBlock []logstore.BrowserGuardRule) {
	for _, rule := range rules {
		if !rule.Active || strings.ToLower(rule.RuleType) != "ai_bot" {
			continue
		}
		applyAIBotDefaults(&rule)
		if strings.TrimSpace(rule.BotPrompt) == "" && strings.TrimSpace(rule.BotReferenceImage) == "" {
			if logstore.NormalizeGuardRuleAction(rule.Action) == "BLOCK" {
				misconfigBlock = append(misconfigBlock, rule)
			}
			continue
		}
		if skipAIBotRuleWithoutImages(rule.BotModel, rule.BotReferenceImage, uploadImages) {
			continue
		}
		ready = append(ready, rule)
	}
	// BLOCK policies first so a hard stop wins under the shared time budget.
	sort.SliceStable(ready, func(i, j int) bool {
		ai := logstore.NormalizeGuardRuleAction(ready[i].Action) == "BLOCK"
		aj := logstore.NormalizeGuardRuleAction(ready[j].Action) == "BLOCK"
		if ai == aj {
			return false
		}
		return ai
	})
	if len(ready) > maxAIBotsPerPrompt {
		ready = ready[:maxAIBotsPerPrompt]
	}
	return ready, misconfigBlock
}

// evalAIBotRulesParallel runs AI Guard Bots concurrently (max latency ≈ slowest bot, not sum).
func (h *BrowserAIHandler) evalAIBotRulesParallel(evalContent string, uploadImages []string, bots []logstore.BrowserGuardRule) []aiBotRuleResult {
	results := make([]aiBotRuleResult, len(bots))
	if len(bots) == 0 {
		return results
	}
	budget := guardBotEvalBudget()
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()

	sem := make(chan struct{}, maxAIBotParallel)
	var wg sync.WaitGroup
	var foundBlock atomic.Bool

	for i := range bots {
		wg.Add(1)
		go func(i int, rule logstore.BrowserGuardRule) {
			defer wg.Done()
			if foundBlock.Load() {
				results[i] = aiBotRuleResult{rule: rule, skipped: true}
				return
			}
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i] = aiBotRuleResult{rule: rule, evalErr: "guard bot eval budget exceeded", skipped: true}
				return
			}
			defer func() { <-sem }()

			if foundBlock.Load() || ctx.Err() != nil {
				results[i] = aiBotRuleResult{rule: rule, skipped: true}
				return
			}

			violated := false
			var evalErr string
			if rulePatternMatches(rule, evalContent) {
				violated = true
			} else {
				violated, evalErr = h.evaluateAIBotRule(rule, evalContent, uploadImages)
			}
			if violated && aiBotViolationLikelyFalsePositive(rule, evalContent) {
				violated = false
			}
			results[i] = aiBotRuleResult{rule: rule, violated: violated, evalErr: evalErr}
			if violated && logstore.NormalizeGuardRuleAction(rule.Action) == "BLOCK" {
				foundBlock.Store(true)
				cancel()
			}
		}(i, bots[i])
	}
	wg.Wait()
	return results
}
type aiBotEvalResult struct {
	Violation   bool   `json:"violation"`
	Reason      string `json:"reason"`
	Explanation string `json:"explanation"`
}

func evaluatorChoiceText(resp *schemas.UnifAIChatResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return ""
	}
	choice := resp.Choices[0]
	if choice.Message == nil {
		return ""
	}
	return assistantMessageText(choice.Message)
}

// assistantMessageText pulls usable text from content, then reasoning/refusal.
// DeepSeek-reasoner / R1-style models often leave content empty and put the
// JSON decision only in reasoning / reasoning_content.
func assistantMessageText(msg *schemas.ChatMessage) string {
	if msg == nil {
		return ""
	}
	if msg.Content != nil {
		if msg.Content.ContentStr != nil {
			if s := strings.TrimSpace(*msg.Content.ContentStr); s != "" {
				return s
			}
		}
		var b strings.Builder
		for _, block := range msg.Content.ContentBlocks {
			if block.Text != nil {
				b.WriteString(*block.Text)
			}
			if block.Refusal != nil {
				b.WriteString(*block.Refusal)
			}
		}
		if s := strings.TrimSpace(b.String()); s != "" {
			return s
		}
	}
	if msg.ChatAssistantMessage != nil {
		if msg.Refusal != nil {
			if s := strings.TrimSpace(*msg.Refusal); s != "" {
				return s
			}
		}
		if msg.Reasoning != nil {
			if s := strings.TrimSpace(*msg.Reasoning); s != "" {
				return s
			}
		}
		for _, d := range msg.ReasoningDetails {
			if d.Text != nil {
				if s := strings.TrimSpace(*d.Text); s != "" {
					return s
				}
			}
			if d.Summary != nil {
				if s := strings.TrimSpace(*d.Summary); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func evaluatorEmptyReason(resp *schemas.UnifAIChatResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return "empty evaluator response (no choices from model)"
	}
	fr := ""
	if resp.Choices[0].FinishReason != nil {
		fr = strings.TrimSpace(*resp.Choices[0].FinishReason)
	}
	if fr == "length" {
		return "empty evaluator response (finish_reason=length — pick a chat model or raise tokens; reasoner models need more room)"
	}
	if fr != "" {
		return "empty evaluator response (finish_reason=" + fr + " — pick a chat model that returns text content, not a free/code/reasoner-only model)"
	}
	return "empty evaluator response (model returned no text — pick a chat model matching your API key, or Download → llama3.2)"
}

func securityVerdictMessage(verdict, ruleName string) string {
	switch verdict {
	case "clear":
		if ruleName != "" {
			return "Security OK — AI Guard Bot found no policy violation (" + ruleName + ")."
		}
		return "Security OK — AI Guard Bot found no policy violation."
	case "violation":
		if ruleName != "" {
			return "Security NOT met — prompt violates AI Guard Bot policy (" + ruleName + "). Blocked."
		}
		return "Security NOT met — prompt violates AI Guard Bot policy. Blocked."
	case "warning":
		if ruleName != "" {
			return "Security warning — prompt matched AI Guard Bot policy (" + ruleName + "). Allowed with warning."
		}
		return "Security warning — prompt matched AI Guard Bot policy. Allowed with warning."
	case "eval_failed":
		return "Security check failed — AI Guard Bot could not evaluate this prompt."
	case "misconfigured":
		return "Security check failed — AI Guard Bot rule is incomplete (provider/model/prompt)."
	default:
		return ""
	}
}

// testAIGuardBot dry-runs an AI Guard Bot policy against a sample prompt (admin only).
func (h *BrowserAIHandler) testAIGuardBot(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	var payload struct {
		BotProvider  string `json:"bot_provider"`
		BotModel     string `json:"bot_model"`
		BotPrompt    string `json:"bot_prompt"`
		SamplePrompt string `json:"sample_prompt"`
		Action       string `json:"action"`
		Name         string `json:"name"`
	}
	if err := sonic.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid JSON payload")
		return
	}
	if strings.TrimSpace(payload.BotPrompt) == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "bot_prompt is required")
		return
	}
	sample := strings.TrimSpace(payload.SamplePrompt)
	if sample == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "sample_prompt is required")
		return
	}
	rule := logstore.BrowserGuardRule{
		Name:        strings.TrimSpace(payload.Name),
		RuleType:    "ai_bot",
		BotProvider: strings.TrimSpace(payload.BotProvider),
		BotModel:    strings.TrimSpace(payload.BotModel),
		BotPrompt:   strings.TrimSpace(payload.BotPrompt),
		Action:      logstore.NormalizeGuardRuleAction(payload.Action),
		Active:      true,
	}
	if rule.Name == "" {
		rule.Name = "Test AI Guard Bot"
	}
	applyAIBotDefaults(&rule)
	violated, evalErr, modelRaw := h.evaluateAIBotRuleDetailed(rule, sample, nil)
	verdict := "clear"
	message := securityVerdictMessage("clear", rule.Name)
	wouldBlock := false
	wouldWarn := false
	if evalErr != "" {
		verdict = "eval_failed"
		message = securityVerdictMessage("eval_failed", rule.Name) + " " + evalErr
		message += " (live traffic is allowed — only confirmed violations block.)"
	} else if violated {
		if rule.Action == "REDACT" || rule.Action == "WARN" {
			verdict = "redact"
			wouldWarn = true
			message = securityVerdictMessage("redact", rule.Name)
		} else {
			verdict = "violation"
			wouldBlock = true
			message = securityVerdictMessage("violation", rule.Name)
		}
	}
	SendJSON(ctx, map[string]any{
		"status":            "success",
		"violation":         violated && evalErr == "",
		"security_verdict":  verdict,
		"security_message":  message,
		"security_met":      verdict == "clear",
		"would_block":       wouldBlock,
		"would_warn":        wouldWarn,
		"eval_error":        evalErr,
		"model_raw":         truncateRunes(modelRaw, 400),
		"rule_name":         rule.Name,
		"bot_provider":      rule.BotProvider,
		"bot_model":         rule.BotModel,
	})
}

// generateRegexFromPolicy turns a security-policy prompt into a RE2 regex
// using Download (Ollama) or Outsource (configured Model Providers) models.
func (h *BrowserAIHandler) generateRegexFromPolicy(ctx *fasthttp.RequestCtx) {
	var payload struct {
		BotProvider string `json:"bot_provider"`
		BotModel    string `json:"bot_model"`
		BotPrompt   string `json:"bot_prompt"`
	}
	if err := sonic.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid JSON payload")
		return
	}
	policy := strings.TrimSpace(payload.BotPrompt)
	if policy == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "bot_prompt is required")
		return
	}
	provider := strings.TrimSpace(payload.BotProvider)
	model := strings.TrimSpace(payload.BotModel)
	if provider == "" {
		provider = browserAIGuardBotDefaultProvider
	}
	if model == "" {
		model = browserAIGuardBotDefaultModel
	}
	provider, model = applyGuardBotDefaults(provider, model)
	providerName, modelName := resolveGuardBotModel(provider, model)
	provider = string(providerName)
	model = modelName

	// Vision-only models are poor at regex generation — prefer text chat models.
	if isVisionOnlyGuardModelName(model) {
		SendError(ctx, fasthttp.StatusBadRequest, "pick a text model (e.g. llama3.2) to generate regex — vision models are for images")
		return
	}

	// Never invent regex from hardcoded policy keywords. The selected model (or admin
	// editing Pattern) defines the rule. We only ask for valid RE2 JSON.
	systemPromptPrimary := `You convert a SECURITY_POLICY into ONE Go RE2 regular expression.

Return ONLY this JSON (no markdown):
{"pattern":"<regex>","focus":"<short>","notes":"<short>"}

pattern RULES:
- Must be a real RE2 regex that can match chat text.
- NEVER invent fake patterns like []word[] or empty [].
- NEVER put English sentences in pattern.
- Prefer \\b, \\d, [A-Za-z], |, (), {n,m}.
- No lookbehind/lookahead, no backreferences (RE2).
- Keep pattern under 180 characters.
- Case is ignored by the engine (?i).
- Derive ONLY from SECURITY_POLICY — do not invent formats the policy does not mention.
- If unsure: keyword alternation with word boundaries from policy terms.`

	systemPromptRetry := `Output ONLY valid JSON: {"pattern":"<RE2 regex>","focus":"match","notes":"ok"}
pattern must compile in Go regexp. No markdown. No English sentences as pattern.
Derive the pattern only from SECURITY_POLICY.`

	userMsg := fmt.Sprintf(
		"SECURITY_POLICY:\n%s\n\nJSON only. pattern = valid RE2 derived from this policy.",
		truncateRunes(policy, 8000),
	)
	userMsgRetry := fmt.Sprintf(
		"SECURITY_POLICY:\n%s\n\nReturn JSON with a usable RE2 pattern now.",
		truncateRunes(policy, 4000),
	)

	callModel := func(systemPrompt, user string, preferJSON bool) (string, error) {
		if isOllamaGuardProvider(provider) {
			runOnce := func(jsonMode bool) (string, error) {
				return callOllamaChatAny(model, systemPrompt, user, jsonMode, 90*time.Second)
			}
			raw, err := runOnce(preferJSON)
			if err != nil || strings.TrimSpace(raw) == "" {
				raw2, err2 := runOnce(false)
				if err2 == nil && strings.TrimSpace(raw2) != "" {
					return raw2, nil
				}
				if err != nil && err2 != nil {
					return "", fmt.Errorf("%v; retry: %v", err, err2)
				}
				if err != nil {
					return "", err
				}
				return "", err2
			}
			return raw, nil
		}
		if h.client == nil {
			return "", fmt.Errorf("unifai client not available for outsource model")
		}
		prov := schemas.ModelProvider(provider)
		unifaiReq := buildGuardOutsourceChatRequest(prov, model, systemPrompt, user, false, preferJSON && !isWeakGuardEvalModel(model) && !isReasoningGuardModel(model))
		if unifaiReq.Params != nil {
			maxTokens := 512
			if isReasoningGuardModel(model) {
				maxTokens = 1024
			}
			temp := 0.1
			unifaiReq.Params.Temperature = &temp
			if unifaiReq.Params.ExtraParams == nil {
				unifaiReq.Params.ExtraParams = map[string]interface{}{}
			}
			unifaiReq.Params.ExtraParams["max_tokens"] = maxTokens
			if strings.EqualFold(string(prov), string(schemas.OpenAI)) || strings.EqualFold(string(prov), string(schemas.Azure)) {
				unifaiReq.Params.MaxCompletionTokens = &maxTokens
			}
		}
		deadline := time.Now().Add(75 * time.Second)
		unifaiCtx := schemas.NewUnifAIContext(context.Background(), deadline)
		unifaiCtx.SetValue(schemas.UnifAIContextKeySkipBudgetAndRateLimits, true)
		unifaiCtx.SetValue(schemas.UnifAIContextKeySkipPluginPipeline, true)
		unifaiCtx.SetValue(schemas.UnifAIContextKeyPassthroughExtraParams, true)
		resp, unifaiErr := h.client.ChatCompletionRequest(unifaiCtx, unifaiReq)
		if unifaiErr != nil {
			return "", fmt.Errorf("%s", unifaiErrorMessage(unifaiErr))
		}
		return evaluatorChoiceText(resp), nil
	}

	genSource := "ollama"
	if !isOllamaGuardProvider(provider) {
		genSource = "outsource"
	}

	tryParse := func(raw string) (pattern, focus, notes string, ok bool) {
		raw = stripEvalMarkdown(strings.TrimSpace(raw))
		if raw == "" {
			return "", "", "", false
		}
		pattern, focus, notes = parseGeneratedRegexPayload(raw)
		pattern = sanitizeGeneratedRegex(pattern)
		if !isUsableGeneratedRegex(pattern) {
			return "", "", "", false
		}
		return pattern, focus, notes, true
	}

	var (
		raw    string
		genErr error
		pat    string
		focus  string
		notes  string
		ok     bool
	)

	// Attempt 1: primary prompt + JSON mode
	raw, genErr = callModel(systemPromptPrimary, userMsg, true)
	if genErr == nil {
		pat, focus, notes, ok = tryParse(raw)
	}
	// Attempt 2: same prompt without JSON mode (many free/outsource models reject json_object)
	if !ok {
		raw2, err2 := callModel(systemPromptPrimary, userMsg, false)
		if err2 == nil {
			if p, f, n, good := tryParse(raw2); good {
				pat, focus, notes, ok = p, f, n, true
				raw, genErr = raw2, nil
			} else if genErr == nil {
				genErr = err2
			}
		} else if genErr == nil {
			genErr = err2
		} else {
			genErr = fmt.Errorf("%v; retry: %v", genErr, err2)
		}
	}
	// Attempt 3: simplified retry prompt (helps small local models)
	if !ok {
		raw3, err3 := callModel(systemPromptRetry, userMsgRetry, false)
		if err3 == nil {
			if p, f, n, good := tryParse(raw3); good {
				pat, focus, notes, ok = p, f, n, true
				raw, genErr = raw3, nil
			} else if genErr == nil && strings.TrimSpace(raw3) != "" {
				genErr = fmt.Errorf("model returned unusable pattern")
			} else if genErr == nil {
				genErr = err3
			}
		} else if genErr == nil {
			genErr = err3
		}
	}
	// Attempt 4: if outsource failed (e.g. OpenRouter free/code 422), fall back to Ollama.
	if !ok && !isOllamaGuardProvider(provider) {
		raw4, err4 := callOllamaChatAny(browserAIGuardBotDefaultModel, systemPromptRetry, userMsgRetry, false, 90*time.Second)
		if err4 == nil {
			if p, f, n, good := tryParse(raw4); good {
				pat, focus, notes, ok = p, f, n, true
				raw, genErr = raw4, nil
				genSource = "ollama-fallback"
				model = browserAIGuardBotDefaultModel
				provider = browserAIGuardBotDefaultProvider
			}
		} else if genErr == nil {
			genErr = err4
		} else {
			genErr = fmt.Errorf("%v; ollama fallback: %v", genErr, err4)
		}
	}

	if !ok {
		msg := "model did not return a usable regex pattern — use AI Prompt evaluate (Download/llama3.2), or edit Pattern manually"
		if genErr != nil {
			msg = "generate-regex failed: " + truncateRunes(genErr.Error(), 180) + " — switch to Download/llama3.2 or a chat model (not free code models)"
		}
		SendError(ctx, fasthttp.StatusBadGateway, msg)
		return
	}
	if focus == "" {
		focus = "policy match"
	}
	_ = raw
	SendJSON(ctx, map[string]any{
		"status":   "success",
		"pattern":  pat,
		"focus":    focus,
		"notes":    notes,
		"model":    model,
		"provider": provider,
		"source":   genSource,
	})
}

func isVisionOnlyGuardModelName(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if strings.Contains(m, "gemma4") || strings.Contains(m, "gemma-4") {
		return false
	}
	return strings.Contains(m, "llava") || strings.Contains(m, "vision") || strings.Contains(m, "bakllava")
}

func sanitizeGeneratedRegex(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	pattern = strings.Trim(pattern, "`\"'")
	pattern = strings.TrimPrefix(pattern, "(?i)")
	pattern = strings.TrimPrefix(pattern, "(?m)")
	pattern = strings.TrimSpace(pattern)
	// Models often emit double-escaped sequences intended as single escapes.
	if strings.Contains(pattern, `\\`) && !strings.Contains(pattern, `\\\\`) {
		if trial := strings.ReplaceAll(pattern, `\\`, `\`); trial != pattern {
			if _, err := regexp.Compile("(?i)" + trial); err == nil {
				pattern = trial
			}
		}
	}
	// Strip unsupported RE2 lookarounds if the rest is usable.
	pattern = stripUnsupportedRE2Lookarounds(pattern)
	return strings.TrimSpace(pattern)
}

func stripUnsupportedRE2Lookarounds(pattern string) string {
	// (?=...), (?!...), (?<=...), (?<!...) are not supported by RE2.
	re := regexp.MustCompile(`\(\?[=!<][^)]*\)`)
	cleaned := re.ReplaceAllString(pattern, "")
	if cleaned == pattern {
		return pattern
	}
	if _, err := regexp.Compile("(?i)" + cleaned); err == nil && isUsableGeneratedRegexLoose(cleaned) {
		return cleaned
	}
	return pattern
}

func isUsableGeneratedRegexLoose(pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	return pattern != "" && len(pattern) >= 2 && len(pattern) <= 400 && !strings.Contains(pattern, "[]")
}

func isUsableGeneratedRegex(pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || len(pattern) < 2 || len(pattern) > 400 {
		return false
	}
	// Reject garbage like []direct[]and[]dial[]
	if strings.Contains(pattern, "[]") {
		return false
	}
	if strings.Count(pattern, " ") > 8 && !strings.ContainsAny(pattern, `\[\].*+?{}|()\\`) {
		return false // looks like a sentence, not a regex
	}
	lower := strings.ToLower(pattern)
	for _, bad := range []string{"http://", "https://", "return ", "you should", "the pattern"} {
		if strings.Contains(lower, bad) {
			return false
		}
	}
	// "policy" alone as pattern is junk; containing the word inside a longer regex is rare — allow compile check to decide.
	if lower == "policy" || strings.HasPrefix(lower, "policy ") {
		return false
	}
	if _, err := regexp.Compile("(?i)" + pattern); err != nil {
		return false
	}
	// Must have at least one "regex-ish" token or a simple word with boundaries
	hasMeta := strings.ContainsAny(pattern, `\.*+?[]{}|()^$`)
	if !hasMeta {
		// plain keyword ok if single token-ish
		if strings.Contains(pattern, " ") {
			return false
		}
	}
	return true
}

func parseGeneratedRegexPayload(raw string) (pattern, focus, notes string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", ""
	}
	var obj struct {
		Pattern string `json:"pattern"`
		Focus   string `json:"focus"`
		Notes   string `json:"notes"`
		Regex   string `json:"regex"`
	}
	if err := sonic.Unmarshal([]byte(raw), &obj); err == nil {
		pattern = strings.TrimSpace(obj.Pattern)
		if pattern == "" {
			pattern = strings.TrimSpace(obj.Regex)
		}
		return pattern, strings.TrimSpace(obj.Focus), strings.TrimSpace(obj.Notes)
	}
	// Fallback: extract "pattern":"..." or 'pattern': '...'
	re := regexp.MustCompile(`(?i)["']pattern["']\s*:\s*["']((?:\\.|[^"'\\])*)["']`)
	if m := re.FindStringSubmatch(raw); len(m) > 1 {
		pattern = m[1]
		pattern = strings.ReplaceAll(pattern, `\\`, `\`)
		pattern = strings.ReplaceAll(pattern, `\"`, `"`)
		return strings.TrimSpace(pattern), "", ""
	}
	re2 := regexp.MustCompile(`(?i)["']regex["']\s*:\s*["']((?:\\.|[^"'\\])*)["']`)
	if m := re2.FindStringSubmatch(raw); len(m) > 1 {
		pattern = m[1]
		pattern = strings.ReplaceAll(pattern, `\\`, `\`)
		return strings.TrimSpace(pattern), "", ""
	}
	// Last resort: first non-empty line that looks like a usable regex
	for _, line := range strings.Split(raw, "\n") {
		line = sanitizeGeneratedRegex(line)
		if line == "" || strings.HasPrefix(line, "{") {
			continue
		}
		if isUsableGeneratedRegex(line) {
			return line, "", ""
		}
	}
	return "", "", ""
}

func resolveGuardBotModel(provider, model string) (schemas.ModelProvider, string) {
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	pLower := strings.ToLower(provider)
	mLower := strings.ToLower(model)
	if pLower != "" && strings.HasPrefix(mLower, pLower+"/") {
		model = model[len(provider)+1:]
	}
	// Keep the admin-selected provider (e.g. openrouter + cohere/north-mini-code:free).
	// Do not let ParseModelString steal "cohere/..." onto native Cohere.
	return schemas.ModelProvider(pLower), model
}

func stringFromUpdate(updates map[string]any, key string) string {
	if updates == nil {
		return ""
	}
	v, ok := updates[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(v)
}

// isWeakGuardEvalModel flags free/code/embed models that often mis-classify Guard policies.
func isWeakGuardEvalModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return false
	}
	return strings.Contains(m, ":free") ||
		strings.Contains(m, "mini-code") ||
		strings.Contains(m, "code-") ||
		strings.HasSuffix(m, "-code") ||
		strings.Contains(m, "embed") ||
		strings.Contains(m, "whisper") ||
		strings.Contains(m, "tts")
}

// isReasoningGuardModel detects thinking/reasoner models that often leave content empty.
func isReasoningGuardModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return false
	}
	return strings.Contains(m, "reasoner") ||
		strings.Contains(m, "reasoning") ||
		strings.Contains(m, "deepseek-r1") ||
		strings.Contains(m, "r1-") ||
		strings.HasSuffix(m, "-r1") ||
		strings.Contains(m, "thinking") ||
		strings.Contains(m, "o1-") ||
		strings.Contains(m, "o3-") ||
		strings.Contains(m, "o4-")
}

// browserAIGuardBotSystemPrompt is used for every AI Guard Bot text rule (any admin policy).
// No predefined DLP topic list — only the admin SECURITY_POLICY defines what to enforce.
func browserAIGuardBotSystemPrompt() string {
	return `You are a careful enterprise policy classifier.

Your ONLY job: decide if CONTENT_TO_EVALUATE clearly breaks SECURITY_POLICY.

Rules:
1. SECURITY_POLICY is the admin's intent. Fix obvious typos (e.g. "notr"→"not"). Informal lists are fine.
2. If the policy forbids a CATEGORY, only a concrete INSTANCE in CONTENT is a violation.
   Example style: policy "fruit names not allowed" + content "banana" → {"violation":true}.
3. Do NOT require the content to repeat the policy wording — but do NOT invent matches.
4. If SECURITY_POLICY mentions PIN/OTP/phone/card/CVV/Aadhaar/SSN (or similar) and CONTENT clearly contains that form → {"violation":true}.
5. If CONTENT is unrelated to the policy, filler with no concrete forbidden instance, or opaque/IDE wire junk → {"violation":false}.
6. Default to {"violation":false} when unsure. Prefer false over false positives.
7. Category policies (e.g. names) need a concrete instance in CONTENT — not a vague guess.

Reply with one JSON object only:
{"violation":true}
or
{"violation":false}`
}

func browserAIGuardBotVisionSystemPrompt() string {
	return `You are a careful enterprise vision policy classifier.

Apply ONLY the admin SECURITY_POLICY to the uploaded image(s) and any EXTRACTED_TEXT.
Interpret policy intent (typos OK). Category policies need concrete visual instances.
Default to {"violation":false} when unsure. Prefer false over false positives.

Reply with one JSON object only:
{"violation":true}
or
{"violation":false}`
}

// buildGuardEvalContent joins the browser chat prompt and any extracted file/audio text
// so the model analyzes everything the employee sent — without adding predefined policies.
func buildGuardEvalContent(prompt, extractedText string) string {
	prompt = strings.TrimSpace(prompt)
	extractedText = strings.TrimSpace(extractedText)
	switch {
	case prompt != "" && extractedText != "" && !strings.EqualFold(prompt, extractedText):
		return "BROWSER_CHAT_PROMPT:\n" + prompt + "\n\nEXTRACTED_TEXT:\n" + extractedText
	case extractedText != "":
		return extractedText
	default:
		return prompt
	}
}

func browserAIGuardBotUserMessage(policy, content string) string {
	policy = strings.TrimSpace(policy)
	if policy == "" {
		policy = "(empty policy — treat as no restriction; violation false)"
	}
	content = strings.TrimSpace(content)
	if content == "" {
		content = "(empty content)"
	}
	return fmt.Sprintf(
		`SECURITY_POLICY:
%s

CONTENT_TO_EVALUATE:
%s

Task: Does CONTENT_TO_EVALUATE clearly break SECURITY_POLICY?
- Concrete forbidden instances only (not guesses).
- Unrelated or opaque/non-prompt junk → {"violation":false}.
- Default {"violation":false} when unsure.

Respond with ONLY: {"violation":true} or {"violation":false}`,
		policy,
		truncateRunes(content, browserAIGuardBotMaxPromptRunes),
	)
}

func browserAIGuardBotStrictUserMessage(policy, content string) string {
	policy = strings.TrimSpace(policy)
	content = strings.TrimSpace(content)
	return fmt.Sprintf(
		`ADMIN_POLICY (intent): %s

EMPLOYEE_TEXT: %s

Decide now.
- True ONLY if EMPLOYEE_TEXT clearly contains something ADMIN_POLICY forbids.
- Unsure or no concrete match → {"violation":false}
JSON only.`,
		truncateRunes(policy, 4000),
		truncateRunes(content, browserAIGuardBotMaxPromptRunes),
	)
}

func (h *BrowserAIHandler) evaluateAIBotRule(rule logstore.BrowserGuardRule, userPrompt string, uploadImages []string) (bool, string) {
	violated, evalErr, _ := h.evaluateAIBotRuleDetailed(rule, userPrompt, uploadImages)
	return violated, evalErr
}

func (h *BrowserAIHandler) evaluateAIBotRuleDetailed(rule logstore.BrowserGuardRule, userPrompt string, uploadImages []string) (bool, string, string) {
	applyAIBotDefaults(&rule)

	if rulePatternMatches(rule, userPrompt) {
		return true, "", "pattern_match"
	}

	providerName, modelName := resolveGuardBotModel(rule.BotProvider, rule.BotModel)
	if providerName == "" || strings.TrimSpace(modelName) == "" {
		return false, "guard bot provider/model missing", ""
	}

	useVision := len(uploadImages) > 0 && (isVisionGuardModel(modelName) || strings.TrimSpace(rule.BotReferenceImage) != "")
	if useVision {
		v, e := h.evaluateAIBotVisionRule(rule, userPrompt, uploadImages, providerName, modelName)
		return v, e, ""
	}

	systemPrompt := browserAIGuardBotSystemPrompt()
	userMsg := browserAIGuardBotUserMessage(rule.BotPrompt, userPrompt)
	shortUserMsg := fmt.Sprintf(
		"SECURITY_POLICY:\n%s\n\nCONTENT:\n%s\n\nIf CONTENT breaks the policy (category instances count), reply ONLY {\"violation\":true} else {\"violation\":false}",
		truncateRunes(strings.TrimSpace(rule.BotPrompt), 4000),
		truncateRunes(strings.TrimSpace(userPrompt), browserAIGuardBotMaxPromptRunes),
	)
	strictUserMsg := browserAIGuardBotStrictUserMessage(rule.BotPrompt, userPrompt)

	if isOllamaGuardProvider(string(providerName)) {
		return runOllamaTextGuardEvalDetailed(modelName, systemPrompt, userMsg, shortUserMsg, strictUserMsg)
	}

	if h.client == nil {
		return false, "unifai client not available", ""
	}

	// Preflight: selected Model Provider must have a usable API key.
	{
		keyCtx := schemas.NewUnifAIContext(context.Background(), time.Now().Add(10*time.Second))
		if _, keyErr := h.client.SelectKeyForProviderRequestType(keyCtx, schemas.ChatCompletionRequest, providerName, modelName); keyErr != nil {
			return false, fmt.Sprintf("no usable API key for provider %s — add/enable the key under Model Providers (model=%s): %v", providerName, modelName, keyErr), ""
		}
	}

	budgetEnd := time.Now().Add(guardBotEvalBudget())
	weakModel := isWeakGuardEvalModel(modelName)
	reasoningModel := isReasoningGuardModel(modelName)
	runOutsource := func(user string, withSystem, withJSON bool) (bool, string, string) {
		remaining := time.Until(budgetEnd)
		if remaining < 2*time.Second {
			return false, "guard bot eval budget exceeded", ""
		}
		unifaiReq := buildGuardOutsourceChatRequest(providerName, modelName, systemPrompt, user, withSystem, withJSON)
		unifaiCtx := schemas.NewUnifAIContext(context.Background(), time.Now().Add(remaining))
		unifaiCtx.SetValue(schemas.UnifAIContextKeySkipBudgetAndRateLimits, true)
		unifaiCtx.SetValue(schemas.UnifAIContextKeySkipPluginPipeline, true)
		unifaiCtx.SetValue(schemas.UnifAIContextKeyPassthroughExtraParams, true)
		resp, unifaiErr := h.client.ChatCompletionRequest(unifaiCtx, unifaiReq)
		if unifaiErr != nil {
			return false, truncateRunes(unifaiErrorMessage(unifaiErr), 220), ""
		}
		rawText := stripEvalMarkdown(evaluatorChoiceText(resp))
		if rawText == "" {
			return false, evaluatorEmptyReason(resp), ""
		}
		violated, recognized := parseAIBotDecision(rawText)
		if !recognized {
			return false, "evaluator returned unparseable output: " + truncateRunes(rawText, 80), rawText
		}
		return violated, "", rawText
	}

	// Few attempts under one shared budget — Send path cannot stack 6× LLM calls.
	// Weak/free/code and reasoner models often reject json_object or leave content empty;
	// start plain-text first for those, JSON-first for normal chat models.
	attempts := []struct {
		user       string
		withSystem bool
		withJSON   bool
	}{
		{shortUserMsg, true, true},
		{userMsg, true, true},
		{strictUserMsg, true, false},
		{shortUserMsg, false, false},
	}
	if weakModel || reasoningModel {
		attempts = []struct {
			user       string
			withSystem bool
			withJSON   bool
		}{
			{shortUserMsg, false, false},
			{strictUserMsg, true, false},
			{shortUserMsg, true, false},
			{userMsg, true, true},
		}
	}
	var firstErr string
	var lastRaw string
	for _, a := range attempts {
		if time.Until(budgetEnd) < 2*time.Second {
			break
		}
		violated, errMsg, raw := runOutsource(a.user, a.withSystem, a.withJSON)
		if raw != "" {
			lastRaw = raw
		}
		if errMsg != "" {
			if firstErr == "" {
				firstErr = errMsg
			}
			continue
		}
		if violated {
			return true, "", raw
		}
		return false, "", raw
	}

	// Last resort only if outsource never returned parseable JSON and budget remains.
	if time.Until(budgetEnd) >= 3*time.Second {
		if violated, ollamaErr, raw := runOllamaTextGuardEvalDetailed(browserAIGuardBotDefaultModel, systemPrompt, userMsg, shortUserMsg, strictUserMsg); ollamaErr == "" {
			return violated, "", raw
		} else if firstErr == "" {
			return false, ollamaErr, lastRaw
		} else {
			return false, firstErr + "; ollama fallback: " + ollamaErr, lastRaw
		}
	}
	return false, firstErr, lastRaw
}

// buildGuardOutsourceChatRequest builds a chat request that works across OpenAI-native
// and OpenAI-compatible providers (OpenRouter, Groq, DeepSeek, etc.) when the
// configured API key matches the selected provider.
func buildGuardOutsourceChatRequest(provider schemas.ModelProvider, model, systemPrompt, user string, withSystem, withJSON bool) *schemas.UnifAIChatRequest {
	// 128 was too low for reasoner models (finish_reason=length + empty content).
	maxTokens := 384
	if isReasoningGuardModel(model) {
		maxTokens = 768
	}
	temp := 0.0
	params := &schemas.ChatParameters{
		Temperature: &temp,
		ExtraParams: map[string]interface{}{
			// Most OpenAI-compatible gateways accept max_tokens; OpenAI also tolerates it.
			"max_tokens": maxTokens,
		},
	}
	// OpenAI / Azure prefer the newer field as well.
	p := strings.ToLower(string(provider))
	if p == string(schemas.OpenAI) || p == string(schemas.Azure) {
		params.MaxCompletionTokens = &maxTokens
	}
	// Skip json_object for weak/reasoner models — many return 422 or empty content.
	if withJSON && !isWeakGuardEvalModel(model) && !isReasoningGuardModel(model) {
		rf := any(map[string]any{"type": "json_object"})
		params.ResponseFormat = &rf
	}

	var input []schemas.ChatMessage
	if withSystem && strings.TrimSpace(systemPrompt) != "" {
		input = []schemas.ChatMessage{
			{
				Role: schemas.ChatMessageRoleSystem,
				Content: &schemas.ChatMessageContent{
					ContentStr: schemas.Ptr(systemPrompt),
				},
			},
			{
				Role: schemas.ChatMessageRoleUser,
				Content: &schemas.ChatMessageContent{
					ContentStr: schemas.Ptr(user),
				},
			},
		}
	} else {
		combined := user
		if strings.TrimSpace(systemPrompt) != "" {
			combined = strings.TrimSpace(systemPrompt) + "\n\n" + user
		}
		input = []schemas.ChatMessage{
			{
				Role: schemas.ChatMessageRoleUser,
				Content: &schemas.ChatMessageContent{
					ContentStr: schemas.Ptr(combined),
				},
			},
		}
	}

	return &schemas.UnifAIChatRequest{
		Provider: provider,
		Model:    model,
		Input:    input,
		Params:   params,
	}
}

// runOllamaTextGuardEval runs the text Guard Bot classifier against Ollama (JSON then plain, full then short).
func runOllamaTextGuardEval(modelName, systemPrompt, userMsg, shortUserMsg string) (bool, string) {
	v, e, _ := runOllamaTextGuardEvalDetailed(modelName, systemPrompt, userMsg, shortUserMsg, "")
	return v, e
}

func runOllamaTextGuardEvalDetailed(modelName, systemPrompt, userMsg, shortUserMsg, strictUserMsg string) (bool, string, string) {
	deadline := time.Now().Add(guardBotEvalBudget())
	runOllama := func(system, user string, jsonMode bool) (bool, string, string) {
		remaining := time.Until(deadline)
		if remaining < 2*time.Second {
			return false, "guard bot eval budget exceeded", ""
		}
		rawText, err := callOllamaChatAny(modelName, system, user, jsonMode, remaining)
		if err != nil {
			return false, truncateRunes(err.Error(), 180), ""
		}
		rawText = stripEvalMarkdown(rawText)
		if rawText == "" {
			return false, "empty evaluator response (Ollama returned no text — confirm llama3.2 is pulled and reachable)", ""
		}
		violated, recognized := parseAIBotDecision(rawText)
		if !recognized {
			return false, "evaluator returned unparseable output: " + truncateRunes(rawText, 80), rawText
		}
		return violated, "", rawText
	}

	// Keep attempts short — enterprise Send latency cannot afford 6×90s Ollama retries.
	attempts := []struct {
		system   string
		user     string
		jsonMode bool
	}{
		{systemPrompt, shortUserMsg, true},
		{systemPrompt, userMsg, true},
		{systemPrompt, shortUserMsg, false},
	}
	if strings.TrimSpace(strictUserMsg) != "" {
		attempts = append(attempts, struct {
			system   string
			user     string
			jsonMode bool
		}{systemPrompt, strictUserMsg, true})
	}

	var firstErr string
	var lastRaw string
	sawClearFalse := false
	for _, a := range attempts {
		if time.Until(deadline) < 2*time.Second {
			break
		}
		violated, errMsg, raw := runOllama(a.system, a.user, a.jsonMode)
		if raw != "" {
			lastRaw = raw
		}
		if errMsg != "" {
			if firstErr == "" {
				firstErr = errMsg
			}
			continue
		}
		if violated {
			return true, "", raw
		}
		sawClearFalse = true
		// First clear parse is enough — do not burn remaining budget on retries.
		return false, "", raw
	}
	if sawClearFalse {
		return false, "", lastRaw
	}
	return false, firstErr, lastRaw
}

func (h *BrowserAIHandler) evaluateAIBotVisionRule(rule logstore.BrowserGuardRule, userPrompt string, uploadImages []string, providerName schemas.ModelProvider, modelName string) (bool, string) {
	systemPrompt := browserAIGuardBotVisionSystemPrompt()

	policy := strings.TrimSpace(rule.BotPrompt)
	if policy == "" {
		policy = "Block uploads that visually match the reference template image."
	}

	userMsg := fmt.Sprintf(
		`SECURITY_POLICY:
%s

EXTRACTED_TEXT (if any):
%s

Evaluate the attached upload image(s) against SECURITY_POLICY / reference template only.
JSON only: {"violation":true} or {"violation":false}`,
		policy,
		truncateRunes(userPrompt, browserAIGuardBotMaxPromptRunes),
	)

	images := make([]string, 0, 1+len(uploadImages))
	if ref := normalizeBase64Image(rule.BotReferenceImage); ref != "" {
		images = append(images, ref)
	}
	for _, img := range uploadImages {
		if cleaned := normalizeBase64Image(img); cleaned != "" {
			images = append(images, cleaned)
		}
	}
	if len(images) == 0 {
		return false, "no vision images to evaluate"
	}

	if isOllamaGuardProvider(string(providerName)) {
		runOllama := func(jsonMode bool) (bool, string) {
			rawText, err := callOllamaVisionAny(modelName, systemPrompt, userMsg, images, jsonMode, 120*time.Second)
			if err != nil {
				return false, truncateRunes(err.Error(), 180)
			}
			rawText = stripEvalMarkdown(rawText)
			if rawText == "" {
				return false, "empty vision evaluator response"
			}
			violated, recognized := parseAIBotDecision(rawText)
			if !recognized {
				return false, "vision evaluator returned unparseable output: " + truncateRunes(rawText, 80)
			}
			return violated, ""
		}
		violated, errMsg := runOllama(true)
		if errMsg == "" {
			return violated, ""
		}
		violated, errMsg2 := runOllama(false)
		if errMsg2 == "" {
			return violated, ""
		}
		return false, errMsg + "; retry: " + errMsg2
	}

	if h.client == nil {
		return false, "unifai client not available"
	}

	// Outsource vision: send text + image data-URLs via UnifAI multimodal chat.
	maxImages := 6
	if len(images) > maxImages {
		images = images[:maxImages]
	}
	blocks := make([]schemas.ChatContentBlock, 0, 1+len(images))
	blocks = append(blocks, schemas.ChatContentBlock{
		Type: schemas.ChatContentBlockTypeText,
		Text: schemas.Ptr(userMsg),
	})
	for _, img := range images {
		img = strings.TrimSpace(img)
		if img == "" {
			continue
		}
		dataURL := img
		if !strings.HasPrefix(strings.ToLower(img), "data:") {
			dataURL = "data:image/png;base64," + img
		}
		blocks = append(blocks, schemas.ChatContentBlock{
			Type: schemas.ChatContentBlockTypeImage,
			ImageURLStruct: &schemas.ChatInputImage{
				URL: dataURL,
			},
		})
	}
	if len(blocks) < 2 {
		return false, "no vision images to evaluate"
	}

	maxTokens := 384
	temp := 0.0
	responseFormat := any(map[string]any{"type": "json_object"})
	unifaiReq := &schemas.UnifAIChatRequest{
		Provider: providerName,
		Model:    modelName,
		Input: []schemas.ChatMessage{
			{
				Role: schemas.ChatMessageRoleSystem,
				Content: &schemas.ChatMessageContent{
					ContentStr: schemas.Ptr(systemPrompt),
				},
			},
			{
				Role: schemas.ChatMessageRoleUser,
				Content: &schemas.ChatMessageContent{
					ContentBlocks: blocks,
				},
			},
		},
		Params: &schemas.ChatParameters{
			MaxCompletionTokens: &maxTokens,
			Temperature:         &temp,
			ResponseFormat:      &responseFormat,
			ExtraParams: map[string]interface{}{
				"max_tokens": maxTokens,
			},
		},
	}

	runOnce := func() (bool, string) {
		deadline := time.Now().Add(45 * time.Second)
		unifaiCtx := schemas.NewUnifAIContext(context.Background(), deadline)
		unifaiCtx.SetValue(schemas.UnifAIContextKeySkipBudgetAndRateLimits, true)
		unifaiCtx.SetValue(schemas.UnifAIContextKeySkipPluginPipeline, true)
		unifaiCtx.SetValue(schemas.UnifAIContextKeyPassthroughExtraParams, true)
		resp, unifaiErr := h.client.ChatCompletionRequest(unifaiCtx, unifaiReq)
		if unifaiErr != nil {
			return false, truncateRunes(unifaiErrorMessage(unifaiErr), 180)
		}
		rawText := stripEvalMarkdown(evaluatorChoiceText(resp))
		if rawText == "" {
			return false, evaluatorEmptyReason(resp)
		}
		violated, recognized := parseAIBotDecision(rawText)
		if !recognized {
			return false, "vision evaluator returned unparseable output: " + truncateRunes(rawText, 80)
		}
		return violated, ""
	}

	violated, errMsg := runOnce()
	if errMsg == "" {
		return violated, ""
	}
	if unifaiReq.Params != nil {
		unifaiReq.Params.ResponseFormat = nil
	}
	violated, errMsg2 := runOnce()
	if errMsg2 == "" {
		return violated, ""
	}
	return false, errMsg + "; retry: " + errMsg2
}

func unifaiErrorMessage(err *schemas.UnifAIError) string {
	if err == nil {
		return "unknown evaluator error"
	}
	parts := make([]string, 0, 4)
	if err.Error != nil {
		if err.Error.Type != nil {
			if t := strings.TrimSpace(*err.Error.Type); t != "" {
				parts = append(parts, t)
			}
		}
		if err.Error.Code != nil {
			if c := strings.TrimSpace(*err.Error.Code); c != "" {
				parts = append(parts, "code="+c)
			}
		}
		if m := strings.TrimSpace(err.Error.Message); m != "" {
			parts = append(parts, m)
		}
	}
	if p := strings.TrimSpace(string(err.ExtraFields.Provider)); p != "" {
		parts = append(parts, "provider="+p)
	}
	if m := strings.TrimSpace(err.ExtraFields.OriginalModelRequested); m != "" {
		parts = append(parts, "model="+m)
	}
	if len(parts) > 0 {
		return strings.Join(parts, ": ")
	}
	return "evaluator request failed"
}

func stripEvalMarkdown(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```json") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimSuffix(raw, "```")
	} else if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
	}
	return strings.TrimSpace(raw)
}

func parseAIBotViolation(raw string) bool {
	v, ok := parseAIBotDecision(raw)
	return ok && v
}

func parseAIBotDecision(raw string) (bool, bool) {
	compact := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(raw, " ", ""), "\n", ""), "\t", ""))
	iTrue := maxIndex(
		strings.LastIndex(compact, `"violation":true`),
		strings.LastIndex(compact, `"violation":1`),
		strings.LastIndex(compact, `violation:true`),
		strings.LastIndex(compact, `"is_violation":true`),
		strings.LastIndex(compact, `"violated":true`),
		strings.LastIndex(compact, `"blocked":true`),
		strings.LastIndex(compact, `"violation":"true"`),
	)
	iFalse := maxIndex(
		strings.LastIndex(compact, `"violation":false`),
		strings.LastIndex(compact, `"violation":0`),
		strings.LastIndex(compact, `violation:false`),
		strings.LastIndex(compact, `"is_violation":false`),
		strings.LastIndex(compact, `"violated":false`),
		strings.LastIndex(compact, `"blocked":false`),
		strings.LastIndex(compact, `"violation":"false"`),
	)
	if iTrue >= 0 || iFalse >= 0 {
		return iTrue > iFalse, true
	}
	if v, ok := violationFromJSON(raw); ok {
		return v, true
	}
	start := strings.LastIndex(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		if v, ok := violationFromJSON(raw[start : end+1]); ok {
			return v, true
		}
	}
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	trimmed = strings.Trim(trimmed, "`\"'")
	switch trimmed {
	case "true", "yes", "block", "blocked", "violation", "violated", "1", "deny", "denied":
		return true, true
	case "false", "no", "allow", "allowed", "safe", "0", "clear", "ok":
		return false, true
	}
	// Last non-empty line often holds the decision for chatty models.
	lines := strings.Split(raw, "\n")
	fullLower := strings.ToLower(strings.TrimSpace(raw))
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.ToLower(strings.TrimSpace(lines[i]))
		line = strings.Trim(line, "`\"'")
		if line == "" || line == fullLower {
			continue
		}
		if v, ok := parseAIBotDecision(line); ok {
			return v, true
		}
		break
	}
	return false, false
}

func maxIndex(vals ...int) int {
	best := -1
	for _, v := range vals {
		if v > best {
			best = v
		}
	}
	return best
}

func violationFromJSON(raw string) (bool, bool) {
	var typed aiBotEvalResult
	if err := sonic.Unmarshal([]byte(raw), &typed); err == nil {
		return typed.Violation, true
	}
	var generic map[string]any
	if err := sonic.Unmarshal([]byte(raw), &generic); err != nil {
		return false, false
	}
	for _, key := range []string{"violation", "is_violation", "violated"} {
		if v, ok := generic[key]; ok {
			return truthyEval(v), true
		}
	}
	return false, false
}

func truthyEval(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "true" || s == "1" || s == "yes"
	case float64:
		return t != 0
	default:
		return false
	}
}

func (h *BrowserAIHandler) generateReplyBotText(ctx *fasthttp.RequestCtx, provider, model, platform, ruleTriggered, userPrompt, kind string) (string, error) {
	provider, model = applyGuardBotDefaults(provider, model)
	providerName, modelName := resolveGuardBotModel(provider, model)

	systemPrompt := `You are UnifAI Guard, an enterprise security assistant embedded in browser AI chats.
A user tried to send sensitive or policy-violating content to a public AI website.
Write a short, professional in-chat reply (4-8 sentences max) that:
1) Clearly states the request was BLOCKED by security policy
2) Names the violation / rule when provided
3) Explains why pasting API keys, passwords, secrets, or confidential company data into web AI chats is prohibited
4) Tells the user to remove secrets and retry with a safe prompt
Do NOT include the secret itself. Do NOT help bypass the policy. Plain text only — no markdown headings.`

	userMsg := fmt.Sprintf(
		"Platform: %s\nViolation rule: %s\nUser prompt (may contain secrets — do not repeat them):\n%s",
		platform,
		ruleTriggered,
		truncateRunes(userPrompt, browserAIGuardBotMaxPromptRunes),
	)

	if kind != "violation" {
		systemPrompt = `You are UnifAI Reply Bot, answering on behalf of an enterprise policy that routes this website's chat through UnifAI.
Answer the user's question helpfully and accurately in plain text (no markdown headings).
Keep replies concise (typically under 12 sentences) unless the user asks for detail.
If the question is unsafe or asks for secrets/credentials, refuse briefly and explain.`
		userMsg = fmt.Sprintf("Platform: %s\nUser question:\n%s", platform, truncateRunes(userPrompt, browserAIGuardBotMaxPromptRunes))
	}

	if isOllamaGuardProvider(string(providerName)) {
		text, err := callOllamaChatAny(modelName, systemPrompt, userMsg, false, 90*time.Second)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(text), nil
	}

	if h.client == nil {
		return "", fmt.Errorf("unifai client not available")
	}

	maxTokens := 350
	temp := 0.3
	if kind != "violation" {
		maxTokens = 900
		temp = 0.5
	}
	unifaiReq := &schemas.UnifAIChatRequest{
		Provider: providerName,
		Model:    modelName,
		Input: []schemas.ChatMessage{
			{
				Role: schemas.ChatMessageRoleSystem,
				Content: &schemas.ChatMessageContent{
					ContentStr: schemas.Ptr(systemPrompt),
				},
			},
			{
				Role: schemas.ChatMessageRoleUser,
				Content: &schemas.ChatMessageContent{
					ContentStr: schemas.Ptr(userMsg),
				},
			},
		},
		Params: &schemas.ChatParameters{
			MaxCompletionTokens: &maxTokens,
			Temperature:         &temp,
		},
	}

	deadline := time.Now().Add(18 * time.Second)
	unifaiCtx := schemas.NewUnifAIContext(context.Background(), deadline)
	unifaiCtx.SetValue(schemas.UnifAIContextKeySkipBudgetAndRateLimits, true)
	unifaiCtx.SetValue(schemas.UnifAIContextKeySkipPluginPipeline, true)
	_ = ctx

	resp, unifaiErr := h.client.ChatCompletionRequest(unifaiCtx, unifaiReq)
	if unifaiErr != nil {
		return "", fmt.Errorf("reply bot completion failed: %v", unifaiErr)
	}
	if resp == nil || len(resp.Choices) == 0 || resp.Choices[0].Message == nil || resp.Choices[0].Message.Content == nil {
		return "", fmt.Errorf("empty reply bot response")
	}
	if resp.Choices[0].Message.Content.ContentStr != nil {
		return strings.TrimSpace(*resp.Choices[0].Message.Content.ContentStr), nil
	}
	return "", fmt.Errorf("reply bot returned non-text content")
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
