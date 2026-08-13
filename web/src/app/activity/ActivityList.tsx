// milestone-4/task-7: renders useActivityFeedQuery()'s result — nothing
// else, same "thin, independently-testable render component" split
// TodosList.tsx established (that file's own header). ActivityList.test.tsx
// mocks the fetch this component's query chain eventually calls, the same
// negative-control discipline TodosList.test.tsx already established
// (Done-when 7's own pattern, reused here rather than reinvented).
import { Activity } from "lucide-react";

import { EmptyState } from "~/components/EmptyState";
import { ErrorState } from "~/components/ErrorState";
import { Spinner } from "~/components/ui/spinner";
import { Button } from "~/components/ui/button";
import { TimelineEventRow } from "~/components/TimelineEventRow";
import { useActivityFeedQuery, activityItemToTimelineEvent } from "~/lib/activity";

export function ActivityList() {
  const query = useActivityFeedQuery();

  if (query.isPending) {
    return (
      <div className="flex justify-center py-16">
        <Spinner size="lg" />
      </div>
    );
  }

  if (query.isError) {
    return <ErrorState message="Couldn't load activity." onRetry={() => void query.refetch()} />;
  }

  const items = query.data.pages.flatMap((page) => page.items);

  if (items.length === 0) {
    return (
      <EmptyState
        icon={Activity}
        title="No activity yet"
        description="Events show up here as todos are created, commented on, and changed."
      />
    );
  }

  return (
    <>
      <ul className="rounded-2xl border border-[var(--line)] bg-[var(--chip-bg)] px-4 shadow-sm">
        {items.map((item) => (
          <TimelineEventRow
            key={item.id}
            event={activityItemToTimelineEvent(item)}
            todoLink={{ id: item.todo.id, title: item.todo.title }}
          />
        ))}
      </ul>

      {query.hasNextPage && (
        <div className="mt-4 flex justify-center">
          <Button
            variant="outline"
            size="sm"
            onClick={() => void query.fetchNextPage()}
            disabled={query.isFetchingNextPage}
          >
            {query.isFetchingNextPage ? <Spinner size="sm" className="mr-2" /> : null}
            Load more
          </Button>
        </div>
      )}
    </>
  );
}
