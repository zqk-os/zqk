package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

// stuckDrainReason is the durable failure reason written into scheduler_job_failed audits
// when a started-without-complete job is drained past the INV threshold.
const stuckDrainReason = "stuck_started_without_complete"

// activityEntryOpenStartedAge returns how long a cache entry has been started without a later
// completion or failure. ok is false when the entry is not an open started-without-complete.
func activityEntryOpenStartedAge(entry *ActivityCacheEntry, now time.Time) (age time.Duration, ok bool) {
	if entry == nil || entry.LastStarted.IsZero() {
		return 0, false
	}
	terminal := entry.LastCompleted
	if entry.LastFailed.After(terminal) {
		terminal = entry.LastFailed
	}
	if !terminal.IsZero() && !terminal.Before(entry.LastStarted) {
		return 0, false
	}
	if now.Before(entry.LastStarted) {
		return 0, false
	}
	return now.Sub(entry.LastStarted), true
}

// stuckDrainThresholdForJob returns the age after which an open started job should be drained.
// Live / occupied jobs keep their max_runtime budget; abandoned audits use the INV stale window.
func stuckDrainThresholdForJob(job *ScheduledJob, occupied bool, staleAfter time.Duration) time.Duration {
	if staleAfter <= 0 {
		staleAfter = DefaultStuckJobDrainAfter
	}
	if !occupied {
		return staleAfter
	}
	maxSec := DefaultMaxRuntimeSeconds
	if job != nil && job.MaxRuntimeSeconds > 0 {
		maxSec = job.MaxRuntimeSeconds
	}
	runtimeBudget := time.Duration(maxSec) * time.Second
	if runtimeBudget > staleAfter {
		return runtimeBudget
	}
	return staleAfter
}

// DrainStuckStartedJobs marks started-without-complete jobs past the INV / max-runtime threshold
// as failed with a durable audit trail so `scheduler activity` / `history --job-id` stop showing
// silent forever-started work. Returns the number of jobs drained this call.
//
// TRACK: GOAL-1794519849204113000-32fabd28 — scheduler never silently loses work (counsel G3).
func (s *Scheduler) DrainStuckStartedJobs(ctx context.Context, staleAfter time.Duration) int {
	if s == nil {
		return 0
	}
	if staleAfter <= 0 {
		staleAfter = DefaultStuckJobDrainAfter
	}
	if ctx == nil {
		ctx = context.Background()
	}

	cache := GetGlobalActivityCache()
	if cache == nil {
		return 0
	}
	if root := s.getProjectRoot(); root != emptyValue {
		if cache.GetMetadata() == nil {
			_ = cache.LoadCache(root) //nolint:errcheck // best-effort warm
		}
	}

	now := time.Now().UTC()
	drained := 0
	for jobID, entry := range cache.GetAllEntries() {
		if jobID == emptyValue || entry == nil {
			continue
		}
		age, open := activityEntryOpenStartedAge(entry, now)
		if !open {
			continue
		}
		if _, already := s.stuckDrained.Load(jobID); already {
			continue
		}

		job, _ := s.GetJob(jobID)
		occupied := s.isJobOccupied(job)
		threshold := stuckDrainThresholdForJob(job, occupied, staleAfter)
		if age < threshold {
			continue
		}

		if occupied {
			s.cancelStuckOccupiedJob(job)
			// Live cancel should surface via the normal executeJob failure path. If the job is a
			// zombie Running flag with no cancel, force the durable trail below.
			if s.isJobOccupied(job) {
				s.forceClearStuckRunning(job)
			} else {
				s.stuckDrained.Store(jobID, struct{}{})
				drained++
				continue
			}
		}

		if s.drainStuckJobAudit(ctx, jobID, job, age) {
			s.stuckDrained.Store(jobID, struct{}{})
			drained++
		}
	}
	return drained
}

func (s *Scheduler) cancelStuckOccupiedJob(job *ScheduledJob) {
	if job == nil {
		return
	}
	job.RunningMu.Lock()
	cancel := job.ExecutionCancel()
	job.RunningMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Scheduler) forceClearStuckRunning(job *ScheduledJob) {
	if job == nil {
		return
	}
	job.RunningMu.Lock()
	job.Running = false
	job.ClearExecutionContext()
	job.RunningMu.Unlock()
	if s.conflictMgr != nil {
		s.conflictMgr.UnregisterRunning(job)
	}
}

func (s *Scheduler) drainStuckJobAudit(ctx context.Context, jobID string, job *ScheduledJob, age time.Duration) bool {
	jobType, category := "unknown", "unknown"
	if job != nil {
		if job.JobType != emptyValue {
			jobType = job.JobType
		}
		if job.Category != emptyValue {
			category = job.Category
		}
	}
	reason := fmt.Sprintf("%s after %s", stuckDrainReason, age.Truncate(time.Second))
	drainErr := errors.New(reason)

	if s.stateRegistry != nil {
		if st, err := s.stateRegistry.GetExecutionState(jobID); err == nil && st != nil &&
			(st.State == jobExecutionStateInProgress || st.State == jobExecutionStateDeferred) {
			_ = s.stateRegistry.CompleteExecution(jobID, st.ExecutionID, "failed_"+stuckDrainReason)
		}
	}

	if err := s.createJobAuditEvent(ctx, auditEventSchedulerFailed, jobID, jobType, category, false, age, drainErr); err != nil {
		SchedulerTriggerQueueLog(s.logger).Debug(LogEventSchedulerTriggerQueueStuckJobDrained).
			JobID(jobID).
			WithError(err).
			Log()
		return false
	}

	SchedulerTriggerQueueLog(s.logger).Info(LogEventSchedulerTriggerQueueStuckJobDrained).
		JobID(jobID).
		String(objects.FieldKeyReason, stuckDrainReason).
		String("age", age.Truncate(time.Second).String()).
		Log()
	s.EmitTriggerQueueEvent(map[string]any{
		objects.FieldKeyEventType: "trigger_queue_stuck_job_drained",
		triggerQueueKeyMessage:    "Drained started-without-complete job past INV/max-runtime threshold",
		triggerQueueKeyJobID:      jobID,
		objects.FieldKeyReason:    stuckDrainReason,
		"age_seconds":             age.Seconds(),
	})
	return true
}
