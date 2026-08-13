// Command server is the template's HTTP service entrypoint. It wires
// config, logging, the SQLite connection, and two gin engines together and
// starts listening on one port: internal/transport/publicapi's routes
// (todo CRUD, GET /me, /keys, mounted under /api/v1 plus /healthz) and
// internal/transport/bff's routes (GET /login, GET /callback, plus the
// embedded Vite SPA — web/embed.go, cmd/server/spa.go — serving "/" and
// everything else bff doesn't explicitly claim, milestone-3/task-1) share
// one *sql.DB and one *identity.Repo/*todo.Service, but each gets its own
// *gin.Engine (and its own copy of platform's cross-cutting middleware —
// see buildHandler) since they're different transport surfaces per
// ARCHITECTURE.md. A stdlib http.ServeMux in front dispatches by path
// prefix to whichever engine owns it — this keeps the "one port"
// deployment model docker-compose.yml and DEPLOY-REQUIREMENTS.md already
// document, rather than needing a second listener for bff.
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gin-gonic/gin"

	"github.com/mildronize/my-template/internal/api"
	"github.com/mildronize/my-template/internal/bffapi"
	"github.com/mildronize/my-template/internal/domain/todo"
	"github.com/mildronize/my-template/internal/identity"
	"github.com/mildronize/my-template/internal/platform"
	"github.com/mildronize/my-template/internal/transport/bff"
	"github.com/mildronize/my-template/internal/transport/publicapi"
	"github.com/mildronize/my-template/web"
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	distFS, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		return fmt.Errorf("opening embedded web/dist: %w", err)
	}

	handler, err := buildHandler(ctx, cfg, db, logger, distFS)
	if err != nil {
		return fmt.Errorf("building HTTP handler: %w", err)
	}

	addr := fmt.Sprintf(":%d", cfg.Port)
	if err := platform.RunServer(ctx, logger, addr, handler); err != nil {
		return fmt.Errorf("running server: %w", err)
	}

	return nil
}

// buildHandler composes both this service's transport surfaces into one
// http.Handler — split out of run() so a test can build (and inspect) the
// exact same handler without actually binding a port. This is what
// task-1's Done-when 3 ("platform/middleware.go's recovery/logging/
// request-ID wired into both publicapi and bff engines — a test confirms
// both, not just one") is checked against: main_test.go builds this
// handler against a temp test database and asserts both the /api/v1
// surface and the bff surface set RequestID's response header, which only
// happens if each engine actually has platform.NewRouter's middleware
// registered on it (gin's Use()-registered middleware runs even on an
// unmatched route, per gin's own 404-handler-chain behavior, so this is
// checkable without a single real route on either engine needing to
// match).
// distFS is the SPA's dist filesystem (already fs.Sub'd to "dist") — run()
// passes fs.Sub(web.DistFS, "dist"), the real Vite-built embed, for
// production; tests pass a synthetic fs.FS (e.g. fstest.MapFS) so the
// SPA-fallback assertions don't depend on `npm run build` having actually
// run before `go test` (cmd/server/bff_negative_check_test.go).
func buildHandler(ctx context.Context, cfg *platform.Config, db *sql.DB, logger *slog.Logger, distFS fs.FS) (http.Handler, error) {
	repo := identity.NewRepo(db)
	todoSvc := todo.NewService(todo.NewRepo(db))

	// --- publicapi ----------------------------------------------------
	apiRouter := platform.NewRouter(logger)

	apiV1, identitySvc, err := wireIdentity(ctx, apiRouter, repo, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("wiring identity module: %w", err)
	}
	if err := wirePublicAPI(apiV1, todoSvc, identitySvc); err != nil {
		return nil, fmt.Errorf("wiring public API: %w", err)
	}

	// --- bff ------------------------------------------------------------
	bffRouter := platform.NewRouter(logger)
	// identitySvc (built once by wireIdentity, above) is shared with bff's
	// own new /api/bff/keys endpoints (milestone-3/task-2) — one
	// *identity.Service instance, not two independently-constructed ones,
	// the same shared-service-layer reasoning todoSvc's own single
	// instance already follows across both engines.
	if err := wireBFF(ctx, bffRouter, cfg, repo, todoSvc, identitySvc, logger, distFS); err != nil {
		return nil, fmt.Errorf("wiring bff: %w", err)
	}

	// Both engines share one port — a stdlib mux dispatches by path prefix.
	// "/api/v1/" and "/healthz" are publicapi's; everything else (bff's
	// "/login", "/callback", and — via NoRoute, wireBFF — the embedded SPA
	// at "/" and every other path) falls through to the "/" pattern, which
	// net/http.ServeMux treats as the catch-all for anything not matched
	// by a more specific registered pattern. Neither engine strips a
	// prefix — each still sees the request's full original path, which is
	// exactly what each engine's own route registrations (below) expect.
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", apiRouter)
	mux.Handle("/healthz", apiRouter)
	mux.Handle("/", bffRouter)

	return mux, nil
}

