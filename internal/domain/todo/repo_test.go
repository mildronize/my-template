package todo

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mildronize/my-template/internal/dbquery"
)

// --- todos: no owner scoping, new fields ----------------------------------

func TestRepo_CreateAndGetRoundTrip(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "user-1", "user-one")
	repo := NewRepo(conn)

	created, err := repo.Create(ctx, "user-1", "write the launch doc", CreateParams{
		AssigneeID: strPtr("user-1"),
		Priority:   strPtr(string(PriorityHigh)),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, "user-1", created.CreatedBy)
	assert.Equal(t, "write the launch doc", created.Title)
	assert.Equal(t, StatusOpen, created.Status, "status must start open")
	require.NotNil(t, created.AssigneeID)
	assert.Equal(t, "user-1", *created.AssigneeID)
	require.NotNil(t, created.Priority)
	assert.Equal(t, string(PriorityHigh), *created.Priority)
	assert.Nil(t, created.DueDate)
	assert.False(t, created.CreatedAt.IsZero())
	assert.Equal(t, created.CreatedAt, created.UpdatedAt)

	got, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created, got)
}

func TestRepo_GetByID_UnknownID(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	repo := NewRepo(conn)

	_, err := repo.GetByID(ctx, "does-not-exist")
	assert.ErrorIs(t, err, ErrNotFound)
}

// TestI3_GetByIDReadsAnyCreator_ScopingRetiredForThisDomain — GOAL.md's
// Ownership model decision / INVARIANTS.md I3's own scope note: a todo
// created by one actor is readable, by id alone, by a repo call that
// names no caller at all — there is no "wrong owner" case left. This is
// the direct negative-control proof that the old owner-scoped lookup
// shape is really gone, not merely renamed. Named with the TestI3_ prefix
// on purpose (internal/invariants_test.go's TestDoneWhen12 requires a
// dedicated TestI3_ test inside every domain module's own package,
// per-domain-module scope — I3's *reach* narrowed for this domain, but
// the invariant itself, and the naming convention proving it's been
// addressed here, did not go away).
func TestI3_GetByIDReadsAnyCreator_ScopingRetiredForThisDomain(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "user-1", "user-one")
	createTestUser(t, conn, "user-2", "user-two")
	repo := NewRepo(conn)

	theirs, err := repo.Create(ctx, "user-2", "someone else made this", CreateParams{})
	require.NoError(t, err)

	// No ownerID argument exists on GetByID at all any more — reading it
	// back needs only the id, regardless of who created it.
	got, err := repo.GetByID(ctx, theirs.ID)
	require.NoError(t, err)
	assert.Equal(t, theirs, got)
}

func TestRepo_List_ReturnsEveryTodoNewestFirst_NotScopedToOneCreator(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "user-1", "user-one")
	createTestUser(t, conn, "user-2", "user-two")
	repo := NewRepo(conn)

	first, err := repo.Create(ctx, "user-1", "first", CreateParams{})
	require.NoError(t, err)
	second, err := repo.Create(ctx, "user-2", "second, different creator", CreateParams{})
	require.NoError(t, err)

	// created_at has second-level resolution in this schema, so force a
	// deterministic order instead of relying on wall-clock gaps.
	_, err = conn.ExecContext(ctx, `UPDATE todos SET created_at = ? WHERE id = ?`, first.CreatedAt.Add(-time.Hour), first.ID)
	require.NoError(t, err)

	list, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2, "List returns every todo, not scoped to one creator (GOAL.md's Ownership model decision)")
	assert.Equal(t, second.ID, list[0].ID, "newest first")
	assert.Equal(t, first.ID, list[1].ID)
}

func TestRepo_Create_TitleTooLongRejectedByStorageLayer(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "user-1", "user-one")
	repo := NewRepo(conn)

	tooLong := make([]byte, 201)
	for i := range tooLong {
		tooLong[i] = 'a'
	}

	_, err := repo.Create(ctx, "user-1", string(tooLong), CreateParams{})
	assert.Error(t, err)
}

func TestRepo_UpdateTitle(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "user-1", "user-one")
	repo := NewRepo(conn)

	created, err := repo.Create(ctx, "user-1", "original title", CreateParams{})
	require.NoError(t, err)

	updated, err := repo.UpdateTitle(ctx, created.ID, "new title")
	require.NoError(t, err)
	assert.Equal(t, "new title", updated.Title)
	assert.Equal(t, created.CreatedAt, updated.CreatedAt, "created_at must not change")
	assert.True(t, !updated.UpdatedAt.Before(created.UpdatedAt))
}

