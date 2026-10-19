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

func TestSameNameRecreateRestoresRemovedPromptAndFolder(t *testing.T) {
	store := setupLifecycleTestStore(t)
	lifecycle := NewPromptLifecycleManager(store)
	ctx := context.Background()
	users := usersTestRoot(t, lifecycle, "Users", "system_users_root")
	removedUsers := usersTestRoot(t, lifecycle, "Removed Users", "system_removed_users_root")
	customers := usersTestRoot(t, lifecycle, "Customers", "system_customers_root")
	removedCustomers := usersTestRoot(t, lifecycle, "Removed Customers", "system_removed_customers_root")
	teams := usersTestRoot(t, lifecycle, "Teams", "system_teams_root")
	_ = usersTestRoot(t, lifecycle, "Removed Teams", "system_removed_teams_root")

	customer := &tables.TableCustomer{ID: syncTestID("cust"), Name: "Acme"}
	_ = store.CreateCustomer(ctx, customer)
	if err := lifecycle.OnCustomerCreated(ctx, customer); err != nil {
		t.Fatalf("OnCustomerCreated: %v", err)
	}
	var customerFolder tables.TableFolder
	if err := store.DB().Where("entity_id = ?", customer.ID).First(&customerFolder).Error; err != nil {
		t.Fatalf("customer folder: %v", err)
	}
	if err := lifecycle.OnCustomerDeleted(ctx, customer); err != nil {
		t.Fatalf("OnCustomerDeleted: %v", err)
	}

	same := &tables.TableCustomer{ID: syncTestID("cust"), Name: "Acme"}
	_ = store.CreateCustomer(ctx, same)
	if err := lifecycle.OnCustomerCreated(ctx, same); err != nil {
		t.Fatalf("recreate customer: %v", err)
	}
	var restored tables.TableFolder
	if err := store.DB().Where("id = ?", customerFolder.ID).First(&restored).Error; err != nil {
		t.Fatalf("restored folder: %v", err)
	}
	if restored.Type != "customer" || restored.ParentID == nil || *restored.ParentID != customers.ID || restored.Name != "Acme" {
		t.Fatalf("same customer name must restore the folder, got %+v", restored)
	}
	if restored.EntityID == nil || *restored.EntityID != same.ID {
		t.Fatalf("restored folder must belong to the new customer")
	}
	var stillRemoved int64
	store.DB().Model(&tables.TableFolder{}).Where("id = ? AND parent_id = ?", customerFolder.ID, removedCustomers.ID).Count(&stillRemoved)
	if stillRemoved != 0 {
		t.Fatalf("restored customer folder must leave Removed Customers")
	}

	other := &tables.TableCustomer{ID: syncTestID("cust"), Name: "Other Co"}
	_ = store.CreateCustomer(ctx, other)
	if err := lifecycle.OnCustomerDeleted(ctx, same); err != nil {
		t.Fatalf("delete restored customer: %v", err)
	}
	if err := lifecycle.OnCustomerCreated(ctx, other); err != nil {
		t.Fatalf("different customer: %v", err)
	}
	var otherFolder tables.TableFolder
	if err := store.DB().Where("entity_id = ?", other.ID).First(&otherFolder).Error; err != nil {
		t.Fatalf("other folder: %v", err)
	}
	if otherFolder.ID == customerFolder.ID {
		t.Fatalf("a different customer name must create a new folder")
	}
	var archived tables.TableFolder
	if err := store.DB().Where("id = ?", customerFolder.ID).First(&archived).Error; err != nil {
		t.Fatalf("archived folder: %v", err)
	}
	if archived.ParentID == nil || *archived.ParentID != removedCustomers.ID {
		t.Fatalf("different name must leave the old folder in Removed Customers")
	}

	team := &tables.TableTeam{ID: syncTestID("team"), Name: "Platform"}
	_ = store.CreateTeam(ctx, team)
	if err := lifecycle.OnTeamCreated(ctx, team); err != nil {
		t.Fatalf("OnTeamCreated: %v", err)
	}
	var teamFolder tables.TableFolder
	_ = store.DB().Where("entity_id = ?", team.ID).First(&teamFolder).Error
	if err := lifecycle.OnTeamDeleted(ctx, team); err != nil {
		t.Fatalf("OnTeamDeleted: %v", err)
	}
	newTeam := &tables.TableTeam{ID: syncTestID("team"), Name: "Platform"}
	_ = store.CreateTeam(ctx, newTeam)
	if err := lifecycle.OnTeamCreated(ctx, newTeam); err != nil {
		t.Fatalf("recreate team: %v", err)
	}
	var teamBack tables.TableFolder
	_ = store.DB().Where("id = ?", teamFolder.ID).First(&teamBack).Error
	if teamBack.Type != "team" || teamBack.ParentID == nil || *teamBack.ParentID != teams.ID {
		t.Fatalf("same team name must restore the folder under Teams")
	}

	user := syncTestUser(t, store)
	if err := lifecycle.OnUserCreated(ctx, user, true); err != nil {
		t.Fatalf("OnUserCreated: %v", err)
	}
	prompt := usersTestPrompt(t, store, user.Email)
	if err := lifecycle.OnUserDeleted(ctx, user); err != nil {
		t.Fatalf("OnUserDeleted: %v", err)
	}
	_ = store.DB().Delete(&tables.TableUser{}, "id = ?", user.ID).Error

	recreated := &tables.TableUser{ID: syncTestID("uid"), Username: user.Username, Email: user.Email}
	if err := store.CreateUser(ctx, recreated); err != nil {
		t.Fatalf("recreate user: %v", err)
	}
	if err := lifecycle.OnUserCreated(ctx, recreated, true); err != nil {
		t.Fatalf("recreate user prompt: %v", err)
	}
	if usersTestPromptCount(store, user.Email) != 1 {
		t.Fatalf("same email must not create a second prompt")
	}
	back := usersTestPrompt(t, store, user.Email)
	if back.ID != prompt.ID || back.FolderID == nil || *back.FolderID != users.ID {
		t.Fatalf("same email must bring the Removed prompt back to Users")
	}
	if back.OwnerUserID == nil || *back.OwnerUserID != recreated.ID {
		t.Fatalf("restored prompt must belong to the new user")
	}
	if !syncTestHasPrompt(t, store, recreated.ID, prompt.ID) {
		t.Fatalf("new user must be allowed to open the restored prompt")
	}

	different := &tables.TableUser{ID: syncTestID("uid"), Username: syncTestID("other"), Email: syncTestID("other") + "@example.com"}
	if err := store.CreateUser(ctx, different); err != nil {
		t.Fatalf("create different user: %v", err)
	}
	if err := lifecycle.OnUserDeleted(ctx, recreated); err != nil {
		t.Fatalf("delete recreated user: %v", err)
	}
	_ = store.DB().Delete(&tables.TableUser{}, "id = ?", recreated.ID).Error
	if err := lifecycle.OnUserCreated(ctx, different, true); err != nil {
		t.Fatalf("different user: %v", err)
	}
	if usersTestPrompt(t, store, user.Email).FolderID == nil || *usersTestPrompt(t, store, user.Email).FolderID != removedUsers.ID {
		t.Fatalf("a different user must leave the old prompt in Removed Users")
	}
	if usersTestPromptCount(store, different.Email) != 1 {
		t.Fatalf("a different email must get its own new prompt")
	}
}
