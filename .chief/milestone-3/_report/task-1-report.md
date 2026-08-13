# Task task-1 Report

## Task
Scaffold `web/` as a Vite + React 19 SPA, port my-task's design system and
app shell into it verbatim (from `origin/main`, not the checked-out
feature branch), wire it into the Go binary via `embed.FS`, and update
the fork docs — laying the ground truth-independent foundation task-2/3
build on.

## Outcome
done

## Decision

- Issue: task-1's spec says to serve the embedded SPA "at `/` for any
  path not already claimed by `/api/v1` or the existing `bff` routes
  (`/login`, `/callback`)" — but `internal/transport/bff/view_handler.go`
  (which task-1 is explicitly told not to touch) already registers an
  explicit `GET /` on the same `bffRouter`, gated behind `RequireSession`.
  gin always prefers an explicit route over `NoRoute`, so leaving that
  registration in place would make `/` permanently unreachable for the
  SPA — the stale-embed verification (curl `/`) would just hit the old
  view's 302-redirect-to-`/login` for an unauthenticated request, never
  the embedded `index.html`.
- Options considered: (a) restructure the top-level `http.ServeMux`
  dispatch in `main.go` to route `/` to the SPA and `/login`+`/callback`
  explicitly to `bffRouter`, leaving `bffRouter`'s own `GET /` untouched
  but unreachable; (b) remove just the one `router.GET("/", ...)`
  registration line from `wireBFF` (`cmd/server/main.go`) and add
  `router.NoRoute(gin.WrapH(spaHandler))` instead, leaving
  `view_handler.go` itself and its test completely untouched; (c) ask
  and block on task-1 until clarified.
- Chosen: (b). It's the smaller, more literal read of the spec text (a
  gin engine's own `NoRoute` is "any path not already claimed by its
  explicit routes," and the spec's claimed-routes list conspicuously
  omits `/` from what bff still owns) and it satisfies "do not touch
  `view_handler.go` or its test" to the letter — I only edited
  `cmd/server/main.go`'s composition, not the `bff` package.
  `view_handler_test.go`/`bff_testutil_test.go` build their own router
  directly (`newTestRouter`, `bff_testutil_test.go`), independent of
  `main.go`'s wiring, so removing that one registration line doesn't
  touch their coverage at all — confirmed by `go test ./...` staying
  green. `NewViewHandler` and `todoSvc`'s parameter to `wireBFF` are now
  unused by that function but still compile (Go allows unused
  parameters); left in place with a comment pointing at task-3, which
  owns `view_handler.go`'s actual removal.

## Notes

