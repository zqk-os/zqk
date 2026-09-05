// Extracted from pkg/storage/high_volume_event_cache.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	stdcontext "context"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/pipeline"
	"github.com/lanceman/zqk/pkg/storage/locknames"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/when"
	"gopkg.in/yaml.v3"
)

func (c *HighVolumeEventCache) BuildCache(ctx stdcontext.Context, projectRoot string, storageProvider ObjectStorageProvider) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	StorageLog(logger).Info(LogEventStorageHighVolumeCacheBuildingInfo).ProjectRoot(projectRoot).Log()

	done := make(chan error, 1)
	buildBudgetExceededErr := errfmt.Errorf(ConstMiscHighVolumeEventCacheBuildcachePipelineGo)
	newHighVolumeCacheGoroutine(ConstMiscHighVolumeEventCacheBuild, ConstMiscHighVolumeEventCacheBuildcachePipeline).
		WithBudgetExceededHandler(func() {
			done <- buildBudgetExceededErr
		}).
		StartSimple(func() {
			var err error

			type buildCachePipelineState struct {
				newCache   map[string]*HighVolumeEventCacheEntry
				newByTime  []*HighVolumeEventCacheEntry
				buildErr   error
				swapErr    error
				entryCount int
			}

			st := &buildCachePipelineState{}
			pl := pipeline.NewBuilder(pipelineKindHighVolumeEventCacheBuild, logger).
				WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
				WithProfile(string(pkgctx.ProfileSystem)).
				AddStage(pipeline.StageIngest, func(pctx *pipeline.Context, payload any) (any, error) {
					newCache, newByTime, buildErr := c.buildCacheData(ctx, projectRoot, storageProvider, logger)
					st.newCache = newCache
					st.newByTime = newByTime
					st.buildErr = buildErr
					return st, nil
				}).
				AddStage(pipeline.StageCommit, func(pctx *pipeline.Context, payload any) (any, error) {
					if st.buildErr != nil {
						return st, nil
					}

					st.swapErr = concurrency.WithLockTimeout(
						&c.mu,
						pkgctx.NewSystemContext(),
						nil,
						logging.NewLockLoggerAdapter(logger),
						locknames.LockNameHighVolumeCacheBuildSwap,
						func() error {
							c.cache = st.newCache
							c.byTime = st.newByTime
							c.evictOldestToCap(maxHighVolumeEventCacheEntries)
							st.entryCount = len(c.cache)
							c.metadata = &HighVolumeEventCacheMetadata{
								Version:     highVolumeEventCacheVersion,
								BuildTime:   time.Now().UTC(),
								ProjectRoot: projectRoot,
								EntryCount:  st.entryCount,
								HasStatus:   true,
							}
							return nil
						},
					)
					return st, nil
				}).
				AddStage(pipeline.StageFinalize, func(pctx *pipeline.Context, payload any) (any, error) {
					if st.buildErr != nil {
						return st, nil
					}
					if st.swapErr != nil {
						return st, nil
					}

					if saveErr := c.SaveCache(projectRoot); saveErr != nil {
						StorageLog(logger).Warn(LogEventStorageHighVolumeCacheSaveFailedWarn).WithError(saveErr).Log()
					}

					StorageLog(logger).Info(LogEventStorageHighVolumeCacheBuiltInfo).
						Int("entry_count", st.entryCount).
						ProjectRoot(projectRoot).
						Log()
					return st, nil
				}).
				Build()

			out, runErr := pl.Run(&pipeline.Context{Ctx: ctx, Outcome: make(map[string]any)}, st)
			_ = out
			when.When(func() bool { return runErr != nil }).Then(func() {
				err = runErr
			}).OrElseWhen(func() bool { return st.buildErr != nil }).Then(func() {
				err = st.buildErr
			}).OrElseWhen(func() bool { return st.swapErr != nil }).Then(func() {
				err = st.swapErr
			}).Run()

			defer func() { done <- err }()
		})

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// buildCacheData builds the cache. Prefers index-based path (CAS ListIDs + parallel read) to avoid
// List()'s 500s legacy scan and 47k full reads; falls back to List when not FileObjectStorage.
func (c *HighVolumeEventCache) buildCacheData(ctx stdcontext.Context, _ string, storageProvider ObjectStorageProvider, logger logging.Logger) (map[string]*HighVolumeEventCacheEntry, []*HighVolumeEventCacheEntry, error) {
	newCache := make(map[string]*HighVolumeEventCacheEntry)
	newByTime := make([]*HighVolumeEventCacheEntry, 0)

	if fileStorage, ok := storageProvider.(*FileObjectStorage); ok {
		var totalEntries atomic.Uint64
		kindPar := getHighVolumeCacheKindParallelism()
		sem := make(chan struct{}, kindPar)
		var mergeMu sync.Mutex
		var wg sync.WaitGroup
		var firstErr error
		var firstErrOnce sync.Once
		setFirstErr := func(e error) {
			if e == nil {
				return
			}
			firstErrOnce.Do(func() { firstErr = e })
		}
		kindBudgetExceededErr := errfmt.Errorf(ConstMiscHighVolumeEventCacheParallelKindBuildGor)
		for _, k := range HighVolumeKindsForCacheBuild() {
			kind := k
			if ctx != nil && ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			newHighVolumeCacheGoroutine(ConstMiscHighVolumeCacheKindWorker, ConstMiscParallelHighVolumeCacheBuildByKind).
				WithWaitGroup(&wg).
				WithBudgetExceededHandler(func() {
					setFirstErr(kindBudgetExceededErr)
				}).
				StartSimple(func() {
					sem <- struct{}{}
					defer func() { <-sem }()

					if ctx != nil && ctx.Err() != nil {
						setFirstErr(ctx.Err())
						return
					}
					kindEntries, buildErr := c.buildOneKindForHighVolumeCache(ctx, fileStorage, kind, logger)
					if buildErr != nil {
						setFirstErr(buildErr)
						return
					}
					mergeMu.Lock()
					defer mergeMu.Unlock()
					if ctx != nil && ctx.Err() != nil {
						setFirstErr(ctx.Err())
						return
					}
					for _, e := range kindEntries {
						newCache[e.ID] = e
						newByTime = append(newByTime, e)
					}
					totalEntries.Add(uint64(len(kindEntries)))
				})
		}
		waitDone := make(chan struct{})
		goroutinelabels.NewGoroutine("storage.high_volume_cache_wait", "waiting for high volume cache workers").
			StartSimple(func() {
				wg.Wait()
				close(waitDone)
			})
		select {
		case <-waitDone:
		case <-time.After(30 * time.Second):
			return nil, nil, errfmt.Errorf("high volume event cache build timed out waiting for worker goroutines")
		}
		if firstErr != nil {
			return nil, nil, firstErr
		}
		if totalEntries.Load() > 0 {
			sort.Slice(newByTime, func(i, j int) bool {
				return newByTime[i].CreatedAt.Before(newByTime[j].CreatedAt)
			})
			StorageLog(logger).Info(LogEventStorageHighVolumeCacheBuiltAllKindsInfo).
				EntryCount(int(totalEntries.Load())).
				Log()
			return newCache, newByTime, nil
		}
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()
	for _, kind := range []string{objects.KindAuditEvent} {
		if ctx != nil && ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		listResult, err := storageProvider.List(ctx, secCtx, storageCtx, ListFilter{
			Kind:  kind,
			Limit: 50000,
		})
		if err != nil {
			StorageLog(logger).Warn(LogEventStorageHighVolumeCacheListFailedWarn).Kind(kind).WithError(err).Log()
			continue
		}
		StorageLog(logger).Info(LogEventStorageHighVolumeCacheBuildingFromListInfo).
			Kind(kind).
			Int("event_count", len(listResult.Objects)).
			Log()
		for _, obj := range listResult.Objects {
			if ctx != nil && ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			id, _ := obj[objects.FieldKeyID].(string)
			if id == emptyValue {
				continue
			}
			var createdAt time.Time
			if createdAtStr := objects.GetString(obj, objects.FieldKeyCreatedAt); createdAtStr != "" {
				if t, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
					createdAt = t
				}
			}
			if createdAt.IsZero() {
				createdAt = time.Now().UTC()
			}
			eventType, _ := obj[objects.FieldKeyEventType].(string)
			filePath := ""
			if fileStorage, ok := storageProvider.(*FileObjectStorage); ok {
				if path, err := fileStorage.GetFilePathForObject(id, kind); err == nil {
					filePath = path
				}
			}
			mtime := time.Now().UTC()
			if filePath != emptyValue {
				statPath := filePath
				if seg, _, ok := StreamPathAndOffset(filePath); ok && seg != emptyValue {
					statPath = seg
				}
				if info, err := fileutil.Stat(statPath); err == nil {
					mtime = info.ModTime()
				}
			}
			entry := &HighVolumeEventCacheEntry{
				ID:        id,
				Kind:      kind,
				CreatedAt: createdAt,
				EventType: eventType,
				FilePath:  filePath,
				MTime:     mtime,
				Exists:    true,
			}
			newCache[id] = entry
			newByTime = append(newByTime, entry)
		}
	}
	sort.Slice(newByTime, func(i, j int) bool {
		return newByTime[i].CreatedAt.Before(newByTime[j].CreatedAt)
	})
	return newCache, newByTime, nil
}

