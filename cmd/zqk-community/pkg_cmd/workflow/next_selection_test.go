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
		&convergenceInfo{ID: "CONV-1", Title: "active"},
		"PLAN-1",
		&backlogInfo{ID: "ITEM-1", Title: "work"},
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
		&convergenceInfo{ID: "CONV-2", Title: "active session"},
		"PLAN-2",
		&backlogInfo{ID: "ITEM-2", Title: "next work"},
	)
	if result.Decision != decisionActiveConvergence {
		t.Fatalf("decision: got %q", result.Decision)
	}
	if result.RecommendedCommand != "zqk object get CONV-2" {
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
		"PLAN-3",
		&backlogInfo{ID: "ITEM-3", Title: "implement thing"},
	)
	if result.Decision != decisionPriorityPlanBacklog {
		t.Fatalf("decision: got %q", result.Decision)
	}
	if result.RecommendedCommand != "zqk object get ITEM-3" {
		t.Fatalf("recommended command: %q", result.RecommendedCommand)
	}
	if len(result.Sources) != 2 {
		t.Fatalf("sources: got %d", len(result.Sources))
	}
}
