package system

import (
	stdcontext "context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	caspkg "github.com/lanceman/zqk/pkg/storage/cas"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/appledouble"
	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/when"

	"github.com/lanceman/zqk/pkg/objects"
)

// Limit concurrent per-file content reads during discovery fallback.
// This prevents goroutine/FD blowups when the filesystem is slow/uninterruptible.
var extractObjectIDReadSem = make(chan struct{}, 32) // bounded, small; discovery should prefer CAS index

// Track how often we skip reads due to contention (best-effort observability without log spam).
var extractObjectIDSkippedReads uint64

const (
	progressStageLoading = "loading"
	outputChannelStdout  = "stdout"
	outputChannelFile    = "file"
	metadataKeyAutoFixed = "auto_fixed"
	msgSetupLoaderPanic  = "Setup loader panic (continuing)"
	msgLoadersReady      = "Loaders ready."
)

// getGoroutineID returns a unique ID for the current goroutine (re-exported from validation package)
func getGoroutineID() int64 {
	return validation.GetGoroutineID()
}

// incrementActiveGoroutines increments the active goroutine counter (re-exported from validation package)
func incrementActiveGoroutines() int64 {
	return validation.IncrementActiveGoroutines()
}

// decrementActiveGoroutines decrements the active goroutine counter (re-exported from validation package)
func decrementActiveGoroutines() int64 {
	return validation.DecrementActiveGoroutines()
}

// getActiveGoroutines returns the current count of active goroutines (re-exported from validation package)
func getActiveGoroutines() int64 {
	return validation.GetActiveGoroutines()
}

// GetAsyncValidator creates a new async validator instance for the given command context
// Each command gets its own validator instance with a properly derived context hierarchy
// ctx: parent context from command entry point - will be derived hierarchically (context.WithCancel)
func GetAsyncValidator(ctx stdcontext.Context, projectRoot string, workerCount int) *validation.AsyncValidator {
	// Use provided worker count, or calculate optimal default from global concurrency config
	if workerCount <= 0 {
		cfg := concurrency.GetGlobalConcurrencyConfig()
		workerCount = cfg.ValidatorMaxWorkers
	}

	// Final safety clamp in case configuration was mis-set
	if workerCount <= 0 {
		workerCount = 2
	}

	// Create new validator instance with context derived from command context
	// NewAsyncValidator will derive its internal context using context.WithCancel(ctx)
	// This maintains proper hierarchical context derivation
	// Note: Semaphore capacity is set at creation time and cannot be dynamically adjusted
	// Metrics analysis will provide recommendations for next run
	cfg := validation.DefaultAsyncValidatorConfig()
	cfg.ProgressChannelSize = 50000
	// Warm path relies on disk-backed ValidationStateCache across CLI processes.
	// A 1h TTL made consecutive checks miss almost everything once LastValidated aged out
	// (observed: cache_hits≈5 / ~5k objects). Mtime/checksum still gate freshness.
	return validation.NewAsyncValidator(ctx, projectRoot, workerCount, validation.DefaultValidationStateCacheMaxAge, cfg)
}

// getRecommendedSemaphoreCapacity calculates recommended semaphore capacity based on metrics
// This helps inform validator creation for future runs
// Note: Semaphore capacity cannot be changed after validator creation, so this is informational
func getRecommendedSemaphoreCapacity() int {
	specLoader := objects.GetGlobalSpecLoader()
	if specLoader == nil {
		return 16 // Default: NumCPU * 2
	}

	metrics := specLoader.GetMetrics()
	if metrics.TotalWaits == 0 {
		return 16 // No data yet, use default
	}

	baseSemaphore := 16 // Current default (NumCPU * 2)

	// Adjust based on contention rate
	if metrics.ContentionRate > 0.3 {
		// High contention - increase semaphore significantly
		return baseSemaphore * 2
	} else if metrics.ContentionRate > 0.1 {
		// Moderate contention - slight increase
		return int(float64(baseSemaphore) * 1.5)
	}

	return baseSemaphore
}

// runCheckAsync runs system check with async validation
func runCheckAsync(cmd *cobra.Command, args []string) error {
	// Prefer pre-set context (e.g. from tests or callers) so deprecated --sync is not required
	var ctx *cli.Context
	if cliCtx := cli.GetContext(cmd); cliCtx != nil && cliCtx.Context != nil {
		ctx = cliCtx
	}
	if ctx == nil {
		initCtx := &pkgctx.CliInitializationContext{
			ProjectRoot: ProjectRootOrResolve(""),
		}
		var err error
		ctx, err = cli.GetContextFromCommand(cmd, initCtx)
		if err != nil {
			return err
		}
	}
	return runCheckAsyncWithContext(cmd, ctx, args)
}

// getLoggerForSystemCheck creates a logger for system check operations
// CRITICAL: When --verbose is set, we want more detailed output but NOT debug-level logging
// Debug-level logging creates excessive audit events. This function ensures verbose means
// "more detailed output" (InfoLevel) not "debug-level logging" (DebugLevel).
// When cmd is nil (e.g. background single-object validation), returns profile-based logger.
func getLoggerForSystemCheck(cmd *cobra.Command, profile string) logging.Logger {
	if cmd == nil {
		return logging.GetLoggerFromProfile(profile)
	}
	verbose, err := cmd.Flags().GetBool("verbose")
	if err == nil && verbose {
		// When verbose is set, use InfoLevel instead of DebugLevel to prevent excessive audit events
		// Create a logging context with InfoLevel override
		loggingCtx := pkgctx.NewLoggingContext(pkgctx.ProfileSystem)
		loggingCtx = loggingCtx.WithLevel(int(logging.InfoLevel))
		// Use command context for formatter context propagation
		return logging.GetLoggerFromLoggingContext(cmd.Context(), loggingCtx)
	}
	// Normal operation: use profile-based logger (may be DebugLevel, but that's OK for non-verbose)
	return logging.GetLoggerFromProfile(profile)
}

// runCheckAsyncWithContext runs async validation with context
func runCheckAsyncWithContext(cmd *cobra.Command, ctx *cli.Context, args []string) error {
	return runCheckAsyncWithContextAndOperationID(cmd, ctx, args, "", nil)
}

// runCheckAsyncWithContextAndOperationID runs async validation with context and operation ID.
// cancelCtx is optional: when non-nil (e.g. from runCheckAsyncWithFollow with signal.NotifyContext),
// it is passed to cache build/warm so SIGINT cancels the check gracefully and defers (e.g. CPU profile flush) run.
//
//nolint:gocyclo // Orchestrates async check pipeline; complexity from phases and branching

// idle watchdog: async check entered (avoids cancel during init when parent is not zqk)

