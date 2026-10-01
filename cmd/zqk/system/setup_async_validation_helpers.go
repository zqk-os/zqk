package system

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/config"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"

	"github.com/zqk-os/zqk/pkg/objects"
)

// Cached async validation contexts per project root to avoid repeated initialization
// Each context creation loads specs and creates storage, which is expensive
var (
	asyncValidationContextCache   = make(map[string]*AsyncValidationContext)
	asyncValidationContextCacheMu sync.RWMutex
)

// AsyncValidationContext groups state for async validation setup
type AsyncValidationContext struct {
	SpecLoader        *objects.SpecLoader
	LifecycleLoader   *objects.LifecycleLoader
	Validator         validation.Validator
	ObjectIDCache     *ObjectIDCache
	HashRegistryPool  *directoryRegistryPool
	HashRegistryCache *HashRegistryCacheType
	YAMLParser        *parser.YAMLParser
	ProjectRoot       string
	Cmd               *cobra.Command
	Ctx               *cli.Context
	Metrics           *validation.ValidationMetrics
	// StorageProvider is set once per run and reused for all ref validations (avoids creating one per object).
	StorageProvider storage.ObjectStorageProvider
}

// initializeAsyncValidationContext sets up the async validation context
func initializeAsyncValidationContext(cmd *cobra.Command, ctx *cli.Context, projectRoot string, metrics *validation.ValidationMetrics) *AsyncValidationContext {
	specLoader := objects.GetGlobalSpecLoader()
	lifecycleLoader := objects.GetGlobalLifecycleLoader()
	validatorRegistry := validation.GetGlobalRegistry()
	validator := validatorRegistry.Get("")

	hashRegistryPool := &directoryRegistryPool{} // sync.Map initialized automatically
	hashRegistryCache := &HashRegistryCacheType{
		mu:    sync.RWMutex{},
		cache: make(map[string]storage.HashRegistryProvider),
	}
	yamlParser := parser.NewYAMLParser()

	return &AsyncValidationContext{
		SpecLoader:        specLoader,
		LifecycleLoader:   lifecycleLoader,
		Validator:         validator,
		HashRegistryPool:  hashRegistryPool,
		HashRegistryCache: hashRegistryCache,
		YAMLParser:        yamlParser,
		ProjectRoot:       projectRoot,
		Cmd:               cmd,
		Ctx:               ctx,
		Metrics:           metrics,
	}
}

// buildMinimalAsyncValidationContextForEnqueue builds an AsyncValidationContext with Cmd=nil
// for use in background single-object validation (e.g. EnqueueValidationForObject).
// Uses projectRoot to create storage and warm object ID cache; best-effort, returns nil on failure.
// CRITICAL: Uses cached storage provider and caches the context itself to avoid creating new
// FileObjectStorage (and loading all specs) for each object. Each FileObjectStorage creation
// loads all specs recursively, causing memory explosion.
func buildMinimalAsyncValidationContextForEnqueue(projectRoot string) *AsyncValidationContext {
	if projectRoot == emptyValue {
		return nil
	}

	// Check cache first to avoid repeated initialization
	asyncValidationContextCacheMu.RLock()
	cached, exists := asyncValidationContextCache[projectRoot]
	asyncValidationContextCacheMu.RUnlock()
	if exists && cached != nil {
		return cached
	}

	// Create new context
	bgCtx := context.Background() // Background: request-or-shutdown derived
	minimalCtx := cli.ContextForProjectAndProfile(projectRoot, "system")
	metrics := validation.NewValidationMetrics()
	asyncCtx := initializeAsyncValidationContext(nil, minimalCtx, projectRoot, metrics)

	// Use cached storage provider to avoid creating new FileObjectStorage for each object
	// Each FileObjectStorage creation loads all specs recursively, causing massive memory usage
	storageCache := storage.GetGlobalStorageProviderCache()
	storageProvider, err := storageCache.GetOrCreate(bgCtx, projectRoot)
	if err != nil {
		logging.Fluent(getLoggerForSystemCheck(nil, "system")).Debug("Enqueue validation: failed to get cached storage").WithError(err).Log()
		return nil
	}
	asyncCtx.StorageProvider = storageProvider
	asyncCtx.ObjectIDCache = buildObjectIDCacheIfNeeded(nil, minimalCtx, projectRoot, true, false, "", storageProvider, bgCtx)

	// Cache the context for reuse
	asyncValidationContextCacheMu.Lock()
	asyncValidationContextCache[projectRoot] = asyncCtx
	asyncValidationContextCacheMu.Unlock()

	return asyncCtx
}

