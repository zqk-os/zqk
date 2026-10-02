package system

import (
	"context"
	"time"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// buildAuditMetadataFromOptions extracts and merges metadata fields from storage.AuditEventOptions.
func buildAuditMetadataFromOptions(options *storage.AuditEventOptions) map[string]any {
	auditMetadata := make(map[string]any)
	if options == nil {
		return auditMetadata
	}
	mergeMetadata(auditMetadata, options.Metadata)
	auditMetadata[eventKeyEventType] = options.EventType
	auditMetadata[eventKeyOperation] = options.Operation
	auditMetadata[eventKeySeverity] = options.Severity
	auditMetadata[eventKeyTargetKind] = options.TargetKind
	if options.TargetID != emptyValue {
		auditMetadata[eventKeyTargetID] = options.TargetID
	}
	if options.TargetPath != emptyValue {
		auditMetadata[objects.FieldKeyTargetPath] = options.TargetPath
	}
	return auditMetadata
}

// appendDurationLoggingField adds duration in seconds to logging fields if positive.
func appendDurationLoggingField(fields []coordination.LoggingField, duration time.Duration) []coordination.LoggingField {
	if duration > 0 {
		return append(fields, coordination.LoggingField{Key: eventKeyDurationSeconds, Value: duration.Seconds()})
	}
	return fields
}

// buildCoordinationEventData constructs EventData with logging fields, audit metadata, and metrics populated with duration and error.
func buildCoordinationEventData(
	loggingFields []coordination.LoggingField,
	auditMetadata map[string]any,
	metricsData map[string]any,
	duration time.Duration,
	err error,
) *coordination.EventData {
	loggingFields = appendDurationLoggingField(loggingFields, duration)
	if metricsData != nil {
		if duration > 0 {
			metricsData[eventKeyDurationSeconds] = duration.Seconds()
			metricsData[eventKeyDurationNS] = duration.Nanoseconds()
		}
		if err != nil {
			metricsData[eventKeyError] = err.Error()
		}
	}
	return &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   metricsData,
	}
}

// createContextWithLoggingProfile delegates to the canonical cli.CreateContextWithLoggingProfile.
func createContextWithLoggingProfile(ctx context.Context, profile string) context.Context {
	return cli.CreateContextWithLoggingProfile(ctx, profile)
}

// setupSystemCoordinator bootstraps a context and coordinator for system operations.
// Returns (ctx, coordinator, true) if projectRoot is non-empty, or (ctx, nil, false) otherwise.
func setupSystemCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	profile string,
	withMetrics bool,
) (context.Context, *coordination.Coordinator, bool) {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		return ctx, nil, false
	}

	ctx = createContextWithLoggingProfile(ctx, profile)
	auditRouter := coordination.NewAuditRouter(projectRoot, storageProvider)

	var metricsRouter coordination.MetricsRouter
	if withMetrics && storageProvider != nil {
		metricsPipeline := metrics.MetricPipelineForProject(storageProvider, projectRoot)
		metricsRouter = coordination.NewMetricPipelineRouter(metricsPipeline)
	}

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		AuditRouter:       auditRouter,
		MetricsRouter:     metricsRouter,
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	return ctx, coordinator, true
}

// severityForStatusOrError returns severityHigh when status is error or err is non-nil, otherwise severityLow.
func severityForStatusOrError(status string, err error) string {
	if status == eventStatusError || err != nil {
		return severityHigh
	}
	return severityLow
}

// buildEventContext constructs a configured coordination.EventContext.
func buildEventContext(
	ctx context.Context,
	operationID string,
	operationType string,
	status string,
	eventData *coordination.EventData,
	duration time.Duration,
	err error,
	logging bool,
	audit bool,
	metrics bool,
	operational bool,
) *coordination.EventContext {
	eventCtx := coordination.NewEventContext(operationID, operationType, status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(logging, audit, metrics, operational)
	if err != nil {
		eventCtx = eventCtx.WithError(err)
	}
	if duration > 0 {
		eventCtx = eventCtx.WithDuration(duration)
	}
	return eventCtx
}

// emitAsyncCoordinationEvent dispatches an event via coordinator inside a budgeted goroutine.
func emitAsyncCoordinationEvent(
	ctx context.Context,
	coordinator coordination.EventCoordinator,
	goroutineName string,
	goroutineDesc string,
	eventCtx *coordination.EventContext,
) {
	if coordinator == nil || eventCtx == nil {
		return
	}
	bud := goroutinelabels.DefaultBudget()
	builder := goroutinelabels.NewGoroutine(goroutineName, goroutineDesc)
	if bud != nil {
		builder = builder.WithBudget(bud)
	}
	builder.StartSimple(func() {
		if emitErr := coordinator.Emit(ctx, eventCtx); emitErr != nil {
			return
		}
	})
}
