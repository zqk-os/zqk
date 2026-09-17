package scenario

import (
	"bytes"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestVerificationShockwaveE2E validates the "Red TDD phase" constraint:
// A backlog_item cannot be shovel-ready (planned) unless it links to a
// criteria object which is linked to a test_case that is draft, active, or error.
func TestVerificationShockwaveE2E(t *testing.T) {
	ctx, secCtx, provider, _, projectRoot := setupAppliedConvergenceLifecycleBundle(t)

	// Create a requirement, criteria, and backlog_item via a scenario bundle.
	// Omit the test_case first to prove the promotion is blocked!
	bundleYAML := `
api_version: "zqk.io/v2"
kind: "Bundle"
metadata:
  name: "shockwave-e2e-no-test"
objects:
  goals:
    - id_hint: "GOAL-SHOCK-1"
      title: "Test Goal"
      status: "active"
      authority: "PM"
      target: "Q1"
  requirements:
    - id_hint: "REQ-SHOCK-1"
      title: "Test Requirement"
      status: "active"
      goal_refs: ["GOAL-SHOCK-1"]
      criteria_refs: ["CRIT-SHOCK-1"]
  criteria:
    - id_hint: "CRIT-SHOCK-1"
      title: "Test Criteria"
      status: "awaiting_verification"
      category: "acceptance"
  backlog_items:
    - id_hint: "BLI-SHOCK-1"
      title: "Test Backlog"
      status: "testing"
      priority: "p3"
      criteria_refs: ["CRIT-SHOCK-1"]
`
	summary, err := ApplyScenarioBundle(ctx, projectRoot, bytes.NewBufferString(bundleYAML), ApplyObjectsOnly, &ApplyOptions{Storage: provider})
	if err != nil {
		t.Fatalf("Failed to apply initial bundle: %v", err)
	}

	bliID := summary.HintToID["BLI-SHOCK-1"]
	if bliID == "" {
		t.Fatalf("Failed to extract BLI ID from summary")
	}

	// Try to promote the BLI to "in_progress". It should FAIL because there is no red test case!
	obj, err := provider.Read(ctx, secCtx, bliID)
	if err != nil {
		t.Fatalf("Failed to read BLI: %v", err)
	}

	// simulate what `zqk object promote` does by using the lifecycle engine or by trying to update the status directly.
	// Actually, we can just use `objects.Promote(ctx, secCtx, provider, obj, ...)` if that exists.
	// Or we can just call the save/update which runs validation:
	obj[objects.FieldKeyStatus] = "in_progress"
	err = provider.Update(ctx, secCtx, bliID, obj)
	if err == nil {
		t.Fatalf("Expected BLI promotion to 'in_progress' to fail due to missing red TDD test, but it succeeded!")
	}
	if !strings.Contains(err.Error(), "Must link to a criteria object which is linked to an active but failing test_case") {
		t.Fatalf("Expected red TDD test validation error, got: %v", err)
	}

	// Now introduce the test_case linked to the criteria!
	bundleTestYAML := `
api_version: "zqk.io/v2"
kind: "Bundle"
metadata:
  name: "shockwave-e2e-with-test"
objects:
  test_cases:
    - id_hint: "TST-SHOCK-1"
      title: "Integration Test"
      status: "draft"
      category: "Integration"
      scope: "integration"
      criteria_refs: ["CRIT-SHOCK-1"]
`
	_, err = ApplyScenarioBundle(ctx, projectRoot, bytes.NewBufferString(bundleTestYAML), ApplyObjectsOnly, &ApplyOptions{Storage: provider})
	if err != nil {
		t.Fatalf("Failed to apply test bundle: %v", err)
	}

	// Try to promote the BLI again!
	obj[objects.FieldKeyStatus] = "in_progress"
	err = provider.Update(ctx, secCtx, bliID, obj)
	if err != nil {
		t.Fatalf("Expected BLI promotion to succeed after test_case was added, but failed: %v", err)
	}

	// Success! The structural integrity constraint is natively verified in Go.
}
