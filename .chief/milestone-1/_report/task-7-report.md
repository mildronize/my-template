# Task 7 Report

## Task
Fix every finding from Clara's second blind fork test: a real security hole
in Done-when 12's invariant check (repo-wide grep let one module's test
mask every other module's missing coverage forever), a doc-ordering bug
that made deleting the only worked example the first instruction, plus
several more P1/P2 doc gaps.

## Outcome
done

## Decision

- **Issue:** `TestDoneWhen12_EveryInvariantHasANamedTest` greped for
  `TestI<N>_` anywhere in the repo. `internal/identity` already had a
  `TestI3_..._Keys` test, which satisfied I3's requirement for the entire
  repo permanently — proven by Clara's agent renaming all `TestI3_` tests
  out of its new `internal/bookmark` module and the suite still passing.
- **Options considered:** (a) leave as repo-wide grep, document the
  limitation (rejected — this is the class of silent gap the milestone has
  spent all day removing, not documenting around), (b) require every
  invariant to have a test in every domain module regardless of relevance
  (rejected — I1/I2/I5-I10 are single-seam auth concerns, not per-domain),
  (c) tag each invariant's scope (`global` vs `per-domain-module`) in
  `INVARIANTS.md` and check accordingly, reusing the architecture
  guardrail's `domainModuleNames()` helper (chosen).
- **Chosen:** (c). This was originally Clara's own design for Done-when
  12 (the earlier (c) that replaced hardcoded (a)/(b) options) — the flaw
  was in the design's granularity (name-anywhere vs name-in-the-right-place),
  not in how it was implemented, and Clara said so explicitly rather than
  letting it read as builder error.

## Notes

- **Independently re-verified (chief-agent/Luna), including the fix
  that mattered most:**
  - Grepped `internal/identity/*_test.go` directly — confirmed its own
    dedicated `TestI3_..._Keys` and new
    `TestI4_IdentityRepoOnlyQueriesUsersAndAPIKeysTables` both exist.
  - **Re-ran the exact exploit independently**, not just trusting the
    builder's scratch-clone claim: cloned to a fresh scratchpad, renamed
    all of `internal/todo`'s `TestI3_`/`TestI4_` tests to non-matching
    names (simulating exactly what Clara's agent did), ran
    `TestDoneWhen12_EveryInvariantHasANamedTest` — **failed correctly**,
    citing `internal/todo`'s missing I3 and I4 tests by name, with the
    scope tag and the reasoning in the error message itself. Scratch clone
    deleted after.
  - Read `_contract/INVARIANTS.md` directly — all 10 invariants carry an
    explicit `scope:` tag (I3/I4 `per-domain-module`, the rest `global`),
    with a short explanation of what each scope means for the check.
  - Read `docs/GETTING-STARTED.md`'s Step 4 directly — confirmed the
    reorder: read `internal/todo` fully (1) → copy and rename (2) → wire
    in (3) → **only then** `rm -rf internal/todo` (4) → deal with the
    invariants that deletes (5). Matches the fix exactly, not just the
    report's description of it.
  - `go test ./...` clean independently.
- One naming correction in the builder's report vs. the actual code: the
  task spec called the embedding trick `compositeServer`; the real type in
  `cmd/server/main.go` is `apiServer`. Builder documented the accurate name
  in GETTING-STARTED, not the task spec's guess — correct call, noted here
  so it doesn't read as a discrepancy.
- P2's "identity-seam cheat sheet" item was judged redundant against the
  new "Patterns worth preserving" list in Step 4 and skipped — reasonable,
  not chased further.
- Commits: `45eeaca` through `34147d6` on `milestone-1/tpl-1-init-template`.

No blockers, no escalation. Ready for a third blind fork test with a fresh
context-free agent.
