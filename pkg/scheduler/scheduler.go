package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/circuitbreaker"
	"github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/pkg/ambience"
	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
		"github.com/lanceman/zqk/pkg/metrics"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/pipeline"
	"github.com/lanceman/zqk/pkg/scheduler/transceiver"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/lanceman/zqk/pkg/zqktime"
	"github.com/robfig/cron/v3"
)

var (
	// globalSchedulerRegistry holds the global scheduler instance for lifecycle hooks
	globalSchedulerRegistry *Scheduler
	globalSchedulerMu       sync.RWMutex
)

const (
	defaultSchedulerGoroutineCap  = 512
	defaultSchedulerTriggeredPool = 8
	minSchedulerTriggeredPoolSize = 8
	maxSchedulerTriggeredPoolSize = 256
	minSchedulerGoroutineCap      = 128
	maxSchedulerGoroutineCap      = 4096

	maxSchedulerImmediateLoadBatchSize  = 50000
	maxSchedulerImmediateLoadBatchPause = 5 * time.Minute

	pipelineKindSchedulerStartLoadAndSchedule = "scheduler.start_load_and_schedule"
)

// evtDataKey is the context key for scheduler job event payload (empty struct; avoids string-key collisions).
type evtDataKey struct{}

func WithEventData(ctx context.Context, data map[string]any) context.Context {
	return context.WithValue(ctx, evtDataKey{}, data)
}

// secJobCtxKey is the context key for operation-execution security context.
type secJobCtxKey struct{}

func getSchedulerGoroutineCap() int {
	v := os.Getenv(zqkenv.SchedulerGoroutineCap())
	if v == emptyValue {
		return defaultSchedulerGoroutineCap
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return defaultSchedulerGoroutineCap
	}
	if n < minSchedulerGoroutineCap {
		return minSchedulerGoroutineCap
	}
	if n > maxSchedulerGoroutineCap {
		return maxSchedulerGoroutineCap
	}
	return n
}

func getSchedulerTriggeredPoolSize() int {
	v := os.Getenv(zqkenv.SchedulerTriggeredPoolSize())
	if v == emptyValue {
		return defaultSchedulerTriggeredPool
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return defaultSchedulerTriggeredPool
	}
	if n < minSchedulerTriggeredPoolSize {
		return minSchedulerTriggeredPoolSize
	}
	if n > maxSchedulerTriggeredPoolSize {
		return maxSchedulerTriggeredPoolSize
	}
	return n
}

