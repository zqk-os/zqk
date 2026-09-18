package system

import (
	"github.com/zqk-os/zqk/pkg/datacell"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	stdcontext "context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"
)

var cacheInvalidationHandler cli.CacheInvalidationHandler

func init() {
	// Register the cache invalidation handler with the processor
	// This allows the processor to perform cache invalidation operations
	// without creating circular dependencies
	cacheInvalidationHandler = BulkInvalidateObjectIDCache
	cli.RegisterCacheInvalidationHandler(cacheInvalidationHandler)

	// Register the cache freshness handler
	RegisterCacheFreshnessHandler(PerformCacheFreshnessCheck)
	cli.RegisterCacheFreshnessHandler(PerformCacheFreshnessCheck)

	// Register default cache item strategies (loads from environment if configured)
	RegisterDefaultDiagnosticStrategies()

	// Register a listener for cache invalidation contexts
	// This listener will be triggered when a CacheInvalidationContext reaches StatePending
	// It runs asynchronously to avoid blocking the main thread
	pkgctx.RegisterListener(
		pkgctx.StatePending,
		func(ctx stdcontext.Context, contextObj any) (any, error) {
			// Type assert to CacheInvalidationContext
			cacheCtx, ok := contextObj.(*pkgctx.CacheInvalidationContext)
			if !ok {
				// Not a CacheInvalidationContext, skip (other listeners may handle it)
				// This prevents errors when other context types (e.g., BlockingCheckContext) are processed
				return nil, nil
			}

			// Perform the actual cache invalidation
			if len(cacheCtx.IDs) == 0 {
				return 0, nil
			}

			// Use the registered handler (object ID cache)
			if cacheInvalidationHandler == nil {
				return nil, errfmt.Errorf("cache invalidation handler not registered")
			}

			count := cacheInvalidationHandler(cacheCtx.IDs, cacheCtx.ProjectRoot)

			// Keep validation state cache (violation cache) in sync: invalidate same IDs
			// so the next system check re-validates these objects.
			InvalidateValidationStateCacheForObjects(cacheCtx.ProjectRoot, cacheCtx.IDs)

			return count, nil
		},
		pkgctx.ListenerConfig{
			Name:     "cache_invalidation",
			Async:    true, // Run in goroutine (non-blocking)
			Priority: 0,    // High priority (execute first)
			Required: true, // Must succeed (but only for CacheInvalidationContext)
		},
	)

	// Register CAS index write queue event callbacks for coordinator integration
	// This allows the write queue to emit events via coordinator without import cycles
	caspkg.SetListingIndexBatchEventCallback(emitListingIndexBatchEventViaCoordinator)
	caspkg.SetListingIndexStateChangeEventCallback(emitListingIndexStateChangeEventViaCoordinator)

	// Register I/O queue state change event callback for coordinator integration
	storage.SetIOQueueStateChangeEventCallback(emitIOQueueStateChangeEventViaCoordinator)

	// Register orphan cleanup queue event callback for coordinator integration
	// This allows the cleanup queue to emit events via coordinator without import cycles
	caspkg.SetOrphanCleanupEventCallback(emitOrphanCleanupEventViaCoordinator)

	// Register hash registry event callback for coordinator integration
	// This allows hash registry save operations to emit events via coordinator without import cycles
	storage.SetHashRegistryEventCallback(emitHashRegistryEventViaCoordinator)

	// Register audit buffer flush event callback for coordinator integration
	// This allows audit buffer flush operations to emit events via coordinator without import cycles
	storage.SetAuditBufferFlushEventCallback(emitAuditBufferFlushEventViaCoordinator)

	// Register change journal event callback for coordinator integration
	// This allows change journal entry creation to emit events via coordinator without import cycles
	storage.SetChangeJournalEventCallback(emitChangeJournalEventViaCoordinator)

	// Register all storage queues with shutdown coordinator for graceful shutdown
	// This ensures all queues participate in coordinated shutdown and drain operations
	shutdownCoordinator := storage.GetGlobalShutdownCoordinator()
	shutdownCoordinator.RegisterQueue(caspkg.GetGlobalOrphanCleanupQueue())
	shutdownCoordinator.RegisterQueue(caspkg.GetGlobalListingIndexWriteQueue())
	// Note: init() runs before command context exists, use system context
	// This is acceptable for package initialization
	shutdownCoordinator.RegisterQueue(storage.GetGlobalIOQueueManager(pkgctx.NewSystemContext()))
	shutdownCoordinator.RegisterQueue(storage.GetGlobalHashRegistryManager())

	// Register a listener for cache freshness contexts
	// This listener will be triggered when a CacheFreshnessContext reaches StatePending
	// It runs asynchronously to avoid blocking the main thread
	pkgctx.RegisterListener(
		pkgctx.StatePending,
		func(ctx stdcontext.Context, contextObj any) (any, error) {
			// Type assert to CacheFreshnessContext
			freshnessCtx, ok := contextObj.(*pkgctx.CacheFreshnessContext)
			if !ok {
				// Not a CacheFreshnessContext, skip (other listeners may handle it)
				return nil, nil
			}

			// Perform the actual cache freshness check
			if freshnessCtx.ProjectRoot == emptyValue {
				return 0, nil
			}

			// Use the registered handler
			if cacheFreshnessHandler == nil {
				return nil, errfmt.Errorf("cache freshness handler not registered")
			}

			count := cacheFreshnessHandler(
				freshnessCtx.ProjectRoot,
				freshnessCtx.Reason,
				freshnessCtx.TriggerOperation,
				freshnessCtx.AffectedKinds,
			)
			return map[string]any{"cleaned_count": count}, nil
		},
		pkgctx.ListenerConfig{
			Name:     "cache_freshness",
			Async:    true,  // Run in goroutine (non-blocking)
			Priority: 50,    // Lower priority than invalidation (run after invalidation)
			Required: false, // Not required - best effort
		},
	)

	// Register a listener for blocking check contexts
	// This listener will be triggered when a BlockingCheckContext reaches StatePending
	// It runs synchronously (Required: true) to block write operations if issues are found
	pkgctx.RegisterListener(
		pkgctx.StatePending,
		func(ctx stdcontext.Context, contextObj any) (any, error) {
			// Type assert to BlockingCheckContext
			blockingCtx, ok := contextObj.(*pkgctx.BlockingCheckContext)
			if !ok {
				// Not a BlockingCheckContext, skip (other listeners may handle it)
				return nil, nil
			}

			// If bypass is set, return no blocking issues
			if blockingCtx.Bypass {
				return &pkgctx.BlockingCheckResult{
					HasBlockingIssues: false,
					Issues:            []pkgctx.BlockingIssue{},
				}, nil
			}

			// Perform quick check for blocking issues
			result := performBlockingCheck(blockingCtx)
			return result, nil
		},
		pkgctx.ListenerConfig{
			Name:     "blocking_check",
			Async:    false, // Run synchronously - must complete before write operation
			Priority: 0,     // Highest priority - must run first
			Required: true,  // Must succeed - if it fails, we allow the operation (best effort)
		},
	)
}

