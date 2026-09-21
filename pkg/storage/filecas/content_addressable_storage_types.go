package filecas

import (
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	errReadCASIndexFmt  = "failed to read index: %w"
	errParseCASIndexFmt = "failed to parse index: %w"
	emptyIDListSize     = 0
	casNegativeMissTTL  = 10 * time.Second
)

// OrphanCleanupCallback is called after a successful CAS update to clean up the old hash file.
// It receives the old hash file path and should delete it if no other IDs reference that hash.
// This is called only after the index update has successfully persisted.
type OrphanCleanupCallback func(oldHashFilePath string) error

// PostSyncCallback is called during live identity commit after the durable
// id→hash index mapping and exactly-one-blob sweep. FileObjectStorage requires
// this hook; raw CAS tests may omit it.
type PostSyncCallback func(objectID, kind, hash, filePath string) error

// ContentAddressableStorage implements Git-style content-addressable storage
// Files are stored with hash as filename, with an ID index for lookups
type ContentAddressableStorage struct {
	kindDir               string
	kind                  string
	index                 *IDIndex
	Mu                    sync.RWMutex
	orphanCleanupCB       OrphanCleanupCallback
	postSyncCB            PostSyncCallback
	identityCacheRequired bool
	opCallback            concurrency.OperationCallback
	// writeQueue is the queue used for index updates; nil means use global singleton
	writeQueue IndexWriteQueue

	// negativeMisses caches object IDs confirmed absent to avoid repeat directory scans
	negativeMisses sync.Map
	// lastScanTime records unix nano timestamp of the last disk scan
	lastScanTime atomic.Int64
	// indexPopulated records whether EnsureIndexPopulated has completed for this instance
	indexPopulated atomic.Bool
	// scanGroup coalesces concurrent scan and population requests
	scanGroup singleflight.Group
}

// contentAddressableStorageOrIndexMissing reports whether get-CAS failed, cas is nil, or the ID index is nil.
func ContentAddressableStorageOrIndexMissing(err error, cas *ContentAddressableStorage) bool {
	return err != nil || cas == nil || cas.index == nil
}

// Locking model (must stay consistent across the system):
//
// - `cas.Mu` is reserved for CAS-level mutable state (if/when CAS grows mutable fields beyond
//   `index.Mappings`). The current implementation avoids using `cas.Mu` for index access.
// - `cas.index.Mu` protects `cas.index.Mappings` (in-memory ID -> hash map).
// - The CAS index file on disk is protected cross-process via `.<kind>.index.lock`.
//
// Lock order rule:
//   If you ever need BOTH `cas.Mu` and `cas.index.Mu`, ALWAYS acquire `cas.Mu` first, then
//   `cas.index.Mu`. Never invert this order.
//
// Preferred pattern:
//   - If you only need to mutate/read `Mappings`, take only `cas.index.Mu` via helpers below.
//     This minimizes contention and reduces deadlock risk.

// IDIndex maps object IDs to their content hashes and optional bucket keys (from bucket strategy).
// BucketKeys is optional; when set, path is kindDir/bucketKey/hash.yaml instead of kindDir/hash.yaml.
// CreatedAt is optional (HIGH_VOLUME_EVENT_INDEXES.md): ID -> RFC3339 for high-volume kinds; enables OldestIDs without full scan.
type IDIndex struct {
	Version      string            `json:"version"`
	Kind         string            `json:"kind"`
	Mappings     map[string]string `json:"mappings"`                // ID -> hash
	BucketKeys   map[string]string `json:"bucket_keys,omitempty"`   // ID -> bucket key (optional)
	CreatedAt    map[string]string `json:"created_at,omitempty"`    // ID -> RFC3339 (optional; for OldestIDs)
	ErasePending map[string]string `json:"erase_pending,omitempty"` // ID -> eventID/reason (tombstone linger)
	FilePath     string
	Mu           sync.RWMutex
}

// GetIndex returns the ID index for direct access (used by auto-fix)
// This allows adding objects to the index when files exist but aren't indexed
func (cas *ContentAddressableStorage) GetIndex() *IDIndex {
	return cas.index
}

