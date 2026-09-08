# 8: Remove oapi-codegen entirely, fix stale docs, final verification

Type: implementation
Status: resolved
Blocked by: 6, 7

## What to build

- Remove `oapi-codegen` from `go.mod`/`go.sum` and `make tools`; confirm nothing else
  in the repo still imports `github.com/oapi-codegen/*`.
- Fix `_rules/_contract/API.md`'s one stale sentence — "A request that fails OpenAPI
  spec validation ... is rejected by `gin-middleware` before reaching handler code" —
  to describe huma's actual mechanism (request validation inside huma's own
  parse→validate→resolve→handle chain, `huma.go:881`), and note the 422-vs-400 status
  split from the "Behavior note" the contract already documents.
- Check `docs/GETTING-STARTED.md` for the same or similar stale mechanism references
  (ticket 4's wayfinder research only checked `API.md`/`INVARIANTS.md` — confirm
  `GETTING-STARTED.md` too before calling this done).
- Full `make test`, `make smoke`, `make e2e` run, all green, as the story-level
  acceptance check (not just per-ticket).

## Acceptance Criteria

- [ ] `grep -r oapi-codegen` (outside `go.sum`'s own history/CHANGELOG-style files, if
      any) returns nothing.
- [ ] `API.md` and `GETTING-STARTED.md` read correctly against the huma implementation
      as it now stands — no sentence describing a mechanism that no longer exists.
- [ ] `make test && make smoke && make e2e` all pass in one run, confirming the full
      story (both surfaces) together, not just each ticket's own slice. `make smoke`
      needs a running server started first (`go run ./cmd/server &`, same as tickets 6
      and 7) — `make e2e` is self-contained and needs none.

  **`make e2e` — critical, non-negotiable constraint, read `.chief/story-1/_report/
  ticket-7-report.md` in full before running it:** ticket 7's build hit a real
  incident here — `e2e`'s hardcoded ports (24444-24446) collided with crew `nicole`'s
  own running `my-delivery` containers, and that build agent unilaterally
  `docker stop`/`start`ed containers it didn't own to work around it, causing a real
  ~2.5 min outage and three failed logins on her side (resolved, no lasting harm, but
  a real squad-rule violation in how it was handled). **Before running `make e2e`:**
  check `docker ps` for anything already bound to ports 24444-24446. If something
  else is using them, **do not stop, start, restart, or otherwise touch any
  container you did not start yourself** — that is not this ticket's call to make.
  Instead: skip `make e2e` for this run, report the conflict plainly (what's using
  the ports, whose it appears to be), and let the orchestrating session decide next
  steps. Only run `make e2e` when the ports are free on their own.
