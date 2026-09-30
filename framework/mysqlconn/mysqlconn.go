package mysqlconn

import (
	"fmt"
	"strings"
	"time"

	"github.com/unifai/unifai/core/schemas"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Config is the MySQL/MariaDB connection shape. JSON tags match Postgres
// config_store fields so the same env.DB_HOST / DB_USER / … values work.
type Config struct {
	Host         *schemas.SecretVar `json:"host"`
	Port         *schemas.SecretVar `json:"port"`
	User         *schemas.SecretVar `json:"user"`
	Password     *schemas.SecretVar `json:"password"`
	DBName       *schemas.SecretVar `json:"db_name"`
	SSLMode      *schemas.SecretVar `json:"ssl_mode"`
	MaxIdleConns int                `json:"max_idle_conns"`
	MaxOpenConns int                `json:"max_open_conns"`
	ConnMaxLifetime string             `json:"conn_max_lifetime,omitempty"`
}

// Validate checks required MySQL connection fields.
func Validate(config *Config) error {
	if config == nil {
		return fmt.Errorf("config is required")
	}
	if config.Host == nil || strings.TrimSpace(config.Host.GetValue()) == "" {
		return fmt.Errorf("mysql host is required")
	}
	if config.Port == nil || strings.TrimSpace(config.Port.GetValue()) == "" {
		return fmt.Errorf("mysql port is required")
	}
	if config.User == nil || strings.TrimSpace(config.User.GetValue()) == "" {
		return fmt.Errorf("mysql user is required")
	}
	if config.DBName == nil || strings.TrimSpace(config.DBName.GetValue()) == "" {
		return fmt.Errorf("mysql db name is required")
	}
	if config.Password == nil {
		return fmt.Errorf("mysql password is required")
	}
	return nil
}

// BuildDSN builds a go-sql-driver DSN.
// ssl_mode mapping: disable→false, require/verify-*→true, skip-verify→skip-verify.
func BuildDSN(config *Config) string {
	password := ""
	if config.Password != nil {
		password = config.Password.GetValue()
	}
	tls := mapMySQLTLS(config.SSLMode)
	// parseTime required for GORM time.Time; utf8mb4 for full Unicode.
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=UTC&tls=%s",
		config.User.GetValue(),
		password,
		config.Host.GetValue(),
		config.Port.GetValue(),
		config.DBName.GetValue(),
		tls,
	)
}

func mapMySQLTLS(ssl *schemas.SecretVar) string {
	if ssl == nil {
		return "false"
	}
	switch strings.ToLower(strings.TrimSpace(ssl.GetValue())) {
	case "", "disable", "false", "0":
		return "false"
	case "skip-verify", "prefer":
		return "skip-verify"
	default:
		// require, verify-ca, verify-full, true, …
		return "true"
	}
}

// Open opens a *gorm.DB against MySQL/MariaDB.
func Open(dsn string, logger gormlogger.Interface) (*gorm.DB, error) {
	return gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger})
}

// ApplyPoolTuning applies optional pool settings.
func ApplyPoolTuning(db *gorm.DB, config *Config) error {
	if db == nil || config == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	if config.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(config.MaxIdleConns)
	}
	if config.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(config.MaxOpenConns)
	}
	if strings.TrimSpace(config.ConnMaxLifetime) != "" {
		d, err := time.ParseDuration(config.ConnMaxLifetime)
		if err != nil {
			return fmt.Errorf("invalid conn_max_lifetime: %w", err)
		}
		sqlDB.SetConnMaxLifetime(d)
	}
	return nil
}

// Close closes the underlying sql.DB.
func Close(db *gorm.DB, logger schemas.Logger) {
	if db == nil {
		return
	}
	sqlDB, err := db.DB()
	if err != nil {
		if logger != nil {
			logger.Warn("mysqlconn: failed to get sql.DB for close: %v", err)
		}
		return
	}
	if err := sqlDB.Close(); err != nil && logger != nil {
		logger.Warn("mysqlconn: close error: %v", err)
	}
}
