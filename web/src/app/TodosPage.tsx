// TPL-1 milestone-3/task-3: mirrors my-task's own /tasks page shape
// (origin/main @ fdceb8befbcebfa25d2216d71264bb7c5e8c96d7,
// src/app/(app)/tasks/page.tsx) — heading, a "New ..." button opening a
// create dialog, a list — trimmed to a todo's own 2-4 fields (title, done,
// createdAt, updatedAt; bff-openapi.yaml's Todo schema). Dropped entirely,
// not adapted, at the time: project/assignee comboboxes, the status-group
// filter checkboxes, cursor pagination, the archived-projects notice —
// none of it applied to a todo yet (no projects, no statuses, no
// pagination on this surface per _contract/API.md's own "unpaginated"
// note). Replaced task-1's placeholder (a bare heading with no data).
//
// This round (มายด์'s assignee-picker ask, GET /api/bff/users landing):
// the assignee filter combobox is back, matching my-task's own list page
// — its own `ALL_ASSIGNEES = "__all__"` sentinel ("Anyone"), same
// Combobox pattern the picker uses. Filtered client-side, not via a new
// query param: GET /api/bff/todos is deliberately unpaginated and
// returns every todo already (_contract/API.md), so there is nothing a
// server-side filter would save here that an array filter doesn't
// already do against data this page has in hand regardless. Status
// filtering, projects, and pagination still don't apply — this domain
// has none of those — so only the assignee filter comes back, not the
// whole dropped block.
import { useState } from "react";
import { Plus } from "lucide-react";

import { Button } from "~/components/ui/button";
import { Combobox } from "~/components/ui/combobox";
import { TodosList } from "./TodosList";
import { NewTodoDialog } from "./NewTodoDialog";
import { useUsersQuery } from "~/lib/users";

/** "Any assignee" — the filter's own sentinel, distinct from the picker's UNASSIGNED (~/lib/users.ts): a real, different meaning ("don't filter" vs "no assignee"), so it gets its own value rather than reusing that one. */
const ALL_ASSIGNEES = "__all__";

export default function TodosPage() {
  const [newTodoOpen, setNewTodoOpen] = useState(false);
  const [assigneeFilter, setAssigneeFilter] = useState(ALL_ASSIGNEES);
  const usersQuery = useUsersQuery();

  const filterOptions = [
    { value: ALL_ASSIGNEES, label: "Anyone" },
    ...(usersQuery.data ?? []).map((u) => ({ value: u.id, label: u.handle })),
  ];

  return (
    <main className="mx-auto max-w-2xl px-4 py-8">
      <div className="mb-6 flex items-center justify-between gap-3">
        <h1 className="text-2xl font-bold text-[var(--sea-ink)]">Todos</h1>
        <Button size="sm" onClick={() => setNewTodoOpen(true)}>
          <Plus className="size-4" />
          New todo
        </Button>
      </div>

      <div className="mb-4 flex items-center gap-2">
        <span className="text-xs text-[var(--sea-ink-soft)]">Assignee</span>
        <Combobox
          value={assigneeFilter}
          options={filterOptions}
          onChange={setAssigneeFilter}
          placeholder="Anyone"
          aria-label="Filter by assignee"
          triggerClassName="h-8 w-48 justify-between font-normal text-sm"
        />
      </div>

      <TodosList assigneeFilter={assigneeFilter === ALL_ASSIGNEES ? undefined : assigneeFilter} />

      <NewTodoDialog open={newTodoOpen} onOpenChange={setNewTodoOpen} />
    </main>
  );
}
