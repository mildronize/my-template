package platform

import (
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	migrations "github.com/mildronize/my-template/db/migrations"
)

// Migrate applies every pending goose migration in db/migrations (embedded
// at build time via migrations.FS, not read off disk) to db. It is safe to
// call on every process start — goose tracks applied versions in its own
// bookkeeping table and skips anything already applied — which is what
// lets `docker compose up` bring the service up against a fresh, empty
// SQLite volume with no separate migrate step (GOAL.md Done-when 8):
// cmd/server calls this before serving any request, and cmd/issue-key
// calls it too so the one-off key-seeding command works even against a
// volume the server hasn't started against yet.
//
// This is the same migration set and the same goose.Up call
// this function's own test (migrate_test.go,
// TestGooseUp_FullMigrationSetAppliesCleanly) and bin/goose (Makefile,
// docs/DEPLOY-REQUIREMENTS.md) apply — embedding just changes where the
// *.sql bytes come from, not what gets run.
func Migrate(db *sql.DB) error {
	goose.SetBaseFS(migrations.FS)
	defer goose.SetBaseFS(nil)

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("setting goose dialect: %w", err)
	}
	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}
	return nil
}
