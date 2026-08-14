import { defineConfig, devices } from "@playwright/test";

// TPL-3: browser e2e against a real, running my-template server —
// nothing here starts the SPA's own dev server (unlike web/'s own vite
// dev workflow); e2e/global-setup.ts builds and runs the real compiled
// binary the same way a deployment would, against a real local OIDC
// issuer. See that file's own header comment for the full bring-up
// sequence and why it owns the whole lifecycle instead of this file's
// own `webServer` option.
export default defineConfig({
  testDir: "./e2e/specs",
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: [["list"]],
  globalSetup: "./e2e/global-setup.ts",
  globalTeardown: "./e2e/global-teardown.ts",
  use: {
    baseURL: "http://127.0.0.1:24080",
    trace: "retain-on-failure",
    video: "off",
    screenshot: "only-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
