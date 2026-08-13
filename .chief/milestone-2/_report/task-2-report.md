# Task 2 Report

## Task
`register-<service>.sh` (env-var-driven, adapted from `register-my-task.sh`
preserving its safety properties), `GETTING-STARTED.md` Step 1 (mandatory,
including local-only dev), the fork-collision line in the rename
checklist, `DEPLOY-REQUIREMENTS.md`'s new section.

## Outcome
done

## Decision

- **Issue:** filename convention — literal placeholder filename
  (`register-SERVICE_NAME.sh`) vs. fixed filename reading env vars.
- **Chosen:** fixed filename (`scripts/register.sh`), env-var-driven.
  Reasoning: one fewer rename step in a task whose other half is
  specifically about forker-rename footguns (the key-path/env-var
  collision) — adding another rename-or-it-breaks-silently spot would
  have worked against the task's own point.
- **Also decided:** dropped my-task's `uat` third environment case
  (dev/prod only) — that case hardcodes a second, fleet-specific Hydra
  host or a fresh fork won't have on day one; the script's structure
  doesn't assume exactly two environments if a fork needs a third later.

## Notes

- **Independently re-verified (chief-agent/Luna):** `go build/vet/test`
  clean except the expected I11–I14 gap (untouched); `bash -n
  scripts/register.sh` confirms syntax. `shellcheck` isn't installed on
  this machine — builder noted this rather than skipping silently, matches
  this project's standing preference for a stated limitation over a
  silent gap.
- Grepped the script and both docs for real fleet values (`thadaw.com`,
  `thw-home`, literal ports) — none found outside comments crediting the
  source script. Placeholder discipline held.
- **Clara independently re-attacked task-1's guardrail in parallel with
  this task** (all 5 rules, including the two — rule 2, rule 3 — this
  session's own verification hadn't exercised) — all confirmed catching
  correctly. Noted here since it's relevant context for the milestone,
  even though it's not this task's own work.
- Commit: `d9c601f` on `milestone-2/close-parity-gap`.

No blockers, no escalation.
