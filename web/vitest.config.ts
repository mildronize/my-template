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
// Deliberately NOT wired into `make test` — that's task-4's job
// specifically (.chief/milestone-3/_plan/_todo.md), so it can verify the
// two-suite claim independently. Run this suite directly with
// `npm run test` / `npx vitest run`.
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
