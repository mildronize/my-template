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

// decodeBFFTodoEvent decodes a bffapi.TodoEvent-shaped response body — the
// POST .../events success shape. Mirrors internal/transport/publicapi's
// own decodeTodoEvent.
func decodeBFFTodoEvent(t *testing.T, rec *httptest.ResponseRecorder) bffapi.TodoEvent {
	t.Helper()
	var got bffapi.TodoEvent
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got
}

// decodeBFFTodoEventList decodes a bffapi.TodoEventList-shaped response
// body — the GET .../events success shape.
func decodeBFFTodoEventList(t *testing.T, rec *httptest.ResponseRecorder) bffapi.TodoEventList {
	t.Helper()
	var got bffapi.TodoEventList
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
// session cookie for each — needed by tests that exercise two distinct
// session-authenticated actors against one shared collection of todos
// (milestone-4: todos are shared, I3 no longer scopes them — see
// TestI3NoLongerApplies_BFFHandlerReadsEveryTodoRegardlessOfCreator,
// below).
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

// TestBFFHandler_FullCRUDRoundTrip_RealSessionCookie is this task's own
// verification item: create -> list -> get -> patch(title) for a single
// owner, against a real (signed) session cookie, not a unit-injected
// actor — asserting each step actually persisted by reading it back
// afterward, not just that the write itself answered 200/201. milestone-4:
// no more delete (DELETE /api/bff/todos/{id} is covered separately below,
// TestDoneWhen6_DeleteTodo_RouteGenuinelyDoesNotExist_BFF) and no more
// `done` — a fresh todo starts `status: open`.
func TestBFFHandler_FullCRUDRoundTrip_RealSessionCookie(t *testing.T) {
	router, sessionValue, _ := newBFFRouterForOwner(t)

	// Create.
	createRec := doBFFJSONRequest(t, router, http.MethodPost, "/api/bff/todos", sessionValue, map[string]any{
		"title":           "write the report",
		"clientRequestId": "create-1",
	})
	require.Equal(t, http.StatusCreated, createRec.Code)
	created := decodeBFFTodo(t, createRec)
	assert.NotEmpty(t, created.Id)
	assert.Equal(t, "write the report", created.Title)
	assert.Equal(t, bffapi.Open, created.Status, "a new todo must start open")
	assert.Nil(t, created.AssigneeId)
	assert.Nil(t, created.Priority)
	assert.Nil(t, created.DueDate)
	assert.NotEmpty(t, created.CreatedBy)

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

	// Patch (title, the only field this endpoint still writes — milestone-4).
	patchRec := doBFFJSONRequest(t, router, http.MethodPatch, "/api/bff/todos/"+created.Id, sessionValue, map[string]any{
		"title":           "write the final report",
		"clientRequestId": "patch-1",
	})
	require.Equal(t, http.StatusOK, patchRec.Code)
	patched := decodeBFFTodo(t, patchRec)
	assert.Equal(t, "write the final report", patched.Title)
	assert.Equal(t, bffapi.Open, patched.Status, "a title-only patch must never touch status")

	// Read back after the patch, independently of the patch response.
	getAfterPatchRec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/todos/"+created.Id, sessionValue, nil)
	require.Equal(t, http.StatusOK, getAfterPatchRec.Code)
	assert.Equal(t, "write the final report", decodeBFFTodo(t, getAfterPatchRec).Title, "the update must actually be persisted, not just echoed back by the patch response")
}

// TestI3NoLongerApplies_BFFHandlerReadsEveryTodoRegardlessOfCreator —
// milestone-4's Ownership model decision (GOAL.md): todos are a shared
// collection, so I3 no longer scopes reads/writes on this domain, on
// either surface. Mirrors internal/transport/publicapi's own
// TestI3NoLongerApplies_HandlerReadsEveryTodoRegardlessOfCreator, proving
// the same property against /api/bff, session-authenticated: a todo
// created by one owner session is visible (GET) and mutable (event write)
// by a second, completely distinct owner session, not a 404 — and an id
// that genuinely never existed is still not_found.
func TestI3NoLongerApplies_BFFHandlerReadsEveryTodoRegardlessOfCreator(t *testing.T) {
	router, sessionA, sessionB, _, _ := newBFFRouterForTwoOwners(t)

	createRec := doBFFJSONRequest(t, router, http.MethodPost, "/api/bff/todos", sessionA, map[string]any{
		"title":           "owner A's todo",
		"clientRequestId": "create-shared-1",
	})
	require.Equal(t, http.StatusCreated, createRec.Code)
	theirs := decodeBFFTodo(t, createRec)

	// A different owner session can read it.
	getRec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/todos/"+theirs.Id, sessionB, nil)
	require.Equal(t, http.StatusOK, getRec.Code)
	assert.Equal(t, theirs.Id, decodeBFFTodo(t, getRec).Id)

	// ...and act on it (comment, via the events endpoint).
	commentRec := doBFFJSONRequest(t, router, http.MethodPost, "/api/bff/todos/"+theirs.Id+"/events", sessionB, map[string]any{
		"type":            "commented",
		"clientRequestId": "comment-1",
		"body":            "looked at this",
	})
	require.Equal(t, http.StatusCreated, commentRec.Code)

	// An id that genuinely never existed is still not_found.
	unknownID := "00000000-0000-0000-0000-000000000000"
	rec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/todos/"+unknownID, sessionB, nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "not_found", decodeBFFError(t, rec).Error.Code)
}

// TestBFFHandler_ListTodos_Unauthenticated_Returns401NotRedirect proves
// _contract/API.md's explicit behavior change on this surface: unlike the
// html/template view's own former session-gate (middleware.go's
// RequireSession, redirect-to-/login shaped — deleted in a milestone-3/
// task-3 fix-round once its one production caller, view_handler.go, was
// itself already retired by the SPA), a missing session on /api/bff
// answers 401 JSON, never a 302 redirect — "a fetch call can't follow a
// redirect the way a browser navigation does."
func TestBFFHandler_ListTodos_Unauthenticated_Returns401NotRedirect(t *testing.T) {
	router, _, _ := newBFFRouterForOwner(t)

	rec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/todos", "", nil)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.NotEqual(t, http.StatusFound, rec.Code, "must never redirect on this JSON surface (_contract/API.md)")
	assert.Empty(t, rec.Header().Get("Location"), "must not set a redirect Location header either")
	assert.Equal(t, "unauthorized", decodeBFFError(t, rec).Error.Code)
}

// TestBFFHandler_CreateTodo_DoneFieldRejected — _contract/API.md's
// todo-shape section: "done is gone... sending it in a write body is a
// validation_error (hint: "done"), not a silently-dropped key." Mirrors
// internal/transport/publicapi's own TestHandler_CreateTodo_DoneFieldRejected.
// CreateTodoRequest's additionalProperties: false makes this a
// bff-openapi.yaml-layer rejection, so this also proves the row was never
// created.
func TestBFFHandler_CreateTodo_DoneFieldRejected(t *testing.T) {
	router, sessionValue, _ := newBFFRouterForOwner(t)

	rec := doBFFJSONRequest(t, router, http.MethodPost, "/api/bff/todos", sessionValue, map[string]any{
		"title":           "sneaky done",
		"clientRequestId": "req-1",
		"done":            true,
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "validation_error", decodeBFFError(t, rec).Error.Code)

	listRec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/todos", sessionValue, nil)
	require.Equal(t, http.StatusOK, listRec.Code)
	var list bffapi.TodoList
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &list))
	assert.Empty(t, list.Todos, "a body declaring done must never reach the handler")
}

