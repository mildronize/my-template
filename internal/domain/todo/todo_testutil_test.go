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
	// this file lives at <root>/internal/domain/todo/<this file>.go.
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))
}

// newTestDB opens a fresh temp-file SQLite database and applies every
// migration under db/migrations against it via goose — the full
// migration set (users, api_keys, todos, the milestone-4 activity-log
// migration), the same set `goose up` applies in production — so these
// tests exercise the real schema.
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

// createTestUser is a small helper repo_test.go/service_test.go use to get
// a users row to attribute/act as, without going through
// internal/identity (todo's tests shouldn't need to import identity's
// repo just to seed a fixture — a plain INSERT is simpler and doesn't
// blur the I4 table-ownership boundary these tests are partly here to
// demonstrate). role defaults callers to "agent" via createTestUser;
// createTestOwner below seeds role='owner' for the paired
// permission-layer tests (I18) that need both roles against the same
// todo.
//
// This is a direct repo insert, not cmd/issue-key's real path — deliberate,
// and stated here plainly rather than left implicit: the permission layer
// under test (can(), permission.go) is role-based only, keyed off the
// users.role column value alone, not off how that role came to be set or
// whether a real API key was ever issued for the row. cmd/issue-key's own
// path is what task-2.md flags as worth using "if you need a real agent
// identity for any of this" — the identity/key-issuance mechanics
// themselves aren't what these tests exercise, so a direct insert is
// equivalent here in a way it would not be for a test of key issuance,
// resolution, or revocation (those live in internal/identity and
// internal/transport, not here).
func createTestUser(t *testing.T, conn *sql.DB, id, handle string) {
	t.Helper()
	createTestUserWithRole(t, conn, id, handle, "agent")
}

func createTestOwner(t *testing.T, conn *sql.DB, id, handle string) {
	t.Helper()
	createTestUserWithRole(t, conn, id, handle, "owner")
}

func createTestUserWithRole(t *testing.T, conn *sql.DB, id, handle, role string) {
	t.Helper()
	_, err := conn.Exec(
		`INSERT INTO users (id, handle, role, active, created_at, updated_at) VALUES (?, ?, ?, TRUE, datetime('now'), datetime('now'))`,
		id, handle, role,
	)
	require.NoError(t, err)
}

// countRows returns the current row count of table — used by the
// append-only (I17/Done-when 3) and idempotency (I19/Done-when 4) tests to
// assert "a row was/wasn't added" directly, rather than inferring it from
// a response shape.
func countRows(t *testing.T, conn *sql.DB, table string) int {
	t.Helper()
	var n int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM `+table).Scan(&n))
	return n
}

// strPtr is a small pointer-literal helper the tests use repeatedly for
// the new nullable string fields (AssigneeID/Priority).
func strPtr(s string) *string { return &s }
