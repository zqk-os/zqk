package system

import (
	"context"
	"path/filepath"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/validation"

	"gopkg.in/yaml.v3"
)

// InvalidateValidationStateCacheForObjects removes the given object IDs from the
// persisted validation state cache (violation cache) so async validation can
// repopulate them and system check can report up-to-date results from cache.
// Keeps validation cache in sync with create/update/delete.
// Best-effort: logs warnings on error and does not fail the caller.
func InvalidateValidationStateCacheForObjects(projectRoot string, objectIDs []string) {
	// Delegate to shared per-project cache so we don't repeatedly load/save
	// the entire cache file on every create/update/delete. Persistence is
	// handled asynchronously by the shared cache flusher.
	validation.InvalidateObjectsInGlobalValidationCache(projectRoot, objectIDs)
}

// InvalidateValidationStateCacheForObjectsWithContext is a context-aware
// variant used by tests and advanced callers. CacheMode on ctx controls
// whether persistence is asynchronous (default) or synchronous (test_sync).
func InvalidateValidationStateCacheForObjectsWithContext(ctx context.Context, projectRoot string, objectIDs []string) {
	validation.InvalidateObjectsInGlobalValidationCacheWithContext(ctx, projectRoot, objectIDs)
}

// InvalidateValidationCacheForCacheContext invalidates the validation state cache
// for all object IDs affected by the given cache operation (create/update/delete).
// Call this from the cache operation handler so violation cache stays in sync
// with object ID cache and storage.
func InvalidateValidationCacheForCacheContext(projectRoot string, cacheCtx *pkgctx.CacheContext) {
	if projectRoot == emptyValue || cacheCtx == nil {
		return
	}
	var ids []string
	switch cacheCtx.Operation {
	case pkgctx.CacheOperationUpdate:
		if cacheCtx.NewID != emptyValue {
			ids = []string{cacheCtx.NewID}
		}
	case pkgctx.CacheOperationInvalidate:
		if cacheCtx.OldID != emptyValue {
			ids = []string{cacheCtx.OldID}
		}
	case pkgctx.CacheOperationInvalidateAndUpdate:
		seen := make(map[string]bool)
		if cacheCtx.OldID != emptyValue {
			seen[cacheCtx.OldID] = true
		}
		if cacheCtx.NewID != emptyValue {
			seen[cacheCtx.NewID] = true
		}
		ids = make([]string, 0, len(seen))
		for id := range seen {
			ids = append(ids, id)
		}
	default:
		return
	}
	// Find dependents using the global reverse reference index.
	// We must also invalidate the validation cache for any objects that reference
	// these changed object IDs (dependents) to prevent stale cross-reference validation cache hits.
	seen := make(map[string]bool)
	var allIds []string
	for _, id := range ids {
		if id != emptyValue && !seen[id] {
			seen[id] = true
			allIds = append(allIds, id)

			// Find dependents from reverse reference index
			dependents := storage.GetGlobalReverseReferenceIndex().GetDependents(id)
			for _, dep := range dependents {
				if dep != emptyValue && !seen[dep] {
					seen[dep] = true
					allIds = append(allIds, dep)
				}
			}
		}
	}

	InvalidateValidationStateCacheForObjects(projectRoot, allIds)

	// Incremental cache repopulation: enqueue affected objects for async validation
	// so the validation state cache is repopulated without a full system check.
	// For update/create we enqueue the new object; for delete we only invalidate.
	switch cacheCtx.Operation {
	case pkgctx.CacheOperationUpdate, pkgctx.CacheOperationInvalidateAndUpdate:
		if cacheCtx.NewID != emptyValue {
			EnqueueValidationForObject(projectRoot, cacheCtx.NewID, cacheCtx.Kind, cacheCtx.FilePath)
		}
	default:
		// no enqueue for delete
	}
}

