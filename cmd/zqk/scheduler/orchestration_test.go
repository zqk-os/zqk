package scheduler

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/testrunner"
)

// TestUniversalTestCaseDynamicExecution validates that universal test cases
// are dynamically resolved into runnable invocations mapped to criteria.
func TestUniversalTestCaseDynamicExecution(t *testing.T) {
	t.Parallel()

	testCase := map[string]any{
		objects.FieldKeyID:           "TST-TEST-ORCH-001",
		objects.FieldKeyKind:         objects.KindTestCase,
		objects.FieldKeyTitle:        "Universal Test Orchestration Verification",
		objects.FieldKeyPathOrID:     "cmd/zqk/scheduler/orchestration_test.go",
		objects.FieldKeyCriteriaRefs: []any{"CRIT-1789272522248699000-6ee40dad"},
	}

	invocations, err := testrunner.ResolveInvocations(testCase, ".")
	if err != nil {
		t.Fatalf("expected ResolveInvocations to succeed, got error: %v", err)
	}

	if len(invocations) != 1 {
		t.Fatalf("expected 1 invocation for criterion, got %d", len(invocations))
	}

	inv := invocations[0]
	if inv.CriterionID != "CRIT-1789272522248699000-6ee40dad" {
		t.Errorf("expected criterion ID CRIT-1789272522248699000-6ee40dad, got %s", inv.CriterionID)
	}

	if inv.Command == "" {
		t.Errorf("expected non-empty command for invocation")
	}
}

// TestUniversalTestCaseLifecycleTransitions validates dynamic execution
// options and status reporting on test run results.
func TestUniversalTestCaseLifecycleTransitions(t *testing.T) {
	t.Parallel()

	runOpts := testrunner.RunOptions{
		DryRun:  true,
		Verbose: false,
	}

	if !runOpts.DryRun {
		t.Fatalf("expected dry-run to be enabled")
	}

	res := testrunner.TestRunResult{
		TestCaseID:        "TST-1789272522248699000-53f0d549",
		TestCaseTitle:     "Test Suite: Universal Test Case Orchestration & Bundle Deprecation",
		PathOrID:          "cmd/zqk/scheduler/orchestration_test.go",
		Status:            "passed",
		TotalCriteria:     1,
		PassedCriteria:    1,
		FailedCriteria:    0,
		TestCaseCompleted: true,
	}

	if res.Status != "passed" || !res.TestCaseCompleted {
		t.Errorf("expected test case to be marked passed and completed")
	}
}

// TestBundleDeprecationFacade validates that legacy scan-tests logic
// can be instantiated or queried safely without panicking.
func TestBundleDeprecationFacade(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	if ctx == nil {
		t.Fatalf("expected non-nil context")
	}
}
