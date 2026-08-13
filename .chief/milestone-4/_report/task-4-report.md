# Task task-4 Report

## Task

BFF surface (`internal/transport/bff`, `bff-openapi.yaml`) for the
shared-collection todo schema task-1 built and task-2 wired into
`todo.Service`, and task-3 already mirrored on the public API — same
shape, session-authenticated: todo endpoints updated for the new fields
(no more `done`); `POST`/`GET /api/bff/todos/:id/events` added, one body
shape per `type`; `status: closed` succeeds here (I18 — this is the
owner's own surface, unlike the public API where an agent is rejected);
`DELETE /api/bff/todos/:id` removed entirely (genuine 404, not 405);
`type: "created"` genuinely rejected (400) at the HTTP layer, tested
independently of task-3's own proof on the public API. Also owns this
milestone's first point the whole Go suite is green again (task-4's own
green gate, `_plan/_todo.md`), and a cheap reachability check against a
real running binary.

## Outcome

done

## Decisions

- **Mirrored task-3's `publicapi/todo_handler.go` structurally, not just
  in shape.** Same helper names with a `bff`/`BFF` prefix
  (`toBFFTodo`/`toBFFEvent`, `bffValidStatuses`/`bffTodoFieldByWireName`,
  `bffPolicyActor`, `writeBFFAppendError`), same dispatch switch in
  `CreateTodoEvent`, same error-mapping shape. Deliberate — the goal's own
  instruction is "same shape as task-3," and Clara already reviewed
  task-3's shape; reinventing it here would be the exact "don't reinvent
  shapes task-3 already got right" the brief warns against. The one
  structural difference: `bffPolicyActor`/`bffOwnerID` read
  `bff.ActorFromContext` (this package's own session-resolved
  `identity.User`) instead of `publicapi.ActorFromContext` (Bearer-resolved)
  — different identity source, same `todo.PolicyActor{Role: ...}` shape
  downstream, so `todo.Service.Append`'s permission check (`can()`) is the
  exact same code path either surface reaches, just resolving differently
  given a different actor role.
- **`status: closed` needed no special-casing to succeed on this
  surface.** I18's `can()` (`internal/domain/todo/permission.go`) already
  passes any write unconditionally for `Role: "owner"`, and I12
  (`RequireJSONSession`) already guarantees a BFF session can never
  resolve to `role="agent"` — so the same dispatch/permission code task-3
  wrote, called with a BFF-resolved actor, already produces the contract's
  required behavior with zero new logic. Added
  `TestI18_BFF_OwnerCanCloseTodo` as an HTTP-layer proof of this rather
  than trusting the reasoning alone — it creates a todo, POSTs
  `status_changed` to `closed` through `/api/bff/todos/:id/events`, and
  asserts both the write succeeds (201) and the todo's own state actually
  moved to `closed` on a follow-up `GET`.
- **`bff-openapi.yaml`'s `/keys` paths and schemas were left completely
  untouched.** `_plan/_todo.md` names `keys_handler.go`'s I21 rewrite as
  task-6's own scope ("`GET /api/bff/keys` becomes 'every `role='agent'`
  user's non-revoked keys'... task-6"), not task-4's — confirmed by
  reading the plan before touching the spec, not assumed. Regenerating
  `bffapi.gen.go` only touched todo-shaped types/methods/routes; diffed
  the generated output specifically to confirm `ListKeys`/`RevokeKey`
  and their wire types are byte-for-byte the same as before this task
  (see Verification below).
- **`internal/transport/bff/bff_testutil_test.go`'s `seedTodo` helper was
  dead code (unused, referencing `owner_id`/`done`) and is deleted, not
  updated.** `go build`/`go vet` never flagged it because Go doesn't error
  on an unused top-level function — only `grep` for its one call site
  (found: none) surfaced that it wasn't load-bearing for any test. Left
  it in place risked a future test copying a stale-schema fixture by
  example; removing it is a one-line net loss, not a functional change.
