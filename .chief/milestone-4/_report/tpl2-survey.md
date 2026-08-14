# TPL-2 survey — my-task's activity log, read from source, and my-template's current state

No code, no plan. Every claim below is either a direct quote/citation (file:line) or explicitly marked as a question/inference. Read `~/gits/my-task`'s actual source and `~/gits/my-template`'s actual current state for this, not any prior description of either — including my own from earlier sessions.

## Part 1 — what my-task's activity log actually is

### Schema

`src/server/db/schema.ts:219-251` — `taskEvents` (`task_events` table):

```
id               text pk
task_id          text, fk -> tasks.id, not null
seq              integer, not null        -- monotonic per task, starts at 1
actor_id         text, fk -> user.id, not null   -- "From the credential only (I1) — never from request input"
type             text, not null   -- 'created'|'commented'|'status_changed'|'assigned'|'field_changed'|'moved'|'labeled'|'unlabeled'
payload          text, nullable   -- JSON, shape depends on type
body             text, nullable   -- comment text, plain text (I8)
client_request_id text, not null, unique   -- the Idempotency-Key (I5)
actor_detail     text, nullable   -- unverified sub-label from X-Actor-Detail
created_at       integer (timestamp), not null
```
Indexes: `(task_id, seq)` unique, `created_at`, `actor_id`.

Comment at line 218: `// Append-only (I3). No UPDATE, no DELETE, no soft-delete column.`

### The enforcement question, answered precisely

`.chief/milestone-1/_contract/INVARIANTS.md:31-47`:

> **I3 — `task_events` is append-only, for everyone.** No `UPDATE`, no `DELETE`, no exceptions for the owner... *Enforced by:* no service method, tRPC procedure, or REST route exists that updates or deletes an event. **Verified by test** (an assertion that the repository exposes no such method is not enough on its own — the test asserts that state changes always add a row).
>
> **I4 — One write path.** Only `TaskService.append()` writes to `tasks` or `task_events`... *Enforced by:* code review plus a repository boundary — the Drizzle table objects for `tasks`/`task_events` are exported only to the service module. **Verified by test** for the transaction property: a failure mid-append leaves neither the event nor the state change.

**Both are application-level, not database-level.** I checked directly: `grep -rn "TRIGGER" drizzle/*.sql` returns nothing — there is no DB trigger or constraint blocking an `UPDATE`/`DELETE` against `task_events`. Nothing at the SQLite layer stops a raw query from mutating a row; the guarantee is "no code path does it," backed by a test that only checks the code paths that exist today, plus the module-boundary below.

I verified the I4 boundary claim directly rather than trusting the comment: `grep -rln "taskEvents\|{ *tasks *}" src/ --include="*.ts" --include="*.tsx"` returns exactly three files — `schema.ts` (the definition), `task.service.ts`, `task.queries.ts` — both inside `src/server/modules/task/`. No router, no script, imports the table objects directly. **This is real** (confirmed, not assumed), but it's a *convention* (which files choose to import a symbol), not something the TypeScript compiler enforces the way a package-private field would — nothing stops a future file from adding the import. The "Verified by test" claim, per I4's own text, only covers the transactional-atomicity property, not the import-boundary itself.

### The write path

`src/server/modules/task/task.service.ts:161-199`, `TaskService.append(input)` — the single entry point:
1. Idempotency lookup first, inside the same transaction (line 165-172): a repeat `clientRequestId` returns the existing row and inserts nothing.
2. Dispatches by `input.type` to one of 7 private `append*` methods (`appendCreated`, `appendCommented`, `appendStatusChanged`, `appendAssigned`, `appendFieldChanged`, `appendMoved`, `appendLabeled`, `appendUnlabeled`), each of which does the domain-specific side effect (e.g. updating `tasks.statusId`) and calls the shared `insertEvent` helper.
3. `insertEvent` (line 493-529): computes `seq` as `max(seq) + 1` for that `task_id`, within the same transaction, then a single `insert`.

Only `.insert(taskEvents)` calls exist anywhere in the service (`grep -n "taskEvents" task.service.ts` — lines 255, 513, both inserts). No `.update()`/`.delete()` against the table anywhere.

