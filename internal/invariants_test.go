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
// heading line matched by invariantHeadingRe. Three forms are
// recognized:
//
//   - "global" — the check greps the whole repo for a TestI<N>_ test
//     (the only behavior this test had before task-7).
//   - "per-domain-module" — the check requires a TestI<N>_ test inside
//     *every* package I3/I4 actually apply to
//     (perDomainModuleScopePackages, below — a small, explicit,
//     hand-maintained list, deliberately not domainModuleNames()). See
//     _contract/INVARIANTS.md's own explanation of the tag, right above
//     I1, for why this exists: a single domain module's test (e.g.
//     internal/identity's TestI3_..._Keys) used to satisfy a global grep
//     for every other domain module forever, so a forked module could
//     ship zero ownership-scoping tests of its own and stay green
//     (task-7, Clara's second blind fork test).
//   - "domain:<name>" — the check requires a TestI<N>_ test inside the
//     one specific package <name> resolves to (domainScopePackageNames,
//     below — an explicit name→path mapping, not a naming convention).
//     Added by the scope-tags fix-round (sequenced before task-6) to stop
//     conflating "applies to every domain module" (a real coverage
//     sweep — I3/I4's actual shape) with "applies to exactly one
//     specific place" (I15-I19, I21's actual shape). The old
//     per-domain-module tag only "worked" for I15-I19/I21 by coincidence,
//     because domainModuleNames() had exactly one member (todo) — the
//     moment I21 (an internal/identity-only invariant) got tagged
//     per-domain-module, a correctly-placed TestI21_ in internal/identity
//     would not satisfy the check, and a wrong-location stub under
//     internal/domain/todo would. <name> is free-form (letters, digits,
//     "_", "-") purely so INVARIANTS.md can name a domain without this
//     regex needing to change; the mapping from <name> to an actual
//     package is intentionally NOT derived from it — see
//     domainScopePackageNames.
var invariantScopeRe = regexp.MustCompile("`scope: (global|per-domain-module|domain:[A-Za-z0-9_-]+)`")

const (
	scopeGlobal          = "global"
	scopePerDomainModule = "per-domain-module"
	// scopeDomainPrefix is the "domain:" half of a "domain:<name>" scope
	// tag — strings.TrimPrefix(scope, scopeDomainPrefix) recovers <name>.
	scopeDomainPrefix = "domain:"
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
				"heading %q in %s has no `scope: global` / `scope: per-domain-module` / `scope: domain:<name>` "+
					"tag — every invariant heading must declare one (see the `scope:` tag note in %s)",
				heading, path, path)
			scope := scopeMatch[1]
			require.Truef(t,
				scope == scopeGlobal || scope == scopePerDomainModule || strings.HasPrefix(scope, scopeDomainPrefix),
				"heading %q in %s declares unrecognized scope %q — must be %q, %q, or %q<name>",
				heading, path, scope, scopeGlobal, scopePerDomainModule, scopeDomainPrefix)

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

// domainScopePackageNames is the explicit, hand-maintained name→package
// mapping a `scope: domain:<name>` tag resolves through. Deliberately not
// derived from a naming convention (e.g. "domain:<name>" ->
// internal/domain/<name>) because not every name lives there:
// internal/identity is deliberately its own layer, not a domain module
// (_rules/_standard/ARCHITECTURE.md's milestone-2 decision), so
// "domain:identity" has to be wired to internal/identity by hand, the
// same as "domain:todo" is wired to internal/domain/todo.
//
// An unrecognized name must never resolve here — TestDoneWhen12 treats a
// lookup miss as a loud abort (require.Truef), not "no package to
// check," because a domain:<name> tag that silently resolved to nothing
// would make that invariant's coverage requirement a no-op that passes
// trivially — the exact failure shape I15's own floor-of-zero bug and the
// sqlc-ignores-Down measurement were both found and fixed for elsewhere
// this milestone. TestDomainScopePackageNames_UnknownNameDoesNotResolve
// below proves this by direct example, not by inspection; the full
// end-to-end abort (TestDoneWhen12 itself failing given a bogus tag in a
// real INVARIANTS.md) is attacked by hand once during review (builder
// report), the same split TestI15Floor_CanActuallyFail already
// establishes for I15's own floor check.
func domainScopePackageNames(root string) map[string]string {
	return map[string]string{
		"todo":     filepath.Join(root, "internal", "domain", "todo"),
		"identity": filepath.Join(root, "internal", "identity"),
	}
}