// getSchedulerImmediateLoadBatchSize is how many immediate jobs to schedule per jobsMu acquisition
// during loadAndScheduleJobs pass 2 (initial load / full reload). 0 means no chunking: one lock for all immediates.
// Default to 100 if not specified to reduce lock contention and prevent startup hangs.
func getSchedulerImmediateLoadBatchSize() int {
	v := strings.TrimSpace(os.Getenv(zqkenv.SchedulerImmediateLoadBatchSize()))
	if v == emptyValue {
		return 100
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	if n == 0 {
		return 0
	}
	if n > maxSchedulerImmediateLoadBatchSize {
		return maxSchedulerImmediateLoadBatchSize
	}
	return n
}

// getSchedulerImmediateLoadBatchPause is the pause between immediate batches when chunking is enabled.
func getSchedulerImmediateLoadBatchPause() time.Duration {
	v := strings.TrimSpace(os.Getenv(zqkenv.SchedulerImmediateLoadBatchPause()))
	if v == emptyValue {
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 0
	}
	if d > maxSchedulerImmediateLoadBatchPause {
		return maxSchedulerImmediateLoadBatchPause
	}
	return d
}

// testDispatchResourceWaitMaxNS when non-zero overrides [getSchedulerDispatchResourceWaitMax] so
// integration tests can expire dispatch quickly without relying on env (minimum 5m there). Cleared via
// [setDispatchResourceWaitMaxForTest](0).
var testDispatchResourceWaitMaxNS atomic.Int64

// setDispatchResourceWaitMaxForTest overrides dispatch wall time used by dispatchContextForScheduledJob
// when max_runtime_seconds > 0 (tests only). Pass 0 to clear.
func setDispatchResourceWaitMaxForTest(d time.Duration) {
	if d <= 0 {
		testDispatchResourceWaitMaxNS.Store(0)
		return
	}
	testDispatchResourceWaitMaxNS.Store(int64(d))
}

// getSchedulerDispatchResourceWaitMax is the maximum wall time a dispatch context may wait on
// goroutine-ceiling gates and blocked pool submit before this attempt is abandoned (default 2h).
// Execution max_runtime_seconds is separate and starts when the handler runs. Invalid or tiny
// values fall back to the default; values above the cap are clamped.
func getSchedulerDispatchResourceWaitMax() time.Duration {
	if ns := testDispatchResourceWaitMaxNS.Load(); ns > 0 {
		return time.Duration(ns)
	}
	const (
		defaultMax = 2 * time.Hour
		minMax     = 5 * time.Minute
		maxMax     = 48 * time.Hour
	)
	v := strings.TrimSpace(os.Getenv(zqkenv.SchedulerDispatchResourceWaitMax()))
	if v == emptyValue {
		return defaultMax
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < minMax {
		return defaultMax
	}
	if d > maxMax {
		return maxMax
	}
	return d
}

// getSchedulerDefaultPackageConcurrency returns the fallback max concurrent run_wrapper jobs per
// go-test package when scan metadata and package_concurrency_limits.json omit the path.
func getSchedulerDefaultPackageConcurrency() int {
	const (
		defaultN = 1
		maxN     = 32
	)
	v := strings.TrimSpace(os.Getenv(zqkenv.SchedulerDefaultPackageConcurrency()))
	if v == emptyValue {
		return defaultN
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultN
	}
	if n == 0 {
		return 0
	}
	if n < 0 {
		return defaultN
	}
	if n > maxN {
		return maxN
	}
	return n
}

type operationCallbackContextKey struct{}

// ContextWithOperationCallback attaches an OperationCallback to the context for scheduler operations.
// If cb is nil, a NoOpOperationCallback is used.
func ContextWithOperationCallback(ctx context.Context, cb concurrency.OperationCallback) context.Context {
	if cb == nil {
		cb = &concurrency.NoOpOperationCallback{}
	}
	return context.WithValue(ctx, operationCallbackContextKey{}, cb)
}

// triggerOriginContextKey is the context key for run trigger origin (e.g. "pre_commit").
// When set to "pre_commit", job CallbackOnCompletion/CallbackOnError are invoked; timer runs do not set this.
type triggerOriginContextKey struct{}

// TriggerOriginPreCommit is the value for pre-commit-triggered runs (callback runs only then).
const TriggerOriginPreCommit = "pre_commit"

// TriggerOriginDataCellEnvelopeTick marks jobs triggered by data_cell_envelope_tick dispatch (follow-up hooks).
const TriggerOriginDataCellEnvelopeTick = "data_cell_envelope_tick"

// ContextWithTriggerOrigin attaches a trigger origin to the context (e.g. TriggerOriginPreCommit).
func ContextWithTriggerOrigin(ctx context.Context, origin string) context.Context {
	if ctx == nil || origin == emptyValue {
		return ctx
	}
	return context.WithValue(ctx, triggerOriginContextKey{}, origin)
}

// submitNonBlockingKey is the context key for "submit to pool without blocking".
// When set, submitTriggeredJob uses SubmitNonBlocking so the trigger-queue loop does not block when the pool is full.
type submitNonBlockingKey struct{}

// ContextWithSubmitNonBlocking marks the context so TriggerJob will use non-blocking pool submit.
// Use from the trigger-queue so the loop can re-enqueue on pool full instead of blocking.
func ContextWithSubmitNonBlocking(ctx context.Context) context.Context {
	if ctx == nil {
		return nil
	}
	return context.WithValue(ctx, submitNonBlockingKey{}, true)
}

// submitNonBlockingFromContext returns true if the context requests non-blocking submit.
func submitNonBlockingFromContext(ctx context.Context) bool {
	return ctx != nil && ctx.Value(submitNonBlockingKey{}) != nil
}

// TriggerOriginFromContext returns the trigger origin from context, or empty if not set.
func TriggerOriginFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v := ctx.Value(triggerOriginContextKey{}); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// RegisterGlobalScheduler registers a scheduler instance globally for lifecycle hooks.
// This allows the storage layer to trigger lifecycle jobs without direct dependency.
// Tests that call NewSchedulerWithProjectRoot must restore the previous registration
// when done (see WithGlobalSchedulerRollback); otherwise later code in the same process
// may observe a test scheduler or call Stop() on the wrong logical instance.
func RegisterGlobalScheduler(s *Scheduler) {
	_ = concurrency.RunInLockWithLogger(
		&globalSchedulerMu,
		LockNameSchedulerRegisterGlobal,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			globalSchedulerRegistry = s
			return nil
		},
	)
}

// WithGlobalSchedulerRollback returns a cleanup function that restores the global
// scheduler registry to whatever was registered when WithGlobalSchedulerRollback was
// called. Call it before NewSchedulerWithProjectRoot in tests: that constructor always
// calls RegisterGlobalScheduler, which is process-wide; without restoring, later tests
// or other packages in the same binary can observe a stopped test scheduler or leak
// registration across test boundaries.
func WithGlobalSchedulerRollback() (cleanup func()) {
	prev := GetGlobalScheduler()
	return func() {
		RegisterGlobalScheduler(prev)
	}
}

// GetGlobalScheduler returns the globally registered scheduler instance.
// It may block if the daemon holds globalSchedulerMu (e.g. during job dispatch).
func GetGlobalScheduler() *Scheduler {
	var s *Scheduler
	_ = concurrency.RunInRLockWithLogger(
		&globalSchedulerMu,
		LockNameSchedulerGetGlobal,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			s = globalSchedulerRegistry
			return nil
		},
	)
	return s
}

