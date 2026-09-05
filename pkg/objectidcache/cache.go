// Package objectidcache is the ObjectIDCache implementation extracted from cmd/zqk/system.
// CLI audit, coordinator events, cache-item strategy, and project-root resolution stay
// in the system package and are injected via SetProjectRootResolver / SetCacheAuditFunc /
// SetCacheLifecycleObservers / SetSidecarsIdleEmitter.
package objectidcache

import (
	stdcontext "context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/loader"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/process"
	"github.com/lanceman/zqk/pkg/storage"
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/when"
)

const pipelineKindEnsureObjectIDCacheReady = "system.ensure_object_id_cache_ready"

// ObjectIDCacheEntry represents a cached object ID with metadata
type ObjectIDCacheEntry struct {
	ID       string    `json:"id"`
	Kind     string    `json:"kind"`
	FilePath string    `json:"file_path"`
	MTime    time.Time `json:"mtime"`
	Exists   bool      `json:"exists"`
}

// MaxObjectIDCacheEntries is the maximum number of entries we load into the object ID cache.
// Prevents multi-GB memory use when object volume grows very large; beyond this we treat cache as missing.
const MaxObjectIDCacheEntries = 1_000_000

// ObjectIDCacheMetadata stores cache metadata for validation
type ObjectIDCacheMetadata struct {
	BuildTime    time.Time `json:"build_time"`
	ProjectRoot  string    `json:"project_root"`
	ProcessDir   string    `json:"process_dir"`
	ProcessMTime time.Time `json:"process_mtime"` // mtime of docs/process directory
}

// kindBucketEntry is one object in a kind bucket (v2 format). Kind implied by container; no "exists" (only missing listed separately).
// Path is relative to the kind directory: filename (e.g. hash.yaml) or subpath for bucketed kinds (e.g. 2026-01/CHA-001.yaml).
// Draft-plane live paths are stored absolute (outside the kind dir). TRACK: TDE-CEF-CAS-IDENTITY-TXN-001
type KindBucketEntry struct {
	ID    string    `json:"id"`
	Path  string    `json:"path"` // relative to kind dir, or absolute draft-plane path
	MTime time.Time `json:"mtime"`
}

func projectRootFromProcessDir(processDir string) string {
	if processDir == emptyValue {
		return emptyValue
	}
	if filepath.Base(processDir) == "process" {
		docs := filepath.Dir(processDir)
		if filepath.Base(docs) == "docs" {
			return filepath.Dir(docs)
		}
	}
	return emptyValue
}

func isObjectDraftPlaneFilePath(processDir, filePath string) bool {
	if projectRoot := projectRootFromProcessDir(processDir); projectRoot != emptyValue {
		return storage.IsObjectDraftPlanePath(projectRoot, filePath)
	}
	slash := filepath.ToSlash(filePath)
	marker := "/" + paths.ProjectDataDir + "/" + paths.ObjectDraftsDir + "/"
	return strings.Contains(slash, marker)
}

// relSlashInsideDir returns filePath as a slash-separated path relative to dir
// when filePath is contained in dir. ok is false if Rel fails, the result is
// empty, or the relative path walks out of dir ("..").
func relSlashInsideDir(dir, filePath string) (string, bool) {
	if dir == emptyValue || filePath == emptyValue {
		return emptyValue, false
	}
	rel, err := filepath.Rel(dir, filePath)
	if err != nil || rel == emptyValue || relWalksOutOfDir(rel) {
		return emptyValue, false
	}
	return filepath.ToSlash(rel), true
}

// relWalksOutOfDir reports whether a filepath.Rel result leaves the base directory.
func relWalksOutOfDir(rel string) bool {
	slash := filepath.ToSlash(rel)
	return slash == ".." || strings.HasPrefix(slash, "../")
}

// kindBucketPathToStore keeps CAS/legacy paths relative to the kind dir.
// Draft-plane YAML lives under .zqk/object_drafts; storing Base() would resolve
// to a fake file in the kind dir and ValidateAndCleanStale would drop the id.
// TRACK: TDE-CEF-CAS-IDENTITY-TXN-001
func kindBucketPathToStore(processDir, kind, filePath string) string {
	if filePath == emptyValue {
		return emptyValue
	}
	if processDir != emptyValue {
		kindDirName := objects.GetDirectoryFromKind(kind)
		if kindDirName != emptyValue {
			kindDir := filepath.Join(processDir, kindDirName)
			if rel, ok := relSlashInsideDir(kindDir, filePath); ok {
				return rel
			}
		}
	}
	if filepath.IsAbs(filePath) && isObjectDraftPlaneFilePath(processDir, filePath) {
		return filePath
	}
	return filepath.Base(filePath)
}

