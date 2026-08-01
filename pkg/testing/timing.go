package testing

import "github.com/lanceman/zqk/pkg/testtiming"

// Re-export test timing helpers from pkg/testtiming for backward compatibility.

// TestTiming tracks test execution durations for timeout estimation
type TestTiming = testtiming.TestTiming

var (
	// RecordTestTiming records the duration of a test execution
	RecordTestTiming = testtiming.RecordTestTiming
	// GetExpectedTimeout returns the recommended timeout for a test based on historical data
	GetExpectedTimeout = testtiming.GetExpectedTimeout
	// ReportTestTiming is a test helper that wraps test execution and records timing
	ReportTestTiming = testtiming.ReportTestTiming
)
