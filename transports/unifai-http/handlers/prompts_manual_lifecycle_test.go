package handlers

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/configstore/tables"
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

func TestPromptDeletion_UnassignsFromUsers(t *testing.T) {
	store := setupLifecycleTestStore(t)
	h := &PromptsHandler{store: store}
	ctx := context.Background()

	// 1. Create prompt
	promptID := "prompt_del_" + uuid.New().String()[:8]
	prompt := &tables.TablePrompt{
		ID:   promptID,
		Name: "Confidential Prompt",
	}
	_ = store.CreatePrompt(ctx, prompt)

	// 2. User Dave with promptID assigned
	dave := &tables.TableUser{
		ID:                 "user_dave_" + uuid.New().String()[:8],
		Username:           "dave",
		Email:              "dave@example.com",
		AllowedPromptRepos: promptID + ",other_prompt_123",
	}
	_ = store.CreateUser(ctx, dave)

	// 3. Unassign on deletion
	h.unassignPromptFromAllUsers(ctx, promptID)

	// 4. Verify Dave no longer has promptID assigned
	refreshedDave, err := store.GetUserByID(ctx, dave.ID)
	if err != nil {
		t.Fatalf("failed to get dave: %v", err)
	}
	if strings.Contains(refreshedDave.AllowedPromptRepos, promptID) {
		t.Fatalf("expected prompt %s to be removed from Dave's AllowedPromptRepos, got %s", promptID, refreshedDave.AllowedPromptRepos)
	}
	if !strings.Contains(refreshedDave.AllowedPromptRepos, "other_prompt_123") {
		t.Fatalf("expected Dave to retain other_prompt_123, got %s", refreshedDave.AllowedPromptRepos)
	}
}

func TestPromptDrag_UpdatesUserTeamMembership(t *testing.T) {
	store := setupLifecycleTestStore(t)
	lifecycle := NewPromptLifecycleManager(store)
	ctx := context.Background()

	// 1. Create Customer A and Team A
	custA := &tables.TableCustomer{
		ID:   "cust_a_" + uuid.New().String()[:8],
		Name: "Customer Alpha",
	}
	_ = store.DB().Create(custA).Error

	teamA := &tables.TableTeam{
		ID:         "team_a_" + uuid.New().String()[:8],
		Name:       "Alpha Support Team",
		CustomerID: &custA.ID,
	}
	_ = store.CreateTeam(ctx, teamA)

	teamAFolder := &tables.TableFolder{
		ID:       uuid.New().String(),
		Name:     teamA.Name,
		Type:     "team",
		EntityID: &teamA.ID,
	}
	_ = store.CreateFolder(ctx, teamAFolder)

	// 2. Create Customer B and Team B
	custB := &tables.TableCustomer{
		ID:   "cust_b_" + uuid.New().String()[:8],
		Name: "Customer Beta",
	}
	_ = store.DB().Create(custB).Error

	teamB := &tables.TableTeam{
		ID:         "team_b_" + uuid.New().String()[:8],
		Name:       "Beta Engineering Team",
		CustomerID: &custB.ID,
	}
	_ = store.CreateTeam(ctx, teamB)

	teamBFolder := &tables.TableFolder{
		ID:       uuid.New().String(),
		Name:     teamB.Name,
		Type:     "team",
		EntityID: &teamB.ID,
	}
	_ = store.CreateFolder(ctx, teamBFolder)

	// Subfolder under Team B
	teamBSubfolder := &tables.TableFolder{
		ID:       uuid.New().String(),
		Name:     "Project Phoenix",
		Type:     "custom",
		ParentID: &teamBFolder.ID,
	}
	_ = store.CreateFolder(ctx, teamBSubfolder)

	// 3. Create User Eve
	eve := &tables.TableUser{
		ID:       "user_eve_" + uuid.New().String()[:8],
		Username: "eve",
		Email:    "eve@example.com",
	}
	_ = store.CreateUser(ctx, eve)

	// Initially Eve is member of Team A
	now := time.Now().UTC()
	_ = store.DB().Create(&tables.TableTeamMember{
		TeamID:    teamA.ID,
		UserID:    eve.ID,
		CreatedAt: now,
		UpdatedAt: now,
	}).Error

	// Create Eve's prompt inside Team A's folder
	evePrompt := &tables.TablePrompt{
		ID:       "prompt_eve_" + uuid.New().String()[:8],
		Name:     eve.Email,
		FolderID: &teamAFolder.ID,
	}
	_ = store.CreatePrompt(ctx, evePrompt)

	// Verify Eve is currently in Team A
	var memberCount int64
	store.DB().Model(&tables.TableTeamMember{}).Where("team_id = ? AND user_id = ?", teamA.ID, eve.ID).Count(&memberCount)
	if memberCount != 1 {
		t.Fatalf("expected Eve to be in Team A, got count %d", memberCount)
	}

	// 4. Drag Eve's prompt into Team B Subfolder (nested inside Team B)
	lifecycle.OnPromptFolderChanged(ctx, evePrompt.ID, teamBSubfolder.ID)

	// Verify Eve is moved from Team A to Team B
	store.DB().Model(&tables.TableTeamMember{}).Where("team_id = ? AND user_id = ?", teamA.ID, eve.ID).Count(&memberCount)
	if memberCount != 0 {
		t.Fatalf("expected Eve to be removed from Team A, got count %d", memberCount)
	}
	store.DB().Model(&tables.TableTeamMember{}).Where("team_id = ? AND user_id = ?", teamB.ID, eve.ID).Count(&memberCount)
	if memberCount != 1 {
		t.Fatalf("expected Eve to be added to Team B, got count %d", memberCount)
	}

	// 5. Drag Eve's prompt to Root (empty folder / out of teams)
	lifecycle.OnPromptFolderChanged(ctx, evePrompt.ID, "")

	// Verify Eve is now standalone (no team memberships)
	store.DB().Model(&tables.TableTeamMember{}).Where("user_id = ?", eve.ID).Count(&memberCount)
	if memberCount != 0 {
		t.Fatalf("expected Eve to have no team memberships after moving to root, got count %d", memberCount)
	}
}

