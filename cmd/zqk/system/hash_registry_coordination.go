package system

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
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
	auditMetadata := map[string]any{
		eventKeyEventType:       fmt.Sprintf("hash_registry_batch_%s", status),
		eventKeyOperation:       fmt.Sprintf("Hash registry batch processing: %d requests, %d hashes for %s", batchSize, hashCount, kind),
		eventKeyTargetKind:      kind,
		eventKeyBatchSize:       batchSize,
		eventKeyHashCount:       hashCount,
		eventKeyDurationSeconds: duration.Seconds(),
		eventKeySeverity:        severityForStatusOrError(status, err),
	}

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: eventKeyKind, Value: kind},
		{Key: eventKeyBatchSize, Value: batchSize},
		{Key: eventKeyHashCount, Value: hashCount},
		{Key: eventKeyStatus, Value: status},
	}

	// Build metrics data
	metricsData := map[string]any{
		eventKeyKind:      kind,
		eventKeyBatchSize: batchSize,
		eventKeyHashCount: hashCount,
		eventKeyStatus:    status,
	}

	operationID := fmt.Sprintf("hash_registry_batch_%s_%d", kind, time.Now().UnixNano())
	emitSystemEventViaCoordinator(
		ctx,
		projectRoot,
		storageProvider,
		operationID,
		eventTypeHashRegistryBatch,
		status,
		duration,
		err,
		loggingFields,
		auditMetadata,
		metricsData,
		false,
		"hash_registry_event_emit",
		fmt.Sprintf("emitting hash registry event for %s", kind),
	)
}
