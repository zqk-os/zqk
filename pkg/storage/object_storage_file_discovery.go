package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/process"
	"github.com/lanceman/zqk/pkg/when"
)

// casIndexRefreshInProgress tracks which project+kind are currently queued or running a background
// CAS index refresh so we don't start duplicate refreshes. Key: projectRoot + "\x00" + kind.
var casIndexRefreshInProgress sync.Map

// reconciliationJob is a unit of work for the single reconciliation worker (INDEX_FIRST_LOW_CPU_SCAN_DESIGN).
type reconciliationJob struct {
	run func()
}

// casReconciliationQueue feeds the single background worker that runs CAS index refresh.
// Bounded so we don't accumulate unbounded work; overflow falls back to one-off goroutine.
const casReconciliationQueueCap = 128

var (
	casReconciliationQueue = make(chan reconciliationJob, casReconciliationQueueCap)
	casReconciliationOnce  sync.Once
)

// startCASReconciliationWorker starts the single global worker that processes CAS index refresh jobs.
// One worker, optional sleep between jobs, so background scan does not starve foreground (INDEX_FIRST_LOW_CPU_SCAN_DESIGN).
func startCASReconciliationWorker() {
	casReconciliationOnce.Do(func() {
		goroutinelabels.NewGoroutine("storage", ConstStreamCasIndexReconciliationWorker).StartSimple(func() {
			for job := range casReconciliationQueue {
				if job.run != nil {
					job.run()
				}
				time.Sleep(150 * time.Millisecond)
			}
		})
	})
}

// casCountCacheEntry holds the last-known disk count and mtimes for a kind so we can skip
// countHashNamedFilesInKindDir when the directory(ies) haven't changed. For bucketed kinds,
// we store each bucket subdir's mtime because new files in an existing bucket don't change
// the parent kind dir's mtime.
type casCountCacheEntry struct {
	DiskCount    int
	KindDirMtime int64            // kind dir mtime (UnixNano)
	BucketMtimes map[string]int64 // bucket name -> mtime (UnixNano); nil for non-bucketed
}

var casCountCache sync.Map // key: projectRoot + "\x00" + kind, value: *casCountCacheEntry

// PathWithID holds a file path and its object ID for discovery.
type PathWithID struct {
	Path string
	ID   string
}

// GetFilePathForObject returns the file path for an object by ID and kind.
// Uses the same bucketing strategy and CAS resolution as the rest of the storage layer.
// This is the public API for callers (e.g. system check findObjectByID) that need path resolution.
func (f *FileObjectStorage) GetFilePathForObject(id, kind string) (string, error) {
	return f.getObjectFilePath(id, kind)
}

// idBelongsToKind returns true if the ID's inferred kind matches the given kind (or if we cannot infer kind).
// Used to keep CAS indexes per-kind: only IDs belonging to that kind are written to that kind's index.
func (f *FileObjectStorage) idBelongsToKind(id, kind string) bool {
	if f.idValidator == nil {
		return true
	}
	inferred := f.idValidator.InferKindFromID(id)
	if inferred == emptyValue {
		return true
	}
	return inferred == kind
}

// EnsureCASIndexFromPath updates the CAS index from a known file path when the path is hash-named.
// Used by warmCASIndexesFromCache to avoid re-scanning for each entry (O(1) per entry instead of O(files) per entry).
// If filePath is not a CAS hash-named file or kind is not CAS, this is a no-op.
// Only adds the mapping if the ID's inferred kind matches the index kind (keeps indexes per-kind).
func (f *FileObjectStorage) EnsureCASIndexFromPath(id, kind, filePath string) bool {
	if id == emptyValue || kind == emptyValue || filePath == emptyValue {
		return false
	}
	if !f.idBelongsToKind(id, kind) {
		return false
	}
	if !f.usesContentAddressableStorage(kind) {
		return false
	}
	base := filepath.Base(filePath)
	if !isHashBasedFilename(base) {
		return false
	}
	hash := strings.TrimSuffix(base, filepath.Ext(base))
	cas, err := f.getContentAddressableStorage(kind)
	if contentAddressableStorageOrIndexMissing(err, cas) {
		return false
	}
	kindDir := f.GetKindDir(kind)
	if kindDir == "" {
		return false
	}
	var bucketKey string
	if f.usesBucketedStorage(kind, kindDir) {
		fileDir := filepath.Dir(filePath)
		if fileDir != kindDir {
			bucketKey = filepath.Base(fileDir)
		}
	}
	when.When(func() bool { return bucketKey != emptyValue }).Then(func() {
		var _err_83115633 = cas.index.SetMapping(id, hash, bucketKey)
		if _err_83115633 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83115633).Log()
		}
	}).OrElse(func() {
		var _err_83115701 = cas.index.SetMapping(id, hash)
		if _err_83115701 != nil {
			logging.Fluent(logging.

				// EnsureCASIndexFromPaths updates the CAS index for a kind from many id->filePath entries in one load/merge/save.
				// Use this when warming from cache to avoid O(n) per-entry SetMapping (each of which does a full index read+write).
				GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83115701).Log()
		}
	}).Run()
	return true
}

