# Ticket 5 Report

## Ticket
Huma foundation — adapter wiring, error envelope, shared middleware. No real route
moves off oapi-codegen yet.

## Outcome
done

## Decision
- **Issue:** the contract cited `configV1.Middlewares`/`configBFF.Middlewares` as where
  the ported `X-Actor` middleware attaches. huma v2.39.1's `Config` struct has no such
  field.
- **Options considered:** (a) treat as a genuine blocker and escalate; (b) find the
  real mechanism and use it, since this is a mechanical API-shape correction, not a
  design ambiguity — the *intent* (router-agnostic middleware, scoped to one surface's
  `huma.API`) was never in question, only which Go symbol expresses it.
- **Chosen:** (b). The real mechanism is `api.UseMiddleware(...)`, called on the
  constructed `huma.API` after `humagin.New(...)`, not a `Config` field set beforehand.
  Same effect, different call site. Not escalated as an ambiguity because there was
  only one correct answer once checked against the actual huma source — nothing for a
  human to decide between. `contract.md` corrected to match (see its "Middleware
  mapping" table).

## Notes

- **Built:** `internal/transport/humamw` (new package) — process-wide `huma.NewError`
  override (`{error:{code,message,hint}}`), `RejectActorHeader` (I1's X-Actor check),
  `RequireActor`/`RequireSession` (Bearer/session actor resolution,
  `ActorFromContext(ctx context.Context)`). 12 new tests, all passing (spot-verified
  independently: `go test ./internal/transport/humamw/... -v` — error envelope shape
  and the 422-for-validation-failure delta both confirmed live).
- Two `huma.API` instances (`apiV1H`, `apiBFF`) created in `cmd/server/main.go`,
  wrapping the same `*gin.Engine` oapi-codegen already serves. Zero operations
  registered; `OpenAPIPath`/`DocsPath` disabled so nothing new becomes reachable.
  `buildHandler` stays byte-identical (delegates to a new `buildHandlerAndHumaAPIs`) so
  every existing test compiles/passes unmodified — independently verified:
  `go build ./...`, `go vet ./...`, `go test ./...` all green.
- `main()` restructured onto `humacli`; `go run ./cmd/server openapi` dumps both specs
  as 3.0.3 YAML without starting the server — verified by the build agent (spec files
  backed up/restored around the manual test) and by design (not yet independently
  re-run here, low risk: it's inert until tickets 6/7 register real operations).
- **`make generate` deliberately NOT wired to the new `openapi` command yet** — doing so
  now would dump huma's current near-empty spec over the hand-authored
  `openapi.yaml`/`bff-openapi.yaml` that oapi-codegen still needs as *input* for every
  real route. Tickets 6 and 7 (updated with an explicit note) each wire their own
  surface's half once that surface no longer depends on the hand-authored file.
  Correct call — not itself a decision needing escalation, just a sequencing note
  carried forward into tickets 6/7's "What to build."
- `RequireActor`/`RequireSession` are built and unit-tested but **not yet attached** to
  `apiV1H`/`apiBFF` (`api.UseMiddleware(...)`) — nothing to protect yet. Tickets 6/7
  (updated) now say explicitly: reuse `humamw`, don't re-port it, attach it when
  registering real operations.
- Housekeeping, not blocking: `.chief/story-1/` and `PROJECT_SUMMARY.md` are still
  untracked in this repo's git (only ticket 5's actual code commit,
  `87ae418`, is committed) — worth a decision at some point on whether story planning
  docs get committed to `my-template` itself or stay local-only, but out of this
  ticket's scope.
