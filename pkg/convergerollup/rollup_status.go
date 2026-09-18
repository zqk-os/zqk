package convergerollup

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

// RollupStatus is surfaces.rollup_v1.rollup_status (Appendix A).
type RollupStatus string

const (
	RollupStatusBlocked        RollupStatus = "blocked"
	RollupStatusPartial        RollupStatus = "partial"
	RollupStatusReadyForReview RollupStatus = "ready_for_review"
	RollupStatusSatisfied      RollupStatus = "satisfied"
)

// Blocker is one machine-readable rollup blocker.
type Blocker struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

// TestBundleInput is the test-bundle slice of surfaces used for status computation.
type TestBundleInput struct {
	ReadyForSessionCompletion bool
	FailingFingerprintsNow    []string
	DeltaAssessment           string
}

// ChildSessionInput is one child CVS row from related_object_refs resolution.
type ChildSessionInput struct {
	ID       string
	Status   string
	Blockers []string
}

// ComputeRollupStatus derives rollup_status, blockers, and ready_for_parent_completion
// from measured surfaces. MatrixPending nil means “not evaluated” (no matrix_rows_pending blocker).
// MatrixErr non-empty marks a hard failure (vetting matrix unreadable).
func ComputeRollupStatus(
	tb TestBundleInput,
	fieldKeyGateExit, zqkEnvGateExit int,
	matrixPending *int,
	matrixErr string,
	children []ChildSessionInput,
) (status RollupStatus, blockers []Blocker, readyForParentCompletion bool, recommendedNext string) {
	readyBundles := tb.ReadyForSessionCompletion
	delta := strings.TrimSpace(tb.DeltaAssessment)
	failing := tb.FailingFingerprintsNow
	if failing == nil {
		failing = []string{}
	}
	hard := false

	if len(failing) > 0 {
		hard = true
		blockers = append(blockers, Blocker{
			Code:   "test_bundles_failing",
			Detail: fmt.Sprintf("%d fingerprint(s) latest bad", len(failing)),
		})
	}
	if delta == "trending_away" {
		hard = true
		blockers = append(blockers, Blocker{
			Code:   "delta_trending_away",
			Detail: "bundle health trending away",
		})
	}
	if fieldKeyGateExit != 0 {
		hard = true
		blockers = append(blockers, Blocker{
			Code:   "field_key_literals_gate",
			Detail: fmt.Sprintf("exit %d", fieldKeyGateExit),
		})
	}
	if zqkEnvGateExit != 0 {
		hard = true
		blockers = append(blockers, Blocker{
			Code:   "zqk_env_literals_gate",
			Detail: fmt.Sprintf("exit %d", zqkEnvGateExit),
		})
	}
	if matrixErr != "" {
		hard = true
		blockers = append(blockers, Blocker{Code: "vetting_matrix", Detail: matrixErr})
	} else if matrixPending != nil && *matrixPending > 0 {
		blockers = append(blockers, Blocker{
			Code:   "matrix_rows_pending",
			Detail: fmt.Sprintf("%d human-scope .go row(s) not fully done (gate columns)", *matrixPending),
		})
	}

	for i := range children {
		ch := &children[i]
		if ch.Status == objects.ObjectStatusError {
			hard = true
		}
		for _, b := range ch.Blockers {
			blockers = append(blockers, Blocker{
				Code:   "child_session",
				Detail: fmt.Sprintf("%s: %s", ch.ID, b),
			})
			if ch.Status == objects.ObjectStatusError || strings.Contains(b, "not_convergence_session") {
				hard = true
			}
		}
	}

	if hard {
		status = RollupStatusBlocked
	} else if !readyBundles {
		if len(blockers) == 0 {
			blockers = append(blockers, Blocker{
				Code:   "bundle_gate_not_ready",
				Detail: "ready_for_session_completion is false (see surfaces.test_bundles)",
			})
		}
		status = RollupStatusPartial
	} else if len(blockers) > 0 {
		onlySoft := true
		for _, b := range blockers {
			if b.Code != "matrix_rows_pending" && b.Code != "child_session" {
				onlySoft = false
				break
			}
		}
		if onlySoft {
			status = RollupStatusReadyForReview
		} else {
			status = RollupStatusPartial
		}
	} else {
		status = RollupStatusSatisfied
	}

	readyForParentCompletion = status == RollupStatusSatisfied && readyBundles

	recommendedNext = BuildRecommendedNextAction(status, blockers, tb, readyBundles)

	return status, blockers, readyForParentCompletion, recommendedNext
}
