import { fileURLToPath, URL } from "node:url";

import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// TPL-1 milestone-3/task-3: Vitest config, kept separate from
// vite.config.ts (which stays build/dev-server-only — Tailwind's Vite
// plugin and the dev-proxy config don't matter to component tests, which
// mock the fetch layer directly rather than talking to a real Go backend
// or needing real Tailwind CSS output). Matches my-task's own choice of
// Vitest (package.json: "vitest": "^4.1.2") per GOAL.md's Decisions table
// ("`make test` and the two-suites problem").
//
// Wired into `make test` as of milestone-3/task-4 (Makefile's `web-test`
// target, which `test` depends on) — that task verified the two-suite
// claim independently from a fresh clone before wiring it in. Still
// runnable directly with `npm test` / `npx vitest run` from web/.
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "~": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    globals: false,
  },
});
