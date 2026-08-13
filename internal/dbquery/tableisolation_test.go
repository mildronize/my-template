package dbquery

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repoRootForDbqueryTests resolves the module root from this test file's
// own location (mirrors internal/identity/identity_testutil_test.go's
// repoRootForTests), so tests work regardless of the directory `go test`
// is invoked from.
func repoRootForDbqueryTests(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "could not determine this test file's own location")
	// this file lives at <root>/internal/dbquery/<this file>.go
	return filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
}

func writeQueryFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
}

// failRecorder implements testingT (tableisolation.go) by recording
// whether a failure happened instead of failing the real test.
//
// Why this exists: t.Run's subtests propagate failure to every ancestor
// test regardless of what the parent asserts afterward — so a permanent,
// always-green "prove this check catches a real violation" test cannot
// be expressed with a real subtest; the subtest failing (correctly) would
// always mark the outer test failed too, no matter what it asserts next.
// A recorder that observes failure without triggering Go's own test
// failure machinery is the standard way around that.
type failRecorder struct {
	failed bool
}

func (r *failRecorder) Errorf(string, ...interface{}) { r.failed = true }
func (r *failRecorder) FailNow()                      { r.failed = true; runtime.Goexit() }
func (r *failRecorder) Helper()                       {}

// checkFails runs fn against a failRecorder in its own goroutine (so a
// require-style FailNow's runtime.Goexit only stops fn, not the real
// test) and reports whether fn reported any failure.
func checkFails(fn func(t testingT)) bool {
	rec := &failRecorder{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(rec)
	}()
	<-done
	return rec.failed
}

// --- comment-prose regression (the first bug this milestone found) ---

func TestReferencedTablesStrict_IgnoresProseInLineComments(t *testing.T) {
	content := `-- and avoiding sqlc.embed here keeps the numbering contiguous from the
-- previous migration, updating the reader's mental model as they go.
SELECT * FROM todo_events WHERE todo_id = ?;`

	got, err := referencedTablesStrict(content)

	require.NoError(t, err)
	assert.Contains(t, got, "todo_events", "the real FROM clause outside any comment must still be found")
	assert.NotContains(t, got, "the", `a comment's "from the"/"into the"/"updating the" must not be read as a table reference`)
	assert.Len(t, got, 1, "only the one real table reference should have been extracted: %v", got)
}

func TestReferencedTablesStrict_CommentedOutQueryIsNotATableReference(t *testing.T) {
	content := `-- old approach, no longer used:
-- SELECT * FROM legacy_table WHERE id = ?;
SELECT * FROM todos WHERE id = ?;`

	got, err := referencedTablesStrict(content)

	require.NoError(t, err)
	assert.Contains(t, got, "todos")
	assert.NotContains(t, got, "legacy_table", "a commented-out query must not count as a live reference")
}

// --- false negatives (the second finding this milestone made): the
// scanner must refuse these forms outright, not silently under-extract ---

func TestReferencedTablesStrict_RefusesCommaJoin(t *testing.T) {
	_, err := referencedTablesStrict(`SELECT * FROM users, api_keys WHERE users.id = api_keys.user_id;`)
	require.Error(t, err, "a comma-separated table list must be refused, not silently reduced to just the first table")
	assert.Contains(t, err.Error(), "unsupported SQL form")
}

func TestReferencedTablesStrict_RefusesDoubleQuotedIdentifier(t *testing.T) {
	_, err := referencedTablesStrict(`SELECT * FROM "users" WHERE id = ?;`)
	require.Error(t, err, `a quoted identifier must be refused, not silently produce zero references`)
	assert.Contains(t, err.Error(), "unsupported SQL form")
}

func TestReferencedTablesStrict_RefusesBacktickIdentifier(t *testing.T) {
	_, err := referencedTablesStrict("SELECT * FROM `users` WHERE id = ?;")
	require.Error(t, err, "a backtick-quoted identifier must be refused, not silently produce zero references")
	assert.Contains(t, err.Error(), "unsupported SQL form")
}

func TestReferencedTablesStrict_RefusesBracketIdentifier(t *testing.T) {
	_, err := referencedTablesStrict(`SELECT * FROM [users] WHERE id = ?;`)
	require.Error(t, err, "a bracket-quoted identifier must be refused, not silently produce zero references")
	assert.Contains(t, err.Error(), "unsupported SQL form")
}

func TestReferencedTablesStrict_CommentBetweenKeywordAndTableIsHandledByCommentStripping(t *testing.T) {
	// Not a remaining gap, despite looking like one at first: comments
	// are stripped before the keyword scan ever runs, so a comment
	// interposed between JOIN and the table name collapses to plain
	// whitespace and resolves normally. This exact shape was a real
	// false negative in the pre-comment-stripping version of this
	// scanner (the same bug class as the "the" phantom-table finding) —
	// fixed as a side effect of stripping comments first, not something
	// the strict-parse refusal needs to additionally handle. Verified
	// here rather than assumed, so a future change to strip order
	// doesn't silently reopen this.
	content := "SELECT * FROM todos t\nJOIN -- why we do this\n    users u ON t.created_by = u.id;"
	got, err := referencedTablesStrict(content)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"todos", "users"}, got)
}

