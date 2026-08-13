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
// longer "your" keys.
//
// milestone-4 handle-exposure fix-round: task-7 found (and named, rather
// than working around) that `ApiKey` carried `id`/`prefix`/`createdAt`/
// `expiresAt` only — no field naming WHICH agent a given key belongs to.
// `handle` closes that gap (`bff-openapi.yaml`'s `ApiKey.handle`,
// `internal/transport/bff/keys_handler.go`'s `toBFFKey`) — this component
// now shows it on every row and names it in the revoke confirmation,
// mirroring my-task's own
// `src/app/(app)/settings/api-key-settings.tsx` exactly:
// `ApiKeyRow`'s `<span className="truncate">{k.handle}</span>` as the
// row's primary text (prefix/dates demoted to the secondary line, the
// reverse of this component's pre-fix-round layout), and
// `RevokeKeyButton`'s dialog title "Revoke {handle}'s key?" verbatim.
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

function RevokeKeyButton({ id, handle }: { id: string; handle: string }) {
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
          {/* my-task's own exact copy shape (api-key-settings.tsx's
              RevokeKeyButton): "Revoke {handle}'s key?" — the handle
              names WHICH agent, not just that a key is being revoked. */}
          <AlertDialogTitle>Revoke {handle}&apos;s key?</AlertDialogTitle>
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
                onSuccess: () => toast.success(`Revoked ${handle}'s key.`),
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
                {/* handle is the row's own primary text, mirroring
                    my-task's ApiKeyRow (`{k.handle}`) — which agent this
                    key belongs to is the thing this whole page exists to
                    show (I21). */}
                <p className="truncate text-sm font-medium text-[var(--sea-ink)]">{k.handle}</p>
                <p className="text-xs text-[var(--sea-ink-soft)]">
                  {k.prefix}… · Created {formatDate(k.createdAt)} · Expires{" "}
                  {formatDate(k.expiresAt)}
                </p>
              </div>
              <RevokeKeyButton id={k.id} handle={k.handle} />
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
