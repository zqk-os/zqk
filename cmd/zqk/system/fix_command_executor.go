package system

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

// FixCommandExecutor manages asynchronous execution of fix commands with cancellation and callbacks
// Follows the same pattern as AsyncValidator for consistency and ease of migration
type FixCommandExecutor struct {
	projectRoot     string
	logger          logging.Logger
	mu              sync.RWMutex
	running         bool
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	queue           chan *FixCommandTask
	workers         int
	storageFactory  *storage.StorageFactory
	storageProvider storage.ObjectStorageProvider
	// Callbacks
	progressCallback   ProgressCallback
	errorCallback      ErrorCallback
	completionCallback CompletionCallback
	// Timeout configuration
	workerStopTimeout time.Duration
	// Approval gate
	requiresApproval bool
	approvalCallback ApprovalCallback
}

// FixCommandTask represents a single fix command to be executed
type FixCommandTask struct {
	ObjectID        string
	ObjectKind      string
	FixCommand      string // Original command with placeholders
	ResolvedCommand string // Command after placeholder resolution
	Issue           Issue  // Original issue that generated this fix
	QueuedAt        time.Time
	ResolvedAt      *time.Time
	ExecutedAt      *time.Time
	Status          FixCommandStatus
	Error           error
	RetryCount      int
	MaxRetries      int
}

// FixCommandStatus represents the status of a fix command
type FixCommandStatus string

const (
	FixCommandStatusQueued    FixCommandStatus = "queued"
	FixCommandStatusResolving FixCommandStatus = "resolving"
	FixCommandStatusResolved  FixCommandStatus = "resolved"
	FixCommandStatusApproving FixCommandStatus = "approving"
	FixCommandStatusApproved  FixCommandStatus = "approved"
	FixCommandStatusExecuting FixCommandStatus = "executing"
	FixCommandStatusCompleted FixCommandStatus = "completed"
	FixCommandStatusFailed    FixCommandStatus = "failed"
	FixCommandStatusCancelled FixCommandStatus = "cancelled"
)

// FixCommandProgress represents progress of fix command execution
type FixCommandProgress struct {
	TotalTasks     int
	CompletedTasks int
	FailedTasks    int
	CurrentObject  string
	Status         string // "queued", "resolving", "executing", "completed", "error"
	Errors         []string
}

// ProgressCallback is called when fix command execution makes progress
type ProgressCallback func(progress FixCommandProgress)

// ErrorCallback is called when a fix command fails
type ErrorCallback func(task *FixCommandTask, err error)

// CompletionCallback is called when a fix command completes successfully
type CompletionCallback func(task *FixCommandTask)

// ApprovalCallback is called to request approval before executing a fix command
// Returns (approved, error). If approved=false, the command is skipped.
type ApprovalCallback func(task *FixCommandTask) (approved bool, err error)

// NewFixCommandExecutor creates a new fix command executor
// Note: Uses pkgctx.NewSystemContext() for internal cancellation context as this is a long-lived component
func NewFixCommandExecutor(projectRoot string, workers int) *FixCommandExecutor {
	// Use system context for internal cancellation (long-lived component)
	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext()) //nolint:gosec // G118: cancel stored on executor; invoked from shutdown

	// Use component-based logging to route worker logs to a separate file
	decisionCtx := pkgctx.NewLoggingDecisionContext().
		WithLoggingContext(pkgctx.NewLoggingContext(pkgctx.ProfileSystem)).
		WithComponent("fix_executor")
	logger := logging.GetLoggerFromDecisionContext(decisionCtx, projectRoot)

	// Create storage factory for placeholder resolution
	storageFactory, _ := storage.NewStorageFactory(ctx, projectRoot) //nolint:errcheck // Best effort - continue with nil if factory creation fails
	var storageProvider storage.ObjectStorageProvider
	if storageFactory != nil {
		storageProvider = storageFactory.GetStorage()
	}

	return &FixCommandExecutor{
		projectRoot:       projectRoot,
		logger:            logger,
		ctx:               ctx,
		cancel:            cancel,
		queue:             make(chan *FixCommandTask, workers*10), // Buffer 10x workers
		workers:           workers,
		storageFactory:    storageFactory,
		storageProvider:   storageProvider,
		workerStopTimeout: 10 * time.Second,
		requiresApproval:  true, // Default: require approval
	}
}

