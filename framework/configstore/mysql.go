package configstore

import (
	"context"
	"fmt"

	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/framework/mysqlconn"
	"gorm.io/gorm"
)

// MysqlConfig is the config_store MySQL/MariaDB connection config.
type MysqlConfig = mysqlconn.Config

// mysqlConfigFromAny accepts the shared env-shaped config used by Postgres
// (config.json + env.DB_*) or an explicit MysqlConfig.
func mysqlConfigFromAny(raw any) (*MysqlConfig, error) {
	switch c := raw.(type) {
	case *MysqlConfig:
		return c, nil
	case MysqlConfig:
		cp := c
		return &cp, nil
	case *PostgresConfig:
		// PostgresConfig is an alias of postgresconn.Config — same shape as MysqlConfig fields.
		return mysqlConfigFromPostgres(c), nil
	default:
		return nil, fmt.Errorf("invalid mysql config: %T", raw)
	}
}

func mysqlConfigFromPostgres(pg *PostgresConfig) *MysqlConfig {
	if pg == nil {
		return nil
	}
	return &MysqlConfig{
		Host:         pg.Host,
		Port:         pg.Port,
		User:         pg.User,
		Password:     pg.Password,
		DBName:       pg.DBName,
		SSLMode:      pg.SSLMode,
		MaxIdleConns: pg.MaxIdleConns,
		MaxOpenConns: pg.MaxOpenConns,
		ConnMaxLifetime: pg.ConnMaxLifetime,
	}
}

func newMysqlConfigStore(ctx context.Context, config *MysqlConfig, logger schemas.Logger) (ConfigStore, error) {
	if err := mysqlconn.Validate(config); err != nil {
		return nil, err
	}
	dsn := mysqlconn.BuildDSN(config)
	logger.Debug("configstore: mysql target host=%s port=%s db=%s",
		config.Host.GetValue(), config.Port.GetValue(), config.DBName.GetValue())

	logger.Info("configstore: opening mysql migration connection pool")
	mDb, err := mysqlconn.Open(dsn, newGormLogger(logger))
	if err != nil {
		logger.Error("configstore: failed to open mysql migration pool: %v", err)
		return nil, err
	}
	logger.Info("configstore: running schema migrations on mysql")
	if err := triggerMigrations(ctx, mDb, logger); err != nil {
		logger.Error("configstore: mysql schema migrations failed: %v", err)
		mysqlconn.Close(mDb, logger)
		return nil, err
	}
	mysqlconn.Close(mDb, logger)

	logger.Info("configstore: opening mysql runtime connection pool")
	db, err := mysqlconn.Open(dsn, newGormLogger(logger))
	if err != nil {
		return nil, err
	}
	if err := mysqlconn.ApplyPoolTuning(db, config); err != nil {
		mysqlconn.Close(db, logger)
		return nil, err
	}
	RegisterVaultCallbacks(db)

	d := &RDBConfigStore{logger: logger}
	d.db.Store(db)

	d.migrateOnFreshFn = func(ctx context.Context, fn func(context.Context, *gorm.DB) error) error {
		tempDB, err := mysqlconn.Open(dsn, newGormLogger(logger))
		if err != nil {
			return err
		}
		defer mysqlconn.Close(tempDB, logger)
		return fn(ctx, tempDB)
	}

	d.refreshPoolFn = func(ctx context.Context) error {
		newDB, err := mysqlconn.Open(dsn, newGormLogger(logger))
		if err != nil {
			return fmt.Errorf("failed to open fresh mysql runtime pool: %w", err)
		}
		if err := mysqlconn.ApplyPoolTuning(newDB, config); err != nil {
			mysqlconn.Close(newDB, logger)
			return err
		}
		RegisterVaultCallbacks(newDB)
		oldDB := d.db.Swap(newDB)
		if oldDB != nil {
			mysqlconn.Close(oldDB, logger)
		}
		return nil
	}

	logger.Info("configstore: encrypting plaintext rows if encryption is enabled")
	if err := d.EncryptPlaintextRows(ctx); err != nil {
		mysqlconn.Close(db, logger)
		return nil, fmt.Errorf("failed to encrypt plaintext rows: %w", err)
	}
	logger.Info("configstore: mysql config store ready")
	return d, nil
}
