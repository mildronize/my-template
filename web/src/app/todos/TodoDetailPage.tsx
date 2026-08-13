// milestone-4/task-7: per-todo detail page (GOAL.md's own task-7 spec,
// item 1) — one todo's own fields plus its own timeline, rendered through
// the SAME `TimelineEventRow` `ActivityPage.tsx` uses (Done-when 5/8).
// Route is "/todos/:id" (App.tsx) — this repo's Vite/react-router
// equivalent of a Next.js `app/todos/[id]/page.tsx` route (this repo has
// no App Router, so there's no literal `[id]` folder — TodosPage.tsx's own
// existing convention, one page component per route, is what this mirrors,
// not Next.js's file-routing convention itself).
//
// The status control's own "closed" gating is the frontend's own
// reflection of I18, not its enforcement — `~/lib/todos.ts`'s
// `canCloseTodo` doc comment has the full reasoning; the short version is
// this page just doesn't offer a control that would always come back
// `unauthorized` on this template's BFF session shape (every BFF session
// is `role: "owner"`, I12).
import { useState, type FormEvent } from "react";
import { useParams, Link } from "react-router-dom";
import { ArrowLeft } from "lucide-react";

import { Button } from "~/components/ui/button";
import { Input } from "~/components/ui/input";
import { Textarea } from "~/components/ui/textarea";
import { Spinner } from "~/components/ui/spinner";
import { Badge } from "~/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "~/components/ui/select";
import { ErrorState } from "~/components/ErrorState";
import { TimelineEventRow } from "~/components/TimelineEventRow";
import { useSession } from "~/lib/auth-client";
import {
  useTodoQuery,
  useTodoEventsQuery,
  useCreateTodoEventMutation,
  todoEventToTimelineEvent,
  canCloseTodo,
  TODO_STATUSES,
  type TodoStatus,
} from "~/lib/todos";

const PRIORITIES = ["low", "medium", "high", "urgent"] as const;

