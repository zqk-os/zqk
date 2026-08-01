package scheduler

import (
	"context"
	"fmt"
	"maps"
	"os"
	"strconv"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"

	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/when"
	"github.com/lanceman/zqk/pkg/zqkenv"
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

type auditAggregationSession struct {
	handler             *AuditAggregationHandler
	ctx                 context.Context
	job                 *ScheduledJob
	service             *storagepkg.AuditAggregationService
	secCtx              *pkgctx.SecurityContext
	storageCtx          *pkgctx.StorageContext
	phaseDurations      map[string]float64
	phaseStart          time.Time
	windowDuration      time.Duration
	deleteAfterDuration time.Duration
	effectiveRetention  time.Duration
	archiveEnabled      bool
	deleteEnabled       bool
	batchSize           int
	windowStart         time.Time
	windowEnd           time.Time

	// Phase-specific state
	preAggCtx           context.Context
	preAggCancel        context.CancelFunc
	retentionMaxBatches int
	catchUpMaxBatches   int

	// Outcome counters
	retentionFirstPass      int
	catchUpProcessed        int
	postAggCleanupProcessed int
	retentionSecondPass     int
	earlyEventCount         int
	earlyCountErr           error
	aggregationDegradedNote string
	retentionOutcomeFields  map[string]any

	// Cleanup functions
	cleanupFuncs []func()
}

func (s *auditAggregationSession) startPhase(name string) {
	s.phaseStart = time.Now()
}

func (s *auditAggregationSession) endPhase(name string) {
	if s.phaseDurations == nil {
		s.phaseDurations = make(map[string]float64)
	}
	s.phaseDurations[name] = time.Since(s.phaseStart).Seconds()
}

func (s *auditAggregationSession) loadConfiguration() error {
	s.batchSize = storagepkg.DefaultBatchSize
	if s.job.EnvironmentVariables != nil {
		if batchSizeStr, ok := s.job.EnvironmentVariables[EnvKeyBatchSize]; ok && batchSizeStr != emptyValue {
			if parsedSize, err := strconv.Atoi(batchSizeStr); err == nil && parsedSize > 0 {
				s.batchSize = parsedSize
			}
		}
	}

	// Create aggregation service with configured batch size
	s.service = storagepkg.NewAuditAggregationServiceWithBatchSize(s.handler.storage, s.batchSize)

	// Get contexts
	s.secCtx = pkgctx.NewSystemSecurityContext()
	s.storageCtx = pkgctx.NewStorageContext()

	// Defaults are lean (1h window, 2h retention) so audit_event count stays near target (~600 per retention_tolerance.yaml).
	s.windowDuration = 1 * time.Hour
	s.deleteAfterDuration = 2 * time.Hour

	if s.job.EnvironmentVariables != nil {
		if windowStr, ok := s.job.EnvironmentVariables[EnvKeyAggregationWindow]; ok && windowStr != emptyValue {
			if duration, err := time.ParseDuration(windowStr); err == nil {
				s.windowDuration = duration
			}
		}
		var retentionStr string
		if retStr, ok := s.job.EnvironmentVariables[EnvKeyRetentionDuration]; ok && retStr != emptyValue {
			retentionStr = retStr
		} else if delStr, ok := s.job.EnvironmentVariables[EnvKeyDeleteAfterDays]; ok && delStr != emptyValue {
			retentionStr = delStr
		}
		if retentionStr != emptyValue {
			if d, ok := parseDurationOrHoursSuffix(retentionStr); ok {
				s.deleteAfterDuration = d
			}
		}

		if val, ok := s.job.EnvironmentVariables[EnvKeyArchiveEnabled]; ok && (val == "true" || val == "1") {
			s.archiveEnabled = true
		}
		if val, ok := s.job.EnvironmentVariables[EnvKeyDeleteEnabled]; ok && (val == "true" || val == "1") {
			s.deleteEnabled = true
		}
	}

	return nil
}

func (s *auditAggregationSession) cleanup() {
	for i := len(s.cleanupFuncs) - 1; i >= 0; i-- {
		s.cleanupFuncs[i]()
	}
	if s.preAggCancel != nil {
		s.preAggCancel()
	}
}

func (s *auditAggregationSession) runPreChecks() (error, error) {
	s.startPhase("pre_checks")
	defer s.endPhase("pre_checks")

	// 1. Health check before execution
	healthCheckErr := s.handler.preExecutionHealthCheck(s.ctx, s.job, objects.KindAuditAggregationMetric)
	if healthCheckErr != nil {
		AuditAggregationLog(s.handler.logger).Warn(LogEventAuditAggregationPreExecHealthFailedWarning).
			WithFields(jobLogFieldsWithErr(s.job, healthCheckErr)...).
			Log()
	}

	// 2. CAS recovery if needed
	if healthCheckErr != nil {
		const casRecoveryMax = 3 * time.Minute
		casCtx, casCancel := context.WithTimeout(s.ctx, casRecoveryMax)
		defer casCancel()
		runCASRecoveryForKind(casCtx, s.handler.projectRoot, objects.KindAuditEvent, s.handler.logger, s.handler.storage)
		if casCtx.Err() != nil {
			AuditAggregationLog(s.handler.logger).Warn(LogEventAuditAggregationCASRecoveryStoppedEarlyTimeLimit).
				JobID(s.job.ID).
				String(aggLogKeyLimit, casRecoveryMax.String()).
				Log()
		}
	}

	// 3. Metric creation timeout environment setup
	var timeoutStr string
	if s.job.EnvironmentVariables != nil {
		timeoutStr = s.job.EnvironmentVariables[zqkenv.AggregationMetricCreationTimeout()]
	}
	when.When(func() bool { return timeoutStr == emptyValue }).Then(func() {
		timeoutStr = metricCreationTimeoutFromJobRuntime(s.job.MaxRuntimeSeconds)
	}).OrElse(func() {
		timeoutStr = normalizeDurationEnv(timeoutStr)
	}).Run()
	prev := os.Getenv(zqkenv.AggregationMetricCreationTimeout())
	if err := os.Setenv(zqkenv.AggregationMetricCreationTimeout(), timeoutStr); err != nil {
		SLog(s.handler.logger).Debug("Failed to set aggregation metric creation timeout env").WithError(err).Log()
	}

	s.cleanupFuncs = append(s.cleanupFuncs, func() {
		when.When(func() bool { return prev == emptyValue }).Then(func() {
			if err := os.Unsetenv(zqkenv.AggregationMetricCreationTimeout()); err != nil {
				SLog(s.handler.logger).Debug("Failed to unset aggregation metric creation timeout env").WithError(err).Log()
			}
		}).OrElse(func() {
			if err := os.Setenv(zqkenv.AggregationMetricCreationTimeout(), prev); err != nil {
				SLog(s.handler.logger).Debug("Failed to restore aggregation metric creation timeout env").WithError(err).Log()
			}
		}).Run()
	})

	// 4. Ensure high-volume event cache is ready
	if fileStorage, ok := s.handler.storage.(*storagepkg.FileObjectStorage); ok {
		projectRoot := fileStorage.GetProjectRoot()
		if projectRoot != emptyValue {
			cache := storagepkg.GetGlobalHighVolumeEventCache()
			cachePopulated := runWithTimeout(s.ctx, 5*time.Second, func() bool {
				return cache != nil && cache.IsPopulatedForProject(projectRoot)
			})
			if !cachePopulated {
				buildTimeout := 5 * time.Minute
				if deadline, ok := s.ctx.Deadline(); ok && time.Until(deadline) < buildTimeout+time.Minute {
					buildTimeout = time.Until(deadline) - time.Minute
					if buildTimeout < 30*time.Second {
						buildTimeout = 30 * time.Second
					}
				}
				buildCtx, buildCancel := context.WithTimeout(s.ctx, buildTimeout)
				defer buildCancel()
				buildErr := storagepkg.EnsureHighVolumeEventCacheReady(buildCtx, projectRoot, s.handler.storage, false)
				if buildErr != nil {
					AuditAggregationLog(s.handler.logger).Warn(LogEventAuditAggregationHVCacheBuildFailedFallback).
						JobID(s.job.ID).
						WithError(buildErr).
						Log()
				} else {
					AuditAggregationLog(s.handler.logger).Info(LogEventAuditAggregationHVCacheBuiltSuccess).
						JobID(s.job.ID).
						String("project_root", projectRoot).
						Log()
				}
			} else {
				AuditAggregationLog(s.handler.logger).Debug(LogEventAuditAggregationHVCacheAlreadyPopulated).
					JobID(s.job.ID).
					Log()
			}
		}
	}

	return healthCheckErr, nil
}

func (s *auditAggregationSession) setupPhaseContexts() {
	// Cap pre-aggregation phase so we always reach aggregation (avoids SCH-002 running 1hr with 0 events).
	// Use preAggCtx for retention, count, catch-up, aggressive/proactive cleanup; then use ctx for aggregation.
	const preAggMax = 20 * time.Minute
	const aggregationReserve = 10 * time.Minute
	preAggDeadline := time.Now().Add(preAggMax)
	if jobDeadline, ok := s.ctx.Deadline(); ok && jobDeadline.Before(preAggDeadline) {
		preAggDeadline = jobDeadline.Add(-aggregationReserve)
		if preAggDeadline.Before(time.Now()) {
			preAggDeadline = time.Now().Add(2 * time.Minute) // at least 2 min for cleanup
		}
	}
	s.preAggCtx, s.preAggCancel = context.WithDeadline(s.ctx, preAggDeadline)

	// When job has a deadline (e.g. max_runtime_seconds 1800), use conservative batch caps so we
	// finish retention/catch-up with time left for aggregation; avoids SCH-002 routinely timing out.
	_, hasDeadline := s.ctx.Deadline()
	s.catchUpMaxBatches = 12
	s.retentionMaxBatches = 8
	if hasDeadline {
		s.catchUpMaxBatches = 8
		s.retentionMaxBatches = 6
	}
}

func (s *auditAggregationSession) hasTimeRemaining() bool {
	const timeBufferForAggregation = 3 * time.Minute
	if deadline, ok := s.preAggCtx.Deadline(); ok {
		remaining := time.Until(deadline)
		return remaining > timeBufferForAggregation
	}
	return true // No deadline, proceed
}

func (s *auditAggregationSession) determineEffectiveRetention() {
	const auditEventCountCriticalThreshold = 100000
	const auditEventCountVeryHighThreshold = 20000
	const auditEventCountHighThreshold = 5000
	const aggressiveRetentionCritical = 10 * time.Minute
	const aggressiveRetentionVeryHigh = 15 * time.Minute
	const aggressiveRetentionHigh = 30 * time.Minute

	s.effectiveRetention = s.deleteAfterDuration
	eventFilter := storagepkg.ListFilter{Kind: objects.KindAuditEvent}
	s.earlyEventCount, s.earlyCountErr = s.handler.storage.Count(s.preAggCtx, s.secCtx, eventFilter)

	if s.earlyCountErr == nil && s.deleteAfterDuration > 0 {
		if s.earlyEventCount > auditEventCountCriticalThreshold {
			s.effectiveRetention = aggressiveRetentionCritical
			if s.retentionMaxBatches < 25 {
				s.retentionMaxBatches = 25
			}
			if s.catchUpMaxBatches < 12 {
				s.catchUpMaxBatches = 12
			}
		} else if s.earlyEventCount > auditEventCountVeryHighThreshold {
			s.effectiveRetention = aggressiveRetentionVeryHigh
			if s.retentionMaxBatches < 12 {
				s.retentionMaxBatches = 12
			}
		} else if s.earlyEventCount > auditEventCountHighThreshold {
			s.effectiveRetention = aggressiveRetentionHigh
			if s.retentionMaxBatches < 10 {
				s.retentionMaxBatches = 10
			}
		}
	}

	if s.effectiveRetention != s.deleteAfterDuration {
		AuditAggregationLog(s.handler.logger).Info(LogEventAuditAggregationCountBasedShortenedRetention).
			JobID(s.job.ID).
			Int("audit_event_count", s.earlyEventCount).
			String("effective_retention", s.effectiveRetention.String()).
			String("configured_retention", s.deleteAfterDuration.String()).
			Log()
	}

	s.retentionOutcomeFields = auditAggregationRetentionOutcomeFields(
		s.windowDuration, s.deleteAfterDuration, s.effectiveRetention, s.earlyEventCount, s.earlyCountErr)
}

func (s *auditAggregationSession) runRetentionFirstPass() {
	s.startPhase("retention_first_pass")
	defer s.endPhase("retention_first_pass")

	if s.effectiveRetention <= 0 {
		return
	}

	cutoffTime := time.Now().UTC().Add(-s.effectiveRetention)
	action := "delete"
	if s.archiveEnabled && !s.deleteEnabled {
		action = "archive"
	}
	AuditAggregationLog(s.handler.logger).Info(LogEventAuditAggregationRetentionFirstPassStarted).
		String(aggLogKeyCutoff, cutoffTime.Format(time.RFC3339)).
		String("retention_duration", s.effectiveRetention.String()).
		String(aggLogKeyAction, action).
		Log()

	const catchUpBatchSize = 2000
	totalProcessed := 0
	for batch := 0; batch < s.retentionMaxBatches; batch++ {
		if s.preAggCtx.Err() != nil {
			AuditAggregationLog(s.handler.logger).Warn(LogEventAuditAggregationRetentionCleanupCancelled).
				JobID(s.job.ID).
				Int(aggLogKeyBatchesProcessed, batch).
				Int(aggLogKeyTotalProcessed, totalProcessed).
				WithError(s.preAggCtx.Err()).
				Log()
			break
		}
		if !s.hasTimeRemaining() {
			AuditAggregationLog(s.handler.logger).Info(LogEventAuditAggregationRetentionStoppingEarlyForAgg).
				JobID(s.job.ID).
				Int(aggLogKeyBatchesProcessed, batch).
				Int(aggLogKeyTotalProcessed, totalProcessed).
				Log()
			break
		}
		oldEventIDs, err := s.service.QueryOldAuditEventsByAge(s.preAggCtx, s.secCtx, s.storageCtx, cutoffTime, catchUpBatchSize)
		if err != nil || len(oldEventIDs) == 0 {
			break
		}
		processedCount, err := s.service.CleanupAggregatedEvents(s.preAggCtx, s.secCtx, oldEventIDs, s.archiveEnabled && !s.deleteEnabled)
		if err != nil {
			AuditAggregationLog(s.handler.logger).Warn(LogEventAuditAggregationRetentionFirstPassBatchFailed).
				WithFields(jobLogFieldsWithErr(s.job, err)...).
				Log()
			break
		}
		totalProcessed += processedCount
		if batch%5 == 0 && batch > 0 {
			AuditAggregationLog(s.handler.logger).Info(LogEventAuditAggregationRetentionCleanupProgress).
				JobID(s.job.ID).
				Int("batch", batch).
				Int(aggLogKeyTotalProcessed, totalProcessed).
				Log()
		}
		if processedCount < len(oldEventIDs) {
			break
		}
	}
	if totalProcessed > 0 {
		AuditAggregationLog(s.handler.logger).Info(LogEventAuditAggregationRetentionFirstPassCompleted).
			JobID(s.job.ID).
			Int(aggLogKeyProcessedCount, totalProcessed).
			Bool(aggLogKeyArchived, s.archiveEnabled && !s.deleteEnabled).
			Log()
		if s.deleteEnabled || !s.archiveEnabled {
			removeEmptyBucketDirs(s.handler.storage, objects.KindAuditEvent, s.handler.logger, s.job.ID)
		}
	}
	s.retentionFirstPass = totalProcessed
}

func (s *auditAggregationSession) runCatchUp() {
	s.startPhase("catch_up")
	defer s.endPhase("catch_up")

	const auditEventLimit = 1000
	const catchUpBatchSize = 2000
	eventFilter := storagepkg.ListFilter{Kind: objects.KindAuditEvent}
	eventCount, countErr := s.handler.storage.Count(s.preAggCtx, s.secCtx, eventFilter)
	if countErr != nil {
		AuditAggregationLog(s.handler.logger).Warn(LogEventAuditAggregationAuditEventCountFailedSkipCatchUp).
			JobID(s.job.ID).
			WithError(countErr).
			Log()
		return
	}

	if eventCount > auditEventLimit {
		catchUpLookback := catchUpAgeLookback(s.windowDuration, s.effectiveRetention)
		cutoffTime := time.Now().UTC().Add(-catchUpLookback)
		AuditAggregationLog(s.handler.logger).Warn(LogEventAuditAggregationAuditEventCountOverLimitCatchUp).
			JobID(s.job.ID).
			Int(aggLogKeyCurrentCount, eventCount).
			Int(aggLogKeyLimit, auditEventLimit).
			String("catch_up_age_lookback", catchUpLookback.String()).
			String(aggLogKeyCutoff, cutoffTime.Format(time.RFC3339)).
			Log()

		totalProcessed := 0
		for batch := 0; batch < s.catchUpMaxBatches; batch++ {
			if s.preAggCtx.Err() != nil {
				AuditAggregationLog(s.handler.logger).Warn(LogEventAuditAggregationCatchUpCleanupCancelled).
					JobID(s.job.ID).
					Int(aggLogKeyBatchesProcessed, batch).
					Int(aggLogKeyTotalProcessed, totalProcessed).
					WithError(s.preAggCtx.Err()).
					Log()
				break
			}
			if !s.hasTimeRemaining() {
				AuditAggregationLog(s.handler.logger).Info(LogEventAuditAggregationCatchUpStoppingEarlyForAgg).
					JobID(s.job.ID).
					Int(aggLogKeyBatchesProcessed, batch).
					Int(aggLogKeyTotalProcessed, totalProcessed).
					Int("remaining_events", eventCount).
					Log()
				break
			}
			oldIDs, queryErr := s.service.QueryOldAuditEventsByAge(s.preAggCtx, s.secCtx, s.storageCtx, cutoffTime, catchUpBatchSize)
			if queryErr != nil || len(oldIDs) == 0 {
				break
			}
			processed, delErr := s.service.CleanupAggregatedEvents(s.preAggCtx, s.secCtx, oldIDs, s.archiveEnabled && !s.deleteEnabled)
			if delErr != nil {
				AuditAggregationLog(s.handler.logger).Warn(LogEventAuditAggregationCatchUpBatchFailed).
					WithFields(jobLogFieldsWithErr(s.job, delErr)...).
					Log()
				break
			}
			totalProcessed += processed
			eventCount -= processed
			if batch%5 == 0 && batch > 0 {
				AuditAggregationLog(s.handler.logger).Info(LogEventAuditAggregationCatchUpCleanupProgress).
					JobID(s.job.ID).
					Int("batch", batch).
					Int(aggLogKeyTotalProcessed, totalProcessed).
					Int("remaining_events", eventCount).
					Log()
			}
			if eventCount <= auditEventLimit {
				break
			}
		}
		if totalProcessed > 0 {
			AuditAggregationLog(s.handler.logger).Info(LogEventAuditAggregationCatchUpCleanupCompleted).
				JobID(s.job.ID).
				Int(aggLogKeyProcessedCount, totalProcessed).
				Bool(aggLogKeyArchived, s.archiveEnabled && !s.deleteEnabled).
				Log()
			if s.deleteEnabled || !s.archiveEnabled {
				removeEmptyBucketDirs(s.handler.storage, objects.KindAuditEvent, s.handler.logger, s.job.ID)
			}
		}
		s.catchUpProcessed = totalProcessed
	}
}

func (s *auditAggregationSession) runAggressiveCleanup() {
	s.startPhase("aggressive_cleanup")
	defer s.endPhase("aggressive_cleanup")

	metricFilter := storagepkg.ListFilter{Kind: objects.KindAuditAggregationMetric}
	metricCount, countErr := s.handler.storage.Count(s.preAggCtx, s.secCtx, metricFilter)
	if countErr == nil && metricCount >= MaxAggregationObjectCountPerKind {
		AuditAggregationLog(s.handler.logger).Warn(LogEventAuditAggregationMetricCountLimitAggressiveCleanup).
			JobID(s.job.ID).
			Int(aggLogKeyCurrentCount, metricCount).
			Int(aggLogKeyLimit, MaxAggregationObjectCountPerKind).
			Log()

		aggressiveRetention := s.deleteAfterDuration / 2
		cutoffTime := time.Now().UTC().Add(-aggressiveRetention)

		if s.preAggCtx.Err() != nil {
			AuditAggregationLog(s.handler.logger).Warn(LogEventAuditAggregationAggressiveCleanupSkippedCancelled).
				JobID(s.job.ID).
				WithError(s.preAggCtx.Err()).
				Log()
			return
		}

		oldEventIDs, queryErr := s.service.QueryOldAggregatedEvents(s.preAggCtx, s.secCtx, s.storageCtx, cutoffTime)
		if queryErr == nil && len(oldEventIDs) > 0 {
			processedCount, delErr := s.service.CleanupAggregatedEvents(s.preAggCtx, s.secCtx, oldEventIDs, s.archiveEnabled && !s.deleteEnabled)
			if delErr == nil {
				AuditAggregationLog(s.handler.logger).Info(LogEventAuditAggregationAggressiveCleanupArchivedCompleted).
					JobID(s.job.ID).
					Int(aggLogKeyProcessedCount, processedCount).
					Bool(aggLogKeyArchived, s.archiveEnabled && !s.deleteEnabled).
					Log()
			}
		}

		eventFilter := storagepkg.ListFilter{Kind: objects.KindAuditEvent}
		eventCount, errCount := s.handler.storage.Count(s.preAggCtx, s.secCtx, eventFilter)
		const auditEventLimit = 1000
		if errCount == nil && eventCount > auditEventLimit {
			auditLookback := metricLimitAuditEventLookback(s.windowDuration, s.deleteAfterDuration, s.effectiveRetention)
			cutoffAudit := time.Now().UTC().Add(-auditLookback)
			AuditAggregationLog(s.handler.logger).Info(LogEventAuditAggregationAggressiveCleanupAuditEventsByAge).
				JobID(s.job.ID).
				String("archived_aggregate_cutoff", cutoffTime.Format(time.RFC3339)).
				String("audit_age_lookback", auditLookback.String()).
				String("audit_cutoff", cutoffAudit.Format(time.RFC3339)).
				Log()
			const catchUpBatchSize = 2000
			oldIDs, errQuery := s.service.QueryOldAuditEventsByAge(s.preAggCtx, s.secCtx, s.storageCtx, cutoffAudit, catchUpBatchSize)
			if errQuery != nil {
				AuditAggregationLog(s.handler.logger).Warn("Failed to query old audit events by age for aggressive cleanup").WithError(errQuery).Log()
			}
			if len(oldIDs) > 0 {
				deleted, errCleanup := s.service.CleanupAggregatedEvents(s.preAggCtx, s.secCtx, oldIDs, s.archiveEnabled && !s.deleteEnabled)
				if errCleanup == nil && deleted > 0 {
					AuditAggregationLog(s.handler.logger).Info(LogEventAuditAggregationAggressiveCleanupOldEventsCompleted).
						JobID(s.job.ID).
						Int(aggLogKeyDeletedCount, deleted).
						Log()
					removeEmptyBucketDirs(s.handler.storage, objects.KindAuditEvent, s.handler.logger, s.job.ID)
				}
			}
		}
	}
}

func (s *auditAggregationSession) runProactiveCleanup() {
	s.startPhase("proactive_cleanup")
	defer s.endPhase("proactive_cleanup")

	if s.preAggCtx.Err() != nil {
		AuditAggregationLog(s.handler.logger).Warn(LogEventAuditAggregationProactiveMetricCleanupSkippedCancelled).
			JobID(s.job.ID).
			WithError(s.preAggCtx.Err()).
			Log()
		return
	}

	const softMetricLimit = 2500
	metricFilter := storagepkg.ListFilter{Kind: objects.KindAuditAggregationMetric}
	currentMetricCount, errMetricCount := s.handler.storage.Count(s.preAggCtx, s.secCtx, metricFilter)
	if errMetricCount == nil && currentMetricCount >= softMetricLimit {
		proactiveRetention := 7 * 24 * time.Hour
		proactiveCutoff := time.Now().UTC().Add(-proactiveRetention)
		oldMetricFilter := storagepkg.ListFilter{
			Kind: objects.KindAuditAggregationMetric,
			Filters: map[string]any{
				objects.FieldKeyCreatedAt: map[string]any{aggFilterOpLessThan: proactiveCutoff.Format(time.RFC3339)},
			},
			SortBy: objects.FieldKeyCreatedAt, SortAsc: true, Limit: 500,
		}
		listResult, listErr := s.handler.storage.List(s.preAggCtx, s.secCtx, s.storageCtx, oldMetricFilter)
		if listErr == nil && listResult != nil && len(listResult.Objects) > 0 {
			ids := make([]string, 0, len(listResult.Objects))
			for _, o := range listResult.Objects {
				if id, ok := o[objects.FieldKeyID].(string); ok && id != emptyValue {
					ids = append(ids, id)
				}
			}
			if len(ids) > 0 {
				var delCount int
				if fileStorage, ok := s.handler.storage.(*storagepkg.FileObjectStorage); ok {
					delResult, delErr := fileStorage.BulkDeleteOptimized(s.preAggCtx, s.secCtx, ids, false, 20)
					if delErr == nil {
						delCount = delResult.SuccessCount
					}
				} else {
					delResult, delErr := s.handler.storage.BulkDelete(s.preAggCtx, s.secCtx, ids, false)
					if delErr == nil {
						delCount = delResult.SuccessCount
					}
				}
				if delCount > 0 {
					AuditAggregationLog(s.handler.logger).Info(LogEventAuditAggregationProactiveMetricCleanupCompleted).
						JobID(s.job.ID).
						Int(aggLogKeyDeletedCount, delCount).
						Log()
				}
			}
		}
	}
}

func (s *auditAggregationSession) runAggregation() (*storagepkg.AuditAggregationResult, error) {
	if s.ctx.Err() != nil {
		AuditAggregationLog(s.handler.logger).Warn(LogEventAuditAggregationSkippingCancelled).
			JobID(s.job.ID).
			WithError(s.ctx.Err()).
			Log()
		s.handler.writeAuditAggregationPartialOutcome(s.job, s.phaseDurations, s.retentionOutcomeFields,
			s.retentionFirstPass, s.catchUpProcessed, 0, 0, 0,
			map[string]any{
				OutcomeKeyAggregationSkipped:    true,
				OutcomeKeyAggregationSkipReason: s.ctx.Err().Error(),
			})
		return nil, s.ctx.Err()
	}

	s.startPhase("aggregation")
	defer s.endPhase("aggregation")

	AuditAggregationLog(s.handler.logger).Info(LogEventAuditAggregationBeginningAggregateWindowPass).
		JobID(s.job.ID).
		String("window_start", s.windowStart.Format(time.RFC3339)).
		String("window_end", s.windowEnd.Format(time.RFC3339)).
		Log()

	result, err := s.service.AggregateAuditEvents(s.ctx, s.secCtx, s.storageCtx, s.windowStart, s.windowEnd)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, aggErrHashMismatch) || strings.Contains(errStr, aggErrReadHashFile) {
			AuditAggregationLog(s.handler.logger).Warn(LogEventAuditAggregationHashMismatchPartialContinue).
				WithFields(jobLogFieldsWithErr(s.job, err)...).
				Log()
			if result == nil {
				aggSec := time.Since(s.phaseStart).Seconds()
				s.handler.writeAuditAggregationPartialOutcome(s.job, s.phaseDurations, s.retentionOutcomeFields,
					s.retentionFirstPass, s.catchUpProcessed, 0, 0, aggSec,
					map[string]any{
						OutcomeKeyAggregationFailed:       true,
						OutcomeKeyAggregationFailureClass: OutcomeFailureClassHashMismatchRetry,
						OutcomeKeyAggregationError:        err.Error(),
					})
				return nil, errfmt.Newf("aggregation failed with hash mismatch, will retry").Wrap(err)
			}
			s.aggregationDegradedNote = OutcomeDegradedNoteHashMismatchPartial
		} else if strings.Contains(errStr, "object already exists") || strings.Contains(errStr, "stale CAS index") {
			AuditAggregationLog(s.handler.logger).Warn(LogEventAuditAggregationAggregationMetricStaleCASOK).
				WithFields(jobLogFieldsWithErr(s.job, err)...).
				Log()
			aggSec := time.Since(s.phaseStart).Seconds()
			s.handler.writeAuditAggregationPartialOutcome(s.job, s.phaseDurations, s.retentionOutcomeFields,
				s.retentionFirstPass, s.catchUpProcessed, 0, 0, aggSec,
				map[string]any{
					OutcomeKeyAggregationTreatedSuccess: true,
					OutcomeKeyAggregationNote:           OutcomeNoteStaleCASOrMetricExists,
				})
			return nil, nil // Treated as success
		} else {
			aggSec := time.Since(s.phaseStart).Seconds()
			AuditAggregationLog(s.handler.logger).Error(LogEventAuditAggregationAggregationFailed, err).
				JobID(s.job.ID).
				Log()
			s.handler.writeAuditAggregationPartialOutcome(s.job, s.phaseDurations, s.retentionOutcomeFields,
				s.retentionFirstPass, s.catchUpProcessed, 0, 0, aggSec,
				map[string]any{
					OutcomeKeyAggregationFailed:       true,
					OutcomeKeyAggregationFailureClass: OutcomeFailureClassAggregationError,
					OutcomeKeyAggregationError:        err.Error(),
				})
			return nil, errfmt.Newf("aggregation failed").Wrap(err)
		}
	}

	if result == nil {
		AuditAggregationLog(s.handler.logger).Info(LogEventAuditAggregationAggregationCompletedNoEvents).
			JobID(s.job.ID).
			Log()
		s.handler.writeAuditAggregationPartialOutcome(s.job, s.phaseDurations, s.retentionOutcomeFields,
			s.retentionFirstPass, s.catchUpProcessed, 0, 0,
			s.phaseDurations["aggregation"],
			nil)
		return nil, nil
	}

	AuditAggregationLog(s.handler.logger).Info(LogEventAuditAggregationAggregationCompleted).
		JobID(s.job.ID).
		Int("events_processed", result.EventCount).
		Int("metrics_created", result.MetricsCreated).
		String("metric_id", result.MetricID).
		Log()

	return result, nil
}

