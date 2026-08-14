import { test, expect } from "@playwright/test";
import { loginAsOwner } from "../fixtures/login-as-owner";

// The residue this ticket actually scoped: not a re-run of anything
// jsdom already tests honestly, but the one thing that argument has to
// prove rather than restate — a case where a real browser and jsdom
// genuinely disagree, on the same widget, for the same interaction.
//
// jsdom has no real layout engine: every element's getBoundingClientRect
// returns zeros, so nothing in that environment can compute whether one
// element visually covers another. @testing-library/user-event's own
// click() dispatches straight to the target DOM node it's given,
// regardless of what else exists in the document at whatever screen
// position that node would occupy in a real page. A real browser does
// the opposite by construction: a click lands on whatever's topmost at
// that pixel, and Chromium (via Playwright's own actionability checks)
// refuses to synthesize a click on an element another one is
// intercepting pointer events for, rather than silently firing it on
// the covered element anyway.
//
// That gap is invisible until something exploits it: a stuck modal
// backdrop, a z-index regression, a toast that didn't unmount — any of
// which leaves a real user genuinely unable to click a control that
// every jsdom-based test for that same control keeps reporting as
// clickable. This spec manufactures exactly that (a transparent
// full-viewport overlay placed above the assignee picker's trigger) and
// shows Playwright refuses the click. web/src/app/todos/
// TodoDetailPage.test.tsx's own companion test proves the other half:
// the identical overlay, the identical trigger, jsdom's userEvent.click()
// still succeeds. One instance of this divergence is what this ticket's
// own gate asked for — if the jsdom side had also gone red, this
// wouldn't be residue, and that would have been the real result to
// report instead.
test("assignee picker: a real click can be blocked by an overlapping element that jsdom's userEvent.click() cannot see", async ({
  page,
  baseURL,
}) => {
  await loginAsOwner(page, baseURL!);

  // A real todo, created through the real BFF API. Deliberately
  // page.request, not the top-level request fixture: page.request
  // shares this page's own session cookie jar (the one loginAsOwner just
  // established); the bare request fixture is a wholly separate,
  // unauthenticated APIRequestContext — using it here was the first,
  // wrong version of this line, caught by the 401 it actually produced.
  const createRes = await page.request.post("/api/bff/todos", {
    data: { title: "residue-spec probe — disposable", clientRequestId: `e2e-residue-${Date.now()}` },
  });
  expect(createRes.ok(), "creating the probe todo via the real BFF API").toBeTruthy();
  const todo = await createRes.json();

  await page.goto(`/todos/${todo.id}`);
  const trigger = page.getByRole("combobox", { name: "Assignee" });
  await expect(trigger).toBeVisible();

  // The manufactured regression: a transparent element covering the
  // whole viewport, painted after (so stacked above) everything else —
  // structurally identical to what a stuck backdrop or a z-index bug
  // produces, deliberately without pointer-events: none (that alone
  // would be a different, already-detectable bug — user-event does
  // check computed pointer-events on the target and its ancestors, just
  // not real screen-space overlap from an unrelated element).
  await page.evaluate(() => {
    const overlay = document.createElement("div");
    overlay.id = "e2e-residue-overlay";
    overlay.style.cssText = "position:fixed;inset:0;z-index:99999;background:transparent;";
    document.body.appendChild(overlay);
  });

  await expect(trigger.click({ timeout: 3_000 })).rejects.toThrow(/intercepts pointer events/);
});
