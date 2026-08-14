package todo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrForbidden is Append's own sentinel for I18's permission refusal — an
// agent attempting status:closed. Deliberately distinct from ErrNotFound
// (I3's "absence, not permission" framing is specific to ownership
// scoping, which this domain no longer has; a permission refusal here is
// a real, named "you may not do this", not a manufactured 404).
//
// Maps to a real 403 (`invalid_transition`, with a hint), not the
// generic 401 unauthorized every credential failure produces (I5) — a
// post-launch correction, not the original design. The agent here is
// genuinely authenticated; a 401 would read as "your key is bad," which
// sends it to rotate a credential that was never the problem, instead of
// asking the owner to close the todo (transport/publicapi/todo_handler.go,
// transport/bff/todo_handler.go). Matches my-task's own
// `unauthorized` -> distinct `invalid_transition` split exactly
// (`~/gits/my-task/src/server/api/v1/errors.ts:130`), which this
// project's first pass at I18 narrowed away by reusing 401 for both
// "who are you" and "you may not do this" — see INVARIANTS.md's I18
// entry for the reasoning in full.
var ErrForbidden = errors.New("todo: forbidden")

// Repository is the subset of Repo's methods Service depends on. Declared
// here (not in repo.go) so tests can supply a fake without a real
// database — repo.go's *Repo satisfies this interface structurally, with
// no import of internal/db required on this side (mirrors
// internal/identity's UserRepo/APIKeyRepo split).
//
// No Delete method exists anywhere on this interface (GOAL.md's "DELETE
// removed" decision, mirroring my-task's I12) — there is nothing for a
// future handler to accidentally wire up.
type Repository interface {
	WithinTx(ctx context.Context, fn func(tx Repository) error) error

	List(ctx context.Context) ([]Todo, error)
	Create(ctx context.Context, createdBy, title string, params CreateParams) (Todo, error)
	GetByID(ctx context.Context, id string) (Todo, error)
	UpdateTitle(ctx context.Context, id, title string) (Todo, error)
	UpdateStatus(ctx context.Context, id string, status Status) (Todo, error)
	UpdateAssignee(ctx context.Context, id string, assigneeID *string) (Todo, error)
	UpdatePriority(ctx context.Context, id string, priority *string) (Todo, error)
	UpdateDueDate(ctx context.Context, id string, dueDate *time.Time) (Todo, error)

	GetEventByClientRequestID(ctx context.Context, clientRequestID string) (TodoEvent, error)
	InsertEvent(ctx context.Context, todoID, actorID string, eventType EventType, payload, body *string, clientRequestID string) (TodoEvent, error)
	ListEventsByTodoID(ctx context.Context, todoID string) ([]TodoEventWithActor, error)
	ListEventsFeed(ctx context.Context, cursorCreatedAt *time.Time, cursorID *string, limit int64) ([]TodoEventFeedRow, error)

	// ResolveUserHandle resolves a user id to its handle — milestone-4
	// fix-round (handle-exposure), used by Append's EventTypeAssigned case
	// below to bake a {id, handle} snapshot into the `assigned` event's
	// payload at write time.
	ResolveUserHandle(ctx context.Context, id string) (string, error)
}

// Service implements the todo domain contract from _contract/API.md and
// _contract/INVARIANTS.md (I15-I19) on top of a Repository. This package
// never resolves an actor itself (I4) — every method below takes the
// already-resolved actor id/role as plain arguments, supplied by
// whichever transport surface (internal/transport/{publicapi,bff}) called
// in.
type Service struct {
	Repo Repository
}

// NewService wires a Service on top of a Repository.
func NewService(repo Repository) *Service {
	return &Service{Repo: repo}
}

// --- reads ---------------------------------------------------------------

// ListTodos returns every todo, created_at descending — no owner filter
// (GOAL.md's Ownership model decision: I3 no longer applies to this
// domain).
func (s *Service) ListTodos(ctx context.Context) ([]Todo, error) {
	return s.Repo.List(ctx)
}

// GetTodo returns a todo by id alone — ErrNotFound only for an unknown id
// (I3 no longer applies: every actor may read every todo).
func (s *Service) GetTodo(ctx context.Context, id string) (Todo, error) {
	return s.Repo.GetByID(ctx, id)
}

// ListEvents returns one todo's own timeline, oldest first, each event
// carrying its actor's handle/role (milestone-4 fix-round,
// handle-exposure — see TodoEventWithActor's own doc comment).
func (s *Service) ListEvents(ctx context.Context, todoID string) ([]TodoEventWithActor, error) {
	return s.Repo.ListEventsByTodoID(ctx, todoID)
}

