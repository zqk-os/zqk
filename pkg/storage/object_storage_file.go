package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	cliContext "github.com/lanceman/zqk/internal/cli/context"
	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/primaryorch"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
	"github.com/lanceman/zqk/pkg/storage/locknames"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/when"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

var (
	// ErrObjectNotFound is returned when an object doesn't exist
	ErrObjectNotFound = errfmt.Errorf("object not found")
	// ErrObjectExists is returned when trying to create an object that already exists
	ErrObjectExists = errfmt.Errorf("object already exists")
	// ErrVersionConflict is returned when optimistic locking fails
	ErrVersionConflict = errfmt.Errorf("version conflict: object was modified since last read")
	// ErrPermissionDenied is returned when the user doesn't have permission
	ErrPermissionDenied = errfmt.Errorf("permission denied")

	// Cache operation handler - set by CLI layer to avoid circular dependency
	// This function is called with cache context from the operation context
	// Signature: func(cacheCtx *pkgctx.CacheContext) error
	cacheOperationHandler func(*pkgctx.CacheContext) error

	// Cache checker function - set by CLI layer to check if an object exists in cache
	// This allows reference validation to check the cache for objects created in the same batch
	// Signature: func(objectID string) (filePath string, exists bool)
	// Uses atomic.Value for lock-free reads (hot path optimization)
	cacheChecker atomic.Value // Stores func(string) (string, bool)

	// Lifecycle hook handler - set by CLI layer to trigger scheduler jobs on lifecycle transitions
	// This function is called when object status changes
	// Signature: func(ctx context.Context, kind, fromState, toState string, objectData map[string]any) error
	lifecycleHookHandler func(context.Context, string, string, string, map[string]any) error

	// Change notification handler (ITEM-643) - called on create/update/delete so subscribers can react
	// Signature: func(ctx context.Context, operation, kind, id string, objectData map[string]any) error
	// operation is OpCreate, OpUpdate, or OpDelete; objectData is the object (nil for delete if not needed)
	changeNotificationHandler func(context.Context, string, string, string, map[string]any) error

	// Test mode mutex for serializing critical sections in concurrent update tests
	// This helps make optimistic locking tests more reliable by ensuring
	// only one update can be in the critical section (re-check + write) at a time
	testModeUpdateMutex sync.Mutex
)

// SetCacheOperationHandler sets the function to handle cache operations
// This should be called by the CLI layer to wire up cache invalidation
// The handler receives CacheContext from the operation context and performs the appropriate cache operation
func SetCacheOperationHandler(handler func(*pkgctx.CacheContext) error) {
	cacheOperationHandler = handler
}

// GetCacheOperationHandler returns the current cache operation handler
// This allows scheduler handlers to access the cache operation handler
func GetCacheOperationHandler() func(*pkgctx.CacheContext) error {
	return cacheOperationHandler
}

// SetCacheChecker sets the function to check if an object exists in the cache
// This should be called by the CLI layer to enable cache-based reference validation
// The checker receives an object ID and returns the file path and whether it exists
// Uses atomic.Value for lock-free writes (writes are rare, reads are frequent)
func SetCacheChecker(checker func(string) (string, bool)) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	when.When(func() bool { return checker == nil }).Then(func() {
		StorageLog(logger).Info(LogEventStorageObjectFileSetCacheCheckerNilInfo).Log()
	}).OrElse(func() {
		StorageLog(logger).Info(LogEventStorageObjectFileSetCacheCheckerSetInfo).
			Bool(ConstStreamCheckerIsNil, checker == nil).
			Log()
	}).Run()
	// Atomic store - lock-free, thread-safe
	cacheChecker.Store(checker)
	// TODO: Emit coordinator event for cache checker state change
	when.When(func() bool { return checker == nil }).Then(func() {
		StorageLog(logger).Info(LogEventStorageObjectFileCacheCheckerNowNilInfo).Log()
	}).OrElse(func() {
		StorageLog(logger).Info(LogEventStorageObjectFileCacheCheckerNowSetInfo).Log()
	}).Run()
}

// GetCacheChecker returns the current cache checker function
// Uses atomic.Value for lock-free reads (hot path optimization)
func GetCacheChecker() func(string) (string, bool) {
	if val := cacheChecker.Load(); val != nil {
		return val.(func(string) (string, bool))
	}
	return nil
}

// SetLifecycleHookHandler sets the function to handle lifecycle transitions
// This should be called by the CLI layer to wire up scheduler job triggers
// The handler receives lifecycle transition information and triggers appropriate scheduler jobs
func SetLifecycleHookHandler(handler func(context.Context, string, string, string, map[string]any) error) {
	lifecycleHookHandler = handler
}

// SetChangeNotificationHandler sets the function to be called on object create/update/delete (ITEM-643).
// The handler receives operation (OpCreate|OpUpdate|OpDelete), kind, id, and object data (nil for delete if unavailable).
func SetChangeNotificationHandler(handler func(context.Context, string, string, string, map[string]any) error) {
	changeNotificationHandler = handler
}

// executeChangeNotification invokes the change notification handler for create/update/delete (ITEM-643).
// Best-effort, non-blocking: errors are logged but not returned.
func executeChangeNotification(ctx context.Context, operation, kind, id string, objectData map[string]any) {
	if changeNotificationHandler == nil {
		return
	}
	// Run asynchronously so storage path is not blocked (same pattern as lifecycle trigger)
	bud := goroutinelabels.DefaultBudget()
	changeBuilder := goroutinelabels.NewGoroutine(ConstStreamChangeNotification, fmt.Sprintf("%s %s %s", operation, kind, id))
	if bud != nil {
		changeBuilder = changeBuilder.WithBudget(bud)
	}
	changeBuilder.StartSimple(func() {
		if err := changeNotificationHandler(ctx, operation, kind, id, objectData); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Debug(LogEventStorageObjectFileChangeNotifyHandlerErrDebug).
				String("operation", operation).
				Kind(kind).
				ObjectID(id).
				WithError(err).
				Log()
		}
	})
}

