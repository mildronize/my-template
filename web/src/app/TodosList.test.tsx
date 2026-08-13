// TPL-1 milestone-3/task-3, Done-when 7 (replacing milestone-2's
// Done-when-9 rendering check now that rendering moved off the Go
// server): renders TodosList against **mocked** API data — global.fetch is
// stubbed, no real backend involved — and asserts two things, not one:
// (a) the one mocked todo's title appears in the rendered output, and
// (b) a title that is NOT in the mocked response does not appear. (b) is
// the negative control: without it, this test could pass even if the
// component quietly rendered from stale or global state instead of the
// query result it actually received — the same discipline
// _rules/_contract's existing "seeds an owner session + a todo, asserts
// the seeded title appears" tests already apply at the Go layer, ported
// here to the component layer.
//
// milestone-4/task-7: the mocked todo shape updated for the shared-
// collection fields (`status`/`assigneeId`/`priority`/`dueDate` — `done`
// is gone, `_contract/API.md`) — TodoRow.tsx now reads `todo.status`
// instead of `todo.done`, so a mock still shaped like `{done: false}`
// would leave `status` `undefined` and the row's own <Select> would throw
// rather than render, which is exactly the kind of drift a stale fixture
// is supposed to be caught by, not silently tolerate.
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import type { ReactElement } from "react";

import { TodosList } from "./TodosList";

// TodoRow.tsx links to `/todos/:id` (react-router's own `Link`, new this
// task) — needs a Router context to render at all, which TodosList itself
// doesn't provide (App.tsx's own BrowserRouter is what normally supplies
// one in the real app). MemoryRouter is react-router's own test-friendly
// substitute — no real browser history, no real navigation.
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

describe("TodosList", () => {
  const originalFetch = global.fetch;

  afterEach(() => {
    cleanup();
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("renders the mocked todo's title, and never a title absent from the mock", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === "string" ? input : input.toString();
      if (url.endsWith("/api/bff/todos")) {
        return new Response(
          JSON.stringify({
            todos: [
              {
                id: "todo-1",
                title: "buy dog food for เจ้านาย",
                status: "open",
                assigneeId: null,
                priority: null,
                dueDate: null,
                createdBy: "owner-1",
                createdAt: "2026-01-01T00:00:00Z",
                updatedAt: "2026-01-01T00:00:00Z",
              },
            ],
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      // TodoRow.tsx's own StatusControl reads the signed-in session to
      // decide whether "closed" is offered (~/lib/auth-client's
      // useSession -> GET /api/bff/me) — mocked here as a plain 401 so
      // that hook resolves to "no session" rather than hanging or
      // throwing; this test isn't about the status control's own gating.
      if (url.endsWith("/api/bff/me")) {
        return new Response(null, { status: 401 });
      }
      throw new Error(`TodosList.test.tsx: unexpected fetch to ${url}`);
    });
    global.fetch = fetchMock as unknown as typeof fetch;

    renderWithClient(<TodosList />);

    expect(await screen.findByText("buy dog food for เจ้านาย")).toBeInTheDocument();

    // Negative control: a title never present in the mocked response must
    // never appear either — this is what would catch the component
    // rendering from stale/global state rather than this query's own
    // result.
    expect(screen.queryByText("someone elses private todo")).not.toBeInTheDocument();
  });
});
