package scheduler

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/observability"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/when"
)

// SchedulerMetricsCollector is the interface for collecting scheduler metrics
// Implementations can send metrics to Prometheus, StatsD, custom backends, etc.
// Aligned with graph provider metrics patterns for uniformity
type SchedulerMetricsCollector interface {
	// Job lifecycle metrics
	RecordJobLoaded(jobID, jobType, triggerType string)
	RecordJobScheduled(jobID, jobType, triggerType string)
	RecordJobExecutionStarted(jobID, jobType string)
	RecordJobExecutionCompleted(jobID, jobType string, duration time.Duration, success bool)
	RecordJobExecutionFailed(jobID, jobType string, duration time.Duration, err error)

	// Handler metrics
	RecordHandlerCreated(jobType string)
	RecordHandlerCreationFailed(jobType string, err error)

	// Trigger validation metrics
	RecordTriggerValidation(jobID, triggerType string, valid bool, reason string)

	// Scheduling metrics
	RecordScheduleAttempt(jobID, triggerType string, success bool)
	RecordScheduleError(jobID, triggerType string, err error)

	// Conflict metrics
	RecordConflictCheck(jobID string, allowed bool)
	RecordConflictDetected(jobID, jobType string)

	// Health metrics
	RecordSchedulerStart(duration time.Duration)
	RecordSchedulerStop(duration time.Duration)
	RecordJobLoadError(err error)

	// Pool/budget metrics (goroutine pool creation declined, fallback used)
	RecordPoolCreationDeclined(poolName, purpose, reason string)

	// Trigger-queue metrics (cross-process enqueue/dequeue; surfaces silent failures)
	RecordTriggerQueueDequeued(count int)
	RecordTriggerQueueTriggerFailed(jobID string, err error)
	RecordTriggerQueueReloadRetry(jobID string)
	RecordTriggerQueueReloadFailed(jobID string, err error)

	// Dispatch pressure: job was not started because dispatch wait budget was exhausted (ceiling/pool submit).
	RecordDispatchPressureDropped(source, reason string)

	// GetObservabilityRecorder returns the recorder used for scheduler metrics (never nil).
	GetObservabilityRecorder() observability.Recorder

	// Get metrics snapshot
	GetMetrics() SchedulerMetricsSnapshot

	// SetTSDBProvider sets the optional TSDBProvider for metrics persistence
	SetTSDBProvider(tsdb any)
}

// Snapshot group types: logical groupings so property names stay short and non-repetitive.

// JobMetrics is the job lifecycle subset of a metrics snapshot.
type JobMetrics struct {
	Loaded, Scheduled, Executed, Succeeded, Failed int64
}

// HandlerMetrics is the handler creation subset of a metrics snapshot.
type HandlerMetrics struct {
	Created, CreationErrors int64
}

// TriggerMetrics is the trigger validation subset of a metrics snapshot.
type TriggerMetrics struct {
	Validations, ValidationErrors int64
}

// ScheduleMetrics is the scheduling attempt subset of a metrics snapshot.
type ScheduleMetrics struct {
	Attempts, Successes, Errors int64
}

// ConflictMetrics is the conflict check subset of a metrics snapshot.
type ConflictMetrics struct {
	Checks, Detected int64
}

// LifecycleMetrics is the scheduler start/stop and load-error subset of a metrics snapshot.
type LifecycleMetrics struct {
	SchedulerStarts, SchedulerStops, JobLoadErrors int64
}

// PoolMetrics is the pool/budget fallback subset of a metrics snapshot.
type PoolMetrics struct {
	CreationDeclined int64
}

// TriggerQueueMetrics is the daemon trigger-queue subset of a metrics snapshot.
type TriggerQueueMetrics struct {
	Dequeued, TriggerFailed, ReloadRetries, ReloadFailed int64
}