func executeLifecycleHook(ctx context.Context, kind, fromState, toState string, objectData map[string]any) error {
	// Only trigger if status actually changed
	if fromState == toState {
		return nil
	}

	// Attentiveness: wake host/project primary orchestrator when a plan-scoped
	// backlog_item enters error (best-effort; never blocks the transition).
	if projectRoot := pkgctx.GetLifecycleProjectRoot(ctx); projectRoot != "" {
		primaryorch.MaybeWakeOnPlanBacklogError(ctx, projectRoot, kind, fromState, toState, objectData)
	}

	// Try handler first (if set by CLI layer)
	if lifecycleHookHandler != nil {
		return lifecycleHookHandler(ctx, kind, fromState, toState, objectData)
	}

	// Fallback to global scheduler registry (if scheduler is running)
	// We use a function call to get the scheduler to avoid direct import
	getGlobalSchedulerMu.RLock()
	getSchedulerFn := getGlobalSchedulerFunc
	getGlobalSchedulerMu.RUnlock()
	if getSchedulerFn != nil {
		if getScheduler := getSchedulerFn(); getScheduler != nil {
			// Type assert to *Scheduler (we know the type from scheduler package)
			// Using any to avoid circular dependency
			type SchedulerInterface interface {
				TriggerJobByLifecycle(context.Context, string, string, string, map[string]any) error
			}
			if sched, ok := getScheduler.(SchedulerInterface); ok && sched != nil {
				// Trigger lifecycle jobs asynchronously (best effort)
				lifecycleBud := goroutinelabels.DefaultBudget()
				lifecycleBuilder := goroutinelabels.NewGoroutine(ConstStreamLifecycleJobTrigger, fmt.Sprintf(ConstStreamTriggeringLifecycleJobForStrStrToStr, kind, fromState, toState))
				if lifecycleBud != nil {
					lifecycleBuilder = lifecycleBuilder.WithBudget(lifecycleBud)
				}
				lifecycleBuilder.StartSimple(func() {
					var _err_83138460 = sched.TriggerJobByLifecycle(ctx, kind, fromState, toState, objectData)
					if //nolint:errcheck // Lifecycle trigger errors are non-critical
					_err_83138460 != nil {
						logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83138460).Log()
					}
				})
			}
		}
	}

	return nil
}

// getGlobalSchedulerFunc returns a function to get the global scheduler
// This avoids direct import of scheduler package to prevent circular dependencies
// The function is set by the scheduler package when it initializes
var (
	getGlobalSchedulerFunc func() any
	getGlobalSchedulerMu   sync.RWMutex
)

// SetGlobalSchedulerGetter sets the function to get the global scheduler
// This should be called by the scheduler package during initialization
func SetGlobalSchedulerGetter(getter func() any) {
	getGlobalSchedulerMu.Lock()
	defer getGlobalSchedulerMu.Unlock()
	getGlobalSchedulerFunc = getter
}

// executeCacheOperation executes cache operations based on context
// This is called by storage operations to perform cache updates/invalidations
// The storage layer can update the cache context with actual file paths and IDs before execution
func executeCacheOperation(ctx context.Context, filePath string) error {
	return executeCacheOperationWithID(ctx, filePath, "")
}

// executeCacheOperationWithID executes cache operations with optional ID update
// This allows the storage layer to update the cache context with the actual ID if it was auto-generated
func executeCacheOperationWithID(ctx context.Context, filePath, id string) error {
	if cacheOperationHandler == nil {
		// No cache handler registered - this is fine, cache operations are optional
		return nil
	}

	cacheCtx := pkgctx.GetCacheContext(ctx)
	if cacheCtx == nil {
		// No cache operation requested in context
		return nil
	}

	// Update ID if provided and not already set (for auto-generated IDs)
	if id != emptyValue && cacheCtx.NewID == emptyValue {
		cacheCtx.NewID = id
	}

	// Update file path if provided and not already set
	if filePath != emptyValue && cacheCtx.FilePath == emptyValue {
		cacheCtx.FilePath = filePath
	}

	return cacheOperationHandler(cacheCtx)
}

// InvalidateCachesForKind triggers the invalidation shockwave for a specific kind.
// It clears both the global ListCache and the in-memory CAS index cache.
func (f *FileObjectStorage) InvalidateCachesForKind(kind string) {
	InvalidateListCacheForKind(kind)
	f.InvalidateCASCacheForKind(kind)
}

// FileObjectStorageOptions configures optional behavior for file-based storage (e.g. test isolation).
type FileObjectStorageOptions struct {
	// SkipGlobalWiring when true prevents wiring to global audit buffer and IO queue manager.
	// Used by test storage so tests do not share or overwrite global state.
	SkipGlobalWiring bool
	// InitContext bounds storage init (e.g. WAL replay). When set, HashRegistry saves during init
	// use SaveWithContext(InitContext) so the daemon does not hang indefinitely. Cleared after init.
	InitContext context.Context
}