// GetGlobalSchedulerIfAvailable returns the global scheduler if the lock is available without blocking.
// Returns (nil, false) if the lock is held (e.g. by the daemon). Use this for quick status checks
// so "scheduler status" never blocks; callers should fall back to PID-file check (IsSchedulerRunning).
func GetGlobalSchedulerIfAvailable() (*Scheduler, bool) {
	if !globalSchedulerMu.TryRLock() {
		return nil, false
	}
	defer globalSchedulerMu.RUnlock()
	return globalSchedulerRegistry, true
}

// Scheduler manages and executes scheduled jobs
type Scheduler struct {
	cron            *cron.Cron
	jobs            map[string]*ScheduledJob
	jobsMu          sync.RWMutex
	executionMu     sync.Mutex // Added for scheduler thread safety
	running         bool
	runningMu       sync.RWMutex
	storage         storagepkg.ObjectStorageProvider
	projectRoot     string // Store project root for handlers (needed for graph backends)
	specLoader      *objects.SpecLoader
	lifecycleLoader *objects.LifecycleLoader
	logger          logging.Logger
	conflictMgr     *ConflictManager

	// Scheduler coordination kernel primitives (PLAN-220 / ITEM-901 / CRIT-9040).
	// These provide distributed job awareness beyond the in-memory ConflictManager.
	stateRegistry       JobStateRegistryInterface
	policyEngine        PolicyEngineInterface
	coordinationChannel CoordinationChannelInterface
	secCtx              *pkgctx.SecurityContext      // Security context for scheduler operations
	notificationContext NotificationContextInterface // Notification context for user notifications
	executor            CommandExecutor              // Command executor for job execution (default: NativeExecutor)

	router               *transceiver.Router // Synchronous router (used by async router)
	asyncRouter          *transceiver.AsyncRouter
	metrics              SchedulerMetricsCollector // Metrics collector for observability
	handlerFactory       HandlerFactoryInterface   // Factory for creating job handlers
	jobLoader            JobLoaderInterface        // Loader for hydrating jobs from storage
	processGroupManager  ProcessGroupManagerInterface
	objectIDCacheBuilder ObjectIDCacheBuilder // optional; for cache_prewarm (ITEM-954)

	// Symbiotic Mesh Anticipatory Logic components
	ambienceMesh              ambience.EventMesh
	anticipatoryEngine        ambience.AnticipatoryEngine
	packageConcurrencyLimiter circuitbreaker.ConcurrencyLimiter

	// Bounded pool for event- and lifecycle-triggered jobs (from goroutine budget factory).
	triggeredPool *goroutinelabels.Pool
	// Priority pool for critical jobs (maintenance, audit_event_aggregation, retention_tolerance, cache_prewarm).
	// Critical jobs are submitted here so they run before general queue work.
	triggeredPriorityPool *goroutinelabels.Pool

	// onlyManualJobs when true: do not schedule timer or immediate jobs at load; only register them so manual trigger still works.
	// Set via SetOnlyManualJobs (from scheduler config jobs_paused). Lets daemon start and stabilize without running the full job set.
	onlyManualJobs bool

	// jobsPausedScheduleExemptIDs, when non-nil, overrides builtin exemption for jobs_paused (from scheduler_maintenance_config.yaml).
	jobsPausedScheduleExemptIDs map[string]bool

	// lockFailureCountByJobID tracks consecutive lock create/acquire failures per job so we can skip logging and optionally disable.
	lockFailureCountByJobID map[string]int
	lockFailureMu           sync.Mutex

	// lastActivityNanos is updated when the scheduler does meaningful work (job completed, trigger processed).
	// Used by the idle watchdog when parent is not zqk to shut down after prolonged inactivity.
	lastActivityNanos atomic.Int64

	// triggerQueueEventWriter, when set by the daemon, receives JSONL trigger-queue events (dequeued, processing, failed)
	// so diagnostics.jsonl has a paper trail for SCH-AUTOFIX-* and other enqueued triggers.
	triggerQueueEventWriter io.Writer
	triggerQueueEventMu     sync.Mutex

	// samplingPipeline is the ITEM-912 MetricPipeline used for sampled coordinator/audit events (distinct from [Scheduler.metrics] collector).
	// Stopped during [Scheduler.Stop] so flush ticker goroutines exit before storage shutdown.
	samplingPipeline *metrics.MetricPipeline

	// Throttle full CAS index scans for scheduler_job during periodic reload (watchJobChanges).
	schedulerJobCASReconcileMu     sync.Mutex
	lastSchedulerJobCASReconcileAt time.Time

	// keepAliveTickerInterval overrides DefaultKeepAliveInterval in keepAliveHeartbeat when > 0 (tests only).
	keepAliveTickerInterval time.Duration

	// cronStartOnce ensures cron.Start runs once per Scheduler lifetime (initial load schedules timers before immediate jobs submit).
	cronStartOnce sync.Once

	// TestHookLoadSchedulingPhase is invoked during loadAndScheduleJobs at phase boundaries (tests only; nil in production).
	TestHookLoadSchedulingPhase func(phase string)

	// TestHookAfterImmediateLoadChunk runs after each immediate-load chunk completes (outside jobsMu), before an
	// inter-chunk pause when batching is enabled (tests only; nil in production).
	TestHookAfterImmediateLoadChunk func(chunkIndex int)

	// TestHookEnvelopeTickTargetForJobType, when non-nil, supplies the follow-up scheduler_job id for that job_type
	// during [DispatchEnvelopeTickResolvedJobs] only; return empty string to fall back to [firstEnabledTriggerableJobIDForJobType]
	// (tests only; nil in production).
	TestHookEnvelopeTickTargetForJobType func(jobType string) string
}

