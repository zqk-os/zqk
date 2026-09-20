package system

import (
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
	"github.com/zqk-os/zqk/pkg/metricsrecording"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/when"
)

const pipelineKindSystemCheck = "system_check_async"

// noopMetricsSink avoids per-stage Info logs for a high-frequency CLI path.
type noopMetricsSinkSystemCheck struct{}

func (noopMetricsSinkSystemCheck) RecordStage(ctx stdcontext.Context, kind, stage string, duration time.Duration, err error) {
	// Intentionally no-op. Stage outcomes still live in pipeline.Context.Outcome.
}

func (noopMetricsSinkSystemCheck) RecordStageWithBuckets(ctx stdcontext.Context, kind, stage string, duration time.Duration, err error, _ map[string]string) {
}

type systemCheckPipelinePayload struct {
	cmd         *cobra.Command
	ctx         *cli.Context
	args        []string
	operationID string
	cancelCtx   stdcontext.Context

	checkCtx     *AsyncCheckContext
	outputQueue  *validation.OutputQueue
	outputWriter *validation.OutputWriter
	cleanups     []func()
}

func (p *systemCheckPipelinePayload) addCleanup(f func()) {
	p.cleanups = append(p.cleanups, f)
}

func (p *systemCheckPipelinePayload) cleanup() {
	// Run cleanups in LIFO order
	for i := len(p.cleanups) - 1; i >= 0; i-- {
		p.cleanups[i]()
	}
}

