# Task 1 Report

## Task
Restructure to the 5-rule `domain`/`identity`/`transport`/`platform`
layout: move `internal/todo` → `internal/domain/todo`, extract transport
code (identity's handler + todo's handler) into
`internal/transport/publicapi`, rewrite `internal/architecture_test.go`
for the new rules, add `platform/middleware.go` (recovery/logging/
request-ID), verify by attacking the rewritten guardrail, verify `docker
compose up` post-restructure.

## Outcome
done

## Notes

- **Independently re-verified (chief-agent/Luna), all four required
  checks — not trusted from the report alone:**
  - `go test ./...`: confirmed exactly I11–I14 red (the expected set per
    `_todo.md`'s durable note), everything else green, including all
    relocated tests. `invariants_test.go`'s logic untouched.
  - **Attack 1** (fake `internal/domain/fakemodule/service.go` importing
    `gin`) in a fresh scratch clone — caught, `TestArchitecture_
    DomainFilesNeverImportGin` fails correctly.
  - **Attack 2** (`internal/platform` importing `internal/domain/todo`)
    in a fresh scratch clone — caught, `TestArchitecture_
    PlatformNeverImportsDomainIdentityOrTransport` fails with the file
    and rule cited.
  - **Attack 3, re-tested in the non-cyclic direction** — the builder's
    original attack 3 (`domain/todo` importing `transport/publicapi`)
    hit a genuine Go import cycle (since `publicapi` already imports
    `domain/todo`), so the violation was caught by the Go toolchain
    before `architecture_test.go`'s own message could fire — a weaker
    citation than attacks 1/2 got, worth checking wasn't hiding a real
    gap. Re-ran against `domain/todo` importing `transport/bff` instead
    (doesn't depend back on `todo`, so no cycle) — `TestArchitecture_
    DomainAndIdentityNeverImportTransport` fired with its own clear
    message, confirming rule 4 works standalone and the cycle in the
    original attack was incidental to which packages happened to be
    tested against, not a gap in the check.
  - **`docker compose up`**: built and ran clean from a fresh volume
    independently (separate from the builder's own run) — `/healthz`
    200, `issue-key` minted a working key inside the container, `GET
    /api/v1/me` round-tripped correctly. Torn down after.
- All three scratch clones deleted after use; no changes to the real
  repo from verification.
- Commit: `4936fec` on `milestone-2/close-parity-gap`.

No blockers, no escalation.