// ObjectIDCacheFileV2 is the on-disk format: bucket by kind, count_by_kind, no redundant id/kind/exists per entry.
type ObjectIDCacheFileV2 struct {
	Metadata      *ObjectIDCacheMetadata       `json:"metadata"`
	CountByKind   map[string]int               `json:"count_by_kind"`
	ByKind        map[string][]KindBucketEntry `json:"by_kind"`
	MissingByKind map[string][]string          `json:"missing_by_kind,omitempty"`
}

// ObjectIDCache is a thread-safe cache for object IDs.
// Uses v2 shape in memory: byKind (path relative to docs/process) and idToKind for lookups.
type ObjectIDCache struct {
	mu          sync.RWMutex
	byKind      map[string][]KindBucketEntry // kind -> entries (path relative to processDir)
	idToKind    map[string]string            // id -> kind for Get(id) lookup
	idToIndex   map[string]int               // id -> index in byKind[kind] (O(1) Get; TRACK: doc_entry validation timeouts)
	metadata    *ObjectIDCacheMetadata
	countByKind map[string]int // kind -> count
	processDir  string         // docs/process base for expanding relative paths; set on load/build
	cacheDir    string         // directory where cache file is stored

	// ensureRunner coordinates load/build with timeout (pkg/loader); one in-flight load per projectRoot
	ensureRunnerMu     sync.Mutex
	currentProjectRoot atomic.Value // string, set before Load(); loadFn reads without locking
	ensureRunner       *loader.Runner
	nextForceRebuild   atomic.Bool
	// progressNotifier is set before Load() so the loader callback can emit progress via coordinator (optional).
	// Holds *progressNotifierHolder only (atomic.Value panics on concrete type change).
	progressNotifier atomic.Value // *progressNotifierHolder
	// warmStorageForNextBuild: when set by EnsureObjectIDCacheReady(..., storageForWarm), BuildCache uses it for
	// warmCASIndexesFromCache so discovery and ref validation use the same warmed instance (avoids cold CAS lookups).
	// Holds *warmStorageHolder so we can clear with holder.storage=nil (atomic.Value cannot store nil interface).
	warmStorageForNextBuild atomic.Value // *warmStorageHolder
}

// Global cache instance (similar to IDValidator pattern)
var (
	globalObjectIDCache *ObjectIDCache
	cacheOnce           sync.Once
)

// GetGlobalObjectIDCache returns the global object ID cache instance
// This cache should be invalidated/updated when objects are created or deleted
func GetGlobalObjectIDCache() *ObjectIDCache {
	cacheOnce.Do(func() {
		globalObjectIDCache = NewObjectIDCache()
	})
	return globalObjectIDCache
}

// NewObjectIDCache creates a new object ID cache
func NewObjectIDCache() *ObjectIDCache {
	return &ObjectIDCache{
		byKind:    make(map[string][]KindBucketEntry),
		idToKind:  make(map[string]string),
		idToIndex: make(map[string]int),
		metadata:  nil,
		cacheDir:  "",
	}
}

// buildIDToIndex builds id → bucket-index from byKind (caller holds lock or owns maps).
func buildIDToIndex(byKind map[string][]KindBucketEntry) map[string]int {
	n := 0
	for _, list := range byKind {
		n += len(list)
	}
	idx := make(map[string]int, n)
	for _, list := range byKind {
		for i := range list {
			if list[i].ID != emptyValue {
				idx[list[i].ID] = i
			}
		}
	}
	return idx
}

// removeIDFromBucketIndexed removes id from list and keeps idToIndex coherent (swap-remove).
func removeIDFromBucketIndexed(list []KindBucketEntry, id string, idToIndex map[string]int) []KindBucketEntry {
	if len(list) == 0 {
		return list
	}
	i, ok := idToIndex[id]
	if !ok || i < 0 || i >= len(list) || list[i].ID != id {
		// Fallback linear scan if index drifted.
		for j := range list {
			if list[j].ID == id {
				i = j
				ok = true
				break
			}
		}
		if !ok {
			return list
		}
	}
	last := len(list) - 1
	delete(idToIndex, id)
	if i != last {
		list[i] = list[last]
		if list[i].ID != emptyValue {
			idToIndex[list[i].ID] = i
		}
	}
	return list[:last]
}

// getProcessDir returns the docs/process base path for expanding relative paths (must hold at least RLock).
func (c *ObjectIDCache) getProcessDir() string {
	if c.metadata != nil && c.metadata.ProjectRoot != emptyValue {
		return datacell.ProcessPrimaryDir(c.metadata.ProjectRoot)
	}
	return c.processDir
}

// entryFromBucket builds an ObjectIDCacheEntry from a kindBucketEntry. Path is relative to kind dir (filename or subdir/filename for bucketed).
func (c *ObjectIDCache) entryFromBucket(kind string, e *KindBucketEntry) *ObjectIDCacheEntry {
	fullPath := e.Path
	if base := c.getProcessDir(); base != emptyValue && !filepath.IsAbs(e.Path) {
		kindDir := objects.GetDirectoryFromKind(kind)
		if kindDir != emptyValue {
			fullPath = filepath.Join(base, kindDir, e.Path)
		} else {
			fullPath = filepath.Join(base, e.Path)
		}
	}
	return &ObjectIDCacheEntry{
		ID:       e.ID,
		Kind:     kind,
		FilePath: fullPath,
		MTime:    e.MTime,
		Exists:   true,
	}
}

