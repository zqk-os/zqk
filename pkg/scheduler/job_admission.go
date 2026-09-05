package scheduler

import (
	"context"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

const (
	admissionReasonTimeout    = "admission_timeout"
	admissionReasonNotInCache = "not_in_cache_after_retries"
	admissionErrorPrefix      = "scheduler admission failed: "
	lockNameAdmissionScanJobs = "scheduler.admission_scan_jobs"
)

// oneTimeImmediateShouldStartOnReload is true when a periodic ReloadJobs must call
// scheduleImmediateJob instead of only registerTriggeredJob.
//
// SCH-run-* / category=testing stay register-only so reload cannot flood the pool
// (the original mem-explosion guard). Callback-bearing CLI one-shots that were
// never queued (LastRunAt unset, not pending) must start or the error callback never runs.
func oneTimeImmediateShouldStartOnReload(job *ScheduledJob, pending bool) bool {
	if job == nil || pending || job.LastRunAt != nil {
		return false
	}
	if !job.Enabled {
		return false
	}
	if job.ExecutionMode != ExecutionModeOneTime {
		return false
	}
	if job.TriggerType != TriggerTypeImmediate {
		return false
	}
	if IsTestBundleJob(job.ID) || job.Category == CategoryTesting {
		return false
	}
	if job.CallbackOnError != emptyValue || job.CallbackOnCompletion != emptyValue {
		return true
	}
	if job.Category == CategoryManual || job.Category == CategoryUser {
		return true
	}
	return job.Priority == JobPriorityHigh || job.Priority == JobPriorityCritical
}

func jobWarrantsAdmissionHourglass(job *ScheduledJob) bool {
	if job == nil || !job.Enabled || job.LastRunAt != nil {
		return false
	}
	if job.ExecutionMode != ExecutionModeOneTime || job.TriggerType != TriggerTypeImmediate {
		return false
	}
	if IsTestBundleJob(job.ID) || job.Category == CategoryTesting {
		return false
	}
	return job.CallbackOnError != emptyValue || job.CallbackOnCompletion != emptyValue
}

func (s *Scheduler) markOneTimeImmediatePending(job *ScheduledJob) {
	if s == nil || job == nil {
		return
	}
	if job.ExecutionMode != ExecutionModeOneTime || job.TriggerType != TriggerTypeImmediate {
		return
	}
	s.immediateDispatchPending.Store(job.ID, struct{}{})
}

func (s *Scheduler) isImmediateDispatchPending(jobID string) bool {
	if s == nil || jobID == emptyValue {
		return false
	}
	_, ok := s.immediateDispatchPending.Load(jobID)
	return ok
}

func (s *Scheduler) clearImmediateDispatchPending(jobID string) {
	if s == nil || jobID == emptyValue {
		return
	}
	s.immediateDispatchPending.Delete(jobID)
}

func (s *Scheduler) markAdmissionFailed(jobID string) bool {
	if s == nil || jobID == emptyValue {
		return false
	}
	_, loaded := s.admissionFailed.LoadOrStore(jobID, struct{}{})
	return !loaded
}

func (s *Scheduler) admissionAlreadyFailed(jobID string) bool {
	if s == nil || jobID == emptyValue {
		return false
	}
	_, ok := s.admissionFailed.Load(jobID)
	return ok
}

// FailJobAdmission invokes callback_on_error and disables the job so a late start cannot double-commit.
// Used when the trigger queue exhausts cache-miss retries for a CLI one-shot.
func (s *Scheduler) FailJobAdmission(ctx context.Context, jobID, reason string) {
	if s == nil || jobID == emptyValue {
		return
	}
	job, ok := s.lookupJobInCache(jobID)
	if !ok || job == nil {
		job = s.readJobForAdmission(ctx, jobID)
	}
	if job == nil {
		SchedulerTriggerQueueLog(s.logger).Warn(LogEventSchedulerTriggerQueueAdmissionFailed).
			JobID(jobID).
			String("reason", reason).
			String("detail", "job not in cache or storage").
			Log()
		return
	}
	s.failJobAdmission(ctx, job, reason)
}

func (s *Scheduler) readJobForAdmission(ctx context.Context, jobID string) *ScheduledJob {
	if s.storage == nil || s.secCtx == nil {
		return nil
	}
	raw, err := s.storage.Read(ctx, s.secCtx, jobID)
	if err != nil || raw == nil {
		return nil
	}
	if s.jobLoader == nil {
		return nil
	}
	job, hydErr := s.jobLoader.HydrateJob(raw)
	if hydErr != nil || job == nil {
		return nil
	}
	return job
}

func (s *Scheduler) failJobAdmission(ctx context.Context, job *ScheduledJob, reason string) {
	if job == nil {
		return
	}
	if !s.markAdmissionFailed(job.ID) {
		return
	}
	s.clearImmediateDispatchPending(job.ID)
	errMsg := admissionErrorPrefix + reason
	payload := map[string]any{
		"outcome":              "error",
		"error":                errMsg,
		objects.FieldKeyReason: reason,
	}
	if job.CallbackOnError != emptyValue {
		InvokeJobCallback(ctx, s.logger, s.asyncRouter, job, callbackTypeError, payload)
	} else if job.CallbackOnCompletion != emptyValue {
		// Seat is still waiting; completion callback with error outcome clears inflight locks.
		InvokeJobCallback(ctx, s.logger, s.asyncRouter, job, callbackTypeCompletion, payload)
	}
	job.Enabled = false
	s.disableJobInStorage(ctx, job)
	SchedulerJobManagementLog(s.logger).Warn(LogEventSchedulerJobMgmtAdmissionTimeout).
		JobID(job.ID).
		String("reason", reason).
		Log()
	s.EmitTriggerQueueEvent(map[string]any{
		objects.FieldKeyEventType: "trigger_queue_admission_failed",
		triggerQueueKeyMessage:    errMsg,
		triggerQueueKeyJobID:      job.ID,
		objects.FieldKeyReason:    reason,
	})
}

func (s *Scheduler) scanAdmissionTimeouts(ctx context.Context) {
	if s == nil {
		return
	}
	timeout := getSchedulerAdmissionTimeout()
	now := time.Now().UTC()
	var due []*ScheduledJob
	_ = concurrency.RunInRLockWithLogger(
		&s.jobsMu, lockNameAdmissionScanJobs, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			for _, job := range s.jobs {
				if !jobWarrantsAdmissionHourglass(job) {
					continue
				}
				if s.admissionAlreadyFailed(job.ID) {
					continue
				}
				if s.isImmediateDispatchPending(job.ID) {
					// Already accepted onto a pool worker; CreatedAt age is not "never started".
					continue
				}
				if job.CreatedAt.IsZero() {
					continue
				}
				if now.Sub(job.CreatedAt) >= timeout {
					due = append(due, job)
				}
			}
			return nil
		},
	)
	for _, job := range due {
		s.failJobAdmission(ctx, job, admissionReasonTimeout)
	}
}
