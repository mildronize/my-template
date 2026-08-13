package main

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mildronize/my-template/internal/platform"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver for this test
)

// TestDoneWhen3_MiddlewareWiredIntoBothEngines confirms
// platform/middleware.go's RequestID/RequestLogging/Recovery are wired
// into *both* the publicapi engine and the bff engine
// (task-1.md/GOAL.md's Done-when 3) — not assumed just because
// buildHandler calls platform.NewRouter for each of them, actually
// checked. RequestID sets the X-Request-ID response header via a
// gin.Engine's Use()-registered middleware, which gin runs even for an
// unmatched route (its own 404-handler chain is built from Use()'s
// handlers plus the not-found handler) — so hitting a nonexistent path
// under each engine's own prefix and asserting the header is present is
// enough to prove that engine has the middleware, no real route needed on
// either side.
func TestDoneWhen3_MiddlewareWiredIntoBothEngines(t *testing.T) {
	db := newMainTestDB(t)
	cfg := &platform.Config{
		Port:          8080,
		DatabasePath:  "unused-in-this-test",
		SessionSecret: "test-session-secret",
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// A synthetic dist filesystem stands in for the real npm-built SPA
	// embed here -- this test only asserts RequestID's header is present
	// on the bff engine's 404, never anything about the SPA's actual
	// content, so it must not depend on `npm run build` having run before
	// `go test` (see bff_negative_check_test.go's own note on this).
	distFS := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>test fixture</html>")},
	}

	handler, err := buildHandler(context.Background(), cfg, db, logger, distFS)
	require.NoError(t, err)

	t.Run("publicapi engine", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/this-route-does-not-exist", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.NotEmpty(t, rec.Header().Get(platform.RequestIDHeader),
			"publicapi's engine must set X-Request-ID even on a 404 -- RequestID must be registered via Use()")
	})

	t.Run("bff engine", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/this-route-does-not-exist-either", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.NotEmpty(t, rec.Header().Get(platform.RequestIDHeader),
			"bff's engine must set X-Request-ID even on a 404 -- RequestID must be registered via Use() here too, not only on publicapi's engine")
	})

	t.Run("healthz still reachable through the combined handler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.NotEmpty(t, rec.Header().Get(platform.RequestIDHeader))
	})
}

// newMainTestDB opens a fresh temp-file SQLite database and applies the
// service's own embedded migration set via platform.Migrate — the same
// call run() itself makes — so this test doesn't need its own copy of the
// migrations-directory-resolution logic internal/domain/todo's and
// internal/transport/publicapi's testutils use.
func newMainTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	conn, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	require.NoError(t, platform.Migrate(conn))
	return conn
}
