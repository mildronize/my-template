// Package internal — a floor for a real, reproduced sqlc bug:
// bin/sqlc v1.31.1 (pinned in go.mod) corrupts its own star-expansion
// byte offsets when a db/queries/*.sql file contains any non-ASCII byte
// (not just an em dash — task-1's own report and todo_events.sql's own
// comment named only the em dash it happened to hit; a real fork
// (DLV-1) hit the identical corruption with "§" and with Thai prose).
// The failure mode is actively misleading: sqlc reports a syntax error
// naming a token that appears in no file ("mismatched input 'SELECd'")
// at a line number that is not where the non-ASCII byte actually is, and
// the corruption can cascade into unrelated queries later in the same
// file. A comment warning this away only reaches whoever opens
// todo_events.sql specifically — this template's own convention
// (em-dash prose, Thai rulings quoted verbatim) makes every other
// db/queries/*.sql file an equally likely place to reintroduce it. This
// check reaches the population the comment can't: anyone editing any
// file under db/queries/, including a fork that never opens
// todo_events.sql at all.
package internal

import (
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

// TestDBQueriesFilesAreASCIIOnly is the floor: every db/queries/*.sql
// byte must be ASCII (<= 0x7F). Migrations are deliberately NOT covered
// here — db/migrations/*.sql already carries non-ASCII prose (em dashes,
// Thai) today and generates fine; the corruption is specific to sqlc's
// query-parsing/star-expansion path, not schema parsing, so scoping this
// check to db/queries/ only (not db/) matches where the bug actually
// lives rather than over-restricting a file type it doesn't affect.
func TestDBQueriesFilesAreASCIIOnly(t *testing.T) {
	root := repoRoot(t)
	queriesDir := filepath.Join(root, "db", "queries")

	entries, err := os.ReadDir(queriesDir)
	require.NoErrorf(t, err, "reading %s", queriesDir)

	found := false
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		found = true
		path := filepath.Join(queriesDir, entry.Name())
		data, err := os.ReadFile(path)
		require.NoErrorf(t, err, "reading %s", path)

		line := 1
		for i := 0; i < len(data); {
			if data[i] < utf8.RuneSelf {
				if data[i] == '\n' {
					line++
				}
				i++
				continue
			}
			r, size := utf8.DecodeRune(data[i:])
			t.Fatalf(
				"%s:%d contains a non-ASCII character (%q) — bin/sqlc v1.31.1 corrupts its own "+
					"star-expansion byte offsets on ANY non-ASCII byte in a db/queries/*.sql file "+
					"(not just an em dash — verified with \"§\" and with Thai prose), producing "+
					"invented tokens and wrong line numbers rather than a useful error at the actual "+
					"location. Use plain ASCII in this file (spell out em dashes as \"--\", "+
					"transliterate or move non-ASCII prose to this test's own comment or to a doc "+
					"outside db/queries/) — see todo_events.sql's own comment for the original, "+
					"narrower report of this bug.",
				path, line, r,
			)
			i += size
		}
	}
	require.Truef(t, found, "%s had no .sql files — this check would otherwise pass trivially", queriesDir)
}
