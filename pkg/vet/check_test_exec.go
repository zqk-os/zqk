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

const msgUnmanagedTestSubprocess = "unmanaged %s.%s invocation in test file; prefer testkit.ManagedCommand to guarantee process group isolation and cleanup"

// walkTestFileCallExprs traverses test files in the configured scan dirs and passes each CallExpr to visitor.
func walkTestFileCallExprs(root string, cfg *GatesConfig, visitor func(fset *token.FileSet, rel string, call *ast.CallExpr, ident *ast.Ident, sel *ast.SelectorExpr) []Finding) ([]Finding, error) {
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

			rel, err := filepath.Rel(root, path)
			if err != nil {
				rel = path
			}
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
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				ident, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				if items := visitor(fset, rel, call, ident, sel); len(items) > 0 {
					findings = append(findings, items...)
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

// CheckTestSubprocessHygiene scans Go test source files (*_test.go) for unmanaged exec.Command
// invocations that lack context deadlines or process group lifecycle management.
// (TDE-F-TST-ORPHANED-PROCESS-LEAK / CRIT-TST-SUBPROCESS-STATIC-GUARD).
func CheckTestSubprocessHygiene(root string, cfg *GatesConfig) ([]Finding, error) {
	return walkTestFileCallExprs(root, cfg, func(fset *token.FileSet, rel string, call *ast.CallExpr, ident *ast.Ident, sel *ast.SelectorExpr) []Finding {
		if (ident.Name == "exec" || ident.Name == "__exec") && (sel.Sel.Name == "Command" || sel.Sel.Name == "CommandContext") {
			pos := fset.Position(call.Pos())
			return []Finding{{
				CheckID:  "hygiene/unmanaged_test_exec",
				Suite:    "hygiene",
				Severity: SeverityError,
				File:     rel,
				Line:     pos.Line,
				Message:  fmt.Sprintf(msgUnmanagedTestSubprocess, ident.Name, sel.Sel.Name),
			}}
		}
		if ident.Name == "os" && sel.Sel.Name == "StartProcess" {
			pos := fset.Position(call.Pos())
			return []Finding{{
				CheckID:  "hygiene/unmanaged_test_exec",
				Suite:    "hygiene",
				Severity: SeverityError,
				File:     rel,
				Line:     pos.Line,
				Message:  fmt.Sprintf(msgUnmanagedTestSubprocess, ident.Name, sel.Sel.Name),
			}}
		}
		return nil
	})
}

// CheckTestFileutilHygiene scans Go test source files (*_test.go) for raw os.*
// file operations that bypass pkg/utils/fileutil consistency.
func CheckTestFileutilHygiene(root string, cfg *GatesConfig) ([]Finding, error) {
	return walkTestFileCallExprs(root, cfg, func(fset *token.FileSet, rel string, call *ast.CallExpr, ident *ast.Ident, sel *ast.SelectorExpr) []Finding {
		if ident.Name == "os" {
			switch sel.Sel.Name {
			case "ReadFile", "WriteFile", "Open", "OpenFile", "RemoveAll":
				pos := fset.Position(call.Pos())
				return []Finding{{
					CheckID:  "hygiene/unmanaged_test_fileutil",
					Suite:    "hygiene",
					Severity: SeverityError,
					File:     rel,
					Line:     pos.Line,
					Message: fmt.Sprintf("raw os.%s invocation in test file; prefer pkg/utils/fileutil.%s for filesystem consistency",
						sel.Sel.Name, sel.Sel.Name),
				}}
			}
		}
		return nil
	})
}
