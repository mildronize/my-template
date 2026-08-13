# Task 6 Report

## Task
Real-HTTP e2e smoke script (`cmd/smoke`) against a live instance with a
real minted key — closes the gap every other test in this codebase
(correctly) leaves open by injecting the actor or signing a session
directly: the actual `Authorization: Bearer` → hash lookup → database
path has never been exercised end to end until this task.

## Outcome
done

## Notes

- **Independently re-run twice (chief-agent/Luna), not trusted from the
  pasted output alone:**
  - Started a genuinely fresh instance myself (new temp SQLite file, new
    port), ran `go run ./cmd/smoke` against it — **16/16 passed**,
    matching the builder's report exactly, confirmed against a server I
    controlled independently rather than reusing theirs.
  - Tested the negative case too (unreachable server) — first attempt
    was confounded by a `go run` job-control artifact on my end (`kill
    %1` doesn't reliably kill `go run`'s child binary process, not a bug
    in the smoke script); found the actual listening process via `lsof`
    and killed it properly. With the server genuinely unreachable: the
    script does a healthz preflight, fails immediately with a clear,
    actionable message, and exits 1 — without even attempting to mint
    keys first. Confirms it can gate a deploy, not just report success
    when things happen to work.
- Commit: `eebb962` on `milestone-2/close-parity-gap`.

No blockers, no escalation.
