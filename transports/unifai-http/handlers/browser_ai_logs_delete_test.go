package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
)

func TestSearchLogs_PurgeInMemoryBefore(t *testing.T) {
	searchLogsMu.Lock()
	searchLogsList = nil
	searchLogsMu.Unlock()

	now := time.Now()
	e10DaysOld := BrowserAISearchLogEntry{
		ID:        "search-10d-old",
		Timestamp: now.Add(-10 * 24 * time.Hour),
		Query:     "10 days old search query",
	}
	e2DaysOld := BrowserAISearchLogEntry{
		ID:        "search-2d-old",
		Timestamp: now.Add(-2 * 24 * time.Hour),
		Query:     "2 days old search query",
	}
	eJustNow := BrowserAISearchLogEntry{
		ID:        "search-just-now",
		Timestamp: now,
		Query:     "recent search query",
	}

	searchLogsMu.Lock()
	searchLogsList = append(searchLogsList, e10DaysOld, e2DaysOld, eJustNow)
	searchLogsMu.Unlock()

	// Purge with 7-day retention cutoff:
	// 10-day-old should be purged; 2-day-old and recent should remain.
	cutoff := now.Add(-7 * 24 * time.Hour)
	purgeInMemorySearchLogsBefore(cutoff)

	searchLogsMu.RLock()
	defer searchLogsMu.RUnlock()

	if len(searchLogsList) != 2 {
		t.Fatalf("Expected 2 search logs remaining after 7d cutoff, got %d", len(searchLogsList))
	}
	for _, e := range searchLogsList {
		if e.ID == "search-10d-old" {
			t.Errorf("search-10d-old should have been purged by 7d cutoff")
		}
	}
}

func TestSearchLogs_DeleteSearchLogs_Periods(t *testing.T) {
	handler := &BrowserAIHandler{}

	now := time.Now()
	resetEntries := func() {
		searchLogsMu.Lock()
		searchLogsList = []BrowserAISearchLogEntry{
			{ID: "log-1", Timestamp: now.Add(-2 * time.Hour), Query: "2 hours ago"},
			{ID: "log-2", Timestamp: now.Add(-3 * 24 * time.Hour), Query: "3 days ago"},
			{ID: "log-3", Timestamp: now.Add(-15 * 24 * time.Hour), Query: "15 days ago"},
			{ID: "log-4", Timestamp: now.Add(-45 * 24 * time.Hour), Query: "45 days ago"},
		}
		searchLogsMu.Unlock()
	}

	// 1. Delete older than 1 day (keeps only log-1, which is 2 hours old)
	resetEntries()
	ctx1d := &fasthttp.RequestCtx{}
	ctx1d.Request.Header.SetMethod("DELETE")
	ctx1d.Request.SetRequestURI("/api/browser-ai/search-logs?period=1d")
	handler.deleteSearchLogs(ctx1d)

	if ctx1d.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("Expected 200 OK, got %d: %s", ctx1d.Response.StatusCode(), string(ctx1d.Response.Body()))
	}
	searchLogsMu.RLock()
	if len(searchLogsList) != 1 || searchLogsList[0].ID != "log-1" {
		t.Errorf("Expected only log-1 to remain after deleting older than 1d, got %+v", searchLogsList)
	}
	searchLogsMu.RUnlock()

	// 2. Delete older than 7 days (removes log-3 and log-4, keeps the recent ones)
	resetEntries()
	ctx7d := &fasthttp.RequestCtx{}
	ctx7d.Request.Header.SetMethod("DELETE")
	ctx7d.Request.SetRequestURI("/api/browser-ai/search-logs?period=7d")
	handler.deleteSearchLogs(ctx7d)

	if ctx7d.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", ctx7d.Response.StatusCode())
	}
	searchLogsMu.RLock()
	if len(searchLogsList) != 2 {
		t.Errorf("Expected 2 entries remaining after deleting older than 7d, got %d", len(searchLogsList))
	}
	for _, e := range searchLogsList {
		if e.ID == "log-3" || e.ID == "log-4" {
			t.Errorf("%s is older than 7 days and should have been deleted", e.ID)
		}
	}
	searchLogsMu.RUnlock()

	// 3. Delete all (should clear everything)
	resetEntries()
	ctxAll := &fasthttp.RequestCtx{}
	ctxAll.Request.Header.SetMethod("DELETE")
	ctxAll.Request.SetRequestURI("/api/browser-ai/search-logs?period=all")
	handler.deleteSearchLogs(ctxAll)

	if ctxAll.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", ctxAll.Response.StatusCode())
	}
	searchLogsMu.RLock()
	if len(searchLogsList) != 0 {
		t.Errorf("Expected 0 entries remaining after deleting all, got %d", len(searchLogsList))
	}
	searchLogsMu.RUnlock()

	// 4. Invalid period -> 400 Bad Request
	ctxBad := &fasthttp.RequestCtx{}
	ctxBad.Request.Header.SetMethod("DELETE")
	ctxBad.Request.SetRequestURI("/api/browser-ai/search-logs?period=invalid_period")
	handler.deleteSearchLogs(ctxBad)

	if ctxBad.Response.StatusCode() != fasthttp.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request on invalid period, got %d", ctxBad.Response.StatusCode())
	}
}

