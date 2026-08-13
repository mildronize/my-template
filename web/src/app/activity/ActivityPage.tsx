// milestone-4/task-7: mirrors my-task's own `/` home page
// (~/gits/my-task/src/app/(app)/page.tsx) — cross-todo, paginated,
// newest-first activity feed. Trimmed to what this template actually has
// (no project filter, no status-group filter — those are my-task-only
// domain concepts, GOAL.md's own out-of-scope note) the same way
// TodosPage.tsx was trimmed from my-task's own `/tasks` page at task-3.
//
// Lives at "/activity" rather than "/" — unlike my-task, this template's
// "/" is already TodosPage (task-1/3's own routing decision, unchanged by
// this task); GOAL.md's plan never asked for that to move.
import { Activity } from "lucide-react";
import { ActivityList } from "./ActivityList";

export default function ActivityPage() {
  return (
    <main className="mx-auto max-w-2xl px-4 py-8">
      <div className="mb-6 flex items-center gap-3">
        <Activity className="size-6 text-[var(--sea-ink)]" />
        <h1 className="text-2xl font-bold text-[var(--sea-ink)]">Activity</h1>
      </div>

      <ActivityList />
    </main>
  );
}
