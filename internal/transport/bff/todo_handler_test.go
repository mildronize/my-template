package bff

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mildronize/my-template/internal/bffapi"
	"github.com/mildronize/my-template/internal/domain/todo"
	"github.com/mildronize/my-template/internal/identity"
)

// decodeBFFTodo decodes a bffapi.Todo-shaped response body — todo-specific
// (unlike decodeBFFError, bff_testutil_test.go), mirrors
// internal/transport/publicapi/todo_handler_test.go's own decodeTodo.
func decodeBFFTodo(t *testing.T, rec *httptest.ResponseRecorder) bffapi.Todo {
	t.Helper()
	var got bffapi.Todo
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got
}

// newBFFRouterForOwner is this file's single-owner setup: a fresh test
// DB, a freshly seeded owner, a live signed session cookie for that owner
// (session.go's Signer.NewSessionCookie, called directly — the same
// session-seeding shortcut milestone-2's own Done-when-9 test established
// (its view_handler_test.go, removed by milestone-3/task-3 once the SPA
// replaced what it rendered) and .chief/milestone-3/_plan/_todo.md's
// task-2 spec points at reusing, "no need to drive it through /callback"),
// and the full /api/bff router
// (real middleware chain: RejectActorFields, RequireJSONSession,
// bff-openapi.yaml's request validator, then the composed
// ServerInterface — newTestRouter, bff_testutil_test.go).
func newBFFRouterForOwner(t *testing.T) (router *gin.Engine, sessionValue string, owner identity.User) {
	t.Helper()
	conn := newTestDB(t)
	repo := identity.NewRepo(conn)
	identitySvc := identity.NewService(repo, repo, nil, nil)
	todoSvc := todo.NewService(todo.NewRepo(conn))

	owner = seedUser(t, conn, "owner", "owner-sub-"+t.Name(), true)

	idp := newFakeIDP(t, "test-client")
	cfg := idp.testConfig()
	signer := NewSigner([]byte(cfg.SessionSecret))
	router = newTestRouter(cfg, signer, newIDVerifier(t, idp), repo, todoSvc, identitySvc)

	var err error
	sessionValue, err = signer.NewSessionCookie(owner.ID)
	require.NoError(t, err)

	return router, sessionValue, owner
}

// newBFFRouterForTwoOwners builds one /api/bff router shared by two
// distinct, freshly seeded owners against one DB, returning a live
// session cookie for each — the fixture TestI3_... below needs (one
// router/DB, two owners), which newBFFRouterForOwner's single-owner shape
// doesn't provide.
func newBFFRouterForTwoOwners(t *testing.T) (router *gin.Engine, ownerASession, ownerBSession string, ownerA, ownerB identity.User) {
	t.Helper()
	conn := newTestDB(t)
	repo := identity.NewRepo(conn)
	identitySvc := identity.NewService(repo, repo, nil, nil)
	todoSvc := todo.NewService(todo.NewRepo(conn))

	ownerA = seedUser(t, conn, "owner", "owner-a-sub-"+t.Name(), true)
	ownerB = seedUser(t, conn, "owner", "owner-b-sub-"+t.Name(), true)

	idp := newFakeIDP(t, "test-client")
	cfg := idp.testConfig()
	signer := NewSigner([]byte(cfg.SessionSecret))
	router = newTestRouter(cfg, signer, newIDVerifier(t, idp), repo, todoSvc, identitySvc)

	var err error
	ownerASession, err = signer.NewSessionCookie(ownerA.ID)
	require.NoError(t, err)
	ownerBSession, err = signer.NewSessionCookie(ownerB.ID)
	require.NoError(t, err)

	return router, ownerASession, ownerBSession, ownerA, ownerB
}

// TestBFFHandler_FullCRUDRoundTrip_RealSessionCookie is
// milestone-3/_goal/GOAL.md Done-when 2 and this task's own verification
// item: create -> list -> get -> update -> delete through /api/bff/todos
// with a real (signed) session cookie, not a unit-injected actor —
// asserting each step actually persisted by reading it back afterward,
// not just that the write itself answered 200/201/204.
func TestBFFHandler_FullCRUDRoundTrip_RealSessionCookie(t *testing.T) {
	router, sessionValue, _ := newBFFRouterForOwner(t)

	// Create.
	createRec := doBFFJSONRequest(t, router, http.MethodPost, "/api/bff/todos", sessionValue, map[string]string{"title": "write the report"})
	require.Equal(t, http.StatusCreated, createRec.Code)
	created := decodeBFFTodo(t, createRec)
	assert.NotEmpty(t, created.Id)
	assert.Equal(t, "write the report", created.Title)
	assert.False(t, created.Done)

	// List — read back after write, not just trust the create response.
	listRec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/todos", sessionValue, nil)
	require.Equal(t, http.StatusOK, listRec.Code)
	var list bffapi.TodoList
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &list))
	require.Len(t, list.Todos, 1, "the created todo must actually be persisted, not just echoed back by the create response")
	assert.Equal(t, created.Id, list.Todos[0].Id)
	assert.Equal(t, "write the report", list.Todos[0].Title)

	// Get.
	getRec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/todos/"+created.Id, sessionValue, nil)
	require.Equal(t, http.StatusOK, getRec.Code)
	got := decodeBFFTodo(t, getRec)
	assert.Equal(t, created.Id, got.Id)
	assert.Equal(t, "write the report", got.Title)

	// Update (partial — only done).
	patchRec := doBFFJSONRequest(t, router, http.MethodPatch, "/api/bff/todos/"+created.Id, sessionValue, map[string]any{"done": true})
	require.Equal(t, http.StatusOK, patchRec.Code)
	patched := decodeBFFTodo(t, patchRec)
	assert.True(t, patched.Done)
	assert.Equal(t, "write the report", patched.Title, "an unset field in the PATCH body must survive untouched")

	// Read back after the update, independently of the patch response.
	getAfterPatchRec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/todos/"+created.Id, sessionValue, nil)
	require.Equal(t, http.StatusOK, getAfterPatchRec.Code)
	assert.True(t, decodeBFFTodo(t, getAfterPatchRec).Done, "the update must actually be persisted, not just echoed back by the patch response")

	// Delete.
	deleteRec := doBFFJSONRequest(t, router, http.MethodDelete, "/api/bff/todos/"+created.Id, sessionValue, nil)
	require.Equal(t, http.StatusNoContent, deleteRec.Code)

	// Read back after the delete: gone for real, not just a 204 that
	// didn't actually remove the row.
	getAfterDeleteRec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/todos/"+created.Id, sessionValue, nil)
	assert.Equal(t, http.StatusNotFound, getAfterDeleteRec.Code)
}

