# 2: Migrate one API surface to huma, or both?

Type: wayfinder:grilling
Status: resolved
Blocked by: 1 (resolved)

## Question

my-template has two OpenAPI-described surfaces: public API (`/api/v1`, Bearer auth,
`openapi.yaml`) and BFF (`/api/bff`, session auth, `bff-openapi.yaml`, the only one the
frontend calls). Given decision 1 (backend-only complexity goal), does this story
migrate both surfaces to huma, or just one?

## Answer

Both, together, in this story. Migrating only one surface would leave oapi-codegen and
huma coexisting permanently (not as a transition state), which works against the
story's own goal of reducing backend complexity — two codegen pipelines is more
complexity, not less.

Risk noted: BFF's spec also feeds `openapi-typescript` for the frontend, so the
downstream-consumer question (ticket 3) applies to BFF specifically, not just public
API.

Resolved by มายด์, 2026-09-06.