func TestUserTeamChange_MovesPromptToNewTeamFolder(t *testing.T) {
	store := setupLifecycleTestStore(t)
	lifecycle := NewPromptLifecycleManager(store)
	ctx := context.Background()

	// 1. Create Team A and Team B
	teamA := &tables.TableTeam{
		ID:   "team_a_" + uuid.New().String()[:8],
		Name: "Frontend Team",
	}
	_ = store.CreateTeam(ctx, teamA)
	teamAFolder := &tables.TableFolder{
		ID:       uuid.New().String(),
		Name:     teamA.Name,
		Type:     "team",
		EntityID: &teamA.ID,
	}
	_ = store.CreateFolder(ctx, teamAFolder)

	teamB := &tables.TableTeam{
		ID:   "team_b_" + uuid.New().String()[:8],
		Name: "Backend Team",
	}
	_ = store.CreateTeam(ctx, teamB)
	teamBFolder := &tables.TableFolder{
		ID:       uuid.New().String(),
		Name:     teamB.Name,
		Type:     "team",
		EntityID: &teamB.ID,
	}
	_ = store.CreateFolder(ctx, teamBFolder)

	// Users root folder
	usersFolder, _ := lifecycle.EnsureSystemFolder(ctx, "Users", "system_users_root", nil)

	// 2. Create User Frank with prompt in Users folder
	frank := &tables.TableUser{
		ID:       "user_frank_" + uuid.New().String()[:8],
		Username: "frank",
		Email:    "frank@example.com",
	}
	_ = store.CreateUser(ctx, frank)

	frankPrompt := &tables.TablePrompt{
		ID:          "prompt_frank_" + uuid.New().String()[:8],
		Name:        frank.Email,
		FolderID:    &usersFolder.ID,
		OwnerUserID: &frank.ID,
	}
	_ = store.CreatePrompt(ctx, frankPrompt)

	// 3. Admin assigns Frank to Team A in Users page
	err := lifecycle.OnTeamMemberAdded(ctx, teamA.ID, frank.ID)
	if err != nil {
		t.Fatalf("unexpected error adding Frank to Team A: %v", err)
	}

	// Verify Frank's prompt moved to Team A folder
	var promptAfterA tables.TablePrompt
	_ = store.DB().Where("id = ?", frankPrompt.ID).First(&promptAfterA)
	if promptAfterA.FolderID == nil || *promptAfterA.FolderID != teamAFolder.ID {
		t.Fatalf("expected Frank's prompt to move to Team A folder %s, got %v", teamAFolder.ID, promptAfterA.FolderID)
	}

	// 4. Admin moves Frank to Team B in Users page
	_ = lifecycle.OnTeamMemberRemoved(ctx, teamA.ID, frank.ID)
	_ = lifecycle.OnTeamMemberAdded(ctx, teamB.ID, frank.ID)

	// Verify Frank's prompt moved to Team B folder
	var promptAfterB tables.TablePrompt
	_ = store.DB().Where("id = ?", frankPrompt.ID).First(&promptAfterB)
	if promptAfterB.FolderID == nil || *promptAfterB.FolderID != teamBFolder.ID {
		t.Fatalf("expected Frank's prompt to move to Team B folder %s, got %v", teamBFolder.ID, promptAfterB.FolderID)
	}

	// 5. Admin unassigns Frank to standalone in Users page
	_ = lifecycle.OnTeamMemberRemoved(ctx, teamB.ID, frank.ID)

	// Verify Frank's prompt moved back to Users root folder
	var promptAfterStandalone tables.TablePrompt
	_ = store.DB().Where("id = ?", frankPrompt.ID).First(&promptAfterStandalone)
	if promptAfterStandalone.FolderID == nil || *promptAfterStandalone.FolderID != usersFolder.ID {
		t.Fatalf("expected Frank's prompt to move to Users folder %s, got %v", usersFolder.ID, promptAfterStandalone.FolderID)
	}
}

