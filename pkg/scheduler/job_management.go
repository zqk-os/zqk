package scheduler

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/pipeline"
	"github.com/lanceman/zqk/pkg/scheduler/transceiver"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/when"
	"github.com/robfig/cron/v3"
)

// hydratedJob holds the result of loading one job from storage into a ScheduledJob.
type hydratedJob struct {
	job *ScheduledJob
	err error
}

const pipelineKindLoadAndScheduleJobs = "job_management_load_and_schedule"

// loadAndScheduleJobs loads all enabled scheduler_job objects and schedules them.
// When reload is true (e.g. from watchJobChanges every 30s), one_time immediate jobs
// are not re-submitted to the triggered pool; they were already submitted on initial load
// or via the trigger queue. Re-submitting them every 30s caused unbounded run_wrapper
// processes and memory explosion (see: mem-explosion fix).
// Caller must hold permission read:scheduler_job.
// LoadAndScheduleJobs loads and schedules all jobs
func (s *Scheduler) LoadAndScheduleJobs(ctx context.Context, reload bool) error {
	return s.loadAndScheduleJobs(ctx, reload)
}

func (s *Scheduler) loadAndScheduleJobs(ctx context.Context, reload bool) error {
	s.logger.Info("loadAndScheduleJobs: starting", logging.Bool("reload", reload))
	if err := s.checkPermission("read:scheduler_job"); err != nil {
		s.logger.Error("loadAndScheduleJobs: permission denied", err)
		return errfmt.Newf("permission denied to load jobs").Wrap(err)
	}
	s.logger.Info("loadAndScheduleJobs: permissions ok")
	// scheduler_job uses CAS on disk; List merges the CAS index with legacy ID-named files. When the index is
	// non-empty but stale (e.g. ensure-retention-jobs just wrote new YAMLs, or index/disk disparity), List may
	// only queue a background refresh — first LoadJobs can miss SCH-* rows. Reconcile synchronously on initial
	// load (same as trigger-queue path). Graph backends no-op here. See CAS_LIST_GET_CONSISTENCY.md.
	if !reload && s.storage != nil {
		s.logger.Info("loadAndScheduleJobs: reconciling CAS index")
		_ = s.ReconcileSchedulerJobCASIndex(ctx)
	}
	// Build path alias cache before LoadJobs so stream resolution works.
	if !reload && s.projectRoot != emptyValue {
		s.logger.Info("loadAndScheduleJobs: building path alias cache")
		storagepkg.BuildPathAliasCacheForProject(s.projectRoot)
	}
	// Required maintenance jobs (SCH-101, SCH-007, SCH-val, etc.) are ensured once at daemon start
	// via zqk system ensure-retention-jobs (config-driven from scheduler_maintenance_config.yaml).
	if s.onlyManualJobs && !reload {
		SchedulerJobManagementLog(s.logger).Info(LogEventSchedulerJobMgmtJobsPausedOnlyManual).Log()
	}
	// On reload, invalidate list cache and stream registry cache so we see jobs created by other processes
	// (e.g. CLI ensure-retention-jobs, or SCH-AUTOFIX-* from system check --auto-fix-scheduler).
	// Without this, the daemon would keep serving a stale list and never run newly created jobs.
	if reload {
		s.logger.Info("loadAndScheduleJobs: invalidating caches")
		storagepkg.InvalidateListCacheForKind(objects.KindSchedulerJob)
		if s.projectRoot != emptyValue {
			storagepkg.InvalidateStreamRegistryCacheForKind(s.projectRoot, objects.KindSchedulerJob)
		}
	}
	s.logger.Info("loadAndScheduleJobs: loading jobs from storage")
	rawJobs, err := s.jobLoader.LoadJobs(ctx)
	if err != nil {
		s.logger.Error("loadAndScheduleJobs: jobLoader.LoadJobs failed", err)
		return err
	}
	s.logger.Info("loadAndScheduleJobs: loaded jobs", logging.Int("count", len(rawJobs)))
	// Sort by priority (critical first) so immediate/triggered jobs are submitted in that order and run first.
	sort.Slice(rawJobs, func(i, j int) bool {
		return PriorityRank(rawJobPriority(rawJobs[i])) > PriorityRank(rawJobPriority(rawJobs[j]))
	})

	type loadAndScheduleJobsPayload struct {
		rawJobs  []map[string]any
		hydrated []hydratedJob
		reload   bool
	}

	logger := s.logger
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}

	pl := pipeline.NewBuilder(pipelineKindLoadAndScheduleJobs, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage(pipeline.StageNormalize, func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*loadAndScheduleJobsPayload](payload)
			if !ok {
				return nil, errfmt.Errorf("NORMALIZE expected loadAndScheduleJobsPayload, got %T", payload)
			}
			in.hydrated = s.hydrateJobs(pctx.Ctx, in.rawJobs)
			return in, nil
		}).
		AddStage(pipeline.StageCommit, func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*loadAndScheduleJobsPayload](payload)
			if !ok {
				return nil, errfmt.Errorf("COMMIT expected loadAndScheduleJobsPayload, got %T", payload)
			}
			s.syncPackageConcurrencyLimits(in.rawJobs)
			lockLog := logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem))
			// Schedule timer and non-immediate jobs first, then start cron + bootstrap cache_prewarm / SCH-101,
			// then submit immediate jobs. Otherwise scheduleImmediateJob can block on a full triggered pool queue
			// for hundreds of bundle jobs before startup bootstrap runs (multi-minute daemon "hang").
			schedulePass := func(immediateOnly bool) error {
				for i, result := range in.rawJobs {
					h := in.hydrated[i]
					isImm := h.job != nil && h.job.TriggerType == TriggerTypeImmediate
					if immediateOnly {
						if !isImm {
							continue
						}
					} else {
						if isImm {
							continue
						}
					}
					job, scheduleErr := s.tryScheduleJob(result, h, in.reload)
					if scheduleErr != nil {
						SchedulerJobManagementLog(s.logger).Warn(LogEventSchedulerJobMgmtFailedToScheduleJob).
							WithFields(jobLogFieldsWithErr(job, scheduleErr)...).
							Log()
						continue
					}
					if job == nil {
						continue
					}
					s.jobs[job.ID] = job
					SchedulerJobManagementLog(s.logger).Info(LogEventSchedulerJobMgmtScheduledJob).
						WithFields(jobLogFieldsWithSchedule(job)...).
						Log()
				}
				return nil
			}
			// Pass 1: timers, manual, workflow, event, lifecycle, jobs_paused register-only, etc.
			_ = concurrency.RunInLockWithLogger(
				&s.jobsMu,
				LockNameSchedulerLoadAndScheduleJobs,
				lockLog,
				func() error {
					s.unscheduleAllJobs()
					return schedulePass(false)
				},
			)
			if s.TestHookLoadSchedulingPhase != nil {
				s.TestHookLoadSchedulingPhase("after_timers_before_cron_and_bootstrap")
			}
			s.startCronOnce()
			if !in.reload {
				s.bootstrapCriticalTimerJobsOnDaemonStart(pctx.Ctx)
			}
			if s.TestHookLoadSchedulingPhase != nil {
				s.TestHookLoadSchedulingPhase("after_bootstrap_before_immediate")
			}
			// Pass 2: immediate jobs (may block on triggered pool queue until workers drain).
			batchSize := getSchedulerImmediateLoadBatchSize()
			pauseBetween := getSchedulerImmediateLoadBatchPause()
			var immIndices []int
			for i, h := range in.hydrated {
				if h.job != nil && h.job.TriggerType == TriggerTypeImmediate {
					immIndices = append(immIndices, i)
				}
			}
			if batchSize <= 0 || len(immIndices) == 0 {
				_ = concurrency.RunInLockWithLogger(
					&s.jobsMu,
					LockNameSchedulerLoadAndScheduleJobs,
					lockLog,
					func() error {
						return schedulePass(true)
					},
				)
			} else {
				if s.logger != nil {
					SchedulerJobManagementLog(s.logger).Info(LogEventSchedulerJobMgmtImmediateLoadChunking).
						Int("batch_size", batchSize).
						String("batch_pause", pauseBetween.String()).
						Int("immediate_jobs", len(immIndices)).
						Log()
				}
				chunkNum := 0
				for start := 0; start < len(immIndices); start += batchSize {
					end := start + batchSize
					if end > len(immIndices) {
						end = len(immIndices)
					}
					chunk := immIndices[start:end]
					_ = concurrency.RunInLockWithLogger(
						&s.jobsMu,
						LockNameSchedulerLoadAndScheduleJobs,
						lockLog,
						func() error {
							for _, idx := range chunk {
								result := in.rawJobs[idx]
								h := in.hydrated[idx]
								job, scheduleErr := s.tryScheduleJob(result, h, in.reload)
								if scheduleErr != nil {
									SchedulerJobManagementLog(s.logger).Warn(LogEventSchedulerJobMgmtFailedToScheduleJob).
										WithFields(jobLogFieldsWithErr(job, scheduleErr)...).
										Log()
									continue
								}
								if job == nil {
									continue
								}
								s.jobs[job.ID] = job
								SchedulerJobManagementLog(s.logger).Info(LogEventSchedulerJobMgmtScheduledJob).
									WithFields(jobLogFieldsWithSchedule(job)...).
									Log()
							}
							return nil
						},
					)
					if s.TestHookAfterImmediateLoadChunk != nil {
						s.TestHookAfterImmediateLoadChunk(chunkNum)
					}
					chunkNum++
					if end < len(immIndices) && pauseBetween > 0 {
						select {
						case <-pctx.Ctx.Done():
							return nil, pctx.Ctx.Err()
						case <-time.After(pauseBetween):
						}
					}
				}
			}
			return in, nil
		}).
		AddStage(pipeline.StageFinalize, func(pctx *pipeline.Context, payload any) (any, error) {
			return payload, nil
		}).
		Build()

	_, runErr := pl.Run(&pipeline.Context{Ctx: ctx, Outcome: make(map[string]any)}, &loadAndScheduleJobsPayload{
		rawJobs: rawJobs,
		reload:  reload,
	})
	if runErr != nil {
		return runErr
	}
	if !reload {
		var schedN int
		var has101, has007, hasEvag bool
		_ = concurrency.RunInLockWithLogger(
			&s.jobsMu,
			LockNameSchedulerLoadAndScheduleJobs,
			logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				schedN = len(s.jobs)
				_, has007 = s.jobs[DefaultCachePrewarmJobID]
				_, hasEvag = s.jobs[SchedulerEventsAggregationJobID]
				return nil
			},
		)
		if logger != nil {
			SchedulerJobManagementLog(logger).Info(LogEventSchedulerJobMgmtInitialLoadSummary).
				Int("scheduled_jobs", schedN).
				Int("objects_from_storage", len(rawJobs)).
				Bool("scheduled_SCH_101", has101).
				Bool("scheduled_SCH_007", has007).
				Bool("scheduled_SCH_evag", hasEvag).
				Log()
		}
	}
	return nil
}

