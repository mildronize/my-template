# Architecture standard

Applies to every milestone in this repo, not just the one that last
touched it — per `AGENTS.md`'s rules hierarchy, `.chief/_rules/**` outranks
any milestone's `_goal`/`_contract`. Lives here because it must outlive
whichever milestone last shaped it: the example domain gets deleted on day
one of every fork, but whichever service replaces it inherits this layout
and its enforcement whole.

**Superseded 2026-08-13 (milestone-2).** The milestone-1 version of this
document (module-first, `internal/{todo,identity,platform}`, transport
fused into the domain module) is retired, not amended in place — the
change is structural, not incremental. Root cause recorded in
milestone-2's `_goal/GOAL.md` Context: milestone-1 planned for exactly one
transport surface, so fusing `handler.go` into the domain module never
surfaced a problem until a second surface (a public API distinct from an
owner-facing one) needed somewhere to live and there was nowhere to put it.

## Contract promotion convention

Applies to any `.chief/<milestone>/_contract/*.md` file, not just this
one — recorded here because this file was the first thing promoted this
way (milestone-1 → milestone-2) and there wasn't yet a place documenting
the convention itself.

When a milestone-scoped contract turns out to be cross-milestone (the
schema, invariants, or API surface it defines outlives the milestone that
wrote it, rather than being replaced), promote it to
`.chief/_rules/_contract/`, per `AGENTS.md`'s own directory convention.
The milestone's original copy stays in the tree — Chief's convention is
that milestone artifacts are history, not deleted — but gets a one-line
"superseded, promoted to `_rules/_contract/X.md`" notice at the top, and
from that point on:

**The milestone copy is prose for a human reading history. It must never
be a second input to anything that reads a contract programmatically.**
A superseded document silently continuing to drive a live check has two
concrete failure modes, both real (found reviewing milestone-2's own
first promotion): editing the "historical" copy — for accuracy, say —
quietly changes what a live check requires, and a fork that accumulates
one contract copy per milestone can only ever *add* to what's required,
because a requirement a later milestone deliberately retired is still
named by an old copy nobody's re-reading for that purpose. Any check that
discovers its contract by globbing (`internal/invariants_test.go`'s
`findInvariantsFiles` is the current example) must check for a promoted
file first and, if one exists, treat it as the *only* input — falling
back to glob-and-union across `.chief/` only for a fork that has never
promoted anything, where "one file per milestone" is still the real shape.

## Layout

```
internal/
  domain/
    todo/            # example domain — delete this whole directory on fork
      service.go  repo.go  service_test.go  repo_test.go
  identity/          # keep on fork — users, api_keys, actor resolution,
                      # key lifecycle. NOT under domain/ — see below.
    service.go  repo.go  jwt.go  *_test.go
  transport/
    publicapi/       # REST for agents/skills, key-authenticated
      todo_handler.go  keys_handler.go  me_handler.go  *_test.go
    bff/             # owner login + minimal authenticated view,
                      # session-authenticated
      login_handler.go  callback_handler.go  *_test.go
  platform/          # keep on fork — config, logging, db, server wiring,
                      # cross-cutting gin middleware shared by every engine
    config.go  logging.go  db.go  server.go  middleware.go
web/                 # repo root, Go convention — not under internal/
```

## Why identity is not under `domain/`

