package internal

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testFuncNameRe matches a top-level Go test function declaration
// (`func TestFoo(`), capturing its name.
var testFuncNameRe = regexp.MustCompile(`(?m)^func (Test\w+)\(`)

// invariantHeadingRe matches an INVARIANTS.md heading line of the form
// "**I1 — Some Title.** `scope: global`", capturing the invariant number
// and the whole rest of that line (so the scope tag on the same line can
// be pulled out of it separately, by invariantScopeRe below). The heading
// format is documented as consistent (task-5.md) — every invariant starts
// a line this way, so this is the one place the required I<N> set is
// derived from, instead of a hardcoded range that goes stale the moment
// someone adds I11 without touching this test.
var invariantHeadingRe = regexp.MustCompile(`(?m)^\*\*I(\d+) —.*$`)

// invariantScopeRe pulls the `scope: <value>` marker out of a single
// heading line matched by invariantHeadingRe. Two values are recognized:
// "global" (the check greps the whole repo for a TestI<N>_ test — the
// only behavior this test had before task-7) and "per-domain-module" (the
// check requires a TestI<N>_ test inside *every* domain module's own
// package). See _contract/INVARIANTS.md's own explanation of the tag,
// right above I1, for why per-domain-module exists: a single domain
// module's test (e.g. internal/identity's TestI3_..._Keys) used to
// satisfy a global grep for every other domain module forever, so a
// forked module could ship zero ownership-scoping tests of its own and
// stay green (task-7, Clara's second blind fork test).
var invariantScopeRe = regexp.MustCompile("`scope: (global|per-domain-module)`")

const (
	scopeGlobal          = "global"
	scopePerDomainModule = "per-domain-module"
)

// promotedInvariantsPath is where a project-wide INVARIANTS.md lives once
// promoted out of a single milestone (milestone-2's own
// _rules/_contract/INVARIANTS.md is the first instance of this) — see
// _rules/_standard/ARCHITECTURE.md's "Contract promotion" note. When this
// file exists it is the *only* authority; a milestone's own _contract/
// copy becomes history the moment its content is promoted, and must not
// keep contributing to this check.
const promotedInvariantsPath = ".chief/_rules/_contract/INVARIANTS.md"

// findInvariantsFiles returns the file(s) that define the required
// invariant set. If a promoted, project-wide INVARIANTS.md exists
// (promotedInvariantsPath), it is the *only* input — not unioned with any
// milestone's own copy, even a still-live one, because a promoted
// document is supposed to be the one place this is defined. Without this
// rule, a superseded milestone copy kept in-tree for history (this repo's
// own convention — see ARCHITECTURE.md) would silently keep driving a
// live check: editing the historical copy would change what's required,
// and an invariant a later milestone deliberately retired would stay
// demanded forever because an old copy still names it (found by Clara,
// milestone-2, reviewing this exact promotion).
//
// Only when no promoted file exists does this fall back to globbing every
// INVARIANTS.md under <root>/.chief/ and unioning them — this preserves
// the original (milestone-1-era) behavior for a fork that never promotes
// a contract at all, where "one file per milestone, no promotion yet" is
// still the actual shape.
func findInvariantsFiles(t *testing.T, root string) []string {
	t.Helper()

	promoted := filepath.Join(root, promotedInvariantsPath)
	if _, err := os.Stat(promoted); err == nil {
		return []string{promoted}
	}

	var paths []string
	err := filepath.WalkDir(filepath.Join(root, ".chief"), func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() && d.Name() == "INVARIANTS.md" {
			paths = append(paths, path)
		}
		return nil
	})
	require.NoErrorf(t, err, "walking %s for INVARIANTS.md", filepath.Join(root, ".chief"))
	require.NotEmptyf(t, paths, "no INVARIANTS.md found anywhere under %s — can't derive the required invariant set", filepath.Join(root, ".chief"))
	return paths
}

// requiredInvariant is one "**I<N> — ...** `scope: ...`" heading parsed out
// of an INVARIANTS.md file.
type requiredInvariant struct {
	number int
	scope  string
}

