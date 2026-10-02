package scheduler

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func newSchedulerCoordinator() *coordination.Coordinator {
	return coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		AuditRouter:       nil,
		MetricsRouter:     nil,
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
}

func initSchedulerCoordination(ctx context.Context, projectRoot, profile string) (context.Context, *coordination.Coordinator, bool) {
	if projectRoot == emptyValue || projectRoot == "." {
		return ctx, nil, false
	}
	ctx = createContextWithLoggingProfile(ctx, profile)
	return ctx, newSchedulerCoordinator(), true
}

func emitCoordinatorEventAsync(ctx context.Context, coordinator *coordination.Coordinator, eventCtx *coordination.EventContext) {
	bud := goroutinelabels.DefaultBudget()
	activityBuilder := goroutinelabels.NewGoroutine("scheduler_event_emitter", "emitting scheduler activity query event")
	if bud != nil {
		activityBuilder = activityBuilder.WithBudget(bud)
	}
	activityBuilder.StartSimple(func() {
		if err := coordinator.Emit(ctx, eventCtx); err != nil {
			return
		}
	})
}

func emitSchedulerLoggingEvent(
	ctx context.Context,
	projectRoot string,
	operationID string,
	eventType string,
	status string,
	loggingFields []coordination.LoggingField,
) {
	if projectRoot == emptyValue || projectRoot == "." {
		return
	}
	coordinator := newSchedulerCoordinator()
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: nil,
		MetricsData:   nil,
	}
	eventCtx := coordination.NewEventContext(operationID, eventType, status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, false, false, false)
	emitCoordinatorEventAsync(ctx, coordinator, eventCtx)
}

// emitSchedulerHistoryEventViaCoordinator emits scheduler history query events via coordinator
func emitSchedulerHistoryEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	_ storagepkg.ObjectStorageProvider,
	operationID string,
	statsCount int,
	jobIDFilter string,
) {
	loggingFields := []coordination.LoggingField{
		{Key: "operation", Value: "scheduler_history_query"},
		{Key: "stats_count", Value: statsCount},
	}
	if jobIDFilter != emptyValue {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: "job_id_filter", Value: jobIDFilter})
	}
	emitSchedulerLoggingEvent(ctx, projectRoot, operationID, "scheduler_history_query", "complete", loggingFields)
}

// createContextWithLoggingProfile creates a context with LoggingContext embedded from profile string
// This ensures coordinator logging events respect --context profile settings
func createContextWithLoggingProfile(ctx context.Context, profile string) context.Context {
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}

	if profile == emptyValue {
		profile = string(pkgctx.ProfileHuman) // Default
	}

	// Convert profile string to LoggingProfile enum
	var loggingCtx *pkgctx.LoggingContext
	switch profile {
	case string(pkgctx.ProfileMCP):
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileMCP)
	case string(pkgctx.ProfileSystem):
		loggingCtx = pkgctx.NewSystemLoggingContext()
	case string(pkgctx.ProfileAIAgent):
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileAIAgent)
	case string(pkgctx.ProfileDebug):
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileDebug)
	case string(pkgctx.ProfileHuman), "":
		loggingCtx = pkgctx.NewHumanLoggingContext()
	default:
		loggingCtx = pkgctx.NewHumanLoggingContext()
	}

	return pkgctx.WithLoggingContext(ctx, loggingCtx)
}

// emitSchedulerHistoryDebugEventViaCoordinator emits debug events for scheduler history operations
func emitSchedulerHistoryDebugEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	_ storagepkg.ObjectStorageProvider,
	operation string,
	filteredCount int,
	rawCount int,
	profile string,
) {
	// Embed LoggingContext in context so coordinator logging respects --context profile
	ctx = createContextWithLoggingProfile(ctx, profile)

	loggingFields := []coordination.LoggingField{
		{Key: "operation", Value: operation},
		{Key: "event", Value: "Found audit events for scheduler jobs"},
		{Key: "filtered_count", Value: filteredCount},
		{Key: "raw_count", Value: rawCount},
	}

	operationID := fmt.Sprintf("scheduler_history_%s_%d", operation, time.Now().UnixNano())
	emitSchedulerLoggingEvent(ctx, projectRoot, operationID, "scheduler_history", "debug", loggingFields)
}

// emitSchedulerActivityEventViaCoordinator emits scheduler activity query events via coordinator
func emitSchedulerActivityEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	_ storagepkg.ObjectStorageProvider,
	operationID string,
	eventCount int,
	stuckJobCount int,
	missedTriggerCount int,
	daemonDown bool,
	jobIDFilter string,
) {
	loggingFields := []coordination.LoggingField{
		{Key: "operation", Value: "scheduler_activity_query"},
		{Key: "event_count", Value: eventCount},
		{Key: "stuck_job_count", Value: stuckJobCount},
		{Key: "missed_trigger_count", Value: missedTriggerCount},
		{Key: "daemon_down", Value: daemonDown},
	}
	if jobIDFilter != emptyValue {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: "job_id_filter", Value: jobIDFilter})
	}

	status := "complete"
	if daemonDown || stuckJobCount > 0 || missedTriggerCount > 0 {
		status = "warning"
	}

	emitSchedulerLoggingEvent(ctx, projectRoot, operationID, "scheduler_activity_query", status, loggingFields)
}
