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
	files, err := collectGoFiles(root, cfg.Hygiene.GoScanDirs)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	forbiddenList := cfg.Hygiene.ForbiddenPathLiterals
	if len(forbiddenList) == 0 {
		forbiddenList = []string{".zqk", ".zqk/"}
	}

	var findings []Finding
	for _, rel := range files {
		if isPathExempt(rel, cfg.Hygiene.Exemptions) {
			continue
		}

		abs := filepath.Join(root, rel)
		src, err := os.ReadFile(abs)
		if err != nil || isGeneratedCode(src) {
			continue
		}

		node, err := parser.ParseFile(fset, rel, src, parser.ParseComments)
		if err != nil {
			continue
		}

		findings = append(findings, auditFileNode(rel, node, fset, cfg, forbiddenList)...)
	}

	return findings, nil
}

func auditFileNode(rel string, node *ast.File, fset *token.FileSet, cfg *GatesConfig, forbiddenList []string) []Finding {
	var fileFindings []Finding
	ast.Inspect(node, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && cfg.Hygiene.CheckPerms {
			if f := inspectCallExprPerms(call, rel, fset); f != nil {
				fileFindings = append(fileFindings, *f)
			}
		}

		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING && cfg.Hygiene.CheckPaths {
			if f := inspectStringLiteralPath(lit, rel, fset, forbiddenList); f != nil {
				fileFindings = append(fileFindings, *f)
			}
		}

		return true
	})
	return fileFindings
}

func inspectCallExprPerms(call *ast.CallExpr, rel string, fset *token.FileSet) *Finding {
	funName := getCallFunctionName(call)
	switch funName {
	case "os.OpenFile", "fileutil.OpenFile", "OpenFile":
		if len(call.Args) >= 3 && isMagicPerm(call.Args[2]) {
			return makePermFinding(rel, fset.Position(call.Args[2].Pos()).Line, "magic file permission literal used; use paths.FilePerm* / paths.DirPerm*")
		}
	case "os.Mkdir", "os.MkdirAll", "fileutil.Mkdir", "fileutil.MkdirAll", "Mkdir", "MkdirAll":
		if len(call.Args) >= 2 && isMagicPerm(call.Args[1]) {
			return makePermFinding(rel, fset.Position(call.Args[1].Pos()).Line, "magic directory permission literal used; use paths.DirPerm755")
		}
	case "os.WriteFile", "fileutil.WriteFile", "WriteFile":
		if len(call.Args) >= 3 && isMagicPerm(call.Args[2]) {
			return makePermFinding(rel, fset.Position(call.Args[2].Pos()).Line, "magic file permission literal used; use paths.FilePerm644 / paths.FilePerm600")
		}
	}
	return nil
}

func inspectStringLiteralPath(lit *ast.BasicLit, rel string, fset *token.FileSet, forbiddenList []string) *Finding {
	val, _ := strconv.Unquote(lit.Value)
	for _, fp := range forbiddenList {
		prefix := strings.TrimSuffix(fp, "/") + "/"
		if val == fp || strings.HasPrefix(val, prefix) {
			pos := fset.Position(lit.Pos())
			return &Finding{
				CheckID:  "hygiene/hardcoded-path",
				Suite:    "hygiene",
				File:     rel,
				Line:     pos.Line,
				Message:  "hardcoded \"" + fp + "\" path literal used; use paths constants",
				Severity: SeverityError,
			}
		}
	}

	if strings.HasPrefix(val, "/Users/") || strings.HasPrefix(val, "/home/") || strings.HasPrefix(val, "/var/folders/") || strings.HasPrefix(val, "/private/var/") {
		pos := fset.Position(lit.Pos())
		return &Finding{
			CheckID:  "hygiene/hardcoded-workstation-path",
			Suite:    "hygiene",
			File:     rel,
			Line:     pos.Line,
			Message:  "hardcoded absolute workstation path literal (\"" + val + "\"); use paths, os.UserHomeDir(), or t.TempDir()",
			Severity: SeverityError,
		}
	}
	return nil
}

func makePermFinding(rel string, line int, msg string) *Finding {
	return &Finding{
		CheckID:  "hygiene/magic-perms",
		Suite:    "hygiene",
		File:     rel,
		Line:     line,
		Message:  msg,
		Severity: SeverityError,
	}
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
