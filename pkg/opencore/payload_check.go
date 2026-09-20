package opencore

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// PayloadOptions configures the payload check scan.
type PayloadCheckOptions struct {
	// ExcludedPatterns are globs that should be skipped during scanning.
	ExcludedPatterns []string
	// CheckBinaries controls whether binary file detection is enabled.
	CheckBinaries bool
}

// Violation represents a single finding in the payload check.
type Violation struct {
	Path     string
	Category string // e.g., "sensitive-pattern", "binary-detect"
	Message  string
	Line     int    // source line number, or 0 when not applicable
	Snippet  string // matched text snippet for context
}

// PayloadReport is the complete result of a payload check scan.
type PayloadReport struct {
	TotalViolations int         `json:"total_violations"`
	Violations      []Violation `json:"violations,omitempty"`
}

// sensitivePattern matches common API key and secret signatures in text files.
var sensitivePattern = regexp.MustCompile("(?i)(api[_-]?key|secret|password|token)\\s*[=:]\\s*[\"']?[A-Za-z0-9+/=_-]{8,}")

// binaryMarker checks if content contains typical ELF/Mach-O/PE magic bytes at start.
var binaryMarker = regexp.MustCompile("^\\x7fELF|^MZ|^\\xfe\\xed\\xfa\\xce|^\\xca\\xfe\\xba\\xbe")

// PayloadCheck scans a directory tree for release-payload concerns:
// - Sensitive credentials (API keys, secrets, passwords)
// - Binary blobs (which should not ship in an open-core archive)
func PayloadCheck(rootDir string, opts PayloadCheckOptions) (*PayloadReport, error) {
	report := &PayloadReport{}

	exclRegexes := make([]*regexp.Regexp, 0, len(opts.ExcludedPatterns))
	for _, pat := range opts.ExcludedPatterns {
		rx, err := filepath.Match(pat, "")
		if err != nil {
			return report, fmt.Errorf("invalid exclusion glob %q: %w", pat, err)
		}
		// Use the pattern as a contains-match via regexp.
		exclRegexes = append(exclRegexes, regexp.MustCompile("^"+pathToRegexp(pat)+"$"))
		_ = rx
	}

	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			return fmt.Errorf("compute rel path: %w", err)
		}

		for _, rx := range exclRegexes {
			if rx.MatchString(relPath) {
				return nil
			}
		}

		if d.IsDir() {
			return nil
		}

		// Check for sensitive patterns in text files.
		data, err := os.ReadFile(filepath.Clean(path)) //nolint:gosec // intentional static payload security scan
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}

		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if sensitivePattern.MatchString(line) {
				report.Violations = append(report.Violations, Violation{
					Path:     relPath,
					Category: "sensitive-pattern",
					Message:  fmt.Sprintf("Sensitive credential pattern detected in %s", path),
					Line:     i + 1,
					Snippet:  strings.TrimSpace(line),
				})
			}
		}

		// Check for binary files when enabled.
		if opts.CheckBinaries && len(data) > 0 && binaryMarker.Match(data[:min(4, len(data))]) {
			report.Violations = append(report.Violations, Violation{
				Path:     relPath,
				Category: "binary-detect",
				Message:  fmt.Sprintf("Binary file detected in release payload: %s", path),
			})
		}

		return nil
	})
	if err != nil {
		return report, fmt.Errorf("walk directory: %w", err)
	}

	report.TotalViolations = len(report.Violations)
	return report, nil
}

// pathToRegexp converts a glob pattern (e.g., "*.env") into a regex anchored string.
func pathToRegexp(pattern string) string {
	var sb strings.Builder
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '*':
			sb.WriteString("[^/]*")
		case '?':
			sb.WriteByte('.')
		default:
			sb.WriteRune(rune(ch))
		}
	}
	return sb.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
