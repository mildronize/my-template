# Handle-exposure fix-round report

## Task

Engineering director's finding: all three "contract gaps" task-7's report
named (`ApiKey` with no `handle`, `TodoEvent`'s per-todo-timeline `actor`
with no `handle`/`role`, `Todo.assigneeId`/the `assigned` event's
`{from, to}` payload as raw ids) are narrowings of "like my-task," the
engagement's standing rule all along — not new scope, not gaps to be
flagged and left. Fix all three, matching what my-task's own source
actually does at each site, citing it, not inventing a shape.

## Outcome

done

## Gap 1 — `ApiKey` gains the owning agent's `handle`

### What my-task actually does

`~/gits/my-task/src/app/(app)/settings/api-key-settings.tsx`:

- `ApiKeyRow` (line 88): `<span className="truncate">{k.handle}</span>` —
  the row's own primary text, not a secondary detail.
- `RevokeKeyButton` (line 57): `<AlertDialogTitle>Revoke {handle}&apos;s
  key?</AlertDialogTitle>` — verbatim copy, the handle is the sentence's
  subject.
- `RevokeKeyButton`'s `onSuccess` toast (line 41): `` `Revoked
  ${res.handle}'s key.` ``.

### What I built

- `db/queries/api_keys.sql`'s `ListAllAgentAPIKeys` — the query's own
  comment already said it selects `api_keys.*` deliberately to keep the
  return type `db.ApiKey`; changed the select list to `api_keys.*,
  users.handle AS handle`, still an explicit column list (not a bare
  `SELECT *` against the join), so sqlc now emits a query-scoped
  `ListAllAgentAPIKeysRow` (`ID, UserID, KeyHash, KeyPrefix, CreatedAt,
  ExpiresAt, RevokedAt, Handle`) instead of reusing `db.ApiKey`. No new
  `ReadOnlyGrant` needed — `api_keys`/`users` are both owned by the
  `identity` module (`internal/dbquery/tableisolation.go`'s
  `TableOwnership`), so a same-module JOIN needs no grant at all, unlike
  the `todo`-module cases below.
- `internal/identity/repo.go`: `APIKey` gained `Handle *string`. **Chose
  a pointer field on the shared type, not a second,
  `ListAllAgentAPIKeys`-only return type** — every consumer already
  handles `APIKey` as one shape (`RevokeAnyAgentAPIKey`,
  `keys_handler.go`'s `toBFFKey`), and a second parallel type would need
  either its own `toBFFKey` twin or a lossy conversion back to `APIKey`
  before reaching that mapper. Every method except `ListAllAgentAPIKeys`
  leaves `Handle` nil — not a guessed or empty-string value — so a caller
  can tell "this method never resolves a handle" apart from a
  hypothetical future empty one. Stated as a judgment call, not silently
  picked: a fork adding a second identity-carrying `APIKey`-returning
  method later might reasonably revisit this in favor of a narrower type,
  but for one method today the shared-type-plus-pointer shape was less
  code and no less honest.
- `internal/transport/bff/keys_handler.go`'s `toBFFKey` now sets
  `Handle: *k.Handle` — asserted non-nil (not defended against with a
  fallback), since `toBFFKey`'s only caller is `ListKeys`, whose only
  data source (`ListAllAgentAPIKeys`) uses `JOIN`, not `LEFT JOIN` — a
  nil `Handle` reaching this function would mean that query's own JOIN
  returned an orphan row, a bug in the query, not a normal runtime
  condition to paper over.
- `bff-openapi.yaml`'s `ApiKey` schema gained `handle` (required, not
  nullable — always present for this endpoint's own JOIN).
  `internal/transport/publicapi`'s own `ApiKey` schema (the self-scoped,
  agent-facing `GET /api/v1/keys`) was **not** touched — out of scope per
  the brief, and I agree with that scoping: that surface was never named
  in the original finding, and every key it can ever return already
  belongs to the caller itself (a real name would repeat information the
  caller already has, not add any).
- `web/src/app/settings/ApiKeySettings.tsx`: the row's primary text is
  now `k.handle` (prefix/dates demoted to the secondary line — the
  reverse of the pre-fix-round layout); the revoke dialog title is
  `` Revoke {handle}'s key? `` verbatim, matching my-task's copy exactly
  (not "necessarily its exact wording" per the brief's allowance, but it
  turned out to match word-for-word once translated to this repo's own
  apostrophe/JSX conventions).

