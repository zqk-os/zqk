package storage

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
	"github.com/zqk-os/zqk/pkg/when"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// QueueShutdownEventCallback is a callback for emitting shutdown events via coordinator
// This avoids import cycles by using dependency injection
type QueueShutdownEventCallback func(
	ctx context.Context,
	queueName string,
	eventType string, // "initiate", "drain_start", "drain_complete", "drain_error", "shutdown_complete"
	pendingCount int64,
	isCritical bool,
	duration time.Duration,
	err error,
)

var (
	globalQueueShutdownEventCallback atomic.Pointer[QueueShutdownEventCallback]
)

// SetQueueShutdownEventCallback sets the global callback for emitting shutdown events via coordinator
// This should be called during system initialization to wire up coordinator integration
func SetQueueShutdownEventCallback(callback QueueShutdownEventCallback) {
	if callback == nil {
		globalQueueShutdownEventCallback.Store(nil)
		return
	}
	ptr := new(QueueShutdownEventCallback)
	*ptr = callback
	globalQueueShutdownEventCallback.Store(ptr)
}

// getQueueShutdownEventCallback returns the global event callback (if set)
func getQueueShutdownEventCallback() QueueShutdownEventCallback {
	ptr := globalQueueShutdownEventCallback.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

// QueueShutdownHandler is the interface that all queues must implement for graceful shutdown
type QueueShutdownHandler interface {
	// InitiateShutdown signals the queue to stop accepting new operations
	InitiateShutdown() error

	// Drain processes all pending operations and returns when complete
	// Returns error if drain fails or timeout exceeded
	Drain(ctx context.Context) error

	// IsDrained returns true if the queue has no pending operations
	IsDrained() bool

	// GetPendingCount returns the number of pending operations
	GetPendingCount() int64

	// GetName returns the queue name for logging
	GetName() string

	// IsCritical returns true if this queue must drain before force shutdown
	IsCritical() bool
}

// ShutdownConfig configures shutdown behavior
type ShutdownConfig struct {
	Timeout       time.Duration // Max time to wait for drain (default: 30s)
	ForceShutdown bool          // Force shutdown after timeout (default: true)
	LogIncomplete bool          // Log incomplete operations (default: true)
	CheckInterval time.Duration // How often to check drain status (default: 100ms)
}

// DefaultShutdownConfig returns default shutdown configuration
func DefaultShutdownConfig() *ShutdownConfig {
	return &ShutdownConfig{
		Timeout:       30 * time.Second,
		ForceShutdown: true,
		LogIncomplete: true,
		CheckInterval: 100 * time.Millisecond,
	}
}

// QueueShutdownCoordinator manages graceful shutdown of all storage queues.
// It executes a two-phase drain process:
//   - Phase 1: Critical queues (e.g. WAL, transactional persistence) drained sequentially.
//   - Phase 2: Non-critical queues (e.g. audit aggregation, orphan cleanup) drained in parallel.
//
// See docs/architecture/STORAGE_COORDINATION.md for detailed Mermaid sequence and state machine diagrams.
type QueueShutdownCoordinator struct {
	shutdownInitiated int32 // Atomic flag: 1 if shutdown initiated, 0 otherwise
	shutdownComplete  chan struct{}
	shutdownOnce      sync.Once // Ensures shutdownComplete is only closed once
	drainAllOnce      sync.Once // Ensures DrainAll executes at most once per coordinator instance
	queues            []QueueShutdownHandler
	mu                sync.RWMutex
	config            *ShutdownConfig
	logger            *logging.EventLogger
	// testOverride controls whether IsShutdownInitiated should ignore test
	// environment shortcuts and report the real shutdown flag. This is used
	// by unit tests that construct their own coordinator instances and
	// need deterministic visibility into shutdown state, even when
	// ZQK_TEST_ROOT is set globally.
	testOverride bool
}

var (
	globalShutdownCoordinator     *QueueShutdownCoordinator
	globalShutdownCoordinatorOnce sync.Once
)

// GetGlobalShutdownCoordinator returns the global shutdown coordinator (singleton)
func GetGlobalShutdownCoordinator() *QueueShutdownCoordinator {
	globalShutdownCoordinatorOnce.Do(func() {
		globalShutdownCoordinator = &QueueShutdownCoordinator{
			shutdownComplete: make(chan struct{}),
			queues:           make([]QueueShutdownHandler, 0),
			config:           DefaultShutdownConfig(),
			logger:           logging.NewEventLogger(pkgctx.NewSystemContext()),
		}
	})
	return globalShutdownCoordinator
}

// RegisterQueue registers a queue for shutdown coordination
func (c *QueueShutdownCoordinator) RegisterQueue(queue QueueShutdownHandler) {
	StorageLog(c.logger.Logger()).Info(LogEventStorageQueueShutdownRegisteringInfo).QueueName(queue.GetName()).Log()
	_ = concurrency.RunInLockOrLog(
		&c.mu, locknames.LockNameQueueShutdownRegister, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			c.queues = append(c.queues, queue)
			return nil
		},
	)
}

