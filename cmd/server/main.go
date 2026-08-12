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

	"github.com/mildronize/my-template/internal/api"
	"github.com/mildronize/my-template/internal/identity"
	"github.com/mildronize/my-template/internal/platform"
	"github.com/mildronize/my-template/internal/todo"
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

	apiV1, err := wireIdentity(ctx, router, db, cfg, logger)
	if err != nil {
		return fmt.Errorf("wiring identity module: %w", err)
	}

	if err := wireAPI(apiV1, db); err != nil {
		return fmt.Errorf("wiring api module: %w", err)
	}

	addr := fmt.Sprintf(":%d", cfg.Port)
	if err := platform.RunServer(ctx, logger, addr, router); err != nil {
		return fmt.Errorf("running server: %w", err)
	}

	return nil
}

// wireIdentity builds the identity module (repo, service, JWT verifier)
// and mounts the /api/v1 group with its two request-gating middlewares —
// I1's RejectActorFields before I2/I5's RequireActor. It does not
// register any routes itself: route registration happens once, in
// wireAPI, after every domain module's ServerInterface piece exists to
// compose (identity's GetMe alongside todo's CRUD), so GET /api/v1/me
// runs through the same generated/validated path as everything else
// (task-3) instead of a bespoke route added here. Composing routes across
// domain modules is exactly what this file's own doc comment says it does
// ("Domain routes ... are registered here").
func wireIdentity(ctx context.Context, router *gin.Engine, conn *sql.DB, cfg *platform.Config, logger *slog.Logger) (*gin.RouterGroup, error) {
	repo := identity.NewRepo(conn)

	// The JWT branch is a wired-but-dormant seam (GOAL.md) — a
	// deployment without both SSO_ISSUER and AUTH_AUDIENCE configured
	// simply never builds a verifier, and Service treats a nil
	// JWTVerifier as "this branch never matches", not an error.
	var jwtVerifier identity.JWTVerifier
	if cfg.SSOIssuer != "" && cfg.AuthAudience != "" {
		v, err := identity.NewJWTVerifier(ctx, cfg.SSOIssuer, cfg.AuthAudience)
		if err != nil {
			return nil, fmt.Errorf("building JWT verifier: %w", err)
		}
		jwtVerifier = v
	} else {
		logger.Warn("SSO_ISSUER/AUTH_AUDIENCE not set — JWT bearer path disabled, API-key auth only")
	}

	svc := identity.NewService(repo, repo, jwtVerifier, logger)

	apiV1 := router.Group("/api/v1")
	apiV1.Use(identity.RejectActorFields(), identity.RequireActor(svc))

	return apiV1, nil
}

// apiServer composes every domain module's ServerInterface piece into the
// one type internal/api.RegisterHandlers needs: identity.MeServer
// contributes GetMe, *todo.Server contributes the todo CRUD methods. No
// method names collide, so plain embedding is sufficient — no hand-written
// delegation methods to keep in sync as endpoints are added.
type apiServer struct {
	identity.MeServer
	*todo.Server
}

var _ api.ServerInterface = apiServer{}

// wireAPI builds the openapi.yaml request validator and every domain
// module's route-level API surface (currently just internal/todo — the
// last new table this milestone adds, task-3), then registers all of it,
// identity's GetMe included, on apiV1 in one call. The validator is
// mounted after RejectActorFields/RequireActor (see wireIdentity) so a
// request is authenticated before its payload shape is validated, not the
// other way around.
func wireAPI(apiV1 *gin.RouterGroup, conn *sql.DB) error {
	validator, err := api.RequestValidator()
	if err != nil {
		return fmt.Errorf("building openapi request validator: %w", err)
	}
	apiV1.Use(validator)

	todoSvc := todo.NewService(todo.NewRepo(conn))

	api.RegisterHandlers(apiV1, apiServer{
		MeServer: identity.MeServer{},
		Server:   todo.NewServer(todoSvc),
	})

	return nil
}