### Real end-to-end proof

`internal/transport/bff/keys_handler_test.go`'s
`TestBFFHandler_ListKeys_ReturnsEveryAgentsKeys` — extended to assert on
the actual response body, not just presence:

```go
assert.Equal(t, "agent-alpha", byID[issuedA.APIKey.ID].Handle, "...")
assert.Equal(t, "agent-beta", byID[issuedB.APIKey.ID].Handle)
```

Ran:

```
$ go test ./internal/transport/bff/... -run TestBFFHandler_ListKeys_ReturnsEveryAgentsKeys -v
--- PASS: TestBFFHandler_ListKeys_ReturnsEveryAgentsKeys (0.12s)
PASS
```

Two real agent identities issued through `identity.Service.
IssueAPIKeyForHandle` (the same method `cmd/issue-key`'s own `run` calls
— the existing fixture discipline this file's own header comment already
established, reused rather than re-litigated), a real HTTP `GET
/api/bff/keys` through the real router, and the response body's `handle`
field genuinely names the right agent for each row — not "the type
allows it."

Frontend: `web/src/app/settings/ApiKeySettings.test.tsx` (new) renders
the component against a mocked `/api/bff/keys` response and asserts the
handle appears as visible text, then clicks the real Revoke button and
asserts the dialog's title reads exactly `Revoke clara's key?`.

```
$ npx vitest run src/app/settings/ApiKeySettings.test.tsx
Test Files  1 passed (1)
     Tests  1 passed (1)
