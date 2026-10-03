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
}

var (
	// lifecycleFolderMu serializes system-folder creation across every manager instance (the
	// session, governance, workspace and prompts handlers each construct one).
	lifecycleFolderMu sync.Mutex
	// lifecycleStartup runs root creation + duplicate repair once per store.
	lifecycleStartup sync.Map
)

// NewPromptLifecycleManager creates a new instance of PromptLifecycleManager
// and ensures all 6 system root folders exist.
func NewPromptLifecycleManager(store configstore.ConfigStore) *PromptLifecycleManager {
	if store == nil {
		return nil
	}
	mgr := &PromptLifecycleManager{store: store}
	if store.DB() != nil {
		once, _ := lifecycleStartup.LoadOrStore(store, &sync.Once{})
		go once.(*sync.Once).Do(func() {
			ctx := context.Background()
			_ = mgr.EnsureAllSystemRoots(ctx)
			mgr.RepairDuplicateFolders(ctx)
		})
	}
	return mgr
}

// RepairDuplicateFolders folds folders that were created twice (earlier concurrent startups
// raced on the system roots) into the oldest copy: duplicate system roots, and duplicate
// team/customer folders for the same entity. Children and prompts move; nothing is deleted
// until it is empty.
func (m *PromptLifecycleManager) RepairDuplicateFolders(ctx context.Context) {
	if m == nil || m.store == nil || m.store.DB() == nil {
		return
	}
	lifecycleFolderMu.Lock()
	defer lifecycleFolderMu.Unlock()
	db := m.store.DB().WithContext(ctx)

	var candidates []tables.TableFolder
	if err := db.Where("type LIKE ? AND (parent_id IS NULL OR parent_id = '')", "system%").
		Order("created_at asc, id asc").Find(&candidates).Error; err == nil {
		roots := candidates[:0]
		for _, f := range candidates {
			if strings.HasPrefix(f.Type, "system_") {
				roots = append(roots, f)
			}
		}
		mergeDuplicateFolders(db, roots, func(f tables.TableFolder) string { return f.Type })
	}

	var entityFolders []tables.TableFolder
	if err := db.Where("type IN ? AND entity_id IS NOT NULL AND entity_id <> ''", []string{"team", "customer"}).
		Order("created_at asc, id asc").Find(&entityFolders).Error; err == nil {
		mergeDuplicateFolders(db, entityFolders, func(f tables.TableFolder) string { return f.Type + "/" + *f.EntityID })
	}
}

