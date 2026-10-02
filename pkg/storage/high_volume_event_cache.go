package storage

import (
	stdcontext "context"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/config"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
)

func timeoutContextWithLogger(timeout time.Duration) (stdcontext.Context, stdcontext.CancelFunc, logging.Logger) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), timeout)
	return ctx, cancel, logger
}

const (
	highVolumeEventCacheFile    = "high-volume-events-cache.json"
	highVolumeEventCacheVersion = "2.0" // v2: bucketed by kind → event_type → id to avoid duplicative fields
	// maxHighVolumeEventCacheEntries caps in-memory cache size to avoid unbounded growth (SCHEDULER_MEMORY_AND_HANG_ANALYSIS).
	// Oldest entries by CreatedAt are evicted when over cap. Retention still deletes from storage; cache reflects a bounded window.
	maxHighVolumeEventCacheEntries = 500000

	pipelineKindHighVolumeEventCacheBuild = "storage.high_volume_event_cache_build"
)

// HighVolumeEventCacheEntry represents a cached high-volume event with metadata.
// Status is populated during cache build (index/stream) for retention max_count with protect_statuses; not persisted in v2 on-disk.
type HighVolumeEventCacheEntry struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
	EventType string    `json:"event_type,omitempty"` // For audit_event
	Status    string    `json:"status,omitempty"`     // For retention: exclude protect_statuses when using QueryOldestByKindExcludingStatus
	FilePath  string    `json:"file_path"`
	MTime     time.Time `json:"mtime"`
	Exists    bool      `json:"exists"`
}

// HighVolumeEventCacheMetadata stores cache metadata.
// HasStatus is true when cache was built from index/stream (status populated); false when loaded from disk (v2 minimal has no status).
type HighVolumeEventCacheMetadata struct {
	Version     string    `json:"version"`
	BuildTime   time.Time `json:"build_time"`
	ProjectRoot string    `json:"project_root"`
	EntryCount  int       `json:"entry_count"`
	HasStatus   bool      `json:"has_status,omitempty"` // true only after build; enables QueryOldestByKindExcludingStatus for retention
}

// highVolumeEventCacheEntryMinimal is the v2 on-disk per-event payload (kind and event_type live in bucket path).
// Only created_at is stored; presence in cache implies exists.
type highVolumeEventCacheEntryMinimal struct {
	CreatedAt time.Time `json:"created_at"`
}

// highVolumeEventCacheV2Buckets is the v2 on-disk shape: kind → event_type → id → minimal entry
type highVolumeEventCacheV2Buckets map[string]map[string]map[string]*highVolumeEventCacheEntryMinimal

// HighVolumeEventCache is a thread-safe cache for high-volume events (audit_event, metrics)
// Optimized for time-window queries and fast Count() operations
type HighVolumeEventCache struct {
	mu       sync.RWMutex
	cache    map[string]*HighVolumeEventCacheEntry // id -> entry
	byTime   []*HighVolumeEventCacheEntry          // Sorted by CreatedAt (ascending)
	metadata *HighVolumeEventCacheMetadata
	cacheDir string

	// Track if cache needs rebuild (stale entries, etc.)
	needsRebuild atomic.Bool
}

var (
	globalHighVolumeEventCache *HighVolumeEventCache
	highVolumeCacheOnce        sync.Once
	globalHighVolumeCacheMu    sync.RWMutex
)

// getHighVolumeCacheBuildWorkers returns worker count for cache build from index. Default 64; override
// with ZQK_HIGH_VOLUME_CACHE_BUILD_WORKERS for low/mid-tier hardware. Clamped for safety.
func getHighVolumeCacheBuildWorkers() int {
	const (
		defaultWorkers = 64
		minWorkers     = 4
		maxWorkers     = 128
	)
	n := config.StorageHighVolumeCacheBuildWorkers().OrDefault(defaultWorkers)
	if n <= 0 {
		return defaultWorkers
	}
	return ClampInt(n, minWorkers, maxWorkers)
}

// getHighVolumeCacheKindParallelism caps how many high-volume kinds build concurrently in buildCacheData.
// Each kind still fans out to getHighVolumeCacheBuildWorkers goroutines for CAS reads; limiting kinds
// avoids exploding open files / scheduler pressure on mid-tier hosts. Override with HIGH_VOLUME_CACHE_KIND_PARALLELISM (brand-prefixed env).
func getHighVolumeCacheKindParallelism() int {
	const (
		defaultParallelKinds = 4
		minParallelKinds     = 1
		maxParallelKinds     = 8
	)
	n := config.StorageHighVolumeCacheKindParallelism().OrDefault(defaultParallelKinds)
	if n <= 0 {
		return defaultParallelKinds
	}
	return ClampInt(n, minParallelKinds, maxParallelKinds)
}

