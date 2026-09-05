package scheduler

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
)

// Log events for cascade_update handler (POL-CODE-007 stable keys).
const (
	LogEventCascadeUpdateJobStart     = JobTypeCascadeUpdate + "_job_start"
	LogEventCascadeUpdateProcessing   = JobTypeCascadeUpdate + "_processing"
	LogEventCascadeUpdateJobCompleted = JobTypeCascadeUpdate + "_job_completed"
)

// CascadeUpdateHandler handles cascade update jobs
type CascadeUpdateHandler struct {
	storage storagepkg.ObjectStorageProvider
	logger  logging.Logger
}

// NewCascadeUpdateHandler creates a new cascade update handler
// NewCascadeUpdateHandler creates a new cascade update handler
func NewCascadeUpdateHandler(storage storagepkg.ObjectStorageProvider) CascadeUpdateHandlerInterface {
	// Use GetLoggerFromProfile - it may panic in test environments, but that's acceptable
	// Tests should handle this by using a test-safe logger or catching panics
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	return &CascadeUpdateHandler{
		storage: storage,
		logger:  logger,
	}
}

// Execute executes cascade update
func (h *CascadeUpdateHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	return RunCascadeUpdateViaPipeline(ctx, h, job)
}

// executeCascadeUpdateCore executes cascade update (called from RunCascadeUpdateViaPipeline NORMALIZE stage).
func (h *CascadeUpdateHandler) executeCascadeUpdateCore(ctx context.Context, job *ScheduledJob) error {
	SLog(h.logger).Info(LogEventCascadeUpdateJobStart).
		JobID(job.ID).
		Log()

	// Get event data from job context (passed via TriggerJobByEvent)
	eventData := ctx.Value(evtDataKey{})
	if eventData == nil {
		return errfmt.Errorf("no event data provided for cascade update job")
	}

	eventDataMap, ok := eventData.(map[string]any)
	if !ok {
		return errfmt.Errorf("invalid event data format for cascade update job")
	}

	// Extract cascade update parameters
	parentID, _ := eventDataMap["parent_id"].(string)
	parentKind, _ := eventDataMap["parent_kind"].(string)
	cascadeType, _ := eventDataMap["cascade_type"].(string)

	if parentID == emptyValue || parentKind == emptyValue || cascadeType == emptyValue {
		return errfmt.Errorf("missing required cascade update parameters: parent_id, parent_kind, cascade_type")
	}

	// Extract dependent IDs
	var dependentIDs []string
	if dependentIDsInterface, ok := eventDataMap["dependent_ids"]; ok {
		switch v := dependentIDsInterface.(type) {
		case []string:
			dependentIDs = v
		case []any:
			dependentIDs = make([]string, 0, len(v))
			for _, id := range v {
				if idStr, ok := id.(string); ok {
					dependentIDs = append(dependentIDs, idStr)
				}
			}
		}
	}

	SLog(h.logger).Info(LogEventCascadeUpdateProcessing).
		String("parent_id", parentID).
		String("parent_kind", parentKind).
		String("cascade_type", cascadeType).
		Int("dependent_count", len(dependentIDs)).
		Log()

	secCtx := pkgctx.NewSystemSecurityContext()

	switch cascadeType {
	case "nullify", "set_null":
		// Unlink references from dependents using storage helper
		if err := storagepkg.UnlinkReferencesFromDependents(ctx, secCtx, h.storage, parentID, dependentIDs); err != nil {
			return errfmt.Errorf("failed to unlink references from dependents: %w", err)
		}
	case "delete":
		// Delete dependents recursively
		for _, depID := range dependentIDs {
			// Check if object still exists before deleting
			if _, err := h.storage.Read(ctx, secCtx, depID); err != nil {
				continue // Skip if already deleted
			}
			// Execute delete with cascade=true (recursive)
			if err := h.storage.Delete(ctx, secCtx, depID, true); err != nil {
				SLog(h.logger).Error(LogEventCascadeUpdateProcessing, err).
					String("dependent_id", depID).
					Log()
			}
		}
	default:
		return errfmt.Errorf("unknown cascade type: %s", cascadeType)
	}

	SLog(h.logger).Info(LogEventCascadeUpdateJobCompleted).
		JobID(job.ID).
		String("parent_id", parentID).
		Int("dependent_count", len(dependentIDs)).
		Log()

	return nil
}