// DispatchPressureMetrics counts dispatch attempts abandoned before the handler ran (resource wait
// budget exhausted). Totals are monotonic for the daemon process lifetime; use for trends and health-data.
type DispatchPressureMetrics struct {
	Total int64
	Cron  int64
	// MissedJobRecovery is recoverMissedJob path (goroutine ceiling / dispatch deadline).
	MissedJobRecovery int64
	// TriggerJobSubmit is manual TriggerJob blocked on pool submit until dispatch deadline.
	TriggerJobSubmit int64
	// ImmediateJobSubmit is scheduleImmediateJob pool submit blocked until dispatch deadline.
	ImmediateJobSubmit int64
	// Other is sources not recognized by the switch (forward compatibility).
	Other int64
}

// TimingMetrics is the execution timing subset of a metrics snapshot.
type TimingMetrics struct {
	TotalExecutionTime, AvgExecutionTime, MaxExecutionTime, MinExecutionTime time.Duration
}

// RecentJobOutcomes holds the last N job IDs for each outcome so callers can see which jobs
// fired (started/completed) and which hit conflict without guessing from logs.
const maxRecentJobIDs = 64

// SchedulerMetricsSnapshot provides a point-in-time view of scheduler metrics.
// Grouped structs keep names short and avoid repetition (e.g. Jobs.Loaded instead of JobsLoaded).
// Recent* slices are the last N job IDs (newest first) for test and CLI assertion of which jobs ran.
type SchedulerMetricsSnapshot struct {
	Timestamp        time.Time
	Jobs             JobMetrics
	Handlers         HandlerMetrics
	Trigger          TriggerMetrics
	Schedule         ScheduleMetrics
	Conflict         ConflictMetrics
	Lifecycle        LifecycleMetrics
	Pool             PoolMetrics
	TriggerQueue     TriggerQueueMetrics
	DispatchPressure DispatchPressureMetrics
	Timing           TimingMetrics

	// Per-job visibility: which job IDs had execution started, completed, or conflict (newest first, max maxRecentJobIDs each).
	RecentExecutionStarted   []string
	RecentExecutionCompleted []string
	RecentConflictDetected   []string
}

// SchedulerMetricsConfig controls scheduler metrics collection behavior
type SchedulerMetricsConfig struct {
	// Enabled enables or disables metrics collection entirely
	Enabled bool
}

// DefaultSchedulerMetricsConfig returns a default scheduler metrics configuration
func DefaultSchedulerMetricsConfig() SchedulerMetricsConfig {
	return SchedulerMetricsConfig{
		Enabled: true,
	}
}

// configDisabled returns true when metrics collection is disabled so Record* methods can exit early.
func (m *DefaultSchedulerMetricsCollector) configDisabled() bool {
	return !m.config.Enabled
}

// DisabledSchedulerMetricsConfig returns a config with metrics disabled
func DisabledSchedulerMetricsConfig() SchedulerMetricsConfig {
	return SchedulerMetricsConfig{
		Enabled: false,
	}
}

// DefaultSchedulerMetricsCollector provides in-memory metrics collection.
// Thread-safe, low overhead, suitable for most use cases.
// Counters are grouped so names stay short (e.g. jobs.Loaded instead of jobsLoaded).
type DefaultSchedulerMetricsCollector struct {
	mu sync.RWMutex

	config   SchedulerMetricsConfig
	recorder any // observability.Recorder - stored as any to avoid import cycles
	tsdb     any // storage.TSDBProvider - stored as any to avoid import cycles

	// Counters (grouped for clarity; atomics use &field)
	jobs         struct{ loaded, scheduled, executed, succeeded, failed int64 }
	handlers     struct{ created, creationErrors int64 }
	trigger      struct{ validations, validationErrors int64 }
	schedule     struct{ attempts, successes, errors int64 }
	conflict     struct{ checks, detected int64 }
	lifecycle    struct{ schedulerStarts, schedulerStops, jobLoadErrors int64 }
	pool         struct{ creationDeclined int64 }
	triggerQueue struct{ dequeued, triggerFailed, reloadRetries, reloadFailed int64 }
	// dispatchPressure: abandoned dispatch attempts (ceiling wait or blocked pool submit).
	dispatchPressure struct{ total, cron, missed, triggerSubmit, immediate, other int64 }

	// Timing (protected by mu in GetMetrics / RecordJobExecutionCompleted)
	timing struct {
		totalExecutionTime time.Duration
		executionCount     int64
		maxExecutionTime   time.Duration
		minExecutionTime   time.Duration
	}

	// Recent job IDs for snapshot (which fired / which conflict); protected by mu
	recentStarted   []string
	recentCompleted []string
	recentConflict  []string

	startTime time.Time
}

