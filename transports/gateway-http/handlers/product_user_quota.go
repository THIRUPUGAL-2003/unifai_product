package handlers

import (
	"context"
	"sync"
	"time"

	"github.com/gateway/gateway/framework/configstore"
	"github.com/gateway/gateway/framework/configstore/tables"
	"github.com/gateway/gateway/framework/logstore"
	"github.com/valyala/fasthttp"
)

var (
	quotaCacheMu    sync.RWMutex
	cachedQuotaData logstore.ProductUserUsage
	cachedQuotaAt   time.Time
)

func loadProductUserUsage(ctx context.Context, store configstore.ConfigStore, bypassCache ...bool) (logstore.ProductUserUsage, error) {
	if store == nil {
		return logstore.ProductUserUsage{}, nil
	}

	skipCache := len(bypassCache) > 0 && bypassCache[0]
	if !skipCache {
		quotaCacheMu.RLock()
		if !cachedQuotaAt.IsZero() && time.Since(cachedQuotaAt) < 5*time.Second {
			cached := cachedQuotaData
			quotaCacheMu.RUnlock()
			return cached, nil
		}
		quotaCacheMu.RUnlock()
	}

	maxUsers := 0
	if db := store.DB(); db != nil {
		mgr := logstore.NewBrowserAIManager(db)
		lic, err := mgr.GetActiveLicense(ctx)
		if err != nil {
			return logstore.ProductUserUsage{}, err
		}
		if lic != nil && lic.IsActive {
			maxUsers = lic.MaxProductUsers
		}
	}
	users, err := store.GetUsers(ctx)
	if err != nil {
		return logstore.ProductUserUsage{}, err
	}
	roles := make([]string, 0, len(users))
	for _, user := range users {
		if user == nil || user.Status == tables.UserStatusRejected {
			continue
		}
		roles = append(roles, user.Role)
	}
	usage := logstore.ProductUserUsageFromRoles(roles, maxUsers)

	quotaCacheMu.Lock()
	cachedQuotaData = usage
	cachedQuotaAt = time.Now()
	quotaCacheMu.Unlock()

	return usage, nil
}

func (h *SessionHandler) rejectIfProductUsersFull(ctx *fasthttp.RequestCtx) bool {
	usage, err := loadProductUserUsage(ctx, h.configStore, true)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Could not read the license user limit")
		return true
	}
	if msg := usage.BlockReason(); msg != "" {
		SendError(ctx, fasthttp.StatusForbidden, msg)
		return true
	}
	return false
}

func (h *SessionHandler) getProductUserQuota(ctx *fasthttp.RequestCtx) {
	if !h.isAdmin(ctx) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}
	usage, err := loadProductUserUsage(ctx, h.configStore, false)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Could not read the license user limit")
		return
	}
	SendJSON(ctx, usage)
}

