package scheduler

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/quality"
	"github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/when"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// schedulerJobCASReconcileMinInterval bounds how often we run a full CAS scan for scheduler_job during
// watchJobChanges. Cross-process object creates can lag the list index until reconciled; see CAS_LIST_GET_CONSISTENCY.md.
const schedulerJobCASReconcileMinInterval = 45 * time.Second

// maybeReconcileSchedulerJobCASIndexBeforeReload runs EnsureCASIndexPopulatedFromScan(scheduler_job) when the
// throttle allows, so timer/manual reload sees jobs created by another process (e.g. zqk object create).
func (s *Scheduler) maybeReconcileSchedulerJobCASIndexBeforeReload(ctx context.Context) {
	if s == nil {
		return
	}
	now := time.Now()
	shouldRun := false
	_ = concurrency.RunInLockWithLogger(
		&s.schedulerJobCASReconcileMu,
		LockNameSchedulerCASReconcileThrottle,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if !s.lastSchedulerJobCASReconcileAt.IsZero() &&
				now.Sub(s.lastSchedulerJobCASReconcileAt) < schedulerJobCASReconcileMinInterval {
				return nil
			}
			shouldRun = true
			return nil
		},
	)
	if !shouldRun {
		return
	}

	if err := s.ReconcileSchedulerJobCASIndex(ctx); err != nil {
		SchedulerLifecycleLog(s.logger).Debug(LogEventSchedulerLifecycleSchedulerJobCASReconcileReloadFailed).
			WithError(err).
			Log()
		return
	}
	_ = concurrency.RunInLockWithLogger(
		&s.schedulerJobCASReconcileMu,
		LockNameSchedulerCASReconcileThrottle,
		logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			s.lastSchedulerJobCASReconcileAt = time.Now()
			return nil
		},
	)
}

// watchJobChanges watches for changes to scheduler_job objects and reloads schedule.
// Job definitions (including environment_variables) are re-read from storage every 30s,
// so config changes (e.g. ARCHIVE_ENABLED, DELETE_ENABLED) take effect without restart.
func (s *Scheduler) watchJobChanges(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Reload jobs if scheduler is running (reload=true: do not re-submit one_time immediate jobs)
			if s.IsRunning() {
				s.maybeReconcileSchedulerJobCASIndexBeforeReload(ctx)
				if err := s.loadAndScheduleJobs(ctx, true); err != nil {
					SchedulerLifecycleLog(s.logger).Warn(LogEventSchedulerLifecycleReloadJobsFailed).
						WithError(err).
						Log()
				}
			}
		}
	}
}

// configWatchInterval is how often the daemon checks for config changes or reload requests.
// Short enough that "zqk scheduler config --no-jobs-paused" is picked up within a few seconds.
const configWatchInterval = 2 * time.Second

// Pipeline kinds are used to keep stage metrics/logging grouped by orchestration flow.
const pipelineKindCheckAndRecoverMissedJobsCompletionMap = "scheduler.check_and_recover_missed_jobs_completion_map"

// watchSchedulerConfig watches for config reload requests (from CLI) and for config file changes.
// When the CLI runs "scheduler config --no-jobs-paused" it writes a reload request file; the daemon
// sees it within configWatchInterval and applies the change immediately. Also applies if jobs_paused
// was edited on disk.
func (s *Scheduler) watchSchedulerConfig(ctx context.Context) {
	if s.projectRoot == emptyValue {
		return
	}
	ticker := time.NewTicker(configWatchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !s.IsRunning() {
				continue
			}
			// If CLI requested reload (wrote reload_config_request), consume it and reload
			if ReloadConfigRequestExists(s.projectRoot) {
				ConsumeReloadConfigRequest(s.projectRoot)
				SchedulerLifecycleLog(s.logger).Debug(LogEventSchedulerLifecycleConfigReloadCLIRequested).Log()
			}
			config, err := LoadSchedulerConfig(s.projectRoot)
			if err != nil {
				SchedulerLifecycleLog(s.logger).Debug(LogEventSchedulerLifecycleLoadConfigWatchFailed).
					WithError(err).
					Log()
				continue
			}
			// Apply jobs_paused change so config --no-jobs-paused takes effect without restart
			if config.JobsPaused != s.onlyManualJobs {
				s.SetOnlyManualJobs(config.JobsPaused)
				SchedulerLifecycleLog(s.logger).Info(LogEventSchedulerLifecycleConfigUpdated).
					Bool("jobs_paused", config.JobsPaused).
					Log()
				if !config.JobsPaused {
					// Transition to unpaused: re-schedule timer and immediate jobs
					if err := s.loadAndScheduleJobs(ctx, true); err != nil {
						SchedulerLifecycleLog(s.logger).Warn(LogEventSchedulerLifecycleRescheduleAfterUnpauseFailed).
							WithError(err).
							Log()
					} else {
						SchedulerLifecycleLog(s.logger).Info(LogEventSchedulerLifecycleTimerJobsRescheduledFlowing).Log()
					}
				}
			}
			s.scanAdmissionTimeouts(ctx)
		}
	}
}

