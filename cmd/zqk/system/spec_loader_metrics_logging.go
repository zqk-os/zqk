package system

import (
	"github.com/zqk-os/zqk/pkg/logging"

	"github.com/zqk-os/zqk/pkg/objects"
)

// logSpecLoaderMetrics logs spec loader metrics for observability
// Helps diagnose lock contention, timeouts, and performance issues
// Now includes analysis and recommendations to inform CLI improvements
func logSpecLoaderMetrics(logger logging.Logger) {
	if logger == nil {
		return
	}

	specLoader := objects.GetGlobalSpecLoader()
	if specLoader == nil {
		return
	}

	metrics := specLoader.GetMetrics()
	if metrics.TotalWaits == 0 && metrics.TotalHolds == 0 {
		// No lock operations occurred - skip logging
		return
	}

	// Log metrics summary (helps diagnose hangs and contention)
	logging.Fluent(logger).Info("Spec loader metrics").
		WithFields(
			logging.Field{Key: "total_waits", Value: metrics.TotalWaits},
			logging.Field{Key: "total_holds", Value: metrics.TotalHolds},
			logging.Field{Key: "avg_wait_time", Value: metrics.AverageWaitTime.String()},
			logging.Field{Key: "max_wait_time", Value: metrics.MaxWaitTime.String()},
			logging.Field{Key: "avg_hold_time", Value: metrics.AverageHoldTime.String()},
			logging.Field{Key: "max_hold_time", Value: metrics.MaxHoldTime.String()},
			logging.Field{Key: "contention_events", Value: metrics.ContentionEvents},
			logging.Field{Key: "timeout_events", Value: metrics.TimeoutEvents},
			logging.Field{Key: "contention_rate", Value: metrics.ContentionRate},
			logging.Field{Key: "timeout_rate", Value: metrics.TimeoutRate},
		).
		Log()

	// Analyze metrics and generate recommendations
	analysis := analyzeSpecLoaderMetrics(metrics)
	logSpecLoaderMetricsAnalysis(logger, analysis)

	// Legacy warnings (kept for backward compatibility)
	if metrics.ContentionRate > 0.1 {
		logging.Fluent(logger).Warn("High spec loader lock contention detected").
			WithFields(
				logging.Field{Key: "contention_rate", Value: metrics.ContentionRate},
				logging.Field{Key: "contention_events", Value: metrics.ContentionEvents},
				logging.Field{Key: "max_wait_time", Value: metrics.MaxWaitTime.String()},
			).
			Log()
	}

	if metrics.TimeoutRate > 0.05 {
		logging.Fluent(logger).Warn("High spec loader timeout rate detected").
			WithFields(
				logging.Field{Key: "timeout_rate", Value: metrics.TimeoutRate},
				logging.Field{Key: "timeout_events", Value: metrics.TimeoutEvents},
			).
			Log()
	}
}
