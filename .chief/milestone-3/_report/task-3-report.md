# Task task-3 Report

## Task
Wire the SPA for real: `openapi-typescript` types + a thin fetch wrapper
against `bff-openapi.yaml`, swap `web/src/lib/auth-client.ts` for a real
TanStack Query hook against `GET /api/bff/me`, replace `TodosPage.tsx` and
`ApiKeySettingsPlaceholder.tsx`'s placeholder content with real CRUD
screens, build the static login page, finish the root layout, remove
`internal/transport/bff/view_handler.go` + its test, and write the three
Vitest tests `_goal/GOAL.md` requires (Done-when 6, 7, 10).

## Outcome
done

## Decision

- **Login page routing.** `_goal/GOAL.md`'s Router decision says
  "react-router, client-side, two real routes (`/`, `/settings`)", but
  its own Screen scope decision lists Login as one of three in-scope
  screens, and this task's own spec explicitly says to build it. I
  traced the actual topology: `cmd/server/main.go`'s `wireBFF` registers
  an explicit `GET /login` on the same gin engine that serves the SPA
  via `NoRoute` — gin always prefers an explicit route, so **a real
  browser navigating to `/login` (including `AuthGate.tsx`'s own
  `window.location.replace("/login...")` redirect) never reaches the
  SPA's bundle at all**, in dev (Vite's proxy also forwards `/login` to
  Go) or in production. A client-side `<Route path="/login">` is
  therefore only ever reachable via a hypothetical future
  `<Link to="/login">` from inside the already-loaded SPA — none exists
  today.
  Options considered: (a) skip the route entirely, treating "two real
  routes" as authoritative; (b) wire it anyway, outside
  `AppLayout`/`AuthGate` so it can't loop; (c) block and ask.
  Chosen: (b). Built the static page (card + `<a href="/login">`, no
  client-side OAuth) and registered it as a third `<Route>`, structurally
  outside `AppLayout` so it never gets wrapped in `AuthGate`. Documented
  the reachability nuance directly in the file's own comment rather than
  silently dropping explicitly-requested work or silently contradicting
  an explicit Decisions-table line — a future task/forker reading the
  file sees why it's there and why it's currently inert in normal use.

- **`RequireSession` (middleware.go) left in place, not removed.**
  Deleting `view_handler.go` removes `RequireSession`'s only production
  caller and its only direct behavioral test (all four of them lived in
  the now-deleted `view_handler_test.go`) — it's now genuinely unused
  and untested code. The task's own instruction was narrower ("check
  `main.go`'s `wireBFF` for any now-genuinely-dead reference to it... and
  clean that up"), not "remove `RequireSession` itself," and its doc
  comment carries the I2/I12 boundary reasoning other files still point
  to (`json_middleware.go`, `cmd/server/main.go`). Removing it would mean
  re-anchoring that reasoning somewhere else, which felt like scope
  creep for a function nothing asked me to delete. Left it in place,
  updated the one doc comment (`ActorFromContext`) that specifically
  named the now-deleted file, and flag this here rather than let it be
  a silent surprise: **`RequireSession` has zero callers and zero direct
  tests as of this commit** — worth a future task either wiring it to a
  real use, giving it its own direct test, or removing it outright.

- **`GET /api/bff/me`'s non-200 handling.** The contract says "any
  non-200 means no session," which taken completely literally would
  include a genuine 5xx/network-shaped failure. I read this together
  with `AuthGate.tsx`'s own no-flash requirement (a transient failure
  must never bounce a previously-authenticated user) and concluded the
  two are compatible only because `GET /api/bff/me` never actually
  returns anything but 200 or 401 (`bff-openapi.yaml`'s own spec: no
  other status is documented) — so in practice "non-200" and "401" are
  the same set, and a *transport*-level failure (`fetch()` itself
  rejecting — offline, server restart mid-request) is a different kind
  of event than a non-200 response and is deliberately left to throw
  into TanStack Query's `error` state instead of being folded into
  "no session." This is what `AuthGate.test.tsx` exercises directly.

