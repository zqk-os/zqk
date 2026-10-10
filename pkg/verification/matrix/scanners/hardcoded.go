package scanners

import (
	"context"
	"regexp"

	"github.com/zqk-os/zqk/pkg/verification/matrix"
)

// CustomRule defines an optional custom pattern rule for the scanner.
type CustomRule struct {
	ID               string
	Message          string
	Severity         string
	Regex            *regexp.Regexp
	TargetClasses    []matrix.FileClass
	IgnoreIfContains []string
}

// HardcodedLogicScanner scans files for magic values, hardcoded release strings, raw permissions, and user paths.
// It is a specialized instance of PatternScanner configured with anti-hardcoding rules.
type HardcodedLogicScanner struct {
	scanner *PatternScanner
}

// NewHardcodedLogicScanner creates a scanner instance initialized with default anti-hardcoding pattern rules.
func NewHardcodedLogicScanner() *HardcodedLogicScanner {
	return &HardcodedLogicScanner{
		scanner: NewPatternScanner("hardcoded-logic", DefaultPatternRules()...),
	}
}

// WithCustomRules adds custom pattern rules to the scanner.
func (s *HardcodedLogicScanner) WithCustomRules(rules []CustomRule) *HardcodedLogicScanner {
	for _, r := range rules {
		s.scanner.AddRules(PatternRule{
			ID:               r.ID,
			Message:          r.Message,
			Severity:         r.Severity,
			Regex:            r.Regex,
			TargetClasses:    r.TargetClasses,
			IgnoreIfContains: r.IgnoreIfContains,
			Dimension:        matrix.DimensionHCODE,
		})
	}
	return s
}

// Scanner returns the underlying PatternScanner instance.
func (s *HardcodedLogicScanner) Scanner() *PatternScanner {
	return s.scanner
}

// Run executes the scanner on a target file entry.
func (s *HardcodedLogicScanner) Run(ctx context.Context, repoRoot string, entry *matrix.FileEntry) (matrix.CheckResult, error) {
	return s.scanner.Run(ctx, repoRoot, entry)
}