// AddEntriesFromBuild adds entries from a cache build (e.g. processCacheJob).
// Path is stored relative to the kind directory (e.g. "filename.yaml" or "2026-01/filename.yaml" for bucketed kinds).
func (c *ObjectIDCache) AddEntriesFromBuild(projectRoot string, entries map[string]*ObjectIDCacheEntry) {
	if len(entries) == 0 {
		return
	}
	processDir := paths.ResolvePath(projectRoot, "prefix:process")
	if c.processDir == emptyValue {
		c.processDir = processDir
	}
	for id, entry := range entries {
		if entry == nil || entry.Kind == emptyValue || entry.FilePath == emptyValue {
			continue
		}
		pathToStore := kindBucketPathToStore(processDir, entry.Kind, entry.FilePath)
		e := KindBucketEntry{ID: id, Path: pathToStore, MTime: entry.MTime}
		c.byKind[entry.Kind] = append(c.byKind[entry.Kind], e)
		c.idToKind[id] = entry.Kind
		if c.idToIndex == nil {
			c.idToIndex = make(map[string]int)
		}
		c.idToIndex[id] = len(c.byKind[entry.Kind]) - 1
	}
	// countByKind will be recomputed on Save
	c.countByKind = nil
}

// getCacheFilePath returns the path to the cache file
func (c *ObjectIDCache) getCacheFilePath(projectRoot string) string {
	if c.cacheDir != emptyValue {
		return filepath.Join(c.cacheDir, paths.ObjectIDCacheFile)
	}
	// Prefer brand-settings path alias ("cache") when the path cache is built so location is driven
	// by zqk-settings.yaml; fallback keeps the canonical .zqk/cache/object-id-cache.json.
	cacheDir := paths.ResolvePathFromCacheOrConstant(projectRoot, "cache", filepath.Join(paths.ProjectDataDir, paths.CacheDir))
	return filepath.Join(cacheDir, paths.ObjectIDCacheFile)
}

// parseObjectIDCacheFile parses cache file bytes (v2 only) into byKind, idToKind, metadata, countByKind.
func ParseObjectIDCacheFile(data []byte) (byKind map[string][]KindBucketEntry, idToKind map[string]string, metadata *ObjectIDCacheMetadata, countByKind map[string]int, err error) {
	var v2 ObjectIDCacheFileV2
	if err := json.Unmarshal(data, &v2); err != nil {
		return nil, nil, nil, nil, err
	}
	if v2.Metadata == nil || v2.ByKind == nil {
		return nil, nil, nil, nil, errfmt.Errorf("v2 cache missing metadata or by_kind")
	}
	byKind = make(map[string][]KindBucketEntry, len(v2.ByKind))
	idToKind = make(map[string]string)
	countByKind = v2.CountByKind
	if countByKind == nil {
		countByKind = make(map[string]int)
	}
	for kind, list := range v2.ByKind {
		byKind[kind] = list
		for i := range list {
			if list[i].ID != emptyValue {
				idToKind[list[i].ID] = kind
			}
		}
	}
	normalizeByKindKeys(byKind, idToKind, countByKind)
	// Recompute countByKind from byKind so we never trust corrupted or stale on-disk counts (e.g. negative).
	// byKind is the source of truth; count_by_kind on disk may be from an old bug or corruption.
	countByKind = make(map[string]int, len(byKind))
	for k, list := range byKind {
		countByKind[k] = len(list)
	}
	return byKind, idToKind, v2.Metadata, countByKind, nil
}

// normalizeByKindKeys merges buckets with invalid/truncated kind keys (e.g. "bac", "back", "backlo")
// into the correct kind inferred from the path. Invalid keys are those where GetDirectoryFromKind(key) == emptyValue.
func normalizeByKindKeys(byKind map[string][]KindBucketEntry, idToKind map[string]string, countByKind map[string]int) {
	for kind, list := range byKind {
		if objects.GetDirectoryFromKind(kind) != emptyValue {
			continue
		}
		var inferredKind string
		for _, e := range list {
			if strings.Contains(e.Path, string(filepath.Separator)) {
				baseDir := filepath.Base(filepath.Dir(e.Path))
				inferredKind = objects.GetKindFromDirectory(baseDir)
				break
			}
		}
		if inferredKind == emptyValue {
			continue
		}
		byKind[inferredKind] = append(byKind[inferredKind], list...)
		for _, e := range list {
			if e.ID != emptyValue {
				idToKind[e.ID] = inferredKind
			}
		}
		delete(byKind, kind)
		if countByKind != nil {
			n := countByKind[kind]
			delete(countByKind, kind)
			countByKind[inferredKind] += n
		}
	}
}

