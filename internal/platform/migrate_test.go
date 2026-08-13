package platform

import (
	"database/sql"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGooseUp_FullMigrationSetAppliesCleanly is GOAL.md Done-when 2's own
// check: the full migration set applies cleanly against a fresh, empty
// SQLite file. It calls Migrate directly — the same function cmd/server
// and cmd/issue-key call on every process start — rather than
// hand-rolling a second goose.Up call against migrations read off disk,
// so this test exercises exactly what production runs, embedded FS and
// all.
//
// Moved here from internal/todo/migration_test.go (task-6.md, P1(b)):
// that location meant this test — Done-when 2's *only* check — was
// deleted silently the moment a fork did GETTING-STARTED.md Step 5's
// `rm -rf internal/todo`, with no invariant name tying it to anything
// that would notice. It exercises Migrate and the embedded migration
// set, not anything domain-specific, so internal/platform (which
// GETTING-STARTED.md says is not part of Step 5's deletion) is where it
// belongs and survives a fork.
//
// Only users and api_keys — internal/identity's tables, which
// GETTING-STARTED.md says stay as-is on fork — are asserted by name.
// The domain-specific table this test used to also check ("todos")
// deliberately isn't: after a fork replaces internal/todo with its own
// domain, that table has a different name, and the new domain's own
// tests already exercise that its replacement migration applies (they
// can't run at all otherwise).
func TestGooseUp_FullMigrationSetAppliesCleanly(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "fresh-*.db")
	require.NoError(t, err)
	dbPath := f.Name()
	require.NoError(t, f.Close())

	conn, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	require.NoError(t, Migrate(conn),
		"Migrate must apply the full migration set cleanly to a fresh, empty SQLite file")

	for _, table := range []string{"users", "api_keys"} {
		var name string
		err := conn.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		assert.NoErrorf(t, err, "table %q must exist after the full migration set applies", table)
		assert.Equal(t, table, name)
	}
}
