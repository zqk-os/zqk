package validation

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
)

// ErrAlreadyRunning is returned when attempting to start an already running async validator
var ErrAlreadyRunning = errfmt.Errorf("async validator is already running")

const (
	// asyncValidatorIdleTimeout is the time a worker waits idle before shutting down
	asyncValidatorIdleTimeout = 5 * time.Minute
	// asyncValidatorCheckInterval is how often workers check for work when idle
	asyncValidatorCheckInterval = 1 * time.Second
)

// Global goroutine tracking for debugging
var (
	activeGoroutines atomic.Int64 // Atomic counter for active goroutines
	goroutineCounter atomic.Int64 // Atomic counter for unique goroutine IDs
)

// GetGoroutineID returns a unique ID for the current goroutine
func GetGoroutineID() int64 {
	return goroutineCounter.Add(1)
}

// IncrementActiveGoroutines increments the active goroutine counter
func IncrementActiveGoroutines() int64 {
	return activeGoroutines.Add(1)
}

// DecrementActiveGoroutines decrements the active goroutine counter
func DecrementActiveGoroutines() int64 {
	return activeGoroutines.Add(-1)
}

// GetActiveGoroutines returns the current count of active goroutines
func GetActiveGoroutines() int64 {
	return activeGoroutines.Load()
}

// getGoroutineID returns a unique ID for the current goroutine (internal alias)
func getGoroutineID() int64 {
	return GetGoroutineID()
}

// incrementActiveGoroutines increments the active goroutine counter (internal alias)
func incrementActiveGoroutines() int64 {
	return IncrementActiveGoroutines()
}

// decrementActiveGoroutines decrements the active goroutine counter (internal alias)
//
//nolint:unparam // Return value is intentionally unused - function is an alias
func decrementActiveGoroutines() int64 {
	DecrementActiveGoroutines()
	return 0
}

// getActiveGoroutines returns the current count of active goroutines (internal alias)
func getActiveGoroutines() int64 {
	return GetActiveGoroutines()
}

// maxInt returns the maximum of two integers
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ValidationFunc is a function that performs full validation on an object
// It takes objectID, objectKind, filePath, and file content, and returns
// a ValidationState with all issues found. This allows the async validator
// to use the same validation logic as sync check.
type ValidationFunc func(ctx context.Context, objectID, objectKind, filePath string, content []byte) (*ValidationState, error)

// ResultCallback is called when validation completes with the result
// This allows writing output directly from the validation goroutine
type ResultCallback func(state *ValidationState, err error)

// ValidationEventCallback is called to emit events via coordinator
type ValidationEventCallback func(eventType string, objectID string, message string, fields map[string]any, severity string)

// ValidationLifecycleEventCallback is called to emit lifecycle events via coordinator
type ValidationLifecycleEventCallback func(
	ctx context.Context,
	projectRoot string,
	storage any, // Storage provider not available in validation package
	operationID string,
	operationType string,
	status string,
	workerCount int,
	processedCount int,
	failedCount int,
	duration time.Duration,
)

var (
	globalValidationLifecycleEventCallback ValidationLifecycleEventCallback
	globalValidationLifecycleCallbackMu    sync.RWMutex
)

// SetValidationLifecycleEventCallback sets the global callback for emitting lifecycle events via coordinator
func SetValidationLifecycleEventCallback(callback ValidationLifecycleEventCallback) {
	if err := concurrency.RunInLockWithLogger(
		&globalValidationLifecycleCallbackMu,
		LockNameValidationSetLifecycleCallback,
		lockLoggerSystem(),
		func() error {
			globalValidationLifecycleEventCallback = callback
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.

			// getValidationLifecycleEventCallback returns the global lifecycle event callback (if set)
			ProfileSystem))).Error(ConstMagicc0e083e1, err).Log()
	}
}

func getValidationLifecycleEventCallback() ValidationLifecycleEventCallback {
	var callback ValidationLifecycleEventCallback
	if err := concurrency.RunInRLockWithLogger(
		&globalValidationLifecycleCallbackMu,
		LockNameValidationGetLifecycleCallback,
		lockLoggerSystem(),
		func() error {
			callback = globalValidationLifecycleEventCallback
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// AsyncValidator performs asynchronous validation of objects
			Error(ConstMagic166fefa7, err).Log()
	}
	return callback
}

type AsyncValidator struct {
	stateCache     *ValidationStateCache
	priorityQueue  *PriorityQueue
	maxWorkers     int // Maximum number of workers (renamed from workers for clarity)
	projectRoot    string
	logger         logging.Logger
	mu             sync.RWMutex
	running        bool
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup // Wait group for worker goroutines only
	validationWg   sync.WaitGroup // Separate wait group for validation goroutines (not waited in Stop())
	progressChan   chan ValidationProgress
	progressClosed bool
	validationFunc ValidationFunc // Optional: if set, uses real validation logic
	resultCallback ResultCallback // Optional: called when validation completes (for writing output)
	workerStates   sync.Map       // map[int]string - tracks worker states for debugging
	// Timeout configuration
	workerStopTimeout time.Duration // Timeout for waiting for workers to stop (default: 10s)
	cacheSaveTimeout  time.Duration // Timeout for saving cache (default: 5s)
	// Performance optimization: channel-based work notification
	taskAvailable chan struct{} // Signals workers when tasks are available (buffered, non-blocking)
	// Performance optimization: bounded goroutine semaphore to prevent goroutine proliferation
	validationSemaphore chan struct{} // Limits concurrent validation goroutines
	// Coordinator integration (optional - for metrics, logs, failures)
	eventCallback ValidationEventCallback // Optional callback for emitting coordinator events
	// Queue empty notification
	queueEmptyCallback         func() // Optional callback fired when queue becomes empty (prevents hangs)
	lastQueueSize              int    // Track last queue size to detect empty transition
	lastSemaphoreFullQueueSize int    // Prior semaphore-full sample; 0 means none yet
	// On-demand worker pattern
	activeWorkers atomic.Int32  // Atomic counter for active workers
	workerMu      sync.Mutex    // Protects worker startup
	shutdown      chan struct{} // Signals shutdown initiation
	stopOnce      sync.Once     // Ensures Stop() only runs once
	// Run issues: set during Stop() when timeouts occur; read by caller for visible summary (no log diving)
	workerStopTimedOut atomic.Bool
	cacheSaveTimedOut  atomic.Bool
	// Dedupe across batches: object IDs already enqueued this run (skip re-enqueue from discovery duplicates)
	enqueuedObjectIDs map[string]bool
}

// IsRunning returns true if the async validator is currently running
func (av *AsyncValidator) IsRunning() bool {
	var running bool
	if err := concurrency.RunInRLockWithLogger(
		&av.mu,
		LockNameAsyncValidatorIsRunning,
		lockLoggerSystem(),
		func() error {
			running = av.running
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// ValidationProgress represents progress of validation
			Error(ConstMagic4e3f89aa, err).Log()
	}
	return running
}

type ValidationProgress struct {
	CurrentObject string
	Status        string // "queued", "validating", "completed", "error"
	Errors        []string
}

func (av *AsyncValidator) sendProgress(progress ValidationProgress) (sent bool) {
	defer func() {
		if r := recover(); r != nil {
			sent = false
		}
	}()

	av.mu.Lock()
	if av.progressClosed {
		av.mu.Unlock()
		return false
	}
	ch := av.progressChan
	av.mu.Unlock()

	select {
	case ch <- progress:
		return true
	case <-av.ctx.Done():
		return false
	case <-time.After(100 * time.Millisecond):
		return false
	}
}
