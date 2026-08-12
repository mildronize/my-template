package todo

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepo_CreateAndGetRoundTrip(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "owner-1", "owner-one")
	repo := NewRepo(conn)

	created, err := repo.Create(ctx, "owner-1", "write the launch doc")
	require.NoError(t, err)
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, "owner-1", created.OwnerID)
	assert.Equal(t, "write the launch doc", created.Title)
	assert.False(t, created.Done, "done must start false")
	assert.False(t, created.CreatedAt.IsZero())
	assert.Equal(t, created.CreatedAt, created.UpdatedAt)

	got, err := repo.GetByIDAndOwner(ctx, created.ID, "owner-1")
	require.NoError(t, err)
	assert.Equal(t, created, got)
}

func TestRepo_ListByOwner_OrderedNewestFirst_OwnRowsOnly(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "owner-1", "owner-one")
	createTestUser(t, conn, "owner-2", "owner-two")
	repo := NewRepo(conn)

	first, err := repo.Create(ctx, "owner-1", "first")
	require.NoError(t, err)
	second, err := repo.Create(ctx, "owner-1", "second")
	require.NoError(t, err)
	_, err = repo.Create(ctx, "owner-2", "someone else's todo")
	require.NoError(t, err)

	// created_at has second-level resolution in this schema (TIMESTAMP),
	// so force a deterministic order instead of relying on wall-clock
	// gaps between the two inserts above.
	_, err = conn.ExecContext(ctx, `UPDATE todos SET created_at = ? WHERE id = ?`, first.CreatedAt.Add(-time.Hour), first.ID)
	require.NoError(t, err)

	list, err := repo.ListByOwner(ctx, "owner-1")
	require.NoError(t, err)
	require.Len(t, list, 2, "must not include owner-2's todo")
	assert.Equal(t, second.ID, list[0].ID, "newest first")
	assert.Equal(t, first.ID, list[1].ID)
}

// TestI3_RepoOwnershipScoping_GetUpdateDelete — I3: a todo that exists but
// belongs to a different owner is indistinguishable, at the repo layer,
// from one that never existed. Exercised directly against Get/Update/
// Delete rather than only through the HTTP layer (handler_test.go), so
// the invariant is proven at the layer that actually enforces it.
func TestI3_RepoOwnershipScoping_GetUpdateDelete(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "owner-1", "owner-one")
	createTestUser(t, conn, "owner-2", "owner-two")
	repo := NewRepo(conn)

	theirs, err := repo.Create(ctx, "owner-2", "someone else's private todo")
	require.NoError(t, err)

	_, err = repo.GetByIDAndOwner(ctx, theirs.ID, "owner-1")
	assert.ErrorIs(t, err, ErrNotFound, "a different owner's todo must read as not_found, never forbidden")

	_, err = repo.Update(ctx, theirs.ID, "owner-1", "hijacked title", true)
	assert.ErrorIs(t, err, ErrNotFound, "a different owner must not be able to update it either")

	err = repo.Delete(ctx, theirs.ID, "owner-1")
	assert.ErrorIs(t, err, ErrNotFound, "a different owner must not be able to delete it either")

	// The row is untouched — none of the above actually mutated it.
	stillTheirs, err := repo.GetByIDAndOwner(ctx, theirs.ID, "owner-2")
	require.NoError(t, err)
	assert.Equal(t, theirs, stillTheirs)
}

func TestRepo_GetByIDAndOwner_UnknownID(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "owner-1", "owner-one")
	repo := NewRepo(conn)

	_, err := repo.GetByIDAndOwner(ctx, "does-not-exist", "owner-1")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRepo_Update_ChangesTitleAndDoneAndBumpsUpdatedAt(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "owner-1", "owner-one")
	repo := NewRepo(conn)

	created, err := repo.Create(ctx, "owner-1", "original title")
	require.NoError(t, err)

	updated, err := repo.Update(ctx, created.ID, "owner-1", "new title", true)
	require.NoError(t, err)
	assert.Equal(t, "new title", updated.Title)
	assert.True(t, updated.Done)
	assert.Equal(t, created.CreatedAt, updated.CreatedAt, "created_at must not change")
	assert.True(t, !updated.UpdatedAt.Before(created.UpdatedAt))
}

// TestRepo_Delete_AlreadyDeletedIsAlsoNotFound — API.md: deleting an
// already-deleted id is not_found too, naturally idempotent, no
// special-casing needed.
func TestRepo_Delete_AlreadyDeletedIsAlsoNotFound(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "owner-1", "owner-one")
	repo := NewRepo(conn)

	created, err := repo.Create(ctx, "owner-1", "delete me")
	require.NoError(t, err)

	require.NoError(t, repo.Delete(ctx, created.ID, "owner-1"))

	err = repo.Delete(ctx, created.ID, "owner-1")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRepo_Create_TitleTooLongRejectedByStorageLayer(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "owner-1", "owner-one")
	repo := NewRepo(conn)

	tooLong := make([]byte, 201)
	for i := range tooLong {
		tooLong[i] = 'a'
	}

	_, err := repo.Create(ctx, "owner-1", string(tooLong))
	// DATA_MODEL.md's 1-200 char rule is enforced primarily by
	// openapi.yaml's request validation (before a request even reaches
	// this layer — handler_test.go covers that), and again here by the
	// todos.title CHECK constraint as defense in depth — this test proves
	// the second layer actually holds on its own, independent of the
	// first.
	assert.Error(t, err)
}

// --- I4: this repo only ever queries the todos table --------------------

// TestI4_TodoRepoOnlyQueriesTodosTable — I4 ("one seam reads identity";
// applied here as "one repo, one table" for the non-identity side of that
// boundary): internal/todo's repo must only ever query the todos table,
// and internal/identity's repo must only ever query users/api_keys.
// Checked statically against the sqlc query source each repo.go is
// generated from (db/queries/*.sql) — the same static-source approach
// internal/architecture_test.go uses for the gin/sqlc import rules —
// rather than inferred indirectly from runtime behavior.
func TestI4_TodoRepoOnlyQueriesTodosTable(t *testing.T) {
	root := repoRootForTests(t)
	queriesDir := filepath.Join(root, "db", "queries")

	assertQueriesReferenceOnlyTable(t, filepath.Join(queriesDir, "todos.sql"), "todos", []string{"users", "api_keys"})
	assertQueriesReferenceOnlyTable(t, filepath.Join(queriesDir, "users.sql"), "users", []string{"todos", "api_keys"})
	assertQueriesReferenceOnlyTable(t, filepath.Join(queriesDir, "api_keys.sql"), "api_keys", []string{"todos", "users"})
}

// assertQueriesReferenceOnlyTable asserts that the sqlc query file at path
// mentions ownTable (as a whole word — proving the file isn't vacuously
// empty of the table it's supposed to own) and mentions none of
// forbiddenTables.
func assertQueriesReferenceOnlyTable(t *testing.T, path, ownTable string, forbiddenTables []string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoErrorf(t, err, "reading %s", path)
	content := string(data)

	assert.Regexpf(t, `(?i)\b`+regexp.QuoteMeta(ownTable)+`\b`, content,
		"%s must reference its own table %q at least once", path, ownTable)

	for _, forbidden := range forbiddenTables {
		assert.NotRegexpf(t, `(?i)\b`+regexp.QuoteMeta(forbidden)+`\b`, content,
			"%s must not reference table %q — I4: one repo, one table", path, forbidden)
	}
}