// NewDefaultSchedulerMetricsCollector creates a new default metrics collector
// NewDefaultSchedulerMetricsCollector creates a new default metrics collector
func NewDefaultSchedulerMetricsCollector() SchedulerMetricsCollector {
	return NewSchedulerMetricsCollectorWithConfig(DefaultSchedulerMetricsConfig())
}

// NewSchedulerMetricsCollectorWithConfig creates a new metrics collector with custom configuration
func NewSchedulerMetricsCollectorWithConfig(config SchedulerMetricsConfig) SchedulerMetricsCollector {
	m := &DefaultSchedulerMetricsCollector{
		config:    config,
		startTime: time.Now(),
	}
	m.timing.minExecutionTime = time.Hour // initialize to large value for min tracking
	return m
}

// SetTSDBProvider sets the optional TSDBProvider for metrics persistence.
func (m *DefaultSchedulerMetricsCollector) SetTSDBProvider(tsdb any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tsdb = tsdb
}

// RecordJobLoaded records a job being loaded
func (m *DefaultSchedulerMetricsCollector) RecordJobLoaded(jobID, jobType, triggerType string) {
	if m.configDisabled() {
		return
	}
	// Record using new builder-pattern API
	recorder := m.getSchedulerMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := buildJobLoadedMetric(jobID, jobType, triggerType)
		_ = recorder.Record("scheduler_job_loaded", builder)
	}

	atomic.AddInt64(&m.jobs.loaded, 1)
}

// RecordJobScheduled records a job being scheduled
func (m *DefaultSchedulerMetricsCollector) RecordJobScheduled(jobID, jobType, triggerType string) {
	if m.configDisabled() {
		return
	}
	// Record using new builder-pattern API
	recorder := m.getSchedulerMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := buildJobScheduledMetric(jobID, jobType, triggerType)
		_ = recorder.Record("scheduler_job_scheduled", builder)
	}

	atomic.AddInt64(&m.jobs.scheduled, 1)
}

// RecordJobExecutionStarted records a job execution starting
func (m *DefaultSchedulerMetricsCollector) RecordJobExecutionStarted(jobID, jobType string) {
	if m.configDisabled() {
		return
	}
	// Record using new builder-pattern API
	recorder := m.getSchedulerMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := buildJobExecutionStartedMetric(jobID, jobType)
		_ = recorder.Record("scheduler_job_execution_started", builder)
	}

	atomic.AddInt64(&m.jobs.executed, 1)
	m.appendRecentJobID(&m.recentStarted, jobID)
}