// requiredInvariantNumbers parses every INVARIANTS.md found under
// <root>/.chief/ (findInvariantsFiles) for every `**I<N> —` heading and
// returns each invariant's number together with its `scope:` tag. Fails
// the test outright if no headings are found at all across every file (an
// empty result would silently turn Done-when 12's check into a no-op
// instead of a real one), or if a heading is missing its `scope:` tag, or
// has one this test doesn't recognize — an unrecognized scope must fail
// loud, not silently fall back to the weaker "global" behavior, per
// ARCHITECTURE.md's own allowlist reasoning (task-5.md).
func requiredInvariantNumbers(t *testing.T, root string) []requiredInvariant {
	t.Helper()

	var invariants []requiredInvariant
	for _, path := range findInvariantsFiles(t, root) {
		data, err := os.ReadFile(path)
		require.NoErrorf(t, err, "reading %s to derive the required invariant set", path)

		matches := invariantHeadingRe.FindAllString(string(data), -1)
		for _, heading := range matches {
			numMatch := invariantHeadingRe.FindStringSubmatch(heading)
			n, convErr := strconv.Atoi(numMatch[1])
			require.NoErrorf(t, convErr, "parsing invariant number from heading %q in %s", heading, path)

			scopeMatch := invariantScopeRe.FindStringSubmatch(heading)
			require.NotNilf(t, scopeMatch,
				"heading %q in %s has no `scope: global` / `scope: per-domain-module` tag — "+
					"every invariant heading must declare one (see the `scope:` tag note in %s)",
				heading, path, path)
			scope := scopeMatch[1]
			require.Truef(t, scope == scopeGlobal || scope == scopePerDomainModule,
				"heading %q in %s declares unrecognized scope %q — must be %q or %q",
				heading, path, scope, scopeGlobal, scopePerDomainModule)

			invariants = append(invariants, requiredInvariant{number: n, scope: scope})
		}
	}
	require.NotEmptyf(t, invariants, "no \"**I<N> —\" headings found in any INVARIANTS.md under %s — can't derive the required invariant set", filepath.Join(root, ".chief"))
	return invariants
}

// collectTestFuncNames walks dir for every *_test.go file (skipping .git
// and bin, neither of which can contain Go source) and returns every
// top-level test function name it finds. Called two ways: once against
// the whole repo root, for global-scope invariants that don't care which
// package a TestI<N>_ test lives in (GOAL.md Done-when 12: "it doesn't
// matter which task's tests satisfy a given invariant, only that by the
// time this is checked, all ten are named somewhere"); once per domain
// module directory, for per-domain-module-scope invariants, where it
// matters a great deal which package the test lives in — that's the
// whole fix task-7 made.
func collectTestFuncNames(t *testing.T, dir string) []string {
	t.Helper()

	var names []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
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
	require.NoErrorf(t, err, "walking %s for _test.go files", dir)
	return names
}

