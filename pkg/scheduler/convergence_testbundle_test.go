package scheduler

import (
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/convergerollup"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

func TestBuildTestBundleConvergenceSnapshot_Empty(t *testing.T) {
	t.Parallel()
	d := t.TempDir()
	s := BuildTestBundleConvergenceSnapshot(d, nil)
	if s.DeltaAssessment != "unknown" {
		t.Fatalf("delta: got %q", s.DeltaAssessment)
	}
	if s.ReadyForSessionCompletion {
		t.Fatal("expected not ready for empty health")
	}
	if len(s.SessionCompletionBlockedReasons) == 0 {
		t.Fatal("expected blocked reasons")
	}
	if s.PrimaryMeasurementOutcome != string(convergerollup.MeasurementYieldsAmbiguousOutcome) {
		t.Fatalf("primary_measurement_outcome: got %q want ambiguous (no lines / partial gate)", s.PrimaryMeasurementOutcome)
	}
}

func TestBuildTestBundleConvergenceSnapshot_TrendingAway(t *testing.T) {
	t.Parallel()
	d := t.TempDir()
	ts := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)
	lines := []map[string]any{
		{
			KeyTimestamp:                ts,
			KeyBundleCommandFingerprint: "abc123",
			KeyTestOutcome:              "test_fail",
			KeySuggestedRerunCommands:   []any{"go test ./pkg/foo -run TestX -timeout 60s"},
		},
	}
	s := BuildTestBundleConvergenceSnapshot(d, lines)
	if s.DeltaAssessment != "trending_away" {
		t.Fatalf("delta: got %q want trending_away", s.DeltaAssessment)
	}
	if len(s.FailingFingerprintsNow) != 1 || s.FailingFingerprintsNow[0] != "abc123" {
		t.Fatalf("failing: %+v", s.FailingFingerprintsNow)
	}
	if len(s.SuggestedRerunByFingerprint["abc123"]) != 1 {
		t.Fatalf("suggested: %+v", s.SuggestedRerunByFingerprint)
	}
	if s.Heartbeat == nil || s.Heartbeat.Stale {
		t.Fatalf("heartbeat: %+v", s.Heartbeat)
	}
	if s.ReadyForSessionCompletion {
		t.Fatal("expected not ready when fingerprints failing")
	}
	if s.PrimaryMeasurementOutcome != string(convergerollup.MeasurementYieldsDivergence) {
		t.Fatalf("primary_measurement_outcome: got %q want divergence", s.PrimaryMeasurementOutcome)
	}
}

func TestBuildTestBundleConvergenceSnapshot_RecoveredFailureSameFingerprint_IsNeutral(t *testing.T) {
	t.Parallel()
	d := t.TempDir()
	ts1 := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	ts2 := time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339)
	lines := []map[string]any{
		{
			KeyTimestamp:                ts1,
			KeyBundleCommandFingerprint: "fp1",
			KeyTestOutcome:              "test_fail",
		},
		{
			KeyTimestamp:                ts2,
			KeyBundleCommandFingerprint: "fp1",
			KeyTestOutcome:              "pass",
		},
	}
	s := BuildTestBundleConvergenceSnapshot(d, lines)
	if s.DeltaAssessment != "neutral" {
		t.Fatalf("delta: got %q want neutral (later pass supersedes earlier fail for same fingerprint)", s.DeltaAssessment)
	}
	if len(s.FailingFingerprintsNow) != 0 {
		t.Fatalf("expected no failing fp, got %+v", s.FailingFingerprintsNow)
	}
	if s.HadFailureInWindow {
		t.Fatal("expected had_failure_in_window false after recovery for same fingerprint")
	}
	if !s.ReadyForSessionCompletion {
		t.Fatalf("expected ready for session completion, blocked: %v", s.SessionCompletionBlockedReasons)
	}
}

func TestBuildTestBundleConvergenceSnapshot_Neutral(t *testing.T) {
	t.Parallel()
	d := t.TempDir()
	ts := zqktime.NowRFC3339UTC()
	lines := []map[string]any{
		{
			KeyTimestamp:                ts,
			KeyBundleCommandFingerprint: "fp1",
			KeyTestOutcome:              "pass",
		},
	}
	s := BuildTestBundleConvergenceSnapshot(d, lines)
	if s.DeltaAssessment != "neutral" {
		t.Fatalf("delta: got %q", s.DeltaAssessment)
	}
	if s.HadFailureInWindow {
		t.Fatal("unexpected had_failure_in_window")
	}
	if !s.ReadyForSessionCompletion {
		t.Fatalf("expected ready for session completion, blocked: %v", s.SessionCompletionBlockedReasons)
	}
	if s.SessionCompletionNote == emptyValue {
		t.Fatal("expected session completion note when ready")
	}
	if s.PrimaryMeasurementOutcome != string(convergerollup.MeasurementYieldsConvergence) {
		t.Fatalf("primary_measurement_outcome: got %q want convergence", s.PrimaryMeasurementOutcome)
	}
	if m := BuildAfterStateSnapshotMap(s); m[objects.FieldKeyPrimaryMeasurementOutcome] == nil {
		t.Fatal("after_state_snapshot should include primary_measurement_outcome")
	}
}

func TestBuildTestBundleConvergenceSnapshot_QuarantinedFlake_IsNotGreen(t *testing.T) {
	t.Parallel()
	d := t.TempDir()
	ts := zqktime.NowRFC3339UTC()
	lines := []map[string]any{
		{
			KeyTimestamp:                ts,
			KeyBundleCommandFingerprint: "fp_flake",
			KeyTestOutcome:              "quarantined_flake",
		},
		{
			KeyTimestamp:                ts,
			KeyBundleCommandFingerprint: "fp_pass",
			KeyTestOutcome:              "pass",
		},
	}
	s := BuildTestBundleConvergenceSnapshot(d, lines)
	if s.DeltaAssessment == "neutral" {
		t.Fatalf("delta: got %q want trending_away (quarantined flake cannot be neutral)", s.DeltaAssessment)
	}
	if s.ReadyForSessionCompletion {
		t.Fatal("expected ReadyForSessionCompletion to be false when flake present")
	}
	if len(s.FlakingFingerprintsNow) != 1 || s.FlakingFingerprintsNow[0] != "fp_flake" {
		t.Fatalf("flaking: %+v want [fp_flake]", s.FlakingFingerprintsNow)
	}
	if s.PassCount != 1 || s.FlakeCount != 1 || s.FailCount != 0 {
		t.Fatalf("counts: pass=%d flake=%d fail=%d want 1/1/0", s.PassCount, s.FlakeCount, s.FailCount)
	}
	var foundFlakeReason bool
	for _, r := range s.SessionCompletionBlockedReasons {
		if strings.Contains(r, "flake") {
			foundFlakeReason = true
			break
		}
	}
	if !foundFlakeReason {
		t.Fatalf("expected blocked reasons to mention flake, got: %v", s.SessionCompletionBlockedReasons)
	}
}