// Set operation ID for unified coordination across discovery + validation

// Fallback: generate one if not provided

// Emit early progress so user sees activity immediately after "System check starting..." (avoids perceived hang).
// idle watchdog: check logic started (avoids cancel during init when parent is not zqk)

// Proactive Stale CAS cleanup when --auto-fix: resolve Tier 3 "Stale CAS version" before validation.
// Run with a deadline so we never hang indefinitely (cleanup can block on I/O or locks for large repos).

// idle watchdog: about to load kinds / run cleanup (avoids cancel during discovery)

// idle watchdog: proactive cleanup starting (avoids cancel when parent is not zqk)

// idle watchdog: progress so runner doesn't cancel during cleanup

// idle watchdog: progress so runner doesn't cancel during cleanup

// idle watchdog: still waiting for cleanup so runner doesn't cancel

// Use the component loader pattern (pkg/loader.Runner) for all load-once components so we get
// deterministic behavior, configurable timeouts, and consistent telemetry. Run all four in parallel.
// One storage for the whole run: discovery and ref validation reuse it (warm still creates its own inside cache build).

// Channel-based "all loaders done" so no goroutine ever blocks indefinitely (no Wait() with no timeout).

// buffer 4 so each loader can send without blocking

// Panic handlers ensure we still signal loaderDone so the watchdog doesn't wait forever for a missing signal.

// Single goroutine (established pattern): collect 4 loader signals or hit deadline, then close setupDone. No raw go func().

// one more loader finished

// deadline reached; exit loop; WithCleanup will close setupDone

// Emit progress before cache build (per pipeline: phase 1 = cache load/build).

// Emit progress so CLI shows we're past cache warm (avoids perceived hang before discovery)

// Setup output queue and writer BEFORE discovery starts (needed for concurrent validation)

//nolint:errcheck

// Emit progress so user sees we're past warmup and starting discovery.

// Start discovery and enqueue concurrently (validation starts immediately as files are discovered)

// runCheckAsyncWithContextAndOperationID runs async validation with context and operation ID.
// It is now pipeline-wrapped for standardized stage boundaries/observability.
func runCheckAsyncWithContextAndOperationID(cmd *cobra.Command, ctx *cli.Context, args []string, operationID string, cancelCtx stdcontext.Context) error {
	return RunSystemCheckViaPipeline(cmd, ctx, args, operationID, cancelCtx)
}

// setupAsyncValidationFunction sets up the validation function for async validator
// This function creates a closure that has access to all the dependencies needed
// for real validation (specLoader, lifecycleLoader, validator, caches, etc.)
// If checkCtx is non-nil and has StorageProvider set, it is reused for ref validation (same instance as discovery).
func setupAsyncValidationFunction(asyncValidator *validation.AsyncValidator, cmd *cobra.Command, ctx *cli.Context, projectRoot string, metrics *validation.ValidationMetrics, operationID string, checkCtx *AsyncCheckContext, cancelCtx stdcontext.Context) {
	asyncCtx := initializeAsyncValidationContext(cmd, ctx, projectRoot, metrics)
	fastMode, checkRefs := getAsyncValidationFlags(cmd, ctx)
	var storageForWarm storage.ObjectStorageProvider
	if checkCtx != nil {
		storageForWarm = checkCtx.StorageProvider
	}
	asyncCtx.ObjectIDCache = buildObjectIDCacheIfNeeded(cmd, ctx, projectRoot, checkRefs, fastMode, operationID, storageForWarm, cancelCtx)
	when.When(func() bool { return checkCtx != nil && checkCtx.StorageProvider != nil }).Then(func() {
		asyncCtx.StorageProvider = checkCtx.StorageProvider
	}).OrElse(func() {
		asyncCtx.StorageProvider = getStorageProviderForCache(projectRoot)
	}).Run()
	validationFunc := createAsyncValidationFunc(asyncCtx)
	asyncValidator.SetValidationFunc(validationFunc)
}

// setupResultCallback sets up the callback to write results directly from validation goroutines
func setupResultCallback(asyncValidator *validation.AsyncValidator, cmd *cobra.Command, ctx *cli.Context, outputQueue *validation.OutputQueue) {
	// Get format handler
	format := cli.GetFormat(cmd)
	handler := cli.GetFormatHandler(format)
	outputPath := cli.GetOutputPath(cmd)

	// Determine output channel (stdout or file)
	outputChannel := outputChannelStdout
	if outputPath != emptyValue {
		outputChannel = outputChannelFile
	}

	// Create callback that writes result directly
	// CRITICAL: Run in goroutine to prevent blocking validation goroutines
	// handler.Stream() may do expensive work (JSON marshaling, etc.) that could block
	callback := func(state *validation.ValidationState, err error) {
		if err != nil {
			// Create a fallback state representing the error
			state = &validation.ValidationState{
				LastValidated: time.Now(),
				Issues: []validation.ValidationIssue{
					{
						Tier:     1,
						Category: "integrity",
						Message:  fmt.Sprintf("Validation processing failed: %v", err),
					},
				},
			}
		}

		// Run output writing in separate goroutine to prevent blocking validation
		outputBud := goroutinelabels.DefaultBudget()
		outputBuilder := goroutinelabels.NewGoroutine("async_check_output", fmt.Sprintf("writing check result for %s", state.ObjectID))
		if outputBud != nil {
			outputBuilder = outputBuilder.WithBudget(outputBud)
		}
		outputBuilder.StartSimple(func() {
			// Convert ValidationState to CheckResult
			result := convertValidationStateToCheckResult(state)

			// If streaming format, write directly using handler
			if handler != nil && handler.IsStreaming() {
				// Create a writer that enqueues to output queue
				logger := getLoggerForSystemCheck(cmd, ctx.Profile)
				queueWriter := &queueWriter{
					queue:     outputQueue,
					channelID: outputChannel,
					flushHint: false,
					logger:    logger,
				}

				// Stream result using handler (non-blocking write to queue)
				// Use command context for proper cancellation propagation
				if streamErr := handler.Stream(cmd.Context(), result, queueWriter); streamErr != nil {
					// Log error but don't block
					logger := getLoggerForSystemCheck(cmd, ctx.Profile)
					logging.Fluent(logger).Debug("Failed to stream result").WithError(streamErr).Log()
				}
			} else {
				// Non-streaming format - collect results (handled by showValidationProgressWithMetrics)
				// For now, just skip writing here
			}
		})
	}

	asyncValidator.SetResultCallback(callback)
}

// queueWriter is an io.Writer that enqueues data to an OutputQueue
// It's non-blocking - if queue is full, it drops the data and returns success
// This prevents validation goroutines from hanging on output
type queueWriter struct {
	queue     *validation.OutputQueue
	channelID string
	flushHint bool
	logger    logging.Logger
}

