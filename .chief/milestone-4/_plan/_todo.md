# Milestone 4 TODO

See `../_goal/GOAL.md` for objective/scope/decisions/done-when,
`../_contract/API.md` for the new endpoints, and `.chief/_rules/_contract/
{DATA_MODEL,INVARIANTS}.md` for the schema and I15–I21. Every Done-when
item is owned by exactly one task. Dependency-ordered — later tasks build
on earlier ones landing first.

**Standing discipline for every task in this milestone, restated because
it's the one that already bit this codebase once**: every new test gets
asked *"would this still be green if the thing it guards were absent
entirely?"* before it's trusted — not "does it pass." No agent fixture by
direct repo insert; go through `cmd/issue-key`'s real path. A
name-matching or count-based check states its floor deliberately (I15's
own fix this milestone is the concrete example) rather than accepting
zero matches as a pass.

**Green gate, stated per task rather than assumed** (Clara's finding: six
tasks touch shared schema/service code before the suite can mean anything
again — without saying which suite is red for which reason at each step,
a genuine regression in task-5 hides inside failures everyone already
agreed to expect):

- **task-1 ends green on its own migration test, in isolation.** Nothing
  else in the repo compiles yet — `todo.Service`, both transports, and
  every existing todo test all reference `owner_id`/`done`, which this
  task removes. **`go build ./...` and `go test ./...` are expected red
  after task-1, for exactly this reason — not a regression, the reason
  is "not updated yet," and task-2 is where that stops being true for
  the service layer.**
- **task-2 ends green on `go test ./internal/domain/todo/...` and the
  architecture test specifically**, not the whole repo — the handler
  packages (`publicapi`, `bff`) still reference the old service shape
  until tasks 3/4 update them. `go build ./...` still red, same reason
  as task-1, now localized to the transport packages instead of the
  domain package.
- **task-3 ends green on `go build ./...` and `go test
  ./internal/transport/publicapi/...`** — the first point the whole repo
  compiles again. `bff` may still be red until task-4 lands (a real,
  expected gap, not a regression).
- **task-4 ends green on `go build ./...` and `go test ./...`** — the
  first point the full Go suite is green again. From here on, red in any
  Go package is a real regression, not an expected gap.
- **tasks 5, 6, 8 each end green on the full Go suite plus their own new
  tests** — no more "expected red" from here.
- **task-7 is where the JS suite exists to have a state at all** — expect
  it red or absent until this task lands; not evaluated before it.
- **task-9 is the only point both suites are required green together.**