func mergeDuplicateFolders(db *gorm.DB, folders []tables.TableFolder, key func(tables.TableFolder) string) {
	keep := map[string]string{}
	for _, f := range folders {
		k := key(f)
		keepID, ok := keep[k]
		if !ok {
			keep[k] = f.ID
			continue
		}
		if err := db.Model(&tables.TableFolder{}).Where("parent_id = ?", f.ID).Update("parent_id", keepID).Error; err != nil {
			if logger != nil {
				logger.Warn("PromptLifecycle: failed to merge duplicate folder %s: %v", f.ID, err)
			}
			continue
		}
		if err := db.Model(&tables.TablePrompt{}).Where("folder_id = ?", f.ID).Update("folder_id", keepID).Error; err != nil {
			if logger != nil {
				logger.Warn("PromptLifecycle: failed to merge duplicate folder %s: %v", f.ID, err)
			}
			continue
		}
		var remaining int64
		db.Model(&tables.TableFolder{}).Where("parent_id = ?", f.ID).Count(&remaining)
		var prompts int64
		db.Model(&tables.TablePrompt{}).Where("folder_id = ?", f.ID).Count(&prompts)
		if remaining == 0 && prompts == 0 {
			_ = db.Where("id = ?", f.ID).Delete(&tables.TableFolder{}).Error
			if logger != nil {
				logger.Info("PromptLifecycle: merged duplicate folder %q (%s) into %s", f.Name, f.ID, keepID)
			}
		}
	}
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

	lifecycleFolderMu.Lock()
	defer lifecycleFolderMu.Unlock()

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

	err := query.Order("created_at asc, id asc").First(&folder).Error
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
// An existing prompt of a re-created user is brought back out of "Removed Users".
func (m *PromptLifecycleManager) OnUserCreated(ctx context.Context, user *tables.TableUser, autoCreatePrompt bool) error {
	if m == nil || m.store == nil || m.store.DB() == nil || user == nil {
		return nil
	}

	promptName := strings.TrimSpace(user.Email)
	if promptName == "" {
		promptName = strings.TrimSpace(user.Username)
	}
	if promptName == "" {
		return nil
	}

	db := m.store.DB().WithContext(ctx)

	// Matching is case-insensitive on email or username, so a differently cased email or a
	// prompt still named after the username never produces a second prompt for the same user.
	if existing := m.findUserPrompt(ctx, user); existing != nil {
		if existing.FolderID != nil && m.folderIsArchived(ctx, *existing.FolderID) {
			if home := m.userHomeFolderID(ctx, user); home != "" {
				_ = db.Model(&tables.TablePrompt{}).Where("id = ?", existing.ID).Update("folder_id", home).Error
			}
		}
		m.ensureUserAllowedPrompt(ctx, user, existing.ID)
		return nil
	}
	if !autoCreatePrompt {
		return nil
	}

	home := m.userHomeFolderID(ctx, user)
	if home == "" {
		return errors.New("users folder not available")
	}

	// Create prompt
	now := time.Now()
	prompt := &tables.TablePrompt{
		ID:        uuid.New().String(),
		Name:      promptName,
		FolderID:  &home,
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

// OnUserLogin keeps the user's access to their own active prompt. It never creates a prompt
// (that is the optional "auto-create" choice made when the user is created) and never pulls
// a prompt an admin deleted back out of "Removed Users".
func (m *PromptLifecycleManager) OnUserLogin(ctx context.Context, user *tables.TableUser) {
	if m == nil || m.store == nil || m.store.DB() == nil || user == nil {
		return
	}
	if existing := m.findUserPrompt(ctx, user); existing != nil {
		if existing.FolderID == nil || !m.folderIsArchived(ctx, *existing.FolderID) {
			m.ensureUserAllowedPrompt(ctx, user, existing.ID)
		}
	}
}

// findUserPrompt returns the user's own prompt (named after the user's email or username,
// case-insensitive), preferring one that is not archived.
func (m *PromptLifecycleManager) findUserPrompt(ctx context.Context, user *tables.TableUser) *tables.TablePrompt {
	names := lowerNames(userPromptNames(user))
	if len(names) == 0 {
		return nil
	}
	var prompts []tables.TablePrompt
	if err := m.store.DB().WithContext(ctx).Where("LOWER(name) IN ?", names).Order("created_at asc").Find(&prompts).Error; err != nil || len(prompts) == 0 {
		return nil
	}
	for i := range prompts {
		if prompts[i].FolderID == nil || !m.folderIsArchived(ctx, *prompts[i].FolderID) {
			return &prompts[i]
		}
	}
	return &prompts[0]
}

// userHomeFolderID is where the user's own prompt belongs: their first team's folder, else "Users/".
func (m *PromptLifecycleManager) userHomeFolderID(ctx context.Context, user *tables.TableUser) string {
	db := m.store.DB().WithContext(ctx)
	var memberships []tables.TableTeamMember
	_ = db.Where("user_id = ?", user.ID).Order("created_at asc").Find(&memberships).Error
	for _, ms := range memberships {
		if f, err := findEntityFolder(db, ms.TeamID, "", "team"); err == nil {
			return f.ID
		}
	}
	usersFolder, err := m.EnsureSystemFolder(ctx, "Users", "system_users_root", nil)
	if err != nil || usersFolder == nil {
		logger.Error("PromptLifecycle: failed to ensure 'Users' folder: %v", err)
		return ""
	}
	return usersFolder.ID
}

// folderIsArchived reports whether the folder is a "Removed ..." root, an archived folder, or inside one.
func (m *PromptLifecycleManager) folderIsArchived(ctx context.Context, folderID string) bool {
	db := m.store.DB().WithContext(ctx)
	currID := folderID
	for i := 0; i < maxFolderDepth && currID != ""; i++ {
		var f tables.TableFolder
		if err := db.Where("id = ?", currID).First(&f).Error; err != nil {
			return false
		}
		if strings.HasPrefix(f.Type, "system_removed_") || strings.HasPrefix(f.Type, "archived_") {
			return true
		}
		if f.ParentID == nil {
			return false
		}
		currID = *f.ParentID
	}
	return false
}

func lowerNames(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, strings.ToLower(n))
	}
	return out
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

	names := lowerNames(userPromptNames(user))
	if len(names) == 0 {
		return nil
	}
	var prompts []tables.TablePrompt
	_ = db.Where("LOWER(name) IN ?", names).Find(&prompts).Error
	for _, p := range prompts {
		// Teammates only had the prompt through its team folder; it must not stay visible to them.
		if p.FolderID != nil && *p.FolderID != removedUsersFolder.ID {
			m.revokePromptFromFolderAudience(ctx, p.ID, *p.FolderID, removedUsersFolder.ID)
		}
		_ = db.Model(&tables.TablePrompt{}).Where("id = ?", p.ID).Update("folder_id", removedUsersFolder.ID).Error
	}
	return nil
}

// OnUserUpdated renames the user's prompt when their email or username is modified.
func (m *PromptLifecycleManager) OnUserUpdated(ctx context.Context, user *tables.TableUser, oldEmail string, oldUsername string) error {
	if m == nil || m.store == nil || m.store.DB() == nil || user == nil {
		return nil
	}
	newPromptName := strings.TrimSpace(user.Email)
	if newPromptName == "" {
		newPromptName = strings.TrimSpace(user.Username)
	}
	if newPromptName == "" {
		return nil
	}

	db := m.store.DB().WithContext(ctx)

	newLower := strings.ToLower(newPromptName)
	oldIdentifiers := []string{}
	for _, old := range []string{strings.TrimSpace(oldEmail), strings.TrimSpace(oldUsername)} {
		if old != "" && strings.ToLower(old) != newLower {
			oldIdentifiers = append(oldIdentifiers, strings.ToLower(old))
		}
	}
	if len(oldIdentifiers) == 0 {
		return nil
	}

	// The user's prompt already carries the new name (e.g. only the username changed while the
	// prompt is named after the email): renaming another prompt would create a duplicate.
	var already int64
	db.Model(&tables.TablePrompt{}).Where("LOWER(name) = ?", newLower).Count(&already)
	if already > 0 {
		return nil
	}
	var own tables.TablePrompt
	if err := db.Where("LOWER(name) IN ?", oldIdentifiers).Order("created_at asc").First(&own).Error; err != nil {
		return nil
	}
	return db.Model(&tables.TablePrompt{}).Where("id = ?", own.ID).Update("name", newPromptName).Error
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
	if custFolder, err := findEntityFolder(db, customer.ID, customer.Name, "customer"); err == nil {
		// Customer-level prompts are archived with the customer: withdraw them from the members
		// of the customer's teams, who only had them through the customer.
		if teams := m.folderAudienceTeams(ctx, custFolder.ID); len(teams) > 0 {
			var prompts []tables.TablePrompt
			_ = db.Select("id").Where("folder_id IN ?", m.audienceFolderIDs(ctx, custFolder.ID)).Find(&prompts).Error
			for _, p := range prompts {
				m.revokePromptFromTeams(ctx, p.ID, teams, nil)
			}
		}
		// Reparent active child teams to root "Teams" folder so active teams aren't trapped in Removed Customers
		teamsRoot, tErr := m.EnsureSystemFolder(ctx, "Teams", "system_teams_root", nil)
		if tErr == nil && teamsRoot != nil {
			_ = db.Model(&tables.TableFolder{}).
				Where("parent_id = ? AND type = 'team'", custFolder.ID).
				Update("parent_id", teamsRoot.ID).Error
		}

		_ = db.Model(custFolder).
			Updates(map[string]any{
				"parent_id": removedCustRoot.ID,
				"type":      "archived_customer",
				"name":      customer.Name + " (Archived)",
			}).Error
	}

	return nil
}

// OnCustomerUpdated renames the customer folder if the customer's name was changed.
func (m *PromptLifecycleManager) OnCustomerUpdated(ctx context.Context, customer *tables.TableCustomer, oldName string) error {
	if m == nil || m.store == nil || m.store.DB() == nil || customer == nil {
		return nil
	}
	db := m.store.DB().WithContext(ctx)

	folder, err := findEntityFolder(db, customer.ID, oldName, "customer")
	if err != nil {
		return m.OnCustomerCreated(ctx, customer)
	}

	if folder.Name != customer.Name {
		return db.Model(folder).Updates(map[string]any{
			"name":      customer.Name,
			"entity_id": customer.ID,
			"type":      "customer",
		}).Error
	}
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

	teamFolder, err := findEntityFolder(db, team.ID, team.Name, "team")
	if err != nil {
		return nil
	}
	// Called before the team row (and its memberships) is deleted: treat every member as
	// leaving the team so their own prompts move out instead of being archived with it.
	var members []tables.TableTeamMember
	_ = db.Where("team_id = ?", team.ID).Find(&members).Error
	for _, member := range members {
		if user, uErr := m.store.GetUserByID(ctx, member.UserID); uErr == nil && user != nil {
			m.memberLeftTeam(ctx, team.ID, teamFolder, user)
		}
	}

	_ = db.Model(teamFolder).
		Updates(map[string]any{
			"parent_id": removedTeamsRoot.ID,
			"type":      "archived_team",
			"name":      team.Name + " (Archived)",
		}).Error

	return nil
}

// OnTeamUpdated updates the team folder name and reparents the folder if team was moved to a new Customer or made Standalone.
func (m *PromptLifecycleManager) OnTeamUpdated(ctx context.Context, team *tables.TableTeam, oldName string, oldCustomerID *string) error {
	if m == nil || m.store == nil || m.store.DB() == nil || team == nil {
		return nil
	}
	db := m.store.DB().WithContext(ctx)

	// Determine new parent folder
	var newParentFolderID *string
	if team.CustomerID != nil && *team.CustomerID != "" {
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
			newParentFolderID = &custFolder.ID
		}
	}

	// Standalone team (no customer) -> place under root "Teams" folder
	if newParentFolderID == nil {
		teamsRoot, err := m.EnsureSystemFolder(ctx, "Teams", "system_teams_root", nil)
		if err == nil && teamsRoot != nil {
			newParentFolderID = &teamsRoot.ID
		}
	}

	// Find the team folder by entity_id or oldName
	teamFolder, err := findEntityFolder(db, team.ID, oldName, "team")
	if err != nil {
		return m.OnTeamCreated(ctx, team)
	}

	updates := map[string]any{
		"name":      team.Name,
		"entity_id": team.ID,
		"type":      "team",
	}
	if newParentFolderID != nil && (teamFolder.ParentID == nil || *teamFolder.ParentID != *newParentFolderID) {
		updates["parent_id"] = *newParentFolderID
	}

	if err := db.Model(teamFolder).Updates(updates).Error; err != nil {
		return err
	}
	if derefID(oldCustomerID) != derefID(team.CustomerID) {
		m.resyncTeamCustomerChange(ctx, team.ID, derefID(oldCustomerID))
	}
	return nil
}

