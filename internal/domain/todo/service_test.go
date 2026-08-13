package todo

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- fake, in-memory Repository -------------------------------------------
//
// Used only for plain delegation/dispatch-shape tests below. Done-when
// 2-5 (atomicity, append-only, idempotency, the paired permission check)
// are proven against a real *Repo/real sqlite database further down this
// file — fakeRepo's WithinTx does not simulate rollback, on purpose, so it
// is never used for anything claiming to prove a transactional property.

type fakeRepo struct {
	todos       map[string]Todo
	events      map[string]TodoEvent
	byClientReq map[string]string // clientRequestID -> event id
	seqByTodo   map[string]int64

	createCalled       bool
	updateStatusCalled bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		todos:       map[string]Todo{},
		events:      map[string]TodoEvent{},
		byClientReq: map[string]string{},
		seqByTodo:   map[string]int64{},
	}
}

func (f *fakeRepo) put(t Todo) { f.todos[t.ID] = t }

func (f *fakeRepo) WithinTx(ctx context.Context, fn func(tx Repository) error) error {
	return fn(f)
}

func (f *fakeRepo) List(ctx context.Context) ([]Todo, error) {
	var out []Todo
	for _, t := range f.todos {
		out = append(out, t)
	}
	return out, nil
}

func (f *fakeRepo) Create(ctx context.Context, createdBy, title string, params CreateParams) (Todo, error) {
	f.createCalled = true
	now := time.Now()
	id := "generated-" + title
	t := Todo{
		ID: id, CreatedBy: createdBy, Title: title, Status: StatusOpen,
		AssigneeID: params.AssigneeID, Priority: params.Priority, DueDate: params.DueDate,
		CreatedAt: now, UpdatedAt: now,
	}
	f.put(t)
	return t, nil
}

func (f *fakeRepo) GetByID(ctx context.Context, id string) (Todo, error) {
	t, ok := f.todos[id]
	if !ok {
		return Todo{}, ErrNotFound
	}
	return t, nil
}

func (f *fakeRepo) UpdateTitle(ctx context.Context, id, title string) (Todo, error) {
	t, ok := f.todos[id]
	if !ok {
		return Todo{}, ErrNotFound
	}
	t.Title = title
	t.UpdatedAt = time.Now()
	f.put(t)
	return t, nil
}

func (f *fakeRepo) UpdateStatus(ctx context.Context, id string, status Status) (Todo, error) {
	f.updateStatusCalled = true
	t, ok := f.todos[id]
	if !ok {
		return Todo{}, ErrNotFound
	}
	t.Status = status
	t.UpdatedAt = time.Now()
	f.put(t)
	return t, nil
}

func (f *fakeRepo) UpdateAssignee(ctx context.Context, id string, assigneeID *string) (Todo, error) {
	t, ok := f.todos[id]
	if !ok {
		return Todo{}, ErrNotFound
	}
	t.AssigneeID = assigneeID
	t.UpdatedAt = time.Now()
	f.put(t)
	return t, nil
}

func (f *fakeRepo) UpdatePriority(ctx context.Context, id string, priority *string) (Todo, error) {
	t, ok := f.todos[id]
	if !ok {
		return Todo{}, ErrNotFound
	}
	t.Priority = priority
	t.UpdatedAt = time.Now()
	f.put(t)
	return t, nil
}

func (f *fakeRepo) UpdateDueDate(ctx context.Context, id string, dueDate *time.Time) (Todo, error) {
	t, ok := f.todos[id]
	if !ok {
		return Todo{}, ErrNotFound
	}
	t.DueDate = dueDate
	t.UpdatedAt = time.Now()
	f.put(t)
	return t, nil
}

func (f *fakeRepo) GetEventByClientRequestID(ctx context.Context, clientRequestID string) (TodoEvent, error) {
	id, ok := f.byClientReq[clientRequestID]
	if !ok {
		return TodoEvent{}, ErrNotFound
	}
	return f.events[id], nil
}

