package system

import (
	"context"
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/metrics"
	"github.com/lanceman/zqk/pkg/storage"
)

// emitIOQueueStateChangeEventViaCoordinator emits I/O queue state change events via the coordination system.
func emitIOQueueStateChangeEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	changeType string,
) {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		return
	}

	ctx = createContextWithLoggingProfile(ctx, systemProfileSystem)

	auditRouter := coordination.NewStorageAuditRouter(projectRoot, storageProvider)

	var metricsRouter coordination.MetricsRouter
	if storageProvider != nil {
		metricsPipeline := metrics.MetricPipelineForProject(storageProvider, projectRoot)
		metricsRouter = coordination.NewMetricPipelineRouter(metricsPipeline)
	}

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		AuditRouter:       auditRouter,
		MetricsRouter:     metricsRouter,
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	auditMetadata := make(map[string]any)
	auditMetadata[eventKeyEventType] = fmt.Sprintf("io_queue_state_%s", changeType)
	auditMetadata[eventKeyOperation] = fmt.Sprintf("I/O queue state change: %s", changeType)
	auditMetadata[eventKeyChangeType] = changeType
	auditMetadata[eventKeySeverity] = severityLow

	loggingFields := []coordination.LoggingField{
		{Key: eventKeyChangeType, Value: changeType},
	}

	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   map[string]any{eventKeyChangeType: changeType},
	}

	operationID := fmt.Sprintf("io_queue_state_%s_%d", changeType, time.Now().UnixNano())

	eventCtx := coordination.NewEventContext(operationID, "io_queue_state_change", changeType).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(false, true, true, false)

	bud := goroutinelabels.DefaultBudget()
	builder := goroutinelabels.NewGoroutine("io_queue_state_change_emit", fmt.Sprintf("emitting I/O queue state change %s", changeType))
	if bud != nil {
		builder = builder.WithBudget(bud)
	}
	builder.StartSimple(func() {
		_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
	})
}
