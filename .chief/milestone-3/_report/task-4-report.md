# Task task-4 Report

## Task
Wire `make test` to run both the Go suite and the JS suite (Vitest,
`web/`), so a green `make test` actually means both are green rather than
`go test ./...` standing in for a JS suite that never runs — verified
from a genuinely fresh clone, with `docs/GETTING-STARTED.md` updated to
match whatever `make test` actually does. `_goal/GOAL.md` Done-when 9.

## Outcome
done

## Decision

- **Wired it in, didn't fall back to documenting an opt-out.** Task-3
  already left `web/vitest.config.ts`'s own comment saying wiring was
  "deliberately NOT" done, reserved for this task, and `web/package.json`
  already ships a `"test": "vitest run"` script — there was no real
  friction to route around. `web/node_modules` isn't guaranteed on a
  fresh clone, so `make test`'s own recipe now runs `npm ci` first (same
  reproducibility reasoning `web-build` already uses for the same
  problem: `web/package-lock.json` is the source of truth, `npm ci`
  refuses to proceed on drift rather than silently resolving something
  new) — the "or, make it self-contained" branch of the task spec, not
  an either/or against wiring it in.
- **New `web-test` target, not an inline second command in `test`.**
  Mirrors `web-build`'s own split (independently invokable, e.g. from CI
  steps that want just the JS suite) rather than writing
  `cd web && npm ci && npm test` directly inside `test`'s recipe.
  `test: web-test` then `go test ./...` — either suite failing fails
  `make test` overall, since `make` stops at the first non-zero exit.
- **Docs land in Prerequisites, right after the existing Node paragraph**,
  not a new section. That's already where a forker learns Node is
  required at all and why (`make build`'s `web-build` step) — the natural
  place to also learn what running the test suite costs, rather than a
  separate "Testing" heading nothing currently points at.
- **Did not touch `go test ./...`'s many direct mentions in Step 5** (the
  todo-domain-replacement walkthrough, lines ~473–690). Those are all
  legitimately about running the raw Go command directly as a fork-step
  checkpoint (proving Go wiring compiles/passes independent of the
  domain's shape) — not claims about `make test`'s two-suite coverage,
  so rewording them wasn't in scope and would have been unrelated churn.
- **Updated `web/vitest.config.ts`'s own comment**, which explicitly said
  "Deliberately NOT wired into `make test` — that's task-4's job" —
  left uncorrected, it would have been the exact kind of stale claim
  this milestone's own goal keeps calling out (a comment asserting
  something the tree no longer does). Now says it's wired in, when, and
  why, still pointing at the direct `npm test` entry point for anyone
  iterating on `web/` alone.
- **Precision on "both suites green," per the task's own Done-when-10
  framing check**: the doc sentence explicitly says the JS suite covers
  "only the two hooks that replaced tRPC-coupled logic... not all of
  `web/`" in the same breath as stating `make test` runs both — so
  "make test runs both suites" can't be misread as "make test proves
  full JS coverage." No new JS tests were written; task-3's three test
  files (7 tests total) are exactly what `make test` now runs.

## Notes

