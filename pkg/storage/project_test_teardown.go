package storage

import (
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"context"
	"io"
	"path/filepath"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/projecttemp"
)

// ProjectTestTeardownPipelineKind is the pipeline "kind" for metrics/logs.
const ProjectTestTeardownPipelineKind = "storage.project_test_teardown"

type projectTeardownNoopMetrics struct{}

func (projectTeardownNoopMetrics) RecordStage(ctx context.Context, kind, stage string, duration time.Duration, err error) {
}

func (projectTeardownNoopMetrics) RecordStageWithBuckets(ctx context.Context, kind, stage string, duration time.Duration, err error, _ map[string]string) {
}

// ProjectTestTeardownOptions configures the standard temp-project isolation pipeline (t.TempDir, ZQK_TEST_ROOT).
type ProjectTestTeardownOptions struct {
	ProjectRoot string
	FileStorage *FileObjectStorage

	SkipWaitWAL               bool
	WALTimeout                time.Duration
	ShutdownTimeout           time.Duration
	SkipFlushAuditBuffer      bool
	TearDownGlobalAuditBuffer bool
	SecCtx                    *pkgctx.SecurityContext
	AuditBufferResetRoot      string

	StripProcessArtifacts bool

	DrainGlobalListingIndexQueueFirst bool
	GlobalListingIndexFlushTimeout    time.Duration

	// AggressiveTempProjectCleanup is legacy: scrub now runs whenever StripProcessArtifacts is set
	// (non-git roots). Kept for callers that already set it; TempProjectTeardown still sets it true.
	AggressiveTempProjectCleanup bool
	ScrubProjectRootMaxAttempts  int
	ScrubProjectRootBackoff      time.Duration

	OrphanCleanupQueue interface{ Shutdown() }

	Logger logging.Logger
}

// WithDefaults fills zero timeouts and scrub bounds.
func (o ProjectTestTeardownOptions) WithDefaults() ProjectTestTeardownOptions {
	if o.WALTimeout <= 0 {
		o.WALTimeout = 15 * time.Second
	}
	if o.ShutdownTimeout <= 0 {
		o.ShutdownTimeout = 15 * time.Second
	}
	if o.GlobalListingIndexFlushTimeout <= 0 {
		o.GlobalListingIndexFlushTimeout = 5 * time.Second
	}
	if o.ScrubProjectRootMaxAttempts <= 0 {
		o.ScrubProjectRootMaxAttempts = 50
	}
	if o.ScrubProjectRootBackoff <= 0 {
		o.ScrubProjectRootBackoff = 25 * time.Millisecond
	}
	return o
}

func projectTeardownDiscardLogger() logging.Logger {
	return logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(pkgctx.NewSystemContext()))
}

// IsProbableGitWorktreeRoot reports whether dir contains Git metadata at the repository worktree marker
// (see [github.com/zqk-os/zqk/pkg/paths.GitWorktreeMetadataEntry]).
// Implementation lives in [github.com/zqk-os/zqk/pkg/projecttemp] so packages that cannot import storage
// (e.g. pkg/validation) still share the same guard.
func IsProbableGitWorktreeRoot(dir string) bool {
	return projecttemp.IsProbableGitWorktreeRoot(dir)
}

// applyGitWorktreeTeardownGuard clears strip/scrub flags when ProjectRoot looks like a real Git working tree.
func applyGitWorktreeTeardownGuard(opts ProjectTestTeardownOptions) ProjectTestTeardownOptions {
	if opts.ProjectRoot == emptyValue || !IsProbableGitWorktreeRoot(opts.ProjectRoot) {
		return opts
	}
	if !opts.StripProcessArtifacts && !opts.AggressiveTempProjectCleanup {
		return opts
	}
	opts.StripProcessArtifacts = false
	opts.AggressiveTempProjectCleanup = false
	if opts.Logger != nil {
		StorageLog(opts.Logger).Warn(LogEventStorageProjectTestTeardownSkipGitWorktreeWarn).
			ProjectRoot(opts.ProjectRoot).
			Log()
	}
	return opts
}

