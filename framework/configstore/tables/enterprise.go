package tables

import "time"

// TableEnterpriseRecord is a typed JSON document store for workspace features
// that persist in the config database (access profiles, RBAC, circuit breaker
// policies, alert channels, and similar).
type TableEnterpriseRecord struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Kind      string    `gorm:"size:64;uniqueIndex:idx_ent_kind_key;not null" json:"kind"`
	Key       string    `gorm:"size:255;uniqueIndex:idx_ent_kind_key;not null" json:"key"`
	Data      string    `gorm:"type:text;not null" json:"data"`
	CreatedAt time.Time `gorm:"index;not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"index;not null" json:"updated_at"`
}

func (TableEnterpriseRecord) TableName() string { return "enterprise_records" }

// TableEnterpriseAuditLog stores administrative activity for the Audit Logs page.
type TableEnterpriseAuditLog struct {
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

func (TableEnterpriseAuditLog) TableName() string { return "enterprise_audit_logs" }
