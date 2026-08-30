# my-template

[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A starting point for a new Go microservice: owner/agent authentication (SSO
session for a human owner, API keys for agents), an OpenAPI-first HTTP layer,
and a small worked example (a shared todo list with an activity log) showing
how the pieces fit together — so a new service can be forked from something
that already runs, instead of built from a blank repo.

## About

Most internal services end up rebuilding the same handful of things: a login
flow, a way for non-human callers (agents, scripts, other services) to
authenticate without a browser session, a typed API contract that both the
backend and a web frontend can rely on, and a database layer that doesn't
drift from the migrations that created it. `my-template` builds all of that
once, wires it together around one small domain (todos with status,
assignee, priority, and an append-only activity log), and documents exactly
what to rename, delete, or replace to turn it into a different service.

It is a **fork target**, not a library — there's nothing to `go get` here.
You clone it, follow the fork checklist, and end up with your own service
sharing none of its git history-independent identity with this one.

## Features

- **Two auth paths, one identity model** — a human owner logs in through SSO
  (session cookie); agents authenticate with a long-lived API key
  (`Authorization: Bearer <key>`). Both resolve to the same actor concept
  the rest of the codebase works with.
- **OpenAPI-first HTTP layer** — two hand-authored specs: a Bearer-token
  public API for agents (`openapi.yaml`, Go server interfaces only, via
  `oapi-codegen`) and a session-authenticated BFF for the web UI
  (`bff-openapi.yaml`, generating *both* Go server interfaces and the
  frontend's TypeScript types, via `oapi-codegen` and `openapi-typescript`
  from the same file). A field rename in the BFF spec breaks `go build`
  and `npm run typecheck` at every call site on both sides — nothing
  silently drifts.
- **Typed, generated DB access** — `sqlc` generates Go from hand-written SQL;
  `goose` manages migrations. No ORM, no hand-rolled query builder.
- **A working example domain** — a shared todo list (status, assignee,
  priority, due date) with a single-write-path, append-only activity log,
  demonstrating the append-only/idempotent-write patterns a real service
  will likely need anyway.
- **A minimal React SPA** — settings (API key management), the todo list,
  and an activity feed, talking to the BFF surface only.

## Requirements

- Go 1.26+
- Node.js 22+ (only needed to build/embed the web frontend)
- A registered OAuth2/OIDC client if you want the SSO login path working
  locally (see [`docs/GETTING-STARTED.md`](docs/GETTING-STARTED.md) Step 1)

## Quickstart

```sh
make tools      # installs pinned sqlc/goose/oapi-codegen into ./bin
make generate   # sqlc + oapi-codegen, from db/ and the two openapi specs
make build      # builds the web SPA, then the Go binary (in that order)
make run        # go run ./cmd/server
```

Or with Docker:

```sh
docker compose up
```

SSO is left unconfigured by default (`SSO_ISSUER`/`SSO_CLIENT_ID`/
`SSO_CLIENT_SECRET` unset) — the service starts and runs on API-key auth
alone; the owner login path answers with a "not configured" page until you
set those. See [`docs/DEPLOY-REQUIREMENTS.md`](docs/DEPLOY-REQUIREMENTS.md)
for what a real deployment needs.

## Forking this template

[`docs/GETTING-STARTED.md`](docs/GETTING-STARTED.md) is the actual fork
checklist — five ordered steps covering the SSO client registration, module
path/service name renames, deleting the example `todo` domain, and the
invariants/tests that need attention when you do. It's long because forking
a running service correctly touches more places than it looks like at first
glance; each step exists because skipping it leaves a specific, identifiable
trace (a dead login path, a stale module name, a leftover example handler).
Read it start to finish before making changes — don't skim for the parts
that look relevant.

## Architecture

Module-first layout, not layer-first: each domain (e.g. `internal/domain/todo`)
owns its own repo/service code together, rather than being split across
parallel `handlers/`, `services/`, `repos/` trees. A request flows
handler → service → repo, with the sqlc-generated package importable only
from `repo.go` — domain types stay independent of the database schema by
design, not by convention.

```
internal/
  domain/        # business logic, one subdirectory per domain (todo is the example)
  identity/      # auth: SSO session handling, API key issuance/verification
  platform/      # config, DB connection, migrations
  transport/
    publicapi/   # Bearer-token surface, for agents
    bff/         # session-authenticated surface, for the web SPA
web/             # React SPA (Vite), talks to internal/transport/bff only
db/
  migrations/    # goose
  queries/       # sqlc source
```

## Testing

```sh
make test         # go test ./... + the web test suite
make vet          # go vet ./...
make smoke        # exercises a running instance (does not start one)
```

Browser end-to-end tests live in `e2e/` (its own `package.json`, brings up
a local OIDC issuer and a real instance of the service, tears both down) —
see `e2e/README.md`.

## License

MIT — see [`LICENSE`](LICENSE).
