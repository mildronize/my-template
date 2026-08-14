# Scope-Tags Fix-Round Report

Not a numbered task — owns no Done-when item, per `.chief/milestone-4/
_plan/_todo.md`'s own entry for this fix-round. Sequenced before task-6.

## The defect (as found and verified by Clara, design already agreed)

`internal/invariants_test.go`'s `scope:` tag mechanism recognized exactly
two values, `global` and `per-domain-module`. `per-domain-module`
conflated two different ideas: I3/I4 genuinely apply to *every* domain
module (a real coverage sweep); I15-I19 and I21 each belong to exactly
*one* specific place (I15-I19 to `internal/domain/todo`, I21 to
`internal/identity`) — an address, not a sweep. This only "worked" by
coincidence because `domainModuleNames()` (what `per-domain-module`
iterated over) had exactly one member (`todo`). A correctly-placed
`TestI21_...` written inside `internal/identity` (what task-6 needs to
write) would **not** have satisfied the old check — it demanded the test
inside `internal/domain/todo` instead, the wrong module — and a stub
`TestI21_...` planted there would have silently satisfied it.

## The fix, implemented as agreed (not redesigned)

`internal/invariants_test.go`:

- New `scope: domain:<name>` tag form. `invariantScopeRe` now matches
  `global|per-domain-module|domain:[A-Za-z0-9_-]+`;
  `requiredInvariantNumbers`'s validation extended to accept it (still
  fails loud on anything else).
- `domainScopePackageNames(root)` — explicit, hand-maintained
  `map[string]string`: `"todo"` -> `internal/domain/todo`, `"identity"`
  -> `internal/identity`. Not derived from a naming convention, since
  `internal/identity` deliberately isn't under `internal/domain/`
  (`ARCHITECTURE.md`'s milestone-2 decision).
- `TestDoneWhen12_EveryInvariantHasANamedTest`'s `domain:<name>` branch:
  looks up `<name>` in that map; a miss is `require.Truef` (loud abort,
  stops the whole test function immediately, not `assert` — the
  unrecognized-name case must not let the loop continue past it as if
  nothing were wrong).
- `perDomainModuleScopePackages(root)` — a **new**, separate, explicit,
  hand-maintained `map[string]string` for I3/I4 only: `"todo"` ->
  `internal/domain/todo`, `"identity"` -> `internal/identity`.
  Deliberately **not** `domainModuleNames()` (`internal/
  architecture_test.go`, left completely untouched) — that function
  answers "what counts as a domain module for fork-restructuring
  purposes," and `internal/identity` correctly failing that question is
  the existing design, not a gap to paper over.
- `TestPerDomainModuleScopeCoversEveryDomainModule` — asserts
  `perDomainModuleScopePackages` is a superset of `domainModuleNames()`'s
  own module names, so a new domain module nobody remembered to add to
  the hand-maintained list fails loudly instead of being silently exempt
  from I3/I4. Comments on `perDomainModuleScopePackages` state plainly
  that it's hand-maintained and what adding a package requires (a
  one-line reason, the same way `internal/identity`'s own entry carries
  one).
- `TestDomainScopePackageNames_UnknownNameDoesNotResolve` — standing
  regression test proving an unrecognized name (e.g.
  `"bogus-nonexistent-name"`) never resolves in
  `domainScopePackageNames`, the same split `TestI15Floor_CanActuallyFail`
  already establishes (logic proven once by direct unit test; the
  full integration abort attacked by hand during review, below).

`.chief/_rules/_contract/INVARIANTS.md`:

- I15, I16, I17, I18, I19: `scope: per-domain-module` -> `scope:
  domain:todo`.
- I21: `scope: per-domain-module` -> `scope: domain:identity`.
- I3, I4: left as `scope: per-domain-module` (unchanged).
- Every other invariant's scope tag: untouched.
- Top-of-file `scope:` tag note extended to document the new
  `domain:<name>` form and `per-domain-module`'s narrowed meaning.

## Attack-and-confirm (the actual deliverable, shown in full)

