package scheduler

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/when"
)

// ObjectIDCacheBuilder builds the object ID cache (e.g. for cache_prewarm).
// Implementations live in cmd so pkg/scheduler does not depend on cmd.
// BLI-954: optional; when nil, object ID cache is built during next system check.
type ObjectIDCacheBuilder interface {
	BuildCache(ctx context.Context, projectRoot string, forceRebuild bool) error
}

// ValidationScanner enqueues all objects from the object ID cache into the async validator
// for background validation, populating the validation state cache.
// Implementations live in cmd so pkg/scheduler does not import cmd packages.
// Optional: when nil, the Tier 4 validation scan step is skipped.
type ValidationScanner interface {
	// EnqueueAll enqueues all objects in the object ID cache for background validation.
	// Enqueue is fast (non-blocking queue push); workers drain the queue in background goroutines.
	// Returns the count of newly enqueued objects and any non-fatal error.
	EnqueueAll(ctx context.Context, projectRoot string) (int, error)
}

// CachePrewarmHandler pre-warms caches (specs, lifecycles, hash registries)
type CachePrewarmHandler struct {
	specLoader           *objects.SpecLoader
	lifecycleLoader      *objects.LifecycleLoader
	storage              storagepkg.ObjectStorageProvider
	projectRoot          string
	objectIDCacheBuilder ObjectIDCacheBuilder // optional; when set, prewarmObjectIDCache calls it
	validationScanner    ValidationScanner    // optional; when set, Tier 4 enqueues all objects for background validation
	logger               logging.Logger
}