func (f *FileObjectStorage) EnsureCASIndexFromPaths(kind string, idToFilePath map[string]string) error {
	if !f.usesContentAddressableStorage(kind) {
		return nil
	}
	if len(idToFilePath) == 0 {
		return nil
	}
	cas, err := f.getContentAddressableStorage(kind)
	if contentAddressableStorageOrIndexMissing(err, cas) {
		return err
	}
	kindDir := f.GetKindDir(kind)
	if kindDir == "" {
		return nil
	}
	mappings := make(map[string]string, len(idToFilePath))
	var bucketKeys map[string]string
	if f.usesBucketedStorage(kind, kindDir) {
		bucketKeys = make(map[string]string)
	}
	for id, filePath := range idToFilePath {
		if !f.idBelongsToKind(id, kind) {
			continue
		}
		base := filepath.Base(filePath)
		if !isHashBasedFilename(base) {
			continue
		}
		hash := strings.TrimSuffix(base, filepath.Ext(base))
		mappings[id] = hash
		if bucketKeys != nil {
			fileDir := filepath.Dir(filePath)
			if fileDir != kindDir {
				bucketKeys[id] = filepath.Base(fileDir)
			}
		}
	}
	if len(mappings) == 0 {
		return nil
	}
	return cas.index.SetMappings(mappings, bucketKeys)
}

