// Package dbquery is a small, non-domain helper package — similar standing
// to internal/api, internal/db, and internal/platform (see
// internal/architecture_test.go's nonDomainInternalDirs) — that holds the
// single implementation behind every domain module's own I4
// ("one seam reads identity" / "one repo, one table") check.
//
// Before task-8, internal/todo and internal/identity each had their own
// near-duplicate copy of this check
// (assertQueriesReferenceOnlyTable/assertQueryFileReferencesOnlyTables),
// and both hardcoded the *other* module's table name(s) as the forbidden
// list: internal/identity's hardcoded "todos", internal/todo's hardcoded
// "users"/"api_keys". Once a fork deletes the todos table and adds its own
// (e.g. snippets), internal/identity's check keeps forbidding a table that
// no longer exists and passes vacuously — proven directly: a test agent
// added `SELECT * FROM snippets;` to db/queries/users.sql and the "table
// isolation" check still passed, because it only ever checked for the
// literal string "todos". This package fixes that by deriving the
// forbidden-table set dynamically from db/queries/*.sql itself, so no
// table name is hardcoded anywhere — see AssertQueryFileReferencesOnlyOwnTable.
package dbquery

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tableReferenceRe matches a table name following FROM, INTO, UPDATE, or
// JOIN — the SQL keywords this repo's queries (db/queries/*.sql) use to
// name the table a statement touches. A reasonable regex for this repo's
// simple, single-table queries; not a general SQL parser.
var tableReferenceRe = regexp.MustCompile(`(?i)\b(?:FROM|INTO|UPDATE|JOIN)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)

// TablesReferencedIn returns the distinct, lowercased table names content
// references via FROM/INTO/UPDATE/JOIN, in first-seen order.
func TablesReferencedIn(content string) []string {
	seen := make(map[string]bool)
	var tables []string
	for _, m := range tableReferenceRe.FindAllStringSubmatch(content, -1) {
		name := strings.ToLower(m[1])
		if !seen[name] {
			seen[name] = true
			tables = append(tables, name)
		}
	}
	return tables
}

// AssertQueryFileReferencesOnlyOwnTable is I4's shared check, the single
// implementation behind every domain module's own dedicated
// TestI4_..._OnlyQueries...Table(s) test (internal/todo/repo_test.go,
// internal/identity/repo_test.go, and any domain module a fork adds
// alongside or instead of them). It asserts two things about the .sql
// file at filepath.Join(queriesDir, filename):
//
//  1. it references ownTable at least once (proving the file isn't
//     vacuously empty of the table it's supposed to own);
//  2. it references no table that belongs to a *different* domain
//     module's query file.
//
// "Belongs to a different module" is derived dynamically, never
// hardcoded: every other .sql file directly under queriesDir is scanned
// (via TablesReferencedIn) for the tables it references, except files
// listed in sameModuleFiles — siblings that belong to the SAME module as
// filename. This distinction matters because a single module can own
// more than one table across more than one file (internal/identity owns
// both users.sql/"users" and api_keys.sql/"api_keys"), and a query
// joining two tables that both belong to the same seam (e.g. resolving an
// API key's owning user) is I4-legal; only a reference to a table owned
// by a genuinely different module is forbidden.
//
// No table name is hardcoded anywhere in this function — a fork that
// renames, adds, or removes a table changes what's forbidden
// automatically, instead of a stale hardcoded list silently passing
// vacuously forever (see this package's doc comment for the exact
// regression this fixes).
func AssertQueryFileReferencesOnlyOwnTable(t *testing.T, queriesDir, filename, ownTable string, sameModuleFiles ...string) {
	t.Helper()

	path := filepath.Join(queriesDir, filename)
	data, err := os.ReadFile(path)
	require.NoErrorf(t, err, "reading %s", path)
	content := string(data)

	assert.Regexpf(t, `(?i)\b`+regexp.QuoteMeta(ownTable)+`\b`, content,
		"%s must reference its own table %q at least once", path, ownTable)

	exempt := map[string]bool{filename: true}
	for _, f := range sameModuleFiles {
		exempt[f] = true
	}

	entries, err := os.ReadDir(queriesDir)
	require.NoErrorf(t, err, "reading %s", queriesDir)

	for _, entry := range entries {
		if entry.IsDir() || exempt[entry.Name()] || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		otherPath := filepath.Join(queriesDir, entry.Name())
		otherData, err := os.ReadFile(otherPath)
		require.NoErrorf(t, err, "reading %s", otherPath)

		for _, forbidden := range TablesReferencedIn(string(otherData)) {
			assert.NotRegexpf(t, `(?i)\b`+regexp.QuoteMeta(forbidden)+`\b`, content,
				"%s must not reference table %q — it belongs to %s, a different module's query file "+
					"(I4: one seam reads identity / one repo, one table)",
				path, forbidden, entry.Name())
		}
	}
}
