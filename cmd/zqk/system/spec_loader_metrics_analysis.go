package system

import (
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"

	"github.com/zqk-os/zqk/pkg/objects"
)

// analyzeSpecLoaderMetrics analyzes spec loader metrics and provides recommendations
// This helps inform improvements to the CLI based on actual lock contention patterns
type SpecLoaderMetricsAnalysis struct {
	Metrics            objects.SpecLoaderMetricsSnapshot
	Recommendations    []string
	Severity           string // "low", "medium", "high", "critical"
	SuggestedTimeout   time.Duration
	SuggestedSemaphore int
	ContentionTrend    string // "improving", "stable", "worsening"
	TimeoutTrend       string
}

// analyzeSpecLoaderMetrics analyzes metrics and generates recommendations
func analyzeSpecLoaderMetrics(metrics objects.SpecLoaderMetricsSnapshot) SpecLoaderMetricsAnalysis {
	analysis := SpecLoaderMetricsAnalysis{
		Metrics:  metrics,
		Severity: "low",
	}

	// Helper function to upgrade severity (only increases, never decreases)
	upgradeSeverity := func(current, new string) string {
		severityLevels := map[string]int{
			"low":      1,
			"medium":   2,
			"high":     3,
			"critical": 4,
		}
		if severityLevels[new] > severityLevels[current] {
			return new
		}
		return current
	}

	// Analyze contention rate
	if metrics.ContentionRate > 0.5 {
		analysis.Severity = upgradeSeverity(analysis.Severity, "critical")
		analysis.Recommendations = append(analysis.Recommendations,
			"CRITICAL: Contention rate >50% - system is severely overloaded. Consider reducing concurrent operations or increasing semaphore capacity.")
	} else if metrics.ContentionRate > 0.3 {
		analysis.Severity = upgradeSeverity(analysis.Severity, "high")
		analysis.Recommendations = append(analysis.Recommendations,
			"High contention rate (>30%) - consider increasing semaphore capacity or reducing concurrent validation goroutines.")
	} else if metrics.ContentionRate > 0.1 {
		analysis.Severity = upgradeSeverity(analysis.Severity, "medium")
		analysis.Recommendations = append(analysis.Recommendations,
			"Moderate contention detected (>10%) - monitor closely. Consider optimizing lock granularity.")
	}

	// Analyze timeout rate
	if metrics.TimeoutRate > 0.2 {
		analysis.Severity = upgradeSeverity(analysis.Severity, "critical")
		analysis.Recommendations = append(analysis.Recommendations,
			fmt.Sprintf("CRITICAL: Timeout rate >20%% (%d timeouts) - timeouts are too short or contention is extreme. Increase timeout from current max.", metrics.TimeoutEvents))
	} else if metrics.TimeoutRate > 0.1 {
		analysis.Severity = upgradeSeverity(analysis.Severity, "high")
		analysis.Recommendations = append(analysis.Recommendations,
			fmt.Sprintf("High timeout rate (>10%%, %d timeouts) - consider increasing lock timeout duration.", metrics.TimeoutEvents))
	} else if metrics.TimeoutRate > 0.05 {
		analysis.Severity = upgradeSeverity(analysis.Severity, "medium")
		analysis.Recommendations = append(analysis.Recommendations,
			fmt.Sprintf("Moderate timeout rate (>5%%, %d timeouts) - monitor timeout patterns.", metrics.TimeoutEvents))
	}

	// Analyze max wait time
	if metrics.MaxWaitTime > 30*time.Second {
		analysis.Severity = upgradeSeverity(analysis.Severity, "critical")
		analysis.Recommendations = append(analysis.Recommendations,
			fmt.Sprintf("CRITICAL: Max wait time %v exceeds 30s - locks are severely contended. Immediate action required.", metrics.MaxWaitTime))
	} else if metrics.MaxWaitTime > 10*time.Second {
		analysis.Severity = upgradeSeverity(analysis.Severity, "high")
		analysis.Recommendations = append(analysis.Recommendations,
			fmt.Sprintf("High max wait time %v - consider increasing semaphore capacity or optimizing lock granularity.", metrics.MaxWaitTime))
	} else if metrics.MaxWaitTime > 5*time.Second {
		analysis.Severity = upgradeSeverity(analysis.Severity, "medium")
		analysis.Recommendations = append(analysis.Recommendations,
			fmt.Sprintf("Moderate max wait time %v - monitor for degradation.", metrics.MaxWaitTime))
	}

	// Analyze average wait time
	if metrics.AverageWaitTime > 1*time.Second {
		analysis.Severity = upgradeSeverity(analysis.Severity, "high")
		analysis.Recommendations = append(analysis.Recommendations,
			fmt.Sprintf("High average wait time %v - system is experiencing consistent contention.", metrics.AverageWaitTime))
	}

	// Analyze max hold time (indicates slow operations)
	if metrics.MaxHoldTime > 5*time.Second {
		analysis.Severity = upgradeSeverity(analysis.Severity, "high")
		analysis.Recommendations = append(analysis.Recommendations,
			fmt.Sprintf("Long lock hold time detected (%v) - operations are slow. Consider optimizing spec loading or reducing lock scope.", metrics.MaxHoldTime))
	}

	// Calculate suggested timeout based on metrics
	// Use 95th percentile wait time + 50% margin, clamped to reasonable bounds
	if metrics.TotalWaits > 0 {
		// Estimate 95th percentile as 2x average (conservative)
		estimated95thPercentile := metrics.AverageWaitTime * 2
		if metrics.MaxWaitTime > estimated95thPercentile {
			estimated95thPercentile = metrics.MaxWaitTime
		}
		analysis.SuggestedTimeout = estimated95thPercentile * 3 / 2 // 50% margin

		// Clamp to reasonable bounds
		minTimeout := 100 * time.Millisecond
		maxTimeout := 120 * time.Second // Allow up to 2 minutes for extreme cases
		if analysis.SuggestedTimeout < minTimeout {
			analysis.SuggestedTimeout = minTimeout
		}
		if analysis.SuggestedTimeout > maxTimeout {
			analysis.SuggestedTimeout = maxTimeout
		}
	} else {
		analysis.SuggestedTimeout = 60 * time.Second // Default if no data
	}

	// Calculate suggested semaphore capacity based on contention
	// If contention is high, increase semaphore to allow more concurrent operations
	baseSemaphore := 16 // Current default (NumCPU * 2)
	if metrics.ContentionRate > 0.3 {
		// High contention - increase semaphore significantly
		analysis.SuggestedSemaphore = baseSemaphore * 2
		analysis.Recommendations = append(analysis.Recommendations,
			fmt.Sprintf("Suggested semaphore capacity: %d (increased from %d due to high contention)", analysis.SuggestedSemaphore, baseSemaphore))
	} else if metrics.ContentionRate > 0.1 {
		// Moderate contention - slight increase
		analysis.SuggestedSemaphore = int(float64(baseSemaphore) * 1.5)
		analysis.Recommendations = append(analysis.Recommendations,
			fmt.Sprintf("Suggested semaphore capacity: %d (increased from %d due to moderate contention)", analysis.SuggestedSemaphore, baseSemaphore))
	} else {
		analysis.SuggestedSemaphore = baseSemaphore
	}

	// Determine trends (would need historical data - placeholder for now)
	analysis.ContentionTrend = "stable"
	analysis.TimeoutTrend = "stable"

	return analysis
}

