// TPL-3: the exact mirror of global-setup.ts — everything started there
// gets stopped here. Reads e2e/.tmp/state.json rather than sharing
// in-memory state with global-setup.ts, because Playwright does not
// guarantee globalSetup and globalTeardown run in the same process.
//
// Kills the app server by the exact pid global-setup.ts recorded — never
// pkill -f, matching this project's own standing rule elsewhere
// (cmd/smoke, the live port-8080 swap): a broad kill-by-pattern has
// taken down the wrong process on this host before. Confirms the pid is
// still this same binary before signaling it, the same "verify before
// you touch it" discipline.
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { existsSync, readFileSync, rmSync } from "node:fs";
import path from "node:path";

const execFileAsync = promisify(execFile);
const REPO_ROOT = path.resolve(__dirname, "..");
const E2E_TMP_DIR = path.join(REPO_ROOT, "e2e", ".tmp");
const STATE_PATH = path.join(E2E_TMP_DIR, "state.json");

export default async function globalTeardown() {
  console.log("[global-teardown] TPL-3 e2e tear-down starting");

  if (!existsSync(STATE_PATH)) {
    console.log("[global-teardown] no state.json — nothing global-setup.ts recorded, nothing to tear down");
    return;
  }
  const state = JSON.parse(readFileSync(STATE_PATH, "utf8")) as {
    composeProject: string;
    composeFile: string;
    serverPid: number;
    dbPath: string;
  };

  // --- 1. Kill the app server by exact pid, verified first -----------
  try {
    const { stdout } = await execFileAsync("ps", ["-p", String(state.serverPid), "-o", "cmd="]);
    if (stdout.includes("server")) {
      process.kill(state.serverPid, "SIGTERM");
      console.log(`[global-teardown] sent SIGTERM to pid ${state.serverPid} (confirmed it was our own server binary)`);
    } else {
      console.warn(
        `[global-teardown] pid ${state.serverPid} exists but its cmd (${stdout.trim()}) doesn't look like our ` +
          `server — NOT killing it, in case it's since been reused by an unrelated process.`,
      );
    }
  } catch {
    console.log(`[global-teardown] pid ${state.serverPid} is already gone`);
  }

  // --- 2. Tear down Hydra + the login-consent stub, volumes included -
  console.log("[global-teardown] docker compose down -v");
  try {
    await execFileAsync("docker", [
      "compose",
      "-f",
      state.composeFile,
      "-p",
      state.composeProject,
      "down",
      "-v",
    ]);
  } catch (err) {
    console.error("[global-teardown] docker compose down -v failed:", err);
  }

  // --- 3. Remove the throwaway database and every other run-scoped file
  rmSync(E2E_TMP_DIR, { recursive: true, force: true });
  console.log(`[global-teardown] removed ${E2E_TMP_DIR} (including the throwaway database)`);
}
