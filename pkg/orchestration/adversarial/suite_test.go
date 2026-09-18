package adversarial

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/orchestration"
	"github.com/zqk-os/zqk/pkg/scheduler"
)

// Adversarial testing: Policy Violation Test
// Goal: Ensure the orchestrator rejects an intent that violates active policies.
func TestPolicyViolation(t *testing.T) {
	// 1. Mock PolicyNegotiator to return rejection
	mockNegotiator := &rejectingNegotiator{}
	manager := orchestration.NewManager(mockNegotiator, nil, nil)

	// 2. Submit prohibited intent
	intent := orchestration.RawIntent{Signature: "malicious-intent"}
	_, err := manager.ProcessIntent(context.Background(), intent)

	// 3. Verify rejection
	if err == nil {
		t.Errorf("expected policy violation error, got nil")
	}
}

type rejectingNegotiator struct{}

func (n *rejectingNegotiator) Propose(ctx context.Context, p scheduler.Proposal) (*scheduler.NegotiationResult, error) {
	return &scheduler.NegotiationResult{Accepted: false, Reason: "policy violation"}, nil
}
