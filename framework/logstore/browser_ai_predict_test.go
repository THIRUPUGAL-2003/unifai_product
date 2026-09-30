package logstore

import (
	"context"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newPredictManager(t *testing.T) *BrowserAIManager {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_predict="+t.Name()), &gorm.Config{})
	if err != nil {
		t.Skipf("sqlite unavailable: %v", err)
	}
	m := NewBrowserAIManager(db)
	if err := m.AutoMigrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	rules := []BrowserGuardRule{
		{ID: "r-block", Name: "Card Number", RuleType: "regex", Severity: "CRITICAL", Action: "BLOCK", Pattern: `\b\d{4}[ -]?\d{4}[ -]?\d{4}[ -]?\d{4}\b`, Active: true},
		{ID: "r-redact", Name: "Email", RuleType: "regex", Severity: "HIGH", Action: "REDACT", Pattern: `[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}`, Active: true},
		{ID: "r-warn", Name: "Confidential", RuleType: "regex", Severity: "MEDIUM", Action: "WARN", Pattern: `confidential`, Active: true},
		{ID: "r-off", Name: "Disabled", RuleType: "regex", Severity: "CRITICAL", Action: "BLOCK", Pattern: `hello`, Active: true},
	}
	for _, r := range rules {
		if err := db.Create(&r).Error; err != nil {
			t.Fatalf("seed rule: %v", err)
		}
	}
	if err := db.Model(&BrowserGuardRule{}).Where("id = ?", "r-off").Update("active", false).Error; err != nil {
		t.Fatalf("disable rule: %v", err)
	}
	return m
}

func TestInterceptPromptPrediction(t *testing.T) {
	ctx := context.Background()
	m := newPredictManager(t)

	cases := []struct {
		name         string
		prompt       string
		metadata     map[string]any
		wantAction   string
		wantRisk     string
		wantCategory string
		wantRule     string
	}{
		{"no rule matches", "hello, summarise this article", nil, "Allowed", "LOW", "SAFE", ""},
		{"block rule", "my card is 4111 1111 1111 1111", nil, "Blocked", "CRITICAL", "SECURITY_POLICY_VIOLATION", "Card Number"},
		{"redact rule is case-insensitive", "mail ME at John.Doe@Example.COM", nil, "Redacted", "HIGH", "SUSPICIOUS_CONTENT", "Email"},
		{"warn rule", "This is CONFIDENTIAL info", nil, "Warned", "LOW", "SUSPICIOUS_CONTENT", "Confidential"},
		{"block beats warn", "confidential card 4111-1111-1111-1111", nil, "Blocked", "CRITICAL", "SECURITY_POLICY_VIOLATION", "Card Number"},
		{"proxy block without reason stays critical", "anything", map[string]any{"is_blocked": true}, "Blocked", "CRITICAL", "SECURITY_POLICY_VIOLATION", ""},
		{"site lock", "[SITE BLOCKED] chat.example.com", map[string]any{"is_blocked": true, "blocked_reason": "Block Entire Website"}, "Blocked", "CRITICAL", "SITE_BLOCK", "Block Entire Website"},
		{"long unmatched prompt bumps to medium", strings.Repeat("word ", 400), nil, "Allowed", "MEDIUM", "SAFE", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			meta := map[string]any{"domain": "chat.example.com"}
			for k, v := range tc.metadata {
				meta[k] = v
			}
			log, _, err := m.InterceptPrompt(ctx, "ChatGPT", tc.prompt, "10.0.0.1", meta)
			if err != nil {
				t.Fatalf("InterceptPrompt: %v", err)
			}
			if log.Action != tc.wantAction || log.PredictiveRisk != tc.wantRisk || log.PredictedCategory != tc.wantCategory || log.RuleTriggered != tc.wantRule {
				t.Fatalf("got action=%s risk=%s category=%s rule=%q; want action=%s risk=%s category=%s rule=%q",
					log.Action, log.PredictiveRisk, log.PredictedCategory, log.RuleTriggered,
					tc.wantAction, tc.wantRisk, tc.wantCategory, tc.wantRule)
			}
		})
	}
}
