package system

import (
	"github.com/lanceman/zqk/pkg/datacell"

	stdcontext "context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/loader"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/pipeline"
	"github.com/lanceman/zqk/pkg/storage"
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
type kindBucketEntry struct {
	ID    string    `json:"id"`
	Path  string    `json:"path"` // relative to kind dir; full path = processDir/kindDir/Path
	MTime time.Time `json:"mtime"`
}

// objectIDCacheFileV2 is the on-disk format: bucket by kind, count_by_kind, no redundant id/kind/exists per entry.
type objectIDCacheFileV2 struct {
	Metadata      *ObjectIDCacheMetadata       `json:"metadata"`
	CountByKind   map[string]int               `json:"count_by_kind"`
	ByKind        map[string][]kindBucketEntry `json:"by_kind"`
	MissingByKind map[string][]string          `json:"missing_by_kind,omitempty"`
}

// ObjectIDCache is a thread-safe cache for object IDs.
// Uses v2 shape in memory: byKind (path relative to docs/process) and idToKind for lookups.
type ObjectIDCache struct {
	mu          sync.RWMutex
	byKind      map[string][]kindBucketEntry // kind -> entries (path relative to processDir)
	idToKind    map[string]string            // id -> kind for Get(id) lookup
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
		byKind:   make(map[string][]kindBucketEntry),
		idToKind: make(map[string]string),
		metadata: nil,
		cacheDir: "",
	}
}

// getProcessDir returns the docs/process base path for expanding relative paths (must hold at least RLock).
func (c *ObjectIDCache) getProcessDir() string {
	if c.metadata != nil && c.metadata.ProjectRoot != emptyValue {
		return datacell.ProcessPrimaryDir(c.metadata.ProjectRoot)
	}
	return c.processDir
}

