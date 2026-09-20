package scheduler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/cmd/zqk/system"
	"github.com/zqk-os/zqk/internal/cli"
	clicontext "github.com/zqk-os/zqk/internal/cli/context"
	"github.com/zqk-os/zqk/pkg/config"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/diagnostics"
	"github.com/zqk-os/zqk/pkg/dispatch"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/functional"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/graph/rpcpool"
	"github.com/zqk-os/zqk/pkg/hivemind/providers"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/orchestration"
	"github.com/zqk-os/zqk/pkg/orchestration/service"
	"github.com/zqk-os/zqk/pkg/orchestration/ticker"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/pipeline"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/strutil"
	"github.com/zqk-os/zqk/pkg/transport"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/validation/qa"
	"github.com/zqk-os/zqk/pkg/zqkenv"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// schedulerDaemonProcessName is argv[0] for the scheduler daemon child process so it shows distinctly in ps/top (e.g. "zqk-scheduler" vs "zqk").
const schedulerDaemonProcessName = "zqk-scheduler"
const schedulerControlTimeout = 20 * time.Second
const (
	schedulerErrProjectRootNotFound    = "project root not found"
	schedulerErrBrandSettingsRequired  = "brand settings required: %w"
	schedulerErrAlreadyRunningFmt      = "scheduler daemon is already running (PID: %d). Use 'zqk scheduler stop' to stop it first"
	schedulerErrHostServiceOwnsRootFmt = "scheduler host service %s owns root %s; use 'zqk scheduler service start --root %s' instead of spawning an unmanaged daemon"
	schedulerErrOpenFmt                = "failed to open %s: %w"
	schedulerErrStopDaemonFmt          = "failed to stop scheduler daemon: %w"
	schedulerProfileSystem             = "system"
	schedulerStatusRunning             = "running"
	schedulerStatusNotRunning          = "not running"
	schedulerFieldProjectRoot          = "project_root"
	schedulerFieldProjectType          = "project_type"
	schedulerFieldJobsPaused           = "jobs_paused"
	schedulerFieldInProcess            = "in_process"
	schedulerFieldPID                  = "pid"
	schedulerFieldDir                  = "dir"
	schedulerGOOSWindows               = "windows"
	schedulerDiagnosticsDirName        = "diagnostics"
	schedulerEventsFileName            = "diagnostics.jsonl"
	schedulerArgScheduler              = "scheduler"
	schedulerArgStart                  = "start"
	schedulerArgForeground             = "--foreground"
	schedulerFlagForce                 = "force"
	schedulerFlagWait                  = "wait"
	schedulerFlagPreCommit             = "pre-commit"
	schedulerStatusHintLine            = "  Status: zqk scheduler status\n"
	schedulerStopForceMsgFmt           = "SIGKILL sent to daemon (PID %d).\n"
	schedulerStopSigtermMsgFmt         = "SIGTERM sent to daemon (PID %d). Daemon shutting down.\n"
	schedulerStopMessageFmt            = "SIGTERM sent to daemon (PID %d). Waiting up to %s for exit...\n"
)

// childIdleShutdownTimeout is how long the scheduler may sit idle (no job completed, no trigger processed) when the parent process is not zqk before we shut down. Prevents hanging when run from a script/IDE that expects the process to exit.
const childIdleShutdownTimeout = 10 * time.Second
const schedulerStorageInitTimeout = 30 * time.Second
const schedulerCoordinatorTimeout = 5 * time.Second

const pipelineKindRunSchedulerControlWithTimeout = "scheduler.run_scheduler_control_with_timeout"

// resolveSchedulerCLIProjectRoot returns the project root for scheduler start/stop and related control.
// When cli.Context has an explicit ProjectRoot (tests, programmatic callers), it takes precedence over
// ResolveProjectRootFromSettings(".") so package tests stay isolated from the process cwd and from a
// real daemon PID file in the developer's repo (fixes flaky cross-process tests when a daemon is running).
func resolveSchedulerCLIProjectRoot(ctx *cli.Context) string {
	if ctx != nil {
		if pr := strings.TrimSpace(ctx.ProjectRoot); pr != emptyValue {
			return pr
		}
	}
	projectRoot, _, err := clicontext.ResolveProjectRootFromSettings(".")
	if err != nil || projectRoot == emptyValue {
		projectRoot = cli.ResolveProjectRoot(".")
	}
	return projectRoot
}

// runSchedulerControlWithTimeout runs fn in a labeled goroutine and waits up to schedulerControlTimeout.
func runSchedulerControlWithTimeout(op string, fn func() error) error {
	return runSchedulerControlWithTimeoutDur(op, schedulerControlTimeout, fn)
}

