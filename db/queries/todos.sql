-- name: ListTodosByOwner :many
SELECT * FROM todos
WHERE owner_id = ?
ORDER BY created_at DESC;

-- name: CreateTodo :one
INSERT INTO todos (id, owner_id, title, done, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetTodoByIDAndOwner :one
SELECT * FROM todos
WHERE id = ? AND owner_id = ?;

-- name: UpdateTodoByIDAndOwner :one
UPDATE todos
SET title = ?, done = ?, updated_at = ?
WHERE id = ? AND owner_id = ?
RETURNING *;

-- name: DeleteTodoByIDAndOwner :execrows
DELETE FROM todos
WHERE id = ? AND owner_id = ?;
