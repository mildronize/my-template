package todo

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGooseUp_FullMigrationSetAppliesCleanly is task-3's explicit check
// for GOAL.md Done-when 2: `todos` is the last new table this milestone
// adds, so this is the point to verify `goose up` applies cleanly against
// a fresh, empty SQLite file with the *full* migration set (users,
// api_keys, todos together) — not just this task's own migration in
// isolation. newTestDB (todo_testutil_test.go) already runs the full set
// for every other test in this file; this test additionally asserts,
// explicitly, that all three tables exist afterward and that the file
// really was empty beforehand (a brand-new os.CreateTemp file, not
// t.TempDir()'s directory reused across tests).
func TestGooseUp_FullMigrationSetAppliesCleanly(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "fresh-*.db")
	require.NoError(t, err)
	dbPath := f.Name()
	require.NoError(t, f.Close())

	conn, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	require.NoError(t, goose.SetDialect("sqlite3"))
	migrationsDir := filepath.Join(repoRootForTests(t), "db", "migrations")

	require.NoError(t, goose.Up(conn, migrationsDir),
		"goose up must apply the full migration set cleanly to a fresh, empty SQLite file")

	for _, table := range []string{"users", "api_keys", "todos"} {
		var name string
		err := conn.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		assert.NoErrorf(t, err, "table %q must exist after the full migration set applies", table)
		assert.Equal(t, table, name)
	}
}
