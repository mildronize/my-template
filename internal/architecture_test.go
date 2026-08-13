// Package internal contains the import-graph test that enforces
// .chief/_rules/_standard/ARCHITECTURE.md — see that document for the
// five dependency rules this milestone's restructure introduced (up from
// three in milestone-1, when domain and transport were still fused
// together) and why each exists. This test runs against the tree as it is
// right now, so it activates automatically (and fails loud) the moment a
// later change violates a rule, instead of surfacing only once the
// milestone is otherwise "done".
package internal

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ginImportPath and sqlcPackageImportPath are the two imports rules 1 and
// 2 restrict. sqlcPackageImportPath matches sqlc.yaml's configured output
// package (internal/db) — update this alongside sqlc.yaml if that ever
// moves.
const (
	ginImportPath         = "github.com/gin-gonic/gin"
	sqlcPackageImportPath = "internal/db"
)

// repoFileRe is the filename pattern ARCHITECTURE.md's rule 2 calls "the
// contract, not an implementation detail": exactly repo.go, or anything
// ending in _repo.go. The optional trailing "_test" accounts for Go's own
// mandatory _test.go suffix — a repo-role file's test legitimately
// imports the sqlc package too (e.g. to drive it against a real
// database).
//
// Unlike milestone-1, there is no equivalent handlerFileRe any more: rule
// 1 is now a flat "no domain file imports gin" check with no allowlist,
// because a domain module holds no handler-role file at all
// post-restructure — every transport-facing file lives in
// internal/transport/* instead (ARCHITECTURE.md, "Why transport is not
// inside a domain module anymore").
var repoFileRe = regexp.MustCompile(`^(repo|.+_repo)(_test)?\.go$`)

// repoRoot resolves the module root (the parent of the internal/
// directory this test file lives in) relative to this file's own path, so
// the test works regardless of the directory `go test` is invoked from.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine architecture_test.go's own location")
	}
	return filepath.Dir(filepath.Dir(thisFile))
}

// domainModuleNames discovers every domain module by listing
// internal/domain/'s own subdirectories — every one of them counts, full
// stop, no exclusion list (ARCHITECTURE.md: "every directory under
// internal/domain/ counts as a domain module"). This is simpler than
// milestone-1's version, which had to enumerate all of internal/ and
// exclude infrastructure directories (api, db, dbquery, platform) by
// name: the domain/transport split means domain discovery now only ever
// looks somewhere infrastructure never lives, so there is nothing left to
// exclude. A fork that renames internal/domain/todo, adds
// internal/domain/note beside it, or removes internal/domain/todo
// entirely gets correct coverage automatically (task-6.md, milestone-1: a
// hardcoded {internal/todo, internal/identity} list gave a fork that
// added a third domain module zero import-rule coverage on it — the
// guardrail failing exactly the case this template exists to prove out).
//
// internal/invariants_test.go's TestDoneWhen12_EveryInvariantHasANamedTest
// calls this exact function for its own per-domain-module invariant check
// (I3/I4) — one enumeration, not two that could drift (task-1.md).
func domainModuleNames(t *testing.T, root string) []string {
	t.Helper()
	domainDir := filepath.Join(root, "internal", "domain")
	entries, err := os.ReadDir(domainDir)
	require.NoErrorf(t, err, "reading %s", domainDir)

	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	require.NotEmptyf(t, names,
		"no domain modules found under %s — expected at least one directory", domainDir)
	return names
}

// transportSurfaceNames discovers every transport surface the same
// filesystem-derived way domainModuleNames discovers domain modules —
// every subdirectory of internal/transport/ counts, no hardcoded
// {"publicapi", "bff"} list (ARCHITECTURE.md: "The same principle now
// also applies to transport surfaces"). Only internal/transport/publicapi
// exists as of this task; internal/transport/bff arrives in task-4 and
// needs no change here to be picked up.
func transportSurfaceNames(t *testing.T, root string) []string {
	t.Helper()
	transportDir := filepath.Join(root, "internal", "transport")
	entries, err := os.ReadDir(transportDir)
	require.NoErrorf(t, err, "reading %s", transportDir)

	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	require.NotEmptyf(t, names,
		"no transport surfaces found under %s — expected at least one directory", transportDir)
	return names
}

// modulePath returns this repo's module path (e.g.
// github.com/mildronize/my-template) via `go list -m`, rather than
// hardcoding it, so the test keeps working after a fork renames the
// module (docs/GETTING-STARTED.md).
func modulePath(t *testing.T, root string) string {
	t.Helper()
	cmd := exec.Command("go", "list", "-m")
	cmd.Dir = root
	out, err := cmd.Output()
	require.NoError(t, err, "go list -m")
	return strings.TrimSpace(string(out))
}

