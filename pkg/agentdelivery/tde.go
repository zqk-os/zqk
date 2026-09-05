package agentdelivery

import (
	"context"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// TDEEnforcer wraps another Deliverer to ensure the prompt contains a valid,
// signed Trust-Domain Envelope (TDE) with Neurological Governance before delivery.
type TDEEnforcer struct {
	Next Deliverer
}

// Name implements Deliverer.
func (t *TDEEnforcer) Name() string {
	return "tde_enforcer"
}

// Deliver implements Deliverer. It intercepts the delivery to verify the TDE.
func (t *TDEEnforcer) Deliver(ctx context.Context, p Prompt) (Result, error) {
	if p.TDE == nil {
		return Result{}, errfmt.Errorf("tde_enforcer: ABORT: Neurological governance payload missing in TDE transport")
	}

	if err := p.TDE.Verify(); err != nil {
		return Result{}, errfmt.Newf("tde_enforcer: ABORT: TDE verification failed").Wrap(err)
	}

	return t.Next.Deliver(ctx, p)
}