// RecordJobExecutionCompleted records a job execution completing
func (m *DefaultSchedulerMetricsCollector) RecordJobExecutionCompleted(jobID, jobType string, duration time.Duration, success bool) {
	if m.configDisabled() {
		return
	}
	// Record using new builder-pattern API
	recorder := m.getSchedulerMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		var err error
		if !success {
			err = errfmt.Errorf("job execution failed")
		}
		builder := buildJobExecutionMetric("execution_completed", jobID, jobType, duration, err)
		_ = recorder.Record("scheduler_job_execution_completed", builder)
	}

	m.mu.RLock()
	tsdb := m.tsdb
	m.mu.RUnlock()

	if tsdb != nil {
		if provider, ok := tsdb.(storage.TSDBProvider); ok {
			statusStr := "success"
			if !success {
				statusStr = "failed"
			}
			pt := storage.TSDBPoint{
				Measurement: "scheduler_job_execution",
				Tags: map[string]string{
					"job_id":                jobID,
					objects.FieldKeyJobType: jobType,
					objects.FieldKeyStatus:  statusStr,
				},
				Fields: map[string]any{
					objects.FieldKeyDurationSeconds: duration.Seconds(),
					"success":                       success,
				},
				Timestamp: time.Now().UTC(),
			}
			// Write asynchronously or inline? The interface is context-aware.
			// Since we're in metrics collection, we use a background context.
			_ = provider.WritePoint(pkgctx.NewSystemContext(), pt)
		}
	}

	// Update internal state for GetMetrics() snapshots
	_ = concurrency.RunInLockWithLogger(
		&m.mu, LockNameSchedulerMetricsRecordExecution, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			m.timing.totalExecutionTime += duration
			m.timing.executionCount++
			if duration > m.timing.maxExecutionTime {
				m.timing.maxExecutionTime = duration
			}
			if duration < m.timing.minExecutionTime {
				m.timing.minExecutionTime = duration
			}
			m.appendRecentJobIDUnlocked(&m.recentCompleted, jobID)
			return nil
		},
	)

	when.When(func() bool { return success }).Then(func() {
		atomic.AddInt64(&m.jobs.succeeded, 1)
	}).OrElse(func() {
		atomic.AddInt64(&m.jobs.failed, 1)
	}).Run()
}

// RecordJobExecutionFailed records a job execution failure
func (m *DefaultSchedulerMetricsCollector) RecordJobExecutionFailed(jobID, jobType string, duration time.Duration, err error) {
	if m.configDisabled() {
		return
	}
	// Record using new builder-pattern API
	recorder := m.getSchedulerMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := buildJobExecutionMetric("execution_failed", jobID, jobType, duration, err)
		_ = recorder.Record("scheduler_job_execution_failed", builder)
	}

	// Also record as completed (failed) for internal state tracking
	m.RecordJobExecutionCompleted(jobID, jobType, duration, false)
}

// RecordHandlerCreated records a handler being created
func (m *DefaultSchedulerMetricsCollector) RecordHandlerCreated(jobType string) {
	if m.configDisabled() {
		return
	}
	// Record using new builder-pattern API
	recorder := m.getSchedulerMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := buildHandlerCreatedMetric(jobType)
		_ = recorder.Record("scheduler_handler_created", builder)
	}

	atomic.AddInt64(&m.handlers.created, 1)
}

// RecordHandlerCreationFailed records a handler creation failure
func (m *DefaultSchedulerMetricsCollector) RecordHandlerCreationFailed(jobType string, err error) {
	if m.configDisabled() {
		return
	}
	// Record using new builder-pattern API
	recorder := m.getSchedulerMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := buildHandlerCreationFailedMetric(jobType, err)
		_ = recorder.Record("scheduler_handler_creation_failed", builder)
	}

	atomic.AddInt64(&m.handlers.creationErrors, 1)
}

// RecordTriggerValidation records a trigger validation
func (m *DefaultSchedulerMetricsCollector) RecordTriggerValidation(jobID, triggerType string, valid bool, reason string) {
	if m.configDisabled() {
		return
	}
	// Record using new builder-pattern API
	recorder := m.getSchedulerMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := buildTriggerValidationMetric(jobID, triggerType, valid, reason)
		_ = recorder.Record("scheduler_trigger_validation", builder)
	}

	atomic.AddInt64(&m.trigger.validations, 1)
	if !valid {
		atomic.AddInt64(&m.trigger.validationErrors, 1)
	}
}

// RecordScheduleAttempt records a schedule attempt
func (m *DefaultSchedulerMetricsCollector) RecordScheduleAttempt(jobID, triggerType string, success bool) {
	if m.configDisabled() {
		return
	}
	// Record using new builder-pattern API
	recorder := m.getSchedulerMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := buildScheduleAttemptMetric(jobID, triggerType, success)
		_ = recorder.Record("scheduler_schedule_attempt", builder)
	}

	atomic.AddInt64(&m.schedule.attempts, 1)
	when.When(func() bool { return success }).Then(func() {
		atomic.AddInt64(&m.schedule.successes, 1)
	}).OrElse(func() {
		atomic.AddInt64(&m.schedule.errors, 1)
	}).Run()
}

