# my-template

A reusable Go microservice + web app fork template with owner/agent auth, an OpenAPI-first API, and a worked example domain.

[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

> [WARNING ⚠️]
> This repo is written AI-agent-first: the fork checklist, the code comments,
> and the `.chief/` planning docs are dense with cross-references and
> reasoning meant for an AI coding agent to read and act on directly, not for
> a human to skim. Some of it (`docs/GETTING-STARTED.md` especially) is long
> and interleaves "why this changed" history with the actual instructions
> rather than separating them. If you're forking this by hand, read slowly —
> or better, point an AI coding assistant at the repo and have it walk you
> through the fork checklist instead of reading it cold yourself.

## About

A starting point for a new Go microservice + web app, so you fork something
that already runs instead of building from a blank repo. It gives you:

- A login flow (SSO, for a human owner)
- API-key auth (for agents, scripts, other services — no browser session needed)
- A typed API contract shared by the backend and the web frontend
- A DB layer that can't drift from its own migrations
- A worked example domain (a shared todo list with an append-only activity log)

It is a **fork target**, not a library — there's nothing to `go get` here.
Clone it, follow the fork checklist, and end up with your own service.

## Features

- **Two auth paths, one identity model** — SSO session for a human owner,
  long-lived API key (`Authorization: Bearer <key>`) for agents. Both
  resolve to the same actor concept.
- **OpenAPI-first HTTP layer** — two hand-authored specs:
  - `openapi.yaml` — Bearer-token public API for agents. Go server
    interfaces only (`oapi-codegen`).
  - `bff-openapi.yaml` — session-authenticated BFF for the web UI. Generates
    *both* Go server interfaces and the frontend's TypeScript types
    (`oapi-codegen` + `openapi-typescript`, same file). A field rename
    breaks `go build` and `npm run typecheck` at every call site — nothing
    silently drifts.
- **Typed, generated DB access** — `sqlc` generates Go from hand-written
  SQL; `goose` manages migrations. No ORM, no hand-rolled query builder.
- **A working example domain** — a shared todo list (status, assignee,
  priority, due date) with a single-write-path, append-only activity log.
- **A minimal React SPA** — settings (API key management), the todo list,
  and an activity feed. Talks to the BFF surface only.

### Tech stack

**Backend**
- Go
- [gin](https://github.com/gin-gonic/gin) — HTTP router
- [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) + `gin-middleware` — OpenAPI codegen + request validation
- [sqlc](https://sqlc.dev/) — typed SQL → Go
- [goose](https://github.com/pressly/goose) — migrations
- [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) — SQLite driver
- [lestrrat-go/jwx](https://github.com/lestrrat-go/jwx) — JWT/JWKS (SSO)
- `log/slog` + [tint](https://github.com/lmittmann/tint) — logging
- [testify](https://github.com/stretchr/testify) — testing

**Frontend** (`web/`)
- React + Vite
- TanStack Query — data fetching
- React Router
- Radix UI + Tailwind — components/styling
- [openapi-typescript](https://openapi-ts.dev/) — TS types from `bff-openapi.yaml`

## Requirements

- Go 1.26+
- Node.js 22+ (only needed to build/embed the web frontend)
- A registered OAuth2/OIDC client for the SSO login path locally — see
  [`docs/GETTING-STARTED.md`](docs/GETTING-STARTED.md) Step 1

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

SSO is unconfigured by default (`SSO_ISSUER`/`SSO_CLIENT_ID`/
`SSO_CLIENT_SECRET` unset):
- The service runs on API-key auth alone.
- The owner login path shows a "not configured" page until you set them.

See [`docs/DEPLOY-REQUIREMENTS.md`](docs/DEPLOY-REQUIREMENTS.md) for what a
real deployment needs.

## Forking this template

[`docs/GETTING-STARTED.md`](docs/GETTING-STARTED.md) is the actual fork
checklist — five ordered steps:

- SSO client registration
- Module path / service name renames
- Deleting the example `todo` domain
- The invariants/tests that need attention when you do

It's long because forking a running service touches more places than it
looks like at first glance. Skipping a step leaves a specific trace (a dead
login path, a stale module name, a leftover example handler). Read it start
to finish — don't skim for the parts that look relevant.

## Architecture

Module-first layout, not layer-first: each domain (e.g.
`internal/domain/todo`) owns its own repo/service code together, instead of
being split across parallel `handlers/`, `services/`, `repos/` trees.

- Request flow: handler → service → repo
- Only `repo.go` may import the sqlc-generated package — domain types stay
  independent of the DB schema by design, not by convention

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

Browser end-to-end tests live in `e2e/` — its own `package.json`, brings up
a local OIDC issuer and a real instance of the service, tears both down.
See `e2e/README.md`.

## License

MIT — see [`LICENSE`](LICENSE).
