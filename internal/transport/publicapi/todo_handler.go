package publicapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mildronize/my-template/internal/api"
	"github.com/mildronize/my-template/internal/domain/todo"
)

// TodoServer adapts todo.Service to internal/api's generated
// ServerInterface's todo-shaped subset (ListTodos, CreateTodo, GetTodo,
// UpdateTodo, ListTodoEvents, CreateTodoEvent). MeServer/KeysServer
// (me_handler.go/keys_handler.go, this package) contribute the
// identity-shaped subset — cmd/server composes all three into one
// api.ServerInterface implementation by embedding, so GET /api/v1/me and
// /api/v1/keys run through the exact same generated-interface,
// openapi-validated path as every other endpoint (task-3, unchanged by
// this package's move).
//
// milestone-4: todos are a shared collection (GOAL.md's Ownership model
// decision) — every method below reads/writes across every todo, not
// scoped to the caller (I3 no longer applies to this domain). There is
// no DeleteTodo method on this type at all (GOAL.md's "DELETE removed"
// decision, mirroring my-task's I12) — nothing for
// api.ServerInterface's route registration to wire up, so
// DELETE /api/v1/todos/{id} is a genuine 404 (no route), not a 405.
type TodoServer struct {
	Service *todo.Service
}

// NewTodoServer builds a TodoServer on top of svc.
func NewTodoServer(svc *todo.Service) *TodoServer {
	return &TodoServer{Service: svc}
}

// todoNotFoundError is the one error body GetTodo/UpdateTodo/
// ListTodoEvents/CreateTodoEvent ever write for an unknown todo id
// (validation_error responses come from internal/api's openapi request
// validator instead, before a request even reaches here — API.md). Named
// with a todo- prefix, not the bare notFoundError this file used before
// task-9, so that a fork copying this file to <new>_handler.go and
// renaming Todo -> <New> throughout ends up with its own distinctly-named
// value instead of redeclaring this one — mirrors keys_handler.go's own
// notFoundBody, one per handler file, never shared. The unauthorized case
// has no equivalent per-file value: its message never varies by domain,
// so actorID (middleware.go) writes the package's one shared
// unauthorizedBody instead of a second, todo-specific copy of the same
// text — and I18's permission rejection (a Bearer-authenticated agent
// attempting status:closed) reuses that exact same body too
// (_contract/API.md's error-shape section: "same body regardless of
// which check failed", mirroring I5 — this project has never had a 403).
var todoNotFoundError = newAPIError("not_found", "no such todo")

// validationErrorBody builds a validation_error-coded api.Error with a
// hint naming the offending field — _contract/API.md's "hint names the
// field" convention, for the request-shape problems this handler itself
// must catch (event-type dispatch, status/field enum values) rather than
// openapi.yaml's own request validator (which already handles missing
// required fields, wrong JSON types, and additionalProperties: false
// violations like a stray `done` before a request ever reaches here).
func validationErrorBody(message, hint string) api.Error {
	e := newAPIError("validation_error", message)
	h := hint
	e.Error.Hint = &h
	return e
}

func toAPITodo(t todo.Todo) api.Todo {
	var priority *api.TodoPriority
	if t.Priority != nil {
		p := api.TodoPriority(*t.Priority)
		priority = &p
	}
	return api.Todo{
		Id:         t.ID,
		Title:      t.Title,
		Status:     api.TodoStatus(t.Status),
		AssigneeId: t.AssigneeID,
		Priority:   priority,
		DueDate:    t.DueDate,
		CreatedBy:  t.CreatedBy,
		CreatedAt:  t.CreatedAt,
		UpdatedAt:  t.UpdatedAt,
	}
}