// RecordScheduleError records a schedule error
func (m *DefaultSchedulerMetricsCollector) RecordScheduleError(jobID, triggerType string, err error) {
	if m.configDisabled() {
		return
	}
	// Record using new builder-pattern API
	recorder := m.getSchedulerMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := buildScheduleErrorMetric(jobID, triggerType, err)
		_ = recorder.Record("scheduler_schedule_error", builder)
	}

	atomic.AddInt64(&m.schedule.errors, 1)
}

// RecordConflictCheck records a conflict check
func (m *DefaultSchedulerMetricsCollector) RecordConflictCheck(jobID string, allowed bool) {
	if m.configDisabled() {
		return
	}
	// Record using new builder-pattern API
	recorder := m.getSchedulerMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := buildConflictCheckMetric(jobID, allowed)
		_ = recorder.Record("scheduler_conflict_check", builder)
	}

	atomic.AddInt64(&m.conflict.checks, 1)
	if !allowed {
		atomic.AddInt64(&m.conflict.detected, 1)
	}
}

// appendRecentJobID appends jobID to slice (newest first), trims to maxRecentJobIDs. Holds mu.
func (m *DefaultSchedulerMetricsCollector) appendRecentJobID(slice *[]string, jobID string) {
	_ = concurrency.RunInLockWithLogger(
		&m.mu, LockNameSchedulerMetricsAppendRecent, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			m.appendRecentJobIDUnlocked(slice, jobID)
			return nil
		},
	)
}

// appendRecentJobIDUnlocked appends jobID to slice (newest first), trims to maxRecentJobIDs. Caller must hold m.mu.
func (m *DefaultSchedulerMetricsCollector) appendRecentJobIDUnlocked(slice *[]string, jobID string) {
	*slice = append([]string{jobID}, *slice...)
	if len(*slice) > maxRecentJobIDs {
		*slice = (*slice)[:maxRecentJobIDs]
	}
}

// RecordConflictDetected records a conflict being detected
func (m *DefaultSchedulerMetricsCollector) RecordConflictDetected(jobID, jobType string) {
	if m.configDisabled() {
		return
	}
	m.appendRecentJobID(&m.recentConflict, jobID)
	// Record using new builder-pattern API
	recorder := m.getSchedulerMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := buildConflictDetectedMetric(jobID, jobType)
		_ = recorder.Record("scheduler_conflict_detected", builder)
	}

	atomic.AddInt64(&m.conflict.detected, 1)
}

// RecordSchedulerStart records scheduler startup
func (m *DefaultSchedulerMetricsCollector) RecordSchedulerStart(duration time.Duration) {
	if m.configDisabled() {
		return
	}
	// Record using new builder-pattern API
	recorder := m.getSchedulerMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := buildSchedulerLifecycleMetric("start", duration)
		_ = recorder.Record("scheduler_start", builder)
	}

	atomic.AddInt64(&m.lifecycle.schedulerStarts, 1)
}

// RecordSchedulerStop records scheduler shutdown
func (m *DefaultSchedulerMetricsCollector) RecordSchedulerStop(duration time.Duration) {
	if m.configDisabled() {
		return
	}
	// Record using new builder-pattern API
	recorder := m.getSchedulerMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := buildSchedulerLifecycleMetric("stop", duration)
		_ = recorder.Record("scheduler_stop", builder)
	}

	atomic.AddInt64(&m.lifecycle.schedulerStops, 1)
}

// RecordJobLoadError records a job load error
func (m *DefaultSchedulerMetricsCollector) RecordJobLoadError(err error) {
	if m.configDisabled() {
		return
	}
	// Record using new builder-pattern API
	recorder := m.getSchedulerMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := buildJobLoadErrorMetric(err)
		_ = recorder.Record("scheduler_job_load_error", builder)
	}

	atomic.AddInt64(&m.lifecycle.jobLoadErrors, 1)
}