**Permission gating**: each REST/tRPC entry point calls `can(actor, {type: ...})` (`src/app/api/v1/tasks/[id]/events/route.ts:50,68,84,117,145,162` — one `can()` check per event type) *before* calling `append()`. `append()` itself does not check permission — the caller does. Not all 8 types are writable from the public REST endpoint: `buildAppendInput`'s switch (`route.ts:45-181`) has no `created` or `moved` case — those two only happen as side effects of task-creation and project-move operations respectively, never as a direct client-specified event type.

### API surface — two different shapes, on purpose

- **`POST /api/v1/tasks/:id/events`** (`src/app/api/v1/tasks/[id]/events/route.ts:184-224`) — REST, agent-facing, one body shape per `type`, requires `Idempotency-Key`. This is **write-only** ("the only write on this surface" — line 2's own comment); reads happen elsewhere.
- **`activity.list`** (`src/server/api/routers/activity.ts`) — tRPC, `ownerProcedure` (owner-session only), **cursor over `task_events` across every task**, newest first, joined to `user` (for role) and `tasks`/`projects` (for the task link). Documented in `API.md:152` as a tRPC-only procedure — **there is no REST equivalent of the cross-task feed**. I checked: `grep -rn "listActivity"` finds exactly one caller outside the query service itself, `activity.ts`.
- Per-task reads (`task_events` for one task) come through `TaskQueryService.getTaskDetail()` (`task.queries.ts:577`), shared by both `task.byRef` (tRPC) and the REST task-detail route — same underlying rows, no separate "per-task events" endpoint.

### The page(s)

**`/` (the home page) is the activity log** — `src/app/(app)/page.tsx:1-11`:
> `/` — the activity feed... the question mild actually opens the app to answer is "what have the agents been doing", not "what is on my list"... Cursor-paginated, polling for freshness while viewing the live page (~10-15s per design-spec.md), no websockets.

Cursor state is a plain `{createdAtMs, id}` object in React state (not the URL); polls every 12s (`POLL_MS = 12_000`) only while on the latest page (`onLatestPage ? POLL_MS : false`); "Load older" / "Back to latest" buttons; explicit empty state and error state.

`src/components/TimelineEvent.tsx` — `TimelineEventRow`, shared verbatim between the home feed and each task's own `/tasks/[ref]` timeline ("used by both... so the two pages agree on how an event reads" — line 2). Per-type rendering: `StatusChangedSummary`, `AssignedSummary`, `FieldChangedSummary` (shows both `from` and `to`, previewed to 140 chars, with distinct wording for "set"/"cleared"/"changed"), `CreatedSummary`, `MovedSummary`. A 🧑/🤖 `ProvenanceMark` on every row (`role === "owner"` → human, anything else → agent) — stated purpose (line 5-8): *"a later agent reading a timeline needs to tell who wrote what before it can trust any of it"*. Comment bodies render through a Markdown-to-React-elements path (`~/components/Markdown`), explicitly never `dangerouslySetInnerHTML` (I8).

## Part 2 — what my-template has today, against each of มายด์'s three bullets

### Bullet 1: "agent has api key, and can revoke on settings page"

**Partially built, and the part that's missing is structural, not cosmetic.**

- Agents already have API keys: `cmd/issue-key` (role always `'agent'` — `cmd/issue-key/main.go:15`), `api_keys` table (`db/migrations/20260812180048_create_users_and_api_keys.sql`), list/revoke already exist on the public API (`GET/DELETE /api/v1/keys`).
- A settings page with list+revoke already exists (`web/src/app/settings/ApiKeySettings.tsx`, milestone-3), calling `GET/DELETE /api/bff/keys`.
- **But that endpoint is scoped to the session owner's own `user_id`** — verified directly: `internal/transport/bff/keys_handler.go`'s `ListKeys` calls `s.Service.ListAPIKeys(ctx, ownerID)` → `internal/identity/service.go`'s `ListAPIKeys` → `s.APIKeys.ListAPIKeysByOwner(ctx, ownerID)`. `ownerID` here is the *session's* user id, always `role='owner'`. `cmd/issue-key` never issues a key to `role='owner'` (confirmed above), and I2 (`Bearer credential never resolves to role='owner'`) means one wouldn't be usable even if it existed. **The settings page, as built, queries a set of keys that is always empty in production**, because no agent's key ever has `user_id = ` the owner's id.

This is the *exact* failure my-task's own code explicitly documents fixing, in the file I already read this session (`src/app/(app)/settings/api-key-settings.tsx`'s own comment): *"Reads through the `apiKey` tRPC router rather than Better Auth's client API. The latter filters on the logged-in user, so it showed the owner an empty list while nineteen agents held working keys."* My-template's current settings page has that exact bug — it wasn't caught because no fork has run it with a real agent key issued and then opened settings to look.

