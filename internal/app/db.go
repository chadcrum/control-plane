package app

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	authmodel "github.com/dcm-project/control-plane/internal/auth/store/model"
	catalogmodel "github.com/dcm-project/control-plane/internal/catalog/store/model"
	placementmodel "github.com/dcm-project/control-plane/internal/placement/store/model"
	policymodel "github.com/dcm-project/control-plane/internal/policy/store/model"
	spmodel "github.com/dcm-project/control-plane/internal/sp/store/model"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openDB(cfg *Config) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch cfg.Database.Type {
	case "pgsql":
		dsn := fmt.Sprintf("host=%s user=%s password=%s port=%s dbname=%s",
			cfg.Database.Hostname,
			cfg.Database.User,
			cfg.Database.Password,
			cfg.Database.Port,
			cfg.Database.Name,
		)
		dialector = postgres.Open(dsn)
	case "sqlite":
		dialector = sqlite.Open(cfg.Database.Name)
	default:
		return nil, fmt.Errorf("unsupported DB_TYPE %q", cfg.Database.Type)
	}

	gormLogLevel, slogLevel := gormLogLevelFromString(cfg.Service.LogLevel)
	gormLogger := logger.New(
		slog.NewLogLogger(slog.Default().Handler(), slogLevel),
		logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  gormLogLevel,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger:         gormLogger,
		TranslateError: true,
	})
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}

	if cfg.Database.Type == "sqlite" {
		if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
			return nil, fmt.Errorf("enable sqlite foreign keys: %w", err)
		}
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("underlying db handle: %w", err)
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if err := db.AutoMigrate(
		&authmodel.Actor{},
		&authmodel.ActorIdentity{},
		&catalogmodel.ServiceType{},
		&catalogmodel.CatalogItem{},
		&catalogmodel.CatalogItemInstance{},
		&placementmodel.Resource{},
		&policymodel.Policy{},
		&spmodel.Provider{},
		&spmodel.ServiceTypeInstance{},
	); err != nil {
		return nil, fmt.Errorf("migrate schema: %w", err)
	}

	slog.Info("Database connection established", "type", cfg.Database.Type)
	return db, nil
}

func gormLogLevelFromString(level string) (logger.LogLevel, slog.Level) {
	switch strings.ToLower(level) {
	case "debug":
		return logger.Info, slog.LevelDebug
	case "info":
		return logger.Info, slog.LevelInfo
	case "warn", "warning":
		return logger.Warn, slog.LevelWarn
	case "error":
		return logger.Error, slog.LevelError
	default:
		return logger.Warn, slog.LevelWarn
	}
}

func closeDB(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
