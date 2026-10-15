package handlers

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gateway/gateway/framework/logstore"
	"github.com/glebarez/sqlite"
	"github.com/valyala/fasthttp"
	"gorm.io/gorm"
)

func TestBrowserAITokensAndCostFullSuite(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test_tokens_cost.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}
	sqlDB, _ := db.DB()
	if sqlDB != nil {
		defer sqlDB.Close()
	}

	mgr := logstore.NewBrowserAIManager(db)
	if err := mgr.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	// 1. Verify Cost Calculation for various models
	t.Run("CostCalculationFormula", func(t *testing.T) {
		costGpt4o := logstore.CalculateBrowserAICost("ChatGPT", "gpt-4o", 1000, 2000, 0)
		// 1000 * 2.50/1M = 0.0025, 2000 * 10/1M = 0.02 -> total 0.0225
		if costGpt4o < 0.022 || costGpt4o > 0.023 {
			t.Errorf("Expected GPT-4o cost ~0.0225, got %f", costGpt4o)
		}

		costClaude := logstore.CalculateBrowserAICost("Claude", "claude-3-5-sonnet", 1000, 1000, 0)
		// 1000 * 3.0/1M = 0.003, 1000 * 15.0/1M = 0.015 -> total 0.018
		if costClaude < 0.017 || costClaude > 0.019 {
			t.Errorf("Expected Claude cost ~0.018, got %f", costClaude)
		}

		costAudio := logstore.CalculateBrowserAICost("ChatGPT Voice", "gpt-4o", 100, 100, 60.0)
		// 60s * 0.0001 = 0.006 (audio) + token cost
		if costAudio < 0.006 {
			t.Errorf("Expected Audio cost >= 0.006, got %f", costAudio)
		}
	})

	// 2. InterceptPrompt with custom tokens and audio duration
	t.Run("InterceptPromptWithTokensAndCost", func(t *testing.T) {
		meta := map[string]any{
			"prompt_tokens":      45,
			"completion_tokens":  120,
			"audio_duration_sec": 15.5,
		}
		log, _, err := mgr.InterceptPrompt(ctx, "ChatGPT", "Explain quantum computing in detail", "127.0.0.1", meta)
		if err != nil {
			t.Fatalf("InterceptPrompt failed: %v", err)
		}

		if log.PromptTokens != 45 {
			t.Errorf("Expected PromptTokens=45, got %d", log.PromptTokens)
		}
		if log.CompletionTokens != 120 {
			t.Errorf("Expected CompletionTokens=120, got %d", log.CompletionTokens)
		}
		if log.TotalTokens != 165 {
			t.Errorf("Expected TotalTokens=165, got %d", log.TotalTokens)
		}
		if log.AudioDurationSec != 15.5 {
			t.Errorf("Expected AudioDurationSec=15.5, got %f", log.AudioDurationSec)
		}
		if log.Cost <= 0 {
			t.Errorf("Expected Cost > 0, got %f", log.Cost)
		}
	})

	// 3. UpdateLogReplyBot updates completion tokens, total tokens, and cost
	t.Run("UpdateLogReplyBotTokensAndCost", func(t *testing.T) {
		log, _, err := mgr.InterceptPrompt(ctx, "ChatGPT", "What is AI?", "127.0.0.1", nil)
		if err != nil {
			t.Fatalf("InterceptPrompt failed: %v", err)
		}
		initialCost := log.Cost

		replyText := "Artificial Intelligence (AI) refers to computer systems capable of performing tasks that historically required human intelligence."
		err = mgr.UpdateLogReplyBot(ctx, log.ID, "openai", "gpt-4o", replyText)
		if err != nil {
			t.Fatalf("UpdateLogReplyBot failed: %v", err)
		}

		fetched, err := mgr.GetLogByID(ctx, log.ID)
		if err != nil {
			t.Fatalf("GetLogByID failed: %v", err)
		}
		if fetched.CompletionTokens <= 0 {
			t.Errorf("Expected CompletionTokens > 0, got %d", fetched.CompletionTokens)
		}
		if fetched.TotalTokens != fetched.PromptTokens+fetched.CompletionTokens {
			t.Errorf("Expected TotalTokens=%d, got %d", fetched.PromptTokens+fetched.CompletionTokens, fetched.TotalTokens)
		}
		if fetched.Cost <= initialCost {
			t.Errorf("Expected updated cost > initialCost (%f), got %f", initialCost, fetched.Cost)
		}
	})

	// 4. UpdateLogTokensAndCost updates all fields
	t.Run("UpdateLogTokensAndCost", func(t *testing.T) {
		log, _, err := mgr.InterceptPrompt(ctx, "DeepSeek", "Hello DeepSeek", "127.0.0.1", nil)
		if err != nil {
			t.Fatalf("InterceptPrompt failed: %v", err)
		}

		err = mgr.UpdateLogTokensAndCost(ctx, log.ID, 30, 250, 280, 12.0, 0.00045)
		if err != nil {
			t.Fatalf("UpdateLogTokensAndCost failed: %v", err)
		}

		fetched, err := mgr.GetLogByID(ctx, log.ID)
		if err != nil {
			t.Fatalf("GetLogByID failed: %v", err)
		}
		if fetched.PromptTokens != 30 {
			t.Errorf("Expected PromptTokens=30, got %d", fetched.PromptTokens)
		}
		if fetched.CompletionTokens != 250 {
			t.Errorf("Expected CompletionTokens=250, got %d", fetched.CompletionTokens)
		}
		if fetched.TotalTokens != 280 {
			t.Errorf("Expected TotalTokens=280, got %d", fetched.TotalTokens)
		}
		if fetched.AudioDurationSec != 12.0 {
			t.Errorf("Expected AudioDurationSec=12.0, got %f", fetched.AudioDurationSec)
		}
		if fetched.Cost != 0.00045 {
			t.Errorf("Expected Cost=0.00045, got %f", fetched.Cost)
		}
	})

	// 5. GetLogs backwards compatibility
	t.Run("GetLogsBackwardsCompatibility", func(t *testing.T) {
		legacyID := "legacy-test-uuid"
		_ = db.Create(&logstore.BrowserAILog{
			ID:                legacyID,
			Timestamp:         time.Now(),
			Platform:          "ChatGPT",
			Domain:            "chatgpt.com",
			UserPromptPreview: "Legacy prompt preview",
			UserPromptFull:    "Legacy prompt full text for tokens",
			EstTokens:         15,
			ClientIP:          "127.0.0.1",
			Action:            "Allowed",
			Status:            "Allowed",
			CreatedAt:         time.Now(),
		}).Error

		logs, _, err := mgr.GetLogs(ctx, "", "", "", "", 10, 0)
		if err != nil {
			t.Fatalf("GetLogs failed: %v", err)
		}

		var legacyFound *logstore.BrowserAILog
		for _, l := range logs {
			if l.ID == legacyID {
				legacyFound = &l
				break
			}
		}
		if legacyFound == nil {
			t.Fatalf("Legacy log not found")
		}
		if legacyFound.PromptTokens != 15 {
			t.Errorf("Expected legacy PromptTokens=15, got %d", legacyFound.PromptTokens)
		}
		if legacyFound.TotalTokens != 15 {
			t.Errorf("Expected legacy TotalTokens=15, got %d", legacyFound.TotalTokens)
		}
		if legacyFound.Cost <= 0 {
			t.Errorf("Expected legacy Cost > 0, got %f", legacyFound.Cost)
		}
	})

	// 6. HTTP API handler: POST /api/browser-ai/logs/{id}/usage
	t.Run("HTTPUpdateLogUsageHandler", func(t *testing.T) {
		handler := &BrowserAIHandler{
			manager: mgr,
		}

		targetLog, _, _ := mgr.InterceptPrompt(ctx, "Claude", "Tell me a story", "127.0.0.1", nil)

		var reqCtx fasthttp.RequestCtx
		reqCtx.Request.Header.SetMethod("POST")
		reqCtx.SetUserValue("id", targetLog.ID)

		usagePayload := map[string]any{
			"prompt_tokens":      50,
			"completion_tokens":  300,
			"total_tokens":       350,
			"audio_duration_sec": 8.5,
			"cost":               0.00125,
		}
		bodyBytes, _ := sonic.Marshal(usagePayload)
		reqCtx.Request.SetBody(bodyBytes)

		handler.updateLogUsage(&reqCtx)

		if reqCtx.Response.StatusCode() != fasthttp.StatusOK {
			t.Fatalf("Expected 200 OK, got %d: %s", reqCtx.Response.StatusCode(), reqCtx.Response.Body())
		}

		updated, err := mgr.GetLogByID(ctx, targetLog.ID)
		if err != nil {
			t.Fatalf("GetLogByID failed: %v", err)
		}
		if updated.PromptTokens != 50 || updated.CompletionTokens != 300 || updated.TotalTokens != 350 {
			t.Errorf("Usage mismatch: got in=%d, out=%d, total=%d", updated.PromptTokens, updated.CompletionTokens, updated.TotalTokens)
		}
		if updated.AudioDurationSec != 8.5 {
			t.Errorf("Audio duration mismatch: got %f", updated.AudioDurationSec)
		}
		if updated.Cost != 0.00125 {
			t.Errorf("Cost mismatch: got %f", updated.Cost)
		}
	})
}
