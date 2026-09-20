package scheduler

import (
	"context"

	"github.com/zqk-os/zqk/pkg/accumulator"
)

// StartSupervisedAccumulators starts and supervises background WAL subscribers for all registered accumulators
// (including WhatsNextMaterializedView, StrategicReadinessView, DashboardState, etc.) in the scheduler daemon.
func StartSupervisedAccumulators(ctx context.Context, projectRoot string) []accumulator.WALSubscriber {
	return accumulator.StartAllSubscribers(ctx, projectRoot)
}