my-task has no identity *module* at all — `src/server/modules/` holds only
`task/`; identity lives in `src/server/lib/`, structurally separate from
the thing "modules" means there. This repo's `domain/` directory means
"delete this on fork, add your own" — identity is never that. Every domain
module depends on identity (to resolve who's calling); identity depends on
no domain module; and it is not deleted or replaced when a fork swaps its
example domain for a real one. Filing it under `domain/identity/` would
have said all three of those things were false.

## Why transport is not inside a domain module anymore

Milestone-1 kept `handler.go` inside the domain module (`internal/todo/
handler.go`) on the reasoning that a domain's transport, service, and data
access should live together so forking is `rm -rf internal/todo` in one
command. That reasoning held for exactly one transport surface. It broke
the moment a second surface (`publicapi` vs `bff`, milestone-2's own item 2)
needed to expose the *same* domain differently — there was nowhere to put
a second handler for one domain module, and `ARCHITECTURE.md` rule 1
("only handler.go may import gin") stopped meaning anything once "the
handler" stopped being singular.

Transport now lives entirely in `internal/transport/{publicapi,bff}`,
outside every domain module. A domain module (`internal/domain/*`) holds
only `service.go`+`repo.go` (+tests) — business logic and data access,
transport-agnostic. Forking is still one directory deletion
(`rm -rf internal/domain/todo`); it's just no longer also the directory
that knows how to serve HTTP.

## Dependency rules

Five rules now, not three — the split into `domain/`+`transport/`+
`identity` (previously two categories: "domain" and "platform") adds real
new edges that need real new rules, not just a rename of the old three.

1. **A domain module (`internal/domain/*`) never imports `gin` — no
   allowlist, no handler-role exemption, because there is no handler
   inside a domain module anymore.** Everything transport-facing moved to
   `internal/transport/`. *Why it matters:* unchanged from milestone-1 —
   without this, business logic accretes `gin.Context` parameters and
   becomes untestable without a router. Simpler to enforce now than
   milestone-1's version, because there's no longer a legitimate in-domain
   file that needs an exemption.

2. **Only a module's repo file(s) may import the sqlc-generated package.**
   Unchanged from milestone-1, still an allowlist (not
   denylist-with-exemption) for the same reason as before — see
   "Enforcement" for the exact pattern and why it's phrased this way.
   Applies inside both `internal/domain/*` and `internal/identity/`.

3. **A domain module never imports a sibling domain module.**
   `internal/domain/todo` may import `internal/identity`; it may not
   import `internal/domain/<anything-else>`, and no future domain module
   may import `internal/domain/todo` either. *Why it matters:* this is
   what makes "fork = delete one domain, add another" actually true —
   the moment one domain depends on another, deleting either breaks the
   one left behind, and the whole reason domains live in separate
   directories (independent forkability) stops holding. Not exercised by
   milestone-1 (there was only one domain module, so nothing could violate
   it) — real the moment a second domain module exists, which milestone-2
   doesn't add but a future fork will.

4. **A domain module or `internal/identity` never imports
   `internal/transport/*`.** Dependencies point one way: transport depends
   on domain and identity, never the reverse. *Why it matters:* without
   this, business logic could grow a dependency on a specific transport
   surface's types (a gin-bound request struct, an HTTP status code
   chosen for one surface's conventions), silently coupling logic that's
   supposed to be shared by both `publicapi` and `bff` to one of them.

5. **`internal/platform` never imports a domain module, `internal/identity`,
   or `internal/transport/*`.** Unchanged in spirit from milestone-1's rule
   3, extended to cover the two new top-level directories. *Why it
   matters:* unchanged — platform stays the layer safe to leave untouched
   on a fork only if it never accumulates branches specific to what's
   above it.

## Cross-cutting transport concerns live in `platform/`, not a shared
## transport package

`publicapi` and `bff` are both gin engines and both need panic recovery,
request logging, and request-ID propagation — real shared concerns,
neither domain nor identity nor policy specific to either surface.
**Decided against a `transport/shared/` package for these** (milestone-2
grill: proposed, then dropped — no concrete shared concern existed to
justify it when the two surfaces were first compared, and an empty
category-named package invites becoming a junk drawer for whatever fits
nowhere else). Once the concrete concern existed (both engines need the
same three middlewares), it went into `platform/middleware.go`, applied to
both engines from `platform/server.go` — infrastructure about how *any*
gin engine in this service behaves, which is what `platform/` already
means, not a new kind of package. This is the layer every forked service
inherits without thinking about it; if a future need turns out not to fit
"infrastructure every engine needs," reconsider then, named for what it
actually is — don't pre-declare a home for it now.

## Enforcement

Rules 1 and 2 are **per-file**, not per-package, for the same reason as
milestone-1: Go's package boundary is the whole directory, so two files in
`internal/domain/todo/` share one import list at the `go list` level. Rule
1 no longer needs a handler-role allowlist (see above — no handler lives
in a domain module now), so it's a flat "no file in this directory imports
gin" check. Rule 2 keeps the allowlist pattern from milestone-1
(`repo.go`/`*_repo.go`, with a `(_test)?` suffix — see milestone-1's
`_plan/_todo.md` "Resolved during grill" for why that suffix exists and
why the pattern is an allowlist, not a denylist-with-exemption; that
reasoning is unchanged, only the file's location moved).

Rules 3, 4, and 5 are clean package-level checks via `go list -json ./...`
— domain-module and transport-surface names are each their own Go package
with their own import list, so inspecting `Imports` per package is
sufficient, the same technique milestone-1's rule 3 already used.

**Domain module discovery stays filesystem-derived, not hardcoded** — the
milestone-1 lesson (`_plan/_todo.md` "Resolved during grill", the
architecture-guardrail hardcoding bug found by a blind fork test): every
directory under `internal/domain/` counts as a domain module, full stop,
no maintained list anywhere. The same principle now also applies to
transport surfaces (`internal/transport/*`) for rule 4's check.

This lives as a Go test (`internal/architecture_test.go` or equivalent)
that fails the build on violation. A rule with no check is a rule that's
only true until the first person who doesn't know it's there — verify any
change to this file's rules the same way every prior guardrail change in
this repo has been verified: clone to a scratch location, construct the
specific violation the rule claims to prevent, confirm the check fails
loud and names the right file and rule. See milestone-2's `_goal/GOAL.md`
Done-when for the specific stopping conditions and `_plan/_todo.md` for
which task owns rewriting this check for the new layout.
