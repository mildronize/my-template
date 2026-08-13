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
