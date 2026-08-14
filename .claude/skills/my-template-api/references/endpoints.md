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
`Authorization: Bearer <credential>`. Every `POST`/`PATCH` carries a
`clientRequestId` too (I19) — see the main `SKILL.md`'s "Rules every
request obeys".

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

No parameters, no pagination (deliberate simplification, flagged in
`_rules/_contract/API.md`). **Every todo, not just the caller's own** —
milestone-4 made todos a shared collection every agent and the owner act
on together (I3 no longer applies here); `created_at` descending.

```jsonc
{ "todos": [
    { "id": "3f9c2e10-…", "title": "Export CSV endpoint",
      "status": "open", "assigneeId": null, "assigneeHandle": null,
      "priority": "high", "dueDate": null,
      "createdBy": "8b1e4f2a-…",
      "createdAt": "2026-08-12T16:34:11Z", "updatedAt": "2026-08-12T16:34:11Z" }
] }
```

`status` is a fixed four-value enum: `open` | `in_progress` | `done` |
`closed` — not an owner-editable table. `createdBy` is attribution only
(who made it), never access-scoping — every caller sees and can act on
every todo regardless of who created it.

---

## `POST /todos` → **201**

```jsonc
{
  "title": "Export CSV endpoint",       // required, 1-200 characters
  "clientRequestId": "…",               // required (I19)
  "assigneeId": null,                   // optional
  "priority": "high",                   // optional: low | medium | high | urgent
  "dueDate": null                       // optional, ISO 8601
}
```

`createdBy` is always the resolved actor — never accepted from the body
(I1; the actor-guard middleware rejects `owner`/`actor`/`createdBy`
upstream of the handler). **`status` is not accepted here** — every new
todo starts `open`, deliberately (change it afterward through an event,
below); sending `done` (the pre-milestone-4 field) is a
`400 validation_error`, not silently dropped. Returns the created todo,
same shape as `GET /todos/:id`.

A missing or out-of-range `title`, or a stray unrecognised field, is
rejected by the OpenAPI request validator before this handler ever runs —
see "Request validation" below.

---

## `GET /todos/:id`

**No longer owner-scoped** — any authenticated caller may read any todo
by id (I3 no longer applies to this domain). An id that never existed is
**404 `not_found`** — there's no "wrong owner" case left to distinguish
from "never existed", so there's nothing for a `403` to protect either.

```jsonc
{ "id": "3f9c2e10-…", "title": "Export CSV endpoint",
  "status": "open", "assigneeId": null, "assigneeHandle": null,
  "priority": "high", "dueDate": null,
  "createdBy": "8b1e4f2a-…",
  "createdAt": "2026-08-12T16:34:11Z", "updatedAt": "2026-08-12T16:34:11Z" }
```

---

## `PATCH /todos/:id`

**Renames only, as of milestone-4** — `status`/`assigneeId`/`priority`/
`dueDate` all moved to `POST .../events` below, the single write path
their permission/audit requirements actually need (I15/I18):

```jsonc
{ "title": "…", "clientRequestId": "…" }   // both required
```

Sending `done` is a `400 validation_error`, same reasoning as `POST`
above. No longer owner-scoped, same 404 rule as `GET`. Returns the
updated todo, same shape as `GET /todos/:id`.

---

## `POST /todos/:id/events` → **201** — append to this todo's timeline (I15)

**There is no `DELETE /todos/:id` any more** — removed in milestone-4.
Finishing a todo means posting a `status_changed` event with `to: "closed"`
here, not deleting the row; the old `DELETE` path is a genuine `404` now
(no route registered), never a `405`.

One body shape, `type` picks which fields matter — always with
`clientRequestId` (I19: a repeated id returns the original event
unchanged and writes nothing new):

```jsonc
// commented — body required
{ "type": "commented", "body": "blocked on the CSV lib", "clientRequestId": "…" }

// status_changed — to is one of open | in_progress | done | closed.
// Moving to "closed" is owner-only (I18): an agent key gets
// 403 invalid_transition, with a hint — not 401, your credential is fine.
{ "type": "status_changed", "to": "in_progress", "clientRequestId": "…" }

// assigned — to is the new assignee's user id, or null to unassign
{ "type": "assigned", "to": "8b1e4f2a-…", "clientRequestId": "…" }
// an unresolvable "to" id is 400 validation_error, not silently stored

// field_changed — field is priority | dueDate | title
{ "type": "field_changed", "field": "priority", "to": "urgent", "clientRequestId": "…" }
```

`type: "created"` (and any other value this endpoint doesn't recognise)
is **rejected** — `400 validation_error` — the same path for both, not a
special case for `"created"` (I16): a `created` event only ever happens
as `POST /todos`'s own side effect, never as something a caller asks for
directly. Returns the created event:

```jsonc
{ "id": "…", "todoId": "3f9c2e10-…", "seq": 3, "actorId": "8b1e4f2a-…",
  "actorHandle": "luna", "type": "status_changed",
  "payload": { "from": "open", "to": "in_progress" }, "body": null,
  "clientRequestId": "…", "createdAt": "2026-08-13T10:00:00Z" }
```

`assigned`'s `payload.from`/`payload.to` are each either `null` or an
`{id, handle}` snapshot resolved once, at write time — a later handle
change never rewrites an old event's history.

---

## `GET /todos/:id/events`

This todo's own timeline, oldest first (the newest-first cross-todo feed
is `bff`-only, not on this surface):

```jsonc
{ "events": [
    { "id": "…", "todoId": "3f9c2e10-…", "seq": 1, "actorId": "8b1e4f2a-…",
      "actorHandle": "luna", "type": "created", "payload": { "title": "…" },
      "body": null, "clientRequestId": "…", "createdAt": "2026-08-13T09:00:00Z" }
] }
```

`type: "created"` appears here as a read value even though `POST
.../events` above rejects it as a write — it's the one event type only
`POST /todos` itself can produce.

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
