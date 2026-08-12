# Architecture standard

Applies to every milestone in this repo, not just milestone-1 — per
`AGENTS.md`'s rules hierarchy, `.chief/_rules/**` outranks any milestone's
`_goal`/`_contract`. This is where it lives specifically *because* it must
outlive milestone-1: the todo domain gets deleted on day one of every fork,
but whichever service replaces it inherits this layout whole.

Decided by มายด์ 2026-08-12 (module-first over layer-first), reasoning
recorded in milestone-1's `_plan/_todo.md` "Resolved during grill".

## Layout

```
internal/
  todo/          # example domain — delete this whole directory on fork
    handler.go  service.go  repo.go  todo_test.go
  identity/      # keep on fork — users, api_keys, actor resolution
    handler.go  middleware.go  service.go  repo.go
  platform/      # keep on fork — config, logging, db open, server wiring
    config.go  logging.go  db.go  server.go
```

One directory per domain module (`todo`, `identity`), each holding its own
handler + service + repo + tests together — not split across parallel
`handler/`, `service/`, `repo/` trees. This is my-task's pattern
(`src/server/modules/task/` keeps `task.service.ts` + `task.queries.ts`
together) and the reason to prefer it here is sharper than in my-task:
**forking this template means deleting one domain and adding another**, and
that operation must be `rm -rf internal/todo`, not "know that a domain also
has pieces living in three sibling folders."

## Dependency rules

Three rules, each with a reason a violation would actually cause:

1. **Only a module's handler file(s) may import `gin`.** Everything else in
   `internal/todo/` and `internal/identity/` — `service.go`, `repo.go`,
   anything else added later — must not import `github.com/gin-gonic/gin`.
   *Why it matters:* without this, service-layer logic accretes
   `gin.Context` parameters over time and becomes untestable without
   spinning up a router.

2. **Only a module's repo file(s) may import the sqlc-generated package.**
   Everything else — the handler included — must go through `repo.go`.
   *Why it matters:* without this, a handler can query the database
   directly and bypass whatever the repo layer is supposed to guarantee
   (e.g. ownership scoping, I3/I4 in milestone-1's `_contract/INVARIANTS.md`)
   — the exact bug class those invariants exist to prevent, entered through
   a different door.

   **Both are phrased as allowlists (who may), not denylists-with-an-
   -exemption (everyone except X), on purpose.** An earlier draft of this
   doc said "everything except `handler.go` may not import gin" — Clara
   caught that this breaks the moment `handler.go` gets renamed (a
   real event: forking this template means renaming things), because a
   file the check doesn't recognize as `handler.go` would then be *wrongly
   forbidden* from importing gin, while a file that inherits the sqlc-import
   permission simply because it wasn't named `handler.go` would slip through
   *unchecked* — the dangerous direction, a silent gap exactly where the
   rule matters most. The allowlist phrasing fails the other way instead:
   if nothing matches the pattern, gin/sqlc imports anywhere get flagged,
   which blocks the build loudly rather than passing silently.

3. **`internal/platform` never imports a domain module.** Dependencies
   point inward (domain → platform), never outward (platform → domain).
   *Why it matters:* without this, config/logging/server code accumulates
   domain-specific branches, and platform stops being the part that's safe
   to leave untouched on a fork.

## Enforcement

Rules 1 and 2 are **per-file**, not per-package — Go's package boundary is
the whole directory, so `handler.go` and `service.go` share one import list
at the `go list` level even though the rule only targets one of them. A
package-level check would false-positive on `handler.go`'s legitimate `gin`
import. Enforce with `go/parser` + `go/ast` (stdlib — no dependency beyond
the given stack) reading each file's own `import` block, not `go list`.

**The filename pattern is the contract, not an implementation detail.** A
file matches the handler role if its name is exactly `handler.go` or ends in
`_handler.go` (so a module can split into `todo_handler.go` +
`batch_handler.go` and both stay recognized); matches the repo role the same
way against `repo.go` / `*_repo.go`. **If you rename a module's transport or
data-access file to something that doesn't match — `http.go`, `storage.go`,
whatever fork convention you'd rather use — the test will start failing on
that file's own gin/sqlc import, on purpose, the moment it's built.** That
failure is the prompt to update `internal/architecture_test.go`'s two
patterns alongside the rename, not a bug in the check. Keeping the pattern
and the rename in sync is the fork's responsibility; the check's job is only
to make forgetting loud instead of silent.

Rule 3 **is** a clean package-level check — `internal/platform` is a
distinct package with its own import list, so `go list -json ./...`
inspecting `internal/platform`'s `Imports` for any `<module>/internal/todo`
or `<module>/internal/identity` entry is sufficient.

This lives as a Go test (`internal/architecture_test.go` or similar) that
fails the build on violation — a rule with no check is a rule that's only
true until the first person who doesn't know it's there. See milestone-1's
`_goal/GOAL.md` Done-when for the specific stopping condition and
`_plan/_todo.md` task-1 for which task owns writing it.
