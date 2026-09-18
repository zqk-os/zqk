package storage

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestHierarchyParentIDs_CollectsListAndSingular(t *testing.T) {
	t.Parallel()
	got := HierarchyParentIDs(map[string]any{
		objects.FieldKeyPriorityPlanRef: "PRI-1",
		objects.FieldKeyMilestoneRefs:   []any{"MIL-A", "MIL-B"},
		objects.FieldKeyGoalRefs:        []string{"GOAL-1"},
		"milestone_ref":                 "MIL-LEGACY",
		objects.FieldKeyBacklogItemRef:  "BLI-1",
	})
	want := map[string]bool{
		"PRI-1":      true,
		"MIL-A":      true,
		"MIL-B":      true,
		"GOAL-1":     true,
		"MIL-LEGACY": true,
		"BLI-1":      true,
	}
	if len(got) != len(want) {
		t.Fatalf("ids = %#v, want %d unique parents", got, len(want))
	}
	for _, id := range got {
		if !want[id] {
			t.Fatalf("unexpected parent %q in %#v", id, got)
		}
	}
}

func TestHierarchyBubbleToStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind, status, wantTo string
		hop                  bool
	}{
		{objects.KindMilestone, objects.ObjectStatusNotStarted, objects.ObjectStatusInProgress, true},
		{objects.KindMilestone, objects.ObjectStatusBlocked, objects.ObjectStatusInProgress, true},
		{objects.KindMilestone, objects.ObjectStatusInProgress, "", false},
		{objects.KindPriorityPlan, objects.ObjectStatusActive, objects.ObjectStatusInProgress, true},
		{objects.KindGoal, objects.ObjectStatusProposed, objects.ObjectStatusActive, true},
		{objects.KindGoal, objects.ObjectStatusActive, "", false},
	}
	for _, tc := range cases {
		to, hop := HierarchyBubbleToStatus(tc.kind, tc.status)
		if hop != tc.hop || to != tc.wantTo {
			t.Fatalf("%s %s: to=%q hop=%v, want to=%q hop=%v", tc.kind, tc.status, to, hop, tc.wantTo, tc.hop)
		}
	}
}
