# Task task-6 Report

## Task

I21 (`_contract/INVARIANTS.md`): `GET /api/bff/keys` (owner session) becomes
"every `role='agent'` user's non-revoked keys"; `DELETE /api/bff/keys/:id`
(owner session) becomes "any agent's key, still session-gated to the
owner." `GET /api/v1/keys` (agent Bearer) stays untouched and self-scoped.
`internal/transport/bff/keys_handler_test.go`'s existing
`newBFFRouterForTwoOwnersWithKeys`-based assertions are rewritten, not left
in place. Revocation is proven both ways: authenticate with a real key
against the real public API first, revoke via the new endpoint, then prove
the same key fails. Owns GOAL.md's **Done-when 7** and **Done-when 11**.

This is step 1 of มายด์'s own acceptance walkthrough: log in, see every
agent's key in Settings, revoke one, confirm it stops working.

## Outcome

done

## What was already there vs. what this task built

Nothing pre-existing touched this endpoint pair beyond the milestone-2/3
implementation described in GOAL.md's survey (session-owner-scoped, and
structurally always empty in production since no key is ever issued to
`role='owner'` — I2). This task built:

- `db/queries/api_keys.sql`: two new queries, `ListAllAgentAPIKeys`
  (`JOIN users ON users.id = api_keys.user_id WHERE users.role = 'agent'
  AND api_keys.revoked_at IS NULL`) and `RevokeAPIKeyByID` (no `user_id`
  scoping). Regenerated via `./bin/sqlc generate` into
  `internal/db/api_keys.sql.go`/`internal/db/querier.go` — diffed before
  trusting, see below.
- `internal/identity/repo.go`: `Repo.ListAllAgentAPIKeys` and
  `Repo.RevokeAPIKeyByID`, thin wrappers over the new sqlc queries,
  matching this file's existing `apiKeyFromRow`/`ErrNotFound` conventions.
- `internal/identity/service.go`: `APIKeyRepo` interface gains both new
  methods; `Service.ListAllAgentAPIKeys` and `Service.RevokeAnyAgentAPIKey`
  call through to them. `internal/identity/service_test.go`'s
  `fakeAPIKeyRepo` gets matching (deliberately role-unaware — see Decisions
  below) stub implementations so the interface still compiles against the
  fake used by unrelated `Service` tests.
- `internal/transport/bff/keys_handler.go`: `ListKeys`/`RevokeKey` call the
  new service methods instead of the old owner-scoped ones. Still requires
  a valid owner session (`bffOwnerID`) to reach either handler at all —
  only the *query scoping*, not the *session gate*, changed.
- `internal/identity/repo_test.go`: new
  `TestI21_ListAllAgentAPIKeys_SpansEveryAgent_ListAPIKeysByOwner_StaysSelfScoped`
  — I21's dedicated `domain:identity`-scoped test, placed next to
  `TestI3_RevokeAPIKeyScopedToOwner_AbsenceNotPermission` per that test's
  own naming/placement convention.
- `internal/transport/bff/keys_handler_test.go`: rewritten wholesale (see
  below).

## Decisions

