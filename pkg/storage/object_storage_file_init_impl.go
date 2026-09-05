package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
	"github.com/lanceman/zqk/pkg/storage/filecas"
	"github.com/lanceman/zqk/pkg/storage/locknames"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/when"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

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
		projectRoot = paths.ResolveProjectRoot(".")
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

	if _, err := fileutil.Stat(processDir); err != nil {
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
	specsDir := filepath.Join(processDir, "_internal", "object_specs")
	specLoader := objects.NewSpecLoader(specsDir)

	// Note: We create instance-specific loaders rather than using global ones
	// to ensure each storage instance uses the correct project's specs/lifecycles.
	// Global loader is still available for other use cases, but storage uses instance-specific.

	lifecyclesDir := filepath.Join(processDir, "_internal", "lifecycles")
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
		casCache:               &ResourceCache[*filecas.ContentAddressableStorage]{},
		hashRegistryCache:      &ResourceCache[*HashRegistry]{},
	}

	// Optionally skip wiring to globals (test isolation)
	if opt != nil && opt.SkipGlobalWiring {
		f.skipGlobalWiring = true
		return f, nil
	}

	// Reverse-ref dependents cache: load disk snapshot for this project so cross-process
	// lookups (promote membership, shockwave) see prior CUD. Incremental SaveCache follows
	// create/update/delete. TRACK: [REDACTED-ID]
	BindReverseReferenceIndexProjectRoot(projectRoot)

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
	// See [REDACTED-ID] for design details.
	if !f.skipGlobalWiring {
		InitializeAsyncValidationStrategies(projectRoot)
	}

	// Write-behind (Option B): shared ObjectWAL per project root + at most one worker.
	// Extra NewFileObjectStorage calls must not OpenFile(object.wal) again — CompactWAL
	// renames leave deleted-inode FDs and explode handle counts (see object_wal_registry.go).
	// Privileged-writer daemon is the CAS endpoint, not a second WAL owner: replay here
	// is the 8GB / EMFILE footgun (sample 2026-08-26 zqk-stable object daemon).
	// TRACK: BLI-CEF-R20-SINGLE-WRITER-BLI-001
	if projectRoot != emptyValue && !f.skipGlobalWiring && !zqkenv.PrivilegedWriterDaemonRole() {
		wal, err := AcquireObjectWAL(projectRoot)
		if err != nil {
			StorageLog(logger).Warn(LogEventStorageObjectFileWALInitFailedSyncFallbackWarn).WithError(err).Log()
		} else {
			f.wal = wal
			if tryClaimWriteBehindOwner(projectRoot, f) {
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
					releaseWriteBehindOwner(projectRoot, f)
					_ = ReleaseObjectWAL(projectRoot, wal)
					f.wal = nil
					return nil, startErr
				}
				GetGlobalShutdownCoordinator().RegisterQueue(f.writeBehindWorker)
			} else {
				logWriteBehindOwnerSkipped(projectRoot)
			}
		}
	}

	if err := rejectWriteBehindOnPrivilegedWriterRole(f); err != nil {
		return nil, err
	}

	return f, nil
}
