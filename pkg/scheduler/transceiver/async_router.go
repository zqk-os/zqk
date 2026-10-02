package transceiver

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
	"github.com/zqk-os/zqk/pkg/scheduler/transceiver/types"
	"github.com/zqk-os/zqk/pkg/when"
)

const (
	// asyncRouterIdleTimeout is the time a worker waits idle before shutting down
	asyncRouterIdleTimeout = 5 * time.Minute
	// asyncRouterCheckInterval is how often workers check for work when idle
	asyncRouterCheckInterval = 1 * time.Second
)

// AsyncRouterEventCallback is a callback for emitting router events via coordinator
// This avoids import cycles by using dependency injection
type AsyncRouterEventCallback func(
	ctx context.Context,
	projectRoot string,
	storageProvider any, // ObjectStorageProvider (using any to avoid import cycles)
	workerID string,
	eventType string, // "worker_start", "worker_shutdown", "worker_idle_shutdown"
	status string,
	workerCount int,
	processedCount int,
	failedCount int,
	duration time.Duration,
)

var (
	globalAsyncRouterEventCallback AsyncRouterEventCallback
	globalAsyncRouterCallbackMu    sync.RWMutex
)

// SetAsyncRouterEventCallback sets the global callback for emitting router events via coordinator
func SetAsyncRouterEventCallback(callback AsyncRouterEventCallback) {
	_ = concurrency.RunInLockWithLogger(
		&globalAsyncRouterCallbackMu,
		LockNameAsyncRouterSetCallback,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			globalAsyncRouterEventCallback = callback
			return nil
		},
	)
}

// getAsyncRouterEventCallback returns the global event callback (if set)
func getAsyncRouterEventCallback() AsyncRouterEventCallback {
	var callback AsyncRouterEventCallback
	_ = concurrency.RunInRLockWithLogger(
		&globalAsyncRouterCallbackMu,
		LockNameAsyncRouterGetCallback,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			callback = globalAsyncRouterEventCallback
			return nil
		},
	)
	return callback
}

// AsyncRouter wraps Router with async execution and on-demand worker management
// Implements the "On-Demand Worker" pattern (wake-on-work with idle shutdown)
type AsyncRouter struct {
	router          *Router
	logger          logging.Logger
	queue           chan routingTask
	shutdown        chan struct{}
	wg              sync.WaitGroup
	started         bool
	startedMu       sync.RWMutex
	metrics         *RouterMetrics
	maxWorkers      int
	activeWorkers   atomic.Int32 // Atomic counter for active workers
	workerMu        sync.Mutex   // Protects worker startup
	ctx             context.Context
	cancel          context.CancelFunc
	projectRoot     string    // For coordinator events
	storageProvider any       // For coordinator events (ObjectStorageProvider, using any to avoid import cycles)
	stopOnce        sync.Once // Ensures Stop() only runs once
}

// routingTask represents a message routing task
type routingTask struct {
	ctx     context.Context
	message types.Message
}

// NewAsyncRouter creates a new async router with on-demand worker management
// NewAsyncRouter creates a new async router
// ctx: parent context from command entry point (should not be created here)
func NewAsyncRouter(ctx context.Context, router *Router, maxWorkers, queueSize int, logger logging.Logger) *AsyncRouter {
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	if maxWorkers <= 0 {
		// Use global concurrency config for smart defaults
		cfg := concurrency.GetGlobalConcurrencyConfig()
		maxWorkers = cfg.AsyncRouterMaxWorkers
	}
	if queueSize <= 0 {
		queueSize = 100 // Default queue size
	}

	// Derive cancellation context from parent (command context)
	ctx, cancel := context.WithCancel(ctx) //nolint:gosec // G118: cancel stored on AsyncRouter; invoked from Stop

	ar := &AsyncRouter{
		router:     router,
		logger:     logger,
		queue:      make(chan routingTask, queueSize),
		shutdown:   make(chan struct{}),
		metrics:    router.GetMetrics(),
		maxWorkers: maxWorkers,
		ctx:        ctx,
		cancel:     cancel,
	}

	// Initialize shutdown channel (not closed initially)
	// This will be closed in Stop() or InitiateShutdown()

	return ar
}

