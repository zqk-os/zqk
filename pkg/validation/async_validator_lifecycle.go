package validation

import (
	"context"
	"runtime"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
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

	// Prefer the per-project shared validation cache so system check Sets and
	// create/update/delete invalidations persist the same in-memory map.
	// A private cache here allowed the shared flusher (often ~tens of CRUD
	// entries) to overwrite a full check Save on disk and permanently starve
	// the warm path (cache_hits≈0).
	stateCache := NewValidationStateCache(projectRoot, maxCacheAge)
	if sc := getSharedValidationCache(projectRoot); sc != nil && sc.cache != nil {
		sc.cache.maxAge = maxCacheAge
		stateCache = sc.cache
	}

	return &AsyncValidator{
		stateCache:        stateCache,
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
		// Bound concurrent validations. Coupling to workers alone still left NumCPU*2 (~20)
		// under the 5s fail-fast budget; 8 was a timeout workaround that made syschk ~6 min
		// for 8k objects. Cap at 16; nested validate still holds the slot until done
		// (no timeout thread leak). Hostload refuses extra slots under foreign pressure.
		// TRACK: BLI-1785895580100186000-c5539372
		validationSemaphore:        make(chan struct{}, validationSemaphoreCapacity(workers)),
		lastQueueSize:              0,
		lastSemaphoreFullQueueSize: 0,
		shutdown:                   make(chan struct{}),
		enqueuedObjectIDs:          make(map[string]bool),
	}
}

// validationSemaphoreCapacity chooses concurrent validation slots for fail-fast budgets.
func validationSemaphoreCapacity(workers int) int {
	const failFastCap = 16
	capN := maxInt(4, min(workers, runtime.NumCPU()*2))
	if capN > failFastCap {
		return failFastCap
	}
	return capN
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
		var workerTimeout, cacheTimeout time.Duration
		if err := concurrency.RunInLockWithLogger(
			&av.mu,
			LockNameAsyncValidatorStopInit,
			lockLoggerSystem(),
			func() error {
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
