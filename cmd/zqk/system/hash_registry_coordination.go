package system

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/storage"
)

// emitHashRegistryEventViaCoordinator emits hash registry batch processing events via the coordination system
// This provides unified event routing for hash registry save operations
func emitHashRegistryEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	kind string,
	batchSize int,
	hashCount int,
	duration time.Duration,
	status string,
	err error,
) {
	ctx, coordinator, ok := setupSystemCoordinator(ctx, projectRoot, storageProvider, systemProfileSystem, true)
	if !ok {
		return
	}

	// Build audit metadata
	auditMetadata := make(map[string]any)
	auditMetadata[eventKeyEventType] = fmt.Sprintf("hash_registry_batch_%s", status)
	auditMetadata[eventKeyOperation] = fmt.Sprintf("Hash registry batch processing: %d requests, %d hashes for %s", batchSize, hashCount, kind)
	auditMetadata[eventKeyTargetKind] = kind
	auditMetadata[eventKeyBatchSize] = batchSize
	auditMetadata[eventKeyHashCount] = hashCount
	auditMetadata[eventKeyDurationSeconds] = duration.Seconds()

	// Determine severity
	severity := severityLow
	if status == eventStatusError || err != nil {
		severity = severityHigh
	}
	auditMetadata[eventKeySeverity] = severity

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: eventKeyKind, Value: kind},
		{Key: eventKeyBatchSize, Value: batchSize},
		{Key: eventKeyHashCount, Value: hashCount},
		{Key: eventKeyStatus, Value: status},
	}
	if duration > 0 {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: eventKeyDurationSeconds, Value: duration.Seconds()})
	}

	// Build metrics data
	metricsData := make(map[string]any)
	metricsData[eventKeyKind] = kind
	metricsData[eventKeyBatchSize] = batchSize
	metricsData[eventKeyHashCount] = hashCount
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
	operationID := fmt.Sprintf("hash_registry_batch_%s_%d", kind, time.Now().UnixNano())

	// Create event context (enable audit and metrics, minimal logging)
	eventCtx := coordination.NewEventContext(operationID, eventTypeHashRegistryBatch, status).
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
	bud := goroutinelabels.DefaultBudget()
	builder := goroutinelabels.NewGoroutine("hash_registry_event_emit", fmt.Sprintf("emitting hash registry event for %s", kind))
	if bud != nil {
		builder = builder.WithBudget(bud)
	}
	builder.StartSimple(func() {
		_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
	})
}
