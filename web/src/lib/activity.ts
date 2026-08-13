// milestone-4/task-7: TanStack Query hook for GET /api/bff/activity — the
// cross-todo activity feed (`_contract/API.md`), mirrors my-task's own
// home page (`~/gits/my-task/src/server/api/routers/activity.ts`'s
// `activity.list`, adapted from tRPC's `{limit, cursor}` input to this
// surface's flat query-string convention — `bff-openapi.yaml`'s own
// `listActivity` operation).
//
// `useInfiniteQuery`, not `useQuery` — the feed is paginated
// (`limit`/`cursorCreatedAtMs`/`cursorId`, `nextCursor: null` when
// exhausted), and ActivityPage.tsx needs a "load more" affordance, the
// same newest-first/cursor shape my-task's own `/` page paginates through.
import { useInfiniteQuery } from "@tanstack/react-query";

import { bffFetch } from "~/lib/api/client";
import type { components } from "~/lib/api/bff-schema.gen";
import type { TimelineEventData } from "~/components/TimelineEventRow";

export type ActivityItem = components["schemas"]["ActivityItem"];
export type ActivityCursor = components["schemas"]["ActivityCursor"];

export const activityQueryKey = ["bff", "activity"] as const;

function cursorToQuery(cursor: ActivityCursor | null): string {
  if (!cursor) return "";
  const params = new URLSearchParams({
    cursorCreatedAtMs: String(cursor.createdAtMs),
    cursorId: cursor.id,
  });
  return `?${params.toString()}`;
}

/**
 * GET /api/bff/activity, newest-first, paginated. Each page's
 * `nextCursor` (or `null` once exhausted, `_contract/API.md`) becomes the
 * next page's own query string, verbatim — the response's
 * `createdAtMs`/`id` field names round-trip directly into the request's
 * `cursorCreatedAtMs`/`cursorId` (`bff-openapi.yaml`'s own note on this).
 */
export function useActivityFeedQuery() {
  return useInfiniteQuery({
    queryKey: activityQueryKey,
    queryFn: ({ pageParam }: { pageParam: ActivityCursor | null }) =>
      bffFetch<components["schemas"]["ActivityFeed"]>(`/activity${cursorToQuery(pageParam)}`),
    initialPageParam: null as ActivityCursor | null,
    getNextPageParam: (lastPage) => lastPage.nextCursor ?? null,
  });
}

/**
 * Maps one wire-shaped `ActivityItem` (`GET /activity`) onto the shared
 * row component's `TimelineEventData` — `ActivityPage.tsx`'s own half of
 * Done-when 8's shared-component pairing (`~/lib/todos.ts`'s
 * `todoEventToTimelineEvent` is the per-todo-timeline's own half).
 * `ActivityItem.actor` is always present on this endpoint's own real wire
 * shape (`internal/transport/bff/todo_handler.go`'s `ActorHandle`/
 * `ActorRole` join) — unlike `TodoEvent`, so this mapping never needs a
 * fallback the way `todoEventToTimelineEvent`'s does.
 */
export function activityItemToTimelineEvent(item: ActivityItem): TimelineEventData {
  return {
    id: item.id,
    seq: item.seq,
    type: item.type,
    payload: item.payload,
    body: item.body ?? null,
    createdAt: item.createdAt,
    actor: { handle: item.actor.handle, role: item.actor.role },
  };
}
