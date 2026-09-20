package convergerollup

import "testing"

func TestComputePrimaryMeasurementOutcome_satisfied(t *testing.T) {
	o, d := ComputePrimaryMeasurementOutcome(RollupStatusSatisfied, nil, "", 0, 0, false)
	if o != MeasurementYieldsConvergence || d != "all_configured_surfaces_green" {
		t.Fatalf("got %s %s", o, d)
	}
}

func TestComputePrimaryMeasurementOutcome_haltGate(t *testing.T) {
	o, d := ComputePrimaryMeasurementOutcome(RollupStatusBlocked, []Blocker{{Code: "field_key_literals_gate", Detail: "exit 1"}}, "", 1, 0, false)
	if o != MeasurementYieldsHaltOrError || d != "literal_gate_failed" {
		t.Fatalf("got %s %s", o, d)
	}
}

func TestComputePrimaryMeasurementOutcome_haltMatrixErr(t *testing.T) {
	o, d := ComputePrimaryMeasurementOutcome(RollupStatusPartial, nil, "matrix_read_error:x", 0, 0, false)
	if o != MeasurementYieldsHaltOrError || d != "vetting_matrix_unreadable" {
		t.Fatalf("got %s %s", o, d)
	}
}

func TestComputePrimaryMeasurementOutcome_divergenceFingerprints(t *testing.T) {
	o, d := ComputePrimaryMeasurementOutcome(RollupStatusBlocked, []Blocker{{Code: "test_bundles_failing", Detail: "1 fingerprint(s) latest bad"}}, "", 0, 0, false)
	if o != MeasurementYieldsDivergence || d != "bundle_health_bad_signal" {
		t.Fatalf("got %s %s", o, d)
	}
}

func TestComputePrimaryMeasurementOutcome_ambiguousPartial(t *testing.T) {
	o, d := ComputePrimaryMeasurementOutcome(RollupStatusPartial, []Blocker{{Code: "bundle_gate_not_ready", Detail: "x"}}, "", 0, 0, false)
	if o != MeasurementYieldsAmbiguousOutcome || d != "bundle_window_incomplete_or_gates_skipped" {
		t.Fatalf("got %s %s", o, d)
	}
}

func TestComputePrimaryMeasurementOutcome_skipGatesNoHalt(t *testing.T) {
	o, d := ComputePrimaryMeasurementOutcome(RollupStatusPartial, []Blocker{{Code: "bundle_gate_not_ready", Detail: "x"}}, "", 99, 99, true)
	if o != MeasurementYieldsAmbiguousOutcome {
		t.Fatalf("expected ambiguous when gates skipped, got %s %s", o, d)
	}
}
