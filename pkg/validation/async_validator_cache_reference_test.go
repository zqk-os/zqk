package validation

import "testing"

// TRACK: REDACTED
func TestHasBlockingReferenceIssues(t *testing.T) {
	if hasBlockingReferenceIssues(nil) {
		t.Fatal("nil issues")
	}
	if hasBlockingReferenceIssues([]ValidationIssue{{Category: "lifecycle", Tier: 1}}) {
		t.Fatal("lifecycle should not force")
	}
	if hasBlockingReferenceIssues([]ValidationIssue{{Category: "reference", Tier: 3}}) {
		t.Fatal("tier-3 reference should not force")
	}
	if !hasBlockingReferenceIssues([]ValidationIssue{{Category: "reference", Tier: 1}}) {
		t.Fatal("tier-1 reference must force re-enqueue")
	}
	if !hasBlockingReferenceIssues([]ValidationIssue{{Category: "GhostRef", Tier: 1}}) {
		t.Fatal("tier-1 GhostRef must force re-enqueue (not a durable cache hit)")
	}
}
