package app

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dcm-project/control-plane/internal/auth"
	authservice "github.com/dcm-project/control-plane/internal/auth/service"
	authstore "github.com/dcm-project/control-plane/internal/auth/store"
	catalogserver "github.com/dcm-project/control-plane/internal/catalog/api/server"
	catalogconfig "github.com/dcm-project/control-plane/internal/catalog/config"
	cataloghandlers "github.com/dcm-project/control-plane/internal/catalog/handlers/v1alpha1"
	catalogplacement "github.com/dcm-project/control-plane/internal/catalog/placement"
	catalogservice "github.com/dcm-project/control-plane/internal/catalog/service"
	catalogstore "github.com/dcm-project/control-plane/internal/catalog/store"
	placementlogging "github.com/dcm-project/control-plane/internal/placement/logging"
	placementpolicy "github.com/dcm-project/control-plane/internal/placement/policy"
	placementservice "github.com/dcm-project/control-plane/internal/placement/service"
	placementsprm "github.com/dcm-project/control-plane/internal/placement/sprm"
	placementstore "github.com/dcm-project/control-plane/internal/placement/store"
	policyserver "github.com/dcm-project/control-plane/internal/policy/api/server"
	policyhandlers "github.com/dcm-project/control-plane/internal/policy/handlers/v1alpha1"
	policylogging "github.com/dcm-project/control-plane/internal/policy/logging"
	policyopa "github.com/dcm-project/control-plane/internal/policy/opa"
	policyservice "github.com/dcm-project/control-plane/internal/policy/service"
	policystore "github.com/dcm-project/control-plane/internal/policy/store"
	spproviderserver "github.com/dcm-project/control-plane/internal/sp/api/provider"
	sprmserver "github.com/dcm-project/control-plane/internal/sp/api/resource_manager"
	spcleanup "github.com/dcm-project/control-plane/internal/sp/cleanup"
	spconfig "github.com/dcm-project/control-plane/internal/sp/config"
	spconsumer "github.com/dcm-project/control-plane/internal/sp/consumer"
	spproviderhandler "github.com/dcm-project/control-plane/internal/sp/handlers/provider"
	sprmhandler "github.com/dcm-project/control-plane/internal/sp/handlers/resource_manager"
	sphealthcheck "github.com/dcm-project/control-plane/internal/sp/healthcheck"
	splogging "github.com/dcm-project/control-plane/internal/sp/logging"
	spprovidersvc "github.com/dcm-project/control-plane/internal/sp/service/provider"
	sprmsvc "github.com/dcm-project/control-plane/internal/sp/service/resource_manager"
	spstore "github.com/dcm-project/control-plane/internal/sp/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const gracefulShutdownTimeout = 5 * time.Second