// NewCachePrewarmHandler creates a new cache pre-warming handler.
// projectRoot and objectIDCacheBuilder are optional (for object ID cache prewarm); pass "" and nil when not used.
// NewCachePrewarmHandler creates a new cache prewarm handler
func NewCachePrewarmHandler(specLoader *objects.SpecLoader, lifecycleLoader *objects.LifecycleLoader, storage storagepkg.ObjectStorageProvider, projectRoot string, objectIDCacheBuilder ObjectIDCacheBuilder) CachePrewarmHandlerInterface {
	return &CachePrewarmHandler{
		specLoader:           specLoader,
		lifecycleLoader:      lifecycleLoader,
		storage:              storage,
		projectRoot:          projectRoot,
		objectIDCacheBuilder: objectIDCacheBuilder,
		logger:               logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// WithValidationScanner sets the optional ValidationScanner used in Tier 4 to enqueue
// all cached objects for background validation after the object ID cache is built.
func (h *CachePrewarmHandler) WithValidationScanner(scanner ValidationScanner) CachePrewarmHandlerInterface {
	h.validationScanner = scanner
	return h
}

// tierTask defines one task in a parallel prewarm tier (goroutine name, log label, and function).
type tierTask struct {
	GoroutineName string
	LogLabel      string // e.g. "lifecycle cache (Tier 2)" for "Failed to pre-warm ..." logs
	Fn            func(context.Context) error
}

// tierTimeoutFromJob returns a timeout for a tier, bounded by the job's deadline when present.
// Used so no tier can mask or exceed the job-defined timeout.
func tierTimeoutFromJob(ctx context.Context, defaultTimeout, buffer time.Duration) time.Duration {
	t := defaultTimeout
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < t {
			t = remaining - buffer
			if t < 0 {
				t = 1 * time.Second
			}
		}
	}
	return t
}

// runSequentialTier runs a single task with a tier-scoped context so the tier cannot consume
// the whole job timeout. Used for Tier 1 (base cache). Logs and continues on error (best-effort).
func (h *CachePrewarmHandler) runSequentialTier(ctx context.Context, jobID string, tierName string, defaultTimeout, buffer time.Duration, fn func(context.Context) error) {
	timeout := tierTimeoutFromJob(ctx, defaultTimeout, buffer)
	tierCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := fn(tierCtx); err != nil {
		CachePrewarmLog(h.logger).Warn(LogEventCachePrewarmTierTaskFailed).
			String("tier_label", tierName).
			WithFields(logErrField(err)...).
			Log()
	}
}

// runParallelTier runs multiple tasks in parallel with a tier-scoped context. Creates tierCtx
// before starting tasks so they are cancelled when the tier times out and the wait goroutine
// never blocks forever (see SCHEDULER_OVERLOAD_AND_TIMEOUT.md). Add Tier 4+ by adding another
// runParallelTier call with the same pattern.
func (h *CachePrewarmHandler) runParallelTier(ctx context.Context, jobID string, tierNum int, tierName string, defaultTimeout, buffer time.Duration, tasks []tierTask) {
	timeout := tierTimeoutFromJob(ctx, defaultTimeout, buffer)
	tierCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var wg sync.WaitGroup
	for _, task := range tasks {
		t := task
		goroutinelabels.NewGoroutine(t.GoroutineName, "pre-warming "+t.LogLabel).
			WithWaitGroup(&wg).
			StartWithContext(tierCtx, func(goroutineCtx context.Context) error {
				if err := t.Fn(goroutineCtx); err != nil {
					CachePrewarmLog(h.logger).Warn(LogEventCachePrewarmTierTaskFailed).
						String("tier_label", t.LogLabel).
						WithFields(logErrField(err)...).
						Log()
				}
				return nil
			})
	}

	done := make(chan struct{})
	goroutinelabels.NewGoroutine("scheduler_prewarm_tier"+strconv.Itoa(tierNum)+"_wait", "waiting for "+tierName+" cache pre-warming").
		WithCleanup(func() { close(done) }).
		StartWithContext(tierCtx, func(c context.Context) error {
			wg.Wait()
			return nil
		})
	select {
	case <-done:
		// All tier tasks completed
	case <-tierCtx.Done():
		CachePrewarmLog(h.logger).Warn(LogEventCachePrewarmTierTimedOut).
			JobID(jobID).
			String("tier_name", tierName).
			WithError(tierCtx.Err()).
			Log()
	}
}

// getProjectRoot returns the project root for cache paths. When the handler's projectRoot is empty
// (e.g. scheduler started without explicit root), falls back to storage's GetProjectRoot() so
// object-id-cache.json and reverse-reference-index.json are written to the correct .zqk/cache.
func (h *CachePrewarmHandler) getProjectRoot() string {
	if h.projectRoot != emptyValue {
		return h.projectRoot
	}
	if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
		return fileStorage.GetProjectRoot()
	}
	return ""
}

// Execute pre-warms all caches with dependency-aware tiered processing.
// Each tier uses a job-derived bounded context (runSequentialTier / runParallelTier) so no tier
// masks the job timeout. Tier 4+ can be added by calling runParallelTier with the same pattern.
//
// Note: "Tier" here means execution phase. A different "tier" exists in system check: issue
// severity (1=blocking, 2=warning, 3=informational, 4=recommendation). Recommendation-style,
// ignorable/suppressible messages (e.g. failed to update document, smart housekeeping hints)
// are issued as system-check Tier 4 (--tier 4 --verbose), not as a cache-prewarm execution phase.
//
// Tier 1 (Base): SpecLoader (sequential; must be first, all others depend on it)
// Tier 2 (Derived): LifecycleLoader, FieldRegistry, SystemFieldsRegistry (parallel)
// Tier 3 (Composite): Object ID cache, validation state cache, reverse reference index (parallel)
func (h *CachePrewarmHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	return RunCachePrewarmViaPipeline(ctx, h, job)
}

