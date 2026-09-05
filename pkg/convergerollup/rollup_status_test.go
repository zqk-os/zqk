package convergerollup

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestComputeRollupStatus_satisfied(t *testing.T) {
	st, blockers, ready, _ := ComputeRollupStatus(
		TestBundleInput{
			ReadyForSessionCompletion: true,
			FailingFingerprintsNow:    nil,
			DeltaAssessment:           "neutral",
		},
		0, 0, intPtr(0), "", nil,
	)
	if st != RollupStatusSatisfied {
		t.Fatalf("status = %q want satisfied", st)
	}
	if len(blockers) != 0 {
		t.Fatalf("blockers = %#v want empty", blockers)
	}
	if !ready {
		t.Fatal("readyForParentCompletion want true")
	}
}

func TestComputeRollupStatus_blockedFingerprints(t *testing.T) {
	st, blockers, _, _ := ComputeRollupStatus(
		TestBundleInput{
			ReadyForSessionCompletion: false,
			FailingFingerprintsNow:    []string{"abc"},
			DeltaAssessment:           "neutral",
		},
		0, 0, intPtr(0), "", nil,
	)
	if st != RollupStatusBlocked {
		t.Fatalf("status = %q want blocked", st)
	}
	found := false
	for _, b := range blockers {
		if b.Code == "test_bundles_failing" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("blockers = %#v", blockers)
	}
}

func TestComputeRollupStatus_readyForReviewMatrix(t *testing.T) {
	st, blockers, ready, _ := ComputeRollupStatus(
		TestBundleInput{
			ReadyForSessionCompletion: true,
			FailingFingerprintsNow:    nil,
			DeltaAssessment:           "neutral",
		},
		0, 0, intPtr(3), "", nil,
	)
	if st != RollupStatusReadyForReview {
		t.Fatalf("status = %q want ready_for_review", st)
	}
	if ready {
		t.Fatal("readyForParentCompletion want false")
	}
	var matrixDetail string
	for _, b := range blockers {
		if b.Code == "matrix_rows_pending" {
			matrixDetail = b.Detail
			break
		}
	}
	if matrixDetail == "" {
		t.Fatalf("blockers = %#v want matrix_rows_pending", blockers)
	}
	want := "3 human-scope .go row(s) not fully done (gate columns)"
	if matrixDetail != want {
		t.Fatalf("matrix_rows_pending detail = %q want %q", matrixDetail, want)
	}
}

func TestComputeRollupStatus_readyForReviewChild(t *testing.T) {
	st, _, ready, _ := ComputeRollupStatus(
		TestBundleInput{
			ReadyForSessionCompletion: true,
			FailingFingerprintsNow:    nil,
			DeltaAssessment:           "neutral",
		},
		0, 0, intPtr(0), "",
		[]ChildSessionInput{
			{ID: "CVS-child", Status: objects.ObjectStatusActive, Blockers: []string{"child_status_active"}},
		},
	)
	if st != RollupStatusReadyForReview {
		t.Fatalf("status = %q want ready_for_review", st)
	}
	if ready {
		t.Fatal("readyForParentCompletion want false")
	}
}

func TestComputeRollupStatus_partialBundleGate(t *testing.T) {
	st, blockers, _, _ := ComputeRollupStatus(
		TestBundleInput{
			ReadyForSessionCompletion: false,
			FailingFingerprintsNow:    nil,
			DeltaAssessment:           "neutral",
		},
		0, 0, intPtr(0), "", nil,
	)
	if st != RollupStatusPartial {
		t.Fatalf("status = %q want partial", st)
	}
	found := false
	for _, b := range blockers {
		if b.Code == "bundle_gate_not_ready" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("blockers = %#v", blockers)
	}
}

func TestComputeRollupStatus_blockedGate(t *testing.T) {
	st, _, _, _ := ComputeRollupStatus(
		TestBundleInput{
			ReadyForSessionCompletion: true,
			FailingFingerprintsNow:    nil,
			DeltaAssessment:           "neutral",
		},
		1, 0, intPtr(0), "", nil,
	)
	if st != RollupStatusBlocked {
		t.Fatalf("status = %q want blocked", st)
	}
}

func intPtr(n int) *int { return &n }
