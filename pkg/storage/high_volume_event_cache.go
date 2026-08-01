package storage

import (
	stdcontext "context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/pipeline"
	"github.com/lanceman/zqk/pkg/storage/locknames"
	"github.com/lanceman/zqk/pkg/when"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"gopkg.in/yaml.v3"
)

const (
	highVolumeEventCacheFile    = "high-volume-events-cache.json"
	highVolumeEventCacheVersion = "2.0" // v2: bucketed by kind → event_type → id to avoid duplicative fields
	// maxHighVolumeEventCacheEntries caps in-memory cache size to avoid unbounded growth (SCHEDULER_MEMORY_AND_HANG_ANALYSIS).
	// Oldest entries by CreatedAt are evicted when over cap. Retention still deletes from storage; cache reflects a bounded window.
	maxHighVolumeEventCacheEntries = 500000

	pipelineKindHighVolumeEventCacheBuild = "storage.high_volume_event_cache_build"
)

// highVolumeKindsForCacheBuild lists kinds that get index-based cache build (CAS + created_at). Order: audit_event first (highest volume), then metrics and other high-volume kinds. See HIGH_VOLUME_EVENT_INDEXES.md, high_volume_kinds.yaml.
var highVolumeKindsForCacheBuild = []string{
	objects.KindAuditEvent,
	objects.KindAuditAggregationMetric,
	objects.KindBaseMetric,
	objects.KindCommandMetric,
	objects.KindFileLockMetric,
	objects.KindCodeQualityMetric,
	objects.KindSchedulerHealthMetric,
	objects.KindChangeJournalEntry,
	objects.KindMcpSession,
	objects.KindSchedulerJob,
	objects.KindZqkSession,
	objects.KindVerificationMatrix,
}

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
)

