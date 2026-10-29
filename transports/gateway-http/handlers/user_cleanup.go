package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/gateway/gateway/framework/configstore"
	"github.com/gateway/gateway/framework/configstore/tables"
	"gorm.io/gorm"
)

// userModelConfigEvicter is implemented by governance managers that cache user-scoped
// model configs; called after a user is deleted.
type userModelConfigEvicter interface {
	DeleteUserModelConfigs(ctx context.Context, userID string)
}

// purgeUserRelations removes the user's persisted relations and user row atomically.
// Prompt lifecycle changes are performed first because they use the store directly rather
// than the transaction handle. Any failure is returned so callers never report a successful
// deletion with stale links or sessions.
func purgeUserRelations(ctx context.Context, cs configstore.ConfigStore, lifecycle *PromptLifecycleManager, user *tables.TableUser) error {
	if cs == nil || user == nil || user.ID == "" {
		return errors.New("config store and user are required to delete user relations")
	}
	if lifecycle != nil {
		if err := lifecycle.OnUserDeleted(ctx, user); err != nil {
			return fmt.Errorf("move user's prompt to Removed Users: %w", err)
		}
	}

	return cs.ExecuteTransaction(ctx, func(tx *gorm.DB) error {
		if tx == nil {
			return errors.New("database transaction is unavailable")
		}
		if user.BudgetID != nil && *user.BudgetID != "" {
			if err := cs.DeleteBudget(ctx, *user.BudgetID, tx); err != nil && !errors.Is(err, configstore.ErrNotFound) {
				return fmt.Errorf("delete user budget %s: %w", *user.BudgetID, err)
			}
		}
		if user.RateLimitID != nil && *user.RateLimitID != "" {
			if err := cs.DeleteRateLimit(ctx, *user.RateLimitID, tx); err != nil && !errors.Is(err, configstore.ErrNotFound) {
				return fmt.Errorf("delete user rate limit %s: %w", *user.RateLimitID, err)
			}
		}
		if tx.Migrator().HasTable(&tables.TableVirtualKeyUser{}) {
			if err := tx.WithContext(ctx).Where("user_id = ?", user.ID).Delete(&tables.TableVirtualKeyUser{}).Error; err != nil {
				return fmt.Errorf("delete user's virtual-key assignments: %w", err)
			}
		}
		if tx.Migrator().HasTable(&tables.TableTeamMember{}) {
			if err := tx.WithContext(ctx).Where("user_id = ?", user.ID).Delete(&tables.TableTeamMember{}).Error; err != nil {
				return fmt.Errorf("delete user's team memberships: %w", err)
			}
		}
		if err := cs.DeleteModelConfigsForScope(ctx, tx, tables.ModelConfigScopeUser, user.ID); err != nil {
			return fmt.Errorf("delete user-scoped model configs: %w", err)
		}
		if err := cs.DeleteSessionsByUsername(ctx, user.Username, tx); err != nil {
			return fmt.Errorf("revoke user's sessions: %w", err)
		}
		if err := cs.DeleteUser(ctx, user.ID, tx); err != nil {
			return fmt.Errorf("delete user row: %w", err)
		}
		return nil
	})
}