func (qw *queueWriter) Write(p []byte) (n int, err error) {
	// Make a copy of the data to avoid issues with slice reuse
	data := make([]byte, len(p))
	copy(data, p)

	packet := validation.OutputPacket{
		ChannelID: qw.channelID,
		Data:      data,
		FlushHint: qw.flushHint,
	}

	// Enqueue (non-blocking, returns error if queue full)
	// If queue is full, we drop the data to prevent blocking validation goroutines
	if enqueueErr := qw.queue.Enqueue(packet); enqueueErr != nil {
		if qw.logger != nil {
			logging.Fluent(qw.logger).Warn("Output queue full, dropping data").
				String("channel", qw.channelID).
				Int("data_size", len(p)).
				WithError(enqueueErr).
				Log()
		}
		// Return success anyway to prevent blocking - data is dropped
		return len(p), nil
	}

	return len(p), nil
}

// convertCheckResultToValidationState converts a CheckResult to ValidationState
func convertCheckResultToValidationState(result *CheckResult) *validation.ValidationState {
	issues := make([]validation.ValidationIssue, 0, len(result.Issues))
	for _, issue := range result.Issues {
		if isTransientCacheCoherenceIssue(issue.Category) {
			continue
		}
		issues = append(issues, validation.ValidationIssue{
			Tier:        issue.Tier,
			Category:    issue.Category,
			Message:     issue.Message,
			AutoFixable: issue.AutoFixable,
			DetectedAt:  time.Now(),
		})
	}

	// Store AutoFixed results and Status in Metadata so they're preserved
	metadata := make(map[string]string)
	if len(result.AutoFixed) > 0 {
		// Store as comma-separated list in metadata
		metadata[metadataKeyAutoFixed] = strings.Join(result.AutoFixed, ",")
	}
	if result.Status != "" {
		metadata[objects.FieldKeyStatus] = result.Status
	}

	return &validation.ValidationState{
		ObjectID:      result.ObjectID,
		ObjectKind:    result.ObjectKind,
		FilePath:      result.FilePath,
		LastValidated: time.Now(),
		Issues:        issues,
		Metadata:      metadata,
	}
}

// convertValidationStateToCheckResult converts a ValidationState to CheckResult
func convertValidationStateToCheckResult(state *validation.ValidationState) CheckResult {
	issues := make([]Issue, 0, len(state.Issues))
	for _, issue := range state.Issues {
		issues = append(issues, Issue{
			Tier:        issue.Tier,
			Category:    issue.Category,
			Message:     issue.Message,
			AutoFixable: issue.AutoFixable,
		})
	}

	// Restore AutoFixed and Status from Metadata if present
	var autoFixed []string
	status := ""
	if state.Metadata != nil {
		if autoFixedStr, ok := state.Metadata[metadataKeyAutoFixed]; ok && autoFixedStr != emptyValue {
			autoFixed = strings.Split(autoFixedStr, ",")
		}
		if s, ok := state.Metadata[objects.FieldKeyStatus]; ok {
			status = s
		}
	}

	return CheckResult{
		ObjectID:   state.ObjectID,
		ObjectKind: state.ObjectKind,
		FilePath:   state.FilePath,
		Status:     status,
		Issues:     issues,
		AutoFixed:  autoFixed,
	}
}

// runCheckFromCache runs check using only cached validation state
func runCheckFromCache(cmd *cobra.Command, ctx *cli.Context, validator *validation.AsyncValidator) error {
	// Get all cached validation states
	total, stale, withIssues, queueSize := validator.GetValidationStats()

	logger := getLoggerForSystemCheck(cmd, ctx.Profile)
	logging.Fluent(logger).Info("Validation Cache Statistics").
		Int("total_cached", total).
		Int("stale_entries", stale).
		Int("objects_with_issues", withIssues).
		Int("queued_tasks", queueSize).
		Log()

	// Convert cached states to CheckResult format and output
	allStates := validator.GetAllCachedStates()

	cachedStateMap := make(map[string]bool, len(allStates))
	results := make([]CheckResult, 0, len(allStates))
	for _, state := range allStates {
		cachedStateMap[state.ObjectID] = true
		results = append(results, convertValidationStateToCheckResult(state))
	}

	// Calculate and report outstanding validations by comparing with ObjectIDCache
	projectRoot := ProjectRootOrResolve(ctx.ProjectRoot)
	objectIDCache := GetGlobalObjectIDCache()
	if _, err := objectIDCache.LoadCache(projectRoot); err == nil {
		allKnownObjects := objectIDCache.GetAll()
		outstandingCount := 0

		for _, entry := range allKnownObjects {
			if !cachedStateMap[entry.ID] {
				outstandingCount++
				results = append(results, CheckResult{
					ObjectID:   entry.ID,
					ObjectKind: entry.Kind,
					FilePath:   entry.FilePath,
					Status:     objects.ObjectStatusPending,
					Issues: []Issue{
						{
							Tier:        3, // Informational
							Category:    "outstanding_validation",
							Message:     "Validation pending (outstanding). Trigger async validation or wait for completion.",
							AutoFixable: false,
						},
					},
				})
			}
		}

		if outstandingCount > 0 {
			logging.Fluent(logger).Info(fmt.Sprintf("Found %d outstanding validations", outstandingCount)).
				Int("outstanding_count", outstandingCount).
				Log()
		}
	}

	// Output results using the same format as sync check
	// Note: We don't have an audit buffer in cache-only mode, so pass nil
	return outputResults(cmd, ctx, results, nil, nil, nil)
}

// showValidationProgress displays validation progress and collects results
//
//nolint:unused // Replaced by showValidationProgressWithMetrics - reserved for legacy compatibility
func showValidationProgress(cmd *cobra.Command, ctx *cli.Context, validator *validation.AsyncValidator, totalTasks int) error {
	metrics := validation.NewValidationMetrics()
	metrics.SetTotalObjects(totalTasks)
	// Create empty map for backward compatibility (this function is deprecated)
	enqueuedObjectIDs := make(map[string]bool)
	// Create a minimal output queue for backward compatibility
	outputQueue := validation.NewOutputQueue(10000)
	return showValidationProgressWithMetrics(cmd, ctx, validator, totalTasks, metrics, enqueuedObjectIDs, nil, nil, outputQueue, "")
}

