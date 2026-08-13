# Getting started — forking this template

This is the fork checklist: what a new service built from this template
must rename, re-register, and replace before it's its own thing rather
than a copy of `my-template` with a different git remote
(`.chief/milestone-1/_goal/GOAL.md` Done-when 10). Follow the five labeled
steps below in order — each is checked independently, so skipping one
leaves a specific, identifiable trace (a dead login path, a stale module
path, a stale service name, an audience that still points at this
template, or a `internal/todo/` nobody meant to keep). Read
"Prerequisites" first, then "Running what you forked" once the five steps
are done, to actually see it work.

## Prerequisites

`bin/` is gitignored (`.gitignore`) — a fresh `git clone` of this
template, or of a fork made from it, has none of `sqlc`, `goose`, or
`oapi-codegen` on disk yet, even though `go.mod`/`go.sum` pin their exact
versions. Run this before anything else, including before Step 2:

```sh
make tools
```

This installs the three pinned tools into `./bin` (`GOBIN=$(CURDIR)/bin`,
Makefile). Nothing in Step 2–5 below works without it — `make generate`
(the Makefile target, run more than once during Step 5: once after adding
your new module's migration/queries/openapi content, again after deleting
`internal/todo`) shells out to `bin/sqlc` and `bin/oapi-codegen` directly,
and they won't exist otherwise. You need these tools as early as Step 5's
migration-authoring sub-step (`bin/goose create`, see "Writing a new
migration" below) — well before the deletion sub-step, not just at it.
`make tools` has nothing to do with Step 1 below (registering a Hydra
client) — that step doesn't touch Go tooling at all.

## Step 1: Register a Hydra client for owner login

**Run this first, before any of the rename steps below — including if
you're only running this template locally and never plan to deploy it.**
`internal/transport/bff`'s owner-login flow (`GET /login`, `GET
/callback`, `authorization_code` + PKCE per `sso-consumer-contract.md`
§2) needs a Hydra OAuth2 client registered before it can complete a
single login. **There is no lighter local path** — per contract §6, a
client's stable `--id` is derived from `<service>`/`<service>-dev`, and
that id doesn't exist until Step 3 below has actually named your fork.
This template ships **no** Hydra client of its own for exactly that
reason (`.chief/milestone-2/_goal/GOAL.md`'s "Client registration"
Decisions row) — one gets created once, by you, with `scripts/
register.sh`.

```sh
ENV=dev \
  SERVICE_NAME=<your-fork's-service-name> \
  SERVICE_PUBLIC_URL=<this-service's-own-public-url-for-dev> \
  SSO_ISSUER=<your-idp's-issuer-url> \
  HYDRA_ADMIN_URL=<hydra-admin-api-url> \
  HYDRA_PUBLIC_URL=<hydra-public-endpoint-url> \
  ./scripts/register.sh
```

See `docs/DEPLOY-REQUIREMENTS.md`'s "Owner-login Hydra client
registration" section for exactly what each variable means, where its
value comes from, and what the script does (and refuses to do) — it
refuses to overwrite an existing client rather than silently recreating
it, reads the registration back from Hydra rather than trusting the
create call's own output, probes the authorize endpoint with both a
registered and an unregistered redirect URI, and **prints** the resulting
`SSO_ISSUER`/`SSO_CLIENT_ID`/`SSO_CLIENT_SECRET`/`AUTH_AUDIENCE` values
rather than writing them to any file — paste them into your own
deployment's config yourself.

Run it again with `ENV=prod` and that environment's own `SERVICE_PUBLIC_URL`
once you actually deploy — one registration per service per environment,
never shared (dev and prod must never accept each other's tokens).

If you haven't done Step 3 (renaming the service) yet, `SERVICE_NAME` here
is still whatever you're about to rename this fork to — decide the name
now if you haven't, since this step's `CLIENT_ID` bakes it in.

## Step 2: Rename the Go module path

The module path is currently `github.com/mildronize/my-template`
(`go.mod`'s `module` line). Change it to your new repo's real path, then
update every import that references it:

```sh
# from the repo root, after editing go.mod's module line
grep -rl 'github.com/mildronize/my-template' --include='*.go' .
```

The grep above is the actual source of truth for which files are
affected — its output has already drifted from a specific count written
here twice, which is exactly why this section no longer states one. Run
it yourself rather than trusting a number in this paragraph. Most of what
it finds need a **functional** import-path change — an actual `import
"github.com/.../my-template/..."` line the build depends on — and `go
build ./...` afterward confirms nothing was missed (a stale import path
fails the build loudly, it doesn't compile-and-silently-misbehave).
`internal/architecture_test.go` is the one file in that list where it's
harmless either way: the module path also appears once in a **doc
comment** (`modulePath`'s example), not in its logic — editing it
is harmless but has no functional effect either way, since the test
itself resolves the module path dynamically at run time (`go list -m`),
never hardcoded. The same test also doesn't need editing for adding,
renaming, or removing a domain module (Step 5) — the set of domain
modules it checks is likewise resolved dynamically (an `internal/*`
directory listing that excludes the four infrastructure directories
`api`/`db`/`dbquery`/`platform` — `dbquery` is a small shared test-helper
package behind every domain module's I4 check, see its own doc comment),
not hardcoded.

## Step 3: Rename the service

Distinct from Step 2 — the module path is Go's internal name for the
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
- `docs/DEPLOY-REQUIREMENTS.md` — already written generically as of this
  milestone (its audience example is a placeholder URL, not this
  template's real one, and its `issue-key` example already notes that its
  compose service key is renameable per this step) — skim it once for
  anything that still assumes this template's specifics, but there's
  little left to actually change; it's primarily a hand-off document for
  hestia's own deployment specifics, not a rename target the way the rest
  of this list is.
- `docs/GETTING-STARTED.md` itself — this document. It accumulates the
  largest number of dangling `todo`/`todos` references of any file in the
  repo once you've forked (see "Dangling references after a correct fork"
  below for the count and why), since it's the document that explains the
  domain being replaced. Give it the same pass once you're done, or plan
  to replace it with your own fork's documentation.
- **The key path (`~/.my-template/keys/`) and the resolver's env var
  (`MY_TEMPLATE_CREW`)** — both still carry the literal `my-template` name
  today, the same as everything else on this list, and skipping this line
  specifically has a worse failure mode than the others: **two forked
  services that both skip this rename write to the *same* directory and
  silently overwrite each other's key files.** No error, no collision
  warning — the second fork to issue or rotate a key just wins, and the
  first fork's agents either start authenticating as the wrong identity or
  find their key replaced out from under them. This is only reproducible
  once someone forks *twice*, so a single-fork smoke test will never catch
  it — rename both of these explicitly, don't assume the rest of this
  checklist implies it. (`~/.my-template/bin/key`'s fallback chain is
  `argument → MY_TEMPLATE_CREW → TYP_CREW_NAME`; rename the middle one to
  match your fork's own service name.)

## Step 4: Set `AUTH_AUDIENCE` to the new service's own public URL

This is a **deployment-configuration step, not a code edit** —
`AUTH_AUDIENCE` is a deploy-time environment variable
(`internal/platform/config.go`'s `Config` struct), read at process
startup. There is no `.env.example` in this repo and no file with a
literal audience value in it to go find and change; there's nothing to
`grep` for the way Step 2 greps for the module path. What you actually do
on fork is set the variable, in whatever your deployment target's
environment mechanism is (`.env`, a compose file, a systemd unit,
hestia's deployment scripts — see `docs/DEPLOY-REQUIREMENTS.md`), to your
new service's own public URL — never an opaque name, and never copied
from this template's value.

**This is the same value as Step 1's `SERVICE_PUBLIC_URL`.** Whatever URL
you registered the Hydra client's audience against in Step 1 is what
`AUTH_AUDIENCE` must be set to at runtime, per environment — a mismatch
here fails at the token exchange, not at the login screen, so it reads
like an application bug rather than a registration/config mismatch.

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
`docs/DEPLOY-REQUIREMENTS.md`'s "Owner-login Hydra client registration"
section (Step 1's own registration, run with `scripts/register.sh`) and
its "Real Hydra client registration (JWT path)" section (a **separate**,
still-dormant path — stable `--id`, `jwt` access-token-strategy, this
same audience convention, and why hestia should not register a client
for *that* path yet).

**Read the JWT-seam note below before deciding this step even applies
yet** — as of this milestone, the JWT Bearer path this audience feeds
into is wired-but-dormant, not live.

## Step 5: Locate and replace the todo domain

The example domain lives entirely in **`internal/todo/`** — one
directory, per this repo's module-first architecture
(`.chief/_rules/_standard/ARCHITECTURE.md`). **Study it fully before you
delete anything.** `internal/todo` is the only worked example this repo
has of several patterns nothing else documents — deleting it first, then
trying to reconstruct those patterns from scratch, is what a second blind
fork test called *"the single worst instruction in the document"* when an
earlier version of this step led with `rm -rf`.

### Patterns worth preserving

These are the specific things in `internal/todo` and its wiring that
nothing else in this repo documents — read this **before** step 1 below,
so you know what to look for during that read-through instead of
discovering it's missing later:

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
  values collide into the same generated method name — but that's a
  separate concern from the embedded *field names* themselves, which
  **do** collide the instant two domain modules coexist, since this
  repo's convention names every module's adapter type `Server`. One
  module embedded at a time genuinely doesn't need its own composition
  mechanism, as this bullet used to claim outright; two coexisting during
  Step 5's copy-then-delete window do — see "Two modules, briefly, at
  once" (below the numbered list) for the type-alias workaround this
  actually requires.

Do it in this order:

1. **Read `internal/todo` end to end** — `handler.go`, `service.go`,
   `repo.go`, `*_test.go`, its migration (`db/migrations/*_create_todos.sql`),
   its sqlc queries (`db/queries/todos.sql`), its `openapi.yaml` paths, and
   how `cmd/server/main.go` wires it in, with "Patterns worth preserving"
   above in mind — those are the specific things in here that are easy to
   miss and hard to reconstruct if you skip straight to deleting.
2. **Copy `internal/todo` to your new module's directory and rename it** —
   new package name, new file/type/identifier names, new table name —
   but **keep the field names todo-shaped for now** (still `title`/`done`,
   not your real domain's fields yet). Don't touch what the domain
   actually does in this step, only what it's called.
3. **In the same pass, add your new module's own migration, sqlc
   queries, and `openapi.yaml` paths/schemas** — a new table, new query
   file, new paths with new `operationId` values (e.g. `createSnippet`,
   not `createTodo`) — still todo-shaped: the same `title`/`done`-style
   fields your copy from step 2 already has, just declared under your new
   table/type/operation names. This is what actually produces the
   generated types your renamed copy references (`api.Snippet`,
   `db.CreateSnippetParams`, ...) — a rename alone doesn't regenerate
   anything, so skip this step and the copy from step 2 can't compile no
   matter how carefully you renamed it.
4. **Wire the new module into `cmd/server/main.go`** the same way
   `internal/todo` was wired — a domain module's `handler.go` contributes
   methods to `apiServer` (see "Patterns worth preserving" above), its
   `service.go`/`repo.go` stay behind `handler.go`, not imported directly
   elsewhere (`ARCHITECTURE.md` rules 1–2, enforced by
   `internal/architecture_test.go`). At this point both your new module
   and the original `internal/todo` exist side by side and both need to
   work — **read "Two modules, briefly, at once" right below before you
   do this step**, because it breaks two ways every time, not
   hypothetically, and the fix for one of them (a naming collision) has
   to go in as you write this wiring, not after.
5. `make generate`.
6. **This is the real checkpoint** — the previous version of this
   instruction claimed one existed a step earlier than it actually could:
   `go build ./...` and `go test ./...` green here, with the copy's
   fields still todo-shaped, proves the wiring (handler↔service↔repo, the
   sqlc/openapi/migration trio, the `main.go` registration from step 4) is
   correct, independent of what the domain actually does. A renamed copy
   straight out of step 2, with no migration/queries/openapi/regenerate
   behind it yet, cannot reach a green build — it references generated
   types that don't exist until steps 3 and 5 create them. That was the
   actual bug in the old "get the copy compiling and its tests passing as
   a copy" instruction: it described a checkpoint one step earlier than
   the one that's actually reachable.
7. **Only now, change the copy's fields to your real domain shape** — your
   own `service.go`/`repo.go` logic, your migration
   (`db/migrations/*_create_<yours>.sql`) and sqlc queries
   (`db/queries/<yours>.sql`, both still todo-shaped since step 3 — this
   is where they actually become your domain's real columns), and your
   `openapi.yaml` paths/schemas' actual field names (following the same
   conventions — owner-scoped, `me`-only actor reference, no actor field
   in the body/query/header; `_contract/API.md` has the milestone-1
   rationale, though your fork's own contract is what governs going
   forward). Re-run `make generate` after changing the migration/queries.
   If this step breaks the build or a test, you now know the break is in
   your domain logic, not in the wiring — step 6's checkpoint already
   ruled the wiring out.
8. **Only now, `rm -rf internal/todo`.** Also remove its migration
   (`db/migrations/*_create_todos.sql`), its sqlc queries
   (`db/queries/todos.sql`), and **its `openapi.yaml` content — the
   `/todos` paths and the `Todo`/`TodoList`/`CreateTodoRequest`/
   `UpdateTodoRequest` schemas.** This last one is easy to skip because
   nothing on this list says "edit `openapi.yaml`" the way it says `rm`
   for the other two — but skipping it leaves `oapi-codegen` regenerating
   the old `CreateTodo`/`ListTodos`/etc. methods on every `make generate`,
   and the build breaks with `apiServer does not implement
   api.ServerInterface (missing method CreateTodo)` the moment you delete
   `internal/todo` out from under those regenerated methods. Then re-run
   `make generate` — it cleans up `internal/db`'s stale generated output
   for you (it deletes every sqlc-generated file there before
   regenerating, so a query file you removed doesn't leave its old
   `.sql.go` behind breaking the build); no manual `rm` step needed
   **for `internal/db`.** That auto-cleanup is specific to
   `internal/db` — `openapi.yaml` itself is never auto-cleaned by
   `make generate` or anything else; the paragraph above is the manual
   edit that has to happen instead, and it has to happen *before* you run
   `make generate` again, not after. Remove `internal/todo`'s
   registration from `cmd/server/main.go` too (the `apiServer` embed and
   `todo.NewService(todo.NewRepo(conn))` line — see "Patterns worth
   preserving" above for what that struct is doing before you touch it).
   **Also remove the `*todo.Server` (or your alias) embed from your own
   new module's test harness** — see "Two modules, briefly, at once"
   above: this does not happen on its own, and `go build ./...` passing
   does not mean you got it, since the stale reference only shows up in
   `go test ./...`. Before moving on, run both:
   ```sh
   go build ./...   # passing here is NOT sufficient evidence step 8 is done
   go test ./...    # this is the check that actually catches a leftover
                     # cross-module test embed
   ```
9. **Deal with the invariants this deletes.** `internal/todo` carried the
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

### Two modules, briefly, at once

Between step 4 above and step 8's delete, your new module and
`internal/todo` both exist and both have to work — this is required, not
an accident: task-6's own fix to this document made "study, then copy,
then delete last" the rule specifically so you always have a working
original to compare against. But two domain modules coexisting breaks
two things, every time, not as an edge case:

- **`apiServer` (in `cmd/server/main.go`) embeds every domain module's
  `ServerInterface`-contributing type — and this repo's own convention
  names every one of them `Server`** (`todo.Server`, and your new
  module's own type if you followed the same convention). Go names an
  embedded field after its type, so embedding two types both named
  `Server` is a guaranteed `Server redeclared in this block` compile
  error the instant you add the second embed in step 4 — not something
  that depends on your domain's specifics. **The workaround: a type
  alias for the embed**, e.g.
  ```go
  type snippetServer = snippet.Server

  type apiServer struct {
      identity.MeServer
      *identity.KeysServer
      *todo.Server
      *snippetServer
  }
  ```
  Contrast this with "Patterns worth preserving" above, which used to
  claim plain embedding "doesn't need its own composition mechanism" —
  true only as long as exactly one domain module is embedded at a time;
  the moment a second one joins it, that claim stops holding and the
  alias above is the composition mechanism. Once step 8 deletes
  `internal/todo`, only your new module's embed remains and the alias is
  no longer strictly necessary — but there's no harm in leaving it.
- **Each domain module's integration tests exercise the entire
  `api.ServerInterface`**, not just their own module's slice of it (they
  drive a real `gin.Engine` wired with `apiServer`, the same struct
  `main.go` builds). That means adding a second module temporarily breaks
  `internal/todo`'s own existing tests too — they now run against an
  `apiServer` that also has to satisfy your new module's routes — and
  your new module's own tests won't compile without embedding
  `*todo.Server` (or the alias above) alongside your own type, purely so
  the test harness's `apiServer` satisfies the full interface. This is a
  temporary cross-module test dependency, in a repo whose architecture
  rule is otherwise that domain modules stay independent
  (`ARCHITECTURE.md`) — **it does not resolve itself.** Deleting
  `internal/todo` in step 8 removes the *package*, not the reference to
  it your own test harness added in step 4 — `go build ./...` will still
  report success (the harness struct's other fields still compile fine
  in isolation), but `go test ./...` fails with `no required module
  provides package .../internal/todo`, because your own test file still
  imports and embeds it. **`go build` passing is not evidence this step
  worked** — see the checklist at the end of step 8 for the check that
  actually catches it. You have to edit your own module's test harness
  to drop the `*todo.Server` (or alias) embed as part of step 8, the same
  way you'd remove any other now-dead import.

Both of these are expected and temporary — they're what step 6's
checkpoint (`go build ./...`/`go test ./...` green with both modules
present) is actually proving you got right, not a sign something's wrong
with your fork.

## Running what you forked

Once Steps 1–5 are done (or even before, against the unforked template,
to see it work at all — Steps 2–5 are skippable purely to try the
unforked template as-is). **Step 1 is different: it's not skippable just
because the rest of this walkthrough happens to work without it.** The
numbered checklist below only exercises the API-key path (steps 1–5
below never touch `bff`), so it'll pass with Step 1 undone — but the
moment you or anyone else opens `GET /login`, an unregistered client
means a dead login path, on the unforked template or any fork, local
deployment included. If owner login is part of what you're validating,
do Step 1 first regardless of whether this walkthrough alone would have
caught its absence:

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
   # you renamed it in Step 3):
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

I3 and I4 are referenced throughout this document — including the two
paragraphs right below and Step 5's invariant-deletion sub-step — without
ever being explained here; their actual text lives only in
`.chief/milestone-1/_contract/INVARIANTS.md`, which this document assumes
you have open. In short, so the choices below can be made informed even if
you don't:

- **I3 — ownership scoping is absence, not permission.** A row that exists
  but belongs to a different owner (a todo, an API key) reads back as
  `not_found`, identical to a row that never existed — never `forbidden`,
  which would confirm the row's existence to someone who shouldn't be able
  to tell. Applies per domain module, to whatever that module's own
  owner-scoped resource is.
- **I4 — one seam reads identity.** Only the actor-resolution middleware
  ever queries `users`/`api_keys` to determine who's calling; every other
  repo (todos, your new domain) only ever queries its own table(s), never
  identity's. In practice this is checked as "one repo, one table (or
  table-set)" per domain module — `TestI4_TodoRepoOnlyQueriesTodosTable`,
  its `internal/identity` equivalent — statically, against the sqlc query
  source.

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
  `dbquery`/`platform` — the same enumeration `internal/architecture_test.go`
  uses) must have **its own dedicated** `TestI<N>_...` test. A test that
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

**Say what that actually means, plainly, because "just a name check" reads
like a mild caveat and understates it:** a fork can gut every
`TestI3_`/`TestI4_` body down to `{}` and simultaneously make `get`/
`update`/`delete` owner-blind (drop the `WHERE owner_id = ?` clause,
return whatever row matches the id regardless of who's asking), and the
full suite — Done-when 12 included — stays green throughout. The result is
one authenticated user reading, changing, or deleting another user's rows
over HTTP with a `200`, with every automated check saying everything is
fine. This is exactly what a test agent on this milestone did once,
deliberately, to prove the point. A name-check cannot close this gap by
construction — an author willing to empty out a test body to make a check
pass is choosing to defeat their own safety net, and no naming convention
detects that choice — so don't read "the check enforces the name, not the
body" as a minor asterisk. It's the difference between "a suite that's
green" and "an API that's actually safe," and only a real test body, read
by a human, closes it.

**Removal works the same way, in reverse, and it's the direction Step 5
above actually hits.** Your new domain module needs its own
`TestI3_...`/`TestI4_...` if it copied `internal/todo`'s pattern
(Step 5 above has you copy `internal/todo` before deleting it, precisely
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
  into your new module as part of Step 5 (above) and just need to rename
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

## Two codegen gotchas only the compiler tells you about

Both of these are easy to lose an hour to on a fork, because nothing
currently states them and the failure mode is a compile error pointing at
generated code, not at the source you actually wrote:

- **`openapi.yaml`'s `operationId` values name the generated
  `ServerInterface` methods.** `oapi-codegen` turns each operation's
  `operationId` (e.g. `listTodos`) directly into a Go method name
  (`ListTodos`) on `internal/api.ServerInterface` — your `handler.go` has
  to implement that exact method name to satisfy the interface. Renaming
  or adding a path without giving it a sensible `operationId` gets you an
  auto-generated one you didn't choose.
- **sqlc capitalizes `url` as `Url`, not `URL`.** sqlc's default Go-name
  casing doesn't treat `url` as an initialism the way `ID` is — a column
  named `url` generates a struct field `Url`. If your fork's schema adds
  a URL column and you write code assuming `.URL` (Go's own convention,
  e.g. `net/url.URL`), it won't compile. Check `internal/db`'s generated
  struct after adding such a column rather than assuming.

## Writing a new migration

Not spelled out elsewhere, though every existing migration under
`db/migrations/` follows it:

- Filename: `<14-digit-timestamp>_<name>.sql` (e.g.
  `20260812190000_create_todos.sql`) —
  `bin/goose sqlite3 ./data/app.db -dir db/migrations create <name> sql`
  generates one with the timestamp and both markers below already in
  place, run from the repo root once `make tools` has installed `goose`
  (the `sqlite3 ./data/app.db` part is just goose's required driver/DB
  arguments for this subcommand — `create` doesn't actually touch that
  database file).
- Two markers inside the file: `-- +goose Up` above the forward migration,
  `-- +goose Down` above its exact reverse (`db/migrations/*_create_todos.sql`
  is the reference example — `CREATE TABLE` / `DROP TABLE`).
- **A new table needs `owner_id TEXT NOT NULL REFERENCES users (id)`** for
  the ownership model (I3, I4) to apply to it — every existing table
  follows this, but nothing states it as a hard requirement rather than
  something you'd infer from reading `todos`' schema. Skipping it means
  your table has no ownership scoping to test in the first place, and I3
  won't apply to it at all.

## Dangling references after a correct fork

Even a fork that gets every step above exactly right leaves references to
`todo`/`todos` behind in places `go build`/`go vet` can't see: doc
comments (`internal/identity/doc.go`, `internal/platform/config.go`,
`internal/api/validator.go`, `internal/architecture_test.go`, and several
more across `internal/identity/*_test.go` all mention todos in prose, not
code) and test/example fixtures (`{"title": "a todo"}`-shaped literals).
**A real number survives a fully correct fork, and it's deliberately not
written here** — an earlier version of this paragraph put a specific
count in prose twice, and both times the actual number had drifted by the
next fix round without anyone noticing until the next blind test. Run the
grep at the end of this section yourself; that output is the only
trustworthy count, excluding everything a correct Step 5 already deletes
or edits (`internal/todo/` itself, its migration and query file, the two
generated outputs `internal/db`/`internal/api/openapi.gen.go`,
`openapi.yaml`'s own `/todos` content per Step 5's now-corrected delete
list, and `cmd/server/main.go`'s registration) and this repo's unrelated
`.chief`-planning-framework use of the word "todo" in `_todo.md`,
`AGENTS.md`, `.agents/`, which has nothing to do with the domain. **This
document's own text accounts for a further, separately-countable set** of
self-references (it's the doc that explains the todo domain, so it
necessarily says "todo" a lot) — not something a fork edits away by
rewriting this file, but worth knowing about if you're grepping your own
fork and wondering why the count looks higher than expected before you've
excluded this file.

**These are not all harmless**, contrary to an earlier version of this
paragraph's claim that "none of these break anything." Most are inert
prose — but not all: P0-4's kind of issue (a hardcoded table/module name
baked into a test assertion's *arguments*, not its prose) is exactly a
dangling reference that's also live code, and grepping for `todo` is one
of the ways you'd actually catch a hardcoded `"todos"` string left behind
in a check that's supposed to be dynamic. Treat a hit inside a `_test.go`
file's assertion arguments (not its comments) as worth a closer look, not
an automatic skip.

This document's own opening line claims "skipping a step leaves a
specific, identifiable trace"; a leftover `todo` in a comment is a trace
regardless of whether any step was actually skipped, so it doesn't
uniquely identify a missed step the way a stale import path does. After
Steps 2–5, run one more pass — **excluding `.chief/`**, which is kept
deliberately as historical record (see "`.chief/`" below) and would
otherwise dominate the results with hits that aren't about your fork at
all, and **including `docs/GETTING-STARTED.md` itself** in what you check
(Step 3 already lists `docs/DEPLOY-REQUIREMENTS.md` as a rename-pass
target for the same reason — this file collects the most references of
any single file once you've forked, precisely because it's the file that
explains the domain being replaced):

```sh
grep -rn 'todo' --include='*.go' --include='*.md' --include='*.sql' --include='*.yaml' . --exclude-dir=.chief
```

and update or remove whatever's left that's about the old domain, not
your new one.

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
