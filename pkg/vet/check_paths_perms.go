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

	files, err := collectGoFiles(root)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()

	for _, rel := range files {
		if shouldSkipPathFile(rel) {
			continue
		}
		if isExempt(rel, cfg.Hygiene.Exemptions) {
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
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			// Check magic file permissions
			if cfg.Hygiene.CheckPerms {
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

			// Check hardcoded paths like ".zqk"
			if cfg.Hygiene.CheckPaths {
				for _, arg := range call.Args {
					lit, ok := arg.(*ast.BasicLit)
					if ok && lit.Kind == token.STRING {
						val, _ := strconv.Unquote(lit.Value)
						if val == ".zqk" || strings.HasPrefix(val, ".zqk/") {
							pos := fset.Position(lit.Pos())
							findings = append(findings, Finding{
								CheckID:  "hygiene/hardcoded-path",
								Suite:    "hygiene",
								File:     rel,
								Line:     pos.Line,
								Message:  "hardcoded \".zqk\" path literal used; use paths constants",
								Severity: SeverityError,
							})
						}
					}
				}
			}

			return true
		})
	}

	return findings, nil
}

func shouldSkipPathFile(rel string) bool {
	switch {
	case strings.HasSuffix(rel, "_test.go"):
		return true
	case strings.HasPrefix(rel, "vendor/"):
		return true
	case strings.HasPrefix(rel, ".git/"):
		return true
	case strings.HasPrefix(rel, "pkg/paths/"):
		return true
	case strings.HasPrefix(rel, "pkg/brand/"):
		return true
	case strings.HasPrefix(rel, "scripts/check_path_and_perm_literals/"):
		return true
	case strings.HasPrefix(rel, "pkg/specbuilder/bldr_"):
		return true
	case strings.HasPrefix(rel, "pkg/cli/bldr_cli_cmd_v1/"):
		return true
	case strings.HasPrefix(rel, "pkg/utils/fileutil/"):
		return true
	case strings.HasPrefix(rel, "scripts/prepare_cas_updates/"):
		return true
	case strings.HasPrefix(rel, "scripts/legacy-decommission/"):
		return true
	case strings.HasPrefix(rel, "pkg/vet/"):
		return true
	case strings.Contains(rel, "tools_sandbox"):
		return true
	}
	return false
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

func isExempt(path string, exemptions []string) bool {
	for _, ex := range exemptions {
		if path == ex {
			return true
		}
		if matched, _ := filepath.Match(ex, path); matched {
			return true
		}
		if matched, _ := filepath.Match(ex, filepath.Base(path)); matched {
			return true
		}
		if strings.HasSuffix(ex, "/*") {
			prefix := strings.TrimSuffix(ex, "/*")
			if strings.HasPrefix(path, prefix+"/") {
				return true
			}
		}
	}
	return false
}
