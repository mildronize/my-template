# Task task-7 Report

## Task

SPA frontend: per-todo detail page, todos list page updated for the new
fields (status/assignee/priority/due-date UI, including a status-change
control that surfaces `closed` only for the owner), activity feed page
(new, mirrors my-task's `/` home page), settings page updated to list
every agent's key (task-6's endpoint) with per-key revoke. One
row-rendering component shared between the feed and the per-todo
timeline, mirroring my-task's `TimelineEventRow` reuse exactly. A 🧑/🤖
provenance mark on every row, branching on the actor's role — proven by a
test that would fail if the branch were hardcoded. Comment-body safety
(I20) proven by a real Vitest test: raw HTML in a body renders as
literal/escaped text, never live DOM. Owns **Done-when 8, 9, 10**.

## Outcome

done

## What was already there vs. what this task built

Everything in `web/src` before this task was pre-milestone-4 shape:
`Todo` had `done: boolean`, no status/assignee/priority/due-date, no
event endpoints, no activity feed, no per-todo detail route. `web/package.json`
had no markdown-rendering library at all. This task built:

- `web/src/components/Markdown.tsx` — new. Ported from my-task's own
  `src/components/Markdown.tsx` (react-markdown + remark-gfm, no HTML
  string ever produced, image loading disabled, link `rel`/`target`
  hardened, `urlTransform` stated locally). Adapted for this repo: same
  logic, one comment-wording change made necessary by a discovery below.
- `web/src/components/TimelineEventRow.tsx` — new, the shared row
  component (Done-when 5/8/9). Ported structurally from my-task's own
  `src/components/TimelineEvent.tsx`: same per-event-type summary split,
  same comment-vs-non-comment layout split, same provenance-mark
  approach — but payload shapes rewritten from scratch against this
  repo's own actual wire data (`internal/domain/todo/service.go`'s
  `Append`), not assumed to carry over from my-task's richer shapes. See
  "Contract gaps found" below for the one place this repo's data is
  structurally thinner than my-task's.
- `web/src/components/TimelineEventRow.test.tsx` — new. Done-when 8/9/10's
  own tests, see "The three graded tests" below for full content.
- `web/src/lib/activity.ts` — new. `useActivityFeedQuery` (`useInfiniteQuery`
  over `GET /api/bff/activity`, cursor round-tripped verbatim) and
  `activityItemToTimelineEvent`, the feed page's own adapter into the
  shared component's prop shape.
- `web/src/lib/todos.ts` — rewritten. `done` replaced by
  `status`/`assigneeId`/`priority`/`dueDate` throughout; `useDeleteTodoMutation`
  removed (`DELETE /api/bff/todos/:id` no longer exists); every mutation
  now carries a `clientRequestId` (I19) generated internally via
  `crypto.randomUUID()`. New: `useTodoQuery`, `useTodoEventsQuery`,
  `useCreateTodoEventMutation` (I15's single write path, every
  client-postable event type), `todoEventToTimelineEvent` (the per-todo
  timeline's own adapter), `canCloseTodo` (I18's frontend reflection,
  documented below).
