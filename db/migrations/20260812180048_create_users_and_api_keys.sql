-- +goose Up
-- handle is declared COLLATE NOCASE so every comparison against it (the
-- unique index below, and any `WHERE handle = ?` query) is
-- case-insensitive by default without callers having to remember to add
-- `COLLATE NOCASE` themselves (DATA_MODEL.md: "matched exactly,
-- case-insensitively, never fuzzily").
CREATE TABLE users (
    id          TEXT PRIMARY KEY,
    handle      TEXT NOT NULL COLLATE NOCASE,
    role        TEXT NOT NULL,
    active      BOOLEAN NOT NULL DEFAULT TRUE,
    sso_subject TEXT,
    created_at  TIMESTAMP NOT NULL,
    updated_at  TIMESTAMP NOT NULL
);

CREATE UNIQUE INDEX users_handle_idx ON users (handle);
CREATE UNIQUE INDEX users_sso_subject_idx ON users (sso_subject) WHERE sso_subject IS NOT NULL;

CREATE TABLE api_keys (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users (id),
    key_hash   TEXT NOT NULL,
    key_prefix TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    revoked_at TIMESTAMP
);

CREATE UNIQUE INDEX api_keys_key_hash_idx ON api_keys (key_hash);
CREATE INDEX api_keys_user_id_idx ON api_keys (user_id);

-- +goose Down
DROP TABLE api_keys;
DROP TABLE users;
