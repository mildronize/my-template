# Deploy requirements

For hestia. Deployment beyond local `docker compose` (systemd, Caddy, DNS,
real Hydra client registration) is out of scope for milestone-1 — this
document is what that milestone hands off, so a real deployment doesn't
need a follow-up question (`.chief/milestone-1/_goal/GOAL.md` Done-when 9).

## Environment variables

The complete set — `internal/platform/config.go`'s `Config` struct, no
more, no fewer:

| Var | Required | Default | Meaning |
| --- | --- | --- | --- |
| `PORT` | no | `8080` | TCP port the HTTP server listens on. |
| `DATABASE_PATH` | no | `./data/app.db` | Filesystem path to the SQLite file. Its parent directory is created on startup if missing. **Must resolve to a path on a volume that survives container restarts and rebuilds** — see "SQLite volume" below. |
| `SSO_ISSUER` | only to enable the JWT path | *(empty)* | Hydra's issuer URL — used both to validate a Bearer JWT's `iss` claim and to locate its `/.well-known/jwks.json` for JWKS fetch+cache (contract §7.3, I7). Also used, when set, as the base URL `internal/transport/bff` builds `/oauth2/auth` and `/oauth2/token` from for the owner-login flow — one issuer var, two consumers. |
| `AUTH_AUDIENCE` | only to enable the JWT path, **and** required for owner login | *(empty)* | This service's own public URL. See "Audience convention" below — do **not** treat this as an opaque service name. `internal/transport/bff`'s `GET /callback` redirect URI is derived from this (`${AUTH_AUDIENCE}/callback`) — the same value Step 1's `scripts/register.sh` registered as `SERVICE_PUBLIC_URL`. |
| `SSO_CLIENT_ID` / `SSO_CLIENT_SECRET` | only to enable owner login (`internal/transport/bff`) | *(empty)* | This service's own Hydra OAuth2 client credentials for the `authorization_code`+PKCE owner-login flow (contract §2) — printed once by `scripts/register.sh` on success (see "Owner-login Hydra client registration" below). Leaving either unset is supported: the server still starts and the public API still works (`docs/GETTING-STARTED.md`'s walkthrough never touches `bff`) — `GET /login`/`GET /callback` just answer with a plain "owner login isn't configured yet" page instead of a working flow. |
| `SESSION_SECRET` | no (but strongly recommended for anything beyond one-off local testing) | *(none — generated per-process if unset)* | HMAC key `internal/transport/bff` signs/verifies its session and state cookies with (`DATA_MODEL.md`'s "BFF session" note — no server-side session store; the signature is the whole validity proof). If unset, `cmd/server` generates a random one at startup and logs a warning — the server still starts, but every existing owner session is invalidated on the next restart. Set this explicitly (and keep it stable across restarts/`chmod 600`) for any deployment where "stay logged in across a restart" matters. |

`SSO_ISSUER` and `AUTH_AUDIENCE` are a pair: `cmd/server` only builds a JWT
verifier when **both** are set (`main.go`'s `wireIdentity`). Leaving either
one unset is a supported, deliberate configuration — it disables the JWT
Bearer path and the service runs on API-key auth only, logging a warning
(`SSO_ISSUER/AUTH_AUDIENCE not set — JWT bearer path disabled, API-key auth
only`) rather than failing to start. **Read `docs/GETTING-STARTED.md`'s
"JWT seam" section before deciding to set these for a real deployment** —
as of this milestone the JWT path is wired-but-dormant by design, not a
live SSO integration hestia should turn on by default.

**`AUTH_AUDIENCE`/`SSO_ISSUER` being dormant-by-design for the JWT path
above is a separate question from whether owner login works.** Owner login
(`internal/transport/bff`) needs `SSO_ISSUER` + `AUTH_AUDIENCE` +
`SSO_CLIENT_ID` + `SSO_CLIENT_SECRET` all set — see "Owner-login Hydra
client registration" below, which is needed unconditionally, unlike the
JWT-Bearer client registration section further down.

A `.env` file in the working directory is loaded first if present
(`godotenv`); real environment variables always take precedence over it.

## SQLite volume

No separate database container or process — SQLite is a file
(`GOAL.md` Decisions: "Deployment"). Requirements:

- A persistent volume (or bind mount) covering `DATABASE_PATH`'s parent
  directory, mounted **before** the service starts.
- `docker-compose.yml`'s own convention: `DATABASE_PATH` is left at its
  default (`./data/app.db`, which resolves to `/app/data/app.db` inside
  the container, since the image's `WORKDIR` is `/app`), and the named
  volume `sqlite-data` is mounted at `/app/data`. Follow this same
  convention for a non-compose deployment — mount persistent storage at
  `/app/data` rather than overriding `DATABASE_PATH` to a different path,
  unless there's a specific reason to.
- The service applies its own schema migrations automatically on every
  startup (`internal/platform/migrate.go`, `goose.Up` against the
  migration set embedded in the binary) — a fresh, empty volume needs no
  separate migrate step, and re-running against an already-migrated
  volume is a safe no-op (goose tracks applied versions itself).
- Back up the file at `DATABASE_PATH` like any other stateful file (it's
  the entire database — every table this service owns, in one file). No
  replication or clustering story exists or is in scope.

## Seeding the first agent API key

There is deliberately no `POST /api/v1/keys` — key issuance is CLI-only
(`cmd/issue-key`, mirrors my-task's `agent:add`). Run it once per agent
that needs one, against the running deployment's own database:

```sh
docker compose exec app /app/issue-key <handle>
```

(`app` is `docker-compose.yml`'s service key in this template — Step 3 of
`docs/GETTING-STARTED.md` lets a fork rename it, so substitute the fork's
actual service key if it's not `app`.)

(or, outside compose, `DATABASE_PATH=<path> ./issue-key <handle>` with the
built binary run directly against the same file the server uses). The raw
key prints to stdout **exactly once** and is never stored anywhere
recoverable (I8) — copy it immediately. Losing it means rotating (run the
command again for the same handle) or revoking
(`DELETE /api/v1/keys/:id`), not retrieving.

## Owner-login Hydra client registration (`scripts/register.sh`)

Needed **unconditionally**, unlike the JWT-Bearer client registration
below — `internal/transport/bff`'s owner-login flow (`GET /login`,
`GET /callback`, `authorization_code` + PKCE per `sso-consumer-
contract.md` §2) has no working path without a registered client, and
that's true even for a local-only deployment that never touches a real
domain. This is a **different** Hydra client than the JWT-Bearer one in
the section below — that one is for machine/agent identity (§3,
currently dormant by design); this one is for a human owner proving
identity through a browser, which is Hydra's ordinary, always-live use
case. Registering this one is not gated on the JWT-path caution below.

This template ships **no** Hydra client of its own — per contract §6 a
stable `--id` is `<service>`/`<service>-dev`, derived from a service name
that doesn't exist until a fork has actually named itself
(`.chief/milestone-2/_goal/GOAL.md`'s "Client registration" Decisions
row). `scripts/register.sh` is what a fork runs, once per environment, to
create its own client. It is a generalized, placeholder-driven adaptation
of `prod-thw-home`'s `deploy/sso/scripts/register-my-task.sh`, preserving
every one of that script's safety properties: refuses to overwrite an
existing client, reads the registration back from Hydra rather than
trusting the create call's own output, probes the authorize endpoint with
both a registered and an unregistered redirect URI, and **prints** the
resulting env values rather than writing them anywhere (a re-run can
never silently clobber a working config).

**Set before running** (see the script's own header comment for the full
description of each):

| Var | Meaning |
| --- | --- |
| `SERVICE_NAME` | This service's stable name (`docs/GETTING-STARTED.md` Step 3) — becomes `<SERVICE_NAME>-dev` / `<SERVICE_NAME>` per contract §6's stable-id convention. |
| `SERVICE_PUBLIC_URL` | This service's own public URL **for the environment being registered right now** — becomes both the Hydra client's `audience` and the base of its redirect URI (`${SERVICE_PUBLIC_URL}/callback`). Same value `AUTH_AUDIENCE` (below) must be set to at runtime. Never an opaque name. |
| `SSO_ISSUER` | The issuer URL to report back — same value this service's own `SSO_ISSUER` config var must be set to. Echoed in the script's printed output only; the script itself never calls it. |
| `HYDRA_ADMIN_URL` | Hydra's Admin API, reachable from wherever the script runs (e.g. `http://127.0.0.1:4445`). Not defaulted — this template doesn't assume any particular host/fleet. |
| `HYDRA_PUBLIC_URL` | Hydra's public endpoint, used only by the script's authorize-probe (e.g. `http://127.0.0.1:4444`). |
| `ENV` | `dev` or `prod` — no default. Run the script once per environment; dev and prod get separate clients with separate audiences. |
| `CLIENT_SECRET` (optional) | An existing secret, to make a rebuild transparent to this service. Pass via environment, never as an argument. |
| `HYDRA_IMAGE` (optional) | Defaults to `oryd/hydra:v2.3`. |

```sh
ENV=dev \
  SERVICE_NAME=<your-fork's-service-name> \
  SERVICE_PUBLIC_URL=<this-service's-own-public-url-for-dev> \
  SSO_ISSUER=<your-idp's-issuer-url> \
  HYDRA_ADMIN_URL=<hydra-admin-api-url> \
  HYDRA_PUBLIC_URL=<hydra-public-endpoint-url> \
  ./scripts/register.sh
```

The script prints `SSO_ISSUER`/`SSO_CLIENT_ID`/`SSO_CLIENT_SECRET`/
`AUTH_AUDIENCE` once, on success — paste those into this service's own
deployment config (`.env`, compose file, systemd unit) yourself. See
`docs/GETTING-STARTED.md` Step 1.

## Real Hydra client registration (JWT path)

Only needed if a real deployment decides to turn on the JWT Bearer path —
see the warning under "Environment variables" above about whether that's
appropriate yet for this service. If it is, register a Hydra client
following `~/gits/prod-thw-home/docs/sso-consumer-contract.md` §6
("Client registration conventions") exactly — this document does not
restate that contract, it points at it:

- **Stable `--id`**: `<service>` and `<service>-dev`, never a generated
  UUID (contract §6) — a UUID makes every rebuild change values consumers
  hold.
- **`--access-token-strategy jwt`**, always — the `opaque` default fails
  JWKS validation outright (contract §6).
- **Audience = this service's own public URL, per contract §6's "Audience
  convention" — see the row below.** Not an opaque name.
- One audience per service per environment — dev and prod must differ, so
  a dev-minted token is never accepted by prod (contract §6).
- Registration is scripted and version-controlled in `prod-thw-home`, not
  done ad hoc against this repo — the registration script prints a
  copy-pasteable env block and never writes to this service's `.env`.

### Audience convention (`GOAL.md` Decisions table, "Audience convention" row)

`AUTH_AUDIENCE` must be **the service's own public URL**
(e.g. `https://your-service.example.com` — a generic placeholder, not
this template's own deployed URL, which a fork must never copy), one per
service per environment
— never an opaque name (contract §6, "Audience convention: the service's
public URL"). A forked service registering its own Hydra client must set
`AUTH_AUDIENCE` to *its* real deployed URL, not copy this template's
value, and must re-register whenever that URL changes (a domain change
already forces re-registration for `redirect_uri`'s sake, so an opaque
audience wouldn't have saved anything here — contract §6).

### Before registering: read the "Resolved during grill" note

As of this milestone, hestia should **not** create a Hydra client for this
service (`.chief/milestone-1/_plan/_todo.md`, "Resolved during grill",
entry "Closed 2026-08-12"): Hydra's Admin API (4445) is still
unauthenticated — any local process can register a client with a `sub` of
its choosing and receive a correctly-signed token, `aud` doesn't help
(contract §7.7) — so a real Hydra-issued machine identity for this kind of
service would be forgeable today. Re-check that note (and
`docs/GETTING-STARTED.md`'s JWT-seam paragraph) for whether that's still
true before registering anything.

### Key rotation: no restart needed after an SSO rebuild (I7)

`sso-consumer-contract.md` §7.4 says to "expect a restart after an SSO
rebuild, and say so in the runbook" — **that expectation is superseded
for this service specifically**, as of milestone-2. Do not schedule a
restart for this service when Hydra's signing key rotates.

`internal/identity/jwt.go`'s JWKS lookup (`rs256KeyProvider.FetchKeys`)
forces exactly one `jwk.Cache.Refresh()` + retry whenever a token's `kid`
isn't found in whatever this service currently has cached, before
failing. In practice: the first request bearing a token signed by a
freshly-rotated Hydra key still verifies correctly — it costs one extra
JWKS fetch against the issuer, not an outage. This closes the gap that
motivated §7.4's contract-wide advice in the first place (`jwk.Cache`
only refetches on its own schedule, up to 15 minutes, never lazily on a
miss) — see `_rules/_contract/INVARIANTS.md`'s I7 for the full reasoning,
including why the retry is bounded to exactly one attempt (a forged/
random `kid` cannot force repeated Hydra calls).

If this service ever stops using `jwk.Cache` this way, or the retry is
removed, §7.4's original restart expectation applies again — until then,
it does not.

**Known, unfixed property: bounded per-call, not bounded across calls.**
The retry above stops a *single* bad-`kid` request from forcing more than
one extra issuer hit. It does nothing about N separate requests, each
carrying a different unknown `kid` — that's still N issuer fetches, no
negative-caching, no coalescing. Deliberately not fixed here: the obvious
fix (cache "this kid doesn't exist") reintroduces the exact 15-minute
outage this task just removed, in miniature, the moment a legitimately
new key rotates in and gets refused because it looks like a cached
negative. This is the same property hestia found in `shipd` and Clara
filed as **FLEET-2** — a fleet-wide question (safe coalescing vs. unsafe
negative-caching), not something to solve per-consumer. If this matters
for your deployment's threat model, check FLEET-2's resolution before
inventing a local fix.

## Reverse proxy / TLS / process supervision

Out of scope for this milestone entirely (`GOAL.md` Context: "systemd,
Caddy, DNS ... is hestia's domain"). This service listens on plain HTTP on
`PORT` and expects a reverse proxy in front of it for TLS — no built-in
TLS termination.
