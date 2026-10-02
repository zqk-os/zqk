package system

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
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
	// Determine severity
	severity := severityLow
	if status == eventStatusError || err != nil {
		severity = severityHigh
	} else if status == eventStatusComplete {
		severity = severityMedium
	}

	auditMetadata := map[string]any{
		eventKeyEventType:       fmt.Sprintf("listing_index_batch_%s", status),
		eventKeyOperation:       fmt.Sprintf("Listing index batch processing: %d updates for %s", batchSize, kind),
		eventKeyTargetKind:      kind,
		eventKeyBatchSize:       batchSize,
		eventKeyDurationSeconds: duration.Seconds(),
		eventKeySeverity:        severity,
	}

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: eventKeyKind, Value: kind},
		{Key: eventKeyBatchSize, Value: batchSize},
		{Key: eventKeyStatus, Value: status},
	}

	// Build metrics data
	metricsData := map[string]any{
		eventKeyKind:      kind,
		eventKeyBatchSize: batchSize,
		eventKeyStatus:    status,
	}

	// Create operation ID
	operationID := fmt.Sprintf("listing_index_batch_%s_%d", kind, time.Now().UnixNano())
	emitSystemEventViaCoordinator(
		ctx,
		projectRoot,
		storageProvider,
		operationID,
		"listing_index_batch",
		status,
		duration,
		err,
		loggingFields,
		auditMetadata,
		metricsData,
		false,
		"listing_index_event_emit",
		fmt.Sprintf("emitting listing index event for %s", kind),
	)
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
	emitStateChangeEventViaCoordinator(
		ctx,
		projectRoot,
		storageProvider,
		fmt.Sprintf("cas_index_state_%s", changeType),
		fmt.Sprintf("CAS index queue state change: %s", changeType),
		changeType,
		"listing_index_state",
		"listing_index_state_change",
		"listing_index_state_change_emit",
		fmt.Sprintf("emitting listing index state change %s", changeType),
	)
}
