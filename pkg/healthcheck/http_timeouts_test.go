package healthcheck_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestHTTPListeners_TimeoutsEnforced verifies REQ-CEF-R2-SEC-HTTP-TIMEOUTS and CRIT-CEF-R2-SEC-HTTP-TIMEOUTS-A:
// - Direct http.ListenAndServe(...) calls are forbidden across production code (pkg/, cmd/, internal/).
// - All http.Server definitions or custom listeners configure ReadHeaderTimeout or timeouts to protect against Slowloris.
func TestHTTPListeners_TimeoutsEnforced(t *testing.T) {
	// Find project root by searching upwards for go.mod
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

	dirsToCheck := []string{
		filepath.Join(root, "cmd"),
		filepath.Join(root, "pkg"),
		filepath.Join(root, "internal"),
	}

	fset := token.NewFileSet()

	for _, dir := range dirsToCheck {
		err := filepath.Walk(dir, func(path string, info fileutil.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				if info.Name() == "vendor" || info.Name() == ".git" || info.Name() == ".zqk" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			src, err := fileutil.ReadFile(path)
			if err != nil {
				return err
			}

			fileNode, err := parser.ParseFile(fset, path, src, parser.AllErrors)
			if err != nil {
				t.Errorf("failed to parse %s: %v", path, err)
				return nil
			}

			ast.Inspect(fileNode, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				// Check for direct http.ListenAndServe(...) calls
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					if pkgIdent, ok := sel.X.(*ast.Ident); ok {
						if pkgIdent.Name == "http" && sel.Sel.Name == "ListenAndServe" {
							pos := fset.Position(call.Pos())
							t.Errorf("forbidden direct http.ListenAndServe call found at %s:%d (must use http.Server with timeouts)", path, pos.Line)
						}
					}
				}
				return true
			})

			return nil
		})

		if err != nil {
			t.Fatalf("filepath.Walk(%s): %v", dir, err)
		}
	}
}