// FileObjectStorage implements ObjectStorageProvider for file-based storage
type FileObjectStorage struct {
	projectRoot            string
	processDir             string
	validator              validation.Validator
	idValidator            *validation.IDValidator
	specLoader             *objects.SpecLoader
	lifecycleLoader        *objects.LifecycleLoader
	bucketingConfig        *BucketingConfigRegistry       // Legacy bucketing config
	bucketStrategyRegistry *DefaultBucketStrategyRegistry // New bucketing strategy system (lazy-loaded)
	bucketStrategyOnce     sync.Once                      // Ensures single initialization (replaces mutex)

	// Content-addressable storage instances (lazy-loaded per kind)
	// Uses ResourceCache abstraction for thread-safe caching
	casCache *ResourceCache[*ContentAddressableStorage]

	// Hash registries per (kind, dir) to avoid spawning a save worker per create/update/delete/move.
	// Reusing one registry per (kind, dir) caps goroutine growth (startSaveWorker) under load.
	hashRegistryCache *ResourceCache[*HashRegistry]

	// Directory I/O caches (reduce repeated ReadDir during List/getObjectFilePath)
	// Process-lifetime cache; directory structure can still change within a run (e.g. first bucket created),
	// so cache entries are keyed by kindDir and validated by dir mtime.
	bucketedCache   sync.Map // kindDir string -> bucketedCacheEntry (usesBucketedStorage result + mtime)
	bucketListCache sync.Map // kindDir string -> []string (subdir names for bucketed lookup)

	// WaitGroup lifecycle management
	wgManager *WaitGroupManager

	// skipHashRegistryShutdownRegistration when true prevents hash registries from being
	// registered with the global HashRegistryManager. Used by test storage so one test's
	// scheduler stop (which triggers global shutdown) does not cancel another test's registries.
	skipHashRegistryShutdownRegistration bool

	// unregisteredHashRegistries holds hash registries created when skipHashRegistryShutdownRegistration
	// is true, so Shutdown() can signal and drain them (graceful shutdown without abandoning work).
	unregisteredHashRegistries   []*HashRegistry
	unregisteredHashRegistriesMu sync.Mutex

	// skipGlobalWiring when true skips wiring to global audit buffer and IO queue manager (test isolation).
	skipGlobalWiring bool

	// orphanCleanupQueue when non-nil is used instead of the global CAS orphan cleanup queue (test isolation).
	orphanCleanupQueue *CASOrphanCleanupQueue

	// Write-behind (Option B): WAL + buffer + background worker for fast create/delete.
	writeBuf          *ObjectWriteBuffer
	wal               *ObjectWAL
	writeBehindWorker *ObjectWriteBehindWorker
	walMu             sync.Mutex // protects wal for append and compaction (close/compact/reopen)

	// initCtx set during NewFileObjectStorage when InitContext is provided; used by saveHashRegistry
	// so WAL replay respects the timeout. Cleared after worker Start() returns.
	initCtx   context.Context
	initCtxMu sync.Mutex

	// replayPhase true while write-behind worker is applying WAL replay. When true, saveHashRegistry
	// uses SaveAsync() so replay stays fast and does not block on HashRegistry I/O.
	replayPhase atomic.Bool

	// writeBehindApplyActive is true while applyCreate/Update/DeleteFromBuffer runs on the write-behind worker.
	// During this window, synchronous Create(audit_event) from finalize would enqueue more WAL work while
	// the worker is still inside apply — deadlock. Audit helpers consult this and defer to pending + flush.

	// streamLocations: for stream-backed high-volume objects, id -> "segmentPath::offset" so GetFilePathForObject can resolve without CAS.
	streamLocations sync.Map

	// traitNormOnce / traitNormRegistry: lazy trait registry for ITEM-210 redundant top-level traits strip before YAML persist.
	traitNormOnce     sync.Once
	traitNormRegistry *objects.TraitRegistry
}

