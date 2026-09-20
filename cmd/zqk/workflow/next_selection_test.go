package workflow

import (
	"testing"
)

func TestApplyDecision_PolicyInterruptPrecedence(t *testing.T) {
	t.Parallel()
	result := &workflowNextResult{
		Decision:           "none",
		Rationale:          "none",
		RecommendedCommand: "none",
	}
	applyDecision(
		result,
		&policyDecision{DedupeKey: "DED-1", Message: "ack me"},
		&convergenceInfo{ID: "CVS-1", Title: "active"},
		"PRI-1",
		&backlogInfo{ID: "BLI-1", Title: "work"},
	)
	if result.Decision != decisionCriticalPolicyInterrupt {
		t.Fatalf("decision: got %q", result.Decision)
	}
	if result.RecommendedCommand != "zqk system policy-interrupts ack --dedupe-key DED-1" {
		t.Fatalf("recommended command: %q", result.RecommendedCommand)
	}
}

func TestApplyDecision_ConvergencePrecedenceOverBacklog(t *testing.T) {
	t.Parallel()
	result := &workflowNextResult{
		Decision:           "none",
		Rationale:          "none",
		RecommendedCommand: "none",
	}
	applyDecision(
		result,
		nil,
		&convergenceInfo{ID: "CVS-2", Title: "active session"},
		"PRI-2",
		&backlogInfo{ID: "BLI-2", Title: "next work"},
	)
	if result.Decision != decisionActiveConvergence {
		t.Fatalf("decision: got %q", result.Decision)
	}
	if result.RecommendedCommand != "zqk object get CVS-2" {
		t.Fatalf("recommended command: %q", result.RecommendedCommand)
	}
}

func TestApplyDecision_BacklogFallback(t *testing.T) {
	t.Parallel()
	result := &workflowNextResult{
		Decision:           "none",
		Rationale:          "none",
		RecommendedCommand: "none",
	}
	applyDecision(
		result,
		nil,
		nil,
		"PRI-3",
		&backlogInfo{ID: "BLI-3", Title: "implement thing"},
	)
	if result.Decision != decisionPriorityPlanBacklog {
		t.Fatalf("decision: got %q", result.Decision)
	}
	if result.RecommendedCommand != "zqk object get BLI-3" {
		t.Fatalf("recommended command: %q", result.RecommendedCommand)
	}
	if len(result.Sources) != 2 {
		t.Fatalf("sources: got %d", len(result.Sources))
	}
}