// entryFromBucket builds an ObjectIDCacheEntry from a kindBucketEntry. Path is relative to kind dir (filename or subdir/filename for bucketed).
func (c *ObjectIDCache) entryFromBucket(kind string, e *kindBucketEntry) *ObjectIDCacheEntry {
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

// removeIDFromBucket returns a new slice without the entry with the given id.
func removeIDFromBucket(list []kindBucketEntry, id string) []kindBucketEntry {
	for i, e := range list {
		if e.ID == id {
			return append(list[:i], list[i+1:]...)
		}
	}
	return list
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
		kindDir := filepath.Join(processDir, objects.GetDirectoryFromKind(entry.Kind))
		var pathToStore string
		if rel, err := filepath.Rel(kindDir, entry.FilePath); err == nil && !strings.HasPrefix(rel, "..") {
			pathToStore = filepath.ToSlash(rel)
		} else {
			pathToStore = filepath.Base(entry.FilePath)
		}
		e := kindBucketEntry{ID: id, Path: pathToStore, MTime: entry.MTime}
		c.byKind[entry.Kind] = append(c.byKind[entry.Kind], e)
		c.idToKind[id] = entry.Kind
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
func parseObjectIDCacheFile(data []byte) (byKind map[string][]kindBucketEntry, idToKind map[string]string, metadata *ObjectIDCacheMetadata, countByKind map[string]int, err error) {
	var v2 objectIDCacheFileV2
	if err := json.Unmarshal(data, &v2); err != nil {
		return nil, nil, nil, nil, err
	}
	if v2.Metadata == nil || v2.ByKind == nil {
		return nil, nil, nil, nil, errfmt.Errorf("v2 cache missing metadata or by_kind")
	}
	byKind = make(map[string][]kindBucketEntry, len(v2.ByKind))
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
func normalizeByKindKeys(byKind map[string][]kindBucketEntry, idToKind map[string]string, countByKind map[string]int) {
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

	data, err := os.ReadFile(cachePath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, errfmt.Newf("failed to read cache file").Wrap(err)
	}

	byKind, idToKind, metadata, countByKind, err := parseObjectIDCacheFile(data)
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
	info, err := os.Stat(processDir)
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
func (c *ObjectIDCache) SaveCache(projectRoot string) error {
	saveStart := time.Now()
	var entryCount int
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	cachePath := c.getCacheFilePath(projectRoot)
	cacheDir := filepath.Dir(cachePath)
	if err := os.MkdirAll(cacheDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create cache directory").Wrap(err)
	}

	processDir := datacell.ProcessPrimaryDir(projectRoot)
	info, err := os.Stat(processDir)
	if err != nil {
		return errfmt.Newf("failed to stat process directory").Wrap(err)
	}

	var v2 objectIDCacheFileV2
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
			byKindCopy := make(map[string][]kindBucketEntry, len(c.byKind))
			for k, list := range c.byKind {
				countByKind[k] = len(list)
				countByKindCopy[k] = len(list)
				listCopy := make([]kindBucketEntry, len(list))
				copy(listCopy, list)
				byKindCopy[k] = listCopy
			}
			c.countByKind = countByKind
			v2 = objectIDCacheFileV2{
				Metadata:    c.metadata,
				CountByKind: countByKindCopy,
				ByKind:      byKindCopy,
			}
			entryCount = len(c.idToKind)
			return nil
		},
	)

	data, err := json.MarshalIndent(v2, "", "  ")
	if err != nil {
		return errfmt.Newf("failed to marshal cache").Wrap(err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(cachePath, data, paths.FilePerm644); err != nil { //nolint:gosec // Cache files - 0600 is acceptable
		return errfmt.Newf("failed to write cache file").Wrap(err)
	}

	// Best-effort cache save event emission; the storageProvider is currently unused by
	// emitCacheSaveEventViaCoordinator, so avoid creating extra storage instances (and WAL handles).
	emitCacheSaveEventViaCoordinator(
		pkgctx.NewSystemContext(),
		projectRoot,
		nil,
		entryCount,
		time.Since(saveStart),
		"system",
	)
	return nil
}

// Get retrieves a cache entry by ID (path expanded from relative to full).
func (c *ObjectIDCache) Get(id string) (*ObjectIDCacheEntry, bool) {
	var entry *ObjectIDCacheEntry
	var exists bool
	var cacheSize int
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	_ = concurrency.WithRLockTimeout(
		&c.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameObjectIDCacheGet,
		func() error {
			if c.byKind == nil || c.idToKind == nil {
				return nil
			}
			kind, ok := c.idToKind[id]
			if !ok {
				cacheSize = len(c.idToKind)
				return nil
			}
			list := c.byKind[kind]
			for i := range list {
				if list[i].ID == id {
					entry = c.entryFromBucket(kind, &list[i])
					exists = true
					break
				}
			}
			cacheSize = len(c.idToKind)
			return nil
		},
	)

	if c.byKind == nil {
		logging.Fluent(logger).Warn("ObjectIDCache.Get called but cache is nil").
			ObjectID(id).
			String("diagnostic", "Cache may not have been loaded or was cleared").
			Log()
		return nil, false
	}

	if !exists {
		registry := GetGlobalCacheItemStrategyRegistry()
		registry.HandleCacheMiss(id, cacheSize, logger)
		return nil, false
	}

	registry := GetGlobalCacheItemStrategyRegistry()
	registry.HandleCacheHit(id, logger)
	return entry, true
}

// Set stores a cache entry. Path is stored relative to kind dir (filename or subdir/filename for bucketed).
func (c *ObjectIDCache) Set(id string, entry *ObjectIDCacheEntry) {
	if entry == nil || entry.Kind == emptyValue {
		return
	}
	pathToStore := ""
	if entry.FilePath != emptyValue {
		pathToStore = filepath.Base(entry.FilePath)
		if base := c.getProcessDir(); base != emptyValue {
			kindDir := filepath.Join(base, objects.GetDirectoryFromKind(entry.Kind))
			if rel, err := filepath.Rel(kindDir, entry.FilePath); err == nil && !strings.HasPrefix(rel, "..") {
				pathToStore = filepath.ToSlash(rel)
			}
		}
	}
	e := kindBucketEntry{ID: id, Path: pathToStore, MTime: entry.MTime}
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
				c.byKind = make(map[string][]kindBucketEntry)
			}
			if c.idToKind == nil {
				c.idToKind = make(map[string]string)
			}
			if oldKind, exists := c.idToKind[id]; exists {
				c.byKind[oldKind] = removeIDFromBucket(c.byKind[oldKind], id)
				if c.countByKind != nil {
					c.countByKind[oldKind]--
					if c.countByKind[oldKind] <= 0 {
						delete(c.countByKind, oldKind)
					}
				}
			}
			c.byKind[entry.Kind] = append(c.byKind[entry.Kind], e)
			c.idToKind[id] = entry.Kind
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
			c.byKind[kind] = removeIDFromBucket(c.byKind[kind], id)
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
	info, err := os.Stat(filePath)
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
					c.byKind[kind] = removeIDFromBucket(c.byKind[kind], id)
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
					info, err := os.Stat(fullPath)
					if err != nil {
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
					c.byKind[kind] = removeIDFromBucket(c.byKind[kind], id)
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

	return staleCount
}

// IsStale checks if a specific cache entry is stale
// Returns true if the entry is stale (file doesn't exist or mtime doesn't match)
func (c *ObjectIDCache) IsStale(id string) bool {
	entry, exists := c.Get(id)

	if !exists {
		return false // Entry doesn't exist, not stale
	}

	// Check if file exists
	info, err := os.Stat(entry.FilePath)
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
		emitCacheBuildEventViaCoordinator(
			pkgctx.NewSystemContext(),
			projectRoot,
			nil,
			"load",
			entryCount,
			false,
			time.Since(buildStart),
			"system", // Default profile for cache operations
		)
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

	if err := buildCacheInParallel(ctx, kinds); err != nil {
		return err
	}

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
		emitCacheBuildEventViaCoordinator(
			pkgctx.NewSystemContext(),
			projectRoot,
			nil,
			"build",
			entryCount,
			forceRebuild,
			time.Since(buildStart),
			"system",
		)
	}).Run()

	return nil
}

// ObjectIDCacheProgressNotifier is called during cache load/build to emit progress via coordinator.
// Implementations should use emitObjectIDCacheProgressViaCoordinator so CLI subscribers show progress.
type ObjectIDCacheProgressNotifier interface {
	NotifyCacheProgress(status string, message string)
}

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

// getEnsureRunner returns the loader.Runner for "ensure object ID cache ready" (lazily created).
// The runner runs BuildCache with configurable timeout so --refresh-cache doesn't hit CLI timeout.
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
	projectRoot = ProjectRootOrResolveDot(projectRoot)
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
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		return false
	}
	cache := GetGlobalObjectIDCache()
	loaded, err := cache.LoadCache(projectRoot)
	if err != nil || !loaded {
		return false
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
	projectRoot = ProjectRootOrResolveDot(projectRoot)
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

// tryBuildAndSaveReverseReferenceIndexSync builds and saves the reverse reference index synchronously.
// Kind discovery is bounded by a short timeout; BuildFromScan and SaveCache run in the caller's goroutine (no spawn).
// Returns true if build and save completed successfully, false otherwise.
// When false, caller should trigger background build so next run or daemon can have the cache.
func tryBuildAndSaveReverseReferenceIndexSync(projectRoot string, discoveryTimeout time.Duration) bool {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
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
	projectRoot = ProjectRootOrResolveDot(projectRoot)
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
// Kept low (2–4) per INDEX_FIRST_LOW_CPU_SCAN_DESIGN so background warm does not starve foreground.
func warmWorkerCap() int {
	n := runtime.NumCPU() / 2
	if n < 2 {
		n = 2
	}
	if n > 4 {
		n = 4
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
		storageProvider = getStorageProviderForCache(projectRoot)
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

	fileStorage, ok := storageProvider.(*storage.FileObjectStorage)
	if !ok {
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

	// Warm CAS indexes from cache only (one merge per kind; no per-file disk reads).
	warmWorkers := warmWorkerCap()
	warmQueueSize := min(numKinds, 256)
	poolWarm := goroutinelabels.NewPool(goroutinelabels.DefaultBudget(), "warm_cas", "warming CAS index", warmWorkers, warmQueueSize)
	poolWarm.Start(poolCtx)
	var warmWg sync.WaitGroup
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
			return nil
		})
	}
	warmWg.Wait()
	poolWarm.Stop()

	if ctx != nil && ctx.Err() != nil {
		return
	}
	// Bounded total flush so many kinds don't cause N×timeout wait (was 5s per kind → minutes)
	if flushTimeout == 0 {
		flushTimeout = 60 * time.Second
	}
	if err := storage.FlushAllListingIndexesForProjectRootWithTimeout(projectRoot, flushTimeout); err != nil {
		flushLogger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		logging.Fluent(flushLogger).Warn("CAS index flush did not complete within timeout; discovery may see partial state").
			WithError(err).
			Log()
	}
	cache.notifyCacheProgress("warming", "CAS indexes warmed")
	logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)).Info("CAS indexes warmed")
}

