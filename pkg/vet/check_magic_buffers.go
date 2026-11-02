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

// CheckMagicBuffers scans Go source files for inline magic number buffer allocations
// (e.g. scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)).
// Per repository memory hygiene governance, memory buffer dimensions must declare
// named constants for buffer initial and max capacities to prevent subtle allocation
// failures and guarantee maintainable operational bounds.
func CheckMagicBuffers(projectRoot string, files []string, cfg *GatesConfig) ([]Finding, error) {
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
			isPathExempt(rel, exemptions) {
			continue
		}

		abs := rel
		if !filepath.IsAbs(rel) {
			abs = filepath.Join(projectRoot, rel)
		}

		src, err := fileutil.ReadFile(abs)
		if err != nil {
			if os.IsNotExist(err) {
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
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			// Check calls to scanner.Buffer(buf, max)
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Buffer" {
				for _, arg := range call.Args {
					if isMagicBufferExpr(arg) {
						pos := fset.Position(arg.Pos())
						findings = append(findings, Finding{
							CheckID:  "hygiene/magic_buffer",
							Suite:    "hygiene",
							Severity: SeverityWarn,
							File:     rel,
							Line:     pos.Line,
							Message:  fmt.Sprintf("magic buffer size expression in %s; extract memory buffer size to a named constant", sel.Sel.Name),
						})
					}
				}
			}

			return true
		})
	}

	return findings, nil
}

func isMagicBufferExpr(expr ast.Expr) bool {
	switch x := expr.(type) {
	case *ast.BinaryExpr:
		if x.Op == token.MUL || x.Op == token.SHL || x.Op == token.ADD {
			if isLiteralNumber(x.X) || isLiteralNumber(x.Y) {
				return true
			}
		}
	case *ast.CallExpr:
		if ident, ok := x.Fun.(*ast.Ident); ok && ident.Name == "make" {
			for _, arg := range x.Args[1:] {
				if isMagicBufferExpr(arg) {
					return true
				}
			}
		}
	}
	return false
}

func isLiteralNumber(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return e.Kind == token.INT
	case *ast.BinaryExpr:
		return isLiteralNumber(e.X) && isLiteralNumber(e.Y)
	}
	return false
}
