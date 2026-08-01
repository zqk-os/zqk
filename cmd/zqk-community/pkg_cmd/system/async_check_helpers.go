package system

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"time"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/metrics"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/pipeline"
	schedulerpkg "github.com/lanceman/zqk/pkg/scheduler"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/when"
	"github.com/spf13/cobra"
)

const pipelineKindDiscoverAndEnqueueObjects = "system.async_check_discover_and_enqueue_objects"

// resolveProcessDirForProject builds the path alias cache for projectRoot and returns the
// absolute directory for the process tree (prefix:process). Use instead of repeating filepath.Join(projectRoot, paths.ProcessDir); see pkg/datacell.ProcessPrimaryDir for the literal layout.
func resolveProcessDirForProject(projectRoot string) (string, error) {
	return resolveProcessDirWithResolver(paths.NewPathResolver(projectRoot))
}

// resolveProcessDirWithResolver builds the path alias cache and resolves prefix:process via [paths.PathResolver].
// When you already have *cli.Context, pass ctx.PathResolver() so resolution stays tied to CLI orientation.
func resolveProcessDirWithResolver(r paths.PathResolver) (string, error) {
	if r == nil || r.ProjectRoot() == emptyValue {
		return "", errfmt.Errorf("project root is required")
	}
	storagepkg.BuildPathAliasCacheForProject(r.ProjectRoot())
	abs, err := r.ResolveStrict(paths.PathSchemePrefix + "process")
	if err != nil {
		return "", errfmt.Newf("resolve process directory").Wrap(err)
	}
	return abs, nil
}

// resolveProcessDirForAsyncCheck uses the CLI PathResolver when [AsyncCheckContext.Ctx] is set
// so injected overrides ([context.Context.WithPathResolver]) apply; otherwise falls back to project root.
func resolveProcessDirForAsyncCheck(checkCtx *AsyncCheckContext) (string, error) {
	if checkCtx != nil && checkCtx.Ctx != nil {
		return resolveProcessDirWithResolver(checkCtx.Ctx.PathResolver())
	}
	if checkCtx != nil && checkCtx.ProjectRoot != emptyValue {
		return resolveProcessDirForProject(checkCtx.ProjectRoot)
	}
	return "", errfmt.Errorf("project root is required")
}

// AsyncCheckContext groups state for async check execution
type AsyncCheckContext struct {
	Cmd               *cobra.Command
	Ctx               *cli.Context
	ProjectRoot       string
	AsyncValidator    *validation.AsyncValidator
	Metrics           *validation.ValidationMetrics
	Logger            logging.Logger
	ValidatorStarted  bool
	TargetKind        string
	TargetIDs         []string
	TotalTasks        int
	EnqueuedObjectIDs map[string]bool
	// EnqueuedObjectInfo stores kind and file path for each enqueued ID so we can report them in final results
	// when the object has no cached validation state (e.g. scheduler_health_metric is not persisted in the cache).
	EnqueuedObjectInfo map[string]struct{ Kind, FilePath string }
	OpCallback         concurrency.OperationCallback
	OperationID        string // Operation ID for coordination events (shared across discovery + validation)
	// StorageProvider is set once per run and reused for discovery and ref validation (avoids extra storage creation).
	StorageProvider storagepkg.ObjectStorageProvider
}

// initializeAsyncCheckContext sets up the async check context
func initializeAsyncCheckContext(cmd *cobra.Command, ctx *cli.Context) (*AsyncCheckContext, error) {
	projectRoot := ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)

	metrics := validation.NewValidationMetrics()
	logger := getLoggerForSystemCheck(cmd, ctx.Profile)

	// Register cache event subscriber for debugging and monitoring
	// This subscribes to cache-related operational events via the global coordinator
	cacheSubscriber := NewCacheEventSubscriber(ctx.Profile)
	coordinator := coordination.GetCoordinator()
	coordinator.Subscribe(cacheSubscriber)

	// Start scheduler in background only when needed (auto-fix batching).
	// Starting the scheduler can block if the scheduler is misconfigured or its
	// internal channels are not initialized; avoid doing this for pure read-only
	// checks to keep `system check` responsive.
	autoFix := false
	if flag, err := cmd.Flags().GetBool("auto-fix"); err == nil {
		autoFix = flag
	}
	useSchedulerBatching := true
	if flag, err := cmd.Flags().GetBool("auto-fix-scheduler"); err == nil {
		useSchedulerBatching = flag
	}
	if autoFix && useSchedulerBatching {
		// Enqueue cache_prewarm so it runs once when the scheduler starts (instead of waiting for next timer tick).
		if projectRoot != emptyValue {
			triggerQueue := schedulerpkg.NewJobTriggerQueue(projectRoot)
			if err := triggerQueue.EnqueueTriggerRequest(schedulerpkg.DefaultCachePrewarmJobID); err != nil {
				logging.Fluent(logger).Debug("Failed to enqueue cache_prewarm trigger (non-critical)").
					WithError(err).
					String("job_id", schedulerpkg.DefaultCachePrewarmJobID).
					Log()
			}
		}
		startSchedulerInBackground(projectRoot, logger, ctx.Profile)
	}

	return &AsyncCheckContext{
		Cmd:                cmd,
		Ctx:                ctx,
		ProjectRoot:        projectRoot,
		Metrics:            metrics,
		Logger:             logger,
		EnqueuedObjectIDs:  make(map[string]bool),
		EnqueuedObjectInfo: make(map[string]struct{ Kind, FilePath string }),
		OpCallback:         &concurrency.NoOpOperationCallback{},
	}, nil
}

