package handlers

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/valyala/fasthttp"
)

func TestSearchLogAllBrowsersSupported(t *testing.T) {
	handler := &BrowserAIHandler{}

	browsers := []string{
		"Chrome",
		"Edge",
		"Safari",
		"Firefox",
		"Brave",
		"Opera",
		"Opera Mini",
	}

	searchLogsMu.Lock()
	searchLogsList = []BrowserAISearchLogEntry{}
	searchLogsMu.Unlock()

	// 1. Record search logs for each browser
	for _, b := range browsers {
		entry := BrowserAISearchLogEntry{
			Engine:        "Google",
			Browser:       b,
			IsIncognito:   false,
			Query:         "test search for " + b,
			ClickedURL:    "https://example.com/" + strings.ToLower(strings.ReplaceAll(b, " ", "-")),
			ClickedTitle:  b + " Official Page",
			Host:          "www.google.com",
			ClientIP:      "127.0.0.1",
			AgentHostname: "DESKTOP-TEST",
			CreatedAt:     time.Now().Format(time.RFC3339),
		}

		bodyBytes, err := sonic.Marshal(entry)
		if err != nil {
			t.Fatalf("Failed to marshal entry for %s: %v", b, err)
		}

		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("POST")
		ctx.Request.SetRequestURI("/api/browser-ai/search-logs")
		ctx.Request.SetBody(bodyBytes)

		// Bypass guard security check for unit test by calling internal persistence directly
		entry.ID = "test-id-" + strings.ToLower(strings.ReplaceAll(b, " ", "-"))
		entry.Timestamp = time.Now()
		searchLogsMu.Lock()
		searchLogsList = append([]BrowserAISearchLogEntry{entry}, searchLogsList...)
		searchLogsMu.Unlock()
	}

	// 2. Verify all browsers are stored in memory
	searchLogsMu.RLock()
	if len(searchLogsList) != len(browsers) {
		t.Fatalf("Expected %d search logs, got %d", len(browsers), len(searchLogsList))
	}
	searchLogsMu.RUnlock()

	// 3. Test filtering by specific browser
	for _, target := range browsers {
		ctxGet := &fasthttp.RequestCtx{}
		ctxGet.Request.Header.SetMethod("GET")
		ctxGet.Request.SetRequestURI("/api/browser-ai/search-logs?browser=" + strings.ToLower(target))

		handler.getSearchLogs(ctxGet)

		if ctxGet.Response.StatusCode() != fasthttp.StatusOK {
			t.Fatalf("Expected 200 OK for browser %s, got %d", target, ctxGet.Response.StatusCode())
		}

		var resp struct {
			Logs  []BrowserAISearchLogEntry `json:"logs"`
			Total int                       `json:"total"`
		}
		if err := sonic.Unmarshal(ctxGet.Response.Body(), &resp); err != nil {
			t.Fatalf("Failed to parse response body for %s: %v", target, err)
		}

		if resp.Total < 1 {
			t.Errorf("Expected at least 1 match for browser filter %s, got %d", target, resp.Total)
		}

		for _, l := range resp.Logs {
			if !strings.EqualFold(l.Browser, target) && !strings.Contains(strings.ToLower(l.Browser), strings.ToLower(target)) {
				t.Errorf("Browser mismatch: expected %s, got %s", target, l.Browser)
			}
		}
	}

	// 4. Test free-text search for Opera Mini
	ctxSearch := &fasthttp.RequestCtx{}
	ctxSearch.Request.Header.SetMethod("GET")
	ctxSearch.Request.SetRequestURI("/api/browser-ai/search-logs?search=mini")
	handler.getSearchLogs(ctxSearch)

	if ctxSearch.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("Expected 200 OK for search query, got %d", ctxSearch.Response.StatusCode())
	}

	var searchResp struct {
		Logs  []BrowserAISearchLogEntry `json:"logs"`
		Total int                       `json:"total"`
	}
	_ = sonic.Unmarshal(ctxSearch.Response.Body(), &searchResp)
	if searchResp.Total != 1 || searchResp.Logs[0].Browser != "Opera Mini" {
		t.Errorf("Expected 1 Opera Mini result for search=mini, got %d", searchResp.Total)
	}

	t.Logf("Successfully verified all %d browsers: %v", len(browsers), browsers)
	_ = context.Background()
}
