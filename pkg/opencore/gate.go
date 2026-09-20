package opencore

import (
	"fmt"
	"strings"
)

// GateDecision is the terminal outcome of the OpenCore export gate.
type GateDecision string

const (
	// GatePass means the payload is cleared for open-core export.
	GatePass GateDecision = "pass"
	// GateDeny means the payload must NOT be exported.
	GateDeny GateDecision = "deny"
)

// Violation category names.
const (
	CategorySensitivePattern = "sensitive-pattern"
	CategoryBinaryDetect     = "binary-detect"
)

// GateOptions configures the export gate evaluation.
type GateOptions struct {
	// ExcludedPatterns are globs of files that should be skipped during scanning.
	ExcludedPatterns []string
	// DenyOnBinary forces a deny when binary blobs are present in the payload.
	DenyOnBinary bool
	// MaxAllowedViolations is the maximum number of sensitive-pattern
	// violations tolerated before the gate denies. Default is 0 (any
	// sensitive pattern is denied).
	MaxAllowedViolations int
}

// GateReport is the complete result of the export gate evaluation.
type GateReport struct {
	// Decision is the terminal gate outcome.
	Decision GateDecision `json:"decision"`
	// Reason is a human-readable explanation of the decision.
	Reason string `json:"reason,omitempty"`
	// PayloadReport embeds the underlying payload check findings.
	PayloadReport *PayloadReport `json:"payload_report,omitempty"`
}

const (
	errInvalidExclusion = "invalid exclusion glob"
	reasonPayloadCheck  = "payload check failed"
	reasonSensitiveDeny = "sensitive-pattern violation(s) exceed tolerance"
	reasonBinaryDeny    = "binary blobs present in strict mode"
	reasonMaxViolations = "MaxAllowedViolations must be >= 0"
	reasonClean         = "payload contains no denied patterns"
)

// RunExportGate evaluates a candidate open-core export directory and returns
// a pass/deny decision with the underlying evidence.
//
// The gate is a thin orchestration layer over PayloadCheck and enforces
// process data boundary isolation: it only reads from the candidate payload
// directory tree, never from outside, and never mutates the tree.
func RunExportGate(rootDir string, opts GateOptions) (*GateReport, error) {
	if opts.MaxAllowedViolations < 0 {
		return nil, fmt.Errorf("%s: %d", reasonMaxViolations, opts.MaxAllowedViolations)
	}

	payloadOpts := PayloadCheckOptions{
		ExcludedPatterns: opts.ExcludedPatterns,
		// Always scan for binaries in the gate so we can report them;
		// DenyOnBinary only controls whether their presence is a denial.
		CheckBinaries: true,
	}

	payloadReport, err := PayloadCheck(rootDir, payloadOpts)
	if err != nil {
		if strings.Contains(err.Error(), errInvalidExclusion) {
			return nil, fmt.Errorf("%s: %w", errInvalidExclusion, err)
		}
		return nil, fmt.Errorf("%s: %w", reasonPayloadCheck, err)
	}

	sensitiveCount, binaryCount := countByCategory(payloadReport.Violations)

	deniedReasons := make([]string, 0, 2)

	if sensitiveCount > opts.MaxAllowedViolations {
		deniedReasons = append(deniedReasons,
			fmt.Sprintf("%d %s (tolerance %d): %s",
				sensitiveCount, CategorySensitivePattern, opts.MaxAllowedViolations, reasonSensitiveDeny))
	}

	if opts.DenyOnBinary && binaryCount > 0 {
		deniedReasons = append(deniedReasons,
			fmt.Sprintf("%d %s: %s", binaryCount, CategoryBinaryDetect, reasonBinaryDeny))
	}

	if len(deniedReasons) > 0 {
		return &GateReport{
			Decision:      GateDeny,
			Reason:        strings.Join(deniedReasons, "; "),
			PayloadReport: payloadReport,
		}, nil
	}

	return &GateReport{
		Decision:      GatePass,
		Reason:        reasonClean,
		PayloadReport: payloadReport,
	}, nil
}

// countByCategory partitions violations into sensitive-pattern and
// binary-detect buckets. Other categories are not counted.
func countByCategory(vs []Violation) (sensitive int, binary int) {
	for _, v := range vs {
		switch v.Category {
		case CategorySensitivePattern:
			sensitive++
		case CategoryBinaryDetect:
			binary++
		}
	}
	return sensitive, binary
}
