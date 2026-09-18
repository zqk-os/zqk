package scheduler

import (
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/observability"
)

// buildJobLoadedMetric builds a metric for job loaded events
func buildJobLoadedMetric(jobID, jobType, triggerType string) observability.Builder {
	return observability.NewBuilder("scheduler_job_loaded").
		WithField("job_id", jobID).
		WithField("job_type", jobType).
		WithField("trigger_type", triggerType).
		WithTags("scheduler", "job", "loaded")
}

// buildJobScheduledMetric builds a metric for job scheduled events
func buildJobScheduledMetric(jobID, jobType, triggerType string) observability.Builder {
	return observability.NewBuilder("scheduler_job_scheduled").
		WithField("job_id", jobID).
		WithField("job_type", jobType).
		WithField("trigger_type", triggerType).
		WithTags("scheduler", "job", "scheduled")
}

// buildJobExecutionStartedMetric builds a metric for job execution started events
func buildJobExecutionStartedMetric(jobID, jobType string) observability.Builder {
	return observability.NewBuilder("scheduler_job_execution_started").
		WithField("job_id", jobID).
		WithField("job_type", jobType).
		WithTags("scheduler", "job", "execution", "started")
}

// buildHandlerCreatedMetric builds a metric for handler creation
func buildHandlerCreatedMetric(jobType string) observability.Builder {
	return observability.NewBuilder("scheduler_handler_created").
		WithField("job_type", jobType).
		WithTags("scheduler", "handler", "created")
}

// buildHandlerCreationFailedMetric builds a metric for handler creation failures
func buildHandlerCreationFailedMetric(jobType string, err error) observability.Builder {
	builder := observability.NewBuilder("scheduler_handler_creation_failed").
		WithField("job_type", jobType).
		WithTags("scheduler", "handler", "creation", "failed")
	if err != nil {
		builder = builder.WithError(err)
	}
	return builder
}

// buildTriggerValidationMetric builds a metric for trigger validation
func buildTriggerValidationMetric(jobID, triggerType string, valid bool, reason string) observability.Builder {
	builder := observability.NewBuilder("scheduler_trigger_validation").
		WithField("job_id", jobID).
		WithField("trigger_type", triggerType).
		WithField("valid", valid).
		WithTags("scheduler", "trigger", "validation")
	if reason != emptyValue {
		builder = builder.WithField("reason", reason)
	}
	if !valid {
		builder = builder.WithError(errfmt.Errorf("trigger validation failed: %s", reason))
	}
	return builder
}

// buildScheduleAttemptMetric builds a metric for schedule attempts
func buildScheduleAttemptMetric(jobID, triggerType string, success bool) observability.Builder {
	builder := observability.NewBuilder("scheduler_schedule_attempt").
		WithField("job_id", jobID).
		WithField("trigger_type", triggerType).
		WithTags("scheduler", "schedule", "attempt")
	if !success {
		builder = builder.WithError(errfmt.Errorf("schedule attempt failed"))
	}
	return builder
}

// buildScheduleErrorMetric builds a metric for schedule errors
func buildScheduleErrorMetric(jobID, triggerType string, err error) observability.Builder {
	return observability.NewBuilder("scheduler_schedule_error").
		WithField("job_id", jobID).
		WithField("trigger_type", triggerType).
		WithError(err).
		WithTags("scheduler", "schedule", "error")
}

// buildConflictCheckMetric builds a metric for conflict checks
func buildConflictCheckMetric(jobID string, allowed bool) observability.Builder {
	builder := observability.NewBuilder("scheduler_conflict_check").
		WithField("job_id", jobID).
		WithField("allowed", allowed).
		WithTags("scheduler", "conflict", "check")
	if !allowed {
		builder = builder.WithTags("conflict_detected")
	}
	return builder
}

// buildConflictDetectedMetric builds a metric for conflict detection
func buildConflictDetectedMetric(jobID, jobType string) observability.Builder {
	return observability.NewBuilder("scheduler_conflict_detected").
		WithField("job_id", jobID).
		WithField("job_type", jobType).
		WithTags("scheduler", "conflict", "detected")
}

// buildJobLoadErrorMetric builds a metric for job load errors
func buildJobLoadErrorMetric(err error) observability.Builder {
	return observability.NewBuilder("scheduler_job_load_error").
		WithError(err).
		WithTags("scheduler", "job", "load", "error")
}

// buildTriggerQueueDequeuedMetric builds a metric for trigger-queue dequeue events
func buildTriggerQueueDequeuedMetric(count int) observability.Builder {
	return observability.NewBuilder("scheduler_trigger_queue_dequeued").
		WithField("count", count).
		WithTags("scheduler", "trigger_queue", "dequeued")
}

// buildTriggerQueueTriggerFailedMetric builds a metric for trigger-queue trigger failures
func buildTriggerQueueTriggerFailedMetric(jobID, errStr string) observability.Builder {
	b := observability.NewBuilder("scheduler_trigger_queue_trigger_failed").
		WithField("job_id", jobID).
		WithTags("scheduler", "trigger_queue", "trigger_failed")
	if errStr != emptyValue {
		b = b.WithError(errfmt.Errorf("%s", errStr))
	}
	return b
}

// buildTriggerQueueReloadRetryMetric builds a metric for trigger-queue reload retries
func buildTriggerQueueReloadRetryMetric(jobID string) observability.Builder {
	return observability.NewBuilder("scheduler_trigger_queue_reload_retry").
		WithField("job_id", jobID).
		WithTags("scheduler", "trigger_queue", "reload_retry")
}

// buildTriggerQueueReloadFailedMetric builds a metric for trigger-queue reload failures
func buildTriggerQueueReloadFailedMetric(jobID string, err error) observability.Builder {
	return observability.NewBuilder("scheduler_trigger_queue_reload_failed").
		WithField("job_id", jobID).
		WithError(err).
		WithTags("scheduler", "trigger_queue", "reload_failed")
}

// buildDispatchPressureDroppedMetric records a dispatch attempt abandoned before the handler ran.
func buildDispatchPressureDroppedMetric(source, reason string) observability.Builder {
	b := observability.NewBuilder("scheduler_dispatch_pressure_dropped").
		WithField("source", source).
		WithTags("scheduler", "dispatch_pressure", "dropped")
	if reason != emptyValue {
		b = b.WithField("reason", reason)
	}
	return b
}
