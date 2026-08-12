# Getting started — forking this template

This is the fork checklist: what a new service built from this template
must rename, re-register, and replace before it's its own thing rather
than a copy of `my-template` with a different git remote
(`.chief/milestone-1/_goal/GOAL.md` Done-when 10). Follow the four labeled
steps below in order — each is checked independently, so skipping one
leaves a specific, identifiable trace (a stale module path, a stale
service name, an audience that still points at this template, or a
`internal/todo/` nobody meant to keep). Read "Prerequisites" first, then
"Running what you forked" once the four steps are done, to actually see
it work.

## Prerequisites

`bin/` is gitignored (`.gitignore`) — a fresh `git clone` of this
template, or of a fork made from it, has none of `sqlc`, `goose`, or
`oapi-codegen` on disk yet, even though `go.mod`/`go.sum` pin their exact
versions. Run this before anything else, including before Step 1:

```sh
make tools
```

This installs the three pinned tools into `./bin` (`GOBIN=$(CURDIR)/bin`,
Makefile). Nothing in Step 1–4 below works without it — `make generate`
(Step 4.2) shells out to `bin/sqlc` and `bin/oapi-codegen` directly, and
they won't exist otherwise.

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
- `docs/DEPLOY-REQUIREMENTS.md` itself — it's written for *this* template
  (its example audience, its service name in the `issue-key` example
  command) and hands off to hestia for the fork's real deployment, so it
  needs the same rename pass as everything else on this list, not just a
  copy left as-is.

## Step 3: Set `AUTH_AUDIENCE` to the new service's own public URL

