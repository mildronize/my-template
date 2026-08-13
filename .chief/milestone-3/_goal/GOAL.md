# Milestone 3: the real frontend — React SPA replaces the Go rendered view

TPL-1's correction, มายด์'s own words: *"the task TPL-1 is simple, replace
backend from node (next.js) to go, react keep the same. (SPA is fine)."*
Milestone-2 closed with a Go `html/template` owner view — a misread of that
same instruction (Clara derived "no React" from a Go-stack list), recorded
as superseded, not delivered, in `milestone-2/_goal/GOAL.md`. This
milestone builds the thing มายด์ actually asked for: my-task's real React
UI, talking to the Go backend milestone-1/2 already built.

Same branch and PR as milestone-2 (`milestone-2/close-parity-gap`, PR #2),
per มายด์'s explicit sequencing decision — commits prefixed
`feat(milestone-3/...)` so the diff stays legible as two separable pieces
of work sharing one PR.

## Objective

Ship a Vite-built React SPA, embedded in the Go binary via `embed.FS`, that
reuses my-task's actual design system and as much of its actual page code
as the evidence supports — not a rewrite wearing the same file names — and
that gives the owner a real CRUD surface for the first time. Today the
owner can log in and see rows Clara planted by hand; nothing on this
service lets them create one. Closing that is not optional scope: มายด์
asked for it directly the moment he had something to click on
(*"No.4 is tested no crud yet, I think you will provide crud on milestone
3 right?"*), and its absence is what made his first acceptance attempt
prove less than it looked like it proved.

## Context — two corrections that shaped this goal, both worth keeping visible

**Correction 1, มายด์'s: the Go-rendered view was never a deliverable.**
Already recorded in milestone-2's GOAL.md; not re-litigated here except to
name what it means for scope: this milestone doesn't extend
`internal/transport/bff/view_handler.go`, it retires it.

**Correction 2, found during this milestone's own grill, same shape one
layer down.** "Keep React" was at first treated as settled by picking
Vite over Next.js as the bundler — an implementation choice quietly
deciding a scope question, the identical error pattern that reopened this
ticket in the first place (Clara: *"'Vite instead of Next' is an
implementation choice that quietly decides how much of his frontend
survives"*). Caught before it shipped, not after, by doing a file-by-file
inventory of my-task's actual frontend instead of assuming.

**That inventory was itself read from the wrong branch on the first
pass** (`feat/due-date-priority`, not `origin/main`) — caught by Clara
diffing three files against main, which found `tasks/page.tsx` carrying
undeployed due-date/priority UI. A full re-audit against the ref (not
just the three files Clara's spot-check covered) found one more instance
of the same class: `src/lib/use-local-date.ts`, counted as portable,
**does not exist on `origin/main` at all** — branch-only, added alongside
the due-date feature. Neither omission changed the shape of the
conclusion; both would have shipped wrong file contents if uncaught.

**Final inventory, `origin/main @ fdceb8b`
(`fdceb8befbcebfa25d2216d71264bb7c5e8c96d7`), fetched and diffed
2026-08-13:**

| Bucket | Count | What |
| --- | --- | --- |
| Carries as-is | 24 | `components/ui/*` (16 of 17 — `combobox.test.tsx` excluded, a test file, not shipped app code), `EmptyState.tsx`, `ErrorState.tsx`, `Footer.tsx`, `ThemeToggle.tsx`, `lib/{utils,use-is-mobile}.ts`, `(app)/layout.tsx`, `settings/page.tsx` (the shell) |
| Mechanical swap | 2 | `Header.tsx` (`next/link`→react-router `Link`, `usePathname`→`useLocation`), `AuthGate.tsx` (only `useSession()`'s data source changes — the confirmed-logged-out tri-state, no-flash redirect, and spinner-not-bounce-on-transient-error logic is plain React and carries verbatim) |
| Must rewrite | 8 | root `layout.tsx` (`next/font`+`Metadata`→Vite `index.html`), `trpc/{server,react,query-client}.ts` as one unit (→ fetch+TanStack Query against the new BFF spec), `tasks/page.tsx`→todos list, `NewTaskDialog.tsx`→create-todo dialog (confirmed byte-identical on both refs — no drift here), `api-key-settings.tsx`→settings/keys screen, `login/page.tsx`→replaced by something simpler (my-task's version drives client-side `better-auth` OAuth; ours needs none, since `GET /login` already does the full PKCE redirect server-side) |

24/2/8 of 34 files. **On this evidence, "react keep the same" is honestly
satisfied**: the design system, the shell, the auth-gating UX, and most
of the page markup are the same code pointed at a different data source,
not a rewrite that happens to share file names. Where a "must rewrite"
file's JSX is largely reusable (the CRUD screens, the settings screen),
that's noted per-file below — bucket 3 means the data-fetching mechanism
inside changes, not necessarily that the file is being authored from
nothing.

## Decisions

| Decision | Answer | Owner |
| --- | --- | --- |
| Frontend framework | React 19 + Vite (not Next.js, not a static export) — my-task's SSR/RSC surface is exactly the part not being carried; a static export would drag Next's build pipeline along for a plain SPA that gains nothing from it. Output sits cleanly in `embed.FS` | Luna, agreed by Clara |
| Dependencies carried verbatim | Tailwind + typography, Radix (alert-dialog, checkbox, dialog, popover, select, slot), lucide, clsx, tailwind-merge, class-variance-authority, sonner, zod, TanStack Query + Table — verified in my-task's actual `package.json`/`src/components/ui/` | มายด์, verified (Luna) |
| Not carried | tRPC — TypeScript-server-only, no meaning once the backend is Go. The SPA talks to the BFF over JSON | Clara |
| SPA serving | `embed.FS` — one binary, one artifact, matches the template's "clone and go" value proposition. `make build` must build the SPA (`web/`) before the Go build, or the binary embeds a stale artifact that passes every test silently. Dev mode needs a Vite-proxy-to-Go story, documented, so "why isn't my change showing" isn't the first question every fork asks | มายด์ (embed vs static), Clara (the two consequences) |
| Screen scope | Login (static link to `GET /login` — no client-side OAuth, unlike my-task's `login/page.tsx`) + Todos (full CRUD) + Settings (list/revoke own keys — **no issuance, no rotate-via-UI**, both stay CLI-only by the same reasoning `API.md` already gives for no `POST /api/v1/keys`: a settings screen with a "create key" button teaches someone to add the matching endpoint and breach the issuance boundary) | Settled by TPL-1's own text — "preserve sso integration, and settings" — per Clara, deciding rather than routing since this is the opposite of the failure mode all night (a derived *removal*); ruled by มายด์'s own wording |
| Router | react-router, client-side, two real routes (`/`, `/settings`) | Luna — a forker arriving from my-task arrives from file-based routing and will look for where routes live; shipping none teaches them to hand-roll one (Clara) |
| Auth seam | BFF's server-side OAuth stays exactly as milestone-2 built it (`GET /login`→Hydra→`GET /callback`→signed HttpOnly session cookie, `Secure` derived from scheme). The SPA is a plain client: fetches with credentials, never sees a token, gets 401→redirect to `/login`. No client-side PKCE — an access token in browser JS would renegotiate every property that flow already earned, for no gain | Clara, agreed (Luna) |
| CRUD is in scope, not deferred again | The BFF gains session-authenticated JSON write endpoints for todos (create/update/delete, plus list/get). Reuses `internal/domain/todo.Service` directly — the same shared-service-layer decision milestone-2 made for the public API extends here without new domain logic. Closes the known limitation milestone-2 documented and parked | มายด์, direct ("I think you will provide crud on milestone 3 right?") |
| Owner-scoping on the new endpoints | I3 applies: every BFF write scopes by the session's owner, 404 (never 403) for another owner's row — same absence-not-permission rule the public API already follows. **This is the first time I3's documented "per-module, not per-layer" granularity gap (milestone-2, known limitation) is not hypothetical** — todo now has two independently-scoped layers (`publicapi` handler, `bff` handler), so a dedicated BFF-layer I3 test is required, not just relying on the existing `publicapi`-layer one | Clara |
| I2/I12 boundary, written down not implied | I2 (Bearer never resolves to owner) and I12 (BFF session never resolves to agent) together are why owner writes belong on the session-authenticated BFF and never the Bearer-authenticated public API — not an inconsistency to reconcile, two disjoint proof-of-identity mechanisms by design. State this explicitly in code comments near both boundaries and in `API.md`/`INVARIANTS.md`, so a forker who notices "owner can write via BFF, can't via API" doesn't 'fix' it by relaxing I2 | Clara |
| API spec: two files, not one | `openapi.yaml` stays the public, Bearer-authenticated, agent-facing surface — unchanged in kind. The BFF's session-authenticated JSON endpoints get a **second spec file**, named for what it holds. Structure enforces the I2/I12 boundary the way `ARCHITECTURE.md`'s import rule enforces the domain/transport boundary — a paragraph asks a rule to be respected, a file split makes violating it require an active choice. Also protects item 9's skill doc (its source of truth stays 100% agent-callable, no "spec minus the parts that don't apply to you") and anyone generating a client from the public spec (gets only what's actually reachable with a Bearer credential) | Clara (changed from Luna's original one-file-two-path-groups proposal) |
| Typed client, still no tRPC | `openapi-typescript` generates types from the BFF's new spec (mirrors `oapi-codegen` on the Go side, one source of truth), a thin `fetch` wrapper underneath, TanStack Query for caching/mutation state — already carried, already familiar to anyone coming from my-task, and the one piece of the data layer that was never tRPC-coupled | Luna |
| Session-check hook | `AuthGate`'s logic ports verbatim (bucket 2); only its data source changes — from `useSession()` (better-auth) to a TanStack Query hook against a new BFF session-check endpoint. Same tri-state (confirmed-logged-out / has-or-had-a-session / pending), same no-flash-on-transient-error behavior | Luna |
| Old view retired | `internal/transport/bff/view_handler.go` and milestone-2's Done-when-9 scoped-rendering test are removed, not extended — they tested the artifact being replaced. A new equivalent (SPA renders scoped owner data after login) replaces it in this milestone's Done-when list | Luna, per milestone-2 GOAL.md's own recorded sequencing plan |
| Fork procedure gains a dimension | A forker now also renames a React app — package name, API base URL, domain nouns inside components — on top of `GETTING-STARTED.md` Step 5's existing three-location Go operation. Documented as its own explicit step, not folded silently into the existing rename checklist. Blind test 6 (deferred from milestone-2) runs after this lands, against the real final procedure | Clara |
| Node as a prerequisite | `make tools` currently installs only Go tooling into a gitignored `bin/`. A fresh clone now also needs a Node toolchain — stated explicitly in `GETTING-STARTED.md`'s prerequisites, not discovered via a failed first command the way blind test 5 caught happening once already | Clara |
| `make test` and the two-suites problem | Six Go packages already ship with no test files, and `cmd/smoke` has a documented history of being invisibly broken behind a green `go test ./...`. Adding a JS test runner (Vitest, matching my-task's own choice) is a second green light that says nothing about the first. `make test` runs both suites; if that's ever not true, the docs say so plainly rather than let one green light stand in for two | Clara |

## Scope

### In scope

- `web/` — Vite + React 19 project: `package.json`, `vite.config.ts`,
  `tsconfig.json`, Tailwind config, ported design system
  (`components/ui/*`, `EmptyState`, `ErrorState`, `Footer`, `ThemeToggle`,
  `lib/{utils,use-is-mobile}.ts`)
- react-router wired for `/` (todos) and `/settings`; `Header.tsx`,
  `AuthGate.tsx` ported per their bucket-2 swaps
- A new BFF session-check endpoint + hook backing `AuthGate`
- BFF JSON endpoints for todo CRUD (create/list/get/update/delete),
  session-authenticated, owner-scoped (I3), reusing
  `internal/domain/todo.Service` directly — no new domain logic
- BFF JSON endpoints for the settings screen: list own keys, revoke one —
  reusing `internal/identity.Service`'s existing
  `ListAPIKeys`/`RevokeAPIKey` methods, session-authenticated instead of
  key-authenticated
- Second OpenAPI spec file for the BFF's session-authenticated JSON
  surface, `oapi-codegen`-generated Go interface, `openapi-typescript`
  types for the SPA
- I2/I12 boundary documented explicitly in code + `API.md`/`INVARIANTS.md`
- Dedicated BFF-layer I3 test for todo (closing the granularity gap
  milestone-2 only documented)
- `embed.FS` wiring, `make build` ordering (SPA before Go embed), Vite
  dev-proxy story documented
- `docs/GETTING-STARTED.md`: Node prerequisite, the React-app fork
  dimension as its own step, `make test`'s two-suite coverage stated
  plainly
- Retiring `internal/transport/bff/view_handler.go` and its
  Go-`html/template` test, replaced by an equivalent SPA-renders-scoped-
  data check
- At least one meaningful Vitest test per new hook that replaces
  tRPC-coupled logic (the session-check hook, the todos-CRUD hook) — not
  full coverage, but not zero either, given the make-test two-suites
  concern above

### Out of scope

Key issuance/rotation via the settings UI (CLI-only, unchanged from
milestone-2 — `API.md`'s no-`POST /api/v1/keys` reasoning extends here) ·
my-task's task/project/status/label/priority/due-date/activity-log domain
(milestone-1's simplification to a bare todo — "without project," 2-4
simple fields, no activity log — already decided and not reopened here) ·
porting Next.js SSR/RSC in any form · tRPC in any form · a second,
crew-facing "how to use your key" skill doc (unchanged from milestone-2's
own out-of-scope note)

## Done when

1. `web/` builds (`npm run build` or equivalent) producing static output;
   `make build` runs it before the Go build, and the resulting binary
   serves that exact output at `GET /` (embedded, not a stale prior
   build — verified by changing a visible string in the SPA and
   confirming it appears after a fresh `make build`, not just that a
   build succeeds).
2. BFF JSON todo endpoints exist, session-authenticated, and a real
   create→list→update→delete round trip succeeds against a live instance
   with a real session cookie (not unit-injected).
3. New BFF-layer I3 test: a session-authenticated request for another
   owner's todo returns 404, never 403 — the same absence-not-permission
   rule, checked at this layer specifically, not inherited from the
   `publicapi`-layer test.
4. I2/I12 boundary reasoning appears as an identifiable comment/section
   at both the point I2 is enforced and the point BFF writes are
   enabled, plus in `API.md`/`INVARIANTS.md` — presence check, not prose
   buried elsewhere.
5. BFF JSON key endpoints (list, revoke) exist, session-authenticated,
   reusing `identity.Service`'s existing methods; no issuance or rotate
   endpoint exists on this surface (a negative check — the settings
   screen can't accidentally grow the capability by someone adding the
   route it visually implies).
6. `AuthGate`'s ported logic: a test proves the no-flash-on-transient-
   error behavior survives the swap from `useSession()` to the new
   session-check hook — a failing/erroring check does not bounce a
   previously-authenticated user to `/login`.
7. `internal/transport/bff/view_handler.go` and milestone-2's Done-when-9
   test are removed; a replacement test proves the SPA path renders
   scoped owner data after login (seed an owner session + a todo, hit the
   new todos-list endpoint through a real session, assert that todo's
   title is in the response) — the same property Done-when 9 checked,
   against the artifact that actually ships now.
8. `docs/GETTING-STARTED.md` states Node as a prerequisite before the
   first command that needs it (mirrors milestone-2's Done-when-4
   shape for `register-<service>.sh`) and has an explicit React-app
   rename step distinct from the existing Go rename checklist.
9. `make test` runs both the Go suite and the JS suite, or
   `GETTING-STARTED.md` states plainly that it doesn't and what a forker
   must run instead — not left to be discovered.
10. At least one Vitest test exists for the session-check hook and one
    for the todos-CRUD hook — not asserting full coverage, asserting
    these two specific replacements for tRPC-coupled logic aren't
    untested by construction.
11. `docker compose up` still works after `embed.FS`/`web/` land — same
    discipline as milestone-2's Done-when 16, checked directly.

## Human acceptance — after the loop, not part of it

**มายด์ logs in, creates a todo through the SPA, sees it persist, edits or
deletes it.** This is the criterion his own acceptance attempt exposed as
missing — a completed login landing on a page that can only ever show
rows someone else inserted does not validate a CRUD template. Not a
stopping condition for the same reason milestone-2's couldn't be: no
unattended loop can drive a real browser through a real Hydra consent
screen. Done-when 2 and 3 (the machine-checkable half — the round trip
works, the scoping holds) are checked in the loop; the real browser, real
click-through is checked once, by a human, afterward — this time all the
way through create, not just view.