// IsShutdownInitiated returns true if shutdown has been initiated
// In test environments, this always returns false to allow parallel test execution
func (c *QueueShutdownCoordinator) IsShutdownInitiated() bool {
	// In production/global coordinator usage, we suppress the shutdown flag
	// when running under tests (ZQK_TEST_ROOT set) so that parallel tests
	// can continue to enqueue work. However, unit tests that create their
	// own coordinator instances can set testOverride to bypass this behavior
	// and assert directly on the shutdown flag.
	if !c.testOverride {
		if zqkenv.TestRoot().Get() != emptyValue {
			return false
		}
	}
	return atomic.LoadInt32(&c.shutdownInitiated) == 1
}

// InitiateShutdown initiates graceful shutdown of all registered queues
// This prevents new operations from being enqueued
func (c *QueueShutdownCoordinator) InitiateShutdown() error {
	// Set shutdown flag atomically (idempotent)
	if !atomic.CompareAndSwapInt32(&c.shutdownInitiated, 0, 1) {
		return nil // Already shutting down
	}

	StorageLog(c.logger.Logger()).Info(LogEventStorageQueueShutdownInitiatingInfo).Log()

	// Notify all queues to stop accepting new operations
	var queues []QueueShutdownHandler
	_ = concurrency.RunInRLockOrLog(
		&c.mu, locknames.LockNameQueueShutdownInitiateCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			queues = make([]QueueShutdownHandler, len(c.queues))
			copy(queues, c.queues)
			return nil
		},
	)

	for _, queue := range queues {
		startTime := time.Now()
		err := queue.InitiateShutdown()
		when.When(func() bool { return err != nil }).Then(func() {
			StorageLog(c.logger.Logger()).Warn(LogEventStorageQueueShutdownInitiateQueueFailedWarn).
				String("queue", queue.GetName()).
				WithError(err).
				Log()
			c.emitShutdownEvent(pkgctx.NewSystemContext(), queue, ConstMiscInitiateError, 0, time.Since(startTime), err)
		}).OrElse(func() {
			c.emitShutdownEvent(pkgctx.NewSystemContext(), queue, "initiate", queue.GetPendingCount(), time.Since(startTime), nil)
		}).Run()
	}

	return nil
}

// DrainAll drains all registered queues and waits for completion.
// Returns error if drain fails or timeout exceeded.
// Safe to call concurrently: only the first call runs the drain pipeline; subsequent calls
// return the same outcome after waiting for the first (sync.Once semantics).
func (c *QueueShutdownCoordinator) DrainAll(ctx context.Context) error {
	var errOut error
	c.drainAllOnce.Do(func() {
		errOut = c.drainAllInner(ctx)
	})
	return errOut
}

