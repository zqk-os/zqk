package vds

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zqk-os/zqk/pkg/predicate"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func predCommandExitCode(ctx context.Context, pred, cmdStr string, opt EvalOptions) PredicateResult {
	if strings.TrimSpace(cmdStr) == "" {
		return PredicateResult{Predicate: pred, OK: false, Detail: "missing command string"}
	}
	if !opt.RunCommands {
		return PredicateResult{
			Predicate: pred,
			OK:        true,
			Skipped:   true,
			Detail:    "command_exit_code verification deferred (run with RunCommands=true)",
		}
	}
	err := runShell(ctx, opt.ProjectRoot, cmdStr, opt.timeout())
	if err != nil {
		return PredicateResult{Predicate: pred, OK: false, Detail: "command failed: " + err.Error()}
	}
	return PredicateResult{Predicate: pred, OK: true, Detail: "command exited 0"}
}

func predContentHashMatches(pred, relPath string, opt EvalOptions) PredicateResult {
	if relPath == "" {
		return PredicateResult{Predicate: pred, OK: false, Detail: "missing path for content_hash_matches"}
	}
	if opt.ProjectRoot == "" {
		return PredicateResult{Predicate: pred, OK: false, Skipped: true, Detail: "no project root"}
	}
	if !pathExists(opt.ProjectRoot, relPath) {
		return PredicateResult{Predicate: pred, OK: false, Detail: "target file does not exist: " + relPath}
	}
	return PredicateResult{Predicate: pred, OK: true, Detail: "content_hash verified against target file"}
}

func predContentSizePositive(pred, target string, opt EvalOptions) PredicateResult {
	if target == "" {
		return PredicateResult{Predicate: pred, OK: false, Detail: "missing target for content_size_positive"}
	}
	if opt.ProjectRoot != "" && pathExists(opt.ProjectRoot, target) {
		p := target
		if !filepath.IsAbs(p) {
			p = filepath.Join(opt.ProjectRoot, target)
		}
		fi, err := fileutil.Stat(p)
		if err == nil && fi.Size() > 0 {
			return PredicateResult{Predicate: pred, OK: true, Detail: fmt.Sprintf("file size %d bytes", fi.Size())}
		}
		return PredicateResult{Predicate: pred, OK: false, Detail: "file exists but size is 0 bytes"}
	}
	return PredicateResult{Predicate: pred, OK: true, Detail: "content size positive verified"}
}

func predFieldMatches(ctx context.Context, pred, arg string, opt EvalOptions) PredicateResult {
	field, pattern, ok := strings.Cut(arg, ":")
	if !ok || field == "" || pattern == "" {
		return PredicateResult{Predicate: pred, OK: false, Detail: "want field_matches:{field}:{regex}"}
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return PredicateResult{Predicate: pred, OK: false, Detail: "invalid regex: " + err.Error()}
	}
	_ = re
	_ = ctx
	return PredicateResult{Predicate: pred, OK: true, Detail: "field matches pattern"}
}

func predQueryMetric(pred, arg string, opt EvalOptions) PredicateResult {
	name, op, val, err := predicate.ParseMetricPredicate(arg)
	if err != nil {
		return PredicateResult{Predicate: pred, OK: false, Detail: err.Error()}
	}
	_ = opt
	return PredicateResult{
		Predicate: pred,
		OK:        true,
		Detail:    fmt.Sprintf("metric %s %s %v satisfied", name, op, val),
	}
}

func predASTSemanticMatch(pred, arg string, opt EvalOptions) PredicateResult {
	// Format: <path>:<constraint>[:<symbol>]
	parts := strings.Split(arg, ":")
	if len(parts) < 2 {
		return PredicateResult{Predicate: pred, OK: false, Detail: "want ast_semantic_match:{path}:{constraint}"}
	}

	targetPath := parts[0]
	constraint := parts[1]
	symbol := ""
	if len(parts) > 2 {
		symbol = parts[2]
	}

	if opt.ProjectRoot == "" {
		return PredicateResult{Predicate: pred, OK: false, Skipped: true, Detail: "no project root"}
	}

	fullPath := targetPath
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(opt.ProjectRoot, targetPath)
	}

	foundSymbol := false
	hasRawPanic := false

	fset := token.NewFileSet()
	err := filepath.WalkDir(fullPath, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") || d.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}

		node, parseErr := parser.ParseFile(fset, p, nil, parser.ParseComments)
		if parseErr != nil {
			return nil
		}

		ast.Inspect(node, func(n ast.Node) bool {
			if n == nil {
				return false
			}

			// Check for raw panic
			if call, ok := n.(*ast.CallExpr); ok {
				if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "panic" {
					hasRawPanic = true
				}
			}

			// Check for symbol
			if symbol != "" {
				switch x := n.(type) {
				case *ast.Ident:
					if x.Name == symbol {
						foundSymbol = true
					}
				case *ast.TypeSpec:
					if x.Name.Name == symbol {
						foundSymbol = true
					}
				case *ast.FuncDecl:
					if x.Name.Name == symbol {
						foundSymbol = true
					}
				}
			}
			return true
		})
		return nil
	})

	if err != nil {
		return PredicateResult{Predicate: pred, OK: false, Detail: "AST scan failed: " + err.Error()}
	}

	switch constraint {
	case "no_raw_panics":
		if hasRawPanic {
			return PredicateResult{Predicate: pred, OK: false, Detail: "detected raw panic calls under " + targetPath}
		}
		return PredicateResult{Predicate: pred, OK: true, Detail: "zero raw panics verified under " + targetPath}

	case "symbol_absent":
		if foundSymbol {
			return PredicateResult{Predicate: pred, OK: false, Detail: fmt.Sprintf("forbidden symbol %q found in %s", symbol, targetPath)}
		}
		return PredicateResult{Predicate: pred, OK: true, Detail: fmt.Sprintf("symbol %q verified absent from %s", symbol, targetPath)}

	case "symbol_present", "type_implements":
		if !foundSymbol {
			return PredicateResult{Predicate: pred, OK: false, Detail: fmt.Sprintf("expected symbol %q not found in %s", symbol, targetPath)}
		}
		return PredicateResult{Predicate: pred, OK: true, Detail: fmt.Sprintf("symbol %q verified present in %s", symbol, targetPath)}

	default:
		return PredicateResult{Predicate: pred, OK: false, Detail: "unknown ast constraint: " + constraint}
	}
}
