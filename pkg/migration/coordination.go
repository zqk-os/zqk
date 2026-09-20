package migration

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

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

// emitMigrationEventViaCoordinator emits migration events via the coordination system
// This provides unified event routing for migration operations
//
//nolint:unparam // Keep profile parameter for future call sites; currently always defaults to "human".
func emitMigrationEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	operationType string,
	status string,
	migrationID string,
	metadata map[string]any,
	err error,
	duration time.Duration,
	profile string, // CLI context profile for logging format (optional, defaults to "human")
) {
	if projectRoot == emptyValue || projectRoot == "." {
		// Best effort - skip if no project root
		return
	}

	// Embed LoggingContext in context so coordinator logging respects --context profile
	ctx = createContextWithLoggingProfile(ctx, profile)

	// Create routers for coordinator
	auditRouter := coordination.NewStorageAuditRouter(projectRoot, storageProvider)

	// Create metrics pipeline for metrics router (if storage provider available)
	var metricsRouter coordination.MetricsRouter
	if storageProvider != nil {
		metricsPipeline := metrics.MetricPipelineForProject(storageProvider, projectRoot)
		metricsRouter = coordination.NewMetricPipelineRouter(metricsPipeline)
	}

	// Create coordinator with routers
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		AuditRouter:       auditRouter,
		MetricsRouter:     metricsRouter,
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	// Build audit metadata
	auditMetadata := make(map[string]any)
	auditMetadata[objects.FieldKeyEventType] = fmt.Sprintf("migration_%s", status)
	auditMetadata[objects.FieldKeyOperation] = fmt.Sprintf("Migration: %s", operationType)
	auditMetadata[objects.FieldKeyTargetKind] = "migration"
	auditMetadata["migration_id"] = migrationID

	// Add metadata fields
	for k, v := range metadata {
		auditMetadata[k] = v
	}

	// Determine severity
	severity := "low"
	switch status {
	case objects.ObjectStatusError, objects.ObjectStatusFailed:
		severity = "high"
	case objects.ObjectStatusCompleted:
		severity = "medium"
	}
	auditMetadata[objects.FieldKeySeverity] = severity

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: "migration_id", Value: migrationID},
		{Key: "operation_type", Value: operationType},
		{Key: "status", Value: status},
	}
	if duration > 0 {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: "duration_seconds", Value: duration.Seconds()})
	}

	// Build metrics data
	metricsData := make(map[string]any)
	metricsData["migration_id"] = migrationID
	metricsData["operation_type"] = operationType
	if duration > 0 {
		metricsData[objects.FieldKeyDurationSeconds] = duration.Seconds()
	}

	// Add metadata to metrics
	for k, v := range metadata {
		metricsData[k] = v
	}

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   metricsData,
	}

	// Create event context (enable all channels for migrations)
	eventCtx := coordination.NewEventContext(operationID, operationType, status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, true, true, true) // All channels enabled for migration operations

	if err != nil {
		eventCtx = eventCtx.WithError(err)
	}

	if duration > 0 {
		eventCtx = eventCtx.WithDuration(duration)
	}

	// Emit via coordinator (async, non-blocking)
	goroutinelabels.NewGoroutine("migration_event_emitter", "emitting migration event").
		StartSimple(func() {
			_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
		})
}