// startSchedulerInBackground starts the scheduler daemon in the background if not already running
// This ensures cache pre-warming jobs can run during system check
func startSchedulerInBackground(projectRoot string, logger logging.Logger, profile string) {
	// Check if scheduler is already running
	sched := schedulerpkg.GetGlobalScheduler()
	if sched != nil && sched.IsRunning() {
		logging.Fluent(logger).Debug("Scheduler already running, skipping background start").Log()
		return
	}

	// Check if scheduler is running in another process (via PID file)
	// IsSchedulerRunning will automatically clean up stale PID files
	// Use timeout to prevent blocking if PID file check hangs
	if projectRoot != emptyValue {
		checkDone := make(chan struct {
			running bool
			pid     int
			err     error
		}, 1)

		statusBud := goroutinelabels.DefaultBudget()
		statusBuilder := goroutinelabels.NewGoroutine("scheduler_status_check", "checking if scheduler is running")
		if statusBud != nil {
			statusBuilder = statusBuilder.WithBudget(statusBud)
		}
		statusBuilder.StartSimple(func() {
			running, pid, err := schedulerpkg.IsSchedulerRunning(projectRoot)
			checkDone <- struct {
				running bool
				pid     int
				err     error
			}{running, pid, err}
		})

		// Wait for check with timeout (2 seconds max)
		select {
		case result := <-checkDone:
			if result.err != nil {
				logging.Fluent(logger).Debug("Failed to check scheduler status, will attempt to start").
					WithError(result.err).
					Log()
			} else if result.running {
				logging.Fluent(logger).Debug("Scheduler daemon already running in another process, skipping background start").
					Int("pid", result.pid).
					Log()
				return
			}
			// If not running, continue to start scheduler (stale PID files are cleaned up by IsSchedulerRunning)
		case <-time.After(2 * time.Second):
			// Timeout - assume scheduler is not running and continue (non-blocking)
			logging.Fluent(logger).Debug("Scheduler status check timed out, assuming not running and continuing").Log()
		}
	}

	// Start scheduler in background goroutine
	bgBud := goroutinelabels.DefaultBudget()
	bgBuilder := goroutinelabels.NewGoroutine("system_check_scheduler_background", "starting scheduler daemon in background")
	if bgBud != nil {
		bgBuilder = bgBuilder.WithBudget(bgBud)
	}
	bgBuilder.StartSimple(func() {
		// Create storage provider for scheduler
		systemCtx := pkgctx.NewSystemContext()
		storageCtx, cancel := context.WithTimeout(systemCtx, 30*time.Second)
		defer cancel()

		storageFactory, err := storagepkg.NewStorageFactory(storageCtx, projectRoot)
		if err != nil {
			logging.Fluent(logger).Debug("Failed to create storage factory for background scheduler (non-critical)").
				WithError(err).
				Log()
			return
		}

		var storageProvider storagepkg.ObjectStorageProvider
		if storageFactory != nil {
			storageProvider = storageFactory.GetStorage()
		}

		// Get or create scheduler
		specLoader := objects.GetGlobalSpecLoader()
		lifecycleLoader := objects.GetGlobalLifecycleLoader()
		sched := schedulerpkg.NewSchedulerWithProjectRoot(storageProvider, specLoader, lifecycleLoader, projectRoot, NewObjectIDCacheBuilderForScheduler())

		if false {
			logging.Fluent(logger).Debug("Failed to create scheduler instance (non-critical)").Log()
			return
		}

		// Set security context
		secCtx := pkgctx.NewSystemSecurityContext()
		sched.SetSecurityContext(secCtx)

		// Create background context that won't be cancelled when check command completes
		// Use system context so scheduler can run independently
		backgroundCtx := pkgctx.NewSystemContext()

		// Start scheduler in background (non-blocking)
		// Note: Start() blocks until context is cancelled, so this goroutine will run until shutdown
		logging.Fluent(logger).Info("Starting scheduler daemon in background for cache pre-warming").
			ProjectRoot(projectRoot).
			Log()
		startErr := sched.Start(backgroundCtx)
		when.When(func() bool { return startErr == nil }).Then(func() {
			logging.Fluent(logger).Info("Scheduler daemon started successfully in background").Log()
		}).OrElse(func() {
			logging.Fluent(logger).Warn("Failed to start scheduler in background (non-critical, cache pre-warming may be slower)").
				WithError(startErr).
				ProjectRoot(projectRoot).
				Log()
			when.When(func() bool { return projectRoot != emptyValue }).Then(func() {
				_ = schedulerpkg.RemovePIDFile(projectRoot) //nolint:errcheck // Best effort cleanup
			}).Run()
		}).Run()
	})
}

func (acc *AsyncCheckContext) getOperationCallback() concurrency.OperationCallback {
	if acc.OpCallback != nil {
		return acc.OpCallback
	}
	return &concurrency.NoOpOperationCallback{}
}

// handleClearCache handles the --clear-cache flag
func handleClearCache(checkCtx *AsyncCheckContext) error {
	clearCache, err := checkCtx.Cmd.Flags().GetBool("clear-cache")
	if err != nil {
		return errfmt.Newf("failed to get clear-cache flag").Wrap(err)
	}

	if !clearCache {
		return nil
	}

	asyncValidator := GetAsyncValidator(checkCtx.Cmd.Context(), checkCtx.ProjectRoot, 0)
	invalidated := asyncValidator.InvalidateIntegrityIssues()
	if err := asyncValidator.ClearCache(); err != nil {
		return errfmt.Newf("failed to clear validation cache").Wrap(err)
	}

	when.When(func() bool { return invalidated > 0 }).Then(func() {
		logging.Fluent(checkCtx.Logger).Info("Validation cache cleared, invalidated cached states with integrity issues").Invalidated(invalidated).Log()
	}).OrElse(func() {
		logging.Fluent(checkCtx.Logger).Info("Validation cache cleared").Log()
	}).Run()

	return nil
}

