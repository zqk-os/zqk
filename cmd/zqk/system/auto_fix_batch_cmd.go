package system

import (
	"bytes"
	stdcontext "context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/spf13/cobra"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	bldr_cli_cmd_v1 "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/config"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/resourcehygiene"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/zqkenv"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const (
	autoFixTraceStatusFixedPersisted  = "fixed_persisted"
	autoFixTraceStatusFixedNotPersist = "fixed_not_persisted"
	autoFixTraceStatusChunkRolledBack = "failed_chunk_rollback"
	autoFixTracePersistenceCommitted  = "object_update_committed"
	autoFixTracePersistenceAborted    = "object_update_aborted"
	autoFixTracePersistenceRolledBack = "rolled_back"

	// Pipeline stage Outcome keys (autofix_batch); stable across DECIDE/COMMIT observability.
	autofixOutcomeKeyIngestCount     = "ingest_count"
	autofixOutcomeKeyNormalizedCount = "normalized_count"
	autofixOutcomeKeyCommitResult    = "commit_result"
	autofixOutcomeKeyCommitSuccess   = "commit_success"
	autofixOutcomeKeyFinalizeCount   = "finalize_count"
	// Audit metadata keys (batch completion event); not ontology field keys.
	autofixAuditMetadataKeyErrorCount   = "error_count"
	autofixAuditMetadataKeySampleErrors = "sample_errors"

	autoFixBatchFilePrefix            = "AUTOFIX-"
	autoFixBatchFileSuffix            = ".json"
	autoFixBatchFixedPrefix           = "FIXED_"
	autoFixBatchDonePrefix            = "PROCESSED_"
	autoFixBatchTargetKind            = "auto_fix_batch"
	autoFixSeverityLow                = "low"
	autoFixSeverityMedium             = "medium"
	autoFixEventTypeSystem            = "system_config_change"
	autoFixStageKind                  = "autofix_batch"
	autoFixStageIngest                = "INGEST"
	autoFixStageNormalize             = "NORMALIZE"
	autoFixStageCommit                = "COMMIT"
	autoFixStageFinalize              = "FINALIZE"
	autoFixSourceCLI                  = "cli"
	autoFixStatusProcessing           = "processing"
	autoFixStatusCompleted            = "completed"
	autoFixStatusCompletedErr         = "completed_with_errors"
	autoFixEventStart                 = "start"
	autoFixEventProgress              = "progress"
	autoFixEventComplete              = "complete"
	autoFixEventCompleteErr           = "complete_with_errors"
	autoFixEventBatchStart            = "batch_start"
	autoFixEventBatchProgress         = "batch_progress"
	autoFixEventBatchComplete         = "batch_complete"
	autoFixFieldEvent                 = "event"
	autoFixFieldBatchID               = "batch_id"
	autoFixFieldTotalIssues           = "total_issues"
	autoFixMetricsOp                  = "auto_fix_batch"
	autoFixFieldTotal                 = "total"
	autoFixTagSystem                  = "system"
	autoFixTagAutoFix                 = "auto_fix"
	autoFixTagBatchProcessing         = "batch_processing"
	autoFixTagHasErrors               = "has_errors"
	autoFixMetricTypeSystem           = "system"
	autoFixMetricSource               = "auto_fix_batch"
	autoFixCreatedBySystem            = pkgctx.SystemAccountID
	autoFixMetricStatusImplemented    = "implemented"
	autoFixTraceStatusFixedPrepared   = "fixed_prepared"
	autoFixTraceStatusSkippedNoChange = "skipped_no_change"
	autoFixProcessingLockFile         = "autofix-processing.lock"
)

var (
	autoFixGlossarySpecsDir  = paths.ProcessInternalObjectSpecsDir
	autoFixGlossaryLifeDir   = paths.ProcessInternalLifecyclesDir
	autoFixGlossaryConfigDir = paths.ProcessInternalConfigsDir
)

// NewAutoFixProcessPendingCmd creates the auto-fix-process-pending command (process all AUTOFIX-*.json in .zqk/autofix/).
// Structure and help from .zqk/cli/specs/system/auto_fix_process_pending_command.yaml (generate-command-builders).
func NewAutoFixProcessPendingCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemAutoFixProcessPendingCommandBuilder()
	cmd.Hidden = true
	cmd.RunE = runAutoFixProcessPending
	return cmd
}

// runAutoFixProcessPending discovers all AUTOFIX-*.json under .zqk/autofix/ and processes each with the batch processor.
func runAutoFixProcessPending(cmd *cobra.Command, args []string) error {
	projectRoot, _ := cmd.Flags().GetString("project-root")
	projectRoot = ProjectRootOrResolve(projectRoot)
	if projectRoot == emptyValue {
		return errfmt.Errorf("--project-root is required when not run from a project directory")
	}
	syncGlossary, _ := cmd.Flags().GetBool("sync-glossary")
	syncGlossaryApply, _ := cmd.Flags().GetBool("sync-glossary-apply")

	autofixDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.AutofixDir)
	entries, err := fileutil.ReadDir(autofixDir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			// No autofix dir, nothing to do; still allow glossary maintenance if requested.
			if syncGlossary {
				return runGlossaryMaintenanceAfterAutofix(cmd, projectRoot, nil, syncGlossaryApply)
			}
			return nil
		}
		return errfmt.Newf("failed to read autofix directory").Wrap(err)
	}

	var batchFiles []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, autoFixBatchFilePrefix) && strings.HasSuffix(name, autoFixBatchFileSuffix) {
			batchFiles = append(batchFiles, filepath.Join(autofixDir, name))
		}
	}
	if len(batchFiles) == 0 {
		return nil
	}

	ctx, err := getAutofixCLIContext(cmd, projectRoot)
	if err != nil {
		return err
	}
	logger := logging.GetLoggerFromProfile(ctx.Profile)
	logging.Fluent(logger).Info("Processing pending autofix batches").
		Int("file_count", len(batchFiles)).
		Log()

	chunkSize := DefaultAutofixTransactionalChunkSize
	if envChunk := config.MaintenanceAutofixBatchChunkSize().OrDefault(50); envChunk > 0 {
		chunkSize = envChunk
	}

	// Create ONE storage factory shared across all batch files.
	// Previously each ProcessBatch + FINALIZE stage + createBatchAuditEvent each called
	// NewStorageFactory, which calls NewFileObjectStorage (spec loader EnsureReady + loaders)
	// per invocation. With N batch files that was ~2N synchronous inits on the critical path.
	// Sharing a single provider reduces this to exactly 1 init regardless of file count.
	stdctx := pkgctx.NewSystemContext()
	var sharedProvider storage.ObjectStorageProvider
	if sf, sfErr := storage.NewStorageFactory(stdctx, projectRoot); sfErr == nil && sf != nil {
		sharedProvider = sf.GetStorage()
		defer func() {
			if fos, ok := sharedProvider.(*storage.FileObjectStorage); ok {
				_ = fos.Shutdown(stdcontext.Background()) // Background: request-or-shutdown derived
			}
		}()
	} else {
		logging.Fluent(logger).Warn("Shared storage factory init failed; each batch will create its own provider").
			WithError(sfErr).
			Log()
	}

	processor := NewAutoFixBatchProcessor(projectRoot, logger, sharedProvider)
	processed := 0
	for _, batchFile := range batchFiles {
		openFDs, maxFDs, fdErr := resourcehygiene.GetProcessFDUsage()
		if fdErr == nil && maxFDs > 0 && openFDs > int(float64(maxFDs)*0.70) {
			logging.Fluent(logger).Warn("Autofix batch processing paused due to high FD usage").
				Int("open_fds", openFDs).
				Int("max_fds", maxFDs).
				Log()
			time.Sleep(200 * time.Millisecond)
		}
		if err := processOneBatchFile(cmd, batchFile, projectRoot, ctx, processor, chunkSize); err != nil {
			logging.Fluent(logger).Warn("Failed to process autofix batch file").
				File(batchFile).
				WithError(err).
				Log()
			continue
		}
		processed++
	}
	logging.NewEvent("Auto-fix process-pending completed").
		Processed(processed).
		Int("total_files", len(batchFiles)).
		Info(logger)

	if syncGlossary {
		return runGlossaryMaintenanceAfterAutofix(cmd, projectRoot, sharedProvider, syncGlossaryApply)
	}
	return nil
}

