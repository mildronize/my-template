// TPL-1 milestone-3/task-3: one shared QueryClient instance for the whole
// SPA — TanStack Query is now the data layer behind every BFF-backed hook
// (auth-client.ts's useSession, lib/todos.ts, lib/keys.ts), replacing
// tRPC's own React Query integration (my-task's src/trpc/react.tsx,
// TRPCReactProvider) now that tRPC itself is out of scope entirely
// (GOAL.md's Decisions table: "Not carried: tRPC ... The SPA talks to the
// BFF over JSON"). Wired into the tree once, in main.tsx.
import { QueryClient } from "@tanstack/react-query";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // A network-level failure (not a non-200 response — see
      // auth-client.ts's own comment on that distinction) shouldn't
      // silently retry several times with backoff before AuthGate's
      // no-flash-on-transient-error logic gets a chance to see the error
      // state at all.
      retry: false,
    },
  },
});
