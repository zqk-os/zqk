package scheduler

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

// MergeConvergencePersistObjectUpdateBody applies operator-preserving merges before storage Update for
// convergence_session measure paths (--persist-session, convergence_session_tick). Pass the same health
// snapshot used to build object_update_body so phase can be preserved when measurement is degraded.
func MergeConvergencePersistObjectUpdateBody(existingCVS map[string]any, body map[string]any, snap *TestBundleConvergenceSnapshot) {
	MergeObjectUpdateBodyPreservingOperatorNextAction(existingCVS, body)
	MergeObjectUpdateBodyPreservingCurrentPhaseOnDegradedSnapshot(existingCVS, body, snap)
}

// MergeObjectUpdateBodyPreservingCurrentPhaseOnDegradedSnapshot keeps a higher-ranked current_phase
// when the snapshot has no usable health window (delta unknown / zero lines). Otherwise a persist tick
// would rewrite c6_exit (or any later phase) back to c1_scope implied by empty data.
func MergeObjectUpdateBodyPreservingCurrentPhaseOnDegradedSnapshot(existingCVS map[string]any, body map[string]any, snap *TestBundleConvergenceSnapshot) {
	if existingCVS == nil || body == nil || snap == nil {
		return
	}
	if !isDegradedHealthSnapshot(snap) {
		return
	}
	ex := strings.TrimSpace(FieldAsString(existingCVS[objects.FieldKeyCurrentPhase]))
	sug := strings.TrimSpace(FieldAsString(body[objects.FieldKeyCurrentPhase]))
	if ex == "" || sug == "" {
		return
	}
	ri := phaseRank(ConvergencePhase(ex))
	rj := phaseRank(ConvergencePhase(sug))
	if ri < 0 || rj < 0 {
		return
	}
	if ri <= rj {
		return
	}
	body[objects.FieldKeyCurrentPhase] = ex
	patchConvergenceActivityLogPhase(body, ex)
}

func isDegradedHealthSnapshot(snap *TestBundleConvergenceSnapshot) bool {
	if snap == nil {
		return false
	}
	if snap.LinesInWindow == 0 || snap.DeltaAssessment == "unknown" {
		return true
	}
	return false
}

func patchConvergenceActivityLogPhase(body map[string]any, phase string) {
	al, ok := body[objects.FieldKeyActivityLog].([]any)
	if !ok || len(al) == 0 {
		return
	}
	last, ok := al[len(al)-1].(map[string]any)
	if !ok {
		return
	}
	last[convSugKeyActivityPhase] = phase
}

// MergeObjectUpdateBodyPreservingOperatorNextAction keeps an existing convergence_session next_action
// when the newly suggested value is measurement-only (bundle hints from BuildNextActionText) and the
// persisted value reads like operator handoff (drift/triage/matrix commands, long guidance).
// This prevents --persist-session and convergence_session_tick from clobbering curated next_action text.
func MergeObjectUpdateBodyPreservingOperatorNextAction(existingCVS map[string]any, body map[string]any) {
	if existingCVS == nil || body == nil {
		return
	}
	ex := strings.TrimSpace(FieldAsString(existingCVS[objects.FieldKeyNextAction]))
	sug := strings.TrimSpace(FieldAsString(body[objects.FieldKeyNextAction]))
	if ex == "" || sug == "" {
		return
	}
	if !isOperatorHandoffNextAction(ex) || !isMeasurementOnlyNextActionText(sug) {
		return
	}
	body[objects.FieldKeyNextAction] = ex
}

func isOperatorHandoffNextAction(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return false
	}
	markers := []string{
		"drift", "triage", "cvs_outcome", paths.ProjectDataDir + "/" + paths.LogsDir + "/" + paths.LogsDriftSubdir, paths.RewriteCanonicalCLIInvocations("zqk test run"), "matrix", "baseline", "hardcoded-go-literals", "rollup", "fieldkeys",
		"packages rollup", "scripts/drift",
	}
	for _, m := range markers {
		if strings.Contains(s, m) {
			return true
		}
	}
	// Long free-text handoffs without a marker keyword (rare).
	return len(s) > 420
}

// AgentPromptPreferredNextAction returns the next_action string for agent-prompt markdown. When the CVS
// already stores operator handoff text and BuildSuggestedConvergenceSessionFields only produced bundle
// measurement hints, the persisted text is shown so the prompt matches storage (same rule as persist merge).
func AgentPromptPreferredNextAction(existingCVS map[string]any, suggested map[string]any) string {
	if suggested == nil {
		return ""
	}
	sug := strings.TrimSpace(FieldAsString(suggested[objects.FieldKeyNextAction]))
	if existingCVS == nil {
		return sug
	}
	ex := strings.TrimSpace(FieldAsString(existingCVS[objects.FieldKeyNextAction]))
	if ex == "" {
		return sug
	}
	if isOperatorHandoffNextAction(ex) && isMeasurementOnlyNextActionText(sug) {
		return ex
	}
	return sug
}

func isMeasurementOnlyNextActionText(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if idx := strings.Index(s, "\n\nSuggested reruns"); idx >= 0 {
		s = strings.TrimSpace(s[:idx])
	}
	exact := []string{
		"No failing outcomes in this window.",
		"No health lines in window; widen --limit or run scheduler test bundles.",
		"One or more bundle fingerprints still failing; use suggested_rerun_by_fingerprint or apply a fix, then re-run bundles.",
		"Latest outcomes are green but an unrecovered bad outcome remains in-window for at least one fingerprint (e.g. ambiguous outcomes after a fail); confirm with another bundle run or inspect health.jsonl.",
	}
	for _, c := range exact {
		if s == c {
			return true
		}
	}
	const trig = "Trigger queue has pending work; wait for bundle jobs to drain before treating empty failing_fingerprints_now as final. "
	if strings.HasPrefix(s, trig) {
		rest := strings.TrimSpace(strings.TrimPrefix(s, trig))
		for _, c := range exact {
			if rest == c {
				return true
			}
		}
	}
	return false
}