// performBlockingCheck performs a quick check for Tier 1 (blocking) issues
// This is a lightweight check that only looks for critical problems that should block write operations
func performBlockingCheck(blockingCtx *pkgctx.BlockingCheckContext) *pkgctx.BlockingCheckResult {
	result := &pkgctx.BlockingCheckResult{
		HasBlockingIssues: false,
		Issues:            []pkgctx.BlockingIssue{},
	}

	// Quick check: use system check infrastructure to find Tier 1 issues
	// We'll use a simplified check that only looks for critical problems
	projectRoot := blockingCtx.ProjectRoot
	if projectRoot == emptyValue {
		// Can't check without project root - allow operation (best effort)
		return result
	}

	// Create a minimal context for checking
	checkCtx := cli.ContextForProjectAndProfile(projectRoot, "system")

	// Use a dummy command for check operations
	dummyCmd := &cobra.Command{}

	// Get all objects and check for Tier 1 issues
	// This is a simplified version that only checks critical problems
	processDir, resolveErr := resolveProcessDirWithResolver(checkCtx.PathResolver())
	if resolveErr != nil {
		processDir = datacell.ProcessPrimaryDir(projectRoot)
	}
	kinds := discoverObjectKinds(processDir)

	// Use global loaders to share caches across sync and async validation
	specLoader := objects.GetGlobalSpecLoader()
	lifecycleLoader := objects.GetGlobalLifecycleLoader()
	validatorRegistry := validation.GetGlobalRegistry()
	validator := validatorRegistry.Get("")

	// Cache hash registries per kind
	hashRegistryCache := &HashRegistryCacheType{
		cache: make(map[string]storage.HashRegistryProvider),
	}

	// Use object ID cache when available; trigger background build if not (keeps command snappy).
	objectIDCache := GetGlobalObjectIDCache()
	if !TryLoadObjectIDCacheOnly(projectRoot) {
		TriggerBackgroundObjectIDCacheBuild(projectRoot)
	}

	// Build list of kinds to check (exclude automated kinds)
	var toCheck []string
	for _, kind := range kinds {
		if kind == objects.KindAuditEvent || kind == objects.KindChangeJournalEntry {
			continue
		}
		toCheck = append(toCheck, kind)
	}
	if len(toCheck) == 0 {
		return result
	}

	// Check kinds in parallel using budgeted pool when available, else bounded semaphore + goroutinelabels
	type kindBlockingResult struct {
		hasBlocking bool
		issues      []pkgctx.BlockingIssue
	}
	kindResults := make(chan kindBlockingResult, len(toCheck))
	maxConcurrency := min(8, len(toCheck))
	queueSize := min(len(toCheck), 256)
	poolCtx := stdcontext.Background() // Background: request-or-shutdown derived
	pool := goroutinelabels.NewPool(goroutinelabels.DefaultBudget(), "blocking_check_kind", "blocking check per kind", maxConcurrency, queueSize)
	pool.Start(poolCtx)
	defer pool.Stop()
	var wg sync.WaitGroup
	for _, kind := range toCheck {
		k := kind
		wg.Add(1)
		_ = pool.Submit(poolCtx, func(taskCtx stdcontext.Context) error {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					kindResults <- kindBlockingResult{hasBlocking: false, issues: nil}
				}
			}()
			checkResults, _, err := CheckKindObjectsWithCache(checkCtx, pkgctx.NewSystemContext(), dummyCmd, k, nil, specLoader, lifecycleLoader, validator, hashRegistryCache, objectIDCache)
			var issues []pkgctx.BlockingIssue
			hasBlocking := false
			if err == nil {
				for _, cr := range checkResults {
					for _, issue := range cr.Issues {
						if issue.Tier == 1 {
							hasBlocking = true
							issues = append(issues, pkgctx.BlockingIssue{
								ObjectID:   cr.ObjectID,
								ObjectKind: cr.ObjectKind,
								Message:    issue.Message,
								Category:   issue.Category,
							})
						}
					}
				}
			}
			kindResults <- kindBlockingResult{hasBlocking: hasBlocking, issues: issues}
			return nil
		})
	}
	waitGroupWithTimeout(&wg, 60*time.Second, "blocking_check_kind_wait")

	for i := 0; i < len(toCheck); i++ {
		r := <-kindResults
		if r.hasBlocking {
			result.HasBlockingIssues = true
		}
		result.Issues = append(result.Issues, r.issues...)
	}
	return result
}

