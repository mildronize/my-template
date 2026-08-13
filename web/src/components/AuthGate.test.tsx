// TPL-1 milestone-3/task-3, Done-when 6: proves AuthGate's no-flash
// behavior survives the swap from task-1's placeholder useSession() to the
// real one (auth-client.ts, backed by TanStack Query + GET /api/bff/me).
// AuthGate.tsx's own logic is unchanged (bucket 2, ported verbatim) — what
// changed is the hook underneath it, so this test exercises the real
// hook with a mocked fetch, not a mocked hook: a first call succeeds (a
// confirmed session), a second call — a transient failure, e.g. a server
// restart or flaky network, simulated by fetch() itself rejecting rather
// than resolving with a non-200 — must NOT bounce a previously
// -authenticated user to /login. See AuthGate.tsx's own doc comment for
// why this distinction (network failure vs. a confirmed 401) matters:
// only a *confirmed* logged-out state (not pending, no error, session ===
// null) redirects.
//
// Fix-round addition (milestone-3/task-3, post-verification): the test
// above only exercises confirmedLoggedOut's `session === null` clause —
// useSession()'s `data: query.data ?? null` means TanStack Query retains
// the last successful `data` across a failed background refetch, so once
// a check has ever succeeded, `session` never goes back to null just
// because a *later* check errors. That leaves confirmedLoggedOut's
// `!error` clause provably unexercised by that test alone (verified by
// temporarily deleting `!error` from AuthGate.tsx and confirming the test
// above still passed). `!error` is load-bearing exactly once: a
// *first-ever* session check that fails transiently, before any check has
// ever succeeded — there `session` genuinely is null (nothing cached yet)
// and `error` is truthy, so without `!error` confirmedLoggedOut would
// wrongly become true and bounce a user who may have a perfectly valid
// session to /login on nothing but a first-request network blip. The two
// tests below cover that: a first-ever transient failure must NOT
// redirect, while a first-ever *genuine* no-session response (a real,
// non-200 Response — not a rejected fetch) still must.
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement } from "react";

import { AuthGate } from "./AuthGate";
import { meQueryKey } from "~/lib/auth-client";

function renderWithClient(ui: ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const utils = render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>);
  return { ...utils, queryClient };
}

describe("AuthGate", () => {
  const originalFetch = global.fetch;
  const originalLocation = window.location;
  let replaceMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    // AuthGate redirects via window.location.replace(...). jsdom's own
    // Location.replace is non-configurable (vi.spyOn can't touch it
    // directly — "Cannot redefine property: replace"), so the whole
    // `window.location` property is swapped for a plain object carrying a
    // mock in its place, rather than letting jsdom attempt (and fail on) a
    // real navigation.
    replaceMock = vi.fn();
    Object.defineProperty(window, "location", {
      configurable: true,
      value: { ...originalLocation, replace: replaceMock },
    });
  });

  afterEach(() => {
    // This suite now renders more than once per describe block (three
    // `it`s, each calling renderWithClient). vitest.config.ts sets
    // `globals: false`, so @testing-library/react's own automatic
    // afterEach-cleanup (which relies on a global `afterEach`) never
    // registers — without an explicit cleanup() call here, a prior test's
    // rendered DOM (e.g. "protected content") stays attached to
    // document.body and leaks into the next test's queries.
    cleanup();
    global.fetch = originalFetch;
    Object.defineProperty(window, "location", {
      configurable: true,
      value: originalLocation,
    });
    vi.restoreAllMocks();
  });

  it("does not redirect to /login when a confirmed session later fails transiently", async () => {
    let callCount = 0;
    const fetchMock = vi.fn(async () => {
      callCount += 1;
      if (callCount === 1) {
        return new Response(
          JSON.stringify({ handle: "owner", role: "owner", active: true }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      // A genuine transport failure, not a 401 — fetch() itself rejects.
      throw new Error("simulated transient network failure");
    });
    global.fetch = fetchMock as unknown as typeof fetch;

    const { queryClient } = renderWithClient(
      <AuthGate>
        <div>protected content</div>
      </AuthGate>,
    );

    // First (successful) check: children render, never the login redirect.
    expect(await screen.findByText("protected content")).toBeInTheDocument();
    expect(replaceMock).not.toHaveBeenCalled();

    // Force a refetch that fails transiently, as if the server had a
    // momentary hiccup on the *next* session check.
    await queryClient.invalidateQueries({ queryKey: meQueryKey });
    await waitFor(() => expect(callCount).toBeGreaterThanOrEqual(2));

    // Still showing the protected content — the failing refetch must not
    // have bounced a previously-authenticated user to /login.
    expect(screen.getByText("protected content")).toBeInTheDocument();
    expect(replaceMock).not.toHaveBeenCalled();
  });

  it("does not redirect to /login when the very first session check fails transiently", async () => {
    // No prior success anywhere in this test — fetch() itself rejects on
    // every call, exactly like a network blip or a server that hasn't
    // finished starting on the user's *first* visit. Unlike the test
    // above, `session` is genuinely null here (nothing has ever been
    // cached), so this is the one case where confirmedLoggedOut's `!error`
    // clause is the only thing standing between this user and a wrongful
    // redirect.
    const fetchMock = vi.fn(async () => {
      throw new Error("simulated transient network failure");
    });
    global.fetch = fetchMock as unknown as typeof fetch;

    renderWithClient(
      <AuthGate>
        <div>protected content</div>
      </AuthGate>,
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    // Let the query settle into its error state (isPending: false, error:
    // set) and give AuthGate's effect a chance to run.
    await waitFor(() => expect(screen.getByRole("status")).toBeInTheDocument());

    expect(screen.queryByText("protected content")).not.toBeInTheDocument();
    expect(replaceMock).not.toHaveBeenCalled();
  });

  it("redirects to /login when the very first session check resolves with a genuine no-session", async () => {
    // Contrast case: a real Response with a non-200 status (not a
    // rejected fetch) — fetchMe (auth-client.ts) treats any non-200 as
    // "no session" and resolves null rather than throwing, so this is a
    // *confirmed* logged-out state, not a transient failure. Proves the
    // guard actually distinguishes the two rather than simply never
    // redirecting.
    const fetchMock = vi.fn(async () => new Response(null, { status: 401 }));
    global.fetch = fetchMock as unknown as typeof fetch;

    renderWithClient(
      <AuthGate>
        <div>protected content</div>
      </AuthGate>,
    );

    await waitFor(() => expect(replaceMock).toHaveBeenCalled());
    expect(replaceMock.mock.calls[0][0]).toContain("/login");
    expect(screen.queryByText("protected content")).not.toBeInTheDocument();
  });
});
