// TPL-1 milestone-3/task-3: NOT a port of my-task's own login/page.tsx
// (origin/main @ fdceb8befbcebfa25d2216d71264bb7c5e8c96d7) — that file
// drives client-side better-auth OAuth (authClient.signIn.oauth2), which
// this template doesn't need: internal/transport/bff's GET /login already
// does the full server-side PKCE redirect to Hydra
// (.chief/milestone-3/_goal/GOAL.md's inventory table, "Auth seam"
// decision). This page keeps only the visual card/button layout my-task's
// version used, replacing its onClick handler with a plain anchor — a
// real browser navigation to GET /login, not a fetch, so the Go handler's
// redirect actually happens (a fetch call can't follow a redirect the way
// a browser navigation does, the same reason _contract/API.md gives for
// /api/bff/* answering 401 instead of redirecting).
//
// Topology note: a real browser navigating to "/login" (including
// AuthGate.tsx's own window.location.replace("/login...") redirect) is
// answered directly by cmd/server's bff gin engine, which registers an
// explicit GET /login route (cmd/server/main.go's wireBFF) — gin always
// prefers an explicit route over its SPA-serving NoRoute fallback, so that
// request never reaches this component. This page is therefore only ever
// rendered via a client-side (react-router) navigation to /login, not a
// full page load — kept as its own route regardless, matching GOAL.md's
// Screen scope decision ("Login ... + Todos ... + Settings"), which lists
// it as one of this SPA's three screens.
import { Button } from "~/components/ui/button";

export default function LoginPage() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-[var(--background)] px-4">
      <div className="w-full max-w-sm rounded-2xl border border-[var(--line)] bg-[var(--chip-bg)] p-8 shadow-sm">
        <h1 className="mb-6 text-2xl font-bold text-[var(--sea-ink)]">Sign in</h1>
        <Button asChild className="w-full">
          <a href="/login">Sign in with SSO</a>
        </Button>
      </div>
    </div>
  );
}
