# 5: Huma foundation — adapter wiring, error envelope, shared middleware

Type: implementation
Status: resolved
Blocked by: None (can start immediately)

## What to build

The shared plumbing both surface migrations (tickets 6, 7) depend on, built and
verified in isolation before any real route moves onto it. Nothing in
`/api/v1`/`/api/bff` changes behavior yet — oapi-codegen still serves every existing
route; this ticket adds huma alongside it, unused by real traffic until ticket 6/7.

- Add `github.com/danielgtaylor/huma/v2` + `.../adapters/humagin` to `go.mod`.
- Create two `huma.API` instances in `cmd/server/main.go`, both wrapping the existing
  `*gin.Engine` (`humagin.New(router, config)`): `apiV1` (public), `apiBFF` — each with
  its own `Info.Title`/`Info.Version` matching today's two spec files' `info` blocks,
  and `Config.RejectUnknownQueryParameters = true` (contract's I1 query-check mapping).
- Override `huma.NewError` (process-wide, once) to build the `ErrorEnvelope`
  (`{error:{code,message,hint}}`) type instead of huma's default RFC9457 model —
  contract's "Error envelope" section.
- Port `RejectActorFields`'s `X-Actor` header check (the one piece that stays bespoke —
  body/query rejection is now free via huma defaults) as a router-agnostic
  `huma.Middlewares` function, registered on both `apiV1` and `apiBFF` configs.
- Port `RequireActor` (Bearer, public) and the BFF session-equivalent as router-agnostic
  middleware storing the resolved actor via `context.WithValue`; add an
  `ActorFromContext(ctx context.Context)` helper replacing today's
  `ActorFromContext(c *gin.Context)`.
- Add the `humacli` `openapi` custom Cobra command (contract's "OpenAPI spec output"
  section) — `go run . openapi` dumps both `apiV1.OpenAPI().DowngradeYAML()` and
  `apiBFF.OpenAPI().DowngradeYAML()`. Wire it into `make generate`. (Output will be
  near-empty until tickets 6/7 register real operations — that's expected at this
  stage.)

## Acceptance Criteria

- [ ] New unit tests for the ported middleware: `X-Actor` header rejection,
      actor-context propagation, error-envelope JSON shape — these are **new** tests
      (nothing existing covers unwired code), not modifications to existing test files.
- [ ] `go build ./...` and `go vet` pass.
- [ ] Every existing test (`make test`, `make smoke`, `make e2e`) still passes
      **unmodified** — no route has moved yet, so nothing observable has changed.
- [ ] `go run . openapi` runs without starting the server and produces valid (if
      currently sparse) YAML for both surfaces.
