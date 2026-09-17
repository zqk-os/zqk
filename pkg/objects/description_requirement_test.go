package objects

import "testing"

func TestKindRequiresDescription(t *testing.T) {
	t.Parallel()

	required := []string{
		KindBacklogItem,
		KindRequirement,
		KindPriorityPlan,
		KindWorkstream,
		KindPersona,
		KindGoal,
		KindTechnicalDebt,
		KindAgentTask,
		KindCriteria,
		KindDecision,
		KindScenario,
		KindRoadmap,
		KindMilestone,
	}

	for _, k := range required {
		if !KindRequiresDescription(k) {
			t.Errorf("expected KindRequiresDescription(%q) to be true", k)
		}
	}

	notRequired := []string{
		"doc_entry",
		"glossary_term",
		"policy",
		"account",
		"qa_success",
		"test_case",
	}

	for _, k := range notRequired {
		if KindRequiresDescription(k) {
			t.Errorf("expected KindRequiresDescription(%q) to be false", k)
		}
	}
}
