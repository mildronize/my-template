package todo

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

func init() {
	gin.SetMode(gin.TestMode)
}

// compositeServer mirrors cmd/server's apiServer — identity.MeServer
// contributes GetMe, *Server (this package's) contributes the todo CRUD
// methods — so these integration tests exercise the exact same
// generated-interface/openapi-validated wiring production uses, not a
// hand-rolled subset of it.
type compositeServer struct {
	identity.MeServer
	*Server
}

// newIntegrationRouter builds a full /api/v1 stack — RejectActorFields,
// RequireActor, the openapi.yaml request validator, then
// api.RegisterHandlers — against a real temp-file SQLite database (not a
// mock), for todo CRUD + ownership-scoping integration tests.
func newIntegrationRouter(t *testing.T) (*gin.Engine, *sql.DB) {
	t.Helper()
	conn := newTestDB(t)

	identityRepo := identity.NewRepo(conn)
	identitySvc := identity.NewService(identityRepo, identityRepo, nil, nil)

	todoSvc := NewService(NewRepo(conn))

	validator, err := api.RequestValidator()
	require.NoError(t, err)

	router := gin.New()
	group := router.Group("/api/v1")
	group.Use(identity.RejectActorFields(), identity.RequireActor(identitySvc), validator)
	api.RegisterHandlers(group, compositeServer{Server: NewServer(todoSvc)})

	return router, conn
}

// createAgentWithKey seeds a users row (role=agent) and a live api_keys
// row for it, returning the user's id and the raw key a test can present
// as `Authorization: Bearer <rawKey>`.
func createAgentWithKey(t *testing.T, conn *sql.DB, handle string) (userID, rawKey string) {
	t.Helper()
	ctx := context.Background()
	repo := identity.NewRepo(conn)

	user, err := repo.CreateUser(ctx, handle, "agent", nil)
	require.NoError(t, err)

	rawKey = "tpl_" + handle + "0123456789abcdef0123456789abcdef"
	hash := identity.HashAPIKey(rawKey)
	_, err = repo.CreateAPIKey(ctx, user.ID, hash, rawKey[:12], time.Now().Add(time.Hour))
	require.NoError(t, err)

	return user.ID, rawKey
}

func doJSONRequest(t *testing.T, router *gin.Engine, method, path, rawKey string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if rawKey != "" {
		req.Header.Set("Authorization", "Bearer "+rawKey)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeTodo(t *testing.T, rec *httptest.ResponseRecorder) api.Todo {
	t.Helper()
	var got api.Todo
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) api.Error {
	t.Helper()
	var got api.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got
}

// TestHandler_FullCRUDRoundTrip walks create -> list -> get -> patch ->
// delete for a single agent, against a real SQLite file.
func TestHandler_FullCRUDRoundTrip(t *testing.T) {
	router, conn := newIntegrationRouter(t)
	_, rawKey := createAgentWithKey(t, conn, "agent-a")

	// Create.
	createRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/todos", rawKey, map[string]string{"title": "write the report"})
	require.Equal(t, http.StatusCreated, createRec.Code)
	created := decodeTodo(t, createRec)
	assert.NotEmpty(t, created.Id)
	assert.Equal(t, "write the report", created.Title)
	assert.False(t, created.Done)

	// List.
	listRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/todos", rawKey, nil)
	require.Equal(t, http.StatusOK, listRec.Code)
	var list api.TodoList
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &list))
	require.Len(t, list.Todos, 1)
	assert.Equal(t, created.Id, list.Todos[0].Id)

	// Get.
	getRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/todos/"+created.Id, rawKey, nil)
	require.Equal(t, http.StatusOK, getRec.Code)
	assert.Equal(t, created.Id, decodeTodo(t, getRec).Id)

	// Patch (partial — only done).
	patchRec := doJSONRequest(t, router, http.MethodPatch, "/api/v1/todos/"+created.Id, rawKey, map[string]any{"done": true})
	require.Equal(t, http.StatusOK, patchRec.Code)
	patched := decodeTodo(t, patchRec)
	assert.True(t, patched.Done)
	assert.Equal(t, "write the report", patched.Title, "an unset field in the PATCH body must survive untouched")

	// Delete.
	deleteRec := doJSONRequest(t, router, http.MethodDelete, "/api/v1/todos/"+created.Id, rawKey, nil)
	require.Equal(t, http.StatusNoContent, deleteRec.Code)

	// Now gone.
	getAfterDeleteRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/todos/"+created.Id, rawKey, nil)
	assert.Equal(t, http.StatusNotFound, getAfterDeleteRec.Code)
}

