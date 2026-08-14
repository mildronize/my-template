package publicapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mildronize/my-template/internal/api"
)

// decodeTodo decodes an api.Todo-shaped response body — todo-specific
// (unlike decodeError, publicapi_testutil_test.go), so it stays in this
// file rather than the shared testutil.
func decodeTodo(t *testing.T, rec *httptest.ResponseRecorder) api.Todo {
	t.Helper()
	var got api.Todo
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got
}

// decodeTodoEvent decodes an api.TodoEvent-shaped response body — the
// POST .../events success shape.
func decodeTodoEvent(t *testing.T, rec *httptest.ResponseRecorder) api.TodoEvent {
	t.Helper()
	var got api.TodoEvent
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got
}

// decodeTodoEventList decodes an api.TodoEventList-shaped response body —
// the GET .../events success shape.
func decodeTodoEventList(t *testing.T, rec *httptest.ResponseRecorder) api.TodoEventList {
	t.Helper()
	var got api.TodoEventList
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got
}

// countTodoEventRows returns the current row count of todo_events for a
// given todo — used below to prove a rejected write truly wrote nothing
// (Done-when 13's own "not silently accepted and dropped" requirement:
// a status-code assertion alone can't tell an outright rejection apart
// from a handler that returns 400 after already having written
// something).
func countTodoEventRows(t *testing.T, conn *sql.DB, todoID string) int {
	t.Helper()
	var n int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM todo_events WHERE todo_id = ?`, todoID).Scan(&n))
	return n
}

// TestHandler_FullCRUDRoundTrip walks create -> list -> get -> patch for a
// single agent, against a real SQLite file. milestone-4: no more delete —
// DELETE /api/v1/todos/{id} is covered separately below
// (TestDoneWhen6_DeleteTodo_RouteGenuinelyDoesNotExist), since it's now a
// route-absence proof, not part of the CRUD lifecycle.
func TestHandler_FullCRUDRoundTrip(t *testing.T) {
	router, conn := newIntegrationRouter(t)
	_, rawKey := createAgentWithKey(t, conn, "agent-a")

	// Create.
	createRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/todos", rawKey, map[string]any{
		"title":           "write the report",
		"clientRequestId": "create-1",
	})
	require.Equal(t, http.StatusCreated, createRec.Code)
	created := decodeTodo(t, createRec)
	assert.NotEmpty(t, created.Id)
	assert.Equal(t, "write the report", created.Title)
	assert.Equal(t, api.Open, created.Status, "a new todo must start open")
	assert.Nil(t, created.AssigneeId)
	assert.Nil(t, created.Priority)
	assert.Nil(t, created.DueDate)
	assert.NotEmpty(t, created.CreatedBy)

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

	// Patch (title, the only field this endpoint still writes —
	// milestone-4, see todo_handler.go's UpdateTodo doc comment).
	patchRec := doJSONRequest(t, router, http.MethodPatch, "/api/v1/todos/"+created.Id, rawKey, map[string]any{
		"title":           "write the final report",
		"clientRequestId": "patch-1",
	})
	require.Equal(t, http.StatusOK, patchRec.Code)
	patched := decodeTodo(t, patchRec)
	assert.Equal(t, "write the final report", patched.Title)
	assert.Equal(t, api.Open, patched.Status, "a title-only patch must never touch status")
}

// TestI3NoLongerApplies_HandlerReadsEveryTodoRegardlessOfCreator —
// milestone-4's Ownership model decision (GOAL.md): todos are a shared
// collection, so I3 no longer scopes reads on this domain. Mirrors the
// old TestI3_HandlerOwnershipScoping_ReturnsNotFoundNotForbidden this
// replaces, proving the opposite of what that test proved — a todo
// created by one agent is visible (GET) and mutable (PATCH) by a
// completely different agent, not a 404.
func TestI3NoLongerApplies_HandlerReadsEveryTodoRegardlessOfCreator(t *testing.T) {
	router, conn := newIntegrationRouter(t)
	_, creatorKey := createAgentWithKey(t, conn, "creator")
	_, otherKey := createAgentWithKey(t, conn, "someone-else")

	createRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/todos", creatorKey, map[string]any{
		"title":           "creator's todo",
		"clientRequestId": "create-shared-1",
	})
	require.Equal(t, http.StatusCreated, createRec.Code)
	theirs := decodeTodo(t, createRec)

	// A different agent can read it.
	getRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/todos/"+theirs.Id, otherKey, nil)
	require.Equal(t, http.StatusOK, getRec.Code)
	assert.Equal(t, theirs.Id, decodeTodo(t, getRec).Id)

	// ...and act on it (comment, via the events endpoint).
	commentRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/todos/"+theirs.Id+"/events", otherKey, map[string]any{
		"type":            "commented",
		"clientRequestId": "comment-1",
		"body":            "looked at this",
	})
	require.Equal(t, http.StatusCreated, commentRec.Code)

	// An id that genuinely never existed is still not_found.
	unknownID := "00000000-0000-0000-0000-000000000000"
	rec := doJSONRequest(t, router, http.MethodGet, "/api/v1/todos/"+unknownID, otherKey, nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "not_found", decodeError(t, rec).Error.Code)
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

	rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/todos", rawKey, map[string]any{"clientRequestId": "req-1"})
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

	rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/todos", rawKey, map[string]any{
		"title":           string(tooLong),
		"clientRequestId": "req-1",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestHandler_CreateTodo_DoneFieldRejected — _contract/API.md's todo-shape
// section: "done is gone... sending it in a write body is a
// validation_error (hint: "done"), not a silently-dropped key."
// CreateTodoRequest's additionalProperties: false makes this an
// openapi.yaml-layer rejection (same mechanism as the missing-title case
// above), so this also proves the row was never created.
func TestHandler_CreateTodo_DoneFieldRejected(t *testing.T) {
	router, conn := newIntegrationRouter(t)
	_, rawKey := createAgentWithKey(t, conn, "agent-a")

	rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/todos", rawKey, map[string]any{
		"title":           "sneaky done",
		"clientRequestId": "req-1",
		"done":            true,
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "validation_error", decodeError(t, rec).Error.Code)

	listRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/todos", rawKey, nil)
	require.Equal(t, http.StatusOK, listRec.Code)
	var list api.TodoList
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &list))
	assert.Empty(t, list.Todos, "a body declaring done must never reach the handler")
}

// TestHandler_UpdateTodo_DoneFieldRejected — same property as
// TestHandler_CreateTodo_DoneFieldRejected, for PATCH.
func TestHandler_UpdateTodo_DoneFieldRejected(t *testing.T) {
	router, conn := newIntegrationRouter(t)
	_, rawKey := createAgentWithKey(t, conn, "agent-a")

	createRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/todos", rawKey, map[string]any{
		"title":           "a todo",
		"clientRequestId": "req-1",
	})
	require.Equal(t, http.StatusCreated, createRec.Code)
	created := decodeTodo(t, createRec)

	rec := doJSONRequest(t, router, http.MethodPatch, "/api/v1/todos/"+created.Id, rawKey, map[string]any{
		"title":           "still a todo",
		"clientRequestId": "patch-1",
		"done":            true,
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "validation_error", decodeError(t, rec).Error.Code)

	getRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/todos/"+created.Id, rawKey, nil)
	require.Equal(t, http.StatusOK, getRec.Code)
	assert.Equal(t, "a todo", decodeTodo(t, getRec).Title, "the rejected patch must never have applied")
}

// TestDoneWhen13_CreateTodoEvent_TypeCreatedRejected — GOAL.md's
// Done-when 13: I16 (created is never client-specifiable) verified at the
// HTTP layer on the public API. A POST .../events with type: "created" is
// genuinely rejected (400, validation_error) — not silently accepted and
// dropped, not misrouted. Asserts both the status code/body AND (via
// countTodoEventRows) that nothing was actually written: a handler that
// returned 400 after already having inserted something would still fail
// this. Also covers an ordinary unrecognised type ("sabotage") the same
// way, in a subtest, to confirm the dispatch logic doesn't special-case
// "created" differently from any other invalid type value.
func TestDoneWhen13_CreateTodoEvent_TypeCreatedRejected(t *testing.T) {
	cases := []struct {
		name string
		typ  string
	}{
		{"type: created", "created"},
		{"type: an ordinary unrecognised string", "sabotage"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router, conn := newIntegrationRouter(t)
			_, rawKey := createAgentWithKey(t, conn, "agent-a")

			createRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/todos", rawKey, map[string]any{
				"title":           "a todo",
				"clientRequestId": "req-1",
			})
			require.Equal(t, http.StatusCreated, createRec.Code)
			created := decodeTodo(t, createRec)

			before := countTodoEventRows(t, conn, created.Id)
			require.Equal(t, 1, before, "the todo's own creation should have written exactly one event")

			rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/todos/"+created.Id+"/events", rawKey, map[string]any{
				"type":            tc.typ,
				"clientRequestId": "attack-1",
				"body":            "forged event",
			})
			require.Equal(t, http.StatusBadRequest, rec.Code)
			errBody := decodeError(t, rec)
			assert.Equal(t, "validation_error", errBody.Error.Code)

			after := countTodoEventRows(t, conn, created.Id)
			assert.Equal(t, before, after, "the rejected write must not have added any row")

			// The todo's own state is untouched too.
			getRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/todos/"+created.Id, rawKey, nil)
			require.Equal(t, http.StatusOK, getRec.Code)
			assert.Equal(t, "a todo", decodeTodo(t, getRec).Title)
			assert.Equal(t, api.Open, decodeTodo(t, getRec).Status)
		})
	}
}

// TestI18_Handler_AgentClosedTransitionRejected_Returns403WithHintAndCode
// is I18's own rejection, verified at the HTTP layer on the public API —
// no equivalent existed before this test (the domain layer's own
// TestI18_Append_SameAgentSameTodo_ClosedRejected_NonClosedSucceeds in
// internal/domain/todo/service_test.go only proves the sentinel error,
// not what a real HTTP caller sees). This project's first pass mapped
// the rejection to the generic 401 unauthorized every credential failure
// produces; that was found to be a narrowing of my-task rather than a
// deliberate choice — my-task returns a distinct 403 invalid_transition
// with a hint, and a bare 401 here would tell a correctly-authenticated
// agent that its key is the problem, which is false. Asserts all three
// of status, code, and the hint's presence (not just the status code —
// a check that stopped at 403 could not tell "the right kind of 403"
// from "some other, unrelated permission refusal"), then proves the
// rejection did not leave a half-applied write (no event row added, the
// todo's own status untouched) — the same "not silently accepted and
// dropped" shape TestDoneWhen13 uses for I16 above.
func TestI18_Handler_AgentClosedTransitionRejected_Returns403WithHintAndCode(t *testing.T) {
	router, conn := newIntegrationRouter(t)
	_, rawKey := createAgentWithKey(t, conn, "agent-a")

	createRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/todos", rawKey, map[string]any{
		"title":           "a todo",
		"clientRequestId": "req-1",
	})
	require.Equal(t, http.StatusCreated, createRec.Code)
	created := decodeTodo(t, createRec)

	before := countTodoEventRows(t, conn, created.Id)

	rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/todos/"+created.Id+"/events", rawKey, map[string]any{
		"type":            "status_changed",
		"to":              "closed",
		"clientRequestId": "close-attempt-1",
	})
	require.Equal(t, http.StatusForbidden, rec.Code)
	errBody := decodeError(t, rec)
	assert.Equal(t, "invalid_transition", errBody.Error.Code)
	require.NotNil(t, errBody.Error.Hint, "the rejection must carry a hint telling the agent what to do next")
	assert.NotEmpty(t, *errBody.Error.Hint)

	after := countTodoEventRows(t, conn, created.Id)
	assert.Equal(t, before, after, "the rejected write must not have added any row")

	getRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/todos/"+created.Id, rawKey, nil)
	require.Equal(t, http.StatusOK, getRec.Code)
	assert.Equal(t, api.Open, decodeTodo(t, getRec).Status, "the rejected close attempt must not have changed the todo's status")
}

// TestHandler_EventsRoundTrip_CreateReadAppendFieldChangedReadTimeline —
// a real wiring sanity check beyond the negative test above: create a
// todo with the new fields, read it back, append a field_changed event
// changing its priority, then read the timeline back and confirm the
// event is there with the right payload.
func TestHandler_EventsRoundTrip_CreateReadAppendFieldChangedReadTimeline(t *testing.T) {
	router, conn := newIntegrationRouter(t)
	_, rawKey := createAgentWithKey(t, conn, "agent-a")
	assigneeID, _ := createAgentWithKey(t, conn, "assignee")

	createRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/todos", rawKey, map[string]any{
		"title":           "ship the release",
		"clientRequestId": "create-rt-1",
		"assigneeId":      assigneeID,
		"priority":        "low",
		"dueDate":         "2026-09-01T00:00:00Z",
	})
	require.Equal(t, http.StatusCreated, createRec.Code)
	created := decodeTodo(t, createRec)
	require.NotNil(t, created.AssigneeId)
	assert.Equal(t, assigneeID, *created.AssigneeId)
	// milestone-4 fix-round (handle-exposure): my-task's own agent-facing
	// REST contract shows a handle to agents too (not just the owner UI),
	// so this surface's Todo also carries the resolved handle now.
	require.NotNil(t, created.AssigneeHandle)
	assert.Equal(t, "assignee", *created.AssigneeHandle)
	require.NotNil(t, created.Priority)
	assert.Equal(t, api.TodoPriority("low"), *created.Priority)
	require.NotNil(t, created.DueDate)

	// Read it back.
	getRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/todos/"+created.Id, rawKey, nil)
	require.Equal(t, http.StatusOK, getRec.Code)
	got := decodeTodo(t, getRec)
	require.NotNil(t, got.Priority)
	assert.Equal(t, api.TodoPriority("low"), *got.Priority)

	// Append a field_changed event, changing priority low -> urgent.
	appendRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/todos/"+created.Id+"/events", rawKey, map[string]any{
		"type":            "field_changed",
		"clientRequestId": "field-change-1",
		"field":           "priority",
		"to":              "urgent",
	})
	require.Equal(t, http.StatusCreated, appendRec.Code)
	appended := decodeTodoEvent(t, appendRec)
	assert.Equal(t, "field_changed", appended.Type)
	require.NotNil(t, appended.Payload)
	payload := *appended.Payload
	assert.Equal(t, "priority", payload["field"])
	assert.Equal(t, "low", payload["from"])
	assert.Equal(t, "urgent", payload["to"])

	// The todo's own state actually changed.
	afterRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/todos/"+created.Id, rawKey, nil)
	require.Equal(t, http.StatusOK, afterRec.Code)
	after := decodeTodo(t, afterRec)
	require.NotNil(t, after.Priority)
	assert.Equal(t, api.TodoPriority("urgent"), *after.Priority)

	// Append an `assigned` event too, re-pointing to the SAME assignee (a
	// no-op reassignment is still a real write for this test's purposes) —
	// milestone-4 fix-round (handle-exposure): the payload's `from`/`to`
	// must now be {id, handle} snapshots, not bare ids.
	assignRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/todos/"+created.Id+"/events", rawKey, map[string]any{
		"type":            "assigned",
		"clientRequestId": "assign-1",
		"to":              assigneeID,
	})
	require.Equal(t, http.StatusCreated, assignRec.Code)
	assignedEvent := decodeTodoEvent(t, assignRec)
	require.NotNil(t, assignedEvent.Payload)
	assignPayload := *assignedEvent.Payload
	fromSnap, ok := assignPayload["from"].(map[string]interface{})
	require.True(t, ok, "assigned event's `from` must be a {id, handle} object, got %#v", assignPayload["from"])
	assert.Equal(t, assigneeID, fromSnap["id"])
	assert.Equal(t, "assignee", fromSnap["handle"])
	toSnap, ok := assignPayload["to"].(map[string]interface{})
	require.True(t, ok, "assigned event's `to` must be a {id, handle} object, got %#v", assignPayload["to"])
	assert.Equal(t, assigneeID, toSnap["id"])
	assert.Equal(t, "assignee", toSnap["handle"])

	// Assigning to an id that names no real user is a 400 validation
	// error, not a 500 and not a silently-stored garbage id — mirrors
	// my-task's own unknownAssigneeError for exactly this case.
	badAssignRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/todos/"+created.Id+"/events", rawKey, map[string]any{
		"type":            "assigned",
		"clientRequestId": "assign-bad-1",
		"to":              "no-such-user-id",
	})
	assert.Equal(t, http.StatusBadRequest, badAssignRec.Code, "an unresolvable assignee id must be rejected, not stored")

	// Read the timeline back and find every event, oldest first.
	timelineRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/todos/"+created.Id+"/events", rawKey, nil)
	require.Equal(t, http.StatusOK, timelineRec.Code)
	timeline := decodeTodoEventList(t, timelineRec)
	require.Len(t, timeline.Events, 3, "the rejected bad-assignee attempt above must not have written a row")
	assert.Equal(t, "created", timeline.Events[0].Type)
	assert.Equal(t, "field_changed", timeline.Events[1].Type)
	assert.Equal(t, appended.Id, timeline.Events[1].Id)
	require.NotNil(t, timeline.Events[1].Payload)
	timelinePayload := *timeline.Events[1].Payload
	assert.Equal(t, "urgent", timelinePayload["to"])
	assert.Equal(t, "assigned", timeline.Events[2].Type)

	// milestone-4 fix-round (handle-exposure): every row's own actorHandle
	// is now populated, a bare handle string (no role — see toAPIEvent's
	// own doc comment for why this surface deliberately withholds role
	// from an agent caller, unlike the bff surface's {handle, role}
	// object). agent-a wrote every one of these events.
	for i, e := range timeline.Events {
		assert.Equalf(t, "agent-a", e.ActorHandle, "event %d's actor handle", i)
	}
}

// TestDoneWhen6_DeleteTodo_RouteGenuinelyDoesNotExist — GOAL.md Done-when
// 6 (public-API half): DELETE /api/v1/todos/{id} genuinely 404s — the
// route doesn't exist at all (no DeleteTodo method on TodoServer,
// nothing registered for that method+path pair), not a 405 (which would
// mean something exists there that merely refuses the method) and not a
// silent 200. Checked against a real built router
// (newIntegrationRouter/api.RegisterHandlers, the exact composition
// cmd/server itself uses), not "the handler doesn't exist so it must
// 404" reasoning alone.
func TestDoneWhen6_DeleteTodo_RouteGenuinelyDoesNotExist(t *testing.T) {
	router, conn := newIntegrationRouter(t)
	_, rawKey := createAgentWithKey(t, conn, "agent-a")

	createRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/todos", rawKey, map[string]any{
		"title":           "not deletable",
		"clientRequestId": "req-1",
	})
	require.Equal(t, http.StatusCreated, createRec.Code)
	created := decodeTodo(t, createRec)

	rec := doJSONRequest(t, router, http.MethodDelete, "/api/v1/todos/"+created.Id, rawKey, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code, "DELETE must be a genuine 404 (no route), never a 405 or a silent 200")

	// The todo is, unsurprisingly, still there.
	getRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/todos/"+created.Id, rawKey, nil)
	assert.Equal(t, http.StatusOK, getRec.Code)
}

// TestHandler_Me_RunsThroughSameGeneratedInterfaceAsTodos — task-3:
// GET /api/v1/me is no longer a bespoke route; it's registered through
// the same api.RegisterHandlers call as the todo endpoints, so it's
// validated the same way as everything else. NOTE: this is currently the
// only test proving that — middleware_test.go's own handleMe coverage
// wires "/me" directly with router.GET, not through compositeServer/
// api.RegisterHandlers. It lives in this file (not a me_handler_test.go
// of its own) only because it needs the full compositeServer/
// newIntegrationRouter harness; deleting this file whole on fork (Step 8)
// silently drops this specific coverage unless it's copied into your new
// module's own handler test file first, the same way the actual
// CRUD/I3 tests below are meant to be.
func TestHandler_Me_RunsThroughSameGeneratedInterfaceAsTodos(t *testing.T) {
	router, conn := newIntegrationRouter(t)
	_, rawKey := createAgentWithKey(t, conn, "luna")

	rec := doJSONRequest(t, router, http.MethodGet, "/api/v1/me", rawKey, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"handle":"luna"`)
}
