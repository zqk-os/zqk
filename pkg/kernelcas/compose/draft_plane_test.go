package compose

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestBacklogItemHierarchyGateRespectsDraftPlane pins the draft-plane contract for the
// hierarchical-chain rule: an item may be written down before its place in the hierarchy is
// known, and the link becomes mandatory when it claims to be ready for work.
//
// Both halves matter. Without the first, a draft cannot be edited at all — the rule fires on
// every update, not just on a transition, so a description fix is refused on an object whose
// whole purpose is to be incomplete. Without the second, the gate that keeps unlinked items
// from presenting as shovel-ready would be gone, which is the reason the rule exists.
func TestBacklogItemHierarchyGateRespectsDraftPlane(t *testing.T) {
	ResetDefaultForTest()
	if err := WarmDefaultRegistry(""); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name       string
		status     string
		wantBlocks bool
	}{
		{"draft is editable without a hierarchy link", objects.ObjectStatusExploring, false},
		{"planned still requires a hierarchy link", objects.ObjectStatusPlanned, true},
		{"in_progress still requires a hierarchy link", objects.ObjectStatusInProgress, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obj := map[string]any{
				objects.FieldKeyKind:   objects.KindBacklogItem,
				objects.FieldKeyStatus: tc.status,
				objects.FieldKeyID:     "BLI-draft-plane-test",
			}
			blocked := hasHierarchyChainError(ValidateObject(
				t.Context(), Default(), objects.KindBacklogItem, obj, nil))
			if blocked != tc.wantBlocks {
				t.Errorf("status %q: hierarchy chain blocked=%v, want %v",
					tc.status, blocked, tc.wantBlocks)
			}
		})
	}
}

// TestBacklogItemHierarchyGateSatisfiedByEitherRef keeps the rule's own contract intact: the
// draft exemption must not be the only way past it, or a linked planned item would fail.
func TestBacklogItemHierarchyGateSatisfiedByEitherRef(t *testing.T) {
	ResetDefaultForTest()
	if err := WarmDefaultRegistry(""); err != nil {
		t.Fatal(err)
	}

	for _, field := range []string{"goal_refs", "requirement_refs"} {
		t.Run(field, func(t *testing.T) {
			obj := map[string]any{
				objects.FieldKeyKind:   objects.KindBacklogItem,
				objects.FieldKeyStatus: objects.ObjectStatusPlanned,
				objects.FieldKeyID:     "BLI-draft-plane-linked",
				field:                  []any{"GOL-something"},
			}
			if hasHierarchyChainError(ValidateObject(
				t.Context(), Default(), objects.KindBacklogItem, obj, nil)) {
				t.Errorf("planned item with %s was still blocked by the hierarchy chain", field)
			}
		})
	}
}

// hasHierarchyChainError isolates this one rule from unrelated overlay findings, so the tests
// above cannot pass or fail on some other precondition the fixture happens to trip — a planned
// item with no refs also trips the priority rule, and counting errors would conflate them.
//
// Matching is on Field rather than Rule: ValidationError.Rule carries the composed definition
// name ("composed_integrity") shared by every rule in the overlay, while Field carries this
// rule's own field_label.
func hasHierarchyChainError(errs []ValidationError) bool {
	for _, e := range errs {
		if e.Field == "goal_refs/requirement_refs" {
			return true
		}
	}
	return false
}
