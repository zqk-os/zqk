package workflow

import (
	"errors"
	"testing"
)

func TestLintObject_VaguePhrasesRejected(t *testing.T) {
	vagueCases := []struct {
		field string
		val   string
	}{
		{"title", "Make code better"},
		{"description", "Refactor the module so it is cleaner"},
		{"statement", "Verify that the code has changed"},
		{"description", "Let's improve the code here"},
		{"acceptance_criteria", "Fix things in the database"},
		{"rationale", "This will make various improvements to the system"},
	}

	for _, tc := range vagueCases {
		obj := map[string]any{
			"id":     "REQ-TEST-001",
			"kind":   "requirement",
			tc.field: tc.val,
		}
		res := LintObject(obj)
		if res.Passed {
			t.Errorf("expected failure for vague content in %s: %q", tc.field, tc.val)
		}
		if len(res.Violations) == 0 {
			t.Errorf("expected violations for %q, got 0", tc.val)
		}
	}
}

func TestLintRequirementWithCriteria_ThreeFoldFormula(t *testing.T) {
	req := map[string]any{
		"id":          "REQ-PAYMENT-001",
		"kind":        "requirement",
		"title":       "Idempotent Payment Charge Processing",
		"description": "Ensure payment charges are executed exactly once per idempotency key with deterministic CAS settlement.",
	}

	// Case 1: Fully satisfied Three-Fold criteria
	critValid := []map[string]any{
		{
			"id":           "CRIT-PAYMENT-INVARIANT-001",
			"title":        "Schema and Ledger Invariants",
			"formula_type": "invariant",
			"statement":    "Transaction ledger records immutable SHA-256 state hash.",
		},
		{
			"id":           "CRIT-PAYMENT-DYNAMIC-001",
			"title":        "End-to-End Dynamic Charge Execution",
			"formula_type": "dynamic",
			"statement":    "Charge operation executes against gateway and returns settled status.",
		},
		{
			"id":           "CRIT-PAYMENT-ADVERSARIAL-001",
			"title":        "Concurrent Double-Charge Fail-Closed Boundary",
			"formula_type": "adversarial",
			"statement":    "Duplicate charge requests within 50ms reject with HTTP 409 conflict.",
		},
	}

	res := LintRequirementWithCriteria(req, critValid)
	if !res.Passed {
		t.Fatalf("expected valid requirement to pass, got violations: %+v", res.Violations)
	}

	err := CheckShovelReadyQuality(req, critValid)
	if err != nil {
		t.Fatalf("expected CheckShovelReadyQuality to pass, got: %v", err)
	}

	// Case 2: Missing Adversarial criterion
	critMissingAdversarial := []map[string]any{
		{
			"id":           "CRIT-1",
			"formula_type": "invariant",
		},
		{
			"id":           "CRIT-2",
			"formula_type": "dynamic",
		},
		{
			"id":           "CRIT-3",
			"formula_type": "dynamic",
		},
	}

	res2 := LintRequirementWithCriteria(req, critMissingAdversarial)
	if res2.Passed {
		t.Fatal("expected failure when missing adversarial criterion, got passed")
	}

	err2 := CheckShovelReadyQuality(req, critMissingAdversarial)
	if err2 == nil || !errors.Is(err2, ErrThreeFoldFormulaUnsatisfied) {
		t.Errorf("expected ErrThreeFoldFormulaUnsatisfied, got: %v", err2)
	}

	// Case 3: Less than 3 criteria
	critTooFew := []map[string]any{
		{
			"id":           "CRIT-1",
			"formula_type": "invariant",
		},
		{
			"id":           "CRIT-2",
			"formula_type": "dynamic",
		},
	}

	res3 := LintRequirementWithCriteria(req, critTooFew)
	if res3.Passed {
		t.Fatal("expected failure when less than 3 criteria, got passed")
	}
}
