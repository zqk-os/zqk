package scheduler

import (
	"time"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/observability"
)

// getSchedulerMetricsRecorder gets a metrics recorder for scheduler operations
// This is in a separate file to help manage import cycles
func (m *DefaultSchedulerMetricsCollector) getSchedulerMetricsRecorder() observability.Recorder {
	if m.recorder == nil {
		m.recorder = observability.GetNoOpRecorder()
	}
	if recorder, ok := m.recorder.(observability.Recorder); ok {
		return recorder
	}
	return observability.GetNoOpRecorder()
}

// GetObservabilityRecorder returns the recorder used for scheduler metrics (never nil).
// Callers such as JobStateRegistry use this to emit infrastructure-scoped operational metrics
// (e.g. retention cleanup of scheduler state files) on the same pipeline as job lifecycle metrics.
func (m *DefaultSchedulerMetricsCollector) GetObservabilityRecorder() observability.Recorder {
	if m == nil {
		return observability.GetNoOpRecorder()
	}
	return m.getSchedulerMetricsRecorder()
}

// buildJobExecutionMetric builds a metric for job execution using the builder pattern
func buildJobExecutionMetric(operation string, jobID, jobType string, duration time.Duration, err error) observability.Builder {
	builder := observability.NewBuilder("scheduler_job_"+operation).
		WithField("job_id", jobID).
		WithField("job_type", jobType).
		WithDuration(duration).
		WithTags("scheduler", "job", operation)

	if err != nil {
		builder = builder.WithError(err)
	}
	return builder
}

// buildSchedulerLifecycleMetric builds a metric for scheduler lifecycle events using the builder pattern
func buildSchedulerLifecycleMetric(operation string, duration time.Duration) observability.Builder {
	return observability.NewBuilder("scheduler_"+operation).
		WithDuration(duration).
		WithTags("scheduler", objects.KindLifecycle, operation)
}
