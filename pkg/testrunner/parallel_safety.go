package testrunner

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// SafetyLevel classifies a test's readiness for t.Parallel().
type SafetyLevel string

const (
	SafetyLevelParallelAlready SafetyLevel = "parallel"
	SafetyLevelSafe            SafetyLevel = "safe"
	SafetyLevelUnsafeSetenv    SafetyLevel = "unsafe_setenv"
	SafetyLevelNeedsReview     SafetyLevel = "needs_review"
)

// TestSafetyResult describes the parallel safety analysis of a single test function.
type TestSafetyResult struct {
	File     string      `json:"file" yaml:"file"`
	Package  string      `json:"package" yaml:"package"`
	FuncName string      `json:"func_name" yaml:"func_name"`
	Level    SafetyLevel `json:"level" yaml:"level"`
	Reasons  []string    `json:"reasons,omitempty" yaml:"reasons,omitempty"`
}

// ScanParallelSafety analyzes test files under the target directory for t.Parallel safety.
func ScanParallelSafety(projectRoot, searchPath string) ([]TestSafetyResult, error) {
	fullPath := searchPath
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(projectRoot, searchPath)
	}

	var results []TestSafetyResult
	fset := token.NewFileSet()

	err := filepath.WalkDir(fullPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "vendor" || name == ".sandbox" || name == "scratch" {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		fileRes, err := analyzeTestFile(fset, projectRoot, path)
		if err != nil {
			// Skip files that fail to parse (e.g. build tags)
			return nil
		}
		results = append(results, fileRes...)
		return nil
	})
	if err != nil {
		return nil, errfmt.Newf("scan parallel safety under %q", fullPath).Wrap(err)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].File != results[j].File {
			return results[i].File < results[j].File
		}
		return results[i].FuncName < results[j].FuncName
	})

	return results, nil
}

func analyzeTestFile(fset *token.FileSet, root, filePath string) ([]TestSafetyResult, error) {
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	relPath, err := filepath.Rel(root, filePath)
	if err != nil {
		relPath = filePath
	}

	pkgName := node.Name.Name
	var results []TestSafetyResult

	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		funcName := fn.Name.Name
		if !strings.HasPrefix(funcName, "Test") {
			continue
		}

		res := analyzeTestFunction(fn, relPath, pkgName)
		results = append(results, res)
	}

	return results, nil
}

func analyzeTestFunction(fn *ast.FuncDecl, relPath, pkgName string) TestSafetyResult {
	funcName := fn.Name.Name
	hasParallel := false
	hasSetenv := false
	hasTempDir := false
	hasOSSetenv := false

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		methodName := sel.Sel.Name
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}

		receiver := ident.Name
		if (receiver == "t" || receiver == "tb") && methodName == "Parallel" {
			hasParallel = true
		}
		if (receiver == "t" || receiver == "tb") && methodName == "Setenv" {
			hasSetenv = true
		}
		if (receiver == "t" || receiver == "tb") && methodName == "TempDir" {
			hasTempDir = true
		}
		if receiver == "os" && methodName == "Setenv" {
			hasOSSetenv = true
		}

		return true
	})

	if hasParallel {
		return TestSafetyResult{
			File:     relPath,
			Package:  pkgName,
			FuncName: funcName,
			Level:    SafetyLevelParallelAlready,
			Reasons:  []string{"already adopts t.Parallel()"},
		}
	}

	if hasSetenv {
		return TestSafetyResult{
			File:     relPath,
			Package:  pkgName,
			FuncName: funcName,
			Level:    SafetyLevelUnsafeSetenv,
			Reasons:  []string{"calls t.Setenv() which panics in parallel tests"},
		}
	}

	var reasons []string
	if hasTempDir {
		reasons = append(reasons, "uses t.TempDir() filesystem isolation")
		if hasOSSetenv {
			reasons = append(reasons, "uses os.Setenv() - verify cleanup isolation")
			return TestSafetyResult{
				File:     relPath,
				Package:  pkgName,
				FuncName: funcName,
				Level:    SafetyLevelNeedsReview,
				Reasons:  reasons,
			}
		}
		return TestSafetyResult{
			File:     relPath,
			Package:  pkgName,
			FuncName: funcName,
			Level:    SafetyLevelSafe,
			Reasons:  reasons,
		}
	}

	if hasOSSetenv {
		reasons = append(reasons, "modifies os environment without t.Setenv")
	} else {
		reasons = append(reasons, "does not declare t.TempDir() or global mutations")
	}

	return TestSafetyResult{
		File:     relPath,
		Package:  pkgName,
		FuncName: funcName,
		Level:    SafetyLevelNeedsReview,
		Reasons:  reasons,
	}
}
