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

// emitSchedulerHistoryEventViaCoordinator emits scheduler history query events via coordinator
func emitSchedulerHistoryEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	_ storagepkg.ObjectStorageProvider,
	operationID string,
	statsCount int,
	jobIDFilter string,
) {
	if projectRoot == emptyValue || projectRoot == "." {
		return
	}

	// Create coordinator with routers
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		AuditRouter:       nil, // History queries don't create audit events
		MetricsRouter:     nil,
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: "operation", Value: "scheduler_history_query"},
		{Key: "stats_count", Value: statsCount},
	}
	if jobIDFilter != emptyValue {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: "job_id_filter", Value: jobIDFilter})
	}

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: nil, // History queries don't create audit events
		MetricsData:   nil,
	}

	// Create event context
	eventCtx := coordination.NewEventContext(operationID, "scheduler_history_query", "complete").
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, false, false, false) // Logging only

	// Emit via coordinator (async, non-blocking)
	bud := goroutinelabels.DefaultBudget()
	historyBuilder := goroutinelabels.NewGoroutine("scheduler_event_emitter", "emitting scheduler history query event")
	if bud != nil {
		historyBuilder = historyBuilder.WithBudget(bud)
	}
	historyBuilder.StartSimple(func() {
		_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
	})
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
	if projectRoot == emptyValue || projectRoot == "." {
		return
	}

	// Embed LoggingContext in context so coordinator logging respects --context profile
	ctx = createContextWithLoggingProfile(ctx, profile)

	// Create coordinator with routers
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		AuditRouter:       nil, // Debug events don't create audit events
		MetricsRouter:     nil,
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: "operation", Value: operation},
		{Key: "event", Value: "Found audit events for scheduler jobs"},
		{Key: "filtered_count", Value: filteredCount},
		{Key: "raw_count", Value: rawCount},
	}

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: nil, // Debug events don't create audit events
		MetricsData:   nil,
	}

	// Create operation ID
	operationID := fmt.Sprintf("scheduler_history_%s_%d", operation, time.Now().UnixNano())

	// Create event context (debug status for verbose logging)
	eventCtx := coordination.NewEventContext(operationID, "scheduler_history", "debug").
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, false, false, false) // Logging only

	// Emit via coordinator (async, non-blocking)
	bud := goroutinelabels.DefaultBudget()
	activityBuilder := goroutinelabels.NewGoroutine("scheduler_event_emitter", "emitting scheduler activity query event")
	if bud != nil {
		activityBuilder = activityBuilder.WithBudget(bud)
	}
	activityBuilder.StartSimple(func() {
		_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
	})
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
	if projectRoot == emptyValue || projectRoot == "." {
		return
	}

	// Create coordinator with routers
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		AuditRouter:       nil, // Activity queries don't create audit events
		MetricsRouter:     nil,
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	// Build logging fields
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

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: nil, // Activity queries don't create audit events
		MetricsData:   nil,
	}

	// Determine status
	status := "complete"
	if daemonDown || stuckJobCount > 0 || missedTriggerCount > 0 {
		status = "warning"
	}

	// Create event context
	eventCtx := coordination.NewEventContext(operationID, "scheduler_activity_query", status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, false, false, false) // Logging only

	// Emit via coordinator (async, non-blocking)
	bud := goroutinelabels.DefaultBudget()
	activityBuilder := goroutinelabels.NewGoroutine("scheduler_event_emitter", "emitting scheduler activity query event")
	if bud != nil {
		activityBuilder = activityBuilder.WithBudget(bud)
	}
	activityBuilder.StartSimple(func() {
		_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
	})
}
