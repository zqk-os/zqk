package system

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"
)

// ValidationProgressContext groups state for validation progress tracking
type ValidationProgressContext struct {
	Cmd                        *cobra.Command
	Ctx                        *cli.Context
	Validator                  *validation.AsyncValidator
	TotalTasks                 int
	Metrics                    *validation.ValidationMetrics
	EnqueuedObjectIDs          map[string]bool
	EnqueuedObjectInfo         map[string]struct{ Kind, FilePath string } // optional; used when building final results for objects with no cached state (e.g. SHM)
	CachedCheckResults         map[string]CheckResult                     // cache-hit snapshots for accurate summary
	OutputQueue                *validation.OutputQueue
	SuppressProgress           bool
	Timeout                    time.Duration
	ProgressChan               <-chan validation.ValidationProgress
	Completed                  int
	Failed                     int
	StartTime                  time.Time
	ValidationStart            time.Time
	CompletedObjectIDs         map[string]bool
	FailedObjectIDs            map[string]string
	Mu                         sync.RWMutex
	QueueEmptySince            *time.Time
	CompletionCheckDone        bool
	LastPrintedCompleted       int
	LastPrintedQueueSize       int
	LastPrintedTime            time.Time // When we last printed progress (for heartbeat so UI never appears hung)
	LastProgressTime           time.Time
	LastProgressCount          int
	StuckTimeout               time.Duration
	Logger                     logging.Logger
	DrainDone                  chan struct{}
	QueueEmpty                 chan struct{} // Channel signaled when queue becomes empty (prevents hangs)
	ForceComplete              bool          // When true, complete with current results (queue empty for long time, some tasks unaccounted)
	ProjectRoot                string
	StorageProvider            storage.ObjectStorageProvider // For coordination events
	OperationID                string                        // Unique operation ID for coordination
	LastProgressSummaryPercent int                           // Last sparse progress-summary threshold emitted (not MIL-*)
}