// forceExitIfValidationStuck os.Exits only for a stuck queue. A finished
// SystemCheckError must return so Cobra can map Layer 0 / Tier 1 / warnings
// to exit 2 / 1 / 3 instead of logging "Validation stuck" and flattening to 1.
func forceExitIfValidationStuck(vpc *ValidationProgressContext, validator *validation.AsyncValidator, err error) error {
	if err == nil {
		return nil
	}
	if isCompletedSystemCheckError(err) {
		return err
	}
	logging.Fluent(vpc.Logger).Error("Validation stuck - forcing immediate process exit", err).
		Log()
	goroutinelabels.NewGoroutine("system", "force validator stop on stuck").StartSimple(func() {
		_ = validator.Stop()
	})
	//nolint:gocritic // Intentional: stuck detector must not wait on deferred cleanup
	os.Exit(1)
	return err
}

// showValidationProgressWithMetrics displays validation progress with metrics tracking
func showValidationProgressWithMetrics(cmd *cobra.Command, ctx *cli.Context, validator *validation.AsyncValidator, totalTasks int, metrics *validation.ValidationMetrics, enqueuedObjectIDs map[string]bool, enqueuedObjectInfo map[string]struct{ Kind, FilePath string }, cachedCheckResults map[string]CheckResult, outputQueue *validation.OutputQueue, operationID string) error {
	vpc, err := initializeValidationProgressContext(cmd, ctx, validator, totalTasks, metrics, enqueuedObjectIDs, enqueuedObjectInfo, cachedCheckResults, outputQueue, operationID)
	if err != nil {
		return err
	}

	if !vpc.SuppressProgress {
		// Clarify that we're validating objects from .zqk/process/ (object ID cache), not all system objects
		// Object ID cache excludes .zqk/ objects (audit_event, metrics, etc.) for performance
		_ = outputQueue.EnqueueStderr(fmt.Sprintf("Validating %d objects (from %s/ via object ID cache)...\n", totalTasks, paths.ProcessDir), true) //nolint:errcheck
	}

	// Use command context for timeout to enable proper cancellation
	ctxTimeout, cancel := stdcontext.WithTimeout(cmd.Context(), vpc.Timeout)
	defer cancel()

	startProgressDrainGoroutine(vpc, ctxTimeout)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctxTimeout.Done():
			return handleValidationTimeout(vpc, ctxTimeout)
		case <-vpc.DrainDone:
			return handleValidationCompletion(vpc)
		case <-vpc.QueueEmpty:
			// Queue just became empty - run completion check immediately instead of waiting for next ticker
			completed, err := handleTickerUpdate(vpc, ticker, ctxTimeout)
			if err != nil {
				return forceExitIfValidationStuck(vpc, validator, err)
			}
			if completed {
				// collectAndOutputFinalResults already ran inside handleTickerUpdate; do not call handleValidationCompletion again
				return err
			}
		case <-ticker.C:
			completed, err := handleTickerUpdate(vpc, ticker, ctxTimeout)
			if err != nil {
				return forceExitIfValidationStuck(vpc, validator, err)
			}
			if completed {
				// collectAndOutputFinalResults already ran inside handleTickerUpdate; do not call handleValidationCompletion again
				return err
			}
		}
	}
}

// determineValidationPriority determines validation priority for an object
func determineValidationPriority(kind, objectID string, validator *validation.AsyncValidator) int {
	// Check cache for previous violations
	if state, ok := validator.GetCachedState(objectID); ok {
		// If object had Tier 1 issues, prioritize it
		for _, issue := range state.Issues {
			if issue.Tier == 1 && issue.ResolvedAt == nil {
				return 1 // Highest priority
			}
		}
		// If object had Tier 2 issues, medium-high priority
		for _, issue := range state.Issues {
			if issue.Tier == 2 && issue.ResolvedAt == nil {
				return 2
			}
		}
	}

	// Default priority based on object kind importance
	// Critical objects get higher priority
	criticalKinds := map[string]bool{
		objects.KindRole:         true,
		objects.KindAccount:      true,
		objects.KindPolicy:       true,
		objects.KindSchedulerJob: true,
	}

	if criticalKinds[kind] {
		return 2 // High priority for critical objects
	}

	return 3 // Default priority
}

