package objectidcache

import (
	stdcontext "context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/loader"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/pipeline"
	"github.com/lanceman/zqk/pkg/process"
	"github.com/lanceman/zqk/pkg/storage"
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"
)

// noOpCacheProgressNotifier is used when no progress emission is wanted; atomic.Value cannot store nil.
var noOpCacheProgressNotifier ObjectIDCacheProgressNotifier = (*noOpCacheProgressNotifierType)(nil)

type noOpCacheProgressNotifierType struct{}

func (*noOpCacheProgressNotifierType) NotifyCacheProgress(string, string) {}

// progressNotifierHolder is the single concrete type stored in progressNotifier atomic.Value.
// atomic.Value panics if the concrete type of a Store differs from the first Store; we always store *progressNotifierHolder.
type progressNotifierHolder struct {
	n ObjectIDCacheProgressNotifier
}

// objectIDCacheProgressCallback invokes the optional notifier (set before Load) so progress is emitted via coordinator.
type objectIDCacheProgressCallback struct {
	cache *ObjectIDCache
}

func (p *objectIDCacheProgressCallback) OnLoading() {
	if h, _ := p.cache.progressNotifier.Load().(*progressNotifierHolder); h != nil && h.n != nil {
		h.n.NotifyCacheProgress("loading", "Preparing object ID cache...")
	}
}
func (p *objectIDCacheProgressCallback) OnLoaded(any) {
	if h, _ := p.cache.progressNotifier.Load().(*progressNotifierHolder); h != nil && h.n != nil {
		h.n.NotifyCacheProgress("ready", "Object ID cache ready.")
	}
}
func (p *objectIDCacheProgressCallback) OnError(err error) {
	if h, _ := p.cache.progressNotifier.Load().(*progressNotifierHolder); h != nil && h.n != nil {
		h.n.NotifyCacheProgress("error", fmt.Sprintf("Object ID cache failed: %v", err))
	}
}
func (p *objectIDCacheProgressCallback) OnTimeout() {
	if h, _ := p.cache.progressNotifier.Load().(*progressNotifierHolder); h != nil && h.n != nil {
		h.n.NotifyCacheProgress("timeout", "Object ID cache timed out.")
	}
}

// notifyCacheProgress emits progress from inside BuildCache so the CLI shows activity during warm/rebuild.
func (c *ObjectIDCache) notifyCacheProgress(status, message string) {
	if h, _ := c.progressNotifier.Load().(*progressNotifierHolder); h != nil && h.n != nil {
		h.n.NotifyCacheProgress(status, message)
	}
}

// GetEnsureRunner returns the loader.Runner for "ensure object ID cache ready" (lazily created).
// The runner runs BuildCache with configurable timeout so --refresh-cache doesn't hit CLI timeout.
func (c *ObjectIDCache) GetEnsureRunner() *loader.Runner {
	return c.getEnsureRunner()
}

// getEnsureRunner returns the loader.Runner for "ensure object ID cache ready" (lazily created).
func (c *ObjectIDCache) getEnsureRunner() *loader.Runner {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.RunInLockWithLogger(
		&c.ensureRunnerMu,
		LockNameObjectIDCacheGetRunner,
		logging.NewLockLoggerAdapter(logger),
		func() error {
			if c.ensureRunner == nil {
				c.ensureRunner = loader.NewRunner("object_id_cache", func(ctx stdcontext.Context) error {
					var projectRoot string
					if v := c.currentProjectRoot.Load(); v != nil {
						projectRoot = v.(string)
					}
					forceRebuild := c.nextForceRebuild.Swap(false)
					return c.BuildCache(ctx, projectRoot, forceRebuild)
				}, loader.WithCallback(&objectIDCacheProgressCallback{cache: c}))
			}
			return nil
		},
	)
	return c.ensureRunner
}

