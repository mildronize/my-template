# task-4 spec: Owner login (BFF)

Written for the same reason milestone-1 wrote task-2 a spec: no Go
reference exists on this fleet for `authorization_code`+PKCE against
Hydra, several judgment calls were already made during planning and must
be implemented precisely (not re-derived, which risks re-losing them the
way a fresh rewrite of `~/.my-task/bin/key` would), and this task lands
two new invariants (I11, I12) in one component. Read `_goal/GOAL.md`'s
Decisions table ("Owner login shape") and `_contract/API.md`'s BFF section
first — this spec fills in what those don't already say.

## Files

`internal/transport/bff/`: `login_handler.go`, `callback_handler.go`,
`view_handler.go` (or similar — the `/` authenticated view), `session.go`
(cookie signing/verification, not a domain concern), plus tests. Per
`_rules/_standard/ARCHITECTURE.md`: this package may import gin (it's
transport), may import `internal/domain/todo` and `internal/identity`
(both are what it composes), must never be imported by either of those.

## Login flow — exact sequence

1. `GET /login`: build an `oauth2.Config` from `SSO_ISSUER`/
   `AUTH_AUDIENCE`/client credentials (config, same shape as `platform/
   config.go`'s existing SSO fields). Call `oauth2.GenerateVerifier()` —
   fresh per request, never reused across logins. Set a short-lived
   (5-minute), `HttpOnly`, `Secure`, `SameSite=Lax` cookie (call it the
   *state cookie* — distinct from the session cookie `/callback` sets on
   success) carrying the verifier, signed the same way the session cookie
   is (one signing helper in `session.go`, two uses). Redirect to
   `cfg.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))` — `state`
   itself should be a random value too, checked back at `/callback`
   (standard CSRF-for-OAuth practice, independent of PKCE).

2. `GET /callback`: read and validate `state` against what `/login` set.
   Read the verifier back out of the state cookie. Exchange:
   `cfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))`. Parse the
   returned ID token's `sub` claim (or call the userinfo endpoint,
   whichever `sso-consumer-contract.md` §2's flow actually returns first —
   check the contract, don't assume). Look up `users` by `sso_subject ==
   sub` — **no match is an error page, never a new row** (I10's "no JIT"
   sibling for the human side, see `_rules/_contract/DATA_MODEL.md`'s
   "Owner provisioning" note). Match found but `role != 'owner'` (i.e. it
   resolved to an agent row) → error page, not a session (I12 — this is
   the invariant this exact check exists to satisfy; write
   `TestI12_BFFSessionNeverResolvesToAgent` against this code path
   directly, not just against the view handler's middleware). Match found
   and `role == 'owner'` and `active` → set the session cookie (signed,
   carries `users.id`, reasonable TTL — an hour is fine, this isn't a
   banking app), clear the state cookie, redirect to `/`.

3. `GET /` (and any other authenticated view route): middleware reads the
   session cookie, verifies its signature, resolves the `users.id` inside
   it. Missing/invalid/expired signature, or resolves to `role='agent'`
   (defense in depth — shouldn't be reachable given step 2's check, but
   I12 is tested at this layer too, not only at the callback) → redirect
   to `/login`, not a 401 (see `_contract/API.md`'s BFF conventions —
   this surface returns HTML, not a JSON error body).

## Session cookie mechanics — no server-side store

Per `_rules/_contract/DATA_MODEL.md`'s "BFF session" note: no `sessions`
table. The cookie itself carries `{userID, expiresAt}`, HMAC-signed with a
key from config (`SESSION_SECRET` or similar — add to `platform/config.go`
alongside the existing SSO fields). Verifying the cookie is: check the
signature, check `expiresAt` hasn't passed, done — no database round-trip
to validate a session exists (there's nothing to look up; the signature
*is* the validity proof). This is intentionally the simplest mechanism
that satisfies "minimum authenticated view," not a JWT library dependency
of its own — `crypto/hmac` + `encoding/json` is enough, no new
dependency needed for this specific piece.

## The minimal view itself

Read-only (or close to it — toggling `done` on an existing todo is
reasonable if trivial to add, creating new todos through this surface is
not required) list of the caller's own todos, calling
`internal/domain/todo`'s existing service — the same one `publicapi`
calls. No new todo-domain logic here; if you find yourself writing
todo-specific logic inside `internal/transport/bff`, that logic belongs in
`internal/domain/todo/service.go` instead (`ARCHITECTURE.md`'s
shared-service-layer rule). Server-rendered (`html/template` from the
standard library is enough — no new templating dependency), no JS
framework, no client-side routing. This exists so มายด์ can look at it and
confirm the identity/domain seam actually works, not to be a usable todo
app.

## Test naming — closes I11 and I12

- `TestI11_...`: prove the login flow cannot reach `AuthCodeURL` or
  `Exchange` without a verifier/challenge pair present — e.g. a test that
  calls the handler with the state cookie missing/tampered and confirms
  the exchange is never attempted, or a test asserting the constructed
  auth URL always contains `code_challenge`/`code_challenge_method=S256`.
- `TestI12_...`: prove a `bff` session resolving to `role='agent'` is
  rejected identically to a missing session (redirect to `/login`), at
  both the callback (a `sub` matching an agent row) and the
  view-middleware layer (a forged/tampered cookie somehow carrying an
  agent's `users.id` — defense in depth, test it directly rather than
  assuming step 2 makes it unreachable).

## Verification

`go build ./...`, `go vet ./...`, `go test ./...` (this task's own tests —
full-suite-green is task-5's gate, not this one's, per `_plan/_todo.md`'s
note on the red tree), `gofmt -l .`. No live Hydra call in any automated
test — mock the OIDC endpoints (`httptest.Server` serving fake authorize/
token/userinfo responses), same reasoning as `internal/identity/jwt_test.go`
already uses a fake JWKS server rather than a live issuer.

## Do NOT do in this task

Don't add a session store/table. Don't build a UI framework, client-side
JS, or anything beyond one server-rendered authenticated page. Don't touch
`internal/domain/todo`'s actual domain logic beyond calling its existing
service methods. Don't implement `rotate` or the key-file/resolver
mechanism — that's task-5's.