// SetProjectRoot sets the project root for coordinator events
func (ar *AsyncRouter) SetProjectRoot(projectRoot string) {
	_ = concurrency.RunInLockWithLogger(
		&ar.startedMu,
		LockNameAsyncRouterSetProjectRoot,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			ar.projectRoot = projectRoot
			return nil
		},
	)
}

// SetStorageProvider sets the storage provider for coordinator events
func (ar *AsyncRouter) SetStorageProvider(storageProvider any) {
	_ = concurrency.RunInLockWithLogger(
		&ar.startedMu,
		LockNameAsyncRouterSetStorageProvider,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			ar.storageProvider = storageProvider
			return nil
		},
	)
}

// Start starts the async router (workers start on-demand)
func (ar *AsyncRouter) Start(ctx context.Context) error {
	var alreadyStarted bool
	err := concurrency.RunInLockWithLogger(
		&ar.startedMu,
		LockNameAsyncRouterStart,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if ar.started {
				alreadyStarted = true
				return nil
			}
			ar.started = true
			return nil
		},
	)
	if err != nil {
		return errfmt.Newf("failed to start async router").Wrap(err)
	}
	if alreadyStarted {
		return errfmt.Errorf("async router already started")
	}
	logging.Fluent(ar.logger).Info(LogEventSchedulerTransceiverAsyncRouterStarting).
		MaxWorkers(ar.maxWorkers).
		QueueSize(cap(ar.queue)).
		Log()

	// Workers start on-demand when messages are enqueued
	// No workers are started here (on-demand pattern)

	return nil
}

// Stop stops the async router and waits for in-flight tasks to complete
func (ar *AsyncRouter) Stop() error {
	var stopErr error
	ar.stopOnce.Do(func() {
		var wasStarted bool
		err := concurrency.RunInLockWithLogger(
			&ar.startedMu,
			LockNameAsyncRouterStop,
			logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				if !ar.started {
					return nil
				}
				wasStarted = true
				ar.started = false
				return nil
			},
		)
		if err != nil {
			stopErr = errfmt.Newf("failed to stop async router").Wrap(err)
			return
		}

		if !wasStarted {
			return
		}

		logging.Fluent(ar.logger).Info(LogEventSchedulerTransceiverAsyncRouterStopping).Log()

		// Signal shutdown
		ar.cancel()

		// Close shutdown channel (idempotent - check if already closed)
		select {
		case <-ar.shutdown:
			// Already closed - do nothing
		default:
			close(ar.shutdown)
		}

		// Close queue to unblock workers
		func() {
			defer func() {
				// Recover from panic if channel already closed
				if r := recover(); r != nil {
					// Channel already closed - this is fine
				}
			}()
			close(ar.queue)
		}()

		// Wait for workers with timeout
		stopComplete := make(chan struct{})
		stopCtx, stopCancel := context.WithTimeout(ar.ctx, 2*time.Second)
		defer stopCancel()

		// Wait for WaitGroup in a goroutine (can't be cancelled, but we'll timeout)
		// Use goroutinelabels for proper labeling, but don't try to cancel the wait
		stopBud := goroutinelabels.DefaultBudget()
		stopWaitBuilder := goroutinelabels.NewGoroutine("async_router_stop_wait", "waiting for async router workers to stop")
		if stopBud != nil {
			stopWaitBuilder = stopWaitBuilder.WithBudget(stopBud)
		}
		stopWaitBuilder.StartWithContext(stopCtx, func(ctx context.Context) error {
			ar.wg.Wait()
			// Workers completed - signal completion
			select {
			case <-stopComplete:
				// Already closed (timeout occurred)
			default:
				close(stopComplete)
			}
			return nil
		})

		select {
		case <-stopComplete:
			// Workers stopped successfully
			logging.Fluent(ar.logger).Info(LogEventSchedulerTransceiverAsyncRouterAllWorkersStopped).Log()
		case <-stopCtx.Done():
			// Timeout - log warning but continue shutdown
			// Note: The wait goroutine will continue running until wg.Wait() completes,
			// but we don't wait for it since we've timed out. This is acceptable as the
			// WaitGroup will eventually complete when workers finish.
			logging.Fluent(ar.logger).Warn(LogEventSchedulerTransceiverAsyncRouterWorkerStopTimeout).Log()
		}

		logging.Fluent(ar.logger).Info(LogEventSchedulerTransceiverAsyncRouterStopped).Log()
	})
	return stopErr
}