// runCheck implements the check command.
// Default: run validation in a background goroutine and wait for completion; the CLI stays
// responsive and receives progress notifications (discovery + validation heartbeat every 5s).
// Use --background to return immediately with an operation ID; completion is reported via coordinator.
func runCheck(cmd *cobra.Command, args []string) error {
	cli.TouchMeaningfulActivity() // idle watchdog: check command entered (avoids cancel during PreRun/setup when parent is not zqk)
	if err := refuseFastWithMutatingFlags(cmd); err != nil {
		return err
	}
	unlockAutoFix, err := acquireExclusiveAutoFixCheckLock(cmd)
	if err != nil {
		return err
	}
	if unlockAutoFix != nil {
		defer unlockAutoFix()
	}
	runInBackground, _ := cmd.Flags().GetBool("background") //nolint:errcheck

	// Eradicate "jerky" backpressure when run interactively by the user.
	// Users want the check to finish fast, not throttle to 1 worker and poll.
	// We only set this if the user hasn't explicitly configured it, and only for foreground non-scheduler runs.
	if !runInBackground && !shouldUseAsyncFollowForBackground() {
		if zqkenv.HostloadDisable().Get() == "" {
			_ = zqkenv.HostloadDisable().Set("1")
		}
	}
	if runInBackground {
		// When invoked from the scheduler's `run_wrapper` job executor, the caller process
		// exits as soon as `--background` returns. The non-blocking implementation starts
		// work in-process and relies on goroutines surviving, which they won't. That leads
		// to "work returns immediately" behavior and can trigger repeated validations.
		//
		// Detect scheduler wrapper execution via ZQK_JOB_ID (set by run_wrapper) and switch
		// to the async "follow" path so the check actually runs to completion inside the
		// scheduler job budget (instead of returning immediately and doing nothing).
		if shouldUseAsyncFollowForBackground() {
			timeout := CalculateCheckProgressiveTimeout(cmd, args)
			return runCheckAsyncWithFollow(cmd, args, timeout)
		}
		return runCheckAsyncNonBlocking(cmd, args)
	}

	cacheOnly, _ := cmd.Flags().GetBool("cache-only") //nolint:errcheck
	if cacheOnly {
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

		projectRoot := ProjectRootOrResolve(ctx.ProjectRoot)

		// Create validator (loads cache on Start)
		validator := GetAsyncValidator(cmd.Context(), projectRoot, 0)
		if err := validator.Start(); err != nil {
			return err
		}
		defer func() { _ = validator.Stop() }() //nolint:errcheck

		// Trigger background scan if needed (best effort)
		// For now, we just show cached results. In the future, we could trigger a background job here.
		// logger := getLoggerForSystemCheck(cmd, ctx.Profile)
		// logger.Info("Showing cached results only. Run without --cache-only to re-validate.")

		return runCheckFromCache(cmd, ctx, validator)
	}

	// Default: wait for async check; progress is shown via TerminalProgressSubscriber (heartbeat every 5s)
	timeout := CalculateCheckProgressiveTimeout(cmd, args)
	return runCheckAsyncWithFollow(cmd, args, timeout)
}

