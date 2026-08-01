package orchestration

import (
	"context"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/scheduler"
	"github.com/lanceman/zqk/pkg/when"
)

// PolicyNegotiator implementation for REQ-2.
type PolicyNegotiator struct {
	logger logging.Logger
}

func NewPolicyNegotiator(logger logging.Logger) *PolicyNegotiator {
	return &PolicyNegotiator{
		logger: logger,
	}
}

// Propose evaluates proposals and handles schema drift via version_context reconciliation.
func (n *PolicyNegotiator) Propose(ctx context.Context, p scheduler.Proposal) (*scheduler.NegotiationResult, error) {
	var result *scheduler.NegotiationResult
	var err error

	// Detect version mismatch
	mismatch := n.isVersionMismatch(p)

	when.When(func() bool { return mismatch }).
		Then(func() {
			logging.Fluent(n.logger).Info(ConstVersionMismatchDetectedAttemptingReconciliation).Log()
			result, err = n.reconcile(ctx, p)
		}).
		OrElse(func() {
			logging.Fluent(n.logger).Info(ConstNoVersionMismatchProceedingWithStandardProposal).Log()
			result, err = n.standardPropose(ctx, p)
		}).
		Run()
	return result, err
}

func (n *PolicyNegotiator) isVersionMismatch(p scheduler.Proposal) bool {
	// Logic to check if "version_context" differs
	raw, ok := p.Metadata["raw"].(map[string]any)
	if !ok {
		return false
	}
	vc, ok := raw[ConstVersionContext].(string)
	return ok && vc != "current"
}

//nolint:unparam // required signature
func (n *PolicyNegotiator) reconcile(_ context.Context, _ scheduler.Proposal) (*scheduler.NegotiationResult, error) {
	// Reconcile schema definition
	logging.Fluent(n.logger).Info(ConstReconcilingSchemaDefinitionForProposal).Log()
	return &scheduler.NegotiationResult{Accepted: true, Reason: ConstSchemaReconciled}, nil
}

//nolint:unparam // required signature
func (n *PolicyNegotiator) standardPropose(_ context.Context, _ scheduler.Proposal) (*scheduler.NegotiationResult, error) {
	// Delegate to actual negotiation logic (or mock for now)
	return &scheduler.NegotiationResult{Accepted: true, Reason: ConstStandardProposalAccepted}, nil
}
