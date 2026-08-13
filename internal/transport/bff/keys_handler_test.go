package bff

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mildronize/my-template/internal/api"
	"github.com/mildronize/my-template/internal/bffapi"
	"github.com/mildronize/my-template/internal/identity"
	"github.com/mildronize/my-template/internal/transport/publicapi"
)

// This file was rewritten wholesale for milestone-4/task-6 (I21), not
// patched in place. The previous version's shared fixture,
// newBFFRouterForTwoOwnersWithKeys, seeded two role='owner' users and
// called repo.CreateAPIKey on them directly — bypassing cmd/issue-key, the
// only real code path that ever produces a role='agent' user with a key.
// Production can never reach the state that fixture created (no key is
// ever issued to a role='owner' user — I2, cmd/issue-key.md/service.go's
// IssueAPIKeyForHandle), so the isolation assertion it built ("each owner
// sees only their own key") was real and green but proved nothing about
// the actual system: GET /api/bff/keys was scoped to the session owner's
// own user_id, a set that structurally could never be non-empty, which is
// exactly why the settings page showed every real deployment an empty
// list (GOAL.md's survey finding). I21 replaces that endpoint's semantics
// (every role='agent' user's non-revoked keys, not one user_id's own) and
// every fixture below seeds its agent identities through
// identity.Service.IssueAPIKeyForHandle — the exact method
// cmd/issue-key/main.go's own run() calls (line 123) — never a direct
// repo.CreateUser(..., "agent", ...) insert standing in for it.

// newAgentPublicAPIRouterForKeys builds a bare /api/v1 stack mounting only
// publicapi.KeysServer.ListKeys — the real Bearer-authenticated surface an
// agent actually calls in production, not a service-layer shortcut. Mirrors
// activity_handler_test.go's own newAgentPublicAPIRouter (same package,
// same "mount only the routes this file's tests need directly" pattern,
// same reasoning: RequireActor + the real openapi.yaml request validator,
// the same middleware chain production traffic hits).
func newAgentPublicAPIRouterForKeys(t *testing.T, identitySvc *identity.Service) *gin.Engine {
	t.Helper()
	validator, err := api.RequestValidator()
	require.NoError(t, err)

	router := gin.New()
	group := router.Group("/api/v1")
	group.Use(publicapi.RejectActorFields(), publicapi.RequireActor(identitySvc), validator)
	keysServer := publicapi.NewKeysServer(identitySvc)
	group.GET("/keys", keysServer.ListKeys)
	return router
}

// TestBFFHandler_ListKeys_ReturnsEveryAgentsKeys is I21's own positive
// proof for GET /api/bff/keys: two real agent identities, each issued a
// key through identity.Service.IssueAPIKeyForHandle (cmd/issue-key's own
// path), both show up in a single owner session's listing — not "the
// session owner's own keys" (there are none; owners never hold keys, I2),
// every role='agent' user's non-revoked keys.
//
// This test is deliberately attacked in the builder report (task-6): the
// query is temporarily reverted to the old, milestone-2/3-era
// user_id-scoped semantics (the exact defect I21 exists to correct) and
// this test is confirmed to go red — empty list, not just wrong count —
// before being reverted.
func TestBFFHandler_ListKeys_ReturnsEveryAgentsKeys(t *testing.T) {
	router, ownerSession, _, _, identitySvc, _ := newBFFRouterForOwnerSharedDB(t)

	issuedA, err := identitySvc.IssueAPIKeyForHandle(t.Context(), "agent-alpha")
	require.NoError(t, err)
	require.Equal(t, "agent", issuedA.User.Role, "IssueAPIKeyForHandle must produce a real role=agent user, the only path cmd/issue-key ever takes")
	issuedB, err := identitySvc.IssueAPIKeyForHandle(t.Context(), "agent-beta")
	require.NoError(t, err)
	require.Equal(t, "agent", issuedB.User.Role)

	rec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/keys", ownerSession, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var list bffapi.ApiKeyList
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))

	ids := make(map[string]bool, len(list.Keys))
	for _, k := range list.Keys {
		ids[k.Id] = true
	}
	assert.Truef(t, ids[issuedA.APIKey.ID], "owner's key listing must include agent-alpha's key — got %+v", list.Keys)
	assert.Truef(t, ids[issuedB.APIKey.ID], "owner's key listing must include agent-beta's key — got %+v", list.Keys)
	assert.Len(t, list.Keys, 2, "exactly the two agent keys issued in this test, nothing else")
}

// TestBFFHandler_RevokeKey_AnyAgentsKey_ThenListNoLongerShowsIt proves the
// owner-facing DELETE can revoke an agent's key it never "owns" in the old
// user_id sense — I21's other half — verified by reading the list back
// afterward rather than only trusting the 204.
func TestBFFHandler_RevokeKey_AnyAgentsKey_ThenListNoLongerShowsIt(t *testing.T) {
	router, ownerSession, _, _, identitySvc, _ := newBFFRouterForOwnerSharedDB(t)

	issued, err := identitySvc.IssueAPIKeyForHandle(t.Context(), "agent-to-revoke")
	require.NoError(t, err)

	revokeRec := doBFFJSONRequest(t, router, http.MethodDelete, "/api/bff/keys/"+issued.APIKey.ID, ownerSession, nil)
	require.Equal(t, http.StatusNoContent, revokeRec.Code, "the owner must be able to revoke an agent's key it never issued itself")

	listRec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/keys", ownerSession, nil)
	require.Equal(t, http.StatusOK, listRec.Code)
	var list bffapi.ApiKeyList
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &list))
	for _, k := range list.Keys {
		assert.NotEqual(t, issued.APIKey.ID, k.Id, "a revoked key must no longer be listed (revoked_at IS NULL is ListAllAgentAPIKeys' own filter)")
	}
}

