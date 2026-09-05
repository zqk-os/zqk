package storage

import (
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"

	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/primaryorch"
	"github.com/lanceman/zqk/pkg/storage/filecas"
	"github.com/lanceman/zqk/pkg/storage/locknames"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

var (
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

	// Change notification handler (BLI-643) - called on create/update/delete so subscribers can react
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

// GetCacheOperationHandler returns the current cache operation handler
// This allows scheduler handlers to access the cache operation handler

// SetCacheChecker sets the function to check if an object exists in the cache
// This should be called by the CLI layer to enable cache-based reference validation
// The checker receives an object ID and returns the file path and whether it exists
// Uses atomic.Value for lock-free writes (writes are rare, reads are frequent)

// GetCacheChecker returns the current cache checker function
// Uses atomic.Value for lock-free reads (hot path optimization)

// cachedLivePath returns the object-id-cache path for id when that file still exists.
// The cache is the identity index: Get/Exists follow this one path (draft YAML or CAS hash).
// List-like operations must omit draft-plane paths unless a draft-plane list view is built on purpose.
// TRACK: TDE-CEF-CAS-IDENTITY-TXN-001

// SetLifecycleHookHandler sets the function to handle lifecycle transitions
// This should be called by the CLI layer to wire up scheduler job triggers
// The handler receives lifecycle transition information and triggers appropriate scheduler jobs

// SetChangeNotificationHandler sets the function to be called on object create/update/delete (BLI-643).
// The handler receives operation (OpCreate|OpUpdate|OpDelete), kind, id, and object data (nil for delete if unavailable).

func executeLifecycleHook(ctx context.Context, kind, fromState, toState string, objectData map[string]any) error {
	// Attentiveness: wake host/project primary orchestrator when a plan-scoped
	// backlog_item enters error (best-effort; never blocks the transition).
	if fromState != toState {
		if projectRoot := pkgctx.GetLifecycleProjectRoot(ctx); projectRoot != "" {
			primaryorch.MaybeWakeOnPlanBacklogError(ctx, projectRoot, kind, fromState, toState, objectData)
		}
	}

	// Single injection path (BLI-CEF-R2-ARCH-GLOBALS-DI): composition roots
	// (cmd/zqk/app OnStorageCreated) install SetLifecycleHookHandler. No scheduler-getter fallback.
	if lifecycleHookHandler == nil {
		return nil
	}
	return lifecycleHookHandler(ctx, kind, fromState, toState, objectData)
}

// executeCacheOperationWithID executes cache operations with optional ID update
// This allows the storage layer to update the cache context with the actual ID if it was auto-generated.
// Always notes object-id-cache pending for CAS paths (never draft-plane), even when handler is nil.
// TRACK: REDACTED

// coupleObjectIDCacheLivePath inserts or swaps object-id-cache for the live file path.
// Draft-plane create has no CAS blob; the id still belongs in the cache (path is the
// draft YAML). CAS Create/Update couple via invokeCASPostSync instead.
// TRACK: TDE-CEF-CAS-IDENTITY-TXN-001

// unwrapFileStorageFromContext is a soft hint; most callers rely on path-derived project root.
func unwrapFileStorageFromContext(ctx context.Context) *FileObjectStorage {
	_ = ctx
	return nil
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
	casCache *ResourceCache[*filecas.ContentAddressableStorage]

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
	orphanCleanupQueue *caspkg.CASOrphanCleanupQueue

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

	// traitNormOnce / traitNormRegistry: lazy trait registry for BLI-210 redundant top-level traits strip before YAML persist.
	traitNormOnce     sync.Once
	traitNormRegistry *objects.TraitRegistry
}

var fileObjectStorageForTestOnce sync.Once

// SetOrphanCleanupQueue sets the CAS orphan cleanup queue to use instead of the global one (test isolation).
// Must be called before any CAS operations that enqueue orphan cleanups. Queue's SetProjectRoot/SetStorage
// should be set by the caller for this storage's project root.
func (f *FileObjectStorage) SetOrphanCleanupQueue(q *caspkg.CASOrphanCleanupQueue) {
	f.orphanCleanupQueue = q
}

// GetOrphanCleanupQueue returns the injected orphan cleanup queue if set (for tests that need the queue).
func (f *FileObjectStorage) GetOrphanCleanupQueue() *caspkg.CASOrphanCleanupQueue {
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
	if f.replayPhase.Load() || zqkenv.PrivilegedWriterDaemonRole() {
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
//
// Ordered CAS pending visibility (CRIT-CAS-PENDING-003 / ADR-CAS-PENDING-VISIBILITY-LAYER):
// reject new pending publishes → dump pending id→hash into durable indexes → then drain queues/WAL.
func (f *FileObjectStorage) Shutdown(ctx context.Context) error {
	if f.projectRoot != emptyValue {
		pending := caspkg.GetCASPendingVisibilityCache(f.projectRoot)
		pending.BeginOrderedShutdown()
		if err := pending.DumpPendingToDurableIndexes(func(kind string) (*filecas.ContentAddressableStorage, error) {
			return f.GetContentAddressableStorage(kind)
		}); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
				Error(ErrMsgSwallowedError, err).Log()
		}
		// Persist any index updates still sitting in the listing-index write queue.
		if q := caspkg.GetListingIndexWriteQueueForProjectRoot(f.projectRoot); q != nil {
			_ = q.FlushAll(15 * time.Second) //nolint:errcheck // best-effort before drain
		}
	}

	if f.writeBehindWorker != nil {
		f.writeBehindWorker.Stop()
		f.writeBehindWorker = nil
		releaseWriteBehindOwner(f.projectRoot, f)
	}
	if f.wal != nil {
		if err := ReleaseObjectWAL(f.projectRoot, f.wal); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		f.wal = nil
	}
	f.writeBuf = nil

	// Drain test CAS queue
	var shutdownErr error
	if f.projectRoot != emptyValue {
		q := caspkg.GetListingIndexWriteQueueForProjectRoot(f.projectRoot)
		if q != nil && q.SkipShutdownCoordinatorCheck.Load() {
			if err := q.InitiateShutdown(); err != nil {
				shutdownErr = err
			}
			if err := q.Drain(ctx); err != nil && shutdownErr == nil {
				shutdownErr = err
			}
			// Remove from registry so subsequent test instances for the same root get a fresh queue
			caspkg.RemoveListingIndexWriteQueueForProjectRoot(f.projectRoot)
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

// TryCompactWAL runs CompactInPlace on the shared ObjectWAL (close → CompactWAL rename → reopen
// under ObjectWAL.mu). Do not replace the *ObjectWAL pointer — holders must not retain a
// deleted-inode FD. Call only when the write-behind buffer is empty. Holding walMu blocks appends
// on this storage; ObjectWAL.mu serializes against other storages sharing the WAL.
func (f *FileObjectStorage) TryCompactWAL() error {
	return concurrency.RunInLockWithLogger(&f.walMu, locknames.LockNameWalCompact, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if f.wal == nil {
			return nil
		}
		if f.writeBuf != nil && f.writeBuf.Len() > 0 {
			return nil // Caller should only invoke when buffer is empty
		}
		return f.wal.CompactInPlace(f.projectRoot)
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

const maxStreamEntriesForCacheBuild = 100000

// getObjectFilePath returns the live file path for an object given its ID and kind.
// Identity lookup is cache first (one path), then CAS index, then draft-plane heal on miss.
// List-like callers must drop draft-plane paths. TRACK: TDE-CEF-CAS-IDENTITY-TXN-001

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

func (f *FileObjectStorage) GetLogger() logging.Logger {
	return logging.GetLoggerFromProfile("system")
}

func (f *FileObjectStorage) ValidateObject(ctx context.Context, obj map[string]any, kind, currentState string) error {
	return f.validateObject(ctx, obj, kind, currentState)
}
