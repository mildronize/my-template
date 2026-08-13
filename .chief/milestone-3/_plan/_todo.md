# Milestone 3 TODO

See `../_goal/GOAL.md` for objective/scope/decisions/done-when and
`../_contract/API.md` for the new `bff-openapi.yaml` surface. Every
Done-when item is owned by exactly one task.

**Ordering is deliberate, not just dependency order.** `_goal/GOAL.md`'s
two-spec-file split (Decision "API spec: two files, not one") is one of
six Decisions rows Clara has flagged to มายด์ as his to cut, and he has
not gated the goal yet. Planning is cheap to redo; code written against
`bff-openapi.yaml`, its generated Go interface, and a typed SPA fetch
layer is not. So: **task 1 does everything that stands regardless of
that decision — the Vite/React port, the design-system carry,
`embed.FS` wiring, `make build` ordering, the docs.** Task 2 is where the
split-dependent work starts (authoring the second spec file, wiring its
codegen, writing the handlers against it). If มายด์ cuts the split, only
task 2 onward needs rework — task 1's output is unaffected either way.
This is good task ordering on its own terms (get the framework-agnostic
work landed and verified before touching anything contract-shaped) and
happens to be cheap insurance against the one open decision.

**Chief-loop does not start until มายด์ gates the goal**, regardless of
this ordering — see this milestone's `loop-readiness` review. This file
plans the work; it doesn't authorize starting it.

- [ ] task-1: Vite/React scaffold + design-system carry — `web/`:
      `package.json`, `vite.config.ts`, `tsconfig.json`, Tailwind config.
      Port bucket 1 verbatim from my-task (`origin/main @ fdceb8b` — see
      `_goal/GOAL.md`'s inventory table): `components/ui/*` (16),
      `EmptyState`, `ErrorState`, `Footer`, `ThemeToggle`,
      `lib/{utils,use-is-mobile}.ts`, `(app)/layout.tsx`-equivalent shell,
      settings-page shell. Port bucket 2's `Header.tsx` (routing swap
      only — `next/link`→react-router `Link`, `usePathname`→
      `useLocation`); leave `AuthGate.tsx`'s data-source swap for task-3,
      since it needs task-2's session-check endpoint to swap onto.
      react-router wired for `/` and `/settings` with placeholder page
      content (real content lands in task-3) — enough to prove routing
      works, not enough to depend on the BFF's new endpoints existing.
      `embed.FS` wiring in `cmd/server`; `make build` builds `web/`
      before the Go build; Vite dev-proxy-to-Go documented. `docs/
      GETTING-STARTED.md`: Node prerequisite stated before the first
      command that needs it, React-app rename step written as its own
      step (package name, API base, domain nouns in components) distinct
      from the existing Go rename checklist. **These two docs items
      (Done-when 8) are among the six Decisions rows Clara flagged to
      มายด์ as cuttable** — if he cuts them, this task needs a trim, not
      a restructure (the cost is a docs edit, not rework of anything
      already built).
      **Verify the embed isn't stale, not just that the build succeeds**:
      change a visible string in a placeholder page, run `make build`,
      confirm the binary serves the new string — the same discipline
      task-1 in milestone-2 applied to its own guardrail rewrite.
      **Owns: Done-when 1, 8.**
