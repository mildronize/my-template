# Task 4 Report

## Task
Owner login (BFF): `authorization_code`+PKCE per `task-4.md`'s spec,
callback resolution with no JIT, HMAC-signed session cookie (no store),
minimal authenticated todo view, middleware wired into a new `bff`
engine alongside `publicapi`. Closes I11, I12, and Done-when 9 (the
scoped-rendering criterion Clara added after finding task-4's original
Done-when set was all auth mechanics an empty page would satisfy).

## Outcome
done

## Decision

- **Issue:** id-token audience — an OIDC id_token's `aud` is the OAuth
  client_id, not the API-audience convention `publicapi`'s Bearer
  verifier uses (`AUTH_AUDIENCE`).
- **Chosen:** reused `identity.NewJWTVerifier` (already I6/I7-hardened)
  but audienced to `SSO_CLIENT_ID`, a separate config value from
  `AUTH_AUDIENCE`. Resolved by reading `scripts/register.sh` (task-2's
  own artifact) and confirming `scope=openid` is registered — id_token
  parsing is the intended path per that registration, not a guess.

## Notes

- **Independently re-verified (chief-agent/Luna) — this was the densest
  task, verified accordingly, not lighter than the others:**
  - `go test ./...`: exactly `TestI13_`/`TestI14_` red (task-5's territory
    only — I11/I12 both green now), `invariants_test.go` untouched.
  - Read `TestDoneWhen3_MiddlewareWiredIntoBothEngines` directly, ran it
    standalone — genuinely hits an unmatched route on each engine and
    asserts `X-Request-ID`, not an assumption from "I called
    `platform.NewRouter` twice."
  - Read `TestAuthenticatedViewRendersOwnersOwnTodos` directly, ran it
    standalone — signs a session cookie directly, asserts the seeded
    todo's exact title text (including non-ASCII, incidentally) appears
    in the response body. Its negative twin,
    `TestAuthenticatedViewOnlyShowsOwnersOwnTodos`, confirms actual
    per-owner scoping, not just that *a* todo renders.
  - Ran all four `TestI11_`/`TestI12_` tests individually — all pass,
    confirmed I12 tested at both the callback and view-middleware layers
    as the spec required.
  - Grepped for live Hydra hostnames in the bff test files — none found.
  - **Manual smoke test** (`go run ./cmd/server`, no docker): `/` with no
    session correctly 302s to `/login`; `/healthz` 200; server logs a
    clear operator warning when SSO isn't configured yet, pointing at
    `scripts/register.sh`/`GETTING-STARTED.md` Step 1 — confirms the
    unconfigured-state UX task-2's work anticipated actually connects to
    this task's code.
- Commit: `352fc75` on `milestone-2/close-parity-gap`.

No blockers, no escalation.
