package storage

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sync"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage/locknames"
)

const (
	errReadCASIndexFmt  = "failed to read index: %w"
	errParseCASIndexFmt = "failed to parse index: %w"
	emptyIDListSize     = 0
)

// OrphanCleanupCallback is called after a successful CAS update to clean up the old hash file.
// It receives the old hash file path and should delete it if no other IDs reference that hash.
// This is called only after the index update has successfully persisted.
type OrphanCleanupCallback func(oldHashFilePath string) error

// PostSyncCallback is called after a successful CAS file sync (after writeFileWithSync succeeds).
// It receives the object ID, kind, hash, and the full file path (including hash-based filename).
// This is useful for cache updates and other operations that need to know the actual file path.
// Called immediately after file sync succeeds, before index update (which may be async).
type PostSyncCallback func(objectID, kind, hash, filePath string) error

// ContentAddressableStorage implements Git-style content-addressable storage
// Files are stored with hash as filename, with an ID index for lookups
type ContentAddressableStorage struct {
	kindDir         string
	kind            string
	index           *IDIndex
	mu              sync.RWMutex
	orphanCleanupCB OrphanCleanupCallback
	postSyncCB      PostSyncCallback
	opCallback      concurrency.OperationCallback
	// writeQueue is the queue used for index updates; nil means use global singleton
	writeQueue *ListingIndexWriteQueue
}

// contentAddressableStorageOrIndexMissing reports whether get-CAS failed, cas is nil, or the ID index is nil.
func contentAddressableStorageOrIndexMissing(err error, cas *ContentAddressableStorage) bool {
	return err != nil || cas == nil || cas.index == nil
}

// Locking model (must stay consistent across the system):
//
// - `cas.mu` is reserved for CAS-level mutable state (if/when CAS grows mutable fields beyond
//   `index.Mappings`). The current implementation avoids using `cas.mu` for index access.
// - `cas.index.mu` protects `cas.index.Mappings` (in-memory ID -> hash map).
// - The CAS index file on disk is protected cross-process via `.<kind>.index.lock`.
//
// Lock order rule:
//   If you ever need BOTH `cas.mu` and `cas.index.mu`, ALWAYS acquire `cas.mu` first, then
//   `cas.index.mu`. Never invert this order.
//
// Preferred pattern:
//   - If you only need to mutate/read `Mappings`, take only `cas.index.mu` via helpers below.
//     This minimizes contention and reduces deadlock risk.

// IDIndex maps object IDs to their content hashes and optional bucket keys (from bucket strategy).
// BucketKeys is optional; when set, path is kindDir/bucketKey/hash.yaml instead of kindDir/hash.yaml.
// CreatedAt is optional (HIGH_VOLUME_EVENT_INDEXES.md): ID -> RFC3339 for high-volume kinds; enables OldestIDs without full scan.
type IDIndex struct {
	Version    string            `json:"version"`
	Kind       string            `json:"kind"`
	Mappings   map[string]string `json:"mappings"`              // ID -> hash
	BucketKeys map[string]string `json:"bucket_keys,omitempty"` // ID -> bucket key (optional)
	CreatedAt  map[string]string `json:"created_at,omitempty"`  // ID -> RFC3339 (optional; for OldestIDs)
	filePath   string
	mu         sync.RWMutex
}

// GetIndex returns the ID index for direct access (used by auto-fix)
// This allows adding objects to the index when files exist but aren't indexed
func (cas *ContentAddressableStorage) GetIndex() *IDIndex {
	return cas.index
}

func (cas *ContentAddressableStorage) setIndexMappingInMemory(objectID, hash string, bucketKey ...string) {
	_ = concurrency.RunInLockOrLog(&cas.index.mu, locknames.LockNameCasSetIndexMappingMemory, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if cas.index.Mappings == nil {
			cas.index.Mappings = make(map[string]string)
		}
		cas.index.Mappings[objectID] = hash
		if len(bucketKey) > 0 && bucketKey[0] != emptyValue {
			if cas.index.BucketKeys == nil {
				cas.index.BucketKeys = make(map[string]string)
			}
			cas.index.BucketKeys[objectID] = bucketKey[0]
		}
		return nil
	})
}

// removeIndexMappingInMemory removes an object ID from the in-memory index immediately.
// Same-process reads (e.g. GetHashForID, Read) then see the removal before the write-queue
// worker persists it. Persistence is still done by the queue so disk stays consistent.
func (cas *ContentAddressableStorage) removeIndexMappingInMemory(objectID string) {
	_ = concurrency.RunInLockOrLog(&cas.index.mu, locknames.LockNameCasRemoveIndexMappingMemory, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		delete(cas.index.Mappings, objectID)
		if cas.index.BucketKeys != nil {
			delete(cas.index.BucketKeys, objectID)
		}
		return nil
	})
}

