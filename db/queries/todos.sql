-- name: ListTodos :many
-- milestone-4: no owner filter - todos are a shared collection (I3 no
-- longer applies to this domain). Reads every row.
SELECT * FROM todos
ORDER BY created_at DESC;

-- name: CreateTodo :one
INSERT INTO todos (id, created_by, title, status, assignee_id, priority, due_date, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetTodoByID :one
-- milestone-4: no owner scoping - any authenticated actor may read any
-- todo (Ownership model decision, GOAL.md).
SELECT * FROM todos
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
