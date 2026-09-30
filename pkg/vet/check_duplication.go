package vet

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	defaultDupMinStatements = 5
	defaultDupMinLines      = 8
)

var (
	commentLineRegex = regexp.MustCompile(`//.*$|/\*[\s\S]*?\*/`)
)

// FuncLocation records where a function was declared.
type FuncLocation struct {
	File  string
	Line  int
	Name  string
	Stmts int
	Lines int
}

type dupGroup struct {
	Stmts     int
	Lines     int
	Locations []FuncLocation
}

// CheckDuplication scans Go source files for identical function bodies,
// enforcing DRY principles across packages and modules.
func CheckDuplication(projectRoot string, files []string, cfg *GatesConfig) ([]Finding, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	if len(files) == 0 {
		var err error
		files, err = collectGoFiles(projectRoot, cfg.Hygiene.GoScanDirs)
		if err != nil {
			return nil, err
		}
	}

	minStmts := cfg.Hygiene.DupMinStatements
	if minStmts <= 0 {
		minStmts = defaultDupMinStatements
	}

	minLines := cfg.Hygiene.DupMinLines
	if minLines <= 0 {
		minLines = defaultDupMinLines
	}

	exemptions := append([]string{}, cfg.Hygiene.Exemptions...)
	exemptions = append(exemptions, cfg.Hygiene.DupExemptions...)
	if len(exemptions) == 0 {
		exemptions = []string{"*_test.go", "vendor/*", "*/testdata/*", "*/mock/*"}
	}

	fset := token.NewFileSet()
	bodyHashMap := make(map[string]*dupGroup)

	for _, rel := range files {
		if err := auditFileDuplication(fset, projectRoot, rel, exemptions, minStmts, minLines, bodyHashMap); err != nil {
			return nil, err
		}
	}

	return collectDuplicationFindings(bodyHashMap), nil
}

func auditFileDuplication(fset *token.FileSet, projectRoot, rel string, exemptions []string, minStmts, minLines int, bodyHashMap map[string]*dupGroup) error {
	rel = filepath.ToSlash(rel)
	if strings.HasSuffix(rel, "_test.go") ||
		strings.HasPrefix(rel, "vendor/") ||
		strings.Contains(rel, "/testdata/") ||
		strings.HasPrefix(rel, "testdata/") ||
		strings.Contains(rel, "/mock/") ||
		isPathExempt(rel, exemptions) {
		return nil
	}

	abs := rel
	if !filepath.IsAbs(rel) {
		abs = filepath.Join(projectRoot, rel)
	}

	src, err := fileutil.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if isGeneratedCode(src) {
		return nil
	}

	node, err := parser.ParseFile(fset, rel, src, 0)
	if err != nil {
		return nil
	}

	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		stmtCount := len(fn.Body.List)
		if stmtCount < minStmts {
			continue
		}

		startPos := fset.Position(fn.Pos())
		endPos := fset.Position(fn.End())
		lineCount := endPos.Line - startPos.Line + 1
		if lineCount < minLines {
			continue
		}

		normalized, err := normalizeFuncBody(fn.Body)
		if err != nil || normalized == "" {
			continue
		}

		hashBytes := sha256.Sum256([]byte(normalized))
		hash := hex.EncodeToString(hashBytes[:])

		group, exists := bodyHashMap[hash]
		if !exists {
			group = &dupGroup{
				Stmts: stmtCount,
				Lines: lineCount,
			}
			bodyHashMap[hash] = group
		}

		group.Locations = append(group.Locations, FuncLocation{
			File:  rel,
			Line:  startPos.Line,
			Name:  fn.Name.Name,
			Stmts: stmtCount,
			Lines: lineCount,
		})
	}
	return nil
}

func collectDuplicationFindings(bodyHashMap map[string]*dupGroup) []Finding {
	var findings []Finding
	var hashes []string
	for h := range bodyHashMap {
		hashes = append(hashes, h)
	}
	sort.Strings(hashes)

	for _, h := range hashes {
		group := bodyHashMap[h]
		if len(group.Locations) < 2 {
			continue
		}

		primary := group.Locations[0]
		for _, dup := range group.Locations[1:] {
			if dup.File == primary.File && dup.Line == primary.Line {
				continue
			}

			findings = append(findings, Finding{
				CheckID:  "hygiene/duplication",
				Suite:    "hygiene",
				Severity: SeverityWarn,
				File:     dup.File,
				Line:     dup.Line,
				Message: fmt.Sprintf(
					"duplicative function body in %q (%d statements, %d lines) is identical to %s:%d (%s); refactor into a shared abstraction (DRY)",
					dup.Name, dup.Stmts, dup.Lines, primary.File, primary.Line, primary.Name,
				),
			})
		}
	}
	return findings
}

func normalizeFuncBody(body *ast.BlockStmt) (string, error) {
	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), body); err != nil {
		return "", err
	}

	raw := buf.String()
	raw = commentLineRegex.ReplaceAllString(raw, "")

	lines := strings.Split(raw, "\n")
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}

	return strings.Join(cleaned, "\n"), nil
}