**So bullet 1 is not satisfied.** Building it for real requires an owner-facing "list every agent's keys" capability that doesn't exist anywhere today — not a bigger `ownerID` scope, a genuinely different query (all `api_keys` joined to `users` where `role='agent'`, or similar), which is also a scope decision (see Part 4).

### Bullet 2: "todo has more fields for support Activity Log"

**Not built. `todos` has 4 columns beyond id/owner_id/timestamps: `title`, `done`.** (`db/migrations/20260812190000_create_todos.sql`) Milestone-1's own decision record (`.chief/project.md:9`): *"reimplemented in Go and simplified — no projects, no activity log, 2-4 basic todo fields."* — this bullet is มายด์ reopening that exclusion directly, in his own words, not something to re-derive.

There is no field on `todos` today that an activity log would have much to describe beyond "created" / "title changed" / "done toggled". My-task's richer event vocabulary (`status_changed`, `assigned`, `moved`, `labeled`/`unlabeled`) exists because `tasks` has `statusId`, `assigneeId`, `projectId`, and a labels join table — fields `todos` doesn't have and milestone-1 explicitly cut. Whether "more fields" means bringing some of those back, or just enough new fields to make a log non-trivial, is not decidable from the ticket text — see Part 4.

### Bullet 3: "activity log page"

**Does not exist in any form.** No `todo_events`-shaped table, no write path, no read endpoint, no page. `internal/transport/bff` today has: session-check, todo CRUD, key list/revoke (milestone-3). Nothing logs anything beyond `updated_at` on the row itself.

## Part 3 — gaps มายด์'s three bullets do not name

His list describes the my-task-visible surface (a settings page, some fields, a log page). It does not name the structural questions those three things sit on top of in my-task, several of which don't have an obvious answer for my-template as it's currently built:

1. **Ownership model mismatch.** In my-task, `tasks` belong to a `project`, and any agent (plus the owner) can act on any task in a project they have access to — the activity feed is meaningful because many actors' actions land on the same shared objects. In my-template today, `todos.owner_id` is *"always the resolved actor"* (`.chief/_rules/_contract/API.md:80` — this session's own earlier reading, re-confirmed against the current file) — whoever creates a todo via the public API owns it, under their own `user_id`. **If two different agents each hold a key and each create todos, those todos never overlap** — there's no shared collection for a cross-actor activity log to be about. My-task's home-page activity feed is valuable specifically because it's cross-agent, on shared objects. Whether my-template's todo model needs to become shared (multiple agents' + the owner's writes visible together) or stays private-per-actor changes what "activity log page" even means here. This is not addressed by any of the three bullets, and it's the single biggest scope fork in this ticket.

2. **No permission/policy layer.** My-task gates every write with `can(actor, {type: ...})` (`src/lib/policy`, referenced at 5 call sites in `route.ts` alone) — who may comment, who may change status, etc. My-template's todo writes today have no equivalent; any authenticated actor (agent via Bearer, owner via session) can write anything about their own rows, full stop. If todos become shared across actors (per gap 1), a policy layer becomes necessary, not optional — otherwise any agent can silently edit or "log" state on behalf of another.

3. **No Idempotency-Key on my-template's public API, by explicit prior decision.** `.chief/_rules/_contract/API.md`'s Conventions: *"No `Idempotency-Key` requirement. Deliberate simplification (this service has no append-only event log — see milestone-1's original `_goal/GOAL.md`)... re-add it if a fork adds one."* **This ticket is that fork.** My-task's I5 (idempotent writes) is enforced by the unique constraint on `client_request_id` plus a lookup at the top of every `append()` call — without it, a network retry on an event-append endpoint double-logs. This needs deciding, not just noticing: it's called out in the very doc that would need to change.

