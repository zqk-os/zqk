package eval

import (
	"context"
	"math"
	"strings"
	"testing"
)

func TestCycle69_AutoEvalGate_StaticFloorInvariants(t *testing.T) {
	t.Parallel()

	t.Run("invalid_min_pass_rate_negative", func(t *testing.T) {
		cfg := GateConfig{MinPassRate: -0.1, MaxVariance: 0.1}
		_, err := NewAutoEvalGate(cfg)
		if err == nil {
			t.Fatalf("expected error for negative MinPassRate")
		}
	})

	t.Run("invalid_min_pass_rate_above_one", func(t *testing.T) {
		cfg := GateConfig{MinPassRate: 1.5, MaxVariance: 0.1}
		_, err := NewAutoEvalGate(cfg)
		if err == nil {
			t.Fatalf("expected error for MinPassRate > 1.0")
		}
	})

	t.Run("invalid_max_variance_negative", func(t *testing.T) {
		cfg := GateConfig{MinPassRate: 0.8, MaxVariance: -0.05}
		_, err := NewAutoEvalGate(cfg)
		if err == nil {
			t.Fatalf("expected error for negative MaxVariance")
		}
	})

	t.Run("valid_config_passes", func(t *testing.T) {
		cfg := GateConfig{MinPassRate: 0.85, MaxVariance: 0.10, RequiredModels: []string{"model-a"}}
		gate, err := NewAutoEvalGate(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gate == nil {
			t.Fatalf("expected non-nil AutoEvalGate")
		}
	})
}

func TestCycle69_AutoEvalGate_OperationalDynamicProof(t *testing.T) {
	t.Parallel()

	cfg := GateConfig{
		MinPassRate:    0.75,
		MaxVariance:    0.05,
		RequiredModels: []string{"gpt-4o", "claude-3-5-sonnet"},
	}
	gate, err := NewAutoEvalGate(cfg)
	if err != nil {
		t.Fatalf("failed to init gate: %v", err)
	}

	outcomes := map[string][]ScenarioOutcome{
		"gpt-4o": {
			{ScenarioID: "sc-1", Success: true},
			{ScenarioID: "sc-2", Success: true},
			{ScenarioID: "sc-3", Success: true},
			{ScenarioID: "sc-4", Success: true},
		}, // 100%
		"claude-3-5-sonnet": {
			{ScenarioID: "sc-1", Success: true},
			{ScenarioID: "sc-2", Success: true},
			{ScenarioID: "sc-3", Success: true},
			{ScenarioID: "sc-4", Success: false},
		}, // 75%
	}

	decision, err := gate.EvaluateOutcomes(context.Background(), outcomes)
	if err != nil {
		t.Fatalf("unexpected evaluation error: %v", err)
	}

	if !decision.Passed {
		t.Fatalf("expected gate to pass, got violations: %v", decision.Violations)
	}

	expectedMean := 0.875
	if math.Abs(decision.MeanPassRate-expectedMean) > 1e-4 {
		t.Errorf("expected mean pass rate %.4f, got %.4f", expectedMean, decision.MeanPassRate)
	}

	if decision.StdDev <= 0 {
		t.Errorf("expected positive standard deviation, got %.4f", decision.StdDev)
	}

	dashboard := gate.FormatDashboard(decision)
	if !strings.Contains(dashboard, "PASSED") {
		t.Errorf("expected dashboard to report PASSED status")
	}
	if !strings.Contains(dashboard, "gpt-4o") || !strings.Contains(dashboard, "claude-3-5-sonnet") {
		t.Errorf("expected dashboard to contain both model breakdowns")
	}

	// Also verify MatrixResult integration
	matrixRes := &MatrixResult{
		ModelConvergence: map[string]*ModelScoreSummary{
			"gpt-4o": {
				Model:       "gpt-4o",
				RunsCount:   4,
				PassedCount: 4,
				PassRate:    1.0,
			},
			"claude-3-5-sonnet": {
				Model:       "claude-3-5-sonnet",
				RunsCount:   4,
				PassedCount: 3,
				PassRate:    0.75,
			},
		},
	}
	matrixDecision, err := gate.EvaluateMatrix(context.Background(), matrixRes)
	if err != nil {
		t.Fatalf("unexpected matrix evaluation error: %v", err)
	}
	if !matrixDecision.Passed {
		t.Fatalf("expected matrix evaluation to pass, got: %v", matrixDecision.Violations)
	}
}

func TestCycle69_AutoEvalGate_NegativeAdversarialBoundary(t *testing.T) {
	t.Parallel()

	gate, err := NewAutoEvalGate(GateConfig{
		MinPassRate:    0.80,
		MaxVariance:    0.02,
		RequiredModels: []string{"primary-model"},
	})
	if err != nil {
		t.Fatalf("failed to init gate: %v", err)
	}

	t.Run("nil_matrix_result_fails_soft", func(t *testing.T) {
		_, err := gate.EvaluateMatrix(context.Background(), nil)
		if err == nil {
			t.Fatalf("expected error on nil MatrixResult")
		}
	})

	t.Run("empty_outcomes_fails_soft", func(t *testing.T) {
		_, err := gate.EvaluateOutcomes(context.Background(), nil)
		if err == nil {
			t.Fatalf("expected error on empty outcomes")
		}
	})

	t.Run("empty_model_outcomes_fails_soft", func(t *testing.T) {
		outcomes := map[string][]ScenarioOutcome{
			"primary-model": {},
		}
		_, err := gate.EvaluateOutcomes(context.Background(), outcomes)
		if err == nil {
			t.Fatalf("expected error on empty scenario outcomes for model")
		}
	})

	t.Run("fail_closed_low_pass_rate", func(t *testing.T) {
		outcomes := map[string][]ScenarioOutcome{
			"primary-model": {
				{ScenarioID: "sc-1", Success: false},
				{ScenarioID: "sc-2", Success: true},
			}, // 50% < 80%
		}
		decision, err := gate.EvaluateOutcomes(context.Background(), outcomes)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if decision.Passed {
			t.Fatalf("expected gate to FAIL due to low pass rate")
		}
		if len(decision.Violations) == 0 {
			t.Fatalf("expected violations to be recorded")
		}
	})

	t.Run("fail_closed_excessive_variance", func(t *testing.T) {
		// Model A: 100%, Model B: 0% -> variance = (1.0-0.5)^2 + (0.0-0.5)^2 = 0.50 > 0.02
		outcomes := map[string][]ScenarioOutcome{
			"primary-model": {
				{ScenarioID: "sc-1", Success: true},
			},
			"flaky-model": {
				{ScenarioID: "sc-1", Success: false},
			},
		}
		decision, err := gate.EvaluateOutcomes(context.Background(), outcomes)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if decision.Passed {
			t.Fatalf("expected gate to FAIL due to excessive variance")
		}
		var varianceReported bool
		for _, v := range decision.Violations {
			if strings.Contains(v, "cross-model variance") {
				varianceReported = true
			}
		}
		if !varianceReported {
			t.Errorf("expected variance violation in: %v", decision.Violations)
		}
	})

	t.Run("fail_closed_missing_required_model", func(t *testing.T) {
		outcomes := map[string][]ScenarioOutcome{
			"other-model": {
				{ScenarioID: "sc-1", Success: true},
			},
		}
		decision, err := gate.EvaluateOutcomes(context.Background(), outcomes)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if decision.Passed {
			t.Fatalf("expected gate to FAIL due to missing required model")
		}
	})

	t.Run("nan_inf_sanitization_in_variance", func(t *testing.T) {
		dirty := []float64{0.80, math.NaN(), 0.90, math.Inf(1)}
		mean, variance, stdDev := CalculateVariance(dirty)
		if math.IsNaN(mean) || math.IsInf(mean, 0) {
			t.Fatalf("expected clean mean, got %f", mean)
		}
		if math.Abs(mean-0.85) > 1e-4 {
			t.Errorf("expected clean mean 0.85, got %f", mean)
		}
		if variance <= 0 || stdDev <= 0 {
			t.Errorf("expected positive variance and stdDev, got var=%f stdDev=%f", variance, stdDev)
		}
	})

	t.Run("single_model_zero_variance", func(t *testing.T) {
		single := []float64{0.95}
		mean, variance, stdDev := CalculateVariance(single)
		if mean != 0.95 {
			t.Errorf("expected mean 0.95, got %f", mean)
		}
		if variance != 0 || stdDev != 0 {
			t.Errorf("expected zero variance for single value, got var=%f stdDev=%f", variance, stdDev)
		}
	})

	t.Run("format_dashboard_nil_safe", func(t *testing.T) {
		if out := gate.FormatDashboard(nil); out != "" {
			t.Errorf("expected empty string for nil decision, got %q", out)
		}
	})
}