func (cas *ContentAddressableStorage) SetIndexMappingInMemory(objectID, hash string, bucketKey ...string) {
	cas.clearNegativeMiss(objectID)
	_ = concurrency.RunInLockOrLog(&cas.index.Mu, locknames.LockNameCasSetIndexMappingMemory, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
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

// HealIndexMappingAsync applies an id→hash mapping in memory and enqueues durable index
// persistence. Use on validation / integrity hot paths instead of SetMapping/Save (those
// take a cross-process file lock with a 5s budget that collides with fail-fast validation).
// TRACK: keep when: integrity never sync-saves on check.
func (cas *ContentAddressableStorage) HealIndexMappingAsync(objectID, hash string, bucketKey ...string) {
	if cas == nil || objectID == emptyValue || hash == emptyValue {
		return
	}
	bk := ""
	if len(bucketKey) > 0 {
		bk = bucketKey[0]
	}
	cas.SetIndexMappingInMemory(objectID, hash, bucketKey...)
	writeQueue := cas.getWriteQueue()
	opCallback := cas.getOperationCallback()
	if GetSkipIndexUpdateWait() {
		_, _ = writeQueue.EnqueueInternal(cas.kind, objectID, hash, bk, "", cas, opCallback, false, false)
		return
	}
	_, _ = writeQueue.EnqueueUpdateWithOperationCallback(cas.kind, objectID, hash, bk, cas, opCallback)
}

// removeIndexMappingInMemory removes an object ID from the in-memory index immediately.
// Same-process reads (e.g. GetHashForID, Read) then see the removal before the write-queue
// worker persists it. Persistence is still done by the queue so disk stays consistent.
func (cas *ContentAddressableStorage) removeIndexMappingInMemory(objectID string) {
	cas.clearNegativeMiss(objectID)
	_ = concurrency.RunInLockOrLog(&cas.index.Mu, locknames.LockNameCasRemoveIndexMappingMemory, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		delete(cas.index.Mappings, objectID)
		if cas.index.BucketKeys != nil {
			delete(cas.index.BucketKeys, objectID)
		}
		return nil
	})
}

// NewContentAddressableStorage creates a new content-addressable storage instance.
// writeQueue is optional: when provided, that queue is used for index updates; otherwise the global singleton is used.
func NewContentAddressableStorage(kindDir, kind string, writeQueue ...IndexWriteQueue) *ContentAddressableStorage {
	indexPath := filepath.Join(kindDir, fmt.Sprintf(".%s.index", kind))
	index := &IDIndex{
		Version:  CASIndexFormatVersion,
		Kind:     kind,
		Mappings: make(map[string]string),
		FilePath: indexPath,
	}

	// Load existing index if it exists (acquire lock, then load)
	_ = concurrency.RunInLockOrLog(&index.Mu, locknames.LockNameListingIndexInitialLoad, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		return index.LoadLocked() //nolint:errcheck // Use empty index if load fails
	})

	var q IndexWriteQueue
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

// NewContentAddressableStorageWithIndex wires a pre-built ID index (tests and recovery
// fixtures that need a non-default FilePath).
func NewContentAddressableStorageWithIndex(idx *IDIndex) *ContentAddressableStorage {
	return &ContentAddressableStorage{index: idx}
}

// getWriteQueue returns the queue to use for index updates (cas.writeQueue or global singleton).
func (cas *ContentAddressableStorage) getWriteQueue() IndexWriteQueue {
	if cas.writeQueue != nil {
		return cas.writeQueue
	}
	return GlobalIndexWriteQueue
}

// GetWriteQueue returns the write queue used by this CAS (exported for callers that need to
// enqueue index updates on the same queue as the CAS, e.g. auto-fix in tests with per-project queues).
func (cas *ContentAddressableStorage) GetWriteQueue() IndexWriteQueue {
	return cas.getWriteQueue()
}

// SetOrphanCleanupCallback sets the callback function to be called after successful updates
// to clean up orphaned hash files. The callback receives the old hash file path and should
// delete it if no other IDs reference that hash.
func (cas *ContentAddressableStorage) SetOrphanCleanupCallback(callback OrphanCleanupCallback) {
	_ = concurrency.RunInLockOrLog(&cas.Mu, locknames.LockNameCasSetOrphanCleanupCallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		cas.orphanCleanupCB = callback
		return nil
	})
}

