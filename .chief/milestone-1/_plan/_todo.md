# Milestone 1 TODO

See `../_goal/GOAL.md` for objective/scope/done-when and
`../_contract/` for API/DATA_MODEL/INVARIANTS. Order matters — each task
depends on the one before it. Every Done-when item is owned by exactly one
task below — none are left to "the full test suite passing" as an implicit
catch-all (per Clara's review: an unowned stopping condition can close a
milestone without anyone ever having deliberately done it).

- [x] task-1: Scaffold — `cmd/server`; `internal/platform/` (config via
      `caarlos0/env/v11` + `godotenv`, logging via `slog`+`tint`, db-open,
      server wiring); empty `internal/todo/` and `internal/identity/`
      directories (populated by task-2/task-3); a health endpoint; goose
      wired to an empty migration; sqlc wired to an empty query file.
      Layout follows `.chief/_rules/_standard/ARCHITECTURE.md` (module-first
      — see that doc for why, and the rationale under "Resolved during
      grill" below). Write the import-graph test now, against the empty
      skeleton, so it fails immediately if a later task puts a file in the
      wrong place instead of surfacing at task-5.

      **Also pin the codegen toolchain — none of sqlc/goose/oapi-codegen
      exist on this machine (verified: not on `PATH`, `~/go/bin` doesn't
      even exist), and Go's install-latest-if-missing behavior is worse than
      failing outright: Done-when 3 requires `sqlc generate` to be
      byte-reproducible, which an unpinned version can't guarantee — the
      exact failure mode in the fleet's `generated-output-must-match-its-
      source-revision` learning.** Use a `tools/tools.go` (`//go:build
      tools`) blank-importing each tool's `cmd` package, so exact versions
      live in `go.mod`/`go.sum` — not on whichever machine happens to run
      the loop. A Makefile/script target `go install`s from those pinned
      versions. `go build ./...` and `go vet ./...` pass.
      **Owns: Done-when 1, 11.**
- [x] task-2: Identity — `internal/identity/`: `users`/`api_keys`
      migrations + sqlc queries (DATA_MODEL.md), `handler.go` +
      `middleware.go` (actor-resolution: API key → JWT → reject, per
      INVARIANTS.md I1–I2, I6–I10) + `service.go`/`repo.go`, the CLI
      key-issuance script. Unit tests for every invariant in that list,
      including I2 on both credential paths, and that every failure mode
      (missing/malformed/expired/revoked/wrong-role) returns the identical
      401 body (I5).
      **Owns: Done-when 5, 6.**
- [x] task-3: Todos — `internal/todo/`: `todos` migration + sqlc queries
      (last new table this milestone adds — nothing after this task changes
      the schema), `openapi.yaml` for `/me` and the `/todos` endpoints,
      oapi-codegen + gin-middleware wiring, `handler.go`/`service.go`/
      `repo.go` (ownership scoping, I3, I4 — `handler.go` must not import
      the sqlc package directly, only through `repo.go`, per the
      architecture standard). Integration tests against a real SQLite file.
      Verify `goose up` applies cleanly on a fresh empty file with the full
      migration set. This is also the task after which every invariant
      I1–I10 should have at least one test named after it (task-2 supplies
      I1,I2,I5–I10; this task supplies I3,I4) — write the grep-based check
      now and let it confirm that, rather than assuming it.
      **Owns: Done-when 2, 4, 7, 12.**
- [x] task-4: Settings — `GET /api/v1/keys`, `DELETE /api/v1/keys/:id`
      added to `openapi.yaml` and implemented inside `internal/identity/`
      (new sqlc queries against the existing `api_keys` table — last task
      this milestone adds queries in, so this is where "sqlc generate
      produces no diff" gets verified for the complete set). Same
      ownership-scoping rule as todos. Tests for I3 applied to keys, I9
      (expired-but-unrevoked still 401s).
      **Owns: Done-when 3.**
- [x] task-5: Ship it — Dockerfile, docker-compose (service + SQLite
      volume), `docs/DEPLOY-REQUIREMENTS.md`, `docs/GETTING-STARTED.md`.
      The getting-started doc's fork checklist must explicitly call out the
      two deliberate simplifications from `_contract/API.md` Conventions
      (no Idempotency-Key, no pagination) as things to reconsider if a fork
      adds an event log or a list that can grow large — a forker reads this
      doc, not a stale milestone's API.md. Full `go test ./...` and
      `docker compose up` smoke pass.

      **Also fix `internal/invariants_test.go`'s Done-when-12 check before
      closing this task — Clara found it hardcodes `for n := 1; n <= 10`
      instead of deriving the required set from `_contract/INVARIANTS.md`
      itself.** A fork that adds `I11` to `INVARIANTS.md` without touching
      this test file gets a check that stays green forever while I11 has no
      test — silent, in the exact direction `ARCHITECTURE.md`'s own
      allowlist reasoning warns against (fail loud on an unrecognized case,
      not quietly pass it through). Fix: parse `INVARIANTS.md` for
      `**I<N> —` headings and use that set instead of a hardcoded range; if
      parsing isn't practical, at minimum fail when the count of those
      headings disagrees with the hardcoded bound, so drift is loud instead
      of invisible. Add a line to `GETTING-STARTED.md`: "a new invariant
      needs both an `INVARIANTS.md` entry and a `TestI<N>_` test — the
      check enforces the second half, not the first."
      **Owns: Done-when 8, 9, 10.**

- [x] task-6: Fix findings from Clara's first blind fork test (2026-08-12) —
      a context-free agent forked the template into `my-notes` using only
      `docs/GETTING-STARTED.md` and succeeded, but only by repairing things
      the doc never warned about. Its own summary: *"the doc says what to
      rename, not what to repair — and all the friction is in repair."**
      **P0 (a guardrail bug, not a doc gap):** `internal/architecture_test.go`
      hardcodes `domainDirs`/`forbidden` to exactly `{internal/todo,
      internal/identity}` in both architecture tests. A fork that keeps
      `internal/todo` and adds `internal/note` beside it gets **zero**
      import-rule coverage on the new module — the guardrail we proved
      fails closed three separate times does not survive the one thing
      this template exists for. Fix: derive the domain module list from the
      filesystem (every `internal/*` except `api`, `db`, `platform`), same
      pattern already used for Done-when 12's invariant list. Also fix
      GETTING-STARTED Step 1's claim that this file "does not need editing"
      on fork — true only for the module-path case; state precisely what's
      now dynamic (module path AND domain module discovery) instead.
      **P1:** (a) Step 4 (delete `internal/todo`) doesn't warn this deletes
      the I3/I4 tests, breaking the Done-when-12 check silently unless the
      fork adds replacement tests for those invariant numbers or removes
      the `INVARIANTS.md` entries — extend the "Invariants: two things, not
      one" section to cover the removal direction, not just addition.
      (b) `TestGooseUp_FullMigrationSetAppliesCleanly` (Done-when 2's only
      check) lives inside `internal/todo/migration_test.go` — deleted
      silently with the domain, no invariant name ties it to anything that
      would complain. Move it somewhere fork-safe (`internal/platform/`,
      alongside `platform.Migrate` which it exercises — doesn't need any
      domain-module import). (c) No Prerequisites section — `bin/` is
      gitignored and nothing tells a fresh `git clone` to run `make tools`
      before anything else; add one. (d) No section on how to actually run
      what you forked — add one (`make tools`, `go run ./cmd/server` or
      `docker compose up`, `cmd/issue-key` as an actual command not a
      parenthetical, `/healthz`).
      **P2:** `make generate` doesn't clean orphaned generated files after a
      domain's queries are removed (fix the Makefile target if safe, else
      document the manual step) · Step 3 reads as "edit a file" when
      `AUTH_AUDIENCE` is deploy-time env with no file to edit and no
      `.env.example` in the repo — clarify · add a `.chief/` section to
      GETTING-STARTED (what a fork does with it) and make
      `internal/invariants_test.go` glob for `INVARIANTS.md` under `.chief/`
      instead of hardcoding `milestone-1`, so the two reinforce each other
      instead of one silently depending on the other · fix
      `DEPLOY-REQUIREMENTS.md:98`'s example still using
      `my-template.thadaw.com` · list `DEPLOY-REQUIREMENTS.md` itself among
      Step 2's rename targets · Step 3 points at
      `~/gits/prod-thw-home/docs/sso-consumer-contract.md`, a path outside
      this repo that won't exist on a fork made elsewhere — inline the
      essential §6 summary and mark the reference as fleet-internal.
      **Also tighten `_goal/GOAL.md`'s Human Acceptance criterion**: Clara's
      own test let the agent read source (correctly — a real forker would
      too), which is why it measured "did it succeed" rather than "how much
      friction," and only produced a useful signal because she separately
      instructed it to report friction. Scope the criterion explicitly: no
      need to read Go source **outside the domain module you're replacing
      `internal/todo` with** — reading further than that to succeed is
      itself the signal a doc gap exists.
      **Owns: none (all 12 Done-when items already closed by task-5) — this
      hardens what they check, it doesn't add new ones.** Re-run the blind
      fork test with a fresh context-free agent after this lands (the first
      one is contaminated).

- [x] task-7: Fix findings from Clara's second blind fork test (2026-08-12,
      real `git clone`, domain `bookmarks`, stricter criterion). Forked
      successfully, `go test ./...` green — but the agent's own words:
      *"the green suite is partly a lie."* Full detail is in Clara's ship
      message; this entry has the essentials.

      **P0-1 (a genuine security hole, not a doc gap):**
      `TestDoneWhen12_EveryInvariantHasANamedTest` greps for `TestI<N>_`
      **anywhere in the repo**. `internal/identity` already has a
      `TestI3_..._Keys` test, so it satisfies I3's requirement for the
      **entire repo forever** — a forked domain module can ship with zero
      ownership-scoping tests of its own and the suite stays green (proven:
      Clara's agent renamed all three `TestI3_` tests out of its new
      `internal/bookmark` module and the suite still passed). This is the
      review's own (c) design from earlier in the milestone — grep for a
      name, not for where it lives — and the flaw is in that design, not in
      how it was implemented.

      Fix: `_contract/INVARIANTS.md` gains a `scope:` tag per invariant —
      ``**I3 — ...** `scope: per-domain-module` `` for I3/I4, `scope:
      global` for I1/I2/I5–I10 (or an equivalent explicit marking — your
      call on exact syntax, but it must be parsed, not just prose). For
      every invariant tagged `per-domain-module`, the check must require a
      `TestI<N>_` **inside that specific domain module's own package** —
      reuse `domainModuleNames()` from the earlier architecture-guardrail
      fix (P0 of task-6) rather than inventing a second way to enumerate
      domain modules. Global-scope invariants keep the existing
      repo-wide-grep behavior.

      **Known consequence to handle, not a surprise to discover later:**
      once this lands, `internal/identity` — itself a domain module under
      `domainModuleNames()` — will need its **own** dedicated `TestI4_`
      test (table-isolation for `users`/`api_keys`), since
      `TestI4_TodoRepoOnlyQueriesTodosTable` living in `internal/todo` only
      covers `todo`'s own table today and (per Clara's finding) is
      apparently the *only* thing incidentally covering table isolation
      more broadly. Write that test now, in `internal/identity`, rather
      than letting the stricter check surface it as a surprise failure.

      **Document the residual limitation precisely once fixed**: the check
      only looks at test *names*, not test bodies — an empty
      `func TestI3_Foo(t *testing.T) {}` still satisfies it. Once
      per-module, writing an empty one is a deliberate lie, not an
      accidental miss — say this in `GETTING-STARTED.md` so it isn't
      mistaken for a stronger guarantee than it is.

      **P0-2 — Step 4's order actively hurts.** The agent called it *"the
      single worst instruction in the document."* Deleting `internal/todo`
      as literally the first sub-step removes the only worked example of:
      how `handler.go` implements the generated `ServerInterface`, the
      `Repository` interface + fake-repo pattern, and the full integration
      test harness (`identity.NewService(repo, repo, nil, nil)` — two nil
      args nothing explains, `RejectActorFields`/`RequireActor` wiring,
      `HashAPIKey`/`CreateAPIKey` test fixtures, the `compositeServer`
      embedding trick). None of this is in `GETTING-STARTED.md` at all —
      the test agent only survived by reading `internal/todo` to
      completion *before* deleting it, which the doc never told it to do.
      Fix: reorder Step 4 to **study `internal/todo` fully → copy it into
      the new module and rename → delete the old one** (not delete-first),
      and add a short "patterns worth preserving" list covering at least
      the `identity.NewService` argument meanings and the `compositeServer`
      embedding trick, since those are flagged as unguessable.

      **P1:**
      - Option (b) in the invariants section ("remove the `INVARIANTS.md`
        entry instead of writing a new test") can walk someone into
        deleting `internal/identity`'s only table-isolation coverage
        (`TestI4_TodoRepoOnlyQueriesTodosTable`, per the P0-1 finding) even
        though the doc says to keep `internal/identity` as-is. Narrow
        option (b), or reframe it as "re-home the test to your new module"
        rather than "delete the requirement" as an equally-valid choice.
        (Note: once P0-1's per-module fix lands, this mostly resolves
        itself — but state it correctly regardless.)
      - `docker compose exec app /app/issue-key` and
        `DEPLOY-REQUIREMENTS.md:63` hardcode the compose service key `app`
        even though Step 2 tells forkers they may rename it — add the
        caveat inline wherever `app` appears in a run command.
      - Step 1 now contradicts itself: says 10 files including
        `architecture_test.go` need the import path changed, then two
        sentences later says that file doesn't need editing. True cause:
        the module path string only appears in that file's **doc
        comment**, not its logic — pick one consistent story (e.g., "9
        files need functional changes; the 10th's only occurrence is a
        comment — harmless either way, but keep it accurate if you touch
        it") instead of leaving two sentences that disagree.
      - `/healthz` returning 200 can be a stale process still holding the
        old port, not proof the fork actually started — a real failure
        mode the agent hit (`bind: address already in use`) that a curl
        200 alone can't distinguish from success. Tell the reader to
        confirm the server's own startup log line first (verify one
        exists — add it if it doesn't), not just curl the health check.

      **P2 (do if time allows, not blocking):** document that `openapi.yaml`
      `operationId` values name the generated `ServerInterface` methods,
      and that sqlc capitalizes `url` → `Url` not `URL` (both currently
      discoverable only via compiler error) · document the migration
      authoring convention (`<14-digit-timestamp>_<name>.sql`, `-- +goose
      Up/Down` markers, `bin/goose create`, and that a new table needs
      `owner_id TEXT NOT NULL REFERENCES users (id)` for the ownership
      model to apply — implied throughout but never stated as a hard
      requirement) · a short identity-seam cheat sheet for writing
      integration tests · note that even a correct fork leaves ~20
      dangling `todo`/`todos` references in comments and fixtures
      (`internal/identity/doc.go`, `internal/platform/config.go`,
      `internal/api/validator.go`, fixtures like `{"title":"a todo"}`) —
      add a step telling forkers to grep the whole tree including doc
      comments, not just functional code, since the doc's own opening
      promise ("skipping a step leaves a specific, identifiable trace")
      is undermined by a trace that's present either way.

      **Owns: none new — hardens Done-when 12's actual guarantee and closes
      a real gap in what it was believed to cover.** Fix P0-1 and P0-2
      before anything else; P1 next; re-run a third blind fork test with a
      fresh context-free agent once landed.

- [x] task-8: Fix findings from Clara's third blind fork test (2026-08-12,
      domain `snippets`). **Clara's stated stopping criterion: this is the
      last required round** — fix all P0 + P1 here, run a fourth
      *confirmatory* (not new-gap-hunting) blind test, and stop if it forks
      by following the doc in order with no new security-severe P0. P2 is
      optional polish, not a gate. Full detail in Clara's ship message;
      essentials below.

      **P0-1 — Step 4's "copy compiles and passes as a copy" checkpoint
      doesn't exist.** A renamed copy immediately references undefined
      generated types (`api.Snippet`, `db.CreateSnippetParams`, etc. don't
      exist yet). Rewrite the flow: copy + rename package/type/dir names
      but **keep todo-shaped field names for now** → add the new
      migration + sqlc queries + `openapi.yaml` paths/schemas (new
      `operationId`s) in the same pass, still todo-shaped → `make
      generate` → **real checkpoint**: green here proves wiring is correct
      while behavior is still todo's → only then change fields to the real
      domain shape.

      **P0-2 — the wire-in-before-delete window (task-6's own fix) forces
      two domain modules to coexist, which breaks two ways every time:**
      (a) `apiServer` embeds both `*todo.Server` and `*snippet.Server` —
      Go names an embedded field after its type, and this repo's own
      convention names every module's adapter type `Server`, so this is a
      guaranteed `Server redeclared`, not an edge case (contrast with the
      doc's existing warning about `operationId` collisions, which are
      possible but avoidable — this one isn't). Needs a documented
      workaround (e.g. `type snippetServer = snippet.Server`) since the
      doc currently claims embedding "doesn't need its own composition
      mechanism," which stops being true the moment two modules coexist.
      (b) each module's integration tests must satisfy the *entire*
      `api.ServerInterface`, so adding a second module breaks the first
      module's existing tests too, and the new module's tests won't
      compile without embedding the other module's `Server` — a
      cross-module test dependency in a repo whose architecture rule is
      that domain modules stay independent. State plainly in Step 4 that
      both of these are expected, temporary, and resolve at the delete
      step — with the workaround for (a).

      **P0-3 — Step 4's delete list never mentions `openapi.yaml`.** It
      lists the migration, sqlc queries, `make generate`, and the
      `main.go` registration — not the `/todos` paths or the
      `Todo`/`TodoList`/`CreateTodoRequest`/`UpdateTodoRequest` schemas in
      `openapi.yaml` itself. Following the checklist exactly leaves them
      in place, `oapi-codegen` regenerates the old methods, and
      `apiServer does not implement api.ServerInterface (missing method
      CreateTodo)` breaks the build. Add it to the list — and because the
      doc already (correctly) says `make generate` cleans up stale
      `internal/db` output automatically, a reader reasonably assumes
      codegen "handles cleanup" generally; say explicitly that
      `openapi.yaml` itself is not auto-cleaned, it's a manual edit.

      **P0-4 — a hardcoding regression in the fix task-7 JUST added, same
      bug shape as P0-1 of task-7, one level deeper.**
      `internal/identity/repo_test.go`'s `TestI4_IdentityRepoOnly...`
      calls `assertQueryFileReferencesOnlyTables(t, ".../users.sql",
      "users", []string{"todos"})` — the forbidden-table list is a
      **hardcoded `"todos"`**. Once a fork deletes that table, this check
      forbids a table that no longer exists and passes vacuously forever,
      checking nothing. Proven: the test agent added `SELECT * FROM
      snippets;` to `db/queries/users.sql` and the "table isolation" test
      still passed. `internal/todo`'s own equivalent
      (`assertQueriesReferenceOnlyTable`, a near-duplicate implementation)
      has the identical shape, hardcoding `["users","api_keys"]` back the
      other way. **Fix: unify the two duplicated helpers into one, and
      derive the forbidden-table set dynamically** — scan every `.sql`
      file under `db/queries/`, extract the table name(s) each one
      references, and forbid whatever the *other* files reference; no
      specific table name hardcoded anywhere. Where the shared logic lives
      is an implementation choice (a small non-domain helper, similar
      standing to `internal/api`/`internal/db`/`internal/platform`, is one
      option) — but there must be exactly one implementation, and the
      actual `TestI4_...` **function** must still live inside each domain
      module's own package (Done-when 12's per-module check looks for the
      function there, not for where its helper logic lives). Verify by
      replaying the exact exploit (inject a foreign-table reference,
      confirm it's now caught) — same method used for every guardrail fix
      this milestone.

      **P1:**
      - The "check only looks at test names, not bodies" limitation reads
        like a mild caveat. State its actual consequence plainly: gutting
        `TestI3_`/`TestI4_` test bodies to `{}` and making `get`/`update`/
        `delete` owner-blind produces a fully green suite (including
        Done-when 12) while one user can read another's rows over HTTP —
        this is what the test agent proved, deliberately, to make the
        point. Not fixable by a name-check (a body-emptying author is
        choosing to defeat their own safety net) — but the doc shouldn't
        undersell what "just a name check" actually means.
      - Dangling-reference count is wrong (doc says "roughly two dozen";
        actual is 45 outside `.chief/`, plus 42 more inside
        `GETTING-STARTED.md` itself that the doc doesn't count), and
        "none of these break anything" is false — some are live assertion
        arguments (see P0-4). Fix the numbers, add `GETTING-STARTED.md`
        itself to the things Step 2 says to check (it already lists
        `DEPLOY-REQUIREMENTS.md`), and correct the "harmless" claim.
      - The dangling-reference grep doesn't exclude `.chief/`, even though
        a separate section says keep `.chief/` as historical record — 94
        of 181 hits are there, so the instruction as written contradicts
        itself. Add `--exclude-dir=.chief`.
      - I3/I4 are referenced ~20 times in `GETTING-STARTED.md` but never
        explained there — their actual text lives only in `.chief/`, and
        the doc offers "remove the entry" as a valid choice without saying
        what's being removed. Add a short paragraph summarizing what I3
        and I4 actually mean, so that choice can be made informed.

      **P2 (optional, not part of the stop/continue gate):** Step 2
      mis-describes `DEPLOY-REQUIREMENTS.md` (it's generic now, the
      description is stale) · "Patterns worth preserving" is positioned
      after the numbered list but tells the reader to read it before step
      1 — move it before the list · Prerequisites section says `rm -rf
      internal/todo` invokes `bin/sqlc` — it's `make generate` that does,
      and tools are needed as early as migration-authoring, not just at
      deletion.

      **Owns: none new.** Re-run a fourth, confirmatory blind fork test
      after this lands.

## Resolved during grill

The JWT/SSO auth path's framing was open (live agent auth vs. a
wired-but-dormant seam per `sso-consumer-contract.md` §3) — **มายด์ confirmed
2026-08-12: dormant seam.** Task-2's JWT-path code and I2/I6/I7/I9/I10 tests
are unaffected (test issuer, not live Hydra). The human-acceptance criterion
asking someone to hit the service with a real Hydra JWT is cut from
GOAL.md. Task-5's `docs/GETTING-STARTED.md` should describe the JWT path as
wired-but-dormant, matching GOAL.md's Context section — not as this
template's live SSO integration.

**Closed 2026-08-12 — hestia confirmed directly against the deployed
docker-compose.yml (main, commit `8126de5`), not the 3-day-old contract
doc:** Hydra's Admin API (4445) is still unauthenticated, bound to
`127.0.0.1` only (not exposed, but any local process can reach it), and the
compose file is shared by thw-home and thw-home-prod — same gap on both
hosts. The fix (unix socket + filesystem permissions) is designed
(`agent-identity-gaps.md` #4) but not built. hestia: do not create a Hydra
client for TPL-1.

**Architecture layout (2026-08-12):** Clara found that no goal/contract/plan
file made the `internal/{handler,service,repo,middleware}` folders mean
anything — no rule, no invariant, no Done-when touched them, so a
passthrough handler→sqlc shortcut would have passed all 10 original
Done-when items. This belongs above milestone-1 (per `AGENTS.md`'s rules
hierarchy, `.chief/_rules/**` outranks a milestone, and this layout must
outlive the todo domain it's demonstrated on) — written to
`.chief/_rules/_standard/ARCHITECTURE.md`. มายด์ chose module-first over
layer-first: `internal/{todo,identity,platform}`, each domain module holding
its own handler+service+repo together, so forking is `rm -rf internal/todo`
in one command rather than knowing a domain also lives in sibling trees.

One refinement made while writing the standard, worth flagging since it
adjusts (not reverses) Clara's proposed mechanism: her two within-module
rules ("todo/identity must not import gin", "handler.go must not import
sqlc directly") can't be checked with a plain `go list -json` **package**
query, because `handler.go` and `service.go`/`repo.go` share one directory
and therefore one import list at the package level — a package-level check
would false-positive on `handler.go`'s legitimate `gin` import. Both rules
are enforced **per-file** instead, via `go/parser`+`go/ast` (still
stdlib-only, no dependency added). Only the third rule
(`internal/platform` importing no domain module) is a clean package-level
`go list -json` check, since platform is its own directory. Full mechanism
in `.chief/_rules/_standard/ARCHITECTURE.md`'s Enforcement section.

**Preflight + commit (2026-08-12):** Clara ran two checks loop-readiness's
static review couldn't — actually probing the machine and the repo's git
state. Neither tool (`sqlc`/`goose`/`oapi-codegen`) exists on `PATH` or in
`~/go/bin` (doesn't exist), and none of today's planning work had been
committed. Fixed: planning work is now `main`'s root commit
(`b988194`) — a clean baseline to diff `chief-loop`'s output against — and
task-1 gained the `tools/tools.go` pinning requirement (Decisions table,
Done-when 1). See `task-2.md` for the identity/auth task spec loop-readiness
called for.

**Doc-vs-enforcement drift (2026-08-12, task-2 → post-review):** task-2's
builder-agent correctly extended `architecture_test.go`'s regex to allow a
handler/repo file's own `_test.go` (needed because Go's test-file naming
collided with the original pattern — the code fix was right), but never
updated `ARCHITECTURE.md`'s prose, which still described the pre-fix
pattern. Clara caught it by diffing the regex against the doc directly, not
by trusting the commit message. Fixed: `ARCHITECTURE.md`'s Enforcement
section now states the real regex and distinguishes the two separate
reasons to touch it (fork rename vs. Go's `_test.go` suffix). **Standing
practice for the rest of this milestone: whenever a task changes an
enforcement mechanism (a test, a generated check, a script), check whether
the doc that defines that mechanism moved with it, in the same commit — not
as a follow-up.** No Done-when item catches this class of drift; it has to
be checked deliberately. task-3/4 are the next likely place it recurs,
since both touch `openapi.yaml` and the schema, and `_contract/API.md`/
`DATA_MODEL.md` are the docs that define those.

Task-5's `docs/GETTING-STARTED.md` should use this exact paragraph for the
JWT seam (per working standard rule 5 — a claim about mutable state carries
its read-time, so a future reader knows to re-check rather than trust it
indefinitely):

> This JWT seam is wired and unit-tested, but **dormant by design**, per
> `sso-consumer-contract.md` §3 — Hydra does not issue machine identity to a
> local process, because its Admin API has no authentication: any process on
> the same host can register a client with a `sub` of its choosing and
> receive a correctly-signed token (`aud` doesn't help either — §7.7).
> **Confirmed against the actually-deployed compose config on 2026-08-12 by
> hestia.** Read §3's "when to revisit" before turning this on.