// NewContentAddressableStorage creates a new content-addressable storage instance.
// writeQueue is optional: when provided, that queue is used for index updates; otherwise the global singleton is used.
func NewContentAddressableStorage(kindDir, kind string, writeQueue ...*ListingIndexWriteQueue) *ContentAddressableStorage {
	indexPath := filepath.Join(kindDir, fmt.Sprintf(".%s.index", kind))
	index := &IDIndex{
		Version:  CASIndexFormatVersion,
		Kind:     kind,
		Mappings: make(map[string]string),
		filePath: indexPath,
	}

	// Load existing index if it exists (acquire lock, then load)
	_ = concurrency.RunInLockOrLog(&index.mu, locknames.LockNameListingIndexInitialLoad, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		return index.loadLocked() //nolint:errcheck // Use empty index if load fails
	})

	var q *ListingIndexWriteQueue
	if len(writeQueue) > 0 {
		q = writeQueue[0]
	}
	return &ContentAddressableStorage{
		kindDir:         kindDir,
		kind:            kind,
		index:           index,
		orphanCleanupCB: nil,
		writeQueue:      q,
	}
}

// getWriteQueue returns the queue to use for index updates (cas.writeQueue or global singleton).
func (cas *ContentAddressableStorage) getWriteQueue() *ListingIndexWriteQueue {
	if cas.writeQueue != nil {
		return cas.writeQueue
	}
	return GetGlobalListingIndexWriteQueue()
}

// GetWriteQueue returns the write queue used by this CAS (exported for callers that need to
// enqueue index updates on the same queue as the CAS, e.g. auto-fix in tests with per-project queues).
func (cas *ContentAddressableStorage) GetWriteQueue() *ListingIndexWriteQueue {
	return cas.getWriteQueue()
}

// SetOrphanCleanupCallback sets the callback function to be called after successful updates
// to clean up orphaned hash files. The callback receives the old hash file path and should
// delete it if no other IDs reference that hash.
func (cas *ContentAddressableStorage) SetOrphanCleanupCallback(callback OrphanCleanupCallback) {
	_ = concurrency.RunInLockOrLog(&cas.mu, locknames.LockNameCasSetOrphanCleanupCallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		cas.orphanCleanupCB = callback
		return nil
	})
}

// SetPostSyncCallback sets a callback function that will be called after a successful CAS file sync.
// The callback receives objectID, kind, hash, and the full file path (hash-based filename).
// This is called immediately after writeFileWithSync succeeds, before index update.
// Useful for cache updates and other operations that need the actual file path.
func (cas *ContentAddressableStorage) SetPostSyncCallback(callback PostSyncCallback) {
	_ = concurrency.RunInLockOrLog(&cas.mu, locknames.LockNameCasSetPostSyncCallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		cas.postSyncCB = callback
		return nil
	})
}

// SetOperationCallback sets the operation callback for CAS operations
func (cas *ContentAddressableStorage) SetOperationCallback(callback concurrency.OperationCallback) {
	_ = concurrency.RunInLockOrLog(&cas.mu, locknames.LockNameCasSetOperationCallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		cas.opCallback = callback
		return nil
	})
}

func (cas *ContentAddressableStorage) getOperationCallback() concurrency.OperationCallback {
	var callback concurrency.OperationCallback
	_ = concurrency.RunInRLockOrLog(&cas.mu, locknames.LockNameCasGetOperationCallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		callback = cas.opCallback
		return nil
	})
	return callback
}

// ListIDs returns a snapshot of all object IDs in the index.
// Safe for concurrent use.
func (idx *IDIndex) ListIDs() []string {
	var ids []string
	err := concurrency.RunInRLockWithLogger(&idx.mu, locknames.LockNameListingIndexListIds, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		// CRITICAL: Copy keys to slice while holding read lock to prevent
		// "concurrent map iteration and map write" panic.
		if idx.Mappings == nil {
			ids = make([]string, emptyIDListSize)
			return nil
		}
		ids = make([]string, 0, len(idx.Mappings))
		for id := range idx.Mappings {
			ids = append(ids, id)
		}
		return nil
	})
	if err != nil {
		StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Warn(LogEventStorageCASIndexReadListFailedEmptyWarn).
			WithError(err).
			Log()
		return make([]string, emptyIDListSize)
	}
	return ids
}

type oldestIDItem struct {
	id        string
	createdAt string
}

type oldestMaxHeap struct {
	data []oldestIDItem
}

func (h *oldestMaxHeap) swap(i, j int) {
	h.data[i], h.data[j] = h.data[j], h.data[i]
}

func (h *oldestMaxHeap) less(i, j int) bool {
	// Max-heap: root is the newest (lexicographically largest string)
	return h.data[i].createdAt < h.data[j].createdAt
}

func (h *oldestMaxHeap) up(j int) {
	for {
		i := (j - 1) / 2
		if i == j || !h.less(i, j) {
			break
		}
		h.swap(i, j)
		j = i
	}
}

func (h *oldestMaxHeap) down(i0, n int) bool {
	i := i0
	for {
		left := 2*i + 1
		if left >= n || left < 0 {
			break
		}
		j := left
		if right := left + 1; right < n && h.less(left, right) {
			j = right
		}
		if !h.less(i, j) {
			break
		}
		h.swap(i, j)
		i = j
	}
	return i > i0
}

