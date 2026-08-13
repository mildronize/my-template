package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mildronize/my-template/internal/identity"
	"github.com/mildronize/my-template/internal/platform"
)

// newSeedTestRepo opens a fresh, fully-migrated SQLite file in a temp dir
// and wires an *identity.Repo on top of it — the same OpenDB/Migrate/
// NewRepo sequence main() itself runs, so this test exercises the real
// database path (unique constraints included), not a fake.
func newSeedTestRepo(t *testing.T) *identity.Repo {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "seed-test.db")
	db, err := platform.OpenDB(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, platform.Migrate(db))

	return identity.NewRepo(db)
}

// TestSeedOwner_IdempotentAcrossTwoRuns is Done-when 13's own check: run
// the seed logic twice against the same database and assert exactly one
// owner row exists after both runs, and that the second run reports
// "already exists" rather than erroring or creating a duplicate.
func TestSeedOwner_IdempotentAcrossTwoRuns(t *testing.T) {
	repo := newSeedTestRepo(t)
	ctx := context.Background()
	const sub = "test-owner-sso-subject-123"

	first, err := seedOwner(ctx, repo, sub)
	require.NoError(t, err)
	assert.False(t, first.alreadyExisted, "the first run must create the row, not find one already there")
	assert.Equal(t, ownerHandle, first.user.Handle)
	assert.Equal(t, "owner", first.user.Role)
	assert.True(t, first.user.Active)
	assert.Equal(t, sub, first.user.SSOSubject)
	require.NotEmpty(t, first.user.ID)

	second, err := seedOwner(ctx, repo, sub)
	require.NoError(t, err, "a second run for the same sso_subject must not error")
	assert.True(t, second.alreadyExisted, "a second run must report the row already existed")
	assert.Equal(t, first.user.ID, second.user.ID, "a second run must resolve to the same row, not a new one")

	owner, err := repo.GetUserBySSOSubject(ctx, sub)
	require.NoError(t, err)
	assert.Equal(t, first.user.ID, owner.ID)
}

// TestSeedOwner_MissingSSOSubjectIsAClearError is the first documented
// edge case: SEED_OWNER_SSO_SUBJECT not set must produce a clear error,
// never a row created with an empty sso_subject.
func TestSeedOwner_MissingSSOSubjectIsAClearError(t *testing.T) {
	repo := newSeedTestRepo(t)

	_, err := seedOwner(context.Background(), repo, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SEED_OWNER_SSO_SUBJECT")

	_, lookupErr := repo.GetUserByHandle(context.Background(), ownerHandle)
	assert.ErrorIs(t, lookupErr, identity.ErrNotFound, "no row must be created when SEED_OWNER_SSO_SUBJECT is unset")
}
