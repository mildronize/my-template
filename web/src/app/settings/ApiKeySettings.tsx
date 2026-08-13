// TPL-1 milestone-3/task-3: replaces ApiKeySettingsPlaceholder.tsx (task-1)
// with the real thing, mirroring my-task's own key-settings section
// (origin/main @ fdceb8befbcebfa25d2216d71264bb7c5e8c96d7,
// src/app/(app)/settings/api-key-settings.tsx) but trimmed to what this
// surface actually supports: list + revoke only. Two differences from
// my-task's own version, both direct consequences of
// .chief/milestone-3/_contract/API.md's GET /api/bff/keys shape:
//
//  - No "show revoked" toggle. my-task's api.apiKey.list returns every key
//    (enabled and revoked) and filters client-side; GET /api/bff/keys
//    returns only non-revoked keys already (same listing behavior as
//    GET /api/v1/keys) — there is nothing revoked to reveal.
//  - No "last used"/idle-key warning. bffapi.ApiKey has no lastRequest
//    field (id, prefix, createdAt, expiresAt only) — this template's key
//    schema doesn't track it.
//
// No issuance or rotation UI anywhere on this screen, even a disabled
// button hinting at one — GOAL.md Done-when 5's negative check, same
// reasoning API.md's Conventions section gives for the public API never
// getting a POST /api/v1/keys: a raw key can't be returned safely over an
// HTTP response.
//
// milestone-4/task-7: GET /api/bff/keys's own semantics were replaced by
// task-6 (I21, `_contract/INVARIANTS.md`) — this endpoint now returns
// EVERY role='agent' user's non-revoked keys, not the always-empty
// session-owner-scoped set milestone-2/3 returned. The component's own
// shape barely changes (still a list + a per-key revoke button, same
// `ApiKey` wire shape) — what changes is the copy, since these are no
// longer "your" keys. One real, named gap found while doing this: `ApiKey`
// (`bff-openapi.yaml`) carries `id`/`prefix`/`createdAt`/`expiresAt` only —
// no field naming WHICH agent a given key belongs to (no `handle`, no
// `userId`). `internal/transport/bff/keys_handler.go`'s own `toBFFKey`
// confirms the Go handler never resolves one either. So the list below
// can show and revoke every agent's key, exactly as I21 requires, but
// cannot label a row "this is clara's key" — only its prefix and dates.
// This is the same shape of gap `TimelineEventRow.tsx`'s own
// `TimelineEventData` doc comment names for `TodoEvent.actorId` and
// `Todo.assigneeId`: a wire schema that carries a raw id/prefix where a
// human-facing UI wants a name, with no lookup endpoint this milestone's
// contract provides. Named here rather than worked around — see task-7's
// report for the pattern across all three sites.
import { toast } from "sonner";

import { Button } from "~/components/ui/button";
import { Spinner } from "~/components/ui/spinner";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "~/components/ui/alert-dialog";
import { useKeysQuery, useRevokeKeyMutation, type ApiKey } from "~/lib/keys";

function formatDate(value: string): string {
  return new Date(value).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

function RevokeKeyButton({ id, prefix }: { id: string; prefix: string }) {
  const revokeKey = useRevokeKeyMutation();

  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button variant="outline" size="sm" disabled={revokeKey.isPending}>
          {revokeKey.isPending ? <Spinner size="sm" className="mr-2" /> : null}
          Revoke
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent onClick={(e) => e.stopPropagation()}>
        <AlertDialogHeader>
          <AlertDialogTitle>Revoke key {prefix}…?</AlertDialogTitle>
          <AlertDialogDescription>
            It stops working on the next request. The record stays, so this key drops out of the
            list above. Issuing a new one is CLI-only — this screen never mints a replacement.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            className="bg-destructive text-white hover:bg-destructive/90"
            onClick={() =>
              revokeKey.mutate(id, {
                onSuccess: () => toast.success(`Revoked ${prefix}…`),
                onError: (err) =>
                  toast.error(err instanceof Error ? err.message : "Couldn't revoke that key."),
              })
            }
          >
            Revoke
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function ApiKeySettings() {
  const keysQuery = useKeysQuery();
  const keys: ApiKey[] = keysQuery.data ?? [];

  return (
    <section className="mb-8 rounded-2xl border border-[var(--line)] bg-[var(--chip-bg)] p-6 shadow-sm">
      <h2 className="mb-1 text-base font-semibold text-[var(--sea-ink)]">Agent API keys</h2>
      <p className="mb-4 text-sm text-[var(--sea-ink-soft)]">
        Every agent&apos;s active key, not just one — you can revoke any of them here (I21). Keys
        are still issued and rotated from the CLI only, never from this page — a raw key
        can&apos;t be returned safely over an HTTP response. Revoking is here because it&apos;s
        the part you may need in a hurry.
      </p>

      {keysQuery.isPending ? (
        <div className="flex justify-center py-6">
          <Spinner size="sm" />
        </div>
      ) : keysQuery.isError ? (
        <p className="text-sm text-[var(--sea-ink-soft)]">Couldn&apos;t load agent API keys.</p>
      ) : keys.length === 0 ? (
        <p className="text-sm text-[var(--sea-ink-soft)]">No active agent API keys.</p>
      ) : (
        <ul className="divide-y divide-[var(--line)]">
          {keys.map((k) => (
            <li key={k.id} className="flex items-center justify-between gap-4 py-3">
              <div className="min-w-0">
                <p className="truncate text-sm font-medium text-[var(--sea-ink)]">{k.prefix}…</p>
                <p className="text-xs text-[var(--sea-ink-soft)]">
                  Created {formatDate(k.createdAt)} · Expires {formatDate(k.expiresAt)}
                </p>
              </div>
              <RevokeKeyButton id={k.id} prefix={k.prefix} />
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
