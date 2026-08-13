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

**Superseding update (Clara, after task-2): named-command gates exclude
by construction.** Task-2's own gate (`go test ./internal/domain/todo/...`
plus `go test ./internal/ -run TestArchitecture`) was green while a real
regression — `TestI4_TodoRepoOnlyQueriesTodosTable` dropped,
`TestI3_...` renamed out of its required prefix — sat undetected in
`internal`, a package neither named filter touched. Found only because
verification happened to also run an unfiltered sweep; the stated gate
itself would not have caught it. **Standing requirement for every
remaining task, superseding the per-task lines above where they
conflict**: end with an **unfiltered** `go test ./internal/...` (no
`-run`), plus the JS suite once task-7 gives it a real state, and
classify every failure against the **named, explicit known-red
baseline** below — not "expected failures exist somewhere," the actual
list:

- `TestDoneWhen12_EveryInvariantHasANamedTest` failing, specifically and
  only on I20 (task-7's) and I21 (task-6's) having no test yet.

**Re-measured at `d84be82`** (task-4's own report commit), from a genuine
fresh clone, not the working tree — superseding the `5d90d1d` measurement
this line has carried since before task-3 landed (the `publicapi` line
above was already stale once task-3 landed and is dropped here, along
with the sha). **`internal/transport/bff` no longer belongs on this
list**: task-4 made the *first point the full
Go suite is green again* except for the two named `TestDoneWhen12` gaps —
confirmed independently (Luna, cold clone, own separate mutation attack
on `TestDoneWhen14`, own reproduction of the reachability check end to
end) before this line was updated. From here on, **any red Go package at
all is a real regression**, not an expected gap — the baseline is down to
exactly the two invariant-coverage lines above.

A baseline is a measurement, and a measurement has a subject — record the
commit it was taken against, not just the list, since the list can read
identical while the *reason* underneath it changed (this milestone's own
scope-tags fix-round is the worked example: `TestDoneWhen12` read
`{I20, I21}` before and after, for a completely different mechanism).
**When any change lands that could alter *why* the baseline fails,
re-measure it fresh against the new commit rather than carrying the old
list forward — if the set or the reason changed, that's a finding to
report, not a number to silently update.**

**Any failure outside this exact list is a regression until proven
otherwise.** The baseline only shrinks as tasks land what it's waiting
on — it never grows silently; if a task finds it genuinely needs to grow
the baseline, that's a report to Clara, not a unilateral edit here.

**`TestDoneWhen12` itself is not a reliable safety net right now** —
Clara's independent finding (see below): its `per-domain-module` scope
check only ever enumerates `internal/domain/*` (currently just `todo`),
so I3/I4 coverage in `internal/identity` is invisible to it even though
real `TestI3_`/`TestI4_` tests exist there, and I15-I19/I21 are
category-mismatched onto the same tag (I21 specifically will block
task-6: a correctly-placed `TestI21_` in `internal/identity` is exactly
what the current mechanism won't accept). **This must be resolved before
task-6** — a design proposal is with Clara, not yet decided or built.
Until it's fixed, `TestDoneWhen12`'s own red/green is read with that
unreliability in mind, not trusted as the mechanism that would have
caught a missing invariant test.

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
- [x] task-4: BFF surface (`internal/transport/bff`, `bff-openapi.yaml`)
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
      real **minted** owner session (`bff.Signer.NewSessionCookie`, the
      same fixture this project's own tests already use), confirm the SPA
      shell still loads at `/` and at least one authenticated BFF endpoint
      (e.g. `GET /api/bff/me`) still answers correctly. **Named precisely,
      not as "the login door still opens"**: this proves session
      *consumption* and user resolution survived the schema rename (task-1)
      — given a valid session, does it still resolve to the right actor
      and serve the right data. **It does not touch session
      *establishment*** — the SSO redirect, the callback, the scope, the
      cookie actually being set by a real login — which stays browser-only
      until task-9. ENG-13's three faults were all upstream of the cookie
      existing (an auth-gate misread, a scope mismatch, a stale cached
      bundle) and every one of them would pass this check, because every
      one of them lives in establishment, not consumption. Still worth
      having exactly here — it catches the specific risk it was added for
      (the rename breaking resolution), two tasks after the rename instead
      of eight — just not a substitute for what it doesn't reach.
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

- [ ] **Fix-round, before task-6, owns no Done-when — not folded into
      task-6.** `internal/invariants_test.go`'s `scope:` mechanism
      conflates two ideas under `per-domain-module`: "applies to every
      domain module" (true of I3/I4) and "belongs to exactly one named
      place" (true of I15-I19, I21) — correct only because the domain-
      module set currently has one member. Confirmed independently:
      `domainModuleNames()` enumerates `internal/domain/*` only;
      `internal/identity`'s real `TestI3_`/`TestI4_` tests exist and are
      genuinely never looked at; I21 (an identity invariant) is tagged
      `per-domain-module`, so a correctly-placed `TestI21_` in
      `internal/identity` — exactly what task-6 needs to write — would
      not satisfy the check, and a stub `TestI21_` under
      `internal/domain/todo` would.
      **The fix**: a new `scope: domain:<name>` tag form, resolved by a
      small explicit name→package mapping (`domain:todo` →
      `internal/domain/todo`, `domain:identity` → `internal/identity`
      — not assumed to live under `internal/domain/`, since identity
      deliberately doesn't per `ARCHITECTURE.md`'s own milestone-2
      decision). Reassign I15-I19 → `domain:todo`, I21 → `domain:identity`.
      **An unknown name in that mapping must abort loudly, not resolve to
      a no-op check** — the exact shape of every other "matches nothing,
      passes trivially" gap this milestone has already found and fixed
      once (I15's own floor, the sqlc-ignores-Down measurement). A test
      proves a deliberately bogus scope tag makes the suite fail, not
      silently pass.
      `per-domain-module` stays, for I3/I4 only, with its own **explicit,
      hand-maintained** enumeration (not `domainModuleNames()` — that
      function answers "what counts as a domain module for restructuring
      purposes," and `internal/identity` failing it is the design, not a
      gap this fix should paper over by widening it). **Assert the
      explicit list is a superset of `domainModuleNames()`**, so a new
      domain module nobody remembered to add fails loudly instead of
      being silently exempt — this doesn't catch a new non-domain package
      the way identity was missed (nothing mechanical will), so the file
      states plainly that the list is hand-maintained and what adding a
      package requires.
      **Attack standard**: move a correctly-placed `TestI21_` out of
      `internal/identity` and confirm the checker catches it; put a stub
      `TestI21_` under `internal/domain/todo` and confirm that does
      **not** satisfy it — this second one is the exact shortcut the old
      mechanism would have silently accepted, and it's the one that
      matters.
      **Not on มายด์'s acceptance walkthrough** — internal engineering
      that doesn't change what he sees; goes on his told-not-gated
      acceptance list instead.
      **Green gate: unfiltered `go test ./internal/...`, checked against
      the current known-red baseline (above) — this fix-round should
      shrink `TestDoneWhen12`'s own failure mode, not add to the
      baseline.**

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
  visibility consequence, TPL-3 (not this milestone's — noted only
  because Clara mentioned it, not investigated), and **`status` is not
  settable on `POST /todos` (Clara's decision)** — every new todo starts
  `open` regardless of what a caller asks for; `assigneeId`/`priority`/
  `dueDate` ARE settable at creation, only `status` is not. Deliberate,
  not an oversight: มายด์'s own walkthrough only requires *changing* a
  todo's status after it exists, never setting one at creation, and
  `open` as the universal starting point satisfies that. `_contract/
  API.md` originally said `status` was optionally acceptable at create;
  task-3's implementation couldn't (`CreateInput` has no such field) and
  flagged the gap rather than silently working around it; Clara took the
  gap as the decision rather than routing a fix. Contract and OpenAPI spec
  updated to state this as intentional, not a residual TODO. A small
  shape choice he might have opinions about — he should hear it from us,
  not discover the asymmetry himself. Anything that would make him angry
  to learn late is an interrupt to Clara, not a list item — bias
  toward telling her rather than deciding something is list-shaped
  unilaterally.
- **I4's contract text was amended (`_rules/_contract/INVARIANTS.md`,
  same shape as the I3 scope note)** — a query file may now *read* (never
  write) a table it doesn't own, but only through an explicit, named,
  mechanically-enforced grant (`internal/dbquery.ReadOnlyGrants`); today's
  one grant lets the cross-todo activity feed show an agent's handle/role
  alongside their events. Clara's call, stated as a clarification of what
  I4 always meant in practice, not a weakening. Internal engineering, does
  not touch มายด์'s walkthrough — on his told-list, not a gate, same as
  I21 and the migration mapping above.
- **The activity feed's same-millisecond order is by `id`, not by write
  order (Clara's decision, `_contract/API.md` states it explicitly)** —
  two events written within the same millisecond can display in either
  relative order; only pagination (no drops, no duplicates) is guaranteed,
  not causal ordering that fine-grained. Found while fixing a real bug: an
  earlier version silently *dropped* a same-millisecond event from the
  feed entirely (a precision mismatch between the stored timestamp and
  the wire cursor), fixed by truncating storage to match the cursor's own
  millisecond precision — the fix trades that data loss for *this*,
  cosmetic, undefined-order residue, which matches my-task's own design
  (its cursor's ids are equally uncorrelated with write order). The feed
  is the centrepiece of what มายด์ actually asked for, and two events
  landing out of causal order in a fast burst is exactly the kind of
  thing he'd notice at acceptance and wonder about — worth him hearing
  it's known and why, not discovering it unexplained.
- **The no-browser-automation limit is a known, accepted gap for this
  milestone specifically**, not a general policy — Clara confirmed
  directly (checked for a fleet-wide `docker-playwright` skill, found it
  unavailable on this crew's path; confirmed `my-template` has no
  `@playwright/test` at all) and is routing a browser-e2e-for-the-template
  gap to มายด์ as its own, separate ticket. Do not add a browser toolchain
  to close task-9's residue — that would be absorbing scope this
  engagement's whole process exists to keep separate.
