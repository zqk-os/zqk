package validation

import (
	"context"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
)

// sharedValidationCacheFlushDebounce coalesces dirty signals before JSON-marshal
// + rename of the full cache. 50ms flushed ~8k entries on a check-completion timer.
// TRACK: TDE-CEF-VALIDATION-CACHE-SAVE-STORM-001
const sharedValidationCacheFlushDebounce = 2 * time.Second

// sharedValidationCache wraps a ValidationStateCache with minimal
// background persistence. It is intended to be used as a per-project
// singleton so hot-path invalidations only mutate in-memory state and
// persistence happens asynchronously.
type sharedValidationCache struct {
	cache       *ValidationStateCache
	projectRoot string
	dirtyCh     chan struct{} // buffered; signals that cache should be saved
}

var (
	sharedCachesMu sync.Mutex
	sharedCaches   = make(map[string]*sharedValidationCache)
)

// EnsureValidationCacheReady ensures the validation state cache file exists on disk.
// It gets or creates the per-project shared cache (loading from disk if present),
// then saves so that .zqk/cache/validation_cache.json is created if it did not exist.
// Used by cache pre-warm so the file exists in the background like other caches.
func EnsureValidationCacheReady(projectRoot string) error {
	if projectRoot == emptyValue {
		return nil
	}
	sc := getSharedValidationCache(projectRoot)
	if sc == nil {
		return nil
	}
	return sc.cache.Save()
}

// getSharedValidationCache returns a per-project shared cache instance.
// It loads from disk once on first use, then reuses the in-memory map
// for subsequent invalidations. A background goroutine persists changes
// when markDirty is called.
func getSharedValidationCache(projectRoot string) *sharedValidationCache {
	if projectRoot == emptyValue {
		return nil
	}

	sharedCachesMu.Lock()
	defer sharedCachesMu.Unlock()

	if c, ok := sharedCaches[projectRoot]; ok && c != nil {
		return c
	}

	cache := NewValidationStateCache(projectRoot, 0)
	if err := cache.Load(); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Warn(
			"Failed to load validation cache for shared instance (create/update/delete sync)").
			ProjectRoot(projectRoot).
			WithError(err).
			Log()
		// Proceed with empty in-memory cache; save path will create a fresh file.
	}

	sc := &sharedValidationCache{
		cache:       cache,
		projectRoot: projectRoot,
		dirtyCh:     make(chan struct{}, 1),
	}
	sharedCaches[projectRoot] = sc

	// Use managed goroutine for flusher
	builder := goroutinelabels.NewGoroutine(ConstMagicc54497bd, ConstMagicfabb6212)
	builder.StartSimple(func() {
		sc.runFlusher(context.Background()) // System-wide shared instance uses background context
	})

	return sc
}

// markDirty signals that the cache has changed and should be persisted.
// Non-blocking: if a flush is already queued, additional signals are coalesced.
func (c *sharedValidationCache) markDirty() {
	select {
	case c.dirtyCh <- struct{}{}:
	default:
		// A flush is already queued; no need to send another signal.
	}
}

// runFlusher listens for dirty signals and writes the current cache state
// to disk.
func (c *sharedValidationCache) runFlusher(ctx context.Context) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.dirtyCh:
			timer := time.NewTimer(sharedValidationCacheFlushDebounce)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			select {
			case <-c.dirtyCh:
			default:
			}
			if err := c.cache.Save(); err != nil {
				logging.Fluent(logger).Warn(
					"Failed to save validation cache from shared flusher (create/update/delete sync)").
					ProjectRoot(c.projectRoot).
					WithError(err).
					Log()
			}
		}
	}
}

// InvalidateObjectsInGlobalValidationCache invalidates the given object IDs
// in the per-project shared validation cache and schedules an asynchronous
// persistence. This avoids repeated Load/Save cycles on every invalidate.
func InvalidateObjectsInGlobalValidationCache(projectRoot string, objectIDs []string) {
	InvalidateObjectsInGlobalValidationCacheWithContext(context.Background(), projectRoot, objectIDs)
}

// InvalidateObjectsInGlobalValidationCacheWithContext is the context-aware
// version used by tests and advanced callers. CacheMode on ctx controls
// whether persistence is asynchronous (default) or synchronous (test_sync).
func InvalidateObjectsInGlobalValidationCacheWithContext(ctx context.Context, projectRoot string, objectIDs []string) {
	if projectRoot == emptyValue || len(objectIDs) == 0 {
		return
	}

	sc := getSharedValidationCache(projectRoot)
	if sc == nil {
		return
	}

	anyRemoved := false
	for _, id := range objectIDs {
		if id != emptyValue {
			if sc.cache.Invalidate(id) {
				anyRemoved = true
			}
		}
	}

	if !anyRemoved {
		return
	}

	mode := pkgctx.GetCacheMode(ctx)
	if mode == pkgctx.CacheModeTestSync {
		// Deterministic behavior for tests: persist immediately on return.
		if err := sc.cache.Save(); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Warn(ConstMagic055d9193).
				ProjectRoot(projectRoot).
				WithError(err).
				Log()
		}
		return
	}

	sc.markDirty()
}

// GetValidationStateForTest returns the current in-memory validation state for
// a given object ID, but only when CacheModeTestSync is set on ctx. This is a
// test helper to avoid disk races.
func GetValidationStateForTest(ctx context.Context, projectRoot, objectID string) (*ValidationState, bool) {
	if pkgctx.GetCacheMode(ctx) != pkgctx.CacheModeTestSync {
		return nil, false
	}

	sc := getSharedValidationCache(projectRoot)
	if sc == nil {
		return nil, false
	}

	return sc.cache.Get(objectID)
}

// UpdateObjectInGlobalValidationCacheWithContext upserts a ValidationState for
// a single object into the shared cache and persists according to CacheMode.
// This is used by incremental single-object validation to repopulate the cache.
func UpdateObjectInGlobalValidationCacheWithContext(ctx context.Context, projectRoot string, state *ValidationState) {
	if projectRoot == emptyValue || state == nil || state.ObjectID == emptyValue {
		return
	}
	if len(state.ObjectID) > MaxObjectIDLength {
		return
	}
	if !ShouldCacheValidationState(state.ObjectKind) {
		return
	}

	sc := getSharedValidationCache(projectRoot)
	if sc == nil {
		return
	}

	sc.cache.Set(state)

	mode := pkgctx.GetCacheMode(ctx)
	if mode == pkgctx.CacheModeTestSync {
		if err := sc.cache.Save(); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Warn(ConstMagicc8a30dfe).
				ProjectRoot(projectRoot).
				WithError(err).
				Log()
		}
		return
	}

	sc.markDirty()
}
