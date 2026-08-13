-- +goose Up
-- Rebuild-table pattern, not a sequence of ALTER TABLEs, because the two
-- column changes below aren't shape-only:
--   * owner_id -> created_by is a rename, but created_by is NOT NULL and
--     SQLite's ALTER TABLE ADD COLUMN can only backfill a NOT NULL column
--     with a single constant DEFAULT, not "whatever this row's owner_id
--     already is" (DATA_MODEL.md: "a rename with a real consequence, not
--     a no-op").
--   * done (bool) -> status (text) is a value transform per row
--     (`done = true` -> `status = 'done'`, `done = false` -> `status =
--     'open'`), which ALTER TABLE ... ADD COLUMN cannot express at all —
--     it can only add a column, never derive one from another column's
--     existing per-row value.
-- CREATE ... AS SELECT (with the transform in the SELECT list) is the
-- standard way to preserve existing row data through a change ALTER
-- TABLE structurally cannot perform, migrating every existing row (both
-- done states) through in one statement rather than add-then-update.
CREATE TABLE todos_new (
    id          TEXT PRIMARY KEY,
    created_by  TEXT NOT NULL REFERENCES users (id),
    title       TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    status      TEXT NOT NULL DEFAULT 'open'
                    CHECK (status IN ('open', 'in_progress', 'done', 'closed')),
    assignee_id TEXT REFERENCES users (id),
    priority    TEXT CHECK (priority IS NULL OR priority IN ('low', 'medium', 'high', 'urgent')),
    due_date    TIMESTAMP,
    created_at  TIMESTAMP NOT NULL,
    updated_at  TIMESTAMP NOT NULL
);

-- The actual mapping (DATA_MODEL.md, verbatim): done = true -> 'done',
-- done = false -> 'open'. owner_id -> created_by is a plain carry-over of
-- the same value into the new column name, no transform.
INSERT INTO todos_new (id, created_by, title, status, assignee_id, priority, due_date, created_at, updated_at)
SELECT
    id,
    owner_id,
    title,
    CASE WHEN done THEN 'done' ELSE 'open' END,
    NULL,
    NULL,
    NULL,
    created_at,
    updated_at
FROM todos;

DROP TABLE todos;
ALTER TABLE todos_new RENAME TO todos;

-- created_by replaces owner_id's index too: attribution lookups only,
-- no longer access-scoping (I3 no longer applies to this domain).
CREATE INDEX todos_created_by_idx ON todos (created_by);
CREATE INDEX todos_assignee_id_idx ON todos (assignee_id);
CREATE INDEX todos_status_idx ON todos (status);
CREATE INDEX todos_updated_at_idx ON todos (updated_at);

-- New table, mirrors my-task's task_events (DATA_MODEL.md's exact
-- columns/indexes). Append-only (I17) at the application level only — no
-- trigger/constraint enforces it here, deliberately (see INVARIANTS.md
-- I17).
CREATE TABLE todo_events (
    id                 TEXT PRIMARY KEY,
    todo_id            TEXT NOT NULL REFERENCES todos (id),
    seq                INTEGER NOT NULL,
    actor_id           TEXT NOT NULL REFERENCES users (id),
    type               TEXT NOT NULL
                           CHECK (type IN ('created', 'commented', 'status_changed', 'assigned', 'field_changed')),
    payload            TEXT,
    body               TEXT,
    client_request_id  TEXT NOT NULL,
    created_at         TIMESTAMP NOT NULL
);

-- Monotonic per todo_id (I15: seq is computed as max(seq)+1 inside the
-- same transaction as the insert) — the uniqueness is what makes a racing
-- double-insert of the same seq impossible, not just unlikely.
CREATE UNIQUE INDEX todo_events_todo_id_seq_idx ON todo_events (todo_id, seq);
-- The activity feed reads this table alone, newest first, across every
-- todo (DATA_MODEL.md).
CREATE INDEX todo_events_created_at_idx ON todo_events (created_at);
CREATE INDEX todo_events_actor_id_idx ON todo_events (actor_id);
-- I19: the idempotency key. Unique, not just indexed — the lookup at the
-- top of the write path relies on this constraint to make a racing
-- duplicate submit impossible, not just unlikely.
CREATE UNIQUE INDEX todo_events_client_request_id_idx ON todo_events (client_request_id);

-- +goose Down
-- IRRECOVERABLE DATA LOSS: this DROP TABLE discards every todo_events
-- row (the entire activity history) with no backup and no later
-- migration that restores it. A fork that runs this Down loses that
-- history for good, not just until some future Up re-applies — there is
-- nothing left afterward to reconstruct it from. Anyone rolling back
-- this migration on a fork with real data must accept that before
-- running it, not discover it after.
DROP TABLE todo_events;

CREATE TABLE todos_old (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL REFERENCES users (id),
    title      TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    done       BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

-- The reverse mapping has to collapse four status values back into one
-- boolean, and only two of the four collapse "cleanly" (done, open).
-- Every one of the four is named explicitly below, none left to fall
-- out of an ELSE by default — an unexamined ELSE is exactly how this
-- mapping was wrong before (it sent status = 'closed' to done = FALSE):
--   * status = 'done'        -> done = TRUE  (finished work)
--   * status = 'closed'      -> done = TRUE  (also finished work: I18
--     makes 'closed' the terminal, owner-only state that replaces the
--     DELETE this milestone removes — finishing work means moving to
--     'closed', not deleting the row. Mapping it to FALSE would
--     silently turn every real fork's finished, closed todos back into
--     open ones on rollback, and — per the DROP TABLE above — there is
--     no todo_events history left afterward to reconstruct that they
--     were ever closed.)
--   * status = 'open'        -> done = FALSE (not started)
--   * status = 'in_progress' -> done = FALSE (deliberate choice, not a
--     default: the pre-migration schema has no "in progress" boolean
--     state, and of the two available booleans, FALSE — "not yet
--     done" — is the accurate one; work that is in progress is, by
--     definition, not done yet.)
-- The missing ELSE is also deliberate, not an oversight to "complete":
-- a status outside these four sends NULL into a NOT NULL column and the
-- rollback aborts loudly. That's the correct failure mode for a value
-- this mapping doesn't know how to collapse — an unrecognised status
-- must abort, never be guessed at. A future status enum value (this
-- domain's own enum growing, per DATA_MODEL.md's own note that this is
-- a fixed enum only "for now") is exactly the case this protects
-- against, and it's the one likeliest to actually arrive. Do NOT add
-- `ELSE FALSE` "for completeness" — that's precisely how this mapping
-- silently lost 'closed' the first time.
INSERT INTO todos_old (id, owner_id, title, done, created_at, updated_at)
SELECT
    id,
    created_by,
    title,
    CASE
        WHEN status = 'done'        THEN TRUE
        WHEN status = 'closed'      THEN TRUE
        WHEN status = 'open'        THEN FALSE
        WHEN status = 'in_progress' THEN FALSE
    END,
    created_at,
    updated_at
FROM todos;

DROP TABLE todos;
ALTER TABLE todos_old RENAME TO todos;

CREATE INDEX todos_owner_id_idx ON todos (owner_id);