// executeCachePrewarmCore pre-warms all caches with dependency-aware tiered processing.
// Called from RunCachePrewarmViaPipeline NORMALIZE stage.
func (h *CachePrewarmHandler) executeCachePrewarmCore(ctx context.Context, job *ScheduledJob) error {
	CachePrewarmLog(h.logger).Info(LogEventCachePrewarmStarted).
		JobID(job.ID).
		Log()

	// Check if we're already close to timeout - if so, skip pre-warming
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < 30*time.Second {
			CachePrewarmLog(h.logger).Warn(LogEventCachePrewarmSkippedInsufficientTime).
				JobID(job.ID).
				String("remaining", remaining.String()).
				Log()
			return nil // Best-effort, return success
		}
	}

	// Tier 0: Path alias cache (highest priority, first in order). Any path uses aliases and scheme
	// (prefix:, abs:, web:) resolved from this cache at runtime before resolving on the local system.
	// See [REDACTED-ID] and POL-CODE-1772597921996901000-00b3c1f5.
	h.runSequentialTier(ctx, job.ID, "path alias cache (Tier 0)", 30*time.Second, 5*time.Second, h.prewarmPathAliasCache)

	// Tier 1: Base cache (sequential, no dependencies)
	h.runSequentialTier(ctx, job.ID, "spec cache (Tier 1)", 120*time.Second, 5*time.Second, h.prewarmSpecCache)

	// Check context again before starting Tier 2 (Tier 1 might have taken time)
	select {
	case <-ctx.Done():
		CachePrewarmLog(h.logger).Warn(LogEventCachePrewarmCancelledAfterTier1).
			JobID(job.ID).
			WithError(ctx.Err()).
			Log()
		return nil // Best-effort success
	default:
	}

	// Tier 2: Derived caches (parallel, after Tier 1 completes)
	h.runParallelTier(ctx, job.ID, 2, "Tier 2", 240*time.Second, 10*time.Second, []tierTask{
		{"scheduler_prewarm_lifecycle", "lifecycle cache (Tier 2)", h.prewarmLifecycleCache},
		{"scheduler_prewarm_field_registry", "field registry (Tier 2)", h.prewarmFieldRegistry},
		{"scheduler_prewarm_system_fields", "system fields registry (Tier 2)", h.prewarmSystemFieldsRegistry},
		{"scheduler_prewarm_jobtype_view_cache", "job_type view cache (Tier 2)", h.prewarmJobTypeViewCache},
	})

	// Tier 3a: Object ID cache — sequential with a large tier budget (bounded by job max_runtime via tierTimeoutFromJob).
	// Do not run OID inside runParallelTier's ~50s tier context: EnsureObjectIDCacheReady/BuildCache inherit that
	// context and can cancel before SaveCache on large repos, leaving object-id-cache.json missing while
	// reverse-reference-index.json (independent scan in Tier 3b) still builds.
	h.runSequentialTier(ctx, job.ID, "object ID cache (Tier 3)", 15*time.Minute, 30*time.Second, h.prewarmObjectIDCache)

	// Tier 3b: Validation state + reverse reference (parallel; shorter tier budget)
	h.runParallelTier(ctx, job.ID, 3, "Tier 3 (derived caches)", 50*time.Second, 5*time.Second, []tierTask{
		{"scheduler_prewarm_validation_state_cache", "validation state cache file (Tier 3)", h.prewarmValidationStateCache},
		{"scheduler_prewarm_reverse_reference_index", "reverse reference index (Tier 3)", h.prewarmReverseReferenceIndex},
	})

	// Tier 4: Background validation scan (sequential, after Tier 3 so object ID cache is ready).
	// Enqueues all objects from the object ID cache into the async validator; workers write
	// results to the shared validation state cache. Non-blocking after enqueue: workers run in
	// background goroutines within the scheduler process.
	h.runSequentialTier(ctx, job.ID, "background validation scan (Tier 4)", 4*time.Minute, 15*time.Second, h.prewarmEnqueueValidation)

	// Final context check before returning - if we're past deadline, log but still return success
	select {
	case <-ctx.Done():
		CachePrewarmLog(h.logger).Warn(LogEventCachePrewarmCompletedContextExpired).
			JobID(job.ID).
			WithError(ctx.Err()).
			Log()
	default:
		CachePrewarmLog(h.logger).Info(LogEventCachePrewarmCompleted).
			JobID(job.ID).
			Log()
	}

	return nil
}

