package validation

import "testing"

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
	if !hasBlockingReferenceIssues([]ValidationIssue{{Category: "traceability", Tier: 1}}) {
		t.Fatal("tier-1 traceability must force re-enqueue")
	}
	if !hasBlockingReferenceIssues([]ValidationIssue{{Category: "reference_integrity", Tier: 1}}) {
		t.Fatal("tier-1 reference_integrity must force re-enqueue")
	}
	if !hasBlockingReferenceIssues([]ValidationIssue{{Category: "REFERENCE", Tier: 1}}) {
		t.Fatal("tier-1 uppercase REFERENCE must force re-enqueue")
	}
}

func TestValidationStateCache_InvalidateBlockingReferenceIssues(t *testing.T) {
	c := NewValidationStateCache(t.TempDir(), 0)
	c.Set(&ValidationState{
		ObjectID:   "CRIT-001",
		ObjectKind: "criteria",
		Issues:     []ValidationIssue{{Category: "traceability", Tier: 1}},
	})
	c.Set(&ValidationState{
		ObjectID:   "REQ-001",
		ObjectKind: "requirement",
		Issues:     []ValidationIssue{{Category: "reference", Tier: 1}},
	})
	c.Set(&ValidationState{
		ObjectID:   "GOAL-001",
		ObjectKind: "goal",
		Issues:     []ValidationIssue{{Category: "lifecycle", Tier: 2}},
	})

	count := c.InvalidateBlockingReferenceIssues()
	if count != 2 {
		t.Fatalf("expected 2 invalidated entries, got %d", count)
	}

	if _, ok := c.Get("CRIT-001"); ok {
		t.Fatal("expected CRIT-001 to be invalidated")
	}
	if _, ok := c.Get("REQ-001"); ok {
		t.Fatal("expected REQ-001 to be invalidated")
	}
	if _, ok := c.Get("GOAL-001"); !ok {
		t.Fatal("expected GOAL-001 to remain in cache")
	}
}