// ListFeed returns the cross-todo activity feed, newest first,
// cursor-paginated — task-5's read path builds on this directly.
func (s *Service) ListFeed(ctx context.Context, cursorCreatedAt *time.Time, cursorID *string, limit int64) ([]TodoEventFeedRow, error) {
	return s.Repo.ListEventsFeed(ctx, cursorCreatedAt, cursorID, limit)
}

// --- creation (the "created" event's own dedicated path, never through
// Append — I16) ------------------------------------------------------------

// CreateInput groups CreateTodo's optional fields (DATA_MODEL.md: all
// nullable) plus the idempotency key every todo_events row needs
// (client_request_id is NOT NULL UNIQUE at the schema level, including for
// "created" rows — I19's guarantee is not special-cased away for
// creation).
type CreateInput struct {
	Title           string
	AssigneeID      *string
	Priority        *string
	DueDate         *time.Time
	ClientRequestID string
}

// CreateTodo creates a new todo attributed to createdBy (attribution
// only, never access-scoping — GOAL.md) and, inside the same transaction,
// inserts the "created" event that starts its timeline. This is the one
// and only place a "created" event is ever written — Append (below) has
// no WriteEventType value that maps to it, so no caller of Append can
// ever ask for one (I16's service-level half).
//
// Idempotent the same way Append is (I19): a repeat ClientRequestID
// returns the todo that request already created, and creates nothing
// new.
func (s *Service) CreateTodo(ctx context.Context, createdBy string, input CreateInput) (Todo, error) {
	var result Todo
	err := s.Repo.WithinTx(ctx, func(tx Repository) error {
		existingEvent, err := tx.GetEventByClientRequestID(ctx, input.ClientRequestID)
		if err == nil {
			existingTodo, err := tx.GetByID(ctx, existingEvent.TodoID)
			if err != nil {
				return err
			}
			result = existingTodo
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}

		created, err := tx.Create(ctx, createdBy, input.Title, CreateParams{
			AssigneeID: input.AssigneeID,
			Priority:   input.Priority,
			DueDate:    input.DueDate,
		})
		if err != nil {
			return err
		}

		payload, err := json.Marshal(map[string]interface{}{"title": created.Title})
		if err != nil {
			return err
		}
		payloadStr := string(payload)

		if _, err := tx.InsertEvent(ctx, created.ID, createdBy, EventCreated, &payloadStr, nil, input.ClientRequestID); err != nil {
			return err
		}

		result = created
		return nil
	})
	return result, err
}

// --- the single write path for todo_events (I15) --------------------------

// TodoField names which of a todo's fields a field_changed event
// describes.
type TodoField string

const (
	FieldTitle    TodoField = "title"
	FieldPriority TodoField = "priority"
	FieldDueDate  TodoField = "due_date"
)

// WriteEventType is the strict subset of EventType a caller may ask
// Append to write. Deliberately excludes EventCreated (I16): "created"
// only ever happens as a side effect of CreateTodo. There is no
// WriteEventType value that maps to "created" — not a runtime check that
// could be forgotten at a future call site, but a fact about this type's
// definition, which is what "the method's own signature/dispatch has no
// path that lets a caller ask it to write a created event" (task-2.md)
// means concretely.
type WriteEventType string

const (
	EventTypeCommented     WriteEventType = WriteEventType(EventCommented)
	EventTypeStatusChanged WriteEventType = WriteEventType(EventStatusChanged)
	EventTypeAssigned      WriteEventType = WriteEventType(EventAssigned)
	EventTypeFieldChanged  WriteEventType = WriteEventType(EventFieldChanged)
)

// CommentInput is AppendInput's payload for EventTypeCommented.
type CommentInput struct {
	Body string
}

// StatusChangeInput is AppendInput's payload for EventTypeStatusChanged.
type StatusChangeInput struct {
	ToStatus Status
}

// AssignmentInput is AppendInput's payload for EventTypeAssigned. A nil
// ToAssigneeID unassigns the todo.
type AssignmentInput struct {
	ToAssigneeID *string
}

// FieldChangeInput is AppendInput's payload for EventTypeFieldChanged.
// Exactly one of Title/Priority/DueDate is read, selected by Field — the
// other two are ignored. Priority/DueDate being nil means "clear this
// field", the same nullable-field convention Repo's own Update* methods
// use.
type FieldChangeInput struct {
	Field    TodoField
	Title    *string
	Priority *string
	DueDate  *time.Time
}