// toAPIEvent decodes a todo.TodoEvent's raw-JSON Payload string into the
// generic object api.TodoEvent's own Payload field expects
// (_contract/API.md's activity-feed example shows payload as a JSON
// object, e.g. `{"from": "open", "to": "in_progress"}`, never a raw
// string) — the one place in this file that touches encoding/json
// directly, since todo.Service/Repo already store it pre-marshalled.
func toAPIEvent(e todo.TodoEvent) (api.TodoEvent, error) {
	apiEvent := api.TodoEvent{
		Id:              e.ID,
		TodoId:          e.TodoID,
		Seq:             e.Seq,
		ActorId:         e.ActorID,
		Type:            string(e.Type),
		Body:            e.Body,
		ClientRequestId: e.ClientRequestID,
		CreatedAt:       e.CreatedAt,
	}
	if e.Payload != nil {
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(*e.Payload), &payload); err != nil {
			return api.TodoEvent{}, err
		}
		apiEvent.Payload = &payload
	}
	return apiEvent, nil
}

// policyActorFor converts the ActorFromContext-resolved identity.User down
// to todo.PolicyActor (I18) at this transport boundary — the same
// role-only shape permission.go's can() expects, deliberately not
// identity.User itself (see permission.go's own doc comment on why).
func policyActorFor(c *gin.Context) (todo.PolicyActor, string, bool) {
	user, ok := ActorFromContext(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, unauthorizedBody)
		return todo.PolicyActor{}, "", false
	}
	return todo.PolicyActor{Role: user.Role}, user.ID, true
}

// writeAppendError maps Append/CreateTodo's error results the way every
// handler below needs: todo.ErrNotFound -> 404, todo.ErrForbidden (I18's
// permission refusal) -> 401 unauthorized (never a distinct forbidden/403
// code — _contract/API.md's error-shape section, this project has never
// had one), anything else -> 500.
func writeAppendError(c *gin.Context, err error) {
	if errors.Is(err, todo.ErrNotFound) {
		c.AbortWithStatusJSON(http.StatusNotFound, todoNotFoundError)
		return
	}
	if errors.Is(err, todo.ErrForbidden) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, unauthorizedBody)
		return
	}
	c.AbortWithStatus(http.StatusInternalServerError)
}

// ListTodos implements api.ServerInterface — GET /api/v1/todos. No
// owner-scoping filter (I3 no longer applies to this domain, GOAL.md) —
// every authenticated actor sees every todo.
func (s *TodoServer) ListTodos(c *gin.Context) {
	if _, ok := ActorFromContext(c); !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, unauthorizedBody)
		return
	}

	todos, err := s.Service.ListTodos(c.Request.Context())
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	resp := api.TodoList{Todos: make([]api.Todo, 0, len(todos))}
	for _, t := range todos {
		resp.Todos = append(resp.Todos, toAPITodo(t))
	}
	c.JSON(http.StatusOK, resp)
}

// CreateTodo implements api.ServerInterface — POST /api/v1/todos.
// createdBy is always the resolved actor — never accepted from the body
// (I1; the body's own shape can't even carry an owner/actor field, since
// RejectActorFields already rejected that upstream) — but is attribution
// only, never access-scoping (GOAL.md). status always starts open: the
// current todo.CreateInput has no field for a caller-chosen initial
// status (see this package's task-3 report for why that's a real gap
// against _contract/API.md's "optionally status" text, not silently
// worked around here).
func (s *TodoServer) CreateTodo(c *gin.Context) {
	actorID, ok := actorID(c)
	if !ok {
		return
	}

	var req api.CreateTodoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// openapi.yaml's request validator (internal/api.RequestValidator)
		// already rejects a malformed/missing-title/missing-clientRequestId
		// body, or one declaring `done` (additionalProperties: false),
		// before this middleware chain reaches the handler at all
		// (API.md, Done-when 7) — this is a defensive fallback for a
		// route ever wired without that validator, not the primary
		// validation path.
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	input := todo.CreateInput{
		Title:           req.Title,
		AssigneeID:      req.AssigneeId,
		DueDate:         req.DueDate,
		ClientRequestID: req.ClientRequestId,
	}
	if req.Priority != nil {
		p := string(*req.Priority)
		input.Priority = &p
	}

	created, err := s.Service.CreateTodo(c.Request.Context(), actorID, input)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusCreated, toAPITodo(created))
}

