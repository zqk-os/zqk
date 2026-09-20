package system

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// emitHashMismatchFixEventViaCoordinator emits hash mismatch fix events via the coordination system
func emitHashMismatchFixEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	objID, kind, relPath, originalHash, newHash string,
	profile string,
) error {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		// Best effort - skip if no project root
		return nil
	}

	// Embed LoggingContext in context so coordinator logging respects --context profile
	ctx = createContextWithLoggingProfile(ctx, profile)

	// Create routers for coordinator
	auditRouter := coordination.NewStorageAuditRouter(projectRoot, storageProvider)
	// Coordinator runs StorageAuditRouter synchronously only when a callback is set
	// (see pkg/coordination/coordinator_emit_pipeline.go). Without this, audit creation is
	// async and callers may list storage before CreateAuditEventWithBuilder completes.
	auditRouter.SetCallback(func(_ string, _ string, _ error) {})

	// Create coordinator with routers (only audit for hash mismatch fix events)
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		AuditRouter:       auditRouter,
		MetricsRouter:     nil, // Hash mismatch fix events don't need metrics router
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	// Build audit metadata
	auditMetadata := make(map[string]any)
	auditMetadata[eventKeyEventType] = "hash_mismatch_fix"
	auditMetadata[eventKeyOperation] = fmt.Sprintf("Regenerated integrity hash for %s (hash mismatch resolved)", objID)
	auditMetadata[eventKeySeverity] = severityHigh
	auditMetadata[eventKeyTargetKind] = kind
	auditMetadata[eventKeyTargetID] = objID
	if relPath != emptyValue {
		auditMetadata[objects.FieldKeyTargetPath] = relPath
	}
	auditMetadata[objects.FieldKeyOriginalValue] = originalHash
	auditMetadata[objects.FieldKeyNewValue] = newHash
	auditMetadata[objects.FieldKeyRecoveryMethod] = "force"
	auditMetadata[objects.FieldKeyCommand] = fmt.Sprintf("%s check --force", "zqk") // Use generic command name
	auditMetadata[objects.FieldKeyContext] = profile
	auditMetadata[eventKeyProjectRoot] = projectRoot

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: []coordination.LoggingField{
			{Key: eventKeyObjectID, Value: objID},
			{Key: eventKeyKind, Value: kind},
			{Key: eventKeyOperation, Value: "hash_mismatch_fix"},
		},
		AuditMetadata: auditMetadata,
		MetricsData:   nil, // Hash mismatch fix events don't create metrics
	}

	// Create operation ID (use wall clock; callers may put a timestamp in ctx with a typed key)
	operationID := fmt.Sprintf("hash_fix_%s_%d", objID, time.Now().UnixNano())

	// Create event context (only audit channel enabled)
	eventCtx := coordination.NewEventContext(operationID, "hash_mismatch_fix", eventStatusComplete).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(false, true, false, false) // Only audit, no logging/metrics/operational

	// Emit synchronously so audit persistence completes before callers flush/list (tests and CLI rely on this).
	return coordinator.Emit(ctx, eventCtx)
}