## Notes

- **Ported from `origin/main` @ `fdceb8befbcebfa25d2216d71264bb7c5e8c96d7`**
  — reconfirmed via `git rev-parse origin/main` in `~/gits/my-task`
  before reading anything (unchanged since task-1/task-2, matches
  `GOAL.md`'s recorded hash). Read every file with `git show
  origin/main:<path>`; the working tree at `~/gits/my-task` was never
  checked out or modified.
  - `src/app/(app)/tasks/page.tsx` + `NewTaskDialog.tsx` →
    `TodosPage.tsx`/`TodosList.tsx`/`TodoRow.tsx`/`NewTodoDialog.tsx`:
    same list/create-dialog JSX shape, project/assignee/status-group
    filtering and pagination dropped (not this template's domain — a
    todo has title/done/createdAt/updatedAt only, per
    `bff-openapi.yaml`'s `Todo` schema).
  - `src/app/(app)/settings/api-key-settings.tsx` → `ApiKeySettings.tsx`:
    list + revoke only, no "show revoked" toggle (`GET /api/bff/keys`
    already excludes revoked keys) and no idle-key warning (`ApiKey` has
    no `lastRequest` field on this surface).
  - `src/app/layout.tsx` → `index.html` + `main.tsx`: `next/font` →
    real Google Fonts `<link>` tags, `Metadata` → a plain `<title>`,
    `TRPCReactProvider` → `QueryClientProvider` (`lib/query-client.ts`).
  - `src/app/login/page.tsx`: NOT ported (its OAuth-driving logic is
    exactly what this template doesn't need) — kept only the card/button
    visual layout, replaced the `onClick` handler with a real
    `<a href="/login">` navigation. See the Decision above on why this
    route is structurally present but practically unreachable via a
    real browser navigation in this deployment.
- **`AuthGate.tsx` and `Header.tsx`**: zero logic changes, as required.
  `Header.tsx` did get one non-logic edit — `NAV_LINKS` (was
  Activity/Tasks/Projects, my-task's own routes) and the logo text (was
  "My Task") updated to this template's actual domain nouns (a single
  "Todos" link, "My Template"). Task-1's own report explicitly flagged
  this as content task-3 should revisit; not a change to any
  auth-consuming logic in either file.
- **`GET /api/bff/me`'s response shape**, confirmed against a live
  instance: `{"handle":"...","role":"owner","active":true}`. Since
  `AuthGate`/`Header` expect `session.user.{name,email}` (better-auth's
  shape, unchanged since that's what they're written against),
  `auth-client.ts` maps `handle` onto both `user.name` and `user.id`,
  and `role` onto `user.email` (the only other field available — shown
  as the popover's secondary line). Documented directly in the file.
- **`signOut()` stays a no-op.** No session-clearing endpoint exists on
  `/api/bff` (task-2's own scope was `GET /me` + todo/key CRUD only) —
  per this task's own instructions, inventing one is out of scope.
  `Header.tsx`'s `handleSignOut` still runs and navigates to `/`
  afterward; since the cookie is untouched, this currently just leaves
  the owner signed in. Flagging for whichever future milestone adds a
  real logout endpoint.
- **`web/dist/.gitkeep`**: `npm run build` empties the whole `dist/`
  directory (Vite's default `emptyOutDir`), which deletes the tracked
  placeholder from the working tree every time a real build runs.
  Restored it (`touch`) before every commit in this task, same as
  task-1's own report describes doing after its stale-embed check.

## Verification

- `go build ./...`, `go vet ./...`, `gofmt -l .` — all clean.
- `go test ./...` — all packages green, including
  `internal/transport/bff`'s full suite post-removal (the first BFF-layer
  I3 test, the real-session round trip, the negative check, the 401-not-
  redirect test — all task-2's, unaffected) and `cmd/server`'s own tests.
  Re-verified on a genuinely fresh `git clone` (not the working tree) —
  `go build`/`go vet`/`gofmt -l .`/`go test ./...` all green with
  `web/dist/` holding only the tracked `.gitkeep`.
- `web/`: `npx tsc -b --noEmit` clean; `npm run build` clean
  (`dist/index.html` 1.13 kB, JS bundle 475.86 kB / gzip 147.73 kB);
  `npx vitest run` — 3 files, 5 tests, all passing. Re-verified on the
  same fresh clone above (`npm ci && npm run build` and `npx vitest run`
  both clean with zero pre-existing `node_modules`/`dist`).
- **Real round trip against a live instance** (not `httptest`, not a
  mocked fetch): built the real SPA (`make build`), ran the real server
  binary against a fresh SQLite file, seeded a real owner user + a real
  signed session cookie via `bff.Signer.NewSessionCookie` (the same
  session-seeding shortcut task-2's own tests use — a throwaway
  `cmd/manualverify-task3` tool, built and removed, never committed, same
  pattern task-2's own `cmd/manualverify` established). Curled the exact
  request shapes the SPA's own hooks issue (same paths, methods, bodies,
  `credentials: "include"`):
  - unauthenticated `GET /api/bff/me` → **401** (a real one, not mocked —
    this is the trigger `AuthGate`'s real redirect-to-`/login` path
    depends on; the no-flash-on-transient-error half is what
    `AuthGate.test.tsx` covers with a mocked fetch, so this closes the
    other half the report asked for explicitly).
  - authenticated `GET /api/bff/me` →
    `{"handle":"manual-verify-owner-task3","role":"owner","active":true}`.
  - `POST /api/bff/todos {"title":"buy milk"}` → `201`, then `GET
    /api/bff/todos` → lists exactly that todo (create → see it appear).
  - `PATCH .../{id} {"done":true}` → `200 done:true`; `PATCH .../{id}
    {"title":"buy oat milk"}` → `200`; `GET .../{id}` again → both edits
    persisted (toggle-done and edit-title, both wired).
  - `DELETE .../{id}` → `204`; `GET .../{id}` again → `404`; `GET
    /api/bff/todos` → empty again (delete actually removes the row).
  - `GET /api/bff/keys` → `{"keys":[]}` (`ApiKeySettings`'s own query
    shape).
  - `POST /api/bff/keys` → still `404` (task-2's own negative check,
    reconfirmed live post-SPA-rebuild — the embedded-SPA `NoRoute`
    fallback still doesn't swallow it).
  - `GET /` served the real built `index.html` (`<title>My Template</title>`,
    the real Google Fonts `<link>`); `GET /settings` → `200` via the SPA
    fallback.
  - Grepped the actual built JS bundle for the literal strings the fetch
    wrapper/hooks construct at runtime: `/api/bff`, `"/todos"`, `"/keys"`,
    `/api/bff/me`, `credentials:"include"` — all present, confirming the
    shipped artifact (not just source) calls the right endpoints.
- `docker compose build` succeeds; `docker compose up` (port temporarily
  remapped to `18080:8080` to work around an unrelated process already
  bound to `8080` in this sandbox, reverted after — `git diff
  docker-compose.yml` is clean) — confirmed `GET /healthz` → `200`,
  `GET /` → real SPA title, `GET /settings` → `200`, `GET /api/bff/me`
  unauthenticated → `401`, `GET /api/v1/me` unauthenticated → `401`
  (publicapi unaffected).

## Commits pushed (branch `milestone-2/close-parity-gap`)

- `84c842d` — `feat(milestone-3/task-3): remove internal/transport/bff/view_handler.go, retired by the SPA`
- `a22db4b` — `feat(milestone-3/task-3): generate BFF types via openapi-typescript, add Vitest tooling`
- `5b6b55d` — `feat(milestone-3/task-3): wire the real auth-client, todos, and keys hooks against the BFF`
- `95041f3` — `feat(milestone-3/task-3): wire the real todos and settings screens, login page, root layout`
- `c2e1388` — `feat(milestone-3/task-3): add Vitest suite -- todos list negative control, AuthGate no-flash, todos CRUD hooks`

## For task-4/task-5

- `make test` still only runs `go test ./...` — untouched, per this
  task's own "don't wire `make test`" instruction. `npx vitest run` (or
  `npm run test`) is the JS suite's entry point; task-4 owns wiring it
  in (or documenting that it doesn't) per `_goal/GOAL.md` Done-when 9.
- `RequireSession` (`internal/transport/bff/middleware.go`) is now
  unused in production code and has no direct test of its own (see
  Decision above) — not blocking anything, but worth a look before this
  milestone's final report calls the tree fully tidy.
- The `/login` SPA route exists but is not reachable via a real browser
  navigating to `/login` in this deployment (Go's own `GET /login`
  claims that exact path unconditionally, ahead of the SPA's `NoRoute`
  fallback) — see the Decision above. Not a bug to fix; just worth
  knowing before anyone goes looking for why it "never renders."
- Real end-to-end verification (build → run → curl the exact SPA request
  shapes) is captured above; the one piece genuinely deferred to a human
  per `_goal/GOAL.md`'s own "Human acceptance" section is a real browser
  clicking through a real Hydra consent screen — task-5/มายด์'s own step,
  not reattempted here.

## Fix-round note (post-verification, 2026-08-13)

A verification pass found three small issues in the above. All three are
fixed on `milestone-2/close-parity-gap`.

- **`AuthGate.test.tsx` had a real coverage gap.** The Done-when-6 test
  (`does not redirect ... when a confirmed session later fails
  transiently`) only exercises `confirmedLoggedOut`'s `session === null`
  clause: `auth-client.ts`'s `useSession()` does `data: query.data ??
  null`, and TanStack Query retains the last successful `data` across a
  failed background refetch, so once a check has ever succeeded, `session`
  never goes back to `null` just because a *later* check errors. That left
  the guard's `!error` clause provably unexercised — confirmed by
  temporarily deleting `!error` from `AuthGate.tsx` and watching the
  existing test still pass. `!error` is load-bearing exactly once: a
  *first-ever* session check that fails transiently, before any check has
  succeeded, where `session` genuinely is `null` (nothing cached yet) and
  `error` is truthy — without it, a user with a perfectly valid session
  could be bounced to `/login` by nothing more than a first-request
  network blip.
  Added two tests to `AuthGate.test.tsx` (did not touch `AuthGate.tsx`
  itself, whose logic was already correct): a first-ever transient
  failure must **not** redirect, and — as the contrast case proving the
  guard distinguishes "flaky" from "confirmed logged out," not just that
  it never redirects — a first-ever *genuine* no-session response (a real
  non-200 `Response`, not a rejected `fetch`) still **must** redirect.
  Adding a second/third test in the same `describe` block also surfaced a
  latent test-isolation gap: `vitest.config.ts`'s `globals: false` means
  `@testing-library/react`'s automatic `afterEach` cleanup never
  registers, so a prior test's rendered DOM was leaking into the next
  test's queries. Added an explicit `cleanup()` call to this file's own
  `afterEach` to fix it — scoped to this file; the other two test files
  (`todos.test.tsx`, `TodosList.test.tsx`) each have only one test per
  `describe` block today so the gap doesn't bite them yet, but it's worth
  keeping in mind if either grows a second test.
  Verified by attack: reintroduced the `!error` mutation, confirmed via
  `tsc -b` that it still compiles (had to add a throwaway `void error;` to
  dodge `noUnusedLocals`, since removing the clause alone made TypeScript
  refuse to build for an unrelated reason), confirmed the new first-load
  test — and only that one — now fails, then reverted and confirmed clean.

- **Deleted `RequireSession` (`internal/transport/bff/middleware.go`).**
  The original report above flagged this as dead code left in place out
  of caution ("felt like scope creep for a function nothing asked me to
  delete"). This fix-round's task explicitly asked for the removal, so:
  deleted the function and its doc comment. Its I2/I12 boundary reasoning
  was real and worth keeping, so the full paragraph moved into
  `RequireJSONSession`'s own doc comment (`json_middleware.go`), replacing
  the condensed version and the now-dangling pointer to `RequireSession`'s
  comment. Updated every other comment that named `RequireSession`
  (`session.go`'s `errInvalidCookie`, `todo_handler_test.go`, `main.go`'s
  `/api/bff` wiring comment, `ActorFromContext`) so nothing in the tree
  points at deleted code.
  Verified by removing, not by inspection: `go build ./...`, `go vet
  ./...`, and `go test ./...` all pass clean with `RequireSession` gone.
  **Nothing depended on it** — the deletion touched no other production
  behavior; `middleware_test.go` (which does still exist) tests
  `secureFromURL`, an unrelated function in the same file, and was
  untouched.

- **`web/package.json`'s `engines` field.** Turned out `web/package.json`
  had **no `engines` field at all** — the fix-round task's premise that it
  "currently declares `^22.22.2 || ^24.15.0 || >=26.0.0`" was tracing a
  *transitive* dependency's own requirement (`jsdom@30.0.1`'s
  `package.json`, confirmed via `package-lock.json`; `@redocly/openapi-
  core@1.34.19`'s `>=18.17.0` floor also warns) rather than this project's
  own field. Added `"engines": { "node": ">=22.22.1" }` to
  `web/package.json` anyway — it's the actually-verified floor and a
  correct, harmless addition — but flagging plainly: **this does not, and
  cannot, eliminate the `npm install` `EBADENGINE` warnings**, since those
  come from `jsdom`'s and `@redocly/openapi-core`'s own declared engine
  requirements, which npm checks independently of this project's own
  `engines` field. Fresh-clone `npm install` below still shows both
  warnings (installed Node `22.22.1` is one patch version short of
  `jsdom`'s `^22.22.2` floor). Fixing that for real would mean upgrading
  Node past `22.22.2` or pinning `jsdom` to an older, looser-range
  version — both out of this fix's scope (touching dependency versions
  wasn't asked for, and downgrading a test-only dependency has its own
  tradeoffs) — so this is left as a known, non-blocking pre-existing
  condition for a future task to pick up if it matters.

### Fresh-clone verification

New `git clone`, `milestone-2/close-parity-gap` @ `541202e`,
`web/dist/` holding only the tracked `.gitkeep`:

- `go build ./...`, `go vet ./...`, `gofmt -l .` — all clean.
- `go test ./...` — all packages green (`internal/transport/bff` included).
- `web/`: `npm install` — succeeds; still shows the two `EBADENGINE`
  warnings described above (`jsdom`, `@redocly/openapi-core` — both
  pre-existing, both unrelated to this project's own `engines` field).
  `npx tsc -b` clean. `npm run build` clean (`dist/index.html` 1.13 kB,
  JS bundle 475.86 kB / gzip 147.73 kB). `npx vitest run` — **3 files, 7
  tests, all passing** (`AuthGate.test.tsx` grew from 1 test to 3;
  `todos.test.tsx` and `TodosList.test.tsx` unchanged at 3 and 1).

### Commits pushed (branch `milestone-2/close-parity-gap`)

- `6c38c84` — `fix(milestone-3/task-3): close AuthGate's untested first-load transient-failure gap`
- `5557095` — `fix(milestone-3/task-3): delete dead RequireSession middleware`
- `541202e` — `fix(milestone-3/task-3): set web/package.json's engines.node to a verified floor`