func runGlossaryMaintenanceAfterAutofix(cmd *cobra.Command, projectRoot string, sharedProvider storage.ObjectStorageProvider, apply bool) error {
	_, logger := resolveCommandLogger(cmd, systemProfileHuman)

	specsDir := filepath.Join(projectRoot, autoFixGlossarySpecsDir)
	lifecyclesDir := filepath.Join(projectRoot, autoFixGlossaryLifeDir)
	configsDir := filepath.Join(projectRoot, autoFixGlossaryConfigDir)

	commandSpecsDir := filepath.Join(projectRoot, paths.CLICommandSpecsDir)
	cands, err := collectGlossaryCandidates(projectRoot, specsDir, commandSpecsDir, lifecyclesDir, configsDir)
	if err != nil {
		return err
	}
	sp := sharedProvider
	if sp == nil {
		// Avoid reintroducing per-batch factory init churn: only init when missing.
		sf, sfErr := storage.NewStorageFactory(pkgctx.NewSystemContext(), projectRoot)
		if sfErr != nil {
			return errfmt.Newf("glossary sync: storage factory").Wrap(sfErr)
		}
		sp = sf.GetStorage()
	}
	titles, sourceKeys, err := loadGlossaryIngestDedupeIndex(sp, cmd)
	if err != nil {
		return err
	}
	missing := make([]glossaryCandidate, 0, len(cands))
	for _, c := range cands {
		if _, ok := titles[c.Title]; !ok {
			missing = append(missing, c)
		}
	}
	if len(missing) == 0 {
		logging.Fluent(logger).Info("autofix: glossary sync found no missing terms").Log()
		return nil
	}
	logging.Fluent(logger).Info("autofix: glossary sync missing terms detected").
		Int("missing", len(missing)).
		Bool("apply", apply).
		Log()

	if !apply {
		return nil
	}

	// Safety cap: do not create an unbounded number of GLS rows from scheduler jobs.
	maxCreate := config.MaintenanceAutofixGlossaryMaxCreate().OrDefault(10)
	if maxCreate <= 0 {
		maxCreate = 25
	}
	if len(missing) > maxCreate {
		return errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("autofix: glossary sync refused to create %d terms (cap %d). Set MAINTENANCE_AUTOFIX_GLOSSARY_MAX_CREATE to override, or run `zqk system sync-glossary-from-specs --apply --dry-run=false` manually", len(missing), maxCreate)))
	}
	created := 0
	skipped := 0
	for _, c := range missing {
		didSkip, err := createGlossaryCandidate(cmd, sp, c, titles, sourceKeys)
		if err != nil {
			return err
		}
		if didSkip {
			skipped++
			continue
		}
		created++
	}
	logging.Fluent(logger).Info("autofix: glossary sync created missing terms").
		Int("created", created).
		Int("skipped_duplicate_ingest", skipped).
		Log()
	return nil
}

// processOneBatchFile reads, processes, and renames one AUTOFIX-*.json batch file.
// Uses the standardized pipeline (INGEST → NORMALIZE → COMMIT → FINALIZE) per docs/architecture/data-pipeline-lifecycle.md.
func processOneBatchFile(cmd *cobra.Command, batchFile, projectRoot string, ctx *cli.Context, processor *AutoFixBatchProcessor, chunkSize int) error {
	return runOneBatchViaPipeline(ctx, cmd, batchFile, projectRoot, processor, chunkSize)
}

