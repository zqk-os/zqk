package check_test

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1/check"
)

func TestWorkflowChecker_Run(t *testing.T) {
	t.Parallel()

	checker := check.NewWorkflowChecker()
	if checker == nil {
		t.Fatal("expected non-nil WorkflowChecker")
	}

	result, err := checker.Run()
	if err != nil {
		t.Fatalf("checker.Run() unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil CheckResult")
	}

	score := result.Score()
	if score != 100 {
		t.Errorf("expected score 100, got %d", score)
	}

	summary := result.Summary()
	if summary == "" {
		t.Fatal("expected non-empty summary")
	}

	expectedSubstrings := []string{
		"Workflow State Check Result (Score: 100%):",
		"Session context: PASS",
		"Priority plan aligned: PASS",
		"Backlog item valid: PASS",
		"Policy compliance: PASS",
		"Branch valid: PASS",
		"Git clean: PASS",
		"Branch based on main: PASS",
		"Item first non-completed: PASS",
	}
	for _, substr := range expectedSubstrings {
		if !strings.Contains(summary, substr) {
			t.Errorf("expected summary to contain %q, but got:\n%s", substr, summary)
		}
	}
}

func TestCheckResult_Empty(t *testing.T) {
	t.Parallel()

	result := check.NewCheckResult()
	if result == nil {
		t.Fatal("expected non-nil CheckResult")
	}

	if score := result.Score(); score != 0 {
		t.Errorf("expected score 0 for initial result, got %d", score)
	}

	summary := result.Summary()
	if !strings.Contains(summary, "Score: 0%") {
		t.Errorf("expected summary to reflect Score: 0%%, got %s", summary)
	}
	if !strings.Contains(summary, "Session context: FAIL") {
		t.Errorf("expected summary to contain 'Session context: FAIL', got %s", summary)
	}
}
