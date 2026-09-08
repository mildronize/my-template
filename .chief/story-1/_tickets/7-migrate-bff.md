# 7: Migrate `/api/bff` (BFF) fully onto huma

Type: implementation
Status: resolved
Blocked by: 5

Independent of ticket 6 — both depend only on ticket 5's foundation, not on each other.
Can be built in either order, or in parallel by different sessions.

## What to build

Same treatment as ticket 6, mirrored onto the BFF surface's handlers. **Reuse
`internal/transport/humamw` (ticket 5's foundation, since extended by ticket 6's fix —
error envelope, `RejectActorFields`, `RequireSession`, `ActorFromContext`) — do not
re-port it.** `RequireSession` still needs attaching to `apiBFFH` via
`apiBFFH.UseMiddleware(...)` (not yet wired — see below), and wire `make generate`'s
`bff-openapi.yaml` step to `go run ./cmd/server openapi`'s BFF half once this ticket
removes the dependency on the hand-authored file. Ticket 6 already landed (commits
`cb1723f`, `b141e19`) — read `.chief/story-1/_report/ticket-6-report.md` and mirror its
`main.go`/`Makefile` pattern rather than inventing a second one.

**Important, already handled — do not redo or second-guess this:** ticket 6 hit and
fixed a real I1 ordering regression (body/query actor-field rejection was moving from
before-auth to after-auth because it relied solely on huma's `additionalProperties`
default, which runs *inside* the chain every middleware wraps). The fix —
`humamw.RejectActorFields()`, checking header+query+body pre-auth — is **already
attached to `apiBFFH`** in `cmd/server/main.go`'s `newHumaAPIs()` (both surfaces got it
in the same commit). You do not need to re-solve this for BFF; just don't attach a
second, redundant pre-auth actor-field check, and do verify (via a test) that BFF's I1
behavior is in fact still correct given this shared middleware, same as ticket 6's
retry did for `/api/v1`.

- Rewrite `internal/transport/bff/{me,keys,users,todo}_handler.go` as `huma.Register`
  operations on `apiBFF` (ticket 5). Explicit `json:"..."` tags matching
  `bff-openapi.yaml` exactly.
- Check `bff-openapi.yaml`'s request schemas for the same `additionalProperties`
  gap ticket 6 found in `CreateTodoEventRequest` — don't assume symmetry, verify
  directly (contract's note: "this needs verifying against `bff-openapi.yaml` directly,
  not assumed from the public-API finding").
- Default statuses matching today's spec (check each operation's actual response code
  in `bff-openapi.yaml`, don't assume 200/201 parity with the public API surface).
- `internal/transport/bff/{callback,login}_handler.go`, `oauth.go`, `session.go` are
  **not** OpenAPI-described operations (they're the OAuth redirect/cookie flow, not
  JSON API) — stay as plain Gin routes, untouched by this ticket. Only
  `json_middleware.go`'s and `middleware.go`'s pieces that feed the *JSON* surface's
  actor resolution are in scope (the session-cookie-to-actor part ticket 5 already
  ported the shape of).
- Remove `router.Group("/api/bff")`'s `bffapi.RegisterHandlers` call, the
  `internal/bffapi` package, and `bff-openapi.yaml`'s status as hand-authored — dumped
  output from here on.
- Update `make generate` to stop invoking `oapi-codegen` for `bff-openapi.yaml`.

## Acceptance Criteria

- [ ] Every existing `internal/transport/bff/*_test.go` passes **unmodified**.
- [ ] `make smoke` passes. **Not self-contained** — start `go run ./cmd/server &`
      first, wait for it to be ready, run `make smoke`, then stop the server (same gap
      ticket 6 documents; don't let a skipped-because-server-wasn't-up run count as a
      pass — this is exactly what happened silently for a full milestone before,
      per `cmd/smoke`'s own doc comment).
- [ ] `make e2e` passes — self-contained (brings up its own local Hydra via Docker, per
      `e2e/README.md`), no manual server start needed. Real Hydra, real browser, login
      flow through to a BFF JSON call — the one check proving the untouched
      login/callback/session path still feeds correctly into the now-migrated JSON
      handlers.
- [ ] `go run . openapi` (BFF half) dumped and diffed against the pre-migration
      `bff-openapi.yaml`: matches, modulo the same accepted 422 delta ticket 6 found,
      plus whatever `additionalProperties` findings this ticket's own check surfaces.
- [ ] `internal/bffapi/` no longer exists; `oapi-codegen`'s BFF invocation is gone from
      `Makefile`.
