package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage/locknames"
	"github.com/lanceman/zqk/pkg/when"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"gopkg.in/yaml.v3"
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
	Data     []byte        // For writes
	Perm     os.FileMode   // For writes
	Result   chan IOResult // Channel for async result
	GroupID  string        // Optional: for work grouping
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
	ctx         context.Context // Parent context from command entry point
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
// ctx: parent context from command entry point (should not be created here)
// Note: For init() functions, context.Background() is acceptable as they run before command context exists
func GetGlobalIOQueueManager(ctx context.Context) *IOQueueManager {
	globalIOQueueOnce.Do(func() {
		globalIOQueueManager = &IOQueueManager{
			queues: make([]*ioQueue, 0),
			config: DefaultIOQueueConfig(),
			ctx:    ctx,
		}
		// Initialize with minimum queues
		for i := 0; i < globalIOQueueManager.config.MinQueues; i++ {
			globalIOQueueManager.addQueue(ctx)
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
// ctx: parent context from command entry point (should not be created here)
func (m *IOQueueManager) addQueue(ctx context.Context) *ioQueue {
	// Derive cancellation context from parent (command context)
	ctx, cancel := context.WithCancel(ctx) //nolint:gosec // G118: cancel stored on ioQueue
	queueID := fmt.Sprintf(ConstMiscIoQueueDD, len(m.queues), time.Now().UnixNano())
	queue := &ioQueue{
		operations: make(chan *IOOperation, m.config.MaxQueueDepth),
		// workerRunning and queueDepth start at 0 (default for atomic types)
		ctx:          ctx,
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
	if os.Getenv(zqkenv.TestRoot()) == emptyValue || m.testOverride {
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
func (q *ioQueue) startWorker() {
	defer func() {
		q.workerRunning.Store(0)
		// Note: wg.Done() is called by goroutinelabels.WithWaitGroup() on goroutine exit
	}()

	batch := make([]*IOOperation, 0, q.batchSize)
	batchTicker := time.NewTicker(q.batchTimeout)
	defer batchTicker.Stop()

	idleTicker := time.NewTicker(q.idleTimeout)
	defer idleTicker.Stop()

	lastWorkTime := time.Now()

	for {
		select {
		case <-q.ctx.Done():
			// Process any remaining operations before shutdown
			if len(batch) > 0 {
				q.processBatch(batch)
			}
			return

		case op := <-q.operations:
			// New work arrived - reset idle timer
			q.queueDepth.Add(-1)
			lastWorkTime = time.Now()
			idleTicker.Stop()
			idleTicker.Reset(q.idleTimeout)

			batch = append(batch, op)
			if len(batch) >= q.batchSize {
				// Batch is full - process immediately
				q.processBatch(batch)
				batch = batch[:0]
				batchTicker.Stop()
				batchTicker.Reset(q.batchTimeout)
			}

		case <-batchTicker.C:
			// Batch timeout reached - process current batch
			// CRITICAL: Check context FIRST - it may have been cancelled while we were waiting
			if q.ctx.Err() != nil {
				// Context cancelled - process any pending batch and exit
				if len(batch) > 0 {
					q.processBatch(batch)
				}
				return
			}
			if len(batch) > 0 {
				lastWorkTime = time.Now()
				q.processBatch(batch)
				batch = batch[:0]
			}

		case <-idleTicker.C:
			// Idle timeout reached - check if we should shut down
			// CRITICAL: Check context FIRST - it may have been cancelled while we were waiting
			if q.ctx.Err() != nil {
				// Context cancelled - process any pending batch and exit
				if len(batch) > 0 {
					q.processBatch(batch)
				}
				return
			}
			queueSize := len(q.operations)
			if queueSize == 0 && len(batch) == 0 {
				idleDuration := time.Since(lastWorkTime)
				if idleDuration >= q.idleTimeout {
					// Been idle long enough - shut down worker (on-demand pattern)
					return
				}
				// Not idle long enough yet - reset timer
				idleTicker.Stop()
				idleTicker.Reset(q.idleTimeout - idleDuration)
			} else {
				// There's work - reset idle timer
				lastWorkTime = time.Now()
				idleTicker.Stop()
				idleTicker.Reset(q.idleTimeout)
			}
		}
	}
}

// processBatch processes a batch of I/O operations
func (q *ioQueue) processBatch(batch []*IOOperation) {
	for _, op := range batch {
		result := q.processOperation(op)
		// Send result (non-blocking)
		select {
		case op.Result <- result:
		default:
			// Result channel is full or closed - operation will timeout
		}
	}
}

// processOperation processes a single I/O operation
func (q *ioQueue) processOperation(op *IOOperation) IOResult {
	switch op.Type {
	case IOOperationRead:
		return q.processRead(op)
	case IOOperationWrite:
		return q.processWrite(op)
	default:
		return IOResult{Err: errfmt.Errorf("unknown I/O operation type: %s", op.Type)}
	}
}

// processRead processes a read operation
func (q *ioQueue) processRead(op *IOOperation) IOResult {
	// Perform read operation directly (bypass queue to avoid circular call)
	// Read file and parse YAML
	data, err := os.ReadFile(op.FilePath)
	if os.IsNotExist(err) {
		return IOResult{Err: errfmt.Errorf(ConstMiscFileNotFoundS, op.FilePath)}
	}
	if err != nil {
		return IOResult{Err: errfmt.Errorf(ConstMiscFailedToReadFileSW, op.FilePath, err)}
	}

	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return IOResult{Err: errfmt.Errorf(ConstMiscFailedToParseYamlFromSW, op.FilePath, err)}
	}

	// Return result with object data
	return IOResult{
		Obj: obj,
		Err: nil,
	}
}

// processWrite processes a write operation
func (q *ioQueue) processWrite(op *IOOperation) IOResult {
	// Validate write operation has required data
	if len(op.Data) == 0 {
		return IOResult{Err: errfmt.Errorf(ConstMiscWriteOperationRequiresDataFileS, op.FilePath)}
	}

	// Determine file permissions (default to 0600 if not specified)
	perm := op.Perm
	if perm == 0 {
		perm = 0600
	}

	// Perform write operation directly (bypass queue to avoid circular call)
	// Ensure directory exists
	dirPath := filepath.Dir(op.FilePath)
	if err := os.MkdirAll(dirPath, paths.DirPerm755); err != nil {
		return IOResult{Err: errfmt.Errorf(ConstMiscFailedToCreateDirectoryForSW, op.FilePath, err)}
	}

	// Write file atomically using DSIA Storage Provider
	dsiaProvider := NewDSIAStorageProvider()
	if err := dsiaProvider.AtomicWriteFile(op.FilePath, op.Data, perm); err != nil {
		return IOResult{Err: errfmt.Errorf(ConstMiscFailedToWriteFileSW, op.FilePath, err)}
	}

	// Return success result
	return IOResult{
		Err: nil,
	}
}

// InitiateShutdown implements QueueShutdownHandler
// Stops accepting new operations
func (m *IOQueueManager) InitiateShutdown() error {
	var queues []*ioQueue
	_ = concurrency.RunInLockOrLog(&m.mu, locknames.LockNameIoQueueInitiateShutdown, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		queues = make([]*ioQueue, len(m.queues))
		copy(queues, m.queues)
		for _, queue := range queues {
			queue.cancel()
		}
		return nil
	})
	return nil
}

// Drain implements QueueShutdownHandler
// Processes all pending operations
func (m *IOQueueManager) Drain(ctx context.Context) error {
	// Initiate shutdown first
	if err := m.InitiateShutdown(); err != nil {
		return err
	}

	var queues []*ioQueue
	_ = concurrency.RunInRLockOrLog(&m.mu, locknames.LockNameIoQueueManagerDrainCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		queues = make([]*ioQueue, len(m.queues))
		copy(queues, m.queues)
		return nil
	})

	if len(queues) == 0 {
		return nil // No queues to drain
	}

	// Wait for all workers to finish
	var drainWg sync.WaitGroup
	ioDrainBud := goroutinelabels.DefaultBudget()
	for _, queue := range queues {
		queueCopy := queue // Capture for goroutine
		ioDrainBuilder := goroutinelabels.NewGoroutine(ConstMiscIoQueueDrainWait, fmt.Sprintf(ConstMiscWaitingForIoQueueWorkerS, queueCopy.queueID)).
			WithWaitGroup(&drainWg)
		if ioDrainBud != nil {
			ioDrainBuilder = ioDrainBuilder.WithBudget(ioDrainBud)
		}
		ioDrainBuilder.StartSimple(func() {
			// Wait for the queue's worker WaitGroup
			wgID := fmt.Sprintf("%s_worker", queueCopy.queueID)
			queueCopy.wgManager.Wait(wgID)
		})
	}

	done := make(chan struct{})
	ioCollectorBud := goroutinelabels.DefaultBudget()
	ioCollectorBuilder := goroutinelabels.NewGoroutine(ConstMiscIoQueueDrainCollector, ConstMiscWaitingForAllIoQueueDrainsToComplete).
		WithCleanup(func() {
			close(done)
		})
	if ioCollectorBud != nil {
		ioCollectorBuilder = ioCollectorBuilder.WithBudget(ioCollectorBud)
	}
	ioCollectorBuilder.StartSimple(func() {
		drainWg.Wait()
	})

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsDrained implements QueueShutdownHandler
func (m *IOQueueManager) IsDrained() bool {
	var drained bool
	_ = concurrency.RunInRLockOrLog(&m.mu, locknames.LockNameIoQueueIsDrained, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		drained = true
		for _, queue := range m.queues {
			if len(queue.operations) > 0 || queue.workerRunning.Load() == 1 {
				drained = false
				break
			}
		}
		return nil
	})
	return drained
}

// GetPendingCount implements QueueShutdownHandler
func (m *IOQueueManager) GetPendingCount() int64 {
	var queues []*ioQueue
	_ = concurrency.RunInRLockOrLog(&m.mu, locknames.LockNameIoQueueManagerGetPendingCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		queues = make([]*ioQueue, len(m.queues))
		copy(queues, m.queues)
		return nil
	})
	var total int64
	for _, queue := range queues {
		total += int64(len(queue.operations))
	}
	return total
}

// GetName implements QueueShutdownHandler
func (m *IOQueueManager) GetName() string {
	return ConstMiscIoQueueManager
}

// IsCritical implements QueueShutdownHandler
// I/O operations are critical - must complete before shutdown
func (m *IOQueueManager) IsCritical() bool {
	return true
}