func TestRepo_UpdateStatus(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "user-1", "user-one")
	repo := NewRepo(conn)

	created, err := repo.Create(ctx, "user-1", "task", CreateParams{})
	require.NoError(t, err)

	updated, err := repo.UpdateStatus(ctx, created.ID, StatusInProgress)
	require.NoError(t, err)
	assert.Equal(t, StatusInProgress, updated.Status)
}

func TestRepo_UpdateStatus_InvalidValueRejectedByCheckConstraint(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "user-1", "user-one")
	repo := NewRepo(conn)

	created, err := repo.Create(ctx, "user-1", "task", CreateParams{})
	require.NoError(t, err)

	_, err = repo.UpdateStatus(ctx, created.ID, Status("not_a_real_status"))
	assert.Error(t, err, "todos.status CHECK constraint must reject a value outside the fixed enum")
}

func TestRepo_UpdateAssignee_SetAndClear(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "user-1", "user-one")
	createTestUser(t, conn, "user-2", "user-two")
	repo := NewRepo(conn)

	created, err := repo.Create(ctx, "user-1", "task", CreateParams{})
	require.NoError(t, err)

	updated, err := repo.UpdateAssignee(ctx, created.ID, strPtr("user-2"))
	require.NoError(t, err)
	require.NotNil(t, updated.AssigneeID)
	assert.Equal(t, "user-2", *updated.AssigneeID)

	cleared, err := repo.UpdateAssignee(ctx, created.ID, nil)
	require.NoError(t, err)
	assert.Nil(t, cleared.AssigneeID)
}

func TestRepo_UpdatePriority_SetAndClear(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "user-1", "user-one")
	repo := NewRepo(conn)

	created, err := repo.Create(ctx, "user-1", "task", CreateParams{})
	require.NoError(t, err)

	updated, err := repo.UpdatePriority(ctx, created.ID, strPtr(string(PriorityUrgent)))
	require.NoError(t, err)
	require.NotNil(t, updated.Priority)
	assert.Equal(t, string(PriorityUrgent), *updated.Priority)

	cleared, err := repo.UpdatePriority(ctx, created.ID, nil)
	require.NoError(t, err)
	assert.Nil(t, cleared.Priority)
}

func TestRepo_UpdateDueDate_SetAndClear(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "user-1", "user-one")
	repo := NewRepo(conn)

	created, err := repo.Create(ctx, "user-1", "task", CreateParams{})
	require.NoError(t, err)

	due := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	updated, err := repo.UpdateDueDate(ctx, created.ID, &due)
	require.NoError(t, err)
	require.NotNil(t, updated.DueDate)
	assert.True(t, due.Equal(*updated.DueDate))

	cleared, err := repo.UpdateDueDate(ctx, created.ID, nil)
	require.NoError(t, err)
	assert.Nil(t, cleared.DueDate)
}

// TestRepo_DeleteMethodDoesNotExist — GOAL.md's "DELETE removed" decision,
// mirroring my-task's I12: this is not "unused", it is gone, checked by
// reflection so a future PR that re-adds a Delete method (even one nothing
// currently calls) fails this test rather than silently reintroducing a
// method a future handler could accidentally wire up.
func TestRepo_DeleteMethodDoesNotExist(t *testing.T) {
	_, found := reflect.TypeOf(&Repo{}).MethodByName("Delete")
	assert.False(t, found, "Repo must not have a Delete method — DELETE is retired for this domain (GOAL.md, mirrors my-task's I12)")

	_, found = reflect.TypeOf((*Repository)(nil)).Elem().MethodByName("Delete")
	assert.False(t, found, "the Repository interface must not declare Delete either")
}

// --- todo_events ------------------------------------------------------