func (s *auditAggregationSession) runPostAggregationCleanup(result *storagepkg.AuditAggregationResult) {
	s.startPhase("post_aggregation_cleanup")
	defer s.endPhase("post_aggregation_cleanup")

	if (s.deleteEnabled || s.archiveEnabled) && len(result.EventsProcessed) > 0 {
		action := "archiving"
		if s.deleteEnabled {
			action = "deleting"
		}
		AuditAggregationLog(s.handler.logger).Info(LogEventAuditAggregationPostAggCleanupStarted).
			JobID(s.job.ID).
			Int("event_count", len(result.EventsProcessed)).
			String(aggLogKeyAction, action).
			Log()

		processedCount, err := s.service.CleanupAggregatedEvents(s.ctx, s.secCtx, result.EventsProcessed, s.archiveEnabled && !s.deleteEnabled)
		if err != nil {
			AuditAggregationLog(s.handler.logger).Warn(LogEventAuditAggregationPostAggCleanupFailed).
				WithFields(jobLogFieldsWithErr(s.job, err)...).
				Log()
		} else {
			s.postAggCleanupProcessed = processedCount
			AuditAggregationLog(s.handler.logger).Info(LogEventAuditAggregationPostAggCleanupCompleted).
				JobID(s.job.ID).
				Int(aggLogKeyProcessedCount, processedCount).
				Bool(aggLogKeyArchived, s.archiveEnabled).
				Log()
		}
	}
}