// discoverObjectsParallel discovers objects in parallel across all kinds
// Returns a channel that streams discovered files as they're found (for concurrent validation)
// and a function to collect final results after discovery completes
func discoverObjectsParallel(ctx stdcontext.Context, projectRoot string, operationID string, kinds []string, targetIDs []string, logger logging.Logger, storageProvider storage.ObjectStorageProvider, profile string) (<-chan []scannedFile, func() []scannedFile) {
	startTime := time.Now()
	filesChan := make(chan []scannedFile, len(kinds))
	maxConcurrent := calculateMaxConcurrentWorkers(len(kinds))
	sem := make(chan struct{}, maxConcurrent)

	var wg sync.WaitGroup

	// Periodic progress emission during discovery (best-effort).
	// This gives early feedback even before validation starts.
	var totalFound int64
	progressDone := make(chan struct{})

	// Channel to stream files as they're discovered (for concurrent validation)
	streamChan := make(chan []scannedFile, 100)

	// Create a wrapper channel that updates totalFound as files are sent
	// This ensures progress shows actual file counts during discovery
	// Also streams files to streamChan for concurrent validation
	wrappedFilesChan := make(chan []scannedFile, len(kinds))
	counterBuilder := goroutinelabels.NewGoroutine("discovery_counter_updater", "updating discovery file counter").
		WithContext(ctx).
		AsControlPlane()
	counterBuilder.StartSimple(func() {
		defer close(wrappedFilesChan)
		defer close(streamChan)
		for files := range filesChan {
			atomic.AddInt64(&totalFound, int64(len(files)))
			// Stream files immediately for concurrent validation
			select {
			case streamChan <- files:
			case <-ctx.Done():
				return
			}
			// Also forward to wrappedFilesChan for collection
			select {
			case wrappedFilesChan <- files:
			case <-ctx.Done():
				return
			}
		}
	})

	// Stop discovery progress ticker when discovery completes
	discoveryComplete := make(chan struct{})
	var discoveryCompleteOnce sync.Once
	var lastCount int64
	lastCountChangeTime := startTime // Initialize to start time
	lastProgressEmitTime := startTime
	tickerBud := goroutinelabels.DefaultBudget()
	tickerBuilder := goroutinelabels.NewGoroutine("discovery_progress_ticker", "emitting periodic discovery progress").
		WithContext(ctx).
		WithCleanup(func() { close(progressDone) })
	if tickerBud != nil {
		tickerBuilder = tickerBuilder.WithBudget(tickerBud)
	}
	tickerBuilder.StartSimple(func() {
		t := time.NewTicker(1 * time.Second) // Check more frequently (every 1s) for faster early completion
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-discoveryComplete:
				// Discovery completed - stop emitting progress
				return
			case <-t.C:
				currentCount := atomic.LoadInt64(&totalFound)
				now := time.Now()

				// Check if count has changed
				if currentCount != lastCount {
					lastCount = currentCount
					lastCountChangeTime = now
				}

				// If count hasn't changed for 3 seconds, signal early completion
				// Reduced from 5s to 3s for faster response, and checking every 1s instead of 2s
				// This prevents waiting unnecessarily when all files are found
				if lastCount > 0 && now.Sub(lastCountChangeTime) >= 3*time.Second {
					logging.Fluent(logger).Debug("Discovery count stable - signaling early completion").
						Int("stable_count", int(lastCount)).
						String("stable_duration", now.Sub(lastCountChangeTime).String()).
						Log()
					discoveryCompleteOnce.Do(func() {
						close(discoveryComplete)
					})
					return
				}

				// Emit progress every 2 seconds (not every tick)
				if now.Sub(lastProgressEmitTime) >= 2*time.Second {
					lastProgressEmitTime = now
					emitDiscoveryProgressEventViaCoordinator(
						ctx,
						projectRoot,
						storageProvider,
						operationID, // Use system_check operation ID for unified coordination
						"",          // overall
						int(currentCount),
						time.Since(startTime),
						profile,
					)
				}
			}
		}
	})

	// Process each kind in parallel (with bounded concurrency)
	for _, kind := range kinds {
		k := kind // Capture loop variable
		var semAcquired int32

		discoverBud := goroutinelabels.DefaultBudget()
		discoverBuilder := goroutinelabels.NewGoroutine(fmt.Sprintf("discover_kind_%s", k), fmt.Sprintf("discovering objects of kind %s", k)).
			WithWaitGroup(&wg).
			WithContext(ctx).
			WithPreCleanup(func() {
				releaseDiscoverySemaphoreIfAcquired(&semAcquired, sem)
			}).
			WithErrorHandler(func(err error) {
				if err != nil && err != stdcontext.Canceled {
					logging.Fluent(logger).Debug("Error during discovery for kind").
						Kind(k).
						WithError(err).
						Log()
				}
			})
		if discoverBud != nil {
			discoverBuilder = discoverBuilder.WithBudget(discoverBud)
		}
		discoverBuilder.StartSimple(func() {
			kindStart := time.Now()
			// Add per-kind timeout (30 seconds) to prevent one slow kind from blocking all discovery
			kindCtx, kindCancel := stdcontext.WithTimeout(ctx, 30*time.Second)
			defer kindCancel()

			// Acquire semaphore with timeout to prevent blocking indefinitely
			// If we can't acquire within 1 second, skip this kind (others are stuck)
			select {
			case sem <- struct{}{}:
				// Acquired semaphore - proceed with discovery
				atomic.StoreInt32(&semAcquired, 1)
			case <-kindCtx.Done():
				// Context cancelled before we could start
				logging.Fluent(logger).Debug("Kind discovery cancelled before semaphore acquisition").
					Kind(k).
					WithError(kindCtx.Err()).
					Log()
				return
			case <-time.After(1 * time.Second):
				// Can't acquire semaphore - likely all slots taken by stuck goroutines
				logging.Fluent(logger).Warn("Kind discovery skipped - semaphore unavailable (other kinds may be stuck)").
					Kind(k).
					Log()
				return
			}

			// Scan kind and emit a per-kind progress snapshot.
			// Write to filesChan - wrapper goroutine will update counter and forward to wrappedFilesChan
			err := processKindForDiscovery(projectRoot, k, targetIDs, logger, filesChan, sem, kindCtx, storageProvider, profile)
			kindDuration := time.Since(kindStart)

			if err != nil && err != stdcontext.Canceled && err != stdcontext.DeadlineExceeded {
				logging.Fluent(logger).Debug("Error during kind discovery").
					Kind(k).
					WithError(err).
					ElapsedString(kindDuration.String()).
					Log()
			}

			// Log if kind took a long time or timed out
			if kindDuration > 20*time.Second {
				when.When(func() bool { return err == stdcontext.DeadlineExceeded }).Then(func() {
					logging.Fluent(logger).Warn("Kind discovery timed out").
						Kind(k).
						ElapsedString(kindDuration.String()).
						Log()
				}).OrElse(func() {
					logging.Fluent(logger).Info("Kind discovery took longer than expected").
						Kind(k).
						ElapsedString(kindDuration.String()).
						Log()
				}).Run()
			}

			emitDiscoveryProgressEventViaCoordinator(
				ctx,
				projectRoot,
				storageProvider,
				operationID, // Use system_check operation ID for unified coordination
				k,
				-1, // unknown at this level; overall counter is updated when results are collected
				kindDuration,
				profile,
			)
		})
	}

	// Wait for all goroutines to complete OR early completion signal
	// CRITICAL: Use shorter timeout (45 seconds) and proceed immediately after timeout
	// Also check for early completion when count stabilizes (no change for 5s)
	timeout := 45 * time.Second
	waitDone := make(chan struct{})

	// Start wait in goroutine, but don't block on it
	waitBuilder := goroutinelabels.NewGoroutine("discover_wait_group", "waiting for discovery wait group").
		WithContext(ctx).
		AsControlPlane().
		WithCleanup(func() {
			close(waitDone)
		})
	waitBuilder.StartSimple(func() {
		wg.Wait()
	})

	// Wait with timeout OR early completion signal - proceed immediately when count stabilizes
	select {
	case <-waitDone:
		// All goroutines completed normally
		logging.Fluent(logger).Debug("Discovery completed - all goroutines finished").
			Int("total_found", int(atomic.LoadInt64(&totalFound))).
			Log()
	case <-discoveryComplete:
		// Count stabilized - proceed with results found so far
		logging.Fluent(logger).Debug("Discovery count stabilized - proceeding with results found so far").
			Int("total_found", int(atomic.LoadInt64(&totalFound))).
			DiagNote("Some kinds may still be running in background - proceeding with files found so far").
			Log()
	case <-time.After(timeout):
		// Timeout - proceed immediately with partial results
		// Don't wait for wg.Wait() - stuck goroutines will continue in background
		logging.Fluent(logger).Warn("Discovery timeout - proceeding with partial results immediately").
			Timeout(timeout.String()).
			Int("total_found_so_far", int(atomic.LoadInt64(&totalFound))).
			DiagNote("Some kinds may still be running in background - proceeding with files found so far").
			Log()
	case <-ctx.Done():
		// Context cancelled
		logging.Fluent(logger).Debug("Discovery cancelled while waiting").
			WithError(ctx.Err()).
			Log()
	}

	// Close channel to signal discovery completion (even if some goroutines are still running)
	// This allows collection to proceed with partial results
	// CRITICAL: Close filesChan to unblock wrapper goroutine, even if discovery goroutines are stuck
	close(filesChan)

	// Collect results from wrapped channel with timeout
	// Note: totalFound is already updated by the wrapper goroutine as files are discovered
	// The wrapper goroutine reads from filesChan and forwards to wrappedFilesChan
	// If wrapper is stuck, we'll timeout and proceed with empty results (files already counted in totalFound)
	collectionTimeout := 10 * time.Second
	collectionDone := make(chan []scannedFile, 1)

	collectBuilder := goroutinelabels.NewGoroutine("discovery_collect_results", "collecting discovery results").
		WithContext(ctx).
		AsControlPlane()
	collectBuilder.StartSimple(func() {
		allFiles := collectDiscoveryResults(wrappedFilesChan)
		collectionDone <- allFiles
	})

	var allFiles []scannedFile
	select {
	case allFiles = <-collectionDone:
		// Collection completed successfully
		logging.Fluent(logger).Debug("Discovery result collection completed").
			Int("files_collected", len(allFiles)).
			Int("total_found_counter", int(atomic.LoadInt64(&totalFound))).
			Log()
	case <-time.After(collectionTimeout):
		// Collection timeout - wrapper goroutine may be stuck
		// Use the counter value as estimate (files were already counted as they were discovered)
		logging.Fluent(logger).Warn("Discovery result collection timeout - wrapper goroutine may be stuck").
			Timeout(collectionTimeout.String()).
			Int("total_found_counter", int(atomic.LoadInt64(&totalFound))).
			DiagNote("Proceeding with empty file list - files were already counted during discovery").
			Log()
		allFiles = []scannedFile{}
		// Close wrappedFilesChan to unblock wrapper if it's waiting
		// This is safe because we're not reading from it anymore
		select {
		case <-wrappedFilesChan:
			// Drain any remaining items
		default:
		}
	}

	// Stop discovery progress ticker now that discovery is complete
	// Use sync.Once to prevent double-close panic (ticker may have already closed it)
	discoveryCompleteOnce.Do(func() {
		close(discoveryComplete)
	})
	select {
	case <-progressDone:
	default:
	}
	duration := time.Since(startTime)

	// Emit completion event via coordinator
	emitDiscoveryCompletionEventViaCoordinator(
		ctx, projectRoot, storageProvider,
		kinds, "", // targetKind empty for multi-kind discovery
		len(allFiles), duration, profile)

	// Return streaming channel and a function to collect final results
	collectFinalResults := func() []scannedFile {
		return allFiles
	}

	return streamChan, collectFinalResults
}

