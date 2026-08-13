// Package internal — this file holds I20's Go-side half. I20
// (`_rules/_contract/INVARIANTS.md`) requires comment bodies to never
// render as raw HTML, proven two complementary ways, neither a
// substitute for the other: a Vitest test (web/src, task-7) proves a
// comment body containing raw HTML tags actually renders escaped, not as
// live markup — the behavioral half; this file proves the dangerous API
// that would let that happen is not reachable anywhere in the frontend
// source at all — the structural half. The Vitest test cannot prove some
// other component won't reach for dangerouslySetInnerHTML next month;
// this check cannot prove escaping actually happens where it's supposed
// to. Together they cover I20; either alone is a claim about half of it.
//
// Go-side, not JS-side, for a mechanical reason: TestDoneWhen12
// (invariants_test.go) collects invariant-test coverage by parsing *.go
// files for TestI<N>_-prefixed function names — a Vitest test named
// TestI20_... in TypeScript is invisible to it. Rather than write a
// hollow Go TestI20_ purely to satisfy that name scan (the honest-looking
// version of the exact "stub that satisfies the check but proves
// nothing" shape this milestone has repeatedly found and fixed
// elsewhere), this file makes the Go-side test real: a genuine,
// independently meaningful guarantee — the forbidden API is not present
// anywhere in web/src — that happens to also be checkable from Go.
package internal

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// jsBlockCommentRe and jsLineCommentRe strip JS/TS comments before the
// scan below runs — the same fix, for the same reason, as
// internal/dbquery/tableisolation.go's stripSQLLineComments: a substring
// scan cannot tell a real usage of the forbidden API from a comment that
// merely NAMES it while explaining it is deliberately avoided ("this file
// never reaches for dangerouslySetInnerHTML"), and this file's own
// original header comment did exactly that, tripping itself as a false
// positive (task-7's report). Block comments are stripped first — a
// line-comment strip alone would leave a multi-line /* ... */ block's own
// interior text unstripped past its first line.
var (
	jsBlockCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)
	jsLineCommentRe  = regexp.MustCompile(`//[^\n]*`)
)

func stripJSComments(content string) string {
	content = jsBlockCommentRe.ReplaceAllString(content, "")
	content = jsLineCommentRe.ReplaceAllString(content, "")
	return content
}

// frontendSourceExtensions is every file type web/src actually holds
// application code in — deliberately broad (not just .tsx) since
// dangerouslySetInnerHTML is a JS-level API, reachable from plain .ts/.js
// just as easily as .tsx.
var frontendSourceExtensions = map[string]bool{
	".ts":  true,
	".tsx": true,
	".js":  true,
	".jsx": true,
}

// dangerouslySetInnerHTMLIdentifier is the exact API I20 forbids —
// React's own escape hatch out of its default (safe) escaping behavior.
// A plain substring check, not an AST parse: unlike architecture_test.go's
// Go-side checks (which have go/ast available), this file has no
// TypeScript parser to reach for, and the identifier itself is
// distinctive enough that a substring match has no realistic false-
// positive surface here — it's not a common English word or a name any
// unrelated identifier would plausibly contain.
const dangerouslySetInnerHTMLIdentifier = "dangerouslySetInnerHTML"

// frontendSourceFiles walks webSrcDir and returns every file with a
// frontendSourceExtensions suffix, in encounter order.
func frontendSourceFiles(t *testing.T, webSrcDir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(webSrcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if frontendSourceExtensions[filepath.Ext(path)] {
			files = append(files, path)
		}
		return nil
	})
	require.NoErrorf(t, err, "walking %s", webSrcDir)
	return files
}

// --- comment-prose regression (found the same day, in this file's own
// first version — task-7's report: Markdown.tsx's own header comment,
// explaining in prose that the forbidden API is never used, tripped this
// exact check as a false positive) ---

func TestStripJSComments_IgnoresProseNamingTheForbiddenAPIInALineComment(t *testing.T) {
	content := "// this file never uses dangerouslySetInnerHTML, on purpose\nconst x = 1;"
	got := stripJSComments(content)
	assert.NotContains(t, got, dangerouslySetInnerHTMLIdentifier,
		"a line comment merely naming the API in prose must not survive stripping")
	assert.Contains(t, got, "const x = 1;", "real code outside the comment must survive stripping")
}

