# Task task-5 Report

## Task

`GET /api/bff/activity` — the cross-todo activity feed: a cursor over
`todo_events` across every todo, newest first, joined to `todos` (title)
and `users` (actor handle/role). Owner-session only, mirrors my-task's
`activity.list` (no agent-facing/REST equivalent on either surface).
Owns GOAL.md's **Done-when 12**: "The feed is proven cross-actor, not
merely dual-page" — a test seeding a real agent identity through
`cmd/issue-key`'s real path, having that agent act on a todo, and
asserting the owner's feed query returns that event correctly attributed
to the agent.

## Outcome

done

## What was already there vs. what this task built

`internal/domain/todo`'s SQL query (`db/queries/todo_events.sql`'s
`ListTodoEventsFeed`), the sqlc-generated `internal/db/todo_events.sql.go`,
and both the repo wrapper (`Repo.ListEventsFeed`) and the service wrapper
(`Service.ListFeed`) already existed before this task — built by task-1/
task-2, exactly as the task brief said to check for before writing new
plumbing. `internal/dbquery/tableisolation.go`'s `ReadOnlyGrants` entry for
`{File: "todo_events.sql", Table: "users"}` was also already present. None
of these were touched.

This task's own scope was the transport layer: the `bff-openapi.yaml`
contract addition, the regenerated `internal/bffapi/bffapi.gen.go`, the
`TodoServer.ListActivity` HTTP handler + `toBFFActivityItem` converter in
`internal/transport/bff/todo_handler.go`, and the tests in a new file,
`internal/transport/bff/activity_handler_test.go`.

## Decisions

- **Query-string cursor shape, adapted from my-task's tRPC input —
  flagging this as a genuine contract ambiguity, not silently resolved.**
  `_contract/API.md`'s `GET /api/bff/activity` section specifies the
  *response* shape (`items`, `nextCursor: {createdAtMs, id}`) in full but
  says nothing about the *request*-side query parameter names — the
  contract only says "cursor over `todo_events`... adapted from tRPC to
  this surface's plain-JSON convention" without naming the adaptation.
  I read my-task's actual `activity.ts`/`task.queries.ts` (`limit` default
  30, max 100 via `z.number().int().min(1).max(100)`; `cursor: {createdAtMs,
  id}`) and designed `limit`/`cursorCreatedAtMs`/`cursorId` as flat query
  params that round-trip the exact field names the response's own
  `nextCursor` uses — a defensible, source-mirroring choice, but a choice
  I made, not something the contract dictated. Flagging this explicitly
  rather than treating it as settled: if Luna or Clara wants different
  param names, this is the one place in this task where I filled a real
  gap.