// getStorageProviderForCache gets a storage provider for cache coordination events
// Returns nil if unavailable (best effort - coordination is optional)
func getStorageProviderForCache(projectRoot string) storage.ObjectStorageProvider {
	stdctx := pkgctx.NewSystemContext()
	storageFactory, err := storage.NewStorageFactory(stdctx, projectRoot)
	if err != nil || storageFactory == nil {
		return nil
	}
	return storageFactory.GetStorage()
}

// GetAll returns all cache entries (for duplicate ID detection).
func (c *ObjectIDCache) GetAll() []*ObjectIDCacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.byKind == nil {
		return nil
	}
	var n int
	for _, list := range c.byKind {
		n += len(list)
	}
	entries := make([]*ObjectIDCacheEntry, 0, n)
	for kind, list := range c.byKind {
		for i := range list {
			entries = append(entries, c.entryFromBucket(kind, &list[i]))
		}
	}
	return entries
}

// GetEntriesByKind returns cache entries for the given kind (for cache-driven discovery).
func (c *ObjectIDCache) GetEntriesByKind(kind string) []*ObjectIDCacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.byKind == nil || kind == emptyValue {
		return nil
	}
	list := c.byKind[kind]
	out := make([]*ObjectIDCacheEntry, 0, len(list))
	for i := range list {
		out = append(out, c.entryFromBucket(kind, &list[i]))
	}
	return out
}

