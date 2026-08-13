# Task 8 Report

## Task
Companion `my-template-api` skill doc scaffolded from `my-task-api`'s
shape but this template's actual contract, both required warning
sections, plus the binding final-suite gate (Done-when 14, 15) and a
final `docker compose up` re-verification (Done-when 16).

## Outcome
done — **milestone-2 complete.**

## Notes

- **Independently re-verified (chief-agent/Luna) — the milestone's final
  checkpoint, verified accordingly:**
  - `go test ./... -count=1` (fresh cache) — every package green,
    matching the builder's 96/0 report.
  - Grepped `SKILL.md` for `^### ` headers directly — both required
    warnings present as identifiable sections: "The indistinguishable-401
    trap" and "`0600` is a rule, not an isolation guarantee."
  - **Full independent `docker compose up`**: built and started clean,
    `/healthz` 200, `issue-key` inside the container minted a working
    `tpl_`-prefixed key, `GET /api/v1/me` round-tripped the correct
    handle. Torn down after.
- **Clara independently attacked task-7's idempotency check in parallel**
  (broke the check-then-insert guard, confirmed the test fails) — found a
  genuinely interesting nuance: the failure came from the database's own
  `sso_subject` unique constraint, not the `alreadyExisted` assertion,
  meaning there are two independent safety nets, not one. Noted here as
  relevant milestone context even though it's not this task's own work.
- Commit: `d24b452` on `milestone-2/close-parity-gap`.

No blockers, no escalation. **All 16 Done-when items hold, independently
verified across all 8 tasks — not just reported.** Milestone-2 is ready
for Clara's final review pass and มายด์'s human acceptance (a real login
against a registered Hydra client, seeing his own todos).
