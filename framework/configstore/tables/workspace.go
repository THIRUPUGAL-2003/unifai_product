package tables

import (
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/unifai/unifai/framework/encrypt"
	"gorm.io/gorm"
)

func writeJSON(value any) (*string, error) {
	if value == nil {
		return nil, nil
	}
	raw, err := sonic.Marshal(value)
	if err != nil {
		return nil, err
	}
	encoded := string(raw)
	return &encoded, nil
}

func readJSON(raw *string, dest any) error {
	if raw == nil || *raw == "" {
		return nil
	}
	return sonic.Unmarshal([]byte(*raw), dest)
}

// TableAccessProfile is a reusable governance policy template.
type TableAccessProfile struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	Name            string         `gorm:"type:varchar(255);not null;uniqueIndex" json:"name"`
	Description     string         `gorm:"type:text" json:"description"`
	IsActive        bool           `gorm:"not null;default:false" json:"is_active"`
	Version         int            `gorm:"not null;default:1" json:"version"`
	CalendarAligned bool           `gorm:"not null;default:false" json:"calendar_aligned"`
	TagsJSON        *string        `gorm:"type:text" json:"-"`
	ParsedTags      []string       `gorm:"-" json:"tags,omitempty"`
	SpecJSON        *string        `gorm:"type:text" json:"-"`
	ParsedSpec      map[string]any `gorm:"-" json:"-"`
	CreatedAt       time.Time      `gorm:"index;not null" json:"created_at"`
	UpdatedAt       time.Time      `gorm:"index;not null" json:"updated_at"`
}

func (TableAccessProfile) TableName() string { return "access_profiles" }

func (p *TableAccessProfile) BeforeSave(tx *gorm.DB) error {
	encoded, err := writeJSON(p.ParsedTags)
	if err != nil {
		return err
	}
	p.TagsJSON = encoded
	encoded, err = writeJSON(p.ParsedSpec)
	if err != nil {
		return err
	}
	p.SpecJSON = encoded
	return nil
}

func (p *TableAccessProfile) AfterFind(tx *gorm.DB) error {
	if err := readJSON(p.TagsJSON, &p.ParsedTags); err != nil {
		return err
	}
	return readJSON(p.SpecJSON, &p.ParsedSpec)
}

func (p TableAccessProfile) Spec() map[string]any {
	if p.ParsedSpec == nil {
		return map[string]any{}
	}
	return p.ParsedSpec
}

// TableRBACRole is a named permission set with a data-access scope.
type TableRBACRole struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	Name                string    `gorm:"type:varchar(255);not null;uniqueIndex" json:"name"`
	Description         string    `gorm:"type:text" json:"description"`
	IsSystemRole        bool      `gorm:"not null;default:false" json:"is_system_role"`
	DAC                 string    `gorm:"type:varchar(32);not null;default:all-data" json:"dac"`
	PermissionIDsJSON   *string   `gorm:"type:text" json:"-"`
	ParsedPermissionIDs []uint    `gorm:"-" json:"permission_ids,omitempty"`
	CreatedAt           time.Time `gorm:"index;not null" json:"created_at"`
	UpdatedAt           time.Time `gorm:"index;not null" json:"updated_at"`
}

func (TableRBACRole) TableName() string { return "rbac_roles" }

func (r *TableRBACRole) BeforeSave(tx *gorm.DB) error {
	encoded, err := writeJSON(r.ParsedPermissionIDs)
	if err != nil {
		return err
	}
	r.PermissionIDsJSON = encoded
	return nil
}

func (r *TableRBACRole) AfterFind(tx *gorm.DB) error {
	return readJSON(r.PermissionIDsJSON, &r.ParsedPermissionIDs)
}

