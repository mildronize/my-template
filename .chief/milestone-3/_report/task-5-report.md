# Task task-5 Report

## Task
Final verification — last task, full-suite gate, same shape as
milestone-2's task-8. Confirm both suites are green together from a
genuinely fresh clone; confirm `docker compose up` still works with
`web/`+`embed.FS` in the mix (Done-when 11); confirm the pinned-port
`/callback` routing check (the newest, most specific requirement); confirm
the documented "one command to a browser" finish line is accurate; confirm
the real write path works end to end against the actual final state of the
branch. Do all live verification on a scratch port — port 8080 is held by
Clara's `mt-server` for มายด์'s milestone-2 acceptance, and is explicitly
off-limits to touch.

## Outcome
done

## Boundary compliance (read first)

- **Port 8080 was occupied for this entire task** by `mt-server`
  (pid 1304524), confirmed at the start (`ss -ltnp | grep :8080`) and again
  at the end, same pid both times — never touched, never killed, never
  interfered with.
- All live verification below ran on scratch ports (`18180` for the Docker
  check, `18181` for the direct-binary CRUD/routing check), in a separate
  `git clone` under a scratchpad directory, never in `~/gits/my-template`'s
  own working tree for the live-process parts.
- **No real browser, no real Hydra flow was attempted anywhere in this
  task.** Every check below is machine-checkable by design; the actual
  browser click-through is explicitly left to มายด์, per `_goal/GOAL.md`'s
  "Human acceptance — after the loop, not part of it" section.

## Check 1 — both suites green together, from a genuinely fresh clone

```
$ git clone ~/gits/my-template fresh-clone && cd fresh-clone
$ git checkout milestone-2/close-parity-gap
$ git log --oneline -1
b104acb fix(milestone-3/task-4): document fix-round in task-4 report
$ ls web/node_modules   # confirm absent
ls: cannot access 'web/node_modules': No such file or directory
$ make test
cd web && npm ci && npm test
...
 Test Files  3 passed (3)
      Tests  7 passed (7)
go test github.com/mildronize/my-template/cmd/issue-key ... /internal/transport/bff .../internal/transport/publicapi .../web
ok    .../cmd/issue-key      0.059s
ok    .../cmd/seed           0.023s
ok    .../cmd/server         0.060s
?     .../cmd/smoke          [no test files]
?     .../db/migrations      [no test files]
ok    .../internal           1.895s
?     .../internal/api       [no test files]
?     .../internal/bffapi    [no test files]
?     .../internal/db        [no test files]
?     .../internal/dbquery   [no test files]
ok    .../internal/domain/todo    0.047s
ok    .../internal/identity       0.942s
ok    .../internal/platform       0.017s
ok    .../internal/transport/bff        1.717s
ok    .../internal/transport/publicapi  0.125s
?     .../web                [no test files]
$ echo "make test exit code: $?"
make test exit code: 0
$ go build ./... ; echo "exit: $?"
exit: 0
$ go vet ./... ; echo "exit: $?"
exit: 0
$ gofmt -l . ; echo "exit: $?"
exit: 0
```

Both suites green together, `make test` exit 0, from a fresh clone with
zero prior `npm install`. Note `go test`'s package list here does **not**
include `web/node_modules/flatted` (task-4's fix-round `GO_PKGS`
exclusion holds). Re-ran the same `make test` in the actual
`~/gits/my-template` working tree afterward as a final sanity pass before
committing — same result, 3 files/7 tests JS, all Go packages ok.

## Check 2 — `docker compose up` still works (Done-when 11), scratch port

Same fresh clone, `docker-compose.yml`'s port mapping temporarily edited
`"8080:8080"` → `"18180:8080"` in that scratch clone only (never in the
real working tree — confirmed `git status --short` in
`~/gits/my-template` shows no `docker-compose.yml` change).

```
$ docker compose -p tpl1-task5-verify build
...
 Image tpl1-task5-verify-app  Built
$ docker compose -p tpl1-task5-verify up -d
...
 Container tpl1-task5-verify-app-1  Started
$ docker compose -p tpl1-task5-verify ps
NAME                      ...  PORTS
tpl1-task5-verify-app-1   ...  0.0.0.0:18180->8080/tcp
$ curl -s -w "\nHTTP_STATUS:%{http_code}\n" http://localhost:18180/healthz
{"status":"ok"}
HTTP_STATUS:200
$ curl -s -D - -o /dev/null http://localhost:18180/
HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
$ curl -s http://localhost:18180/ | head -20
<!doctype html>
...
    <title>My Template</title>
    <script type="module" crossorigin src="/assets/index-C3ywW6A5.js"></script>
    <link rel="stylesheet" crossorigin href="/assets/index-B4x7aGNN.css">
  </head>
  <body>
    <div id="root"></div>
  </body>
</html>
```

