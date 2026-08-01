package validation

import (
	"context"
	"runtime"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
)

// NewAsyncValidator creates a new async validator.
// ctx: parent context from command entry point (should not be created here).
// opts: optional config; if provided, timeouts and progress channel size come from it (see AsyncValidatorConfig).
func NewAsyncValidator(ctx context.Context, projectRoot string, workers int, maxCacheAge time.Duration, opts ...*AsyncValidatorConfig) *AsyncValidator {
	ctx, cancel := context.WithCancel(ctx) //nolint:gosec // G118: cancel stored on AsyncValidator; invoked from Stop/shutdown paths

	cfg := DefaultAsyncValidatorConfig()
	if len(opts) > 0 && opts[0] != nil {
		cfg = opts[0]
		cfg.applyDefaults()
	}

	// Use component-based logging to route worker logs to a separate file
	// This keeps the main debug log cleaner and groups worker logs together
	decisionCtx := pkgctx.NewLoggingDecisionContext().
		WithLoggingContext(pkgctx.NewLoggingContext(pkgctx.ProfileSystem)).
		WithComponent("validation") // Routes to .zqk/logs/components/validation-events.json (not log-events.json)
	logger := logging.GetLoggerFromDecisionContext(decisionCtx, projectRoot)

	return &AsyncValidator{
		stateCache:        NewValidationStateCache(projectRoot, maxCacheAge),
		priorityQueue:     NewPriorityQueue(),
		maxWorkers:        workers,
		projectRoot:       projectRoot,
		logger:            logger,
		ctx:               ctx,
		cancel:            cancel,
		workerStopTimeout: cfg.WorkerStopTimeout,
		cacheSaveTimeout:  cfg.CacheSaveTimeout,
		progressChan:      make(chan ValidationProgress, cfg.ProgressChannelSize),
		taskAvailable:     make(chan struct{}, 1), // Buffered channel for non-blocking notification
		// Limit validation goroutines to prevent thread proliferation
		// Use NumCPU * 2 to allow some parallelism while preventing unbounded growth
		validationSemaphore: make(chan struct{}, maxInt(4, runtime.NumCPU()*2)),
		lastQueueSize:       0,                     // Initialize to track queue size transitions
		shutdown:            make(chan struct{}),   // Initialize shutdown channel (not closed initially)
		enqueuedObjectIDs:   make(map[string]bool), // Dedupe: skip re-enqueue of same ObjectID across batches
	}
}

// SetTimeouts configures timeout values for worker stop and cache save operations
func (av *AsyncValidator) SetTimeouts(workerStopTimeout, cacheSaveTimeout time.Duration) {
	if err := concurrency.RunInLockWithLogger(
		&av.mu,
		LockNameAsyncValidatorSetTimeouts,
		lockLoggerSystem(),
		func() error {
			if workerStopTimeout > 0 {
				av.workerStopTimeout = workerStopTimeout
			}
			if cacheSaveTimeout > 0 {
				av.cacheSaveTimeout = cacheSaveTimeout
			}
			return nil
		},
	); err != nil {
		logging.Fluent(av.logger).Error(ErrMsgSetTimeouts, err).Log()
	}
}

// Start starts the async validator (workers start on-demand)
func (av *AsyncValidator) Start() error {
	var alreadyRunning bool
	if err := concurrency.RunInLockWithLogger(
		&av.mu,
		LockNameAsyncValidatorStart,
		lockLoggerSystem(),
		func() error {
			if av.running {
				alreadyRunning = true
				return nil
			}
			av.running = true
			av.workerStopTimedOut.Store(false)
			av.cacheSaveTimedOut.Store(false)
			return nil
		},
	); err != nil {
		return err
	}

	if alreadyRunning {
		return ErrAlreadyRunning
	}

	// Load validation state cache (NO LOCK HELD during I/O)
	if err := av.stateCache.Load(); err != nil {
		logging.Fluent(av.logger).Warn(ErrMsgLoadCache).
			WithError(err).
			Log()
	}

	// Get semaphore capacity for logging
	semaphoreCapacity := cap(av.validationSemaphore)
	logging.Fluent(av.logger).Info(LogMsgValStarted).
		MaxWorkers(av.maxWorkers).
		SemaphoreCapacity(semaphoreCapacity).
		ActiveGoroutines(int(getActiveGoroutines())).
		WorkerStopTimeout(av.workerStopTimeout.String()).
		CacheSaveTimeout(av.cacheSaveTimeout.String()).
		Log()
	// Workers start on-demand when tasks are enqueued
	// No workers are started here (on-demand pattern)
	return nil
}

// Stop stops the async validator workers. Shutdown is modeled as a pkg/pipeline sequence
// (see async_validator_stop_pipeline.go): wait workers → persist cache → close progress.
func (av *AsyncValidator) Stop() error {
	var stopErr error
	av.stopOnce.Do(func() {
		var wasRunning bool
		var workerTimeout, cacheTimeout time.Duration
		if err := concurrency.RunInLockWithLogger(
			&av.mu,
			LockNameAsyncValidatorStopInit,
			lockLoggerSystem(),
			func() error {
				if !av.running {
					return nil
				}
				wasRunning = true
				av.cancel()
				av.running = false

				// Signal shutdown (prevents new workers from starting)
				select {
				case <-av.shutdown:
					// Already closed
				default:
					close(av.shutdown)
				}

				// Get timeout values (read while holding lock)
				workerTimeout = av.workerStopTimeout
				cacheTimeout = av.cacheSaveTimeout
				return nil
			},
		); err != nil {
			logging.Fluent(av.logger).Error(ErrMsgInitStopSeq, err).Log()
		}

		if !wasRunning {
			return
		}

		stopErr = runAsyncValidatorStopPipeline(workerTimeout, cacheTimeout, av)
	})
	return stopErr
}

// RunIssues holds flags set during Stop() when timeouts occur so the caller can show a visible summary.
type RunIssues struct {
	WorkerStopTimedOut bool
	CacheSaveTimedOut  bool
}

// GetRunIssues returns run issues from the last Stop() (e.g. timeouts). Call after Stop() to include in user-visible output.
func (av *AsyncValidator) GetRunIssues() RunIssues {
	return RunIssues{
		WorkerStopTimedOut: av.workerStopTimedOut.Load(),
		CacheSaveTimedOut:  av.cacheSaveTimedOut.Load(),
	}
}
