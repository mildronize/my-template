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

## `todos` (example domain — shared collection as of milestone-4)

| Column | Type | Notes |
| --- | --- | --- |
| `id` | text (uuid), pk | |
| `created_by` | text, fk → `users.id`, not null | **milestone-4: replaces `owner_id`.** Audit/attribution only — who made it, never who may see or act on it. A todo is visible and actionable by every authenticated actor (I3 no longer applies to this domain — see `INVARIANTS.md` I3's own scope note) |
| `title` | text, not null, 1–200 chars | the only required user-supplied field, unchanged |
| `status` | text, not null, default `'open'` | **milestone-4: replaces `done` (bool).** Fixed enum: `open` \| `in_progress` \| `done` \| `closed`. Not an owner-editable table like my-task's `statuses` — deliberately smaller (`INVARIANTS.md` I18 for the `closed` restriction) |
| `assignee_id` | text, fk → `users.id`, nullable | **milestone-4, new.** Any role — an owner can be assigned same as an agent |
| `priority` | text, nullable | **milestone-4, new.** `low` \| `medium` \| `high` \| `urgent`, my-task's convention |
| `due_date` | timestamp, nullable | **milestone-4, new.** Rendered in the reader's local timezone client-side, never compared to "now" server-side without knowing whose "now" that is — mirrors my-task's own `dueDate` handling |
| `created_at`, `updated_at` | timestamp, not null | unchanged |

Indexes: `created_by` (attribution lookups, no longer access-scoping),
`assignee_id`, `status`, `updated_at` (default list ordering). **No index
on `owner_id`** — that column and its scoping purpose are both gone.

**Migration from milestone-1/2/3's shape, on existing rows** (`INVARIANTS.md`
I15 for the write-path this migration must not bypass): `owner_id` → `created_by`
is a rename with a real consequence, not a no-op — every existing row
becomes visible to every actor once this lands, where before only its
creator could see it (`_goal/milestone-4/_goal/GOAL.md`'s Ownership model
decision — deliberate, but a fork should know it happened, not discover
it). `done = true` rows become `status = 'done'`, **not** `'closed'`;
`done = false` rows become `status = 'open'` — `closed` is reachable only
by an explicit owner action going forward (I18), never assigned by the
migration itself, so that a fork's already-finished todos don't silently
become locked away from every agent that could touch them yesterday.

Any future domain module's table still follows the general
`created_by`/`owner_id`-style attribution pattern from earlier milestones
where that module's own scoping model calls for it — `todos` is the one
domain where milestone-4 deliberately removes the access-scoping half of
it, not a new template default.

## `todo_events` (milestone-4, new — mirrors my-task's `task_events`)

| Column | Type | Notes |
| --- | --- | --- |
| `id` | text (uuid), pk | |
| `todo_id` | text, fk → `todos.id`, not null | |
| `seq` | integer, not null | monotonic per `todo_id`, starts at 1 — computed as `max(seq)+1` inside the same transaction as the insert (I15) |
| `actor_id` | text, fk → `users.id`, not null | from the resolved credential only (I1) — never from request input |
| `type` | text, not null | `created` \| `commented` \| `status_changed` \| `assigned` \| `field_changed` — no `moved`/`labeled`/`unlabeled` (no projects, no labels in this domain). `created` is never client-specifiable — see `_rules/_contract/API.md`'s milestone-4 section |
| `payload` | text, nullable | JSON, shape depends on `type` — `{from, to}` pairs for `status_changed`/`assigned`/`field_changed`, mirroring my-task's own shape |
| `body` | text, nullable | comment text, plain text at write time, Markdown-rendered at read time, never raw HTML (I20) |
| `client_request_id` | text, not null, unique | the Idempotency-Key (I19) |
| `created_at` | timestamp, not null | |

Indexes: `(todo_id, seq)` unique, `created_at` (the activity feed reads
this table alone, newest first, across every todo), `actor_id`.

**Append-only (I17), for everyone, no exceptions.** No `UPDATE`, no
`DELETE`, no soft-delete column — corrections are new events, mirroring
my-task's own I3 exactly, including its enforcement shape: application-level
(no service method/route exists that updates or deletes), not a database
trigger or constraint. See `INVARIANTS.md` I17 for why not going further
than the named source is itself a deliberate choice, not an oversight.

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
