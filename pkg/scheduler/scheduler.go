package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/zqk-os/zqk/pkg/ambience"
	"github.com/zqk-os/zqk/pkg/ambient"
	"github.com/zqk-os/zqk/pkg/circuitbreaker"
	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/contractchange"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/scheduler/transceiver"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const (
	pipelineKindSchedulerStartLoadAndSchedule = "scheduler.start_load_and_schedule"
	lockFailureDisableThreshold               = 5
	defaultPackageConcurrencyMaxWait          = 2 * time.Second
)

func getPackageConcurrencyMaxWait() time.Duration {
	if s := zqkenv.SchedulerPackageConcurrencyMaxWait().Get(); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d >= 0 {
			return d
		}
	}
	return defaultPackageConcurrencyMaxWait
}

// shouldSkipJobDueToLockFailures returns true if this job has already exceeded the lock-failure skip threshold (do not try lock, do not log again).
func (s *Scheduler) shouldSkipJobDueToLockFailures(jobID string) bool {
	s.lockFailureMu.Lock()
	defer s.lockFailureMu.Unlock()
	return s.lockFailureCountByJobID[jobID] >= lockFailureSkipThreshold
}

// clearLockFailureCount resets the lock-failure count for the job (e.g. after successful acquire so re-enabled jobs can run).
func (s *Scheduler) clearLockFailureCount(jobID string) {
	s.lockFailureMu.Lock()
	defer s.lockFailureMu.Unlock()
	delete(s.lockFailureCountByJobID, jobID)
}

// getDispatchDropRetryCount returns the current dispatch drop retry count for the job.
func (s *Scheduler) getDispatchDropRetryCount(jobID string) int {
	if s == nil {
		return 0
	}
	s.dispatchDropRetriesMu.Lock()
	defer s.dispatchDropRetriesMu.Unlock()
	return s.dispatchDropRetriesByJobID[jobID]
}

// incrementDispatchDropRetryCount increments and returns the dispatch drop retry count for the job.
func (s *Scheduler) incrementDispatchDropRetryCount(jobID string) int {
	if s == nil {
		return 0
	}
	s.dispatchDropRetriesMu.Lock()
	defer s.dispatchDropRetriesMu.Unlock()
	if s.dispatchDropRetriesByJobID == nil {
		s.dispatchDropRetriesByJobID = make(map[string]int)
	}
	s.dispatchDropRetriesByJobID[jobID]++
	return s.dispatchDropRetriesByJobID[jobID]
}

// clearDispatchDropRetryCount resets the dispatch drop retry count for the job.
func (s *Scheduler) clearDispatchDropRetryCount(jobID string) {
	if s == nil {
		return
	}
	s.dispatchDropRetriesMu.Lock()
	defer s.dispatchDropRetriesMu.Unlock()
	if s.dispatchDropRetriesByJobID != nil {
		delete(s.dispatchDropRetriesByJobID, jobID)
	}
}

// startCronOnce starts the robfig cron scheduler exactly once per Scheduler instance.
func (s *Scheduler) startCronOnce() {
	if s == nil || s.cron == nil {
		return
	}
	s.cronStartOnce.Do(func() {
		s.cron.Start()
	})
}

// TouchActivity records that the scheduler did meaningful work (e.g. job completed, trigger processed).
// Used by the idle watchdog when parent is not zqk to avoid shutting down while work is in progress.
func (s *Scheduler) TouchActivity() {
	s.lastActivityNanos.Store(time.Now().UnixNano())
}

// GetLastActivity returns the time of last meaningful activity for idle detection.
func (s *Scheduler) GetLastActivity() time.Time {
	n := s.lastActivityNanos.Load()
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n)
}

// SetTriggerQueueEventWriter sets the writer for trigger-queue events (dequeued, processing, failed).
// When set by the daemon (e.g. CoordinatorWriter for diagnostics.jsonl), EmitTriggerQueueEvent writes JSONL there.
func (s *Scheduler) SetTriggerQueueEventWriter(w io.Writer) {
	s.triggerQueueEventMu.Lock()
	defer s.triggerQueueEventMu.Unlock()
	s.triggerQueueEventWriter = w
}

// EmitTriggerQueueEvent writes a single JSON object (one line) to the trigger-queue event writer if set.
// Used so diagnostics.jsonl records trigger queue activity (SCH-AUTOFIX-* and other enqueued jobs).
func (s *Scheduler) EmitTriggerQueueEvent(event map[string]any) {
	s.triggerQueueEventMu.Lock()
	w := s.triggerQueueEventWriter
	s.triggerQueueEventMu.Unlock()
	if w == nil {
		return
	}
	if event == nil {
		return
	}
	if _, has := event["timestamp"]; !has {
		event["timestamp"] = zqktime.NowRFC3339UTC()
	}
	if _, has := event[objects.FieldKeyEventType]; !has {
		event[objects.FieldKeyEventType] = "trigger_queue"
	}
	line, err := json.Marshal(event)
	if err != nil {
		return
	}
	line = append(line, '\n')
	_, _ = w.Write(line)
}