func TestPromptLifecycle_UserApprovalFlow(t *testing.T) {
	store := setupLifecycleTestStore(t)
	lifecycle := NewPromptLifecycleManager(store)
	ctx := context.Background()

	// 1. Pending user self-registers (no prompt repo created initially)
	grace := &tables.TableUser{
		ID:       "user_grace_" + uuid.New().String()[:8],
		Username: "grace",
		Email:    "grace@company.com",
		Role:     "user",
		Status:   tables.UserStatusPending,
	}
	_ = store.CreateUser(ctx, grace)

	var promptCount int64
	store.DB().Model(&tables.TablePrompt{}).Where("name = ?", grace.Email).Count(&promptCount)
	if promptCount != 0 {
		t.Fatalf("expected 0 prompts for pending user, got %d", promptCount)
	}

	// 2. Admin approves user -> OnUserCreated called
	grace.Status = tables.UserStatusApproved
	_ = store.UpdateUser(ctx, grace)
	err := lifecycle.OnUserCreated(ctx, grace, true)
	if err != nil {
		t.Fatalf("OnUserCreated failed: %v", err)
	}

	// Verify prompt was created in Users system folder
	var gracePrompt tables.TablePrompt
	err = store.DB().Where("name = ?", grace.Email).First(&gracePrompt).Error
	if err != nil {
		t.Fatalf("expected prompt for approved user Grace, got error: %v", err)
	}

	var usersFolder tables.TableFolder
	_ = store.DB().Where("type = 'system_users_root' OR name = 'Users'").First(&usersFolder)
	if gracePrompt.FolderID == nil || *gracePrompt.FolderID != usersFolder.ID {
		t.Fatalf("expected Grace's prompt to be in Users folder %s, got %v", usersFolder.ID, gracePrompt.FolderID)
	}

	// Verify user permissions
	updatedGrace, _ := store.GetUserByID(ctx, grace.ID)
	if !strings.Contains(updatedGrace.AllowedPromptRepos, gracePrompt.ID) {
		t.Fatalf("expected Grace AllowedPromptRepos to contain %s, got %s", gracePrompt.ID, updatedGrace.AllowedPromptRepos)
	}

	// 3. User logs in -> idempotent check should not duplicate prompt
	_ = lifecycle.OnUserCreated(ctx, updatedGrace, true)
	store.DB().Model(&tables.TablePrompt{}).Where("name = ?", grace.Email).Count(&promptCount)
	if promptCount != 1 {
		t.Fatalf("expected exactly 1 prompt after login, got %d", promptCount)
	}
}