// GetKinds returns unique object kinds present in the cache (for cache-driven discovery).
func (c *ObjectIDCache) GetKinds() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.byKind == nil {
		return nil
	}
	internalKinds := map[string]bool{objects.KindBaseMetric: true}
	out := make([]string, 0, len(c.byKind))
	for k := range c.byKind {
		if k != emptyValue && !internalKinds[k] {
			out = append(out, k)
		}
	}
	return out
}

// CountByKind returns the number of cached object IDs per kind.
func (c *ObjectIDCache) CountByKind() map[string]int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.byKind == nil {
		return nil
	}
	if len(c.countByKind) > 0 {
		out := make(map[string]int, len(c.countByKind))
		for k, n := range c.countByKind {
			out[k] = n
		}
		return out
	}
	out := make(map[string]int, len(c.byKind))
	for k, list := range c.byKind {
		out[k] = len(list)
	}
	return out
}

// ClearInMemoryCache clears the in-memory cache so IsPopulatedForProject returns false until
// the next load or build. Call when cache is stale (e.g. count mismatch with storage) so
// discovery uses storage until a rebuild completes.
func (c *ObjectIDCache) ClearInMemoryCache(projectRoot string) {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		return
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameObjectIDCacheClearInMemory,
		func() error {
			meta := c.metadata
			if meta != nil && meta.ProjectRoot == projectRoot {
				c.byKind = make(map[string][]kindBucketEntry)
				c.idToKind = make(map[string]string)
				c.metadata = nil
				c.countByKind = nil
				c.processDir = ""
			}
			return nil
		},
	)
	if r := c.getEnsureRunner(); r != nil {
		r.ResetLoaded()
	}
	storage.InvalidateListCache()
}

// IsPopulatedForProject returns true if the cache has been built for the given projectRoot and has entries.
func (c *ObjectIDCache) IsPopulatedForProject(projectRoot string) bool {
	meta := c.GetMetadata()
	if meta == nil || meta.ProjectRoot != projectRoot {
		return false
	}
	return c.GetEntryCount() > 0
}