// runSchedulerControlWithTimeoutDur runs fn in a labeled goroutine and waits up to timeout (minimum schedulerControlTimeout).
// fn does not take context (blocking control paths); wall-clock timeout is on the waiter only — see
// docs/architecture/concurrency-patterns-v1.0.md § "Wall-clock timeout multiplexing (blocking fn)".
func runSchedulerControlWithTimeoutDur(op string, timeout time.Duration, fn func() error) error {
	if timeout <= 0 {
		timeout = schedulerControlTimeout
	}
	type state struct {
		done chan error
		err  error
	}
	st := &state{}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	runCtx := pkgctx.NewSystemContext()

	pl := pipeline.NewBuilder(pipelineKindRunSchedulerControlWithTimeout, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage(pipeline.StageIngest, func(pctx *pipeline.Context, payload any) (any, error) {
			st.done = make(chan error, 1)
			builder := goroutinelabels.NewGoroutine("scheduler_control_operation", fmt.Sprintf("run %s with timeout", op))
			if bud := goroutinelabels.DefaultBudget(); bud != nil {
				builder = builder.WithBudget(bud)
			}
			builder.StartSimple(func() {
				st.done <- fn()
			})
			return st, nil
		}).
		AddStage(pipeline.StageCommit, func(pctx *pipeline.Context, payload any) (any, error) {
			select {
			case err := <-st.done:
				st.err = err
			case <-time.After(timeout):
				st.err = errfmt.Errorf("%s timed out after %s", op, timeout)
			}
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

// buildConfigAtShutdownFields loads current scheduler config and returns a map with config_at_shutdown for stop-event metrics. Returns non-nil map (may be empty if load fails).
func buildConfigAtShutdownFields(projectRoot string) map[string]any {
	cfg, err := schedulerpkg.LoadSchedulerConfig(projectRoot)
	if err != nil || cfg == nil {
		return map[string]any{}
	}
	return map[string]any{
		"config_at_shutdown": map[string]any{
			objects.FieldKeyEnabled:   cfg.Enabled,
			schedulerFieldProjectType: cfg.ProjectType,
			schedulerFieldJobsPaused:  cfg.JobsPaused,
		},
	}
}

// startScheduler starts the scheduler daemon
//
//nolint:gocyclo // Scheduler setup is inherently complex
func startScheduler(ctx *cli.Context, cmd *cobra.Command) error {
	scrubSchedulerDaemonProcessEnv()

	inTest := false
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "-test.") {
			inTest = true
			break
		}
	}
	if strings.HasSuffix(os.Args[0], ".test") {
		inTest = true
	}

	if !inTest && zqkenv.RawGraphEnabled().Get() == "" {
		_ = zqkenv.RawGraphEnabled().Set("true")
	}

	minimal, err := cmd.Flags().GetBool("minimal")
	if err != nil {
		return errfmt.Newf("failed to parse 'minimal' flag").Wrap(err)
	}

	// Project root: explicit ctx (tests) first, then settings / persisted use / cwd — see resolveSchedulerCLIProjectRoot.
	projectRoot := resolveSchedulerCLIProjectRoot(ctx)
	if projectRoot == emptyValue {
		return errors.New(schedulerErrProjectRootNotFound)
	}
	if abs, err := filepath.Abs(projectRoot); err != nil {
		return errfmt.Newf("failed to resolve absolute project root").Wrap(err)
	} else {
		projectRoot = abs
	}
	if _, err := clicontext.LoadBrandSettings(projectRoot); err != nil {
		return errfmt.Errorf(schedulerErrBrandSettingsRequired, err)
	}

	// Clear no-auto-restart so ensure-scheduler-running.sh (cron) may start the daemon again if it dies later
	if err := schedulerpkg.RemoveNoAutoRestartFile(projectRoot); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		schedulerpkg.SLog(logger).Warn("Failed to remove no-auto-restart file").WithError(err).Log()
	}

	// Register this process as the daemon immediately so "scheduler status" sees us before slow init
	// (GetOrCreate can take many seconds). We must check for another instance first, allowing our own PID.
	running, existingPID, err := schedulerpkg.IsSchedulerRunning(projectRoot)
	if err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		schedulerpkg.SLog(logger).Warn("Failed to check if scheduler is running").WithError(err).Log()
	}
	if running && existingPID != os.Getpid() {
		return errfmt.Errorf(schedulerErrAlreadyRunningFmt, existingPID)
	}
	if err := schedulerpkg.WritePIDFile(projectRoot); err != nil {
		pidLogger := logging.GetLoggerFromProfile(strutil.OrDefault(ctx.Profile, schedulerProfileSystem))
		schedulerpkg.SLog(pidLogger).Warn("Failed to write scheduler PID file; status/dump may not see this daemon").
			ProjectRoot(projectRoot).
			WithError(err).
			Log()
	}

	profile := strutil.OrDefault(ctx.Profile, schedulerProfileSystem)
	logger := logging.GetLoggerFromProfile(profile)
	slog := logger // Alias for consistency with pipeline instrumentation

	// Redirect stdout/stderr to daemon.stdio (combined) and route events to diagnostics.jsonl (see daemon_stdio.go)
	// Keep the default "file" destination so logs also appear in the usual log file (log-events-{profile}.json/.log).
	// Add scheduler_daemon so scheduler events are also written to diagnostics.jsonl.
	stdioResult := SetupDaemonStdioRedirect(projectRoot)
	if stdioResult.Cleanup != nil {
		// Plain text to the daemon stdout pipe so daemon.stdio is non-empty on startup/shutdown.
		// System profile loggers do not attach stdio (pkg/logging/context_logger.go); logger.Info only hits log files
		// and router destinations added below—not the redirected stdout used for tailing daemon.stdio.
		// Use a named io.Writer (out) so check-logging-compliance treats Fprintf(out,…) as compliant (POL-CODE-007 script).
		out := os.Stdout
		startedAt := zqktime.NowRFC3339UTC()
		if _, err := fmt.Fprintf(out, "[scheduler] daemon started at %s\n", startedAt); err != nil {
			schedulerpkg.SLog(logger).Warn("Failed to write start message to stdout").WithError(err).Log()
		}
		schedulerpkg.SLog(logger).Info("[scheduler] daemon started").
			String("at", startedAt).
			Log()
		defer func() {
			shutdownAt := zqktime.NowRFC3339UTC()
			if _, err := fmt.Fprintf(out, "[scheduler] daemon shutting down at %s\n", shutdownAt); err != nil {
				schedulerpkg.SLog(logger).Warn("Failed to write shutdown message to stdout").WithError(err).Log()
			}
			schedulerpkg.SLog(logger).Info("[scheduler] daemon shutting down").
				String("at", shutdownAt).
				Log()
			stdioResult.Cleanup()
		}()
		if stdioResult.CoordinatorWriter != nil {
			if router := logging.GetGlobalRouter(); router != nil {
				levelStr := strutil.OrDefault(config.GetLogLevel(projectRoot), "info")
				daemonLogLevel := logging.InfoLevel
				if lv, ok := logging.ParseLevel(levelStr); ok {
					daemonLogLevel = lv
				}
				router.AddDestination("scheduler_daemon", stdioResult.CoordinatorWriter, daemonLogLevel, logging.NewJSONFormatter(pkgctx.NewSystemContext()))
				// Note: Error-level logs are already included in scheduler_daemon destination (InfoLevel+ includes ErrorLevel)
				// No need for separate stderr destination - errors go to diagnostics.jsonl and don't need duplication in daemon.stdio
			}
		}
	}

	// Build path alias cache before storage starts so the write-behind worker can resolve stream
	// paths (audit_event, change_journal_entry, etc.) as soon as it applies WAL or new ops.
	// Otherwise apply fails with "path alias not in cache" until cache_prewarm (SCH-cache-prewarm) runs.
	storagepkg.BuildPathAliasCacheForProject(projectRoot)

	// Standardized initialization pipeline
	type initPayload struct {
		projectRoot     string
		storageProvider storagepkg.ObjectStorageProvider
	}
	pl := pipeline.NewBuilder("scheduler_init", logger).
		WithProfile(profile).
		AddStage(schedulerpkg.StageValidateDependencies, func(pctx *pipeline.Context, payload any) (any, error) {
			slog.Info("startScheduler: validating dependencies")
			return &initPayload{projectRoot: projectRoot}, nil
		}).
		AddStage(schedulerpkg.StageInitializeStorage, func(pctx *pipeline.Context, payload any) (any, error) {
			in := payload.(*initPayload)
			slog.Info("startScheduler: initializing storage...")
			storageCtx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), schedulerStorageInitTimeout)
			defer cancel()
			sp, err := storagepkg.GetGlobalStorageProviderCache().GetOrCreate(storageCtx, in.projectRoot)
			if err != nil {
				return nil, errfmt.Newf("failed to get storage provider").Wrap(err)
			}
			in.storageProvider = sp
			return in, nil
		}).
		AddStage(schedulerpkg.StageConfigureScheduler, func(pctx *pipeline.Context, payload any) (any, error) {
			in := payload.(*initPayload)
			slog.Info("startScheduler: storage provider loaded")

			// Initialize TSDBProvider for the project
			tsdbProvider := storagepkg.NewEmbeddedTSDBProvider(filepath.Join(in.projectRoot, paths.ProjectDataDir, paths.SchedulerDir, "tsdb"))
			if err := tsdbProvider.Initialize(pctx.Ctx); err != nil {
				schedulerpkg.SLog(logger).Warn("startScheduler: failed to initialize TSDBProvider").WithError(err).Log()
			} else {
				schedulerpkg.SetGlobalTSDBProvider(tsdbProvider)
				schedulerpkg.SLog(logger).Info("startScheduler: TSDB initialized").Log()
			}

			// proceed with scheduler startup using in.storageProvider
			return in, nil
		}).
		Build()

	out, err := pl.Run(&pipeline.Context{Ctx: context.Background(), Outcome: make(map[string]any)}, &initPayload{projectRoot: projectRoot}) // Background: request-or-shutdown derived
	if err != nil {
		return err
	}
	in := out.(*initPayload)
	storageProvider := in.storageProvider
	if warning := schedulerpkg.SchedulerCLIBinaryConfigWarning(projectRoot); warning != emptyValue {
		schedulerpkg.SLog(logger).Warn("Scheduler CLI binary configuration warning").
			ProjectRoot(projectRoot).
			String("warning", warning).
			Log()
	}

	// Check scheduler configuration
	config, configErr := loadSchedulerConfig(projectRoot)
	defaultConfig := &schedulerConfig{Enabled: true, ProjectType: "production", JobsPaused: false}
	functional.When(func() bool { return configErr != nil }).Then(func() {
		emitSchedulerWarningEventViaCoordinator(
			pkgctx.NewSystemContext(),
			projectRoot,
			storageProvider,
			"Failed to load scheduler config, proceeding with defaults",
			configErr,
			profile,
		)
		config = defaultConfig
	}).OrElseWhen(func() bool { return config == nil }).Then(func() {
		config = defaultConfig
	}).Run()

	// When jobs_paused is true, timer and immediate jobs do not run (maintenance, object-count-report, etc.).
	// Log prominently so it is obvious; user updates config when the time is right.
	if config.JobsPaused {
		logging.Fluent(logging.GetLoggerFromProfile(profile)).Error(
			"JOBS PAUSED (jobs_paused=true): Timer and immediate jobs will NOT run. Maintenance (aggregation, retention), object-count-report, and other scheduled jobs are paused. Only manually triggered jobs will run. To resume: zqk scheduler config --no-jobs-paused",
			errfmt.Errorf("jobs_paused=true in .zqk/scheduler/config.yaml")).
			Log()
	}

	if !config.Enabled {
		return errfmt.Errorf("scheduler is disabled for this project (see %s/%s/%s). "+
			"This is typically done to prevent overwhelming the system with events during testing. "+
			"To enable, set 'enabled: true' in the config file, or use a test-scenarios folder for isolated testing",
			paths.ProjectDataDir, paths.SchedulerDir, paths.SchedulerConfigFile)
	}

	// Reject if scheduler is already running in this process, or in another process (PID file has a different PID).
	// We already wrote our PID above, so requireSchedulerNotRunning would see "running" (us); check explicitly.
	status, errStatus := getSchedulerStatus(ctx)
	if errStatus != nil {
		logger := logging.GetLoggerFromProfile(profile)
		schedulerpkg.SLog(logger).Warn("Failed to get scheduler status during start check").WithError(errStatus).Log()
	}
	if status != nil {
		if status.InProcess {
			return errfmt.Errorf("scheduler is already running in this process")
		}
		if status.Running && status.ProcessID != os.Getpid() {
			return errfmt.Errorf(schedulerErrAlreadyRunningFmt, status.ProcessID)
		}
	}

	// Ensure we have storage (e.g. if initial GetOrCreate failed)
	if storageProvider == nil {
		storageCtx2, cancel2 := context.WithTimeout(pkgctx.NewSystemContext(), schedulerStorageInitTimeout)
		defer cancel2()
		var factoryErr error
		storageProvider, factoryErr = storagepkg.GetGlobalStorageProviderCache().GetOrCreate(storageCtx2, projectRoot)
		if factoryErr != nil {
			return errfmt.Newf("failed to get storage from cache").Wrap(factoryErr)
		}
	}

	if p, ok := storageProvider.(interface {
		GetPool() provider.ConnectionPool
	}); ok {
		if pool := p.GetPool(); pool != nil {
			sockPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerSubdir, "rpcpool.sock")
			if err := rpcpool.StartServer(sockPath, pool); err != nil {
				schedulerpkg.SLog(logger).Warn("Failed to start RPC pool proxy server").WithError(err).Log()
			} else {
				schedulerpkg.SLog(logger).Info("RPC pool proxy server started").Log()
			}
		}
	}

	factoryProjectRoot := projectRoot

	// Create spec and lifecycle loaders
	specLoader := objects.GetGlobalSpecLoader()
	lifecycleLoader := objects.GetGlobalLifecycleLoader()

	// Instance context: one-stop-shop for storage and registries (same as MCP serve).
	// One-shot CLI leaves instance context nil; scheduler and MCP set it when they start and clear on exit.
	if storageProvider != nil {
		if cli.OnStorageCreated != nil {
			cli.OnStorageCreated(storageProvider)
		}
		cli.RegisterStorageForProjectRoot(factoryProjectRoot, storageProvider)
		ic := &dispatch.InstanceContext{
			Storage:     storageProvider,
			ProjectRoot: factoryProjectRoot,
			Profile:     profile,
		}
		ic.SetRegistry(dispatch.KeySpecLoader, specLoader)
		ic.SetRegistry(dispatch.KeyLifecycleLoader, lifecycleLoader)
		ic.SetRegistry(dispatch.KeySynonymResolver, objects.GetGlobalSynonymResolver())
		ic.SetRegistry(dispatch.KeyKindMapper, objects.GetGlobalKindMapper())
		ic.SetRegistry(dispatch.KeyValidatorRegistry, validation.GetGlobalRegistry())
		ic.SetRegistry(dispatch.KeyFieldRegistry, objects.GetGlobalFieldRegistry())
		dispatch.SetInstanceContext(ic)
		defer dispatch.ClearInstanceContext()
	}

	// Create scheduler with project root (needed for graph backends).
	// Pass object ID cache builder so cache_prewarm jobs build the cache in the background,
	// keeping CLI commands snappy (they use cache when available without blocking on build).
	// Note: NewSchedulerWithProjectRoot automatically registers the scheduler globally.
	sched := schedulerpkg.NewSchedulerWithProjectRoot(storageProvider, specLoader, lifecycleLoader, factoryProjectRoot, system.NewObjectIDCacheBuilderForScheduler())

	// Inject validation scanner so cache_prewarm Tier 4 enqueues all cached objects for background
	// validation after the object ID cache is built. This populates the validation state cache so
	// system check --cache-only returns accurate results without running a full validation pass.
	sched.SetValidationScanner(system.NewValidationScannerForScheduler())

	// Set security context
	secCtx := pkgctx.NewSystemSecurityContext()
	sched.SetSecurityContext(secCtx)

	// When jobs_paused is true, scheduler loads jobs but does not run timer/immediate; only manual trigger runs
	sched.SetOnlyManualJobs(config.JobsPaused)

	// Route trigger-queue events (dequeued, processing, failed) to diagnostics.jsonl for visibility
	if stdioResult.CoordinatorWriter != nil {
		sched.SetTriggerQueueEventWriter(stdioResult.CoordinatorWriter)
	}

	// Capture full config at start so we can see how values change over time (each start event = one snapshot).
	startPayload := map[string]any{
		schedulerFieldProjectRoot: projectRoot,
		schedulerFieldJobsPaused:  config.JobsPaused,
		"config_at_start": map[string]any{
			objects.FieldKeyEnabled:   config.Enabled,
			schedulerFieldProjectType: config.ProjectType,
			schedulerFieldJobsPaused:  config.JobsPaused,
		},
	}
	if config.JobsPaused {
		startPayload["jobs_paused_action"] = "zqk scheduler config --no-jobs-paused to resume"
	}
	startMsg := "Starting scheduler daemon"
	if config.JobsPaused {
		startMsg = "Starting scheduler daemon (JOBS PAUSED: timer/immediate disabled; zqk scheduler config --no-jobs-paused to resume)"
	}
	emitSchedulerStartEventViaCoordinator(
		pkgctx.NewSystemContext(),
		projectRoot,
		storageProvider,
		startMsg,
		profile,
		startPayload,
	)

	// Create context that will be cancelled on interrupt
	startCtx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	defer cancel()

	// Start lifecycle event listener and transition updater (WAL → criteria → transitions). Bounded: 1 listener + 1 updater goroutine.
	if storageProvider != nil && factoryProjectRoot != emptyValue {
		if wal, err := lifecycle.GetOrCreateLifecycleWAL(factoryProjectRoot); err == nil {
			stopListener := lifecycle.RunListenerAndUpdater(startCtx, factoryProjectRoot, wal, cli.GetObjectStorageForProjectRoot)
			defer stopListener()
		}
	}

	// Start daemon-hosted reactive accumulator WAL subscribers (Section 8 of REACTIVE_MATERIALIZED_VIEW_ACCUMULATOR_PATTERN.md).
	// Keeps zero-cost projections (.zqk/state/*_lite.json) hot and fresh continuously via O(degree) WAL deltas.
	if factoryProjectRoot != emptyValue {
		StartSupervisedAccumulators(startCtx, factoryProjectRoot)
	}

	// Start Convergence Engine
	if factoryProjectRoot != emptyValue && !minimal {
		stopConvergence := sched.StartConvergenceEngine(startCtx)
		defer stopConvergence(startCtx)
	} else if minimal {
		schedulerpkg.SLog(logger).Info("Minimal mode: running without convergence engine").
			ProjectRoot(projectRoot).
			Log()
	}

	// Start QA Auditor Service (Truth Sentinel)
	if factoryProjectRoot != emptyValue && !minimal {
		if wal, err := lifecycle.GetOrCreateLifecycleWAL(factoryProjectRoot); err == nil {
			auditorPrivKey := filepath.Join(factoryProjectRoot, paths.ProjectDataDir, "keystore", "auditor.priv")
			signer, errSigner := qa.NewAuditorSigner(auditorPrivKey)
			if errSigner == nil {
				emitter := qa.NewInterruptEmitter(factoryProjectRoot)
				engine := qa.NewGuidanceEngine()
				gate := qa.NewAuditorGateForProject(storageProvider, factoryProjectRoot)
				svc := qa.NewAuditorService(wal, storageProvider, signer, emitter, engine, gate)

				auditorBud := goroutinelabels.DefaultBudget()
				auditorBuilder := goroutinelabels.NewGoroutine("scheduler_qa_auditor", "running background QA auditor service")
				if auditorBud != nil {
					auditorBuilder = auditorBuilder.WithBudget(auditorBud)
				}
				auditorBuilder.StartWithContext(startCtx, func(ctx context.Context) error {
					if err := svc.Run(ctx); err != nil {
						schedulerpkg.SLog(logger).Error("AuditorService exited with error", err).Log()
						return err
					}
					return nil
				})
			} else {
				schedulerpkg.SLog(logger).Warn("Failed to create QA Auditor signer; truth sentinel not started").WithError(errSigner).Log()
			}
		} else {
			schedulerpkg.SLog(logger).Warn("Failed to get lifecycle WAL; truth sentinel not started").WithError(err).Log()
		}
	}

	// Build high-volume event cache at daemon start so retention/aggregation have fast path on first run.
	// Decoupled from aggregation job timeout (OBJECT_MAINTENANCE_REDESIGN §4): long dedicated timeout, non-blocking.
	if storageProvider != nil && factoryProjectRoot != emptyValue {
		const daemonCacheBuildTimeout = 20 * time.Minute
		bud := goroutinelabels.DefaultBudget()
		builder := goroutinelabels.NewGoroutine("scheduler_daemon_high_volume_cache_build", "build high-volume event cache at daemon start")
		if bud != nil {
			builder = builder.WithBudget(bud)
		}
		builder.StartWithContext(startCtx, func(bgCtx context.Context) error {
			buildCtx, buildCancel := context.WithTimeout(bgCtx, daemonCacheBuildTimeout)
			defer buildCancel()
			logger := logging.GetLoggerFromProfile(profile)
			if err := storagepkg.EnsureHighVolumeEventCacheReady(buildCtx, factoryProjectRoot, storageProvider, false); err != nil {
				schedulerpkg.SLog(logger).Warn("Daemon-start high-volume cache build failed or timed out; retention/aggregation may use slower path").
					ProjectRoot(factoryProjectRoot).
					WithError(err).
					Log()
				return err
			}
			schedulerpkg.SLog(logger).Info("High-volume event cache ready at daemon start (retention/aggregation fast path available)").
				ProjectRoot(factoryProjectRoot).
				Log()
			return nil
		})
	}

	// Use a context that is cancelled on SIGTERM/SIGINT so the daemon exits when "scheduler stop"
	// sends SIGTERM or user hits Ctrl+C. The timeout hook does not pass a signal-aware context for
	// daemon commands, so cmd.Context() is never cancelled on signal (see SIGNAL_AND_CONTEXT.md).
	interruptCtx := cmd.Context()
	if interruptCtx == nil {
		interruptCtx = pkgctx.NewSystemContext()
	}
	if runtime.GOOS != schedulerGOOSWindows {
		sigCtx, stopSignal := signal.NotifyContext(pkgctx.NewSystemContext(), os.Interrupt, syscall.SIGTERM)
		defer stopSignal()
		interruptCtx = sigCtx
	}

	startSchedulerBackgroundWatchers(cmd, startCtx, cancel, interruptCtx, projectRoot, storageProvider, profile, sched)

	// Optional pprof HTTP server for performance profiling (ZQK_PPROF=1, default port 6060)
	diagnostics.StartPprofServerIfEnabled()

	// Single ensure at daemon start: required maintenance jobs from scheduler_maintenance_config.yaml.
	// Ensures SCH-maintenance-wal, SCH-cache-prewarm, SCH-val, SCH-evag, retention, audit aggregation, etc. so aggregate-then-retention runs.
	{
		logger := logging.GetLoggerFromProfile(profile)
		if result, err := system.EnsureRetentionJobsInProject(projectRoot, logger, storageProvider); err != nil {
			schedulerpkg.SLog(logger).Warn("Ensure maintenance jobs failed; run 'zqk system ensure-retention-jobs' so critical maintenance runs").
				WithError(err).
				Log()
		} else if !result.AlreadySatisfied {
			schedulerpkg.SLog(logger).Info("Ensure maintenance jobs: created or updated jobs from scheduler_maintenance_config.yaml").
				String("message", result.Message).
				Log()
		}
	}
	// Belt-and-suspenders: invalidate list + stream-registry snapshots before load (scheduler_job is CAS-backed;
	// load path also reconciles CAS index — see pkg/scheduler loadAndScheduleJobs).
	if projectRoot != emptyValue {
		storagepkg.InvalidateListCacheForKind(objects.KindSchedulerJob)
		storagepkg.InvalidateStreamRegistryCacheForKind(projectRoot, objects.KindSchedulerJob)
	}

	// Start Autonomy Inbox UI Server
	if projectRoot != emptyValue {
		goroutinelabels.NewGoroutine("refactored_worker", "Refactored raw goroutine").
			StartSimple(func() {
				func() {
					webDir := filepath.Join(projectRoot, "web")
					if fileutil.Exists(webDir) {
						mux := http.NewServeMux()
						mux.Handle("/", http.FileServer(http.Dir(webDir)))

						// SSE endpoint for live TSDB metrics
						mux.HandleFunc("/api/stream", func(w http.ResponseWriter, r *http.Request) {
							w.Header().Set("Content-Type", "text/event-stream")
							w.Header().Set("Cache-Control", "no-cache")
							w.Header().Set("Connection", "keep-alive")

							flusher, ok := w.(http.Flusher)
							if !ok {
								http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
								return
							}

							// Stream at 10Hz to give the Canvas a smooth feed
							ticker := time.NewTicker(100 * time.Millisecond)
							defer ticker.Stop()

							for {
								select {
								case <-r.Context().Done():
									return
								case <-ticker.C:
									fmt.Fprintf(w, "data: %s\n\n", autonomyTelemetryJSON(projectRoot))
									flusher.Flush()
								}
							}
						})

						// WebSocket endpoint for live telemetry stream
						mux.HandleFunc(routeTelemetryWS, func(w http.ResponseWriter, r *http.Request) {
							conn, err := upgradeToWebSocket(w, r)
							if err != nil {
								schedulerpkg.SLog(logging.GetLoggerFromProfile(profile)).Warn("Failed to upgrade connection to WebSocket").WithError(err).Log()
								return
							}
							defer conn.Close()

							// Stream at 10Hz to give the Canvas a smooth feed
							ticker := time.NewTicker(telemetryTickerDuration)
							defer ticker.Stop()

							// Start reading loop to keep connection alive and detect disconnect
							disconnectChan := make(chan struct{})
							goroutinelabels.NewGoroutine("websocket_read_loop", "Reads from client websocket to detect connection drop").
								StartSimple(func() {
									buf := make([]byte, webSocketReadBufferSize)
									for {
										_, err := conn.Read(buf)
										if err != nil {
											close(disconnectChan)
											return
										}
									}
								})

							for {
								select {
								case <-r.Context().Done():
									return
								case <-disconnectChan:
									return
								case <-ticker.C:
									err := writeWebSocketTextFrame(conn, []byte(autonomyTelemetryJSON(projectRoot)))
									if err != nil {
										return
									}
								}
							}
						})

						// Intent dispatch endpoint
						opts := transport.Options{
							Logger:      logging.GetLoggerFromProfile(profile),
							HandlerName: "SchedulerIntentHandler",
						}
						mux.HandleFunc("/api/intent", transport.NewHTTPHandler(opts, func(req transport.Request[any]) (transport.Response[map[string]any], error) {
							if req.Raw.Method != "POST" {
								return transport.Response[map[string]any]{
									StatusCode: http.StatusMethodNotAllowed,
								}, fmt.Errorf("Method not allowed")
							}

							// Fail-closed stub: do not report ok for unimplemented CAP wiring.
							schedulerpkg.SLog(logging.GetLoggerFromProfile(profile)).Info("Received intent from UI (stub — not dispatched)").Log()

							return transport.Response[map[string]any]{
								StatusCode: http.StatusNotImplemented,
								Body: map[string]any{
									"status":  "not_implemented",
									"message": "Autonomy intent→CAP dispatch is not implemented; refusing false success",
									"stub":    true,
								},
							}, nil
						}))
						corsMux := buildAutonomyCorsHandler(mux)

						// Loopback-only.
						// Set timeouts to prevent unbounded resource consumption.
						const autonomyUIAddr = "127.0.0.1:5173"
						schedulerpkg.SLog(logging.GetLoggerFromProfile(profile)).Info("Autonomy Inbox UI running on http://" + autonomyUIAddr).Log()
						uiServer := &http.Server{
							Addr:              autonomyUIAddr,
							Handler:           corsMux,
							ReadHeaderTimeout: 5 * time.Second,
							IdleTimeout:       60 * time.Second,
						}
						if err := uiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
							schedulerpkg.SLog(logging.GetLoggerFromProfile(profile)).Warn("Failed to start UI server").WithError(err).Log()
						}
					}
				}()
			})
	}

	// Start scheduler (this blocks until context is cancelled)
	// The scheduler runs in the foreground, but the process stays alive
	if err := sched.Start(startCtx); err != nil {
		return errfmt.Newf("failed to start scheduler").Wrap(err)
	}

	return nil
}

