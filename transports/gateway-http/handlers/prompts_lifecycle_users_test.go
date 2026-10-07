package handlers

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gateway/gateway/framework/configstore/tables"
)

func usersTestRoot(t *testing.T, lifecycle *PromptLifecycleManager, name, folderType string) *tables.TableFolder {
	t.Helper()
	root, err := lifecycle.EnsureSystemFolder(context.Background(), name, folderType, nil)
	if err != nil {
		t.Fatalf("ensure %s root: %v", name, err)
	}
	return root
}

func usersTestPromptCount(store *lifecycleTestStore, name string) int64 {
	var n int64
	store.DB().Model(&tables.TablePrompt{}).Where("LOWER(name) = ?", strings.ToLower(name)).Count(&n)
	return n
}

func usersTestPrompt(t *testing.T, store *lifecycleTestStore, name string) *tables.TablePrompt {
	t.Helper()
	var p tables.TablePrompt
	if err := store.DB().Where("LOWER(name) = ?", strings.ToLower(name)).First(&p).Error; err != nil {
		t.Fatalf("prompt %q missing: %v", name, err)
	}
	return &p
}

func TestRepairDuplicateFolders_MergesDuplicateSystemRoot(t *testing.T) {
	store := setupLifecycleTestStore(t)
	lifecycle := NewPromptLifecycleManager(store)
	ctx := context.Background()

	primary := usersTestRoot(t, lifecycle, "Users", "system_users_root")
	dup := &tables.TableFolder{ID: uuid.New().String(), Name: "Users", Type: "system_users_root", CreatedAt: time.Now().Add(time.Hour), UpdatedAt: time.Now()}
	if err := store.DB().Create(dup).Error; err != nil {
		t.Fatalf("create duplicate root: %v", err)
	}
	stray := &tables.TablePrompt{ID: syncTestID("prompt"), Name: syncTestID("stray"), FolderID: &dup.ID}
	if err := store.CreatePrompt(ctx, stray); err != nil {
		t.Fatalf("create prompt: %v", err)
	}

	lifecycle.RepairDuplicateFolders(ctx)

	var n int64
	store.DB().Model(&tables.TableFolder{}).Where("id = ?", dup.ID).Count(&n)
	if n != 0 {
		t.Fatalf("duplicate Users root must be removed")
	}
	moved := usersTestPrompt(t, store, stray.Name)
	if moved.FolderID == nil || *moved.FolderID != primary.ID {
		t.Fatalf("prompt in duplicate root must move into the primary Users root")
	}
}

func TestOnUserLogin_DoesNotCreatePrompt(t *testing.T) {
	store := setupLifecycleTestStore(t)
	lifecycle := NewPromptLifecycleManager(store)
	user := syncTestUser(t, store)

	lifecycle.OnUserLogin(context.Background(), user)

	if usersTestPromptCount(store, user.Email) != 0 {
		t.Fatalf("login must not create a prompt for a user created without one")
	}
}

func TestOnUserCreated_ReusesPromptCaseInsensitively(t *testing.T) {
	store := setupLifecycleTestStore(t)
	lifecycle := NewPromptLifecycleManager(store)
	ctx := context.Background()
	users := usersTestRoot(t, lifecycle, "Users", "system_users_root")
	user := syncTestUser(t, store)

	existing := &tables.TablePrompt{ID: syncTestID("prompt"), Name: strings.ToUpper(user.Email), FolderID: &users.ID, OwnerUserID: &user.ID}
	if err := store.CreatePrompt(ctx, existing); err != nil {
		t.Fatalf("create prompt: %v", err)
	}

	if err := lifecycle.OnUserCreated(ctx, user, true); err != nil {
		t.Fatalf("OnUserCreated: %v", err)
	}
	if got := usersTestPromptCount(store, user.Email); got != 1 {
		t.Fatalf("expected 1 prompt for the user, got %d", got)
	}
	if !syncTestHasPrompt(t, store, user.ID, existing.ID) {
		t.Fatalf("user must get access to the existing prompt")
	}
}

func TestUserPromptFollowsOwnerNotName(t *testing.T) {
	store := setupLifecycleTestStore(t)
	lifecycle := NewPromptLifecycleManager(store)
	ctx := context.Background()
	usersTestRoot(t, lifecycle, "Users", "system_users_root")
	removed := usersTestRoot(t, lifecycle, "Removed Users", "system_removed_users_root")
	user := syncTestUser(t, store)
	if err := lifecycle.OnUserCreated(ctx, user, true); err != nil {
		t.Fatalf("OnUserCreated: %v", err)
	}
	own := usersTestPrompt(t, store, user.Email)

	// Someone else's prompt that happens to carry the user's email as its name.
	lookalike := &tables.TablePrompt{ID: syncTestID("prompt"), Name: strings.ToUpper(user.Email)}
	if err := store.CreatePrompt(ctx, lookalike); err != nil {
		t.Fatalf("create prompt: %v", err)
	}
	// The user's own prompt is renamed manually.
	_ = store.DB().Model(&tables.TablePrompt{}).Where("id = ?", own.ID).Update("name", "My work prompt").Error

	if err := lifecycle.OnUserCreated(ctx, user, true); err != nil {
		t.Fatalf("OnUserCreated again: %v", err)
	}
	var owned int64
	store.DB().Model(&tables.TablePrompt{}).Where("owner_user_id = ?", user.ID).Count(&owned)
	if owned != 1 {
		t.Fatalf("renamed prompt must still be found by owner, got %d owned prompts", owned)
	}

	if err := lifecycle.OnUserDeleted(ctx, user); err != nil {
		t.Fatalf("OnUserDeleted: %v", err)
	}
	var after tables.TablePrompt
	_ = store.DB().Where("id = ?", own.ID).First(&after).Error
	if after.FolderID == nil || *after.FolderID != removed.ID {
		t.Fatalf("user's renamed prompt must move to Removed Users")
	}
	var other tables.TablePrompt
	_ = store.DB().Where("id = ?", lookalike.ID).First(&other).Error
	if other.FolderID != nil && *other.FolderID == removed.ID {
		t.Fatalf("a prompt that only shares the name must not be treated as the user's")
	}
}

