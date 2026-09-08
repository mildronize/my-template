# 4: How do this repo's actor/ownership invariants map onto huma's request model?

Type: wayfinder:research
Status: resolved
Blocked by: None (can start immediately)

## Question

`.chief/_rules/_contract` documents invariants enforced today via oapi-codegen's
generated handler interfaces plus hand-written middleware/checks — at minimum:

- I1: no actor fields in request bodies/headers (actor resolved server-side)
- I3: unowned rows return 404, never 403
- I4: only actor-resolution code touches `users`/`api_keys` repos
- the static table-ownership checks in `internal/dbquery` (per `PROJECT_SUMMARY.md`'s
  "Known Gotchas" #5: "read sqlc sources, not runtime behavior")

`docs/research/frontend-lightweight-alternatives.md` §3 already flags huma's `Resolver`
as "a proper home" for cross-field/business-rule validation (citing I18-style transition
checks) and confirms `additionalProperties: false`-style strict validation is real and
enforced at request time (`huma/validate.go:841,955`) — matching I1. That research was
exploratory, not a full mapping.

Resolve by reading huma source (`~/ref-idea/docs/huma`) and this repo's actual
invariant-check code/tests, and answering concretely: does each invariant above have a
clear huma-native equivalent (Resolver, middleware, strict validation), or does any of
them need a bespoke mechanism that doesn't exist in oapi-codegen either? This needs an
answer before `/chief-plan` can write a contract that states how invariants are
preserved post-migration.

## Answer

*Resolved by research subagent, 2026-09-06.* huma cloned at `~/ref-idea/docs/huma`,
commit `ad801e09f5cad8f531ea8d17f31f1649ec4355bd` (`v2.39.1-4-gad801e0`, 2026-08-24).
This repo's current mechanisms read from `main.v2` as checked out.

### I1 — no actor fields in request bodies/headers

Current mechanism (`internal/transport/publicapi/middleware.go:75-193`,
`RejectActorFields`): one bespoke Gin middleware, run before actor resolution, that
hand-parses the JSON body into `map[string]json.RawMessage`, walks query params, and
checks the `X-Actor` header — explicitly **not** relying on oapi-codegen/
`additionalProperties: false`, because (middleware.go:130-134) that property "has to be
remembered on every request schema."

Huma verdict: **body and query are now a native, zero-code fit; header stays bespoke,
but smaller than today.**

- **Body** — huma schemas generated from Go structs default to
  `additionalProperties: false` (`registry.go:216`, `AllowAdditionalPropertiesByDefault:
  false`; wired into struct-schema generation at `schema.go:973-985`), enforced at
  request-validation time in `validate.go:841-859` (`map[string]any` case) and
  `validate.go:955-970` (`map[any]any` case). **Both citations in
  `docs/research/frontend-lightweight-alternatives.md` §3 — `validate.go:841,955` —
  verified accurate against this exact clone, unchanged.** An `actorId`/`ownerId` body
  field is rejected automatically (`validation.MsgUnexpectedProperty`) with no
  per-handler code, as long as no input struct opts back into permissiveness via the
  `additionalProperties:"true"` struct-tag escape hatch
  (`docs/docs/features/request-validation.md:200-214`). This *inverts* today's failure
  mode: oapi-codegen requires each schema to remember to declare strictness; huma
  requires each schema to remember to declare an exception. Strictly better fit.
- **Query** — native via `huma.Config.RejectUnknownQueryParameters` (global) or
  `Operation.RejectUnknownQueryParameters` (per-op) — `api.go:248-250`, consumed at
  `huma.go:907`, default `false` in `defaults.go:80`
  (`docs/docs/features/request-validation.md:216-237`: unknown query param → `422`).
  Set once, globally, in `huma.Config` and it applies to every operation — this closes
  exactly the "has to be remembered per schema" gap middleware.go's own comment cites as
  the reason today's check is hand-rolled rather than schema-driven.
- **Header** — no native equivalent: huma only validates headers explicitly bound to an
  input-struct field, so an unbound `X-Actor` header is silently ignored by the
  framework, same gap oapi-codegen has today. Still needs one hand-written check, but
  huma gives it an intended seam: a single global router-agnostic middleware
  (`api.UseMiddleware`, `huma.Middlewares`, `docs/docs/features/middleware.md:52-76`)
  checking `ctx.Header("X-Actor")` before calling `next`. Confirmed this runs *before*
  body parsing/validation/resolvers: `huma.Register`'s wiring
  (`huma.go:881`) is `a.Handle(&op, api.Middlewares().Handler(op.Middlewares.Handler(func(ctx
  Context) {...})))` — global middleware wraps the entire parse→validate→resolve→handle
  chain, preserving today's "fail on shape before spending a DB lookup" ordering intent.
- **Wire format** — the `{error:{code,message,hint}}` envelope (`_contract/API.md`)
  survives: `huma.NewError` is an overridable package-level `var`
  (`error.go:245`, documented `docs/docs/features/response-errors.md:132`) that reshapes
  every error body huma writes; `actor_field_present` can keep its exact code/shape.

Net for I1: goes from "one large bespoke middleware doing 3 checks" to "2 checks become
config, 1 stays a small bespoke middleware." Improvement, not parity.

### I3 — unowned rows return 404, never 403 (absence, not permission)

Current mechanism is **not** a transport-layer concern at all: ownership scoping is a
SQL `WHERE` clause plus a typed sentinel error, mapped to a status code in the handler —
e.g. `internal/transport/publicapi/todo_handler.go:183-186`
(`errors.Is(err, todo.ErrNotFound)` → `http.StatusNotFound`, `todoNotFoundError`) and
`internal/transport/publicapi/keys_handler.go:82-90` (identity domain, where I3 still
applies per `INVARIANTS.md`'s milestone-4 scope note). Tests:
`internal/identity/repo_test.go:197` (`TestI3_RevokeAPIKeyScopedToOwner_
AbsenceNotPermission`), `internal/transport/publicapi/keys_handler_test.go:107`
(`TestI3_HandlerOwnershipScoping_ReturnsNotFoundNotForbidden_Keys`); the todo domain's
own `TestI3_...` (`internal/domain/todo/repo_test.go:66`) now asserts I3 no longer
applies there (milestone-4 scope narrowing, unrelated to this migration).

Huma verdict: **no native mechanism applies, and none is needed — huma neither helps nor
hurts.** This pattern (sentinel error → `errors.Is` → status code, decided one layer
below any web framework) is framework-agnostic and ports verbatim: swap
`c.AbortWithStatusJSON(http.StatusNotFound, todoNotFoundError)` for
`return nil, huma.Error404NotFound("no such todo")` (`huma.Error404NotFound`,
`error.go:315`, implements `StatusError`) inside the operation handler. Identical
property under Gin+oapi-codegen, huma, or a bare `net/http` mux — this invariant lives
entirely in `internal/domain/*`/`internal/identity`, not in the transport glue this
migration touches.

### I4 — only actor-resolution code touches `users`/`api_keys` repos (+ dbquery static checks)

I4 is enforced by two independent, purely static, build/test-time mechanisms, **neither
of which touches HTTP routing, request schemas, or the web framework at all**:

1. `internal/architecture_test.go:215-243`
   (`TestArchitecture_OnlyRepoFilesImportSqlc`) — a Go-AST import-graph check: only
   `repo.go`/`*_repo.go` files may import the generated `sqlc` package, across every
   domain module and `internal/identity`.
2. `internal/dbquery/tableisolation.go` — `TableOwnership` (:60-65),
   `ReadOnlyGrant`/`ReadOnlyGrants` (:74-98), and
   `AssertQueryFileReferencesOnlyOwnTable` (:214-277) — a regex-based static scan of
   `db/queries/*.sql` (sqlc's own source files), verified by
   `internal/dbquery/tableisolation_test.go`. This is exactly the mechanism
   `PROJECT_SUMMARY.md`'s "Known Gotchas" #5 warns about ("Table isolation checks are
   static — they read sqlc sources, not runtime behavior") — read in full; confirmed it
   is a source-text scan with an explicit, hand-maintained ownership map and
   mechanically-enforced read-only grants (a grant does not permit `INTO`/`UPDATE`
   against the granted table, checked directly against SQL text), not a runtime guard.

Huma verdict: **fully orthogonal — huma neither helps nor hurts here, and nothing about
this migration should touch these two mechanisms.** Both operate purely on Go source
files and `.sql` query files; they would be byte-for-byte identical under Gin+oapi-
codegen, huma, or any other Go web framework, because they never look at
`openapi.yaml`, generated handler interfaces, request/response types, or routing at all.
This is also the direct answer to the ticket's fourth bullet (`PROJECT_SUMMARY.md`
gotcha #5): the "static, not runtime" caveat is unrelated to oapi-codegen vs huma and
does not change post-migration.

### Anything huma makes worse?

No — across I1/I3/I4, huma is either a net improvement (I1: 2 of 3 checks become
zero-code config) or a neutral pass-through (I3, I4: mechanisms live below the
transport layer huma replaces). One nuance worth flagging for `/chief-plan`, not a
regression: huma's `RejectUnknownQueryParameters` and the `additionalProperties`
default are both *schema/config*-level, not *per-request-runtime-decision*-level like
today's `RejectActorFields` — so a fork that adds a *new* endpoint needing to allow an
arbitrary extra query param (there is no such case in this repo today) would need an
explicit per-operation opt-out (`Operation.RejectUnknownQueryParameters: false` /
`SkipValidateParams`) rather than the current design's uniform, always-on check. Not a
weakening of I1 as it stands, just a design-shape difference to note in the contract if
`/chief-plan` writes one.

### `_rules/_contract/API.md`/`INVARIANTS.md` rewrite question

Narrow answer, not a full rewrite: `INVARIANTS.md`'s I1/I3/I4 prose (`INVARIANTS.md:44-
49, 57-75, 77-100`) is already implementation-agnostic (states the property, not the
mechanism — never names oapi-codegen, Gin, or a specific file) and carries over
unchanged. `API.md` has exactly one sentence that names a mechanism and would need
rewriting on migration: "A request that fails OpenAPI spec validation ... is rejected by
`gin-middleware` before reaching handler code at all" (`API.md`, Error shape section) —
under huma there is no separate `gin-middleware` package; validation happens inside
`huma.Register`'s generated wrapper itself. That single sentence (and any future
contract text describing *how* I1 is enforced) should name huma's mechanism instead once
the migration lands — everything else in both files is unaffected.
