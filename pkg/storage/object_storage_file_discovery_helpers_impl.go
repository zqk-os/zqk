package storage

import (
	"context"
	"path/filepath"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/crud"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func (f *FileObjectStorage) countHashNamedFilesInKindDir(kind string) (int, error) {
	if !f.usesContentAddressableStorage(kind) {
		return 0, nil
	}
	if StreamStorageEnabledForKind(kind) {
		return 0, nil
	}
	kindDir := f.GetKindDir(kind)
	if kindDir == "" {
		return 0, nil
	}
	if _, err := fileutil.Stat(kindDir); err != nil {
		if fileutil.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	var count int
	countDir := func(dir string) error {
		entries, err := fileutil.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			stem := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			if crud.IsHashBasedFilename(stem) {
				count++
			}
		}
		return nil
	}
	if err := countDir(kindDir); err != nil {
		return 0, err
	}
	if f.usesBucketedStorage(kind, kindDir) {
		entries, err := fileutil.ReadDir(kindDir)
		if err != nil {
			return 0, err
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if err := countDir(filepath.Join(kindDir, e.Name())); err != nil {
				return 0, err
			}
		}
	}
	return count, nil
}

// getKindDirMtimes returns the kind dir mtime and, for bucketed storage, each bucket subdir's mtime.
// Caller can use these to detect if any file was added/removed without running a full count.
func (f *FileObjectStorage) getKindDirMtimes(kind string) (kindDirMtime int64, bucketMtimes map[string]int64, err error) {
	if !f.usesContentAddressableStorage(kind) {
		return 0, nil, nil
	}
	kindDir := f.GetKindDir(kind)
	if kindDir == "" {
		return 0, nil, nil
	}
	info, err := fileutil.Stat(kindDir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return 0, nil, nil
		}
		return 0, nil, err
	}
	kindDirMtime = info.ModTime().UnixNano()
	if !f.usesBucketedStorage(kind, kindDir) {
		return kindDirMtime, nil, nil
	}
	entries, err := fileutil.ReadDir(kindDir)
	if err != nil {
		return kindDirMtime, nil, err
	}
	bucketMtimes = make(map[string]int64)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		subDir := filepath.Join(kindDir, e.Name())
		subInfo, statErr := fileutil.Stat(subDir)
		if statErr != nil {
			continue
		}
		bucketMtimes[e.Name()] = subInfo.ModTime().UnixNano()
	}
	return kindDirMtime, bucketMtimes, nil
}

// mtimesMatch returns true if the current mtimes match the cached entry (no directory change).
func mtimesMatch(entry *casCountCacheEntry, kindDirMtime int64, bucketMtimes map[string]int64) bool {
	if entry == nil {
		return false
	}
	if entry.KindDirMtime != kindDirMtime {
		return false
	}
	if len(entry.BucketMtimes) != len(bucketMtimes) {
		return false
	}
	for name, mtime := range bucketMtimes {
		if entry.BucketMtimes[name] != mtime {
			return false
		}
	}
	return true
}

// getCachedDiskCountOrScan returns the on-disk count of hash-named files for the kind.
// It uses a cache keyed by (projectRoot, kind) and the kind dir + bucket mtimes: when mtimes
// haven't changed, returns the cached count without scanning. When mtimes differ or cache miss,
// runs countHashNamedFilesInKindDir and updates the cache.
// Stream-backed kinds: do not scan YAML dir; return 0 (callers add stream count separately).
func (f *FileObjectStorage) getCachedDiskCountOrScan(kind string) (diskCount int, err error) {
	if StreamStorageEnabledForKind(kind) {
		return 0, nil
	}
	kindDirMtime, bucketMtimes, mtimeErr := f.getKindDirMtimes(kind)
	if mtimeErr != nil {
		return f.countHashNamedFilesInKindDir(kind)
	}
	key := f.projectRoot + "\x00" + kind
	if v, ok := casCountCache.Load(key); ok {
		entry := v.(*casCountCacheEntry)
		if mtimesMatch(entry, kindDirMtime, bucketMtimes) {
			return entry.DiskCount, nil
		}
	}
	diskCount, err = f.countHashNamedFilesInKindDir(kind)
	if err != nil {
		return 0, err
	}
	// Clone bucketMtimes so we don't retain a reference to a map that might be reused
	var bucketCopy map[string]int64
	if len(bucketMtimes) > 0 {
		bucketCopy = make(map[string]int64, len(bucketMtimes))
		for k, v := range bucketMtimes {
			bucketCopy[k] = v
		}
	}
	casCountCache.Store(key, &casCountCacheEntry{
		DiskCount:    diskCount,
		KindDirMtime: kindDirMtime,
		BucketMtimes: bucketCopy,
	})
	return diskCount, nil
}