func (s *auditAggregationSession) runRetentionSecondPass() {
	s.startPhase("retention_second_pass")
	defer s.endPhase("retention_second_pass")

	if s.effectiveRetention <= 0 {
		return
	}

	cutoffTime := time.Now().UTC().Add(-s.effectiveRetention)
	AuditAggregationLog(s.handler.logger).Info(LogEventAuditAggregationRetentionSecondPassStarted).
		String(aggLogKeyCutoff, cutoffTime.Format(time.RFC3339)).
		String("retention_duration", s.effectiveRetention.String()).
		Log()

	const catchUpBatchSize = 2000
	totalProcessed := 0
	for batch := 0; batch < s.retentionMaxBatches; batch++ {
		if s.ctx.Err() != nil {
			break
		}
		if !s.hasTimeRemaining() {
			AuditAggregationLog(s.handler.logger).Info(LogEventAuditAggregationPostAggRetentionStoppingEarlyTime).
				JobID(s.job.ID).
				Int(aggLogKeyBatchesProcessed, batch).
				Int(aggLogKeyTotalProcessed, totalProcessed).
				Log()
			break
		}
		oldEventIDs, err := s.service.QueryOldAuditEventsByAge(s.ctx, s.secCtx, s.storageCtx, cutoffTime, catchUpBatchSize)
		if err != nil || len(oldEventIDs) == 0 {
			break
		}
		processedCount, err := s.service.CleanupAggregatedEvents(s.ctx, s.secCtx, oldEventIDs, s.archiveEnabled && !s.deleteEnabled)
		if err != nil {
			AuditAggregationLog(s.handler.logger).Warn(LogEventAuditAggregationRetentionSecondPassCleanupFailed).
				WithFields(jobLogFieldsWithErr(s.job, err)...).
				Log()
			break
		}
		totalProcessed += processedCount
		if processedCount < len(oldEventIDs) {
			break
		}
	}
	if totalProcessed > 0 {
		AuditAggregationLog(s.handler.logger).Info(LogEventAuditAggregationRetentionSecondPassCleanedUp).
			JobID(s.job.ID).
			Int(aggLogKeyProcessedCount, totalProcessed).
			Bool(aggLogKeyArchived, s.archiveEnabled && !s.deleteEnabled).
			Log()
	}
	s.retentionSecondPass = totalProcessed
}