// StandardProjectTestTeardownPipeline builds the ordered teardown pipeline for temp project roots.
func StandardProjectTestTeardownPipeline(opts ProjectTestTeardownOptions) *pipeline.Pipeline {
	opts = applyGitWorktreeTeardownGuard(opts.WithDefaults())
	logger := opts.Logger
	if logger == nil {
		logger = projectTeardownDiscardLogger()
	}

	return pipeline.NewBuilder(ProjectTestTeardownPipelineKind, logger).
		WithMetricsConfig(&pipeline.MetricsConfig{Sink: projectTeardownNoopMetrics{}, Strategy: pipeline.NoopBucketing{}}).
		AddStage(ConstMiscGlobalListingIndexQueueFlush, projectTeardownStageGlobalListingIndexQueueFlush(opts)).
		AddStage(ConstMiscFlushListingIndex, projectTeardownStageFlushListingIndexes(opts)).
		AddStage(ConstMiscListingIndexQueueShutdown, projectTeardownStageListingIndexQueueShutdown(opts)).
		AddStage("WAIT_WAL", projectTeardownStageWaitWAL(opts)).
		AddStage(ConstMiscFileStorageShutdown, projectTeardownStageFileStorageShutdown(opts)).
		AddStage(ConstMiscOrphanCleanupQueueShutdown, projectTeardownStageOrphanCleanupQueueShutdown(opts)).
		AddStage(ConstMiscFlushAuditBuffer, projectTeardownStageFlushAuditBuffer(opts)).
		AddStage(ConstMiscTeardownGlobalAuditBuffer, projectTeardownStageTearDownGlobalAuditBuffer(opts)).
		AddStage(ConstMiscStripProcessArtifacts, projectTeardownStageStripProcessArtifacts(opts)).
		AddStage(ConstMiscScrubProjectRoot, projectTeardownStageScrubProjectRoot(opts)).
		Build()
}

// TempProjectTeardown returns options for typical t.TempDir trees: strip, scrub, 45s WAL/shutdown, global CAS pre-flush.
func TempProjectTeardown(projectRoot string, fileStorage *FileObjectStorage) ProjectTestTeardownOptions {
	return ProjectTestTeardownOptions{
		ProjectRoot:                       projectRoot,
		FileStorage:                       fileStorage,
		SkipWaitWAL:                       fileStorage == nil,
		StripProcessArtifacts:             true,
		WALTimeout:                        45 * time.Second,
		ShutdownTimeout:                   45 * time.Second,
		DrainGlobalListingIndexQueueFirst: true,
		AggressiveTempProjectCleanup:      true,
	}
}

// RunProjectTestTeardown runs the standard pipeline once.
func RunProjectTestTeardown(opts ProjectTestTeardownOptions) error {
	opts = applyGitWorktreeTeardownGuard(opts.WithDefaults())
	pl := StandardProjectTestTeardownPipeline(opts)
	pctx := &pipeline.Context{Ctx: pkgctx.NewSystemContext(), Outcome: make(map[string]any)}
	_, err := pl.Run(pctx, opts)
	ShutdownAsyncValidationStrategies()
	ShutdownHashRegistriesForProjectTesting(opts.ProjectRoot)
	ShutdownAllAuditBuffersForTesting()
	// Reset global singletons to prevent cross-test contamination when ZQK_TEST_ROOT changes
	objects.ResetGlobalFieldRegistryForTesting()
	objects.ResetGlobalKindMapperForTesting()
	return err
}