func shouldUseAsyncFollowForBackground() bool {
	// `run_wrapper` sets ZQK_JOB_ID for child processes so we can tag logs and identify
	// scheduler execution context. In this context, `--background` must *not* return
	// immediately without doing work, otherwise the spawned goroutines get killed with
	// the parent process and the system check never actually runs.
	return zqkenv.JobID().Get() != emptyValue
}

// CheckKindObjectsWithCache checks objects with cached loaders/validators for performance
// Returns deferred checks that should be performed after all fixes are complete
// CheckKindObjectsWithCache checks objects with cached loaders/validators for performance
// Returns deferred checks that should be performed after all fixes are complete
func CheckKindObjectsWithCache(ctx *cli.Context, stdCtx stdcontext.Context, cmd *cobra.Command, kind string, ids []string, specLoader *objects.SpecLoader, lifecycleLoader *objects.LifecycleLoader, validator validation.Validator, hashRegistryCache *HashRegistryCacheType, objectIDCache *ObjectIDCache) ([]CheckResult, []deferredHashCheck, error) {
	checkCtx, err := initializeCheckKindContext(ctx, stdCtx, cmd, kind, ids, specLoader, lifecycleLoader, validator, hashRegistryCache, objectIDCache)
	if err != nil {
		return nil, nil, err
	}

	files, err := scanKindFiles(checkCtx)
	if err != nil {
		return nil, nil, err
	}

	var results []CheckResult
	var allDeferredChecks []deferredHashCheck
	var mu sync.Mutex

	maxConcurrency := 16
	if len(files) < maxConcurrency {
		maxConcurrency = len(files)
	}
	if maxConcurrency == 0 {
		return results, allDeferredChecks, nil
	}

	queueSize := len(files)
	if queueSize > 1024 {
		queueSize = 1024
	}

	poolCtx := stdcontext.Background() // Background: request-or-shutdown derived
	pool := goroutinelabels.NewPool(goroutinelabels.DefaultBudget(), "check_files", "check files parallel", maxConcurrency, queueSize)
	pool.Start(poolCtx)
	defer pool.Stop()

	var wg sync.WaitGroup
	for _, file := range files {
		if !shouldProcessFileForCheck(file, checkCtx.IDs, checkCtx.Logger) {
			continue
		}

		f := file
		wg.Add(1)
		_ = pool.Submit(poolCtx, func(taskCtx stdcontext.Context) error {
			defer wg.Done()

			result, deferredChecksPtr, err := processFileForCheck(checkCtx, f)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				results = append(results, result)
				return nil
			}

			if deferredChecksPtr != nil {
				allDeferredChecks = append(allDeferredChecks, *deferredChecksPtr...)
			}
			results = append(results, result)
			return nil
		})
	}
	waitGroupWithTimeout(&wg, 60*time.Second, "check_kind_files_wait")

	return results, allDeferredChecks, nil
}