func (s *auditAggregationSession) finalize(result *storagepkg.AuditAggregationResult) {
	s.startPhase("empty_bucket_cleanup")
	removeEmptyBucketDirs(s.handler.storage, objects.KindAuditEvent, s.handler.logger, s.job.ID)
	s.endPhase("empty_bucket_cleanup")

	outcome := map[string]any{
		OutcomeKeyEventsProcessed:                 result.EventCount,
		OutcomeKeyMetricsCreated:                  result.MetricsCreated,
		OutcomeKeyMetricID:                        result.MetricID,
		OutcomeKeyPhaseDurations:                  s.phaseDurations,
		OutcomeKeyRetentionFirstPassProcessed:     s.retentionFirstPass,
		OutcomeKeyCatchUpProcessed:                s.catchUpProcessed,
		OutcomeKeyPostAggregationCleanupProcessed: s.postAggCleanupProcessed,
		OutcomeKeyRetentionSecondPassProcessed:    s.retentionSecondPass,
		OutcomeKeyEventsDeletedOrArchivedTotal:    s.retentionFirstPass + s.catchUpProcessed + s.postAggCleanupProcessed + s.retentionSecondPass,
	}
	if patch := auditAggregationDegradedSuccessOutcomePatch(s.aggregationDegradedNote, nil); patch != nil {
		maps.Copy(outcome, patch)
	}
	maps.Copy(outcome, s.retentionOutcomeFields)
	WriteJobOutcome(s.handler.projectRoot, s.job.ID, JobTypeAuditEventAggregation, outcome)
	logPhaseDurations(s.handler.logger, s.job.ID, s.phaseDurations)
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

// ChangeJournalAggregationHandler aggregates change journal entries
type ChangeJournalAggregationHandler struct {
	storage storagepkg.ObjectStorageProvider
	logger  logging.Logger
}

// preExecutionHealthCheck performs health checks before job execution
// This helps identify issues that could cause job failures
func (h *ChangeJournalAggregationHandler) preExecutionHealthCheck(ctx context.Context, job *ScheduledJob, metricKind string) error {
	return preExecutionHealthCheck(ctx, h.storage, h.logger, job, metricKind)
}

// NewChangeJournalAggregationHandler creates a new change journal aggregation handler
func NewChangeJournalAggregationHandler(storage storagepkg.ObjectStorageProvider) ChangeJournalAggregationHandlerInterface {
	return &ChangeJournalAggregationHandler{
		storage: storage,
		logger:  logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// Execute aggregates change journal entries via the standardized pipeline (INGEST → NORMALIZE → COMMIT → FINALIZE).
func (h *ChangeJournalAggregationHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	ChangeJournalAggregationLog(h.logger).Info(LogEventChangeJournalAggregationStarted).
		JobID(job.ID).
		Log()

	result, err := RunChangeJournalAggregationViaPipeline(ctx, h, job)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, aggErrHashMismatch) || strings.Contains(errStr, aggErrReadHashFile) {
			ChangeJournalAggregationLog(h.logger).Warn(LogEventChangeJournalAggregationHashMismatchPartialContinue).
				WithFields(jobLogFieldsWithErr(job, err)...).
				Log()
			if result == nil {
				return errfmt.Newf("aggregation failed with hash mismatch, will retry").Wrap(err)
			}
		} else if strings.Contains(errStr, "object already exists") || strings.Contains(errStr, "stale CAS index") {
			ChangeJournalAggregationLog(h.logger).Warn(LogEventChangeJournalAggregationAggregationMetricStaleCASOK).
				WithFields(jobLogFieldsWithErr(job, err)...).
				Log()
			return nil
		}
		ChangeJournalAggregationLog(h.logger).Error(LogEventChangeJournalAggregationAggregationFailed, err).
			JobID(job.ID).
			Log()
		return errfmt.Newf("aggregation failed").Wrap(err)
	}

	if result == nil {
		ChangeJournalAggregationLog(h.logger).Info(LogEventChangeJournalAggregationAggregationCompletedNoEntries).
			JobID(job.ID).
			Log()
		return nil
	}

	ChangeJournalAggregationLog(h.logger).Info(LogEventChangeJournalAggregationAggregationCompleted).
		JobID(job.ID).
		Int("entries_processed", result.EntryCount).
		Int("metrics_created", result.MetricsCreated).
		String("metric_id", result.MetricID).
		Log()
	return nil
}

// AggregationMetricsCleanupHandler cleans up old audit_aggregation_metric objects
type AggregationMetricsCleanupHandler struct {
	storage storagepkg.ObjectStorageProvider
	logger  logging.Logger
}

// NewAggregationMetricsCleanupHandler creates a new aggregation metrics cleanup handler
func NewAggregationMetricsCleanupHandler(storage storagepkg.ObjectStorageProvider) AggregationMetricsCleanupHandlerInterface {
	return &AggregationMetricsCleanupHandler{
		storage: storage,
		logger:  logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// Execute cleans up old aggregation metrics via the pipeline (INGEST → NORMALIZE → FINALIZE).
func (h *AggregationMetricsCleanupHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	return RunAggregationMetricsCleanupViaPipeline(ctx, h, job)
}

// executeAggregationMetricsCleanupCore runs the cleanup logic. Called from RunAggregationMetricsCleanupViaPipeline NORMALIZE stage.
func (h *AggregationMetricsCleanupHandler) executeAggregationMetricsCleanupCore(ctx context.Context, job *ScheduledJob) error {
	AggregationMetricsCleanupLog(h.logger).Info(LogEventAggregationMetricsCleanupStarted).
		JobID(job.ID).
		Log()

	retentionDays := retentionDaysFromJobEnv(job.EnvironmentVariables, DefaultAggregationMetricsCleanupRetentionDays)

	if retentionDays <= 0 {
		AggregationMetricsCleanupLog(h.logger).Info(LogEventAggregationMetricsCleanupRetentionSkipNonPositive).
			JobID(job.ID).
			Log()
		return nil
	}

	cutoffTime := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)

	AggregationMetricsCleanupLog(h.logger).Info(LogEventAggregationMetricsCleanupCleaningOldMetrics).
		String(aggLogKeyCutoff, cutoffTime.Format(time.RFC3339)).
		Int(aggFieldRetentionDays, retentionDays).
		Log()

	// Get contexts
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// Query for old aggregation metrics
	filter := storagepkg.ListFilter{
		Kind: objects.KindAuditAggregationMetric,
		Filters: map[string]any{
			objects.FieldKeyCreatedAt: map[string]any{
				aggFilterOpLessThan: cutoffTime.Format(time.RFC3339), // Older than cutoff
			},
		},
		SortBy:  objects.FieldKeyCreatedAt,
		SortAsc: true,
		Limit:   0, // No limit - get all old metrics
	}

	result, err := h.storage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		AggregationMetricsCleanupLog(h.logger).Error(LogEventAggregationMetricsCleanupQueryOldMetricsFailed, err).
			JobID(job.ID).
			Log()
		return errfmt.Newf("failed to query old aggregation metrics").Wrap(err)
	}
	if result == nil || result.Objects == nil || len(result.Objects) == 0 {
		AggregationMetricsCleanupLog(h.logger).Info(LogEventAggregationMetricsCleanupNoOldMetricsFound).
			JobID(job.ID).
			String(aggLogKeyCutoff, cutoffTime.Format(time.RFC3339)).
			Log()
		return nil
	}

	// Collect metric IDs
	metricIDs := make([]string, 0, len(result.Objects))
	for _, obj := range result.Objects {
		if id, ok := obj[objects.FieldKeyID].(string); ok && id != emptyValue {
			metricIDs = append(metricIDs, id)
		}
	}

	AggregationMetricsCleanupLog(h.logger).Info(LogEventAggregationMetricsCleanupFoundOldMetrics).
		JobID(job.ID).
		MetricsFound(len(metricIDs)).
		Log()

	// Delete old metrics using bulk delete
	// Use optimized bulk delete if available (FileObjectStorage)
	if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
		deleteResult, err := fileStorage.BulkDeleteOptimized(ctx, secCtx, metricIDs, false, 20)
		if err != nil {
			AggregationMetricsCleanupLog(h.logger).Error(LogEventAggregationMetricsCleanupDeleteOldMetricsFailed, err).
				JobID(job.ID).
				MetricsFound(len(metricIDs)).
				Log()
			return errfmt.Newf("failed to delete old aggregation metrics").Wrap(err)
		}

		AggregationMetricsCleanupLog(h.logger).Info(LogEventAggregationMetricsCleanupCleanedUpOldMetrics).
			JobID(job.ID).
			MetricsFound(len(metricIDs)).
			DeletedCount(deleteResult.SuccessCount).
			FailedCount(deleteResult.FailureCount).
			Log()

		if deleteResult.FailureCount > 0 {
			AggregationMetricsCleanupLog(h.logger).Warn(LogEventAggregationMetricsCleanupSomeMetricsDeleteFailed).
				JobID(job.ID).
				FailedCount(deleteResult.FailureCount).
				Log()
		}
	} else {
		// Fallback to standard BulkDelete
		deleteResult, err := h.storage.BulkDelete(ctx, secCtx, metricIDs, false)
		if err != nil {
			AggregationMetricsCleanupLog(h.logger).Error(LogEventAggregationMetricsCleanupDeleteOldMetricsFailed, err).
				JobID(job.ID).
				MetricsFound(len(metricIDs)).
				Log()
			return errfmt.Newf("failed to delete old aggregation metrics").Wrap(err)
		}

		AggregationMetricsCleanupLog(h.logger).Info(LogEventAggregationMetricsCleanupCleanedUpOldMetrics).
			JobID(job.ID).
			MetricsFound(len(metricIDs)).
			DeletedCount(deleteResult.SuccessCount).
			FailedCount(deleteResult.FailureCount).
			Log()

		if deleteResult.FailureCount > 0 {
			AggregationMetricsCleanupLog(h.logger).Warn(LogEventAggregationMetricsCleanupSomeMetricsDeleteFailed).
				JobID(job.ID).
				FailedCount(deleteResult.FailureCount).
				Log()
		}
	}

	return nil
}