// newHighVolumeCacheGoroutine returns a builder with DefaultBudget when configured, matching
// goroutinelabels usage in operation_executor_workers and system coordination packages.
func newHighVolumeCacheGoroutine(name, purpose string) *goroutinelabels.GoroutineBuilder {
	b := goroutinelabels.NewGoroutine(name, purpose)
	if bud := goroutinelabels.DefaultBudget(); bud != nil {
		b = b.WithBudget(bud)
	}
	return b
}

// GetGlobalHighVolumeEventCache returns the global high-volume event cache instance
func GetGlobalHighVolumeEventCache() *HighVolumeEventCache {
	globalHighVolumeCacheMu.RLock()
	c := globalHighVolumeEventCache
	globalHighVolumeCacheMu.RUnlock()
	if c != nil {
		return c
	}

	globalHighVolumeCacheMu.Lock()
	defer globalHighVolumeCacheMu.Unlock()
	if globalHighVolumeEventCache == nil {
		globalHighVolumeEventCache = NewHighVolumeEventCache()
	}
	return globalHighVolumeEventCache
}

// ResetGlobalHighVolumeEventCacheForTesting resets the global high-volume event cache instance for tests.
func ResetGlobalHighVolumeEventCacheForTesting() {
	StopHighVolumeEventCachePersistForTest(emptyValue)
	globalHighVolumeCacheMu.Lock()
	defer globalHighVolumeCacheMu.Unlock()
	globalHighVolumeEventCache = nil
	highVolumeCacheOnce = sync.Once{}
}

// NewHighVolumeEventCache creates a new high-volume event cache
func NewHighVolumeEventCache() *HighVolumeEventCache {
	return &HighVolumeEventCache{
		cache:    make(map[string]*HighVolumeEventCacheEntry),
		byTime:   make([]*HighVolumeEventCacheEntry, 0),
		metadata: nil,
		cacheDir: "",
	}
}

// getCacheFilePath returns the path to the cache file
func (c *HighVolumeEventCache) getCacheFilePath(projectRoot string) string {
	if c.cacheDir != emptyValue {
		return filepath.Join(c.cacheDir, highVolumeEventCacheFile)
	}
	// Default to .zqk/cache in project root
	cacheDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.CacheDir)
	return filepath.Join(cacheDir, highVolumeEventCacheFile)
}

// LoadCache loads the cache from disk if it exists
// Returns true if cache was successfully loaded, false if cache needs to be rebuilt

// Check if cache file exists

// Parse cache file (v1: entries; v2: buckets)

// Cache corrupted, will rebuild

// Validate cache metadata

// Check if project root matches

// Different project

// Accept current version or v1 (so we can load old caches and re-save as v2)

// Version mismatch, rebuild

// rebuildTimeIndex rebuilds the time-indexed slice from the cache map
// Must be called with write lock held
func (c *HighVolumeEventCache) rebuildTimeIndex() {
	c.byTime = make([]*HighVolumeEventCacheEntry, 0, len(c.cache))
	for _, entry := range c.cache {
		if entry != nil && entry.Exists {
			c.byTime = append(c.byTime, entry)
		}
	}
	// Sort by CreatedAt (ascending)
	sort.Slice(c.byTime, func(i, j int) bool {
		return c.byTime[i].CreatedAt.Before(c.byTime[j].CreatedAt)
	})
}

// evictOldestToCap removes oldest entries (by CreatedAt) until cache size <= maxEntries.
// Must be called with write lock held. byTime must already be sorted by CreatedAt asc.
// No-op when maxEntries <= 0 (cap disabled).
func (c *HighVolumeEventCache) evictOldestToCap(maxEntries int) {
	if maxEntries <= 0 {
		return
	}
	if len(c.cache) <= maxEntries {
		return
	}
	toRemove := len(c.byTime) - maxEntries
	if toRemove <= 0 {
		return
	}
	for i := 0; i < toRemove && i < len(c.byTime); i++ {
		delete(c.cache, c.byTime[i].ID)
	}
	c.byTime = c.byTime[toRemove:]
}