// getHighVolumeCacheBuildWorkers returns worker count for cache build from index. Default 64; override
// with ZQK_HIGH_VOLUME_CACHE_BUILD_WORKERS for low/mid-tier hardware. Clamped for safety.
func getHighVolumeCacheBuildWorkers() int {
	const (
		defaultWorkers = 64
		minWorkers     = 4
		maxWorkers     = 128
	)
	v := os.Getenv(zqkenv.HighVolumeCacheBuildWorkers())
	if v == emptyValue {
		return defaultWorkers
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return defaultWorkers
	}
	if n < minWorkers {
		return minWorkers
	}
	if n > maxWorkers {
		return maxWorkers
	}
	return n
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
	v := os.Getenv(zqkenv.HighVolumeCacheKindParallelism())
	if v == emptyValue {
		return defaultParallelKinds
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return defaultParallelKinds
	}
	if n < minParallelKinds {
		return minParallelKinds
	}
	if n > maxParallelKinds {
		return maxParallelKinds
	}
	return n
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
	highVolumeCacheOnce.Do(func() {
		globalHighVolumeEventCache = NewHighVolumeEventCache()
	})
	return globalHighVolumeEventCache
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
func (c *HighVolumeEventCache) LoadCache(projectRoot string) (bool, error) {
	cachePath := c.getCacheFilePath(projectRoot)
	c.cacheDir = filepath.Dir(cachePath)

	// Check if cache file exists
	data, err := os.ReadFile(cachePath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, errfmt.Newf(ConstMiscFailedToReadCacheFile).Wrap(err)
	}

	// Parse cache file (v1: entries; v2: buckets)
	var cacheData struct {
		Metadata *HighVolumeEventCacheMetadata         `json:"metadata"`
		Entries  map[string]*HighVolumeEventCacheEntry `json:"entries"`
		Buckets  highVolumeEventCacheV2Buckets         `json:"buckets"`
	}
	if err := json.Unmarshal(data, &cacheData); err != nil {
		return false, nil // Cache corrupted, will rebuild
	}

	// Validate cache metadata
	if cacheData.Metadata == nil {
		return false, nil
	}

	// Check if project root matches
	if cacheData.Metadata.ProjectRoot != projectRoot {
		return false, nil // Different project
	}

	// Accept current version or v1 (so we can load old caches and re-save as v2)
	version := cacheData.Metadata.Version
	if version != highVolumeEventCacheVersion && version != "1.0" {
		return false, nil // Version mismatch, rebuild
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var err_swallow_16 = concurrency.WithLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameHighVolumeCacheLoad,
		func() error {
			when.When(func() bool { return version == "2.0" && len(cacheData.Buckets) > 0 }).Then(func() {
				c.cache = c.flatMapFromBuckets(cacheData.Buckets)
			}).OrElseWhen(func() bool { return len(cacheData.Entries) > 0 }).Then(func() {
				c.cache = cacheData.Entries
			}).OrElse(func() {
				c.cache = make(map[string]*HighVolumeEventCacheEntry)
			}).Run()
			c.metadata = cacheData.Metadata
			if c.metadata != nil {
				c.metadata.HasStatus = false
			}
			c.rebuildTimeIndex()
			c.evictOldestToCap(maxHighVolumeEventCacheEntries)
			return nil
		},
	)
	if err_swallow_16 != nil {
		logging.LogSwallowedError(err_swallow_16)
	}

	StorageLog(logger).Debug(LogEventStorageHighVolumeCacheLoadedDebug).
		EntryCount(len(c.cache)).
		ProjectRoot(projectRoot).
		Log()
	return true, nil
}

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
func (c *HighVolumeEventCache) flatMapFromBuckets(buckets highVolumeEventCacheV2Buckets) map[string]*HighVolumeEventCacheEntry {
	out := make(map[string]*HighVolumeEventCacheEntry)
	for kind, byEventType := range buckets {
		if byEventType == nil {
			continue
		}
		for eventType, byID := range byEventType {
			if byID == nil {
				continue
			}
			for id, m := range byID {
				if m == nil {
					continue
				}
				out[id] = &HighVolumeEventCacheEntry{
					ID:        id,
					Kind:      kind,
					CreatedAt: m.CreatedAt,
					EventType: eventType,
					Exists:    true, // in cache => exists
				}
			}
		}
	}
	return out
}

// bucketsFromCache builds v2 buckets from the flat cache (kind → event_type → id → minimal).
// Must be called with read or write lock held.
func (c *HighVolumeEventCache) bucketsFromCache() highVolumeEventCacheV2Buckets {
	buckets := make(highVolumeEventCacheV2Buckets)
	for id, e := range c.cache {
		if e == nil {
			continue
		}
		kind := e.Kind
		if kind == emptyValue {
			kind = objects.KindAuditEvent
		}
		eventType := e.EventType
		minimal := &highVolumeEventCacheEntryMinimal{CreatedAt: e.CreatedAt}
		if byKind, ok := buckets[kind]; !ok {
			buckets[kind] = map[string]map[string]*highVolumeEventCacheEntryMinimal{
				eventType: {id: minimal},
			}
		} else {
			if byEventType, ok := byKind[eventType]; !ok {
				byKind[eventType] = map[string]*highVolumeEventCacheEntryMinimal{id: minimal}
			} else {
				byEventType[id] = minimal
			}
		}
	}
	return buckets
}

// SaveCache saves the cache to disk
// Follows established pattern: minimize lock hold time, especially during I/O
// Pattern: RLock to read count → I/O operations → Lock to prepare data → I/O operations
func (c *HighVolumeEventCache) SaveCache(projectRoot string) error {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var entryCount int
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	var err_swallow_17 = concurrency.WithRLockTimeout(
		&c.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameHighVolumeCacheSaveGetCount,
		func() error {
			entryCount = len(c.cache)
			return nil
		},
	)
	if err_swallow_17 !=

		// Step 2: I/O operations WITHOUT lock (cache directory creation)
		nil {
		logging.LogSwallowedError(err_swallow_17)
	}

	cachePath := c.getCacheFilePath(projectRoot)
	cacheDir := filepath.Dir(cachePath)
	if err := os.MkdirAll(cacheDir, paths.DirPerm755); err != nil {
		return errfmt.Newf(ConstMiscFailedToCreateCacheDirectory).Wrap(err)
	}

	// Step 3: Prepare cache data with Lock (update metadata; build v2 buckets for smaller on-disk size)
	var cacheData struct {
		Metadata *HighVolumeEventCacheMetadata `json:"metadata"`
		Buckets  highVolumeEventCacheV2Buckets `json:"buckets"`
	}
	ctx2, cancel2 := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel2()
	var err_swallow_18 = concurrency.WithLockTimeout(
		&c.mu,
		ctx2,
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameHighVolumeCacheSavePrepare,
		func() error {
			when.When(func() bool { return c.metadata == nil }).Then(func() {
				c.metadata = &HighVolumeEventCacheMetadata{
					Version:     highVolumeEventCacheVersion,
					BuildTime:   time.Now().UTC(),
					ProjectRoot: projectRoot,
					EntryCount:  entryCount,
				}
			}).OrElse(func() {
				c.metadata.Version = highVolumeEventCacheVersion
				c.metadata.BuildTime = time.Now().UTC()
				c.metadata.EntryCount = entryCount

			}).Run()
			cacheData.Metadata = c.metadata
			cacheData.Buckets = c.bucketsFromCache()
			return nil
		},
	)
	if err_swallow_18 !=

		// Step 4: I/O operations WITHOUT lock (JSON marshaling and file write)
		nil {
		logging.LogSwallowedError(err_swallow_18)
	}

	data, err := json.MarshalIndent(cacheData, "", "  ")
	if err != nil {
		return errfmt.Newf(ConstMiscFailedToMarshalCache).Wrap(err)
	}

	if err := os.WriteFile(cachePath, data, paths.FilePerm600); err != nil {
		return errfmt.Newf(ConstMiscFailedToWriteCacheFile).Wrap(err)
	}

	return nil
}

// Get retrieves a cache entry by ID
func (c *HighVolumeEventCache) Get(id string) (*HighVolumeEventCacheEntry, bool) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var entry *HighVolumeEventCacheEntry
	var exists bool
	var err_swallow_19 = concurrency.WithRLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameHighVolumeCacheGet,
		func() error {
			var ok bool
			entry, ok = c.cache[id]
			exists = ok && entry != nil && entry.Exists
			return nil
		},
	)
	if err_swallow_19 != nil {
		logging.

			// Set stores a cache entry. Uses incremental byTime update (insert in order) to avoid
			// full rebuild on every create, which caused high allocation and GC pressure at 500k entries (SCHEDULER_MEMORY_AND_HANG_ANALYSIS).
			LogSwallowedError(err_swallow_19)
	}
	return entry, exists
}