// prewarmSpecCache loads all object specs into cache
// Dynamically discovers all spec files from the specs directory instead of using a hardcoded list
//
//nolint:unparam // Always returns nil error - function is designed to always succeed (best-effort)
func (h *CachePrewarmHandler) prewarmSpecCache(ctx context.Context) error {
	// Get specs directory from spec loader (use reflection or add method to get it)
	// For now, use the same logic as findSpecsDir() - we'll need to add a method to SpecLoader
	// or use the global instance which has the specsDir set

	// Use shared logic from pkg/objects (BLI-954)
	specsDir := objects.FindSpecsDir()
	if specsDir == emptyValue {
		CachePrewarmLog(h.logger).Warn(LogEventCachePrewarmSpecsDirNotFound).Log()
		// Fallback to hardcoded list if we can't find directory
		return h.prewarmSpecCacheHardcoded(ctx)
	}

	// Scan directory for all YAML files
	entries, err := fileutil.ReadDir(specsDir)
	if err != nil {
		CachePrewarmLog(h.logger).Warn(LogEventCachePrewarmSpecsDirReadFailed).
			WithFields(logErrField(err)...).
			Log()
		return h.prewarmSpecCacheHardcoded(ctx)
	}

	loadedCount := 0
	skippedCount := 0

	for _, entry := range entries {
		// Check context cancellation before each iteration
		select {
		case <-ctx.Done():
			CachePrewarmLog(h.logger).Debug(LogEventCachePrewarmSpecScanCancelled).
				Int("loaded", loadedCount).
				Int("skipped", skippedCount).
				WithError(ctx.Err()).
				Log()
			return nil // Best-effort, return nil on cancellation
		default:
		}

		if entry.IsDir() {
			continue
		}

		// Only process YAML files
		if !strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}

		// Skip placeholder files
		if strings.HasSuffix(entry.Name(), "_placeholder.yaml") {
			continue
		}

		// Skip base specs that are typically loaded on-demand
		baseName := strings.TrimSuffix(entry.Name(), ".yaml")
		baseName = strings.TrimSuffix(baseName, ".yml")
		if baseName == objects.KindBaseObject || baseName == objects.KindAuditable || baseName == objects.KindWorkInterval || baseName == objects.KindWorkUnit {
			// These are loaded on-demand when needed, skip for pre-warming
			continue
		}

		// Load spec to warm cache
		specFile := entry.Name()
		_, err := h.specLoader.LoadSpecWithInheritance(specFile)
		when.When(func() bool { return err != nil }).Then(func() {
			CachePrewarmLog(h.logger).Debug(LogEventCachePrewarmSpecInvalidOrMissing).
				WithFields(append([]logging.Field{logging.String("spec_file", specFile)}, logErrField(err)...)...).
				Log()
			skippedCount++
		}).OrElse(func() {
			loadedCount++
		}).Run()
	}

	CachePrewarmLog(h.logger).Info(LogEventCachePrewarmSpecCacheCompleted).
		Int("loaded", loadedCount).
		Int("skipped", skippedCount).
		Log()

	return nil
}

// prewarmSpecCacheHardcoded loads a hardcoded list of specs (fallback)
func (h *CachePrewarmHandler) prewarmSpecCacheHardcoded(ctx context.Context) error {
	// Get list of all object kinds (fallback)
	kinds := []string{
		objects.KindBacklogItem, objects.KindMilestone, objects.KindGoal, objects.KindWorkstream, objects.KindPriorityPlan,
		objects.KindRequirement, objects.KindTestCase, objects.KindCriteria, objects.KindDecision, objects.KindRoadmap,
		objects.KindSchedulerJob, objects.KindPolicy, objects.KindQuestion, objects.KindAuditEvent, objects.KindChangeJournalEntry,
	}

	for _, kind := range kinds {
		// Check context cancellation before each iteration
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		specFile := kind + ".yaml"
		if _, err := h.specLoader.LoadSpecWithInheritance(specFile); err != nil {
			// Log but continue - some specs may not exist
			CachePrewarmLog(h.logger).Debug(LogEventCachePrewarmSpecKindLoadSkipped).
				WithFields(append([]logging.Field{logging.String("kind", kind)}, logErrField(err)...)...).
				Log()
		}
	}

	return nil
}

