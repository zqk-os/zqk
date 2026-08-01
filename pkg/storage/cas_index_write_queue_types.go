package storage

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage/locknames"
)

// ListingIndexBatchEventCallback is a callback function type for emitting batch processing events
// This allows the write queue to emit events without creating import cycles
// The callback is set by the CLI layer (cmd/zqk/system) which has access to coordination package
type ListingIndexBatchEventCallback func(
	ctx context.Context,
	projectRoot string,
	storageProvider ObjectStorageProvider,
	kind string,
	batchSize int,
	duration time.Duration,
	status string,
	err error,
)

var (
	globalListingIndexBatchEventCallback       atomic.Pointer[ListingIndexBatchEventCallback]
	globalListingIndexCallbacksRegisteredTotal atomic.Int64
	globalListingIndexCallbacksTriggeredTotal  atomic.Int64
)

// GetListingIndexCallbackStats returns lifetime counters for callbacks registered and triggered.
func GetListingIndexCallbackStats() (registered, triggered int64) {
	return globalListingIndexCallbacksRegisteredTotal.Load(), globalListingIndexCallbacksTriggeredTotal.Load()
}

// SetListingIndexBatchEventCallback sets the callback for emitting batch processing events
// This should be called by the CLI layer to wire up coordinator integration
// Thread-safe
func SetListingIndexBatchEventCallback(callback ListingIndexBatchEventCallback) {
	globalListingIndexCallbacksRegisteredTotal.Add(1)
	if callback == nil {
		globalListingIndexBatchEventCallback.Store(nil)
		return
	}
	ptr := new(ListingIndexBatchEventCallback)
	*ptr = callback
	globalListingIndexBatchEventCallback.Store(ptr)
}

// getListingIndexBatchEventCallback returns the current callback (if set)
// Thread-safe
func getListingIndexBatchEventCallback() ListingIndexBatchEventCallback {
	ptr := globalListingIndexBatchEventCallback.Load()
	if ptr == nil {
		return nil
	}
	globalListingIndexCallbacksTriggeredTotal.Add(1)
	return *ptr
}

// ListingIndexStateChangeEventCallback is a callback for emitting CAS index queue state change events via coordinator.
// changeType is e.g. ConstStreamProjectRootSet or "storage_set". Set by the CLI layer for coordinator integration.
type ListingIndexStateChangeEventCallback func(
	ctx context.Context,
	projectRoot string,
	storageProvider ObjectStorageProvider,
	changeType string,
)

var (
	globalListingIndexStateChangeEventCallback atomic.Pointer[ListingIndexStateChangeEventCallback]
)

// SetListingIndexStateChangeEventCallback sets the callback for state change events.
// This should be called by the CLI layer to wire up coordinator integration. Thread-safe.
func SetListingIndexStateChangeEventCallback(callback ListingIndexStateChangeEventCallback) {
	if callback == nil {
		globalListingIndexStateChangeEventCallback.Store(nil)
		return
	}
	ptr := new(ListingIndexStateChangeEventCallback)
	*ptr = callback
	globalListingIndexStateChangeEventCallback.Store(ptr)
}

