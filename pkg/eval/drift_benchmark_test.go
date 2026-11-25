package eval

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSummarizeDriftBenchmark_DeterministicCrossArmMetrics(t *testing.T) {
	obs := []DriftTurnObservation{
		// Control A (advisory CLAUDE.md): drifts after compaction, silent false-done on s1
		{SessionID: "s1", Arm: "control_a_claude_md", Turn: 5},
		{SessionID: "s1", Arm: "control_a_claude_md", Turn: 18, ViolatedInvariants: []string{"no_raw_grep"}},
		{SessionID: "s1", Arm: "control_a_claude_md", Turn: 36, ViolatedInvariants: []string{"race_clean"}, ClaimedDone: true},
		{SessionID: "s2", Arm: "control_a_claude_md", Turn: 38},

		// Treatment C (ZQK fail-closed gates): blocks violations pre-PR with zero silent false-dones
		{SessionID: "s1", Arm: "treatment_c_zqk_gates", Turn: 5},
		{SessionID: "s1", Arm: "treatment_c_zqk_gates", Turn: 18, ViolatedInvariants: []string{"no_raw_grep"}, BlockedByGate: true, RecoveryRetries: 1, GateOverheadTokens: 600},
		{SessionID: "s1", Arm: "treatment_c_zqk_gates", Turn: 36, ViolatedInvariants: []string{"race_clean"}, BlockedByGate: true, ClaimedDone: true, RecoveryRetries: 1, GateOverheadTokens: 900},
	}

	report := SummarizeDriftBenchmark(obs, 0, 0)
	require.Equal(t, 10, report.BucketSize)
	require.Equal(t, 40, report.MaxTurns)
	require.Len(t, report.Arms, 2)

	// Deterministic alphabetical ordering of arms
	armA := report.Arms[0]
	assert.Equal(t, "control_a_claude_md", armA.Arm)
	assert.Equal(t, 2, armA.Sessions)
	assert.Equal(t, 4, armA.TotalTurns)
	assert.Equal(t, 2, armA.TotalViolations)
	assert.Equal(t, 0, armA.BlockedViolations)
	assert.Equal(t, 1, armA.SilentFalseDoneCount)
	assert.InDelta(t, 50.0, armA.SilentFalseDoneRatePc, 0.01)
	assert.InDelta(t, 0.0, armA.PrePRCatchRatePc, 0.01)
	require.Len(t, armA.Buckets, 4)
	assert.Equal(t, "1-10", armA.Buckets[0].Label)
	assert.InDelta(t, 0.0, armA.Buckets[0].ViolationRatePc, 0.01)
	assert.Equal(t, "11-20", armA.Buckets[1].Label)
	assert.InDelta(t, 100.0, armA.Buckets[1].ViolationRatePc, 0.01)
	assert.Equal(t, "31-40", armA.Buckets[3].Label)
	assert.InDelta(t, 50.0, armA.Buckets[3].ViolationRatePc, 0.01)

	armC := report.Arms[1]
	assert.Equal(t, "treatment_c_zqk_gates", armC.Arm)
	assert.Equal(t, 1, armC.Sessions)
	assert.Equal(t, 2, armC.TotalViolations)
	assert.Equal(t, 2, armC.BlockedViolations)
	assert.Equal(t, 0, armC.SilentFalseDoneCount)
	assert.InDelta(t, 0.0, armC.SilentFalseDoneRatePc, 0.01)
	assert.InDelta(t, 100.0, armC.PrePRCatchRatePc, 0.01)
	assert.InDelta(t, 2.0, armC.AvgRecoveryRetries, 0.01)
	assert.InDelta(t, 1500.0, armC.AvgGateOverheadTokens, 0.01)

	csv := FormatDriftCurveCSV(report)
	assert.Contains(t, csv, "arm,turn_bucket,total_turns,violated_turns,violation_rate_pct\n")
	assert.Contains(t, csv, "control_a_claude_md,11-20,1,1,100.00\n")
	assert.Contains(t, csv, "treatment_c_zqk_gates,31-40,1,1,100.00\n")

	md := FormatDriftReportMarkdown(report)
	assert.Contains(t, md, "## Study 3 — Compaction Drift & Fail-Closed Done-Gate Report")
	assert.Contains(t, md, "| `control_a_claude_md` | 2 | 4 | 2 | 0.00% | 50.00% | 0.00 | 0 |")
	assert.Contains(t, md, "| `treatment_c_zqk_gates` | 1 | 3 | 2 | 100.00% | 0.00% | 2.00 | 1500 |")
}

func TestSummarizeDriftBenchmark_EmptyAndOutOfRangeTurns(t *testing.T) {
	empty := SummarizeDriftBenchmark(nil, 10, 40)
	assert.Empty(t, empty.Arms)
	assert.Equal(t, "arm,turn_bucket,total_turns,violated_turns,violation_rate_pct\n", FormatDriftCurveCSV(empty))

	obs := []DriftTurnObservation{
		{Turn: 0},
		{Turn: 99, ViolatedInvariants: []string{"x"}},
	}
	report := SummarizeDriftBenchmark(obs, 10, 40)
	require.Len(t, report.Arms, 1)
	assert.Equal(t, "unknown", report.Arms[0].Arm)
	assert.Equal(t, 1, report.Arms[0].Sessions)
	assert.Equal(t, 1, report.Arms[0].TotalViolations)
	for _, b := range report.Arms[0].Buckets {
		assert.Equal(t, 0, b.TotalTurns)
	}
}
