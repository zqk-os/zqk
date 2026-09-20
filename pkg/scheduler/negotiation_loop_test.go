package scheduler

import (
	"context"
	"testing"
)

func TestNegotiationLoop_Execute(t *testing.T) {
	t.Run("Accepts on first try", func(t *testing.T) {
		mock := &mockNegotiator{
			responses: []*NegotiationResult{{Accepted: true, Reason: "ok"}},
		}
		loop := NewNegotiationLoop(mock, 3)
		res, err := loop.Execute(context.Background(), Proposal{Type: ProposalThrottle})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.Accepted {
			t.Errorf("expected acceptance")
		}
	})

	t.Run("Reaches retry limit", func(t *testing.T) {
		mock := &mockNegotiator{
			responses: []*NegotiationResult{
				{Accepted: false, Reason: "try again"},
				{Accepted: false, Reason: "still no"},
			},
		}
		loop := NewNegotiationLoop(mock, 1)
		res, err := loop.Execute(context.Background(), Proposal{Type: ProposalThrottle})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Accepted {
			t.Errorf("expected rejection")
		}
	})
}

type mockNegotiator struct {
	responses []*NegotiationResult
	count     int
}

func (m *mockNegotiator) Propose(ctx context.Context, p Proposal) (*NegotiationResult, error) {
	if m.count >= len(m.responses) {
		return &NegotiationResult{Accepted: false, Reason: "out of responses"}, nil
	}
	res := m.responses[m.count]
	m.count++
	return res, nil
}