// EnsureCASIndexPopulatedFromScan scans the kind directory for hash-named (CAS) files,
// reads each file's "id" field, and populates the CAS index via SetMappings. Used when
// the index is empty so list and get see the same set of objects, and by background refresh
// when disk and index counts disagree (see CAS_LIST_GET_CONSISTENCY.md).
//
// When two hash files contain the same logical id (e.g. old hash not yet removed after an update),
// the mapping for that id is taken from the file with the newer ModTime so the index cannot
// flip to stale content based on readdir order.
// If ctx is cancelled (e.g. job timeout), the scan aborts and returns ctx.Err() without updating the index.
func (f *FileObjectStorage) EnsureCASIndexPopulatedFromScan(ctx context.Context, kind string) error {
	if !f.usesContentAddressableStorage(kind) {
		return nil
	}
	// Stream-backed kinds: index is populated from stream registry; do not scan YAML dir.
	if StreamStorageEnabledForKind(kind) {
		return nil
	}
	cas, err := f.getContentAddressableStorage(kind)
	if contentAddressableStorageOrIndexMissing(err, cas) {
		return err
	}
	kindDir := f.GetKindDir(kind)
	if kindDir == "" {
		return nil
	}
	if _, err := os.Stat(kindDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	// best id -> winning hash for that id when multiple CAS files claim the same id (orphan + new hash).
	type casScanBest struct {
		hash      string
		mtime     int64
		bucketKey string
	}
	best := make(map[string]casScanBest)

	const abortCheckInterval = 500 // check ctx every N files so long scans can abort (e.g. SCH-002 timeout)
	n := 0
	scanDir := func(dir string, bucketKey string) bool {
		entries, readErr := os.ReadDir(dir)
		if readErr != nil {
			return true
		}
		for _, e := range entries {
			if ctx.Err() != nil {
				return false
			}
			if e.IsDir() {
				continue
			}
			name := e.Name()
			stem := strings.TrimSuffix(name, filepath.Ext(name))
			if !isHashBasedFilename(stem) {
				continue
			}
			n++
			if n%abortCheckInterval == 0 {
				process.TouchMeaningfulActivity()
				if ctx.Err() != nil {
					return false
				}
			}
			path := filepath.Join(dir, name)
			id, fileKind := f.getObjectIDAndKindFromPath(path, kind)
			if id == emptyValue {
				continue
			}
			// For shared dirs (e.g. metrics/), only index files whose kind matches this index.
			// Otherwise the same file would be added to every kind's index and Count would be N×files.
			if fileKind != emptyValue && fileKind != kind {
				continue
			}
			var mtime int64
			if fi, statErr := os.Stat(path); statErr == nil {
				mtime = fi.ModTime().UnixNano()
			}
			prev, ok := best[id]
			better := !ok || mtime > prev.mtime || (mtime == prev.mtime && stem > prev.hash)
			if !better {
				continue
			}
			best[id] = casScanBest{hash: stem, mtime: mtime, bucketKey: bucketKey}
		}
		return true
	}

	if !scanDir(kindDir, "") {
		return ctx.Err()
	}
	if f.usesBucketedStorage(kind, kindDir) {
		entries, err := os.ReadDir(kindDir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if !e.IsDir() {
				continue
			}
			subDir := filepath.Join(kindDir, e.Name())
			if !scanDir(subDir, e.Name()) {
				return ctx.Err()
			}
		}
	}

	if len(best) == 0 {
		return nil
	}
	mappings := make(map[string]string, len(best))
	var bucketKeys map[string]string
	if f.usesBucketedStorage(kind, kindDir) {
		bucketKeys = make(map[string]string)
	}
	for id, c := range best {
		mappings[id] = c.hash
		if bucketKeys != nil && c.bucketKey != emptyValue {
			bucketKeys[id] = c.bucketKey
		}
	}
	return cas.index.SetMappings(mappings, bucketKeys)
}

// countHashNamedFilesInKindDir returns the number of hash-named (CAS) files in the kind directory.
// Fast path: ReadDir only, no YAML reads. Used to detect index-vs-disk disparity for background refresh.
// Stream-backed kinds: do not scan YAML dir; return 0 (callers add stream count separately).
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
	if _, err := os.Stat(kindDir); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	var count int
	countDir := func(dir string) error {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			stem := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			if isHashBasedFilename(stem) {
				count++
			}
		}
		return nil
	}
	if err := countDir(kindDir); err != nil {
		return 0, err
	}
	if f.usesBucketedStorage(kind, kindDir) {
		entries, err := os.ReadDir(kindDir)
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
	info, err := os.Stat(kindDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil, nil
		}
		return 0, nil, err
	}
	kindDirMtime = info.ModTime().UnixNano()
	if !f.usesBucketedStorage(kind, kindDir) {
		return kindDirMtime, nil, nil
	}
	entries, err := os.ReadDir(kindDir)
	if err != nil {
		return kindDirMtime, nil, err
	}
	bucketMtimes = make(map[string]int64)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		subDir := filepath.Join(kindDir, e.Name())
		subInfo, statErr := os.Stat(subDir)
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
func (f *FileObjectStorage) ListPathsForDiscovery(ctx context.Context, kind string) ([]PathWithID, error) {
	kindDir := f.GetKindDir(kind)
	if kindDir == "" {
		return nil, errfmt.Errorf(ConstStreamUnknownObjectKindStr, kind)
	}

	if _, err := os.Stat(kindDir); err != nil {
		if os.IsNotExist(err) {
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
		out := make([]PathWithID, 0, len(ids))
		for _, id := range ids {
			path, err := cas.GetFilePathForID(id)
			if err != nil {
				continue
			}
			out = append(out, PathWithID{Path: path, ID: id})
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
	out := make([]PathWithID, 0, len(filePaths))
	for _, path := range filePaths {
		id := f.getObjectIDFromPath(path, kind)
		if id == emptyValue {
			continue
		}
		out = append(out, PathWithID{Path: path, ID: id})
	}
	return out, nil
}

// getObjectIDAndKindFromPath extracts object ID and (for hash-based files) kind from a file.
// For hash-based files returns (id, fileKind); for others returns (id, "") so callers can skip kind filtering.
func (f *FileObjectStorage) getObjectIDAndKindFromPath(path, kind string) (id, fileKind string) {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext != emptyValue {
		base = strings.TrimSuffix(base, ext)
	}
	if len(base) == 64 && isHex(base) {
		return objects.ReadIDAndKindFromYAMLFile(path)
	}
	id = f.getObjectIDFromPath(path, kind)
	return id, ""
}

// getObjectIDFromPath extracts object ID from a file path (filename or file content for hash-based files).
func (f *FileObjectStorage) getObjectIDFromPath(path, kind string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext != emptyValue {
		base = strings.TrimSuffix(base, ext)
	}
	// Hash-based (CAS) filename: 64 hex chars — read only id field (fast path, no full YAML parse)
	if len(base) == 64 && isHex(base) {
		return objects.ReadIDFromYAMLFile(path)
	}
	// Account: account-username -> account:username
	accountDir := objects.GetDirectoryFromKind(objects.KindAccount)
	if accountDir != emptyValue && objects.GetDirectoryFromKind(kind) == accountDir && strings.HasPrefix(base, "account-") {
		return "account:" + strings.TrimPrefix(base, "account-")
	}
	if base != emptyValue {
		return base
	}
	return ""
}

func isHex(s string) bool {
	for _, c := range s {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return false
	}
	return true
}

// isHashBasedFilename reports whether the filename stem is 64-char hex (content-addressed name).
func isHashBasedFilename(filename string) bool {
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	return len(stem) == 64 && isHex(stem)
}
