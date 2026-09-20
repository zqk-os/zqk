package scheduler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/objects"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

func TestBuildPredictionDebrief_DecreasingFingerprintMatched(t *testing.T) {
	pred := map[string]any{
		"expected_signal":       "decreasing_failing_fingerprint_count",
		"hypothesis_confidence": "medium",
	}
	snap := &schedpkg.TestBundleConvergenceSnapshot{
		DeltaAssessment:           "neutral",
		HadFailureInWindow:        false,
		FailingFingerprintsNow:    nil,
		ReadyForSessionCompletion: true,
		HealthWatermarkRFC3339:    "2025-03-21T12:00:00Z",
	}
	d := schedpkg.BuildPredictionDebrief(pred, snap, "")
	if d["signal_alignment"] != "matched" {
		t.Fatalf("signal_alignment: %v", d["signal_alignment"])
	}
	s := d["debrief_summary"].(string)
	if !strings.Contains(s, "neutral") {
		t.Fatalf("summary: %v", s)
	}
}

func TestBuildPredictionDebrief_Mismatched(t *testing.T) {
	pred := map[string]any{"expected_signal": "decreasing_failing_fingerprint_count"}
	snap := &schedpkg.TestBundleConvergenceSnapshot{
		DeltaAssessment:        "trending_away",
		FailingFingerprintsNow: []string{"abc"},
		HadFailureInWindow:     true,
		HealthWatermarkRFC3339: "2025-03-21T12:00:00Z",
	}
	d := schedpkg.BuildPredictionDebrief(pred, snap, "")
	if d["signal_alignment"] != "mismatched" {
		t.Fatalf("signal_alignment: %v", d["signal_alignment"])
	}
}

func TestMarshalConvergence_FinalizeDebriefIncludesPredictions(t *testing.T) {
	snap := &schedpkg.TestBundleConvergenceSnapshot{
		DeltaAssessment:           "neutral",
		HealthWatermarkRFC3339:    "2025-03-21T12:00:00Z",
		LinesInWindow:             5,
		ReadyForSessionCompletion: true,
		FingerprintLatestOutcome: map[string]string{
			"fp1": "pass",
		},
	}
	meta := map[string]any{"read_attempted": false}
	predictions := map[string]any{
		"expected_signal":       "decreasing_failing_fingerprint_count",
		"hypothesis_confidence": "medium",
	}
	b, err := marshalConvergenceOutputJSON(snap, "CVS-z", "", "", meta, nil, false, predictions, true, "next: prefer short bundles first", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]any
	if err := json.Unmarshal(b, &top); err != nil {
		t.Fatal(err)
	}
	sug, _ := top["suggested_convergence_session_fields"].(map[string]any)
	ou, _ := sug["object_update_body"].(map[string]any)
	p, ok := ou[objects.FieldKeyPredictions].(map[string]any)
	if !ok {
		t.Fatalf("expected predictions in object_update_body: %#v", ou)
	}
	rRaw, ok := p["retrospective"]
	if !ok {
		t.Fatalf("expected predictions.retrospective: %#v", p)
	}
	rRaw, ok = nildecode.DecodeNonNilPayload[any](rRaw)
	if !ok {
		t.Fatalf("expected predictions.retrospective: %#v", p)
	}
	r, ok := rRaw.(map[string]any)
	if !ok || r["signal_alignment"] == nil {
		t.Fatalf("expected predictions.retrospective: %#v", p)
	}
	if ou[objects.FieldKeyDebriefNotes] != "next: prefer short bundles first" {
		t.Fatalf("debrief_notes: %v", ou[objects.FieldKeyDebriefNotes])
	}
}
