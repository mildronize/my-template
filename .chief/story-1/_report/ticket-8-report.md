# Ticket 8 Report

## Ticket
Remove oapi-codegen entirely, fix stale docs, final story-level verification.

## Outcome
done, except one acceptance-criteria line (`make e2e`) — see Notes. This ticket's own
work is complete and verified; the story as a whole is not yet fully closed.

## Decision
No ambiguity this ticket — the port-conflict constraint (written directly into the
ticket and the build prompt, following ticket 7's incident) worked as intended: the
build agent found the same two ports still occupied by `nicole`'s containers, did not
touch them, and reported the conflict plainly instead of improvising a workaround.
Nothing to decide here beyond what to do about the still-open `make e2e` gap (see
Notes).

## Notes

- Independently verified by the orchestrating session: `go build ./...`, `go vet ./...`
  clean; `grep -rn oapi-codegen --include=*.go` returns only historical/explanatory
  comments (e.g. "oapi-codegen used to be pinned here"), no live import, no
  `go.mod`/`go.sum` entry.
- `oapi-codegen` fully removed from `tools/tools.go`/`go.mod`/`go.sum`.
- `_rules/_contract/API.md`'s stale `gin-middleware` sentence fixed — and a real
  correction caught by this ticket's own code review before it landed: the first
  draft got the huma ordering backwards (claimed shape-validation now runs before
  auth, when actually auth runs first for *generic* validation failures — only the
  bespoke actor-field/`X-Actor` check, ticket 6's fix, stays pre-auth). Also fixed
  `docs/GETTING-STARTED.md` (multiple passages still describing the retired
  oapi-codegen/`ServerInterface` mechanism), `README.md`, `PROJECT_SUMMARY.md`
  (untracked, per ticket 5's precedent — edited but not committed), and
  `.claude/skills/my-template-api/references/{endpoints,errors}.md` (a real
  correctness bug: claimed a still-`400` status for cases that now correctly answer
  `422` — agent-facing docs, so this one actually mattered for correctness, not just
  prose staleness).
- `make test` (Go + Vitest, 27 tests) and `make smoke` (31/31, live server) both
  green, independently plausible given clean build/vet and ticket 6/7's own
  independently-verified state.
- **`make e2e` — resolved, not just deferred.** The actual fix was simpler than the
  three options first presented to มายด์ (wait / coordinate with nicole / accept the
  gap): the port collision was caused by `my-template`'s own `e2e/` suite hardcoding
  ports 24444-24446/24080, which `nicole`'s `my-delivery` (forked from `my-template`)
  copied unchanged — a purely local config value this repo already owned, not
  someone else's infrastructure to negotiate around. มายด์ corrected this framing
  directly. Fixed by moving the suite onto 25444-25446/25080 (verified free via
  `docker ps`/`ss` first) across `e2e/docker-compose.yml`, `global-setup.ts`,
  `hydra/hydra.yml`, `playwright.config.ts`, `specs/login.spec.ts`. Ran `make e2e` for
  real afterward, not just assumed the port change would work: **2/2 tests passed**
  (`assignee-picker-real-click`, `login.spec` — a real login round-trip through the
  local Hydra issuer), clean teardown. This is the story's goal-level done-when
  criterion now genuinely met, not deferred.
- Commits: `a85b6dd` (this ticket's own work), `a823bf5` (e2e port fix, orchestrating
  session).