func TestRepo_InsertEvent_ComputesMonotonicSeqPerTodo(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "user-1", "user-one")
	repo := NewRepo(conn)

	todo, err := repo.Create(ctx, "user-1", "task", CreateParams{})
	require.NoError(t, err)

	first, err := repo.InsertEvent(ctx, todo.ID, "user-1", EventCreated, nil, nil, "req-1")
	require.NoError(t, err)
	assert.EqualValues(t, 1, first.Seq)

	second, err := repo.InsertEvent(ctx, todo.ID, "user-1", EventCommented, nil, strPtr("hi"), "req-2")
	require.NoError(t, err)
	assert.EqualValues(t, 2, second.Seq)

	// A second, unrelated todo starts its own seq back at 1 — seq is
	// monotonic per todo_id, not global.
	otherTodo, err := repo.Create(ctx, "user-1", "another task", CreateParams{})
	require.NoError(t, err)
	otherFirst, err := repo.InsertEvent(ctx, otherTodo.ID, "user-1", EventCreated, nil, nil, "req-3")
	require.NoError(t, err)
	assert.EqualValues(t, 1, otherFirst.Seq)
}

func TestRepo_GetEventByClientRequestID_HitAndMiss(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "user-1", "user-one")
	repo := NewRepo(conn)

	todo, err := repo.Create(ctx, "user-1", "task", CreateParams{})
	require.NoError(t, err)
	inserted, err := repo.InsertEvent(ctx, todo.ID, "user-1", EventCreated, nil, nil, "req-1")
	require.NoError(t, err)

	got, err := repo.GetEventByClientRequestID(ctx, "req-1")
	require.NoError(t, err)
	assert.Equal(t, inserted, got)

	_, err = repo.GetEventByClientRequestID(ctx, "no-such-request-id")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRepo_ListEventsByTodoID_OldestFirst(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "user-1", "user-one")
	repo := NewRepo(conn)

	todo, err := repo.Create(ctx, "user-1", "task", CreateParams{})
	require.NoError(t, err)
	e1, err := repo.InsertEvent(ctx, todo.ID, "user-1", EventCreated, nil, nil, "req-1")
	require.NoError(t, err)
	e2, err := repo.InsertEvent(ctx, todo.ID, "user-1", EventCommented, nil, strPtr("hi"), "req-2")
	require.NoError(t, err)

	list, err := repo.ListEventsByTodoID(ctx, todo.ID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, e1.ID, list[0].ID, "oldest first")
	assert.Equal(t, e2.ID, list[1].ID)
}

func TestRepo_ListEventsFeed_NewestFirstAcrossTodos_WithJoinFields(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "user-1", "user-one")
	createTestUserWithRole(t, conn, "agent-1", "agent-one", "agent")
	repo := NewRepo(conn)

	todoA, err := repo.Create(ctx, "user-1", "todo A", CreateParams{})
	require.NoError(t, err)
	todoB, err := repo.Create(ctx, "user-1", "todo B", CreateParams{})
	require.NoError(t, err)

	eventA, err := repo.InsertEvent(ctx, todoA.ID, "user-1", EventCreated, nil, nil, "req-a")
	require.NoError(t, err)
	// Force a deterministic ordering (created_at has second-level
	// resolution in this schema).
	_, err = conn.ExecContext(ctx, `UPDATE todo_events SET created_at = ? WHERE id = ?`, eventA.CreatedAt.Add(-time.Hour), eventA.ID)
	require.NoError(t, err)

	eventB, err := repo.InsertEvent(ctx, todoB.ID, "agent-1", EventCreated, nil, nil, "req-b")
	require.NoError(t, err)

	page, err := repo.ListEventsFeed(ctx, nil, nil, 10)
	require.NoError(t, err)
	require.Len(t, page, 2)
	assert.Equal(t, eventB.ID, page[0].Event.ID, "newest first")
	assert.Equal(t, "todo B", page[0].TodoTitle)
	assert.Equal(t, "agent-one", page[0].ActorHandle)
	assert.Equal(t, "agent", page[0].ActorRole)

	assert.Equal(t, eventA.ID, page[1].Event.ID)
	assert.Equal(t, "todo A", page[1].TodoTitle)
	assert.Equal(t, "user-one", page[1].ActorHandle)
	// user-1 was seeded via createTestUser, which defaults to role='agent'
	// in this package's fixtures (see todo_testutil_test.go).
	assert.Equal(t, "agent", page[1].ActorRole)

	// Cursor pagination: page 2, using page 1's last row as the cursor,
	// must return nothing further (only 2 events exist).
	last := page[len(page)-1]
	nextPage, err := repo.ListEventsFeed(ctx, &last.Event.CreatedAt, &last.Event.ID, 10)
	require.NoError(t, err)
	assert.Empty(t, nextPage)
}

