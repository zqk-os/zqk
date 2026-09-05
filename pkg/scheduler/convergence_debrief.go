package scheduler

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
)

// BuildPredictionDebrief compares persisted convergence_session.predictions to the current health
// snapshot. Intended for post-iteration review and for predictions.retrospective at finalize.
func BuildPredictionDebrief(snapshotPredictions map[string]any, snap *TestBundleConvergenceSnapshot, operatorNotes string) map[string]any {
	if snap == nil {
		return nil
	}
	out := map[string]any{
		"measurement_watermark_rfc3339":           snap.HealthWatermarkRFC3339,
		"delta_assessment_actual":                 snap.DeltaAssessment,
		"had_failure_in_window":                   snap.HadFailureInWindow,
		"failing_fingerprints_count":              len(snap.FailingFingerprintsNow),
		objects.FieldKeyReadyForSessionCompletion: snap.ReadyForSessionCompletion,
	}
	if snap.Heartbeat != nil {
		out["heartbeat_stale"] = snap.Heartbeat.Stale
	}
	var expectedSignal, hypothesisConfidence string
	if snapshotPredictions != nil {
		expectedSignal = FieldAsString(snapshotPredictions["expected_signal"])
		hypothesisConfidence = FieldAsString(snapshotPredictions["hypothesis_confidence"])
		out["expected_signal_stated"] = expectedSignal
		out["hypothesis_confidence_stated"] = hypothesisConfidence
	}
	align := assessSignalAlignment(expectedSignal, snap)
	out["signal_alignment"] = align
	out["confidence_vs_outcome"] = assessConfidenceVsOutcome(hypothesisConfidence, snap, align)
	if strings.TrimSpace(operatorNotes) != emptyValue {
		out["operator_notes"] = strings.TrimSpace(operatorNotes)
	}
	out["debrief_summary"] = buildDebriefSummaryText(out, expectedSignal, snap, operatorNotes)
	return out
}

func assessSignalAlignment(expectedSignal string, snap *TestBundleConvergenceSnapshot) string {
	exp := strings.TrimSpace(strings.ToLower(expectedSignal))
	failN := len(snap.FailingFingerprintsNow)
	if exp == emptyValue {
		return "unknown"
	}
	switch {
	case strings.Contains(exp, "decreasing") && strings.Contains(exp, "fingerprint"):
		if failN == 0 && !snap.HadFailureInWindow {
			return "matched"
		}
		if failN == 0 && snap.HadFailureInWindow {
			return "partial"
		}
		if failN > 0 {
			return "mismatched"
		}
		return "partial"
	default:
		if failN == 0 && snap.ReadyForSessionCompletion {
			return "matched"
		}
		if failN > 0 {
			return "mismatched"
		}
		return "partial"
	}
}

func assessConfidenceVsOutcome(confidence string, snap *TestBundleConvergenceSnapshot, alignment string) string {
	c := strings.TrimSpace(strings.ToLower(confidence))
	if c == emptyValue {
		return "not_stated"
	}
	switch alignment {
	case "matched":
		return "consistent_with_" + c
	case "partial":
		return "mixed_vs_" + c
	case "mismatched":
		return "underdelivered_vs_" + c
	default:
		return "unknown_vs_" + c
	}
}

func buildDebriefSummaryText(debrief map[string]any, expectedSignal string, snap *TestBundleConvergenceSnapshot, operatorNotes string) string {
	var b strings.Builder
	b.WriteString("Convergence debrief (predictions vs measurement).\n")
	if strings.TrimSpace(expectedSignal) != emptyValue {
		fmt.Fprintf(&b, "Stated expected_signal: %s\n", strings.TrimSpace(expectedSignal))
	} else {
		b.WriteString("No expected_signal on session.predictions; alignment is heuristic-only.\n")
	}
	fmt.Fprintf(&b, "Measured delta_assessment: %s; failing fingerprints now: %d; ready_for_session_completion: %v.\n",
		snap.DeltaAssessment, len(snap.FailingFingerprintsNow), snap.ReadyForSessionCompletion)
	if v, ok := debrief["signal_alignment"].(string); ok {
		fmt.Fprintf(&b, "Signal alignment: %s.\n", v)
	}
	if v, ok := debrief["confidence_vs_outcome"].(string); ok {
		fmt.Fprintf(&b, "Confidence vs outcome: %s.\n", v)
	}
	if strings.TrimSpace(operatorNotes) != emptyValue {
		b.WriteString("\nOperator notes (for next session / handoff):\n")
		b.WriteString(strings.TrimSpace(operatorNotes))
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

// MergePredictionsWithRetrospective deep-copies session predictions and sets predictions.retrospective
// for persistence at finalize (zqk object update).
func MergePredictionsWithRetrospective(snapshotPredictions map[string]any, snap *TestBundleConvergenceSnapshot, operatorNotes string) map[string]any {
	retro := BuildPredictionDebrief(snapshotPredictions, snap, operatorNotes)
	if retro == nil {
		retro = map[string]any{}
	}
	out := cloneMapJSON(snapshotPredictions)
	if out == nil {
		out = map[string]any{}
	}
	out["retrospective"] = retro
	return out
}

func cloneMapJSON(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}
