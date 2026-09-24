package scheduler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/objects"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

func TestMarshalConvergenceOutputJSON_WithoutSessionID(t *testing.T) {
	snap := &schedpkg.TestBundleConvergenceSnapshot{
		DeltaAssessment: "neutral",
		LinesInWindow:   3,
	}
	b, err := marshalConvergenceOutputJSON(snap, "", "", "", nil, nil, false, nil, false, "", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["convergence_session_id"]; ok {
		t.Fatalf("expected no convergence_session_id, got %v", m["convergence_session_id"])
	}
	if _, ok := m["suggested_convergence_session_fields"]; ok {
		t.Fatal("expected no suggested_convergence_session_fields")
	}
	if _, ok := m["rollup_status_core"]; ok {
		t.Fatal("expected no rollup_status_core when rollupCore nil")
	}
	if m[objects.FieldKeyDeltaAssessment] != "neutral" {
		t.Fatalf("delta_assessment: got %v", m[objects.FieldKeyDeltaAssessment])
	}
}

func TestMarshalConvergenceOutputJSON_IncludesRollupStatusCoreWhenProvided(t *testing.T) {
	snap := &schedpkg.TestBundleConvergenceSnapshot{DeltaAssessment: "neutral"}
	rollup := map[string]any{
		"rollup_status":               "satisfied",
		objects.FieldKeyBlockers:      []any{},
		"ready_for_parent_completion": true,
		"recommended_next_action":     "ok",
	}
	b, err := marshalConvergenceOutputJSON(snap, "", "", "", nil, nil, false, nil, false, "", nil, nil, rollup)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	rscRaw, ok := m["rollup_status_core"]
	if !ok {
		t.Fatalf("rollup_status_core: %#v", m["rollup_status_core"])
	}
	rscRaw, ok = nildecode.DecodeNonNilPayload[any](rscRaw)
	if !ok {
		t.Fatalf("rollup_status_core: %#v", m["rollup_status_core"])
	}
	rsc, ok := rscRaw.(map[string]any)
	if !ok || rsc["rollup_status"] != "satisfied" {
		t.Fatalf("rollup_status_core: %#v", m["rollup_status_core"])
	}
}

func TestMarshalConvergenceOutputJSON_WithSessionID(t *testing.T) {
	snap := &schedpkg.TestBundleConvergenceSnapshot{
		HealthWatermarkRFC3339: "2025-03-21T12:00:00Z",
		LinesInWindow:          10,
		DeltaAssessment:        "trending_toward",
		NextActionHint:         "wait for bundles",
		SuggestedRerunByFingerprint: map[string][]string{
			"fp-b": {"go test ./b"},
			"fp-a": {"go test ./a"},
		},
	}
	meta := map[string]any{
		"convergence_session_id":  "CVS-test-1",
		"read_attempted":          false,
		"effective_current_phase": "",
		"effective_flow_variant":  "",
		"current_phase_source":    "empty",
		"flow_variant_source":     "empty",
	}
	b, err := marshalConvergenceOutputJSON(snap, "CVS-test-1", "", "", meta, nil, false, nil, false, "", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["convergence_session_id"] != "CVS-test-1" {
		t.Fatalf("convergence_session_id: got %v", m["convergence_session_id"])
	}
	sugRaw, ok := m["suggested_convergence_session_fields"]
	if !ok {
		t.Fatalf("suggested_convergence_session_fields: %v", m["suggested_convergence_session_fields"])
	}
	sugRaw, ok = nildecode.DecodeNonNilPayload[any](sugRaw)
	if !ok {
		t.Fatalf("suggested_convergence_session_fields: %v", m["suggested_convergence_session_fields"])
	}
	sug, ok := sugRaw.(map[string]any)
	if !ok {
		t.Fatalf("suggested_convergence_session_fields: %v", m["suggested_convergence_session_fields"])
	}
	if sug[objects.FieldKeyDeltaAssessment] != "trending_toward" {
		t.Fatalf("suggested delta_assessment: %v", sug[objects.FieldKeyDeltaAssessment])
	}
	if sug[objects.FieldKeyLastMeasurementAt] != "2025-03-21T12:00:00Z" {
		t.Fatalf("suggested last_measurement_at: %v", sug[objects.FieldKeyLastMeasurementAt])
	}
	next, _ := sug[objects.FieldKeyNextAction].(string)
	if next == emptyValue {
		t.Fatal("expected non-empty next_action")
	}
	// buildNextActionText lists fp-a before fp-b (sorted)
	if idxA, idxB := indexOf(next, "fp-a"), indexOf(next, "fp-b"); idxA < 0 || idxB < 0 || idxA >= idxB {
		t.Fatalf("next_action should list fp-a before fp-b:\n%s", next)
	}
	prRaw, ok := sug["phase_router"]
	if !ok {
		t.Fatalf("expected phase_router with c5_verify, got %#v", sug["phase_router"])
	}
	prRaw, ok = nildecode.DecodeNonNilPayload[any](prRaw)
	if !ok {
		t.Fatalf("expected phase_router with c5_verify, got %#v", sug["phase_router"])
	}
	pr, ok := prRaw.(map[string]any)
	if !ok || pr["measurement_implied_phase"] != "c5_verify" {
		t.Fatalf("expected phase_router with c5_verify, got %#v", sug["phase_router"])
	}
	if sug[objects.FieldKeyCurrentPhase] != "c5_verify" {
		t.Fatalf("current_phase: %v", sug[objects.FieldKeyCurrentPhase])
	}
	ou, ok := sug["object_update_body"].(map[string]any)
	if !ok {
		t.Fatalf("object_update_body: %T", sug["object_update_body"])
	}
	for _, k := range []string{
		objects.FieldKeyDeltaAssessment,
		objects.FieldKeyLastMeasurementAt,
		objects.FieldKeyAfterStateSnapshot,
		objects.FieldKeyNextAction,
		objects.FieldKeyCurrentPhase,
		objects.FieldKeyActivityLog,
	} {
		if _, ok := ou[k]; !ok {
			t.Fatalf("object_update_body missing %q", k)
		}
	}
	al, ok := ou[objects.FieldKeyActivityLog].([]any)
	if !ok || len(al) != 1 {
		t.Fatalf("object_update_body activity_log: want one entry, got %#v", ou[objects.FieldKeyActivityLog])
	}
	ent, ok := al[0].(map[string]any)
	if !ok || ent["action"] != "measure_test_bundle_health" {
		t.Fatalf("activity_log entry: %#v", al[0])
	}
	td, ok := sug["tombstone_disparity"].(map[string]any)
	if !ok || td["active_tombstone"] != false {
		t.Fatalf("expected tombstone_disparity with no active tombstone: %#v", sug["tombstone_disparity"])
	}
	afterRaw, ok := ou[objects.FieldKeyAfterStateSnapshot]
	if !ok {
		t.Fatalf("after_state_snapshot should include scope_fingerprint_sha256: %#v", ou[objects.FieldKeyAfterStateSnapshot])
	}
	afterRaw, ok = nildecode.DecodeNonNilPayload[any](afterRaw)
	if !ok {
		t.Fatalf("after_state_snapshot should include scope_fingerprint_sha256: %#v", ou[objects.FieldKeyAfterStateSnapshot])
	}
	after, ok := afterRaw.(map[string]any)
	if !ok || after["scope_fingerprint_sha256"] == nil {
		t.Fatalf("after_state_snapshot should include scope_fingerprint_sha256: %#v", ou[objects.FieldKeyAfterStateSnapshot])
	}
}

func TestMarshalConvergenceOutputJSON_StampTombstone(t *testing.T) {
	snap := &schedpkg.TestBundleConvergenceSnapshot{
		DeltaAssessment:        "neutral",
		HealthWatermarkRFC3339: "2025-03-21T12:00:00Z",
		FingerprintLatestOutcome: map[string]string{
			"fp1": "pass",
		},
	}
	meta := map[string]any{"read_attempted": false}
	b, err := marshalConvergenceOutputJSON(snap, "CVS-t", "", "", meta, nil, true, nil, false, "", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]any
	if err := json.Unmarshal(b, &top); err != nil {
		t.Fatal(err)
	}
	sug, _ := top["suggested_convergence_session_fields"].(map[string]any)
	ou, ok := sug["object_update_body"].(map[string]any)
	if !ok {
		t.Fatal("missing object_update_body")
	}
	if _, ok := ou[objects.FieldKeyBeforeStateSnapshot]; !ok {
		t.Fatalf("expected before_state_snapshot when stamp tombstone: %#v", ou)
	}
}

func TestMarshalConvergenceOutputJSON_PhaseAlignment(t *testing.T) {
	snap := &schedpkg.TestBundleConvergenceSnapshot{
		DeltaAssessment:           "neutral",
		ReadyForSessionCompletion: true,
	}
	meta := map[string]any{
		"convergence_session_id":  "CVS-x",
		"read_attempted":          false,
		"effective_current_phase": "c5_verify",
		"effective_flow_variant":  "scheduler_fast",
		"current_phase_source":    "flag",
		"flow_variant_source":     "flag",
	}
	b, err := marshalConvergenceOutputJSON(snap, "CVS-x", "c5_verify", "scheduler_fast", meta, nil, false, nil, false, "", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]any
	if err := json.Unmarshal(b, &top); err != nil {
		t.Fatal(err)
	}
	sug, _ := top["suggested_convergence_session_fields"].(map[string]any)
	pr, _ := sug["phase_router"].(map[string]any)
	if pr["phase_alignment"] != "session_behind" {
		t.Fatalf("alignment: %v", pr["phase_alignment"])
	}
	ou, ok := sug["object_update_body"].(map[string]any)
	if !ok || ou[objects.FieldKeyCurrentPhase] != "c6_exit" {
		t.Fatalf("object_update_body.current_phase: %#v", sug["object_update_body"])
	}
}

func TestMarshalConvergenceOutputJSON_CompletionGateObservabilityNote(t *testing.T) {
	snap := &schedpkg.TestBundleConvergenceSnapshot{
		DeltaAssessment:           "neutral",
		ReadyForSessionCompletion: true,
	}
	meta := map[string]any{"read_attempted": false}
	th := map[string]any{
		"completion_gate": map[string]any{"require_ready_for_session_completion": false},
	}
	b, err := marshalConvergenceOutputJSON(snap, "CVS-cg", "c6_exit", "default", meta, nil, false, nil, false, "", th, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]any
	if err := json.Unmarshal(b, &top); err != nil {
		t.Fatal(err)
	}
	sug, _ := top["suggested_convergence_session_fields"].(map[string]any)
	prRaw, ok := sug["phase_router"]
	if !ok {
		t.Fatal("missing phase_router")
	}
	prRaw, ok = nildecode.DecodeNonNilPayload[any](prRaw)
	if !ok {
		t.Fatal("phase_router decode")
	}
	pr, ok := prRaw.(map[string]any)
	if !ok {
		t.Fatalf("phase_router type: %T", prRaw)
	}
	notesRaw, ok := pr[objects.FieldKeyNotes]
	if !ok {
		t.Fatal("missing notes")
	}
	notes, ok := notesRaw.([]any)
	if !ok || len(notes) < 1 {
		t.Fatalf("notes: %#v", notesRaw)
	}
	first, ok := notes[0].(string)
	if !ok || !strings.Contains(first, "Observed: thresholds.completion_gate") {
		t.Fatalf("first note: %v", notes[0])
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestFormatAgentMarkdown_LinkedBacklogSection(t *testing.T) {
	t.Parallel()
	md := formatAgentMarkdown("CVS-test", map[string]any{
		objects.FieldKeyHypothesis: "H",
	}, map[string]any{
		objects.FieldKeyNextAction: "n",
	}, &schedpkg.TestBundleConvergenceSnapshot{
		DeltaAssessment: "neutral", HealthWatermarkRFC3339: "2025-01-01T00:00:00Z",
	}, "", nil, map[string]any{"rollup_status": "satisfied"}, "default", nil,
		"## Linked backlog (acceptance criteria)\n\n- **demo**\n")
	if !strings.Contains(md, "Linked backlog (acceptance criteria)") || !strings.Contains(md, "demo") {
		t.Fatalf("expected injected backlog section in markdown:\n%s", md)
	}
}

func TestFormatAgentMarkdown(t *testing.T) {
	md := formatAgentMarkdown("CVS-test", map[string]any{
		objects.FieldKeyHypothesis:      "H1",
		objects.FieldKeyDesiredEndState: "Green bundles",
		objects.FieldKeyStatus:          objects.ObjectStatusActive,
		objects.FieldKeyFlowVariant:     "scheduler_fast",
		objects.FieldKeyPredictions:     map[string]any{"p1": "x"},
	}, map[string]any{
		objects.FieldKeyNextAction: "fix tests",
		"phase_router":             map[string]any{"measurement_implied_phase": "c4_act"},
	}, &schedpkg.TestBundleConvergenceSnapshot{
		DeltaAssessment:           "trending_away",
		HealthWatermarkRFC3339:    "2025-01-01T00:00:00Z",
		FailingFingerprintsNow:    []string{"fp1"},
		ReadyForSessionCompletion: false,
	}, "", nil, map[string]any{
		"rollup_status":               "blocked",
		"recommended_next_action":     "Run zqk test run for failing test_case objects.",
		objects.FieldKeyBlockers:      []any{map[string]any{objects.FieldKeyCode: "test_bundles_failing", "detail": "1 fingerprint(s) latest bad"}},
		"ready_for_parent_completion": false,
	}, "scheduler_fast", map[string]any{"flow_variant_source": "object", "effective_flow_variant": "scheduler_fast"}, "")
	if !strings.Contains(md, "CVS-test") || !strings.Contains(md, "trending_away") || !strings.Contains(md, "fix tests") {
		t.Fatalf("unexpected markdown:\n%s", md)
	}
	if !strings.Contains(md, "H1") || !strings.Contains(md, "Green bundles") {
		t.Fatalf("expected CVS fields in markdown:\n%s", md)
	}
	if !strings.Contains(md, "Scope: test bundles vs session contract") || !strings.Contains(md, "Bundle-health completion gate") {
		t.Fatalf("expected measurement scope + bundle-health gate labels in markdown:\n%s", md)
	}
	if !strings.Contains(md, "rollup_v1") || !strings.Contains(md, "convergence overseer") {
		t.Fatalf("expected scope to mention rollup_v1 (Python rollup) and convergence overseer:\n%s", md)
	}
	if !strings.Contains(md, "Next measured action (rollup)") || !strings.Contains(md, "rollup_status") {
		t.Fatalf("expected rollup section in markdown:\n%s", md)
	}
	if !strings.Contains(md, "## Convergence recommendation (this iteration)") || !strings.Contains(md, "Desired end state") {
		t.Fatalf("expected convergence recommendation section toward desired end state:\n%s", md)
	}
	if !strings.Contains(md, "## CLI diagnostic") || !strings.Contains(md, "Running executable") {
		t.Fatalf("expected CLI diagnostic section:\n%s", md)
	}
}

func TestFormatAgentMarkdown_CLIStaleBinaryHintOnUnknownFlowVariant(t *testing.T) {
	md := formatAgentMarkdown("CVS-test", map[string]any{
		objects.FieldKeyHypothesis:      "H",
		objects.FieldKeyDesiredEndState: "D",
		objects.FieldKeyStatus:          objects.ObjectStatusActive,
		objects.FieldKeyFlowVariant:     "product_delivery_datacell",
	}, map[string]any{
		objects.FieldKeyNextAction: "noop",
		"phase_router": map[string]any{
			"routing_profile":           "default",
			"measurement_implied_phase": "c6_exit",
			"suggested_current_phase":   "c6_exit",
			objects.FieldKeyNotes: []any{
				`Unknown flow_variant "product_delivery_datacell"; using default measurement-implied phase rules.`,
			},
		},
	}, &schedpkg.TestBundleConvergenceSnapshot{
		DeltaAssessment:           "neutral",
		HealthWatermarkRFC3339:    "2025-01-01T00:00:00Z",
		ReadyForSessionCompletion: true,
	}, "", nil, map[string]any{
		"rollup_status": "satisfied",
	}, "product_delivery_datacell", map[string]any{
		"flow_variant_source":    "object",
		"effective_flow_variant": "product_delivery_datacell",
	}, "")
	if !strings.Contains(md, "Stale binary hint") {
		t.Fatalf("expected stale binary hint when phase_router notes unknown flow_variant:\n%s", md)
	}
}

func TestFormatAgentMarkdown_convergenceRecommendationRespectsCompletionGateBypass(t *testing.T) {
	t.Parallel()
	md := formatAgentMarkdown("CVS-x", map[string]any{
		objects.FieldKeyHypothesis:      "H",
		objects.FieldKeyDesiredEndState: "D",
		objects.FieldKeyStatus:          objects.ObjectStatusActive,
		objects.FieldKeyCurrentPhase:    "c6_exit",
		objects.FieldKeyThresholds: map[string]any{
			"completion_gate": map[string]any{"require_ready_for_session_completion": false},
		},
	}, map[string]any{
		objects.FieldKeyNextAction: "n",
		"phase_router": map[string]any{
			"phase_alignment":         "session_ahead",
			"suggested_current_phase": "c1_scope",
		},
	}, &schedpkg.TestBundleConvergenceSnapshot{
		DeltaAssessment:                 "unknown",
		ReadyForSessionCompletion:       false,
		SessionCompletionBlockedReasons: []string{"no health data in window"},
	}, "", nil, map[string]any{"rollup_status": "satisfied"}, "default", nil, "")
	if strings.Contains(md, "Clear `ready_for_session_completion`") {
		t.Fatalf("should not tell agent to clear RFS when completion_gate relaxes bundle readiness:\n%s", md)
	}
	if !strings.Contains(md, "**completion_gate:**") || !strings.Contains(md, "blocking gate") {
		t.Fatalf("expected completion_gate signal in recommendation:\n%s", md)
	}
	if !strings.Contains(md, "Bundle-health (informative)") || !strings.Contains(md, "desired end state") {
		t.Fatalf("expected informative RFS line + contract focus:\n%s", md)
	}
}

func TestFormatAgentMarkdown_prefersCVSNextActionWhenSuggestedIsBundleOnly(t *testing.T) {
	md := formatAgentMarkdown("CVS-test", map[string]any{
		objects.FieldKeyHypothesis:      "H1",
		objects.FieldKeyDesiredEndState: "D",
		objects.FieldKeyStatus:          objects.ObjectStatusActive,
		objects.FieldKeyNextAction:      "Operator handoff: triage .zqk/logs/drift/ then zqk test run.",
		objects.FieldKeyCurrentPhase:    "c6_exit",
		objects.FieldKeyFlowVariant:     "code_quality_drift_and_standardization",
	}, map[string]any{
		objects.FieldKeyNextAction: "No failing outcomes in this window.",
		"phase_router": map[string]any{
			"phase_alignment":           "aligned",
			"measurement_implied_phase": "c6_exit",
			"suggested_current_phase":   "c6_exit",
			"routing_profile":           "code_quality_drift_and_standardization",
		},
	}, &schedpkg.TestBundleConvergenceSnapshot{
		DeltaAssessment:           "neutral",
		HealthWatermarkRFC3339:    "2025-01-01T00:00:00Z",
		FailingFingerprintsNow:    nil,
		ReadyForSessionCompletion: true,
		TriggerQueuePending:       0,
	}, "", nil, map[string]any{
		"rollup_status":               "satisfied",
		"ready_for_parent_completion": true,
		"recommended_next_action":     "ok",
	}, "code_quality_drift_and_standardization", map[string]any{"flow_variant_source": "object", "effective_flow_variant": "code_quality_drift_and_standardization"}, "")
	if !strings.Contains(md, "Operator handoff: triage .zqk/logs/drift/") {
		t.Fatalf("expected persisted CVS next_action in markdown:\n%s", md)
	}
	if strings.Contains(md, "No failing outcomes in this window") {
		t.Fatalf("should not show bundle-only next_action when CVS has operator handoff:\n%s", md)
	}
	if !strings.Contains(md, "Primary next work") || !strings.Contains(md, "Desired end state") {
		t.Fatalf("expected green-path convergence recommendation toward desired end state:\n%s", md)
	}
}
