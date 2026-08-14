// New this round (มายด์'s assignee-picker ask): TodoDetailPage.tsx's
// AssigneeControl was a free-text id input until GET /api/bff/users
// landed — this is the first render test the detail page has ever had,
// and it exists specifically to prove the picker actually works end to
// end: opens the real Combobox, picks a real user by handle, and asserts
// the mutation that fires carries that user's id — not a unit test of a
// mapping function, a test of the control มายด์ himself clicks.
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router-dom";

import TodoDetailPage from "./TodoDetailPage";

function renderDetailPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/todos/todo-1"]}>
        <Routes>
          <Route path="/todos/:id" element={<TodoDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("TodoDetailPage's AssigneeControl", () => {
  const originalFetch = global.fetch;

  afterEach(() => {
    cleanup();
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("opens the real picker, shows a real user by handle, and posts that user's id on selection", async () => {
    let assignPostBody: unknown;

    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === "string" ? input : input.toString();
      if (url.endsWith("/api/bff/me")) {
        return new Response(JSON.stringify({ handle: "owner", role: "owner", active: true }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      if (url.endsWith("/api/bff/users")) {
        return new Response(
          JSON.stringify({
            users: [
              { id: "user-clara", handle: "clara", role: "agent", active: true },
              { id: "user-luna", handle: "luna", role: "agent", active: true },
            ],
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      if (url.endsWith("/api/bff/todos/todo-1/events")) {
        if (init?.method === "POST") {
          assignPostBody = JSON.parse(init.body as string);
          return new Response(
            JSON.stringify({
              id: "event-1",
              todoId: "todo-1",
              seq: 2,
              actorId: "owner-1",
              actorHandle: "owner",
              type: "assigned",
              payload: { from: null, to: { id: "user-clara", handle: "clara" } },
              body: null,
              clientRequestId: "cr-1",
              createdAt: "2026-01-01T00:00:00Z",
            }),
            { status: 201, headers: { "Content-Type": "application/json" } },
          );
        }
        return new Response(JSON.stringify({ events: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      if (url.endsWith("/api/bff/todos/todo-1")) {
        return new Response(
          JSON.stringify({
            id: "todo-1",
            title: "review the pr",
            status: "open",
            assigneeId: null,
            assigneeHandle: null,
            priority: null,
            dueDate: null,
            createdBy: "owner-1",
            createdAt: "2026-01-01T00:00:00Z",
            updatedAt: "2026-01-01T00:00:00Z",
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      throw new Error(`TodoDetailPage.test.tsx: unexpected fetch to ${url}`);
    });
    global.fetch = fetchMock as unknown as typeof fetch;

    renderDetailPage();

    const trigger = await screen.findByRole("combobox", { name: "Assignee" });
    await userEvent.click(trigger);

    // ~/components/ui/combobox.tsx's own popover content is a plain list
    // of buttons (not a Radix Select — no listbox/option ARIA roles), so
    // each option is queried as a button by its visible label text.
    expect(await screen.findByRole("button", { name: "Unassigned" })).toBeInTheDocument();
    const claraOption = await screen.findByRole("button", { name: "clara" });
    expect(claraOption).toBeInTheDocument();
    // Positive control: a handle NOT in the mocked user list must never
    // appear, so this test can't pass just because the popover rendered
    // something.
    expect(screen.queryByRole("button", { name: "not-a-real-user" })).not.toBeInTheDocument();

    await userEvent.click(claraOption);

    expect(assignPostBody).toMatchObject({ type: "assigned", to: "user-clara" });
  });
});
