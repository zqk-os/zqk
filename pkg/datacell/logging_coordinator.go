package datacell

import (
	"context"

	"github.com/lanceman/zqk/pkg/logging"
)

// LoggingCellCoordinator implements [CellCoordinator] by emitting structured logs for each
// maintenance enqueue, then optionally delegating to [CellCoordinator] Inner (e.g. [NoopCellCoordinator],
// tests, or scheduler StewardEnqueueCoordinator wrapped as Inner for JSONL persistence — see pkg/scheduler).
// Keeps stewardship operations observable when Inner is nil (POL-CODE-007 — use logger, not fmt).
type LoggingCellCoordinator struct {
	Logger logging.Logger
	// Inner receives the same op after logging; nil means log-only (same as no-op downstream).
	Inner CellCoordinator
	// Profile is optional context for logs (which cell profile enqueued).
	Profile StorageProfile
}

// Enqueue implements [CellCoordinator].
func (c *LoggingCellCoordinator) Enqueue(ctx context.Context, op MaintenanceOp) error {
	if c == nil {
		return nil
	}
	if c.Logger != nil {
		entry := logging.Fluent(c.Logger).Info(LogEventDataCellMaintenanceEnqueue).
			Op(op.Name).
			OpDetail(op.Detail)
		if c.Profile != "" {
			entry = entry.StorageProfileName(string(c.Profile))
		}
		entry.Log()
	}
	if c.Inner == nil {
		_ = ctx
		return nil
	}
	return c.Inner.Enqueue(ctx, op)
}