```

### Regenerated-file diff

`internal/db/api_keys.sql.go` — only `ListAllAgentAPIKeys` changed: a new
`ListAllAgentAPIKeysRow` type with the extra `Handle string` field, the
function's return type updated to it. `GetAPIKeyByHash`,
`ListAPIKeysByOwner`, `CreateAPIKey`, `RevokeAPIKey`,
`RevokeAPIKeyByID` — untouched (confirmed via `git diff internal/db/`,
only 4 files touched total across all three gaps, all expected).
`internal/bffapi/bffapi.gen.go` — only `ApiKey`'s struct gained a
`Handle string` field (plus the embedded swagger-spec blob, which always
changes on regeneration regardless of which field moved).

## Gap 2 — `TodoEvent`'s per-todo-timeline actor gains `handle`/`role`

### What my-task actually does

Two different shapes for two different surfaces, not one — this was the
one place I found my-task's own behavior more nuanced than the brief's
framing suggested, and matched that nuance rather than picking the
simpler-looking option:

- **Owner-facing tRPC (`task.byRef`)**: `~/gits/my-task/src/server/api/
  routers/task.ts`'s `withActorRoles` (lines 91-101) — a follow-up query
  resolving each distinct actor handle to a role, applied ONLY at the
  tRPC router layer, on top of `TaskQueryService.getTaskDetail()`'s
  shared result. Its own doc comment (lines 82-90) states why: `actor` is
  a plain handle string because "that shape is REST's documented JSON
  contract" and the shared service can't change it without breaking
  REST — so role-based marking is bolted on separately, owner-UI only.
- **Agent-facing REST (`GET /api/v1/tasks/:id`,
  `~/gits/my-task/src/app/api/v1/tasks/[id]/route.ts`)**: calls the same
  `getTaskDetail()` directly, `serializeTaskDetail` (`response.ts`)
  passes `e.actor` through unchanged — a bare handle string, **no
  role at all**. Confirmed also on `GET /api/v1/tasks`'s list endpoint
  (`route.ts:128`: `assignee: t.assignee`, same asymmetry for the
  assignee field, addressed in gap 3).

So my-task shows agents WHO acted (a handle), but withholds the
human/agent role marker from them — that marker is owner-UI-only.

### What I built

- `db/queries/todo_events.sql`'s `ListTodoEventsByTodoID` — changed from
  `SELECT * ... WHERE todo_id = ?` to an explicit column list `JOIN
  users`, the identical shape `ListTodoEventsFeed` (same file) already
  uses for the cross-todo feed. No new `ReadOnlyGrant` needed — this file
  already had one for `users` (earned by `ListTodoEventsFeed`'s own
  join), and `AssertEveryReadOnlyGrantIsExercised`'s own test confirms a
  grant only needs exercising once per file, not per query.
- `internal/domain/todo/repo.go`: new `TodoEventWithActor{Event
  TodoEvent, ActorHandle, ActorRole string}` — a **new type, not
  `TodoEventFeedRow` reused**, even though the shapes are similar:
  `TodoEventFeedRow` also carries `TodoTitle` (the cross-todo feed's own
  extra context), which the per-todo timeline has no use for — it's
  already scoped to one todo. `ListEventsByTodoID`'s return type changed
  from `[]TodoEvent` to `[]TodoEventWithActor`; `Service.ListEvents`
  followed.
- **Different shape per surface, matching the asymmetry found above:**
  - `internal/transport/bff/todo_handler.go`'s `toBFFEvent` gained an
    `actor bffapi.ActivityActor` parameter (`{handle, role}` — the exact
    same schema `ActivityItem.actor` already uses); `ListTodoEvents` and
    `CreateTodoEvent`'s write-response both populate it (the write
    response's actor is the caller itself, read straight off
    `ActorFromContext`, no extra lookup needed).
  - `internal/transport/publicapi/todo_handler.go`'s `toAPIEvent` gained
    an `actorHandle string` parameter — a bare handle, deliberately no
    role field added, mirroring my-task's own REST asymmetry exactly.
  - `bff-openapi.yaml`'s `TodoEvent` gained `actor: $ref
    ActivityActor` (required). `openapi.yaml`'s own `TodoEvent` gained
    `actorHandle: string` (required) — **not** the `{handle, role}`
    object, on purpose, citing the my-task asymmetry above in the schema
    doc comment itself.
- `web/src/lib/todos.ts`'s `todoEventToTimelineEvent` no longer takes an
  `actor?` second argument — reads `event.actor` directly now that the
  wire genuinely carries one. `web/src/components/TimelineEventRow.tsx`'s
  header/doc comments updated to say the gap is closed, not still open;
  `AssignedSummary` updated for gap 3's new payload shape (below).
- **`ProvenanceMark`'s third ("unknown") state: kept, not removed** —
  judgment call, stated plainly. Every real wire response now supplies
  `"owner"` or `"agent"` on every row, so this branch should never fire
  in production again. But it costs nothing to keep as a guard against a
  genuinely malformed/unexpected response, and `TimelineEventRow.test.tsx`'s
  own Done-when 9 test still exercises it directly — removing the branch
  would also mean deciding what that test proves instead, which felt
  like scope past this fix-round's actual finding.

### Real end-to-end proof

`internal/transport/bff/todo_handler_test.go`'s
`TestBFFHandler_EventsRoundTrip_CreateReadAppendFieldChangedReadTimeline`
— extended with a real `assigned` event append, then asserts on every
row of a real `GET /api/bff/todos/:id/events` response body:

```go
for i, e := range timeline.Events {
    assert.Equalf(t, owner.Handle, e.Actor.Handle, "event %d's actor handle", i)
    assert.Equalf(t, "owner", e.Actor.Role, "event %d's actor role", i)
}
```

```
$ go test ./internal/transport/bff/... -run TestBFFHandler_EventsRoundTrip... -v
--- PASS: TestBFFHandler_EventsRoundTrip_CreateReadAppendFieldChangedReadTimeline (0.06s)
```

`internal/transport/publicapi/todo_handler_test.go`'s equivalent test —
extended similarly, asserting `e.ActorHandle` (bare string, no role) on
every row of a real `GET /api/v1/todos/:id/events` response:

```go
for i, e := range timeline.Events {
    assert.Equalf(t, "agent-a", e.ActorHandle, "event %d's actor handle", i)
}
```

```
$ go test ./internal/transport/publicapi/... -run TestHandler_EventsRoundTrip... -v
--- PASS: TestHandler_EventsRoundTrip_CreateReadAppendFieldChangedReadTimeline (0.02s)
```

Also `internal/domain/todo/repo_test.go`'s
`TestRepo_ListEventsByTodoID_OldestFirst` — extended to assert
`ActorHandle`/`ActorRole` directly off the repo layer, one level below
the handler tests above.

### Regenerated-file diff

`internal/db/todo_events.sql.go` — only `ListTodoEventsByTodoID` changed:
new `ListTodoEventsByTodoIDRow` (base columns plus `ActorHandle,
ActorRole string`), return type updated. `InsertTodoEvent`,
`GetTodoEventMaxSeqByTodoID`, `GetTodoEventByClientRequestID`,
`ListTodoEventsFeed` — untouched. `internal/api/openapi.gen.go` /
`internal/bffapi/bffapi.gen.go` — `TodoEvent` gained `ActorHandle`
(api) / `Actor ActivityActor` (bffapi) respectively; no other struct
changed (`git diff | grep '^\(+\|-\)type '` shows zero type
additions/removals in either file — every change is a field inside an
existing struct).

## Gap 3 — `Todo.assigneeId` and the `assigned` event payload become handles

### What my-task actually does — read carefully, two different shapes for two different things

**The `Task.assignee` field (read side, list + detail, both REST and
tRPC)**: a **flat handle string**, resolved via `LEFT JOIN` at READ
time, not stored. `~/gits/my-task/src/server/modules/task/
task.queries.ts` — `assigneeHandle: assigneeUser.handle` joined in
(lines 185, 324, 593), returned as `assignee: r.assigneeHandle ?? null`
(lines 205, 422, 631). There is no separate `assigneeId` field on the
wire at all — my-task's Task type only ever exposes the handle, because
every write path also accepts a handle (`findUserByHandle`, not a raw
id) — confirmed in `~/gits/my-task/src/app/api/v1/tasks/route.ts` (both
list-filter and create-time assignee resolution) and
`~/gits/my-task/src/app/(app)/tasks/[ref]/page.tsx`'s own `Combobox`
(`value={task.assignee ?? UNASSIGNED}`, options built from `u.handle`).

**The `assigned` event's `{from, to}` payload (write side)**: baked in
at **WRITE time**, not read time, as `{id, handle}` snapshots —
`~/gits/my-task/src/server/modules/task/task.service.ts`'s
`appendAssigned` (lines 340-368) calls `mustGetAssignee` (lines 537-545,
a straight `SELECT id, handle FROM user WHERE id = ?`, throwing
`TaskAssigneeNotFoundError` if the id doesn't resolve) for BOTH `from`
(the current assignee) and `to` (the new one), and stores
`payload = {from, to}` — literal `AssigneeSnapshot{id, handle}` objects,
`JSON.stringify`'d into the row. Rendered via `p.to.handle`/`p.from.handle`
(`~/gits/my-task/src/components/TimelineEvent.tsx:54-60`). **This is a
genuinely immutable historical record**: a later handle change (if
my-task ever allowed renaming a user) would never retroactively rewrite
an old event's payload — the snapshot is what it was at the moment of
the write, permanently. I matched this write-time, not read-time,
choice deliberately, and state the reasoning here because the brief
explicitly warned not to assume it had to match gap 1/2's shape just
because they're read-time joins — it doesn't, and my-task's own source
settles which.

### What I built

**Read side (`Todo.assigneeHandle`, additive, not a replacement for
`assigneeId`):**

- `db/queries/todos.sql`'s `ListTodos`/`GetTodoByID` gained an explicit
  column list + `LEFT JOIN users` for `assignee_handle` (LEFT, not JOIN
  — `assignee_id` is nullable, an unassigned todo must still return, with
  a null handle, not get dropped). New query `GetUserHandleByID` (`SELECT
  id, handle FROM users WHERE id = ?`) for the two follow-up uses below.
  New `ReadOnlyGrant`: `{File: "todos.sql", Table: "users"}` —
  `internal/dbquery/tableisolation.go`, exercised by all three of the
  above.
- `internal/domain/todo/repo.go`: `Todo` gained `AssigneeHandle *string`
  (nil exactly when `AssigneeID` is nil). `List`/`GetByID` populate it
  from the LEFT JOIN. `Create` (no join-friendly `RETURNING *` to
  piggyback on) does a follow-up `ResolveUserHandle` call when
  `AssigneeID` is set — **and degrades to a nil `AssigneeHandle`, not an
  error, if that id doesn't resolve** — `CreateTodo` has never validated
  its `AssigneeID` input names a real user, and I did not add that
  validation here; only `Append`'s `assigned` case (below) gained new
  validation, because that's the specific site the finding named
  (`service.go:337`). Named as an intentional scope boundary, not an
  oversight.
- **Chose a flat, additive `assigneeHandle` field, not a nested `{id,
  handle}` object and not a rename of `assigneeId`** — the brief
  explicitly offered this as one of the shapes to consider. my-task's
  own field is a flat handle string, but it REPLACES an id field that
  never existed there in the first place (every write path speaks in
  handles). This repo's existing write paths (`CreateTodoRequest.
  assigneeId`, `CreateTodoEventRequest`'s `to` for `assigned`) already
  speak in ids and are depended on by existing tests/behavior; renaming
  or restructuring `assigneeId` to match my-task's field-naming exactly
  would be a breaking, larger change the finding didn't ask for. A flat
  additive `assigneeHandle` sibling field gets the same practical result
  (a name where the UI wants one) without that risk. Stated as a
  judgment call, not silently picked.
- `internal/transport/{bff,publicapi}/todo_handler.go`'s
  `toBFFTodo`/`toAPITodo` both gained `AssigneeHandle`. **Both surfaces**,
  not bff-only — `~/gits/my-task/src/app/api/v1/tasks/route.ts:128`
  confirms my-task's own agent-facing REST list endpoint shows
  `assignee: t.assignee` (a handle) too, and `GET /api/v1/tasks/:id`
  reuses the identical `serializeTaskDetail`. This is the one place gap 3
  clearly differs from gap 1 (owner-only, `publicapi` deliberately
  untouched) — read my-task's actual behavior and it names both surfaces.
- `bff-openapi.yaml` and `openapi.yaml` both gained `Todo.assigneeHandle`
  (nullable, required — always present, null exactly when `assigneeId`
  is null).

**Write side (`assigned` event payload, write-time snapshot):**

- `internal/domain/todo/service.go`: new `assigneeSnapshot{ID, Handle
  string}` (JSON `{id, handle}`), new `resolveAssigneeSnapshot(ctx, tx,
  id *string, required bool)` helper, new `ErrUnknownAssignee` sentinel.
  `Append`'s `EventTypeAssigned` case replaced `strPtrToAny(...)` with
  two calls: `resolveAssigneeSnapshot(..., current.AssigneeID, false)`
  for `from`, `resolveAssigneeSnapshot(..., input.Assignment.
  ToAssigneeID, true)` for `to`.
- **The `required` asymmetry is a judgment call the brief flagged as a
  real consequence, stated here explicitly**: `to` is caller-supplied
  input for THIS write — an id that resolves to no real user is now a
  genuine validation error (`ErrUnknownAssignee` → 400
  `validation_error`, mirroring my-task's own `unknownAssigneeError` for
  the identical case). `from` is whatever the todo's `assignee_id`
  already was before this write — a lookup failure there (stale data,
  not this write's fault) degrades to a snapshot whose handle equals the
  raw id, rather than blocking a legitimate reassignment over a data
  anomaly unrelated to it. **This is new, real behavior a caller can now
  observe that didn't exist before this fix-round**: assigning to a
  garbage id used to silently succeed (writing an unresolvable id with no
  validation at all); it now 400s. I judged this correct and matching
  my-task, not a scope overreach, because resolving the handle is
  unavoidable once the payload needs to carry one — there's no version of
  "bake in a handle" that doesn't first require successfully looking one
  up.
- `internal/transport/{bff,publicapi}/todo_handler.go`'s
  `writeBFFAppendError`/`writeAppendError` both gained a branch mapping
  `ErrUnknownAssignee` → 400 `validation_error`, hint `"to"`.
- `web/src/components/TimelineEventRow.tsx`'s `AssignedSummary` updated
  to read `p.to.handle`/`p.from.handle` (object shape) instead of
  `p.to`/`p.from` (bare strings) — mirrors my-task's own
  `AssignedSummary` exactly.

**Frontend display, assignment-setting UI (largest surface, per the
brief's own note):**

- `web/src/app/TodoRow.tsx`: shows `todo.assigneeHandle ?? todo.
  assigneeId` (falls back to the raw id only if a handle genuinely isn't
  available) instead of the raw id unconditionally.
- `web/src/app/todos/TodoDetailPage.tsx`'s `AssigneeControl`: **kept as a
  free-text id input, not built into a picker** — my-task's own task
  detail page uses a `<Combobox>` of known handles
  (`usersQuery`/`u.handle`), but that needs a `GET /api/bff/users`-shaped
  endpoint this milestone's contract doesn't have. Building one
  speculatively is real, separate scope past a fix-round about wire
  shapes — named as a follow-up, not attempted. What DID change: the
  control now shows the current assignee's resolved handle as a label
  below the input (`currently: {handle}`), so at least the display half
  of the gap closes even though the input mechanism doesn't yet.

### Real end-to-end proof

`internal/transport/bff/todo_handler_test.go`'s round-trip test —
extended: creates a todo with `assigneeId: owner.ID`, asserts
`created.AssigneeHandle` equals `owner.Handle` in the real response
body; appends an `assigned` (unassign) event, asserts the real response
body's `payload.from` is a `{id, handle}` object matching the owner.

`internal/transport/publicapi/todo_handler_test.go`'s round-trip test —
extended: creates a todo with a real assignee, asserts
`created.AssigneeHandle == "assignee"` in the real response; appends a
real `assigned` event, asserts both `from` and `to` are `{id, handle}`
objects; **then attempts an `assigned` event with `to: "no-such-user-id"`
and asserts the real HTTP response is `400`**, and that the timeline
afterward still has only 3 events (the rejected attempt wrote nothing):

```go
badAssignRec := doJSONRequest(t, router, http.MethodPost, ".../events", rawKey, map[string]any{
    "type": "assigned", "clientRequestId": "assign-bad-1", "to": "no-such-user-id",
})
assert.Equal(t, http.StatusBadRequest, badAssignRec.Code, "...")
...
require.Len(t, timeline.Events, 3, "the rejected bad-assignee attempt above must not have written a row")
```

```
$ go test ./internal/transport/bff/... -run TestBFFHandler_EventsRoundTrip... -v
--- PASS (0.06s)
$ go test ./internal/transport/publicapi/... -run TestHandler_EventsRoundTrip... -v
--- PASS (0.02s)
```

Frontend: `web/src/app/TodosList.test.tsx` — new test, mocked
`/api/bff/todos` response with `assigneeId`/`assigneeHandle` both set,
asserts the rendered row shows `→ clara`, and explicitly asserts `→
user-abc123` (the raw id) does NOT appear:

```
$ npx vitest run src/app/TodosList.test.tsx
Test Files  1 passed (1)
     Tests  2 passed (2)
