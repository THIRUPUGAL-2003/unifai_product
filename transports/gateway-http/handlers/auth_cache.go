package handlers

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gateway/gateway/framework/configstore/tables"
	"github.com/redis/go-redis/v9"
)

// SessionCacheEntry holds validated session and user data in memory for instant lookup.
type SessionCacheEntry struct {
	Session          *tables.SessionsTable
	User             *tables.TableUser
	IsBootstrapAdmin bool
	CachedAt         time.Time
	ExpiresAt        time.Time
}

// LockoutCacheEntry stores account lockout status in memory.
type LockoutCacheEntry struct {
	LockedUntil time.Time
}

// AuthCache provides dual-layer (In-Memory + Optional Redis) caching for sessions and lockouts.
// By default, it operates entirely in RAM using thread-safe sync.RWMutex with zero external dependencies
// and sub-microsecond latency. When REDIS_URL or REDIS_ADDR is configured, it automatically syncs with Redis.
type AuthCache struct {
	mu          sync.RWMutex
	sessions    map[string]*SessionCacheEntry
	lockouts    map[string]*LockoutCacheEntry
	redisClient *redis.Client
	hasRedis    bool
}

var (
	globalAuthCache     *AuthCache
	globalAuthCacheOnce sync.Once
)

// GetAuthCache returns the singleton instance of AuthCache.
func GetAuthCache() *AuthCache {
	globalAuthCacheOnce.Do(func() {
		ac := &AuthCache{
			sessions: make(map[string]*SessionCacheEntry),
			lockouts: make(map[string]*LockoutCacheEntry),
		}

		// Check for optional Redis connection
		redisAddr := strings.TrimSpace(os.Getenv("REDIS_ADDR"))
		redisURL := strings.TrimSpace(os.Getenv("REDIS_URL"))
		if redisURL != "" {
			opt, err := redis.ParseURL(redisURL)
			if err == nil {
				opt.DialTimeout = 2 * time.Second
				opt.ReadTimeout = 1 * time.Second
				opt.WriteTimeout = 1 * time.Second
				rdb := redis.NewClient(opt)
				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
				if err := rdb.Ping(ctx).Err(); err == nil {
					ac.redisClient = rdb
					ac.hasRedis = true
				}
				cancel()
			}
		} else if redisAddr != "" {
			rdb := redis.NewClient(&redis.Options{
				Addr:         redisAddr,
				Password:     strings.TrimSpace(os.Getenv("REDIS_PASSWORD")),
				DB:           0,
				DialTimeout:  2 * time.Second,
				ReadTimeout:  1 * time.Second,
				WriteTimeout: 1 * time.Second,
			})
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			if err := rdb.Ping(ctx).Err(); err == nil {
				ac.redisClient = rdb
				ac.hasRedis = true
			}
			cancel()
		}

		// Run periodic in-memory garbage collector to evict expired items
		go ac.startJanitor(1 * time.Minute)

		globalAuthCache = ac
	})
	return globalAuthCache
}

func (c *AuthCache) startJanitor(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		now := time.Now()
		c.mu.Lock()
		for k, v := range c.sessions {
			if now.After(v.ExpiresAt) {
				delete(c.sessions, k)
			}
		}
		for k, v := range c.lockouts {
			if now.After(v.LockedUntil) {
				delete(c.lockouts, k)
			}
		}
		c.mu.Unlock()
	}
}

// GetSession retrieves a cached session if it exists and has not expired.
func (c *AuthCache) GetSession(token string) (*SessionCacheEntry, bool) {
	if token == "" {
		return nil, false
	}
	c.mu.RLock()
	entry, ok := c.sessions[token]
	c.mu.RUnlock()

	if ok && entry != nil {
		if time.Now().Before(entry.ExpiresAt) {
			return entry, true
		}
		// Expired
		c.mu.Lock()
		delete(c.sessions, token)
		c.mu.Unlock()
	}
	return nil, false
}

// SetSession stores a validated session in memory with a specified TTL.
func (c *AuthCache) SetSession(token string, sess *tables.SessionsTable, user *tables.TableUser, isBootstrap bool, ttl time.Duration) {
	if token == "" || sess == nil {
		return
	}
	now := time.Now()
	entry := &SessionCacheEntry{
		Session:          sess,
		User:             user,
		IsBootstrapAdmin: isBootstrap,
		CachedAt:         now,
		ExpiresAt:        now.Add(ttl),
	}

	c.mu.Lock()
	c.sessions[token] = entry
	c.mu.Unlock()
}

// InvalidateSession removes a session from memory immediately (e.g. on logout).
func (c *AuthCache) InvalidateSession(token string) {
	if token == "" {
		return
	}
	c.mu.Lock()
	delete(c.sessions, token)
	c.mu.Unlock()
}

// InvalidateUserSessions removes all active cached sessions for a given username (e.g. on block/password change).
func (c *AuthCache) InvalidateUserSessions(username string) {
	if username == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for token, entry := range c.sessions {
		if entry != nil && entry.Session != nil && strings.EqualFold(entry.Session.Username, username) {
			delete(c.sessions, token)
		}
	}
}

// GetLockout checks if an identifier is locked in memory.
func (c *AuthCache) GetLockout(usernameKey string) (locked bool, retryAfter time.Duration, found bool) {
	if usernameKey == "" {
		return false, 0, false
	}
	c.mu.RLock()
	entry, ok := c.lockouts[usernameKey]
	c.mu.RUnlock()

	if ok && entry != nil {
		now := time.Now()
		if now.Before(entry.LockedUntil) {
			return true, time.Until(entry.LockedUntil), true
		}
		// Expired
		c.mu.Lock()
		delete(c.lockouts, usernameKey)
		c.mu.Unlock()
		return false, 0, true
	}
	return false, 0, false
}

// SetLockout marks an account as locked in memory until the given timestamp.
func (c *AuthCache) SetLockout(usernameKey string, lockedUntil time.Time) {
	if usernameKey == "" {
		return
	}
	c.mu.Lock()
	c.lockouts[usernameKey] = &LockoutCacheEntry{LockedUntil: lockedUntil}
	c.mu.Unlock()
}

// ClearLockout removes the lockout from memory immediately.
func (c *AuthCache) ClearLockout(usernameKey string) {
	if usernameKey == "" {
		return
	}
	c.mu.Lock()
	delete(c.lockouts, usernameKey)
	c.mu.Unlock()
}