func (c *QueueShutdownCoordinator) drainAllInner(ctx context.Context) error {
	if !c.IsShutdownInitiated() {
		// Initiate shutdown first
		if err := c.InitiateShutdown(); err != nil {
			return errfmt.Newf(ConstMiscFailedToInitiateShutdown).Wrap(err)
		}
	}

	var queues []QueueShutdownHandler
	_ = concurrency.RunInRLockOrLog(
		&c.mu, locknames.LockNameQueueShutdownDrainCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			queues = make([]QueueShutdownHandler, len(c.queues))
			copy(queues, c.queues)
			return nil
		},
	)

	if len(queues) == 0 {
		StorageLog(c.logger.Logger()).Info(LogEventStorageQueueShutdownNoQueuesInfo).Log()
		c.shutdownOnce.Do(func() {
			close(c.shutdownComplete)
		})
		return nil
	}

	queueNames := make([]string, 0, len(queues))
	for _, q := range queues {
		queueNames = append(queueNames, q.GetName())
	}
	StorageLog(c.logger.Logger()).Info(fmt.Sprintf("Draining queues [%s]", strings.Join(queueNames, ", "))).
		Int("queue_count", len(queues)).
		Log()

	// Create context with timeout
	drainCtx, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()

	// Drain all queues in parallel
	var wg sync.WaitGroup
	drainErrors := make(chan error, len(queues))
	var drainErrorsMu sync.Mutex
	drainErrorsClosed := false

	drainBud := goroutinelabels.DefaultBudget()
	for _, queue := range queues {
		queueCopy := queue // Capture for goroutine
		drainBuilder := goroutinelabels.NewGoroutine(ConstMiscQueueShutdownDrain, fmt.Sprintf(ConstMiscDrainingQueueS, queue.GetName())).
			WithWaitGroup(&wg)
		if drainBud != nil {
			drainBuilder = drainBuilder.WithBudget(drainBud)
		}
		drainBuilder.StartSimple(func() {
			// Emit drain start event
			c.emitShutdownEvent(drainCtx, queueCopy, "drain_start", queueCopy.GetPendingCount(), 0, nil)

			drainStart := time.Now()
			drainErr := queueCopy.Drain(drainCtx)
			duration := time.Since(drainStart)
			when.When(func() bool { return drainErr != nil }).Then(func() {
				c.emitShutdownEvent(drainCtx, queueCopy, "drain_error", queueCopy.GetPendingCount(), duration, drainErr)
				_ = concurrency.RunInLockOrLog(
					&drainErrorsMu, locknames.LockNameQueueShutdownSendDrainError, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
					func() error {
						if !drainErrorsClosed {
							select {
							case drainErrors <- errfmt.Newf(ConstMiscQueueSDrainFailed, queueCopy.GetName()).Wrap(drainErr):
							default:
							}
						}
						return nil
					},
				)
			}).OrElse(func() {
				c.emitShutdownEvent(drainCtx, queueCopy, ConstMiscDrainComplete, 0, duration, nil)
			}).Run()
		})
	}

	// Check if context is already cancelled before starting wait
	select {
	case <-drainCtx.Done():
		// Context already cancelled - skip waiting
		StorageLog(c.logger.Logger()).Warn(LogEventStorageQueueShutdownDrainCtxCancelledWarn).Log()
		return drainCtx.Err()
	default:
	}

	waitDone := make(chan struct{})

	// Start goroutine to wait for WaitGroup (BLI-CEF-CON-002)
	goroutinelabels.NewGoroutine("storage", ConstMiscQueueShutdownWaitgroupWaiter).StartSimple(func() {
		wg.Wait()
		close(waitDone)
	})

	select {
	case <-waitDone:
		// All queues drained
		StorageLog(c.logger.Logger()).Info(LogEventStorageQueueShutdownAllDrainedSuccessInfo).Log()
	case <-drainCtx.Done():
		// Timeout exceeded
		if c.config.ForceShutdown {
			StorageLog(c.logger.Logger()).Warn(LogEventStorageQueueShutdownTimeoutForcingWarn).
				String("timeout", c.config.Timeout.String()).
				Log()
			// Log incomplete operations
			if c.config.LogIncomplete {
				c.logIncompleteOperations(queues)
			}
		} else {
			// Collect drain errors
			_ = concurrency.RunInLockOrLog(
				&drainErrorsMu, locknames.LockNameQueueShutdownCloseDrainErrors, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					drainErrorsClosed = true
					close(drainErrors)
					return nil
				},
			)
			var errors []error
			for err := range drainErrors {
				errors = append(errors, err)
			}
			if len(errors) > 0 {
				return errfmt.Errorf(ConstMiscDrainFailedV, errors)
			}
			return errfmt.Errorf(ConstMiscDrainTimeoutExceeded)
		}
	}

	// Check for any drain errors
	_ = concurrency.RunInLockOrLog(
		&drainErrorsMu, locknames.LockNameQueueShutdownFinalizeDrainErrors, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if !drainErrorsClosed {
				close(drainErrors)
				drainErrorsClosed = true
			}
			return nil
		},
	)

	var errors []error
	for err := range drainErrors {
		errors = append(errors, err)
	}

	// Verify all queues are drained
	undrained := make([]string, 0)
	for _, queue := range queues {
		if !queue.IsDrained() {
			undrained = append(undrained, queue.GetName())
		}
	}

	if len(undrained) > 0 {
		StorageLog(c.logger.Logger()).Warn(LogEventStorageQueueShutdownSomeNotDrainedWarn).
			String("queues", fmt.Sprintf("%v", undrained)).
			Log()
	}

	// Check for critical queues that must drain
	criticalUndrained := make([]string, 0)
	for _, queue := range queues {
		if queue.IsCritical() && !queue.IsDrained() {
			criticalUndrained = append(criticalUndrained, queue.GetName())
		}
	}

	if len(criticalUndrained) > 0 && !c.config.ForceShutdown {
		return errfmt.Errorf(ConstMiscCriticalQueuesNotDrainedV, criticalUndrained)
	}

	if len(errors) > 0 {
		StorageLog(c.logger.Logger()).Warn(LogEventStorageQueueShutdownSomeDrainErrorsWarn).
			Int("error_count", len(errors)).
			Log()
	}

	// Emit shutdown complete event
	c.emitShutdownEvent(ctx, nil, ConstMiscShutdownComplete, 0, 0, nil)

	// Close shutdownComplete channel (idempotent - only closes once)
	c.shutdownOnce.Do(func() {
		close(c.shutdownComplete)
	})
	return nil
}

