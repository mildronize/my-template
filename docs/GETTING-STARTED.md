# Getting started — forking this template

This is the fork checklist: what a new service built from this template
must rename, re-register, and replace before it's its own thing rather
than a copy of `my-template` with a different git remote
(`.chief/milestone-1/_goal/GOAL.md` Done-when 10). Follow the four labeled
steps below in order — each is checked independently, so skipping one
leaves a specific, identifiable trace (a stale module path, a stale
service name, an audience that still points at this template, or a
`internal/todo/` nobody meant to keep).

## Step 1: Rename the Go module path

The module path is currently `github.com/mildronize/my-template`
(`go.mod`'s `module` line). Change it to your new repo's real path, then
update every import that references it:

```sh
# from the repo root, after editing go.mod's module line
grep -rl 'github.com/mildronize/my-template' --include='*.go' .
```

As of this milestone, that's `go.mod` plus ten `.go` files across
`cmd/issue-key`, `cmd/server`, `internal/architecture_test.go`,
`internal/identity/`, `internal/platform/migrate.go`, and
`internal/todo/` — update each import path, then `go build ./...` to
confirm nothing was missed (a stale import path fails the build loudly,
it doesn't compile-and-silently-misbehave). `internal/architecture_test.go`
itself does **not** need editing for a module rename, nor for adding,
renaming, or removing a domain module (Step 4) — both the module path
and the set of domain modules it checks are resolved dynamically at test
time (`go list -m` for the former; an `internal/*` directory listing that
excludes the three infrastructure directories `api`/`db`/`platform` for
the latter), rather than either being hardcoded.

## Step 2: Rename the service

Distinct from Step 1 — the module path is Go's internal name for the
code, the service name is what it's called everywhere else:

- `openapi.yaml`'s `info.title` (currently `my-template API`).
- `docker-compose.yml`'s service key (currently `app` — fine to leave
  generic, but rename it if your fork's convention names services after
  the service, e.g. `my-real-service`).
- The Docker image name Compose derives from the project directory name —
  rename the directory (or set `COMPOSE_PROJECT_NAME`) if you want the
  built image called something other than `<new-dir>-app`.
- Any place hestia's deployment scripts or `prod-thw-home` reference this
  service by name (out of scope for this doc to enumerate — see
  `docs/DEPLOY-REQUIREMENTS.md` and coordinate with hestia directly, since
  that's her domain per `GOAL.md`'s Context section).

## Step 3: Re-register `AUTH_AUDIENCE` to the new service's own public URL

`AUTH_AUDIENCE` (`internal/platform/config.go`) must be the **new**
service's own public URL, one per service per environment — never an
opaque name, and never copied from this template's value. This is
`sso-consumer-contract.md` §6's "Audience convention"
(`~/gits/prod-thw-home/docs/sso-consumer-contract.md`), cross-referenced
rather than re-derived here — see `docs/DEPLOY-REQUIREMENTS.md`'s "Real
Hydra client registration" section for the full registration
requirements (stable `--id`, `jwt` access-token-strategy, the audience
convention, and why hestia should not register a client for *this*
template as-is).

**Read the JWT-seam note below before deciding this step even applies
yet** — as of this milestone, the JWT Bearer path this audience feeds
into is wired-but-dormant, not live.

## Step 4: Locate and replace the todo domain

The example domain lives entirely in **`internal/todo/`** — one
directory, per this repo's module-first architecture
(`.chief/_rules/_standard/ARCHITECTURE.md`). To replace it with your real
domain:

1. `rm -rf internal/todo` — this is the whole domain: handler, service,
   repo, tests, all in one place, by design (`ARCHITECTURE.md`'s stated
   reason: forking must be `rm -rf internal/todo`, not "know a domain
   also has pieces living in three sibling folders").
2. Remove its migration (`db/migrations/*_create_todos.sql`) and its sqlc
   queries (`db/queries/todos.sql`), then re-run `make generate`.
3. Remove the `/todos` paths from `openapi.yaml` and add your own
   resource's paths in their place, following the same conventions
   (owner-scoped, `me`-only actor reference, no actor field in the
   body/query/header — `_contract/API.md` if you want the milestone-1
   rationale, though your fork's own contract is the one that governs
   going forward).
4. Remove `internal/todo`'s registration from `cmd/server/main.go`
   (`wireAPI`'s `apiServer` struct currently embeds `*todo.Server`,
   and `todo.NewService(todo.NewRepo(conn))` above it) and wire your new
   domain module the same way `internal/todo` was wired — a domain
   module's `handler.go` contributes methods to `apiServer`, its
   `service.go`/`repo.go` stay behind `handler.go`, not imported directly
   elsewhere (`ARCHITECTURE.md` rules 1–2, enforced by
   `internal/architecture_test.go`).

`internal/identity/` and `internal/platform/` are **not** part of this
step — keep both as-is on fork (user/API-key identity and
config/logging/db/server wiring aren't template placeholders).

## JWT seam: wired, but dormant by design

> This JWT seam is wired and unit-tested, but **dormant by design**, per
> `sso-consumer-contract.md` §3 — Hydra does not issue machine identity to a
> local process, because its Admin API has no authentication: any process on
> the same host can register a client with a `sub` of its choosing and
> receive a correctly-signed token (`aud` doesn't help either — §7.7).
> **Confirmed against the actually-deployed compose config on 2026-08-12 by
> hestia.** Read §3's "when to revisit" before turning this on.

In practice: agent identity in this template is API-key only
(`cmd/issue-key`). The JWT Bearer code path exists, is exercised by unit
tests against a test issuer, and validates everything
`sso-consumer-contract.md` §7 requires (RS256 pinned, `iss`/`aud`/`exp`
checked, JWKS cached and never pinned to one key) — but no request in a
real deployment should be expected to authenticate this way yet. Don't
treat this as this template's live SSO integration; treat it as a
reference implementation to build on once §3's "when to revisit"
condition is met.

## Two things to reconsider if your fork's needs change

`_contract/API.md`'s Conventions section makes two deliberate
simplifications for milestone-1's todo domain. Both were reasonable
*here* — reconsider them if your fork's domain doesn't share the same
shape:

- **No `Idempotency-Key` requirement.** Reconsider this if you add an
  event/activity log: my-task requires an idempotency key because its
  append-only event log turns a duplicate write into a correctness bug
  forever. This template has no such log, so a duplicate `POST` just
  produces a second row — annoying, not a data-integrity problem. That
  stops being true the moment a fork adds an append-only log of its own.
- **No pagination on list endpoints.** Reconsider this if your fork's
  list can grow large: this template's todo list is inherently small and
  owner-scoped, so unpaginated `GET /todos` was a fine simplification for
  it specifically, not a general rule about REST APIs.

## Invariants: two things, not one

A new invariant needs both an `INVARIANTS.md` entry and a `TestI<N>_`
test — the check in `internal/invariants_test.go` enforces the second
half, not the first. Adding a numbered entry to `_contract/INVARIANTS.md`
with no matching `TestI<N>_...` test anywhere in the module fails that
check loudly (it parses `INVARIANTS.md`'s own headings to know what's
required, rather than a hardcoded list); the reverse — a test named
`TestI<N>_...` for an `I<N>` that was never documented — is not something
that check can catch, and reviewing for it is a human/reviewer
responsibility, not a machine-checkable one.