// AppendProjectTestTeardownStages appends the same stages as [StandardProjectTestTeardownPipeline].
func AppendProjectTestTeardownStages(b *pipeline.Builder, opts ProjectTestTeardownOptions) *pipeline.Builder {
	if b == nil {
		return nil
	}
	opts = applyGitWorktreeTeardownGuard(opts.WithDefaults())
	return b.
		AddStage(ConstMiscGlobalListingIndexQueueFlush, projectTeardownStageGlobalListingIndexQueueFlush(opts)).
		AddStage(ConstMiscFlushListingIndex, projectTeardownStageFlushListingIndexes(opts)).
		AddStage(ConstMiscListingIndexQueueShutdown, projectTeardownStageListingIndexQueueShutdown(opts)).
		AddStage("WAIT_WAL", projectTeardownStageWaitWAL(opts)).
		AddStage(ConstMiscFileStorageShutdown, projectTeardownStageFileStorageShutdown(opts)).
		AddStage(ConstMiscOrphanCleanupQueueShutdown, projectTeardownStageOrphanCleanupQueueShutdown(opts)).
		AddStage(ConstMiscFlushAuditBuffer, projectTeardownStageFlushAuditBuffer(opts)).
		AddStage(ConstMiscTeardownGlobalAuditBuffer, projectTeardownStageTearDownGlobalAuditBuffer(opts)).
		AddStage(ConstMiscStripProcessArtifacts, projectTeardownStageStripProcessArtifacts(opts)).
		AddStage(ConstMiscScrubProjectRoot, projectTeardownStageScrubProjectRoot(opts))
}

func projectTeardownStageGlobalListingIndexQueueFlush(opts ProjectTestTeardownOptions) pipeline.StageFunc {
	return func(pctx *pipeline.Context, payload any) (any, error) {
		if !opts.DrainGlobalListingIndexQueueFirst {
			return payload, nil
		}
		if q := caspkg.GetGlobalListingIndexWriteQueue(); q != nil {
			logging.LogSwallowedError(q.FlushAll(opts.GlobalListingIndexFlushTimeout))
		}
		return payload, nil
	}
}

func projectTeardownStageOrphanCleanupQueueShutdown(opts ProjectTestTeardownOptions) pipeline.StageFunc {
	return func(pctx *pipeline.Context, payload any) (any, error) {
		if opts.OrphanCleanupQueue != nil {
			opts.OrphanCleanupQueue.Shutdown()
		}
		return payload, nil
	}
}

func projectTeardownStageScrubProjectRoot(opts ProjectTestTeardownOptions) pipeline.StageFunc {
	return func(pctx *pipeline.Context, payload any) (any, error) {
		// STRIP_PROCESS_ARTIFACTS removes .zqk/process and .zqk, but async WAL/listeners or test code
		// can leave other top-level entries or recreate files before RemoveAll runs. Scrub all children
		// of the temp project root (bounded retries) whenever we strip, so t.TempDir cleanup does not
		// flake with "directory not empty" on macOS. This must not be gated on AggressiveTempProjectCleanup:
		// many tests only set StripProcessArtifacts.
		if !opts.StripProcessArtifacts || opts.ProjectRoot == emptyValue {
			return payload, nil
		}
		if IsProbableGitWorktreeRoot(opts.ProjectRoot) {
			return payload, nil
		}
		ScrubProjectRootForTempCleanup(opts.ProjectRoot, opts.ScrubProjectRootMaxAttempts, opts.ScrubProjectRootBackoff)
		return payload, nil
	}
}

// ScrubProjectRootForTempCleanup removes every child of projectRoot in a bounded retry loop until empty or maxAttempts.
func ScrubProjectRootForTempCleanup(projectRoot string, maxAttempts int, backoff time.Duration) {
	if projectRoot == emptyValue || maxAttempts <= 0 {
		return
	}
	if IsProbableGitWorktreeRoot(projectRoot) {
		return
	}
	if backoff <= 0 {
		backoff = 25 * time.Millisecond
	}
	for range maxAttempts {
		entries, err := fileutil.ReadDir(projectRoot)
		if err != nil || len(entries) == 0 {
			return
		}
		for _, e := range entries {
			logging.LogSwallowedError(fileutil.RemoveAll(filepath.Join(projectRoot, e.Name())))
		}
		time.Sleep(backoff)
	}
}

