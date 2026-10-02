package handlers

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/configstore/tables"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type lifecycleTestStore struct {
	configstore.ConfigStore
	db *gorm.DB
}

func (s *lifecycleTestStore) DB() *gorm.DB {
	return s.db
}

func (s *lifecycleTestStore) CreateTeam(ctx context.Context, team *tables.TableTeam, tx ...*gorm.DB) error {
	return s.db.WithContext(ctx).Create(team).Error
}

func (s *lifecycleTestStore) GetTeam(ctx context.Context, id string) (*tables.TableTeam, error) {
	var team tables.TableTeam
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&team).Error; err != nil {
		return nil, err
	}
	return &team, nil
}

func (s *lifecycleTestStore) CreateUser(ctx context.Context, user *tables.TableUser, tx ...*gorm.DB) error {
	return s.db.WithContext(ctx).Create(user).Error
}

func (s *lifecycleTestStore) GetUserByID(ctx context.Context, id string) (*tables.TableUser, error) {
	var user tables.TableUser
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *lifecycleTestStore) GetUserByUsername(ctx context.Context, username string) (*tables.TableUser, error) {
	var user tables.TableUser
	if err := s.db.WithContext(ctx).Where("username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *lifecycleTestStore) UpdateUser(ctx context.Context, user *tables.TableUser, tx ...*gorm.DB) error {
	return s.db.WithContext(ctx).Save(user).Error
}

func (s *lifecycleTestStore) CreateFolder(ctx context.Context, folder *tables.TableFolder) error {
	return s.db.WithContext(ctx).Create(folder).Error
}

func (s *lifecycleTestStore) GetFolderByID(ctx context.Context, id string) (*tables.TableFolder, error) {
	var folder tables.TableFolder
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&folder).Error; err != nil {
		return nil, err
	}
	return &folder, nil
}

func (s *lifecycleTestStore) CreatePrompt(ctx context.Context, prompt *tables.TablePrompt, tx ...*gorm.DB) error {
	return s.db.WithContext(ctx).Create(prompt).Error
}

func (s *lifecycleTestStore) CreatePromptSession(ctx context.Context, session *tables.TablePromptSession) error {
	return s.db.WithContext(ctx).Create(session).Error
}

func setupLifecycleTestStore(t *testing.T) *lifecycleTestStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_loc=auto"), &gorm.Config{})
	if err != nil {
		t.Skipf("sqlite unavailable: %v", err)
	}

	err = db.AutoMigrate(
		&tables.TableCustomer{},
		&tables.TableTeam{},
		&tables.TableTeamMember{},
		&tables.TableUser{},
		&tables.TableFolder{},
		&tables.TablePrompt{},
		&tables.TablePromptSession{},
	)
	if err != nil {
		t.Fatalf("failed to migrate tables: %v", err)
	}

	return &lifecycleTestStore{db: db}
}