// triggerBackgroundCASIndexRefresh enqueues a CAS index refresh for the given kind (INDEX_FIRST_LOW_CPU_SCAN_DESIGN).
// A single reconciliation worker processes jobs so background scan does not starve foreground. If not already
// in progress for this project+kind, enqueues; if queue is full, runs in a one-off goroutine so refresh is not dropped.
// List/count return immediately with current index; next list sees updated index after the worker runs.
func (f *FileObjectStorage) triggerBackgroundCASIndexRefresh(kind string) {
	key := f.projectRoot + "\x00" + kind
	if _, loaded := casIndexRefreshInProgress.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	fCopy := f
	kindCopy := kind
	run := func() {
		defer casIndexRefreshInProgress.Delete(key)
		var _err_83126154 = fCopy.EnsureCASIndexPopulatedFromScan(context.Background(), kindCopy)
		if _err_83126154 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83126154).Log()
		}
	}
	startCASReconciliationWorker()
	select {
	case casReconciliationQueue <- reconciliationJob{run: run}:
	default:
		goroutinelabels.NewGoroutine("storage", ConstStreamCasIndexRefreshFallback).StartSimple(func() {
			run()
		})
	}
}

// ListPathsForDiscovery returns all object file paths for a kind.
// All kinds are CAS: we use the CAS index (ID → path). Legacy path uses collectFilePathsWithStrategy for migration.
func (f *FileObjectStorage) ListPathsForDiscovery(ctx context.Context, kind string) ([]crud.PathWithID, error) {
	kindDir := f.GetKindDir(kind)
	if kindDir == "" {
		return nil, errfmt.Errorf(ConstStreamUnknownObjectKindStr, kind)
	}

	if _, err := fileutil.Stat(kindDir); err != nil {
		if fileutil.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	eventLogger := logging.NewEventLogger(ctx)

	// All kinds are CAS: use index for path per ID.
	if f.usesContentAddressableStorage(kind) {
		cas, err := f.getContentAddressableStorage(kind)
		if err != nil {
			return nil, err
		}
		ids, err := cas.ListIDs()
		if err != nil {
			return nil, err
		}
		out := make([]crud.PathWithID, 0, len(ids))
		for _, id := range ids {
			path, err := cas.GetFilePathForID(id)
			if err != nil {
				continue
			}
			out = append(out, crud.PathWithID{Path: path, ID: id})
		}
		return out, nil
	}

	// Legacy: use bucketing strategy when kind is not CAS (all kinds are CAS in practice).
	var bucketStrategy BucketStrategy
	if kind != objects.KindBucketingStrategy && kind != objects.KindSynonym {
		registry := f.getBucketStrategyRegistry(ctx)
		if registry != nil {
			strategy, err := registry.GetStrategyForKind(ctx, kind)
			if err == nil {
				bucketStrategy = strategy
			}
		}
	}
	filePaths, err := f.collectFilePathsWithStrategy(ctx, kind, kindDir, bucketStrategy, nil, eventLogger)
	if err != nil {
		return nil, err
	}
	out := make([]crud.PathWithID, 0, len(filePaths))
	for _, path := range filePaths {
		id := crud.GetObjectIDFromPath(path, kind)
		if id == emptyValue {
			continue
		}
		out = append(out, crud.PathWithID{Path: path, ID: id})
	}
	return out, nil
}