// setupCPUProfiling sets up CPU profiling if requested
func setupCPUProfiling(checkCtx *AsyncCheckContext) (func(), error) {
	cpuProfile, err := checkCtx.Cmd.Flags().GetString("cpu-profile")
	if err != nil {
		return nil, errfmt.Newf("failed to get cpu-profile flag").Wrap(err)
	}

	if cpuProfile == emptyValue {
		return func() {}, nil
	}

	f, err := os.Create(cpuProfile)
	if err != nil {
		return nil, errfmt.Newf("failed to create CPU profile").Wrap(err)
	}

	if err := pprof.StartCPUProfile(f); err != nil {
		f.Close()
		return nil, errfmt.Newf("failed to start CPU profile").Wrap(err)
	}

	return func() {
		pprof.StopCPUProfile()
		f.Close()
	}, nil
}

// setupGoroutineProfiling sets up goroutine profiling if requested
func setupGoroutineProfiling(checkCtx *AsyncCheckContext) (func(), error) {
	goroutineProfile, err := checkCtx.Cmd.Flags().GetString("goroutine-profile")
	if err != nil {
		return nil, errfmt.Newf("failed to get goroutine-profile flag").Wrap(err)
	}

	if goroutineProfile == emptyValue {
		return func() {}, nil
	}

	// Write initial goroutine profile
	if err := writeGoroutineProfile(goroutineProfile); err != nil {
		logging.Fluent(checkCtx.Logger).Warn("Failed to write initial goroutine profile").WithError(err).Log()
	}

	// Return function to write final profile on cleanup
	return func() {
		if err := writeGoroutineProfile(goroutineProfile); err != nil {
			logging.Fluent(checkCtx.Logger).Warn("Failed to write final goroutine profile").WithError(err).Log()
		}
	}, nil
}

// writeGoroutineProfile writes a goroutine profile to file
func writeGoroutineProfile(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return errfmt.Newf("failed to create goroutine profile").Wrap(err)
	}
	defer f.Close()

	profile := pprof.Lookup("goroutine")
	if profile == nil {
		return errfmt.Errorf("goroutine profile not available")
	}

	if err := profile.WriteTo(f, 0); err != nil {
		return errfmt.Newf("failed to write goroutine profile").Wrap(err)
	}

	return nil
}

// setupMetricsAndCleanup sets up metrics and cleanup handlers
func setupMetricsAndCleanup(checkCtx *AsyncCheckContext) {
	checkCtx.Metrics.Finalize()
	metrics.PersistAsyncValidationMetricsToStorageAsync(
		checkCtx.StorageProvider,
		checkCtx.Metrics,
		checkCtx.OperationID,
		checkCtx.Logger,
	)
	metrics.FlushCacheValidationMetricsToStorage(checkCtx.StorageProvider, checkCtx.Logger)
	metricsFile, err := checkCtx.Cmd.Flags().GetString("metrics-file")
	if err == nil && metricsFile != emptyValue {
		opCallback := checkCtx.getOperationCallback()
		operationID := fmt.Sprintf("async_check_metrics_save_%d", time.Now().UnixNano())
		startTime := time.Now()
		opCallback.OnStart(operationID, map[string]any{
			"metrics_file": metricsFile,
		})

		saveDone := make(chan error, 1)
		metricsBud := goroutinelabels.DefaultBudget()
		metricsBuilder := goroutinelabels.NewGoroutine("async_check_metrics_saver", fmt.Sprintf("saving metrics to %s", metricsFile))
		if metricsBud != nil {
			metricsBuilder = metricsBuilder.WithBudget(metricsBud)
		}
		metricsBuilder.StartSimple(func() {
			saveDone <- checkCtx.Metrics.Save(metricsFile)
		})
		select {
		case err := <-saveDone:
			when.When(func() bool { return err == nil }).Then(func() {
				opCallback.OnComplete(operationID, map[string]any{
					"metrics_file": metricsFile,
				}, time.Since(startTime))
			}).OrElse(func() {
				opCallback.OnError(operationID, err)
				logging.Fluent(checkCtx.Logger).Warn("Failed to save metrics").WithError(err).Log()
			}).Run()
		case <-time.After(5 * time.Second):
			opCallback.OnError(operationID, errfmt.Errorf("timeout saving metrics file"))
			logging.Fluent(checkCtx.Logger).Warn("Timeout saving metrics file - metrics may not be saved").Log()
		}
	}
}

