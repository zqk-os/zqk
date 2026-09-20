package git

import (
	"testing"
	"time"
)

// Metrics tests use package-global state; do not use t.Parallel() here.

func TestMetrics_RecordAnalysis(t *testing.T) {
	ResetMetrics()

	// Record some analysis operations
	RecordAnalysis(100*time.Millisecond, true)
	RecordAnalysis(200*time.Millisecond, true)
	RecordAnalysis(150*time.Millisecond, false)

	metrics := GetMetrics()

	if metrics.CommitsAnalyzed != 3 {
		t.Errorf("expected 3 commits analyzed, got %d", metrics.CommitsAnalyzed)
	}

	if metrics.CommitsFailed != 1 {
		t.Errorf("expected 1 failed commit, got %d", metrics.CommitsFailed)
	}

	if metrics.AvgAnalysisTime == 0 {
		t.Error("expected average analysis time to be set")
	}
}

func TestMetrics_RecordLinking(t *testing.T) {
	ResetMetrics()

	// Record some linking operations
	RecordLinking(50*time.Millisecond, true)
	RecordLinking(75*time.Millisecond, true)
	RecordLinking(60*time.Millisecond, false)

	metrics := GetMetrics()

	if metrics.CommitsLinked != 2 {
		t.Errorf("expected 2 commits linked, got %d", metrics.CommitsLinked)
	}

	if metrics.CommitsFailed != 1 {
		t.Errorf("expected 1 failed link, got %d", metrics.CommitsFailed)
	}

	if metrics.AvgLinkingTime == 0 {
		t.Error("expected average linking time to be set")
	}
}

func TestMetrics_RecordRetry(t *testing.T) {
	ResetMetrics()

	RecordRetry()
	RecordRetry()

	metrics := GetMetrics()

	if metrics.RetryCount != 2 {
		t.Errorf("expected 2 retries, got %d", metrics.RetryCount)
	}
}

func TestMetrics_RecordTimeout(t *testing.T) {
	ResetMetrics()

	RecordTimeout()
	RecordTimeout()
	RecordTimeout()

	metrics := GetMetrics()

	if metrics.TimeoutCount != 3 {
		t.Errorf("expected 3 timeouts, got %d", metrics.TimeoutCount)
	}
}

func TestMetrics_Reset(t *testing.T) {
	// Set some metrics
	RecordAnalysis(100*time.Millisecond, true)
	RecordLinking(50*time.Millisecond, true)
	RecordRetry()
	RecordTimeout()

	// Reset
	ResetMetrics()

	metrics := GetMetrics()

	if metrics.CommitsAnalyzed != 0 {
		t.Error("expected metrics to be reset")
	}
	if metrics.CommitsLinked != 0 {
		t.Error("expected metrics to be reset")
	}
	if metrics.RetryCount != 0 {
		t.Error("expected metrics to be reset")
	}
	if metrics.TimeoutCount != 0 {
		t.Error("expected metrics to be reset")
	}
}
