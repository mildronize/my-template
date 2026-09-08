# API contract — public API surface

Promoted from milestone-1's `_contract/API.md` to `_rules/_contract/` in
milestone-2, same reasoning as `DATA_MODEL.md`/`INVARIANTS.md` — this
surface's conventions are cross-milestone now that a second surface
(`bff`) exists beside it. Milestone-1's copy is historical only.

`internal/transport/publicapi`, mounted at `/api/v1`, running on
[huma](https://github.com/danielgtaylor/huma) (story-1) — code-first, not
spec-first: `openapi.yaml` (repo root) is generated *from* the handler
code (`go run ./cmd/server openapi`, wired into `make generate`), still
readable without a build per this project's own convention, just no
longer the thing the code is generated from. Everything authenticates over
`Authorization: Bearer <credential>`, where the credential is either an
`api_keys` row's raw key or an SSO-issued JWT (`INVARIANTS.md` I2, I6–I10,
I13–I14). See milestone-2's own `_contract/API.md` for the second surface
(`bff`) this now sits beside, and for what actually changed this
milestone (nothing here — only this surface's code location moved, from
`internal/todo`+`internal/identity` handlers to
`internal/transport/publicapi`).

## Conventions

- `Authorization: Bearer <credential>` on every request. Missing, malformed,
  expired, revoked, wrong-role (`owner`), or inactive-user → **401**, same
  shape regardless of which check failed (I5).
- `me` is the only way an actor refers to itself — there is no endpoint that
  takes another actor's id.
- Any request carrying `actor`, `actorId`, `ownerId`, or `X-Actor` in body,
  query, or header → **400** (I1).
- **Stale as of milestone-4 (TPL-2), corrected here rather than left to
  mislead a later reader:** this bullet used to say "No `Idempotency-Key`
  requirement... this service has no append-only event log... re-add it
  if a fork adds one." Milestone-4 added exactly that log
  (`todo_events`), and `clientRequestId` is now **required**, not absent
  — `openapi.yaml`'s `CreateTodoRequest`/`todo_events`-request schemas
  both list it in `required`, enforced at the write path (I19,
  `INVARIANTS.md`). The reasoning in the old bullet was correct for the
  template *as it stood in milestone-1* — it just stopped being a
  standing simplification the moment the event log this bullet predicted
  actually got added, and nothing updated it at the time. Found by DLV-1,
  this template's first real fork, reading this file as current when it
  wasn't (`docs/GETTING-STARTED.md`'s "One thing already reconsidered,
  and one still to" carried the identical stale claim — fixed there too,
  same source).
- No pagination on `GET /todos` — a personal todo list is small. Flagged
  here rather than silently decided, since it's the one place this
  contract diverges furthest from my-task's API shape.
- There is no key-rotation HTTP endpoint, and no `POST /api/v1/keys` —
  issuance and rotation are both CLI-only (milestone-2 `_contract/API.md`'s
  "Public API — unchanged this milestone" explains why rotation
  specifically can't be a safe HTTP endpoint).

## Error shape

```jsonc
{ "error": {
    "code": "validation_error",
    "message": "title must be 1-200 characters",
    "hint": "title"
} }
```

| Code | Status | When |
| --- | --- | --- |
| `unauthorized` | 401 | any credential failure (I2, I5, I9, I10) |
| `not_found` | 404 | unknown resource id, or one that exists but isn't the caller's (I3 — absence, not 403) |
| `actor_field_present` | 400 | request tried to declare an actor (I1) |
| `validation_error` | 400/422 | `hint` names the field — see the note below for the 400/422 split |
| `invalid_transition` | 403 | a genuine permission refusal for a valid, authenticated credential — distinct from `unauthorized`, since the credential itself is fine. First and currently only case: an agent moving a todo to `closed` (`domain:todo`'s I18). `hint` says what to do instead. |

Both surfaces run on [huma](https://github.com/danielgtaylor/huma)
(story-1). huma's own request-shape validation (missing required field,
wrong type, an unrecognized field on a schema that doesn't allow one)
happens inside its own parse→validate→resolve→handle chain (`huma.go:881`)
— and that whole chain is itself wrapped by every `api.UseMiddleware`,
including the actor-resolution middleware behind `unauthorized` above
(`RequireActor`/`RequireSession`). So for a generic shape failure, actor
resolution's DB lookup runs *first*: an invalid credential still answers
401 regardless of the body's shape, and only once the credential resolves
does huma's own validation get a chance to reject the body/query at 422 —
this is the one deliberate, disclosed ordering delta from this repo's
pre-huma behavior (where a hand-rolled OpenAPI validator rejected shape
violations for every request, authenticated or not, before either check
ran). Huma's own default status for that case is **422**, not 400,
which is the other deliberate, disclosed delta — a generic
OpenAPI-validation rejection used to answer 400 here too — so
`validation_error` now appears at both statuses: **422** for huma's own
generic shape-validation failures, **400** for this repo's own
handler-level validation checks (cross-field rules a schema alone can't
express, e.g. `CreateTodoEventRequest`'s per-`type` dispatch). The
`X-Actor` header/body/query actor-field check (`actor_field_present`) is
the one path that keeps the *old* pre-auth ordering intact: it's a
bespoke check (`internal/transport/humamw`'s `RejectActorFields`),
registered as its own `api.UseMiddleware` *before* the actor-resolution
middleware, that peeks at the raw header/body/query without touching the
database — so a request declaring `actor`/`actorId`/`ownerId`/`X-Actor`
is rejected at 400 before any credential lookup runs, exactly as I1
requires, regardless of what huma's own later validation would have done
with it.

## Endpoints

### `GET /api/v1/me`

```jsonc
{ "handle": "luna", "role": "agent", "active": true }
```

### `GET /api/v1/todos`

Returns only the caller's own todos, `created_at` descending, unpaginated.

```jsonc
{ "todos": [
    { "id": "…", "title": "Export CSV endpoint", "done": false,
      "createdAt": "2026-08-12T16:34:11Z", "updatedAt": "2026-08-12T16:34:11Z" }
] }
```

### `POST /api/v1/todos`

Body: `{ "title": "…" }`. `owner_id` is always the resolved actor — never
accepted from the body (I1). `done` starts `false`.

### `GET /api/v1/todos/:id`

Owner-scoped (I3): another owner's id, or an id that never existed, both
return `not_found`.

### `PATCH /api/v1/todos/:id`

Body: `{ "title"?: "…", "done"?: true }`. Owner-scoped, same 404 rule.

### `DELETE /api/v1/todos/:id`

Owner-scoped, same 404 rule. Deleting an already-deleted id is also
`not_found` (naturally idempotent, no special-casing needed).

### `GET /api/v1/keys`

Lists the caller's own non-revoked keys (`revoked_at IS NULL`), regardless
of expiry — an expired-but-unrevoked key still shows up so the caller can
see it needs rotating.

```jsonc
{ "keys": [
    { "id": "…", "prefix": "tpl_a1b2c3d4", "createdAt": "…", "expiresAt": "…" }
] }
```

### `DELETE /api/v1/keys/:id`

Sets `revoked_at`. Owner-scoped, same 404 rule as todos. Deliberately no
`POST /api/v1/keys` and no rotation endpoint — issuance and rotation are
both CLI-only (`docs/DEPLOY-REQUIREMENTS.md` covers the scripts).
