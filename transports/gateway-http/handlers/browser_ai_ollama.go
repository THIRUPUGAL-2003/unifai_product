package handlers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gateway/gateway/framework/logstore"
)

const (
	browserAIGuardBotDefaultProvider = "ollama"
	browserAIGuardBotDefaultModel    = "llama3.2"
	// Ollama base URL comes only from env: BROWSER_AI_OLLAMA_URL / OLLAMA_URL / OLLAMA_BASE_URL.
	browserAIGuardBotMaxPromptRunes     = 50000
	browserAIGuardBotMaxReferenceImageB = 512 * 1024 // 512 KiB raw base64 payload limit
)

type ollamaChatMessage struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"`
}

type ollamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
	Format   string              `json:"format,omitempty"`
	Options  map[string]any      `json:"options,omitempty"`
}

// ollamaGuardEvalOptions keeps guard bot decisions deterministic (same input → same output).
func ollamaGuardEvalOptions() map[string]any {
	return map[string]any{
		"temperature": 0,
		"num_predict": 256,
	}
}

type ollamaChatResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Error string `json:"error,omitempty"`
}

var (
	cachedOllamaBaseURL string
	ollamaURLCacheMu    sync.RWMutex
)

func addOllamaURLCandidate(seen map[string]bool, out *[]string, raw string) {
	u := strings.TrimRight(strings.TrimSpace(raw), "/")
	if u == "" || seen[u] {
		return
	}
	seen[u] = true
	*out = append(*out, u)
}

// ollamaBaseURLCandidates lists URLs from env only (no hardcoded :11434 hosts).
// Set BROWSER_AI_OLLAMA_URL or OLLAMA_URL in .env.
func ollamaBaseURLCandidates() []string {
	seen := map[string]bool{}
	out := make([]string, 0, 4)
	for _, key := range []string{"BROWSER_AI_OLLAMA_URL", "OLLAMA_URL", "OLLAMA_BASE_URL"} {
		addOllamaURLCandidate(seen, &out, os.Getenv(key))
	}
	ollamaURLCacheMu.RLock()
	if cachedOllamaBaseURL != "" {
		addOllamaURLCandidate(seen, &out, cachedOllamaBaseURL)
	}
	ollamaURLCacheMu.RUnlock()
	return out
}

func defaultOllamaBaseURL() string {
	candidates := ollamaBaseURLCandidates()
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

func rememberWorkingOllamaURL(baseURL string) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return
	}
	ollamaURLCacheMu.Lock()
	cachedOllamaBaseURL = baseURL
	ollamaURLCacheMu.Unlock()
}

// resolveWorkingOllamaBase finds a live Ollama with a short /api/tags probe.
// Never spend the full chat timeout on dead candidate URLs (that made Guard eval
// miss the proxy's ~95s deadline and look like "model never evaluates").
func resolveWorkingOllamaBase(probeTimeout time.Duration) (string, error) {
	if probeTimeout <= 0 {
		probeTimeout = 1500 * time.Millisecond
	}
	// Hard cap total probe budget so dead Docker hostnames cannot burn Guard Bot eval time.
	deadline := time.Now().Add(4 * time.Second)

	ollamaURLCacheMu.RLock()
	cached := cachedOllamaBaseURL
	ollamaURLCacheMu.RUnlock()

	candidates := ollamaBaseURLCandidates()
	ordered := make([]string, 0, len(candidates)+1)
	seen := map[string]bool{}
	if cached != "" {
		ordered = append(ordered, cached)
		seen[cached] = true
	}
	for _, u := range candidates {
		if seen[u] {
			continue
		}
		seen[u] = true
		ordered = append(ordered, u)
	}

	var errs []string
	for _, base := range ordered {
		if time.Now().After(deadline) {
			errs = append(errs, "probe budget exhausted")
			break
		}
		left := time.Until(deadline)
		pt := probeTimeout
		if left < pt {
			pt = left
		}
		if pt < 200*time.Millisecond {
			errs = append(errs, "probe budget exhausted")
			break
		}
		if _, err := fetchOllamaTags(base, pt); err == nil {
			rememberWorkingOllamaURL(base)
			return base, nil
		} else {
			errs = append(errs, truncateRunes(base+": "+err.Error(), 100))
		}
	}
	if len(errs) == 0 {
		return "", fmt.Errorf("ollama unreachable: no candidate URLs configured (set BROWSER_AI_OLLAMA_URL)")
	}
	return "", fmt.Errorf("ollama unreachable (%d probes): %s — set BROWSER_AI_OLLAMA_URL", len(errs), strings.Join(errs, "; "))
}

