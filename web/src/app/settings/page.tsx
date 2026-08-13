// Ported from my-task (origin/main @ fdceb8befbcebfa25d2216d71264bb7c5e8c96d7,
// src/app/(app)/settings/page.tsx) — the settings page shell. Verbatim
// except: no StatusSettings section (my-task's status/label/priority
// domain is out of this template's scope per GOAL.md's own out-of-scope
// note), ApiKeySettings now wired for real (milestone-3/task-3, replacing
// task-1's ApiKeySettingsPlaceholder — see ApiKeySettings.tsx's own
// comment), and `export const dynamic = "force-dynamic"` dropped — a
// Next.js App Router route-segment config with no meaning under Vite's
// plain client-side rendering.
import { ApiKeySettings } from "./ApiKeySettings";

export default function SettingsPage() {
  return (
    <main className="mx-auto max-w-2xl px-4 py-8">
      <h1 className="mb-8 text-2xl font-bold text-[var(--sea-ink)]">Settings</h1>

      <section className="mb-8 rounded-2xl border border-[var(--line)] bg-[var(--chip-bg)] p-6 shadow-sm">
        <h2 className="mb-1 text-base font-semibold text-[var(--sea-ink)]">Account</h2>
        <p className="text-sm text-[var(--sea-ink-soft)]">
          Your account is managed by the homelab SSO. Sign-in and password are handled there.
        </p>
      </section>
      <ApiKeySettings />
    </main>
  );
}
