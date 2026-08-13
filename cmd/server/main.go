// Command server is the template's HTTP service entrypoint. It wires
// config, logging, the SQLite connection, and the gin router together and
// starts listening. internal/transport/publicapi's routes (todo CRUD,
// GET /me, /keys) are composed here — this file stays thin on purpose.
// internal/transport/bff's own routes are wired the same way once task-4
// adds that engine.
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
	"github.com/mildronize/my-template/internal/domain/todo"
	"github.com/mildronize/my-template/internal/identity"
	"github.com/mildronize/my-template/internal/platform"
	"github.com/mildronize/my-template/internal/transport/publicapi"
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

	if err := platform.Migrate(db); err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}

	// platform.NewRouter already wires the cross-cutting middleware every
	// engine needs (RequestID, RequestLogging, Recovery — platform/
	// middleware.go) — this router is internal/transport/publicapi's
	// engine; internal/transport/bff gets its own from the same
	// constructor once task-4 adds it.
	router := platform.NewRouter(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	apiV1, identitySvc, err := wireIdentity(ctx, router, db, cfg, logger)
	if err != nil {
		return fmt.Errorf("wiring identity module: %w", err)
	}

	if err := wirePublicAPI(apiV1, db, identitySvc); err != nil {
		return fmt.Errorf("wiring public API: %w", err)
	}

	addr := fmt.Sprintf(":%d", cfg.Port)
	if err := platform.RunServer(ctx, logger, addr, router); err != nil {
		return fmt.Errorf("running server: %w", err)
	}

	return nil
}

// wireIdentity builds the identity module (repo, service, JWT verifier)
// and mounts the /api/v1 group with its two request-gating middlewares —
// I1's RejectActorFields before I2/I5's RequireActor, both now living in
// internal/transport/publicapi (ARCHITECTURE.md — internal/identity holds
// no transport code of its own). It does not register any routes itself:
// route registration happens once, in wirePublicAPI, after every piece of
// publicapi's ServerInterface exists to compose (identity's GetMe/keys
// alongside todo's CRUD), so GET /api/v1/me runs through the same
// generated/validated path as everything else (task-3) instead of a
// bespoke route added here.
//
// It also returns the built *identity.Service (not just the
// gin.RouterGroup), so wirePublicAPI can build publicapi.KeysServer on top
// of the exact same Service instance RequireActor authenticates requests
// with — one Service per process, not a second one constructed
// independently in wirePublicAPI that would happen to work today but
// drift the moment identity.NewService gains a dependency wirePublicAPI
// doesn't have.
func wireIdentity(ctx context.Context, router *gin.Engine, conn *sql.DB, cfg *platform.Config, logger *slog.Logger) (*gin.RouterGroup, *identity.Service, error) {
	repo := identity.NewRepo(conn)

	// The JWT branch is a wired-but-dormant seam (GOAL.md) — a
	// deployment without both SSO_ISSUER and AUTH_AUDIENCE configured
	// simply never builds a verifier, and Service treats a nil
	// JWTVerifier as "this branch never matches", not an error.
	var jwtVerifier identity.JWTVerifier
	if cfg.SSOIssuer != "" && cfg.AuthAudience != "" {
		v, err := identity.NewJWTVerifier(ctx, cfg.SSOIssuer, cfg.AuthAudience)
		if err != nil {
			return nil, nil, fmt.Errorf("building JWT verifier: %w", err)
		}
		jwtVerifier = v
	} else {
		logger.Warn("SSO_ISSUER/AUTH_AUDIENCE not both set — JWT bearer path disabled, API-key auth only")
	}

	svc := identity.NewService(repo, repo, jwtVerifier, logger)

	apiV1 := router.Group("/api/v1")
	apiV1.Use(publicapi.RejectActorFields(), publicapi.RequireActor(svc))

	return apiV1, svc, nil
}

// apiServer composes every publicapi ServerInterface piece into the one
// type internal/api.RegisterHandlers needs: publicapi.MeServer
// contributes GetMe, *publicapi.KeysServer contributes
// ListKeys/RevokeKey, *publicapi.TodoServer contributes the todo CRUD
// methods. No method names collide, so plain embedding is sufficient — no
// hand-written delegation methods to keep in sync as endpoints are added.
type apiServer struct {
	publicapi.MeServer
	*publicapi.KeysServer
	*publicapi.TodoServer
}

var _ api.ServerInterface = apiServer{}

// wirePublicAPI builds the openapi.yaml request validator and every piece
// of internal/transport/publicapi's route-level API surface (identity's
// keys endpoints, internal/domain/todo's CRUD), then registers all of it,
// identity's GetMe included, on apiV1 in one call. The validator is
// mounted after RejectActorFields/RequireActor (see wireIdentity) so a
// request is authenticated before its payload shape is validated, not the
// other way around.
func wirePublicAPI(apiV1 *gin.RouterGroup, conn *sql.DB, identitySvc *identity.Service) error {
	validator, err := api.RequestValidator()
	if err != nil {
		return fmt.Errorf("building openapi request validator: %w", err)
	}
	apiV1.Use(validator)

	todoSvc := todo.NewService(todo.NewRepo(conn))

	api.RegisterHandlers(apiV1, apiServer{
		MeServer:   publicapi.MeServer{},
		KeysServer: publicapi.NewKeysServer(identitySvc),
		TodoServer: publicapi.NewTodoServer(todoSvc),
	})

	return nil
}