func (f *fakeRepo) InsertEvent(ctx context.Context, todoID, actorID string, eventType EventType, payload, body *string, clientRequestID string) (TodoEvent, error) {
	f.seqByTodo[todoID]++
	seq := f.seqByTodo[todoID]
	id := fmt.Sprintf("event-%s-%d", todoID, seq)
	e := TodoEvent{
		ID: id, TodoID: todoID, Seq: seq, ActorID: actorID, Type: eventType,
		Payload: payload, Body: body, ClientRequestID: clientRequestID, CreatedAt: time.Now(),
	}
	f.events[id] = e
	f.byClientReq[clientRequestID] = id
	return e, nil
}

func (f *fakeRepo) ListEventsByTodoID(ctx context.Context, todoID string) ([]TodoEvent, error) {
	var out []TodoEvent
	for _, e := range f.events {
		if e.TodoID == todoID {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out, nil
}

func (f *fakeRepo) ListEventsFeed(ctx context.Context, cursorCreatedAt *time.Time, cursorID *string, limit int64) ([]TodoEventFeedRow, error) {
	var out []TodoEventFeedRow
	for _, e := range f.events {
		out = append(out, TodoEventFeedRow{Event: e, TodoTitle: f.todos[e.TodoID].Title})
	}
	return out, nil
}

// --- plain delegation / dispatch-shape tests (fakeRepo) -------------------

func TestService_CreateTodo_DelegatesToRepoAndInsertsCreatedEvent(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	created, err := svc.CreateTodo(context.Background(), "user-1", CreateInput{
		Title: "write tests", ClientRequestID: "req-1",
	})
	require.NoError(t, err)
	assert.True(t, repo.createCalled)
	assert.Equal(t, "user-1", created.CreatedBy)
	assert.Equal(t, "write tests", created.Title)
	assert.Equal(t, StatusOpen, created.Status)

	events, err := svc.ListEvents(context.Background(), created.ID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, EventCreated, events[0].Type)
	assert.EqualValues(t, 1, events[0].Seq)
}

func TestService_ListTodos_ReturnsEveryTodo(t *testing.T) {
	repo := newFakeRepo()
	repo.put(Todo{ID: "t1", CreatedBy: "user-1", Title: "mine"})
	repo.put(Todo{ID: "t2", CreatedBy: "user-2", Title: "not mine, but still listed"})
	svc := NewService(repo)

	got, err := svc.ListTodos(context.Background())
	require.NoError(t, err)
	assert.Len(t, got, 2, "GOAL.md's Ownership model decision: ListTodos is not scoped to one creator")
}

func TestService_GetTodo_ReadsAnyCreatorsTodoByIDAlone(t *testing.T) {
	repo := newFakeRepo()
	repo.put(Todo{ID: "theirs", CreatedBy: "user-2", Title: "not mine"})
	svc := NewService(repo)

	got, err := svc.GetTodo(context.Background(), "theirs")
	require.NoError(t, err, "I3 no longer applies to this domain — GetTodo takes no ownerID")
	assert.Equal(t, "not mine", got.Title)

	_, err = svc.GetTodo(context.Background(), "does-not-exist")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_Append_StatusChanged_DispatchesRepoUpdate(t *testing.T) {
	repo := newFakeRepo()
	repo.put(Todo{ID: "t1", CreatedBy: "user-1", Title: "task", Status: StatusOpen})
	svc := NewService(repo)

	event, err := svc.Append(context.Background(), AppendInput{
		TodoID: "t1", Actor: PolicyActor{Role: "owner"}, ActorID: "owner-1",
		ClientRequestID: "req-1", Type: EventTypeStatusChanged,
		StatusChange: &StatusChangeInput{ToStatus: StatusInProgress},
	})
	require.NoError(t, err)
	assert.True(t, repo.updateStatusCalled)
	assert.Equal(t, EventStatusChanged, event.Type)
	require.NotNil(t, event.Payload)
	assert.Contains(t, *event.Payload, "in_progress")

	got, err := repo.GetByID(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, StatusInProgress, got.Status, "the side effect must actually apply")
}

// --- I16: "created" is unreachable through Append --------------------------

// TestI16_Append_HasNoWriteEventTypeForCreated documents, at compile time,
// that WriteEventType has no constant mapping to "created" — this is the
// primary proof (a caller simply cannot construct AppendInput{Type:
// EventTypeCreated}, because no such identifier exists). This test adds a
// second, runtime layer on top: even a caller that manually constructs the
// underlying string value "created" (bypassing the type system by hand,
// something no real caller in this codebase does) still gets refused by
// Append's dispatch, which has no case for it.
func TestI16_Append_RejectsHandCraftedCreatedType(t *testing.T) {
	repo := newFakeRepo()
	repo.put(Todo{ID: "t1", CreatedBy: "user-1", Title: "task", Status: StatusOpen})
	svc := NewService(repo)

	_, err := svc.Append(context.Background(), AppendInput{
		TodoID: "t1", Actor: PolicyActor{Role: "owner"}, ActorID: "owner-1",
		ClientRequestID: "req-1", Type: WriteEventType("created"),
	})
	assert.Error(t, err, `Append's dispatch has no case for a hand-crafted "created" WriteEventType value`)

	events, _ := repo.ListEventsByTodoID(context.Background(), "t1")
	assert.Empty(t, events, "no event must have been written")
}

// --- Done-when 2: transactional atomicity (I15), against a real database --

// failureInjectingRepo wraps a real, tx-scoped Repository and, once
// armed, returns an error from UpdateStatus *after* delegating to the
// real implementation — so the real UPDATE statement really executes
// against the open transaction before the injected failure propagates up
// through Append and back into WithinTx's rollback path. This proves real
// rollback semantics (the already-applied write is undone), not merely
// "the side effect was never attempted." Per task-2.md's own suggestion
// ("inject a failure via a test double at a specific step") — the
// transaction and the failure are both real; only the *trigger* for the
// failure is a thin decorator.
type failureInjectingRepo struct {
	Repository
	armed bool
}

var errInjectedForAtomicityTest = errors.New("injected failure: forced after the side effect already executed inside this transaction")

func (f *failureInjectingRepo) UpdateStatus(ctx context.Context, id string, status Status) (Todo, error) {
	updated, err := f.Repository.UpdateStatus(ctx, id, status)
	if err != nil {
		return updated, err
	}
	if f.armed {
		return updated, errInjectedForAtomicityTest
	}
	return updated, nil
}

func (f *failureInjectingRepo) WithinTx(ctx context.Context, fn func(tx Repository) error) error {
	return f.Repository.WithinTx(ctx, func(tx Repository) error {
		return fn(&failureInjectingRepo{Repository: tx, armed: f.armed})
	})
}

func TestI15_Append_FailureMidWriteLeavesNeitherEventNorStateChange(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestOwner(t, conn, "owner-1", "owner-one")
	realRepo := NewRepo(conn)

	created, err := realRepo.Create(ctx, "owner-1", "task", CreateParams{})
	require.NoError(t, err)

	svc := NewService(&failureInjectingRepo{Repository: realRepo, armed: true})

	eventsBefore := countRows(t, conn, "todo_events")

	_, err = svc.Append(ctx, AppendInput{
		TodoID: created.ID, Actor: PolicyActor{Role: "owner"}, ActorID: "owner-1",
		ClientRequestID: "req-atomicity", Type: EventTypeStatusChanged,
		StatusChange: &StatusChangeInput{ToStatus: StatusInProgress},
	})
	require.ErrorIs(t, err, errInjectedForAtomicityTest, "the injected failure must propagate out of Append")

	eventsAfter := countRows(t, conn, "todo_events")
	assert.Equal(t, eventsBefore, eventsAfter, "no todo_events row may persist when the write path fails partway through")

	stillOpen, err := realRepo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, stillOpen.Status, "the status update must have rolled back with the rest of the transaction")
}

// --- Done-when 3: append-only — every state change adds a row, checked by
// counting, not by "no update method exists" (I17's own distinction) -----

func TestI17_Append_EachStateChangeAddsExactlyOneEventRow(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestOwner(t, conn, "owner-1", "owner-one")
	repo := NewRepo(conn)
	svc := NewService(repo)

	created, err := repo.Create(ctx, "owner-1", "task", CreateParams{})
	require.NoError(t, err)

	actions := []AppendInput{
		{Type: EventTypeStatusChanged, StatusChange: &StatusChangeInput{ToStatus: StatusInProgress}, ClientRequestID: "a1"},
		{Type: EventTypeAssigned, Assignment: &AssignmentInput{ToAssigneeID: strPtr("owner-1")}, ClientRequestID: "a2"},
		{Type: EventTypeFieldChanged, FieldChange: &FieldChangeInput{Field: FieldTitle, Title: strPtr("renamed task")}, ClientRequestID: "a3"},
		{Type: EventTypeCommented, Comment: &CommentInput{Body: "looks good"}, ClientRequestID: "a4"},
		{Type: EventTypeStatusChanged, StatusChange: &StatusChangeInput{ToStatus: StatusDone}, ClientRequestID: "a5"},
	}

	for i, action := range actions {
		before := countRows(t, conn, "todo_events")

		action.TodoID = created.ID
		action.Actor = PolicyActor{Role: "owner"}
		action.ActorID = "owner-1"
		_, err := svc.Append(ctx, action)
		require.NoErrorf(t, err, "action %d", i)

		after := countRows(t, conn, "todo_events")
		assert.Equalf(t, before+1, after, "action %d (%s) must add exactly one todo_events row", i, action.Type)
	}

	events, err := repo.ListEventsByTodoID(ctx, created.ID)
	require.NoError(t, err)
	// This test seeds the todo via repo.Create directly (not
	// Service.CreateTodo), which does not itself insert a "created" event —
	// so exactly len(actions) rows are expected, one per Append call above.
	assert.Len(t, events, len(actions))
	for i, e := range events {
		assert.EqualValues(t, i+1, e.Seq, "seq must be strictly monotonic across every action on this one todo")
	}
}

// --- Done-when 4: idempotency (I19) ----------------------------------------

func TestI19_Append_RepeatedClientRequestIDReturnsOriginalAndCreatesNothing(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestOwner(t, conn, "owner-1", "owner-one")
	repo := NewRepo(conn)
	svc := NewService(repo)

	created, err := repo.Create(ctx, "owner-1", "task", CreateParams{})
	require.NoError(t, err)

	input := AppendInput{
		TodoID: created.ID, Actor: PolicyActor{Role: "owner"}, ActorID: "owner-1",
		ClientRequestID: "repeat-me", Type: EventTypeStatusChanged,
		StatusChange: &StatusChangeInput{ToStatus: StatusInProgress},
	}

	first, err := svc.Append(ctx, input)
	require.NoError(t, err)
	afterFirst := countRows(t, conn, "todo_events")

	// A repeat with the SAME client_request_id, but a different requested
	// status — proving the second call really did nothing (if it had
	// re-dispatched, the todo's status would have moved to StatusDone).
	input.StatusChange = &StatusChangeInput{ToStatus: StatusDone}
	second, err := svc.Append(ctx, input)
	require.NoError(t, err)
	afterSecond := countRows(t, conn, "todo_events")

	assert.Equal(t, first, second, "a repeated client_request_id must return the original event, unchanged")
	assert.Equal(t, afterFirst, afterSecond, "a repeated client_request_id must create nothing")

	stillInProgress, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusInProgress, stillInProgress.Status, "the second call's requested status change must never have been applied")
}

func TestI19_CreateTodo_RepeatedClientRequestIDReturnsOriginalAndCreatesNothing(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestOwner(t, conn, "owner-1", "owner-one")
	repo := NewRepo(conn)
	svc := NewService(repo)

	input := CreateInput{Title: "first title", ClientRequestID: "repeat-create"}

	first, err := svc.CreateTodo(ctx, "owner-1", input)
	require.NoError(t, err)
	todosAfterFirst := countRows(t, conn, "todos")
	eventsAfterFirst := countRows(t, conn, "todo_events")

	input.Title = "a different title, must be ignored"
	second, err := svc.CreateTodo(ctx, "owner-1", input)
	require.NoError(t, err)
	todosAfterSecond := countRows(t, conn, "todos")
	eventsAfterSecond := countRows(t, conn, "todo_events")

	assert.Equal(t, first.ID, second.ID)
	assert.Equal(t, "first title", second.Title, "the second call's different title must never have been applied")
	assert.Equal(t, todosAfterFirst, todosAfterSecond)
	assert.Equal(t, eventsAfterFirst, eventsAfterSecond)
}

// --- Done-when 5: the permission layer, paired, through Append itself
// (not just can() in isolation — permission_test.go already covers that) --

// TestI18_Append_SameAgentSameTodo_ClosedRejected_NonClosedSucceeds is
// Done-when 5's exact shape: the same agent, against the same todo, has a
// status:closed attempt rejected AND a non-closed action succeed, in the
// same test — proving the refusal is about closed specifically, not the
// agent generally.
//
// Fixture note: the agent identity here is a plain seeded users row
// (createTestUser, role='agent'), not one produced via cmd/issue-key.
// That is a deliberate, stated shortcut, not an oversight: Append's
// permission check (can(), permission.go) is role-based only — it reads
// PolicyActor.Role, a value this test supplies directly, and never
// resolves an actor from a credential itself (I4: only
// internal/identity's own middleware does that, one layer up, outside
// this package). Testing key issuance/resolution through cmd/issue-key's
// real path belongs to internal/identity's and
// internal/transport/publicapi's own tests, not here.
func TestI18_Append_SameAgentSameTodo_ClosedRejected_NonClosedSucceeds(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "agent-1", "agent-one") // role='agent'
	repo := NewRepo(conn)
	svc := NewService(repo)

	created, err := repo.Create(ctx, "agent-1", "task", CreateParams{})
	require.NoError(t, err)

	agentActor := PolicyActor{Role: "agent"}

	_, err = svc.Append(ctx, AppendInput{
		TodoID: created.ID, Actor: agentActor, ActorID: "agent-1",
		ClientRequestID: "agent-closed-attempt", Type: EventTypeStatusChanged,
		StatusChange: &StatusChangeInput{ToStatus: StatusClosed},
	})
	assert.ErrorIs(t, err, ErrForbidden, "an agent must never be able to move a todo to closed")

	_, err = svc.Append(ctx, AppendInput{
		TodoID: created.ID, Actor: agentActor, ActorID: "agent-1",
		ClientRequestID: "agent-comment", Type: EventTypeCommented,
		Comment: &CommentInput{Body: "still working on it"},
	})
	assert.NoError(t, err, "the SAME agent, against the SAME todo, must still be able to do a non-closed action — proves the refusal above was about closed specifically")

	stillOpen, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, stillOpen.Status, "the rejected closed-attempt must never have changed the todo's status")

	eventsAfter, err := repo.ListEventsByTodoID(ctx, created.ID)
	require.NoError(t, err)
	require.Len(t, eventsAfter, 1, "only the successful comment was written — the rejected attempt wrote nothing")
	assert.Equal(t, EventCommented, eventsAfter[0].Type)
}

