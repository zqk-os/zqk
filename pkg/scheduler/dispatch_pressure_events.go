package scheduler

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/circuitbreaker"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const dispatchPressureJSONLFile = "dispatch_pressure.jsonl"

// dispatchPressureReasonResourceWaitDeadlineExceeded is recorded when WaitUnderGoroutineCeiling or
// Pool.Submit expires the dispatch wait budget (ZQK_SCHEDULER_DISPATCH_RESOURCE_WAIT_MAX).
const dispatchPressureReasonResourceWaitDeadlineExceeded = "dispatch_resource_wait_deadline_exceeded"

// dispatch_wait_stage JSON values for dispatch_pressure rows (wait-budget drops only).
const (
	dispatchWaitStageGoroutineCeiling    = "goroutine_ceiling"
	dispatchWaitStageTriggeredPoolSubmit = "triggered_pool_submit"
	dispatchWaitStageExecuteJobOuterCtx  = "execute_job_outer_context"
)

var dispatchPressureAppendMu sync.Mutex

// recordDispatchAttemptDropped records that the scheduler abandoned starting a job before the handler ran
// (dispatch resource wait budget exhausted, or blocked pool submit timed out). This is weaker than a job
// failure: a later schedule tick, trigger, or trigger-queue retry may still run the work; under sustained
// load, runs may still fail or time out for unrelated reasons.
// Optional detailErr is appended as "error" in the JSONL / coordinator stream when non-nil (e.g. Acquire error text).
func (s *Scheduler) recordDispatchAttemptDropped(source, reason string, job *ScheduledJob, detailErr ...error) {
	if job == nil {
		return
	}
	root := s.getProjectRoot()
	if root == emptyValue {
		return
	}
	rec := map[string]any{
		objects.FieldKeyEventType:   "dispatch_pressure",
		"dispatch_subtype":          "attempt_dropped",
		objects.FieldKeySource:      source,
		objects.FieldKeyReason:      reason,
		"job_id":                    job.ID,
		objects.FieldKeyJobType:     job.JobType,
		objects.FieldKeyCategory:    job.Category,
		objects.FieldKeyTriggerType: job.TriggerType,
		"guarantee":                 "Weaker: this dispatch attempt did not start the handler. A later cron tick, manual trigger, or trigger-queue pass may retry. Under load, a started run may still fail or hit max_runtime_seconds.",
		"dispatch_wait_max":         getSchedulerDispatchResourceWaitMax().String(),
		"timestamp":                 zqktime.NowRFC3339UTC(),
	}
	if len(detailErr) > 0 && detailErr[0] != nil {
		rec["error"] = detailErr[0].Error()
	}
	if reason == jobExecReasonPackageConcurrencyLimit && job.JobType == jobTypeRunWrapper {
		if pp := strings.TrimSpace(circuitbreaker.ExtractPackagePathFromRunWrapperCommand(job.Command, job.CommandArgs)); pp != emptyValue {
			rec["package_path"] = pp
			if s.packageConcurrencyLimiter != nil {
				lim, used := s.packageConcurrencyLimiter.Snapshot(pp)
				if lim > 0 {
					rec["package_concurrency_limit"] = lim
				}
				rec["package_slots_in_use"] = used
			}
		}
	}
	if reason == dispatchPressureReasonResourceWaitDeadlineExceeded || reason == jobExecReasonDispatchDeadline {
		if stage := dispatchWaitStageForDrop(source, reason); stage != emptyValue {
			rec["dispatch_wait_stage"] = stage
		}
		s.enrichDispatchPressureTriggeredPoolSnapshot(job, rec)
	}
	appendDispatchPressureJSONL(root, rec)
	s.EmitTriggerQueueEvent(rec)
	if s.metrics != nil {
		s.metrics.RecordDispatchPressureDropped(source, reason)
	}
}

func dispatchWaitStageForDrop(source, reason string) string {
	switch reason {
	case jobExecReasonDispatchDeadline:
		if source == "execute_job" {
			return dispatchWaitStageExecuteJobOuterCtx
		}
	case dispatchPressureReasonResourceWaitDeadlineExceeded:
		switch source {
		case "cron":
			return dispatchWaitStageGoroutineCeiling
		case "cron_pool_submit", "immediate_job_submit", "trigger_job_submit", "missed_job_recovery":
			return dispatchWaitStageTriggeredPoolSubmit
		}
	}
	return emptyValue
}

func (s *Scheduler) enrichDispatchPressureTriggeredPoolSnapshot(job *ScheduledJob, rec map[string]any) {
	if s == nil || job == nil || rec == nil {
		return
	}
	pool := s.triggeredPool
	route := "default"
	if priorityDispatchJob(job) && s.triggeredPriorityPool != nil {
		pool = s.triggeredPriorityPool
		route = "priority"
	}
	if pool == nil {
		return
	}
	workers, queued, qcap := pool.QueuePressureSnapshot()
	rec["triggered_pool_route"] = route
	rec["triggered_pool_name"] = pool.Name()
	rec["triggered_pool_workers"] = workers
	rec["triggered_pool_queue_depth"] = queued
	if qcap > 0 {
		rec["triggered_pool_queue_capacity"] = qcap
		rec["triggered_pool_queue_depth_pct"] = (100 * queued) / qcap
	}
}

func appendDispatchPressureJSONL(projectRoot string, rec map[string]any) {
	dispatchPressureAppendMu.Lock()
	defer dispatchPressureAppendMu.Unlock()

	dir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir)
	if err := fileutil.EnsureDir(dir); err != nil {
		return
	}
	path := filepath.Join(dir, dispatchPressureJSONLFile)
	f, err := fileutil.OpenFile(path, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			SLog(logger).Debug("Failed to close dispatch pressure log").WithError(err).Log()
		}
	}()
	line, err := json.Marshal(rec)
	if err != nil {
		return
	}
	line = append(line, '\n')
	if _, err := f.Write(line); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Debug("Failed to write dispatch pressure event").WithError(err).Log()
	}
}

