// Package dbquery is a small, non-domain helper package — a sibling of
// internal/api, internal/db, and internal/platform, all living directly
// under internal/ rather than internal/domain/ (internal/architecture_
// test.go's domainModuleNames only ever looks under internal/domain/, so
// none of these need excluding by name the way milestone-1's version had
// to) — that holds the single implementation behind every domain module's
// own I4 ("one seam reads identity" / "one repo, one table") check.
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
// literal string "todos". This package fixed that first pass by deriving
// the forbidden-table set dynamically from db/queries/*.sql itself, by
// scanning every other file's content — but a scan-and-guess derivation
// cannot tell a legitimate cross-module *read* from an ownership claim,
// which is exactly the bug milestone-4 found: db/queries/todo_events.sql's
// ListTodoEventsFeed legitimately JOINs users (for the cross-todo feed's
// actor handle/role), and the scan attributed "users" to todo_events.sql
// as if it owned it — which then made users.sql's own, real, "FROM users"
// look like a violation of someone else's ownership.
//
// milestone-4's second pass replaces derivation with an explicit,
// hand-maintained source of truth (TableOwnership, ReadOnlyGrants) —
// same trade the scope-tags fix already made elsewhere this milestone:
// explicit and correct over automatic and wrong. No file is ever scanned
// to guess what it owns; ownership is declared once, and a table with no
// declared owner is a build error, not a silent pass.
package dbquery

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TableOwnership is the explicit, hand-maintained map from every table
// name (lowercase, as it appears in db/queries/*.sql) to the domain
// module that owns it. A module can own more than one table across more
// than one file (internal/identity owns both users.sql/"users" and
// api_keys.sql/"api_keys") — module name, not filename, is the unit of
// ownership, so a query file referencing its own module's *other* table
// is automatically legal without needing a separate exemption list.
//
// A table referenced in db/queries/*.sql with no entry here is a hard
// failure (see referencedTablesStrict's caller in
// AssertQueryFileReferencesOnlyOwnTable), not a silent pass — a fork that
// adds a table and forgets to declare its owner finds out immediately.
var TableOwnership = map[string]string{
	"users":       "identity",
	"api_keys":    "identity",
	"todos":       "todo",
	"todo_events": "todo",
}

// ReadOnlyGrant is the only sanctioned way for a query file to reference
// a table owned by a different module: an explicit, named, per-file,
// per-table grant, enforced read-only *by mechanism* (see writesToTable),
// not merely by the grantor's intent. Adding a grant here is a deliberate
// act, the same shape as perDomainModuleScopePackages
// (internal/invariants_test.go) — a human declaring a fact, not a
// heuristic inferring one.
type ReadOnlyGrant struct {
	File  string // e.g. "todo_events.sql"
	Table string // lowercase; must be owned by a module other than File's own
}

// ReadOnlyGrants is the complete, hand-maintained list. Every entry must
// be exercised — see AssertEveryReadOnlyGrantIsExercised — an unused
// grant is a permanent, unexplained exemption and fails loudly rather
// than accreting silently; an exemption nobody needs is an exemption
// nobody notices.
var ReadOnlyGrants = []ReadOnlyGrant{
	// todo_events.sql's ListTodoEventsFeed joins users for the cross-todo
	// activity feed's actor handle/role (so callers can tell human from
	// agent) — a display read, never a write; todo_events.sql has no
	// query that ever writes to users.
	{File: "todo_events.sql", Table: "users"},
}

func grantedReadOnly(file, table string) bool {
	for _, g := range ReadOnlyGrants {
		if g.File == file && g.Table == table {
			return true
		}
	}
	return false
}

// sqlLineCommentRe matches a `--` line comment through end of line.
// Applied before any scanning below: this repo's .sql files are full of
// English prose in comments (explaining *why*, per this repo's own
// convention), and prose routinely contains "from the", "into the", etc.
// — which a keyword-adjacency scan cannot distinguish from a real SQL
// clause. Without this, a comment's stray "from the" extracts "the" as a
// phantom table name and then flags any *other* file whose comments
// merely contain the word "the" (nearly all of them) as an I4 violation —
// the bug this milestone found and fixed first.
var sqlLineCommentRe = regexp.MustCompile(`--[^\n]*`)

