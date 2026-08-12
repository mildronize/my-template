-- name: GetAPIKeyByHash :one
SELECT * FROM api_keys
WHERE key_hash = ?;

-- name: ListAPIKeysByOwner :many
SELECT * FROM api_keys
WHERE user_id = ? AND revoked_at IS NULL
ORDER BY created_at DESC;

-- name: CreateAPIKey :one
INSERT INTO api_keys (id, user_id, key_hash, key_prefix, created_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: RevokeAPIKey :one
UPDATE api_keys
SET revoked_at = ?
WHERE id = ? AND user_id = ? AND revoked_at IS NULL
RETURNING *;
