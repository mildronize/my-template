import type { Page } from "@playwright/test";

// Shared by every spec that needs an authenticated session, not just
// login.spec.ts — the residue specs (real-browser widget behavior) need
// to be logged in to reach the pages they test, and re-running the real
// redirect chain once per spec file (rather than reaching for
// Playwright's storageState reuse) keeps every spec independently
// provable: any one of them can be read on its own, still going through
// the real /login -> issuer -> /callback path login.spec.ts itself
// proves, not a shortcut that only that one file exercises.
export async function loginAsOwner(page: Page, baseURL: string) {
  await page.goto("/login");
  await page.waitForURL((url) => url.origin === new URL(baseURL).origin && url.pathname === "/", {
    timeout: 15_000,
  });
}