// TestI18_Append_OwnerCanCloseTheSameTodo is Done-when 5's second half,
// tested separately: the owner's status:closed attempt succeeds.
func TestI18_Append_OwnerCanCloseTheSameTodo(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestOwner(t, conn, "owner-1", "owner-one")
	repo := NewRepo(conn)
	svc := NewService(repo)

	created, err := repo.Create(ctx, "owner-1", "task", CreateParams{})
	require.NoError(t, err)

	_, err = svc.Append(ctx, AppendInput{
		TodoID: created.ID, Actor: PolicyActor{Role: "owner"}, ActorID: "owner-1",
		ClientRequestID: "owner-closes-it", Type: EventTypeStatusChanged,
		StatusChange: &StatusChangeInput{ToStatus: StatusClosed},
	})
	require.NoError(t, err)

	closed, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, closed.Status)
}

// TestI18_Append_UnknownTodoID_NotFoundBeforePermissionOrSideEffect proves
// a nonexistent todo id fails at the existence check, not the permission
// check or the side effect — the ordering Append documents.
func TestI18_Append_UnknownTodoID_NotFoundBeforePermissionOrSideEffect(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	repo := NewRepo(conn)
	svc := NewService(repo)

	_, err := svc.Append(ctx, AppendInput{
		TodoID: "does-not-exist", Actor: PolicyActor{Role: "owner"}, ActorID: "owner-1",
		ClientRequestID: "req-1", Type: EventTypeCommented,
		Comment: &CommentInput{Body: "hi"},
	})
	assert.ErrorIs(t, err, ErrNotFound)
}
