package tde

import (
	"context"
	"testing"
)

func TestTransportEnforcer_StandardRisk(t *testing.T) {
	t.Parallel()
	enforcer := DefaultTransportEnforcer()
	env := &Envelope{
		ID:        "ENV-1",
		Kind:      "backlog_item",
		TargetID:  "ITEM-123",
		Operation: "update",
	}

	res, err := enforcer.EvaluateTransport(context.Background(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != GovernanceApproved {
		t.Fatalf("expected GovernanceApproved, got %s", res.Status)
	}
}

func TestTransportEnforcer_HighRiskBlocked(t *testing.T) {
	t.Parallel()
	enforcer := DefaultTransportEnforcer()
	env := &Envelope{
		ID:        "ENV-2",
		Kind:      "policy",
		TargetID:  "POLICY-SECURITY-001",
		Operation: "delete",
	}

	res, err := enforcer.EvaluateTransport(context.Background(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != GovernanceBlocked {
		t.Fatalf("expected GovernanceBlocked for high-risk policy mutation, got %s", res.Status)
	}
}

func TestTransportEnforcer_NilEnvelope(t *testing.T) {
	t.Parallel()
	enforcer := DefaultTransportEnforcer()
	_, err := enforcer.EvaluateTransport(context.Background(), nil)
	if err == nil {
		t.Fatalf("expected error evaluating nil envelope")
	}
}

func TestFastPathEvaluator(t *testing.T) {
	t.Parallel()
	eval := &FastPathEvaluator{}
	env := &Envelope{ID: "ENV-FAST", Kind: "backlog_item", TargetID: "ITEM-1"}
	res, err := eval.Evaluate(context.Background(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != GovernanceApproved {
		t.Fatalf("expected GovernanceApproved, got %s", res.Status)
	}
}

func TestSkillSigningEvaluator_UnsignedBlocked(t *testing.T) {
	t.Parallel()
	eval := &SkillSigningEvaluator{}
	env := &Envelope{ID: "ENV-SKILL", Kind: "agent_skill", TargetID: "ASKILL-1"}
	res, err := eval.Evaluate(context.Background(), env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != GovernanceBlocked {
		t.Fatalf("expected GovernanceBlocked for unsigned skill mutation, got %s", res.Status)
	}
}