// ReloadJobs reloads scheduler_job objects from storage (reload=true: one_time immediate jobs are not re-submitted).
// Used when processing trigger requests so newly created jobs (e.g. SCH-AUTOFIX-*) are visible without waiting for watchJobChanges.
// Invalidates the daemon's list cache for scheduler_job so List() hits storage; otherwise the daemon would serve a stale
// cache from before the creating process (e.g. SCH-autofix-run) wrote the new jobs (cross-process cache is not shared).
func (s *Scheduler) ReloadJobs(ctx context.Context) error {
	storagepkg.InvalidateListCacheForKind(objects.KindSchedulerJob)
	return s.loadAndScheduleJobs(ctx, true)
}

// casIndexReconciler is implemented by file-backed storage; graph backends may omit it.
type casIndexReconciler interface {
	EnsureCASIndexPopulatedFromScan(ctx context.Context, kind string) error
}

// ReconcileSchedulerJobCASIndex runs a synchronous full scan of scheduler_job CAS files
// (when file storage supports it) so List/LoadJobs sees the same object IDs as on disk.
// Call before ReloadJobs when processing trigger batches that include jobs created by another
// process (e.g. zqk scheduler scan-tests --all SCH-run-*, scripts/test-runner.sh SCH-run-test-*
// or legacy SCH-<unix>); otherwise cross-process CAS index lag can leave new jobs out of the
// cache and the trigger queue will drop their intents.
// See docs/architecture/README.md.
func (s *Scheduler) ReconcileSchedulerJobCASIndex(ctx context.Context) error {
	if s == nil || s.storage == nil {
		return nil
	}
	r, ok := s.storage.(casIndexReconciler)
	if !ok {
		return nil
	}
	storagepkg.InvalidateListCacheForKind(objects.KindSchedulerJob)
	return r.EnsureCASIndexPopulatedFromScan(ctx, objects.KindSchedulerJob)
}

