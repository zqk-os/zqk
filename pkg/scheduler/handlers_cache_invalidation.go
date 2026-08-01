package scheduler

import (
	"context"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/when"
)

// Log events for cache_invalidation handler (POLICY-CODE-007 stable keys). Wire strings use JobType prefix [JobTypeCacheInvalidation].
const (
	LogEventCacheInvalidationJobStart     = JobTypeCacheInvalidation + "_job_start"
	LogEventCacheInvalidationEntryFailed  = JobTypeCacheInvalidation + "_entry_failed"
	LogEventCacheInvalidationNoHandler    = JobTypeCacheInvalidation + "_no_handler"
	LogEventCacheInvalidationJobCompleted = JobTypeCacheInvalidation + "_job_completed"
)

// stewardInvalidateReasonMax caps detail stored in the steward enqueue queue (JSONL line size bound).
const stewardInvalidateReasonMax = 512

// stewardCacheInvalidateDetail builds job_id=<scheduler_job_id>|<reason> without duplicating the id in reason:
// keep a fixed prefix so envelope tick / operators can correlate; reason is truncated to fit.
func stewardCacheInvalidateDetail(jobID, reason string) string {
	jobID = strings.TrimSpace(jobID)
	reason = strings.TrimSpace(reason)
	prefix := "job_id=" + jobID + "|"
	if len(prefix) >= stewardInvalidateReasonMax {
		if len(prefix) > stewardInvalidateReasonMax {
			return prefix[:stewardInvalidateReasonMax]
		}
		return prefix
	}
	remain := stewardInvalidateReasonMax - len(prefix)
	if len(reason) <= remain {
		return prefix + reason
	}
	return prefix + reason[:remain]
}

// CacheInvalidationHandler handles cache invalidation jobs
type CacheInvalidationHandler struct {
	storage     storagepkg.ObjectStorageProvider
	projectRoot string
	logger      logging.Logger
}

// NewCacheInvalidationHandler creates a new cache invalidation handler.
// When storage is [*storagepkg.FileObjectStorage], projectRoot is resolved for data-cell steward enqueue (best-effort).
func NewCacheInvalidationHandler(storage storagepkg.ObjectStorageProvider) CacheInvalidationHandlerInterface {
	// Use GetLoggerFromProfile - it may panic in test environments, but that's acceptable
	// Tests should handle this by using a test-safe logger or catching panics
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	projectRoot := ""
	if fs, ok := storage.(*storagepkg.FileObjectStorage); ok {
		projectRoot = fs.GetProjectRoot()
	}
	return &CacheInvalidationHandler{
		storage:     storage,
		projectRoot: projectRoot,
		logger:      logger,
	}
}

// Execute executes cache invalidation
func (h *CacheInvalidationHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	return RunCacheInvalidationViaPipeline(ctx, h, job)
}

// executeCacheInvalidationCore executes cache invalidation (called from RunCacheInvalidationViaPipeline NORMALIZE stage).
func (h *CacheInvalidationHandler) executeCacheInvalidationCore(ctx context.Context, job *ScheduledJob) error {
	SLog(h.logger).Info(LogEventCacheInvalidationJobStart).
		JobID(job.ID).
		Log()

	// Get event data from job context (passed via TriggerJobByEvent)
	// Event data contains: object_ids, reason, type
	eventData := ctx.Value(evtDataKey{})
	if eventData == nil {
		return errfmt.Errorf("no event data provided for cache invalidation job")
	}

	eventDataMap, ok := eventData.(map[string]any)
	if !ok {
		return errfmt.Errorf("invalid event data format for cache invalidation job")
	}

	// Extract object IDs
	objectIDsInterface, ok := eventDataMap["object_ids"]
	if !ok {
		return errfmt.Errorf("missing object_ids in event data")
	}

	var objectIDs []string
	switch v := objectIDsInterface.(type) {
	case []string:
		objectIDs = v
	case []any:
		objectIDs = make([]string, 0, len(v))
		for _, id := range v {
			if idStr, ok := id.(string); ok {
				objectIDs = append(objectIDs, idStr)
			}
		}
	default:
		return errfmt.Errorf("invalid object_ids format: expected []string or []any")
	}

	// Extract reason (avoid embedding job.ID here — steward detail prefixes job_id= separately).
	reason, _ := eventDataMap[objects.FieldKeyReason].(string)
	if reason == emptyValue {
		reason = "cache_invalidation"
	}

	// Execute cache invalidation using existing handler
	// Note: The handler expects *CacheContext, but we need to invalidate multiple IDs
	// For now, we'll invalidate them one by one, or use a different approach
	// TODO: Update handler to support bulk invalidation
	cacheOperationHandler := storagepkg.GetCacheOperationHandler()
	when.When(func() bool { return cacheOperationHandler != nil }).Then(func() {
		for _, objectID := range objectIDs {
			cacheCtx := &pkgctx.CacheContext{
				Operation: pkgctx.CacheOperationInvalidate,
				OldID:     objectID,
			}
			if err := cacheOperationHandler(cacheCtx); err != nil {
				SLog(h.logger).Warn(LogEventCacheInvalidationEntryFailed).
					String("object_id", objectID).
					WithFields(logErrField(err)...).
					Log()
			}
		}
	}).OrElse(func() {
		SLog(h.logger).Warn(LogEventCacheInvalidationNoHandler).Log()
	}).Run()

	SLog(h.logger).Info(LogEventCacheInvalidationJobCompleted).
		JobID(job.ID).
		ObjectCount(len(objectIDs)).
		Log()

	// Data-cell steward (v1): enqueue stream-profile maintenance op for envelope tick to drain.
	if h.projectRoot != "" {
		detail := stewardCacheInvalidateDetail(job.ID, reason)
		if err := datacell.EnqueueStewardMaintenance(ctx, h.projectRoot, datacell.ProfileStream,
			datacell.MaintenanceOp{Name: datacell.MaintenanceOpInvalidateCache, Detail: detail}, h.logger); err != nil {
			SLog(h.logger).Warn("Failed to enqueue steward maintenance for cache invalidation").WithError(err).Log()
		}
	}

	return nil
}