//nolint:gocyclo
func runSystemCheckPipelineWithOutcome(
	cmd *cobra.Command,
	ctx *cli.Context,
	args []string,
	operationID string,
	cancelCtx stdcontext.Context,
) (map[string]any, error) {
	// Do not persist sampled metric objects during check. grprof_traces.txt:
	// hundreds of coordinator_metrics_router Gs blocked on FileObjectStorage.Create.
	metricsrecording.EnterHotPathNoPersist()
	defer metricsrecording.LeaveHotPathNoPersist()

	outcome := make(map[string]any)

	// Resolve logger early
	loggerProfile := string(pkgctx.ProfileSystem)
	if ctx != nil && ctx.Profile != emptyValue {
		loggerProfile = ctx.Profile
	}
	logger := logging.GetLoggerFromProfile(loggerProfile)

	pctx := &pipeline.Context{
		Ctx:     pkgctx.NewSystemContext(),
		Outcome: outcome,
	}

	payload := &systemCheckPipelinePayload{
		cmd:         cmd,
		ctx:         ctx,
		args:        args,
		operationID: operationID,
		cancelCtx:   cancelCtx,
	}
	// Defer cleanup of pipeline resources (profiles, metrics, queues, etc.)
	defer payload.cleanup()

	pl := pipeline.NewBuilder(pipelineKindSystemCheck, logger).
		WithProfile(string(pkgctx.ProfileSystem)).
		WithMetricsConfig(&pipeline.MetricsConfig{Sink: noopMetricsSinkSystemCheck{}, Strategy: pipeline.NoopBucketing{}}).
		AddStage("INGEST", func(stageCtx *pipeline.Context, _ any) (any, error) {
			stageCtx.Outcome[pipeline.OutcomeKeyIngestStageStarted] = true

			if payload.cmd == nil {
				stageCtx.Outcome[pipeline.OutcomeKeyIngestError] = "nil cmd"
				return nil, errfmt.Errorf("system check: cmd required")
			}
			if payload.ctx == nil {
				stageCtx.Outcome[pipeline.OutcomeKeyIngestError] = "nil ctx"
				return nil, errfmt.Errorf("system check: ctx required")
			}

			cli.TouchMeaningfulActivity() // idle watchdog: async check entered

			checkCtx, err := initializeAsyncCheckContext(payload.cmd, payload.ctx)
			if err != nil {
				return nil, err
			}

			when.When(func() bool { return payload.operationID != emptyValue }).Then(func() {
				checkCtx.OperationID = payload.operationID
			}).OrElse(func() {
				checkCtx.OperationID = fmt.Sprintf("check_%d", time.Now().UnixNano())
			}).Run()

			cli.TouchMeaningfulActivity()
			if checkCtx.OperationID != emptyValue && checkCtx.ProjectRoot != emptyValue {
				emitObjectIDCacheProgressViaCoordinator(checkCtx.Cmd.Context(), checkCtx.OperationID, progressStageLoading, "Preparing loaders and cache...")
			}

			if err := handleClearCache(checkCtx); err != nil {
				return nil, err
			}

			payload.checkCtx = checkCtx

			stageCtx.Outcome[objects.FieldKeyOperationID] = checkCtx.OperationID
			stageCtx.Outcome[pipeline.OutcomeKeyArgsCount] = len(payload.args)
			return payload, nil
		}).
		AddStage("CACHE_LOAD_BUILD", func(stageCtx *pipeline.Context, p any) (any, error) {
			stageCtx.Outcome[pipeline.OutcomeKeyCacheLoadBuildStageStarted] = true
			plLoad, ok := nildecode.DecodeNonNilPayload[*systemCheckPipelinePayload](p)
			if !ok {
				return nil, errfmt.Errorf("CACHE_LOAD_BUILD expected *systemCheckPipelinePayload")
			}

			checkCtx := plLoad.checkCtx
			if autoFix, _ := plLoad.cmd.Flags().GetBool("auto-fix"); autoFix && checkCtx.ProjectRoot != emptyValue {
				cli.TouchMeaningfulActivity()
				const proactiveCleanupTimeout = 2 * time.Minute
				if checkCtx.OperationID != emptyValue && checkCtx.ProjectRoot != emptyValue {
					emitObjectIDCacheProgressViaCoordinator(checkCtx.Cmd.Context(), checkCtx.OperationID, progressStageLoading, "Proactive Stale CAS cleanup starting...")
				}
				processDir, err := resolveProcessDirForAsyncCheck(checkCtx)
				if err != nil {
					return nil, err
				}
				objectIDCache := GetGlobalObjectIDCache()
				var allKinds []string
				if objectIDCache.IsPopulatedForProject(checkCtx.ProjectRoot) {
					allKinds = objectIDCache.GetKinds()
					logging.Fluent(checkCtx.Logger).Debug("Proactive Stale CAS cleanup: using object ID cache for kinds").KindsCount(len(allKinds)).Log()
				}
				if len(allKinds) == 0 {
					logging.Fluent(checkCtx.Logger).Debug("Proactive Stale CAS cleanup: discovering object kinds...").Log()
					allKinds = discoverObjectKindsWithTimeout(plLoad.cancelCtx, processDir, 30*time.Second, checkCtx.Logger)
				}
				if allKinds == nil {
					allKinds = []string{}
				}
				if len(allKinds) > 0 {
					cli.TouchMeaningfulActivity()
					if checkCtx.OperationID != emptyValue {
						emitObjectIDCacheProgressViaCoordinator(checkCtx.Cmd.Context(), checkCtx.OperationID, progressStageLoading,
							fmt.Sprintf("Proactive Stale CAS cleanup: cleaning %d kinds (timeout %s)...", len(allKinds), proactiveCleanupTimeout))
					}
					done := make(chan struct{})
					var total int
					var errs []error
					var onKindProgress KindProgressFunc
					if checkCtx.OperationID != emptyValue {
						onKindProgress = func(kind string, completedIndex, totalKinds int) {
							cli.TouchMeaningfulActivity()
							emitObjectIDCacheProgressViaCoordinator(checkCtx.Cmd.Context(), checkCtx.OperationID, progressStageLoading,
								fmt.Sprintf("Proactive Stale CAS cleanup: %s (%d/%d)...", kind, completedIndex, totalKinds))
						}
					} else {
						onKindProgress = func(kind string, completedIndex, totalKinds int) {
							cli.TouchMeaningfulActivity()
						}
					}
					cleanupBuilder := goroutinelabels.NewGoroutine("async_check_proactive_cleanup", "RunHashDuplicatesCleanupForKinds with timeout").
						AsCleanup()
					cleanupBuilder.StartSimple(func() {
						defer close(done)
						deleteForKinds := storage.GetGlobalBlockingCheckConfig().GetBypassKinds()
						total, errs = RunHashDuplicatesCleanupForKinds(checkCtx.ProjectRoot, allKinds, false, false, checkCtx.Logger, onKindProgress, deleteForKinds)
					})
					timeoutCtx, stopTimeout := stdcontext.WithTimeout(stdcontext.Background(), proactiveCleanupTimeout) // Background: request-or-shutdown derived
					defer stopTimeout()
					idleTicker := time.NewTicker(5 * time.Second)
					defer idleTicker.Stop()
					cleanupDone := false
					for !cleanupDone {
						select {
						case <-done:
							cleanupDone = true
							if total > 0 {
								logging.Fluent(checkCtx.Logger).Info("Proactive Stale CAS cleanup completed").
									KindsCount(len(allKinds)).Int("files_quarantined", total).Log()
							}
							for _, e := range errs {
								if e != nil {
									logging.Fluent(checkCtx.Logger).Warn("Stale CAS cleanup reported error").WithError(e).Log()
								}
							}
						case <-timeoutCtx.Done():
							cleanupDone = true
							logging.Fluent(checkCtx.Logger).Warn("Proactive Stale CAS cleanup timed out - continuing with check").Log()
						case <-plLoad.cancelCtx.Done():
							cleanupDone = true
							logging.Fluent(checkCtx.Logger).Warn("Proactive Stale CAS cleanup cancelled - continuing with check").Log()
						case <-idleTicker.C:
							cli.TouchMeaningfulActivity()
						}
					}
				}
			}

			stopCPUProfile, err := setupCPUProfiling(checkCtx)
			if err != nil {
				return nil, err
			}
			plLoad.addCleanup(stopCPUProfile)

			stopGoroutineProfile, err := setupGoroutineProfiling(checkCtx)
			if err != nil {
				return nil, err
			}
			plLoad.addCleanup(stopGoroutineProfile)
			plLoad.addCleanup(func() { setupMetricsAndCleanup(checkCtx) })

			if err := initializeAsyncValidator(checkCtx); err != nil {
				return nil, err
			}

			initializeLoggingAndValidators(checkCtx)

			if checkCtx.OperationID != emptyValue && checkCtx.ProjectRoot != emptyValue {
				emitObjectIDCacheProgressViaCoordinator(checkCtx.Cmd.Context(), checkCtx.OperationID, progressStageLoading, "Loaders starting...")
			}

			var storageProvider storage.ObjectStorageProvider
			var specReadyErr, lifecycleReadyErr, idPatternsErr error
			setupCtx := checkCtx.Cmd.Context()
			if setupCtx == nil {
				setupCtx = stdcontext.Background() // Background: request-or-shutdown derived
			}
			setupTimeout := 15 * time.Second
			loaderDone := make(chan struct{}, 4)
			setupDone := make(chan struct{})
			var setupDoneOnce sync.Once
			closeSetupDone := func() { setupDoneOnce.Do(func() { close(setupDone) }) }

			setupBud := goroutinelabels.DefaultBudget()
			specBuilder := goroutinelabels.NewGoroutine("async_check_spec_loader_ready", "spec_loader EnsureReady").
				WithPanicHandler(func(r any) {
					logging.Fluent(checkCtx.Logger).Warn(msgSetupLoaderPanic).LoaderPhase("spec_loader").PanicSummary(fmt.Sprint(r)).Log()
					loaderDone <- struct{}{}
				})
			if setupBud != nil {
				specBuilder = specBuilder.WithBudget(setupBud)
			}
			specBuilder.StartSimple(func() {
				specReadyErr = objects.GetGlobalSpecLoader().EnsureReady(setupCtx)
				loaderDone <- struct{}{}
			})

			lifecycleBuilder := goroutinelabels.NewGoroutine("async_check_lifecycle_loader_ready", "lifecycle EnsureReady").
				WithPanicHandler(func(r any) {
					logging.Fluent(checkCtx.Logger).Warn(msgSetupLoaderPanic).LoaderPhase("lifecycle_loader").PanicSummary(fmt.Sprint(r)).Log()
					loaderDone <- struct{}{}
				})
			if setupBud != nil {
				lifecycleBuilder = lifecycleBuilder.WithBudget(setupBud)
			}
			lifecycleBuilder.StartSimple(func() {
				lifecycleReadyErr = objects.GetGlobalLifecycleLoader().EnsureReady(setupCtx)
				loaderDone <- struct{}{}
			})

			idPatternsBuilder := goroutinelabels.NewGoroutine("async_check_id_patterns_ready", "id_patterns LoadPatterns").
				WithPanicHandler(func(r any) {
					logging.Fluent(checkCtx.Logger).Warn(msgSetupLoaderPanic).LoaderPhase("id_patterns").PanicSummary(fmt.Sprint(r)).Log()
					loaderDone <- struct{}{}
				})
			if setupBud != nil {
				idPatternsBuilder = idPatternsBuilder.WithBudget(setupBud)
			}
			idPatternsBuilder.StartSimple(func() {
				idPatternsErr = validation.GetIDValidator().LoadPatterns()
				loaderDone <- struct{}{}
			})

			storageBuilder := goroutinelabels.NewGoroutine("async_check_storage_provider", "creating storage provider").
				WithPanicHandler(func(r any) {
					logging.Fluent(checkCtx.Logger).Warn(msgSetupLoaderPanic).LoaderPhase("storage_provider").PanicSummary(fmt.Sprint(r)).Log()
					loaderDone <- struct{}{}
				})
			if setupBud != nil {
				storageBuilder = storageBuilder.WithBudget(setupBud)
			}
			storageBuilder.StartSimple(func() {
				storageProvider = getStorageProviderForCache(checkCtx.ProjectRoot)
				loaderDone <- struct{}{}
			})

			watchdogBuilder := goroutinelabels.NewGoroutine("async_check_setup_watchdog", "collecting loader signals or deadline then closing setupDone").
				WithPanicHandler(func(r any) {
					logging.Fluent(checkCtx.Logger).Warn("Setup watchdog panic (closing setupDone)").PanicSummary(fmt.Sprint(r)).Log()
				}).
				WithCleanup(closeSetupDone)
			if setupBud != nil {
				watchdogBuilder = watchdogBuilder.WithBudget(setupBud)
			}
			watchdogBuilder.StartSimple(func() {
				deadline := time.Now().Add(setupTimeout)
			watchdog:
				for i := 0; i < 4; i++ {
					remaining := time.Until(deadline)
					if remaining <= 0 {
						break
					}
					select {
					case <-loaderDone:
					case <-time.After(remaining):
						break watchdog
					}
				}
			})

			if plLoad.cancelCtx != nil {
				select {
				case <-setupDone:
					if checkCtx.OperationID != emptyValue && checkCtx.ProjectRoot != emptyValue {
						emitObjectIDCacheProgressViaCoordinator(checkCtx.Cmd.Context(), checkCtx.OperationID, progressStageLoading, msgLoadersReady)
					}
				case <-time.After(setupTimeout):
					logging.Fluent(checkCtx.Logger).Warn("Setup phase timed out - continuing with partial loaders").Timeout(setupTimeout.String()).Log()
					if checkCtx.OperationID != emptyValue && checkCtx.ProjectRoot != emptyValue {
						emitObjectIDCacheProgressViaCoordinator(checkCtx.Cmd.Context(), checkCtx.OperationID, progressStageLoading, "Loaders timed out, continuing...")
					}
				case <-plLoad.cancelCtx.Done():
					return nil, plLoad.cancelCtx.Err()
				}
			} else {
				select {
				case <-setupDone:
					if checkCtx.OperationID != emptyValue && checkCtx.ProjectRoot != emptyValue {
						emitObjectIDCacheProgressViaCoordinator(checkCtx.Cmd.Context(), checkCtx.OperationID, progressStageLoading, msgLoadersReady)
					}
				case <-time.After(setupTimeout):
					logging.Fluent(checkCtx.Logger).Warn("Setup phase timed out - continuing with partial loaders").Timeout(setupTimeout.String()).Log()
					if checkCtx.OperationID != emptyValue && checkCtx.ProjectRoot != emptyValue {
						emitObjectIDCacheProgressViaCoordinator(checkCtx.Cmd.Context(), checkCtx.OperationID, progressStageLoading, "Loaders timed out, continuing...")
					}
				}
			}

			if specReadyErr != nil {
				logging.Fluent(checkCtx.Logger).Debug("Spec loader EnsureReady failed").WithError(specReadyErr).Log()
			}
			if lifecycleReadyErr != nil {
				logging.Fluent(checkCtx.Logger).Warn("Lifecycle loader EnsureReady failed (startup YAML parse or warmup)").WithError(lifecycleReadyErr).Log()
			}
			if idPatternsErr != nil {
				logging.Fluent(checkCtx.Logger).Debug("ID patterns load failed").WithError(idPatternsErr).Log()
			}

			checkCtx.StorageProvider = storageProvider
			if storageProvider != nil {
				plLoad.addCleanup(func() {
					_ = storageProvider.Shutdown(stdcontext.Background()) // Background: request-or-shutdown derived
				})
			}
			return plLoad, nil
		}).
		AddStage("WARM_CAS", func(stageCtx *pipeline.Context, p any) (any, error) {
			stageCtx.Outcome[pipeline.OutcomeKeyWarmCasStageStarted] = true
			plLoad := p.(*systemCheckPipelinePayload)

			if plLoad.checkCtx.OperationID != emptyValue && plLoad.checkCtx.ProjectRoot != emptyValue {
				emitObjectIDCacheProgressViaCoordinator(plLoad.checkCtx.Cmd.Context(), plLoad.checkCtx.OperationID, progressStageLoading, "Loading object ID cache...")
			}

			setupAsyncValidationFunction(plLoad.checkCtx.AsyncValidator, plLoad.cmd, plLoad.ctx, plLoad.checkCtx.ProjectRoot, plLoad.checkCtx.Metrics, plLoad.checkCtx.OperationID, plLoad.checkCtx, plLoad.cancelCtx)
			return plLoad, nil
		}).
		AddStage("DISCOVERY_ENQUEUE", func(stageCtx *pipeline.Context, p any) (any, error) {
			stageCtx.Outcome[pipeline.OutcomeKeyDiscoveryEnqueueStageStarted] = true
			plLoad := p.(*systemCheckPipelinePayload)

			if plLoad.checkCtx.OperationID != emptyValue && plLoad.checkCtx.ProjectRoot != emptyValue {
				emitObjectIDCacheProgressViaCoordinator(plLoad.checkCtx.Cmd.Context(), plLoad.checkCtx.OperationID, progressStageLoading, "Cache ready, starting validator...")
			}
			if err := startAsyncValidator(plLoad.checkCtx); err != nil {
				return nil, err
			}

			plLoad.addCleanup(func() {
				if plLoad.checkCtx.ValidatorStarted {
					if stopErr := plLoad.checkCtx.AsyncValidator.Stop(); stopErr != nil {
						logging.Fluent(plLoad.checkCtx.Logger).Warn("Failed to stop async validator during cleanup").WithError(stopErr).Log()
					}
				}
				if plLoad.checkCtx.StorageProvider != nil {
					_ = plLoad.checkCtx.StorageProvider.Shutdown(stdcontext.Background()) // Background: request-or-shutdown derived
				}
			})

			if err := determineCheckTarget(plLoad.checkCtx, plLoad.args); err != nil {
				return nil, err
			}

			outputQueue, outputWriter, outputCancel := createOutputQueueAndWriter(plLoad.checkCtx)
			plLoad.outputQueue = outputQueue
			plLoad.outputWriter = outputWriter
			plLoad.addCleanup(outputCancel)

			if err := registerOutputHandlers(plLoad.checkCtx, outputWriter); err != nil {
				return nil, err
			}

			outputPath := cli.GetOutputPath(plLoad.cmd)
			if outputPath == emptyValue {
				stdoutHandler := validation.NewWriterOutputHandler(cli.CommandOutputWriter(plLoad.cmd, nil), false)
				outputWriter.RegisterHandler(outputChannelStdout, stdoutHandler)
			}

			outputWriter.Start()
			plLoad.addCleanup(func() { _ = outputWriter.Stop() })

			setupResultCallback(plLoad.checkCtx.AsyncValidator, plLoad.cmd, plLoad.ctx, outputQueue)

			if plLoad.checkCtx.OperationID != emptyValue && plLoad.checkCtx.ProjectRoot != emptyValue {
				emitObjectIDCacheProgressViaCoordinator(plLoad.checkCtx.Cmd.Context(), plLoad.checkCtx.OperationID, progressStageLoading, "Discovery starting...")
			}

			if err := discoverAndEnqueueObjects(plLoad.checkCtx); err != nil {
				return nil, err
			}

			return plLoad, nil
		}).
		AddStage("VALIDATION", func(stageCtx *pipeline.Context, p any) (any, error) {
			stageCtx.Outcome[pipeline.OutcomeKeyValidationStageStarted] = true
			plLoad := p.(*systemCheckPipelinePayload)

			err := showValidationProgressWithMetrics(
				plLoad.cmd, plLoad.ctx, plLoad.checkCtx.AsyncValidator, plLoad.checkCtx.TotalTasks,
				plLoad.checkCtx.Metrics, plLoad.checkCtx.EnqueuedObjectIDs, plLoad.checkCtx.EnqueuedObjectInfo,
				plLoad.checkCtx.CachedCheckResults, plLoad.outputQueue, plLoad.checkCtx.OperationID,
			)

			if err != nil {
				stageCtx.Outcome[pipeline.OutcomeKeyValidationError] = err.Error()
			} else {
				stageCtx.Outcome[pipeline.OutcomeKeyValidationSuccess] = true
			}
			return plLoad, err
		}).
		AddStage("CAP_PROBE", func(stageCtx *pipeline.Context, payload any) (any, error) {
			plLoad := payload.(*systemCheckPipelinePayload)
			if plLoad.checkCtx != nil && plLoad.checkCtx.ProjectRoot != emptyValue {
				journalPath := filepath.Join(plLoad.checkCtx.ProjectRoot, paths.ProjectDataDir, paths.LogsDir, "scheduler", objects.JobIDCapOrchestrator, objects.JobIDCapOrchestrator+".events.jsonl")
				stat, err := fileutil.Stat(journalPath)
				if err == nil {
					if time.Since(stat.ModTime()) > 4*time.Hour {
						return nil, errfmt.Errorf("CAP journal is stale: no updates in over 4 hours (last modified %s)", stat.ModTime().Format(time.RFC3339))
					}
				} else if !fileutil.IsNotExist(err) {
					return nil, errfmt.Errorf("failed to check CAP journal freshness: %w", err)
				}
			}
			return payload, nil
		}).
		AddStage("FINALIZE", func(stageCtx *pipeline.Context, payload any) (any, error) {
			stageCtx.Outcome[pipeline.OutcomeKeyFinalizeDone] = true
			plLoad := payload.(*systemCheckPipelinePayload)
			if plLoad.cmd != nil {
				fmt.Fprintln(plLoad.cmd.ErrOrStderr(), "Finalizing caches and storage queues...")
			}
			return payload, nil
		}).
		Build()

	_, err := pl.Run(pctx, struct{}{}) // Initial payload is struct{}{} since INGEST creates the real payload
	return outcome, err
}

// RunSystemCheckViaPipeline wraps the system check async orchestration in the standardized pipeline lifecycle.
func RunSystemCheckViaPipeline(
	cmd *cobra.Command,
	ctx *cli.Context,
	args []string,
	operationID string,
	cancelCtx stdcontext.Context,
) error {
	_, err := runSystemCheckPipelineWithOutcome(cmd, ctx, args, operationID, cancelCtx)
	return err
}
