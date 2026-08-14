// milestone-4 handle-exposure fix-round: renders ApiKeySettings against
// **mocked** /api/bff/keys data — global.fetch is stubbed, no real
// backend involved — and asserts the actual rendered output carries the
// owning agent's handle, both on the row itself and in the revoke
// confirmation dialog's title ("Revoke {handle}'s key?", my-task's own
// exact copy shape — see RevokeKeyButton's own doc comment). Same
// discipline TodosList.test.tsx already applies to TodoRow's own handle
// display: a real render, not just a type-level check that ApiKey.handle
// exists on the wire type.
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement } from "react";
import userEvent from "@testing-library/user-event";

import { ApiKeySettings } from "./ApiKeySettings";

function renderWithClient(ui: ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>);
}

describe("ApiKeySettings", () => {
  const originalFetch = global.fetch;

  afterEach(() => {
    cleanup();
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("shows the owning agent's handle on the row, and names it in the revoke confirmation", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === "string" ? input : input.toString();
      if (url.endsWith("/api/bff/keys")) {
        return new Response(
          JSON.stringify({
            keys: [
              {
                id: "key-1",
                prefix: "tpl_abc123",
                handle: "clara",
                createdAt: "2026-01-01T00:00:00Z",
                expiresAt: "2027-01-01T00:00:00Z",
              },
            ],
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      throw new Error(`ApiKeySettings.test.tsx: unexpected fetch to ${url}`);
    });
    global.fetch = fetchMock as unknown as typeof fetch;

    renderWithClient(<ApiKeySettings />);

    // The row's own primary text is the handle, not the prefix — mirrors
    // my-task's ApiKeyRow.
    expect(await screen.findByText("clara")).toBeInTheDocument();

    // Opening the revoke dialog shows my-task's own exact copy shape,
    // naming the agent — not a generic "Revoke this key?".
    await userEvent.click(screen.getByRole("button", { name: /revoke/i }));
    expect(await screen.findByText("Revoke clara's key?")).toBeInTheDocument();
  });
});