// GetEntryCount returns the number of entries in the cache
func (c *ObjectIDCache) GetEntryCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.idToKind)
}

// ValidateCacheFreshnessAgainstStorage checks whether cache per-kind counts match storage.
// Samples up to maxKindsToCheck kinds; if any has cache count != storage count, returns false (stale).
// Used so we don't rely on a stale cache (e.g. loaded from disk from when an object didn't exist).
// Bounded by ctx and a small timeout so it doesn't block long.
// ValidateCacheFreshnessAgainstStorage checks whether the object ID cache is still consistent
// with actual storage. Uses a two-stage strategy:
//
// Stage 1: Total count comparison — get the total count of all objects in storage (one fast
// Count call with no Kind filter) and compare against the total number of entries in the cache.
// If the totals differ the cache is stale; this reliably catches both additions and deletions
// regardless of which kinds changed, without relying on probabilistic sampling.
//
// Stage 2: Per-kind spot-check (up to maxKindsToCheck kinds) — if totals match, verify that
// a sample of kinds individually match to catch rebalancing across kinds with the same total.
//
// maxKindsToCheck is used only for Stage 2. Pass 0 to skip the per-kind spot-check.
func (c *ObjectIDCache) ValidateCacheFreshnessAgainstStorage(ctx stdcontext.Context, projectRoot string, storageProvider storage.ObjectStorageProvider, maxKindsToCheck int) bool {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if storageProvider == nil || projectRoot == emptyValue {
		return true // Can't validate, assume fresh
	}
	c.mu.RLock()
	entryCount := len(c.idToKind)
	kinds := make([]string, 0, len(c.byKind))
	for k := range c.byKind {
		if k != emptyValue {
			kinds = append(kinds, k)
		}
	}
	c.mu.RUnlock()
	if entryCount == 0 || len(kinds) == 0 {
		return true
	}

	secCtx := pkgctx.NewSystemSecurityContext()

	// Stage 1: Total count comparison — one Count call, no kind filter.
	// This is O(1) and catches additions/deletions of any kind deterministically.
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	totalStorageCount, err := storageProvider.Count(ctx, secCtx, storage.ListFilter{})
	if err == nil {
		if entryCount != totalStorageCount {
			return false // Stale: total counts differ
		}
	}
	// If the total Count call errored (e.g. not supported by backend), fall through to per-kind.

	// Stage 2: Per-kind spot-check on a sample of kinds to catch rebalancing edge-cases.
	if maxKindsToCheck <= 0 {
		return true
	}
	if len(kinds) > maxKindsToCheck {
		kinds = kinds[:maxKindsToCheck]
	}
	for _, kind := range kinds {
		if ctx != nil && ctx.Err() != nil {
			return false
		}
		cacheCount := len(c.GetEntriesByKind(kind))
		storageCount, err := storageProvider.Count(ctx, secCtx, storage.ListFilter{Kind: kind})
		if err != nil {
			continue // Skip kind on error (e.g. timeout), don't treat as stale
		}
		if cacheCount != storageCount {
			return false // Stale: per-kind counts don't match
		}
	}
	return true
}

// GetMetadata returns a copy of the cache metadata
func (c *ObjectIDCache) GetMetadata() *ObjectIDCacheMetadata {
	var metadata *ObjectIDCacheMetadata
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithRLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameObjectIDCacheGetMetadata,
		func() error {
			if c.metadata == nil {
				metadata = nil
				return nil
			}
			// Return a copy
			metadataCopy := *c.metadata
			metadata = &metadataCopy
			return nil
		},
	)
	return metadata
}

// InvalidateObjectIDCache invalidates the cache entry for a deleted object
// This should be called when an object is deleted
// Creates an audit event for security and compliance tracking
func InvalidateObjectIDCache(id string) {
	cache := GetGlobalObjectIDCache()

	// Get entry before invalidating (for audit event)
	entry, _ := cache.Get(id)

	cache.Invalidate(id)

	// Create audit event for cache invalidation
	if entry != nil {
		// Only audit if entry existed (don't audit no-ops)
		createCacheAuditEvent("cache_invalidation", id, entry.Kind, entry.FilePath, "Cache entry invalidated due to object deletion", "low", "human")
	}

	persistCacheChanges(cache, emptyValue, "Failed to save cache after invalidation")
}