func derefID(id *string) string {
	if id == nil {
		return ""
	}
	return *id
}

// resyncTeamCustomerChange moves the members' customer-level grants from the team's previous
// customer to its current one.
func (m *PromptLifecycleManager) resyncTeamCustomerChange(ctx context.Context, teamID, oldCustomerID string) {
	db := m.store.DB().WithContext(ctx)
	var oldCustomerPrompts []string
	if oldCustomerID != "" {
		if f, err := findEntityFolder(db, oldCustomerID, "", "customer"); err == nil {
			var prompts []tables.TablePrompt
			_ = db.Select("id").Where("folder_id IN ?", m.audienceFolderIDs(ctx, f.ID)).Find(&prompts).Error
			for _, p := range prompts {
				oldCustomerPrompts = append(oldCustomerPrompts, p.ID)
			}
		}
	}
	teamPrompts := m.teamPromptIDs(ctx, teamID)

	var members []tables.TableTeamMember
	_ = db.Where("team_id = ?", teamID).Find(&members).Error
	for _, member := range members {
		user, err := m.store.GetUserByID(ctx, member.UserID)
		if err != nil || user == nil {
			continue
		}
		if len(oldCustomerPrompts) > 0 {
			keep := map[string]bool{}
			var memberships []tables.TableTeamMember
			_ = db.Where("user_id = ?", user.ID).Find(&memberships).Error
			for _, ms := range memberships {
				for _, id := range m.teamPromptIDs(ctx, ms.TeamID) {
					keep[id] = true
				}
			}
			if names := lowerNames(userPromptNames(user)); len(names) > 0 {
				var own []tables.TablePrompt
				_ = db.Select("id").Where("LOWER(name) IN ?", names).Find(&own).Error
				for _, p := range own {
					keep[p.ID] = true
				}
			}
			var revoke []string
			for _, id := range oldCustomerPrompts {
				if !keep[id] {
					revoke = append(revoke, id)
				}
			}
			m.removeUserAllowedPrompts(ctx, user, revoke)
		}
		m.addUserAllowedPrompts(ctx, user, teamPrompts)
	}
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
	teamFolder, err := findEntityFolder(db, team.ID, team.Name, "team")
	if err != nil {
		_ = m.OnTeamCreated(ctx, team)
		teamFolder, _ = findEntityFolder(db, team.ID, team.Name, "team")
	}
	if teamFolder == nil || teamFolder.ID == "" {
		return nil
	}

	{
		// Reuse the user's own prompt wherever it is (Users folder, old team, Removed Users) —
		// case-insensitive on email/username — so joining a team never creates a second one.
		if userPrompt := m.findUserPrompt(ctx, user); userPrompt != nil {
			inTeamFolder := userPrompt.FolderID != nil && *userPrompt.FolderID == teamFolder.ID
			// Move it unless it already lives in another team the user still belongs to
			// (multi-team users keep one prompt in their first team).
			if !inTeamFolder && !m.promptInOtherActiveTeam(ctx, userPrompt, user.ID, team.ID) {
				_ = db.Model(&tables.TablePrompt{}).Where("id = ?", userPrompt.ID).Update("folder_id", teamFolder.ID).Error
			}
			m.ensureUserAllowedPrompt(ctx, user, userPrompt.ID)
		} else if !promptAutoCreateDisabled(ctx, m.store, user.ID) {
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
		}
	}

	// Grant everything the team's audience sees: the team folder subtree and prompts kept at the
	// team's customer level (the same set assignPromptToFolderMembers grants on create/move).
	m.addUserAllowedPrompts(ctx, user, m.teamPromptIDs(ctx, team.ID))
	return nil
}