- Encountered an unrelated, uncommitted change already sitting in the
  working tree when I started (`web/src/App.tsx`, `web/vite.config.ts`
  modified, `web/src/app/login/page.tsx` deleted, `web/dist/.gitkeep`
  deleted) — not mine, not part of this task's scope (SPA screens/
  routing, explicitly off-limits per this task's own instructions).
  `git fetch` confirmed `origin/milestone-2/close-parity-gap` matched my
  local `HEAD` before I started, so this was local, uncommitted,
  presumably a concurrent session's in-progress work on this same shared
  working tree. Staged and committed only the three files this task
  touched (`git add Makefile docs/GETTING-STARTED.md
  web/vitest.config.ts` by name, not `-A`) so that work was left
  untouched in the working tree, not swept into this task's commit.
  Fresh-clone verification below used a separate `git clone` of `origin`,
  so it was unaffected by that dirty state either way. That work landed
  on its own shortly after, as `708daec` (`fix(milestone-3/task-3):
  delete the SPA's dead /login route`) and `af85c89` documenting it —
  both a concurrent session's commits, not this task's; my local working
  tree fast-forwarded onto them (clean, no conflict) while I was running
  the fresh-clone verification below. `e7547a1` (this task's commit)
  sits directly under both in the branch's history.
- The pre-existing `EBADENGINE` warnings task-3's report already
  documented (`jsdom@30.0.1` wanting `^22.22.2`, `@redocly/openapi-
  core@1.34.19` wanting `>=18.17.0`, against installed Node `22.22.1`)
  still print during `npm ci` — non-blocking, unrelated to this task,
  left as-is per task-3's own note that fixing it is out of scope.

## Verification

- `go build ./...`, `go vet ./...`, `gofmt -l .` — clean, in the working
  tree.
- **Fresh clone** (`git clone https://github.com/mildronize/my-template.git`
  into a new scratch directory, `git checkout milestone-2/close-parity-gap`
  @ `e7547a1`, confirmed `web/node_modules` and `web/dist` absent before
  running anything):

  ```
  $ make test
  cd web && npm ci && npm test
  npm WARN EBADENGINE Unsupported engine { package: '@redocly/openapi-core@1.34.19', ... }
  npm WARN EBADENGINE Unsupported engine { package: 'jsdom@30.0.1', ... }

  added 267 packages, and audited 268 packages in 4s
  found 0 vulnerabilities

  > my-template-web@0.1.0 test
  > vitest run

   RUN  v4.1.10 .../web

   Test Files  3 passed (3)
        Tests  7 passed (7)
     Start at  08:24:26
     Duration  3.03s

  go test ./...
  ok  	github.com/mildronize/my-template/cmd/issue-key	0.046s
  ok  	github.com/mildronize/my-template/cmd/seed	0.020s
  ok  	github.com/mildronize/my-template/cmd/server	0.061s
  ?   	github.com/mildronize/my-template/cmd/smoke	[no test files]
  ?   	github.com/mildronize/my-template/db/migrations	[no test files]
  ok  	github.com/mildronize/my-template/internal	2.138s
  ?   	github.com/mildronize/my-template/internal/api	[no test files]
  ?   	github.com/mildronize/my-template/internal/bffapi	[no test files]
  ?   	github.com/mildronize/my-template/internal/db	[no test files]
  ?   	github.com/mildronize/my-template/internal/dbquery	[no test files]
  ok  	github.com/mildronize/my-template/internal/domain/todo	0.049s
  ok  	github.com/mildronize/my-template/internal/identity	1.645s
  ok  	github.com/mildronize/my-template/internal/platform	0.024s
  ok  	github.com/mildronize/my-template/internal/transport/bff	2.110s
  ok  	github.com/mildronize/my-template/internal/transport/publicapi	0.125s
  ?   	github.com/mildronize/my-template/web	[no test files]
  ?   	.../web/node_modules/flatted/golang/pkg/flatted	[no test files]
  ```

  One `make test` invocation, zero manual setup beyond the clone itself,
  both suites reported green.
- Also re-ran `go build ./...` / `go vet ./...` / `gofmt -l .` in that
  same fresh clone — all clean.
- `docker compose up` sanity check, same fresh clone (port remapped
  `18080:8080` locally to dodge an unrelated process already bound to
  `8080` in this sandbox, same workaround task-3's report used — not
  committed, `docker-compose.yml`'s tracked content is untouched):
  `docker compose build` succeeded, `docker compose up -d` started
  cleanly, `GET /healthz` → `200`, `GET /` → `200` (served SPA), `GET
  /api/bff/me` unauthenticated → `401`. Logs show migrations applying
  (`create_users_and_api_keys`, `create_todos`) and the expected
  SSO/session warnings (unconfigured in this compose file by design, per
  its own comments). `docker compose down -v` + image removal after.

## Commits pushed (branch `milestone-2/close-parity-gap`)

- `e7547a1` — `feat(milestone-3/task-4): wire make test to run both Go and JS suites`

## For task-5

- `make test` now runs both suites end to end; task-5's own "confirm
  `go test ./...` and the JS suite are both green together" Done-when
  can point at this one command rather than two separately-run ones.
- The `/login` route removal noted above (`708daec`) landed as a
  concurrent task-3 fix-round commit, not authored or evaluated for
  correctness by this task — flagging only that it's now committed
  history, not left as in-progress/uncommitted state the way it briefly
  was while this task was running.
- `RequireSession`'s removal and the other fix-round items task-3's
  report already flagged as resolved; nothing new found here that
  affects them.
