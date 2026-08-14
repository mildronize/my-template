// milestone-4: TanStack Query hook for the BFF's assignee-picker source
// (GET /api/bff/users, _contract/API.md) — มายด์'s ask: "the assignee
// form in /todos/:id [should be] a drop[down] of the assignee, not
// freeform text, see in my-task." List only, owner-session only, every
// active user of either role, ordered by handle — mirrors my-task's own
// user.ts router and lib/keys.ts's own minimal shape (one query, no
// mutations: this surface has no create/update/delete, by design).
import { useQuery } from "@tanstack/react-query";

import { bffFetch } from "~/lib/api/client";
import type { components } from "~/lib/api/bff-schema.gen";
import type { ComboboxOption } from "~/components/ui/combobox";

export type User = components["schemas"]["User"];

export const usersQueryKey = ["bff", "users"] as const;

/**
 * GET /api/bff/users — every active user, either role, ordered by
 * handle. `enabled` (default true) mirrors my-task's own
 * `NewTaskDialog.tsx` — `usersQuery = api.user.list.useQuery(undefined, {
 * enabled: open })` — so a picker inside a dialog only fetches while
 * that dialog is actually open, not on every page load that happens to
 * render the (closed) dialog's component tree.
 */
export function useUsersQuery(enabled = true) {
  return useQuery({
    queryKey: usersQueryKey,
    queryFn: () => bffFetch<components["schemas"]["UserList"]>("/users").then((r) => r.users),
    enabled,
  });
}

/**
 * The synthetic "no assignee" option value every assignee `<Combobox>` on
 * this surface uses — the literal my-task's own task detail page and
 * `NewTaskDialog` share (`UNASSIGNED = "__unassigned__"`,
 * `~/gits/my-task/src/app/(app)/tasks/[ref]/page.tsx`), kept identical
 * here so a reader who knows one codebase recognizes the other's intent
 * immediately. Radix's `<Select>`/this repo's own `<Combobox>` have no
 * real empty value, so "no assignee" needs one that isn't a real id.
 */
export const UNASSIGNED = "__unassigned__";

/**
 * Builds an assignee `<Combobox>`'s option list: "Unassigned" first, then
 * every active user labeled by handle. The option `value` is the user's
 * **id**, not their handle — a deliberate divergence from my-task's own
 * Combobox (which writes a handle, since my-task's own `assigned` event
 * `to` takes one): this template's wire contract writes an id
 * (`Todo.assigneeId`, `CreateTodoEventRequest.to` for `assigned` —
 * `_contract/API.md`), unchanged by this feature, so the picker's value
 * has to match what the write actually expects, not the display text.
 */
export function assigneeOptions(users: User[]): ComboboxOption[] {
  return [
    { value: UNASSIGNED, label: "Unassigned" },
    ...users.map((u) => ({ value: u.id, label: u.handle })),
  ];
}