// UpdateObjectIDCache updates the cache entry for a created or updated object
// This should be called after creating or updating an object file
// Returns error if file doesn't exist (for created objects, file should exist)
// Creates an audit event for security and compliance tracking
// High-volume kinds (audit_event, metrics, etc.) are excluded from the object-id-cache; no-op for those.
func UpdateObjectIDCache(id, kind, filePath string) error {
	if storage.IsHighVolumeKindForCache(kind) {
		return nil // high-volume kinds use high-volume event cache, not object-id-cache
	}
	cache := GetGlobalObjectIDCache()

	// Check if entry existed before update (for audit event)
	_, existed := cache.Get(id)

	err := cache.Update(id, kind, filePath)
	if err != nil {
		return err
	}

	// Create audit event for cache update
	operation := "Cache entry updated for new object"
	if existed {
		operation = "Cache entry updated for modified object"
	}
	createCacheAuditEvent("cache_update", id, kind, filePath, operation, "low", "human")

	persistCacheChanges(cache, emptyValue, "Failed to save cache after update")

	return nil
}

// InvalidateObjectIDCacheKind invalidates all cache entries for a specific kind
// This should be called when bulk operations affect a kind
// Creates an audit event for security and compliance tracking
func InvalidateObjectIDCacheKind(kind string) {
	cache := GetGlobalObjectIDCache()

	// Count entries before invalidation (for audit event)
	var count int
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithRLockTimeout(
		&cache.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameObjectIDCacheCountKind,
		func() error {
			count = len(cache.byKind[kind])
			return nil
		},
	)

	cache.InvalidateKind(kind)
	storage.InvalidateListCacheForKind(kind)

	// Create audit event for bulk cache invalidation
	if count > 0 {
		createCacheAuditEvent("cache_bulk_invalidation", "", kind, "", fmt.Sprintf("Bulk cache invalidation: %d entries removed for kind %s", count, kind), "medium", "human")

		persistCacheChanges(cache, emptyValue, "Failed to save cache after kind invalidation")
	}
}

// CleanStaleCacheEntries validates and removes all stale entries from the object ID cache
// This is useful for cleaning up the cache after bulk deletions or when cache gets out of sync
// Creates an audit event for security and compliance tracking
// Also invalidates the list cache so it stays in sync with the ID cache.
// NOTE: This function only removes stale cache entries. To also update hash registries,
// use PerformCacheFreshnessCheck instead, which calls this AND updates hash registries.
func CleanStaleCacheEntries(projectRoot string) int {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	cache := GetGlobalObjectIDCache()

	// Validate and clean stale entries
	staleCount := cache.ValidateAndCleanStale()

	if staleCount > 0 {
		storage.InvalidateListCache()
	}

	if staleCount > 0 {
		createCacheAuditEvent("cache_cleanup", "", "", "", fmt.Sprintf("Cleaned %d stale cache entries", staleCount), "low", "human")
		persistCacheChanges(cache, projectRoot, "Failed to save cache after cleanup")
	}

	return staleCount
}

// CacheFreshnessHandler is a function type for performing cache freshness checks
type CacheFreshnessHandler func(projectRoot, reason, triggerOperation string, affectedKinds []string) int

var cacheFreshnessHandler CacheFreshnessHandler

// RegisterCacheFreshnessHandler registers a handler for cache freshness checks
// This allows the processor to perform cache freshness operations without creating circular dependencies
func RegisterCacheFreshnessHandler(handler CacheFreshnessHandler) {
	cacheFreshnessHandler = handler
}