// getAsyncValidationFlags gets the fast mode and check refs flags
func getAsyncValidationFlags(cmd *cobra.Command, ctx *cli.Context) (fastMode, checkRefs bool) {
	fastMode, err := cmd.Flags().GetBool("fast")
	if err != nil {
		logger := getLoggerForSystemCheck(cmd, ctx.Profile)
		logging.Fluent(logger).Debug("Failed to get fast flag, using default").WithError(err).Log()
		fastMode = false
	}

	checkRefs, err = cmd.Flags().GetBool("check-refs")
	if err != nil {
		logger := getLoggerForSystemCheck(cmd, ctx.Profile)
		logging.Fluent(logger).Debug("Failed to get check-refs flag, using default").WithError(err).Log()
		checkRefs = true // Default to checking refs
	}

	return fastMode, checkRefs
}

// buildObjectIDCacheIfNeeded builds the object ID cache if needed
// coordinatorCacheProgressNotifier emits cache progress via the coordinator so CLI subscribers show it.
type coordinatorCacheProgressNotifier struct {
	ctx         context.Context
	operationID string
}

func (n *coordinatorCacheProgressNotifier) NotifyCacheProgress(status string, message string) {
	emitObjectIDCacheProgressViaCoordinator(n.ctx, n.operationID, status, message)
}

