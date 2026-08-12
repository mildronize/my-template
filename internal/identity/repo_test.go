package identity

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepo_UserCRUDRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(newTestDB(t))

	sub := "hydra|sub-123"
	created, err := repo.CreateUser(ctx, "Luna", "agent", &sub)
	require.NoError(t, err)
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, "Luna", created.Handle)
	assert.Equal(t, "agent", created.Role)
	assert.True(t, created.Active)
	assert.Equal(t, sub, created.SSOSubject)

	byID, err := repo.GetUserByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created, byID)

	byHandle, err := repo.GetUserByHandle(ctx, "Luna")
	require.NoError(t, err)
	assert.Equal(t, created, byHandle)

	bySub, err := repo.GetUserBySSOSubject(ctx, sub)
	require.NoError(t, err)
	assert.Equal(t, created, bySub)

	_, err = repo.GetUserByID(ctx, "does-not-exist")
	assert.ErrorIs(t, err, ErrNotFound)
}

// TestRepo_HandleLookupCaseInsensitive exercises DATA_MODEL.md's "matched
// exactly, case-insensitively, never fuzzily" rule for users.handle.
func TestRepo_HandleLookupCaseInsensitive(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(newTestDB(t))

	created, err := repo.CreateUser(ctx, "Luna", "agent", nil)
	require.NoError(t, err)

	for _, variant := range []string{"luna", "LUNA", "LuNa"} {
		got, err := repo.GetUserByHandle(ctx, variant)
		require.NoError(t, err, "handle lookup for %q", variant)
		assert.Equal(t, created.ID, got.ID)
	}

	// A handle that isn't even close (not a fuzzy match) stays not found.
	_, err = repo.GetUserByHandle(ctx, "luna2")
	assert.ErrorIs(t, err, ErrNotFound)
}

// TestI8_APIKeyStoredHashedNotRaw — I8: the raw key exists only at
// issuance; api_keys.key_hash is one-way and never the raw value itself.
func TestI8_APIKeyStoredHashedNotRaw(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	repo := NewRepo(conn)

	user, err := repo.CreateUser(ctx, "agent-a", "agent", nil)
	require.NoError(t, err)

	raw := "tpl_" + "deadbeefdeadbeefdeadbeefdeadbeef"
	hash := HashAPIKey(raw)
	require.NotEqual(t, raw, hash, "HashAPIKey must not be the identity function")

	created, err := repo.CreateAPIKey(ctx, user.ID, hash, raw[:12], time.Now().Add(90*24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, hash, created.KeyHash)

	// The raw key never touches the stored row at all — only its hash and
	// prefix do. Querying the raw value back out of the table (as sqlc
	// would if a column stored it) is not possible; assert instead that
	// the only way to find this row is by the hash, and that the row's
	// own key_hash column is not (and cannot coincidentally be) the raw
	// key.
	var rawStoredSomewhere string
	err = conn.QueryRowContext(ctx, `SELECT key_hash FROM api_keys WHERE id = ?`, created.ID).Scan(&rawStoredSomewhere)
	require.NoError(t, err)
	assert.Equal(t, hash, rawStoredSomewhere)
	assert.NotEqual(t, raw, rawStoredSomewhere)

	found, err := repo.GetAPIKeyByHash(ctx, hash)
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)

	_, err = repo.GetAPIKeyByHash(ctx, raw) // looking up by the raw value itself must miss
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRepo_RevokeAPIKeyScopedToOwner(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(newTestDB(t))

	owner, err := repo.CreateUser(ctx, "owner-of-key", "agent", nil)
	require.NoError(t, err)
	other, err := repo.CreateUser(ctx, "someone-else", "agent", nil)
	require.NoError(t, err)

	key, err := repo.CreateAPIKey(ctx, owner.ID, HashAPIKey("tpl_abc"), "tpl_abcdefgh", time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.Nil(t, key.RevokedAt)

	// A different user_id can't revoke someone else's key — same
	// "absence, not permission" shape I3 gives todos, applied here to
	// keys.
	_, err = repo.RevokeAPIKey(ctx, key.ID, other.ID)
	assert.ErrorIs(t, err, ErrNotFound)

	revoked, err := repo.RevokeAPIKey(ctx, key.ID, owner.ID)
	require.NoError(t, err)
	require.NotNil(t, revoked.RevokedAt)

	// Revoking an already-revoked key is also a no-match update (the
	// query only matches revoked_at IS NULL).
	_, err = repo.RevokeAPIKey(ctx, key.ID, owner.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}
