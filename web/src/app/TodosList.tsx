// TPL-1 milestone-3/task-3: renders useTodosQuery()'s result — nothing
// else. Split out of TodosPage.tsx specifically so it's independently
// testable against mocked API data: TodosList.test.tsx (Done-when 7,
// replacing milestone-2's Done-when-9 rendering check now that rendering
// moved off the Go server) mocks the fetch this component's query chain
// eventually calls, renders just this component, and asserts both that
// the one mocked todo's title appears AND that a title never present in
// the mock does not — the negative control that makes the test able to
// fail for the right reason (not rendering from stale/global state).
//
// This round: an optional `assigneeFilter` (a user id, or undefined for
// "everyone" — TodosPage.tsx's own `ALL_ASSIGNEES` sentinel never
// reaches this component, only its resolved meaning does), applied
// client-side against the already-fetched, already-unpaginated list
// (`_contract/API.md`) — GET /api/bff/todos returns every todo
// regardless, so there is no server round trip a client-side filter here
// skips.
import { ListTodo } from "lucide-react";

import { EmptyState } from "~/components/EmptyState";
import { ErrorState } from "~/components/ErrorState";
import { Spinner } from "~/components/ui/spinner";
import { useTodosQuery } from "~/lib/todos";
import { TodoRow } from "./TodoRow";

export function TodosList({ assigneeFilter }: { assigneeFilter?: string } = {}) {
  const query = useTodosQuery();

  if (query.isPending) {
    return (
      <div className="flex justify-center py-16">
        <Spinner size="lg" />
      </div>
    );
  }

  if (query.isError) {
    return <ErrorState message="Couldn't load todos." onRetry={() => void query.refetch()} />;
  }

  const todos = assigneeFilter
    ? query.data.filter((todo) => todo.assigneeId === assigneeFilter)
    : query.data;

  if (todos.length === 0) {
    return (
      <EmptyState
        icon={ListTodo}
        title={assigneeFilter ? "No todos for this assignee" : "No todos yet"}
        description={assigneeFilter ? "Try a different assignee, or Anyone." : "Create one to get started."}
      />
    );
  }

  return (
    <ul className="rounded-2xl border border-[var(--line)] bg-[var(--chip-bg)] px-4 shadow-sm">
      {todos.map((todo) => (
        <TodoRow key={todo.id} todo={todo} />
      ))}
    </ul>
  );
}
