package healthcheck_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/gitconstants"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestHTTPListeners_TimeoutsEnforced verifies REQ-CEF-R2-SEC-HTTP-TIMEOUTS and CRIT-CEF-R2-SEC-HTTP-TIMEOUTS-A:
// - Direct http.ListenAndServe(...) calls are forbidden across production code (pkg/, cmd/, internal/).
func TestHTTPListeners_TimeoutsEnforced(t *testing.T) {
	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}

	root := wd
	for {
		if _, err := fileutil.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatalf("could not find project root containing go.mod from %s", wd)
		}
		root = parent
	}

	cmd := testkit.ManagedCommand(t, t.Context(), gitconstants.BinaryGit, "-C", root, "grep", "-l", "-I", "--", "ListenAndServe", "cmd", "pkg", "internal")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return // no matches
		}
		t.Fatalf("git grep ListenAndServe: %v", err)
	}

	fset := token.NewFileSet()
	for _, rel := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		rel = strings.TrimSpace(rel)
		if rel == "" || strings.HasSuffix(rel, "_test.go") {
			continue
		}
		path := filepath.Join(root, rel)
		src, err := fileutil.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		fileNode, err := parser.ParseFile(fset, path, src, parser.AllErrors)
		if err != nil {
			t.Errorf("failed to parse %s: %v", path, err)
			continue
		}
		ast.Inspect(fileNode, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkgIdent, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if pkgIdent.Name == "http" && sel.Sel.Name == "ListenAndServe" {
				pos := fset.Position(call.Pos())
				t.Errorf("forbidden direct http.ListenAndServe call found at %s:%d (must use http.Server with timeouts)", path, pos.Line)
			}
			return true
		})
	}
}
