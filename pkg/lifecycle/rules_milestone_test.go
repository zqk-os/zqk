package lifecycle

import "testing"

func TestDefaultTransitionRules_IncludesMilestoneCompletion(t *testing.T) {
	rules := DefaultTransitionRules()
	var found bool
	for _, r := range rules {
		if r.CriterionID != criterionAllCriteriaCompleteForMilestone {
			continue
		}
		found = true
		if r.Kind != kindMilestone {
			t.Fatalf("Kind: got %q want %q", r.Kind, kindMilestone)
		}
		if r.IDFromScope != scopeMilestoneID {
			t.Fatalf("IDFromScope: got %q want %q", r.IDFromScope, scopeMilestoneID)
		}
		if r.ToStatus != statusComplete {
			t.Fatalf("ToStatus: got %q", r.ToStatus)
		}
	}
	if !found {
		t.Fatal("expected all_criteria_complete_for_milestone transition rule")
	}
}

func TestTransitionRule_RuleMatch_MilestoneCriterion(t *testing.T) {
	r := TransitionRule{
		CriterionID: criterionAllCriteriaCompleteForMilestone,
		Kind:        kindMilestone,
		IDFromScope: scopeMilestoneID,
		ToStatus:    statusComplete,
	}
	scope := map[string]string{scopeMilestoneID: "MIL-test-1"}
	ev := &LifecycleEvent{Scope: scope}
	sk := ev.scopeKey()
	if !r.RuleMatch(criterionAllCriteriaCompleteForMilestone, sk, scope) {
		t.Fatal("RuleMatch should succeed for milestone scope")
	}
	if r.ObjectID(scope) != "MIL-test-1" {
		t.Fatalf("ObjectID: got %q", r.ObjectID(scope))
	}
}

func TestDefaultTransitionRules_IncludesMilestoneBacklogCompletion(t *testing.T) {
	rules := DefaultTransitionRules()
	var found bool
	for _, r := range rules {
		if r.CriterionID != criterionAllBacklogCompleteForMilestone {
			continue
		}
		found = true
		if r.Kind != kindMilestone {
			t.Fatalf("Kind: got %q want %q", r.Kind, kindMilestone)
		}
		if r.IDFromScope != scopeMilestoneID {
			t.Fatalf("IDFromScope: got %q want %q", r.IDFromScope, scopeMilestoneID)
		}
		if r.ToStatus != statusComplete {
			t.Fatalf("ToStatus: got %q", r.ToStatus)
		}
	}
	if !found {
		t.Fatal("expected all_backlog_items_complete_for_milestone transition rule")
	}
}

func TestTransitionRule_RuleMatch_MilestoneBacklogCriterion(t *testing.T) {
	r := TransitionRule{
		CriterionID: criterionAllBacklogCompleteForMilestone,
		Kind:        kindMilestone,
		IDFromScope: scopeMilestoneID,
		ToStatus:    statusComplete,
	}
	scope := map[string]string{scopeMilestoneID: "MIL-test-2"}
	ev := &LifecycleEvent{Scope: scope}
	sk := ev.scopeKey()
	if !r.RuleMatch(criterionAllBacklogCompleteForMilestone, sk, scope) {
		t.Fatal("RuleMatch should succeed for milestone backlog scope")
	}
	if r.ObjectID(scope) != "MIL-test-2" {
		t.Fatalf("ObjectID: got %q", r.ObjectID(scope))
	}
}