4. **No "one write path" architectural test for todos today**, unlike my-task's I4 (verified above) or my-template's own I4 for identity (`internal/invariants_test.go`'s per-module checks). If an events table is added, whether it gets the same "only the service module may import this table" treatment — and whether that's checked by a test the way `internal/architecture_test.go` already checks todo/identity/transport boundaries — isn't decided anywhere yet.

5. **No per-todo detail view in the SPA.** Milestone-3 built a list page and a create dialog; there's no `/todos/:id` route or equivalent. My-task's per-task timeline lives on the task detail page (`/tasks/[ref]`). An activity log needs somewhere to attach to, if it's meant to be per-todo rather than (or in addition to) a home-page feed.

6. **`.chief/project.md`'s Directory Structure section is stale** (describes milestone-1's `internal/todo/`-flat layout, not the current `domain/identity/transport` split) — noticed while reading it for the activity-log citation, unrelated to this ticket but worth a one-line fix whenever that file is next touched.

## Part 4 — questions only มายด์ can answer, full option sets, not narrowed

1. **Scope of the activity log: per-todo, cross-todo home feed, or both?** My-task has both (shared component, two pages). Options: (a) a home-page-style cross-todo feed only, mirroring my-task's actual "what have the agents been doing" framing; (b) a per-todo timeline only (simpler, no new page, extends the existing todo detail — which doesn't exist yet either); (c) both, matching my-task exactly; (d) something narrower than either (e.g. just a `history` field on the todo response, no dedicated page at all).

2. **Does the todo ownership model change?** Options: (a) keep todos strictly private per-actor (today's model) — an activity log then only ever shows one actor's own history, which is a much smaller feature than my-task's; (b) make todos a shared collection any authenticated actor (agents + the owner) can act on, mirroring my-task's shared-task model — this is the option that makes a cross-actor activity log meaningful the way my-task's is, but it's a real architecture change (touches I1-I3's current per-owner scoping) and needs its own review, not something to infer from "like my-task"; (c) something in between (e.g. a fixed small set of named agents, not open like keys are today).

3. **What does "todo has more fields" mean concretely?** Options: (a) reintroduce specific my-task fields — which ones (priority? due date? assignee? something else)? (b) something new, not from my-task, that มายด์ has in mind and hasn't named yet; (c) no new *domain* fields — just whatever the events table itself needs (actor, type, payload, etc.), with the log describing title/done changes only.

4. **Bullet 1's real ask: does the settings page need to show every agent's keys, or just confirm one already works?** Given the settings page today always shows an empty list (Part 2), the fix is either (a) a genuinely new "all agents' keys" capability, owner-visible, mirroring my-task's fix exactly (the more likely reading of "like my-task"); or (b) มายด์ meant something narrower he hasn't stated — worth confirming before building the bigger version.

5. **Should idempotency (`Idempotency-Key`/`clientRequestId`) be added now**, closing the gap `API.md` already names as a "re-add it if a fork adds [an event log]" case? Or is a simpler current-template stance (no idempotency, accept the double-write risk on retry) acceptable for this size of fork? My-task's I5 exists because event-append is exactly the kind of write where a retry is dangerous; a template deciding not to have it needs to say so on purpose, not by omission.

6. **Should writes be permission-gated per event type** (my-task's `can()`), or is "any authenticated actor may act on rows they're scoped to" sufficient here? This only becomes a real question if question 2 answers toward a shared model — flagged together for that reason.

7. **Enforcement approach for append-only**, if built: my-task's is convention (module-boundary + code review + test-that-checks-current-code-paths), explicitly *not* database-enforced. Does my-template want to match that exactly, or go further (a DB trigger/constraint, which my-task itself doesn't have)? Worth deciding rather than defaulting to "whatever my-task does," since my-task's own choice here is a real trade-off (simpler, but only as strong as the code review that maintains it), not an obviously-correct baseline.

---

No plan, no code, no chief scaffolding past this point, per your instruction. Everything above is either a direct citation or explicitly marked as a question.