// keepAliveHeartbeat periodically writes a keep-alive timestamp
// This allows external health checks to detect if the daemon is alive
func (s *Scheduler) keepAliveHeartbeat(ctx context.Context) {
	if s.projectRoot == emptyValue {
		return // Can't write keep-alive without project root
	}

	interval := DefaultKeepAliveInterval
	if s.keepAliveTickerInterval > 0 {
		interval = s.keepAliveTickerInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Write initial keep-alive
	_ = writeKeepAlive(s.projectRoot) //nolint:errcheck // Keep-alive write errors are non-critical

	for {
		select {
		case <-ctx.Done():
			// Remove keep-alive file on shutdown
			_ = removeKeepAlive(s.projectRoot) //nolint:errcheck // Cleanup errors are non-critical
			return
		case <-ticker.C:
			if s.IsRunning() {
				// Only write keep-alive if scheduler is actually running
				if err := writeKeepAlive(s.projectRoot); err != nil {
					SchedulerLifecycleLog(s.logger).Debug(LogEventSchedulerLifecycleKeepAliveWriteFailed).
						WithError(err).
						Log()
				}
			}
		}
	}
}

// healthMonitor monitors job execution health and recovers from missed triggers
func (s *Scheduler) healthMonitor(ctx context.Context) {
	// Check every 2 minutes for missed jobs
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !s.IsRunning() {
				continue
			}

			healthStart := time.Now()

			goroutineCount := runtime.NumGoroutine()
			runtimeThreadCount := GetRuntimeThreadCount()
			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)
			heapAllocBytes := int(memStats.Alloc) //nolint:gosec
			sysMemoryBytes := int(memStats.Sys)   //nolint:gosec

			// Warn when high so operators see "things slowing down" before ceiling blocks new work
			if goroutineCount > concurrency.GoroutineCountWarningThreshold {
				SchedulerLifecycleLog(s.logger).Warn(LogEventSchedulerLifecycleHighGoroutineCount).
					Int("goroutine_count", goroutineCount).
					Int("warning_threshold", concurrency.GoroutineCountWarningThreshold).
					Int("ceiling", concurrency.DefaultGoroutineCeiling).
					Log()
			}

			// Warn when heap memory is too high (memory bloat early warning)
			if heapAllocBytes > concurrency.HeapAllocWarningThresholdBytes {
				SchedulerLifecycleLog(s.logger).Warn(LogEventSchedulerLifecycleHighHeapMemory).
					Int("heap_alloc_bytes", heapAllocBytes).
					Int("warning_threshold_bytes", concurrency.HeapAllocWarningThresholdBytes).
					Int("sys_memory_bytes", sysMemoryBytes).
					Log()
			}

			// Check cron is running
			if s.cron == nil {
				SchedulerLifecycleLog(s.logger).Error(LogEventSchedulerLifecycleCronNilRestarting,
					errfmt.Errorf("cron scheduler unexpectedly nil")).
					Log()
				// Attempt to restart cron
				s.cron = cron.New(cron.WithSeconds(), cron.WithLocation(time.UTC))
				s.cron.Start()
				// Reload jobs to reschedule them (reload=true: do not re-submit one_time immediate)
				loadErr := s.loadAndScheduleJobs(ctx, true)
				when.When(func() bool { return loadErr != nil }).Then(func() {
					SchedulerLifecycleLog(s.logger).Error(LogEventSchedulerLifecycleReloadJobsAfterCronRestartFailed,
						loadErr).
						String("error", loadErr.Error()).
						Log()
				}).OrElse(func() {
					SchedulerLifecycleLog(s.logger).Info(LogEventSchedulerLifecycleCronRestartedJobsReloaded).Log()
				}).Run()
				s.recordHealthMetric(ctx, healthStart, 1, 0, 0, 1, 0, goroutineCount, runtimeThreadCount, heapAllocBytes, sysMemoryBytes)
				continue
			}

			// Check for missed triggers and recover
			missedCount, recoveredCount := s.checkAndRecoverMissedJobs(ctx)

			// Periodic stale lock sweep (clean unheld lock files older than 5 minutes)
			if s.projectRoot != emptyValue {
				_, _ = s.CleanStaleLocks(5 * time.Minute)
			}

			healthDuration := time.Since(healthStart)
			s.recordHealthMetric(ctx, healthStart, 0, missedCount, recoveredCount, 0, healthDuration, goroutineCount, runtimeThreadCount, heapAllocBytes, sysMemoryBytes)
		}
	}
}


// checkAndRecoverMissedJobs checks for missed job triggers and recovers them
// Returns: (missedCount, recoveredCount)
// checkAndRecoverMissedJobs checks for missed job triggers and attempts recovery
//
//nolint:gocyclo // Function orchestrates job detection and recovery; complexity reduced via helper methods
func (s *Scheduler) checkAndRecoverMissedJobs(ctx context.Context) (checkedCount, recoveredCount int) {
	now := time.Now().UTC()
	maxLookback := 1 * time.Hour

	// Collect enabled timer jobs
	timerJobs, hasEnabledTimerJobs := s.collectTimerJobs()

	// Check if daemon should be running but isn't
	if hasEnabledTimerJobs && !s.IsRunning() {
		SchedulerLifecycleLog(s.logger).Error(LogEventSchedulerLifecycleCriticalDaemonNotRunningTimerJobs,
			errfmt.Errorf("daemon should be running to execute scheduled jobs")).
			Int("enabled_timer_jobs", len(timerJobs)).
			Log()
		s.recordHealthMetric(ctx, now, 1, len(timerJobs), 0, 0, 0, runtime.NumGoroutine(), GetRuntimeThreadCount(), 0, 0)
		return len(timerJobs), 0
	}

	// If scheduler isn't running and no timer jobs, nothing to check
	if !s.IsRunning() {
		return 0, 0
	}

	// When jobs_paused (onlyManualJobs), do not recover missed timer jobs; they would bypass the pause.
	if s.onlyManualJobs {
		return 0, 0
	}

	// Prefer activity cache for completion times to avoid slow audit_event List (e.g. 904s with many events).
	type completionMapBuildState struct {
		auditResult     *storagepkg.QueryResult
		lastCompletions map[string]time.Time
		abort           bool
	}

	logger := s.logger
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}

	pl := pipeline.NewBuilder(pipelineKindCheckAndRecoverMissedJobsCompletionMap, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage(pipeline.StageIngest, func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*completionMapBuildState](payload)
			if !ok {
				in = &completionMapBuildState{}
			}
			in.lastCompletions = s.buildCompletionMapFromActivityCache()
			return in, nil
		}).
		AddStage(pipeline.StageCommit, func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*completionMapBuildState](payload)
			if !ok {
				return &completionMapBuildState{abort: true}, nil
			}
			if len(in.lastCompletions) != 0 {
				return in, nil
			}

			// Cache empty: query audit events as slow-path.
			var err error
			in.auditResult, err = s.queryJobCompletionEvents(pctx.Ctx)
			if err != nil {
				SchedulerLifecycleLog(s.logger).Warn(LogEventSchedulerLifecycleQueryAuditEventsHealthCheckFailed).
					WithError(err).
					Log()
				in.abort = true
				return in, nil
			}
			in.lastCompletions = s.buildCompletionMap(in.auditResult)
			return in, nil
		}).
		AddStage(pipeline.StageFinalize, func(pctx *pipeline.Context, payload any) (any, error) {
			return payload, nil
		}).
		Build()

	st := &completionMapBuildState{}
	out, runErr := pl.Run(&pipeline.Context{Ctx: ctx, Outcome: make(map[string]any)}, st)
	if runErr != nil {
		return 0, 0
	}
	ds, ok := nildecode.DecodeNonNilPayload[*completionMapBuildState](out)
	if !ok || ds.abort {
		return 0, 0
	}

	auditResult := ds.auditResult
	lastCompletions := ds.lastCompletions
	if lastCompletions == nil {
		lastCompletions = map[string]time.Time{}
	}

	// Check each job for missed triggers and recover
	recoveredCount = 0
	for _, job := range timerJobs {
		if s.isJobMissed(job, now, maxLookback, lastCompletions, auditResult) {
			s.recoverMissedJob(job)
			recoveredCount++
		}
	}

	// Count missed jobs (for metrics)
	missedCount := s.countMissedJobs(timerJobs, now, maxLookback, lastCompletions, auditResult)

	return missedCount, recoveredCount
}