// LoadCache loads the cache from disk if it exists and is still valid
// Returns true if cache was successfully loaded, false if cache needs to be rebuilt
func (c *ObjectIDCache) LoadCache(projectRoot string) (bool, error) {
	cachePath := c.getCacheFilePath(projectRoot)
	c.cacheDir = filepath.Dir(cachePath)

	data, err := fileutil.ReadFile(cachePath)
	if fileutil.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, errfmt.Newf("failed to read cache file").Wrap(err)
	}

	byKind, idToKind, metadata, countByKind, err := ParseObjectIDCacheFile(data)
	if err != nil {
		return false, nil
	}
	if metadata == nil || metadata.ProjectRoot != projectRoot {
		return false, nil
	}
	if len(idToKind) > MaxObjectIDCacheEntries {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		logging.Fluent(logger).Warn("Object ID cache exceeds max entries, skipping load to bound memory").
			Int("entries", len(idToKind)).
			Int("max", MaxObjectIDCacheEntries).
			Log()
		return false, nil
	}

	processDir := datacell.ProcessPrimaryDir(projectRoot)
	info, err := fileutil.Stat(processDir)
	if err != nil {
		return false, nil
	}

	currentMTime := info.ModTime()
	cachedMTime := metadata.ProcessMTime
	timeDiff := currentMTime.Sub(cachedMTime)
	if timeDiff < 0 {
		timeDiff = -timeDiff
	}
	cacheAge := time.Since(metadata.BuildTime)
	cacheIsRecent := cacheAge < 24*time.Hour
	entryCount := len(idToKind)
	if timeDiff > 5*time.Second {
		if !cacheIsRecent || entryCount == 0 {
			return false, nil
		}
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	_ = concurrency.WithLockTimeout(
		&c.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameObjectIDCacheLoad,
		func() error {
			c.byKind = byKind
			c.idToKind = idToKind
			c.idToIndex = buildIDToIndex(byKind)
			c.metadata = metadata
			c.countByKind = countByKind
			c.processDir = processDir
			return nil
		},
	)

	logging.Fluent(logger).Debug("Loaded object ID cache from disk").
		EntryCount(entryCount).
		String("build_time", metadata.BuildTime.Format(time.RFC3339)).
		Log()
	if entryCount == 0 {
		logging.Fluent(logger).Warn("Cache loaded from disk but is empty - this may cause validation issues").Log()
	}
	return true, nil
}

// kindNamesForReverseReferenceScan returns kind keys from the in-memory object ID cache (after LoadCache or BuildCache).
// Used to build the reverse reference index in the same goroutine as cache load without a separate discovery pass.
// Matches other ObjectIDCache readers (e.g. Get): [concurrency.WithRLockTimeout] + lock logger + bounded ctx.
func (c *ObjectIDCache) kindNamesForReverseReferenceScan() []string {
	var kinds []string
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	_ = concurrency.WithRLockTimeout(
		&c.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameObjectIDCacheKindNamesForReverseRef,
		func() error {
			if len(c.byKind) == 0 {
				return nil
			}
			kinds = make([]string, 0, len(c.byKind))
			for k := range c.byKind {
				kinds = append(kinds, k)
			}
			return nil
		},
	)
	return kinds
}

// SaveCache saves the cache to disk in v2 format (by_kind, count_by_kind; no redundant id/kind/exists per entry).
// Compact JSON: this file is a machine cache rewritten from mutation paths; indenting it made
// json.MarshalIndent / appendIndent the hottest stack under bulk delete (see persistCacheChanges).
func (c *ObjectIDCache) SaveCache(projectRoot string) error {
	saveStart := time.Now()
	var entryCount int
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	cachePath := c.getCacheFilePath(projectRoot)
	cacheDir := filepath.Dir(cachePath)
	if err := fileutil.MkdirAll(cacheDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create cache directory").Wrap(err)
	}

	processDir := datacell.ProcessPrimaryDir(projectRoot)
	info, err := fileutil.Stat(processDir)
	if err != nil {
		return errfmt.Newf("failed to stat process directory").Wrap(err)
	}

	var v2 ObjectIDCacheFileV2
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	_ = concurrency.WithLockTimeout(
		&c.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameObjectIDCacheSavePrepare,
		func() error {
			c.metadata = &ObjectIDCacheMetadata{
				BuildTime:    time.Now(),
				ProjectRoot:  projectRoot,
				ProcessDir:   processDir,
				ProcessMTime: info.ModTime(),
			}
			c.processDir = processDir
			countByKind := make(map[string]int, len(c.byKind))
			countByKindCopy := make(map[string]int, len(c.byKind))
			byKindCopy := make(map[string][]KindBucketEntry, len(c.byKind))
			for k, list := range c.byKind {
				countByKind[k] = len(list)
				countByKindCopy[k] = len(list)
				listCopy := make([]KindBucketEntry, len(list))
				copy(listCopy, list)
				byKindCopy[k] = listCopy
			}
			c.countByKind = countByKind
			v2 = ObjectIDCacheFileV2{
				Metadata:    c.metadata,
				CountByKind: countByKindCopy,
				ByKind:      byKindCopy,
			}
			entryCount = len(c.idToKind)
			return nil
		},
	)

	data, err := json.Marshal(v2)
	if err != nil {
		return errfmt.Newf("failed to marshal cache").Wrap(err)
	}
	data = append(data, '\n')

	if err := fileutil.WriteStandardFile(cachePath, data); err != nil {
		return errfmt.Newf("failed to write cache file").Wrap(err)
	}

	// Best-effort cache save event emission; the storageProvider is currently unused by
	// emitCacheSaveEventViaCoordinator, so avoid creating extra storage instances (and WAL handles).
	if onCacheSave != nil {
		onCacheSave(projectRoot, entryCount, time.Since(saveStart))
	}
	return nil
}

