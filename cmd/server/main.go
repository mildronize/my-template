// Command server is the template's HTTP service entrypoint. It wires
// config, logging, the SQLite connection, and the gin router together and
// starts listening. Domain routes (todo, identity) are registered here too
// once those modules exist — this file stays thin on purpose.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"

	"github.com/mildronize/my-template/internal/identity"
	"github.com/mildronize/my-template/internal/platform"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := platform.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	logger := platform.NewLogger()
	slog.SetDefault(logger)

	db, err := platform.OpenDB(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	router := platform.NewRouter(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := wireIdentity(ctx, router, db, cfg, logger); err != nil {
		return fmt.Errorf("wiring identity module: %w", err)
	}

	addr := fmt.Sprintf(":%d", cfg.Port)
	if err := platform.RunServer(ctx, logger, addr, router); err != nil {
		return fmt.Errorf("running server: %w", err)
	}

	return nil
}

// wireIdentity builds the identity module (repo, service, JWT verifier)
// and mounts its routes on a new /api/v1 group. The group — and its
// middleware order, I1's RejectActorFields before I2/I5's RequireActor —
// is built here rather than inside internal/identity, since composing
// routes across domain modules is exactly what this file's own doc
// comment says it does ("Domain routes ... are registered here"); a
// later task layering openapi.yaml/gin-middleware validation onto this
// same group does so here too, not by internal/identity reaching back
// out to gin plumbing it shouldn't own.
func wireIdentity(ctx context.Context, router *gin.Engine, conn *sql.DB, cfg *platform.Config, logger *slog.Logger) error {
	repo := identity.NewRepo(conn)

	// The JWT branch is a wired-but-dormant seam (GOAL.md) — a
	// deployment without both SSO_ISSUER and AUTH_AUDIENCE configured
	// simply never builds a verifier, and Service treats a nil
	// JWTVerifier as "this branch never matches", not an error.
	var jwtVerifier identity.JWTVerifier
	if cfg.SSOIssuer != "" && cfg.AuthAudience != "" {
		v, err := identity.NewJWTVerifier(ctx, cfg.SSOIssuer, cfg.AuthAudience)
		if err != nil {
			return fmt.Errorf("building JWT verifier: %w", err)
		}
		jwtVerifier = v
	} else {
		logger.Warn("SSO_ISSUER/AUTH_AUDIENCE not set — JWT bearer path disabled, API-key auth only")
	}

	svc := identity.NewService(repo, repo, jwtVerifier, logger)

	apiV1 := router.Group("/api/v1")
	apiV1.Use(identity.RejectActorFields(), identity.RequireActor(svc))
	identity.RegisterRoutes(apiV1, svc)

	return nil
}