// RecordTriggerQueueDequeued records trigger-queue metrics when the daemon dequeues requests.
func (s *Scheduler) RecordTriggerQueueDequeued(count int) {
	if s.metrics != nil {
		s.metrics.RecordTriggerQueueDequeued(count)
	}
}

// RecordTriggerQueueTriggerFailed records when triggering a job from the queue failed.
func (s *Scheduler) RecordTriggerQueueTriggerFailed(jobID string, err error) {
	if s.metrics != nil {
		s.metrics.RecordTriggerQueueTriggerFailed(jobID, err)
	}
}

// RecordTriggerQueueReloadRetry records when the daemon reloaded jobs and retried a trigger.
func (s *Scheduler) RecordTriggerQueueReloadRetry(jobID string) {
	if s.metrics != nil {
		s.metrics.RecordTriggerQueueReloadRetry(jobID)
	}
}

// RecordTriggerQueueReloadFailed records when reload (before retry) failed; job will not run.
func (s *Scheduler) RecordTriggerQueueReloadFailed(jobID string, err error) {
	if s.metrics != nil {
		s.metrics.RecordTriggerQueueReloadFailed(jobID, err)
	}
}

// recordLockFailure increments the consecutive lock-failure count for the job and returns (count, shouldLogFirstOrRepeat, shouldDisable).
// shouldLogFirstOrRepeat: true on first failure (log full error), true at skip threshold (log one "skipping" message), false in between (no log).
// shouldDisable: true when count reaches disable threshold (caller should disable job in storage).
func (s *Scheduler) recordLockFailure(jobID string) (count int, shouldLog bool, shouldDisable bool) {
	s.lockFailureMu.Lock()
	defer s.lockFailureMu.Unlock()
	s.lockFailureCountByJobID[jobID]++
	count = s.lockFailureCountByJobID[jobID]
	shouldLog = count == 1 || count == lockFailureSkipThreshold
	shouldDisable = count == lockFailureDisableThreshold
	return count, shouldLog, shouldDisable
}

// triggeredJobWork is a unit of work for the triggered-job worker pool.
// When the producer creates a context with timeout (e.g. cron, recoverMissedJob), it must pass
// the CancelFunc here so the worker can cancel when done; the producer must NOT defer cancel()
// or the context is canceled as soon as the producer returns, before the worker runs.
type triggeredJobWork struct {
	job         *ScheduledJob
	handler     JobHandler
	ctx         context.Context
	cancel      context.CancelFunc // optional; worker calls when done so context outlives producer
	nonBlocking bool               // when true, use SubmitNonBlocking so trigger-queue loop does not block
}

// submitTriggeredJob enqueues work to the bounded pool. Critical jobs go to the priority pool so they run first.
// When work.nonBlocking is true, uses SubmitNonBlocking and returns ErrPoolFull when the pool is full.
// Returns an error if neither pool is started or submit fails.
func (s *Scheduler) submitTriggeredJob(work triggeredJobWork) error {
	pool := s.triggeredPool
	if priorityDispatchJob(work.job) && s.triggeredPriorityPool != nil {
		pool = s.triggeredPriorityPool
	}
	if pool == nil {
		return errfmt.Errorf("scheduler triggered pool not started")
	}

	if !priorityDispatchJob(work.job) {
		countAtStart := runtime.NumGoroutine()
		blockStart := time.Now()
		if err := concurrency.WaitUnderGoroutineCeiling(work.ctx, concurrency.DefaultGoroutineCeiling, 200*time.Millisecond); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				callerType := work.job.TriggerType
				if callerType == "" {
					callerType = "submit"
				}
				s.recordDispatchAttemptDropped(callerType, dispatchPressureReasonResourceWaitDeadlineExceeded, work.job)
				SchedulerJobManagementLog(s.logger).Warn(LogEventSchedulerJobMgmtCronDispatchSkippedCeiling).
					JobID(work.job.ID).
					String("hint", "Increase concurrency.DefaultGoroutineCeiling or optimize job logic; dispatch dropped to shed load.").
					Log()
			}
			return err
		}
		if countAtStart >= concurrency.DefaultGoroutineCeiling {
			callerType := work.job.TriggerType
			if callerType == "" {
				callerType = "submit"
			}
			emitGoroutineCeilingBlockViaCoordinator(work.ctx, s.projectRoot, countAtStart, concurrency.DefaultGoroutineCeiling, time.Since(blockStart), callerType, string(pkgctx.ProfileSystem))
		}
	}

	fn := func(ctx context.Context) error {
		s.executeJob(ctx, work.job, work.handler)
		if work.cancel != nil {
			work.cancel()
		}
		return nil
	}
	var err error
	if work.nonBlocking {
		err = pool.SubmitNonBlocking(work.ctx, fn)
	} else {
		err = pool.Submit(work.ctx, fn)
	}
	if err == nil {
		s.markOneTimeImmediatePending(work.job)
	}
	return err
}

