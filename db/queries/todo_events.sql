-- Every query in this file is named with a "TodoEvent" prefix on
-- purpose, not just for readability: INVARIANTS.md I15 extends
-- internal/architecture_test.go with a check that only
-- internal/domain/todo's own repo file references the generated
-- *TodoEvent*-named query functions - the name is part of the contract
-- that check matches against, not incidental.

-- name: InsertTodoEvent :one
-- The single write path (I15) computes seq as
-- GetTodoEventMaxSeqByTodoID's result + 1 in the same transaction as this
-- insert, and checks GetTodoEventByClientRequestID first (I19). This
-- query only ever appends, never mutates (I17: append-only, no
-- exceptions).
INSERT INTO todo_events (id, todo_id, seq, actor_id, type, payload, body, client_request_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetTodoEventMaxSeqByTodoID :one
-- COALESCE to 0 so a todo with no events yet still returns a usable
-- value (the caller adds 1 unconditionally) instead of NULL/sql.ErrNoRows
-- forcing a special first-event case.
SELECT CAST(COALESCE(MAX(seq), 0) AS INTEGER) AS max_seq FROM todo_events
WHERE todo_id = ?;

-- name: GetTodoEventByClientRequestID :one
-- The idempotency lookup (I19): checked at the top of the write path,
-- inside the same transaction as the insert it would otherwise perform.
-- A hit means "return this row unchanged, write nothing."
SELECT * FROM todo_events
WHERE client_request_id = ?;

-- name: ListTodoEventsByTodoID :many
-- The per-todo timeline (`_contract/API.md`'s milestone-4 section):
-- oldest-first, unlike the cross-todo feed below.
--
-- milestone-4 fix-round (handle-exposure): joins users for the actor's
-- handle/role, the same shape ListTodoEventsFeed below already carries -
-- task-7's own report found this query never had it, leaving the
-- per-todo timeline structurally unable to tell human from agent
-- (Done-when 6/9) or show a name at all (see TimelineEventRow.tsx's own
-- TimelineEventData doc comment, which named this exact gap). Explicit
-- column list, not SELECT *, for the same star-expansion-safety reason
-- ListTodoEventsFeed's own comment above gives for its join. No new
-- ReadOnlyGrant needed - this file already has one for users
-- (dbquery.ReadOnlyGrants: "todo_events.sql" / "users"), earned by
-- ListTodoEventsFeed's own join.
SELECT
    todo_events.id, todo_events.todo_id, todo_events.seq, todo_events.actor_id, todo_events.type, todo_events.payload, todo_events.body, todo_events.client_request_id, todo_events.created_at,
    users.handle AS actor_handle,
    users.role AS actor_role
FROM todo_events
JOIN users ON users.id = todo_events.actor_id
WHERE todo_events.todo_id = ?
ORDER BY todo_events.seq ASC;

-- name: ListTodoEventsFeed :many
-- The cross-todo activity feed (`GET /api/bff/activity`,
-- `_contract/API.md`): every event across every todo, newest first,
-- joined to todos (title) and users (actor handle/role, so the caller can
-- mark human vs agent). Cursor-paginated on (created_at, id): the first
-- page passes both cursor columns as NULL; every later page passes the
-- previous page's last row's created_at/id.
--
-- Explicit column list (not sqlc.embed) and sqlc.arg(limit) (not a bare
-- anonymous placeholder) on purpose, not just style: sqlc.embed was
-- observed to reserve a phantom numbered-placeholder slot ahead of the
-- first sqlc.narg below, which pushed every later placeholder's number
-- one higher than the generated Go function's own argument order
-- accounts for. modernc.org/sqlite binds database/sql's positional args
-- strictly by call order to the SQL text's own numbered placeholders, not
-- by re-deriving numbers from how many args were passed - so a query
-- whose numbering starts one higher than expected silently drops the
-- first bound arg onto an unused slot and leaves the highest-numbered
-- placeholder (LIMIT) with nothing bound to it at all ("missing argument
-- with index N"), rather than erroring at generate time. Keeping every
-- placeholder inside sqlc's own numbering (no bare anonymous placeholder)
-- and avoiding sqlc.embed here keeps the numbering contiguous from the
-- start, matching ListTodoEventsFeedParams field order exactly.
-- (Also: plain ASCII only in this comment block, never an em dash or any
-- other non-ASCII byte, immediately above a SELECT line - task-1's own
-- report found that an em dash there corrupts sqlc v1.31.1's
-- star-expansion byte offsets; this query hit a variant of the same
-- corruption during task-2 until this comment's em dashes were replaced.
-- Wider than em dashes specifically: DLV-1's first real fork hit the
-- identical corruption with a section-sign character and with Thai prose
-- in a different db/queries/*.sql file. internal/db_queries_ascii_test.go
-- is the actual floor now - it fails loudly on any non-ASCII byte
-- anywhere under db/queries/, not just here, so this paragraph is
-- historical context for why the rule exists, not the only thing
-- enforcing it.)
SELECT
    todo_events.id, todo_events.todo_id, todo_events.seq, todo_events.actor_id, todo_events.type, todo_events.payload, todo_events.body, todo_events.client_request_id, todo_events.created_at,
    todos.id AS todo_id_ref,
    todos.title AS todo_title,
    users.id AS actor_user_id,
    users.handle AS actor_handle,
    users.role AS actor_role
FROM todo_events
JOIN todos ON todos.id = todo_events.todo_id
JOIN users ON users.id = todo_events.actor_id
WHERE (
    sqlc.narg(cursor_created_at) IS NULL
    OR todo_events.created_at < sqlc.narg(cursor_created_at)
    OR (todo_events.created_at = sqlc.narg(cursor_created_at) AND todo_events.id < sqlc.narg(cursor_id))
)
ORDER BY todo_events.created_at DESC, todo_events.id DESC
LIMIT sqlc.arg(limit);
