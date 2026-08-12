package platform

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver (pure Go, no cgo)
)

// OpenDB opens the SQLite database file at path, creating its parent
// directory if necessary, and returns a ready-to-use *sql.DB. This is a
// plain database/sql connection today; once sqlc's generated code exists
// (a later task), it's built on top of the *sql.DB returned here rather
// than replacing it.
func OpenDB(path string) (*sql.DB, error) {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating database directory %q: %w", dir, err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening database %q: %w", path, err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging database %q: %w", path, err)
	}

	return db, nil
}