// initializeAsyncValidator initializes the async validator
func initializeAsyncValidator(checkCtx *AsyncCheckContext) error {
	workerCount, err := checkCtx.Cmd.Flags().GetInt("workers")
	if err != nil {
		workerCount = 0
	}

	checkCtx.AsyncValidator = GetAsyncValidator(checkCtx.Cmd.Context(), checkCtx.ProjectRoot, workerCount)
	actualWorkerCount := checkCtx.AsyncValidator.GetMaxWorkers() // Use max workers for metrics
	checkCtx.Metrics.SetWorkerCount(actualWorkerCount)

	// Register async validator with shutdown coordinator for graceful shutdown
	shutdownCoordinator := storagepkg.GetGlobalShutdownCoordinator()
	shutdownCoordinator.RegisterQueue(checkCtx.AsyncValidator) // AsyncValidator implements QueueShutdownHandler

	// CRITICAL: For auto-fix, invalidate only entries with integrity issues instead of clearing entire cache
	// This allows unchanged files (mtime check) to use cache, dramatically speeding up validation
	// shouldUseCachedState() already checks mtime and integrity issues, so we only need to invalidate
	// entries that have integrity issues (which may need fixing)
	if shouldAutoFix(checkCtx.Cmd) {
		clearCache, _ := checkCtx.Cmd.Flags().GetBool("clear-cache")
		if !clearCache {
			// Only invalidate entries with integrity issues - unchanged files will use cache via shouldUseCachedState()
			invalidated := checkCtx.AsyncValidator.InvalidateIntegrityIssues()
			if invalidated > 0 {
				logging.Fluent(checkCtx.Logger).Debug("Invalidated validation cache entries with integrity issues for auto-fix").
					Int("invalidated", invalidated).
					Log()
			} else {
				logging.Fluent(checkCtx.Logger).Debug("No integrity issues in cache to invalidate - will use cache for unchanged files").Log()
			}
		} else {
			// User explicitly requested --clear-cache, so clear everything
			clearErr := checkCtx.AsyncValidator.ClearCache()
			when.When(func() bool { return clearErr == nil }).Then(func() {
				logging.Fluent(checkCtx.Logger).Debug("Cleared validation cache (--clear-cache flag)").Log()
			}).OrElse(func() {
				logging.Fluent(checkCtx.Logger).Debug("Failed to clear validation cache (non-critical)").
					WithError(clearErr).
					Log()
			}).Run()
		}
	}

	return nil
}

// initializeLoggingAndValidators initializes logging and ID validators
func initializeLoggingAndValidators(checkCtx *AsyncCheckContext) {
	logging.Fluent(checkCtx.Logger).Debug("Initializing check command").Profile(checkCtx.Ctx.Profile).Log()

	idValidator := validation.GetIDValidator()
	if err := idValidator.LoadPatterns(); err != nil {
		logging.Fluent(checkCtx.Logger).Warn("Failed to load ID patterns").WithError(err).Log()
	}
}

// startAsyncValidator starts the async validator
func startAsyncValidator(checkCtx *AsyncCheckContext) error {
	if err := checkCtx.AsyncValidator.Start(); err != nil {
		if !errors.Is(err, validation.ErrAlreadyRunning) {
			return errfmt.Newf("failed to start async validator").Wrap(err)
		}
		checkCtx.ValidatorStarted = false
	} else {
		checkCtx.ValidatorStarted = true
	}
	return nil
}