// SetPostSyncCallback sets a callback fired after the live identity commit
// (durable index + exactly one blob). Raw CAS tests may omit the callback.
func (cas *ContentAddressableStorage) SetPostSyncCallback(callback PostSyncCallback) {
	cas.setPostSyncCallback(callback, false)
}

// SetRequiredPostSyncCallback is the FileObjectStorage path: a missing or
// failing callback aborts Create/Update so ACK cannot outrun object-id-cache.
// TRACK: TDE-CEF-CAS-IDENTITY-TXN-001
func (cas *ContentAddressableStorage) SetRequiredPostSyncCallback(callback PostSyncCallback) {
	cas.setPostSyncCallback(callback, true)
}

func (cas *ContentAddressableStorage) setPostSyncCallback(callback PostSyncCallback, required bool) {
	_ = concurrency.RunInLockOrLog(&cas.Mu, locknames.LockNameCasSetPostSyncCallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		cas.postSyncCB = callback
		cas.identityCacheRequired = required
		return nil
	})
}

// invokeCASPostSync couples object-id-cache after blob + index are durable.
// FileObjectStorage sets identityCacheRequired: nil callback or handler error
// fails the mutation. Raw CAS tests without a callback still ACK.
// Always notes the pending journal before the handler (never for draft paths).
// TRACK: TDE-CEF-CAS-IDENTITY-TXN-001
func (cas *ContentAddressableStorage) invokeCASPostSync(objectID, hash, filePath string) error {
	if objectID == emptyValue || hash == emptyValue || filePath == emptyValue {
		return errfmt.Errorf("CAS identity post-sync requires object id, hash, and path")
	}
	projectRoot := projectRootFromCASKindDir(cas.kindDir)
	NoteObjectIDCachePending(projectRoot, string(ObjectIDCachePendingOpUpdate), objectID, cas.kind, filePath, "cas_post_sync")

	var postSyncCB PostSyncCallback
	var required bool
	_ = concurrency.RunInRLockOrLog(&cas.Mu, locknames.LockNameCasGetPostSyncCallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		postSyncCB = cas.postSyncCB
		required = cas.identityCacheRequired
		return nil
	})
	if postSyncCB == nil {
		if required {
			return errfmt.Errorf(ConstMiscIdentityCachePostSyncRequired)
		}
		WarnOnceNilCacheHandler(objectID, cas.kind)
		return nil
	}
	if err := postSyncCB(objectID, cas.kind, hash, filePath); err != nil {
		StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Warn(LogEventStorageObjectCASRegisterCachePostSyncFailedWarn).
			ObjectID(objectID).
			Kind(cas.kind).
			String("file_path", filePath).
			WithError(err).
			Log()
		return errfmt.Newf(ConstMiscIdentityCachePostSyncFailed).Wrap(err)
	}
	return nil
}

// commitLiveIdentity is the Create/Update ACK gate: one live blob + cache couple.
// Call only after the durable id→hash index mapping succeeded.
func (cas *ContentAddressableStorage) commitLiveIdentity(objectID, hash, filePath string) error {
	if err := cas.ensureExactlyOneLiveBlob(objectID, hash); err != nil {
		return errfmt.Newf(ConstMiscFailedToCommitLiveIdentity).Wrap(err)
	}
	if err := cas.invokeCASPostSync(objectID, hash, filePath); err != nil {
		return errfmt.Newf(ConstMiscFailedToCommitLiveIdentity).Wrap(err)
	}
	return nil
}

// SetOperationCallback sets the operation callback for CAS operations
func (cas *ContentAddressableStorage) SetOperationCallback(callback concurrency.OperationCallback) {
	_ = concurrency.RunInLockOrLog(&cas.Mu, locknames.LockNameCasSetOperationCallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		cas.opCallback = callback
		return nil
	})
}

func (cas *ContentAddressableStorage) getOperationCallback() concurrency.OperationCallback {
	var callback concurrency.OperationCallback
	_ = concurrency.RunInRLockOrLog(&cas.Mu, locknames.LockNameCasGetOperationCallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		callback = cas.opCallback
		return nil
	})
	return callback
}

