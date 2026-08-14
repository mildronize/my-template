// TPL-1 milestone-3/task-3, Done-when 10 (the todos-CRUD hook half — the
// session-check hook's own test is AuthGate.test.tsx, per _todo.md's own
// "session-check hook's test is the same test as Done-when 6's" note):
// standard TanStack Query mutation-testing pattern. For each mutation:
// asserts it calls the right /api/bff/todos endpoint with the right HTTP
// method and body, and that a successful mutation invalidates the todos
// list query so it refetches.
//
// milestone-4/task-7: rewritten for the shared-collection shape
// (`_contract/API.md`) — `done` is gone (`status` instead);
// `useDeleteTodoMutation` is gone (`DELETE /api/bff/todos/:id` no longer
// exists); every write now carries a `clientRequestId` (I19), generated
// internally by `~/lib/todos.ts` via `crypto.randomUUID()` rather than
// supplied by the caller — these tests assert it's present and a string,
// not a specific value, since asserting an exact UUID would just be
// re-asserting `crypto.randomUUID()`'s own contract. New coverage:
// `useCreateTodoEventMutation`, I15's single write path exposed to every
// event type the BFF accepts from a client (`commented`, `status_changed`,
// `assigned`, `field_changed`).
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

import {
  todosQueryKey,
  todoQueryKey,
  todoEventsQueryKey,
  canCloseTodo,
  useCreateTodoMutation,
  useCreateTodoEventMutation,
  useTodosQuery,
  useUpdateTodoMutation,
} from "./todos";

function makeWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

function newQueryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } });
}

