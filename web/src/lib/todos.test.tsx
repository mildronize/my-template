// TPL-1 milestone-3/task-3, Done-when 10 (the todos-CRUD hook half — the
// session-check hook's own test is AuthGate.test.tsx, per _todo.md's own
// "session-check hook's test is the same test as Done-when 6's" note):
// standard TanStack Query mutation-testing pattern. For each of
// create/update/delete: asserts the mutation calls the right
// /api/bff/todos endpoint with the right HTTP method and body, and that a
// successful mutation invalidates the todos list query so it refetches.
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

import {
  todosQueryKey,
  useCreateTodoMutation,
  useDeleteTodoMutation,
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

describe("todos CRUD hooks", () => {
  const originalFetch = global.fetch;

  afterEach(() => {
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("useCreateTodoMutation POSTs /api/bff/todos with {title} and invalidates the list", async () => {
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
        expect(JSON.parse(init?.body as string)).toEqual({ title: "New todo" });
        return new Response(
          JSON.stringify({
            id: "todo-1",
            title: "New todo",
            done: false,
            createdAt: "2026-01-01T00:00:00Z",
            updatedAt: "2026-01-01T00:00:00Z",
          }),
          { status: 201, headers: { "Content-Type": "application/json" } },
        );
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
      await mutation.current.mutateAsync("New todo");
    });

    await waitFor(() => expect(mutation.current.isSuccess).toBe(true));
    await waitFor(() => expect(listFetchCount).toBe(2));
  });

  it("useUpdateTodoMutation PATCHes /api/bff/todos/:id with the partial body and invalidates the list", async () => {
    const queryClient = newQueryClient();
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === "string" ? input : input.toString();
      const method = init?.method ?? "GET";

      if (url.endsWith("/api/bff/todos/todo-1") && method === "PATCH") {
        expect(JSON.parse(init?.body as string)).toEqual({ done: true });
        return new Response(
          JSON.stringify({
            id: "todo-1",
            title: "unchanged",
            done: true,
            createdAt: "2026-01-01T00:00:00Z",
            updatedAt: "2026-01-01T00:00:01Z",
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      throw new Error(`unexpected fetch: ${method} ${url}`);
    });
    global.fetch = fetchMock as unknown as typeof fetch;

    const { result } = renderHook(() => useUpdateTodoMutation(), {
      wrapper: makeWrapper(queryClient),
    });
    await act(async () => {
      await result.current.mutateAsync({ id: "todo-1", done: true });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: todosQueryKey });
  });

  it("useDeleteTodoMutation DELETEs /api/bff/todos/:id and invalidates the list", async () => {
    const queryClient = newQueryClient();
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === "string" ? input : input.toString();
      const method = init?.method ?? "GET";

      if (url.endsWith("/api/bff/todos/todo-1") && method === "DELETE") {
        return new Response(null, { status: 204 });
      }
      throw new Error(`unexpected fetch: ${method} ${url}`);
    });
    global.fetch = fetchMock as unknown as typeof fetch;

    const { result } = renderHook(() => useDeleteTodoMutation(), {
      wrapper: makeWrapper(queryClient),
    });
    await act(async () => {
      await result.current.mutateAsync("todo-1");
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: todosQueryKey });
  });
});
