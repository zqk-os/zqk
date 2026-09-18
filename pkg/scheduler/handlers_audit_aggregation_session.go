package scheduler

import (
	"context"
	"maps"
	"strconv"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/when"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

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
		timeoutStr = s.job.EnvironmentVariables[zqkenv.AggregationMetricCreationTimeout().Name()]
	}
	when.When(func() bool { return timeoutStr == emptyValue }).Then(func() {
		timeoutStr = metricCreationTimeoutFromJobRuntime(s.job.MaxRuntimeSeconds)
	}).OrElse(func() {
		timeoutStr = normalizeDurationEnv(timeoutStr)
	}).Run()
	prev := zqkenv.AggregationMetricCreationTimeout().Get()
	if err := zqkenv.AggregationMetricCreationTimeout().Set(timeoutStr); err != nil {
		SLog(s.handler.logger).Debug("Failed to set aggregation metric creation timeout env").WithError(err).Log()
	}

	s.cleanupFuncs = append(s.cleanupFuncs, func() {
		when.When(func() bool { return prev == emptyValue }).Then(func() {
			if err := zqkenv.AggregationMetricCreationTimeout().Unset(); err != nil {
				SLog(s.handler.logger).Debug("Failed to unset aggregation metric creation timeout env").WithError(err).Log()
			}
		}).OrElse(func() {
			if err := zqkenv.AggregationMetricCreationTimeout().Set(prev); err != nil {
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
