---
name: my-template-api
description: Call this my-template-based service's /api/v1 REST surface with an agent API key — auth, the invariant rules every request obeys, all eight endpoints, request/response shapes, and the error envelope. Mechanics only, against any instance. There is no separate spec doc; `openapi.yaml` plus this skill and its references are the source of truth.
---

# `<service>` API

`/api/v1/*` is a key-authenticated REST surface for agents, scaffolded from
this repo's `my-template` origin (`_contract/API.md`,
`_rules/_contract/API.md`). It sits beside a second, owner-facing surface
(`bff` — a browser login + one authenticated view) that this skill does not
cover: the owner's operations are **absent** from `/api/v1`, not forbidden
on it, so a key can never reach them by accident.

This skill describes the mechanics against **any** instance of a service
forked from this template. It does not know which one you talk to, and does
not hold a key. Where a crew's key actually comes from day to day — the
resolver, the fallback chain, how to treat this in a working session — is a
consumer-side concern and belongs in a different, fork-specific doc, the
same way `my-task-guide` sits beside `my-task-api` in this fleet.

## Base URL

Every example below expands `$BASE_URL`. Set it to the instance you mean:

```bash
export BASE_URL="http://localhost:8080"          # local dev (docker compose up)
export BASE_URL="https://<your-fork's-domain>"    # a real deployment
```

There is no default here on purpose — a wrong-instance default fails
silently against a real database. This value is instance-agnostic by
design: nothing about `/api/v1`'s shape depends on which fork you're
pointed at, only the data behind it does.

## Auth

```
Authorization: Bearer <key>
```

Keys are minted host-side only:

```bash
go run ./cmd/issue-key <handle>            # issue
go run ./cmd/issue-key -rotate <handle>    # rotate
```

never through the `bff` surface, because issuing one means writing a file
on the host. A key belongs to exactly one user (role `agent`) and carries
that user's full access — no scopes. Keys carry the `tpl_` prefix
(`tpl_a1b2c3d4…`), the first 8 characters of the random portion stored in
the clear alongside the key row so a listed key is identifiable without
exposing it.

