package bff

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mildronize/my-template/internal/domain/todo"
)

// --- wire types (bff-openapi.yaml's schemas, ticket-7) -----------------
//
// Replaces internal/bffapi's oapi-codegen-generated types (deleted this
// ticket) — mirrors contract.md's "File/package layout" for the /api/v1
// migration: these framework-agnostic response/request types move into
// this package itself, alongside the huma Input/Output structs that now
// produce/consume identical JSON. Same field names, JSON tags, and
// validation constraints (enum/minLength/maxLength) as bff-openapi.yaml
// declared before this ticket dumps a new one from these struct tags
// instead. Field names deliberately keep oapi-codegen's own `Id`-style
// casing rather than renaming to `ID` the way internal/transport/
// publicapi's ticket-6 migration did for /api/v1 — same JSON wire shape
// either way (the field names below are exactly what the JSON tags name,
// so the wire format is unaffected regardless), a lower-risk choice for
// this ticket's own test-file diff (every existing bffapi.Todo/.../
// TodoEvent/... field reference in this package's tests only needs the
// `bffapi.` qualifier dropped, not a field-name rename too).

// TodoPriority is Todo.priority's fixed four-value enum
// (bff-openapi.yaml).
type TodoPriority string

// TodoStatus is Todo.status's fixed four-value enum — milestone-4:
// replaces `done`, not an owner-editable table (GOAL.md).
type TodoStatus string

// Defines values for TodoStatus — bare names (not TodoStatus-prefixed),
// matching internal/bffapi's own oapi-codegen-generated constant names so
// every existing reference in this package's own tests only needs its
// "bffapi." qualifier dropped, not a rename (Testing Decisions: harness/
// import-path churn, not an assertion change).
const (
	Open       TodoStatus = "open"
	InProgress TodoStatus = "in_progress"
	Done       TodoStatus = "done"
	Closed     TodoStatus = "closed"
)

// ActivityActor is TodoEvent.actor/ActivityItem.actor's shape — carried so
// the UI can mark human vs agent (I20's sibling requirement, not itself an
// invariant — a rendering fact, not a safety one). Unlike
// internal/transport/publicapi's own TodoEvent.ActorHandle (a bare
// string — agents see WHO acted, not a role-based marker, per that
// package's own doc comment), this owner-only surface's whole reason for
// existing is the human/agent provenance mark, which needs role too.
type ActivityActor struct {
	Handle string `json:"handle"`
	Role   string `json:"role"`
}

// Todo defines model for Todo (bff-openapi.yaml).
type Todo struct {
	Id             string        `json:"id"`
	Title          string        `json:"title"`
	Status         TodoStatus    `json:"status" enum:"open,in_progress,done,closed"`
	AssigneeId     *string       `json:"assigneeId"`
	AssigneeHandle *string       `json:"assigneeHandle"`
	Priority       *TodoPriority `json:"priority" enum:"low,medium,high,urgent"`
	DueDate        *time.Time    `json:"dueDate"`
	CreatedBy      string        `json:"createdBy"`
	CreatedAt      time.Time     `json:"createdAt"`
	UpdatedAt      time.Time     `json:"updatedAt"`
}

// TodoList defines model for TodoList.
type TodoList struct {
	Todos []Todo `json:"todos"`
}

// TodoEvent defines model for TodoEvent (bff-openapi.yaml). Payload is
// map[string]any (contract.md's "Free-form fields" section) — it has no
// additionalProperties concept to configure, so it already accepts
// arbitrary JSON with no huma struct tag needed to preserve that
// permissiveness.
type TodoEvent struct {
	Id              string          `json:"id"`
	TodoId          string          `json:"todoId"`
	Seq             int64           `json:"seq"`
	ActorId         string          `json:"actorId"`
	Actor           ActivityActor   `json:"actor"`
	Type            string          `json:"type"`
	Payload         *map[string]any `json:"payload,omitempty"`
	Body            *string         `json:"body,omitempty"`
	ClientRequestId string          `json:"clientRequestId"`
	CreatedAt       time.Time       `json:"createdAt"`
}

// TodoEventList defines model for TodoEventList.
type TodoEventList struct {
	Events []TodoEvent `json:"events"`
}

// ActivityTodoRef is ActivityItem.todo's shape — the todo an activity
// event belongs to, enough to link/label it, not the full Todo shape.
type ActivityTodoRef struct {
	Id    string `json:"id"`
	Title string `json:"title"`
}