// Get retrieves a cache entry by ID (path expanded from relative to full).
func (c *ObjectIDCache) Get(id string) (*ObjectIDCacheEntry, bool) {
	var entry *ObjectIDCacheEntry
	var exists bool
	var cacheSize int
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	// Plain RLock: WithRLockTimeout used a 5s parent context matching the validation
	// fail-fast budget, so brief write-lock waits burned the entire object timeout.
	// Get is O(1) under the lock; prefer not to compete with validationCtx.
	// TRACK: REDACTED
	c.mu.RLock()
	if c.byKind != nil && c.idToKind != nil {
		kind, ok := c.idToKind[id]
		if !ok {
			cacheSize = len(c.idToKind)
		} else {
			list := c.byKind[kind]
			i := -1
			if c.idToIndex != nil {
				if idx, found := c.idToIndex[id]; found {
					i = idx
				}
			}
			if i < 0 || i >= len(list) || list[i].ID != id {
				i = -1
				for j := range list {
					if list[j].ID == id {
						i = j
						break
					}
				}
			}
			if i >= 0 {
				entry = c.entryFromBucket(kind, &list[i])
				exists = true
			}
			cacheSize = len(c.idToKind)
		}
	}
	c.mu.RUnlock()

	if c.byKind == nil {
		logging.Fluent(logger).Warn("ObjectIDCache.Get called but cache is nil").
			ObjectID(id).
			String("diagnostic", "Cache may not have been loaded or was cleared").
			Log()
		return nil, false
	}

	if !exists {
		if onCacheMiss != nil {
			onCacheMiss(id, cacheSize, logger)
		}
		return nil, false
	}

	if onCacheHit != nil {
		onCacheHit(id, logger)
	}
	return entry, exists
}

// Set stores a cache entry. CAS/legacy paths are relative to the kind dir;
// draft-plane paths stay absolute. TRACK: TDE-CEF-CAS-IDENTITY-TXN-001
func (c *ObjectIDCache) Set(id string, entry *ObjectIDCacheEntry) {
	if entry == nil || entry.Kind == emptyValue {
		return
	}
	pathToStore := kindBucketPathToStore(c.getProcessDir(), entry.Kind, entry.FilePath)
	e := KindBucketEntry{ID: id, Path: pathToStore, MTime: entry.MTime}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	_ = concurrency.WithLockTimeout(
		&c.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameObjectIDCacheSet,
		func() error {
			if c.byKind == nil {
				c.byKind = make(map[string][]KindBucketEntry)
			}
			if c.idToKind == nil {
				c.idToKind = make(map[string]string)
			}
			if c.idToIndex == nil {
				c.idToIndex = make(map[string]int)
			}
			if oldKind, exists := c.idToKind[id]; exists {
				c.byKind[oldKind] = removeIDFromBucketIndexed(c.byKind[oldKind], id, c.idToIndex)
				if c.countByKind != nil {
					c.countByKind[oldKind]--
					if c.countByKind[oldKind] <= 0 {
						delete(c.countByKind, oldKind)
					}
				}
			}
			c.byKind[entry.Kind] = append(c.byKind[entry.Kind], e)
			c.idToKind[id] = entry.Kind
			c.idToIndex[id] = len(c.byKind[entry.Kind]) - 1
			if c.countByKind != nil {
				c.countByKind[entry.Kind]++
			}
			return nil
		},
	)
}

// Invalidate removes an entry from the cache (for deleted objects)
func (c *ObjectIDCache) Invalidate(id string) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	_ = concurrency.WithLockTimeout(
		&c.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameObjectIDCacheInvalidate,
		func() error {
			if c.idToKind == nil {
				return nil
			}
			kind, ok := c.idToKind[id]
			if !ok {
				return nil
			}
			delete(c.idToKind, id)
			if c.idToIndex == nil {
				c.idToIndex = make(map[string]int)
			}
			c.byKind[kind] = removeIDFromBucketIndexed(c.byKind[kind], id, c.idToIndex)
			if c.countByKind != nil {
				c.countByKind[kind]--
				if c.countByKind[kind] <= 0 {
					delete(c.countByKind, kind)
				}
			}
			return nil
		},
	)
}

