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
SELECT * FROM todo_events
WHERE todo_id = ?
ORDER BY seq ASC;

-- name: ListTodoEventsFeed :many
-- The cross-todo activity feed (`GET /api/bff/activity`,
-- `_contract/API.md`): every event across every todo, newest first,
-- joined to todos (title) and users (actor handle/role, so the caller can
-- mark human vs agent). Cursor-paginated on (created_at, id): the first
-- page passes both cursor columns as NULL; every later page passes the
-- previous page's last row's created_at/id.
SELECT
    sqlc.embed(todo_events),
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
LIMIT ?;