- **`limit`'s 1-100 bound is enforced by `bff-openapi.yaml`'s own
  `minimum`/`maximum` on the parameter (the request validator, mounted
  ahead of every handler on this surface), not re-checked in the
  handler.** The handler only applies the default (30, omitted case) and
  trusts the validator for range — consistent with how every other
  bounded field on this surface (e.g. `title`'s `minLength`/`maxLength`)
  already works. The `cursorCreatedAtMs`/`cursorId` "supplied together or
  neither" rule is NOT expressible in OpenAPI's per-parameter schema, so
  that one check is the handler's own (`bffValidationErrorBody`, `hint:
  "cursor"`), matching this file's existing convention for cross-field
  validation the schema can't carry (e.g. `to` required for
  `status_changed`).
- **Pagination algorithm mirrors my-task's `TaskQueryService.listActivity`
  exactly**: fetch `limit + 1` rows, an extra row present means
  `hasMore`, and the (limit)th row's own `(created_at, id)` becomes
  `nextCursor` — not a separate `COUNT` query or a `has-more` flag
  computed a different way. The existing `ListTodoEventsFeed` SQL query
  (task-1/task-2, untouched) already has a `WHERE (cursor IS NULL OR
  created_at < cursor OR (created_at = cursor AND id < cursor_id))`
  structure that matches this exactly — the first page simply passes both
  cursor args nil, no special-cased "first page" branch needed anywhere
  in the handler.
- **A known, unaddressed edge case, named rather than silently left
  implicit**: the wire cursor (`createdAtMs`) is millisecond-precision by
  contract (mirrors my-task's own `Date.getTime()`), but
  `todo_events.created_at` is written via Go's `time.Now().UTC()`
  (nanosecond precision) by `Repo.InsertEvent` — code this task didn't
  touch and was told not to rewrite. Converting a stored nanosecond-precision
  timestamp to a millisecond-precision wire cursor and back is a lossy
  round-trip: if two events land in the same millisecond, the SQL query's
  equality tie-break (`created_at = cursor_created_at`) compares the
  *reconstructed*, truncated cursor against the *original*,
  full-precision stored value, which will not be bit-for-bit equal even
  though they represent "the same" boundary row. In practice this only
  bites when two events are written within the same real millisecond
  *and* a page boundary happens to fall exactly there — my own
  pagination test (`TestBFFHandler_ListActivity_OrderingAndPagination`)
  did not hit it because the sequential writes in that test scenario
  landed on distinct milliseconds every time it ran, not because the
  boundary case is proven safe. I did not fix this (fixing it cleanly
  would mean either truncating `Repo.InsertEvent`'s own timestamp
  precision — a shared write path task-5 wasn't told to touch — or
  loosening the SQL's tie-break, which the task brief explicitly said not
  to rewrite) and I'm naming it here rather than leaving it as a silent
  gap.
- **`ActivityItem` carries `body`, not shown in `_contract/API.md`'s one
  illustrative example (a `status_changed` event, which has no body).**
  Read my-task's actual `task.queries.ts:522-568` (`listActivity`) to
  confirm: it does include `body` on rows where it's non-null, needed for
  a `commented` event to render meaningfully on the feed at all — Done-when
  9's raw-HTML-escaping requirement and Done-when 8's shared-row-component
  requirement both presuppose the feed actually carries a comment's body,
  not just its metadata. Included as a straightforward extension matching
  the cited source, not a deviation from it.
- **Test-fixture discipline, applied to this task's own new test.** The
  cross-actor test seeds its agent through
  `identity.Service.IssueAPIKeyForHandle` — the exact method
  `cmd/issue-key/main.go`'s own `run()` calls (line 123) — not
  `repo.CreateUser(ctx, handle, "agent", nil)` directly. This is
  deliberately **not** the same helper
  `internal/transport/publicapi/publicapi_testutil_test.go`'s
  `createAgentWithKey` uses (that helper calls `repo.CreateUser` directly
  and is used by many pre-existing tests in that package — none of which
  this task touches or needed to). The agent's actions
  (`POST /api/v1/todos`, `POST /api/v1/todos/:id/events`) go through a
  real `/api/v1` router built in the new test file
  (`newAgentPublicAPIRouter`), mounting `publicapi.TodoServer`'s routes
  directly behind `RejectActorFields`/`RequireActor`/the real openapi
  request validator — the same middleware chain production traffic hits,
  not a service-layer shortcut that skips HTTP entirely.

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

### Green gate — unfiltered `go test ./internal/...`, no `-run`

```
$ go test ./internal/... -count=1
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
FAIL
```

Matches this task's own baseline instruction exactly (anchored at
`d84be82`, reconfirmed at `0039dcd` before starting): the only failure is
`TestDoneWhen12_EveryInvariantHasANamedTest`, failing on exactly `I20`
and `I21` — out of scope for this task, untouched.

Also ran the Makefile's own broader package set
(`go list ./... | grep -v /node_modules/`), matching `cmd/*` and
`db/migrations` too:

```
ok  	github.com/mildronize/my-template/cmd/issue-key
ok  	github.com/mildronize/my-template/cmd/seed
ok  	github.com/mildronize/my-template/cmd/server
?   	github.com/mildronize/my-template/cmd/smoke	[no test files]
?   	github.com/mildronize/my-template/db/migrations	[no test files]
(...internal/* identical to above...)
?   	github.com/mildronize/my-template/web	[no test files]
FAIL   (TestDoneWhen12_EveryInvariantHasANamedTest only, same I20/I21)
```

### `internal/transport/bff` — full `-v` listing (this task's own package)

```
--- PASS: TestDoneWhen12_ActivityFeed_CrossActorAttribution (0.07s)
--- PASS: TestBFFHandler_ListActivity_OrderingAndPagination (0.04s)
--- PASS: TestBFFHandler_ListActivity_MalformedCursorRejected (0.10s)
    --- PASS: .../api/bff/activity?cursorCreatedAtMs=12345
    --- PASS: .../api/bff/activity?cursorId=some-id
--- PASS: TestBFFHandler_ListActivity_Unauthenticated_Returns401 (0.09s)
--- PASS: TestI11_CallbackNeverExchangesWithoutStateCookie (0.21s)
--- PASS: TestI12_BFFSessionNeverResolvesToAgent_Callback (0.10s)
--- PASS: TestCallback_UnrecognizedSubIsAnErrorPageNeverAJITRow (0.19s)
--- PASS: TestCallback_SuccessfulOwnerLoginSetsSessionAndRedirects (0.09s)
--- PASS: TestBFFHandler_ListKeys_ReturnsOwnersOwnKeys (0.18s)
--- PASS: TestBFFHandler_RevokeKey_ThenListNoLongerShowsIt (0.21s)
--- PASS: TestI3_BFFHandlerOwnershipScoping_ReturnsNotFoundNotForbidden_Keys (0.10s)
--- PASS: TestI11_LoginRedirectAlwaysIncludesPKCEChallenge (0.04s)
--- PASS: TestLogin_MissingConfigShowsErrorNotACrash (0.06s)
--- PASS: TestSecureFromURL_FollowsConfiguredScheme (0.00s)
--- PASS: TestNegativeCheck_NoKeyIssuanceOrRotateEndpointOnBFFSurface (0.08s)
--- PASS: TestBFFHandler_FullCRUDRoundTrip_RealSessionCookie (0.11s)
--- PASS: TestI3NoLongerApplies_BFFHandlerReadsEveryTodoRegardlessOfCreator (0.17s)
--- PASS: TestBFFHandler_ListTodos_Unauthenticated_Returns401NotRedirect (0.19s)
--- PASS: TestBFFHandler_CreateTodo_DoneFieldRejected (0.13s)
--- PASS: TestBFFHandler_UpdateTodo_DoneFieldRejected (0.22s)
--- PASS: TestDoneWhen14_CreateTodoEvent_TypeCreatedRejected (0.09s)
--- PASS: TestI18_BFF_OwnerCanCloseTodo (0.21s)
--- PASS: TestBFFHandler_EventsRoundTrip_CreateReadAppendFieldChangedReadTimeline (0.06s)
--- PASS: TestDoneWhen6_DeleteTodo_RouteGenuinelyDoesNotExist_BFF (0.09s)
PASS
ok  	github.com/mildronize/my-template/internal/transport/bff	2.845s
```

25 top-level tests (4 new, 21 pre-existing), 0 failures.

### Cold-clone verification

Committed (`65d69e5`), pushed to `origin/milestone-4/activity-log`, then
cloned that exact commit fresh into an isolated directory
(`/tmp/.../scratchpad/coldclone/repo`, not a cleaned copy of the working
tree) and re-ran both `go build ./...` and the full `go test` sweep there:

```
$ git clone --branch milestone-4/activity-log <repo> repo
$ cd repo && git log -1 --format=%H
65d69e5d17a79f614601738e9f55f812b984241e

$ go build ./...
(exit 0, no output)

$ go test $(go list ./... | grep -v /node_modules/) -count=1
... identical result to the working-tree run above (I20/I21 only) ...
```

**What stayed warm**: the shared Go module cache (`$GOPATH/pkg/mod`) and
build cache (`$GOCACHE`) — legitimate dependency/compiler caches, not test
artifacts or fixture state. The git working tree, database files (each
test opens its own fresh temp-file SQLite via `t.TempDir()`), and build
output directory were all freshly created by the clone; nothing from my
original working tree's build was reused there.

### `bffapi.gen.go` regeneration — diffed, not trusted blind

Ran the Makefile's exact invocation
(`./bin/oapi-codegen -generate types,gin,spec -package bffapi -o
internal/bffapi/bffapi.gen.go bff-openapi.yaml`) and diffed the result:

- New types: `ActivityActor`, `ActivityCursor`, `ActivityFeed`,
  `ActivityItem`, `ActivityTodoRef`, `ListActivityParams`.
- New `ServerInterface` method `ListActivity(c *gin.Context, params
  ListActivityParams)`, new wrapper (`ServerInterfaceWrapper.ListActivity`,
  binding `limit`/`cursorCreatedAtMs`/`cursorId` from the query string),
  new route registration (`router.GET(options.BaseURL+"/activity",
  wrapper.ListActivity)`).
- Grepped the diff for `Key`/`Todo` (non-Activity) symbols: `ListKeys`,
  `RevokeKey`, `ApiKey`, `ApiKeyList`, `Todo`, `TodoList`, `TodoEvent`,
  `TodoEventList`, `CreateTodoRequest`, `UpdateTodoRequest`,
  `CreateTodoEventRequest` are all byte-for-byte unchanged — confirmed no
  incidental scope leakage into surfaces this task doesn't own.
- The embedded base64 spec blob changed (expected — it's the compiled
  spec) and nothing else outside the Activity-shaped additions appeared.

### Attack 1 — negative direction: does the feed actually show cross-actor events?

Per this engagement's standing discipline: temporarily edited
`TodoServer.ListActivity` to scope the feed to the viewer's own events
only — the exact bug Done-when 12's test exists to catch:

```go
// ATTACK-NEGATIVE-DIRECTION: temporarily scope to the viewer's own
// events only, simulating the exact bug Done-when 12's cross-actor
// test exists to catch. Reverted immediately after confirming red.
viewerID, _ := bffOwnerID(c)

resp := bffapi.ActivityFeed{Items: make([]bffapi.ActivityItem, 0, len(rows))}
for _, row := range rows {
    if row.Event.ActorID != viewerID {
        continue
    }
    item, err := toBFFActivityItem(row)
    ...
```

Confirmed the edit landed (`grep -n "ATTACK-NEGATIVE-DIRECTION"` printed
the patched line) and the package still compiled (`go build ./...` exit
0) before trusting the result. Ran the target test against the planted
bug:

```
$ go test ./internal/transport/bff/... -run TestDoneWhen12_ActivityFeed_CrossActorAttribution -v
--- FAIL: TestDoneWhen12_ActivityFeed_CrossActorAttribution (0.13s)
    activity_handler_test.go:202:
        Error:  Should NOT be empty, but was []
        Messages: the owner's feed must not be trivially empty — the agent
          just wrote two events into the shared collection
FAIL
```

Went red for the right reason: with the scoping bug in place, the owner
(who wrote zero events itself in this test — only the agent acted) got
back an empty feed, exactly the failure mode this test exists to catch.
Reverted the edit, confirmed `grep -c "ATTACK-NEGATIVE-DIRECTION\|viewerID"
internal/transport/bff/todo_handler.go` → `0` (no leftover marker), rebuilt
clean, reran with `-count=1` (bypassing Go's test cache) to confirm green
again:

```
$ go test ./internal/transport/bff/... -run TestDoneWhen12_ActivityFeed_CrossActorAttribution -v -count=1
--- PASS: TestDoneWhen12_ActivityFeed_CrossActorAttribution (0.11s)
PASS
```

### Attack 2 — positive direction: does the test still fail against a silently-empty feed?

The specific failure mode Clara flagged as commonly skipped: a handler
that always answers `200 {"items":[]}` regardless of what's actually in
the table (e.g. silently swallowing an internal error). A test that only
asserts "if found, the fields match" — never separately requiring that
something *was* found — would pass vacuously against this. Temporarily
replaced the entire handler body with exactly that:

```go
func (s *TodoServer) ListActivity(c *gin.Context, params bffapi.ListActivityParams) {
	if _, ok := bffOwnerID(c); !ok {
		return
	}

	// ATTACK-POSITIVE-DIRECTION: simulate a handler that silently
	// swallows and always answers 200 with an empty feed, regardless of
	// what's actually in todo_events. Reverted immediately after
	// confirming red.
	c.JSON(http.StatusOK, bffapi.ActivityFeed{Items: []bffapi.ActivityItem{}})
	return

	limit := int64(bffActivityDefaultLimit)
	... (rest of the real implementation, now unreachable)
```

Confirmed the edit landed (`grep -n "ATTACK-POSITIVE-DIRECTION"`) and the
package still compiled (`go build ./...` exit 0 — an unreachable
statement after `return` is not a Go compile error) before trusting the
result:

```
$ go test ./internal/transport/bff/... -run TestDoneWhen12_ActivityFeed_CrossActorAttribution -v
--- FAIL: TestDoneWhen12_ActivityFeed_CrossActorAttribution (0.05s)
    activity_handler_test.go:202:
        Error:  Should NOT be empty, but was []
        Messages: the owner's feed must not be trivially empty — the agent
          just wrote two events into the shared collection
FAIL
```

The test failed on the explicit `require.NotEmpty(t, feed.Items, ...)`
check written specifically to guard this — not vacuously, not on an
unrelated panic, and not by accidentally passing. This is the assertion
that makes the test's later "find the agent's event, require it exists"
loop meaningful: without it, a broken empty-response handler could in
principle reach that loop, find nothing, and (if the test only checked
fields conditionally on `found`) pass anyway.

Reverted the edit, confirmed no `ATTACK-POSITIVE-DIRECTION` marker
remains, rebuilt clean, reran with `-count=1`:

```
$ go test ./internal/transport/bff/... -run TestDoneWhen12_ActivityFeed_CrossActorAttribution -v -count=1
--- PASS: TestDoneWhen12_ActivityFeed_CrossActorAttribution (0.07s)
PASS
```

Both attacks (negative-direction: real scoping bug; positive-direction:
silently-empty response) were caught, and neither leftover marker exists
in the final diff — confirmed by `git diff internal/transport/bff/todo_handler.go`
showing only the permanent implementation, no `ATTACK-` strings anywhere
in `git grep ATTACK-` across the whole tree.

## What I did not establish

- **The query-param cursor names (`cursorCreatedAtMs`/`cursorId`) are my
  own design, not literally specified by `_contract/API.md`.** Flagged
  above under Decisions — this is the one real ambiguity I resolved with
  judgment rather than finding settled in the contract.
- **The millisecond-vs-nanosecond cursor precision edge case is named,
  not fixed.** See Decisions above — I did not modify `Repo.InsertEvent`
  (a shared write path outside this task's scope, and the SQL query
  itself was explicitly not mine to rewrite), so a page boundary that
  happens to land between two events sharing the exact same millisecond
  is a real, currently-unguarded gap. Did not attempt to construct a test
  that reliably reproduces it (doing so deterministically would require
  either mocking `time.Now()` inside `Repo.InsertEvent` or a database-level
  fixture that writes `created_at` directly, both of which reach outside
  this task's stated scope).
- **Did not build or exercise any frontend/SPA code for this feed.**
  `_plan/_todo.md`'s task-5 entry is Go/BFF-only; the SPA's own
  activity-feed page (sharing `TimelineEventRow` with the per-todo
  timeline, GOAL.md Done-when 8) is a later task's scope, not verified
  here.
- **Did not re-verify Done-when 8's "same row component" requirement**
  beyond confirming `ActivityItem`'s wire shape carries the same
  `type`/`payload`/`body`/`actor` semantics `TodoEvent` does — the actual
  shared-rendering-component proof is a frontend test, out of this task's
  (Go-only) scope.
- **Did not investigate I20/I21 beyond confirming they match the
  declared baseline** — not this task's to fix.
- **Did not run the JS/Vitest suite** — this task's own green gate
  (`_plan/_todo.md`) is Go-only.
- **Reachability check against a real running binary was not performed**
  for this task specifically (unlike task-4's own check) — the cold-clone
  `go test`/`go build` verification above is the load-bearing evidence for
  this task; a live-binary `curl` walkthrough of `/api/bff/activity` was
  judged redundant given the handler-level integration tests already
  exercise the real router/middleware chain (`newTestRouter`,
  `bffapi.RegisterHandlers`) against a real (temp-file) SQLite database,
  not a mock — but it is a narrower instrument than a spawned OS process,
  and I'm naming that boundary rather than letting the cold-clone result
  imply more than it does.

## Commits pushed (branch `milestone-4/activity-log`)

- `65d69e5` — `feat(milestone-4/task-5): cross-todo activity feed (GET /api/bff/activity)`