// checkKindObjects checks objects of a specific kind (used by tests and check_baseline flow).
// It builds loaders/cache and delegates to CheckKindObjectsWithCache.
func checkKindObjects(ctx *cli.Context, cmd *cobra.Command, kind string, ids []string) ([]CheckResult, error) {
	// Spec/lifecycle loaders resolve under .zqk/specs — never pass project
	// root as the loader directory (that looks for <root>/criteria_lifecycle.yaml).
	specLoader := objects.GetGlobalSpecLoader()
	lifecycleLoader := objects.GetGlobalLifecycleLoader()
	validatorRegistry := validation.GetGlobalRegistry()
	validator := validatorRegistry.Get("") // Default validator
	hashRegistryCache := &HashRegistryCacheType{
		cache: make(map[string]storage.HashRegistryProvider),
	}
	objectIDCache := GetGlobalObjectIDCache()

	projectRoot := ProjectRootOrResolve(ctx.ProjectRoot)
	refreshCache := false
	if cmd != nil {
		refreshCache, _ = cmd.Flags().GetBool("refresh-cache") //nolint:errcheck // non-critical
	}
	if refreshCache {
		loadCtx := pkgctx.NewSystemContext()
		if cmd != nil && cmd.Context() != nil {
			loadCtx = cmd.Context()
		}
		if err := EnsureObjectIDCacheReady(loadCtx, projectRoot, true, nil, nil); err != nil {
			logger := logging.GetLoggerFromProfile(ctx.Profile)
			logging.Fluent(logger).Warn("Failed to build object ID cache for kind check; reference validation may be incomplete").
				WithError(err).
				Log()
		}
	} else {
		if !TryLoadObjectIDCacheOnly(projectRoot) {
			TriggerBackgroundObjectIDCacheBuild(projectRoot)
		}
	}

	results, _, err := CheckKindObjectsWithCache(ctx, pkgctx.NewSystemContext(), cmd, kind, ids, specLoader, lifecycleLoader, validator, hashRegistryCache, objectIDCache)
	return results, err
}

// checkObject performs all checks on a single object
func checkObject(ctx *cli.Context, cmd *cobra.Command, obj *parser.ParsedObject, filePath, kind string) CheckResult {
	checkCtx := initializeCheckObjectLegacyContext(ctx, cmd, obj, filePath, kind)
	checkCtx.Kind = normalizeKindForLegacyCheck(checkCtx)
	return performLegacyChecks(checkCtx)
}

