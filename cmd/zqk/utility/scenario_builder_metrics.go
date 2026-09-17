package utility

import (
	"github.com/lanceman/zqk/pkg/observability"
)

// MetricRecorder is an alias for the shared observability.Recorder interface
// This maintains backward compatibility with existing code
type MetricRecorder = observability.Recorder

// MetricBuilder is an alias for the shared observability.Builder interface
// This maintains backward compatibility with existing code
type MetricBuilder = observability.Builder

// MetricData is an alias for the shared observability.Data struct
// This maintains backward compatibility with existing code
type MetricData = observability.Data

// MetricRecorderFactory is an alias for the shared observability.Factory interface
// This maintains backward compatibility with existing code
type MetricRecorderFactory = observability.Factory

// NewMetricBuilder creates a new metric builder for the given operation
// This is a convenience wrapper around observability.NewBuilder
func NewMetricBuilder(operation string) MetricBuilder {
	return observability.NewBuilder(operation).WithTags(ScenarioBuilderProfileName) // Add default tag
}

// GetNoOpMetricRecorder returns a no-op metric recorder
// This is a convenience wrapper around observability.GetNoOpRecorder
func GetNoOpMetricRecorder() MetricRecorder {
	return observability.GetNoOpRecorder()
}

// NewMetricRecorderFactory creates a new metric recorder factory
// This is a convenience wrapper around observability.NewFactory
func NewMetricRecorderFactory(enabled bool) MetricRecorderFactory {
	return observability.NewFactory(enabled)
}