// runOneBatchViaPipeline runs one autofix batch through the canonical pipeline stages (INGEST → NORMALIZE → COMMIT → FINALIZE).
// Per-stage outcomes are recorded in pipeline context and via MetricsSink for observability.
// processor may be nil; when nil a fresh processor (and storage factory) is created for this batch only.
func runOneBatchViaPipeline(ctx *cli.Context, cmd *cobra.Command, batchFile, projectRoot string, processor *AutoFixBatchProcessor, chunkSize int) error {
	logger := logging.GetLoggerFromProfile(ctx.Profile)
	processingLock, acquired, err := tryAcquireAutoFixProcessingLock(projectRoot)
	if err != nil {
		return err
	}
	if !acquired {
		logging.Fluent(logger).Info("Another auto-fix processor owns the batch lock; leaving file pending").
			File(batchFile).
			Log()
		return nil
	}
	defer func() {
		if closeErr := processingLock.Close(); closeErr != nil {
			logging.Fluent(logger).Warn("Failed to close auto-fix processing lock").
				WithError(closeErr).
				Log()
		}
	}()
	if processor == nil {
		processor = NewAutoFixBatchProcessor(projectRoot, logger, nil)
	}
	basename := filepath.Base(batchFile)
	receivedAt := time.Now()

	pl := pipeline.NewBuilder(autoFixStageKind, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage(autoFixStageIngest, func(pctx *pipeline.Context, payload any) (any, error) {
			path, _ := payload.(string)
			raw, err := fileutil.ReadFile(path)
			if err != nil {
				return nil, err
			}
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[autofixOutcomeKeyIngestCount] = 1
			env := &pipeline.Envelope{
				TraceID:        basename,
				Source:         autoFixSourceCLI,
				ReceivedAt:     receivedAt,
				IdempotencyKey: "autofix_batch:" + projectRoot + ":" + basename,
				PartitionKey:   projectRoot,
				Payload:        raw,
			}
			pctx.IdempotencyKey = env.IdempotencyKey
			pctx.PartitionKey = env.PartitionKey
			return env, nil
		}).
		AddStage(autoFixStageNormalize, func(pctx *pipeline.Context, payload any) (any, error) {
			env, ok := nildecode.DecodeNonNilPayload[*pipeline.Envelope](payload)
			if !ok {
				return nil, errfmt.Errorf("NORMALIZE expected Envelope, got %T", payload)
			}
			raw, ok := env.Payload.([]byte)
			if !ok {
				return nil, errfmt.Errorf("NORMALIZE expected []byte payload, got %T", env.Payload)
			}
			batch, trailingData, err := decodeAutoFixBatch(raw)
			if err != nil {
				return nil, err
			}
			if trailingData {
				logging.Fluent(logger).Warn("Recovered auto-fix batch with trailing data from a concurrent write").
					File(batchFile).
					Log()
			}
			if pctx.Outcome != nil {
				pctx.Outcome[autofixOutcomeKeyNormalizedCount] = 1
			}
			batch.Status = autoFixStatusProcessing
			batch.Progress.LastUpdate = time.Now()
			env.Payload = &batch
			emitBatchStartEvent(batch.BatchID, batch.Progress.Total)
			return env, nil
		}).
		AddStage(autoFixStageCommit, func(pctx *pipeline.Context, payload any) (any, error) {
			env, ok := nildecode.DecodeNonNilPayload[*pipeline.Envelope](payload)
			if !ok {
				return nil, errfmt.Errorf("COMMIT expected Envelope, got %T", payload)
			}
			batch, ok := nildecode.DecodeNonNilPayload[*AutoFixBatch](env.Payload)
			if !ok {
				return nil, errfmt.Errorf("COMMIT expected *AutoFixBatch payload, got %T", env.Payload)
			}
			result := processor.ProcessBatch(ctx, cmd, batch, batch.BatchID, chunkSize)
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[autofixOutcomeKeyCommitResult] = result
			pctx.Outcome[autofixOutcomeKeyCommitSuccess] = result.Failed == 0
			return env, nil
		}).
		AddStage(autoFixStageFinalize, func(pctx *pipeline.Context, payload any) (any, error) {
			env, ok := nildecode.DecodeNonNilPayload[*pipeline.Envelope](payload)
			if !ok {
				return nil, errfmt.Errorf("FINALIZE expected Envelope, got %T", payload)
			}
			batch, ok := nildecode.DecodeNonNilPayload[*AutoFixBatch](env.Payload)
			if !ok {
				return payload, nil
			}
			resultVal := pctx.Outcome[autofixOutcomeKeyCommitResult]
			result, _ := resultVal.(*ProcessBatchResult)
			if result != nil {
				if result.Failed > 0 {
					batch.Status = autoFixStatusCompletedErr
				} else {
					batch.Status = autoFixStatusCompleted
				}
				batch.Progress = result.Progress
				batch.Progress.LastUpdate = time.Now()
				batch.Trace = result.Trace
			}
			updatedBatchData, err := json.MarshalIndent(batch, "", "  ")
			if err == nil {
				_ = fileutil.WriteFile(batchFile, updatedBatchData, paths.FilePerm644) //nolint:errcheck,gosec
			}
			duration := time.Since(receivedAt)
			if result != nil {
				logging.NewEvent("Auto-fix batch completed").
					BatchID(batch.BatchID).
					Processed(result.Processed).
					Fixed(result.Fixed).
					Failed(result.Failed).
					Duration("duration", duration).
					Info(logger)
				emitBatchCompleteEvent(batch.BatchID, result, duration)
				stdctx := pkgctx.NewSystemContext()
				// Use the shared provider when available; only create a new factory as fallback
				// so FINALIZE does not add another expensive NewFileObjectStorage call per file.
				sp := processor.storageProvider
				var localSp bool
				if sp == nil {
					if sf, sfErr := storage.NewStorageFactory(stdctx, projectRoot); sfErr == nil && sf != nil {
						sp = sf.GetStorage()
						localSp = true
					}
				}
				if sp != nil {
					createBatchMetrics(stdctx, sp, batch.BatchID, result, duration)
				}
				createBatchAuditEvent(stdctx, projectRoot, batch.BatchID, result, duration, sp)
				if localSp && sp != nil {
					if fos, ok := sp.(*storage.FileObjectStorage); ok {
						_ = fos.Shutdown(stdcontext.Background()) // Background: request-or-shutdown derived
					}
				}
			}
			if pctx.Outcome != nil {
				pctx.Outcome[autofixOutcomeKeyFinalizeCount] = 1
			}
			_ = renameBatchFileAfterProcessing(batchFile, result == nil || result.Failed == 0)
			return payload, nil
		}).
		Build()

	pctx := &pipeline.Context{Ctx: pkgctx.NewSystemContext(), Outcome: make(map[string]any)}
	_, err = pl.Run(pctx, batchFile)
	if err != nil {
		return err
	}
	resultVal := pctx.Outcome[autofixOutcomeKeyCommitResult]
	result, _ := resultVal.(*ProcessBatchResult)
	if result != nil && result.Failed > 0 {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Warn(fmt.Sprintf("batch completed with failures (failed: %d, fixed: %d)", result.Failed, result.Fixed)).
			Log()
		// We do not return an error here because the batch file is renamed,
		// and retrying the command would fail with 'no such file or directory'.
	}
	return nil
}

func tryAcquireAutoFixProcessingLock(projectRoot string) (*storage.FileLock, bool, error) {
	lockPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.LockDir, autoFixProcessingLockFile)
	lock, err := storage.NewFileLock(lockPath)
	if err != nil {
		return nil, false, errfmt.Newf("failed to open auto-fix processing lock").Wrap(err)
	}
	acquired, err := lock.TryLock()
	if err != nil {
		_ = lock.Close()
		return nil, false, errfmt.Newf("failed to acquire auto-fix processing lock").Wrap(err)
	}
	if !acquired {
		_ = lock.Close()
		return nil, false, nil
	}
	return lock, true, nil
}

