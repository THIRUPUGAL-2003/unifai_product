package handlers

import (
	"context"
	"errors"

	"github.com/raksha/raksha/framework/configstore"
	"github.com/raksha/raksha/framework/configstore/tables"
	"gorm.io/gorm"
)

// userModelConfigEvicter is implemented by governance managers that cache user-scoped
// model configs; called after a user is deleted.
type userModelConfigEvicter interface {
	DeleteUserModelConfigs(ctx context.Context, userID string)
}

// purgeUserRelations removes everything that references a user who is about to be deleted:
// personal budget and rate limit, virtual-key links, team memberships, and the user's
// prompt (moved to Removed Users and withdrawn from teammates). Used by the admin Users
// page and by SCIM so both delete paths leave the same state behind.
func purgeUserRelations(ctx context.Context, cs configstore.ConfigStore, lifecycle *PromptLifecycleManager, user *tables.TableUser) {
	if cs == nil || user == nil {
		return
	}
	if user.BudgetID != nil && *user.BudgetID != "" {
		if err := cs.DeleteBudget(ctx, *user.BudgetID); err != nil && !errors.Is(err, configstore.ErrNotFound) && logger != nil {
			logger.Error("failed to delete user budget id=%s: %v", *user.BudgetID, err)
		}
	}
	_ = cs.ExecuteTransaction(ctx, func(tx *gorm.DB) error {
		if tx == nil {
			return nil
		}
		if tx.Migrator().HasTable(&tables.TableVirtualKeyUser{}) {
			_ = tx.Where("user_id = ?", user.ID).Delete(&tables.TableVirtualKeyUser{}).Error
		}
		return tx.Where("user_id = ?", user.ID).Delete(&tables.TableBudget{}).Error
	})
	if err := cs.ExecuteTransaction(ctx, func(tx *gorm.DB) error {
		if tx == nil {
			return nil
		}
		return cs.DeleteModelConfigsForScope(ctx, tx, tables.ModelConfigScopeUser, user.ID)
	}); err != nil && logger != nil {
		logger.Error("failed to delete user-scoped model configs user=%s: %v", user.ID, err)
	}
	if user.RateLimitID != nil && *user.RateLimitID != "" {
		if err := cs.DeleteRateLimit(ctx, *user.RateLimitID); err != nil && !errors.Is(err, configstore.ErrNotFound) && logger != nil {
			logger.Error("failed to delete user rate limit id=%s: %v", *user.RateLimitID, err)
		}
	}
	if ws, ok := configstore.AsWorkspaceStore(cs); ok && ws != nil {
		if memberships, err := ws.ListTeamsForUser(ctx, user.ID); err == nil {
			for _, m := range memberships {
				_ = ws.RemoveTeamMember(ctx, m.TeamID, user.ID)
			}
		}
	}
	if lifecycle != nil {
		_ = lifecycle.OnUserDeleted(ctx, user)
	}
}
