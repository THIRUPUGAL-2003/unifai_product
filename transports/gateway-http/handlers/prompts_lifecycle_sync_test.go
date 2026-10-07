package handlers

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gateway/gateway/framework/configstore/tables"
)

func syncTestID(prefix string) string { return prefix + "_" + uuid.New().String()[:8] }

func syncTestTeamWithFolder(t *testing.T, store *lifecycleTestStore, lifecycle *PromptLifecycleManager, customerID *string) (*tables.TableTeam, *tables.TableFolder) {
	t.Helper()
	ctx := context.Background()
	team := &tables.TableTeam{ID: syncTestID("team"), Name: syncTestID("Team"), CustomerID: customerID}
	if err := store.CreateTeam(ctx, team); err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := lifecycle.OnTeamCreated(ctx, team); err != nil {
		t.Fatalf("create team folder: %v", err)
	}
	folder, err := findEntityFolder(store.DB(), team.ID, "", "team")
	if err != nil {
		t.Fatalf("team folder missing: %v", err)
	}
	return team, folder
}

func syncTestUser(t *testing.T, store *lifecycleTestStore) *tables.TableUser {
	t.Helper()
	name := syncTestID("user")
	u := &tables.TableUser{ID: syncTestID("uid"), Username: name, Email: name + "@example.com"}
	if err := store.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u
}

func syncTestJoin(t *testing.T, store *lifecycleTestStore, teamID, userID string) {
	t.Helper()
	if err := store.DB().Create(&tables.TableTeamMember{TeamID: teamID, UserID: userID}).Error; err != nil {
		t.Fatalf("join team: %v", err)
	}
}

func syncTestMemberCount(store *lifecycleTestStore, teamID, userID string) int64 {
	var n int64
	store.DB().Model(&tables.TableTeamMember{}).Where("team_id = ? AND user_id = ?", teamID, userID).Count(&n)
	return n
}