// ErrAutoFixLockBusy indicates another system check --auto-fix or autofix batch process holds the exclusive lock.
var ErrAutoFixLockBusy = errors.New("another system check --auto-fix (or autofix batch) is already running; wait for it to finish")

// acquireExclusiveAutoFixCheckLock fail-closes overlapping `system check --auto-fix` processes.
// Three concurrent checks (parent launchd) were observed at 227% CPU / 1000+ OS threads.
func acquireExclusiveAutoFixCheckLock(cmd *cobra.Command) (unlock func(), err error) {
	if cmd == nil {
		return nil, nil
	}
	autoFix, _ := cmd.Flags().GetBool("auto-fix")
	force, _ := cmd.Flags().GetBool("force")
	if !autoFix && !force {
		return nil, nil
	}
	projectRoot := ""
	if cliCtx := cli.GetContext(cmd); cliCtx != nil && cliCtx.ProjectRoot != emptyValue {
		projectRoot = cliCtx.ProjectRoot
	}
	projectRoot = ProjectRootOrResolve(projectRoot)
	lock, acquired, err := tryAcquireAutoFixProcessingLock(projectRoot)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, ErrAutoFixLockBusy
	}
	return func() {
		_ = lock.Close()
	}, nil
}

func decodeAutoFixBatch(raw []byte) (AutoFixBatch, bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var batch AutoFixBatch
	if err := decoder.Decode(&batch); err != nil {
		return AutoFixBatch{}, false, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); errors.Is(err, io.EOF) {
		return batch, false, nil
	}
	return batch, true, nil
}

// NewAutoFixBatchCmd creates the auto-fix-batch command
// This command processes a batch of auto-fixable issues submitted by the scheduler
func NewAutoFixBatchCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemAutoFixProcessPendingCommandBuilder(), &cobra.Command{
		Use:    "auto-fix-batch",
		Short:  "Process a batch of auto-fixable issues (internal command, called by scheduler)",
		Hidden: true, // Hide from help - this is an internal command
		RunE:   runAutoFixBatch,
	})

	cmd.Flags().String("batch-file", "", "Path to batch JSON file (required)")
	// cmd.Flags().String("project-root", "", "Project root directory (required)")
	cmd.Flags().Int("chunk-size", 0, fmt.Sprintf("Number of issues per transactional chunk (0 = use default or %s env; default 20)", zqkenv.AutofixBatchChunkSize()))

	return cmd
}

// runAutoFixBatch processes a batch of auto-fixable issues via the standardized pipeline
// (INGEST → NORMALIZE → COMMIT → FINALIZE). See docs/architecture/data-pipeline-lifecycle.md and data-pipeline-pilot-selection.md.
func runAutoFixBatch(cmd *cobra.Command, args []string) error {
	batchFile, _ := cmd.Flags().GetString("batch-file")
	projectRoot, _ := cmd.Flags().GetString("project-root")

	if batchFile == emptyValue {
		return errfmt.Errorf("--batch-file is required")
	}
	if projectRoot == emptyValue {
		return errfmt.Errorf("--project-root is required")
	}

	ctx, err := getAutofixCLIContext(cmd, projectRoot)
	if err != nil {
		return err
	}

	chunkSize, _ := cmd.Flags().GetInt("chunk-size")
	if chunkSize <= 0 {
		chunkSize = DefaultAutofixTransactionalChunkSize
		if envChunk := config.MaintenanceAutofixBatchChunkSize().OrDefault(50); envChunk > 0 {
			chunkSize = envChunk
		}
	}

	return runOneBatchViaPipeline(ctx, cmd, batchFile, projectRoot, nil, chunkSize)
}

// AutoFixBatchProcessor processes batches of auto-fixable issues
type AutoFixBatchProcessor struct {
	projectRoot     string
	logger          logging.Logger
	pl              *pipeline.Pipeline
	storageProvider storage.ObjectStorageProvider // shared provider; nil = create per-batch (expensive)
}

// DefaultAutofixTransactionalChunkSize is the number of issues applied in one transaction.
// Committing per chunk allows incremental progress: if a later chunk fails, earlier chunks are already committed.
const DefaultAutofixTransactionalChunkSize = 20