// discoverObjectKindsWithTimeout runs discoverObjectKinds with a timeout to avoid indefinite hang
// (e.g. field registry LoadFields can block on I/O). Uses shared DiscoverObjectKindsWithContext.
// Returns nil on timeout or context cancel.
func discoverObjectKindsWithTimeout(ctx context.Context, processDir string, timeout time.Duration, logger logging.Logger) []string {
	if ctx != nil && ctx.Err() != nil {
		return nil
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	var discoverCtx context.Context
	var cancel context.CancelFunc
	if ctx != nil {
		discoverCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	} else {
		discoverCtx = context.Background()
	}
	kinds := DiscoverObjectKindsWithContext(discoverCtx, processDir)
	if kinds == nil && logger != nil && (ctx == nil || ctx.Err() == nil) {
		logging.Fluent(logger).Warn("Discovering object kinds timed out - using fallback").
			String("timeout", timeout.String()).
			String("process_dir", processDir).
			Log()
	}
	return kinds
}

// determineCheckTarget determines what to check based on args and --ids-from-file.
// Per docs/architecture/system-check-pipeline.md and system-check-performance-targets.md:
// when object ID cache is populated, kinds MUST come from ObjectIDCache.GetKinds() only;
// do not call discoverObjectKinds (field registry) to avoid redundant work and meet cold ≤20s target.
func determineCheckTarget(checkCtx *AsyncCheckContext, args []string) error {
	// Fast iteration: validate only IDs listed in file (skips full discovery of other objects)
	if path, _ := checkCtx.Cmd.Flags().GetString("ids-from-file"); path != emptyValue {
		ids, err := readIDsFromFile(path)
		if err != nil {
			return err
		}
		checkCtx.TargetKind = ""
		checkCtx.TargetIDs = ids
		return nil
	}

	objectIDCache := GetGlobalObjectIDCache()
	processDir, err := resolveProcessDirForAsyncCheck(checkCtx)
	if err != nil {
		return err
	}
	var allKinds []string
	if objectIDCache.IsPopulatedForProject(checkCtx.ProjectRoot) {
		allKinds = objectIDCache.GetKinds()
	}
	if len(allKinds) == 0 {
		allKinds = discoverObjectKindsWithTimeout(checkCtx.Cmd.Context(), processDir, 30*time.Second, checkCtx.Logger)
		if allKinds == nil {
			allKinds = []string{}
		}
	}

	if len(args) == 0 || args[0] == "all" {
		checkCtx.TargetKind = ""
		checkCtx.TargetIDs = nil
		return nil
	}

	isKind := false
	for _, k := range allKinds {
		if args[0] == k {
			isKind = true
			break
		}
	}

	if isKind {
		checkCtx.TargetKind = args[0]
		if len(args) > 1 {
			checkCtx.TargetIDs = args[1:]
		}
	} else {
		checkCtx.TargetKind = ""
		checkCtx.TargetIDs = args
	}

	return nil
}

// discoverAndEnqueueObjects discovers objects and enqueues validation tasks.
// Implements CVS desired_end_state (2): discovery runs under pkg/pipeline with kind pipelineKindDiscoverAndEnqueueObjects
// (see docs/architecture/system-check-pipeline.md). Per-file inventory may still list pipeline_opportunity heuristically;
// the normative path here is the pipeline builder below, not ad-hoc stage wiring.
func discoverAndEnqueueObjects(checkCtx *AsyncCheckContext) error {
	type state struct {
		err error
	}

	st := &state{}

	logger := checkCtx.Logger
	if logger == nil {
		logger = getLoggerForSystemCheck(checkCtx.Cmd, checkCtx.Ctx.Profile)
	}

	runCtx := checkCtx.Cmd.Context()
	if runCtx == nil {
		runCtx = pkgctx.NewSystemContext()
	}

	pl := pipeline.NewBuilder(pipelineKindDiscoverAndEnqueueObjects, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(checkCtx.Ctx.Profile).
		AddStage(pipeline.StageIngest, func(pctx *pipeline.Context, payload any) (any, error) {
			st.err = discoverAndEnqueueObjectsImpl(checkCtx)
			return st, nil
		}).
		AddStage(pipeline.StageFinalize, func(pctx *pipeline.Context, payload any) (any, error) {
			return st, nil
		}).
		Build()

	_, runErr := pl.Run(&pipeline.Context{Ctx: runCtx, Outcome: make(map[string]any)}, st)
	if runErr != nil {
		return runErr
	}
	return st.err
}

func discoverAndEnqueueObjectsImpl(checkCtx *AsyncCheckContext) error {
	processDir, err := resolveProcessDirForAsyncCheck(checkCtx)
	if err != nil {
		return err
	}
	objectIDCache := GetGlobalObjectIDCache()
	useCacheDiscovery := objectIDCache.IsPopulatedForProject(checkCtx.ProjectRoot)

	var kinds []string

	// Use shared storage from run when set (discovery + ref validation share one); else create for this scope only.
	storageProvider := checkCtx.StorageProvider
	if storageProvider == nil && checkCtx.ProjectRoot != emptyValue {
		var storageErr error
		storageProvider, storageErr = storagepkg.NewFileObjectStorage(checkCtx.ProjectRoot)
		if storageErr == nil {
			defer func() { _ = storageProvider.Shutdown(context.Background()) }()
		}
		if storageErr != nil {
			storageProvider = nil
		}
	}

	// Validate cache vs storage counts before picking kinds so we don't use a stale cache
	// (e.g. loaded from disk when it was built before some objects existed).
	// If counts don't match, fall back to storage discovery and trigger rebuild.
	if useCacheDiscovery && storageProvider != nil {
		validateCtx := checkCtx.Cmd.Context()
		if validateCtx == nil {
			validateCtx = pkgctx.NewSystemContext()
		}
		validateCtx, cancel := context.WithTimeout(validateCtx, 5*time.Second)
		fresh := objectIDCache.ValidateCacheFreshnessAgainstStorage(validateCtx, checkCtx.ProjectRoot, storageProvider, 5)
		cancel()
		if !fresh {
			useCacheDiscovery = false
			if checkCtx.Logger != nil {
				logging.Fluent(checkCtx.Logger).Info("Object ID cache count mismatch with storage; using storage discovery and triggering cache rebuild").
					Log()
			}
			objectIDCache.ClearInMemoryCache(checkCtx.ProjectRoot)
			TriggerBackgroundObjectIDCacheForceRebuild(checkCtx.ProjectRoot)
		}
	}

	if checkCtx.TargetKind != emptyValue {
		kinds = []string{checkCtx.TargetKind}
	} else if len(checkCtx.TargetIDs) > 0 {
		// When checking specific IDs, only discover kinds that can contain those IDs.
		kindSet := make(map[string]bool)
		for _, id := range checkCtx.TargetIDs {
			if k := inferKindFromID(id); k != emptyValue {
				kindSet[k] = true
			}
		}
		if len(kindSet) > 0 {
			kinds = make([]string, 0, len(kindSet))
			for k := range kindSet {
				kinds = append(kinds, k)
			}
		} else {
			kinds = objectIDCache.GetKinds()
			if len(kinds) == 0 {
				kinds = discoverObjectKindsWithTimeout(checkCtx.Cmd.Context(), processDir, 30*time.Second, checkCtx.Logger)
				if kinds == nil {
					kinds = []string{}
				}
			}
		}
	} else {
		// When cache is populated, get kinds from cache (avoids field registry LoadFields + GetAllKinds).
		if useCacheDiscovery {
			kinds = objectIDCache.GetKinds()
		}
		if len(kinds) == 0 {
			kinds = discoverObjectKindsWithTimeout(checkCtx.Cmd.Context(), processDir, 30*time.Second, checkCtx.Logger)
			if kinds == nil {
				kinds = []string{}
			}
		}
	}

	// Surface discovery phase via structured logging/coordination instead of
	// writing directly to stderr by default. This keeps us aligned with the
	// logging and notification framework while still giving early visibility.
	if checkCtx.Logger != nil {
		logging.Fluent(checkCtx.Logger).Info("Starting object discovery").
			Int("kind_count", len(kinds)).
			String("target_kind", checkCtx.TargetKind).
			Log()
	}

	profile := systemProfileHuman // Default
	if checkCtx.Ctx != nil && checkCtx.Ctx.Profile != emptyValue {
		profile = checkCtx.Ctx.Profile
	}

	// Emit discovery start event via coordinator (using system_check operation ID for multi-agent coordination)
	operationID := checkCtx.OperationID
	if operationID == emptyValue {
		// Fallback if operation ID not set (shouldn't happen in normal flow)
		operationID = fmt.Sprintf("check_%d", time.Now().UnixNano())
	}

	// Emit discovery start event immediately (synchronously) so subscribers can catch it
	// before discovery work begins
	emitDiscoveryStartEventViaCoordinator(
		checkCtx.Cmd.Context(),
		checkCtx.ProjectRoot,
		storageProvider,
		operationID, // Use same operation ID as system_check for unified coordination
		kinds,
		checkCtx.TargetKind,
		processDir,
		profile,
	)

	// Discovery start message is emitted via emitDiscoveryStartEventViaCoordinator above;
	// TerminalProgressSubscriber prints it (single source, avoids duplicate "Starting discovery across N kinds...").

	enqueueStart := time.Now()

	var filesStream <-chan []scannedFile
	var collectFinalResults func() []scannedFile
	// When checking specific IDs, use storage-based discovery so we find objects that exist on disk
	// but may not be in the object ID cache yet (e.g. new or uncached criteria). Cache-based
	// discovery only returns entries already in the cache, so "check CRIT-9005" would validate
	// 0 objects if CRIT-9005 was never cached.
	useStorageDiscoveryForTargetIDs := len(checkCtx.TargetIDs) > 0
	// Prefer cache-based discovery when the requested IDs are already present in the object ID cache
	// and the cached file paths exist on disk. This avoids the storage-based discovery path missing
	// disk-only YAML objects when stream/CAS indexes lag behind.
	if useCacheDiscovery && useStorageDiscoveryForTargetIDs && objectIDCache != nil && len(checkCtx.TargetIDs) > 0 {
		allTargetsInCache := true
		// For small ID sets (typical CLI usage), confirm cached file paths exist for determinism.
		// For large sets, avoid per-ID fs stats and trust cache presence.
		checkExistence := len(checkCtx.TargetIDs) <= 25
		// Resolve once: resolveProcessDirForProject rebuilds the path cache (reads brand settings);
		// doing that per ID would multiply file opens and work under ulimit.
		var procBase string
		var procResolveErr error
		if checkExistence {
			procBase, procResolveErr = resolveProcessDirForAsyncCheck(checkCtx)
		}
		for _, id := range checkCtx.TargetIDs {
			entry, ok := objectIDCache.Get(id)
			if !ok {
				allTargetsInCache = false
				break
			}
			entry, ok = nildecode.DecodeNonNilPayload[*ObjectIDCacheEntry](entry)
			if !ok || entry.FilePath == emptyValue {
				allTargetsInCache = false
				break
			}
			if checkExistence {
				entryPath := entry.FilePath
				candidates := []string{}
				if filepath.IsAbs(entryPath) {
					candidates = append(candidates, entryPath)
				} else {
					// Best-effort: try common expansions. ObjectIDCacheEntry.FilePath is
					// expected to be absolute, but if it isn't (e.g. cache metadata not
					// loaded for the current process), these candidates keep discovery
					// deterministic for on-disk YAML.
					if procResolveErr != nil {
						candidates = append(candidates,
							filepath.Join(checkCtx.ProjectRoot, entryPath),
							filepath.Join(datacell.ProcessPrimaryDir(checkCtx.ProjectRoot), entryPath),
						)
					} else {
						candidates = append(candidates,
							filepath.Join(checkCtx.ProjectRoot, entryPath),
							filepath.Join(procBase, entryPath),
						)
					}
					if entry.Kind != emptyValue {
						if kindDir := objects.GetDirectoryFromKind(entry.Kind); kindDir != emptyValue {
							if procResolveErr != nil {
								candidates = append(candidates,
									filepath.Join(datacell.CellCASPrimaryDir(checkCtx.ProjectRoot, kindDir), entryPath),
								)
							} else {
								candidates = append(candidates,
									filepath.Join(procBase, kindDir, entryPath),
								)
							}
						}
					}
				}

				found := false
				for _, p := range candidates {
					if _, err := os.Stat(p); err == nil {
						found = true
						break
					}
				}
				if !found {
					allTargetsInCache = false
					break
				}
			}
		}

		if allTargetsInCache {
			useStorageDiscoveryForTargetIDs = false
			if checkCtx.Logger != nil {
				logging.Fluent(checkCtx.Logger).Info("Using object ID cache discovery for requested IDs").
					Int("target_id_count", len(checkCtx.TargetIDs)).
					Log()
			}
		}
	}
	if useCacheDiscovery && !useStorageDiscoveryForTargetIDs {
		if checkCtx.Logger != nil {
			logging.Fluent(checkCtx.Logger).Info("Using object ID cache for discovery (skipping storage list)").
				Int("kind_count", len(kinds)).
				Log()
		}
		filesStream, collectFinalResults = discoverFromCache(checkCtx.Cmd.Context(), checkCtx.ProjectRoot, operationID, kinds, checkCtx.TargetIDs, objectIDCache, checkCtx.Logger, storageProvider, profile)
	} else {
		if useStorageDiscoveryForTargetIDs && checkCtx.Logger != nil {
			logging.Fluent(checkCtx.Logger).Debug("Discovering specific IDs via storage (cache may not contain requested IDs)").
				Int("kind_count", len(kinds)).
				Int("target_id_count", len(checkCtx.TargetIDs)).
				Log()
		}
		filesStream, collectFinalResults = discoverObjectsParallel(checkCtx.Cmd.Context(), checkCtx.ProjectRoot, operationID, kinds, checkCtx.TargetIDs, checkCtx.Logger, storageProvider, profile)
	}

	// Start enqueuing files as they're discovered (concurrent with discovery)
	// This allows validation to start immediately instead of waiting for all discovery to complete
	enqueueDone := make(chan struct{})
	var enqueueErr error
	enqueueBud := goroutinelabels.DefaultBudget()
	enqueueBuilder := goroutinelabels.NewGoroutine("discovery_enqueue_stream", "enqueuing files as they're discovered").
		WithContext(checkCtx.Cmd.Context()).
		WithCleanup(func() {
			close(enqueueDone)
		})
	if enqueueBud != nil {
		enqueueBuilder = enqueueBuilder.WithBudget(enqueueBud)
	}
	enqueueBuilder.StartSimple(func() {
		enqueueErr = enqueueFilesAsDiscovered(checkCtx, filesStream)
	})

	// Wait for enqueue to complete (or discovery to finish)
	select {
	case <-enqueueDone:
		// Enqueue completed
	case <-checkCtx.Cmd.Context().Done():
		// Context cancelled
		logging.Fluent(checkCtx.Logger).Debug("Discovery enqueue cancelled").WithError(checkCtx.Cmd.Context().Err()).Log()
	}

	if enqueueErr != nil {
		return enqueueErr
	}

	checkCtx.Metrics.RecordEnqueue(time.Since(enqueueStart))
	checkCtx.Metrics.SetTotalObjects(checkCtx.TotalTasks)

	// Collect final results for logging/diagnostics (files already enqueued via streaming)
	allFiles := collectFinalResults()
	if len(allFiles) > 0 {
		logging.Fluent(checkCtx.Logger).Debug("Discovery completed").
			Int("total_files_discovered", len(allFiles)).
			Int("total_enqueued", checkCtx.TotalTasks).
			Log()
	}

	return nil
}

// shouldUseCachedState returns true if the cached state is valid to use as a cache hit:
// state is present, cached kind matches ID-derived kind, file unchanged since LastValidated,
// and there are no integrity issues in the cached result.
func shouldUseCachedState(objectID, filePath string, state *validation.ValidationState, bypassCacheWithIssues bool) bool {
	if state == nil {
		return false
	}
	if bypassCacheWithIssues && len(state.Issues) > 0 {
		return false
	}
	effectiveKind := inferKindFromID(objectID)
	if effectiveKind != emptyValue && state.ObjectKind != effectiveKind {
		return false
	}
	if state.FilePath != "" && filePath != "" && filepath.Clean(state.FilePath) != filepath.Clean(filePath) {
		return false
	}
	info, err := os.Stat(filePath)
	if err != nil || info.ModTime().After(state.LastValidated) {
		return false
	}
	for _, issue := range state.Issues {
		if issue.Category == "integrity" {
			return false
		}
	}
	return true
}

// enqueueOrUseCache either records a cache hit or enqueues the file for validation.
// Updates checkCtx (TotalTasks, EnqueuedObjectIDs, Metrics), handledObjectIDs, cacheHits,
// and appends to tasksToEnqueue (flushing when batch is full). pendingByID is optional;
// when non-nil, the objectID is removed from it on enqueue.
// Returns true if a task was enqueued (caller may use for e.g. progress logging).
func enqueueOrUseCache(
	checkCtx *AsyncCheckContext,
	file scannedFile,
	pendingByID map[string]scannedFile,
	tasksToEnqueue *[]validation.ValidationTask,
	batchSize int,
	tasksActuallyEnqueued *int,
	handledObjectIDs map[string]bool,
	cacheHits *int,
) (didEnqueue bool) {
	state, inCache := checkCtx.AsyncValidator.GetCachedState(file.ObjectID)
	bypassCacheWithIssues := false
	if checkCtx.Cmd != nil {
		autoFix, _ := checkCtx.Cmd.Flags().GetBool("auto-fix")
		force, _ := checkCtx.Cmd.Flags().GetBool("force")
		bypassCacheWithIssues = autoFix || force
	}
	if inCache && shouldUseCachedState(file.ObjectID, file.Path, state, bypassCacheWithIssues) {
		checkCtx.Metrics.IncrementCacheHit()
		*cacheHits++
		handledObjectIDs[file.ObjectID] = true
		checkCtx.EnqueuedObjectIDs[file.ObjectID] = true
		checkCtx.EnqueuedObjectInfo[file.ObjectID] = struct{ Kind, FilePath string }{Kind: file.Kind, FilePath: file.Path}
		checkCtx.TotalTasks++
		return false
	}
	checkCtx.Metrics.IncrementCacheMiss()
	checkCtx.EnqueuedObjectIDs[file.ObjectID] = true
	checkCtx.EnqueuedObjectInfo[file.ObjectID] = struct{ Kind, FilePath string }{Kind: file.Kind, FilePath: file.Path}
	handledObjectIDs[file.ObjectID] = true
	checkCtx.TotalTasks++
	if pendingByID != nil {
		delete(pendingByID, file.ObjectID)
	}
	priority := determineValidationPriority(file.Kind, file.ObjectID, checkCtx.AsyncValidator)
	*tasksToEnqueue = append(*tasksToEnqueue, validation.ValidationTask{
		ObjectID:   file.ObjectID,
		ObjectKind: file.Kind,
		FilePath:   file.Path,
		Priority:   priority,
		Checksum:   "",
		EnqueuedAt: time.Now(),
		MaxRetries: 3,
	})
	if len(*tasksToEnqueue) >= batchSize {
		*tasksActuallyEnqueued += len(*tasksToEnqueue)
		checkCtx.AsyncValidator.EnqueueBatch(*tasksToEnqueue)
		*tasksToEnqueue = (*tasksToEnqueue)[:0]
	}
	return true
}

// enqueueFilesAsDiscovered enqueues files for validation as they're discovered.
// When the same ObjectID appears in multiple kind directories (e.g. from a stale or
// wrong CAS index), we prefer the kind that matches the ID prefix (e.g. REQ-026 → requirement)
// so validation runs against the correct spec/lifecycle and wrong-kind violations are avoided.
func enqueueFilesAsDiscovered(checkCtx *AsyncCheckContext, filesStream <-chan []scannedFile) error {
	const batchSize = 100
	tasksToEnqueue := make([]validation.ValidationTask, 0, batchSize)
	cacheHits := 0
	handledObjectIDs := make(map[string]bool)
	pendingByID := make(map[string]scannedFile)
	var tasksActuallyEnqueued int

	for files := range filesStream {
		for _, file := range files {
			if handledObjectIDs[file.ObjectID] {
				continue
			}
			expectedKind := inferKindFromID(file.ObjectID)
			if expectedKind != emptyValue && file.Kind != expectedKind {
				pendingByID[file.ObjectID] = file
				continue
			}
			if enqueueOrUseCache(checkCtx, file, pendingByID, &tasksToEnqueue, batchSize, &tasksActuallyEnqueued, handledObjectIDs, &cacheHits) {
				if checkCtx.TotalTasks%100 == 0 {
					logging.Fluent(checkCtx.Logger).Debug("Enqueuing objects...").
						Int("enqueued", checkCtx.TotalTasks).
						Log()
				}
			}
		}
	}

	// Enqueue deferred (wrong-directory) items only when this path matches the ID's kind.
	// Skip wrong-kind entries (e.g. account:system from requirements/, BAS-* as file_lock_metric);
	// the correct path was already enqueued in the first pass, so skipping avoids duplicate
	// tasks and validation against the wrong spec (which can hang or fail).
	for objectID, file := range pendingByID {
		if handledObjectIDs[objectID] {
			continue
		}
		expectedKind := inferKindFromID(objectID)
		if expectedKind != emptyValue && file.Kind != expectedKind {
			logging.Fluent(checkCtx.Logger).Debug("Skipping wrong-kind discovery entry (correct path already enqueued)").
				String("object_id", objectID).
				String("discovered_kind", file.Kind).
				String("inferred_kind", expectedKind).
				Log()
			continue
		}
		enqueueOrUseCache(checkCtx, file, pendingByID, &tasksToEnqueue, batchSize, &tasksActuallyEnqueued, handledObjectIDs, &cacheHits)
	}
	if len(tasksToEnqueue) > 0 {
		tasksActuallyEnqueued += len(tasksToEnqueue)
		checkCtx.AsyncValidator.EnqueueBatch(tasksToEnqueue)
	}

	// Diagnostic: verify enqueue count matches TotalTasks (ensures no tasks lost between discovery and validator)
	enqueuedIDCount := len(checkCtx.EnqueuedObjectIDs)
	if checkCtx.TotalTasks != enqueuedIDCount || tasksActuallyEnqueued != checkCtx.TotalTasks {
		logging.Fluent(checkCtx.Logger).Warn("Enqueue count mismatch - possible missing or duplicate tasks").
			TotalTasks(checkCtx.TotalTasks).
			EnqueuedObjectIDs(enqueuedIDCount).
			TasksActuallyEnqueued(tasksActuallyEnqueued).
			Log()
	}
	logging.Fluent(checkCtx.Logger).Info("Enqueued objects total").
		Total(checkCtx.TotalTasks).
		EnqueuedIDs(enqueuedIDCount).
		TasksPassedToValidator(tasksActuallyEnqueued).
		CacheHits(cacheHits).
		CacheMisses(checkCtx.TotalTasks).
		Log()

	// Metrics recorded by caller
	checkCtx.Metrics.SetTotalObjects(checkCtx.TotalTasks)

	return nil
}

// createOutputQueueAndWriter creates the output queue and writer
func createOutputQueueAndWriter(checkCtx *AsyncCheckContext) (*validation.OutputQueue, *validation.OutputWriter, context.CancelFunc) {
	outputQueue := validation.NewOutputQueue(100000)
	systemCtx := pkgctx.NewSystemContext()
	outputCtx, outputCancel := context.WithCancel(systemCtx)
	outputWriter := validation.NewOutputWriter(outputCtx, outputQueue, checkCtx.Logger)

	return outputQueue, outputWriter, outputCancel
}

// registerOutputHandlers registers output handlers
func registerOutputHandlers(checkCtx *AsyncCheckContext, outputWriter *validation.OutputWriter) error {
	outputPath := cli.GetOutputPath(checkCtx.Cmd)

	stderrHandler := validation.NewWriterOutputHandler(checkCtx.Cmd.ErrOrStderr(), false)
	outputWriter.RegisterHandler("stderr", stderrHandler)

	if outputPath != emptyValue {
		file, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, paths.FilePerm644)
		if err != nil {
			return errfmt.Newf("failed to open output file").Wrap(err)
		}
		defer file.Close()

		fileHandler := validation.NewWriterOutputHandler(file, false)
		outputWriter.RegisterHandler("file", fileHandler)
	}

	return nil
}
