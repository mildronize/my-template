# Task 6 Report

## Task
Fix every finding from Clara's first blind fork test: P0 (architecture
guardrail hardcoded its domain list, so a new domain module got zero
coverage), P1 (invariant-removal direction undocumented, a Done-when-2 test
buried in the deleted domain, no prerequisites/run instructions), P2
(orphaned generated files, misleading env-var step, `.chief/` unaddressed,
stale example values, a fleet-internal path that won't exist on a fork), and
tightening the Human Acceptance criterion's scope.

## Outcome
done

## Decision

- **Issue (P0):** `internal/architecture_test.go` hardcoded
  `{internal/todo, internal/identity}` in two places (both tests' domain
  lists) — a fork keeping `internal/todo` and adding `internal/note` beside
  it would get zero import-rule coverage on the new module, silently.
- **Options considered:** derive the domain list from the filesystem
  (chosen — matches the pattern already used for Done-when 12's invariant
  list), or document the coupling and require manual updates on fork
  (rejected — same silent-gap shape the milestone has spent all day fixing
  everywhere else it appeared).
- **Chosen:** filesystem-derived domain module list (every `internal/*`
  except `api`/`db`/`platform`), one shared helper for both tests.

## Notes

- **Independently re-verified (chief-agent/Luna), not just trusted from the
  report:**
  - Re-ran the scratch-clone injection test myself, independently of the
    one the builder-agent already ran: planted `internal/note/service.go`
    (imports gin, non-handler name) in a fresh clone — confirmed the fixed
    guardrail catches it (`TestArchitecture_DomainFileImportRules` fails
    with the correct message, citing the file and ARCHITECTURE.md rule 1).
    Scratch clone deleted after.
  - `go test ./...` clean independently, including `internal/platform`
    now having tests (the relocated migration test).
  - Read `internal/invariants_test.go` directly — confirmed it now
    `filepath.WalkDir`s under `.chief/` for any `INVARIANTS.md` rather than
    hardcoding `milestone-1`'s path.
  - Read `docs/GETTING-STARTED.md`'s section list directly — Prerequisites,
    Running-what-you-forked, and `.chief/` sections all present, in
    addition to the four original Step N headings.
  - Read `_goal/GOAL.md`'s Human Acceptance section directly — the
    domain-module-scoped reading rule is stated explicitly, not left
    implicit.
- This task owns no Done-when items (all 12 were already closed by task-5)
  — it hardens what those checks actually verify, closing gaps a
  context-free tester found that no automated check was positioned to
  catch (the whole point of running one).
- 8 commits (`f57220a` through `e83630c`) on `milestone-1/tpl-1-init-template`,
  each a self-contained logical chunk per the task-6 spec's P0/P1/P2
  grouping, plus a bonus fix for a stale comment the P1(b) test-move left
  behind.

No blockers, no escalation. Ready for Clara's second blind fork test with a
fresh, uncontaminated context-free agent.
