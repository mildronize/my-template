import { test, expect, type TestInfo } from "@playwright/test";
import { loginAsOwner } from "../fixtures/login-as-owner";

// TPL-3's first spec — the path nothing in this project has ever
// exercised before: a real browser navigating to /login, following the
// real redirect through a real (local) OIDC issuer's authorization
// endpoint, a real login/consent round trip, and back through
// /callback — not a directly-signed session cookie, not an in-process
// actor injection, both of which are what every other test in this repo
// uses instead (see this repo's own internal/transport/bff test suite).
//
// Which issuer this ran against, stated explicitly per this project's own
// standing rule (TPL-3 comment 6): a green run here must never be read as
// "the production SSO login works." It proves the OIDC contract —
// redirect, PKCE, callback, session issuance — the same contract
// production Hydra also implements, against a local instance seeded for
// this run alone.
test("owner logs in through the real local OIDC issuer and lands on an authenticated page", async ({ page, baseURL }, testInfo: TestInfo) => {
  console.log(`[login.spec] issuer under test: http://127.0.0.1:24444 (local, e2e-only — not production Hydra)`);

  // The full redirect chain — /login -> Hydra's /oauth2/auth -> the
  // login-consent stub's /login -> Hydra -> the stub's /consent -> Hydra
  // -> back to this app's own /callback -> a final redirect to "/" — all
  // happens via real HTTP 302s the browser itself follows. No step here
  // is mocked or skipped.
  await loginAsOwner(page, baseURL!);

  // The real proof this worked: a session cookie now exists (set by
  // internal/transport/bff/callback_handler.go's own signer, not by
  // anything this test wrote), and the page renders real, session-backed
  // content — not a login form, not an error page.
  const cookies = await page.context().cookies();
  const sessionCookie = cookies.find((c) => c.name === "session");
  expect(sessionCookie, "a real session cookie must be set after a real login").toBeTruthy();

  await expect(page.getByRole("heading", { name: "Todos", exact: true })).toBeVisible();

  // Header.tsx's user-menu popover renders session.user.name, which maps
  // to the resolved handle (auth-client.ts's toAuthSession) — "owner" is
  // the one fixed handle cmd/seed ever creates (cmd/seed/main.go's own
  // ownerHandle constant). Only the initial ("O") shows on the collapsed
  // trigger, so this opens it — seeing the full handle inside is proof
  // the session resolved to a real, specific identity, not merely "some
  // cookie exists."
  await page.getByRole("button", { name: "User menu" }).click();
  await expect(page.getByText("owner", { exact: true }).first()).toBeVisible();

  // Attached on every run, pass or fail — not just Playwright's own
  // failure-only default (playwright.config.ts's screenshot: "only-on-
  // failure" covers unexpected crashes; this is deliberate evidence for
  // a spot-check reading a report rather than running the suite
  // themselves, so a passing run has to leave something to look at too).
  await testInfo.attach("logged-in-owner-view", { body: await page.screenshot(), contentType: "image/png" });
});
