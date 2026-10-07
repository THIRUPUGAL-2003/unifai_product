package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fasthttp/router"
	"github.com/glebarez/sqlite"
	"github.com/gateway/gateway/framework/configstore"
	"github.com/gateway/gateway/framework/configstore/tables"
	"gorm.io/gorm"
)

type retentionTestStore struct {
	configstore.ConfigStore
	db *gorm.DB
}

func (s *retentionTestStore) DB() *gorm.DB {
	return s.db
}

func (s *retentionTestStore) GetPromptHistorySettings(ctx context.Context) (*tables.PromptHistoryRetentionSettings, error) {
	var configEntry tables.TableGovernanceConfig
	if err := s.db.WithContext(ctx).First(&configEntry, "key = ?", tables.ConfigPromptHistorySettingsKey).Error; err != nil {
		return &tables.PromptHistoryRetentionSettings{
			AutoDelete: false,
			Retention:  "7d",
		}, nil
	}
	var settings tables.PromptHistoryRetentionSettings
	if err := json.Unmarshal([]byte(configEntry.Value), &settings); err != nil {
		return nil, err
	}
	return &settings, nil
}

func (s *retentionTestStore) SetPromptHistorySettings(ctx context.Context, settings *tables.PromptHistoryRetentionSettings) error {
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Save(&tables.TableGovernanceConfig{
		Key:   tables.ConfigPromptHistorySettingsKey,
		Value: string(data),
	}).Error
}

func (s *retentionTestStore) DeletePromptSessionsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	var sessionIDs []uint
	if err := s.db.WithContext(ctx).Model(&tables.TablePromptSession{}).Where("updated_at < ?", cutoff).Pluck("id", &sessionIDs).Error; err != nil {
		return 0, err
	}
	if len(sessionIDs) == 0 {
		return 0, nil
	}
	_ = s.db.WithContext(ctx).Where("session_id IN ?", sessionIDs).Delete(&tables.TablePromptSessionMessage{}).Error
	res := s.db.WithContext(ctx).Where("id IN ?", sessionIDs).Delete(&tables.TablePromptSession{})
	return res.RowsAffected, res.Error
}

func (s *retentionTestStore) ClearAllPromptSessions(ctx context.Context) (int64, error) {
	_ = s.db.WithContext(ctx).Where("1 = 1").Delete(&tables.TablePromptSessionMessage{}).Error
	res := s.db.WithContext(ctx).Where("1 = 1").Delete(&tables.TablePromptSession{})
	return res.RowsAffected, res.Error
}

func (s *retentionTestStore) GetSession(ctx context.Context, token string) (*tables.SessionsTable, error) {
	return &tables.SessionsTable{
		Token:    token,
		Username: "admin",
		Role:     "admin",
	}, nil
}

func (s *retentionTestStore) GetUserByUsername(ctx context.Context, username string) (*tables.TableUser, error) {
	return &tables.TableUser{
		ID:       "usr_admin",
		Username: "admin",
	}, nil
}

func setupRetentionTestStore(t *testing.T) *retentionTestStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_loc=auto"), &gorm.Config{})
	if err != nil {
		t.Skipf("sqlite unavailable: %v", err)
	}

	err = db.AutoMigrate(
		&tables.TableGovernanceConfig{},
		&tables.TablePromptSession{},
		&tables.TablePromptSessionMessage{},
		&tables.TableUser{},
		&tables.SessionsTable{},
	)
	if err != nil {
		t.Fatalf("failed to migrate tables: %v", err)
	}

	return &retentionTestStore{db: db}
}

func TestPromptHistoryRetention_Lifecycle(t *testing.T) {
	store := setupRetentionTestStore(t)
	handler := &PromptsHandler{
		store: store,
	}

	r := router.New()
	handler.RegisterRoutes(r)

	// 1. Check default settings
	ctx := context.Background()
	settings, err := store.GetPromptHistorySettings(ctx)
	if err != nil {
		t.Fatalf("unexpected error getting default settings: %v", err)
	}
	if settings.AutoDelete != false || settings.Retention != "7d" {
		t.Fatalf("expected defaults false and 7d, got %+v", settings)
	}

	// 2. Insert dummy sessions: 1 old (10 days old), 1 recent (1 hour old)
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	recentTime := time.Now().Add(-1 * time.Hour)

	oldSession := tables.TablePromptSession{
		PromptID:  "prompt_1",
		Name:      "Old Session",
		UserID:    "user_1",
		CreatedAt: oldTime,
		UpdatedAt: oldTime,
	}
	if err := store.db.Create(&oldSession).Error; err != nil {
		t.Fatalf("failed to create old session: %v", err)
	}

	recentSession := tables.TablePromptSession{
		PromptID:  "prompt_1",
		Name:      "Recent Session",
		UserID:    "user_1",
		CreatedAt: recentTime,
		UpdatedAt: recentTime,
	}
	if err := store.db.Create(&recentSession).Error; err != nil {
		t.Fatalf("failed to create recent session: %v", err)
	}

	// Also insert messages for both
	store.db.Create(&tables.TablePromptSessionMessage{
		PromptID:    "prompt_1",
		SessionID:   oldSession.ID,
		OrderIndex:  0,
		MessageJSON: `{"role":"user","content":"hello old"}`,
	})
	store.db.Create(&tables.TablePromptSessionMessage{
		PromptID:    "prompt_1",
		SessionID:   recentSession.ID,
		OrderIndex:  0,
		MessageJSON: `{"role":"user","content":"hello recent"}`,
	})

	// 3. Enable auto-delete with 7d retention
	err = store.SetPromptHistorySettings(ctx, &tables.PromptHistoryRetentionSettings{
		AutoDelete: true,
		Retention:  "7d",
	})
	if err != nil {
		t.Fatalf("failed to save settings: %v", err)
	}

	// 4. Trigger applyPromptHistoryAutoDelete
	deleted, err := handler.applyPromptHistoryAutoDelete(ctx)
	if err != nil {
		t.Fatalf("failed to run applyPromptHistoryAutoDelete: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 session deleted, got %d", deleted)
	}

	// Verify old session is gone and recent session remains
	var remainingCount int64
	store.db.Model(&tables.TablePromptSession{}).Count(&remainingCount)
	if remainingCount != 1 {
		t.Fatalf("expected 1 session remaining, got %d", remainingCount)
	}

	var remainingSession tables.TablePromptSession
	store.db.First(&remainingSession)
	if remainingSession.ID != recentSession.ID {
		t.Fatalf("expected recent session to remain, got ID %d", remainingSession.ID)
	}

	// 5. Test Clear All Sessions
	clearedCount, err := store.ClearAllPromptSessions(ctx)
	if err != nil {
		t.Fatalf("failed to clear all sessions: %v", err)
	}
	if clearedCount != 1 {
		t.Fatalf("expected 1 session cleared, got %d", clearedCount)
	}

	store.db.Model(&tables.TablePromptSession{}).Count(&remainingCount)
	if remainingCount != 0 {
		t.Fatalf("expected 0 sessions after clear all, got %d", remainingCount)
	}

	var remainingMsgs int64
	store.db.Model(&tables.TablePromptSessionMessage{}).Count(&remainingMsgs)
	if remainingMsgs != 0 {
		t.Fatalf("expected 0 session messages after clear all, got %d", remainingMsgs)
	}
}
