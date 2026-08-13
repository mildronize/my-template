# Task task-2 Report

## Task
Author `bff-openapi.yaml`, wire a second `oapi-codegen` invocation into a
distinctly named package (`internal/bffapi`), build the BFF's
session-authenticated JSON handlers (todo CRUD + keys list/revoke) on top
of `internal/domain/todo.Service`/`internal/identity.Service` directly, and
write the first BFF-layer I3 test plus the Done-when-5 negative check
(no key issuance, no rotate, anywhere on this surface).

## Outcome
done

## Decision

- Issue: the task spec says "Requires a valid session (reuse
  `bff.RequireSession`... same pattern)", but `_contract/API.md` is
  explicit that this JSON surface must answer 401 on a missing/expired/
  wrong-role session, never `RequireSession`'s existing redirect-to-
  `/login` (a `fetch` call can't follow a redirect the way a browser
  navigation does).
  Options considered: (a) literally reuse `RequireSession`'s
  `gin.HandlerFunc` and accept that `/api/bff` would redirect instead of
  401 on an auth failure; (b) refactor `RequireSession` to extract a
  shared session-resolution helper both it and a new middleware call,
  differing only in how they respond to failure; (c) write a second,
  self-contained middleware (`RequireJSONSession`, `json_middleware.go`)
  that performs the identical checks (cookie parse, signature/expiry,
  user lookup, I12's role check, active check) but writes a 401 JSON
  envelope instead of redirecting, leaving `RequireSession` and its
  tests completely untouched.
  Chosen: (c). The contract's behavior requirement is unambiguous and
  overrides the task text's looser "reuse" wording — "reuse" is honored
  at the level of checks performed and helpers called (same
  `Signer`/`identity.UserRepo` primitives, same I12 role check, same
  session cookie name), not at the level of one shared `gin.HandlerFunc`
  whose response behavior would then have to differ by caller. (b) was
  the more DRY option but carried real risk of subtly changing
  `RequireSession`'s own tested redirect behavior for no benefit; (c)
  keeps `view_handler_test.go` and every existing `bff` test provably
  unaffected (confirmed: unchanged assertions, all still green) while
  still expressing the shared reasoning as a single pointer comment on
  `RequireSession`'s own doc comment rather than two independently
  drifting explanations.
- Issue (found via manual live-instance verification, not caught by any
  package-level test): `wireBFF`'s embedded-SPA fallback
  (`router.NoRoute`, task-1) is router-wide, not scoped to `/`. An
  unmapped path under `/api/bff/` — including this task's own
  Done-when-5 negative-check target, `POST /api/bff/keys` — fell through
  to the SPA and answered `200 text/html` (the embedded `index.html`)
  instead of a real 404. `internal/transport/bff`'s own test router
  (`bff_testutil_test.go`) never registers a `NoRoute` handler at all, so
  its own `negative_check_test.go` couldn't see this interaction; only
  curling a real running binary surfaced it.
  Fixed in `wireBFF`: the `NoRoute` handler now checks for an
  `/api/bff/` path prefix and answers a proper 404 JSON envelope
  (reusing `publicapi.NewErrorEnvelope`) before falling through to the
  SPA for everything else. Verified against a rebuilt live binary (curl)
  and locked in with `cmd/server/bff_negative_check_test.go`
  (`TestBFF_UnmatchedAPIBFFPathAnswers404NotTheSPA`), which also confirms
  non-API paths (e.g. `/settings`) still correctly fall through to the
  SPA. This edit touches only `wireBFF`'s `NoRoute` registration in
  `main.go` — `internal/transport/bff/view_handler.go` and
  `cmd/server/spa.go` are both untouched.
- Ambiguity: the task spec says to "reuse the exact same
  `{error:{code,message,hint}}` envelope `internal/transport/publicapi`
  already uses... don't redefine it in `bff`", but `publicapi` itself
  already has two Go types producing that identical JSON shape:
  `internal/api.Error` (oapi-codegen-generated, used by
  `todo_handler.go`'s `todoNotFoundError` and the request validator) and
  a hand-rolled `errorEnvelope`/`newErrorEnvelope` (used by
  `middleware.go`'s `unauthorizedBody` and `keys_handler.go`'s
  `notFoundBody`). "The exact same envelope" therefore can't mean one
  literal shared Go type end to end without contradicting `publicapi`'s
  own existing precedent.
  Resolved by exporting the hand-rolled type
  (`errorEnvelope`→`ErrorEnvelope`, `newErrorEnvelope`→
  `NewErrorEnvelope`, `internal/transport/publicapi/middleware.go`) and
  having `bff`'s own hand-written 401/404 bodies import and call it
  directly (`bff` importing `publicapi` — allowed; `ARCHITECTURE.md`'s
  five rules restrict domain/identity/platform imports, not
  inter-transport-surface imports, and no existing rule or test
  forbids it). `internal/bffapi`'s own request validator continues the
  established parallel pattern instead — writing its own generated
  `bffapi.Error` type for validation failures, exactly mirroring how
  `internal/api`'s validator writes `api.Error` rather than the
  hand-rolled envelope. Both paths produce byte-identical JSON shape;
  the split mirrors `publicapi`'s own existing internal split rather than
  inventing a third pattern.
- `RejectActorFields()` (I1) is reused directly from `publicapi` on the
  new `/api/bff` group rather than reimplemented — it was already
  exported, has no `publicapi`-internal state, and the task's own
  Done-when 4/`_contract/API.md` note that I1 is "mechanism-level, not
  surface-level" and still applies here.

## Notes

- **`GET /api/bff/me`'s exact response shape (for task-3)**:
  `{"handle": string, "role": string, "active": bool}` — verified against
  a live instance:
  `{"handle":"manual-verify-owner","role":"owner","active":true}`.
  Byte-for-byte the same field set as `GET /api/v1/me`. Handled by
  `internal/transport/bff/me_handler.go`'s `MeServer`/`handleBFFMe`,
  session-gated by `RequireJSONSession` (401 JSON on no/invalid/wrong-role
  session, never a redirect). `web/src/lib/auth-client.ts`'s replacement
  (task-1's placeholder, per its own report's note) should call this
  endpoint with `credentials: "include"` (the session cookie is
  `HttpOnly`, `SameSite=Lax`) and treat any non-200 as "no session" —
  there's no separate "session expired vs never had one" distinction on
  the wire (I5: "401 never leaks why"), matching the tri-state
  `AuthGate.tsx` already expects (confirmed-logged-out / has-or-had-a-
  session / pending) once wrapped in a query hook.
- Todo/Key JSON shapes on `/api/bff/*` are also byte-identical to their
  `/api/v1/*` counterparts (`Todo`: `id,title,done,createdAt,updatedAt`;
  `ApiKey`: `id,prefix,createdAt,expiresAt`) — `bff-openapi.yaml`'s
  schemas were authored as literal copies of `openapi.yaml`'s, per the
  contract's own "same shape" language throughout.
- I2/I12 boundary presence check (Done-when 4): confirmed both
  `.chief/milestone-3/_contract/API.md` (its own "The I2/I12 boundary —
  why owner writes exist on `bff` and never on `publicapi`" section) and
  `.chief/_rules/_contract/INVARIANTS.md` (I2 and I12's own entries,
  I12 explicitly "the inverse of I2") already state this reasoning —
  nothing missing, no doc edit needed. Condensed pointers to that
  existing prose were added as code comments at `bff.RequireSession`'s
  definition (`internal/transport/bff/middleware.go`) and at the point
  `/api/bff`'s write routes are registered (`wireBFF`,
  `cmd/server/main.go`), per the task spec.
- `internal/bffapi`'s package doc comment and `internal/transport/bff`'s
  own new files each cite `_contract/API.md`/`ARCHITECTURE.md` directly,
  mirroring `internal/api`/`internal/transport/publicapi`'s existing
  comment conventions rather than inventing new ones.
- `identity.Service` is now threaded into `wireBFF` (previously only
  `*identity.Repo`) — the exact same instance `wireIdentity`/
  `wirePublicAPI` already build and use for the public API's `KeysServer`,
  per `ARCHITECTURE.md`'s shared-service-layer rule. No second
  `identity.Service` is constructed anywhere.
- A throwaway `cmd/manualverify` tool (mints a real owner user + signed
  session cookie against a live SQLite file) was used to curl a real
  running binary end to end and is exactly what surfaced the NoRoute/SPA
  interaction above — it was not committed; the regression it caught is
  now covered by `cmd/server/bff_negative_check_test.go` instead.

## Verification

- `go build ./...` — clean.
- `go vet ./...` — clean.
- `gofmt -l .` — no output (clean).
- `go test ./...` — all packages green, including:
  - `internal/transport/bff`'s new
    `TestBFFHandler_FullCRUDRoundTrip_RealSessionCookie` (create→list→
    get→update→delete via `/api/bff/todos`, reading back after every
    write),
  - `TestI3_BFFHandlerOwnershipScoping_ReturnsNotFoundNotForbidden` (the
    first BFF-layer I3 test — both `require.Equal(...,404,...)` and a
    separate `assert.NotEqual(...,403,...)` per case, plus a check that
    the wrong-owner and never-existed responses are byte-identical, plus
    a check that the row itself was never mutated),
  - `TestBFFHandler_ListTodos_Unauthenticated_Returns401NotRedirect` (the
    401-not-redirect behavior change, checked against both the status
    code and the absence of a `Location` header),
  - `internal/transport/bff/keys_handler_test.go`'s list/revoke +
    keys' own `TestI3_..._Keys`,
  - `internal/transport/bff/negative_check_test.go`'s
    `TestNegativeCheck_NoKeyIssuanceOrRotateEndpointOnBFFSurface` (a
    structural check against `bffapi.ServerInterface`'s actual method set
    via reflection, plus a live-router HTTP check that
    `POST /api/bff/keys` and a few rotate-shaped paths never answer 2xx),
  - `cmd/server`'s new `TestBFF_FullCRUDRoundTrip_ThroughAssembledMainHandler`
    and `TestBFF_UnmatchedAPIBFFPathAnswers404NotTheSPA` (the same round
    trip and negative check, run through the real `main.go`-composed
    handler rather than `internal/transport/bff`'s isolated test router
    — this is what the SPA/NoRoute fix above is regression-guarded by),
  - every pre-existing test in `internal/transport/bff` and
    `internal/transport/publicapi`, unaffected (`view_handler_test.go`,
    `login_handler_test.go`, `callback_handler_test.go` only needed a
    mechanical extra `nil` argument for `newTestRouter`'s new
    `identitySvc` parameter — `view_handler.go` itself has a clean,
    zero-line `git diff`),
  - `internal`'s `TestDoneWhen12_EveryInvariantHasANamedTest` and every
    `TestArchitecture_*` check (`internal/architecture_test.go`) — the
    new `bff`→`publicapi` import (for the shared error envelope) doesn't
    violate any of `ARCHITECTURE.md`'s five rules, confirmed by these
    tests staying green, not just by inspection.
- `make generate` — ran cleanly with both spec files. Verified twice: once
  normally, once after deleting both `internal/api/openapi.gen.go` and
  `internal/bffapi/bffapi.gen.go` and regenerating from scratch —
  `internal/api/openapi.gen.go` came back byte-identical (`git diff`
  empty), `internal/bffapi/bffapi.gen.go` regenerated cleanly, both
  packages (`internal/api`, `internal/bffapi`) compile together with no
  symbol collision.
- **Real round trip against a live instance** (not just `httptest`): built
  and ran the actual server binary (`go run ./cmd/server`) against a real
  SQLite file, minted a real signed session cookie via
  `bff.Signer.NewSessionCookie` (the same session-seeding helper the
  tests reuse) for a real `role=owner` user row, then `curl`'d
  `GET /api/bff/me` → `{"handle":"manual-verify-owner","role":"owner","active":true}`,
  `POST /api/bff/todos` → `201` with the created todo, `GET
  /api/bff/todos` → `200` listing exactly that one todo (read back, not
  just trusting the create response), `GET /api/bff/todos/{id}` → `200`,
  `PATCH .../{id}` (`{"done":true}`) → `200` with `done:true` and `title`
  unchanged, `GET .../{id}` again → confirms the patch persisted,
  `DELETE .../{id}` → `204`, `GET .../{id}` again → `404` (confirms the
  delete actually removed the row). Also confirmed unauthenticated
  `GET /api/bff/todos` → `401` (not a redirect), and — after the NoRoute
  fix — `POST /api/bff/keys` → `404` JSON (was `200 text/html` before the
  fix, the interaction documented in Decision above) while
  `GET /settings` (a non-API path) still correctly serves the SPA (`200`).
- `docker compose`/`make build` (which require `npm`) were not re-run —
  out of this task's scope (`web/` untouched, no Dockerfile/compose
  change), already verified by task-1 and scheduled again at task-5's
  full-suite gate.

## Commits pushed (branch `milestone-2/close-parity-gap`)

- `a3f0d6a` — `feat(milestone-3/task-2): author bff-openapi.yaml, wire second oapi-codegen invocation into internal/bffapi`
- `8ec2d99` — `feat(milestone-3/task-2): export publicapi's error envelope for bff to reuse directly`
- `456e6c2` — `feat(milestone-3/task-2): implement BFF JSON handlers, wire /api/bff into main.go`
- `60beba0` — `feat(milestone-3/task-2): add BFF JSON surface tests -- real round trip, first BFF-layer I3 test, negative check`
