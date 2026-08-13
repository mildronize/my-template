// Package platform holds the pieces every service built from this
// template keeps as-is on fork: config, logging, DB wiring, HTTP server
// setup, and the cross-cutting gin middleware (middleware.go) shared by
// every transport engine. It must never import a domain module
// (internal/domain/*), internal/identity, or an internal/transport/*
// surface — see .chief/_rules/_standard/ARCHITECTURE.md rule 5.
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

	// SSOClientID/SSOClientSecret are internal/transport/bff's Hydra OAuth2
	// client credentials for the owner-login flow (authorization_code +
	// PKCE, sso-consumer-contract.md §2) — printed by scripts/register.sh
	// on success (docs/DEPLOY-REQUIREMENTS.md). Unlike SSOIssuer/
	// AuthAudience's JWT-path dormant seam, leaving these unset does not
	// stop cmd/server from starting: GETTING-STARTED.md's own walkthrough
	// (steps 1-5) never touches bff, so the server and the public API must
	// keep working with no Hydra client registered yet. internal/transport/
	// bff's own wiring (cmd/server/main.go's wireBFF) checks these and
	// simply serves a clear "owner login isn't configured" error from
	// GET /login and GET /callback instead of a working flow when either is
	// empty — the same pattern wireIdentity already uses for the JWT branch.
	SSOClientID     string `env:"SSO_CLIENT_ID"`
	SSOClientSecret string `env:"SSO_CLIENT_SECRET"`

	// SessionSecret HMAC-signs/verifies internal/transport/bff's session
	// and state cookies (DATA_MODEL.md's "BFF session" note — no
	// server-side session store; the signature itself is the whole
	// validity proof). Deliberately not required at startup, unlike the
	// fields above staying-optional-by-design being an availability
	// concern — this one is a security concern instead, so cmd/server
	// doesn't silently run with an empty (trivially forgeable) key: if
	// unset, it generates a random one for that process's lifetime and
	// logs a warning that existing sessions won't survive a restart. Set
	// this explicitly for any real deployment (docs/DEPLOY-REQUIREMENTS.md).
	SessionSecret string `env:"SESSION_SECRET"`

	// SeedOwnerSSOSubject is the owner's known Hydra `sub` claim,
	// cmd/seed's sole input (DATA_MODEL.md's "Owner provisioning" note —
	// the owner row is seeded once from a known subject, never
	// JIT-created). Deliberately not required here at config-parse time,
	// the same way SSOClientID/SSOClientSecret above are optional at this
	// layer — cmd/seed is the only consumer, and it is the one that
	// enforces this is set, with a clear error, rather than LoadConfig
	// failing every other command (cmd/server, cmd/issue-key, cmd/smoke)
	// over a var they never read.
	SeedOwnerSSOSubject string `env:"SEED_OWNER_SSO_SUBJECT"`
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