function StatusControl({ todoId, status }: { todoId: string; status: TodoStatus }) {
  const { data: session } = useSession();
  const createEvent = useCreateTodoEventMutation();
  // I18's own frontend reflection — see this file's header comment.
  const offeredStatuses = canCloseTodo(session?.user.email)
    ? TODO_STATUSES
    : TODO_STATUSES.filter((s) => s !== "closed");

  return (
    <Select
      value={status}
      onValueChange={(next) =>
        createEvent.mutate({ id: todoId, type: "status_changed", to: next as TodoStatus })
      }
      disabled={createEvent.isPending}
    >
      <SelectTrigger className="w-40" aria-label="Status">
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

function PriorityControl({ todoId, priority }: { todoId: string; priority: string | null }) {
  const createEvent = useCreateTodoEventMutation();
  return (
    <Select
      value={priority ?? "none"}
      onValueChange={(next) =>
        createEvent.mutate({
          id: todoId,
          type: "field_changed",
          field: "priority",
          to: next === "none" ? null : next,
        })
      }
      disabled={createEvent.isPending}
    >
      <SelectTrigger className="w-36" aria-label="Priority">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="none">no priority</SelectItem>
        {PRIORITIES.map((p) => (
          <SelectItem key={p} value={p}>
            {p}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

/**
 * Free-text assignee-id field, not a picker — this template's contract
 * has no endpoint that lists users/agents by handle (`Todo.assigneeId` is
 * a bare user id on the wire, same gap as `TodoEvent.actorId` —
 * `TimelineEventRow.tsx`'s own `TimelineEventData` doc comment names the
 * general shape of this across the milestone). A real picker needs a
 * lookup this milestone's contract doesn't provide; typing an id
 * verbatim is the honest minimum rather than a fake-looking dropdown of
 * one.
 */
function AssigneeControl({ todoId, assigneeId }: { todoId: string; assigneeId: string | null }) {
  const createEvent = useCreateTodoEventMutation();
  const [draft, setDraft] = useState(assigneeId ?? "");

  function commit() {
    const trimmed = draft.trim();
    if (trimmed === (assigneeId ?? "")) return;
    createEvent.mutate({ id: todoId, type: "assigned", to: trimmed || null });
  }

  return (
    <Input
      value={draft}
      onChange={(e) => setDraft(e.target.value)}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === "Enter") commit();
      }}
      placeholder="unassigned (paste a user id)"
      className="h-8 w-56"
      aria-label="Assignee user id"
    />
  );
}

function CommentBox({ todoId }: { todoId: string }) {
  const createEvent = useCreateTodoEventMutation();
  const [body, setBody] = useState("");

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    const trimmed = body.trim();
    if (!trimmed) return;
    createEvent.mutate(
      { id: todoId, type: "commented", body: trimmed },
      { onSuccess: () => setBody("") },
    );
  }

  return (
    <form onSubmit={handleSubmit} className="mt-6 flex flex-col gap-2">
      <Textarea
        value={body}
        onChange={(e) => setBody(e.target.value)}
        placeholder="Add a comment (Markdown supported)…"
        rows={3}
      />
      <div className="flex justify-end">
        <Button type="submit" size="sm" disabled={createEvent.isPending || !body.trim()}>
          {createEvent.isPending ? <Spinner size="sm" className="mr-2" /> : null}
          Comment
        </Button>
      </div>
    </form>
  );
}

export default function TodoDetailPage() {
  const { id = "" } = useParams<{ id: string }>();
  const todoQuery = useTodoQuery(id);
  const eventsQuery = useTodoEventsQuery(id);

  if (todoQuery.isPending) {
    return (
      <div className="flex justify-center py-16">
        <Spinner size="lg" />
      </div>
    );
  }

  if (todoQuery.isError || !todoQuery.data) {
    return (
      <main className="mx-auto max-w-2xl px-4 py-8">
        <ErrorState message="Couldn't load this todo." onRetry={() => void todoQuery.refetch()} />
      </main>
    );
  }

  const todo = todoQuery.data;

  return (
    <main className="mx-auto max-w-2xl px-4 py-8">
      <Link
        to="/"
        className="mb-4 inline-flex items-center gap-1 text-sm text-[var(--sea-ink-soft)] hover:text-[var(--sea-ink)]"
      >
        <ArrowLeft className="size-4" /> Back to todos
      </Link>

      <h1 className="mb-4 text-2xl font-bold text-[var(--sea-ink)]">{todo.title}</h1>

      <div className="mb-6 flex flex-wrap items-center gap-3 rounded-2xl border border-[var(--line)] bg-[var(--chip-bg)] p-4 shadow-sm">
        <div className="flex flex-col gap-1">
          <span className="text-xs text-[var(--sea-ink-soft)]">Status</span>
          <StatusControl todoId={todo.id} status={todo.status} />
        </div>
        <div className="flex flex-col gap-1">
          <span className="text-xs text-[var(--sea-ink-soft)]">Priority</span>
          <PriorityControl todoId={todo.id} priority={todo.priority} />
        </div>
        <div className="flex flex-col gap-1">
          <span className="text-xs text-[var(--sea-ink-soft)]">Assignee</span>
          <AssigneeControl todoId={todo.id} assigneeId={todo.assigneeId} />
        </div>
        {todo.dueDate && (
          <Badge variant="outline" className="ml-auto">
            due {new Date(todo.dueDate).toLocaleDateString()}
          </Badge>
        )}
      </div>

      <CommentBox todoId={todo.id} />

      <h2 className="mt-8 mb-2 text-base font-semibold text-[var(--sea-ink)]">Timeline</h2>
      {eventsQuery.isPending ? (
        <div className="flex justify-center py-8">
          <Spinner size="md" />
        </div>
      ) : eventsQuery.isError ? (
        <ErrorState
          message="Couldn't load this todo's timeline."
          onRetry={() => void eventsQuery.refetch()}
        />
      ) : (
        <ul className="rounded-2xl border border-[var(--line)] bg-[var(--chip-bg)] px-4 shadow-sm">
          {eventsQuery.data.map((event) => (
            <TimelineEventRow key={event.id} event={todoEventToTimelineEvent(event)} />
          ))}
        </ul>
      )}
    </main>
  );
}