// Run starts the control-plane monolith.
func Run() int {
	cfg, err := LoadConfig()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		return 1
	}

	logger := initLogger(cfg.Service.LogLevel)
	slog.SetDefault(logger)
	placementlogging.Init(cfg.Service.LogLevel)
	policylogging.Init(cfg.Service.LogLevel)
	splogging.Init(cfg.Service.LogLevel)

	slog.Info("Configuration loaded",
		"bind_address", cfg.Service.BindAddress,
		"db_type", cfg.Database.Type,
		"db_host", cfg.Database.Hostname,
		"db_name", cfg.Database.Name,
		"log_level", cfg.Service.LogLevel,
	)

	db, err := openDB(cfg)
	if err != nil {
		slog.Error("Failed to initialize database", "error", err)
		return 1
	}
	defer func() { _ = closeDB(db) }()

	authDataStore := authstore.NewStore(db)
	authSvc := authservice.NewService(authDataStore, cfg.Auth.AdminSubject, logger)

	catalogDataStore := catalogstore.NewStore(db, logger)
	policyDataStore := policystore.NewStore(db)
	placementDataStore := placementstore.NewStore(db)
	spDataStore := spstore.NewStore(db)

	opaEngine := policyopa.NewEngine()
	policyService := policyservice.NewPolicyService(policyDataStore, opaEngine)
	evaluationService := policyservice.NewEvaluationService(policyDataStore.Policy(), opaEngine)
	if err := policyService.CompileAll(context.Background()); err != nil {
		slog.Error("Failed to compile policies on startup", "error", err)
		return 1
	}

	spProviderService := spprovidersvc.NewProviderService(spDataStore)
	spInstanceService := sprmsvc.NewInstanceService(spDataStore, nil)

	policyClient := placementpolicy.NewServiceClient(evaluationService)
	sprmClient := placementsprm.NewServiceClient(spInstanceService)

	placementService := placementservice.NewPlacementService(placementDataStore, policyClient, sprmClient)
	pmClient, err := buildPlacementClient(cfg, placementService, logger)
	if err != nil {
		slog.Error("Failed to initialize placement client", "error", err)
		return 1
	}

	seedCfg := catalogconfig.SeedConfig{
		RegionDefault: cfg.Seed.RegionDefault,
		RegionEnum:    cfg.Seed.RegionEnum,
	}
	catalogSvc, err := catalogservice.NewService(catalogDataStore, pmClient, seedCfg, logger)
	if err != nil {
		slog.Error("Failed to create catalog service", "error", err)
		return 1
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := authSvc.Seed(ctx); err != nil {
		slog.Error("Failed to seed auth database", "error", err)
		return 1
	}

	seedCtx := auth.WithActorInfo(ctx, auth.ActorInfo{
		ActorType: "system",
	})
	if err := catalogSvc.Seed(seedCtx); err != nil {
		slog.Error("Failed to seed catalog database", "error", err)
		return 1
	}

	if !cfg.NATS.Disabled {
		statusConsumer, err := spconsumer.New(cfg.NATS.URL, cfg.NATS.Subject, spDataStore,
			spconsumer.SetStreamName(cfg.NATS.StreamName),
			spconsumer.SetConsumerName(cfg.NATS.ConsumerName),
		)
		if err != nil {
			slog.Error("Failed to initialize status consumer", "error", err)
			return 1
		}
		if err := statusConsumer.Start(ctx); err != nil {
			slog.Error("Failed to start status consumer", "error", err)
			return 1
		}
		defer statusConsumer.Stop()
	}

	healthMonitor := sphealthcheck.NewMonitor(
		spDataStore.Provider(),
		spDataStore.ServiceTypeInstance(),
		&spconfig.HealthCheckConfig{
			Interval:               cfg.SP.HealthCheckInterval,
			Timeout:                cfg.SP.HealthCheckTimeout,
			MaxConsecutiveFailures: cfg.SP.HealthCheckMaxConsecutiveFailures,
			BaseBackoffInterval:    cfg.SP.HealthCheckBaseBackoffInterval,
			MaxBackoffInterval:     cfg.SP.HealthCheckMaxBackoffInterval,
		},
	)
	healthMonitor.Start(ctx)
	defer healthMonitor.Stop()

	cleanupScheduler := spcleanup.NewScheduler(spDataStore, spInstanceService, &spconfig.CleanupConfig{
		Interval:   cfg.SP.CleanupInterval,
		MaxRetries: cfg.SP.CleanupMaxRetries,
		Timeout:    cfg.SP.CleanupTimeout,
	})
	cleanupScheduler.Start(ctx)
	defer cleanupScheduler.Stop()

	var authMiddleware func(http.Handler) http.Handler
	if cfg.Auth.Disabled {
		authMiddleware = auth.DisabledMiddleware(logger)
	} else {
		if cfg.Auth.ProxySecret == "" && cfg.Auth.IssuerURL == "" {
			slog.Error("AUTH_PROXY_SECRET or AUTH_ISSUER_URL is required when authentication is enabled")
			return 1
		}
		actorCache := auth.NewActorCache(cfg.Auth.CacheTTL)
		mwCfg := auth.MiddlewareConfig{
			ProxySecret: cfg.Auth.ProxySecret,
			Resolver:    authSvc,
			Cache:       actorCache,
			Logger:      logger,
		}
		if cfg.Auth.IssuerURL != "" {
			jwtValidator, err := auth.NewOIDCValidator(ctx, cfg.Auth.IssuerURL, cfg.Auth.Audience)
			if err != nil {
				slog.Error("Failed to initialize OIDC JWT validator", "error", err, "issuer", cfg.Auth.IssuerURL)
				return 1
			}
			mwCfg.JWTValidator = jwtValidator
			if cfg.Auth.Audience == "" {
				slog.Warn("AUTH_JWT_AUDIENCE is empty - audience validation is disabled, tokens from any client in this issuer realm will be accepted", "issuer", cfg.Auth.IssuerURL, "component", "auth")
			}
			slog.Info("JWT bearer validation enabled", "issuer", cfg.Auth.IssuerURL, "component", "auth")
		}
		authMiddleware = auth.Middleware(mwCfg)
	}

	router, err := newRouter(authMiddleware, RouteHandlers{
		Catalog:    cataloghandlers.NewHandler(catalogSvc, logger),
		Policy:     policyhandlers.NewPolicyHandler(policyService),
		SPProvider: spproviderhandler.NewHandler(spProviderService),
		SPRM:       sprmhandler.NewHandler(spInstanceService),
	})
	if err != nil {
		slog.Error("Failed to configure HTTP router", "error", err)
		return 1
	}

	listener, err := net.Listen("tcp", cfg.Service.BindAddress)
	if err != nil {
		slog.Error("Failed to create listener", "error", err)
		return 1
	}

	srv := &http.Server{
		Handler:           router,
		ReadHeaderTimeout: cfg.Service.ReadHeaderTimeout,
		ReadTimeout:       cfg.Service.ReadTimeout,
		WriteTimeout:      cfg.Service.WriteTimeout,
		IdleTimeout:       cfg.Service.IdleTimeout,
		MaxHeaderBytes:    cfg.Service.MaxHeaderBytes,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
		defer shutdownCancel()
		srv.SetKeepAlivesEnabled(false)
		slog.Info("Shutting down server")
		_ = srv.Shutdown(shutdownCtx)
	}()

	slog.Info("Starting control-plane", "address", listener.Addr().String())
	if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("Server failed", "error", err)
		return 1
	}
	return 0
}