// lockFailureSkipThreshold: after this many consecutive lock failures, skip execution and stop logging the error.
const lockFailureSkipThreshold = 3

// lockFailureDisableThreshold: after this many consecutive lock failures, disable the job in storage so it drops out of the process flow.
const lockFailureDisableThreshold = 5

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
	fn := func(ctx context.Context) error {
		s.executeJob(ctx, work.job, work.handler)
		if work.cancel != nil {
			work.cancel()
		}
		return nil
	}
	if work.nonBlocking {
		return pool.SubmitNonBlocking(work.ctx, fn)
	}
	return pool.Submit(work.ctx, fn)
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

	// Initialize metrics pipeline for sampler-based event batching (ITEM-912).
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

	// Wire audit metrics collector to MetricPipeline (ITEM-912 Phase 2)
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
	ingestSvc := ambience.NewAmbientIngestService(projectRoot, secCtx)
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
		},
		secCtx:                    secCtx,
		notificationContext:       notificationContext,
		executor:                  &NativeExecutor{},
		router:                    router,
		asyncRouter:               asyncRouter,
		metrics:                   metricsCollector,
		handlerFactory:            handlerFactory,
		jobLoader:                 jobLoader,
		processGroupManager:       processGroupManager,
		objectIDCacheBuilder:      objectIDCacheBuilder,
		ambienceMesh:              ambienceMesh,
		anticipatoryEngine:        fseventsEngine,
		packageConcurrencyLimiter: circuitbreaker.NewConcurrencyLimiter(logger, 30*time.Minute),
		lockFailureCountByJobID:   make(map[string]int),
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

	// Register globally for lifecycle hooks
	RegisterGlobalScheduler(sched)

	// Wire up storage layer getter to avoid circular dependency
	// Use package-level function from storage package
	storagepkg.SetGlobalSchedulerGetter(func() any {
		return GetGlobalScheduler()
	})

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
			SchedulerDaemonLog(s.logger).Info(fmt.Sprintf("Cleaned up %d stale scheduler lock files on startup", cleaned)).Log()
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
			s.coordinationChannel.WatchEvents(ctx)
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
		// SCH-007 is submitted once at startup via bootstrapCriticalTimerJobsOnDaemonStart (priority pool);
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
	return nil
}

