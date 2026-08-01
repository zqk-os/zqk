package storage

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// SchedulerJobManager manages background operations using scheduler_job objects
type SchedulerJobManager struct {
	storage   ObjectStorageProvider
	scheduler SchedulerInterface
	logger    *logging.EventLogger
}

// SchedulerInterface defines the interface for scheduler operations
// This avoids direct dependency on the scheduler package
type SchedulerInterface interface {
	// TriggerJob manually triggers a job execution
	TriggerJob(ctx context.Context, jobID string) error

	// TriggerJobByEvent triggers a job based on an event
	TriggerJobByEvent(ctx context.Context, eventType, eventKind string, eventData map[string]any) error

	// TriggerJobByLifecycle triggers a job based on a lifecycle state transition
	TriggerJobByLifecycle(ctx context.Context, kind, fromState, toState string, objectData map[string]any) error
}

// NewSchedulerJobManager creates a new scheduler job manager
func NewSchedulerJobManager(storage ObjectStorageProvider, scheduler SchedulerInterface) *SchedulerJobManager {
	return &SchedulerJobManager{
		storage:   storage,
		scheduler: scheduler,
		logger:    logging.NewEventLogger(pkgctx.NewSystemContext()),
	}
}

// ScheduleCacheInvalidation schedules a cache invalidation job
// Creates or uses an existing scheduler_job for cache invalidation
func (sjm *SchedulerJobManager) ScheduleCacheInvalidation(
	ctx context.Context,
	objectIDs []string,
	reason string,
) error {
	if sjm.scheduler == nil {
		// No scheduler available, fall back to direct execution
		return sjm.executeCacheInvalidationDirect(ctx, objectIDs, reason)
	}

	// Create event data for cache invalidation
	eventData := map[string]any{
		"object_ids":           objectIDs,
		objects.FieldKeyReason: reason,
		objects.FieldKeyType:   ConstMiscCacheInvalidation,
	}

	// Trigger job by event (job should have trigger_type: event, event_type: cache_invalidation)
	err := sjm.scheduler.TriggerJobByEvent(ctx, ConstMiscCacheInvalidation, ConstMiscCacheOperation, eventData)
	if err != nil {
		// If event trigger fails, try to find and trigger a manual job
		jobID, findErr := sjm.findCacheInvalidationJob(ctx)
		if findErr == nil && jobID != emptyValue {
			// Trigger the job manually
			return sjm.scheduler.TriggerJob(ctx, jobID)
		}

		// Fall back to direct execution
		StorageLog(sjm.logger.Logger()).Warn(LogEventStorageSchedulerIntegrationCacheInvalidationScheduleFallbackWarn).
			WithError(err).
			Log()
		return sjm.executeCacheInvalidationDirect(ctx, objectIDs, reason)
	}

	return nil
}

// ScheduleCascadeUpdate schedules a cascade update job
// Creates or uses an existing scheduler_job for cascade updates
func (sjm *SchedulerJobManager) ScheduleCascadeUpdate(
	ctx context.Context,
	parentID string,
	parentKind string,
	cascadeType string, // "nullify", "set_null", "delete"
	dependentIDs []string,
) error {
	if sjm.scheduler == nil {
		// No scheduler available, fall back to direct execution
		return sjm.executeCascadeUpdateDirect(ctx, parentID, parentKind, cascadeType, dependentIDs)
	}

	// Create event data for cascade update
	eventData := map[string]any{
		"parent_id":          parentID,
		"parent_kind":        parentKind,
		"cascade_type":       cascadeType,
		"dependent_ids":      dependentIDs,
		objects.FieldKeyType: ConstMiscCascadeUpdate,
	}

	// Trigger job by event (job should have trigger_type: event, event_type: cascade_update)
	err := sjm.scheduler.TriggerJobByEvent(ctx, ConstMiscCascadeUpdate, ConstMiscCascadeOperation, eventData)
	if err != nil {
		// If event trigger fails, try to find and trigger a manual job
		jobID, findErr := sjm.findCascadeUpdateJob(ctx)
		if findErr == nil && jobID != emptyValue {
			// Trigger the job manually
			return sjm.scheduler.TriggerJob(ctx, jobID)
		}

		// Fall back to direct execution
		StorageLog(sjm.logger.Logger()).Warn(LogEventStorageSchedulerIntegrationCascadeUpdateScheduleFallbackWarn).
			WithError(err).
			Log()
		return sjm.executeCascadeUpdateDirect(ctx, parentID, parentKind, cascadeType, dependentIDs)
	}

	return nil
}

// ScheduleOperation schedules an operation job
// Creates or uses an existing scheduler_job for operation execution
func (sjm *SchedulerJobManager) ScheduleOperation(
	ctx context.Context,
	operationType string, // OpCreate, OpUpdate, or OpDelete
	objectID string,
	objectKind string,
	data map[string]any,
) error {
	if sjm.scheduler == nil {
		// No scheduler available, operation should be executed directly
		return errfmt.Errorf(ConstMiscSchedulerNotAvailableForOperationExecuti)
	}

	// Create event data for operation
	eventData := map[string]any{
		ConstMiscOperationType:     operationType,
		"object_id":                objectID,
		objects.FieldKeyObjectKind: objectKind,
		"data":                     data,
		objects.FieldKeyType:       "operation",
	}

	// Trigger job by event (job should have trigger_type: event, event_type: operation)
	err := sjm.scheduler.TriggerJobByEvent(ctx, "operation", ConstMiscOperationExecution, eventData)
	if err != nil {
		// If event trigger fails, try to find and trigger a manual job
		jobID, findErr := sjm.findOperationJob(ctx, operationType)
		if findErr == nil && jobID != emptyValue {
			// Trigger the job manually
			return sjm.scheduler.TriggerJob(ctx, jobID)
		}

		return errfmt.Newf(ConstMiscFailedToTriggerOperationJob).Wrap(err)
	}

	return nil
}

