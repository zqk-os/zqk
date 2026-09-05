package testing

import "github.com/lanceman/zqk/pkg/testkit"

// Re-export test timing helpers from pkg/testkit for backward compatibility.

// TestTiming tracks test execution durations for timeout estimation
type TestTiming = testkit.TestTiming

var (
	// RecordTestTiming records the duration of a test execution
	RecordTestTiming = testkit.RecordTestTiming
	// GetExpectedTimeout returns the recommended timeout for a test based on historical data
	GetExpectedTimeout = testkit.GetExpectedTimeout
	// ReportTestTiming is a test helper that wraps test execution and records timing
	ReportTestTiming = testkit.ReportTestTiming
)
