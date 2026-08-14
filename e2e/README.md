# Browser e2e (TPL-3)

Real browser, real Hydra, real redirect/PKCE/callback chain — against a
**local** OIDC issuer this suite stands up and tears down itself, not
production SSO. See `global-setup.ts`'s own header comment for exactly
what it brings up and why (real Ory Hydra, not a mock — `oauth.go`
hardcodes Hydra's own `/oauth2/auth`/`/oauth2/token` paths rather than
doing OIDC discovery, so a generic mock issuer wouldn't match without
reshaping it into looking like Hydra anyway).

## Prerequisites

- **Docker.** Already a dependency of this template (`docker-compose.yml`,
  `docs/GETTING-STARTED.md`'s own "Running what you forked" section) —
  this suite doesn't add it, it uses what's already there to run a real
  local Hydra.
- **Go and Node**, same as the rest of this repo (`make tools`,
  `web/`'s own `npm ci`) — this suite builds the real `cmd/server` and
  `cmd/seed` binaries and the real SPA, the same way a deployment would,
  not a stand-in for either.

Nothing else. In particular, **not** a separately-installed Playwright
browser: `npm ci` below downloads it automatically (`package.json`'s own
`postinstall`), the same way `npm ci` in `web/` pulls the rest of that
project's dependencies. Verified genuinely from empty, not assumed: run
with `PLAYWRIGHT_BROWSERS_PATH` pointed at a directory that doesn't exist
yet if you want to confirm this on your own machine — `npm ci` creates it
and downloads into it, no separate `npx playwright install` step.

## Running it

```sh
cd e2e
npm ci
npm run test:e2e
```

One command after `npm ci`. `global-setup.ts` does the rest — brings up
Hydra and its login-consent stub, registers a real OAuth2 client against
it (`../scripts/register.sh`, unmodified — the same script a real
deployment uses), builds the SPA and the Go binaries fresh, seeds a
throwaway database with one owner row, starts the app, and only then lets
specs run. `global-teardown.ts` reverses every one of those steps after —
nothing is left running or left on disk (`e2e/.tmp/`, gitignored, deleted
every run; the Hydra/stub containers are brought up and torn down by
project name each run, never left up between runs).

## What this does and does not prove

**Proves**: the real login code path (`internal/transport/bff`) —
redirect to the issuer, PKCE, the callback, session issuance — works
against a real OIDC implementation. This is the one path nothing else in
this repo's test suite exercises (everything else injects the actor
directly or signs a session cookie in-process).

**Does not prove**: that your own, real, production Hydra deployment is
correctly configured. The issuer here is local and disposable, seeded
fresh every run — a green run here is evidence about the *code*, not
about *your* Hydra. Every spec states which issuer it ran against for
exactly this reason (`login.spec.ts`'s own first `console.log` line) —
never read a green run here as "the production SSO login works."

## Layout

- `docker-compose.yml`, `hydra/hydra.yml` — the local Hydra instance
  (real `oryd/hydra:v2.3`, sqlite-backed, torn down with `-v` every run).
- `fixtures/oidc-login-stub.mjs` — the login/consent auto-accept stub
  Hydra redirects to (it has no built-in login UI of its own). Test-only,
  never imported by `internal/` or `web/src/` — see its own header
  comment for what it does and why an auto-accepting stub is the right
  amount of realism here.
- `global-setup.ts` / `global-teardown.ts` — the full bring-up/tear-down
  lifecycle (own the sequencing directly rather than relying on
  Playwright's `webServer` option — see `global-setup.ts`'s own comment
  for why).
- `specs/` — the actual tests.

## Why this directory has its own `package.json`

Two separate reasons, not one: it keeps Playwright's dependency entirely
out of `web/package.json` (the SPA's own build tooling has no reason to
carry a browser-automation framework), and it keeps a Go template's own
top-level shape a Go template's shape — no `package.json`/`node_modules`
at the repo root just because one part of the test suite happens to be
written in TypeScript. `cd e2e` (or `make e2e` from the repo root, which
does exactly that) is the cost of that isolation, and it's a small one.
