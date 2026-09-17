package storage

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/process"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/when"
)

func (f *FileObjectStorage) countFromCASIndex(ctx context.Context, kind string, kindDir string) (int, error) {
	cas, err := f.getContentAddressableStorage(kind)
	if err != nil {
		return 0, errfmt.Newf(ErrMsgGetCAS).Wrap(err)
	}
	casIDs, err := cas.ListIDs()
	if err != nil {
		return 0, errfmt.Newf(ConstStreamFailedToListIdsFromContentAddressableStorage).Wrap(err)
	}

	// If index is empty, use cached disk count for fast response and trigger background refresh
	if len(casIDs) == 0 {
		// Use cached disk count for immediate response (fast path)
		cachedCount, cacheErr := f.getCachedDiskCountOrScan(kind)
		if cacheErr == nil && cachedCount > 0 {
			// Trigger background refresh so next count/list sees updated index
			f.triggerBackgroundCASIndexRefresh(kind)
			// Also scan ID-based files for legacy objects (skip for stream-backed kinds; no YAML read)
			idSet := make(map[string]bool)
			if !StreamStorageEnabledForKind(kind) {
				when.When(func() bool { return f.usesBucketedStorage(kind, kindDir) }).Then(func() {
					for id := range f.ScanIDBasedFilesRecursive(kindDir, kind) {
						idSet[id] = true
					}
				}).OrElse(func() {
					for _, id := range f.scanIDBasedFiles(kindDir, kind) {
						idSet[id] = true
					}
				}).Run()
			}
			n := cachedCount + len(idSet)
			if StreamStorageEnabledForKind(kind) {
				n += len(ListStreamIDsFromPersistentRegistry(f.projectRoot, kind))
			}
			return n, nil
		}
		var _err_83038590 = f.EnsureCASIndexPopulatedFromScan(ctx, kind)
		if _err_83038590 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83038590).Log()
		}
		casIDs, err = cas.ListIDs()
		if err != nil {
			return 0, errfmt.Newf(ConstStreamFailedToListIdsFromContentAddressableStorage).Wrap(err)
		}
	}

	// Fast path: high-volume CAS-only kinds (audit_event, *_metric) with large index. Skip reconcile and legacy
	// scan so Count() returns in milliseconds instead of 500s+ (catch_up phase was spending 8+ min on Count alone).
	if (kind == objects.KindAuditEvent || (len(kind) > 7 && kind[len(kind)-7:] == "_metric")) && len(casIDs) >= highVolumeCASCountFastPathThreshold {
		n := len(casIDs)
		if StreamStorageEnabledForKind(kind) {
			n += len(ListStreamIDsFromPersistentRegistry(f.projectRoot, kind))
		}
		return n, nil
	}

	// Reconcile: if disk has many more hash-named files than index, index is stale (e.g. files added elsewhere or index lost)
	// Repopulate index from disk so we don't abandon data and Count/List stay accurate. Respect ctx so job timeout aborts long scans.
	if diskCount, diskErr := f.getCachedDiskCountOrScan(kind); diskErr == nil && diskCount > len(casIDs)+casIndexReconcileDelta {
		var _err_83041010 = f.EnsureCASIndexPopulatedFromScan(ctx, kind)
		if _err_83041010 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83041010).Log()
		}
		casIDs, err = cas.ListIDs()
		if err != nil {
			return 0, errfmt.Newf(ConstStreamFailedToListIdsAfterReconciliation).Wrap(err)
		}
	}

	// Index has entries: use index as source of truth, only scan ID-based files for legacy objects
	idSet := make(map[string]bool)
	for _, id := range casIDs {
		idSet[id] = true
	}
	// Scan ID-based files only if index was empty (legacy migration case)
	// When index is populated, CAS is the source of truth
	// Stream-backed kinds: do not read YAML dir; source of truth is stream registry.
	// Optimization: Skip legacy scan if directory doesn't exist (no legacy files possible)
	if !StreamStorageEnabledForKind(kind) && len(casIDs) == 0 {
		if _, err := fileutil.Stat(kindDir); err == nil {
			when.When(func() bool { return f.usesBucketedStorage(kind, kindDir) }).Then(func() {
				for id := range f.ScanIDBasedFilesRecursive(kindDir, kind) {
					idSet[id] = true
				}
			}).OrElse(func() {
				for _, id := range f.scanIDBasedFiles(kindDir, kind) {
					idSet[id] = true
				}
			}).Run()
		}
		// Draft plane omitted from Count — same contract as List (post-membrane / CAS only).
	}
	casAndLegacy := len(idSet)
	// When stream storage is enabled for this kind, add count from segment files so Count() is accurate
	// when high-volume cache is empty (DATA_STORAGE_PRODUCTION_ROADMAP §1; no unbounded full scan).
	// Subtract stream-deleted count so logically deleted IDs are not counted.
	if StreamStorageEnabledForKind(kind) {
		streamCount := len(ListStreamIDsFromPersistentRegistry(f.projectRoot, kind))
		return casAndLegacy + streamCount, nil
	}
	return casAndLegacy, nil
}

// countFiles counts files in a directory without parsing
// Uses the same unified traversal logic as collectFilePaths
func (f *FileObjectStorage) countFiles(ctx context.Context, kindDir string, _ bool) (int, error) {
	EmitListCountWaitProgress(ctx)
	listCtx, err := AcquireListCountSlot(ctx)
	if err != nil {
		return 0, err
	}
	defer ReleaseListCountSlot(ctx)

	// Use unified file collection logic
	filePaths, err := f.collectFilePaths(listCtx, kindDir, nil, nil)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	return len(filePaths), nil
}

