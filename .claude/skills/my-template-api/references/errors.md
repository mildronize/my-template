# `/api/v1` errors

**The envelope, codes, and I-numbered rules below are domain-agnostic —
`todo`/`key` mentions are just this template's current example resource,
not a fixed part of the shape.** Update the couple of `todo` mentions
below for your fork's own domain as part of `docs/GETTING-STARTED.md`
Step 3's rename checklist.

Every failure that reaches a handler — mapped or not — comes back as this
envelope (`_rules/_contract/API.md`):

```jsonc
{ "error": {
    "code": "validation_error",
    "message": "title must be 1-200 characters",
    "hint": "title"
} }
```

`code` and `message` are always present. `hint` appears only where the
table below says so — do not require it, and where it's present, treat it
as best-effort: it comes from pattern-matching the underlying validator's
own error text, not a guaranteed structured field.

## Codes

| Code | HTTP | When | Carries |
| --- | --- | --- | --- |
| `unauthorized` | 401 | any credential failure at all (I2, I5, I9) | — |
| `not_found` | 404 | an unknown todo id (any todo is readable/actionable by any caller, I3 no longer scopes todos — this is purely "never existed"); a key id that exists but isn't the caller's own, or never existed (I3 still scopes keys — absence, not `403`) | — |
| `actor_field_present` | 400 | request tried to declare an actor (I1) | — |
| `validation_error` | 400 | a malformed or missing field, caught either by the OpenAPI request validator or a handler's own fallback check; also an `assigned` event's `to` naming a user id that doesn't resolve (`hint: "to"`) | `hint` names the field, when the underlying error names exactly one |
| `invalid_transition` | 403 | an agent key attempting `status_changed` to `closed` (owner-only, I18) — a real permission refusal, not a credential failure | `hint` says what to do instead: ask the owner |

There is no `internal_error` code documented in the contract — an
unmapped failure is a bug in the service, not a designed response; report
it rather than retrying into it.

## Reading them

**401 tells you nothing about why, on purpose (I5).** Missing, malformed,
expired, revoked, inactive-user, and owner-role credentials are all
indistinguishable in the response — the real reason is logged server-side
only. Do not branch on it, and see the main `SKILL.md`'s "The
indistinguishable-401 trap" before assuming the credential itself is what
failed — an empty or unset key produces the exact same response as a
genuinely wrong one.

**404 means different things for a todo and a key.** For a key: one you
don't own returns exactly the same `not_found` a nonexistent id would
(I3, unchanged) — deliberate, since a `403` would leak that the row
exists. For a todo: there is no "not yours" case left to leak — every
todo is every caller's to read and act on — so `not_found` here only
ever means the id never existed.

**Moving a todo to `closed` is this surface's one `403` — read the hint,
don't retry the same request, and don't rotate your key.** An agent key
gets `invalid_transition`, not `unauthorized`: your credential is fine,
you are simply not allowed to make this specific change. The `hint`
tells you what to do instead (ask the owner). This is different from
every other rejection on this surface, which is either "your credential
is wrong" (`401`) or "this row doesn't exist" (`404`) — `invalid_transition`
is the one case where you are correctly who you say you are, looking at
a real row, and the answer is still no.

**400 `actor_field_present` is a loud refusal, not a dropped field.** The
guard checks the `X-Actor` header, and `actor` / `actorId` / `ownerId` in
the body **and the query string**. Identity comes from the credential
only; there is no free-text identity sub-label on this surface the way
`my-task-api`'s `X-Actor-Detail` is (that surface's own escape hatch does
not exist here).

**400 `validation_error` can come from two different places, and they
don't always carry `hint`.** Most of the time it's `openapi.yaml`'s
request validator, running before any handler code — that's the case
`hint` is most likely to be populated for, when the underlying validation
error names exactly one field or parameter. A handler's own defensive
`ShouldBindJSON` fallback (present in case a route is ever wired without
the validator ahead of it) returns the same code with no `hint` at all —
treat `hint`'s absence as "re-check your request body against
`references/endpoints.md`," not as a signal about which path produced the
error.

## What does not error

**Repeating a `clientRequestId` never errors and never writes twice
(I19).** `POST /todos`, `PATCH /todos/:id`, and `POST .../events` are all
idempotent on it: a repeat returns the *original* write's result
unchanged (same `200`/`201`, same body) and creates nothing new. Retrying
a request you're unsure went through is always safe on this surface —
there is no separate `Idempotency-Key` header to remember, the same field
that names the write is what makes it safe to repeat.

Revoking an already-revoked key is `404 not_found`, the same as any other
unknown id — naturally idempotent from the caller's side with no
special-casing needed, but that idempotency shows up as a repeat 404, not
a repeat success. **There is no `DELETE /todos/:id` to ask the same
question of** — it was removed in milestone-4; see `references/
endpoints.md`.

`GET /keys` lists an expired-but-unrevoked key without erroring or
filtering it out — expiry is checked only when that key is actually
presented for auth (I9), not when listing. Seeing a key here does not mean
it still authenticates.
