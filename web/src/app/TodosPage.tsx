// TPL-1 milestone-3/task-3: mirrors my-task's own /tasks page shape
// (origin/main @ fdceb8befbcebfa25d2216d71264bb7c5e8c96d7,
// src/app/(app)/tasks/page.tsx) — heading, a "New ..." button opening a
// create dialog, a list — trimmed to a todo's own 2-4 fields (title, done,
// createdAt, updatedAt; bff-openapi.yaml's Todo schema). Dropped entirely,
// not adapted: project/assignee comboboxes, the status-group filter
// checkboxes, cursor pagination, the archived-projects notice — none of
// it applies to a todo (no projects, no statuses, no pagination on this
// surface per _contract/API.md's own "unpaginated" note). Replaces
// task-1's placeholder (a bare heading with no data).
import { useState } from "react";
import { Plus } from "lucide-react";

import { Button } from "~/components/ui/button";
import { TodosList } from "./TodosList";
import { NewTodoDialog } from "./NewTodoDialog";

export default function TodosPage() {
  const [newTodoOpen, setNewTodoOpen] = useState(false);

  return (
    <main className="mx-auto max-w-2xl px-4 py-8">
      <div className="mb-6 flex items-center justify-between gap-3">
        <h1 className="text-2xl font-bold text-[var(--sea-ink)]">Todos</h1>
        <Button size="sm" onClick={() => setNewTodoOpen(true)}>
          <Plus className="size-4" />
          New todo
        </Button>
      </div>

      <TodosList />

      <NewTodoDialog open={newTodoOpen} onOpenChange={setNewTodoOpen} />
    </main>
  );
}
