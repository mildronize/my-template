import { defineConfig, devices } from "@playwright/test";

// TPL-3: browser e2e against a real, running my-template server —
// nothing here starts the SPA's own dev server (unlike web/'s own vite
// dev workflow); e2e/global-setup.ts builds and runs the real compiled
// binary the same way a deployment would, against a real local OIDC
// issuer. See that file's own header comment for the full bring-up
// sequence and why it owns the whole lifecycle instead of this file's
// own `webServer` option.
export default defineConfig({
  testDir: "./specs",
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  // "list" for terminal output; "html" (open: "never" — a spot-check
  // reads the report file, this shouldn't also try to launch a browser
  // tab on whatever machine runs it) so every run's attachments —
  // login.spec.ts's own logged-in-owner screenshot included — are
  // actually saved somewhere a report can point at, not only kept for
  // failures the way trace/screenshot below already are.
  reporter: [["list"], ["html", { open: "never" }]],
  globalSetup: "./global-setup.ts",
  globalTeardown: "./global-teardown.ts",
  use: {
    baseURL: "http://127.0.0.1:24080",
    trace: "retain-on-failure",
    video: "off",
    screenshot: "only-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
