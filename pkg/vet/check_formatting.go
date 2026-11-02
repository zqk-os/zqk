package vet

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// CheckIDFormatting identifies findings emitted by CheckGoFormatting.
const CheckIDFormatting = "hygiene/formatting"

// CheckGoFormatting scans Go source files for compliance with standard gofmt formatting.
// Any Go source file exhibiting formatting drift reports a SeverityError finding.
func CheckGoFormatting(projectRoot string, files []string, cfg *GatesConfig) ([]Finding, error) {
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

	exemptions := append([]string{}, cfg.Hygiene.Exemptions...)
	exemptions = append(exemptions, cfg.Hygiene.FormattingExemptions...)
	if len(exemptions) == 0 {
		exemptions = []string{"vendor/*", "*/testdata/*"}
	}

	for _, rel := range files {
		rel = filepath.ToSlash(rel)
		if !strings.HasSuffix(rel, ".go") {
			continue
		}

		if strings.HasPrefix(rel, "vendor/") ||
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

		formatted, err := format.Source(src)
		if err != nil {
			findings = append(findings, Finding{
				CheckID:  CheckIDFormatting,
				Suite:    "hygiene",
				File:     rel,
				Line:     1,
				Message:  fmt.Sprintf("go format error: %v", err),
				Severity: SeverityError,
			})
			continue
		}

		if !bytes.Equal(src, formatted) {
			line := findFirstDiffLine(src, formatted)
			findings = append(findings, Finding{
				CheckID:  CheckIDFormatting,
				Suite:    "hygiene",
				File:     rel,
				Line:     line,
				Message:  fmt.Sprintf("formatting drift detected: file is not gofmt compliant; run 'gofmt -w %s' to fix", rel),
				Severity: SeverityError,
			})
		}
	}

	return findings, nil
}

// findFirstDiffLine locates the 1-based line index of the first difference between two byte slices.
func findFirstDiffLine(a, b []byte) int {
	linesA := bytes.Split(a, []byte("\n"))
	linesB := bytes.Split(b, []byte("\n"))
	minLen := len(linesA)
	if len(linesB) < minLen {
		minLen = len(linesB)
	}
	for i := 0; i < minLen; i++ {
		if !bytes.Equal(linesA[i], linesB[i]) {
			return i + 1
		}
	}
	return minLen + 1
}