// logIncompleteOperations logs queues with pending operations
func (c *QueueShutdownCoordinator) logIncompleteOperations(queues []QueueShutdownHandler) {
	for _, queue := range queues {
		pending := queue.GetPendingCount()
		if pending > 0 {
			StorageLog(c.logger.Logger()).Warn(LogEventStorageQueueShutdownIncompleteOpsWarn).
				String("queue", queue.GetName()).
				Int("pending_count", int(pending)).
				Bool("critical", queue.IsCritical()).
				Log()
		}
	}
}

// WaitForShutdown waits for shutdown to complete
func (c *QueueShutdownCoordinator) WaitForShutdown() <-chan struct{} {
	return c.shutdownComplete
}

// SetConfig updates the shutdown configuration
func (c *QueueShutdownCoordinator) SetConfig(config *ShutdownConfig) {
	_ = concurrency.RunInLockOrLog(
		&c.mu, locknames.LockNameQueueShutdownSetConfig, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			c.config = config
			return nil
		},
	)
}

// emitShutdownEvent emits a shutdown event via coordinator callback
func (c *QueueShutdownCoordinator) emitShutdownEvent(
	ctx context.Context,
	queue QueueShutdownHandler,
	eventType string,
	pendingCount int64,
	duration time.Duration,
	err error,
) {
	callback := getQueueShutdownEventCallback()
	if callback == nil {
		return // No callback set - skip
	}

	var queueName string
	var isCritical bool
	when.When(func() bool { return queue != nil }).Then(func() {
		queueName = queue.GetName()
		isCritical = queue.IsCritical()
	}).OrElse(func() {
		queueName = "all_queues"
		isCritical = false
	}).Run()

	// Emit via callback (async, non-blocking)
	eventBud := goroutinelabels.DefaultBudget()
	eventBuilder := goroutinelabels.NewGoroutine(ConstMiscQueueShutdownEvent, fmt.Sprintf(ConstMiscEmittingShutdownEventSForS, eventType, queueName))
	if eventBud != nil {
		eventBuilder = eventBuilder.WithBudget(eventBud)
	}
	eventBuilder.StartSimple(func() {
		callback(ctx, queueName, eventType, pendingCount, isCritical, duration, err)
	})
}