type RouteHandlers struct {
	Catalog    catalogserver.StrictServerInterface
	Policy     policyserver.StrictServerInterface
	SPProvider spproviderserver.StrictServerInterface
	SPRM       sprmserver.StrictServerInterface
}

func newRouter(authMW func(http.Handler) http.Handler, h RouteHandlers) (chi.Router, error) {
	validators, err := newOpenAPIValidators()
	if err != nil {
		return nil, err
	}

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)
	router.Use(authMW)
	router.Use(validators.middleware())

	const baseURL = "/api/v1alpha1"

	// Single monolith health endpoint; domain OpenAPI specs omit /health to avoid
	// duplicate chi route registration when mounting multiple generated servers.
	registerMonolithHealth(router)

	catalogserver.HandlerFromMuxWithBaseURL(
		catalogserver.NewStrictHandler(h.Catalog, nil),
		router,
		baseURL,
	)
	policyserver.HandlerFromMuxWithBaseURL(
		policyserver.NewStrictHandler(h.Policy, nil),
		router,
		baseURL,
	)

	apiRouter := chi.NewRouter()
	spproviderserver.HandlerFromMux(
		spproviderserver.NewStrictHandler(h.SPProvider, nil),
		apiRouter,
	)
	sprmserver.HandlerFromMux(
		sprmserver.NewStrictHandler(h.SPRM, nil),
		apiRouter,
	)
	router.Mount(baseURL, apiRouter)

	return router, nil
}

func buildPlacementClient(cfg *Config, svc *placementservice.PlacementService, logger *slog.Logger) (catalogplacement.Client, error) {
	if url := strings.TrimSpace(cfg.Wiring.PlacementManagerURL); url != "" {
		return catalogplacement.NewClient(url, cfg.Wiring.PlacementManagerTimeout, logger)
	}
	return catalogplacement.NewLocalClient(svc, logger), nil
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
