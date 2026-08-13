# API contract — milestone-3: `bff` becomes a JSON surface

Extends `.chief/_rules/_contract/API.md` (public API, unchanged) and
supersedes milestone-2's own `_contract/API.md` on everything about
`bff` **except** `GET /login` and `GET /callback`, which are unchanged.
`GET /` (the Go `html/template` view) is retired — see
`milestone-3/_goal/GOAL.md`'s Decisions table.

## Two specs, not one

| Surface | Package | Spec file | Consumer | Auth |
| --- | --- | --- | --- | --- |
| Public API | `internal/transport/publicapi` | `openapi.yaml` | agents/skills | `Authorization: Bearer <credential>` (I1–I2, I5–I10, I13–I14) |
| BFF (JSON) | `internal/transport/bff` | `bff-openapi.yaml` (new, repo root, alongside `openapi.yaml`) | the SPA, in the owner's browser | session cookie (I1, I3, I5, I11–I12) |

**Why a second file rather than a second path group in the existing
one** (milestone-3 `_goal/GOAL.md`'s Decisions table has Clara's full
reasoning; restated here because it's the thing this contract enforces):
the two surfaces have genuinely different auth models, and a shared spec
file reads as one surface with two prefixes even when the code enforces
otherwise. `oapi-codegen` generates a second, independent
`ServerInterface` from `bff-openapi.yaml`, exactly as it already does for
`openapi.yaml` — same tool, same request-shape validation via
`gin-middleware`, mounted on the `bff` engine instead of `publicapi`'s.
`openapi-typescript` generates the SPA's fetch types from
`bff-openapi.yaml` only — the SPA never sees, and never needs, anything
in `openapi.yaml`.

**`bff-openapi.yaml` declares no `security` scheme**, same reasoning
`openapi.yaml`'s own header comment already gives: this spec's job is
request/response *shape* validation, not auth — the session-cookie check
happens in Go middleware (`bff.RequireSession`), not at the OpenAPI
layer.

## `GET /login`, `GET /callback` — unchanged

Both endpoints and their behavior are exactly as milestone-2's
`_contract/API.md` describes (PKCE, state cookie, session cookie on
success). Not re-described here; that file remains the source for both.
Neither belongs in `bff-openapi.yaml` — they redirect, they don't return
JSON, and they exist outside the SPA's data-fetching lifecycle entirely
(the browser navigates to them directly, not via `fetch`).

## `bff-openapi.yaml` — new this milestone

All endpoints below are session-cookie-gated (`bff.RequireSession`, I1,
I12 — a session resolving to `role='agent'` is rejected identically to a
missing one). Missing, expired, or wrong-role session → **401** on this
JSON surface (a behavior change from milestone-2's redirect-to-`/login`,
since a `fetch` call can't follow a redirect the way a browser navigation
does — the SPA's own `AuthGate`-equivalent hook is what turns a 401 into
a redirect, client-side, not the BFF).

### `GET /api/bff/me`

Session-check endpoint backing the SPA's `AuthGate`-equivalent hook.

```jsonc
{ "handle": "owner", "role": "owner", "active": true }
```

Same shape as the public API's `GET /api/v1/me` deliberately — one
response shape for "who am I," regardless of which surface asked.

### `GET /api/bff/todos`

Returns only the session owner's own todos, `created_at` descending,
unpaginated — same shape and same no-pagination reasoning as
`_rules/_contract/API.md`'s `GET /api/v1/todos`.

### `POST /api/bff/todos`

Body: `{ "title": "…" }`. `owner_id` is always the resolved session
owner — never accepted from the body (I1, same rule, different auth
mechanism). `done` starts `false`. Calls
`internal/domain/todo.Service.CreateTodo` directly — the same service
instance and method the public API's handler calls, per the
shared-service-layer rule (`_rules/_standard/ARCHITECTURE.md`).

### `GET /api/bff/todos/:id`

Owner-scoped (I3): another owner's id, or an id that never existed, both
return `not_found`. **This is the first BFF-layer I3 check** — milestone-2
documented I3's "per-module, not per-layer" test granularity as a known
limitation because only one layer (`publicapi`) existed to check; this
endpoint's test is what makes the limitation no longer hypothetical for
`todo`. See `milestone-3/_goal/GOAL.md` Done-when 3.

### `PATCH /api/bff/todos/:id`

Body: `{ "title"?: "…", "done"?: true }`. Owner-scoped, same 404 rule.

### `DELETE /api/bff/todos/:id`

Owner-scoped, same 404 rule. Deleting an already-deleted id is also
`not_found` (naturally idempotent, same as the public API).

### `GET /api/bff/keys`

Lists the session owner's own non-revoked keys. Same response shape as
`_rules/_contract/API.md`'s `GET /api/v1/keys`. Calls
`internal/identity.Service.ListAPIKeys` directly — same service, same
method, session-resolved owner instead of Bearer-resolved actor.

### `DELETE /api/bff/keys/:id`

Sets `revoked_at`. Owner-scoped, same 404 rule. Calls
`internal/identity.Service.RevokeAPIKey` directly.

**Deliberately absent: `POST /api/bff/keys` and any rotate endpoint.**
Same reasoning as the public API's absence of both (`_rules/_contract/
API.md`'s Conventions) — issuance and rotation both mint a new secret
that can't be returned safely over an HTTP response (it would sit in
browser history, devtools network logs, a proxy's request log), so both
stay CLI-only regardless of which surface is asking. A settings screen
that could issue or rotate would need a UI element inviting exactly the
action this boundary exists to prevent — see
`milestone-3/_goal/GOAL.md` Done-when 5's negative check.

## The I2/I12 boundary — why owner writes exist on `bff` and never on `publicapi`

Stated explicitly here because `milestone-3/_goal/GOAL.md`'s Decisions
table requires it not be left implicit: I2 (`INVARIANTS.md` — a Bearer
credential never resolves to `role='owner'`) and I12 (a BFF session never
resolves to `role='agent'`) are two halves of one design, not two
independent rules that happen to coexist. An owner has no Bearer
credential to present (I2 forecloses it structurally — there's no code
path that issues one to `role='owner'`), so the only way an owner can
ever authenticate is a BFF session; conversely an agent has no session to
present, so the only way an agent can ever authenticate is a Bearer
credential. **The two proof-of-identity mechanisms are disjoint by
construction, not by convention** — which is why owner writes belong on
`bff` and can never be added to `publicapi` "for consistency": doing so
would require either issuing owners a Bearer credential (breaching I2) or
accepting sessions on `publicapi` (a surface with no session concept and
no reason to grow one). A forker who notices "owner can write via BFF,
can't via API" and reads only the endpoint list without this section
might conclude that's an oversight to fix by relaxing I2. It isn't one.

## Error shape — `bff-openapi.yaml` reuses `publicapi`'s envelope

```jsonc
{ "error": {
    "code": "validation_error",
    "message": "title must be 1-200 characters",
    "hint": "title"
} }
```

Same codes as `_rules/_contract/API.md`'s table (`unauthorized`,
`not_found`, `actor_field_present`, `validation_error`), same meanings.
**Deliberate reuse, not two envelopes to keep in sync** — the SPA and any
future BFF consumer get one error shape to parse, and the two surfaces
already share the domain service layer that produces these errors in the
first place; giving them different wire shapes for the same underlying
condition would be a distinction with no source. `actor_field_present`
still applies on `bff` (I1 is mechanism-level, not surface-level, per
milestone-2's `_contract/API.md` Conventions) even though nothing on this
JSON surface's request bodies would obviously invite an actor field —
same reasoning, restated for the new endpoints rather than assumed to
carry over silently.
