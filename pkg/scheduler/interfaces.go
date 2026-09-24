package scheduler

import (
	"context"
	"io"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/observability"
)

// AuditAggregationHandlerInterface defines the interface for audit event aggregation.
type AuditAggregationHandlerInterface interface {
	JobHandler
}

// ChangeJournalAggregationHandlerInterface defines the interface for change journal aggregation.
type ChangeJournalAggregationHandlerInterface interface {
	JobHandler
}

// AggregationMetricsCleanupHandlerInterface defines the interface for metrics cleanup.
type AggregationMetricsCleanupHandlerInterface interface {
	JobHandler
}

// GenericMetricsCleanupHandlerInterface defines the interface for metrics cleanup.
type GenericMetricsCleanupHandlerInterface interface {
	JobHandler
}

// CachePrewarmHandlerInterface defines the interface for cache prewarming.
type CachePrewarmHandlerInterface interface {
	JobHandler
	WithValidationScanner(scanner ValidationScanner) CachePrewarmHandlerInterface
}

// CacheInvalidationHandlerInterface defines the interface for cache invalidation.
type CacheInvalidationHandlerInterface interface {
	JobHandler
}

// RetentionToleranceHandlerInterface defines the interface for retention tolerance checks.
type RetentionToleranceHandlerInterface interface {
	JobHandler
	SetProgressFunc(fn ProgressFunc)
}

// SchedulerJobRetentionHandlerInterface defines the interface for scheduler job retention.
type SchedulerJobRetentionHandlerInterface interface {
	JobHandler
}

// AutofixBatchCleanupHandlerInterface defines the interface for autofix batch cleanup.
type AutofixBatchCleanupHandlerInterface interface {
	JobHandler
}

// LogRotatorHandlerInterface defines the interface for log rotation.
type LogRotatorHandlerInterface interface {
	JobHandler
}

// CleanupConfigHandlerInterface defines the interface for cleanup configuration.
type CleanupConfigHandlerInterface interface {
	JobHandler
}

// LifecycleCheckHandlerInterface defines the interface for lifecycle checks.
type LifecycleCheckHandlerInterface interface {
	JobHandler
}

// IntegrityCheckHandlerInterface defines the interface for integrity checks.
type IntegrityCheckHandlerInterface interface {
	JobHandler
}

// ObjectValidationHandlerInterface defines the interface for object validation.
type ObjectValidationHandlerInterface interface {
	JobHandler
}

// OperationExecutionHandlerInterface defines the interface for operation execution.
type OperationExecutionHandlerInterface interface {
	JobHandler
}

// CallbackListenerHandlerInterface defines the interface for callback listening.
type CallbackListenerHandlerInterface interface {
	JobHandler
}

// TestIOHandlerInterface defines the interface for test I/O operations.
type TestIOHandlerInterface interface {
	JobHandler
}

// RunWrapperHandlerInterface defines the interface for run wrapper operations.
type RunWrapperHandlerInterface interface {
	JobHandler
	BindScheduler(s SchedulerInterface)
	StopNotificationContext()
}

// ContextRefreshHandlerInterface defines the interface for context refresh operations.
type ContextRefreshHandlerInterface interface {
	JobHandler
}

// CascadeUpdateHandlerInterface defines the interface for cascade updates.
type CascadeUpdateHandlerInterface interface {
	JobHandler
}

// CapacityScalingHandlerInterface defines the interface for capacity scaling.
type CapacityScalingHandlerInterface interface {
	JobHandler
}

// MeshLeaseSupervisionHandlerInterface defines the interface for mesh lease supervision.
type MeshLeaseSupervisionHandlerInterface interface {
	JobHandler
}

// StrategicPulseHandlerInterface defines the interface for strategic pulses.
type StrategicPulseHandlerInterface interface {
	JobHandler
}

