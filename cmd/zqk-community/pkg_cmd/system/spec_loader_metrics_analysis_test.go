package system

import (
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/logging"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestAnalyzeSpecLoaderMetrics_LowSeverity(t *testing.T) {
	t.Parallel()

	metrics := objects.SpecLoaderMetricsSnapshot{
		TotalWaits:       1000,
		TotalHolds:       1000,
		ContentionEvents: 20, // 1% contention rate
		TimeoutEvents:    5,  // 0.25% timeout rate
		MaxWaitTime:      100 * time.Millisecond,
		AverageWaitTime:  50 * time.Millisecond,
		MaxHoldTime:      200 * time.Millisecond,
		AverageHoldTime:  100 * time.Millisecond,
		ContentionRate:   0.01,
		TimeoutRate:      0.0025,
	}

	analysis := analyzeSpecLoaderMetrics(metrics)

	if analysis.Severity != "low" {
		t.Errorf("Expected severity 'low', got '%s'", analysis.Severity)
	}

	if len(analysis.Recommendations) > 0 {
		t.Errorf("Expected no recommendations for low severity, got %d", len(analysis.Recommendations))
	}

	if analysis.SuggestedSemaphore != 16 {
		t.Errorf("Expected suggested semaphore 16, got %d", analysis.SuggestedSemaphore)
	}
}

func TestAnalyzeSpecLoaderMetrics_HighContention(t *testing.T) {
	t.Parallel()

	metrics := objects.SpecLoaderMetricsSnapshot{
		TotalWaits:       1000,
		TotalHolds:       1000,
		ContentionEvents: 350, // 35% contention rate
		TimeoutEvents:    150, // 7.5% timeout rate
		MaxWaitTime:      15 * time.Second,
		AverageWaitTime:  2 * time.Second,
		MaxHoldTime:      3 * time.Second,
		AverageHoldTime:  500 * time.Millisecond,
		ContentionRate:   0.35,
		TimeoutRate:      0.075,
	}

	analysis := analyzeSpecLoaderMetrics(metrics)

	if analysis.Severity != "high" {
		t.Errorf("Expected severity 'high', got '%s'", analysis.Severity)
	}

	if len(analysis.Recommendations) == 0 {
		t.Error("Expected recommendations for high contention, got none")
	}

	// Should suggest increased semaphore capacity
	if analysis.SuggestedSemaphore <= 16 {
		t.Errorf("Expected suggested semaphore > 16 for high contention, got %d", analysis.SuggestedSemaphore)
	}

	// Should suggest increased timeout
	if analysis.SuggestedTimeout < 15*time.Second {
		t.Errorf("Expected suggested timeout >= 15s (based on max wait), got %v", analysis.SuggestedTimeout)
	}
}

func TestAnalyzeSpecLoaderMetrics_CriticalSeverity(t *testing.T) {
	t.Parallel()

	metrics := objects.SpecLoaderMetricsSnapshot{
		TotalWaits:       1000,
		TotalHolds:       1000,
		ContentionEvents: 600, // 60% contention rate
		TimeoutEvents:    250, // 12.5% timeout rate
		MaxWaitTime:      45 * time.Second,
		AverageWaitTime:  5 * time.Second,
		MaxHoldTime:      8 * time.Second,
		AverageHoldTime:  2 * time.Second,
		ContentionRate:   0.60,
		TimeoutRate:      0.125,
	}

	analysis := analyzeSpecLoaderMetrics(metrics)

	if analysis.Severity != "critical" {
		t.Errorf("Expected severity 'critical', got '%s'", analysis.Severity)
	}

	if len(analysis.Recommendations) == 0 {
		t.Error("Expected recommendations for critical severity, got none")
	}

	// Should suggest significantly increased semaphore capacity
	if analysis.SuggestedSemaphore <= 16 {
		t.Errorf("Expected suggested semaphore > 16 for critical contention, got %d", analysis.SuggestedSemaphore)
	}

	// Should suggest very high timeout
	if analysis.SuggestedTimeout < 45*time.Second {
		t.Errorf("Expected suggested timeout >= 45s (based on max wait), got %v", analysis.SuggestedTimeout)
	}
}

func TestAnalyzeSpecLoaderMetrics_TimeoutRate(t *testing.T) {
	t.Parallel()

	metrics := objects.SpecLoaderMetricsSnapshot{
		TotalWaits:       1000,
		TotalHolds:       1000,
		ContentionEvents: 50,
		TimeoutEvents:    250, // 12.5% timeout rate (high)
		MaxWaitTime:      20 * time.Second,
		AverageWaitTime:  1 * time.Second,
		MaxHoldTime:      2 * time.Second,
		AverageHoldTime:  500 * time.Millisecond,
		ContentionRate:   0.025,
		TimeoutRate:      0.125,
	}

	analysis := analyzeSpecLoaderMetrics(metrics)

	if analysis.Severity != "high" {
		t.Errorf("Expected severity 'high' due to timeout rate, got '%s'", analysis.Severity)
	}

	// Should have recommendation about timeout rate
	hasTimeoutRecommendation := false
	for _, rec := range analysis.Recommendations {
		if strings.Contains(rec, "timeout") {
			hasTimeoutRecommendation = true
			break
		}
	}
	if !hasTimeoutRecommendation {
		t.Error("Expected recommendation about timeout rate")
	}
}

func TestLogSpecLoaderMetricsAnalysis(t *testing.T) {
	t.Parallel()

	logger := logging.GetLoggerFromProfile("test")

	analysis := SpecLoaderMetricsAnalysis{
		Metrics: objects.SpecLoaderMetricsSnapshot{
			TotalWaits:      1000,
			ContentionRate:  0.15,
			TimeoutRate:     0.05,
			MaxWaitTime:     5 * time.Second,
			AverageWaitTime: 1 * time.Second,
		},
		Severity:           "medium",
		SuggestedTimeout:   10 * time.Second,
		SuggestedSemaphore: 24,
		Recommendations: []string{
			"Test recommendation 1",
			"Test recommendation 2",
		},
	}

	// Should not panic
	logSpecLoaderMetricsAnalysis(logger, analysis)
}

func TestGetRecommendedSemaphoreCapacity_NoData(t *testing.T) {
	t.Parallel()

	// When no metrics data exists, should return default
	capacity := getRecommendedSemaphoreCapacity()
	if capacity != 16 {
		t.Errorf("Expected default semaphore capacity 16, got %d", capacity)
	}
}
