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
| `not_found` | 404 | unknown todo/key id, or one that exists but isn't the caller's (I3 — absence, not `403`) | — |
| `actor_field_present` | 400 | request tried to declare an actor (I1) | — |
| `validation_error` | 400 | a malformed or missing field, caught either by the OpenAPI request validator or a handler's own fallback check | `hint` names the field, when the underlying error names exactly one |

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

**404 means "not yours or doesn't exist," never "exists but forbidden."**
There is no `403` on this surface at all. A todo or key you don't own
returns exactly the same `not_found` a nonexistent id would — this is
deliberate (I3): a `403` would leak that the row exists.

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

`PATCH /todos/:id` with neither `title` nor `done` set is a well-formed,
no-op update — both fields are optional, and sending neither is not a
validation failure. It still returns `200` with the todo unchanged.

Deleting an already-deleted todo or an already-revoked key is
`404 not_found`, the same as any other unknown id — deletion and
revocation are both naturally idempotent from the caller's side with no
special-casing needed, but that idempotency shows up as a repeat 404, not
as a repeat success.

`GET /keys` lists an expired-but-unrevoked key without erroring or
filtering it out — expiry is checked only when that key is actually
presented for auth (I9), not when listing. Seeing a key here does not mean
it still authenticates.
