# Milestone 2: close the my-task parity gap, restructure for two surfaces

TPL-1 was reopened after มายด์ reviewed milestone-1's merged template and
found five things present in my-task that nobody ever asked to remove.
Clara then found four more by actually comparing my-task to my-template
properly. This milestone closes all nine, plus a breaking architecture
restructure that underpins three of them, plus a confirmed defect in
already-shipped code found while verifying a related claim.

## Objective

Ship a Go microservice template that actually preserves what TPL-1 asked
for — my-task's identity/settings pattern in full, not a narrowed reading
of it — with two transport surfaces (a public API for agents/skills, a
minimal owner-facing surface for มายด์'s own validation) sharing one
domain/identity layer underneath, and with the SSO/JWT verification code
this milestone touches actually correct against verified library behavior,
not assumed behavior.

## Context — how milestone-1 under-scoped, in the interest of not repeating it

Milestone-1's grill asked Clara whether "preserve SSO integration" meant
JWT-Bearer-only rather than a session-cookie login, since the stack is Go
with no frontend named in the task. **Clara said agree before opening
my-task's code.** That single answer collapsed several independent
architectural properties my-task actually has into one narrow shape:

- A web owner login (my-task has one; the answer implied none needed)
- An internal API surface serving that frontend (tRPC), distinct from the
  public REST API agents use
- A service layer shared by both surfaces (my-task has one;
  milestone-1's module-first layout fused transport into the domain
  module, `internal/todo/handler.go`, leaving nowhere to put a second
  surface even if one had been planned)

Everything downstream — GOAL.md, contracts, all nine milestone-1 tasks,
and four rounds of blind fork testing — inherited that narrowed scope.
The blind tests measured whether the fork *procedure* worked (does
`GETTING-STARTED.md` lead to a working fork), never whether the forked
thing matched the original brief — so they could not have caught this.
มายด์ built exactly what the plan said; the plan was wrong at the top.

Clara found four more gaps by comparing my-task to my-template directly
(not against milestone-1's own GOAL.md, which was downstream of the same
bad scope call): no e2e test against the public API with a real credential
(unit tests inject the actor directly, so the actual credential-resolution
path has never been exercised end to end — same gap my-task's own
`smoke-api-v1.ts` exists to close), key lifecycle stops at issue (no
list/rotate/revoke), no seed script, no companion API skill doc for other
agents to call this service.

**Review this time checks delivery against TPL-1's original task text
(not this repo's own GOAL.md) plus Clara's gap list** — precisely because
last time's review checked against an artifact downstream of the mistake.

## Decisions

| Decision | Answer | Owner |
| --- | --- | --- |
| Architecture restructure | `internal/domain/{todo,…}` (deletable on fork; may import `identity`, never a sibling domain) · `internal/identity/` (its own layer — every domain depends on it, it depends on none, never deleted on fork, so it does not belong under `domain/`) · `internal/transport/{publicapi,bff}` (no `shared/` — see below) · `internal/platform/` (config, logging, db, server, plus cross-cutting gin concerns both engines need) · `web/` at repo root (Go convention, not under `internal/`) | มายด์ (shape), Clara (identity-not-domain refinement) |
| `transport/shared/` | **Dropped.** Proposed as a placeholder, flagged by Clara herself as a junk-drawer risk since she couldn't name what belongs in it. Luna's research: my-task's two surfaces (REST + tRPC) share exactly one thing — the service layer, which already lives outside transport entirely. No concrete shared *transport-level* concern existed to justify the package. Principle: don't pre-declare a package named for a category rather than a thing; add it later, named for what it actually holds, if something genuinely shared shows up | Clara (accepted the finding, recorded the scope change as hers) |
| `ARCHITECTURE.md` rule 1, strengthened | A domain module imports **no transport package at all** (not "only handler.go may import gin" — domain files don't do transport anymore, period, now that transport lives in `internal/transport/`). Repo-wide rule change, outlives this milestone | Clara |
| Cross-cutting gin concerns (panic recovery, request logging, request ID) | Live in `platform/`, applied to both `publicapi` and `bff` engines from one place. Reasoning: these are infrastructure concerns about how *any* gin engine in this service behaves, not policy specific to either surface — the line that keeps working when a third surface eventually appears | Luna (reasoning), Clara (agreed, called it better than her own instinct) |
| BFF framework | gin — reuses the existing framework rather than introducing a second one, fits the "minimum authenticated view" scope (server-rendered, not a SPA) | Luna |
| Public API | REST for agents/skills, key-authenticated (`mtk_`-style prefix pattern, mirrors my-task's `agent:add`), owns `openapi.yaml`. Distinct surface from the BFF, per item 2 | มายด์ |
| Shared service layer | One transport-agnostic service package per domain — actor passed as an explicit parameter (constructor or method arg), never derived from a request/context object inside the service. Both `publicapi` and `bff` call into the same instance. Mirrors my-task's `TaskService`/`TaskQueryService` exactly | มายด์ (item 4), verified against my-task's actual code (Luna) |
| Owner login shape | **Not** "port my-task's login" (hestia's framing correction: "needs a UI" and "has an owner who must prove identity" are independent questions — `authorization_code`+PKCE per contract §2 needs a redirect-receiving surface, not a frontend). Login + callback per §2, **plus the minimum authenticated view needed to exercise the domain** (log in, see todos) — not a UI framework, session handling only as far as serving one authenticated page requires. This is มายด์'s own validation path ("this is my test... to look at UI and validate logic") — not optional scope, it's how he checks the template at all | Clara's call, recorded as hers; open to being argued down during the build if it proves heavier than it sounds |
| Client registration | Template ships **no** Hydra client — structurally impossible per contract §6 (`--id` is `<service>`/`<service>-dev`, derived from a service name that doesn't exist pre-fork; confirmed by hestia reading `register-my-task.sh`, which is Freya's own script with my-task's literal values hardcoded, not a generic tool). Ships `register-<service>.sh` as a placeholder template (`<SERVICE_NAME>`, `<PORT>`, …), filled in once at fork time — envsubst-style, same pattern as `hydra.yml.template`. **Local-only dev is not a special case** — a fork that never deploys still runs the filled-in script once with the `-dev` client before owner login works even on localhost. This is step 1 of `GETTING-STARTED.md`, not optional-by-omission | hestia (via Clara) |
| PKCE (Go-side) | `golang.org/x/oauth2` (`GenerateVerifier`/`S256ChallengeOption`/`VerifierOption`, added v0.13.0) is opt-in only — nothing automatic. Verifier generated per auth attempt, threaded through both the redirect (`AuthCodeURL`) and the callback (`Exchange`) explicitly. Verified against the library's actual doc comments, not assumed from having heard "the library supports PKCE" | Luna, verified against source |
| RS256 pinning (Go-side, jwx/v3) | `jws.WithKeySet(set)` pins by `kid`+`alg` as long as each JWK carries an explicit `"alg":"RS256"` (most IdPs do). **Never enable `WithInferAlgorithmFromKey`** — library maintainer calls it more vulnerable, off by default, must stay off. `none`-alg tokens rejected by default. Verified against jwx/v3's actual pkg.go.dev doc comments, not the earlier milestone's secondhand claim | Luna, verified against source |
| JWKS rotation handling — **correction to milestone-1's shipped code** | `jwk.Cache` refreshes on a schedule only (15min default window), **never lazily on a cache miss** — confirmed both against jwx's own maintainer (GitHub Discussion #1433) and directly in `internal/identity/jwt.go:96`, where a `kid` miss just returns an error today. Fix: on `kid`-not-found, force `Cache.Refresh()` and retry **exactly once**, then fail — not a loop, so a random-`kid` probe can't turn into issuer-hammering. This isn't a contract violation (§7.4 already expects a post-rebuild restart fleet-wide) but removes the need for one, which is the behavior a reference implementation should demonstrate | Clara (found independently, confirmed the same line Luna flagged), Luna (root cause verified against jwx source) |
| I7 wording | Currently describes the caching mechanism and TTL but omits the failure mode on a `kid` miss. Rewrite to state it explicitly (mirrors the fix above) | Clara |
| Key lifecycle | Extend beyond issue-only to `list`/`rotate`/`revoke`, mirroring my-task's `agent:add/list/rotate/revoke`. **`rotate` diverges from my-task's actual ordering, deliberately.** Read `agent.ts`'s `cmdRotate` directly (lines 236-256): it calls `disableAllKeysFor` *before* `issueKey` — a real, uncommented gap where the agent has zero valid keys between those two calls, worse than an atomic instant swap. This is the same shape as the fleet learning `hydra-client-secret-no-expand-contract-window` (an individual Hydra client secret has no dual-valid window; the safe pattern is "new alongside old, verify, then remove old" — option (a) in that learning). The template's `rotate` reorders to **issue-new-then-disable-old**, removing the guaranteed zero-key window, matching that learning's safe pattern rather than my-task's actual (undocumented, likely accidental) ordering. **Deliberately not** adding a longer dual-valid grace period on top of that reorder, unlike Hydra's `SECRETS_SYSTEM` case: agent-key rotation here is primarily a leak-response operation ("if a key turns up somewhere it should not be, it needs rotating" — my-task's own docs), so extending the old key's remaining validity window is actively the wrong direction — minimize it, don't schedule it. Copy the *reasoning* from the fleet learning, not its literal window length.

**No-grace-period is only defensible if re-resolving is actually cheap — checked, and today it isn't.** `my-task-guide`'s documented consumer pattern (`AUTH="Authorization: Bearer $(~/.my-task/bin/key)"`) expands once, at assignment — a session holding that variable keeps presenting the old key until it re-runs that line. That's fine *because* my-task's `agent:add`/`agent:rotate` also call `writeKeyFile()` + `ensureKeyResolver()`, so "re-resolve" really is re-running one line that reads a file rotate already overwrote. **Checked milestone-1's `cmd/issue-key`: it only prints the raw key to stdout — no key file, no resolver script.** So today, "just re-resolve" would mean manually re-copying a value from a terminal, not re-running a one-liner — the no-grace-period call would be true in theory and false in practice. Fix: `issue`/`rotate` in this milestone also write a key file (`~/.my-template/keys/<handle>`, mode 0600) and ensure a resolver script (`~/.my-template/bin/key`), mirroring my-task's actual mechanism — this is what makes "rotation invalidates immediately, re-resolve to recover" a one-liner instead of a manual key hunt. Document explicitly, wherever rotate's behavior is documented: rotation invalidates the old key immediately by design; any holder that captured the key into a variable must re-run the resolver, it will not pick up the new key on its own | มายด์ (item 7) for scope; ordering and the key-file gap verified against my-task's actual source and decided independently, not copied (Luna, per Clara's instruction not to default to my-task's answer) |
| Key resolver script — **port verbatim, don't rewrite from the description** | `~/.my-task/bin/key` (read in full) carries a real, non-obvious refinement: an argument that's *present but empty* (`key "$UNSET_VAR"`) is refused explicitly rather than silently falling through to the environment via bash's `${1:-...}` — because that fallthrough once let a documented-but-wrong invocation survive undetected on this host (found by tiana). A resolver written fresh from "resolve crew from arg or env" would re-lose this guard. Port `~/.my-template/bin/key` from the actual script, including: the empty-argument-is-a-mistake check + its exact error message, the fallback chain `argument → MY_TEMPLATE_CREW → TYP_CREW_NAME` (last one host-specific on purpose — the comment explaining *why* the host-crew-naming mapping belongs in the resolver, not the app, ports too), and `0600` on the key file. **State plainly that `0600` is a rule, not an isolation guarantee** — every crew on this host shares one uid, so every key file is readable by every crew regardless of the mode bit; my-task's resolver says this honestly rather than implying protection it doesn't have, and the template should too | Clara (found by reading the resolver directly) |
| Fork collision — key path + env var both carry the service name | `~/.my-template/keys/<handle>` and `MY_TEMPLATE_CREW` both bake in the (un-renamed) service name. **Two forked services that both skip the rename step write to the same directory and silently overwrite each other's key files** — no error, no collision warning, second fork just wins, first fork's agents start authenticating as the wrong identity or failing with a key replaced underneath them. Silent, cross-service, only reproducible once someone forks twice — exactly the kind of gap the earlier blind-fork-test rounds existed to catch, except this one only shows up on a *second* fork, which no single-fork blind test would surface. Must be an explicit `GETTING-STARTED.md` Step 2 rename-checklist line, not left implicit alongside module path/service name | Clara |
| Indistinguishable-401 trap — documentation requirement for item 9's skill doc | `Bearer $(key)` swallows a resolver failure: the resolver correctly exits 1 with a message on stderr, but `$(...)` expansion inside the header just yields empty, so the request goes out as `Authorization: Bearer ` with nothing after it — indistinguishable from a wrong key at the HTTP layer. Anything running outside a crew's own pane (cron, systemd, an escaped process) has no `TYP_CREW_NAME`-equivalent and hits this. The `<service>-api` companion skill (item 9) must carry the same warning my-task-guide does: "when you see a 401, look back at what the key command itself printed" — otherwise every forked service's first agent burns a debugging session on a 401 that was never about the key | Clara |
| e2e smoke test | A real-HTTP script hitting the public API with a real minted key (mirrors my-task's `smoke-api-v1.ts`) — auth negatives (no cred, spoofed actor field), idempotency replay, actor-attribution — closing the gap that all of milestone-1's tests inject the actor directly and never traverse actor-resolution → key lookup → database for real | มายด์ (item 6) |
| Seed script | Idempotent (check-then-insert per natural key, not blind upsert), creates only contract-mandated fixed rows. Mirrors my-task's `seed.ts` pattern | มายด์ (item 8) |
| Companion API skill doc | `<service>-api/SKILL.md` scaffolded from `my-task-api`'s shape: base URL, auth, invariant rules, endpoint table + worked examples, `references/` split for endpoints vs errors — instance-agnostic and key-agnostic. A second, separate consumer-facing skill ("how does my crew actually get and use its key") is out of this milestone's scope — my-task's own split (`my-task-api` vs `my-task-guide`) puts that in a different doc | มายด์ (item 9) |

## Scope

### In scope

- Architecture restructure per the Decisions table (`domain/`, `identity/`,
  `transport/{publicapi,bff}`, `platform/`, `web/`), including migrating
  `internal/todo` → `internal/domain/todo` and updating
  `ARCHITECTURE.md`'s rule 1
- Public API (`internal/transport/publicapi`) — the milestone-1 `/todos`
  and `/keys` endpoints, moved, plus key `list`/`rotate`/`revoke`
- BFF (`internal/transport/bff`) — owner login (`authorization_code`+PKCE
  per §2), callback, session handling limited to serving one authenticated
  page, minimal view of the todo domain
- `platform/` gains shared gin middleware (recovery, request logging,
  request ID) applied to both transport engines
- `register-<service>.sh` placeholder template + `GETTING-STARTED.md`
  step making it step 1, not optional
- JWKS kid-miss fix in `internal/identity/jwt.go` (force-refresh-once),
  `INVARIANTS.md` I7 rewording, `DEPLOY-REQUIREMENTS.md` rotation-behavior
  note
- Key lifecycle: `list`/`rotate`/`revoke` endpoints + CLI, plus `issue`/`rotate` writing a key file + resolver script (currently missing — `cmd/issue-key` only prints to stdout), so re-resolving after a rotation is a one-liner, not a manual copy
- Resolver script ported verbatim from `~/.my-task/bin/key` (empty-argument guard, fallback chain, `0600`-is-a-rule wording) — not rewritten fresh
- `GETTING-STARTED.md` Step 2 gains an explicit line: rename the key path and env var too, or a second fork silently overwrites the first fork's keys
- e2e smoke script against a real running instance with a real key
- Idempotent seed script
- `<service>-api` companion skill doc, including the indistinguishable-401 warning ("look at what the key command printed") and the `0600`-is-a-rule-not-isolation note

### Out of scope

CI workflow yaml (มายด์ ruled out for now, real gap) · Hydra client
retirement/teardown for a decommissioned fork (hestia's — a genuine hole
in `sso-consumer-contract.md` §6, parked with her, not planned here) ·
agent/machine SSO credential design beyond what hestia's review already
settled · a second, crew-facing "how to use your key" skill (my-task's
`my-task-guide` equivalent — different doc, not this milestone's)

## Done when

Machine-checkable stopping conditions, one task owner each (assigned in
`_plan/_todo.md`, not left as an implicit "full suite passing" catch-all):

1. `internal/architecture_test.go` rewritten for the 5-rule layout (domain
   never imports transport, only repo.go imports sqlc, no sibling-domain
   imports, transport never imported by domain/identity, platform imports
   nothing above it) and passes. **Not satisfied by green alone — attack
   the rewritten discovery logic specifically before moving on**, the same
   discipline that's caught two silent guardrail failures already this
   project (the hardcoded `domainDirs` giving a new module zero coverage;
   the invariants check unioning a superseded document). At minimum:
   plant a fake new domain module with a file that imports `gin`
   incorrectly, confirm it's caught by the *dynamically discovered* module
   list (not a name that happened to already be known); make
   `internal/platform` import a domain module and confirm rule 5 catches
   it; make a domain file import `internal/transport/*` and confirm rule
   4 catches it. Green on a correct layout proves nothing about a
   rewritten discovery function — only a caught violation does.
2. Every milestone-1 test still passes after the code move (no regression
   from relocation alone).
3. `platform/middleware.go`'s recovery/logging/request-ID wired into both
   `publicapi` and `bff` engines — a test confirms both, not just one.
4. `register-<service>.sh` exists as a placeholder template with every
   variable `GETTING-STARTED.md` documents; `GETTING-STARTED.md`'s Step 1
   is running it, not optional-by-omission.
5. `GETTING-STARTED.md`'s rename checklist has an explicit line for the
   key path + env var (the fork-collision Clara found) — structural
   presence check, same shape as milestone-1's task-9 fix.
6. I7's fix: a test using a fake JWKS endpoint proves a `kid` miss forces
   exactly one `Cache.Refresh()` + retry (not zero, not a loop) before
   failing.
7. I11: a test proves the owner-login flow cannot construct an auth URL or
   complete a token exchange without a PKCE verifier/challenge pair — not
   just that the library supports one.
8. I12: a test proves a `bff` session resolving to `role='agent'` is
   rejected identically to a missing session.
9. **The authenticated view renders scoped domain data, not just a 200.**
   `GET /` returning success satisfies nothing on its own — Done-when 3,
   7, and 8 are all auth mechanics a dead, empty page would also satisfy.
   An integration test seeds an owner session and a todo belonging to
   that owner, requests `GET /`, and asserts the response body contains
   that todo's title. This is what actually checks the one deliverable
   มายด์'s validation path depends on (found by Clara: the login flow had
   four criteria and the page it logs into had none — same shape as
   milestone-1's original "easy to get started" being the least
   instrumented item because it was the least mechanical to check).
10. I13: a test proves `rotate` issues the new key before disabling the
    old one(s) — ordering, not just eventual consistency.
11. I14: a test proves the ported resolver refuses a present-but-empty
    argument (the exact guard from `~/.my-task/bin/key`), and that
    `issue`/`rotate` leave a working key file + resolver behind.
12. An e2e smoke script runs real HTTP against a live instance with a real
    minted key — auth negatives (no credential, spoofed actor field),
    real CRUD round-trip — the credential-resolution path actually
    exercised end to end, not just unit-injected.
13. Seed script is idempotent — running it twice leaves exactly one owner
    row, checked directly, not assumed from "it uses check-then-insert."
14. `<service>-api` skill doc exists with the sections `_goal/GOAL.md`'s
    Decisions table names (base URL, auth, invariant rules, endpoint
    table + examples, `references/` split), plus both required warnings
    (indistinguishable-401, `0600`-is-a-rule) present as identifiable
    sections, not buried in prose.
15. `internal/invariants_test.go`'s promoted-contract-is-sole-authority
    fix (already landed during planning, see commit `4eac2c0`) still
    passes with the full I1–I14 set once every task above has landed its
    tests.
16. **`docker compose up` still works after the restructure.** Not
    inherited from milestone-1 — Clara's milestone-1 verification pass
    deliberately deferred this check (a port conflict with a running
    blind-fork-test agent) and never came back to it, so **the last
    confirmed-working state was Luna's manual check before four rounds of
    fixes landed on top**, not anything re-verified since. The restructure
    moves every package `Dockerfile`/`docker-compose.yml` implicitly
    reference by path — check it directly rather than assuming relocation
    alone didn't break the container build.

## Known limitation: the owner has no supported way to create a todo

Found by Clara standing up a real dev instance for มายด์'s acceptance
attempt (2026-08-13, task-10): the public API rejects owner Bearer tokens
by design (I2), and the BFF (`internal/transport/bff`) is read-only —
`GET /login`, `GET /callback`, `GET /` — no write path exists anywhere
for the owner. Clara had to `INSERT` directly into the database to give
มายด์ anything to look at. **This is expected, not a bug** — not adding
owner-write endpoints this milestone (out of scope, มายด์'s call, not
asked for) — but it's real enough that a future person standing up a demo
or acceptance check should be told, not left to discover it by hitting
the same wall Clara did. Documented in `docs/GETTING-STARTED.md`'s
"Running what you forked" section: seeding data currently requires
reaching into the database directly (`cmd/seed`-style direct access, or
manual SQL).

**Open design question, not answered here:** if the owner never holds an
API credential (I2) and the BFF never writes, who creates the owner's
data in a real fork? The likely answer, matching my-task's own actual
pattern, is agents acting on the owner's behalf — an agent holds the API
key and performs the write, the owner only ever views. If that's the
intended shape for this template too, a future milestone should say so
explicitly, so this read-only surface reads as a deliberate design
decision rather than an unfinished stub. Not designed or decided here —
only named, so it isn't lost.

### Human acceptance — after the loop, not part of it

**มายด์ logs in through the owner-login flow against a real, registered
Hydra client, on a real deployment, and sees his own todos.** Not "logs
in" alone — his own words were "this is my test... to look at UI and
validate logic," and a completed login landing on a page that renders
nothing would satisfy "logs in" without validating anything. This is
explicitly his own validation path for the template, not a formality —
and it can't be a stopping condition for the same reason milestone-1's
original JWT human-acceptance criterion couldn't: no unattended loop can
complete a real interactive login, and — per this milestone's
client-registration decision — no fork has a registered Hydra client
until `register-<service>.sh` is run once by a human against a real
issuer. Done-when 7's PKCE test, Done-when 9's scoped-data-rendering test,
and Done-when 13's seed-script check are the machine-checkable half; that
last stretch — a real browser, real Hydra, real consent screen — is
checked once, by a human,
afterward.
