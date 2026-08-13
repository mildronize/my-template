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
- `scope: per-domain-module` — requires the test **inside every package
  I3/I4's ownership-scoping and single-seam-identity-read properties
  actually apply to** (`perDomainModuleScopePackages()`,
  `internal/invariants_test.go` — a small, explicit, hand-maintained list,
  today `internal/domain/todo` and `internal/identity`; asserted a
  superset of `domainModuleNames()` so a new domain module never goes
  silently unchecked). Closes the hole where one domain module's test
  satisfied the check for every other module forever (milestone-1
  task-7's finding). **Reserved for I3/I4 only** since milestone-4's
  scope-tags fix-round — see `domain:<name>` below for every other
  single-place invariant.
- `scope: domain:<name>` — requires the test **inside the one specific
  package `<name>` resolves to** (`domainScopePackageNames()`,
  `internal/invariants_test.go` — an explicit name→path mapping, e.g.
  `domain:todo` → `internal/domain/todo`, `domain:identity` →
  `internal/identity`; not derived from a naming convention, since
  `internal/identity` deliberately isn't under `internal/domain/` per
  `_rules/_standard/ARCHITECTURE.md`'s milestone-2 decision). Added by
  milestone-4's scope-tags fix-round: `per-domain-module` used to also
  carry I15-I19 and I21, each of which belongs to exactly one specific
  place, not a coverage sweep across every domain module — that only
  "worked" because `domainModuleNames()` had exactly one member (`todo`)
  at the time. An unrecognized `<name>` makes the check abort loudly, not
  silently pass.

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

*Scope, per domain (milestone-4 correction — generalised from the
original `A todo (or API key)` wording to `A resource`, since this
invariant is `scope: per-domain-module` and naming specific resources in
its general statement was always slightly off; reach narrowed per domain
below):* holds for the identity/API-key domain exactly as
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

*Clarification (milestone-4 — of what this always meant in practice, not a
weakening; the mechanism enforcing this changed and the text has to say
what it actually enforces, not what a reader's own reasonable-sounding
gloss on "one repo, one table" might otherwise assume):* **a query file
may read (never write) a table it does not own, but only via an explicit,
named, per-file grant** — `internal/dbquery.ReadOnlyGrants`, enforced
mechanically (a grant does not make a table's writes legal; `INTO`/
`UPDATE` against a granted table still fails this invariant, checked
directly against the SQL text, not trusted to the grantor's stated
intent) and required to be exercised (an unused grant is itself a
failure, not a permanent dormant exemption). Today's one grant:
`todo_events.sql` may read `users` (the cross-todo activity feed's actor
handle/role), because it displays identity data, never decides anything
from it. Without this clarification, a cross-module read looks equally
forbidden under "one repo, one table" as written — the milestone-4
mechanism that implements the distinction was found to be a *reading* of
the text, not something the text itself said, which is a trap for
whoever reads I4 next without also reading the mechanism.

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

**I15 — One write path (todo domain).** `scope: domain:todo`
*(New, milestone-4.)* Only the todo domain's service module writes to
`todos` or `todo_events`; no handler, script, or other module touches
those tables directly. Mirrors my-task's *intent* (its own I4), but
**not its enforcement shape — the two are not available to this
codebase in parity, and that gap is stated rather than closed by
wording.** my-task's I4 works because Drizzle lets a module export table
objects to only the files that need them; `sqlc.yaml` here emits one
shared `db` package (`package: "db"`, `out: "internal/db"`) that every
module can import — there is no per-table export boundary to withhold.
*Enforced by:* `internal/architecture_test.go`'s
`TestArchitecture_OnlyRepoFilesImportSqlc` — a real, existing,
per-file-parsed import-graph test. **State plainly what it actually
buys**: it's a *layer* rule (only `repo.go`/`*_repo.go` files may import
the generated package, across every domain module and identity) — it
does not stop a repo file in one module from querying another module's
table by name, the way my-task's table-level export boundary would. The
Go mechanism is coarser than the source's. **Extended this milestone**
with a second, table-specific check: an architecture test asserting only
`internal/domain/todo`'s own repo file references the generated
`*TodoEvent*`-named query functions — cheaper than a second `sqlc`
output package, and it restores the actual property (not just the
layer-level approximation of it) without requiring a second generated
package this template's size doesn't otherwise need. **The check must
first assert it found at least 3 such functions** (the design's own
floor: insert one event, list a single todo's events, list the
cross-todo feed — the minimum operations this milestone's write/read
paths require) **before it asserts who references them.** A name-matcher
that matches zero functions passes trivially and enforces nothing — the
same shape `keys_handler_test.go` already cost this project once. If the
count ever drops below 3, that's a real signal (the design changed, or a
rename broke the matcher) and both deserve a failing test, not a silent
pass. **Verified by test**, two of them: the transaction property (a
failure mid-write leaves neither the event row nor the `todos` state
change) and the new table-specific reference check above, itself gated
on finding a non-trivial number of functions to check.

**I16 — `created` is never client-specifiable.** `scope: domain:todo`
*(New, milestone-4.)* No request body, on either `publicapi` or `bff`,
may set `type: "created"` directly — a `created` event only ever happens
as a side effect of `POST /todos` itself. Mirrors my-task's
`buildAppendInput` switch, which has no `created` case at all
(`~/gits/my-task/src/app/api/v1/tasks/[id]/events/route.ts:45-181`). A
security property, not a style choice: a client that could post
`type: "created"` could forge a creation event under any actor and
timestamp, in the log that exists specifically to establish who did what.
*Enforced by:* the write path's dispatch has no case that accepts a
client-supplied `created` type — the same shape I1 already uses for actor
identity, applied here to event type instead. **Verified by test**: a
`POST` with `type: "created"` is rejected (400), not silently accepted or
silently ignored.