// maxDispatchDropReenqueueRetries is the maximum number of times a job dropped due to concurrency limits
// may be re-enqueued before stopping to prevent tight re-enqueue loops.
const maxDispatchDropReenqueueRetries = 3

// baseDispatchDropBackoff is the starting delay before re-enqueueing a concurrency-dropped job.
const baseDispatchDropBackoff = 1 * time.Second

// maxDispatchDropBackoff is the upper bound on exponential backoff for re-enqueueing a dropped job.
const maxDispatchDropBackoff = 8 * time.Second

func dispatchDropBackoffDuration(retries int) time.Duration {
	if retries <= 1 {
		return baseDispatchDropBackoff
	}
	shift := retries - 1
	if shift > 3 {
		shift = 3
	}
	d := baseDispatchDropBackoff * time.Duration(1<<uint(shift))
	if d > maxDispatchDropBackoff {
		d = maxDispatchDropBackoff
	}
	return d
}

func isConcurrencyDropReason(priorReason string) bool {
	return priorReason == jobExecReasonPackageConcurrencyLimit ||
		priorReason == jobExecReasonGlobalTestConcurrencyLimit
}

// reenqueueTriggerAfterDispatchDrop appends to the file trigger queue so a later poll calls TriggerJob
// again with a fresh dispatch context. Used when executeJob returns before the handler ran:
//   - SCH-run-* test bundles: package concurrency wait or expired dispatch deadline (existing behavior);
//   - non-concurrent run_wrapper (e.g. maintenance): lost ConflictManager.CanRun to another run_wrapper.
func (s *Scheduler) reenqueueTriggerAfterDispatchDrop(job *ScheduledJob, priorReason string) {
	if job == nil {
		return
	}
	if !shouldReenqueueTriggerAfterDispatchDrop(job, priorReason) {
		return
	}
	root := s.getProjectRoot()
	if root == emptyValue {
		return
	}

	if isConcurrencyDropReason(priorReason) {
		retries := s.incrementDispatchDropRetryCount(job.ID)
		if retries > maxDispatchDropReenqueueRetries {
			logger := s.logger
			if logger == nil {
				logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			}
			SchedulerDispatchLog(logger).Warn(LogEventSchedulerDispatchMaxRetriesExceeded).
				JobID(job.ID).
				String("prior_reason", priorReason).
				Int("retries", retries).
				Int("max_retries", maxDispatchDropReenqueueRetries).
				Log()
			s.EmitTriggerQueueEvent(map[string]any{
				objects.FieldKeyEventType: "trigger_queue_drop_max_retries_exceeded",
				"message":                 "Job exceeded max dispatch drop retries and will not be re-enqueued",
				"job_id":                  job.ID,
				"prior_reason":            priorReason,
				"retries":                 retries,
				"max_retries":             maxDispatchDropReenqueueRetries,
				"timestamp":               zqktime.NowRFC3339UTC(),
			})
			return
		}

		delay := dispatchDropBackoffDuration(retries)
		if s.testHookDispatchDropBackoffDuration != nil {
			delay = s.testHookDispatchDropBackoffDuration(retries)
		}
		if delay > 0 {
			time.AfterFunc(delay, func() {
				s.doEnqueueTriggerAfterDrop(job, priorReason, retries)
			})
			return
		}
		s.doEnqueueTriggerAfterDrop(job, priorReason, retries)
		return
	}

	s.doEnqueueTriggerAfterDrop(job, priorReason, 0)
}

func (s *Scheduler) doEnqueueTriggerAfterDrop(job *ScheduledJob, priorReason string, retries int) {
	root := s.getProjectRoot()
	if root == emptyValue {
		return
	}
	q := NewJobTriggerQueue(root)
	if err := q.EnqueueTriggerRequest(job.ID); err != nil {
		logger := s.logger
		if logger == nil {
			logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		}
		SchedulerDispatchLog(logger).Error(LogEventSchedulerDispatchReenqueueAfterDropFailed,
			err).
			JobID(job.ID).
			String("prior_reason", priorReason).
			Log()
		return
	}
	if s.logger != nil {
		ev := LogEventSchedulerDispatchReenqueuedAfterDrop
		if strings.HasPrefix(job.ID, TestBundleJobIDPrefix) {
			ev = LogEventSchedulerDispatchTestBundleReenqueuedAfterDrop
		}
		SchedulerDispatchLog(s.logger).Info(ev).
			JobID(job.ID).
			String("prior_reason", priorReason).
			Int("retries", retries).
			Log()
	}
	s.EmitTriggerQueueEvent(map[string]any{
		objects.FieldKeyEventType: "trigger_queue_reenqueued_after_dispatch_drop",
		"message":                 "Re-enqueued job so a future trigger-queue pass retries after the run could not start",
		"job_id":                  job.ID,
		"prior_reason":            priorReason,
		"retries":                 retries,
		"timestamp":               zqktime.NowRFC3339UTC(),
	})
}

// shouldReenqueueTriggerAfterDispatchDrop mirrors the conditions under which we append to the trigger queue.
func shouldReenqueueTriggerAfterDispatchDrop(job *ScheduledJob, priorReason string) bool {
	if job == nil {
		return false
	}
	if strings.HasPrefix(job.ID, TestBundleJobIDPrefix) {
		return true
	}
	return priorReason == jobExecReasonConflict && job.JobType == jobTypeRunWrapper && !job.ConcurrentAllowed
}
