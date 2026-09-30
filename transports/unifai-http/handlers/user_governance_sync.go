package handlers

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/configstore/tables"
	"gorm.io/gorm"
)

// UserGovernanceSyncer pushes materialized user budget/rate-limit rows into the
// governance in-memory store so enforcement picks them up without a restart.
type UserGovernanceSyncer interface {
	SyncUserGovernance(ctx context.Context, userID string, budget *tables.TableBudget, rateLimit *tables.TableRateLimit)
	DeleteUserGovernance(ctx context.Context, userID string)
}

const (
	userBudgetResetDuration = "1M"
	userRateLimitReset      = "1m"
)

// materializeUserGovernanceLimits turns Users.Budget / Users.RateLimit UI fields
// into live TableBudget / TableRateLimit rows owned by the user, then syncs memory.
// Optional tx keeps create/update + materialize atomic when passed from ExecuteTransaction.
func (h *SessionHandler) materializeUserGovernanceLimits(ctx context.Context, user *tables.TableUser, tx ...*gorm.DB) error {
	if h == nil || h.configStore == nil || user == nil || user.ID == "" {
		return nil
	}
	now := time.Now().UTC()
	uid := user.ID
	var txArgs []*gorm.DB
	if len(tx) > 0 && tx[0] != nil {
		txArgs = tx
	}

	var budget *tables.TableBudget
	if user.Budget > 0 {
		if user.BudgetID != nil && *user.BudgetID != "" {
			existing, err := h.configStore.GetBudget(ctx, *user.BudgetID)
			if err == nil && existing != nil {
				existing.MaxLimit = user.Budget
				existing.ResetDuration = userBudgetResetDuration
				existing.UserID = &uid
				existing.UpdatedAt = now
				if err := h.configStore.UpdateBudget(ctx, existing, txArgs...); err != nil {
					return err
				}
				budget = existing
			} else if err != nil && !errors.Is(err, configstore.ErrNotFound) {
				return err
			}
		}
		if budget == nil {
			bid := uuid.New().String()
			budget = &tables.TableBudget{
				ID:            bid,
				MaxLimit:      user.Budget,
				ResetDuration: userBudgetResetDuration,
				LastReset:     now,
				CurrentUsage:  0,
				UserID:        &uid,
				CreatedAt:     now,
				UpdatedAt:     now,
			}
			if err := h.configStore.CreateBudget(ctx, budget, txArgs...); err != nil {
				return err
			}
			user.BudgetID = &bid
		}
	} else if user.BudgetID != nil && *user.BudgetID != "" {
		if err := h.configStore.DeleteBudget(ctx, *user.BudgetID, txArgs...); err != nil && !errors.Is(err, configstore.ErrNotFound) {
			return err
		}
		user.BudgetID = nil
	}

	var rateLimit *tables.TableRateLimit
	if user.RateLimit > 0 {
		maxReq := int64(user.RateLimit)
		reset := userRateLimitReset
		if user.RateLimitID != nil && *user.RateLimitID != "" {
			existing, err := h.configStore.GetRateLimit(ctx, *user.RateLimitID)
			if err == nil && existing != nil {
				existing.RequestMaxLimit = &maxReq
				existing.RequestResetDuration = &reset
				existing.UpdatedAt = now
				if err := h.configStore.UpdateRateLimit(ctx, existing, txArgs...); err != nil {
					return err
				}
				rateLimit = existing
			} else if err != nil && !errors.Is(err, configstore.ErrNotFound) {
				return err
			}
		}
		if rateLimit == nil {
			rid := uuid.New().String()
			rateLimit = &tables.TableRateLimit{
				ID:                   rid,
				RequestMaxLimit:      &maxReq,
				RequestResetDuration: &reset,
				RequestLastReset:     now,
				TokenLastReset:       now,
				CreatedAt:            now,
				UpdatedAt:            now,
			}
			if err := h.configStore.CreateRateLimit(ctx, rateLimit, txArgs...); err != nil {
				return err
			}
			user.RateLimitID = &rid
		}
	} else if user.RateLimitID != nil && *user.RateLimitID != "" {
		if err := h.configStore.DeleteRateLimit(ctx, *user.RateLimitID, txArgs...); err != nil && !errors.Is(err, configstore.ErrNotFound) {
			return err
		}
		user.RateLimitID = nil
	}

	// Memory sync is best-effort and stays outside DB transactions.
	if len(txArgs) == 0 && h.userGovernance != nil {
		if budget == nil && rateLimit == nil {
			h.userGovernance.DeleteUserGovernance(ctx, user.ID)
		} else {
			h.userGovernance.SyncUserGovernance(ctx, user.ID, budget, rateLimit)
		}
	}
	return nil
}

// syncUserGovernanceMemory refreshes in-memory meters after a committed DB write.
func (h *SessionHandler) syncUserGovernanceMemory(ctx context.Context, user *tables.TableUser) {
	if h == nil || h.userGovernance == nil || user == nil || user.ID == "" {
		return
	}
	var budget *tables.TableBudget
	var rateLimit *tables.TableRateLimit
	if user.BudgetID != nil && *user.BudgetID != "" {
		if b, err := h.configStore.GetBudget(ctx, *user.BudgetID); err == nil {
			budget = b
		}
	}
	if user.RateLimitID != nil && *user.RateLimitID != "" {
		if r, err := h.configStore.GetRateLimit(ctx, *user.RateLimitID); err == nil {
			rateLimit = r
		}
	}
	if budget == nil && rateLimit == nil {
		h.userGovernance.DeleteUserGovernance(ctx, user.ID)
		return
	}
	h.userGovernance.SyncUserGovernance(ctx, user.ID, budget, rateLimit)
}
