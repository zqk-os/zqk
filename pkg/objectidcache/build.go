package objectidcache

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/migration/scanner"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// CacheBuildContext groups state for cache building
type CacheBuildContext struct {
	Cache        *ObjectIDCache
	ProjectRoot  string
	ForceRebuild bool
	Logger       logging.Logger
}

// initializeCacheBuildContext sets up the cache build context
func initializeCacheBuildContext(cache *ObjectIDCache, projectRoot string, forceRebuild bool) *CacheBuildContext {
	return &CacheBuildContext{
		Cache:        cache,
		ProjectRoot:  projectRoot,
		ForceRebuild: forceRebuild,
		Logger:       logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// tryLoadExistingCache attempts to load existing cache
func tryLoadExistingCache(ctx *CacheBuildContext) (bool, error) {
	if ctx.ForceRebuild {
		logging.Fluent(ctx.Logger).Debug("Force rebuild requested, clearing cache and rebuilding").Log()
		return false, nil
	}

	loaded, err := ctx.Cache.LoadCache(ctx.ProjectRoot)
	if err != nil {
		logging.Fluent(ctx.Logger).Debug("Cache load error, rebuilding cache").
			WithError(err).
			Log()
		return false, nil
	}

	if !loaded {
		logging.Fluent(ctx.Logger).Debug("Cache not found or invalid, rebuilding").Log()
		return false, nil
	}

	// ValidateAndCleanStale removes entries where file mtime doesn't match
	// This is important for cache accuracy, but we should be careful not to remove
	// entries unnecessarily. The mtime check has a 1-second tolerance.
	// CRITICAL: Get entry count before cleanup to detect if cleanup is too aggressive
	var entryCountBeforeCleanup int
	_ = concurrency.WithRLockTimeout(
		&ctx.Cache.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(ctx.Logger),
		LockNameCacheBuildGetCountBefore,
		func() error {
			entryCountBeforeCleanup = len(ctx.Cache.idToKind)
			return nil
		},
	)

	staleCount := ctx.Cache.ValidateAndCleanStale()
	if staleCount > 0 {
		logging.Fluent(ctx.Logger).Debug("Removed stale cache entries").
			Int("stale_count", staleCount).
			Int("entries_before", entryCountBeforeCleanup).
			Log()

		// Warn if cleanup removed too many entries (may indicate a problem)
		if staleCount > entryCountBeforeCleanup/10 {
			logging.Fluent(ctx.Logger).Warn("ValidateAndCleanStale removed a large number of entries - this may indicate file path or timing issues").
				Int("stale_count", staleCount).
				Int("entries_before", entryCountBeforeCleanup).
				Log()
		}

		// Only save if we removed entries (to avoid unnecessary writes)
		if err := ctx.Cache.SaveCache(ctx.ProjectRoot); err != nil {
			logging.Fluent(ctx.Logger).Warn("Failed to save cleaned cache").
				WithError(err).
				Log()
		}
	}

	var entryCount int
	_ = concurrency.WithRLockTimeout(
		&ctx.Cache.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(ctx.Logger),
		LockNameCacheBuildGetCountAfter,
		func() error {
			entryCount = len(ctx.Cache.idToKind)
			return nil
		},
	)

	if entryCount > 0 {
		logging.Fluent(ctx.Logger).Debug("Using cached object ID cache").
			EntryCount(entryCount).
			EntriesRemoved(staleCount).
			Log()
		return true, nil
	}

	logging.Fluent(ctx.Logger).Debug("Cache loaded but empty after cleanup, rebuilding").
		EntriesBeforeCleanup(entryCountBeforeCleanup).
		EntriesRemoved(staleCount).
		Log()
	return false, nil
}

// clearCacheForRebuild clears the cache before rebuilding and sets processDir for the build.
func clearCacheForRebuild(ctx *CacheBuildContext, kinds []string) {
	_ = concurrency.WithLockTimeout(
		&ctx.Cache.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(ctx.Logger),
		LockNameCacheBuildClear,
		func() error {
			ctx.Cache.byKind = make(map[string][]KindBucketEntry)
			for _, kind := range kinds {
				if !storage.IsHighVolumeKindForCache(kind) && kind != objects.KindBaseMetric {
					ctx.Cache.byKind[kind] = []KindBucketEntry{}
				}
			}
			ctx.Cache.idToKind = make(map[string]string)
			ctx.Cache.idToIndex = make(map[string]int)
			ctx.Cache.countByKind = nil
			ctx.Cache.processDir = datacell.ProcessPrimaryDir(ctx.ProjectRoot)
			return nil
		},
	)
	storage.InvalidateListCache()
}

// discoverKindsForCache discovers object kinds for cache building. If ctx is non-nil it is used
// to bound the call (avoids indefinite hang in FieldRegistry.LoadFields). Returns nil on timeout/cancel.
func discoverKindsForCache(ctx context.Context, projectRoot string) []string {
	processDir := datacell.ProcessPrimaryDir(projectRoot)
	if ctx != nil {
		return discoverObjectKindsWithContext(ctx, processDir)
	}
	return discoverObjectKinds(processDir)
}

// cacheJob represents a single cache building job
type cacheJob struct {
	kind    string
	kindDir string
}

// cacheWorker processes cache jobs
func cacheWorker(jobs <-chan cacheJob, results chan<- error, ctx *CacheBuildContext) {
	defer func() {
		if r := recover(); r != nil {
			results <- errfmt.Errorf("panic in cache worker: %v", r)
		}
	}()

	for job := range jobs {
		if err := processCacheJob(ctx, job); err != nil {
			results <- err
			continue
		}
		results <- nil
	}
}

// processCacheJob processes a single cache job and adds entries to the cache (v2 shape).
func processCacheJob(ctx *CacheBuildContext, job cacheJob) error {
	scnr := scanner.NewYAMLScanner(job.kindDir)
	files, err := scnr.Scan()
	if err != nil {
		return err
	}

	entries := make(map[string]*ObjectIDCacheEntry)
	for _, file := range files {
		entry, err := createCacheEntry(file, job.kind)
		if err != nil {
			continue
		}
		entries[file.ObjectID] = entry
	}

	// CRITICAL: Call AddEntriesFromBuild in the same goroutine that holds the lock.
	// WithLockTimeout runs its callback in a new goroutine; if the lock times out the parent
	// releases the lock while the child still runs, allowing another worker to run and causing
	// concurrent map writes (fatal error: concurrent map writes). Use RunInLockWithLogger so
	// the map writes happen while we hold the lock and no other worker can run AddEntriesFromBuild.
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.RunInLockWithLogger(
		&ctx.Cache.mu,
		LockNameCacheBuildStoreEntries,
		logging.NewLockLoggerAdapter(logger),
		func() error {
			ctx.Cache.AddEntriesFromBuild(ctx.ProjectRoot, entries)
			return nil
		},
	)

	return nil
}

// createCacheEntry creates a cache entry for a file. Uses file.Kind from scanner (directory or
// from the single read when we needed ID) so we do not read the file again — avoids 2x I/O per file.
// When the scanner infers a non-kind from a bucketed subdir (e.g. "2026-01" under change_journal),
// we use defaultKind so all entries under that kind directory are cached under the correct kind.
func createCacheEntry(file *scanner.YAMLFile, defaultKind string) (*ObjectIDCacheEntry, error) {
	actualKind := file.Kind
	if actualKind == emptyValue || objects.GetDirectoryFromKind(actualKind) == emptyValue {
		actualKind = defaultKind
	}

	return &ObjectIDCacheEntry{
		ID:       file.ObjectID,
		Kind:     actualKind,
		FilePath: file.Path,
		MTime:    time.Unix(file.ModTime, 0),
		Exists:   true,
	}, nil
}

// buildCacheInParallel builds the cache using parallel workers.
// Worker count capped at 2–4 per INDEX_FIRST_LOW_CPU_SCAN_DESIGN so background build does not starve foreground.
// High-volume kinds (audit_event, metrics, scheduler_job, etc.) are excluded: they use the high-volume event
// cache for Count/OldestIDs/retention; putting them in the object-id-cache would bloat it and duplicate data.
func buildCacheInParallel(ctx *CacheBuildContext, kinds []string) error {
	var jobs []cacheJob
	for _, kind := range kinds {
		if storage.IsHighVolumeKindForCache(kind) {
			continue // exclude audit_event, metrics, etc.; they use high-volume event cache
		}
		kindDir := kindDirectory(ctx.ProjectRoot, kind)
		if kindDir != emptyValue {
			jobs = append(jobs, cacheJob{kind: kind, kindDir: kindDir})
		}
	}
	if len(jobs) == 0 {
		return nil
	}

	numWorkers := min(4, max(2, runtime.NumCPU()/2))
	numWorkers = min(numWorkers, len(jobs))
	queueSize := min(len(jobs), 256)
	poolCtx := context.Background()
	pool := goroutinelabels.NewPool(goroutinelabels.DefaultBudget(), "cache_build_worker", "building object ID cache", numWorkers, queueSize)
	var wg sync.WaitGroup
	results := make(chan error, len(jobs))
	pool.Start(poolCtx)
	defer pool.Stop()
	for _, job := range jobs {
		wg.Add(1)
		err := pool.Submit(poolCtx, func(taskCtx context.Context) error {
			defer wg.Done()
			if err := processCacheJob(ctx, job); err != nil {
				results <- err
				return nil
			}
			results <- nil
			return nil
		})
		if err != nil {
			func() {
				defer wg.Done()
				if err := processCacheJob(ctx, job); err != nil {
					results <- err
				} else {
					results <- nil
				}
			}()
		}
	}
	wg.Wait()
	for i := 0; i < len(jobs); i++ {
		if err := <-results; err != nil {
			// Log but continue
		}
	}
	return nil
}

func kindDirectory(projectRoot, kind string) string {
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == emptyValue {
		return emptyValue
	}
	return datacell.CellCASPrimaryDir(projectRoot, dirName)
}
