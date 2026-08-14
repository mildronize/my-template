package bff

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mildronize/my-template/internal/bffapi"
	"github.com/mildronize/my-template/internal/domain/todo"
	"github.com/mildronize/my-template/internal/transport/publicapi"
)

// bffActivityDefaultLimit mirrors my-task's own activity.list default
// (`~/gits/my-task/src/server/api/routers/activity.ts`:
// "limit: input.limit ?? 30") — applied here when the query omits `limit`
// entirely. The 1-100 bound itself is enforced one layer up, by
// bff-openapi.yaml's own `minimum`/`maximum` on the `limit` parameter
// (the request validator mounted ahead of this handler, same as every
// other bff-openapi.yaml-declared constraint on this surface) — this
// handler only ever sees either "omitted" or an already-in-range value.
const bffActivityDefaultLimit = 30

// TodoServer adapts todo.Service to internal/bffapi's generated
// ServerInterface's todo-shaped subset (ListTodos, CreateTodo, GetTodo,
// UpdateTodo, ListTodoEvents, CreateTodoEvent, ListActivity) — the
// session-authenticated counterpart to internal/transport/publicapi.TodoServer.
// ListActivity has no publicapi equivalent at all (task-5,
// `_contract/API.md`'s "Owner-session only; no agent-facing equivalent" —
// mirrors my-task's own `activity.list`, tRPC/owner-only). Every other
// method here calls the exact same *todo.Service instance/methods publicapi's own TodoServer calls
// (_rules/_standard/ARCHITECTURE.md's shared-service-layer rule,
// _contract/API.md's per-endpoint service-method citations) — no
// todo-specific logic is duplicated or reimplemented here, only the
// identity source differs (session-resolved owner instead of
// Bearer-resolved actor).
//
// milestone-4: todos are a shared collection (GOAL.md's Ownership model
// decision) — every method below reads/writes across every todo, not
// scoped to the session owner (I3 no longer applies to this domain, on
// either surface). There is no DeleteTodo method on this type at all
// (GOAL.md's "DELETE removed" decision, mirroring my-task's I12 and
// publicapi.TodoServer's own doc comment on the exact same point) —
// nothing for bffapi.ServerInterface's route registration to wire up, so
// DELETE /api/bff/todos/{id} is a genuine 404 (no route), not a 405.
type TodoServer struct {
	Service *todo.Service
}

// NewTodoServer builds a TodoServer on top of svc.
func NewTodoServer(svc *todo.Service) *TodoServer {
	return &TodoServer{Service: svc}
}

// bffTodoNotFoundError is the one 404 response body GetTodo/UpdateTodo/
// ListTodoEvents/CreateTodoEvent ever write for an unknown todo id — I3 no
// longer applies to this domain (GOAL.md), so there is no "wrong owner"
// case left to produce it, only "never existed". Reuses
// internal/transport/publicapi's ErrorEnvelope/NewErrorEnvelope directly,
// per _contract/API.md's explicit "bff-openapi.yaml reuses publicapi's
// envelope" decision — mirrors publicapi's own todoNotFoundError in shape
// and intent without redefining the envelope type in this package.
var bffTodoNotFoundError = publicapi.NewErrorEnvelope("not_found", "no such todo", "")

// bffValidationErrorBody builds a validation_error-coded ErrorEnvelope
// with a hint naming the offending field — mirrors publicapi's own
// validationErrorBody, reusing the same shared envelope type this
// package's other hand-written error bodies already use (bffTodoNotFoundError,
// jsonUnauthorizedBody, bffKeyNotFoundBody).
func bffValidationErrorBody(message, hint string) publicapi.ErrorEnvelope {
	return publicapi.NewErrorEnvelope("validation_error", message, hint)
}

func toBFFTodo(t todo.Todo) bffapi.Todo {
	var priority *bffapi.TodoPriority
	if t.Priority != nil {
		p := bffapi.TodoPriority(*t.Priority)
		priority = &p
	}
	return bffapi.Todo{
		Id:             t.ID,
		Title:          t.Title,
		Status:         bffapi.TodoStatus(t.Status),
		AssigneeId:     t.AssigneeID,
		AssigneeHandle: t.AssigneeHandle,
		Priority:       priority,
		DueDate:        t.DueDate,
		CreatedBy:      t.CreatedBy,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
	}
}

