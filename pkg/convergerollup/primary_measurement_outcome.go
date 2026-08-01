package convergerollup

import (
	"strings"
)

// PrimaryMeasurementOutcome is the normative taxonomy in docs/architecture/MEASUREMENT_OUTCOME_TAXONOMY.md
// (rollup_v1.primary_measurement_outcome).
type PrimaryMeasurementOutcome string

const (
	MeasurementYieldsConvergence      PrimaryMeasurementOutcome = "measurement_yields_convergence"
	MeasurementYieldsDivergence       PrimaryMeasurementOutcome = "measurement_yields_divergence"
	MeasurementYieldsHaltOrError      PrimaryMeasurementOutcome = "measurement_yields_halt_or_error"
	MeasurementYieldsAmbiguousOutcome PrimaryMeasurementOutcome = "measurement_yields_ambiguous_outcome"
)

// ComputePrimaryMeasurementOutcome maps measured rollup state to the four primary outcomes.
// When gatesSkipped is true, fkExit/zeExit should be treated as unused (callers pass 0).
// matrixErr non-empty means the vetting matrix surface could not be read (Python full rollup only).
func ComputePrimaryMeasurementOutcome(
	status RollupStatus,
	blockers []Blocker,
	matrixErr string,
	fkExit, zeExit int,
	gatesSkipped bool,
) (PrimaryMeasurementOutcome, string) {
	if strings.TrimSpace(matrixErr) != "" {
		return MeasurementYieldsHaltOrError, "vetting_matrix_unreadable"
	}
	if !gatesSkipped && (fkExit != 0 || zeExit != 0) {
		return MeasurementYieldsHaltOrError, "literal_gate_failed"
	}

	if status == RollupStatusSatisfied {
		return MeasurementYieldsConvergence, "all_configured_surfaces_green"
	}

	codes := blockerCodes(blockers)

	if status == RollupStatusBlocked {
		if hasCode(codes, "test_bundles_failing") || hasCode(codes, "delta_trending_away") {
			return MeasurementYieldsDivergence, "bundle_health_bad_signal"
		}
		return MeasurementYieldsDivergence, "rollup_blocked"
	}

	if status == RollupStatusReadyForReview {
		return MeasurementYieldsDivergence, "follow_up_required_matrix_or_child"
	}

	if status == RollupStatusPartial {
		if ambiguousPartialBlockers(codes) {
			return MeasurementYieldsAmbiguousOutcome, "bundle_window_incomplete_or_gates_skipped"
		}
		return MeasurementYieldsDivergence, "composite_partial"
	}

	return MeasurementYieldsAmbiguousOutcome, "unclassified"
}

func blockerCodes(blockers []Blocker) []string {
	out := make([]string, 0, len(blockers))
	for i := range blockers {
		out = append(out, blockers[i].Code)
	}
	return out
}

func hasCode(codes []string, want string) bool {
	for _, c := range codes {
		if c == want {
			return true
		}
	}
	return false
}

// ambiguousPartialBlockers is true when the only blockers are the soft “bundles not ready” / gates-skipped
// signals (stale window, incomplete measurement), per MEASUREMENT_OUTCOME_TAXONOMY.md supplementary guidance.
func ambiguousPartialBlockers(codes []string) bool {
	if len(codes) == 0 {
		return false
	}
	for _, c := range codes {
		if c != "bundle_gate_not_ready" && c != "gates_skipped" {
			return false
		}
	}
	return true
}