func TestOnUserCreated_RestoresArchivedPrompt(t *testing.T) {
	store := setupLifecycleTestStore(t)
	lifecycle := NewPromptLifecycleManager(store)
	ctx := context.Background()
	users := usersTestRoot(t, lifecycle, "Users", "system_users_root")
	removed := usersTestRoot(t, lifecycle, "Removed Users", "system_removed_users_root")
	user := syncTestUser(t, store)

	if err := lifecycle.OnUserCreated(ctx, user, true); err != nil {
		t.Fatalf("OnUserCreated: %v", err)
	}
	if err := lifecycle.OnUserDeleted(ctx, user); err != nil {
		t.Fatalf("OnUserDeleted: %v", err)
	}
	if p := usersTestPrompt(t, store, user.Email); p.FolderID == nil || *p.FolderID != removed.ID {
		t.Fatalf("deleted user's prompt must move to Removed Users")
	}

	if err := lifecycle.OnUserCreated(ctx, user, false); err != nil {
		t.Fatalf("OnUserCreated again: %v", err)
	}
	if got := usersTestPromptCount(store, user.Email); got != 1 {
		t.Fatalf("re-created user must not get a second prompt, got %d", got)
	}
	p := usersTestPrompt(t, store, user.Email)
	if p.FolderID == nil || *p.FolderID != users.ID {
		t.Fatalf("re-created user's prompt must come back to Users")
	}
	if !syncTestHasPrompt(t, store, user.ID, p.ID) {
		t.Fatalf("re-created user must get access to the restored prompt")
	}
}

func TestOnUserDeleted_RevokesPromptFromTeammates(t *testing.T) {
	store := setupLifecycleTestStore(t)
	lifecycle := NewPromptLifecycleManager(store)
	ctx := context.Background()
	usersTestRoot(t, lifecycle, "Removed Users", "system_removed_users_root")

	team, folder := syncTestTeamWithFolder(t, store, lifecycle, nil)
	leaving := syncTestUser(t, store)
	teammate := syncTestUser(t, store)
	syncTestJoin(t, store, team.ID, leaving.ID)
	if err := lifecycle.OnUserCreated(ctx, leaving, true); err != nil {
		t.Fatalf("OnUserCreated: %v", err)
	}
	own := usersTestPrompt(t, store, leaving.Email)
	if own.FolderID == nil || *own.FolderID != folder.ID {
		t.Fatalf("team member's prompt must be created in the team folder")
	}

	syncTestJoin(t, store, team.ID, teammate.ID)
	if err := lifecycle.OnTeamMemberAdded(ctx, team.ID, teammate.ID); err != nil {
		t.Fatalf("OnTeamMemberAdded: %v", err)
	}
	if !syncTestHasPrompt(t, store, teammate.ID, own.ID) {
		t.Fatalf("teammate must see prompts in the team folder")
	}

	if err := lifecycle.OnUserDeleted(ctx, leaving); err != nil {
		t.Fatalf("OnUserDeleted: %v", err)
	}
	if syncTestHasPrompt(t, store, teammate.ID, own.ID) {
		t.Fatalf("deleted user's prompt must be withdrawn from teammates")
	}
	if usersTestPromptCount(store, teammate.Email) != 1 {
		t.Fatalf("teammate must have exactly one own prompt")
	}
}

func TestOnUserUpdated_UsernameChangeDoesNotDuplicateEmailPrompt(t *testing.T) {
	store := setupLifecycleTestStore(t)
	lifecycle := NewPromptLifecycleManager(store)
	ctx := context.Background()
	usersTestRoot(t, lifecycle, "Users", "system_users_root")
	user := syncTestUser(t, store)
	if err := lifecycle.OnUserCreated(ctx, user, true); err != nil {
		t.Fatalf("OnUserCreated: %v", err)
	}
	oldUsername := user.Username
	unrelated := &tables.TablePrompt{ID: syncTestID("prompt"), Name: oldUsername}
	if err := store.CreatePrompt(ctx, unrelated); err != nil {
		t.Fatalf("create prompt: %v", err)
	}

	user.Username = syncTestID("renamed")
	if err := lifecycle.OnUserUpdated(ctx, user, user.Email, oldUsername); err != nil {
		t.Fatalf("OnUserUpdated: %v", err)
	}
	if got := usersTestPromptCount(store, user.Email); got != 1 {
		t.Fatalf("expected exactly 1 prompt named after the email, got %d", got)
	}
	if usersTestPromptCount(store, oldUsername) != 1 {
		t.Fatalf("unrelated prompt must keep its name")
	}
}
