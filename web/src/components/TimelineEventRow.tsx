// milestone-4/task-7: renders one `todo_events` row — used by BOTH
// `ActivityPage` (the cross-todo feed, `/activity`) and `TodoDetailPage`'s
// own per-todo timeline (`/todos/:id`), so the two pages cannot drift
// apart in how an event reads. This is GOAL.md's Done-when 5/8, not a
// nice-to-have: `.chief/milestone-4/_contract/API.md`'s own `ActivityItem`
// schema doc comment says it directly — "same underlying row `TodoEvent`
// ... represents on the per-todo timeline, so the two share a rendering
// component."
//
// The named source this mirrors is my-task's own
// `~/gits/my-task/src/components/TimelineEvent.tsx` (`TimelineEventRow`) —
// structure, not just existence: the per-event-type summary components,
// the 🧑/🤖 provenance-mark split on `role === "owner"` (my-task's own
// "owner" is my-task's human; this repo's BFF session actor is
// analogously always `role: "owner"`, `todo_handler.go:139`'s own comment
// — same branch, same meaning), and the comment-vs-non-comment layout
// split (a comment's body IS the content; a status change's summary line
// IS the content, with any body underneath as a footnote).
//
// Adapted, not copied verbatim, in three ways this repo's own shape
// requires:
//
//  1. `next/link`'s `Link` (`href`) -> react-router's `Link` (`to`) —
//     App.tsx's own routing convention, mechanical only.
//  2. No `useLocalDateTime` hook exists in this repo (my-task's own
//     `~/lib/use-local-date`) — `createdAt` is formatted inline with
//     `toLocaleString()`, the same primitive ApiKeySettings.tsx's own
//     `formatDate` already uses elsewhere in this codebase.
//  3. Payload shapes are this repo's own, not my-task's — read
//     `internal/domain/todo/service.go`'s `Append` (the actual JSON this
//     project's Go backend marshals) rather than assuming my-task's own
//     richer `{from:{name}}`/`{to:{handle}}` object shapes carry over:
//       - `status_changed`: `{from, to}` are bare status strings (this
//         template's `Status` is a flat enum, not an entity with its own
//         name field).
//       - `assigned`: `{from, to}` are bare user ids (or `null`), not
//         `{handle}` objects — `service.go:337`'s `strPtrToAny` on
//         `AssigneeID` directly. See `ProvenanceMark`'s own doc comment
//         below for the load-bearing consequence of this on the per-todo
//         timeline specifically.
//       - `field_changed`: `{field, from, to}`, `field` one of
//         `title`|`priority`|`dueDate` (`service.go`'s `FieldChangeInput`).
//       - `created`: `{title}` only (`service.go:137`) — no status object
//         to report, unlike my-task's own richer create payload.
//
// I20 (`_contract/INVARIANTS.md`) lives one layer down, in `~/components/
// Markdown` — this file never touches a comment body directly except to
// hand it to that component; see that file's header before changing
// anything about how a body reaches the screen. See
// `TimelineEventRow.test.tsx` for I20's own proof (a raw-HTML body renders
// escaped, not live) and for Done-when 8/9's own proofs (shared-component
// identity across both pages' real prop shapes; the provenance mark
// actually branching on role rather than always showing one mark).
import { Link } from "react-router-dom";
import { Markdown } from "~/components/Markdown";

/**
 * The shape both pages' own event data reduces to before reaching this
 * component. `actor` is REQUIRED, not optional — matching my-task's own
 * `TimelineEventData` shape exactly, and matching this repo's own
 * `bffapi.ActivityActor` (`{handle, role}`) wire shape exactly, which is
 * what `GET /api/bff/activity`'s `ActivityItem.actor` genuinely carries on
 * every row.
 *
 * `GET /api/bff/todos/:id/events`'s own `TodoEvent`, by contrast, carries
 * only `actorId` (a bare id) — no `handle`, no `role` — confirmed by
 * reading `internal/bffapi/bffapi.gen.go`'s `TodoEvent` struct and
 * `internal/transport/bff/todo_handler.go:100-105`'s `toBFFEvent`, which
 * never resolves one. This is a genuine, unresolved contract gap this
 * task found and is not this task's to fix (no `.go` files touched, per
 * task-7's own scope) — see the task-7 report's "Contract gap" section.
 * `TodoDetailPage.tsx` copes with it honestly: it sets `role: "unknown"`
 * for every row it renders (never guesses `"owner"` or `"agent"`), and
 * `ProvenanceMark` below renders a visibly distinct third mark for that
 * case rather than defaulting to either real one — the same discipline
 * Done-when 9's own test exists to enforce (a mark that silently defaults
 * to one value regardless of the data is exactly the failure class that
 * test is built to catch, and defaulting an *unknown* role to a real one
 * would be exactly that failure, just self-inflicted instead of a bug).
 */
