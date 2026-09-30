package storage

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/when"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// IOOperationType represents the type of I/O operation
type IOOperationType string

const (
	IOOperationRead  IOOperationType = "read"
	IOOperationWrite IOOperationType = "write"
)

// IOOperation represents a single I/O operation
type IOOperation struct {
	Type     IOOperationType
	FilePath string
	Data     []byte            // For writes
	Perm     fileutil.FileMode // For writes
	Result   chan IOResult     // Channel for async result
	GroupID  string            // Optional: for work grouping
}

// IOResult represents the result of an I/O operation
type IOResult struct {
	Data []byte
	Obj  map[string]any // For reads
	Err  error
}

// IOQueueConfig configures I/O queue behavior
type IOQueueConfig struct {
	MinQueues     int           // Minimum number of queues (default: 1)
	MaxQueues     int           // Maximum number of queues (default: 10)
	MaxQueueDepth int           // Threshold for creating new queue (default: 1000)
	BatchSize     int           // Batch size for processing (default: 50)
	BatchTimeout  time.Duration // Batch timeout (default: 100ms)
	IdleTimeout   time.Duration // Worker idle timeout (default: 5min)
	Strategy      QueueStrategy // Round-robin or grouped
}

// QueueStrategy determines how operations are distributed across queues
type QueueStrategy string

const (
	QueueStrategyRoundRobin QueueStrategy = "round_robin"
	QueueStrategyGrouped    QueueStrategy = "grouped"
)

// DefaultIOQueueConfig returns default I/O queue configuration
func DefaultIOQueueConfig() *IOQueueConfig {
	return &IOQueueConfig{
		MinQueues:     1,
		MaxQueues:     10,
		MaxQueueDepth: 1000,
		BatchSize:     50,
		BatchTimeout:  100 * time.Millisecond,
		IdleTimeout:   5 * time.Minute,
		Strategy:      QueueStrategyRoundRobin,
	}
}

// ioQueue represents a single I/O queue with dedicated worker
type ioQueue struct {
	operations    chan *IOOperation
	workerRunning atomic.Int32
	queueDepth    atomic.Int64 // Atomic counter for queue depth
	ctx           context.Context
	cancel        context.CancelFunc
	wgManager     *WaitGroupManager
	queueID       string // Unique ID for this queue (for WaitGroupManager)
	batchSize     int
	batchTimeout  time.Duration
	idleTimeout   time.Duration
	manager       *IOQueueManager // Reference to manager for storage access
}

// IOQueueManager manages multiple I/O queues for load distribution
type IOQueueManager struct {
	queues      []*ioQueue
	queueIndex  atomic.Int32 // Atomic counter for round-robin selection
	mu          sync.RWMutex // Protects queues slice and config
	config      *IOQueueConfig
	projectRoot atomic.Value    // Stores string (lock-free reads)
	storage     atomic.Value    // Stores any (lock-free reads)
	ctx         context.Context // Dedicated daemon lifecycle context
	cancel      context.CancelFunc
	// testOverride controls whether Enqueue should honor global shutdown
	// state when running under tests (ZQK_TEST_ROOT is set). By default,
	// tests ignore shutdown to avoid cross-test interference; specific
	// tests that verify shutdown behavior can enable this flag.
	testOverride bool
}

var (
	globalIOQueueManager *IOQueueManager
	globalIOQueueOnce    sync.Once

	globalIOQueueStateChangeEventCallback atomic.Pointer[IOQueueStateChangeEventCallback]
)

// IOQueueStateChangeEventCallback is a callback for emitting I/O queue state change events via coordinator.
// changeType is e.g. "project_root_set" or "storage_set". Set by the CLI layer for coordinator integration.
type IOQueueStateChangeEventCallback func(
	ctx context.Context,
	projectRoot string,
	storageProvider ObjectStorageProvider,
	changeType string,
)

// SetIOQueueStateChangeEventCallback sets the callback for I/O queue state change events.
// This should be called by the CLI layer to wire up coordinator integration. Thread-safe.
func SetIOQueueStateChangeEventCallback(callback IOQueueStateChangeEventCallback) {
	if callback == nil {
		globalIOQueueStateChangeEventCallback.Store(nil)
		return
	}
	ptr := new(IOQueueStateChangeEventCallback)
	*ptr = callback
	globalIOQueueStateChangeEventCallback.Store(ptr)
}

