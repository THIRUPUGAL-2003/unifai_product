package handlers

import (
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/raksha/raksha/framework/configstore"
	"github.com/raksha/raksha/framework/configstore/tables"
	"github.com/raksha/raksha/framework/encrypt"
	"github.com/valyala/fasthttp"
)

// allowOpenAuthWhenDisabled permits unauthenticated "open admin" only from
// loopback, or when ALLOW_OPEN_AUTH=1 is explicitly set (local bootstrap).
func allowOpenAuthWhenDisabled(ctx *fasthttp.RequestCtx) bool {
	v := strings.TrimSpace(os.Getenv("ALLOW_OPEN_AUTH"))
	if v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes") {
		return true
	}
	return isLoopbackIP(clientIPAddress(ctx))
}

func isLoopbackIP(ipStr string) bool {
	ipStr = strings.TrimSpace(ipStr)
	if ipStr == "" {
		return false
	}
	// Strip port if present (RemoteIP sometimes formats oddly).
	if host, _, err := net.SplitHostPort(ipStr); err == nil {
		ipStr = host
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return ipStr == "localhost" || ipStr == "::1"
	}
	return ip.IsLoopback()
}

// cookieShouldBeSecure sets the Secure flag only for real TLS or trusted proxies.
func cookieShouldBeSecure(ctx *fasthttp.RequestCtx) bool {
	if ctx.IsTLS() {
		return true
	}
	if !directPeerIsTrustedProxy(ctx) {
		return false
	}
	return strings.EqualFold(string(ctx.Request.Header.Peek("X-Forwarded-Proto")), "https")
}

// --- Persisted IP / forgot rate limits (survive restart; shared via DB) ---

const (
	maxLoginAttemptsPerIP = 30
	loginIPWindow         = 15 * time.Minute
)

func loginIPLockoutKey(ip string) string {
	return "ip:" + strings.TrimSpace(ip)
}

func forgotUsernameKey(kind, value string) string {
	return "fuser:" + kind + ":" + strings.ToLower(strings.TrimSpace(value))
}

func forgotPasswordIPKey(ip string) string {
	return "fpw:ip:" + strings.TrimSpace(ip)
}

func forgotPasswordTargetKey(target string) string {
	return "fpw:tgt:" + strings.ToLower(strings.TrimSpace(target))
}

func registrationIPKey(ip string) string {
	return "reg:ip:" + strings.TrimSpace(ip)
}

func wsNonceLockoutKey(nonce string) string {
	return "wsnonce:" + strings.TrimSpace(nonce)
}

// checkLoginIPRateLimit reports whether this IP is currently locked (read-only).
// Does not increment counters — successes must not push shared NAT users into a DoS.
func checkLoginIPRateLimit(store configstore.ConfigStore, ctx *fasthttp.RequestCtx) (blocked bool, retryAfter time.Duration) {
	ip := clientIPAddress(ctx)
	if ip == "" || store == nil {
		return false, 0
	}
	key := loginIPLockoutKey(ip)
	now := time.Now()
	row, _ := store.GetLoginLockout(ctx, key)
	if row == nil || row.LockedUntil == nil {
		return false, 0
	}
	if !now.Before(*row.LockedUntil) {
		return false, 0
	}
	// Locked window after hitting the cap (FailedCount cleared on lock).
	if row.FailedCount == 0 {
		return true, time.Until(*row.LockedUntil)
	}
	return false, 0
}

// recordLoginIPFailure increments the per-IP failed-login counter (DB-backed).
func recordLoginIPFailure(store configstore.ConfigStore, ctx *fasthttp.RequestCtx) (blocked bool, retryAfter time.Duration) {
	ip := clientIPAddress(ctx)
	if ip == "" || store == nil {
		return false, 0
	}
	key := loginIPLockoutKey(ip)
	now := time.Now()
	row, _ := store.GetLoginLockout(ctx, key)
	if row == nil {
		row = &tables.TableLoginLockout{UsernameKey: key}
	}
	// Already in lockout window after cap.
	if row.LockedUntil != nil && now.Before(*row.LockedUntil) && row.FailedCount == 0 {
		return true, time.Until(*row.LockedUntil)
	}
	// Fresh window after prior lock/window expired.
	if row.LockedUntil != nil && !now.Before(*row.LockedUntil) {
		row.FailedCount = 0
		row.LockedUntil = nil
	}
	if row.LockedUntil == nil {
		until := now.Add(loginIPWindow)
		row.LockedUntil = &until
	}
	row.FailedCount++
	row.LastFailedAt = &now
	if row.FailedCount >= maxLoginAttemptsPerIP {
		until := now.Add(loginIPWindow)
		row.LockedUntil = &until
		row.FailedCount = 0 // marker: locked (count 0 + LockedUntil future)
		_ = store.UpsertLoginLockout(ctx, row)
		return true, loginIPWindow
	}
	_ = store.UpsertLoginLockout(ctx, row)
	return false, 0
}

// clearLoginIPFailures resets the per-IP counter after a successful login.
func clearLoginIPFailures(store configstore.ConfigStore, ctx *fasthttp.RequestCtx) {
	ip := clientIPAddress(ctx)
	if ip == "" || store == nil {
		return
	}
	key := loginIPLockoutKey(ip)
	_ = store.UpsertLoginLockout(ctx, &tables.TableLoginLockout{
		UsernameKey:  key,
		FailedCount:  0,
		LastFailedAt: nil,
		LockedUntil:  nil,
		UpdatedAt:    time.Now(),
	})
}

