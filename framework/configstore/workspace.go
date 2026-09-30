package configstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/unifai/unifai/framework/configstore/tables"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	WorkspaceSettingCluster      = "cluster"
	WorkspaceSettingLoadBalancer = "load_balancer"
	WorkspaceSettingSCIM         = "scim"
	WorkspaceSettingAudit        = "audit"
)

func WorkspaceSettingConnector(name string) string {
	return "connector:" + name
}

// AuditLogQuery filters persisted administrative mutations.
type AuditLogQuery struct {
	Search  string
	Action  string
	Outcome string
	Start   *time.Time
	End     *time.Time
	Limit   int
	Offset  int
}

// WorkspaceStore is the persistence surface for workspace features
// (access profiles, RBAC, circuit breaker, audit logs, and related pages).
// RDBConfigStore implements it. File-backed ConfigStore does not.
type WorkspaceStore interface {
	ListAccessProfiles(ctx context.Context) ([]tables.TableAccessProfile, error)
	GetAccessProfile(ctx context.Context, id uint) (*tables.TableAccessProfile, error)
	CreateAccessProfile(ctx context.Context, row *tables.TableAccessProfile) error
	UpdateAccessProfile(ctx context.Context, row *tables.TableAccessProfile) error
	DeleteAccessProfile(ctx context.Context, id uint) error

	ListRBACRoles(ctx context.Context) ([]tables.TableRBACRole, error)
	GetRBACRole(ctx context.Context, id uint) (*tables.TableRBACRole, error)
	CreateRBACRole(ctx context.Context, row *tables.TableRBACRole) error
	UpdateRBACRole(ctx context.Context, row *tables.TableRBACRole) error
	DeleteRBACRole(ctx context.Context, id uint) error
	EnsureRBACRoles(ctx context.Context) error

	ListBusinessUnits(ctx context.Context) ([]tables.TableBusinessUnit, error)
	GetBusinessUnit(ctx context.Context, id string) (*tables.TableBusinessUnit, error)
	CreateBusinessUnit(ctx context.Context, row *tables.TableBusinessUnit) error
	UpdateBusinessUnit(ctx context.Context, row *tables.TableBusinessUnit) error
	DeleteBusinessUnit(ctx context.Context, id string) error

	ListAlertChannels(ctx context.Context) ([]tables.TableAlertChannel, error)
	GetAlertChannel(ctx context.Context, id uint) (*tables.TableAlertChannel, error)
	CreateAlertChannel(ctx context.Context, row *tables.TableAlertChannel) error
	UpdateAlertChannel(ctx context.Context, row *tables.TableAlertChannel) error
	DeleteAlertChannel(ctx context.Context, id uint) error

	ListCircuitBreakerPolicies(ctx context.Context) ([]tables.TableCircuitBreakerPolicy, error)
	GetCircuitBreakerPolicy(ctx context.Context, name string) (*tables.TableCircuitBreakerPolicy, error)
	CreateCircuitBreakerPolicy(ctx context.Context, row *tables.TableCircuitBreakerPolicy) error
	UpdateCircuitBreakerPolicy(ctx context.Context, row *tables.TableCircuitBreakerPolicy) error
	DeleteCircuitBreakerPolicy(ctx context.Context, name string) error

	ListMCPToolGroups(ctx context.Context) ([]tables.TableMCPToolGroup, error)
	GetMCPToolGroup(ctx context.Context, id uint) (*tables.TableMCPToolGroup, error)
	CreateMCPToolGroup(ctx context.Context, row *tables.TableMCPToolGroup) error
	UpdateMCPToolGroup(ctx context.Context, row *tables.TableMCPToolGroup) error
	DeleteMCPToolGroup(ctx context.Context, id uint) error

	ListPromptDeployments(ctx context.Context, promptID string) ([]tables.TablePromptDeployment, error)
	GetPromptDeployment(ctx context.Context, id uint) (*tables.TablePromptDeployment, error)
	CreatePromptDeployment(ctx context.Context, row *tables.TablePromptDeployment) error
	UpdatePromptDeployment(ctx context.Context, row *tables.TablePromptDeployment) error
	DeletePromptDeployment(ctx context.Context, id uint) error

	ListVirtualKeyUsers(ctx context.Context, virtualKeyID string) ([]tables.TableVirtualKeyUser, error)
	SetVirtualKeyUser(ctx context.Context, virtualKeyID, userID string) error
	DeleteVirtualKeyUser(ctx context.Context, virtualKeyID string) error
	RemoveVirtualKeyUser(ctx context.Context, virtualKeyID, userID string) error
	ListVirtualKeysForUser(ctx context.Context, userID string) ([]tables.TableVirtualKeyUser, error)

	ListTeamMembers(ctx context.Context, teamID string) ([]tables.TableTeamMember, error)
	AddTeamMember(ctx context.Context, teamID, userID string) error
	RemoveTeamMember(ctx context.Context, teamID, userID string) error
	ListTeamsForUser(ctx context.Context, userID string) ([]tables.TableTeamMember, error)

	GetWorkspaceSetting(ctx context.Context, key string) (*tables.TableWorkspaceSetting, error)
	UpsertWorkspaceSetting(ctx context.Context, key, data string) error
	DeleteWorkspaceSetting(ctx context.Context, key string) error

	ListAuditLogs(ctx context.Context, query AuditLogQuery) ([]tables.TableAuditLog, int64, error)
	CreateAuditLog(ctx context.Context, row *tables.TableAuditLog) error
}