func releaseDiscoverySemaphoreIfAcquired(semAcquired *int32, sem chan struct{}) {
	// Release only when this goroutine successfully acquired a semaphore slot.
	// Avoid blocking cleanup on timeout/cancel paths that never acquired.
	if atomic.LoadInt32(semAcquired) == 1 {
		<-sem
	}
}

// discoverFromCache streams the evaluation set from the object ID cache (no storage list or directory scan).
// Use when cache is populated for this project so discovery is O(cache) in-memory instead of O(kinds × list/scan).
// Kinds are processed in parallel so the enqueue consumer gets batches sooner and validation can start earlier.
// Returns same shape as discoverObjectsParallel: stream of []scannedFile and collectFinalResults.
func discoverFromCache(ctx stdcontext.Context, projectRoot, operationID string, kinds, targetIDs []string, cache *ObjectIDCache, logger logging.Logger, storageProvider storage.ObjectStorageProvider, profile string) (<-chan []scannedFile, func() []scannedFile) {
	streamChan := make(chan []scannedFile, len(kinds)+1)
	collectChan := make(chan []scannedFile, len(kinds))
	allFilesChan := make(chan []scannedFile, 1)
	startTime := time.Now()
	targetSet := make(map[string]bool)
	for _, id := range targetIDs {
		targetSet[id] = true
	}

	maxConcurrent := calculateMaxConcurrentWorkers(len(kinds))
	kindsCh := make(chan string, len(kinds))
	for _, k := range kinds {
		kindsCh <- k
	}
	close(kindsCh)

	var totalDiscovered int64
	progressDone := make(chan struct{})
	progressBuilder := goroutinelabels.NewGoroutine("discover_cache_progress_heartbeat", "emitting discovery progress from cache").
		AsControlPlane().
		WithContext(ctx)
	progressBuilder.StartSimple(func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		lastCount := int64(0)
		lastEmitTime := time.Now()

		for {
			select {
			case <-progressDone:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				currentCount := atomic.LoadInt64(&totalDiscovered)
				now := time.Now()
				if currentCount != lastCount || now.Sub(lastEmitTime) >= 2*time.Second {
					lastCount = currentCount
					lastEmitTime = now
					emitDiscoveryProgressEventViaCoordinator(
						ctx,
						projectRoot,
						storageProvider,
						operationID,
						"",
						int(currentCount),
						time.Since(startTime),
						profile,
					)
				}
			}
		}
	})

	var workerWg sync.WaitGroup
	for i := 0; i < maxConcurrent; i++ {
		workerID := i
		cacheBuilder := goroutinelabels.NewGoroutine(fmt.Sprintf("discover_cache_worker_%d", workerID), "discovery from cache worker").
			WithContext(ctx).
			WithWaitGroup(&workerWg)
		if cacheBud := goroutinelabels.DefaultBudget(); cacheBud != nil {
			cacheBuilder = cacheBuilder.WithBudget(cacheBud)
		}
		cacheBuilder.StartSimple(func() {
			for kind := range kindsCh {
				if ctx != nil && ctx.Err() != nil {
					return
				}
				entries := cache.GetEntriesByKind(kind)
				if len(entries) == 0 {
					continue
				}
				files := make([]scannedFile, 0, len(entries))
				for _, e := range entries {
					if e == nil || e.ID == emptyValue || e.Kind == emptyValue {
						continue
					}
					if e.FilePath == emptyValue {
						continue
					}
					if len(targetSet) > 0 && !targetSet[e.ID] {
						continue
					}
					path := e.FilePath
					// Object-id-cache often lags CAS hash renames (path still names the
					// deleted blob). Re-resolve via listing index before enqueueing so
					// validators do not emit false "stale index entry" Tier-1 blockers.
					// TRACK: BLI-REDACTED
					if caspkg.CachePathNeedsCASResolve(path) {
						if live, ok := caspkg.ResolveLiveCASFilePath(projectRoot, kind, e.ID); ok {
							path = live
							// Persist heal so the next process does not rediscover ghosts.
							_ = UpdateObjectIDCache(e.ID, kind, live) //nolint:errcheck // best-effort
						} else {
							// Deleted / draft-moved: drop ghost cache entry from this run.
							InvalidateObjectIDCache(e.ID)
							continue
						}
					}
					files = append(files, scannedFile{ObjectID: e.ID, Kind: kind, Path: path})
				}
				if len(files) == 0 {
					continue
				}
				atomic.AddInt64(&totalDiscovered, int64(len(files)))
				select {
				case streamChan <- files:
					collectChan <- files
				case <-ctx.Done():
					return
				}
			}
		})
	}

	// Coordinator: wait for all workers, close stream and collect channels, then emit event.
	// AsControlPlane guarantees this coordinator is never throttled or dropped by worker budgets.
	coordBuilder := goroutinelabels.NewGoroutine("discover_from_cache_coordinator", "closing discovery stream and collecting results").
		WithContext(ctx).
		AsControlPlane()
	coordBuilder.StartSimple(func() {
		workerWg.Wait()
		close(progressDone)
		close(streamChan)
		close(collectChan)
	})

	// Collector: aggregate all batches for collectFinalResults and emit completion event.
	// AsControlPlane guarantees this collector is never throttled or dropped by worker budgets.
	aggBuilder := goroutinelabels.NewGoroutine("discover_from_cache_collector", "aggregating discovery results").
		WithContext(ctx).
		AsControlPlane()
	aggBuilder.StartSimple(func() {
		var all []scannedFile
		for batch := range collectChan {
			all = append(all, batch...)
		}
		allFilesChan <- all
		emitDiscoveryCompletionEventViaCoordinator(ctx, projectRoot, storageProvider, kinds, "", len(all), time.Since(startTime), profile)
	})

	collectFinalResults := func() []scannedFile {
		return <-allFilesChan
	}
	return streamChan, collectFinalResults
}

