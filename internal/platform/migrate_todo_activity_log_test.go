package platform

import (
	"database/sql"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver for this test
)

// preActivityLogMigrationVersion is 20260812190000_create_todos.sql's own
// goose version — the last migration before this milestone's schema
// change. postActivityLogMigrationVersion is
// 20260813100000_todo_activity_log.sql's version, the migration under
// test. Named as constants (not inlined at each call site) so the intent
// ("everything up to, but not including, the migration under test" /
// "now apply exactly that migration") reads at the call site instead of
// two bare integers that only mean something next to db/migrations'
// actual filenames.
const (
	preActivityLogMigrationVersion  int64 = 20260812190000
	postActivityLogMigrationVersion int64 = 20260813100000
)

// repoRootForMigrationTest resolves the module root from this file's own
// location, mirroring every other package's repoRootForTests helper
// (internal/domain/todo/todo_testutil_test.go,
// internal/transport/publicapi/publicapi_testutil_test.go, etc.) — kept
// as its own unexported function here (not shared) since
// migrate_test.go's sibling test in this same package already has no
// need for it (it drives platform.Migrate's embedded FS, not a
// disk-relative migrations directory).
func repoRootForMigrationTest(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "could not determine this test file's own location")
	// this file lives at <root>/internal/platform/<this file>.go
	return filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
}

// TestTodoActivityLogMigration_PreservesExistingRows is GOAL.md Done-when
// 1's own check, and the actual deliverable of task-1 (task-1.md, `_plan/
// _todo.md`): the migration mapping is verified against a database
// seeded with pre-existing rows in both `done` states before the
// migration runs, not an empty one. A migration tested only against an
// empty database has proven it parses, nothing about what it does to a
// fork's actual data (Clara's finding, GOAL.md's Decisions table).
//
// This test is deliberately narrower than
// TestGooseUp_FullMigrationSetAppliesCleanly above: it doesn't re-prove
// the whole migration set applies cleanly (that test already does), it
// proves the *value transform* one specific migration performs on rows
// that existed before it ran — the exact mapping DATA_MODEL.md states in
// prose: `done = true` -> `status = 'done'`, `done = false` -> `status =
// 'open'`, `owner_id` -> `created_by` (rename, same value, no
// transform).
func TestTodoActivityLogMigration_PreservesExistingRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pre-migration.db")
	conn, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	require.NoError(t, goose.SetDialect("sqlite3"))
	migrationsDir := filepath.Join(repoRootForMigrationTest(t), "db", "migrations")

	// Step 1: apply every migration up to (but not including) this
	// milestone's schema change — the pre-migration shape: todos.owner_id,
	// todos.done, no todo_events table.
	require.NoError(t, goose.UpTo(conn, migrationsDir, preActivityLogMigrationVersion),
		"applying every migration up to (not including) the todo-activity-log migration")

	// Step 2: insert rows directly at that pre-migration schema shape —
	// one done=true row, one done=false row, each with a real owner_id.
	// A users row is required first: todos.owner_id is a real FK.
	_, err = conn.Exec(
		`INSERT INTO users (id, handle, role, active, created_at, updated_at)
		 VALUES ('user-1', 'alice', 'owner', TRUE, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
	)
	require.NoError(t, err, "seeding a pre-migration users row")

	_, err = conn.Exec(
		`INSERT INTO todos (id, owner_id, title, done, created_at, updated_at)
		 VALUES ('todo-done', 'user-1', 'already finished', TRUE, '2026-01-02T00:00:00Z', '2026-01-02T00:00:00Z')`,
	)
	require.NoError(t, err, "seeding a pre-migration done=true todos row")

	_, err = conn.Exec(
		`INSERT INTO todos (id, owner_id, title, done, created_at, updated_at)
		 VALUES ('todo-open', 'user-1', 'still open', FALSE, '2026-01-03T00:00:00Z', '2026-01-03T00:00:00Z')`,
	)
	require.NoError(t, err, "seeding a pre-migration done=false todos row")

	// Step 3: apply this migration.
	require.NoError(t, goose.UpTo(conn, migrationsDir, postActivityLogMigrationVersion),
		"applying the todo-activity-log migration itself")

	// Step 4: assert the exact mapping, per row, not just that the
	// migration executed without error.
	type gotRow struct {
		Status    string
		CreatedBy string
	}
	fetch := func(id string) gotRow {
		var got gotRow
		err := conn.QueryRow(`SELECT status, created_by FROM todos WHERE id = ?`, id).
			Scan(&got.Status, &got.CreatedBy)
		require.NoErrorf(t, err, "reading post-migration row %q", id)
		return got
	}

	doneRow := fetch("todo-done")
	require.Equal(t, "done", doneRow.Status,
		"a pre-migration done=true row must land on status='done', not 'closed' or anything else")
	require.Equal(t, "user-1", doneRow.CreatedBy,
		"created_by must carry the exact value owner_id held before the migration (a rename, not a transform)")

	openRow := fetch("todo-open")
	require.Equal(t, "open", openRow.Status,
		"a pre-migration done=false row must land on status='open'")
	require.Equal(t, "user-1", openRow.CreatedBy,
		"created_by must carry the exact value owner_id held before the migration (a rename, not a transform)")

	// The old columns are genuinely gone, not just unused — a rebuild
	// migration that forgot to drop them would still pass every assertion
	// above.
	for _, droppedColumn := range []string{"owner_id", "done"} {
		_, err := conn.Exec(`SELECT ` + droppedColumn + ` FROM todos LIMIT 1`)
		require.Errorf(t, err, "column %q must no longer exist on todos after this migration", droppedColumn)
	}

	// todo_events must exist per DATA_MODEL.md, with the right columns
	// reachable (a cheap smoke check — the schema shape's own detailed
	// coverage lives in this milestone's later tasks' tests, once
	// something writes/reads it).
	var name string
	require.NoError(t, conn.QueryRow(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'todo_events'`,
	).Scan(&name))
	require.Equal(t, "todo_events", name)
}
