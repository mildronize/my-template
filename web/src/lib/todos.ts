// milestone-4/task-7: TanStack Query hooks for the BFF's todo surface,
// updated for the shared-collection shape (_contract/API.md): `done` is
// gone, replaced by `status`/`assigneeId`/`priority`/`dueDate`;
// `DELETE /api/bff/todos/:id` is gone (finishing a todo is now a
// `status_changed` event, PATCH only still renames); and the single write
// path (I15) is exposed as `POST`/`GET /todos/:id/events`.
//
// Every mutation still invalidates todosQueryKey on success so the list
// refetches — same pattern task-3 established, extended here to also
// invalidate the one todo's own detail/events queries where relevant
// (TodoDetailPage.tsx reads those).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { bffFetch } from "~/lib/api/client";
import type { components } from "~/lib/api/bff-schema.gen";
import type { TimelineEventData } from "~/components/TimelineEventRow";

export type Todo = components["schemas"]["Todo"];
export type TodoStatus = Todo["status"];
export type TodoEvent = components["schemas"]["TodoEvent"];

export const todosQueryKey = ["bff", "todos"] as const;
export const todoQueryKey = (id: string) => ["bff", "todos", id] as const;
export const todoEventsQueryKey = (id: string) => ["bff", "todos", id, "events"] as const;

/** Every status a todo can be in, in the order the status control shows them. */
export const TODO_STATUSES: TodoStatus[] = ["open", "in_progress", "done", "closed"];

/**
 * `closed` is owner-only at the API layer (I18) — the BFF accepts it
 * (`_contract/API.md`: "status: closed succeeds here — this is the
 * owner's surface"), but this template's BFF session can only ever
 * resolve to `role: "owner"` (`todo_handler.go:139`'s own comment, I12),
 * so on the SPA every signed-in session IS the owner and `closed` is
 * always offerable here. Kept as an explicit, named function rather than
 * inlining `true` at each call site: it is the SPA's own reflection of
 * I18, not the enforcement of it (the backend rejects an agent's own
 * attempt over the public API regardless of anything this function
 * returns) — GOAL.md's task-7 spec's own distinction. If a future
 * milestone gives the BFF a non-owner session shape, this is the one
 * place that has to change.
 */
export function canCloseTodo(sessionRole: string | undefined): boolean {
  return sessionRole === "owner";
}

function newClientRequestId(): string {
  return crypto.randomUUID();
}

/** GET /api/bff/todos — every todo, unpaginated (I3 no longer applies). */
export function useTodosQuery() {
  return useQuery({
    queryKey: todosQueryKey,
    queryFn: () =>
      bffFetch<components["schemas"]["TodoList"]>("/todos").then((r) => r.todos),
  });
}

/** GET /api/bff/todos/:id — one todo, for TodoDetailPage.tsx. */
export function useTodoQuery(id: string) {
  return useQuery({
    queryKey: todoQueryKey(id),
    queryFn: () => bffFetch<Todo>(`/todos/${id}`),
    enabled: !!id,
  });
}

/** GET /api/bff/todos/:id/events — this todo's own timeline, oldest-first. */
export function useTodoEventsQuery(id: string) {
  return useQuery({
    queryKey: todoEventsQueryKey(id),
    queryFn: () =>
      bffFetch<components["schemas"]["TodoEventList"]>(`/todos/${id}/events`).then(
        (r) => r.events,
      ),
    enabled: !!id,
  });
}