// checkObjectWithCache performs all checks on a single object using cached loaders/validators
//
//nolint:unused // Replaced by checkObjectWithCacheAndContent - reserved for legacy compatibility
func checkObjectWithCache(ctx *cli.Context, cmd *cobra.Command, obj *parser.ParsedObject, filePath, kind string, specLoader *objects.SpecLoader, lifecycleLoader *objects.LifecycleLoader, validator validation.Validator, registry storage.HashRegistryProvider, objectIDCache *ObjectIDCache, hashRegistryCache *HashRegistryCacheType) CheckResult {
	// Read file content for hash calculation
	content, err := fileutil.ReadFile(filePath)
	if err != nil {
		return CheckResult{
			ObjectID:   obj.ID,
			ObjectKind: kind,
			FilePath:   filePath,
			Issues: []Issue{
				{
					Tier:     1,
					Category: "integrity",
					Message:  fmt.Sprintf("Failed to read file: %v", err),
				},
			},
		}
	}
	// Use unified function to get the correct registry for this file
	// This ensures consistency with validation path and handles bucketed objects correctly
	projectRoot := ProjectRootOrResolve(ctx.ProjectRoot)
	fileRegistry := getHashRegistryForFile(cmd.Context(), filePath, kind, projectRoot, hashRegistryCache)
	// Prefer the passed registry if it's the same instance (for non-bucketed objects)
	if registry != nil && registry == fileRegistry {
		// Same instance - use passed one
	} else if registry != nil {
		// Different instance - for non-bucketed, prefer passed registry
		fileDir := filepath.Dir(filePath)
		kindDir := getKindDirectory(projectRoot, kind)
		if fileDir == kindDir {
			// Non-bucketed - prefer passed registry
			fileRegistry = registry
		}
		// For bucketed objects, always use unified function result
	}
	return checkObjectWithCacheAndContent(ctx, pkgctx.NewSystemContext(), cmd, obj, filePath, kind, content, specLoader, lifecycleLoader, validator, fileRegistry, objectIDCache, hashRegistryCache, nil, nil)
}

// checkObjectWithCacheAndContent performs all checks on a single object using cached loaders/validators and pre-read file content.
// deferredChecks is a slice that will accumulate hash integrity checks to be performed after all fixes are complete.
// storageProvider is optional; when set (e.g. async system check) ref validation reuses it instead of creating one per object.
func checkObjectWithCacheAndContent(ctx *cli.Context, stdCtx stdcontext.Context, cmd *cobra.Command, obj *parser.ParsedObject, filePath, kind string, content []byte, specLoader *objects.SpecLoader, lifecycleLoader *objects.LifecycleLoader, validator validation.Validator, registry storage.HashRegistryProvider, objectIDCache *ObjectIDCache, hashRegistryCache *HashRegistryCacheType, deferredChecks *[]deferredHashCheck, storageProvider storage.ObjectStorageProvider) CheckResult {
	checkCtx := initializeCheckObjectContext(ctx, stdCtx, cmd, obj, filePath, kind, content, specLoader, lifecycleLoader, validator, registry, objectIDCache, hashRegistryCache, deferredChecks, storageProvider)

	checkCtx.Kind = normalizeKindForCheck(checkCtx)

	if checkCtx.IsDebugObject {
		logInitialDebugState(checkCtx)
	}

	latestContent, err := readLatestContent(checkCtx)
	if err == nil && len(latestContent) > 0 {
		if checkCtx.IsDebugObject {
			logging.Fluent(checkCtx.Logger).Info("Re-read file from disk").
				Int("latest_content_len", len(latestContent)).
				String("file_path", checkCtx.FilePath).
				Log()
		}
	}

	objMap := parseObjectContent(checkCtx, latestContent)

	if checkCtx.IsDebugObject {
		logFinalDebugState(checkCtx, objMap)
	}

	return performAllChecks(checkCtx, objMap)
}

// waitGroupWithTimeout waits for wg with a hard deadline so bare wg.Wait cannot hang forever
// (goroutine architecture policy — TRACK: BLI-CEF-ARCH-SYSTEM-TRANCHE1).
func waitGroupWithTimeout(wg *sync.WaitGroup, timeout time.Duration, label string) {
	waitDone := make(chan struct{})
	goroutinelabels.NewGoroutine(label, "waiting for wait group").
		WithCleanup(func() { close(waitDone) }).
		StartSimple(func() { wg.Wait() })
	select {
	case <-waitDone:
	case <-time.After(timeout):
	}
}

// Note: Extracted functions are now in separate files:
// - Helper functions: check_impl_helpers.go
// - Output functions: check_impl_output.go
// - Audit buffer: check_impl_audit_buffer.go
// - Validation checks: check_impl_validators.go
// - Integrity checks: check_impl_integrity.go
// - Hash operations: check_impl_hash.go
// - Auto-fix operations: check_impl_autofix.go