// ScheduledJob represents a job scheduled for execution
type ScheduledJob struct {
	ID                string
	JobType           string
	Category          string
	Title             string // Human-readable job title
	Description       string // Job description explaining what it does
	TriggerType       string // "timer", "manual", "immediate", "workflow", "event", "lifecycle", "lifecycle"
	ScheduleExpr      string // Cron expression (for timer trigger_type)
	WorkflowRef       string // Workflow reference (for workflow trigger_type)
	EventFilter       string // Event filter (for event trigger_type)
	LifecycleFilter   string // Lifecycle filter (for lifecycle trigger_type)
	Enabled           bool
	Status            string // Object lifecycle status (e.g. active, archived); skip schedule when archived
	ExecutionMode     string // "reusable" or "one_time"
	Transactional     bool   // If true, wrap all storage operations in a transaction (all-or-nothing)
	MaxRuntimeSeconds int    // Maximum execution time in seconds (0 = no timeout)
	// Run wrapper fields (for run_wrapper job_type)
	Command              string            // Command to execute
	CommandArgs          []string          // Command arguments
	WorkingDirectory     string            // Working directory
	EnvironmentVariables map[string]string // Environment variables
	RetryCount           int               // Number of retry attempts
	RetryDelaySeconds    int               // Delay between retries
	// Callback hooks (for run_wrapper job_type)
	CallbackOnCompletion string // Callback URL/command on success
	CallbackOnError      string // Callback URL/command on failure
	CallbackOnStatus     string // Callback URL/command for status updates
	CallbackType         string // "webhook", "command", or "event"
	// Callback listener fields (for callback_listener job_type)
	ListenerPort           int               // Port for HTTP server
	ListenerPath           string            // Base path for routes
	IdleShutdownSeconds    int               // Idle timeout before shutdown
	RouteHandlers          map[string]string // Route -> handler_type mappings
	AuthType               string            // Authentication type (none, jwt, x509, api_key, oauth2)
	AuthConfig             map[string]any    // Auth configuration
	CreatedAt              time.Time         // Object created_at; admission hourglass ages from this
	LastRunAt              *time.Time
	NextRunAt              *time.Time
	CronEntryID            cron.EntryID
	Running                bool
	RunningMu              sync.RWMutex
	ConcurrentAllowed      bool   // Whether multiple instances can run concurrently (from allow_parallel_execution or job-type default)
	AllowParallelExecution bool   // When true, overrides job-type default and allows concurrent execution (spec field)
	LogLevel               string // Logging verbosity: "default", "verbose", "debug" (default: "default")
	Priority               string // "normal", "high", "critical" — scheduling order (critical runs first)
	// Metadata is optional scheduler_job.metadata (e.g. test bundle fields, bundle_command_fingerprint).
	Metadata        map[string]any
	ExecutionDepth  int
	executionCtx    context.Context    // Context for current execution (for cancellation)
	executionCancel context.CancelFunc // Cancel function for current execution
}

// NewScheduler creates a new scheduler instance
func NewScheduler(storage storagepkg.ObjectStorageProvider, specLoader *objects.SpecLoader, lifecycleLoader *objects.LifecycleLoader) SchedulerInterface {
	return NewSchedulerWithProjectRoot(storage, specLoader, lifecycleLoader, "", nil)
}

