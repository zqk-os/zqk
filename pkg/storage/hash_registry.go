package storage

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage/locknames"
)

// HashRegistryEventCallback is a callback for emitting events via coordinator
// This avoids import cycles by using dependency injection
type HashRegistryEventCallback func(
	ctx context.Context,
	projectRoot string,
	storage ObjectStorageProvider,
	kind string,
	batchSize int,
	hashCount int,
	duration time.Duration,
	status string,
	err error,
)

var (
	globalHashRegistryEventCallback atomic.Pointer[HashRegistryEventCallback]

	globalHashRegistryTracker   []*HashRegistry
	globalHashRegistryTrackerMu sync.Mutex
)

// SetHashRegistryEventCallback sets the global callback for emitting events via coordinator
// This should be called during system initialization to wire up coordinator integration
func SetHashRegistryEventCallback(callback HashRegistryEventCallback) {
	if callback == nil {
		globalHashRegistryEventCallback.Store(nil)
		return
	}
	ptr := new(HashRegistryEventCallback)
	*ptr = callback
	globalHashRegistryEventCallback.Store(ptr)
}

func getHashRegistryEventCallback() HashRegistryEventCallback {
	ptr := globalHashRegistryEventCallback.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

const (
	// saveBatchSize is the maximum number of save requests to process in a single batch.
	// Kept small to limit memory: each request holds a full copy of the hash map (path→hash).
	saveBatchSize = 100
	// saveQueueCapacity is the max number of save requests queued. When full, SaveAsync drops
	// the new request (worker will persist a recent snapshot). Reduces memory from 1000 full
	// copies to at most saveQueueCapacity + saveBatchSize (see SCHEDULER_DUMP_ANALYSIS_20260302.md).
	saveQueueCapacity = 100
	// saveBatchTimeout is the maximum time to wait before processing a batch (even if not full)
	// Reduced from 100ms to 5ms to prevent Nagle-like delays in sequential object creation (e.g. system init).
	saveBatchTimeout = 5 * time.Millisecond
	// hashRegistryIdleTimeout is the time to wait with empty queue before shutting down worker
	hashRegistryIdleTimeout = 5 * time.Minute
	// hashRegistrySaveMaxWait is the maximum time a caller blocks waiting for Save() to complete.
	// processSave no longer calls file.Sync() (F_FULLFSYNC on macOS), so the actual I/O is fast
	// (write + rename for a small file). 60s is a generous safety net for queue-processing overhead.
	hashRegistrySaveMaxWait = 60 * time.Second
)

// saveRequest represents a save operation queued for processing
type saveRequest struct {
	data map[string]string // Snapshot of hashes to save
	done chan error        // Channel to signal completion
}

// HashRegistry manages hashes for all files of a given object kind
// Uses a single file per kind (e.g., ".backlog_item.hashes") instead of
// individual hash files per object file.
//
// This is the file-based implementation of HashRegistryProvider.
// For graph-based storage, use GraphHashRegistry instead.
//
// Thread Safety:
// - Uses sync.RWMutex for in-process thread safety
// - Mutex is process-local: if a process dies, the lock is automatically released
// - No file-based locking: concurrent access from multiple processes is not protected
// - File operations (ReadFile/WriteFile) are not wrapped with timeouts - they rely on OS defaults
// - Save operations are serialized through a background worker to prevent race conditions
//
// Note: For cross-process locking, file-based locks would be needed, but that's not
// currently implemented as hash registry operations are expected to be fast and
// typically single-process.
type HashRegistry struct {
	kind         string
	dir          string
	hashes       map[string]string // filename -> hash
	mu           sync.RWMutex      // Process-local mutex (released automatically if process dies)
	filePath     string
	lastLoadTime time.Time // Track when registry was last loaded
	fileMtime    time.Time // Track file modification time to avoid unnecessary reloads

	// Save queue for serialized saves
	saveQueue     chan *saveRequest
	workerRunning atomic.Int32 // Atomic flag to track if worker is currently running (on-demand pattern)
	ctx           context.Context
	cancel        context.CancelFunc
	wgManager     *WaitGroupManager // Centralized WaitGroup management

	// Coordinator integration (optional - set via SetProjectRoot/SetStorage)
	projectRoot atomic.Value // Stores string (lock-free reads)
	storage     atomic.Value // Stores ObjectStorageProvider (lock-free reads)

	// skipShutdownCoordinatorCheck when true skips the global coordinator check in Save().
	// Set for registries created by test storage so global shutdown from another test does not fail saves.
	skipShutdownCoordinatorCheck atomic.Bool
}

// SetProjectRoot sets the project root for coordinator event emission
// Uses atomic.Value for lock-free reads (replaces mutex)
func (hr *HashRegistry) SetProjectRoot(projectRoot string) {
	hr.projectRoot.Store(projectRoot)
	// TODO: Emit coordinator event for state change
}

// GetProjectRoot returns the project root (lock-free read)
func (hr *HashRegistry) GetProjectRoot() string {
	if val := hr.projectRoot.Load(); val != nil {
		return val.(string)
	}
	return ""
}

// SetStorage sets the storage provider for coordinator event emission
// Uses atomic.Value for lock-free reads (replaces mutex)
func (hr *HashRegistry) SetStorage(storage ObjectStorageProvider) {
	hr.storage.Store(storage)
	// TODO: Emit coordinator event for state change
}

// GetStorage returns the storage provider (lock-free read)
func (hr *HashRegistry) GetStorage() ObjectStorageProvider {
	if val := hr.storage.Load(); val != nil {
		return val.(ObjectStorageProvider)
	}
	return nil
}

// SetSkipShutdownCoordinatorCheck sets whether Save() should skip the global shutdown coordinator check.
// Used by test storage so one test's global shutdown does not fail another test's saves.
func (hr *HashRegistry) SetSkipShutdownCoordinatorCheck(skip bool) {
	hr.skipShutdownCoordinatorCheck.Store(skip)
}

// Ensure HashRegistry implements HashRegistryProvider
var _ HashRegistryProvider = (*HashRegistry)(nil)

// NewHashRegistry creates a new hash registry for a given object kind
// ctx: parent context from command entry point (should not be created here)
func NewHashRegistry(ctx context.Context, kind, dir string) *HashRegistry {
	// Use kind name for hash file (e.g., "backlog_item" -> ".backlog_item.hashes")
	hashFileName := fmt.Sprintf(".%s.hashes", kind)
	ctx, cancel := context.WithCancel(ctx) //nolint:gosec // G118: cancel stored on HashRegistry; invoked from Close/shutdown
	hr := &HashRegistry{
		kind:      kind,
		dir:       dir,
		hashes:    make(map[string]string),
		filePath:  filepath.Join(dir, hashFileName),
		saveQueue: make(chan *saveRequest, saveQueueCapacity),
		// workerRunning starts at 0 (default for atomic.Int32) - Worker starts stopped (on-demand)
		ctx:       ctx,
		cancel:    cancel,
		wgManager: NewWaitGroupManager(),
	}

	// Register with global tracker for test teardown
	_ = concurrency.RunInLock(&globalHashRegistryTrackerMu, func() error {
		globalHashRegistryTracker = append(globalHashRegistryTracker, hr)
		return nil
	})

	return hr
}

// ShutdownAllHashRegistriesForTesting forcefully shuts down all created HashRegistries.
// Called by project_test_teardown.go to prevent goroutine leaks.
func ShutdownAllHashRegistriesForTesting() {
	ShutdownHashRegistriesForProjectTesting("")
}

// ShutdownHashRegistriesForProjectTesting shuts down HashRegistries belonging to a specific project.
func ShutdownHashRegistriesForProjectTesting(projectRoot string) {
	var regsToShutdown []*HashRegistry
	_ = concurrency.RunInLock(&globalHashRegistryTrackerMu, func() error {
		var remaining []*HashRegistry
		for _, hr := range globalHashRegistryTracker {
			if projectRoot == "" || strings.HasPrefix(hr.dir, projectRoot) {
				regsToShutdown = append(regsToShutdown, hr)
			} else {
				remaining = append(remaining, hr)
			}
		}
		globalHashRegistryTracker = remaining
		return nil
	})
	for _, hr := range regsToShutdown {
		_ = hr.InitiateShutdown()
	}
}

// wakeWorkerIfNeeded starts the worker if it's not already running
// Thread-safe (uses atomic operations)
func (hr *HashRegistry) GetHash(filename string) string {
	var result string
	if err := concurrency.RunInRLockWithLogger(&hr.mu, locknames.LockNameHashRegistryGetHash, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		result = hr.hashes[filename]
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// SetHash sets the hash for a given filename
			Error(ErrMsgLockFailedGen, err).Log()
	}

	return result
}

func (hr *HashRegistry) SetHash(filename, hash string) {
	if err := concurrency.RunInLockWithLogger(&hr.mu, locknames.LockNameHashRegistrySetHash, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		hr.hashes[filename] = hash
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// DeleteHash removes the hash for a given filename
			Error(ErrMsgLockFailedGen, err).Log()
	}

}

func (hr *HashRegistry) DeleteHash(filename string) {
	if err := concurrency.RunInLockWithLogger(&hr.mu, locknames.LockNameHashRegistryDeleteHash, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		delete(hr.hashes, filename)
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// HasHash returns true if a hash exists for the given filename
			Error(ErrMsgLockFailedGen, err).Log()
	}

}

func (hr *HashRegistry) HasHash(filename string) bool {
	var exists bool
	if err := concurrency.RunInRLockWithLogger(&hr.mu, locknames.LockNameHashRegistryHasHash, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		_, exists = hr.hashes[filename]
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// GetAllHashes returns a copy of all hashes
			Error(ErrMsgLockFailedGen, err).Log()
	}

	return exists
}

func (hr *HashRegistry) GetAllHashes() map[string]string {
	var result map[string]string
	if err := concurrency.RunInRLockWithLogger(&hr.mu, locknames.LockNameHashRegistryGetAllHashes, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		result = make(map[string]string, len(hr.hashes))
		maps.Copy(result, hr.hashes)
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// InitiateShutdown implements QueueShutdownHandler
			// Stops accepting new save operations
			Error(ErrMsgLockFailedGen, err).Log()
	}

	return result
}

func (hr *HashRegistry) InitiateShutdown() error {
	hr.cancel()
	return nil
}

// Drain implements QueueShutdownHandler
// Processes all pending save operations
func (hr *HashRegistry) Drain(ctx context.Context) error {
	// Cancel context to stop accepting new work
	hr.cancel()

	// Wait for worker to finish
	wgID := fmt.Sprintf(DescHashRegWorker, hr.kind)
	done := make(chan struct{})

	// Use callback pattern: wait in goroutine, notify via channel
	// The goroutine will exit when the context is cancelled
	goroutinelabels.NewGoroutine(DescHashRegDrainWait, fmt.Sprintf(DescWaitHashRegWorker, hr.kind)).
		WithContext(ctx).
		WithCleanup(func() {
			select {
			case <-done:
				// Already closed
			default:
				close(done)
			}
		}).
		StartSimple(func() {
			// Wait for worker to complete, but check context periodically
			waitDone := make(chan struct{})
			goroutinelabels.NewGoroutine(DescHashRegDrainWaitInner, fmt.Sprintf(DescWaitWaitGroup, wgID)).
				StartSimple(func() {
					hr.wgManager.Wait(wgID)
					close(waitDone)
				})

			select {
			case <-waitDone:
				// Worker completed
				select {
				case <-done:
				default:
					close(done)
				}
			case <-ctx.Done():
				// Timeout or context cancelled - exit without closing done
				return
			}
		})

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsDrained implements QueueShutdownHandler
func (hr *HashRegistry) IsDrained() bool {
	return len(hr.saveQueue) == 0 && hr.workerRunning.Load() == 0
}

// GetPendingCount implements QueueShutdownHandler
func (hr *HashRegistry) GetPendingCount() int64 {
	return int64(len(hr.saveQueue))
}

// GetName implements QueueShutdownHandler
func (hr *HashRegistry) GetName() string {
	return fmt.Sprintf(DescHashRegName, hr.kind)
}

// IsCritical implements QueueShutdownHandler
// Hash registry saves are critical - must complete before shutdown
func (hr *HashRegistry) IsCritical() bool {
	return true
}