func (c *HighVolumeEventCache) Set(entry *HighVolumeEventCacheEntry) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var err_swallow_20 = concurrency.WithLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
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
	if err_swallow_20 !=

		// Invalidate removes an entry from the cache. Uses incremental byTime update (remove one element)
		// to avoid full rebuild on every delete.
		nil {
		logging.LogSwallowedError(err_swallow_20)
	}
}

func (c *HighVolumeEventCache) Invalidate(id string) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var err_swallow_21 = concurrency.WithLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameHighVolumeCacheInvalidate,
		func() error {
			delete(c.cache, id)
			c.removeEntryFromByTime(id)

			return nil
		},
	)
	if err_swallow_21 !=

		// InvalidateForProject clears the cache for the given project so IsPopulatedForProject(projectRoot) becomes false.
		// Call after bulk deletes or when index/disk reconciliation may have changed counts, so next Count() uses index or triggers rebuild.
		nil {
		logging.LogSwallowedError(err_swallow_21)
	}
}

func (c *HighVolumeEventCache) InvalidateForProject(projectRoot string) {
	if projectRoot == emptyValue {
		return
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var err_swallow_22 = concurrency.WithLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameHighVolumeCacheInvalidateProject,
		func() error {
			if c.metadata != nil && c.metadata.ProjectRoot == projectRoot {
				c.cache = make(map[string]*HighVolumeEventCacheEntry)
				c.byTime = make([]*HighVolumeEventCacheEntry, 0)
				c.metadata = nil
				StorageLog(logger).Debug(LogEventStorageHighVolumeCacheInvalidatedProjectDebug).
					ProjectRoot(projectRoot).
					Log()
			}
			return nil
		},
	)
	if err_swallow_22 !=

		// QueryByTimeWindow returns event IDs within a time window (inclusive)
		// Uses binary search for O(log n) performance
		nil {
		logging.LogSwallowedError(err_swallow_22)
	}
}

