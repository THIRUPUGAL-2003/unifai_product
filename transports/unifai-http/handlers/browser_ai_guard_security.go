package handlers

import (
	"crypto/sha256"
	"crypto/subtle"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
)

// In-memory token-bucket rate limiter for public Browser AI endpoints
// to protect against DDoS, automated prompt flooding, and wallet drainage.
const (
	browserAIRateLimitWindow  = 1 * time.Minute
	browserAIMaxRequestsPerIP = 120 // 120 requests/min per IP (~2 req/sec, plenty for normal employees)
	// Requests carrying the valid Guard secret: many laptops can share one office NAT IP,
	// and each Guard sends intercepts + heartbeats + uninstall checks.
	browserAIMaxKeyedRequestsPerIP = 1200
	// Guard policy sync (targets/rules/controls/fleet + proxy bundle) polls often per laptop;
	// kept in its own bucket so it can never starve prompt intercepts.
	guardSyncMaxRequestsPerIP = 6000
	browserAIMaxPromptChars   = 300000 // 300k chars max to prevent memory exhaustion
)

type ipRateTracker struct {
	mu        sync.Mutex
	counts    map[string]*ipWindowRecord
	lastPurge time.Time
}

type ipWindowRecord struct {
	windowStart time.Time
	count       int
}

func newIPRateTracker() *ipRateTracker {
	return &ipRateTracker{counts: make(map[string]*ipWindowRecord), lastPurge: time.Now()}
}

var (
	browserAIRateTracker      = newIPRateTracker()
	browserAIKeyedRateTracker = newIPRateTracker()
	guardSyncRateTracker      = newIPRateTracker()
)

func (t *ipRateTracker) exceeded(ip string, limit int) bool {
	if ip == "" {
		return false
	}
	now := time.Now()

	t.mu.Lock()
	defer t.mu.Unlock()

	// Periodic cleanup of expired records every 5 minutes
	if now.Sub(t.lastPurge) > 5*time.Minute {
		for k, rec := range t.counts {
			if now.Sub(rec.windowStart) > browserAIRateLimitWindow {
				delete(t.counts, k)
			}
		}
		t.lastPurge = now
	}

	rec, exists := t.counts[ip]
	if !exists || now.Sub(rec.windowStart) > browserAIRateLimitWindow {
		t.counts[ip] = &ipWindowRecord{windowStart: now, count: 1}
		return false
	}

	rec.count++
	return rec.count > limit
}

// isBrowserAIRateLimited checks if the client IP has exceeded the allowed request threshold.
func isBrowserAIRateLimited(ip string) bool {
	return browserAIRateTracker.exceeded(ip, browserAIMaxRequestsPerIP)
}

func isGuardSyncRateLimited(ip string) bool {
	return guardSyncRateTracker.exceeded(ip, guardSyncMaxRequestsPerIP)
}

func configuredGuardSecret() string {
	s := strings.TrimSpace(os.Getenv("UNIFAI_GUARD_SECRET"))
	if s == "" {
		s = strings.TrimSpace(os.Getenv("GUARD_SECRET_KEY"))
	}
	return s
}

func guardSecretRequired() bool {
	v := strings.TrimSpace(os.Getenv("UNIFAI_GUARD_REQUIRE_SECRET"))
	if v == "0" || strings.EqualFold(v, "false") || strings.EqualFold(v, "no") || strings.EqualFold(v, "off") {
		return false
	}
	// Fail-closed by default: Guard agent shared secret is required unless explicitly disabled.
	return true
}

func guardSecretMatches(provided, configured string) bool {
	if configured == "" {
		return false
	}
	// Constant-time compare via SHA-256 digests (handles unequal lengths safely).
	a := sha256.Sum256([]byte(provided))
	b := sha256.Sum256([]byte(configured))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

func extractGuardKey(ctx *fasthttp.RequestCtx) string {
	providedKey := strings.TrimSpace(string(ctx.Request.Header.Peek("X-UnifAI-Guard-Key")))
	if providedKey != "" {
		return providedKey
	}
	auth := string(ctx.Request.Header.Peek("Authorization"))
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return ""
}

// verifyGuardSecurity checks shared-secret auth (when required) and IP rate limiting.
// Returns true if the request is permitted; false after writing an error response.
func (h *BrowserAIHandler) verifyGuardSecurity(ctx *fasthttp.RequestCtx) bool {
	configuredSecret := configuredGuardSecret()
	if guardSecretRequired() {
		if configuredSecret == "" {
			SendError(ctx, fasthttp.StatusServiceUnavailable, "Guard agent secret not configured — set UNIFAI_GUARD_SECRET in .env")
			return false
		}
		if !guardSecretMatches(extractGuardKey(ctx), configuredSecret) {
			SendError(ctx, fasthttp.StatusUnauthorized, "Unauthorized: Invalid or missing Guard agent secret key")
			return false
		}
	}

	clientIP := clientIPAddress(ctx)
	keyed := configuredSecret != "" && guardSecretMatches(extractGuardKey(ctx), configuredSecret)
	limited := false
	if keyed {
		limited = browserAIKeyedRateTracker.exceeded(clientIP, browserAIMaxKeyedRequestsPerIP)
	} else {
		limited = isBrowserAIRateLimited(clientIP)
	}
	if limited {
		SendError(ctx, fasthttp.StatusTooManyRequests, "Too many requests: Browser AI rate limit exceeded. Please wait a minute.")
		return false
	}

	return true
}
