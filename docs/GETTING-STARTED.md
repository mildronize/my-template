# Getting started — forking this template

This is the fork checklist: what a new service built from this template
must rename, re-register, and replace before it's its own thing rather
than a copy of `my-template` with a different git remote
(`.chief/milestone-1/_goal/GOAL.md` Done-when 10). Follow the five labeled
steps below in order — each is checked independently, so skipping one
leaves a specific, identifiable trace (a dead login path, a stale module
path, a stale service name, an audience that still points at this
template, or a leftover `internal/domain/todo`/`todo_handler.go` nobody
meant to keep). Read "Prerequisites" first, then "Running what you
forked" once the five steps are done, to actually see it work.

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
`internal/domain/todo`) shells out to `bin/sqlc` and `bin/oapi-codegen` directly,
and they won't exist otherwise. You need these tools as early as Step 5's
migration-authoring sub-step (`bin/goose create`, see "Writing a new
migration" below) — well before the deletion sub-step, not just at it.
`make tools` has nothing to do with Step 1 below (registering a Hydra
client) — that step doesn't touch Go tooling at all.

**A Node.js toolchain is also required, since milestone-3** — a fresh
clone needs Node (22+; tested against Node 22, matching `web/package.json`'s
own dependency versions) before `make build` or `make run`'s underlying
`go run ./cmd/server` will do anything useful, and before the first time
you run either. `internal/embed.FS`-style embedding (`web/embed.go`,
`cmd/server/spa.go`) bakes whatever's on disk under `web/dist` into the Go
binary at compile time — `make build` runs `web/`'s own build
(`cd web && npm ci && npm run build`, the Makefile's `web-build` target)
before `go build` for exactly this reason, so Node has to be on `PATH`
before that target can succeed. A bare `go build ./...`/`go test ./...`
(no `make build`) still compiles without Node — a tracked placeholder
(`web/dist/.gitkeep`) keeps the embed directive satisfiable on a fresh
checkout — but the resulting binary would serve an empty SPA at `GET /`,
which is never what you actually want to ship or test against.

**`make test` runs both suites — `go test ./...` and web/'s own Vitest
suite — from a bare fresh clone, no manual `npm install` first.** The
Makefile's `test` target depends on `web-test`
(`cd web && npm ci && npm test`), so `make test` alone installs `web/`'s
dependencies from its lockfile and runs both suites; a green `make test`
means neither suite silently sat unrun. That is not a claim the JS suite
is exhaustive — it deliberately covers only the two hooks that replaced
tRPC-coupled logic (session-check, todos-CRUD; GOAL.md Done-when 10), not
all of `web/`. Running just `go test ./...` by hand still works and still
skips the JS suite, same as before; `make test` is the command that
covers both.

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

**If an agent is running this script, not a human at a terminal, this
printed output is a real footgun.** Printing rather than writing to a
file is correct for a human — they read the terminal once and it scrolls
away — but an agent's stdout becomes its own conversation context, which
can't be un-read the way a terminal scrolls away. Most fork setups on
this fleet will be agent-run. If that's you: once `SSO_CLIENT_SECRET` has
been copied out to wherever the real deployment config lives, **rotate
this client's secret** rather than leave the value the script printed
live in your own context — see `~/gits/prod-thw-home/docs/
secret-rotation.md`'s "OAuth2 client secrets (Hydra)" section for the
actual procedure (register a second client and cut over, or `PATCH` the
existing client's secret in place).

Run it again with `ENV=prod` and that environment's own `SERVICE_PUBLIC_URL`
once you actually deploy — one registration per service per environment,
never shared (dev and prod must never accept each other's tokens).

**`http://localhost` for local dev works with zero extra setup — the
cookie `Secure` attribute follows `AUTH_AUDIENCE`'s own scheme.**
`internal/transport/bff` sets `Secure` on its login/session cookies only
when `AUTH_AUDIENCE` (Step 4, the same value as this step's
`SERVICE_PUBLIC_URL`) starts with `https://`; a plain `http://localhost`
value gets a non-`Secure` cookie instead. This isn't a manually-flippable
dev/prod flag someone could ship in the wrong position — it's read from
the same URL that already has to be correct for the OAuth redirect to
match at all, so there's no separate setting to get wrong. **Don't read
this as "cookies over plain http are safe in general"** — it's specifically
safe here because local dev is expected to use `http://localhost`
(nothing but your own browser ever sees that traffic), and a real
deployment's `SERVICE_PUBLIC_URL`/`AUTH_AUDIENCE` will always be
`https://`, which always gets a `Secure` cookie automatically.

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
modules it checks is likewise resolved dynamically, by listing
`internal/domain/`'s own subdirectories (`domainModuleNames`,
`internal/architecture_test.go`), not hardcoded. Since milestone-2's
domain/transport split, every domain module lives under `internal/domain/`
and nothing else does, so this needs no exclusion list at all — simpler
than milestone-1's version, which had to enumerate all of `internal/*` and
exclude the four infrastructure directories `api`/`db`/`dbquery`/`platform`
by name. (`dbquery` is a small shared test-helper package behind every
domain module's I4 check, see its own doc comment.)

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
- **`.claude/skills/my-template-api/`** (`SKILL.md` and
  `references/endpoints.md`/`references/errors.md`) — the agent-facing
  contract for calling this fork's own `/api/v1`, not just internal prose.
  Auth, the I-numbered rules, and the error envelope stay as-is (identity,
  not the example domain); the Endpoints table and every worked `curl`
  example are `/todos`-shaped and need the same domain swap as
  `openapi.yaml` itself. An agent (or crew) calling your fork by following
  this skill instead of reading the real `openapi.yaml` gets 404s against
  paths that don't exist if this pass is skipped. **Keep the `-api` suffix
  on whatever you rename this directory to** (e.g. `my-delivery-api`, not
  `my-delivery-skill`) — `internal/skill_doc_test.go`'s own coverage check
  discovers this directory by scanning `.claude/skills/` for exactly one
  `*-api` entry rather than hardcoding a name (found via DLV-1, the first
  real fork), so a rename that drops the suffix fails that check loudly
  (`found []`) rather than silently — loud, but still a rule this document
  has to actually state, not one a fork should have to discover from a red
  test.
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

## Step 3b: Rename the React app (`web/`)

**A separate checklist from the one above, not folded into it** — the
list above is every place this template's *Go* service is named;
milestone-3 added a whole second application (the Vite SPA under `web/`)
with its own places carrying `my-template`/`todo` by name, distinct
enough from the Go-side list that a forker doing only the checklist above
would still ship a mis-named frontend:

- **`web/package.json`'s `name`** (currently `my-template-web`) — the
  same kind of rename as `openapi.yaml`'s `info.title` above, just for
  the frontend's own manifest.
- **The API base URL, if you ever hardcode one.** Nothing in `web/`
  hardcodes a base URL to rename: `vite.config.ts`'s dev-time proxy
  (`server.proxy`, see "Running `web/` against a live Go backend" below)
  and the built SPA's own `fetch` calls (`web/src/lib/{auth-client,todos,
  keys}.ts`, wired against `/api/bff/*` as of milestone-3/task-3) all go
  through same-origin relative paths (`/api/...`), which need no per-fork
  edit. If your fork ever adds an absolute base URL (a separately-hosted
  API, a CDN-served SPA talking to a different origin than it's served
  from), that's the value to rename here — check the codebase for one
  before assuming there's nothing to do.
- **Domain nouns inside the ported UI components.** `web/src/components/
  Header.tsx`'s `NAV_LINKS` still reads "Activity"/"Tasks"/"Projects" (a
  verbatim port of my-task's own nav, per `.chief/milestone-3/_plan/
  _todo.md`'s task-1 spec — no `/tasks` or `/projects` route actually
  exists in this template yet), `web/src/app/TodosPage.tsx`'s heading
  says "Todos", `web/src/app/settings/ApiKeySettings.tsx`
  and other copy reference "Agent API keys" — the same
  "todo"-domain language `docs/GETTING-STARTED.md`'s own "Dangling
  references after a correct fork" section (below) already tells you to
  grep for on the Go side, now covering `*.tsx`/`*.ts` too since
  milestone-3. That section's own command is the one to run; a
  narrower version scoped to just this step, while you're only touching
  `web/`, is:
  ```sh
  grep -rn -i 'todo' --include='*.tsx' --include='*.ts' web/src
  ```
  and swap whatever's left for your fork's own domain nouns once you've
  replaced the example domain in Step 5 below.

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

The example domain's *definition* is split across **two locations**, not
one, per this repo's domain/transport architecture
(`.chief/_rules/_standard/ARCHITECTURE.md`, "Why transport is not inside a
domain module anymore" — this superseded milestone-1's single-directory
`internal/todo/` shape): business logic and data access live in
**`internal/domain/todo/`** (`service.go`, `repo.go`, their tests), and the
HTTP adapter that exposes them lives separately in
**`internal/transport/publicapi/todo_handler.go`** (+ its test), alongside
every other domain's own handler and the identity actor-resolution
middleware. There is no longer one directory that holds "the todo domain"
whole. **A third place — `internal/transport/bff/todo_handler.go` (plus
its own test files), and the SPA screens under `web/src/app/` that call it
— depends on the domain directly without living in either location
above**, so "two locations" describes where the domain is *defined*, not
everywhere it's *used*; see step 1 and step 8 below for what that third
place means at delete time. **Study all three fully before you delete
anything.** The first two are the only worked example this
repo has of several patterns nothing else documents — deleting either
first, then trying to reconstruct those patterns from scratch, is what a
second blind fork test called *"the single worst instruction in the
document"* when an earlier version of this step led with `rm -rf`.

### Patterns worth preserving

These are the specific things in `internal/domain/todo`,
`internal/transport/publicapi/todo_handler.go`, and their wiring that
nothing else in this repo documents — read this **before** step 1 below,
so you know what to look for during that read-through instead of
discovering it's missing later:

- **How `todo_handler.go` implements the generated `ServerInterface`.**
  `oapi-codegen` generates `internal/api.ServerInterface` from
  `openapi.yaml`'s `operationId` values (one generated method per
  operation); `internal/transport/publicapi/todo_handler.go`'s
  `*TodoServer` type implements the subset of that interface covering
  `/todos`. Your new module's `<new>_handler.go`, added to that same
  `internal/transport/publicapi` package, does the same for your own
  paths — note the type is named `TodoServer`, not a generic `Server` the
  way milestone-1's did; keep that `<Domain>Server` naming convention for
  your own type (see the `apiServer` bullet below for why it matters now).
- **The `Repository` interface + fake-repo test pattern.**
  `internal/domain/todo/service.go` depends on a small `Repository`
  interface, not `*Repo` directly, so `service_test.go` can substitute a
  fake/in-memory implementation instead of a real SQLite connection. Copy
  this shape rather than testing the service only through a real
  database.
- **The integration test harness, specifically
  `identity.NewService(identityRepo, identityRepo, nil, nil)`.**
  `identity.NewService`'s signature is `NewService(users UserRepo, apiKeys
  APIKeyRepo, jwtVerifier JWTVerifier, logger *slog.Logger)`. In
  `internal/transport/publicapi/publicapi_testutil_test.go`'s
  `newIntegrationRouter` (shared by every handler test in that package,
  not reconstructed per file), the *same* repo is passed as both `users`
  and `apiKeys` — `internal/identity`'s repo implements both interfaces —
  and the last two arguments are `nil`: a `nil` `jwtVerifier` just means
  the JWT branch never matches (it's a wired-but-dormant seam in
  production too, see below), and a `nil` `*slog.Logger` is fine because
  `identity.Service` only logs on paths a unit test doesn't normally
  exercise. Nothing else in this repo states this plainly — copy this
  exact call shape for your new module's own integration tests (or just
  reuse `newIntegrationRouter` — you likely don't need your own).
- **`RejectActorFields`/`RequireActor` middleware wiring.** These now live
  in `internal/transport/publicapi/middleware.go` (moved out of
  `internal/identity`, which holds no transport code post-restructure —
  ARCHITECTURE.md). `cmd/server/main.go`'s `wireIdentity` mounts
  `publicapi.RejectActorFields()` then `publicapi.RequireActor(svc)` on
  the `/api/v1` group before any domain module's routes are registered —
  I1's request-shape check runs before I2/I5's actor-resolution check, and
  every domain module's handlers rely on both having already run. Your
  new module doesn't add this wiring itself; it just has to keep living
  inside the `apiV1` group it's already mounted on.
- **`HashAPIKey`/`CreateAPIKey` test-fixture helpers.** `internal/identity`
  exports `HashAPIKey` and its repo's `CreateAPIKey`;
  `internal/transport/publicapi/publicapi_testutil_test.go`'s
  `createAgentWithKey` uses both to build a real, authenticatable API key
  for integration tests rather than hand-rolling a fake credential — one
  shared helper for every handler test file in that package
  (`todo_handler_test.go`, `keys_handler_test.go`, `middleware_test.go`),
  not one per domain module. Reuse it rather than reinventing key
  fixtures for your own module's tests.
- **The `apiServer` embedding trick in `cmd/server/main.go`.**
  `wirePublicAPI` builds one `apiServer` struct that embeds every domain
  module's `ServerInterface`-contributing type —
  `publicapi.MeServer`, `*publicapi.KeysServer`, `*publicapi.TodoServer`
  today — and passes it to `api.RegisterHandlers` as the single value
  satisfying the whole generated interface. This works with plain Go
  embedding, no hand-written delegation methods, *because* no two domain
  modules' `operationId` values collide into the same generated method
  name, **and**, unlike milestone-1, because the embedded field names
  don't collide either: milestone-1 named every domain's adapter type the
  same generic `Server`, guaranteeing a `Server redeclared in this block`
  compile error the instant a second one was embedded. This repo's
  handler types are named after their domain instead (`TodoServer`,
  `KeysServer`, `MeServer`), specifically so more than one can be embedded
  at once with no alias or other workaround — keep following that
  `<Domain>Server` convention for your own type and step 4's embed just
  works. See "Two modules, briefly, at once" (below the numbered list) for
  what *does* still need attention while your new module and `todo`
  coexist.

Do it in this order:

1. **Read both locations end to end** —
   `internal/domain/todo/{service.go,repo.go,service_test.go,repo_test.go,todo_testutil_test.go}`
   (business logic + data access) and
   `internal/transport/publicapi/{todo_handler.go,todo_handler_test.go}`
   (the HTTP adapter) — plus the migration
   (`db/migrations/*_create_todos.sql`), the sqlc queries
   (`db/queries/todos.sql`), the `openapi.yaml` `/todos` paths, and how
   `cmd/server/main.go` wires all of it in (`buildHandler`, `wireIdentity`,
   `wirePublicAPI`), with "Patterns worth preserving" above in mind — those
   are the specific things here that are easy to miss and hard to
   reconstruct if you skip straight to deleting. **A third place also
   depends on this domain directly and is easy to miss because it isn't
   under either location above:** `internal/transport/bff/todo_handler.go`
   (the session-authenticated JSON CRUD surface at `/api/bff/todos`)
   imports `internal/domain/todo` directly, and the SPA's own todos
   screens (`web/src/app/{TodosPage,TodosList,TodoRow,NewTodoDialog}.tsx`,
   `web/src/lib/todos.ts`) reference the same domain's shape through
   generated types. This is what the owner actually sees and edits after
   `GET /login` — an earlier version of this document pointed here at a
   Go-`html/template` view (`internal/transport/bff/view_handler.go`),
   retired by milestone-3's SPA; the file changed, the "there's a third
   place, don't miss it" fact didn't. See the note at the end of step 8
   below for what this means at delete time.
2. **Copy both locations to your new module's homes and rename them** —
   `internal/domain/todo/` → `internal/domain/<new>/` (new package name,
   new file/type/identifier names, new table name) **and**
   `internal/transport/publicapi/todo_handler.go` (+ its test) →
   `internal/transport/publicapi/<new>_handler.go` (+ its test), with the
   handler's adapter type renamed to `<New>Server` (same `<Domain>Server`
   convention `TodoServer` already follows — see "Patterns worth
   preserving" above for why that convention matters). Keep the two
   copies consistent with each other — the handler's `Service` field type
   has to match your renamed domain package's `Service` type — but **keep
   the field names todo-shaped for now** (still `title`/`done`, not your
   real domain's fields yet). Don't touch what the domain actually does in
   this step, only what it's called and where it lives.
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
4. **Wire the new module into `cmd/server/main.go`** the same way `todo`
   is wired today: build the service once in `buildHandler`
   (`todoSvc := todo.NewService(todo.NewRepo(db))` is the pattern — add an
   equivalent line for your new module), add `*publicapi.<New>Server` to
   the `apiServer` struct, and add
   `<New>Server: publicapi.New<New>Server(newSvc)` to the composite
   literal `wirePublicAPI` builds (see "Patterns worth preserving" above
   for what that struct is doing). A domain module's `service.go`/`repo.go`
   stay behind its `internal/transport/publicapi` handler, not imported
   directly elsewhere (`ARCHITECTURE.md` rules 1–2, enforced by
   `internal/architecture_test.go`). At this point both your new module
   and the original `todo` domain exist side by side and both need to
   work — **read "Two modules, briefly, at once" right below before you
   do this step** — the failure modes there are different from what an
   earlier version of this document described, so don't assume you
   already know them.

   *Optional, and separate from the `api.ServerInterface` wiring above:*
   if you also want the owner-facing BFF surface
   (`internal/transport/bff/todo_handler.go`'s `/api/bff/todos`, and the
   SPA screens that call it) to show your new domain instead of todos,
   `wireBFF`'s own `todoSvc` argument and `NewTodoServer` need the same
   swap, plus the equivalent rename pass through
   `web/src/app/{TodosPage,TodosList,TodoRow,NewTodoDialog}.tsx` and
   `web/src/lib/todos.ts` (Step 3b above covers the SPA-side domain-noun
   sweep in general). Nothing requires this before step 8 — but step 8
   requires resolving it one way or another, since deleting
   `internal/domain/todo` breaks `internal/transport/bff/todo_handler.go`'s
   import regardless of whether you've adapted it yet.
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
8. **Only now, delete both locations — `rm -rf internal/domain/todo` AND
   remove `internal/transport/publicapi/todo_handler.go` (+ its test).**
   Both deletions, not just one — it's easy to do the domain-directory
   `rm -rf` (it's the dramatic one) and forget the handler file sitting in
   a package alongside other files that aren't going anywhere. Also remove
   its migration (`db/migrations/*_create_todos.sql`), its sqlc queries
   (`db/queries/todos.sql`), and **its `openapi.yaml` content — the
   `/todos` paths and the `Todo`/`TodoList`/`CreateTodoRequest`/
   `UpdateTodoRequest` schemas.** This last one is easy to skip because
   nothing on this list says "edit `openapi.yaml`" the way it says `rm`
   for the other two — but skipping it leaves `oapi-codegen` regenerating
   the old `CreateTodo`/`ListTodos`/etc. methods on every `make generate`,
   and the build breaks with `apiServer does not implement
   api.ServerInterface (missing method CreateTodo)` the moment you delete
   `internal/domain/todo` and `todo_handler.go` out from under those
   regenerated methods. Then re-run `make generate` — it cleans up
   `internal/db`'s stale generated output for you (it deletes every
   sqlc-generated file there before regenerating, so a query file you
   removed doesn't leave its old `.sql.go` behind breaking the build); no
   manual `rm` step needed **for `internal/db`.** That auto-cleanup is
   specific to `internal/db` — `openapi.yaml` itself is never
   auto-cleaned by `make generate` or anything else; the paragraph above
   is the manual edit that has to happen instead, and it has to happen
   *before* you run `make generate` again, not after. Remove `todo`'s
   registration from `cmd/server/main.go` too — `buildHandler`'s
   `todoSvc := todo.NewService(todo.NewRepo(db))` line, the
   `*publicapi.TodoServer` field on `apiServer`, and the
   `TodoServer: publicapi.NewTodoServer(todoSvc)` entry in
   `wirePublicAPI`'s composite literal (see "Patterns worth preserving"
   above for what that struct is doing before you touch it).
   **Also remove every `TodoServer` reference in
   `internal/transport/publicapi/publicapi_testutil_test.go`'s shared
   `compositeServer` — that's two live edits, not one: the `*TodoServer`
   embed in the struct definition, AND the `TodoServer:
   NewTodoServer(todoSvc)` entry in `newIntegrationRouter`'s own
   `compositeServer{...}` literal** (a separate composite literal from
   `wirePublicAPI`'s in `cmd/server/main.go`, addressed above — this one's
   test-only and easy to miss precisely because the production one already
   got fixed). Doing only the embed and stopping there still fails:
   `go vet ./...` reports `undefined: NewTodoServer` for the
   now-orphaned initializer. See "Two modules, briefly, at once" above:
   none of this happens on its own, and `go build ./...` passing does not
   mean you got it, since the stale reference only shows up in
   `go test ./...`/`go vet ./...`. Before moving on, run both:
   ```sh
   go build ./...   # passing here is NOT sufficient evidence step 8 is done
   go test ./...    # this is the check that actually catches a leftover
                     # cross-module test embed
   ```

   **A third place also imports the todo domain directly:
   `internal/transport/bff/todo_handler.go`** (`NewTodoServer(svc
   *todo.Service)`, `toBFFTodo(t todo.Todo) bffapi.Todo`, and every
   `ListTodos`/`CreateTodo`/`GetTodo`/`UpdateTodo`/`DeleteTodo` method
   calling `s.Service.*Todo(...)`) — the session-authenticated
   `/api/bff/todos` surface the owner's SPA calls after `GET /login`, not
   through the shared-service-layer indirection alone but through this
   file naming the domain type directly, same as
   `internal/transport/publicapi/todo_handler.go` already does. **This is
   structurally the same file you already adapted once, in step 2 above —
   `internal/transport/bff/todo_handler.go` is `internal/transport/
   publicapi/todo_handler.go`'s session-authenticated twin, generated from
   a second OpenAPI spec (`bff-openapi.yaml`) into a second package
   (`internal/bffapi`) instead of `internal/api`.** Deleting
   `internal/domain/todo` without touching this file is a guaranteed
   `go build ./...` failure (`no required module provides package
   .../internal/domain/todo`) — loud, not silent.

   **Adapt it to your new domain, the same rename-in-place you already did
   for the `publicapi` handler in step 2** — new `bffapi.<New>`-shaped
   request/response types (from your own `bff-openapi.yaml` edits and
   `make generate`, mirroring step 3's `openapi.yaml` work), the same
   method bodies calling your renamed service's own methods.
   `internal/transport/bff/todo_handler_test.go` carries three things a
   sloppy rename can silently break rather than catch:
   `TestBFFHandler_FullCRUDRoundTrip_RealSessionCookie` (the real-cookie
   round trip this document's own Step 5 checklist and task-5's own final
   verification both rely on), `TestI3_BFFHandlerOwnershipScoping_
   ReturnsNotFoundNotForbidden` (I3 at this specific layer — a dedicated
   check, not inherited from `publicapi`'s own I3 test), and
   `TestBFFHandler_ListTodos_Unauthenticated_Returns401NotRedirect` (I12's
   "answer 401, never redirect" behavior on this JSON surface). Carry all
   three over the same way you carried `internal/domain/todo`'s own tests
   in step 2. `cmd/server/main.go`'s `wireBFF` call passes `todoSvc` as an
   argument to `bff.NewTodoServer` — that call site has to keep matching
   whatever your renamed constructor ends up expecting.

   **The SPA side needs the same rename, and it's easy to treat as
   optional because nothing in `go build`/`go test ./...` will ever ask
   for it.** `web/src/app/{TodosPage,TodosList,TodoRow,
   NewTodoDialog}.tsx` and `web/src/lib/todos.ts` reference "todo"/
   `title`/`done` by name (Step 3b above already flags this file set for
   the general domain-noun sweep). `web/src/app/TodosList.test.tsx`
   ("renders the mocked todo's title, and never a title absent from the
   mock" — Done-when 7's negative-control render check) and
   `web/src/lib/todos.test.tsx` (the create/update/delete hook tests) are
   this rename's own regression guard on that side — carry them over the
   same way, not just the Go tests.

   **`internal/transport/bff`'s own test files hit the same build-vs-test
   trap the `compositeServer` warning above describes for
   `publicapi` — extend that warning to this package too.**
   `todo_handler_test.go` and `bff_testutil_test.go` (`newTestRouter`'s
   `todoSvc *todo.Service` parameter, `seedTodo`) both import
   `internal/domain/todo` directly, same as `todo_handler.go` itself.
   `go build ./...` never compiles `_test.go` files at all, so getting
   `todo_handler.go` itself to compile cleanly proves nothing about either
   of these two — a stale `todo.Service`/`todo.Todo` reference left behind
   in either one only surfaces once you run `go test ./...` (or
   `go vet ./...`), exactly the way `compositeServer`'s stale `*TodoServer`
   embed only surfaced there for `publicapi_testutil_test.go`. Update both
   test files' own domain references (and rename `seedTodo` if you're
   keeping that fixture) in the same pass you adapt `todo_handler.go`, not
   after a green `go build ./...` makes it look finished.
9. **Deal with the invariants this deletes.** `internal/domain/todo/
   repo_test.go` carried the `TestI3_...`/`TestI4_...` tests for
   `_rules/_contract/INVARIANTS.md`'s I3 and I4 (per-domain-module scope —
   see "Invariants: two things, not one" below); deleting it deletes those
   tests along with everything else. If you copied them into your new
   module in step 2 and renamed them, you're already covered — otherwise
   `internal/invariants_test.go`'s Done-when-12 check fails the moment you
   run it, naming your new module specifically. Resolve it one of the ways
   "Invariants: two things, not one" describes.
10. **Update `cmd/smoke/main.go` for your domain — do this even though
    nothing in Steps 1–9 above or `go build`/`go test ./...` will ever ask
    you to.** `cmd/smoke` is the only real-HTTP check in this entire repo
    of I1 (actor-field rejection) and I3 (ownership scoping) — every other
    I1/I3 test in the suite runs in-process, injecting the actor directly
    or signing a session cookie itself rather than going over the wire
    through a real `Authorization: Bearer` header against a genuinely
    separate running server (`cmd/smoke/main.go`'s own package doc explains
    why). `cmd/smoke` has no test file of its own and its domain-specific
    literals are plain strings (`/todos`, `title`, `done`), so a fork that
    skips this step keeps compiling, keeps running, and keeps printing
    `16/16 passed` — against a `/todos` path your fork no longer serves,
    testing nothing. `go build ./...` and `go test ./...` both stay green
    regardless; nothing else in the suite will tell you this happened.
    Everything that needs to change is isolated in one place: the
    `==== EDIT THIS FOR YOUR DOMAIN ====` banner block near the top of
    `cmd/smoke/main.go` (`resourcePath`, the `resource`/
    `resourceListResponse` shapes, and the `newCreateBody`/
    `newCreateBodyWithForbiddenField`/`newUpdateDoneBody` request-body
    builders) — update that block to your new domain's path and fields,
    then re-run `make smoke` against a live instance (`go run ./cmd/server
    &`, then `make smoke`) and confirm it's still 16/16, this time for
    real.

`internal/identity/` and `internal/platform/` are **not** part of this
step — keep both as-is on fork (user/API-key identity and
config/logging/db/server wiring aren't template placeholders).

### Two modules, briefly, at once

Between step 4 above and step 8's delete, your new module and the `todo`
domain both exist and both have to work — this is required, not an
accident: task-6's own fix to this document (milestone-1) made "study,
then copy, then delete last" the rule specifically so you always have a
working original to compare against; milestone-2's domain/transport
restructure didn't change that reasoning, only where the two halves of
"the todo domain" each live. What actually breaks while both coexist looks
different from an earlier version of this section, though — read this
fresh rather than assuming the old failure modes still apply as described:

- **No handler-type naming collision by default — a change from
  milestone-1.** Milestone-1 named every domain's adapter type the same
  generic `Server`, so `apiServer` (`cmd/server/main.go`) embedding two of
  them was a guaranteed `Server redeclared in this block` compile error
  the instant a second domain module joined, and needed a type-alias
  workaround to avoid. That workaround no longer applies: this repo's
  handler types all live together in one `internal/transport/publicapi`
  package now and are named after their domain specifically so more than
  one can coexist — `apiServer` (`cmd/server/main.go`) and
  `compositeServer` (`internal/transport/publicapi/
  publicapi_testutil_test.go`) both already embed `MeServer`,
  `*KeysServer`, and `*TodoServer` side by side with no alias anywhere.
  The only way to reintroduce the old collision is to not follow the
  `<Domain>Server` convention for your new module's type — keep following
  it (see "Patterns worth preserving" above) and step 4's embed just
  compiles.
- **The shared integration-test harness still has to satisfy the whole
  interface, though — that part of milestone-1's problem is unchanged in
  spirit.** `internal/transport/publicapi/publicapi_testutil_test.go`'s
  `compositeServer` and `newIntegrationRouter` are shared by every handler
  test file in that package (`todo_handler_test.go`, `keys_handler_test.go`,
  `middleware_test.go`, and your new `<new>_handler_test.go` once you add
  it) — one definition, not one per domain module. Adding your new
  module's handler to `compositeServer` in step 4 means the existing
  `todo_handler_test.go` tests now run against a router that also has to
  satisfy your new module's routes, and your own new handler's tests won't
  compile without `compositeServer` embedding `*TodoServer` alongside your
  own type, purely so the shared harness satisfies the full generated
  interface. This is a temporary cross-module test dependency, in a repo
  whose architecture rule is otherwise that domain modules stay
  independent (`ARCHITECTURE.md`) — **it does not resolve itself.**
  Deleting `internal/domain/todo` in step 8 removes the *package*, not
  `compositeServer`'s embed of `*TodoServer` — `go build ./...` will
  still report success (the struct's other fields still compile fine in
  isolation), but `go test ./...` fails with `no required module provides
  package .../internal/domain/todo`, because `publicapi_testutil_test.go`
  itself still imports and embeds it. **`go build` passing is not
  evidence this step worked** — see the checklist at the end of step 8
  for the check that actually catches it. Drop the `*TodoServer` embed
  from `compositeServer` as part of step 8, the same way you'd remove any
  other now-dead import.

Both of these are what step 6's checkpoint (`go build ./...`/
`go test ./...` green with both modules present) is actually proving you
got right, not a sign something's wrong with your fork.

## Running what you forked

Once Steps 1–5 are done (or even before, against the unforked template,
to see it work at all — Steps 2–5 are skippable purely to try the
unforked template as-is). **Step 1 is different: it's not skippable just
because the rest of this walkthrough happens to work without it.** The
numbered checklist below (1–5) only exercises the API-key path (it never
touches `bff`), so it'll pass with Step 1 undone — but the moment you or
anyone else opens `GET /login`, an unregistered client means a dead login
path, on the unforked template or any fork, local deployment included.
If owner login — or the owner-facing SPA's todo CRUD, step 6 below — is
part of what you're validating, do Step 1 first regardless of whether
this walkthrough alone would have caught its absence. **If you're
running this exact template (not a fresh fork) against an already-live
Hydra deployment, check whether Step 1 has already been done for you**
— a registered client's `redirect_uris` pins a specific host/port
(`sso-consumer-contract.md` §6), so it's a one-time-per-service-per-
environment setup step, not something every run repeats:

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
   should return the handle you issued the key for. That's the agent/
   API-key path's own stopping point — the one `.chief/milestone-1/
   _goal/GOAL.md` and `.chief/milestone-2/_goal/GOAL.md`'s own Human
   Acceptance criteria stop at. It does **not** touch `bff`, owner login,
   or the SPA at all — step 6 below is the separate, further path that
   does.
6. **Owner login + todo CRUD through the real SPA** — this is
   `.chief/milestone-3/_goal/GOAL.md`'s own Human Acceptance criterion,
   and needs Step 1 done (a registered Hydra client) first, unlike steps
   1–5 above:
   - Open `http://localhost:8080/login` (or your `PORT`) in a real
     browser. This redirects to Hydra and back through `GET /callback`
     once you complete a real login there. No automated check can drive
     *your own* production Hydra's real consent screen for you — but
     `e2e/` does prove this exact code path (redirect, PKCE,
     callback, session issuance) automatically, against a real local
     OIDC issuer it stands up and tears down itself (`make e2e`,
     `e2e/README.md`). That's a claim about the *code*, not about
     whether *your* production Hydra is configured correctly — this step
     is still the one that proves the latter, and still isn't something
     an unattended loop can do for you.
   - A successful login redirects to `/`, the SPA's todos page
     (`web/src/app/TodosPage.tsx`), session-cookie-authenticated against
     `/api/bff/me`.
   - **Create** a todo (the "New todo" button/dialog), confirm it
     **appears in the list**, **edit** it (toggle done, or retitle), then
     **delete** it — all backed by real `/api/bff/todos` JSON endpoints
     (`internal/transport/bff`, session-authenticated, owner-scoped).
     This is what closes the "owner can log in but there's nothing to
     create" gap milestone-2's acceptance attempt exposed; milestone-3
     is what actually ships it.

### Running `web/` against a live Go backend (Vite dev mode)

Everything above runs the SPA **built and embedded** — `web/dist`'s
output baked into the Go binary at compile time (`web/embed.go`,
Makefile's `build` target). That's the right mode for actually using the
service, but the wrong one for iterating on `web/`'s own source: editing
a `.tsx` file doesn't do anything to an already-running embedded binary —
you'd have to re-run `make build` and restart the server for every change,
losing Vite's whole live-reloading point. Milestone-2's own docs flagged
this exact trap for a different reason (a stale generated-code artifact
looking current when it wasn't); the embedded-SPA version of it is "why
isn't my change showing" the moment you edit `web/src/*` while a
built-and-embedded binary is what's actually running.

The second mode — **Vite dev server proxying to the Go backend** — is
for that iteration loop instead:

1. Start the Go backend first, same as step 2 above (`go run ./cmd/server`
   or `docker compose up`) — it still owns `/api/v1`, `/healthz`,
   `/login`, `/callback`, and the BFF's own JSON endpoints (task-2/3).
   Vite's dev server below only ever serves `web/`'s own source; it has
   no backend of its own to talk to.
2. In a second terminal, from `web/`:
   ```sh
   npm install   # first time only, or after web/package.json changes
   npm run dev
   ```
   This starts Vite's dev server (default `http://localhost:5173`) with
   live reloading — edits to `web/src/*` show up immediately, no rebuild,
   no restart.
3. Open the Vite dev server's URL (`http://localhost:5173`), not the Go
   backend's port. `vite.config.ts`'s `server.proxy` forwards `/api`,
   `/login`, and `/callback` from Vite's own server to the Go backend
   (`http://localhost:8080` by default — override with
   `VITE_BFF_PROXY_TARGET` if your Go backend runs somewhere else, e.g. a
   non-default `PORT` or a `docker compose` service reachable at a
   different host). This is what makes `GET /login`'s server-side OAuth
   redirect and any BFF `fetch` call work from the Vite-served page
   without a CORS or cookie-domain problem to configure — the browser
   only ever talks to one origin (Vite's), which quietly forwards the
   handful of paths that need the real backend.

**Which mode you're in changes what a given command actually does** —
worth stating plainly since this is exactly the kind of trap
milestone-2's own docs called out once already for a different pair of
commands:

| You're running... | `GET /` serves... | Edits to `web/src/*` show up... |
| --- | --- | --- |
| `go run ./cmd/server` / `docker compose up` alone | `web/dist`'s embedded build, frozen at the last `make build` | Never, until you re-run `make build` and restart the server |
| `npm run dev` (backend running separately) | Vite's dev server, live | Immediately, no rebuild |

### The owner can create, edit, and delete their own todos

This was a real limitation through milestone-3/task-1 (the SPA's todos
page was a data-free placeholder) — **closed by task-2/task-3, not still
true.** `GET /` serves the Vite SPA (`web/`, embedded via
`web/embed.go`/`cmd/server/spa.go`); once logged in via `GET /login`,
the todos page (`web/src/app/TodosPage.tsx`) lists the owner's own todos
and offers create (a dialog), toggle-done/retitle (edit), and delete —
all backed by real session-authenticated JSON endpoints under
`/api/bff/todos` (`internal/transport/bff`, reusing
`internal/domain/todo.Service` directly, no new domain logic). The public
API's rejection of an owner's Bearer token (I2 — a browser session must
never carry API-key-equivalent write authority) is still true and still
the reason this write path lives on `/api/bff` and never on
`/api/v1` — see `_contract/API.md`'s "I2/I12 boundary" section — but it
no longer means the owner has *no* way to write at all. The BFF itself is
no longer read-only.

If you just want to seed a todo without going through the SPA/login flow
at all (a demo, or a quick DB-level check), reaching into the database
directly still works — `cmd/seed`-style direct DB access, or a manual
`INSERT` against `DATABASE_PATH`'s SQLite file — but it is no longer the
*only* supported path, just a shortcut around it.

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

`.chief/_rules/_contract/API.md`'s Conventions section (promoted from
milestone-1, unchanged since — see "Invariants: two things, not one"
below for what "promoted" means here) makes two deliberate simplifications
for the todo domain. Both were reasonable
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
`.chief/_rules/_contract/INVARIANTS.md`, which this document assumes you
have open. (That file is milestone-2's promotion of milestone-1's own
`_contract/INVARIANTS.md` — the milestone-1 copy is superseded and
historical only, per `_rules/_standard/ARCHITECTURE.md`'s "Contract
promotion convention"; read the promoted copy, not the milestone-1 one, so
you're not looking at a version that's stopped being the live authority.)
In short, so the choices below can be made informed even if you don't:

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
  source, via `internal/dbquery`'s explicit `TableOwnership` map (not
  guessed by scanning). A module may *read* (never write) a table it
  doesn't own, but only through an explicit, named
  `dbquery.ReadOnlyGrants` entry, enforced mechanically — writing to a
  granted table still fails this check regardless of the grant
  (`_rules/_contract/INVARIANTS.md`'s I4 clarification, milestone-4).

A new invariant needs both an `INVARIANTS.md` entry and a `TestI<N>_`
test — the check in `internal/invariants_test.go` enforces the second
half, not the first. Adding a numbered entry to
`.chief/_rules/_contract/INVARIANTS.md` (the promoted, live copy — see
above) with no matching `TestI<N>_...` test fails that check loudly. Once
a promoted `INVARIANTS.md` exists there, `findInvariantsFiles`
(`internal/invariants_test.go`) reads **only** that file, not a union of
every `INVARIANTS.md` under `.chief/` — a milestone's own historical
`_contract/INVARIANTS.md` copy no longer contributes to what's required
once promotion has happened (`_rules/_standard/ARCHITECTURE.md`'s
"Contract promotion convention"; this repo promoted in milestone-2, so
that's the live behavior here). A fork that never promotes a contract at
all falls back to the older behavior — globbing and unioning every
`INVARIANTS.md` under `.chief/` — but that's not this repo's current
state. Either way, the reverse — a test named `TestI<N>_...` for an `I<N>`
that was never documented — is not something that check can catch, and
reviewing for it is a human/reviewer responsibility, not a
machine-checkable one.

**Each heading also carries a `scope:` tag** (` `scope: global` ` or
` `scope: per-domain-module` `, appended after the bold heading line) —
this decides *where* the check looks for the required test, not just
whether one exists:

- `scope: global` (I1, I2, I5–I10): a `TestI<N>_...` test **anywhere in
  the repo** satisfies it, same as before this tag existed.
- `scope: per-domain-module` (I3, I4 — both about ownership/table
  scoping): every domain module — every subdirectory of `internal/domain/`
  (`domainModuleNames()`, `internal/architecture_test.go`; simpler than
  milestone-1's version, which enumerated all of `internal/*` and excluded
  `api`/`db`/`dbquery`/`platform` by name, back when domain modules weren't
  yet split into their own top-level directory) — must have **its own
  dedicated** `TestI<N>_...` test. A test that
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
`TestI3_...`/`TestI4_...` if it copied `internal/domain/todo`'s pattern
(Step 5 above has you copy `internal/domain/todo` before deleting it,
precisely so this is a rename, not a from-scratch rewrite). `internal/
identity` keeps its own dedicated `TestI3_...` and `TestI4_...` regardless
of what you do to the `todo` domain or your new module — per-domain-module
scope means each module's test now stands on its own, so deleting
`internal/domain/todo` can no longer silently take `internal/identity`'s
only table-isolation coverage down with it (that risk existed before
task-7's fix; it doesn't anymore). If the Done-when-12 check fails after
you delete `internal/domain/todo`, it's naming your new module
specifically, and you have three equally valid ways to resolve it,
whichever reflects your fork —

- Your new domain has an equivalent invariant (e.g. it's also
  owner-scoped) — write a `TestI3_...`/`TestI4_...` (or renumbered)
  test for it in your new module's own package, so the entry stays
  honest.
- You already copied `internal/domain/todo`'s `TestI3_...`/`TestI4_...`
  tests into your new module as part of Step 5 (above) and just need to
  rename them to match your new module's naming — re-homing an existing
  test, not writing one from nothing.
- Your new domain doesn't need that invariant at all — remove the I3/I4
  entries from your fork's own copy of `_rules/_contract/INVARIANTS.md`.
  This is safe to do purely on your new module's account: it does not
  touch `internal/identity`'s own coverage, which is a separate dedicated
  test under per-domain-module scope, not something borrowed from the
  `todo` domain.

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
or edits (`internal/domain/todo/`, `internal/transport/publicapi/
todo_handler.go`, and `internal/transport/bff/todo_handler.go` themselves,
its migration and query file, the two generated outputs
`internal/db`/`internal/api/openapi.gen.go`/`internal/bffapi/
bffapi.gen.go`, `openapi.yaml`'s and `bff-openapi.yaml`'s own `/todos`
content per Step 5's now-corrected delete list, `cmd/server/main.go`'s
registration, and the SPA screens under `web/src/app/` and
`web/src/lib/todos.ts` (Step 5's BFF note above covers this set) and this
repo's unrelated
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
Steps 2–5 (and Step 3b, milestone-3's own React-app rename step above),
run one more pass — **excluding `.chief/`**, which is kept deliberately
as historical record (see "`.chief/`" below) and would otherwise dominate
the results with hits that aren't about your fork at all; **excluding
`web/node_modules/` and `web/dist/`**, both generated/vendored and full
of unrelated `todo` hits (third-party library internals, your own last
built SPA bundle) that have nothing to do with your fork's domain; and
**including `docs/GETTING-STARTED.md` itself** in what you check (Step 3
already lists `docs/DEPLOY-REQUIREMENTS.md` as a rename-pass target for
the same reason — this file collects the most references of any single
file once you've forked, precisely because it's the file that explains
the domain being replaced). `*.tsx`/`*.ts` are in this pass too, milestone-3
on — Step 3b's own grep above is this same command, scoped down to just
`web/src` while you're in the middle of that step specifically:

```sh
grep -rn 'todo' --include='*.go' --include='*.md' --include='*.sql' --include='*.yaml' --include='*.tsx' --include='*.ts' . --exclude-dir=.chief --exclude-dir=node_modules --exclude-dir=dist
```

and update or remove whatever's left that's about the old domain, not
your new one.

## `.chief/`

This directory holds this template's planning history, in two layers now,
not one: per-milestone history (`milestone-1/`, `milestone-2/` — each
with its own `_goal/GOAL.md`, `_contract/` (API/DATA_MODEL/INVARIANTS),
`_plan/_todo.md`, and the task specs that built it) and `_rules/` — the
repo-wide standards and promoted contracts that outrank any single
milestone (`_rules/_standard/ARCHITECTURE.md`, `_rules/_contract/`; see
`_rules/_standard/ARCHITECTURE.md`'s "Contract promotion convention" for
how a milestone's contract graduates into `_rules/_contract/` and why its
milestone-scoped original stays in the tree as superseded history rather
than being deleted). Keep all of it after forking: it's the record of
*why* the code looks the way it does (why SQLite, why the domain/transport
split replaced milestone-1's module-first layout, why the JWT seam is
dormant, why ownership scoping works the way it does) — genuinely useful
context for whoever maintains the fork later. `internal/invariants_test.go`'s
Done-when-12 check depends on it too, so this isn't even inert scaffolding
— specifically, on `.chief/_rules/_contract/INVARIANTS.md` if a promoted
copy exists (it does, in this template, since milestone-2), and only
falls back to globbing every `INVARIANTS.md` under `.chief/` if no
promoted copy is found (see "Invariants: two things, not one" above). It's
safe to delete `.chief/` once you're confident you won't want that
history — nothing at runtime depends on it existing, and deleting it
doesn't disable any test (`findInvariantsFiles` fails loudly, not
silently, if it finds no `INVARIANTS.md` at all). If you delete it, write
your fork's own `INVARIANTS.md` (or equivalent) somewhere under a
`.chief/` of your own instead — following the promoted-file convention
(a single `_rules/_contract/INVARIANTS.md`) rather than a milestone-scoped
copy is the simpler shape for a fork with no milestone history of its own
— so that check has something to check against.