func (c *HighVolumeEventCache) QueryByTimeWindow(startTime, endTime time.Time, limit int) []string {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var result []string
	var err_swallow_23 = concurrency.WithRLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameHighVolumeCacheQueryTimeWindow,
		func() error {
			if len(c.byTime) == 0 {
				return nil
			}

			startIdx := sort.Search(len(c.byTime), func(i int) bool {
				return !c.byTime[i].CreatedAt.Before(startTime)
			})

			endIdx := sort.Search(len(c.byTime), func(i int) bool {
				return c.byTime[i].CreatedAt.After(endTime)
			})

			result = make([]string, 0, endIdx-startIdx)
			for i := startIdx; i < endIdx && (limit <= 0 || len(result) < limit); i++ {
				if c.byTime[i].Exists {
					result = append(result, c.byTime[i].ID)
				}
			}
			return nil
		},
	)
	if err_swallow_23 != nil {

		// QueryOlderThan returns event IDs older than cutoff time
		// Note: If cache is partial (e.g., only 10k of 17k events cached), this will only return
		// IDs from cached events. The aggregation service falls back to storage queries if cache
		// returns empty or insufficient results.
		logging.LogSwallowedError(err_swallow_23)
	}
	return result
}

func (c *HighVolumeEventCache) QueryOlderThan(cutoffTime time.Time, limit int) []string {
	return c.QueryByTimeWindow(time.Time{}, cutoffTime, limit)
}

// QueryOldestByKind returns the oldest event IDs for a given kind (by created_at), up to limit.
// Used by retention max_count enforcement to delete oldest audit_events without List() over 260k+ files.
func (c *HighVolumeEventCache) QueryOldestByKind(kind string, limit int) []string {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var result []string
	var err_swallow_24 = concurrency.WithRLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameHighVolumeCacheQueryOldestByKind,
		func() error {
			for _, entry := range c.byTime {
				if limit > 0 && len(result) >= limit {
					break
				}
				if entry != nil && entry.Exists && entry.Kind == kind {
					result = append(result, entry.ID)
				}
			}
			return nil
		},
	)
	if err_swallow_24 != nil {

		// QueryOldestByKindExcludingStatus returns oldest event IDs for kind (by created_at) whose status is not in excludeStatuses, up to limit.
		// Use when cache was built from index/stream (HasStatus true); avoids full List for retention max_count with protect_statuses. See RETENTION_MAX_COUNT_PERFORMANCE.md.
		logging.LogSwallowedError(err_swallow_24)
	}
	return result
}

func (c *HighVolumeEventCache) QueryOldestByKindExcludingStatus(kind string, limit int, excludeStatuses []string) []string {
	excludeSet := make(map[string]bool)
	for _, s := range excludeStatuses {
		excludeSet[s] = true
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var result []string
	var err_swallow_25 = concurrency.WithRLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameHighVolumeCacheQueryOldestExcludingStatus,
		func() error {
			for _, entry := range c.byTime {
				if limit > 0 && len(result) >= limit {
					break
				}
				if entry == nil || !entry.Exists || entry.Kind != kind {
					continue
				}
				if entry.Status == emptyValue {
					continue
				}
				if excludeSet[entry.Status] {
					continue
				}
				result = append(result, entry.ID)
			}
			return nil
		},
	)
	if err_swallow_25 != nil {

		// HasStatus returns true when cache entries have Status populated (built from index/stream). False when loaded from disk.
		logging.LogSwallowedError(err_swallow_25)
	}
	return result
}

func (c *HighVolumeEventCache) HasStatus() bool {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var has bool
	var err_swallow_26 = concurrency.WithRLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameHighVolumeCacheHasStatus,
		func() error {
			has = c.metadata != nil && c.metadata.HasStatus
			return nil
		},
	)
	if err_swallow_26 !=

		// Count returns total count of events in cache
		nil {
		logging.LogSwallowedError(err_swallow_26)
	}
	return has
}

