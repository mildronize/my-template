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
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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

  // milestone-4 handle-exposure fix-round: TodoRow.tsx now prefers
  // assigneeHandle over the raw assigneeId for display (director's
  // finding: "shipping raw ids where the named source shows names").
  // Real rendered output, not just a type-level check that the field
  // exists — a raw assigneeId with no assigneeHandle set at all would
  // have passed the old code just as easily.
  it("shows the assignee's handle on the row, not the bare id, when the mock carries one", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === "string" ? input : input.toString();
      if (url.endsWith("/api/bff/todos")) {
        return new Response(
          JSON.stringify({
            todos: [
              {
                id: "todo-2",
                title: "review the pr",
                status: "open",
                assigneeId: "user-abc123",
                assigneeHandle: "clara",
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
      if (url.endsWith("/api/bff/me")) {
        return new Response(null, { status: 401 });
      }
      throw new Error(`TodosList.test.tsx: unexpected fetch to ${url}`);
    });
    global.fetch = fetchMock as unknown as typeof fetch;

    renderWithClient(<TodosList />);

    expect(await screen.findByText("→ clara")).toBeInTheDocument();
    expect(screen.queryByText("→ user-abc123")).not.toBeInTheDocument();
  });
});

// Found by Clara reading มายด์'s own walkthrough data: he made five
// owner status-changes and never once had "closed" as an option, because
// both call sites (this file's TodoRow.tsx, and TodoDetailPage.tsx) pass
// `session?.user.email` into `canCloseTodo`, which compares against the
// literal "owner" — the exact predicate is correct in isolation
// (`todos.test.tsx`'s own unit tests prove that), but no test before this
// one ever rendered the control the way มายด์ actually uses it. This is
// the same "green for a reason unrelated to what it guards" shape as
// `keys_handler_test.go` earlier this engagement: a unit test of a pure
// function proves the function, not the wiring that feeds it.
//
// Opens the real Radix <Select> (userEvent.click on its trigger, not a
// synthetic change event) and reads its real option list — a test that
// only checked `todo.status`'s current value would never notice a
// missing option, since the trigger's own displayed value never mentions
// what ISN'T offered.
describe("TodoRow's StatusControl — closed offered only for an owner session", () => {
  const originalFetch = global.fetch;

  afterEach(() => {
    cleanup();
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  function mockFetchWithSession(meResponse: Response) {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === "string" ? input : input.toString();
      if (url.endsWith("/api/bff/todos")) {
        return new Response(
          JSON.stringify({
            todos: [
              {
                id: "todo-1",
                title: "a todo",
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
      if (url.endsWith("/api/bff/me")) {
        return meResponse.clone();
      }
      throw new Error(`TodosList.test.tsx: unexpected fetch to ${url}`);
    });
    global.fetch = fetchMock as unknown as typeof fetch;
  }

  it("offers closed as a real, selectable option for a real owner session", async () => {
    mockFetchWithSession(
      new Response(JSON.stringify({ handle: "owner", role: "owner", active: true }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    renderWithClient(<TodosList />);
    const trigger = await screen.findByRole("combobox", { name: 'Status for "a todo"' });
    await userEvent.click(trigger);

    const listbox = await screen.findByRole("listbox");
    expect(within(listbox).getByRole("option", { name: "closed" })).toBeInTheDocument();
  });

  it("does not offer closed for a non-owner (agent) session", async () => {
    mockFetchWithSession(
      new Response(JSON.stringify({ handle: "some-agent", role: "agent", active: true }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    renderWithClient(<TodosList />);
    const trigger = await screen.findByRole("combobox", { name: 'Status for "a todo"' });
    await userEvent.click(trigger);

    const listbox = await screen.findByRole("listbox");
    expect(within(listbox).queryByRole("option", { name: "closed" })).not.toBeInTheDocument();
    // Positive control on the same render: some other status must still
    // be there, so an empty/broken listbox can't pass this test by
    // accident — the assertion above only means something because this
    // one also holds.
    expect(within(listbox).getByRole("option", { name: "open" })).toBeInTheDocument();
  });

  it("does not offer closed with no session at all (the 401 case)", async () => {
    mockFetchWithSession(new Response(null, { status: 401 }));

    renderWithClient(<TodosList />);
    const trigger = await screen.findByRole("combobox", { name: 'Status for "a todo"' });
    await userEvent.click(trigger);

    const listbox = await screen.findByRole("listbox");
    expect(within(listbox).queryByRole("option", { name: "closed" })).not.toBeInTheDocument();
  });
});