// AppendInput is the single write path's request shape (mirrors my-task's
// AppendInput, task.service.ts:161). Exactly one of Comment/StatusChange/
// Assignment/FieldChange should be set, matching Type — Append validates
// this and returns an error otherwise rather than guessing.
type AppendInput struct {
	TodoID          string
	Actor           PolicyActor
	ActorID         string
	ClientRequestID string
	Type            WriteEventType

	Comment      *CommentInput
	StatusChange *StatusChangeInput
	Assignment   *AssignmentInput
	FieldChange  *FieldChangeInput
}

type fromToPayload struct {
	From interface{} `json:"from"`
	To   interface{} `json:"to"`
}

type fieldChangePayload struct {
	Field string      `json:"field"`
	From  interface{} `json:"from"`
	To    interface{} `json:"to"`
}

func strPtrToAny(p *string) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

// ErrUnknownAssignee is Append's own sentinel for an `assigned` event
// whose "to" id does not resolve to any real user — my-task's own
// equivalent (findUserByHandle failing in the assigned-event branch of
// POST /api/v1/tasks/:id/events) surfaces as a 400 validation error
// (unknownAssigneeError), not a 500; this repo's handler layer
// (internal/transport/{bff,publicapi}/todo_handler.go) maps this error the
// same way.
var ErrUnknownAssignee = errors.New("todo: unknown assignee")

// assigneeSnapshot is the `assigned` event payload's `from`/`to` shape —
// milestone-4 fix-round (handle-exposure), matching my-task's own
// AssigneeSnapshot
// (~/gits/my-task/src/server/modules/task/task.service.ts:139-142) field
// for field: `{id, handle}`, resolved once at write time and baked into
// the stored payload permanently (see resolveAssigneeSnapshot below for
// why write-time, not read-time).
type assigneeSnapshot struct {
	ID     string `json:"id"`
	Handle string `json:"handle"`
}

// resolveAssigneeSnapshot resolves id (if non-nil) to a {id, handle}
// snapshot baked into the `assigned` event's payload at write time —
// matching my-task's own mustGetAssignee
// (~/gits/my-task/src/server/modules/task/task.service.ts:537-545), which
// does exactly this at exactly this moment: a snapshot captured when the
// event is written, not a live join resolved later when the event is
// read, so a later handle change (a user renamed, in a system that
// allowed it) does not retroactively rewrite old history — the same
// "immutable historical record" property every other todo_events row
// already has (I17: append-only). This is a deliberate, load-bearing
// choice, not an arbitrary one: my-task's own appendAssigned resolves
// both `from` and `to` this way, at the moment the event is created, and
// nowhere does a read path re-derive an old event's assignee snapshot
// from the current users table.
//
// required distinguishes the two call sites' different failure
// tolerance:
//
//   - the "to" id is caller-supplied input for THIS write (required:
//     true) — an id that does not resolve to a real user is a genuine
//     validation error (ErrUnknownAssignee), mirroring my-task's own
//     unknownAssigneeError for exactly this case (POST /api/v1/tasks/:id/
//     events's `assigned` branch, task-ref/route.ts).
//   - the "from" id is whatever this todo's assignee_id already was
//     before this write (required: false) — a lookup failure there means
//     the previously-recorded assignee no longer resolves (e.g. stale
//     data from before this fix-round, or a user row removed by some
//     future admin action this milestone doesn't have yet). That is not
//     something THIS write should fail over: it degrades to a snapshot
//     whose handle equals the raw id (visibly a raw id, not a fabricated
//     name) rather than blocking a legitimate reassignment because of a
//     data anomaly unrelated to the reassignment itself.
func resolveAssigneeSnapshot(ctx context.Context, tx Repository, id *string, required bool) (*assigneeSnapshot, error) {
	if id == nil {
		return nil, nil
	}
	handle, err := tx.ResolveUserHandle(ctx, *id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			if required {
				return nil, ErrUnknownAssignee
			}
			return &assigneeSnapshot{ID: *id, Handle: *id}, nil
		}
		return nil, err
	}
	return &assigneeSnapshot{ID: *id, Handle: handle}, nil
}

func timePtrToAny(p *time.Time) interface{} {
	if p == nil {
		return nil
	}
	return p.UTC().Format(time.RFC3339)
}