**I17 — `todo_events` is append-only, for everyone.** `scope: domain:todo`
*(New, milestone-4.)* No `UPDATE`, no `DELETE`, no exceptions for the
owner. Corrections are new events. Mirrors my-task's I3 exactly, including
its enforcement shape and its explicit limit: application-level only (no
service method, handler, or route exists that updates or deletes an
event) — **no database trigger or CHECK constraint**, matching the named
source, not exceeding it. If evidence ever surfaces that convention-only
enforcement is insufficient here, that's a question to raise, not
something to unilaterally strengthen (`milestone-4/_goal/GOAL.md`'s
Append-only enforcement decision). *Enforced by:* no code path exists
that updates or deletes a `todo_events` row. **Verified by test** — the
same distinction my-task's own I3 test draws: the test asserts state
changes always add a row, not merely that "no update method exists on the
repo."

**I18 — Only the owner may move a todo to `closed`.** `scope: domain:todo`
*(New, milestone-4.)* Any agent may comment, assign, change fields, or
change status to anything except `closed`, on any shared todo. Only a
session-authenticated owner may set `status: closed`. Mirrors my-task's
I10 (`can()`'s `task:status_change` rule refusing an agent's move into the
`closed` group) applied to a single terminal status rather than a status
group, since this domain's enum has one `closed` value where my-task has
a `closed` group that can (in principle) hold more than one status.
*Enforced by:* the permission layer (`can(actor, action)`, role-based, not
per-todo-identity-based), checked **inside I15's single write path**,
before it dispatches to any type-specific handling — not at each
`publicapi`/`bff` call site the way my-task's own `can()` is checked (once
per entry point, before calling `append()`). **This is a deliberate
strengthening past the named source, stated as one, not an accidental
reading of an ambiguous sentence**: my-task needs a `can()` call at every
entry point specifically *because* its `append()` itself doesn't check
permission (survey, Part 1: *"append() itself does not check permission —
the caller does"*) — a shape that only works if every current and future
caller remembers to check first. I15 already centralizes every todo write
through one function; putting the permission check there means a future
caller cannot skip it by forgetting, the same way my-task's shape
structurally can. **This is the opposite direction from I17's
enforcement, on purpose, not a drift toward inconsistency**: I17
deliberately matches my-task's append-only enforcement without exceeding
it, because there was no existing centralization to exploit for that
property; I18 exceeds my-task's permission enforcement because I15's
centralization makes exceeding it nearly free, not because "stronger is
always better" was applied uniformly. **Verified by test**, paired: the
same agent,
against the same todo, has a `status: closed` attempt rejected and a
non-closed action succeed in the same test — a permission layer that
rejects everything would pass a reject-only assertion just as well as a
correct one.

**I19 — Writes are idempotent when the client request id is reused (todo domain).** `scope: domain:todo`
*(New, milestone-4.)* A repeat `POST` carrying the same `clientRequestId`
returns the original event, unchanged, and creates nothing — checked
inside the same transaction as the write itself, before dispatch. Mirrors
my-task's I5 exactly. `_rules/_contract/API.md`'s own Conventions text
already named this exact case ("No Idempotency-Key requirement... re-add
it if a fork adds one [an event log]") — this is that fork. *Enforced
by:* the unique constraint on `todo_events.client_request_id` plus a
lookup at the top of the write path. **Verified by test** — the same key
twice yields one row and two identical responses.

**I20 — Comment bodies never render as raw HTML.** `scope: global`
*(New, milestone-4.)* A comment's `body` is written as plain text and
rendered, client-side, through a Markdown-to-React-elements path — never
`dangerouslySetInnerHTML`, never a raw-HTML string reaching the DOM.
Mirrors my-task's I8 exactly, including its stated reason: the activity
log is a cross-agent channel, and an unescaped body is an injection
surface into whatever reads it next, human or agent. *Enforced by:* the
rendering component's implementation — there is exactly one place a
comment body is rendered (mirrors my-task's shared `TimelineEventRow`),
so there is exactly one place this can be gotten wrong. **Verified by
test**: a body containing raw HTML tags renders as literal text/escaped
elements in the rendered output, not as unescaped markup.

**I21 — The owner's key-listing spans every agent's keys; an agent's own key-listing stays self-scoped.** `scope: domain:identity`
*(New, milestone-4. Correction to milestone-2/3's `bff` key-listing,
which scoped to the session owner's own `user_id` — a set that can never
be non-empty, since `cmd/issue-key` never issues to `role='owner'` and I2
forecloses it structurally. That endpoint's semantics are replaced, not
kept alongside a new one — see `milestone-4/_goal/GOAL.md`'s decision.)*
`GET /api/bff/keys` (owner session) returns every `role='agent'` user's
non-revoked keys; `DELETE /api/bff/keys/:id` (owner session) may revoke
any of them. `GET /api/v1/keys` (agent Bearer credential) is unchanged —
still scoped to the caller's own keys only, I3 unchanged for this half of
the identity domain. This is the first case where the owner is
deliberately given visibility into another user's private resource, by
design — I3's "absence, not permission" framing does not apply to the
owner's half of this endpoint pair, on purpose. *Enforced by:* the BFF
handler's query joins on `role='agent'` rather than filtering by the
session's own `user_id`; the public API handler is untouched. **Verified
by test**: the owner-facing query returns keys seeded through
`cmd/issue-key`'s real path (not a direct repo insert on a convenient
role — the exact trap `keys_handler_test.go`'s pre-milestone-4 fixture
fell into), and a separate test proves the agent-facing endpoint stays
self-scoped.