All three attacks used real edits against the live files, real `go
test`/`go build` runs, and were reverted and confirmed clean before the
commit. Backed up `INVARIANTS.md` to a scratch file first; diffed it
byte-identical after reverting.

### Attack 1 — bogus `domain:<name>` scope tag aborts loudly

Edited I15's heading in `INVARIANTS.md` to `scope:
domain:bogus-nonexistent-name`, confirmed the edit landed (`grep`), ran
`go test ./internal/ -run TestDoneWhen12 -v`:

```
invariants_test.go:441:
    Error Trace: /home/thw-home/gits/my-template/internal/invariants_test.go:441
    Error:       Should be true
    Test:        TestDoneWhen12_EveryInvariantHasANamedTest
    Messages:    _contract/INVARIANTS.md's I15 declares scope "domain:bogus-nonexistent-name",
                 but "bogus-nonexistent-name" is not a known domain name — add it to
                 domainScopePackageNames (internal/invariants_test.go) with an explicit
                 package path, or fix the typo in INVARIANTS.md (GOAL.md Done-when 12)
--- FAIL: TestDoneWhen12_EveryInvariantHasANamedTest (0.04s)
FAIL
```

The `require.Truef` fired and stopped the test function immediately —
invariants after I15 in iteration order were never reached this run,
which is the loud-abort behavior working as designed, not a partial
"some invariants still got checked" pass. Reverted I15's tag back to
`scope: domain:todo`; `diff` against the pre-attack backup showed the
file byte-identical.

### Attack 2 — wrong-location I21 stub rejected, correct-location accepted

First checked whether a real `TestI21_...` exists anywhere yet:
`grep -rln "TestI21_" internal/` returned nothing, and `git log --all -p`
over `internal/identity/{repo,service}_test.go` confirms no `TestI21_`
has ever existed there. So the attack ran in the "no real test yet"
direction the brief anticipated:

1. Appended a temporary stub to `internal/domain/todo/repo_test.go` (the
   **wrong** package for I21):
   ```go
   func TestI21_AttackStubWrongLocation(t *testing.T) {}
   ```
   `go build ./internal/domain/todo/...` — clean. Ran
   `go test ./internal/ -run TestDoneWhen12 -v`:
   ```
   invariants_test.go:450:
       Messages: no test named TestI21_<something> found inside
                 /home/thw-home/gits/my-template/internal/identity (scope: domain:identity)
                 — _contract/INVARIANTS.md's I21 requires a dedicated test in that specific
                 package, not just somewhere in the repo (GOAL.md Done-when 12)
   --- FAIL: TestDoneWhen12_EveryInvariantHasANamedTest (0.04s)
   ```
   Confirmed: the wrong-location stub does **not** satisfy I21 — exactly
   the shortcut the old `per-domain-module` mechanism would have
   silently accepted.

2. With the wrong-location stub still in place, additionally appended a
   temporary stub to `internal/identity/repo_test.go` (the **correct**
   package):
   ```go
   func TestI21_AttackStubCorrectLocation(t *testing.T) {}
   ```
   `go build ./internal/identity/...` — clean. Re-ran
   `go test ./internal/ -run TestDoneWhen12 -v`: the I21 failure is
   **gone** — only the pre-existing I3 (see "Found, not fixed" below)
   and I20 (task-7's job) failures remain. Confirms the correct-location
   stub satisfies the check regardless of the wrong-location stub's
   continued presence — the mechanism checks the specific package, not
   "a `TestI21_` exists somewhere."

3. Reverted both: `git checkout -- internal/domain/todo/repo_test.go
   internal/identity/repo_test.go`; `git diff --stat` on both showed no
   output (clean).

### Attack 3 — superset assertion fires on a missing domain module

Temporarily commented out the `"todo"` entry in
`perDomainModuleScopePackages` (`internal/invariants_test.go`), confirmed
the edit landed (`grep -n "ATTACK-ONLY REMOVAL"`), confirmed
`go vet ./internal/` still compiles clean, then ran
`go test ./internal/ -run TestPerDomainModuleScopeCoversEveryDomainModule -v`:

```
invariants_test.go:313:
    Error: map[string]string{"identity":".../internal/identity"} does not contain "todo"
    Messages: domain module "todo" (domainModuleNames, internal/architecture_test.go) is
              missing from perDomainModuleScopePackages (internal/invariants_test.go) —
              a domain module must never be silently exempt from I3/I4; add it to that
              map explicitly, with a one-line reason
