package system

import (
	"context"
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/storage"
)

// emitAutoFixEventViaCoordinator emits auto-fix events via the coordination system
// This provides unified event routing for auto-fix operations during validation
func emitAutoFixEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	eventType string, // "auto_fix_applied", "auto_fix_stored", "auto_fix_collected", "auto_fix_conversion_issue"
	objectID string,
	objectKind string,
	fixes []string,
	message string,
	profile string,
) {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
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
		// Note: We don't create metrics for individual auto-fix events to avoid bloat
		// Batch auto-fix operations have their own metrics
		metricsRouter = nil
	}

	// Create coordinator with routers
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		AuditRouter:       auditRouter,
		MetricsRouter:     metricsRouter,
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: "operation_id", Value: operationID},
		{Key: "operation_type", Value: "auto_fix"},
		{Key: eventKeyEventType, Value: eventType},
		{Key: "object_id", Value: objectID},
		{Key: "kind", Value: objectKind},
		{Key: "fixes_count", Value: len(fixes)},
	}
	if len(fixes) > 0 {
		loggingFields = append(loggingFields, coordination.LoggingField{
			Key:   "fixes",
			Value: strings.Join(fixes, ", "),
		})
	}
	if message != emptyValue {
		loggingFields = append(loggingFields, coordination.LoggingField{
			Key:   "message",
			Value: message,
		})
	}

	// Build audit metadata (only for important events)
	auditMetadata := make(map[string]any)
	shouldAudit := eventType == "auto_fix_applied" || eventType == "auto_fix_conversion_issue"
	if shouldAudit {
		auditMetadata[eventKeyEventType] = eventTypeSystemConfigChange
		if message != emptyValue {
			auditMetadata[eventKeyOperation] = message
		} else {
			auditMetadata[eventKeyOperation] = fmt.Sprintf("Auto-fix %s for %s (%s)", eventType, objectID, objectKind)
		}
		auditMetadata[eventKeySeverity] = severityLow
		auditMetadata[eventKeyTargetKind] = objectKind
		auditMetadata[eventKeyTargetID] = objectID
		if len(fixes) > 0 {
			auditMetadata["fixes"] = fixes
			auditMetadata["fixes_count"] = len(fixes)
		}
	}

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   nil, // Individual auto-fix events don't create metrics (batches do)
	}

	// Determine status based on event type
	status := eventStatusComplete
	if eventType == "auto_fix_conversion_issue" {
		status = eventStatusWarning
	}

	// Create event context
	eventCtx := coordination.NewEventContext(operationID, "auto_fix", status).
		WithEventData(eventData).
		WithContext(ctx)

	// Enable channels based on event type
	// Applied/conversion issues: logging + audit (important events)
	// Stored/collected: logging only (debug/informational)
	if shouldAudit {
		eventCtx = eventCtx.WithChannels(true, true, false, true) // Logging, audit, operational (no metrics)
	} else {
		eventCtx = eventCtx.WithChannels(true, false, false, true) // Logging, operational only
	}

	// Emit via coordinator (async, non-blocking)
	goroutinelabels.NewGoroutine("auto_fix_event_emit", fmt.Sprintf("emitting auto-fix event for %s", objectID)).
		StartSimple(func() {
			_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
		})
}

// emitAutoFixAppliedViaCoordinator emits an event when auto-fix is applied during validation
func emitAutoFixAppliedViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	objectID string,
	objectKind string,
	fixes []string,
	profile string,
) {
	message := fmt.Sprintf("Auto-fix applied during validation: %d fix(es) for %s (%s)", len(fixes), objectID, objectKind)
	emitAutoFixEventViaCoordinator(
		ctx, projectRoot, storageProvider, operationID,
		"auto_fix_applied", objectID, objectKind, fixes, message, profile,
	)
}

// emitAutoFixStoredViaCoordinator emits an event when auto-fix results are stored in validation state metadata
func emitAutoFixStoredViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	objectID string,
	objectKind string,
	fixes []string,
	_ string, // metadataValue - kept for API compatibility but not currently used
	profile string,
) {
	message := fmt.Sprintf("Storing auto-fix results in validation state metadata for %s", objectID)
	emitAutoFixEventViaCoordinator(
		ctx, projectRoot, storageProvider, operationID,
		"auto_fix_stored", objectID, objectKind, fixes, message, profile,
	)
}

// emitAutoFixCollectedViaCoordinator emits an event when auto-fix results are collected from validation state
func emitAutoFixCollectedViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	objectID string,
	objectKind string,
	fixes []string,
	profile string,
) {
	message := fmt.Sprintf("Collected auto-fix results from validation state for %s", objectID)
	emitAutoFixEventViaCoordinator(
		ctx, projectRoot, storageProvider, operationID,
		"auto_fix_collected", objectID, objectKind, fixes, message, profile,
	)
}

// emitAutoFixConversionIssueViaCoordinator emits a warning when metadata contains auto_fixed but result.AutoFixed is empty
func emitAutoFixConversionIssueViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	objectID string,
	objectKind string,
	_ string, // metadataValue - kept for API compatibility but not currently used
	profile string,
) {
	message := fmt.Sprintf("Metadata contains auto_fixed but result.AutoFixed is empty (conversion issue) for %s", objectID)
	emitAutoFixEventViaCoordinator(
		ctx, projectRoot, storageProvider, operationID,
		"auto_fix_conversion_issue", objectID, objectKind, nil, message, profile,
	)
}
