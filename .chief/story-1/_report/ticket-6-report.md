# Ticket 6 Report

## Ticket
Migrate `/api/v1` (public API) fully onto huma — the entire surface in one atomic step
(`internal/api.RegisterHandlers` registers every route in one call, so no clean
partial migration exists within the surface).

## Outcome
done (two rounds — first attempt blocked, resolved, second attempt found and fixed a
real regression before landing)

## Decision

- **Issue 1 — test-harness contradiction.** Ticket required both deleting
  `internal/api` and every `internal/transport/publicapi/*_test.go` passing
  "unmodified." A first build attempt correctly stopped rather than guess: deleting
  `internal/api` breaks compilation of 3 of 4 test files outright (they import its
  generated types and directly wire gin-specific plumbing).
- **Options considered** (via a decision-support pass): (1) keep a types-only
  `internal/api` shim just for tests — rejected, contradicts the already-approved
  contract's "internal/api/ no longer exists"; (2) keep `internal/api` alive entirely —
  rejected, defeats the story's LOC-reduction goal; (3) treat test *harness* code
  (router construction, middleware-mounting, type import paths) as legitimately
  changeable when the transport framework changes, while every test *assertion*
  (expected status/body/error values) stays identical — confirmed concretely: the
  response types are plain framework-agnostic structs, so only construction code
  needed rewriting, not assertion values; (4) escalate/stall on a contract amendment.
- **Chosen:** (3), refined and written into `contract.md`'s Testing Decisions section
  precisely (harness may change, assertions may not) — not escalated to มายด์, since
  `/chief-loop` is auto-mode and this had a clear, defensible resolution once
  investigated rather than a genuine multi-way judgment call.

- **Issue 2 — real behavior regression found mid-build, not just a harness question.**
  The retry (built under the clarified rule) found that two tests
  (`TestI1_RejectActorFields_BodyField`/`QueryParam`) needed an *added*
  `Authorization` header to keep passing — not just an import/harness change. Root
  cause: relying on huma's automatic `additionalProperties:false`/
  `RejectUnknownQueryParameters` defaults for I1's body/query actor-field check (the
  contract's original plan) moves that check from *before* auth to *after* auth, since
  huma's core body/query validation runs inside the chain every `api.UseMiddleware`
  (including `RequireActor`) wraps entirely. This reverses I1's explicit, commented
  design intent ("a shape violation never spends a DB lookup on credential
  resolution") and the goal's "behave exactly as before" — a real regression, not
  acceptable harness churn, and not something the earlier 400→422 status delta
  decision covers (that was about a *different*, already-disclosed class of
  validation failure).
- **Options considered:** accept the ordering delta (reasoned it might be
  security-neutral-or-positive, since unauthenticated callers now learn less) vs. fix
  it by keeping a bespoke pre-auth check for body/query too (mirroring the header
  check ticket 5 already proved works pre-auth), on top of huma's defaults as a
  second-layer catch-all for non-actor fields.
- **Chosen:** fix it. The goal's zero-behavior-change bar was stated explicitly and
  repeatedly through this story's planning; accepting a second, undisclosed ordering
  delta on top of the one already negotiated erodes that bar further than warranted,
  especially when a concrete, low-risk fix existed (extend the already-working
  pre-auth header-check middleware to also cover body/query, exactly mirroring the
  original hand-rolled logic). `contract.md`'s middleware-mapping table updated
  accordingly before dispatching the fix.

## Notes

- Final state, independently verified (build/vet/`go test ./internal/...` re-run by
  the orchestrating session, not just trusted from the build agent's own report):
  clean build, clean vet, all tests green.
- The fix-up build also found and fixed a second, independent stale assumption in
  `cmd/smoke/main.go` (asserted `422` where `400 actor_field_present` was correct) —
  caught by `make smoke` (28/31 → 31/31), not by `go test`, which is exactly the kind
  of gap `cmd/smoke`'s own doc comment says it exists to catch (real HTTP, real
  server, not an in-process gin context).
- The fix-up build proved its own restore-the-body logic actually matters by
  temporarily breaking it and confirming a real test failure, then restoring — the
  right level of rigor for a change this easy to get subtly wrong (a body-restore bug
  would silently break every real request, not just the actor-field-rejection path).
- `internal/api/` is gone; `oapi-codegen`'s public-API `Makefile` invocation is gone;
  `make generate` now dumps `openapi.yaml` via `go run ./cmd/server openapi v1`.
- `docs/GETTING-STARTED.md`/`API.md`'s stale mechanism references intentionally not
  touched here — `API.md`'s sentence is still partially true until ticket 7 lands
  (BFF still uses the old gin-middleware validator); ticket 8 owns the final fix.
- Commits: `cb1723f` (initial migration), `b141e19` (I1 ordering fix).
