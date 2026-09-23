// Package app implements the dcm-gitops controller process.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	agentstore "github.com/dcm-project/control-plane/internal/agent/store/agent"
	catalogconfig "github.com/dcm-project/control-plane/internal/catalog/config"
	catalogplacement "github.com/dcm-project/control-plane/internal/catalog/placement"
	catalogservice "github.com/dcm-project/control-plane/internal/catalog/service"
	catalogstore "github.com/dcm-project/control-plane/internal/catalog/store"
	"github.com/dcm-project/control-plane/internal/gitops/config"
	"github.com/dcm-project/control-plane/internal/gitops/controller"
	gitopsstore "github.com/dcm-project/control-plane/internal/gitops/store"
	gitopsmodel "github.com/dcm-project/control-plane/internal/gitops/store/model"
	placementagent "github.com/dcm-project/control-plane/internal/placement/agent"
	placementpolicy "github.com/dcm-project/control-plane/internal/placement/policy"
	placementservice "github.com/dcm-project/control-plane/internal/placement/service"
	placementsprm "github.com/dcm-project/control-plane/internal/placement/sprm"
	placementstore "github.com/dcm-project/control-plane/internal/placement/store"
	policyopa "github.com/dcm-project/control-plane/internal/policy/opa"
	policyservice "github.com/dcm-project/control-plane/internal/policy/service"
	policystore "github.com/dcm-project/control-plane/internal/policy/store"
	"github.com/dcm-project/control-plane/internal/sp/messaging"
	sprmsvc "github.com/dcm-project/control-plane/internal/sp/service/resource_manager"
	spstore "github.com/dcm-project/control-plane/internal/sp/store"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Run starts the dcm-gitops controller process.
func Run() int {
	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		return 1
	}

	slogLogger := initLogger(cfg.LogLevel)
	slog.SetDefault(slogLogger)

	slog.Info("dcm-gitops starting",
		"db_type", cfg.Database.Type,
		"db_host", cfg.Database.Hostname,
		"db_name", cfg.Database.Name,
		"git_work_dir", cfg.GitWorkDir,
		"poll_interval", cfg.PollInterval,
		"nats_disabled", cfg.NATS.Disabled,
		"nats_url", cfg.NATS.URL,
	)

	db, err := openDB(cfg)
	if err != nil {
		slog.Error("Failed to initialize database", "error", err)
		return 1
	}
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()

	// Migrate gitops schema
	if err := db.AutoMigrate(&gitopsmodel.GitRepository{}, &gitopsmodel.ManagedInstance{}); err != nil {
		slog.Error("Failed to migrate gitops schema", "error", err)
		return 1
	}

	// Create stores
	gitopsDataStore := gitopsstore.NewStore(db)
	catalogDataStore := catalogstore.NewStore(db, slogLogger)
	policyDataStore := policystore.NewStore(db)
	placementDataStore := placementstore.NewStore(db)
	spDataStore := spstore.NewStore(db)
	agentSt := agentstore.NewAgent(db)
	agentClient := placementagent.NewServiceClient(agentSt)

	// Build catalog service (same wiring as dcm-server)
	opaEngine := policyopa.NewEngine()
	policySvc := policyservice.NewPolicyService(policyDataStore, opaEngine)
	evaluationSvc := policyservice.NewEvaluationService(policyDataStore.Policy(), opaEngine)
	if err := policySvc.CompileAll(context.Background()); err != nil {
		slog.Error("Failed to compile policies on startup", "error", err)
		return 1
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var publisher *messaging.Publisher
	if !cfg.NATS.Disabled {
		agentConn, err := messaging.Connect(ctx, cfg.NATS.URL)
		if err != nil {
			slog.Error("Failed to initialize agent messaging", "error", err)
			return 1
		}
		defer agentConn.Close()
		publisher = agentConn.Publisher
	}

	spInstanceSvc := sprmsvc.NewInstanceService(spDataStore, publisher, agentSt)
	policyClient := placementpolicy.NewServiceClient(evaluationSvc)
	sprmClient := placementsprm.NewServiceClient(spInstanceSvc)
	placementSvc := placementservice.NewPlacementService(
		placementDataStore, policyClient, sprmClient,
		placementservice.WithAgentClient(agentClient),
	)

	pmClient := catalogplacement.NewLocalClient(placementSvc, slogLogger)

	catalogSvc, err := catalogservice.NewService(catalogDataStore, pmClient, catalogconfig.SeedConfig{}, slogLogger)
	if err != nil {
		slog.Error("Failed to create catalog service", "error", err)
		return 1
	}

	// Create git client and reconciler
	gitClient := controller.NewGitClient(cfg.GitWorkDir)
	reconciler := controller.NewReconciler(gitopsDataStore, catalogSvc.CatalogItemInstance(), catalogSvc.CatalogItem(), gitClient)

	// Create and start controller
	pollInterval := time.Duration(cfg.PollInterval) * time.Second
	ctrl := controller.NewController(reconciler, gitopsDataStore, pollInterval)

	ctrl.Start(ctx)
	defer ctrl.Stop()

	slog.Info("dcm-gitops controller started")

	<-ctx.Done()
	slog.Info("dcm-gitops shutting down")
	return 0
}

func openDB(cfg *config.Config) (*gorm.DB, error) {
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

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
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

	return db, nil
}

func initLogger(level string) *slog.Logger {
	var logLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn", "warning":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
}