// TestBFFHandler_RevokeKey_UnknownID_ReturnsNotFound: an id that never
// existed returns not_found, never a 500 — basic error-handling coverage
// for the rewritten handler, distinct from the old (now-removed) I3
// two-owners test this file used to carry. I3's "absence, not permission"
// ownership-scoping no longer applies to this endpoint pair on purpose
// (INVARIANTS.md's I21 text, GOAL.md's I3-scope-correction note) — the
// owner is deliberately allowed to see and revoke every agent's key, so
// there is no "wrong owner" case left to distinguish from "never existed."
func TestBFFHandler_RevokeKey_UnknownID_ReturnsNotFound(t *testing.T) {
	router, ownerSession, _ := newBFFRouterForOwner(t)

	rec := doBFFJSONRequest(t, router, http.MethodDelete, "/api/bff/keys/00000000-0000-0000-0000-000000000000", ownerSession, nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "not_found", decodeBFFError(t, rec).Error.Code)
}

// TestBFFHandler_ListKeys_Unauthenticated_Returns401 and
// TestBFFHandler_RevokeKey_Unauthenticated_Returns401 mirror every other
// /api/bff endpoint's own unauthenticated-401 test (e.g.
// activity_handler_test.go's TestBFFHandler_ListActivity_Unauthenticated_
// Returns401) — a valid owner session is still required to reach either
// handler at all, even though neither is scoped to that session's own
// user_id anymore.
func TestBFFHandler_ListKeys_Unauthenticated_Returns401(t *testing.T) {
	router, _, _ := newBFFRouterForOwner(t)

	rec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/keys", "", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestBFFHandler_RevokeKey_Unauthenticated_Returns401(t *testing.T) {
	router, _, _ := newBFFRouterForOwner(t)

	rec := doBFFJSONRequest(t, router, http.MethodDelete, "/api/bff/keys/some-id", "", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TestDoneWhen11_RevocationActuallyStopsTheKey_BothHalves is GOAL.md's
// Done-when 11, task-6's own scope item, proven both ways, not just the
// post-revoke half:
//
//  1. A real agent key is issued through identity.Service.
//     IssueAPIKeyForHandle — the exact method cmd/issue-key's own run()
//     calls — never a direct repo insert.
//  2. That key authenticates successfully against the REAL public API
//     (GET /api/v1/keys, through publicapi.RequireActor's real middleware
//     chain, not a service-layer shortcut) BEFORE revocation — a genuine
//     2xx, so the negative half below means something. A test that only
//     checked the post-revoke 401 would be equally consistent with the key
//     never having worked at all (GOAL.md's own stated reasoning for this
//     item).
//  3. The owner revokes it through this task's new DELETE
//     /api/bff/keys/:id.
//  4. The SAME raw key is presented again against the real public API and
//     now fails with 401 — not a different key, not a different route.
//
// Attacked in the builder report (task-6): revocation is temporarily made
// a no-op (the DB write skipped) and this test is confirmed to catch that
// the key still authenticates after the "revoke" call, before the fix is
// reverted.
func TestDoneWhen11_RevocationActuallyStopsTheKey_BothHalves(t *testing.T) {
	bffRouter, ownerSession, _, _, identitySvc, _ := newBFFRouterForOwnerSharedDB(t)
	agentRouter := newAgentPublicAPIRouterForKeys(t, identitySvc)

	// 1. Issue a real agent key through the exact path cmd/issue-key's own
	// run() calls (cmd/issue-key/main.go:123).
	issued, err := identitySvc.IssueAPIKeyForHandle(t.Context(), "revocation-both-halves-agent")
	require.NoError(t, err)
	require.Equal(t, "agent", issued.User.Role, "IssueAPIKeyForHandle must produce a real role=agent user, the only path cmd/issue-key ever takes")
	rawKey := issued.RawKey
	require.NotEmpty(t, rawKey)

	// 2. Positive half FIRST: a genuine 2xx against the real public API,
	// before any revocation — proves the key actually worked.
	beforeRec := doAgentAPIRequest(t, agentRouter, http.MethodGet, "/api/v1/keys", rawKey, nil)
	require.Equal(t, http.StatusOK, beforeRec.Code, "the freshly issued key must authenticate successfully against the real /api/v1 surface before revocation")
	var beforeList api.ApiKeyList
	require.NoError(t, json.Unmarshal(beforeRec.Body.Bytes(), &beforeList))
	require.Len(t, beforeList.Keys, 1, "the agent's own self-scoped listing must show exactly the one key just issued")
	assert.Equal(t, issued.APIKey.ID, beforeList.Keys[0].Id)

	// 3. The owner revokes it through the new owner-facing BFF endpoint.
	revokeRec := doBFFJSONRequest(t, bffRouter, http.MethodDelete, "/api/bff/keys/"+issued.APIKey.ID, ownerSession, nil)
	require.Equal(t, http.StatusNoContent, revokeRec.Code, "the owner must be able to revoke the agent's key via the new I21 endpoint")

	// 4. Negative half: the SAME raw key now fails authentication against
	// the same real public API surface.
	afterRec := doAgentAPIRequest(t, agentRouter, http.MethodGet, "/api/v1/keys", rawKey, nil)
	require.Equal(t, http.StatusUnauthorized, afterRec.Code, "the same key must fail authentication after revocation — this is Done-when 11's whole point")
}
