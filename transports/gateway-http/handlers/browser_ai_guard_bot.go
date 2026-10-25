package handlers

import (
	"fmt"
	"strings"

	"github.com/gateway/gateway/framework/logstore"
)

const (
	browserAIGuardBotMaxPromptRunes     = 50000
	browserAIGuardBotMaxReferenceImageB = 512 * 1024 // 512 KiB raw base64 payload limit
)

func applyAIBotDefaults(rule *logstore.BrowserGuardRule) {
	if rule == nil || strings.ToLower(strings.TrimSpace(rule.RuleType)) != "ai_bot" {
		return
	}
	rule.BotProvider = strings.TrimSpace(rule.BotProvider)
	rule.BotModel = strings.TrimSpace(rule.BotModel)
}

func applyGuardBotDefaults(provider, model string) (string, string) {
	return strings.TrimSpace(provider), strings.TrimSpace(model)
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
	return strings.Contains(m, "llava") || strings.Contains(m, "vision")
}

func isVisionGuardModel(model string) bool {
	return isVisionOnlyGuardModel(model) || isMultimodalGuardModel(model)
}

// skipAIBotRuleWithoutImages skips vision-only rules when no upload images.
// Multimodal models still run on text-only prompts and extracted file text.
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
	if strings.TrimSpace(rule.BotProvider) == "" || strings.TrimSpace(rule.BotModel) == "" {
		return fmt.Errorf("select a configured model provider and model for the AI Guard Bot rule")
	}
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
		return fmt.Errorf("vision rules require a security policy prompt and/or reference template image")
	}
	return nil
}