// NewFileObjectStorage creates a new file-based object storage.
// If projectRoot is empty, it will attempt to find the project root.
// opts is optional (pass nil or omit); when opts != nil && opts.SkipGlobalWiring is true,
// global audit buffer and IO queue manager are not wired (test isolation).
// NOTE: Explicit projectRoot parameter takes precedence over environment variables
// to allow clean context switching between projects without shell-specific env vars.
func NewFileObjectStorage(projectRoot string, opts ...*FileObjectStorageOptions) (*FileObjectStorage, error) {
	var opt *FileObjectStorageOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	// Priority: explicit parameter > environment variable > auto-discovery
	// This allows clean context switching without shell-specific environment variables
	explicitProjectRoot := projectRoot != emptyValue // capture before resolve so we know caller intent
	if projectRoot == emptyValue {
		// Same precedence as CLI (ZQK_PROJECT_ROOT, ZQK_TEST_ROOT, then CWD discovery).
		// Use internal/cli/context here, not internal/cli (import cycle: cli pulls in storage).
		projectRoot = cliContext.ResolveProjectRoot(".")
	}
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf(ConstStreamCouldNotFindProjectRoot)
	}

	// Determine process directory.
	// When caller passed explicit projectRoot (e.g. tests pass testRoot), always use
	// projectRoot-based path so ZQK_TEST_DATA_DIR from other parallel tests never wins.
	// When using ZQK_TEST_DATA_DIR, require it to be under projectRoot so parallel tests
	// cannot pollute our processDir (e.g. other test's path containing /001/).
	var processDir string
	testDataDir := os.Getenv(zqkenv.TestDataDir())
	when.When(func() bool {
		return !explicitProjectRoot && testDataDir != emptyValue && projectRoot == os.Getenv(zqkenv.TestRoot())
	}).Then(func() {
		absDataDir, err := filepath.Abs(testDataDir)
		when.When(func() bool {
			if err != nil {
				return false
			}
			rel, relErr := filepath.Rel(projectRoot, absDataDir)
			return relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
		}).Then(func() {
			processDir = absDataDir
		}).OrElse(func() {
			processDir = paths.ResolvePathFromCacheOrConstant(projectRoot, "process", paths.ProcessDir)
		}).Run()
	}).OrElse(func() {
		processDir = paths.ResolvePathFromCacheOrConstant(projectRoot, "process", paths.ProcessDir)
	}).Run()

	if _, err := os.Stat(processDir); err != nil {
		return nil, errfmt.Newf(ConstStreamProcessDirectoryNotFound).Wrap(err)
	}

	// Stream-backed kinds resolve segment dirs via paths.ResolvePathStrict (prefix:streams/<kind>).
	// Without the per-project alias cache, AppendToStream fails with ErrPathAliasNotInCache and
	// write-behind apply never advances (drain timeouts). Same as scheduler pre-warm / path-cache check.
	if err := EnsurePathAliasCacheReady(projectRoot); err != nil {
		return nil, errfmt.Newf(ConstStreamEnsurePathAliasCache).Wrap(err)
	}

	// Initialize loaders with instance-specific directories
	// Each storage instance uses its own loaders pointing to its project's directories
	// This allows clean context switching between projects without global state conflicts
	// Schema plane (object specs + lifecycles) lives under ProcessInternalDir.
	// Instance plane (backlog, etc.) stays under ProcessDir — do not assume
	// processDir/_internal after open-core split (architecture/_internal vs process/).
	specsDir := filepath.Join(projectRoot, paths.ProcessInternalDir, "object_specs")
	specLoader := objects.NewSpecLoader(specsDir)

	// Note: We create instance-specific loaders rather than using global ones
	// to ensure each storage instance uses the correct project's specs/lifecycles.
	// Global loader is still available for other use cases, but storage uses instance-specific.

	lifecyclesDir := filepath.Join(projectRoot, paths.ProcessInternalDir, "lifecycles")
	lifecycleLoader := objects.NewLifecycleLoader(lifecyclesDir)

	// Set builder registry on spec loader to enable version-aware loading
	// This allows the validator to load specs by version when objects have schema_version
	builderRegistry := builders.GetGlobalRegistry()
	adapter := builders.NewSpecLoaderAdapter(builderRegistry)
	specLoader.SetBuilderRegistry(adapter)

	// Load only base specs (EnsureReady). Do NOT pre-warm every kind: profile showed
	// LoadSpecWithInheritance + loadSpecWithInheritanceRecursive at ~35% of CRUD CPU; doing
	// ReadDir + LoadSpecWithInheritance for every spec on every NewFileObjectStorage (each CLI
	// run is a new process) made init slow. Specs are loaded on first use and cached (use cache).
	// OBJECT_OPERATIONS_PERFORMANCE.md: avoid spec load in hot path; target millisecond-level.
	if err := specLoader.EnsureReady(context.Background()); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Debug(LogEventStorageObjectFileSpecEnsureReadyWarnDebug).WithError(err).Log()
	}

	// Create validator with loaders
	// Use instance-specific loaders to ensure correct project context
	validator := validation.NewGoValidatorWithLoaders(specLoader, lifecycleLoader)

	// Initialize ID validator with instance-specific specs directory
	// This ensures ID validation uses the correct project's specs
	idValidator := validation.NewIDValidator(specsDir)

	// Initialize bucketing configuration registry (legacy)
	bucketingConfig := NewBucketingConfigRegistry(projectRoot)
	if err := bucketingConfig.Load(); err != nil {
		// Log warning but continue - will use defaults
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageBucketingLoadLegacyConfigFailedWarn).WithError(err).Log()
	}

	// Initialize default bucket strategy registry (new system using bucketing_strategy objects)
	// NOTE: This is lazy-loaded to avoid circular dependencies during initialization
	// The registry will be created on first use via getBucketStrategyRegistry()
	bucketStrategyRegistry := (*DefaultBucketStrategyRegistry)(nil)

	f := &FileObjectStorage{
		projectRoot:            projectRoot,
		processDir:             processDir,
		validator:              validator,
		idValidator:            idValidator,
		specLoader:             specLoader,
		lifecycleLoader:        lifecycleLoader,
		bucketingConfig:        bucketingConfig,
		bucketStrategyRegistry: bucketStrategyRegistry,
		wgManager:              NewWaitGroupManager(),
		casCache:               &ResourceCache[*ContentAddressableStorage]{},
		hashRegistryCache:      &ResourceCache[*HashRegistry]{},
	}

	// Optionally skip wiring to globals (test isolation)
	if opt != nil && opt.SkipGlobalWiring {
		f.skipGlobalWiring = true
		return f, nil
	}

	// Set fileStorage on the global audit event buffer for CAS routing
	// NOTE: This is file-backend specific - global singletons are only used for file backend
	// CRITICAL: Only set if buffer doesn't already have a fileStorage with a different project root
	// This prevents overwriting when multiple storage instances exist (e.g., main project + scenario)
	buffer := GetGlobalAuditEventBuffer()
	var existingFileStorage *FileObjectStorage
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	err := concurrency.RunInRLockWithLogger(&buffer.mu, locknames.LockNameAuditBufferGetFilestorage, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		existingFileStorage = buffer.fileStorage
		return nil
	})
	if err != nil {
		StorageLog(logger).Warn(LogEventStorageObjectFileAuditBufferReadFailedWarn).WithError(err).Log()
	}
	when.When(func() bool { return existingFileStorage == nil }).Then(func() {
		buffer.SetFileStorage(f)
		buffer.SetProjectRoot(projectRoot)
	}).OrElseWhen(func() bool {
		return existingFileStorage != nil && existingFileStorage.GetProjectRoot() == projectRoot
	}).Then(func() {
		buffer.SetFileStorage(f)
		buffer.SetProjectRoot(projectRoot)
	}).OrElse(func() {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		existingRoot := "unknown"
		if existingFileStorage != nil {
			existingRoot = existingFileStorage.GetProjectRoot()
		}
		msg := ConstStreamSkippingSetfilestorageOnGlobalAuditBuffer
		if IsTestOrTempProjectRoot(projectRoot) {
			StorageLog(logger).Debug(msg).
				String(ConstStreamExistingProjectRoot, existingRoot).
				String(ConstStreamNewProjectRoot, projectRoot).
				Log()
		} else {
			StorageLog(logger).Warn(msg).
				String(ConstStreamExistingProjectRoot, existingRoot).
				String(ConstStreamNewProjectRoot, projectRoot).
				Log()
		}
	}).Run()

	// Initialize I/O queue manager with this storage instance
	// NOTE: This is file-backend specific - IO queue is only used for file backend
	// This enables queued I/O operations when feature flag is enabled
	// CRITICAL: Only set if queue manager doesn't already have storage with a different project root
	// Note: IOQueueManager is a global singleton that may be initialized before command context exists
	// Use system context for package initialization
	ioQueueManager := GetGlobalIOQueueManager(pkgctx.NewSystemContext())
	// Lock-free reads using atomic.Value (replaces mutex)
	existingQueueProjectRoot := ioQueueManager.GetProjectRoot()
	var existingStorage ObjectStorageProvider
	if storage := ioQueueManager.GetStorage(); storage != nil {
		if storageProvider, ok := storage.(ObjectStorageProvider); ok {
			existingStorage = storageProvider
		}
	}
	when.When(func() bool { return existingStorage == nil || existingQueueProjectRoot == projectRoot }).Then(func() {
		ioQueueManager.SetStorage(f)
		ioQueueManager.SetProjectRoot(projectRoot)
	}).OrElse(func() {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		msg := "Skipping SetStorage/SetProjectRoot on global IO queue manager - different project root already set"
		if IsTestOrTempProjectRoot(projectRoot) {
			StorageLog(logger).Debug(msg).
				String(ConstStreamExistingProjectRoot, existingQueueProjectRoot).
				String(ConstStreamNewProjectRoot, projectRoot).
				Log()
		} else {
			StorageLog(logger).Warn(msg).
				String(ConstStreamExistingProjectRoot, existingQueueProjectRoot).
				String(ConstStreamNewProjectRoot, projectRoot).
				Log()
		}
	}).Run()

	// Initialize async validation strategies for high-volume CAS kinds (audit_event, mcp_session, doc_entry).
	// This starts background file scanners that maintain a cache of existing files, reducing
	// os.Stat calls during index persistence. Skipped in test mode to avoid global state.
	// See ITEM-EXAMPLE for design details.
	if !f.skipGlobalWiring {
		InitializeAsyncValidationStrategies(projectRoot)
	}

	// Write-behind (Option B): WAL + buffer + worker for sub-100ms create/delete.
	if projectRoot != emptyValue && !f.skipGlobalWiring {
		wal, err := NewObjectWAL(projectRoot)
		if err != nil {
			StorageLog(logger).Warn(LogEventStorageObjectFileWALInitFailedSyncFallbackWarn).WithError(err).Log()
		} else {
			f.wal = wal
			f.writeBuf = NewObjectWriteBuffer()
			f.writeBehindWorker = NewObjectWriteBehindWorker(f.writeBuf, wal, projectRoot, f)
			initCtx := context.Background()
			if opt != nil && opt.InitContext != nil {
				initCtx = opt.InitContext
			}
			f.initCtxMu.Lock()
			f.initCtx = initCtx
			f.initCtxMu.Unlock()
			defer func() {
				f.initCtxMu.Lock()
				f.initCtx = nil
				f.initCtxMu.Unlock()
			}()
			if startErr := f.writeBehindWorker.Start(initCtx); startErr != nil {
				return nil, startErr
			}
			if !f.skipGlobalWiring {
				GetGlobalShutdownCoordinator().RegisterQueue(f.writeBehindWorker)
			}
		}
	}

	return f, nil
}