// criticalJobTypesForSubmission are job types that may bypass the goroutine ceiling so they
// still get submitted when the process is overloaded; they then run (or timeout on their own
// max_runtime_seconds) instead of never being submitted. See SCHEDULER_OVERLOAD_AND_TIMEOUT.md.
// cache_prewarm is critical so Tier 3 caches (object-id, validation, reverse-reference) are built
// even under load; without this, overload could prevent cache_prewarm from ever running.
// data_cell_envelope_tick (SCH-dce-tick) uses category data_cell_envelope, not maintenance, so it does not
// match priorityDispatchJob's timer+CategoryMaintenance rule; list explicitly so cron fires are not dropped
// under goroutine ceiling (see dispatch_pressure.jsonl: dispatch_resource_wait_deadline_exceeded).
var criticalJobTypesForSubmission = map[string]bool{
	"audit_event_aggregation":          true,
	"cache_prewarm":                    true,
	objects.FieldKeyRetentionTolerance: true,
	JobTypeDataCellEnvelopeTick:        true,
	JobTypeCapOrchestrator:             true,
}

// priorityDispatchJob is true when the job should use the priority triggered pool and may bypass
// WaitUnderGoroutineCeiling at submission (same as criticalJobTypesForSubmission).
// Timer + CategoryMaintenance covers scheduled housekeeping (convergence_session_tick, run_wrapper
// maintenance, etc.) so testing load on the main pool and goroutine storms do not starve cron fires.
// Job types in criticalJobTypesForSubmission also bypass the ceiling (e.g. data_cell_envelope_tick).
// Explicit scheduler_job.priority high/critical also uses the priority pool so process-evidence test
// bundles (criteria_refs / test_case_refs) and other urgent run_wrapper work is not starved behind
// bulk SCH-run-bundle-* immediate jobs on the default pool.
func priorityDispatchJob(job *ScheduledJob) bool {
	if job == nil {
		return false
	}
	if criticalJobTypesForSubmission[job.JobType] {
		return true
	}
	if job.Priority == JobPriorityHigh || job.Priority == JobPriorityCritical {
		return true
	}
	return job.TriggerType == "timer" && job.Category == CategoryMaintenance
}

