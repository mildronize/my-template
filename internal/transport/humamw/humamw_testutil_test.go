package humamw

import (
	"database/sql"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver for these tests

	"github.com/mildronize/my-template/internal/identity"
)

// repoRootForTests mirrors internal/transport/publicapi's own test helper
// of the same name (publicapi_testutil_test.go) — each package with
// DB-backed tests keeps its own copy rather than sharing one across a
// package boundary ARCHITECTURE.md deliberately drew (see that file's own
// doc comment).
func repoRootForTests(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "could not determine this test file's own location")
	// this file lives at <root>/internal/transport/humamw/<this file>.go
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))
}

// newTestDB opens a fresh temp-file SQLite database and applies every
// migration under db/migrations against it via goose — RequireActor's
// tests drive a real identity.Service against real schema, same
// reasoning as internal/transport/publicapi's own newTestDB.
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

// seedAgentWithAPIKey creates a role="agent" user and a live (unexpired,
// unrevoked) API key for it, returning the raw key — the credential a
// test sends as "Authorization: Bearer <raw>". Mirrors cmd/issue-key's
// own hash-then-store convention via identity.HashAPIKey (the same
// function ResolveActor verifies against, per that function's own doc
// comment: "one definition, not two copies that could drift apart").
func seedAgentWithAPIKey(t *testing.T, repo *identity.Repo) (user identity.User, rawKey string) {
	t.Helper()
	ctx := t.Context()

	user, err := repo.CreateUser(ctx, "test-agent", "agent", nil)
	require.NoError(t, err)

	rawKey = "tpl_test_" + user.ID
	_, err = repo.CreateAPIKey(ctx, user.ID, identity.HashAPIKey(rawKey), "tpl_test", time.Now().Add(time.Hour))
	require.NoError(t, err)

	return user, rawKey
}
