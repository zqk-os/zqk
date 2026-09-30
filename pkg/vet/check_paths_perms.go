package vet

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CheckPathsAndPerms scans Go source files for hardcoded paths and magic permission numbers.
func CheckPathsAndPerms(root string, cfg *GatesConfig) ([]Finding, error) {
	var findings []Finding

	files, err := collectGoFiles(root, cfg.Hygiene.GoScanDirs)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()

	forbiddenList := cfg.Hygiene.ForbiddenPathLiterals
	if len(forbiddenList) == 0 {
		forbiddenList = []string{".zqk", ".zqk/"}
	}

	for _, rel := range files {
		if isPathExempt(rel, cfg.Hygiene.Exemptions) {
			continue
		}

		abs := filepath.Join(root, rel)
		src, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		if isGeneratedCode(src) {
			continue
		}

		node, err := parser.ParseFile(fset, rel, src, parser.ParseComments)
		if err != nil {
			continue
		}

		ast.Inspect(node, func(n ast.Node) bool {
			// Check magic file permissions
			if call, ok := n.(*ast.CallExpr); ok && cfg.Hygiene.CheckPerms {
				funName := getCallFunctionName(call)
				switch funName {
				case "os.OpenFile", "fileutil.OpenFile", "OpenFile":
					if len(call.Args) >= 3 && isMagicPerm(call.Args[2]) {
						pos := fset.Position(call.Args[2].Pos())
						findings = append(findings, Finding{
							CheckID:  "hygiene/magic-perms",
							Suite:    "hygiene",
							File:     rel,
							Line:     pos.Line,
							Message:  "magic file permission literal used; use paths.FilePerm* / paths.DirPerm*",
							Severity: SeverityError,
						})
					}
				case "os.Mkdir", "os.MkdirAll", "fileutil.Mkdir", "fileutil.MkdirAll", "Mkdir", "MkdirAll":
					if len(call.Args) >= 2 && isMagicPerm(call.Args[1]) {
						pos := fset.Position(call.Args[1].Pos())
						findings = append(findings, Finding{
							CheckID:  "hygiene/magic-perms",
							Suite:    "hygiene",
							File:     rel,
							Line:     pos.Line,
							Message:  "magic directory permission literal used; use paths.DirPerm755",
							Severity: SeverityError,
						})
					}
				case "os.WriteFile", "fileutil.WriteFile", "WriteFile":
					if len(call.Args) >= 3 && isMagicPerm(call.Args[2]) {
						pos := fset.Position(call.Args[2].Pos())
						findings = append(findings, Finding{
							CheckID:  "hygiene/magic-perms",
							Suite:    "hygiene",
							File:     rel,
							Line:     pos.Line,
							Message:  "magic file permission literal used; use paths.FilePerm644 / paths.FilePerm600",
							Severity: SeverityError,
						})
					}
				}
			}

			// Check configurable forbidden path literals on all string literals
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING && cfg.Hygiene.CheckPaths {
				val, _ := strconv.Unquote(lit.Value)
				for _, fp := range forbiddenList {
					prefix := strings.TrimSuffix(fp, "/") + "/"
					if val == fp || strings.HasPrefix(val, prefix) {
						pos := fset.Position(lit.Pos())
						findings = append(findings, Finding{
							CheckID:  "hygiene/hardcoded-path",
							Suite:    "hygiene",
							File:     rel,
							Line:     pos.Line,
							Message:  "hardcoded \"" + fp + "\" path literal used; use paths constants",
							Severity: SeverityError,
						})
						break
					}
				}
			}

			return true
		})
	}

	return findings, nil
}

func getCallFunctionName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		if x, ok := fun.X.(*ast.Ident); ok {
			return x.Name + "." + fun.Sel.Name
		}
		return fun.Sel.Name
	}
	return ""
}

func isMagicPerm(expr ast.Expr) bool {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return false
	}
	val, err := strconv.ParseInt(lit.Value, 0, 64)
	if err != nil {
		return false
	}
	return val == 0755 || val == 0644 || val == 0600 || val == 0700 || val == 0750
}
