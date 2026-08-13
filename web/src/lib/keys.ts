// TPL-1 milestone-3/task-3: TanStack Query hooks for the BFF's key
// list/revoke surface (GET /api/bff/keys, DELETE /api/bff/keys/:id —
// _contract/API.md), replacing tRPC's own api.apiKey.* hooks (my-task's
// src/app/(app)/settings/api-key-settings.tsx). Deliberately no
// useCreateKeyMutation/useRotateKeyMutation — this surface has no
// POST /api/bff/keys or rotate endpoint by design (GOAL.md Done-when 5,
// _contract/API.md's own "Deliberately absent" note): a raw key can't be
// returned safely over an HTTP response, so issuance/rotation stay
// CLI-only regardless of which surface is asking.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { bffFetch } from "~/lib/api/client";
import type { components } from "~/lib/api/bff-schema.gen";

export type ApiKey = components["schemas"]["ApiKey"];

export const keysQueryKey = ["bff", "keys"] as const;

/** GET /api/bff/keys — the session owner's own non-revoked keys. */
export function useKeysQuery() {
  return useQuery({
    queryKey: keysQueryKey,
    queryFn: () =>
      bffFetch<components["schemas"]["ApiKeyList"]>("/keys").then((r) => r.keys),
  });
}

/** DELETE /api/bff/keys/:id — owner-scoped, 404 (never 403) on mismatch. */
export function useRevokeKeyMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => bffFetch<void>(`/keys/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: keysQueryKey });
    },
  });
}