// buildObjectIDCacheIfNeeded loads or builds the object ID cache so validation can use it.
// When --refresh-cache is set, builds synchronously (user explicitly requested fresh cache).
// Otherwise uses a non-blocking path: try load from disk only; if cache not ready, trigger
// background build and return immediately so the command stays snappy. Validation runs with
// cache when available (or best-effort when background build is in progress).
// When storageForWarm is non-nil and cache was loaded, warms that storage from cache.
func buildObjectIDCacheIfNeeded(cmd *cobra.Command, ctx *cli.Context, projectRoot string, checkRefs, fastMode bool, operationID string, storageForWarm storage.ObjectStorageProvider, cancelCtx context.Context) *ObjectIDCache {
	objectIDCache := GetGlobalObjectIDCache()
	forceRebuild := false
	if cmd != nil {
		forceRebuild, _ = cmd.Flags().GetBool("refresh-cache") //nolint:errcheck
	}

	// Blocking path: user requested --refresh-cache; build synchronously.
	// Must run even with --fast / when ref checks are off: refresh is an explicit disk-cache rebuild
	// (e.g. after CAS rotation); skipping it made --refresh-cache --fast a silent no-op.
	if forceRebuild {
		loadCtx := pkgctx.NewSystemContext()
		if cancelCtx != nil {
			loadCtx = cancelCtx
		} else if cmd != nil && cmd.Context() != nil {
			loadCtx = cmd.Context()
		}
		var notifier ObjectIDCacheProgressNotifier
		if operationID != emptyValue {
			notifier = &coordinatorCacheProgressNotifier{ctx: loadCtx, operationID: operationID}
		}
		if err := EnsureObjectIDCacheReady(loadCtx, projectRoot, forceRebuild, notifier, storageForWarm); err != nil {
			logger := getLoggerForSystemCheck(cmd, ctx.Profile)
			logging.Fluent(logger).Warn("Failed to build object ID cache").WithError(err).Log()
			// Continue anyway - validation will use file system lookup as fallback
		} else {
			logger := getLoggerForSystemCheck(cmd, ctx.Profile)

			// Verify cache is loaded and ready. Run with bounded wait so we never hang here
			// (RLock acquisition has no timeout; if another goroutine held Lock we could block forever).
			// Per GOROUTINE_ARCHITECTURE_POLICY: use goroutinelabels.NewGoroutine and select-with-timeout.
			var cacheSize int
			var cacheIsNil bool
			type verifyResult struct {
				size  int
				isNil bool
			}
			verifyCh := make(chan verifyResult, 1)
			verifyBud := goroutinelabels.DefaultBudget()
			verifyBuilder := goroutinelabels.NewGoroutine(LockNameObjectIDCacheVerifyReady, "verifying object ID cache after warm").
				WithPanicHandler(func(r any) {
					logging.Fluent(logger).Warn("Cache verification panic (continuing)").PanicSummary(fmt.Sprint(r)).Log()
					verifyCh <- verifyResult{size: 0, isNil: false}
				})
			if verifyBud != nil {
				verifyBuilder = verifyBuilder.WithBudget(verifyBud)
			}
			verifyBuilder.StartSimple(func() {
				size, isNil := objectIDCache.SnapshotStats()
				verifyCh <- verifyResult{size: size, isNil: isNil}
			})
			select {
			case res := <-verifyCh:
				cacheSize, cacheIsNil = res.size, res.isNil
			case <-time.After(5 * time.Second):
				logging.Fluent(logger).Warn("Object ID cache verification timed out (5s) - continuing; cache may be in use elsewhere").Log()
				cacheIsNil = false
				cacheSize = 0
			}

			if cacheIsNil {
				logging.Fluent(logger).Warn("Object ID cache map is nil after build - cache may not be properly initialized").Log()
			} else if cacheSize == 0 {
				logging.Fluent(logger).Warn("Object ID cache is empty after build - this may cause reference validation issues").Log()
			} else {
				logging.Fluent(logger).Debug("Object ID cache ready for validation").Entries(cacheSize).Log()
				// Diagnostic tracking only when explicitly enabled (avoids O(n) loop on every check)
				if config.MaintenanceCacheDiagnosticObjects().OrDefault("") != emptyValue {
					objectIDs := objectIDCache.CollectIDs()
					registry := GetGlobalCacheItemStrategyRegistry()
					for _, objectID := range objectIDs {
						registry.HandleCacheLoad(objectID, cacheSize, logger)
					}
				}
			}

			// Emit cache availability event via coordinator to ensure cache is tracked
			stdctx := pkgctx.NewSystemContext()
			storageFactory, err := storage.NewStorageFactory(stdctx, projectRoot)
			if err == nil && storageFactory != nil {
				storageProvider := storageFactory.GetStorage()
				if storageProvider != nil {
					emitCacheAvailabilityEventViaCoordinator(
						stdctx,
						projectRoot,
						storageProvider,
						cacheSize > 0, // Available if cache has entries
						cacheSize,
						"async_validation_start",
						ctx.Profile,
					)
				}
			}
		}

		return objectIDCache
	}

	if !checkRefs || fastMode {
		return nil
	}

	// Non-blocking path: use cache if already on disk; otherwise trigger background build and proceed.
	if TryLoadObjectIDCacheOnly(projectRoot) {
		if storageForWarm != nil {
			loadCtx := pkgctx.NewSystemContext()
			if cancelCtx != nil {
				loadCtx = cancelCtx
			} else if cmd != nil && cmd.Context() != nil {
				loadCtx = cmd.Context()
			}
			// Surface warm progress on the same channel as "Loading object ID cache..."
			// (previously notifier was unset here → silent multi-second stall).
			if operationID != emptyValue {
				objectIDCache.SetProgressNotifier(&coordinatorCacheProgressNotifier{ctx: loadCtx, operationID: operationID})
				defer objectIDCache.SetProgressNotifier(nil)
			}
			objectIDCache.NotifyCacheProgress("warming", "Warming CAS indexes...")
			WarmCASIndexesFromCache(loadCtx, projectRoot, objectIDCache, storageForWarm, 0)
		}
		return objectIDCache
	}
	TriggerBackgroundObjectIDCacheBuild(projectRoot)
	return objectIDCache // May be empty; validation is best-effort until background build completes
}

// getHashRegistryForValidation gets the hash registry for validation.
// stdCtx is used when asyncCtx.Cmd is nil (e.g. background single-object validation).
func getHashRegistryForValidation(stdCtx context.Context, asyncCtx *AsyncValidationContext, objectKind, filePath string) storage.HashRegistryProvider {
	kindDir := getKindDirectory(asyncCtx.ProjectRoot, objectKind)
	if kindDir == emptyValue {
		return nil
	}

	fileDir := filepath.Dir(filePath)
	isBucketed := fileDir != kindDir

	if isBucketed {
		return getBucketedHashRegistry(stdCtx, asyncCtx, objectKind, fileDir, kindDir)
	}

	return getNonBucketedHashRegistry(asyncCtx, objectKind, kindDir)
}

