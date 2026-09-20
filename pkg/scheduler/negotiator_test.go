package scheduler

import (
	"context"
	"testing"
)

func TestPolicyNegotiator_Propose_Defer(t *testing.T) {
	negotiator := NewNegotiator(NewPolicyEngine(nil, "", nil))

	t.Run("Valid deferral", func(t *testing.T) {
		res, err := negotiator.Propose(context.Background(), Proposal{
			AgentID:     "agent-1",
			TargetJobID: "job-1",
			Type:        ProposalDefer,
			Metadata:    map[string]any{"until": "2026-01-01T00:00:00Z"},
		})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !res.Accepted {
			t.Errorf("expected acceptance, got rejection: %s", res.Reason)
		}
	})

	t.Run("Invalid deferral metadata", func(t *testing.T) {
		res, err := negotiator.Propose(context.Background(), Proposal{
			AgentID:     "agent-1",
			TargetJobID: "job-1",
			Type:        ProposalDefer,
			Metadata:    map[string]any{},
		})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if res.Accepted {
			t.Errorf("expected rejection, got acceptance")
		}
	})
}

func TestPolicyNegotiator_Propose_Override(t *testing.T) {
	negotiator := NewNegotiator(NewPolicyEngine(nil, "", nil))

	t.Run("Override with permissions", func(t *testing.T) {
		// Mock implementation would check permissions in engine
		res, err := negotiator.Propose(context.Background(), Proposal{
			AgentID:     "admin-agent",
			TargetJobID: "job-1",
			Type:        ProposalOverride,
		})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		// Based on business logic to be added
		if !res.Accepted {
			t.Errorf("expected acceptance for admin agent")
		}
	})
}
