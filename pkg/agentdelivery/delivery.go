package agentdelivery

import (
	"context"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/policy"
)

// Prompt is the payload to hand to an agent surface (IDE, API, queue).
// Markdown is typically the body from zqk scheduler convergence measure --format agent-prompt.
type Prompt struct {
	Markdown  []byte
	SessionID string
	Format    string // e.g. "agent-prompt"
	// DestPath is set when a deliverer writes to disk (file, stable handoff path).
	DestPath string
	// TDE is the Trust-Domain Envelope providing Neurological Governance rules.
	TDE *policy.TrustDomainEnvelope
}

// Result describes where the prompt landed for audit logs (CRIT-AO-002).
type Result struct {
	DeliveredTo    string // human-readable, e.g. "file:/path/to/prompt.md"
	HTTPStatusCode int    // set by HTTPDeliverer on success (2xx)
}

// Deliverer sends a prompt to one consumer (file, clipboard bridge, HTTP, MCP, etc.).
type Deliverer interface {
	Name() string
	Deliver(ctx context.Context, p Prompt) (Result, error)
}

// Composite runs deliverers in order; stops on first error.
type Composite struct {
	Steps []Deliverer
}

// Name implements Deliverer.
func (c Composite) Name() string { return "composite" }

// Deliver implements Deliverer.
func (c Composite) Deliver(ctx context.Context, p Prompt) (Result, error) {
	var last Result
	for i, step := range c.Steps {
		if step == nil {
			return last, errfmt.Errorf("agentdelivery: composite step %d is nil", i)
		}
		r, err := step.Deliver(ctx, p)
		if err != nil {
			return last, errfmt.Newf("agentdelivery: step %s", step.Name()).Wrap(err)
		}
		last = r
	}
	return last, nil
}
