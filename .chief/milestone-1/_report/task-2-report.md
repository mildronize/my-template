# Task 2 Report

## Task
Identity: `users`/`api_keys` migration + sqlc queries, actor-resolution
middleware (API key → JWT → reject), the I1 actor-field-rejection
middleware, the CLI key-issuance script, `GET /api/v1/me`, and named tests
covering this task's share of Done-when 12 (I1, I2, I5–I10).

## Outcome
done

## Decision

- **Issue:** task-1's `architecture_test.go` regexes required an exact
  `_handler.go`/`_repo.go` suffix, which can never match Go's mandatory
  `_test.go` suffix — so any test file importing gin (needed here to drive
  the middleware through a real `gin.Engine`) would fail the architecture
  check regardless of correctness.
- **Options considered:** (a) name test files to somehow dodge the pattern
  (not really possible given Go's `_test.go` requirement), (b) extend the
  regex to accept an optional trailing `_test`, (c) escalate.
- **Chosen:** (b) — extended both regexes. Doesn't weaken what the rule
  protects (service-layer code staying gin-free); only accounts for test
  files of already-permitted handler/repo files. Not a design ambiguity
  worth escalating — a mechanical bug in a check written one task ago.

## Notes

- Verified live (not just unit tests): `goose up` on a fresh DB, `sqlc
  generate` idempotent, full manual smoke test including the dormant-JWT
  path logging as expected with `SSO_ISSUER` unset.
- **Independently re-verified after the fact (not by builder-agent, by
  chief-agent/Luna):** Clara red-teamed task-1's architecture guardrail
  (rules 1 and 3) by cloning to a scratch copy and injecting violations —
  both caught correctly, with actionable error messages. Rule 2 (only
  `repo.go`/`*_repo.go` may import the sqlc-generated package) couldn't be
  tested at the time because `internal/db` didn't exist yet. It exists now
  (this task generated it) — same scratch-clone-and-inject method applied
  to rule 2: a deliberately-planted `internal/identity/leak_service.go`
  importing `internal/db` directly was caught, naming the file and citing
  ARCHITECTURE.md rule 2. All three rules now confirmed to actually fail
  closed, not just pass because nothing has violated them yet.
- Commit: `18a3f06` on `milestone-1/tpl-1-init-template`.

No blockers, no escalation beyond the one mechanical fix above.
