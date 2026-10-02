package system

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/storage"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
)

// emitListingIndexBatchEventViaCoordinator emits listing-index batch processing events via the coordination system.
func emitListingIndexBatchEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProviderCas caspkg.CASFacade,
	kind string,
	batchSize int,
	duration time.Duration,
	status string,
	err error,
) {
	storageProvider := storageProviderCas.(storage.ObjectStorageProvider)
	ctx, coordinator, ok := setupSystemCoordinator(ctx, projectRoot, storageProvider, systemProfileSystem, true)
	if !ok {
		return
	}

	// Build audit metadata
	auditMetadata := make(map[string]any)
	auditMetadata[eventKeyEventType] = fmt.Sprintf("listing_index_batch_%s", status)
	auditMetadata[eventKeyOperation] = fmt.Sprintf("Listing index batch processing: %d updates for %s", batchSize, kind)
	auditMetadata[eventKeyTargetKind] = kind
	auditMetadata[eventKeyBatchSize] = batchSize
	auditMetadata[eventKeyDurationSeconds] = duration.Seconds()

	// Determine severity
	severity := severityLow
	if status == eventStatusError || err != nil {
		severity = severityHigh
	} else if status == eventStatusComplete {
		severity = severityMedium
	}
	auditMetadata[eventKeySeverity] = severity

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: eventKeyKind, Value: kind},
		{Key: eventKeyBatchSize, Value: batchSize},
		{Key: eventKeyStatus, Value: status},
	}
	if duration > 0 {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: eventKeyDurationSeconds, Value: duration.Seconds()})
	}

	// Build metrics data
	metricsData := make(map[string]any)
	metricsData[eventKeyKind] = kind
	metricsData[eventKeyBatchSize] = batchSize
	metricsData[eventKeyStatus] = status
	if duration > 0 {
		metricsData[eventKeyDurationSeconds] = duration.Seconds()
		metricsData[eventKeyDurationNS] = duration.Nanoseconds()
	}
	if err != nil {
		metricsData[eventKeyError] = err.Error()
	}

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   metricsData,
	}

	// Create operation ID
	operationID := fmt.Sprintf("listing_index_batch_%s_%d", kind, time.Now().UnixNano())

	// Create event context (enable audit and metrics, minimal logging)
	eventCtx := coordination.NewEventContext(operationID, "listing_index_batch", status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(false, true, true, false) // Audit and metrics, no logging/operational

	if err != nil {
		eventCtx = eventCtx.WithError(err)
	}

	if duration > 0 {
		eventCtx = eventCtx.WithDuration(duration)
	}

	// Emit via coordinator (async, non-blocking)
	builder := goroutinelabels.NewGoroutine("listing_index_event_emit", fmt.Sprintf("emitting listing index event for %s", kind))
	builder.StartSimple(func() {
		_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
	})
}

// emitListingIndexStateChangeEventViaCoordinator emits listing-index queue state change events via the coordination system.
func emitListingIndexStateChangeEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProviderCas caspkg.CASFacade,
	changeType string,
) {
	var storageProvider storage.ObjectStorageProvider
	if sp, ok := storageProviderCas.(storage.ObjectStorageProvider); ok {
		storageProvider = sp
	}
	ctx, coordinator, ok := setupSystemCoordinator(ctx, projectRoot, storageProvider, systemProfileSystem, true)
	if !ok {
		return
	}

	auditMetadata := make(map[string]any)
	auditMetadata[eventKeyEventType] = fmt.Sprintf("cas_index_state_%s", changeType)
	auditMetadata[eventKeyOperation] = fmt.Sprintf("CAS index queue state change: %s", changeType)
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

	operationID := fmt.Sprintf("listing_index_state_%s_%d", changeType, time.Now().UnixNano())

	eventCtx := coordination.NewEventContext(operationID, "listing_index_state_change", changeType).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(false, true, true, false)

	stateBuilder := goroutinelabels.NewGoroutine("listing_index_state_change_emit", fmt.Sprintf("emitting listing index state change %s", changeType))
	stateBuilder.StartSimple(func() {
		_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
	})
}