// checkForgotUsernameCooldown returns true when still cooling down (DB-backed).
// When returning false it records the attempt so the next call within cooldown is blocked.
func checkForgotUsernameCooldown(store configstore.ConfigStore, ctx *fasthttp.RequestCtx, key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" || store == nil {
		return false
	}
	dbKey := forgotUsernameKey("id", key)
	now := time.Now()
	row, _ := store.GetLoginLockout(ctx, dbKey)
	if row != nil && row.LockedUntil != nil && now.Before(*row.LockedUntil) {
		return true
	}
	until := now.Add(forgotPasswordCooldown)
	_ = store.UpsertLoginLockout(ctx, &tables.TableLoginLockout{
		UsernameKey:  dbKey,
		FailedCount:  1,
		LastFailedAt: &now,
		LockedUntil:  &until,
		UpdatedAt:    now,
	})
	return false
}

// consumeForgotPasswordQuota enforces per-IP and per-target forgot-password limits.
// Returns true when the request should be silently throttled.
func consumeForgotPasswordQuota(store configstore.ConfigStore, ctx *fasthttp.RequestCtx, ip, target string) bool {
	if store == nil {
		return false
	}
	now := time.Now()
	ip = strings.TrimSpace(ip)
	target = strings.ToLower(strings.TrimSpace(target))

	var ipRow *tables.TableLoginLockout
	if ip != "" {
		ipRow, _ = store.GetLoginLockout(ctx, forgotPasswordIPKey(ip))
		if ipRow != nil && ipRow.LockedUntil != nil && now.Before(*ipRow.LockedUntil) {
			if ipRow.FailedCount >= maxForgotPasswordPerHourIP {
				return true
			}
		} else {
			ipRow = &tables.TableLoginLockout{UsernameKey: forgotPasswordIPKey(ip)}
		}
	}

	var tgtRow *tables.TableLoginLockout
	if target != "" {
		tgtRow, _ = store.GetLoginLockout(ctx, forgotPasswordTargetKey(target))
		if tgtRow != nil {
			if tgtRow.LastFailedAt != nil && now.Sub(*tgtRow.LastFailedAt) < forgotPasswordCooldown {
				return true
			}
			if tgtRow.LockedUntil != nil && now.Before(*tgtRow.LockedUntil) && tgtRow.FailedCount >= maxForgotPasswordPerHourTarget {
				return true
			}
			if tgtRow.LockedUntil == nil || !now.Before(*tgtRow.LockedUntil) {
				tgtRow = &tables.TableLoginLockout{UsernameKey: forgotPasswordTargetKey(target)}
			}
		} else {
			tgtRow = &tables.TableLoginLockout{UsernameKey: forgotPasswordTargetKey(target)}
		}
	}

	if ipRow != nil {
		if ipRow.LockedUntil == nil || !now.Before(*ipRow.LockedUntil) {
			until := now.Add(time.Hour)
			ipRow.LockedUntil = &until
			ipRow.FailedCount = 0
		}
		ipRow.FailedCount++
		ipRow.LastFailedAt = &now
		_ = store.UpsertLoginLockout(ctx, ipRow)
	}
	if tgtRow != nil {
		if tgtRow.LockedUntil == nil {
			until := now.Add(time.Hour)
			tgtRow.LockedUntil = &until
		}
		tgtRow.FailedCount++
		tgtRow.LastFailedAt = &now
		_ = store.UpsertLoginLockout(ctx, tgtRow)
	}
	return false
}

// consumeRegistrationQuota blocks excessive public signups from one IP (DB-backed).
func consumeRegistrationQuota(store configstore.ConfigStore, ctx *fasthttp.RequestCtx) bool {
	if store == nil {
		return false
	}
	ip := clientIPAddress(ctx)
	if ip == "" {
		return false
	}
	now := time.Now()
	key := registrationIPKey(ip)
	row, _ := store.GetLoginLockout(ctx, key)
	if row != nil && row.LockedUntil != nil && now.Before(*row.LockedUntil) {
		if row.FailedCount >= maxRegistrationsPerHour {
			return true
		}
	} else {
		row = &tables.TableLoginLockout{UsernameKey: key}
	}
	if row.LockedUntil == nil || !now.Before(*row.LockedUntil) {
		until := now.Add(time.Hour)
		row.LockedUntil = &until
		row.FailedCount = 0
	}
	row.FailedCount++
	row.LastFailedAt = &now
	_ = store.UpsertLoginLockout(ctx, row)
	return false
}

// padPasswordCompare burns a bcrypt compare so unknown-user login timing
// matches a real password mismatch (mitigates username enumeration).
var (
	padHashOnce sync.Once
	padHash     string
)

func padPasswordCompare(password string) {
	padHashOnce.Do(func() {
		h, err := encrypt.Hash("raksha-login-timing-pad-v1")
		if err != nil {
			// Fixed bcrypt cost-10 hash of a known string (never a real password).
			padHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
			return
		}
		padHash = h
	})
	_, _ = encrypt.CompareHash(padHash, password)
}