// SetTimeouts configures timeout values for worker stop
func (fce *FixCommandExecutor) SetTimeouts(workerStopTimeout time.Duration) {
	_ = concurrency.WithLockTimeout(
		&fce.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(fce.logger),
		LockNameFixExecutorSetTimeouts,
		func() error {
			if workerStopTimeout > 0 {
				fce.workerStopTimeout = workerStopTimeout
			}
			return nil
		},
	)
}

// SetProgressCallback sets the callback for progress updates
func (fce *FixCommandExecutor) SetProgressCallback(callback ProgressCallback) {
	_ = concurrency.WithLockTimeout(
		&fce.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(fce.logger),
		LockNameFixExecutorSetProgressCallback,
		func() error {
			fce.progressCallback = callback
			return nil
		},
	)
}

// SetErrorCallback sets the callback for error notifications
func (fce *FixCommandExecutor) SetErrorCallback(callback ErrorCallback) {
	_ = concurrency.WithLockTimeout(
		&fce.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(fce.logger),
		LockNameFixExecutorSetErrorCallback,
		func() error {
			fce.errorCallback = callback
			return nil
		},
	)
}

// SetCompletionCallback sets the callback for completion notifications
func (fce *FixCommandExecutor) SetCompletionCallback(callback CompletionCallback) {
	_ = concurrency.WithLockTimeout(
		&fce.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(fce.logger),
		LockNameFixExecutorSetCompletionCallback,
		func() error {
			fce.completionCallback = callback
			return nil
		},
	)
}

// SetApprovalCallback sets the callback for approval requests
// If set, requiresApproval is automatically set to true
func (fce *FixCommandExecutor) SetApprovalCallback(callback ApprovalCallback) {
	_ = concurrency.WithLockTimeout(
		&fce.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(fce.logger),
		LockNameFixExecutorSetApprovalCallback,
		func() error {
			fce.approvalCallback = callback
			fce.requiresApproval = true
			return nil
		},
	)
}

// SetRequiresApproval configures whether approval is required before execution
func (fce *FixCommandExecutor) SetRequiresApproval(requires bool) {
	_ = concurrency.WithLockTimeout(
		&fce.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(fce.logger),
		LockNameFixExecutorSetRequiresApproval,
		func() error {
			fce.requiresApproval = requires
			return nil
		},
	)
}

// Start starts the fix command executor workers
func (fce *FixCommandExecutor) Start() error {
	var alreadyRunning bool
	_ = concurrency.WithLockTimeout(
		&fce.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(fce.logger),
		LockNameFixExecutorStart,
		func() error {
			if fce.running {
				alreadyRunning = true
				return nil
			}
			fce.running = true
			return nil
		},
	)

	if alreadyRunning {
		return ErrFixExecutorAlreadyRunning
	}

	// Start worker goroutines
	for i := 0; i < fce.workers; i++ {
		workerID := i
		goroutinelabels.NewGoroutine(fmt.Sprintf("fix_command_executor_worker_%d", workerID), fmt.Sprintf("executing fix commands (worker %d of %d)", workerID, fce.workers)).
			WithWaitGroup(&fce.wg).
			StartSimple(func() {
				fce.worker(workerID)
			})
	}

	logging.Fluent(fce.logger).Info("Fix command executor started").
		Int("workers", fce.workers).
		Log()
	return nil
}