// bootstrapCriticalTimerJobsOnDaemonStart submits cache_prewarm (SCH-007) and the maintenance WAL
// trigger (SCH-101) once immediately after initial load and cron registration. Without this,
// operators can wait up to one schedule interval (e.g. */10) or longer when the file trigger queue
// is backed up behind bulk SCH-run-* batches and ReloadJobs. Matches SCH-007 YAML ("daemon start").
func (s *Scheduler) bootstrapCriticalTimerJobsOnDaemonStart(ctx context.Context) {
	if s == nil || s.onlyManualJobs || s.triggeredPool == nil {
		return
	}
	if ctx != nil && ctx.Err() != nil {
		return
	}
	jobIDs := []string{
		DefaultCachePrewarmJobID,
		SyncExternalAgentsJobID,
		CapOrchestratorJobID,
	}
	for _, jobID := range jobIDs {
		job, ok := s.lookupJobInCache(jobID)
		if !ok || job == nil || !job.Enabled || job.TriggerType != TriggerTypeTimer {
			continue
		}
		handler := s.createJobHandler(job)
		jobCtx, cancel := dispatchContextForScheduledJob(job)
		if !priorityDispatchJob(job) {
			if err := concurrency.WaitUnderGoroutineCeiling(jobCtx, concurrency.DefaultGoroutineCeiling, 200*time.Millisecond); err != nil {
				cancel()
				SchedulerJobManagementLog(s.logger).Warn(LogEventSchedulerJobMgmtStartupBootstrapSubmitFailed).
					JobID(jobID).
					String("reason", "goroutine_ceiling_wait").
					WithError(err).
					Log()
				continue
			}
		}
		work := triggeredJobWork{job: job, handler: handler, ctx: jobCtx, cancel: cancel}
		if err := s.submitTriggeredJob(work); err != nil {
			cancel()
			SchedulerJobManagementLog(s.logger).Warn(LogEventSchedulerJobMgmtStartupBootstrapSubmitFailed).
				JobID(jobID).
				String("reason", "submit_triggered_job").
				WithError(err).
				Log()
			continue
		}
		SchedulerJobManagementLog(s.logger).Info(LogEventSchedulerJobMgmtStartupBootstrapSubmitted).
			JobID(jobID).
			Log()
	}
}

// maxConcurrentHydrations caps how many job hydration worker goroutines exist.
// Prevents thread exhaustion when loading hundreds/thousands of jobs (e.g. many SCH-AUTOFIX).
const maxConcurrentHydrations = 16

type hydrationWorkItem struct {
	idx int
	raw map[string]any
}

