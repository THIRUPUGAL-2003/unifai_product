package tables

import (
	"fmt"
	"time"

	"github.com/raksha/raksha/framework/encrypt"
	"gorm.io/gorm"
)

// TableSMTPConfig stores outbound email (SMTP) settings for dashboard auth mail.
type TableSMTPConfig struct {
	ID               uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Enabled          bool      `json:"enabled"`
	Host             string    `gorm:"type:varchar(255)" json:"host"`
	Port             int       `json:"port"`
	Username         string    `gorm:"type:varchar(255)" json:"username"`
	Password         string    `gorm:"type:text" json:"password"`
	FromEmail        string    `gorm:"type:varchar(255)" json:"from_email"`
	FromName         string    `gorm:"type:varchar(255)" json:"from_name"`
	UseTLS           bool      `json:"use_tls"`
	NotifyOnLogin    bool      `json:"notify_on_login"`
	NotifyOnUserCreate bool   `json:"notify_on_user_create"`
	EncryptionStatus string    `gorm:"type:varchar(20);default:'plain_text'" json:"-"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (TableSMTPConfig) TableName() string { return "config_smtp" }

func (s *TableSMTPConfig) BeforeSave(tx *gorm.DB) error {
	if encrypt.IsEnabled() && s.Password != "" {
		enc, err := encrypt.Encrypt(s.Password)
		if err != nil {
			return fmt.Errorf("encrypt smtp password: %w", err)
		}
		s.Password = enc
		s.EncryptionStatus = EncryptionStatusEncrypted
	}
	return nil
}

func (s *TableSMTPConfig) AfterFind(tx *gorm.DB) error {
	if s.EncryptionStatus == EncryptionStatusEncrypted && s.Password != "" {
		dec, err := encrypt.Decrypt(s.Password)
		if err != nil {
			return fmt.Errorf("decrypt smtp password: %w", err)
		}
		s.Password = dec
	}
	return nil
}

// TableLoginLockout tracks failed dashboard logins for lockout (3 fails → 20 min).
type TableLoginLockout struct {
	ID           uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	UsernameKey  string     `gorm:"type:varchar(255);uniqueIndex;not null" json:"username_key"`
	FailedCount  int        `json:"failed_count"`
	LockedUntil  *time.Time `json:"locked_until,omitempty"`
	LastFailedAt *time.Time `json:"last_failed_at,omitempty"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (TableLoginLockout) TableName() string { return "auth_login_lockouts" }

// TablePasswordResetOTP stores hashed one-time codes for password reset.
type TablePasswordResetOTP struct {
	ID             uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Username       string    `gorm:"type:varchar(255);index;not null" json:"username"`
	Email          string    `gorm:"type:varchar(255)" json:"email"`
	OTPHash        string    `gorm:"type:text;not null" json:"-"`
	ExpiresAt      time.Time `gorm:"index;not null" json:"expires_at"`
	Used           bool      `json:"used"`
	FailedAttempts int       `gorm:"default:0" json:"failed_attempts"`
	CreatedAt      time.Time `json:"created_at"`
}

func (TablePasswordResetOTP) TableName() string { return "auth_password_reset_otps" }

// TableLoginDevice tracks known login devices so "notify on login" emails
// fire only on first sign-in or a new device (IP + User-Agent fingerprint).
type TableLoginDevice struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UsernameKey  string    `gorm:"type:varchar(255);uniqueIndex:idx_login_device_user_fp;not null" json:"username_key"`
	Fingerprint  string    `gorm:"type:varchar(64);uniqueIndex:idx_login_device_user_fp;not null" json:"fingerprint"`
	UserAgent    string    `gorm:"type:varchar(512)" json:"user_agent"`
	IPAddress    string    `gorm:"type:varchar(64)" json:"ip_address"`
	FirstSeenAt  time.Time `json:"first_seen_at"`
	LastSeenAt   time.Time `json:"last_seen_at"`
}

func (TableLoginDevice) TableName() string { return "auth_login_devices" }