// collectTimerJobs collects enabled timer jobs
func (s *Scheduler) collectTimerJobs() ([]*ScheduledJob, bool) {
	var timerJobs []*ScheduledJob
	var hasEnabledTimerJobs bool
	_ = concurrency.RunInRLockWithLogger(
		&s.jobsMu, LockNameSchedulerCollectTimerJobs, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			timerJobs = make([]*ScheduledJob, 0)
			hasEnabledTimerJobs = false
			for _, job := range s.jobs {
				if job.TriggerType == "timer" && job.Enabled && job.ScheduleExpr != emptyValue {
					timerJobs = append(timerJobs, job)
					hasEnabledTimerJobs = true
				}
			}
			return nil
		},
	)
	return timerJobs, hasEnabledTimerJobs
}

// queryJobCompletionEvents queries audit events for job completions
// Uses high-volume event cache when available for faster queries
func (s *Scheduler) queryJobCompletionEvents(ctx context.Context) (*storagepkg.QueryResult, error) {
	secCtx := s.secCtx
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}

	storageCtx := pkgctx.NewStorageContext()
	filters := map[string]any{
		objects.FieldKeyTargetKind: objects.KindSchedulerJob,
	}

	// Try high-volume event cache first (fast path)
	cache := storagepkg.GetGlobalHighVolumeEventCache()
	if fileStorage, ok := s.storage.(*storagepkg.FileObjectStorage); ok {
		projectRoot := fileStorage.GetProjectRoot()
		if cache.IsPopulatedForProject(projectRoot) {
			// Query recent events (last 7 days) from cache
			weekAgo := time.Now().UTC().Add(-7 * 24 * time.Hour)
			cacheEventIDs := cache.QueryByTimeWindow(weekAgo, time.Now().UTC().Add(24*time.Hour), 1000)
			if len(cacheEventIDs) > 0 {
				// Read events from cache IDs
				matchedObjs := make([]map[string]any, 0, len(cacheEventIDs))
				for _, id := range cacheEventIDs {
					if obj, err := s.storage.Read(ctx, secCtx, id); err == nil {
						// Filter by target_kind in memory (cache doesn't filter by target_kind)
						if targetKind, _ := obj[objects.FieldKeyTargetKind].(string); targetKind == objects.KindSchedulerJob {
							matchedObjs = append(matchedObjs, obj)
						}
					}
				}
				return &storagepkg.QueryResult{Objects: matchedObjs}, nil
			}
		}
	}

	// Fallback to storage query
	return s.storage.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{
		Kind:    objects.KindAuditEvent,
		Filters: filters,
		SortBy:  "created_at",
		SortAsc: false,
		Limit:   1000, // Get recent events
	})
}

// buildCompletionMapFromActivityCache builds last completion times from the in-memory activity cache.
// Used by the health check to avoid a slow audit_event List when the daemon has been running.
func (s *Scheduler) buildCompletionMapFromActivityCache() map[string]time.Time {
	lastCompletions := make(map[string]time.Time)
	for jobID, entry := range GetGlobalActivityCache().GetAllEntries() {
		var last time.Time
		if !entry.LastCompleted.IsZero() {
			last = entry.LastCompleted
		}
		if !entry.LastFailed.IsZero() && entry.LastFailed.After(last) {
			last = entry.LastFailed
		}
		if !last.IsZero() {
			lastCompletions[jobID] = last
		}
	}
	return lastCompletions
}

// buildCompletionMap builds a map of last completion times by job ID from audit query result
func (s *Scheduler) buildCompletionMap(auditResult *storagepkg.QueryResult) map[string]time.Time {
	lastCompletions := make(map[string]time.Time)
	for _, event := range auditResult.Objects {
		eventType := getStringFromMap(event, "event_type")
		if eventType != "scheduler_job_completed" && eventType != "scheduler_job_failed" {
			continue
		}
		targetID := getStringFromMap(event, "target_id")
		if targetID == emptyValue {
			continue
		}
		if createdAtStr := getStringFromMap(event, "created_at"); createdAtStr != emptyValue {
			if t, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
				if lastCompletions[targetID].IsZero() || t.After(lastCompletions[targetID]) {
					lastCompletions[targetID] = t
				}
			}
		}
	}
	return lastCompletions
}