// NewSchedulerWithProjectRoot creates a new scheduler instance with explicit project root.
func NewSchedulerWithProjectRoot(storage storagepkg.ObjectStorageProvider, specLoader *objects.SpecLoader, lifecycleLoader *objects.LifecycleLoader, projectRoot string, objectIDCacheBuilder ObjectIDCacheBuilder) SchedulerInterface {
	// Create cron with seconds support (6-field cron: second minute hour day month weekday)
	c := cron.New(cron.WithSeconds(), cron.WithLocation(time.UTC))

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	notificationContext := NewNotificationContext(logger, nil)

	// If project root not provided, try to get from file-based storage
	if projectRoot == emptyValue {
		if fileStorage, ok := storage.(*storagepkg.FileObjectStorage); ok {
			projectRoot = fileStorage.GetProjectRoot()
		}
	}

	// Initialize metrics pipeline for sampler-based event batching (BLI-912).
	// Uses process-wide singleton per project root so coordinator emit paths reuse the same
	// sampler registry instead of leaking flushTickerLoop goroutines per emit.
	metricsPipeline := metrics.MetricPipelineForProject(storage, projectRoot)
	if projectRoot != emptyValue {
		if reg := metricsPipeline.GetSamplerRegistry(); reg != nil {
			stats := reg.GetStats()
			dc, _ := stats["distinct_sampler_count"].(int)
			rc, _ := stats["sampler_count"].(int)
			pending, _ := stats["total_pending_events"].(int)
			SchedulerDaemonLog(logger).Info(LogEventSchedulerDaemonMetricsSamplerRegistryStats).
				Int("distinct_sampler_count", dc).
				Int("registry_key_count", rc).
				Int("total_pending_events", pending).
				Log()
		}
	}

	// Create event emitter function that uses MetricPipeline.Sample()
	// This allows scheduler metrics events to be batched/sampled for high-frequency events
	eventEmitter := func(event map[string]any) error {
		_, err := metricsPipeline.Sample(event)
		return err
	}

	// Initialize metrics collector
	metricsConfig := DefaultSchedulerMetricsConfig()
	metricsCollector := NewSchedulerMetricsCollectorWithConfig(metricsConfig)
	if provider := GetGlobalTSDBProvider(); provider != nil {
		metricsCollector.SetTSDBProvider(provider)
	}

	// Wire audit metrics collector to MetricPipeline (BLI-912 Phase 2)
	// This enables audit event metrics to be batched/sampled for high-frequency events
	// Disabled by default (opt-in) - can be enabled when needed for agent observability
	auditMetricsCollector := storagepkg.GetGlobalAuditMetricsCollector()
	auditMetricsCollector.SetEventEmitter(eventEmitter, false) // Default: disabled

	// Initialize transceiver router (best effort; do not fail scheduler construction)
	router := transceiver.NewRouterWithDefaults(logger)
	// Note: Scheduler may be created before command context exists, use system context
	// This is acceptable for long-lived components initialized at startup
	asyncRouter, err := transceiver.NewAsyncRouterFromProfile(pkgctx.NewSystemContext(), router, transceiver.GetDefaultProfileName(), logger)
	if err != nil {
		SchedulerDaemonLog(logger).Warn(LogEventSchedulerDaemonAsyncRouterFromProfileFailed).
			WithError(err).
			Log()
	}

	secCtx := pkgctx.NewSystemSecurityContext() // Default to system context

	// Initialize handler factory and job loader
	handlerFactory := NewHandlerFactory(
		storage, specLoader, lifecycleLoader, projectRoot,
		logger, notificationContext, asyncRouter, metricsCollector,
		objectIDCacheBuilder,
	)
	jobLoader := NewJobLoader(storage, secCtx, logger, metricsCollector)

	// Create process group manager with 3 second shutdown timeout
	// Short timeout ensures scheduler.Stop() completes quickly for tests and graceful shutdown
	// Note: Scheduler may be created before command context exists, use system context
	processGroupManager := NewProcessGroupManager(pkgctx.NewSystemContext(), 3*time.Second)

	ambienceMesh := ambience.NewInMemoryEventMesh()
	fseventsEngine := ambience.NewFSEventsEngine(ambienceMesh)

	// Bind ingest service to the event mesh (Event-Sourcing Layer)
	ingestSvc := ambient.NewAmbientIngestService(projectRoot, secCtx)
	ingestSvc.BindToMesh(ambienceMesh)

	sched := &Scheduler{
		cron:             c,
		jobs:             make(map[string]*ScheduledJob),
		storage:          storage,
		projectRoot:      projectRoot,
		specLoader:       specLoader,
		lifecycleLoader:  lifecycleLoader,
		logger:           logger,
		samplingPipeline: metricsPipeline,
		conflictMgr: &ConflictManager{
			runningJobs: make(map[string]*ScheduledJob),
			projectRoot: projectRoot,
		},
		secCtx:                       secCtx,
		notificationContext:          notificationContext,
		executor:                     &NativeExecutor{},
		router:                       router,
		asyncRouter:                  asyncRouter,
		metrics:                      metricsCollector,
		handlerFactory:               handlerFactory,
		jobLoader:                    jobLoader,
		processGroupManager:          processGroupManager,
		objectIDCacheBuilder:         objectIDCacheBuilder,
		ambienceMesh:                 ambienceMesh,
		anticipatoryEngine:           fseventsEngine,
		packageConcurrencyLimiter:    circuitbreaker.NewConcurrencyLimiter(logger, getPackageConcurrencyMaxWait()),
		globalTestConcurrencyLimiter: circuitbreaker.NewConcurrencyLimiter(logger, getSchedulerDispatchResourceWaitMax()),
		lockFailureCountByJobID:      make(map[string]int),
		dispatchDropRetriesByJobID:   make(map[string]int),
	}
	sched.jobsPausedScheduleExemptIDs = loadJobsPausedScheduleExemptIDs(projectRoot)
	handlerFactory.BindScheduler(sched)

	// Wire coordination kernel primitives if we have a project root.
	// This keeps the integration off in init/test contexts where we don't have a stable .zqk dir.
	if projectRoot != emptyValue {
		reg := NewJobStateRegistry(projectRoot)
		reg.SetObservabilityRecorder(metricsCollector.GetObservabilityRecorder())
		sched.stateRegistry = reg
		sched.policyEngine = NewPolicyEngine(reg, projectRoot, storage)
		sched.coordinationChannel = NewCoordinationChannel(projectRoot)
	}

	// Register globally so CLI GetGlobalScheduler can trigger lifecycle jobs.
	RegisterGlobalScheduler(sched)

	return sched
}

