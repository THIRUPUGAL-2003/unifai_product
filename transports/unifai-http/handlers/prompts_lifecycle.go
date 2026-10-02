package handlers

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/configstore/tables"
	"gorm.io/gorm"
)

// PromptLifecycleManager automates the prompt repository folder hierarchy,
// user prompt creation, team/customer folder organization, and 6-root non-destructive
// archival (Customers, Removed Customers, Teams, Removed Teams, Users, Removed Users).
type PromptLifecycleManager struct {
	store configstore.ConfigStore
	mu    sync.Mutex
}

// NewPromptLifecycleManager creates a new instance of PromptLifecycleManager
// and ensures all 6 system root folders exist.
func NewPromptLifecycleManager(store configstore.ConfigStore) *PromptLifecycleManager {
	if store == nil {
		return nil
	}
	mgr := &PromptLifecycleManager{store: store}
	if store.DB() != nil {
		go func() {
			_ = mgr.EnsureAllSystemRoots(context.Background())
		}()
	}
	return mgr
}

// EnsureAllSystemRoots guarantees that the 6 system root folders exist:
// 1. Customers (system_customers_root)
// 2. Removed Customers (system_removed_customers_root)
// 3. Teams (system_teams_root)
// 4. Removed Teams (system_removed_teams_root)
// 5. Users (system_users_root)
// 6. Removed Users (system_removed_users_root)
func (m *PromptLifecycleManager) EnsureAllSystemRoots(ctx context.Context) error {
	if m == nil || m.store == nil || m.store.DB() == nil {
		return nil
	}

	roots := []struct {
		name  string
		fType string
	}{
		{"Customers", "system_customers_root"},
		{"Removed Customers", "system_removed_customers_root"},
		{"Teams", "system_teams_root"},
		{"Removed Teams", "system_removed_teams_root"},
		{"Users", "system_users_root"},
		{"Removed Users", "system_removed_users_root"},
	}

	for _, r := range roots {
		if _, err := m.EnsureSystemFolder(ctx, r.name, r.fType, nil); err != nil {
			logger.Warn("PromptLifecycle: failed to ensure system root folder %s: %v", r.name, err)
		}
	}
	return nil
}

