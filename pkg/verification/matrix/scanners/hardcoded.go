package scanners

import (
	"bufio"
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/verification/matrix"
)

// Scanner rule definitions for hardcoded logic patterns.
var (
	rxHardcodedIP      = regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b`)
	rxHardcodedUserDir = regexp.MustCompile(`/(?:Users|home)/[a-zA-Z0-9_\.\-]+`)
	rxRawPermOctal     = regexp.MustCompile(`\b0[67][0-7]{2}\b`)
	rxHardcodedRelease = regexp.MustCompile(`"v[0-9]+\.[0-9]+\.[0-9]+(?:-[a-zA-Z0-9\.\-]+)?"`)
	rxHardcodedSecret  = regexp.MustCompile(`(?i)(?:bearer\s+[a-zA-Z0-9_\-\.]{20,}|ghp_[a-zA-Z0-9]{20,}|sk_live_[a-zA-Z0-9]{20,}|AKIA[0-9A-Z]{16})`)
)

// HardcodedLogicScanner scans files for magic values, hardcoded release strings, raw permissions, and user paths.
type HardcodedLogicScanner struct{}

// NewHardcodedLogicScanner creates a scanner instance.
func NewHardcodedLogicScanner() *HardcodedLogicScanner {
	return &HardcodedLogicScanner{}
}

// Run executes the scanner on a target file entry.
func (s *HardcodedLogicScanner) Run(ctx context.Context, repoRoot string, entry *matrix.FileEntry) (matrix.CheckResult, error) {
	fullPath := filepath.Join(repoRoot, entry.Path)
	file, err := fileutil.OpenRead(fullPath)
	if err != nil {
		return matrix.CheckResult{
			CheckID:     "hardcoded-logic",
			Status:      matrix.CheckStatusFailed,
			Evaluator:   "scanner:hardcoded-logic",
			EvaluatedAt: time.Now().UTC(),
			Feedback:    fmt.Sprintf("Failed to open file: %v", err),
		}, err
	}
	defer file.Close()

	var findings []matrix.Finding
	scanner := bufio.NewScanner(file)
	lineNum := 0

	isTestFile := entry.Class == matrix.ClassGoTest || strings.HasSuffix(entry.Path, "_test.go")
	isDocsOrConfig := entry.Class == matrix.ClassDocs || entry.Class == matrix.ClassConfigFile

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip comments and blank lines
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "/*") || trimmed == "" {
			continue
		}

		// 1. Hardcoded User Directory Paths (forbidden everywhere except docs/fixtures)
		if !isDocsOrConfig && rxHardcodedUserDir.MatchString(line) {
			findings = append(findings, matrix.Finding{
				Line:     lineNum,
				RuleID:   "no-hardcoded-user-path",
				Message:  "Forbidden hardcoded absolute user home directory path detected",
				Severity: "error",
			})
		}

		// 2. Hardcoded Release Tags in Production Code
		if entry.Class == matrix.ClassGoProd && rxHardcodedRelease.MatchString(line) {
			// Ignore if it's explicitly inside an example, test mock, or documentation string
			if !strings.Contains(line, "example") && !strings.Contains(line, "mock") {
				findings = append(findings, matrix.Finding{
					Line:     lineNum,
					RuleID:   "no-hardcoded-release-tag",
					Message:  "Production Go code contains hardcoded semantic version release tag; use build ldflags or config",
					Severity: "error",
				})
			}
		}

		// 3. Raw Permission Octals in Production Go Code
		if entry.Class == matrix.ClassGoProd && rxRawPermOctal.MatchString(line) {
			// Exclude fileutil package itself which defines the permission constants
			if !strings.Contains(entry.Path, "pkg/utils/fileutil") && !strings.Contains(entry.Path, "pkg/paths") {
				findings = append(findings, matrix.Finding{
					Line:     lineNum,
					RuleID:   "no-raw-permission-octal",
					Message:  "Raw octal file permission literal used in production code; prefer fileutil constants",
					Severity: "error",
				})
			}
		}

		// 4. Hardcoded Secrets / Tokens
		if rxHardcodedSecret.MatchString(line) {
			findings = append(findings, matrix.Finding{
				Line:     lineNum,
				RuleID:   "no-hardcoded-secret",
				Message:  "Potential secret, API key, or authentication token pattern detected",
				Severity: "error",
			})
		}

		// 5. Hardcoded IP Addresses in Production Code
		if !isTestFile && !isDocsOrConfig && rxHardcodedIP.MatchString(line) {
			// Ignore common loopback inside local server default comments or explicit localhost binding
			if !strings.Contains(line, "127.0.0.1") && !strings.Contains(line, "0.0.0.0") {
				findings = append(findings, matrix.Finding{
					Line:     lineNum,
					RuleID:   "no-hardcoded-ip",
					Message:  "Hardcoded IP address literal detected in non-test file",
					Severity: "warning",
				})
			}
		}
	}

	status := matrix.CheckStatusPassed
	feedback := "All hardcoded logic and literal checks passed."
	for _, f := range findings {
		if f.Severity == "error" {
			status = matrix.CheckStatusFailed
			feedback = fmt.Sprintf("Found %d hardcoded logic violations.", len(findings))
			break
		}
	}

	return matrix.CheckResult{
		CheckID:     "hardcoded-logic",
		Status:      status,
		Evaluator:   "scanner:hardcoded-logic",
		EvaluatedAt: time.Now().UTC(),
		Feedback:    feedback,
		Findings:    findings,
	}, nil
}
