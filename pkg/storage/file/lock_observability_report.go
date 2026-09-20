package file

import (
	"time"
)

// LockObservabilityReport aggregates lock contention, durations, and health indicators.
type LockObservabilityReport struct {
	TotalAttempts     int64
	TotalAcquisitions int64
	TotalFailures     int64
	TotalTimeouts     int64
	TotalContention   int64
	ContentionRate    float64
	AvgWaitTime       time.Duration
	MaxWaitTime       time.Duration
	Histogram         HistogramSnapshot
	Healthy           bool
	Warnings          []string
}

// GenerateLockObservabilityReport compiles a LockObservabilityReport from metrics and histogram.
func GenerateLockObservabilityReport(m *FileLockMetrics, h *ContentionHistogram) LockObservabilityReport {
	if m == nil {
		m = GetFileLockMetrics()
	}
	if h == nil {
		h = GetGlobalContentionHistogram()
	}

	acquisitions := m.TotalAcquisitions.Load()
	failures := m.TotalFailures.Load()
	timeouts := m.TotalTimeouts.Load()
	contention := m.TotalContention.Load()
	totalWait := m.TotalWaitTime.Load()
	maxWait := m.MaxWaitTime.Load()

	totalAttempts := acquisitions + failures
	var contentionRate float64
	if totalAttempts > 0 {
		contentionRate = float64(contention) / float64(totalAttempts)
	}

	var avgWait time.Duration
	if totalAttempts > 0 && totalWait > 0 {
		avgWait = time.Duration(totalWait / totalAttempts)
	}

	histSnap := h.Snapshot()

	var warnings []string
	healthy := true

	if contentionRate > 0.40 {
		healthy = false
		warnings = append(warnings, "high lock contention detected (>40%)")
	}

	if time.Duration(maxWait) > 5*time.Second {
		warnings = append(warnings, "excessive maximum lock wait time observed (>5s)")
	}

	if timeouts > 0 && float64(timeouts)/float64(totalAttempts) > 0.05 {
		healthy = false
		warnings = append(warnings, "lock timeout failure rate exceeds 5%")
	}

	return LockObservabilityReport{
		TotalAttempts:     totalAttempts,
		TotalAcquisitions: acquisitions,
		TotalFailures:     failures,
		TotalTimeouts:     timeouts,
		TotalContention:   contention,
		ContentionRate:    contentionRate,
		AvgWaitTime:       avgWait,
		MaxWaitTime:       time.Duration(maxWait),
		Histogram:         histSnap,
		Healthy:           healthy,
		Warnings:          warnings,
	}
}