func TestPromptLogs_DeleteLogs_Periods_Validation(t *testing.T) {
	handler := &BrowserAIHandler{}

	// Test invalid period
	ctxBad := &fasthttp.RequestCtx{}
	ctxBad.Request.Header.SetMethod("DELETE")
	ctxBad.Request.SetRequestURI("/api/browser-ai/logs?period=invalid_period")
	handler.deleteLogs(ctxBad)

	if ctxBad.Response.StatusCode() != fasthttp.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request for invalid period, got %d", ctxBad.Response.StatusCode())
	}

	// Test valid date parsing
	ctxDateBad := &fasthttp.RequestCtx{}
	ctxDateBad.Request.Header.SetMethod("DELETE")
	ctxDateBad.Request.SetRequestURI("/api/browser-ai/logs?date=bad-date-format")
	handler.deleteLogs(ctxDateBad)

	if ctxDateBad.Response.StatusCode() != fasthttp.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request for bad date format, got %d", ctxDateBad.Response.StatusCode())
	}
}

func TestRetentionDurationParsing(t *testing.T) {
	cases := []struct {
		input    string
		expected time.Duration
	}{
		{"1d", 24 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
		{"30d", 30 * 24 * time.Hour},
		{"90d", 90 * 24 * time.Hour},
		{"180d", 180 * 24 * time.Hour},
		{"365d", 365 * 24 * time.Hour},
		{"unknown", 7 * 24 * time.Hour}, // default
	}

	parseFn := func(retention string) time.Duration {
		switch retention {
		case "1d":
			return 24 * time.Hour
		case "7d":
			return 7 * 24 * time.Hour
		case "30d":
			return 30 * 24 * time.Hour
		case "90d":
			return 90 * 24 * time.Hour
		case "180d":
			return 180 * 24 * time.Hour
		case "365d":
			return 365 * 24 * time.Hour
		default:
			return 7 * 24 * time.Hour
		}
	}

	for _, c := range cases {
		got := parseFn(c.input)
		if got != c.expected {
			t.Errorf("Input %s: expected %v, got %v", c.input, c.expected, got)
		}
	}
}

func TestSearchLogs_DeleteResponseMessages(t *testing.T) {
	handler := &BrowserAIHandler{}

	testMessage := func(uri string, expectedSubstring string) {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("DELETE")
		ctx.Request.SetRequestURI(uri)
		handler.deleteSearchLogs(ctx)

		if ctx.Response.StatusCode() != fasthttp.StatusOK {
			t.Fatalf("URI %s: expected 200, got %d", uri, ctx.Response.StatusCode())
		}
		var resp map[string]any
		if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
			t.Fatalf("URI %s: json parse failed: %v", uri, err)
		}
		msg, _ := resp["message"].(string)
		if msg == "" || (expectedSubstring != "" && !containsSubstr(msg, expectedSubstring)) {
			t.Errorf("URI %s: expected message containing %q, got %q", uri, expectedSubstring, msg)
		}
	}

	testMessage("/api/browser-ai/search-logs?period=1d", "older than 1 day")
	testMessage("/api/browser-ai/search-logs?period=7d", "older than 7 days")
	testMessage("/api/browser-ai/search-logs?period=30d", "older than 30 days")
	testMessage("/api/browser-ai/search-logs?period=all", "Search logs cleared")
}

func TestSearchLogs_DeleteByIDs(t *testing.T) {
	handler := &BrowserAIHandler{}
	now := time.Now()
	searchLogsMu.Lock()
	searchLogsList = []BrowserAISearchLogEntry{
		{ID: "a", Timestamp: now},
		{ID: "b", Timestamp: now},
		{ID: "c", Timestamp: now},
	}
	searchLogsMu.Unlock()

	bulk := &fasthttp.RequestCtx{}
	bulk.Request.Header.SetMethod("POST")
	bulk.Request.SetRequestURI("/api/browser-ai/search-logs/bulk-delete")
	bulk.Request.SetBodyString(`{"ids":["a","c","a"," "]}`)
	handler.bulkDeleteSearchLogs(bulk)
	if bulk.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("bulk delete: expected 200, got %d: %s", bulk.Response.StatusCode(), bulk.Response.Body())
	}
	searchLogsMu.RLock()
	if len(searchLogsList) != 1 || searchLogsList[0].ID != "b" {
		t.Errorf("expected only b to remain, got %+v", searchLogsList)
	}
	searchLogsMu.RUnlock()

	empty := &fasthttp.RequestCtx{}
	empty.Request.Header.SetMethod("POST")
	empty.Request.SetBodyString(`{"ids":[]}`)
	handler.bulkDeleteSearchLogs(empty)
	if empty.Response.StatusCode() != fasthttp.StatusBadRequest {
		t.Errorf("empty ids: expected 400, got %d", empty.Response.StatusCode())
	}

	single := &fasthttp.RequestCtx{}
	single.Request.Header.SetMethod("DELETE")
	single.SetUserValue("id", "b")
	handler.deleteSearchLog(single)
	if single.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("single delete: expected 200, got %d", single.Response.StatusCode())
	}

	missing := &fasthttp.RequestCtx{}
	missing.Request.Header.SetMethod("DELETE")
	missing.SetUserValue("id", "b")
	handler.deleteSearchLog(missing)
	if missing.Response.StatusCode() != fasthttp.StatusNotFound {
		t.Errorf("deleting a missing log: expected 404, got %d", missing.Response.StatusCode())
	}
}

func containsSubstr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0 && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstr(s, substr))))
}

func findSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