Any credential failure returns **401 `unauthorized`**, and the body is the
same regardless of which check failed: missing, malformed, expired,
revoked, or an inactive user's key — **and an owner's credential is refused
here too** (I2: a Bearer credential never resolves to `role='owner'`;
owner authority only exists through the `bff` surface's session). The real
reason is logged server-side only. Do not try to infer it from the
response.

### The indistinguishable-401 trap

`Authorization: Bearer $(~/.my-template/bin/key)` expands at the shell —
that's the whole point of the resolver, so the raw key value never lands
in a transcript or a log. But it also means a **resolver failure is
invisible at the HTTP layer**: if the resolver can't find a key file, or
can't resolve which handle it's running as, it exits `1` and prints its
reason to **stderr** — and the `$(...)` substitution around it still
collapses to an empty string. The request goes out as
`Authorization: Bearer ` with nothing after it, which is byte-identical,
from the server's point of view, to a wrong or revoked key. Anything
invoking the resolver outside an interactive session (cron, systemd, a
detached process) is exposed the same way with no one watching stderr.

**So when you see a 401, look back at what the key command itself
printed** before concluding the key is bad — run the resolver on its own
first (`~/.my-template/bin/key`) and confirm it actually produced a
value, rather than reasoning about the 401 body. There is nothing in that
body to reason about: I5 guarantees a missing key, a malformed one, an
expired one, a revoked one, an inactive user's, and an owner's credential
all produce the exact same response on purpose.

Confirm a key and see who it belongs to:

```bash
curl -sS -H "Authorization: Bearer $KEY" "$BASE_URL/api/v1/me"
# {"handle":"luna","role":"agent","active":true}
```

A `200` here means *a* key was accepted — check the `handle` in the body
before assuming it was accepted as **you**.

## Rules every request obeys

**1. The actor comes from the credential, never the request (I1).** `me`
is not even a convention on this surface — there is no endpoint that takes
another actor's id, so there is nothing to declare. A request that tries
anyway is refused, not ignored: `actor`, `actorId`, or `ownerId` in the
body **or the query string**, or an `X-Actor` header, all return **400
`actor_field_present`**. This applies identically on `bff`'s session-based
surface too — the invariant is about the mechanism (identity only ever
comes from a resolved credential), not about which surface makes the
mistake easy to try.

**2. There is no `Idempotency-Key` requirement.** If you're coming from
`my-task-api`, this is the rule you'd expect and won't find — it's an
absence, not an oversight: this service has no append-only event log
(every write is a direct row create/update/delete), so there's no replay
window for a duplicate `POST` to land in. Retrying a `POST /todos` twice
creates two todos. If a fork adds an event log later, re-add this rule
there; don't assume it already holds.

**3. Ownership scoping is absence, not permission (I3).** A todo or key
that exists but belongs to a different user returns **404 `not_found`** —
never `403`, because a `403` would confirm the row exists at all. Every
read and write below is scoped to the caller's own rows; there is no way
to address another user's todo or key, correctly-guessed id or not.

## Endpoints

| Method | Path | |
| --- | --- | --- |
| `GET` | `/api/v1/me` | who this key is |
| `GET` | `/api/v1/todos` | the caller's own todos, unpaginated |
| `POST` | `/api/v1/todos` | create → **201** |
| `GET` | `/api/v1/todos/:id` | one of the caller's own todos |
| `PATCH` | `/api/v1/todos/:id` | update title and/or done |
| `DELETE` | `/api/v1/todos/:id` | delete → **204** |
| `GET` | `/api/v1/keys` | the caller's own non-revoked keys |
| `DELETE` | `/api/v1/keys/:id` | revoke → **204** |

Full request/response shapes: `references/endpoints.md`. Every error code
and what triggers it: `references/errors.md`.

Nothing on this surface issues or rotates a key, and there is deliberately
no `POST /api/v1/keys` — a rotated or freshly issued key is printed to a
terminal exactly once (I8), which an HTTP response can't do without the
value sitting in access logs, browser history, or a proxy's request log.
Issuance and rotation are both CLI-only, always (`cmd/issue-key`).

## Worked examples

**List your own todos**

```bash
curl -sS -H "Authorization: Bearer $KEY" "$BASE_URL/api/v1/todos"
```

**Create one**

```bash
curl -sS -X POST "$BASE_URL/api/v1/todos" \
  -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"title":"Export CSV endpoint"}'
```

**Read one**

```bash
curl -sS -H "Authorization: Bearer $KEY" "$BASE_URL/api/v1/todos/$ID"
```

**Mark one done**

```bash
curl -sS -X PATCH "$BASE_URL/api/v1/todos/$ID" \
  -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"done":true}'
```

**Delete one**

```bash
curl -sS -X DELETE "$BASE_URL/api/v1/todos/$ID" -H "Authorization: Bearer $KEY"
```

**See your own keys, to know what needs rotating**

```bash
curl -sS -H "Authorization: Bearer $KEY" "$BASE_URL/api/v1/keys"
```

**Revoke a key by id** — not the one you're currently authenticating
with, unless you mean to lock yourself out immediately:

```bash
curl -sS -X DELETE "$BASE_URL/api/v1/keys/$KEY_ID" -H "Authorization: Bearer $KEY"
```

## Handling a key

A key is a bearer credential: whoever holds the string is that user.
Expand it at the shell so its value never lands in a transcript, a log, or
a comment — `-H "Authorization: Bearer $(...)"` rather than pasting it.
Never print a key file.

If a key turns up somewhere it should not be, rotate it:

```bash
go run ./cmd/issue-key -rotate <handle>
```

`rotate` issues the new key **before** disabling the old one(s) (I13), so
there is never a window where the handle has zero valid keys — but there
is also deliberately **no grace period beyond that reorder**. Rotation is
primarily a leak-response operation, so extending the old key's remaining
validity would be the wrong direction: the old key stops working
immediately once the new one exists. Anything still holding the old value
in a shell variable must **re-run the resolver**, not reuse what it
already expanded — it will not pick up the new key on its own. `issue` and
`rotate` both also write `~/.my-template/keys/<handle>` and ensure
`~/.my-template/bin/key` exists (I14), so that re-run is one command, not
a manual copy out of a terminal scrollback.

### `0600` is a rule, not an isolation guarantee

The key file `issue`/`rotate` write is mode `0600`. That is worth having —
a deliberate widening of the mode is visible in a diff — but be honest
about what it does not do: **every crew or process on a shared host runs
as the same uid**, so every key file under `~/.my-template/keys/` is
readable by every other crew or process on that host regardless of the
file's mode bit. `0600` is not a permission boundary between crews on this
machine; it never was one. If you need that guarantee, it has to come from
somewhere other than the file mode — a different host, a different uid per
crew, or a secrets manager, none of which this template provides on its
own.
