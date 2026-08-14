# Task task-1 Report

## Task
Schema migration for todos (status/assignee_id/priority/due_date/
created_by, replacing done/owner_id) plus the new `todo_events` table,
the sqlc queries both need, and a migration test proving the `done`/
`owner_id` -> `status`/`created_by` mapping against pre-existing rows in
both `done` states — not just that the migration runs on an empty
database. This task's own scope ends there: `internal/domain/todo/
service.go`, both transports, and every existing todo test still
reference `owner_id`/`done` and are task-2's to fix.

## Outcome
done

## Decision

- Issue: `created_by` is `NOT NULL` and must carry each row's existing
  `owner_id` value; `status` is `NOT NULL` and must be *derived per row*
  from the old `done` boolean. SQLite's `ALTER TABLE ... ADD COLUMN` can
  backfill a `NOT NULL` column only with a single constant `DEFAULT` —
  it cannot say "whatever this row's `owner_id` already is" and it
  cannot express a per-row value transform at all. The existing
  migrations in this repo (`create_users_and_api_keys.sql`,
  `create_todos.sql`) are both pure `CREATE TABLE`s and gave no
  precedent for this constraint.
- Chosen: the rebuild-table pattern — `CREATE TABLE todos_new (final
  shape)`, `INSERT INTO todos_new (...) SELECT ..., CASE WHEN done THEN
  'done' ELSE 'open' END, ... FROM todos`, `DROP TABLE todos`, `ALTER
  TABLE todos_new RENAME TO todos`, then recreate indexes. This is the
  only shape that can express "each row's `status` depends on that same
  row's `done`" while running as ordinary SQL goose applies as one `Up`
  block. Wrote a symmetric `Down` (rebuild back to `owner_id`/`done`,
  reverse-mapping `status = 'done'` -> `done = TRUE`, else `FALSE`) so
  the migration is reversible, matching the existing migrations' own
  `+goose Down` convention.
- Added `CHECK` constraints on `status` (the fixed four-value enum),
  `priority` (the fixed four-value enum or NULL), and `todo_events.type`
  (the fixed five-value enum) — not explicitly required by the task spec,
  but matches this repo's own established style (`todos.title`'s
  existing `CHECK (length(title) BETWEEN 1 AND 200)`) and the domain
  model's own framing of these as fixed enums, not owner-editable tables.
- Naming for the `todo_events` sqlc queries: every one of them is
  `TodoEvent`-prefixed on purpose, for I15's architecture-test
  name-matcher (task-2's to build) to match against by substring. Chose
  five: `InsertTodoEvent`, `GetTodoEventMaxSeqByTodoID`,
  `GetTodoEventByClientRequestID`, `ListTodoEventsByTodoID`,
  `ListTodoEventsFeed` — comfortably above I15's stated floor of 3
  (insert, list-per-todo, list-cross-todo-feed). `seq` computation is a
  read-then-insert pair (`GetTodoEventMaxSeqByTodoID` then
  `InsertTodoEvent` with `seq = max+1`, both inside task-2's single
  write-path transaction) rather than one combined query — no existing
  read-then-insert precedent in this codebase to mirror, and SQLite's
  syntax for "insert with a computed default from another query" inside
  one statement (`INSERT ... SELECT ... FROM (SELECT MAX(seq)...)`) reads
  worse than two named, individually-testable queries for a value this
  security/correctness-sensitive (I15's monotonic `seq` requirement).
  `GetTodoEventByClientRequestID` is I19's idempotency lookup, called
  first in the same write-path transaction. `ListTodoEventsFeed` uses
  `sqlc.narg` for the cursor pair (`cursor_created_at`, `cursor_id`) so
  the first page passes `NULL` and every later page passes the previous
  page's last row — confirmed sqlc v1.31.1 supports `sqlc.narg` cleanly
  against the SQLite engine.
- **A real sqlc bug hit while writing the queries, not a design
  decision, but worth recording**: em dashes (`—`) inside SQL comments
  immediately preceding a `SELECT *`/`RETURNING *` line broke sqlc
  v1.31.1's star-expansion — it computes the byte offset to splice the
  expanded column list into by counting characters in a way that
  disagrees with the multi-byte UTF-8 em dash, corrupting the
  regenerated query text (`SELECT *` became `SELECid` in one case). Fix:
  every SQL comment in `db/queries/todos.sql` and `db/queries/
  todo_events.sql` uses plain ASCII hyphens instead. Flagging this in
  case a later task's own query file edits reach for an em dash near a
  `*` line and reproduce the same corruption.

## Notes — exact sqlc query function names (for task-2)

`db/queries/todos.sql`: `ListTodos` (no owner filter — reads every row),
`CreateTodo`, `GetTodoByID`, `UpdateTodoStatus`, `UpdateTodoAssignee`,
`UpdateTodoPriority`, `UpdateTodoDueDate`, `UpdateTodoTitle` — one
single-field update query per field task-2's write path needs, matching
this milestone's one-event-type-per-field-change shape rather than one
combined multi-field `UPDATE`.

`db/queries/todo_events.sql`: `InsertTodoEvent`,
`GetTodoEventMaxSeqByTodoID` (returns `int64`, `COALESCE(MAX(seq),0)` so
a todo with zero events still returns a usable value — caller adds 1
unconditionally), `GetTodoEventByClientRequestID` (the I19 idempotency
lookup), `ListTodoEventsByTodoID` (oldest-first, per `_contract/API.md`'s
milestone-4 section), `ListTodoEventsFeed` (`ListTodoEventsFeedParams{
CursorCreatedAt, CursorID, Limit}`, returns
`[]ListTodoEventsFeedRow{TodoEvent, TodoIDRef, TodoTitle, ActorUserID,
ActorHandle, ActorRole}`, newest-first, cursor-paginated).

`db.Todo` struct: `ID, CreatedBy, Title, Status, AssigneeID
(sql.NullString), Priority (sql.NullString), DueDate (sql.NullTime),
CreatedAt, UpdatedAt` — `OwnerID`/`Done` are gone. `db.TodoEvent`
struct: `ID, TodoID, Seq (int64), ActorID, Type, Payload
(sql.NullString), Body (sql.NullString), ClientRequestID, CreatedAt`.

## Migration mapping — exact SQL (`db/migrations/
20260813100000_todo_activity_log.sql`, `+goose Up`)

```sql
CREATE TABLE todos_new (
    id          TEXT PRIMARY KEY,
    created_by  TEXT NOT NULL REFERENCES users (id),
    title       TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    status      TEXT NOT NULL DEFAULT 'open'
                    CHECK (status IN ('open', 'in_progress', 'done', 'closed')),
    assignee_id TEXT REFERENCES users (id),
    priority    TEXT CHECK (priority IS NULL OR priority IN ('low', 'medium', 'high', 'urgent')),
    due_date    TIMESTAMP,
    created_at  TIMESTAMP NOT NULL,
    updated_at  TIMESTAMP NOT NULL
);

