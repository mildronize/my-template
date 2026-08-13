# Invariants

Promoted from milestone-1's `_contract/INVARIANTS.md` to `_rules/_contract/`
in milestone-2, same reasoning as `DATA_MODEL.md` — genuinely
cross-milestone, `AGENTS.md` names `_rules/_contract/` for exactly this.
Milestone-1's copy is historical only from here.

Numbered so `API.md`, `DATA_MODEL.md`, and test names can point at a
specific one instead of re-describing it. Authority for the auth-related
invariants (I2, I6, I7, I9, I10, I11, I12) is
`~/gits/prod-thw-home/docs/sso-consumer-contract.md` — cited by section
below, not restated in full.

**`scope:` tag**, machine-parsed by `internal/invariants_test.go` to
decide *where* it looks for an invariant's `TestI<N>_...` test:

- `scope: global` — greps the whole repo for the test, same as before this
  tag existed.
- `scope: per-domain-module` — requires the test **inside every domain
  module's own package** (`domainModuleNames()`, updated in milestone-2 to
  enumerate `internal/domain/*` — see `_rules/_standard/ARCHITECTURE.md` —
  not `internal/*` as milestone-1's version did, since domain modules moved
  under `internal/domain/` this milestone). Closes the hole where one
  domain module's test satisfied the check for every other module forever
  (milestone-1 task-7's finding).

**I1 — Actor identity never comes from the request.** `scope: global`
No body field, query param, or header naming an actor is ever read for
identity; presence of one is a 400 (`actor_field_present`), not a value
that gets ignored. Identity comes only from the resolved credential.
Applies to both `publicapi` (key/JWT-derived actor) and `bff` (session-
derived actor) — a caller cannot declare who they are on either surface.

**I2 — A Bearer credential never resolves to `role='owner'`.** `scope: global`
Neither the API-key path nor the JWT path may return a user row with
`role='owner'` as an authenticated actor (contract §5). Owner authority
comes only from the BFF's interactive session (I12), never from a Bearer
credential on `publicapi`.

**I3 — Ownership scoping is absence, not permission.** `scope: per-domain-module`
A resource that exists but belongs to a different owner returns
`not_found`, the same response as one that never existed. Never
`forbidden` — that would confirm the row exists.

*Scope, per domain (milestone-4 correction — wording unchanged, reach is
not universal):* holds for the identity/API-key domain exactly as
written — an agent's own key-listing stays scoped to itself; a wrong-id
request there is still `not_found`, never `forbidden`. **Does not apply
to the todo domain from milestone-4 onward**: todos are a shared
collection every authenticated actor can see and act on
(`milestone-4/_goal/GOAL.md`'s Ownership model decision), so there is no
"belongs to a different owner" case left for a todo to trigger this rule
on. A narrowing of I3's *reach*, not a reversal of what it says — noted
here, not only in the milestone goal, because this is the file the next
reader of I3 actually opens.

**I4 — One seam reads identity.** `scope: per-domain-module`
Only the actor-resolution middleware queries `users`/`api_keys` to
establish who's calling. Handlers receive an already-resolved actor; they
never do their own lookup against those tables. Practical form: "one repo,
one table" per domain module.

**I5 — 401 never leaks why.** `scope: global`
Missing credential, malformed credential, expired key, revoked key,
expired JWT, wrong issuer/audience, and the I2/I12 role-rejections all
produce the same response body. The specific reason is for server-side
logs only (contract §7.5).

**I6 — JWT algorithm is pinned, never negotiated.** `scope: global`
Validation always specifies RS256; the token's own `alg` header is never
trusted to choose it (contract §7.2 — defends against algorithm-confusion
attacks). Go-side: `jws.WithKeySet`, never `jws.WithInferAlgorithmFromKey`
(verified against `lestrrat-go/jwx/v3`'s own doc comments, milestone-2).

**I7 — JWKS is cached, never pinned to one key, and a `kid` miss forces exactly one refresh, not an outage.** `scope: global`
The key set is refetched from the issuer's `/.well-known/jwks.json` on a
schedule (`jwk.Cache`'s refresh window); a specific key is never hardcoded
or pinned, because Hydra's signing key regenerates on every SSO rebuild
(contract §7.3). **Milestone-2 correction to milestone-1's actual shipped
behavior:** `jwk.Cache` does not refetch lazily on a cache miss (verified
against the jwx maintainer directly, GitHub Discussion #1433) — a `kid`
not found in the currently-cached set must trigger exactly one
`Cache.Refresh()` + retry, then fail, not silently wait out the refresh
window (up to 15 minutes) or loop indefinitely (an attacker sending random
`kid`s must not be able to force repeated issuer calls). This isn't a
contract violation — §7.4 already expects a restart after an SSO rebuild
fleet-wide — but removes the need for one, which is the behavior a
reference Go implementation should demonstrate.

**I8 — API keys are stored hashed.** `scope: global`
The raw key exists only at issuance (CLI stdout); `api_keys.key_hash` is
one-way. A key can be revoked or regenerated, never retrieved.

**I9 — Expired or revoked fails identically to wrong.** `scope: global`
`api_keys` rows past `expires_at` or with a non-null `revoked_at` are
checked at *query* time on every request, not just at issuance — a key
valid an hour ago must fail now if it expired or was revoked in between.

**I10 — No JIT provisioning for machine identity.** `scope: global`
A JWT `sub` with no matching `users.sso_subject` row is unauthorized,
never auto-created. Not the same as the contract's §2/§4 JIT-for-humans
rule — that's about the interactive login flow. Human provisioning (I2/I12's
owner row) is also not JIT in this template, but for a different reason —
see `DATA_MODEL.md`'s "Owner provisioning" note.

**I11 — PKCE is never optional on the owner login flow.** `scope: global`
*(New, milestone-2.)* `authorization_code` + PKCE, always, per contract
§2 — Go-side, `golang.org/x/oauth2`'s PKCE support (`GenerateVerifier`/
`S256ChallengeOption`/`VerifierOption`, since v0.13.0) is opt-in, not
automatic; the login flow must call all three explicitly, every time,
with no code path that constructs the auth URL or performs the token
exchange without them. Verified against the library's own source, not
assumed from "the library supports PKCE" (the same shape of claim that
undersold jwx/v3's JWKS guarantee — see I7).

**I12 — A BFF session never resolves to `role='agent'`.** `scope: global`
*(New, milestone-2.)* The inverse of I2: a session-authenticated request
on `bff` whose resolved user has `role='agent'` is unauthorized. Machine
identity never gets the owner surface, the same way owner identity never
gets a Bearer credential (I2) — the two roles' respective proof-of-identity
mechanisms are disjoint, not just conventionally separated. Mirrors
my-task's actual `resolveActor()` session-path check.

**I13 — Key rotation issues the new key before disabling the old one(s).** `scope: global`
*(New, milestone-2.)* Milestone-2 correction to my-task's
actual (undocumented) ordering — my-task's `agent.ts` `cmdRotate` disables
before issuing, producing a real gap where the caller holds zero valid
keys. This template's `rotate` reorders to remove that gap. Deliberately
**not** an extended dual-valid grace period beyond that reorder — key
rotation here is primarily leak response, and extending the compromised
key's remaining validity window is the wrong direction for that case (see
milestone-2 `_goal/GOAL.md`'s Key lifecycle decision for the full
reasoning, including why this differs from the fleet's
`hydra-client-secret-no-expand-contract-window` pattern rather than
copying it).

**I14 — A resolved key/session must be re-derivable after rotation without a manual value hunt.** `scope: global`
*(New, milestone-2.)* `issue`/
`rotate` write a key file (`~/.my-template/keys/<handle>`, `0600`) and
maintain a resolver script (`~/.my-template/bin/key`) the same shape as
my-task's, including the empty-argument guard (an argument that's present
but empty is refused, not silently treated as "no argument" — the
refinement that cost the fleet a real debugging session, ported verbatim
per `_goal/GOAL.md`'s Key resolver decision, not rewritten from
description). Without this, I13's "no grace period" design is only true in
theory — recovery from rotation must be one command, not a copy-paste from
a terminal scrollback.