// wireIdentity builds the identity module's actor-resolution
// middlewares onto router's "/api/v1" group — I1's RejectActorFields
// before I2/I5's RequireActor, both living in internal/transport/publicapi
// (ARCHITECTURE.md — internal/identity holds no transport code of its
// own). It does not register any routes itself: route registration
// happens once, in wirePublicAPI, after every piece of publicapi's
// ServerInterface exists to compose (identity's GetMe/keys alongside
// todo's CRUD), so GET /api/v1/me runs through the same
// generated/validated path as everything else (task-3) instead of a
// bespoke route added here.
//
// repo is built once by buildHandler and passed in (rather than
// constructed here) so internal/transport/bff's own callback/view
// handlers resolve users through the exact same *identity.Repo instance —
// one repo per process, not two independently-constructed ones that
// happen to wrap the same *sql.DB today but could drift.
//
// It also returns the built *identity.Service (not just the
// gin.RouterGroup), so wirePublicAPI can build publicapi.KeysServer on top
// of the exact same Service instance RequireActor authenticates requests
// with.
func wireIdentity(ctx context.Context, router *gin.Engine, repo *identity.Repo, cfg *platform.Config, logger *slog.Logger) (*gin.RouterGroup, *identity.Service, error) {
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
// other way around. todoSvc is built once by buildHandler and shared with
// wireBFF's own GET / handler (ARCHITECTURE.md's shared-service-layer
// rule — one todo.Service instance, not two).
func wirePublicAPI(apiV1 *gin.RouterGroup, todoSvc *todo.Service, identitySvc *identity.Service) error {
	validator, err := api.RequestValidator()
	if err != nil {
		return fmt.Errorf("building openapi request validator: %w", err)
	}
	apiV1.Use(validator)

	api.RegisterHandlers(apiV1, apiServer{
		MeServer:   publicapi.MeServer{},
		KeysServer: publicapi.NewKeysServer(identitySvc),
		TodoServer: publicapi.NewTodoServer(todoSvc),
	})

	return nil
}

// bffServer composes every internal/bffapi ServerInterface piece into the
// one type internal/bffapi.RegisterHandlers needs — the bff-surface mirror
// of apiServer, above. bff.MeServer contributes GetMe, *bff.KeysServer
// contributes ListKeys/RevokeKey, *bff.TodoServer contributes the todo
// CRUD methods. No method names collide, so plain embedding is
// sufficient, same reasoning as apiServer.
type bffServer struct {
	bff.MeServer
	*bff.KeysServer
	*bff.TodoServer
}

var _ bffapi.ServerInterface = bffServer{}

// wireBFF builds internal/transport/bff's session signer and its
// owner-login flow's id-token verifier, then registers GET /login and
// GET /callback on router (task-4.md, _contract/API.md's BFF section),
// the new /api/bff JSON surface (milestone-3/task-2, below), plus the
// embedded SPA (cmd/server/spa.go) as router's NoRoute handler
// (milestone-3/task-1) — completing task-1's Done-when 3 (platform's
// middleware wired into both engines): router here already carries
// platform.NewRouter's RequestID/RequestLogging/Recovery, applied by
// buildHandler exactly the way it's applied to publicapi's own router.
//
// Neither cfg.SessionSecret being auto-generated nor
// SSOClientID/SSOClientSecret being unset stop this from wiring routes —
// GETTING-STARTED.md's own walkthrough promises the server (and the
// public API) keep working with no Hydra client registered yet, so
// GET /login and GET /callback mount unconditionally and check
// oauth.go's configured() themselves at request time, returning a clear
// error page instead of a working flow when the config is incomplete.
//
// milestone-3/task-1/task-3 note on GET /: internal/transport/bff/
// view_handler.go (the old Go-html/template owner view) claimed the exact
// path "/" until task-1, which stopped registering it here so the embedded
// SPA (below, via NoRoute) could actually serve "/" instead — gin always
// prefers an explicit route over NoRoute, so leaving that registration in
// place would have made "/" permanently unreachable for the SPA.
// view_handler.go and its test were deliberately left in place through
// task-1/task-2 (each task's own "what NOT to do" list) and removed by
// task-3, once the SPA's own todos screen replaced what it rendered —
// there is no view_handler.go left in this package at all now. todoSvc is
// still accepted here for the /api/bff JSON surface below
// (milestone-3/task-2 — the same instance wirePublicAPI's own TodoServer
// uses, per ARCHITECTURE.md's shared-service-layer rule).
// distFS is the SPA's already-fs.Sub'd dist filesystem — see buildHandler's
// own doc comment on why this is a parameter rather than wireBFF reaching
// into web.DistFS itself.
func wireBFF(ctx context.Context, router *gin.Engine, cfg *platform.Config, repo *identity.Repo, todoSvc *todo.Service, identitySvc *identity.Service, logger *slog.Logger, distFS fs.FS) error {
	secret := []byte(cfg.SessionSecret)
	if len(secret) == 0 {
		secret = make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return fmt.Errorf("generating ephemeral session secret: %w", err)
		}
		logger.Warn("SESSION_SECRET not set — generated an ephemeral one for this process; " +
			"existing BFF sessions will not survive a restart. Set SESSION_SECRET for any real " +
			"deployment (docs/DEPLOY-REQUIREMENTS.md).")
	}
	signer := bff.NewSigner(secret)

	// The id-token verifier is built the same way internal/identity's own
	// Bearer-JWT verifier is (identity.NewJWTVerifier — RS256-pinned,
	// iss/aud/exp checked, I6/I7), but audienced to this OAuth client's own
	// id rather than AUTH_AUDIENCE: an id_token's `aud` claim is the
	// client_id per OIDC, not the API audience publicapi's Bearer path
	// checks. Left nil (the same "dormant seam" shape as wireIdentity's own
	// jwtVerifier) when SSO_ISSUER/SSO_CLIENT_ID aren't both set —
	// callback_handler.go's configured() check already covers the same
	// condition, so requests fail with a clear message rather than this
	// verifier being nil unexpectedly.
	var idVerifier identity.JWTVerifier
	if cfg.SSOIssuer != "" && cfg.SSOClientID != "" {
		v, err := identity.NewJWTVerifier(ctx, cfg.SSOIssuer, cfg.SSOClientID)
		if err != nil {
			return fmt.Errorf("building bff id-token verifier: %w", err)
		}
		idVerifier = v
	} else {
		logger.Warn("SSO_ISSUER/SSO_CLIENT_ID not both set — owner login (bff) will show a " +
			"configuration error until scripts/register.sh's Step 1 is run (see docs/GETTING-STARTED.md)")
	}

	router.GET("/login", bff.NewLoginHandler(cfg, signer, logger))
	router.GET("/callback", bff.NewCallbackHandler(cfg, signer, idVerifier, repo, logger))

	// --- /api/bff: milestone-3/task-2's new session-authenticated JSON
	// surface (bff-openapi.yaml, internal/bffapi) -----------------------
	//
	// This is the point BFF writes are enabled — the I2/I12 boundary,
	// condensed from _contract/API.md's "The I2/I12 boundary" section
	// (see also internal/transport/bff/middleware.go's RequireSession doc
	// comment, the other point this same reasoning is pinned to): I2 (a
	// Bearer credential never resolves to role='owner') and I12 (a BFF
	// session never resolves to role='agent') are two halves of one
	// design. An owner has no Bearer credential to present at all, so a
	// BFF session — gated here by bff.RequireJSONSession — is the *only*
	// path by which POST/PATCH/DELETE /api/bff/todos and DELETE
	// /api/bff/keys/{id} ever run; an agent has no session to present, so
	// it can never reach these routes regardless. This group is
	// deliberately the only place in this service that registers an
	// owner-authenticated write route — wirePublicAPI (above) registers
	// none, and per the boundary reasoning above, never should.
	//
	// Middleware order mirrors wireIdentity/wirePublicAPI's own: I1's
	// RejectActorFields (reused directly from internal/transport/
	// publicapi — a pure request-shape guard with no publicapi-specific
	// state) runs before RequireJSONSession's own DB lookup, so a shape
	// violation never spends a session-resolution query; the bff-openapi.yaml
	// request validator runs last, after authentication, same as
	// wirePublicAPI's own validator placement.
	bffValidator, err := bffapi.RequestValidator()
	if err != nil {
		return fmt.Errorf("building bff-openapi request validator: %w", err)
	}
	apiBFF := router.Group("/api/bff")
	apiBFF.Use(publicapi.RejectActorFields(), bff.RequireJSONSession(signer, repo, logger), bffValidator)
	bffapi.RegisterHandlers(apiBFF, bffServer{
		MeServer:   bff.MeServer{},
		KeysServer: bff.NewKeysServer(identitySvc),
		TodoServer: bff.NewTodoServer(todoSvc),
	})

	spaHandler, err := newSPAHandler(distFS)
	if err != nil {
		return fmt.Errorf("building embedded SPA handler: %w", err)
	}
	// NoRoute is router-wide (any path this gin.Engine has no explicit
	// route for), not scoped to "/" — without the /api/bff/ prefix check
	// below, an unmapped path under /api/bff/ (e.g. a typo'd endpoint, or
	// milestone-3/task-2's own negative check target,
	// POST /api/bff/keys) would fall through to the SPA fallback and
	// answer 200 text/html instead of a real 404, undermining Done-when
	// 5's negative check the moment it's observed against this live
	// router rather than internal/transport/bff's own isolated test
	// router (which never registers a NoRoute handler at all, so it 404s
	// correctly without needing this carve-out). Every other unmatched
	// path (e.g. "/settings", react-router's own client-side routes)
	// still falls through to the SPA exactly as milestone-3/task-1 built
	// it.
	router.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/bff/") {
			c.AbortWithStatusJSON(http.StatusNotFound, publicapi.NewErrorEnvelope(
				"not_found", "no such route", ""))
			return
		}
		gin.WrapH(spaHandler)(c)
	})

	return nil
}
