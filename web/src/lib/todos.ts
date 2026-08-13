// TPL-1 milestone-3/task-3: TanStack Query hooks for the BFF's todo CRUD
// surface (GET/POST /api/bff/todos, GET/PATCH/DELETE /api/bff/todos/:id —
// _contract/API.md), replacing tRPC's own api.task.* hooks (my-task's
// src/trpc/react.tsx, dropped per GOAL.md's Decisions table). Every
// mutation invalidates todosQueryKey on success so the list refetches —
// standard TanStack Query mutation pattern, exercised directly by
// lib/todos.test.tsx (Done-when 10).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { bffFetch } from "~/lib/api/client";
import type { components } from "~/lib/api/bff-schema.gen";

export type Todo = components["schemas"]["Todo"];

export const todosQueryKey = ["bff", "todos"] as const;

/** GET /api/bff/todos — the session owner's own todos, unpaginated. */
export function useTodosQuery() {
  return useQuery({
    queryKey: todosQueryKey,
    queryFn: () =>
      bffFetch<components["schemas"]["TodoList"]>("/todos").then((r) => r.todos),
  });
}

/** POST /api/bff/todos — owner_id is always the resolved session owner. */
export function useCreateTodoMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (title: string) =>
      bffFetch<Todo>("/todos", {
        method: "POST",
        body: JSON.stringify({
          title,
        } satisfies components["schemas"]["CreateTodoRequest"]),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: todosQueryKey });
    },
  });
}

export interface UpdateTodoInput {
  id: string;
  title?: string;
  done?: boolean;
}

/** PATCH /api/bff/todos/:id — owner-scoped, 404 (never 403) on mismatch. */
export function useUpdateTodoMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...patch }: UpdateTodoInput) =>
      bffFetch<Todo>(`/todos/${id}`, {
        method: "PATCH",
        body: JSON.stringify(patch satisfies components["schemas"]["UpdateTodoRequest"]),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: todosQueryKey });
    },
  });
}

/** DELETE /api/bff/todos/:id — owner-scoped, 404 (never 403) on mismatch. */
export function useDeleteTodoMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => bffFetch<void>(`/todos/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: todosQueryKey });
    },
  });
}