// PerformCacheFreshnessCheck performs a cache freshness check
// This validates and cleans stale entries from the object ID cache
// Additionally, it updates hash registry for files that have been modified
// to ensure hash registry stays in sync with file modifications
func PerformCacheFreshnessCheck(projectRoot, reason, triggerOperation string, affectedKinds []string) int {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		return 0
	}

	cache := GetGlobalObjectIDCache()

	// First, update hash registry for files that exist but have been modified
	// This must be done BEFORE removing stale entries so we can still access the cache entries
	// This prevents hash mismatches when files are modified outside normal storage operations
	hashRegistryUpdated := updateHashRegistryForModifiedFiles(cache, projectRoot, affectedKinds)

	// Then validate and clean stale entries
	staleCount := cache.ValidateAndCleanStale()

	if staleCount > 0 {
		storage.InvalidateListCache()
	}

	if staleCount > 0 || hashRegistryUpdated > 0 {
		createCacheAuditEvent("cache_cleanup", "", "", "",
			fmt.Sprintf("Cleaned %d stale cache entries, updated %d hash registry entries", staleCount, hashRegistryUpdated), "low", "human")
		if projectRoot != emptyValue {
			if err := cache.SaveCache(projectRoot); err != nil {
				l := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				logging.Fluent(l).Warn("Failed to save cache after cleanup").WithError(err).Log()
			}
		}
	}

	return staleCount
}

// updateHashRegistryForModifiedFiles updates hash registry for files that exist but have been modified
// This ensures hash registry stays in sync when files are modified outside normal storage operations.
// Runs per-kind updates in parallel using bounded concurrency and goroutinelabels.
func updateHashRegistryForModifiedFiles(cache *ObjectIDCache, projectRoot string, affectedKinds []string) int {
	updateCtx := initializeHashRegistryUpdateContext(cache, projectRoot, affectedKinds)
	collectModifiedFiles(updateCtx)

	if len(updateCtx.KindToFiles) == 0 {
		return 0
	}

	// Run per-kind updates in parallel (each kind uses its own registry; no shared mutable state).
	type kindWork struct {
		kind      string
		filePaths []string
	}
	var work []kindWork
	for kind, filePaths := range updateCtx.KindToFiles {
		work = append(work, kindWork{kind: kind, filePaths: filePaths})
	}

	results := make(chan int, len(work))
	maxConcurrency := min(8, len(work))
	queueSize := min(len(work), 256)

	poolCtx := stdcontext.Background()
	pool := goroutinelabels.NewPool(goroutinelabels.DefaultBudget(), "update_hash_registry", "updating hash registry", maxConcurrency, queueSize)
	pool.Start(poolCtx)
	for _, w := range work {
		kind, filePaths := w.kind, w.filePaths
		_ = pool.Submit(poolCtx, func(stdcontext.Context) error {
			n := updateHashRegistryForKind(updateCtx, kind, filePaths)
			results <- n
			return nil
		})
	}
	pool.Stop()

	updatedCount := 0
	for i := 0; i < len(work); i++ {
		updatedCount += <-results
	}
	return updatedCount
}

// BulkInvalidateObjectIDCache invalidates multiple cache entries by ID
// This should be called when bulk delete operations are performed
// Creates an audit event for security and compliance tracking
// Optionally saves the cache to disk if projectRoot is provided
// Also invalidates the list cache so it stays in sync with the ID cache.
func BulkInvalidateObjectIDCache(ids []string, projectRoot string) int {
	cache := GetGlobalObjectIDCache()

	// Bulk invalidate and get count
	count := cache.BulkInvalidate(ids)

	if count > 0 {
		storage.InvalidateListCache()
	}

	// Create audit event for bulk cache invalidation
	if count > 0 {
		createCacheAuditEvent("cache_bulk_invalidation", "", "", "", fmt.Sprintf("Bulk cache invalidation: %d entries removed", count), "medium", "human")

		persistCacheChanges(cache, projectRoot, "Failed to save cache after bulk invalidation")
	}

	return count
}

// persistCacheChanges is a helper function to avoid duplicating the persistence block across cache mutations.
func persistCacheChanges(cache *ObjectIDCache, explicitProjectRoot, contextMsg string) {
	root := explicitProjectRoot
	if root == emptyValue {
		if meta := cache.GetMetadata(); meta != nil {
			root = meta.ProjectRoot
		}
	}
	if root != emptyValue {
		if err := cache.SaveCache(root); err != nil {
			l := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			logging.Fluent(l).Warn(contextMsg).WithError(err).Log()
		}
	}
}
