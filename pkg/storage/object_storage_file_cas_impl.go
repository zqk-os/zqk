package storage

import (
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"context"
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/storage/filecas"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/crud"
)

// usesContentAddressableStorage checks if a kind uses content-addressable (hash-named) storage.
// All kinds are CAS-based; there is no ID-based storage. Objects are always stored with hash filenames.
//
//nolint:unparam // Always returns true - all kinds use CAS
func (f *FileObjectStorage) usesContentAddressableStorage(_ string) bool {
	return true
}

// ClearCASCaches clears the CAS caches. Used in tests where the same FileObjectStorage
// instance checks files created by a different process.
func (f *FileObjectStorage) ClearCASCaches() {
	if f.casCache != nil {
		f.casCache.Clear()
	}
}

// InvalidateCASCacheForKind clears the CAS cache for a specific kind. Used to keep the CAS index cache fresh
// when objects are created, updated, or deleted, preventing stale index reads.
func (f *FileObjectStorage) InvalidateCASCacheForKind(kind string) {
	if f.casCache != nil {
		f.casCache.Delete(kind)
	}
}

// GetContentAddressableStorage gets or creates a filecas.ContentAddressableStorage instance for a kind (exported for auto-fix)
func (f *FileObjectStorage) GetContentAddressableStorage(kind string) (*filecas.ContentAddressableStorage, error) {
	return f.getContentAddressableStorage(kind)
}

