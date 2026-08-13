# Data model

Promoted from milestone-1's `_contract/DATA_MODEL.md` to `_rules/_contract/`
in milestone-2 — the schema is genuinely cross-milestone (milestone-2 adds
key-lifecycle behavior and an owner login on top of it, doesn't replace
it), and `AGENTS.md`'s own directory convention names `_rules/_contract/`
for exactly this: "data models, API contracts, schemas" shared across
milestones, not owned by whichever milestone happened to write them first.
Milestone-1's copy is historical only from here — this file is the live
authority.

Three tables. No `projects`, no `task_events` — see milestone-1's
`_goal/GOAL.md` for why. SQLite via sqlc + goose.

## `users`

| Column | Type | Notes |
| --- | --- | --- |
| `id` | text (uuid), pk | |
| `handle` | text, unique, not null | how an actor is referred to; matched exactly, case-insensitively, never fuzzily (matches my-task's convention) |
| `role` | text, not null | `owner` \| `agent` — see `INVARIANTS.md` I2, I12 |
| `active` | bool, not null, default true | inactive → unauthorized regardless of credential validity |
| `sso_subject` | text, unique, nullable | Hydra's `sub` claim. Null for a row that only ever authenticates via API key |
| `created_at`, `updated_at` | timestamp, not null | |

**Owner provisioning — milestone-2 addition, decided by the same pattern
my-task actually uses, not the JIT capability the contract merely allows.**
Milestone-1 left `sso_subject` unused in practice (no login path existed).
Milestone-2 adds one, but the owner row is still seeded, not JIT-created
on first login — mirrors my-task's actual seed (`handle=mild,
ssoSubject=wizthear102`, set in advance from a known Hydra `sub`), not the
JIT-for-humans capability `sso-consumer-contract.md` §2/§4 merely permits.
Reasoning: a template's owner is a single, known person per deployment
(whoever forked it), not an open population — provisioning that one row at
seed time is simpler than a JIT-creation path this service would only ever
exercise once, and keeps "creates identity, never authority" (§4) true by
construction rather than by a runtime check. `SEED_OWNER_SSO_SUBJECT`
(config, set once at fork/deploy time) is what the seed script uses to
provision this row — see `_plan/task specs` for the exact env var.

## `api_keys`

| Column | Type | Notes |
| --- | --- | --- |
| `id` | text (uuid), pk | |
| `user_id` | text, fk → `users.id`, not null | |
| `key_hash` | text, not null | SHA-256 of the raw key. The raw key exists only once, at issuance (CLI stdout) — never stored, never re-derivable |
| `key_prefix` | text, not null | the `tpl_` literal plus the first 8 characters of the random portion (12 chars total, e.g. `tpl_a1b2c3d4`), stored in the clear, so a listed key is identifiable without exposing it |
| `created_at` | timestamp, not null | |
| `expires_at` | timestamp, not null | 90-day TTL at issuance, matching my-task's `mtk_` convention |
| `revoked_at` | timestamp, nullable | set by `revoke` or by `rotate`'s disable-old step; a revoked key fails auth identically to an expired one (I9) |

One user can hold multiple keys. Milestone-2 adds `list`/`rotate` on top
of milestone-1's issue/revoke — schema unchanged, only which rows exist
and when changes (see `INVARIANTS.md` I13 for `rotate`'s issue-before-
disable ordering). Issuance is still CLI-only; there is still no
`POST /api/v1/keys`.

## `todos` (example domain — moved, not changed)

| Column | Type | Notes |
| --- | --- | --- |
| `id` | text (uuid), pk | |
| `owner_id` | text, fk → `users.id`, not null | the actor that created it — proves the identity system is load-bearing, not decorative |
| `title` | text, not null, 1–200 chars | the only required user-supplied field |
| `done` | bool, not null, default false | |
| `created_at`, `updated_at` | timestamp, not null | |

Index on `owner_id` — every read is scoped to it (I3). **Table shape is
unchanged from milestone-1** — only its code location moves, from
`internal/todo/` to `internal/domain/todo/`, per `_rules/_standard/
ARCHITECTURE.md`'s milestone-2 restructure. Any future domain module's
table follows the same `owner_id TEXT NOT NULL REFERENCES users (id)`
pattern — this is the column that makes ownership-scoping (I3) possible at
all, and its absence would leave that domain module unable to satisfy I3.

## BFF session — no new table, decided minimal on purpose

The owner login (milestone-2 item 3) needs *some* session mechanism once
the OIDC callback succeeds. **No `sessions` table.** A signed (HMAC or
equivalent), short-lived cookie carrying the resolved `users.id` is the
whole mechanism — no server-side session store to provision, expire, or
clean up. This matches the milestone's own scope for the BFF ("session
handling only as far as serving one authenticated page requires... not a
UI framework") — a session store is the kind of infrastructure a real
frontend needs and this explicitly isn't one. If a fork's BFF grows beyond
"one authenticated view" this decision should be revisited there, not
inherited by default.
