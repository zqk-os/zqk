// Package metricsrecording centralizes whether test runs should record metrics (storage
// counters, pipeline sampling, metric object creation). Production binaries always record;
// binaries built with "go test" default to not recording unless opted in.
package metricsrecording

import (
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zqk-os/zqk/pkg/config"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

const (
	envEnabledNumeric = "1"
	envEnabledLiteral = "true"
)

var allowRecordingRef int32
var hotPathNoPersistRef int32

// EnterAllowRecording marks the current test scope as allowed to record metrics (refcounted).
// Pair with LeaveAllowRecording via t.Cleanup. Used by storage.TestingFactory when
// TestEnvironmentOptions.NoopStorageMetrics is false, or by tests that exercise metrics.
func EnterAllowRecording() {
	atomic.AddInt32(&allowRecordingRef, 1)
}

// LeaveAllowRecording ends one scope started with EnterAllowRecording.
func LeaveAllowRecording() {
	decRef(&allowRecordingRef)
}

// EnterHotPathNoPersist suppresses metric-object Create (and related recording)
// for the rest of this process until LeaveHotPathNoPersist. system check was
// spawning hundreds of coordinator_metrics_router Gs that serialized on
// FileObjectStorage.Create and timed out the 5m CLI floor.
func EnterHotPathNoPersist() {
	atomic.AddInt32(&hotPathNoPersistRef, 1)
}

// LeaveHotPathNoPersist ends one EnterHotPathNoPersist scope.
func LeaveHotPathNoPersist() {
	decRef(&hotPathNoPersistRef)
}

func decRef(p *int32) {
	for {
		v := atomic.LoadInt32(p)
		if v <= 0 {
			return
		}
		if atomic.CompareAndSwapInt32(p, v, v-1) {
			return
		}
	}
}

// Enabled returns true if metrics recording should run (storage counters, pipeline Sample,
// MetricFactory creates, etc.).
//
// In non-test binaries, this is always true unless EnterHotPathNoPersist is active
// (system check). In test binaries, recording is off unless:
//   - ZQK_TEST_METRICS_RECORDING is 1 or true (whole suite / CI), or
//   - at least one active EnterAllowRecording scope (e.g. SetupCompleteTestEnvironment with
//     NoopStorageMetrics: false), or
//   - tests call EnterAllowRecording directly.
func Enabled() bool {
	if atomic.LoadInt32(&hotPathNoPersistRef) > 0 {
		return false
	}
	if !testing.Testing() {
		return true
	}
	if isTruthy(zqkenv.TestMetricsRecording().Get()) || config.TestingMetricsRecording().OrDefault(false) {
		return true
	}
	return atomic.LoadInt32(&allowRecordingRef) > 0
}

func isTruthy(value string) bool {
	return value == envEnabledNumeric || strings.EqualFold(value, envEnabledLiteral)
}

// CollectAndReset executes collection if metrics recording is enabled, then executes reset.
func CollectAndReset(collect func() (string, error), reset func()) (string, error) {
	if !Enabled() {
		return "", nil
	}
	metricID, err := collect()
	if err != nil {
		return "", err
	}
	if reset != nil {
		reset()
	}
	return metricID, nil
}
