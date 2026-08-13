package bff

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mildronize/my-template/internal/bffapi"
	"github.com/mildronize/my-template/internal/domain/todo"
	"github.com/mildronize/my-template/internal/identity"
)

// newBFFRouterForTwoOwnersWithKeys is this file's shared setup: two
// distinct, freshly seeded owners sharing one DB/router, each with one
// live (non-revoked) API key issued via internal/identity's own repo
// (CreateAPIKey — a real key_hash write, not a raw INSERT, since this
// fixture exercises internal/identity.Service.ListAPIKeys/RevokeAPIKey
// directly), plus a session cookie for each owner.
func newBFFRouterForTwoOwnersWithKeys(t *testing.T) (router *gin.Engine, sessionA, sessionB string, keyIDA, keyIDB string) {
	t.Helper()
	conn := newTestDB(t)
	repo := identity.NewRepo(conn)
	identitySvc := identity.NewService(repo, repo, nil, nil)
	todoSvc := todo.NewService(todo.NewRepo(conn))

	ownerA := seedUser(t, conn, "owner", "owner-a-sub-"+t.Name(), true)
	ownerB := seedUser(t, conn, "owner", "owner-b-sub-"+t.Name(), true)

	keyA, err := repo.CreateAPIKey(t.Context(), ownerA.ID, identity.HashAPIKey("tpl_ownerAkey0123456789abcdef0123456789ab"), "tpl_ownerA01", time.Now().Add(time.Hour))
	require.NoError(t, err)
	keyB, err := repo.CreateAPIKey(t.Context(), ownerB.ID, identity.HashAPIKey("tpl_ownerBkey0123456789abcdef0123456789ab"), "tpl_ownerB01", time.Now().Add(time.Hour))
	require.NoError(t, err)

	idp := newFakeIDP(t, "test-client")
	cfg := idp.testConfig()
	signer := NewSigner([]byte(cfg.SessionSecret))
	router = newTestRouter(cfg, signer, newIDVerifier(t, idp), repo, todoSvc, identitySvc)

	sessionA, err = signer.NewSessionCookie(ownerA.ID)
	require.NoError(t, err)
	sessionB, err = signer.NewSessionCookie(ownerB.ID)
	require.NoError(t, err)

	return router, sessionA, sessionB, keyA.ID, keyB.ID
}

// TestBFFHandler_ListKeys_ReturnsOwnersOwnKeys proves GET /api/bff/keys
// reuses internal/identity.Service.ListAPIKeys directly, scoped to the
// session owner — same shape and reasoning as
// internal/transport/publicapi's own ListKeys handler, session-resolved
// instead of Bearer-resolved. Each owner sees only their own key, never
// the other's.
func TestBFFHandler_ListKeys_ReturnsOwnersOwnKeys(t *testing.T) {
	router, sessionA, sessionB, keyIDA, keyIDB := newBFFRouterForTwoOwnersWithKeys(t)

	recA := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/keys", sessionA, nil)
	require.Equal(t, http.StatusOK, recA.Code)
	var listA bffapi.ApiKeyList
	require.NoError(t, json.Unmarshal(recA.Body.Bytes(), &listA))
	require.Len(t, listA.Keys, 1)
	assert.Equal(t, keyIDA, listA.Keys[0].Id)

	recB := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/keys", sessionB, nil)
	require.Equal(t, http.StatusOK, recB.Code)
	var listB bffapi.ApiKeyList
	require.NoError(t, json.Unmarshal(recB.Body.Bytes(), &listB))
	require.Len(t, listB.Keys, 1)
	assert.Equal(t, keyIDB, listB.Keys[0].Id)
}

// TestBFFHandler_RevokeKey_ThenListNoLongerShowsIt proves DELETE
// /api/bff/keys/{id} actually revokes (sets revoked_at) via
// internal/identity.Service.RevokeAPIKey, verified by reading the list
// back afterward rather than only trusting the 204.
func TestBFFHandler_RevokeKey_ThenListNoLongerShowsIt(t *testing.T) {
	router, sessionA, _, keyIDA, _ := newBFFRouterForTwoOwnersWithKeys(t)

	revokeRec := doBFFJSONRequest(t, router, http.MethodDelete, "/api/bff/keys/"+keyIDA, sessionA, nil)
	require.Equal(t, http.StatusNoContent, revokeRec.Code)

	listRec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/keys", sessionA, nil)
	require.Equal(t, http.StatusOK, listRec.Code)
	var list bffapi.ApiKeyList
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &list))
	assert.Empty(t, list.Keys, "a revoked key must no longer be listed (revoked_at IS NULL is ListAPIKeys' own filter)")
}

// TestI3_BFFHandlerOwnershipScoping_ReturnsNotFoundNotForbidden_Keys
// mirrors internal/transport/publicapi's own
// TestI3_HandlerOwnershipScoping_ReturnsNotFoundNotForbidden_Keys (I3):
// a second owner attempting to revoke the first owner's key id gets 404
// (not_found), never 403 — the row's continued existence (owner A's key
// is still live afterward) is asserted too, so this isn't just a status
// code check.
func TestI3_BFFHandlerOwnershipScoping_ReturnsNotFoundNotForbidden_Keys(t *testing.T) {
	router, sessionA, sessionB, keyIDA, _ := newBFFRouterForTwoOwnersWithKeys(t)

	rec := doBFFJSONRequest(t, router, http.MethodDelete, "/api/bff/keys/"+keyIDA, sessionB, nil)
	require.Equal(t, http.StatusNotFound, rec.Code, "must be 404 (not_found), never 403 (I3)")
	assert.NotEqual(t, http.StatusForbidden, rec.Code, "must never be 403 — that would confirm the key exists to a caller who doesn't own it (I3)")
	assert.Equal(t, "not_found", decodeBFFError(t, rec).Error.Code)

	// The key itself is untouched — owner B's failed attempt didn't
	// revoke it.
	listRec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/keys", sessionA, nil)
	require.Equal(t, http.StatusOK, listRec.Code)
	var list bffapi.ApiKeyList
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &list))
	require.Len(t, list.Keys, 1, "owner A's key must still be live — owner B's 404'd attempt must not have revoked it")
	assert.Equal(t, keyIDA, list.Keys[0].Id)
}
