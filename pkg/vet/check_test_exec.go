package vet

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// CheckTestSubprocessHygiene scans Go test source files (*_test.go) for unmanaged exec.Command
// invocations that lack context deadlines or process group lifecycle management.
// (TDE-F-TST-ORPHANED-PROCESS-LEAK / CRIT-TST-SUBPROCESS-STATIC-GUARD).
func CheckTestSubprocessHygiene(root string, cfg *GatesConfig) ([]Finding, error) {
	var findings []Finding
	fset := token.NewFileSet()

	dirs := []string{"pkg", "cmd"}
	if cfg != nil && len(cfg.Hygiene.GoScanDirs) > 0 {
		dirs = cfg.Hygiene.GoScanDirs
	}

	for _, dir := range dirs {
		absDir := filepath.Join(root, dir)
		if _, err := os.Stat(absDir); err != nil {
			continue
		}

		err := filepath.Walk(absDir, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil || info.IsDir() {
				return walkErr
			}
			if !strings.HasSuffix(info.Name(), "_test.go") {
				return nil
			}

			rel, _ := filepath.Rel(root, path)
			src, readErr := fileutil.ReadFile(path)
			if readErr != nil {
				return nil
			}

			node, parseErr := parser.ParseFile(fset, rel, src, parser.ParseComments)
			if parseErr != nil {
				return nil
			}

			ast.Inspect(node, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				// Check for exec.Command(...) calls
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}

				ident, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}

				if (ident.Name == "exec" || ident.Name == "__exec") && sel.Sel.Name == "Command" {
					pos := fset.Position(call.Pos())
					findings = append(findings, Finding{
						CheckID:  "hygiene/unmanaged_test_exec",
						Suite:    "hygiene",
						Severity: SeverityWarn,
						File:     rel,
						Line:     pos.Line,
						Message: fmt.Sprintf("unmanaged %s.%s invocation in test file; prefer testkit.ManagedCommand to guarantee process group isolation and cleanup",
							ident.Name, sel.Sel.Name),
					})
				}
				return true
			})

			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return findings, nil
}
