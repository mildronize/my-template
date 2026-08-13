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

-- name: ListAllAgentAPIKeys :many
-- I21 (_contract/INVARIANTS.md): the owner-facing key-listing endpoint
-- (GET /api/bff/keys) needs every role='agent' user's non-revoked keys,
-- not one user_id's own keys - the settings page's whole reason for
-- existing (GOAL.md's "Owner-facing key visibility" decision). A JOIN on
-- users from this file is a same-module reference (both api_keys and
-- users belong to the identity module per internal/dbquery/
-- tableisolation.go's TableOwnership) so it needs no ReadOnlyGrant, unlike
-- todo_events.sql's cross-module JOIN on users.
--
-- Explicit column list (api_keys.*), not a bare SELECT *, so the join
-- against users never leaks a users column into the returned row shape -
-- this query's Go return type must stay exactly db.ApiKey, matching every
-- other query in this file.
SELECT api_keys.*
FROM api_keys
JOIN users ON users.id = api_keys.user_id
WHERE users.role = 'agent' AND api_keys.revoked_at IS NULL
ORDER BY api_keys.created_at DESC;

-- name: RevokeAPIKeyByID :one
-- The owner-facing revoke endpoint's own query (I21): session-gated to a
-- valid owner by the handler above this layer, but not scoped to any
-- particular user_id here - the owner may revoke any agent's key, and
-- there is structurally no api_keys row whose user_id ever belongs to a
-- role='owner' user (I2, cmd/issue-key only ever issues to role='agent'),
-- so no explicit role filter is needed for this to mean exactly "any
-- agent's key".
UPDATE api_keys
SET revoked_at = ?
WHERE id = ? AND revoked_at IS NULL
RETURNING *;
