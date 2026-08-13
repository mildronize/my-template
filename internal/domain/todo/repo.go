package todo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/mildronize/my-template/internal/db"
)

// ErrNotFound is returned by every Repo lookup/mutation when no row
// matches — both "no such id" and "not this caller's id" collapse to this
// one sentinel (I3: absence, not permission). service.go (and, one layer
// further out, internal/transport/publicapi/todo_handler.go) only ever
// see this domain-level error, never sql.ErrNoRows, so nothing outside
// this file needs to know sqlc or database/sql exist (ARCHITECTURE.md
// rule 2: only repo.go/*_repo.go may import the sqlc-generated package).
var ErrNotFound = errors.New("todo: not found")

// Todo is this package's own representation of a todos row, deliberately
// distinct from db.Todo (the sqlc-generated type) so every other file in
// this package can talk about "a todo" without importing internal/db
// itself.
type Todo struct {
	ID        string
	OwnerID   string
	Title     string
	Done      bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

func todoFromRow(row db.Todo) Todo {
	return Todo{
		ID:        row.ID,
		OwnerID:   row.OwnerID,
		Title:     row.Title,
		Done:      row.Done,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

// Repo is the only type in this package that imports the sqlc-generated
// package (internal/db) — every other file reaches the database only
// through Repo's methods (ARCHITECTURE.md rule 2). Every method here is
// scoped by owner_id; there is no lookup-by-id-alone method, on purpose —
// that shape doesn't exist for a reason (I3, I4): a caller of this repo
// physically cannot fetch a todo without also proving whose it must be,
// and this repo never queries any table but todos (I4).
type Repo struct {
	q *db.Queries
}

// NewRepo builds a Repo on top of an already-open *sql.DB (see
// platform.OpenDB).
func NewRepo(conn *sql.DB) *Repo {
	return &Repo{q: db.New(conn)}
}

// ListByOwner returns every todo belonging to ownerID, created_at
// descending (API.md: unpaginated, deliberately — see _contract/API.md
// Conventions).
func (r *Repo) ListByOwner(ctx context.Context, ownerID string) ([]Todo, error) {
	rows, err := r.q.ListTodosByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	todos := make([]Todo, 0, len(rows))
	for _, row := range rows {
		todos = append(todos, todoFromRow(row))
	}
	return todos, nil
}

// Create inserts a new todo owned by ownerID. id and the timestamps are
// generated here, not left to the caller — done always starts false
// (API.md).
func (r *Repo) Create(ctx context.Context, ownerID, title string) (Todo, error) {
	now := time.Now().UTC()
	row, err := r.q.CreateTodo(ctx, db.CreateTodoParams{
		ID:        uuid.NewString(),
		OwnerID:   ownerID,
		Title:     title,
		Done:      false,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return Todo{}, err
	}
	return todoFromRow(row), nil
}

// GetByIDAndOwner looks up a todo scoped to (id, ownerID). A todo that
// belongs to a different owner produces the exact same ErrNotFound as one
// that never existed (I3).
func (r *Repo) GetByIDAndOwner(ctx context.Context, id, ownerID string) (Todo, error) {
	row, err := r.q.GetTodoByIDAndOwner(ctx, db.GetTodoByIDAndOwnerParams{ID: id, OwnerID: ownerID})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Todo{}, ErrNotFound
		}
		return Todo{}, err
	}
	return todoFromRow(row), nil
}

// Update sets title/done (both always provided in full — service.go
// merges a partial PATCH into the existing row before calling this) and
// bumps updated_at, scoped to (id, ownerID). Same ErrNotFound-for-both
// shape as GetByIDAndOwner (I3).
func (r *Repo) Update(ctx context.Context, id, ownerID, title string, done bool) (Todo, error) {
	row, err := r.q.UpdateTodoByIDAndOwner(ctx, db.UpdateTodoByIDAndOwnerParams{
		Title:     title,
		Done:      done,
		UpdatedAt: time.Now().UTC(),
		ID:        id,
		OwnerID:   ownerID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Todo{}, ErrNotFound
		}
		return Todo{}, err
	}
	return todoFromRow(row), nil
}

// Delete removes the todo scoped to (id, ownerID). Deleting an id that
// doesn't exist, or that belongs to a different owner, or that was
// already deleted, are all ErrNotFound — no special-casing needed, since
// the query naturally affects zero rows in every one of those cases
// (API.md: "naturally idempotent").
func (r *Repo) Delete(ctx context.Context, id, ownerID string) error {
	n, err := r.q.DeleteTodoByIDAndOwner(ctx, db.DeleteTodoByIDAndOwnerParams{ID: id, OwnerID: ownerID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
