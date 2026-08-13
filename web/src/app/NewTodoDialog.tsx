// TPL-1 milestone-3/task-3: mirrors my-task's own create-task dialog
// (origin/main @ fdceb8befbcebfa25d2216d71264bb7c5e8c96d7,
// src/app/(app)/tasks/NewTaskDialog.tsx) — same ResponsiveDialog/form
// shape, trimmed to a todo's own fields (title only: no project, no
// assignee, no description — this template's domain has none of those,
// per GOAL.md's out-of-scope note) and rewired against
// lib/todos.ts's useCreateTodoMutation instead of tRPC's api.task.create.
import { useState, type FormEvent } from "react";
import { toast } from "sonner";

import { ResponsiveDialog } from "~/components/ui/responsive-dialog";
import { Button } from "~/components/ui/button";
import { Input } from "~/components/ui/input";
import { Spinner } from "~/components/ui/spinner";
import { useCreateTodoMutation } from "~/lib/todos";

export function NewTodoDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [title, setTitle] = useState("");
  const createTodo = useCreateTodoMutation();

  function resetForm() {
    setTitle("");
  }

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    const trimmed = title.trim();
    if (!trimmed) return;
    createTodo.mutate(trimmed, {
      onSuccess: () => {
        resetForm();
        onOpenChange(false);
      },
      onError: (err) => {
        toast.error(err instanceof Error ? err.message : "Couldn't create the todo.");
      },
    });
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

        <Button type="submit" disabled={createTodo.isPending || !title.trim()}>
          {createTodo.isPending ? <Spinner size="sm" className="mr-2" /> : null}
          Create todo
        </Button>
      </form>
    </ResponsiveDialog>
  );
}
