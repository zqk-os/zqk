package vet

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// CheckImperativeBlankValidation scans Go source files for imperative blank string validation:
//
//	if strings.TrimSpace(val) == "" { return fmt.Errorf(...) }
//
// Per POL-CODING-FUNCTIONAL-BLANK-VALIDATION-001, code should employ declarative and functional
// validation helpers (e.g., strutil.ErrorOnBlank / stringutil.ErrorOnBlank) instead of procedural checks.
func CheckImperativeBlankValidation(projectRoot string, files []string, cfg *GatesConfig) ([]Finding, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	var findings []Finding

	if len(files) == 0 {
		var err error
		files, err = collectGoFiles(projectRoot, cfg.Hygiene.GoScanDirs)
		if err != nil {
			return nil, err
		}
	}

	exemptions := cfg.Hygiene.Exemptions
	fset := token.NewFileSet()

	for _, rel := range files {
		rel = filepath.ToSlash(rel)
		if strings.HasSuffix(rel, "_test.go") ||
			strings.HasPrefix(rel, "vendor/") ||
			strings.Contains(rel, "/testdata/") ||
			strings.HasPrefix(rel, "testdata/") ||
			rel == "pkg/strutil/blank.go" ||
			isPathExempt(rel, exemptions) {
			continue
		}

		abs := rel
		if !filepath.IsAbs(rel) {
			abs = filepath.Join(projectRoot, rel)
		}

		src, err := fileutil.ReadFile(abs)
		if err != nil {
			if fileutil.IsNotExist(err) {
				continue
			}
			return nil, err
		}

		if isGeneratedCode(src) {
			continue
		}

		node, err := parser.ParseFile(fset, rel, src, 0)
		if err != nil {
			continue
		}

		ast.Inspect(node, func(n ast.Node) bool {
			ifStmt, ok := n.(*ast.IfStmt)
			if !ok {
				return true
			}

			if !isTrimSpaceBlankCheck(ifStmt.Cond) {
				return true
			}

			if hasErrorReturn(ifStmt.Body) {
				pos := fset.Position(ifStmt.Pos())
				findings = append(findings, Finding{
					CheckID:  "hygiene/anti_pattern_blank_check",
					Suite:    "hygiene",
					Severity: SeverityWarn,
					File:     rel,
					Line:     pos.Line,
					Message:  "imperative blank check anti-pattern detected; use declarative stringutil.ErrorOnBlank(value, format, args...) or strutil.ErrorOnBlank in place of 'if strings.TrimSpace(...) == \"\" { return fmt.Errorf(...) }'",
				})
			}

			return true
		})
	}

	return findings, nil
}

// isTrimSpaceBlankCheck verifies if the condition matches:
// 1. strings.TrimSpace(...) == "" (or "" == strings.TrimSpace(...))
// 2. len(strings.TrimSpace(...)) == 0 (or 0 == len(strings.TrimSpace(...)))
func isTrimSpaceBlankCheck(expr ast.Expr) bool {
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok || bin.Op != token.EQL {
		return false
	}

	// Case 1: strings.TrimSpace(...) == ""
	if (isTrimSpaceCall(bin.X) && isEmptyStringLit(bin.Y)) ||
		(isEmptyStringLit(bin.X) && isTrimSpaceCall(bin.Y)) {
		return true
	}

	// Case 2: len(strings.TrimSpace(...)) == 0
	if (isLenTrimSpaceCall(bin.X) && isZeroLit(bin.Y)) ||
		(isZeroLit(bin.X) && isLenTrimSpaceCall(bin.Y)) {
		return true
	}

	return false
}

func isTrimSpaceCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "strings" && sel.Sel.Name == "TrimSpace"
}

func isLenTrimSpaceCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	ident, ok := call.Fun.(*ast.Ident)
	if !ok || ident.Name != "len" || len(call.Args) != 1 {
		return false
	}
	return isTrimSpaceCall(call.Args[0])
}

func isEmptyStringLit(expr ast.Expr) bool {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	return lit.Value == `""` || lit.Value == "``"
}

func isZeroLit(expr ast.Expr) bool {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return false
	}
	return lit.Value == "0"
}

func hasErrorReturn(block *ast.BlockStmt) bool {
	if block == nil {
		return false
	}
	found := false
	ast.Inspect(block, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, res := range ret.Results {
			if isErrorConstructExpr(res) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func isErrorConstructExpr(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		selName := fun.Sel.Name
		if pkgIdent, ok := fun.X.(*ast.Ident); ok {
			pkg := pkgIdent.Name
			if (pkg == "fmt" && selName == "Errorf") ||
				(pkg == "errors" && selName == "New") ||
				(pkg == "errfmt" && (selName == "Newf" || selName == "Wrap")) {
				return true
			}
		}
	}
	return false
}
