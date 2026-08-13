# Milestone 4: activity log, like my-task actually does it

TPL-2 (https://my-task.thadaw.com/tasks/TPL-2). มายด์'s words, verbatim:

> Support Activity Log like my-task
>
> make sure
> - agent has api key, and can revoke on settings page
> - todo has more fields for support Activity Log
> - activity log page

Clara directed this one, not built it. The process this milestone followed
deliberately repeats TPL-1's late-learned discipline from the start rather
than re-learning it: a source-read survey before any plan (`_report/` —
see below), every question that would *narrow* "like my-task" by removing
something the source actually has routed to มายด์ specifically rather than
settled between Luna and Clara, and the grill itself citing file/line
against the real my-task repo, not a description of it.

## Objective

Give my-task's actual activity-log behavior to my-template's todo domain:
todos become a shared collection every agent and the owner act on
together, carry the fields needed for that activity to mean something
(status, assignee, priority, due date), and every change to any of it is
visible two ways — a cross-todo feed and a per-todo timeline sharing one
row-rendering component, exactly as my-task's `/` and `/tasks/[ref]` share
`TimelineEventRow` — with every row legibly marked human or agent.

## Context — the survey, and how the grill's three narrowing questions got resolved

Full survey (schema, enforcement mechanism, write path, API surface, page
structure — all cited file:line against my-task's actual source, plus
my-template's current state verified the same way) is in
`.chief/milestone-4/_report/tpl2-survey.md`. Three findings from it that
shape this goal directly:

1. **Bullet 1 was already partially built and silently broken.** The
   settings page (`web/src/app/settings/ApiKeySettings.tsx`) already
   lists and revokes keys, but `internal/transport/bff/keys_handler.go`'s
   `ListKeys` scopes to the session owner's own `user_id` — and
   `cmd/issue-key` only ever issues to `role='agent'` users. No agent key
   can ever match; the page has shown an empty list in every real
   deployment so far. This is the exact bug my-task's own
   `api-key-settings.tsx` documents having fixed for itself ("filters on
   the logged-in user... showed the owner an empty list while nineteen
   agents held working keys").
2. **The gap none of the three bullets named**: my-template's todos are
   private-per-actor (`owner_id` is always the resolved creator); my-task's
   tasks are a shared collection. A cross-actor activity log is only
   meaningful once the thing it's logging is shared — this is the actual
   size of the ticket, not a detail inside it.
3. **A test already in the codebase would have hidden a version of bug
   1 forever.** `internal/transport/bff/keys_handler_test.go`'s
   `newBFFRouterForTwoOwnersWithKeys` seeds keys directly onto
   `role='owner'` fixtures via `repo.CreateAPIKey`, bypassing
   `cmd/issue-key` — the only path that ever produces a real agent key.
   The isolation assertion built on it is real and green, and tests a
   state production cannot reach. Every new test this milestone writes is
   held to one question: **would this still be green if the thing it
   guards were absent entirely?** Fixtures that need an agent go through
   `cmd/issue-key`'s actual path, not a direct insert with a role that
   happens to be convenient.

**On "like my-task" as an instruction, after two milestones of getting it
wrong in both directions**: milestone-2 reopened because "like my-task"
was read too narrowly (a stack list read as scope removal). Milestone-3's
own grill caught a second, smaller instance of the same shape before it
shipped (Vite-vs-Next read as a frontend-code removal). This milestone's
grill produced three genuine candidates for a *third* instance — each
question below is one the grill flagged as "matching my-task exactly
would mean removing/restricting something my-template currently has, or
declining to add something my-task actually has" — and every one of them
went to มายด์ rather than being settled between Luna and Clara, on the
explicit reasoning that removals must come from มายด์'s own words, never
derived.

## Decisions

| Decision | Answer | Owner |
| --- | --- | --- |
| Ownership model | Todos become a shared collection: any authenticated actor (agent via Bearer, owner via session) can see and act on any todo. `todos.owner_id`'s current meaning ("always the resolved creator, and the sole access-scoping key") is retired for this domain — see the I3-scope note below. A `created_by` column replaces it for audit/attribution only, never for access control | มายด์ (ruling 1, TPL-2 grill) |
| Fields added | `status` (fixed enum: `open` / `in_progress` / `done` / `closed` — replaces the `done` boolean, not alongside it), `assignee_id` (nullable, references `users`, any role), `priority` (nullable, my-task's convention: `low`/`medium`/`high`/`urgent`), `due_date` (nullable timestamp) | มายด์ (ruling 2) |
| No manageable-statuses table | Fixed enum, not an owner-editable `statuses` table like my-task's (with I11's delete-with-destination logic). Not a natural reading of "todo has more fields" — a manageable-statuses feature is materially bigger than field tracking, and nothing in มายด์'s ask implies wanting to rename/reorder statuses | Luna's recommendation, มายด์ took it (ruling 2) |
| Comments — in scope | `commented` event type, `body` field (plain text write, Markdown-rendered read, per my-task's I8 pattern below). Luna recommended out (reasoning from มายด์'s literal three bullets, which don't mention comments); Clara overrode on the ground that my-task's log is substantially `type: commented` — "it is how an agent says anything at all, and a log that only records field changes never carries what anyone wanted to say." A judgment call, not a fact — recorded as one | Clara (ruling 3, มายด์ took her lean) |
| Comment rendering safety | Body text only, rendered client-side through a Markdown-to-React-elements path — never `dangerouslySetInnerHTML`, never a raw-HTML string reaching the DOM. Mirrors my-task's I8 exactly: *"the log is a cross-agent injection channel"* is the stated reason, not decoration | Mirrors my-task's I8 (`~/gits/my-task/.chief/milestone-1/_contract/INVARIANTS.md` — "No raw HTML, ever") |
| DELETE removed | `DELETE /api/v1/todos/:id` and `DELETE /api/bff/todos/:id` are both retired. Finishing a todo means moving it to `status: closed`, mirroring my-task's I12 exactly. Luna flagged this as a real tension rather than deciding it — my-task's own I12 text (quoted precisely, scope clause included: *"There is no hard delete **in this milestone**, for any role"*) is a milestone-1 decision, not an unconditional law, though its stated reason (*"deletion destroys the timeline, which is the reason the system exists"*) carries no such caveat. มายด์ ruled on both halves together | มายด์ (ruling 1, TPL-2 grill) |
| Permission model | Mirrors my-task's `can()` (`~/gits/my-task/src/lib/policy.ts`) exactly, **including** the agent-restricted move-to-closed: any agent may comment/assign/change fields/change status on any shared todo; **only the owner may move a todo to `closed`**. Luna recommended dropping that restriction (nothing in มายด์'s ask implied needing owner sign-off on any status value); Clara did not take it — her reasoning: adding fidelity to the named source is safe, dropping it is a removal, and `closed` now exists in the enum specifically so the restriction has somewhere to bind | Clara (ruling 4) — role-based (not per-todo-identity-based), same shape as my-task's `can()`: owner passes unconditionally, agents get a permission table |
| Owner-facing key visibility | A genuinely new query: the settings page lists **every** agent's non-revoked keys (not the session owner's own — which structurally can never be non-empty), and the owner may revoke any of them. Still **no key issuance from the UI** — that option was on the table and มายด์ did not take it; TPL-1's CLI-only issuance stands unchanged | มายด์ (ruling 4 of the survey's questions) |
| `GET`/`DELETE /api/bff/keys` — replace, not add beside | The existing endpoint's session-owner-scoped semantics are **replaced**, not kept alongside a new one. `GET /api/bff/keys` becomes "every agent-role user's non-revoked keys"; `DELETE /api/bff/keys/:id` becomes "any agent's key, still session-gated to the owner" (no longer requiring `user_id = ownerID`). Reasoning: keeping the old owner-scoped endpoint around unchanged, next to the new one, means shipping a surface that is *known*, on purpose, to always return empty — exactly the kind of technically-correct-but-useless thing this milestone exists to remove, not add a second instance of. **Consequence: `internal/transport/bff/keys_handler_test.go`'s existing assertions are now wrong under the new semantics and must be rewritten as part of this milestone** — not left alone. Its `newBFFRouterForTwoOwnersWithKeys` fixture also tested a scenario (two distinct owners) that was already fictional under this template's actual single-seeded-owner model, independent of this change | Luna, per Clara's explicit request to decide it in the goal rather than leave it to the plan |
| Idempotency added | `clientRequestId` (unique constraint) on `todo_events`, lookup at the top of the write path, inside the same transaction — mirrors my-task's I5 exactly. `.chief/_rules/_contract/API.md`'s existing Conventions text names this precise case: *"No Idempotency-Key requirement... re-add it if a fork adds one [an event log]."* This is that fork | Clara (decided directly, not มายด์'s to rule on) |
| Append-only enforcement | Application-level only, matching my-task's I3/I4 exactly — module-import boundary (only the todo domain/service module may import the new events table's generated types) plus a test that asserts state changes always add a row (not just "no update method exists"). **No DB trigger or CHECK constraint.** Going further than the named source without being asked is explicitly out — if evidence surfaces during the build that convention-only enforcement is insufficient here, that's a question back to Clara/มายด์, not something to unilaterally strengthen | Clara (decided directly) |
| Test-fixture discipline | Every new test that needs an agent identity goes through `cmd/issue-key`'s real path, not a direct repo insert with a convenient role. Every new test is checked against "would this still be green if the thing it guards were absent entirely?" before being trusted. **This milestone's own key-endpoint replacement (row above) makes `keys_handler_test.go`'s existing gap load-bearing, so it gets fixed here** — not left standing on the original reasoning ("not this milestone's to fix unless it becomes load-bearing"), which no longer holds once the endpoint it tests changes shape | Clara, updated per her own condition once E resolved toward "replace" |
| Branch | New branch off `main` (not a continuation of `milestone-2/close-parity-gap` — that branch's PR is merged; PR #3 is open, separate, and unrelated to this ticket) | Luna (ruling 5, uncontested) |

### I3's scope, corrected for the todo domain

**Lives in `.chief/_rules/_contract/INVARIANTS.md` itself, next to I3 —
not held here.** The contract file is what the next reader of I3 actually
opens; a scope correction that only exists in this goal has not really
been written. Referenced, not restated: I3's wording is unchanged, its
*reach* now excludes the todo domain (a shared collection has no "belongs
to a different owner" case left to protect), and continues to hold for
the identity/api-keys domain exactly as before.

## Scope

### In scope

- Schema: `todo_events` table (id, todo_id, seq, actor_id, type, payload,
  body, client_request_id, created_at — mirrors `task_events`'
  columns/indexes); `todos` gains `status` (replaces `done`),
  `assignee_id`, `priority`, `due_date`, `created_by` (replaces the
  access-scoping meaning of `owner_id` — exact column-level migration
  shape is the plan's to specify, not this goal's)
- Permission layer mirroring my-task's `can()` shape — role-based, a
  small allow-table, owner unconditional, agents restricted only from
  `status: closed`
- Single write path (mirrors `TaskService.append()`): idempotency check →
  dispatch by event type → domain-specific side effect → insert, one
  transaction
- BFF and public API surfaces for: creating/reading todo events —
  `comment`, `status_change`, `assign`, `field_change` are directly
  client-specifiable. **`created` is never client-specifiable — it only
  ever happens as a side effect of `POST /todos` itself, the same way
  `buildAppendInput`'s switch has no `created` case
  (`~/gits/my-task/src/app/api/v1/tasks/[id]/events/route.ts:45-181`,
  cited in the survey).** This is a security property, not a style
  choice: a client that could POST `type: "created"` could forge a
  creation event under any actor and timestamp in the log that exists
  specifically to establish who did what. Not something the plan is free
  to decide either way
- Cross-todo activity feed (owner-session, tRPC-shaped-equivalent on the
  BFF's JSON surface) — the new home-page-equivalent view
- Per-todo timeline, sharing one row-rendering component with the feed
- Per-todo detail page in the SPA (doesn't exist yet — created by this
  milestone as a consequence of the timeline requirement)
- New owner-facing "every agent's keys" query + settings-page wiring
- `DELETE /api/v1/todos/:id` and `DELETE /api/bff/todos/:id` removed;
  **the companion `<service>-api` skill doc (TPL-1 item 9) updated to
  stop naming DELETE** — its own task, not a cleanup line item, since an
  agent-facing doc that still says "call DELETE" makes the removal
  undone for the only readers who act on it
- `clientRequestId` idempotency on the new write path
- Comment body rendering: Markdown → React elements, never raw HTML

### Out of scope

Key issuance from the UI (still CLI-only, `cmd/issue-key`) · a
manageable/owner-editable statuses table (fixed enum instead) · projects
or labels (still not part of this domain — todos stay a flat collection,
just a shared one) · CI workflow yaml (already ruled out, TPL-1) ·
identity-domain changes beyond the new owner-facing key-listing query (I2,
I9, I12-I14 etc. unchanged)

## Done when

Machine-checkable stopping conditions, one task owner each (assigned in
`_plan/_todo.md`). **These are not the milestone's real finish line** —
see Human Acceptance below, which is explicit about that.

1. `todo_events` exists with the columns/indexes above; a migration moves
   `todos` from `done` (boolean) to `status` (enum) and adds
   `assignee_id`/`priority`/`due_date`/`created_by`, with `owner_id`'s
   old access-scoping role retired.
2. The single write path (mirrors `TaskService.append()`) exists: a test
   proves a failure mid-write leaves neither the event row nor the
   `todos` state change (the same transactional-atomicity property
   my-task's I4 test checks).
3. `todo_events` is append-only: a test proves state changes always add a
   row (not merely "no update method exists on the repo" — the same
   distinction my-task's own I3 test draws).
4. Idempotency: a test proves a repeated `clientRequestId` returns the
   original event and creates nothing.
5. Permission layer: a test proves an agent attempting `status: closed`
   is rejected, and the owner attempting the same succeeds. A second test
   proves every other action type (comment, assign, field-change,
   non-closed status-change) succeeds for both roles.
6. `DELETE /api/v1/todos/:id` and `DELETE /api/bff/todos/:id` genuinely
   404/405 (not silently 200) — a negative check, same shape as
   milestone-3's key-issuance negative check. The companion skill doc no
   longer names DELETE anywhere (a presence/absence check against the
   doc's actual text, not assumed from the code change alone).
7. Owner-facing key query: a test proves it returns keys belonging to
   **agent**-role users, seeded through `cmd/issue-key`'s real path (not
   a direct repo insert on an owner-role fixture — the exact trap
   `keys_handler_test.go`'s existing test fell into). A second test
   proves an agent's own Bearer-authenticated key listing stays
   self-scoped (I3, unchanged for this domain). **`keys_handler_test.go`'s
   existing `newBFFRouterForTwoOwnersWithKeys`-based assertions are
   rewritten, not left in place** — they test semantics this milestone's
   own decision (`GET`/`DELETE /api/bff/keys` — replace, not add beside)
   removes.
8. Cross-todo feed and per-todo timeline both render through the same
   row component — a test (Vitest, mirrors milestone-3's Done-when-7
   negative-control shape) proves a given event renders identically
   (same summary text, same provenance mark) regardless of which page's
   query fed it.
9. Comment bodies: a test proves a body containing raw HTML tags renders
   as literal text/escaped elements, never as unescaped markup — the
   concrete version of "never `dangerouslySetInnerHTML`."
10. Provenance: a test proves an owner-authored event and an agent-authored
    event render distinguishably (not just that a role field exists on
    the response — that the UI actually branches on it, mirroring
    milestone-2's Done-when-9 lesson about testing the property that
    matters, not a proxy for it).
11. **Revocation actually stops the key, proven both ways, not just the
    post-revoke half.** A test: issue a key through `cmd/issue-key`,
    authenticate with it successfully against the public API (the
    positive half — proves the key worked before revocation, so the
    negative half means something), revoke it through the new
    owner-facing endpoint, then prove the *same* key now fails
    authentication. A test that only checks the post-revoke 401 is
    consistent with the key never having worked at all — both halves are
    required for this item to count as satisfied. (Clara's finding: the
    milestone could otherwise go fully green with a revoke button that
    writes `revoked_at` and a middleware that never reads it — ENG-13's
    exact shape.)
12. **The feed is proven cross-actor, not merely dual-page.** Done-when 8
    proves the feed and the per-todo timeline render a *given* event the
    same way — it does not prove either page ever shows another actor's
    event. A test: an agent (via `cmd/issue-key`-issued Bearer credential)
    creates a todo and acts on it (at least one non-`created` event);
    separately, the owner's session-authenticated feed query returns that
    event, attributed to the agent. This is ruling 1's only real proof —
    a feed that happened to only ever show the viewer's own events would
    pass every other item in this list.

## Human acceptance — the real finish line, not a supplement to Done-when

Clara's framing, carried forward exactly: **this is a walkthrough มายด์
performs himself through the web owner login, not a criteria table.**
Anything only a test suite can exercise does not count toward it — same
standing constraint as every milestone before this one. He logs in and,
in one session:

1. Sees every agent's key in Settings and revokes one, **and that key
   stops working** — the clause that makes it a test, not a click.
2. Sees todos agents created, not just his own.
3. Sees a todo with status/assignee/priority/due date and changes one.
4. Opens the feed and sees who did what, with human vs agent
   distinguishable on every row.
5. Opens one todo and sees that same history reading identically (same
   row-rendering component as the feed).

Nothing merges to `main` without his own validation, same standing rule
as every milestone before this one.