// prewarmLifecycleCache loads all lifecycle definitions into cache
//
//nolint:unparam // Always returns nil error - function is designed to always succeed (best-effort)
func (h *CachePrewarmHandler) prewarmLifecycleCache(ctx context.Context) error {
	// Get list of all object kinds with lifecycles
	kinds := []string{
		objects.KindBacklogItem, objects.KindMilestone, objects.KindGoal, objects.KindWorkstream, objects.KindPriorityPlan,
		objects.KindRequirement, objects.KindTestCase, objects.KindCriteria, objects.KindDecision, objects.KindPolicy, objects.KindQuestion,
	}

	for _, kind := range kinds {
		// Check context cancellation before each iteration
		select {
		case <-ctx.Done():
			CachePrewarmLog(h.logger).Debug(LogEventCachePrewarmLifecycleScanCancelled).
				String("kind", kind).
				WithError(ctx.Err()).
				Log()
			return nil // Best-effort, return nil on cancellation
		default:
		}

		if _, err := h.lifecycleLoader.LoadLifecycle(kind); err != nil {
			// Log but continue - some lifecycles may not exist
			CachePrewarmLog(h.logger).Debug(LogEventCachePrewarmLifecycleKindNotFound).
				WithFields(append([]logging.Field{logging.String("kind", kind)}, logErrField(err)...)...).
				Log()
		}
	}

	return nil
}

// prewarmFieldRegistry pre-warms the FieldRegistry cache
// Depends on: Tier 1 (SpecLoader) - must be pre-warmed first
//
//nolint:unparam // Always returns nil error - function is designed to always succeed (best-effort)
func (h *CachePrewarmHandler) prewarmFieldRegistry(ctx context.Context) error {
	// Check context cancellation before starting expensive operation
	select {
	case <-ctx.Done():
		CachePrewarmLog(h.logger).Debug(LogEventCachePrewarmFieldRegistryCancelledBeforeStart).
			WithError(ctx.Err()).
			Log()
		return nil // Best-effort, return nil on cancellation
	default:
	}

	fieldRegistry := objects.GetGlobalFieldRegistry()
	// Trigger load by calling GetAllKinds() - this will load all specs and extract fields
	// This is expensive (~100ms-1s+) so pre-warming is beneficial
	// NOTE: GetAllKinds() may block on file I/O and doesn't respect context cancellation
	// If it takes too long, the context timeout will cancel the goroutine
	// Best-effort: if it fails, log warning but don't fail the job
	if _, err := fieldRegistry.GetAllKinds(); err != nil {
		CachePrewarmLog(h.logger).Warn(LogEventCachePrewarmFieldRegistryFailed).
			WithFields(logErrField(err)...).
			Log()
		// Return nil to indicate best-effort success - job should not fail due to cache pre-warm issues
		return nil
	}
	return nil
}

// prewarmSystemFieldsRegistry pre-warms the SystemFieldsRegistry cache
// Depends on: Tier 1 (SpecLoader) - must be pre-warmed first
//
//nolint:unparam // Always returns nil error - function is designed to always succeed (best-effort)
func (h *CachePrewarmHandler) prewarmSystemFieldsRegistry(ctx context.Context) error {
	// Check context cancellation before starting operation
	select {
	case <-ctx.Done():
		CachePrewarmLog(h.logger).Debug(LogEventCachePrewarmSystemFieldsCancelledBeforeStart).
			WithError(ctx.Err()).
			Log()
		return nil // Best-effort, return nil on cancellation
	default:
	}

	systemFieldsRegistry := objects.GetGlobalSystemFieldsRegistry()
	// Trigger load by calling GetSystemGeneratedFields() - this will load base_object and auditable specs
	// This is medium cost (~10-100ms) so pre-warming is beneficial
	// NOTE: GetSystemGeneratedFields() may block on file I/O and doesn't respect context cancellation
	// If it takes too long, the context timeout will cancel the goroutine
	// Best-effort: if it fails, log warning but don't fail the job
	if _, err := systemFieldsRegistry.GetSystemGeneratedFields(); err != nil {
		CachePrewarmLog(h.logger).Warn(LogEventCachePrewarmSystemFieldsFailed).
			WithFields(logErrField(err)...).
			Log()
		// Return nil to indicate best-effort success - job should not fail due to cache pre-warm issues
		return nil
	}
	return nil
}

