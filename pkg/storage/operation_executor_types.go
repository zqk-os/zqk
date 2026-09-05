package storage

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage/locknames"
)

const (
	// operationExecutorIdleTimeout is the time to wait with empty queue before shutting down workers
	operationExecutorIdleTimeout = 5 * time.Minute
	// operationExecutorCheckInterval is how often workers check for work when idle
	operationExecutorCheckInterval = 1 * time.Second
)

// OperationExecutorEventCallback is a callback for emitting events via coordinator
// This avoids import cycles by using dependency injection
type OperationExecutorEventCallback func(
	ctx context.Context,
	projectRoot string,
	storage ObjectStorageProvider,
	operationID string,
	operationType string,
	status string,
	workerCount int,
	processedCount int,
	failedCount int,
	duration time.Duration,
)

var (
	globalOperationExecutorEventCallback OperationExecutorEventCallback
	globalOperationExecutorCallbackMu    sync.RWMutex
)

// SetOperationExecutorEventCallback sets the global callback for emitting events via coordinator
// This should be called during system initialization to wire up coordinator integration
func SetOperationExecutorEventCallback(callback OperationExecutorEventCallback) {
	_ = concurrency.RunInLockOrLog(&globalOperationExecutorCallbackMu, locknames.LockNameOperationExecutorSetCallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		globalOperationExecutorEventCallback = callback
		return nil
	})
}

// getOperationExecutorEventCallback returns the global event callback (if set)
func getOperationExecutorEventCallback() OperationExecutorEventCallback {
	var callback OperationExecutorEventCallback
	_ = concurrency.RunInRLockOrLog(&globalOperationExecutorCallbackMu, locknames.LockNameOperationExecutorGetCallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		callback = globalOperationExecutorEventCallback
		return nil
	})
	return callback
}

// OperationExecutor executes operations from the queue with enhanced features:
// - Timeout handling for all I/O operations
// - Retry logic with configurable retry config
// - Cache-aware operations
// - Background cache invalidation with consistency reporting
// - On-demand worker pattern (wake-on-work with idle shutdown)
type OperationExecutor struct {
	storage          ObjectStorageProvider
	queue            *OperationQueue
	maxWorkers       int               // Maximum number of workers
	activeWorkers    atomic.Int32      // Atomic counter for active workers
	wgManager        *WaitGroupManager // Centralized WaitGroup management
	ctx              context.Context
	cancel           context.CancelFunc
	logger           *logging.EventLogger
	conflictResolver ConflictResolver
	projectRoot      string

	// Enhanced features
	ioTimeout               time.Duration
	retryConfig             *RetryConfig
	cacheManager            *CacheManager
	pendingInvalidations    map[string]*PendingInvalidation
	invalidationMu          sync.RWMutex
	operationsExecutedTotal atomic.Int64
	operationsFailedTotal   atomic.Int64

	// On-demand pattern state
	workerMu sync.Mutex // Protects worker state transitions
}

// ConflictResolver resolves conflicts between operations
type ConflictResolver interface {
	ResolveConflict(ctx context.Context, op *Operation, conflict *Conflict) (ResolutionStrategy, error)
}

// ResolutionStrategy defines how to resolve a conflict
type ResolutionStrategy string

const (
	StrategyRetry  ResolutionStrategy = "retry"
	StrategyMerge  ResolutionStrategy = "merge"
	StrategyReject ResolutionStrategy = "reject"
	StrategySkip   ResolutionStrategy = "skip"
	StrategyQueue  ResolutionStrategy = "queue"
)

// Conflict represents a conflict between operations
type Conflict struct {
	Type        string // "version", "dependency", "concurrent"
	OperationID string
	ObjectID    string
	Message     string
	Details     map[string]any
}

// ConsistencyStatus represents cache consistency status
type ConsistencyStatus struct {
	IsConsistent         bool
	PendingCount         int
	PendingInvalidations []string
	Warnings             []string
	LastChecked          time.Time
}

// PendingInvalidation tracks a cache invalidation in progress
type PendingInvalidation struct {
	ID          string
	ObjectIDs   []string
	Reason      string
	Status      string // pending, processing, completed, failed
	StartedAt   time.Time
	CompletedAt *time.Time
	Error       error
}
