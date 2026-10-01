// BLI-STARTER-COMMUNITY-013 / PRI-STARTER-COMMUNITY-013 coverage elevation
package convergerollup

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestRecommendedNext_AllBranches(t *testing.T) {
	t.Parallel()

	tb := TestBundleInput{
		FailingFingerprintsNow:    []string{"pkg/a.TestX"},
		ReadyForSessionCompletion: false,
	}

	// 1. Ready and satisfied with no blockers
	line := BuildRecommendedNextAction(RollupStatusSatisfied, nil, tb, true)
	if !strings.Contains(line, "All configured rollup surfaces in this command's scope are green") {
		t.Errorf("unexpected line: %s", line)
	}

	// 2. Priority blockers in order
	// delta_trending_away
	blockersDelta := []Blocker{{Code: "delta_trending_away", Detail: "trending"}}
	lineDelta := BuildRecommendedNextAction(RollupStatusBlocked, blockersDelta, tb, false)
	if !strings.Contains(lineDelta, "trending away from the session goal") {
		t.Errorf("unexpected line: %s", lineDelta)
	}

	// test_bundles_failing with empty failing fingerprints (defaults to 1)
	tbEmpty := TestBundleInput{}
	blockersTB := []Blocker{{Code: "test_bundles_failing", Detail: "failing"}}
	lineTB := BuildRecommendedNextAction(RollupStatusBlocked, blockersTB, tbEmpty, false)
	if !strings.Contains(lineTB, "1 fingerprint(s)") {
		t.Errorf("unexpected line: %s", lineTB)
	}

	// field_key_literals_gate
	blockersFK := []Blocker{{Code: "field_key_literals_gate", Detail: "bad key"}}
	lineFK := BuildRecommendedNextAction(RollupStatusBlocked, blockersFK, tb, false)
	if !strings.Contains(lineFK, "Field-key literal gate failed (bad key)") {
		t.Errorf("unexpected line: %s", lineFK)
	}

	// zqk_env_literals_gate
	blockersZE := []Blocker{{Code: "zqk_env_literals_gate", Detail: "bad env"}}
	lineZE := BuildRecommendedNextAction(RollupStatusBlocked, blockersZE, tb, false)
	if !strings.Contains(lineZE, "ZQK env literal gate failed (bad env)") {
		t.Errorf("unexpected line: %s", lineZE)
	}

	// vetting_matrix
	blockersVM := []Blocker{{Code: "vetting_matrix", Detail: "csv error"}}
	lineVM := BuildRecommendedNextAction(RollupStatusBlocked, blockersVM, tb, false)
	if !strings.Contains(lineVM, "Vetting matrix error: csv error") {
		t.Errorf("unexpected line: %s", lineVM)
	}

	// matrix_rows_pending
	blockersMRP := []Blocker{{Code: "matrix_rows_pending", Detail: "3 pending"}}
	lineMRP := BuildRecommendedNextAction(RollupStatusBlocked, blockersMRP, tb, false)
	if !strings.Contains(lineMRP, "Verification matrix still has pending rows (3 pending)") {
		t.Errorf("unexpected line: %s", lineMRP)
	}

	// child_session with CVS- prefix
	blockersChildCVS := []Blocker{{Code: "child_session", Detail: "CVS-123 active"}}
	lineChildCVS := BuildRecommendedNextAction(RollupStatusBlocked, blockersChildCVS, tb, false)
	if !strings.Contains(lineChildCVS, "Complete or archive child convergence_session before parent exit: CVS-123 active") {
		t.Errorf("unexpected line: %s", lineChildCVS)
	}

	// child_session without CVS- prefix
	blockersChildOther := []Blocker{{Code: "child_session", Detail: "other issue"}}
	lineChildOther := BuildRecommendedNextAction(RollupStatusBlocked, blockersChildOther, tb, false)
	if !strings.Contains(lineChildOther, "Child convergence_session under related_object_refs needs attention: other issue") {
		t.Errorf("unexpected line: %s", lineChildOther)
	}

	// bundle_gate_not_ready
	blockersBG := []Blocker{{Code: "bundle_gate_not_ready", Detail: "not ready"}}
	lineBG := BuildRecommendedNextAction(RollupStatusBlocked, blockersBG, tb, false)
	if !strings.Contains(lineBG, "ready_for_session_completion is false") {
		t.Errorf("unexpected line: %s", lineBG)
	}

	// default unknown blocker
	lineCustom := actionLineForBlocker("custom_code", "custom detail", tb)
	if !strings.Contains(lineCustom, "Resolve blocker custom_code: custom detail") {
		t.Errorf("unexpected line: %s", lineCustom)
	}

	// 3. Fallbacks when no priority blockers match
	// status == RollupStatusReadyForReview
	lineRFR := BuildRecommendedNextAction(RollupStatusReadyForReview, nil, tb, true)
	if !strings.Contains(lineRFR, "Bundles and literal gates are green; resolve matrix backlog") {
		t.Errorf("unexpected line: %s", lineRFR)
	}

	// !readyBundles fallback
	lineNotReady := BuildRecommendedNextAction(RollupStatusPartial, nil, tb, false)
	if !strings.Contains(lineNotReady, "Wait for bundle health to satisfy ready_for_session_completion") {
		t.Errorf("unexpected line: %s", lineNotReady)
	}

	// Default general fallback
	lineFallback := BuildRecommendedNextAction(RollupStatusPartial, nil, tb, true)
	if !strings.Contains(lineFallback, "Address rollup blockers above; re-run convergence measure") {
		t.Errorf("unexpected line: %s", lineFallback)
	}
}

