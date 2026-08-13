# Task task-2 Report

## Task

Domain service layer for the shared-collection todo schema task-1 built:
`todo.Repo`/`todo.Todo` updated for the new columns and the retired
owner-scoping (I3 no longer applies to this domain); `todo.Service`
extended with the single write path for `todo_events` (I15) — idempotency
(I19) → permission (I18, inside the write path) → dispatch → side effect
→ insert, one transaction; the permission layer (`can`, role-based);
I15's own architecture-test fix (the table-specific check, floor-first).
Scope ends at the service layer — `internal/transport/{publicapi,bff}`
still reference the old shape and are tasks 3/4's to fix.

## Outcome

done

## Decisions

- **`Todo`/`Repo` shape**: `OwnerID`/`Done` → `CreatedBy`/`Status`
  (`Status` a typed `string`, four constants, no boolean survives).
  `AssigneeID`/`Priority` are `*string`, `DueDate` is `*time.Time` — all
  nullable, matching `DATA_MODEL.md`. `ListByOwner` → `List` (every
  todo), `GetByIDAndOwner` → `GetByID` (id alone — there is no "wrong
  owner" case left to produce `ErrNotFound` for). One repo method per
  field for updates (`UpdateTitle`/`UpdateStatus`/`UpdateAssignee`/
  `UpdatePriority`/`UpdateDueDate`), mirroring the generated
  `UpdateTodoXxx` queries one-for-one, rather than a single
  multi-pointer `Update`. `Delete` removed from both `*Repo` and the
  `Repository` interface — not left unused, genuinely absent, checked by
  a reflection test (`TestRepo_DeleteMethodDoesNotExist`) so a future PR
  re-adding it (even one nothing calls yet) fails loud.
- **Transaction seam**: `Repo.WithinTx(ctx, func(tx Repository) error)
  error` opens a real `*sql.Tx`, hands the callback a `Repo` bound to it
  (satisfying `Repository`), commits on `nil`/rolls back otherwise.
  Chosen over exposing `*sql.Tx` or the sqlc package to `service.go`
  directly, so `ARCHITECTURE.md` rule 2 stays literally true — only
  `repo.go` ever imports `internal/db`, including for the transaction
  type itself. This was new ground for the codebase: no prior
  transaction-using code existed anywhere in this repo to mirror
  (checked — `internal/identity` has none either), so this shape is
  task-2's own design, not a port of an existing pattern.
- **`WriteEventType` excludes `EventCreated` by construction (I16)**: not
  a runtime check inside `Append` that a future edit could accidentally
  remove, but a fact about the type itself — there is no
  `WriteEventType` value that maps to `"created"`, so `AppendInput{Type:
  EventTypeCreated}` cannot be written by any caller in this codebase.
  `CreateTodo` is the "created" event's one dedicated path, its own
  method, inside its own `WithinTx` call, sharing `Repo.InsertEvent`'s
  seq computation with `Append`. Added a defense-in-depth runtime test
  (`TestI16_Append_RejectsHandCraftedCreatedType`) that hand-constructs
  the underlying string value anyway and confirms `Append`'s dispatch
  switch still has no case for it.
- **`CreateTodo` idempotency**: not explicitly required by any Done-when
  item owned by this task, but `todo_events.client_request_id` is `NOT
  NULL UNIQUE` at the schema level for every row including `"created"` —
  there was no way to give `CreateTodo` a client_request_id without also
  deciding what a repeat means. Gave it the same idempotency shape as
  `Append` (lookup first, inside the same transaction, return the
  original todo and write nothing on a repeat) rather than leaving it
  undefined. Flagged here as a design extension beyond the task's literal
  text, not hidden inside the diff.
- **Permission layer (`permission.go`)**: `can(actor PolicyActor,
  eventType WriteEventType, toStatus Status) bool`, its own file, zero
  imports beyond the package itself — mirrors my-task's `can()`
  (`~/gits/my-task/src/server/lib/policy.ts`) in shape: owner
  unconditional, a small allow-table for the agent role, fail-closed on
  an unrecognised role or event type. `PolicyActor` is a role-only struct,
  deliberately not `identity.User` itself (even though
  `ARCHITECTURE.md` rule 3 would permit `internal/domain/todo` importing
  `internal/identity`), so `can()` stays trivially unit-testable with no
  imports and no database.
- **`Append`'s dispatch payloads**: `{from, to}` JSON pairs for
  `status_changed`/`assigned`, `{field, from, to}` for `field_changed` —
  matches `DATA_MODEL.md`'s stated shape. Assignment payloads carry the
  assignee's user id only (no handle), not an enrichment lookup against
  `users` — `internal/domain/todo`'s repo must only ever query its own
  table for *writes* (I4's "one repo, one table" applied to the write
  side; the read-side `ListEventsFeed` already legitimately joins `users`
  for the feed's actor handle/role, which is a different, already-decided
  cross-table read, not a new one this task added).

## A pre-existing bug found and fixed (in scope: it blocked this task's own repo tests)

`db/queries/todo_events.sql`'s `ListTodoEventsFeed` (task-1) mixed
`sqlc.narg`-generated numbered placeholders (`?2`, `?3` — `sqlc.embed`
appears to reserve a phantom `?1` slot) with a bare anonymous `?` for
`LIMIT`. `modernc.org/sqlite` binds `database/sql`'s positional call
arguments strictly by order to the SQL text's own numbered placeholders,
not by re-deriving numbers from how many arguments were passed — so the
generated Go function's 3-argument call left the query's real `?4`
(LIMIT) unbound, and every call failed with `missing argument with index
4`. Reproduced in isolation first (a 6-line minimal repro against
`modernc.org/sqlite` directly) to confirm the mechanism before touching
the real query, then confirmed the same failure against the actual query
text, then confirmed it via `git stash` that this was **already broken
on `eff83f3` (task-1's merged HEAD), not something this task introduced**
— `repo.go`'s `ListEventsFeed` is one of the five named functions this
task was told to wrap and use, so this blocked `TestRepo_
ListEventsFeed_NewestFirstAcrossTodos_WithJoinFields` outright, not an
optional nice-to-have.

Fix: explicit column list (not `sqlc.embed`) and `sqlc.arg(limit)` (not a
bare `?`), keeping every placeholder inside sqlc's own numbering,
contiguous from `?1`. Regenerated via `bin/sqlc generate`. Also hit (and
fixed the same way task-1's own report did) sqlc v1.31.1's star-expansion
byte-offset corruption from an em dash in my own new doc comment sitting
directly above a `SELECT` line — ASCII hyphens only in that comment now.
Row shape changed as a side effect of dropping `sqlc.embed`
(`ListTodoEventsFeedRow`'s columns are flat, no nested `TodoEvent`
struct) — updated in `Repo.ListEventsFeed`, the one caller.

## Near-miss: a regression this task itself introduced, then caught before reporting done

Rewriting `repo_test.go` wholesale for the new schema silently broke
`internal/invariants_test.go`'s `TestDoneWhen12_EveryInvariantHasANamedTest`
in two ways that this task's own stated green gate (`go test
./internal/domain/todo/...` + `go test ./internal/ -run TestArchitecture`)
does **not** cover, because `TestDoneWhen12` lives in the same `internal`
package but under a different `-run` filter: the old `TestI3_
RepoOwnershipScoping_GetUpdateDelete` got renamed to `TestI3NoLongerApplies_
GetByIDReadsAnyCreator` (losing the required `TestI3_` prefix), and the
old `TestI4_TodoRepoOnlyQueriesTodosTable` was dropped entirely while
rewriting the file around the new schema.

Caught only because, before writing this report, I ran a broader sweep
(`go test ./internal/... -run '.*'`) than the task's own stated gate
strictly requires — the failure would not have shown up if I had stopped
at the two commands the green-gate note names. Confirmed via a throwaway
`git worktree` at `eff83f3` (task-1's merged HEAD, before this task
touched anything) that both tests passed there, so this was a regression
this task introduced, not a pre-existing gap. Fixed in a dedicated commit
(`f4377af`): `TestI3_GetByIDReadsAnyCreator_ScopingRetiredForThisDomain`
restores the prefix; `TestI4_TodoRepoOnlyQueriesTodosTable` is restored,
now passing `todo_events.sql` as a `sameModuleFiles` argument (both files
are the same domain module) so `todo_events.sql`'s legitimate join to its
own module's `todos` table doesn't trip the checker. Re-verified cold
(fresh clone, `go clean -cache -testcache`) after the fix: `TestDoneWhen12`
now fails only on I20/I21, both task-6/7's own scope, matching the exact
residual set already present at task-1's own merged HEAD.

Stated here as its own section, not folded into the Verification numbers
below, because a check that would have let this ship (task-2's own
literal green-gate command) is worth naming plainly rather than only
fixing quietly.

## A second pre-existing bug found, NOT fixed (out of this task's scope)

While running a broader check than my own green gate strictly requires
(`go test ./internal/identity/...`, to make sure nothing outside my
package regressed), found `TestI4_IdentityRepoOnlyQueriesUsersAndAPIKeysTables`
failing:

```
Error: Expect "...users.sql's own content..." to NOT match "(?i)\busers\b"
Messages: db/queries/users.sql must not reference table "users" — it
belongs to todo_events.sql, a different module's query file
```

**Confirmed pre-existing, not caused by this task**: reproduced the same
failure via `git stash` against `eff83f3` (task-1's own merged commit,
before any task-2 change), then `git stash pop` to restore. Root cause:
`internal/dbquery.AssertQueryFileReferencesOnlyOwnTable` (task-1's own
`ListTodoEventsFeed` query, unchanged in shape by this task's fix —
only its placeholder numbering changed) infers table ownership by
scanning every *other* `.sql` file for table names it references and
forbidding the file under test from mentioning any of them. This breaks
down the moment a table is legitimately referenced by more than one
module's query file — `todo_events.sql`'s `JOIN users ...` (a legitimate
read, for the feed's actor handle/role, already present before this task
touched the file) makes `users` appear in `todo_events.sql`'s scanned
table list, and the checker then forbids `users.sql` — the file that
*actually owns* `users` — from containing its own table's name, which is
obviously always true. Confirmed via `go test ./internal/identity/...
-v`: exactly one failing test in the whole package, isolated to this one
assertion.

**Not fixed here, on purpose**: task-2's own green gate is `go test
./internal/domain/todo/...` and the architecture test — `internal/
identity` isn't part of it, and `internal/dbquery`'s ownership-inference
model needs an actual design decision (how to represent "table X is
legitimately read, not owned, by file Y") that deserves its own
consideration rather than a rushed fix bundled into this task's commits.
Flagging it here plainly rather than leaving it as a silent surprise for
whichever task next runs the full suite (task-4 is the first point the
plan expects `go test ./...` fully green — this will block that unless
fixed before then). Recommend routing to Clara for scoping, the same way
task-1's own Down-mapping defect got a dedicated fix-round rather than
being folded silently into a later task.

## Verification

### Green gate (task-2's own, narrower than the whole repo, per `_plan/_todo.md`)

Both commands run **cold**, from a fresh `git clone --branch
milestone-4/activity-log` into a scratch directory with `go clean
-cache -testcache` first — not just the warm working tree:

```
$ go test -count=1 ./internal/domain/todo/...
ok  	github.com/mildronize/my-template/internal/domain/todo	0.158s

$ go test -count=1 ./internal/ -run 'TestArchitecture|TestI15Floor' -v
--- PASS: TestArchitecture_DomainFilesNeverImportGin (0.00s)
--- PASS: TestArchitecture_OnlyRepoFilesImportSqlc (0.01s)
--- PASS: TestArchitecture_DomainModulesNeverImportSiblingDomains (0.41s)
--- PASS: TestArchitecture_DomainAndIdentityNeverImportTransport (0.25s)
--- PASS: TestI15Floor_CanActuallyFail (0.00s)
--- PASS: TestArchitecture_OnlyTodoRepoReferencesTodoEventQueries (0.02s)
--- PASS: TestArchitecture_PlatformNeverImportsDomainIdentityOrTransport (0.25s)
PASS
ok  	github.com/mildronize/my-template/internal	0.940s
```

Both green. `go test ./internal/... -run TestArchitecture` (the literal
recursive form) reports overall `FAIL` — **not because any architecture
test failed**, but because `./internal/...` also matches `internal/
transport/{publicapi,bff}`, which don't build yet (tasks 3/4's own
scope, expected per the plan's green-gate note). The package that
actually contains `TestArchitecture_*` (`github.com/mildronize/
my-template/internal`) reports `ok` on its own, shown above scoped
correctly with `go test ./internal/`.

Also ran with `-race`: `go test -count=1 -race ./internal/domain/todo/...`
→ `ok  ... 3.191s`, no races.

`gofmt -l` on every file this task touched (`internal/domain/todo`,
`internal/architecture_test.go`, `db/queries/`): empty output, clean.

**Beyond the two literal commands above**: also ran `go test
./internal/...` and `go test ./internal/ -v` (no `-run` filter) as a
broader sweep before considering this task done — this is how the
regression documented in "Near-miss" below was actually caught (`TestI3_`/
`TestI4_` naming/coverage broken by the `repo_test.go` rewrite, invisible
to the two commands task-2's own green gate names). After the fix
(`f4377af`), re-verified cold once more: `TestDoneWhen12_
EveryInvariantHasANamedTest` fails only on I20/I21 (task-6/7's own
scope), matching the exact residual set already present at task-1's
merged HEAD.

### Done-when 1 (task-1's, re-confirmed unaffected)

`go test -count=1 ./internal/platform/...` → `ok`. Both
`TestTodoActivityLogMigration_PreservesExistingRows` and
`TestTodoActivityLogMigration_DownCollapsesFourStatusesToBoolean` still
pass — this task did not touch any migration file.

### Done-when 2 — transactional atomicity

`TestI15_Append_FailureMidWriteLeavesNeitherEventNorStateChange`
(`internal/domain/todo/service_test.go`): real `*sql.DB`/`*sql.Tx`
(`newTestDB`, the project's existing goose-migrated-sqlite fixture — the
full, real migration set, not a hand-maintained schema copy), plus a
thin `failureInjectingRepo` decorator wrapping the real, tx-scoped
`Repository`. It delegates `UpdateStatus` to the real implementation
first — so the actual `UPDATE` executes against the open transaction —
then returns an injected error. Proves genuine rollback (the
already-applied write is undone), not merely "the side effect was never
attempted". Asserts `todo_events` row count is unchanged and the todo's
status is still `open` after the call.

Two supporting tests exercise the same seam one layer down, with no
decorator or mock at all: `TestRepo_WithinTx_RollsBackOnError` (a plain
insert inside `WithinTx`, followed by a returned sentinel error, proven
not to persist by row count) and `TestRepo_WithinTx_CommitsOnSuccess`
(the positive control).

```
--- PASS: TestI15_Append_FailureMidWriteLeavesNeitherEventNorStateChange (0.01s)
--- PASS: TestRepo_WithinTx_RollsBackOnError (0.01s)
--- PASS: TestRepo_WithinTx_CommitsOnSuccess (0.01s)
```

### Done-when 3 — append-only, checked by row count

`TestI17_Append_EachStateChangeAddsExactlyOneEventRow`: five different
`Append` calls (status change, assign, field change, comment, second
status change) on the same todo, counting `todo_events` rows
before/after each — asserts exactly `+1` every time, then asserts `seq`
is strictly monotonic (1..5) across all of them. This is the row-count
distinction I17's own text draws, not "no update method exists on the
repo" (which is also true here, structurally, but isn't what this test
checks).

```
--- PASS: TestI17_Append_EachStateChangeAddsExactlyOneEventRow (0.01s)
```

### Done-when 4 — idempotency

`TestI19_Append_RepeatedClientRequestIDReturnsOriginalAndCreatesNothing`:
calls `Append` twice with the same `ClientRequestID` but a **different**
requested target status on the second call. Asserts the two responses
are identical, the `todo_events` row count is unchanged between calls,
and — the part that proves the second call was truly a no-op, not just
"looked similar" — the todo's status never moved to the second call's
target. `TestI19_CreateTodo_RepeatedClientRequestIDReturnsOriginalAndCreatesNothing`
covers the same property for `CreateTodo`'s own idempotency (this task's
own design extension, see Decisions above), with a different title on
the repeat, asserted ignored.

```
--- PASS: TestI19_Append_RepeatedClientRequestIDReturnsOriginalAndCreatesNothing (0.01s)
--- PASS: TestI19_CreateTodo_RepeatedClientRequestIDReturnsOriginalAndCreatesNothing (0.01s)
```

### Done-when 5 — permission layer, paired

`TestI18_Append_SameAgentSameTodo_ClosedRejected_NonClosedSucceeds`: the
**same** agent identity, against the **same** todo — a `status:closed`
attempt rejected (`ErrForbidden`), immediately followed by a comment on
the same todo succeeding. Then asserts the todo's status is still `open`
(the rejected attempt changed nothing) and exactly one event exists (the
successful comment — the rejected attempt wrote nothing). This is the
exact pairing the task calls for: a permission layer that rejected
everything would fail the second half, not just pass the first.
`TestI18_Append_OwnerCanCloseTheSameTodo` covers the owner half
separately. `permission_test.go` additionally unit-tests `can()` itself
in complete isolation (`TestI18_Can_AgentPaired`, plus owner/unknown-role/
unknown-event-type edge cases) — zero database, proving the permission
*function* is independently testable, not only reachable through
`Append`.

**Fixture note, stated plainly per the task's own instruction**: every
agent/owner identity in these tests is a plain seeded `users` row
(`createTestUser`/`createTestOwner` in `todo_testutil_test.go`), not
`cmd/issue-key`'s real path. This is deliberate, not a shortcut taken
without noticing: `can()`/`Append`'s permission check is role-based only
— it reads `PolicyActor.Role`, a value these tests supply directly — and
never resolves an actor from a credential itself; that resolution is
`internal/identity`'s own seam, one layer up, with its own tests already
covering it (confirmed: `TestResolveActor_APIKeySuccess` et al. already
exist there). `cmd/issue-key`'s real path matters for testing key
issuance/resolution, which this layer's tests are not exercising.

```
--- PASS: TestI18_Append_SameAgentSameTodo_ClosedRejected_NonClosedSucceeds (0.01s)
--- PASS: TestI18_Append_OwnerCanCloseTheSameTodo (0.01s)
--- PASS: TestCan_OwnerUnconditional (0.00s)
--- PASS: TestI18_Can_AgentPaired (0.00s)
--- PASS: TestCan_UnknownRoleFailsClosed (0.00s)
--- PASS: TestCan_UnknownEventTypeFailsClosedForNonOwner (0.00s)
```

### I15's own architecture-test fix — attacked and reverted

`TestArchitecture_OnlyTodoRepoReferencesTodoEventQueries` +
`TestI15Floor_CanActuallyFail`, added to `internal/architecture_test.go`.

**Attack 1** — a throwaway file *inside* `internal/domain/todo` but not
matching the repo-file allowlist pattern
(`internal/domain/todo/zz_attack_test.go`, calling `db.New(nil).
InsertTodoEvent(...)`):

```
architecture_test.go:527: .../internal/domain/todo/zz_attack_test.go:
references "InsertTodoEvent" — only internal/domain/todo's own repo file
may reference the todo_events sqlc query functions (INVARIANTS.md I15's
table-specific check)
--- FAIL: TestArchitecture_OnlyTodoRepoReferencesTodoEventQueries (0.02s)
```

Caught, named correctly. Reverted (`rm`).

**Attack 2** — a second throwaway file entirely *outside* the todo
domain (`internal/identity/zz_attack_test.go`, calling
`GetTodoEventMaxSeqByTodoID`):

```
architecture_test.go:527: .../internal/identity/zz_attack_test.go:
references "GetTodoEventMaxSeqByTodoID" — only internal/domain/todo's
own repo file may reference the todo_events sqlc query functions
(INVARIANTS.md I15's table-specific check)
--- FAIL: TestArchitecture_OnlyTodoRepoReferencesTodoEventQueries (0.02s)
```

Caught, named correctly. Reverted (`rm`), confirmed `git status --short`
clean and the full architecture suite green again afterward.

**The floor check's own ability to fail** was proven a different way —
not by physically deleting real query functions from `internal/db` on
every test run, but by extracting the `>= 3` rule into its own predicate
(`meetsI15Floor`) and testing it directly at every boundary
(`TestI15Floor_CanActuallyFail`: `0`/`1`/`2` → `false`, `3`/`4` → `true`).
This proves the predicate really can flip, independent of trusting the
`>=` expression was written correctly by inspection.

### Build status — exactly what's still broken, and why

`go build ./...` fails with **one root-cause package**:

```
# github.com/mildronize/my-template/internal/transport/publicapi
internal/transport/publicapi/todo_handler.go:48:16: t.Done undefined (type todo.Todo has no field or method Done)
internal/transport/publicapi/todo_handler.go:61:57: too many arguments in call to s.Service.ListTodos
internal/transport/publicapi/todo_handler.go:95:69: cannot use req.Title (variable of type string) as todo.CreateInput value in argument to s.Service.CreateTodo
internal/transport/publicapi/todo_handler.go:112:64: too many arguments in call to s.Service.GetTodo
internal/transport/publicapi/todo_handler.go:138:28: s.Service.UpdateTodo undefined (type *todo.Service has no field or method UpdateTodo)
internal/transport/publicapi/todo_handler.go:159:22: s.Service.DeleteTodo undefined (type *todo.Service has no field or method DeleteTodo)
```

`go vet ./...` confirms the same single package name in its `#` failure
markers (`internal/transport/publicapi`, listed twice — once for the
package itself, once for its `[...]` test-binary variant). `internal/
transport/bff` and `cmd/{server,smoke}` fail too, but **only
transitively** through their own import of `publicapi` (confirmed by
building `internal/transport/bff` alone — it reports the exact same
`publicapi/todo_handler.go` errors, not any error of its own). This is
exactly the shape the plan's green-gate note predicts for this task:
"`go build ./...` still red, same reason as task-1, now localized to the
transport packages instead of the domain package" — not a regression,
`todo_handler.go` simply hasn't been updated for the new `Service` shape
yet, which is tasks 3/4's own scope.

## What I did not establish

- Did not run or attempt anything at the HTTP layer — no handler, no
  route, no `POST`/`GET` against a running server. Everything above is a
  Go-level service/repo test, which is the correct instrument for this
  task's scope but does not stand in for tasks 3/4's own HTTP-level
  proofs (Done-when 13/14 specifically).
- Did not fix `internal/identity`'s pre-existing `TestI4_...` failure
  (see above) — confirmed its cause, confirmed it predates this task,
  chose not to fix it since it's outside this task's declared scope.
  This will need attention before task-4's "first point the full Go
  suite is green again" gate.
- Did not extend `internal/domain/todo`'s own I4 coverage to
  `todo_events.sql` (only `todos.sql` has a
  `TestI4_..._OnlyQueriesTodosTable`-style check today, unchanged from
  before this task) — not one of this task's Done-when items, noted here
  since I noticed the gap while working in the area.

## Commits pushed (branch `milestone-4/activity-log`)

- `0d89d95` — `fix(milestone-4/task-2): fix ListTodoEventsFeed's sqlc placeholder-numbering bug`
- `a663466` — `feat(milestone-4/task-2): todo domain service layer for the shared-collection schema`
- `5e0a7cb` — `test(milestone-4/task-2): I15's table-specific architecture check, floor-first`
- `3e02ef4` — `test(milestone-4/task-2): Done-when 2-5 - atomicity, append-only, idempotency, permission`
- `8746e81` — `docs(milestone-4/task-2): task-2 report` (superseded by this file's current content — the near-miss below was found after this commit, fixed in the next one)
- `f4377af` — `fix(milestone-4/task-2): restore TestI3_/TestI4_ named tests dropped in the repo_test.go rewrite`

Pushed through `f4377af`: `eff83f3..f4377af milestone-4/activity-log -> milestone-4/activity-log` (two pushes — `eff83f3..8746e81` first, then `8746e81..f4377af` after the near-miss fix).
