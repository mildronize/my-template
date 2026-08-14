# Task task-9 Report

## Task

Final verification — full-suite gate (`go test ./...` and the JS suite
green together, from a fresh clone, not independently at the point each
was last touched; `docker compose up` still works), then มายด์'s own
five-step acceptance walkthrough attempted as far as this crew's tooling
actually reaches. Every step gets one of three labels — completed,
partially reached, or browser-only — with what was verified at the API
layer and what was not verified at the layer มายด์ uses both stated
separately. Report to Clara before claiming the milestone ready, not
after. Owns no new Done-when — confirms what the rest of this
milestone's tasks already built.

## Outcome

done

## Full-suite gate

Fresh clone, `ac9fed6` (task-8's own closing commit — the current branch
head at the start of this task).

```
$ make test
cd web && npm ci && npm test
...
 Test Files  6 passed (6)
      Tests  22 passed (22)
...
go test $(GO_PKGS)
ok  	github.com/mildronize/my-template/cmd/issue-key
ok  	github.com/mildronize/my-template/cmd/seed
ok  	github.com/mildronize/my-template/cmd/server
ok  	github.com/mildronize/my-template/internal
ok  	github.com/mildronize/my-template/internal/dbquery
ok  	github.com/mildronize/my-template/internal/domain/todo
ok  	github.com/mildronize/my-template/internal/identity
ok  	github.com/mildronize/my-template/internal/platform
ok  	github.com/mildronize/my-template/internal/transport/bff
ok  	github.com/mildronize/my-template/internal/transport/publicapi
```

Both suites, one command, from a fresh `npm ci` and a fresh Go build —
not independently re-run at whichever commit each was last individually
touched.

### `docker compose up`

Port 8080 is `mt-server` (a live instance, unrelated to this milestone) —
untouched, per the standing boundary from earlier engagements. Ran on a
scratch port (`18190:8080`, edited only in the scratch clone's own
`docker-compose.yml`, never committed):

```
$ docker compose -p task9-scratch up -d --build
... real multi-stage build, not cached from an earlier session ...
 Container task9-scratch-app-1 Started

$ curl -sS http://localhost:18190/healthz
{"status":"ok"}

$ curl -sS http://localhost:18190/ | grep -o '<title>[^<]*</title>'
<title>My Template</title>

$ docker compose -p task9-scratch down -v
$ docker image rm task9-scratch-app
```

Real image, real container, real `200`s, torn down and the image removed
afterward — confirmed no leftover container, volume, or image before
moving on.

## The walkthrough

