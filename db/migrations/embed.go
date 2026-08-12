// Package migrations embeds this directory's goose SQL migration files
// into the compiled binary, so cmd/server (and cmd/issue-key) can apply
// them at startup without db/migrations/ needing to exist on disk inside
// the runtime container — the Dockerfile's runtime stage copies only the
// compiled binaries, not the source tree. sqlc and the goose CLI
// (bin/goose, Makefile) keep reading these same *.sql files straight off
// disk as before; this file adds a second, embedded way to reach them and
// changes nothing about the migrations themselves.
package migrations

import "embed"

// FS embeds every *.sql file in this directory at its root (not nested
// under "migrations/"), so goose.Up(db, ".") after
// goose.SetBaseFS(migrations.FS) applies the same migration set
// bin/goose applies from disk.
//
//go:embed *.sql
var FS embed.FS
