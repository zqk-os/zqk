package scheduler

import (
	"context"
	"time"

	caspkg "github.com/lanceman/zqk/pkg/storage/cas"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
)

// Log events for operation_execution handler (POL-CODE-007 stable keys).
const (
	LogEventOperationExecutionJobStart     = JobTypeOperationExecution + "_job_start"
	LogEventOperationExecutionProcessing   = JobTypeOperationExecution + "_processing"
	LogEventOperationExecutionJobCompleted = JobTypeOperationExecution + "_job_completed"
)

// OperationExecutionHandler handles operation execution jobs
type OperationExecutionHandler struct {
	storage storagepkg.ObjectStorageProvider
	logger  logging.Logger
}

// NewOperationExecutionHandler creates a new operation execution handler
// NewOperationExecutionHandler creates a new operation execution handler
func NewOperationExecutionHandler(storage storagepkg.ObjectStorageProvider) OperationExecutionHandlerInterface {
	return &OperationExecutionHandler{
		storage: storage,
		logger:  logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// Execute executes an operation (create, update, delete)
func (h *OperationExecutionHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	return RunOperationExecutionViaPipeline(ctx, h, job)
}

// executeOperationExecutionCore executes an operation (create, update, delete).
// Called from RunOperationExecutionViaPipeline NORMALIZE stage.
func (h *OperationExecutionHandler) executeOperationExecutionCore(ctx context.Context, job *ScheduledJob) error {
	SLog(h.logger).Info(LogEventOperationExecutionJobStart).
		JobID(job.ID).
		Log()

	// Get event data from job context (passed via TriggerJobByEvent)
	eventData := ctx.Value(evtDataKey{})
	if eventData == nil {
		return errfmt.Errorf("no event data provided for operation job")
	}

	eventDataMap, ok := eventData.(map[string]any)
	if !ok {
		return errfmt.Errorf("invalid event data format for operation job")
	}

	// Extract operation parameters
	operationType, _ := eventDataMap["operation_type"].(string)
	objectID, _ := eventDataMap["object_id"].(string)
	objectKind, _ := eventDataMap[objects.FieldKeyObjectKind].(string)
	data := eventDataMap["data"] // Keep as any for type assertion later

	if operationType == emptyValue || objectID == emptyValue || objectKind == emptyValue {
		return errfmt.Errorf("missing required operation parameters: operation_type, object_id, object_kind")
	}

	// Get security context from job context or use system context
	secCtx := ctx.Value(secJobCtxKey{})
	if secCtx == nil {
		// Use system context as fallback
		secCtx = pkgctx.NewSystemSecurityContext()
	}

	securityCtx, ok := secCtx.(*pkgctx.SecurityContext)
	if !ok {
		return errfmt.Errorf("invalid security context type")
	}

	SLog(h.logger).Info(LogEventOperationExecutionProcessing).
		String("operation_type", operationType).
		String("object_id", objectID).
		String("object_kind", objectKind).
		Log()

	// Execute operation based on type
	var err error
	switch operationType {
	case "create":
		if data == nil {
			return errfmt.Errorf("create operation requires data")
		}
		objData, ok := data.(map[string]any)
		if !ok {
			return errfmt.Errorf("create operation requires object data map")
		}
		err = h.storage.Create(ctx, securityCtx, objData)
	case "update":
		updates, ok := data.(map[string]any)
		if !ok {
			return errfmt.Errorf("update operation requires updates map")
		}
		err = h.storage.Update(ctx, securityCtx, objectID, updates)
	case "delete":
		cascade := false
		if cascadeVal, ok := eventDataMap["cascade"].(bool); ok {
			cascade = cascadeVal
		}
		err = h.storage.Delete(ctx, securityCtx, objectID, cascade)
	default:
		return errfmt.Errorf("unknown operation type: %s", operationType)
	}

	if err != nil {
		return errfmt.Newf("operation execution failed").Wrap(err)
	}

	// Flush the queue used by this handler's storage (per-project in tests, global in prod). See CAS_LIST_GET_CONSISTENCY.md.
	if objectKind == objects.KindSchedulerJob {
		projectRoot := ""
		if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
			projectRoot = fileStorage.GetProjectRoot()
		}
		if err := caspkg.GetListingIndexWriteQueueForProjectRoot(projectRoot).FlushKind(objects.KindSchedulerJob, 2*time.Second); err != nil {
			SLog(h.logger).Debug("Failed to flush listing index write queue for scheduler jobs").WithError(err).Log()
		}
	}

	SLog(h.logger).Info(LogEventOperationExecutionJobCompleted).
		JobID(job.ID).
		String("operation_type", operationType).
		String("object_id", objectID).
		Log()

	return nil
}