// toBFFEvent decodes a todo.TodoEvent's raw-JSON Payload string into the
// generic object bffapi.TodoEvent's own Payload field expects — mirrors
// publicapi's own toAPIEvent exactly, the one place in this file that
// touches encoding/json directly, since todo.Service/Repo already store it
// pre-marshalled.
//
// actor is bffapi.ActivityActor ({handle, role}) — the exact same shape
// toBFFActivityItem below already gives ActivityItem.actor, on purpose
// (milestone-4 fix-round, handle-exposure): this owner-only surface's
// whole reason for existing is the 🧑/🤖 provenance mark
// (TimelineEventRow.tsx), which needs role, not just a name. Contrast
// publicapi's own toAPIEvent (internal/transport/publicapi/
// todo_handler.go), which gives an agent caller only a handle, no role —
// see that function's own doc comment for the my-task research behind
// that asymmetry.
func toBFFEvent(e todo.TodoEvent, actor bffapi.ActivityActor) (bffapi.TodoEvent, error) {
	event := bffapi.TodoEvent{
		Id:              e.ID,
		TodoId:          e.TodoID,
		Seq:             e.Seq,
		ActorId:         e.ActorID,
		Actor:           actor,
		Type:            string(e.Type),
		Body:            e.Body,
		ClientRequestId: e.ClientRequestID,
		CreatedAt:       e.CreatedAt,
	}
	if e.Payload != nil {
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(*e.Payload), &payload); err != nil {
			return bffapi.TodoEvent{}, err
		}
		event.Payload = &payload
	}
	return event, nil
}

// bffOwnerID reads the actor RequireJSONSession already resolved onto the
// gin context (I4: this package never queries users itself for this
// purpose). The !ok branch is defensive, mirroring handleBFFMe: it should
// be unreachable given the intended middleware order (RequireJSONSession
// before any of this file's handlers), and only guards a route ever wired
// without it.
func bffOwnerID(c *gin.Context) (string, bool) {
	user, ok := ActorFromContext(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, jsonUnauthorizedBody)
		return "", false
	}
	return user.ID, true
}

// bffPolicyActor converts the ActorFromContext-resolved identity.User down
// to todo.PolicyActor (I18) at this transport boundary, and returns its id
// alongside — mirrors publicapi's own policyActorFor exactly. On this
// surface Role is always "owner" (I12: a BFF session can never resolve to
// role="agent"), so can() always permits the write; the dispatch/error
// handling shape below is still shared with publicapi's, rather than
// special-cased, so the two handlers stay structurally identical wherever
// the contract calls for it.
func bffPolicyActor(c *gin.Context) (todo.PolicyActor, string, bool) {
	user, ok := ActorFromContext(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, jsonUnauthorizedBody)
		return todo.PolicyActor{}, "", false
	}
	return todo.PolicyActor{Role: user.Role}, user.ID, true
}

// bffInvalidTransitionErrorBody mirrors publicapi's own
// invalidTransitionErrorBody (see that function's doc comment for the
// full reasoning) — unreachable through this surface in practice, since
// an owner session always passes I18's can() unconditionally, but kept
// correct rather than left as dead-but-wrong defensive code: if this
// surface's own reachability assumption ever changes, it should already
// say the right thing.
func bffInvalidTransitionErrorBody() publicapi.ErrorEnvelope {
	return publicapi.NewErrorEnvelope("invalid_transition",
		"Agents cannot move a task into the closed group",
		"Ask the owner to close this task.")
}

// writeBFFAppendError maps Append/CreateTodo's error results the way every
// handler below needs — mirrors publicapi's own writeAppendError:
// todo.ErrNotFound -> 404, todo.ErrForbidden (I18's permission refusal,
// unreachable for an owner session in practice, I12) -> 403
// invalid_transition with a hint, todo.ErrUnknownAssignee (milestone-4
// fix-round, handle-exposure: an `assigned` event whose "to" id does not
// resolve to any real user) -> 400 validation_error, mirroring my-task's
// own unknownAssigneeError for the same case, anything else -> 500.
func writeBFFAppendError(c *gin.Context, err error) {
	if errors.Is(err, todo.ErrNotFound) {
		c.AbortWithStatusJSON(http.StatusNotFound, bffTodoNotFoundError)
		return
	}
	if errors.Is(err, todo.ErrForbidden) {
		c.AbortWithStatusJSON(http.StatusForbidden, bffInvalidTransitionErrorBody())
		return
	}
	if errors.Is(err, todo.ErrUnknownAssignee) {
		c.AbortWithStatusJSON(http.StatusBadRequest, bffValidationErrorBody("unknown assignee id", "to"))
		return
	}
	c.AbortWithStatus(http.StatusInternalServerError)
}