// buildOneKindForHighVolumeCache aggregates CAS index entries and stream overlay for one kind.
// Index/list errors are non-fatal (logged, empty/partial merge). Returns (nil, err) only for context cancellation.
func (c *HighVolumeEventCache) buildOneKindForHighVolumeCache(ctx stdcontext.Context, fileStorage *FileObjectStorage, kind string, logger logging.Logger) (map[string]*HighVolumeEventCacheEntry, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	kindEntries := make(map[string]*HighVolumeEventCacheEntry)
	indexEntries, buildErr := c.buildCacheFromIndex(ctx, fileStorage, kind, logger)
	if buildErr != nil {
		StorageLog(logger).Debug(LogEventStorageHighVolumeCacheSkipKindNoCASEmptyDebug).
			Kind(kind).
			WithError(buildErr).
			Log()
	}
	for _, e := range indexEntries {
		kindEntries[e.ID] = e
	}
	if StreamStorageEnabledForKind(kind) {
		streamEntries, streamErr := fileStorage.BuildHighVolumeCacheEntriesFromStream(ctx, kind, logger)
		if streamErr != nil && ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		for _, e := range streamEntries {
			kindEntries[e.ID] = e
		}
	}
	return kindEntries, nil
}

// buildCacheFromIndex builds cache entries from CAS index (ListIDs) + parallel minimal read. No legacy scan.
func (c *HighVolumeEventCache) buildCacheFromIndex(ctx stdcontext.Context, f *FileObjectStorage, kind string, logger logging.Logger) ([]*HighVolumeEventCacheEntry, error) {
	cas, err := f.GetContentAddressableStorage(kind)
	if err != nil || cas == nil {
		return nil, err
	}
	ids, err := cas.ListIDs()
	if err != nil || len(ids) == 0 {
		return nil, err
	}

	if ctx != nil {
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < 2*time.Minute && len(ids) > 100000 {
			ids = ids[:100000]
			StorageLog(logger).Info(LogEventStorageHighVolumeCacheBuildingCappedTimeoutInfo).Capped(100000).Log()
		}
	}
	workers := getHighVolumeCacheBuildWorkers()
	type result struct {
		entry *HighVolumeEventCacheEntry
	}
	workCh := make(chan string, len(ids))
	for _, id := range ids {
		workCh <- id
	}
	close(workCh)
	resultCh := make(chan result, workers*2)
	for w := 0; w < workers && w < len(ids); w++ {
		newHighVolumeCacheGoroutine(ConstMiscHighVolumeCacheBuildWorker, ConstMiscBuildCacheFromIndex).
			StartSimple(func() {
				for id := range workCh {
					if ctx != nil && ctx.Err() != nil {
						resultCh <- result{}
						return
					}
					data, err := cas.Read(id)
					if err != nil {
						resultCh <- result{}
						continue
					}
					var obj map[string]any
					if yaml.Unmarshal(data, &obj) != nil {
						resultCh <- result{}
						continue
					}
					var createdAt time.Time
					if s := objects.GetString(obj, objects.FieldKeyCreatedAt); s != "" {
						if t, err := time.Parse(time.RFC3339, s); err == nil {
							createdAt = t
						}
					}
					if createdAt.IsZero() {
						createdAt = time.Now().UTC()
					}
					eventType, _ := obj[objects.FieldKeyEventType].(string)
					status, _ := obj[objects.FieldKeyStatus].(string)
					filePath := ""
					if path, err := f.GetFilePathForObject(id, kind); err == nil {
						filePath = path
					}
					mtime := time.Now().UTC()
					if filePath != emptyValue {
						statPath := filePath
						if seg, _, ok := StreamPathAndOffset(filePath); ok && seg != emptyValue {
							statPath = seg
						}
						if info, err := fileutil.Stat(statPath); err == nil {
							mtime = info.ModTime()
						}
					}
					resultCh <- result{entry: &HighVolumeEventCacheEntry{
						ID: id, Kind: kind, CreatedAt: createdAt, EventType: eventType, Status: status,
						FilePath: filePath, MTime: mtime, Exists: true,
					}}
				}
			})
	}
	var entries []*HighVolumeEventCacheEntry
	for i := 0; i < len(ids); i++ {
		r := <-resultCh
		if r.entry != nil {
			entries = append(entries, r.entry)
		}
	}
	return entries, nil
}