// getBucketedHashRegistry gets the hash registry for bucketed objects.
// ctx is used for GetOrCreate when asyncCtx.Cmd is nil.
func getBucketedHashRegistry(ctx context.Context, asyncCtx *AsyncValidationContext, objectKind, fileDir, kindDir string) storage.HashRegistryProvider {
	bucketedStart := time.Now()
	defer func() {
		asyncCtx.Metrics.RecordHashRegistryBucketedProcessing(time.Since(bucketedStart))
	}()

	registryCtx := ctx
	if asyncCtx.Cmd != nil {
		registryCtx = asyncCtx.Cmd.Context()
	}
	registry := asyncCtx.HashRegistryPool.GetOrCreate(registryCtx, objectKind, fileDir)

	loadStart := time.Now()
	if loadErr := registry.Load(); loadErr != nil {
		logger := getLoggerForSystemCheck(asyncCtx.Cmd, asyncCtx.Ctx.Profile)
		logging.Fluent(logger).Warn("Failed to load hash registry").Kind(objectKind).Dir(fileDir).WithError(loadErr).Log()
	}
	loadDuration := time.Since(loadStart)
	asyncCtx.Metrics.RecordHashRegistryLoad(loadDuration)

	return registry
}

// getNonBucketedHashRegistry gets the hash registry for non-bucketed objects
func getNonBucketedHashRegistry(asyncCtx *AsyncValidationContext, objectKind, kindDir string) storage.HashRegistryProvider {
	nonBucketedStart := time.Now()
	defer func() {
		asyncCtx.Metrics.RecordHashRegistryNonBucketedProcessing(time.Since(nonBucketedStart))
	}()

	// Use timeout wrapper for cache read lock (prevents deadlocks)
	cacheKey := objectKind
	var cached storage.HashRegistryProvider
	var found bool

	logger := getLoggerForSystemCheck(asyncCtx.Cmd, asyncCtx.Ctx.Profile)
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	err := concurrency.WithRLockTimeout(
		&asyncCtx.HashRegistryCache.mu,
		ctx,
		asyncCtx.Metrics,
		logging.NewLockLoggerAdapter(logger),
		LockNameHashRegistryCacheGet,
		func() error {
			var ok bool
			cached, ok = asyncCtx.HashRegistryCache.cache[cacheKey]
			found = ok
			return nil
		},
	)
	if err != nil {
		logging.Fluent(logger).Warn("Timeout acquiring hash registry cache read lock, creating new registry").
			Kind(objectKind).
			WithError(err).
			Log()
		// Continue with new registry creation (skip cache)
		registry := storage.NewHashRegistry(pkgctx.NewSystemContext(), objectKind, kindDir)
		loadStart := time.Now()
		if loadErr := registry.Load(); loadErr != nil {
			logging.Fluent(logger).Debug("Failed to load hash registry").Kind(objectKind).WithError(loadErr).Log()
		}
		loadDuration := time.Since(loadStart)
		asyncCtx.Metrics.RecordHashRegistryLoad(loadDuration)
		return registry
	}

	if found && cached != nil {
		// Found in cache - return it
		return cached
	}

	// Not in cache - acquire write lock with timeout
	ctx2, cancel2 := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel2()
	err = concurrency.WithLockTimeout(
		&asyncCtx.HashRegistryCache.mu,
		ctx2,
		asyncCtx.Metrics,
		logging.NewLockLoggerAdapter(logger),
		LockNameHashRegistryCacheSet,
		func() error {
			// Double-check after acquiring write lock
			if existing, ok := asyncCtx.HashRegistryCache.cache[cacheKey]; ok {
				cached = existing
				found = true
				return nil
			}

			// Create registry and add to cache first (without loading)
			registry := storage.NewHashRegistry(pkgctx.NewSystemContext(), objectKind, kindDir)
			asyncCtx.HashRegistryCache.cache[cacheKey] = registry
			cached = registry
			found = true
			return nil
		},
	)
	if err != nil {
		logging.Fluent(logger).Warn("Timeout acquiring hash registry cache write lock, creating uncached registry").
			Kind(objectKind).
			WithError(err).
			Log()
		// Continue with uncached registry
		cached = storage.NewHashRegistry(pkgctx.NewSystemContext(), objectKind, kindDir)
	} else if found && cached != nil {
		// Registry was found or created in cache
		// Continue to load it below
	}

	// Ensure we have a registry (fallback if timeout occurred)
	if cached == nil {
		cached = storage.NewHashRegistry(pkgctx.NewSystemContext(), objectKind, kindDir)
	}

	// Load registry AFTER releasing lock (file I/O can be slow)
	loadStart := time.Now()
	if loadErr := cached.Load(); loadErr != nil {
		logging.Fluent(logger).Debug("Failed to load hash registry").Kind(objectKind).WithError(loadErr).Log()
	}
	loadDuration := time.Since(loadStart)
	asyncCtx.Metrics.RecordHashRegistryLoad(loadDuration)

	return cached
}