// TestI3_BFFHandlerOwnershipScoping_ReturnsNotFoundNotForbidden is the
// first BFF-layer I3 test (milestone-3/_goal/GOAL.md Done-when 3,
// _contract/API.md's "This is the first BFF-layer I3 check" note):
// milestone-2's own TestI3_... coverage only ever existed at the
// publicapi layer (internal/transport/publicapi/todo_handler_test.go's
// TestI3_HandlerOwnershipScoping_ReturnsNotFoundNotForbidden), so this is
// what makes the "per-module, not per-layer" granularity gap no longer
// hypothetical for todo. Mirrors that test's naming convention and shape,
// against /api/bff instead of /api/v1, session-cookie-authenticated
// instead of Bearer-authenticated.
//
// Both halves matter, independently asserted, exactly as the task spec
// calls for: the response must BE 404 (not_found) — require.Equal below —
// AND it must NOT be 403 — the explicit assert.NotEqual against 403 makes
// that second half a real assertion rather than something merely implied
// by the first one passing (a test that only checked "is 404" wouldn't
// separately catch a hypothetical future regression that returns, say,
// 403 alongside a "not_found"-labeled body by mistake).
func TestI3_BFFHandlerOwnershipScoping_ReturnsNotFoundNotForbidden(t *testing.T) {
	router, ownerASession, otherOwnerSession, _, otherOwner := newBFFRouterForTwoOwners(t)
	_ = otherOwner

	createRec := doBFFJSONRequest(t, router, http.MethodPost, "/api/bff/todos", ownerASession, map[string]string{"title": "owner A's private todo"})
	require.Equal(t, http.StatusCreated, createRec.Code)
	theirs := decodeBFFTodo(t, createRec)

	unknownID := "00000000-0000-0000-0000-000000000000"

	cases := []struct {
		name   string
		method string
		id     string
	}{
		{"GET another owner's id", http.MethodGet, theirs.Id},
		{"GET an id that never existed", http.MethodGet, unknownID},
		{"PATCH another owner's id", http.MethodPatch, theirs.Id},
		{"PATCH an id that never existed", http.MethodPatch, unknownID},
		{"DELETE another owner's id", http.MethodDelete, theirs.Id},
		{"DELETE an id that never existed", http.MethodDelete, unknownID},
	}

	var firstErrorBody string
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body any
			if tc.method == http.MethodPatch {
				body = map[string]any{"done": true}
			}
			rec := doBFFJSONRequest(t, router, tc.method, "/api/bff/todos/"+tc.id, otherOwnerSession, body)

			require.Equalf(t, http.StatusNotFound, rec.Code, "case %q: must be 404 (not_found), never 403 (I3 — absence, not permission)", tc.name)
			assert.NotEqualf(t, http.StatusForbidden, rec.Code, "case %q: must never be 403 — that would confirm the row exists to a caller who doesn't own it (I3)", tc.name)

			errBody := decodeBFFError(t, rec)
			assert.Equal(t, "not_found", errBody.Error.Code)

			if firstErrorBody == "" {
				firstErrorBody = rec.Body.String()
			} else {
				assert.Equalf(t, firstErrorBody, rec.Body.String(),
					"case %q: a wrong-owner id and a never-existed id must be indistinguishable (I3)", tc.name)
			}
		})
	}

	// The row itself is untouched — none of the other owner's attempts
	// above actually mutated or deleted it.
	stillTheirs := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/todos/"+theirs.Id, ownerASession, nil)
	require.Equal(t, http.StatusOK, stillTheirs.Code)
	assert.Equal(t, "owner A's private todo", decodeBFFTodo(t, stillTheirs).Title)
}

// TestBFFHandler_ListTodos_Unauthenticated_Returns401NotRedirect proves
// _contract/API.md's explicit behavior change on this surface: unlike
// RequireSession (middleware.go — the html/template view's own
// session-gate, redirect-to-/login shaped, now unused in production code
// since milestone-3/task-3 removed its one caller, view_handler.go), a
// missing session on /api/bff answers 401 JSON, never a 302 redirect —
// "a fetch call can't follow a redirect the way a browser navigation
// does."
func TestBFFHandler_ListTodos_Unauthenticated_Returns401NotRedirect(t *testing.T) {
	router, _, _ := newBFFRouterForOwner(t)

	rec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/todos", "", nil)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.NotEqual(t, http.StatusFound, rec.Code, "must never redirect on this JSON surface (_contract/API.md)")
	assert.Empty(t, rec.Header().Get("Location"), "must not set a redirect Location header either")
	assert.Equal(t, "unauthorized", decodeBFFError(t, rec).Error.Code)
}
