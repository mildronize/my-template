# `/api/v1` endpoints

Derived from `openapi.yaml` (repo root) and `.chief/_rules/_contract/
API.md` — where a fork's actual code and this file might ever disagree,
`openapi.yaml` is the source of truth, since every request is validated
against it before a handler runs at all (see "Request validation" below).

**`/todos` below is this template's example domain, not a fixed part of
the surface** — update this file for your fork's own paths/fields as part
of `docs/GETTING-STARTED.md` Step 3's rename checklist (`GET /me`, `GET
/keys`, and `DELETE /keys/:id` stay as-is; they're identity, not the
example domain).

Every path below is relative to `$BASE_URL`. Every request carries
`Authorization: Bearer <credential>`. There is no `Idempotency-Key`
requirement on this surface (see the main `SKILL.md`'s "Rules every
request obeys").

---

## `GET /me`

No parameters. Confirms the key and states who it belongs to.

```jsonc
{ "handle": "luna", "role": "agent", "active": true }
```

`role` is always `agent` in practice — an owner's credential is refused
with 401 on this whole surface (I2), so a successful response can never
carry `owner`.

---

## `GET /todos`

No parameters, no pagination — a personal todo list is small (deliberate
simplification, flagged in `_rules/_contract/API.md` as the one place this
contract diverges furthest from `my-task`'s shape). Returns only the
caller's own todos, `created_at` descending.

```jsonc
{ "todos": [
    { "id": "3f9c2e10-…", "title": "Export CSV endpoint", "done": false,
      "createdAt": "2026-08-12T16:34:11Z", "updatedAt": "2026-08-12T16:34:11Z" }
] }
```

---

## `POST /todos` → **201**

```jsonc
{ "title": "Export CSV endpoint" }   // required, 1-200 characters
```

`owner_id` is always the resolved actor — it is never accepted from the
body (I1; the body's own shape can't even carry an `owner`/`actor` field,
since the actor-guard middleware rejects that upstream of the handler).
`done` always starts `false`. Returns the created todo, same shape as
`GET /todos/:id`:

```jsonc
{ "id": "3f9c2e10-…", "title": "Export CSV endpoint", "done": false,
  "createdAt": "2026-08-12T16:34:11Z", "updatedAt": "2026-08-12T16:34:11Z" }
```

A missing or out-of-range `title` is rejected by the OpenAPI request
validator before this handler ever runs — see "Request validation" below.

---

## `GET /todos/:id`

Owner-scoped (I3): another user's id, or an id that never existed, both
return **404 `not_found`** — never `403`, since a `403` would confirm the
row exists at all.

```jsonc
{ "id": "3f9c2e10-…", "title": "Export CSV endpoint", "done": false,
  "createdAt": "2026-08-12T16:34:11Z", "updatedAt": "2026-08-12T16:34:11Z" }
```

---

## `PATCH /todos/:id`

Both fields optional — send only what changed:

```jsonc
{ "title": "…", "done": true }
```

Owner-scoped, same 404 rule as `GET`. Returns the updated todo, same shape
as `GET /todos/:id`.

---

## `DELETE /todos/:id` → **204**

Owner-scoped, same 404 rule. Deleting an already-deleted id is also
`not_found` — naturally idempotent from the caller's point of view, no
special-casing needed on either side.

---

## `GET /keys`

No parameters. Lists the caller's own **non-revoked** keys
(`revoked_at IS NULL`), regardless of expiry — an expired-but-unrevoked
key still shows up here so the caller can see it needs rotating; this is a
separate check from the auth-time expiry check (I9), not a relaxation of
it.

```jsonc
{ "keys": [
    { "id": "8b1e4f2a-…", "prefix": "tpl_a1b2c3d4",
      "createdAt": "2026-05-01T00:00:00Z", "expiresAt": "2026-07-30T00:00:00Z" }
] }
```

`prefix` is the `tpl_` literal plus the first 8 characters of the random
portion — stored in the clear specifically so a listed key is
identifiable without exposing the raw value (I8: the raw key exists only
once, at issuance, and is never stored or re-derivable).

---

## `DELETE /keys/:id` → **204**

Sets `revoked_at`. Owner-scoped, same 404 rule as every other owner-scoped
resource on this surface (I3). There is
deliberately no `POST /api/v1/keys` and no rotation endpoint — see the
main `SKILL.md`'s "Endpoints" section for why rotation specifically can't
be a safe HTTP call.

---

## Request validation

Every request is checked against `openapi.yaml`'s declared shape
(`gin-middleware`'s OpenAPI request validator) **before** it reaches any
handler above — a missing required field or a wrong type never reaches
application code at all, and comes back as `400 validation_error` (see
`errors.md`) with `hint` naming the field, on a best-effort basis. Auth is
deliberately **not** declared as an OpenAPI `security` scheme — credential
resolution is entirely `internal/identity`'s job (I1, I2, I5), enforced as
separate middleware that runs before the OpenAPI validator, not encoded in
the spec.

## Not on this surface

No endpoint issues or rotates a key, and none creates a second domain
resource type beyond what this fork's own `openapi.yaml` documents — a
service forked from this template that adds its own domain module gets
its own endpoints added to `openapi.yaml` directly; check that file first
if this reference and a running instance ever disagree.