// hydrateJobs hydrates raw job objects from storage using a bounded worker pool (not one goroutine per job).
// Returns a slice of hydratedJob aligned with rawJobs; each entry may have job or err set.
func (s *Scheduler) hydrateJobs(ctx context.Context, rawJobs []map[string]any) []hydratedJob {
	hydrated := make([]hydratedJob, len(rawJobs))
	if len(rawJobs) == 0 {
		return hydrated
	}
	hydrationCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	workCh := make(chan hydrationWorkItem, len(rawJobs)+maxConcurrentHydrations)
	for i, raw := range rawJobs {
		workCh <- hydrationWorkItem{idx: i, raw: raw}
	}
	close(workCh)

	numWorkers := maxConcurrentHydrations
	if len(rawJobs) < numWorkers {
		numWorkers = len(rawJobs)
	}
	var wg sync.WaitGroup
	wg.Add(numWorkers)
	bud := goroutinelabels.DefaultBudget()
	for w := 0; w < numWorkers; w++ {
		hydrateBuilder := goroutinelabels.NewGoroutine("scheduler_hydrate_job", "hydration worker")
		if bud != nil {
			hydrateBuilder = hydrateBuilder.WithBudget(bud)
		}
		hydrateBuilder.StartWithContext(hydrationCtx, func(ctx context.Context) error {
			defer wg.Done()
			for item := range workCh {
				job, err := s.jobLoader.HydrateJob(item.raw)
				hydrated[item.idx] = hydratedJob{job: job, err: err}
			}
			return nil
		})
	}
	wg.Wait()
	return hydrated
}

// unscheduleAllJobs removes all cron entries and clears the job map.
// Must be called with s.jobsMu held. Prevents duplicate cron entries on reload
// (TriggerJob / TriggerJobByLifecycle / TriggerJobByEvent use s.jobs, so re-adding
// jobs in the same lock re-registers them).
func (s *Scheduler) unscheduleAllJobs() {
	if s.cron != nil {
		for _, job := range s.jobs {
			if job.CronEntryID != 0 {
				s.cron.Remove(job.CronEntryID)
			}
		}
	}
	s.jobs = make(map[string]*ScheduledJob)
}

// tryScheduleJob schedules one job by trigger type and records metrics.
// When reload is true, one_time immediate jobs are not submitted (only registered in s.jobs);
// they were already submitted on initial load to avoid duplicate run_wrapper processes.
// Must be called with s.jobsMu held. Returns (nil, nil) to skip (hydrate failed, disabled, or nil job);
// (job, nil) on success; (job, err) when scheduling failed so caller can log.
func (s *Scheduler) tryScheduleJob(result map[string]any, h hydratedJob, reload bool) (*ScheduledJob, error) {
	if h.err != nil {
		jobID, _ := result[objects.FieldKeyID].(string)
		SchedulerJobManagementLog(s.logger).Warn(LogEventSchedulerJobMgmtFailedToHydrateJob).
			WithFields(jobLogFieldsByIDAndErr(jobID, h.err)...).
			Log()
		return nil, nil
	}
	job := h.job
	if job == nil || !job.Enabled {
		return nil, nil
	}
	if job.AllowParallelExecution {
		job.ConcurrentAllowed = true
	} else {
		job.ConcurrentAllowed = s.isConcurrentAllowed(job.JobType, job.Category)
	}

	var scheduleErr error
	// When onlyManualJobs (jobs_paused): do not run timer or immediate at load; only register so manual trigger works.
	// Exception: jobs_paused_schedule_exempt_job_ids in scheduler_maintenance_config.yaml (or builtin fallback).
	// Otherwise jobs_paused strips cron from those timers and maintenance logs never advance.
	pauseBlocksTimerOrImmediate := s.onlyManualJobs &&
		(job.TriggerType == TriggerTypeTimer || job.TriggerType == TriggerTypeImmediate) &&
		!s.isJobsPausedScheduleExempt(job.ID)
	if pauseBlocksTimerOrImmediate {
		scheduleErr = s.registerTriggeredJob(job)
	} else {
		switch job.TriggerType {
		case TriggerTypeTimer:
			scheduleErr = s.scheduleTimerJob(job)
		case TriggerTypeImmediate:
			if reload && job.ExecutionMode == "one_time" {
				// Do not re-submit one_time immediate jobs on reload; they are already in the pool or were triggered.
				scheduleErr = s.registerTriggeredJob(job)
			} else {
				scheduleErr = s.scheduleImmediateJob(job)
			}
		case TriggerTypeManual, TriggerTypeWorkflow, TriggerTypeEvent, TriggerTypeLifecycle:
			scheduleErr = s.registerTriggeredJob(job)
		default:
			scheduleErr = errfmt.Errorf("unknown trigger_type: %s", job.TriggerType)
		}
	}

	if s.metrics != nil {
		success := scheduleErr == nil
		s.metrics.RecordScheduleAttempt(job.ID, job.TriggerType, success)
		when.When(func() bool { return scheduleErr != nil }).Then(func() {
			s.metrics.RecordScheduleError(job.ID, job.TriggerType, scheduleErr)
		}).OrElse(func() {
			s.metrics.RecordJobScheduled(job.ID, job.JobType, job.TriggerType)
		}).Run()
	}
	if scheduleErr != nil {
		return job, scheduleErr
	}
	return job, nil
}

