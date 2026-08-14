# Task task-3 Report

## Task

Public API surface (`internal/transport/publicapi`, `openapi.yaml`) for
the shared-collection todo schema task-1 built and task-2 wired into
`todo.Service`: todo endpoints updated for the new fields (no more `done`
— sending it is `validation_error`); `POST`/`GET
/api/v1/todos/:id/events` added, one body shape per `type`, mirroring
my-task's own REST shape; `DELETE /api/v1/todos/:id` removed entirely
(genuine 404, not 405); `type: "created"` genuinely rejected (400) at the
HTTP layer, not inferred from I16 holding at the service layer. Scope
ends at `publicapi` — `internal/transport/bff` still references the old
`todo.Service` shape and is task-4's to fix.

## Outcome

done

## Decisions

- **`ownerId` → `createdBy`**: `_contract/API.md`'s Todo wire shape shows
  `createdBy` explicitly (`"createdBy": "…", // milestone-4: replaces
  ownerId, attribution only`), so this is settled by the contract
  directly rather than by checking whether `ownerId` was ever exposed on
  the old wire shape (it wasn't — the old `Todo` schema had no owner
  field at all). Added `createdBy` as a new required response field.
- **`PATCH /api/v1/todos/{id}` is title-only, not extended for the new
  fields — a judgment call, stated as one.** `_contract/API.md`'s intro
  sentence for the todo endpoints ("same shapes as milestone-1/2,
  extended with the new fields above") is ambiguous about whether that
  extension applies to PATCH's *request* body or only to the *response*
  Todo shape every endpoint now returns. Resolved against three
  converging pieces of evidence, not a guess: (1) `todo.Service`
  structurally exposes no generic multi-field update method any more —
  only `Append` (I15's single write path, one event per call) and
  `CreateTodo` exist; there is no method a PATCH handler could call to
  apply several field changes atomically outside of `Append`. (2)
  my-task's own named source has **no task-update REST endpoint at
  all** — the survey (`tpl2-survey.md` Part 1) is explicit that `POST
  /api/v1/tasks/:id/events` is "**write-only** ('the only write on this
  surface')"; every field/status/assignee change routes through events,
  never a separate PATCH. (3) Unlike PATCH, `_contract/API.md` spells out
  POST's optional-field acceptance in its own explicit sentence
  immediately after the ambiguous intro line — if PATCH were meant to
  gain the same write surface, the same explicit treatment would be
  expected there too, and it isn't. Implementation: `UpdateTodoRequest`
  is now `{title (required), clientRequestId (required)}` only,
  `additionalProperties: false`. Internally it still funnels through
  `Append` (as a `field_changed(title)` event), which is why it now needs
  `clientRequestId` at all — it didn't before, since the old
  `Service.UpdateTodo` wasn't part of the append-only write path.
  `status`/`assigneeId`/`priority`/`dueDate` changes go through
  `POST /todos/:id/events` instead. If this reading is wrong, it's a
  one-paragraph contract clarification to fix, not a rearchitecture —
  flagging here rather than silently picking a side.
- **`CreateTodoRequest` has no `status` field — a real gap against
  `_contract/API.md`, not silently worked around.** The contract's POST
  section says the body accepts "optionally `status`/`assigneeId`/
  `priority`/`dueDate`", but `todo.CreateInput` (task-2's real, current
  shape) has `Title`/`AssigneeID`/`Priority`/`DueDate`/`ClientRequestID`
  — no `Status` field — and `Repo.Create` hardcodes every new todo to
  `StatusOpen`. Per this task's own instructions not to touch
  `internal/domain/todo`, I did not add one. `CreateTodoRequest` accepts
  `assigneeId`/`priority`/`dueDate` but not `status`; every created todo
  starts `open` regardless of what a caller sends. Recommend routing to
  Clara/มายด์: either `CreateInput` gains a `Status` field (small,
  task-2-scoped change), or `_contract/API.md`'s POST section is corrected
  to drop "status" from the optional-field list.
- **`type` in `CreateTodoEventRequest` is an open string, not an OpenAPI
  `enum`.** Considered enum-restricting it to the four valid values,
  which would make an invalid `type` (including `"created"`) a
  validator-layer rejection before the handler ever runs — functionally
  equivalent for Done-when 13's purposes. Went with an open string
  instead so the handler's own dispatch switch is what performs the
  real rejection, matching this task's own framing ("your parsing code's
  `type` switch/dispatch should have no case that produces a
  `created`-mapped `AppendInput`... an unrecognized or explicitly-
  `created` type value falls through to your existing 'unknown type'
  validation error path") and mirroring my-task's own `buildAppendInput`
  switch shape (code-level dispatch, not a schema-level `oneOf`
  discriminator).
- **`field_changed`'s `field` selector uses camelCase wire values**
  (`"title"` / `"priority"` / `"dueDate"`), mapped internally to
  `todo.FieldTitle`/`FieldPriority`/`FieldDueDate` (the domain's own
  snake_case-flavored constant for due date is `"due_date"`). Chosen so
  the selector value matches the same JSON key name a caller would see
  on the `Todo` response (`dueDate`, not `due_date`) — the API surface is
  camelCase throughout (`assigneeId`, `clientRequestId`, `createdAt`),
  and `_contract/API.md`'s own `field_changed` example (`"field":
  "priority"`) doesn't disambiguate this case since `priority` is
  spelled the same either way.
- **`status_changed`/`field_changed(priority)` values are validated
  against the known enum at the transport boundary**
  (`validStatuses`/`todoFieldByWireName` maps in `todo_handler.go`), not
  passed through unchecked to `Repo.UpdateStatus`/`UpdatePriority` (which
  have no enum validation of their own — task-2's repo layer stores
  whatever string it's given). An unrecognised `to`/`field` value is
  `validation_error` with the offending key named in `hint`.
- **I18's permission rejection (`todo.ErrForbidden`) reuses the exact
  same 401 body as an unauthenticated request** (`unauthorizedBody`,
  `"authentication required"`), rather than a new, todo-specific
  unauthorized message. Matches `_contract/API.md`'s explicit "same body
  regardless of which check failed (I5)" instruction literally — I5 is
  about credential-resolution failures specifically, but the contract
  cites it here as the general precedent for "the body doesn't leak
  why," and reusing the identical package-level value is the most literal
  reading of "same body," not just "same code."
- **`additionalProperties: false` on `CreateTodoRequest`/
  `UpdateTodoRequest`** makes a stray `done` an openapi.yaml-layer
  `validation_error` (same mechanism the existing missing-title case
  already uses), rather than a handler-level check — this is what
  `_contract/API.md`'s "sending `done`... is a `validation_error`, not a
  silently-dropped key" needed structurally, since OpenAPI 3.0 schemas
  allow additional properties by default.
- **`GetTodo`/`ListTodoEvents`/`CreateTodoEvent` all check `Service.GetTodo`
  (or let `Append`'s own internal `GetByID`) surface `todo.ErrNotFound`
  as 404** before doing anything else — `ListTodoEvents` in particular
  calls `Service.GetTodo` first specifically so an unknown todo id is a
  genuine 404 rather than a silently-empty event list (`ListEvents` alone
  can't distinguish "no events yet" from "no such todo").

## Verification

### Green gate (task-3's own, per `_plan/_todo.md`)

```
$ go build ./...
# github.com/mildronize/my-template/internal/transport/bff
internal/transport/bff/todo_handler.go:46:16: t.Done undefined ...
internal/transport/bff/todo_handler.go:74:57: too many arguments in call to s.Service.ListTodos ...
internal/transport/bff/todo_handler.go:109:69: cannot use req.Title ... in argument to s.Service.CreateTodo ...
internal/transport/bff/todo_handler.go:127:64: too many arguments in call to s.Service.GetTodo ...
internal/transport/bff/todo_handler.go:153:28: s.Service.UpdateTodo undefined ...
internal/transport/bff/todo_handler.go:174:22: s.Service.DeleteTodo undefined ...
```

**Exactly one root-cause package: `internal/transport/bff`**, unchanged
by this task (out of scope, task-4's own — see "What I did not
establish" below). Confirmed the errors are `bff`'s own, not
`publicapi`'s leaking through: `go build ./internal/transport/publicapi/...`
succeeds standalone (`OK`, no output). Confirmed `cmd/server`'s own build
failure is purely transitive through its import of `bff` — building it
directly (`go build ./cmd/server/...`) reports the identical six `bff`
errors above and no error of its own; `cmd/smoke` (which does not import
`bff`) builds clean on its own
(`go build ./cmd/smoke/... ` → no output). This is exactly the shape
`_plan/_todo.md`'s green-gate note predicts for this task: "`bff` may
still be red until task-4 lands (a real, expected gap, not a
regression)" — not literally "the whole repo" yet, since `cmd/server`
embeds `bff`, but every package this task owns or that doesn't depend on
`bff` is clean.

```
$ go test -count=1 ./internal/transport/publicapi/...
ok  	github.com/mildronize/my-template/internal/transport/publicapi	0.264s

$ go test -count=1 -race ./internal/transport/publicapi/...
ok  	github.com/mildronize/my-template/internal/transport/publicapi	4.066s

$ go vet ./internal/transport/publicapi/...
(no output — clean)

$ gofmt -l internal/transport/publicapi/todo_handler.go internal/transport/publicapi/todo_handler_test.go
(no output — clean)
```

Every test in the package (existing keys/middleware coverage plus this
task's own) passes — full `-v` listing confirmed 27 top-level tests, 0
failures, including the pre-existing `keys_handler_test.go`/
`middleware_test.go` suites, unaffected by this task's changes.

### Done-when 13 — verified at the HTTP layer, exact pass output

`TestDoneWhen13_CreateTodoEvent_TypeCreatedRejected`, two subtests:

```
--- PASS: TestDoneWhen13_CreateTodoEvent_TypeCreatedRejected (0.02s)
    --- PASS: TestDoneWhen13_CreateTodoEvent_TypeCreatedRejected/type:_created (0.01s)
    --- PASS: TestDoneWhen13_CreateTodoEvent_TypeCreatedRejected/type:_an_ordinary_unrecognised_string (0.01s)
```

Each subtest: creates a real todo over HTTP, records the real
`todo_events` row count for it (1, from the todo's own `created` event),
`POST`s `{"type": "created", "clientRequestId": "attack-1", "body":
"forged event"}` (and, in the second subtest, `{"type": "sabotage", ...}`)
to `/api/v1/todos/:id/events`, asserts `400` + `error.code ==
"validation_error"`, then re-counts `todo_events` for that todo directly
against the SQLite connection and asserts the count is **unchanged** —
proving the rejection is genuine (nothing written), not a 400 returned
after an insert already happened. Also re-`GET`s the todo afterward and
confirms `title`/`status` are untouched. Both `"created"` and the
arbitrary unrecognised string `"sabotage"` are asserted to behave
identically (same status, same code, same "nothing written" property),
confirming the dispatch switch in `CreateTodoEvent`
(`todo_handler.go`) has no special case for `"created"` — it simply has
no case that maps to it, the same as any other string the switch doesn't
recognise.

### Round-trip sanity check

`TestHandler_EventsRoundTrip_CreateReadAppendFieldChangedReadTimeline` —
`PASS (0.01s)`. Creates a todo with `assigneeId`/`priority: "low"`/
`dueDate` set; reads it back and confirms all three; appends a
`field_changed` event moving `priority` `low` → `urgent`; asserts the
returned event's `payload` is `{"field": "priority", "from": "low", "to":
"urgent"}`; re-reads the todo and confirms `priority` is now `urgent`
(the side effect actually applied, not just the event recorded); reads
`GET /todos/:id/events` and confirms both events (`created`, then
`field_changed`) are present, oldest-first, with the same event id and
payload as the `POST`'s own response.

### DELETE genuinely 404s

`TestDoneWhen6_DeleteTodo_RouteGenuinelyDoesNotExist` — `PASS (0.01s)`,
against the real built router (`newIntegrationRouter`/
`api.RegisterHandlers`, the same composition `cmd/server` itself uses,
not a hand-rolled subset). Manually inspected the raw response outside
the assertion (a throwaway check, not committed) to confirm the shape:
`STATUS=404 BODY="404 page not found"` — gin's own default not-found
page, not this package's JSON error envelope, confirming this is a
genuine route-absence (no `DeleteTodo` method exists on `TodoServer` at
all, nothing registered for that method+path pair) rather than an
application-level `not_found` response that happens to also be a 404.
`HandleMethodNotAllowed` is not set anywhere in this codebase (checked:
`grep -rn "HandleMethodNotAllowed"` finds nothing outside vendor code),
so gin's default (`false`) applies — a request to an existing path with
an unregistered method is a plain 404, not a 405, matching the
contract's explicit requirement ("not a `405` — there's nothing at that
path to name a wrong method for"). Confirmed the todo itself is
untouched afterward (still `GET`-able, `200`).

## What I did not establish

- Did not touch `internal/transport/bff` or `bff-openapi.yaml` at all —
  out of scope (task-4's own). It remains in the exact broken state
  task-2's report already described (same six errors, same file,
  unchanged by anything in this task) — confirmed by direct `go build`
  above, not assumed.
- Did not resolve the `CreateTodoRequest`/`status` gap against
  `todo.CreateInput` (see Decisions above) — flagged for Clara/มายด์
  rather than silently deciding it myself in either direction, since it
  touches `internal/domain/todo`, which this task was told not to edit.
- Did not add FK-style validation for `assigneeId` (confirming the id
  actually names an existing user) on either `CreateTodo` or the
  `assigned` event type — matches `todo.Repo`'s own current behavior
  (no such check exists at the repo layer either; a bad id would surface
  as a raw SQL/FK error today, a pre-existing gap this task didn't
  introduce and wasn't asked to close).
- Did not investigate the `TestDoneWhen12_EveryInvariantHasANamedTest`
  failures observed while re-running the broader `internal` package
  suite (`I3`-in-`internal/identity`, `I20`, `I21` all currently
  unsatisfied) — these come from `internal/invariants_test.go`/
  `_rules/_contract/INVARIANTS.md`'s own scope-tag semantics, which I
  did not touch; a concurrent commit already on this branch
  (`0b55d4f fix(milestone-4/scope-tags): ...`, landed on
  `origin/milestone-4/activity-log` during this task's own work,
  confirmed via `git log`/`git fetch` to be a clean ancestor of this
  task's commit, not something this task authored or needed to rebase
  around) changed that scope-tag scheme; its own residual failures are
  outside this task's green gate (`internal/transport/publicapi`, not
  `internal`) and outside its declared scope (`openapi.yaml`,
  `internal/transport/publicapi`).

## Commits pushed (branch `milestone-4/activity-log`)

- `8aca5f3` — `feat(milestone-4/task-3): publicapi surface for the shared-collection todo schema`

Pushed on top of `0b55d4f` (a concurrent, unrelated scope-tags fix-round
commit already on `origin/milestone-4/activity-log` by the time this
task's own commit was made — confirmed via `git fetch`/`git status -sb`
to be a clean fast-forward, `ahead 1`, no divergence, no rebase needed).
