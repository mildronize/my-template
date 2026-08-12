// Package platform holds the pieces every service built from this
// template keeps as-is on fork: config, logging, DB wiring, and HTTP
// server setup. It must never import a domain module (internal/todo,
// internal/identity) — see .chief/_rules/_standard/ARCHITECTURE.md rule 3.
package platform

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Config is the service's runtime configuration, populated from
// environment variables (with a .env file, if present, loaded first —
// real environment variables always win over .env).
type Config struct {
	// Port is the TCP port the HTTP server listens on.
	Port int `env:"PORT" envDefault:"8080"`

	// SSOIssuer is the Hydra issuer URL used to verify Bearer JWTs and to
	// locate its JWKS endpoint. Wired-but-dormant in this template — see
	// docs/GETTING-STARTED.md.
	SSOIssuer string `env:"SSO_ISSUER"`

	// AuthAudience is this service's own public URL, one per service per
	// environment (sso-consumer-contract.md §6) — never a literal/opaque
	// name. A forked service must set this to its real deployed URL.
	AuthAudience string `env:"AUTH_AUDIENCE"`

	// DatabasePath is the filesystem path to the SQLite database file.
	DatabasePath string `env:"DATABASE_PATH" envDefault:"./data/app.db"`
}

// LoadConfig loads a .env file if one exists in the working directory
// (silently skipped if absent — a missing .env is normal in prod, where
// config comes from real environment variables), then parses Config from
// the environment. Real environment variables always take precedence over
// values loaded from .env.
func LoadConfig() (*Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("loading .env: %w", err)
	}

	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config from environment: %w", err)
	}

	return &cfg, nil
}
