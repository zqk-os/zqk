package check

import (
	"fmt"
)

// CheckResult represents the outcome of a workflow check operation.
type CheckResult struct {
	sessionValid     bool
	planAligned      bool
	itemValid        bool
	policyOK         bool
	branchValid      bool
	gitClean         bool
	branchLatestMain bool
	itemFirstNonComp bool
	feedback         []string
}

// NewCheckResult creates a new CheckResult with initial values.
func NewCheckResult() *CheckResult {
	return &CheckResult{
		feedback: []string{},
	}
}

// Score returns the adherence score (0-100%).
func (r *CheckResult) Score() int {
	total := 8
	passed := 0
	if r.sessionValid { passed++ }
	if r.planAligned { passed++ }
	if r.itemValid { passed++ }
	if r.policyOK { passed++ }
	if r.branchValid { passed++ }
	if r.gitClean { passed++ }
	if r.branchLatestMain { passed++ }
	if r.itemFirstNonComp { passed++ }
	return (passed * 100) / total
}

// Summary returns a human-readable summary of the check results.
func (r *CheckResult) Summary() string {
	summary := fmt.Sprintf("Workflow State Check Result (Score: %d%%):\n", r.Score())
	summary += fmt.Sprintf("  Session context: %v\n", statusLabel(r.sessionValid))
	summary += fmt.Sprintf("  Priority plan aligned: %v\n", statusLabel(r.planAligned))
	summary += fmt.Sprintf("  Backlog item valid: %v\n", statusLabel(r.itemValid))
	summary += fmt.Sprintf("  Policy compliance: %v\n", statusLabel(r.policyOK))
	summary += fmt.Sprintf("  Branch valid: %v\n", statusLabel(r.branchValid))
	summary += fmt.Sprintf("  Git clean: %v\n", statusLabel(r.gitClean))
	summary += fmt.Sprintf("  Branch based on main: %v\n", statusLabel(r.branchLatestMain))
	summary += fmt.Sprintf("  Item first non-completed: %v\n", statusLabel(r.itemFirstNonComp))
	if len(r.feedback) > 0 {
		summary += "\nFeedback:\n"
		for _, f := range r.feedback {
			summary += fmt.Sprintf("  - %s\n", f)
		}
	}
	return summary
}

// statusLabel returns "PASS" or "FAIL" based on the boolean value.
func statusLabel(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}

// WorkflowChecker performs validation of the current workflow state.
type WorkflowChecker struct{}

// NewWorkflowChecker creates a new WorkflowChecker instance.
func NewWorkflowChecker() *WorkflowChecker {
	return &WorkflowChecker{}
}

// Run executes all workflow checks and returns the aggregated result.
func (c *WorkflowChecker) Run() (*CheckResult, error) {
	result := NewCheckResult()

	// NOTE: Placeholder implementations
	result.sessionValid = true
	result.planAligned = true
	result.itemValid = true
	result.policyOK = true
	result.branchValid = true
	result.gitClean = true
	result.branchLatestMain = true
	result.itemFirstNonComp = true

	return result, nil
}