// getIOQueueStateChangeEventCallback returns the current state change callback (if set). Thread-safe.
func getIOQueueStateChangeEventCallback() IOQueueStateChangeEventCallback {
	ptr := globalIOQueueStateChangeEventCallback.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

// GetGlobalIOQueueManager returns the global I/O queue manager (singleton)
// Uses a dedicated background daemon lifecycle context rather than borrowing
// an ephemeral CLI command context, preventing background queues from being
// cancelled upon command completion.
func GetGlobalIOQueueManager(ctx context.Context) *IOQueueManager {
	globalIOQueueOnce.Do(func() {
		daemonCtx, cancel := context.WithCancel(context.Background())
		globalIOQueueManager = &IOQueueManager{
			queues: make([]*ioQueue, 0),
			config: DefaultIOQueueConfig(),
			ctx:    daemonCtx,
			cancel: cancel,
		}
		// Initialize with minimum queues using daemon lifecycle context
		for i := 0; i < globalIOQueueManager.config.MinQueues; i++ {
			globalIOQueueManager.addQueue(daemonCtx)
		}
		// Register with shutdown coordinator so InitiateShutdown() propagates to all queues
		coordinator := GetGlobalShutdownCoordinator()
		if coordinator != nil {
			coordinator.RegisterQueue(globalIOQueueManager)
		}
	})
	return globalIOQueueManager
}

// SetConfig updates the I/O queue manager configuration
func (m *IOQueueManager) SetConfig(config *IOQueueConfig) {
	_ = concurrency.RunInLockOrLog(&m.mu, locknames.LockNameIoQueueSetConfig, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		m.config = config
		return nil
	})
}

// SetProjectRoot sets the project root for coordination events
// Uses atomic.Value for lock-free reads (replaces mutex)
func (m *IOQueueManager) SetProjectRoot(projectRoot string) {
	if m.GetProjectRoot() == projectRoot {
		return
	}
	m.projectRoot.Store(projectRoot)
	callback := getIOQueueStateChangeEventCallback()
	if callback != nil && projectRoot != emptyValue {
		if storageProvider, ok := m.GetStorage().(ObjectStorageProvider); ok {
			stateBud := goroutinelabels.DefaultBudget()
			stateBuilder := goroutinelabels.NewGoroutine(ConstMiscIoQueueStateChangeEvent, ConstMiscEmittingStateChangeProjectRootSet)
			if stateBud != nil {
				stateBuilder = stateBuilder.WithBudget(stateBud)
			}
			stateBuilder.StartSimple(func() {
				callback(pkgctx.NewSystemContext(), projectRoot, storageProvider, ConstMiscProjectRootSet)
			})
		}
	}
}

// GetProjectRoot returns the project root (lock-free read)
func (m *IOQueueManager) GetProjectRoot() string {
	if val := m.projectRoot.Load(); val != nil {
		return val.(string)
	}
	return ""
}

// SetStorage sets the storage provider for coordination events
// Uses atomic.Value for lock-free reads (replaces mutex)
func (m *IOQueueManager) SetStorage(storage any) {
	m.storage.Store(storage)
	callback := getIOQueueStateChangeEventCallback()
	if callback != nil && storage != nil {
		if storageProvider, ok := storage.(ObjectStorageProvider); ok {
			projectRoot := m.GetProjectRoot()
			if projectRoot != emptyValue {
				stateBud := goroutinelabels.DefaultBudget()
				stateBuilder := goroutinelabels.NewGoroutine(ConstMiscIoQueueStateChangeEvent, ConstMiscEmittingStateChangeStorageSet)
				if stateBud != nil {
					stateBuilder = stateBuilder.WithBudget(stateBud)
				}
				stateBuilder.StartSimple(func() {
					callback(pkgctx.NewSystemContext(), projectRoot, storageProvider, "storage_set")
				})
			}
		}
	}
}

// GetStorage returns the storage provider (lock-free read)
func (m *IOQueueManager) GetStorage() any {
	return m.storage.Load()
}

// addQueue creates a new I/O queue and starts its worker
func (m *IOQueueManager) addQueue(ctx context.Context) *ioQueue {
	parent := m.ctx
	if parent == nil {
		parent = ctx
	}
	if parent == nil {
		parent = context.Background()
	}
	// Derive cancellation context from manager's daemon lifecycle context
	queueCtx, cancel := context.WithCancel(parent) //nolint:gosec // G118: cancel stored on ioQueue
	queueID := fmt.Sprintf(ConstMiscIoQueueDD, len(m.queues), time.Now().UnixNano())
	queue := &ioQueue{
		operations: make(chan *IOOperation, m.config.MaxQueueDepth),
		// workerRunning and queueDepth start at 0 (default for atomic types)
		ctx:          queueCtx,
		cancel:       cancel,
		wgManager:    NewWaitGroupManager(),
		queueID:      queueID,
		batchSize:    m.config.BatchSize,
		batchTimeout: m.config.BatchTimeout,
		idleTimeout:  m.config.IdleTimeout,
		manager:      m, // Store reference to manager for storage access
	}

	m.queues = append(m.queues, queue)
	queue.wakeWorkerIfNeeded()
	return queue
}