// createAsyncValidationFunc creates the validation function for async validation
func createAsyncValidationFunc(asyncCtx *AsyncValidationContext) validation.ValidationFunc {
	// CRITICAL: Verify cache is ready at function creation time
	// This ensures the cache reference is valid and populated before async workers start
	if asyncCtx.ObjectIDCache != nil {
		cacheSize, _ := asyncCtx.ObjectIDCache.SnapshotStats()
		if cacheSize == 0 {
			logger := getLoggerForSystemCheck(asyncCtx.Cmd, asyncCtx.Ctx.Profile)
			logging.Fluent(logger).Warn("Object ID cache not ready when creating async validation function - reference validation may fail").
				Int("cache_size", cacheSize).
				Log()
		}
	}

	return func(stdCtx context.Context, objectID, objectKind, filePath string, content []byte) (*validation.ValidationState, error) {
		// Kind-from-ID is source of truth: use ID-derived kind first, then object's declared kind, then discovery kind
		effectiveKind := inferKindFromID(objectID)
		if effectiveKind != emptyValue {
			objectKind = effectiveKind
		}

		// Parse object
		obj, err := asyncCtx.YAMLParser.ParseBytes(content)
		if err != nil {
			return &validation.ValidationState{
				ObjectID:      objectID,
				ObjectKind:    objectKind,
				FilePath:      filePath,
				LastValidated: time.Now(),
				Issues: []validation.ValidationIssue{
					{
						Tier:        1,
						Category:    "integrity",
						Message:     fmt.Sprintf("failed to parse object: %v", err),
						AutoFixable: false,
					},
				},
			}, nil
		}

		// When inferKindFromID returned empty (e.g. MCP-*), use kind from file content
		if effectiveKind == emptyValue && obj.Kind != emptyValue {
			objectKind = obj.Kind
		}

		// HashRegistry is unused for CAS integrity (checkIntegrity ignores it). Skip load/cache
		// unless --auto-fix may need it — loading .doc_entry.hashes (~180KB) under fan-out
		// contended with the 5s fail-fast budget.
		var registry storage.HashRegistryProvider
		if shouldAutoFix(asyncCtx.Cmd) {
			registry = getHashRegistryForValidation(stdCtx, asyncCtx, objectKind, filePath)
		}

		// Perform validation using the same logic as sync check
		checkCtx := cli.ContextForProjectAndProfile(asyncCtx.ProjectRoot, asyncCtx.Ctx.Profile)

		result := checkObjectWithCacheAndContent(
			checkCtx,
			stdCtx, // Pass context from validation function for cancellation/timeouts
			asyncCtx.Cmd,
			obj,
			filePath,
			objectKind,
			content,
			asyncCtx.SpecLoader,
			asyncCtx.LifecycleLoader,
			asyncCtx.Validator,
			registry,
			asyncCtx.ObjectIDCache,
			asyncCtx.HashRegistryCache,
			nil,                      // No deferred checks in async mode
			asyncCtx.StorageProvider, // Reuse single storage for ref validation (avoids N storage creations per run)
		)

		// Convert CheckResult to ValidationState
		// CRITICAL: AutoFixed results must be preserved in metadata
		state := convertCheckResultToValidationState(&result)

		// Emit auto-fix stored event via coordinator if auto-fix results are present
		if len(result.AutoFixed) > 0 {
			projectRoot := asyncCtx.ProjectRoot
			// Reuse asyncCtx.StorageProvider for coordinator (already set for this run)
			// Generate operation ID for this auto-fix operation
			operationID := fmt.Sprintf("auto_fix_%s_%d", result.ObjectID, time.Now().UnixNano())

			// Emit via coordinator
			emitAutoFixStoredViaCoordinator(
				pkgctx.NewSystemContext(), projectRoot, asyncCtx.StorageProvider, operationID,
				result.ObjectID, result.ObjectKind, result.AutoFixed, state.Metadata["auto_fixed"], asyncCtx.Ctx.Profile,
			)
		}

		return state, nil
	}
}
