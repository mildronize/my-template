// Ported from my-task (origin/main @ fdceb8befbcebfa25d2216d71264bb7c5e8c96d7,
// src/components/Header.tsx) per this milestone's bucket-2 rule: routing
// only, nothing else changes. next/link's `Link` -> react-router's `Link`
// (whose navigation prop is named `to`, not `href` — the one mechanical
// consequence of swapping components, not a logic change), and
// next/navigation's `usePathname` -> react-router's `useLocation().pathname`.
//
// NAV_LINKS and the logo text below are the one further edit task-1 left
// for task-3 (its own report: "flagged in docs/GETTING-STARTED.md's new
// Step 3b as one of the domain-noun spots ... task-3 still needs to
// revisit") — a content fix, not a logic change: at task-3 time, my-task's
// own Activity/Projects routes didn't exist in this template, so the list
// was trimmed to the one real content route this SPA had.
//
// milestone-4/task-7 adds "Activity" back — this template does now have
// an activity log (ActivityPage.tsx, "/activity"), the one my-task-shaped
// route task-3's own comment above said didn't apply yet. Still no
// "Projects" — that domain concept genuinely doesn't exist here
// (GOAL.md's out-of-scope note, unchanged by this milestone).
import { useState } from "react";
import { Link, useLocation } from "react-router-dom";
import ThemeToggle from "./ThemeToggle";
import { useSession, signOut } from "~/lib/auth-client";
import { Menu, X, ListChecks } from "lucide-react";
import {
  Popover,
  PopoverTrigger,
  PopoverContent,
} from "~/components/ui/popover";

const NAV_LINKS: ReadonlyArray<{ href: string; label: string }> = [
  { href: "/", label: "Todos" },
  { href: "/activity", label: "Activity" },
];

/** Return up to two uppercase initials from a display name. */
function initials(name: string): string {
  const parts = name.trim().split(/\s+/);
  if (parts.length === 1) return (parts[0]?.[0] ?? "").toUpperCase();
  return ((parts[0]?.[0] ?? "") + (parts[parts.length - 1]?.[0] ?? "")).toUpperCase();
}

