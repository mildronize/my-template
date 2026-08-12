# Milestone 1 TODO

See `../_goal/GOAL.md` for objective/scope/done-when and
`../_contract/` for API/DATA_MODEL/INVARIANTS. Order matters — each task
depends on the one before it. Every Done-when item is owned by exactly one
task below — none are left to "the full test suite passing" as an implicit
catch-all (per Clara's review: an unowned stopping condition can close a
milestone without anyone ever having deliberately done it).

- [x] task-1: Scaffold — `cmd/server`; `internal/platform/` (config via
      `caarlos0/env/v11` + `godotenv`, logging via `slog`+`tint`, db-open,
      server wiring); empty `internal/todo/` and `internal/identity/`
      directories (populated by task-2/task-3); a health endpoint; goose
      wired to an empty migration; sqlc wired to an empty query file.
      Layout follows `.chief/_rules/_standard/ARCHITECTURE.md` (module-first
      — see that doc for why, and the rationale under "Resolved during
      grill" below). Write the import-graph test now, against the empty
      skeleton, so it fails immediately if a later task puts a file in the
      wrong place instead of surfacing at task-5.

      **Also pin the codegen toolchain — none of sqlc/goose/oapi-codegen
      exist on this machine (verified: not on `PATH`, `~/go/bin` doesn't
      even exist), and Go's install-latest-if-missing behavior is worse than
      failing outright: Done-when 3 requires `sqlc generate` to be
      byte-reproducible, which an unpinned version can't guarantee — the
      exact failure mode in the fleet's `generated-output-must-match-its-
      source-revision` learning.** Use a `tools/tools.go` (`//go:build
      tools`) blank-importing each tool's `cmd` package, so exact versions
      live in `go.mod`/`go.sum` — not on whichever machine happens to run
      the loop. A Makefile/script target `go install`s from those pinned
      versions. `go build ./...` and `go vet ./...` pass.
      **Owns: Done-when 1, 11.**
- [x] task-2: Identity — `internal/identity/`: `users`/`api_keys`
      migrations + sqlc queries (DATA_MODEL.md), `handler.go` +
      `middleware.go` (actor-resolution: API key → JWT → reject, per
      INVARIANTS.md I1–I2, I6–I10) + `service.go`/`repo.go`, the CLI
      key-issuance script. Unit tests for every invariant in that list,
      including I2 on both credential paths, and that every failure mode
      (missing/malformed/expired/revoked/wrong-role) returns the identical
      401 body (I5).
      **Owns: Done-when 5, 6.**
- [ ] task-3: Todos — `internal/todo/`: `todos` migration + sqlc queries
      (last new table this milestone adds — nothing after this task changes
      the schema), `openapi.yaml` for `/me` and the `/todos` endpoints,
      oapi-codegen + gin-middleware wiring, `handler.go`/`service.go`/
      `repo.go` (ownership scoping, I3, I4 — `handler.go` must not import
      the sqlc package directly, only through `repo.go`, per the
      architecture standard). Integration tests against a real SQLite file.
      Verify `goose up` applies cleanly on a fresh empty file with the full
      migration set. This is also the task after which every invariant
      I1–I10 should have at least one test named after it (task-2 supplies
      I1,I2,I5–I10; this task supplies I3,I4) — write the grep-based check
      now and let it confirm that, rather than assuming it.
      **Owns: Done-when 2, 4, 7, 12.**
- [ ] task-4: Settings — `GET /api/v1/keys`, `DELETE /api/v1/keys/:id`
      added to `openapi.yaml` and implemented inside `internal/identity/`
      (new sqlc queries against the existing `api_keys` table — last task
      this milestone adds queries in, so this is where "sqlc generate
      produces no diff" gets verified for the complete set). Same
      ownership-scoping rule as todos. Tests for I3 applied to keys, I9
      (expired-but-unrevoked still 401s).
      **Owns: Done-when 3.**
- [ ] task-5: Ship it — Dockerfile, docker-compose (service + SQLite
      volume), `docs/DEPLOY-REQUIREMENTS.md`, `docs/GETTING-STARTED.md`.
      The getting-started doc's fork checklist must explicitly call out the
      two deliberate simplifications from `_contract/API.md` Conventions
      (no Idempotency-Key, no pagination) as things to reconsider if a fork
      adds an event log or a list that can grow large — a forker reads this
      doc, not a stale milestone's API.md. Full `go test ./...` and
      `docker compose up` smoke pass.
      **Owns: Done-when 8, 9, 10.**

## Resolved during grill

The JWT/SSO auth path's framing was open (live agent auth vs. a
wired-but-dormant seam per `sso-consumer-contract.md` §3) — **มายด์ confirmed
2026-08-12: dormant seam.** Task-2's JWT-path code and I2/I6/I7/I9/I10 tests
are unaffected (test issuer, not live Hydra). The human-acceptance criterion
asking someone to hit the service with a real Hydra JWT is cut from
GOAL.md. Task-5's `docs/GETTING-STARTED.md` should describe the JWT path as
wired-but-dormant, matching GOAL.md's Context section — not as this
template's live SSO integration.

**Closed 2026-08-12 — hestia confirmed directly against the deployed
docker-compose.yml (main, commit `8126de5`), not the 3-day-old contract
doc:** Hydra's Admin API (4445) is still unauthenticated, bound to
`127.0.0.1` only (not exposed, but any local process can reach it), and the
compose file is shared by thw-home and thw-home-prod — same gap on both
hosts. The fix (unix socket + filesystem permissions) is designed
(`agent-identity-gaps.md` #4) but not built. hestia: do not create a Hydra
client for TPL-1.

**Architecture layout (2026-08-12):** Clara found that no goal/contract/plan
file made the `internal/{handler,service,repo,middleware}` folders mean
anything — no rule, no invariant, no Done-when touched them, so a
passthrough handler→sqlc shortcut would have passed all 10 original
Done-when items. This belongs above milestone-1 (per `AGENTS.md`'s rules
hierarchy, `.chief/_rules/**` outranks a milestone, and this layout must
outlive the todo domain it's demonstrated on) — written to
`.chief/_rules/_standard/ARCHITECTURE.md`. มายด์ chose module-first over
layer-first: `internal/{todo,identity,platform}`, each domain module holding
its own handler+service+repo together, so forking is `rm -rf internal/todo`
in one command rather than knowing a domain also lives in sibling trees.

One refinement made while writing the standard, worth flagging since it
adjusts (not reverses) Clara's proposed mechanism: her two within-module
rules ("todo/identity must not import gin", "handler.go must not import
sqlc directly") can't be checked with a plain `go list -json` **package**
query, because `handler.go` and `service.go`/`repo.go` share one directory
and therefore one import list at the package level — a package-level check
would false-positive on `handler.go`'s legitimate `gin` import. Both rules
are enforced **per-file** instead, via `go/parser`+`go/ast` (still
stdlib-only, no dependency added). Only the third rule
(`internal/platform` importing no domain module) is a clean package-level
`go list -json` check, since platform is its own directory. Full mechanism
in `.chief/_rules/_standard/ARCHITECTURE.md`'s Enforcement section.

**Preflight + commit (2026-08-12):** Clara ran two checks loop-readiness's
static review couldn't — actually probing the machine and the repo's git
state. Neither tool (`sqlc`/`goose`/`oapi-codegen`) exists on `PATH` or in
`~/go/bin` (doesn't exist), and none of today's planning work had been
committed. Fixed: planning work is now `main`'s root commit
(`b988194`) — a clean baseline to diff `chief-loop`'s output against — and
task-1 gained the `tools/tools.go` pinning requirement (Decisions table,
Done-when 1). See `task-2.md` for the identity/auth task spec loop-readiness
called for.

Task-5's `docs/GETTING-STARTED.md` should use this exact paragraph for the
JWT seam (per working standard rule 5 — a claim about mutable state carries
its read-time, so a future reader knows to re-check rather than trust it
indefinitely):

> This JWT seam is wired and unit-tested, but **dormant by design**, per
> `sso-consumer-contract.md` §3 — Hydra does not issue machine identity to a
> local process, because its Admin API has no authentication: any process on
> the same host can register a client with a `sub` of its choosing and
> receive a correctly-signed token (`aud` doesn't help either — §7.7).
> **Confirmed against the actually-deployed compose config on 2026-08-12 by
> hestia.** Read §3's "when to revisit" before turning this on.