--- FAIL: TestPerDomainModuleScopeCoversEveryDomainModule (0.00s)
FAIL
```

Reverted the `"todo"` entry; re-ran the same test:

```
=== RUN   TestPerDomainModuleScopeCoversEveryDomainModule
--- PASS: TestPerDomainModuleScopeCoversEveryDomainModule (0.00s)
PASS
```

### Bogus-scope-tag-aborts-loudly standing test

`TestDomainScopePackageNames_UnknownNameDoesNotResolve` (permanent,
committed) exercises the underlying property directly — the same split
`TestI15Floor_CanActuallyFail` already uses for I15's own floor:

```
=== RUN   TestDomainScopePackageNames_UnknownNameDoesNotResolve
--- PASS: TestDomainScopePackageNames_UnknownNameDoesNotResolve (0.00s)
```

The full end-to-end abort (`TestDoneWhen12` itself failing given a bogus
tag in the real `INVARIANTS.md`) is Attack 1 above, done by hand, not
baked into the suite as a standing repo mutation.

## Found, not fixed — flag for Clara/มายด์

Two real, pre-existing issues surfaced during verification. Neither is in
`internal/invariants_test.go`, `internal/architecture_test.go`, or
`INVARIANTS.md` (my assigned files), so I did not touch either — reported
instead of worked around.

**1. `internal/identity` has never had a `TestI3_...` test.** The
fix-round's own brief stated as a "confirmed" premise that
"`internal/identity` has real, existing `TestI3_`/`TestI4_` tests that
the current mechanism never looks at." I verified this against the
actual codebase: `internal/identity` has `TestI4_
IdentityRepoOnlyQueriesUsersAndAPIKeysTables` (real), but **no**
`TestI3_...` test — `git log --all -p` over every commit that ever
touched `internal/identity/{repo,service}_test.go` shows no `TestI3_`
has ever existed there. I3's actual "an agent's own key-listing stays
scoped to itself" coverage lives in
`internal/transport/{bff,publicapi}/keys_handler_test.go` instead (HTTP-
level, where key-listing is actually exercised), not inside
`internal/identity`'s own package.

Because `perDomainModuleScopePackages` correctly (per the agreed design)
now includes `internal/identity` for I3, `TestDoneWhen12` now also fails
on I3, specifically demanding a test literally inside `internal/
identity`'s own package:

```
invariants_test.go:424:
    Messages: no test named TestI3_<something> found inside
              /home/thw-home/gits/my-template/internal/identity's own package —
              _contract/INVARIANTS.md's I3 (scope: per-domain-module) requires a
              dedicated test in every package I3/I4 apply to
              (perDomainModuleScopePackages), not just somewhere in the repo
              (GOAL.md Done-when 12; task-7)
