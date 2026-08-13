package bff

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mildronize/my-template/internal/api"
	"github.com/mildronize/my-template/internal/bffapi"
	"github.com/mildronize/my-template/internal/domain/todo"
	"github.com/mildronize/my-template/internal/identity"
	"github.com/mildronize/my-template/internal/transport/publicapi"
)

// decodeBFFActivityFeed decodes a bffapi.ActivityFeed-shaped response body
// — GET /api/bff/activity's own success shape.
func decodeBFFActivityFeed(t *testing.T, rec *httptest.ResponseRecorder) bffapi.ActivityFeed {
	t.Helper()
	var got bffapi.ActivityFeed
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got
}

// newBFFRouterForOwnerSharedDB is newBFFRouterForOwner's own setup
// (bff_testutil_test.go), except it also returns the underlying *sql.DB,
// *identity.Service, and *todo.Service — this file's own tests need a
// second, Bearer-authenticated router (newAgentPublicAPIRouter, below)
// sharing the exact same database, so an agent acting through the real
// public API and an owner session reading through /api/bff observe the
// same rows.
func newBFFRouterForOwnerSharedDB(t *testing.T) (router *gin.Engine, sessionValue string, owner identity.User, conn *sql.DB, identitySvc *identity.Service, todoSvc *todo.Service) {
	t.Helper()
	conn = newTestDB(t)
	repo := identity.NewRepo(conn)
	identitySvc = identity.NewService(repo, repo, nil, nil)
	todoSvc = todo.NewService(todo.NewRepo(conn))

	owner = seedUser(t, conn, "owner", "owner-sub-"+t.Name(), true)

	idp := newFakeIDP(t, "test-client")
	cfg := idp.testConfig()
	signer := NewSigner([]byte(cfg.SessionSecret))
	router = newTestRouter(cfg, signer, newIDVerifier(t, idp), repo, todoSvc, identitySvc)

	var err error
	sessionValue, err = signer.NewSessionCookie(owner.ID)
	require.NoError(t, err)

	return router, sessionValue, owner, conn, identitySvc, todoSvc
}

// newAgentPublicAPIRouter builds a bare /api/v1 stack — RejectActorFields,
// RequireActor, then internal/api's own request validator — mounting only
// TodoServer's CreateTodo/CreateTodoEvent routes directly (the same
// "mount routes directly, skip the full generated ServerInterface"
// pattern internal/transport/publicapi/keys_handler_test.go's own
// newKeysIntegrationRouter already established, for the same reason: this
// helper only needs two of TodoServer's six routes, not every route the
// full compositeServer/api.RegisterHandlers wiring would otherwise
// require satisfying). This is the real Bearer-authenticated surface an
// agent actually calls in production — not a shortcut standing in for it.
func newAgentPublicAPIRouter(t *testing.T, identitySvc *identity.Service, todoSvc *todo.Service) *gin.Engine {
	t.Helper()
	validator, err := api.RequestValidator()
	require.NoError(t, err)

	router := gin.New()
	group := router.Group("/api/v1")
	group.Use(publicapi.RejectActorFields(), publicapi.RequireActor(identitySvc), validator)
	todoServer := publicapi.NewTodoServer(todoSvc)
	group.POST("/todos", todoServer.CreateTodo)
	group.POST("/todos/:id/events", func(c *gin.Context) { todoServer.CreateTodoEvent(c, c.Param("id")) })
	return router
}