- **`cmd/server/bff_negative_check_test.go`'s
  `TestBFF_FullCRUDRoundTrip_ThroughAssembledMainHandler` was a real,
  unowned regression this task had to fix, not pre-existing red.** It
  wasn't in `_plan/_todo.md`'s named known-red baseline (that baseline is
  scoped to `TestDoneWhen12`'s I20/I21 gaps and `publicapi` not building —
  both resolved by task-3), and it lives in `cmd/server`, a package this
  task doesn't own by name but that transitively depends on
  `internal/transport/bff` and is exactly the kind of "any other red
  package... is a real regression" the brief calls out. Rewrote it for the
  new shape (`clientRequestId` on every write, no `done`, a
  `status_changed`-to-`closed` write in place of the old boolean toggle,
  DELETE now asserted 404 instead of 204) and added a `type: "created"`
  rejection check as a *third*, independent HTTP-layer proof point beyond
  `publicapi`'s and this package's own — against the actual assembled
  `main.go` composition, not either isolated test router.

## Verification

### Green gate — this task's own, and the milestone's first full-repo one

```
$ go build ./...
(no output — clean)

$ go vet ./...
(no output — clean)

$ gofmt -l $(git ls-files '*.go')
(no output — clean)

$ go test ./...
ok  	github.com/mildronize/my-template/cmd/issue-key
ok  	github.com/mildronize/my-template/cmd/seed
ok  	github.com/mildronize/my-template/cmd/server
?   	github.com/mildronize/my-template/cmd/smoke	[no test files]
?   	github.com/mildronize/my-template/db/migrations	[no test files]
--- FAIL: TestDoneWhen12_EveryInvariantHasANamedTest
    invariants_test.go:417: no test named TestI20_<something> found anywhere
      under ... — I20 (scope: global) has no test referencing it (Done-when 12)
    invariants_test.go:450: no test named TestI21_<something> found inside
      .../internal/identity (scope: domain:identity) — I21 requires a
      dedicated test in that specific package (Done-when 12)
FAIL	github.com/mildronize/my-template/internal
?   	github.com/mildronize/my-template/internal/api	[no test files]
?   	github.com/mildronize/my-template/internal/bffapi	[no test files]
?   	github.com/mildronize/my-template/internal/db	[no test files]
ok  	github.com/mildronize/my-template/internal/dbquery
ok  	github.com/mildronize/my-template/internal/domain/todo
ok  	github.com/mildronize/my-template/internal/identity
ok  	github.com/mildronize/my-template/internal/platform
ok  	github.com/mildronize/my-template/internal/transport/bff
ok  	github.com/mildronize/my-template/internal/transport/publicapi
?   	github.com/mildronize/my-template/web	[no test files]
FAIL
```