func TestNest_EdgeCases(t *testing.T) {
	t.Parallel()

	// ValidateNestLink errors:
	// empty parent / child
	if err := ValidateNestLink("", "", "", 0, nil); err == nil {
		t.Errorf("expected error on empty parent/child")
	}
	// non-CVS prefixes
	if err := ValidateNestLink("TASK-1", "TASK-1", "TASK-2", 0, nil); err == nil {
		t.Errorf("expected error on non-CVS id")
	}
	// parent == child
	if err := ValidateNestLink("CVS-1", "CVS-1", "CVS-1", 0, nil); err == nil {
		t.Errorf("expected error on self-nesting")
	}

	// parent not found under coordinator
	nodeFor := func(id string) ([]string, string, string, error) {
		return nil, "active", "", nil
	}
	if err := ValidateNestLink("CVS-root", "CVS-missing", "CVS-child", 3, nodeFor); err == nil {
		t.Errorf("expected error when parent not under coordinator")
	}

	// NestStatus empty parentID
	if _, err := NestStatus("", 0, nodeFor); err == nil {
		t.Errorf("expected error on empty parentID")
	}

	// AppendRelatedObjectRef empty id and existing id
	refs := []string{"CVS-1", "CVS-2"}
	if len(AppendRelatedObjectRef(refs, "")) != 2 {
		t.Errorf("empty id should return unchanged refs")
	}
	if len(AppendRelatedObjectRef(refs, "CVS-1")) != 2 {
		t.Errorf("existing id should return unchanged refs")
	}

	// RelatedObjectRefsFromMap edge cases
	if refs := RelatedObjectRefsFromMap(nil); refs != nil {
		t.Errorf("expected nil on nil map")
	}
	if refs := RelatedObjectRefsFromMap(map[string]any{}); refs != nil {
		t.Errorf("expected nil on empty map")
	}
	if refs := RelatedObjectRefsFromMap(map[string]any{objects.FieldKeyRelatedObjectRefs: 123}); refs != nil {
		t.Errorf("expected nil on non-slice")
	}
	// []string slice
	strRefs := RelatedObjectRefsFromMap(map[string]any{objects.FieldKeyRelatedObjectRefs: []string{"CVS-a", "CVS-b"}})
	if len(strRefs) != 2 || strRefs[0] != "CVS-a" {
		t.Errorf("expected 2 string refs")
	}

	// ChildSessionFields with NextAction and Predictions
	req := NestSpawnRequest{
		ParentID:        "CVS-p",
		Title:           "custom title",
		CurrentPhase:    "c2_triage",
		NextAction:      "run test",
		Predictions:     map[string]any{"conf": 0.9},
		Hypothesis:      "hyp",
		DesiredEndState: "end",
	}
	fields := ChildSessionFields(req)
	if fields[objects.FieldKeyNextAction] != "run test" {
		t.Errorf("NextAction not set")
	}
	if fields[objects.FieldKeyCurrentPhase] != "c2_triage" {
		t.Errorf("CurrentPhase not set")
	}
}

