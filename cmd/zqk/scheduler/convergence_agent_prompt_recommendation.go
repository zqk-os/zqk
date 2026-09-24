package scheduler

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

// formatConvergenceDesiredEndStateRecommendation is a deterministic end-of-iteration synthesis:
// it does not parse or score free-text desired_end_state, but tells the agent how to prioritize
// contract work vs measurement/rollup signals already printed above.
func formatConvergenceDesiredEndStateRecommendation(cvs, sug map[string]any, snap *schedpkg.TestBundleConvergenceSnapshot, rollupCore map[string]any) string {
	thresholds := schedpkg.ThresholdsMapFromObject(cvs)
	rfsRequired := schedpkg.ThresholdsCompletionGateRequiresReadyForSessionCompletion(thresholds)

	var b strings.Builder
	b.WriteString("## Convergence recommendation (this iteration)\n\n")
	b.WriteString("**Ground rule:** Treat **Desired end state** (session contract above) as the authoritative completion checklist — together with linked **acceptance criteria** when present. ")
	b.WriteString("This command does **not** parse or verify every bullet in **desired_end_state**; bundle health, rollup, and phase hints below are **supporting** signals only.\n\n")

	b.WriteString("**Signals (this run):**\n")
	if len(rollupCore) == 0 {
		b.WriteString(paths.RewriteCanonicalCLIInvocations("- **rollup_status_core:** missing — re-run `zqk scheduler convergence measure` with a current `zqk` binary.\n"))
	} else {
		if rs, ok := rollupCore["rollup_status"].(string); ok && strings.TrimSpace(rs) != "" {
			fmt.Fprintf(&b, "- **rollup_status:** `%s`\n", strings.TrimSpace(rs))
		}
		nBl := rollupBlockerCount(rollupCore)
		if nBl > 0 {
			fmt.Fprintf(&b, "- **rollup blockers:** %d\n", nBl)
		}
	}
	if !rfsRequired {
		b.WriteString("- **completion_gate:** `require_ready_for_session_completion` is **false** — rollup does not treat **`ready_for_session_completion`** as a blocking gate; health.jsonl is informative.\n")
	}
	if snap == nil {
		b.WriteString("- **Latest measurement:** *(no snapshot — empty window or unavailable.)*\n")
	} else {
		fmt.Fprintf(&b, "- **failing fingerprints (latest):** %d\n", len(snap.FailingFingerprintsNow))
		fmt.Fprintf(&b, "- **ready_for_session_completion:** %v\n", snap.ReadyForSessionCompletion)
	}
	curPhase := strings.TrimSpace(convergenceFieldString(cvs, objects.FieldKeyCurrentPhase))
	pa := phaseRouterStringFromSuggested(sug, "phase_alignment")
	sugPhase := phaseRouterStringFromSuggested(sug, "suggested_current_phase")
	if curPhase != "" || pa != "" || sugPhase != "" {
		if curPhase != "" {
			fmt.Fprintf(&b, "- **persisted current_phase:** `%s`\n", curPhase)
		}
		if pa != "" {
			fmt.Fprintf(&b, "- **phase_alignment:** `%s`\n", pa)
		}
		if sugPhase != "" {
			fmt.Fprintf(&b, "- **suggested_current_phase (measurement):** `%s`\n", sugPhase)
		}
	}
	b.WriteString("\n**Recommended focus for the next iteration:**\n")

	des := strings.TrimSpace(convergenceFieldString(cvs, objects.FieldKeyDesiredEndState))
	var focus []string
	if des == "" {
		focus = append(focus, "**Contract:** `desired_end_state` is empty — define the written target on the CVS before closing the session.")
	}

	rs := rollupStatusTrimmed(rollupCore)
	nBlock := rollupBlockerCount(rollupCore)

	if len(rollupCore) > 0 {
		if nBlock > 0 {
			focus = append(focus, "**Gates:** Address **rollup blockers** and **Recommended next (measurement-derived)** above; re-run **convergence measure** until blockers clear.")
		} else if rs == "blocked" || rs == "partial" {
			focus = append(focus, "**Gates:** `rollup_status` is not `satisfied` — follow the rollup section until measurement-derived gates are green or you document an explicit process exception.")
		} else if rs == "ready_for_review" {
			focus = append(focus, "**Gates:** `rollup_status` is `ready_for_review` — finish vetting matrix / child CVS / documented acceptance (per rollup) before treating the session as fully converged.")
		}
	}

	if snap != nil && len(snap.FailingFingerprintsNow) > 0 {
		focus = append(focus, paths.RewriteCanonicalCLIInvocations("**Tests:** Fix failing test_case objects (`zqk test run`) until Latest measurement is green."))
	}
	if snap != nil && len(snap.FailingFingerprintsNow) == 0 && !snap.ReadyForSessionCompletion {
		if rfsRequired {
			focus = append(focus, "**Bundle-health:** Clear `ready_for_session_completion` (heartbeat, trigger queue, or window) using blocked reasons in Latest measurement above.")
		} else {
			focus = append(focus, "**Bundle-health (informative):** `ready_for_session_completion` is false (e.g. no health data in window). With **`thresholds.completion_gate.require_ready_for_session_completion: false`**, do **not** block **desired end state** work on this signal — run targeted bundles only when you want health.jsonl visibility.")
		}
	}

	switch strings.ToLower(strings.TrimSpace(pa)) {
	case "session_behind":
		focus = append(focus, "**Phase:** `session_behind` — measurement is ahead of persisted phase. Advance **`current_phase`** only when **desired end state** work for the current phase is actually complete; do not jump to **`c6_exit`** on green bundles alone.")
	case "session_ahead":
		if rfsRequired {
			focus = append(focus, "**Phase:** `session_ahead` — persisted phase is ahead of measurement. Stabilize bundles and evidence before **status → completed**; confirm remaining **desired end state** items for phases already recorded.")
		} else {
			focus = append(focus, "**Phase:** `session_ahead` — persisted phase is ahead of a weak or empty measurement window (expected when bundle completion is not the gate). **Primary focus:** verify **desired end state** and linked acceptance criteria before **status → completed**; use health.jsonl as supporting evidence only.")
		}
	case "unknown":
		if strings.TrimSpace(curPhase) != "" {
			focus = append(focus, "**Phase:** alignment is **unknown** — fix invalid **`current_phase`**, or restore a non-degraded snapshot, before relying on phase hints.")
		}
	}

	rollupOK := len(rollupCore) > 0 && nBlock == 0 && strings.EqualFold(rs, "satisfied")
	var bundlesOK bool
	if snap != nil {
		if rfsRequired {
			bundlesOK = len(snap.FailingFingerprintsNow) == 0 && snap.ReadyForSessionCompletion
		} else {
			bundlesOK = len(snap.FailingFingerprintsNow) == 0
		}
	}
	if snap == nil && len(rollupCore) > 0 && rollupOK && len(focus) == 0 {
		if rfsRequired {
			focus = append(focus, "**Measurement:** No health snapshot — run targeted bundles and re-measure so Latest measurement aligns with rollup before closing.")
		} else {
			focus = append(focus, "**Measurement:** No health snapshot — optional targeted bundles to populate health.jsonl for visibility; rollup can still be satisfied when completion_gate does not require bundle readiness.")
		}
	}
	if rollupOK && bundlesOK && len(focus) == 0 {
		focus = append(focus, paths.RewriteCanonicalCLIInvocations("**Contract:** Rollup and bundle-health gates look clear. **Primary next work:** walk **Desired end state** and acceptance criteria; when satisfied per process rules, update **`status`** / **`current_phase`** via `zqk object update`. Green measurement does not replace missing contract items."))
	}
	if len(focus) == 0 {
		focus = append(focus, "**Next:** Use **Desired end state** as the checklist; align **`status`** and **`current_phase`** with process rules; use rollup and Latest measurement as supporting evidence.")
	}

	for _, line := range focus {
		fmt.Fprintf(&b, "- %s\n", line)
	}
	return b.String()
}

func phaseRouterStringFromSuggested(sug map[string]any, key string) string {
	if sug == nil {
		return ""
	}
	pr, _ := sug["phase_router"].(map[string]any)
	if pr == nil {
		return ""
	}
	v, ok := pr[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func rollupStatusTrimmed(rollup map[string]any) string {
	if rollup == nil {
		return ""
	}
	s, _ := rollup["rollup_status"].(string)
	return strings.ToLower(strings.TrimSpace(s))
}

func rollupBlockerCount(rollup map[string]any) int {
	if rollup == nil {
		return 0
	}
	bl, _ := rollup[objects.FieldKeyBlockers].([]any)
	return len(bl)
}
