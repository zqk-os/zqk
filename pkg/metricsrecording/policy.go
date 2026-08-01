// Package metricsrecording centralizes whether test runs should record metrics (storage
// counters, pipeline sampling, metric object creation). Production binaries always record;
// binaries built with "go test" default to not recording unless opted in.
package metricsrecording

import (
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

const (
	envEnabledNumeric = "1"
	envEnabledLiteral = "true"
)

var allowRecordingRef int32

// EnterAllowRecording marks the current test scope as allowed to record metrics (refcounted).
// Pair with LeaveAllowRecording via t.Cleanup. Used by storage.TestingFactory when
// TestEnvironmentOptions.NoopStorageMetrics is false, or by tests that exercise metrics.
func EnterAllowRecording() {
	atomic.AddInt32(&allowRecordingRef, 1)
}

// LeaveAllowRecording ends one scope started with EnterAllowRecording.
func LeaveAllowRecording() {
	for {
		v := atomic.LoadInt32(&allowRecordingRef)
		if v <= 0 {
			return
		}
		if atomic.CompareAndSwapInt32(&allowRecordingRef, v, v-1) {
			return
		}
	}
}

// Enabled returns true if metrics recording should run (storage counters, pipeline Sample,
// MetricFactory creates, etc.).
//
// In non-test binaries, this is always true. In test binaries, recording is off unless:
//   - ZQK_TEST_METRICS_RECORDING is 1 or true (whole suite / CI), or
//   - at least one active EnterAllowRecording scope (e.g. SetupCompleteTestEnvironment with
//     NoopStorageMetrics: false), or
//   - tests call EnterAllowRecording directly.
func Enabled() bool {
	if !testing.Testing() {
		return true
	}
	if v := os.Getenv(zqkenv.TestMetricsRecording()); isTruthy(v) {
		return true
	}
	return atomic.LoadInt32(&allowRecordingRef) > 0
}

func isTruthy(value string) bool {
	return value == envEnabledNumeric || strings.EqualFold(value, envEnabledLiteral)
}
