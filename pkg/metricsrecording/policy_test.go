package metricsrecording

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/config"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

const testSkipReasonEnvOptIn = "ZQK_TEST_METRICS_RECORDING opts in whole suite"

func TestEnabled_OptInRefCount(t *testing.T) {
	if !testing.Testing() {
		t.Skip("test binary only")
	}
	if config.TestingMetricsRecording().Safe() {
		t.Skip(testSkipReasonEnvOptIn)
	}
	if Enabled() {
		t.Fatal("expected default disabled in test binary without opt-in")
	}
	EnterAllowRecording()
	defer LeaveAllowRecording()
	if !Enabled() {
		t.Fatal("expected enabled after EnterAllowRecording")
	}
}

func TestEnabled_HotPathNoPersist(t *testing.T) {
	if config.TestingMetricsRecording().Safe() {
		t.Skip(testSkipReasonEnvOptIn)
	}
	EnterAllowRecording()
	defer LeaveAllowRecording()
	if !Enabled() {
		t.Fatal("expected enabled after EnterAllowRecording")
	}
	EnterHotPathNoPersist()
	if Enabled() {
		t.Fatal("hot path must suppress recording")
	}
	LeaveHotPathNoPersist()
	if !Enabled() {
		t.Fatal("expected recording restored after LeaveHotPathNoPersist")
	}
}

func TestEnabled_TestMetricsRecordingEnv(t *testing.T) {
	t.Setenv(zqkenv.TestMetricsRecording().Name(), "1")
	if !Enabled() {
		t.Fatal("expected Enabled() to return true when ZQK_TEST_METRICS_RECORDING=1")
	}
}
