// Package config provides configuration loading for the dcm-gitops process.
package config

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
)

// Config holds configuration for the dcm-gitops process.
type Config struct {
	Database   DatabaseConfig
	NATS       NATSConfig
	LogLevel   string `envconfig:"LOG_LEVEL" default:"info"`
	GitWorkDir string `envconfig:"GIT_WORK_DIR" default:"/tmp/dcm-gitops"`
	// PollInterval is how often (in seconds) the controller reloads the list of repos from the DB.
	PollInterval int `envconfig:"POLL_INTERVAL" default:"15"`
}

// DatabaseConfig holds database connection settings.
type DatabaseConfig struct {
	Type     string `envconfig:"DB_TYPE" default:"pgsql"`
	Hostname string `envconfig:"DB_HOST" default:"localhost"`
	Port     string `envconfig:"DB_PORT" default:"5432"`
	Name     string `envconfig:"DB_NAME" default:"control-plane"`
	User     string `envconfig:"DB_USER"`
	Password string `envconfig:"DB_PASSWORD"`
}

// NATSConfig holds NATS settings used to publish agent create requests.
// Response/status consumption stays on the control-plane process.
type NATSConfig struct {
	URL      string `envconfig:"NATS_URL" default:"nats://localhost:4222"`
	Disabled bool   `envconfig:"NATS_DISABLED" default:"false"`
}

// LoadConfig reads configuration from environment variables.
func LoadConfig() (*Config, error) {
	cfg := &Config{}
	if err := envconfig.Process("", cfg); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if cfg.PollInterval < 1 {
		return nil, fmt.Errorf("POLL_INTERVAL must be >= 1 second, got %d", cfg.PollInterval)
	}
	return cfg, nil
}
