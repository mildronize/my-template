-- name: GetUserByID :one
SELECT * FROM users
WHERE id = ?;

-- name: GetUserByHandle :one
-- handle is COLLATE NOCASE at the schema level (see migration), so this
-- comparison is case-insensitive without needing an explicit COLLATE here.
SELECT * FROM users
WHERE handle = ?;

-- name: GetUserBySSOSubject :one
SELECT * FROM users
WHERE sso_subject = ?;

-- name: CreateUser :one
INSERT INTO users (id, handle, role, active, sso_subject, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;
