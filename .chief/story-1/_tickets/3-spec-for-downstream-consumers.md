# 3: How do downstream consumers (frontend TS gen, fork docs) get an OpenAPI spec once huma is code-first?

Type: wayfinder:grilling
Status: resolved
Blocked by: 2 (resolved)

## Question

huma derives its spec from code (no hand-authored `.yaml` file). Two things currently
depend on a spec *file on disk*: `openapi-typescript` (generates TS types for the BFF
surface) and `docs/GETTING-STARTED.md`'s fork workflow, which expects a contract file a
fork can read without building/running the service. How does the migration preserve
this?

## Answer

Verified against huma's own docs (`~/ref-idea/docs/huma/docs/docs/tutorial/client-sdks.md`,
`.../features/cli.md`) — this is huma's own documented pattern, not a workaround:

Add a `humacli` custom command (`cli.Root().AddCommand(...)`) that calls
`api.OpenAPI().DowngradeYAML()` (or `.YAML()` for 3.1) and prints it — runnable as
`go run . openapi > path/to/spec.yaml` **without starting the server**. Wire this into
`make generate`, commit the dumped file, and point `openapi-typescript` at it exactly as
today. This preserves the "readable without a build" property `GETTING-STARTED.md`
depends on.

huma also serves the spec live by default (`/openapi.json`, `/openapi.yaml`,
`/openapi-3.0.yaml` per `huma.Config`) — useful for dev-time inspection, but not a
substitute for the committed file, since a fork needs to read the contract before
running anything.

Resolved by มายด์, 2026-09-06 (verified against huma source before recording).