/**
 * Maps one wire-shaped `TodoEvent` (`GET /todos/:id/events`) onto the
 * shared row component's `TimelineEventData` — `TodoDetailPage.tsx`'s own
 * adapter, the per-todo-timeline half of Done-when 8's shared-component
 * pairing (`~/lib/activity.ts`'s `activityItemToTimelineEvent` is the
 * feed's own half).
 *
 * milestone-4 handle-exposure fix-round: `event.actor` is read directly
 * now — the wire `TodoEvent` genuinely carries `{handle, role}` on every
 * row (`bff-openapi.yaml`'s `TodoEvent.actor`, the same `ActivityActor`
 * shape `ActivityItem.actor` already had), the same JOIN
 * `ListTodoEventsByTodoID`'s own SQL query now does (task-7's report
 * found this endpoint had none — see git history on this file's own
 * previous version for the gap this closed). No more optional second
 * argument standing in for missing data; `TimelineEventRow.test.tsx`'s
 * Done-when 8 test still supplies its own event fixture directly, it just
 * no longer needs a second parameter to do it.
 */
export function todoEventToTimelineEvent(event: TodoEvent): TimelineEventData {
  return {
    id: event.id,
    seq: event.seq,
    type: event.type,
    payload: event.payload,
    body: event.body ?? null,
    createdAt: event.createdAt,
    actor: event.actor,
  };
}

/** POST /api/bff/todos — createdBy is always the resolved session owner. */
export function useCreateTodoMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: {
      title: string;
      assigneeId?: string | null;
      priority?: components["schemas"]["CreateTodoRequest"]["priority"];
      dueDate?: string | null;
    }) =>
      bffFetch<Todo>("/todos", {
        method: "POST",
        body: JSON.stringify({
          ...input,
          clientRequestId: newClientRequestId(),
        } satisfies components["schemas"]["CreateTodoRequest"]),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: todosQueryKey });
    },
  });
}

export interface UpdateTodoInput {
  id: string;
  title: string;
}

/**
 * PATCH /api/bff/todos/:id — `title` only now (milestone-4:
 * status/assigneeId/priority/dueDate all route through
 * `useCreateTodoEventMutation` instead — `_contract/API.md`'s own
 * `updateTodo` description).
 */
export function useUpdateTodoMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, title }: UpdateTodoInput) =>
      bffFetch<Todo>(`/todos/${id}`, {
        method: "PATCH",
        body: JSON.stringify({
          title,
          clientRequestId: newClientRequestId(),
        } satisfies components["schemas"]["UpdateTodoRequest"]),
      }),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: todosQueryKey });
      void queryClient.invalidateQueries({ queryKey: todoQueryKey(variables.id) });
    },
  });
}

export type CreateTodoEventInput =
  | { id: string; type: "commented"; body: string }
  | { id: string; type: "status_changed"; to: TodoStatus }
  | { id: string; type: "assigned"; to: string | null }
  | { id: string; type: "field_changed"; field: "title" | "priority" | "dueDate"; to: string | null };

/**
 * POST /api/bff/todos/:id/events — I15's single write path, exposed here
 * for every event type the BFF may client-post (`commented`,
 * `status_changed`, `assigned`, `field_changed` — `created` is rejected
 * server-side and never offered here, I16). Invalidates the todo's own
 * detail (status/assignee/priority/dueDate may have changed as this
 * event's side effect), its own timeline, and the shared list.
 */
export function useCreateTodoEventMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...rest }: CreateTodoEventInput) => {
      const body: components["schemas"]["CreateTodoEventRequest"] = {
        type: rest.type,
        clientRequestId: newClientRequestId(),
        ...(rest.type === "commented" ? { body: rest.body } : {}),
        ...(rest.type === "status_changed" ? { to: rest.to } : {}),
        ...(rest.type === "assigned" ? { to: rest.to } : {}),
        ...(rest.type === "field_changed" ? { field: rest.field, to: rest.to } : {}),
      };
      return bffFetch<TodoEvent>(`/todos/${id}/events`, {
        method: "POST",
        body: JSON.stringify(body),
      });
    },
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: todosQueryKey });
      void queryClient.invalidateQueries({ queryKey: todoQueryKey(variables.id) });
      void queryClient.invalidateQueries({ queryKey: todoEventsQueryKey(variables.id) });
    },
  });
}
