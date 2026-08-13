package todo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/mildronize/my-template/internal/db"
)

// ErrNotFound is returned by every Repo lookup when no row matches. Unlike
// milestone-1/2/3, this is no longer "no such id" collapsed together with
// "not this caller's id" (I3 no longer applies to this domain — GOAL.md's
// Ownership model decision, INVARIANTS.md I3's own scope note): a todo
// that exists is visible to every authenticated actor, so the only case
// left is "no such id" at all. service.go (and, one layer further out,
// internal/transport/{publicapi,bff}) only ever see this domain-level
// error, never sql.ErrNoRows, so nothing outside this file needs to know
// sqlc or database/sql exist (ARCHITECTURE.md rule 2: only repo.go/
// *_repo.go may import the sqlc-generated package).
var ErrNotFound = errors.New("todo: not found")

// Status is the fixed four-value enum DATA_MODEL.md gives todos.status,
// replacing the milestone-1/2/3 `done` boolean entirely — there is no
// boolean anywhere in this package any more. Not an owner-editable table
// like my-task's `statuses` (GOAL.md's "No manageable-statuses table"
// decision).
type Status string

const (
	StatusOpen       Status = "open"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
	StatusClosed     Status = "closed"
)

// Priority is the fixed four-value enum DATA_MODEL.md gives todos.priority
// (nullable — a todo may have no priority at all).
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
	PriorityUrgent Priority = "urgent"
)