// getListingIndexStateChangeEventCallback returns the current state change callback (if set). Thread-safe.
func getListingIndexStateChangeEventCallback() ListingIndexStateChangeEventCallback {
	ptr := globalListingIndexStateChangeEventCallback.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

const (
	// casIndexBatchSize is the maximum number of index updates to process in a single batch
	casIndexBatchSize = 100
	// casIndexBatchTimeout is the maximum time to wait before processing a batch (even if not full)
	// Reduced from 120ms to 5ms to prevent Nagle-like delays in sequential object creation (e.g. system init).
	// This ensures FlushKindContext and related operations don't stall for long periods.
	casIndexBatchTimeout = 5 * time.Millisecond
	// casIndexIdleTimeout is the time to wait with empty queue before shutting down worker
	casIndexIdleTimeout = 5 * time.Minute

	// casIndexEmptyQueueExtraWait is extra safety time added by FlushKindContext after the
	// queue appears empty twice in a row.
	casIndexEmptyQueueExtraWait = 20 * time.Millisecond
)

// indexUpdateRequest represents a single CAS index update request (set or remove)
type indexUpdateRequest struct {
	objectID    string
	hash        string
	bucketKey   string     // from bucket strategy at create time; empty = base dir (set only)
	createdAt   string     // RFC3339 for high-volume kinds (enables OldestIDs); empty = not set
	remove      bool       // if true, remove mapping for objectID; hash is ignored
	done        chan error // Optional: channel to signal completion
	opCallback  concurrency.OperationCallback
	operationID string
	startedAt   time.Time
}

// indexQueue manages batched writes for a single kind's CAS index
// Implements the "On-Demand Worker" pattern (wake-on-work with idle shutdown)
type indexQueue struct {
	kind             string
	queue            chan *indexUpdateRequest
	cas              *ContentAddressableStorage
	batchSize        int
	timeout          time.Duration
	workerRunning    atomic.Int32 // Atomic flag to track if worker is currently running
	workerProcessing atomic.Int32 // Atomic flag to track if worker is actively processing a batch
	pendingItems     atomic.Int64 // Atomic flag to track number of pending items
	enqueuedTotal    atomic.Int64 // Atomic counter for lifetime enqueued items
	processedTotal   atomic.Int64 // Atomic counter for lifetime processed items
	mu               sync.RWMutex // Protects cas field updates
	projectRoot      atomic.Value // Stores string (lock-free reads)
	storage          atomic.Value // Stores ObjectStorageProvider (lock-free reads)
	secCtx           *pkgctx.SecurityContext
	ctx              context.Context
	cancel           context.CancelFunc
	wgManager        *WaitGroupManager // Centralized WaitGroup management
	queueID          string            // Unique ID for this queue (for WaitGroupManager)
}

// ListingIndexWriteQueue manages batched writes to CAS indexes for all kinds
// This prevents race conditions by serializing index updates through a single writer per kind
type ListingIndexWriteQueue struct {
	queues      map[string]*indexQueue
	mu          sync.RWMutex // Protects queues map
	projectRoot atomic.Value // Stores string (lock-free reads)
	storage     atomic.Value // Stores ObjectStorageProvider (lock-free reads)
	secCtx      *pkgctx.SecurityContext
	// skipShutdownCoordinatorCheck when true skips the global coordinator check in enqueue.
	// Set on queues created for tests so one test's global shutdown does not fail another test's updates.
	skipShutdownCoordinatorCheck atomic.Bool
}

var (
	globalListingIndexWriteQueue *ListingIndexWriteQueue
	globalCASQueueOnce           sync.Once
)

// ListingIndexWriteQueueFactory returns a CAS index write queue for the given project root.
// When the default factory is nil, GetListingIndexWriteQueueForProjectRoot uses the global singleton.
// Tests can set a factory that returns one queue per project root for isolation (no contention).
type ListingIndexWriteQueueFactory func(projectRoot string) *ListingIndexWriteQueue

var (
	defaultCASQueueFactory   ListingIndexWriteQueueFactory
	defaultCASQueueFactoryMu sync.RWMutex
)

// SetListingIndexWriteQueueFactory sets the factory used to obtain a write queue per project root.
// When f is nil, the process uses the global singleton (production behavior).
// For tests, use SetListingIndexWriteQueueFactoryToPerProjectRoot() so each project root gets its own queue.
func SetListingIndexWriteQueueFactory(f ListingIndexWriteQueueFactory) {
	if err := concurrency.RunInLockWithLogger(&defaultCASQueueFactoryMu, locknames.LockNameListingIndexSetFactory, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		defaultCASQueueFactory = f
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ConstStreamFailedToSetListingIndexWriteQueueFactoryValN, err).Log()
	}
}

// GetListingIndexWriteQueueForProjectRoot returns the batched writer that materializes per-kind
// listing indexes after writes (so List() observes new objects). The CAS* name is historical;
// behavior is the storage-layer listing contract, not a promise of a particular on-disk CAS layout
// to callers.
// If a factory is set, it returns factory(projectRoot). Otherwise returns the global singleton.
func GetListingIndexWriteQueueForProjectRoot(projectRoot string) *ListingIndexWriteQueue {
	var f ListingIndexWriteQueueFactory
	if err := concurrency.RunInRLock(&defaultCASQueueFactoryMu, func() error {
		f = defaultCASQueueFactory
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ConstStreamFailedToGetListingIndexWriteQueueFactoryValN, err).Log()
	}
	if f != nil {
		return f(projectRoot)
	}
	return GetGlobalListingIndexWriteQueue()
}

var perProjectRootQueuesMu sync.Mutex
var perProjectRootQueues = make(map[string]*ListingIndexWriteQueue)

// SetListingIndexWriteQueueFactoryToPerProjectRoot sets the factory to one that returns a dedicated
// queue per project root. Use in test setup so each test's storage has its own queue (no contention).
func SetListingIndexWriteQueueFactoryToPerProjectRoot() {
	SetListingIndexWriteQueueFactory(func(projectRoot string) *ListingIndexWriteQueue {
		var q *ListingIndexWriteQueue
		if err := concurrency.RunInLock(&perProjectRootQueuesMu, func() error {
			if existing, ok := perProjectRootQueues[projectRoot]; ok {
				q = existing
				return nil
			}
			newQ := NewListingIndexWriteQueueForTest()
			perProjectRootQueues[projectRoot] = newQ
			q = newQ
			return nil
		}); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ConstStreamFailedToSetListingIndexWriteQueueFactoryFor, err).Log()
		}
		return q
	})
}

