package handlers

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/unifai/unifai/framework/logstore"
	"github.com/valyala/fasthttp"
)

// BrowserAISearchLogEntry represents an in-memory live search event.
// As instructed, this is kept in-memory (no database required for now).
type BrowserAISearchLogEntry struct {
	ID             string    `json:"id"`
	Timestamp      time.Time `json:"timestamp"`
	Engine         string    `json:"engine"`          // "Google", "Bing", "Safari / Apple", "DuckDuckGo", "Yahoo"
	Browser        string    `json:"browser"`         // "Edge", "Chrome", "Safari", "Firefox", "Brave"
	IsIncognito    bool      `json:"is_incognito"`    // true if incognito / private mode
	Query          string    `json:"query"`           // Searched keywords
	ClickedURL     string    `json:"clicked_url"`     // URL of search result link clicked
	ClickedTitle   string    `json:"clicked_title"`   // Host or title of clicked result
	URL            string    `json:"url"`             // Raw search URL
	Host           string    `json:"host"`            // Search engine host
	ClientIP       string    `json:"client_ip"`       // Originating IP
	AgentHostname  string    `json:"agent_hostname"`  // Device hostname
	AgentID        string    `json:"agent_id"`        // Device / agent ID
	RiskScore      int       `json:"risk_score"`      // 0 - 100
	PredictiveRisk string    `json:"predictive_risk"` // "LOW" | "MEDIUM" | "HIGH" | "CRITICAL"
	RiskCategory   string    `json:"risk_category"`   // "DLP / Data Leak", "Exploit / Bypass", "Reconnaissance", "General Search"
	CreatedAt      string    `json:"created_at"`
}

var (
	searchLogsMu   sync.RWMutex
	searchLogsList []BrowserAISearchLogEntry
)

func purgeInMemorySearchLogsBefore(cutoff time.Time) {
	searchLogsMu.Lock()
	defer searchLogsMu.Unlock()
	kept := searchLogsList[:0]
	for _, e := range searchLogsList {
		ts := e.Timestamp
		if ts.IsZero() {
			if parsed, err := time.Parse(time.RFC3339, e.CreatedAt); err == nil {
				ts = parsed
			}
		}
		if ts.Before(cutoff) {
			continue
		}
		kept = append(kept, e)
	}
	searchLogsList = kept
}

// computeSearchRisk assigns threat scores and category based on query / clicked URL keywords.
func computeSearchRisk(text string) (int, string, string) {
	q := strings.ToLower(strings.TrimSpace(text))
	if q == "" {
		return 10, "LOW", "General Search"
	}
	critKeywords := []string{"bypass", "kill agent", "disable dlp", "exploit", "ssn", "password", "private key", "secret key", "leak", "payroll", "unauthorized"}
	for _, kw := range critKeywords {
		if strings.Contains(q, kw) {
			if strings.Contains(q, "bypass") || strings.Contains(q, "kill") || strings.Contains(q, "exploit") {
				return 92, "CRITICAL", "Exploit / Bypass"
			}
			return 90, "CRITICAL", "DLP / Data Leak"
		}
	}

	highKeywords := []string{"confidential", "internal roadmap", "acquisition", "financial model", "merger", "patent pending", "source code"}
	for _, kw := range highKeywords {
		if strings.Contains(q, kw) {
			return 78, "HIGH", "DLP / Data Leak"
		}
	}

	medKeywords := []string{"competitor", "pricing sheet", "salary", "bonus", "audit", "security test"}
	for _, kw := range medKeywords {
		if strings.Contains(q, kw) {
			return 45, "MEDIUM", "Reconnaissance"
		}
	}

	return 10, "LOW", "General Search"
}

func toLogstoreSearchLog(e *BrowserAISearchLogEntry) *logstore.BrowserAISearchLog {
	ts := e.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	return &logstore.BrowserAISearchLog{
		ID:             e.ID,
		Timestamp:      ts,
		Engine:         e.Engine,
		Browser:        e.Browser,
		IsIncognito:    e.IsIncognito,
		Query:          e.Query,
		ClickedURL:     e.ClickedURL,
		ClickedTitle:   e.ClickedTitle,
		URL:            e.URL,
		Host:           e.Host,
		ClientIP:       e.ClientIP,
		AgentHostname:  e.AgentHostname,
		AgentID:        e.AgentID,
		RiskScore:      e.RiskScore,
		PredictiveRisk: e.PredictiveRisk,
		RiskCategory:   e.RiskCategory,
		CreatedAt:      ts,
	}
}

