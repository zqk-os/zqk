package scheduler

import (
	"testing"
)

func TestBuildAfterStateSnapshotMap_alwaysEmitsSessionCompletionBlockedReasons(t *testing.T) {
	t.Parallel()
	snap := &TestBundleConvergenceSnapshot{
		HealthWatermarkRFC3339:          "2026-01-02T00:00:00Z",
		LinesInWindow:                   3,
		DeltaAssessment:                 "neutral",
		ReadyForSessionCompletion:       true,
		SessionCompletionBlockedReasons: nil,
		FailingFingerprintsNow:          []string{},
		FingerprintLatestOutcome:        map[string]string{},
	}
	m := BuildAfterStateSnapshotMap(snap)
	br, ok := m[convSugKeySessionCompletionBlockedReasons].([]string)
	if !ok {
		t.Fatalf("blocked reasons type got %T", m[convSugKeySessionCompletionBlockedReasons])
	}
	if len(br) != 0 {
		t.Fatalf("want empty slice, got %#v", br)
	}
}

func TestBuildAfterStateSnapshotMap_preservesNonEmptyBlockedReasons(t *testing.T) {
	t.Parallel()
	snap := &TestBundleConvergenceSnapshot{
		HealthWatermarkRFC3339:    "2026-01-02T00:00:00Z",
		LinesInWindow:             0,
		DeltaAssessment:           "unknown",
		ReadyForSessionCompletion: false,
		SessionCompletionBlockedReasons: []string{
			"no health data in window",
		},
		FailingFingerprintsNow:   []string{},
		FingerprintLatestOutcome: map[string]string{},
	}
	m := BuildAfterStateSnapshotMap(snap)
	br, ok := m[convSugKeySessionCompletionBlockedReasons].([]string)
	if !ok {
		t.Fatalf("blocked reasons type got %T", m[convSugKeySessionCompletionBlockedReasons])
	}
	if len(br) != 1 || br[0] != "no health data in window" {
		t.Fatalf("got %#v", br)
	}
}
