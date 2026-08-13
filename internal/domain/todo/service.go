package todo

import "context"

// Repository is the subset of Repo's methods Service depends on. Declared
// here (not in repo.go) so tests can supply a fake without a real
// database — repo.go's *Repo satisfies this interface structurally, with
// no import of internal/db required on this side (mirrors
// internal/identity's UserRepo/APIKeyRepo split).
type Repository interface {
	ListByOwner(ctx context.Context, ownerID string) ([]Todo, error)
	Create(ctx context.Context, ownerID, title string) (Todo, error)
	GetByIDAndOwner(ctx context.Context, id, ownerID string) (Todo, error)
	Update(ctx context.Context, id, ownerID, title string, done bool) (Todo, error)
	Delete(ctx context.Context, id, ownerID string) error
}

// Service implements the todo CRUD contract from _contract/API.md, scoped
// to whichever owner the caller (internal/transport/publicapi/
// todo_handler.go, via the actor internal/identity's middleware already
// resolved) already resolved — this package never resolves an actor
// itself (I4).
type Service struct {
	Repo Repository
}

// NewService wires a Service on top of a Repository.
func NewService(repo Repository) *Service {
	return &Service{Repo: repo}
}

// ListTodos returns ownerID's own todos, created_at descending,
// unpaginated (API.md Conventions).
func (s *Service) ListTodos(ctx context.Context, ownerID string) ([]Todo, error) {
	return s.Repo.ListByOwner(ctx, ownerID)
}

// CreateTodo creates a todo owned by ownerID. title's length is enforced
// by openapi.yaml's request validation (1-200 chars, DATA_MODEL.md)
// before a request ever reaches here, and again by the todos.title CHECK
// constraint at the storage layer — this method itself does no
// re-validation, trusting both of those layers rather than adding a
// third copy of the same rule.
func (s *Service) CreateTodo(ctx context.Context, ownerID, title string) (Todo, error) {
	return s.Repo.Create(ctx, ownerID, title)
}

// GetTodo returns ownerID's own todo by id — ErrNotFound for both an
// unknown id and one belonging to a different owner (I3).
func (s *Service) GetTodo(ctx context.Context, ownerID, id string) (Todo, error) {
	return s.Repo.GetByIDAndOwner(ctx, id, ownerID)
}

// UpdateTodo applies a partial patch (either field may be nil, meaning
// "leave as-is") to ownerID's own todo. It reads the existing row first
// so a PATCH that only sets `done` doesn't clobber `title` (and vice
// versa) — the repo layer always writes both fields in full.
func (s *Service) UpdateTodo(ctx context.Context, ownerID, id string, title *string, done *bool) (Todo, error) {
	existing, err := s.Repo.GetByIDAndOwner(ctx, id, ownerID)
	if err != nil {
		return Todo{}, err
	}

	newTitle := existing.Title
	if title != nil {
		newTitle = *title
	}
	newDone := existing.Done
	if done != nil {
		newDone = *done
	}

	return s.Repo.Update(ctx, id, ownerID, newTitle, newDone)
}

// DeleteTodo deletes ownerID's own todo by id — ErrNotFound for an
// unknown id, a different owner's id, or an already-deleted id alike
// (API.md: naturally idempotent, I3).
func (s *Service) DeleteTodo(ctx context.Context, ownerID, id string) error {
	return s.Repo.Delete(ctx, id, ownerID)
}