// logSpecLoaderMetricsAnalysis logs metrics analysis and recommendations
func logSpecLoaderMetricsAnalysis(logger logging.Logger, analysis SpecLoaderMetricsAnalysis) {
	if logger == nil {
		return
	}

	// Log analysis summary
	logging.Fluent(logger).Info("Spec loader metrics analysis").
		WithFields(
			logging.Field{Key: "severity", Value: analysis.Severity},
			logging.Field{Key: "contention_rate", Value: fmt.Sprintf("%.2f%%", analysis.Metrics.ContentionRate*100)},
			logging.Field{Key: "timeout_rate", Value: fmt.Sprintf("%.2f%%", analysis.Metrics.TimeoutRate*100)},
			logging.Field{Key: "max_wait_time", Value: analysis.Metrics.MaxWaitTime.String()},
			logging.Field{Key: "avg_wait_time", Value: analysis.Metrics.AverageWaitTime.String()},
			logging.Field{Key: "suggested_timeout", Value: analysis.SuggestedTimeout.String()},
			logging.Field{Key: "suggested_semaphore", Value: analysis.SuggestedSemaphore},
		).
		Log()

	// Log recommendations
	if len(analysis.Recommendations) > 0 {
		logging.Fluent(logger).Warn("Spec loader metrics recommendations").
			Int("recommendation_count", len(analysis.Recommendations)).
			Log()
		for i, rec := range analysis.Recommendations {
			logging.Fluent(logger).Info(fmt.Sprintf("Recommendation %d", i+1)).
				WithFields(logging.Field{Key: "recommendation", Value: rec}).
				Log()
		}
	}
}