func syncTestHasPrompt(t *testing.T, store *lifecycleTestStore, userID, promptID string) bool {
	t.Helper()
	u, err := store.GetUserByID(context.Background(), userID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	for _, id := range strings.Split(u.AllowedPromptRepos, ",") {
		if strings.TrimSpace(id) == promptID {
			return true
		}
	}
	return false
}

// Moving a shared prompt must not touch the membership of the user who created/used it.
func TestPromptMove_SharedPromptDoesNotChangeCreatorMembership(t *testing.T) {
	store := setupLifecycleTestStore(t)
	lifecycle := NewPromptLifecycleManager(store)
	ctx := context.Background()

	teamA, _ := syncTestTeamWithFolder(t, store, lifecycle, nil)
	_, folderB := syncTestTeamWithFolder(t, store, lifecycle, nil)
	creator := syncTestUser(t, store)
	syncTestJoin(t, store, teamA.ID, creator.ID)

	prompt := &tables.TablePrompt{ID: syncTestID("prompt"), Name: syncTestID("Shared helper")}
	_ = store.CreatePrompt(ctx, prompt)
	_ = store.CreatePromptSession(ctx, &tables.TablePromptSession{PromptID: prompt.ID, Name: "s", UserID: creator.ID})

	empty := ""
	lifecycle.OnPromptMoved(ctx, prompt.ID, &empty, folderB.ID)

	if syncTestMemberCount(store, teamA.ID, creator.ID) != 1 {
		t.Fatalf("creator must stay in team A after moving a shared prompt")
	}
}

// A shared prompt that merely carries a user's email as its name is not that user's prompt.
func TestPromptMove_NameMatchWithoutOwnerDoesNotChangeMembership(t *testing.T) {
	store := setupLifecycleTestStore(t)
	lifecycle := NewPromptLifecycleManager(store)
	ctx := context.Background()

	teamA, folderA := syncTestTeamWithFolder(t, store, lifecycle, nil)
	teamB, folderB := syncTestTeamWithFolder(t, store, lifecycle, nil)
	user := syncTestUser(t, store)
	syncTestJoin(t, store, teamA.ID, user.ID)

	oldFolderID := folderA.ID
	prompt := &tables.TablePrompt{ID: syncTestID("prompt"), Name: user.Email, FolderID: &oldFolderID}
	_ = store.CreatePrompt(ctx, prompt)
	lifecycle.OnPromptMoved(ctx, prompt.ID, &oldFolderID, folderB.ID)

	if syncTestMemberCount(store, teamA.ID, user.ID) != 1 {
		t.Fatalf("user must stay in team A: the prompt is not owned by them")
	}
	if syncTestMemberCount(store, teamB.ID, user.ID) != 0 {
		t.Fatalf("user must not be added to team B: the prompt is not owned by them")
	}
}

// A multi-team user's prompt moved from team A to team B leaves team C untouched.
func TestPromptMove_KnownOriginKeepsOtherMemberships(t *testing.T) {
	store := setupLifecycleTestStore(t)
	lifecycle := NewPromptLifecycleManager(store)
	ctx := context.Background()

	teamA, folderA := syncTestTeamWithFolder(t, store, lifecycle, nil)
	teamB, folderB := syncTestTeamWithFolder(t, store, lifecycle, nil)
	teamC, _ := syncTestTeamWithFolder(t, store, lifecycle, nil)
	user := syncTestUser(t, store)
	syncTestJoin(t, store, teamA.ID, user.ID)
	syncTestJoin(t, store, teamC.ID, user.ID)

	oldFolderID := folderA.ID
	startFolder := folderA.ID
	prompt := &tables.TablePrompt{ID: syncTestID("prompt"), Name: user.Email, FolderID: &startFolder, OwnerUserID: &user.ID}
	_ = store.CreatePrompt(ctx, prompt)
	_ = store.DB().Model(&tables.TablePrompt{}).Where("id = ?", prompt.ID).Update("folder_id", folderB.ID).Error
	lifecycle.OnPromptMoved(ctx, prompt.ID, &oldFolderID, folderB.ID)

	if syncTestMemberCount(store, teamA.ID, user.ID) != 0 {
		t.Fatalf("expected user removed from team A")
	}
	if syncTestMemberCount(store, teamB.ID, user.ID) != 1 {
		t.Fatalf("expected user added to team B")
	}
	if syncTestMemberCount(store, teamC.ID, user.ID) != 1 {
		t.Fatalf("expected user to stay in team C")
	}
}

// Prompts moved out of a team folder are withdrawn from members who only had them via the team.
func TestPromptMove_RevokesFromOldTeamMembers(t *testing.T) {
	store := setupLifecycleTestStore(t)
	lifecycle := NewPromptLifecycleManager(store)
	ctx := context.Background()

	teamA, folderA := syncTestTeamWithFolder(t, store, lifecycle, nil)
	member := syncTestUser(t, store)
	syncTestJoin(t, store, teamA.ID, member.ID)

	oldFolderID := folderA.ID
	startFolder := folderA.ID
	prompt := &tables.TablePrompt{ID: syncTestID("prompt"), Name: syncTestID("Team playbook"), FolderID: &startFolder}
	_ = store.CreatePrompt(ctx, prompt)
	_ = lifecycle.OnPromptCreated(ctx, prompt, "")
	if !syncTestHasPrompt(t, store, member.ID, prompt.ID) {
		t.Fatalf("member should get the team prompt")
	}

	_ = store.DB().Model(&tables.TablePrompt{}).Where("id = ?", prompt.ID).Update("folder_id", nil).Error
	lifecycle.OnPromptMoved(ctx, prompt.ID, &oldFolderID, "")
	if syncTestHasPrompt(t, store, member.ID, prompt.ID) {
		t.Fatalf("member should lose the prompt once it leaves the team folder")
	}
}

// Joining a team grants prompts in team subfolders and at the customer level; leaving revokes them.
func TestTeamMembership_GrantsAndRevokesTeamAudience(t *testing.T) {
	store := setupLifecycleTestStore(t)
	lifecycle := NewPromptLifecycleManager(store)
	ctx := context.Background()

	customer := &tables.TableCustomer{ID: syncTestID("cust"), Name: syncTestID("Customer")}
	_ = store.DB().Create(customer).Error
	_ = lifecycle.OnCustomerCreated(ctx, customer)
	custFolder, err := findEntityFolder(store.DB(), customer.ID, "", "customer")
	if err != nil {
		t.Fatalf("customer folder missing: %v", err)
	}
	team, teamFolder := syncTestTeamWithFolder(t, store, lifecycle, &customer.ID)

	sub := &tables.TableFolder{ID: uuid.New().String(), Name: "Sub", Type: "custom", ParentID: &teamFolder.ID}
	_ = store.CreateFolder(ctx, sub)
	subPrompt := &tables.TablePrompt{ID: syncTestID("prompt"), Name: syncTestID("Sub prompt"), FolderID: &sub.ID}
	custPrompt := &tables.TablePrompt{ID: syncTestID("prompt"), Name: syncTestID("Customer prompt"), FolderID: &custFolder.ID}
	_ = store.CreatePrompt(ctx, subPrompt)
	_ = store.CreatePrompt(ctx, custPrompt)

	user := syncTestUser(t, store)
	syncTestJoin(t, store, team.ID, user.ID)
	if err := lifecycle.OnTeamMemberAdded(ctx, team.ID, user.ID); err != nil {
		t.Fatalf("member added: %v", err)
	}
	if !syncTestHasPrompt(t, store, user.ID, subPrompt.ID) || !syncTestHasPrompt(t, store, user.ID, custPrompt.ID) {
		t.Fatalf("new member should get team subfolder and customer-level prompts")
	}

	_ = store.DB().Where("team_id = ? AND user_id = ?", team.ID, user.ID).Delete(&tables.TableTeamMember{}).Error
	_ = lifecycle.OnTeamMemberRemoved(ctx, team.ID, user.ID)
	if syncTestHasPrompt(t, store, user.ID, subPrompt.ID) || syncTestHasPrompt(t, store, user.ID, custPrompt.ID) {
		t.Fatalf("removed member should lose the team's prompts")
	}
}