func fromLogstoreSearchLog(e *logstore.BrowserAISearchLog) BrowserAISearchLogEntry {
	return BrowserAISearchLogEntry{
		ID:             e.ID,
		Timestamp:      e.Timestamp,
		Engine:         e.Engine,
		Browser:        e.Browser,
		IsIncognito:    e.IsIncognito,
		Query:          e.Query,
		ClickedURL:     e.ClickedURL,
		ClickedTitle:   e.ClickedTitle,
		URL:            e.URL,
		Host:           e.Host,
		ClientIP:       e.ClientIP,
		AgentHostname:  e.AgentHostname,
		AgentID:        e.AgentID,
		RiskScore:      e.RiskScore,
		PredictiveRisk: e.PredictiveRisk,
		RiskCategory:   e.RiskCategory,
		CreatedAt:      e.Timestamp.Format(time.RFC3339),
	}
}

// getSearchLogs serves search logs from PostgreSQL (pgAdmin) with fallback to in-memory buffer.
func (h *BrowserAIHandler) getSearchLogs(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)

	engineFilter := strings.ToLower(strings.TrimSpace(string(ctx.QueryArgs().Peek("engine"))))
	browserFilter := strings.ToLower(strings.TrimSpace(string(ctx.QueryArgs().Peek("browser"))))
	incognitoFilter := strings.ToLower(strings.TrimSpace(string(ctx.QueryArgs().Peek("is_incognito"))))
	searchFilter := strings.ToLower(strings.TrimSpace(string(ctx.QueryArgs().Peek("search"))))
	limitStr := string(ctx.QueryArgs().Peek("limit"))
	offsetStr := string(ctx.QueryArgs().Peek("offset"))

	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	// Auto-purge old search logs when admin enabled retention.
	if h.manager != nil {
		if cutoff := h.manager.ApplySearchLogAutoDelete(ctx); cutoff != nil {
			purgeInMemorySearchLogsBefore(*cutoff)
		}
	}

	// 1. Try PostgreSQL database first
	if h.manager != nil && h.manager.GetDB() != nil {
		dbLogs, total, incognitoCount, queriesCount, clicksCount, err := h.manager.GetSearchLogs(
			ctx, engineFilter, browserFilter, incognitoFilter, searchFilter, limit, offset,
		)
		if err == nil {
			paged := make([]BrowserAISearchLogEntry, 0, len(dbLogs))
			for _, l := range dbLogs {
				paged = append(paged, fromLogstoreSearchLog(&l))
			}
			SendJSON(ctx, map[string]any{
				"logs":            paged,
				"total":           total,
				"incognito_count": incognitoCount,
				"queries_count":   queriesCount,
				"clicks_count":    clicksCount,
				"limit":           limit,
				"offset":          offset,
			})
			return
		}
	}

	// 2. Fallback to in-memory buffer if DB is offline
	searchLogsMu.RLock()
	defer searchLogsMu.RUnlock()

	var filtered []BrowserAISearchLogEntry
	incognitoCount := 0
	queriesCount := 0
	clicksCount := 0

	for _, entry := range searchLogsList {
		if entry.IsIncognito {
			incognitoCount++
		}
		if entry.Query != "" {
			queriesCount++
		}
		if entry.ClickedURL != "" {
			clicksCount++
		}

		// Filter Engine
		if engineFilter != "" && !strings.Contains(strings.ToLower(entry.Engine), engineFilter) {
			continue
		}
		// Filter Browser
		if browserFilter != "" && !strings.Contains(strings.ToLower(entry.Browser), browserFilter) {
			continue
		}
		// Filter Incognito
		if incognitoFilter == "true" && !entry.IsIncognito {
			continue
		} else if incognitoFilter == "false" && entry.IsIncognito {
			continue
		}
		// Filter Search term
		if searchFilter != "" {
			qMatch := strings.Contains(strings.ToLower(entry.Query), searchFilter)
			cMatch := strings.Contains(strings.ToLower(entry.ClickedURL), searchFilter) || strings.Contains(strings.ToLower(entry.ClickedTitle), searchFilter)
			hMatch := strings.Contains(strings.ToLower(entry.AgentHostname), searchFilter) || strings.Contains(strings.ToLower(entry.ClientIP), searchFilter)
			if !qMatch && !cMatch && !hMatch {
				continue
			}
		}

		filtered = append(filtered, entry)
	}

	total := len(filtered)
	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	paged := filtered[start:end]
	if paged == nil {
		paged = []BrowserAISearchLogEntry{}
	}

	SendJSON(ctx, map[string]any{
		"logs":            paged,
		"total":           total,
		"incognito_count": incognitoCount,
		"queries_count":   queriesCount,
		"clicks_count":    clicksCount,
		"limit":           limit,
		"offset":          offset,
	})
}