func TestPromptLifecycle_ManualPromptInTeamFolder_AutoAssignsToTeamMembers(t *testing.T) {
	store := setupLifecycleTestStore(t)
	lifecycle := NewPromptLifecycleManager(store)
	ctx := context.Background()

	// 1. Create Team
	team := &tables.TableTeam{
		ID:        "team_dev_" + uuid.New().String()[:8],
		Name:      "Development Team",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := store.CreateTeam(ctx, team); err != nil {
		t.Fatalf("failed to create team: %v", err)
	}

	// 2. Create User (Alice)
	alice := &tables.TableUser{
		ID:                 "user_alice_" + uuid.New().String()[:8],
		Username:           "alice",
		Email:              "alice@example.com",
		Role:               "user",
		AllowedPromptRepos: "",
	}
	if err := store.CreateUser(ctx, alice); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// 3. Add Alice to Team
	if err := store.DB().Create(&tables.TableTeamMember{
		TeamID:    team.ID,
		UserID:    alice.ID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}).Error; err != nil {
		t.Fatalf("failed to link team member: %v", err)
	}

	// 4. Create Team Folder
	teamFolder := &tables.TableFolder{
		ID:        "folder_team_" + uuid.New().String()[:8],
		Name:      team.Name,
		Type:      "team",
		EntityID:  &team.ID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := store.CreateFolder(ctx, teamFolder); err != nil {
		t.Fatalf("failed to create team folder: %v", err)
	}

	// 5. Manually create a prompt inside this team folder
	manualPrompt := &tables.TablePrompt{
		ID:        "prompt_manual_" + uuid.New().String()[:8],
		Name:      "Shared Code Review Bot",
		FolderID:  &teamFolder.ID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := store.CreatePrompt(ctx, manualPrompt); err != nil {
		t.Fatalf("failed to create manual prompt: %v", err)
	}

	// Call OnPromptCreated
	if err := lifecycle.OnPromptCreated(ctx, manualPrompt, "admin"); err != nil {
		t.Fatalf("OnPromptCreated failed: %v", err)
	}

	// 6. Verify Alice has the manual prompt in AllowedPromptRepos
	refreshedAlice, err := store.GetUserByID(ctx, alice.ID)
	if err != nil {
		t.Fatalf("failed to get refreshed alice: %v", err)
	}

	if !strings.Contains(refreshedAlice.AllowedPromptRepos, manualPrompt.ID) {
		t.Fatalf("expected Alice to have prompt %s in AllowedPromptRepos, got %s", manualPrompt.ID, refreshedAlice.AllowedPromptRepos)
	}
}

func TestPromptLifecycle_NewTeamMember_GetsAlreadyExistingPrompts(t *testing.T) {
	store := setupLifecycleTestStore(t)
	lifecycle := NewPromptLifecycleManager(store)
	ctx := context.Background()

	// 1. Create Team
	team := &tables.TableTeam{
		ID:        "team_sec_" + uuid.New().String()[:8],
		Name:      "Security Team",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := store.CreateTeam(ctx, team); err != nil {
		t.Fatalf("failed to create team: %v", err)
	}

	// 2. Ensure Team Folder
	if err := lifecycle.OnTeamCreated(ctx, team); err != nil {
		t.Fatalf("failed in OnTeamCreated: %v", err)
	}

	var teamFolder tables.TableFolder
	if err := store.DB().Where("entity_id = ?", team.ID).First(&teamFolder).Error; err != nil {
		t.Fatalf("failed to locate team folder: %v", err)
	}

	// 3. Pre-existing prompt inside Security Team folder
	preExistingPrompt := &tables.TablePrompt{
		ID:        "prompt_pre_existing_" + uuid.New().String()[:8],
		Name:      "Incident Response Assistant",
		FolderID:  &teamFolder.ID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := store.CreatePrompt(ctx, preExistingPrompt); err != nil {
		t.Fatalf("failed to create pre-existing prompt: %v", err)
	}

	// 4. Bob joins Security Team
	bob := &tables.TableUser{
		ID:                 "user_bob_" + uuid.New().String()[:8],
		Username:           "bob",
		Email:              "bob@example.com",
		Role:               "user",
		AllowedPromptRepos: "",
	}
	if err := store.CreateUser(ctx, bob); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	if err := lifecycle.OnTeamMemberAdded(ctx, team.ID, bob.ID); err != nil {
		t.Fatalf("OnTeamMemberAdded failed: %v", err)
	}

	// 5. Verify Bob got the pre-existing prompt assigned
	refreshedBob, err := store.GetUserByID(ctx, bob.ID)
	if err != nil {
		t.Fatalf("failed to get refreshed bob: %v", err)
	}

	if !strings.Contains(refreshedBob.AllowedPromptRepos, preExistingPrompt.ID) {
		t.Fatalf("expected Bob to have pre-existing prompt %s in AllowedPromptRepos, got %s", preExistingPrompt.ID, refreshedBob.AllowedPromptRepos)
	}
}

func TestPromptLifecycle_CustomerDeleted_PreservesActiveTeams(t *testing.T) {
	store := setupLifecycleTestStore(t)
	lifecycle := NewPromptLifecycleManager(store)
	ctx := context.Background()

	// 1. Create Customer
	customer := &tables.TableCustomer{
		ID:        "cust_" + uuid.New().String()[:8],
		Name:      "Acme Corp",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := store.DB().Create(customer).Error; err != nil {
		t.Fatalf("failed to create customer: %v", err)
	}

	if err := lifecycle.OnCustomerCreated(ctx, customer); err != nil {
		t.Fatalf("OnCustomerCreated failed: %v", err)
	}

	var custFolder tables.TableFolder
	if err := store.DB().Where("entity_id = ? OR (name = ? AND type = 'customer')", customer.ID, customer.Name).First(&custFolder).Error; err != nil {
		t.Fatalf("failed to locate customer folder: %v", err)
	}

	// 2. Create child team folder inside customer folder
	teamFolder := &tables.TableFolder{
		ID:        uuid.New().String(),
		Name:      "Engineering",
		Type:      "team",
		ParentID:  &custFolder.ID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := store.CreateFolder(ctx, teamFolder); err != nil {
		t.Fatalf("failed to create child team folder: %v", err)
	}

	// 3. Delete customer
	if err := lifecycle.OnCustomerDeleted(ctx, customer); err != nil {
		t.Fatalf("OnCustomerDeleted failed: %v", err)
	}

	// 4. Verify Customer folder was moved to Removed Customers and renamed
	var updatedCustFolder tables.TableFolder
	if err := store.DB().Where("id = ?", custFolder.ID).First(&updatedCustFolder).Error; err != nil {
		t.Fatalf("failed to find updated customer folder: %v", err)
	}
	if updatedCustFolder.Type != "archived_customer" {
		t.Errorf("expected customer folder type 'archived_customer', got %s", updatedCustFolder.Type)
	}
	if updatedCustFolder.Name != "Acme Corp (Archived)" {
		t.Errorf("expected customer folder name 'Acme Corp (Archived)', got %s", updatedCustFolder.Name)
	}

	// 5. Verify child team folder was reparented to root Teams folder
	teamsRoot, err := lifecycle.EnsureSystemFolder(ctx, "Teams", "system_teams_root", nil)
	if err != nil {
		t.Fatalf("failed to get Teams root: %v", err)
	}

	var updatedTeamFolder tables.TableFolder
	if err := store.DB().Where("id = ?", teamFolder.ID).First(&updatedTeamFolder).Error; err != nil {
		t.Fatalf("failed to find updated team folder: %v", err)
	}
	if updatedTeamFolder.ParentID == nil || *updatedTeamFolder.ParentID != teamsRoot.ID {
		t.Errorf("expected child team folder to be reparented to Teams root (%s), got %v", teamsRoot.ID, updatedTeamFolder.ParentID)
	}
}

func TestPromptLifecycle_PromptFolderChanged(t *testing.T) {
	store := setupLifecycleTestStore(t)
	lifecycle := NewPromptLifecycleManager(store)
	ctx := context.Background()

	// 1. Create Team
	team := &tables.TableTeam{
		ID:        "team_dev_" + uuid.New().String()[:8],
		Name:      "DevOps",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_ = store.CreateTeam(ctx, team)
	_ = lifecycle.OnTeamCreated(ctx, team)

	var teamFolder tables.TableFolder
	_ = store.DB().Where("entity_id = ?", team.ID).First(&teamFolder)

	// 2. Member Charlie in DevOps
	charlie := &tables.TableUser{
		ID:       "user_charlie_" + uuid.New().String()[:8],
		Username: "charlie",
		Email:    "charlie@example.com",
	}
	_ = store.CreateUser(ctx, charlie)
	member := &tables.TableTeamMember{
		TeamID:    team.ID,
		UserID:    charlie.ID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_ = store.DB().Create(member)

	// 3. Prompt starts in general folder
	genFolder := &tables.TableFolder{
		ID:   uuid.New().String(),
		Name: "General",
		Type: "custom",
	}
	_ = store.CreateFolder(ctx, genFolder)

	prompt := &tables.TablePrompt{
		ID:       "prompt_k8s_" + uuid.New().String()[:8],
		Name:     "K8s Helper",
		FolderID: &genFolder.ID,
	}
	_ = store.CreatePrompt(ctx, prompt)

	// 4. Move prompt to DevOps team folder
	lifecycle.OnPromptFolderChanged(ctx, prompt.ID, teamFolder.ID)

	// 5. Verify Charlie gets allowed prompt repo updated
	refreshedCharlie, err := store.GetUserByID(ctx, charlie.ID)
	if err != nil {
		t.Fatalf("failed to get charlie: %v", err)
	}
	if !strings.Contains(refreshedCharlie.AllowedPromptRepos, prompt.ID) {
		t.Fatalf("expected charlie to have prompt %s in AllowedPromptRepos, got %s", prompt.ID, refreshedCharlie.AllowedPromptRepos)
	}
}