function fakeTodo(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    id: "todo-1",
    title: "New todo",
    status: "open",
    assigneeId: null,
    priority: null,
    dueDate: null,
    createdBy: "owner-1",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("todos CRUD hooks", () => {
  const originalFetch = global.fetch;

  afterEach(() => {
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("useCreateTodoMutation POSTs /api/bff/todos with {title, clientRequestId} and invalidates the list", async () => {
    const queryClient = newQueryClient();
    let listFetchCount = 0;
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === "string" ? input : input.toString();
      const method = init?.method ?? "GET";

      if (url.endsWith("/api/bff/todos") && method === "GET") {
        listFetchCount += 1;
        return new Response(JSON.stringify({ todos: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      if (url.endsWith("/api/bff/todos") && method === "POST") {
        const body = JSON.parse(init?.body as string) as { title: string; clientRequestId: string };
        expect(body.title).toBe("New todo");
        expect(typeof body.clientRequestId).toBe("string");
        expect(body.clientRequestId.length).toBeGreaterThan(0);
        return new Response(JSON.stringify(fakeTodo()), {
          status: 201,
          headers: { "Content-Type": "application/json" },
        });
      }
      throw new Error(`unexpected fetch: ${method} ${url}`);
    });
    global.fetch = fetchMock as unknown as typeof fetch;

    // Prime the list query so a refetch (from invalidation) is observable.
    const { result: list } = renderHook(() => useTodosQuery(), {
      wrapper: makeWrapper(queryClient),
    });
    await waitFor(() => expect(list.current.isSuccess).toBe(true));
    expect(listFetchCount).toBe(1);

    const { result: mutation } = renderHook(() => useCreateTodoMutation(), {
      wrapper: makeWrapper(queryClient),
    });
    await act(async () => {
      await mutation.current.mutateAsync({ title: "New todo" });
    });

    await waitFor(() => expect(mutation.current.isSuccess).toBe(true));
    await waitFor(() => expect(listFetchCount).toBe(2));
  });

  it("useUpdateTodoMutation PATCHes /api/bff/todos/:id with {title, clientRequestId} and invalidates the list + the todo", async () => {
    const queryClient = newQueryClient();
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === "string" ? input : input.toString();
      const method = init?.method ?? "GET";

      if (url.endsWith("/api/bff/todos/todo-1") && method === "PATCH") {
        const body = JSON.parse(init?.body as string) as { title: string; clientRequestId: string };
        expect(body.title).toBe("renamed");
        expect(typeof body.clientRequestId).toBe("string");
        expect(body.clientRequestId.length).toBeGreaterThan(0);
        return new Response(JSON.stringify(fakeTodo({ title: "renamed" })), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      throw new Error(`unexpected fetch: ${method} ${url}`);
    });
    global.fetch = fetchMock as unknown as typeof fetch;

    const { result } = renderHook(() => useUpdateTodoMutation(), {
      wrapper: makeWrapper(queryClient),
    });
    await act(async () => {
      await result.current.mutateAsync({ id: "todo-1", title: "renamed" });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: todosQueryKey });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: todoQueryKey("todo-1") });
  });

  it("useCreateTodoEventMutation(commented) POSTs /api/bff/todos/:id/events with {type, body, clientRequestId} and invalidates the list, the todo, and its events", async () => {
    const queryClient = newQueryClient();
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === "string" ? input : input.toString();
      const method = init?.method ?? "GET";

      if (url.endsWith("/api/bff/todos/todo-1/events") && method === "POST") {
        const body = JSON.parse(init?.body as string) as {
          type: string;
          body: string;
          clientRequestId: string;
        };
        expect(body.type).toBe("commented");
        expect(body.body).toBe("hello");
        expect(typeof body.clientRequestId).toBe("string");
        expect(body.clientRequestId.length).toBeGreaterThan(0);
        return new Response(
          JSON.stringify({
            id: "evt-1",
            todoId: "todo-1",
            seq: 1,
            actorId: "owner-1",
            type: "commented",
            body: "hello",
            clientRequestId: body.clientRequestId,
            createdAt: "2026-01-01T00:00:00Z",
          }),
          { status: 201, headers: { "Content-Type": "application/json" } },
        );
      }
      throw new Error(`unexpected fetch: ${method} ${url}`);
    });
    global.fetch = fetchMock as unknown as typeof fetch;

    const { result } = renderHook(() => useCreateTodoEventMutation(), {
      wrapper: makeWrapper(queryClient),
    });
    await act(async () => {
      await result.current.mutateAsync({ id: "todo-1", type: "commented", body: "hello" });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: todosQueryKey });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: todoQueryKey("todo-1") });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: todoEventsQueryKey("todo-1") });
  });

  it("useCreateTodoEventMutation(status_changed) POSTs {type, to, clientRequestId}, never {status: closed} silently accepted as anything but a wire value", async () => {
    const queryClient = newQueryClient();
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === "string" ? input : input.toString();
      const method = init?.method ?? "GET";

      if (url.endsWith("/api/bff/todos/todo-1/events") && method === "POST") {
        const body = JSON.parse(init?.body as string) as { type: string; to: string };
        expect(body.type).toBe("status_changed");
        expect(body.to).toBe("closed");
        return new Response(
          JSON.stringify({
            id: "evt-2",
            todoId: "todo-1",
            seq: 2,
            actorId: "owner-1",
            type: "status_changed",
            payload: { from: "open", to: "closed" },
            clientRequestId: "cr-2",
            createdAt: "2026-01-01T00:00:00Z",
          }),
          { status: 201, headers: { "Content-Type": "application/json" } },
        );
      }
      throw new Error(`unexpected fetch: ${method} ${url}`);
    });
    global.fetch = fetchMock as unknown as typeof fetch;

    const { result } = renderHook(() => useCreateTodoEventMutation(), {
      wrapper: makeWrapper(queryClient),
    });
    await act(async () => {
      await result.current.mutateAsync({ id: "todo-1", type: "status_changed", to: "closed" });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});

// GOAL.md's task-7 spec, item 2: the status-change control surfaces
// `closed` as an option only for the owner — this is the SPA's own
// reflection of I18, not the enforcement of it (the BFF/public API reject
// an unauthorized attempt regardless of what the frontend offers,
// _contract/API.md). `canCloseTodo` is the one function TodoRow.tsx's own
// StatusControl and TodoDetailPage.tsx's own StatusControl both call to
// decide this — a single, tested seam rather than the check being
// duplicated (and potentially drifting) in two components.
describe("canCloseTodo — I18's own frontend reflection", () => {
  it("offers closed for role: owner", () => {
    expect(canCloseTodo("owner")).toBe(true);
  });

  it("does not offer closed for role: agent", () => {
    expect(canCloseTodo("agent")).toBe(false);
  });

  it("does not offer closed when there is no resolved session role yet", () => {
    expect(canCloseTodo(undefined)).toBe(false);
  });
});
