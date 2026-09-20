package agentdelivery

import "context"

// NoopDeliverer records delivery without I/O (tests, dry-run).
type NoopDeliverer struct{}

// Name implements Deliverer.
func (NoopDeliverer) Name() string { return "noop" }

// Deliver implements Deliverer.
func (NoopDeliverer) Deliver(_ context.Context, p Prompt) (Result, error) {
	return Result{DeliveredTo: "noop:skipped"}, nil
}