```

This is a **real, previously-invisible gap that this fix correctly
surfaces**, not a regression this fix introduces — the old mechanism
never scanned `internal/identity` at all (that was the whole reason
`domainModuleNames()`-driven `per-domain-module` was unsafe), so this gap
existed on every prior commit too. It grows the milestone's stated
known-red baseline (`_todo.md`'s "`TestDoneWhen12` failing... specifically
and only on I20 and I21") by one item: I3. Per `_todo.md`'s own protocol
("if a task finds it genuinely needs to grow the baseline, that's a
report to Clara, not a unilateral edit here"), I did not add a test or
alter the design to paper over this — that's a design call (does I3 need
a literal `internal/identity`-package test, or does the existing
transport-layer coverage need a different scope tag / mechanism
accommodation?) outside this fix-round's charter and outside my assigned
files.

**2. `TestI4_IdentityRepoOnlyQueriesUsersAndAPIKeysTables` fails on the
current committed HEAD, unrelated to this fix.** Confirmed via `git
status`/`git diff` before I made any edits: this failure pre-exists on
clean HEAD, not introduced by the concurrent task-3 agent's uncommitted
work either. `internal/dbquery/tableisolation.go` appears to parse
words out of `db/queries/todo_events.sql`'s own comments (e.g. "the",
"users" — both appear in prose comments in that file, not as real table
references) and treat them as forbidden-table names belonging to that
file, then flags `db/queries/users.sql` for matching them:

```
Error: /home/thw-home/gits/my-template/db/queries/users.sql must not
       reference table "the" — it belongs to todo_events.sql, ...
Error: /home/thw-home/gits/my-template/db/queries/users.sql must not
       reference table "users" — it belongs to todo_events.sql, ...
```

Out of scope for this fix-round (`internal/dbquery/`, `db/queries/` are
not in my assigned file set); not touched. Flagging since it's a real,
currently-failing test outside the `_todo.md`-stated baseline.

## Green gate — unfiltered `go test ./internal/...`, classified

```
FAIL	github.com/mildronize/my-template/internal
  - TestDoneWhen12_EveryInvariantHasANamedTest: fails on I3 (NEW finding,
    see above) and I20 (baseline, task-7's job). I21 no longer fails —
    this fix-round's stated goal.
?   internal/api, bffapi, db, dbquery       [no test files]
ok  	internal/domain/todo
FAIL	internal/identity
  - TestI4_IdentityRepoOnlyQueriesUsersAndAPIKeysTables (pre-existing,
    unrelated, see "Found, not fixed" #2 above)
ok  	internal/platform
FAIL	internal/transport/bff   [build failed — task-4 hasn't landed;
  expected per _todo.md's own per-task green-gate language, even though
  the "superseding" baseline paragraph names only publicapi]
ok  	internal/transport/publicapi   [task-3's concurrent WIP now builds
  clean as of this run — was still failing to build at the start of this
  session]
```

Classified against the stated baseline ("`TestDoneWhen12` failing
specifically and only on I20 and I21"; "`publicapi/todo_handler.go` not
building until task-3"): **I21 no longer fails (fix-round's goal met)**;
I20 unchanged (still task-7's); publicapi now builds (task-3's concurrent
progress, not this fix-round's doing); one net-new item (I3) and one
pre-existing-but-previously-unlisted item (`TestI4_...` in identity) are
both real and both out of this fix-round's file scope — reported above,
not fixed or hidden.

## Working-directory note

This repo is a **shared local checkout** with the concurrent task-3
agent, not separate worktrees — `git log`/`git status` showed their
commits and uncommitted changes directly, with no `git pull` needed on my
end to see them. A `git stash`/`git stash pop` I ran early to get a clean
`go build ./...` baseline round-tripped their in-flight uncommitted
changes to `internal/api/openapi.gen.go`, `internal/transport/publicapi/
{todo_handler.go,todo_handler_test.go}`, and `openapi.yaml` safely (diff
confirmed byte-identical before/after), but this was a heavier operation
than necessary — from partway through the session onward I switched to
package-scoped `go build`/`go vet` calls instead of whole-tree ones to
avoid the risk. No data loss occurred; noting this so it isn't repeated.

## Commit pushed (branch `milestone-4/activity-log`)

- `0b55d4f` — `fix(milestone-4/scope-tags): add domain:<name> scope form, split I3/I4 from I15-I19/I21`

`git fetch` immediately before push confirmed the branch was exactly
even with `origin` (not behind) before this commit, so no rebase was
needed; `git pull --rebase` was attempted first per process but declined
(`cannot rebase: You have unstaged changes` — the concurrent agent's own
uncommitted files) and was unnecessary once confirmed not-behind. Pushed
directly; remote accepted `a2cca1d..0b55d4f` with no conflicts.