func stripSQLLineComments(content string) string {
	return sqlLineCommentRe.ReplaceAllString(content, "")
}

// keywordRe finds every FROM/INTO/UPDATE/JOIN occurrence — the SQL
// keywords this repo's queries (db/queries/*.sql) use to name the table a
// statement touches.
var keywordRe = regexp.MustCompile(`(?i)\b(?:FROM|INTO|UPDATE|JOIN)\b`)

// identThenTailRe requires at least one whitespace character after the
// keyword, then a bare identifier, then captures whatever (whitespace)
// follows it — used to decide whether the very next thing after the
// identifier is a comma (see referencedTablesStrict).
var identThenTailRe = regexp.MustCompile(`^(\s+)([a-zA-Z_][a-zA-Z0-9_]*)(\s*)`)

// referencedTablesStrict returns the distinct, lowercased table names
// content references via FROM/INTO/UPDATE/JOIN, in first-seen order —
// or an error if any such keyword is followed by something this scanner
// does not understand.
//
// Deliberately NOT "handle more SQL forms": milestone-4 found this
// scanner has real, confirmed blind spots (quoted/bracketed identifiers,
// comma-joins, a comment interposed between keyword and table name) —
// each empirically verified to make the *old* version extract nothing at
// all for that reference, silently. Widening the regex to cover the next
// form found is an arms race against forms nobody has enumerated yet, and
// every unhandled form would stay invisible in exactly the same way.
// Instead: require every keyword occurrence to resolve to a plain
// identifier with nothing else attached, and fail loudly — "unsupported
// SQL form" — the moment it does not. That converts an unknown-unknown
// into a build error: the mechanism stops claiming to have checked SQL it
// did not understand, instead of silently checking less than it reports.
func referencedTablesStrict(content string) ([]string, error) {
	stripped := stripSQLLineComments(content)
	seen := make(map[string]bool)
	var tables []string

	for _, loc := range keywordRe.FindAllStringIndex(stripped, -1) {
		keyword := stripped[loc[0]:loc[1]]
		rest := stripped[loc[1]:]

		m := identThenTailRe.FindStringSubmatch(rest)
		if m == nil {
			snippet := rest
			if len(snippet) > 40 {
				snippet = snippet[:40]
			}
			return nil, fmt.Errorf(
				"unsupported SQL form: %q is not followed by a plain identifier (found %q) — quoted/"+
					"bracketed identifiers, subqueries, and other non-bare-identifier forms after FROM/"+
					"INTO/UPDATE/JOIN are not understood by this scanner; rewrite the query to use a "+
					"plain identifier, or extend this scanner deliberately (referencedTablesStrict)",
				keyword, snippet)
		}

		afterIdent := rest[len(m[0]):]
		if len(afterIdent) > 0 && afterIdent[0] == ',' {
			return nil, fmt.Errorf(
				"unsupported SQL form: %q %s is immediately followed by a comma — a comma-separated "+
					"table list after FROM/INTO/UPDATE/JOIN is not understood by this scanner (only one "+
					"table name per clause); rewrite as an explicit JOIN, or extend this scanner "+
					"deliberately (referencedTablesStrict)",
				keyword, strings.TrimSpace(m[2]))
		}

		name := strings.ToLower(m[2])
		if !seen[name] {
			seen[name] = true
			tables = append(tables, name)
		}
	}
	return tables, nil
}

// writesToTable reports whether (already comment-stripped) content
// writes to table via INTO or UPDATE specifically — used to enforce that
// a ReadOnlyGrant is read-only *by mechanism*, not merely by the
// grantor's stated intent.
func writesToTable(strippedContent, table string) bool {
	re := regexp.MustCompile(`(?i)\b(?:INTO|UPDATE)\s+` + regexp.QuoteMeta(table) + `\b`)
	return re.MatchString(strippedContent)
}

// testingT is the subset of *testing.T this file's assertions need —
// deliberately an interface, not a concrete *testing.T, so this
// package's own tests can substitute a recording fake to prove a check
// fails without that failure propagating to the real test binary (see
// tableisolation_test.go's failRecorder). *testing.T satisfies this
// structurally; every real call site is unaffected.
type testingT interface {
	require.TestingT
	Helper()
}

