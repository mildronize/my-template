-- +goose Up
-- owner_id has no ON DELETE behavior specified — this template never
-- deletes a users row, only deactivates it (DATA_MODEL.md), so the
-- default RESTRICT is correct and deliberate, not an oversight.
CREATE TABLE todos (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL REFERENCES users (id),
    title      TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    done       BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

-- Every read is scoped to owner_id (I3) — see DATA_MODEL.md.
CREATE INDEX todos_owner_id_idx ON todos (owner_id);

-- +goose Down
DROP TABLE todos;