// Stop stops the scheduler daemon
func (s *Scheduler) Stop() {
	stopTime := time.Now()

	// Check permission to manage scheduler
	if err := s.checkPermission("manage:scheduler"); err != nil {
		SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonPermissionDeniedStop).
			WithError(err).
			Log()
		return
	}

	var isRunning bool
	_ = concurrency.RunInLockWithLogger(
		&s.runningMu,
		LockNameSchedulerStopCheck,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			isRunning = s.running
			if s.running {
				s.running = false
			}
			return nil
		},
	)

	if !isRunning {
		return
	}

	SchedulerDaemonLog(s.logger).Info(LogEventSchedulerDaemonStopping).Log()

	if s.anticipatoryEngine != nil {
		if err := s.anticipatoryEngine.Stop(); err != nil {
			SchedulerDaemonLog(s.logger).Warn("Failed to stop anticipatory engine").
				WithError(err).
				Log()
		} else {
			SchedulerDaemonLog(s.logger).Info("Anticipatory engine stopped successfully").Log()
		}
	}

	// Mark any currently running job as abandoned so events files have a final outcome (not just "started").
	// Collect IDs under lock (no I/O); write log entries outside lock to avoid holding mutex during I/O.
	var runningJobIDs []string
	s.jobsMu.RLock()
	for _, job := range s.jobs {
		job.RunningMu.RLock()
		running := job.Running
		job.RunningMu.RUnlock()
		if running {
			runningJobIDs = append(runningJobIDs, job.ID)
		}
	}
	s.jobsMu.RUnlock()
	stopTimeStr := zqktime.FormatRFC3339UTC(stopTime)
	for _, jobID := range runningJobIDs {
		writeJobLogEntry(s.projectRoot, jobID, map[string]any{
			KeyEventType: JobLogEventAbandoned,
			KeyJobID:     jobID,
			KeyTimestamp: stopTimeStr,
		})
	}
	if s.projectRoot != emptyValue {
		CloseAllJobLogWriters(s.projectRoot)
	}

	// Stop metric sampling (flush tickers + pending batches) before cron/storage teardown so shutdown
	// does not contend with WAL/hash drain and does not strand flushTickerLoop goroutines until process exit.
	const metricsSamplerStopBudget = 4 * time.Second
	if s.samplingPipeline != nil {
		reg := s.samplingPipeline.GetSamplerRegistry()
		if reg != nil {
			done := make(chan struct{})
			goroutinelabels.NewGoroutine("scheduler", "stop metrics samplers").StartSimple(func() {
				defer close(done)
				_ = reg.StopAll() //nolint:errcheck // best-effort; continue shutdown regardless
			})
			select {
			case <-done:
			case <-time.After(metricsSamplerStopBudget):
				SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonMetricsSamplerStopTimedOut).
					String("budget", metricsSamplerStopBudget.String()).
					Log()
			}
		}
	}

	// Remove keep-alive file FIRST (before stopping cron) to ensure cleanup happens
	// even if process is killed during shutdown
	if s.projectRoot != emptyValue {
		if err := removeKeepAlive(s.projectRoot); err != nil {
			SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonKeepAliveRemoveFailed).
				ProjectRoot(s.projectRoot).
				WithError(err).
				Log()
			// Don't fail shutdown - keep-alive cleanup is best effort
		}
	}

	// Stop cron scheduler with timeout to prevent hanging
	stopCtx := s.cron.Stop()
	select {
	case <-stopCtx.Done():
		// Cron stopped successfully
	case <-time.After(2 * time.Second):
		// Timeout - log warning but continue shutdown
		SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonCronStopTimedOut).Log()
	}

	// Stop priority pool first so critical jobs can finish, then general triggered-job pool (releases budget slots).
	// Use bounded wait so shutdown completes even if in-flight jobs don't respect context cancellation promptly.
	stopTriggeredPool := func(p *goroutinelabels.Pool, name string) {
		if p == nil {
			return
		}
		poolDone := make(chan struct{})
		poolStopBud := goroutinelabels.DefaultBudget()
		poolStopBuilder := goroutinelabels.NewGoroutine("scheduler_triggered_pool_stop", name)
		if poolStopBud != nil {
			poolStopBuilder = poolStopBuilder.WithBudget(poolStopBud)
		}
		poolStopBuilder.WithCleanup(func() { close(poolDone) }).StartSimple(func() { p.Stop() })
		select {
		case <-poolDone:
		case <-time.After(5 * time.Second):
			SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonTriggeredPoolStopTimedOut).Log()
		}
	}
	stopTriggeredPool(s.triggeredPriorityPool, "stopping priority triggered job pool during shutdown")
	s.triggeredPriorityPool = nil
	stopTriggeredPool(s.triggeredPool, "stopping triggered job pool during shutdown")
	s.triggeredPool = nil

	// Stop async router if running
	if s.asyncRouter != nil {
		if err := s.asyncRouter.Stop(); err != nil {
			SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonAsyncRouterStopFailed).
				WithError(err).
				Log()
		}
	}

	// CRITICAL: Shutdown process group manager to control all tracked subprocesses
	// This ensures all spawned processes are properly terminated
	// Timeout is configured in NewProcessGroupManager (3 seconds)
	if s.processGroupManager != nil {
		if err := s.processGroupManager.Shutdown("scheduler daemon stopped"); err != nil {
			SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonProcessGroupShutdownIssues).
				WithError(err).
				Log()
		}
	}

	// Stop notification context
	if s.notificationContext != nil {
		s.notificationContext.Stop()
	}

	// Stop activity cache writer goroutine (cleanup)
	cache := GetGlobalActivityCache()
	if cache != nil {
		cache.Stop()
	}

	// Drain all queues registered with the global shutdown coordinator (hash registry, IO queue,
	// audit buffers, async validation strategies, etc.). InitiateShutdown alone only signals
	// stop-accepting; Drain runs each handler's Drain() with the coordinator timeout (default 30s).
	// DrainAll is idempotent per coordinator instance (sync.Once), so concurrent callers are safe.
	shutdownCoordinator := storagepkg.GetGlobalShutdownCoordinator()
	if shutdownCoordinator != nil {
		const schedulerQueueDrainBudget = 45 * time.Second
		drainCtx, drainCancel := context.WithTimeout(pkgctx.NewSystemContext(), schedulerQueueDrainBudget)
		if err := shutdownCoordinator.DrainAll(drainCtx); err != nil {
			SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonQueueShutdownDrainIncomplete).
				WithError(err).
				String("budget", schedulerQueueDrainBudget.String()).
				Log()
		}
		drainCancel()
	}

	// Determinism: also shut down the concrete storage instance so storage-local background
	// workers (notably the ObjectWriteBehindWorker WAL replay/apply loop) stop promptly.
	// Relying only on the global shutdown coordinator cancels some workers, but the write-behind
	// worker is stopCh-driven and may otherwise continue WAL replay during a "stop --wait".
	type storageShutdowner interface {
		Shutdown(context.Context) error
	}
	if s.storage != nil {
		if sd, ok := s.storage.(storageShutdowner); ok {
			stopCtx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 8*time.Second)
			defer cancel()
			if err := sd.Shutdown(stopCtx); err != nil {
				SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonStorageShutdownFailed).
					WithError(err).
					Log()
			}
		}
	}

	// Record metrics
	if s.metrics != nil {
		s.metrics.RecordSchedulerStop(time.Since(stopTime))
	}

	// Remove PID file on shutdown
	if s.projectRoot != emptyValue {
		if err := removePIDFile(s.projectRoot); err != nil {
			SchedulerDaemonLog(s.logger).Warn(LogEventSchedulerDaemonPidRemoveFailed).
				ProjectRoot(s.projectRoot).
				WithError(err).
				Log()
			// Don't fail shutdown - PID file cleanup is best effort
		}
	}

	// Update state via callback (not defer) for predictable execution order
	_ = concurrency.RunInLockWithLogger(
		&s.runningMu,
		LockNameSchedulerStopCheck,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			s.running = false
			return nil
		},
	)
	SchedulerDaemonLog(s.logger).Info(LogEventSchedulerDaemonStopped).Log()
}