// EnqueueValidationForObject enqueues a single object for validation so the
// validation state cache can be repopulated incrementally after create/update.
// Called from the cache operation handler; best-effort and non-blocking.
// Work runs on a bounded pool: one goroutine per object minted an OS thread per
// blocked ReadFile and exhausted the CLI 512-thread cap on object promote.
// TRACK: BLI-CEF-STORAGE-INDEX-CACHE-001
func EnqueueValidationForObject(projectRoot, objectID, kind, filePath string) {
	if projectRoot == emptyValue || objectID == emptyValue {
		return
	}
	pr, id, k, fp := projectRoot, objectID, kind, filePath
	err := incrementalValidationPool().SubmitNonBlocking(context.Background(), func(context.Context) error {
		_ = RunEnqueueValidationForObjectViaPipeline(pr, id, k, fp)
		return nil
	})
	if err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Debug("Enqueue validation dropped (pool full or stopped)").
			ObjectID(id).
			Kind(k).
			WithError(err).
			Log()
	}
}

// enqueueValidationForObjectCore executes the actual incremental validation work for one object.
// Best-effort: it returns nil on any failure so callers remain non-blocking.
func enqueueValidationForObjectCore(projectRoot, objectID, kind, filePath string) error {
	bgCtx := context.Background() // Background: request-or-shutdown derived
	asyncCtx := buildMinimalAsyncValidationContextForEnqueue(projectRoot)
	if asyncCtx == nil {
		return nil
	}
	fn := createAsyncValidationFunc(asyncCtx)

	// Object create/update often passes an empty file path in cache context; incremental
	// validation must still read the CAS file. Resolve canonical .zqk/process/<dir>/<id>.yaml.
	resolved := filePath
	if resolved == emptyValue && kind != emptyValue && objectID != emptyValue {
		if dirName := objects.GetDirectoryFromKind(kind); dirName != emptyValue {
			resolved = filepath.Join(datacell.CellCASPrimaryDir(".", dirName), objectID+".yaml")
		}
	}
	absPath := resolved
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(projectRoot, absPath)
	}

	content, err := fileutil.ReadFile(absPath)
	if err != nil && asyncCtx.StorageProvider != nil {
		// CAS/hash-backed objects may not live at .zqk/process/<dir>/<id>.yaml; read via storage API.
		readCtx := pkgctx.NewSystemContext()
		secCtx := pkgctx.NewSystemSecurityContext()
		if obj, rerr := asyncCtx.StorageProvider.Read(readCtx, secCtx, objectID); rerr == nil {
			content, err = yaml.Marshal(obj)
		}
	}
	if err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Debug("Enqueue validation: failed to read object bytes").
			Path(absPath).
			WithError(err).
			Log()
		return nil
	}

	state, err := fn(bgCtx, objectID, kind, absPath, content)
	if err != nil || state == nil {
		return nil
	}

	// Write result into shared per-project cache so incremental validation
	// repopulates the same cache used by invalidation.
	validation.UpdateObjectInGlobalValidationCacheWithContext(bgCtx, projectRoot, state)

	// Notify coordinator so tests/subscribers can block on completion (e.g. callback-based assertions).
	emitIncrementalValidationComplete(projectRoot, objectID)

	return nil
}

// emitIncrementalValidationComplete emits an operational "complete" event for
// incremental validation so subscribers (e.g. integration tests) can block on
// callback instead of polling the cache. Uses EventCoordinator.Emit (not a
// *Coordinator type assert) so any global coordinator implementation delivers
// operational events to subscribers.
func emitIncrementalValidationComplete(projectRoot, objectID string) {
	if projectRoot == emptyValue || objectID == emptyValue {
		return
	}
	ec := coordination.GetCoordinator()
	if ec == nil {
		return
	}
	operationID := "incremental_validation_" + objectID
	eventData := &coordination.EventData{
		MetricsData: map[string]any{"object_id": objectID},
		LoggingFields: []coordination.LoggingField{
			{Key: "object_id", Value: objectID},
			{Key: "operation_type", Value: "incremental_validation"},
		},
	}
	eventCtx := coordination.NewEventContext(operationID, "incremental_validation", "complete").
		WithEventData(eventData).
		WithChannels(false, false, false, true)
	// Coordinator.buildEmitPipeline runs operational delivery sync when subscribers exist.
	_ = ec.Emit(context.Background(), eventCtx) //nolint:errcheck // Best-effort // Background: request-or-shutdown derived
}