// NewAutoFixBatchProcessor creates a new batch processor.
// storageProvider is an optional pre-created provider shared across many batch files.
// Passing a non-nil provider avoids NewFileObjectStorage init (spec loading) on every batch,
// reducing per-file overhead from O(N×init_cost) to O(1×init_cost) for the caller's run.
func NewAutoFixBatchProcessor(projectRoot string, logger logging.Logger, storageProvider storage.ObjectStorageProvider) *AutoFixBatchProcessor {
	pl := pipeline.NewBuilder(autoFixStageKind, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage("process_chunks", func(pctx *pipeline.Context, payload any) (any, error) {
			fn, ok := payload.(func(*pipeline.Context) error)
			if !ok || fn == nil {
				return nil, errfmt.Errorf("unexpected payload type for autofix_batch pipeline")
			}
			return nil, fn(pctx)
		}).
		Build()

	return &AutoFixBatchProcessor{
		projectRoot:     projectRoot,
		logger:          logger,
		pl:              pl,
		storageProvider: storageProvider,
	}
}

// ProcessBatchResult represents the result of processing a batch
type ProcessBatchResult struct {
	Processed int
	Fixed     int
	Failed    int
	Skipped   int
	Progress  AutoFixBatchProgress
	Errors    []string
	Trace     []AutoFixIssueTrace
}

// ProcessBatch processes a batch of auto-fixable issues in optimally-sized transactional chunks.
// Each chunk is committed independently so partial progress is persisted even when some fixes fail or return empty.
// chunkSize is the number of issues per transaction (use DefaultAutofixTransactionalChunkSize or > 0).
func (abp *AutoFixBatchProcessor) ProcessBatch(ctx *cli.Context, cmd *cobra.Command, batch *AutoFixBatch, batchID string, chunkSize int) *ProcessBatchResult {
	issues := normalizeAutoFixBatchIssues(batch)
	result := &ProcessBatchResult{
		Progress: AutoFixBatchProgress{
			Total:      len(issues),
			LastUpdate: time.Now(),
		},
		Errors: []string{},
		Trace:  []AutoFixIssueTrace{},
	}

	stdctx := pkgctx.NewSystemContext()
	// Use the shared provider (set once by the caller for all batch files); fall back to
	// creating a new factory only when processing a standalone batch (e.g. runAutoFixBatch).
	var storageProvider storage.ObjectStorageProvider
	if abp.storageProvider != nil {
		storageProvider = abp.storageProvider
	} else {
		storageFactory, sfErr := storage.NewStorageFactory(stdctx, abp.projectRoot)
		if sfErr != nil {
			result.Failed = len(issues)
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to create storage factory: %v", sfErr))
			return result
		}
		storageProvider = storageFactory.GetStorage()
		if storageProvider == nil {
			result.Failed = len(issues)
			result.Errors = append(result.Errors, "Storage provider is nil")
			return result
		}
		defer func() {
			if fos, ok := storageProvider.(*storage.FileObjectStorage); ok {
				_ = fos.Shutdown(stdcontext.Background())
			}
		}()
	}

	hashRegistryCache := &HashRegistryCacheType{
		cache: make(map[string]storage.HashRegistryProvider),
	}
	objectIDCache := GetGlobalObjectIDCache()
	valCache := validation.NewValidationStateCache(abp.projectRoot, 24*time.Hour)
	_ = valCache.Load() // best effort
	secCtx := pkgctx.NewSystemSecurityContext()
	if chunkSize <= 0 {
		chunkSize = DefaultAutofixTransactionalChunkSize
	}

	pctx := &pipeline.Context{
		Ctx: pkgctx.NewSystemContext(),
		Outcome: map[string]any{
			objects.FieldKeyBatchID: batchID,
		},
	}

	work := func(_ *pipeline.Context) error {
		for chunkStart := 0; chunkStart < len(issues); chunkStart += chunkSize {
			openFDs, maxFDs, fdErr := resourcehygiene.GetProcessFDUsage()
			if fdErr == nil && maxFDs > 0 && openFDs > int(float64(maxFDs)*0.75) {
				result.Errors = append(result.Errors, fmt.Sprintf("Autofix chunk processing throttled: FD usage %d/%d", openFDs, maxFDs))
				time.Sleep(150 * time.Millisecond)
				openFDs, maxFDs, _ = resourcehygiene.GetProcessFDUsage()
				if maxFDs > 0 && openFDs > int(float64(maxFDs)*0.85) {
					result.Errors = append(result.Errors, fmt.Sprintf("Autofix chunk processing aborted: critical FD usage %d/%d", openFDs, maxFDs))
					break
				}
			}

			chunkEnd := chunkStart + chunkSize
			if chunkEnd > len(issues) {
				chunkEnd = len(issues)
			}
			chunk := issues[chunkStart:chunkEnd]

			chunkTx, txErr := storage.NewTransactionalStorageWrapper(stdctx, storageProvider)
			if txErr != nil {
				for _, batchIssue := range chunk {
					result.Processed++
					result.Failed++
					result.Errors = append(result.Errors, fmt.Sprintf("Failed to start transaction for %s: %v", batchIssue.ObjectID, txErr))
				}
				result.Progress.Processed = result.Processed
				result.Progress.LastUpdate = time.Now()
				continue
			}

			chunkFixed := 0
			chunkSkipped := 0
			chunkFailed := 0

			issuesByObject := make(map[string][]AutoFixBatchIssue, len(chunk))
			objectOrder := make([]string, 0, len(chunk))
			kindByObject := make(map[string]string, len(chunk))
			for _, batchIssue := range chunk {
				if _, exists := issuesByObject[batchIssue.ObjectID]; !exists {
					objectOrder = append(objectOrder, batchIssue.ObjectID)
					kindByObject[batchIssue.ObjectID] = batchIssue.ObjectKind
				}
				issuesByObject[batchIssue.ObjectID] = append(issuesByObject[batchIssue.ObjectID], batchIssue)
			}

			for _, objectID := range objectOrder {
				objectIssues := issuesByObject[objectID]
				if len(objectIssues) == 0 {
					continue
				}

				first := objectIssues[0]
				filePath := first.FilePath
				if resolved, ok := resolveBatchIssueFilePath(storageProvider, first.ObjectID, first.ObjectKind, first.FilePath); ok {
					filePath = resolved
				}

				obj, readErr := chunkTx.Read(stdctx, secCtx, objectID)
				if readErr != nil {
					for range objectIssues {
						result.Processed++
						result.Progress.Processed = result.Processed
						result.Progress.LastUpdate = time.Now()
					}
					if errors.Is(readErr, storage.ErrObjectNotFound) {
						chunkSkipped += len(objectIssues)
						result.Skipped += len(objectIssues)
						result.Progress.Skipped = result.Skipped
						invalidateStaleCachesForMissingObject(objectIDCache, valCache, objectID)
						continue
					}
					chunkFailed += len(objectIssues)
					result.Failed += len(objectIssues)
					result.Errors = append(result.Errors, fmt.Sprintf("Failed to read object %s: %v", objectID, readErr))
					continue
				}

				kind := kindByObject[objectID]
				liveIssues, staleIssues := filterStaleAutoFixBatchIssues(kind, obj, objectIssues)
				for _, batchIssue := range staleIssues {
					result.Processed++
					chunkSkipped++
					result.Skipped++
					result.Progress.Processed = result.Processed
					result.Progress.Skipped = result.Skipped
					result.Progress.LastUpdate = time.Now()
					result.Trace = append(result.Trace, AutoFixIssueTrace{
						ObjectID:       batchIssue.ObjectID,
						ObjectKind:     batchIssue.ObjectKind,
						Category:       batchIssue.Issue.Category,
						Message:        batchIssue.Issue.Message,
						AutoFixable:    batchIssue.Issue.AutoFixable,
						Status:         autoFixTraceStatusSkippedNoChange,
						FixMessage:     "skipped: snapshotted issue no longer applies to live object",
						Persisted:      false,
						ProcessedAtUTC: time.Now().UTC(),
					})
				}
				objectIssues = liveIssues
				if len(objectIssues) == 0 {
					continue
				}

				parsedObj := &parser.ParsedObject{
					ID:         objectID,
					Kind:       kind,
					Properties: obj,
				}
				registry := getHashRegistryForFile(stdctx, filePath, kind, abp.projectRoot, hashRegistryCache)
				fixCtx := initializeAutoFixContext(ctx, cmd, parsedObj, filePath, kind, hashRegistryCache, objectIDCache)
				specFixer := NewSpecBasedAutoFixer(ctx, abp.logger)
				originalProps := make(map[string]any, len(parsedObj.Properties))
				maps.Copy(originalProps, parsedObj.Properties)

				objectTraceIndexes := make([]int, 0, len(objectIssues))
				for _, batchIssue := range objectIssues {
					result.Processed++
					result.Progress.Processed = result.Processed
					result.Progress.LastUpdate = time.Now()

					fixMsg := processIssueForAutoFix(fixCtx, batchIssue.Issue, registry, specFixer, chunkTx)
					if fixMsg != emptyValue {
						chunkFixed++
						result.Fixed++
						result.Progress.Fixed = result.Fixed
						result.Trace = append(result.Trace, AutoFixIssueTrace{
							ObjectID:       batchIssue.ObjectID,
							ObjectKind:     batchIssue.ObjectKind,
							Category:       batchIssue.Issue.Category,
							Message:        batchIssue.Issue.Message,
							AutoFixable:    batchIssue.Issue.AutoFixable,
							Status:         autoFixTraceStatusFixedPrepared,
							FixMessage:     fixMsg,
							ProcessedAtUTC: time.Now().UTC(),
						})
						objectTraceIndexes = append(objectTraceIndexes, len(result.Trace)-1)
						logging.NewEvent("Fixed issue in batch").
							BatchID(batch.BatchID).
							String("object_id", batchIssue.ObjectID).
							String("message", fixMsg).
							Debug(abp.logger)
					} else {
						chunkSkipped++
						result.Skipped++
						result.Progress.Skipped = result.Skipped
						result.Trace = append(result.Trace, AutoFixIssueTrace{
							ObjectID:       batchIssue.ObjectID,
							ObjectKind:     batchIssue.ObjectKind,
							Category:       batchIssue.Issue.Category,
							Message:        batchIssue.Issue.Message,
							AutoFixable:    batchIssue.Issue.AutoFixable,
							Status:         autoFixTraceStatusSkippedNoChange,
							Persisted:      false,
							ProcessedAtUTC: time.Now().UTC(),
						})
					}
				}

				if !reflect.DeepEqual(originalProps, parsedObj.Properties) {
					persisted := applySpecFix(fixCtx, originalProps, parsedObj.Properties, chunkTx)
					for _, idx := range objectTraceIndexes {
						if idx < 0 || idx >= len(result.Trace) {
							continue
						}
						if persisted {
							result.Trace[idx].Status = autoFixTraceStatusFixedPersisted
							result.Trace[idx].Persistence = autoFixTracePersistenceCommitted
							result.Trace[idx].Persisted = true
						} else {
							result.Trace[idx].Status = autoFixTraceStatusFixedNotPersist
							result.Trace[idx].Persistence = autoFixTracePersistenceAborted
							result.Trace[idx].Persisted = false
						}
					}
				}
			}

			if commitErr := chunkTx.Commit(stdctx); commitErr != nil {
				_ = chunkTx.Rollback(stdctx)
				logging.NewEvent("Autofix chunk commit failed, chunk rolled back").
					BatchID(batch.BatchID).
					Int("chunk_start", chunkStart).
					Int("chunk_size", len(chunk)).
					ChunkFixed(chunkFixed).
					Error(abp.logger, commitErr)
				result.Fixed -= chunkFixed
				result.Progress.Fixed = result.Fixed
				result.Failed += chunkFixed
				result.Errors = append(result.Errors, fmt.Sprintf("Chunk commit failed (start=%d, size=%d): %v", chunkStart, len(chunk), commitErr))
				for i := range result.Trace {
					if result.Trace[i].Status == autoFixTraceStatusFixedPersisted {
						result.Trace[i].Status = autoFixTraceStatusChunkRolledBack
						result.Trace[i].Persisted = false
						result.Trace[i].Persistence = autoFixTracePersistenceRolledBack
						result.Trace[i].Error = commitErr.Error()
					}
				}
			} else if chunkSkipped > 0 || chunkFixed > 0 {
				msg := autofixChunkCommitLogMessage(chunkFixed, chunkSkipped)
				logging.NewEvent(msg).
					BatchID(batch.BatchID).
					ChunkFixed(chunkFixed).
					ChunkSkipped(chunkSkipped).
					ChunkFailed(chunkFailed).
					Info(abp.logger)
			}

			emitBatchProgressEvent(batchID, result.Processed, result.Fixed, result.Failed, len(issues))
		}
		return nil
	}

	_, _ = abp.pl.Run(pctx, work)
	return result
}

// autofixChunkCommitLogMessage returns the log message for a committed chunk so "skipped" is clearly distinguished from "fixed".
func autofixChunkCommitLogMessage(chunkFixed, chunkSkipped int) string {
	if chunkFixed == 0 && chunkSkipped > 0 {
		return "Autofix chunk committed (skipped = no fix applied: already fixed or not applicable)"
	}
	return "Autofix chunk committed"
}

// resolveBatchIssueFilePath returns a better file path for an object when the stored path is missing or stale.
// This uses storage's own resolution/index so we don't depend on stale cache entries in the batch file.
func resolveBatchIssueFilePath(storageProvider storage.ObjectStorageProvider, objectID, kind, storedPath string) (string, bool) {
	if storedPath != emptyValue {
		if _, err := fileutil.Stat(storedPath); err == nil {
			return storedPath, true
		}
	}
	if fileStorage, ok := storageProvider.(*storage.FileObjectStorage); ok {
		if p, err := fileStorage.GetFilePathForObject(objectID, kind); err == nil && p != emptyValue {
			return p, true
		}
	}
	return storedPath, false
}

func invalidateStaleCachesForMissingObject(objectIDCache *ObjectIDCache, valCache *validation.ValidationStateCache, objectID string) {
	if objectID == emptyValue {
		return
	}
	if objectIDCache != nil {
		objectIDCache.Invalidate(objectID)
	}
	if valCache != nil {
		valCache.Invalidate(objectID)
		_ = valCache.Save() // best effort persistence
	}
}

// normalizeAutoFixBatchIssues returns the flat list of issues to process.
// Supports both:
// - legacy batches with batch.Issues (one row per issue)
// - grouped batches with batch.Objects (one row per object with many issues)
func normalizeAutoFixBatchIssues(batch *AutoFixBatch) []AutoFixBatchIssue {
	if batch == nil {
		return nil
	}
	if len(batch.Issues) > 0 {
		return batch.Issues
	}
	if len(batch.Objects) == 0 {
		return nil
	}
	out := make([]AutoFixBatchIssue, 0, 64)
	for _, obj := range batch.Objects {
		for _, issue := range obj.Issues {
			out = append(out, AutoFixBatchIssue{
				ObjectID:   obj.ObjectID,
				ObjectKind: obj.ObjectKind,
				FilePath:   obj.FilePath,
				Issue:      issue,
			})
		}
	}
	return out
}

// emitBatchStartEvent emits an event when a batch starts processing
func emitBatchStartEvent(batchID string, totalIssues int) {
	coordinator := coordination.GetCoordinator()
	if coordinator == nil {
		return
	}

	eventCtx := coordination.NewEventContext(batchID, autoFixMetricsOp, autoFixEventStart).
		WithEventData(&coordination.EventData{
			LoggingFields: []coordination.LoggingField{
				{Key: autoFixFieldEvent, Value: "auto_fix_batch_start"},
				{Key: autoFixFieldBatchID, Value: batchID},
				{Key: autoFixFieldTotalIssues, Value: totalIssues},
			},
			AuditMetadata: map[string]any{
				objects.FieldKeyEventType: autoFixEventTypeSystem,
				objects.FieldKeyOperation: fmt.Sprintf("Auto-fix batch %s started processing %d issues", batchID, totalIssues),
				objects.FieldKeySeverity:  autoFixSeverityLow,
			},
			MetricsData: map[string]any{
				objects.FieldKeyOperation: autoFixMetricsOp,
				objects.FieldKeyEventType: autoFixEventBatchStart,
				objects.FieldKeyBatchID:   batchID,
				autoFixFieldTotalIssues:   totalIssues,
			},
		}).
		WithChannels(true, true, true, true) // All channels

	_ = coordinator.Emit(pkgctx.NewSystemContext(), eventCtx) //nolint:errcheck // Async, best-effort
}

// emitBatchProgressEvent emits an event for batch progress updates
func emitBatchProgressEvent(batchID string, processed, fixed, failed, total int) {
	coordinator := coordination.GetCoordinator()
	if coordinator == nil {
		return
	}

	eventCtx := coordination.NewEventContext(batchID, autoFixMetricsOp, autoFixEventProgress).
		WithEventData(&coordination.EventData{
			LoggingFields: []coordination.LoggingField{
				{Key: autoFixFieldEvent, Value: "auto_fix_batch_progress"},
				{Key: autoFixFieldBatchID, Value: batchID},
				{Key: objects.FieldKeyProcessed, Value: processed},
				{Key: objects.FieldKeyFixed, Value: fixed},
				{Key: objects.FieldKeyFailed, Value: failed},
				{Key: autoFixFieldTotal, Value: total},
			},
			MetricsData: map[string]any{
				objects.FieldKeyOperation: autoFixMetricsOp,
				objects.FieldKeyEventType: autoFixEventBatchProgress,
				objects.FieldKeyBatchID:   batchID,
				objects.FieldKeyProcessed: processed,
				objects.FieldKeyFixed:     fixed,
				objects.FieldKeyFailed:    failed,
				autoFixFieldTotal:         total,
			},
		}).
		WithChannels(true, false, true, true) // Logging, metrics, operational (no audit for progress)

	_ = coordinator.Emit(pkgctx.NewSystemContext(), eventCtx) //nolint:errcheck // Async, best-effort
}

// emitBatchCompleteEvent emits an event when a batch completes processing
func emitBatchCompleteEvent(batchID string, result *ProcessBatchResult, duration time.Duration) {
	coordinator := coordination.GetCoordinator()
	if coordinator == nil {
		return
	}

	status := autoFixEventComplete
	if result.Failed > 0 {
		status = autoFixEventCompleteErr
	}

	eventCtx := coordination.NewEventContext(batchID, autoFixMetricsOp, status).
		WithEventData(&coordination.EventData{
			LoggingFields: []coordination.LoggingField{
				{Key: autoFixFieldEvent, Value: "auto_fix_batch_complete"},
				{Key: autoFixFieldBatchID, Value: batchID},
				{Key: objects.FieldKeyProcessed, Value: result.Processed},
				{Key: objects.FieldKeyFixed, Value: result.Fixed},
				{Key: objects.FieldKeyFailed, Value: result.Failed},
				{Key: objects.FieldKeySkipped, Value: result.Skipped},
				{Key: objects.FieldKeyDurationSeconds, Value: duration.Seconds()},
			},
			AuditMetadata: map[string]any{
				objects.FieldKeyEventType: autoFixEventTypeSystem,
				objects.FieldKeyOperation: fmt.Sprintf("Auto-fix batch %s completed: %d processed, %d fixed, %d failed, %d skipped", batchID, result.Processed, result.Fixed, result.Failed, result.Skipped),
				objects.FieldKeySeverity:  autoFixSeverityLow,
			},
			MetricsData: map[string]any{
				objects.FieldKeyOperation:       autoFixMetricsOp,
				objects.FieldKeyEventType:       autoFixEventBatchComplete,
				objects.FieldKeyBatchID:         batchID,
				objects.FieldKeyProcessed:       result.Processed,
				objects.FieldKeyFixed:           result.Fixed,
				objects.FieldKeyFailed:          result.Failed,
				objects.FieldKeySkipped:         result.Skipped,
				objects.FieldKeyDurationSeconds: duration.Seconds(),
			},
		}).
		WithDuration(duration).
		WithChannels(true, true, true, true) // All channels

	_ = coordinator.Emit(pkgctx.NewSystemContext(), eventCtx) //nolint:errcheck // Async, best-effort
}

// createBatchMetrics creates base_metric objects to capture batch processing statistics
func createBatchMetrics(ctx stdcontext.Context, storageProvider storage.ObjectStorageProvider, batchID string, result *ProcessBatchResult, duration time.Duration) {
	// Fire metric capture in background (async per POL-OBS-001)
	goroutinelabels.NewGoroutine("auto_fix_batch_metrics", fmt.Sprintf("creating metrics for batch %s", batchID)).
		StartSimple(func() {
			now := time.Now().UTC()

			// Build tags
			tags := []string{
				autoFixTagSystem,
				autoFixTagAutoFix,
				autoFixTagBatchProcessing,
			}
			if result.Failed > 0 {
				tags = append(tags, autoFixTagHasErrors)
			}

			// Create base_metric object
			metricObj := map[string]any{
				objects.FieldKeyKind:            objects.KindBaseMetric,
				objects.FieldKeyTitle:           fmt.Sprintf("Auto-fix batch %s: %d processed, %d fixed", batchID, result.Processed, result.Fixed),
				objects.FieldKeyMetricType:      autoFixMetricTypeSystem,
				objects.FieldKeySource:          autoFixMetricSource,
				objects.FieldKeyTags:            tags,
				objects.FieldKeyCollectionCount: 1,
				objects.FieldKeyFirstSeen:       zqktime.FormatLayoutUTC(now, zqktime.LayoutObjectDateTimeZ),
				objects.FieldKeyLastSeen:        zqktime.FormatLayoutUTC(now, zqktime.LayoutObjectDateTimeZ),
				objects.FieldKeyCreatedAt:       zqktime.FormatLayoutUTC(now, zqktime.LayoutObjectDateTimeZ),
				objects.FieldKeyCreatedBy:       autoFixCreatedBySystem,
				objects.FieldKeyStatus:          autoFixMetricStatusImplemented,
				objects.FieldKeyNamespaceID:     validation.DefaultNamespaceKernel,
				objects.FieldKeyOriginProject:   validation.DefaultOriginProject,
				objects.FieldKeyOriginSystem:    validation.DefaultOriginSystem,
				objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,

				// Batch processing specific fields
				objects.FieldKeyBatchID:         batchID,
				objects.FieldKeyProcessed:       result.Processed,
				objects.FieldKeyFixed:           result.Fixed,
				objects.FieldKeyFailed:          result.Failed,
				objects.FieldKeySkipped:         result.Skipped,
				objects.FieldKeyDurationSeconds: duration.Seconds(),
				objects.FieldKeySuccessRate:     float64(result.Fixed) / float64(result.Processed) * 100.0,
			}

			secCtx := pkgctx.NewSystemSecurityContext()
			if err := storageProvider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, metricObj); err != nil {
				// Log error but don't fail - metrics are non-critical
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Debug("Failed to create batch metric (non-critical)").
					BatchID(batchID).
					WithError(err).
					Log()
			}
		})
}

