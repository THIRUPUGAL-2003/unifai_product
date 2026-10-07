package tables

import (
	"time"
)

// User registration / approval statuses.
const (
	UserStatusPending  = "pending"
	UserStatusApproved = "approved"
	UserStatusRejected = "rejected"
	// UserStatusEmailUnverified: self-registered, waiting for the emailed sign-up code.
	UserStatusEmailUnverified = "email_unverified"
	// UserStatusDisabled: deactivated by the identity provider (SCIM active=false).
	UserStatusDisabled = "disabled"
)

// TableUser represents a custom user record stored in PostgreSQL
type TableUser struct {
	ID                 string     `gorm:"primaryKey;type:varchar(255)" json:"id"`
	Username           string     `gorm:"type:varchar(255);uniqueIndex;not null" json:"username"`
	Email              string     `gorm:"type:varchar(255);index" json:"email"`
	Password           string     `gorm:"type:text;not null" json:"password"`
	Role               string     `gorm:"type:varchar(50);not null" json:"role"`                          // "admin" or "user"
	Status             string     `gorm:"type:varchar(50);not null;default:approved;index" json:"status"` // pending | approved | rejected
	Budget             float64    `json:"budget"`                                                         // Cost limit in USD (UI); materialized to BudgetID row
	RateLimit          int        `json:"rate_limit"`                                                     // Requests per minute (RPM); materialized to RateLimitID row
	BudgetID           *string    `gorm:"type:varchar(255);index" json:"budget_id,omitempty"`             // Live TableBudget owner for this user
	RateLimitID        *string    `gorm:"type:varchar(255);index" json:"rate_limit_id,omitempty"`         // Live TableRateLimit for this user
	AllowedPromptRepos string     `gorm:"type:text" json:"allowed_prompt_repos"`                          // Comma-separated allowed prompt IDs
	AllowedSections    string     `gorm:"type:text" json:"allowed_sections"`                              // Comma-separated sidebar section keys for role=user
	ReviewedAt         *time.Time `json:"reviewed_at,omitempty"`
	MustChangePassword bool       `gorm:"default:false" json:"must_change_password"`
	ExternalID         string     `gorm:"type:varchar(255);index" json:"external_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// TableName returns the table name for custom users
func (TableUser) TableName() string {
	return "governance_users"
}

// IsApproved reports whether the user may log in.
func (u *TableUser) IsApproved() bool {
	if u == nil {
		return false
	}
	s := u.Status
	if s == "" {
		return true // legacy rows before status column
	}
	return s == UserStatusApproved
}