- [ ] task-2: `bff-openapi.yaml` + generated BFF JSON handlers — **starts
      only once มายด์ has gated the goal; if the two-spec split is cut,
      re-scope this task before starting it, not after.** Author
      `bff-openapi.yaml` per `_contract/API.md`'s endpoint list: `GET
      /api/bff/me` (session-check), full todo CRUD (`GET/POST
      /api/bff/todos`, `GET/PATCH/DELETE /api/bff/todos/:id`), `GET
      /api/bff/keys`, `DELETE /api/bff/keys/:id`. Wire `oapi-codegen`
      for it (mirrors `openapi.yaml`'s existing `make generate` target —
      second invocation, second output file, not a modified first one).
      Handlers call `internal/domain/todo.Service` and
      `internal/identity.Service` directly — no new domain logic, per
      `_contract/API.md`'s per-endpoint service-method citations. Every
      write is owner-scoped from the session (I3): a session-authenticated
      request for another owner's todo returns 404, never 403 — **this
      is the first BFF-layer I3 test**, closing the granularity gap
      milestone-2 only documented (its own `TestI3_...` only ever existed
      at the `publicapi` layer). I2/I12 boundary reasoning
      (`_contract/API.md`'s own section) added as a code comment at
      `bff.RequireSession` and at the point BFF writes are enabled, plus
      confirmed present in `API.md`/`INVARIANTS.md` (already is, per the
      contract phase — this task is the presence check, not new prose).
      Negative check: no `POST /api/bff/keys`, no rotate endpoint exists
      anywhere on this surface.
      **Owns: Done-when 2, 3, 4, 5.**
- [ ] task-3: Wire the SPA for real — `openapi-typescript` generates
      types from `bff-openapi.yaml`; a thin `fetch` wrapper + TanStack
      Query hooks replace task-1's placeholder pages. `AuthGate.tsx`'s
      data-source swap (bucket 2, deferred from task-1): `useSession()`
      (better-auth) → the new session-check hook against `GET
      /api/bff/me` — its confirmed-logged-out tri-state, no-flash
      redirect, and spinner-not-bounce-on-transient-error logic carries
      verbatim, only the hook's data source changes. Bucket-3 files get
      their real rewrite here: root `layout.tsx`-equivalent (Vite
      `index.html` + CSS, no `next/font`/`Metadata`), the todos list +
      create-todo dialog (mirrors `tasks/page.tsx`/`NewTaskDialog.tsx`'s
      JSX, data hooks rewritten against the new endpoints instead of
      tRPC), the settings/keys screen (mirrors `api-key-settings.tsx`),
      login page (replaced by a static link to `GET /login` — not a
      port, per `_goal/GOAL.md`'s inventory note that my-task's version
      drives client-side OAuth this template doesn't need).
      `internal/transport/bff/view_handler.go` and milestone-2's
      Done-when-9 test removed. A Vitest component test renders the
      todos-list component against mocked API data and asserts: the
      mocked todo's title appears, and a title *not* in the mocked
      response does not — the negative control `_goal/GOAL.md`'s
      Done-when 7 requires, replacing the rendering property Done-when 9
      checked without re-proving scoping (that's task-2's job). A Vitest
      test for `AuthGate`'s no-flash behavior surviving the swap
      (Done-when 6) and one for the todos-CRUD hook (Done-when 10, the
      other half — session-check hook's test is the same test as
      Done-when 6's).
      **Owns: Done-when 6, 7, 10.**
- [ ] task-4: `make test` runs both suites — wires the JS test runner
      (Vitest, matching my-task's own choice) into `make test` alongside
      `go test ./...`, or `docs/GETTING-STARTED.md` states plainly it
      doesn't and what a forker must run instead. Trivial once task-1's
      Vite/npm setup and task-2/3's tests exist; kept as its own task
      rather than folded into task-3 so it's independently checkable
      that the two-suite claim is true, not just that both suites happen
      to exist.
      **Owns: Done-when 9.**
- [ ] task-5: Final verification — **last task, full-suite gate, same
      shape as milestone-2's task-8.** `docker compose up` still works
      with `web/`+`embed.FS` in the build (Done-when 11 — not inherited,
      checked directly, same discipline as milestone-2's own Done-when
      16 note about not assuming relocation/addition didn't break the
      container build). Confirm `go test ./...` and the JS suite are
      both green together, not just independently at the point each was
      last touched — three tasks land code after task-2's endpoints
      exist, any of which could regress what an earlier task's tests
      confirmed.
      **Owns: Done-when 11.**