// GetFileObjectStorageForTest creates file-based storage for tests. Hash registries are not
// registered with the global shutdown coordinator, and global audit buffer/IO queue are not
// wired, so tests do not share or overwrite global state.
func NewFileObjectStorageForTest(projectRoot string) (*FileObjectStorage, error) {
	f, err := NewFileObjectStorage(projectRoot, &FileObjectStorageOptions{SkipGlobalWiring: true})
	if err != nil {
		return nil, err
	}
	f.skipHashRegistryShutdownRegistration = true
	return f, nil
}

// SetOrphanCleanupQueue sets the CAS orphan cleanup queue to use instead of the global one (test isolation).
// Must be called before any CAS operations that enqueue orphan cleanups. Queue's SetProjectRoot/SetStorage
// should be set by the caller for this storage's project root.
func (f *FileObjectStorage) SetOrphanCleanupQueue(q *CASOrphanCleanupQueue) {
	f.orphanCleanupQueue = q
}

// GetOrphanCleanupQueue returns the injected orphan cleanup queue if set (for tests that need the queue).
func (f *FileObjectStorage) GetOrphanCleanupQueue() *CASOrphanCleanupQueue {
	return f.orphanCleanupQueue
}

// GetTestCleanup returns a cleanup function that runs [RunProjectTestTeardown] with [TempProjectTeardown]
// defaults (CAS/WAL drain, strip, project-root scrub). Strip/scrub are skipped automatically when the
// storage project root looks like a Git worktree ([IsProbableGitWorktreeRoot]) so a real checkout cannot be wiped.
// Idempotent via sync.Once so duplicate defer/t.Cleanup paths are safe. Returns nil when receiver is nil.
// Orphan cleanup queue shutdown is included when [FileObjectStorage.SetOrphanCleanupQueue] was used (TestingFactory).
func (f *FileObjectStorage) GetTestCleanup() func() {
	if f == nil {
		return nil
	}
	projectRoot := f.projectRoot
	fs := f
	var once sync.Once
	orphan := f.orphanCleanupQueue
	return func() {
		once.Do(func() {
			opts := TempProjectTeardown(projectRoot, fs)
			if orphan != nil {
				opts.OrphanCleanupQueue = orphan
			}
			var _err_83158095 = RunProjectTestTeardown(opts)
			if //nolint:errcheck // best-effort test cleanup
			_err_83158095 != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.

					// GetProjectRoot returns the project root path
					// This is exported so other packages (like scheduler) can access it for audit event creation
					ProfileSystem))).Error(ErrMsgSwallowedError,

					_err_83158095).Log()
			}
		})
	}
}

func (f *FileObjectStorage) GetProjectRoot() string {
	return f.projectRoot
}

// SetReplayPhase sets whether the write-behind worker is currently applying WAL replay.
// When true, saveHashRegistry uses SaveAsync() so replay does not block on HashRegistry I/O.
// Called by ObjectWriteBehindWorker at replay start and when replay completes.
func (f *FileObjectStorage) SetReplayPhase(inReplay bool) {
	f.replayPhase.Store(inReplay)
}

// augmentCtxForAuditDuringWriteBehindApply marks ctx so audit events are queued (WithDeferAuditEvents)
// when this storage is executing a write-behind worker apply. Prevents nested synchronous
// Create(audit_event) from deadlocking the single worker goroutine.
func (f *FileObjectStorage) augmentCtxForAuditDuringWriteBehindApply(ctx context.Context) context.Context {
	if f == nil || !isSkipWriteBehind(ctx) {
		return ctx
	}
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}
	return WithDeferAuditEvents(ctx)
}

// saveHashRegistry saves the hash registry; during storage init uses SaveWithContext(initCtx)
// so init is bounded and the daemon does not hang. During WAL replay uses SaveAsync() so the
// apply path stays fast. After init and replay, uses Save().
func (f *FileObjectStorage) saveHashRegistry(hr *HashRegistry) error {
	if f.replayPhase.Load() {
		hr.SaveAsync()
		return nil
	}
	f.initCtxMu.Lock()
	ctx := f.initCtx
	f.initCtxMu.Unlock()
	if ctx != nil && ctx.Err() == nil {
		return hr.SaveWithContext(ctx)
	}
	return hr.Save()
}