// callOllamaChatAny probes for a live Ollama once, then runs a single chat call.
func callOllamaChatAny(model, systemPrompt, userMsg string, jsonMode bool, timeout time.Duration) (string, error) {
	base, err := resolveWorkingOllamaBase(3 * time.Second)
	if err != nil {
		return "", err
	}
	text, chatErr := callOllamaChat(base, model, systemPrompt, userMsg, jsonMode, timeout)
	if chatErr == nil {
		return text, nil
	}
	// Working URL may have gone stale — clear cache, re-probe, one retry.
	ollamaURLCacheMu.Lock()
	if cachedOllamaBaseURL == base {
		cachedOllamaBaseURL = ""
	}
	ollamaURLCacheMu.Unlock()
	base2, err2 := resolveWorkingOllamaBase(3 * time.Second)
	if err2 != nil {
		return "", fmt.Errorf("%v; re-probe: %v", chatErr, err2)
	}
	return callOllamaChat(base2, model, systemPrompt, userMsg, jsonMode, timeout)
}

func isOllamaGuardProvider(provider string) bool {
	p := strings.ToLower(strings.TrimSpace(provider))
	return p == "" || p == "ollama"
}

func applyAIBotDefaults(rule *logstore.BrowserGuardRule) {
	if rule == nil || strings.ToLower(strings.TrimSpace(rule.RuleType)) != "ai_bot" {
		return
	}
	if strings.TrimSpace(rule.BotProvider) == "" {
		rule.BotProvider = browserAIGuardBotDefaultProvider
	}
	if strings.TrimSpace(rule.BotModel) == "" {
		rule.BotModel = browserAIGuardBotDefaultModel
	}
}

func applyGuardBotDefaults(provider, model string) (string, string) {
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	if provider == "" {
		provider = browserAIGuardBotDefaultProvider
	}
	if model == "" {
		model = browserAIGuardBotDefaultModel
	}
	return provider, model
}

func isMultimodalGuardModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(m, "gemma4") || strings.Contains(m, "gemma-4")
}

func isVisionOnlyGuardModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if isMultimodalGuardModel(m) {
		return false
	}
	return strings.Contains(m, "llava") || strings.Contains(m, "vision") || strings.Contains(m, "bakllava")
}

func isVisionGuardModel(model string) bool {
	return isVisionOnlyGuardModel(model) || isMultimodalGuardModel(model)
}

// skipAIBotRuleWithoutImages skips vision-only rules (LLaVA) when no upload images.
// Multimodal models (e.g. gemma4) still run on text-only prompts and extracted file text.
func skipAIBotRuleWithoutImages(model, referenceImage string, uploadImages []string) bool {
	if len(uploadImages) > 0 {
		return false
	}
	if isMultimodalGuardModel(model) {
		return false
	}
	return isVisionOnlyGuardModel(model) || strings.TrimSpace(referenceImage) != ""
}

func normalizeBase64Image(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if idx := strings.Index(raw, "base64,"); idx >= 0 {
		raw = raw[idx+len("base64,"):]
	}
	return strings.TrimSpace(raw)
}

func validateAIBotRuleFields(rule *logstore.BrowserGuardRule) error {
	if rule == nil {
		return fmt.Errorf("rule is nil")
	}
	applyAIBotDefaults(rule)
	rule.BotPrompt = strings.TrimSpace(rule.BotPrompt)
	rule.BotReferenceImage = normalizeBase64Image(rule.BotReferenceImage)
	rule.BotReferenceImageType = strings.TrimSpace(rule.BotReferenceImageType)
	if rule.BotPrompt == "" && rule.BotReferenceImage == "" {
		return fmt.Errorf("evaluation prompt or reference template image is required for AI Guard Bot rule")
	}
	if rule.BotReferenceImage != "" {
		if len(rule.BotReferenceImage) > browserAIGuardBotMaxReferenceImageB*2 {
			return fmt.Errorf("reference template image is too large (max %d KB)", browserAIGuardBotMaxReferenceImageB/1024)
		}
		if rule.BotReferenceImageType == "" {
			rule.BotReferenceImageType = "image/png"
		}
	}
	if isVisionOnlyGuardModel(rule.BotModel) && rule.BotReferenceImage == "" && rule.BotPrompt == "" {
		return fmt.Errorf("LLaVA vision rules require a security policy prompt and/or reference template image")
	}
	return nil
}

// callOllamaVisionAny probes for a live Ollama once, then runs a single vision call.
func callOllamaVisionAny(model, systemPrompt, userMsg string, images []string, jsonMode bool, timeout time.Duration) (string, error) {
	base, err := resolveWorkingOllamaBase(3 * time.Second)
	if err != nil {
		return "", err
	}
	text, chatErr := callOllamaVision(base, model, systemPrompt, userMsg, images, jsonMode, timeout)
	if chatErr == nil {
		return text, nil
	}
	ollamaURLCacheMu.Lock()
	if cachedOllamaBaseURL == base {
		cachedOllamaBaseURL = ""
	}
	ollamaURLCacheMu.Unlock()
	base2, err2 := resolveWorkingOllamaBase(3 * time.Second)
	if err2 != nil {
		return "", fmt.Errorf("%v; re-probe: %v", chatErr, err2)
	}
	return callOllamaVision(base2, model, systemPrompt, userMsg, images, jsonMode, timeout)
}