export default function Header() {
  const { data: session, isPending } = useSession();
  const pathname = useLocation().pathname;
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const [userPopoverOpen, setUserPopoverOpen] = useState(false);

  async function handleSignOut() {
    await signOut();
    window.location.href = "/";
  }

  function isActive(href: string) {
    if (href === "/") return pathname === "/";
    return pathname.startsWith(href);
  }

  return (
    <header className="sticky top-0 z-50 border-b border-[var(--line)] bg-[var(--header-bg)] px-4">
      <nav className="mx-auto flex max-w-7xl items-center gap-3 py-3 sm:py-4">
        {/* Logo pill */}
        <Link
          to="/"
          className="inline-flex items-center gap-2 rounded-full border border-[var(--chip-line)] bg-[var(--chip-bg)] px-3 py-1.5 text-sm font-semibold text-[var(--sea-ink)] no-underline shadow-[0_8px_24px_rgba(26,46,74,0.08)] sm:px-4 sm:py-2"
        >
          <ListChecks className="size-4" />
          {/* Hide text on mobile, show on md+ */}
          <span className="hidden lg:inline">My Template</span>
        </Link>

        {/* Desktop nav links */}
        <div className="hidden items-center gap-1 lg:flex">
          {NAV_LINKS.map((link) => (
            <Link
              key={link.href}
              to={link.href}
              className={[
                "rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
                isActive(link.href)
                  ? "bg-[var(--link-bg-hover)] text-[var(--sea-ink)]"
                  : "text-[var(--sea-ink-soft)] hover:bg-[var(--link-bg-hover)] hover:text-[var(--sea-ink)]",
              ].join(" ")}
            >
              {link.label}
            </Link>
          ))}
        </div>

        <div className="ml-auto flex items-center gap-2">
          <ThemeToggle />

          {/* Desktop: user avatar popover with Settings + Sign out */}
          {!isPending && session && (
            <Popover open={userPopoverOpen} onOpenChange={setUserPopoverOpen}>
              <PopoverTrigger asChild>
                <button
                  className="hidden lg:inline-flex items-center justify-center size-8 rounded-full border border-[var(--line)] bg-[var(--chip-bg)] text-xs font-semibold text-[var(--sea-ink)] transition hover:border-[var(--lagoon)] hover:bg-[var(--link-bg-hover)]"
                  aria-label="User menu"
                  title={session.user.name}
                >
                  {initials(session.user.name)}
                </button>
              </PopoverTrigger>
              <PopoverContent
                align="end"
                className="w-48 p-1 border-[var(--line)] bg-[var(--chip-bg)]"
              >
                <div className="mb-1 px-3 py-2 border-b border-[var(--line)]">
                  <p className="text-xs font-medium text-[var(--sea-ink)] truncate">
                    {session.user.name}
                  </p>
                  <p className="text-xs text-[var(--sea-ink-soft)] truncate">
                    {session.user.email}
                  </p>
                </div>
                <Link
                  to="/settings"
                  onClick={() => setUserPopoverOpen(false)}
                  className={[
                    "block rounded-md px-3 py-2 text-sm font-medium text-[var(--sea-ink)] transition-colors hover:bg-[var(--link-bg-hover)]",
                    pathname === "/settings" ? "bg-[var(--link-bg-hover)]" : "",
                  ].join(" ")}
                >
                  Settings
                </Link>
                <button
                  onClick={() => { setUserPopoverOpen(false); void handleSignOut(); }}
                  className="w-full rounded-md px-3 py-2 text-left text-sm font-medium text-[var(--sea-ink)] transition-colors hover:bg-[var(--link-bg-hover)]"
                >
                  Sign out
                </button>
              </PopoverContent>
            </Popover>
          )}

          {/* Mobile hamburger — visible below lg */}
          <button
            onClick={() => setMobileMenuOpen((prev) => !prev)}
            className="inline-flex size-8 items-center justify-center rounded-full text-[var(--sea-ink-soft)] transition hover:bg-[var(--link-bg-hover)] hover:text-[var(--sea-ink)] lg:hidden"
            aria-label={mobileMenuOpen ? "Close menu" : "Open menu"}
          >
            {mobileMenuOpen ? <X className="size-4" /> : <Menu className="size-4" />}
          </button>
        </div>
      </nav>

      {/* Mobile dropdown — nav links + Settings + Sign out */}
      {mobileMenuOpen && (
        <div className="border-t border-[var(--line)] py-2 lg:hidden">
          {NAV_LINKS.map((link) => (
            <Link
              key={link.href}
              to={link.href}
              onClick={() => setMobileMenuOpen(false)}
              className={[
                "block px-4 py-2.5 text-sm font-medium transition-colors",
                isActive(link.href)
                  ? "bg-[var(--link-bg-hover)] text-[var(--sea-ink)]"
                  : "text-[var(--sea-ink-soft)] hover:bg-[var(--link-bg-hover)] hover:text-[var(--sea-ink)]",
              ].join(" ")}
            >
              {link.label}
            </Link>
          ))}
          {!isPending && session && (
            <>
              <div className="my-2 border-t border-[var(--line)]" />
              <Link
                to="/settings"
                onClick={() => setMobileMenuOpen(false)}
                className={[
                  "block px-4 py-2.5 text-sm font-medium transition-colors",
                  pathname === "/settings"
                    ? "bg-[var(--link-bg-hover)] text-[var(--sea-ink)]"
                    : "text-[var(--sea-ink-soft)] hover:bg-[var(--link-bg-hover)] hover:text-[var(--sea-ink)]",
                ].join(" ")}
              >
                Settings
              </Link>
              <button
                onClick={() => { setMobileMenuOpen(false); void handleSignOut(); }}
                className="block w-full px-4 py-2.5 text-left text-sm font-medium text-[var(--sea-ink-soft)] transition-colors hover:bg-[var(--link-bg-hover)] hover:text-[var(--sea-ink)]"
              >
                Sign out
              </button>
            </>
          )}
        </div>
      )}
    </header>
  );
}
