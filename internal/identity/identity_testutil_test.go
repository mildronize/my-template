package identity

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
// location (mirrors internal/architecture_test.go's repoRoot), so tests
// work regardless of the directory `go test` is invoked from.
func repoRootForTests(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "could not determine this test file's own location")
	// this file lives at <root>/internal/identity/<this file>.go
	return filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
}

// newTestDB opens a fresh temp-file SQLite database and applies every
// migration under db/migrations against it via goose — the same
// migrations `goose up` applies in production — so these tests exercise
// the real schema rather than a hand-maintained copy of it.
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