// RecordPoolCreationDeclined records that a goroutine pool could not be created from the budget
// (no budget or budget exceeded) and a single-worker fallback was used instead.
func (m *DefaultSchedulerMetricsCollector) RecordPoolCreationDeclined(poolName, purpose, reason string) {
	if m.configDisabled() {
		return
	}
	atomic.AddInt64(&m.pool.creationDeclined, 1)
}

// RecordTriggerQueueDequeued records that the daemon dequeued N trigger requests from the queue.
func (m *DefaultSchedulerMetricsCollector) RecordTriggerQueueDequeued(count int) {
	if m.configDisabled() || count <= 0 {
		return
	}
	recorder := m.getSchedulerMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		_ = recorder.Record("scheduler_trigger_queue_dequeued", buildTriggerQueueDequeuedMetric(count))
	}
	atomic.AddInt64(&m.triggerQueue.dequeued, int64(count))
}

// RecordTriggerQueueTriggerFailed records that triggering a job from the queue failed (after reload retry if any).
func (m *DefaultSchedulerMetricsCollector) RecordTriggerQueueTriggerFailed(jobID string, err error) {
	if m.configDisabled() {
		return
	}
	recorder := m.getSchedulerMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		errStr := ""
		if err != nil {
			errStr = err.Error()
		}
		_ = recorder.Record("scheduler_trigger_queue_trigger_failed", buildTriggerQueueTriggerFailedMetric(jobID, errStr))
	}
	atomic.AddInt64(&m.triggerQueue.triggerFailed, 1)
}

// RecordTriggerQueueReloadRetry records that the daemon reloaded jobs and retried trigger (job was not in cache).
func (m *DefaultSchedulerMetricsCollector) RecordTriggerQueueReloadRetry(jobID string) {
	if m.configDisabled() {
		return
	}
	recorder := m.getSchedulerMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		_ = recorder.Record("scheduler_trigger_queue_reload_retry", buildTriggerQueueReloadRetryMetric(jobID))
	}
	atomic.AddInt64(&m.triggerQueue.reloadRetries, 1)
}

// RecordTriggerQueueReloadFailed records that reload (before retry) failed; job will not run.
func (m *DefaultSchedulerMetricsCollector) RecordTriggerQueueReloadFailed(jobID string, err error) {
	if m.configDisabled() {
		return
	}
	recorder := m.getSchedulerMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		_ = recorder.Record("scheduler_trigger_queue_reload_failed", buildTriggerQueueReloadFailedMetric(jobID, err))
	}
	atomic.AddInt64(&m.triggerQueue.reloadFailed, 1)
}

// RecordDispatchPressureDropped records that a job dispatch attempt was abandoned before the handler ran
// (dispatch resource wait deadline: goroutine ceiling or blocked worker-pool submit).
func (m *DefaultSchedulerMetricsCollector) RecordDispatchPressureDropped(source, reason string) {
	if m.configDisabled() {
		return
	}
	recorder := m.getSchedulerMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		_ = recorder.Record("scheduler_dispatch_pressure_dropped", buildDispatchPressureDroppedMetric(source, reason))
	}
	atomic.AddInt64(&m.dispatchPressure.total, 1)
	switch source {
	case "cron":
		atomic.AddInt64(&m.dispatchPressure.cron, 1)
	case "missed_job_recovery":
		atomic.AddInt64(&m.dispatchPressure.missed, 1)
	case "trigger_job_submit":
		atomic.AddInt64(&m.dispatchPressure.triggerSubmit, 1)
	case "immediate_job_submit":
		atomic.AddInt64(&m.dispatchPressure.immediate, 1)
	default:
		atomic.AddInt64(&m.dispatchPressure.other, 1)
	}
}

