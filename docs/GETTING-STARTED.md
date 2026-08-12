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
(Step 4's `rm -rf internal/todo` sub-step) shells out to `bin/sqlc` and
`bin/oapi-codegen` directly, and they won't exist otherwise.

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
`internal/todo/`. Nine of those ten need a **functional** import-path
change — an actual `import "github.com/.../my-template/..."` line the
build depends on — and `go build ./...` afterward confirms nothing was
missed (a stale import path fails the build loudly, it doesn't
compile-and-silently-misbehave). The tenth, `internal/architecture_test.go`,
is only in the grep's output because the module path also appears once in
a **doc comment** (`modulePath`'s example), not in its logic — editing it
is harmless but has no functional effect either way, since the test
itself resolves the module path dynamically at run time (`go list -m`),
never hardcoded. The same test also doesn't need editing for adding,
renaming, or removing a domain module (Step 4) — the set of domain
modules it checks is likewise resolved dynamically (an `internal/*`
directory listing that excludes the three infrastructure directories
`api`/`db`/`platform`), not hardcoded.

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
(`.chief/_rules/_standard/ARCHITECTURE.md`). **Study it fully before you
delete anything.** `internal/todo` is the only worked example this repo
has of several patterns nothing else documents — deleting it first, then
trying to reconstruct those patterns from scratch, is what a second blind
fork test called *"the single worst instruction in the document"* when an
earlier version of this step led with `rm -rf`. Do it in this order
instead:

1. **Read `internal/todo` end to end** — `handler.go`, `service.go`,
   `repo.go`, `*_test.go`, its migration (`db/migrations/*_create_todos.sql`),
   its sqlc queries (`db/queries/todos.sql`), its `openapi.yaml` paths, and
   how `cmd/server/main.go` wires it in. See "Patterns worth preserving"
   below for the specific things in here that are easy to miss and hard to
   reconstruct if you skip straight to deleting.
2. **Copy `internal/todo` to your new module's directory and rename it** —
   new package name, new file/type/identifier names, new table name.
   Get the copy compiling and its tests passing *as a copy* before you
   touch its actual logic; this proves you've correctly reproduced the
   wiring (handler↔service↔repo, the sqlc/openapi/migration trio, the
   `main.go` registration) before you start changing what the domain
   actually does. Then adapt the copy: your own migration, your own sqlc
   queries, your own `openapi.yaml` paths (following the same conventions
   — owner-scoped, `me`-only actor reference, no actor field in the
   body/query/header; `_contract/API.md` has the milestone-1 rationale,
   though your fork's own contract is what governs going forward), and
   your own domain logic in `service.go`/`repo.go`.
3. **Wire the new module into `cmd/server/main.go`** the same way
   `internal/todo` was wired — a domain module's `handler.go` contributes
   methods to `apiServer` (see "Patterns worth preserving" below),
   its `service.go`/`repo.go` stay behind `handler.go`, not imported
   directly elsewhere (`ARCHITECTURE.md` rules 1–2, enforced by
   `internal/architecture_test.go`). At this point both the copy and the
   original `internal/todo` exist side by side and both work — confirm
   `go build ./...` and `go test ./...` are still green with both present
   before moving on, so a mistake in step 2 or 3 doesn't get masked by
   also deleting the thing you could have compared against.
4. **Only now, `rm -rf internal/todo`.** Also remove its migration
   (`db/migrations/*_create_todos.sql`) and its sqlc queries
   (`db/queries/todos.sql`), then re-run `make generate` — it cleans up
   `internal/db`'s stale generated output for you (it deletes every
   sqlc-generated file there before regenerating, so a query file you
   removed doesn't leave its old `.sql.go` behind breaking the build); no
   manual `rm` step needed. Remove its registration from
   `cmd/server/main.go` too (the `apiServer` embed and
   `todo.NewService(todo.NewRepo(conn))` line — see "Patterns worth
   preserving" below for what that struct is doing before you touch it).
5. **Deal with the invariants this deletes.** `internal/todo` carried the
   `TestI3_...`/`TestI4_...` tests for `_contract/INVARIANTS.md`'s I3 and
   I4 (per-domain-module scope — see "Invariants: two things, not one"
   below); deleting it deletes those tests along with everything else.
   If you copied them into your new module in step 2 and renamed them,
   you're already covered — otherwise `internal/invariants_test.go`'s
   Done-when-12 check fails the moment you run it, naming your new module
   specifically. Resolve it one of the ways "Invariants: two things, not
   one" describes.

`internal/identity/` and `internal/platform/` are **not** part of this
step — keep both as-is on fork (user/API-key identity and
config/logging/db/server wiring aren't template placeholders).

### Patterns worth preserving

These are the specific things in `internal/todo` and its wiring that
nothing else in this repo documents — read this before step 1's read-through
above, so you know what to look for:

- **How `handler.go` implements the generated `ServerInterface`.**
  `oapi-codegen` generates `internal/api.ServerInterface` from
  `openapi.yaml`'s `operationId` values (one generated method per
  operation); `internal/todo/handler.go`'s `*Server` type implements the
  subset of that interface covering `/todos`. Your new module's
  `handler.go` does the same for your own paths.
- **The `Repository` interface + fake-repo test pattern.** `service.go`
  depends on a small `Repository` interface, not `*Repo` directly, so
  `service_test.go` can substitute a fake/in-memory implementation instead
  of a real SQLite connection. Copy this shape rather than testing the
  service only through a real database.
- **The integration test harness, specifically
  `identity.NewService(repo, repo, nil, nil)`.** `identity.NewService`'s
  signature is `NewService(users UserRepo, apiKeys APIKeyRepo, jwtVerifier
  JWTVerifier, logger *slog.Logger)`. In `internal/todo/handler_test.go`
  (and anywhere else a test needs a working identity service to
  authenticate requests against), the *same* repo is passed as both
  `users` and `apiKeys` — `internal/identity`'s repo implements both
  interfaces — and the last two arguments are `nil`: a `nil` `jwtVerifier`
  just means the JWT branch never matches (it's a wired-but-dormant seam
  in production too, see below), and a `nil` `*slog.Logger` is fine
  because `identity.Service` only logs on paths a unit test doesn't
  normally exercise. Nothing else in this repo states this plainly — copy
  this exact call shape for your new module's own integration tests.
- **`RejectActorFields`/`RequireActor` middleware wiring.** `cmd/server/main.go`'s
  `wireIdentity` mounts `identity.RejectActorFields()` then
  `identity.RequireActor(svc)` on the `/api/v1` group before any domain
  module's routes are registered — I1's request-shape check runs before
  I2/I5's actor-resolution check, and every domain module's handlers rely
  on both having already run. Your new module doesn't add this wiring
  itself; it just has to keep living inside the `apiV1` group it's already
  mounted on.
- **`HashAPIKey`/`CreateAPIKey` test-fixture helpers.** `internal/identity`
  exports `HashAPIKey` and its repo's `CreateAPIKey`, and `internal/todo`'s
  own tests use both to build a real, authenticatable API key for
  integration tests rather than hand-rolling a fake credential. Reuse
  these rather than reinventing key fixtures per module.
- **The `apiServer` embedding trick in `cmd/server/main.go`.** `wireAPI`
  builds one `apiServer` struct that embeds every domain module's
  `ServerInterface`-contributing type — `identity.MeServer`,
  `*identity.KeysServer`, `*todo.Server` today — and passes it to
  `api.RegisterHandlers` as the single value satisfying the whole
  generated interface. This works with plain Go embedding, no hand-written
  delegation methods, *because* no two domain modules' `operationId`
  values collide into the same generated method name. Your new module
  adds one more embedded field to that same struct; it doesn't need its
  own composition mechanism.

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
3. **Check the server actually started, before checking `/healthz`.**
   Look for its own startup log line first — `internal/platform/server.go`'s
   `RunServer` logs `"server starting"` with the address it's about to
   listen on (`:8080`, or whatever `PORT` you've set) before it calls
   `ListenAndServe`. Confirming this
   line is what tells you *your* process is the one now listening. Only
   then does `curl http://localhost:8080/healthz` returning `200 OK` mean
   what you want it to mean — a `200` alone doesn't prove your fork
   started at all, it proves *something* is answering on that port, which
   can just as easily be a stale process left over from a previous run
   that never exited (a real failure mode: starting a second instance
   against an already-bound port fails with `bind: address already in
   use`, logged as an error, not a silent hang — but only if you're
   looking at the log rather than only the health check). `healthz` is
   still the right liveness check `docker-compose.yml` itself points at
   (the runtime image ships no shell or HTTP client to run a Docker
   `HEALTHCHECK` with) — just don't treat it as your only signal.
4. Issue yourself an agent API key — there is no `POST` endpoint for
   this, it's CLI-only by design (`_contract/API.md`,
   `docs/DEPLOY-REQUIREMENTS.md`'s "Seeding the first agent API key"):

   ```sh
   go run ./cmd/issue-key <handle>
   # or, against a running docker compose service (docker-compose.yml's
   # service key is "app" in this template — use your fork's own key if
   # you renamed it in Step 2):
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
(Step 4 above has you copy `internal/todo` before deleting it, precisely
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
