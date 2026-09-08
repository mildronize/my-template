package publicapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gin-gonic/gin"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver for these tests

	"github.com/mildronize/my-template/internal/domain/todo"
	"github.com/mildronize/my-template/internal/identity"
	"github.com/mildronize/my-template/internal/transport/humamw"
)

func init() {
	gin.SetMode(gin.TestMode)
	humamw.InstallErrorEnvelope()
}

// repoRootForTests resolves the module root from this test file's own
// location (mirrors internal/domain/todo's todo_testutil_test.go and
// internal/identity's identity_testutil_test.go), so tests work
// regardless of the directory `go test` is invoked from.
func repoRootForTests(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "could not determine this test file's own location")
	// this file lives at <root>/internal/transport/publicapi/<this file>.go
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))
}

// newTestDB opens a fresh temp-file SQLite database and applies every
// migration under db/migrations against it via goose — the same
// migrations `goose up` applies in production — so these tests exercise
// the real schema rather than a hand-maintained copy of it. This
// package's handler tests are integration tests against a real database,
// not unit tests against fakes: the fakes internal/identity's
// service_test.go defines for its own unit tests are unexported and
// package-private, so they cannot cross the package boundary this
// package's own move out of internal/identity created (ARCHITECTURE.md —
// only internal/identity keeps service.go/repo.go, this package keeps the
// handlers/middleware that used to sit alongside them).
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	conn, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	require.NoError(t, goose.SetDialect("sqlite3"))
	migrationsDir := filepath.Join(repoRootForTests(t), "db", "migrations")
	require.NoError(t, goose.Up(conn, migrationsDir))

	return conn
}

// newHumaV1API wraps a fresh gin.Engine in a huma.API instance, exactly
// the shape cmd/server/main.go's newHumaAPIs/apiV1H builds for the real
// service (ticket-6): humagin.New over the whole engine (no route group —
// contract.md's "Framework & wiring"), RejectUnknownQueryParameters set,
// humamw.RejectActorFields then humamw.RequireActor(svc) attached in that
// order (I1's X-Actor check before any DB-backed credential resolution).
// Every integration router this file's test helpers build below is this
// plus whichever RegisterXRoutes calls that test needs — so these tests
// exercise the real /api/v1 wiring shape, not a hand-rolled subset of it.
func newHumaV1API(router *gin.Engine, svc *identity.Service) huma.API {
	config := huma.DefaultConfig("my-template API", "1.0.0")
	config.OpenAPIPath = ""
	config.DocsPath = ""
	config.RejectUnknownQueryParameters = true

	api := humagin.New(router, config)
	api.UseMiddleware(humamw.RejectActorFields())
	api.UseMiddleware(humamw.RequireActor(svc))
	return api
}

// newIntegrationRouter builds a full /api/v1 stack — humamw.RejectActorFields,
// humamw.RequireActor, then every RegisterXRoutes call — against a real
// temp-file SQLite database (not a mock), for todo CRUD + ownership-scoping
// integration tests.
func newIntegrationRouter(t *testing.T) (*gin.Engine, *sql.DB) {
	t.Helper()
	conn := newTestDB(t)

	identityRepo := identity.NewRepo(conn)
	identitySvc := identity.NewService(identityRepo, identityRepo, nil, nil)

	todoSvc := todo.NewService(todo.NewRepo(conn))

	router := gin.New()
	api := newHumaV1API(router, identitySvc)
	RegisterMeRoutes(api)
	RegisterKeysRoutes(api, identitySvc)
	RegisterTodoRoutes(api, todoSvc)

	return router, conn
}

// createAgentWithKey seeds a users row (role=agent) and a live api_keys
// row for it, returning the user's id and the raw key a test can present
// as `Authorization: Bearer <rawKey>`. Shared by every handler test file
// in this package (todo, keys, middleware) — one definition, not one per
// file, now that they all live in the same package.
func createAgentWithKey(t *testing.T, conn *sql.DB, handle string) (userID, rawKey string) {
	t.Helper()
	ctx := context.Background()
	repo := identity.NewRepo(conn)

	user, err := repo.CreateUser(ctx, handle, "agent", nil)
	require.NoError(t, err)

	rawKey = "tpl_" + handle + "0123456789abcdef0123456789abcdef"
	hash := identity.HashAPIKey(rawKey)
	_, err = repo.CreateAPIKey(ctx, user.ID, hash, rawKey[:12], time.Now().Add(time.Hour))
	require.NoError(t, err)

	return user.ID, rawKey
}

// doJSONRequest is the shared HTTP-call helper every handler test file in
// this package uses — generic across every domain, not todo-specific, so
// it lives here rather than in a per-domain handler test file.
func doJSONRequest(t *testing.T, router *gin.Engine, method, path, rawKey string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if rawKey != "" {
		req.Header.Set("Authorization", "Bearer "+rawKey)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// apiErrorBody decodes this service's {error:{code,message,hint}} envelope
// shape — replacing internal/api.Error (deleted this ticket, contract.md's
// "File/package layout": internal/api's framework-agnostic response types
// move into this package). Every real error response on this surface
// (huma's own request-validation failures and huma.Error404NotFound/
// Error403Forbidden, via humamw.InstallErrorEnvelope's override, and this
// package's own apiError for its 400 validation_error responses,
// middleware.go) already writes this exact JSON shape — this type only
// exists so tests can decode it back, mirroring the old api.Error's own
// Hint *string (omitempty) shape so existing assertions
// (require.NotNil(...Hint), *errBody.Error.Hint) keep compiling unchanged.
type apiErrorBody struct {
	Error struct {
		Code    string  `json:"code"`
		Message string  `json:"message"`
		Hint    *string `json:"hint,omitempty"`
	} `json:"error"`
}

// decodeError decodes an apiErrorBody-shaped response body — generic
// across every domain (the error envelope shape is package-wide, not
// todo-specific), so it lives here rather than in a per-domain handler
// test file.
func decodeError(t *testing.T, rec *httptest.ResponseRecorder) apiErrorBody {
	t.Helper()
	var got apiErrorBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got
}