// findCacheInvalidationJob finds a scheduler_job for cache invalidation
func (sjm *SchedulerJobManager) findCacheInvalidationJob(ctx context.Context) (string, error) {
	// Query for scheduler_job with job_type that handles cache invalidation
	// This would typically be a job with job_type: "cache_invalidation" or similar
	// For now, return empty (job should be created separately)
	return "", nil
}

// findCascadeUpdateJob finds a scheduler_job for cascade updates
func (sjm *SchedulerJobManager) findCascadeUpdateJob(ctx context.Context) (string, error) {
	// Query for scheduler_job with job_type that handles cascade updates
	// This would typically be a job with job_type: "cascade_update" or similar
	// For now, return empty (job should be created separately)
	return "", nil
}

// findOperationJob finds a scheduler_job for operation execution
func (sjm *SchedulerJobManager) findOperationJob(ctx context.Context, operationType string) (string, error) {
	// Query for scheduler_job with job_type that handles operations
	// This would typically be a job with job_type: "operation_execution" or similar
	// For now, return empty (job should be created separately)
	return "", nil
}

// executeCacheInvalidationDirect executes cache invalidation directly (fallback)
//
//nolint:unparam // ctx and reason parameters are kept for API consistency
func (sjm *SchedulerJobManager) executeCacheInvalidationDirect(
	_ context.Context,
	objectIDs []string,
	_ string,
) error {
	// Use existing cache invalidation handler
	// Note: cacheOperationHandler expects CacheContext, not CacheInvalidationContext
	// We need to call handler for each object ID individually
	if cacheOperationHandler := GetCacheOperationHandler(); cacheOperationHandler != nil {
		// Invalidate each object ID individually
		for _, objectID := range objectIDs {
			cacheCtx := &pkgctx.CacheContext{
				Operation: pkgctx.CacheOperationInvalidate,
				OldID:     objectID,
			}
			if err := cacheOperationHandler(cacheCtx); err != nil {
				// Log but continue with other objects
				continue
			}
		}
	}
	return nil
}

// executeCascadeUpdateDirect executes cascade update directly (fallback)
func (sjm *SchedulerJobManager) executeCascadeUpdateDirect(
	ctx context.Context,
	parentID string,
	parentKind string,
	cascadeType string,
	dependentIDs []string,
) error {
	// Implementation depends on cascade update logic
	// Wire message aligns with scheduler cascade_update log family (POLICY-CODE-007); fallback when no scheduler.
	StorageLog(sjm.logger.Logger()).Info(LogEventStorageSchedulerIntegrationCascadeUpdateDirectInfo).
		String("parent_id", parentID).
		String("parent_kind", parentKind).
		String("cascade_type", cascadeType).
		Int(ConstMiscDependentCount, len(dependentIDs)).
		Log()

	secCtx := pkgctx.NewSystemSecurityContext()

	switch cascadeType {
	case "nullify", "set_null":
		// Unlink references from dependents
		return UnlinkReferencesFromDependents(ctx, secCtx, sjm.storage, parentID, dependentIDs)
	case "delete":
		// Delete dependents recursively
		for _, depID := range dependentIDs {
			// Check if object still exists before deleting
			if _, err := sjm.storage.Read(ctx, secCtx, depID); err != nil {
				continue // Skip if already deleted
			}
			// Execute delete with cascade=true (recursive)
			if err := sjm.storage.Delete(ctx, secCtx, depID, true); err != nil {
				// Log error but continue with other dependents
				StorageLog(sjm.logger.Logger()).Error(LogEventStorageSchedulerIntegrationCascadeUpdateDirectFailError, err).
					String("dependent_id", depID).
					Log()
			}
		}
	default:
		return errfmt.Errorf("unknown cascade type: %s", cascadeType)
	}

	return nil
}

// GetSchedulerInterface returns the scheduler interface from global scheduler getter
// This uses the same pattern as getGlobalSchedulerFunc in object_storage_file.go
var getGlobalSchedulerInterface func() SchedulerInterface

// SetGlobalSchedulerInterface sets the function to get the global scheduler interface
func SetGlobalSchedulerInterface(getter func() SchedulerInterface) {
	getGlobalSchedulerInterface = getter
}

// GetSchedulerJobManager creates a scheduler job manager using the global scheduler
func GetSchedulerJobManager(storage ObjectStorageProvider) *SchedulerJobManager {
	var scheduler SchedulerInterface
	if getGlobalSchedulerInterface != nil {
		scheduler = getGlobalSchedulerInterface()
	}
	return NewSchedulerJobManager(storage, scheduler)
}