// Len returns the number of mappings currently in the index.
// Safe for concurrent use.
func (idx *IDIndex) Len() int {
	if idx == nil {
		return 0
	}
	var count int
	_ = concurrency.RunInRLockWithLogger(&idx.Mu, locknames.LockNameListingIndexListIds, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		count = len(idx.Mappings)
		return nil
	})
	return count
}

// ListIDs returns a snapshot of all object IDs in the index.
// Safe for concurrent use.
func (idx *IDIndex) ListIDs() []string {
	var ids []string
	err := concurrency.RunInRLockWithLogger(&idx.Mu, locknames.LockNameListingIndexListIds, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
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
	err := concurrency.RunInRLockWithLogger(&idx.Mu, locknames.LockNameListingIndexOldestIds, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
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
// Safe for concurrent use. Loads from disk when the in-memory map is empty so
// system-check warm can delta against a populated index without Stat storms.
func (idx *IDIndex) SnapshotMappings() map[string]string {
	_ = concurrency.RunInLockWithLogger(&idx.Mu, locknames.LockNameListingIndexSnapshotMappings, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if len(idx.Mappings) == 0 {
			_ = idx.LoadLocked() //nolint:errcheck // best-effort
		}
		return nil
	})
	var out map[string]string
	err := concurrency.RunInRLockWithLogger(&idx.Mu, locknames.LockNameListingIndexSnapshotMappings, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
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

// SnapshotBucketKeys returns a copy of ID → bucket key mappings (may be nil/empty).
func (idx *IDIndex) SnapshotBucketKeys() map[string]string {
	var out map[string]string
	_ = concurrency.RunInRLockWithLogger(&idx.Mu, locknames.LockNameListingIndexGetBucketKey, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if len(idx.BucketKeys) == 0 {
			return nil
		}
		out = make(map[string]string, len(idx.BucketKeys))
		maps.Copy(out, idx.BucketKeys)
		return nil
	})
	return out
}

// loadLocked loads the index from disk (must be called with lock held)
func (idx *IDIndex) LoadLocked() error {
	data, err := fileutil.ReadFile(idx.FilePath)
	if fileutil.IsNotExist(err) {
		// No index yet
		if idx.Mappings == nil {
			idx.Mappings = make(map[string]string)
		}
		return nil
	}
	if err != nil {
		logging.FluentEvent(logging.GetLogger()).Error("loadLocked failed", err).String("filePath", idx.FilePath).Log()
		return errfmt.Errorf(errReadCASIndexFmt, err)
	}

	var loaded IDIndex
	if err := json.Unmarshal(data, &loaded); err != nil {
		logging.FluentEvent(logging.GetLogger()).Error("loadLocked Unmarshal failed", err).String("filePath", idx.FilePath).Log()
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

	// ErasePending (optional; tombstone linger until shockwave FINALIZE)
	if len(loaded.ErasePending) > 0 {
		newErasePending := make(map[string]string, len(loaded.ErasePending))
		maps.Copy(newErasePending, loaded.ErasePending)
		idx.ErasePending = newErasePending
	} else {
		idx.ErasePending = nil
	}

	// Pending fills locator gaps for cross-process RYW (ADR-CAS-PENDING). Never let a
	// stale pending hash shadow a different durable mapping — that yields get→not-found
	// while the CAS blob for the durable hash still exists.
	pendingCache := GetCASPendingVisibilityCache(projectRootFromCASIndexPath(idx.FilePath))
	if pendingCache != nil {
		for _, pendingEntry := range pendingCache.SnapshotPending() {
			if pendingEntry.Kind != "" && idx.Kind != pendingEntry.Kind {
				continue
			}
			if pendingEntry.ObjectID == emptyValue || pendingEntry.Hash == emptyValue {
				continue
			}
			if durableHash, ok := idx.Mappings[pendingEntry.ObjectID]; ok && durableHash != emptyValue {
				if durableHash != pendingEntry.Hash {
					continue // stale or superseded pending; durable wins
				}
				continue // same hash already durable; eviction is SetMapping's job
			}
			idx.Mappings[pendingEntry.ObjectID] = pendingEntry.Hash
			if pendingEntry.BucketKey != "" {
				if idx.BucketKeys == nil {
					idx.BucketKeys = make(map[string]string)
				}
				idx.BucketKeys[pendingEntry.ObjectID] = pendingEntry.BucketKey
			}
		}
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

// IsErasePending reports whether an object is currently in the erase_pending linger state.
func (idx *IDIndex) IsErasePending(objectID string) bool {
	if idx == nil || objectID == emptyValue {
		return false
	}
	var pending bool
	_ = concurrency.RunInRLockWithLogger(&idx.Mu, locknames.LockNameListingIndexListIds, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if idx.ErasePending != nil {
			_, pending = idx.ErasePending[objectID]
		}
		return nil
	})
	return pending
}

// GetErasePendingEventID returns the event/reason associated with an erase_pending object ID.
func (idx *IDIndex) GetErasePendingEventID(objectID string) (string, bool) {
	if idx == nil || objectID == emptyValue {
		return "", false
	}
	var evt string
	var ok bool
	_ = concurrency.RunInRLockWithLogger(&idx.Mu, locknames.LockNameListingIndexListIds, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if idx.ErasePending != nil {
			evt, ok = idx.ErasePending[objectID]
		}
		return nil
	})
	return evt, ok
}

// SetErasePending sets an object ID in the erase_pending linger map.
func (idx *IDIndex) SetErasePending(objectID, eventID string) {
	if idx == nil || objectID == emptyValue {
		return
	}
	_ = concurrency.RunInLockWithLogger(&idx.Mu, locknames.LockNameListingIndexListIds, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if idx.ErasePending == nil {
			idx.ErasePending = make(map[string]string)
		}
		if eventID == emptyValue {
			eventID = "erase_pending"
		}
		idx.ErasePending[objectID] = eventID
		return nil
	})
}

// ClearErasePending removes an object ID from the erase_pending linger map.
func (idx *IDIndex) ClearErasePending(objectID string) {
	if idx == nil || objectID == emptyValue {
		return
	}
	_ = concurrency.RunInLockWithLogger(&idx.Mu, locknames.LockNameListingIndexListIds, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if idx.ErasePending != nil {
			delete(idx.ErasePending, objectID)
		}
		return nil
	})
}

// SnapshotErasePending returns a copy of ID -> eventID erase_pending mappings.
func (idx *IDIndex) SnapshotErasePending() map[string]string {
	if idx == nil {
		return nil
	}
	var out map[string]string
	_ = concurrency.RunInRLockWithLogger(&idx.Mu, locknames.LockNameListingIndexListIds, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if len(idx.ErasePending) == 0 {
			return nil
		}
		out = make(map[string]string, len(idx.ErasePending))
		maps.Copy(out, idx.ErasePending)
		return nil
	})
	return out
}

// IsErasePending returns true if objectID is in erase_pending linger state.
func (cas *ContentAddressableStorage) IsErasePending(objectID string) bool {
	if cas == nil || cas.index == nil {
		return false
	}
	return cas.index.IsErasePending(objectID)
}

// GetErasePendingEventID returns the event ID/reason if objectID is in erase_pending linger state.
func (cas *ContentAddressableStorage) GetErasePendingEventID(objectID string) (string, bool) {
	if cas == nil || cas.index == nil {
		return "", false
	}
	return cas.index.GetErasePendingEventID(objectID)
}

// SetErasePending marks an object ID as erase_pending in the CAS index.
func (cas *ContentAddressableStorage) SetErasePending(objectID, eventID string) {
	if cas == nil || cas.index == nil {
		return
	}
	cas.index.SetErasePending(objectID, eventID)
}

// ClearErasePending clears erase_pending status for an object ID in the CAS index.
func (cas *ContentAddressableStorage) ClearErasePending(objectID string) {
	if cas == nil || cas.index == nil {
		return
	}
	cas.index.ClearErasePending(objectID)
}

// SnapshotErasePending returns all active erase_pending IDs in the CAS index.
func (cas *ContentAddressableStorage) SnapshotErasePending() map[string]string {
	if cas == nil || cas.index == nil {
		return nil
	}
	return cas.index.SnapshotErasePending()
}
func (cas *ContentAddressableStorage) GetKindDir() string {
	return cas.kindDir
}

func (cas *ContentAddressableStorage) GetKind() string {
	if cas == nil {
		return ""
	}
	return cas.kind
}

func (cas *ContentAddressableStorage) RemoveIndexMappingInMemory(objectID string) {
	if cas == nil {
		return
	}
	cas.removeIndexMappingInMemory(objectID)
}

func (cas *ContentAddressableStorage) isNegativeMiss(objectID string) bool {
	if objectID == emptyValue {
		return true
	}
	val, ok := cas.negativeMisses.Load(objectID)
	if !ok {
		return false
	}
	t, ok := val.(time.Time)
	if !ok {
		return false
	}
	if time.Since(t) > casNegativeMissTTL {
		cas.negativeMisses.Delete(objectID)
		return false
	}
	return true
}

func (cas *ContentAddressableStorage) recordNegativeMiss(objectID string) {
	if objectID == emptyValue {
		return
	}
	cas.negativeMisses.Store(objectID, time.Now())
}

func (cas *ContentAddressableStorage) clearNegativeMiss(objectID string) {
	if objectID == emptyValue {
		return
	}
	cas.negativeMisses.Delete(objectID)
}

// EnsureIndexPopulated ensures the CAS index is loaded into memory, loading from disk
// or scanning kindDir in bulk if the index is completely unpopulated.
func (cas *ContentAddressableStorage) EnsureIndexPopulated() error {
	if cas == nil || cas.index == nil {
		return nil
	}
	if cas.indexPopulated.Load() || cas.index.Len() > 0 {
		return nil
	}
	_, err, _ := cas.scanGroup.Do("ensure_index_populated", func() (any, error) {
		if cas.indexPopulated.Load() || cas.index.Len() > 0 {
			return nil, nil
		}
		// First try loading existing disk index
		_ = concurrency.RunInLockOrLog(&cas.index.Mu, locknames.LockNameListingIndexInitialLoad, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
			return cas.index.LoadLocked()
		})
		if cas.index.Len() > 0 {
			cas.indexPopulated.Store(true)
			return nil, nil
		}
		// If still 0, scan kindDir once to populate all existing objects into the index
		err := cas.populateIndexFromDirScan()
		cas.indexPopulated.Store(true)
		return nil, err
	})
	return err
}

