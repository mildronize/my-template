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
      actor: sharedActor,
      clientRequestId: "cr-1",
    };
    const activityItem: ActivityItem = {
      ...sharedFields,
      actor: sharedActor,
      todo: { id: "todo-9", title: "ship the thing" },
    };

    // TodoDetailPage.tsx's own real per-todo-timeline path: TodoEvent now
    // carries its own actor directly (milestone-4 handle-exposure
    // fix-round — GET /api/bff/todos/:id/events's own query now joins
    // users the same way the feed's query already did), so this adapter
    // no longer takes a second argument at all — proving the
    // adapter+component pairing renders identically to the feed's own
    // path from equivalent real wire data, not from data plugged in by
    // hand to paper over a gap.
    const fromTodoTimeline: TimelineEventData = todoEventToTimelineEvent(todoEvent);
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