// isJobMissed checks if a job has missed its trigger
func (s *Scheduler) isJobMissed(job *ScheduledJob, now time.Time, maxLookback time.Duration, lastCompletions map[string]time.Time, auditResult *storagepkg.QueryResult) bool {
	schedule, err := s.parseCronSchedule(job.ScheduleExpr)
	if err != nil {
		return false // Skip invalid cron
	}

	lastRun := lastCompletions[job.ID]
	if lastRun.IsZero() {
		lastRun = now.Add(-1 * time.Hour)
	}

	checkTime := lastRun
	if now.Sub(lastRun) > maxLookback {
		checkTime = now.Add(-maxLookback)
	}

	nextExpected := schedule.Next(checkTime)
	gracePeriod := 5 * time.Minute

	if !nextExpected.Before(now.Add(-gracePeriod)) {
		return false // Not missed yet
	}

	// Check if there's a completion event for this expected time
	return !s.hasCompletionEventForTime(job.ID, nextExpected, gracePeriod, auditResult, lastCompletions[job.ID])
}

// hasCompletionEventForTime checks if there's a completion event within grace period.
// When auditResult is nil (cache-only path), lastCompletion is used to approximate.
func (s *Scheduler) hasCompletionEventForTime(jobID string, expectedTime time.Time, gracePeriod time.Duration, auditResult *storagepkg.QueryResult, lastCompletion time.Time) bool {
	if auditResult == nil {
		if lastCompletion.IsZero() {
			return false
		}
		diff := lastCompletion.Sub(expectedTime)
		return diff >= 0 && diff <= gracePeriod
	}
	for _, event := range auditResult.Objects {
		eventTargetID := getStringFromMap(event, "target_id")
		if eventTargetID != jobID {
			continue
		}
		eventType := getStringFromMap(event, "event_type")
		if eventType != "scheduler_job_completed" && eventType != "scheduler_job_failed" {
			continue
		}
		if eventTimeStr := getStringFromMap(event, "created_at"); eventTimeStr != emptyValue {
			if eventTime, err := time.Parse(time.RFC3339, eventTimeStr); err == nil {
				diff := eventTime.Sub(expectedTime)
				if diff >= 0 && diff <= gracePeriod {
					return true
				}
			}
		}
	}
	return false
}

// ParseCronSchedule parses a cron expression, handling both 5 and 6 field formats.
func ParseCronSchedule(expr string) (cron.Schedule, error) {
	if len(strings.Fields(expr)) == 5 {
		expr = "0 " + expr
	}

	specParser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	return specParser.Parse(expr)
}

// parseCronSchedule parses a cron expression, handling both 5 and 6 field formats
func (s *Scheduler) parseCronSchedule(expr string) (cron.Schedule, error) {
	return ParseCronSchedule(expr)
}


// recoverMissedJob attempts to recover a missed job by submitting to the bounded pool (same as cron/event/lifecycle).
func (s *Scheduler) recoverMissedJob(job *ScheduledJob) {
	SchedulerLifecycleLog(s.logger).Warn(LogEventSchedulerLifecycleMissedJobTriggerRecovery).
		JobID(job.ID).
		SchedulerJobType(job.JobType).
		Log()

	if s.triggeredPool == nil {
		return
	}
	handler := s.createJobHandler(job)
	recoveryCtx, cancel := dispatchContextForScheduledJob(job)

	work := triggeredJobWork{job: job, handler: handler, ctx: recoveryCtx, cancel: cancel}
	if err := s.submitTriggeredJob(work); err != nil {
		cancel()
		return
	}
}

// countMissedJobs counts how many jobs have missed their triggers
func (s *Scheduler) countMissedJobs(timerJobs []*ScheduledJob, now time.Time, maxLookback time.Duration, lastCompletions map[string]time.Time, auditResult *storagepkg.QueryResult) int {
	missedCount := 0
	for _, job := range timerJobs {
		if s.isJobMissed(job, now, maxLookback, lastCompletions, auditResult) {
			missedCount++
		}
	}
	return missedCount
}

