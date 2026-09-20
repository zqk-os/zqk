package lifecycle

import "testing"

func TestDefaultTransitionRules_IncludesBacklogCompletionFromAcceptanceCriteria(t *testing.T) {
	rules := DefaultTransitionRules()
	var found bool
	for _, r := range rules {
		if r.CriterionID != criterionAllAcceptanceCriteriaMetForBacklogItem {
			continue
		}
		found = true
		if r.Kind != kindBacklogItem {
			t.Fatalf("Kind: got %q want %q", r.Kind, kindBacklogItem)
		}
		if r.IDFromScope != scopeBacklogItemID {
			t.Fatalf("IDFromScope: got %q want %q", r.IDFromScope, scopeBacklogItemID)
		}
		if r.ToStatus != statusComplete {
			t.Fatalf("ToStatus: got %q", r.ToStatus)
		}
	}
	if !found {
		t.Fatal("expected all_acceptance_criteria_met_for_backlog_item transition rule")
	}
}

func TestTransitionRule_RuleMatch_BacklogCriterion(t *testing.T) {
	r := TransitionRule{
		CriterionID: criterionAllAcceptanceCriteriaMetForBacklogItem,
		Kind:        kindBacklogItem,
		IDFromScope: scopeBacklogItemID,
		ToStatus:    statusComplete,
	}
	scope := map[string]string{scopeBacklogItemID: "BLI-test-1"}
	ev := &LifecycleEvent{Scope: scope}
	sk := ev.scopeKey()
	if !r.RuleMatch(criterionAllAcceptanceCriteriaMetForBacklogItem, sk, scope) {
		t.Fatal("RuleMatch should succeed for backlog scope")
	}
	if r.ObjectID(scope) != "BLI-test-1" {
		t.Fatalf("ObjectID: got %q", r.ObjectID(scope))
	}
}
