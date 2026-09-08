# 1: Does the huma migration's LOC/complexity goal cover the frontend, or backend only?

Type: wayfinder:grilling
Status: resolved
Blocked by: None (can start immediately)

## Question

ENG-37 says "my-template v2 will migrate to huma, expectation to reduce complexity of
code and LOC." `docs/research/frontend-lightweight-alternatives.md` §7 already concluded
huma is "not recommended as a full swap" because it doesn't touch the frontend LOC
complaint (the Go↔TS boundary is the actual cost, not which tool generates the Go-side
spec). Does ENG-37 intend to override that conclusion, or is its scope narrower?

## Answer

Backend only. The LOC/complexity goal is scoped to the Go backend (removing the
hand-authored OpenAPI files + oapi-codegen generated glue) — the frontend/React SPA is
untouched and out of scope for this story. This does not conflict with the research
doc's conclusion: that doc was evaluating huma as a fix for the *frontend* LOC
complaint, which this story doesn't claim.

Resolved by มายด์, 2026-09-06.