// TestBFFHandler_UpdateTodo_DoneFieldRejected — same property as
// TestBFFHandler_CreateTodo_DoneFieldRejected, for PATCH.
func TestBFFHandler_UpdateTodo_DoneFieldRejected(t *testing.T) {
	router, sessionValue, _ := newBFFRouterForOwner(t)

	createRec := doBFFJSONRequest(t, router, http.MethodPost, "/api/bff/todos", sessionValue, map[string]any{
		"title":           "a todo",
		"clientRequestId": "req-1",
	})
	require.Equal(t, http.StatusCreated, createRec.Code)
	created := decodeBFFTodo(t, createRec)

	rec := doBFFJSONRequest(t, router, http.MethodPatch, "/api/bff/todos/"+created.Id, sessionValue, map[string]any{
		"title":           "still a todo",
		"clientRequestId": "patch-1",
		"done":            true,
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "validation_error", decodeBFFError(t, rec).Error.Code)

	getRec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/todos/"+created.Id, sessionValue, nil)
	require.Equal(t, http.StatusOK, getRec.Code)
	assert.Equal(t, "a todo", decodeBFFTodo(t, getRec).Title, "the rejected patch must never have applied")
}

// TestDoneWhen14_CreateTodoEvent_TypeCreatedRejected — GOAL.md's
// Done-when 14: I16 (created is never client-specifiable) verified at the
// HTTP layer on the BFF, independently of Done-when 13's proof on the
// public API — two handlers, two separate chances to get this wrong. A
// POST /api/bff/todos/{id}/events with type: "created" is genuinely
// rejected (400, validation_error) — not silently accepted and dropped,
// not misrouted. Asserts both the status code/body AND (via
// countBFFTodoEventRows) that nothing was actually written: a handler
// that returned 400 after already having inserted something would still
// fail this. Also covers an ordinary unrecognised type ("sabotage") the
// same way, in a subtest, to confirm the dispatch logic doesn't
// special-case "created" differently from any other invalid type value —
// mirrors internal/transport/publicapi's own
// TestDoneWhen13_CreateTodoEvent_TypeCreatedRejected exactly, against
// this package's own handler.
func TestDoneWhen14_CreateTodoEvent_TypeCreatedRejected(t *testing.T) {
	cases := []struct {
		name string
		typ  string
	}{
		{"type: created", "created"},
		{"type: an ordinary unrecognised string", "sabotage"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router, sessionValue, _ := newBFFRouterForOwner(t)

			createRec := doBFFJSONRequest(t, router, http.MethodPost, "/api/bff/todos", sessionValue, map[string]any{
				"title":           "a todo",
				"clientRequestId": "req-1",
			})
			require.Equal(t, http.StatusCreated, createRec.Code)
			created := decodeBFFTodo(t, createRec)

			rec := doBFFJSONRequest(t, router, http.MethodPost, "/api/bff/todos/"+created.Id+"/events", sessionValue, map[string]any{
				"type":            tc.typ,
				"clientRequestId": "attack-1",
				"body":            "forged event",
			})
			require.Equal(t, http.StatusBadRequest, rec.Code)
			errBody := decodeBFFError(t, rec)
			assert.Equal(t, "validation_error", errBody.Error.Code)

			// The todo's own timeline is untouched — only its own
			// "created" event exists, the rejected write added nothing.
			timelineRec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/todos/"+created.Id+"/events", sessionValue, nil)
			require.Equal(t, http.StatusOK, timelineRec.Code)
			timeline := decodeBFFTodoEventList(t, timelineRec)
			require.Len(t, timeline.Events, 1, "the rejected write must not have added any event row")
			assert.Equal(t, "created", timeline.Events[0].Type)

			// The todo's own state is untouched too.
			getRec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/todos/"+created.Id, sessionValue, nil)
			require.Equal(t, http.StatusOK, getRec.Code)
			assert.Equal(t, "a todo", decodeBFFTodo(t, getRec).Title)
			assert.Equal(t, bffapi.Open, decodeBFFTodo(t, getRec).Status)
		})
	}
}

