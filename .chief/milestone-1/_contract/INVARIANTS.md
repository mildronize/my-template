# Invariants

Numbered so `API.md`, `DATA_MODEL.md`, and test names can point at a
specific one instead of re-describing it. Authority for the auth-related
invariants (I2, I6, I7, I9, I10) is
`~/gits/prod-thw-home/docs/sso-consumer-contract.md` — cited by section
below, not restated in full.

**`scope:` tag.** Each heading below carries a `scope:` marker,
machine-parsed by `internal/invariants_test.go`
(`TestDoneWhen12_EveryInvariantHasANamedTest`) to decide *where* it looks
for that invariant's `TestI<N>_...` test, not just whether one exists
anywhere in the repo:

- `scope: global` — the check greps the whole repo for a `TestI<N>_...`
  test, same as before this tag existed.
- `scope: per-domain-module` — the check requires a `TestI<N>_...` test
  **inside every domain module's own package**
  (`domainModuleNames()`, `internal/architecture_test.go`), not just
  somewhere in the repo. This closes the hole where one domain module's
  test satisfied the check for every other module forever (task-7,
  Clara's second blind fork test): a forked domain module can no longer
  ship with zero ownership-scoping tests of its own while the suite stays
  green.

**I1 — Actor identity never comes from the request.** `scope: global`
No body field, query param, or header naming an actor is ever read for
identity; presence of one is a 400 (`actor_field_present`), not a value
that gets ignored. Identity comes only from the resolved credential.

**I2 — A Bearer credential never resolves to `role='owner'`.** `scope: global`
Neither the API-key path nor the JWT path may return a user row with
`role='owner'` as an authenticated actor (contract §5). Owner authority
requires an interactive session and a password, which this service does
not implement — so in practice no request should ever legitimately reach
this check with an owner row, which is exactly why it's tested directly
(GOAL.md Done-when 5) rather than assumed unreachable.

**I3 — Ownership scoping is absence, not permission.** `scope: per-domain-module`
A todo (or API key) that exists but belongs to a different owner returns
`not_found`, the same response as one that never existed. Never
`forbidden` — that would confirm the row exists. Per-domain-module because
this is an invariant about *each* module's own ownership-scoped resource,
not a single global property one module's test can vouch for on behalf of
another.

**I4 — One seam reads identity.** `scope: per-domain-module`
Only the actor-resolution middleware queries `users`/`api_keys` to
establish who's calling. Handlers receive an already-resolved actor; they
never do their own lookup against those tables. Per-domain-module because
"one repo, one table" (this invariant's practical form —
`TestI4_TodoRepoOnlyQueriesTodosTable`, `internal/todo`) is a claim about
each module's own repo, not something one module's test can prove for
another's.

**I5 — 401 never leaks why.** `scope: global`
Missing credential, malformed credential, expired key, revoked key,
expired JWT, wrong issuer/audience, and the I2 owner-rejection all produce
the same response body. The specific reason is for server-side logs only
(contract §7.5).

**I6 — JWT algorithm is pinned, never negotiated.** `scope: global`
Validation always specifies RS256; the token's own `alg` header is never
trusted to choose it (contract §7.2 — defends against algorithm-confusion
attacks).

**I7 — JWKS is cached, never pinned to one key.** `scope: global`
The key set is refetched from the issuer's `/.well-known/jwks.json` (with
a cache TTL); a specific key is never hardcoded or pinned, because
Hydra's signing key regenerates on every SSO rebuild (contract §7.3). A
service that pins a key breaks on the next SSO rebuild, silently, until
someone notices 401s.

**I8 — API keys are stored hashed.** `scope: global`
The raw key exists only at issuance (CLI stdout); `api_keys.key_hash` is
one-way. A key can be revoked or regenerated, never retrieved.

**I9 — Expired or revoked fails identically to wrong.** `scope: global`
`api_keys` rows past `expires_at` or with a non-null `revoked_at` are
checked at *query* time on every request, not just at issuance — a key
valid an hour ago must fail now if it expired or was revoked in between.

**I10 — No JIT provisioning for machine identity.** `scope: global`
A JWT `sub` with no matching `users.sso_subject` row is unauthorized,
never auto-created. This mirrors my-task's agent path and is *not* the
same as the contract's §2/§4 JIT-for-humans rule — that rule is about the
interactive login flow, which this service doesn't have at all.