func callOllamaVision(baseURL, model, systemPrompt, userMsg string, images []string, jsonMode bool, timeout time.Duration) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = defaultOllamaBaseURL()
	}
	if baseURL == "" {
		return "", fmt.Errorf("ollama URL not set — set BROWSER_AI_OLLAMA_URL or OLLAMA_URL in .env")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = browserAIGuardBotDefaultModel
	}

	cleanImages := make([]string, 0, len(images))
	for _, img := range images {
		img = normalizeBase64Image(img)
		if img != "" {
			cleanImages = append(cleanImages, img)
		}
	}

	messages := make([]ollamaChatMessage, 0, 2)
	if strings.TrimSpace(systemPrompt) != "" {
		messages = append(messages, ollamaChatMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}
	userMessage := ollamaChatMessage{
		Role:    "user",
		Content: userMsg,
	}
	if len(cleanImages) > 0 {
		userMessage.Images = cleanImages
	}
	messages = append(messages, userMessage)

	reqBody := ollamaChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
		Options:  ollamaGuardEvalOptions(),
	}
	if jsonMode {
		reqBody.Format = "json"
	}

	body, err := sonic.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("ollama vision request encode failed: %w", err)
	}

	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ollama vision request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama unreachable at %s: %w", baseURL, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ollama vision response read failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama HTTP %d: %s", resp.StatusCode, truncateRunes(string(respBody), 180))
	}

	var parsed ollamaChatResponse
	if err := sonic.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("ollama vision response decode failed: %w", err)
	}
	if strings.TrimSpace(parsed.Error) != "" {
		return "", fmt.Errorf("ollama error: %s", strings.TrimSpace(parsed.Error))
	}
	text := strings.TrimSpace(parsed.Message.Content)
	if text == "" {
		return "", fmt.Errorf("empty ollama vision response")
	}
	return text, nil
}

func callOllamaChat(baseURL, model, systemPrompt, userMsg string, jsonMode bool, timeout time.Duration) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = defaultOllamaBaseURL()
	}
	if baseURL == "" {
		return "", fmt.Errorf("ollama URL not set — set BROWSER_AI_OLLAMA_URL or OLLAMA_URL in .env")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = browserAIGuardBotDefaultModel
	}

	messages := make([]ollamaChatMessage, 0, 2)
	if strings.TrimSpace(systemPrompt) != "" {
		messages = append(messages, ollamaChatMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}
	messages = append(messages, ollamaChatMessage{
		Role:    "user",
		Content: userMsg,
	})

	reqBody := ollamaChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
		Options:  ollamaGuardEvalOptions(),
	}
	if jsonMode {
		reqBody.Format = "json"
	}

	body, err := sonic.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("ollama request encode failed: %w", err)
	}

	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ollama request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama unreachable at %s: %w", baseURL, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ollama response read failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama HTTP %d: %s", resp.StatusCode, truncateRunes(string(respBody), 180))
	}

	var parsed ollamaChatResponse
	if err := sonic.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("ollama response decode failed: %w", err)
	}
	if strings.TrimSpace(parsed.Error) != "" {
		return "", fmt.Errorf("ollama error: %s", strings.TrimSpace(parsed.Error))
	}
	text := strings.TrimSpace(parsed.Message.Content)
	if text == "" {
		return "", fmt.Errorf("empty ollama response")
	}
	return text, nil
}

type ollamaTagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// listOllamaInstalledModels returns model names from the live Ollama /api/tags endpoint.
func listOllamaInstalledModels(timeout time.Duration) ([]string, string, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	// Cap per-URL probe so a dead candidate cannot burn the whole budget.
	probe := timeout
	if probe > 3*time.Second {
		probe = 3 * time.Second
	}
	base, err := resolveWorkingOllamaBase(probe)
	if err != nil {
		return nil, "", err
	}
	names, err := fetchOllamaTags(base, timeout)
	if err != nil {
		return nil, "", err
	}
	return names, base, nil
}

func fetchOllamaTags(baseURL string, timeout time.Duration) ([]string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("empty ollama base URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("ollama tags request failed: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama unreachable at %s: %w", baseURL, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ollama tags read failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama HTTP %d: %s", resp.StatusCode, truncateRunes(string(respBody), 180))
	}

	var parsed ollamaTagsResponse
	if err := sonic.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("ollama tags decode failed: %w", err)
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		name := strings.TrimSpace(m.Name)
		name = strings.TrimSuffix(name, ":latest")
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("ollama at %s returned no models", baseURL)
	}
	return out, nil
}
