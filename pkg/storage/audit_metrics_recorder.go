package storage

import (
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/observability"
)

// getAuditMetricsRecorder gets a metrics recorder for audit operations
// This is in a separate file to help manage import cycles
func (c *AuditMetricsCollector) getAuditMetricsRecorder() observability.Recorder {
	if c.recorder == nil {
		c.recorder = observability.GetNoOpRecorder()
	}
	if recorder, ok := c.recorder.(observability.Recorder); ok {
		return recorder
	}
	return observability.GetNoOpRecorder()
}

// buildAuditEventValidationMetric builds a metric for audit event validation using the builder pattern
func buildAuditEventValidationMetric(success bool) observability.Builder {
	builder := observability.NewBuilder(MetricNameAuditEventValidation).
		WithTags(TagAudit, TagEvent, TagValidation)

	if !success {
		builder = builder.WithError(errfmt.Errorf(ConstAuditAuditEventValidationFailed))
	}
	return builder
}