// scanObjectFiles scans a directory for object files (legacy - use scanObjectFilesWithContext)
func scanObjectFiles(dir, kind string) ([]scannedFile, error) {
	ctx := pkgctx.NewSystemContext()
	return scanObjectFilesWithContext(ctx, dir, kind, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)), nil)
}

// scanObjectFilesWithContext scans a directory for object files with context cancellation and timeout.
// When storageProvider is non-nil and *storage.FileObjectStorage, uses cached CAS to avoid loading the full index (OOM risk).
func scanObjectFilesWithContext(ctx stdcontext.Context, dir, kind string, logger logging.Logger, storageProvider storage.ObjectStorageProvider) ([]scannedFile, error) {
	// FAST PATH: Prefer CAS ID index over walking + reading files.
	// For CAS kinds (hash-based filenames), extracting IDs by reading every file content is expensive
	// and can stall on slow/uninterruptible filesystem operations. The CAS index provides ID -> hash.
	indexPath := filepath.Join(dir, fmt.Sprintf(".%s.index", kind))

	// Check for index file with timeout (non-blocking)
	indexExists := false
	statDone := make(chan bool, 1)
	indexBud := goroutinelabels.DefaultBudget()
	indexBuilder := goroutinelabels.NewGoroutine("scan_check_index", fmt.Sprintf("checking for index file %s", indexPath)).
		WithContext(ctx)
	if indexBud != nil {
		indexBuilder = indexBuilder.WithBudget(indexBud)
	}
	indexBuilder.StartSimple(func() {
		when.When(func() bool {
			_, statErr := fileutil.Stat(indexPath)
			return statErr == nil
		}).Then(func() {
			statDone <- true
		}).OrElse(func() {
			statDone <- false
		}).Run()
	})

	select {
	case indexExists = <-statDone:
		// Got result
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(2 * time.Second):
		// Stat timeout - assume no index, fall through to slow path
		indexExists = false
	}

	if indexExists {
		var cas *storage.ContentAddressableStorage
		if fileStorage := extractFileStorage(storageProvider); fileStorage != nil {
			if c, err := fileStorage.GetContentAddressableStorage(kind); err == nil && c != nil {
				cas = c
			}
		}
		if cas == nil {
			cas = storage.NewContentAddressableStorage(dir, kind)
		}
		mappings, _ := cas.GetAllMappings()
		if len(mappings) > 0 {
			// Pre-scan bucket directories once (bucketed kinds store hash files under YYYY-MM or YYYY-MM-DD folders).
			// Use timeout for ReadDir to prevent blocking
			readDirDone := make(chan []fileutil.DirEntry, 1)
			readDirBud := goroutinelabels.DefaultBudget()
			readDirBuilder := goroutinelabels.NewGoroutine("scan_read_dir", fmt.Sprintf("reading directory %s", dir)).
				WithContext(ctx)
			if readDirBud != nil {
				readDirBuilder = readDirBuilder.WithBudget(readDirBud)
			}
			readDirBuilder.StartSimple(func() {
				entries, _ := fileutil.ReadDir(dir)
				readDirDone <- entries
			})

			var entries []fileutil.DirEntry
			select {
			case entries = <-readDirDone:
				// Got result
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
				// ReadDir timeout - use empty list, will fall back to non-bucketed layout
				entries = []fileutil.DirEntry{}
			}
			datePattern := regexp.MustCompile(`^\d{4}-\d{2}(-\d{2})?$`)
			bucketDirs := make([]string, 0, len(entries))
			for _, e := range entries {
				if e.IsDir() && datePattern.MatchString(e.Name()) {
					bucketDirs = append(bucketDirs, e.Name())
				}
			}

			// OPTIMIZATION: For CAS files with index, trust the index and skip file stat
			// This dramatically speeds up discovery when we have 88k+ files
			// File existence will be verified during validation if needed
			files := make([]scannedFile, 0, len(mappings))
			for objectID, hash := range mappings {
				// Default path assumes non-bucketed layout
				hashFile := filepath.Join(dir, hash+".yaml")

				// If we have bucket directories, use most recent bucket (likely location)
				// Skip expensive stat checks entirely - just construct the path
				// Validation will verify file existence if needed
				if len(bucketDirs) > 0 {
					// Use most recent bucket (last in list, typically most recent date)
					mostRecentBucket := bucketDirs[len(bucketDirs)-1]
					hashFile = filepath.Join(dir, mostRecentBucket, hash+".yaml")
				}

				files = append(files, scannedFile{
					ObjectID: objectID,
					Kind:     kind,
					Path:     hashFile,
				})
			}
			return files, nil
		}
	}

	// Use timeout to prevent indefinite blocking on slow filesystems
	// Per-kind timeout is already applied by caller (30 seconds), so use shorter timeout here
	// to ensure we don't block longer than the per-kind timeout
	walkCtx, cancel := stdcontext.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	// Channel to collect files from walk
	filesChan := make(chan scannedFile, 100)
	walkErrChan := make(chan error, 1)

	// Run filepath.Walk in a goroutine to allow cancellation
	walkBud := goroutinelabels.DefaultBudget()
	walkBuilder := goroutinelabels.NewGoroutine("scan_object_files_walk", fmt.Sprintf("walking directory %s for kind %s", dir, kind)).
		WithContext(walkCtx)
	if walkBud != nil {
		walkBuilder = walkBuilder.WithBudget(walkBud)
	}
	walkBuilder.StartSimple(func() {
		defer close(filesChan)
		walkErr := filepath.Walk(dir, func(path string, info fileutil.FileInfo, err error) error {
			// Check for cancellation periodically (every file)
			select {
			case <-walkCtx.Done():
				return walkCtx.Err()
			default:
			}

			if err != nil {
				return nil // Continue on error
			}

			if info.IsDir() {
				return nil
			}

			if appledouble.SkipPathInTreeWalk(path) {
				return nil
			}

			// Only process YAML files
			if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
				return nil
			}

			// Try to extract object ID from file (with timeout)
			objectID := extractObjectIDFromFileWithContext(walkCtx, path, kind, logger)
			if objectID == emptyValue {
				return nil
			}

			// Send file to channel (non-blocking if channel has capacity)
			select {
			case filesChan <- scannedFile{
				ObjectID: objectID,
				Kind:     kind,
				Path:     path,
			}:
			case <-walkCtx.Done():
				return walkCtx.Err()
			}

			return nil
		})
		walkErrChan <- walkErr
	})

	// Collect files from channel until walk completes or times out
	var files []scannedFile
	collectDone := make(chan struct{})
	collectWalkBud := goroutinelabels.DefaultBudget()
	collectWalkBuilder := goroutinelabels.NewGoroutine("scan_object_files_collect", fmt.Sprintf("collecting files from walk for %s", kind)).
		WithContext(walkCtx)
	if collectWalkBud != nil {
		collectWalkBuilder = collectWalkBuilder.WithBudget(collectWalkBud)
	}
	collectWalkBuilder.StartSimple(func() {
		for file := range filesChan {
			files = append(files, file)
		}
		close(collectDone)
	})

	// Wait for walk to complete or timeout
	select {
	case err := <-walkErrChan:
		// Wait for collection to finish
		<-collectDone
		return files, err
	case <-walkCtx.Done():
		logging.Fluent(logger).Warn("Directory scan timed out or was cancelled").
			String("dir", dir).
			Kind(kind).
			WithError(walkCtx.Err()).
			Log()
		// Wait a bit for collection to finish (with timeout)
		select {
		case <-collectDone:
		case <-time.After(1 * time.Second):
		}
		// Return partial results if we got any
		return files, walkCtx.Err()
	}
}

