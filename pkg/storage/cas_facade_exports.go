package storage

import (
	"github.com/lanceman/zqk/pkg/objects"
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"

	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

func (f *FileObjectStorage) CASUsesContentAddressableStorage(kind string) bool {
	return f.usesContentAddressableStorage(kind)
}

func (f *FileObjectStorage) CASGenerateID(ctx context.Context, kind string) (string, error) {
	return f.generateID(ctx, kind)
}

type shutdownCoordinatorWrapper struct{}

func (w *shutdownCoordinatorWrapper) RegisterQueue(h caspkg.QueueShutdownHandler) {
	if c := GetGlobalShutdownCoordinator(); c != nil {
		c.RegisterQueue(&queueShutdownWrapper{h: h})
	}
}
func (w *shutdownCoordinatorWrapper) IsShutdownInitiated() bool {
	if c := GetGlobalShutdownCoordinator(); c != nil {
		return c.IsShutdownInitiated()
	}
	return false
}

type queueShutdownWrapper struct {
	h caspkg.QueueShutdownHandler
}

func (w *queueShutdownWrapper) GetName() string                 { return w.h.GetName() }
func (w *queueShutdownWrapper) InitiateShutdown() error         { return w.h.InitiateShutdown() }
func (w *queueShutdownWrapper) Drain(ctx context.Context) error { return w.h.Drain(ctx) }
func (w *queueShutdownWrapper) IsDrained() bool                 { return w.h.IsDrained() }
func (w *queueShutdownWrapper) GetPendingCount() int64          { return w.h.GetPendingCount() }
func (w *queueShutdownWrapper) IsCritical() bool                { return w.h.IsCritical() }

type validationStrategyWrapper struct {
	strategy ValidationStrategy
}

func (w *validationStrategyWrapper) ValidateMappings(kindDir string, mappings map[string]string, bucketKeys map[string]string) (map[string]string, map[string]string, int) {
	return w.strategy.ValidateMappings(kindDir, mappings, bucketKeys)
}

type validationRegistryWrapper struct{}

func (r *validationRegistryWrapper) GetStrategy(kind string) caspkg.ValidationStrategy {
	return &validationStrategyWrapper{strategy: GetGlobalValidationStrategyRegistry().GetStrategy(kind)}
}

func init() {
	caspkg.GlobalNewWaitGroupManager = func() caspkg.WaitGroupManager { return NewWaitGroupManager() }
	caspkg.GlobalShutdownCoordinator = &shutdownCoordinatorWrapper{}
	caspkg.GlobalValidationRegistry = &validationRegistryWrapper{}
	caspkg.GlobalGetValidationMetrics = func(kind string) caspkg.ValidationMetrics { return GetValidationMetrics(kind) }

	caspkg.SetAuditCreator(func(ctx context.Context, secCtx *pkgctx.SecurityContext, projectRoot string, storage caspkg.CASFacade, batchSize, successCount, failureCount int, durationMs int64, failedFiles []string) {
		if provider, ok := storage.(ObjectStorageProvider); ok {
			eventType := ConstStreamOrphanCleanupComplete
			if failureCount > 0 {
				eventType = ConstStreamOrphanCleanupError
			}
			metadata := map[string]any{
				objects.FieldKeySource:       ConstStreamBackgroundWorker,
				objects.FieldKeyBatchSize:    batchSize,
				objects.FieldKeySuccessCount: successCount,
				objects.FieldKeyFailureCount: failureCount,
				"duration_ms":                durationMs,
				"operation_type":             ConstStreamOrphanCleanup,
			}
			if len(failedFiles) > 0 {
				metadata["failed_files"] = failedFiles
			}
			options := &AuditEventOptions{
				EventType:  eventType,
				Operation:  "cleaned up orphaned cas files",
				Severity:   "low",
				TargetKind: ConstStreamCasOrphanCleanup,
				TargetID:   "batch",
				Metadata:   metadata,
				CreatedBy:  secCtx.AccountID,
			}
			_ = CreateAuditEventWithBuilder(ctx, projectRoot, secCtx, provider, options)
		}
	})
}