// scheduleTimerJob schedules a timer-based job with cron
func (s *Scheduler) scheduleTimerJob(job *ScheduledJob) error {
	if job.TriggerType != "timer" {
		return errfmt.Errorf("scheduleTimerJob called for non-timer job: %s", job.TriggerType)
	}

	// Parse cron expression
	// Support both 5-field (minute hour day month weekday) and 6-field (second minute hour day month weekday)
	scheduleExpr := job.ScheduleExpr

	// If 5-field, prepend "0 " to make it 6-field (run at start of minute)
	parts := len(strings.Fields(scheduleExpr))
	if parts == 5 {
		scheduleExpr = "0 " + scheduleExpr
	}

	specParser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	schedule, err := specParser.Parse(scheduleExpr)
	if err != nil {
		return errfmt.Newf("invalid cron expression").Wrap(err)
	}

	// Create job handler
	handler := s.createJobHandler(job)

	// Schedule with cron. Submit to bounded pool (same as event/lifecycle) to cap concurrent job executions.
	// Add panic recovery to prevent scheduler crash if job panics
	entryID, err := s.cron.AddFunc(scheduleExpr, func() {
		defer func() {
			if r := recover(); r != nil {
				SchedulerJobManagementLog(s.logger).Error(LogEventSchedulerJobMgmtCronExecutionPanicked,
					errfmt.Errorf("panic: %v", r)).
					WithFields(jobLogFieldsWithCategory(job)...).
					Log()
			}
		}()
		// Dispatch context: long outer deadline for queue/ceiling; executeJob times execution from start.
		jobCtx, cancel := dispatchContextForScheduledJob(job)

		if s.triggeredPool == nil {
			cancel()
			return
		}
		if !priorityDispatchJob(job) {
			if err := concurrency.WaitUnderGoroutineCeiling(jobCtx, concurrency.DefaultGoroutineCeiling, 200*time.Millisecond); err != nil {
				cancel()
				if errors.Is(err, context.DeadlineExceeded) {
					s.recordDispatchAttemptDropped("cron", dispatchPressureReasonResourceWaitDeadlineExceeded, job)
					SchedulerJobManagementLog(s.logger).Warn(LogEventSchedulerJobMgmtCronDispatchSkippedCeiling).
						JobID(job.ID).
						String("hint", "This fire is skipped; the next schedule tick will try again. Under load, use trigger-queue + re-enqueue for must-not-drop work, or raise ZQK_SCHEDULER_DISPATCH_RESOURCE_WAIT_MAX. See .zqk/scheduler/dispatch_pressure.jsonl.").
						Log()
				}
				return
			}
		}
		work := triggeredJobWork{job: job, handler: handler, ctx: jobCtx, cancel: cancel}
		if err := s.submitTriggeredJob(work); err != nil {
			cancel()
			// Parity with scheduleImmediateJob: blocked pool submit (queue full) waits on dispatch ctx
			// until ZQK_SCHEDULER_DISPATCH_RESOURCE_WAIT_MAX; record so operators distinguish ceiling vs pool.
			// Pool handoff semantics (including one stuck worker blocking the next Submit) are isolated in
			// goroutinelabels.TestPool_Submit_DeadlineExceededWhenWorkerHeldByLongTask and
			// TestSubmitTriggeredJob_integrationPoolStallDeadlineExceeded (full executeJob preamble + injected handler).
			if errors.Is(err, context.DeadlineExceeded) {
				s.recordDispatchAttemptDropped("cron_pool_submit", dispatchPressureReasonResourceWaitDeadlineExceeded, job)
				SchedulerJobManagementLog(s.logger).Warn(LogEventSchedulerJobMgmtCronDispatchSkippedPoolTimeout).
					JobID(job.ID).
					String("hint", "Next cron tick may retry. Under sustained testing load, reduce concurrency or tune ZQK_SCHEDULER_TRIGGERED_POOL_SIZE. See SCHEDULER_OVERLOAD_AND_TIMEOUT.md.").
					Log()
			}
			return
		}
	})
	if err != nil {
		return errfmt.Newf("failed to add cron job").Wrap(err)
	}

	job.CronEntryID = entryID

	// Calculate next run time
	nextRun := schedule.Next(time.Now())
	job.NextRunAt = &nextRun

	return nil
}

