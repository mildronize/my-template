package publicapi

import (
	"database/sql"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver for these tests
)

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
