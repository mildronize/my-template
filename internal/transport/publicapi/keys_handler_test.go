package publicapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mildronize/my-template/internal/api"
	"github.com/mildronize/my-template/internal/identity"
)

// newKeysIntegrationRouter builds a full /api/v1 stack — RejectActorFields,
// RequireActor, then the openapi.yaml request validator — against a real
// temp-file SQLite database (not a mock), for GET /keys and
// DELETE /keys/:id integration tests. It mounts KeysServer's two routes
// directly on the gin group rather than going through the full generated
// api.RegisterHandlers (which would require these keys-only tests to also
// implement internal/domain/todo's ServerInterface methods —
// todo_handler_test.go already covers the composite-registration path end
// to end). The openapi.yaml request validator matches requests against
// the spec's own path templates, independent of gin's route table, so
// this still exercises the same validated-shape guarantee (Done-when 7)
// the composite wiring gives /todos and /me.
func newKeysIntegrationRouter(t *testing.T) (*gin.Engine, *sql.DB) {
	t.Helper()
	conn := newTestDB(t)

	repo := identity.NewRepo(conn)
	svc := identity.NewService(repo, repo, nil, nil)
	keysServer := NewKeysServer(svc)

	validator, err := api.RequestValidator()
	require.NoError(t, err)

	router := gin.New()
	group := router.Group("/api/v1")
	group.Use(RejectActorFields(), RequireActor(svc), validator)
	group.GET("/keys", keysServer.ListKeys)
	group.DELETE("/keys/:id", func(c *gin.Context) { keysServer.RevokeKey(c, c.Param("id")) })

	return router, conn
}

func doKeysRequest(t *testing.T, router *gin.Engine, method, path, rawKey string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(nil))
	if rawKey != "" {
		req.Header.Set("Authorization", "Bearer "+rawKey)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeKeysError(t *testing.T, rec *httptest.ResponseRecorder) api.Error {
	t.Helper()
	var got api.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got
}

// TestHandler_KeysListAndRevokeRoundTrip walks list -> revoke -> list
// (revoked key gone) for a single agent, against a real SQLite file.
func TestHandler_KeysListAndRevokeRoundTrip(t *testing.T) {
	router, conn := newKeysIntegrationRouter(t)
	ownerID, rawKey := createAgentWithKey(t, conn, "agent-a")

	// A second live key for the same owner, created directly via the repo
	// (issuance is CLI-only — API.md — so tests seed keys this way rather
	// than through an HTTP POST that deliberately doesn't exist).
	repo := identity.NewRepo(conn)
	second, err := repo.CreateAPIKey(context.Background(), ownerID, identity.HashAPIKey("tpl_second"), "tpl_secondkey", time.Now().Add(2*time.Hour))
	require.NoError(t, err)

	listRec := doKeysRequest(t, router, http.MethodGet, "/api/v1/keys", rawKey)
	require.Equal(t, http.StatusOK, listRec.Code)
	var list api.ApiKeyList
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &list))
	require.Len(t, list.Keys, 2, "both the seeded key and the freshly created one must be listed")

	revokeRec := doKeysRequest(t, router, http.MethodDelete, "/api/v1/keys/"+second.ID, rawKey)
	require.Equal(t, http.StatusNoContent, revokeRec.Code)

	listAfterRec := doKeysRequest(t, router, http.MethodGet, "/api/v1/keys", rawKey)
	require.Equal(t, http.StatusOK, listAfterRec.Code)
	var listAfter api.ApiKeyList
	require.NoError(t, json.Unmarshal(listAfterRec.Body.Bytes(), &listAfter))
	require.Len(t, listAfter.Keys, 1, "the revoked key must no longer be listed")
	assert.NotEqual(t, second.ID, listAfter.Keys[0].Id)
}