// getStringFromMap safely gets a string value from a map
func getStringFromMap(m map[string]any, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// recordHealthMetric records scheduler health metrics as a metric object.
// Includes runtime goroutine and OS thread count every cycle for thread explosion diagnosis.
func (s *Scheduler) recordHealthMetric(ctx context.Context, timestamp time.Time, healthChecks, missedTriggers, recoveredJobs, cronRestarts int, healthCheckDuration time.Duration, goroutineCount, runtimeThreadCount, heapAllocBytes, sysMemoryBytes int) {
	// Record every cycle so we get a time series of goroutine/thread counts; skip only if no runtime data at all
	if goroutineCount == 0 && runtimeThreadCount == 0 && healthChecks == 0 && missedTriggers == 0 && recoveredJobs == 0 && cronRestarts == 0 {
		return
	}

	secCtx := s.secCtx
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}

	// Build metric using instance builder (created_by/updated_by set by BaseInstanceBuilder defaults) (same pattern as audit_aggregation, file_lock_metrics_collector).
	metricID := fmt.Sprintf("SHM-%d", timestamp.Unix())
	timestampStr := zqktime.FormatLayoutUTC(timestamp, zqktime.LayoutObjectDateTimeZ)

	titleParts := []string{}
	if healthChecks > 0 {
		titleParts = append(titleParts, fmt.Sprintf("%d health check(s)", healthChecks))
	}
	if missedTriggers > 0 {
		titleParts = append(titleParts, fmt.Sprintf("%d missed trigger(s)", missedTriggers))
	}
	if recoveredJobs > 0 {
		titleParts = append(titleParts, fmt.Sprintf("%d recovered job(s)", recoveredJobs))
	}
	if cronRestarts > 0 {
		titleParts = append(titleParts, fmt.Sprintf("%d cron restart(s)", cronRestarts))
	}
	title := "Scheduler Health Metric"
	if len(titleParts) > 0 {
		title = fmt.Sprintf("Scheduler Health: %s", strings.Join(titleParts, ", "))
	}

	builder := instance_builders.NewForKind(objects.KindSchedulerHealthMetric, objects.DefaultSchemaVersion)
	builder.SetID(metricID)
	builder.SetStatus("active")
	builder.SetField(objects.FieldKeyTitle, title)
	builder.SetField(objects.FieldKeyMetricType, "system")
	builder.SetField(objects.FieldKeySource, "scheduler")
	builder.SetField(objects.FieldKeyCollectionCount, 1)
	builder.SetField(objects.FieldKeyFirstSeen, timestampStr)
	builder.SetField(objects.FieldKeyLastSeen, timestampStr)
	builder.SetField(objects.FieldKeyTags, []string{"scheduler", "health", "monitoring"})
	builder.SetField(objects.FieldKeyHealthChecks, healthChecks)
	builder.SetField(objects.FieldKeyMissedTriggers, missedTriggers)
	builder.SetField(objects.FieldKeyRecoveredJobs, recoveredJobs)
	builder.SetField(objects.FieldKeyCronRestarts, cronRestarts)
	builder.SetField(objects.FieldKeyHealthCheckDurationMs, float64(healthCheckDuration.Milliseconds()))
	builder.SetField(objects.FieldKeyGoroutineCount, goroutineCount)
	builder.SetField(objects.FieldKeyRuntimeThreadCount, runtimeThreadCount)
	builder.SetField(objects.FieldKeyHeapAllocBytes, heapAllocBytes)
	builder.SetField(objects.FieldKeySysMemoryBytes, sysMemoryBytes)
	builder.SetField(objects.FieldKeyMeasurementWindowStart, timestampStr)
	builder.SetField(objects.FieldKeyMeasurementWindowEnd, timestampStr)

	metricData, buildErr := builder.Build()
	if buildErr != nil {
		SchedulerLifecycleLog(s.logger).Debug(LogEventSchedulerLifecycleHealthMetricBuildFailed).
			WithError(buildErr).
			String("metric_id", metricID).
			Log()
		return
	}

	// Create metric synchronously (best effort). Avoids one goroutine per health cycle.
	createErr := s.storage.Create(ctx, secCtx, metricData)
	when.When(func() bool { return createErr != nil }).Then(func() {
		SchedulerLifecycleLog(s.logger).Debug(LogEventSchedulerLifecycleHealthMetricRecordFailed).
			WithFields(append(logErrField(createErr), logging.String("metric_id", metricID))...).
			Log()
	}).OrElse(func() {
		SchedulerLifecycleLog(s.logger).Debug(LogEventSchedulerLifecycleHealthMetricRecorded).
			String("metric_id", metricID).
			Int("health_checks", healthChecks).
			Int("missed_triggers", missedTriggers).
			Int("recovered_jobs", recoveredJobs).
			Int("cron_restarts", cronRestarts).
			Log()
	}).Run()

	// Emit coordination event for observability (audit, logging, operational)
	// Note: Metric object creation happens via storage.Create() above (async)
	// Coordinator provides unified observability across channels
	emitSchedulerHealthMetricViaCoordinator(
		ctx,
		s.projectRoot,
		s.storage,
		metricID,
		timestamp,
		healthChecks,
		missedTriggers,
		recoveredJobs,
		cronRestarts,
		healthCheckDuration,
		goroutineCount,
		runtimeThreadCount,
		heapAllocBytes,
		sysMemoryBytes,
		"scheduler",
		string(pkgctx.ProfileSystem), // Default profile for scheduler (system operation)
	)
	// Proactive issue detector: clear issues file when no recent failures (so "ok" is visible when healthy)
	ClearIssuesIfOk(s.projectRoot)
}

// TriggerJobWithCallback manually triggers a job execution with a provided OperationCallback.
// The callback will be invoked in addition to the default coordinator callback.
func (s *Scheduler) TriggerJobWithCallback(ctx context.Context, jobID string, opCallback concurrency.OperationCallback) error {
	return s.TriggerJob(ContextWithOperationCallback(ctx, opCallback), jobID)
}

// JobInCache returns true if the job ID is present in the scheduler's loaded job set.
// Used by the trigger queue to avoid calling TriggerJob for jobs that do not exist (e.g. stale queue entries).
func (s *Scheduler) JobInCache(jobID string) bool {
	_, ok := s.lookupJobInCache(jobID)
	return ok
}

// lookupJobInCache returns the job from the in-memory cache if present.
func (s *Scheduler) lookupJobInCache(jobID string) (*ScheduledJob, bool) {
	var job *ScheduledJob
	var ok bool
	_ = concurrency.RunInRLockWithLogger(
		&s.jobsMu, LockNameSchedulerJobInCache, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			job, ok = s.jobs[jobID]
			return nil
		},
	)
	return job, ok
}

