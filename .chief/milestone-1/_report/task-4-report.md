# Task 4 Report

## Task
`GET /api/v1/keys`, `DELETE /api/v1/keys/:id` — new sqlc queries against
the existing `api_keys` table (last task adding queries this milestone,
closing Done-when 3), same ownership-scoping as todos (I3 on a second
resource), I9 applied to the listing behavior specifically.

## Outcome
done

## Notes

- Found mid-task that task-2 had already added the `RevokeAPIKey` query/
  repo method but never wired it to the service or HTTP layer — this task
  completed the wiring rather than duplicating it. Not treated as an
  ambiguity worth escalating (mechanical completion of existing work, no
  design decision involved).
- **Independently re-verified:** `go test ./...` clean; grepped `TestI1_`
  through `TestI10_` directly — still all present (I3 now 5 hits across
  todos+keys, I9 now 2). Confirmed `sqlc generate`/`oapi-codegen` are true
  no-ops across the *entire* generated output (`git diff --stat
  internal/db internal/api` empty after a forced re-run), which is this
  task's actual Done-when (3) — not just the new query in isolation.
- No enforcement-mechanism doc updates were owed (none of this task's work
  changed a rule's definition) — confirmed deliberately per the standing
  practice from task-2/3's reports, not left unstated.

## Carried into task-5

Clara found a real gap in task-3's Done-when 12 check
(`internal/invariants_test.go`): it hardcodes `for n := 1; n <= 10` instead
of deriving the required invariant set from `_contract/INVARIANTS.md`
itself. A fork adding `I11` without touching the test file gets a check
that stays green forever — silent, the same failure direction
`ARCHITECTURE.md`'s own allowlist design explicitly warns against. Not
fixed in task-4 (out of scope, and Clara asked to bundle it with task-5's
`GETTING-STARTED.md` work) — recorded as a required part of task-5 in
`_plan/_todo.md`.

Commit: `cba284a` on `milestone-1/tpl-1-init-template`.

No blockers, no escalation.
