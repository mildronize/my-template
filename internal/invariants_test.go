package internal

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testFuncNameRe matches a top-level Go test function declaration
// (`func TestFoo(`), capturing its name.
var testFuncNameRe = regexp.MustCompile(`(?m)^func (Test\w+)\(`)

// collectTestFuncNames walks root for every *_test.go file (skipping .git
// and bin, neither of which can contain Go source) and returns every
// top-level test function name it finds, across every package — this
// test doesn't care which package or task a given TestI<N>_ test lives
// in, only that it exists somewhere (GOAL.md Done-when 12: "it doesn't
// matter which task's tests satisfy a given invariant, only that by the
// time this is checked, all ten are named somewhere").
func collectTestFuncNames(t *testing.T, root string) []string {
	t.Helper()

	var names []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "bin" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, m := range testFuncNameRe.FindAllStringSubmatch(string(data), -1) {
			names = append(names, m[1])
		}
		return nil
	})
	require.NoErrorf(t, err, "walking %s for _test.go files", root)
	return names
}

// TestDoneWhen12_EveryInvariantHasANamedTest is GOAL.md Done-when 12's own
// check, not just an instance of the convention it enforces: every
// invariant _contract/INVARIANTS.md numbers (I1-I10) must have at least
// one test whose name references it — checked by grepping test names for
// the `TestI<N>_...` convention task-2.md established and this task
// (task-3) continues (`TestI3_...`, `TestI4_...`).
//
// task-2 supplied I1, I2, I5-I10; task-3 supplies I3, I4 (todos is the
// last new table this milestone adds — there is no task after this one
// that could still be missing an invariant test), so this is the first
// point in the plan all ten should be present. This test is what
// confirms that fact rather than assuming it — it fails loudly, naming
// exactly which invariant has no test, if any one of I1-I10 is missing a
// TestI<N>_-prefixed test anywhere under this module.
func TestDoneWhen12_EveryInvariantHasANamedTest(t *testing.T) {
	root := repoRoot(t)
	names := collectTestFuncNames(t, root)

	for n := 1; n <= 10; n++ {
		prefix := fmt.Sprintf("TestI%d_", n)
		found := false
		for _, name := range names {
			if strings.HasPrefix(name, prefix) {
				found = true
				break
			}
		}
		assert.Truef(t, found,
			"no test named %s<something> found anywhere under %s — "+
				"_contract/INVARIANTS.md's I%d has no test referencing it (GOAL.md Done-when 12)",
			prefix, root, n)
	}
}
