# task-2 spec: Identity

Written because `loop-readiness` (run 2026-08-12) singled this task out as
the one needing a spec, not the other four: it's the only task translating a
reference implementation across languages (my-task's `resolveActor()`,
TypeScript) while satisfying 7 of `INVARIANTS.md`'s 10 invariants in one
component, with a library decision (`_goal/GOAL.md` Decisions: JWT/JWKS row)
that didn't exist until this review round. Read `_contract/DATA_MODEL.md`
(`users`, `api_keys`) and `_contract/INVARIANTS.md` (I1, I2, I5–I10) first —
this spec doesn't repeat their content, only the parts that need judgment
calls made explicit.

## Files

`internal/identity/`: `handler.go`, `middleware.go`, `service.go`, `repo.go`,
plus tests. Per `_rules/_standard/ARCHITECTURE.md`: only `handler.go` (or
`*_handler.go`) imports `gin`; only `repo.go` (or `*_repo.go`) imports the
sqlc-generated package.

## Actor-resolution middleware — exact order

Mirrors my-task's `resolveActor()` (`~/gits/my-task/src/server/lib/
resolve-actor.ts`), minus the session-cookie branch (this service has none):

1. Read `Authorization` header. Missing or not `Bearer ` → unauthorized (no
   further checks needed).
2. **Try as an API key first.** Hash the token (SHA-256, matching
   `DATA_MODEL.md`), look up `api_keys` by `key_hash`. Found, not expired,
   not revoked → resolve to that row's `user_id`. Found but expired/revoked
   → unauthorized (I9) — do **not** fall through to the JWT branch; a token
   that matched an `api_keys` row is an API key, full stop, and a key that's
   expired isn't suddenly worth trying as a JWT.
3. **Only if no `api_keys` row matched at all**, try as a JWT via
   `jwx/v3`: parse and verify against `SSO_ISSUER`'s JWKS (cached, RS256
   pinned per I6/I7), validate `iss`/`aud`/`exp`. **A JWT validation failure
   here is the routine case, not an error** — most Bearer tokens presented
   to this service will be API keys, so log at debug, not error (my-task's
   `resolveActor` comment says this explicitly; do the same here so the
   logs don't cry wolf on every ordinary API-key request). A validation
   *success* looks up `users` by `sso_subject = claims.sub` (I10 — no match
   → unauthorized, never auto-created).
4. Either branch resolving to a `users` row with `role='owner'` →
   unauthorized (I2) — checked identically for both branches, not shared
   logic that happens to cover both, because a future edit to one branch
   must not silently stop covering the other.
5. Either branch resolving to `active=false` → unauthorized.
6. No branch resolved anything → unauthorized.

Every unauthorized exit in this function returns the *same* response (I5) —
structure the code so there's one exit point producing the 401, not one
`return unauthorized()` call per branch that could drift out of sync.

## `jwx/v3` usage notes

- Build the `jwk.Cache` once at startup (or lazily on first use), pointed at
  `{SSO_ISSUER}/.well-known/jwks.json` — not per-request. The cache is what
  satisfies I7 (never pin one key; the library refetches on its own
  schedule); don't add a second caching layer on top of it.
- Pin `jwa.RS256` explicitly in the verify call (I6) — don't read the
  algorithm from the token's own header.
- Validate `aud` against `AUTH_AUDIENCE` from env (never a literal) and
  `iss` against `SSO_ISSUER`.

## CLI key-issuance script

`cmd/issue-key` (or similar), not exposed over HTTP (`API.md` — no
`POST /api/v1/keys`, deliberately). Generates a random token, prefixes it
(`tpl_`), stores `key_hash` (SHA-256) + `key_prefix` (first 8 chars) +
`expires_at` (90 days, matching my-task's `mtk_` convention), prints the raw
key to stdout **once**. Mirrors my-task's `agent:add`.

## Test naming — this is what closes Done-when 12

`_goal/GOAL.md` Done-when 12 requires every invariant I1–I10 to have a test
whose name references it. This task supplies I1, I2, I5, I6, I7, I8, I9,
I10 (task-3 supplies I3, I4 — don't duplicate those here). Concretely, test
names should look like `TestI2_BearerNeverResolvesToOwner_APIKeyPath`,
`TestI2_BearerNeverResolvesToOwner_JWTPath` (both paths, separately — see
Done-when 5), `TestI6_JWTAlgorithmPinnedToRS256`, `TestI7_JWKSCachedNotPinned`,
and so on. The grep check in Done-when 12 is only as good as this
convention being followed — don't leave I8 (hashed storage) or I9
(expired/revoked checked live) as incidental assertions inside a
differently-named test where the grep won't find them.
