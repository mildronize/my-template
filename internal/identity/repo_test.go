package identity

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mildronize/my-template/internal/dbquery"
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

// TestRepo_ListActiveUsers_BothRoles_ExcludesInactive_OrderedByHandle is
// GET /api/bff/users' own source query — mirrors my-task's user.ts
// router (`WHERE active = true ORDER BY handle`, "humans and agents in
// one list"). Both roles must appear (not just agents, unlike
// ListAllAgentAPIKeys) and an inactive user must not — a floor-first
// shape would be pointless here (a real users table always has at least
// the owner row), but the exclusion and ordering are each independently
// asserted rather than inferred from one one of them holding.
func TestRepo_ListActiveUsers_BothRoles_ExcludesInactive_OrderedByHandle(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	repo := NewRepo(conn)

	_, err := repo.CreateUser(ctx, "zed-owner", "owner", nil)
	require.NoError(t, err)
	_, err = repo.CreateUser(ctx, "alice-agent", "agent", nil)
	require.NoError(t, err)
	agent2, err := repo.CreateUser(ctx, "bob-agent", "agent", nil)
	require.NoError(t, err)

	// Deactivated directly at the SQL level — Repo has no deactivate
	// method of its own (nothing in this codebase ever flips a user
	// inactive; the column exists for completeness/future use), so this
	// reaches past the repo layer on purpose, the same way other repo
	// tests seed fixture state no exported method produces.
	_, err = conn.ExecContext(ctx, `UPDATE users SET active = 0 WHERE id = ?`, agent2.ID)
	require.NoError(t, err)

	users, err := repo.ListActiveUsers(ctx)
	require.NoError(t, err)

	var handles []string
	for _, u := range users {
		handles = append(handles, u.Handle)
	}
	// alice-agent, then zed-owner, alphabetically by handle — NOT
	// insertion order (owner was created first) and NOT role-grouped.
	assert.Equal(t, []string{"alice-agent", "zed-owner"}, handles,
		"both active roles present, ordered by handle, inactive bob-agent excluded")

	for _, u := range users {
		assert.NotEqual(t, agent2.ID, u.ID, "the deactivated user must not appear")
	}
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

// TestRepo_ListAPIKeysByOwner_ExcludesRevoked_IncludesExpired_OwnRowsOnly
// exercises API.md's `GET /api/v1/keys` listing rule at the repo layer: a
// revoked key never shows up, an expired-but-unrevoked key still does
// (this is a listing decision, not I9's auth-time check — see
// service.go's ListAPIKeys doc comment), and another owner's keys never
// leak in.
func TestRepo_ListAPIKeysByOwner_ExcludesRevoked_IncludesExpired_OwnRowsOnly(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(newTestDB(t))

	owner, err := repo.CreateUser(ctx, "owner-of-keys", "agent", nil)
	require.NoError(t, err)
	other, err := repo.CreateUser(ctx, "someone-else", "agent", nil)
	require.NoError(t, err)

	live, err := repo.CreateAPIKey(ctx, owner.ID, HashAPIKey("tpl_live"), "tpl_livelive1", time.Now().Add(time.Hour))
	require.NoError(t, err)

	expiredButUnrevoked, err := repo.CreateAPIKey(ctx, owner.ID, HashAPIKey("tpl_expired"), "tpl_expiredex", time.Now().Add(-time.Hour))
	require.NoError(t, err)

	toRevoke, err := repo.CreateAPIKey(ctx, owner.ID, HashAPIKey("tpl_revoked"), "tpl_revokedre", time.Now().Add(time.Hour))
	require.NoError(t, err)
	_, err = repo.RevokeAPIKey(ctx, toRevoke.ID, owner.ID)
	require.NoError(t, err)

	_, err = repo.CreateAPIKey(ctx, other.ID, HashAPIKey("tpl_someone-else"), "tpl_otherowne", time.Now().Add(time.Hour))
	require.NoError(t, err)

	list, err := repo.ListAPIKeysByOwner(ctx, owner.ID)
	require.NoError(t, err)

	ids := make([]string, 0, len(list))
	for _, k := range list {
		ids = append(ids, k.ID)
	}
	assert.ElementsMatch(t, []string{live.ID, expiredButUnrevoked.ID}, ids,
		"a revoked key must be excluded, an expired-but-unrevoked key must still show up, and another owner's key must never leak in")
}

// TestI3_RevokeAPIKeyScopedToOwner_AbsenceNotPermission is I3's
// repo-layer proof for this package — internal/invariants_test.go's
// TestDoneWhen12 requires a dedicated TestI3_ test inside every package
// perDomainModuleScopePackages names, and internal/identity is one of
// them (I3's ownership-scoping applies to key-listing/revocation, not
// just todos). This test already existed as TestRepo_RevokeAPIKeyScoped-
// ToOwner and already proved the property — renamed only, to carry the
// naming convention the check greps for; the transport-layer half of I3
// (404 not 403 on someone else's key) lives separately in
// keys_handler_test.go's own TestI3_ tests.
func TestI3_RevokeAPIKeyScopedToOwner_AbsenceNotPermission(t *testing.T) {
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

// TestI21_ListAllAgentAPIKeys_SpansEveryAgent_ListAPIKeysByOwner_StaysSelfScoped
// is I21's own dedicated test — internal/invariants_test.go's
// TestDoneWhen12 requires a test named TestI21_<something> specifically
// inside internal/identity (I21's scope tag is `domain:identity`, not
// `per-domain-module`: this is the one and only package it belongs in, not
// a coverage sweep across every domain module). Two agents each get a
// real key (seeded via repo.CreateUser(..., "agent", ...) directly, the
// same repo-layer-fixture convention TestI3_RevokeAPIKeyScopedToOwner_
// AbsenceNotPermission above already uses — this is NOT the trap GOAL.md
// warns about: that trap was seeding role='owner' fixtures with keys,
// simulating a state production can never reach; role='agent' is exactly
// what a real key-holder is, at the layer that has no access to
// identity.Service.IssueAPIKeyForHandle's CLI-mirroring path in the first
// place). A third, revoked agent key proves the "non-revoked" half of
// I21's own wording.
//
// Both halves of I21's sentence get one test each: the owner-facing query
// spans every agent (not one user_id), and the agent-facing query
// (ListAPIKeysByOwner, GET /api/v1/keys's own repo call, untouched by this
// milestone) stays exactly as self-scoped as it always was.
func TestI21_ListAllAgentAPIKeys_SpansEveryAgent_ListAPIKeysByOwner_StaysSelfScoped(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(newTestDB(t))

	agentA, err := repo.CreateUser(ctx, "agent-a", "agent", nil)
	require.NoError(t, err)
	agentB, err := repo.CreateUser(ctx, "agent-b", "agent", nil)
	require.NoError(t, err)

	keyA, err := repo.CreateAPIKey(ctx, agentA.ID, HashAPIKey("tpl_agent_a"), "tpl_agenta001", time.Now().Add(time.Hour))
	require.NoError(t, err)
	keyB, err := repo.CreateAPIKey(ctx, agentB.ID, HashAPIKey("tpl_agent_b"), "tpl_agentb001", time.Now().Add(time.Hour))
	require.NoError(t, err)

	// A revoked agent key must never appear in the owner-facing listing —
	// I21's own wording is "non-revoked keys", not "every key ever issued".
	revokedKey, err := repo.CreateAPIKey(ctx, agentB.ID, HashAPIKey("tpl_agent_b_old"), "tpl_agentbold1", time.Now().Add(time.Hour))
	require.NoError(t, err)
	_, err = repo.RevokeAPIKey(ctx, revokedKey.ID, agentB.ID)
	require.NoError(t, err)

	// Half 1: the owner-facing query spans BOTH agents, not one user_id.
	all, err := repo.ListAllAgentAPIKeys(ctx)
	require.NoError(t, err)
	allIDs := make([]string, 0, len(all))
	for _, k := range all {
		allIDs = append(allIDs, k.ID)
	}
	assert.ElementsMatch(t, []string{keyA.ID, keyB.ID}, allIDs,
		"ListAllAgentAPIKeys must return every agent's non-revoked key — both agent-a's and agent-b's — and exclude agent-b's revoked one")

	// Half 2: I21's other clause — an agent's own key-listing stays
	// self-scoped (I3, unchanged for this half of the identity domain).
	// agentA's own listing must never include agentB's key.
	scopedToA, err := repo.ListAPIKeysByOwner(ctx, agentA.ID)
	require.NoError(t, err)
	require.Len(t, scopedToA, 1, "an agent's own key-listing must stay scoped to that agent alone")
	assert.Equal(t, keyA.ID, scopedToA[0].ID)

	// RevokeAPIKeyByID (the owner-facing DELETE's own query) can revoke
	// agent-a's key even though the caller isn't "agent-a" itself — no
	// user_id scoping, unlike RevokeAPIKey.
	revoked, err := repo.RevokeAPIKeyByID(ctx, keyA.ID)
	require.NoError(t, err)
	require.NotNil(t, revoked.RevokedAt)

	allAfterRevoke, err := repo.ListAllAgentAPIKeys(ctx)
	require.NoError(t, err)
	afterIDs := make([]string, 0, len(allAfterRevoke))
	for _, k := range allAfterRevoke {
		afterIDs = append(afterIDs, k.ID)
	}
	assert.NotContains(t, afterIDs, keyA.ID, "a key revoked via RevokeAPIKeyByID must no longer appear in the owner-facing listing")
	assert.Contains(t, afterIDs, keyB.ID, "revoking agent-a's key must not touch agent-b's")
}

// TestRepo_DisableOtherAPIKeys_RevokesEverythingExceptKeepID_ScopedToOwner
// exercises DisableOtherAPIKeys against the real schema: every one of
// owner's live keys except keepID gets revoked, a key already revoked
// beforehand is neither double-counted nor errored on, and another
// owner's key is never touched.
func TestRepo_DisableOtherAPIKeys_RevokesEverythingExceptKeepID_ScopedToOwner(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(newTestDB(t))

	owner, err := repo.CreateUser(ctx, "owner-of-keys", "agent", nil)
	require.NoError(t, err)
	other, err := repo.CreateUser(ctx, "someone-else", "agent", nil)
	require.NoError(t, err)

	keep, err := repo.CreateAPIKey(ctx, owner.ID, HashAPIKey("tpl_keep"), "tpl_keepkeep1", time.Now().Add(time.Hour))
	require.NoError(t, err)
	old1, err := repo.CreateAPIKey(ctx, owner.ID, HashAPIKey("tpl_old1"), "tpl_old1old1", time.Now().Add(time.Hour))
	require.NoError(t, err)
	old2, err := repo.CreateAPIKey(ctx, owner.ID, HashAPIKey("tpl_old2"), "tpl_old2old2", time.Now().Add(time.Hour))
	require.NoError(t, err)

	alreadyRevoked, err := repo.CreateAPIKey(ctx, owner.ID, HashAPIKey("tpl_already"), "tpl_alreadyre", time.Now().Add(time.Hour))
	require.NoError(t, err)
	_, err = repo.RevokeAPIKey(ctx, alreadyRevoked.ID, owner.ID)
	require.NoError(t, err)

	_, err = repo.CreateAPIKey(ctx, other.ID, HashAPIKey("tpl_others"), "tpl_othersoth", time.Now().Add(time.Hour))
	require.NoError(t, err)

	count, err := repo.DisableOtherAPIKeys(ctx, owner.ID, keep.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, count, "only old1 and old2 were live and not keepID — alreadyRevoked must not be double-counted")

	keptAfter, err := repo.GetAPIKeyByHash(ctx, HashAPIKey("tpl_keep"))
	require.NoError(t, err)
	assert.Nil(t, keptAfter.RevokedAt, "keepID must not be touched")

	for _, id := range []string{old1.ID, old2.ID} {
		list, err := repo.ListAPIKeysByOwner(ctx, owner.ID)
		require.NoError(t, err)
		for _, k := range list {
			assert.NotEqual(t, id, k.ID, "revoked key must no longer show up as live")
		}
	}

	othersAfter, err := repo.GetAPIKeyByHash(ctx, HashAPIKey("tpl_others"))
	require.NoError(t, err)
	assert.Nil(t, othersAfter.RevokedAt, "a different owner's key must never be touched")
	assert.Equal(t, other.ID, othersAfter.UserID)
}

// --- I4: this repo only ever queries users/api_keys ----------------------

// TestI4_IdentityRepoOnlyQueriesUsersAndAPIKeysTables — I4 ("one seam reads
// identity"; applied here as "one repo, one set of tables" for the
// identity side of that boundary, mirroring
// internal/domain/todo/repo_test.go's TestI4_TodoRepoOnlyQueriesTodosTable):
// internal/identity's repo must only ever query users/api_keys, and must
// never query a table that belongs to a different domain module (todos,
// or whatever a fork replaces it with) — except through an explicit,
// mechanically-enforced read-only grant (dbquery.ReadOnlyGrants).
//
// Checked statically against the sqlc query source each repo.go is
// generated from (db/queries/*.sql), via internal/dbquery — the single
// shared implementation behind this check and internal/domain/todo's
// equivalent, so the two can't drift into two different (and, as task-8
// found, differently buggy) copies of the same logic. Ownership is an
// explicit map (dbquery.TableOwnership), never derived by scanning other
// files' content — see internal/dbquery's own doc comment for why an
// earlier, scan-and-guess version of this mechanism got two things wrong:
// first a hardcoded forbidden-table list that passed vacuously once a
// fork's tables changed underneath it, then (milestone-4) a heuristic
// that could not tell a legitimate cross-module read from an ownership
// claim, misattributing "users" to todo_events.sql because its feed
// query legitimately JOINs it.
//
// This is I4's dedicated identity-module test, added in task-7 once
// _contract/INVARIANTS.md tagged I4 `scope: per-domain-module`
// (internal/invariants_test.go): before that, internal/todo's
// TestI4_TodoRepoOnlyQueriesTodosTable happened to already check
// users.sql/api_keys.sql too, so I4 had real coverage for identity's
// tables — but that coverage lived entirely outside internal/identity's
// own package, which is exactly the gap task-7's per-domain-module scope
// closes: a repo-wide grep for "a TestI4_ test exists somewhere" doesn't
// prove identity has any dedicated test of its own, only that some module
// does.
func TestI4_IdentityRepoOnlyQueriesUsersAndAPIKeysTables(t *testing.T) {
	root := repoRootForTests(t)
	queriesDir := filepath.Join(root, "db", "queries")

	// users.sql and api_keys.sql both belong to the same module
	// (dbquery.TableOwnership) — a query joining them (e.g. resolving an
	// API key's owning user) is automatically I4-legal from that alone, no
	// per-call exemption list needed; a reference to any *other* module's
	// table is still forbidden unless explicitly granted.
	dbquery.AssertQueryFileReferencesOnlyOwnTable(t, queriesDir, "users.sql", "users")
	dbquery.AssertQueryFileReferencesOnlyOwnTable(t, queriesDir, "api_keys.sql", "api_keys")
}
