// TPL-1 milestone-3/task-3: one todo's row — checkbox to toggle done,
// click-to-edit title, delete behind a confirm (mirrors
// ApiKeySettings.tsx's own AlertDialog-confirmed revoke). Split out of
// TodosList.tsx so that component stays a thin "render the query result"
// component, easy to test against mocked API data (Done-when 7).
import { useState } from "react";
import { Pencil, Trash2 } from "lucide-react";

import { Button } from "~/components/ui/button";
import { Checkbox } from "~/components/ui/checkbox";
import { Input } from "~/components/ui/input";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "~/components/ui/alert-dialog";
import { useDeleteTodoMutation, useUpdateTodoMutation, type Todo } from "~/lib/todos";

export function TodoRow({ todo }: { todo: Todo }) {
  const updateTodo = useUpdateTodoMutation();
  const deleteTodo = useDeleteTodoMutation();
  const [editing, setEditing] = useState(false);
  const [draftTitle, setDraftTitle] = useState(todo.title);

  function commitEdit() {
    const trimmed = draftTitle.trim();
    setEditing(false);
    if (!trimmed || trimmed === todo.title) {
      setDraftTitle(todo.title);
      return;
    }
    updateTodo.mutate({ id: todo.id, title: trimmed });
  }

  return (
    <li className="flex items-center gap-3 border-b border-[var(--line)] py-3 last:border-0">
      <Checkbox
        checked={todo.done}
        onCheckedChange={(checked) => updateTodo.mutate({ id: todo.id, done: checked === true })}
        aria-label={todo.done ? "Mark as not done" : "Mark as done"}
      />

      {editing ? (
        <Input
          autoFocus
          value={draftTitle}
          onChange={(e) => setDraftTitle(e.target.value)}
          onBlur={commitEdit}
          onKeyDown={(e) => {
            if (e.key === "Enter") commitEdit();
            if (e.key === "Escape") {
              setDraftTitle(todo.title);
              setEditing(false);
            }
          }}
          className="h-8 flex-1"
        />
      ) : (
        <button
          type="button"
          onClick={() => setEditing(true)}
          className={[
            "flex-1 truncate text-left text-sm font-medium",
            todo.done ? "text-[var(--sea-ink-soft)] line-through" : "text-[var(--sea-ink)]",
          ].join(" ")}
        >
          {todo.title}
        </button>
      )}

      {!editing && (
        <Button
          variant="ghost"
          size="icon"
          onClick={() => setEditing(true)}
          aria-label={`Edit "${todo.title}"`}
        >
          <Pencil className="size-4" />
        </Button>
      )}

      <AlertDialog>
        <AlertDialogTrigger asChild>
          <Button variant="ghost" size="icon" aria-label={`Delete "${todo.title}"`}>
            <Trash2 className="size-4" />
          </Button>
        </AlertDialogTrigger>
        <AlertDialogContent onClick={(e) => e.stopPropagation()}>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete &quot;{todo.title}&quot;?</AlertDialogTitle>
            <AlertDialogDescription>This can&apos;t be undone.</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              onClick={() => deleteTodo.mutate(todo.id)}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </li>
  );
}