// TriggerJob manually triggers a job execution (for manual, workflow, or event triggers).
// If the job is not in cache, ReloadJobs is called once so newly created jobs (e.g. from another process) become visible.
func (s *Scheduler) TriggerJob(ctx context.Context, jobID string) error {
	// Check permission to execute jobs
	if err := s.checkPermission("execute:scheduler_job"); err != nil {
		return errfmt.Newf("permission denied").Wrap(err)
	}

	job, ok := s.lookupJobInCache(jobID)
	if !ok {
		// Refresh job list once so cache is valid (e.g. job created by another process or trigger queue batch).
		if reloadErr := s.ReloadJobs(ctx); reloadErr != nil {
			return errfmt.Errorf("job not found: %s (reload failed: %w)", jobID, reloadErr)
		}
		job, ok = s.lookupJobInCache(jobID)
	}
	if !ok {
		// CAS list index can lag cross-process creates (object get works; List omits id until index catches up).
		// Sync scan + second reload — same idea as trigger-queue batchNeedsCASReconcileBeforeTriggerReload.
		if recErr := s.ReconcileSchedulerJobCASIndex(ctx); recErr != nil {
			SchedulerLifecycleLog(s.logger).Debug(LogEventSchedulerLifecycleCASReconcileSecondReloadFailed).
				JobID(jobID).
				WithError(recErr).
				Log()
		}
		if reloadErr := s.ReloadJobs(ctx); reloadErr != nil {
			return errfmt.Errorf("job not found: %s (reload after CAS reconcile failed: %w)", jobID, reloadErr)
		}
		job, ok = s.lookupJobInCache(jobID)
	}
	if !ok {
		// Get/Read can succeed while List still omits the id (CAS index lag).
		if s.AdmitJobFromStorage(ctx, jobID) {
			job, ok = s.lookupJobInCache(jobID)
		}
	}
	if !ok {
		return errfmt.Errorf("job not found: %s", jobID)
	}

	if !job.Enabled {
		return errfmt.Errorf("job is disabled: %s", jobID)
	}

	// Validate trigger type allows manual/queue triggering
	// immediate: used by trigger queue (e.g. SCH-run-* leftover jobs); timer/manual/workflow/event/lifecycle for CLI or events
	if job.TriggerType != TriggerTypeManual && job.TriggerType != TriggerTypeWorkflow && job.TriggerType != TriggerTypeEvent && job.TriggerType != TriggerTypeLifecycle && job.TriggerType != TriggerTypeTimer && job.TriggerType != TriggerTypeImmediate {
		return errfmt.Errorf("job trigger_type (%s) does not support manual triggering", job.TriggerType)
	}

	// Create job handler
	handler := s.createJobHandler(job)

	SchedulerLifecycleLog(s.logger).Info(LogEventSchedulerLifecycleManualTriggerJob).
		JobID(jobID).
		String("trigger_type", job.TriggerType).
		Log()

	// Ensure manual trigger updates next_run_at for timer jobs (especially perpetual ones)
	if job.TriggerType == TriggerTypeTimer || job.TriggerType == "timer" {
		s.updateJobInStorage(ctx, job)
	}

	// Dispatch context uses a long outer bound for queue/ceiling; execution timeout is in executeJob from run start.
	jobCtx, cancel := dispatchContextForScheduledJob(job)
	var opCB concurrency.OperationCallback
	if v := ctx.Value(operationCallbackContextKey{}); v != nil {
		if c, ok := v.(concurrency.OperationCallback); ok {
			opCB = c
		}
	}
	jobCtx = ContextWithOperationCallback(jobCtx, opCB)
	if origin := TriggerOriginFromContext(ctx); origin != emptyValue {
		jobCtx = ContextWithTriggerOrigin(jobCtx, origin)
	}

	// Use bounded worker pool when scheduler is started to avoid one goroutine per manual trigger (thread explosion).
	if s.triggeredPool != nil {
		work := triggeredJobWork{
			job:         job,
			handler:     handler,
			ctx:         jobCtx,
			cancel:      cancel,
			nonBlocking: submitNonBlockingFromContext(ctx),
		}
		if err := s.submitTriggeredJob(work); err != nil {
			cancel()
			if errors.Is(err, context.DeadlineExceeded) {
				s.recordDispatchAttemptDropped("trigger_job_submit", dispatchPressureReasonResourceWaitDeadlineExceeded, job)
			}
			return err
		}
		return nil
	}

	// Fallback when scheduler not started (e.g. tests): run in a single goroutine
	fallbackBuilder := goroutinelabels.NewGoroutine("scheduler_job_executor", fmt.Sprintf("executing job %s triggered by event", job.ID))
	if goroutinelabels.DefaultBudget() != nil {
		fallbackBuilder = fallbackBuilder.WithBudget(goroutinelabels.DefaultBudget())
	}
	fallbackBuilder.StartWithContext(jobCtx, func(execCtx context.Context) error {
		defer cancel()
		s.executeJob(execCtx, job, handler)
		return nil
	})
	return nil
}

// TriggerJobByEvent triggers a job based on an event (for event trigger_type)
func (s *Scheduler) TriggerJobByEvent(ctx context.Context, eventType, eventKind string, eventData map[string]any) error {
	var jobsToTrigger []*ScheduledJob
	_ = concurrency.RunInRLockWithLogger(
		&s.jobsMu, LockNameSchedulerTriggerJobByEvent, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Find jobs with event trigger_type that match the event filter
			for _, job := range s.jobs {
				if job.TriggerType != "event" || !job.Enabled {
					continue
				}

				// Simple event filter matching (can be enhanced)
				// Format: "event_type:kind" or "event_type" or "*"
				if s.matchesEventFilter(job.EventFilter, eventType, eventKind) {
					jobsToTrigger = append(jobsToTrigger, job)
				}
			}
			return nil
		},
	)

	// Submit to bounded pool (avoids one goroutine per matching job)
	if s.triggeredPool == nil {
		return nil
	}
	for _, job := range jobsToTrigger {

		handler := s.createJobHandler(job)
		jobCtx, cancel := dispatchContextForScheduledJob(job)
		jobCtx = context.WithValue(jobCtx, evtDataKey{}, eventData)
		var opCB concurrency.OperationCallback
		if v := ctx.Value(operationCallbackContextKey{}); v != nil {
			if c, ok := v.(concurrency.OperationCallback); ok {
				opCB = c
			}
		}
		jobCtx = ContextWithOperationCallback(jobCtx, opCB)
		work := triggeredJobWork{job: job, handler: handler, ctx: jobCtx, cancel: cancel}
		if err := s.submitTriggeredJob(work); err != nil {
			cancel()
			return err
		}
	}
	return nil
}

// matchesEventFilter checks if an event matches a job's event filter
func (s *Scheduler) matchesEventFilter(filter, eventType, eventKind string) bool {
	if filter == emptyValue || filter == "*" {
		return true // Match all events
	}

	// Parse filter format: "event_type:kind" or "event_type" or "*"
	parts := strings.Split(filter, ":")
	if len(parts) == 2 {
		// Format: "event_type:kind"
		return parts[0] == eventType && parts[1] == eventKind
	} else if len(parts) == 1 {
		// Format: "event_type" (matches any kind)
		return parts[0] == eventType
	}

	return false
}