This is a **deployment-configuration step, not a code edit** —
`AUTH_AUDIENCE` is a deploy-time environment variable
(`internal/platform/config.go`'s `Config` struct), read at process
startup. There is no `.env.example` in this repo and no file with a
literal audience value in it to go find and change; there's nothing to
`grep` for the way Step 1 greps for the module path. What you actually do
on fork is set the variable, in whatever your deployment target's
environment mechanism is (`.env`, a compose file, a systemd unit,
hestia's deployment scripts — see `docs/DEPLOY-REQUIREMENTS.md`), to your
new service's own public URL — never an opaque name, and never copied
from this template's value.

**The essential rule, one per service per environment (`sso-consumer-
contract.md` §6, "Audience convention"):**

> The audience is the service's own public URL — e.g. `https://your-
> service.example.com`, not an opaque name. Dev and prod audiences must
> differ, so a dev-minted token is never accepted by prod. The argument
> for an opaque name (surviving a domain change) doesn't hold: a
> `redirect_uri` must already be a real URL, so a domain change already
> forces re-registration — an opaque audience wouldn't have saved
> anything.

**`sso-consumer-contract.md` is fleet-internal**
(`~/gits/prod-thw-home/docs/sso-consumer-contract.md`) — it won't exist
on a fork made on a different machine or fleet than `thw-home`'s. The
summary above is everything this step needs; if you're not on this
fleet, adapt this step to your own SSO/IDP's audience convention instead
of chasing that path. If you *are* on this fleet, see
`docs/DEPLOY-REQUIREMENTS.md`'s "Real Hydra client registration" section
for the full registration requirements (stable `--id`, `jwt`
access-token-strategy, this same audience convention, and why hestia
should not register a client for *this* template as-is).

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
   queries (`db/queries/todos.sql`), then re-run `make generate` — it
   cleans up `internal/db`'s stale generated output for you (it deletes
   every sqlc-generated file there before regenerating, so a query file
   you removed doesn't leave its old `.sql.go` behind breaking the
   build); no manual `rm` step needed.
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
5. Deal with the invariants this deletes: `internal/todo` carried the
   `TestI3_...`/`TestI4_...` tests for `_contract/INVARIANTS.md`'s I3
   and I4, and step 1's `rm -rf` deletes them along with everything
   else. `internal/invariants_test.go`'s Done-when-12 check fails the
   moment you run it afterward unless you either (a) write replacement
   `TestI<N>_...` tests for I3/I4 in your new domain, if your domain has
   an equivalent ownership-scoping invariant, or (b) remove the I3/I4
   entries from your fork's own copy of `_contract/INVARIANTS.md` if it
   doesn't. See "Invariants: two things, not one" below — this is that
   section's removal direction, not just the addition one.

`internal/identity/` and `internal/platform/` are **not** part of this
step — keep both as-is on fork (user/API-key identity and
config/logging/db/server wiring aren't template placeholders).

## Running what you forked

Once Steps 1–4 are done (or even before, against the unforked template,
to see it work at all):

1. `make tools`, if you haven't already (Prerequisites, above).
2. Start the service, either:
   - `go run ./cmd/server` — runs directly against the Go toolchain, using
     `.env`/real env vars for config (`docs/DEPLOY-REQUIREMENTS.md`'s
     table); or
   - `docker compose up` — builds the image and runs it against the
     `sqlite-data` named volume (`docker-compose.yml`); this is the same
     smoke pass GOAL.md's Done-when 8 checks.

   Either way, migrations apply automatically on startup
   (`platform.Migrate`, idempotent) — no separate migrate step.
3. Check it's actually up: `curl http://localhost:8080/healthz` (or
   whatever `PORT` you've set) should return `200 OK`. This is the
   liveness check `docker-compose.yml` itself points at, since the
   runtime image ships no shell or HTTP client to run a Docker
   `HEALTHCHECK` with.
4. Issue yourself an agent API key — there is no `POST` endpoint for
   this, it's CLI-only by design (`_contract/API.md`,
   `docs/DEPLOY-REQUIREMENTS.md`'s "Seeding the first agent API key"):

   ```sh
   go run ./cmd/issue-key <handle>
   # or, against a running docker compose service:
   docker compose exec app /app/issue-key <handle>
   ```

   The raw key prints to stdout exactly once — copy it immediately, it's
   never stored anywhere recoverable (I8).
5. Use the key: `curl -H "Authorization: Bearer <key>" http://localhost:8080/api/v1/me`
   should return the handle you issued the key for. That's the same
   condition GOAL.md's own Human Acceptance criterion stops at.

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
with no matching `TestI<N>_...` test fails that check loudly (it parses
every `INVARIANTS.md` under `.chief/` for its own headings to know what's
required, rather than a hardcoded list); the reverse — a test named
`TestI<N>_...` for an `I<N>` that was never documented — is not something
that check can catch, and reviewing for it is a human/reviewer
responsibility, not a machine-checkable one.

**Each heading also carries a `scope:` tag** (` `scope: global` ` or
` `scope: per-domain-module` `, appended after the bold heading line) —
this decides *where* the check looks for the required test, not just
whether one exists:

- `scope: global` (I1, I2, I5–I10): a `TestI<N>_...` test **anywhere in
  the repo** satisfies it, same as before this tag existed.
- `scope: per-domain-module` (I3, I4 — both about ownership/table
  scoping): every domain module (`internal/*`, excluding `api`/`db`/
  `platform` — the same enumeration `internal/architecture_test.go` uses)
  must have **its own dedicated** `TestI<N>_...` test. A test that
  happens to live in a different module no longer counts, even if it
  incidentally covers your module's tables too — this closes a real hole
  a second blind fork test found (task-7): one domain module's test used
  to silently satisfy the requirement for every other module forever, so
  a forked module could ship with zero ownership-scoping tests of its own
  and the suite would stay green.

**Known limitation, both scopes: this check only reads test *names*, not
bodies.** An empty `func TestI3_Foo(t *testing.T) {}` satisfies it just as
well as a real one. That was already true before the `scope:` tag
existed; it matters more now that per-domain-module invariants require a
dedicated test per module — writing an empty one to make the check pass
is a deliberate lie about your fork's coverage, not an accidental miss,
and nothing catches that for you. Write the real test.

**Removal works the same way, in reverse, and it's the direction Step 4
above actually hits.** Your new domain module needs its own
`TestI3_...`/`TestI4_...` if it copied `internal/todo`'s pattern
(Step 4 below has you copy `internal/todo` before deleting it, precisely
so this is a rename, not a from-scratch rewrite). `internal/identity`
keeps its own dedicated `TestI3_...` and `TestI4_...` regardless of what
you do to `internal/todo` or your new module — per-domain-module scope
means each module's test now stands on its own, so deleting
`internal/todo` can no longer silently take `internal/identity`'s only
table-isolation coverage down with it (that risk existed before task-7's
fix; it doesn't anymore). If the Done-when-12 check fails after you
delete `internal/todo`, it's naming your new module specifically, and you
have three equally valid ways to resolve it, whichever reflects your
fork —

- Your new domain has an equivalent invariant (e.g. it's also
  owner-scoped) — write a `TestI3_...`/`TestI4_...` (or renumbered)
  test for it in your new module's own package, so the entry stays
  honest.
- You already copied `internal/todo`'s `TestI3_...`/`TestI4_...` tests
  into your new module as part of Step 4 (above) and just need to rename
  them to match your new module's naming — re-homing an existing test,
  not writing one from nothing.
- Your new domain doesn't need that invariant at all — remove the I3/I4
  entries from your fork's own copy of `_contract/INVARIANTS.md`. This is
  safe to do purely on your new module's account: it does not touch
  `internal/identity`'s own coverage, which is a separate dedicated test
  under per-domain-module scope, not something borrowed from
  `internal/todo`.

Leaving both the entries and the failing check as-is is the one option
that isn't fine — that's exactly the drift Done-when 12 exists to catch.

## `.chief/`

This directory is milestone-1's planning history for *this* template —
`_goal/GOAL.md`, `_contract/` (API/DATA_MODEL/INVARIANTS), `_plan/_todo.md`,
and the task specs that built it. Keep it after forking: it's the record
of *why* the code looks the way it does (why SQLite, why module-first,
why the JWT seam is dormant, why ownership scoping works the way it
does) — genuinely useful context for whoever maintains the fork later,
and `internal/invariants_test.go`'s Done-when-12 check reads whatever
`INVARIANTS.md` file(s) it finds under here, so it isn't even inert
scaffolding. It's safe to delete once you're confident you won't want
that history — nothing at runtime depends on `.chief/` existing, and
deleting it doesn't disable any test (`requiredInvariantNumbers` fails
loudly, not silently, if it finds no `INVARIANTS.md` at all — see
"Invariants" above). If you delete it, write your fork's own
`_contract/INVARIANTS.md` (or equivalent) somewhere under a `.chief/` of
your own instead, so that check has something to check against.