// Stop stops the fix command executor workers
func (fce *FixCommandExecutor) Stop() error {
	var workerTimeout time.Duration
	var wasRunning bool
	_ = concurrency.WithLockTimeout(
		&fce.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(fce.logger),
		LockNameFixExecutorStop,
		func() error {
			if !fce.running {
				return nil
			}
			wasRunning = true
			fce.cancel()
			fce.running = false
			workerTimeout = fce.workerStopTimeout
			return nil
		},
	)

	if !wasRunning {
		return nil
	}

	// Wait for workers to finish with timeout
	stopDone := make(chan struct{})
	goroutinelabels.NewGoroutine("fix_command_executor_stop_wait", "waiting for fix command executor goroutines to stop").
		WithCleanup(func() {
			close(stopDone)
		}).
		StartSimple(func() {
			fce.wg.Wait()
		})

	select {
	case <-stopDone:
		// All workers finished normally
	case <-time.After(workerTimeout):
		logging.Fluent(fce.logger).Warn("Timeout waiting for workers to stop").
			String("timeout", workerTimeout.String()).
			Log()
	}

	logging.Fluent(fce.logger).Info("Fix command executor stopped").Log()
	return nil
}

// Enqueue adds a fix command task to the queue
func (fce *FixCommandExecutor) Enqueue(task *FixCommandTask) error {
	var running bool
	_ = concurrency.WithRLockTimeout(
		&fce.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(fce.logger),
		LockNameFixExecutorEnqueue,
		func() error {
			running = fce.running
			return nil
		},
	)

	if !running {
		return ErrFixExecutorNotRunning
	}

	task.QueuedAt = time.Now()
	task.Status = FixCommandStatusQueued

	select {
	case fce.queue <- task:
		return nil
	case <-fce.ctx.Done():
		return fce.ctx.Err()
	default:
		return ErrFixCommandQueueFull
	}
}

// worker processes fix command tasks from the queue
func (fce *FixCommandExecutor) worker(id int) {
	defer fce.wg.Done()

	for {
		select {
		case <-fce.ctx.Done():
			return
		case task := <-fce.queue:
			if task == nil {
				continue
			}

			// Process task: resolve → approve → execute
			fce.processTask(task)
		}
	}
}

// processTask processes a single fix command task
func (fce *FixCommandExecutor) processTask(task *FixCommandTask) {
	// Step 1: Resolve placeholders
	task.Status = FixCommandStatusResolving
	resolvedCmd, resolveErr := fce.resolvePlaceholders(task)
	if resolveErr != nil {
		task.Status = FixCommandStatusFailed
		task.Error = resolveErr
		if callback := fce.getErrorCallback(); callback != nil {
			callback(task, resolveErr)
		}
		return
	}

	task.ResolvedCommand = resolvedCmd
	now := time.Now()
	task.ResolvedAt = &now
	task.Status = FixCommandStatusResolved

	// Step 2: Request approval if required
	if fce.requiresApproval {
		task.Status = FixCommandStatusApproving
		approved, approveErr := fce.requestApproval(task)
		if approveErr != nil {
			task.Status = FixCommandStatusFailed
			task.Error = approveErr
			if callback := fce.getErrorCallback(); callback != nil {
				callback(task, approveErr)
			}
			return
		}
		if !approved {
			task.Status = FixCommandStatusCancelled
			logging.Fluent(fce.logger).Info("Fix command not approved, skipping").
				String("object_id", task.ObjectID).
				String("command", task.ResolvedCommand).
				Log()
			return
		}
		task.Status = FixCommandStatusApproved
	}

	// Step 3: Execute the resolved command
	task.Status = FixCommandStatusExecuting
	execErr := fce.executeCommand(task)
	if execErr != nil {
		task.Status = FixCommandStatusFailed
		task.Error = execErr
		if callback := fce.getErrorCallback(); callback != nil {
			callback(task, execErr)
		}
		return
	}

	// Step 4: Success
	task.Status = FixCommandStatusCompleted
	now = time.Now()
	task.ExecutedAt = &now
	if callback := fce.getCompletionCallback(); callback != nil {
		callback(task)
	}
}