- `web/src/app/activity/ActivityList.tsx` + `ActivityPage.tsx` + `ActivityList.test.tsx`
  — new. Mirrors `TodosList`/`TodosPage`'s existing thin-render-component
  split (Done-when 7's own convention), paginated with a "Load more"
  button.
- `web/src/app/todos/TodoDetailPage.tsx` — new. Status/priority/assignee
  controls, a comment box, and the timeline rendered through
  `TimelineEventRow`.
- `web/src/app/TodoRow.tsx` — rewritten. Done-checkbox/delete-behind-confirm
  shape replaced by a status-select, priority badge, due-date, assignee,
  and a link to the detail page. `useDeleteTodoMutation` no longer exists
  to call.
- `web/src/app/NewTodoDialog.tsx` — one-line change (`createTodo.mutate(trimmed, ...)`
  → `createTodo.mutate({ title: trimmed }, ...)`, matching the new hook
  signature). Still title-only creation — GOAL.md's task-7 spec never
  asks for assignee/priority/due-date at creation time.
- `web/src/app/settings/ApiKeySettings.tsx` — copy updated ("your keys" →
  "every agent's keys"); the component's actual shape barely changes
  since `ApiKey`'s wire schema didn't change, only `GET /api/bff/keys`'s
  query scope did (task-6).
- `web/src/App.tsx` / `web/src/components/Header.tsx` — two new routes
  (`/todos/:id`, `/activity`) and an "Activity" nav link.
- `web/src/lib/api/bff-schema.gen.ts` — regenerated via
  `npm run generate:api` against the already-updated `bff-openapi.yaml`
  (tasks 3-6). Not hand-edited.
- `web/package.json` / `package-lock.json` — added `react-markdown@^10.1.0`,
  `remark-gfm@^4.0.1`.
- Existing tests updated for the new shape: `web/src/app/TodosList.test.tsx`,
  `web/src/lib/todos.test.tsx` (rewritten for `status`, `clientRequestId`,
  the new event mutation, plus a `canCloseTodo` unit-test block).

## Contract gaps found (named, not silently resolved)

One recurring pattern, three sites, all found while building this task,
none of them mine to fix (no `.go` files touched, per this task's own
scope):

1. **`GET /api/bff/todos/:id/events`'s `TodoEvent` carries `actorId`
   only — no `handle`, no `role`.** Confirmed by reading
   `internal/bffapi/bffapi.gen.go`'s `TodoEvent` struct and
   `internal/transport/bff/todo_handler.go:100-105`'s `toBFFEvent`, which
   never resolves one. `GET /api/bff/activity`'s `ActivityItem`, by
   contrast, genuinely carries `actor: {handle, role}` on every row (a
   real SQL join, `todo_handler.go` around line 502). `bff-openapi.yaml`'s
   own `ActivityItem` doc comment says the two schemas represent "the same
   underlying row... so the two share a rendering component" — true for
   `type`/`payload`/`body`, not true for `actor`. This is load-bearing for
   Done-when 6 ("a provenance mark on every row"): the per-todo timeline
   genuinely cannot know whether a row's actor is the owner or an agent
   from this endpoint's own data. I did not invent a client-side lookup —
   there's no endpoint that maps a user id to a handle/role in this
   milestone's contract. `TimelineEventRow`'s `ProvenanceMark` has a
   third, honest state for this (`role !== "owner" && role !== "agent"` →
   ❔ "Unknown provenance"), and `todoEventToTimelineEvent`'s adapter
   (`~/lib/todos.ts`) defaults every per-todo-timeline row to
   `{handle: event.actorId, role: "unknown"}` in production — never a
   guessed real role. Both are documented in-code at the exact spot a
   future reader would look (`TimelineEventRow.tsx`'s `TimelineEventData`
   doc comment, `todoEventToTimelineEvent`'s own doc comment).
2. **`Todo.assigneeId` and the `assigned` event's `{from, to}` payload are
   bare user ids, not handles** (`internal/domain/todo/service.go:337`'s
   `strPtrToAny(current.AssigneeID)` directly) — same category of gap.
   `TodoRow.tsx`/`TodoDetailPage.tsx`'s assignee controls show/accept the
   raw id verbatim rather than inventing a display name.
3. **`ApiKey` (task-6, `bff-openapi.yaml`) carries `id`/`prefix`/`createdAt`/`expiresAt`
   only — no field naming which agent a key belongs to.** Confirmed
   against `internal/transport/bff/keys_handler.go`'s own `toBFFKey`.
   `ApiKeySettings.tsx` can list and revoke every agent's key (I21, as
   required) but cannot label a row "this is clara's key," only its
   prefix and dates.

All three share one root cause: a wire schema that carries a raw
id/prefix where a human-facing UI wants a name, and no lookup endpoint
this milestone's contract provides to bridge the two. I flagged each at
its own site rather than inventing a client-side fix (a fabricated
handle would be worse than an honest gap), and I'm naming the pattern
here rather than leaving it as three separate, easy-to-miss comments.
This is Clara's call, not mine to resolve unilaterally — a future task
adding something like `GET /api/bff/users` (or enriching `TodoEvent`/
`ApiKey`/`assigned` payloads directly) would close all three at once.

## A discovery mid-task: I20's Go-side half already existed, and it changes the report I expected to write

The task brief asked me to read `internal/invariants_test.go`'s
`TestDoneWhen12` logic and report on whether any bridge exists from a
Vitest test name into its Go-only `TestI<N>_`-prefixed scan — flagging it
as an unresolved gap if none did, not inventing one unilaterally.

At session start (branch head `935ff21`), that gap was real:
`collectTestFuncNames` (`internal/invariants_test.go`) only walks
`*_test.go` files for `^func (Test\w+)\(` — a Vitest test named
`TestI20_...` in TypeScript is structurally invisible to it, and
`go test ./internal/...` at that commit showed exactly one failure:

```
$ go test ./internal/... 2>&1 | tail
--- FAIL: TestDoneWhen12_EveryInvariantHasANamedTest
    invariants_test.go:417: no test named TestI20_<something> found
      anywhere under ... — I20 (scope: global) has no test referencing it
FAIL	github.com/mildronize/my-template/internal	1.939s
ok  	.../internal/dbquery	(cached)
ok  	.../internal/domain/todo	0.248s
ok  	.../internal/identity	1.197s
ok  	.../internal/platform	(cached)
ok  	.../internal/transport/bff	3.715s
ok  	.../internal/transport/publicapi	0.300s
FAIL
```

Partway through this task, I re-ran `go test ./internal/...` (after
writing `TimelineEventRow.tsx`/`Markdown.tsx`, before finishing the other
pages) and it failed differently — not `TestDoneWhen12` anymore, but a
**new** test, `TestI20_FrontendNeverUsesDangerouslySetInnerHTML`, in a
file I had never seen: `internal/frontend_safety_test.go`. I did not
create this file — `git status --short` at every point in this task
shows only `web/` changes, and I made zero `.go` edits and zero commits
during this task. `git log`/`git reflog` show it landed as a real commit,
`2787432` (`feat(milestone-4/i20-static-check): Go-side structural half
of I20`, author `Thada Wangthammang`, co-authored `Claude Sonnet 5`,
timestamp `2026-08-13T16:49:54Z`), on top of `935ff21` — i.e. someone
else pushed directly to this branch's HEAD while this task was in
flight, adding exactly the missing bridge.

Reading that file (`internal/frontend_safety_test.go`), its own package
doc comment states the reasoning almost verbatim to what the task brief
asked me to investigate: `TestDoneWhen12` can't see a Vitest test by
name, so rather than write a hollow `TestI20_` Go stub purely to satisfy
the name scan, it adds a **genuine, independently meaningful** Go-side
check — `dangerouslySetInnerHTML` (the literal identifier) does not
appear anywhere in `web/src`'s `.ts`/`.tsx`/`.js`/`.jsx` files — with a
non-zero-floor assertion (`meetsI20Floor`, ≥10 files scanned, same shape
as I15's own floor fix) so a moved/renamed `web/src` can't make the scan
pass by finding nothing. It is named `TestI20_FrontendNeverUsesDangerouslySetInnerHTML`,
which satisfies `TestDoneWhen12`'s global-scope name-prefix check by
construction, **and** does real, independent work — the exact "structural
half, not a substitute for the behavioral half" pairing its own comment
describes, with the Vitest test in this task named as its explicit
counterpart.

**So the answer to the question the task brief asked me to investigate
is: the bridge exists, was not invented by me, and is not a stub.**
`TestDoneWhen12`'s failure list is now empty after this task — not
"only I20, as expected," but genuinely empty, because I20 is doubly
covered (Go-side structural + JS-side behavioral) rather than left as
the one open item. This is a materially better outcome than the green
gate section of the task brief described as the expected shape ("`go
test ./internal/...` should show only `TestDoneWhen12` failing on I20"),
and I want that difference stated plainly rather than quietly enjoyed.

**One real consequence of this for my own work**: `TestI20_FrontendNeverUsesDangerouslySetInnerHTML`
does a literal, case-sensitive substring scan across every frontend
source file, including comments. My first draft of `Markdown.tsx`'s own
header comment explained, in prose, that "`dangerouslySetInnerHTML` is
never used" — which is itself an occurrence of the forbidden substring,
and tripped the check as a false positive (the failure output is
reproduced in full below, in Verification). I fixed it by rephrasing the
comment to describe the API without spelling the identifier literally,
confirmed no file under `web/src` contains the substring
(`grep -rl dangerouslySetInnerHTML web/src` → no matches), and reran the
full Go suite green. I'm naming this rather than treating it as a
one-line fix with no story: a naive substring check has this exact
failure mode built in — writing *about* the forbidden thing trips the
same wire as *doing* it — and it's worth knowing that constraint applies
to every frontend source comment from here on, not just this file's.

## The three graded tests — verbatim

`web/src/components/TimelineEventRow.test.tsx`, full file:

```tsx
// milestone-4/task-7: the three tests this task is actually graded on
// (GOAL.md Done-when 8, 9, 10 — see task-7's own report for the full
// discipline this file follows, including the two required attacks run
// against it).
//
//  - "Done-when 8" describe block: proves TimelineEventRow is genuinely
//    ONE shared component driving both the activity feed and the
//    per-todo timeline, not two components that happen to render
//    alike today — imports the component once, drives it through both
//    pages' own real adapter functions (`todoEventToTimelineEvent`,
//    `activityItemToTimelineEvent`), and asserts byte-identical rendered
//    output for the same semantic event.
//  - "Done-when 9" describe block: proves the 🧑/🤖 provenance mark
//    actually branches on `actor.role`, not just that a mark exists —
//    written so a hardcoded mark would fail it (attacked by hand, see
//    task-7's report).
//  - "Done-when 10 / I20" describe block: the one Clara said she is
//    personally reading, not trusting the mechanism for — proves a
//    comment body containing raw HTML tags renders as literal, escaped
//    text, never as live DOM elements (no `<script>`, no live `<img>`).
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

import { TimelineEventRow, type TimelineEventData } from "./TimelineEventRow";
import { todoEventToTimelineEvent, type TodoEvent } from "~/lib/todos";
import { activityItemToTimelineEvent, type ActivityItem } from "~/lib/activity";

// vitest.config.ts sets `globals: false`, so @testing-library/react's own
// automatic afterEach-cleanup never registers (see AuthGate.test.tsx's own
// identical comment on this) — without this, a prior test's rendered DOM
// (a provenance mark, a rendered <script> tag as text, ...) stays attached
// to document.body and leaks into the next test's `screen` queries.
afterEach(() => {
  cleanup();
});

describe("TimelineEventRow — Done-when 8: shared between the feed and the per-todo timeline", () => {
  it("renders byte-identical output for the same event through both pages' own adapters", () => {
    // The same semantic event ("todo-9's status moved from open to
    // in_progress, done by agent-luna"), expressed once as GET
    // /todos/:id/events's own TodoEvent wire shape and once as GET
    // /activity's own ActivityItem wire shape — the two real response
    // types this repo's two pages actually decode.
    const sharedActor = { handle: "agent-luna", role: "agent" };
    const sharedFields = {
      id: "evt-shared-1",
      seq: 3,
      type: "status_changed",
      payload: { from: "open", to: "in_progress" },
      body: null,
      createdAt: "2026-01-01T12:00:00Z",
    };

    const todoEvent: TodoEvent = {
      ...sharedFields,
      todoId: "todo-9",
      actorId: "user-agent-luna",
      clientRequestId: "cr-1",
    };
    const activityItem: ActivityItem = {
      ...sharedFields,
      actor: sharedActor,
      todo: { id: "todo-9", title: "ship the thing" },
    };

    // TodoDetailPage.tsx's own real per-todo-timeline path: TodoEvent has
    // no actor of its own (see TimelineEventRow.tsx's TimelineEventData
    // doc comment for why), so its adapter takes one as a second
    // argument — here supplied explicitly to prove the adapter+component
    // pairing renders identically to the feed's own path when given
    // equivalent data, independent of that separately-documented gap.
    const fromTodoTimeline: TimelineEventData = todoEventToTimelineEvent(todoEvent, sharedActor);
    // ActivityPage.tsx's own real feed path: ActivityItem carries its own
    // actor directly.
    const fromActivityFeed: TimelineEventData = activityItemToTimelineEvent(activityItem);

    // One import of TimelineEventRow, driven twice — not two components
    // that happen to agree today.
    const { container: timelineContainer } = render(<TimelineEventRow event={fromTodoTimeline} />);
    const { container: feedContainer } = render(<TimelineEventRow event={fromActivityFeed} />);

    expect(timelineContainer.textContent).toBe(feedContainer.textContent);
    expect(timelineContainer.innerHTML).toBe(feedContainer.innerHTML);

    // Not a vacuous comparison of two empty strings — pin what's actually
    // in there.
    expect(timelineContainer.textContent).toContain("changed status");
    expect(timelineContainer.textContent).toContain("open");
    expect(timelineContainer.textContent).toContain("in_progress");
    expect(timelineContainer.textContent).toContain("agent-luna");
  });
});

describe("TimelineEventRow — Done-when 9: the provenance mark branches on role", () => {
  const baseEvent: TimelineEventData = {
    id: "evt-1",
    seq: 1,
    type: "commented",
    body: "hi",
    createdAt: "2026-01-01T00:00:00Z",
    actor: { handle: "someone", role: "owner" },
  };

  it("shows the human mark for role: owner, and never the agent mark", () => {
    render(<TimelineEventRow event={{ ...baseEvent, actor: { handle: "มายด์", role: "owner" } }} />);
    expect(screen.getByLabelText("Human")).toBeInTheDocument();
    expect(screen.queryByLabelText("Agent")).not.toBeInTheDocument();
  });

  it("shows the agent mark for role: agent, and never the human mark", () => {
    render(
      <TimelineEventRow event={{ ...baseEvent, actor: { handle: "clara-bot", role: "agent" } }} />,
    );
    expect(screen.getByLabelText("Agent")).toBeInTheDocument();
    expect(screen.queryByLabelText("Human")).not.toBeInTheDocument();
  });

  it("shows neither real mark for an unrecognized role, rather than guessing", () => {
    render(
      <TimelineEventRow event={{ ...baseEvent, actor: { handle: "?", role: "unknown" } }} />,
    );
    expect(screen.queryByLabelText("Human")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Agent")).not.toBeInTheDocument();
    expect(screen.getByLabelText("Unknown provenance")).toBeInTheDocument();
  });
});

describe("TimelineEventRow — Done-when 10 / I20: comment bodies never render as raw HTML", () => {
  it("renders a <script> tag in a comment body as literal escaped text, never as a live element", () => {
    const maliciousEvent: TimelineEventData = {
      id: "evt-xss-1",
      seq: 1,
      type: "commented",
      body: "look at this: <script>window.__xss = true;</script>",
      createdAt: "2026-01-01T00:00:00Z",
      actor: { handle: "some-agent", role: "agent" },
    };

    const { container } = render(<TimelineEventRow event={maliciousEvent} />);

    // The actual proof: no live <script> element reached the DOM.
    expect(container.querySelector("script")).toBeNull();
    // And the dangerous markup didn't just vanish (which skipHtml would
    // have done, silently) — it's visible, literal text.
    expect(container.textContent).toContain("<script>window.__xss = true;</script>");
  });

  it("renders an <img onerror=...> tag in a comment body as literal escaped text, never as a live element", () => {
    const maliciousEvent: TimelineEventData = {
      id: "evt-xss-2",
      seq: 1,
      type: "commented",
      body: '<img src=x onerror="window.__xss = true">',
      createdAt: "2026-01-01T00:00:00Z",
      actor: { handle: "some-agent", role: "agent" },
    };

    const { container } = render(<TimelineEventRow event={maliciousEvent} />);

    // No live <img> element with an onerror handler reached the DOM. This
    // also indirectly proves onerror never fired: there is nothing in the
    // DOM capable of firing it.
    expect(container.querySelector("img[onerror]")).toBeNull();
    expect(container.querySelector("img")).toBeNull();
    expect(container.textContent).toContain('<img src=x onerror="window.__xss = true">');
  });

  it("renders raw HTML in a field_changed event's from/to preview as literal text too (not just comment bodies)", () => {
    // field_changed previews are plain text (TimelineEventRow.tsx's own
    // FieldChangedSummary — deliberately not markdown), so the relevant
    // proof there is React's own default text-node escaping, not
    // react-markdown's — included as a second, independent site so I20's
    // "there is exactly one place a comment body is rendered" claim is
    // checked rather than assumed for the one field that actually goes
    // through Markdown, and confirmed as plain text everywhere else.
    const event: TimelineEventData = {
      id: "evt-xss-3",
      seq: 1,
      type: "field_changed",
      payload: { field: "title", from: "old", to: '<img src=x onerror="window.__xss = true">' },
      body: null,
      createdAt: "2026-01-01T00:00:00Z",
      actor: { handle: "some-agent", role: "agent" },
    };

    const { container } = render(<TimelineEventRow event={event} />);

    expect(container.querySelector("img")).toBeNull();
    expect(container.textContent).toContain('<img src=x onerror="window.__xss = true">');
  });
});
```

## The two required attacks — real before/after output

### Attack 1 (Done-when 9) — hardcode the provenance mark to always show 🧑 (owner), regardless of role

`ProvenanceMark` temporarily replaced with:

```tsx
function ProvenanceMark({ role: _role }: { role: string }) {
  // ATTACK (task-7 report, Done-when 9): hardcoded regardless of actual
  // role — temporary, reverted before commit.
  return (
    <span title="Human" aria-label="Human" className="text-base leading-none">
      🧑
    </span>
  );
}
```

**Before (attack applied):**

```
$ npx vitest run src/components/TimelineEventRow.test.tsx --reporter=verbose
 ✓ ... Done-when 8 ...
 ✓ ... shows the human mark for role: owner ...
 × ... shows the agent mark for role: agent, and never the human mark
   → expected element not to be in the document, found <span aria-label="Human" ...>🧑</span> instead
   (screen.getByLabelText("Agent") also failed to find any match)
 × ... shows neither real mark for an unrecognized role, rather than guessing
   → expected document not to contain element, found <span aria-label="Human" ...>🧑</span> instead

 Test Files  1 failed (1)
      Tests  2 failed | 5 passed (7)
```

Caught exactly as intended: the "agent" test and the "unknown role" test
both failed because the mark showed 🧑 regardless of the actor's real
role; the "owner" test still passed (a hardcoded-owner attack is
invisible to a test that only ever checks the owner case — which is
exactly why Done-when 9 needs the non-owner cases, not just one).

**After (reverted):**

```
$ npx vitest run src/components/TimelineEventRow.test.tsx
 Test Files  1 passed (1)
      Tests  7 passed (7)
```

Diffed the reverted file against a pre-attack backup — byte-identical.

### Attack 2 (Done-when 10 / I20) — swap the escaping-safe render path for `dangerouslySetInnerHTML` on the same body

The comment-rendering branch temporarily changed from:

```tsx
<div className="rounded-xl ...">
  <Markdown>{event.body!}</Markdown>
</div>
```

to:

```tsx
<div
  className="rounded-xl ..."
  /* ATTACK (task-7 report, Done-when 10 / I20): raw/unsafe render
     path on the same body — temporary, reverted before commit. */
  dangerouslySetInnerHTML={{ __html: event.body! }}
/>
```

**Before (attack applied):**

```
$ npx vitest run src/components/TimelineEventRow.test.tsx --reporter=verbose
 ✓ ... Done-when 8 ...
 ✓ ... Done-when 9 (all three) ...
 × ... renders a <script> tag in a comment body as literal escaped text, never as a live element
   AssertionError: expected <script></script> to be null
   - Expected: null
   + Received:
   <script>
     window.__xss = true;
   </script>
 × ... renders an <img onerror=...> tag in a comment body as literal escaped text, never as a live element
   AssertionError: expected <img src="x" …(1)></img> to be null
   - Expected: null
   + Received:
   <img
     onerror="window.__xss = true"
     src="x"
   />
 ✓ ... field_changed raw HTML in from/to preview ... (unaffected — different code path)

 Test Files  1 failed (1)
      Tests  2 failed | 5 passed (7)
```

This is the actual proof Clara asked for: under the unsafe path, a real
`<script>` element and a real `<img onerror=...>` element genuinely land
in the DOM tree (`container.querySelector` finds them), not just "the
test failed for some reason" — the failure output shows the live,
parsed elements themselves.

**After (reverted):**

```
$ npx vitest run src/components/TimelineEventRow.test.tsx
 Test Files  1 passed (1)
      Tests  7 passed (7)
```

Diffed the reverted file against a pre-attack backup — byte-identical.

## Verification

### Typecheck and build

```
$ npx tsc -b --noEmit
(no output — clean)

$ npm run build
✓ 2015 modules transformed.
dist/index.html                   1.13 kB
dist/assets/index-*.css          65.03 kB
dist/assets/index-*.js          673.58 kB
✓ built in 4.57s
```

(Build artifacts were removed from `web/dist` afterward and the tracked
`.gitkeep` restored — this task doesn't commit build output.)

### The I20-substring collision, full output

```
$ go test ./internal/... 2>&1 | tail -3
--- FAIL: TestI20_FrontendNeverUsesDangerouslySetInnerHTML
    frontend_safety_test.go:132: "...never used.** \`react-markdown\`..."
      should not contain "dangerouslySetInnerHTML"
    Messages: web/src/components/Markdown.tsx must never use
      dangerouslySetInnerHTML — I20 forbids comment bodies (or anything
      else) reaching the DOM as raw HTML
FAIL
```

Fixed by rephrasing `Markdown.tsx`'s header comment to describe the API
without spelling the identifier literally. Confirmed:

```
$ grep -rl "dangerouslySetInnerHTML" web/src --include="*.ts" --include="*.tsx" --include="*.js" --include="*.jsx"
(no matches)
```

### JS suite — full run, every test named

```
$ npx vitest run --reporter=verbose
 ✓ src/lib/todos.test.tsx > todos CRUD hooks > useCreateTodoMutation POSTs /api/bff/todos with {title, clientRequestId} and invalidates the list
 ✓ src/components/AuthGate.test.tsx > AuthGate > does not redirect to /login when a confirmed session later fails transiently
 ✓ src/lib/todos.test.tsx > todos CRUD hooks > useUpdateTodoMutation PATCHes /api/bff/todos/:id with {title, clientRequestId} and invalidates the list + the todo
 ✓ src/lib/todos.test.tsx > todos CRUD hooks > useCreateTodoEventMutation(commented) POSTs /api/bff/todos/:id/events with {type, body, clientRequestId} and invalidates the list, the todo, and its events
 ✓ src/lib/todos.test.tsx > todos CRUD hooks > useCreateTodoEventMutation(status_changed) POSTs {type, to, clientRequestId}, never {status: closed} silently accepted as anything but a wire value
 ✓ src/lib/todos.test.tsx > canCloseTodo — I18's own frontend reflection > offers closed for role: owner
 ✓ src/lib/todos.test.tsx > canCloseTodo — I18's own frontend reflection > does not offer closed for role: agent
 ✓ src/lib/todos.test.tsx > canCloseTodo — I18's own frontend reflection > does not offer closed when there is no resolved session role yet
 ✓ src/components/AuthGate.test.tsx > AuthGate > does not redirect to /login when the very first session check fails transiently
 ✓ src/components/AuthGate.test.tsx > AuthGate > redirects to /login when the very first session check resolves with a genuine no-session
 ✓ src/app/TodosList.test.tsx > TodosList > renders the mocked todo's title, and never a title absent from the mock
 ✓ src/components/TimelineEventRow.test.tsx > ... Done-when 8 ... renders byte-identical output for the same event through both pages' own adapters
 ✓ src/components/TimelineEventRow.test.tsx > ... Done-when 9 ... shows the human mark for role: owner, and never the agent mark
 ✓ src/components/TimelineEventRow.test.tsx > ... Done-when 9 ... shows the agent mark for role: agent, and never the human mark
 ✓ src/components/TimelineEventRow.test.tsx > ... Done-when 9 ... shows neither real mark for an unrecognized role, rather than guessing
 ✓ src/components/TimelineEventRow.test.tsx > ... Done-when 10 / I20 ... renders a <script> tag ... never as a live element
 ✓ src/components/TimelineEventRow.test.tsx > ... Done-when 10 / I20 ... renders an <img onerror=...> tag ... never as a live element
 ✓ src/components/TimelineEventRow.test.tsx > ... Done-when 10 / I20 ... renders raw HTML in a field_changed event's from/to preview as literal text too
 ✓ src/app/activity/ActivityList.test.tsx > ActivityList > renders the mocked feed item's actor/summary/todo title, and never content absent from the mock
 ✓ src/app/activity/ActivityList.test.tsx > ActivityList > shows the empty state when the feed has no items

 Test Files  5 passed (5)
      Tests  20 passed (20)
```

No pre-existing JS test was left broken; `AuthGate.test.tsx` (untouched by
this task) still passes unchanged.

### Go suite — full run, both `go test ./internal/...` and the Makefile's canonical gate

```
$ go test ./internal/... 2>&1 | tail
ok  	github.com/mildronize/my-template/internal
?   	github.com/mildronize/my-template/internal/api	[no test files]
?   	github.com/mildronize/my-template/internal/bffapi	[no test files]
?   	github.com/mildronize/my-template/internal/db	[no test files]
ok  	github.com/mildronize/my-template/internal/dbquery
ok  	github.com/mildronize/my-template/internal/domain/todo
ok  	github.com/mildronize/my-template/internal/identity
ok  	github.com/mildronize/my-template/internal/platform
ok  	github.com/mildronize/my-template/internal/transport/bff
ok  	github.com/mildronize/my-template/internal/transport/publicapi
```

Fully green — **not** "only `TestDoneWhen12` failing on I20" (the shape
the task brief described as expected); see the discovery section above
for why. No `.go` file was touched by this task (`git status --short`
throughout shows only `web/` paths).

```
$ make test
cd web && npm ci && npm test
...
 Test Files  5 passed (5)
      Tests  20 passed (20)
...
go test $(GO_PKGS)
ok  	.../cmd/issue-key
ok  	.../cmd/seed
ok  	.../cmd/server
ok  	.../internal
ok  	.../internal/dbquery
ok  	.../internal/domain/todo
ok  	.../internal/identity
ok  	.../internal/platform
ok  	.../internal/transport/bff
ok  	.../internal/transport/publicapi
```

The canonical two-suite gate (`make test`, `npm ci` + `npm test` + Go)
run clean from a fresh `npm ci`, not just a warm `node_modules`.

### Cold-clone verification

Pushed (`570638b`), then cloned that exact commit fresh into an isolated
scratchpad directory (not a cleaned copy of the working tree) and re-ran
both suites there:

```
$ git clone --branch milestone-4/activity-log <repo> repo
$ cd repo && git log -1 --format=%H
570638b214afc36ed14e12fc387836043df6f3a7

$ go test ./internal/... 2>&1 | tail
ok  	.../internal
ok  	.../internal/dbquery
ok  	.../internal/domain/todo
ok  	.../internal/identity
ok  	.../internal/platform
ok  	.../internal/transport/bff
ok  	.../internal/transport/publicapi

$ cd web && npm ci && npm test
 Test Files  5 passed (5)
      Tests  20 passed (20)
```

Identical result to the working-tree run above. What stayed warm: the Go
module/build caches and the npm cache (legitimate dependency caches, not
test artifacts) — the git clone, `node_modules`, and every test's own
temp-file SQLite DB were freshly created. The clone was deleted
afterward.

## Decisions

- **`ProvenanceMark` has three states, not two** (`"owner"` → 🧑,
  `"agent"` → 🤖, anything else → ❔ "Unknown provenance"). GOAL.md's
  task-7 spec only names two. Added the third deliberately, for the
  per-todo-timeline gap named above: with `role` required (matching
  my-task's own shape and `ActivityActor`'s real wire shape) but no real
  role data available on that page, the only honest choices were "guess
  a real value" (exactly the Done-when-9-style bug of defaulting to one
  answer regardless of the data — self-inflicted this time) or "show a
  visibly distinct unknown state." I chose the latter and tested it as
  its own case in Done-when 9's own describe block.
- **`todoEventToTimelineEvent` takes `actor` as an optional second
  argument rather than reading it off `TodoEvent`.** This is what makes
  Done-when 8's test able to prove genuine adapter+component sharing
  (by supplying an equivalent actor to both adapters) independently of
  the contract gap in point 1 above — the real `TodoDetailPage.tsx`
  never supplies one (no real data to supply), but the function's
  contract is honest about needing one for full fidelity.
- **`canCloseTodo` checks `sessionRole === "owner"`, always true for a
  live BFF session on this template** (`todo_handler.go:139`'s own
  comment: a BFF session can only ever resolve to `role: "owner"`, I12).
  Kept as a named function rather than inlining `true` at both call
  sites (`TodoRow.tsx`, `TodoDetailPage.tsx`) so there is exactly one
  place to change if a future milestone ever gives the BFF a
  non-owner session shape, and so it's testable in isolation
  (`todos.test.tsx`'s own `canCloseTodo` describe block).
- **The activity feed uses `useInfiniteQuery` with a "Load more" button,
  not infinite scroll.** GOAL.md's task-7 spec says "paginated," not
  which pagination UX; my-task's own home page uses a similar
  load-more pattern. A reasonable choice, not the only one the task text
  names.
- **`NewTodoDialog.tsx` stays title-only.** `useCreateTodoMutation` now
  accepts optional `assigneeId`/`priority`/`dueDate`, but the creation
  dialog doesn't expose them — `TodoDetailPage.tsx` is where those get
  set, immediately after creation, through the single write path (I15).
  Adding three more fields to a dialog nobody asked for would be scope
  creep past GOAL.md's task-7 spec, not completeness.

## What I did not establish

- **No live-server/browser walkthrough.** Everything above is
  Vitest-mocked-fetch and `tsc`/`vite build` — I did not start
  `cmd/server` and drive the SPA against a real backend in a real
  browser. `docs/GETTING-STARTED.md`'s dev-proxy setup would be the way
  to do that; not attempted here.
- **The per-todo timeline's real production behavior for provenance is
  honestly incomplete, not just under-tested.** This isn't "a test I
  didn't write" — it's a real limitation of the shipped page, named
  above and in-code, that depends on a backend contract change (adding
  actor identity to `TodoEvent`) I'm not able to make (no `.go` files
  this task).
- **Did not add a picker/lookup UI for assignee** — typed raw-id input
  only, for the same contract-gap reason.
- **Did not attempt to change `bff-openapi.yaml`'s `ApiKey`/`TodoEvent`
  schemas to add the missing identity fields.** Editing the spec without
  a matching Go handler change would make the generated TypeScript types
  promise a field the wire response never actually sends — worse than
  the gap itself. Named as Clara's decision, not mine to make
  unilaterally by editing only the doc half of a two-sided contract.
- **Did not verify มายด์'s full acceptance walkthrough end-to-end** —
  task-6's report already noted step 1 (keys) is proven at the API layer
  only, and this task doesn't add a browser-driven confirmation of any
  step either.

## Ambiguities flagged (not silently resolved)

- The three contract gaps in "Contract gaps found" above — real,
  structural, and outside this task's ability to fix (no `.go` files) or
  authority to decide how to fix (a backend schema change is Clara's
  call).
- The I20/`TestDoneWhen12` bridging question the task brief asked me to
  investigate turned out to already be answered by a concurrent commit
  (`2787432`) that landed on this branch mid-task, not by me — see the
  discovery section above for the full account, including the one real
  consequence it had for my own work (the substring collision in
  `Markdown.tsx`'s comment).
- `ProvenanceMark`'s third ("unknown") state is an addition GOAL.md's
  task-7 spec doesn't explicitly ask for (it names 🧑/🤖 only) — a
  judgment call made to keep the mark honest given gap #1 above, not
  something the contract dictated either way.

## Commits pushed (branch `milestone-4/activity-log`)

- `feat(milestone-4/task-7): SPA — per-todo detail page, activity feed, shared TimelineEventRow, updated todos list and settings`
