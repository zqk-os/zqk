package objects

import (
	"reflect"
	"testing"
)

func TestPromoteTransitionTargets(t *testing.T) {
	// Nil lifecycle or empty current
	if targets := PromoteTransitionTargets(nil, "active"); len(targets) != 0 {
		t.Errorf("expected empty targets for nil lifecycle, got %v", targets)
	}
	if targets := PromoteTransitionTargets(&Lifecycle{}, ""); len(targets) != 0 {
		t.Errorf("expected empty targets for empty current, got %v", targets)
	}

	lc := &Lifecycle{
		Transitions: []Transition{
			{From: "planned", To: "in_progress", Manual: true, Auto: false},
			{From: "planned", To: "archived", Manual: false, Auto: true},     // auto-only: excluded
			{From: "planned", To: "complete", Manual: true, Auto: true},      // dual: included
			{From: "*", To: "rejected", Manual: true, Auto: false},           // wildcard: included
			{From: "planned", To: ""},                                        // empty To: skipped
			{From: "in_progress", To: "complete", Manual: true, Auto: false}, // from mismatch: skipped
		},
	}

	targets := PromoteTransitionTargets(lc, "planned")
	if _, ok := targets["in_progress"]; !ok {
		t.Errorf("expected in_progress in targets")
	}
	if _, ok := targets["complete"]; !ok {
		t.Errorf("expected complete in targets")
	}
	if _, ok := targets["rejected"]; !ok {
		t.Errorf("expected rejected in targets")
	}
	if _, ok := targets["archived"]; ok {
		t.Errorf("archived should be excluded because it is auto-only")
	}
}

func TestTransitionIsAutoOnly(t *testing.T) {
	if !TransitionIsAutoOnly(Transition{Auto: true, Manual: false}) {
		t.Error("expected true for Auto && !Manual")
	}
	if TransitionIsAutoOnly(Transition{Auto: true, Manual: true}) {
		t.Error("expected false for Auto && Manual")
	}
	if TransitionIsAutoOnly(Transition{Auto: false, Manual: true}) {
		t.Error("expected false for !Auto && Manual")
	}
}

func TestTransitionClearFields(t *testing.T) {
	if got := TransitionClearFields(nil, "a", "b"); got != nil {
		t.Errorf("expected nil for nil lifecycle")
	}
	if got := TransitionClearFields(&Lifecycle{}, "", "b"); got != nil {
		t.Errorf("expected nil for empty from")
	}
	if got := TransitionClearFields(&Lifecycle{}, "a", ""); got != nil {
		t.Errorf("expected nil for empty to")
	}

	lc := &Lifecycle{
		Transitions: []Transition{
			{
				From:   "active",
				To:     "complete",
				Manual: true,
				SideEffects: []TransitionSideEffect{
					{Clear: "active_order"},
					{Clear: ""},
					{Clear: "assigned_to"},
				},
			},
			{
				From:   "active",
				To:     "archived",
				Auto:   true,
				Manual: false, // auto-only, skipped
				SideEffects: []TransitionSideEffect{
					{Clear: "some_field"},
				},
			},
		},
	}

	cleared := TransitionClearFields(lc, "active", "complete")
	want := []string{"active_order", "assigned_to"}
	if !reflect.DeepEqual(cleared, want) {
		t.Errorf("TransitionClearFields got %v, want %v", cleared, want)
	}

	// Auto-only edge returns nil
	if clearedAuto := TransitionClearFields(lc, "active", "archived"); clearedAuto != nil {
		t.Errorf("expected nil for auto-only edge, got %v", clearedAuto)
	}

	// Non-matching edge returns nil
	if clearedNone := TransitionClearFields(lc, "planned", "complete"); clearedNone != nil {
		t.Errorf("expected nil for non-matching edge, got %v", clearedNone)
	}
}