// Enqueue enqueues an I/O operation
// Automatically scales queues if needed
// Returns ErrShutdownInProgress if shutdown has been initiated
func (m *IOQueueManager) Enqueue(op *IOOperation) error {
	// Check if shutdown has been initiated. In tests, this check is
	// skipped by default to avoid global shutdown from one test
	// affecting others. Tests that need to verify shutdown behavior
	// can enable testOverride explicitly.
	if zqkenv.TestRoot().Get() == emptyValue || m.testOverride {
		coordinator := GetGlobalShutdownCoordinator()
		if coordinator.IsShutdownInitiated() {
			return errfmt.Errorf("shutdown in progress, cannot enqueue I/O operation")
		}
	}

	// Check if we need to scale up
	m.checkAndScale()

	// Select queue based on strategy
	var queue *ioQueue
	when.When(func() bool { return m.config.Strategy == QueueStrategyGrouped && op.GroupID != emptyValue }).Then(func() {
		queue = m.selectQueueGrouped(op.GroupID)
	}).OrElse(func() {
		queue = m.selectQueueRoundRobin()
	}).Run()

	// Enqueue operation
	select {
	case queue.operations <- op:
		queue.queueDepth.Add(1)
		// Wake worker if needed (on-demand pattern)
		queue.wakeWorkerIfNeeded()
		return nil
	default:
		// Queue is full - try to create new queue if possible
		if len(m.queues) < m.config.MaxQueues {
			newQueue := m.addQueue(m.ctx)
			select {
			case newQueue.operations <- op:
				newQueue.queueDepth.Add(1)
				return nil
			default:
				return errfmt.Errorf("I/O queue is full and cannot create new queue")
			}
		}
		return errfmt.Errorf("I/O queue is full")
	}
}

// selectQueueRoundRobin selects a queue using round-robin strategy
func (m *IOQueueManager) selectQueueRoundRobin() *ioQueue {
	var queue *ioQueue
	_ = concurrency.RunInRLockOrLog(&m.mu, locknames.LockNameIoQueueManagerSelectRoundRobin, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if len(m.queues) == 0 {
			queue = m.addQueue(m.ctx)
			return nil
		}
		queueCount := len(m.queues)
		if queueCount == 0 {
			queue = nil
			return nil
		}
		idx := int(m.queueIndex.Add(1))
		index := idx % queueCount
		if index < 0 {
			index += queueCount
		}
		queue = m.queues[index]
		return nil
	})
	return queue
}

// selectQueueGrouped selects a queue based on group ID (consistent hashing)
func (m *IOQueueManager) selectQueueGrouped(groupID string) *ioQueue {
	var queueCount int
	err := concurrency.RunInRLockWithLogger(&m.mu, locknames.LockNameIoQueueGetQueue, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		queueCount = len(m.queues)
		return nil
	})
	if err != nil {
		return nil
	}
	if queueCount == 0 {
		return m.addQueue(m.ctx)
	}
	// Simple hash-based selection (bytes avoid gosec G115 on rune→uint32)
	hash := uint32(0)
	for i := 0; i < len(groupID); i++ {
		hash = hash*31 + uint32(groupID[i])
	}
	index := int(hash) % len(m.queues)
	return m.queues[index]
}

// checkAndScale checks queue depths and creates new queues if needed
func (m *IOQueueManager) checkAndScale() {
	var needsScale bool
	var maxQueues int
	_ = concurrency.RunInRLockOrLog(&m.mu, locknames.LockNameIoQueueManagerCheckScale, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if len(m.queues) >= m.config.MaxQueues {
			return nil
		}
		maxQueues = m.config.MaxQueues
		for _, queue := range m.queues {
			depth := queue.queueDepth.Load()
			if depth > int64(m.config.MaxQueueDepth) {
				needsScale = true
				break
			}
		}
		return nil
	})

	if needsScale {
		_ = concurrency.RunInLockOrLog(&m.mu, locknames.LockNameIoQueueManagerScaleAdd, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
			if len(m.queues) < maxQueues {
				m.addQueue(m.ctx)
			}
			return nil
		})
	}
}

// wakeWorkerIfNeeded starts the worker if it's not already running
func (q *ioQueue) wakeWorkerIfNeeded() {
	if q.workerRunning.CompareAndSwap(0, 1) {
		wgID := fmt.Sprintf("%s_worker", q.queueID)
		wg := q.wgManager.CreateGroupForGoroutine(wgID, ConstMiscIoQueueWorker)
		ioBud := goroutinelabels.DefaultBudget()
		ioWorkerBuilder := goroutinelabels.NewGoroutine(ConstMiscIoQueueWorker, "processing I/O operations").
			WithWaitGroup(wg)
		if ioBud != nil {
			ioWorkerBuilder = ioWorkerBuilder.WithBudget(ioBud)
		}
		ioWorkerBuilder.StartSimple(q.startWorker)
	}
}

// startWorker processes I/O operations in batches