// Todo is this package's own representation of a todos row, deliberately
// distinct from db.Todo (the sqlc-generated type) so every other file in
// this package can talk about "a todo" without importing internal/db
// itself. CreatedBy replaces OwnerID (attribution only, never
// access-scoping — GOAL.md); Status replaces Done; AssigneeID/Priority/
// DueDate are new, all nullable.
type Todo struct {
	ID         string
	CreatedBy  string
	Title      string
	Status     Status
	AssigneeID *string
	Priority   *string
	DueDate    *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// EventType is todo_events.type's fixed vocabulary (DATA_MODEL.md) — the
// full set, including "created", used to represent an event once it
// exists (read side). See WriteEventType below for the strictly smaller
// set Append (I15's single write path) accepts as a caller-specified
// value.
type EventType string

const (
	EventCreated       EventType = "created"
	EventCommented     EventType = "commented"
	EventStatusChanged EventType = "status_changed"
	EventAssigned      EventType = "assigned"
	EventFieldChanged  EventType = "field_changed"
)

// TodoEvent is this package's own representation of a todo_events row,
// deliberately distinct from db.TodoEvent for the same reason Todo is
// distinct from db.Todo.
type TodoEvent struct {
	ID              string
	TodoID          string
	Seq             int64
	ActorID         string
	Type            EventType
	Payload         *string
	Body            *string
	ClientRequestID string
	CreatedAt       time.Time
}

// TodoEventFeedRow is one row of the cross-todo activity feed
// (ListEventsFeed) — a TodoEvent joined to the todo it belongs to and the
// user who produced it, matching db.ListTodoEventsFeedRow's shape
// one-for-one (task-5's own read path builds on this).
type TodoEventFeedRow struct {
	Event       TodoEvent
	TodoTitle   string
	ActorHandle string
	ActorRole   string
}

func todoFromRow(row db.Todo) Todo {
	return Todo{
		ID:         row.ID,
		CreatedBy:  row.CreatedBy,
		Title:      row.Title,
		Status:     Status(row.Status),
		AssigneeID: nullStringToPtr(row.AssigneeID),
		Priority:   nullStringToPtr(row.Priority),
		DueDate:    nullTimeToPtr(row.DueDate),
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
}

func todoEventFromRow(row db.TodoEvent) TodoEvent {
	return TodoEvent{
		ID:              row.ID,
		TodoID:          row.TodoID,
		Seq:             row.Seq,
		ActorID:         row.ActorID,
		Type:            EventType(row.Type),
		Payload:         nullStringToPtr(row.Payload),
		Body:            nullStringToPtr(row.Body),
		ClientRequestID: row.ClientRequestID,
		CreatedAt:       row.CreatedAt,
	}
}

func nullStringToPtr(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	v := s.String
	return &v
}

func ptrToNullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func nullTimeToPtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func ptrToNullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// Repo is the only type in this package that imports the sqlc-generated
// package (internal/db) — every other file reaches the database only
// through Repo's methods (ARCHITECTURE.md rule 2). Unlike milestone-1/2/3,
// there is no owner-scoping baked into any lookup here: todos are a
// shared collection (I3 no longer applies to this domain), so a caller
// only ever needs an id, never an id plus whose it must be. This repo
// still never queries any table but todos/todo_events (I4).
//
// conn is kept alongside q (not just q) so WithinTx below can open a real
// *sql.Tx — the seam I15's single write path (service.go's Append) needs
// to make the idempotency check, permission check, side effect, seq
// computation, and event insert one atomic unit.
type Repo struct {
	conn *sql.DB
	q    *db.Queries
}

// NewRepo builds a Repo on top of an already-open *sql.DB (see
// platform.OpenDB).
func NewRepo(conn *sql.DB) *Repo {
	return &Repo{conn: conn, q: db.New(conn)}
}

// WithinTx runs fn against a Repo bound to a single database transaction —
// committing if fn returns nil, rolling back (and returning fn's error)
// otherwise. This is the transactional seam I15 requires: service.go's
// Append calls this once, and every repo call fn makes (idempotency
// lookup, permission-relevant reads, the domain-specific side effect, seq
// computation, event insert) runs against the same *sql.Tx, so a failure
// partway through leaves neither the event row nor the todos state change
// (GOAL.md Done-when 2). fn receives a Repository (the interface, not
// *Repo) so service.go's Append never needs to know sql.Tx or the sqlc
// package exist — ARCHITECTURE.md rule 2 stays true: only this file
// imports internal/db.
func (r *Repo) WithinTx(ctx context.Context, fn func(tx Repository) error) error {
	sqlTx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	txRepo := &Repo{conn: r.conn, q: r.q.WithTx(sqlTx)}
	if err := fn(txRepo); err != nil {
		if rbErr := sqlTx.Rollback(); rbErr != nil {
			return errors.Join(err, rbErr)
		}
		return err
	}
	return sqlTx.Commit()
}

// --- todos ------------------------------------------------------------

// List returns every todo, created_at descending — no owner filter
// (GOAL.md's Ownership model decision: todos are a shared collection,
// every authenticated actor can see every one).
func (r *Repo) List(ctx context.Context) ([]Todo, error) {
	rows, err := r.q.ListTodos(ctx)
	if err != nil {
		return nil, err
	}
	todos := make([]Todo, 0, len(rows))
	for _, row := range rows {
		todos = append(todos, todoFromRow(row))
	}
	return todos, nil
}

// CreateParams groups Create's optional fields — all nullable per
// DATA_MODEL.md.
type CreateParams struct {
	AssigneeID *string
	Priority   *string
	DueDate    *time.Time
}

// Create inserts a new todo attributed to createdBy (attribution only,
// never access-scoping — GOAL.md). id and the timestamps are generated
// here; status always starts StatusOpen.
func (r *Repo) Create(ctx context.Context, createdBy, title string, params CreateParams) (Todo, error) {
	now := time.Now().UTC()
	row, err := r.q.CreateTodo(ctx, db.CreateTodoParams{
		ID:         uuid.NewString(),
		CreatedBy:  createdBy,
		Title:      title,
		Status:     string(StatusOpen),
		AssigneeID: ptrToNullString(params.AssigneeID),
		Priority:   ptrToNullString(params.Priority),
		DueDate:    ptrToNullTime(params.DueDate),
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		return Todo{}, err
	}
	return todoFromRow(row), nil
}

// GetByID looks up a todo by id alone — there is no "wrong owner" case
// left to return ErrNotFound for (I3 no longer applies to this domain):
// every actor can see every todo, so this either finds the row or it
// never existed.
func (r *Repo) GetByID(ctx context.Context, id string) (Todo, error) {
	row, err := r.q.GetTodoByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Todo{}, ErrNotFound
		}
		return Todo{}, err
	}
	return todoFromRow(row), nil
}

// UpdateTitle sets title and bumps updated_at.
func (r *Repo) UpdateTitle(ctx context.Context, id, title string) (Todo, error) {
	row, err := r.q.UpdateTodoTitle(ctx, db.UpdateTodoTitleParams{
		Title:     title,
		UpdatedAt: time.Now().UTC(),
		ID:        id,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Todo{}, ErrNotFound
		}
		return Todo{}, err
	}
	return todoFromRow(row), nil
}

// UpdateStatus sets status and bumps updated_at. Callers (service.go's
// Append) are responsible for permission-gating status:closed (I18) —
// this method itself performs no permission check, matching every other
// Repo method in this package (permission lives one layer up, I18's own
// text: "checked inside I15's single write path", not the repo).
func (r *Repo) UpdateStatus(ctx context.Context, id string, status Status) (Todo, error) {
	row, err := r.q.UpdateTodoStatus(ctx, db.UpdateTodoStatusParams{
		Status:    string(status),
		UpdatedAt: time.Now().UTC(),
		ID:        id,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Todo{}, ErrNotFound
		}
		return Todo{}, err
	}
	return todoFromRow(row), nil
}

// UpdateAssignee sets assignee_id (nil clears it) and bumps updated_at.
func (r *Repo) UpdateAssignee(ctx context.Context, id string, assigneeID *string) (Todo, error) {
	row, err := r.q.UpdateTodoAssignee(ctx, db.UpdateTodoAssigneeParams{
		AssigneeID: ptrToNullString(assigneeID),
		UpdatedAt:  time.Now().UTC(),
		ID:         id,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Todo{}, ErrNotFound
		}
		return Todo{}, err
	}
	return todoFromRow(row), nil
}

// UpdatePriority sets priority (nil clears it) and bumps updated_at.
func (r *Repo) UpdatePriority(ctx context.Context, id string, priority *string) (Todo, error) {
	row, err := r.q.UpdateTodoPriority(ctx, db.UpdateTodoPriorityParams{
		Priority:  ptrToNullString(priority),
		UpdatedAt: time.Now().UTC(),
		ID:        id,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Todo{}, ErrNotFound
		}
		return Todo{}, err
	}
	return todoFromRow(row), nil
}

// UpdateDueDate sets due_date (nil clears it) and bumps updated_at.
func (r *Repo) UpdateDueDate(ctx context.Context, id string, dueDate *time.Time) (Todo, error) {
	row, err := r.q.UpdateTodoDueDate(ctx, db.UpdateTodoDueDateParams{
		DueDate:   ptrToNullTime(dueDate),
		UpdatedAt: time.Now().UTC(),
		ID:        id,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Todo{}, ErrNotFound
		}
		return Todo{}, err
	}
	return todoFromRow(row), nil
}

// Delete does not exist on this domain any more (GOAL.md's "DELETE
// removed" decision, mirroring my-task's I12): there is nothing left for
// a future handler to accidentally wire up, on purpose — see doc.go/
// service.go for the same absence.

// --- todo_events (I15, I17, I19) ---------------------------------------

// GetEventByClientRequestID is I19's idempotency lookup — called first,
// inside the same transaction as everything else in Append.
func (r *Repo) GetEventByClientRequestID(ctx context.Context, clientRequestID string) (TodoEvent, error) {
	row, err := r.q.GetTodoEventByClientRequestID(ctx, clientRequestID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TodoEvent{}, ErrNotFound
		}
		return TodoEvent{}, err
	}
	return todoEventFromRow(row), nil
}