// getContentAddressableStorage gets or creates a filecas.ContentAddressableStorage instance for a kind
func (f *FileObjectStorage) getContentAddressableStorage(kind string) (*filecas.ContentAddressableStorage, error) {
	// Check if kind uses content-addressable storage
	if !f.usesContentAddressableStorage(kind) {
		return nil, errfmt.Errorf(ConstStreamKindStrDoesNotUseContentAddressableStorage, kind)
	}

	kindDir := f.GetKindDir(kind)
	if kindDir == "" {
		return nil, errfmt.Errorf(ConstStreamUnknownObjectKindStr, kind)
	}

	// Get write queue for this project root (from factory or global singleton)
	writeQueue := caspkg.GetListingIndexWriteQueueForProjectRoot(f.projectRoot)
	if writeQueue != caspkg.GetGlobalListingIndexWriteQueue() {
		// Dedicated queue (e.g. from per-project-root factory); set its context
		writeQueue.SetProjectRoot(f.projectRoot)
		writeQueue.SetStorage(f)
	} else {
		// Global queue: only set if not already set for a different project root
		existingWriteProjectRoot := writeQueue.GetProjectRoot()
		existingWriteStorage := writeQueue.GetStorage()
		if existingWriteStorage == nil || existingWriteProjectRoot == f.projectRoot {
			writeQueue.SetProjectRoot(f.projectRoot)
			writeQueue.SetStorage(f)
		} else {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			msg := "Skipping SetProjectRoot/SetStorage on global CAS index write queue - different project root already set"
			if IsTestOrTempProjectRoot(f.projectRoot) {
				StorageLog(logger).Debug(msg).
					String(ConstStreamExistingProjectRoot, existingWriteProjectRoot).
					String(ConstStreamNewProjectRoot, f.projectRoot).
					Log()
			} else {
				StorageLog(logger).Warn(msg).
					String(ConstStreamExistingProjectRoot, existingWriteProjectRoot).
					String(ConstStreamNewProjectRoot, f.projectRoot).
					Log()
			}
		}
	}

	// Get or create CAS instance using ResourceCache abstraction (thread-safe)
	// Use system context for CAS cache operations
	ctx := pkgctx.NewSystemContext()
	cas, err := f.casCache.GetOrCreate(ctx, kind, func(ctx context.Context, key string) (*filecas.ContentAddressableStorage, error) {
		casInstance := filecas.NewContentAddressableStorage(kindDir, key, writeQueue)
		// Wire per-kind CAS in-memory index to InvalidationShockwaveBus ()
		GetGlobalInvalidationBus().Subscribe(NewCASIndexInvalidationSubscriber(casInstance))
		return casInstance, nil
	})
	if err != nil {
		return nil, errfmt.Errorf(ConstStreamFailedToCreateCasInstanceForKindStrErr, kind, err)
	}

	// Set up default orphan cleanup callback to queue old hash files for cleanup
	// NOTE: This is file-backend specific - CAS queues are only used for file backend
	// This prevents orphaned files when objects are updated with new content (new hash)
	// Cleanup is queued (not immediate) to avoid blocking CAS updates and provide flexibility
	var cleanupQueue *caspkg.CASOrphanCleanupQueue
	if f.orphanCleanupQueue != nil {
		cleanupQueue = f.orphanCleanupQueue
		// Injected queue (test isolation): already configured by factory
	} else {
		cleanupQueue = caspkg.GetGlobalOrphanCleanupQueue()
		// Configure global queue with project root and storage for audit events
		// CRITICAL: Only set if queue doesn't already have storage with a different project root
		existingCleanupProjectRoot := cleanupQueue.GetProjectRoot()
		existingCleanupStorage := cleanupQueue.GetStorage()
		if existingCleanupStorage == nil || existingCleanupProjectRoot == f.projectRoot {
			cleanupQueue.SetProjectRoot(f.projectRoot)
			cleanupQueue.SetStorage(f)
		} else {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			msg := "Skipping SetProjectRoot/SetStorage on global CAS orphan cleanup queue - different project root already set"
			if IsTestOrTempProjectRoot(f.projectRoot) {
				StorageLog(logger).Debug(msg).
					String(ConstStreamExistingProjectRoot, existingCleanupProjectRoot).
					String(ConstStreamNewProjectRoot, f.projectRoot).
					Log()
			} else {
				StorageLog(logger).Warn(msg).
					String(ConstStreamExistingProjectRoot, existingCleanupProjectRoot).
					String(ConstStreamNewProjectRoot, f.projectRoot).
					Log()
			}
		}
	}

	cas.SetOrphanCleanupCallback(func(oldHashFilePath string) error {
		// Queue the cleanup operation instead of executing immediately
		// This provides:
		// - Non-blocking updates (doesn't slow down CAS operations)
		// - Batching (multiple cleanups processed together)
		// - Retry capability (failed cleanups can be retried)
		// - Flexibility (can defer cleanup under load)
		// - Worker lifecycle management (shuts down when idle, wakes on work)
		if err := cleanupQueue.EnqueueCleanup(oldHashFilePath); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageObjectCASEnqueueOrphanCleanupFailedWarn).
				String("file_path", oldHashFilePath).
				WithError(err).
				Log()
			// Synchronous fallback: same rename-then-delete as the queue worker so we never
			// rely solely on the channel when it is full or shutdown is in progress.
			if rmErr := caspkg.RemoveOrphanCASHashFileSync(oldHashFilePath); rmErr != nil {
				StorageLog(logger).Warn(LogEventStorageObjectCASOrphanCleanupSyncFailedWarn).
					String("file_path", oldHashFilePath).
					WithError(rmErr).
					Log()
			}
		}
		return nil // Always return nil - cleanup is best-effort and queued
	})

	// Identity commit: required post-sync so ACK cannot outrun object-id-cache.
	// TRACK: TDE-CEF-CAS-IDENTITY-TXN-001
	cas.SetRequiredPostSyncCallback(func(objectID, casKind, hash, filePath string) error {
		if cacheOperationHandler == nil {
			return errfmt.Errorf(ErrMsgIdentityCacheHandlerRequired)
		}

		cacheCtx := &pkgctx.CacheContext{
			Operation: pkgctx.CacheOperationUpdate,
			NewID:     objectID,
			Kind:      casKind,
			FilePath:  filePath,
		}

		if err := cacheOperationHandler(cacheCtx); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageObjectCASRegisterCachePostSyncFailedWarn).
				ObjectID(objectID).
				Kind(casKind).
				String("file_path", filePath).
				WithError(err).
				Log()
			return err
		}

		return nil
	})

	// CAS instance already cached in ResourceCache during GetOrCreate() above.
	// Write queue project root and storage were set above when we obtained the queue.

	return cas, nil
}

// filecas.CasHashFilePeekContainsObjectID returns true if path is a readable hash-named YAML whose content
// subdirs whose embedded id matches objectID. After an Update, an older content hash file can remain
// on disk while the index points at the new hash; Delete removes only the indexed file. Without this
// pass, Read's CAS discovery would re-index the orphan and resurrect the object (stale content).