func (h *oldestMaxHeap) Push(item oldestIDItem) {
	h.data = append(h.data, item)
	h.up(len(h.data) - 1)
}

func (h *oldestMaxHeap) Pop() oldestIDItem {
	n := len(h.data) - 1
	h.swap(0, n)
	h.down(0, n)
	item := h.data[n]
	h.data = h.data[:n]
	return item
}

// OldestIDs returns up to limit object IDs with smallest created_at (for retention max_count).
// Returns nil if CreatedAt is not populated (caller should use high-volume cache). Safe for concurrent use.
func (idx *IDIndex) OldestIDs(limit int) []string {
	if limit <= 0 {
		return nil
	}
	var heap oldestMaxHeap
	err := concurrency.RunInRLockWithLogger(&idx.mu, locknames.LockNameListingIndexOldestIds, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if len(idx.CreatedAt) == 0 {
			return nil
		}
		// Pre-allocate heap slice to avoid dynamic growth allocations.
		heap.data = make([]oldestIDItem, 0, limit)
		for id, s := range idx.CreatedAt {
			// Skip malformed/empty created_at fields to match time.Parse error filtering.
			if len(s) < 19 {
				continue
			}
			item := oldestIDItem{id: id, createdAt: s}
			if len(heap.data) < limit {
				heap.Push(item)
			} else if s < heap.data[0].createdAt {
				// s is older than the newest item currently in the heap, so replace root
				heap.data[0] = item
				heap.down(0, len(heap.data))
			}
		}
		return nil
	})
	if err != nil || len(heap.data) == 0 {
		return nil
	}

	// Pop elements from heap (newest to oldest) and fill output slice from end to beginning.
	n := len(heap.data)
	out := make([]string, n)
	for i := n - 1; i >= 0; i-- {
		out[i] = heap.Pop().id
	}
	return out
}

// SnapshotMappings returns a snapshot copy of ID -> hash mappings.
// Safe for concurrent use.
func (idx *IDIndex) SnapshotMappings() map[string]string {
	var out map[string]string
	err := concurrency.RunInRLockWithLogger(&idx.mu, locknames.LockNameListingIndexSnapshotMappings, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		out = make(map[string]string, len(idx.Mappings))
		maps.Copy(out, idx.Mappings)
		return nil
	})
	if err != nil {
		// Timeout or error - return empty map to avoid blocking caller
		StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Warn(LogEventStorageCASIndexReadMappingsTimeoutEmptyWarn).
			WithError(err).
			Log()
		return make(map[string]string)
	}
	return out
}

// loadLocked loads the index from disk (must be called with lock held)
func (idx *IDIndex) loadLocked() error {
	data, err := os.ReadFile(idx.filePath)
	if os.IsNotExist(err) {
		// No index yet
		if idx.Mappings == nil {
			idx.Mappings = make(map[string]string)
		}
		return nil
	}
	if err != nil {
		logging.FluentEvent(logging.GetLogger()).Error("loadLocked failed", err).String("filePath", idx.filePath).Log()
		return errfmt.Errorf(errReadCASIndexFmt, err)
	}

	var loaded IDIndex
	if err := json.Unmarshal(data, &loaded); err != nil {
		logging.FluentEvent(logging.GetLogger()).Error("loadLocked Unmarshal failed", err).String("filePath", idx.filePath).Log()
		return errfmt.Errorf(errParseCASIndexFmt, err)
	}

	// CRITICAL: Disk is the cross-process source of truth. Replace in-memory mappings
	// to avoid re-saving stale state and overwriting newer updates from other processes.
	// Atomically replace the entire map to prevent "concurrent map iteration and map write" panic.
	// Creating a new map and assigning it is atomic, whereas clearing and repopulating
	// can cause race conditions with concurrent readers iterating over the map.
	if loaded.Mappings != nil {
		// Create new map and populate it (atomic assignment)
		newMappings := make(map[string]string, len(loaded.Mappings))
		maps.Copy(newMappings, loaded.Mappings)
		// Atomic replacement (single assignment operation)
		idx.Mappings = newMappings
	} else {
		// No mappings in loaded data - create empty map
		idx.Mappings = make(map[string]string)
	}

	// Bucket keys (optional; from bucket strategy at create time)
	if len(loaded.BucketKeys) > 0 {
		newBucketKeys := make(map[string]string, len(loaded.BucketKeys))
		maps.Copy(newBucketKeys, loaded.BucketKeys)
		idx.BucketKeys = newBucketKeys
	} else {
		idx.BucketKeys = nil
	}

	// CreatedAt (optional; for high-volume kinds, enables OldestIDs)
	if len(loaded.CreatedAt) > 0 {
		newCreatedAt := make(map[string]string, len(loaded.CreatedAt))
		maps.Copy(newCreatedAt, loaded.CreatedAt)
		idx.CreatedAt = newCreatedAt
	} else {
		idx.CreatedAt = nil
	}

	// Update version and kind from loaded (if not already set)
	if idx.Version == emptyValue {
		idx.Version = loaded.Version
	}
	if idx.Kind == emptyValue {
		idx.Kind = loaded.Kind
	}

	return nil
}