// GetLifecycleLoader returns the lifecycle loader instance
// This allows components to use the storage instance's lifecycle loader
// which points to the correct project's lifecycle directory
func (f *FileObjectStorage) GetLifecycleLoader() *objects.LifecycleLoader {
	return f.lifecycleLoader
}

// Shutdown signals and drains all unregistered hash registries (those created with
// skipHashRegistryShutdownRegistration). Call this from test cleanup so background
// save workers finish transactional work and exit, avoiding goroutine leaks and timeouts.
// No-op if no unregistered registries exist. Also drains the write-behind worker and closes the WAL.
func (f *FileObjectStorage) Shutdown(ctx context.Context) error {
	if f.writeBehindWorker != nil {
		f.writeBehindWorker.Stop()
		f.writeBehindWorker = nil
	}
	if f.wal != nil {
		var _err_83160911 = f.wal.Close()
		if _err_83160911 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83160911).Log()
		}
		f.wal = nil
	}
	f.writeBuf = nil

	// Drain test CAS queue
	var shutdownErr error
	if f.projectRoot != emptyValue {
		q := GetListingIndexWriteQueueForProjectRoot(f.projectRoot)
		if q != nil && q.skipShutdownCoordinatorCheck.Load() {
			if err := q.InitiateShutdown(); err != nil && shutdownErr == nil {
				shutdownErr = err
			}
			if err := q.Drain(ctx); err != nil && shutdownErr == nil {
				shutdownErr = err
			}
			// Remove from registry so subsequent test instances for the same root get a fresh queue
			RemoveListingIndexWriteQueueForProjectRoot(f.projectRoot)
		}
	}

	var regs []*HashRegistry
	var _err_83160799 = concurrency.RunInLock(&f.unregisteredHashRegistriesMu, func() error {
		regs = f.unregisteredHashRegistries
		f.unregisteredHashRegistries = nil
		return nil
	})
	if _err_83160799 != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83160799).Log()
	}
	for _, hr := range regs {
		if err := hr.InitiateShutdown(); err != nil && shutdownErr == nil {
			shutdownErr = err
		}
		if err := hr.Drain(ctx); err != nil && shutdownErr == nil {
			shutdownErr = err
		}
	}
	return shutdownErr
}

// TryCompactWAL closes the WAL, runs CompactWAL (keeps only unapplied entries), and reopens the WAL.
// Call only when the write-behind buffer is empty to avoid losing in-memory ops. Holding walMu blocks appends briefly.
func (f *FileObjectStorage) TryCompactWAL() error {
	return concurrency.RunInLockWithLogger(&f.walMu, locknames.LockNameWalCompact, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if f.wal == nil {
			return nil
		}
		if f.writeBuf != nil && f.writeBuf.Len() > 0 {
			return nil // Caller should only invoke when buffer is empty
		}
		var _err_83161762 = f.wal.Close()
		if _err_83161762 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Reopen WAL on compaction failure so we don't leave storage without WAL
				Error(ErrMsgSwallowedError, _err_83161762).Log()

		}
		f.wal = nil
		if err := CompactWAL(f.projectRoot); err != nil {

			if wal, reopenErr := NewObjectWAL(f.projectRoot); reopenErr == nil {
				f.wal = wal
			}
			return err
		}
		wal, err := NewObjectWAL(f.projectRoot)
		if err != nil {
			return err
		}
		f.wal = wal
		return nil
	})
}

// GetProcessDir returns the process directory (docs/process) used by this storage.
// Used by tests to resolve kind directories when CAS index may not be flushed yet.
func (f *FileObjectStorage) GetProcessDir() string {
	return f.processDir
}

// GetKindDir returns the absolute directory path for a given kind, resolving via the path cache if possible.
func (f *FileObjectStorage) GetKindDir(kind string) string {
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == emptyValue {
		return ""
	}
	if rel := paths.GetPathAlias(f.projectRoot, dirName); rel != emptyValue {
		return filepath.Join(f.projectRoot, rel)
	}
	return filepath.Join(f.processDir, dirName)
}

// streamLocationKey returns the sync.Map key for stream-backed object location (kind\x00id).
func streamLocationKey(id, kind string) string {
	return kind + "\x00" + id
}

// setStreamLocation registers the segment path and offset for a stream-backed object (used by writeObjectToStream).
func (f *FileObjectStorage) setStreamLocation(id, kind, location string) {
	f.streamLocations.Store(streamLocationKey(id, kind), location)
}

// getStreamLocation returns "segmentPath::offset" if this object was written via stream; otherwise "".
// Checks in-memory first, then persistent registry (so retention/List work across processes).
func (f *FileObjectStorage) getStreamLocation(id, kind string) string {
	if v, ok := f.streamLocations.Load(streamLocationKey(id, kind)); ok {
		if s, _ := v.(string); s != emptyValue {
			return s
		}
	}
	if f.projectRoot != emptyValue {
		if loc := getStreamLocationFromPersistentRegistry(f.projectRoot, kind, id); loc != emptyValue {
			return loc
		}
	}
	return ""
}

// removeStreamLocation removes the stream location for an object (on delete).
func (f *FileObjectStorage) removeStreamLocation(id, kind string) {
	f.streamLocations.Delete(streamLocationKey(id, kind))
}