// fileImports returns the import paths a single Go source file imports,
// read from that file's own import block via go/parser — not `go list`,
// which reports imports at the whole-package level and would false-
// positive on, e.g., service.go merely sharing a package with a sibling
// file.
func fileImports(path string) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}

	imports := make([]string, 0, len(f.Imports))
	for _, imp := range f.Imports {
		unquoted, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return nil, err
		}
		imports = append(imports, unquoted)
	}
	return imports, nil
}

// filesIn returns the .go files directly inside dir (non-recursive — no
// domain module or internal/identity currently has subpackages of its
// own, matching milestone-1's same non-recursive assumption).
func filesIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoErrorf(t, err, "reading %s", dir)

	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	return files
}

// TestArchitecture_DomainFilesNeverImportGin enforces rule 1: no .go file
// directly inside any domain module (domainModuleNames, filesystem-
// derived) may import gin — a flat check with no allowlist, because a
// domain module holds no transport-facing file post-restructure. Every
// transport-facing file lives in internal/transport/* instead.
func TestArchitecture_DomainFilesNeverImportGin(t *testing.T) {
	root := repoRoot(t)

	for _, name := range domainModuleNames(t, root) {
		dir := filepath.Join(root, "internal", "domain", name)

		for _, path := range filesIn(t, dir) {
			imports, err := fileImports(path)
			require.NoErrorf(t, err, "parsing imports of %s", path)

			for _, imp := range imports {
				if imp == ginImportPath {
					t.Errorf(
						"%s: imports %q — a domain module file may never import gin, no exceptions "+
							"(ARCHITECTURE.md rule 1; transport-facing code belongs in internal/transport/*)",
						path, ginImportPath,
					)
				}
			}
		}
	}
}

// TestArchitecture_OnlyRepoFilesImportSqlc enforces rule 2: only a
// repo-role file (repo.go/*_repo.go, `(_test)?` suffix allowed) may
// import the sqlc-generated package, inside every domain module AND
// internal/identity — unchanged allowlist pattern from milestone-1, now
// spanning two categories of directory instead of one, since identity
// keeps its own repo.go outside internal/domain/ (ARCHITECTURE.md, "Why
// identity is not under domain/").
func TestArchitecture_OnlyRepoFilesImportSqlc(t *testing.T) {
	root := repoRoot(t)
	module := modulePath(t, root)
	sqlcImportPath := module + "/" + sqlcPackageImportPath

	var checkDirs []string
	for _, name := range domainModuleNames(t, root) {
		checkDirs = append(checkDirs, filepath.Join(root, "internal", "domain", name))
	}
	checkDirs = append(checkDirs, filepath.Join(root, "internal", "identity"))

	for _, dir := range checkDirs {
		for _, path := range filesIn(t, dir) {
			imports, err := fileImports(path)
			require.NoErrorf(t, err, "parsing imports of %s", path)

			isRepo := repoFileRe.MatchString(filepath.Base(path))
			for _, imp := range imports {
				if imp == sqlcImportPath && !isRepo {
					t.Errorf(
						"%s: imports %q but its name doesn't match repo.go/*_repo.go — "+
							"only repo-role files may import the sqlc-generated package (ARCHITECTURE.md rule 2)",
						path, sqlcImportPath,
					)
				}
			}
		}
	}
}

// goListPackage is the subset of `go list -json`'s per-package output
// rules 3, 4, and 5 need.
type goListPackage struct {
	ImportPath string   `json:"ImportPath"`
	Imports    []string `json:"Imports"`
}

// goListPackages runs `go list -json ./...` once and returns every
// package keyed by import path. Rules 3, 4, and 5 are all clean
// package-level checks (ARCHITECTURE.md "Enforcement": "domain-module and
// transport-surface names are each their own Go package with their own
// import list, so inspecting Imports per package is sufficient"), so they
// share this one invocation instead of shelling out three separate times.
//
// -e (not plain `go list -json ./...`) matters here for the same reason
// the Makefile's GO_PKGS does: web/ sits inside this module's root, so an
// npm dependency's own .go file (e.g.
// web/node_modules/flatted/golang/pkg/flatted) is part of `./...` too.
// Without -e, a single such file with an import `go list` can't resolve
// turns the whole `go list -json ./...` invocation into a nonzero exit —
// require.NoError below would then fail every rule-3/4/5 test for a
// reason that has nothing to do with this repo's own import graph. -e
// makes go list tolerate per-package errors instead of aborting, and the
// node_modules filter below then drops that package (and any error it
// carries) before the rules ever see it — the same "npm install must
// never affect Go tooling" guarantee the Makefile enforces for
// build/vet/test/fmt-check.
func goListPackages(t *testing.T, root string) map[string]goListPackage {
	t.Helper()
	cmd := exec.Command("go", "list", "-e", "-json", "./...")
	cmd.Dir = root
	out, err := cmd.Output()
	require.NoError(t, err, "go list -e -json ./...")

	pkgs := make(map[string]goListPackage)
	dec := json.NewDecoder(strings.NewReader(string(out)))
	for dec.More() {
		var pkg goListPackage
		require.NoError(t, dec.Decode(&pkg), "decoding `go list -e -json ./...` output")
		if strings.Contains(pkg.ImportPath, "/node_modules/") {
			continue
		}
		pkgs[pkg.ImportPath] = pkg
	}
	return pkgs
}