// SetSecurityContext sets the security context for scheduler operations
func (s *Scheduler) SetSecurityContext(secCtx *pkgctx.SecurityContext) {
	s.secCtx = secCtx
}

// SetNotificationContext sets the notification context for scheduler operations
func (s *Scheduler) SetNotificationContext(nc *NotificationContext) {
	s.notificationContext = nc
}

// SetOnlyManualJobs sets whether the scheduler should only allow manually triggered jobs (no timer or immediate at load).
// When true, jobs are still loaded and registered so "zqk scheduler trigger <id>" works, but cron and immediate runs are skipped.
func (s *Scheduler) SetOnlyManualJobs(only bool) {
	s.onlyManualJobs = only
}

// SetValidationScanner injects a ValidationScanner into the handler factory so cache_prewarm
// Tier 4 can enqueue all cached objects for background validation after the object ID cache is built.
// Call before the scheduler starts processing jobs (e.g. immediately after NewSchedulerWithProjectRoot).
func (s *Scheduler) SetValidationScanner(scanner ValidationScanner) {
	if s.handlerFactory != nil {
		s.handlerFactory.SetValidationScanner(scanner)
	}
}

// GetNotificationContext returns the notification context
func (s *Scheduler) GetNotificationContext() NotificationContextInterface {
	return s.notificationContext
}

// RegisterJob registers a job with the scheduler
func (s *Scheduler) RegisterJob(job *ScheduledJob) error {
	return s.registerTriggeredJob(job)
}

// GetJob returns a job by ID
func (s *Scheduler) GetJob(jobID string) (*ScheduledJob, bool) {
	s.jobsMu.RLock()
	defer s.jobsMu.RUnlock()
	job, ok := s.jobs[jobID]
	return job, ok
}

// ListJobs returns all registered jobs
func (s *Scheduler) ListJobs() []*ScheduledJob {
	s.jobsMu.RLock()
	defer s.jobsMu.RUnlock()
	jobs := make([]*ScheduledJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}
	return jobs
}

// GetExecutor returns the command executor
func (s *Scheduler) GetExecutor() CommandExecutor {
	if s.executor == nil {
		return &NativeExecutor{}
	}
	return s.executor
}

