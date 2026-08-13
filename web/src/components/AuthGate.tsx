// Ported from my-task (origin/main @ fdceb8befbcebfa25d2216d71264bb7c5e8c96d7,
// src/components/AuthGate.tsx) per this milestone's bucket-2 rule: the
// confirmed-logged-out tri-state, the no-flash-on-transient-error
// behavior, and the redirect-without-flashing logic below are unchanged,
// verbatim React — only `useSession`'s data source changes, and that
// change is task-3's (once task-2's GET /api/bff/me exists). For now
// `~/lib/auth-client` is a local stub (see that file's own comment) that
// always reports a signed-in placeholder session, not the real
// better-auth client my-task's copy imports the same name from.
import { useSession } from "~/lib/auth-client";
import { useEffect, useRef, type ReactNode } from "react";
import { Spinner } from "~/components/ui/spinner";

interface AuthGateProps {
  children: ReactNode;
}

export function AuthGate({ children }: AuthGateProps) {
  const { data: session, isPending, error } = useSession();
  const redirectingRef = useRef(false);
  const hadSessionRef = useRef(false);

  if (session) hadSessionRef.current = true;

  // Only treat the user as logged out when the check is *confirmed*
  // unauthenticated: not loading, no transport error, and the server returned
  // no session. A failed/errored check (server restart, flaky network, a
  // mobile tab restored from memory) must NOT bounce the user to /login —
  // that was the source of spurious logouts.
  const confirmedLoggedOut = !isPending && !error && session === null;

  useEffect(() => {
    if (confirmedLoggedOut && !redirectingRef.current) {
      redirectingRef.current = true;
      // Use window.location so we avoid adding useSearchParams (which requires
      // a Suspense boundary in Next 16). The effect only runs client-side.
      const currentPath =
        window.location.pathname +
        (window.location.search ? window.location.search : "");
      const isOnLogin = window.location.pathname === "/login";
      const loginUrl = isOnLogin
        ? "/login"
        : `/login?redirect=${encodeURIComponent(currentPath)}`;
      window.location.replace(loginUrl);
    }
  }, [confirmedLoggedOut]);

  // Render the app when we have a session, or when we previously had one and
  // the current check is merely failing/refetching — don't flash a logout.
  if (session || (hadSessionRef.current && !confirmedLoggedOut)) {
    return <>{children}</>;
  }

  // Initial load, a failing first check, or while redirecting: show a spinner
  // rather than kicking to /login on a transient error.
  return (
    <div className="flex min-h-[calc(100vh-80px)] items-center justify-center">
      <Spinner size="lg" />
    </div>
  );
}
