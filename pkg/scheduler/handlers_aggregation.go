package scheduler

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

const (
	aggLogKeyCutoff                 = "cutoff"
	aggLogKeyAction                 = "action"
	aggLogKeyLimit                  = "limit"
	aggLogKeyCurrentCount           = "current_count"
	aggLogKeyProcessedCount         = "processed_count"
	aggLogKeyDeletedCount           = "deleted_count"
	aggLogKeyBatchesProcessed       = "batches_processed"
	aggLogKeyTotalProcessed         = "total_processed"
	aggLogKeyArchived               = "archived"
	aggFieldRetentionDays           = "retention_days"
	aggFilterOpLessThan             = "$lt"
	aggErrHashMismatch              = "hash mismatch"
	aggErrReadHashFile              = "failed to read hash file"
	aggLogFieldKind                 = "kind"
	aggErrFailedDeleteOldMetricsFmt = "failed to delete old metrics: %w"
)

func aggregationHealthLogRoot(logger logging.Logger, metricKind string) *SchedulerLogRoot {
	if strings.Contains(metricKind, "change_journal") {
		return ChangeJournalAggregationLog(logger)
	}
	return AuditAggregationLog(logger)
}

func healthLogEventListFailed(metricKind string) string {
	if strings.Contains(metricKind, "change_journal") {
		return LogEventChangeJournalAggregationHealthListMetricsFailed
	}
	return LogEventAuditAggregationHealthListMetricsFailed
}

func healthLogEventHashMismatchSample(metricKind string) string {
	if strings.Contains(metricKind, "change_journal") {
		return LogEventChangeJournalAggregationHealthHashMismatchSample
	}
	return LogEventAuditAggregationHealthHashMismatchSample
}

func healthLogEventMetricsPotentialIssues(metricKind string) string {
	if strings.Contains(metricKind, "change_journal") {
		return LogEventChangeJournalAggregationHealthMetricsPotentialIssues
	}
	return LogEventAuditAggregationHealthMetricsPotentialIssues
}

// AuditAggregationHandler aggregates audit events
type AuditAggregationHandler struct {
	storage     storagepkg.ObjectStorageProvider
	projectRoot string // Store project root for ID validator initialization (needed for graph backends)
	logger      logging.Logger
}

// preExecutionHealthCheck performs health checks before job execution
// This helps identify issues that could cause job failures
func (h *AuditAggregationHandler) preExecutionHealthCheck(ctx context.Context, job *ScheduledJob, metricKind string) error {
	return preExecutionHealthCheck(ctx, h.storage, h.logger, job, metricKind)
}

// NewAuditAggregationHandler creates a new audit aggregation handler
func NewAuditAggregationHandler(storage storagepkg.ObjectStorageProvider) AuditAggregationHandlerInterface {
	return NewAuditAggregationHandlerWithProjectRoot(storage, "")
}