// ListTodos implements bffapi.ServerInterface — GET /api/bff/todos. No
// owner-scoping filter (I3 no longer applies to this domain, GOAL.md) —
// every authenticated actor, including the session owner, sees every
// todo.
func (s *TodoServer) ListTodos(c *gin.Context) {
	if _, ok := bffOwnerID(c); !ok {
		return
	}

	todos, err := s.Service.ListTodos(c.Request.Context())
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	resp := bffapi.TodoList{Todos: make([]bffapi.Todo, 0, len(todos))}
	for _, t := range todos {
		resp.Todos = append(resp.Todos, toBFFTodo(t))
	}
	c.JSON(http.StatusOK, resp)
}

// CreateTodo implements bffapi.ServerInterface — POST /api/bff/todos.
// createdBy is always the resolved session owner — never accepted from
// the body (I1; RejectActorFields, reused from internal/transport/
// publicapi and mounted ahead of this handler, already rejects a request
// declaring one) — but is attribution only, never access-scoping
// (GOAL.md). status always starts open, mirroring publicapi's own
// CreateTodo.
func (s *TodoServer) CreateTodo(c *gin.Context) {
	ownerID, ok := bffOwnerID(c)
	if !ok {
		return
	}

	var req bffapi.CreateTodoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// bff-openapi.yaml's request validator (internal/bffapi.RequestValidator)
		// already rejects a malformed/missing-title/missing-clientRequestId
		// body, or one declaring `done` (additionalProperties: false),
		// before this middleware chain reaches the handler at all —
		// this is a defensive fallback for a route ever wired without
		// that validator, mirroring publicapi's own TodoServer.CreateTodo.
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

	created, err := s.Service.CreateTodo(c.Request.Context(), ownerID, input)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusCreated, toBFFTodo(created))
}

// GetTodo implements bffapi.ServerInterface — GET /api/bff/todos/{id}. No
// owner-scoping (I3 no longer applies to this domain): an unknown id is
// not_found, there is no "wrong owner" case left to produce it.
func (s *TodoServer) GetTodo(c *gin.Context, id string) {
	if _, ok := bffOwnerID(c); !ok {
		return
	}

	found, err := s.Service.GetTodo(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, todo.ErrNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, bffTodoNotFoundError)
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, toBFFTodo(found))
}

// UpdateTodo implements bffapi.ServerInterface — PATCH /api/bff/todos/{id}.
// milestone-4: the only field this endpoint still writes is title —
// status/assigneeId/priority/dueDate route through POST .../events
// instead (mirrors publicapi's own UpdateTodo doc comment exactly — same
// reasoning, same single write path). Internally still funnels through
// Append as a field_changed(title) event, so it carries the same
// clientRequestId (I19) and I18 permission check as every other write —
// I18 never actually restricts a title rename for either role, and an
// owner session always passes it unconditionally regardless.
func (s *TodoServer) UpdateTodo(c *gin.Context, id string) {
	actor, actorID, ok := bffPolicyActor(c)
	if !ok {
		return
	}

	var req bffapi.UpdateTodoRequest
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
		writeBFFAppendError(c, err)
		return
	}

	updated, err := s.Service.GetTodo(c.Request.Context(), id)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, toBFFTodo(updated))
}