// ActivityCursor round-trips verbatim: the response's nextCursor and the
// request's cursorCreatedAtMs/cursorId query params name the same two
// values.
type ActivityCursor struct {
	CreatedAtMs int64  `json:"createdAtMs"`
	Id          string `json:"id"`
}

// ActivityItem is one todo_events row, shaped for the cross-todo feed —
// same underlying row TodoEvent (above) represents on the per-todo
// timeline, plus actor/todo context a single-todo timeline doesn't need
// to repeat.
type ActivityItem struct {
	Id        string          `json:"id"`
	Seq       int64           `json:"seq"`
	Type      string          `json:"type"`
	Actor     ActivityActor   `json:"actor"`
	Payload   *map[string]any `json:"payload,omitempty"`
	Body      *string         `json:"body,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
	Todo      ActivityTodoRef `json:"todo"`
}

// ActivityFeed is GET /api/bff/activity's response envelope. NextCursor
// stays `omitempty` (not a bare nullable pointer) — matches
// internal/bffapi's own generated field exactly: exhausted means the key
// is omitted from the response body, not present as JSON null. A test
// decoding this into a *ActivityCursor sees the same nil either way, so
// this is a wire-format detail, not a behavior one.
type ActivityFeed struct {
	Items      []ActivityItem  `json:"items"`
	NextCursor *ActivityCursor `json:"nextCursor,omitempty"`
}

// CreateTodoRequestPriority defines model for CreateTodoRequest.Priority.
type CreateTodoRequestPriority string

// CreateTodoRequest — bff-openapi.yaml declares `additionalProperties:
// false` explicitly, matching huma's own default for every input struct
// (contract.md's "Middleware mapping" table), so a stray `done`
// (milestone-1/2/3's removed field) is a huma-native 422 validation
// failure, not a value silently dropped — the accepted status delta
// contract.md's "Behavior note" documents (400 -> 422, this case is
// huma's own generic shape validation, not this package's own
// handler-level validation_error). No `status` field, deliberately: every
// created todo starts `open` regardless of what a caller might ask for.
type CreateTodoRequest struct {
	Title           string                     `json:"title" minLength:"1" maxLength:"200"`
	AssigneeId      *string                    `json:"assigneeId,omitempty"`
	Priority        *CreateTodoRequestPriority `json:"priority,omitempty" enum:"low,medium,high,urgent"`
	DueDate         *time.Time                 `json:"dueDate,omitempty"`
	ClientRequestId string                     `json:"clientRequestId" minLength:"1"`
}

// UpdateTodoRequest — milestone-4: `title` only, same
// additionalProperties: false default as CreateTodoRequest above
// (bff-openapi.yaml declares it explicitly here too).
type UpdateTodoRequest struct {
	Title           string `json:"title" minLength:"1" maxLength:"200"`
	ClientRequestId string `json:"clientRequestId" minLength:"1"`
}

// CreateTodoEventRequest — one shape covers all four client-postable
// `type` values; cross-field validation (e.g. `body` required for
// `commented`) is this file's own dispatch logic (below), not this
// schema's, so `type` stays a bare string, not a huma `enum` tag.
// Carries contract.md's `additionalProperties: "true"` escape hatch —
// ticket-7's own required check (contract.md: "this needs verifying
// against bff-openapi.yaml directly, not assumed from the public-API
// finding") confirms bff-openapi.yaml's CreateTodoEventRequest schema has
// the identical gap openapi.yaml's own version had (no
// `additionalProperties: false` declared at all, unlike
// CreateTodoRequest/UpdateTodoRequest above) — so this is the one place
// on this surface too where huma's own strict-by-default behavior would
// be a real behavior change if left unset.
type CreateTodoEventRequest struct {
	_               struct{} `json:"-" additionalProperties:"true"`
	Type            string   `json:"type"`
	ClientRequestId string   `json:"clientRequestId" minLength:"1"`
	Body            *string  `json:"body,omitempty"`
	To              *string  `json:"to,omitempty"`
	Field           *string  `json:"field,omitempty"`
}

// --- huma Input/Output structs ----------------------------------------

type ListTodosOutput struct{ Body TodoList }

type CreateTodoInput struct{ Body CreateTodoRequest }
type CreateTodoOutput struct{ Body Todo }

type GetTodoInput struct {
	ID string `path:"id" doc:"The todo's own id."`
}
type GetTodoOutput struct{ Body Todo }

type UpdateTodoInput struct {
	ID   string `path:"id" doc:"The todo's own id."`
	Body UpdateTodoRequest
}
type UpdateTodoOutput struct{ Body Todo }

type ListTodoEventsInput struct {
	ID string `path:"id" doc:"The todo's own id."`
}
type ListTodoEventsOutput struct{ Body TodoEventList }

type CreateTodoEventInput struct {
	ID   string `path:"id" doc:"The todo's own id."`
	Body CreateTodoEventRequest
}
type CreateTodoEventOutput struct{ Body TodoEvent }

// activityDefaultLimit mirrors my-task's own activity.list default
// (`~/gits/my-task/src/server/api/routers/activity.ts`:
// "limit: input.limit ?? 30") — applied here when the query omits `limit`
// entirely. The 1-100 bound itself is enforced one layer up, by
// ListActivityInput.Limit's own `minimum`/`maximum` tags (huma's own
// request validation) — this handler only ever sees either "omitted" or
// an already-in-range value.
const activityDefaultLimit = 30

// OptionalParam is huma's own documented pattern for a query/header/path
// parameter that must distinguish "not provided" from "provided as the
// zero value" (huma.ParamWrapper + huma.ParamReactor, huma.go's own doc
// comment on those two interfaces gives this exact generic type as the
// worked example) — needed here because huma v2.39.1 has no pointer
// support for form/header/path/query parameters at all (confirmed
// empirically: huma.Register panics at registration time, "pointers are
// not supported for form/header/path/query parameters", parseParamLocation
// in huma.go — the *int/*int64/*string fields this type originally used,
// mirroring the request-body pointer-field pattern CreateTodoRequest/
// CreateTodoEventRequest use elsewhere in this file, don't work for query
// parameters the way they do for a JSON body). ListActivityInput's
// CursorCreatedAtMs/CursorID need exactly this: the "must be supplied
// together or not at all" check below only makes sense if "not supplied"
// is distinguishable from "supplied as 0"/"" (see huma_test.go, this
// module's own test suite, for the same pattern in Huma's own tests —
// including the Schema method below, without which huma.SchemaFromField
// would generate a schema for the OptionalParam[T] wrapper struct itself
// (an object with Value/IsSet properties) instead of for T, silently
// mismatching every request that actually supplies the parameter:
// confirmed empirically building this ticket, `?limit=2` failed
// validation with a bare "query.limit" hint and no further detail until
// this method was added).
type OptionalParam[T any] struct {
	Value T
	IsSet bool
}

// Schema satisfies huma.SchemaProvider — huma's own schema.go calls this
// instead of reflecting over OptionalParam[T]'s two fields, so the
// generated OpenAPI schema (and this field's own minimum/maximum tags,
// applied on top afterward by SchemaFromField) describes T, not the
// wrapper.
func (o OptionalParam[T]) Schema(r huma.Registry) *huma.Schema {
	return huma.SchemaFromType(r, reflect.TypeOf(o.Value))
}

func (o *OptionalParam[T]) Receiver() reflect.Value {
	return reflect.ValueOf(o).Elem().Field(0)
}

func (o *OptionalParam[T]) OnParamSet(isSet bool, _ any) {
	o.IsSet = isSet
}

// ListActivityInput is ListActivity's huma.Register input —
// bff-openapi.yaml's `limit`/`cursorCreatedAtMs`/`cursorId` query
// parameters, all optional.
type ListActivityInput struct {
	Limit             OptionalParam[int]    `query:"limit" minimum:"1" maximum:"100"`
	CursorCreatedAtMs OptionalParam[int64]  `query:"cursorCreatedAtMs"`
	CursorID          OptionalParam[string] `query:"cursorId"`
}
type ListActivityOutput struct{ Body ActivityFeed }

// --- conversions --------------------------------------------------------

func toBFFTodo(t todo.Todo) Todo {
	var priority *TodoPriority
	if t.Priority != nil {
		p := TodoPriority(*t.Priority)
		priority = &p
	}
	return Todo{
		Id:             t.ID,
		Title:          t.Title,
		Status:         TodoStatus(t.Status),
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
// generic object TodoEvent's own Payload field expects — mirrors
// internal/transport/publicapi's own toTodoEvent, the one place in this
// file that touches encoding/json directly, since todo.Service/Repo
// already store it pre-marshalled. actor is ActivityActor
// ({handle, role}) — the exact same shape toBFFActivityItem below already
// gives ActivityItem.actor, on purpose (this owner-only surface's whole
// reason for existing is the human/agent provenance mark, which needs
// role, not just a name — contrast publicapi's own toTodoEvent, which
// gives an agent caller only a handle, no role).
func toBFFEvent(e todo.TodoEvent, actor ActivityActor) (TodoEvent, error) {
	event := TodoEvent{
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
		var payload map[string]any
		if err := json.Unmarshal([]byte(*e.Payload), &payload); err != nil {
			return TodoEvent{}, err
		}
		event.Payload = &payload
	}
	return event, nil
}

// toBFFActivityItem converts one todo.TodoEventFeedRow (the cross-todo
// feed's own row shape, already joined to todos/users one layer down in
// internal/domain/todo) into the wire ActivityItem — mirrors toBFFEvent's
// payload-decode step exactly, plus the actor/todo context the per-todo
// timeline doesn't carry.
func toBFFActivityItem(row todo.TodoEventFeedRow) (ActivityItem, error) {
	item := ActivityItem{
		Id:        row.Event.ID,
		Seq:       row.Event.Seq,
		Type:      string(row.Event.Type),
		Actor:     ActivityActor{Handle: row.ActorHandle, Role: row.ActorRole},
		Body:      row.Event.Body,
		CreatedAt: row.Event.CreatedAt,
		Todo:      ActivityTodoRef{Id: row.Event.TodoID, Title: row.TodoTitle},
	}
	if row.Event.Payload != nil {
		var payload map[string]any
		if err := json.Unmarshal([]byte(*row.Event.Payload), &payload); err != nil {
			return ActivityItem{}, err
		}
		item.Payload = &payload
	}
	return item, nil
}

// --- errors --------------------------------------------------------------

var todoNotFoundMessage = "no such todo"

// invalidTransitionMessage/-Hint are I18's own rejection text — mirrors
// internal/transport/publicapi's own identical constants. Unreachable
// through this surface in practice (an owner session always passes I18's
// can() unconditionally, I12), but kept correct rather than left as
// dead-but-wrong defensive code: if this surface's own reachability
// assumption ever changes, it should already say the right thing.
const (
	invalidTransitionMessage = "Agents cannot move a task into the closed group"
	invalidTransitionHint    = "Ask the owner to close this task."
)

// mapAppendError maps Append/CreateTodo's error results the way every
// operation below needs — mirrors internal/transport/publicapi's own
// mapAppendError exactly: todo.ErrNotFound -> 404, todo.ErrForbidden
// (I18's permission refusal) -> 403 invalid_transition with a hint,
// todo.ErrUnknownAssignee (an `assigned` event whose "to" id does not
// resolve to any real user) -> 400 validation_error via this package's
// own apiError, anything else -> 500.
func mapAppendError(err error) error {
	if errors.Is(err, todo.ErrNotFound) {
		return huma.Error404NotFound(todoNotFoundMessage)
	}
	if errors.Is(err, todo.ErrForbidden) {
		return huma.Error403Forbidden(invalidTransitionMessage, &huma.ErrorDetail{Location: invalidTransitionHint})
	}
	if errors.Is(err, todo.ErrUnknownAssignee) {
		return newValidationError("unknown assignee id", "to")
	}
	return internalServerError()
}

// policyActorFor converts the actorFromContext-resolved identity.User down
// to todo.PolicyActor (I18) at this transport boundary, and returns its id
// alongside — mirrors internal/transport/publicapi's own policyActorFor
// exactly. On this surface Role is always "owner" (I12: a BFF session can
// never resolve to role="agent"), so can() always permits the write; the
// dispatch/error handling shape below is still shared with publicapi's,
// rather than special-cased, so the two handlers stay structurally
// identical wherever the contract calls for it.
func policyActorFor(ctx context.Context) (todo.PolicyActor, string, error) {
	user, err := actorFromContext(ctx)
	if err != nil {
		return todo.PolicyActor{}, "", err
	}
	return todo.PolicyActor{Role: user.Role}, user.ID, nil
}

// --- registration --------------------------------------------------------

// validStatuses is the fixed four-value set a status_changed event's `to`
// may name (DATA_MODEL.md) — mirrors internal/transport/publicapi's own
// validStatuses.
var validStatuses = map[string]todo.Status{
	string(todo.StatusOpen):       todo.StatusOpen,
	string(todo.StatusInProgress): todo.StatusInProgress,
	string(todo.StatusDone):       todo.StatusDone,
	string(todo.StatusClosed):     todo.StatusClosed,
}

// todoFieldByWireName maps CreateTodoEventRequest's `field` value
// (camelCase, matching the Todo response's own JSON key names) to the
// TodoField the domain service expects — mirrors internal/transport/
// publicapi's own todoFieldByWireName.
var todoFieldByWireName = map[string]todo.TodoField{
	"title":    todo.FieldTitle,
	"priority": todo.FieldPriority,
	"dueDate":  todo.FieldDueDate,
}

// RegisterTodoRoutes registers every todo CRUD/events/activity operation
// on api (ticket-7), replacing internal/bffapi.ServerInterface's
// generated-interface registration for this domain's subset. milestone-4:
// todos are a shared collection (GOAL.md's Ownership model decision) —
// every operation below (except ListActivity, this surface's own,
// task-5) reads/writes across every todo, not scoped to the caller (I3 no
// longer applies to this domain). There is no DeleteTodo operation at all
// (GOAL.md's "DELETE removed" decision) — so DELETE /api/bff/todos/{id}
// is a genuine 404 (no route), not a 405. `status: closed` genuinely
// succeeds through this surface's CreateTodoEvent (I18 — this is the
// owner's own surface; contrast internal/transport/publicapi's own
// CreateTodoEvent, which rejects the same write for a Bearer-authenticated
// agent). api must already have humamw.RequireSession attached
// (cmd/server/main.go).
func RegisterTodoRoutes(api huma.API, svc *todo.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "listTodos",
		Method:      http.MethodGet,
		Path:        "/api/bff/todos",
		Summary:     "List every todo.",
		Errors:      []int{http.StatusUnauthorized},
	}, func(ctx context.Context, _ *struct{}) (*ListTodosOutput, error) {
		if _, err := actorFromContext(ctx); err != nil {
			return nil, err
		}
		todos, err := svc.ListTodos(ctx)
		if err != nil {
			return nil, internalServerError()
		}
		resp := TodoList{Todos: make([]Todo, 0, len(todos))}
		for _, t := range todos {
			resp.Todos = append(resp.Todos, toBFFTodo(t))
		}
		return &ListTodosOutput{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "createTodo",
		Method:        http.MethodPost,
		Path:          "/api/bff/todos",
		Summary:       "Create a todo, attributed to the caller.",
		Description:   "`createdBy` is always the resolved session owner — never accepted from the body (I1, same rule as the public API, different auth mechanism) — but is attribution only, never access-scoping (GOAL.md). `status` always starts `open`.",
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusBadRequest, http.StatusUnauthorized},
	}, func(ctx context.Context, input *CreateTodoInput) (*CreateTodoOutput, error) {
		user, err := actorFromContext(ctx)
		if err != nil {
			return nil, err
		}

		req := input.Body
		createInput := todo.CreateInput{
			Title:           req.Title,
			AssigneeID:      req.AssigneeId,
			DueDate:         req.DueDate,
			ClientRequestID: req.ClientRequestId,
		}
		if req.Priority != nil {
			p := string(*req.Priority)
			createInput.Priority = &p
		}

		created, err := svc.CreateTodo(ctx, user.ID, createInput)
		if err != nil {
			return nil, internalServerError()
		}
		return &CreateTodoOutput{Body: toBFFTodo(created)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "getTodo",
		Method:      http.MethodGet,
		Path:        "/api/bff/todos/{id}",
		Summary:     "Get a todo by id.",
		Description: "milestone-4: every authenticated actor may read every todo (I3 no longer applies to this domain) — an unknown id is not_found, there is no \"wrong owner\" case left to produce it.",
		Errors:      []int{http.StatusUnauthorized, http.StatusNotFound},
	}, func(ctx context.Context, input *GetTodoInput) (*GetTodoOutput, error) {
		if _, err := actorFromContext(ctx); err != nil {
			return nil, err
		}
		found, err := svc.GetTodo(ctx, input.ID)
		if err != nil {
			if errors.Is(err, todo.ErrNotFound) {
				return nil, huma.Error404NotFound(todoNotFoundMessage)
			}
			return nil, internalServerError()
		}
		return &GetTodoOutput{Body: toBFFTodo(found)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "updateTodo",
		Method:      http.MethodPatch,
		Path:        "/api/bff/todos/{id}",
		Summary:     "Rename a todo.",
		Description: "milestone-4: the only field this endpoint still writes is title — status/assigneeId/priority/dueDate changes route through POST .../events instead.",
		Errors:      []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound},
	}, func(ctx context.Context, input *UpdateTodoInput) (*UpdateTodoOutput, error) {
		actor, actorID, err := policyActorFor(ctx)
		if err != nil {
			return nil, err
		}

		title := input.Body.Title
		_, err = svc.Append(ctx, todo.AppendInput{
			TodoID:          input.ID,
			Actor:           actor,
			ActorID:         actorID,
			ClientRequestID: input.Body.ClientRequestId,
			Type:            todo.EventTypeFieldChanged,
			FieldChange:     &todo.FieldChangeInput{Field: todo.FieldTitle, Title: &title},
		})
		if err != nil {
			return nil, mapAppendError(err)
		}

		updated, err := svc.GetTodo(ctx, input.ID)
		if err != nil {
			return nil, internalServerError()
		}
		return &UpdateTodoOutput{Body: toBFFTodo(updated)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "listTodoEvents",
		Method:      http.MethodGet,
		Path:        "/api/bff/todos/{id}/events",
		Summary:     "This todo's own timeline.",
		Description: "Oldest first. This todo's own timeline only; the cross-todo feed is GET /api/bff/activity, a separate endpoint.",
		Errors:      []int{http.StatusUnauthorized, http.StatusNotFound},
	}, func(ctx context.Context, input *ListTodoEventsInput) (*ListTodoEventsOutput, error) {
		if _, err := actorFromContext(ctx); err != nil {
			return nil, err
		}

		// GetTodo first so an unknown todo id is a genuine 404 rather than a
		// silently-empty event list.
		if _, err := svc.GetTodo(ctx, input.ID); err != nil {
			if errors.Is(err, todo.ErrNotFound) {
				return nil, huma.Error404NotFound(todoNotFoundMessage)
			}
			return nil, internalServerError()
		}

		events, err := svc.ListEvents(ctx, input.ID)
		if err != nil {
			return nil, internalServerError()
		}

		resp := TodoEventList{Events: make([]TodoEvent, 0, len(events))}
		for _, e := range events {
			bffEvent, err := toBFFEvent(e.Event, ActivityActor{Handle: e.ActorHandle, Role: e.ActorRole})
			if err != nil {
				return nil, internalServerError()
			}
			resp.Events = append(resp.Events, bffEvent)
		}
		return &ListTodoEventsOutput{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "createTodoEvent",
		Method:        http.MethodPost,
		Path:          "/api/bff/todos/{id}/events",
		Summary:       "Append an event to this todo's timeline (I15's single write path).",
		Description:   "**`status: closed` succeeds on this surface** (I18 — this is the owner's own surface; contrast with the public API, where a Bearer-authenticated agent caller is rejected).",
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound},
	}, func(ctx context.Context, input *CreateTodoEventInput) (*CreateTodoEventOutput, error) {
		actor, actorID, err := policyActorFor(ctx)
		if err != nil {
			return nil, err
		}

		req := input.Body
		appendInput := todo.AppendInput{
			TodoID:          input.ID,
			Actor:           actor,
			ActorID:         actorID,
			ClientRequestID: req.ClientRequestId,
		}

		switch req.Type {
		case string(todo.EventTypeCommented):
			if req.Body == nil {
				return nil, newValidationError(`"body" is required for type: commented`, "body")
			}
			appendInput.Type = todo.EventTypeCommented
			appendInput.Comment = &todo.CommentInput{Body: *req.Body}

		case string(todo.EventTypeStatusChanged):
			if req.To == nil {
				return nil, newValidationError(`"to" is required for type: status_changed`, "to")
			}
			status, ok := validStatuses[*req.To]
			if !ok {
				return nil, newValidationError("unrecognised status value", "to")
			}
			appendInput.Type = todo.EventTypeStatusChanged
			appendInput.StatusChange = &todo.StatusChangeInput{ToStatus: status}

		case string(todo.EventTypeAssigned):
			// req.To == nil covers both an omitted key and an explicit
			// `"to": null` — both mean "unassign", per _contract/API.md.
			appendInput.Type = todo.EventTypeAssigned
			appendInput.Assignment = &todo.AssignmentInput{ToAssigneeID: req.To}

		case string(todo.EventTypeFieldChanged):
			if req.Field == nil {
				return nil, newValidationError(`"field" is required for type: field_changed`, "field")
			}
			field, ok := todoFieldByWireName[*req.Field]
			if !ok {
				return nil, newValidationError("unrecognised field name", "field")
			}

			change := &todo.FieldChangeInput{Field: field}
			switch field {
			case todo.FieldTitle:
				if req.To == nil {
					return nil, newValidationError(`"to" is required for field: title`, "to")
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
					parsed, parseErr := time.Parse(time.RFC3339, *req.To)
					if parseErr != nil {
						return nil, newValidationError(`"to" must be an RFC3339 timestamp for field: dueDate`, "to")
					}
					change.DueDate = &parsed
				}
			}
			appendInput.Type = todo.EventTypeFieldChanged
			appendInput.FieldChange = change

		default:
			// Covers type: "created" (I16 — never client-specifiable) and any
			// other unrecognised string alike — deliberately the same path,
			// not a special case for "created".
			return nil, newValidationError(`unrecognised or unsupported "type"`, "type")
		}

		event, err := svc.Append(ctx, appendInput)
		if err != nil {
			return nil, mapAppendError(err)
		}

		// The actor who just wrote this event is the session owner itself —
		// its handle/role are already resolved onto ctx (actorFromContext),
		// no extra lookup needed the way ListTodoEvents' read path needs one
		// per row. Role is always "owner" on this surface (I12).
		actorUser, _ := actorFromContext(ctx)
		bffEvent, err := toBFFEvent(event, ActivityActor{Handle: actorUser.Handle, Role: actorUser.Role})
		if err != nil {
			return nil, internalServerError()
		}
		return &CreateTodoEventOutput{Body: bffEvent}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "listActivity",
		Method:      http.MethodGet,
		Path:        "/api/bff/activity",
		Summary:     "The cross-todo activity feed — every event, across every todo, newest first.",
		Description: "A cursor over todo_events across every todo, newest first, joined to todos (title) and users (actor handle/role). Owner-session only; there is no agent-facing equivalent on either surface.",
		Errors:      []int{http.StatusBadRequest, http.StatusUnauthorized},
	}, func(ctx context.Context, input *ListActivityInput) (*ListActivityOutput, error) {
		if _, err := actorFromContext(ctx); err != nil {
			return nil, err
		}

		limit := int64(activityDefaultLimit)
		if input.Limit.IsSet {
			limit = int64(input.Limit.Value)
		}

		var cursorCreatedAt *time.Time
		var cursorID *string
		switch {
		case input.CursorCreatedAtMs.IsSet && input.CursorID.IsSet:
			t := time.UnixMilli(input.CursorCreatedAtMs.Value).UTC()
			cursorCreatedAt = &t
			cursorID = &input.CursorID.Value
		case input.CursorCreatedAtMs.IsSet || input.CursorID.IsSet:
			return nil, newValidationError(`"cursorCreatedAtMs" and "cursorId" must be supplied together`, "cursor")
		}

		// Fetch one extra row so an extra row present means there's a next
		// page (my-task's own hasMore check) — the (limit)th row's own
		// (created_at, id) is what nextCursor names.
		rows, err := svc.ListFeed(ctx, cursorCreatedAt, cursorID, limit+1)
		if err != nil {
			return nil, internalServerError()
		}

		hasMore := int64(len(rows)) > limit
		if hasMore {
			rows = rows[:limit]
		}

		resp := ActivityFeed{Items: make([]ActivityItem, 0, len(rows))}
		for _, row := range rows {
			item, err := toBFFActivityItem(row)
			if err != nil {
				return nil, internalServerError()
			}
			resp.Items = append(resp.Items, item)
		}

		if hasMore {
			last := rows[len(rows)-1]
			resp.NextCursor = &ActivityCursor{
				CreatedAtMs: last.Event.CreatedAt.UnixMilli(),
				Id:          last.Event.ID,
			}
		}

		return &ListActivityOutput{Body: resp}, nil
	})
}