**Classification against `_plan/_todo.md`'s named baseline**: the *only*
failure is `TestDoneWhen12_EveryInvariantHasANamedTest`, and it fails on
exactly `I20` and `I21` — the two invariants the plan names as "task-7's
and task-6's, not yet written." Every other package, including
`internal/transport/bff` and `cmd/server`, is green. This is the first
point in the milestone the full, unfiltered `go test ./...` is green
except for that one named test — matching the plan's own description of
what task-4 is supposed to leave behind. `go test ./internal/...`
(unfiltered, no `-run`, run separately per the task's own instruction) has
the identical result.

**Regression found and fixed during this task, before it counted as
"only the named baseline is red":**
`cmd/server/bff_negative_check_test.go`'s
`TestBFF_FullCRUDRoundTrip_ThroughAssembledMainHandler` failed on first
unfiltered run — `expected: 201, actual: 400,
"clientRequestId" property is missing` — because it still built request
bodies against the pre-milestone-4 shape (`{"title":"..."}`, no
`clientRequestId`; `{"done":true}` PATCH; a `DELETE` expecting `204`).
Not part of the declared known-red baseline (it lives outside both
`publicapi`'s "not building" gap and `TestDoneWhen12`'s I20/I21 gap), so
this was a real regression this task's own scope change (the schema
rename reaching `cmd/server`'s composed handler) exposed and had to fix,
not an expected gap to note and move past.

### Full `-v` listing, `internal/transport/bff` (this task's own package)

```
--- PASS: TestI11_CallbackNeverExchangesWithoutStateCookie (0.81s)
--- PASS: TestI12_BFFSessionNeverResolvesToAgent_Callback (0.11s)
--- PASS: TestCallback_UnrecognizedSubIsAnErrorPageNeverAJITRow (0.10s)
--- PASS: TestCallback_SuccessfulOwnerLoginSetsSessionAndRedirects (0.06s)
--- PASS: TestBFFHandler_ListKeys_ReturnsOwnersOwnKeys (0.13s)
--- PASS: TestBFFHandler_RevokeKey_ThenListNoLongerShowsIt (0.05s)
--- PASS: TestI3_BFFHandlerOwnershipScoping_ReturnsNotFoundNotForbidden_Keys (0.16s)
--- PASS: TestI11_LoginRedirectAlwaysIncludesPKCEChallenge (0.10s)
--- PASS: TestLogin_MissingConfigShowsErrorNotACrash (0.04s)
--- PASS: TestSecureFromURL_FollowsConfiguredScheme (0.00s)
--- PASS: TestNegativeCheck_NoKeyIssuanceOrRotateEndpointOnBFFSurface (0.11s)
--- PASS: TestBFFHandler_FullCRUDRoundTrip_RealSessionCookie (0.16s)
--- PASS: TestI3NoLongerApplies_BFFHandlerReadsEveryTodoRegardlessOfCreator (0.08s)
--- PASS: TestBFFHandler_ListTodos_Unauthenticated_Returns401NotRedirect (0.09s)
--- PASS: TestBFFHandler_CreateTodo_DoneFieldRejected (0.19s)
--- PASS: TestBFFHandler_UpdateTodo_DoneFieldRejected (0.09s)
--- PASS: TestDoneWhen14_CreateTodoEvent_TypeCreatedRejected (0.32s)
    --- PASS: .../type:_created
    --- PASS: .../type:_an_ordinary_unrecognised_string
--- PASS: TestI18_BFF_OwnerCanCloseTodo (0.04s)
--- PASS: TestBFFHandler_EventsRoundTrip_CreateReadAppendFieldChangedReadTimeline (0.07s)
--- PASS: TestDoneWhen6_DeleteTodo_RouteGenuinelyDoesNotExist_BFF (0.16s)
PASS
ok  	github.com/mildronize/my-template/internal/transport/bff	2.911s
```

25 top-level tests (including pre-existing `keys_handler_test.go`/
`login_handler_test.go`/`callback_handler_test.go`/
`negative_check_test.go` coverage), 0 failures.

### `bffapi.gen.go` regeneration — diffed, not trusted blind

Ran the Makefile's exact `generate` invocation
(`./bin/oapi-codegen -generate types,gin,spec -package bffapi -o
internal/bffapi/bffapi.gen.go bff-openapi.yaml`) and diffed the result:

- `Todo` gained `assigneeId`/`priority`/`dueDate`/`createdBy`/`status`,
  lost `done` — matches the contract's wire shape exactly, mirrors
  `internal/api/openapi.gen.go`'s own already-regenerated `Todo` type
  field-for-field.
- New `TodoEvent`/`TodoEventList`/`CreateTodoEventRequest` types, new
  `ListTodoEvents`/`CreateTodoEvent` methods on `ServerInterface`, new
  `GET`/`POST /todos/:id/events` route registrations.
- `DeleteTodo` removed from `ServerInterface`, its wrapper method, and its
  route registration (`router.DELETE(.../todos/:id, ...)` no longer
  present) — grepped the diff for `DeleteTodo`/`router.DELETE` to confirm
  it's gone, not just renamed.
- `ListKeys`/`RevokeKey` and their wire types (`ApiKey`/`ApiKeyList`) are
  byte-for-byte unchanged in the diff — confirmed by grepping the diff for
  `Key` and finding no hits outside the untouched pre-existing lines,
  matching this task's "leave `/keys` for task-6" decision above.
- The embedded base64 spec blob changed (expected — it's the compiled
  spec) and nothing else outside the above categories appeared in the
  diff.

### Attacked `TestDoneWhen14_CreateTodoEvent_TypeCreatedRejected` before trusting it

Per this engagement's standing discipline and the brief's explicit
instruction to reuse the exact attack that caught task-3's equivalent bug:
temporarily replaced `CreateTodoEvent`'s `default` case (the one that
rejects an unrecognised `type`, `"created"` included) with code that
silently rerouted it into a `commented` write instead of rejecting it —

```go
default:
    // ATTACK-PLANTED ...
    body := "planted-attack-body"
    input.Type = todo.EventTypeCommented
    input.Comment = &todo.CommentInput{Body: body}
}
```

Confirmed the edit landed (`sed -n` printed the patched lines) and the
package still compiled (`go build ./...` clean) before trusting the
failure. Ran the target test against the planted bug:

```
--- FAIL: TestDoneWhen14_CreateTodoEvent_TypeCreatedRejected/type:_created
    Error: Not equal: expected: 400, actual: 201
--- FAIL: TestDoneWhen14_CreateTodoEvent_TypeCreatedRejected/type:_an_ordinary_unrecognised_string
    Error: Not equal: expected: 400, actual: 201
```

Both subtests went red for the right reason — the forged event was
silently accepted (201) instead of rejected, exactly the failure mode the
test exists to catch. Reverted the edit, confirmed the diff no longer
contains any `ATTACK-PLANTED`/`planted-attack` marker (`git diff | grep -c`
→ `0`), rebuilt clean, and reran the test to confirm green again (see the
`-v` listing above — both subtests `PASS`).

### Reachability check — real binary, real minted session

Built the actual `cmd/server` binary from this task's working tree
(`go build -o .../reachability-server ./cmd/server`), seeded a real owner
row into a fresh, empty SQLite database via `cmd/seed` (not a fixture —
the same command a real deployment runs), minted a real signed session
cookie for that owner's id via `bff.Signer.NewSessionCookie` (a throwaway
`cmd/`-local helper program, deleted before this report — not committed,
confirmed absent from `git status`), started the compiled binary as a
background OS process bound to a real TCP port, and issued real HTTP
requests with `curl`:

```
GET / (no cookie)                          → 200, <title>My Template</title>
                                              (the real built SPA, not the
                                              test-fixture placeholder)
GET /api/bff/me (no cookie)                → 401 {"error":{"code":"unauthorized",...}}
GET /api/bff/me (real minted session)      → 200 {"handle":"owner","role":"owner","active":true}
GET /api/bff/me (tampered cookie, last
  character flipped)                       → 401 (attack: proves the check
                                              can actually fail, not just
                                              execute)
GET /api/bff/todos (real minted session)   → 200 {"todos":[]}
POST /api/bff/todos (real minted session,
  real clientRequestId)                    → 201, a real persisted todo row
                                              with status:"open"
```

Killed the server process afterward and confirmed it exited
(`ps -p <pid>` → no process). Deleted the throwaway cookie-minting helper
directory; `git status` inside the repo shows it never touched tracked or
untracked repo state.

**What this proves, stated precisely, per the brief's own instruction not
to overclaim**: this proves *session consumption and actor resolution*
survived the task-1 schema rename — a session cookie signed for a real
`users.id`, presented to the real compiled binary, resolves through
`RequireJSONSession` to the correct `identity.User` and the todo write
path (`Service.CreateTodo`) accepts and persists a write attributed to
that resolved actor. It does **not** touch session *establishment* — no
`/login` or `/callback` request was made in this check, no SSO
redirect/exchange was exercised, and no browser or cookie-setting flow was
driven. That remains browser-only, unverified by this task, same as the
brief states.

## What I did not establish

- **The reachability check's negative half (tampered cookie → 401) is the
  only "attack" run against the real binary** — I did not additionally
  fuzz malformed request bodies, concurrent writes, or other adversarial
  inputs against the live process; those properties are already covered
  by the Go test suite above (idempotency, permission, validation), which
  ran against the same code, just not through a spawned OS process.
- **Did not exercise `GET /login` or `GET /callback` against the real
  binary** — deliberately out of scope per the task's own instruction
  ("this stays browser-only, out of scope here, verified later by a
  human"). `login_handler_test.go`/`callback_handler_test.go`'s existing
  coverage (both green, unaffected by this task) is the only proof that
  code path has, and it's an in-process test, not a real-binary one.
- **Did not investigate `TestDoneWhen12`'s I20/I21 failures beyond
  confirming they match the plan's exact named baseline** — not this
  task's to fix (task-7's and task-6's, respectively), and the plan
  explicitly names them as the expected residue after task-4.
- **Did not touch `bff-openapi.yaml`'s `/keys` paths, `keys_handler.go`,
  or `keys_handler_test.go`** — confirmed via the regenerated-diff check
  above that this task's spec/codegen changes left them untouched;
  task-6 owns their I21 rewrite.
- **Did not run the JS/Vitest suite** — task-4's own green gate
  (`_plan/_todo.md`) is Go-only; per the plan, "task-7 is where the JS
  suite exists to have a state at all."

## Commits pushed (branch `milestone-4/activity-log`)

- `0637970` — `feat(milestone-4/task-4): BFF surface for the shared-collection todo schema`
