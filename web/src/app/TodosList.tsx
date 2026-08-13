// TPL-1 milestone-3/task-3: renders useTodosQuery()'s result — nothing
// else. Split out of TodosPage.tsx specifically so it's independently
// testable against mocked API data: TodosList.test.tsx (Done-when 7,
// replacing milestone-2's Done-when-9 rendering check now that rendering
// moved off the Go server) mocks the fetch this component's query chain
// eventually calls, renders just this component, and asserts both that
// the one mocked todo's title appears AND that a title never present in
// the mock does not — the negative control that makes the test able to
// fail for the right reason (not rendering from stale/global state).
import { ListTodo } from "lucide-react";

import { EmptyState } from "~/components/EmptyState";
import { ErrorState } from "~/components/ErrorState";
import { Spinner } from "~/components/ui/spinner";
import { useTodosQuery } from "~/lib/todos";
import { TodoRow } from "./TodoRow";

export function TodosList() {
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

  if (query.data.length === 0) {
    return (
      <EmptyState icon={ListTodo} title="No todos yet" description="Create one to get started." />
    );
  }

  return (
    <ul className="rounded-2xl border border-[var(--line)] bg-[var(--chip-bg)] px-4 shadow-sm">
      {query.data.map((todo) => (
        <TodoRow key={todo.id} todo={todo} />
      ))}
    </ul>
  );
}
