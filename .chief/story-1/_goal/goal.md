# Goal

Replace `oapi-codegen` + Gin with [huma](https://github.com/danielgtaylor/huma) as the
HTTP/OpenAPI layer for **both** API surfaces — public API (`/api/v1`) and BFF
(`/api/bff`) — so the backend has one code-first framework generating its own spec,
instead of two hand-authored spec files (`openapi.yaml`, `bff-openapi.yaml`, 1,363 lines
combined) each feeding a separate `oapi-codegen` run (1,532 lines of generated
server-interface code, `internal/api/openapi.gen.go` + `internal/bffapi/bffapi.gen.go`).

From มายด์'s perspective, this story is done when:

- Both surfaces run on huma; `oapi-codegen`, the two hand-authored spec files, and their
  generated output are gone from the repo.
- Every existing route, request/response shape, and the `{error:{code,message,hint}}`
  error envelope behave exactly as before — this is a mechanical swap of *how* the HTTP
  layer is built, not a behavior change. `make test` (Go + Vitest), `make smoke`, and
  `make e2e` all still pass unmodified.
- Actor/ownership invariants (I1 actor-field rejection, I3 404-not-403, I4 identity-seam
  isolation) hold exactly as documented in `INVARIANTS.md` — no regression, and I1 comes
  out *smaller* (body/query rejection becomes huma-native config instead of hand-rolled
  middleware logic; only the `X-Actor` header check stays bespoke).
- A fork can still read a single committed OpenAPI spec file per surface without
  building or running anything — `make generate` gains a step that dumps huma's
  generated spec to `openapi.yaml`/`bff-openapi.yaml` (same filenames, same purpose,
  different origin: derived from code instead of hand-authored). `openapi-typescript`
  keeps reading `bff-openapi.yaml` unchanged.
- `docs/GETTING-STARTED.md` and `_rules/_contract/API.md`'s one sentence naming
  `gin-middleware` as the actor-field-rejection mechanism are updated to match the new
  implementation.

This is also this repo's first story run under chief-v5's story/ticket model (see
`.chief/story-1/_map.md` for how its scope was charted) — that process itself is มายด์'s
own thing to evaluate, not a goal this story tracks or reports on.

## Out of Scope

- **Frontend / React SPA.** `docs/research/frontend-lightweight-alternatives.md` already
  established huma doesn't reduce frontend LOC (the Go↔TS boundary is the actual cost,
  not the spec-gen tool) — this story's LOC/complexity goal is backend-only. No `web/`
  file changes.
- **`INVARIANTS.md` rewrite.** Wayfinder ticket 4 confirmed its I1/I3/I4 text is already
  implementation-agnostic and needs no changes.
- **Any new invariant, endpoint, or behavior.** This is a like-for-like framework swap —
  no new routes, no new validation rules beyond what huma gives for free on the
  invariants that already exist, no auth-mechanism changes (Hydra/session handling is
  untouched).
- **DB / sqlc layer, migrations, `internal/dbquery` static checks.** Confirmed orthogonal
  to this migration (wayfinder ticket 4) — not touched.
- **Sequencing/ticket-slicing** (one PR for both surfaces vs. split) — a `/chief-plan`
  Phase 3 concern, decided in this document's ticket breakdown, not a goal-level
  question.
