package handlers

import (
	"context"

	"github.com/gateway/gateway/framework/configstore"
	"github.com/gateway/gateway/framework/configstore/tables"
	"github.com/gateway/gateway/framework/logstore"
	"github.com/valyala/fasthttp"
)

func loadProductUserUsage(ctx context.Context, store configstore.ConfigStore) (logstore.ProductUserUsage, error) {
	if store == nil {
		return logstore.ProductUserUsage{}, nil
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
	return logstore.ProductUserUsageFromRoles(roles, maxUsers), nil
}

func (h *SessionHandler) rejectIfProductUsersFull(ctx *fasthttp.RequestCtx) bool {
	usage, err := loadProductUserUsage(ctx, h.configStore)
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
	usage, err := loadProductUserUsage(ctx, h.configStore)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Could not read the license user limit")
		return
	}
	SendJSON(ctx, usage)
}
