package cas

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage/file"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// PendingVisibilityEntry represents a pending CAS object creation/update
// visible to other processes before index persistence completes.
type PendingVisibilityEntry struct {
	ObjectID  string    `json:"object_id"`
	Kind      string    `json:"kind"`
	Hash      string    `json:"hash"`
	BucketKey string    `json:"bucket_key,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// PendingVisibilityMap represents the cross-process JSON structure stored in .zqk/cas_pending_visibility.json
type PendingVisibilityMap struct {
	Version string                            `json:"version"`
	Entries map[string]PendingVisibilityEntry `json:"entries"` // Keyed by ObjectID
}

// CASPendingVisibilityCache manages the cross-process pending visibility layer for CAS objects.
type CASPendingVisibilityCache struct {
	mu           sync.RWMutex
	filePath     string
	inMemory     map[string]PendingVisibilityEntry
	rejectWrites atomic.Bool // set during ordered shutdown (CRIT-CAS-PENDING-003)
}

var (
	globalPendingVisibilityCaches = make(map[string]*CASPendingVisibilityCache)
	// Guards lazy construction per projectRoot.
	globalPendingCacheMu sync.Mutex
)

// GetCASPendingVisibilityCache returns the process-wide CAS pending visibility cache for the given project root.
func GetCASPendingVisibilityCache(projectRoot string) *CASPendingVisibilityCache {
	if projectRoot == "" {
		projectRoot = "."
	}
	// Normalize so Publish (kindDir → root) and Evict (indexPath → root) hit the same
	// filePath even when one side is absolute and the other is "." / relative.
	if abs, err := filepath.Abs(projectRoot); err == nil {
		projectRoot = abs
	}
	targetPath := filepath.Join(projectRoot, ".zqk", "cas_pending_visibility.json")

	globalPendingCacheMu.Lock()
	defer globalPendingCacheMu.Unlock()

	c, exists := globalPendingVisibilityCaches[targetPath]
	if !exists {
		c = &CASPendingVisibilityCache{
			filePath: targetPath,
			inMemory: make(map[string]PendingVisibilityEntry),
		}
		c.loadFromDisk()
		globalPendingVisibilityCaches[targetPath] = c
	}

	return c
}

// loadFromDisk loads the pending visibility file from disk into inMemory.
func (c *CASPendingVisibilityCache) loadFromDisk() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loadFromDiskLocked()
}

func (c *CASPendingVisibilityCache) loadFromDiskLocked() {
	data, err := fileutil.ReadFile(c.filePath)
	if err != nil {
		// Keep in-memory on read error so a transient miss does not wipe local publishes.
		if fileutil.IsNotExist(err) {
			c.inMemory = make(map[string]PendingVisibilityEntry)
		}
		return
	}
	var mapData PendingVisibilityMap
	if err := json.Unmarshal(data, &mapData); err != nil {
		return
	}

	// Replace (not merge): disk is cross-process truth under the pending file lock.
	// Merge-only resurrected entries peers had already evicted.
	now := time.Now()
	next := make(map[string]PendingVisibilityEntry)
	for id, entry := range mapData.Entries {
		if now.Sub(entry.CreatedAt) < 10*time.Minute {
			next[id] = entry
		}
	}
	c.inMemory = next
}

// BeginOrderedShutdown rejects further PublishPending calls (CRIT-CAS-PENDING-003).
// Call before dumping pending mappings into the durable CAS index and draining WAL/index queues.
func (c *CASPendingVisibilityCache) BeginOrderedShutdown() {
	if c == nil {
		return
	}
	c.rejectWrites.Store(true)
}

// WritesRejected reports whether ordered shutdown has begun.
func (c *CASPendingVisibilityCache) WritesRejected() bool {
	return c != nil && c.rejectWrites.Load()
}

// SnapshotPending returns a copy of current pending entries (for shutdown dump).
func (c *CASPendingVisibilityCache) SnapshotPending() []PendingVisibilityEntry {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]PendingVisibilityEntry, 0, len(c.inMemory))
	for _, e := range c.inMemory {
		out = append(out, e)
	}
	return out
}

// withPendingFileLock serializes cross-process read-modify-write of cas_pending_visibility.json.
func (c *CASPendingVisibilityCache) withPendingFileLock(fn func() error) error {
	if c == nil {
		return fn()
	}
	lockPath := c.filePath + ".lock"
	strategy := file.NewAutoCleanupStrategy()
	handle, err := strategy.AcquireLock(lockPath, 30*time.Second)
	if err != nil {
		return errfmt.Newf("cas pending visibility file lock").Wrap(err)
	}
	defer func() {
		_ = handle.Release()
	}()
	return fn()
}

// PublishPending publishes an ID -> Hash mapping to the cross-process pending visibility layer before creation/update returns.
func (c *CASPendingVisibilityCache) PublishPending(objectID, kind, hash, bucketKey string) error {
	if c.WritesRejected() {
		return errfmt.Errorf("cas pending visibility: writes rejected during ordered shutdown")
	}
	entry := PendingVisibilityEntry{
		ObjectID:  objectID,
		Kind:      kind,
		Hash:      hash,
		BucketKey: bucketKey,
		CreatedAt: time.Now(),
	}

	// File lock outside in-process mu so we do not hold mu while waiting on peers.
	// TRACK: [REDACTED-ID] — parallel create pending RMW races.
	return c.withPendingFileLock(func() error {
		return concurrency.RunInLockWithLogger(&c.mu, locknames.LockNameListingIndexSave, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
			if c.rejectWrites.Load() {
				return errfmt.Errorf("cas pending visibility: writes rejected during ordered shutdown")
			}
			c.loadFromDiskLocked()
			c.inMemory[objectID] = entry
			return c.saveToDiskLocked()
		})
	})
}

// DumpPendingToDurableIndexes promotes pending id→hash mappings into per-kind CAS indexes,
// then evicts each successfully promoted entry. Used on ordered shutdown (CRIT-CAS-PENDING-003).
func (c *CASPendingVisibilityCache) DumpPendingToDurableIndexes(getCAS func(kind string) (*filecas.ContentAddressableStorage, error)) error {
	if c == nil {
		return nil
	}
	c.BeginOrderedShutdown()
	entries := c.SnapshotPending()
	var firstErr error
	for _, e := range entries {
		if e.ObjectID == "" || e.Hash == "" || e.Kind == "" {
			continue
		}
		if getCAS == nil {
			continue
		}
		cas, err := getCAS(e.Kind)
		if err != nil || cas == nil || cas.GetIndex() == nil {
			if firstErr == nil && err != nil {
				firstErr = err
			}
			continue
		}
		args := []string{}
		if e.BucketKey != "" {
			args = append(args, e.BucketKey)
		}
		if setErr := cas.GetIndex().SetMapping(e.ObjectID, e.Hash, args...); setErr != nil {
			if firstErr == nil {
				firstErr = setErr
			}
			continue
		}
		cas.SetIndexMappingInMemory(e.ObjectID, e.Hash, args...)
		_ = c.EvictPending(e.ObjectID) //nolint:errcheck // best-effort eviction after durable SetMapping
	}
	return firstErr
}

// EvictPending removes an ID from the pending visibility layer after durable index confirmation.
func (c *CASPendingVisibilityCache) EvictPending(objectID string) error {
	return c.withPendingFileLock(func() error {
		return concurrency.RunInLockWithLogger(&c.mu, locknames.LockNameListingIndexSave, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
			c.loadFromDiskLocked()
			if _, exists := c.inMemory[objectID]; !exists {
				return nil
			}
			delete(c.inMemory, objectID)
			return c.saveToDiskLocked()
		})
	})
}

// EvictPendingIfHash removes a pending entry only when it still advertises hash.
// Used after durable SetMapping confirms id→hash so a newer in-flight PublishPending
// (concurrent update) is not clobbered.
// TRACK: [REDACTED-ID] — pending must not shadow durable after confirm.
func (c *CASPendingVisibilityCache) EvictPendingIfHash(objectID, hash string) error {
	if c == nil || objectID == "" || hash == "" {
		return nil
	}
	return c.withPendingFileLock(func() error {
		return concurrency.RunInLockWithLogger(&c.mu, locknames.LockNameListingIndexSave, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
			// Always reload before save so eviction does not clobber peer pending entries.
			c.loadFromDiskLocked()
			entry, exists := c.inMemory[objectID]
			if !exists || entry.Hash != hash {
				return nil
			}
			delete(c.inMemory, objectID)
			return c.saveToDiskLocked()
		})
	})
}

// LookupPending checks the pending visibility cache (in-memory + re-read disk if miss) for an object ID.
func (c *CASPendingVisibilityCache) LookupPending(objectID string) (PendingVisibilityEntry, bool) {
	c.mu.RLock()
	entry, found := c.inMemory[objectID]
	c.mu.RUnlock()
	if found {
		return entry, true
	}

	// Cross-process re-read from disk (same TTL purge as loadFromDiskLocked).
	c.mu.Lock()
	defer c.mu.Unlock()

	c.loadFromDiskLocked()
	entry, found = c.inMemory[objectID]
	return entry, found
}

// ProjectRootFromCASIndexPath resolves project root from a kind index path
// ({root}/.zqk/process/<kind_dir>/.*.index). Same depth as loadLocked pending merge.
func ProjectRootFromCASIndexPath(indexPath string) string {
	if indexPath == "" {
		return "."
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(indexPath))))
}

// projectRootFromCASKindDir resolves project root from a kind directory
// ({root}/.zqk/process/<kind_dir>). Equivalent to ProjectRootFromCASIndexPath for
// indexes stored directly in kindDir.
func projectRootFromCASKindDir(kindDir string) string {
	if kindDir == "" {
		return "."
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(kindDir)))
}

// ConfirmPendingAfterDurableMapping evicts pending when the durable CAS index has
// persisted the same id→hash (ADR-CAS-PENDING-VISIBILITY-LAYER §3).
// Must read the on-disk index: EvictPendingIfHash alone only checks the pending
// entry's hash, so a save that omitted the id (async validate cache lag) still
// cleared pending and produced create→get ghosts.
// TRACK: [REDACTED-ID]
func ConfirmPendingAfterDurableMapping(indexPath, objectID, hash string) {
	if objectID == "" || hash == "" || indexPath == "" {
		return
	}
	data, err := fileutil.ReadFile(indexPath)
	if err != nil {
		return
	}
	var idx struct {
		Mappings map[string]string `json:"mappings"`
	}
	if err := json.Unmarshal(data, &idx); err != nil || idx.Mappings == nil {
		return
	}
	if got, ok := idx.Mappings[objectID]; !ok || got != hash {
		return
	}
	pendingCache := GetCASPendingVisibilityCache(ProjectRootFromCASIndexPath(indexPath))
	if pendingCache == nil {
		return
	}
	_ = pendingCache.EvictPendingIfHash(objectID, hash) //nolint:errcheck // best-effort; durable index is source of truth
}

// RestorePendingAfterFailedMutation repairs the pending layer when Create/Update
// published a new hash then rolled back before durable confirmation.
func RestorePendingAfterFailedMutation(projectRoot, objectID, kind, oldHash, oldBucketKey string) {
	pendingCache := GetCASPendingVisibilityCache(projectRoot)
	if pendingCache == nil || objectID == "" {
		return
	}
	if oldHash != "" {
		_ = pendingCache.PublishPending(objectID, kind, oldHash, oldBucketKey) //nolint:errcheck // best-effort rollback
		return
	}
	_ = pendingCache.EvictPending(objectID) //nolint:errcheck // best-effort
}

// saveToDiskLocked writes the current inMemory map to disk using atomic temp file replace.
func (c *CASPendingVisibilityCache) saveToDiskLocked() error {
	mapData := PendingVisibilityMap{
		Version: "1.0.0",
		Entries: c.inMemory,
	}
	data, err := json.Marshal(mapData)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(c.filePath)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return err
	}

	tmpFile, err := fileutil.CreateTemp(dir, filepath.Base(c.filePath)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = tmpFile.Close()
		_ = fileutil.Remove(tmpName)
	}()

	if err := fileutil.Chmod(tmpName, paths.FilePerm644); err != nil && !fileutil.IsNotExist(err) {
		return err
	}
	if _, err := tmpFile.Write(data); err != nil {
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	return fileutil.Rename(tmpName, c.filePath)
}
