package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mildronize/my-template/internal/domain/todo"
)

// --- wire types (openapi.yaml's schemas, ticket-6) -------------------
//
// Replaces internal/api's oapi-codegen-generated types (deleted this
// ticket) — contract.md's "File/package layout": these framework-agnostic
// response/request types move into this package itself, alongside the
// huma Input/Output structs that now produce/consume identical JSON. Same
// field names, JSON tags, and validation constraints (enum/minLength/
// maxLength) as openapi.yaml declared before this ticket dumps a new one
// from these struct tags instead.

// TodoPriority is Todo.priority's fixed four-value enum
// (openapi.yaml:300-303).
type TodoPriority string

// TodoStatus is Todo.status's fixed four-value enum — milestone-4:
// replaces `done`, not an owner-editable table (GOAL.md).
type TodoStatus string

// Defines values for TodoStatus — bare names (not TodoStatus-prefixed),
// matching internal/api's own oapi-codegen-generated constant names so
// every existing reference in this package's own tests only needs its
// "api." qualifier dropped, not a rename (Testing Decisions: harness/
// import-path churn, not an assertion change).
const (
	Open       TodoStatus = "open"
	InProgress TodoStatus = "in_progress"
	Done       TodoStatus = "done"
	Closed     TodoStatus = "closed"
)

