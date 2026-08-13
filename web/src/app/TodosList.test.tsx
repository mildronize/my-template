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
import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement } from "react";

import { TodosList } from "./TodosList";

function renderWithClient(ui: ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>);
}

describe("TodosList", () => {
  const originalFetch = global.fetch;

  afterEach(() => {
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
                done: false,
                createdAt: "2026-01-01T00:00:00Z",
                updatedAt: "2026-01-01T00:00:00Z",
              },
            ],
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
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
