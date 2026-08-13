package todo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- fake ----------------------------------------------------------------

type fakeRepo struct {
	byID         map[string]Todo // keyed by id only — ownership is enforced in the methods below, exactly like the real repo
	createCalled bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byID: map[string]Todo{}}
}

func (f *fakeRepo) put(t Todo) { f.byID[t.ID] = t }

func (f *fakeRepo) ListByOwner(_ context.Context, ownerID string) ([]Todo, error) {
	var out []Todo
	for _, t := range f.byID {
		if t.OwnerID == ownerID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeRepo) Create(_ context.Context, ownerID, title string) (Todo, error) {
	f.createCalled = true
	now := time.Now()
	t := Todo{ID: "generated-" + title, OwnerID: ownerID, Title: title, Done: false, CreatedAt: now, UpdatedAt: now}
	f.put(t)
	return t, nil
}

func (f *fakeRepo) GetByIDAndOwner(_ context.Context, id, ownerID string) (Todo, error) {
	t, ok := f.byID[id]
	if !ok || t.OwnerID != ownerID {
		return Todo{}, ErrNotFound
	}
	return t, nil
}

func (f *fakeRepo) Update(_ context.Context, id, ownerID, title string, done bool) (Todo, error) {
	t, ok := f.byID[id]
	if !ok || t.OwnerID != ownerID {
		return Todo{}, ErrNotFound
	}
	t.Title = title
	t.Done = done
	t.UpdatedAt = time.Now()
	f.put(t)
	return t, nil
}

func (f *fakeRepo) Delete(_ context.Context, id, ownerID string) error {
	t, ok := f.byID[id]
	if !ok || t.OwnerID != ownerID {
		return ErrNotFound
	}
	delete(f.byID, id)
	return nil
}

// --- tests -----------------------------------------------------------------

func TestService_CreateTodo_DelegatesToRepo(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	created, err := svc.CreateTodo(context.Background(), "owner-1", "write tests")
	require.NoError(t, err)
	assert.True(t, repo.createCalled)
	assert.Equal(t, "owner-1", created.OwnerID)
	assert.Equal(t, "write tests", created.Title)
	assert.False(t, created.Done)
}

func TestService_ListTodos_OwnerScoped(t *testing.T) {
	repo := newFakeRepo()
	repo.put(Todo{ID: "t1", OwnerID: "owner-1", Title: "mine"})
	repo.put(Todo{ID: "t2", OwnerID: "owner-2", Title: "not mine"})
	svc := NewService(repo)

	got, err := svc.ListTodos(context.Background(), "owner-1")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "t1", got[0].ID)
}

// TestI3_ServiceOwnershipScoping_GetUpdateDeleteReturnNotFound — I3, at
// the service layer: a todo belonging to a different owner is
// ErrNotFound, the same as one that never existed, on every operation
// that takes an id.
func TestI3_ServiceOwnershipScoping_GetUpdateDeleteReturnNotFound(t *testing.T) {
	repo := newFakeRepo()
	repo.put(Todo{ID: "theirs", OwnerID: "owner-2", Title: "private", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	svc := NewService(repo)

	_, err := svc.GetTodo(context.Background(), "owner-1", "theirs")
	assert.ErrorIs(t, err, ErrNotFound)

	newTitle := "hijacked"
	_, err = svc.UpdateTodo(context.Background(), "owner-1", "theirs", &newTitle, nil)
	assert.ErrorIs(t, err, ErrNotFound)

	err = svc.DeleteTodo(context.Background(), "owner-1", "theirs")
	assert.ErrorIs(t, err, ErrNotFound)

	// Unaffected — the wrong-owner calls above didn't touch it.
	got, err := svc.GetTodo(context.Background(), "owner-2", "theirs")
	require.NoError(t, err)
	assert.Equal(t, "private", got.Title)
}

func TestI3_ServiceOwnershipScoping_UnknownIDAlsoNotFound(t *testing.T) {
	svc := NewService(newFakeRepo())

	_, err := svc.GetTodo(context.Background(), "owner-1", "does-not-exist")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_UpdateTodo_PartialPatchLeavesUnsetFieldAlone(t *testing.T) {
	repo := newFakeRepo()
	repo.put(Todo{ID: "t1", OwnerID: "owner-1", Title: "original", Done: false, CreatedAt: time.Now(), UpdatedAt: time.Now()})
	svc := NewService(repo)

	// Patch only `done` — title must survive untouched.
	trueVal := true
	updated, err := svc.UpdateTodo(context.Background(), "owner-1", "t1", nil, &trueVal)
	require.NoError(t, err)
	assert.Equal(t, "original", updated.Title)
	assert.True(t, updated.Done)

	// Patch only `title` — done must survive untouched (still true from above).
	newTitle := "renamed"
	updated, err = svc.UpdateTodo(context.Background(), "owner-1", "t1", &newTitle, nil)
	require.NoError(t, err)
	assert.Equal(t, "renamed", updated.Title)
	assert.True(t, updated.Done)
}

func TestService_DeleteTodo_AlreadyDeletedIsNotFound(t *testing.T) {
	repo := newFakeRepo()
	repo.put(Todo{ID: "t1", OwnerID: "owner-1", Title: "gone soon"})
	svc := NewService(repo)

	require.NoError(t, svc.DeleteTodo(context.Background(), "owner-1", "t1"))

	err := svc.DeleteTodo(context.Background(), "owner-1", "t1")
	assert.ErrorIs(t, err, ErrNotFound)
}