INSERT INTO todos_new (id, created_by, title, status, assignee_id, priority, due_date, created_at, updated_at)
SELECT
    id,
    owner_id,
    title,
    CASE WHEN done THEN 'done' ELSE 'open' END,
    NULL, NULL, NULL,
    created_at,
    updated_at
FROM todos;

DROP TABLE todos;
ALTER TABLE todos_new RENAME TO todos;

CREATE INDEX todos_created_by_idx ON todos (created_by);
CREATE INDEX todos_assignee_id_idx ON todos (assignee_id);
CREATE INDEX todos_status_idx ON todos (status);
CREATE INDEX todos_updated_at_idx ON todos (updated_at);

CREATE TABLE todo_events (
    id                 TEXT PRIMARY KEY,
    todo_id            TEXT NOT NULL REFERENCES todos (id),
    seq                INTEGER NOT NULL,
    actor_id           TEXT NOT NULL REFERENCES users (id),
    type               TEXT NOT NULL
                           CHECK (type IN ('created', 'commented', 'status_changed', 'assigned', 'field_changed')),
    payload            TEXT,
    body               TEXT,
    client_request_id  TEXT NOT NULL,
    created_at         TIMESTAMP NOT NULL
);

CREATE UNIQUE INDEX todo_events_todo_id_seq_idx ON todo_events (todo_id, seq);
CREATE INDEX todo_events_created_at_idx ON todo_events (created_at);
CREATE INDEX todo_events_actor_id_idx ON todo_events (actor_id);
CREATE UNIQUE INDEX todo_events_client_request_id_idx ON todo_events (client_request_id);
```

(Full file, including the reversing `+goose Down`, at the path above.)

## Verification

- **Migration test (the task's actual deliverable)** —
  `internal/platform/migrate_todo_activity_log_test.go`,
  `TestTodoActivityLogMigration_PreservesExistingRows`: applies every
  migration up to (not including) this one, inserts one `done=true` and
  one `done=false` `todos` row directly at the pre-migration shape (real
  `owner_id`), applies this migration, asserts `status='done'`/
  `created_by='user-1'` and `status='open'`/`created_by='user-1'`
  respectively, and asserts `owner_id`/`done` no longer exist as columns
  and `todo_events` exists. Ran in isolation:
  ```
  === RUN   TestTodoActivityLogMigration_PreservesExistingRows
  --- PASS: TestTodoActivityLogMigration_PreservesExistingRows (0.01s)
  PASS
  ok  	github.com/mildronize/my-template/internal/platform	0.014s
  ```
- Full `internal/platform` package (includes the pre-existing
  `TestGooseUp_FullMigrationSetAppliesCleanly`, which re-verifies the
  full migration set — now four migrations — still applies cleanly to an
  empty database): all green, `ok`.
- `go build ./internal/db/...` — clean (sqlc output compiles standalone).
- `go build ./...` — **fails, expected**: exactly one root cause,
  `internal/domain/todo/repo.go` (10 errors: `row.OwnerID`, `row.Done`,
  `ListTodosByOwner`, `GetTodoByIDAndOwner`, `UpdateTodoByIDAndOwner`,
  `DeleteTodoByIDAndOwner`, and their `*Params` types — all gone from
  `internal/db` now, all still referenced by `repo.go`). Confirmed this
  is the *only* root cause, not independent breakage per package: built
  `internal/transport/publicapi`, `internal/transport/bff`, `cmd/server`,
  and `internal/domain/todo` individually — every one reports the exact
  same `internal/domain/todo/repo.go` error set (they fail transitively
  through their import of `internal/domain/todo`, not on their own
  code). `go vet` on the full package set confirms the same single
  package name in its `#` failure markers.