```

### Regenerated-file diff

`internal/db/todos.sql.go` — `ListTodos`/`GetTodoByID` both gained an
`AssigneeHandle sql.NullString` field on new query-scoped row types
(`ListTodosRow`, `GetTodoByIDRow`); new `GetUserHandleByID` function/row
type added. `CreateTodo`, `UpdateTodoStatus`, `UpdateTodoAssignee`,
`UpdateTodoPriority`, `UpdateTodoDueDate`, `UpdateTodoTitle` — all
untouched (still return bare `db.Todo`, `RETURNING *`, no join).
`internal/api/openapi.gen.go` / `internal/bffapi/bffapi.gen.go` — `Todo`
gained `AssigneeHandle *string` in both files; no other struct changed.

## Discipline confirmed

- **Every regenerated file diffed before trusting it**: `git diff
  internal/db/` (4 files: `api_keys.sql.go`, `querier.go`,
  `todo_events.sql.go`, `todos.sql.go` — exactly the four query files
  touched, nothing else); `git diff internal/api/openapi.gen.go
  internal/bffapi/bffapi.gen.go | grep '^\(+\|-\)type '` (zero
  additions/removals — every change is a field inside an existing
  struct); `git diff web/src/lib/api/bff-schema.gen.ts` (7 lines: exactly
  `ApiKey.handle`, `Todo.assigneeHandle`, `TodoEvent.actor`, plus their
  doc comments — nothing else in the generated file moved).
- **Go suite, unfiltered, fresh (`-count=1`, not cached)**:

  ```
  $ go test ./internal/... -count=1
  ok  	.../internal	2.038s
  ok  	.../internal/dbquery	0.006s
  ok  	.../internal/domain/todo	0.261s
  ok  	.../internal/identity	1.321s
  ok  	.../internal/platform	0.039s
  ok  	.../internal/transport/bff	4.086s
  ok  	.../internal/transport/publicapi	0.285s
  ```

  Fully green — the same set of packages green as the pre-fix-round
  baseline (`go build ./...` clean, `go vet ./...` clean).
- **JS suite, unfiltered, fresh**:

  ```
  $ npm run typecheck   # tsc -b --noEmit — clean
  $ npx vitest run
  Test Files  6 passed (6)
       Tests  22 passed (22)
  ```

  Baseline was 5 files / 20 tests; this fix-round added
  `ApiKeySettings.test.tsx` (1 test) and one test to `TodosList.test.tsx`
  (1 test) — 6/22, fully green, no regressions.

## Judgment calls, named plainly (repeated here as one list)

1. `identity.APIKey.Handle` — shared type + optional pointer, not a
   second return type. (Gap 1)
2. `publicapi`'s own `ApiKey` schema — left untouched, per the brief's
   own explicit scoping; I agree with it and didn't second-guess it.
   (Gap 1)
3. `TodoEventWithActor` — a new type, not `TodoEventFeedRow` reused,
   because the per-todo timeline has no use for `TodoTitle`. (Gap 2)
4. bff's `TodoEvent.actor` is `{handle, role}`; publicapi's is a bare
   `actorHandle` string, no role — a deliberate asymmetry matching
   my-task's own REST-contract-freeze reasoning, not an oversight or a
   simplification. (Gap 2)
5. `ProvenanceMark`'s third state — kept as defensive handling, not
   removed, now that the gap it existed for is closed. (Gap 2)
6. `assigned`'s payload resolves at WRITE time (immutable snapshot),
   matching my-task's `mustGetAssignee`/`appendAssigned` exactly — not
   assumed to match gap 1/2's read-time-join shape just because they're
   adjacent. (Gap 3)
7. `Todo.assigneeHandle` — flat, additive sibling field, not a rename of
   `assigneeId` and not a nested `{id, handle}` object — chosen to avoid
   a breaking change to the existing id-based write path. (Gap 3)
8. `publicapi`'s `Todo` gains `assigneeHandle` too (unlike gap 1's
   `ApiKey`) — my-task's own agent-facing REST shows assignee handles to
   agents too, confirmed by reading `route.ts` directly, not assumed by
   analogy to gap 1. (Gap 3)
9. `resolveAssigneeSnapshot`'s `required` asymmetry — a lookup failure
   for `to` (400, new caller-visible behavior) vs. `from` (degrades
   gracefully) — a real behavior change this fix-round introduces,
   named rather than left as an implicit side effect. (Gap 3)
10. `CreateTodo`'s own `AssigneeID` input still has no existence
    validation — only `Append`'s `assigned` case gained one, because
    that's the specific site the finding named. Named as an intentional
    scope boundary. (Gap 3)
11. `AssigneeControl` stays a free-text id input, no picker — building a
    real picker needs a `GET /api/bff/users`-shaped endpoint this
    milestone's contract doesn't have; named as a follow-up rather than
    built speculatively. (Gap 3)

## A concurrent push landed on this branch mid-task

While this fix-round was in progress, two commits I did not make appeared
on this branch's local HEAD: `960f741` ("guard the URL false-negative the
comment-strip fix introduced") and `a467782` ("document the
protocol-relative-URL residual, stop hardening the heuristic"), both
timestamped `2026-08-13T17:3x` — before this fix-round's own commit. Both
touch exactly one file: `internal/frontend_safety_test.go`
(`git diff-tree --no-commit-id --name-only -r 960f741 a467782`) — I20's
Go-side static-check heuristic, unrelated to anything in this fix-round's
own scope (identity/todo domain, transport handlers, openapi specs,
frontend). No file overlap with anything I touched, no conflict on
`git push` (a clean fast-forward), and every Go/JS suite run after they
appeared was still fully green. Named here for the same reason task-7's
report named a similar concurrent landing on this branch: worth knowing
about, not something to quietly absorb without a record.

## Commit (branch `milestone-4/activity-log`)

One commit, `916cafb` — see "Discipline confirmed" above for why this
fix-round is one commit rather than three despite spanning three gaps
(shared sqlc/oapi-codegen aggregator output makes a clean per-gap split
impractical without re-running codegen three times against
temporarily-reverted SQL/YAML state). Pushed cleanly on top of the
concurrent commits above.