func projectTeardownStageFlushListingIndexes(opts ProjectTestTeardownOptions) pipeline.StageFunc {
	return func(pctx *pipeline.Context, payload any) (any, error) {
		if opts.ProjectRoot == emptyValue {
			return payload, nil
		}
		logging.LogSwallowedError(FlushAllListingIndexesForProjectRoot(opts.ProjectRoot))
		return payload, nil
	}
}

func projectTeardownStageListingIndexQueueShutdown(opts ProjectTestTeardownOptions) pipeline.StageFunc {
	return func(pctx *pipeline.Context, payload any) (any, error) {
		if opts.ProjectRoot == emptyValue {
			return payload, nil
		}
		q := caspkg.GetListingIndexWriteQueueForProjectRoot(opts.ProjectRoot)
		if q != nil {
			logging.LogSwallowedError(q.FlushAll(5 * time.Second))
			logging.LogSwallowedError(q.Shutdown())
		}
		return payload, nil
	}
}

func projectTeardownStageWaitWAL(opts ProjectTestTeardownOptions) pipeline.StageFunc {
	return func(pctx *pipeline.Context, payload any) (any, error) {
		if opts.SkipWaitWAL || opts.ProjectRoot == emptyValue {
			return payload, nil
		}
		logging.LogSwallowedError(WaitForWALProcessing(opts.ProjectRoot, opts.WALTimeout))
		return payload, nil
	}
}

func projectTeardownStageFlushAuditBuffer(opts ProjectTestTeardownOptions) pipeline.StageFunc {
	return func(pctx *pipeline.Context, payload any) (any, error) {
		if opts.SkipFlushAuditBuffer || opts.ProjectRoot == emptyValue {
			return payload, nil
		}
		FlushGlobalAuditBufferForProjectRoot(opts.ProjectRoot)
		return payload, nil
	}
}

func projectTeardownStageTearDownGlobalAuditBuffer(opts ProjectTestTeardownOptions) pipeline.StageFunc {
	return func(pctx *pipeline.Context, payload any) (any, error) {
		if !opts.TearDownGlobalAuditBuffer || opts.ProjectRoot == emptyValue || opts.SecCtx == nil || opts.AuditBufferResetRoot == emptyValue {
			return payload, nil
		}
		logging.LogSwallowedError(TearDownGlobalAuditBufferForTestProjectRoot(opts.ProjectRoot, opts.AuditBufferResetRoot, opts.SecCtx))
		return payload, nil
	}
}

func projectTeardownStageFileStorageShutdown(opts ProjectTestTeardownOptions) pipeline.StageFunc {
	return func(pctx *pipeline.Context, payload any) (any, error) {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), opts.ShutdownTimeout) // Background: request-or-shutdown derived
		defer cancel()

		if opts.FileStorage != nil {
			logging.LogSwallowedError(opts.FileStorage.Shutdown(shutdownCtx))
		}

		if opts.ProjectRoot != "" {
			if provider, ok := GetGlobalStorageProviderCache().Get(opts.ProjectRoot); ok {
				if provider != nil {
					if err := provider.Shutdown(shutdownCtx); err != nil {
						logging.LogSwallowedError(err)
					}
				}
				GetGlobalStorageProviderCache().Delete(opts.ProjectRoot)
			}
			GetGlobalBufferRegistry().Unregister(opts.ProjectRoot)
			caspkg.RemoveListingIndexWriteQueueForProjectRoot(opts.ProjectRoot)
		}

		return payload, nil
	}
}

func projectTeardownStageStripProcessArtifacts(opts ProjectTestTeardownOptions) pipeline.StageFunc {
	return func(pctx *pipeline.Context, payload any) (any, error) {
		if !opts.StripProcessArtifacts || opts.ProjectRoot == emptyValue {
			return payload, nil
		}
		if err := projecttemp.RunIsolatedRootStrip(opts.ProjectRoot); err != nil {
			return nil, err
		}
		return payload, nil
	}
}
