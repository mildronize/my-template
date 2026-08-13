// Package web embeds the Vite-built SPA (web/dist) so cmd/server can
// serve it without a separate static-file deployment step — one binary,
// one artifact, per .chief/milestone-3/_goal/GOAL.md's "SPA serving"
// Decisions row.
//
// This file has to live here, inside web/, rather than in cmd/server
// alongside the rest of the server-wiring code: a //go:embed pattern is
// resolved relative to the source file's own directory and can't cross a
// ".." — cmd/server is a sibling of web/, not an ancestor, so only a file
// physically inside web/ can embed web/dist. ARCHITECTURE.md's own layout
// section already names this convention ("web/ # repo root, Go
// convention — not under internal/"); this is the one Go file that lives
// there.
//
// dist's real content only exists after `npm run build` (or `make build`,
// which runs it first) — see web/.gitignore's own comment for why a
// tracked dist/.gitkeep placeholder keeps `go build ./...`/`go test ./...`
// compiling on a fresh clone that hasn't run npm yet, even though the
// embedded FS is empty-but-for-that-placeholder until someone does.
package web

import "embed"

// DistFS holds web/dist's contents at Go build time — "all:" so Vite's
// own dist/.gitkeep-adjacent output (nothing dot-prefixed today, but
// nothing rules it out later) isn't silently excluded the way a plain
// "embed dist" pattern would exclude dotfiles.
//
//go:embed all:dist
var DistFS embed.FS