// Update adds or updates a cache entry (for created/updated objects)
// This should be called after creating or updating an object file
func (c *ObjectIDCache) Update(id, kind, filePath string) error {
	info, err := fileutil.Stat(filePath)
	if err != nil {
		// File doesn't exist - invalidate cache entry
		c.Invalidate(id)
		return err
	}

	entry := &ObjectIDCacheEntry{
		ID:       id,
		Kind:     kind,
		FilePath: filePath,
		MTime:    info.ModTime(),
		Exists:   true,
	}

	c.Set(id, entry)
	return nil
}

// InvalidateKind removes all entries for a specific kind (for bulk operations)
func (c *ObjectIDCache) InvalidateKind(kind string) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameObjectIDCacheInvalidateKind,
		func() error {
			if c.byKind == nil {
				return nil
			}
			for _, e := range c.byKind[kind] {
				delete(c.idToKind, e.ID)
				delete(c.idToIndex, e.ID)
			}
			delete(c.byKind, kind)
			if c.countByKind != nil {
				delete(c.countByKind, kind)
			}
			return nil
		},
	)
}

// BulkInvalidate removes multiple entries from the cache by ID (for bulk delete operations).
func (c *ObjectIDCache) BulkInvalidate(ids []string) int {
	var count int
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameObjectIDCacheBulkInvalidate,
		func() error {
			if c.idToKind == nil {
				return nil
			}
			idsByKind := make(map[string][]string)
			for _, id := range ids {
				kind, exists := c.idToKind[id]
				if !exists {
					continue
				}
				idsByKind[kind] = append(idsByKind[kind], id)
			}
			for kind, kindIDs := range idsByKind {
				for _, id := range kindIDs {
					delete(c.idToKind, id)
					if c.idToIndex == nil {
						c.idToIndex = make(map[string]int)
					}
					c.byKind[kind] = removeIDFromBucketIndexed(c.byKind[kind], id, c.idToIndex)
					count++
				}
				if c.countByKind != nil {
					c.countByKind[kind] -= len(kindIDs)
					if c.countByKind[kind] <= 0 {
						delete(c.countByKind, kind)
					}
				}
			}
			return nil
		},
	)
	return count
}

// ValidateAndCleanStale validates all cache entries and removes stale ones
// A stale entry is one where:
//   - The file doesn't exist
//   - The file's mtime doesn't match the cached mtime
//
// Additionally, if a file exists but mtime changed, this also updates the hash registry
// to ensure hash registry stays in sync with file modifications
// Returns the number of stale entries removed
// NOTE: This function performs file I/O (os.Stat) while holding the lock, which is not ideal.
func (c *ObjectIDCache) ValidateAndCleanStale() int {
	var staleCount int
	var healedCount int
	var hashRegistryUpdates map[string]map[string]string
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameObjectIDCacheValidateClean,
		func() error {
			staleIDs := make([]string, 0)
			hashRegistryUpdates = make(map[string]map[string]string)
			base := c.getProcessDir()

			for kind, list := range c.byKind {
				kindDir := objects.GetDirectoryFromKind(kind)
				for i := range list {
					e := &list[i]
					fullPath := e.Path
					if base != emptyValue && !filepath.IsAbs(e.Path) {
						if kindDir != emptyValue {
							fullPath = filepath.Join(base, kindDir, filepath.Base(e.Path))
						} else {
							fullPath = filepath.Join(base, e.Path)
						}
					}
					info, err := fileutil.Stat(fullPath)
					if err != nil {
						// Prefer re-pointing to the live CAS hash over dropping the
						// entry (update/promote deletes the old hash file first).
						// TRACK: REDACTED
						var kindDirPath string
						if kindDir != emptyValue && base != emptyValue {
							kindDirPath = filepath.Join(base, kindDir)
						} else {
							kindDirPath = filepath.Dir(fullPath)
						}
						if live, ok := caspkg.ResolveLiveCASFilePathFromKindDir(kindDirPath, kind, e.ID); ok {
							liveInfo, liveErr := fileutil.Stat(live)
							if liveErr == nil {
								// Keep path relative to kind dir (Set/Update contract).
								pathToStore := filepath.Base(live)
								if kindDir != emptyValue && base != emptyValue {
									if rel, ok := relSlashInsideDir(filepath.Join(base, kindDir), live); ok {
										pathToStore = rel
									}
								}
								c.byKind[kind][i].Path = pathToStore
								c.byKind[kind][i].MTime = liveInfo.ModTime()
								healedCount++
								continue
							}
						}
						staleIDs = append(staleIDs, e.ID)
						continue
					}
					timeDiff := info.ModTime().Sub(e.MTime)
					if timeDiff < -time.Second || timeDiff > time.Second {
						c.byKind[kind][i].MTime = info.ModTime()
						if hashRegistryUpdates[kind] == nil {
							hashRegistryUpdates[kind] = make(map[string]string)
						}
						hashRegistryUpdates[kind][filepath.Base(fullPath)] = fullPath
					}
				}
			}

			for _, id := range staleIDs {
				kind, ok := c.idToKind[id]
				if ok {
					delete(c.idToKind, id)
					if c.idToIndex == nil {
						c.idToIndex = make(map[string]int)
					}
					c.byKind[kind] = removeIDFromBucketIndexed(c.byKind[kind], id, c.idToIndex)
					if len(c.byKind[kind]) == 0 {
						delete(c.byKind, kind)
					}
					if c.countByKind != nil {
						c.countByKind[kind]--
						if c.countByKind[kind] <= 0 {
							delete(c.countByKind, kind)
						}
					}
				}
			}
			staleCount = len(staleIDs)
			return nil
		},
	)

	// Update hash registry for files that were modified (mtime changed but file exists)
	// This ensures hash registry stays in sync when files are modified outside normal storage operations
	if len(hashRegistryUpdates) > 0 {
		// We need project root to find kind directories, but we don't have it here
		// So we'll update hash registry in PerformCacheFreshnessCheck where we have project root
		// For now, just remove stale cache entries
	}

	// Healed renames must count so callers SaveCache (otherwise disk keeps
	// deleted-hash paths and the next process rediscovers the same ghosts).
	return staleCount + healedCount
}

