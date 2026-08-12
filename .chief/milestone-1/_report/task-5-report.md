# Task 5 Report

## Task
Dockerfile, docker-compose, `docs/DEPLOY-REQUIREMENTS.md`,
`docs/GETTING-STARTED.md`, fix `internal/invariants_test.go`'s hardcoded
Done-when-12 range (Clara's finding from task-4's review), final full-suite
pass.

## Outcome
done — milestone-1 complete (all 12 Done-when items independently verified,
not just reported).

## Decision

- **Issue:** a fresh named Docker volume mounts root-owned by default, and
  the runtime image runs as `nonroot` — caused `SQLITE_CANTOPEN` on first
  `docker compose up`.
- **Options considered:** run the container as root (rejected — no reason
  to weaken the runtime image's posture for this), pre-create and chown the
  data directory in the build stage and copy it into the runtime image with
  matching ownership (chosen), or add an init container to fix permissions
  at startup (more moving parts than needed for a single-volume template).
- **Chosen:** pre-create `/app/data` chowned to `65532:65532` in the build
  stage, `COPY --chown` it into the runtime stage. Verified fixed by
  actually running `docker compose up` afterward, not just reasoning about
  the fix.

## Notes

- **Independently re-verified (chief-agent/Luna), including the parts most
  worth not trusting on report alone:**
  - Ran `docker compose up --build` myself from a clean state, seeded a key
    via `docker compose exec app /app/issue-key smoketest`, confirmed
    `GET /api/v1/me` returns `{"handle":"smoketest","role":"agent",
    "active":true}`, then tore down with `docker compose down -v`.
  - Read `internal/invariants_test.go` directly — confirmed it now parses
    `_contract/INVARIANTS.md` for `**I<N> —` headings rather than a
    hardcoded range (Clara's task-4 finding, fixed here as promised).
  - Grepped `docs/GETTING-STARTED.md` directly for all required elements:
    four separately-labeled `## Step N:` headings, the JWT-seam paragraph
    (must match `_todo.md`'s approved wording verbatim — confirmed),
    the Idempotency-Key/pagination reconsider-if framing, and the
    invariant-requires-both-a-doc-entry-and-a-test line.
  - `go test ./...` clean independently (not just trusting the builder
    report).
- **Milestone completion — all 12 Done-when items checked against actual
  state, not assumed from task reports:**
  1. build/vet clean, toolchain pinned — **hold**
  2. `goose up` full migration set — **hold**
  3. `sqlc generate`/`oapi-codegen` zero diff — **hold**
  4. ownership-scoping tests pass — **hold**
  5. owner-role rejected on both credential paths — **hold**
  6. uniform 401 — **hold**
  7. OpenAPI spec validation rejects bad requests — **hold**
  8. `docker compose up` + seeded-key `/me` — **hold, re-verified live**
  9. `docs/DEPLOY-REQUIREMENTS.md` complete — **hold**
  10. `docs/GETTING-STARTED.md` — structural presence confirmed; **actual
      sufficiency is explicitly NOT this check's job** — see Human
      acceptance below
  11. architecture import-graph test — **hold**
  12. every invariant has a named test, derived dynamically from
      `INVARIANTS.md` — **hold, fix independently confirmed**
- **Human acceptance (per `_goal/GOAL.md`, deliberately not a stopping
  condition):** Clara is running this herself with a context-free agent —
  spawned with only the instruction to read `docs/GETTING-STARTED.md` and
  fork the repo into a differently-named service with its own resource in
  place of `todos`, with no access to `.chief/`, task reports, or anyone to
  ask. She explicitly disqualified herself from running this personally,
  since she's read every planning document today and would pass using
  context the doc doesn't actually provide — exactly the
  `rigour-mechanisms-certify-instead-of-testing` failure mode this
  human-acceptance step exists to avoid. Wherever that agent gets stuck is
  what the doc is missing.
- Commits: `8cc51b9`, `271c4bd`, `7bd2235` on
  `milestone-1/tpl-1-init-template`.

No blockers, no escalation.