// ListTodoEvents implements bffapi.ServerInterface —
// GET /api/bff/todos/{id}/events. This todo's own timeline, oldest first
// — mirrors publicapi's own ListTodoEvents exactly (same service method,
// same ordering). No cross-todo feed on this surface — that's
// GET /api/bff/activity, task-5's own separate endpoint.
func (s *TodoServer) ListTodoEvents(c *gin.Context, id string) {
	if _, ok := bffOwnerID(c); !ok {
		return
	}

	// GetTodo first so an unknown todo id is a genuine 404 rather than a
	// silently-empty event list (ListEvents alone can't distinguish "no
	// events yet" from "no such todo" — it would return an empty slice
	// either way).
	if _, err := s.Service.GetTodo(c.Request.Context(), id); err != nil {
		if errors.Is(err, todo.ErrNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, bffTodoNotFoundError)
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

	resp := bffapi.TodoEventList{Events: make([]bffapi.TodoEvent, 0, len(events))}
	for _, e := range events {
		event, err := toBFFEvent(e.Event, bffapi.ActivityActor{Handle: e.ActorHandle, Role: e.ActorRole})
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		resp.Events = append(resp.Events, event)
	}
	c.JSON(http.StatusOK, resp)
}

// bffValidStatuses is the fixed four-value set a status_changed event's
// `to` may name (DATA_MODEL.md) — mirrors publicapi's own validStatuses,
// checked here at the transport boundary rather than letting an arbitrary
// string reach Repo.UpdateStatus, which has no enum validation of its own.
var bffValidStatuses = map[string]todo.Status{
	string(todo.StatusOpen):       todo.StatusOpen,
	string(todo.StatusInProgress): todo.StatusInProgress,
	string(todo.StatusDone):       todo.StatusDone,
	string(todo.StatusClosed):     todo.StatusClosed,
}

// bffTodoFieldByWireName maps CreateTodoEventRequest's `field` value
// (camelCase, matching the Todo response's own JSON key names) to the
// TodoField the domain service expects — mirrors publicapi's own
// todoFieldByWireName.
var bffTodoFieldByWireName = map[string]todo.TodoField{
	"title":    todo.FieldTitle,
	"priority": todo.FieldPriority,
	"dueDate":  todo.FieldDueDate,
}

// CreateTodoEvent implements bffapi.ServerInterface —
// POST /api/bff/todos/{id}/events, I15's single write path exposed over
// HTTP on the owner-facing surface. Dispatches by req.Type into the
// matching todo.AppendInput shape entirely before ever calling
// s.Service.Append — mirrors publicapi's own CreateTodoEvent dispatch
// exactly, so an invalid or unsupported type (type: "created" included,
// GOAL.md/INVARIANTS.md I16) is rejected here, with nothing written,
// rather than reaching the service layer at all. Not a special case for
// "created" specifically: the switch below simply has no case that
// produces a WriteEventType-shaped AppendInput for it, the same way it
// has none for any other string it doesn't recognise (Done-when 14's own
// "genuinely rejected... not misrouted" requirement, verified
// independently of publicapi's own Done-when-13 proof by
// TestDoneWhen14_CreateTodoEvent_TypeCreatedRejected in
// todo_handler_test.go).
//
// status: closed genuinely succeeds when reached through this handler
// (I18 — this is the owner's own surface): bffPolicyActor above always
// resolves Role: "owner" on this surface (I12), and todo.Service's own
// can() passes an owner unconditionally, so no extra branch is needed
// here to grant it — the same permission check publicapi's handler also
// calls simply resolves differently given a different actor role.
func (s *TodoServer) CreateTodoEvent(c *gin.Context, id string) {
	actor, actorID, ok := bffPolicyActor(c)
	if !ok {
		return
	}

	var req bffapi.CreateTodoEventRequest
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
			c.AbortWithStatusJSON(http.StatusBadRequest, bffValidationErrorBody(`"body" is required for type: commented`, "body"))
			return
		}
		input.Type = todo.EventTypeCommented
		input.Comment = &todo.CommentInput{Body: *req.Body}

	case string(todo.EventTypeStatusChanged):
		if req.To == nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, bffValidationErrorBody(`"to" is required for type: status_changed`, "to"))
			return
		}
		status, ok := bffValidStatuses[*req.To]
		if !ok {
			c.AbortWithStatusJSON(http.StatusBadRequest, bffValidationErrorBody("unrecognised status value", "to"))
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
			c.AbortWithStatusJSON(http.StatusBadRequest, bffValidationErrorBody(`"field" is required for type: field_changed`, "field"))
			return
		}
		field, ok := bffTodoFieldByWireName[*req.Field]
		if !ok {
			c.AbortWithStatusJSON(http.StatusBadRequest, bffValidationErrorBody("unrecognised field name", "field"))
			return
		}

		change := &todo.FieldChangeInput{Field: field}
		switch field {
		case todo.FieldTitle:
			if req.To == nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, bffValidationErrorBody(`"to" is required for field: title`, "to"))
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
					c.AbortWithStatusJSON(http.StatusBadRequest, bffValidationErrorBody(`"to" must be an RFC3339 timestamp for field: dueDate`, "to"))
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
		c.AbortWithStatusJSON(http.StatusBadRequest, bffValidationErrorBody(`unrecognised or unsupported "type"`, "type"))
		return
	}

	event, err := s.Service.Append(c.Request.Context(), input)
	if err != nil {
		writeBFFAppendError(c, err)
		return
	}

	// The actor who just wrote this event is the session owner itself —
	// its handle/role are already resolved onto the gin context
	// (ActorFromContext), no extra lookup needed the way ListTodoEvents'
	// read path needs one per row. Role is always "owner" on this surface
	// (I12), matching bffPolicyActor's own comment above.
	actorUser, _ := ActorFromContext(c)
	bffEvent, err := toBFFEvent(event, bffapi.ActivityActor{Handle: actorUser.Handle, Role: actorUser.Role})
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusCreated, bffEvent)
}

