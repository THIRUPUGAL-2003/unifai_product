package lib

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/unifai/unifai/core/schemas"
)

const ClientSafeInternalErrorMessage = "internal server error"

var mcpHTTPStatusRe = regexp.MustCompile(`(?i)(?:status|http)[^\d]{0,12}(\d{3})`)

// SanitizeUnifAIErrorForClient returns a copy safe to serialize to API clients.
// Internal errors can contain stack traces or database details; keep those in logs only.
func SanitizeUnifAIErrorForClient(err *schemas.UnifAIError) *schemas.UnifAIError {
	if err == nil {
		return nil
	}

	sanitized := *err
	if err.Error != nil {
		errorField := *err.Error
		if shouldHideErrorDetails(err, err.Error) {
			errorField.Message = ClientSafeInternalErrorMessage
			errorField.Error = nil
			errorField.Param = nil
		}
		sanitized.Error = &errorField
	}

	return &sanitized
}

// ClientSafeMCPConnectMessage turns upstream MCP transport errors into a short
// client-safe string. Cloudflare/HTML bodies (common on wrong URLs / 429s) must
// never be forwarded into API toasts.
func ClientSafeMCPConnectMessage(prefix string, err error) string {
	if err == nil {
		if prefix == "" {
			return "MCP connection failed"
		}
		return prefix
	}
	detail := sanitizeMCPUpstreamDetail(err.Error())
	if prefix == "" {
		return detail
	}
	// Avoid "Failed to connect MCP client: Could not connect to MCP client …"
	if strings.EqualFold(strings.TrimSpace(prefix), "Failed to connect MCP client") ||
		strings.EqualFold(strings.TrimSpace(prefix), "Failed to reconnect MCP client") ||
		strings.EqualFold(strings.TrimSpace(prefix), "Failed to register MCP client") {
		if strings.HasPrefix(strings.ToLower(detail), "could not connect") ||
			strings.HasPrefix(strings.ToLower(detail), "could not reconnect") {
			return detail
		}
	}
	return prefix + ": " + detail
}

func sanitizeMCPUpstreamDetail(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "unknown MCP connection error"
	}

	clientName := extractMCPClientName(msg)
	root := unwrapMCPConnectNoise(msg)

	status, hasStatus := extractEmbeddedHTTPStatus(root)
	htmlish := looksLikeHTMLOrStatusPage(root) || looksLikeHTMLOrStatusPage(msg)
	oversized := utf8.RuneCountInString(root) > 400 || utf8.RuneCountInString(msg) > 400

	if hasStatus && (htmlish || oversized) {
		switch status {
		case 401, 403:
			return withMCPClientName(clientName, fmt.Sprintf("upstream returned HTTP %d (unauthorized) — switch Authentication to Headers or OAuth and add a valid API key / token", status))
		case 404:
			return withMCPClientName(clientName, "upstream returned HTTP 404 — this URL does not look like a valid MCP endpoint")
		case 429:
			return withMCPClientName(clientName, "upstream returned HTTP 429 (rate limited or blocked) — this URL is not a usable MCP endpoint, or authentication is required")
		default:
			if status >= 400 {
				return withMCPClientName(clientName, fmt.Sprintf("upstream returned HTTP %d — check the MCP server URL and authentication", status))
			}
		}
	}

	if htmlish || oversized {
		return withMCPClientName(clientName, "upstream returned a non-MCP response (HTML or oversized body) — use the real MCP endpoint URL from the provider docs, not a website homepage")
	}

	if hasStatus && status >= 400 {
		switch status {
		case 401, 403:
			return withMCPClientName(clientName, fmt.Sprintf("upstream returned HTTP %d (unauthorized) — switch Authentication to Headers or OAuth and add a valid API key / token", status))
		case 429:
			return withMCPClientName(clientName, "upstream returned HTTP 429 (rate limited or blocked)")
		default:
			return withMCPClientName(clientName, fmt.Sprintf("upstream returned HTTP %d", status))
		}
	}

	if mapped := mapMCPRootCause(root); mapped != "" {
		return withMCPClientName(clientName, mapped)
	}

	return withMCPClientName(clientName, truncateRunes(root, 220))
}

var (
	mcpClientNameQuotedRe = regexp.MustCompile(`(?i)failed to connect MCP client\s+'([^']+)'`)
	mcpClientNamePlainRe  = regexp.MustCompile(`(?i)failed to connect MCP client\s+([A-Za-z0-9_.-]+)`)
	mcpConnectNoiseRe     = regexp.MustCompile(`(?i)(?:failed to connect MCP client(?:\s+'[^']+'|\s+[A-Za-z0-9_.-]+)?\s*:\s*)+`)
	mcpTransportRetriesRe = regexp.MustCompile(`(?i)failed to start MCP client transport after \d+ retries:\s*`)
)

func extractMCPClientName(msg string) string {
	if m := mcpClientNameQuotedRe.FindStringSubmatch(msg); len(m) == 2 {
		return m[1]
	}
	if m := mcpClientNamePlainRe.FindStringSubmatch(msg); len(m) == 2 {
		return m[1]
	}
	return ""
}