// IsStale checks if a specific cache entry is stale
// Returns true if the entry is stale (file doesn't exist or mtime doesn't match)
func (c *ObjectIDCache) IsStale(id string) bool {
	entry, exists := c.Get(id)

	if !exists {
		return false // Entry doesn't exist, not stale
	}

	// Check if file exists
	info, err := fileutil.Stat(entry.FilePath)
	if err != nil {
		return true // File doesn't exist - stale
	}

	// Check if mtime matches (within 1 second tolerance)
	timeDiff := info.ModTime().Sub(entry.MTime)
	if timeDiff < -time.Second || timeDiff > time.Second {
		return true // Mtime doesn't match - stale
	}

	return false // Entry is valid
}

// BuildCache builds the object ID cache by scanning all object directories
// It first tries to load from disk, and only rebuilds if cache is missing or stale
// forceRebuild forces a full rebuild even if cache exists and is valid.
// buildCtx is the context passed by the loader (e.g. cancelCtx from signal.NotifyContext so warm respects SIGINT).
func (c *ObjectIDCache) BuildCache(buildCtx stdcontext.Context, projectRoot string, forceRebuild bool) error {
	if buildCtx != nil && buildCtx.Err() != nil {
		return buildCtx.Err()
	}
	buildStart := time.Now()
	ctx := initializeCacheBuildContext(c, projectRoot, forceRebuild)

	loaded, err := tryLoadExistingCache(ctx)
	if err == nil && loaded {
		// Reverse reference index: build synchronously using kinds already in the object ID cache so
		// EnsureObjectIDCacheReady does not return while a background SaveCache is still writing under projectRoot.
		revIndex := storage.GetGlobalReverseReferenceIndex()
		revLoaded, _ := revIndex.LoadCache(projectRoot)
		if !revLoaded {
			processDir := datacell.ProcessPrimaryDir(projectRoot)
			kinds := c.kindNamesForReverseReferenceScan()
			if err := ensureReverseReferenceIndexSync(projectRoot, processDir, kinds); err != nil {
				logging.Fluent(ctx.Logger).Debug("Reverse reference index sync after object ID cache load failed (background fallback)").
					WithError(err).
					Log()
				triggerBackgroundReverseReferenceIndexBuild(projectRoot)
			}
		}
		// Cache was loaded successfully. Warm CAS indexes so List/discovery see the same
		// objects as the cache. When caller passed storageForWarm, warm runs once in EnsureObjectIDCacheReady after Load().
		storageForWarm := c.getWarmStorageForBuild()
		if storageForWarm == nil {
			c.notifyCacheProgress("warming", "Warming CAS indexes...")
			process.TouchMeaningfulActivity()
			warmCASIndexesFromCache(buildCtx, projectRoot, c, nil, 0)
		}

		// Emit cache load event
		var entryCount int
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		_ = concurrency.WithRLockTimeout(
			&c.mu,
			pkgctx.NewSystemContext(),
			nil,
			logging.NewLockLoggerAdapter(logger),
			LockNameObjectIDCacheBuildGetCountLoaded,
			func() error {
				entryCount = len(c.idToKind)
				return nil
			},
		)

		// Emit cache load event via coordinator (if storage provider available)
		// storageProvider is currently unused by emitCacheBuildEventViaCoordinator, so avoid creating
		// extra storage instances (and WAL handles) during tests/warm paths.
		if onCacheBuild != nil {
			onCacheBuild(projectRoot, "load", entryCount, false, time.Since(buildStart))
		}
		return nil
	}

	// Bound kind discovery so BuildCache never hangs on FieldRegistry.LoadFields (uses existing
	// loader pattern via DiscoverObjectKindsWithContext; fallback to kind mapper if timeout).
	discoverCtx, discoverCancel := stdcontext.WithTimeout(buildCtx, 30*time.Second)
	kinds := discoverKindsForCache(discoverCtx, projectRoot)
	discoverCancel()
	if kinds == nil {
		if discoverCtx.Err() != nil {
			logging.Fluent(ctx.Logger).Debug("Kind discovery timed out or cancelled, using kind mapper fallback").
				WithError(discoverCtx.Err()).
				Log()
		}
		if km := objects.GetGlobalKindMapper(); km != nil {
			if err := km.EnsureReady(buildCtx); err == nil {
				kinds = km.GetAllKinds()
			}
		}
		if kinds == nil {
			kinds = []string{}
		}
	}

	clearCacheForRebuild(ctx, kinds)
	c.notifyCacheProgress("building", "Building object ID cache...")
	// Idle watchdog (non-zqk parent) cancels when silent too long; rebuild+warm can
	// exceed the default 10s window without progress touches.
	// TRACK: TDE-SYSCHECK-COLD-REBUILD-IDLE-001 — remove when: warm/rebuild always
	// report activity via a shared progress heartbeat helper.
	process.TouchMeaningfulActivity()

	if err := buildCacheInParallel(ctx, kinds); err != nil {
		return err
	}
	process.TouchMeaningfulActivity()

	// Warm CAS indexes from the cache we just built. When caller passed storageForWarm it's warmed once in EnsureObjectIDCacheReady after Load().
	storageForWarm := c.getWarmStorageForBuild()
	if storageForWarm == nil {
		c.notifyCacheProgress("warming", "Warming CAS indexes...")
		warmCASIndexesFromCache(buildCtx, projectRoot, c, nil, 0)
	}

	// Verify cache was populated after rebuild
	var entryCount int
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithRLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameObjectIDCacheBuildVerify,
		func() error {
			entryCount = len(c.idToKind)
			return nil
		},
	)

	when.When(func() bool { return entryCount == 0 }).Then(func() {
		logging.Fluent(ctx.Logger).Warn("Cache rebuild completed but cache is empty - this may cause validation issues").Log()
	}).OrElse(func() {
		logging.Fluent(ctx.Logger).Debug("Cache rebuild completed successfully").EntryCount(entryCount).Log()
	}).Run()

	err = c.SaveCache(projectRoot)
	when.When(func() bool { return err != nil }).Then(func() {
		logging.Fluent(ctx.Logger).Warn("Failed to save object ID cache").WithError(err).Log()
	}).OrElse(func() {
		// Ensure reverse reference index is ready (load or build from same kinds)
		processDir := datacell.ProcessPrimaryDir(projectRoot)
		revIndex := storage.GetGlobalReverseReferenceIndex()
		if revLoaded, _ := revIndex.LoadCache(projectRoot); !revLoaded {
			if buildErr := revIndex.BuildFromScan(projectRoot, processDir, kinds); buildErr != nil {
				logging.Fluent(ctx.Logger).Debug("Reverse reference index build failed (non-fatal)").WithError(buildErr).Log()
			} else if saveErr := revIndex.SaveCache(projectRoot); saveErr != nil {
				logging.Fluent(ctx.Logger).Debug("Reverse reference index save failed (non-fatal)").WithError(saveErr).Log()
			}
		}
		// Cache was built successfully - emit event
		_ = concurrency.WithRLockTimeout(
			&c.mu,
			pkgctx.NewSystemContext(),
			nil,
			logging.NewLockLoggerAdapter(logger),
			LockNameObjectIDCacheBuildGetCountBuilt,
			func() error {
				entryCount = len(c.idToKind)
				return nil
			},
		)

		// Emit cache build event via coordinator
		if onCacheBuild != nil {
			onCacheBuild(projectRoot, "build", entryCount, forceRebuild, time.Since(buildStart))
		}
	}).Run()

	return nil
}

// ObjectIDCacheProgressNotifier is called during cache load/build to emit progress via coordinator.
// Implementations should use emitObjectIDCacheProgressViaCoordinator so CLI subscribers show progress.
type ObjectIDCacheProgressNotifier interface {
	NotifyCacheProgress(status string, message string)
}

// GetFilePathsForKind returns all cached file paths for a given kind
func (c *ObjectIDCache) GetFilePathsForKind(kind string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	bucket, exists := c.byKind[kind]
	if !exists {
		return nil
	}

	processDir := c.getProcessDir()
	paths := make([]string, 0, len(bucket))
	for _, entry := range bucket {
		absPath := kindBucketPathToStore(processDir, kind, entry.Path)
		paths = append(paths, absPath)
	}
	return paths
}