func (f *FileObjectStorage) WriteObjectRaw(ctx context.Context, kind, id string, data []byte) error {
	secCtx := pkgctx.NewSystemSecurityContext()
	if StreamStorageEnabledForKind(kind) {
		return f.writeObjectToStream(ctx, id, kind, data)
	}

	useDraftPlane := false
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err == nil {
		useDraftPlane = caspkg.UseObjectDraftPlane(kind, obj, false)
	}

	if useDraftPlane {
		return f.WriteObjectToDraftPlane(id, kind, data)
	}

	kindDir := f.GetKindDir(kind)
	if kindDir == "" {
		return errfmt.Errorf("unknown object kind: %s", kind)
	}
	filePath := filepath.Join(kindDir, id+".yaml")
	return f.writeCASThroughMembrane(ctx, id, kind, data, false, func() error {
		return f.writeObjectToCAS(ctx, id, kind, filePath, data, secCtx)
	})
}

type pendingCacheWrapper struct {
	cache *caspkg.CASPendingVisibilityCache
}

func (w *pendingCacheWrapper) LookupPending(id string) (filecas.PendingVisibilityEntry, bool) {
	entry, found := w.cache.LookupPending(id)
	return filecas.PendingVisibilityEntry{
		ObjectID:  entry.ObjectID,
		Hash:      entry.Hash,
		BucketKey: entry.BucketKey,
		Kind:      entry.Kind,
	}, found
}

func (w *pendingCacheWrapper) PublishPending(objectID, kind, hash, bucketKey string) error {
	return w.cache.PublishPending(objectID, kind, hash, bucketKey)
}

func (w *pendingCacheWrapper) EvictPending(objectID string) error {
	return w.cache.EvictPending(objectID)
}

func (w *pendingCacheWrapper) SnapshotPending() []filecas.PendingVisibilityEntry {
	snapshot := w.cache.SnapshotPending()
	result := make([]filecas.PendingVisibilityEntry, len(snapshot))
	for i, v := range snapshot {
		result[i] = filecas.PendingVisibilityEntry{
			ObjectID:  v.ObjectID,
			Hash:      v.Hash,
			BucketKey: v.BucketKey,
			Kind:      v.Kind,
		}
	}
	return result
}

type metricsWrapper struct {
	m *caspkg.CASMetrics
}

func (w *metricsWrapper) RecordCreate(duration time.Duration, success bool) {
	if success {
		w.m.RecordCreate(duration, nil)
	} else {
		w.m.RecordCreate(duration, errfmt.Errorf("error"))
	}
}
func (w *metricsWrapper) RecordRead(duration time.Duration, success bool) {
	if success {
		w.m.RecordRead(duration, nil)
	} else {
		w.m.RecordRead(duration, errfmt.Errorf("error"))
	}
}
func (w *metricsWrapper) RecordUpdate(duration time.Duration, success bool) {
	if success {
		w.m.RecordUpdate(duration, nil)
	} else {
		w.m.RecordUpdate(duration, errfmt.Errorf("error"))
	}
}
func (w *metricsWrapper) RecordDelete(duration time.Duration, success bool) {
	if success {
		w.m.RecordDelete(duration, nil)
	} else {
		w.m.RecordDelete(duration, errfmt.Errorf("error"))
	}
}
func (w *metricsWrapper) RecordIndexFileLock(success bool, duration time.Duration) {
	w.m.RecordIndexFileLock(success, duration)
}
func (w *metricsWrapper) RecordIndexSave(duration time.Duration, err error, count int) {
	w.m.RecordIndexSave(duration, err, count)
}
func (w *metricsWrapper) RecordSetMapping(duration time.Duration, err error, isNew bool) {
	w.m.RecordSetMapping(duration, err, isNew)
}
func (w *metricsWrapper) RecordIndexReload() {
	w.m.RecordIndexReload()
}
func (w *metricsWrapper) RecordRemoveMapping(duration time.Duration, err error) {
	w.m.RecordRemoveMapping(duration, err)
}

func init() {
	filecas.GetCASPendingVisibilityCache = func(dir string) filecas.PendingVisibilityCache {
		cache := caspkg.GetCASPendingVisibilityCache(dir)
		if cache == nil {
			return nil
		}
		return &pendingCacheWrapper{cache: cache}
	}
	filecas.GetMetrics = func() filecas.StorageMetrics {
		return &metricsWrapper{m: caspkg.GetObjectStorageMetrics()}
	}
}

func init() {
	filecas.NoteObjectIDCachePending = func(projectRoot, op, id, kind, filePath, reason string) {
		NoteObjectIDCachePending(projectRoot, op, id, kind, filePath, reason)
	}
	filecas.WarnOnceNilCacheHandler = func(objectID, kind string) {
		warnOnceNilCacheHandler(objectID, kind)
	}
}

func init() {
	filecas.IsHighVolumeKindForCache = func(kind string) bool {
		return IsHighVolumeKindForCache(kind)
	}
}