func TestPromptLifecycle_SCIMUserAndGroupFlow(t *testing.T) {
	store := setupLifecycleTestStore(t)
	lifecycle := NewPromptLifecycleManager(store)
	ctx := context.Background()

	// 1. SCIM provisions user Hank
	hank := &tables.TableUser{
		ID:       "user_hank_" + uuid.New().String()[:8],
		Username: "hank",
		Email:    "hank@enterprise.com",
		Role:     "user",
		Status:   tables.UserStatusApproved,
	}
	_ = store.CreateUser(ctx, hank)
	err := lifecycle.OnUserCreated(ctx, hank, true)
	if err != nil {
		t.Fatalf("SCIM OnUserCreated failed: %v", err)
	}

	var hankPrompt tables.TablePrompt
	err = store.DB().Where("name = ?", hank.Email).First(&hankPrompt).Error
	if err != nil {
		t.Fatalf("expected SCIM user prompt created: %v", err)
	}

	// 2. SCIM provisions group "Engineering"
	engTeam := &tables.TableTeam{
		ID:   "team_eng_" + uuid.New().String()[:8],
		Name: "Engineering",
	}
	_ = store.CreateTeam(ctx, engTeam)
	err = lifecycle.OnTeamCreated(ctx, engTeam)
	if err != nil {
		t.Fatalf("SCIM OnTeamCreated failed: %v", err)
	}

	var engFolder tables.TableFolder
	err = store.DB().Where("entity_id = ?", engTeam.ID).First(&engFolder).Error
	if err != nil || engFolder.ID == "" {
		t.Fatalf("expected Engineering team folder created in prompt repo: %v", err)
	}

	// 3. SCIM adds Hank as a member of Engineering
	err = lifecycle.OnTeamMemberAdded(ctx, engTeam.ID, hank.ID)
	if err != nil {
		t.Fatalf("SCIM OnTeamMemberAdded failed: %v", err)
	}

	// Hank's prompt should have moved into the Engineering folder
	var promptAfterAdd tables.TablePrompt
	_ = store.DB().Where("id = ?", hankPrompt.ID).First(&promptAfterAdd)
	if promptAfterAdd.FolderID == nil || *promptAfterAdd.FolderID != engFolder.ID {
		t.Fatalf("expected Hank's prompt to move to Engineering folder %s, got %v", engFolder.ID, promptAfterAdd.FolderID)
	}

	// 4. SCIM removes Hank from Engineering
	err = lifecycle.OnTeamMemberRemoved(ctx, engTeam.ID, hank.ID)
	if err != nil {
		t.Fatalf("SCIM OnTeamMemberRemoved failed: %v", err)
	}

	// Hank's prompt should move back to Users root folder
	var usersFolder tables.TableFolder
	_ = store.DB().Where("type = 'system_users_root' OR name = 'Users'").First(&usersFolder)
	var promptAfterRemove tables.TablePrompt
	_ = store.DB().Where("id = ?", hankPrompt.ID).First(&promptAfterRemove)
	if promptAfterRemove.FolderID == nil || *promptAfterRemove.FolderID != usersFolder.ID {
		t.Fatalf("expected Hank's prompt to return to Users folder %s, got %v", usersFolder.ID, promptAfterRemove.FolderID)
	}
}
