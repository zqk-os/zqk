package validation

import "testing"

func TestIsReusablePriorValidationState(t *testing.T) {
	t.Parallel()
	if isReusablePriorValidationState(nil) {
		t.Fatal("nil must not be reusable")
	}
	if !isReusablePriorValidationState(&ValidationState{}) {
		t.Fatal("empty issues should be reusable")
	}
	if !isReusablePriorValidationState(&ValidationState{
		Issues: []ValidationIssue{{Tier: 2, Category: "instance_validation", Message: "warn"}},
	}) {
		t.Fatal("Tier-2 only should be reusable")
	}
	if isReusablePriorValidationState(&ValidationState{
		Issues: []ValidationIssue{{Tier: 1, Category: "validation_error", Message: "validation timeout after 5s for X"}},
	}) {
		t.Fatal("Tier-1 must not be reusable")
	}
}