// AssertQueryFileReferencesOnlyOwnTable is I4's shared check, the single
// implementation behind every domain module's own dedicated
// TestI4_..._OnlyQueries...Table(s) test (internal/domain/todo/
// repo_test.go, internal/identity/repo_test.go, and any domain module a
// fork adds alongside or instead of them). It asserts, about the .sql
// file at filepath.Join(queriesDir, filename):
//
//  1. it references ownTable at least once (proving the file isn't
//     vacuously empty of the table it's supposed to own);
//  2. every table it references either belongs to the same module as
//     ownTable (TableOwnership), or is covered by an explicit
//     ReadOnlyGrant naming this exact file and table — and if the latter,
//     that the file never writes (INTO/UPDATE) to that table;
//  3. every table it references has a declared owner in TableOwnership
//     at all — a table with no entry there is a hard failure, not a
//     silent pass, the same "explicit or it doesn't count" discipline as
//     everything else in this file.
//
// Ownership is never guessed by scanning other files' content — see this
// package's own doc comment for why that used to be the design, and what
// it got wrong.
func AssertQueryFileReferencesOnlyOwnTable(t testingT, queriesDir, filename, ownTable string) {
	t.Helper()

	path := filepath.Join(queriesDir, filename)
	data, err := os.ReadFile(path)
	require.NoErrorf(t, err, "reading %s", path)
	content := string(data)
	stripped := stripSQLLineComments(content)

	ownTableLower := strings.ToLower(ownTable)
	module, ok := TableOwnership[ownTableLower]
	require.Truef(t, ok,
		"%s claims to own table %q, but %q has no entry in TableOwnership (internal/dbquery/"+
			"tableisolation.go) — add it there first", filename, ownTable, ownTable)

	tables, err := referencedTablesStrict(content)
	require.NoErrorf(t, err, "%s", path)

	assert.Containsf(t, tables, ownTableLower,
		"%s must reference its own table %q at least once", path, ownTable)

	for _, tbl := range tables {
		owner, ok := TableOwnership[tbl]
		if !ok {
			assert.Failf(t, "undeclared table",
				"%s references table %q, which has no entry in TableOwnership (internal/dbquery/"+
					"tableisolation.go) — add it there before this file may reference it", path, tbl)
			continue
		}
		if owner == module {
			continue
		}
		if grantedReadOnly(filename, tbl) {
			assert.Falsef(t, writesToTable(stripped, tbl),
				"%s has a ReadOnlyGrant for table %q but writes to it (INTO/UPDATE) — grants are "+
					"read-only by mechanism, not just by intent (I4)", path, tbl)
			continue
		}
		assert.Failf(t, "I4 violation",
			"%s must not reference table %q — it belongs to the %q module and %s has no ReadOnlyGrant "+
				"for it (I4: one seam reads identity / one repo, one table)", path, tbl, owner, filename)
	}
}

// AssertEveryReadOnlyGrantIsExercised asserts every entry in
// ReadOnlyGrants actually corresponds to a real reference in the named
// file — an unused grant is a permanent, unexplained exemption, and this
// is the floor-assertion that catches it (the same shape as I15's own
// floor, inverted: there, zero matches meant nothing was checked; here,
// zero uses means something is permanently permitted for no reason).
func AssertEveryReadOnlyGrantIsExercised(t testingT, queriesDir string) {
	t.Helper()

	for _, g := range ReadOnlyGrants {
		path := filepath.Join(queriesDir, g.File)
		data, err := os.ReadFile(path)
		require.NoErrorf(t, err, "reading %s", path)

		tables, err := referencedTablesStrict(string(data))
		require.NoErrorf(t, err, "%s", path)

		assert.Containsf(t, tables, strings.ToLower(g.Table),
			"ReadOnlyGrant{File: %q, Table: %q} is unused — %s never references table %q; remove the "+
				"grant (an exemption nobody needs is an exemption nobody notices)", g.File, g.Table, g.File, g.Table)
	}
}
