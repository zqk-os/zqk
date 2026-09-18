package transceiver

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/scheduler/transceiver/types"
)

// SchedulerBroker is a future enhancement that would use the scheduler itself
// to handle async routing by creating transient scheduler jobs.
// For now, use AsyncRouter which provides async execution with worker pools.
//
// Future implementation would:
// 1. Create transient scheduler jobs for routing tasks
// 2. Leverage scheduler's retry, timeout, and monitoring infrastructure
// 3. Use scheduler's job queue for guaranteed delivery
//
// This is kept as a placeholder for future scheduler integration.
type SchedulerBroker struct {
	router *Router
	logger logging.Logger
}

// NewSchedulerBroker creates a new scheduler-based broker (placeholder)
// Use AsyncRouter instead for current implementation
func NewSchedulerBroker(router *Router, logger logging.Logger) *SchedulerBroker {
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	return &SchedulerBroker{
		router: router,
		logger: logger,
	}
}

// RouteAsync routes a message (placeholder - use AsyncRouter instead)
//
//nolint:gocritic // Message passed by value to avoid shared mutation
func (sb *SchedulerBroker) RouteAsync(ctx context.Context, message types.Message) error {
	// Future: Create scheduler job for routing
	// For now, just route synchronously
	return sb.router.Route(ctx, message)
}

// RouteSync routes a message synchronously
//
//nolint:gocritic // Message passed by value to avoid shared mutation
func (sb *SchedulerBroker) RouteSync(ctx context.Context, message types.Message) error {
	return sb.router.Route(ctx, message)
}