// AsWorkspaceStore returns the workspace persistence layer when the config
// store is an RDB implementation.
func AsWorkspaceStore(store ConfigStore) (WorkspaceStore, bool) {
	if store == nil {
		return nil, false
	}
	ws, ok := store.(WorkspaceStore)
	return ws, ok
}

func (s *RDBConfigStore) ListAccessProfiles(ctx context.Context) ([]tables.TableAccessProfile, error) {
	var rows []tables.TableAccessProfile
	err := s.DB().WithContext(ctx).Order("id asc").Find(&rows).Error
	return rows, err
}

func (s *RDBConfigStore) GetAccessProfile(ctx context.Context, id uint) (*tables.TableAccessProfile, error) {
	return firstByID[tables.TableAccessProfile](s.DB().WithContext(ctx), id)
}

func (s *RDBConfigStore) CreateAccessProfile(ctx context.Context, row *tables.TableAccessProfile) error {
	return s.DB().WithContext(ctx).Create(row).Error
}

func (s *RDBConfigStore) UpdateAccessProfile(ctx context.Context, row *tables.TableAccessProfile) error {
	return s.DB().WithContext(ctx).Save(row).Error
}

func (s *RDBConfigStore) DeleteAccessProfile(ctx context.Context, id uint) error {
	return deleteByID[tables.TableAccessProfile](s.DB().WithContext(ctx), id)
}

func (s *RDBConfigStore) ListRBACRoles(ctx context.Context) ([]tables.TableRBACRole, error) {
	var rows []tables.TableRBACRole
	err := s.DB().WithContext(ctx).Order("id asc").Find(&rows).Error
	return rows, err
}

func (s *RDBConfigStore) GetRBACRole(ctx context.Context, id uint) (*tables.TableRBACRole, error) {
	return firstByID[tables.TableRBACRole](s.DB().WithContext(ctx), id)
}

func (s *RDBConfigStore) CreateRBACRole(ctx context.Context, row *tables.TableRBACRole) error {
	return s.DB().WithContext(ctx).Create(row).Error
}

func (s *RDBConfigStore) UpdateRBACRole(ctx context.Context, row *tables.TableRBACRole) error {
	return s.DB().WithContext(ctx).Save(row).Error
}

func (s *RDBConfigStore) DeleteRBACRole(ctx context.Context, id uint) error {
	return deleteByID[tables.TableRBACRole](s.DB().WithContext(ctx), id)
}

