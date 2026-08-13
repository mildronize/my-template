# Task 7 Report

## Task
Idempotent owner seed script (`cmd/seed`), `SEED_OWNER_SSO_SUBJECT`
config, check-then-insert on `sso_subject`, no JIT, nothing beyond the
owner row.

## Outcome
done

## Decision

- **Issue:** the contract only names `SEED_OWNER_SSO_SUBJECT` as
  configurable — no env var for the owner's handle.
- **Chosen:** fixed literal `handle="owner"`, mirroring how `role`/
  `active` are also fixed rather than configurable — didn't invent a
  second env var the contract doesn't ask for.

## Notes

- **Independently re-verified (chief-agent/Luna), using Clara's own
  method (run twice, count rows directly)** rather than trusting either
  the builder's paste or a single verification pass:
  - Ran `go run ./cmd/seed` twice against a fresh temp SQLite file I
    controlled myself — same output shape as reported (creates, then
    "already exists — left alone").
  - Queried the database directly via `python3`'s `sqlite3` module
    (`sqlite3` CLI isn't installed on this machine) — **exactly 1 row**
    in `users` total, exactly 1 with `role='owner'`, after both runs.
    Not the CLI's own success message — the actual table state.
- `go test ./...` still fully green, no regression from task-5's first
  clean run.
- Commit: `c33678a` on `milestone-2/close-parity-gap`.

No blockers, no escalation.
