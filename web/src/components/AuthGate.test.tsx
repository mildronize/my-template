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
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
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
});
