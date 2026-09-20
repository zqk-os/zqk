package storage

import (
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/observability"
)

// getSpecMetricsRecorder gets a metrics recorder for spec operations
// This is in a separate file to help manage import cycles
func (c *SpecMetricsCollector) getSpecMetricsRecorder() observability.Recorder {
	if c.recorder == nil {
		c.recorder = observability.GetNoOpRecorder()
	}
	if recorder, ok := c.recorder.(observability.Recorder); ok {
		return recorder
	}
	return observability.GetNoOpRecorder()
}

// buildSpecFieldOperationMetric builds a metric for field operations using the builder pattern
func buildSpecFieldOperationMetric(ontology, fieldName, operation string, breakingChange bool, duration time.Duration, success bool) observability.Builder {
	builder := observability.NewBuilder(ConstMiscSpecFieldOperation).
		WithField("ontology", ontology).
		WithField("field_name", fieldName).
		WithField("operation", operation).
		WithField(ConstMiscBreakingChange, breakingChange).
		WithDuration(duration).
		WithTags("system", ConstMiscSpecManagement, ConstMiscFieldOperation, operation, ontology)

	if breakingChange {
		builder = builder.WithTags(ConstMiscBreakingChange)
	}
	if !success {
		builder = builder.WithError(errfmt.Errorf(ConstMiscFieldOperationFailed))
	}
	return builder
}

// buildSpecChangeMetric builds a metric for spec changes using the builder pattern
func buildSpecChangeMetric(ontology, changeType string, fieldCount, breakingChangeCount int, duration time.Duration) observability.Builder {
	builder := observability.NewBuilder("spec_change").
		WithField("ontology", ontology).
		WithField("change_type", changeType).
		WithField("field_count", fieldCount).
		WithField(ConstMiscBreakingChangeCount, breakingChangeCount).
		WithDuration(duration).
		WithTags("system", ConstMiscSpecManagement, "spec_change", changeType, ontology)

	if breakingChangeCount > 0 {
		builder = builder.WithTags(ConstMiscBreakingChanges)
	}
	return builder
}

// buildSpecBreakingChangeDetectionMetric builds a metric for breaking change detection using the builder pattern
func buildSpecBreakingChangeDetectionMetric(ontology, fieldName string, reasons []string) observability.Builder {
	return observability.NewBuilder(ConstMiscSpecBreakingChangeDetection).
		WithField("ontology", ontology).
		WithField("field_name", fieldName).
		WithField("reasons", reasons).
		WithField("reason_count", len(reasons)).
		WithTags("system", ConstMiscSpecManagement, ConstMiscBreakingChange, "detection", ontology)
}