// ConvergenceSessionTickHandlerInterface defines the interface for convergence session ticks.
type ConvergenceSessionTickHandlerInterface interface {
	JobHandler
	MaybeRunOrchestrateRollupAfterTick(ctx context.Context, job *ScheduledJob, sessionID string)
}

// DataCellEnvelopeTickHandlerInterface defines the interface for datacell envelope ticks.
type DataCellEnvelopeTickHandlerInterface interface {
	JobHandler
}

// JobLockInterface defines the job locking operations.
type JobLockInterface interface {
	Acquire() error
	TryAcquire() (bool, error)
	Release() error
	Close() error
	IsLocked() bool
	AcquiredAt() *time.Time
}

// JobTriggerQueueInterface defines the job trigger queue operations.
type JobTriggerQueueInterface interface {
	EnqueueTriggerRequest(jobID string) error
	EnqueueTriggerRequestWithOrigin(jobID, triggerOrigin string) error
	EnqueueLifecycleTrigger(kind, fromState, toState string, objectData map[string]any) error
	EnqueueTriggerRequests(jobIDs []string, triggerOrigin string) error
	EnqueueTriggerRequestStructs(toAppend []JobTriggerRequest) error
	PeekTriggerRequests() ([]JobTriggerRequest, error)
	DequeueTriggerRequests(limit int) ([]JobTriggerRequest, error)
	HasPendingTriggerWithOrigin(jobID, triggerOrigin string) (bool, error)
	WatchTriggerQueue(ctx context.Context, scheduler *Scheduler)
}

// CoordinationChannelInterface defines the coordination channel operations.
type CoordinationChannelInterface interface {
	PublishEvent(ev Event) error
	Subscribe() <-chan Event
	WatchEvents(ctx context.Context) error
}

// NotificationContextInterface defines the notification context operations.
// NotificationRouter added for targeted notification routing
type NotificationRouter interface {
	RouteNotificationTarget(target string, n *JobNotification) error
}

type NotificationContextInterface interface {
	Notify(n *JobNotification)
	ShouldNotify(n *JobNotification) bool
	Suppress(jobID string, duration time.Duration)
	GetHistory(limit int) []*JobNotification
	GetUnacknowledged() []*JobNotification
	Acknowledge(notifID string)
	TerminalChannel() <-chan *JobNotification
	DesktopChannel() <-chan *JobNotification
	EventChannel() <-chan *JobNotification
	SetMinPriority(priority NotificationPriority)
	SetEnabled(enabled bool)
	SetCategories(categories []string)
	Stop()
}

// NotificationDisplayInterface defines the notification display operations.
type NotificationDisplayInterface interface {
	Display(notif *JobNotification)
	DisplayTerminalNotification(notif *JobNotification)
	DisplayDesktopNotification(notif *JobNotification)
}

// LogRotatorInterface defines the log rotator operations.
type LogRotatorInterface interface {
	Rotate(ctx context.Context, retention time.Duration) error
}

// NegotiationLoopInterface defines the negotiation loop operations.
type NegotiationLoopInterface interface {
	Execute(ctx context.Context, initialProposal Proposal) (*NegotiationResult, error)
}

// ActivityCacheInterface defines the activity cache operations.
type ActivityCacheInterface interface {
	GetEntry(jobID string) (*ActivityCacheEntry, bool)
	GetAllEntries() map[string]*ActivityCacheEntry
	UpdateEvent(jobID, eventType string, duration time.Duration, err error)
	LoadCache(projectRoot string) error
	SaveCache(projectRoot string) error
	GetMetadata() *ActivityCacheMetadata
	Stop()
}

// PackageConcurrencyLimiterInterface defines the package concurrency limiter operations.

// StewardEnqueueCoordinatorInterface defines the steward enqueue coordinator operations.
type StewardEnqueueCoordinatorInterface interface {
	Enqueue(ctx context.Context, op datacell.MaintenanceOp) error
}