// scannedFile represents a scanned object file
type scannedFile struct {
	ObjectID string
	Kind     string
	Path     string
}

// extractObjectIDFromFile extracts object ID from file path or content (legacy - use extractObjectIDFromFileWithContext)
func extractObjectIDFromFile(filePath, kind string) string {
	ctx := pkgctx.NewSystemContext()
	return extractObjectIDFromFileWithContext(ctx, filePath, kind, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))
}

// extractObjectIDFromFileWithContext extracts object ID from file path or content with timeout
// For CAS files (hash-based filenames), always extracts from content, not filename
func extractObjectIDFromFileWithContext(ctx stdcontext.Context, filePath, kind string, logger logging.Logger) string {
	base := filepath.Base(filePath)

	// Check if this is a CAS file (hash-based filename)
	// CAS files have 64-character hex hash as filename, so we must extract ID from content
	isCASFile := len(base) == 69 && strings.HasSuffix(base, ".yaml") && isHexString(base[:64])

	// For CAS files, skip filename extraction and go straight to content
	// For non-CAS files, try filename first (e.g., "BLI-001.yaml" -> "BLI-001")
	if !isCASFile {
		if ext := filepath.Ext(base); ext != emptyValue {
			id := base[:len(base)-len(ext)]
			// Basic validation - check if it looks like an object ID (not a hash)
			// Object IDs typically have format like "BLI-001", "CHA-002", etc.
			// Hashes are 64 hex characters, so if it's that long, it's likely a hash
			if id != emptyValue && len(id) < 64 {
				return id
			}
		}
	}

	// Try to read file and extract ID from content (with timeout to prevent blocking)
	type readResult struct {
		data []byte
		err  error
	}
	resultChan := make(chan readResult, 1)

	// Acquire bounded semaphore before starting a read goroutine.
	// If we can't acquire quickly (or context is cancelled), skip reading this file.
	select {
	case extractObjectIDReadSem <- struct{}{}:
		// acquired
	case <-ctx.Done():
		return ""
	case <-time.After(50 * time.Millisecond):
		skipped := atomic.AddUint64(&extractObjectIDSkippedReads, 1)
		// Log occasionally to avoid spam.
		if skipped == 1 || skipped%1000 == 0 {
			logging.Fluent(logger).Warn("Skipping object ID extraction reads due to contention (fallback path)").
				String("skipped_reads", fmt.Sprintf("%d", skipped)).
				Kind(kind).
				Log()
		}
		return ""
	}

	extractBud := goroutinelabels.DefaultBudget()
	extractBuilder := goroutinelabels.NewGoroutine("extract_object_id_read", fmt.Sprintf("reading file %s to extract ID", filepath.Base(filePath))).
		WithContext(ctx)
	if extractBud != nil {
		extractBuilder = extractBuilder.WithBudget(extractBud)
	}
	extractBuilder.StartSimple(func() {
		defer func() { <-extractObjectIDReadSem }()
		data, err := fileutil.ReadFile(filePath)
		resultChan <- readResult{data: data, err: err}
	})

	// Wait for read with timeout (2 seconds max per file to prevent blocking)
	select {
	case result := <-resultChan:
		if result.err != nil {
			return ""
		}
		data := result.data

		// Simple YAML parsing to extract ID
		for line := range strings.SplitSeq(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "id:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					id := strings.TrimSpace(parts[1])
					if id != emptyValue {
						return id
					}
				}
			}
		}
		return ""
	case <-time.After(2 * time.Second):
		// Timeout - log but don't fail (skip this file)
		logging.Fluent(logger).Debug("Timeout reading file to extract ID").
			File(filePath).
			Kind(kind).
			Log()
		return ""
	case <-ctx.Done():
		// Context cancelled
		return ""
	}
}