- `gofmt -l` on every file this task touched — clean.
- `git status` confirms no file outside `db/migrations/`, `db/queries/`,
  `internal/db/` (generated), and the one new test file was touched —
  `internal/domain/todo/service.go`, every handler, and every pre-existing
  test are untouched, as scoped.

## Commit pushed (branch `milestone-4/activity-log`)

- `d0f7a9c` — `feat(milestone-4/task-1): todos gain status/assignee/priority/due_date/created_by, new todo_events table`

## Fix-round note — Down mapping silently unfinished closed todos on rollback

Clara found a real defect in the `+goose Down` mapping in this same
migration file during review, verified and reported after this task's
original report above was written.

**The defect**: Down mapped `status` back to the old `done` boolean as
`CASE WHEN status = 'done' THEN TRUE ELSE FALSE END`. The Up side
correctly maps two `done` states to four `status` values
(`open`/`in_progress`/`done`/`closed`); the reverse has to collapse
those four back into two, and the `ELSE` branch collapsed
`status = 'closed'` into `done = FALSE`. Per `INVARIANTS.md` I18,
`closed` is a terminal, owner-only state, and per I12/มายด์'s ruling
this milestone, it is the direct replacement for the `DELETE` this
milestone removes — finishing work means moving to `closed`, not
deleting the row. Rolling back this migration on a fork with real
closed todos would silently turn every one of them back into an
*unfinished* (`done = FALSE`) todo, and — because Down's first
statement is `DROP TABLE todo_events` — there would be nothing left
afterward to reconstruct the true prior state from. Same shape of bug
as the original defect the Up mapping was attacked for: an unexamined
`ELSE` doing something no one decided on purpose.

**The fix** (`db/migrations/20260813100000_todo_activity_log.sql`,
`+goose Down`) — every one of the four status values named explicitly,
none left to an `ELSE`:

