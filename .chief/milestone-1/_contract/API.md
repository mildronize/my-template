# API contract

One REST surface, `/api/v1`, OpenAPI-first (`openapi.yaml` at repo root —
GOAL.md Decisions). No web UI, no session cookie, no tRPC — everything
authenticates over `Authorization: Bearer <credential>`, where the
credential is either an `api_keys` row's raw key or an SSO-issued JWT
(INVARIANTS.md I2, I6–I10).

## Conventions

- `Authorization: Bearer <credential>` on every request. Missing, malformed,
  expired, revoked, wrong-role (`owner`), or inactive-user → **401**, same
  shape regardless of which check failed (I5).
- `me` is the only way an actor refers to itself — there is no endpoint that
  takes another actor's id.
- Any request carrying `actor`, `actorId`, `ownerId`, or `X-Actor` in body,
  query, or header → **400** (I1).
- No `Idempotency-Key` requirement. **This is a deliberate simplification,
  not an oversight**: my-task requires it because `TaskService.append()`
  writes to an append-only event log where a duplicate write is a
  correctness bug forever. This template has no event log — `PATCH` and
  `DELETE` are naturally idempotent, and a duplicate `POST /todos` produces
  a second todo, which is a normal (if mildly annoying) REST outcome, not a
  data-integrity one. A forked service that re-adds an event log should
  re-add this requirement too.
- No pagination on `GET /todos` in this milestone — a personal todo list is
  small. Flagged here rather than silently decided, since it's the one
  place this contract diverges furthest from my-task's API shape.

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
| `not_found` | 404 | unknown todo id, or a todo id that exists but isn't the caller's (I3 — absence, not 403) |
| `actor_field_present` | 400 | request tried to declare an actor (I1) |
| `validation_error` | 400 | `hint` names the field |

A request that fails OpenAPI spec validation (missing required field, wrong
type) is rejected by `gin-middleware` before reaching handler code at all —
it never reaches the codes above.

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
    { "id": "…", "prefix": "tpl_a1b2c3", "createdAt": "…", "expiresAt": "…" }
] }
```

### `DELETE /api/v1/keys/:id`

Sets `revoked_at`. Owner-scoped, same 404 rule as todos. There is
deliberately no `POST /api/v1/keys` — issuance is CLI-only (GOAL.md Scope,
`docs/DEPLOY-REQUIREMENTS.md` covers the script).