- [ ] task-1: Schema + migration — `todos` gains `status` (enum, replaces
      `done`), `assignee_id`, `priority`, `due_date`, `created_by`
      (replaces `owner_id`); new `todo_events` table (`_rules/_contract/
      DATA_MODEL.md`'s exact columns/indexes). sqlc queries for all of the
      above, including the ones I15's own architecture-test fix will need
      to reference by name (`internal/domain/todo`'s repo file only).
      **The migration itself is the deliverable, not just the schema**:
      `done = true` → `status = 'done'`; `done = false` → `status =
      'open'`. `owner_id` → `created_by` (rename, no value transform).
      **Verified against a database seeded with pre-existing rows in both
      `done` states before the migration runs** — a test asserting the
      exact post-migration `status` value per pre-migration `done` value,
      not merely that the migration executes without error against an
      empty database. This test must be green in isolation even though
      the rest of the repo isn't — see the green-gate note above.
      **Owns: Done-when 1.**
- [ ] task-2: Domain service layer — `todo.Service` extended for the new
      fields; the single write path for `todo_events` (I15): idempotency
      check (I19) → permission check (I18, **inside** the write path
      itself, per the contract's explicit strengthening — not at each
      call site) → dispatch by event type → domain-specific side effect →
      insert, one transaction. `type: "created"` never accepted as a
      client-specified write at the service level (I16) — the HTTP-level
      proof of this is Done-when 13/14, owned by tasks 3/4, not this one.
      Permission layer (`can`-equivalent, role-based): owner
      unconditional, agents refused only on `status: closed`.
      **I15's own enforcement fix, built here**: extend
      `internal/architecture_test.go` (or a new test in the same file)
      with the table-specific check — asserts **at least 3** functions
      matching the todo-event query names exist (the design's floor:
      insert, list-per-todo, list-cross-todo-feed) before asserting only
      `internal/domain/todo`'s repo file references them. A check that
      passes by matching zero functions enforces nothing — this is the
      one place this milestone's own contract review already caught that
      shape once; don't reintroduce it while building the fix for it.
      **Green gate: `go test ./internal/domain/todo/...` plus the
      architecture test, not the whole repo** — see the green-gate note
      above for why `go build ./...` staying red here is expected, not a
      regression.
      **Owns: Done-when 2, 3, 4, 5.**
- [ ] task-3: Public API surface (`internal/transport/publicapi`,
      `openapi.yaml`) — todo endpoints updated for the new fields (no more
      `done` in request/response bodies — a request that sends it is
      `validation_error`, per `_contract/API.md`). `POST/GET
      /api/v1/todos/:id/events` — the agent-facing event write/read
      surface, mirrors my-task's own REST shape (`_contract/API.md`'s
      exact body shapes per `type`). **`DELETE /api/v1/todos/:id`
      removed** — the route no longer exists (genuine 404, not 405).
      **`type: "created"` genuinely rejected (400), not silently
      accepted or dropped** — a real HTTP-level test against this
      handler, not inferred from I16 holding at the service layer.
      Implementation work for Done-when 6's public-API half; task-8 owns
      that Done-when item once the skill doc is also updated.
      **Green gate: `go build ./...` clean (first point the whole repo
      compiles again) and `go test ./internal/transport/publicapi/...`
      green.** `bff` may still be red until task-4 — expected, not a
      regression.
      **Owns: Done-when 13.**
- [ ] task-4: BFF surface (`internal/transport/bff`, `bff-openapi.yaml`)
      — same shape as task-3, session-authenticated: todo endpoints
      updated, `POST/GET /api/bff/todos/:id/events`, **`status: closed`
      succeeds here** (I18 — this is the owner's surface).
      **`DELETE /api/bff/todos/:id` removed.** **`type: "created"`
      genuinely rejected (400)**, tested independently of task-3's proof
      — two handlers, two chances to get this wrong separately.
      Implementation work for Done-when 6's BFF half; task-8 owns that
      Done-when item.
      **A cheap reachability check, at the end of this task specifically
      — not deferred to task-9's walkthrough.** Real running binary, a
      real owner session (the same `bff.Signer.NewSessionCookie` fixture
      this project's own tests already use), confirm the SPA shell still
      loads at `/` and at least one authenticated BFF endpoint (e.g.
      `GET /api/bff/me`) still answers correctly. Not the walkthrough —
      just proof the login door still opens, at the point the schema
      rename (task-1) has had the most opportunity to have broken session
      or user resolution without anyone noticing. If this breaks, the
      cause is at most two tasks back, not eight.
      **Green gate: `go build ./...` and `go test ./...` both clean — the
      first point the full Go suite is green again.** From here on, any
      red Go package is a real regression, not an expected gap.
      **Owns: Done-when 14.**
- [ ] task-5: Cross-todo activity feed — `GET /api/bff/activity`
      (`_contract/API.md`'s exact shape: cursor over `todo_events` across
      every todo, newest first, joined to `todos`/`users`). Owner-session
      only, mirrors my-task's `activity.list`.
      **The cross-actor proof, not just a working endpoint**: a test
      seeds an agent identity through `cmd/issue-key`'s real path, has
      that agent create a todo and act on it (at least one non-`created`
      event), then asserts the owner's feed query returns that event,
      correctly attributed to the agent. A feed that only ever showed the
      viewer's own events would pass every other check in this milestone
      — this is ruling 1's only real proof, and it doesn't exist until
      this test does.
      **Green gate: full Go suite green, plus this task's own new tests.**
      **Owns: Done-when 12.**
- [ ] task-6: Key-listing replacement (I21) — `GET /api/bff/keys` becomes
      "every `role='agent'` user's non-revoked keys"; `DELETE
      /api/bff/keys/:id` becomes "any agent's key, still session-gated to
      the owner." `GET /api/v1/keys` (agent Bearer) stays untouched and
      self-scoped — a test proves it. **`keys_handler_test.go`'s existing
      `newBFFRouterForTwoOwnersWithKeys`-based assertions are rewritten,
      not left in place** — they assert semantics this task removes, and
      the two-distinct-owners scenario they build was already fictional
      under this template's single-seeded-owner model independent of this
      change. New fixtures seed agent identities through `cmd/issue-key`,
      not a direct repo insert on a convenient role — the exact trap the
      old fixture fell into.
      **Revocation actually stops the key, both halves, not just the
      post-revoke check**: a test issues a key through `cmd/issue-key`,
      authenticates with it successfully against the public API first (so
      the negative half means something), revokes it through this
      milestone's new owner-facing endpoint, then proves the *same* key
      now fails. A test that only checks the post-revoke 401 is
      consistent with the key never having worked at all.
      **Green gate: full Go suite green, plus this task's own new tests
      (including the rewritten `keys_handler_test.go`).**
      **Owns: Done-when 7, 11.**
- [ ] task-7: SPA — per-todo detail page (`web/src/app/todos/[id]/` or
      equivalent, doesn't exist yet), todos list page updated for the new
      fields (status/assignee/priority/due-date UI, including a
      status-change control that surfaces `closed` only for the owner —
      the frontend's own reflection of I18, not a substitute for the
      backend enforcing it), activity feed page (new, mirrors my-task's
      `/` home page), settings page updated to list every agent's key
      (task-6's new endpoint) with per-key revoke.
      **One row-rendering component, shared between the feed and the
      per-todo timeline** — mirrors my-task's `TimelineEventRow` reuse
      exactly, so the two pages cannot drift apart in how an event reads.
      A 🧑/🤖 (or equivalent) provenance mark on every row, branching on
      the actor's role from the response — **a test proves the UI
      actually branches on it**, not just that a role field exists on the
      wire (milestone-2's Done-when-9 lesson, restated because it's
      exactly the right shape here too).
      **Comment body safety**: a Vitest test proves a body containing raw
      HTML tags renders as literal text/escaped elements in the rendered
      output — the concrete version of "never `dangerouslySetInnerHTML`"
      (I20).
      **The row-component-shared test**: mirrors milestone-3's
      Done-when-7 negative-control shape — a given event renders
      identically (same summary text, same provenance mark) regardless of
      which page's query fed it into the component.
      **Green gate: full Go suite green (unaffected by this task); this
      is where the JS suite's own state begins meaning something — expect
      it red or absent before this task, not evaluated before it.**
      **Owns: Done-when 8, 9, 10.**
- [ ] task-8: Companion `<service>-api` skill doc — DELETE no longer
      named anywhere (presence/absence check against the doc's actual
      text, not assumed from the code change alone); the new event
      endpoints documented (body shapes per `type`, mirroring how the
      existing todo/key endpoints are already documented); the field list
      updated for `status`/`assignee`/`priority`/`dueDate`. This is the
      integration point for Done-when 6 — tasks 3/4 already removed
      `DELETE` from the code; this task confirms the doc caught up too, a
      doc that still says "call DELETE" makes the removal undone for the
      only readers who act on it.
      **Green gate: full Go suite green (doc-only task, unaffected).**
      **Owns: Done-when 6.**
- [ ] task-9: Final verification — **last task, full-suite gate, same
      shape as milestone-3's task-5.** `go test ./...` and the JS suite
      green together, from a fresh clone, not independently at the point
      each was last touched. `docker compose up` still works.
      **Then the walkthrough gate (Clara/มายด์'s explicit instruction,
      not optional)**: attempt มายด์'s own five-step acceptance walkthrough
      as far as this crew's tooling actually reaches — no browser
      automation exists in this toolset (checked, confirmed absent; not
      to be closed by adding one, that's parked as its own ticket). Every
      step gets one of three labels, not a pass/fail: **completed** (the
      real thing, say what was seen), **partially reached** (say exactly
      how far and name the residue — e.g. "verified by curl against the
      running binary with a real minted key; the Settings button that
      triggers it is unexercised"), or **browser-only** (not reachable by
      any means available). The two facts — verified at the API layer,
      not verified at the layer มายด์ uses — both get stated; neither
      substitutes for the other. Report to Clara before claiming the
      milestone ready, not after.
      **Owns: none new — confirms what the rest of this milestone's tasks
      already built.**

## Open items, not owned by any task above

- **มายด์'s gate is non-blocking until the acceptance walkthrough** — his
  own words, relayed by Clara: *"I'll be the non-blocking, I'll waiting
  for acceptance test on the ui at the end."* Clara is the sole gate on
  goal/contract/plan/build; nothing here waits on มายด์ mid-flight.
  **This does not change the close** (TPL-2 still closes only on his
  quoted acceptance of the real walkthrough) or **the merge rule**
  (nothing goes to `main` without his own validation, unchanged from
  TPL-1) — both stated here so neither gets assumed loosened by the gate
  change.
- **Clara is keeping a running list of things มายด์ would want to know at
  acceptance, not now** — I21 (the owner deliberately seeing every
  agent's keys), the `done`→`status` migration mapping and its
  visibility consequence, and TPL-3 (not this milestone's — noted only
  because Clara mentioned it, not investigated). Anything that would make
  him angry to learn late is an interrupt to Clara, not a list item — bias
  toward telling her rather than deciding something is list-shaped
  unilaterally.
- **The no-browser-automation limit is a known, accepted gap for this
  milestone specifically**, not a general policy — Clara confirmed
  directly (checked for a fleet-wide `docker-playwright` skill, found it
  unavailable on this crew's path; confirmed `my-template` has no
  `@playwright/test` at all) and is routing a browser-e2e-for-the-template
  gap to มายด์ as its own, separate ticket. Do not add a browser toolchain
  to close task-9's residue — that would be absorbing scope this
  engagement's whole process exists to keep separate.
