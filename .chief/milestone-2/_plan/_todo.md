# Milestone 2 TODO

See `../_goal/GOAL.md` for objective/scope/decisions/done-when and
`../_contract/API.md` + `.chief/_rules/_contract/{API,DATA_MODEL,
INVARIANTS}.md` for the contract. Order matters — later tasks depend on
the restructure landing first. Every Done-when item is owned by exactly
one task. (Done-when 15's underlying *mechanism* — the invariants-
authority fix — already landed during planning; task-8 owns *re-checking*
it holds once every other task's tests exist, not rebuilding it.)

**`go test ./...` is red right now, on purpose, and stays red until
task-5 lands** — the contract (`_rules/_contract/INVARIANTS.md`) already
declares I11–I14, and the code that satisfies them doesn't exist yet.
This follows milestone-1's actual pattern exactly, not a new adaptation
for a red tree: milestone-1's own tasks each owned specific numbered
Done-when items (task-1 owned 1/11, task-2 owned 5/6, and so on) and
**full-suite-green was never a per-task gate there either** — it belonged
to task-5 alone (`_goal/GOAL.md`'s original Done-when 8/9/10). The only
difference here is that this milestone's contract legitimately runs ahead
of its code for longer, so the gap is more visible — the plan structure
that already handles it doesn't need to change. **Each task's own
verification is: build clean, and the specific `TestI<N>_...`/architecture
tests its own Done-when items name pass — not the whole suite.** Task-5
(the last task landing an I11–I14 invariant, per the ordering below) is
where `go test ./...` fully green becomes the real check, and a still-red
I1–I10 at that point is a genuine regression, since those were green
before this milestone started.

- [x] task-1: Architecture restructure — `internal/todo` → `internal/
      domain/todo`; `internal/identity`'s `handler.go` → `internal/
      transport/publicapi` (identity's `service.go`/`repo.go`/`jwt.go`
      stay in `internal/identity`); rewrite `internal/architecture_test.go`
      for the 5-rule layout (`_rules/_standard/ARCHITECTURE.md`) — domain
      module discovery moves to `internal/domain/*`, transport-surface
      discovery is new (`internal/transport/*`), the old handler-role
      allowlist for rule 1 simplifies to a flat "no domain file imports
      gin." `platform/middleware.go`: recovery, request logging, request
      ID — wire into the existing `publicapi` engine now (wiring into
      `bff` happens in task-4, once that engine exists). Verify the
      rewritten guardrail by attacking it, not by reading it green (see
      `_goal/GOAL.md` Done-when 1 for the three specific violations to
      construct — a fake new module with a bad import, platform importing
      a domain, a domain file importing transport). Also verify `docker
      compose up` still works post-restructure — not assumed, the last
      person to actually check this was Luna, before four rounds of fixes
      landed on top; nobody has re-verified it since.
      **Expected failing at this point:** `TestI11_`…`TestI14_` don't
      exist yet — those land in tasks 4/5. **Do not modify
      `internal/invariants_test.go` or write stub tests to make them
      pass** — verify only this task's own tests plus I1–I10 (already
      green before this milestone) still hold.
      **Owns: Done-when 1, 2, 16.**
- [x] task-2: Fork-safe client registration — `register-<service>.sh`
      placeholder template (`<SERVICE_NAME>`, `<PORT>`, …, envsubst-style,
      mirrors `hydra.yml.template`'s pattern). `docs/GETTING-STARTED.md`:
      make running it Step 1 (not optional-by-omission — a service with
      no client is a dead login path, local-only dev included), add the
      explicit key-path/env-var rename line to the existing rename
      checklist (the fork-collision Clara found — two forks that both
      skip it silently overwrite each other's key files).
      **Expected failing at this point:** same as task-1 — `TestI11_`…
      `TestI14_` still don't exist. Not this task's regression; don't
      touch `invariants_test.go` to silence it.
      **Owns: Done-when 4, 5.**
- [x] task-3: JWKS rotation fix — `internal/identity/jwt.go`: on `kid`
      not found in the cached set, force exactly one `Cache.Refresh()` +
      retry, then fail (not zero, not a loop — a random-`kid` probe must
      not be able to force repeated issuer calls). Test against a fake
      JWKS endpoint (`httptest`) simulating a rotated key. `docs/
      DEPLOY-REQUIREMENTS.md`: state the rotation behavior explicitly, so
      a forker knows they don't need `sso-consumer-contract.md` §7.4's
      restart-after-rebuild expectation anymore.
      **Expected failing at this point:** same as task-1/2 —
      `TestI11_`…`TestI14_` still don't exist. Not this task's
      regression; don't touch `invariants_test.go` to silence it.
      **Owns: Done-when 6.**
- [x] task-4: Owner login (BFF) — see `task-4.md` (spec below; this is
      the milestone's densest task, same reasoning milestone-1 gave task-2
      a spec: a library integration with no Go reference on this fleet to
      copy, several judgment calls already made during planning that need
      to be implemented precisely, not re-derived). `internal/transport/
      bff`: `GET /login`, `GET /callback`, `GET /` (minimal authenticated
      todo view) per `_contract/API.md`. Wire `platform/middleware.go`
      into the `bff` engine too, completing task-1's Done-when 3.
      **Also owns Done-when 9** (Clara's finding: auth mechanics alone
      don't prove the view renders anything — an integration test seeds
      an owner session + a todo, hits `GET /`, and asserts the todo's
      title is in the response body, not just that the route returns
      200) — see `task-4.md`'s "the minimal view itself" section.
      **Expected failing at this point:** this task lands I11/I12 —
      `TestI13_`/`TestI14_` (rotate ordering, resolver) still won't exist
      until task-5. Confirm I11/I12 now pass; I13/I14 remaining red is
      not this task's regression.
      **Owns: Done-when 3, 7, 8, 9.**
- [x] task-5: Key lifecycle finish — `rotate` CLI command
      (issue-new-then-disable-old ordering, I13), key-file + resolver
      script ported verbatim from `~/.my-task/bin/key` (I14 — the
      empty-argument guard, the fallback chain, the `0600`-is-a-rule
      wording, all with their original reasoning comments, not rewritten
      from description). Apply the same file-writing to `issue` too
      (currently stdout-only). **This is the last task landing an
      I11–I14 test** — after this one, `go test ./...` should be fully
      green for the first time this milestone. Confirm it, but the
      binding final-suite gate is task-8's (below), since three more
      tasks still touch the repo after this one and could regress it.
      **Owns: Done-when 10, 11.**
- [x] task-6: e2e smoke script — real HTTP against a live instance with a
      real minted key (`cmd/smoke` or similar, mirrors my-task's
      `smoke-api-v1.ts` in purpose, not in its task-specific assertion
      list — this template's domain is todos, not my-task's tasks/
      projects/labels). Auth negatives (no credential, spoofed actor
      field in body/query/header), real CRUD round-trip. This is what
      finally exercises `resolveActor`-equivalent → key lookup →
      database for real — every existing test injects the actor directly.
      **Owns: Done-when 12.**
- [x] task-7: Seed script — idempotent (check-then-insert per natural
      key, not blind upsert), creates the owner row (`SEED_OWNER_
      SSO_SUBJECT` from config — see `_rules/_contract/DATA_MODEL.md`'s
      "Owner provisioning" note, no JIT) and nothing speculative beyond
      what the contract mandates.
      **Owns: Done-when 13.**
- [x] task-8: Companion `<service>-api` skill doc — scaffolded from
      `my-task-api`'s shape (base URL, auth, invariant rules, endpoint
      table + worked examples, `references/` split for endpoints vs
      errors). Must carry, as identifiable sections not buried in prose:
      the indistinguishable-401 warning ("when you see a 401, look back
      at what the key command itself printed") and the `0600`-is-a-rule-
      not-isolation note (every crew on this host shares a uid). **Last
      task in the milestone — also runs the final `go test ./...`,
      confirming the full I1–I14 set holds (Done-when 15) with nothing
      from tasks 6/7 having regressed what task-5 landed.**
      **Owns: Done-when 14, 15.**

- [ ] task-9: Fix findings from Clara's fifth blind fork test (2026-08-13,
      domain `my-quotes`, forked from the post-restructure branch). Not
      mergeable as-is — four blockers plus the worst finding on this
      project: `cmd/smoke` is 100% broken after a by-the-book fork (41
      stale `/todos` references, `title`/`done` payloads) while `go
      build`/`go test ./...` stay fully green — invisible to build
      because the paths are strings, invisible to test because `cmd/
      smoke` has no test files. Real fork run: `3/7 passed` on the exact
      check (I1/I3 over real HTTP) task-6 exists to guarantee. Same shape
      as every prior guardrail hole this project has found, one directory
      over from where the others were caught.

      **Fix in the template, not just the doc, per Clara's explicit call
      — a shape task-1's blind-test rounds already established (the
      architecture guardrail was fixed in code, not documented around):**
      - **Blocker A**: `compositeServer`/`newIntegrationRouter`/
        `createAgentWithKey` are documented as living in `publicapi_
        testutil_test.go` but actually live in `todo_handler_test.go` —
        the file Step 8 deletes on fork. Move the harness for real.
      - **Blocker B**: `todo_handler.go` holds package-shared code
        (`newAPIError`, `unauthorizedError`, `actorID`, the package doc
        comment) every handler needs. `rm`-ing it per Step 8 breaks the
        package. Move to `middleware.go` or a new `publicapi.go`.
      - **Blocker C**: copying a handler into the same package
        redeclares those same shared helpers, plus test function names —
        resolves once B moves them out of any single domain handler
        file, but verify, don't assume.
      - **Blocker D**: `internal/transport/bff`'s own test files
        (`view_handler_test.go`, `bff_testutil_test.go`) import the todo
        domain too — the doc's build-vs-test-trap warning only covers
        `publicapi`, not `bff`. Extend it.
      - **`cmd/smoke`**: needs a visible, costly fork step of its own —
        not offered as build-passes-so-it's-fine. Consider restructuring
        the domain-specific literals into an obviously-marked edit zone
        at the top of the file, on top of an explicit `GETTING-STARTED`
        step stating what's lost if it's skipped (mirrors the
        `Idempotency-Key`/pagination "state the cost" pattern already
        used elsewhere in the doc).
      - **View-handler consequence, stated not implied**: stripping
        `internal/transport/bff/view_handler.go` on fork (offered as an
        equal option today) silently drops **both** Done-when 9's tests
        **and** `TestI12_BFFSessionNeverResolvesToAgent_ViewMiddleware` —
        found by the test agent reading the test files directly, not the
        doc, since the doc never said. Don't offer strip as an equal
        option; state the cost, same shape as the Idempotency-Key note.
      - **Done-when 12 granularity gap** (probe, not one of the four
        blockers, still real): renaming only `repo_test.go`'s `TestI3_`
        leaves `service_test.go`'s covering the per-module check —
        checked per-module, not per-layer, weaker than the invariant's
        actual current shape (this codebase already writes
        `TestI3_Repo.../Service.../Handler...` as three separate tests
        per module). Decide: strengthen to layer-aware, or document the
        limitation explicitly (mirrors the existing name-not-body
        limitation) — Luna's call unless Clara wants to weigh in, given
        the risk of a fragile layer-inference heuristic.
      - **Also**: `.claude/skills/my-template-api/` has 33 stale `/todos`
        references and isn't on any rename/delete list — it's the
        agent-facing contract for a forked service · `internal/
        invariants_test.go`'s own failure message says `internal/quote`
        instead of `internal/domain/quote` (stale error-message
        construction, a code fix) · Step 5's heading still says "two
        locations" while its own body now names a third · `openapi.yaml`'s
        keys description says "same 404 rule as todos" — a shipped API
        string, not internal prose, needs to be domain-agnostic.

      **Owns: none new — hardens what already-closed Done-when items
      actually guarantee.** Blind test 6 after this lands.

- [ ] task-10: Fix findings from มายด์'s actual acceptance attempt
      (2026-08-13) and Clara standing up a real dev instance for it — the
      first time anything, human or agent, tried a real browser login.
      **มายด์'s login failed in the first minute**, and it's the reason
      no test ever caught it: five blind fork tests, eight tasks, sixteen
      Done-when items, and not one of them ever completed a real browser
      round-trip, because nothing in an automated suite can.

      **The bug**: `internal/transport/bff/middleware.go`'s cookie-setting
      hardcodes `Secure: true` unconditionally. Safari refuses to store a
      `Secure` cookie over plain `http://localhost`; Chrome happens to
      allow it (treats `http://localhost` as a trustworthy origin), which
      is why this went undetected — it's silently browser-dependent, and
      มายด์'s own setup (`SERVICE_PUBLIC_URL=http://localhost:8080` over
      an SSH forward, Safari) hit the failing case. Without a valid
      `oauth_state` cookie (which carries the PKCE verifier), the
      callback can't validate anything and the flow dead-ends.

      **มายด์'s decision (asked live, blocking him): derive `Secure` from
      the configured URL's scheme** (`SERVICE_PUBLIC_URL`/`AUTH_AUDIENCE`)
      rather than a separately-settable flag — ties it to the same value
      already driving audience/redirect config instead of an independent
      knob someone could ship in the wrong position. `http://localhost`
      keeps working with zero extra setup; a real (always-https)
      deployment is unaffected. Update `GETTING-STARTED.md`/
      `DEPLOY-REQUIREMENTS.md` to say the owner-login flow's cookie
      behavior now follows the configured scheme, so a forker
      understands why local http "just works" without assuming it's
      always safe to assume that.

      **Also fold in, found in the same session, both real, neither
      blocking on their own:**
      - **`scripts/register.sh` prints `SSO_CLIENT_SECRET` to stdout** —
        correct design for a human (per hestia's §6 reasoning: writing it
        to a file is the footgun, not printing it), but **wrong for an
        agent**, whose stdout is its own context and can't be un-read.
        On this fleet, most fork setups will be agent-run. Add a warning
        to the script's own output and to `GETTING-STARTED.md`'s Step 1:
        if an agent ran this, the secret is now in that agent's context
        and should be rotated afterward — point at whatever
        decommission/rotation path hestia's contract update adds.
      - **The owner has no supported way to create a todo.** Public API
        rejects owner Bearer tokens by design (I2); the BFF is
        read-only (`GET /login`/`GET /callback`/`GET /`). Clara had to
        `INSERT` directly to give มายด์ anything to look at. Not adding
        write endpoints this milestone (scope, มายด์'s call, not asked
        for) — but state this as a known limitation rather than let the
        next person discover it the way Clara did, and record the open
        design question: if the owner never holds an API credential and
        the BFF never writes, who creates the owner's data — presumably
        agents acting on the owner's behalf, my-task's actual answer, if
        that's the intended pattern here too, say so rather than let the
        view read as an unfinished stub.

      **Owns: none new — fixes a real login-blocking bug found by the
      first real user, plus two documentation gaps found standing the
      first real instance up.** Blind test 6 (already planned) should
      also include a real browser login attempt this time, not just an
      API-key path, if that's practical for a context-free agent to
      attempt.

## Open items, not owned by any task above

- **`docs/DEPLOY-REQUIREMENTS.md`** needs updates threaded through
  task-2 (registration script), task-3 (rotation behavior), task-4
  (Hydra client fields for the owner-login audience) — not a separate
  task, but don't let any of the three skip its slice.
- **Human acceptance** (`_goal/GOAL.md`) is มายด์'s to run once task-2 and
  task-4 land and a real fork is deployed with a registered client — not
  scheduled here, tracked in `_goal/GOAL.md`'s Human acceptance section.
- If the owner-login path is exercised for real against
  `sso-uat.thadaw.com` during human acceptance, that run is the first real
  consumer of the caveat hestia wrote into the SSO contract at commit
  `b8a4bf9` (per Clara, not a blocker, worth the runner knowing in
  advance).