No browser automation exists in this crew's toolset — checked again,
still absent, not something this task adds (that gap is parked as its
own ticket, per Clara's earlier instruction). Built the real SPA
(`npm run build`), built `server`/`issue-key`/`seed` from the same clone,
ran the compiled `server` binary as a real OS process on a scratch port
with a fixed `SESSION_SECRET`, seeded a real owner and issued two real
agent keys (`luna`, `clara`) through the real CLI tools — not fixtures,
not mocks. Every step below is a real HTTP round trip against that real
process.

### Step 1 — sees every agent's key in Settings and revokes one, and that key stops working. **Partially reached.**

```
$ curl --cookie "session=$OWNER" .../api/bff/keys
{"keys":[{"handle":"clara","prefix":"tpl_8f25c480",...},{"handle":"luna","prefix":"tpl_4be7a8aa",...}]}

$ curl -H "Authorization: Bearer $LUNA_KEY" .../api/v1/me
{"handle":"luna","role":"agent","active":true}   # 200, before revoke

$ curl -X DELETE --cookie "session=$OWNER" .../api/bff/keys/$LUNA_KEY_ID
204

$ curl -H "Authorization: Bearer $LUNA_KEY" .../api/v1/me
{"error":{"code":"unauthorized","message":"authentication required"}}   # 401, same raw key
```

**Verified at the API layer**: both halves, not just the post-revoke
check — the same raw key authenticated successfully first, then failed
after revocation, and the list no longer shows it. **Not verified at the
layer มายด์ uses**: nobody opened Settings in a browser or clicked a
Revoke button. Task-7's `ApiKeySettings.test.tsx` proves the component
renders the handle and names it in the confirmation dialog, given this
same data shape — a component test, not a browser against a running
instance.

### Step 2 — sees todos agents created, not just his own. **Partially reached.**

```
$ curl -X POST -H "Authorization: Bearer $CLARA_KEY" .../api/v1/todos -d '{"title":"...","clientRequestId":"..."}'
{"id":"...","createdBy":"<clara's user id>",...}   # 201

$ curl --cookie "session=$OWNER" .../api/bff/todos
{"todos":[{"id":"...","createdBy":"<clara's user id>",...}]}
```

**Verified at the API layer**: a todo the owner never touched, created by
an agent, visible in the owner's own list. **Not verified at the layer
มายด์ uses**: the todos list page was never opened in a browser.

### Step 3 — sees a todo with status/assignee/priority/due date and changes one. **Partially reached.**

Set priority, due date, and assignee through three real
`POST .../events` calls, then changed status (`open` → `in_progress`) as
the owner. Final state, one response:

```
$ curl --cookie "session=$OWNER" .../api/bff/todos/$ID
{"status":"in_progress","assigneeHandle":"luna","assigneeId":"...",
 "priority":"high","dueDate":"2026-09-01T00:00:00Z",...}
```

**Verified at the API layer**: all four fields set and correct in one
final read, not four separate unconnected claims. **Not verified at the
layer มายด์ uses**: the detail page's status/priority/assignee controls
were never clicked.

### Step 4 — opens the feed and sees who did what, human vs agent distinguishable on every row. **Partially reached.**

```
$ curl -X POST -H "Authorization: Bearer $CLARA_KEY" .../api/v1/todos/$ID/events -d '{"type":"commented",...}'
201

$ curl --cookie "session=$OWNER" ".../api/bff/activity?limit=10"
{"items":[
  {"actor":{"handle":"clara","role":"agent"},"type":"commented",...},
  {"actor":{"handle":"owner","role":"owner"},"type":"status_changed",...},
  ...
]}
```

**Verified at the API layer**: six real events on one todo, agent rows
genuinely carrying `role: "agent"`, owner rows genuinely carrying
`role: "owner"` — the actual cross-actor proof (Done-when 12's own
property), not two same-shaped rows. **Not verified at the layer มายด์
uses**: nobody saw a 🧑/🤖 mark rendered. Task-7's Done-when 9 suite
proves the mark branches correctly on this exact data shape (attacked
and confirmed earlier this session) — proving the branch and watching it
render are different claims.

### Step 5 — opens one todo and sees that same history reading identically. **Partially reached.**

```
$ curl --cookie "session=$OWNER" .../api/bff/todos/$ID/events
{"events":[ ...same six events, same actor/type/payload/body as the feed above... ]}
```

**Verified at the API layer**: the same six events, compared field by
field against the feed's own rows for this todo — identical. **Not
verified at the layer มายด์ uses**: "reads identically" is fundamentally
a rendering claim, not a data one — that a browser looking at both pages
would see the same row shape. That's Done-when 8's own proof (one
`TimelineEventRow` import, driven by both pages' real adapters,
byte-identical `innerHTML`), not something `curl` can show. The data
handed to the shared component is confirmed identical; the pixels are
not.

## The one fact underneath all five

Every step above proves session **consumption** — a minted, valid
session resolving to the right actor and serving the right data. Session
**establishment** — the real SSO login, the redirect, `GET /login`/
`GET /callback`, a browser actually obtaining that cookie — was not
reached at all. The running instance logged
`SSO_ISSUER/SSO_CLIENT_ID not both set` and refused the flow; this crew
has no Hydra client registered in this environment and no way to drive a
real login regardless (no browser automation). This is the same accepted
gap named at every prior gate this milestone (task-4's own reachability
check drew the identical line) — restated here because it is the one
thing genuinely **browser-only** across all five steps, not partially
reached: nothing above touches it, and nothing above should be read as
if it does.

## What I did not establish

- Nothing above was driven through an actual browser — every step is a
  real HTTP round trip against a real running binary, which is a
  narrower instrument than a browser sitting in front of a rendered
  page, stated as its own thing per step above, not folded into a single
  caveat at the end.
- Session establishment (the SSO login flow) was not exercised at all —
  see above.
- The claim "this crew's tooling has no browser automation" was checked
  again for this task specifically, not just carried forward from an
  earlier finding — still absent.

## Not claiming the milestone ready

That is มายด์'s own walkthrough to run, not something this task's own
API-layer proof substitutes for. Reported to Clara before any claim of
readiness, per the standing instruction.