// InsertEvent computes seq as GetTodoEventMaxSeqByTodoID's result + 1 and
// inserts the new event — both inside whatever transaction this Repo
// value is already bound to (I15's atomicity requirement: the caller
// reaches this method only via WithinTx). This is the one place seq is
// computed, so every event type's insert (including CreateTodo's own
// "created" event, which does not go through service.go's Append) shares
// the same monotonic-per-todo guarantee. Append-only (I17): this is an
// INSERT and nothing else — there is no corresponding Update/Delete method
// anywhere in this file.
func (r *Repo) InsertEvent(ctx context.Context, todoID, actorID string, eventType EventType, payload, body *string, clientRequestID string) (TodoEvent, error) {
	maxSeq, err := r.q.GetTodoEventMaxSeqByTodoID(ctx, todoID)
	if err != nil {
		return TodoEvent{}, err
	}

	row, err := r.q.InsertTodoEvent(ctx, db.InsertTodoEventParams{
		ID:              uuid.NewString(),
		TodoID:          todoID,
		Seq:             maxSeq + 1,
		ActorID:         actorID,
		Type:            string(eventType),
		Payload:         ptrToNullString(payload),
		Body:            ptrToNullString(body),
		ClientRequestID: clientRequestID,
		CreatedAt:       time.Now().UTC(),
	})
	if err != nil {
		return TodoEvent{}, err
	}
	return todoEventFromRow(row), nil
}

// ListEventsByTodoID returns one todo's own timeline, oldest first
// (`_contract/API.md`'s milestone-4 section — task-3/4's to wire up).
func (r *Repo) ListEventsByTodoID(ctx context.Context, todoID string) ([]TodoEvent, error) {
	rows, err := r.q.ListTodoEventsByTodoID(ctx, todoID)
	if err != nil {
		return nil, err
	}
	events := make([]TodoEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, todoEventFromRow(row))
	}
	return events, nil
}

// ListEventsFeed returns the cross-todo activity feed, newest first,
// cursor-paginated on (created_at, id) — task-5's read path builds on
// this. The first page passes cursorCreatedAt/cursorID both nil.
func (r *Repo) ListEventsFeed(ctx context.Context, cursorCreatedAt *time.Time, cursorID *string, limit int64) ([]TodoEventFeedRow, error) {
	var cursorCreatedAtArg interface{}
	if cursorCreatedAt != nil {
		cursorCreatedAtArg = *cursorCreatedAt
	}
	rows, err := r.q.ListTodoEventsFeed(ctx, db.ListTodoEventsFeedParams{
		CursorCreatedAt: cursorCreatedAtArg,
		CursorID:        ptrToNullString(cursorID),
		Limit:           limit,
	})
	if err != nil {
		return nil, err
	}
	items := make([]TodoEventFeedRow, 0, len(rows))
	for _, row := range rows {
		items = append(items, TodoEventFeedRow{
			Event: TodoEvent{
				ID:              row.ID,
				TodoID:          row.TodoID,
				Seq:             row.Seq,
				ActorID:         row.ActorID,
				Type:            EventType(row.Type),
				Payload:         nullStringToPtr(row.Payload),
				Body:            nullStringToPtr(row.Body),
				ClientRequestID: row.ClientRequestID,
				CreatedAt:       row.CreatedAt,
			},
			TodoTitle:   row.TodoTitle,
			ActorHandle: row.ActorHandle,
			ActorRole:   row.ActorRole,
		})
	}
	return items, nil
}