// TableBusinessUnit groups teams under an organizational unit.
type TableBusinessUnit struct {
	ID              string         `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Name            string         `gorm:"type:varchar(255);not null;uniqueIndex" json:"name"`
	TeamIDsJSON     *string        `gorm:"type:text" json:"-"`
	ParsedTeamIDs   []string       `gorm:"-" json:"team_ids,omitempty"`
	BudgetJSON      *string        `gorm:"type:text" json:"-"`
	ParsedBudget    map[string]any `gorm:"-" json:"budget,omitempty"`
	RateLimitJSON   *string        `gorm:"type:text" json:"-"`
	ParsedRateLimit map[string]any `gorm:"-" json:"rate_limit,omitempty"`
	CreatedAt       time.Time      `gorm:"index;not null" json:"created_at"`
	UpdatedAt       time.Time      `gorm:"index;not null" json:"updated_at"`
}

func (TableBusinessUnit) TableName() string { return "governance_business_units" }

func (u *TableBusinessUnit) BeforeSave(tx *gorm.DB) error {
	var err error
	if u.TeamIDsJSON, err = writeJSON(u.ParsedTeamIDs); err != nil {
		return err
	}
	if u.BudgetJSON, err = writeJSON(u.ParsedBudget); err != nil {
		return err
	}
	u.RateLimitJSON, err = writeJSON(u.ParsedRateLimit)
	return err
}

func (u *TableBusinessUnit) AfterFind(tx *gorm.DB) error {
	if err := readJSON(u.TeamIDsJSON, &u.ParsedTeamIDs); err != nil {
		return err
	}
	if err := readJSON(u.BudgetJSON, &u.ParsedBudget); err != nil {
		return err
	}
	return readJSON(u.RateLimitJSON, &u.ParsedRateLimit)
}

// TableAlertChannel is a notification destination.
type TableAlertChannel struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	Name         string         `gorm:"type:varchar(255);not null" json:"name"`
	Type         string         `gorm:"type:varchar(32);not null" json:"type"`
	Enabled      bool           `gorm:"not null;default:true" json:"enabled"`
	ConfigJSON   *string        `gorm:"type:text" json:"-"`
	ParsedConfig map[string]any `gorm:"-" json:"config"`
	// EncryptionStatus tracks whether ConfigJSON (webhook URLs / tokens) is AES-encrypted at rest.
	EncryptionStatus string    `gorm:"type:varchar(20);default:'plain_text'" json:"-"`
	CreatedAt        time.Time `gorm:"index;not null" json:"created_at"`
	UpdatedAt        time.Time `gorm:"index;not null" json:"updated_at"`
}

func (TableAlertChannel) TableName() string { return "alert_channels" }

func (c *TableAlertChannel) BeforeSave(tx *gorm.DB) error {
	encoded, err := writeJSON(c.ParsedConfig)
	if err != nil {
		return err
	}
	c.ConfigJSON = encoded
	if encrypt.IsEnabled() && c.ConfigJSON != nil && strings.TrimSpace(*c.ConfigJSON) != "" && *c.ConfigJSON != "{}" && *c.ConfigJSON != "null" {
		enc, err := encrypt.Encrypt(*c.ConfigJSON)
		if err != nil {
			return err
		}
		c.ConfigJSON = &enc
		c.EncryptionStatus = EncryptionStatusEncrypted
	} else if c.EncryptionStatus == "" {
		c.EncryptionStatus = EncryptionStatusPlainText
	}
	return nil
}

func (c *TableAlertChannel) AfterFind(tx *gorm.DB) error {
	if c.ParsedConfig == nil {
		c.ParsedConfig = map[string]any{}
	}
	if c.EncryptionStatus == EncryptionStatusEncrypted && c.ConfigJSON != nil && *c.ConfigJSON != "" {
		dec, err := encrypt.Decrypt(*c.ConfigJSON)
		if err != nil {
			return err
		}
		c.ConfigJSON = &dec
	}
	return readJSON(c.ConfigJSON, &c.ParsedConfig)
}

// TableCircuitBreakerPolicy fails over a primary provider+model when a signal matches.
type TableCircuitBreakerPolicy struct {
	Name                string         `gorm:"primaryKey;type:varchar(255)" json:"name"`
	Enabled             bool           `gorm:"not null;default:true" json:"enabled"`
	PrimaryProvider     string         `gorm:"type:varchar(100);not null" json:"primary_provider"`
	PrimaryModel        string         `gorm:"type:varchar(255);not null" json:"primary_model"`
	FallbackProvider    string         `gorm:"type:varchar(100);not null" json:"fallback_provider"`
	FallbackModel       string         `gorm:"type:varchar(255);not null" json:"fallback_model"`
	DefaultCooldown     string         `gorm:"type:varchar(32);not null;default:30s" json:"default_cooldown"`
	CooldownHeader      string         `gorm:"type:varchar(255)" json:"cooldown_header,omitempty"`
	ConditionJSON       *string        `gorm:"type:text" json:"-"`
	ParsedCondition     map[string]any `gorm:"-" json:"condition"`
	PrimaryKeyIDsJSON   *string        `gorm:"type:text" json:"-"`
	ParsedPrimaryKeyIDs []string       `gorm:"-" json:"primary_key_ids,omitempty"`
	CreatedAt           time.Time      `gorm:"index;not null" json:"created_at"`
	UpdatedAt           time.Time      `gorm:"index;not null" json:"updated_at"`
}

func (TableCircuitBreakerPolicy) TableName() string { return "circuit_breaker_policies" }

func (p *TableCircuitBreakerPolicy) BeforeSave(tx *gorm.DB) error {
	var err error
	if p.ConditionJSON, err = writeJSON(p.ParsedCondition); err != nil {
		return err
	}
	p.PrimaryKeyIDsJSON, err = writeJSON(p.ParsedPrimaryKeyIDs)
	return err
}

func (p *TableCircuitBreakerPolicy) AfterFind(tx *gorm.DB) error {
	if err := readJSON(p.ConditionJSON, &p.ParsedCondition); err != nil {
		return err
	}
	return readJSON(p.PrimaryKeyIDsJSON, &p.ParsedPrimaryKeyIDs)
}

// TableMCPToolGroup bundles tools from one or more MCP clients.
type TableMCPToolGroup struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	Name        string         `gorm:"type:varchar(255);not null" json:"name"`
	Description string         `gorm:"type:text" json:"description"`
	Enabled     bool           `gorm:"not null;default:false" json:"enabled"`
	SpecJSON    *string        `gorm:"type:text" json:"-"`
	ParsedSpec  map[string]any `gorm:"-" json:"-"`
	CreatedAt   time.Time      `gorm:"index;not null" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"index;not null" json:"updated_at"`
}

func (TableMCPToolGroup) TableName() string { return "mcp_tool_groups" }

func (g *TableMCPToolGroup) BeforeSave(tx *gorm.DB) error {
	encoded, err := writeJSON(g.ParsedSpec)
	if err != nil {
		return err
	}
	g.SpecJSON = encoded
	return nil
}

func (g *TableMCPToolGroup) AfterFind(tx *gorm.DB) error {
	return readJSON(g.SpecJSON, &g.ParsedSpec)
}

// TablePromptDeployment pins a prompt version to an environment.
type TablePromptDeployment struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	PromptID      string    `gorm:"type:varchar(36);not null;index" json:"prompt_id"`
	PromptName    string    `gorm:"type:varchar(255)" json:"prompt_name"`
	VersionNumber int       `gorm:"not null" json:"version_number"`
	Environment   string    `gorm:"type:varchar(64);not null" json:"environment"`
	Enabled       bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt     time.Time `gorm:"index;not null" json:"created_at"`
	UpdatedAt     time.Time `gorm:"index;not null" json:"updated_at"`
}

func (TablePromptDeployment) TableName() string { return "prompt_deployments" }

// TableWorkspaceSetting stores singleton workspace configs (cluster, load balancer, SCIM, connectors).
type TableWorkspaceSetting struct {
	Key       string    `gorm:"primaryKey;type:varchar(64)" json:"key"`
	Data      string    `gorm:"type:text;not null" json:"data"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`
}

func (TableWorkspaceSetting) TableName() string { return "workspace_settings" }

// TableVirtualKeyUser links a virtual key to a governance user (AP-managed detection).
type TableVirtualKeyUser struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	VirtualKeyID string    `gorm:"type:varchar(255);not null;index:idx_vk_user_pair,unique;index:idx_vk_id" json:"virtual_key_id"`
	UserID       string    `gorm:"type:varchar(255);not null;index:idx_vk_user_pair,unique;index:idx_user_id" json:"user_id"`
	CreatedAt    time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt    time.Time `gorm:"not null" json:"updated_at"`
}

func (TableVirtualKeyUser) TableName() string { return "governance_virtual_key_users" }

// TableAuditLog records administrative mutations.
type TableAuditLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Action     string    `gorm:"size:64;index;not null" json:"action"`
	Outcome    string    `gorm:"size:32;index;not null" json:"outcome"`
	Initiator  string    `gorm:"size:255;index" json:"initiator"`
	Target     string    `gorm:"size:512;index" json:"target"`
	Method     string    `gorm:"size:16" json:"method"`
	Path       string    `gorm:"size:512" json:"path"`
	IP         string    `gorm:"size:64" json:"ip"`
	DurationMs int64     `json:"duration_ms"`
	Detail     string    `gorm:"type:text" json:"detail"`
	CreatedAt  time.Time `gorm:"index;not null" json:"created_at"`
}

func (TableAuditLog) TableName() string { return "audit_logs" }

// WorkspaceModels is the AutoMigrate set for workspace feature tables.
func WorkspaceModels() []any {
	return []any{
		&TableAccessProfile{},
		&TableRBACRole{},
		&TableBusinessUnit{},
		&TableAlertChannel{},
		&TableCircuitBreakerPolicy{},
		&TableMCPToolGroup{},
		&TablePromptDeployment{},
		&TableVirtualKeyUser{},
		&TableTeamMember{},
		&TableWorkspaceSetting{},
		&TableAuditLog{},
	}
}