// EnsureObjectIDCacheReady ensures the object ID cache is loaded or built for projectRoot, with timeout.
// Uses pkg/loader so timeouts are configurable (e.g. component_loaders.object_id_cache) and concurrent
// callers wait on one in-flight load instead of each running BuildCache.
// When projectRoot changes, the runner is reset so the new project is loaded.
// If notifier is non-nil, progress is emitted via the notifier (e.g. coordinator) for CLI subscribers.
// If storageForWarm is non-nil, BuildCache uses it for warmCASIndexesFromCache so the same instance is warmed
// and later used for discovery/ref validation (avoids cold CAS indexes on a different storage instance).
//
// The load/warm sequence runs under pkg/pipeline (kind pipelineKindEnsureObjectIDCacheReady; ingest → commit → finalize),
// aligned with docs/architecture/system-check-pipeline.md and CVS desired_end_state (2).
func EnsureObjectIDCacheReady(ctx stdcontext.Context, projectRoot string, forceRebuild bool, notifier ObjectIDCacheProgressNotifier, storageForWarm storage.ObjectStorageProvider) error {
	projectRoot = resolveProjectRoot(projectRoot)
	type ensureReadyState struct {
		cache             *ObjectIDCache
		storageForWarmSet bool
		err               error
	}

	state := &ensureReadyState{}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	pl := pipeline.NewBuilder(pipelineKindEnsureObjectIDCacheReady, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage(pipeline.StageIngest, func(pctx *pipeline.Context, payload any) (any, error) {
			state.cache = GetGlobalObjectIDCache()

			// Always store the same concrete type (*progressNotifierHolder) so atomic.Value never panics on type change.
			n := noOpCacheProgressNotifier
			if notifier != nil {
				n = notifier
			}
			state.cache.progressNotifier.Store(&progressNotifierHolder{n: n})

			if storageForWarm != nil {
				state.storageForWarmSet = true
				state.cache.warmStorageForNextBuild.Store(&warmStorageHolder{storage: storageForWarm})
			}

			return state, nil
		}).
		AddStage(pipeline.StageCommit, func(pctx *pipeline.Context, payload any) (any, error) {
			if state.cache == nil {
				state.err = errfmt.Errorf("object ID cache is nil")
				return state, nil
			}
			if err := concurrency.RunInLockWithLogger(
				&state.cache.ensureRunnerMu,
				LockNameObjectIDCacheEnsureReady,
				logging.NewLockLoggerAdapter(logger),
				func() error {
					old, _ := state.cache.currentProjectRoot.Load().(string)
					if projectRoot != old {
						if state.cache.ensureRunner != nil {
							state.cache.ensureRunner.ResetLoaded()
						}
						state.cache.currentProjectRoot.Store(projectRoot)
					}
					// --refresh-cache must rebuild even when this process already
					// Loaded the cache (loader.Runner fast-path otherwise no-ops).
					if forceRebuild && state.cache.ensureRunner != nil {
						state.cache.ensureRunner.ResetLoaded()
					}
					return nil
				},
			); err != nil {
				state.err = err
				return state, nil
			}

			state.cache.nextForceRebuild.Store(forceRebuild)
			if err := state.cache.getEnsureRunner().Load(ctx); err != nil {
				state.err = err
				return state, nil
			}

			// When we provided a storage for warm, warm it here (once). Covers both: Load() ran BuildCache (which skipped warm)
			// and Load() returned early (cache already loaded). Discovery and ref validation use this instance.
			if storageForWarm != nil {
				state.cache.notifyCacheProgress("warming", "Warming CAS indexes...")
				warmCASIndexesFromCache(ctx, projectRoot, state.cache, storageForWarm, 0)
			}

			// Drain object-id-cache pending journal so mid-refresh check sees coherent entries.
			// TRACK: BLI-REDACTED
			drainObjectIDCachePending(projectRoot)

			return state, nil
		}).
		AddStage(pipeline.StageFinalize, func(pctx *pipeline.Context, payload any) (any, error) {
			if state.cache != nil {
				state.cache.progressNotifier.Store(&progressNotifierHolder{n: noOpCacheProgressNotifier})
				if state.storageForWarmSet {
					state.cache.warmStorageForNextBuild.Store(&warmStorageHolder{storage: nil})
				}
			}
			return state, nil
		}).
		Build()

	_, runErr := pl.Run(&pipeline.Context{Ctx: ctx, Outcome: make(map[string]any)}, state)
	if runErr != nil {
		return runErr
	}
	return state.err
}

// TryLoadObjectIDCacheOnly loads the object ID cache from disk only (no build, no wait).
// Also tries to load the reverse reference index so findDependents can use it when available.
// Returns true if the cache was loaded and has at least one entry, so callers can use it
// without blocking. Used to keep commands snappy: use cache when available; otherwise
// trigger a background build and proceed with best-effort (e.g. ref validation may be incomplete).
func TryLoadObjectIDCacheOnly(projectRoot string) bool {
	projectRoot = resolveProjectRoot(projectRoot)
	if projectRoot == emptyValue {
		return false
	}
	cache := GetGlobalObjectIDCache()
	loaded, err := cache.LoadCache(projectRoot)
	if err != nil || !loaded {
		return false
	}
	// Heal object-id-cache paths that still name deleted CAS hashes (update/promote
	// lag). Without this, async system check discovery validates ghosts and emits
	// false "stale index entry" Tier-1 blockers.
	// TRACK: BLI-REDACTED
	if n := cache.ValidateAndCleanStale(); n > 0 {
		storage.InvalidateListCache()
		persistCacheChanges(cache, projectRoot, "Failed to save object ID cache after stale-path heal")
	}
	// Best effort: load or build reverse reference index so findDependents can use it.
	// Prefer synchronous build using kinds from the cache we just loaded (avoids racing teardown on temp roots).
	revIndex := storage.GetGlobalReverseReferenceIndex()
	revLoaded, _ := revIndex.LoadCache(projectRoot)
	if !revLoaded {
		kinds := cache.kindNamesForReverseReferenceScan()
		processDir := datacell.ProcessPrimaryDir(projectRoot)
		if err := ensureReverseReferenceIndexSync(projectRoot, processDir, kinds); err != nil {
			triggerBackgroundReverseReferenceIndexBuild(projectRoot)
		}
	}
	var entryCount int
	_ = concurrency.WithRLockTimeout(
		&cache.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))),
		LockNameObjectIDCacheTryLoadCount,
		func() error {
			if cache.byKind != nil {
				entryCount = len(cache.idToKind)
			}
			return nil
		},
	)
	return entryCount > 0
}