// scheduleImmediateJob schedules a job to run immediately (once).
// One-time immediate jobs that have already run (LastRunAt set) are not re-submitted on reload,
// so we avoid flooding the triggered pool every 30s when watchJobChanges reloads (e.g. 300 test jobs).
func (s *Scheduler) scheduleImmediateJob(job *ScheduledJob) error {
	if job.TriggerType != "immediate" {
		return errfmt.Errorf("scheduleImmediateJob called for non-immediate job: %s", job.TriggerType)
	}

	if job.ExecutionMode == "one_time" && job.LastRunAt != nil {
		SchedulerJobManagementLog(s.logger).Debug(LogEventSchedulerJobMgmtSkippingOneTimeImmediateAlreadyRun).
			WithFields(jobLogFieldsForReexec(job)...).
			Log()
		return nil // Job still added to s.jobs; do not re-submit to pool
	}
	when.When(func() bool { return job.ExecutionMode == "one_time" }).Then(func() {
		SchedulerJobManagementLog(s.logger).Info(LogEventSchedulerJobMgmtSchedulingOneTimeImmediate).
			WithFields(jobLogFieldsForImmediate(job)...).
			Log()
	}).OrElse(func() {
		SchedulerJobManagementLog(s.logger).Info(LogEventSchedulerJobMgmtReExecutingImmediate).
			WithFields(jobLogFieldsForImmediate(job)...).
			Log()
	}).Run()

	// Create job handler
	handler := s.createJobHandler(job)

	// Run immediately in a goroutine
	// Submit to bounded pool (workers started before loadAndScheduleJobs). Worker owns cancel.
	jobCtx, cancel := dispatchContextForScheduledJob(job)
	work := triggeredJobWork{job: job, handler: handler, ctx: jobCtx, cancel: cancel}
	if s.triggeredPool == nil {
		cancel()
		return errfmt.Errorf("triggered pool not started")
	}
	if err := s.submitTriggeredJob(work); err != nil {
		cancel()
		if errors.Is(err, context.DeadlineExceeded) {
			s.recordDispatchAttemptDropped("immediate_job_submit", dispatchPressureReasonResourceWaitDeadlineExceeded, job)
		}
		return errfmt.Errorf("immediate job %s: submit to worker pool failed: %w", job.ID, err)
	}

	// Set next run to now (already running)
	now := time.Now()
	job.NextRunAt = &now

	return nil
}

// registerTriggeredJob registers a job that will be triggered on-demand (manual, workflow, event)
//
//nolint:unparam // Always returns nil error - function is designed to always succeed
func (s *Scheduler) registerTriggeredJob(job *ScheduledJob) error {
	SchedulerJobManagementLog(s.logger).Debug(LogEventSchedulerJobMgmtRegisteredTriggeredJob).
		WithFields(jobLogFieldsWithTriggerType(job)...).
		Log()
	return nil
}

// createJobHandler creates the appropriate handler for a job type
func (s *Scheduler) createJobHandler(job *ScheduledJob) JobHandler {
	return s.handlerFactory.CreateHandler(job)
}

// createHandlerWithStorage creates a handler with a specific storage provider
// Used for transactional mode to inject transactional storage wrapper
func (s *Scheduler) createHandlerWithStorage(job *ScheduledJob, storage storagepkg.ObjectStorageProvider) JobHandler {
	// Create a temporary handler factory with the transactional storage
	factory := NewHandlerFactory(
		storage, // Use transactional storage instead of s.storage
		s.specLoader,
		s.lifecycleLoader,
		s.projectRoot,
		s.logger,
		s.notificationContext,
		s.asyncRouter,
		s.metrics,
		s.objectIDCacheBuilder,
	)
	factory.BindScheduler(s)
	return factory.CreateHandler(job)
}