// listStreamIDsForKind returns all object IDs registered for the given kind (in-memory + persistent registry).
// Used by List to merge stream-backed IDs with CAS IDs when stream storage is enabled for the kind.
func (f *FileObjectStorage) listStreamIDsForKind(kind string) []string {
	seen := make(map[string]bool)
	prefix := kind + "\x00"

	// Load deleted set to exclude soft-deleted objects
	var deletedMap map[string]bool
	if f.projectRoot != emptyValue {
		_, deletedMap = readStreamRegistryJSONLIntoMaps(f.projectRoot, kind)
	}

	f.streamLocations.Range(func(key, value interface{}) bool {
		if k, ok := key.(string); ok && len(k) > len(prefix) && k[:len(prefix)] == prefix {
			id := k[len(prefix):]
			if deletedMap == nil || !deletedMap[id] {
				seen[id] = true
			}
		}
		return true
	})
	if f.projectRoot != emptyValue {
		for _, id := range ListStreamIDsFromPersistentRegistry(f.projectRoot, kind) {
			if deletedMap == nil || !deletedMap[id] {
				seen[id] = true
			}
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	return ids
}

const maxStreamEntriesForCacheBuild = 100000

// BuildHighVolumeCacheEntriesFromStream returns high-volume cache entries for the given kind from the stream location registry.
// Used when building the high-volume event cache so Count/OldestIDs include stream-backed objects after rebuild or restart.
// Respects ctx deadline; caps entries at maxStreamEntriesForCacheBuild to avoid long builds.
func (f *FileObjectStorage) BuildHighVolumeCacheEntriesFromStream(ctx context.Context, kind string, logger logging.Logger) ([]*HighVolumeEventCacheEntry, error) {
	if !StreamStorageEnabledForKind(kind) {
		return nil, nil
	}
	ids := f.listStreamIDsForKind(kind)
	if len(ids) == 0 {
		return nil, nil
	}
	if ctx != nil {
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < 2*time.Minute && len(ids) > maxStreamEntriesForCacheBuild {
			ids = ids[:maxStreamEntriesForCacheBuild]
			if logger != nil {
				StorageLog(logger).Info(LogEventStorageHighVolumeCacheBuildingFromStreamCappedInfo).
					Kind(kind).
					Int("capped", maxStreamEntriesForCacheBuild).
					Log()
			}
		}
	}
	var entries []*HighVolumeEventCacheEntry
	for _, id := range ids {
		if ctx != nil && ctx.Err() != nil {
			return entries, ctx.Err()
		}
		loc := f.getStreamLocation(id, kind)
		if loc == emptyValue {
			continue
		}
		segmentPath, offset, ok := StreamPathAndOffset(loc)
		if !ok || segmentPath == emptyValue {
			continue
		}
		obj, err := ReadRecordAt(segmentPath, offset)
		if err != nil {
			if logger != nil {
				StorageLog(logger).Debug(LogEventStorageHighVolumeCacheSkipStreamRecordDebug).
					ObjectID(id).
					Kind(kind).
					WithError(err).
					Log()
			}
			continue
		}
		// Do not call AppendStreamLocationToRegistry here. New stream objects are already registered
		// on write (object_storage_file_create). Re-appending the same id on every cache build was
		// duplicative; each append also invalidates the in-process stream registry snapshot, so the
		// next getStreamLocation re-read the full registry file from disk — O(n²) I/O for n IDs.
		var createdAt time.Time
		if s := objects.GetString(obj, objects.FieldKeyCreatedAt); s != emptyValue {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				createdAt = t
			}
		}
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}
		eventType, _ := obj[objects.FieldKeyEventType].(string)
		status, _ := obj[objects.FieldKeyStatus].(string)
		mtime := time.Now().UTC()
		if info, err := os.Stat(segmentPath); err == nil {
			mtime = info.ModTime()
		}
		entries = append(entries, &HighVolumeEventCacheEntry{
			ID: id, Kind: kind, CreatedAt: createdAt, EventType: eventType, Status: status,
			FilePath: loc, MTime: mtime, Exists: true,
		})
	}
	return entries, nil
}