// TriggerJobByLifecycle triggers a job based on a lifecycle state transition
func (s *Scheduler) TriggerJobByLifecycle(ctx context.Context, kind, fromState, toState string, objectData map[string]any) error {
	// Update any matching matrices registered with lifecycle triggers
	goroutinelabels.NewGoroutine("update_matrices_on_lifecycle", "updating quality matrices from lifecycle event").
		StartSimple(func() {
			s.handleLifecycleMatrixTriggers(pkgctx.NewSystemContext(), kind, fromState, toState, objectData)
		})

	// Auto-complete milestones when all child backlog items are completed
	if kind == "backlog_item" && objects.GetGlobalStatusChecker().IsTerminal(kind, toState) {
		goroutinelabels.NewGoroutine("auto_transition_milestones", "checking and auto-completing milestones").
			StartSimple(func() {
				s.autoCompleteMilestonesForBacklogItem(pkgctx.NewSystemContext(), objectData)
			})
	}

	var jobsToTrigger []*ScheduledJob
	_ = concurrency.RunInRLockWithLogger(
		&s.jobsMu, LockNameSchedulerTriggerJobByLifecycle, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Find jobs with lifecycle trigger_type that match the lifecycle filter
			for _, job := range s.jobs {
				if job.TriggerType != TriggerTypeLifecycle || !job.Enabled {
					continue
				}

				// Check if lifecycle filter matches
				if s.matchesLifecycleFilter(job.LifecycleFilter, kind, fromState, toState) {
					jobsToTrigger = append(jobsToTrigger, job)
				}
			}
			return nil
		},
	)

	// Submit to bounded pool (avoids one goroutine per matching job)
	if s.triggeredPool == nil {
		return nil
	}
	for _, job := range jobsToTrigger {
		// Don't add more work when already overloaded (circuit breaker), except priority-dispatch jobs which bypass so they can run/timeout

		handler := s.createJobHandler(job)
		jobCtx, cancel := dispatchContextForScheduledJob(job)
		var opCB concurrency.OperationCallback
		if v := ctx.Value(operationCallbackContextKey{}); v != nil {
			if c, ok := v.(concurrency.OperationCallback); ok {
				opCB = c
			}
		}
		jobCtx = ContextWithOperationCallback(jobCtx, opCB)

		// Inject the object data that triggered this lifecycle transition
		if objectData != nil {
			jobCtx = context.WithValue(jobCtx, evtDataKey{}, objectData)
		}

		work := triggeredJobWork{job: job, handler: handler, ctx: jobCtx, cancel: cancel}
		if err := s.submitTriggeredJob(work); err != nil {
			cancel()
			return err
		}
	}

	// Trigger the CAP orchestrator (SCH-cap-orchestrator) on completion of a backlog_item,
	// or on a convergence session lifecycle event (escalation or other state changes).
	// This replaces polling with event-driven callbacks.
	if (kind == "backlog_item" && objects.GetGlobalStatusChecker().IsTerminal(kind, toState)) || kind == "convergence_session" {
		goroutinelabels.NewGoroutine("trigger_cap_orchestrator_on_lifecycle", "triggering cap orchestrator from lifecycle event").
			StartSimple(func() {
				sCtx := pkgctx.NewSystemContext()
				if err := s.TriggerJob(sCtx, CapOrchestratorJobID); err != nil {
					SchedulerLifecycleLog(s.logger).Warn("failed to trigger cap orchestrator on lifecycle transition").
						String("kind", kind).
						String("toState", toState).
						WithError(err).
						Log()
				} else {
					SchedulerLifecycleLog(s.logger).Info("triggered cap orchestrator on lifecycle transition").
						String("kind", kind).
						String("toState", toState).
						Log()
				}
			})
	}

	return nil
}

// handleLifecycleMatrixTriggers scans the matrix registry for matching lifecycle triggers and updates CSV rows.
func (s *Scheduler) handleLifecycleMatrixTriggers(ctx context.Context, kind, fromState, toState string, objectData map[string]any) {
	projectRoot := s.projectRoot
	if projectRoot == "" {
		projectRoot = "."
	}

	reg, err := quality.LoadMatrixRegistry(projectRoot, "")
	if err != nil {
		// Registry missing or unreadable - ignore (best effort)
		return
	}

	objectID, _ := objectData[objects.FieldKeyID].(string)
	if objectID == "" {
		return
	}

	for matrixName, entry := range reg.Matrices {
		for _, trigger := range entry.LifecycleTriggers {
			// Check if trigger matches the transition
			if trigger.Kind != kind {
				continue
			}
			if trigger.ToState != toState {
				continue
			}
			if trigger.FromState != "" && trigger.FromState != "*" && trigger.FromState != fromState {
				continue
			}

			// Resolve CSV and profile paths
			_, csvPath, profilePath, err := reg.Resolve(projectRoot, matrixName)
			if err != nil || csvPath == "" || profilePath == "" {
				continue
			}

			// Check if files exist
			if _, err := fileutil.Stat(csvPath); err != nil {
				continue
			}
			if _, err := fileutil.Stat(profilePath); err != nil {
				continue
			}

			// Evaluate set pairs with placeholders from objectData
			var evaluatedPairs []string
			for _, pair := range trigger.SetPairs {
				parts := strings.SplitN(pair, "=", 2)
				if len(parts) != 2 {
					continue
				}
				key := parts[0]
				valPattern := parts[1]

				val := valPattern
				for k, v := range objectData {
					placeholder := "{" + k + "}"
					if strings.Contains(val, placeholder) {
						vStr := fmt.Sprintf("%v", v)
						val = strings.ReplaceAll(val, placeholder, vStr)
					}
				}
				evaluatedPairs = append(evaluatedPairs, key+"="+val)
			}

			// Perform update
			_, updateErr := quality.UpdateMatrixCSVRow(csvPath, profilePath, matrixName, trigger.MatchColumn, objectID, evaluatedPairs, nil, false, nil)
			if updateErr != nil {
				SchedulerLifecycleLog(s.logger).Error("failed to update matrix on lifecycle transition", updateErr).
					String("matrix", matrixName).
					String("object_id", objectID).
					Log()
			} else {
				SchedulerLifecycleLog(s.logger).Info("automatically updated matrix on lifecycle transition").
					String("matrix", matrixName).
					String("object_id", objectID).
					Log()
			}
		}
	}
}