// GetMetrics returns a snapshot of current metrics
func (m *DefaultSchedulerMetricsCollector) GetMetrics() SchedulerMetricsSnapshot {
	var snapshot SchedulerMetricsSnapshot
	_ = concurrency.RunInRLockWithLogger(
		&m.mu, LockNameSchedulerMetricsGet, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var avgExecutionTime time.Duration
			if m.timing.executionCount > 0 {
				avgExecutionTime = m.timing.totalExecutionTime / time.Duration(m.timing.executionCount)
			}

			snapshot = SchedulerMetricsSnapshot{
				Timestamp: time.Now(),
				Jobs: JobMetrics{
					Loaded:    atomic.LoadInt64(&m.jobs.loaded),
					Scheduled: atomic.LoadInt64(&m.jobs.scheduled),
					Executed:  atomic.LoadInt64(&m.jobs.executed),
					Succeeded: atomic.LoadInt64(&m.jobs.succeeded),
					Failed:    atomic.LoadInt64(&m.jobs.failed),
				},
				Handlers: HandlerMetrics{
					Created:        atomic.LoadInt64(&m.handlers.created),
					CreationErrors: atomic.LoadInt64(&m.handlers.creationErrors),
				},
				Trigger: TriggerMetrics{
					Validations:      atomic.LoadInt64(&m.trigger.validations),
					ValidationErrors: atomic.LoadInt64(&m.trigger.validationErrors),
				},
				Schedule: ScheduleMetrics{
					Attempts:  atomic.LoadInt64(&m.schedule.attempts),
					Successes: atomic.LoadInt64(&m.schedule.successes),
					Errors:    atomic.LoadInt64(&m.schedule.errors),
				},
				Conflict: ConflictMetrics{
					Checks:   atomic.LoadInt64(&m.conflict.checks),
					Detected: atomic.LoadInt64(&m.conflict.detected),
				},
				Lifecycle: LifecycleMetrics{
					SchedulerStarts: atomic.LoadInt64(&m.lifecycle.schedulerStarts),
					SchedulerStops:  atomic.LoadInt64(&m.lifecycle.schedulerStops),
					JobLoadErrors:   atomic.LoadInt64(&m.lifecycle.jobLoadErrors),
				},
				Pool: PoolMetrics{
					CreationDeclined: atomic.LoadInt64(&m.pool.creationDeclined),
				},
				TriggerQueue: TriggerQueueMetrics{
					Dequeued:      atomic.LoadInt64(&m.triggerQueue.dequeued),
					TriggerFailed: atomic.LoadInt64(&m.triggerQueue.triggerFailed),
					ReloadRetries: atomic.LoadInt64(&m.triggerQueue.reloadRetries),
					ReloadFailed:  atomic.LoadInt64(&m.triggerQueue.reloadFailed),
				},
				DispatchPressure: DispatchPressureMetrics{
					Total:              atomic.LoadInt64(&m.dispatchPressure.total),
					Cron:               atomic.LoadInt64(&m.dispatchPressure.cron),
					MissedJobRecovery:  atomic.LoadInt64(&m.dispatchPressure.missed),
					TriggerJobSubmit:   atomic.LoadInt64(&m.dispatchPressure.triggerSubmit),
					ImmediateJobSubmit: atomic.LoadInt64(&m.dispatchPressure.immediate),
					Other:              atomic.LoadInt64(&m.dispatchPressure.other),
				},
				Timing: TimingMetrics{
					TotalExecutionTime: m.timing.totalExecutionTime,
					AvgExecutionTime:   avgExecutionTime,
					MaxExecutionTime:   m.timing.maxExecutionTime,
					MinExecutionTime:   m.timing.minExecutionTime,
				},
				RecentExecutionStarted:   copyRecent(m.recentStarted),
				RecentExecutionCompleted: copyRecent(m.recentCompleted),
				RecentConflictDetected:   copyRecent(m.recentConflict),
			}
			return nil
		},
	)
	return snapshot
}

func copyRecent(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	out := make([]string, len(s))
	copy(out, s)
	return out
}
