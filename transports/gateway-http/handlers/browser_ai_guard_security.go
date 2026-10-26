package handlers

import (
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
	secret := strings.TrimSpace(os.Getenv("GATEWAY_GUARD_SECRET"))
	if len([]byte(secret)) < 32 {
		return ""
	}
	return secret
}

func guardSecretRequired() bool {
	return true
}

func guardSecretMatches(provided, configured string) bool {
	if provided == "" || configured == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(configured)) == 1
}

const guardKeyHeader = "X-Gateway-Guard-Key"

// guardKeyHeaderPrefix returns "X-<Brand>-" for a request authenticated with an
// X-<Brand>-Guard-Key header. Older Guard builds use a different brand in the header
// name; accepting any brand lets them authenticate and auto-update to the current build.
func guardKeyHeaderPrefix(ctx *fasthttp.RequestCtx) (prefix, key string) {
	if v := strings.TrimSpace(string(ctx.Request.Header.Peek(guardKeyHeader))); v != "" {
		return "X-Gateway-", v
	}
	for name, value := range ctx.Request.Header.All() {
		n := string(name)
		if len(n) <= len("X--Guard-Key") || !strings.EqualFold(n[:2], "X-") || !strings.EqualFold(n[len(n)-len("-Guard-Key"):], "-Guard-Key") {
			continue
		}
		brand := n[2 : len(n)-len("-Guard-Key")]
		if strings.ContainsAny(brand, "-_ ") {
			continue
		}
		if v := strings.TrimSpace(string(value)); v != "" {
			return n[:2+len(brand)+1], v
		}
	}
	return "", ""
}

// setGuardVersionHeaders advertises the release version under the current header name and,
// for an older Guard, under the header name that build reads.
func setGuardVersionHeaders(ctx *fasthttp.RequestCtx, version string) {
	ctx.Response.Header.Set("X-Gateway-Guard-Version", version)
	if prefix, _ := guardKeyHeaderPrefix(ctx); prefix != "" && !strings.EqualFold(prefix, "X-Gateway-") {
		ctx.Response.Header.Set(prefix+"Guard-Version", version)
	}
}

func extractGuardKey(ctx *fasthttp.RequestCtx) string {
	if _, key := guardKeyHeaderPrefix(ctx); key != "" {
		return key
	}
	auth := string(ctx.Request.Header.Peek("Authorization"))
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return ""
}

// verifyGuardSecurity checks the per-IP intercept rate limit. Agent routes are
// authenticated by the middleware before they reach this handler.
func (h *BrowserAIHandler) verifyGuardSecurity(ctx *fasthttp.RequestCtx) bool {
	clientIP := clientIPAddress(ctx)
	if isBrowserAIRateLimited(clientIP) {
		SendError(ctx, fasthttp.StatusTooManyRequests, "Too many requests: Browser AI rate limit exceeded. Please wait a minute.")
		return false
	}
	return true
}
