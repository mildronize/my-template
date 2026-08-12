# Milestone 1: my-template v1

Turn this empty repo into a working, minimal Go microservice — a todo API —
that future services on this fleet clone as their starting point. It exists
to prove out a pattern (SSO-backed auth, agent identity, self-service
settings) in Go, not to be a feature-complete task tracker.

Source: TPL-1 (https://my-task.thadaw.com/tasks/TPL-1). Reference: my-task
(`/home/thw-home/gits/my-task`), whose identity/auth design this milestone
reimplements in Go and whose domain model it deliberately strips down.

## Objective

Ship a runnable Go service where an agent holding an API key can create,
list, update, and delete its own todos over a REST API, and where a request
carrying a valid SSO-issued (Hydra) JWT is also accepted as an authenticated
actor. The auth/identity seam must actually work end-to-end — a template
where auth is stubbed out can't be verified or trusted as a starting point.
It must also actually be easy to fork into a new service — the todo domain
is a placeholder, and this milestone is not done until a future engineer can
tell, from the repo alone, exactly what to rip out and where their own
domain goes.

## Context

Repo is empty at the start of this milestone; everything is created here.
Grill session (2026-08-12) ran with Luna deciding low-risk technical choices
directly and consulting Clara (Engineering Director) on five higher-leverage
ones — DB engine, exact todo field set, SSO auth shape, settings scope, and
whether deployment tooling belongs in this milestone. Clara verified the DB
choice against actual fleet code rather than guessing (see Decisions table).

**This is the first Go service on the fleet.** No other service to pattern-
match against exists yet (`find ~/gits -name go.mod` returns nothing), so the
choices below — especially DB engine and the identity/auth seam shape — set
convention for every Go service that forks this template afterward. That is
why the DB engine decision is called out for มายด์'s review rather than
treated as an implementation detail.

Deployment beyond local `docker compose` (systemd, Caddy, DNS, real Hydra
client registration) is **hestia's** domain and out of scope — see
`docs/DEPLOY-REQUIREMENTS.md` for what this milestone hands her.

**SSO shape is governed by `~/gits/prod-thw-home/docs/sso-consumer-contract.md`
(status: agreed, 2026-08-09) — that document is the authority, not my-task.**
My-task's spec itself points there rather than restating it; this milestone
does the same rather than deriving auth shape secondhand from my-task's code.
Corrected during Clara's review: my-task's JWT branch
(`src/server/lib/resolve-actor.ts` → `defaultVerifyBearerJwt` →
`src/server/lib/jwt.ts`) is **wired and exercised on every Bearer request**
today, not "unused" — what's actually true is narrower: the code path runs
on every request, but no real SSO-issued JWT has ever been minted for an
agent yet, so it has never been exercised end-to-end against live Hydra. It
is a working reference implementation (with tests) to port, not something
to build from a written spec alone.

**Second correction, from the same review round:** contract §3 says Hydra
should **not** issue machine identity for a local process — only a service's
own API keys count as "vouched for" (the Admin API is unauthenticated today,
so an SSO-issued machine identity is forgeable). Since this template has no
browser/login UI, an agent authenticating via a real Hydra JWT would be
exactly the case §3 rules out. **มายด์ confirmed (2026-08-12): the JWT path
stays as a wired-but-dormant seam** — code and unit tests (against a test
issuer) are in scope, agent identity in practice is API-key only, and the
human-acceptance step asking someone to hit the service with a real Hydra
JWT is cut (see below). This mirrors exactly how my-task itself treats the
same code today.

## Decisions

| Decision | Answer | Owner |
| --- | --- | --- |
| Scope shape | Fully working service, not a bare scaffold — auth+identity+CRUD wired end-to-end | Luna |
| DB engine | **SQLite**, not Postgres. Every current fleet service is SQLite (my-task via turso/libsql, my-money, Hydra/SSO) — Postgres would be the first DB with no backup tooling and added RAM pressure on an already-tight prod host. **Sets convention for future Go services.** | Clara (verified against live fleet code) |
| DB access | sqlc (typed generated queries) + goose (migrations) | given (task stack) |
| Todo fields | Core: `title` (string, required), `done` (bool). System columns `id`, `owner_id`, `created_at`, `updated_at` exist but don't count against the "2-4 field" budget — `owner_id` exists specifically so the CRUD path proves the identity system works, not decoratively | Luna + Clara |
| Explicitly cut | projects, priority, due date, labels, assignee-distinct-from-owner, the append-only activity/event log | task (TPL-1), confirmed in grill |
| SSO shape | **Wired-but-dormant seam, not live agent auth** — contract §3 rules out Hydra issuing machine identity to a local process (forgeable while its Admin API is unauthenticated), and this service has no browser/login flow for a *human* SSO path either. JWT Bearer verification code + unit tests (test issuer only) stay in scope: RS256 pinned (never trust the token's own `alg` header, §7.2); `iss`/`aud`/`exp` validated, `AUTH_AUDIENCE` from env, never a literal (§7.1); JWKS fetched and cached, **never pin one key** (§7.3); 401 on any failure with no detail about which check failed (§7.5). Agent identity **in practice** is API-key only (§3: "a service can vouch for its own agents because it issued their keys") | Luna, corrected by Clara against the contract; confirmed by มายด์ 2026-08-12 |
| Audience convention | `AUTH_AUDIENCE` must be the service's own **public URL**, one per service per environment (contract §6) — not an opaque name. A forked service must set this to its real deployed URL | contract §6 |
| Owner invariant | A Bearer credential — API key **or** JWT — must **never** resolve to `role='owner'` (contract §5: owner authority comes only from an interactive session + password, which this service doesn't implement). Both credential paths reject `role='owner'` explicitly, mirroring my-task's I2, and this is a stopping condition, not just an intention, because the loop that builds this runs unattended | Clara |
| Agent identity | Single `users` table, `role` column (`owner`/`agent`), one actor-resolution middleware every handler goes through first (mirrors my-task's `resolveActor()`: API key → JWT → reject; JWT validation failing is the *routine* case since most Bearer tokens are API keys, not an error). Agent API keys are CLI-issued only (no web/API issuance path), matching my-task's `agent:add` pattern | Luna |
| Settings scope | Self-service API-key management only (list/revoke own keys). No configurable statuses — there's no status/group concept left to configure once `done` is a plain bool | Luna, confirmed by Clara |
| Deployment | Dockerfile + docker-compose (service + SQLite file on a volume, no separate DB container needed) is **in scope** for this milestone | Luna, confirmed by Clara |
| API design | OpenAPI-first: `openapi.yaml` at repo root, `oapi-codegen` generates server interfaces, `gin-middleware` validates requests against the spec | Luna (from task stack) |
| Testing | testify: unit tests on the service layer, integration tests on handlers against a real (temp-file or in-memory) SQLite DB. No e2e/browser tests — there's no browser | Luna |
| JWT/JWKS library | **`github.com/lestrrat-go/jwx/v3`** — not in TPL-1's original stack list (gin, oapi-codegen, gin-middleware, sqlc, goose, slog+tint, env+godotenv, testify has no JWT library), added here because task-2 needs one and the contract narrows the real choice: §7.3 requires JWKS fetched *and* cached but **never pinned to one key** (Hydra regenerates its signing key on every rebuild). `jwx/v3`'s `jwk.Cache` makes that a guaranteed library feature, not hand-rolled caching code a future forker could get wrong. The alternative, `golang-jwt/jwt/v5`, is more widely used but has no JWKS fetching/caching built in — it would need a second dependency (`MicahParks/keyfunc`) bolted on, making the §7.3 guarantee something assembled rather than something the library owns. RS256 is pinned explicitly regardless of library (§7.2 — never let the token's own `alg` header choose it; `shipd`'s `Validation::new(Algorithm::RS256)` is the fleet reference). **One new dependency, chosen because the security property it must guarantee is the property this library is for** | Clara (recommended), Luna (decided) |
| Toolchain pinning | `tools/tools.go` (`//go:build tools`) blank-imports `sqlc`, `goose`, and `oapi-codegen`'s `cmd` packages so exact versions live in `go.mod`/`go.sum`, installed via a Makefile/script target — not assumed pre-installed. Found missing entirely by Clara doing a preflight check on the actual machine (none of the three tools exist on `PATH`, `~/go/bin` doesn't exist) rather than assuming dev tooling works. Matters most for Done-when 3 (`sqlc generate` reproducibility) — an unpinned version can't guarantee byte-identical output on a fork built months later | Clara (found it), Luna |
| Go module path | `github.com/mildronize/my-template` (matches the repo). Go version: not pre-pinned — task-1 runs `go mod init` against whatever Go toolchain is actually installed and the resulting `go.mod` `go` directive is the record, rather than guessing a version number nothing has tested against | Luna |
| Architecture layout | **Module-first**, not layer-first: `internal/todo/` (example domain, deleted whole on fork), `internal/identity/`, `internal/platform/` — each holding its own handler+service+repo together, not split across parallel trees. Governed by `.chief/_rules/_standard/ARCHITECTURE.md` (repo-wide, outranks this milestone per `AGENTS.md`'s rules hierarchy — the layout must outlive milestone-1 even though the todo domain doesn't). Caught missing entirely from goal/contract by มายด์'s question during this review round: the folders were named in `_todo.md` with no rule making them mean anything, so a passthrough handler→sqlc shortcut would have passed all 10 original Done-when items | มายด์ (layout choice), Clara (found the gap) |

## Scope

### In scope

- `cmd/server` entrypoint, config via `caarlos0/env/v11` + `godotenv`, structured logging via `log/slog` + `lmittmann/tint`
- `users` table (`id`, `handle`, `role`, `active`, `sso_subject` nullable, timestamps) and the actor-resolution middleware (agent API key → JWT → reject)
- Agent API key issuance via a CLI script (mirrors my-task's `agent:add`), stored hashed, prefixed, with an expiry
- JWT Bearer verification against `SSO_ISSUER` via `lestrrat-go/jwx/v3` (env-driven, dev vs prod audiences distinct)
- `todos` table and full CRUD scoped to the authenticated actor's own rows: create, list (own only), get, update (`title`/`done`), delete
- `GET /api/v1/me`
- Settings surface: `GET /api/v1/keys` (list own), `DELETE /api/v1/keys/:id` (revoke own)
- `openapi.yaml` covering every endpoint above, oapi-codegen-generated server interface, gin-middleware request validation
- goose migrations for the schema above; sqlc queries and generated code
- Dockerfile + docker-compose for local dev (service + SQLite volume)
- `docs/DEPLOY-REQUIREMENTS.md` for hestia, listing what a real deployment needs (Hydra client registration incl. the public-URL audience per §6, env vars, volume for the SQLite file)
- `docs/GETTING-STARTED.md` — the fork checklist: what to rename (module path, service name), what to re-register (`AUTH_AUDIENCE` to the new service's own public URL), where the todo domain lives so it's obvious what to delete and replace with a real domain, and that `tools/tools.go`'s pinned versions are a deliberate choice to re-check/bump on fork, not an artifact to ignore
- Unit + integration tests (testify) covering the auth/identity seam (including the owner-invariant above) and CRUD ownership scoping

### Out of scope

Projects · priority · due dates · labels · an assignee distinct from the
creator · an append-only activity/event log · a browser session/cookie login
flow · a web UI of any kind · Postgres or any DB other than SQLite · CI
pipeline setup · real Hydra client registration or any change to
`prod-thw-home` · rate limiting · multi-tenancy beyond per-owner row scoping.

## Done when

Machine-checkable stopping conditions for the unattended loop:

1. `go build ./...` and `go vet ./...` pass, and `tools/tools.go` pins exact versions of `sqlc`, `goose`, and `oapi-codegen` in `go.mod`/`go.sum` — no step in this milestone relies on a tool already being installed on whatever machine runs it.
2. `goose up` applies cleanly against a fresh, empty SQLite file.
3. `sqlc generate` produces no diff against committed generated code (queries and schema stay in sync).
4. `go test ./...` passes, and includes: an agent with only its own key can create/list/update/delete only its own todos, and gets 404 (not 403 — no leaking existence) touching another owner's todo.
5. A `users` row with `role='owner'` is rejected as unauthorized when presented over Bearer — tested for **both** the API-key path and the JWT path (contract §5). This is checked directly, not inferred from the ownership-scoping test in (4).
6. A request with a missing, malformed, expired, or revoked credential gets 401 with no detail about which reason applied.
7. A request that violates `openapi.yaml` (missing required field, wrong type) is rejected by gin-middleware's spec validation before it reaches handler code.
8. `docker compose up` brings the service up against its SQLite volume, and `GET /api/v1/me` with a seeded agent key returns that agent's handle.
9. `docs/DEPLOY-REQUIREMENTS.md` exists and lists everything hestia needs (env vars, Hydra client fields incl. the public-URL audience, volume) without follow-up questions.
10. `docs/GETTING-STARTED.md` exists and contains an explicit, separately-labeled step for each of: renaming the Go module path, renaming the service, re-registering `AUTH_AUDIENCE`, and locating where the todo domain lives to replace it. Checked by presence of each labeled step, not by the loop's own judgment that the doc is clear — see Human acceptance below for why that distinction matters here.
11. The architecture import-graph test (`.chief/_rules/_standard/ARCHITECTURE.md`) passes: only handler-role files (`handler.go`/`*_handler.go`) in `internal/todo/` or `internal/identity/` import `gin`; only repo-role files (`repo.go`/`*_repo.go`) import the sqlc-generated package; `internal/platform` imports no domain module.
12. Every invariant `_contract/INVARIANTS.md` numbers (I1–I10) has at least one test whose name references that invariant number, checked by grepping test names against the I1–I10 list. `INVARIANTS.md`'s own opening line says test names are how it expects to be traced — this stopping condition is what makes that true instead of aspirational. One check, not ten: it doesn't matter which task's tests satisfy a given invariant, only that by the time this is checked, all ten are named somewhere.

### Human acceptance — after the loop, not part of it

**The Hydra-JWT criterion from earlier drafts is cut, not replaced** — see
Context and the SSO shape row above; asking a human to mint the exact
credential `sso-consumer-contract.md` §3 says shouldn't exist for this kind
of service was the wrong ask.

**A different, narrower one stays, caught by Clara during review:** Done-when
10 can only check that `docs/GETTING-STARTED.md` names the right steps, not
that a stranger can actually follow them — the loop grading its own
documentation against its own code is the exact shape of
`rigour-mechanisms-certify-instead-of-testing` (fleet learning), since
writer and checker share every bit of context a real forker wouldn't have.

**Someone outside this loop — human or agent, but not Luna and not anyone who
watched this milestone get built — reads only `docs/GETTING-STARTED.md`
(no source code, no asking Luna) and forks the template into a differently-
named service with its own resource in place of `todos`, until
`GET /api/v1/me` answers.** Wherever they get stuck is exactly what the doc
is missing. Can't be a stopping condition — the loop cannot recruit a
context-free reader for itself — so this happens once, after, same shape as
the criterion it replaces.
