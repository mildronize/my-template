# Frontend/API architecture alternatives — a study, not a decision

Status: **research only, no code changed**. Written up at Mild's request after a
conversation exploring whether my-template's current stack (Go + OpenAPI-first REST +
React SPA) could be made lighter, prompted by comparing it against my-task's Next.js +
tRPC stack. As of `main@8009eaa` (2026-08-15).

## 1. The starting complaint

my-template feels like it has more hand-written (non-codegen) code, and is slower to
write against, than my-task — despite being the smaller project. This document traces
why, and evaluates two concrete alternatives that came up: replacing `oapi-codegen`
with [huma](https://github.com/danielgtaylor/huma), and replacing the React SPA with
[templ](https://github.com/a-h/templ) SSR + islands (React/Svelte for the genuinely
interactive parts, [Alpine.js](https://alpinejs.dev/) + htmx for everything else).

## 2. Current architecture, briefly

- **Two OpenAPI specs, hand-authored**: `openapi.yaml` (public, Bearer-token,
  agent-facing) and `bff-openapi.yaml` (session-authenticated, web-facing).
- **`oapi-codegen`** generates Go server interfaces + types from both
  (`internal/api/openapi.gen.go`, `internal/bffapi/bffapi.gen.go`).
- **`openapi-typescript`** generates TS types from `bff-openapi.yaml` alone
  (`web/src/lib/api/bff-schema.gen.ts`) — the web frontend only ever talks to the BFF
  surface.
- One hand-authored spec file feeds two independent codegen tools, one per language.
  Neither generator knows about the other; the guarantee that both stay in sync comes
  from each language's own compiler refusing to build against a stale field name, not
  from any shared runtime.

### Why this is the way it is

This was a deliberate milestone-1 decision (`_goal/GOAL.md`: "OpenAPI-first... Luna
(from task stack)"), not a default nobody questioned. It buys two things a same-language
stack doesn't need to buy:

1. **A stable, language-agnostic contract for agent clients.** `openapi.yaml` exists
   specifically so a non-TypeScript caller (an agent, a CLI, another service) can
   consume the public API without ever touching this repo's Go or TS code. my-task has
   no equivalent requirement — its only consumer is its own Next.js frontend, which is
   exactly what tRPC is built for.
2. **Compiler-enforced non-drift across a language boundary.** Traced concretely: renaming
   `Todo.title` → `Todo.name` in `bff-openapi.yaml` breaks `go build` at every Go call
   site (`internal/transport/bff/todo_handler.go:84,250,309,480`) and breaks
   `npm run typecheck` at every TS call site (`TodoRow.tsx`, `TodoDetailPage.tsx`,
   `ActivityList.tsx`, `TimelineEventRow.tsx`) the moment both codegen steps re-run.
   There is no code path where a stale field name silently continues to work on one
   side.

## 3. Alternative considered: huma (replace `oapi-codegen`)

huma is code-first (Go struct tags → generated OpenAPI/JSON Schema), not spec-first.
Verified against huma's own source (`/tmp/huma`, cloned for this study):

**Real advantages, post-migration (migration cost set aside):**
- Spec/code drift becomes structurally impossible — the spec *is* derived from the code,
  there is no separate hand-authored file that can fall out of sync with the
  implementation.
- `additionalProperties: false`-style strict validation is real and enforced at request
  time (`huma/validate.go:841,955`), not just documented — matches this project's I1
  (actor-field rejection) requirement.
- Error shape is pluggable (`huma.NewError` is an overridable `var`) — the custom
  `{error:{code,message,hint}}` envelope this project uses is not locked out.
- `Resolver` gives cross-field/business-rule validation (e.g. I18's transition check) a
  proper home, cleaner than the current inline handler checks.
- Free extras this template doesn't have today: `/docs` (Stoplight Elements), served
  `/openapi.json`/`.yaml`, conditional-request helpers, JSON/CBOR content negotiation.
- `humagin` adapter exists — no router change required.

**Real costs, post-migration:**
- No single hand-authored file a fork can read before writing any code — the spec is
  derived, not the source. Keeping `docs/GETTING-STARTED.md`'s "read the contract before
  touching code" property would require bolting on a dump-spec-to-file step huma doesn't
  ship.
- Does not touch the actual complaint. huma only changes how the *Go-side* spec is
  produced; the frontend still needs a JSON Schema/OpenAPI file on disk for
  `openapi-typescript` to read, so the Go↔TS bridge — and everything that costs — stays
  exactly as it is.

**Conclusion: not recommended as a full-repo swap.** It's a genuine improvement in one
narrow respect (drift-proofing) but doesn't touch the LOC/velocity complaint that started
this study, and gives up a property (single-file, buildless contract) the fork workflow
actively depends on.

## 4. A useful data point: typ-fleet's own Rust↔TS boundary

For contrast, typ-fleet (`~/gits/typ-fleet-home/typ-fleet`, Rust/axum backend + React/Vite
console) crosses the exact same kind of language boundary with **no bridge at all**:

- `CrewInfo` (Rust, `crates/typ-proto/src/types.rs:28`) and `CrewInfoSchema` (TS Zod,
  `console/src/api/schemas.ts:19`) are independently hand-written. No `ts-rs`/`specta`/
  `utoipa`/`schemars` in the workspace. A human keeps them in sync by hand; Zod only
  catches drift at runtime, when a real request actually flows through it.
- On the Rust side there's also no DB/domain/API-DTO split the way my-template has —
  `CrewInfo` is built directly in `store.rs`/`manager.rs` and shipped straight to the
  HTTP layer. One type, not three.

This is a legitimate design point on the same spectrum, not a mistake — typ-fleet is an
internal tool built and consumed by the same small team, so the drift risk is cheap.
my-template is a fork template handed to people who don't share context with whoever
wrote the backend, which is exactly the situation compiler-enforced sync is worth paying
for.

| | Cross-language? | Bridge | Drift caught | Types per concept |
|---|---|---|---|---|
| my-task (tRPC) | No (TS↔TS) | type inference, no spec file | compile-time | 1 |
| my-template (OpenAPI) | Yes (Go↔TS) | hand-authored spec + 2 codegens | compile-time, both sides | 3–4 |
| typ-fleet | Yes (Rust↔TS) | none — hand-synced | runtime only | 1 per side, unsynced |

## 5. Root cause of the LOC/velocity gap vs my-task

Traced concretely on one field (`Todo`): it exists as **four representations** —
`db.Todo` (sqlc), `todo.Todo` (domain, `internal/domain/todo/repo.go:57`), `bffapi.Todo`
(generated API DTO), and a generated TS type — bridged by two hand-written conversion
functions, `todoFromRow` (`repo.go:138`) and `toBFFTodo` (`todo_handler.go:76`).

This is **not a tooling problem** (huma wouldn't remove it — see §3) and not really an
"OpenAPI vs tRPC" problem either. Two separate, compounding causes:

1. **Go's nominal typing vs TS's structural typing.** Go requires a named struct per
   distinct meaning and an explicit function to convert between structurally-identical
   ones. TS/Zod lets one inferred type flow through an entire call chain unchanged.
2. **my-template's own layering rule** (`ARCHITECTURE.md`: only `repo.go` may import the
   sqlc-generated package) deliberately keeps DB/domain/API types distinct so schema
   changes don't leak into business logic. This is an independent design decision, not
   an OpenAPI artifact — a tRPC codebase with the same layering discipline would pay the
   same tax, just less visibly (structural typing hides it).

Genuinely useful, but not free: the same-language stack (my-task) buys velocity that a
Go↔TS boundary structurally cannot match, regardless of which spec tool sits at that
boundary.

## 6. Alternative considered: templ SSR + islands (addresses the LOC complaint directly)

[templ](https://github.com/a-h/templ) (cloned to `/tmp` for this study) is a Go HTML
templating language with generated, type-safe render functions, integrates with `gin`
via `HTMLRender` with no router change, and has a first-class fragment-swap pattern for
pairing with htmx (`templ.Fragment(...)` + `hx-get`/`hx-swap`, verified against
`examples/htmx-fragments/main.templ`). The idea: SSR the pages that aren't heavily
interactive, keep a JS island (React or Svelte) only where real client-side state is
needed, and use Alpine.js (~15–40KB) + htmx for local UI state and server round-trips on
the SSR'd pages — instead of shipping the full 664KB React SPA bundle
(`web/dist/assets/index-*.js`) to every route.

### Per-page interactivity audit (verified against actual code, not assumed)

| Route | Actual interactivity found | Verdict |
|---|---|---|
| `/settings` | List + one revoke button + confirm dialog only (`ApiKeySettings.tsx`'s own header: "list + revoke only") | SSR (templ+htmx+Alpine) |
| `/activity` | Initial fetch + "load more" cursor pagination only. **No auto-poll** — `useActivityFeedQuery` (`web/src/lib/activity.ts:39`) has no `refetchInterval`; the 12s-poll behavior is my-task's own and was not carried over. (This corrects an earlier assumption made mid-conversation before the code was actually checked.) | SSR (templ+htmx) |
| `/todos/:id` | Two distinct halves: a read-only timeline (reuses `TimelineEventRow`) and editable fields (status/priority/assignee selects) | Split: timeline → SSR, edit fields → htmx+Alpine or small island |
| `/` (todos list) | Status `Select`, priority `Select`, assignee `Combobox` (search-select), inline title edit, a responsive new-todo dialog. **No client-side table sort/filter is actually wired** — `@tanstack/react-table` is a `package.json` dependency but has zero real usages in `src/` | Feasible to SSR too (see below) — but the one page worth thinking hardest about |

### The shared-component constraint survives, if handled correctly

Mild's own milestone-4 ruling required the cross-todo feed and the per-todo timeline to
render through **one** shared component so they can't drift apart (mirrors my-task's
`TimelineEventRow` reuse). That's only at risk if the two consumers end up on *different*
stacks (one templ, one React). Migrating `/activity` and `/todos/:id`'s timeline half
*together* keeps them on one shared templ component — the constraint holds, it just
moves from a `.tsx` file to a `.templ` file.

### Can `/` go SSR too?

Yes — its interactivity turned out to be simpler than assumed once actually audited: all
form-level controls (select-and-submit, text-and-submit), squarely in htmx's sweet spot.
Two things are **not** free, though, and need to be rebuilt rather than deleted:

- **The assignee `Combobox`** (search-as-you-type) — no native HTML equivalent; needs a
  small hand-written Alpine filter widget.
- **The responsive new-todo dialog** (desktop dialog / mobile bottom drawer,
  `responsive-dialog.tsx`, 87 lines today) — native `<dialog>` covers the modal behavior,
  but the mobile-drawer variant needs new CSS/Alpine, not just deletion.

Migrating `/` also means accepting round-trip-per-edit (no client-side optimistic
update the way TanStack Query gives today) as a deliberate UX trade, unless a small
island is kept specifically for inline-edit responsiveness.

### LOC accounting (the part Mild specifically asked to weigh, not just runtime weight)

Current hand-written frontend, excluding tests and generated files:
**3,738 lines** (`web/src`), broken down as `src/app` 943, `src/lib` 612,
`src/components` (non-ui) 1,230 — of which `TimelineEventRow.tsx` alone is 335 —
`src/components/ui` (Radix wrapper kit: Select/Dialog/AlertDialog/Combobox/Table/etc.)
**1,243**.

- **Partial migration (settings + activity + half of todo-detail, `/` stays React):**
  a real but modest net reduction, roughly **300–400 lines (~8–10%)**. The big line item,
  `src/components/ui` (1,243 lines), does **not** shrink — `/` still imports `Select`/
  `Dialog` directly, so the whole Radix kit has to stay for that one page's sake.
- **Full migration (all four routes, including `/`):** removes `src/lib` (612),
  `src/components/ui` (1,243, since there'd be no remaining React consumer), and most of
  `src/app` + `TimelineEventRow` (≈1,470 combined) — call it **~3,000+ TS lines removed**.
  Partially offset by new Go: templ template files (comparable density to the JSX they
  replace) plus new fragment-serving routes that don't exist today (today one JSON
  endpoint serves every client rendering; htmx wants distinct full-page and
  fragment-shaped responses per interaction), plus the hand-rebuilt combobox/
  responsive-drawer Alpine code — a rough **1,000–1,500 line** add-back.
  **Net estimate: ~40–50% reduction** on the current 3,738-line baseline — a materially
  different outcome from the partial-migration number, because it's the only version
  where the Radix kit itself actually goes away.

## 7. Summary / open decision

- **huma**: not recommended as a full swap. Real drift-proofing win, but doesn't touch
  the actual LOC/velocity complaint (the Go↔TS boundary itself is the cost, not which
  tool generates the Go-side spec), and gives up the single-file buildless contract the
  fork workflow relies on.
- **templ + islands + htmx/Alpine**: does address the LOC complaint, but the win is
  small unless committed to *all four* routes — a partial migration barely moves the
  needle because the Radix UI kit survives as long as any one page still needs it.
  Committing fully means budgeting real engineering time for two rebuilt controls
  (searchable combobox, responsive drawer) and a new class of backend route (fragment
  handlers), and accepting round-trip-per-edit UX on `/` unless a small island is kept
  there deliberately.
- Nothing in this document changes running code. It's written up so the tradeoffs are on
  record before any decision gets made — the actual call (partial vs. full migration,
  or leaving the stack as-is) is Mild's.

---

*This report reflects an exploratory conversation, not an implementation plan. Every
number and file:line reference above was checked against the actual repository/cloned
source at the time of writing, not assumed from memory.*