// TestArchitecture_DomainModulesNeverImportSiblingDomains enforces rule 3:
// a domain module may never import another domain module — what actually
// makes "fork = delete one domain, add another" true (ARCHITECTURE.md).
// domainModuleNames is filesystem-derived, so with exactly one domain
// module (todo) as of this task, there is nothing to violate yet; this
// rule is only proven real by a scratch-clone attack with a fake second
// domain module (see task-1's builder report), not by this test staying
// green on today's tree.
func TestArchitecture_DomainModulesNeverImportSiblingDomains(t *testing.T) {
	root := repoRoot(t)
	module := modulePath(t, root)
	pkgs := goListPackages(t, root)

	modules := domainModuleNames(t, root)
	importPathOf := make(map[string]string, len(modules)) // module name -> its own import path
	for _, name := range modules {
		importPathOf[name] = module + "/internal/domain/" + name
	}

	for _, name := range modules {
		pkg, found := pkgs[importPathOf[name]]
		if !found {
			continue // no non-test .go files in this module's own package yet — nothing to check
		}

		for _, imp := range pkg.Imports {
			for otherName, otherPath := range importPathOf {
				if otherName == name {
					continue
				}
				if imp == otherPath {
					t.Errorf(
						"internal/domain/%s imports %q — a domain module must never import a sibling "+
							"domain module (ARCHITECTURE.md rule 3)",
						name, otherPath,
					)
				}
			}
		}
	}
}

// TestArchitecture_DomainAndIdentityNeverImportTransport enforces rule 4:
// no domain module and internal/identity may ever import
// internal/transport/* — dependencies point one way, transport depends on
// domain/identity, never the reverse (ARCHITECTURE.md: "business logic
// could grow a dependency on a specific transport surface's types").
func TestArchitecture_DomainAndIdentityNeverImportTransport(t *testing.T) {
	root := repoRoot(t)
	module := modulePath(t, root)
	pkgs := goListPackages(t, root)

	forbidden := make(map[string]bool)
	for _, name := range transportSurfaceNames(t, root) {
		forbidden[module+"/internal/transport/"+name] = true
	}

	checkPkgs := map[string]string{ // import path -> friendly label for the failure message
		module + "/internal/identity": "internal/identity",
	}
	for _, name := range domainModuleNames(t, root) {
		checkPkgs[module+"/internal/domain/"+name] = "internal/domain/" + name
	}

	for pkgPath, label := range checkPkgs {
		pkg, found := pkgs[pkgPath]
		if !found {
			continue
		}
		for _, imp := range pkg.Imports {
			if forbidden[imp] {
				t.Errorf(
					"%s imports %q — a domain module or internal/identity must never import a "+
						"transport package (ARCHITECTURE.md rule 4)",
					label, imp,
				)
			}
		}
	}
}

// TestArchitecture_PlatformNeverImportsDomainIdentityOrTransport enforces
// rule 5: internal/platform must never import a domain module,
// internal/identity, or any internal/transport/* surface — platform stays
// the layer safe to leave untouched on a fork only if it never
// accumulates branches specific to what's above it (ARCHITECTURE.md).
func TestArchitecture_PlatformNeverImportsDomainIdentityOrTransport(t *testing.T) {
	root := repoRoot(t)
	module := modulePath(t, root)
	pkgs := goListPackages(t, root)

	forbidden := map[string]bool{
		module + "/internal/identity": true,
	}
	for _, name := range domainModuleNames(t, root) {
		forbidden[module+"/internal/domain/"+name] = true
	}
	for _, name := range transportSurfaceNames(t, root) {
		forbidden[module+"/internal/transport/"+name] = true
	}

	platformPkg := module + "/internal/platform"
	pkg, found := pkgs[platformPkg]
	require.Truef(t, found, "go list -json ./... did not report a package at %s", platformPkg)

	for _, imp := range pkg.Imports {
		if forbidden[imp] {
			t.Errorf(
				"internal/platform imports %q — platform must never import a domain module, "+
					"internal/identity, or a transport package (ARCHITECTURE.md rule 5)",
				imp,
			)
		}
	}
}
