package scheduler

import (
	"context"
	"errors"
	"fmt"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// admissionDeferralAuditEvent is recorded when TriggerJob cannot admit work and
// the trigger-queue re-enqueues the request. History aggregation treats failed
// job events as durable runs so `zqk scheduler history --job-id` is non-empty
// under pool_full churn (previously only diagnostics.jsonl saw the deferral).
//
// TRACK: BLI-1794352583586579500-b4aad3b8 — replace failed-with-reason with a
// dedicated scheduler_job_deferred_* audit enum + history Deferred outcome once
// the audit_event enum lands; keep fail-closed AUD persist semantics.
const admissionDeferralAuditEvent = auditEventSchedulerFailed

// RecordAdmissionDeferral writes one durable audit event per job ID so one-shot
// (and other) triggers that bounce on ErrPoolFull / deadline / cancel leave a
// history row instead of silent re-enqueue loops.
func (s *Scheduler) RecordAdmissionDeferral(ctx context.Context, jobIDs []string, cause error) {
	if s == nil || len(jobIDs) == 0 {
		return
	}
	reason := "admission deferred; re-enqueued for next poll"
	switch {
	case errors.Is(cause, goroutinelabels.ErrPoolFull):
		reason = "admission deferred: pool_full; re-enqueued for next poll"
	case errors.Is(cause, context.DeadlineExceeded):
		reason = "admission deferred: deadline_exceeded; re-enqueued for next poll"
	case errors.Is(cause, context.Canceled):
		reason = "admission deferred: canceled; re-enqueued for next poll"
	case cause != nil:
		reason = fmt.Sprintf("admission deferred: %v; re-enqueued for next poll", cause)
	}
	deferErr := errors.New(reason)

	for _, jobID := range jobIDs {
		if jobID == emptyValue {
			continue
		}
		jobType, category := "unknown", "unknown"
		if job, ok := s.GetJob(jobID); ok && job != nil {
			if job.JobType != emptyValue {
				jobType = job.JobType
			}
			if job.Category != emptyValue {
				category = job.Category
			}
		}
		if err := s.createJobAuditEvent(ctx, admissionDeferralAuditEvent, jobID, jobType, category, false, 0, deferErr); err != nil {
			SchedulerTriggerQueueLog(s.logger).Debug(LogEventSchedulerTriggerQueueFailedReenqueuePoolFull).
				JobID(jobID).
				WithError(err).
				Log()
		}
	}
}