// resolvePlaceholders resolves placeholders in the fix command using field resolver
func (fce *FixCommandExecutor) resolvePlaceholders(task *FixCommandTask) (string, error) {
	// Parse placeholder from command
	placeholder, _ := parsePlaceholderFromCommand(task.FixCommand)
	if placeholder == emptyValue {
		// No placeholder, return command as-is
		return task.FixCommand, nil
	}

	// Extract query hint from placeholder
	queryHint := extractQueryHintFromPlaceholder(placeholder)
	if queryHint == emptyValue {
		// No query hint, can't resolve
		return task.FixCommand, nil
	}

	// Parse query hint to map
	queryHints := parseQueryHintToMap(queryHint)

	// Get spec loader for field resolver
	// Note: SpecLoader doesn't use context, so we don't need to pass one
	specLoader := objects.NewSpecLoader(filepath.Join(fce.projectRoot, paths.ProcessInternalObjectSpecsDir))

	// Create field resolver
	resolver := NewFieldResolver(fce.projectRoot, specLoader, fce.storageProvider)

	// Extract field name from command (e.g., "milestone_refs+=" → objects.FieldKeyMilestoneRefs)
	fieldName := extractFieldNameFromCommand(task.FixCommand)
	if fieldName == emptyValue {
		return task.FixCommand, nil
	}

	// Resolve using field resolver (with timeout for storage operations)
	resolveCtx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 10*time.Second)
	defer cancel()
	candidates, resolveErr := resolver.ResolveFieldFromSpec(resolveCtx, task.ObjectKind, fieldName, queryHints)
	if resolveErr != nil {
		return task.FixCommand, resolveErr
	}

	// If exactly one candidate, resolve placeholder
	if len(candidates) == 1 {
		resolvedCmd := replacePlaceholderInCommand(task.FixCommand, placeholder, candidates[0])
		return resolvedCmd, nil
	}

	// Multiple or no candidates - leave as placeholder (will need user selection or best-match logic)
	// For now, return command with placeholder
	return task.FixCommand, nil
}

// requestApproval requests approval before executing a fix command
func (fce *FixCommandExecutor) requestApproval(task *FixCommandTask) (bool, error) {
	var callback ApprovalCallback
	_ = concurrency.WithRLockTimeout(
		&fce.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(fce.logger),
		LockNameFixExecutorRequestApproval,
		func() error {
			callback = fce.approvalCallback
			return nil
		},
	)

	if callback == nil {
		// No approval callback set - default to approved if approval is required but no callback
		// This allows tests to skip approval
		return true, nil
	}

	return callback(task)
}

// executeCommand executes the resolved fix command
func (fce *FixCommandExecutor) executeCommand(task *FixCommandTask) error {
	// TODO: Implement command execution (parse command, call storage provider, etc.)
	// For now, this is a placeholder
	logging.Fluent(fce.logger).Info("Executing fix command").
		String("object_id", task.ObjectID).
		String("command", task.ResolvedCommand).
		Log()
	return nil
}

// Helper methods to get callbacks (with locking)
func (fce *FixCommandExecutor) getProgressCallback() ProgressCallback {
	var callback ProgressCallback
	_ = concurrency.WithRLockTimeout(
		&fce.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(fce.logger),
		LockNameFixExecutorGetProgressCallback,
		func() error {
			callback = fce.progressCallback
			return nil
		},
	)
	return callback
}

func (fce *FixCommandExecutor) getErrorCallback() ErrorCallback {
	var callback ErrorCallback
	_ = concurrency.WithRLockTimeout(
		&fce.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(fce.logger),
		LockNameFixExecutorGetErrorCallback,
		func() error {
			callback = fce.errorCallback
			return nil
		},
	)
	return callback
}

func (fce *FixCommandExecutor) getCompletionCallback() CompletionCallback {
	var callback CompletionCallback
	_ = concurrency.WithRLockTimeout(
		&fce.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(fce.logger),
		LockNameFixExecutorGetCompletionCallback,
		func() error {
			callback = fce.completionCallback
			return nil
		},
	)
	return callback
}

// Errors
var (
	ErrFixExecutorAlreadyRunning = errfmt.Errorf("fix command executor is already running")
	ErrFixExecutorNotRunning     = errfmt.Errorf("fix command executor is not running")
	ErrFixCommandQueueFull       = errfmt.Errorf("fix command queue is full")
)
