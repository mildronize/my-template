# API contract — milestone-4: shared todos, activity log, corrected key visibility

Extends `.chief/_rules/_contract/API.md` (public API shape/error envelope
conventions, unchanged) and `milestone-3/_contract/API.md` (the two-spec
split, `GET /login`/`GET /callback`, both unchanged). Supersedes both on:
todo shape and endpoints (new fields, `DELETE` removed, event endpoints
added), and `GET`/`DELETE /api/bff/keys` (semantics replaced — see
`INVARIANTS.md` I21).

## Todo shape — both surfaces

```jsonc
{
  "id": "…",
  "title": "…",
  "status": "open",           // open | in_progress | done | closed
  "assigneeId": null,         // nullable, references a user (any role)
  "priority": null,           // nullable: low | medium | high | urgent
  "dueDate": null,            // nullable, ISO 8601
  "createdBy": "…",           // milestone-4: replaces ownerId, attribution only
  "createdAt": "…",
  "updatedAt": "…"
}
```

`done` is gone. A client that sent it is a client written against
milestone-1/2/3's shape and needs updating — not silently accepted and
ignored, per I1's own "presence of an actor field is a 400" precedent
applied here to a removed field: sending `done` in a write body is a
`validation_error` (`hint: "done"`), not a silently-dropped key.

## `publicapi` (agent-facing, Bearer) — todo endpoints

`GET /api/v1/todos`, `POST /api/v1/todos`, `GET /api/v1/todos/:id`,
`PATCH /api/v1/todos/:id` — same shapes as milestone-1/2, extended with
the new fields above. **No owner-scoping filter on `GET /todos`
anymore** — returns every todo, not just the caller's own (I3 no longer
applies to this domain). `POST` accepts `title` (required) and optionally
`assigneeId`/`priority`/`dueDate`. **Not `status`** — every created todo
starts `open` regardless of what a caller asks for, deliberately (Clara's
decision, not a residual gap): มายด์'s own walkthrough only requires
*changing* a todo's status after it exists, never setting one at
creation, so `open` as the universal starting point already satisfies
the brief. The asymmetry with `assigneeId`/`priority`/`dueDate` being
settable at create is intentional, not an oversight — on มายด์'s
acceptance list as something he's told, not a gate. The created event
this produces is `type: "created"`, never client-specifiable as a
distinct write (I16).

**`DELETE /api/v1/todos/:id` — removed.** Finishing a todo means
`PATCH`ing `status` to `closed` (owner-only, I18) or `done` (any actor).
A request to the old path is a genuine `404` — the route doesn't exist —
not a `405` (there's nothing at that path to name a wrong method for).

### `POST /api/v1/todos/:id/events` — new

One body shape per `type`, mirrors my-task's own event-append endpoint
exactly (`_report/tpl2-survey.md`'s Part 1 citation of
`~/gits/my-task/src/app/api/v1/tasks/[id]/events/route.ts`):

```jsonc
// commented
{ "type": "commented", "clientRequestId": "…", "body": "…" }
// status_changed — "closed" rejected here for a Bearer-authenticated caller (I18)
{ "type": "status_changed", "clientRequestId": "…", "to": "in_progress" }
// assigned
{ "type": "assigned", "clientRequestId": "…", "to": "some-handle" }  // or null to unassign
// field_changed
{ "type": "field_changed", "clientRequestId": "…", "field": "priority", "to": "high" }
```

`type: "created"` in this body → `validation_error` (I16). Idempotent on
`clientRequestId` (I19). Response: the created event, same shape as
`GET /todos/:id/events` below.

### `GET /api/v1/todos/:id/events` — new

The per-todo timeline, newest-first-or-oldest-first (plan's to specify;
my-task's own per-task read is chronological, oldest-first, unlike its
cross-task feed which is newest-first — mirror that asymmetry unless the
plan finds a reason not to). No cross-todo feed on this surface — that's
`bff`-only, same asymmetry as milestone-3's key-listing was before this
milestone widened it, and the same reasoning my-task itself uses (its own
cross-task feed is tRPC/owner-only too).

## `bff` (owner-facing, session) — todo endpoints

`GET /api/bff/todos`, `POST /api/bff/todos`, `GET /api/bff/todos/:id`,
`PATCH /api/bff/todos/:id`, `POST /api/bff/todos/:id/events`,
`GET /api/bff/todos/:id/events` — same shapes as the public equivalents
above. **`status: closed` succeeds here** (I18 — this is the owner's
surface). **`DELETE /api/bff/todos/:id` — removed**, same reasoning as
the public surface.

### `GET /api/bff/activity` — new, the cross-todo feed

```jsonc
{
  "items": [
    {
      "id": "…", "seq": 3, "type": "status_changed",
      "actor": { "handle": "…", "role": "owner" },   // role carried so the UI can mark human vs agent (I20's sibling requirement, not itself an invariant — a rendering fact, not a safety one)
      "payload": { "from": "open", "to": "in_progress" },
      "createdAt": "…",
      "todo": { "id": "…", "title": "…" }
    }
  ],
  "nextCursor": { "createdAtMs": 0, "id": "…" } // null when exhausted
}
```

Cursor over `todo_events` across every todo, newest first, joined to
`todos` (for the title/link) and `users` (for the actor's role) — mirrors
my-task's `activity.list` (`_report/tpl2-survey.md`'s citation of
`~/gits/my-task/src/server/api/routers/activity.ts`) exactly in shape,
adapted from tRPC to this surface's plain-JSON convention. Owner-session
only; no agent-facing equivalent (same reasoning `activity.list` itself
has no REST counterpart in my-task).

**Same-millisecond order is by `id`, not by write order — stated
explicitly, not left implicit.** `(created_at, id)` gives the cursor a
*stable total order* (no drops, no duplicates across a paginated walk,
regardless of how a tie resolves), which is the guarantee this endpoint
actually makes. It does not give a *causal* one: two events written
within the same millisecond can come back in either relative order, since
`id` (a UUID) carries no relationship to write sequence. This matches
my-task's own design exactly (its `{createdAtMs, id}` cursor uses cuids,
equally uncorrelated with write order) — same-millisecond order was never
guaranteed by either system, only unlikely to be exercised until this
table's own write frequency made it common. Causal order *within* one
todo is `seq`, monotonic and unaffected by any of this; what's undefined
is only cross-todo interleaving inside a single millisecond, which
neither this endpoint nor the named source has ever promised.

## `bff` — key endpoints, semantics replaced (I21)

`GET /api/bff/keys` now returns every `role='agent'` user's non-revoked
keys, not the session owner's own (which was always empty in practice —
see the survey). `DELETE /api/bff/keys/:id` may revoke any of them, still
session-gated to an authenticated owner. **`GET /api/v1/keys` (agent
Bearer) is unchanged** — still the caller's own keys only. Still no
`POST` on either surface — issuance stays CLI-only, unchanged from
TPL-1/milestone-2.

## Error shape — unchanged

Same envelope, same codes (`unauthorized`, `not_found`,
`actor_field_present`, `validation_error`) as `_rules/_contract/API.md`.
No new codes this milestone — `type: "created"` and a stray `done` field
both map to the existing `validation_error` shape (`hint` names the
field), the same way an unrecognized `type` value already does. A
`status: closed` attempt from an agent (I18) is `unauthorized` (I5 — same
body regardless of which check failed), not a distinct `forbidden` code;
this project has never had a 403.