type lockStrategyWrapper struct {
	s *AutoCleanupStrategy
}

func (w *lockStrategyWrapper) AcquireLock(lockPath string, timeout time.Duration) (filecas.FileLockHandle, error) {
	return w.s.AcquireLock(lockPath, timeout)
}

func init() {
	filecas.NewAutoCleanupStrategy = func() filecas.LockStrategy {
		return &lockStrategyWrapper{s: NewAutoCleanupStrategy()}
	}
}

func (f *FileObjectStorage) DeleteObjectRaw(ctx context.Context, kind, id string) error {
	cas, err := f.GetContentAddressableStorage(kind)
	if err != nil {
		return err
	}
	return cas.Delete(id)
}
func (f *FileObjectStorage) RenameObjectRaw(ctx context.Context, kind, oldID, newID string) error {
	cas, err := f.GetContentAddressableStorage(kind)
	if err != nil {
		return err
	}
	data, err := cas.Read(oldID)
	if err != nil {
		return err
	}
	renamedData, err := crud.RewriteObjectIDForRawRename(data, newID)
	if err != nil {
		return errfmt.Newf("rewrite object ID for raw rename").Wrap(err)
	}
	return cas.UpdateWithIDChange(oldID, newID, renamedData)
}

func init() {
	filecas.ConfirmPendingAfterDurableMapping = func(indexPath, objectID, hash string) {
		caspkg.ConfirmPendingAfterDurableMapping(indexPath, objectID, hash)
	}
}

func init() {
	filecas.GlobalIndexWriteQueue = caspkg.GetGlobalListingIndexWriteQueue()
	filecas.RestorePendingAfterFailedMutation = func(projectRoot, objectID, kind, oldHash, oldBucketKey string) {
		caspkg.RestorePendingAfterFailedMutation(projectRoot, objectID, kind, oldHash, oldBucketKey)
	}
	filecas.GetCacheOperationHandler = func() func(*pkgctx.CacheContext) error {
		return GetCacheOperationHandler()
	}
}
func (f *FileObjectStorage) RecoverCASObjectViaStorage(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	objectID string,
	options *caspkg.CASRecoveryOptions,
) (*caspkg.CASRecoveryResult, error) {
	// Infer kind from ID
	if err := f.idValidator.LoadPatterns(); err != nil {
		return nil, errfmt.Newf(ConstStreamFailedToLoadIdPatterns).Wrap(err)
	}
	kind := f.idValidator.InferKindFromID(objectID)
	if kind == emptyValue {
		return nil, errfmt.Errorf(ConstStreamCouldNotInferKindFromIdStr, objectID)
	}

	// Check if this kind uses CAS
	if !f.usesContentAddressableStorage(kind) {
		return nil, errfmt.Errorf(ConstStreamKindStrDoesNotUseContentAddressableStorage, kind)
	}

	// Use the recovery function with project root
	projectRoot := f.projectRoot
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf(ConstStreamProjectRootNotSet)
	}

	return caspkg.RecoverCASObject(ctx, projectRoot, objectID, kind, options, nil)
}

// PutStreamSegmentChunk chunks, compresses, and persists a stream-segment payload under content-addressed
// identity. Identical chunks share a single CAS blob. Mapping failure is fail-closed.
func (f *FileObjectStorage) PutStreamSegmentChunk(kind, objectID string, data []byte) (string, error) {
	cas, err := f.getContentAddressableStorage(kind)
	if err != nil {
		return "", err
	}
	return cas.PutStreamSegmentChunk(objectID, data)
}

// PutStreamSegment partitions, compresses, and persists a multi-chunk stream segment under content-addressed
// identity. Duplicate chunks across segments share a single CAS blob.
func (f *FileObjectStorage) PutStreamSegment(kind, objectID string, data []byte) ([]string, error) {
	cas, err := f.getContentAddressableStorage(kind)
	if err != nil {
		return nil, err
	}
	return cas.PutStreamSegment(objectID, data)
}

// GetStreamSegmentChunk rehydrates the original bytes for objectID from deduped/compressed CAS blobs.
// Miss or hash mismatch is fail-closed, not a silent kind-root scan success.
func (f *FileObjectStorage) GetStreamSegmentChunk(kind, objectID string) ([]byte, error) {
	cas, err := f.getContentAddressableStorage(kind)
	if err != nil {
		return nil, err
	}
	return cas.GetStreamSegmentChunk(objectID)
}