- **Ported from `origin/main` @ `fdceb8befbcebfa25d2216d71264bb7c5e8c96d7`**
  — verified via `git rev-parse origin/main` in `~/gits/my-task` before
  reading anything (matches `GOAL.md`'s own recorded hash exactly, main
  hasn't moved). Read every file with `git show origin/main:<path>`,
  never checked that ref out — `~/gits/my-task`'s working tree was left
  untouched throughout.
- **`AuthGate`'s stub state**: `web/src/lib/auth-client.ts` is a new file
  (not a port — my-task's real `auth-client.ts` wraps better-auth, which
  this template drops entirely). It exports a `useSession`/`signOut` pair
  matching better-auth's `{ data, isPending, error }` shape, always
  returning a placeholder signed-in session. Both `AuthGate.tsx` and
  `Header.tsx` (bucket 1/2, ported near-verbatim) import from this same
  module path, so task-3 can replace just this one file with a real
  TanStack Query hook against `GET /api/bff/me` once task-2 builds it —
  neither `AuthGate.tsx` nor `Header.tsx` should need further edits at
  that point, only the import's target module's own implementation
  changes underneath them.
- **`embed.FS` wiring shape**: the `//go:embed all:dist` directive lives
  in a new `web/embed.go` (package `web`), not in `cmd/server` — a
  `go:embed` pattern can't cross a `..` to reach `web/dist` from
  `cmd/server/`, so the directive has to physically live inside `web/`.
  `cmd/server/spa.go`'s `newSPAHandler()` wraps `web.DistFS` with the
  standard SPA fallback (real file → served as-is; anything else →
  `index.html`, letting react-router own client routing) and is mounted
  via `bffRouter.NoRoute(gin.WrapH(spaHandler))` in `wireBFF`.
  `web/dist`'s real build output is gitignored; a tracked
  `web/dist/.gitkeep` placeholder is what keeps `go build ./...`/
  `go vet ./...`/`go test ./...` green on a fresh clone that has never
  run `npm` — verified directly against a fresh `git clone` of the pushed
  branch (see Verification below).
- **Surprise from my-task's source**: `Header.tsx`'s `NAV_LINKS` still
  reads Activity/Tasks/Projects — my-task's own nav, pointing at routes
  this template doesn't have. Bucket 2's instruction for this file is
  "swap `Link`/`usePathname` only, nothing else changes," so I left it
  exactly as-is rather than trimming it to `/`+`/settings` — flagged in
  `docs/GETTING-STARTED.md`'s new Step 3b as one of the domain-noun spots
  a forker (and presumably task-3) still needs to revisit.
- `globals.css` was ported byte-for-byte (it's effectively Tailwind v4's
  CSS-first config — the "Tailwind config" GOAL.md's scope names). It
  references `--font-manrope`/`--font-fraunces`, which my-task's root
  `layout.tsx` sets via `next/font` — a bucket-3 file task-1 doesn't
  touch. Added a small `web/src/styles/fonts.css` stopgap defining those
  two vars as system-font fallbacks, clearly commented as task-3's to
  replace when it rewrites the root layout/`index.html`.
- `web/dist/`, `web/node_modules/` are excluded from the Docker build
  context (`.dockerignore`) so a host machine's own locally-installed
  `node_modules` (which can carry platform-specific native bindings) or
  stale local `dist` can never leak into either Docker stage.

## Verification

- `go build ./...` — clean.
- `go vet ./...` — clean.
- `gofmt -l .` — no output (clean).
- `go test ./...` — all packages green (including
  `internal/transport/bff`'s existing view-handler/session tests,
  unaffected by the `main.go` routing change — see Decision above).
- `web/` builds cleanly: `npm install && npm run build` (also exercised
  via `npm ci && npm run build`, the Makefile's actual `web-build`
  target) — `tsc -b` (strict) then `vite build`, no errors.
- **Stale-embed check, exact method**: built once with
  `web/index.html`'s `<title>` = `"my-template"` and
  `TodosPage.tsx`'s heading = `"Todos"`; ran the resulting binary and
  `curl`'d `/`, confirming that baseline title and JS asset hash
  (`index-DZx32mOf.js`). Changed both strings to include a unique marker
  (`STALE_EMBED_CHECK_9f21c4`), deleted `web/dist`'s built output (clean
  state), ran `make build`, started the new binary, and `curl`'d `/`
  again: the new title/marker appeared in the served `index.html`, the
  new (differently-hashed) JS bundle fetched via the path `index.html`
  itself referenced contained the marker text, and requesting the *old*
  asset's hashed path now hit the SPA fallback (served `index.html`, not
  the old JS) instead of the old content. Reverted the marker strings
  back to clean placeholder text afterward and rebuilt once more to leave
  the tree in its intended final state.
- **`docker compose up`**: `docker compose build` succeeds (new
  `web-build` Docker stage runs `npm ci && npm run build` before the Go
  build stage, whose `COPY --from=web-build ... ./web/dist` overwrites
  the tracked placeholder with the real build). `docker compose up`
  starts the container (host port temporarily remapped to work around an
  unrelated process already bound to 8080 in this sandbox — reverted
  after, `git diff` on `docker-compose.yml` is clean); confirmed
  `GET /healthz` → `200 {"status":"ok"}`, `GET /` → SPA `index.html` with
  the real (non-placeholder) title, `GET /settings` → `200` via the SPA
  fallback, `GET /api/v1/me` → `401` (still routed to `publicapi`,
  unaffected by the `bff` routing change).
- **Fresh-clone sanity pass**: `git clone` of the pushed branch into a
  scratch directory, `go build ./...`/`go vet ./...`/`go test ./...` all
  green with `web/dist/` containing only the tracked `.gitkeep` (never
  ran `npm` first) — proves the placeholder mechanism actually holds for
  a real clone, not just my working tree. `make build` from that same
  fresh clone also succeeded end to end.

## Commits pushed (branch `milestone-2/close-parity-gap`)

- `cb153ca` — `feat(milestone-3/task-1): scaffold web/ as Vite + React 19, port my-task's design system`
- `b9b98d8` — `feat(milestone-3/task-1): embed web/dist into cmd/server, build web/ before Go in make build and Docker`
- `12ca4ad` — `feat(milestone-3/task-1): document Node prerequisite, Vite dev-proxy mode, React-app rename step`
