// Ported verbatim from my-task (origin/main @ fdceb8befbcebfa25d2216d71264bb7c5e8c96d7,
// src/components/Footer.tsx) — the only change is the env-var access on the
// line below: Next.js's `process.env.NEXT_PUBLIC_*` has no meaning under
// Vite, whose equivalent build-time-substituted env access is
// `import.meta.env.VITE_*` (https://vite.dev/guide/env-and-mode.html).
// Everything else — the JSX, the dev-vs-real-commit branch — is unchanged.
const commitSha = import.meta.env.VITE_COMMIT_SHA ?? "dev";
const repoUrl = "https://github.com/mildronize/prompt-shelf";

export default function Footer() {
  const isdev = commitSha === "dev";

  return (
    <footer className="py-4 text-center text-xs" style={{ color: "var(--sea-ink-soft)" }}>
      {isdev ? (
        <span>v.dev</span>
      ) : (
        <a
          href={`${repoUrl}/commit/${commitSha}`}
          target="_blank"
          rel="noopener noreferrer"
          className="no-underline transition hover:opacity-80"
          style={{ color: "var(--sea-ink-soft)" }}
        >
          v.{commitSha}
        </a>
      )}
    </footer>
  );
}