// GetTodo implements api.ServerInterface — GET /api/v1/todos/{id}. No
// owner-scoping (I3 no longer applies to this domain): an unknown id is
// not_found, there is no "wrong owner" case left to produce it.
func (s *TodoServer) GetTodo(c *gin.Context, id string) {
	if _, ok := ActorFromContext(c); !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, unauthorizedBody)
		return
	}

	found, err := s.Service.GetTodo(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, todo.ErrNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, todoNotFoundError)
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, toAPITodo(found))
}

// UpdateTodo implements api.ServerInterface — PATCH /api/v1/todos/{id}.
// milestone-4: the only field this endpoint still writes is title —
// status/assigneeId/priority/dueDate route through
// POST .../events instead (see openapi.yaml's updateTodo description for
// the reasoning: todo.Service exposes no generic multi-field update
// method any more, only Append, I15's single write path, and my-task's
// own named source has no task-update REST endpoint at all — every write
// goes through its events endpoint). Internally this still funnels
// through Append as a field_changed(title) event, so it carries the same
// clientRequestId (I19) and I18 permission check as every other write —
// though I18 never actually restricts a title rename for either role.
func (s *TodoServer) UpdateTodo(c *gin.Context, id string) {
	actor, actorID, ok := policyActorFor(c)
	if !ok {
		return
	}

	var req api.UpdateTodoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	title := req.Title
	_, err := s.Service.Append(c.Request.Context(), todo.AppendInput{
		TodoID:          id,
		Actor:           actor,
		ActorID:         actorID,
		ClientRequestID: req.ClientRequestId,
		Type:            todo.EventTypeFieldChanged,
		FieldChange:     &todo.FieldChangeInput{Field: todo.FieldTitle, Title: &title},
	})
	if err != nil {
		writeAppendError(c, err)
		return
	}

	updated, err := s.Service.GetTodo(c.Request.Context(), id)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, toAPITodo(updated))
}

// ListTodoEvents implements api.ServerInterface —
// GET /api/v1/todos/{id}/events. This todo's own timeline, oldest first
// (todo.Service.ListEvents' own doc comment — mirrors my-task's per-task
// read, unlike the newest-first cross-todo feed, per _contract/API.md).
// No cross-todo feed on this surface — that's bff-only
// (GET /api/bff/activity, task-5's scope).
func (s *TodoServer) ListTodoEvents(c *gin.Context, id string) {
	if _, ok := ActorFromContext(c); !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, unauthorizedBody)
		return
	}

	// GetTodo first so an unknown todo id is a genuine 404 rather than a
	// silently-empty event list (ListEvents alone can't distinguish "no
	// events yet" from "no such todo" — it would return an empty slice
	// either way).
	if _, err := s.Service.GetTodo(c.Request.Context(), id); err != nil {
		if errors.Is(err, todo.ErrNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, todoNotFoundError)
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	events, err := s.Service.ListEvents(c.Request.Context(), id)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	resp := api.TodoEventList{Events: make([]api.TodoEvent, 0, len(events))}
	for _, e := range events {
		apiEvent, err := toAPIEvent(e)
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		resp.Events = append(resp.Events, apiEvent)
	}
	c.JSON(http.StatusOK, resp)
}

// validStatuses is the fixed four-value set a status_changed event's `to`
// may name (DATA_MODEL.md) — checked here, at the transport boundary,
// rather than let an arbitrary string reach Repo.UpdateStatus, which has
// no enum validation of its own.
var validStatuses = map[string]todo.Status{
	string(todo.StatusOpen):       todo.StatusOpen,
	string(todo.StatusInProgress): todo.StatusInProgress,
	string(todo.StatusDone):       todo.StatusDone,
	string(todo.StatusClosed):     todo.StatusClosed,
}

// todoFieldByWireName maps CreateTodoEventRequest's `field` value (camelCase,
// matching the Todo response's own JSON key names — "dueDate", not the
// internal snake_case todo.FieldDueDate constant) to the TodoField the
// domain service expects.
var todoFieldByWireName = map[string]todo.TodoField{
	"title":    todo.FieldTitle,
	"priority": todo.FieldPriority,
	"dueDate":  todo.FieldDueDate,
}

