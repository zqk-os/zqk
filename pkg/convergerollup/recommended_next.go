package convergerollup

import (
	"fmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"strings"
)

// BuildRecommendedNextAction turns rollup_status and blockers into a single actionable line
// for agent prompts and overseer output (measurement-disambiguated next step).
func BuildRecommendedNextAction(status RollupStatus, blockers []Blocker, tb TestBundleInput, readyBundles bool) string {
	if len(blockers) == 0 && status == RollupStatusSatisfied && readyBundles {
		return "All configured rollup surfaces in this command's scope are green (bundles, literal gates, linked child CVS rows). " + paths.RewriteCanonicalCLIInvocations("Advance phase or complete the session via zqk object update when lifecycle rules and desired_end_state allow.")
	}

	priority := []string{
		"test_bundles_failing",
		"delta_trending_away",
		"field_key_literals_gate",
		"zqk_env_literals_gate",
		"vetting_matrix",
		"matrix_rows_pending",
		"child_session",
		"bundle_gate_not_ready",
	}
	for _, code := range priority {
		for _, b := range blockers {
			if b.Code == code {
				return actionLineForBlocker(code, b.Detail, tb)
			}
		}
	}

	if status == RollupStatusReadyForReview {
		return "Bundles and literal gates are green; resolve matrix backlog and/or complete child convergence_session rows " +
			"(related_object_refs), or document explicit acceptance. See docs/architecture/CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md."
	}
	if !readyBundles {
		return paths.RewriteCanonicalCLIInvocations("Wait for test_case evidence to satisfy ready_for_session_completion (zqk test run, scheduler activity); ") + paths.RewriteCanonicalCLIInvocations("then re-measure with zqk scheduler convergence measure --format json --session-id <CVS>.")
	}
	return "Address rollup blockers above; re-run convergence measure and literal gates. " +
		"See docs/architecture/CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md."
}

func actionLineForBlocker(code, detail string, tb TestBundleInput) string {
	switch code {
	case "test_bundles_failing":
		n := len(tb.FailingFingerprintsNow)
		if n == 0 {
			n = 1
		}
		return fmt.Sprintf(paths.RewriteCanonicalCLIInvocations("Fix failing test_case objects (%d fingerprint(s) in latest measurement): run zqk test run ")+
			"and inspect .zqk/logs/scheduler/cvs/test-bundles/; re-run measure when green.", n,
		)
	case "delta_trending_away":
		return "Bundle health is trending away from the session goal; stabilize tests and scheduler load before advancing phase. " + paths.RewriteCanonicalCLIInvocations("Re-run zqk test run for the test_case objects implicated by failing fingerprints.")
	case "field_key_literals_gate":
		return "Field-key literal gate failed (" + detail + "). Inspect field key literals and use pkg/objects accessors."
	case "zqk_env_literals_gate":
		return "ZQK env literal gate failed (" + detail + "). Use pkg/zqkenv accessors."
	case "vetting_matrix":
		return "Vetting matrix error: " + detail + ". Fix CSV/process inputs or re-run matrix verification."
	case "matrix_rows_pending":
		return "Verification matrix still has pending rows (" + detail + "). Drive matrix to completion or adjust session scope."
	case "child_session":
		if strings.HasPrefix(detail, "CVS-") {
			return "Complete or archive child convergence_session before parent exit: " + detail + "."
		}
		return "Child convergence_session under related_object_refs needs attention: " + detail + "."
	case "bundle_gate_not_ready":
		return "ready_for_session_completion is false; finish in-flight bundles and clear heartbeat/trigger-queue blockers, then re-measure."
	default:
		return fmt.Sprintf("Resolve blocker %s: %s", code, detail)
	}
}
