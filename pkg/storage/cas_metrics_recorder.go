package storage

import (
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/observability"
)

// getCASMetricsRecorder gets a metrics recorder for CAS operations (thread-safe lazy init).
// This is in a separate file to help manage import cycles.
func (m *CASMetrics) getCASMetricsRecorder() observability.Recorder {
	var rec any
	if err := concurrency.RunInLock(&m.recorderMu, func() error {
		if m.recorder == nil {
			m.recorder = observability.GetNoOpRecorder()
		}
		rec = m.recorder
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ConstStreamFailedToAcquireLockForCasMetricsRecorderInitValN, err).Log()
	}
	if recorder, ok := rec.(observability.Recorder); ok {
		return recorder
	}
	return observability.GetNoOpRecorder()
}

// buildCASOperationMetric builds a metric for content-addressed storage operations.
func buildCASOperationMetric(operation string, duration time.Duration, err error) observability.Builder {
	builder := observability.NewBuilder(ConstStreamContentAddressed+operation).
		WithDuration(duration).
		WithTags("storage", ConstStreamContentAddressed2, operation)

	if err != nil {
		builder = builder.WithError(err)
	}
	return builder
}

// buildListingIndexOperationMetric builds a metric for listing-index operations.
func buildListingIndexOperationMetric(operation string, duration time.Duration, err error, additionalFields map[string]any) observability.Builder {
	builder := observability.NewBuilder(ConstStreamListingIndex+operation).
		WithDuration(duration).
		WithTags("storage", "listing_index", operation)

	for k, v := range additionalFields {
		builder = builder.WithField(k, v)
	}

	if err != nil {
		builder = builder.WithError(err)
	}
	return builder
}
