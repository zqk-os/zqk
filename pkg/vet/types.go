package vet

import (
	"fmt"
	"time"
)

// Severity represents check finding severity.
type Severity string

const (
	SeverityError Severity = "ERROR"
	SeverityWarn  Severity = "WARN"
)

// Finding represents a single check violation or notice.
type Finding struct {
	CheckID  string   `json:"check_id" yaml:"check_id"`
	Suite    string   `json:"suite" yaml:"suite"`
	File     string   `json:"file,omitempty" yaml:"file,omitempty"`
	Line     int      `json:"line,omitempty" yaml:"line,omitempty"`
	Message  string   `json:"message" yaml:"message"`
	Severity Severity `json:"severity" yaml:"severity"`
}

func (f Finding) String() string {
	if f.File != "" && f.Line > 0 {
		return fmt.Sprintf("[%s] %s:%d: %s (%s)", f.Severity, f.File, f.Line, f.Message, f.CheckID)
	}
	if f.File != "" {
		return fmt.Sprintf("[%s] %s: %s (%s)", f.Severity, f.File, f.Message, f.CheckID)
	}
	return fmt.Sprintf("[%s] %s (%s)", f.Severity, f.Message, f.CheckID)
}

// SuiteResult contains findings and metadata for a suite run.
type SuiteResult struct {
	Suite    string        `json:"suite" yaml:"suite"`
	Passed   bool          `json:"passed" yaml:"passed"`
	Duration time.Duration `json:"duration" yaml:"duration"`
	Findings []Finding     `json:"findings" yaml:"findings"`
}

// Report contains overall results from all executed suites.
type Report struct {
	Passed       bool                   `json:"passed" yaml:"passed"`
	Duration     time.Duration          `json:"duration" yaml:"duration"`
	SuiteResults map[string]SuiteResult `json:"suites" yaml:"suites"`
	TotalErrors  int                    `json:"total_errors" yaml:"total_errors"`
	TotalWarns   int                    `json:"total_warns" yaml:"total_warns"`
}
