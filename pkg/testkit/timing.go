// Package testtiming records per-test durations and suggests timeouts for scheduler run_wrapper jobs
// without requiring importers to depend on the full pkg/testing surface.
package testkit

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TimingJSONRelativePath is written under the current working directory (typically project root).
const TimingJSONRelativePath = ".zqk/test_timings.json"

// TestTiming tracks test execution durations for timeout estimation
type TestTiming struct {
	TestName        string        `json:"test_name"`
	Package         string        `json:"package"`
	Duration        time.Duration `json:"duration"`
	Timestamp       time.Time     `json:"timestamp"`
	Success         bool          `json:"success"`
	RunCount        int           `json:"run_count"`
	AverageDuration time.Duration `json:"average_duration"`
	MinDuration     time.Duration `json:"min_duration"`
	MaxDuration     time.Duration `json:"max_duration"`
}

var (
	timingMutex sync.Mutex
	testTimings = make(map[string]*TestTiming)
)

// RecordTestTiming records the duration of a test execution
// This enables timeout estimation for scheduler jobs running tests
func RecordTestTiming(t *testing.T, duration time.Duration) {
	_ = concurrency.RunInLock(&timingMutex, func() error {
		testKey := fmt.Sprintf("%s/%s", t.Name(), getPackageName())

		timing, exists := testTimings[testKey]
		if !exists {
			timing = &TestTiming{
				TestName:    t.Name(),
				Package:     getPackageName(),
				MinDuration: duration,
				MaxDuration: duration,
			}
			testTimings[testKey] = timing
		}

		timing.RunCount++
		timing.Duration = duration
		timing.Timestamp = time.Now()
		timing.Success = !t.Failed()

		if duration < timing.MinDuration {
			timing.MinDuration = duration
		}
		if duration > timing.MaxDuration {
			timing.MaxDuration = duration
		}

		if timing.RunCount == 1 {
			timing.AverageDuration = duration
		} else {
			alpha := 0.3
			timing.AverageDuration = time.Duration(float64(timing.AverageDuration)*0.7 + float64(duration)*alpha)
		}

		saveTimings()

		expectedTimeout := timing.AverageDuration * 3
		if timing.MaxDuration > expectedTimeout {
			expectedTimeout = timing.MaxDuration * 2
		}

		t.Logf("Test timing: duration=%v, avg=%v, min=%v, max=%v, runs=%d, expected_timeout=%v",
			duration, timing.AverageDuration, timing.MinDuration, timing.MaxDuration, timing.RunCount, expectedTimeout)
		return nil
	})
}

// GetExpectedTimeout returns the recommended timeout for a test based on historical data
func GetExpectedTimeout(testName, packageName string) time.Duration {
	var timeout time.Duration
	_ = concurrency.RunInLock(&timingMutex, func() error {
		testKey := fmt.Sprintf("%s/%s", testName, packageName)
		timing, exists := testTimings[testKey]
		if !exists {
			timeout = 5 * time.Minute
			return nil
		}

		timeout = timing.AverageDuration * 3
		if timing.MaxDuration*2 > timeout {
			timeout = timing.MaxDuration * 2
		}
		if timeout < 30*time.Second {
			timeout = 30 * time.Second
		}
		if timeout > 1*time.Hour {
			timeout = 1 * time.Hour
		}
		return nil
	})
	return timeout
}

// ReportTestTiming is a test helper that wraps test execution and records timing
// Usage: defer ReportTestTiming(t, time.Now())()
func ReportTestTiming(t *testing.T, startTime time.Time) func() {
	return func() {
		duration := time.Since(startTime)
		RecordTestTiming(t, duration)
	}
}

func saveTimings() {
	dir := filepath.Dir(TimingJSONRelativePath)
	if err := fileutil.EnsureDir(dir); err != nil {
		return // Silently fail - timing is optional
	}

	timings := make([]*TestTiming, 0, len(testTimings))
	for _, timing := range testTimings {
		timings = append(timings, timing)
	}

	data, err := json.MarshalIndent(timings, "", "  ")
	if err != nil {
		return
	}

	_ = fileutil.WriteSecureFile(TimingJSONRelativePath, data) //nolint:errcheck // Timing data write errors are non-critical
}

func loadTimings() {
	data, err := fileutil.ReadFile(TimingJSONRelativePath)
	if err != nil {
		return // File doesn't exist yet - that's okay
	}

	var timings []*TestTiming
	if err := json.Unmarshal(data, &timings); err != nil {
		return
	}

	for _, timing := range timings {
		testKey := fmt.Sprintf("%s/%s", timing.TestName, timing.Package)
		testTimings[testKey] = timing
	}
}

// LoadTimingsIntoMap loads timing JSON into the provided map (used by the test scanner accessor).
func LoadTimingsIntoMap(timings map[string]*TestTiming) {
	data, err := fileutil.ReadFile(TimingJSONRelativePath)
	if err != nil {
		return // File doesn't exist yet - that's okay
	}

	var timingList []*TestTiming
	if err := json.Unmarshal(data, &timingList); err != nil {
		return
	}

	for _, timing := range timingList {
		testKey := fmt.Sprintf("%s/%s", timing.TestName, timing.Package)
		timings[testKey] = timing
	}
}

func getPackageName() string {
	wd, err := fileutil.Getwd()
	if err != nil {
		return "unknown"
	}
	return filepath.Base(wd)
}

func init() {
	loadTimings()
}
