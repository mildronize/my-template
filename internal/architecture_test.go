// Package internal contains the import-graph test that enforces
// .chief/_rules/_standard/ARCHITECTURE.md — see that document for the
// three dependency rules and why each exists. This test runs against the
// tree as it is right now, so it activates automatically (and fails loud)
// the moment a later change violates a rule, instead of surfacing only
// once the milestone is otherwise "done".
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

// handlerFileRe and repoFileRe are the filename patterns
// ARCHITECTURE.md calls "the contract, not an implementation detail":
// exactly handler.go/repo.go, or anything ending in _handler.go/_repo.go.
// The optional trailing "_test" accounts for Go's own mandatory _test.go
// suffix — a handler-role file's test (handler_test.go,
// middleware_handler_test.go, ...) legitimately imports gin too (e.g. to
// drive a real gin.Engine with httptest), and without this the rule would
// otherwise forbid the very test files this task's integration tests need
// to write, which isn't what ARCHITECTURE.md's rule 1 is protecting
// against (untestable service-layer code accreting gin.Context params) —
// that's a production-code concern, not a test-file one.
var (
	handlerFileRe = regexp.MustCompile(`^(handler|.+_handler)(_test)?\.go$`)
	repoFileRe    = regexp.MustCompile(`^(repo|.+_repo)(_test)?\.go$`)
)

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
// positive on, e.g., service.go merely sharing a package with handler.go.
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

// TestArchitecture_DomainFileImportRules enforces ARCHITECTURE.md rules 1
// and 2 across every .go file directly inside internal/todo and
// internal/identity: only a handler-role file may import gin, and only a
// repo-role file may import the sqlc-generated package. Both domain
// directories are still empty of real files as of this task (task-1) —
// this test is written now precisely so it starts enforcing the instant
// task-2/task-3 add handler.go/service.go/repo.go, rather than being
// added retroactively once a violation already landed.
func TestArchitecture_DomainFileImportRules(t *testing.T) {
	root := repoRoot(t)
	module := modulePath(t, root)
	sqlcImportPath := module + "/" + sqlcPackageImportPath

	domainDirs := []string{
		filepath.Join(root, "internal", "todo"),
		filepath.Join(root, "internal", "identity"),
	}

	for _, dir := range domainDirs {
		entries, err := os.ReadDir(dir)
		require.NoErrorf(t, err, "reading %s", dir)

		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") {
				continue
			}

			path := filepath.Join(dir, name)
			imports, err := fileImports(path)
			require.NoErrorf(t, err, "parsing imports of %s", path)

			isHandler := handlerFileRe.MatchString(name)
			isRepo := repoFileRe.MatchString(name)

			for _, imp := range imports {
				if imp == ginImportPath && !isHandler {
					t.Errorf(
						"%s: imports %q but its name doesn't match handler.go/*_handler.go — "+
							"only handler-role files may import gin (ARCHITECTURE.md rule 1)",
						path, ginImportPath,
					)
				}
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
// this test needs.
type goListPackage struct {
	ImportPath string   `json:"ImportPath"`
	Imports    []string `json:"Imports"`
}

// TestArchitecture_PlatformNeverImportsDomain enforces ARCHITECTURE.md
// rule 3: internal/platform must never import internal/todo or
// internal/identity. Unlike rules 1/2, this is a clean package-level
// check — internal/platform is a distinct package with its own import
// list, so `go list -json` is sufficient here.
func TestArchitecture_PlatformNeverImportsDomain(t *testing.T) {
	root := repoRoot(t)
	module := modulePath(t, root)

	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = root
	out, err := cmd.Output()
	require.NoError(t, err, "go list -json ./...")

	forbidden := map[string]bool{
		module + "/internal/todo":     true,
		module + "/internal/identity": true,
	}
	platformPkg := module + "/internal/platform"

	dec := json.NewDecoder(strings.NewReader(string(out)))
	found := false
	for dec.More() {
		var pkg goListPackage
		require.NoError(t, dec.Decode(&pkg), "decoding `go list -json ./...` output")
		if pkg.ImportPath != platformPkg {
			continue
		}
		found = true

		for _, imp := range pkg.Imports {
			if forbidden[imp] {
				t.Errorf(
					"internal/platform imports %q — platform must never import a domain module (ARCHITECTURE.md rule 3)",
					imp,
				)
			}
		}
	}

	if !found {
		t.Fatalf("go list -json ./... did not report a package at %s", platformPkg)
	}
}
