package vet

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var exemptFuncs = map[string]struct{}{
	"CLIUsage":                       {},
	"CLIInvocation":                  {},
	"RewriteCanonicalCLIInvocations": {},
}

// CheckCLINames scans Go files in projectRoot for hardcoded canonical CLI invocations.
func CheckCLINames(projectRoot string, files []string) ([]Finding, error) {
	var findings []Finding

	if len(files) == 0 {
		var err error
		files, err = collectGoFiles(projectRoot)
		if err != nil {
			return nil, err
		}
	}

	for _, rel := range files {
		if shouldSkipCLINameCheck(rel) {
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

		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, rel, src, parser.ParseComments)
		if err != nil {
			continue
		}

		var stack []ast.Node
		ast.Inspect(f, func(n ast.Node) bool {
			if n == nil {
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
				return false
			}
			stack = append(stack, n)
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if callExempt(stack) || inConstDecl(stack) {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if hit, verb := firstCanonicalCLIHit(s); hit {
				pos := fset.Position(lit.Pos())
				findings = append(findings, Finding{
					CheckID:  "cli_name_literal",
					Suite:    "hygiene",
					File:     rel,
					Line:     pos.Line,
					Message:  "hardcoded \"" + brand.CanonicalExecutableToken + "\" " + verb + " invocation — use pkg/paths.CLIUsage / CLIInvocation / RewriteCanonicalCLIInvocations",
					Severity: SeverityError,
				})
			}
			return true
		})
	}

	return findings, nil
}

func shouldSkipCLINameCheck(rel string) bool {
	rel = filepath.ToSlash(rel)
	switch {
	case strings.HasSuffix(rel, "_test.go"):
		return true
	case strings.Contains(rel, "/testdata/"):
		return true
	case strings.HasPrefix(rel, "vendor/"):
		return true
	case rel == "pkg/paths/cli_command_name.go":
		return true
	case strings.HasPrefix(rel, "scripts/check_cli_name_literals/"):
		return true
	case strings.HasPrefix(rel, "pkg/vet/"):
		return true
	}
	return false
}

func firstCanonicalCLIHit(s string) (bool, string) {
	needle := brand.CanonicalExecutableToken + " "
	rest := s
	for {
		i := strings.Index(rest, needle)
		if i < 0 {
			return false, ""
		}
		if i > 0 {
			prev := rune(rest[i-1])
			if unicode.IsLetter(prev) || unicode.IsDigit(prev) || prev == '_' || prev == '-' {
				rest = rest[i+len(needle):]
				continue
			}
		}
		after := rest[i+len(needle):]
		verb, _, _ := strings.Cut(after, " ")
		verb = strings.TrimRight(verb, "\"'`.,;:)")
		if paths.IsProductCLIVerb(verb) {
			return true, verb
		}
		rest = after
	}
}

func callExempt(stack []ast.Node) bool {
	for i := len(stack) - 1; i >= 0; i-- {
		call, ok := stack[i].(*ast.CallExpr)
		if !ok {
			continue
		}
		name := callFunName(call)
		if _, ok := exemptFuncs[name]; ok {
			return true
		}
	}
	return false
}

func inConstDecl(stack []ast.Node) bool {
	for _, n := range stack {
		if gd, ok := n.(*ast.GenDecl); ok && gd.Tok == token.CONST {
			return true
		}
	}
	return false
}

func callFunName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		if fun.Sel != nil {
			return fun.Sel.Name
		}
	}
	return ""
}