// prewarmJobTypeViewCache builds a derived job_type view cache for fast grouping/listing operations.
// It is derived from:
//   - scheduler_job.yaml enum values via SpecLoader
//   - local job_type handler registry keys
//
// Best-effort: errors are logged and do not fail the cache prewarm job.
func (h *CachePrewarmHandler) prewarmJobTypeViewCache(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	projectRoot := h.getProjectRoot()
	if strings.TrimSpace(projectRoot) == emptyValue {
		projectRoot = inferProjectRootFromSpecsDir()
	}
	if strings.TrimSpace(projectRoot) == emptyValue {
		return nil
	}
	if h.specLoader == nil {
		return nil
	}

	if err := EnsureJobTypeViewCacheReady(ctx, projectRoot, h.specLoader); err != nil {
		CachePrewarmLog(h.logger).Warn(LogEventCachePrewarmJobTypeViewFailed).
			WithFields(append(logErrField(err), logging.String("project_root", projectRoot))...).
			Log()
		return nil
	}
	// Optional dynamic overlay from scheduler_handler_binding system objects.
	prewarmSchedulerHandlerBindingOverlay(ctx, h.storage, h.logger)

	return nil
}

// inferProjectRootFromSpecsDir infers the repo projectRoot purely from the on-disk spec tree.
// This is storage-backend agnostic (CAS vs stream), since it depends only on where the
// specs exist on disk.
func inferProjectRootFromSpecsDir() string {
	specsDir := objects.FindSpecsDir()
	if strings.TrimSpace(specsDir) == emptyValue {
		return ""
	}
	// specsDir ends with: docs/process/_internal/object_specs
	sd := strings.ReplaceAll(specsDir, "\\", "/")
	marker := "/" + strings.ReplaceAll(paths.ProcessInternalObjectSpecsDir, "\\", "/")
	if idx := strings.Index(sd, marker); idx >= 0 {
		return strings.TrimRight(sd[:idx], "/")
	}
	return ""
}

// prewarmPathAliasCache populates the path alias cache (project structure + stream aliases + doc_entry paths)
// so any path resolves via prefix:/abs:/web: at runtime. Runs refresh async with heartbeat; tier waits for completion.
// Context cancellation aborts the refresh; OnHeartbeat logs progress.
//
//nolint:unparam // Always returns nil error - function is designed to always succeed (best-effort)
func (h *CachePrewarmHandler) prewarmPathAliasCache(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	default:
	}
	projectRoot := h.getProjectRoot()
	if projectRoot == emptyValue {
		return nil
	}
	opts := &storagepkg.PathCacheRefreshOptions{
		OnHeartbeat: func(msg string) {
			CachePrewarmLog(h.logger).Debug(LogEventCachePrewarmPathAliasHeartbeat).
				String("heartbeat", msg).
				ProjectRoot(projectRoot).
				Log()
		},
	}
	doneCh := storagepkg.RefreshPathCacheForProject(ctx, projectRoot, h.storage, opts)
	select {
	case <-ctx.Done():
		return nil
	case err := <-doneCh:
		if err != nil {
			CachePrewarmLog(h.logger).Debug(LogEventCachePrewarmPathCacheFinishedWithError).
				ProjectRoot(projectRoot).
				WithError(err).
				Log()
		}
		return nil
	}
}