// EvtDataKeyForTesting exposes the key for testing purposes
func EvtDataKeyForTesting() any {
	return evtDataKey{}
}

// matchesLifecycleFilter checks if a lifecycle transition matches a job's lifecycle filter
// Format: "kind:from_state->to_state" or "kind:*->to_state" or "kind:from_state->*" or "kind:*->*"
func (s *Scheduler) matchesLifecycleFilter(filter, kind, fromState, toState string) bool {
	if filter == emptyValue || filter == "*" {
		return true // Match all lifecycle transitions
	}

	// Parse filter format: "kind:from_state->to_state"
	// Examples:
	//   "backlog_item:exploring->validated" - specific transition
	//   "backlog_item:*->validated" - any state to validated
	//   "backlog_item:exploring->*" - exploring to any state
	//   "backlog_item:*->*" - any transition for backlog_item
	//   "*:exploring->validated" - exploring->validated for any kind
	//   "*:*->*" - any transition for any kind

	parts := strings.Split(filter, ":")
	if len(parts) != 2 {
		return false // Invalid format
	}

	filterKind := parts[0]
	transitionPart := parts[1]

	// Check kind match (wildcard matches all)
	if filterKind != "*" && filterKind != kind {
		return false
	}

	// Parse transition: "from_state->to_state"
	transitionParts := strings.Split(transitionPart, "->")
	if len(transitionParts) != 2 {
		return false // Invalid format
	}

	filterFromState := transitionParts[0]
	filterToState := transitionParts[1]

	// Check from state match (wildcard matches all)
	if filterFromState != "*" && filterFromState != fromState {
		return false
	}

	// Check to state match (wildcard matches all)
	if filterToState != "*" && filterToState != toState {
		return false
	}

	return true
}

// isConcurrentAllowed determines if a job type allows concurrent execution (distinct job IDs).
// JobTypeCachePrewarm: parallel per-kind warm-up.
// JobTypeRunWrapper + CategoryTesting: many SCH-run-* leftover jobs may enqueue at once; the
// triggered worker pool (many goroutines) would otherwise dequeue jobs while one bundle is still
// running, hit ConflictManager's "same job type" rule, and exit without retry—dropping most bundles.
// Same job ID is still serialized in ConflictManager. Hot packages (e.g. pkg/storage) stay serial
// via PackageConcurrencyLimiter.
// JobTypeAuditEventAggregation: must stay false (duplicate stream metrics if two scheduler-tracked runs).
// Execute() holds AuditAggregationSingletonLockID; false here is defensive.
func (s *Scheduler) isConcurrentAllowed(jobType, category string) bool {
	switch jobType {
	case JobTypeCachePrewarm:
		return true
	case JobTypeRunWrapper:
		return category == CategoryTesting
	default:
		return false
	}
}

// autoCompleteMilestonesForBacklogItem checks all milestones linked to a completed backlog item
// and automatically completes the milestone(s) if all their child backlog items are complete.
func (s *Scheduler) autoCompleteMilestonesForBacklogItem(ctx context.Context, objectData map[string]any) {
	if s == nil || s.storage == nil {
		return
	}

	var milestoneRefs []string
	if refsRaw, ok := objectData[objects.FieldKeyMilestoneRefs]; ok {
		if slice, ok := refsRaw.([]any); ok {
			for _, item := range slice {
				if sItem, ok := item.(string); ok && sItem != "" {
					milestoneRefs = append(milestoneRefs, sItem)
				}
			}
		} else if slice, ok := refsRaw.([]string); ok {
			milestoneRefs = append(milestoneRefs, slice...)
		}
	}

	if len(milestoneRefs) == 0 {
		return
	}

	storageCtx := pkgctx.NewStorageContext()

	for _, milestoneID := range milestoneRefs {
		// Find all backlog items for this milestone
		blResult, err := s.storage.List(ctx, s.secCtx, storageCtx, storagepkg.ListFilter{
			Kind: objects.KindBacklogItem,
			Filters: map[string]any{
				objects.FieldKeyMilestoneRefs: map[string]any{"$has": milestoneID},
			},
		})
		if err != nil {
			SchedulerLifecycleLog(s.logger).Error("failed to list backlog items for milestone", err).
				String("milestoneID", milestoneID).
				Log()
			continue
		}

		// Filter incomplete backlog items using spec-driven StatusChecker
		var incompleteCount int
		sc := objects.GetGlobalStatusChecker()
		for _, item := range blResult.Objects {
			itemStatus, _ := item[objects.FieldKeyStatus].(string)
			if !sc.IsTerminal(objects.KindBacklogItem, itemStatus) && !sc.IsArchive(objects.KindBacklogItem, itemStatus) && !sc.IsSystem(objects.KindBacklogItem, itemStatus) {
				incompleteCount++
			}
		}

		// If no active incomplete backlog items remain, transition milestone to complete
		if incompleteCount == 0 {
			// Get current status of milestone to avoid redundant updates
			milestone, err := s.storage.Read(ctx, s.secCtx, milestoneID)
			if err != nil {
				SchedulerLifecycleLog(s.logger).Warn("failed to read milestone for auto-transition").
					String("milestoneID", milestoneID).
					WithError(err).
					Log()
				continue
			}

			if currentStatus, _ := milestone[objects.FieldKeyStatus].(string); currentStatus != objects.ObjectStatusComplete {
				err = s.storage.Update(ctx, s.secCtx, milestoneID, map[string]any{
					objects.FieldKeyStatus: objects.ObjectStatusComplete,
				})
				if err != nil {
					SchedulerLifecycleLog(s.logger).Error("failed to auto-complete milestone", err).
						String("milestoneID", milestoneID).
						Log()
				} else {
					SchedulerLifecycleLog(s.logger).Info("milestone auto-completed because all child backlog items are complete").
						String("milestoneID", milestoneID).
						Log()
				}
			}
		}
	}
}