// initializeValidationProgressContext sets up the validation progress context
func initializeValidationProgressContext(cmd *cobra.Command, ctx *cli.Context, validator *validation.AsyncValidator, totalTasks int, metrics *validation.ValidationMetrics, enqueuedObjectIDs map[string]bool, enqueuedObjectInfo map[string]struct{ Kind, FilePath string }, cachedCheckResults map[string]CheckResult, outputQueue *validation.OutputQueue, operationID string) (*ValidationProgressContext, error) {
	format := cli.GetFormat(cmd)
	outputPath := cli.GetOutputPath(cmd)
	suppressProgress := (format == cli.FormatJSON || format == cli.FormatJSONL) && outputPath != emptyValue

	progressChan := validator.GetProgress()
	startTime := time.Now()
	validationStart := time.Now()
	completedObjectIDs := make(map[string]bool)
	failedObjectIDs := make(map[string]string)

	timeout, err := calculateValidationTimeout(cmd, totalTasks)
	if err != nil {
		return nil, err
	}

	var logger logging.Logger
	if ctx != nil {
		logger = getLoggerForSystemCheck(cmd, ctx.Profile)
	} else {
		logger = getLoggerForSystemCheck(cmd, systemProfileHuman)
	}

	// Get project root and storage provider for coordination events
	var projectRoot string
	var storageProvider storage.ObjectStorageProvider
	if ctx != nil {
		projectRoot = ctx.ProjectRoot
	}
	projectRoot = ProjectRootOrResolve(projectRoot)
	if projectRoot != emptyValue {
		factory, storageErr := storage.NewStorageFactory(cmd.Context(), projectRoot)
		if storageErr == nil {
			storageProvider = factory.GetStorage()
			defer func() { _ = storageProvider.Shutdown(context.Background()) }() // Background: request-or-shutdown derived
		}
		if storageErr != nil {
			// Best effort - continue without coordinator if storage unavailable
			storageProvider = nil
		}
	}

	// Use provided operation ID if available, otherwise generate a unique one
	if operationID == emptyValue {
		operationID = fmt.Sprintf("check_%d", time.Now().Unix())
	}

	// Create high-level operation callback for this check run.
	// This provides a single, canonical lifecycle view on top of the more
	// granular coordinator events already emitted elsewhere.
	opProfile := systemProfileHuman
	if ctx != nil && ctx.Profile != emptyValue {
		opProfile = ctx.Profile
	}
	var opCallback concurrency.OperationCallback
	if projectRoot != emptyValue && storageProvider != nil {
		opCallback = coordination.NewCoordinatorOperationCallback(
			pkgctx.NewSystemContext(),
			projectRoot,
			storageProvider,
			"system_check",
			opProfile,
		)
		// Emit start event with basic metadata
		opCallback.OnStart(operationID, map[string]any{
			"operation_type": "system_check",
			"total_tasks":    totalTasks,
		})
	} else {
		opCallback = &concurrency.NoOpOperationCallback{}
	}

	// Helper functions for extracting fields from map
	getIntField := func(fields map[string]any, key string, defaultValue int) int {
		if val, ok := fields[key]; ok {
			if intVal, ok := val.(int); ok {
				return intVal
			}
		}
		return defaultValue
	}
	getStringField := func(fields map[string]any, key string, defaultValue string) string {
		if val, ok := fields[key]; ok {
			if strVal, ok := val.(string); ok {
				return strVal
			}
		}
		return defaultValue
	}

	// Set event callback on validator for coordinator event emission
	// This allows async validator to emit events without import cycles
	validator.SetEventCallback(func(eventType, objectID, message string, fields map[string]any, severity string) {
		// Emit via coordinator (non-blocking, async)
		emitBud := goroutinelabels.DefaultBudget()
		emitBuilder := goroutinelabels.NewGoroutine("validation_event_emit", fmt.Sprintf("emitting validation %s event for %s", eventType, objectID))
		if emitBud != nil {
			emitBuilder = emitBuilder.WithBudget(emitBud)
		}
		emitBuilder.StartSimple(func() {
			switch eventType {
			case "semaphore_full":
				emitSemaphoreFullWarningViaCoordinator(
					pkgctx.NewSystemContext(), projectRoot, storageProvider,
					operationID, objectID,
					getIntField(fields, "queue_size", 0),
					getIntField(fields, "semaphore_capacity", 0),
					ctx.Profile,
				)
			case "validation_timeout":
				timeoutDuration := 30 * time.Second
				if timeoutStr, ok := fields["timeout_duration"].(string); ok {
					if parsed, err := time.ParseDuration(timeoutStr); err == nil {
						timeoutDuration = parsed
					}
				}
				emitValidationTimeoutViaCoordinator(
					pkgctx.NewSystemContext(), projectRoot, storageProvider,
					operationID, objectID, timeoutDuration, ctx.Profile,
				)
			case "file_read_error":
				filePath := getStringField(fields, "file_path", "")
				errMsg := getStringField(fields, "error", "")
				var err error
				if errMsg != emptyValue {
					err = errfmt.Errorf("%s", errMsg)
				}
				emitFileReadErrorViaCoordinator(
					pkgctx.NewSystemContext(), projectRoot, storageProvider,
					operationID, objectID, filePath, err, ctx.Profile,
				)
			}
		})
	})

	vpc := &ValidationProgressContext{
		Cmd:                        cmd,
		Ctx:                        ctx,
		Validator:                  validator,
		TotalTasks:                 totalTasks,
		Metrics:                    metrics,
		EnqueuedObjectIDs:          enqueuedObjectIDs,
		EnqueuedObjectInfo:         enqueuedObjectInfo,
		CachedCheckResults:         cachedCheckResults,
		OutputQueue:                outputQueue,
		SuppressProgress:           suppressProgress,
		Timeout:                    timeout,
		ProgressChan:               progressChan,
		StartTime:                  startTime,
		ValidationStart:            validationStart,
		CompletedObjectIDs:         completedObjectIDs,
		FailedObjectIDs:            failedObjectIDs,
		LastProgressTime:           time.Now(),
		StuckTimeout:               validation.GetGlobalValidationTimeoutConfig().StuckTimeout(), // From config or default 30s
		Logger:                     logger,
		DrainDone:                  make(chan struct{}),
		QueueEmpty:                 make(chan struct{}, 1), // Buffered to prevent blocking
		ProjectRoot:                projectRoot,
		StorageProvider:            storageProvider,
		OperationID:                operationID,
		LastProgressSummaryPercent: 0,
	}

	// Set up queue empty callback AFTER vpc is created (needs reference to vpc.QueueEmpty)
	// This prevents hangs when queue becomes empty but completion isn't detected immediately
	validator.SetQueueEmptyCallback(func() {
		// Signal queue empty channel (non-blocking, buffered)
		select {
		case vpc.QueueEmpty <- struct{}{}:
			// Successfully signaled
		default:
			// Channel already has signal (prevents duplicate signals)
		}

		// Emit an immediate progress event when the queue becomes empty.
		// This restores the “actionable ASAP” feel (queue size hits 0) without waiting for the next ticker.
		if vpc.ProjectRoot != emptyValue && vpc.StorageProvider != nil {
			profile := systemProfileHuman
			if vpc.Ctx != nil && vpc.Ctx.Profile != emptyValue {
				profile = vpc.Ctx.Profile
			}
			// Best-effort snapshot. Avoid expensive cache counting here.
			vpc.Mu.RLock()
			currentCompleted := vpc.Completed
			currentFailed := vpc.Failed
			progressReceivedCount := len(vpc.CompletedObjectIDs)
			vpc.Mu.RUnlock()
			_, _, _, queueSize := vpc.Validator.GetValidationStats()

			emitCheckProgressEventViaCoordinator(
				pkgctx.NewSystemContext(),
				vpc.ProjectRoot,
				vpc.StorageProvider,
				vpc.OperationID,
				"queue_empty",
				progressReceivedCount, // best-effort progress count
				vpc.TotalTasks,
				currentCompleted,
				currentFailed,
				queueSize,
				"Validation queue is empty (awaiting in-flight workers and finalization)",
				profile,
			)
		}
	})

	return vpc, nil
}