// TestI18_BFF_OwnerCanCloseTodo proves this task's own scope item at the
// HTTP layer, not just inferred from todo.Service's own I18 test
// (internal/domain/todo/permission_test.go): a session-authenticated
// owner's `status: closed` write genuinely succeeds through
// POST /api/bff/todos/{id}/events — I18's "this is the owner's surface"
// half. Contrast with internal/transport/publicapi's own
// TestDoneWhen13_..., where the same write from a Bearer-authenticated
// agent is rejected — the same permission layer resolving differently
// given a different actor role, not different code paths.
func TestI18_BFF_OwnerCanCloseTodo(t *testing.T) {
	router, sessionValue, _ := newBFFRouterForOwner(t)

	createRec := doBFFJSONRequest(t, router, http.MethodPost, "/api/bff/todos", sessionValue, map[string]any{
		"title":           "close me",
		"clientRequestId": "req-1",
	})
	require.Equal(t, http.StatusCreated, createRec.Code)
	created := decodeBFFTodo(t, createRec)

	closeRec := doBFFJSONRequest(t, router, http.MethodPost, "/api/bff/todos/"+created.Id+"/events", sessionValue, map[string]any{
		"type":            "status_changed",
		"clientRequestId": "close-1",
		"to":              "closed",
	})
	require.Equal(t, http.StatusCreated, closeRec.Code, "an owner-session status:closed write must succeed (I18)")
	closedEvent := decodeBFFTodoEvent(t, closeRec)
	assert.Equal(t, "status_changed", closedEvent.Type)

	getRec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/todos/"+created.Id, sessionValue, nil)
	require.Equal(t, http.StatusOK, getRec.Code)
	assert.Equal(t, bffapi.Closed, decodeBFFTodo(t, getRec).Status, "the todo must actually have moved to closed, not just answered 201")
}