// getObjectFilePath returns the file path for an object given its ID and kind.
// All kinds use CAS (hash-named files); index or scan returns the path. Legacy ID-based path is fallback only.
// Uses cache-first approach: checks ObjectIDCache via GetCacheChecker before scanning.
// For stream-backed kinds when stream storage is enabled, returns "segmentPath::offset" from stream registry.
func (f *FileObjectStorage) getObjectFilePath(id, kind string) (string, error) {
	// Stream-backed: prefer stream_current overlay (runtime delta), else stream registry
	if StreamStorageEnabledForKind(kind) {
		if overlay := GetStreamBackedObjectFilePath(f.projectRoot, kind, id); overlay != emptyValue {
			return overlay, nil
		}
		if loc := f.getStreamLocation(id, kind); loc != emptyValue {
			return loc, nil
		}
		return "", errfmt.Errorf(ConstStreamObjectStrKindStrNotFoundInStreamErr, id, kind, ErrObjectNotFound)
	}

	// Cache-first: check ObjectIDCache if available (lock-free read via atomic.Value)
	if checker := GetCacheChecker(); checker != nil {
		if cachedPath, exists := checker(id); exists && cachedPath != emptyValue {
			// Verify file still exists at cached path (cache may be stale)
			if _, err := os.Stat(cachedPath); err == nil {
				return cachedPath, nil
			}
			// Cache entry exists but file doesn't - cache is stale, continue to normal resolution
		}
	}

	kindDir := f.GetKindDir(kind)
	if kindDir == "" {
		return "", errfmt.Errorf(ConstStreamUnknownObjectKindStr, kind)
	}

	// Check if this kind uses content-addressable storage
	// For CAS objects, resolve via index first, then discover by scanning if index has no entry (CAS-based resources must resolve for reference validation)
	if f.usesContentAddressableStorage(kind) {
		var cas *ContentAddressableStorage
		if c, err := f.getContentAddressableStorage(kind); err == nil {
			cas = c
			if hashPath, err := cas.GetFilePathForID(id); err == nil {
				return hashPath, nil
			}
		}
		// Index miss or no CAS instance: discover by scanning for hash-named files whose "id" field matches
		discoveredPath, discoveredHash, scanErr := f.findCASFilePathByScanning(id, kindDir)
		if scanErr == nil {
			// Automatically update cache and index files so future lookups use the index
			if cas != nil && cas.index != nil {
				var _err_83169646 = cas.index.SetMapping(id, discoveredHash)
				if _err_83169646 != nil {
					logging.Fluent(logging.GetLoggerFromProfile(

						// CAS resolution failed. Use legacy path only when that file actually exists (migration: ID-based file on disk).
						// Otherwise return error so reference validation does not get a wrong path (e.g. criteria/CRIT-9005.yaml when object is hash-named).
						// Create does not rely on this: prepareObjectPath builds the path directly for CAS kinds.
						string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83169646).Log()
				}
			}
			return discoveredPath, nil
		}

		config := GetStorageConfig()
		accountDir := objects.GetDirectoryFromKind(objects.KindAccount)
		var legacyFilename string
		when.When(func() bool {
			return accountDir == "accounts" && objects.GetDirectoryFromKind(kind) == accountDir && strings.HasPrefix(id, "account:")
		}).Then(func() {
			username := strings.TrimPrefix(id, "account:")
			legacyFilename = fmt.Sprintf("account-%s%s", username, config.YAMLExtension)
		}).OrElse(func() {
			legacyFilename = fmt.Sprintf("%s%s", id, config.YAMLExtension)
		}).Run()
		legacyPath := filepath.Join(kindDir, legacyFilename)
		if _, statErr := os.Stat(legacyPath); statErr == nil {
			return legacyPath, nil
		}
		// Wrap ErrObjectNotFound so callers (e.g. Read) can use errors.Is for missing objects after CAS scan miss.
		return "", errfmt.Errorf(ConstStreamObjectStrKindStrNotFoundValErr, id, kind, scanErr, ErrObjectNotFound)
	}

	// Legacy fallback: only reached when usesContentAddressableStorage(kind) is false.
	// All kinds are CAS in practice; this path exists only for migration/backward compatibility.
	// For accounts, use account-{username}.yaml; for others, use {id}.yaml.
	config := GetStorageConfig()
	// Check if this is an account kind by checking if directory is "accounts"
	accountDir := objects.GetDirectoryFromKind(objects.KindAccount)
	var filename string
	when.When(func() bool {
		return accountDir == "accounts" && objects.GetDirectoryFromKind(kind) == accountDir && strings.HasPrefix(id, "account:")
	}).Then(func() {
		username := strings.TrimPrefix(id, "account:")
		filename = fmt.Sprintf("account-%s%s", username, config.YAMLExtension)
	}).OrElse(func() {
		filename = fmt.Sprintf("%s%s", id, config.YAMLExtension)
	}).Run()

	// Check if this kind uses bucketed storage (e.g., audit_event in audit/YYYY-MM/)
	if f.usesBucketedStorage(kind, kindDir) {
		// For bucketed storage, optimize by checking most likely months first
		// Strategy: Check current month, last month, and a few recent months
		// This avoids expensive full directory scans for the common case
		now := time.Now()
		monthsToCheck := []string{
			now.Format("2006-01"),                   // Current month (most likely)
			now.AddDate(0, -1, 0).Format("2006-01"), // Last month
			now.AddDate(0, -2, 0).Format("2006-01"), // 2 months ago
		}

		// Fast path: Try likely months first (just stat, no directory listing)
		// This is O(1) per file and very fast
		for _, month := range monthsToCheck {
			dateDir := filepath.Join(kindDir, month)
			filePath := filepath.Join(dateDir, filename)
			if _, err := os.Stat(filePath); err == nil {
				return filePath, nil
			}
		}

		// Slow path: If not found in likely months, we need to search
		// Cache bucket subdir names per kindDir so multiple getObjectFilePath calls don't each ReadDir
		var bucketNames []string
		if val, ok := f.bucketListCache.Load(kindDir); ok {
			bucketNames = val.([]string)
		} else {
			entries, readErr := os.ReadDir(kindDir)
			if readErr != nil {
				if !os.IsNotExist(readErr) {
					return "", errfmt.Newf(ConstStreamFailedToReadDirectory).Wrap(readErr)
				}
				bucketNames = []string{} // kindDir missing; don't cache, fall through
			} else {
				for _, entry := range entries {
					if entry.IsDir() {
						bucketNames = append(bucketNames, entry.Name())
					}
				}
				f.bucketListCache.Store(kindDir, bucketNames)
			}
		}

		// Build set of already-checked months for efficiency
		checkedSet := make(map[string]bool)
		for _, m := range monthsToCheck {
			checkedSet[m] = true
		}

		// Search through remaining subdirectories
		// Only do this if fast path failed (should be rare)
		// Search ALL subdirectories - works with any bucket strategy (not just date-based)
		for _, bucketName := range bucketNames {
			// Skip already-checked directories (from fast path)
			if checkedSet[bucketName] {
				continue
			}
			// Check all subdirectories (not just date patterns) - supports any bucket strategy
			bucketDir := filepath.Join(kindDir, bucketName)
			filePath := filepath.Join(bucketDir, filename)
			if _, err := os.Stat(filePath); err == nil {
				return filePath, nil
			}
		}
		// If not found in any date directory, return the path without date (for backward compatibility)
		// This will cause a "not found" error, which is correct behavior
	}

	return filepath.Join(kindDir, filename), nil
}

// ============================================================================
// Extracted Operations
// ============================================================================
//
// The following operations have been extracted to separate files:
// - CRUD operations (Create, Read, Update, Delete, Move) -> object_storage_file_crud.go
// - Bulk operations (BulkCreate, BulkUpdate, BulkGet, BulkDelete) -> object_storage_file_bulk.go
// - Transaction operations (BeginTransaction, BeginEnhancedTransaction) -> object_storage_file_transaction.go
// - Graph operations (GetRelated, GetPath, GetNeighbors) -> object_storage_file_graph.go
// - List/Query operations (List, Query, Count, Exists) -> object_storage_file_list.go
// - Search operations -> object_storage_file_search.go
// - Validation operations -> object_storage_file_validation.go
// - Helper functions (file I/O, hash, metadata, permissions) -> object_storage_file_helpers.go
//
// This file now contains only core infrastructure:
// - Global variables and handlers
// - FileObjectStorage struct definition
// - NewFileObjectStorage() constructor
// - Core getters and infrastructure methods
// - CAS detection and management
// - Hash registry creation
// - File path resolution

type contextKeySkipWriteBehind struct{}

func withSkipWriteBehind(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextKeySkipWriteBehind{}, true)
}

// WithSkipWriteBehind marks the context so that storage operations skip write-behind and apply immediately.
func WithSkipWriteBehind(ctx context.Context) context.Context {
	return withSkipWriteBehind(ctx)
}

func isSkipWriteBehind(ctx context.Context) bool {
	v, _ := ctx.Value(contextKeySkipWriteBehind{}).(bool)
	return v
}
