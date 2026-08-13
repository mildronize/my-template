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

To be finalized after Phase 2 (contracts) — placeholder structure, filled
in once the contract for the two-surface split and the owner-login flow is
written. Will follow milestone-1's pattern: one stopping condition per
requirement above, each owned by exactly one task, no implicit
"full suite passing" catch-alls.