func (s *RDBConfigStore) EnsureRBACRoles(ctx context.Context) error {
	now := time.Now().UTC()
	allIDs := make([]uint, 0, len(RBACPermissions()))
	readIDs := make([]uint, 0)
	subAdminIDs := make([]uint, 0)
	for _, perm := range RBACPermissions() {
		allIDs = append(allIDs, perm.ID)
		if perm.Operation == "View" || perm.Operation == "Read" {
			switch perm.Resource {
			case "Dashboard", "Logs", "Inference", "PromptRepository", "Observability", "MCPGateway":
				readIDs = append(readIDs, perm.ID)
			}
		}

		// Sub-Admin permissions:
		switch perm.Resource {
		case "Dashboard", "Inference", "Observability", "MCPLogs", "RoutingRules", "Metrics":
			if perm.Operation == "View" || perm.Operation == "Read" {
				subAdminIDs = append(subAdminIDs, perm.ID)
			}
		case "Logs":
			// Browser AI (rules import, targets, agents, controls) is RBAC-gated as Logs.
			// Sub-admins need write ops so Guard Rules Import / CRUD works end-to-end.
			if perm.Operation == "View" || perm.Operation == "Read" ||
				perm.Operation == "Create" || perm.Operation == "Update" ||
				perm.Operation == "Delete" || perm.Operation == "Download" {
				subAdminIDs = append(subAdminIDs, perm.ID)
			}
		case "VirtualKeys", "PromptRepository":
			// Full control over virtual keys and prompts
			subAdminIDs = append(subAdminIDs, perm.ID)
		case "Governance":
			// Can view and update budgets/rate limits
			if perm.Operation == "View" || perm.Operation == "Read" || perm.Operation == "Update" {
				subAdminIDs = append(subAdminIDs, perm.ID)
			}
		case "MCPGateway", "MCPToolGroups":
			if perm.Operation == "View" || perm.Operation == "Read" || perm.Operation == "Create" || perm.Operation == "Update" {
				subAdminIDs = append(subAdminIDs, perm.ID)
			}
		case "ModelProvider":
			// View-only for model selection (cannot create, edit, or view master provider API keys)
			if perm.Operation == "View" || perm.Operation == "Read" {
				subAdminIDs = append(subAdminIDs, perm.ID)
			}
		}
	}

	var existing []tables.TableRBACRole
	_ = s.DB().WithContext(ctx).Find(&existing).Error
	hasAdmin := false
	hasSubAdmin := false
	hasUser := false
	for _, r := range existing {
		switch r.Name {
		case "admin":
			hasAdmin = true
		case "sub_admin":
			hasSubAdmin = true
		case "user":
			hasUser = true
		}
	}

	var toCreate []*tables.TableRBACRole
	if !hasAdmin {
		toCreate = append(toCreate, &tables.TableRBACRole{
			Name: "admin", Description: "Full workspace access", IsSystemRole: true,
			DAC: "all-data", ParsedPermissionIDs: allIDs, CreatedAt: now, UpdatedAt: now,
		})
	}
	if !hasSubAdmin {
		toCreate = append(toCreate, &tables.TableRBACRole{
			Name: "sub_admin", Description: "Workspace Manager - manages virtual keys, budgets, prompts, logs, and Browser AI without access to master provider keys or system settings", IsSystemRole: true,
			DAC: "all-data", ParsedPermissionIDs: subAdminIDs, CreatedAt: now, UpdatedAt: now,
		})
	}
	if !hasUser {
		toCreate = append(toCreate, &tables.TableRBACRole{
			Name: "user", Description: "Basic workspace access", IsSystemRole: true,
			DAC: "own-data", ParsedPermissionIDs: readIDs, CreatedAt: now, UpdatedAt: now,
		})
	}

	if len(toCreate) > 0 {
		if err := s.DB().WithContext(ctx).Create(toCreate).Error; err != nil {
			return err
		}
	}

	// Keep system role permission catalogs in sync (e.g. Browser AI write via Logs).
	// Merge required IDs into existing system roles without stripping custom extras.
	for i := range existing {
		r := existing[i]
		var required []uint
		switch r.Name {
		case "admin":
			required = allIDs
		case "sub_admin":
			required = subAdminIDs
		case "user":
			required = readIDs
		default:
			continue
		}
		merged := mergeUintUnique(r.ParsedPermissionIDs, required)
		if sameUintSlice(r.ParsedPermissionIDs, merged) {
			continue
		}
		r.ParsedPermissionIDs = merged
		r.UpdatedAt = now
		if err := s.DB().WithContext(ctx).Save(&r).Error; err != nil {
			return err
		}
	}
	return nil
}