// TestBFFHandler_EventsRoundTrip_CreateReadAppendFieldChangedReadTimeline
// — a real wiring sanity check beyond the negative tests above: create a
// todo with the new fields through the BFF, read it back, append a
// field_changed event changing its priority, then read the timeline back
// and confirm the event is there with the right payload. Mirrors
// internal/transport/publicapi's own
// TestHandler_EventsRoundTrip_CreateReadAppendFieldChangedReadTimeline.
func TestBFFHandler_EventsRoundTrip_CreateReadAppendFieldChangedReadTimeline(t *testing.T) {
	router, sessionValue, owner := newBFFRouterForOwner(t)

	createRec := doBFFJSONRequest(t, router, http.MethodPost, "/api/bff/todos", sessionValue, map[string]any{
		"title":           "ship the release",
		"clientRequestId": "create-rt-1",
		"assigneeId":      owner.ID,
		"priority":        "low",
		"dueDate":         "2026-09-01T00:00:00Z",
	})
	require.Equal(t, http.StatusCreated, createRec.Code)
	created := decodeBFFTodo(t, createRec)
	require.NotNil(t, created.AssigneeId)
	assert.Equal(t, owner.ID, *created.AssigneeId)
	require.NotNil(t, created.Priority)
	assert.Equal(t, bffapi.TodoPriority("low"), *created.Priority)
	require.NotNil(t, created.DueDate)

	// Append a field_changed event, changing priority low -> urgent.
	appendRec := doBFFJSONRequest(t, router, http.MethodPost, "/api/bff/todos/"+created.Id+"/events", sessionValue, map[string]any{
		"type":            "field_changed",
		"clientRequestId": "field-change-1",
		"field":           "priority",
		"to":              "urgent",
	})
	require.Equal(t, http.StatusCreated, appendRec.Code)
	appended := decodeBFFTodoEvent(t, appendRec)
	assert.Equal(t, "field_changed", appended.Type)
	require.NotNil(t, appended.Payload)
	payload := *appended.Payload
	assert.Equal(t, "priority", payload["field"])
	assert.Equal(t, "low", payload["from"])
	assert.Equal(t, "urgent", payload["to"])

	// The todo's own state actually changed.
	afterRec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/todos/"+created.Id, sessionValue, nil)
	require.Equal(t, http.StatusOK, afterRec.Code)
	after := decodeBFFTodo(t, afterRec)
	require.NotNil(t, after.Priority)
	assert.Equal(t, bffapi.TodoPriority("urgent"), *after.Priority)

	// Read the timeline back and find both events, oldest first.
	timelineRec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/todos/"+created.Id+"/events", sessionValue, nil)
	require.Equal(t, http.StatusOK, timelineRec.Code)
	timeline := decodeBFFTodoEventList(t, timelineRec)
	require.Len(t, timeline.Events, 2)
	assert.Equal(t, "created", timeline.Events[0].Type)
	assert.Equal(t, "field_changed", timeline.Events[1].Type)
	assert.Equal(t, appended.Id, timeline.Events[1].Id)
	require.NotNil(t, timeline.Events[1].Payload)
	timelinePayload := *timeline.Events[1].Payload
	assert.Equal(t, "urgent", timelinePayload["to"])
}

// TestDoneWhen6_DeleteTodo_RouteGenuinelyDoesNotExist_BFF — GOAL.md
// Done-when 6 (BFF-API half): DELETE /api/bff/todos/{id} genuinely 404s —
// the route doesn't exist at all (no DeleteTodo method on TodoServer,
// nothing registered for that method+path pair), not a 405 (which would
// mean something exists there that merely refuses the method) and not a
// silent 204. Checked against a real built router (newTestRouter/
// bffapi.RegisterHandlers, the exact composition cmd/server itself uses
// for this surface), not "the handler doesn't exist so it must 404"
// reasoning alone. Mirrors internal/transport/publicapi's own
// TestDoneWhen6_DeleteTodo_RouteGenuinelyDoesNotExist.
func TestDoneWhen6_DeleteTodo_RouteGenuinelyDoesNotExist_BFF(t *testing.T) {
	router, sessionValue, _ := newBFFRouterForOwner(t)

	createRec := doBFFJSONRequest(t, router, http.MethodPost, "/api/bff/todos", sessionValue, map[string]any{
		"title":           "not deletable",
		"clientRequestId": "req-1",
	})
	require.Equal(t, http.StatusCreated, createRec.Code)
	created := decodeBFFTodo(t, createRec)

	rec := doBFFJSONRequest(t, router, http.MethodDelete, "/api/bff/todos/"+created.Id, sessionValue, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code, "DELETE must be a genuine 404 (no route), never a 405 or a silent 204")

	// The todo is, unsurprisingly, still there.
	getRec := doBFFJSONRequest(t, router, http.MethodGet, "/api/bff/todos/"+created.Id, sessionValue, nil)
	assert.Equal(t, http.StatusOK, getRec.Code)
}