// Append is I15's single write path: every event-producing action on an
// existing todo funnels through this one method (mirrors my-task's
// TaskService.append(), task.service.ts:161-199). Steps, all inside one
// transaction (WithinTx):
//
//  1. Idempotency lookup (I19) — a repeat ClientRequestID returns the
//     original event, unchanged, and writes nothing further.
//  2. Permission check (I18) — resolved here, inside the write path
//     itself, before dispatch. This is the deliberate strengthening past
//     my-task's own shape that INVARIANTS.md's I18 entry states as one:
//     my-task's append() does not check permission itself (its callers
//     do, once per entry point); this method's callers cannot skip the
//     check by forgetting, because it is not theirs to perform.
//  3. type:"created" is not reachable here at all — see WriteEventType's
//     doc comment (I16).
//  4. Dispatch by event type -> the domain-specific side effect (e.g.
//     status_changed also updates todos.status) -> insert the event, with
//     seq computed inside the same transaction (I15's atomicity
//     requirement; Repo.InsertEvent does the seq computation).
//
// A failure at any step rolls the whole transaction back — WithinTx's own
// guarantee — so neither the event row nor any todos state change persists
// (GOAL.md Done-when 2).
func (s *Service) Append(ctx context.Context, input AppendInput) (TodoEvent, error) {
	var result TodoEvent
	err := s.Repo.WithinTx(ctx, func(tx Repository) error {
		// 1. Idempotency (I19), first, inside this transaction.
		existing, err := tx.GetEventByClientRequestID(ctx, input.ClientRequestID)
		if err == nil {
			result = existing
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}

		current, err := tx.GetByID(ctx, input.TodoID)
		if err != nil {
			return err
		}

		var toStatus Status
		if input.Type == EventTypeStatusChanged {
			if input.StatusChange == nil {
				return fmt.Errorf("todo: status_changed event missing StatusChange input")
			}
			toStatus = input.StatusChange.ToStatus
		}

		// 2. Permission (I18) — inside the write path, before dispatch.
		if !can(input.Actor, input.Type, toStatus) {
			return ErrForbidden
		}

		// 3. type:"created" is unreachable by construction (I16) — nothing
		// to check here; WriteEventType simply has no such value.

		// 4. Dispatch -> side effect -> (payload built below) -> insert.
		var payloadJSON *string
		var body *string

		switch input.Type {
		case EventTypeCommented:
			if input.Comment == nil {
				return fmt.Errorf("todo: commented event missing Comment input")
			}
			b := input.Comment.Body
			body = &b

		case EventTypeStatusChanged:
			p, err := json.Marshal(fromToPayload{From: string(current.Status), To: string(toStatus)})
			if err != nil {
				return err
			}
			ps := string(p)
			payloadJSON = &ps

			if _, err := tx.UpdateStatus(ctx, input.TodoID, toStatus); err != nil {
				return err
			}

		case EventTypeAssigned:
			if input.Assignment == nil {
				return fmt.Errorf("todo: assigned event missing Assignment input")
			}
			fromSnap, err := resolveAssigneeSnapshot(ctx, tx, current.AssigneeID, false)
			if err != nil {
				return err
			}
			toSnap, err := resolveAssigneeSnapshot(ctx, tx, input.Assignment.ToAssigneeID, true)
			if err != nil {
				return err
			}
			p, err := json.Marshal(fromToPayload{From: fromSnap, To: toSnap})
			if err != nil {
				return err
			}
			ps := string(p)
			payloadJSON = &ps

			if _, err := tx.UpdateAssignee(ctx, input.TodoID, input.Assignment.ToAssigneeID); err != nil {
				return err
			}

		case EventTypeFieldChanged:
			if input.FieldChange == nil {
				return fmt.Errorf("todo: field_changed event missing FieldChange input")
			}
			fc := input.FieldChange

			var from, to interface{}
			switch fc.Field {
			case FieldTitle:
				if fc.Title == nil {
					return fmt.Errorf("todo: field_changed(title) missing Title value")
				}
				from = current.Title
				to = *fc.Title
				if _, err := tx.UpdateTitle(ctx, input.TodoID, *fc.Title); err != nil {
					return err
				}

			case FieldPriority:
				from = strPtrToAny(current.Priority)
				to = strPtrToAny(fc.Priority)
				if _, err := tx.UpdatePriority(ctx, input.TodoID, fc.Priority); err != nil {
					return err
				}

			case FieldDueDate:
				from = timePtrToAny(current.DueDate)
				to = timePtrToAny(fc.DueDate)
				if _, err := tx.UpdateDueDate(ctx, input.TodoID, fc.DueDate); err != nil {
					return err
				}

			default:
				return fmt.Errorf("todo: field_changed unknown field %q", fc.Field)
			}

			p, err := json.Marshal(fieldChangePayload{Field: string(fc.Field), From: from, To: to})
			if err != nil {
				return err
			}
			ps := string(p)
			payloadJSON = &ps

		default:
			return fmt.Errorf("todo: unknown write event type %q", input.Type)
		}

		event, err := tx.InsertEvent(ctx, input.TodoID, input.ActorID, EventType(input.Type), payloadJSON, body, input.ClientRequestID)
		if err != nil {
			return err
		}
		result = event
		return nil
	})
	return result, err
}