// toBFFActivityItem converts one todo.TodoEventFeedRow (the cross-todo
// feed's own row shape, already joined to todos/users one layer down in
// internal/domain/todo) into the wire ActivityItem — mirrors toBFFEvent's
// payload-decode step exactly, plus the actor/todo context the per-todo
// timeline doesn't carry (`_contract/API.md`'s `GET /api/bff/activity`).
func toBFFActivityItem(row todo.TodoEventFeedRow) (bffapi.ActivityItem, error) {
	item := bffapi.ActivityItem{
		Id:        row.Event.ID,
		Seq:       row.Event.Seq,
		Type:      string(row.Event.Type),
		Actor:     bffapi.ActivityActor{Handle: row.ActorHandle, Role: row.ActorRole},
		Body:      row.Event.Body,
		CreatedAt: row.Event.CreatedAt,
		Todo:      bffapi.ActivityTodoRef{Id: row.Event.TodoID, Title: row.TodoTitle},
	}
	if row.Event.Payload != nil {
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(*row.Event.Payload), &payload); err != nil {
			return bffapi.ActivityItem{}, err
		}
		item.Payload = &payload
	}
	return item, nil
}

// ListActivity implements bffapi.ServerInterface — GET /api/bff/activity,
// task-5's own endpoint: a cursor over todo_events across every todo,
// newest first, joined to todos (title) and users (actor handle/role) —
// `_contract/API.md`. Owner-session only; there is no agent-facing
// equivalent on either surface (this type's own doc comment, mirrors
// my-task's `activity.list` having no REST counterpart).
//
// This is ruling 1's (GOAL.md) only real proof that todos are a genuinely
// shared collection rather than merely dual-paged: TestDoneWhen12_*
// (todo_handler_test.go) seeds a real agent identity through
// identity.Service.IssueAPIKeyForHandle (the same method cmd/issue-key's
// own `run` calls — task-5-report.md's own note on why this isn't the
// newBFFRouterForTwoOwnersWithKeys-shaped shortcut GOAL.md's "Test-fixture
// discipline" row warns against), has that agent act on a todo over the
// real Bearer-authenticated publicapi surface, then asserts this handler's
// response actually contains that event, attributed to the agent.
//
// Pagination mirrors my-task's own TaskQueryService.listActivity exactly:
// fetch limit+1 rows so an extra row present means there's a next page
// (hasMore), the (limit)th row's own (createdAt, id) becomes nextCursor,
// and the query's own cursor args stay nil for the first page — no
// separate "first page" branch needed, ListFeed/ListTodoEventsFeed
// already treat a nil cursor as "start from the newest row"
// (todo_events.sql's own sqlc.narg(cursor_created_at) IS NULL branch).
//
// cursorCreatedAtMs/cursorId are validated as a pair here (bff-openapi.yaml
// can declare each parameter's own type/range but not a cross-field "both
// or neither" rule) — everything else about `limit`'s own bounds is
// already enforced one layer up by the request validator before this
// handler is ever reached.
func (s *TodoServer) ListActivity(c *gin.Context, params bffapi.ListActivityParams) {
	if _, ok := bffOwnerID(c); !ok {
		return
	}

	limit := int64(bffActivityDefaultLimit)
	if params.Limit != nil {
		limit = int64(*params.Limit)
	}

	var cursorCreatedAt *time.Time
	var cursorID *string
	switch {
	case params.CursorCreatedAtMs != nil && params.CursorId != nil:
		t := time.UnixMilli(*params.CursorCreatedAtMs).UTC()
		cursorCreatedAt = &t
		cursorID = params.CursorId
	case params.CursorCreatedAtMs != nil || params.CursorId != nil:
		c.AbortWithStatusJSON(http.StatusBadRequest, bffValidationErrorBody(
			`"cursorCreatedAtMs" and "cursorId" must be supplied together`, "cursor"))
		return
	}

	// Fetch one extra row so an extra row present means there's a next
	// page (my-task's own hasMore check, task_events.ts:544-545) — the
	// (limit)th row's own (created_at, id) is what nextCursor names.
	rows, err := s.Service.ListFeed(c.Request.Context(), cursorCreatedAt, cursorID, limit+1)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	hasMore := int64(len(rows)) > limit
	if hasMore {
		rows = rows[:limit]
	}

	resp := bffapi.ActivityFeed{Items: make([]bffapi.ActivityItem, 0, len(rows))}
	for _, row := range rows {
		item, err := toBFFActivityItem(row)
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		resp.Items = append(resp.Items, item)
	}

	if hasMore {
		last := rows[len(rows)-1]
		resp.NextCursor = &bffapi.ActivityCursor{
			CreatedAtMs: last.Event.CreatedAt.UnixMilli(),
			Id:          last.Event.ID,
		}
	}

	c.JSON(http.StatusOK, resp)
}
