# API contract — public API surface

Promoted from milestone-1's `_contract/API.md` to `_rules/_contract/` in
milestone-2, same reasoning as `DATA_MODEL.md`/`INVARIANTS.md` — this
surface's conventions are cross-milestone now that a second surface
(`bff`) exists beside it. Milestone-1's copy is historical only.

`internal/transport/publicapi`, mounted at `/api/v1`, OpenAPI-first
(`openapi.yaml` at repo root). Everything authenticates over
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
- No `Idempotency-Key` requirement. Deliberate simplification (this
  service has no append-only event log — see milestone-1's original
  `_goal/GOAL.md` for the full reasoning); re-add it if a fork adds one.
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
    { "id": "…", "prefix": "tpl_a1b2c3d4", "createdAt": "…", "expiresAt": "…" }
] }
```

### `DELETE /api/v1/keys/:id`

Sets `revoked_at`. Owner-scoped, same 404 rule as todos. Deliberately no
`POST /api/v1/keys` and no rotation endpoint — issuance and rotation are
both CLI-only (`docs/DEPLOY-REQUIREMENTS.md` covers the scripts).