// countWithFilters counts objects matching filters
// This is similar to List() but only counts, doesn't load objects
//
//nolint:gocritic // Internal helper; filter passed by value for consistency with interface
func (f *FileObjectStorage) countWithFilters(ctx context.Context, kindDir string, filter ListFilter, _ bool) (int, error) {
	EmitListCountWaitProgress(ctx)
	listCtx, err := AcquireListCountSlot(ctx)
	if err != nil {
		return 0, err
	}
	defer ReleaseListCountSlot(ctx)

	// Collect file paths using unified traversal logic
	timeRange := f.extractTimeRangeFromFilters(filter.Filters)
	filePaths, err := f.collectFilePaths(listCtx, kindDir, timeRange, nil)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return 0, nil
		}
		return 0, errfmt.Newf(ConstStreamFailedToCollectFilePaths).Wrap(err)
	}

	// Bounded worker pool: fixed number of goroutines (not one per file) to avoid thread exhaustion.
	// workCh must be buffered to len(filePaths): we enqueue before workers start (same pattern as
	// CAS list). A small buffer (maxWorkers*2) deadlocks when file count exceeds the buffer —
	// default namespace-scoped object count hit this for kinds with >128 YAML files.
	// TRACK: BLI-1785903709847957000-8c7a991c
	count := 0
	maxWorkers := getListReadWorkers()
	results := make(chan bool, maxWorkers*2)
	workCh := make(chan string, len(filePaths))
	numWorkers := maxWorkers
	if len(filePaths) < numWorkers {
		numWorkers = len(filePaths)
	}
	if numWorkers == 0 {
		return 0, nil
	}

	for _, p := range filePaths {
		select {
		case <-listCtx.Done():
			return 0, listCtx.Err()
		case workCh <- p:
		}
	}
	close(workCh)

	var countWg sync.WaitGroup
	countWg.Add(numWorkers)
	countBud := goroutinelabels.DefaultBudget()
	var processedFiles int64
	progressFn := pkgctx.GetValidationProgress(listCtx)
	lastProgressTime := time.Now()
	var progressMu sync.Mutex
	totalFiles := len(filePaths)

	for w := 0; w < numWorkers; w++ {
		countWorkerBuilder := goroutinelabels.NewGoroutine(ConstStreamFileStorageCountFile, ConstStreamCountFilterWorker)
		if countBud != nil {
			countWorkerBuilder = countWorkerBuilder.WithBudget(countBud)
		}
		countWorkerBuilder.StartSimple(func() {
			defer countWg.Done()
			for filePath := range workCh {
				select {
				case <-listCtx.Done():
					return
				default:
				}

				curProcessed := atomic.AddInt64(&processedFiles, 1)
				if curProcessed%100 == 0 {
					process.TouchMeaningfulActivity()
				}
				if progressFn != nil && (curProcessed%250 == 0 || curProcessed == 1) {
					progressMu.Lock()
					if curProcessed == 1 || time.Since(lastProgressTime) >= 1*time.Second {
						lastProgressTime = time.Now()
						progressFn("storage.count", fmt.Sprintf("Filtering %s objects (%d/%d evaluated)...", filter.Kind, curProcessed, totalFiles))
					}
					progressMu.Unlock()
				}

				if IsObjectDraftPlanePath(f.projectRoot, filePath) {
					select {
					case <-listCtx.Done():
						return
					case results <- false:
					}
					continue
				}
				obj, err := f.readObjectFile(listCtx, filePath)
				if err != nil {
					select {
					case <-listCtx.Done():
						return
					case results <- false:
					}
					continue
				}
				idVal, _ := obj[objects.FieldKeyID].(string)
				obj = f.MaterializeCasYAMLMapAfterLoad(f.projectRoot, filter.Kind, idVal, obj)
				parsed, err := objects.ParseObject(obj)
				if err != nil {
					parsed = &objects.ParsedObject{Raw: obj}
				}
				matches := f.matchesFiltersParsed(parsed, filter.Filters)
				// Don't send after context cancelled (closer may have closed channel)
				select {
				case <-listCtx.Done():
					return
				case results <- matches:
				}
			}
		})
	}

	countCloserBud := goroutinelabels.DefaultBudget()
	var closeOnce sync.Once
	closeResults := func() { closeOnce.Do(func() { close(results) }) }
	wgDone := make(chan struct{})
	goroutinelabels.NewGoroutine(ConstStreamFileStorageCountWaiter, ConstStreamWaitingForCountWorkers).StartSimple(func() {
		countWg.Wait()
		close(wgDone)
	})
	countCloserBuilder := goroutinelabels.NewGoroutine(ConstStreamFileStorageCountResultsCloser, ConstStreamWaitingForCountWorkersAndClosingResultsChannel).
		WithCleanup(closeResults)
	if countCloserBud != nil {
		countCloserBuilder = countCloserBuilder.WithBudget(countCloserBud)
	}
	countCloserBuilder.StartSimple(func() {
		select {
		case <-listCtx.Done():
			closeResults()
		case <-wgDone:
			closeResults()
		}
	})

	for match := range results {
		if match {
			count++
		}
	}
	if err := listCtx.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

// extractTimeRangeFromFilters extracts time range from created_at filters