- **`RevokeAPIKeyByID` has no `role='agent'` filter on the SQL side.**
  `ListAllAgentAPIKeys` does filter on `users.role = 'agent'` (it's a
  listing query with no other way to exclude an owner's hypothetical key),
  but the revoke query just matches `id` and `revoked_at IS NULL`. This is
  deliberate, not an oversight: I2 makes it structurally impossible for an
  `api_keys` row to ever belong to a `role='owner'` user (no code path
  ever issues one), so "revoke by id alone" and "revoke any agent's key by
  id" are the same operation in practice. Adding a redundant `JOIN
  users...WHERE role='agent'` to the revoke query would be defense against
  a state the schema/write-path already forecloses elsewhere — I chose not
  to add it, on the same "don't go further than asked without evidence"
  reasoning GOAL.md's Decisions table applies to append-only enforcement
  elsewhere in this milestone. Flagging this as a judgment call, not
  something the contract dictated either way.
- **`fakeAPIKeyRepo`'s new `ListAllAgentAPIKeys`/`RevokeAPIKeyByID` stubs
  are role-unaware** (they just filter on `revoked_at`, not role) — that
  fake has no notion of a `users` table at all, and the `Service` tests it
  backs were never testing I21's role-scoping property in the first place.
  I21's actual role-scoping behavior is proven against a real SQLite
  schema only: `repo_test.go`'s new `TestI21_...` and
  `keys_handler_test.go`'s rewritten suite. Said explicitly in both files'
  own doc comments so a future reader doesn't mistake the fake's coverage
  for more than it is.
- **The old BFF-level `TestI3_BFFHandlerOwnershipScoping_ReturnsNotFoundNotForbidden_Keys`
  test is not carried forward in any form.** GOAL.md's own I3-scope
  correction states I3's ownership-scoping no longer applies to this half
  of the identity domain — the owner is deliberately allowed to see and
  revoke every agent's key, so there is no "wrong owner, must get 404 not
  403" case left to test at this layer. I3 coverage for
  `internal/identity` overall is untouched:
  `TestI3_RevokeAPIKeyScopedToOwner_AbsenceNotPermission` (repo layer,
  proving the *agent-facing*, still-self-scoped `RevokeAPIKey` path) was
  not modified and still exists. I replaced the retired BFF-level test
  with a narrower `TestBFFHandler_RevokeKey_UnknownID_ReturnsNotFound`
  (an id that never existed still 404s, not a 500) — ordinary error
  handling, not an I3 claim.
- **`TestDoneWhen11_RevocationActuallyStopsTheKey_BothHalves` authenticates
  against `GET /api/v1/keys`, not `POST /api/v1/todos`**, for its
  real-HTTP round trip. Either would satisfy the letter of Done-when 11
  ("authenticate... against the public API... a genuine 2xx"); I chose
  `/api/v1/keys` because it doubles as a live check that the agent's own
  self-scoped listing (I21's other, unchanged half) actually works for a
  real Bearer-authenticated request, not just at the repo layer. A
  reasonable choice, not the only one the task text names.
- **No `_contract/API.md` changes.** I grepped `.chief/_rules/_contract/API.md`
  for `api/bff/keys` and found no dedicated section documenting that
  endpoint pair at all (only `/api/v1/keys` has one) — there was nothing
  stale to correct. `INVARIANTS.md`'s own I21 entry (written during the
  milestone-4 scope-tags fix-round, before this task) already documents
  the corrected semantics in present tense and the old bug in past tense;
  I did not need to touch it.

## Verification

### Build

```
$ go build ./...
(no output — clean)

$ go vet ./...
(no output — clean)

$ gofmt -l $(git ls-files '*.go')
(no output — clean)
```

### sqlc regeneration — diffed, not trusted blind

```
$ for f in internal/db/*.go; do grep -q '^// Code generated' "$f" && rm -f "$f"; done
$ ./bin/sqlc generate
$ git status --short internal/db/
 M internal/db/api_keys.sql.go
 M internal/db/querier.go
```

Diff (`git diff internal/db/`) is exactly the two new generated functions
(`ListAllAgentAPIKeys`, `RevokeAPIKeyByID`) and their `Querier` interface
entries — nothing else in `internal/db/` changed, no stray file
regenerated differently, no todo/todo_events output touched.

### `internal/dbquery` — table-isolation mechanism, unmodified, still green

Per the task brief's instruction to check `dbquery.TableOwnership` first
and not touch `tableisolation.go`: `users` and `api_keys` are both already
declared under the `identity` module, so `ListAllAgentAPIKeys`'s
`JOIN users` needed no new `ReadOnlyGrant` — confirmed by
`internal/identity/repo_test.go`'s existing (untouched)
`TestI4_IdentityRepoOnlyQueriesUsersAndAPIKeysTables` staying green, and
by the dedicated suite:

```
$ go test ./internal/dbquery/... -count=1 -v
... 15 tests, all PASS ...
ok  	github.com/mildronize/my-template/internal/dbquery	0.005s
```

`internal/dbquery/tableisolation.go` itself was not edited.

### Green gate — unfiltered `go test ./internal/...`, no `-run`

```
$ go test ./internal/... -count=1
--- FAIL: TestDoneWhen12_EveryInvariantHasANamedTest
    invariants_test.go:417: no test named TestI20_<something> found anywhere
      under ... — I20 (scope: global) has no test referencing it (Done-when 12)
FAIL	github.com/mildronize/my-template/internal	1.913s
?   	github.com/mildronize/my-template/internal/api	[no test files]
?   	github.com/mildronize/my-template/internal/bffapi	[no test files]
?   	github.com/mildronize/my-template/internal/db	[no test files]
ok  	github.com/mildronize/my-template/internal/dbquery	0.006s
ok  	github.com/mildronize/my-template/internal/domain/todo	0.236s
ok  	github.com/mildronize/my-template/internal/identity	1.275s
ok  	github.com/mildronize/my-template/internal/platform	0.047s
ok  	github.com/mildronize/my-template/internal/transport/bff	3.222s
ok  	github.com/mildronize/my-template/internal/transport/publicapi	0.275s
FAIL
```

Exactly the required shape: **I21 no longer appears in the failure list**;
the only failure is I20 (task-7's, out of scope here, untouched).

Also ran the Makefile's broader package set:

```
$ go test $(go list ./... | grep -v /node_modules/) -count=1
ok  	.../cmd/issue-key	0.048s
ok  	.../cmd/seed	0.023s
ok  	.../cmd/server	0.078s
?   	.../cmd/smoke	[no test files]
?   	.../db/migrations	[no test files]
(...internal/* identical to above — I20 only...)
?   	.../web	[no test files]
FAIL   (TestDoneWhen12_EveryInvariantHasANamedTest only, I20 only)
```

### `internal/transport/bff` — full `-v` listing, this task's own package

```
$ go test ./internal/transport/bff/... -v -count=1
--- PASS: TestBFFHandler_ListKeys_ReturnsEveryAgentsKeys (0.16s)
--- PASS: TestBFFHandler_RevokeKey_AnyAgentsKey_ThenListNoLongerShowsIt (0.15s)
--- PASS: TestBFFHandler_RevokeKey_UnknownID_ReturnsNotFound (0.12s)
--- PASS: TestBFFHandler_ListKeys_Unauthenticated_Returns401 (0.14s)
--- PASS: TestBFFHandler_RevokeKey_Unauthenticated_Returns401 (0.53s)
--- PASS: TestDoneWhen11_RevocationActuallyStopsTheKey_BothHalves (0.11s)
--- PASS: TestNegativeCheck_NoKeyIssuanceOrRotateEndpointOnBFFSurface (0.22s)
--- PASS: TestDoneWhen12_ActivityFeed_CrossActorAttribution (0.07s)
--- PASS: TestBFFHandler_ListActivity_OrderingAndPagination (...)
--- PASS: TestBFFHandler_ListActivity_MalformedCursorRejected (...)
--- PASS: TestBFFHandler_ListActivity_Unauthenticated_Returns401 (...)
--- PASS: TestI11_CallbackNeverExchangesWithoutStateCookie (...)
--- PASS: TestI12_BFFSessionNeverResolvesToAgent_Callback (...)
--- PASS: TestCallback_UnrecognizedSubIsAnErrorPageNeverAJITRow (...)
--- PASS: TestCallback_SuccessfulOwnerLoginSetsSessionAndRedirects (...)
--- PASS: TestI11_LoginRedirectAlwaysIncludesPKCEChallenge (...)
--- PASS: TestLogin_MissingConfigShowsErrorNotACrash (...)
--- PASS: TestSecureFromURL_FollowsConfiguredScheme (...)
--- PASS: TestBFFHandler_FullCRUDRoundTrip_RealSessionCookie (...)
--- PASS: TestI3NoLongerApplies_BFFHandlerReadsEveryTodoRegardlessOfCreator (...)
--- PASS: TestBFFHandler_ListTodos_Unauthenticated_Returns401NotRedirect (...)
--- PASS: TestBFFHandler_CreateTodo_DoneFieldRejected (...)
--- PASS: TestBFFHandler_UpdateTodo_DoneFieldRejected (...)
--- PASS: TestDoneWhen14_CreateTodoEvent_TypeCreatedRejected (...)
--- PASS: TestI18_BFF_OwnerCanCloseTodo (...)
--- PASS: TestBFFHandler_EventsRoundTrip_CreateReadAppendFieldChangedReadTimeline (...)
--- PASS: TestDoneWhen6_DeleteTodo_RouteGenuinelyDoesNotExist_BFF (...)
PASS
ok  	github.com/mildronize/my-template/internal/transport/bff	3.209s
```

31 top-level tests (6 new to this task, 25 pre-existing), 0 failures.

### `internal/transport/publicapi` — full `-v` listing, unchanged surface

```
$ go test ./internal/transport/publicapi/... -v -count=1
--- PASS: TestHandler_KeysListAndRevokeRoundTrip (0.01s)
--- PASS: TestI3_HandlerOwnershipScoping_ReturnsNotFoundNotForbidden_Keys (0.01s)
--- PASS: TestI9_ListKeys_ExpiredButUnrevokedKeyStillListed_RevokedKeyExcluded (0.01s)
--- PASS: TestHandler_ListKeys_Unauthenticated_Returns401 (0.01s)
--- PASS: TestHandler_RevokeKey_Unauthenticated_Returns401 (0.01s)
--- PASS: TestI1_RejectActorFields_BodyField (...)
--- PASS: TestI1_RejectActorFields_QueryParam (...)
--- PASS: TestI1_RejectActorFields_XActorHeader (...)
--- PASS: TestI1_RejectActorFields_AllowsCleanRequest (...)
--- PASS: TestI5_UnauthorizedResponseBodyIdenticalAcrossFailureReasons (...)
--- PASS: TestRequireActor_SetsActorOnContextForHandler (...)
--- PASS: TestHandler_FullCRUDRoundTrip (...)
--- PASS: TestI3NoLongerApplies_HandlerReadsEveryTodoRegardlessOfCreator (...)
--- PASS: TestHandler_ListTodos_Unauthenticated_Returns401 (...)
--- PASS: TestHandler_CreateTodo_MissingTitleRejectedByOpenAPIValidator (...)
--- PASS: TestHandler_CreateTodo_TitleTooLongRejectedByOpenAPIValidator (...)
--- PASS: TestHandler_CreateTodo_DoneFieldRejected (...)
--- PASS: TestHandler_UpdateTodo_DoneFieldRejected (...)
--- PASS: TestDoneWhen13_CreateTodoEvent_TypeCreatedRejected (...)
--- PASS: TestHandler_EventsRoundTrip_CreateReadAppendFieldChangedReadTimeline (...)
--- PASS: TestDoneWhen6_DeleteTodo_RouteGenuinelyDoesNotExist (...)
--- PASS: TestHandler_Me_RunsThroughSameGeneratedInterfaceAsTodos (...)
PASS
ok  	github.com/mildronize/my-template/internal/transport/publicapi	0.266s
```

**Zero files in `internal/transport/publicapi` were touched by this
task.** This full green run (including every pre-existing test, none
skipped) is the "GET /api/v1/keys stays untouched and self-scoped" proof
by construction — nothing here changed, and it still passes.

### Cold-clone verification

Pushed (`3c244bc`), then cloned that exact commit fresh into an isolated
scratchpad directory (not a cleaned copy of the working tree) and re-ran
both `go build ./...` and the full `go test` sweep there:

```
$ git clone --branch milestone-4/activity-log <repo> repo
$ cd repo && git log -1 --format=%H
3c244bca7b062a3a115b80cd9fade1195cacb8b4

$ go build ./...
(exit 0, no output)

$ go test $(go list ./... | grep -v /node_modules/) -count=1
... identical result to the working-tree run above (I20 only) ...
```

**What stayed warm**: the shared Go module cache (`$GOPATH/pkg/mod`) and
build cache (`$GOCACHE`) — legitimate dependency/compiler caches, not test
artifacts or fixture state. The git working tree, database files (each
test opens its own fresh temp-file SQLite via `t.TempDir()`), and the
clone's own directory were all freshly created; nothing from my original
working tree's build was reused there.

## The three required attacks

### Attack 1 — revert `ListKeys` to the old (broken) owner-scoped semantics

Per this engagement's standing discipline: temporarily reverted
`KeysServer.ListKeys` to call `s.Service.ListAPIKeys(ctx, ownerID)` (the
milestone-2/3 semantics) instead of the new
`s.Service.ListAllAgentAPIKeys(ctx)`:

```go
func (s *KeysServer) ListKeys(c *gin.Context) {
	ownerID, ok := bffOwnerID(c)
	if !ok {
		return
	}

	// ATTACK-REGRESSION: reverting to the old (broken, milestone-2/3)
	// user_id-scoped semantics — the exact defect I21 exists to correct.
	// Reverted immediately after confirming TestBFFHandler_
	// ListKeys_ReturnsEveryAgentsKeys goes red for this reason.
	keys, err := s.Service.ListAPIKeys(c.Request.Context(), ownerID)
	...
```

Confirmed the edit landed (`grep -n "ATTACK-REGRESSION"` printed the
patched line) and the package still compiled (`go build ./...` exit 0)
before trusting the result:

```
$ go test ./internal/transport/bff/... -run TestBFFHandler_ListKeys_ReturnsEveryAgentsKeys -v -count=1
--- FAIL: TestBFFHandler_ListKeys_ReturnsEveryAgentsKeys (0.09s)
    keys_handler_test.go:88: owner's key listing must include agent-alpha's
      key — got []
    keys_handler_test.go:89: owner's key listing must include agent-beta's
      key — got []
    keys_handler_test.go:90: "[]" should have 2 item(s), but has 0
FAIL
```

Went red for the right reason: **empty list, not a wrong count** — the
session owner's own `user_id` structurally never matches any agent's key,
so the old query returns nothing at all. This is exactly the failure mode
I21 exists to correct and the exact one GOAL.md's own attack standard
names.

Reverted the edit, confirmed `grep -c "ATTACK-REGRESSION"
internal/transport/bff/keys_handler.go` → `0`, rebuilt clean, reran with
`-count=1`:

```
$ go test ./internal/transport/bff/... -run TestBFFHandler_ListKeys_ReturnsEveryAgentsKeys -v -count=1
--- PASS: TestBFFHandler_ListKeys_ReturnsEveryAgentsKeys (0.05s)
PASS
```

### Attack 2 — Done-when 11's both-halves test: make revocation a DB-write no-op

Temporarily replaced `KeysServer.RevokeKey`'s real body with an early
return that answers `204` without ever calling
`s.Service.RevokeAnyAgentAPIKey` — simulating "a revoke button that writes
nothing," the exact ENG-13 shape GOAL.md's Done-when 11 names by name:

```go
func (s *KeysServer) RevokeKey(c *gin.Context, id string) {
	if _, ok := bffOwnerID(c); !ok {
		return
	}

	// ATTACK-NOOP-REVOKE: skip the actual DB write entirely and answer 204
	// as if it worked — simulating a revoke button wired to nothing (the
	// exact ENG-13 shape Done-when 11 exists to catch). Reverted
	// immediately after confirming TestDoneWhen11_
	// RevocationActuallyStopsTheKey_BothHalves goes red for this reason.
	c.Status(http.StatusNoContent)
	return

	if _, err := s.Service.RevokeAnyAgentAPIKey(c.Request.Context(), id); err != nil {
	...
```

Confirmed the edit landed (`grep -n "ATTACK-NOOP-REVOKE"`) and the package
still compiled (`go build ./...` exit 0 — an unreachable statement after
`return` is not a Go compile error) before trusting the result:

```
$ go test ./internal/transport/bff/... -run TestDoneWhen11_RevocationActuallyStopsTheKey_BothHalves -v -count=1
--- FAIL: TestDoneWhen11_RevocationActuallyStopsTheKey_BothHalves (0.13s)
    keys_handler_test.go:203:
        Error:  Not equal:
                    expected: 401
                    actual  : 200
        Messages: the same key must fail authentication after revocation —
          this is Done-when 11's whole point
FAIL
```

Went red on the exact assertion this test exists to guard: the fake `204`
fooled nothing — the raw key still authenticated with a genuine `200`
against the real `/api/v1/keys` after the sham "revocation." This is the
test's second, negative half catching the attack; its first, positive half
(`beforeRec` must be `200`) had already been exercised as a real assertion
earlier in the same run, so this failure means the key really did keep
working, not that the test's setup was broken.

Reverted the edit, confirmed `grep -c "ATTACK-NOOP-REVOKE"
internal/transport/bff/keys_handler.go` → `0`, rebuilt clean, reran with
`-count=1`:

```
$ go test ./internal/transport/bff/... -run TestDoneWhen11_RevocationActuallyStopsTheKey_BothHalves -v -count=1
--- PASS: TestDoneWhen11_RevocationActuallyStopsTheKey_BothHalves (0.18s)
PASS
```

### Attack 3 — I21's own scope-fix mechanism: correct placement accepted, wrong placement rejected

Two sub-attacks against `internal/invariants_test.go`'s
`TestDoneWhen12_EveryInvariantHasANamedTest`, per the task brief's explicit
instruction to prove this mechanism (built during the milestone-4
scope-tags fix-round) actually works rather than trust it was built
correctly — this is the first task that ever writes a real `TestI21_` test,
so neither sub-attack had been exercised before.

**Sub-attack 3a — move the correctly-placed test out.** Temporarily
renamed `TestI21_ListAllAgentAPIKeys_SpansEveryAgent_ListAPIKeysByOwner_StaysSelfScoped`
(in `internal/identity/repo_test.go`) to
`AttackI21MovedOut_ListAllAgentAPIKeys_...` — a name that no longer matches
the `Test\w+` regex `TestDoneWhen12` scans for, simulating the test having
been moved out of `internal/identity` entirely:

```
$ go build ./... && go test ./internal/ -run TestDoneWhen12 -v -count=1
--- FAIL: TestDoneWhen12_EveryInvariantHasANamedTest
    invariants_test.go:417: no test named TestI20_<something> ...
    invariants_test.go:450: no test named TestI21_<something> found inside
      .../internal/identity (scope: domain:identity) — I21 requires a
      dedicated test in that specific package (Done-when 12)
FAIL
```

I21's failure reappeared exactly as it was before this task started.
Reverted the rename, confirmed `go test ./internal/ -run TestDoneWhen12`
was back to I20-only.

**Sub-attack 3b — wrong-package stub.** Added a throwaway
`internal/domain/todo/attack_i21_stub_test.go` containing exactly
`func TestI21_Stub(t *testing.T) {}`. Running `TestDoneWhen12` with
*both* the real test in `internal/identity` and this stub present passed
trivially (the real test alone already satisfies the check) — so this
sub-attack is only meaningful isolated: with the real
`internal/identity` test renamed out too (same rename as 3a), the stub
became the *only* `TestI21_`-named test anywhere in the repo:

```
$ go build ./... && go test ./internal/ -run TestDoneWhen12 -v -count=1
--- FAIL: TestDoneWhen12_EveryInvariantHasANamedTest
    invariants_test.go:417: no test named TestI20_<something> ...
    invariants_test.go:450: no test named TestI21_<something> found inside
      .../internal/identity (scope: domain:identity) — I21 requires a
      dedicated test in that specific package (Done-when 12)
FAIL
```

**The wrong-package stub did NOT satisfy the check** — `TestDoneWhen12`
still reported I21 uncovered, correctly refusing to accept a `TestI21_`
that exists somewhere in the repo but not in the specific package
`domain:identity` resolves to. This is exactly the shortcut the OLD
(pre-scope-tags-fix) `per-domain-module`-only mechanism would have
silently accepted (any domain module's own `TestI3_`-shaped test used to
satisfy I3 for every other domain module too, until that mechanism was
replaced — see `internal/invariants_test.go`'s own doc comment on
`TestDoneWhen12`).

Reverted both halves: renamed
`AttackI21MovedOut_...`/`AttackI21WrongPackageIsolation_...` back to
`TestI21_ListAllAgentAPIKeys_SpansEveryAgent_ListAPIKeysByOwner_StaysSelfScoped`,
deleted `internal/domain/todo/attack_i21_stub_test.go` entirely, confirmed
`git grep -n "ATTACK-"` across the whole tree returns no matches, rebuilt
clean, and reran the full green gate one final time (see above — I21 no
longer appears, only I20).

**Summary of Attack 3's two results, stated plainly:**

- Correct placement (`internal/identity`) → **accepted**: `TestDoneWhen12`
  passes I21's check.
- Wrong placement (`internal/domain/todo`, isolated so it's the only
  `TestI21_`-named test in the repo) → **rejected**: `TestDoneWhen12`
  still fails, naming I21 as uncovered, with the exact same error message
  as before any `TestI21_` test existed anywhere.

## What I did not establish

- **No frontend/SPA verification.** This task is Go/BFF-only, matching
  `_plan/_todo.md`'s task-6 scope; the settings page's actual UI wiring to
  this endpoint (task-7's scope) is not exercised here. มายด์'s own
  acceptance walkthrough item 1 ("sees every agent's key in Settings and
  revokes one, and that key stops working") is only proven at the HTTP/API
  layer by this task — the browser-driven half is task-7's and มายด์'s own
  to verify.
- **Did not run the JS/Vitest suite** — out of this task's Go-only green
  gate.
- **Did not investigate I20 beyond confirming it matches the declared
  baseline** — task-7's, explicitly out of scope here, untouched.
- **`RevokeAPIKeyByID`'s lack of a role filter (see Decisions above) is a
  reasoned choice, not something exercised by an attack.** I did not
  construct a test proving "an owner-role user's hypothetical key survives
  a revoke attempt because the query has no role filter," because I2
  already makes that state unreachable through any real code path — there
  is no fixture-through-a-real-path way to create it. Naming this rather
  than silently assuming the absence of a role filter is risk-free.
- **Did not re-verify Done-when 8/9/10/12/13/14** — this task's own scope
  is exactly Done-when 7 and 11; the others are prior or later tasks'.
- **Reachability check against a real running binary (`cmd/server`) was
  not performed for this task specifically** — the cold-clone
  `go test`/`go build` verification above, plus the full real-middleware-chain
  HTTP round trips inside `TestDoneWhen11_RevocationActuallyStopsTheKey_BothHalves`
  (real `RequireActor`/`RequireJSONSession` middleware, real openapi
  request validators, real temp-file SQLite — not mocks or service-layer
  shortcuts), is the load-bearing evidence here. A live-binary `curl`
  walkthrough was judged redundant given that, but it is a narrower
  instrument than a spawned OS process, and I'm naming that boundary
  rather than letting the test-suite result imply more than it does.

## Ambiguities flagged (not silently resolved)

- Which `/api/v1/...` endpoint to use for Done-when 11's positive-half
  real-HTTP round trip (`/keys` vs. `/todos`) was not specified by name in
  the task brief. I chose `/api/v1/keys` — see Decisions above for why —
  but this was a judgment call, not something the contract dictated.
- `RevokeAPIKeyByID`'s query has no `role='agent'` SQL-level filter,
  relying on I2 making an owner-held key structurally unreachable rather
  than defending against it directly in this query. Flagged in Decisions
  above as a choice made, not a gap found and left.

## Commits pushed (branch `milestone-4/activity-log`)

- `3c244bc` — `feat(milestone-4/task-6): I21 - owner-facing key-listing spans every agent's keys`