export interface TimelineEventData {
  id: string;
  seq: number;
  type: string;
  actor: { handle: string; role: string };
  payload?: unknown;
  body?: string | null;
  createdAt: string;
}

function formatCreatedAt(value: string): string {
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/**
 * The provenance mark — Done-when 6/9's own subject. Three states, not
 * two: `role === "owner"` is มายด์ himself (the only role a BFF session
 * ever resolves to, `todo_handler.go:139`); `role === "agent"` is any
 * Bearer-authenticated crew member; anything else (concretely: `"unknown"`,
 * `TodoDetailPage.tsx`'s own honest placeholder for the contract gap
 * above) renders neither real mark — a wrong guess would be worse than an
 * admitted unknown, on a screen whose entire job is telling a later reader
 * who wrote what (`_contract/INVARIANTS.md` I20's own stated threat model:
 * "the activity log is a cross-agent channel").
 */
function ProvenanceMark({ role }: { role: string }) {
  if (role === "owner") {
    return (
      <span title="Human" aria-label="Human" className="text-base leading-none">
        🧑
      </span>
    );
  }
  if (role === "agent") {
    return (
      <span title="Agent" aria-label="Agent" className="text-base leading-none">
        🤖
      </span>
    );
  }
  return (
    <span
      title="Unknown provenance"
      aria-label="Unknown provenance"
      className="text-base leading-none"
    >
      ❔
    </span>
  );
}

function CreatedSummary({ payload }: { payload: unknown }) {
  const p = payload as { title?: string } | undefined;
  return <>created this todo{p?.title ? <> — <b>{p.title}</b></> : null}</>;
}

function StatusChangedSummary({ payload }: { payload: unknown }) {
  const p = payload as { from?: string; to?: string } | undefined;
  return (
    <>
      changed status: <b>{p?.from ?? "?"}</b> → <b>{p?.to ?? "?"}</b>
    </>
  );
}

/**
 * `to`/`from` are bare user ids (`internal/domain/todo/service.go:337`),
 * not handles — this repo's `Todo.assigneeId` and `TodoEvent`'s own
 * `assigned` payload never carry a display name, the same category of gap
 * `TimelineEventData.actor`'s own doc comment names for the per-todo
 * timeline's actor. Shown as-is (a raw id) rather than invented — see the
 * task-7 report's "Contract gap" section.
 */
function AssignedSummary({ payload }: { payload: unknown }) {
  const p = payload as { from?: string | null; to?: string | null } | undefined;
  if (!p?.to) {
    return <>unassigned{p?.from ? <> (was {p.from})</> : null}</>;
  }
  return (
    <>
      assigned to <b>{p.to}</b>
    </>
  );
}

/** Longest previous/new value shown inline before it is cut. */
const FIELD_PREVIEW = 140;

function preview(value: unknown): string | null {
  if (value === null || value === undefined) return null;
  const text = String(value).replace(/\s+/g, " ").trim();
  if (text === "") return null;
  return text.length > FIELD_PREVIEW ? `${text.slice(0, FIELD_PREVIEW)}…` : text;
}

/**
 * Shows WHAT changed, not just that something did — mirrors my-task's own
 * `FieldChangedSummary` reasoning verbatim (see that file): the timeline's
 * entire job is answering "what was it before," which the current value
 * shown elsewhere on the page can never answer.
 *
 * Deliberately plain text, not markdown, same reasoning as the named
 * source: these are quoted values, and rendering a previous title as
 * headings/bold would make the quote look like the page's own voice.
 */
function FieldChangedSummary({ payload }: { payload: unknown }) {
  const p = payload as { field?: string; from?: unknown; to?: unknown } | undefined;
  const field = p?.field ?? "a field";
  const from = preview(p?.from);
  const to = preview(p?.to);

  if (from && !to) {
    return (
      <>
        cleared <b>{field}</b> <span className="line-through">{from}</span>
      </>
    );
  }
  if (!from && to) {
    return (
      <>
        set <b>{field}</b> to <span className="text-[var(--sea-ink)]">{to}</span>
      </>
    );
  }
  if (from && to) {
    return (
      <>
        changed <b>{field}</b> from <span className="line-through">{from}</span> to{" "}
        <span className="text-[var(--sea-ink)]">{to}</span>
      </>
    );
  }
  return (
    <>
      changed <b>{field}</b>
    </>
  );
}

function EventSummary({ event }: { event: TimelineEventData }) {
  switch (event.type) {
    case "created":
      return <CreatedSummary payload={event.payload} />;
    case "commented":
      return <>commented</>;
    case "status_changed":
      return <StatusChangedSummary payload={event.payload} />;
    case "assigned":
      return <AssignedSummary payload={event.payload} />;
    case "field_changed":
      return <FieldChangedSummary payload={event.payload} />;
    default:
      return <>{event.type}</>;
  }
}

/**
 * `todoLink` is `ActivityPage`'s own extra context (which todo an event
 * belongs to) — omitted entirely by `TodoDetailPage`, which is already on
 * that todo's own page and would be repeating itself to show it (mirrors
 * my-task's own `taskLink?` prop, same reasoning, same optionality).
 */
export function TimelineEventRow({
  event,
  todoLink,
}: {
  event: TimelineEventData;
  todoLink?: { id: string; title: string };
}) {
  // A comment and a status change are different kinds of thing: for a
  // status change the summary line IS the content; for a comment, the
  // prose someone wrote is the thing worth reading and the byline is the
  // caption. Mirrors my-task's own `isComment` split exactly.
  const isComment = event.type === "commented" && !!event.body;

  const byline = (
    <>
      <span className="font-medium">{event.actor.handle}</span>
      {todoLink && (
        <>
          {" on "}
          <Link to={`/todos/${todoLink.id}`} className="font-medium">
            {todoLink.title}
          </Link>
        </>
      )}
      {" · "}
      {formatCreatedAt(event.createdAt)}
    </>
  );

  if (isComment) {
    return (
      <li className="flex gap-3 border-b border-[var(--line)] py-3 last:border-0">
        <ProvenanceMark role={event.actor.role} />
        <div className="min-w-0 flex-1">
          <div className="rounded-xl border border-[var(--line)] bg-[var(--surface,transparent)] px-3 py-2 text-sm text-[var(--sea-ink)]">
            <Markdown>{event.body!}</Markdown>
          </div>
          <p className="mt-1 text-xs text-[var(--sea-ink-soft)]">{byline}</p>
        </div>
      </li>
    );
  }

  return (
    <li className="flex gap-3 border-b border-[var(--line)] py-3 last:border-0">
      <ProvenanceMark role={event.actor.role} />
      <div className="min-w-0 flex-1">
        <p className="text-sm text-[var(--sea-ink)]">
          <span className="font-medium">{event.actor.handle}</span>{" "}
          <span className="text-[var(--sea-ink-soft)]">
            <EventSummary event={event} />
          </span>
          {todoLink && (
            <>
              {" "}
              on{" "}
              <Link to={`/todos/${todoLink.id}`} className="font-medium">
                {todoLink.title}
              </Link>
            </>
          )}
        </p>
        {event.body && (
          <div className="mt-1 text-sm text-[var(--sea-ink)]">
            <Markdown>{event.body}</Markdown>
          </div>
        )}
        <p className="mt-1 text-xs text-[var(--sea-ink-soft)]">{formatCreatedAt(event.createdAt)}</p>
      </div>
    </li>
  );
}
