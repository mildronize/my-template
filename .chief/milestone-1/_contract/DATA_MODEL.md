# Data model

Two tables plus an auth table. No `projects`, no `task_events` — see
`../_goal/GOAL.md` for why. SQLite via sqlc + goose (Decisions table,
`../_goal/GOAL.md`).

## `users`

| Column | Type | Notes |
| --- | --- | --- |
| `id` | text (uuid), pk | |
| `handle` | text, unique, not null | how an actor is referred to; matched exactly, case-insensitively, never fuzzily (matches my-task's convention) |
| `role` | text, not null | `owner` \| `agent` — see INVARIANTS.md I2 |
| `active` | bool, not null, default true | inactive → unauthorized regardless of credential validity |
| `sso_subject` | text, unique, nullable | Hydra's `sub` claim. Null for a row that only ever authenticates via API key |
| `created_at`, `updated_at` | timestamp, not null | |

No `email` — the contract's JIT provisioning (§2/§4) is for the human
sign-in flow, which this template doesn't implement (no browser session, no
login UI — see GOAL.md Scope). Every `users` row here is provisioned
manually (CLI script or seed), same as my-task's agents today.

## `api_keys`

| Column | Type | Notes |
| --- | --- | --- |
| `id` | text (uuid), pk | |
| `user_id` | text, fk → `users.id`, not null | |
| `key_hash` | text, not null | SHA-256 of the raw key. The raw key exists only once, at issuance (CLI stdout) — never stored, never re-derivable |
| `key_prefix` | text, not null | first 8 chars of the raw key, stored in the clear, so a listed key is identifiable without exposing it (`tpl_a1b2c3…`) |
| `created_at` | timestamp, not null | |
| `expires_at` | timestamp, not null | 90-day TTL at issuance, matching my-task's `mtk_` convention |
| `revoked_at` | timestamp, nullable | set by `DELETE /api/v1/keys/:id`; a revoked key fails auth identically to an expired one (I9) |

One user can hold multiple keys (rotation without a gap). Issuance is
CLI-only — see GOAL.md Scope; there is no `POST /api/v1/keys`.

## `todos`

| Column | Type | Notes |
| --- | --- | --- |
| `id` | text (uuid), pk | |
| `owner_id` | text, fk → `users.id`, not null | the actor that created it — this is the field that proves the identity system is load-bearing, not decorative (GOAL.md Objective) |
| `title` | text, not null, 1–200 chars | the only required user-supplied field |
| `done` | bool, not null, default false | |
| `created_at`, `updated_at` | timestamp, not null | |

Index on `owner_id` — every read is scoped to it (I3).

**Deliberately absent:** `project_id`, `priority`, `due_date`, labels, an
`assignee` distinct from `owner_id`, and any event/timeline table. If a
forked service needs these back, `docs/GETTING-STARTED.md` should point at
my-task's `tasks`/`task_events` schema as the fuller reference rather than
this milestone re-deriving it.