func TestStripJSComments_IgnoresProseNamingTheForbiddenAPIInABlockComment(t *testing.T) {
	content := "/*\n * dangerouslySetInnerHTML is deliberately never reached for here.\n */\nconst x = 1;"
	got := stripJSComments(content)
	assert.NotContains(t, got, dangerouslySetInnerHTMLIdentifier,
		"a multi-line block comment merely naming the API in prose must not survive stripping")
	assert.Contains(t, got, "const x = 1;")
}

func TestStripJSComments_StillCatchesRealUsageOutsideAComment(t *testing.T) {
	content := "// safe rendering below\nconst el = <div dangerouslySetInnerHTML={{ __html: body }} />;"
	got := stripJSComments(content)
	assert.Contains(t, got, dangerouslySetInnerHTMLIdentifier,
		"real code outside any comment must survive stripping — this is the positive control proving "+
			"the strip doesn't just hide everything")
}

// meetsI20Floor is this check's own floor rule, the same shape as I15's
// meetsI15Floor (architecture_test.go) and for the identical reason: a
// scan that silently finds zero (or nearly zero) files — a moved web/src,
// a typo'd extension list, a build-layout change — would report "no
// violations found" and go green while having checked nothing. 10 is
// comfortably below web/src's actual file count at the time this check
// was written (30+) but high enough that "the scan is fundamentally
// broken" and "this is a small, legitimate repo" are distinguishable.
func meetsI20Floor(count int) bool {
	return count >= 10
}

// TestI20Floor_CanActuallyFail proves meetsI20Floor's >= expression
// really does flip to false below the floor, rather than trusting it was
// written correctly by inspection — mirrors TestI15Floor_CanActuallyFail
// exactly, same reasoning.
func TestI20Floor_CanActuallyFail(t *testing.T) {
	assert.False(t, meetsI20Floor(0), "zero scanned files must fail the floor, not pass trivially")
	assert.False(t, meetsI20Floor(1))
	assert.False(t, meetsI20Floor(9))
	assert.True(t, meetsI20Floor(10))
	assert.True(t, meetsI20Floor(30))
}

// TestI20_FrontendNeverUsesDangerouslySetInnerHTML is I20's Go-side half
// — see this file's own package doc comment above for why a structural
// check and a behavioral (Vitest) check together, not either alone.
//
// Fails loudly, not silently, if web/src is missing entirely (a moved
// directory, a build-layout change) rather than treating "nothing to
// scan" as "nothing found, therefore safe" — the exact distinction I15's
// own floor exists to draw, applied here to a missing directory instead
// of a missing function set.
func TestI20_FrontendNeverUsesDangerouslySetInnerHTML(t *testing.T) {
	root := repoRoot(t)
	webSrcDir := filepath.Join(root, "web", "src")

	info, err := os.Stat(webSrcDir)
	require.NoErrorf(t, err, "web/src must exist at %s for this check to mean anything — a missing directory "+
		"is not the same claim as \"scanned and found nothing\", and must not be silently treated as one (I20)",
		webSrcDir)
	require.Truef(t, info.IsDir(), "%s exists but is not a directory", webSrcDir)

	files := frontendSourceFiles(t, webSrcDir)
	require.GreaterOrEqualf(t, len(files), 10,
		"expected at least 10 frontend source files (.ts/.tsx/.js/.jsx) under %s — found %d: %v. "+
			"A scan that finds too few (or zero) files would pass the check below trivially and enforce "+
			"nothing (I20).", webSrcDir, len(files), files)
	require.Truef(t, meetsI20Floor(len(files)),
		"meetsI20Floor disagrees with the >= 10 assertion just above — should never happen")

	for _, path := range files {
		data, err := os.ReadFile(path)
		require.NoErrorf(t, err, "reading %s", path)
		rel := strings.TrimPrefix(path, root+string(filepath.Separator))
		assert.NotContainsf(t, stripJSComments(string(data)), dangerouslySetInnerHTMLIdentifier,
			"%s must never use %s — I20 forbids comment bodies (or anything else) reaching the DOM as raw "+
				"HTML; render through the shared row component's Markdown-to-React-elements path instead",
			rel, dangerouslySetInnerHTMLIdentifier)
	}
}
