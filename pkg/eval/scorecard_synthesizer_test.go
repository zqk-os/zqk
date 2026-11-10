package eval

import (
	"math"
	"testing"
	"time"
)

func sampleBenchmarkRuns() []BenchmarkRun {
	baseTime := time.Date(2026, 11, 10, 1, 0, 0, 0, time.UTC)
	return []BenchmarkRun{
		{
			RunID: "run-m1-1", ScenarioID: "sc-01", ModelID: "gemini-pro",
			Passed: true, DurationMs: 350.0, TokensUsed: 400, Variance: 0.02, Timestamp: baseTime,
		},
		{
			RunID: "run-m1-2", ScenarioID: "sc-02", ModelID: "gemini-pro",
			Passed: true, DurationMs: 400.0, TokensUsed: 450, Variance: 0.03, Timestamp: baseTime,
		},
		{
			RunID: "run-m2-1", ScenarioID: "sc-01", ModelID: "gemini-flash",
			Passed: true, DurationMs: 120.0, TokensUsed: 250, Variance: 0.08, Timestamp: baseTime,
		},
		{
			RunID: "run-m2-2", ScenarioID: "sc-02", ModelID: "gemini-flash",
			Passed: false, DurationMs: 150.0, TokensUsed: 300, Variance: 0.12, Timestamp: baseTime,
		},
	}
}

func TestCycle71_ScorecardSynthesizer_StaticFloorInvariants(t *testing.T) {
	synth := NewScorecardSynthesizer(PolicyBalanced)
	runs := sampleBenchmarkRuns()

	card, quarantined, err := synth.Synthesize("BSC-TEST-001", runs, PolicyBalanced)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(quarantined) != 0 {
		t.Errorf("expected 0 quarantined runs, got %d", len(quarantined))
	}
	if card.SchemaVersion != BenchmarkScorecardSchemaV1 {
		t.Errorf("expected schema version %s, got %s", BenchmarkScorecardSchemaV1, card.SchemaVersion)
	}
	if card.ScorecardID != "BSC-TEST-001" {
		t.Errorf("expected scorecard ID BSC-TEST-001, got %s", card.ScorecardID)
	}
	if len(card.Leaderboard) != 2 {
		t.Fatalf("expected 2 leaderboard entries, got %d", len(card.Leaderboard))
	}
	for _, entry := range card.Leaderboard {
		if entry.CompositeScore < 0.0 || entry.CompositeScore > 100.0 {
			t.Errorf("composite score out of bounds: %f", entry.CompositeScore)
		}
		if entry.PassRate < 0.0 || entry.PassRate > 1.0 {
			t.Errorf("pass rate out of bounds: %f", entry.PassRate)
		}
	}
}

func TestCycle71_ScorecardSynthesizer_OperationalDynamicProof(t *testing.T) {
	synth := NewScorecardSynthesizer(PolicyQualityFirst)
	runs := sampleBenchmarkRuns()

	card, _, err := synth.Synthesize("BSC-DYNAMIC-01", runs, PolicyQualityFirst)
	if err != nil {
		t.Fatalf("synthesis failed: %v", err)
	}
	if len(card.Leaderboard) < 2 {
		t.Fatalf("insufficient leaderboard entries: %d", len(card.Leaderboard))
	}

	first := card.Leaderboard[0]
	second := card.Leaderboard[1]

	if first.Rank != 1 || second.Rank != 2 {
		t.Errorf("incorrect ranks: #1=%d, #2=%d", first.Rank, second.Rank)
	}
	if first.CompositeScore < second.CompositeScore {
		t.Errorf("leaderboard not sorted: rank 1 (%f) < rank 2 (%f)", first.CompositeScore, second.CompositeScore)
	}
	if first.ModelID != "gemini-pro" {
		t.Errorf("expected gemini-pro to lead under quality_first, got %s", first.ModelID)
	}
}

func TestCycle71_ScorecardSynthesizer_NegativeAdversarialBoundary(t *testing.T) {
	synth := NewScorecardSynthesizer(PolicyBalanced)
	adversarialRuns := sampleBenchmarkRuns()

	adversarialRuns = append(adversarialRuns,
		BenchmarkRun{RunID: "bad-nan", ModelID: "gemini-flash", DurationMs: math.NaN()},
		BenchmarkRun{RunID: "bad-inf", ModelID: "gemini-flash", DurationMs: math.Inf(1)},
		BenchmarkRun{RunID: "bad-neg", ModelID: "gemini-flash", DurationMs: -50.0},
		BenchmarkRun{RunID: "run-m1-1", ScenarioID: "sc-01", ModelID: "gemini-pro"}, // duplicate
	)

	card, quarantined, err := synth.Synthesize("BSC-ADV-01", adversarialRuns, PolicyBalanced)
	if err != nil {
		t.Fatalf("expected successful synthesis with quarantine, got error: %v", err)
	}
	if len(quarantined) != 4 {
		t.Errorf("expected exactly 4 quarantined runs, got %d", len(quarantined))
	}
	if card.QuarantinedRunsCount != 4 {
		t.Errorf("expected card.QuarantinedRunsCount=4, got %d", card.QuarantinedRunsCount)
	}

	_, _, emptyErr := synth.Synthesize("BSC-EMPTY", nil, PolicyBalanced)
	if emptyErr == nil {
		t.Error("expected error for empty runs, got nil")
	}
}
