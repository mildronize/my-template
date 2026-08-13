// TPL-1 milestone-3/task-1 stub.
//
// my-task's real src/lib/auth-client.ts (not ported — better-auth is a
// bucket-3 dependency this template drops entirely, per
// .chief/milestone-3/_goal/GOAL.md's Decisions table: "Session-check
// hook ... only its data source changes — from useSession() (better-auth)
// to a TanStack Query hook against a new BFF session-check endpoint")
// wraps createAuthClient()'s useSession/signOut. AuthGate.tsx and
// Header.tsx both import `useSession`/`signOut` from this module path —
// bucket 2's instruction is to port their *structure* verbatim (the
// tri-state logic in AuthGate, the routing swap in Header) without
// wiring either to a real backend yet, since the BFF's GET /api/bff/me
// endpoint this hook should eventually call doesn't exist until task-2.
//
// TODO(task-3): replace this whole file with a TanStack Query hook
// against GET /api/bff/me, matching this same { data, isPending, error }
// shape so AuthGate.tsx and Header.tsx need no further changes beyond
// swapping the import. Until then this always reports a signed-in
// placeholder owner — deliberately not wired to any endpoint (real or
// fake) that doesn't exist yet, per task-1's instructions — so routing
// and the app shell are provable end-to-end without task-2/3's pieces.
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

const placeholderSession: AuthSession = {
  user: {
    id: "placeholder-owner",
    name: "Owner",
    email: "owner@example.invalid",
  },
};

export function useSession(): UseSessionResult {
  return { data: placeholderSession, isPending: false, error: null };
}

export async function signOut(): Promise<void> {
  // No-op until task-3 wires this to a real BFF logout/session-clear call.
}
