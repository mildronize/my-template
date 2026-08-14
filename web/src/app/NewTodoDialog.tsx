// TPL-1 milestone-3/task-3: mirrors my-task's own create-task dialog
// (origin/main @ fdceb8befbcebfa25d2216d71264bb7c5e8c96d7,
// src/app/(app)/tasks/NewTaskDialog.tsx) — same ResponsiveDialog/form
// shape, trimmed to a todo's own fields (title only: no project, no
// description — this template's domain has none of those, per GOAL.md's
// out-of-scope note) and rewired against lib/todos.ts's
// useCreateTodoMutation instead of tRPC's api.task.create.
//
// milestone-4/task-7: `useCreateTodoMutation` also accepted optional
// assigneeId/priority/dueDate from the start (`_contract/API.md`'s own
// CreateTodoRequest), but this dialog asked for title only — GOAL.md's
// task-7 spec asked for those fields on the LIST and DETAIL pages, never
// this one, and adding three more fields nobody asked for would have
// been scope creep, not completeness.
//
// This round (มายด์'s assignee-picker ask, GET /api/bff/users landing):
// adds assignee back, and only assignee — my-task's own NewTaskDialog
// has exactly this one extra field (an assignee Combobox, `UNASSIGNED`
// default), not priority/dueDate too, so matching it doesn't reopen the
// scope question task-7 already settled for the other two fields.
import { useState, type FormEvent } from "react";
import { toast } from "sonner";

import { ResponsiveDialog } from "~/components/ui/responsive-dialog";
import { Button } from "~/components/ui/button";
import { Input } from "~/components/ui/input";
import { Spinner } from "~/components/ui/spinner";
import { Combobox } from "~/components/ui/combobox";
import { useCreateTodoMutation } from "~/lib/todos";
import { useUsersQuery, assigneeOptions, UNASSIGNED } from "~/lib/users";

export function NewTodoDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [title, setTitle] = useState("");
  const [assignee, setAssignee] = useState(UNASSIGNED);
  const createTodo = useCreateTodoMutation();
  // Deliberately the same literal my-task's own NewTaskDialog uses:
  // enabled: open — fetch the picker's data only while this dialog is
  // actually visible, not on every render of a closed dialog's tree.
  const usersQuery = useUsersQuery(open);

  function resetForm() {
    setTitle("");
    setAssignee(UNASSIGNED);
  }

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    const trimmed = title.trim();
    if (!trimmed) return;
    createTodo.mutate(
      { title: trimmed, assigneeId: assignee === UNASSIGNED ? undefined : assignee },
      {
        onSuccess: () => {
          resetForm();
          onOpenChange(false);
        },
        onError: (err) => {
          toast.error(err instanceof Error ? err.message : "Couldn't create the todo.");
        },
      },
    );
  }

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) resetForm();
        onOpenChange(next);
      }}
      title="New todo"
    >
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <div className="flex flex-col gap-1.5">
          <label htmlFor="new-todo-title" className="text-sm font-medium text-[var(--sea-ink)]">
            Title
          </label>
          <Input
            id="new-todo-title"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="What needs doing?"
            autoFocus
            required
          />
        </div>

        <div className="flex flex-col gap-1.5">
          <span className="text-sm font-medium text-[var(--sea-ink)]">Assignee</span>
          <Combobox
            value={assignee}
            options={assigneeOptions(usersQuery.data ?? [])}
            onChange={setAssignee}
            placeholder={usersQuery.isPending ? "Loading…" : "Unassigned"}
            aria-label="Assignee"
            triggerClassName="h-9 w-full justify-between font-normal"
          />
        </div>

        <Button type="submit" disabled={createTodo.isPending || !title.trim()}>
          {createTodo.isPending ? <Spinner size="sm" className="mr-2" /> : null}
          Create todo
        </Button>
      </form>
    </ResponsiveDialog>
  );
}
