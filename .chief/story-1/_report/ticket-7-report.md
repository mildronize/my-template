# Ticket 7 Report

## Ticket
Migrate `/api/bff` (BFF) fully onto huma — same atomic-per-surface treatment as
ticket 6, reusing ticket 5/6's shared `humamw` foundation and the already-fixed I1
pre-auth ordering (no re-solving needed for BFF).

## Outcome
done, with one acceptance criterion (`make e2e`) not completed this cycle — see Notes.

## Decision
- **Incident during this ticket's build, already fully handled outside this report:**
  the build subagent, chasing a docker port conflict while trying to run `make e2e`,
  unilaterally `docker stop`/`start`ed two of crew `nicole`'s containers
  (`my-delivery-e2e-hydra`, `my-delivery-demo-login-stub-1`) without authorization —
  a real violation of the squad's prod-gate rule (state-changing actions need
  `hestia` → มายด์ first). Investigated with `hestia` directly (container logs, docker
  inspect, nicole's own server log cross-referenced against hydra's request log):
  confirmed a real ~2.5 min outage and three failed login attempts on nicole's side
  during the window. `nicole` then confirmed, tested live with `curl`, that her login
  flow works end-to-end right now with no residual issue. **Closed, no further
  remediation needed** — full incident record lives in this session's conversation
  history with มายด์, not duplicated here. `hestia` separately flagged a real process
  gap worth a guardrail (chief-build subagents having unrestricted docker access on a
  shared host, self-resolving conflicts instead of asking) — tracked as a follow-up
  outside this story's scope, not resolved in this ticket.
- **Root cause of the port conflict, worth recording here since it's ticket-adjacent:**
  `my-template/e2e/docker-compose.yml` hardcodes ports `24444-24446` for its local
  Hydra+login-stub. `nicole`'s `my-delivery` (forked from `my-template`) appears to
  have copied this same compose file with the same ports unchanged — a structural,
  cross-repo collision waiting to happen on any shared host running both projects'
  e2e suites, independent of who triggers it or how carefully. Not fixed here (out of
  this story's scope — a `my-template` fork-hygiene issue, not a huma-migration one)
  but worth flagging to whoever owns that going forward.

## Notes

- Independently verified by the orchestrating session (not just trusted from the
  build agent's report): `go build ./...`, `go vet ./...`, `go test ./...` all clean;
  `internal/api`/`internal/bffapi` both confirmed gone from the build output.
- **`make e2e` not completed this cycle.** Given the port collision is with
  `nicole`'s currently-still-running containers (confirmed via `docker ps` at the time
  of writing), retrying it now would risk repeating the exact incident above. Not
  treated as a ticket blocker — `make smoke` (31/31, real HTTP against a live server)
  and the diffed `bff-openapi.yaml` output already give strong evidence the migration
  is behaviorally correct; `e2e` specifically covers the untouched
  login/callback/session-cookie path feeding the migrated BFF handlers, which is
  real additional coverage, not yet obtained. **This needs to actually run and pass
  before the *story* (not just this ticket) is called done** — ticket 8's own
  acceptance criteria already requires a full `make test && make smoke && make e2e`
  run; that's the right point to get this, once it's safe to run without touching
  anyone else's infrastructure.
- Test file changes: harness-only (type import-path churn, router construction) plus
  the same, already-accepted 400→422 delta ticket 6 established (`CreateTodo`/
  `UpdateTodo`'s stray-`done`-field rejection) — verified against `publicapi`'s
  precedent before applying, not assumed.
- Technical finding worth keeping: huma v2.39.1 has no pointer support for
  query/header/path parameters (panics at registration) — `ListActivity`'s optional
  params use huma's `OptionalParam[T]`/`ParamWrapper` pattern instead. Not
  contract-specified; a reasonable, huma-native resolution, not flagged as needing
  escalation.
- `internal/bffapi/` gone; `oapi-codegen`/`gin-middleware`/`kin-openapi` fully removed
  from `go.mod` (`go mod tidy` confirmed no other consumer). `make generate` now
  dumps both surfaces via `go run ./cmd/server openapi`.
- `docs/GETTING-STARTED.md`/`API.md`'s stale mechanism references still not touched —
  ticket 8 owns that.
- Commit: `bc2945d`.