func (c *HighVolumeEventCache) Count() int {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var count int
	var err_swallow_27 = concurrency.WithRLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameHighVolumeCacheCount,
		func() error {
			count = len(c.cache)
			return nil
		},
	)
	if err_swallow_27 != nil {

		// CountByKind returns count of events for a specific kind
		logging.LogSwallowedError(err_swallow_27)
	}
	return count
}

func (c *HighVolumeEventCache) CountByKind(kind string) int {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var count int
	var err_swallow_28 = concurrency.WithRLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameHighVolumeCacheCountByKind,
		func() error {
			for _, entry := range c.cache {
				if entry != nil && entry.Exists && entry.Kind == kind {
					count++
				}
			}
			return nil
		},
	)
	if err_swallow_28 != nil {

		// CountByTimeWindow returns count of events within a time window
		logging.LogSwallowedError(err_swallow_28)
	}
	return count
}

func (c *HighVolumeEventCache) CountByTimeWindow(startTime, endTime time.Time) int {
	ids := c.QueryByTimeWindow(startTime, endTime, 0)
	return len(ids)
}

// IsPopulatedForProject returns true if cache has been built for the given projectRoot
func (c *HighVolumeEventCache) IsPopulatedForProject(projectRoot string) bool {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var populated bool
	var err_swallow_29 = concurrency.WithRLockTimeout(
		&c.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		locknames.LockNameHighVolumeCacheIsPopulated,
		func() error {
			if c.metadata == nil {
				populated = false
				return nil
			}
			populated = c.metadata.ProjectRoot == projectRoot && len(c.cache) > 0
			return nil
		},
	)
	if err_swallow_29 != nil {
		logging.

			// BuildCache builds the cache by scanning high-volume event directories.
			// Heavy work (List, building map/slice) runs in a background goroutine; the only lock hold is the final swap.
			// Caller blocks until the background build and swap complete (so cache is ready when BuildCache returns).
			LogSwallowedError(err_swallow_29)
	}
	return populated
}

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

					// Only block (hold lock) during the final swap.
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
								HasStatus:   true, // built from index/stream so entries have Status for retention
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

	// Respect ctx deadline so aggregation handler 10s timeout works (scheduler diagnostics: BuildCache was blocking 78+ min).
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// buildCacheData builds the cache. Prefers index-based path (CAS ListIDs + parallel read) to avoid
// List()'s 500s legacy scan and 47k full reads; falls back to List when not FileObjectStorage.
func (c *HighVolumeEventCache) buildCacheData(ctx stdcontext.Context, projectRoot string, storageProvider ObjectStorageProvider, logger logging.Logger) (map[string]*HighVolumeEventCacheEntry, []*HighVolumeEventCacheEntry, error) {
	newCache := make(map[string]*HighVolumeEventCacheEntry)
	newByTime := make([]*HighVolumeEventCacheEntry, 0)

	// Fast path: build from CAS index and stream registry for all high-volume kinds. Merge by ID so stream-backed
	// objects (scheduler_job, zqk_session, etc.) are included after rebuild/restart. Enables efficient Count/OldestIDs (HIGH_VOLUME_EVENT_INDEXES.md).
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
		for _, k := range highVolumeKindsForCacheBuild {
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
		wg.Wait()
		if firstErr != nil {
			return nil, nil, firstErr
		}
		if totalEntries.Load() > 0 {
			sort.Slice(newByTime, func(i, j int) bool {
				return newByTime[i].CreatedAt.Before(newByTime[j].CreatedAt)
			})
			StorageLog(logger).Info(LogEventStorageHighVolumeCacheBuiltAllKindsInfo).
				EntryCount(int(totalEntries.Load())). //nolint:gosec // Safe: totalEntries.Load() is unlikely to exceed int range for log counts.
				Log()
			return newCache, newByTime, nil
		}
	}

	// Fallback: List (slow for 47k: legacy scan + full reads; may timeout)
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
				if info, err := os.Stat(statPath); err == nil {
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
			kindEntries[e.ID] = e // stream overwrites CAS when both exist
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
	// Cap work when ctx has deadline so aggregation handler timeout can stop build (scheduler diagnostics: 78+ min blocks).
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
						if info, err := os.Stat(statPath); err == nil {
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
