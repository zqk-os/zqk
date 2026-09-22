// BLI-STARTER-COMMUNITY-043 / PRI-STARTER-COMMUNITY-043 coverage elevation
package metricsrecording

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestExtraKindGatesAndRefcountFloor(t *testing.T) {
	ClearDeniedKinds()

	DenyKind("")
	DenyKind("metric_event")
	t.Cleanup(func() {
		AllowKind("metric_event")
		ClearDeniedKinds()
	})

	EnterAllowRecording()
	t.Cleanup(LeaveAllowRecording)

	if !EnabledForKind("") {
		t.Fatal("empty kind")
	}
	if EnabledForKind("metric_event") {
		t.Fatal("denied kind")
	}
	AllowKind("metric_event")
	if !EnabledForKind("metric_event") {
		t.Fatal("allowed again")
	}
	DenyKind("metric_event")
	ClearDeniedKinds()
	if !EnabledForKind("metric_event") {
		t.Fatal("cleared denials")
	}

	t.Setenv(zqkenv.TestMetricsRecording().Name(), "TRUE")
	if !Enabled() {
		t.Fatal("true env")
	}
	if !isTruthy("true") || isTruthy("no") {
		t.Fatal("isTruthy")
	}
}