// createBatchAuditEvent creates an audit event capturing the results of the batch run.
// storageProvider may be non-nil (shared from caller) to avoid a redundant factory init;
// when nil the goroutine creates its own factory (standalone / fallback path).
func createBatchAuditEvent(ctx stdcontext.Context, projectRoot, batchID string, result *ProcessBatchResult, duration time.Duration, storageProvider storage.ObjectStorageProvider) {
	// Fire audit event creation in background (async per POL-OBS-001)
	goroutinelabels.NewGoroutine("auto_fix_batch_audit", fmt.Sprintf("creating audit event for batch %s", batchID)).
		StartSimple(func() {
			sp := storageProvider
			if sp == nil {
				// Fallback: standalone caller (e.g. runAutoFixBatch without shared provider).
				sf, sfErr := storage.NewStorageFactory(ctx, projectRoot)
				if sfErr != nil {
					return // Best effort
				}
				sp = sf.GetStorage()
				if sp == nil {
					return // Best effort
				}
			}
			storageProvider := sp

			operation := fmt.Sprintf("Auto-fix batch %s completed: %d processed, %d fixed, %d failed, %d skipped in %.2f seconds",
				batchID, result.Processed, result.Fixed, result.Failed, result.Skipped, duration.Seconds())

			severity := autoFixSeverityLow
			if result.Failed > 0 {
				severity = autoFixSeverityMedium
			}

			metadata := map[string]any{
				objects.FieldKeyBatchID:         batchID,
				objects.FieldKeyProcessed:       result.Processed,
				objects.FieldKeyFixed:           result.Fixed,
				objects.FieldKeyFailed:          result.Failed,
				objects.FieldKeySkipped:         result.Skipped,
				objects.FieldKeyDurationSeconds: duration.Seconds(),
				objects.FieldKeySuccessRate:     float64(result.Fixed) / float64(result.Processed) * 100.0,
			}

			if len(result.Errors) > 0 {
				metadata[autofixAuditMetadataKeyErrorCount] = len(result.Errors)
				// Include first few errors in metadata
				maxErrors := 5
				if len(result.Errors) < maxErrors {
					maxErrors = len(result.Errors)
				}
				metadata[autofixAuditMetadataKeySampleErrors] = result.Errors[:maxErrors]
			}

			secCtx := pkgctx.NewSystemSecurityContext()
			if err := storage.CreateAuditEventWithBuilder(pkgctx.WithPromoteOnCreate(ctx), projectRoot, secCtx, storageProvider, &storage.AuditEventOptions{
				EventType:  autoFixEventTypeSystem,
				Operation:  operation,
				Severity:   severity,
				TargetKind: autoFixBatchTargetKind,
				TargetID:   batchID,
				Metadata:   metadata,
			}); err != nil {
				// Log error but don't fail - audit events are best effort, async
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Debug("Failed to create batch audit event (non-critical)").
					BatchID(batchID).
					WithError(err).
					Log()
			}
		})
}