// RouteAsync routes a message asynchronously (non-blocking)
// Returns immediately after queuing the message
//
//nolint:gocritic // Message passed by value to avoid shared mutation across goroutines
func (ar *AsyncRouter) RouteAsync(ctx context.Context, message types.Message) error {
	return ar.RouteAsyncWithTimeout(ctx, message, 100*time.Millisecond)
}

// RouteAsyncWithTimeout routes a message asynchronously with custom timeout
//
//nolint:gocritic // Message passed by value to avoid shared mutation across goroutines
func (ar *AsyncRouter) RouteAsyncWithTimeout(ctx context.Context, message types.Message, timeout time.Duration) error {
	var started bool
	_ = concurrency.RunInRLockWithLogger(
		&ar.startedMu,
		LockNameAsyncRouterRouteCheck,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			started = ar.started
			return nil
		},
	)

	if !started {
		return errfmt.Errorf("async router not started")
	}

	// Check if shutdown has been initiated (via QueueShutdownHandler)
	select {
	case <-ar.shutdown:
		return errfmt.Errorf("async router is shutting down")
	default:
		// Continue
	}

	// Create routing task
	task := routingTask{
		ctx:     ctx,
		message: message,
	}

	// Try to queue (non-blocking with timeout)
	select {
	case ar.queue <- task:
		// Update queue depth metric
		if ar.metrics != nil {
			ar.metrics.UpdateQueueDepth(int64(len(ar.queue)))
		}
		logging.Fluent(ar.logger).Debug(LogEventSchedulerTransceiverAsyncRouterMessageQueued).
			EventType(message.EventType).
			Log()

		// Wake worker if needed (on-demand pattern)
		ar.wakeWorkerIfNeeded()

		return nil
	case <-time.After(timeout):
		// Queue full or router shutting down
		if ar.metrics != nil {
			ar.metrics.RecordQueueFull()
			ar.metrics.RecordMessageDropped()
		}
		logging.Fluent(ar.logger).Warn(LogEventSchedulerTransceiverAsyncRouterQueueFailed).
			EventType(message.EventType).
			Timeout(timeout.String()).
			Log()
		return errfmt.Errorf("failed to queue message: queue full or timeout")
	case <-ar.shutdown:
		if ar.metrics != nil {
			ar.metrics.RecordMessageDropped()
		}
		return errfmt.Errorf("async router is shutting down")
	}
}

// RouteSync routes a message synchronously (blocking)
// Useful for testing or when you need to wait for completion
//
//nolint:gocritic // Message passed by value to avoid shared mutation across goroutines
func (ar *AsyncRouter) RouteSync(ctx context.Context, message types.Message) error {
	return ar.router.Route(ctx, message)
}

// wakeWorkerIfNeeded starts a worker if there's work and we're under max workers
func (ar *AsyncRouter) wakeWorkerIfNeeded() {
	// Check if we need to start a worker
	currentWorkers := int(ar.activeWorkers.Load())
	if currentWorkers >= ar.maxWorkers {
		return // Already at max workers
	}

	// Try to start a worker (atomic check-and-set)
	_ = concurrency.RunInLockWithLogger(
		&ar.workerMu,
		LockNameAsyncRouterWakeWorker,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Double-check after acquiring lock
			currentWorkers = int(ar.activeWorkers.Load())
			if currentWorkers >= ar.maxWorkers {
				return nil
			}

			// Check if there's work to do
			if len(ar.queue) == 0 {
				return nil // No work available
			}

			// Start a new worker
			workerID := currentWorkers
			ar.activeWorkers.Add(1)

			// Emit worker start event
			callback := getAsyncRouterEventCallback()
			if callback != nil && ar.projectRoot != emptyValue {
				var projectRoot string
				var storageProvider any
				_ = concurrency.RunInRLockWithLogger(
					&ar.startedMu,
					LockNameAsyncRouterWorkerStartEvent,
					logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
					func() error {
						projectRoot = ar.projectRoot
						storageProvider = ar.storageProvider
						return nil
					},
				)
				// Use router's context (or derived context) instead of creating new Background()
				callback(
					ar.ctx,
					projectRoot,
					storageProvider,
					fmt.Sprintf("worker_%d", workerID),
					"worker_start",
					"started",
					int(ar.activeWorkers.Load()),
					0,
					0,
					0,
				)
			}

			workerBud := goroutinelabels.DefaultBudget()
			workerBuilder := goroutinelabels.NewGoroutine(fmt.Sprintf("async_router_worker_%d", workerID), fmt.Sprintf("processing routing tasks (worker %d)", workerID)).
				WithWaitGroup(&ar.wg)
			if workerBud != nil {
				workerBuilder = workerBuilder.WithBudget(workerBud)
			}
			workerBuilder.StartWithContext(ar.ctx, func(ctx context.Context) error {
				ar.worker(workerID)
				return nil
			})
			return nil
		},
	)
}