// EnsureSystemFolder ensures a system root or parent folder exists, creating it idempotently if missing.
func (m *PromptLifecycleManager) EnsureSystemFolder(ctx context.Context, name string, folderType string, parentID *string) (*tables.TableFolder, error) {
	if m == nil || m.store == nil || m.store.DB() == nil {
		return nil, errors.New("store not available")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	db := m.store.DB().WithContext(ctx)
	var folder tables.TableFolder

	query := db.Model(&tables.TableFolder{})
	if folderType != "" {
		query = query.Where("type = ? OR name = ?", folderType, name)
	} else {
		query = query.Where("name = ?", name)
	}

	if parentID != nil {
		query = query.Where("parent_id = ?", *parentID)
	} else {
		query = query.Where("parent_id IS NULL OR parent_id = ''")
	}

	err := query.First(&folder).Error
	if err == nil {
		if folderType != "" && folder.Type != folderType {
			folder.Type = folderType
			_ = db.Model(&folder).Update("type", folderType).Error
		}
		return &folder, nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	now := time.Now()
	newFolder := &tables.TableFolder{
		ID:        uuid.New().String(),
		Name:      name,
		Type:      folderType,
		ParentID:  parentID,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := m.store.CreateFolder(ctx, newFolder); err != nil {
		return nil, err
	}

	return newFolder, nil
}

// OnUserCreated is called whenever a new user is created.
// If autoCreatePrompt is true, it ensures the "Users" folder exists, creates a prompt named
// with the user's email/username, sets up a default playground session, and grants access.
// If autoCreatePrompt is false, prompt creation is skipped (prompt repository is optional).
func (m *PromptLifecycleManager) OnUserCreated(ctx context.Context, user *tables.TableUser, autoCreatePrompt bool) error {
	if m == nil || m.store == nil || m.store.DB() == nil || user == nil || !autoCreatePrompt {
		return nil
	}

	promptName := strings.TrimSpace(user.Email)
	if promptName == "" {
		promptName = strings.TrimSpace(user.Username)
	}
	if promptName == "" {
		return nil
	}

	usersFolder, err := m.EnsureSystemFolder(ctx, "Users", "system_users_root", nil)
	if err != nil {
		logger.Error("PromptLifecycle: failed to ensure 'Users' folder: %v", err)
		return err
	}

	db := m.store.DB().WithContext(ctx)

	// Check if prompt already exists in the Users folder
	var existing tables.TablePrompt
	err = db.Where("name = ? AND folder_id = ?", promptName, usersFolder.ID).First(&existing).Error
	if err == nil {
		m.ensureUserAllowedPrompt(ctx, user, existing.ID)
		return nil
	}

	// Create prompt
	now := time.Now()
	prompt := &tables.TablePrompt{
		ID:        uuid.New().String(),
		Name:      promptName,
		FolderID:  &usersFolder.ID,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := m.store.CreatePrompt(ctx, prompt); err != nil {
		logger.Error("PromptLifecycle: failed to create prompt for user %s: %v", promptName, err)
		return err
	}

	// Initialize default session with temperature and standard model parameters
	session := &tables.TablePromptSession{
		PromptID: prompt.ID,
		Name:     "Default Session",
		UserID:   user.ID,
		ModelParams: tables.ModelParams{
			"temperature": 0.7,
			"max_tokens":  2048,
			"top_p":       1.0,
			"stream":      true,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := m.store.CreatePromptSession(ctx, session); err != nil {
		logger.Warn("PromptLifecycle: failed to create default session: %v", err)
	}

	m.ensureUserAllowedPrompt(ctx, user, prompt.ID)
	return nil
}

// OnUserDeleted moves the user's prompt from "Users/" or any team folder to "Removed Users/" without destroying chat history.
func (m *PromptLifecycleManager) OnUserDeleted(ctx context.Context, user *tables.TableUser) error {
	if m == nil || m.store == nil || m.store.DB() == nil || user == nil {
		return nil
	}

	removedUsersFolder, err := m.EnsureSystemFolder(ctx, "Removed Users", "system_removed_users_root", nil)
	if err != nil {
		logger.Error("PromptLifecycle: failed to ensure 'Removed Users' folder: %v", err)
		return err
	}

	db := m.store.DB().WithContext(ctx)

	userIdentifiers := []string{}
	if user.Email != "" {
		userIdentifiers = append(userIdentifiers, user.Email)
	}
	if user.Username != "" {
		userIdentifiers = append(userIdentifiers, user.Username)
	}

	if len(userIdentifiers) > 0 {
		_ = db.Model(&tables.TablePrompt{}).
			Where("name IN ?", userIdentifiers).
			Update("folder_id", removedUsersFolder.ID).Error
	}

	return nil
}

// OnCustomerCreated creates a folder under "Customers/<customer_name>"
func (m *PromptLifecycleManager) OnCustomerCreated(ctx context.Context, customer *tables.TableCustomer) error {
	if m == nil || m.store == nil || m.store.DB() == nil || customer == nil {
		return nil
	}

	customersRoot, err := m.EnsureSystemFolder(ctx, "Customers", "system_customers_root", nil)
	if err != nil {
		logger.Error("PromptLifecycle: failed to ensure 'Customers' root folder: %v", err)
		return err
	}

	db := m.store.DB().WithContext(ctx)

	var existing tables.TableFolder
	err = db.Where("entity_id = ? OR (name = ? AND parent_id = ?)", customer.ID, customer.Name, customersRoot.ID).First(&existing).Error
	if err == nil {
		if existing.EntityID == nil || *existing.EntityID != customer.ID {
			_ = db.Model(&existing).Updates(map[string]any{"entity_id": customer.ID, "type": "customer"}).Error
		}
		return nil
	}

	now := time.Now()
	custFolder := &tables.TableFolder{
		ID:        uuid.New().String(),
		Name:      customer.Name,
		ParentID:  &customersRoot.ID,
		Type:      "customer",
		EntityID:  &customer.ID,
		CreatedAt: now,
		UpdatedAt: now,
	}

	return m.store.CreateFolder(ctx, custFolder)
}

// OnCustomerDeleted moves the customer folder from "Customers/" to "Removed Customers/"
// preserving all child teams, member prompts, sessions, and chat history.
func (m *PromptLifecycleManager) OnCustomerDeleted(ctx context.Context, customer *tables.TableCustomer) error {
	if m == nil || m.store == nil || m.store.DB() == nil || customer == nil {
		return nil
	}

	removedCustRoot, err := m.EnsureSystemFolder(ctx, "Removed Customers", "system_removed_customers_root", nil)
	if err != nil {
		logger.Error("PromptLifecycle: failed to ensure 'Removed Customers' folder: %v", err)
		return err
	}

	db := m.store.DB().WithContext(ctx)

	_ = db.Model(&tables.TableFolder{}).
		Where("entity_id = ? OR (name = ? AND type = 'customer')", customer.ID, customer.Name).
		Updates(map[string]any{
			"parent_id": removedCustRoot.ID,
			"type":      "archived_customer",
			"name":      customer.Name + " (Archived)",
		}).Error

	return nil
}

// OnTeamCreated creates a team subfolder under its customer folder if linked to a Customer,
// or under the root "Teams/" folder if standalone.
func (m *PromptLifecycleManager) OnTeamCreated(ctx context.Context, team *tables.TableTeam) error {
	if m == nil || m.store == nil || m.store.DB() == nil || team == nil {
		return nil
	}

	db := m.store.DB().WithContext(ctx)

	var parentFolderID *string
	if team.CustomerID != nil && *team.CustomerID != "" {
		// Locate parent customer folder
		var custFolder tables.TableFolder
		err := db.Where("entity_id = ? OR (type = 'customer' AND entity_id = ?)", *team.CustomerID, *team.CustomerID).First(&custFolder).Error
		if err != nil {
			customer, getErr := m.store.GetCustomer(ctx, *team.CustomerID)
			if getErr == nil && customer != nil {
				_ = m.OnCustomerCreated(ctx, customer)
				_ = db.Where("entity_id = ?", customer.ID).First(&custFolder).Error
			}
		}
		if custFolder.ID != "" {
			parentFolderID = &custFolder.ID
		}
	}

	// Standalone team (no customer) -> place under root "Teams" folder
	if parentFolderID == nil {
		teamsRoot, err := m.EnsureSystemFolder(ctx, "Teams", "system_teams_root", nil)
		if err == nil && teamsRoot != nil {
			parentFolderID = &teamsRoot.ID
		}
	}

	if parentFolderID == nil {
		return errors.New("parent folder not available")
	}

	// Check if team folder exists
	var existing tables.TableFolder
	err := db.Where("entity_id = ? OR (name = ? AND parent_id = ?)", team.ID, team.Name, *parentFolderID).First(&existing).Error
	if err == nil {
		updates := map[string]any{"entity_id": team.ID, "type": "team"}
		if existing.ParentID == nil || *existing.ParentID != *parentFolderID {
			updates["parent_id"] = *parentFolderID
		}
		_ = db.Model(&existing).Updates(updates).Error
		return nil
	}

	now := time.Now()
	teamFolder := &tables.TableFolder{
		ID:        uuid.New().String(),
		Name:      team.Name,
		ParentID:  parentFolderID,
		Type:      "team",
		EntityID:  &team.ID,
		CreatedAt: now,
		UpdatedAt: now,
	}

	return m.store.CreateFolder(ctx, teamFolder)
}

// OnTeamDeleted moves the team folder into "Removed Teams/" preserving all user prompts and chat histories.
func (m *PromptLifecycleManager) OnTeamDeleted(ctx context.Context, team *tables.TableTeam) error {
	if m == nil || m.store == nil || m.store.DB() == nil || team == nil {
		return nil
	}

	removedTeamsRoot, err := m.EnsureSystemFolder(ctx, "Removed Teams", "system_removed_teams_root", nil)
	if err != nil {
		logger.Error("PromptLifecycle: failed to ensure 'Removed Teams' folder: %v", err)
		return err
	}

	db := m.store.DB().WithContext(ctx)

	_ = db.Model(&tables.TableFolder{}).
		Where("entity_id = ? OR (name = ? AND type = 'team')", team.ID, team.Name).
		Updates(map[string]any{
			"parent_id": removedTeamsRoot.ID,
			"type":      "archived_team",
			"name":      team.Name + " (Archived)",
		}).Error

	return nil
}

// OnTeamMemberAdded creates a user prompt inside the team folder when a member is added to a team.
func (m *PromptLifecycleManager) OnTeamMemberAdded(ctx context.Context, teamID string, userID string) error {
	if m == nil || m.store == nil || m.store.DB() == nil || teamID == "" || userID == "" {
		return nil
	}

	team, err := m.store.GetTeam(ctx, teamID)
	if err != nil || team == nil {
		return err
	}

	user, err := m.store.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		return err
	}

	promptName := strings.TrimSpace(user.Email)
	if promptName == "" {
		promptName = strings.TrimSpace(user.Username)
	}
	if promptName == "" {
		return nil
	}

	db := m.store.DB().WithContext(ctx)

	// Ensure team folder exists (works for both customer-linked teams and standalone teams)
	var teamFolder tables.TableFolder
	err = db.Where("entity_id = ? OR (name = ? AND (type = 'team' OR type = 'archived_team'))", team.ID, team.Name).First(&teamFolder).Error
	if err != nil {
		_ = m.OnTeamCreated(ctx, team)
		_ = db.Where("entity_id = ?", team.ID).First(&teamFolder).Error
	}

	if teamFolder.ID == "" {
		return nil
	}

	// Check if prompt already exists in this team folder
	var existing tables.TablePrompt
	err = db.Where("name = ? AND folder_id = ?", promptName, teamFolder.ID).First(&existing).Error
	if err == nil {
		m.ensureUserAllowedPrompt(ctx, user, existing.ID)
		return nil
	}

	now := time.Now()
	prompt := &tables.TablePrompt{
		ID:        uuid.New().String(),
		Name:      promptName,
		FolderID:  &teamFolder.ID,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := m.store.CreatePrompt(ctx, prompt); err != nil {
		return err
	}

	// Default session for this prompt
	session := &tables.TablePromptSession{
		PromptID: prompt.ID,
		Name:     team.Name + " Session",
		UserID:   user.ID,
		ModelParams: tables.ModelParams{
			"temperature": 0.7,
			"max_tokens":  2048,
			"top_p":       1.0,
			"stream":      true,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	_ = m.store.CreatePromptSession(ctx, session)

	m.ensureUserAllowedPrompt(ctx, user, prompt.ID)
	return nil
}

// OnTeamMemberRemoved moves the member's prompt from the team folder to "Removed Users/"
// and revokes prompt repo access for this user.
func (m *PromptLifecycleManager) OnTeamMemberRemoved(ctx context.Context, teamID string, userID string) error {
	if m == nil || m.store == nil || m.store.DB() == nil || teamID == "" || userID == "" {
		return nil
	}

	team, err := m.store.GetTeam(ctx, teamID)
	if err != nil || team == nil {
		return err
	}

	user, err := m.store.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		return err
	}

	removedUsersFolder, err := m.EnsureSystemFolder(ctx, "Removed Users", "system_removed_users_root", nil)
	if err != nil {
		return err
	}

	db := m.store.DB().WithContext(ctx)

	// Locate team folder
	var teamFolder tables.TableFolder
	if err := db.Where("entity_id = ? OR (name = ? AND (type = 'team' OR type = 'archived_team'))", team.ID, team.Name).First(&teamFolder).Error; err != nil {
		return nil
	}

	userIdentifiers := []string{}
	if user.Email != "" {
		userIdentifiers = append(userIdentifiers, user.Email)
	}
	if user.Username != "" {
		userIdentifiers = append(userIdentifiers, user.Username)
	}

	if len(userIdentifiers) > 0 {
		var memberPrompts []tables.TablePrompt
		_ = db.Where("name IN ? AND folder_id = ?", userIdentifiers, teamFolder.ID).Find(&memberPrompts).Error

		for _, p := range memberPrompts {
			_ = db.Model(&p).Update("folder_id", removedUsersFolder.ID).Error
			m.removeUserAllowedPrompt(ctx, user, p.ID)
		}
	}

	return nil
}

// ensureUserAllowedPrompt appends promptID to user.AllowedPromptRepos if not already present.
func (m *PromptLifecycleManager) ensureUserAllowedPrompt(ctx context.Context, user *tables.TableUser, promptID string) {
	if user == nil || promptID == "" {
		return
	}
	repos := strings.Split(user.AllowedPromptRepos, ",")
	for _, r := range repos {
		if strings.TrimSpace(r) == promptID {
			return
		}
	}

	newRepos := strings.TrimSpace(user.AllowedPromptRepos)
	if newRepos == "" {
		newRepos = promptID
	} else {
		newRepos += "," + promptID
	}

	user.AllowedPromptRepos = newRepos
	_ = m.store.UpdateUser(ctx, user)
}

// removeUserAllowedPrompt removes promptID from user.AllowedPromptRepos.
func (m *PromptLifecycleManager) removeUserAllowedPrompt(ctx context.Context, user *tables.TableUser, promptID string) {
	if user == nil || promptID == "" || user.AllowedPromptRepos == "" {
		return
	}
	repos := strings.Split(user.AllowedPromptRepos, ",")
	filtered := make([]string, 0, len(repos))
	for _, r := range repos {
		t := strings.TrimSpace(r)
		if t != "" && t != promptID {
			filtered = append(filtered, t)
		}
	}
	user.AllowedPromptRepos = strings.Join(filtered, ",")
	_ = m.store.UpdateUser(ctx, user)
}