// SchedulerInterface defines the core scheduler operations.
type SchedulerInterface interface {
	Start(ctx context.Context) error
	Stop()
	IsRunning() bool
	RegisterJob(job *ScheduledJob) error
	TriggerJob(ctx context.Context, jobID string) error
	TriggerJobByEvent(ctx context.Context, eventType, eventKind string, eventData map[string]any) error
	TriggerJobWithCallback(ctx context.Context, jobID string, opCallback concurrency.OperationCallback) error
	GetJob(jobID string) (*ScheduledJob, bool)
	ListJobs() []*ScheduledJob
	ReloadJobs(ctx context.Context) error
	LoadAndScheduleJobs(ctx context.Context, reload bool) error
	DisableJobInStorage(ctx context.Context, job *ScheduledJob)
	GetExecutor() CommandExecutor
	GetNotificationContext() NotificationContextInterface
	SetSecurityContext(secCtx *pkgctx.SecurityContext)
	SetValidationScanner(scanner ValidationScanner)
	SetOnlyManualJobs(only bool)
	SetTriggerQueueEventWriter(w io.Writer)
	StartConvergenceEngine(ctx context.Context) (stop func(context.Context))
	TouchActivity()
	GetLastActivity() time.Time
	DispatchEnvelopeTickResolvedJobs(ctx context.Context, envelopeJob *ScheduledJob, resolvedJobTypes []string) EnvelopeTickDispatchOutcome
	EvaluateActiveConvergenceSessions(ctx context.Context)
	EvaluateStaleConvergenceSessions(ctx context.Context)
	GetRunningJobIDs() []string
}

// PolicyEngineInterface defines the policy engine operations.
type PolicyEngineInterface interface {
	Evaluate(jobID string, job *ScheduledJob) (*ExecutionDecision, error)
	Reload() error
	SavePolicy(policy *ExecutionPolicy) error
}

// NegotiatorInterface defines the negotiator operations.
type NegotiatorInterface interface {
	Propose(ctx context.Context, p Proposal) (*NegotiationResult, error)
}

// JobStateRegistryInterface defines the job state registry operations.
type JobStateRegistryInterface interface {
	GetExecutionState(jobID string) (*JobExecutionState, error)
	UpdateState(jobID string, state *JobExecutionState) error
	ListInProgress() ([]*JobExecutionState, error)
	RegisterExecution(jobID, executionID string, processID int) error
	RegisterExecutionWithLease(jobID, executionID string, processID int, maxRuntimeSeconds int) (*JobExecutionLease, error)
	GetActiveLease(jobID string) (*JobExecutionLease, error)
	CompleteExecution(jobID, executionID, result string) error
	DeferExecution(jobID, reason string, deferUntil *time.Time) error
	SetObservabilityRecorder(rec observability.Recorder)
	MigrateLegacyFlatStateFilesBestEffort() (int, error)
	MigrateUnbucketedJobStateDirsBestEffort() (int, error)
	Summarize(staleAfter time.Duration) (*JobStateSummary, error)
	CleanStaleLocks(staleAge time.Duration) (int, error)
}

// HandlerFactoryInterface defines the handler factory operations.
type HandlerFactoryInterface interface {
	CreateHandler(job *ScheduledJob) JobHandler
	BindScheduler(s SchedulerInterface)
	SetValidationScanner(scanner ValidationScanner)
}

// JobLoaderInterface defines the job loader operations.
type JobLoaderInterface interface {
	LoadJobs(ctx context.Context) ([]map[string]any, error)
	RefreshEnvironmentVariablesFromRaw(job *ScheduledJob, rawJob map[string]any)
	HydrateJob(result map[string]any) (*ScheduledJob, error)
}

// ProcessGroupManagerInterface defines the process group manager operations.
type ProcessGroupManagerInterface interface {
	GetShutdownContext() context.Context
	IsShuttingDown() bool
	RegisterSubprocess(id, jobID, description string, processID, processGroupID int, critical bool, killFunc func() error)
	UnregisterSubprocess(id string)
	Shutdown(reason string) error
	GetStatus() ProcessGroupStatus
}
