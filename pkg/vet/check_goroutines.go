package vet

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// CheckRawGoroutines scans Go source files for naked/unmanaged 'go' statements.
// Per repository concurrency policy (POL-CONCURRENCY), all asynchronous execution
// in production code must be managed via pkg/goroutinelabels (or goroutinelabels.Pool)
// to ensure traceability, panic handling, cancellation, and metrics.
func CheckRawGoroutines(projectRoot string, files []string, cfg *GatesConfig) ([]Finding, error) {
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

	exemptions := cfg.Hygiene.GoroutineExemptions
	if len(exemptions) == 0 {
		exemptions = []string{"*_test.go", "vendor/*", "pkg/goroutinelabels/*", "*/testdata/*"}
	}

	fset := token.NewFileSet()

	for _, rel := range files {
		rel = filepath.ToSlash(rel)
		if strings.HasSuffix(rel, "_test.go") ||
			strings.HasPrefix(rel, "pkg/goroutinelabels/") ||
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
			// Skip files that fail parsing (e.g. invalid syntax during draft editing)
			continue
		}

		ast.Inspect(node, func(n ast.Node) bool {
			goStmt, ok := n.(*ast.GoStmt)
			if !ok {
				return true
			}

			pos := fset.Position(goStmt.Pos())
			findings = append(findings, Finding{
				CheckID:  "hygiene/raw_goroutine",
				Suite:    "hygiene",
				Severity: SeverityError,
				File:     rel,
				Line:     pos.Line,
				Message:  "naked goroutine statement 'go ...' prohibited; use goroutinelabels.NewGoroutine(\"<name>\", \"<purpose>\").StartSimple(...) or goroutinelabels.Pool for managed concurrency (POL-CONCURRENCY)",
			})
			return true
		})
	}

	return findings, nil
}