Container builds, runs, `GET /healthz` → 200, `GET /` serves the real SPA
(embedded via `embed.FS`, not a stale placeholder — real title, real
hashed asset filenames). Torn down after
(`docker compose -p tpl1-task5-verify down -v`, image removed).

## Check 3 — pinned-port / `/callback` routing check

**Static check first**: `internal/platform/config.go` line 23 —
`Port int \`env:"PORT" envDefault:"8080"\`` — the binary's default listen
port is genuinely 8080. Checked `git log` on this file across
milestone-3's commits: untouched by tasks 1-4.

`cmd/server/main.go` registers `GET /callback` explicitly on the router
(line 317, `router.GET("/callback", bff.NewCallbackHandler(...))`),
*before* `router.NoRoute(...)` is registered (line 374) — gin always
prefers an explicit route over `NoRoute`, so structurally `/callback`
cannot fall through to the SPA regardless of request shape.

**Live confirmation, same scratch-port container from Check 2** (garbage/
incomplete requests — no valid query params, exactly per the task's own
"an incomplete/garbage request is fine, you're checking which handler
answers" instruction):

```
$ curl -s -D - http://localhost:18180/callback
HTTP/1.1 401 Unauthorized
Content-Type: text/html; charset=utf-8
Content-Length: 160

<!doctype html><html><body><h1>Login failed</h1><p>Something went wrong
signing you in. Please try again.</p><p><a href="/login">Try again</a>
</p></body></html>

$ curl -s -D - "http://localhost:18180/callback?state=garbage&code=garbage"
HTTP/1.1 401 Unauthorized
Content-Type: text/html; charset=utf-8
Content-Length: 160
<!doctype html>...<h1>Login failed</h1>...   (same body)

$ curl -s -D - http://localhost:18180/some-genuinely-unmatched-path-xyz
HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
<!doctype html>
...
    <title>My Template</title>
    <script type="module" crossorigin src="/assets/index-C3ywW6A5.js"></script>
  <body>
    <div id="root"></div>
  </body>
</html>
```

`GET /callback` (with no valid params) answers **401, HTML,
`<h1>Login failed</h1>`** — `bff.callback_handler.go`'s `renderLoginError`
firing on the "no state cookie present" branch (`renderLoginError` is
defined in `internal/transport/bff/middleware.go`). This exact shape —
"a plain error page" on a `/callback` failure — is also what
`.chief/milestone-2/_contract/API.md` line 54 documents as this surface's
own convention, so the check is verified against the contract's own
described shape, not an assumption.

A genuinely unmatched path answers **200, HTML, `<div id="root"></div>`**
— the SPA fallback. The two are clearly distinguishable on both status
code and body content, so this check can actually tell "hit the real
handler" from "hit the SPA," per the task's own requirement that the
check prove it can distinguish them.

**Explicit statement per the task's boundary**: none of the above touched
port 8080 or a real Hydra client. This verifies the routing logic in
isolation, on a scratch port, with garbage callback params against the
real `bff.NewCallbackHandler` code path (which fails fast on the missing
state cookie, before ever reaching Hydra). The actual live confirmation —
that `localhost:8080/callback` behaves the same way once Clara's `mt-server`
hand-over is complete and `my-template-dev`'s registered client is used for
a real login — is Clara's/มายด์'s to do, not reattempted here.

## Check 4 — the "one command to a browser" finish line, docs audit

Re-read `docs/GETTING-STARTED.md`'s "Running what you forked" section
against the actual current repo state (routes, ports, the SPA) and found
real drift, all fixed in this task's commit:

1. **"Known limitation: the owner has no supported way to create a
   todo"** was still describing task-1's placeholder state (a bare
   "Todos" heading, no data). Task-2/task-3 shipped real BFF CRUD and a
   real SPA todos screen since that section was written. Replaced with
   what's actually true now — the owner *can* create/edit/delete via
   `GET /login` → the SPA — while keeping the still-true I2 reasoning for
   why the write path lives on `/api/bff` and never `/api/v1`.