// prewarmObjectIDCache builds the object ID cache (object-id-cache.json) when a builder is injected (BLI-954).
// When objectIDCacheBuilder is nil or projectRoot is empty, no-op so the next system check can build it.
//
//nolint:unparam // Always returns nil error - function is designed to always succeed (best-effort)
func (h *CachePrewarmHandler) prewarmObjectIDCache(ctx context.Context) error {
	select {
	case <-ctx.Done():
		CachePrewarmLog(h.logger).Debug(LogEventCachePrewarmObjectIDCancelled).
			WithError(ctx.Err()).
			Log()
		return nil
	default:
	}

	if h.objectIDCacheBuilder == nil {
		CachePrewarmLog(h.logger).Info(LogEventCachePrewarmObjectIDSkippedNoBuilder).Log()
		return nil
	}
	projectRoot := h.getProjectRoot()
	if projectRoot == emptyValue {
		CachePrewarmLog(h.logger).Info(LogEventCachePrewarmObjectIDSkippedNoProjectRoot).Log()
		return nil
	}
	err := h.objectIDCacheBuilder.BuildCache(ctx, projectRoot, false)
	when.When(func() bool { return err != nil }).Then(func() {
		CachePrewarmLog(h.logger).Warn(LogEventCachePrewarmObjectIDFailed).
			WithFields(append(logErrField(err), logging.String("project_root", projectRoot))...).
			Log()
	}).OrElse(func() {
		CachePrewarmLog(h.logger).Info(LogEventCachePrewarmObjectIDSucceeded).
			ProjectRoot(projectRoot).
			Log()
	}).Run()
	return nil
}

// prewarmValidationStateCache ensures the validation state cache file (.zqk/cache/validation_cache.json) exists.
// It loads or creates the shared cache and saves so the file is created in the background like other caches.
//
//nolint:unparam // Always returns nil error - function is designed to always succeed (best-effort)
func (h *CachePrewarmHandler) prewarmValidationStateCache(ctx context.Context) error {
	select {
	case <-ctx.Done():
		CachePrewarmLog(h.logger).Debug(LogEventCachePrewarmValidationStateCancelled).
			WithError(ctx.Err()).
			Log()
		return nil
	default:
	}
	projectRoot := h.getProjectRoot()
	if projectRoot == emptyValue {
		CachePrewarmLog(h.logger).Debug(LogEventCachePrewarmValidationStateSkippedNoProjectRoot).Log()
		return nil
	}
	if err := validation.EnsureValidationCacheReady(projectRoot); err != nil {
		CachePrewarmLog(h.logger).Warn(LogEventCachePrewarmValidationStateFailed).
			WithFields(append(logErrField(err), logging.String("project_root", projectRoot))...).
			Log()
		return nil // Best-effort, don't fail the job
	}
	CachePrewarmLog(h.logger).Info(LogEventCachePrewarmValidationStateSucceeded).
		ProjectRoot(projectRoot).
		Log()
	return nil
}

// prewarmEnqueueValidation enqueues all objects from the object ID cache into the async validator
// for background validation (Tier 4). Workers write results to the shared validation state cache
// so subsequent system check --cache-only calls return accurate, complete results.
// Returns immediately after enqueueing; workers drain the queue in background goroutines.
//
//nolint:unparam // Always returns nil error - function is designed to always succeed (best-effort)
func (h *CachePrewarmHandler) prewarmEnqueueValidation(ctx context.Context) error {
	select {
	case <-ctx.Done():
		CachePrewarmLog(h.logger).Debug(LogEventCachePrewarmBackgroundValidationCancelledBeforeStart).
			WithError(ctx.Err()).
			Log()
		return nil
	default:
	}

	if h.validationScanner == nil {
		CachePrewarmLog(h.logger).Debug(LogEventCachePrewarmBackgroundValidationSkippedNoScanner).Log()
		return nil
	}
	projectRoot := h.getProjectRoot()
	if projectRoot == emptyValue {
		CachePrewarmLog(h.logger).Debug(LogEventCachePrewarmBackgroundValidationSkippedNoProjectRoot).Log()
		return nil
	}

	n, err := h.validationScanner.EnqueueAll(ctx, projectRoot)
	if err != nil {
		CachePrewarmLog(h.logger).Warn(LogEventCachePrewarmBackgroundValidationEnqueueFailed).
			WithFields(append(logErrField(err), logging.String("project_root", projectRoot))...).
			Log()
		return nil
	}
	if n == 0 {
		CachePrewarmLog(h.logger).Info(LogEventCachePrewarmBackgroundValidationNothingEnqueued).Log()
	} else {
		CachePrewarmLog(h.logger).Info(LogEventCachePrewarmBackgroundValidationEnqueued).
			Int("enqueued", n).
			ProjectRoot(projectRoot).
			Log()
	}
	return nil
}

