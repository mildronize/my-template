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
| `SSO_ISSUER` | only to enable the JWT path | *(empty)* | Hydra's issuer URL — used both to validate a Bearer JWT's `iss` claim and to locate its `/.well-known/jwks.json` for JWKS fetch+cache (contract §7.3, I7). |
| `AUTH_AUDIENCE` | only to enable the JWT path | *(empty)* | This service's own public URL. See "Audience convention" below — do **not** treat this as an opaque service name. |

`SSO_ISSUER` and `AUTH_AUDIENCE` are a pair: `cmd/server` only builds a JWT
verifier when **both** are set (`main.go`'s `wireIdentity`). Leaving either
one unset is a supported, deliberate configuration — it disables the JWT
Bearer path and the service runs on API-key auth only, logging a warning
(`SSO_ISSUER/AUTH_AUDIENCE not set — JWT bearer path disabled, API-key auth
only`) rather than failing to start. **Read `docs/GETTING-STARTED.md`'s
"JWT seam" section before deciding to set these for a real deployment** —
as of this milestone the JWT path is wired-but-dormant by design, not a
live SSO integration hestia should turn on by default.

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
  the entire database — users, api_keys, todos). No replication or
  clustering story exists or is in scope.

## Seeding the first agent API key

There is deliberately no `POST /api/v1/keys` — key issuance is CLI-only
(`cmd/issue-key`, mirrors my-task's `agent:add`). Run it once per agent
that needs one, against the running deployment's own database:

```sh
docker compose exec app /app/issue-key <handle>
```

(or, outside compose, `DATABASE_PATH=<path> ./issue-key <handle>` with the
built binary run directly against the same file the server uses). The raw
key prints to stdout **exactly once** and is never stored anywhere
recoverable (I8) — copy it immediately. Losing it means rotating (run the
command again for the same handle) or revoking
(`DELETE /api/v1/keys/:id`), not retrieving.

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

## Reverse proxy / TLS / process supervision

Out of scope for this milestone entirely (`GOAL.md` Context: "systemd,
Caddy, DNS ... is hestia's domain"). This service listens on plain HTTP on
`PORT` and expects a reverse proxy in front of it for TLS — no built-in
TLS termination.