// IsRunning returns whether the scheduler is running
func (s *Scheduler) IsRunning() bool {
	var isRunning bool
	_ = concurrency.RunInRLockWithLogger(
		&s.runningMu,
		LockNameSchedulerIsRunning,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			isRunning = s.running
			return nil
		},
	)
	return isRunning
}

// GetMetricsSnapshot returns a point-in-time snapshot of scheduler metrics.
// Returns a zero-valued snapshot if the scheduler has no metrics collector.
// Used by dashboard/report commands (e.g. health-data, scheduler activity).
func (s *Scheduler) GetMetricsSnapshot() SchedulerMetricsSnapshot {
	if s.metrics == nil {
		return SchedulerMetricsSnapshot{}
	}
	return s.metrics.GetMetrics()
}

// GetRunningJobIDs returns job IDs currently executing (from conflict manager).
// Only populated when scheduler runs in this process; empty when daemon is in another process.
func (s *Scheduler) GetRunningJobIDs() []string {
	if s.conflictMgr == nil {
		return nil
	}
	return s.conflictMgr.GetRunningJobIDs()
}

// loadAndScheduleJobs, scheduleTimerJob, scheduleImmediateJob, registerTriggeredJob,
// createJobHandler, createHandlerWithStorage, GetAsyncRouter are defined in job_management.go
// executeJob, updateJobInStorage, and disableJobInStorage are defined in job_execution.go