// renameBatchFileAfterProcessing renames a batch file to indicate processing status
// FIXED_ prefix for successful batches, PROCESSED_ for batches with errors
func renameBatchFileAfterProcessing(batchFile string, isSuccess bool) error {
	// Skip if file doesn't exist or already renamed
	if _, err := fileutil.Stat(batchFile); fileutil.IsNotExist(err) {
		return nil // File already gone, nothing to rename
	}

	dir := filepath.Dir(batchFile)
	baseName := filepath.Base(batchFile)

	// Skip if already renamed
	if strings.HasPrefix(baseName, autoFixBatchFixedPrefix) || strings.HasPrefix(baseName, autoFixBatchDonePrefix) {
		return nil // Already renamed
	}

	// Determine prefix based on success
	prefix := autoFixBatchDonePrefix
	if isSuccess {
		prefix = autoFixBatchFixedPrefix
	}

	// Create new filename
	newName := prefix + baseName
	newPath := filepath.Join(dir, newName)

	// Rename the file
	if err := fileutil.Rename(batchFile, newPath); err != nil {
		return errfmt.Newf("failed to rename batch file").Wrap(err)
	}

	return nil
}

func getAutofixCLIContext(cmd *cobra.Command, projectRoot string) (*cli.Context, error) {
	initCtx := &pkgctx.CliInitializationContext{ProjectRoot: projectRoot}
	ctx, err := cli.GetContextFromCommand(cmd, initCtx)
	if err != nil {
		return nil, errfmt.Newf("failed to get context").Wrap(err)
	}
	return ctx, nil
}
