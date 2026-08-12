# Task 3 Report

## Task
`todos` migration + sqlc queries (last new table this milestone adds),
`openapi.yaml` for `/me` and `/todos`, oapi-codegen + gin-middleware wiring,
owner-scoped CRUD (I3/I4), verify `goose up` on a fresh file with the full
migration set, write the Done-when 12 grep check now that all 10 invariants
should have named tests.

## Outcome
done

## Notes

- **Integration point handled correctly:** `GET /api/v1/me` was hand-wired
  directly in gin by task-2 (before `openapi.yaml` existed). This task moved
  it onto the generated interface alongside `/todos`, rather than leaving
  one endpoint on a different code path — confirmed by reading `main.go`'s
  route registration directly, not just trusting the report.
- **Independently re-verified (chief-agent/Luna, not just trusting the
  builder-agent report):**
  - `go test ./...` — clean, including `internal/architecture_test.go`
    (todo's handler/repo still obey the naming rules) and the Done-when 12
    check.
  - `grep`'d all `*_test.go` under `internal/` directly: `TestI1_` through
    `TestI10_` are all present — Done-when 12 genuinely holds, not just
    "the check that verifies it passed."
  - Read `db/migrations/*todos*.sql` directly against `_contract/
    DATA_MODEL.md`'s documented schema — matches exactly (columns, the
    `owner_id` FK with deliberate `RESTRICT` behavior explained in a
    comment, the `owner_id` index for I3).
  - Read `openapi.yaml`'s `servers:`/`paths:` against `main.go`'s route
    group mounting — `/api/v1` prefix applied consistently in both places,
    matching `_contract/API.md`'s documented paths.
- **Doc-vs-enforcement drift check (per the standing practice added after
  task-2's review — see `_todo.md` "Resolved during grill"):** this task
  didn't change any enforcement mechanism's *definition* (it implements
  what `API.md`/`DATA_MODEL.md` already specified, doesn't alter the rules
  themselves), so no doc update was owed here. Checked deliberately, not
  assumed.
- Separately, before this task: caught and fixed a real doc-vs-enforcement
  drift from task-2 (`ARCHITECTURE.md`'s handler/repo pattern prose hadn't
  been updated when task-2's regex gained a `_test` allowance) — see
  commit `9d1c101`, found by Clara.
- Commit: `313ae82` on `milestone-1/tpl-1-init-template` (task-3 itself),
  `9d1c101` (the doc-drift fix, committed just before this task's
  bookkeeping).

No blockers, no escalation.