// NewAuditAggregationHandlerWithProjectRoot creates a new audit aggregation handler with explicit project root
// This is needed for graph backends which don't store project root in the storage object
func NewAuditAggregationHandlerWithProjectRoot(storage storagepkg.ObjectStorageProvider, projectRoot string) AuditAggregationHandlerInterface {
	// If project root not provided, try to get from file-based storage
	if projectRoot == emptyValue {
		if fileStorage, ok := storage.(*storagepkg.FileObjectStorage); ok {
			projectRoot = fileStorage.GetProjectRoot()
		}
	}

	return &AuditAggregationHandler{
		storage:     storage,
		projectRoot: projectRoot,
		logger:      logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// catchUpAgeLookback picks how far before "now" catch-up treats audit_events as old enough to delete.
// Retention first pass uses effectiveRetention (shortened when counts are critical). Catch-up must use
// the same or stricter horizon; if it used only windowDuration (often 1h) while effectiveRetention is
// 10m, events between those ages would not be deleted in catch-up after retention stopped on batch limits.
func catchUpAgeLookback(windowDuration, effectiveRetention time.Duration) time.Duration {
	if effectiveRetention > 0 && (windowDuration == 0 || effectiveRetention < windowDuration) {
		return effectiveRetention
	}
	return windowDuration
}

// metricLimitAuditEventLookback is the age threshold for deleting audit_events when audit_aggregation_metric
// count is at the hard limit. Uses half of configured retention, then applies the same tightening as catch-up
// (count-shortened effectiveRetention + aggregation window) so we do not leave a gap vs the first pass.
func metricLimitAuditEventLookback(windowDuration, deleteAfterDuration, effectiveRetention time.Duration) time.Duration {
	half := deleteAfterDuration / 2
	if half <= 0 {
		return catchUpAgeLookback(windowDuration, effectiveRetention)
	}
	candidate := half
	if effectiveRetention > 0 && effectiveRetention < candidate {
		candidate = effectiveRetention
	}
	return catchUpAgeLookback(windowDuration, candidate)
}

// auditAggregationRetentionOutcomeFields returns stable strings for job outcome JSON (scheduler forensics).
func auditAggregationRetentionOutcomeFields(
	windowDuration, deleteAfterDuration, effectiveRetention time.Duration,
	earlyEventCount int,
	earlyCountErr error,
) map[string]any {
	m := map[string]any{
		OutcomeKeyAggregationWindow:           windowDuration.String(),
		OutcomeKeyConfiguredRetention:         deleteAfterDuration.String(),
		OutcomeKeyEffectiveRetention:          effectiveRetention.String(),
		OutcomeKeyCatchUpAgeLookback:          catchUpAgeLookback(windowDuration, effectiveRetention).String(),
		OutcomeKeyMetricLimitAuditAgeLookback: metricLimitAuditEventLookback(windowDuration, deleteAfterDuration, effectiveRetention).String(),
	}
	if earlyCountErr == nil {
		m[OutcomeKeyAuditEventCountAtStart] = earlyEventCount
	}
	return m
}

// auditAggregationDegradedSuccessOutcomePatch returns extra outcome keys when aggregation finished with a non-nil
// result despite a recoverable error (e.g. hash mismatch with partial data). Returns nil if note is empty.
func auditAggregationDegradedSuccessOutcomePatch(note string, err error) map[string]any {
	if note == emptyValue {
		return nil
	}
	m := map[string]any{
		OutcomeKeyAggregationDegraded:     true,
		OutcomeKeyAggregationDegradedNote: note,
	}
	if err != nil {
		m[OutcomeKeyAggregationDegradedError] = err.Error()
	}
	return m
}

// writeAuditAggregationPartialOutcome persists job outcome when the run does not reach the full success path
// that processes a non-nil aggregate result (or after aggregate returns nil with err==nil). extra may be nil.
func (h *AuditAggregationHandler) writeAuditAggregationPartialOutcome(
	job *ScheduledJob,
	phaseDurations map[string]float64,
	retentionOutcomeFields map[string]any,
	retentionFirstPass, catchUpProcessed, postAggCleanupProcessed, retentionSecondPassProcessed int,
	aggregationSeconds float64,
	extra map[string]any,
) {
	phaseDurations["aggregation"] = aggregationSeconds
	outcome := map[string]any{
		OutcomeKeyEventsProcessed:                 0,
		OutcomeKeyMetricsCreated:                  0,
		OutcomeKeyPhaseDurations:                  phaseDurations,
		OutcomeKeyRetentionFirstPassProcessed:     retentionFirstPass,
		OutcomeKeyCatchUpProcessed:                catchUpProcessed,
		OutcomeKeyPostAggregationCleanupProcessed: postAggCleanupProcessed,
		OutcomeKeyRetentionSecondPassProcessed:    retentionSecondPassProcessed,
		OutcomeKeyEventsDeletedOrArchivedTotal:    retentionFirstPass + catchUpProcessed + postAggCleanupProcessed + retentionSecondPassProcessed,
	}
	for k, v := range extra {
		outcome[k] = v
	}
	maps.Copy(outcome, retentionOutcomeFields)
	WriteJobOutcome(h.projectRoot, job.ID, JobTypeAuditEventAggregation, outcome)
	logPhaseDurations(h.logger, job.ID, phaseDurations)
}

// Execute aggregates audit events via the standardized pipeline (INGEST → NORMALIZE → FINALIZE).
// Acquires AuditAggregationSingletonLockID before running to prevent concurrent executions from
// producing duplicate audit_aggregation_metric stream entries. Two independent callers compete:
// the scheduler cron job (SCH-002) and the MaintenanceRunner (which bypasses ConflictManager).
// If the lock is already held, this run is skipped with a log warning and nil is returned.
func (h *AuditAggregationHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	jobID := emptyValue
	if job != nil {
		jobID = job.ID
	}
	if h.projectRoot != emptyValue {
		jobLock, lockErr := NewJobLock(AuditAggregationSingletonLockID, h.projectRoot)
		if lockErr != nil {
			AuditAggregationLog(h.logger).Warn(LogEventAuditAggregationSingletonLockCreateFailed).
				JobID(jobID).
				WithError(lockErr).
				Log()
		} else {
			acquired, tryErr := jobLock.TryAcquire()
			if tryErr != nil {
				AuditAggregationLog(h.logger).Warn(LogEventAuditAggregationSingletonLockTryAcquireFailed).
					JobID(jobID).
					WithError(tryErr).
					Log()
			} else if !acquired {
				AuditAggregationLog(h.logger).Info(LogEventAuditAggregationSingletonLockHeldSkipExecution).
					JobID(jobID).
					Log()
				if job != nil && job.ID != emptyValue {
					WriteJobOutcome(h.projectRoot, job.ID, JobTypeAuditEventAggregation, map[string]any{
						OutcomeKeyAggregationSkipped:    true,
						OutcomeKeyAggregationSkipReason: OutcomeSkipReasonSingletonLock,
					})
				}
				return nil
			} else {
				defer func() {
					if relErr := jobLock.Release(); relErr != nil {
						AuditAggregationLog(h.logger).Warn(LogEventAuditAggregationSingletonLockReleaseFailed).
							JobID(jobID).
							WithError(relErr).
							Log()
					}
				}()
			}
		}
	}
	return RunAuditAggregationViaPipeline(ctx, h, job)
}

// logPhaseDurations logs each phase duration for bottleneck analysis (e.g. SCH-002 taking 15m for 0 events).
func logPhaseDurations(logger logging.Logger, jobID string, phases map[string]float64) {
	if logger == nil || len(phases) == 0 {
		return
	}
	entry := AuditAggregationLog(logger).Info(LogEventAuditAggregationPhaseDurationsBottleneckDump).JobID(jobID)
	var total float64
	for name, sec := range phases {
		entry = entry.String("phase_"+name+"_sec", fmt.Sprintf("%.2f", sec))
		total += sec
	}
	entry.String("phase_total_sec", fmt.Sprintf("%.2f", total)).Log()
}

// runWithTimeout runs fn in a goroutine and returns its result, or false if ctx is done or timeout elapses.
// Used so aggregation job never blocks indefinitely on cache check (e.g. waiting for cache RLock).
func runWithTimeout(ctx context.Context, timeout time.Duration, fn func() bool) bool {
	type result struct{ v bool }
	done := make(chan result, 1)
	goroutinelabels.NewGoroutine("scheduler", "run with timeout").StartSimple(func() {
		done <- result{v: fn()}
	})
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case r := <-done:
		return r.v
	case <-ctx.Done():
		return false
	case <-timer.C:
		return false
	}
}

// metricCreationTimeoutFromJobRuntime returns a duration string for metric creation timeout,
// derived from job max runtime so long-running jobs get more room. Timeouts are generous by design;
// hitting one indicates a broader issue (load, contention, backlog)—investigate rather than only increasing.
// Formula: min(120s, max(90s, max_runtime_seconds/6)); if max_runtime is 0, returns "90s".
func metricCreationTimeoutFromJobRuntime(maxRuntimeSeconds int) string {
	const minTimeout = 90 * time.Second
	const maxTimeout = 120 * time.Second
	d := minTimeout
	if maxRuntimeSeconds > 0 {
		d = time.Duration(maxRuntimeSeconds) * time.Second / 6
		if d < minTimeout {
			d = minTimeout
		}
		if d > maxTimeout {
			d = maxTimeout
		}
	}
	return strconv.Itoa(int(d.Seconds())) + "s"
}

// normalizeDurationEnv returns a duration string suitable for ZQK_AGGREGATION_METRIC_CREATION_TIMEOUT.
// If s is already a valid duration (e.g. "60s"), returns it; if s is a bare number (e.g. "60" from YAML), returns "60s".
func normalizeDurationEnv(s string) string {
	s = strings.TrimSpace(s)
	if s == emptyValue {
		return s
	}
	if _, err := time.ParseDuration(s); err == nil {
		return s
	}
	if _, err := strconv.Atoi(s); err == nil {
		return s + "s"
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil && f > 0 {
		return strconv.Itoa(int(f)) + "s"
	}
	return s
}

// removeEmptyBucketDirs removes empty bucket folders for the kind (e.g. audit/2026-01-20/) after cleanup.
func removeEmptyBucketDirs(storage storagepkg.ObjectStorageProvider, kind string, logger logging.Logger, jobID string) {
	if fileStorage, ok := storage.(*storagepkg.FileObjectStorage); ok {
		removed, err := fileStorage.RemoveEmptyBucketDirectories(kind)
		if err != nil {
			AuditAggregationLog(logger).Warn(LogEventAuditAggregationRemoveEmptyBucketDirsFailed).
				JobID(jobID).
				Kind(kind).
				WithError(err).
				Log()
		} else if removed > 0 {
			AuditAggregationLog(logger).Info(LogEventAuditAggregationRemoveEmptyBucketDirsRemoved).
				JobID(jobID).
				Kind(kind).
				Int("removed", removed).
				Log()
		}
	}
}