// GenericMetricsCleanupHandler cleans up old metric objects of any kind
// The metric kind is specified via job environment variable METRIC_KIND
type GenericMetricsCleanupHandler struct {
	storage storagepkg.ObjectStorageProvider
	logger  logging.Logger
}

// NewGenericMetricsCleanupHandler creates a new generic metrics cleanup handler
func NewGenericMetricsCleanupHandler(storage storagepkg.ObjectStorageProvider) GenericMetricsCleanupHandlerInterface {
	return &GenericMetricsCleanupHandler{
		storage: storage,
		logger:  logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// Execute cleans up old metrics of the specified kind via the pipeline (INGEST → NORMALIZE → FINALIZE).
func (h *GenericMetricsCleanupHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	return RunGenericMetricsCleanupViaPipeline(ctx, h, job)
}

// executeGenericMetricsCleanupCore runs the cleanup logic. Called from RunGenericMetricsCleanupViaPipeline NORMALIZE stage.
func (h *GenericMetricsCleanupHandler) executeGenericMetricsCleanupCore(ctx context.Context, job *ScheduledJob) error {
	// Get metric kind from job environment variables (required)
	metricKind := ""
	if job.EnvironmentVariables != nil {
		if kind, ok := job.EnvironmentVariables[EnvKeyMetricKind]; ok && kind != emptyValue {
			metricKind = kind
		}
	}

	if metricKind == emptyValue {
		err := errfmt.Errorf("%s environment variable is required for generic metrics cleanup", EnvKeyMetricKind)
		GenericMetricsCleanupLog(h.logger).Error(LogEventGenericMetricsCleanupMetricKindEnvNotSet, err).
			JobID(job.ID).
			Log()
		return err
	}

	GenericMetricsCleanupLog(h.logger).Info(LogEventGenericMetricsCleanupStarted).
		JobID(job.ID).
		MetricKind(metricKind).
		Log()

	retentionDays := retentionDaysFromJobEnv(job.EnvironmentVariables, DefaultGenericMetricsCleanupRetentionDays)

	if retentionDays <= 0 {
		GenericMetricsCleanupLog(h.logger).Info(LogEventGenericMetricsCleanupRetentionSkipNonPositive).
			JobID(job.ID).
			MetricKind(metricKind).
			Log()
		return nil
	}

	// Get contexts
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// CRITICAL: Check object count before cleanup to enforce 3k limit
	// If over limit, use aggressive cleanup (reduce retention by 50%)
	metricFilter := storagepkg.ListFilter{
		Kind: metricKind,
	}
	metricCount, countErr := h.storage.Count(ctx, secCtx, metricFilter)
	if countErr == nil && metricCount >= MaxAggregationObjectCountPerKind {
		// Object count at or above limit - trigger aggressive cleanup
		GenericMetricsCleanupLog(h.logger).Warn(LogEventGenericMetricsCleanupObjectCountLimitAggressive).
			JobID(job.ID).
			MetricKind(metricKind).
			Int(aggLogKeyCurrentCount, metricCount).
			Int(aggLogKeyLimit, MaxAggregationObjectCountPerKind).
			Log()

		// Reduce retention duration to 50% to free up space faster
		retentionDays /= 2
		if retentionDays < 1 {
			retentionDays = 1 // Minimum 1 day retention
		}
	}

	cutoffTime := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)

	GenericMetricsCleanupLog(h.logger).Info(LogEventGenericMetricsCleanupCleaningOldMetrics).
		JobID(job.ID).
		MetricKind(metricKind).
		String(aggLogKeyCutoff, cutoffTime.Format(time.RFC3339)).
		Int(aggFieldRetentionDays, retentionDays).
		Log()

	// Query for old metrics
	filter := storagepkg.ListFilter{
		Kind: metricKind,
		Filters: map[string]any{
			objects.FieldKeyCreatedAt: map[string]any{
				aggFilterOpLessThan: cutoffTime.Format(time.RFC3339), // Older than cutoff
			},
		},
		SortBy:  objects.FieldKeyCreatedAt,
		SortAsc: true,
		Limit:   0, // No limit - get all old metrics
	}

	result, err := h.storage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		GenericMetricsCleanupLog(h.logger).Error(LogEventGenericMetricsCleanupQueryOldMetricsFailed, err).
			JobID(job.ID).
			MetricKind(metricKind).
			Log()
		return errfmt.Newf("failed to query old metrics").Wrap(err)
	}
	if result == nil || result.Objects == nil || len(result.Objects) == 0 {
		GenericMetricsCleanupLog(h.logger).Info(LogEventGenericMetricsCleanupNoOldMetricsFound).
			JobID(job.ID).
			MetricKind(metricKind).
			String(aggLogKeyCutoff, cutoffTime.Format(time.RFC3339)).
			Log()
		return nil
	}

	// Collect metric IDs
	metricIDs := make([]string, 0, len(result.Objects))
	for _, obj := range result.Objects {
		if id, ok := obj[objects.FieldKeyID].(string); ok && id != emptyValue {
			metricIDs = append(metricIDs, id)
		}
	}

	GenericMetricsCleanupLog(h.logger).Info(LogEventGenericMetricsCleanupFoundOldMetrics).
		JobID(job.ID).
		MetricKind(metricKind).
		MetricsFound(len(metricIDs)).
		Log()

	// Delete old metrics using bulk delete
	// Use optimized bulk delete if available (FileObjectStorage)
	if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
		deleteResult, err := fileStorage.BulkDeleteOptimized(ctx, secCtx, metricIDs, false, 20)
		if err != nil {
			GenericMetricsCleanupLog(h.logger).Error(LogEventGenericMetricsCleanupDeleteOldMetricsFailed, err).
				JobID(job.ID).
				MetricKind(metricKind).
				MetricsFound(len(metricIDs)).
				Log()
			return errfmt.Errorf(aggErrFailedDeleteOldMetricsFmt, err)
		}

		GenericMetricsCleanupLog(h.logger).Info(LogEventGenericMetricsCleanupCleanedUpOldMetrics).
			JobID(job.ID).
			MetricKind(metricKind).
			MetricsFound(len(metricIDs)).
			DeletedCount(deleteResult.SuccessCount).
			FailedCount(deleteResult.FailureCount).
			Log()

		if deleteResult.FailureCount > 0 {
			GenericMetricsCleanupLog(h.logger).Warn(LogEventGenericMetricsCleanupSomeMetricsDeleteFailed).
				JobID(job.ID).
				MetricKind(metricKind).
				FailedCount(deleteResult.FailureCount).
				Log()
		}
	} else {
		// Fallback to standard BulkDelete
		deleteResult, err := h.storage.BulkDelete(ctx, secCtx, metricIDs, false)
		if err != nil {
			GenericMetricsCleanupLog(h.logger).Error(LogEventGenericMetricsCleanupDeleteOldMetricsFailed, err).
				JobID(job.ID).
				MetricKind(metricKind).
				MetricsFound(len(metricIDs)).
				Log()
			return errfmt.Errorf(aggErrFailedDeleteOldMetricsFmt, err)
		}

		GenericMetricsCleanupLog(h.logger).Info(LogEventGenericMetricsCleanupCleanedUpOldMetrics).
			JobID(job.ID).
			MetricKind(metricKind).
			MetricsFound(len(metricIDs)).
			DeletedCount(deleteResult.SuccessCount).
			FailedCount(deleteResult.FailureCount).
			Log()

		if deleteResult.FailureCount > 0 {
			GenericMetricsCleanupLog(h.logger).Warn(LogEventGenericMetricsCleanupSomeMetricsDeleteFailed).
				JobID(job.ID).
				MetricKind(metricKind).
				FailedCount(deleteResult.FailureCount).
				Log()
		}
	}

	return nil
}