// Start starts the scheduler daemon
func (s *Scheduler) Start(ctx context.Context) error {
	startTime := time.Now()
	s.TouchActivity() // so idle watchdog does not fire before any job runs

	// Check permission to manage scheduler
	if err := s.checkPermission("manage:scheduler"); err != nil {
		return errfmt.Newf("permission denied").Wrap(err)
	}

	var alreadyRunning bool
	var existingPID int
	var pidCheckRunning bool
	err := concurrency.RunInLockWithLogger(
		&s.runningMu,

		LockNameSchedulerStartCheck,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if s.running {
				alreadyRunning = true
				return nil
			}

			// Check if scheduler is already running in another process (via PID file)
			if s.projectRoot != emptyValue {
				running, pid, err := IsSchedulerRunning(s.projectRoot)
				if err == nil && running {
					if pid == os.Getpid() {
						// This is our own process (CLI wrote the PID file before calling Start)
						// Set running to true so keep-alive and other loops know we are active.
						s.running = true
						return nil
					}
					existingPID = pid
					pidCheckRunning = true
					return nil
				}
			}

			s.running = true
			return nil
		},
	)
	if err != nil {
		return errfmt.Newf("failed to check scheduler state").Wrap(err)
	}

	if alreadyRunning {
		return errfmt.Errorf("scheduler is already running")
	}

	started := false
	defer func() {
		if !started {
			_ = concurrency.RunInLockWithLogger(
				&s.runningMu,
				LockNameSchedulerStartFailedReset,
				logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					s.running = false
					return nil
				},
			)
		}
	}()

	// CLI writes PID file before calling Start(), so PID file may hold our own PID; only fail when another process is running.
	if pidCheckRunning && existingPID > 0 && existingPID != os.Getpid() {
		return errfmt.Errorf("scheduler daemon is already running in another process (PID: %d)", existingPID)
	}

	SchedulerDaemonLog(s.logger).Info(LogEventSchedulerDaemonStarting).Log()

	// When started by launchd (or without CLI context), projectRoot can be empty; resolve from CWD
	// so we still write PID file and keep-alive (WorkingDirectory in plist sets CWD).
	if s.projectRoot == emptyValue {
		if root := ResolveProjectRootFromCWD(); root != emptyValue {
			s.projectRoot = root
		}
	}

	// Architectural sanity check: ensure storage provider is injected.
	if s.storage == nil {
		SchedulerDaemonLog(s.logger).Error("Scheduler daemon failed to start: storage provider is nil", errfmt.Errorf("missing storage provider")).Log()
		return errfmt.Errorf("scheduler daemon configuration error: missing storage provider")
	}

	// Write PID file so other processes can detect if scheduler is running
	if s.projectRoot != emptyValue {
		if err := writePIDFile(s.projectRoot); err != nil {
			SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonPidWriteFailed).
				ProjectRoot(s.projectRoot).
				WithError(err).
				Log()
			// Don't fail startup - PID file is for convenience, not critical
		}
	}

	// Set process-wide goroutine budget and create bounded pool for triggered jobs (factory pattern).
	const triggeredQueueSize = 10000
	globalGoroutineCap := getSchedulerGoroutineCap()
	triggeredPoolSize := getSchedulerTriggeredPoolSize()
	budget := goroutinelabels.NewBudget(goroutinelabels.BudgetConfig{MaxTotal: globalGoroutineCap})
	goroutinelabels.SetDefaultBudget(budget)
	// When pool creation is declined (no budget or budget exceeded), notify coordinator and record metric.
	projectRoot := s.projectRoot
	storageProvider := s.storage
	metricsCollector := s.metrics
	goroutinelabels.SetPoolCreationDeclinedNotifier(func(name, purpose, reason string) {
		emitPoolCreationDeclinedViaCoordinator(pkgctx.NewSystemContext(), projectRoot, storageProvider, name, purpose, reason)
		if metricsCollector != nil {
			metricsCollector.RecordPoolCreationDeclined(name, purpose, reason)
		}
	})
	pool := budget.NewPool("scheduler_triggered_job_worker", "executing event/lifecycle/immediate-triggered jobs", triggeredPoolSize, triggeredQueueSize)
	s.triggeredPool = pool
	pool.Start(ctx)

	// Priority pool for critical jobs (timer maintenance, aggregation, retention, cache_prewarm). Same budget; separate workers so testing load on the main pool cannot starve scheduled maintenance.
	const triggeredPriorityPoolSize = 8
	const triggeredPriorityQueueSize = 96
	priorityPool := budget.NewPool("scheduler_triggered_priority", "executing critical maintenance/aggregation/retention jobs", triggeredPriorityPoolSize, triggeredPriorityQueueSize)
	s.triggeredPriorityPool = priorityPool
	priorityPool.Start(ctx)

	// Clean up stale scheduler lock files on startup (older than 5 minutes)
	if s.projectRoot != emptyValue && s.stateRegistry != nil {
		if cleaned, err := s.stateRegistry.CleanStaleLocks(5 * time.Minute); err == nil && cleaned > 0 {
			slog.Info("Cleaned up stale scheduler lock files on startup", "cleaned_count", cleaned)
		}
	}

	// Contract-change shockwave: demote shovel_ready|execution_locked that fail new invariants.
	// TRACK: BLI-1785918841712163000-f128dc79
	if s.projectRoot != emptyValue && s.storage != nil {
		if res, err := contractchange.ApplyPending(ctx, s.projectRoot, s.storage); err != nil {
			slog.Warn("contract-change apply failed on scheduler start", "error", err)
		} else if res.EventsConsumed > 0 || len(res.Demoted) > 0 {
			slog.Info("contract-change shockwave applied", "events_consumed", res.EventsConsumed, "demoted", len(res.Demoted))
		}
	}

	// Load and schedule all enabled jobs (immediate jobs submit to pool above; reload=false = initial load)
	type startLoadAndScheduleState struct {
		loadErr   error
		returnErr error
	}
	st := &startLoadAndScheduleState{}
	pl := pipeline.NewBuilder(pipelineKindSchedulerStartLoadAndSchedule, s.logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(s.logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage(pipeline.StageIngest, func(pctx *pipeline.Context, payload any) (any, error) {
			return payload, nil
		}).
		AddStage(StageLoadAndScheduleJobs, func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*startLoadAndScheduleState](payload)
			if !ok {
				in = &startLoadAndScheduleState{}
			}
			in.loadErr = s.loadAndScheduleJobs(ctx, false)
			if in.loadErr != nil {
				in.returnErr = errfmt.Newf("failed to load jobs").Wrap(in.loadErr)
			}
			return in, nil
		}).
		AddStage(pipeline.StageFinalize, func(pctx *pipeline.Context, payload any) (any, error) {
			return payload, nil
		}).
		Build()

	out, runErr := pl.Run(&pipeline.Context{Ctx: ctx, Outcome: make(map[string]any)}, st)
	_ = out // payload is mutated in-place
	if runErr != nil {
		return runErr
	}
	if st.returnErr != nil {
		return st.returnErr
	}

	// Cron is started from loadAndScheduleJobs (startCronOnce) after timer jobs are scheduled and
	// startup bootstrap runs, but before immediate jobs are submitted — so cache_prewarm cannot be
	// delayed by a full queue of bundle submits during initial load.

	// Watch for job changes in background (counts against process goroutine budget)
	bud := goroutinelabels.DefaultBudget()

	// One-time cleanup: move legacy flat state/*.yaml files into per-job subdirectories so the
	// state/ top level stays scannable (see FILESYSTEM_DATA_LAYOUT.md, JobStateRegistry).
	if s.stateRegistry != nil && s.projectRoot != emptyValue {
		reg := s.stateRegistry
		rootLog := s.projectRoot
		migrationBuilder := goroutinelabels.NewGoroutine("scheduler_legacy_state_migration", "migrating legacy flat scheduler state YAML into per-job directories")
		if bud != nil {
			migrationBuilder = migrationBuilder.WithBudget(bud)
		}
		migrationBuilder.StartWithContext(ctx, func(ctx context.Context) error {
			nFlat, migErr := reg.MigrateLegacyFlatStateFilesBestEffort()
			if migErr != nil {
				SchedulerDaemonLog(s.logger).Debug(LogEventSchedulerDaemonLegacyStateMigrationFinishedWithError).
					WithError(migErr).
					Log()
				return nil
			}
			nBucket, migErr2 := reg.MigrateUnbucketedJobStateDirsBestEffort()
			if migErr2 != nil {
				SchedulerDaemonLog(s.logger).Debug(LogEventSchedulerDaemonLegacyStateMigrationFinishedWithError).
					WithError(migErr2).
					Log()
				return nil
			}
			if nFlat > 0 {
				SchedulerDaemonLog(s.logger).Info(LogEventSchedulerDaemonLegacyStateFilesMigrated).
					Int("files", nFlat).
					ProjectRoot(rootLog).
					Log()
			}
			if nBucket > 0 {
				SchedulerDaemonLog(s.logger).Info(LogEventSchedulerDaemonStateBucketDirsMigrated).
					Int("dirs", nBucket).
					ProjectRoot(rootLog).
					Log()
			}
			return nil
		})
	}

	watchBuilder := goroutinelabels.NewGoroutine("scheduler_watch_jobs", "watching for job configuration changes")
	if bud != nil {
		watchBuilder = watchBuilder.WithBudget(bud)
	}
	watchBuilder.StartWithContext(ctx, func(ctx context.Context) error {
		s.watchJobChanges(ctx)
		return nil
	})

	// Recover/reset any orphaned agent tasks left in 'in_progress' or 'pending_verification' status from prior crashed runs.
	s.recoverOrphanedTasks(ctx)

	swarmWatchBuilder := goroutinelabels.NewGoroutine("scheduler_watch_swarm", "watching for queued agent tasks for swarm")
	if bud != nil {
		swarmWatchBuilder = swarmWatchBuilder.WithBudget(bud)
	}
	_ = fileutil.WriteStandardFile("/tmp/zqk-swarm-watcher-started.txt", []byte("YES"))
	swarmWatchBuilder.StartWithContext(ctx, func(ctx context.Context) error {
		_ = fileutil.WriteStandardFile("/tmp/zqk-swarm-watcher-inside.txt", []byte("YES"))
		s.watchSwarmTasks(ctx)
		return nil
	})

	// Watch scheduler config so jobs_paused changes (e.g. config --no-jobs-paused) take effect without restart
	if s.projectRoot != emptyValue {
		configWatchBuilder := goroutinelabels.NewGoroutine("scheduler_watch_config", "watching scheduler config for jobs_paused changes")
		if bud != nil {
			configWatchBuilder = configWatchBuilder.WithBudget(bud)
		}
		configWatchBuilder.StartWithContext(ctx, func(ctx context.Context) error {
			s.watchSchedulerConfig(ctx)
			return nil
		})
	}

	// Watch for coordination channel events (including drift detection)
	if s.projectRoot != emptyValue && s.coordinationChannel != nil {
		coordWatchBuilder := goroutinelabels.NewGoroutine("scheduler_coordination_watch", "watching coordination channel events")
		if bud != nil {
			coordWatchBuilder = coordWatchBuilder.WithBudget(bud)
		}
		coordWatchBuilder.StartWithContext(ctx, func(ctx context.Context) error {
			_ = s.coordinationChannel.WatchEvents(ctx)
			return nil
		})

		coordHandlerBuilder := goroutinelabels.NewGoroutine("scheduler_coordination_handler", "handling coordination channel events")
		if bud != nil {
			coordHandlerBuilder = coordHandlerBuilder.WithBudget(bud)
		}
		coordHandlerBuilder.StartWithContext(ctx, func(ctx context.Context) error {
			s.handleCoordinationEvents(ctx)
			return nil
		})

		s.startHourglassWatcher(ctx, bud)
	}

	// Watch for cross-process job trigger requests
	if s.projectRoot != emptyValue {
		triggerQueue := NewJobTriggerQueue(s.projectRoot)
		triggerBuilder := goroutinelabels.NewGoroutine("scheduler_trigger_queue", "watching cross-process job trigger queue")
		if bud != nil {
			triggerBuilder = triggerBuilder.WithBudget(bud)
		}
		// SCH-cache-prewarm is submitted once at startup via bootstrapCriticalTimerJobsOnDaemonStart (priority pool);
		// do not enqueue here—file queue can lag behind bulk SCH-run-* drains and ReloadJobs.
		triggerBuilder.StartWithContext(ctx, func(ctx context.Context) error {
			triggerQueue.WatchTriggerQueue(ctx, s)
			return nil
		})
	}

	// Start health monitor to detect and recover from missed jobs
	healthBuilder := goroutinelabels.NewGoroutine("scheduler_health_monitor", "monitoring scheduler health and recovering from missed jobs")
	if bud != nil {
		healthBuilder = healthBuilder.WithBudget(bud)
	}
	healthBuilder.StartWithContext(ctx, func(ctx context.Context) error {
		s.healthMonitor(ctx)
		return nil
	})

	if s.anticipatoryEngine != nil {
		if err := s.anticipatoryEngine.Start(ctx); err != nil {
			SchedulerDaemonLog(s.logger).Warn("Failed to start anticipatory engine").
				WithError(err).
				Log()
		} else {
			SchedulerDaemonLog(s.logger).Info("Anticipatory engine started successfully").Log()
		}
	}

	// Start keep-alive heartbeat
	keepAliveCtx, keepAliveCancel := context.WithCancel(ctx)
	defer keepAliveCancel()
	keepAliveBuilder := goroutinelabels.NewGoroutine("scheduler_keepalive", "sending keep-alive heartbeats")
	if bud != nil {
		keepAliveBuilder = keepAliveBuilder.WithBudget(bud)
	}
	keepAliveBuilder.StartWithContext(keepAliveCtx, func(ctx context.Context) error {
		s.keepAliveHeartbeat(ctx)
		return nil
	})

	// Record metrics
	if s.metrics != nil {
		s.metrics.RecordSchedulerStart(time.Since(startTime))
	}

	// Start async router if available
	if s.asyncRouter != nil {
		// Load routing rules before starting
		if s.router != nil {
			ruleLoader := transceiver.NewRoutingRuleLoader("", s.logger)
			rules, err := ruleLoader.LoadAllRules()
			if err != nil {
				SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonRoutingRulesLoadFailed).
					WithError(err).
					Log()
			} else {
				// Validate rules before loading
				validationErrors := transceiver.ValidateRoutingRules(rules, s.router, s.logger)
				if len(validationErrors) > 0 {
					SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonRoutingRulesValidationErrorsFound).
						ErrorCount(len(validationErrors)).
						Log()
					for _, validationErr := range validationErrors {
						SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonRoutingRuleValidationRow).
							ValidationRowField(validationErr.Field).
							ValidationRowMessage(validationErr.Message).
							Log()
					}
					// Continue loading rules despite validation errors (best-effort)
				}

				s.router.LoadRules(rules)
				SchedulerDaemonLog(s.logger).Info(LogEventSchedulerDaemonRoutingRulesLoaded).
					RuleCount(len(rules)).
					ValidationErrorCount(len(validationErrors)).
					Log()
			}
		}

		// Set project root and storage provider for coordinator events
		if s.projectRoot != emptyValue {
			s.asyncRouter.SetProjectRoot(s.projectRoot)
			s.asyncRouter.SetStorageProvider(s.storage)
		}

		// Register async router with shutdown coordinator for graceful shutdown
		// Note: Using storagepkg alias to avoid import cycle
		shutdownCoordinator := storagepkg.GetGlobalShutdownCoordinator()
		shutdownCoordinator.RegisterQueue(s.asyncRouter)

		if err := s.asyncRouter.Start(ctx); err != nil {
			SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonAsyncRouterStartFailed).
				WithError(err).
				Log()
		}
	}

	// Wait for context cancellation
	started = true
	<-ctx.Done()

	// Cancel keep-alive heartbeat to ensure cleanup
	keepAliveCancel()

	s.Stop()
	s.Stop()
	return nil
}
