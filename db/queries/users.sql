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

-- name: ListActiveUsers :many
-- Every active user, either role -- the assignee-picker's own source
-- (GET /api/bff/users), mirroring my-task's user.ts router exactly
-- (WHERE active = true ORDER BY handle, "humans and agents in one
-- list"). Inactive users are excluded so a picker never offers assigning
-- to someone who can no longer act.
SELECT * FROM users
WHERE active = TRUE
ORDER BY handle;