// handleCoordinationEvents watches for coordination events and handles drift detection.
func (s *Scheduler) handleCoordinationEvents(ctx context.Context) {
	if s.coordinationChannel == nil {
		return
	}
	ch := s.coordinationChannel.Subscribe()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			switch ev.Type {
			case "drift_detected":
				if ev.JobID != "" {
					if job, ok := s.GetJob(ev.JobID); ok {
						SchedulerDaemonLog(s.logger).Warn("Pausing errant job due to drift").
							String("job_id", ev.JobID).
							Int("process_id", ev.ProcessID).
							Log()

						job.RunningMu.RLock()
						cancel := job.executionCancel
						job.RunningMu.RUnlock()
						if cancel != nil {
							cancel()
						}

						// Enqueue trigger request for respawn
						_ = s.TriggerJob(ctx, job.ID)
					}
				}
			case "process_started":
				if s.notificationContext != nil {
					notif := CreateJobNotification(ev.JobID, "subagent", "swarm", notificationEventStatusUpdate, PriorityLow, 0, nil, ev.Metadata)
					notif.Title = "Subagent Task Started"
					notif.Message = fmt.Sprintf("Subagent task %s has started execution in the swarm.", ev.JobID)
					s.notificationContext.Notify(notif)
				}
			case "process_completed":
				if s.notificationContext != nil {
					notif := CreateJobNotification(ev.JobID, "subagent", "swarm", notificationEventCompleted, PriorityMedium, 0, nil, ev.Metadata)
					notif.Title = "Subagent Task Completed"
					notif.Message = fmt.Sprintf("Subagent task %s completed successfully.", ev.JobID)
					s.notificationContext.Notify(notif)
				}
			case "process_errored":
				if s.notificationContext != nil {
					var execErr error
					if errMsg, ok := ev.Metadata["error"].(string); ok && errMsg != "" {
						execErr = fmt.Errorf("%s", errMsg)
					}
					notif := CreateJobNotification(ev.JobID, "subagent", "swarm", notificationEventFailed, PriorityHigh, 0, execErr, ev.Metadata)
					notif.Title = "Subagent Task Failed"
					notif.Message = fmt.Sprintf("Subagent task %s has failed.", ev.JobID)
					s.notificationContext.Notify(notif)
				}
			}
		}
	}
}
