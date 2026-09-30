package storage

import (
	"context"
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/storage/filecas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"strings"
	"sync"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/process"
	"github.com/zqk-os/zqk/pkg/storage/crud"
	"github.com/zqk-os/zqk/pkg/when"
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
				// Low-CPU pacing between reconciliation jobs (INDEX_FIRST_LOW_CPU_SCAN_DESIGN).
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
	if !crud.IsHashBasedFilename(base) {
		return false
	}
	hash := strings.TrimSuffix(base, filepath.Ext(base))
	cas, err := f.getContentAddressableStorage(kind)
	if filecas.ContentAddressableStorageOrIndexMissing(err, cas) {
		return false
	}
	kindDir := f.GetKindDir(kind)
	if kindDir == "" {
		return false
	}
	// Skip stale cache paths whose CAS blob was already replaced (see EnsureCASIndexFromPaths).
	// TRACK: follow-up in kernel backlog
	if !casHashFileExistsAt(kindDir, filePath, hash) {
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
		var _err_83115633 = cas.GetIndex().SetMapping(id, hash, bucketKey)
		if _err_83115633 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83115633).Log()
		}
	}).OrElse(func() {
		var _err_83115701 = cas.GetIndex().SetMapping(id, hash)
		if _err_83115701 != nil {
			logging.Fluent(logging.

				// EnsureCASIndexFromPaths updates the CAS index for a kind from many id->filePath entries in one load/merge/save.
				// Use this when warming from cache to avoid O(n) per-entry SetMapping (each of which does a full index read+write).
				GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83115701).Log()
		}
	}).Run()
	return true
}

// getCASAndKindDir returns the CAS instance and kind directory, or an error if missing.
func (f *FileObjectStorage) getCASAndKindDir(kind string) (*filecas.ContentAddressableStorage, string, error) {
	cas, err := f.getContentAddressableStorage(kind)
	if filecas.ContentAddressableStorageOrIndexMissing(err, cas) {
		return nil, "", err
	}
	kindDir := f.GetKindDir(kind)
	return cas, kindDir, nil
}

func (f *FileObjectStorage) EnsureCASIndexFromPaths(kind string, idToFilePath map[string]string) error {
	if !f.usesContentAddressableStorage(kind) || len(idToFilePath) == 0 {
		return nil
	}
	cas, kindDir, err := f.getCASAndKindDir(kind)
	if err != nil || kindDir == "" {
		return err
	}

	// Prefer in-memory/disk index when it already matches cache paths so warm does not
	// Stat+rewrite every blob on every system check (multi-second stall).
	// TRACK: follow-up in kernel backlog
	current := cas.GetIndex().SnapshotMappings()
	currentBuckets := cas.GetIndex().SnapshotBucketKeys()

	mappings := make(map[string]string, len(idToFilePath))
	var bucketKeys map[string]string
	if f.usesBucketedStorage(kind, kindDir) {
		bucketKeys = make(map[string]string)
	}
	for id, filePath := range idToFilePath {
		if !f.idBelongsToKind(id, kind) {
			continue
		}
		if IsObjectDraftPlanePath(f.projectRoot, filePath) {
			continue
		}
		base := filepath.Base(filePath)
		if !crud.IsHashBasedFilename(base) {
			continue
		}
		hash := strings.TrimSuffix(base, filepath.Ext(base))
		if cur, ok := current[id]; ok && cur == hash {
			// Index already points at this hash; skip Stat/rewrite. Bucket key: only
			// enqueue when bucketed and the key is missing/mismatched.
			if bucketKeys != nil {
				fileDir := filepath.Dir(filePath)
				want := ""
				if fileDir != kindDir {
					want = filepath.Base(fileDir)
				}
				if currentBuckets[id] != want && want != "" {
					mappings[id] = hash
					bucketKeys[id] = want
				}
			}
			continue
		}
		// Object-id-cache often lags CAS updates (path still names the deleted hash).
		// Warming those mappings via SetMappings (explicit-wins) was reverting healed
		// indexes and making system check flap 0↔hundreds of "orphans".
		// always updates CAS paths on write and warm refuses stale paths.
		if !casHashFileExistsAt(kindDir, filePath, hash) {
			continue
		}
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
	return cas.GetIndex().SetMappings(mappings, bucketKeys)
}

// casHashFileExistsAt reports whether the warm path's hash blob is still on disk.
func casHashFileExistsAt(kindDir, filePath, hash string) bool {
	if filePath != "" {
		if _, err := fileutil.Stat(filePath); err == nil {
			return true
		}
	}
	return filecas.CasHashYAMLExists(kindDir, hash)
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
// casScanBest tracks winning hash for an ID when multiple CAS files claim the same ID.
type casScanBest struct {
	hash      string
	mtime     int64
	bucketKey string
}

func scanCASDirEntries(ctx context.Context, dir, kind, bucketKey string, best map[string]casScanBest) bool {
	entries, readErr := fileutil.ReadDir(dir)
	if readErr != nil {
		return true
	}
	const abortCheckInterval = 500
	for i, e := range entries {
		if ctx.Err() != nil {
			return false
		}
		if e.IsDir() {
			continue
		}
		name := e.Name()
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		if !crud.IsHashBasedFilename(stem) {
			continue
		}
		if (i+1)%abortCheckInterval == 0 {
			process.TouchMeaningfulActivity()
			if ctx.Err() != nil {
				return false
			}
		}
		path := filepath.Join(dir, name)
		id, fileKind := crud.GetObjectIDAndKindFromPath(path, kind)
		if id == emptyValue || (fileKind != emptyValue && fileKind != kind) {
			continue
		}
		var mtime int64
		if fi, infoErr := e.Info(); infoErr == nil {
			mtime = fi.ModTime().UnixNano()
		} else if fi, statErr := fileutil.Stat(path); statErr == nil {
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

func (f *FileObjectStorage) scanBucketedCASDirs(ctx context.Context, kind, kindDir string, best map[string]casScanBest) error {
	entries, err := fileutil.ReadDir(kindDir)
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
		if !scanCASDirEntries(ctx, subDir, kind, e.Name(), best) {
			return ctx.Err()
		}
	}
	return nil
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
	if !f.usesContentAddressableStorage(kind) || StreamStorageEnabledForKind(kind) {
		return nil
	}
	cas, kindDir, err := f.getCASAndKindDir(kind)
	if err != nil || kindDir == "" {
		return err
	}
	if _, err := fileutil.Stat(kindDir); err != nil {
		if fileutil.IsNotExist(err) {
			return nil
		}
		return err
	}

	best := make(map[string]casScanBest)
	if !scanCASDirEntries(ctx, kindDir, kind, "", best) {
		return ctx.Err()
	}
	if f.usesBucketedStorage(kind, kindDir) {
		if err := f.scanBucketedCASDirs(ctx, kind, kindDir, best); err != nil {
			return err
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
	return cas.GetIndex().SetMappings(mappings, bucketKeys)
}

// countHashNamedFilesInKindDir returns the number of hash-named (CAS) files in the kind directory.
// Fast path: ReadDir only, no YAML reads. Used to detect index-vs-disk disparity for background refresh.
// Stream-backed kinds: do not scan YAML dir; return 0 (callers add stream count separately).
