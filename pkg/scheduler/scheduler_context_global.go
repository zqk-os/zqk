package scheduler

import (
	"strconv"
	"strings"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"context"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/ambience"
	"github.com/lanceman/zqk/pkg/circuitbreaker"
	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/config"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/metrics"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/scheduler/transceiver"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
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
)

type evtDataKey struct{}

func WithEventData(ctx context.Context, data map[string]any) context.Context {
	return context.WithValue(ctx, evtDataKey{}, data)
}

// secJobCtxKey is the context key for operation-execution security context.
type secJobCtxKey struct{}

func getSchedulerGoroutineCap() int {
	v := config.SchedulerGoroutineCap().OrDefault(defaultSchedulerGoroutineCap)
	if v <= 0 {
		return defaultSchedulerGoroutineCap
	}
	if v < minSchedulerGoroutineCap {
		return minSchedulerGoroutineCap
	}
	if v > maxSchedulerGoroutineCap {
		return maxSchedulerGoroutineCap
	}
	return v
}

func getSchedulerTriggeredPoolSize() int {
	v := config.SchedulerTriggeredPoolSize().OrDefault(defaultSchedulerTriggeredPool)
	if v <= 0 {
		return defaultSchedulerTriggeredPool
	}
	if v < minSchedulerTriggeredPoolSize {
		return minSchedulerTriggeredPoolSize
	}
	if v > maxSchedulerTriggeredPoolSize {
		return maxSchedulerTriggeredPoolSize
	}
	return v
}

// getSchedulerImmediateLoadBatchSize is how many immediate jobs to schedule per jobsMu acquisition
// during loadAndScheduleJobs pass 2 (initial load / full reload). 0 means no chunking: one lock for all immediates.
// Default to 100 if not specified to reduce lock contention and prevent startup hangs.
func getSchedulerImmediateLoadBatchSize() int {
	var v int
	envStr := zqkenv.SchedulerImmediateLoadBatchSize().Get()
	if envStr != "" {
		trimmed := strings.TrimSpace(envStr)
		if trimmed != "" {
			if parsed, err := strconv.Atoi(trimmed); err == nil {
				v = parsed
			}
		} else {
			v = 100 // default for whitespace
		}
	} else {
		v = config.SchedulerImmediateLoadBatchSize().OrDefault(100)
	}

	if v < 0 {
		return 0
	}
	if v > maxSchedulerImmediateLoadBatchSize {
		return maxSchedulerImmediateLoadBatchSize
	}
	return v
}

// getSchedulerImmediateLoadBatchPause is the pause between immediate batches when chunking is enabled.
func getSchedulerImmediateLoadBatchPause() time.Duration {
	v := config.SchedulerImmediateLoadBatchPause().OrDefault("0s")
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

// testAdmissionTimeoutNS when non-zero overrides [getSchedulerAdmissionTimeout] (tests only).
var testAdmissionTimeoutNS atomic.Int64

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
	v := config.SchedulerDispatchResourceWaitMax().OrDefault("2h")
	d, err := time.ParseDuration(v)
	if err != nil || d < minMax {
		return defaultMax
	}
	if d > maxMax {
		return maxMax
	}
	return d
}

// getSchedulerAdmissionTimeout is how long a callback-bearing one_time immediate job may wait
// to *start* before the error callback fires and the job is disabled (default 3m).
// Distinct from [getSchedulerDispatchResourceWaitMax] (pool/ceiling wait, default 2h).
// Jobs already accepted onto a worker pool are not timed out here (see scanAdmissionTimeouts).
func getSchedulerAdmissionTimeout() time.Duration {
	if ns := testAdmissionTimeoutNS.Load(); ns > 0 {
		return time.Duration(ns)
	}
	const (
		defaultMax = 3 * time.Minute
		minMax     = 10 * time.Second
		maxMax     = 10 * time.Minute
	)
	v := config.SchedulerAdmissionTimeout().OrDefault("3m")
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
	n := config.SchedulerDefaultPackageConcurrency().OrDefault(1)
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

// TriggerOriginCLISubmit marks file-queue triggers from `zqk scheduler submit` (callback-bearing one-shots).
const TriggerOriginCLISubmit = "cli_submit"

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

	// Scheduler coordination kernel primitives (PRI-220 / BLI-901 / CRIT-9040).
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
	objectIDCacheBuilder ObjectIDCacheBuilder // optional; for cache_prewarm (BLI-954)

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

	// dispatchDropRetriesByJobID tracks consecutive dispatch drops (e.g. concurrency limits) per job for throttled re-enqueue.
	dispatchDropRetriesByJobID          map[string]int
	dispatchDropRetriesMu               sync.Mutex
	testHookDispatchDropBackoffDuration func(retries int) time.Duration

	// lastActivityNanos is updated when the scheduler does meaningful work (job completed, trigger processed).
	// Used by the idle watchdog when parent is not zqk to shut down after prolonged inactivity.
	lastActivityNanos atomic.Int64

	// triggerQueueEventWriter, when set by the daemon, receives JSONL trigger-queue events (dequeued, processing, failed)
	// so diagnostics.jsonl has a paper trail for SCH-AUTOFIX-* and other enqueued triggers.
	triggerQueueEventWriter io.Writer
	triggerQueueEventMu     sync.Mutex

	// samplingPipeline is the BLI-912 MetricPipeline used for sampled coordinator/audit events (distinct from [Scheduler.metrics] collector).
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

	// immediateDispatchPending tracks one_time immediate jobs accepted onto a worker pool but not yet
	// started (LastRunAt unset). Reload must not re-submit these (pool flood) but may start never-queued ones.
	immediateDispatchPending sync.Map // jobID -> struct{}

	// admissionFailed records jobs whose admission hourglass already invoked callback_on_error
	// so a late pool start cannot double-run or double-callback.
	admissionFailed sync.Map // jobID -> struct{}
}

// lockFailureSkipThreshold: after this many consecutive lock failures, skip execution and stop logging the error.
const lockFailureSkipThreshold = 3
