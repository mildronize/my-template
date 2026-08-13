// milestone-4/task-7: renders todo/comment bodies as Markdown. Ported from
// my-task's own src/components/Markdown.tsx (the named source
// TimelineEventRow.tsx mirrors — see that file's header for the citation),
// mechanically adapted for this repo (no other logic change):
//
//  - No import path change needed — this repo's own `~` alias also
//    resolves to `src/` (vite.config.ts), so `~/components/Markdown` reads
//    identically here and there.
//
// Read this before changing anything here — I20 (`_contract/INVARIANTS.md`,
// `.chief/_rules/_contract/INVARIANTS.md` once promoted) is this file's
// entire reason to exist, and it mirrors my-task's own I8 exactly,
// including the stated reason: the activity log is a cross-agent channel,
// and comment bodies are written by agents (or pasted in from outside by
// one) — this is the one place a stored string becomes something มายด์'s
// own browser executes.
//
// **No HTML string is ever produced, and `dangerouslySetInnerHTML` —
// React's raw-HTML escape hatch — is never reached for.**
// `react-markdown` parses to an AST and emits React elements directly —
// there is no intermediate HTML string for a sanitiser to be bypassed on,
// because there is no sanitiser and no HTML.
// (`internal/frontend_safety_test.go`'s `TestI20_...` check greps web/src
// for that identifier verbatim to prove this file-wide — comments
// included; it strips comments before scanning, so naming the API here,
// in prose explaining it is deliberately avoided, does not trip it.)
//
// What is actually doing the work here, same four points my-task's own
// header documents (not re-verified independently for this port beyond
// this milestone's own I20 test, TimelineEventRow.test.tsx's
// "I20: escapes raw HTML instead of rendering it live" — see that test for
// this repo's own proof):
//
//  1. `components.img` — images are not loaded. A remote image fetches the
//     moment the page opens; showing alt text instead means an agent can't
//     make มายด์'s browser fetch a URL of its own choosing with no click
//     involved.
//  2. `components.a` — `rel="noopener noreferrer nofollow"`,
//     `target="_blank"`.
//  3. `urlTransform` — react-markdown's own default already blocks every
//     scheme this allowlist blocks. Kept anyway so the policy is ours and
//     stated locally, not inherited from a transitive dependency's default
//     that can loosen without anything here changing. Must not be
//     described as what stops `javascript:` URLs — the tests establish
//     that, and they pass either way.
//  4. No `skipHtml` — raw HTML does not render as elements; react-markdown
//     escapes it, so `<img src=x onerror=...>` arrives as inert visible
//     text. `skipHtml` would DELETE anything shaped like a tag instead,
//     which is not a safety control here and would silently eat legitimate
//     text (an agent discussing code who types `<div>` in a sentence).
//
// `remark-gfm` is on (tables, strikethrough, task lists, footnotes,
// autolinked bare URLs) — the one thing that touches this file's threat
// model is autolinks, which go through the same `urlTransform` as an
// authored link.
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

const ALLOWED_PROTOCOLS = new Set(["http:", "https:", "mailto:"]);

/**
 * Returns "" for anything not on the allowlist, which react-markdown
 * renders as an anchor with no `href` — visible text, dead link. Relative
 * URLs are resolved against a dummy base purely to read their protocol;
 * the original string is what gets returned.
 */
export function safeUrl(url: string): string {
  try {
    const parsed = new URL(url, "https://my-template.invalid/");
    return ALLOWED_PROTOCOLS.has(parsed.protocol) ? url : "";
  } catch {
    return "";
  }
}

export function Markdown({ children }: { children: string }) {
  return (
    <div className="prose-sm max-w-none break-words [&_*]:my-1 [&_code]:rounded [&_code]:bg-[var(--chip-bg)] [&_code]:px-1 [&_code]:font-mono [&_code]:text-xs [&_h1]:text-base [&_h1]:font-bold [&_h2]:text-sm [&_h2]:font-bold [&_h3]:text-sm [&_h3]:font-semibold [&_li]:ml-4 [&_li]:list-disc [&_ol>li]:list-decimal [&_pre]:overflow-x-auto [&_pre]:rounded [&_pre]:bg-[var(--chip-bg)] [&_pre]:p-2">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        urlTransform={safeUrl}
        components={{
          a: ({ href, children: label }) => (
            <a
              href={href}
              target="_blank"
              rel="noopener noreferrer nofollow"
              className="underline underline-offset-2"
            >
              {label}
            </a>
          ),
          // Deliberately not an <img>. See note 1 in the header.
          img: ({ alt }) => (
            <span className="text-[var(--sea-ink-soft)]">[image{alt ? `: ${alt}` : ""}]</span>
          ),
        }}
      >
        {children}
      </ReactMarkdown>
    </div>
  );
}
