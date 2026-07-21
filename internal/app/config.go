// Package app wires the control-plane monolith.
package app

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config holds monolith runtime configuration.
type Config struct {
	Service  ServiceConfig
	Database DatabaseConfig
	Auth     AuthConfig
	Seed     SeedConfig
	NATS     NATSConfig
	SP       SPConfig
	Wiring   WiringConfig
}

type AuthConfig struct {
	Disabled     bool          `envconfig:"AUTH_DISABLED" default:"true"`
	ProxySecret  string        `envconfig:"AUTH_PROXY_SECRET"`
	AdminSubject string        `envconfig:"DCM_ADMIN_SUBJECT"`
	CacheTTL     time.Duration `envconfig:"AUTH_CACHE_TTL" default:"60s"`
	IssuerURL    string        `envconfig:"AUTH_ISSUER_URL"`
	Audience     string        `envconfig:"AUTH_JWT_AUDIENCE"`
}

type ServiceConfig struct {
	BindAddress       string        `envconfig:"BIND_ADDRESS" default:"127.0.0.1:8080"`
	LogLevel          string        `envconfig:"LOG_LEVEL" default:"info"`
	ReadHeaderTimeout time.Duration `envconfig:"READ_HEADER_TIMEOUT" default:"10s"`
	ReadTimeout       time.Duration `envconfig:"READ_TIMEOUT" default:"15s"`
	WriteTimeout      time.Duration `envconfig:"WRITE_TIMEOUT" default:"15s"`
	IdleTimeout       time.Duration `envconfig:"IDLE_TIMEOUT" default:"60s"`
	MaxHeaderBytes    int           `envconfig:"MAX_HEADER_BYTES" default:"1048576"`
}

type DatabaseConfig struct {
	Type     string `envconfig:"DB_TYPE" default:"pgsql"`
	Hostname string `envconfig:"DB_HOST" default:"localhost"`
	Port     string `envconfig:"DB_PORT" default:"5432"`
	Name     string `envconfig:"DB_NAME" default:"control-plane"`
	User     string `envconfig:"DB_USER"`
	Password string `envconfig:"DB_PASSWORD"`
}

type SeedConfig struct {
	RegionDefault string   `envconfig:"SEED_REGION_DEFAULT" default:""`
	RegionEnum    []string `envconfig:"SEED_REGION_ENUM" default:"region-a,region-b"`
}

type NATSConfig struct {
	URL          string `envconfig:"NATS_URL" default:"nats://localhost:4222"`
	Subject      string `envconfig:"NATS_STATUS_SUBJECT" default:"dcm.*"`
	StreamName   string `envconfig:"NATS_STREAM_NAME" default:"dcm-status"`
	ConsumerName string `envconfig:"NATS_CONSUMER_NAME" default:"control-plane"`
	Disabled     bool   `envconfig:"NATS_DISABLED" default:"false"`
}

type SPConfig struct {
	HealthCheckInterval               time.Duration `envconfig:"HEALTH_CHECK_INTERVAL" default:"10s"`
	HealthCheckTimeout                time.Duration `envconfig:"HEALTH_CHECK_TIMEOUT" default:"5s"`
	HealthCheckMaxConsecutiveFailures int           `envconfig:"HEALTH_CHECK_MAX_CONSECUTIVE_FAILURES" default:"3"`
	HealthCheckBaseBackoffInterval    time.Duration `envconfig:"HEALTH_CHECK_BASE_BACKOFF_INTERVAL" default:"10s"`
	HealthCheckMaxBackoffInterval     time.Duration `envconfig:"HEALTH_CHECK_MAX_BACKOFF_INTERVAL" default:"5m"`
	CleanupInterval                   time.Duration `envconfig:"CLEANUP_INTERVAL" default:"1m"`
	CleanupMaxRetries                 int           `envconfig:"CLEANUP_MAX_RETRIES" default:"10"`
	CleanupTimeout                    time.Duration `envconfig:"CLEANUP_TIMEOUT" default:"10s"`
}

// WiringConfig holds optional HTTP client URLs for subsystem tests.
// When empty, cross-domain calls use in-process implementations.
type WiringConfig struct {
	PlacementManagerURL     string        `envconfig:"PLACEMENT_MANAGER_URL"`
	PlacementManagerTimeout time.Duration `envconfig:"PLACEMENT_MANAGER_TIMEOUT" default:"10s"`
}

func LoadConfig() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	if cfg.Database.Password == "" {
		if pass := strings.TrimSpace(os.Getenv("DB_PASS")); pass != "" {
			cfg.Database.Password = pass
		}
	}
	if err := validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func validate(cfg *Config) error {
	dbType, err := normalizeDBType(cfg.Database.Type)
	if err != nil {
		return err
	}
	cfg.Database.Type = dbType
	if !cfg.Auth.Disabled && cfg.Auth.AdminSubject == "" {
		return errors.New("DCM_ADMIN_SUBJECT is required when AUTH_DISABLED is not set")
	}
	if dbType != "pgsql" {
		return nil
	}
	if cfg.Database.User == "" {
		return errors.New("DB_USER is required when DB_TYPE is pgsql")
	}
	if cfg.Database.Password == "" {
		return errors.New("DB_PASSWORD is required when DB_TYPE is pgsql")
	}
	return nil
}

func normalizeDBType(dbType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(dbType)) {
	case "pgsql", "postgres", "postgresql":
		return "pgsql", nil
	case "sqlite":
		return "sqlite", nil
	default:
		return "", fmt.Errorf("unsupported DB_TYPE %q (supported: pgsql, sqlite)", dbType)
	}
}
