# 6: Migrate `/api/v1` (public API) fully onto huma

Type: implementation
Status: resolved
Blocked by: 5

## What to build

**Ticket 5 already built and tested the foundation this ticket wires up** —
`internal/transport/humamw` has the error-envelope override, `RejectActorHeader`
(X-Actor), and `RequireActor`/`RequireSession` (actor resolution, `ActorFromContext`),
plus the two `huma.API` instances (`apiV1H`, `apiBFF`) already created in
`cmd/server/main.go` (currently with zero operations registered). **Reuse these — do
not re-port or duplicate them.** This ticket's job is: register real operations on
`apiV1H`, attach `humamw.RequireActor` + the X-Actor middleware to it via
`apiV1H.UseMiddleware(...)` (ticket 5 built the pieces but didn't attach them, since no
operation existed yet to protect), and wire `make generate`'s `openapi.yaml` step to
the new `go run ./cmd/server openapi` dump (ticket 5 deliberately left this unwired,
since dumping earlier would have overwritten the hand-authored file oapi-codegen still
needed — safe now that this ticket removes that dependency).

`internal/api.RegisterHandlers` registers every public-API route in **one call**
(`cmd/server/main.go`) — there's no clean way to move this surface's routes to huma one
at a time without regenerating a shrunk oapi-codegen interface mid-flight (extra,
unwanted complexity per `AGENTS.md`'s Simplicity First). This ticket moves the whole
surface in one atomic step instead.

- Rewrite `internal/transport/publicapi/{me,keys,todo}_handler.go` as `huma.Register`
  operations on `apiV1` (ticket 5), with typed Input/Output structs replacing the
  oapi-codegen-generated request/response types. Every field carries an explicit
  `json:"..."` tag matching today's spec exactly (contract's "Schema fidelity"
  concern) — casing, nullability (pointer types), `enum`/`minLength`/`maxLength` tags
  matching `openapi.yaml`'s constraints.
- `POST /todos` and `POST /todos/{id}/events` get `DefaultStatus: 201` (huma's default
  is 200; today's spec is 201 — `openapi.yaml:71,190`).
- `CreateTodoEventRequest` gets the `additionalProperties:"true"` dummy-field tag
  (contract's "`additionalProperties` exceptions" section) — the one schema that's
  permissive today and must stay that way.
- `payload` (`TodoEvent`'s free-form field) becomes `map[string]any`.
- Domain-layer errors map via `huma.Error404NotFound`/`huma.Error403Forbidden` etc.,
  same codes/bodies as today (`invalid_transition` for I18, `not_found` for I3).
- Remove `router.Group("/api/v1")` + its `RejectActorFields()`/`RequireActor()`
  middleware registration, the `internal/api` package (oapi-codegen output + the
  `apiServer` composition struct + `validator.go`), and `openapi.yaml`'s status as a
  hand-authored file — it's dumped output from here on.
- Update `make generate` to stop invoking `oapi-codegen` for `openapi.yaml`.

## Acceptance Criteria

- [ ] Every existing `internal/transport/publicapi/*_test.go` passes **unmodified** —
      if a test needs its own content changed to pass, that's a behavior drift; stop
      and report it rather than editing the test (contract's Testing Decisions).
- [ ] `make smoke` passes. **`make smoke` is not self-contained** — `cmd/smoke`'s own
      doc comment says so explicitly ("start a live instance first — not this script's
      job"). Start `go run ./cmd/server &` first, wait for it to be ready, run
      `make smoke`, then stop the server. (This exact gap bit milestone-4: its own
      comment records that `make smoke` went unrun for the entire milestone because
      nothing wired it into the automatic gate — don't repeat that here by treating a
      skipped-because-server-wasn't-up smoke run as a pass.)
- [ ] `go run . openapi > openapi.yaml`, then diff the result against the pre-migration
      version's paths/operationIds/required fields: matches, modulo the one accepted
      delta (generic shape-validation failures now 422, not 400 — `actor_field_present`
      unaffected).
- [ ] `internal/api/` no longer exists; `oapi-codegen`'s public-API invocation is gone
      from `Makefile`.