// worker processes routing tasks from the queue
// Implements on-demand pattern: processes work, then shuts down after idle timeout
func (ar *AsyncRouter) worker(workerID int) {
	defer func() {
		ar.activeWorkers.Add(-1)
		// Note: ar.wg.Done() is NOT called here because WithWaitGroup() in wakeWorkerIfNeeded()
		// automatically calls Done() when the goroutine exits. Calling it here would cause
		// a double Done() and panic with "negative WaitGroup counter".

		// Emit worker shutdown event
		callback := getAsyncRouterEventCallback()
		if callback != nil && ar.projectRoot != emptyValue {
			var projectRoot string
			var storageProvider any
			_ = concurrency.RunInRLockWithLogger(
				&ar.startedMu,
				LockNameAsyncRouterWorkerShutdownEvent,
				logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					projectRoot = ar.projectRoot
					storageProvider = ar.storageProvider
					return nil
				},
			)
			// Use router's context (or derived context) instead of creating new Background()
			callback(
				ar.ctx,
				projectRoot,
				storageProvider,
				fmt.Sprintf("worker_%d", workerID),
				"worker_shutdown",
				"stopped",
				int(ar.activeWorkers.Load()),
				0,
				0,
				0,
			)
		}
	}()

	startTime := time.Now()
	idleStartTime := startTime
	var processedCount, failedCount int

	for {
		select {
		case <-ar.ctx.Done():
			return
		case <-ar.shutdown:
			return
		default:
			// Try to get a task from queue
			select {
			case task, ok := <-ar.queue:
				if !ok {
					// Channel closed
					return
				}

				// Reset idle timer when we get work
				idleStartTime = time.Now()

				// Update queue depth metric
				if ar.metrics != nil {
					ar.metrics.UpdateQueueDepth(int64(len(ar.queue)))
				}

				// Execute routing task
				err := ar.router.Route(task.ctx, task.message)
				when.When(func() bool { return err != nil }).Then(func() {
					failedCount++
					logging.Fluent(ar.logger).Warn(LogEventSchedulerTransceiverAsyncRouterRoutingFailed).
						EventType(task.message.EventType).
						WithError(err).
						Log()
				}).OrElse(func() {
					processedCount++
					logging.Fluent(ar.logger).Debug(LogEventSchedulerTransceiverAsyncRouterRoutingCompleted).
						EventType(task.message.EventType).
						Log()
				}).Run()

				// Wake another worker if there's more work
				ar.wakeWorkerIfNeeded()

			default:
				// No tasks available - check idle timeout
				idleDuration := time.Since(idleStartTime)
				if idleDuration >= asyncRouterIdleTimeout {
					// Idle timeout reached - shut down worker
					logging.Fluent(ar.logger).Info(LogEventSchedulerTransceiverAsyncRouterWorkerIdleShutdown).
						WorkerID(workerID).
						IdleDuration(idleDuration.String()).
						ProcessedCount(processedCount).
						FailedCount(failedCount).
						Log()

					// Emit worker idle shutdown event
					callback := getAsyncRouterEventCallback()
					if callback != nil && ar.projectRoot != emptyValue {
						var projectRoot string
						var storageProvider any
						_ = concurrency.RunInRLockWithLogger(
							&ar.startedMu,
							LockNameAsyncRouterWorkerIdleShutdownEvent,
							logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
							func() error {
								projectRoot = ar.projectRoot
								storageProvider = ar.storageProvider
								return nil
							},
						)
						// Use router's context (or derived context) instead of creating new Background()
						callback(
							ar.ctx,
							projectRoot,
							storageProvider,
							fmt.Sprintf("worker_%d", workerID),
							"worker_idle_shutdown",
							"idle_shutdown",
							int(ar.activeWorkers.Load()),
							processedCount,
							failedCount,
							time.Since(startTime),
						)
					}
					return
				}

				// Wait a bit before checking again (with cancellation check)
				select {
				case <-ar.ctx.Done():
					return
				case <-ar.shutdown:
					return
				case <-time.After(asyncRouterCheckInterval):
					// Continue loop to check idle timeout
				}
			}
		}
	}
}

