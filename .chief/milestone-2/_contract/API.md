# API contract — milestone-2 additions

Extends `.chief/_rules/_contract/API.md`'s conventions (promoted from
milestone-1, unchanged this milestone) with a second surface. Two things
share one service layer now, not one thing:

| Surface | Package | Consumer | Auth |
| --- | --- | --- | --- |
| Public API | `internal/transport/publicapi` | agents/skills | `Authorization: Bearer <credential>` (I1–I2, I5–I10, I13–I14) |
| BFF | `internal/transport/bff` | มายด์, in a browser | session cookie (I1, I5, I11–I12) |

## Public API — unchanged this milestone

Every endpoint from milestone-1 (`GET /api/v1/me`, the `/todos` CRUD,
`GET /api/v1/keys`, `DELETE /api/v1/keys/:id`) is unchanged — only its code
location moves (`internal/todo` → `internal/domain/todo`,
`internal/identity`'s handler → `internal/transport/publicapi`). **There is
still no `POST /api/v1/keys` and no key-rotation HTTP endpoint** — `rotate`
stays CLI-only, same reasoning as `issue`: a rotated key is printed to
stdout exactly once (I8), which an HTTP response can't do safely (it would
sit in access logs, browser history, a proxy's request log). Rotation is
an operator action against the host, not a self-service API call. **Do not
add a `POST /api/v1/keys/:id/rotate` for symmetry with `GET`/`DELETE`** —
`rotate` *issues* a key exactly like `issue` does, so putting it on HTTP
would breach the "issuance is script-only" line from the other side; list
and revoke don't mint new secrets, which is why they're safe as endpoints
and rotation isn't.

## BFF — new this milestone

No `openapi.yaml` entries — this surface serves HTML, not a JSON API, and
isn't meant for programmatic consumption (that's what `publicapi` is for).

### `GET /login`

Redirects to the SSO issuer's authorization endpoint. Generates a PKCE
`code_verifier` (I11 — `oauth2.GenerateVerifier()`), stores it against a
short-lived, HTTP-only state cookie (not the session cookie — this one
only survives the redirect round-trip), and includes the corresponding
`code_challenge` (`oauth2.S256ChallengeOption`) in the auth URL. No
username/password form here — SSO owns that, per `sso-consumer-contract.md`
§2.

### `GET /callback`

Exchanges the authorization code for a token (`oauth2.VerifierOption`,
reading the verifier back from the state cookie set by `/login` — I11).
Resolves the returned identity's `sub` against `users.sso_subject`
(**no JIT** — see `_rules/_contract/DATA_MODEL.md`'s "Owner provisioning"
note; an unrecognized `sub` here is an error page, not a new user row).
On success: sets the session cookie (signed, short-lived, carries the
resolved `users.id`), clears the state cookie, redirects to the
authenticated view. On failure (unrecognized `sub`, expired state,
token-exchange error): a plain error page, no session cookie set.

### `GET /` (authenticated view)

Session-cookie-gated (I1, I12 — a session resolving to `role='agent'` is
rejected the same as a missing one; see Conventions below). Read-only-or-
minimal view of the caller's own todos — enough for มายด์ to visually
confirm the domain works end to end, not a management UI. Calls the same
`todo` domain service the public API's handlers call (`_rules/_standard/
ARCHITECTURE.md`'s shared-service-layer rule) — no BFF-specific todo logic
exists anywhere.

## Conventions specific to `bff`

- No session cookie, an expired one, or one resolving to `role='agent'` →
  redirect to `/login`, not a JSON 401 (I5's "don't leak why" still
  applies — the redirect doesn't distinguish "no cookie" from "bad
  session" from "wrong role" any more than `publicapi`'s 401 body would).
- Still I1: nothing about actor identity is ever read from a request field
  on this surface either, even though it's not exposing a body callers
  would think to put one in — the invariant is about the mechanism, not
  about which surface makes the mistake easy.
- CSRF: the state-cookie-carries-the-PKCE-verifier design already ties
  `/callback` to a `/login` this same browser initiated; no separate CSRF
  token scheme is added on top for a single-page, single-action flow.

## Error shape — unchanged for `publicapi`, N/A for `bff`

`publicapi`'s `{error:{code,message,hint}}` envelope (`_rules/_contract/
API.md`) is unchanged. `bff` returns HTML (redirects or a plain error
page), not this JSON shape — it's a browser surface, not an API.
