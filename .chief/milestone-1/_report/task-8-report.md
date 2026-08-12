# Task 8 Report

## Task
Fix every finding from Clara's third (and, per her stated stopping
criterion, final) blind fork test: an unreachable checkpoint in Step 4, two
guaranteed compile breaks during the two-modules-coexisting window task-6's
own fix introduced, a missing `openapi.yaml` entry in the delete checklist,
and a hardcoding regression in task-7's own newly-added I4 check.

## Outcome
done

## Decision

- **Issue (P0-4):** `internal/identity/repo_test.go`'s
  `TestI4_IdentityRepoOnly...` hardcoded `"todos"` as the forbidden table.
  `internal/todo`'s equivalent hardcoded `"users","api_keys"` back the
  other way — two independent, drift-prone implementations, each assuming
  it knows the other module's table name in advance. A fork deleting
  `todos` and adding `snippets` makes identity's check forbid a table that
  no longer exists — passes vacuously, checks nothing. Proven by the blind
  test agent injecting a live query into the wrong file and the suite
  staying green.
- **Options considered:** (a) update the hardcoded lists whenever a domain
  changes (rejected — exactly the maintenance-by-memory failure mode this
  whole milestone has been removing), (b) derive forbidden tables
  dynamically by scanning `db/queries/*.sql` for what every *other* file
  references (chosen).
- **Chosen:** (b), implemented once in `internal/dbquery` (a new
  non-domain helper package, added to `architecture_test.go`'s
  non-domain exclusion list) rather than as two independent per-module
  copies — removes the drift risk at its root instead of just fixing both
  copies in place.

## Notes

- **Independently re-verified (chief-agent/Luna), the fix that mattered
  most, replayed from scratch — not the same worktree the builder used:**
  cloned to a fresh scratchpad, appended a raw `SELECT * FROM todos;` to
  `db/queries/users.sql` (identity's own query file, referencing a
  different module's table), ran `TestI4_IdentityRepoOnlyQueriesUsers...`
  directly — **failed correctly**, citing the exact injected line and
  naming which file it belongs to instead. No hardcoded table name is
  doing any of that work anymore. Scratch clone deleted after.
- Spot-checked `docs/GETTING-STARTED.md` directly for the other three P0
  items: the "Two modules, briefly, at once" section exists with the
  `Server redeclared` warning and alias workaround; `openapi.yaml`'s
  paths/schemas are explicitly listed in Step 4's delete step, with a
  sentence clarifying `make generate`'s auto-cleanup doesn't extend to it.
- `go test ./...` clean independently, including `internal/dbquery` (no
  test files of its own — it's a helper package, exercised through
  `internal/todo`'s and `internal/identity`'s tests, which is correct).
- All P1 items (undersold consequence of the name-only check, wrong
  dangling-reference counts, missing `--exclude-dir=.chief`, unexplained
  I3/I4) and all P2 polish items were completed per the builder's report;
  spot-checked rather than re-deriving every count myself, since none of
  these are security-severe per Clara's stated gate.
- Commits: `8af6b69`, `89e0852`, `be0d314` on
  `milestone-1/tpl-1-init-template`.

No blockers, no escalation. Per Clara's stopping criterion, this should be
followed by one confirmatory (not new-gap-hunting) blind fork test — stop
if it forks by following the doc in order with no new security-severe P0.