func startSchedulerBackgroundWatchers(_ *cobra.Command, startCtx context.Context, cancel context.CancelFunc, interruptCtx context.Context, projectRoot string, storageProvider storagepkg.ObjectStorageProvider, profile string, sched schedulerpkg.SchedulerInterface) {
	// Handle SIGUSR1: capture process dump (goroutines, heap, threads) when threads are escalating.
	// Emit events via async coordinator (same pattern as start/stop/trigger). Log immediately and run
	// capture in a background goroutine so the handler can receive more signals and slow capture
	// (e.g. 10k+ goroutines) doesn't block the handler.
	if runtime.GOOS != schedulerGOOSWindows {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGUSR1)
		dumpBud := goroutinelabels.DefaultBudget()
		dumpHandlerBuilder := goroutinelabels.NewGoroutine("scheduler_dump_handler", "handling SIGUSR1 for process dump")
		if dumpBud != nil {
			dumpHandlerBuilder = dumpHandlerBuilder.WithBudget(dumpBud)
		}
		dumpHandlerBuilder.StartSimple(func() {
			for {
				select {
				case <-startCtx.Done():
					return
				case sig, ok := <-sigChan:
					if !ok {
						return
					}
					if sig == syscall.SIGUSR1 {
						diagnosticsDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir, schedulerDiagnosticsDirName)
						emitSchedulerDumpEventViaCoordinator(startCtx, projectRoot, storageProvider, "started", nil, profile, map[string]any{schedulerFieldDir: diagnosticsDir})
						captureBuilder := goroutinelabels.NewGoroutine("scheduler_dump_capture", "capturing diagnostics after SIGUSR1")
						if dumpBud != nil {
							captureBuilder = captureBuilder.WithBudget(dumpBud)
						}
						captureBuilder.StartSimple(func() {
							defer func() {
								if r := recover(); r != nil {
									emitSchedulerDumpEventViaCoordinator(pkgctx.NewSystemContext(), projectRoot, storageProvider, "error", errfmt.Errorf("panic: %v", r), profile, map[string]any{schedulerFieldDir: diagnosticsDir})
								}
							}()
							captureErr := captureDiagnostics(diagnosticsDir, "scheduler_daemon")
							functional.When(func() bool { return captureErr != nil }).Then(func() {
								emitSchedulerDumpEventViaCoordinator(pkgctx.NewSystemContext(), projectRoot, storageProvider, "error", captureErr, profile, map[string]any{schedulerFieldDir: diagnosticsDir})
							}).OrElse(func() {
								emitSchedulerDumpEventViaCoordinator(pkgctx.NewSystemContext(), projectRoot, storageProvider, "complete", nil, profile, map[string]any{schedulerFieldDir: diagnosticsDir})
							}).Run()
						})
					}
				}
			}
		})
	}

	// Handle interrupt signal (SIGINT, SIGTERM)
	// interruptCtx is cancelled when the process receives SIGTERM or SIGINT (see above).
	// Cancel startCtx immediately so sched.Start() unblocks and shutdown begins; then best-effort
	// emit and RemoveKeepAlive. Doing I/O before cancel() could block (coordinator/storage under load)
	// and prevent the daemon from ever exiting (SCHEDULER_HUNG_SAMPLE_ANALYSIS, shutdown hang).
	interruptBud := goroutinelabels.DefaultBudget()
	interruptBuilder := goroutinelabels.NewGoroutine("scheduler_interrupt_handler", "handling scheduler interrupt signal")
	if interruptBud != nil {
		interruptBuilder = interruptBuilder.WithBudget(interruptBud)
	}
	interruptBuilder.StartWithContext(interruptCtx, func(ctx context.Context) error {
		<-ctx.Done()
		cancel() // First: unblock sched.Start() so shutdown runs; do not block on I/O below
		emitSchedulerStartEventViaCoordinator(
			pkgctx.NewSystemContext(),
			projectRoot,
			storageProvider,
			"Received interrupt signal, stopping scheduler daemon",
			profile,
			nil,
		)
		if projectRoot != emptyValue {
			if errKeepAlive := schedulerpkg.RemoveKeepAlive(projectRoot); errKeepAlive != nil {
				schedulerpkg.SLog(logging.GetLoggerFromProfile(profile)).Debug("Failed to remove keep-alive file").WithError(errKeepAlive).Log()
			}
		}
		return ctx.Err()
	})

	// Optional wall-clock limit (e.g. CI/E2E): detached daemons use argv[0]=zqk-scheduler and skip the idle
	// watchdog below, so this is the only in-process backstop if the parent test dies without teardown.
	if wallRaw := config.SchedulerMaxWallDuration().OrDefault(""); wallRaw != emptyValue {
		if wallDur, err := time.ParseDuration(wallRaw); err != nil {
			schedulerpkg.SLog(logging.GetLoggerFromProfile(profile)).Warn("Invalid scheduler max wall duration; ignoring").
				String(zqkenv.SchedulerMaxWallDuration().Name(), wallRaw).
				WithError(err).
				Log()
		} else if wallDur > 0 {
			wallBud := goroutinelabels.DefaultBudget()
			wallBuilder := goroutinelabels.NewGoroutine("scheduler_max_wall_duration", "cancel scheduler context after wall duration from env")
			if wallBud != nil {
				wallBuilder = wallBuilder.WithBudget(wallBud)
			}
			wallBuilder.StartWithContext(startCtx, func(ctx context.Context) error {
				timer := time.NewTimer(wallDur)
				defer timer.Stop()
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-timer.C:
					schedulerpkg.SLog(logging.GetLoggerFromProfile(profile)).Info("Scheduler max wall duration reached; shutting down").
						String(zqkenv.SchedulerMaxWallDuration().Name(), wallRaw).
						String("duration", wallDur.String()).
						Log()
					cancel()
					return nil
				}
			})
		}
	}

	// Start synthesis service
	if storageProvider != nil && projectRoot != emptyValue {
		if wal, err := lifecycle.GetOrCreateLifecycleWAL(projectRoot); err == nil {
			memStore := providers.NewMemGraphMemoryStore(nil)
			orchestrator := orchestration.NewManager(nil, memStore, storageProvider)

			// Register via pattern
			orchestration.GetRegistry().RegisterManager(orchestrator)

			activityTicker := ticker.NewActivityTicker()
			synthesisSvc := service.NewSynthesisService(wal, orchestrator, activityTicker)

			// Store in registry if needed for other components
			// provider.GetRegistry().RegisterSynthesisService(synthesisSvc)

			synthesisBud := goroutinelabels.DefaultBudget()
			synthesisBuilder := goroutinelabels.NewGoroutine("scheduler_synthesis_service", "running autonomous capability synthesis service")
			if synthesisBud != nil {
				synthesisBuilder = synthesisBuilder.WithBudget(synthesisBud)
			}
			synthesisBuilder.StartWithContext(startCtx, func(ctx context.Context) error {
				if err := synthesisSvc.Run(ctx); err != nil {
					schedulerpkg.SLog(logging.GetLoggerFromProfile(profile)).Error("SynthesisService exited with error", err).Log()
					return err
				}
				return nil
			})
		}
	}

	// When parent is not the main zqk process (e.g. run from script or IDE), shut down after prolonged idle so we don't hang forever.
	// Skip the watchdog when we're the designated long-lived daemon (started via "zqk use" or "zqk scheduler start" background):
	// the child is exec'd with argv[0]=zqk-scheduler and the parent exits, so we get reparented to init and IsParentZqk() is false.
	// On macOS, argv[0] manipulation doesn't always hide the true binary path from os.Args[0], so we also check an env var.
	if !cli.IsParentZqk() && os.Args[0] != schedulerDaemonProcessName && !strings.Contains(os.Args[0], schedulerDaemonProcessName) && !config.SchedulerDaemonMode().OrDefault(false) {
		sched.TouchActivity() // set before watchdog so first tick doesn't see zero and cancel immediately
		idleWatchdogBuilder := goroutinelabels.NewGoroutine("scheduler_child_idle_watchdog", "shutting down after idle when parent is not zqk")
		if bud := goroutinelabels.DefaultBudget(); bud != nil {
			idleWatchdogBuilder = idleWatchdogBuilder.WithBudget(bud)
		}
		idleWatchdogBuilder.StartWithContext(startCtx, func(ctx context.Context) error {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-ticker.C:
					if time.Since(sched.GetLastActivity()) > childIdleShutdownTimeout {
						schedulerpkg.SLog(logging.GetLoggerFromProfile(profile)).Info("Scheduler idle with no meaningful work; parent is not main zqk — shutting down").
							String("idle_timeout", childIdleShutdownTimeout.String()).
							Log()
						cancel()
						return nil
					}
				}
			}
		})
	}
}

// buildAutonomyCorsHandler wraps an http.Handler with restricted CORS headers for the Autonomy UI.
func buildAutonomyCorsHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "http://127.0.0.1:5173" || origin == "http://localhost:5173" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}