// insertEntryInTimeOrder inserts entry into byTime keeping CreatedAt ascending order.
// Only inserts when entry.Exists. Must be called with write lock held.
// Caller should call evictOldestToCap after if cap is enforced.
func (c *HighVolumeEventCache) insertEntryInTimeOrder(entry *HighVolumeEventCacheEntry) {
	if entry == nil || !entry.Exists {
		return
	}
	// Binary search: smallest i such that byTime[i].CreatedAt >= entry.CreatedAt (or end)
	pos := sort.Search(len(c.byTime), func(i int) bool {
		return !c.byTime[i].CreatedAt.Before(entry.CreatedAt)
	})
	// Insert at pos: byTime = [..., entry, ...]
	c.byTime = append(c.byTime, nil)
	copy(c.byTime[pos+1:], c.byTime[pos:])
	c.byTime[pos] = entry
}

// removeEntryFromByTime removes the entry with the given id from byTime (single occurrence).
// Must be called with write lock held. No-op if id not found.
func (c *HighVolumeEventCache) removeEntryFromByTime(id string) {
	for i := 0; i < len(c.byTime); i++ {
		if c.byTime[i] != nil && c.byTime[i].ID == id {
			c.byTime = append(c.byTime[:i], c.byTime[i+1:]...)
			return
		}
	}
}

// flatMapFromBuckets converts v2 buckets (kind → event_type → id → minimal) into id → full entry map.
// Must be called with write lock held (or during load before any other use).

// in cache => exists

// bucketsFromCache builds v2 buckets from the flat cache (kind → event_type → id → minimal).
// Must be called with read or write lock held.

// SaveCache saves the cache to disk
// Follows established pattern: minimize lock hold time, especially during I/O
// Pattern: RLock to read count → I/O operations → Lock to prepare data → I/O operations

// Step 2: I/O operations WITHOUT lock (cache directory creation)

// Step 3: Prepare cache data with Lock (update metadata; build v2 buckets for smaller on-disk size)

// Step 4: I/O operations WITHOUT lock (JSON marshaling and file write)

func (c *HighVolumeEventCache) withLockInternal(lockName string, isWrite bool, op func() error) error {
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	return concurrency.WithRWMutexCtxLogger(&c.mu, ctx, lockName, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), isWrite, op)
}

func (c *HighVolumeEventCache) withReadLock(lockName string, op func() error) error {
	return c.withLockInternal(lockName, false, op)
}

func (c *HighVolumeEventCache) withWriteLock(lockName string, op func() error) error {
	return c.withLockInternal(lockName, true, op)
}

// Get retrieves a cache entry by ID
func (c *HighVolumeEventCache) Get(id string) (*HighVolumeEventCacheEntry, bool) {
	var entry *HighVolumeEventCacheEntry
	var exists bool
	var err_swallow_19 = c.withReadLock(
		locknames.LockNameHighVolumeCacheGet,
		func() error {
			var ok bool
			entry, ok = c.cache[id]
			exists = ok && entry != nil && entry.Exists
			return nil
		},
	)
	if err_swallow_19 != nil {
		logging.LogSwallowedError(err_swallow_19)
	}
	return entry, exists
}

func (c *HighVolumeEventCache) Set(entry *HighVolumeEventCacheEntry) {
	var err_swallow_20 = c.withWriteLock(
		locknames.LockNameHighVolumeCacheSet,
		func() error {
			if existing := c.cache[entry.ID]; existing != nil {
				c.removeEntryFromByTime(entry.ID)
			}
			c.cache[entry.ID] = entry
			c.insertEntryInTimeOrder(entry)
			c.evictOldestToCap(maxHighVolumeEventCacheEntries)
			return nil
		},
	)
	if err_swallow_20 != nil {
		logging.LogSwallowedError(err_swallow_20)
	}
}

func (c *HighVolumeEventCache) Invalidate(id string) {
	var err_swallow_21 = c.withWriteLock(
		locknames.LockNameHighVolumeCacheInvalidate,
		func() error {
			delete(c.cache, id)
			c.removeEntryFromByTime(id)
			return nil
		},
	)
	if err_swallow_21 != nil {
		logging.LogSwallowedError(err_swallow_21)
	}
}

func (c *HighVolumeEventCache) InvalidateForProject(projectRoot string) {
	if projectRoot == emptyValue {
		return
	}
	var err_swallow_22 = c.withWriteLock(
		locknames.LockNameHighVolumeCacheInvalidateProject,
		func() error {
			if c.metadata != nil && c.metadata.ProjectRoot == projectRoot {
				c.cache = make(map[string]*HighVolumeEventCacheEntry)
				c.byTime = make([]*HighVolumeEventCacheEntry, 0)
				c.metadata = nil
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				StorageLog(logger).Debug(LogEventStorageHighVolumeCacheInvalidatedProjectDebug).
					ProjectRoot(projectRoot).
					Log()
			}
			return nil
		},
	)
	if err_swallow_22 != nil {
		logging.LogSwallowedError(err_swallow_22)
	}
}