// TestDomainScopePackageNames_UnknownNameDoesNotResolve proves the
// property domainScopePackageNames' own doc comment states in prose: a
// name nobody wired up (a typo in INVARIANTS.md, or a deliberately bogus
// tag like a fixture's "domain:bogus-nonexistent-name") does not resolve
// to a package — it has to come back "not found" so the caller
// (TestDoneWhen12) is forced into its loud-abort branch instead of
// silently treating a typo'd scope as "nothing to check."
func TestDomainScopePackageNames_UnknownNameDoesNotResolve(t *testing.T) {
	root := repoRoot(t)
	_, ok := domainScopePackageNames(root)["bogus-nonexistent-name"]
	assert.Falsef(t, ok,
		"domain:bogus-nonexistent-name must not resolve to a package — an invariant tagged with an "+
			"unrecognized domain name has to make TestDoneWhen12 abort loudly, not silently skip its "+
			"coverage check")
}

// perDomainModuleScopePackages is the explicit, hand-maintained set of
// packages I3 (ownership scoping) and I4 (one seam reads identity)
// actually apply to — the two invariants that keep the `scope:
// per-domain-module` tag, per the scope-tags fix-round. Deliberately NOT
// domainModuleNames() (internal/architecture_test.go): that function
// answers a different question — "what counts as a domain module for
// fork-restructuring purposes" — and internal/identity correctly failing
// that question is the existing design (identity is its own layer, not a
// domain module, per ARCHITECTURE.md's milestone-2 decision), not a gap
// this list should paper over by widening domainModuleNames() itself.
//
// Hand-maintained on purpose: when a new package needs I3/I4 coverage,
// add it here explicitly, with a one-line reason, the same way
// internal/identity's own entry below carries one. Nothing mechanical can
// discover that a *non-domain* package needs I3/I4 the way it can
// discover a missing domain module (see the superset assertion below,
// TestPerDomainModuleScopeCoversEveryDomainModule) — a human has to
// notice and add the line.
func perDomainModuleScopePackages(root string) map[string]string {
	return map[string]string{
		// The todo domain module: I3's ownership-scoping and I4's
		// single-seam-identity-read properties both apply to it
		// directly.
		"todo": filepath.Join(root, "internal", "domain", "todo"),
		// internal/identity is deliberately not under internal/domain/
		// (ARCHITECTURE.md's milestone-2 decision) but owns the
		// users/api_keys tables I4 is actually about, and I3's
		// ownership-scoping applies to its own key-listing — it belongs
		// in this list even though it fails domainModuleNames().
		"identity": filepath.Join(root, "internal", "identity"),
	}
}

