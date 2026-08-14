package bff

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

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

// onlyEventID reads todoID's own timeline through the real GET
// .../events endpoint and returns the id of its one and only event —
// used by TestBFFHandler_ListActivity_OrderingAndPagination to find a
// just-created todo's own "created" event id, which the todo-creation
// response itself doesn't carry (that response describes the todo, not
// the event CreateTodo's own side effect wrote).
func onlyEventID(t *testing.T, router *gin.Engine, sessionValue, todoID string) string {
	t.Helper()
	rec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/todos/"+todoID+"/events", sessionValue, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	list := decodeBFFTodoEventList(t, rec)
	require.Len(t, list.Events, 1, "expected exactly one event on this todo's timeline")
	return list.Events[0].Id
}

// onlyEventIDOfType is onlyEventID's sibling for a todo whose timeline
// has more than one event — finds the single event of eventType, failing
// loudly if there's zero or more than one (a test relying on this
// helper's own timestamp-pinning depends on there being exactly one
// unambiguous target).
func onlyEventIDOfType(t *testing.T, router *gin.Engine, sessionValue, todoID, eventType string) string {
	t.Helper()
	rec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/todos/"+todoID+"/events", sessionValue, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	list := decodeBFFTodoEventList(t, rec)
	var found []string
	for _, e := range list.Events {
		if e.Type == eventType {
			found = append(found, e.Id)
		}
	}
	require.Lenf(t, found, 1, "expected exactly one %q event on this todo's timeline, found %d", eventType, len(found))
	return found[0]
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
// two todos come back paginated with no gap and no duplicate, and newest-
// first ordering holds between events that are genuinely, distinctly
// ordered in time — mirrors task-4's own TestBFFHandler_EventsRoundTrip_*
// sanity-check shape.
//
// Two properties, asserted separately, on purpose — this test's first
// version asserted only a single hardcoded page-by-page order and was
// genuinely flaky (~30-45% failure rate under repeated runs, confirmed by
// stress-testing it, not assumed): three events written back-to-back with
// no delay regularly land two of them on the identical millisecond, and
// same-millisecond order is undefined (`_contract/API.md`'s I15/pagination
// note — matches my-task's own {createdAtMs, id} cursor exactly, whose
// ids are cuids with no causal relationship to write order either;
// same-millisecond order was never guaranteed by either system, just
// astronomically unlikely to be exercised before this table existed).
// `(created_at, id)` gives a *stable total order* (same rows, same order,
// every query — no drops, no duplicates), which is the real pagination
// guarantee; it does not give a *causal* one, and this test must not
// assert a causal property the schema never promised.
//
//  1. Set-completeness and no-duplicates across every page, regardless of
//     order — holds unconditionally, no timestamp control needed.
//  2. Newest-first ordering, checked only between events pinned to
//     explicitly distinct milliseconds (the same direct-SQL-overwrite
//     technique internal/domain/todo/repo_test.go already uses) — so this
//     assertion can fail for exactly one reason: a real ordering defect,
//     never an unlucky same-millisecond tie.
func TestBFFHandler_ListActivity_OrderingAndPagination(t *testing.T) {
	router, sessionValue, _, conn, _, _ := newBFFRouterForOwnerSharedDB(t)

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
	commentBID := decodeBFFTodoEvent(t, commentB).Id

	// Pin all three events to explicitly distinct, one-hour-apart
	// milliseconds — created(A) oldest, created(B) middle, commented(B)
	// newest — removing any dependence on how fast this test happens to
	// run. This is what makes property 2 below able to fail for only one
	// reason.
	base := time.Now().UTC().Truncate(time.Millisecond)
	pin := func(eventID string, offsetHours int) {
		_, err := conn.ExecContext(t.Context(), `UPDATE todo_events SET created_at = ? WHERE id = ?`,
			base.Add(time.Duration(offsetHours)*time.Hour), eventID)
		require.NoError(t, err)
	}
	// createA's and createB's own event ids aren't returned by
	// doBFFJSONRequest's todo-creation response (that's the todo, not the
	// event) — look them up via each todo's own timeline.
	createAEventID := onlyEventID(t, router, sessionValue, todoA.Id)
	createBEventID := onlyEventIDOfType(t, router, sessionValue, todoB.Id, "created")
	pin(createAEventID, 0)
	pin(createBEventID, 1)
	pin(commentBID, 2)

	// Property 1: walk every page (limit=2, forcing at least two pages)
	// and confirm the union is exactly the three events written, no more,
	// no less, no duplicate — regardless of what order they came back in.
	seen := map[string]bool{}
	var itemCount int
	path := "/api/bff/activity?limit=2"
	for path != "" {
		rec := doBFFJSONRequest(t, router, http.MethodGet, path, sessionValue, nil)
		require.Equal(t, http.StatusOK, rec.Code)
		feed := decodeBFFActivityFeed(t, rec)
		for _, item := range feed.Items {
			assert.Falsef(t, seen[item.Id], "event %s appeared on more than one page — duplicate", item.Id)
			seen[item.Id] = true
			itemCount++
		}
		if feed.NextCursor == nil {
			break
		}
		path = "/api/bff/activity?limit=2&cursorCreatedAtMs=" +
			strconv.FormatInt(feed.NextCursor.CreatedAtMs, 10) + "&cursorId=" + feed.NextCursor.Id
	}
	assert.Equal(t, 3, itemCount, "exactly three events were written; the full paginated walk must surface exactly three, no loss")
	assert.True(t, seen[createAEventID] && seen[createBEventID] && seen[commentBID], "every written event must appear exactly once across the full walk")

	// Property 2: newest-first ordering, checked only across the pinned,
	// distinctly-timestamped events — commented(B) (newest) before
	// created(B) (middle) before created(A) (oldest), on a single
	// unpaginated fetch of all three.
	allRec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/activity?limit=10", sessionValue, nil)
	require.Equal(t, http.StatusOK, allRec.Code)
	all := decodeBFFActivityFeed(t, allRec)
	require.Len(t, all.Items, 3)
	assert.Equal(t, commentBID, all.Items[0].Id, "newest-first: the most recently pinned event must lead")
	assert.Equal(t, createBEventID, all.Items[1].Id, "newest-first: the middle-pinned event must be second")
	assert.Equal(t, createAEventID, all.Items[2].Id, "newest-first: the oldest-pinned event must be last")
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
