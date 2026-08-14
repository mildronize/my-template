-- name: ListTodos :many
-- milestone-4: no owner filter - todos are a shared collection (I3 no
-- longer applies to this domain). Reads every row.
--
-- milestone-4 fix-round (handle-exposure): LEFT JOIN users for the
-- assignee's handle - my-task's own task list/detail views show a plain
-- handle (`t.assignee`, `task.queries.ts`'s `assigneeHandle` LEFT JOIN),
-- never a bare id, and task-7's report found this repo's own
-- `Todo.assigneeId` had no equivalent. LEFT JOIN, not JOIN: assignee_id is
-- nullable (an unassigned todo must still be returned, with a null
-- handle, not dropped). Explicit column list, not SELECT *, for the same
-- star-expansion-safety reason ListTodoEventsFeed's own comment
-- (todo_events.sql) gives for its join. Needs its own ReadOnlyGrant
-- (dbquery.ReadOnlyGrants: "todos.sql" / "users") - this file owns
-- "todos", not "users".
SELECT
    todos.id, todos.created_by, todos.title, todos.status, todos.assignee_id, todos.priority, todos.due_date, todos.created_at, todos.updated_at,
    users.handle AS assignee_handle
FROM todos
LEFT JOIN users ON users.id = todos.assignee_id
ORDER BY todos.created_at DESC;

-- name: CreateTodo :one
INSERT INTO todos (id, created_by, title, status, assignee_id, priority, due_date, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetTodoByID :one
-- milestone-4: no owner scoping - any authenticated actor may read any
-- todo (Ownership model decision, GOAL.md).
--
-- milestone-4 fix-round (handle-exposure): same LEFT JOIN as ListTodos
-- above, same reasoning.
SELECT
    todos.id, todos.created_by, todos.title, todos.status, todos.assignee_id, todos.priority, todos.due_date, todos.created_at, todos.updated_at,
    users.handle AS assignee_handle
FROM todos
LEFT JOIN users ON users.id = todos.assignee_id
WHERE todos.id = ?;

-- name: GetUserHandleByID :one
-- milestone-4 fix-round (handle-exposure): resolves a user id to its
-- handle - used both to fill in a freshly created todo's assignee handle
-- (CreateTodo's own RETURNING * has no join to piggyback on) and to bake
-- a {id, handle} snapshot into the `assigned` event's payload at write
-- time (internal/domain/todo/service.go's Append), matching my-task's own
-- mustGetAssignee
-- (~/gits/my-task/src/server/modules/task/task.service.ts:537-545), which
-- resolves a handle at the exact same moment for the exact same reason:
-- an immutable historical snapshot, not a live join, so a later handle
-- change never rewrites old history. Read-only display lookup, same
-- ReadOnlyGrant as this file's own joins above (todos.sql / users) - not
-- a new grant.
SELECT id, handle FROM users
WHERE id = ?;

-- name: UpdateTodoStatus :one
UPDATE todos
SET status = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: UpdateTodoAssignee :one
UPDATE todos
SET assignee_id = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: UpdateTodoPriority :one
UPDATE todos
SET priority = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: UpdateTodoDueDate :one
UPDATE todos
SET due_date = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: UpdateTodoTitle :one
UPDATE todos
SET title = ?, updated_at = ?
WHERE id = ?
RETURNING *;
