package todo

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
// location (mirrors internal/identity's identity_testutil_test.go and
// internal/architecture_test.go's repoRoot), so tests work regardless of
// the directory `go test` is invoked from.
func repoRootForTests(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "could not determine this test file's own location")
	// this file lives at <root>/internal/todo/<this file>.go
	return filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
}

// newTestDB opens a fresh temp-file SQLite database and applies every
// migration under db/migrations against it via goose — the full
// three-migration set (users, api_keys, todos), the same set `goose up`
// applies in production — so these tests exercise the real schema, and
// incidentally re-verify on every run that the full migration set still
// applies cleanly to a fresh, empty file (GOAL.md Done-when 2; see also
// internal/platform/migrate_test.go's TestGooseUp_FullMigrationSetAppliesCleanly
// for a dedicated, explicit, fork-safe check of that same fact — moved
// there from this package in task-6.md since it doesn't need any
// domain-module import).
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

// createTestUser is a small helper repo_test.go/handler_test.go both use
// to get a users row to own todos against, without going through
// internal/identity (todo's tests shouldn't need to import identity's
// repo just to seed a fixture — a plain INSERT is simpler and doesn't
// blur the I4 table-ownership boundary these tests are partly here to
// demonstrate).
func createTestUser(t *testing.T, conn *sql.DB, id, handle string) {
	t.Helper()
	_, err := conn.Exec(
		`INSERT INTO users (id, handle, role, active, created_at, updated_at) VALUES (?, ?, 'agent', TRUE, datetime('now'), datetime('now'))`,
		id, handle,
	)
	require.NoError(t, err)
}
