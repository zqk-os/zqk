package scheduler

import (
	"context"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// TriggerImmediate dispatches a job for immediate execution, bypassing background load cycles.
// It persists the job object to storage using the Data Cell stewardship pattern before execution.
func (s *Scheduler) TriggerImmediate(ctx context.Context, job *ScheduledJob) error {
	// Persist job object using standard storage provider (Data Cell compliant)
	if s.storage == nil {
		return errfmt.Errorf("scheduler: storage provider not available")
	}

	// Ensure job object is recorded in CAS via storage interface
	// Using s.storage which is our ObjectStorageProvider
	err := s.storage.Create(ctx, s.secCtx, job.ToMap())
	if err != nil {
		return errfmt.Errorf("scheduler: failed to persist job: %w", err)
	}

	// Submit directly to the scheduler execution pool (internal helper method)
	// Triggered jobs are dispatched via submitTriggeredJob
	// Note: We need to pass the handler; normally loadAndScheduleJobs creates this.
	// Since immediate jobs are pre-defined, we assume the handler factory can create it.
	handler := s.handlerFactory.CreateHandler(job)
	if handler == nil {
		return errfmt.Errorf("scheduler: failed to create handler for job %s", job.ID)
	}

	work := triggeredJobWork{
		job:     job,
		handler: handler,
		ctx:     ctx,
	}

	return s.submitTriggeredJob(work)
}