// TestRepo_InsertEvent_CreatedAtSurvivesMillisecondWireRoundTrip targets
// the fix itself directly, not just its downstream consequence: proves
// InsertEvent's stored created_at is already millisecond-precision, so
// converting it to the feed's wire cursor format (UnixMilli) and back
// (time.UnixMilli) is lossless — the actual property the pagination fix
// depends on. This is deliberately independent of
// TestRepo_ListEventsFeed_SameMillisecondCollisionPaginatesCorrectly
// below, which proves the query's tie-break is correct GIVEN a genuine
// collision (via a direct SQL overwrite, bypassing InsertEvent's own
// clock entirely) — that test alone would still pass even without this
// fix, since it never exercises InsertEvent's actual precision. Together:
// this test proves storage and wire precision agree; the one below proves
// pagination is correct once they do.
//
// Deliberately re-queries the row with its own fresh SELECT rather than
// trusting InsertEvent's own returned struct (sourced from the INSERT's
// RETURNING clause): RETURNING and a later SELECT are not guaranteed to
// go through identical encode/decode paths in every driver, and the
// fix's actual dependency is on what modernc.org/sqlite persists to and
// reads back from the todo_events table's TIMESTAMP column (SQLite has
// no native timestamp type — the driver's own time.Time <-> stored-value
// mapping is what this test is really checking), not on what a single
// Go struct happened to hold in memory immediately after the call that
// built it.
func TestRepo_InsertEvent_CreatedAtSurvivesMillisecondWireRoundTrip(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "user-1", "user-one")
	repo := NewRepo(conn)

	todo, err := repo.Create(ctx, "user-1", "task", CreateParams{})
	require.NoError(t, err)

	inserted, err := repo.InsertEvent(ctx, todo.ID, "user-1", EventCreated, nil, nil, "req-1")
	require.NoError(t, err)

	var reread time.Time
	require.NoError(t, conn.QueryRowContext(ctx,
		`SELECT created_at FROM todo_events WHERE id = ?`, inserted.ID,
	).Scan(&reread))
	reread = reread.UTC()

	roundTripped := time.UnixMilli(reread.UnixMilli()).UTC()
	assert.Truef(t, reread.Equal(roundTripped),
		"created_at, freshly re-read from the database via its own SELECT (not InsertEvent's own returned "+
			"struct, which only proves the RETURNING clause's value, not a genuine write-then-read round trip "+
			"through the driver) (%v), must equal its own millisecond-wire round trip (%v) — a mismatch here is "+
			"exactly what let the pagination boundary bug through: a cursor rebuilt from the wire format could "+
			"never equal the fuller-precision value it was supposedly reconstructing", reread, roundTripped)
}

