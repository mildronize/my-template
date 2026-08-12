# Project Configuration

## Project
`my-template` — a reusable Go microservice template. Starting point for new
services on this fleet: a minimal todo API that demonstrates the intended
patterns (SSO auth, settings, agent identity) so a new service can be
scaffolded from it instead of from scratch. Derived from my-task
(/home/thw-home/gits/my-task, TS/Next.js/Drizzle), reimplemented in Go and
simplified — no projects, no activity log, 2-4 basic todo fields.

Source task: TPL-1 (https://my-task.thadaw.com/tasks/TPL-1).

## Development Commands

Repo is currently empty — no build/test/lint commands exist yet. These will be
defined during the TPL-1 milestone as the scaffold is built, and this section
should be updated once they're real (`go build ./...`, `go test ./...`,
`sqlc generate`, `goose ...`, `oapi-codegen ...` are expected but unconfirmed).

## Architecture Overview

### Tech Stack
- Language: Go
- HTTP: github.com/gin-gonic/gin
- API spec / codegen: github.com/oapi-codegen/oapi-codegen/v2 +
  github.com/oapi-codegen/gin-middleware (OpenAPI-first: handlers generated
  from spec)
- DB access: sqlc (generated, typed SQL)
- Migrations: github.com/pressly/goose/v3
- Logging: log/slog + github.com/lmittmann/tint
- Config: github.com/caarlos0/env/v11 + godotenv
- Testing: github.com/stretchr/testify

### Key Architectural Patterns
OpenAPI-first HTTP layer (oapi-codegen generates handler interfaces from
spec) backed by a service layer over sqlc-generated repositories. Exact
layering (handler → service → repo) to be confirmed in chief-plan.

### Directory Structure
Module-first (not layer-first) — see `.chief/_rules/_standard/ARCHITECTURE.md`
for the full standard and dependency rules:

```
internal/
  todo/          # example domain, deleted whole on fork
  identity/      # kept on fork — users, api_keys, actor resolution
  platform/      # kept on fork — config, logging, db, server wiring
```

## Important Development Rules
- This is a template repo: code here should read as a clean example other
  services fork from, not a one-off — keep it minimal and explained rather
  than clever.
- SSO integration, settings, and agent identity concepts must be preserved
  from my-task's design even though the todo domain model is heavily
  simplified — see TPL-1 for the constraint.