// recordSearchLog records a search query or click in PostgreSQL and in-memory buffer.
func (h *BrowserAIHandler) recordSearchLog(ctx *fasthttp.RequestCtx) {
	if !h.verifyGuardSecurity(ctx) {
		return
	}
	h.ensureDB(ctx)

	var body BrowserAISearchLogEntry
	if err := sonic.Unmarshal(ctx.PostBody(), &body); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if body.ID == "" {
		body.ID = uuid.New().String()
	}
	if body.Timestamp.IsZero() {
		body.Timestamp = time.Now()
	}
	body.CreatedAt = body.Timestamp.Format(time.RFC3339)

	if h.manager != nil {
		if cutoff := h.manager.ApplySearchLogAutoDelete(ctx); cutoff != nil {
			purgeInMemorySearchLogsBefore(*cutoff)
		}
	}

	if body.Engine == "" {
		body.Engine = "Google"
	}
	if body.Browser == "" {
		body.Browser = "Unknown"
	}

	// Predictive risk from typed query and/or clicked result URL (any browser).
	riskText := strings.TrimSpace(body.Query + " " + body.ClickedURL + " " + body.ClickedTitle)
	if body.RiskScore == 0 && riskText != "" {
		score, risk, cat := computeSearchRisk(riskText)
		body.RiskScore = score
		body.PredictiveRisk = risk
		body.RiskCategory = cat
	} else if body.PredictiveRisk == "" {
		body.PredictiveRisk = "LOW"
		body.RiskCategory = "General Search"
	}

	// 1. Persist to PostgreSQL (pgAdmin: browser_ai_search_logs)
	if h.manager != nil && h.manager.GetDB() != nil {
		dbEntry := toLogstoreSearchLog(&body)
		if err := h.manager.RecordSearchLog(ctx, dbEntry); err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, "Failed to save search log: "+err.Error())
			return
		}
	}

	// 2. Also keep in-memory buffer (fallback when DB unavailable)
	searchLogsMu.Lock()
	searchLogsList = append([]BrowserAISearchLogEntry{body}, searchLogsList...)
	if len(searchLogsList) > 500 {
		searchLogsList = searchLogsList[:500]
	}
	searchLogsMu.Unlock()

	SendJSON(ctx, map[string]any{
		"status": "success",
		"log":    body,
	})
}

// deleteSearchLogs clears search logs in PostgreSQL and in-memory buffer.
// Query params (optional):
//   period=1d|7d|30d|all  — delete logs from the last N period (or all)
//   date=YYYY-MM-DD       — delete logs for that calendar day (local server day)
// With no params, clears all (legacy).
func (h *BrowserAIHandler) deleteSearchLogs(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)

	period := strings.ToLower(strings.TrimSpace(string(ctx.QueryArgs().Peek("period"))))
	dateStr := strings.TrimSpace(string(ctx.QueryArgs().Peek("date")))

	var since, until *time.Time
	now := time.Now()

	switch {
	case dateStr != "":
		day, err := time.ParseInLocation("2006-01-02", dateStr, now.Location())
		if err != nil {
			SendError(ctx, fasthttp.StatusBadRequest, "Invalid date (use YYYY-MM-DD)")
			return
		}
		start := day
		end := day.Add(24 * time.Hour)
		since, until = &start, &end
	case period == "1d" || period == "day":
		start := now.Add(-24 * time.Hour)
		since, until = &start, &now
	case period == "7d" || period == "week" || period == "weekly":
		start := now.Add(-7 * 24 * time.Hour)
		since, until = &start, &now
	case period == "30d" || period == "month" || period == "monthly":
		start := now.Add(-30 * 24 * time.Hour)
		since, until = &start, &now
	case period == "" || period == "all":
		since, until = nil, nil
	default:
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid period (use 1d, 7d, 30d, or all)")
		return
	}

	if h.manager != nil && h.manager.GetDB() != nil {
		_ = h.manager.ClearSearchLogsInRange(ctx, since, until)
	}

	searchLogsMu.Lock()
	if since == nil && until == nil {
		searchLogsList = []BrowserAISearchLogEntry{}
	} else {
		kept := searchLogsList[:0]
		for _, e := range searchLogsList {
			ts := e.Timestamp
			if ts.IsZero() {
				if parsed, err := time.Parse(time.RFC3339, e.CreatedAt); err == nil {
					ts = parsed
				}
			}
			if since != nil && ts.Before(*since) {
				kept = append(kept, e)
				continue
			}
			if until != nil && !ts.Before(*until) {
				kept = append(kept, e)
				continue
			}
			// in range → drop
		}
		searchLogsList = kept
	}
	searchLogsMu.Unlock()

	msg := "Search logs cleared"
	if dateStr != "" {
		msg = "Search logs deleted for " + dateStr
	} else if period == "1d" || period == "day" {
		msg = "Search logs deleted for the last 1 day"
	} else if period == "7d" || period == "week" || period == "weekly" {
		msg = "Search logs deleted for the last 7 days"
	} else if period == "30d" || period == "month" || period == "monthly" {
		msg = "Search logs deleted for the last 30 days"
	}

	SendJSON(ctx, map[string]any{
		"status":  "success",
		"message": msg,
	})
}