// RemoveListingIndexWriteQueueForProjectRoot removes the queue for a given project root.
func RemoveListingIndexWriteQueueForProjectRoot(projectRoot string) {
	if err := concurrency.RunInLock(&perProjectRootQueuesMu, func() error {
		if q, ok := perProjectRootQueues[projectRoot]; ok {
			q.Shutdown()
			delete(perProjectRootQueues, projectRoot)
		}
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("Failed to remove listing index write queue for project root: %v", err).Log()
	}
}

// NewListingIndexWriteQueue creates a new CAS index write queue (not registered with shutdown coordinator).
// Used by the per-project-root factory so tests get isolated queues.
func NewListingIndexWriteQueue() *ListingIndexWriteQueue {
	return &ListingIndexWriteQueue{
		queues: make(map[string]*indexQueue),
		secCtx: pkgctx.NewSystemSecurityContext(),
	}
}

// NewListingIndexWriteQueueForTest creates a queue that skips the global shutdown coordinator check in enqueue.
// Use in the per-project-root factory so test queues are not failed by another test's global shutdown.
func NewListingIndexWriteQueueForTest() *ListingIndexWriteQueue {
	q := NewListingIndexWriteQueue()
	q.skipShutdownCoordinatorCheck.Store(true)
	return q
}

// GetGlobalListingIndexWriteQueue returns the global CAS index write queue instance.
// Thread-safe singleton pattern. Prefer GetListingIndexWriteQueueForProjectRoot(projectRoot) so
// tests can use a factory for per-project-root queues.
func GetGlobalListingIndexWriteQueue() *ListingIndexWriteQueue {
	globalCASQueueOnce.Do(func() {
		globalListingIndexWriteQueue = NewListingIndexWriteQueue()
		// Register with shutdown coordinator so InitiateShutdown() propagates to all queues
		coordinator := GetGlobalShutdownCoordinator()
		if coordinator != nil {
			coordinator.RegisterQueue(globalListingIndexWriteQueue)
		}
	})
	return globalListingIndexWriteQueue
}

// SetProjectRoot sets the project root for coordination events
// Uses atomic.Value for lock-free reads (replaces mutex)
func (q *ListingIndexWriteQueue) SetProjectRoot(projectRoot string) {
	q.projectRoot.Store(projectRoot)
	callback := getListingIndexStateChangeEventCallback()
	if callback != nil && projectRoot != emptyValue {
		storageProvider := q.GetStorage()
		goroutinelabels.NewGoroutine(ConstStreamCasIndexStateChangeEvent, ConstStreamEmittingStateChangeProjectRootSet).
			StartSimple(func() {
				callback(pkgctx.NewSystemContext(), projectRoot, storageProvider, ConstStreamProjectRootSet)
			})
	}
}

// GetProjectRoot returns the project root (lock-free read)
func (q *ListingIndexWriteQueue) GetProjectRoot() string {
	if val := q.projectRoot.Load(); val != nil {
		return val.(string)
	}
	return ""
}

// SetStorage sets the storage provider for coordination events
// Uses atomic.Value for lock-free reads (replaces mutex)
func (q *ListingIndexWriteQueue) SetStorage(storage ObjectStorageProvider) {
	q.storage.Store(storage)
	callback := getListingIndexStateChangeEventCallback()
	if callback != nil && storage != nil {
		projectRoot := q.GetProjectRoot()
		if projectRoot != emptyValue {
			goroutinelabels.NewGoroutine(ConstStreamCasIndexStateChangeEvent, ConstStreamEmittingStateChangeStorageSet).
				StartSimple(func() {
					callback(pkgctx.NewSystemContext(), projectRoot, storage, "storage_set")
				})
		}
	}
}

// GetStorage returns the storage provider (lock-free read)
func (q *ListingIndexWriteQueue) GetStorage() ObjectStorageProvider {
	if val := q.storage.Load(); val != nil {
		return val.(ObjectStorageProvider)
	}
	return nil
}

// getProjectRoot returns the project root (lock-free read)
func (iq *indexQueue) getProjectRoot() string {
	if val := iq.projectRoot.Load(); val != nil {
		return val.(string)
	}
	return ""
}

// getStorage returns the storage provider (lock-free read)
func (iq *indexQueue) getStorage() ObjectStorageProvider {
	if val := iq.storage.Load(); val != nil {
		return val.(ObjectStorageProvider)
	}
	return nil
}