// hasTestWithPrefix reports whether any name in names starts with prefix.
func hasTestWithPrefix(names []string, prefix string) bool {
	for _, name := range names {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// TestDoneWhen12_EveryInvariantHasANamedTest is GOAL.md Done-when 12's own
// check, not just an instance of the convention it enforces: every
// invariant _contract/INVARIANTS.md numbers must have at least one test
// whose name references it — checked by grepping test names for the
// `TestI<N>_...` convention task-2.md established and task-3 continued
// (`TestI3_...`, `TestI4_...`).
//
// The required set of invariant numbers is parsed from INVARIANTS.md
// itself (requiredInvariantNumbers), not hardcoded as a range — a fork
// that adds I11 to INVARIANTS.md without adding a TestI11_ test anywhere
// gets a failing check that names the exact gap, instead of a check that
// stays green forever because it only ever knew about I1-I10 (task-5.md).
//
// Each invariant also carries a `scope:` tag (task-7, fixing a real
// security hole Clara's second blind fork test found): a `global`-scope
// invariant keeps the original behavior — a TestI<N>_ test anywhere in
// the repo satisfies it. A `per-domain-module`-scope invariant (I3, I4)
// instead requires a TestI<N>_ test inside *every* domain module's own
// package (domainModuleNames, internal/architecture_test.go — the same
// enumeration TestArchitecture_DomainFileImportRules uses, so the two
// tests can never disagree about what counts as a domain module).
// Without this distinction, one domain module's test (e.g.
// internal/identity's pre-existing TestI3_..._Keys) silently satisfied
// I3's requirement for every other domain module forever — proven by
// Clara's agent renaming every TestI3_ test out of its new
// internal/bookmark module and watching the suite stay green anyway.
//
// task-2 supplied I1, I2, I5-I10; task-3 supplied I3, I4 (todos is the
// last new table this milestone adds — there is no task after this one
// that could still be missing an invariant test), so this is the first
// point in the plan all ten should be present. This test is what
// confirms that fact rather than assuming it — it fails loudly, naming
// exactly which invariant (and, for per-domain-module ones, which module)
// has no test.
//
// Known limitation, documented for readers in docs/GETTING-STARTED.md's
// "Invariants" section too: this only checks test *names*, not bodies. An
// empty `func TestI3_Foo(t *testing.T) {}` satisfies it. That was always
// true; it matters more now that the check is per-module, since writing
// an empty one to silence a real gap is a deliberate lie, not an
// accidental miss.
//
// Second known limitation, same section of the doc (task-9, Clara's fifth
// blind fork test): per-domain-module scope (above) requires *some*
// TestI<N>_ test in the module's package, not one per layer. This repo's
// own convention (internal/domain/todo) writes a separate
// TestI3_Repo.../TestI3_Service.../TestI3_Handler... per module — but
// nothing here requires that granularity, so renaming away only the
// repo-layer test (leaving service/handler's alone) stays invisible to
// this check: it still finds a TestI3_ match and reports the module
// covered. Deliberately not hardened into a layer-aware check: I4 (the
// other per-domain-module invariant) only ever has a repo-layer test by
// nature — it's a static check against sqlc query source, not something a
// service or handler layer would meaningfully re-test — so "require
// Repo+Service+Handler" would be wrong for I4 the moment it was applied
// uniformly, and a per-invariant table of which layers to require would
// be exactly the kind of special-cased, drift-prone mechanism this
// project has repeatedly found silent gaps behind elsewhere. A human
// reviewer checking that a module's I3 coverage actually spans repo,
// service, and handler — the same way "checks names, not bodies" already
// asks a human to read the test body — is the deliberate choice here,
// not an oversight.
func TestDoneWhen12_EveryInvariantHasANamedTest(t *testing.T) {
	root := repoRoot(t)
	required := requiredInvariantNumbers(t, root)

	globalNames := collectTestFuncNames(t, root)

	modules := domainModuleNames(t, root)
	perModuleNames := make(map[string][]string, len(modules))
	for _, module := range modules {
		// domainModuleNames (internal/architecture_test.go) enumerates
		// internal/domain/*'s own subdirectories post-restructure — join
		// against that same path, not internal/<module> (milestone-1's
		// path, before the domain/transport split moved every domain
		// module one directory deeper).
		perModuleNames[module] = collectTestFuncNames(t, filepath.Join(root, "internal", "domain", module))
	}

	for _, inv := range required {
		prefix := fmt.Sprintf("TestI%d_", inv.number)

		switch inv.scope {
		case scopeGlobal:
			assert.Truef(t, hasTestWithPrefix(globalNames, prefix),
				"no test named %s<something> found anywhere under %s — "+
					"_contract/INVARIANTS.md's I%d (scope: global) has no test referencing it (GOAL.md Done-when 12)",
				prefix, root, inv.number)

		case scopePerDomainModule:
			for _, module := range modules {
				assert.Truef(t, hasTestWithPrefix(perModuleNames[module], prefix),
					"no test named %s<something> found inside internal/domain/%s's own package — "+
						"_contract/INVARIANTS.md's I%d (scope: per-domain-module) requires a dedicated test "+
						"in every domain module, not just somewhere in the repo (GOAL.md Done-when 12; task-7)",
					prefix, module, inv.number)
			}

		default:
			// requiredInvariantNumbers already validates scope against
			// the known set, so this is unreachable — kept as a loud
			// failure rather than a silent fallthrough if that ever
			// changes.
			t.Fatalf("I%d has unrecognized scope %q", inv.number, inv.scope)
		}
	}
}
