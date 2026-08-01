package git

import (
	"sync"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"

	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/logging"
)

// OperationMetrics tracks metrics for Git operations
type OperationMetrics struct {
	// Analysis metrics
	CommitsAnalyzed   int64
	CommitsLinked     int64
	CommitsFailed     int64
	TotalAnalysisTime time.Duration
	TotalLinkingTime  time.Duration

	// Per-operation metrics
	AvgAnalysisTime time.Duration
	AvgLinkingTime  time.Duration

	// Error tracking
	RetryCount   int64
	TimeoutCount int64

	mu sync.RWMutex
}

var globalMetrics = &OperationMetrics{}

// GetMetrics returns the global operation metrics
func GetMetrics() *OperationMetrics {
	var metrics *OperationMetrics
	_ = concurrency.RunInRLockWithLogger(
		&globalMetrics.mu, LockNameGitMetricsGet, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Return a copy to avoid race conditions
			metrics = &OperationMetrics{
				CommitsAnalyzed:   globalMetrics.CommitsAnalyzed,
				CommitsLinked:     globalMetrics.CommitsLinked,
				CommitsFailed:     globalMetrics.CommitsFailed,
				TotalAnalysisTime: globalMetrics.TotalAnalysisTime,
				TotalLinkingTime:  globalMetrics.TotalLinkingTime,
				AvgAnalysisTime:   globalMetrics.AvgAnalysisTime,
				AvgLinkingTime:    globalMetrics.AvgLinkingTime,
				RetryCount:        globalMetrics.RetryCount,
				TimeoutCount:      globalMetrics.TimeoutCount,
			}
			return nil
		},
	)
	return metrics
}

// RecordAnalysis records metrics for a commit analysis operation
func RecordAnalysis(duration time.Duration, success bool) {
	_ = concurrency.RunInLockWithLogger(
		&globalMetrics.mu, LockNameGitMetricsRecordAnalysis, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			globalMetrics.CommitsAnalyzed++
			globalMetrics.TotalAnalysisTime += duration

			// Update average (exponential moving average)
			if globalMetrics.CommitsAnalyzed == 1 {
				globalMetrics.AvgAnalysisTime = duration
			} else {
				//nolint:gocritic // Documenting EMA formula for clarity
				globalMetrics.AvgAnalysisTime = time.Duration(
					float64(globalMetrics.AvgAnalysisTime)*0.9 + float64(duration)*0.1)
			}

			if !success {
				globalMetrics.CommitsFailed++
			}
			return nil
		},
	)
}

// RecordLinking records metrics for a commit linking operation
func RecordLinking(duration time.Duration, success bool) {
	_ = concurrency.RunInLockWithLogger(
		&globalMetrics.mu, LockNameGitMetricsRecordLinking, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if success {
				globalMetrics.CommitsLinked++
			} else {
				globalMetrics.CommitsFailed++
			}

			globalMetrics.TotalLinkingTime += duration

			// Update average (exponential moving average)
			totalOps := globalMetrics.CommitsLinked + globalMetrics.CommitsFailed
			if totalOps == 1 {
				globalMetrics.AvgLinkingTime = duration
			} else {
				//nolint:gocritic // Documenting EMA formula for clarity
				globalMetrics.AvgLinkingTime = time.Duration(
					float64(globalMetrics.AvgLinkingTime)*0.9 + float64(duration)*0.1)
			}
			return nil
		},
	)
}

// RecordRetry records a retry operation
func RecordRetry() {
	_ = concurrency.RunInLockWithLogger(
		&globalMetrics.mu, LockNameGitMetricsRecordRetry, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			globalMetrics.RetryCount++
			return nil
		},
	)
}

// RecordTimeout records a timeout
func RecordTimeout() {
	_ = concurrency.RunInLockWithLogger(
		&globalMetrics.mu, LockNameGitMetricsRecordTimeout, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			globalMetrics.TimeoutCount++
			return nil
		},
	)
}

// ResetMetrics resets all metrics (useful for testing)
func ResetMetrics() {
	_ = concurrency.RunInLockWithLogger(
		&globalMetrics.mu, LockNameGitMetricsReset, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			globalMetrics.CommitsAnalyzed = 0
			globalMetrics.CommitsLinked = 0
			globalMetrics.CommitsFailed = 0
			globalMetrics.TotalAnalysisTime = 0
			globalMetrics.TotalLinkingTime = 0
			globalMetrics.AvgAnalysisTime = 0
			globalMetrics.AvgLinkingTime = 0
			globalMetrics.RetryCount = 0
			globalMetrics.TimeoutCount = 0
			return nil
		},
	)
}