func mergeUintUnique(base, add []uint) []uint {
	seen := make(map[uint]bool, len(base)+len(add))
	out := make([]uint, 0, len(base)+len(add))
	for _, v := range base {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	for _, v := range add {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func sameUintSlice(a, b []uint) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[uint]int, len(a))
	for _, v := range a {
		counts[v]++
	}
	for _, v := range b {
		counts[v]--
		if counts[v] < 0 {
			return false
		}
	}
	for _, c := range counts {
		if c != 0 {
			return false
		}
	}
	return true
}

func (s *RDBConfigStore) ListBusinessUnits(ctx context.Context) ([]tables.TableBusinessUnit, error) {
	var rows []tables.TableBusinessUnit
	err := s.DB().WithContext(ctx).Order("created_at desc").Find(&rows).Error
	return rows, err
}

func (s *RDBConfigStore) GetBusinessUnit(ctx context.Context, id string) (*tables.TableBusinessUnit, error) {
	var row tables.TableBusinessUnit
	err := s.DB().WithContext(ctx).First(&row, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *RDBConfigStore) CreateBusinessUnit(ctx context.Context, row *tables.TableBusinessUnit) error {
	return s.DB().WithContext(ctx).Create(row).Error
}

func (s *RDBConfigStore) UpdateBusinessUnit(ctx context.Context, row *tables.TableBusinessUnit) error {
	return s.DB().WithContext(ctx).Save(row).Error
}

func (s *RDBConfigStore) DeleteBusinessUnit(ctx context.Context, id string) error {
	res := s.DB().WithContext(ctx).Where("id = ?", id).Delete(&tables.TableBusinessUnit{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *RDBConfigStore) ListAlertChannels(ctx context.Context) ([]tables.TableAlertChannel, error) {
	var rows []tables.TableAlertChannel
	err := s.DB().WithContext(ctx).Order("id asc").Find(&rows).Error
	return rows, err
}

func (s *RDBConfigStore) GetAlertChannel(ctx context.Context, id uint) (*tables.TableAlertChannel, error) {
	return firstByID[tables.TableAlertChannel](s.DB().WithContext(ctx), id)
}

func (s *RDBConfigStore) CreateAlertChannel(ctx context.Context, row *tables.TableAlertChannel) error {
	return s.DB().WithContext(ctx).Create(row).Error
}

func (s *RDBConfigStore) UpdateAlertChannel(ctx context.Context, row *tables.TableAlertChannel) error {
	return s.DB().WithContext(ctx).Save(row).Error
}

func (s *RDBConfigStore) DeleteAlertChannel(ctx context.Context, id uint) error {
	return deleteByID[tables.TableAlertChannel](s.DB().WithContext(ctx), id)
}

func (s *RDBConfigStore) ListCircuitBreakerPolicies(ctx context.Context) ([]tables.TableCircuitBreakerPolicy, error) {
	var rows []tables.TableCircuitBreakerPolicy
	err := s.DB().WithContext(ctx).Order("name asc").Find(&rows).Error
	return rows, err
}

func (s *RDBConfigStore) GetCircuitBreakerPolicy(ctx context.Context, name string) (*tables.TableCircuitBreakerPolicy, error) {
	var row tables.TableCircuitBreakerPolicy
	err := s.DB().WithContext(ctx).First(&row, "name = ?", name).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *RDBConfigStore) CreateCircuitBreakerPolicy(ctx context.Context, row *tables.TableCircuitBreakerPolicy) error {
	return s.DB().WithContext(ctx).Create(row).Error
}

func (s *RDBConfigStore) UpdateCircuitBreakerPolicy(ctx context.Context, row *tables.TableCircuitBreakerPolicy) error {
	return s.DB().WithContext(ctx).Save(row).Error
}

func (s *RDBConfigStore) DeleteCircuitBreakerPolicy(ctx context.Context, name string) error {
	res := s.DB().WithContext(ctx).Where("name = ?", name).Delete(&tables.TableCircuitBreakerPolicy{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *RDBConfigStore) ListMCPToolGroups(ctx context.Context) ([]tables.TableMCPToolGroup, error) {
	var rows []tables.TableMCPToolGroup
	err := s.DB().WithContext(ctx).Order("id asc").Find(&rows).Error
	return rows, err
}

func (s *RDBConfigStore) GetMCPToolGroup(ctx context.Context, id uint) (*tables.TableMCPToolGroup, error) {
	return firstByID[tables.TableMCPToolGroup](s.DB().WithContext(ctx), id)
}

func (s *RDBConfigStore) CreateMCPToolGroup(ctx context.Context, row *tables.TableMCPToolGroup) error {
	return s.DB().WithContext(ctx).Create(row).Error
}

func (s *RDBConfigStore) UpdateMCPToolGroup(ctx context.Context, row *tables.TableMCPToolGroup) error {
	return s.DB().WithContext(ctx).Save(row).Error
}

func (s *RDBConfigStore) DeleteMCPToolGroup(ctx context.Context, id uint) error {
	return deleteByID[tables.TableMCPToolGroup](s.DB().WithContext(ctx), id)
}

func (s *RDBConfigStore) ListPromptDeployments(ctx context.Context, promptID string) ([]tables.TablePromptDeployment, error) {
	query := s.DB().WithContext(ctx).Order("id asc")
	if promptID != "" {
		query = query.Where("prompt_id = ?", promptID)
	}
	var rows []tables.TablePromptDeployment
	err := query.Find(&rows).Error
	return rows, err
}

func (s *RDBConfigStore) GetPromptDeployment(ctx context.Context, id uint) (*tables.TablePromptDeployment, error) {
	return firstByID[tables.TablePromptDeployment](s.DB().WithContext(ctx), id)
}

func (s *RDBConfigStore) CreatePromptDeployment(ctx context.Context, row *tables.TablePromptDeployment) error {
	return s.DB().WithContext(ctx).Create(row).Error
}

func (s *RDBConfigStore) UpdatePromptDeployment(ctx context.Context, row *tables.TablePromptDeployment) error {
	return s.DB().WithContext(ctx).Save(row).Error
}

func (s *RDBConfigStore) DeletePromptDeployment(ctx context.Context, id uint) error {
	return deleteByID[tables.TablePromptDeployment](s.DB().WithContext(ctx), id)
}

func (s *RDBConfigStore) ensureVirtualKeyUsersTable(ctx context.Context) {
	db := s.DB().WithContext(ctx)
	if !db.Migrator().HasTable(&tables.TableVirtualKeyUser{}) {
		_ = db.AutoMigrate(&tables.TableVirtualKeyUser{})
		return
	}
	// Drop old unique index on virtual_key_id alone if it exists so multiple users can share the same virtual key.
	_ = db.Migrator().DropIndex(&tables.TableVirtualKeyUser{}, "virtual_key_id")
	_ = db.Migrator().DropIndex(&tables.TableVirtualKeyUser{}, "idx_governance_virtual_key_users_virtual_key_id")
	_ = db.Migrator().DropIndex(&tables.TableVirtualKeyUser{}, "uix_governance_virtual_key_users_virtual_key_id")
	_ = db.AutoMigrate(&tables.TableVirtualKeyUser{})
}

func (s *RDBConfigStore) ListVirtualKeyUsers(ctx context.Context, virtualKeyID string) ([]tables.TableVirtualKeyUser, error) {
	s.ensureVirtualKeyUsersTable(ctx)
	var rows []tables.TableVirtualKeyUser
	err := s.DB().WithContext(ctx).Where("virtual_key_id = ?", virtualKeyID).Find(&rows).Error
	return rows, err
}

func (s *RDBConfigStore) ListVirtualKeysForUser(ctx context.Context, userID string) ([]tables.TableVirtualKeyUser, error) {
	s.ensureVirtualKeyUsersTable(ctx)
	var rows []tables.TableVirtualKeyUser
	err := s.DB().WithContext(ctx).Where("user_id = ?", userID).Find(&rows).Error
	return rows, err
}

func (s *RDBConfigStore) SetVirtualKeyUser(ctx context.Context, virtualKeyID, userID string) error {
	s.ensureVirtualKeyUsersTable(ctx)
	now := time.Now().UTC()

	var existing tables.TableVirtualKeyUser
	err := s.DB().WithContext(ctx).Where("virtual_key_id = ? AND user_id = ?", virtualKeyID, userID).First(&existing).Error
	if err == nil {
		existing.UpdatedAt = now
		return s.DB().WithContext(ctx).Save(&existing).Error
	}
	return s.DB().WithContext(ctx).Create(&tables.TableVirtualKeyUser{
		VirtualKeyID: virtualKeyID,
		UserID:       userID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}).Error
}

func (s *RDBConfigStore) DeleteVirtualKeyUser(ctx context.Context, virtualKeyID string) error {
	s.ensureVirtualKeyUsersTable(ctx)
	return s.DB().WithContext(ctx).Where("virtual_key_id = ?", virtualKeyID).Delete(&tables.TableVirtualKeyUser{}).Error
}

func (s *RDBConfigStore) RemoveVirtualKeyUser(ctx context.Context, virtualKeyID, userID string) error {
	s.ensureVirtualKeyUsersTable(ctx)
	return s.DB().WithContext(ctx).Where("virtual_key_id = ? AND user_id = ?", virtualKeyID, userID).Delete(&tables.TableVirtualKeyUser{}).Error
}

func (s *RDBConfigStore) ensureTeamMembersTable(ctx context.Context) error {
	db := s.DB().WithContext(ctx)
	if db.Migrator().HasTable(&tables.TableTeamMember{}) {
		return nil
	}
	if err := db.AutoMigrate(&tables.TableTeamMember{}); err != nil {
		return fmt.Errorf("ensure governance_team_members table: %w", err)
	}
	return nil
}

func (s *RDBConfigStore) ListTeamMembers(ctx context.Context, teamID string) ([]tables.TableTeamMember, error) {
	if err := s.ensureTeamMembersTable(ctx); err != nil {
		return nil, err
	}
	var rows []tables.TableTeamMember
	err := s.DB().WithContext(ctx).Where("team_id = ?", teamID).Find(&rows).Error
	return rows, err
}

func (s *RDBConfigStore) ListTeamsForUser(ctx context.Context, userID string) ([]tables.TableTeamMember, error) {
	if err := s.ensureTeamMembersTable(ctx); err != nil {
		return nil, err
	}
	var rows []tables.TableTeamMember
	err := s.DB().WithContext(ctx).Where("user_id = ?", userID).Find(&rows).Error
	return rows, err
}

func (s *RDBConfigStore) AddTeamMember(ctx context.Context, teamID, userID string) error {
	if err := s.ensureTeamMembersTable(ctx); err != nil {
		return err
	}
	now := time.Now().UTC()
	var existing tables.TableTeamMember
	err := s.DB().WithContext(ctx).Where("team_id = ? AND user_id = ?", teamID, userID).First(&existing).Error
	if err == nil {
		return nil // already a member
	}
	return s.DB().WithContext(ctx).Create(&tables.TableTeamMember{
		TeamID:    teamID,
		UserID:    userID,
		CreatedAt: now,
		UpdatedAt: now,
	}).Error
}

func (s *RDBConfigStore) RemoveTeamMember(ctx context.Context, teamID, userID string) error {
	if err := s.ensureTeamMembersTable(ctx); err != nil {
		return err
	}
	return s.DB().WithContext(ctx).Where("team_id = ? AND user_id = ?", teamID, userID).Delete(&tables.TableTeamMember{}).Error
}

func (s *RDBConfigStore) GetWorkspaceSetting(ctx context.Context, key string) (*tables.TableWorkspaceSetting, error) {
	var row tables.TableWorkspaceSetting
	err := s.DB().WithContext(ctx).First(&row, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *RDBConfigStore) UpsertWorkspaceSetting(ctx context.Context, key, data string) error {
	row := tables.TableWorkspaceSetting{Key: key, Data: data, UpdatedAt: time.Now().UTC()}
	return s.DB().WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"data", "updated_at"}),
	}).Create(&row).Error
}

func (s *RDBConfigStore) DeleteWorkspaceSetting(ctx context.Context, key string) error {
	res := s.DB().WithContext(ctx).Where("key = ?", key).Delete(&tables.TableWorkspaceSetting{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *RDBConfigStore) ListAuditLogs(ctx context.Context, query AuditLogQuery) ([]tables.TableAuditLog, int64, error) {
	db := s.DB().WithContext(ctx).Model(&tables.TableAuditLog{})
	if query.Search != "" {
		like := "%" + query.Search + "%"
		db = db.Where("LOWER(initiator) LIKE LOWER(?) OR LOWER(target) LIKE LOWER(?) OR LOWER(path) LIKE LOWER(?) OR LOWER(ip) LIKE LOWER(?)", like, like, like, like)
	}
	if query.Action != "" {
		db = db.Where("action = ?", query.Action)
	}
	if query.Outcome != "" {
		db = db.Where("outcome = ?", query.Outcome)
	}
	if query.Start != nil {
		db = db.Where("created_at >= ?", *query.Start)
	}
	if query.End != nil {
		db = db.Where("created_at <= ?", *query.End)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := query.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	var rows []tables.TableAuditLog
	err := db.Order("created_at desc").Limit(limit).Offset(query.Offset).Find(&rows).Error
	return rows, total, err
}

func (s *RDBConfigStore) CreateAuditLog(ctx context.Context, row *tables.TableAuditLog) error {
	return s.DB().WithContext(ctx).Create(row).Error
}

func firstByID[T any](db *gorm.DB, id uint) (*T, error) {
	var row T
	err := db.First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func deleteByID[T any](db *gorm.DB, id uint) error {
	var row T
	res := db.Delete(&row, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// RBACPermission is a catalog entry. Permissions are code-declared, not stored.
type RBACPermission struct {
	ID        uint   `json:"id"`
	Resource  string `json:"resource"`
	Operation string `json:"operation"`
}

// RBACResourceNames is the workspace resource catalog.
var RBACResourceNames = []string{
	"GuardrailsConfig", "GuardrailsProviders", "GuardrailRules", "UserProvisioning", "Cluster",
	"Settings", "Users", "Logs", "Observability", "Dashboard", "VirtualKeys", "ModelProvider",
	"Plugins", "MCPGateway", "MCPToolGroups", "MCPLogs", "AdaptiveRouter", "AuditLogs",
	"Customers", "Teams", "RBAC", "Governance", "RoutingRules", "PromptRepository",
	"PromptDeploymentStrategy", "SkillsRepository", "AccessProfiles", "APIKeys",
	"Inference", "Metrics", "FeatureFlags", "CircuitBreaker",
}

// RBACOperationNames is the workspace operation catalog.
var RBACOperationNames = []string{"Read", "View", "Create", "Update", "Delete", "Download"}

// RBACPermissions returns the full cartesian catalog of resource x operation.
func RBACPermissions() []RBACPermission {
	perms := make([]RBACPermission, 0, len(RBACResourceNames)*len(RBACOperationNames))
	var id uint = 1
	for _, resource := range RBACResourceNames {
		for _, operation := range RBACOperationNames {
			perms = append(perms, RBACPermission{ID: id, Resource: resource, Operation: operation})
			id++
		}
	}
	return perms
}