// doAgentAPIRequest issues an HTTP request against the /api/v1 surface,
// presenting rawKey as `Authorization: Bearer <rawKey>` — the agent's own
// auth mechanism, distinct from doBFFJSONRequest's session cookie.
// Mirrors internal/transport/publicapi's own doJSONRequest.
func doAgentAPIRequest(t *testing.T, router *gin.Engine, method, path, rawKey string, body any) *httptest.ResponseRecorder {
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

// decodeAgentAPITodo/-Event decode internal/api's own Todo/TodoEvent
// response shapes — a distinct Go type from bffapi.Todo/TodoEvent (two
// independently oapi-codegen-generated packages, `_contract/API.md`'s
// "Two specs, not one" — same field names, different types), needed here
// because this file's agent-side requests go through internal/api's
// ServerInterface, not internal/bffapi's.
func decodeAgentAPITodo(t *testing.T, rec *httptest.ResponseRecorder) api.Todo {
	t.Helper()
	var got api.Todo
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got
}

func decodeAgentAPIEvent(t *testing.T, rec *httptest.ResponseRecorder) api.TodoEvent {
	t.Helper()
	var got api.TodoEvent
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got
}

// TestDoneWhen12_ActivityFeed_CrossActorAttribution is GOAL.md's Done-when
// 12, task-5's own scope item: "The feed is proven cross-actor, not merely
// dual-page." Done-when 8 (todo_handler_test.go, task-4) proves a given
// event renders identically regardless of which page's query fed it — it
// never proves either page shows another actor's event. This test is
// ruling 1's only real proof that todos are a genuinely shared collection
// (GOAL.md's Ownership model decision), not just two views onto the same
// single actor's own data:
//
//  1. A real agent identity is seeded through
//     identity.Service.IssueAPIKeyForHandle — the exact method
//     cmd/issue-key's own `run` calls (cmd/issue-key/main.go:123), not a
//     direct repo.CreateUser(..., "agent", ...) fixture insert. GOAL.md's
//     "Test-fixture discipline" row names precisely this trap:
//     internal/transport/publicapi/publicapi_testutil_test.go's own
//     createAgentWithKey (used by many pre-existing tests in that
//     package, none of which this task touches) takes exactly that
//     shortcut, and internal/transport/bff/keys_handler_test.go's
//     newBFFRouterForTwoOwnersWithKeys is the specific historical example
//     GOAL.md calls out as "green for an unrelated reason." Neither is
//     reused here.
//  2. That agent creates a todo and comments on it (a non-"created"
//     event) over the real Bearer-authenticated /api/v1 surface — the
//     same handler production traffic hits, not a service-layer shortcut.
//  3. A completely separate owner session reads GET /api/bff/activity and
//     the agent's comment is actually there, attributed to the agent
//     (actor.handle == the agent's own handle, actor.role == "agent") —
//     not just present in raw count, but findable by id with the right
//     provenance.
//
// The assertion shape matters as much as the setup: this test requires
// the feed non-empty up front (guards the "silently swallow errors,
// return {items:[]}" failure mode), then separately requires (not merely
// checks) that the agent's specific event is found by id (guards the
// "feed only ever shows the viewer's own events" failure mode) — both are
// must-fail-loudly assertions, not conditionals that would let this test
// pass vacuously on broken input. Both attacks (invert the handler to
// scope by actor; invert it to return an empty feed) were exercised
// manually against this exact test and reverted — see
// task-5-report.md for the real command output of both.
func TestDoneWhen12_ActivityFeed_CrossActorAttribution(t *testing.T) {
	bffRouter, ownerSession, _, _, identitySvc, todoSvc := newBFFRouterForOwnerSharedDB(t)
	agentRouter := newAgentPublicAPIRouter(t, identitySvc, todoSvc)

	// 1. Seed a real agent identity through the same service method
	// cmd/issue-key's own `run` calls — not a direct repo insert.
	issued, err := identitySvc.IssueAPIKeyForHandle(t.Context(), "activity-feed-agent")
	require.NoError(t, err)
	require.Equal(t, "agent", issued.User.Role, "IssueAPIKeyForHandle must produce a real role=agent user, the only path cmd/issue-key ever takes")
	agentKey := issued.RawKey
	require.NotEmpty(t, agentKey)

	// 2. The agent creates a todo, then comments on it (a non-"created"
	// event), over the real Bearer-authenticated public API.
	createRec := doAgentAPIRequest(t, agentRouter, http.MethodPost, "/api/v1/todos", agentKey, map[string]any{
		"title":           "agent's own todo",
		"clientRequestId": "agent-create-1",
	})
	require.Equal(t, http.StatusCreated, createRec.Code, "agent must be able to create a todo over the real public API")
	agentTodo := decodeAgentAPITodo(t, createRec)
	require.NotEmpty(t, agentTodo.Id)

	commentRec := doAgentAPIRequest(t, agentRouter, http.MethodPost, "/api/v1/todos/"+agentTodo.Id+"/events", agentKey, map[string]any{
		"type":            "commented",
		"clientRequestId": "agent-comment-1",
		"body":            "an agent said this",
	})
	require.Equal(t, http.StatusCreated, commentRec.Code, "agent must be able to comment on its own todo over the real public API")
	agentEvent := decodeAgentAPIEvent(t, commentRec)
	require.Equal(t, "commented", agentEvent.Type)

	// 3. A completely separate owner session reads the cross-todo feed.
	feedRec := doBFFJSONRequest(t, bffRouter, http.MethodGet, "/api/bff/activity", ownerSession, nil)
	require.Equal(t, http.StatusOK, feedRec.Code)
	feed := decodeBFFActivityFeed(t, feedRec)

	// Guards the "silently swallow errors / return {items:[]}" failure
	// mode: an empty feed must never be mistaken for a passing result.
	require.NotEmpty(t, feed.Items, "the owner's feed must not be trivially empty — the agent just wrote two events into the shared collection")

	// Guards the "feed only ever shows the viewer's own events" failure
	// mode: find the agent's specific comment by id, not just assert a
	// nonzero count (a viewer-scoped feed would still show the owner's
	// own zero events as a nonzero-looking response in some broken
	// implementations, so the identity of what's found matters, not just
	// that something was).
	var found *bffapi.ActivityItem
	for i := range feed.Items {
		if feed.Items[i].Id == agentEvent.Id {
			found = &feed.Items[i]
			break
		}
	}
	require.NotNil(t, found, "the agent's comment must be present in the OWNER's session-authenticated feed — a feed that only ever showed the viewer's own events would fail exactly here")

	assert.Equal(t, "commented", found.Type)
	require.NotNil(t, found.Body)
	assert.Equal(t, "an agent said this", *found.Body)
	assert.Equal(t, agentTodo.Id, found.Todo.Id)
	assert.Equal(t, "agent's own todo", found.Todo.Title)

	// The actual point of Done-when 12: correctly attributed to the
	// agent, not the owner, not blank, not misrouted.
	assert.Equal(t, issued.User.Handle, found.Actor.Handle, "the event must be attributed to the agent's own handle")
	assert.Equal(t, "agent", found.Actor.Role, "the event's actor role must read back as agent, not owner")
}

// TestBFFHandler_ListActivity_OrderingAndPagination is a wiring/shape
// sanity check beyond the cross-actor proof above: several events across
// two todos come back newest-first, a page size smaller than the total
// event count produces a non-nil nextCursor, and following that cursor
// reaches the remaining events with no gap and no duplicate — mirrors
// task-4's own TestBFFHandler_EventsRoundTrip_* sanity-check shape.
func TestBFFHandler_ListActivity_OrderingAndPagination(t *testing.T) {
	router, sessionValue, _ := newBFFRouterForOwner(t)

	// Two todos, three events total: created(A), created(B), commented(B).
	createA := doBFFJSONRequest(t, router, http.MethodPost, "/api/bff/todos", sessionValue, map[string]any{
		"title": "todo A", "clientRequestId": "a-create",
	})
	require.Equal(t, http.StatusCreated, createA.Code)
	todoA := decodeBFFTodo(t, createA)

	createB := doBFFJSONRequest(t, router, http.MethodPost, "/api/bff/todos", sessionValue, map[string]any{
		"title": "todo B", "clientRequestId": "b-create",
	})
	require.Equal(t, http.StatusCreated, createB.Code)
	todoB := decodeBFFTodo(t, createB)

	commentB := doBFFJSONRequest(t, router, http.MethodPost, "/api/bff/todos/"+todoB.Id+"/events", sessionValue, map[string]any{
		"type": "commented", "clientRequestId": "b-comment", "body": "on B",
	})
	require.Equal(t, http.StatusCreated, commentB.Code)

	// Page 1: limit=2, newest first — should be [commented(B), created(B)].
	page1Rec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/activity?limit=2", sessionValue, nil)
	require.Equal(t, http.StatusOK, page1Rec.Code)
	page1 := decodeBFFActivityFeed(t, page1Rec)
	require.Len(t, page1.Items, 2)
	assert.Equal(t, "commented", page1.Items[0].Type)
	assert.Equal(t, todoB.Id, page1.Items[0].Todo.Id)
	assert.Equal(t, "created", page1.Items[1].Type)
	assert.Equal(t, todoB.Id, page1.Items[1].Todo.Id)
	require.NotNil(t, page1.NextCursor, "a third event (created(A)) still exists beyond this page")

	// Page 2: follow the cursor — should be exactly [created(A)], and
	// exhausted (nil nextCursor).
	page2Path := "/api/bff/activity?limit=2&cursorCreatedAtMs=" +
		strconv.FormatInt(page1.NextCursor.CreatedAtMs, 10) + "&cursorId=" + page1.NextCursor.Id
	page2Rec := doBFFJSONRequest(t, router, http.MethodGet, page2Path, sessionValue, nil)
	require.Equal(t, http.StatusOK, page2Rec.Code)
	page2 := decodeBFFActivityFeed(t, page2Rec)
	require.Len(t, page2.Items, 1)
	assert.Equal(t, "created", page2.Items[0].Type)
	assert.Equal(t, todoA.Id, page2.Items[0].Todo.Id)
	assert.Nil(t, page2.NextCursor, "every event has now been paged through")

	// No overlap between the two pages.
	page1IDs := map[string]bool{}
	for _, item := range page1.Items {
		page1IDs[item.Id] = true
	}
	assert.False(t, page1IDs[page2.Items[0].Id], "page 2's event must not have already appeared on page 1")
}

// TestBFFHandler_ListActivity_MalformedCursorRejected — a lone
// cursorCreatedAtMs or cursorId (without its pair) is a validation_error,
// not silently treated as "no cursor" or a panic. bff-openapi.yaml can
// declare each query parameter's own type but not a cross-field "both or
// neither" rule, so this is the handler's own check.
func TestBFFHandler_ListActivity_MalformedCursorRejected(t *testing.T) {
	router, sessionValue, _ := newBFFRouterForOwner(t)

	cases := []string{
		"/api/bff/activity?cursorCreatedAtMs=12345",
		"/api/bff/activity?cursorId=some-id",
	}
	for _, path := range cases {
		t.Run(path, func(t *testing.T) {
			rec := doBFFJSONRequest(t, router, http.MethodGet, path, sessionValue, nil)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, "validation_error", decodeBFFError(t, rec).Error.Code)
		})
	}
}

// TestBFFHandler_ListActivity_Unauthenticated_Returns401 mirrors every
// other bff todo endpoint's own unauthenticated-401 test.
func TestBFFHandler_ListActivity_Unauthenticated_Returns401(t *testing.T) {
	router, _, _ := newBFFRouterForOwner(t)

	rec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/activity", "", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "unauthorized", decodeBFFError(t, rec).Error.Code)
}
