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

// Standard regex definitions for universal anti-hardcoding and security patterns.
var (
	rxHardcodedIP      = regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b`)
	rxHardcodedUserDir = regexp.MustCompile(`/(?:Users|home)/[a-zA-Z0-9_\.\-]+`)
	rxRawPermOctal     = regexp.MustCompile(`\b0[67][0-7]{2}\b`)
	rxHardcodedRelease = regexp.MustCompile(`"v[0-9]+\.[0-9]+\.[0-9]+(?:-[a-zA-Z0-9\.\-]+)?"`)
	rxHardcodedSecret  = regexp.MustCompile(`(?i)(?:bearer\s+[a-zA-Z0-9_\-\.]{20,}|ghp_[a-zA-Z0-9]{20,}|sk_live_[a-zA-Z0-9]{20,}|AKIA[0-9A-Z]{16})`)
)

// PatternRule defines a declarative pattern inspection rule.
type PatternRule struct {
	ID               string               `json:"id" yaml:"id"`
	Message          string               `json:"message" yaml:"message"`
	Severity         string               `json:"severity" yaml:"severity"` // "critical", "error", "warning", "info"
	Pattern          string               `json:"pattern,omitempty" yaml:"pattern,omitempty"`
	Regex            *regexp.Regexp       `json:"-" yaml:"-"`
	TargetClasses    []matrix.FileClass   `json:"target_classes,omitempty" yaml:"target_classes,omitempty"`
	ExcludeClasses   []matrix.FileClass   `json:"exclude_classes,omitempty" yaml:"exclude_classes,omitempty"`
	IgnoreIfContains []string             `json:"ignore_if_contains,omitempty" yaml:"ignore_if_contains,omitempty"`
	Dimension        matrix.DimensionCode `json:"dimension,omitempty" yaml:"dimension,omitempty"`
}

// Compile compiles the rule's Pattern string into Regex if not already compiled.
func (r *PatternRule) Compile() error {
	if r.Regex != nil {
		return nil
	}
	if r.Pattern != "" {
		rx, err := regexp.Compile(r.Pattern)
		if err != nil {
			return fmt.Errorf("invalid pattern regex %q for rule %s: %w", r.Pattern, r.ID, err)
		}
		r.Regex = rx
	}
	return nil
}

// DefaultPatternRules returns the standard set of pattern rules for anti-hardcoding and secrets.
func DefaultPatternRules() []PatternRule {
	return []PatternRule{
		{
			ID:             "no-hardcoded-user-path",
			Message:        "Forbidden hardcoded absolute user home directory path detected",
			Severity:       "error",
			Regex:          rxHardcodedUserDir,
			ExcludeClasses: []matrix.FileClass{matrix.ClassDocs, matrix.ClassConfigFile},
			Dimension:      matrix.DimensionHCODE,
		},
		{
			ID:               "no-hardcoded-release-tag",
			Message:          "Production code contains hardcoded semantic version release tag; use build variables or configuration",
			Severity:         "error",
			Regex:            rxHardcodedRelease,
			TargetClasses:    []matrix.FileClass{matrix.ClassSource},
			IgnoreIfContains: []string{"example", "mock"},
			Dimension:        matrix.DimensionHCODE,
		},
		{
			ID:               "no-raw-permission-octal",
			Message:          "Raw octal file permission literal used in production code; prefer symbolic permission constants",
			Severity:         "error",
			Regex:            rxRawPermOctal,
			TargetClasses:    []matrix.FileClass{matrix.ClassSource},
			IgnoreIfContains: []string{"fileutil", "permissions"},
			Dimension:        matrix.DimensionHCODE,
		},
		{
			ID:        "no-hardcoded-secret",
			Message:   "Potential secret, API key, or authentication token pattern detected",
			Severity:  "error",
			Regex:     rxHardcodedSecret,
			Dimension: matrix.DimensionSECOBS,
		},
		{
			ID:               "no-hardcoded-ip",
			Message:          "Hardcoded IP address literal detected in non-test file",
			Severity:         "warning",
			Regex:            rxHardcodedIP,
			ExcludeClasses:   []matrix.FileClass{matrix.ClassTest, matrix.ClassDocs, matrix.ClassConfigFile},
			IgnoreIfContains: []string{"127.0.0.1", "0.0.0.0"},
			Dimension:        matrix.DimensionHCODE,
		},
	}
}

// PatternScanner is a generic, language-agnostic scanner executing declarative regex pattern rules.
type PatternScanner struct {
	checkID string
	rules   []PatternRule
}

// NewPatternScanner creates a new PatternScanner with the given check ID and pattern rules.
func NewPatternScanner(checkID string, rules ...PatternRule) *PatternScanner {
	if checkID == "" {
		checkID = "pattern-scanner"
	}
	s := &PatternScanner{
		checkID: checkID,
	}
	s.AddRules(rules...)
	return s
}

// AddRules appends pattern rules to the scanner, compiling any uncompiled regexes.
func (s *PatternScanner) AddRules(rules ...PatternRule) *PatternScanner {
	for _, r := range rules {
		_ = r.Compile()
		s.rules = append(s.rules, r)
	}
	return s
}

// Rules returns a copy of the active pattern rules.
func (s *PatternScanner) Rules() []PatternRule {
	cp := make([]PatternRule, len(s.rules))
	copy(cp, s.rules)
	return cp
}

// Run executes all configured pattern rules line-by-line against the target file.
func (s *PatternScanner) Run(ctx context.Context, repoRoot string, entry *matrix.FileEntry) (matrix.CheckResult, error) {
	fullPath := filepath.Join(repoRoot, entry.Path)
	file, err := fileutil.OpenRead(fullPath)
	if err != nil {
		return matrix.CheckResult{
			CheckID:     s.checkID,
			Status:      matrix.CheckStatusFailed,
			Evaluator:   "scanner:" + s.checkID,
			EvaluatedAt: time.Now().UTC(),
			Feedback:    fmt.Sprintf("Failed to open file: %v", err),
		}, err
	}
	defer file.Close()

	var findings []matrix.Finding
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip comments and blank lines across all common languages
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") ||
			strings.HasPrefix(trimmed, "--") || strings.HasPrefix(trimmed, ";") ||
			trimmed == "" {
			continue
		}

		for _, rule := range s.rules {
			// Check target classes filter
			if len(rule.TargetClasses) > 0 {
				matchedClass := false
				for _, tc := range rule.TargetClasses {
					if tc == entry.Class {
						matchedClass = true
						break
					}
				}
				if !matchedClass {
					continue
				}
			}

			// Check exclude classes filter
			if len(rule.ExcludeClasses) > 0 {
				excluded := false
				for _, ec := range rule.ExcludeClasses {
					if ec == entry.Class {
						excluded = true
						break
					}
				}
				if excluded {
					continue
				}
			}

			if rule.Regex == nil {
				continue
			}

			// Check if line contains any ignored phrases
			ignored := false
			for _, ign := range rule.IgnoreIfContains {
				if strings.Contains(line, ign) || strings.Contains(entry.Path, ign) {
					ignored = true
					break
				}
			}
			if ignored {
				continue
			}

			if rule.Regex.MatchString(line) {
				findings = append(findings, matrix.Finding{
					Line:     lineNum,
					RuleID:   rule.ID,
					Message:  rule.Message,
					Severity: rule.Severity,
					Category: string(rule.Dimension),
				})
			}
		}
	}

	status := matrix.CheckStatusPassed
	feedback := fmt.Sprintf("All %s pattern checks passed.", s.checkID)
	for _, f := range findings {
		if f.Severity == "error" || f.Severity == "critical" {
			status = matrix.CheckStatusFailed
			feedback = fmt.Sprintf("Found %d pattern violations in %s.", len(findings), entry.Path)
			break
		}
	}

	return matrix.CheckResult{
		CheckID:     s.checkID,
		Status:      status,
		Evaluator:   "scanner:" + s.checkID,
		EvaluatedAt: time.Now().UTC(),
		Feedback:    feedback,
		Findings:    findings,
	}, nil
}
