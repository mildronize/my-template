## Destination

"Fog clear" for this story means: scope is fixed on reducing Go-backend LOC/complexity
(oapi-codegen removal) for **both** `/api/v1` and `/api/bff`, with a settled answer for
how downstream consumers (frontend TS gen, fork docs) still get a spec file once huma is
code-first, and a clear read on whether this repo's actor/ownership invariants have a
huma-native equivalent — everything `/chief-plan` needs to write a contract that doesn't
hand-wave the invariant story.

## Notes

- ENG-37 (my-task), assignee luna, branch `main.v2`.
- huma source cloned locally at `~/ref-idea/docs/huma` — read it directly rather than
  guessing or re-fetching from the web.
- Prior research already on record and merged to `main`:
  `docs/research/frontend-lightweight-alternatives.md` §3 (huma eval, exploratory, no
  code changed). Its "not recommended as a full swap" conclusion is about the
  *frontend* LOC complaint specifically — this story's backend-only scope (ticket 1)
  does not conflict with it.
- This is the first story in this repo to use chief-v5's story/ticket model — repo
  previously only had v4-style `.chief/milestone-1..4/`. มายด์ is evaluating the
  chief-v5 process himself through this session; that evaluation is not a deliverable
  of this story (see Out of scope).
- Skill to use for the open research ticket: `research`.

## Decisions so far

- [Scope: backend-only vs full repo](_tickets/1-scope-backend-only.md): Backend only —
  frontend/SPA untouched, doesn't conflict with the research doc since that doc was
  scoped to the frontend complaint.
- [Scope: one surface or both](_tickets/2-scope-both-surfaces.md): Both `/api/v1` and
  `/api/bff` migrate together, in this story — avoids a permanent two-codegen-pipeline
  end state.
- [Spec file for downstream consumers](_tickets/3-spec-for-downstream-consumers.md):
  Add a `humacli` custom `openapi` command (`api.OpenAPI().DowngradeYAML()`, per huma's
  own client-sdks.md tutorial) wired into `make generate`, dumped file committed —
  preserves `GETTING-STARTED.md`'s build-free-readable-contract property.
- [Invariant enforcement mapping](_tickets/4-invariant-enforcement-mapping.md): I1
  (actor-field rejection) gets *better* under huma — body/query become zero-code via
  default `additionalProperties:false` and `RejectUnknownQueryParameters`, only the
  `X-Actor` header check stays a (smaller) bespoke middleware. I3 (404-not-403) and I4
  (identity-seam + `internal/dbquery` static checks) live entirely below the transport
  layer and are unaffected either way — huma neither helps nor hurts them. No invariant
  gets harder to enforce.
- `_rules/_contract` docs: no rewrite needed as part of this story.
  `INVARIANTS.md`'s I1/I3/I4 text is already implementation-agnostic (states the
  property, never names oapi-codegen/Gin) and carries over unchanged. `API.md` has
  exactly one sentence naming a mechanism that goes stale ("...rejected by
  `gin-middleware` before reaching handler code at all") — a one-line fix within this
  story's own PR, not a separate rewrite story.

## Not yet specified

*(none — sequencing of one PR vs. multiple tickets for the two surfaces is left to
`/chief-plan`'s own ticket-slicing; it doesn't affect what the goal/contract need to
say, so it was never wayfinder-level fog.)*

## Out of scope

- React SPA / frontend code changes — research doc already shows huma doesn't reduce
  frontend LOC; not this story's goal (ticket 1).
- Broader evaluation of the chief-v5 process itself — มายด์ is assessing that directly
  through this session, not something this story tracks or reports on.