// GetAsyncRouter returns the async router instance (for registration with shutdown coordinator)
func (s *Scheduler) GetAsyncRouter() *transceiver.AsyncRouter {
	return s.asyncRouter
}

// jobLogFields returns logging fields for job identity (job_id, job_type).
func jobLogFields(job *ScheduledJob) []logging.Field {
	return []logging.Field{
		logging.String(KeyJobID, job.ID),
		logging.String(KeyJobType, job.JobType),
	}
}

// jobLogFieldsWithErr returns job identity plus error. Use for Warn/Error logs that include an error.
func jobLogFieldsWithErr(job *ScheduledJob, err error) []logging.Field {
	return append(jobLogFields(job), logging.Error(err))
}

// jobLogFieldsByIDAndErr returns job_id and error when job struct is not available (e.g. hydrate failure).
func jobLogFieldsByIDAndErr(jobID string, err error) []logging.Field {
	return []logging.Field{
		logging.String(KeyJobID, jobID),
		logging.Error(err),
	}
}

// jobLogFieldsByID returns just job_id for logs that have no error (e.g. trigger validation warnings).
func jobLogFieldsByID(jobID string) []logging.Field {
	return []logging.Field{logging.String(KeyJobID, jobID)}
}

// logErrField returns a single logging field for an error. Use for Warn/Error logs that only add an error.
func logErrField(err error) []logging.Field {
	return logging.ErrField(err)
}

// jobLogFieldsWithSchedule returns identity plus trigger_type and schedule.
func jobLogFieldsWithSchedule(job *ScheduledJob) []logging.Field {
	return append(jobLogFields(job),
		logging.String("trigger_type", job.TriggerType),
		logging.String("schedule", job.ScheduleExpr))
}

// jobLogFieldsForImmediate returns identity plus execution_mode (for immediate job logs).
func jobLogFieldsForImmediate(job *ScheduledJob) []logging.Field {
	return append(jobLogFields(job), logging.String("execution_mode", job.ExecutionMode))
}

// jobLogFieldsForReexec returns identity plus last_run_at and note. Call only when job.LastRunAt != nil.
func jobLogFieldsForReexec(job *ScheduledJob) []logging.Field {
	return append(jobLogFields(job),
		logging.String("last_run_at", job.LastRunAt.Format(time.RFC3339)),
		logging.String("note", "One-time jobs can be re-run until disabled or deleted"))
}

// jobLogFieldsWithTriggerType returns identity plus trigger_type (for triggered-job registration).
func jobLogFieldsWithTriggerType(job *ScheduledJob) []logging.Field {
	return append(jobLogFields(job), logging.String("trigger_type", job.TriggerType))
}

// jobLogFieldsWithCategory returns identity plus category (e.g. for panic logs).
func jobLogFieldsWithCategory(job *ScheduledJob) []logging.Field {
	return append(jobLogFields(job), logging.String(KeyCategory, job.Category))
}

// jobLogFieldsWithCategoryAndDuration returns identity, category, and duration. Use for execution completion/failure logs.
func jobLogFieldsWithCategoryAndDuration(job *ScheduledJob, duration time.Duration) []logging.Field {
	return append(jobLogFieldsWithCategory(job), logging.String(KeyDuration, duration.String()))
}

// jobLogFieldsForFailedCommand returns identity plus failure_kind, failure_reason, command, exit_code, duration, attempt, and error (for run_wrapper failure logs).
func jobLogFieldsForFailedCommand(job *ScheduledJob, cmdStr, failureKind, failureReason string, exitCode, attempt int, duration time.Duration, err error) []logging.Field {
	return append(jobLogFields(job),
		logging.String(KeyFailureKind, failureKind),
		logging.String(KeyFailureReason, failureReason),
		logging.String(KeyCommand, cmdStr),
		logging.Int(KeyExitCode, exitCode),
		logging.String(KeyDuration, duration.String()),
		logging.Int(KeyAttempt, attempt),
		logging.Error(err))
}

// jobLogFieldsForPanic returns logging fields for a run_wrapper panic (job_id, job_type, category, command, stack).
// Used when job may be nil so individual strings and stack are passed.
func jobLogFieldsForPanic(jobID, jobType, category, command string, stack []byte) []logging.Field {
	return []logging.Field{
		logging.String(KeyJobID, jobID),
		logging.String(KeyJobType, jobType),
		logging.String(KeyCategory, category),
		logging.String(KeyCommand, command),
		logging.String("stack", string(stack)),
	}
}