func TestOverseer_BuildArbitratedParentMessage(t *testing.T) {
	t.Parallel()

	tree := []CVSTreeNode{
		{ID: "CVS-root", Depth: 0, Status: objects.ObjectStatusActive},
		{ID: "CVS-child1", Depth: 1, Status: objects.ObjectStatusActive},
	}

	// rollup nil, tree has active children
	msg := BuildArbitratedParentMessage(nil, tree, "CVS-root")
	if !strings.Contains(msg, "1 active child session(s) in tree") {
		t.Errorf("expected active child mention in: %s", msg)
	}
	if !strings.Contains(msg, "Run zqk scheduler convergence measure") {
		t.Errorf("expected measurement prompt in: %s", msg)
	}

	// rollup with recommended_next_action
	rollup := map[string]any{
		"recommended_next_action":     "Do this next",
		"ready_for_parent_completion": true,
	}
	msg2 := BuildArbitratedParentMessage(rollup, tree, "CVS-root")
	if strings.Contains(msg2, "Do not treat parent as ready") {
		t.Errorf("should not contain unready warning when ready_for_parent_completion is true: %s", msg2)
	}
	if !strings.Contains(msg2, "Measured rollup directive: Do this next") {
		t.Errorf("expected directive in: %s", msg2)
	}
}

func TestComputePrimaryMeasurementOutcome_AdditionalCases(t *testing.T) {
	t.Parallel()

	// RollupStatusBlocked with delta_trending_away
	o, d := ComputePrimaryMeasurementOutcome(RollupStatusBlocked, []Blocker{{Code: "delta_trending_away", Detail: "away"}}, "", 0, 0, false)
	if o != MeasurementYieldsDivergence || d != "bundle_health_bad_signal" {
		t.Errorf("expected divergence bad signal, got %s %s", o, d)
	}

	// RollupStatusBlocked with other blocker
	o, d = ComputePrimaryMeasurementOutcome(RollupStatusBlocked, []Blocker{{Code: "child_session", Detail: "blocked"}}, "", 0, 0, false)
	if o != MeasurementYieldsDivergence || d != "rollup_blocked" {
		t.Errorf("expected divergence rollup blocked, got %s %s", o, d)
	}

	// RollupStatusReadyForReview
	o, d = ComputePrimaryMeasurementOutcome(RollupStatusReadyForReview, nil, "", 0, 0, false)
	if o != MeasurementYieldsDivergence || d != "follow_up_required_matrix_or_child" {
		t.Errorf("expected divergence follow up, got %s %s", o, d)
	}

	// RollupStatusPartial with composite_partial
	o, d = ComputePrimaryMeasurementOutcome(RollupStatusPartial, []Blocker{{Code: "custom_code", Detail: "custom"}}, "", 0, 0, false)
	if o != MeasurementYieldsDivergence || d != "composite_partial" {
		t.Errorf("expected divergence composite partial, got %s %s", o, d)
	}

	// RollupStatusUnknown / unclassified
	o, d = ComputePrimaryMeasurementOutcome(RollupStatus("unknown"), nil, "", 0, 0, false)
	if o != MeasurementYieldsAmbiguousOutcome || d != "unclassified" {
		t.Errorf("expected ambiguous unclassified, got %s %s", o, d)
	}
}

func TestComputeRollupStatus_AdditionalCases(t *testing.T) {
	t.Parallel()

	// Child in error status
	tb := TestBundleInput{ReadyForSessionCompletion: true}
	children := []ChildSessionInput{
		{
			ID:       "CVS-err",
			Status:   objects.ObjectStatusError,
			Blockers: []string{"error occurred"},
		},
	}
	status, blockers, ready, _ := ComputeRollupStatus(tb, 0, 0, nil, "", children)
	if status != RollupStatusBlocked || ready {
		t.Errorf("expected blocked and not ready, got %s %v", status, ready)
	}
	if len(blockers) == 0 {
		t.Errorf("expected child blockers")
	}

	// Child blocker containing not_convergence_session
	children2 := []ChildSessionInput{
		{
			ID:       "CVS-other",
			Status:   objects.ObjectStatusActive,
			Blockers: []string{"not_convergence_session"},
		},
	}
	status2, _, _, _ := ComputeRollupStatus(tb, 0, 0, nil, "", children2)
	if status2 != RollupStatusBlocked {
		t.Errorf("expected blocked on not_convergence_session, got %s", status2)
	}

	// matrixPending > 0
	pending := 2
	status3, blockers3, _, _ := ComputeRollupStatus(tb, 0, 0, &pending, "", nil)
	if status3 != RollupStatusReadyForReview {
		t.Errorf("expected ReadyForReview with only soft blockers, got %s", status3)
	}
	if len(blockers3) != 1 || blockers3[0].Code != "matrix_rows_pending" {
		t.Errorf("expected matrix_rows_pending blocker, got %v", blockers3)
	}
}
