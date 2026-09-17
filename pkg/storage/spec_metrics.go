package storage

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
)

// SpecMetricsCollector collects metrics for spec operations
type SpecMetricsCollector struct {
	storage  ObjectStorageProvider
	recorder any // observability.Recorder - stored as any to avoid import cycle
}

// NewSpecMetricsCollector creates a new spec metrics collector
func NewSpecMetricsCollector(storageProvider ObjectStorageProvider) *SpecMetricsCollector {
	return &SpecMetricsCollector{
		storage: storageProvider,
	}
}

// RecordFieldOperation records a metric for a field operation
// This is called asynchronously to avoid blocking spec operations (per POL-OBS-001)
func (c *SpecMetricsCollector) RecordFieldOperation(
	ctx context.Context,
	ontology string,
	fieldName string,
	operation string,
	breakingChange bool,
	duration time.Duration,
	success bool,
) {
	recorder := c.getSpecMetricsRecorder()

	// Build metric using new builder pattern
	builder := buildSpecFieldOperationMetric(ontology, fieldName, operation, breakingChange, duration, success)

	// Fire metric capture in background (async per POL-OBS-001)
	goroutinelabels.NewGoroutine(ConstMiscSpecMetricsFieldOperation, fmt.Sprintf(ConstMiscRecordingFieldOperationMetricSOnSS, operation, ontology, fieldName)).
		StartSimple(func() {
			// Record using new builder-pattern API
			if recorder != nil && recorder.IsEnabled() {
				var _err_83951022 = recorder.Record(ConstMiscSpecFieldOperation, builder)
				if _err_83951022 !=

					// RecordSpecChange records a metric for spec-level changes
					nil {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83951022).Log()
				}
			}
		})
}

func (c *SpecMetricsCollector) RecordSpecChange(
	ctx context.Context,
	ontology string,
	changeType string,
	fieldCount int,
	breakingChangeCount int,
	duration time.Duration,
) {
	recorder := c.getSpecMetricsRecorder()

	// Build metric using new builder pattern
	builder := buildSpecChangeMetric(ontology, changeType, fieldCount, breakingChangeCount, duration)

	// Fire metric capture in background (async per POL-OBS-001)
	goroutinelabels.NewGoroutine(ConstMiscSpecMetricsSpecChange, fmt.Sprintf(ConstMiscRecordingSpecChangeMetricSOnS, changeType, ontology)).
		StartSimple(func() {
			// Record using new builder-pattern API
			if recorder != nil && recorder.IsEnabled() {
				var _err_83951829 = recorder.Record("spec_change", builder)
				if _err_83951829 !=

					// RecordBreakingChangeDetection records a metric when breaking changes are detected
					nil {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83951829).Log()
				}
			}
		})
}

func (c *SpecMetricsCollector) RecordBreakingChangeDetection(
	ctx context.Context,
	ontology string,
	fieldName string,
	reasons []string,
) {
	recorder := c.getSpecMetricsRecorder()

	// Build metric using new builder pattern
	builder := buildSpecBreakingChangeDetectionMetric(ontology, fieldName, reasons)

	// Fire metric capture in background (async per POL-OBS-001)
	goroutinelabels.NewGoroutine(ConstMiscSpecMetricsBreakingChange, fmt.Sprintf(ConstMiscRecordingBreakingChangeDetectionMetricFo, ontology, fieldName)).
		StartSimple(func() {
			// Record using new builder-pattern API
			if recorder != nil && recorder.IsEnabled() {
				var _err_83952614 = recorder.Record(ConstMiscSpecBreakingChangeDetection, builder)
				if _err_83952614 != nil {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83952614).Log()
				}
			}
		})
}