func TestReferencedTablesStrict_PlainNewlineBetweenKeywordAndTableStillWorks(t *testing.T) {
	// Control: a bare newline (no comment text in between) is ordinary
	// whitespace and must still resolve normally — this isn't "reject
	// anything unusual", only "reject what isn't a plain identifier".
	content := "SELECT * FROM todos t\nJOIN\n    users u ON t.created_by = u.id;"
	got, err := referencedTablesStrict(content)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"todos", "users"}, got)
}

// --- the redesigned mechanism itself: explicit ownership + narrow,
// mechanically-enforced read-only grants ---

func TestAssertQueryFileReferencesOnlyOwnTable_LegitimateSameModuleReferenceStillPasses(t *testing.T) {
	dir := t.TempDir()
	writeQueryFile(t, dir, "users.sql", `
-- name: GetUser :one
SELECT * FROM users WHERE id = ?;

-- name: JoinOwnModuleTable :many
SELECT * FROM users JOIN api_keys ON api_keys.user_id = users.id;
`)
	// api_keys belongs to the same module (identity) as users — must pass
	// with no grant needed at all, purely from TableOwnership.
	AssertQueryFileReferencesOnlyOwnTable(t, dir, "users.sql", "users")
}

func TestAssertQueryFileReferencesOnlyOwnTable_GrantedReadOnlyReferencePasses(t *testing.T) {
	dir := t.TempDir()
	writeQueryFile(t, dir, "todo_events.sql", `
-- name: InsertEvent :one
INSERT INTO todo_events (id) VALUES (?);

-- name: Feed :many
SELECT * FROM todo_events JOIN users ON users.id = todo_events.actor_id;
`)
	// users is owned by a different module (identity), but todo_events.sql
	// has a real ReadOnlyGrant for it, and this content only reads it.
	AssertQueryFileReferencesOnlyOwnTable(t, dir, "todo_events.sql", "todo_events")
}

func TestAssertQueryFileReferencesOnlyOwnTable_UngrantedCrossModuleReferenceFails(t *testing.T) {
	dir := t.TempDir()
	writeQueryFile(t, dir, "todos.sql", `
-- name: SneakyRead :one
SELECT * FROM todos JOIN api_keys ON api_keys.user_id = todos.created_by;
`)
	// api_keys belongs to identity, not todo, and there is no grant for
	// todos.sql to read it — this must still be caught.
	failed := checkFails(func(t testingT) {
		AssertQueryFileReferencesOnlyOwnTable(t, dir, "todos.sql", "todos")
	})
	assert.True(t, failed, "todos.sql reading api_keys (identity's table) with no grant must fail I4")
}

func TestAssertQueryFileReferencesOnlyOwnTable_GrantIsReadOnlyByMechanismNotJustIntent(t *testing.T) {
	dir := t.TempDir()
	writeQueryFile(t, dir, "todo_events.sql", `
-- name: InsertEvent :one
INSERT INTO todo_events (id) VALUES (?);

-- name: MaliciousWrite :one
UPDATE users SET role = 'owner' WHERE id = ?;
`)
	// todo_events.sql has a real ReadOnlyGrant for users — but this
	// content WRITES to users via UPDATE. The grant must not cover that:
	// Clara's required attack, stated exactly — "todo_events.sql doing
	// UPDATE users fails even though users is on its allowlist".
	failed := checkFails(func(t testingT) {
		AssertQueryFileReferencesOnlyOwnTable(t, dir, "todo_events.sql", "todo_events")
	})
	assert.True(t, failed, "a ReadOnlyGrant must not permit a write to the granted table")
}

func TestAssertQueryFileReferencesOnlyOwnTable_UnknownTableHasNoOwnerFailsLoudly(t *testing.T) {
	dir := t.TempDir()
	writeQueryFile(t, dir, "todos.sql", `
-- name: ReadTodos :many
SELECT * FROM todos;

-- name: ReadUnknownTable :many
SELECT * FROM snippets;
`)
	failed := checkFails(func(t testingT) {
		AssertQueryFileReferencesOnlyOwnTable(t, dir, "todos.sql", "todos")
	})
	assert.True(t, failed, "a table with no entry in TableOwnership must fail loudly, not pass silently")
}

// --- AssertEveryReadOnlyGrantIsExercised: an unused grant is a finding,
// not a no-op ---

func TestAssertEveryReadOnlyGrantIsExercised_CatchesAnUnusedGrant(t *testing.T) {
	dir := t.TempDir()
	// ReadOnlyGrants (package-level, real) names todo_events.sql/users —
	// this synthetic todo_events.sql never references users at all, so
	// the real grant is unused against this directory.
	writeQueryFile(t, dir, "todo_events.sql", `
-- name: InsertEvent :one
INSERT INTO todo_events (id) VALUES (?);
`)
	failed := checkFails(func(t testingT) {
		AssertEveryReadOnlyGrantIsExercised(t, dir)
	})
	assert.True(t, failed, "a grant naming a reference that doesn't exist in the file must fail, not pass silently")
}

func TestAssertEveryReadOnlyGrantIsExercised_RealGrantsAreAllUsed(t *testing.T) {
	root := repoRootForDbqueryTests(t)
	AssertEveryReadOnlyGrantIsExercised(t, filepath.Join(root, "db", "queries"))
}
