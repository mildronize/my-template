// TPL-3: brings up everything login.spec.ts (and the other specs) need,
// once, before any test runs — real Hydra, the login-consent stub, a
// registered client, a freshly-built app binary, a freshly-seeded
// throwaway database, and the app itself running and provably
// SSO-configured. e2e/global-teardown.ts is this file's exact mirror:
// everything started here gets stopped there, nothing left running or
// left on disk between runs (gate 4).
//
// Owns the WHOLE lifecycle itself rather than splitting it across this
// file and playwright.config.ts's own `webServer` option: bringing up
// Hydra, running scripts/register.sh, seeding the database, and only
// THEN starting the app (which needs the registration's own output as
// its own config) is a real dependency chain a single `webServer` entry
// can't express, and Playwright's `webServer` array runs its entries
// concurrently, not in this specific sequence — globalSetup, as one
// script with real control flow, is the more honest fit for "these five
// things happen in this order, and the last one needs what the middle
// ones produced," not a workaround for a missing feature.
//
// Every path/port below is fixed and deliberately far from anything real
// this host runs: sso-hydra (this fleet's real Hydra) is on 4444/4445;
// this uses 24444/24445/24446/24080. A real developer's own `make dev`
// / `docker compose up` writes to ./data/app.db by default (Makefile,
// docker-compose.yml); this writes to e2e/.tmp/e2e-only-throwaway.db — a
// path nothing else in this repo ever reads or writes, named so a
// collision with a real database is not just unlikely but would have to
// be deliberate.
import { execFile, spawn, type ChildProcess } from "node:child_process";
import { promisify } from "node:util";
import { existsSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { setTimeout as sleep } from "node:timers/promises";
import path from "node:path";

const execFileAsync = promisify(execFile);

const REPO_ROOT = path.resolve(__dirname, "..");
const E2E_TMP_DIR = path.join(REPO_ROOT, "e2e", ".tmp");
const DB_PATH = path.join(E2E_TMP_DIR, "e2e-only-throwaway.db");
const STATE_PATH = path.join(E2E_TMP_DIR, "state.json");
const SERVER_BIN_PATH = path.join(E2E_TMP_DIR, "server");

const COMPOSE_PROJECT = "my-template-e2e";
const COMPOSE_FILE = path.join(REPO_ROOT, "e2e", "docker-compose.yml");

const HYDRA_PUBLIC_URL = "http://127.0.0.1:24444";
const HYDRA_ADMIN_URL = "http://127.0.0.1:24445";
const APP_PORT = "24080";
const APP_BASE_URL = `http://127.0.0.1:${APP_PORT}`;

// Must match e2e/fixtures/oidc-login-stub.mjs's own E2E_TEST_SUBJECT
// default exactly — this is the one identity the whole stack ever
// authenticates as. Duplicated rather than shared from a common module
// on purpose: the stub runs inside a separate Node process (a Docker
// container), so "shared" would mean a build step or an env var passed
// through docker-compose.yml, more moving parts than restating one
// string in two places that both say why.
const TEST_SSO_SUBJECT = "e2e-test-owner-sub";
const TEST_SESSION_SECRET = "e2e-only-session-secret-not-a-real-one-0123456789";

async function waitFor(label: string, checkFn: () => Promise<boolean>, timeoutMs: number, intervalMs = 500) {
  const deadline = Date.now() + timeoutMs;
  let lastErr: unknown;
  while (Date.now() < deadline) {
    try {
      if (await checkFn()) return;
    } catch (err) {
      lastErr = err;
    }
    await sleep(intervalMs);
  }
  throw new Error(`timed out waiting for ${label} after ${timeoutMs}ms` + (lastErr ? ` (last error: ${lastErr})` : ""));
}

export default async function globalSetup() {
  console.log("[global-setup] TPL-3 e2e bring-up starting");

  // --- Fail closed on an unexpected existing database, per Clara's own
  // instruction: this file must refuse rather than silently open
  // whatever is already at DB_PATH. The only thing that should ever
  // create this file is this function, a few lines below — anything
  // already there is either a previous run's leftover (global-teardown
  // failed to clean up) or evidence the path computation above is wrong,
  // and neither case is safe to just open and reuse.
  if (existsSync(DB_PATH)) {
    throw new Error(
      `[global-setup] refusing to start: ${DB_PATH} already exists. ` +
        `This path is exclusively this suite's own throwaway database — ` +
        `nothing else in this repo ever writes here. Remove it by hand ` +
        `and re-run if you're sure it's leftover from an interrupted run ` +
        `(e.g. \`rm -f ${DB_PATH}\`); do not skip this check.`,
    );
  }
  rmSync(E2E_TMP_DIR, { recursive: true, force: true });
  mkdirSync(E2E_TMP_DIR, { recursive: true });

  // --- 1. Hydra + the login-consent stub -----------------------------
  console.log("[global-setup] docker compose up (Hydra + login-consent stub)");
  await execFileAsync("docker", [
    "compose",
    "-f",
    COMPOSE_FILE,
    "-p",
    COMPOSE_PROJECT,
    "up",
    "-d",
    "--wait",
  ]);

  // Belt-and-braces on top of compose's own --wait/healthcheck: hit
  // Hydra's readiness endpoint from the host, the same way a real
  // consumer would, rather than trusting Docker's own view of "healthy."
  await waitFor(
    "Hydra readiness",
    async () => {
      const res = await fetch(`${HYDRA_ADMIN_URL}/health/ready`);
      return res.ok;
    },
    30_000,
  );
  console.log("[global-setup] Hydra is up and ready");

  // --- 2. Register a real client via the real registration script ----
  console.log("[global-setup] registering the e2e OAuth2 client via scripts/register.sh");
  const { stdout: registerOut } = await execFileAsync(
    "bash",
    [path.join(REPO_ROOT, "scripts", "register.sh")],
    {
      env: {
        ...process.env,
        ENV: "dev",
        SERVICE_NAME: "my-template-e2e",
        SERVICE_PUBLIC_URL: APP_BASE_URL,
        SSO_ISSUER: HYDRA_PUBLIC_URL,
        HYDRA_ADMIN_URL,
        HYDRA_PUBLIC_URL,
      },
    },
  );
  const clientId = /^SSO_CLIENT_ID=(.+)$/m.exec(registerOut)?.[1];
  const clientSecret = /^SSO_CLIENT_SECRET=(.+)$/m.exec(registerOut)?.[1];
  if (!clientId || !clientSecret) {
    throw new Error(
      "[global-setup] could not parse SSO_CLIENT_ID/SSO_CLIENT_SECRET out of scripts/register.sh's own output — " +
        "the script's own read-back verification would have failed first if registration itself failed, so " +
        "this means its output shape changed, not that the client wasn't created.",
    );
  }
  console.log(`[global-setup] registered client ${clientId}`);

  // --- 3. Build the real app (SPA first, then the Go binary) ---------
  // Same ordering Makefile's own `build` target enforces, for the same
  // reason (cmd/server embeds web/dist at compile time — building the
  // binary first would bake in whatever was on disk before, stale or
  // absent).
  console.log("[global-setup] building the SPA (npm ci && npm run build)");
  await execFileAsync("npm", ["ci"], { cwd: path.join(REPO_ROOT, "web") });
  await execFileAsync("npm", ["run", "build"], { cwd: path.join(REPO_ROOT, "web") });

  console.log("[global-setup] building cmd/server and cmd/seed");
  await execFileAsync("go", ["build", "-o", SERVER_BIN_PATH, "./cmd/server"], { cwd: REPO_ROOT });
  const seedBinPath = path.join(E2E_TMP_DIR, "seed");
  await execFileAsync("go", ["build", "-o", seedBinPath, "./cmd/seed"], { cwd: REPO_ROOT });

  // --- 4. Seed the owner row, identified by the stub's own fixed sub -
  console.log(`[global-setup] seeding the owner row (sub=${TEST_SSO_SUBJECT})`);
  await execFileAsync(seedBinPath, [], {
    env: { ...process.env, DATABASE_PATH: DB_PATH, SEED_OWNER_SSO_SUBJECT: TEST_SSO_SUBJECT },
  });

  // --- 5. Start the app itself ----------------------------------------
  console.log("[global-setup] starting the app server");
  const serverEnv = {
    ...process.env,
    PORT: APP_PORT,
    DATABASE_PATH: DB_PATH,
    SSO_ISSUER: HYDRA_PUBLIC_URL,
    SSO_CLIENT_ID: clientId,
    SSO_CLIENT_SECRET: clientSecret,
    AUTH_AUDIENCE: APP_BASE_URL,
    SESSION_SECRET: TEST_SESSION_SECRET,
  };
  const server: ChildProcess = spawn(SERVER_BIN_PATH, [], {
    cwd: REPO_ROOT,
    env: serverEnv,
    stdio: ["ignore", "pipe", "pipe"],
    detached: false,
  });
  if (!server.pid) {
    throw new Error("[global-setup] app server failed to spawn (no pid)");
  }
  const serverPid = server.pid;
  server.stdout?.on("data", (d) => process.stdout.write(`[app] ${d}`));
  server.stderr?.on("data", (d) => process.stderr.write(`[app] ${d}`));
  server.on("exit", (code) => {
    if (code !== null && code !== 0) {
      console.error(`[global-setup] app server exited early with code ${code}`);
    }
  });

  // Also written now, before the readiness wait below, so a failed wait
  // still leaves global-teardown able to find and kill this pid rather
  // than leaking a process this run itself started.
  writeFileSync(
    STATE_PATH,
    JSON.stringify({ composeProject: COMPOSE_PROJECT, composeFile: COMPOSE_FILE, serverPid, dbPath: DB_PATH }, null, 2),
  );

  // --- 6. The gate that actually matters: prove SSO landed, not just
  // that the process is up. GET /login unauthenticated: configured()
  // false -> renderLoginError -> 401 with an HTML error body
  // (internal/transport/bff/middleware.go); configured() true -> a real
  // 302 to this exact issuer's own /oauth2/auth
  // (internal/transport/bff/login_handler.go). /healthz would return 200
  // in BOTH cases — this is deliberately not that check.
  console.log("[global-setup] waiting for /login to prove SSO is actually wired (not just that the process answers)");
  await waitFor(
    "app SSO configuration (GET /login -> 302 to the local issuer)",
    async () => {
      const res = await fetch(`${APP_BASE_URL}/login`, { redirect: "manual" });
      if (res.status !== 302) return false;
      const location = res.headers.get("location") ?? "";
      return location.startsWith(`${HYDRA_PUBLIC_URL}/oauth2/auth`);
    },
    30_000,
  );
  console.log("[global-setup] confirmed: GET /login redirects to the local issuer's own /oauth2/auth. Ready.");
}
