package logstore

import (
	"context"
	"fmt"

	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/framework/mysqlconn"
)

// MysqlConfig is logs_store MySQL/MariaDB config (same JSON shape as Postgres).
type MysqlConfig = mysqlconn.Config

func mysqlLogConfigFromAny(raw any) (*MysqlConfig, error) {
	switch c := raw.(type) {
	case *MysqlConfig:
		return c, nil
	case MysqlConfig:
		cp := c
		return &cp, nil
	case *PostgresConfig:
		// PostgresConfig embeds postgresconn.Config fields; reuse for DB_TYPE override.
		return &MysqlConfig{
			Host:         c.Host,
			Port:         c.Port,
			User:         c.User,
			Password:     c.Password,
			DBName:       c.DBName,
			SSLMode:      c.SSLMode,
			MaxIdleConns: c.MaxIdleConns,
			MaxOpenConns: c.MaxOpenConns,
			ConnMaxLifetime: c.ConnMaxLifetime,
		}, nil
	default:
		return nil, fmt.Errorf("invalid mysql logstore config: %T", raw)
	}
}

// newMysqlLogStore opens MySQL for logs. Postgres-only features (matviews, GIN
// indexes, version gate) are skipped — core log CRUD/search still work.
func newMysqlLogStore(ctx context.Context, config *MysqlConfig, logger schemas.Logger) (LogStore, error) {
	if err := mysqlconn.Validate(config); err != nil {
		return nil, err
	}
	dsn := mysqlconn.BuildDSN(config)
	logger.Debug("logstore: mysql target host=%s port=%s db=%s",
		config.Host.GetValue(), config.Port.GetValue(), config.DBName.GetValue())

	logger.Info("logstore: opening mysql migration connection pool")
	mDb, err := mysqlconn.Open(dsn, newGormLogger(logger))
	if err != nil {
		return nil, err
	}
	logger.Info("logstore: running schema migrations on mysql")
	if err := triggerMigrations(ctx, mDb, logger); err != nil {
		sqlDB, _ := mDb.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		return nil, err
	}
	if sqlDB, err := mDb.DB(); err == nil {
		_ = sqlDB.Close()
	}

	logger.Info("logstore: opening mysql runtime connection pool")
	db, err := mysqlconn.Open(dsn, newGormLogger(logger))
	if err != nil {
		return nil, err
	}
	if err := mysqlconn.ApplyPoolTuning(db, config); err != nil {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		return nil, err
	}

	logger.Info("logstore: mysql log store ready (matviews/GIN indexes are Postgres-only and skipped)")
	return &RDBLogStore{db: db, logger: logger}, nil
}