// CreateTodoEvent implements api.ServerInterface —
// POST /api/v1/todos/{id}/events, I15's single write path exposed over
// HTTP. Dispatches by req.Type into the matching todo.AppendInput shape
// entirely before ever calling s.Service.Append — so an invalid or
// unsupported type (type: "created" included, GOAL.md/INVARIANTS.md I16)
// is rejected here, with nothing written, rather than reaching the
// service layer at all. Not a special case for "created" specifically:
// the switch below simply has no case that produces a
// WriteEventType-shaped AppendInput for it, the same way it has none for
// any other string it doesn't recognise (Done-when 13's own "genuinely
// rejected... not misrouted" requirement, verified by
// TestDoneWhen13_CreateTodoEvent_TypeCreatedRejected in
// todo_handler_test.go).
func (s *TodoServer) CreateTodoEvent(c *gin.Context, id string) {
	actor, actorID, ok := policyActorFor(c)
	if !ok {
		return
	}

	var req api.CreateTodoEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	input := todo.AppendInput{
		TodoID:          id,
		Actor:           actor,
		ActorID:         actorID,
		ClientRequestID: req.ClientRequestId,
	}

	switch req.Type {
	case string(todo.EventTypeCommented):
		if req.Body == nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, validationErrorBody(`"body" is required for type: commented`, "body"))
			return
		}
		input.Type = todo.EventTypeCommented
		input.Comment = &todo.CommentInput{Body: *req.Body}

	case string(todo.EventTypeStatusChanged):
		if req.To == nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, validationErrorBody(`"to" is required for type: status_changed`, "to"))
			return
		}
		status, ok := validStatuses[*req.To]
		if !ok {
			c.AbortWithStatusJSON(http.StatusBadRequest, validationErrorBody("unrecognised status value", "to"))
			return
		}
		input.Type = todo.EventTypeStatusChanged
		input.StatusChange = &todo.StatusChangeInput{ToStatus: status}

	case string(todo.EventTypeAssigned):
		// req.To == nil covers both an omitted key and an explicit
		// `"to": null` — both mean "unassign", per _contract/API.md.
		input.Type = todo.EventTypeAssigned
		input.Assignment = &todo.AssignmentInput{ToAssigneeID: req.To}

	case string(todo.EventTypeFieldChanged):
		if req.Field == nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, validationErrorBody(`"field" is required for type: field_changed`, "field"))
			return
		}
		field, ok := todoFieldByWireName[*req.Field]
		if !ok {
			c.AbortWithStatusJSON(http.StatusBadRequest, validationErrorBody("unrecognised field name", "field"))
			return
		}

		change := &todo.FieldChangeInput{Field: field}
		switch field {
		case todo.FieldTitle:
			if req.To == nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, validationErrorBody(`"to" is required for field: title`, "to"))
				return
			}
			change.Title = req.To
		case todo.FieldPriority:
			// req.To == nil (omitted or explicit null) clears priority —
			// the same nullable-field convention Repo.UpdatePriority uses.
			change.Priority = req.To
		case todo.FieldDueDate:
			if req.To == nil {
				change.DueDate = nil
			} else {
				parsed, err := time.Parse(time.RFC3339, *req.To)
				if err != nil {
					c.AbortWithStatusJSON(http.StatusBadRequest, validationErrorBody(`"to" must be an RFC3339 timestamp for field: dueDate`, "to"))
					return
				}
				change.DueDate = &parsed
			}
		}
		input.Type = todo.EventTypeFieldChanged
		input.FieldChange = change

	default:
		// Covers type: "created" (I16 — never client-specifiable, see
		// todo.WriteEventType's own doc comment: there is no value it
		// could map to here) and any other unrecognised string alike —
		// deliberately the same path, not a special case for "created".
		c.AbortWithStatusJSON(http.StatusBadRequest, validationErrorBody(`unrecognised or unsupported "type"`, "type"))
		return
	}

	event, err := s.Service.Append(c.Request.Context(), input)
	if err != nil {
		writeAppendError(c, err)
		return
	}

	apiEvent, err := toAPIEvent(event)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusCreated, apiEvent)
}