// Todo defines model for Todo (openapi.yaml:255-318).
type Todo struct {
	ID             string        `json:"id"`
	Title          string        `json:"title"`
	Status         TodoStatus    `json:"status" enum:"open,in_progress,done,closed"`
	AssigneeID     *string       `json:"assigneeId"`
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

// TodoEvent defines model for TodoEvent (openapi.yaml:327-387). Payload
// is map[string]any (contract.md's "Free-form fields" section) — it has
// no additionalProperties concept to configure, so it already accepts
// arbitrary JSON with no huma struct tag needed to preserve that
// permissiveness.
type TodoEvent struct {
	ID              string          `json:"id"`
	TodoID          string          `json:"todoId"`
	Seq             int64           `json:"seq"`
	ActorID         string          `json:"actorId"`
	ActorHandle     string          `json:"actorHandle"`
	Type            string          `json:"type"`
	Payload         *map[string]any `json:"payload,omitempty"`
	Body            *string         `json:"body,omitempty"`
	ClientRequestID string          `json:"clientRequestId"`
	CreatedAt       time.Time       `json:"createdAt"`
}

// TodoEventList defines model for TodoEventList.
type TodoEventList struct {
	Events []TodoEvent `json:"events"`
}

// CreateTodoRequestPriority defines model for CreateTodoRequest.Priority.
type CreateTodoRequestPriority string

// CreateTodoRequest — huma defaults every struct to
// additionalProperties: false (contract.md's "Middleware mapping" table),
// so a stray `done` (milestone-1/2/3's removed field) is now a huma-native
// 422 validation failure, not a value silently dropped — the accepted
// status delta contract.md's "Behavior note" documents (400 -> 422, this
// case is huma's own generic shape validation, not this package's own
// handler-level validation_error). No `status` field, deliberately: every
// created todo starts `open` regardless of what a caller might ask for.
type CreateTodoRequest struct {
	Title           string                     `json:"title" minLength:"1" maxLength:"200"`
	AssigneeID      *string                    `json:"assigneeId,omitempty"`
	Priority        *CreateTodoRequestPriority `json:"priority,omitempty" enum:"low,medium,high,urgent"`
	DueDate         *time.Time                 `json:"dueDate,omitempty"`
	ClientRequestID string                     `json:"clientRequestId" minLength:"1"`
}

// UpdateTodoRequest — milestone-4: `title` only, same
// additionalProperties: false default as CreateTodoRequest above.
type UpdateTodoRequest struct {
	Title           string `json:"title" minLength:"1" maxLength:"200"`
	ClientRequestID string `json:"clientRequestId" minLength:"1"`
}

// CreateTodoEventRequest — one shape covers all four client-postable
// `type` values; cross-field validation (e.g. `body` required for
// `commented`) is this file's own dispatch logic (below), not this
// schema's, so `type` stays a bare string, not a huma `enum` tag — an
// unrecognised value still needs to reach the dispatch switch below to be
// rejected there (I16), not bounced earlier by schema validation with a
// different code/status. Carries contract.md's `additionalProperties:
// "true"` escape hatch — the one request schema today that is NOT
// additionalProperties:false (openapi.yaml:477-501 declares no such
// restriction), so this is the one place huma's own strict-by-default
// behavior would be a real behavior change if left unset.
type CreateTodoEventRequest struct {
	_               struct{} `json:"-" additionalProperties:"true"`
	Type            string   `json:"type"`
	ClientRequestID string   `json:"clientRequestId" minLength:"1"`
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

// --- conversions --------------------------------------------------------

func toTodo(t todo.Todo) Todo {
	var priority *TodoPriority
	if t.Priority != nil {
		p := TodoPriority(*t.Priority)
		priority = &p
	}
	return Todo{
		ID:             t.ID,
		Title:          t.Title,
		Status:         TodoStatus(t.Status),
		AssigneeID:     t.AssigneeID,
		AssigneeHandle: t.AssigneeHandle,
		Priority:       priority,
		DueDate:        t.DueDate,
		CreatedBy:      t.CreatedBy,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
	}
}

// toTodoEvent decodes a todo.TodoEvent's raw-JSON Payload string into the
// generic object TodoEvent's own Payload field expects (_contract/API.md's
// activity-feed example shows payload as a JSON object) — the one place in
// this file that touches encoding/json directly, since todo.Service/Repo
// already store it pre-marshalled.
//
// actorHandle is a plain string, not the bff surface's {handle, role}
// object — deliberately asymmetric (see this function's pre-ticket-6
// doc comment history for the full my-task-comparison reasoning, carried
// forward unchanged): agents see WHO acted (a name), not a role-based
// human/agent marker.
func toTodoEvent(e todo.TodoEvent, actorHandle string) (TodoEvent, error) {
	apiEvent := TodoEvent{
		ID:              e.ID,
		TodoID:          e.TodoID,
		Seq:             e.Seq,
		ActorID:         e.ActorID,
		ActorHandle:     actorHandle,
		Type:            string(e.Type),
		Body:            e.Body,
		ClientRequestID: e.ClientRequestID,
		CreatedAt:       e.CreatedAt,
	}
	if e.Payload != nil {
		var payload map[string]any
		if err := json.Unmarshal([]byte(*e.Payload), &payload); err != nil {
			return TodoEvent{}, err
		}
		apiEvent.Payload = &payload
	}
	return apiEvent, nil
}

// --- errors --------------------------------------------------------------

var todoNotFoundMessage = "no such todo"

// invalidTransitionMessage/-Hint are I18's own rejection text: an
// agent-authenticated caller attempting to move a todo to status: closed.
// Matches my-task's own equivalent check's text exactly, not a paraphrase
// (see this file's pre-ticket-6 history for the full reasoning this
// carries forward unchanged).
const (
	invalidTransitionMessage = "Agents cannot move a task into the closed group"
	invalidTransitionHint    = "Ask the owner to close this task."
)

// mapAppendError maps Append/CreateTodo's error results the way every
// operation below needs: todo.ErrNotFound -> 404, todo.ErrForbidden
// (I18's permission refusal) -> 403 invalid_transition with a hint
// (via huma.Error403Forbidden + a huma.ErrorDetail carrying the hint as
// its Location — humamw's InstallErrorEnvelope reads that back out as
// Hint, contract.md's "Error envelope" section), todo.ErrUnknownAssignee
// (an `assigned` event whose "to" id does not resolve to any real user)
// -> 400 validation_error via this package's own apiError (not huma's
// native 422 — this is a handler-level check CreateTodoEventRequest's own
// schema deliberately doesn't perform, not a generic shape-validation
// failure), anything else -> 500.
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
// to todo.PolicyActor (I18) at this transport boundary — the same
// role-only shape permission.go's can() expects, deliberately not
// identity.User itself.
func policyActorFor(ctx context.Context) (todo.PolicyActor, string, error) {
	user, err := actorFromContext(ctx)
	if err != nil {
		return todo.PolicyActor{}, "", err
	}
	return todo.PolicyActor{Role: user.Role}, user.ID, nil
}

// --- registration --------------------------------------------------------

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

// todoFieldByWireName maps CreateTodoEventRequest's `field` value
// (camelCase, matching the Todo response's own JSON key names —
// "dueDate", not the internal snake_case todo.FieldDueDate constant) to
// the TodoField the domain service expects.
var todoFieldByWireName = map[string]todo.TodoField{
	"title":    todo.FieldTitle,
	"priority": todo.FieldPriority,
	"dueDate":  todo.FieldDueDate,
}

// RegisterTodoRoutes registers every todo CRUD/events operation on api
// (ticket-6), replacing internal/api.RegisterHandlers' single-call
// generated registration for this domain's subset. milestone-4: todos are
// a shared collection (GOAL.md's Ownership model decision) — every
// operation below reads/writes across every todo, not scoped to the
// caller (I3 no longer applies to this domain). There is no
// DeleteTodo operation at all (GOAL.md's "DELETE removed" decision) — so
// DELETE /api/v1/todos/{id} is a genuine 404 (no route), not a 405. api
// must already have humamw.RequireActor attached (cmd/server/main.go).
func RegisterTodoRoutes(api huma.API, svc *todo.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "listTodos",
		Method:      http.MethodGet,
		Path:        "/api/v1/todos",
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
			resp.Todos = append(resp.Todos, toTodo(t))
		}
		return &ListTodosOutput{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "createTodo",
		Method:        http.MethodPost,
		Path:          "/api/v1/todos",
		Summary:       "Create a todo, attributed to the caller.",
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
			AssigneeID:      req.AssigneeID,
			DueDate:         req.DueDate,
			ClientRequestID: req.ClientRequestID,
		}
		if req.Priority != nil {
			p := string(*req.Priority)
			createInput.Priority = &p
		}

		created, err := svc.CreateTodo(ctx, user.ID, createInput)
		if err != nil {
			return nil, internalServerError()
		}
		return &CreateTodoOutput{Body: toTodo(created)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "getTodo",
		Method:      http.MethodGet,
		Path:        "/api/v1/todos/{id}",
		Summary:     "Get a todo by id.",
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
		return &GetTodoOutput{Body: toTodo(found)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "updateTodo",
		Method:      http.MethodPatch,
		Path:        "/api/v1/todos/{id}",
		Summary:     "Rename a todo.",
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
			ClientRequestID: input.Body.ClientRequestID,
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
		return &UpdateTodoOutput{Body: toTodo(updated)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "listTodoEvents",
		Method:      http.MethodGet,
		Path:        "/api/v1/todos/{id}/events",
		Summary:     "This todo's own timeline.",
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
			apiEvent, err := toTodoEvent(e.Event, e.ActorHandle)
			if err != nil {
				return nil, internalServerError()
			}
			resp.Events = append(resp.Events, apiEvent)
		}
		return &ListTodoEventsOutput{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "createTodoEvent",
		Method:        http.MethodPost,
		Path:          "/api/v1/todos/{id}/events",
		Summary:       "Append an event to this todo's timeline (I15's single write path).",
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
			ClientRequestID: req.ClientRequestID,
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
			// Covers type: "created" (I16 — never client-specifiable, see
			// todo.WriteEventType's own doc comment) and any other
			// unrecognised string alike — deliberately the same path, not a
			// special case for "created".
			return nil, newValidationError(`unrecognised or unsupported "type"`, "type")
		}

		event, err := svc.Append(ctx, appendInput)
		if err != nil {
			return nil, mapAppendError(err)
		}

		// The actor who just wrote this event is the caller itself — its
		// handle is already resolved onto ctx (actorFromContext), no extra
		// lookup needed the way ListTodoEvents' read path needs one per row.
		actorUser, _ := actorFromContext(ctx)
		apiEvent, err := toTodoEvent(event, actorUser.Handle)
		if err != nil {
			return nil, internalServerError()
		}
		return &CreateTodoEventOutput{Body: apiEvent}, nil
	})
}