// prewarmReverseReferenceIndex loads or builds the reverse reference index so findDependents uses O(1) lookup.
// When cache is missing, builds from scan using kind mapper; saves to .zqk/cache/reverse-reference-index.json.
//
//nolint:unparam // Always returns nil error - function is designed to always succeed (best-effort)
func (h *CachePrewarmHandler) prewarmReverseReferenceIndex(ctx context.Context) error {
	select {
	case <-ctx.Done():
		CachePrewarmLog(h.logger).Debug(LogEventCachePrewarmReverseReferenceCancelled).
			WithError(ctx.Err()).
			Log()
		return nil
	default:
	}
	projectRoot := h.getProjectRoot()
	if projectRoot == emptyValue {
		CachePrewarmLog(h.logger).Debug(LogEventCachePrewarmReverseReferenceSkippedNoProjectRoot).Log()
		return nil
	}
	revIndex := storagepkg.GetGlobalReverseReferenceIndex()
	loaded, errLoad := revIndex.LoadCache(projectRoot)
	if errLoad != nil {
		CachePrewarmLog(h.logger).Debug("Failed to load reverse reference index cache").WithError(errLoad).Log()
	}
	if loaded {
		CachePrewarmLog(h.logger).Debug(LogEventCachePrewarmReverseReferenceAlreadyLoaded).
			ProjectRoot(projectRoot).
			Log()
		return nil
	}
	processDir, err := paths.ResolvePathStrict(projectRoot, "prefix:process")
	if err != nil {
		CachePrewarmLog(h.logger).Debug(LogEventCachePrewarmReverseReferenceSkippedProcessDir).
			WithError(err).
			Log()
		return nil
	}
	km := objects.GetGlobalKindMapper()
	if km == nil {
		CachePrewarmLog(h.logger).Debug(LogEventCachePrewarmReverseReferenceSkippedNoKindMapper).Log()
		return nil
	}
	if err := km.EnsureReady(ctx); err != nil {
		CachePrewarmLog(h.logger).Debug(LogEventCachePrewarmReverseReferenceKindMapperEnsureReadyFailed).
			WithError(err).
			Log()
		return nil
	}
	kinds := km.GetAllKinds()
	if len(kinds) == 0 {
		CachePrewarmLog(h.logger).Debug(LogEventCachePrewarmReverseReferenceSkippedNoKinds).Log()
		return nil
	}
	if err := revIndex.BuildFromScan(projectRoot, processDir, kinds); err != nil {
		CachePrewarmLog(h.logger).Warn(LogEventCachePrewarmReverseReferenceBuildFailed).
			WithFields(append(logErrField(err), logging.String("project_root", projectRoot))...).
			Log()
		return nil
	}
	if err := revIndex.SaveCache(projectRoot); err != nil {
		CachePrewarmLog(h.logger).Warn(LogEventCachePrewarmReverseReferenceSaveFailed).
			WithFields(append(logErrField(err), logging.String("project_root", projectRoot))...).
			Log()
		return nil
	}
	CachePrewarmLog(h.logger).Info(LogEventCachePrewarmReverseReferenceSucceeded).
		ProjectRoot(projectRoot).
		Log()
	return nil
}
