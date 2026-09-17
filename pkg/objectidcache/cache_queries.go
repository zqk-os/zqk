package objectidcache

import (
	stdcontext "context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"
)

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

// EntriesForID returns all cache entries for an object ID within its kind bucket.
// Allocates only for matches (usually 0–1). Prefer this over GetEntriesByKind + filter
// when checking duplicates under concurrent validation load.
func (c *ObjectIDCache) EntriesForID(id string) []*ObjectIDCacheEntry {
	if c == nil || id == emptyValue {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.byKind == nil || c.idToKind == nil {
		return nil
	}
	kind, ok := c.idToKind[id]
	if !ok {
		return nil
	}
	list := c.byKind[kind]
	var out []*ObjectIDCacheEntry
	for i := range list {
		if list[i].ID == id {
			out = append(out, c.entryFromBucket(kind, &list[i]))
		}
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
	projectRoot = resolveProjectRoot(projectRoot)
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
				c.byKind = make(map[string][]KindBucketEntry)
				c.idToKind = make(map[string]string)
				c.idToIndex = make(map[string]int)
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
	projectRoot = resolveProjectRoot(projectRoot)
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

// drainObjectIDCachePending trues object-id-cache from the storage pending journal.
// TRACK: BLI-REDACTED
func drainObjectIDCachePending(projectRoot string) {
	pending := storage.ListObjectIDCachePending(projectRoot)
	if len(pending) == 0 {
		return
	}
	// Burst drain ⇒ validation entries for those IDs may still hold Tier-1
	// reference misses; mark so the next system check refreshes object-id-cache
	// and invalidates pending validation IDs (does not wipe the whole cache).
	if len(pending) >= storage.SignificantCacheChangePendingThreshold {
		storage.NoteSignificantCacheChangeDetail(projectRoot, storage.SignificantChangeReasonPendingDrain, len(pending), 0)
	}
	cache := GetGlobalObjectIDCache()
	hasChanges := false
	for _, e := range pending {
		switch e.Op {
		case storage.ObjectIDCachePendingOpInvalidate:
			cache.Invalidate(e.ID)
			storage.ClearObjectIDCachePending(projectRoot, e.ID)
			hasChanges = true
		default:
			kind := e.Kind
			path := e.FilePath
			if live, ok := caspkg.ResolveLiveCASFilePath(projectRoot, kind, e.ID); ok {
				path = live
			}
			if path == emptyValue || caspkg.CachePathNeedsCASResolve(path) {
				cache.Invalidate(e.ID)
				storage.ClearObjectIDCachePending(projectRoot, e.ID)
				hasChanges = true
				continue
			}
			if kind == emptyValue {
				kind, _ = parseReferenceID(e.ID)
			}
			if !storage.IsHighVolumeKindForCache(kind) {
				if err := cache.Update(e.ID, kind, path); err == nil {
					storage.ClearObjectIDCachePending(projectRoot, e.ID)
					hasChanges = true
				}
			} else {
				storage.ClearObjectIDCachePending(projectRoot, e.ID)
			}
		}
	}
	if hasChanges {
		persistCacheChanges(cache, emptyValue, "Failed to save cache after pending drain")
	}
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
		emitCacheAudit("cache_invalidation", id, entry.Kind, entry.FilePath, "Cache entry invalidated due to object deletion", "low", "human")
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
	emitCacheAudit("cache_update", id, kind, filePath, operation, "low", "human")

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
		emitCacheAudit("cache_bulk_invalidation", "", kind, "", fmt.Sprintf("Bulk cache invalidation: %d entries removed for kind %s", count, kind), "medium", "human")

		persistCacheChanges(cache, emptyValue, "Failed to save cache after kind invalidation")
		if meta := cache.GetMetadata(); meta != nil && meta.ProjectRoot != emptyValue {
			storage.NoteSignificantCacheChangeDetail(meta.ProjectRoot, storage.SignificantChangeReasonKindInvalidation, 0, count)
		}
	}
}

// CleanStaleCacheEntries validates and removes all stale entries from the object ID cache
// This is useful for cleaning up the cache after bulk deletions or when cache gets out of sync
// Creates an audit event for security and compliance tracking
// Also invalidates the list cache so it stays in sync with the ID cache.
// NOTE: This function only removes stale cache entries. To also update hash registries,
// use PerformCacheFreshnessCheck instead, which calls this AND updates hash registries.
func CleanStaleCacheEntries(projectRoot string) int {
	projectRoot = resolveProjectRoot(projectRoot)
	cache := GetGlobalObjectIDCache()

	// Validate and clean stale entries
	staleCount := cache.ValidateAndCleanStale()

	if staleCount > 0 {
		storage.InvalidateListCache()
	}

	if staleCount > 0 {
		emitCacheAudit("cache_cleanup", "", "", "", fmt.Sprintf("Cleaned %d stale cache entries", staleCount), "low", "human")
		persistCacheChanges(cache, projectRoot, "Failed to save cache after cleanup")
	}

	return staleCount
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
		emitCacheAudit("cache_bulk_invalidation", "", "", "", fmt.Sprintf("Bulk cache invalidation: %d entries removed", count), "medium", "human")

		persistCacheChanges(cache, projectRoot, "Failed to save cache after bulk invalidation")
	}

	return count
}

// Machine cache on the mutation path: do not pretty-print or rewrite the whole file
// per Invalidate. Memory is updated immediately; disk coalesces.
// TRACK: sample to-investigate/high-cpu-samp.txt — MarshalIndent + SaveCache on every
// CascadeOnObjectChange delete (retention max-count) dominated CPU.
var (
	cachePersistDebounce = 200 * time.Millisecond
	cachePersistMaxDelay = 2 * time.Second
)

var (
	persistMu      sync.Mutex
	persistTimer   *time.Timer
	persistDirty   atomic.Bool
	persistRoot    atomic.Value // string
	persistFirstAt atomic.Int64 // unix nano of first dirty in this burst
	persistWarnMsg atomic.Value // string
)

// persistCacheChanges marks the on-disk object-id cache dirty. Invalidate/Update
// already mutated memory; disk write waits for a quiet window so bulk delete
// does not json.Marshal the full map once per object.
func persistCacheChanges(cache *ObjectIDCache, explicitProjectRoot, contextMsg string) {
	root := explicitProjectRoot
	if root == emptyValue {
		if meta := cache.GetMetadata(); meta != nil {
			root = meta.ProjectRoot
		}
	}
	if root == emptyValue {
		return
	}
	persistDirty.Store(true)
	persistRoot.Store(root)
	if contextMsg != emptyValue {
		persistWarnMsg.Store(contextMsg)
	}
	now := time.Now()
	persistMu.Lock()
	first := persistFirstAt.Load()
	if first == 0 {
		first = now.UnixNano()
		persistFirstAt.Store(first)
	}
	elapsed := now.Sub(time.Unix(0, first))
	if elapsed >= cachePersistMaxDelay {
		persistMu.Unlock()
		flushPendingObjectIDCachePersist()
		return
	}
	wait := cachePersistDebounce
	if remain := cachePersistMaxDelay - elapsed; remain < wait {
		wait = remain
	}
	if persistTimer == nil {
		persistTimer = time.AfterFunc(wait, flushPendingObjectIDCachePersist)
	} else {
		persistTimer.Reset(wait)
	}
	persistMu.Unlock()
}

// FlushPendingObjectIDCachePersist writes a dirty object-id cache to disk now.
func FlushPendingObjectIDCachePersist() {
	flushPendingObjectIDCachePersist()
}

func flushPendingObjectIDCachePersist() {
	for {
		persistMu.Lock()
		if persistTimer != nil {
			persistTimer.Stop()
			persistTimer = nil
		}
		persistFirstAt.Store(0)
		persistMu.Unlock()
		if !persistDirty.Swap(false) {
			return
		}
		root, _ := persistRoot.Load().(string)
		if root == emptyValue {
			return
		}
		cache := GetGlobalObjectIDCache()
		if err := cache.SaveCache(root); err != nil {
			persistDirty.Store(true)
			l := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			msg := "Failed to save object ID cache"
			if v := persistWarnMsg.Load(); v != nil {
				if s, ok := v.(string); ok && s != emptyValue {
					msg = s
				}
			}
			logging.Fluent(l).Warn(msg).WithError(err).Log()
			return
		}
	}
}
