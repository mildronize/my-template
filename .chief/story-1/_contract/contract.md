# Contract

## Framework & wiring

- **huma v2** (`github.com/danielgtaylor/huma/v2`) with the **`humagin`** adapter
  (`github.com/danielgtaylor/huma/v2/adapters/humagin`) — no router change. The existing
  single `*gin.Engine` (`internal/platform/server.go:32`, `gin.New()`) stays exactly as
  is, including its global Gin middleware (request-id, logging, recovery,
  `internal/platform/middleware.go`).
- **Two `huma.API` instances**, both wrapping that same `*gin.Engine`:
  - `apiV1 := humagin.New(router, configV1)` — public API
  - `apiBFF := humagin.New(router, configBFF)` — BFF
  - `humagin.New` binds to the full `*gin.Engine`, not a route group — today's
    `router.Group("/api/v1")` / `router.Group("/api/bff")` (`cmd/server/main.go:188,351`)
    are **removed**; every `huma.Operation.Path` is the full absolute path
    (`/api/v1/todos`, `/api/bff/todos/{id}`, etc.) instead of group-relative.
  - Each surface keeps its own OpenAPI `Info.Title`/`Info.Version`
    (`huma.DefaultConfig("...", "...")` per surface, matching today's two separate spec
    files' own `info` blocks).
  - Surface-specific auth stays surface-specific *because it's a different `huma.API`
    instance*, not because of a route group: `apiV1`'s middlewares carry Bearer/actor
    resolution, `apiBFF`'s carry session-cookie resolution. Nothing else needs the old
    per-surface Gin group.

## Middleware mapping (I1, I4, I5)

| Today (Gin, oapi-codegen) | huma equivalent |
|---|---|
| `RejectActorFields` — body-field check (`middleware.go:130-152`) | **Correction (found building ticket 6): stays bespoke, folded into the same pre-auth middleware as the header check — not fully replaced by huma's default.** Original plan was "gone, huma's `additionalProperties:false` default catches it" — true for the *rejection itself*, but huma's core body/query validation runs *inside* the parse→validate→handle chain, which every `api.UseMiddleware` (including `RequireActor`) wraps entirely (`huma.go:881`). So relying on the default alone means actor-field rejection moves from *before* auth to *after* auth — reversing I1's own explicit design intent (`middleware.go`'s comment: "so a shape violation never spends a DB lookup on credential resolution") and the story's goal of zero behavior change. Confirmed empirically: ticket 6's build hit exactly this — `TestI1_RejectActorFields_BodyField`/`QueryParam` started needing a valid `Authorization` header just to reach the check. **Decision: keep the bespoke pre-auth check for body too**, extended into the same middleware as the header check (ticket 5's `RejectActorHeader`, renamed to reflect the wider scope) — mirrors `hasForbiddenBodyField`'s exact approach (peek at the raw body via `huma.Context.BodyReader()`, restore it for huma's own parsing afterward — needs adapter-level access since the `Context` interface has no body-reset method; `humagin.Unwrap(ctx)` is the escape hatch). Huma's `additionalProperties:false` default still applies underneath as a second-layer catch-all for non-actor unrecognized fields (typos etc.) — the two aren't in conflict, the bespoke check just runs first and short-circuits for the three actor-field names specifically. |
| `RejectActorFields` — query-param check (`middleware.go:135-140`) | **Same correction as the body-field row** — stays bespoke, in the same extended pre-auth middleware (`ctx.URL().Query()`), for the identical ordering reason. `Config.RejectUnknownQueryParameters = true` stays set too (`api.go:248`) as the second-layer catch-all for non-actor unrecognized query params, same relationship as the body case above. |
| `RejectActorFields` — `X-Actor` header check (`middleware.go:125-128`) | **Stays bespoke**, ported as one router-agnostic huma middleware, registered on **both** `apiV1H` and `apiBFF` (I1 applies to both surfaces per `INVARIANTS.md`). **Correction (found building ticket 5): `huma.Config` has no `Middlewares` field** — the contract's original citation of `configV1.Middlewares`/`configBFF.Middlewares` doesn't exist in huma v2.39.1. The real mechanism is `api.UseMiddleware(...)`, called on the constructed `huma.API` instance after `humagin.New(...)` returns, not set on `Config` beforehand. Same effect (router-agnostic, applies to every operation on that `huma.API`), different call site. Implemented in `internal/transport/humamw`. Runs before body parsing (confirmed: `huma.go:881`'s wiring wraps the entire parse→validate→resolve→handle chain in `api.Middlewares().Handler(...)`), preserving today's "fail on shape before spending a DB lookup" ordering. |
| `RequireActor` (`middleware.go:184-196`, I4/I5) | Ported as a router-agnostic middleware per surface (Bearer-credential version on `apiV1`, session-cookie version on `apiBFF`), calling the same `identity.Service.ResolveActor`/session-resolution code as today. Resolved actor is stored via `context.WithValue` on the `context.Context` huma passes into the operation handler (huma operation handlers receive a plain `context.Context`, not `huma.Context` — `huma.go:779`'s `Register[I, O any](api API, op Operation, handler func(context.Context, *I) (*O, error))`). A small `ActorFromContext(ctx context.Context)` helper replaces today's `ActorFromContext(c *gin.Context)` — same I4 property (one seam reads identity; handlers never look it up themselves), same I5 property (every resolution failure writes the identical `unauthorized` body, regardless of which check inside `ResolveActor` failed). |

## Error envelope (I5, API.md's error table)

Override `huma.NewError` (`error.go:245`, an overridable package-level `var`, per
`docs/docs/features/response-errors.md:132`) once, at startup, to build
`ErrorEnvelope{Error: {Code, Message, Hint}}` — the exact JSON shape `API.md` documents
today — instead of huma's default RFC9457 `application/problem+json` body. This is a
process-wide override, not per-operation.

The status/code table in `_rules/_contract/API.md` does not change:

| Code | Status | Mechanism after migration |
|---|---|---|
| `unauthorized` | 401 | `RequireActor`-equivalent middleware, same single write site (I5) |
| `not_found` | 404 | unchanged — domain-layer sentinel error → `huma.Error404NotFound` (drop-in swap for today's `http.StatusNotFound` write, `error.go:315`) |
| `actor_field_present` | 400 | `X-Actor` header middleware only (body/query now 422 from request validation — see "Behavior note" below) |
| `validation_error` | 400/422 | see "Behavior note" |
| `invalid_transition` | 403 | unchanged — domain/permission-layer check (I18), same status/code, huma-agnostic |

**Behavior note, flagged rather than silently absorbed:** today, a body/query actor-field
violation and a generic shape-validation failure (missing required field, wrong type)
both surface as this repo's own codes at **400** (`actor_field_present`, or
`gin-middleware`'s generic OpenAPI-validation rejection per `API.md`'s "Error shape"
section — note that sentence itself is the one going stale, and gets corrected in this
story per the goal). Huma's own default status for a request-validation failure
(unknown field, wrong type, failed `additionalProperties`) is **422**, not 400
(`docs/docs/features/request-validation.md`). Decision: **keep huma's native 422 for
generic shape-validation failures** (do not force it back to 400) — the goal is
behavior-preserving for what a *correct* client sees and for every named invariant, not
byte-identical status codes on a class of malformed request nothing in `INVARIANTS.md`
numbers. The `X-Actor` header case is the one path that keeps its own `400
actor_field_present`, because I1 specifically names it and a test already asserts it.

## `additionalProperties` exceptions (preserve today's permissiveness)

huma defaults every input struct to `additionalProperties: false`. Checked every
request-body schema in both spec files against this: **`CreateTodoEventRequest`**
(`openapi.yaml:477-501`, body of `POST /todos/{id}/events`) is the one request schema
today that does **not** declare `additionalProperties: false` — under OpenAPI's own
default semantics (and confirmed against this repo's actual oapi-codegen/kin-openapi
validation behavior), an unrecognized field on this endpoint is silently accepted today.
Left on huma's default, this endpoint would newly *start* rejecting such requests (422)
— a real behavior change, not just the generic 400→422 status delta above.

**Decision: preserve today's permissiveness.** Tag `CreateTodoEventRequest`'s huma
struct with the documented escape hatch (`docs/docs/features/request-validation.md:200-214`)
— a dummy field carrying the tag, not a struct-level annotation:

```go
type CreateTodoEventRequest struct {
    _               struct{} `json:"-" additionalProperties:"true"`
    Type            string   `json:"type"`
    ClientRequestID string   `json:"clientRequestId" minLength:"1"`
    Body            *string  `json:"body,omitempty"`
    To              *string  `json:"to,omitempty"`
    Field           *string  `json:"field,omitempty"`
}
```

Per-struct only — do **not** set `Config.AllowAdditionalPropertiesByDefault = true`
(the global switch), which would silently give every other endpoint back the
permissiveness they *don't* have today and this migration is supposed to tighten for
free. Same check applies to `bff-openapi.yaml`'s request schemas before implementation —
this repo's convention is symmetric enough between the two surfaces that the equivalent
BFF endpoint (if one exists with the same shape) should get the same tag, but this needs
verifying against `bff-openapi.yaml` directly, not assumed from the public-API finding.

## Free-form fields

`payload` (`todo_events`' response field, `openapi.yaml:361-375` /
`bff-openapi.yaml:pending`) is `type: object, additionalProperties: true, nullable:
true` today — a deliberately shapeless JSON blob, not a strict-schema escape hatch. Maps
to a Go field typed `map[string]any` (or `*map[string]any`) in the huma output struct.
`map[string]any` has no `additionalProperties` concept to configure — it already accepts
arbitrary JSON, so no huma struct tag is needed to preserve this field's permissiveness.

## OpenAPI spec output (wayfinder ticket 3)

One `humacli` custom Cobra command (or two — see below) added to `cmd/server/main.go`,
wired into `make generate`:

```go
cli.Root().AddCommand(&cobra.Command{
    Use:   "openapi",
    Short: "Dump both surfaces' OpenAPI specs to disk",
    Run: func(cmd *cobra.Command, args []string) {
        writeSpec(apiV1.OpenAPI(), "openapi.yaml")
        writeSpec(apiBFF.OpenAPI(), "bff-openapi.yaml")
    },
})
```

- Both existing files are OpenAPI **3.0.3** (`openapi.yaml:1`, `bff-openapi.yaml:1`) —
  use `.DowngradeYAML()` (not `.YAML()`, which emits 3.1) so the dumped files stay 3.0.3
  and every existing 3.0-only downstream reader (`openapi-typescript` config, any tooling
  pinned to 3.0) needs no changes.
- Same filenames, same repo-root location, same purpose (`GETTING-STARTED.md`'s
  "readable without a build" property) — only the authoring direction reverses (huma
  generates it from code; the file becomes committed output, not committed input).
  `openapi-typescript`'s existing invocation against `bff-openapi.yaml` is unchanged.
- `go run . openapi` never starts the server (confirmed against huma's own
  `client-sdks.md` tutorial pattern) — safe to run in `make generate` alongside the
  existing `sqlc`/`goose` steps.

## File/package layout

- **Deleted**: `internal/api/` (oapi-codegen output for `/api/v1`), `internal/bffapi/`
  (oapi-codegen output for `/api/bff`), and the `oapi-codegen` invocations in `Makefile`'s
  `generate` target.
- **Kept, same paths, rewritten contents**: `openapi.yaml`, `bff-openapi.yaml` (now
  generated, not hand-authored — see above); `internal/transport/publicapi/*_handler.go`
  and `internal/transport/bff/*_handler.go` (rewritten from `<Domain>Server`
  interface-method implementations to `huma.Register(...)` calls with typed
  input/output structs replacing the oapi-codegen-generated request/response types);
  `internal/transport/publicapi/middleware.go` (rewritten per the table above, same
  file, same exported helper names where practical so callers outside this package don't
  need to change).
- **Not touched**: everything under `internal/domain/`, `internal/identity` (below the
  transport seam), `internal/db`, `internal/dbquery`, `db/`, `web/`.

## Testing Decisions

- **No existing test's assertions should need to change — its harness may.**
  Clarified after ticket 6 hit a real contradiction: `internal/transport/publicapi`'s
  test harness (`publicapi_testutil_test.go`'s router-construction, `middleware_test.go`'s
  direct gin-wiring of `RejectActorFields`/`RequireActor`/`handleMe`) is unavoidably
  coupled to the transport framework itself — it has to change when oapi-codegen+Gin's
  wiring shape changes to huma's, no matter how faithfully the migration preserves
  behavior. That's plumbing churn, not behavior drift. **What counts as drift, and
  must NOT change:** any expected status code, JSON body value, error code/message, or
  test scenario/input — the actual claims a test makes about behavior. **What's
  expected to change, and is fine:** how a test builds the router/handler under test
  (construction functions, middleware-mounting calls, type import paths for types that
  move — e.g. `internal/api`'s framework-agnostic response types moving into
  `internal/transport/publicapi` itself alongside the huma structs that now produce
  identical JSON). If a change to a test file touches only this harness layer, it does
  not need to be reported as a blocker. If it touches an assertion's expected value,
  that's the real stop-and-report signal this rule exists for.
- **New, migration-only verification** (not a standing CI addition — huma's spec is
  derived, so there's nothing to keep "in sync" going forward): before deleting the old
  hand-authored `openapi.yaml`/`bff-openapi.yaml`, diff their paths/operationIds/required
  fields against the newly-dumped versions and confirm they match (modulo the one known
  intentional change: generic shape-validation failures move from 400 to huma's native
  422 — see "Behavior note" above). This is a one-time acceptance check for the
  migration ticket(s), not a new automated test.
- **`X-Actor` header rejection and the two `additionalProperties:false`-today request
  schemas** (`openapi.yaml:420,459`, the ones guarding against a stray `done` field) are
  the two places worth a manual smoke check post-migration even though existing tests
  should already cover them — they're exactly the two spots where "huma's default now
  does this for free" could silently mean "nothing exercises this anymore" if an
  existing test happened to test the old middleware directly rather than the resulting
  HTTP behavior.
