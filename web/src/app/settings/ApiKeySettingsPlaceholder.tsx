// TPL-1 milestone-3/task-1 placeholder.
//
// my-task's real src/app/(app)/settings/api-key-settings.tsx (bucket 3,
// "must rewrite" per .chief/milestone-3/_goal/GOAL.md's inventory table)
// drives its list/revoke UI through tRPC, which this template doesn't
// carry. task-3 replaces this file with the real thing, wired against
// GET/DELETE /api/bff/keys (.chief/milestone-3/_contract/API.md) once
// task-2 builds those endpoints — list and revoke only, per this
// milestone's Decisions table: no issuance or rotation from this screen,
// same reasoning API.md's Conventions section already gives for the
// public API never getting a POST /api/v1/keys.
//
// Until then this exists purely so settings/page.tsx (below) has
// something to compose, proving the route/shell wiring works.
export function ApiKeySettingsPlaceholder() {
  return (
    <section className="mb-8 rounded-2xl border border-[var(--line)] bg-[var(--chip-bg)] p-6 shadow-sm">
      <h2 className="mb-1 text-base font-semibold text-[var(--sea-ink)]">Agent API keys</h2>
      <p className="text-sm text-[var(--sea-ink-soft)]">
        Key management lands in task-3, once GET/DELETE /api/bff/keys exist.
      </p>
    </section>
  );
}
