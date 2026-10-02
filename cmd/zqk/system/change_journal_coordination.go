package system

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/storage"
)

// emitChangeJournalEventViaCoordinator emits change journal entry creation events via the coordination system
// This provides unified event routing for change tracking operations
func emitChangeJournalEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	operationType string,
	status string,
	changeType string,
	objectRef string,
	kind string,
	objectID string,
	duration time.Duration,
	err error,
) {
	// Build audit metadata
	auditMetadata := map[string]any{
		eventKeyEventType:       fmt.Sprintf("change_journal_%s", status),
		eventKeyOperation:       fmt.Sprintf("Change journal entry created: %s %s", changeType, objectRef),
		eventKeyTargetKind:      kind,
		eventKeyTargetID:        objectID,
		eventKeyChangeType:      changeType,
		eventKeyObjectRef:       objectRef,
		eventKeyDurationSeconds: duration.Seconds(),
		eventKeySeverity:        severityForStatusOrError(status, err),
	}

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: eventKeyChangeType, Value: changeType},
		{Key: eventKeyObjectRef, Value: objectRef},
		{Key: eventKeyKind, Value: kind},
		{Key: eventKeyObjectID, Value: objectID},
		{Key: eventKeyStatus, Value: status},
	}

	// Build metrics data
	metricsData := map[string]any{
		eventKeyChangeType: changeType,
		eventKeyKind:       kind,
		eventKeyStatus:     status,
	}

	emitSystemEventViaCoordinator(
		ctx,
		projectRoot,
		storageProvider,
		operationID,
		operationType,
		status,
		duration,
		err,
		loggingFields,
		auditMetadata,
		metricsData,
		true,
		"change_journal_event_emit",
		fmt.Sprintf("emitting change journal event: %s %s", changeType, objectRef),
	)
}