```sql
CASE
    WHEN status = 'done'        THEN TRUE
    WHEN status = 'closed'      THEN TRUE
    WHEN status = 'open'        THEN FALSE
    WHEN status = 'in_progress' THEN FALSE
END
```

`done`/`closed` both collapse to `TRUE` (both are finished states);
`open`/`in_progress` both collapse to `FALSE`, with `in_progress ->
FALSE` justified in an inline SQL comment as a deliberate call ("work
that is in progress is, by definition, not done yet"), not a default
that happened to fall out. A second comment now sits directly above
`DROP TABLE todo_events` stating plainly that this is irrecoverable
data loss — a fork meets that fact at the point of the decision
(reading the migration), not after running it.

**New test** —
`internal/platform/migrate_todo_activity_log_test.go`,
`TestTodoActivityLogMigration_DownCollapsesFourStatusesToBoolean`:
migrates up through this migration (`postActivityLogMigrationVersion`),
seeds one `todos` row in each of the four post-migration `status`
values with a real `created_by`, rolls back via `goose.DownTo(conn,
migrationsDir, preActivityLogMigrationVersion)`, and asserts the exact
resulting `done` boolean per prior status. Also asserts `created_by`/
`status` no longer exist as columns and `todo_events` no longer exists
after Down. Passing in isolation alongside the original Up test:

```
=== RUN   TestTodoActivityLogMigration_PreservesExistingRows
--- PASS: TestTodoActivityLogMigration_PreservesExistingRows (0.01s)
=== RUN   TestTodoActivityLogMigration_DownCollapsesFourStatusesToBoolean
--- PASS: TestTodoActivityLogMigration_DownCollapsesFourStatusesToBoolean (0.01s)
PASS
ok  	github.com/mildronize/my-template/internal/platform	0.023s
```

**Attack-and-revert**, same standard as the original Up-side attack in
this task: temporarily mutated the fixed migration's `closed` line
from `THEN TRUE` back to `THEN FALSE` (the original bug's exact
behavior), re-ran the new Down test.

- Confirmed the mutation was a real semantic change, not a syntax
  break: goose logged the Down migration applying successfully
  (`OK   20260813100000_todo_activity_log.sql (1.77ms)`) — the mutated
  SQL is valid and runs, it just computes the wrong value.
- The new test failed on exactly the `closed` row's assertion and
  nowhere else:
  ```
  migrate_todo_activity_log_test.go:217:
      Error:      Should be true
      Test:       TestTodoActivityLogMigration_DownCollapsesFourStatusesToBoolean
      Messages:   status='closed' must roll back to done=TRUE — 'closed' is a
                  terminal, finished state (I18/I12), not a deleted or
                  unfinished one, and Down also drops todo_events, so there
                  is no history left to reconstruct the true prior state
                  from if this collapses wrong
  --- FAIL: TestTodoActivityLogMigration_DownCollapsesFourStatusesToBoolean (0.01s)
  ```
  The `open`, `in_progress`, and `done` row assertions all still
  passed — the failure isolates to precisely the mutated line.
- Reverted the mutation (`THEN FALSE` -> `THEN TRUE`); `diff` against
  the pre-mutation file showed the reverted file byte-identical to the
  fixed version. Re-ran both tests: both green again (output above).

**Scope check**: `git status --porcelain` before commit showed exactly
two modified files — `db/migrations/20260813100000_todo_activity_log.sql`
and `internal/platform/migrate_todo_activity_log_test.go` — nothing
under `internal/domain/todo/`, `internal/transport/`, or
`internal/architecture_test.go` touched, per task-2's concurrent claim
on that territory. `go build ./...` still fails with the same single
pre-existing root cause as before this fix (`internal/domain/todo/
repo.go` referencing columns/queries task-2 hasn't updated yet) —
unrelated to and unaffected by this change; `internal/platform` itself
builds and tests clean.

**Commit pushed** (branch `milestone-4/activity-log`, rebased onto
`origin` first — no concurrent commits landed in between):

- `ab61cd1` — `fix(milestone-4/task-1): Down mapping silently unfinished closed todos on rollback`