// GetQueueSize returns the current queue size
func (ar *AsyncRouter) GetQueueSize() int {
	return len(ar.queue)
}

// GetQueueCapacity returns the queue capacity
func (ar *AsyncRouter) GetQueueCapacity() int {
	return cap(ar.queue)
}

// GetMetrics returns the router metrics
func (ar *AsyncRouter) GetMetrics() *RouterMetrics {
	return ar.metrics
}

// GetActiveWorkers returns the number of active workers
func (ar *AsyncRouter) GetActiveWorkers() int {
	return int(ar.activeWorkers.Load())
}

// IsWorkerRunning returns true if any workers are currently running
func (ar *AsyncRouter) IsWorkerRunning() bool {
	return ar.activeWorkers.Load() > 0
}

// InitiateShutdown implements QueueShutdownHandler
// Signals the router to stop accepting new messages
func (ar *AsyncRouter) InitiateShutdown() error {
	var wasStarted bool
	_ = concurrency.RunInLockWithLogger(
		&ar.startedMu,
		LockNameAsyncRouterInitiateShutdown,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if !ar.started {
				return nil
			}
			wasStarted = true
			return nil
		},
	)

	if !wasStarted {
		return nil // Already stopped
	}

	// Signal shutdown (prevents new messages from being queued)
	select {
	case <-ar.shutdown:
		// Already closed
	default:
		close(ar.shutdown)
	}

	return nil
}

// Drain implements QueueShutdownHandler
// Processes all pending messages and waits for workers to complete
func (ar *AsyncRouter) Drain(ctx context.Context) error {
	if err := ar.InitiateShutdown(); err != nil {
		return err
	}

	// Wait for workers with timeout
	drainComplete := make(chan struct{})

	// Use callback pattern: wait in goroutine, notify via channel
	// The goroutine will exit when the context is cancelled
	drainBud := goroutinelabels.DefaultBudget()
	drainWaitBuilder := goroutinelabels.NewGoroutine("async_router_drain_wait", "waiting for async router workers to drain").
		WithCleanup(func() {
			select {
			case <-drainComplete:
				// Already closed
			default:
				close(drainComplete)
			}
		})
	if drainBud != nil {
		drainWaitBuilder = drainWaitBuilder.WithBudget(drainBud)
	}
	drainWaitBuilder.StartWithContext(ctx, func(ctx context.Context) error {
		// Wait for workers to complete, but check context periodically
		done := make(chan struct{})
		innerBud := goroutinelabels.DefaultBudget()
		innerBuilder := goroutinelabels.NewGoroutine("async_router_drain_wait_inner", "waiting for WaitGroup in drain operation")
		if innerBud != nil {
			innerBuilder = innerBuilder.WithBudget(innerBud)
		}
		innerBuilder.StartWithContext(ctx, func(ctx context.Context) error {
			ar.wg.Wait()
			select {
			case <-done:
				// Already closed
			default:
				close(done)
			}
			return nil
		})

		select {
		case <-done:
			// Workers completed
			select {
			case <-drainComplete:
			default:
				close(drainComplete)
			}
		case <-ctx.Done():
			// Timeout or context cancelled - exit without closing drainComplete
			return ctx.Err()
		}
		return nil
	})

	select {
	case <-drainComplete:
		// All workers finished
		return nil
	case <-ctx.Done():
		// Timeout or context cancelled (timeout is fallback)
		return ctx.Err()
	}
}

// IsDrained implements QueueShutdownHandler
func (ar *AsyncRouter) IsDrained() bool {
	// Check if queue is empty and no workers are running
	return len(ar.queue) == 0 && ar.activeWorkers.Load() == 0
}

// GetPendingCount implements QueueShutdownHandler
func (ar *AsyncRouter) GetPendingCount() int64 {
	return int64(len(ar.queue))
}

// GetName implements QueueShutdownHandler
func (ar *AsyncRouter) GetName() string {
	return "async_router"
}

// IsCritical implements QueueShutdownHandler
// Async router is not critical - messages can be dropped during shutdown
func (ar *AsyncRouter) IsCritical() bool {
	return false
}
