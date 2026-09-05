package scheduler

import (
	"context"

	"github.com/lanceman/zqk/pkg/entitlements"
)

// ProposalType defines the category of policy adjustment.
type ProposalType string

const (
	ProposalDefer    ProposalType = "Defer"
	ProposalOverride ProposalType = "Override"
	ProposalThrottle ProposalType = "Throttle"
)

// Proposal represents an agent's request to modify policy for a specific scope.
type Proposal struct {
	AgentID     string
	TargetJobID string
	Type        ProposalType
	Metadata    map[string]any
}

// NegotiationResult holds the outcome of a negotiation attempt.
type NegotiationResult struct {
	Accepted bool
	Reason   string
}

// PolicyNegotiator defines the interface for actor-based policy negotiation.
type PolicyNegotiator interface {
	Propose(ctx context.Context, p Proposal) (*NegotiationResult, error)
}

const (
	MetadataKeyUntil = "until"
)

// Negotiator orchestrates the policy adjustment based on agent input.
type Negotiator struct {
	engine PolicyEngineInterface
}

// NewNegotiator creates a new negotiator
func NewNegotiator(engine PolicyEngineInterface) NegotiatorInterface {
	return &Negotiator{engine: engine}
}

func (n *Negotiator) Propose(ctx context.Context, p Proposal) (*NegotiationResult, error) {
	switch p.Type {
	case ProposalDefer:
		val, ok := p.Metadata[MetadataKeyUntil]
		if !ok || val == "" {
			return &NegotiationResult{Accepted: false, Reason: "missing required metadata: until"}, nil
		}
		// In a real system, we'd persist this deferral to the Knowledge Kernel.
		return &NegotiationResult{Accepted: true, Reason: "deferral registered"}, nil

	case ProposalOverride:
		// Check entitlement
		if err := entitlements.CheckEvolveEntitlement(ctx, 1); err != nil {
			return &NegotiationResult{Accepted: false, Reason: err.Error()}, nil
		}
		return &NegotiationResult{Accepted: true, Reason: "authorized override"}, nil

	case ProposalThrottle:
		// Implementation would integrate with a shared resource registry.
		return &NegotiationResult{Accepted: true, Reason: "throttling policy updated"}, nil

	default:
		return &NegotiationResult{Accepted: false, Reason: "unknown proposal type"}, nil
	}
}
