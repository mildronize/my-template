# Milestone 3 TODO

See `../_goal/GOAL.md` for objective/scope/decisions/done-when and
`../_contract/API.md` for the new `bff-openapi.yaml` surface. Every
Done-when item is owned by exactly one task.

**Ordering was deliberate while the goal was ungated, and stands even
now that it isn't.** `_goal/GOAL.md`'s two-spec-file split (Decision "API
spec: two files, not one") was one of six Decisions rows Clara flagged to
มายด์ as his to cut. **He has since gated the goal — "for this LGTM" —
and cut nothing. All six stand, including the split.** Task-2 is
unblocked as written; no re-scope needed. The ordering below (task-1
first, split-dependent work starting at task-2) is kept because it's
good ordering on its own terms regardless of the gate having landed —
framework-agnostic work verified before anything contract-shaped — not
because task-2 is still conditionally blocked.

**Human acceptance is a browser, not the suite** (Clara, relaying
มายด์'s own words: *"i'll waiting until milestone-3 is ready to login and
test on my browser"*). Eleven green Done-when items are not the
finish line — an instance he can point a browser at and create a todo
in is. What it takes to get him from nothing to that browser (a running
instance, a registered Hydra client, one command or as close to it as
possible) is part of what "ready" means, not a separate concern to
assemble afterward — see task-5.

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
      **Generate into a distinctly named Go package** (e.g.
      `internal/bffapi`, not `internal/api`) — two independent
      `oapi-codegen` runs each generate their own `Error`,
      `ServerInterface`, `RegisterHandlers`, etc.; sharing a package name
      between them risks a symbol collision that a single-spec project
      never has to think about. Decide the name before generating, not
      after hitting the collision.
      Handlers call `internal/domain/todo.Service` and
      `internal/identity.Service` directly — no new domain logic, per
      `_contract/API.md`'s per-endpoint service-method citations. Every
      write is owner-scoped from the session (I3): a session-authenticated
      request for another owner's todo returns 404, never 403 — **this
      is the first BFF-layer I3 test**, closing the granularity gap
      milestone-2 only documented (its own `TestI3_...` only ever existed
      at the `publicapi` layer). **For the real-session-cookie tests this
      task and Done-when 2 need: reuse the session-seeding helper
      milestone-2's own Done-when-9 test already built** (seeds a signed
      session directly rather than driving a real Hydra login — the same
      shortcut that test's "seeds an owner session + a todo" language
      describes) instead of rediscovering or reinventing it. I2/I12
      boundary reasoning (`_contract/API.md`'s own section) added as a
      code comment at `bff.RequireSession` and at the point BFF writes
      are enabled, plus confirmed present in `API.md`/`INVARIANTS.md`
      (already is, per the contract phase — this task is the presence
      check, not new prose). Negative check: no `POST /api/bff/keys`, no
      rotate endpoint exists anywhere on this surface.
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
      to exist. **JS coverage is deliberately partial (Done-when 10's own
      wording: two specific hooks, not full coverage) — that's a
      decision, not a gap to close here or in task-5's final report.**
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

      **Also owns, not a Done-when item but the actual finish line
      (มายด์, relayed by Clara): confirm the path from "loop done" to
      "มายด์ has a browser pointed at a working instance" is one command,
      or as close to it as this fork's real deployment allows.** `docs/
      GETTING-STARTED.md`'s existing "Running what you forked" section
      already documents `docker compose up` (or `go run ./cmd/server`)
      plus a one-time `register-<service>.sh` against a real Hydra
      client (Step 1) as the path to an owner-login-capable instance —
      confirm that path still holds with `web/`+`embed.FS` in the mix
      (it should, since Done-when 11 already checks `docker compose up`
      works), and that opening `GET /login` in a browser against a
      running instance now actually lets the owner create a todo, not
      just view one. If reaching that instance in practice takes more
      than `docker compose up` plus a URL (an SSH tunnel, a specific
      host, anything Clara had to assemble by hand for milestone-2's
      acceptance), that's the gap to close or hand to Clara explicitly
      before reporting done — not something to leave for her to
      re-discover the way she had to last time.
      **Owns: Done-when 11.**