// calculateValidationTimeout calculates the validation timeout
func calculateValidationTimeout(cmd *cobra.Command, totalTasks int) (time.Duration, error) {
	timeout := cli.GetTimeout(cmd)

	if timeout == 0 {
		baseTimePerObject := 100 * time.Millisecond
		workerCount := 4
		estimatedTime := time.Duration(totalTasks) * baseTimePerObject / time.Duration(workerCount)
		timeout = estimatedTime*2 + 30*time.Second

		if timeout < 1*time.Minute {
			timeout = 1 * time.Minute
		}
		if timeout > 30*time.Minute {
			timeout = 30 * time.Minute
		}
	}

	return timeout, nil
}

// copyCompletedObjectIDs safely copies the completed object IDs map
func (vpc *ValidationProgressContext) copyCompletedObjectIDs() map[string]bool {
	vpc.Mu.RLock()
	defer vpc.Mu.RUnlock()
	completedCopy := make(map[string]bool, len(vpc.CompletedObjectIDs))
	maps.Copy(completedCopy, vpc.CompletedObjectIDs)
	return completedCopy
}

// copyFailedObjectIDs safely copies the failed object IDs map
func (vpc *ValidationProgressContext) copyFailedObjectIDs() map[string]string {
	vpc.Mu.RLock()
	defer vpc.Mu.RUnlock()
	failedCopy := make(map[string]string, len(vpc.FailedObjectIDs))
	maps.Copy(failedCopy, vpc.FailedObjectIDs)
	return failedCopy
}