// TestPerDomainModuleScopeCoversEveryDomainModule asserts
// perDomainModuleScopePackages is a superset of domainModuleNames()'s own
// module names — a new domain module nobody remembered to add to the
// hand-maintained I3/I4 list has to fail this loudly (a real coverage
// gap: that module silently exempt from I3/I4), not pass silently. This
// cannot and does not try to catch the other direction — a new
// *non-domain* package (like internal/identity) needing I3/I4 coverage —
// nothing mechanical can know that in advance; see
// perDomainModuleScopePackages' own comment for what adding one requires.
func TestPerDomainModuleScopeCoversEveryDomainModule(t *testing.T) {
	root := repoRoot(t)
	explicit := perDomainModuleScopePackages(root)
	for _, module := range domainModuleNames(t, root) {
		assert.Containsf(t, explicit, module,
			"domain module %q (domainModuleNames, internal/architecture_test.go) is missing from "+
				"perDomainModuleScopePackages (internal/invariants_test.go) — a domain module must never be "+
				"silently exempt from I3/I4; add it to that map explicitly, with a one-line reason",
			module)
	}
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
// security hole Clara's second blind fork test found, extended by the
// scope-tags fix-round ahead of task-6): a `global`-scope invariant keeps
// the original behavior — a TestI<N>_ test anywhere in the repo satisfies
// it. A `per-domain-module`-scope invariant (I3, I4 only, as of the
// fix-round) instead requires a TestI<N>_ test inside *every* package
// perDomainModuleScopePackages names — a small, explicit, hand-maintained
// list (asserted a superset of domainModuleNames() by
// TestPerDomainModuleScopeCoversEveryDomainModule, above), deliberately
// not domainModuleNames() itself. Without the per-domain-module
// distinction, one domain module's test (e.g. internal/identity's
// pre-existing TestI3_..._Keys) silently satisfied I3's requirement for
// every other domain module forever — proven by Clara's agent renaming
// every TestI3_ test out of its new internal/bookmark module and watching
// the suite stay green anyway.
//
// A `domain:<name>`-scope invariant (I15-I19, I21, since the fix-round)
// requires a TestI<N>_ test inside the one specific package <name>
// resolves to (domainScopePackageNames, above) — not a coverage sweep
// like per-domain-module, an address. This exists because
// per-domain-module originally conflated the two: I15-I19/I21 each
// belong to exactly one place (I15-I19 to internal/domain/todo, I21 to
// internal/identity), and tagging them per-domain-module only "worked"
// because domainModuleNames() had exactly one member (todo) — the moment
// I21 needed a test in internal/identity specifically, per-domain-module
// would have accepted a wrong-location stub under internal/domain/todo
// instead of demanding the real thing.
//
// task-2 supplied I1, I2, I5-I10; task-3 supplied I3, I4; the fix-round
// re-scoped I15-I19 and I21 from per-domain-module to domain:<name> so
// this check demands their tests in the right package instead of the
// wrong one. This test confirms the full required set has a test rather
// than assuming it — it fails loudly, naming exactly which invariant
// (and, for per-domain-module/domain:<name> ones, which package) has no
// test, and aborts outright if a domain:<name> tag names a package this
// file doesn't know about (domainScopePackageNames' own doc comment).
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

	perModulePkgs := perDomainModuleScopePackages(root)
	perModuleNames := make(map[string][]string, len(perModulePkgs))
	for module, dir := range perModulePkgs {
		perModuleNames[module] = collectTestFuncNames(t, dir)
	}

	domainPkgs := domainScopePackageNames(root)
	domainScopeNames := make(map[string][]string, len(domainPkgs))

	for _, inv := range required {
		prefix := fmt.Sprintf("TestI%d_", inv.number)

		switch {
		case inv.scope == scopeGlobal:
			assert.Truef(t, hasTestWithPrefix(globalNames, prefix),
				"no test named %s<something> found anywhere under %s — "+
					"_contract/INVARIANTS.md's I%d (scope: global) has no test referencing it (GOAL.md Done-when 12)",
				prefix, root, inv.number)

		case inv.scope == scopePerDomainModule:
			for module, dir := range perModulePkgs {
				assert.Truef(t, hasTestWithPrefix(perModuleNames[module], prefix),
					"no test named %s<something> found inside %s's own package — "+
						"_contract/INVARIANTS.md's I%d (scope: per-domain-module) requires a dedicated test "+
						"in every package I3/I4 apply to (perDomainModuleScopePackages), not just somewhere in "+
						"the repo (GOAL.md Done-when 12; task-7)",
					prefix, dir, inv.number)
			}

		case strings.HasPrefix(inv.scope, scopeDomainPrefix):
			name := strings.TrimPrefix(inv.scope, scopeDomainPrefix)
			dir, ok := domainPkgs[name]
			// Loud abort, not a silent "nothing to check": a domain:<name>
			// tag naming a package this file doesn't know about must never
			// let I<N>'s coverage requirement resolve to a no-op that
			// passes trivially — the same failure shape I15's own
			// floor-of-zero bug and the sqlc-ignores-Down measurement were
			// both found and fixed for elsewhere this milestone.
			require.Truef(t, ok,
				"_contract/INVARIANTS.md's I%d declares scope %q, but %q is not a known domain name — "+
					"add it to domainScopePackageNames (internal/invariants_test.go) with an explicit package "+
					"path, or fix the typo in INVARIANTS.md (GOAL.md Done-when 12)",
				inv.number, inv.scope, name)

			if _, cached := domainScopeNames[name]; !cached {
				domainScopeNames[name] = collectTestFuncNames(t, dir)
			}
			assert.Truef(t, hasTestWithPrefix(domainScopeNames[name], prefix),
				"no test named %s<something> found inside %s (scope: domain:%s) — "+
					"_contract/INVARIANTS.md's I%d requires a dedicated test in that specific package, not "+
					"just somewhere in the repo (GOAL.md Done-when 12)",
				prefix, dir, name, inv.number)

		default:
			// requiredInvariantNumbers already validates scope against
			// the known set, so this is unreachable — kept as a loud
			// failure rather than a silent fallthrough if that ever
			// changes.
			t.Fatalf("I%d has unrecognized scope %q", inv.number, inv.scope)
		}
	}
}
