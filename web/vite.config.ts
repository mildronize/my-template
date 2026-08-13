import { fileURLToPath, URL } from "node:url";

import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";

// TPL-1 milestone-3/task-1: Vite scaffold for the SPA that replaces
// internal/transport/bff's html/template view. Two things this config is
// responsible for, both documented in docs/GETTING-STARTED.md's
// "Running web/ against a live Go backend (Vite dev mode)" section:
//
//  1. `~` resolves to `src/`, matching my-task's own tsconfig path alias
//     (`~/lib/utils`, `~/components/ui/*`, ...) — every ported bucket-1/2
//     file's imports carry over unchanged, no per-file import rewriting.
//  2. `server.proxy` forwards `/api`, `/login`, and `/callback` to the Go
//     backend (default :8080, overridable via VITE_BFF_PROXY_TARGET) so
//     `npm run dev`'s live-reloading server can exercise the real BFF
//     without CORS or a second origin to configure client-side.
//
// The "/login" and "/callback" entries below are LOAD-BEARING, not
// incidental: they are the entire reason the SPA has no "/login" route or
// login-page component of its own. Go's cmd/server registers real GET
// /login and GET /callback handlers (the actual PKCE/OAuth redirect to
// Hydra) that a browser must reach directly — a react-router route can
// only ever show a static placeholder, never perform that redirect. In
// dev mode, this proxy list is what forwards a browser hitting /login or
// /callback to those real Go handlers instead of Vite's dev server; in
// production, cmd/server's own routing (an explicit handler beating the
// SPA's NoRoute fallback) does the equivalent job. If either entry is
// ever removed from this list, dev mode stops forwarding that path to Go
// — the SPA's catch-all route would then try to serve it instead, and
// since there is no real login/callback UI behind that route, the result
// is a silently broken dev-mode navigation with nothing real behind it.
// Do not remove "/login" or "/callback" from this proxy list without
// first adding a real SPA-side implementation of whatever they were
// covering.
export default defineConfig(({ mode }) => {
  const proxyTarget = process.env.VITE_BFF_PROXY_TARGET ?? "http://localhost:8080";

  return {
    plugins: [react(), tailwindcss()],
    resolve: {
      alias: {
        "~": fileURLToPath(new URL("./src", import.meta.url)),
      },
    },
    build: {
      // Vite's own default ("dist") — stated explicitly since
      // cmd/server's //go:embed directive and the Makefile's build
      // ordering both depend on this exact path.
      outDir: "dist",
    },
    server:
      mode === "development"
        ? {
            proxy: {
              "/api": proxyTarget,
              "/login": proxyTarget,
              "/callback": proxyTarget,
            },
          }
        : undefined,
  };
});