// TestI3_HandlerOwnershipScoping_ReturnsNotFoundNotForbidden — I3, at the
// HTTP layer: a todo belonging to a different owner returns 404
// (not_found), the exact same response as an id that never existed, on
// GET/PATCH/DELETE alike. Never 403 — that would confirm the row exists.
func TestI3_HandlerOwnershipScoping_ReturnsNotFoundNotForbidden(t *testing.T) {
	router, conn := newIntegrationRouter(t)
	_, ownerKey := createAgentWithKey(t, conn, "owner")
	_, otherKey := createAgentWithKey(t, conn, "someone-else")

	createRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/todos", ownerKey, map[string]string{"title": "owner's private todo"})
	require.Equal(t, http.StatusCreated, createRec.Code)
	theirs := decodeTodo(t, createRec)

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

	var body any
	var firstErrorBody string
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body = nil
			if tc.method == http.MethodPatch {
				body = map[string]any{"done": true}
			}
			rec := doJSONRequest(t, router, tc.method, "/api/v1/todos/"+tc.id, otherKey, body)
			require.Equalf(t, http.StatusNotFound, rec.Code, "case %q", tc.name)

			errBody := decodeError(t, rec)
			assert.Equal(t, "not_found", errBody.Error.Code)

			if firstErrorBody == "" {
				firstErrorBody = rec.Body.String()
			} else {
				assert.Equalf(t, firstErrorBody, rec.Body.String(),
					"case %q: a wrong-owner id and a never-existed id must be indistinguishable (I3)", tc.name)
			}
		})
	}

	// The row itself is untouched — none of otherKey's attempts above
	// actually mutated or deleted it.
	stillTheirs := doJSONRequest(t, router, http.MethodGet, "/api/v1/todos/"+theirs.Id, ownerKey, nil)
	require.Equal(t, http.StatusOK, stillTheirs.Code)
	assert.Equal(t, "owner's private todo", decodeTodo(t, stillTheirs).Title)
}

func TestHandler_ListTodos_Unauthenticated_Returns401(t *testing.T) {
	router, _ := newIntegrationRouter(t)

	rec := doJSONRequest(t, router, http.MethodGet, "/api/v1/todos", "", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TestHandler_CreateTodo_MissingTitleRejectedByOpenAPIValidator — GOAL.md
// Done-when 7: a request violating openapi.yaml (missing required field)
// is rejected by gin-middleware's spec validation before it reaches
// handler code — proven here by checking the row was never created, not
// just that the status code was 400.
func TestHandler_CreateTodo_MissingTitleRejectedByOpenAPIValidator(t *testing.T) {
	router, conn := newIntegrationRouter(t)
	_, rawKey := createAgentWithKey(t, conn, "agent-a")

	rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/todos", rawKey, map[string]string{})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	listRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/todos", rawKey, nil)
	require.Equal(t, http.StatusOK, listRec.Code)
	var list api.TodoList
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &list))
	assert.Empty(t, list.Todos, "the invalid request must never have reached the handler")
}

// TestHandler_CreateTodo_TitleTooLongRejectedByOpenAPIValidator —
// DATA_MODEL.md's 1-200 char rule, enforced at the openapi.yaml layer.
func TestHandler_CreateTodo_TitleTooLongRejectedByOpenAPIValidator(t *testing.T) {
	router, conn := newIntegrationRouter(t)
	_, rawKey := createAgentWithKey(t, conn, "agent-a")

	tooLong := make([]byte, 201)
	for i := range tooLong {
		tooLong[i] = 'a'
	}

	rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/todos", rawKey, map[string]string{"title": string(tooLong)})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestHandler_Me_RunsThroughSameGeneratedInterfaceAsTodos — task-3:
// GET /api/v1/me is no longer a bespoke route; it's registered through
// the same api.RegisterHandlers call as the todo endpoints, so it's
// validated the same way as everything else.
func TestHandler_Me_RunsThroughSameGeneratedInterfaceAsTodos(t *testing.T) {
	router, conn := newIntegrationRouter(t)
	_, rawKey := createAgentWithKey(t, conn, "luna")

	rec := doJSONRequest(t, router, http.MethodGet, "/api/v1/me", rawKey, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"handle":"luna"`)
}
