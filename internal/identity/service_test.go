package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- fakes -------------------------------------------------------------

type fakeUserRepo struct {
	byID         map[string]User
	byHandle     map[string]User
	bySub        map[string]User
	createCalled bool
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{byID: map[string]User{}, byHandle: map[string]User{}, bySub: map[string]User{}}
}

func (f *fakeUserRepo) put(u User) {
	f.byID[u.ID] = u
	f.byHandle[strings.ToLower(u.Handle)] = u
	if u.SSOSubject != "" {
		f.bySub[u.SSOSubject] = u
	}
}

func (f *fakeUserRepo) GetUserByID(_ context.Context, id string) (User, error) {
	u, ok := f.byID[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}

func (f *fakeUserRepo) GetUserByHandle(_ context.Context, handle string) (User, error) {
	u, ok := f.byHandle[strings.ToLower(handle)]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}

func (f *fakeUserRepo) GetUserBySSOSubject(_ context.Context, sub string) (User, error) {
	u, ok := f.bySub[sub]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}

func (f *fakeUserRepo) CreateUser(_ context.Context, handle, role string, ssoSubject *string) (User, error) {
	f.createCalled = true
	u := User{ID: "generated-" + handle, Handle: handle, Role: role, Active: true}
	if ssoSubject != nil {
		u.SSOSubject = *ssoSubject
	}
	f.put(u)
	return u, nil
}

type fakeAPIKeyRepo struct {
	byHash       map[string]APIKey
	createCalled bool
	revokeCalled bool
}

func newFakeAPIKeyRepo() *fakeAPIKeyRepo {
	return &fakeAPIKeyRepo{byHash: map[string]APIKey{}}
}

func (f *fakeAPIKeyRepo) put(k APIKey) { f.byHash[k.KeyHash] = k }

func (f *fakeAPIKeyRepo) GetAPIKeyByHash(_ context.Context, hash string) (APIKey, error) {
	k, ok := f.byHash[hash]
	if !ok {
		return APIKey{}, ErrNotFound
	}
	return k, nil
}

func (f *fakeAPIKeyRepo) CreateAPIKey(_ context.Context, userID, keyHash, keyPrefix string, expiresAt time.Time) (APIKey, error) {
	f.createCalled = true
	k := APIKey{ID: "key-" + keyHash[:8], UserID: userID, KeyHash: keyHash, KeyPrefix: keyPrefix, CreatedAt: time.Now(), ExpiresAt: expiresAt}
	f.put(k)
	return k, nil
}

func (f *fakeAPIKeyRepo) ListAPIKeysByOwner(_ context.Context, userID string) ([]APIKey, error) {
	var keys []APIKey
	for _, k := range f.byHash {
		if k.UserID == userID && k.RevokedAt == nil {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (f *fakeAPIKeyRepo) RevokeAPIKey(_ context.Context, id, userID string) (APIKey, error) {
	f.revokeCalled = true
	for hash, k := range f.byHash {
		if k.ID == id && k.UserID == userID && k.RevokedAt == nil {
			now := time.Now()
			k.RevokedAt = &now
			f.byHash[hash] = k
			return k, nil
		}
	}
	return APIKey{}, ErrNotFound
}

// DisableOtherAPIKeys mirrors repo.go's real implementation exactly
// (ListAPIKeysByOwner-shaped filter, then revoke each) rather than a
// shortcut, so a test exercising Service.Rotate against this fake is
// exercising the same "list live, skip keepID, revoke the rest" shape the
// real Repo does.
func (f *fakeAPIKeyRepo) DisableOtherAPIKeys(_ context.Context, userID, keepID string) (int, error) {
	count := 0
	for hash, k := range f.byHash {
		if k.UserID != userID || k.ID == keepID || k.RevokedAt != nil {
			continue
		}
		now := time.Now()
		k.RevokedAt = &now
		f.byHash[hash] = k
		count++
	}
	return count, nil
}

// ListAllAgentAPIKeys and RevokeAPIKeyByID exist only to satisfy the
// APIKeyRepo interface for this fake's other (unrelated) Service tests —
// this fake has no notion of a users table/role at all, so it cannot
// reproduce the real Repo's role='agent' filter. I21's actual role-scoping
// behavior is proven against a real SQLite schema instead: repo_test.go's
// TestI21_... and internal/transport/bff/keys_handler_test.go's rewritten
// suite, neither of which uses this fake.
func (f *fakeAPIKeyRepo) ListAllAgentAPIKeys(_ context.Context) ([]APIKey, error) {
	var keys []APIKey
	for _, k := range f.byHash {
		if k.RevokedAt == nil {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (f *fakeAPIKeyRepo) RevokeAPIKeyByID(_ context.Context, id string) (APIKey, error) {
	f.revokeCalled = true
	for hash, k := range f.byHash {
		if k.ID == id && k.RevokedAt == nil {
			now := time.Now()
			k.RevokedAt = &now
			f.byHash[hash] = k
			return k, nil
		}
	}
	return APIKey{}, ErrNotFound
}

type fakeJWTVerifier struct {
	sub   string
	err   error
	calls int
}

func (f *fakeJWTVerifier) Verify(_ context.Context, _ string) (string, error) {
	f.calls++
	return f.sub, f.err
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestService(users UserRepo, keys APIKeyRepo, jwtV JWTVerifier, now time.Time) *Service {
	svc := NewService(users, keys, jwtV, testLogger())
	svc.Now = func() time.Time { return now }
	return svc
}

func putAPIKey(keys *fakeAPIKeyRepo, rawToken, userID string, expiresAt time.Time, revokedAt *time.Time) {
	keys.put(APIKey{
		ID:        "key-" + rawToken,
		UserID:    userID,
		KeyHash:   HashAPIKey(rawToken),
		KeyPrefix: rawToken[:min(len(rawToken), 12)],
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
		RevokedAt: revokedAt,
	})
}

// --- HashAPIKey ----------------------------------------------------------

func TestHashAPIKey_SHA256Hex(t *testing.T) {
	sum := sha256.Sum256([]byte("tpl_example"))
	want := hex.EncodeToString(sum[:])
	assert.Equal(t, want, HashAPIKey("tpl_example"))
	assert.NotEqual(t, "tpl_example", HashAPIKey("tpl_example"))
}

// --- bearer token shape ---------------------------------------------------

func TestResolveActor_MissingAuthorizationHeader(t *testing.T) {
	svc := newTestService(newFakeUserRepo(), newFakeAPIKeyRepo(), nil, time.Now())
	_, err := svc.ResolveActor(context.Background(), "")
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestResolveActor_MalformedAuthorizationHeader(t *testing.T) {
	svc := newTestService(newFakeUserRepo(), newFakeAPIKeyRepo(), nil, time.Now())
	_, err := svc.ResolveActor(context.Background(), "Basic dXNlcjpwYXNz")
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestResolveActor_EmptyBearerToken(t *testing.T) {
	svc := newTestService(newFakeUserRepo(), newFakeAPIKeyRepo(), nil, time.Now())
	_, err := svc.ResolveActor(context.Background(), "Bearer ")
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestResolveActor_UnknownTokenResolvesViaNeitherPath(t *testing.T) {
	jwtV := &fakeJWTVerifier{err: errors.New("not a jwt")}
	svc := newTestService(newFakeUserRepo(), newFakeAPIKeyRepo(), jwtV, time.Now())
	_, err := svc.ResolveActor(context.Background(), "Bearer garbage-token")
	assert.ErrorIs(t, err, ErrUnauthorized)
	assert.Equal(t, 1, jwtV.calls, "an unmatched API key token should still be tried as a JWT")
}

// --- API key path ----------------------------------------------------------

func TestResolveActor_APIKeySuccess(t *testing.T) {
	users := newFakeUserRepo()
	keys := newFakeAPIKeyRepo()
	agent := User{ID: "u1", Handle: "agent-a", Role: "agent", Active: true}
	users.put(agent)
	now := time.Now()
	putAPIKey(keys, "tpl_liveliveliveliv", agent.ID, now.Add(time.Hour), nil)

	svc := newTestService(users, keys, nil, now)
	got, err := svc.ResolveActor(context.Background(), "Bearer tpl_liveliveliveliv")
	require.NoError(t, err)
	assert.Equal(t, agent.ID, got.ID)
}

func TestI9_ExpiredAPIKeyFailsAuth(t *testing.T) {
	users := newFakeUserRepo()
	keys := newFakeAPIKeyRepo()
	agent := User{ID: "u1", Handle: "agent-a", Role: "agent", Active: true}
	users.put(agent)
	now := time.Now()
	putAPIKey(keys, "tpl_expiredexpired12", agent.ID, now.Add(-time.Minute), nil) // expired 1 minute ago

	svc := newTestService(users, keys, nil, now)
	_, err := svc.ResolveActor(context.Background(), "Bearer tpl_expiredexpired12")
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestI9_RevokedAPIKeyFailsAuth(t *testing.T) {
	users := newFakeUserRepo()
	keys := newFakeAPIKeyRepo()
	agent := User{ID: "u1", Handle: "agent-a", Role: "agent", Active: true}
	users.put(agent)
	now := time.Now()
	revokedAt := now.Add(-time.Hour)
	putAPIKey(keys, "tpl_revokedrevoked12", agent.ID, now.Add(time.Hour), &revokedAt) // not expired, but revoked

	svc := newTestService(users, keys, nil, now)
	_, err := svc.ResolveActor(context.Background(), "Bearer tpl_revokedrevoked12")
	assert.ErrorIs(t, err, ErrUnauthorized)
}

// TestI9_ExpiredAPIKeyDoesNotFallThroughToJWT — task-2.md: a token that
// matched an api_keys row is an API key, full stop; an expired/revoked
// match must not be retried against the JWT branch even when a JWT
// verifier is wired and would happily accept the same token string.
func TestI9_ExpiredAPIKeyDoesNotFallThroughToJWT(t *testing.T) {
	users := newFakeUserRepo()
	keys := newFakeAPIKeyRepo()
	agent := User{ID: "u1", Handle: "agent-a", Role: "agent", Active: true}
	users.put(agent)
	now := time.Now()
	putAPIKey(keys, "tpl_expiredexpired12", agent.ID, now.Add(-time.Minute), nil)

	jwtV := &fakeJWTVerifier{sub: "would-succeed-if-tried"} // deliberately always "valid"
	users.bySub["would-succeed-if-tried"] = User{ID: "sneaky", Handle: "sneaky", Role: "agent", Active: true}

	svc := newTestService(users, keys, jwtV, now)
	_, err := svc.ResolveActor(context.Background(), "Bearer tpl_expiredexpired12")
	assert.ErrorIs(t, err, ErrUnauthorized)
	assert.Equal(t, 0, jwtV.calls, "expired API key match must not fall through to the JWT branch")
}

// --- JWT path ----------------------------------------------------------

func TestResolveActor_JWTSuccess(t *testing.T) {
	users := newFakeUserRepo()
	keys := newFakeAPIKeyRepo()
	agent := User{ID: "u1", Handle: "agent-a", Role: "agent", Active: true, SSOSubject: "sso|abc"}
	users.put(agent)

	jwtV := &fakeJWTVerifier{sub: "sso|abc"}
	svc := newTestService(users, keys, jwtV, time.Now())
	got, err := svc.ResolveActor(context.Background(), "Bearer some.jwt.token")
	require.NoError(t, err)
	assert.Equal(t, agent.ID, got.ID)
	assert.Equal(t, 1, jwtV.calls)
}

// TestI10_JWTSubjectWithNoMatchingUserIsUnauthorized_NeverAutoProvisioned
// — I10: a JWT `sub` with no matching users.sso_subject row is
// unauthorized, never auto-created.
func TestI10_JWTSubjectWithNoMatchingUserIsUnauthorized_NeverAutoProvisioned(t *testing.T) {
	users := newFakeUserRepo()
	keys := newFakeAPIKeyRepo()
	jwtV := &fakeJWTVerifier{sub: "sso|unknown-subject"}

	svc := newTestService(users, keys, jwtV, time.Now())
	_, err := svc.ResolveActor(context.Background(), "Bearer some.jwt.token")
	assert.ErrorIs(t, err, ErrUnauthorized)
	assert.False(t, users.createCalled, "an unrecognized JWT subject must never provision a users row")
}

func TestResolveActor_JWTValidationFailureIsUnauthorized(t *testing.T) {
	users := newFakeUserRepo()
	keys := newFakeAPIKeyRepo()
	jwtV := &fakeJWTVerifier{err: errors.New("signature invalid")}

	svc := newTestService(users, keys, jwtV, time.Now())
	_, err := svc.ResolveActor(context.Background(), "Bearer some.jwt.token")
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestResolveActor_NilJWTVerifierMeansBranchNeverMatches(t *testing.T) {
	svc := newTestService(newFakeUserRepo(), newFakeAPIKeyRepo(), nil, time.Now())
	_, err := svc.ResolveActor(context.Background(), "Bearer anything")
	assert.ErrorIs(t, err, ErrUnauthorized)
}

// --- I2: the shared owner-rejection gate, exercised via both branches ------

// TestI2_BearerNeverResolvesToOwner_APIKeyPath — I2: neither credential
// path may resolve to role='owner'. Drives execution through the shared
// gate via the API-key branch.
func TestI2_BearerNeverResolvesToOwner_APIKeyPath(t *testing.T) {
	users := newFakeUserRepo()
	keys := newFakeAPIKeyRepo()
	owner := User{ID: "owner-1", Handle: "มายด์", Role: "owner", Active: true}
	users.put(owner)
	now := time.Now()
	putAPIKey(keys, "tpl_ownerkeyownerkey1", owner.ID, now.Add(time.Hour), nil)

	svc := newTestService(users, keys, nil, now)
	_, err := svc.ResolveActor(context.Background(), "Bearer tpl_ownerkeyownerkey1")
	assert.ErrorIs(t, err, ErrUnauthorized)
}

// TestI2_BearerNeverResolvesToOwner_JWTPath — same invariant, driven
// through the JWT branch instead, per task-2.md's requirement that I2 be
// independently verified on both paths rather than assumed from one.
func TestI2_BearerNeverResolvesToOwner_JWTPath(t *testing.T) {
	users := newFakeUserRepo()
	keys := newFakeAPIKeyRepo()
	owner := User{ID: "owner-1", Handle: "มายด์", Role: "owner", Active: true, SSOSubject: "sso|owner"}
	users.put(owner)

	jwtV := &fakeJWTVerifier{sub: "sso|owner"}
	svc := newTestService(users, keys, jwtV, time.Now())
	_, err := svc.ResolveActor(context.Background(), "Bearer some.owner.jwt")
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestResolveActor_InactiveUserRejected_APIKeyPath(t *testing.T) {
	users := newFakeUserRepo()
	keys := newFakeAPIKeyRepo()
	inactive := User{ID: "u1", Handle: "retired-agent", Role: "agent", Active: false}
	users.put(inactive)
	now := time.Now()
	putAPIKey(keys, "tpl_inactivekeyinact1", inactive.ID, now.Add(time.Hour), nil)

	svc := newTestService(users, keys, nil, now)
	_, err := svc.ResolveActor(context.Background(), "Bearer tpl_inactivekeyinact1")
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestResolveActor_InactiveUserRejected_JWTPath(t *testing.T) {
	users := newFakeUserRepo()
	keys := newFakeAPIKeyRepo()
	inactive := User{ID: "u1", Handle: "retired-agent", Role: "agent", Active: false, SSOSubject: "sso|retired"}
	users.put(inactive)

	jwtV := &fakeJWTVerifier{sub: "sso|retired"}
	svc := newTestService(users, keys, jwtV, time.Now())
	_, err := svc.ResolveActor(context.Background(), "Bearer some.jwt")
	assert.ErrorIs(t, err, ErrUnauthorized)
}

// --- key issuance (cmd/issue-key's dependency) ------------------------

func TestIssueAPIKeyForHandle_CreatesNewUserWhenMissing(t *testing.T) {
	users := newFakeUserRepo()
	keys := newFakeAPIKeyRepo()
	svc := newTestService(users, keys, nil, time.Now())

	result, err := svc.IssueAPIKeyForHandle(context.Background(), "brand-new-agent")
	require.NoError(t, err)
	assert.True(t, users.createCalled)
	assert.Equal(t, "agent", result.User.Role)
	assert.True(t, strings.HasPrefix(result.RawKey, "tpl_"))
	assert.Equal(t, result.RawKey[:12], result.APIKey.KeyPrefix)
	assert.Equal(t, HashAPIKey(result.RawKey), result.APIKey.KeyHash)
	assert.True(t, keys.createCalled)
}

func TestIssueAPIKeyForHandle_ReusesExistingUser(t *testing.T) {
	users := newFakeUserRepo()
	keys := newFakeAPIKeyRepo()
	existing := User{ID: "existing-1", Handle: "already-here", Role: "agent", Active: true}
	users.put(existing)

	svc := newTestService(users, keys, nil, time.Now())
	result, err := svc.IssueAPIKeyForHandle(context.Background(), "already-here")
	require.NoError(t, err)
	assert.False(t, users.createCalled)
	assert.Equal(t, existing.ID, result.User.ID)
}

// --- I13: rotate issues the new key before disabling the old one(s) -------

// TestI13_RotateIssuesNewKeyBeforeDisablingOld is I13's ordering proof.
// End-state assertions alone ("the new key is live and the old key is
// revoked, once Rotate returns") can't distinguish issue-then-disable from
// my-task's actual disable-then-issue — both orderings reach the exact
// same end state. This test instead uses Service's
// rotateAfterNewKeyIssued hook as an observable sequencing point *inside*
// the call: at the moment the hook fires (right after the new key is
// stored, before DisableOtherAPIKeys has run), it independently queries
// the fake repo and asserts both that the new key is already resolvable
// by hash AND that every pre-existing key is still live. Only the real
// issue-first ordering can make both of those true at that exact point —
// disable-then-issue would show the old key(s) already revoked here.
func TestI13_RotateIssuesNewKeyBeforeDisablingOld(t *testing.T) {
	users := newFakeUserRepo()
	keys := newFakeAPIKeyRepo()
	agent := User{ID: "u1", Handle: "agent-a", Role: "agent", Active: true}
	users.put(agent)
	now := time.Now()

	// Two pre-existing live keys — "old one(s)", plural, per I13's own
	// wording — so the test also proves DisableOtherAPIKeys doesn't stop
	// after the first.
	putAPIKey(keys, "tpl_oldoneoldoneoldo1", agent.ID, now.Add(time.Hour), nil)
	putAPIKey(keys, "tpl_oldtwooldtwooldt2", agent.ID, now.Add(time.Hour), nil)

	svc := newTestService(users, keys, nil, now)

	var (
		hookFired           bool
		newKeyQueryable     bool
		allOldKeysStillLive bool
	)
	svc.rotateAfterNewKeyIssued = func(newKey APIKey) {
		hookFired = true

		got, err := keys.GetAPIKeyByHash(context.Background(), newKey.KeyHash)
		newKeyQueryable = err == nil && got.ID == newKey.ID

		allOldKeysStillLive = true
		for _, raw := range []string{"tpl_oldoneoldoneoldo1", "tpl_oldtwooldtwooldt2"} {
			old, err := keys.GetAPIKeyByHash(context.Background(), HashAPIKey(raw))
			if err != nil || old.RevokedAt != nil {
				allOldKeysStillLive = false
			}
		}
	}

	result, err := svc.Rotate(context.Background(), "agent-a")
	require.NoError(t, err)

	require.True(t, hookFired, "rotateAfterNewKeyIssued must fire — otherwise this test proves nothing about ordering")
	assert.True(t, newKeyQueryable, "the new key must already exist and be queryable at the mid-call observation point")
	assert.True(t, allOldKeysStillLive, "every pre-existing key must still be live at the mid-call observation point — proves issue-then-disable, not disable-then-issue")

	// End state, checked separately from the ordering proof above: both
	// old keys are now disabled, the new key is live, and nothing else got
	// touched.
	assert.Equal(t, 2, result.RevokedCount)
	for _, raw := range []string{"tpl_oldoneoldoneoldo1", "tpl_oldtwooldtwooldt2"} {
		old, err := keys.GetAPIKeyByHash(context.Background(), HashAPIKey(raw))
		require.NoError(t, err)
		assert.NotNil(t, old.RevokedAt, "old key must be disabled once Rotate returns")
	}
	newAfter, err := keys.GetAPIKeyByHash(context.Background(), result.APIKey.KeyHash)
	require.NoError(t, err)
	assert.Nil(t, newAfter.RevokedAt, "the freshly-rotated key must not itself be disabled")
	assert.True(t, strings.HasPrefix(result.RawKey, "tpl_"))
}

// TestRotate_UnknownHandleFails — Rotate never creates a users row the way
// IssueAPIKeyForHandle does; a handle with no existing user has nothing to
// rotate.
func TestRotate_UnknownHandleFails(t *testing.T) {
	users := newFakeUserRepo()
	keys := newFakeAPIKeyRepo()
	svc := newTestService(users, keys, nil, time.Now())

	_, err := svc.Rotate(context.Background(), "never-issued-to")
	assert.ErrorIs(t, err, ErrNotFound)
	assert.False(t, keys.createCalled, "must not issue a key for a handle that was never provisioned")
}