// TestI3_HandlerOwnershipScoping_ReturnsNotFoundNotForbidden_Keys — I3, at
// the HTTP layer, applied to keys instead of todos (a second resource, not
// a duplicate of todo_handler_test.go's version of this test): a key
// belonging to a different owner returns 404 (not_found), the exact same
// response as an id that never existed. Never 403 — that would confirm
// the row exists.
func TestI3_HandlerOwnershipScoping_ReturnsNotFoundNotForbidden_Keys(t *testing.T) {
	router, conn := newKeysIntegrationRouter(t)
	ownerID, ownerKey := createAgentWithKey(t, conn, "owner")
	_, otherKey := createAgentWithKey(t, conn, "someone-else")

	repo := identity.NewRepo(conn)
	theirs, err := repo.CreateAPIKey(context.Background(), ownerID, identity.HashAPIKey("tpl_owners-secret"), "tpl_ownerssecr", time.Now().Add(time.Hour))
	require.NoError(t, err)

	unknownID := "00000000-0000-0000-0000-000000000000"

	cases := []struct {
		name string
		id   string
	}{
		{"DELETE another owner's key id", theirs.ID},
		{"DELETE an id that never existed", unknownID},
	}

	var firstErrorBody string
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doKeysRequest(t, router, http.MethodDelete, "/api/v1/keys/"+tc.id, otherKey)
			require.Equalf(t, http.StatusNotFound, rec.Code, "case %q", tc.name)

			errBody := decodeKeysError(t, rec)
			assert.Equal(t, "not_found", errBody.Error.Code)

			if firstErrorBody == "" {
				firstErrorBody = rec.Body.String()
			} else {
				assert.Equalf(t, firstErrorBody, rec.Body.String(),
					"case %q: a wrong-owner key id and a never-existed id must be indistinguishable (I3)", tc.name)
			}
		})
	}

	// The row itself is untouched — otherKey's attempt above did not
	// revoke it. Authenticate as the actual owner (ownerKey) to confirm.
	listRec := doKeysRequest(t, router, http.MethodGet, "/api/v1/keys", ownerKey)
	require.Equal(t, http.StatusOK, listRec.Code)
	var list api.ApiKeyList
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &list))
	found := false
	for _, k := range list.Keys {
		if k.Id == theirs.ID {
			found = true
		}
	}
	assert.True(t, found, "owner's key must still be listed — the other owner's revoke attempt must not have succeeded")
}

// TestI9_ListKeys_ExpiredButUnrevokedKeyStillListed_RevokedKeyExcluded —
// API.md's `GET /api/v1/keys`: an expired-but-unrevoked key still shows up
// in the list so the caller can see it needs rotating, while a revoked key
// never does. This is deliberately about the LIST endpoint's behavior, not
// a duplicate of internal/identity/service_test.go's I9 coverage
// (TestI9_ExpiredAPIKeyFailsAuth / TestI9_RevokedAPIKeyFailsAuth), which
// proves the opposite-looking fact that the very same expired/revoked
// key fails *authentication* — two different checks (see
// identity.Service.ListAPIKeys's doc comment): this test never
// authenticates with the expired key, it authenticates with a separate
// live key and lists.
func TestI9_ListKeys_ExpiredButUnrevokedKeyStillListed_RevokedKeyExcluded(t *testing.T) {
	router, conn := newKeysIntegrationRouter(t)
	ownerID, liveKey := createAgentWithKey(t, conn, "owner")

	repo := identity.NewRepo(conn)
	ctx := context.Background()

	expired, err := repo.CreateAPIKey(ctx, ownerID, identity.HashAPIKey("tpl_expired-not-revoked"), "tpl_expirednot", time.Now().Add(-time.Hour))
	require.NoError(t, err)

	toRevoke, err := repo.CreateAPIKey(ctx, ownerID, identity.HashAPIKey("tpl_to-be-revoked"), "tpl_toberevoke", time.Now().Add(time.Hour))
	require.NoError(t, err)
	_, err = repo.RevokeAPIKey(ctx, toRevoke.ID, ownerID)
	require.NoError(t, err)

	rec := doKeysRequest(t, router, http.MethodGet, "/api/v1/keys", liveKey)
	require.Equal(t, http.StatusOK, rec.Code)

	var list api.ApiKeyList
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))

	ids := make(map[string]bool, len(list.Keys))
	for _, k := range list.Keys {
		ids[k.Id] = true
	}
	assert.True(t, ids[expired.ID], "an expired-but-unrevoked key must still be listed (API.md — deliberate UX choice, not a bug)")
	assert.False(t, ids[toRevoke.ID], "a revoked key must never be listed")
}

func TestHandler_ListKeys_Unauthenticated_Returns401(t *testing.T) {
	router, _ := newKeysIntegrationRouter(t)

	rec := doKeysRequest(t, router, http.MethodGet, "/api/v1/keys", "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandler_RevokeKey_Unauthenticated_Returns401(t *testing.T) {
	router, _ := newKeysIntegrationRouter(t)

	rec := doKeysRequest(t, router, http.MethodDelete, "/api/v1/keys/some-id", "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
