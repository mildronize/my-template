// milestone-4/task-7: same negative-control discipline TodosList.test.tsx
// established at Done-when 7 (task-3) — renders ActivityList against
// **mocked** GET /api/bff/activity data and asserts (a) the one mocked
// event's summary/actor/todo title appear, and (b) content never present
// in the mock does not — the negative control that catches the component
// rendering from stale/global state instead of this query's own result.
//
// Also exercises this page's own real prop shape through
// `TimelineEventRow` — ActivityPage.tsx passes `todoLink`, unlike
// TodoDetailPage.tsx (see TimelineEventRow.test.tsx's own Done-when 8
// test for the shared-component proof); this test pins that the todo
// title actually renders as a link, which only ActivityList's own usage
// exercises for real.
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import type { ReactElement } from "react";

import { ActivityList } from "./ActivityList";

function renderWithClient(ui: ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("ActivityList", () => {
  const originalFetch = global.fetch;

  afterEach(() => {
    cleanup();
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("renders the mocked feed item's actor/summary/todo title, and never content absent from the mock", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === "string" ? input : input.toString();
      if (url.includes("/api/bff/activity")) {
        return new Response(
          JSON.stringify({
            items: [
              {
                id: "evt-1",
                seq: 1,
                type: "commented",
                actor: { handle: "clara-bot", role: "agent" },
                body: "looks good to me",
                createdAt: "2026-01-01T00:00:00Z",
                todo: { id: "todo-1", title: "ship the activity feed" },
              },
            ],
            nextCursor: null,
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      throw new Error(`ActivityList.test.tsx: unexpected fetch to ${url}`);
    });
    global.fetch = fetchMock as unknown as typeof fetch;

    renderWithClient(<ActivityList />);

    expect(await screen.findByText("clara-bot")).toBeInTheDocument();
    expect(screen.getByText("looks good to me")).toBeInTheDocument();
    expect(screen.getByText("ship the activity feed")).toBeInTheDocument();
    // The agent mark, since this mocked event's actor.role is "agent" —
    // not the owner mark.
    expect(screen.getByLabelText("Agent")).toBeInTheDocument();
    expect(screen.queryByLabelText("Human")).not.toBeInTheDocument();

    // Negative control.
    expect(screen.queryByText("someone else's private comment")).not.toBeInTheDocument();
    expect(screen.queryByText("a todo from a different mock")).not.toBeInTheDocument();
  });

  it("shows the empty state when the feed has no items", async () => {
    const fetchMock = vi.fn(async () => {
      return new Response(JSON.stringify({ items: [], nextCursor: null }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    });
    global.fetch = fetchMock as unknown as typeof fetch;

    renderWithClient(<ActivityList />);

    expect(await screen.findByText("No activity yet")).toBeInTheDocument();
  });
});