// TestRepo_ListEventsFeed_SameMillisecondCollisionPaginatesCorrectly proves
// the millisecond-precision fix by construction, not by repetition.
//
// The bug this guards: InsertEvent used to store time.Now().UTC() at full
// (sub-millisecond) Go precision, while the feed's own wire cursor
// (bff.ActivityCursor.CreatedAtMs, _contract/API.md) is millisecond-only —
// mirroring my-task's own Date.getTime()-based cursor. Reconstructing a
// cursor from that wire value (time.UnixMilli) and comparing it against a
// full-precision stored created_at meant the query's `created_at =
// cursor_created_at` tie-break branch essentially never matched the true
// boundary row, silently dropping any event sharing that row's exact
// millisecond from the next page. TestBFFHandler_ListActivity_
// OrderingAndPagination (internal/transport/bff) hit this ~30% of the time
// under repeated runs — a real, hardware-dependent flake, not a
// theoretical edge case — because it wrote three events back-to-back with
// no artificial delay, which regularly landed two of them in the same
// wall-clock millisecond.
//
// A flaky reproduction proves "it did not happen this time", never "it
// cannot happen" — thirty green runs are still zero proof against the
// thirty-first. This test instead forces two events onto the exact same
// millisecond directly (the same technique the test above already uses to
// force a deterministic ordering) and asserts the page boundary is correct
// unconditionally, every run, by construction.
func TestRepo_ListEventsFeed_SameMillisecondCollisionPaginatesCorrectly(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "user-1", "user-one")
	repo := NewRepo(conn)

	todo, err := repo.Create(ctx, "user-1", "collision todo", CreateParams{})
	require.NoError(t, err)

	older, err := repo.InsertEvent(ctx, todo.ID, "user-1", EventCreated, nil, nil, "req-older")
	require.NoError(t, err)

	first, err := repo.InsertEvent(ctx, todo.ID, "user-1", EventCommented, nil, strPtr("first"), "req-collide-1")
	require.NoError(t, err)
	second, err := repo.InsertEvent(ctx, todo.ID, "user-1", EventCommented, nil, strPtr("second"), "req-collide-2")
	require.NoError(t, err)

	// Force first and second onto the exact same millisecond-truncated
	// instant — genuinely equal, not merely close — regardless of how fast
	// or slow the machine running this test happens to be. Deliberately a
	// value already truncated to millisecond precision (matches what
	// InsertEvent itself now writes), so this test exercises the same
	// invariant the fix establishes, not a stricter one.
	//
	// older is ALSO pinned explicitly, to a value unambiguously before the
	// collision instant — this test's own first version left older's
	// timestamp at whatever wall-clock InsertEvent happened to write, and
	// on a fast enough run older could land in the very same millisecond
	// as first/second too, turning an intended two-way collision into an
	// accidental three-way one and making the rest of this test flaky in
	// exactly the way it exists to rule out. Every timestamp this test's
	// correctness depends on must be pinned; none may be left to the
	// wall clock.
	collisionInstant := time.Now().UTC().Truncate(time.Millisecond)
	olderInstant := collisionInstant.Add(-time.Hour)
	_, err = conn.ExecContext(ctx, `UPDATE todo_events SET created_at = ? WHERE id = ?`, olderInstant, older.ID)
	require.NoError(t, err)
	for _, id := range []string{first.ID, second.ID} {
		_, err := conn.ExecContext(ctx, `UPDATE todo_events SET created_at = ? WHERE id = ?`, collisionInstant, id)
		require.NoError(t, err)
	}

	// Sanity: the collision is real before trusting anything downstream of
	// it — reading the rows back and comparing timestamps directly,
	// independent of whatever the feed query itself does with them.
	var storedCount int
	require.NoError(t, conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM todo_events WHERE created_at = ? AND id IN (?, ?)`,
		collisionInstant, first.ID, second.ID,
	).Scan(&storedCount))
	require.Equal(t, 2, storedCount, "both rows must genuinely share one created_at value before this test proves anything about pagination across it")

	// Page 1, limit 1: newest first, so whichever of {first, second} sorts
	// later by id (the query's own tie-break for equal created_at) comes
	// back, with a non-nil cursor pointing at that exact row.
	page1, err := repo.ListEventsFeed(ctx, nil, nil, 1)
	require.NoError(t, err)
	require.Len(t, page1, 1)
	boundary := page1[0]
	assert.Contains(t, []string{first.ID, second.ID}, boundary.Event.ID)

	// Page 2, following that cursor: must return the OTHER same-millisecond
	// row, not skip straight past both of them to `older`. This is exactly
	// where the bug dropped a row — the truncated, reconstructed cursor
	// failing to `=`-match the full-precision stored value meant the true
	// tie-break sibling was excluded by both branches of the query's WHERE
	// clause.
	page2, err := repo.ListEventsFeed(ctx, &boundary.Event.CreatedAt, &boundary.Event.ID, 1)
	require.NoError(t, err)
	require.Len(t, page2, 1, "the same-millisecond sibling must not be silently dropped")
	otherOfPair := first.ID
	if boundary.Event.ID == first.ID {
		otherOfPair = second.ID
	}
	assert.Equal(t, otherOfPair, page2[0].Event.ID)

	// Page 3: the older, distinctly-timestamped event, and only that —
	// proving the walk continues correctly past the collision rather than
	// re-returning something already seen.
	page3, err := repo.ListEventsFeed(ctx, &page2[0].Event.CreatedAt, &page2[0].Event.ID, 1)
	require.NoError(t, err)
	require.Len(t, page3, 1)
	assert.Equal(t, older.ID, page3[0].Event.ID)

	// No duplicates and no loss across the full walk: exactly the three
	// events inserted, each exactly once.
	seen := map[string]bool{page1[0].Event.ID: true, page2[0].Event.ID: true, page3[0].Event.ID: true}
	assert.Len(t, seen, 3)
}

// --- I15's transaction seam, exercised directly at the repo layer -------

// TestRepo_WithinTx_RollsBackOnError proves WithinTx's own mechanism: a
// write that happens inside the callback, followed by a returned error,
// leaves no trace once WithinTx returns — the real *sql.Tx really rolled
// back, not merely "the callback stopped early." This is the seam
// service.go's Append builds I15's atomicity guarantee on top of;
// TestI15_Append_FailureMidWriteLeavesNeitherEventNorStateChange
// (service_test.go) exercises the same guarantee through Append itself.
func TestRepo_WithinTx_RollsBackOnError(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "user-1", "user-one")
	repo := NewRepo(conn)

	beforeTodos := countRows(t, conn, "todos")

	sentinelErr := assert.AnError
	err := repo.WithinTx(ctx, func(tx Repository) error {
		_, err := tx.Create(ctx, "user-1", "should not survive", CreateParams{})
		require.NoError(t, err, "the insert itself must succeed against the open transaction")
		return sentinelErr
	})
	assert.ErrorIs(t, err, sentinelErr)

	afterTodos := countRows(t, conn, "todos")
	assert.Equal(t, beforeTodos, afterTodos, "the insert made inside the transaction must not persist once WithinTx rolls back")
}

func TestRepo_WithinTx_CommitsOnSuccess(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	createTestUser(t, conn, "user-1", "user-one")
	repo := NewRepo(conn)

	beforeTodos := countRows(t, conn, "todos")

	var createdID string
	err := repo.WithinTx(ctx, func(tx Repository) error {
		created, err := tx.Create(ctx, "user-1", "should survive", CreateParams{})
		if err != nil {
			return err
		}
		createdID = created.ID
		return nil
	})
	require.NoError(t, err)

	afterTodos := countRows(t, conn, "todos")
	assert.Equal(t, beforeTodos+1, afterTodos)

	got, err := repo.GetByID(ctx, createdID)
	require.NoError(t, err)
	assert.Equal(t, "should survive", got.Title)
}

// --- I4: this repo only ever queries the todos table --------------------

// TestI4_TodoRepoOnlyQueriesTodosTable — I4 ("one seam reads identity";
// applied here as "one repo, one table" for the non-identity side of that
// boundary): db/queries/todos.sql must only ever reference the todos
// table, never a table owned by a different domain module (users/
// api_keys) — except through an explicit, mechanically-enforced read-only
// grant (dbquery.ReadOnlyGrants). Unchanged in shape from before this task
// (task-1/milestone-1) other than living in the rewritten repo_test.go —
// restored here rather than dropped, since internal/invariants_test.go's
// TestDoneWhen12 requires a dedicated TestI4_ test inside every domain
// module's own package (per-domain-module scope) and this is that test.
//
// todos.sql and todo_events.sql both belong to the same module
// (dbquery.TableOwnership: both "todo") — a query referencing both is
// automatically I4-legal from that alone, no per-call exemption list
// needed, the same shape internal/identity's own test uses for
// users.sql/api_keys.sql.
//
// todo_events.sql is now ALSO checked as its own subject file below —
// this was NOT true when this comment (and this test's original design)
// was first written. That first version found that ListTodoEventsFeed's
// legitimate JOIN to users (identity's table, for the feed's actor
// handle/role) had no way to be expressed under the old mechanism's
// binary "same module: exempt" / "different module: forbidden" model, and
// resolved it by never checking todo_events.sql as a subject at all —
// which meant this test could not have caught a real violation in that
// file, by construction, and the explanation of why read as diligence
// while carrying the same information as "this file is not examined."
// Flagged as its own finding (not a design note) once recognized as that
// shape; internal/dbquery's redesign (explicit TableOwnership +
// dbquery.ReadOnlyGrants, read-only enforced mechanically, unused grants
// themselves asserted exercised) closes the gap this comment used to
// describe — see internal/dbquery's own doc comment for the full history.
func TestI4_TodoRepoOnlyQueriesTodosTable(t *testing.T) {
	root := repoRootForTests(t)
	queriesDir := filepath.Join(root, "db", "queries")

	dbquery.AssertQueryFileReferencesOnlyOwnTable(t, queriesDir, "todos.sql", "todos")
	dbquery.AssertQueryFileReferencesOnlyOwnTable(t, queriesDir, "todo_events.sql", "todo_events")
}