// TriggerBackgroundObjectIDCacheBuild starts a background goroutine to build and save the
// object ID cache. Does not block. Call when cache is not ready so the next command or
// scheduler run can use it. Sync work (cache build) runs in the background to keep CLI responsive.
func TriggerBackgroundObjectIDCacheBuild(projectRoot string) {
	triggerBackgroundObjectIDCacheBuild(projectRoot, false)
}

// TriggerBackgroundObjectIDCacheForceRebuild starts a background goroutine to force-rebuild
// and save the object ID cache (ignores existing cache file). Use when cache is stale (e.g.
// count mismatch with storage) so the next run sees a fresh cache.
func TriggerBackgroundObjectIDCacheForceRebuild(projectRoot string) {
	triggerBackgroundObjectIDCacheBuild(projectRoot, true)
}

func triggerBackgroundObjectIDCacheBuild(projectRoot string, forceRebuild bool) {
	projectRoot = resolveProjectRoot(projectRoot)
	if projectRoot == emptyValue {
		return
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	bud := goroutinelabels.DefaultBudget()
	// When no budget is set, StartWithContext always starts the goroutine — count before spawn so
	// WaitProjectCacheBackgroundWork does not observe a false idle between schedule and fn body.
	if bud == nil {
		projectCacheBgIncObjectID(projectRoot)
	}
	builder := goroutinelabels.NewGoroutine("object_id_cache_background_build", "building object ID cache in background")
	if bud != nil {
		builder = builder.WithBudget(bud)
	}
	builder.StartWithContext(stdcontext.Background(), func(ctx stdcontext.Context) error {
		if bud != nil {
			projectCacheBgIncObjectID(projectRoot)
		}
		defer projectCacheBgDecObjectID(projectRoot)
		if err := EnsureObjectIDCacheReady(ctx, projectRoot, forceRebuild, nil, nil); err != nil {
			logging.Fluent(logger).Debug("Background object ID cache build failed (non-blocking)").
				ProjectRoot(projectRoot).
				WithError(err).
				Log()
			return nil // Don't propagate; this is best-effort
		}
		return nil
	})
}

func TryBuildAndSaveReverseReferenceIndexSync(projectRoot string, discoveryTimeout time.Duration) bool {
	return tryBuildAndSaveReverseReferenceIndexSync(projectRoot, discoveryTimeout)
}

// tryBuildAndSaveReverseReferenceIndexSync builds and saves the reverse reference index synchronously.
// Kind discovery is bounded by a short timeout; BuildFromScan and SaveCache run in the caller's goroutine (no spawn).
// Returns true if build and save completed successfully, false otherwise.
// When false, caller should trigger background build so next run or daemon can have the cache.
func tryBuildAndSaveReverseReferenceIndexSync(projectRoot string, discoveryTimeout time.Duration) bool {
	projectRoot = resolveProjectRoot(projectRoot)
	if projectRoot == emptyValue || discoveryTimeout <= 0 {
		return false
	}
	ctx, cancel := stdcontext.WithTimeout(stdcontext.Background(), discoveryTimeout)
	processDir := datacell.ProcessPrimaryDir(projectRoot)
	kinds := discoverKindsForCache(ctx, projectRoot)
	cancel()
	// When discovery times out or returns nil, use kind mapper so we still build and populate the cache.
	// Use a bounded context for EnsureReady so we don't block indefinitely or delay shutdown.
	kindMapperTimeout := 10 * time.Second
	if kindMapperTimeout > discoveryTimeout {
		kindMapperTimeout = discoveryTimeout
	}
	if kinds == nil {
		kmCtx, kmCancel := stdcontext.WithTimeout(stdcontext.Background(), kindMapperTimeout)
		if km := objects.GetGlobalKindMapper(); km != nil && km.EnsureReady(kmCtx) == nil {
			kinds = km.GetAllKinds()
		}
		kmCancel()
		if kinds == nil {
			kinds = []string{}
		}
	}
	revIndex := storage.GetGlobalReverseReferenceIndex()
	if err := revIndex.BuildFromScan(projectRoot, processDir, kinds); err != nil {
		l := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		logging.Fluent(l).Debug("Reverse reference index sync build failed").
			ProjectRoot(projectRoot).
			WithError(err).
			Log()
		return false
	}
	if err := revIndex.SaveCache(projectRoot); err != nil {
		l := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		logging.Fluent(l).Debug("Reverse reference index sync save failed").
			ProjectRoot(projectRoot).
			WithError(err).
			Log()
		return false
	}
	return true
}

// reverseReferenceIndexDiscoveryTimeout bounds kind discovery when building the reverse reference index; BuildFromScan then runs synchronously.
const reverseReferenceIndexDiscoveryTimeout = 30 * time.Second

// ensureReverseReferenceIndexSync builds and saves the reverse index when missing, using kinds from the object ID
// cache when non-empty so BuildFromScan does not depend on a separate discovery pass.
func ensureReverseReferenceIndexSync(projectRoot, processDir string, kinds []string) error {
	revIndex := storage.GetGlobalReverseReferenceIndex()
	if revLoaded, _ := revIndex.LoadCache(projectRoot); revLoaded {
		return nil
	}
	if len(kinds) == 0 {
		if tryBuildAndSaveReverseReferenceIndexSync(projectRoot, reverseReferenceIndexDiscoveryTimeout) {
			return nil
		}
		return errfmt.Errorf("reverse reference index: sync with discovery failed")
	}
	if err := revIndex.BuildFromScan(projectRoot, processDir, kinds); err != nil {
		return err
	}
	if err := revIndex.SaveCache(projectRoot); err != nil {
		return err
	}
	return nil
}

// triggerBackgroundReverseReferenceIndexBuild builds and saves the reverse reference index in the background.
// Call when object ID cache is loaded but reverse ref index is missing (e.g. first run after adding the feature).
// Tries sync build first (with timeout) so the cache file exists before the process exits; falls back to background if sync fails or times out.
func triggerBackgroundReverseReferenceIndexBuild(projectRoot string) {
	projectRoot = resolveProjectRoot(projectRoot)
	if projectRoot == emptyValue {
		return
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	if tryBuildAndSaveReverseReferenceIndexSync(projectRoot, reverseReferenceIndexDiscoveryTimeout) {
		logging.Fluent(logger).Debug("Reverse reference index built and saved synchronously").
			ProjectRoot(projectRoot).
			Log()
		return
	}
	bud := goroutinelabels.DefaultBudget()
	if bud == nil {
		projectCacheBgIncReverseRef(projectRoot)
	}
	builder := goroutinelabels.NewGoroutine("reverse_reference_index_background_build", "building reverse reference index in background")
	if bud != nil {
		builder = builder.WithBudget(bud)
	}
	builder.StartWithContext(stdcontext.Background(), func(ctx stdcontext.Context) error {
		if bud != nil {
			projectCacheBgIncReverseRef(projectRoot)
		}
		defer projectCacheBgDecReverseRef(projectRoot)
		processDir := datacell.ProcessPrimaryDir(projectRoot)
		kinds := discoverKindsForCache(ctx, projectRoot)
		if kinds == nil {
			if km := objects.GetGlobalKindMapper(); km != nil && km.EnsureReady(ctx) == nil {
				kinds = km.GetAllKinds()
			}
			if kinds == nil {
				kinds = []string{}
			}
		}
		revIndex := storage.GetGlobalReverseReferenceIndex()
		if err := revIndex.BuildFromScan(projectRoot, processDir, kinds); err != nil {
			logging.Fluent(logger).Debug("Background reverse reference index build failed (non-blocking)").
				ProjectRoot(projectRoot).
				WithError(err).
				Log()
			return nil
		}
		if err := revIndex.SaveCache(projectRoot); err != nil {
			logging.Fluent(logger).Debug("Background reverse reference index save failed (non-blocking)").
				ProjectRoot(projectRoot).
				WithError(err).
				Log()
		}
		return nil
	})
}

// warmStorageHolder wraps storage for warmStorageForNextBuild so we can clear with storage=nil (atomic.Value cannot store nil).
type warmStorageHolder struct {
	storage storage.ObjectStorageProvider
}

// getWarmStorageForBuild returns the optional storage to use for warm (set by EnsureObjectIDCacheReady caller).
func (c *ObjectIDCache) getWarmStorageForBuild() storage.ObjectStorageProvider {
	if v := c.warmStorageForNextBuild.Load(); v != nil {
		if h, ok := v.(*warmStorageHolder); ok && h.storage != nil {
			return h.storage
		}
	}
	return nil
}

// warmWorkerCap returns bounded concurrency for warm-phase workers (pre-init and merge).
// Scaled (4–16 workers) to maximize hardware utilization during CAS index warming.
func warmWorkerCap() int {
	n := runtime.NumCPU()
	if n < 4 {
		n = 4
	}
	if n > 16 {
		n = 16
	}
	return n
}

// warmCASIndexesFromCache populates CAS index files so discovery (ListPathsForDiscovery) sees
// all objects. Avoids O(entries × files_per_kind) by doing one scan per kind instead of
// GetFilePathForObject per entry when path is missing. See docs/MY_AI_AGENT_LIES.md.
// When storageForWarm is non-nil (async check path), that instance is warmed so discovery/ref validation use it.
// When ctx is cancelled (e.g. SIGINT via signal.NotifyContext), warm returns early so the check can exit gracefully.
// flushTimeout is how long to wait for CAS index writes to flush; 0 means default (60s). Tests can pass a shorter value.
func warmCASIndexesFromCache(ctx stdcontext.Context, projectRoot string, cache *ObjectIDCache, storageForWarm storage.ObjectStorageProvider, flushTimeout time.Duration) {
	if ctx != nil && ctx.Err() != nil {
		return
	}

	storageProvider := storageForWarm
	createdStorageProvider := false
	if storageProvider == nil {
		storageProvider = GetStorageProviderForCache(projectRoot)
		createdStorageProvider = storageProvider != nil
	}
	if storageProvider == nil {
		return
	}

	// If we created the storage provider just for warming, shut it down so WAL handles
	// don't keep TempDir trees from being removed.
	if createdStorageProvider {
		shutdownTimeout := flushTimeout
		if shutdownTimeout == 0 {
			// Warm path is best-effort; use a bounded timeout to avoid test hangs.
			shutdownTimeout = 20 * time.Second
		}
		shutdownCtx, cancel := stdcontext.WithTimeout(stdcontext.Background(), shutdownTimeout)
		defer cancel()

		if fileStorage, ok := storageProvider.(*storage.FileObjectStorage); ok {
			defer func() { _ = fileStorage.Shutdown(shutdownCtx) }()
		} else {
			type shutdownable interface {
				Shutdown(stdcontext.Context) error
			}
			if s, ok := storageProvider.(shutdownable); ok {
				defer func() { _ = s.Shutdown(shutdownCtx) }()
			}
		}
	}

	fileStorage := storage.UnwrapToFileObjectStorage(storageProvider)
	if fileStorage == nil {
		return
	}
	entries := cache.GetAll()

	// Build id->filePath per kind from the existing object ID cache only. We never scan directories
	// or read files here—warming uses only pre-existing cache data. Cache load already rejects
	// any cache with missing FilePath, so we always have paths after load or build.
	byKind := make(map[string]map[string]string) // kind -> id -> filePath
	for _, entry := range entries {
		if entry == nil || entry.FilePath == emptyValue || entry.ID == emptyValue || entry.Kind == emptyValue {
			continue
		}
		if byKind[entry.Kind] == nil {
			byKind[entry.Kind] = make(map[string]string)
		}
		byKind[entry.Kind][entry.ID] = entry.FilePath
	}

	// Ensure kind mapper is ready via component loader pattern (timeouts, telemetry).
	// Then pre-initialize per-kind so warm workers get cache hits and don't trigger Initialize() under lock.
	if ctx != nil && ctx.Err() != nil {
		return
	}
	if err := objects.GetGlobalKindMapper().EnsureReady(ctx); err != nil {
		kmLogger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		logging.Fluent(kmLogger).Debug("Kind mapper EnsureReady failed, continuing with per-kind pre-init").
			WithError(err).
			Log()
	}
	// Pre-initialize each kind so later warm workers hit cache (no Initialize() under lock).
	preInitWorkers := warmWorkerCap()
	numKinds := len(byKind)
	preInitQueueSize := min(numKinds, 256)
	poolCtx := ctx
	if poolCtx == nil {
		poolCtx = stdcontext.Background()
	}
	poolPre := goroutinelabels.NewPool(goroutinelabels.DefaultBudget(), "warm_preinit", "pre-initialize kind mapper", preInitWorkers, preInitQueueSize)
	poolPre.Start(poolCtx)
	var preInitWg sync.WaitGroup
	for kind := range byKind {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		k := kind
		preInitWg.Add(1)
		_ = poolPre.Submit(poolCtx, func(stdcontext.Context) error {
			defer preInitWg.Done()
			_ = objects.GetDirectoryFromKind(k)
			return nil
		})
	}
	preInitWg.Wait()
	poolPre.Stop()

	cache.notifyCacheProgress("warming", fmt.Sprintf("Merging CAS indexes for %d kinds...", numKinds))
	process.TouchMeaningfulActivity()

	// Warm CAS indexes from cache only (one merge per kind; no per-file disk reads).
	warmWorkers := warmWorkerCap()
	warmQueueSize := min(numKinds, 256)
	poolWarm := goroutinelabels.NewPool(goroutinelabels.DefaultBudget(), "warm_cas", "warming CAS index", warmWorkers, warmQueueSize)
	poolWarm.Start(poolCtx)
	var warmWg sync.WaitGroup
	var warmTouch atomic.Uint64
	for kind, idToPath := range byKind {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		k, paths := kind, idToPath
		warmWg.Add(1)
		_ = poolWarm.Submit(poolCtx, func(taskCtx stdcontext.Context) error {
			defer warmWg.Done()
			if taskCtx != nil && taskCtx.Err() != nil {
				return nil
			}
			_ = fileStorage.EnsureCASIndexFromPaths(k, paths)
			// Heartbeat every few kinds so interactive idle watchdog does not cancel mid-warm.
			if warmTouch.Add(1)%4 == 0 {
				process.TouchMeaningfulActivity()
			}
			return nil
		})
	}
	warmWg.Wait()
	poolWarm.Stop()
	process.TouchMeaningfulActivity()

	if ctx != nil && ctx.Err() != nil {
		return
	}
	// Bounded total flush so many kinds don't cause N×timeout wait (was 5s per kind → minutes)
	if flushTimeout == 0 {
		flushTimeout = 60 * time.Second
	}
	if err := caspkg.FlushAllListingIndexesForProjectRootWithTimeout(projectRoot, flushTimeout); err != nil {
		flushLogger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		logging.Fluent(flushLogger).Warn("CAS index flush did not complete within timeout; discovery may see partial state").
			WithError(err).
			Log()
	}
	process.TouchMeaningfulActivity()
	cache.notifyCacheProgress("warming", "CAS indexes warmed")
	logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)).Info("CAS indexes warmed")
}

// getStorageProviderForCache gets a storage provider for cache coordination events
// Returns nil if unavailable (best effort - coordination is optional)
func GetStorageProviderForCache(projectRoot string) storage.ObjectStorageProvider {
	stdctx := pkgctx.NewSystemContext()
	storageFactory, err := storage.NewStorageFactory(stdctx, projectRoot)
	if err != nil || storageFactory == nil {
		return nil
	}
	return storageFactory.GetStorage()
}
