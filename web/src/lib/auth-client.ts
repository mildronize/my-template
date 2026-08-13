// TPL-1 milestone-3/task-3: replaces task-1's placeholder (always a
// signed-in stub session) with a real TanStack Query hook against
// GET /api/bff/me (internal/transport/bff/me_handler.go,
// .chief/milestone-3/_contract/API.md). Same exported shape as before
// (useSession() -> { data, isPending, error }, signOut()) — AuthGate.tsx
// and Header.tsx need zero further changes beyond this file's own
// implementation swapping underneath their existing `~/lib/auth-client`
// import, per task-3's own spec.
//
// GET /api/bff/me's response shape (task-2's report):
// { "handle": string, "role": string, "active": bool } — byte-identical
// field set to GET /api/v1/me. AuthGate/Header only ever read `session`
// (truthy/falsy) and, in Header's case, `session.user.name`/
// `session.user.email` for display — this hook maps `handle` onto both of
// those (there's no separate display name/email on this surface; `role`
// rides along as the secondary line since it's the next most useful thing
// to show in that popover).
import { useQuery } from "@tanstack/react-query";

import type { components } from "~/lib/api/bff-schema.gen";

export interface AuthUser {
  id: string;
  name: string;
  email: string;
}

export interface AuthSession {
  user: AuthUser;
}

interface UseSessionResult {
  data: AuthSession | null;
  isPending: boolean;
  error: Error | null;
}

export const meQueryKey = ["bff", "me"] as const;

function toAuthSession(me: components["schemas"]["Me"]): AuthSession {
  return {
    user: { id: me.handle, name: me.handle, email: me.role },
  };
}

/**
 * Fetches GET /api/bff/me. Returns null (not an error) for any non-200
 * response — I5: "401 never leaks why" applies to how this client treats
 * the response, not just how the server writes it, so a 401 (missing,
 * expired, or wrong-role session — the only non-200 this endpoint
 * documents) is never distinguished from any other failure reason, and its
 * body is never parsed. A genuine transport failure (the fetch call itself
 * rejecting — offline, a server restart mid-request) is left to throw, so
 * it surfaces as this query's `error` instead of being folded into "no
 * session" — that distinction is exactly what AuthGate's confirmed-
 * logged-out tri-state needs to keep working (middleware.go's/
 * AuthGate.tsx's shared "don't bounce on a transient failure" rule).
 */
async function fetchMe(): Promise<AuthSession | null> {
  const res = await fetch("/api/bff/me", { credentials: "include" });
  if (!res.ok) {
    return null;
  }
  const me = (await res.json()) as components["schemas"]["Me"];
  return toAuthSession(me);
}

export function useSession(): UseSessionResult {
  const query = useQuery({
    queryKey: meQueryKey,
    queryFn: fetchMe,
  });

  return {
    data: query.data ?? null,
    isPending: query.isPending,
    error: query.error,
  };
}

/**
 * No BFF session-clear endpoint exists yet — task-2's scope was GET
 * /api/bff/me plus todo/key CRUD, no logout/session-clear route
 * (.chief/milestone-3/_contract/API.md, _plan/_todo.md's task-2 spec).
 * Inventing one is explicitly out of this task's scope (task-3's own
 * instructions: "don't invent a new BFF endpoint yourself"), so this stays
 * a no-op until a future milestone adds that endpoint — Header.tsx's own
 * handleSignOut still navigates to "/" afterward, which (since the session
 * cookie is untouched by this call) simply leaves the owner signed in.
 */
export async function signOut(): Promise<void> {}