// preExecutionHealthCheck performs health checks before aggregation job execution
// Checks for common issues that could cause job failures:
// - Hash mismatches in existing metrics
// - Missing objects referenced in aggregations
// - CAS inconsistencies
func preExecutionHealthCheck(ctx context.Context, storage storagepkg.ObjectStorageProvider, logger logging.Logger, job *ScheduledJob, metricKind string) error {
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// Check for existing metrics with potential issues
	// Limit to recent metrics (last 24 hours) to avoid performance impact
	filter := storagepkg.ListFilter{
		Kind: metricKind,
		Filters: map[string]any{
			objects.FieldKeyCreatedAt: map[string]any{
				"$gte": time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339),
			},
		},
		Limit: 100, // Limit check to recent metrics
	}

	result, err := storage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		// List failure is not critical - log and continue
		aggregationHealthLogRoot(logger, metricKind).Debug(healthLogEventListFailed(metricKind)).
			JobID(job.ID).
			WithError(err).
			MetricKind(metricKind).
			Log()
		return nil // Don't fail job on health check errors
	}
	if result == nil || result.Objects == nil {
		return nil
	}

	// Check a sample of metrics for hash mismatches
	// We don't check all metrics to avoid performance impact
	sampleSize := 10
	if len(result.Objects) < sampleSize {
		sampleSize = len(result.Objects)
	}

	issuesFound := 0
	for i := 0; i < sampleSize; i++ {
		obj := result.Objects[i]
		id, ok := obj[objects.FieldKeyID].(string)
		if !ok || id == emptyValue {
			continue
		}

		// Try to read the object - if it fails with hash mismatch, log it
		_, readErr := storage.Read(ctx, secCtx, id)
		if readErr != nil {
			errStr := readErr.Error()
			if strings.Contains(errStr, aggErrHashMismatch) || strings.Contains(errStr, aggErrReadHashFile) {
				issuesFound++
				aggregationHealthLogRoot(logger, metricKind).Warn(healthLogEventHashMismatchSample(metricKind)).
					JobID(job.ID).
					MetricID(id).
					MetricKind(metricKind).
					Log()
			}
		}
	}

	if issuesFound > 0 {
		aggregationHealthLogRoot(logger, metricKind).Warn(healthLogEventMetricsPotentialIssues(metricKind)).
			JobID(job.ID).
			MetricKind(metricKind).
			Int("issues_found", issuesFound).
			Int("sample_size", sampleSize).
			String("note", "Consider running system check to fix hash mismatches").
			Log()
		// Don't return error - health check is advisory
		// Job will attempt to handle these gracefully
	}

	return nil
}
