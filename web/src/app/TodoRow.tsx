// TPL-1 milestone-3/task-3, rewritten milestone-4/task-7: one todo's row
// in the shared-collection list. milestone-4 replaces the done-checkbox/
// inline-edit/delete-behind-confirm shape entirely: `done` is gone
// (replaced by `status`), and `DELETE /api/bff/todos/:id` no longer
// exists (`_contract/API.md`) — this row is now a status/priority/
// assignee/due-date summary that links to TodoDetailPage.tsx for the
// full timeline and every mutating control (GOAL.md's task-7 spec, item
// 2: "Todos list page updated for the new fields ... including a
// status-change control that surfaces closed as an option only for the
// owner"). Title stays inline-editable here (still a PATCH, still the
// only field this list itself writes) — everything else routes through
// the detail page's own event-posting controls.
import { useState } from "react";
import { Link } from "react-router-dom";
import { Pencil } from "lucide-react";

import { Badge } from "~/components/ui/badge";
import { Button } from "~/components/ui/button";
import { Input } from "~/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "~/components/ui/select";
import { useSession } from "~/lib/auth-client";
import {
  useCreateTodoEventMutation,
  useUpdateTodoMutation,
  canCloseTodo,
  TODO_STATUSES,
  type Todo,
  type TodoStatus,
} from "~/lib/todos";

/**
 * The status-change control this row owns directly (rather than only on
 * TodoDetailPage.tsx) — GOAL.md's task-7 spec names this explicitly as a
 * list-page requirement, not just a detail-page one. `closed` is filtered
 * out of the options unless the signed-in session may close a todo
 * (`~/lib/todos.ts`'s `canCloseTodo` — the frontend's own reflection of
 * I18, not its enforcement: the backend rejects an agent's own attempt
 * regardless of what this control offers).
 */
function StatusControl({ todo }: { todo: Todo }) {
  const { data: session } = useSession();
  const createEvent = useCreateTodoEventMutation();
  const offeredStatuses = canCloseTodo(session?.user.email)
    ? TODO_STATUSES
    : TODO_STATUSES.filter((s) => s !== "closed");

  return (
    <Select
      value={todo.status}
      onValueChange={(next) =>
        createEvent.mutate({ id: todo.id, type: "status_changed", to: next as TodoStatus })
      }
      disabled={createEvent.isPending}
    >
      <SelectTrigger className="h-7 w-32 text-xs" aria-label={`Status for "${todo.title}"`}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {offeredStatuses.map((s) => (
          <SelectItem key={s} value={s}>
            {s}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

export function TodoRow({ todo }: { todo: Todo }) {
  const updateTodo = useUpdateTodoMutation();
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
    <li className="flex flex-wrap items-center gap-3 border-b border-[var(--line)] py-3 last:border-0">
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
        <Link
          to={`/todos/${todo.id}`}
          className="min-w-0 flex-1 truncate text-sm font-medium text-[var(--sea-ink)] hover:underline"
        >
          {todo.title}
        </Link>
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

      <StatusControl todo={todo} />

      {todo.priority && (
        <Badge variant="outline" className="capitalize">
          {todo.priority}
        </Badge>
      )}

      {todo.dueDate && (
        <span className="text-xs text-[var(--sea-ink-soft)]">
          due {new Date(todo.dueDate).toLocaleDateString()}
        </span>
      )}

      {/* milestone-4 handle-exposure fix-round: the handle is the
          display text now, mirroring my-task's own task list (`t.assignee`,
          a plain handle) — the raw id is still available (`title`, a
          hover tooltip) rather than dropped, since it is still what a
          caller writes back. assigneeHandle is only ever absent when
          assigneeId itself is null (Todo's own doc comment). */}
      {todo.assigneeId && (
        <span className="truncate text-xs text-[var(--sea-ink-soft)]" title={todo.assigneeId}>
          → {todo.assigneeHandle ?? todo.assigneeId}
        </span>
      )}
    </li>
  );
}