2. **The numbered "Running what you forked" checklist (1-5) only ever
   exercised the agent/API-key path** and its closing line ("that's the
   same condition GOAL.md's own Human Acceptance criterion stops at")
   pointed at milestone-1/2's criterion as though it were still the
   finish line. Milestone-3's own Human Acceptance criterion is further
   (create a todo through the SPA). Added step 6 — open `GET /login` in a
   real browser, log in, then create → see-in-list → edit → delete a todo
   through the SPA — and explicitly noted it needs Step 1 (Hydra client
   registration) first, unlike steps 1-5.
3. Added an explicit note that **for this exact template/instance,
   `register-<service>.sh` does not need re-running** — the registered
   client (`my-template-dev`) already exists and pins `redirect_uris`, so
   Step 1 is one-time-per-service-per-environment, not something every
   run repeats. This directly answers the task's "make sure the docs
   don't imply one is needed for this milestone specifically" instruction.
4. **Step 3b's React-app rename checklist** still named
   `web/src/app/settings/ApiKeySettingsPlaceholder.tsx` — task-3 renamed
   this file to `ApiKeySettings.tsx` (confirmed: `find web/src -iname
   '*Placeholder*'` returns nothing). Also described the SPA's `fetch`
   calls as "task-3 adds these — none exist yet," which is now false
   (`web/src/lib/{auth-client,todos,keys}.ts` all wire real requests
   against `/api/bff/*`). Both fixed.

**Is the documented path actually "one command, or close to it"?**
Plainly: **yes, for the case that actually matters right now** — this
exact template, with an already-registered Hydra client. Once port 8080
is free, the path is genuinely `docker compose up` (or `go run
./cmd/server`) plus opening a URL in a browser — one command plus a click,
matching what `_todo.md`'s task-5 spec asks for. For a **fresh fork**,
it's accurately "one required one-time setup command
(`scripts/register.sh`) plus one run command plus a URL" — not
disguised as fewer steps than it is, but also not artificially made to
sound harder than it is: `register.sh` is a single invocation, run once
per service per environment, not a recurring cost. No gap requiring
hand-assembly (an SSH tunnel, a special host) was found — `docker compose
up` plus `localhost:<PORT>` is genuinely sufficient once the port and the
client are both in place.

## Check 5 — the real write path, end to end, against the actual final branch state

Built a throwaway tool (`cmd/manualverify-task5/main.go`, **not
committed** — same pattern task-2's `cmd/manualverify` and task-3's
`cmd/manualverify-task3` used) that seeds a real `role=owner` user row via
`identity.Repo.CreateUser` and mints a real signed session cookie via
`bff.NewSigner([]byte(secret)).NewSessionCookie(user.ID)` — the exact
technique `internal/transport/bff/todo_handler_test.go`'s
`newBFFRouterForOwner` uses (`session.go`'s `Signer.NewSessionCookie`,
seeded directly, "no need to drive it through `/callback`" per
`_todo.md`'s own task-2 spec).

```
$ DATABASE_PATH=.../task5.db PORT=18181 SESSION_SECRET=<fixed> \
    go run ./cmd/server &
...INF server starting addr=:18181
$ curl -s http://localhost:18181/healthz
{"status":"ok"}

$ COOKIE=$(DATABASE_PATH=.../task5.db SESSION_SECRET=<fixed> \
    go run ./cmd/manualverify-task5)

$ curl -s -w "\nHTTP:%{http_code}\n" -H "Cookie: session=$COOKIE" \
    http://localhost:18181/api/bff/me
{"handle":"task5-manual-verify-owner","role":"owner","active":true}
HTTP:200

$ curl -s -D - -o /dev/null http://localhost:18181/api/bff/me   # no cookie
HTTP/1.1 401 Unauthorized                                       # not a redirect

$ curl -s -w "\nHTTP:%{http_code}\n" -H "Cookie: session=$COOKIE" \
    http://localhost:18181/api/bff/todos
{"todos":[]}
HTTP:200

$ curl -s -w "\nHTTP:%{http_code}\n" -H "Cookie: session=$COOKIE" \
    -H "Content-Type: application/json" -X POST \
    -d '{"title":"buy dog food for เจ้านาย"}' \
    http://localhost:18181/api/bff/todos
{"createdAt":"2026-08-13T09:06:04...","done":false,
 "id":"19f70118-c3ec-49ee-97ed-d8f7ee1b715c",
 "title":"buy dog food for เจ้านาย","updatedAt":"2026-08-13T09:06:04..."}
HTTP:201

$ curl -s -w "\nHTTP:%{http_code}\n" -H "Cookie: session=$COOKIE" \
    http://localhost:18181/api/bff/todos
{"todos":[{"id":"19f70118-...","title":"buy dog food for เจ้านาย",...}]}
HTTP:200                                                          # see it in the list

$ curl -s -w "\nHTTP:%{http_code}\n" -H "Cookie: session=$COOKIE" \
    -H "Content-Type: application/json" -X PATCH \
    -d '{"done":true,"title":"buy premium dog food for เจ้านาย"}' \
    http://localhost:18181/api/bff/todos/19f70118-c3ec-49ee-97ed-d8f7ee1b715c
{"...","done":true,"title":"buy premium dog food for เจ้านาย",...}
HTTP:200                                                          # edit

$ curl -s -w "\nHTTP:%{http_code}\n" -H "Cookie: session=$COOKIE" \
    http://localhost:18181/api/bff/todos/19f70118-c3ec-49ee-97ed-d8f7ee1b715c
{"...","done":true,"title":"buy premium dog food for เจ้านาย",...}
HTTP:200                                                          # edit persisted

$ curl -s -w "\nHTTP:%{http_code}\n" -H "Cookie: session=$COOKIE" -X DELETE \
    http://localhost:18181/api/bff/todos/19f70118-c3ec-49ee-97ed-d8f7ee1b715c
HTTP:204                                                          # delete

$ curl -s -w "\nHTTP:%{http_code}\n" -H "Cookie: session=$COOKIE" \
    http://localhost:18181/api/bff/todos/19f70118-c3ec-49ee-97ed-d8f7ee1b715c
{"error":{"code":"not_found","message":"no such todo"}}
HTTP:404                                                          # confirmed gone

$ curl -s -w "\nHTTP:%{http_code}\n" -H "Cookie: session=$COOKIE" \
    http://localhost:18181/api/bff/todos
{"todos":[]}
HTTP:200                                                          # list empty again
```

Full create → see-it-in-a-list → edit → delete cycle confirmed against
the actual final state of the branch (post task-4's `GO_PKGS`/fix-round
commits), using the exact request shapes the SPA's own
`web/src/lib/todos.ts` issues (`/api/bff/todos`, same methods/bodies).
Unauthenticated request confirmed 401, not a redirect (I12/I2 boundary
still holds at this layer). Server process killed and scratch DB/cookie
files removed afterward; the throwaway `cmd/manualverify-task5` directory
was never committed (confirmed `git status --short` shows only the doc
change).

## Notes

- Port 8080 confirmed occupied by `mt-server` (pid 1304524) both before
  and after this task's work — same pid both times, never touched.
- All five checks above ran without ever needing port 8080 or a real
  Hydra client, per the task's own boundary.
- The `RequireSession` removal, engines-floor fix, `/login` route
  deletion, and `GO_PKGS`/`fmt-check` node_modules exclusion — all
  fix-round items from tasks 3/4 — were re-confirmed as still holding via
  Check 1's fresh-clone `make test` pass (no `node_modules` path in the Go
  package list, `RequireSession` absent, `web/package.json`'s `engines`
  field present).
- No functional code was changed by this task — `docs/GETTING-STARTED.md`
  was the only file touched, per the doc-drift found in Check 4.

## Commits pushed (branch `milestone-2/close-parity-gap`)

- `883afb5` — `docs(milestone-3/task-5): fix stale GETTING-STARTED.md against final milestone-3 state`

## Final summary — what มายด์ types, starting from nothing

**Right now, this minute**: port 8080 is held by Clara's `mt-server`
(milestone-2's dev server, pid 1304524) — she's handling that hand-over
with มายด์ directly, per this task's own boundary instructions. Nothing
below can be run against the real port/real Hydra client until that port
is free; everything in this report was instead verified on scratch ports
as shown above.

**Once port 8080 is free**, from a clone of `milestone-2/close-parity-gap`
at or after `883afb5`:

```sh
docker compose up
```

(or `go run ./cmd/server`, if not using Docker) — **no `register.sh`
re-run needed**: the Hydra client `my-template-dev` already exists and
already pins `redirect_uris` to `http://localhost:8080/callback`, which
this branch's `PORT` default (8080, `internal/platform/config.go`) and
`GET /callback` routing (Check 3 above) both still match exactly.

Then open a browser at:

```
http://localhost:8080/login
```

Complete the real Hydra login/consent screen (the one step no agent in
this loop can do). It redirects back through `GET /callback` and lands on
`/` — the SPA's todos page. From there: click "New todo" to create one,
watch it appear in the list, click it (or its checkbox) to edit, and
delete it — the full CRUD cycle this report's Check 5 already confirmed
works at the API layer, now exercised through the real UI as
`_goal/GOAL.md`'s Human Acceptance section describes.

That's the whole path: one command, one URL, one real login — the gap
milestone-2's acceptance attempt exposed (a login that led to nothing to
create) is closed.

## Fix-round note (found by independent verification, not by this task's own builder)

Clara's own check — extracting every backticked filename from
`docs/GETTING-STARTED.md` and testing each as a real repo path, restricted
to full repo-relative paths after an initial pass produced mostly false
positives (bare names like `main.go` in prose, Go type references like
`internal/domain/todo.Service`) — found that Check 4's doc pass above
fixed the `ApiKeySettingsPlaceholder.tsx` rename and the "known
limitation" section, but missed a bigger one: **Step 5 ("Locate and
replace the todo domain") still instructed forkers to edit
`internal/transport/bff/view_handler.go` and `view_handler_test.go` in six
places**, both deleted by milestone-3/task-3. Three were live instructions
(the "optional BFF view" paragraph in step 4, and two passages in step
8's delete/adapt block), not background prose — a forker following the
checklist literally would hit a file that isn't there and reasonably
conclude they'd broken something, exactly the class of problem a blind
fork test caught once already last milestone.

Fixed by replacing the "third file" framing throughout Step 5 (the
section intro, step 1, step 4's optional BFF paragraph, and step 8's full
adapt-or-carry-tests block) with what's actually there now:
`internal/transport/bff/todo_handler.go` is structurally the BFF-side twin
of `internal/transport/publicapi/todo_handler.go` (`NewTodoServer`,
`ListTodos`/`CreateTodo`/`GetTodo`/`UpdateTodo`/`DeleteTodo` methods
calling `s.Service.*Todo(...)`, generated from a second OpenAPI spec,
`bff-openapi.yaml`, into a second package, `internal/bffapi`) — the same
rename-in-place already described for the `publicapi` handler applies
here, not a bespoke Go-`html/template` adaptation. Also added the SPA
screens under `web/src/app/` and `web/src/lib/todos.ts` (which reference
the domain by name through generated types) to the same instruction, and
named the three real Go tests to carry over
(`TestBFFHandler_FullCRUDRoundTrip_RealSessionCookie`,
`TestI3_BFFHandlerOwnershipScoping_ReturnsNotFoundNotForbidden`,
`TestBFFHandler_ListTodos_Unauthenticated_Returns401NotRedirect`) and the
two Vitest files that guard the SPA side (`TodosList.test.tsx`,
`todos.test.tsx`) in place of the deleted view-handler tests' now-wrong
names. Fixed the same stale reference in the "Dangling references after a
correct fork" section's delete-list parenthetical (also added
`bff-openapi.yaml`/`internal/bffapi/bffapi.gen.go` to that list, which the
original text never covered even for the BFF surface in general).

**Also checked, per Clara's own note**: `internal/todo/` (singular
mention, Step 5's intro paragraph) is legitimate historical prose — "this
superseded milestone-1's single-directory `internal/todo/` shape" —
describing a shape that no longer exists, not an instruction pointing at
a live path. Not stale, correctly framed as history already.

Verified via a mechanical scan (restricted to backticked strings
containing a `/` that resolve as repo-relative paths, run against every
file in `docs/`) that the only remaining `view_handler.go` mention in
`docs/GETTING-STARTED.md` is one intentional historical note (Step 1's
"an earlier version of this document pointed here at a Go-`html/template`
view ... retired by milestone-3's SPA"), explicitly framed as retired
rather than as an instruction. No functional code changed — doc only.
`go build ./...` reconfirmed clean (doc-only change).

### Commits pushed (branch `milestone-2/close-parity-gap`)

- `dc60435` — `docs(milestone-3/task-5): fix Step 5's stale view_handler.go references`