// OnTeamMemberRemoved moves the member's prompt to "Users/" (standalone) or to their other active team folder.
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

	db := m.store.DB().WithContext(ctx)

	teamFolder, err := findEntityFolder(db, team.ID, team.Name, "team", "archived_team")
	if err != nil {
		return nil
	}
	m.memberLeftTeam(ctx, team.ID, teamFolder, user)
	return nil
}

// memberLeftTeam moves the user's own prompt out of the team folder (to their remaining team's
// folder, else "Users/"), withdraws it from the team members who only had it through the team,
// and revokes the team's prompts from the user unless another of their teams still grants them.
func (m *PromptLifecycleManager) memberLeftTeam(ctx context.Context, teamID string, teamFolder *tables.TableFolder, user *tables.TableUser) {
	db := m.store.DB().WithContext(ctx)

	var targetFolderID string
	var remaining []tables.TableTeamMember
	_ = db.Where("user_id = ? AND team_id <> ?", user.ID, teamID).Find(&remaining).Error
	for _, rm := range remaining {
		if f, fErr := findEntityFolder(db, rm.TeamID, "", "team"); fErr == nil {
			targetFolderID = f.ID
			break
		}
	}
	if targetFolderID == "" {
		if usersFolder, _ := m.EnsureSystemFolder(ctx, "Users", "system_users_root", nil); usersFolder != nil {
			targetFolderID = usersFolder.ID
		}
	}

	if ids := lowerNames(userPromptNames(user)); len(ids) > 0 && targetFolderID != "" {
		var memberPrompts []tables.TablePrompt
		_ = db.Where("LOWER(name) IN ? AND folder_id IN ?", ids, m.audienceFolderIDs(ctx, teamFolder.ID)).Find(&memberPrompts).Error
		for _, p := range memberPrompts {
			_ = db.Model(&tables.TablePrompt{}).Where("id = ?", p.ID).Update("folder_id", targetFolderID).Error
			m.revokePromptFromFolderAudience(ctx, p.ID, teamFolder.ID, targetFolderID)
		}
	}

	m.revokeTeamPrompts(ctx, user, teamID)
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

// OnPromptCreated handles lifecycle sync when a prompt is created manually or via API:
// 1. If created inside a team folder, automatically assigns it to all current members of that team.
// 2. If created inside a customer folder, assigns it to all members of teams under that customer.
// 3. If created by a non-admin user, automatically assigns it to that user.
// 4. Ensures an initial default session exists for playground execution.
func (m *PromptLifecycleManager) OnPromptCreated(ctx context.Context, prompt *tables.TablePrompt, creatorUsername string) error {
	if m == nil || m.store == nil || m.store.DB() == nil || prompt == nil {
		return nil
	}

	db := m.store.DB().WithContext(ctx)

	// 1. Assign to creator if specified
	if creatorUsername != "" {
		creator, err := m.store.GetUserByUsername(ctx, creatorUsername)
		if err == nil && creator != nil {
			m.ensureUserAllowedPrompt(ctx, creator, prompt.ID)
		}
	}

	// 2. If inside a folder, assign to all members of that team/customer
	if prompt.FolderID != nil && *prompt.FolderID != "" {
		m.assignPromptToFolderMembers(ctx, prompt.ID, *prompt.FolderID)
	}

	// 3. Ensure a default session exists so playground works out of the box
	var count int64
	_ = db.Model(&tables.TablePromptSession{}).Where("prompt_id = ?", prompt.ID).Count(&count).Error
	if count == 0 {
		var creatorID string
		if creatorUsername != "" {
			if u, err := m.store.GetUserByUsername(ctx, creatorUsername); err == nil && u != nil {
				creatorID = u.ID
			}
		}
		now := time.Now()
		session := &tables.TablePromptSession{
			PromptID: prompt.ID,
			Name:     prompt.Name + " Default Session",
			UserID:   creatorID,
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
	}

	return nil
}

// OnPromptFolderChanged assigns the prompt to all members if moved into a team or customer folder,
// and synchronizes the user's team membership in governance_team_members if the prompt represents a user.
func (m *PromptLifecycleManager) OnPromptFolderChanged(ctx context.Context, promptID string, newFolderID string) {
	m.OnPromptMoved(ctx, promptID, nil, newFolderID)
}

// OnPromptMoved syncs access and membership after a prompt moved from oldFolderID (nil when
// unknown) to newFolderID ("" = root): members of the new folder's team/customer gain the
// prompt, members who only had it through the old folder lose it, and a user's own prompt
// moves that user from the old folder's team to the new one.
func (m *PromptLifecycleManager) OnPromptMoved(ctx context.Context, promptID string, oldFolderID *string, newFolderID string) {
	if m == nil || m.store == nil || m.store.DB() == nil || promptID == "" {
		return
	}
	m.syncUserTeamOnPromptMove(ctx, promptID, oldFolderID, newFolderID)
	if oldFolderID != nil && *oldFolderID != "" && *oldFolderID != newFolderID {
		m.revokePromptFromFolderAudience(ctx, promptID, *oldFolderID, newFolderID)
	}
	if newFolderID != "" {
		m.assignPromptToFolderMembers(ctx, promptID, newFolderID)
	}
}

// syncUserTeamOnPromptMove updates team membership when a user's own prompt (named after the
// user's email/username) is moved. Only that naming identifies a user prompt: moving a shared
// prompt must never change the membership of whoever created or uses it.
func (m *PromptLifecycleManager) syncUserTeamOnPromptMove(ctx context.Context, promptID string, oldFolderID *string, newFolderID string) {
	db := m.store.DB().WithContext(ctx)

	user := m.promptOwner(ctx, promptID)
	if user == nil {
		return
	}

	targetTeamID := ""
	if newFolderID != "" {
		targetTeamID = m.resolveTeamIDFromFolder(ctx, newFolderID)
	}

	var leaving []string
	if oldFolderID != nil {
		oldTeamID := ""
		if *oldFolderID != "" {
			oldTeamID = m.resolveTeamIDFromFolder(ctx, *oldFolderID)
		}
		if oldTeamID == targetTeamID {
			return
		}
		if oldTeamID != "" {
			leaving = append(leaving, oldTeamID)
		}
	} else {
		// Origin unknown: the prompt's location defines the user's team.
		var current []tables.TableTeamMember
		_ = db.Where("user_id = ? AND team_id <> ?", user.ID, targetTeamID).Find(&current).Error
		for _, c := range current {
			leaving = append(leaving, c.TeamID)
		}
	}

	for _, teamID := range leaving {
		_ = db.Where("team_id = ? AND user_id = ?", teamID, user.ID).Delete(&tables.TableTeamMember{}).Error
		m.revokeTeamPrompts(ctx, user, teamID)
	}

	if targetTeamID != "" {
		var currentMember tables.TableTeamMember
		if err := db.Where("team_id = ? AND user_id = ?", targetTeamID, user.ID).First(&currentMember).Error; err != nil {
			now := time.Now().UTC()
			_ = db.Create(&tables.TableTeamMember{
				TeamID:    targetTeamID,
				UserID:    user.ID,
				CreatedAt: now,
				UpdatedAt: now,
			}).Error
		}
		ids := m.teamPromptIDs(ctx, targetTeamID)
		ids = append(ids, promptID)
		m.addUserAllowedPrompts(ctx, user, ids)
	}
}

// promptOwner returns the user whose own prompt this is (prompt named after their email/username).
func (m *PromptLifecycleManager) promptOwner(ctx context.Context, promptID string) *tables.TableUser {
	db := m.store.DB().WithContext(ctx)
	var prompt tables.TablePrompt
	if err := db.Where("id = ?", promptID).First(&prompt).Error; err != nil {
		return nil
	}
	name := strings.TrimSpace(prompt.Name)
	if name == "" {
		return nil
	}
	var user tables.TableUser
	if err := db.Where("LOWER(email) = LOWER(?) OR LOWER(username) = LOWER(?)", name, name).First(&user).Error; err != nil || user.ID == "" {
		return nil
	}
	return &user
}

// resolveTeamIDFromFolder walks up the folder tree to find the enclosing team ID or customer's team.
func (m *PromptLifecycleManager) resolveTeamIDFromFolder(ctx context.Context, folderID string) string {
	if m == nil || m.store == nil || m.store.DB() == nil || folderID == "" {
		return ""
	}
	db := m.store.DB().WithContext(ctx)

	currID := folderID
	for i := 0; i < 10; i++ {
		if currID == "" {
			break
		}
		var currentFolder tables.TableFolder
		if err := db.Model(&tables.TableFolder{}).Where("id = ?", currID).First(&currentFolder).Error; err != nil {
			break
		}

		if currentFolder.Type == "team" && currentFolder.EntityID != nil && *currentFolder.EntityID != "" {
			return *currentFolder.EntityID
		}
		if currentFolder.Type == "team" {
			var team tables.TableTeam
			if err := db.Where("name = ?", currentFolder.Name).First(&team).Error; err == nil && team.ID != "" {
				return team.ID
			}
		}
		if currentFolder.Type == "customer" {
			if currentFolder.EntityID != nil && *currentFolder.EntityID != "" {
				var team tables.TableTeam
				if err := db.Where("customer_id = ?", *currentFolder.EntityID).Order("created_at asc").First(&team).Error; err == nil && team.ID != "" {
					return team.ID
				}
			}
			var cust tables.TableCustomer
			if err := db.Where("name = ?", currentFolder.Name).First(&cust).Error; err == nil && cust.ID != "" {
				var team tables.TableTeam
				if err := db.Where("customer_id = ?", cust.ID).Order("created_at asc").First(&team).Error; err == nil && team.ID != "" {
					return team.ID
				}
			}
		}

		if currentFolder.ParentID == nil || *currentFolder.ParentID == "" {
			break
		}
		currID = *currentFolder.ParentID
	}
	return ""
}

func (m *PromptLifecycleManager) assignPromptToFolderMembers(ctx context.Context, promptID string, folderID string) {
	for _, teamID := range m.folderAudienceTeams(ctx, folderID) {
		m.assignPromptToTeamMembers(ctx, promptID, teamID)
	}
}

func (m *PromptLifecycleManager) assignPromptToTeamMembers(ctx context.Context, promptID string, teamID string) {
	db := m.store.DB().WithContext(ctx)
	var members []tables.TableTeamMember
	if err := db.Where("team_id = ?", teamID).Find(&members).Error; err != nil {
		return
	}
	for _, member := range members {
		user, err := m.store.GetUserByID(ctx, member.UserID)
		if err == nil && user != nil {
			m.ensureUserAllowedPrompt(ctx, user, promptID)
		}
	}
}

// maxFolderDepth bounds folder-tree walks so a corrupted parent cycle cannot loop forever.
const maxFolderDepth = 32

// folderAudienceTeams returns the teams whose members are granted prompts stored in folderID:
// the nearest enclosing team folder's team, or every team of the nearest enclosing customer folder.
func (m *PromptLifecycleManager) folderAudienceTeams(ctx context.Context, folderID string) []string {
	db := m.store.DB().WithContext(ctx)
	currID := folderID
	for i := 0; i < maxFolderDepth && currID != ""; i++ {
		var folder tables.TableFolder
		if err := db.Where("id = ?", currID).First(&folder).Error; err != nil {
			return nil
		}
		if folder.EntityID != nil && *folder.EntityID != "" {
			switch folder.Type {
			case "team":
				return []string{*folder.EntityID}
			case "customer":
				var teams []tables.TableTeam
				_ = db.Where("customer_id = ?", *folder.EntityID).Find(&teams).Error
				ids := make([]string, 0, len(teams))
				for _, t := range teams {
					ids = append(ids, t.ID)
				}
				return ids
			}
		}
		if folder.ParentID == nil {
			return nil
		}
		currID = *folder.ParentID
	}
	return nil
}

// audienceFolderIDs returns rootID plus its descendants that share its audience, i.e. without
// descending into nested team/customer folders (those have an audience of their own).
func (m *PromptLifecycleManager) audienceFolderIDs(ctx context.Context, rootID string) []string {
	db := m.store.DB().WithContext(ctx)
	ids := []string{rootID}
	seen := map[string]bool{rootID: true}
	frontier := []string{rootID}
	for depth := 0; depth < maxFolderDepth && len(frontier) > 0; depth++ {
		var children []tables.TableFolder
		if err := db.Where("parent_id IN ?", frontier).Find(&children).Error; err != nil {
			break
		}
		frontier = frontier[:0]
		for _, c := range children {
			if seen[c.ID] || ((c.Type == "team" || c.Type == "customer") && c.EntityID != nil && *c.EntityID != "") {
				continue
			}
			seen[c.ID] = true
			ids = append(ids, c.ID)
			frontier = append(frontier, c.ID)
		}
	}
	return ids
}

// teamPromptIDs returns every prompt a member of teamID is granted through the team: prompts
// in the team folder subtree plus prompts kept at the team's customer folder level.
func (m *PromptLifecycleManager) teamPromptIDs(ctx context.Context, teamID string) []string {
	db := m.store.DB().WithContext(ctx)
	var folderIDs []string
	if f, err := findEntityFolder(db, teamID, "", "team"); err == nil {
		folderIDs = append(folderIDs, m.audienceFolderIDs(ctx, f.ID)...)
	}
	var team tables.TableTeam
	if err := db.Where("id = ?", teamID).First(&team).Error; err == nil && team.CustomerID != nil && *team.CustomerID != "" {
		if f, err := findEntityFolder(db, *team.CustomerID, "", "customer"); err == nil {
			folderIDs = append(folderIDs, m.audienceFolderIDs(ctx, f.ID)...)
		}
	}
	if len(folderIDs) == 0 {
		return nil
	}
	var prompts []tables.TablePrompt
	if err := db.Select("id").Where("folder_id IN ?", folderIDs).Find(&prompts).Error; err != nil {
		return nil
	}
	ids := make([]string, 0, len(prompts))
	for _, p := range prompts {
		ids = append(ids, p.ID)
	}
	return ids
}

// revokeTeamPrompts removes the prompts granted through teamID from the user, keeping the
// user's own prompts and anything another of the user's teams still grants.
func (m *PromptLifecycleManager) revokeTeamPrompts(ctx context.Context, user *tables.TableUser, teamID string) {
	revoke := m.teamPromptIDs(ctx, teamID)
	if len(revoke) == 0 {
		return
	}
	db := m.store.DB().WithContext(ctx)
	keep := map[string]bool{}
	var remaining []tables.TableTeamMember
	_ = db.Where("user_id = ? AND team_id <> ?", user.ID, teamID).Find(&remaining).Error
	for _, rm := range remaining {
		for _, id := range m.teamPromptIDs(ctx, rm.TeamID) {
			keep[id] = true
		}
	}
	if names := lowerNames(userPromptNames(user)); len(names) > 0 {
		var own []tables.TablePrompt
		_ = db.Select("id").Where("LOWER(name) IN ?", names).Find(&own).Error
		for _, p := range own {
			keep[p.ID] = true
		}
	}
	filtered := revoke[:0]
	for _, id := range revoke {
		if !keep[id] {
			filtered = append(filtered, id)
		}
	}
	m.removeUserAllowedPrompts(ctx, user, filtered)
}

// revokePromptFromFolderAudience withdraws a prompt that left oldFolderID from the members of
// the old folder's teams, except the prompt's owner and members of the new folder's audience.
func (m *PromptLifecycleManager) revokePromptFromFolderAudience(ctx context.Context, promptID, oldFolderID, newFolderID string) {
	var newTeams []string
	if newFolderID != "" {
		newTeams = m.folderAudienceTeams(ctx, newFolderID)
	}
	m.revokePromptFromTeams(ctx, promptID, m.folderAudienceTeams(ctx, oldFolderID), newTeams)
}

// OnFolderMoved re-syncs access for the prompts inside a folder whose parent changed:
// oldTeams is the folder's audience before the move (folderAudienceTeams).
func (m *PromptLifecycleManager) OnFolderMoved(ctx context.Context, folderID string, oldTeams []string) {
	if m == nil || m.store == nil || m.store.DB() == nil || folderID == "" {
		return
	}
	newTeams := m.folderAudienceTeams(ctx, folderID)
	if sameStringSet(oldTeams, newTeams) {
		return
	}
	var prompts []tables.TablePrompt
	if err := m.store.DB().WithContext(ctx).Select("id").Where("folder_id IN ?", m.audienceFolderIDs(ctx, folderID)).Find(&prompts).Error; err != nil {
		return
	}
	for _, p := range prompts {
		m.revokePromptFromTeams(ctx, p.ID, oldTeams, newTeams)
		for _, teamID := range newTeams {
			m.assignPromptToTeamMembers(ctx, p.ID, teamID)
		}
	}
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]bool, len(a))
	for _, s := range a {
		set[s] = true
	}
	for _, s := range b {
		if !set[s] {
			return false
		}
	}
	return true
}

