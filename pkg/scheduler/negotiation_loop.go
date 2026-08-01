package scheduler

import (
	"context"
	"fmt"
)

// NegotiationLoop manages the iterative process of agent-negotiator policy interaction.
type NegotiationLoop struct {
	negotiator PolicyNegotiator
	maxRetries int
}

// NewNegotiationLoop creates a new negotiation loop
func NewNegotiationLoop(n PolicyNegotiator, maxRetries int) NegotiationLoopInterface {
	return &NegotiationLoop{
		negotiator: n,
		maxRetries: maxRetries,
	}
}

// Execute performs the dialogue.
func (nl *NegotiationLoop) Execute(ctx context.Context, initialProposal Proposal) (*NegotiationResult, error) {
	curr := initialProposal
	for i := 0; i <= nl.maxRetries; i++ {
		res, err := nl.negotiator.Propose(ctx, curr)
		if err != nil {
			return nil, err
		}
		if res.Accepted {
			return res, nil
		}

		if i == nl.maxRetries {
			return res, nil
		}
	}
	return nil, fmt.Errorf("negotiation failed after %d attempts", nl.maxRetries)
}
