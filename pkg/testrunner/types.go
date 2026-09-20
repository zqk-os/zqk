package testrunner

import (
	"time"
)

// RunOptions defines options for running test cases.
type RunOptions struct {
	Verbose        bool
	DryRun         bool
	Timeout        time.Duration
	CriteriaFilter []string
}

// CriterionInvocation represents a concrete command invocation tied to a criterion.
type CriterionInvocation struct {
	CriterionID string   `json:"criterion_id"`
	Command     string   `json:"command"`
	Args        []string `json:"args,omitempty"`
	Env         []string `json:"env,omitempty"`
	Dir         string   `json:"dir,omitempty"`
}

// CriterionRunResult represents the outcome of executing a test for one criterion.
type CriterionRunResult struct {
	CriterionID string        `json:"criterion_id"`
	Command     string        `json:"command"`
	Passed      bool          `json:"passed"`
	ExitCode    int           `json:"exit_code"`
	Stdout      string        `json:"stdout,omitempty"`
	Stderr      string        `json:"stderr,omitempty"`
	Duration    time.Duration `json:"duration_ms"`
	Error       string        `json:"error,omitempty"`
}

// TestRunResult holds the comprehensive result of executing a test case.
type TestRunResult struct {
	TestCaseID           string               `json:"test_case_id"`
	TestCaseTitle        string               `json:"test_case_title"`
	PathOrID             string               `json:"path_or_id"`
	Status               string               `json:"status"` // "passed", "failed", "partial"
	TotalCriteria        int                  `json:"total_criteria"`
	PassedCriteria       int                  `json:"passed_criteria"`
	FailedCriteria       int                  `json:"failed_criteria"`
	CriteriaResults      []CriterionRunResult `json:"criteria_results"`
	RemainingOpenCount   int                  `json:"remaining_open_count"`
	TestCaseCompleted    bool                 `json:"test_case_completed"`
	RequirementCompleted bool                 `json:"requirement_completed"`
}