// revokePromptFromTeams withdraws a prompt from members of oldTeams, except the prompt's owner
// and anyone who is a member of one of keepTeams (the prompt's new audience).
func (m *PromptLifecycleManager) revokePromptFromTeams(ctx context.Context, promptID string, oldTeams, keepTeams []string) {
	if len(oldTeams) == 0 {
		return
	}
	newTeams := make(map[string]bool, len(keepTeams))
	for _, id := range keepTeams {
		newTeams[id] = true
	}
	ownerID := ""
	if owner := m.promptOwner(ctx, promptID); owner != nil {
		ownerID = owner.ID
	}

	db := m.store.DB().WithContext(ctx)
	var members []tables.TableTeamMember
	if err := db.Where("team_id IN ?", oldTeams).Find(&members).Error; err != nil {
		return
	}
	done := map[string]bool{}
	for _, member := range members {
		if member.UserID == ownerID || done[member.UserID] {
			continue
		}
		done[member.UserID] = true
		if len(newTeams) > 0 {
			var memberships []tables.TableTeamMember
			_ = db.Where("user_id = ?", member.UserID).Find(&memberships).Error
			stillInAudience := false
			for _, ms := range memberships {
				if newTeams[ms.TeamID] {
					stillInAudience = true
					break
				}
			}
			if stillInAudience {
				continue
			}
		}
		if user, err := m.store.GetUserByID(ctx, member.UserID); err == nil && user != nil {
			m.removeUserAllowedPrompts(ctx, user, []string{promptID})
		}
	}
}