func (cas *ContentAddressableStorage) populateIndexFromDirScan() error {
	if cas == nil || cas.kindDir == emptyValue {
		return nil
	}
	if _, err := fileutil.Stat(cas.kindDir); err != nil {
		return nil
	}

	type scanItem struct {
		hash      string
		mtime     int64
		bucketKey string
	}
	best := make(map[string]scanItem)

	scanDir := func(dir, bucketKey string) {
		entries, err := fileutil.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !CasHashFilenameRe.MatchString(name) {
				continue
			}
			path := filepath.Join(dir, name)
			id := CasHashFilePeekObjectID(path)
			if id == emptyValue {
				continue
			}
			stem := strings.TrimSuffix(name, filepath.Ext(name))
			var mtime int64
			if fi, statErr := fileutil.Stat(path); statErr == nil {
				mtime = fi.ModTime().UnixNano()
			}
			prev, ok := best[id]
			better := !ok || mtime > prev.mtime || (mtime == prev.mtime && stem > prev.hash)
			if better {
				best[id] = scanItem{hash: stem, mtime: mtime, bucketKey: bucketKey}
			}
		}
	}

	scanDir(cas.kindDir, "")
	entries, err := fileutil.ReadDir(cas.kindDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				scanDir(filepath.Join(cas.kindDir, e.Name()), e.Name())
			}
		}
	}

	if len(best) == 0 {
		return nil
	}

	mappings := make(map[string]string, len(best))
	bucketKeys := make(map[string]string)
	for id, item := range best {
		mappings[id] = item.hash
		cas.SetIndexMappingInMemory(id, item.hash, item.bucketKey)
		if item.bucketKey != emptyValue {
			bucketKeys[id] = item.bucketKey
		}
	}

	return cas.index.SetMappings(mappings, bucketKeys)
}