func unwrapMCPConnectNoise(msg string) string {
	msg = strings.TrimSpace(msg)
	// Peel nested "failed to connect MCP client …:" wrappers.
	for i := 0; i < 8; i++ {
		next := strings.TrimSpace(mcpConnectNoiseRe.ReplaceAllString(msg, ""))
		next = strings.TrimSpace(mcpTransportRetriesRe.ReplaceAllString(next, ""))
		if next == msg {
			break
		}
		msg = next
	}
	return strings.TrimSpace(msg)
}

func mapMCPRootCause(root string) string {
	lower := strings.ToLower(root)
	switch {
	case strings.Contains(lower, "waiting for endpoint"),
		strings.Contains(lower, "timeout waiting"),
		(strings.Contains(lower, "timeout") && strings.Contains(lower, "endpoint")):
		return "endpoint timed out — check the MCP URL is reachable, the server is running, and auth (if required) is configured"
	case strings.Contains(lower, "context deadline exceeded"),
		strings.Contains(lower, "deadline exceeded"):
		return "connection timed out — check the MCP URL, network, and authentication"
	case strings.Contains(lower, "connection refused"):
		return "connection refused — nothing is listening at that host/port"
	case strings.Contains(lower, "no such host"),
		strings.Contains(lower, "name resolution"):
		return "DNS lookup failed — check the MCP hostname"
	case strings.Contains(lower, "certificate"),
		strings.Contains(lower, "x509"):
		return "TLS/certificate error — check HTTPS settings for this MCP endpoint"
	case strings.Contains(lower, "executable file not found"),
		strings.Contains(lower, "command not found"),
		strings.Contains(lower, "no such file"):
		return "stdio command not found — install the MCP binary/npx package on the UnifAI host"
	case strings.Contains(lower, "401"), strings.Contains(lower, "unauthorized"):
		return "unauthorized — switch Authentication to Headers or OAuth and add a valid API key / token"
	case strings.Contains(lower, "403"), strings.Contains(lower, "forbidden"):
		return "forbidden — check API key permissions or OAuth scopes"
	default:
		return ""
	}
}

func withMCPClientName(name, detail string) string {
	detail = strings.TrimSpace(detail)
	if name == "" {
		return detail
	}
	return fmt.Sprintf("Could not connect to MCP client %q: %s", name, detail)
}

func extractEmbeddedHTTPStatus(msg string) (int, bool) {
	// Prefer the mcp-go phrasing: "request failed with status 429: ..."
	lower := strings.ToLower(msg)
	if idx := strings.Index(lower, "status "); idx >= 0 {
		rest := msg[idx+len("status "):]
		var n int
		for i, r := range rest {
			if r < '0' || r > '9' {
				if i == 0 {
					break
				}
				n, _ = strconv.Atoi(rest[:i])
				if n >= 100 && n <= 599 {
					return n, true
				}
				break
			}
			if i == len(rest)-1 {
				n, _ = strconv.Atoi(rest)
				if n >= 100 && n <= 599 {
					return n, true
				}
			}
		}
	}
	m := mcpHTTPStatusRe.FindStringSubmatch(msg)
	if len(m) == 2 {
		n, err := strconv.Atoi(m[1])
		if err == nil && n >= 100 && n <= 599 {
			return n, true
		}
	}
	return 0, false
}

func looksLikeHTMLOrStatusPage(msg string) bool {
	lower := strings.ToLower(msg)
	markers := []string{
		"<!doctype",
		"<html",
		"<head",
		"<body",
		"<script",
		"cloudflare",
		"cf-ray",
		"_status_page_config_",
		"429_title",
		"too many requests",
		"error code 429",
		"window.__",
		"\u003chtml",
		"\\u003c",
	}
	for _, m := range markers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:max])) + "…"
}

func shouldHideErrorDetails(_ *schemas.UnifAIError, field *schemas.ErrorField) bool {
	message := field.Message
	if field.Error != nil {
		message += " " + field.Error.Error()
	}

	return containsStackTrace(message) || containsSQLDetails(message) || looksLikeHTMLOrStatusPage(message)
}

func containsStackTrace(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "stack trace") ||
		strings.Contains(lower, "traceback (most recent call last)") ||
		strings.Contains(lower, "runtime/debug.stack") ||
		strings.Contains(lower, "goroutine ") ||
		strings.Contains(lower, "panic:") ||
		strings.Contains(lower, ".go:")
}

func containsSQLDetails(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "sqlstate") ||
		strings.Contains(lower, "sql:") ||
		strings.Contains(lower, "pq:") ||
		strings.Contains(lower, "pgx:") ||
		strings.Contains(lower, "duplicate key value violates") ||
		strings.Contains(lower, "violates foreign key constraint") ||
		strings.Contains(lower, "violates unique constraint") ||
		strings.Contains(lower, "syntax error at or near") ||
		strings.Contains(lower, "relation does not exist") ||
		strings.Contains(lower, "database/sql")
}