// promptInOtherActiveTeam reports whether the prompt sits in the folder of a team (other than
// excludeTeamID) that the user is still a member of.
func (m *PromptLifecycleManager) promptInOtherActiveTeam(ctx context.Context, prompt *tables.TablePrompt, userID, excludeTeamID string) bool {
	if prompt == nil || prompt.FolderID == nil || *prompt.FolderID == "" {
		return false
	}
	teamID := m.resolveTeamIDFromFolder(ctx, *prompt.FolderID)
	if teamID == "" || teamID == excludeTeamID {
		return false
	}
	var member tables.TableTeamMember
	return m.store.DB().WithContext(ctx).Where("team_id = ? AND user_id = ?", teamID, userID).First(&member).Error == nil
}

// findEntityFolder returns the folder linked to an entity, preferring the entity_id link and
// falling back to a name match only for legacy folders that were never linked to any entity.
func findEntityFolder(db *gorm.DB, entityID, name string, folderTypes ...string) (*tables.TableFolder, error) {
	var folder tables.TableFolder
	if entityID != "" {
		q := db.Where("entity_id = ?", entityID)
		if len(folderTypes) > 0 {
			q = q.Where("type IN ?", folderTypes)
		}
		err := q.First(&folder).Error
		if err == nil {
			return &folder, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	if name == "" {
		return nil, gorm.ErrRecordNotFound
	}
	q := db.Where("name = ? AND (entity_id IS NULL OR entity_id = '')", name)
	if len(folderTypes) > 0 {
		q = q.Where("type IN ?", folderTypes)
	}
	if err := q.First(&folder).Error; err != nil {
		return nil, err
	}
	return &folder, nil
}

func userPromptNames(user *tables.TableUser) []string {
	if user == nil {
		return nil
	}
	names := make([]string, 0, 2)
	if e := strings.TrimSpace(user.Email); e != "" {
		names = append(names, e)
	}
	if u := strings.TrimSpace(user.Username); u != "" && u != strings.TrimSpace(user.Email) {
		names = append(names, u)
	}
	return names
}

// addUserAllowedPrompts grants several prompts with a single user update.
func (m *PromptLifecycleManager) addUserAllowedPrompts(ctx context.Context, user *tables.TableUser, promptIDs []string) {
	if user == nil || len(promptIDs) == 0 {
		return
	}
	current := splitPromptIDs(user.AllowedPromptRepos)
	have := make(map[string]bool, len(current))
	for _, id := range current {
		have[id] = true
	}
	changed := false
	for _, id := range promptIDs {
		if id != "" && !have[id] {
			have[id] = true
			current = append(current, id)
			changed = true
		}
	}
	if !changed {
		return
	}
	user.AllowedPromptRepos = strings.Join(current, ",")
	_ = m.store.UpdateUser(ctx, user)
}

// removeUserAllowedPrompts revokes several prompts with a single user update.
func (m *PromptLifecycleManager) removeUserAllowedPrompts(ctx context.Context, user *tables.TableUser, promptIDs []string) {
	if user == nil || len(promptIDs) == 0 || user.AllowedPromptRepos == "" {
		return
	}
	drop := make(map[string]bool, len(promptIDs))
	for _, id := range promptIDs {
		drop[id] = true
	}
	current := splitPromptIDs(user.AllowedPromptRepos)
	kept := current[:0]
	for _, id := range current {
		if !drop[id] {
			kept = append(kept, id)
		}
	}
	if len(kept) == len(current) {
		return
	}
	user.AllowedPromptRepos = strings.Join(kept, ",")
	_ = m.store.UpdateUser(ctx, user)
}

func splitPromptIDs(csv string) []string {
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
