package router

import (
	"context"
	"errors"
	"testing"
)

func setupTestRouter(t *testing.T, budget int) *AdaptiveRouter {
	t.Helper()
	r, err := NewAdaptiveRouter(budget, TierStandard, "fallback-standard")
	if err != nil {
		t.Fatalf("failed to create adaptive router: %v", err)
	}

	if err := r.RegisterModelCost(ModelCostProfile{ModelID: "gemini-flash", Tier: TierFast, CostPer1k: 0.001}); err != nil {
		t.Fatalf("register flash cost failed: %v", err)
	}
	if err := r.RegisterModelCost(ModelCostProfile{ModelID: "gemini-pro", Tier: TierStandard, CostPer1k: 0.005}); err != nil {
		t.Fatalf("register pro cost failed: %v", err)
	}
	if err := r.RegisterModelCost(ModelCostProfile{ModelID: "gemini-ultra", Tier: TierHeavy, CostPer1k: 0.020}); err != nil {
		t.Fatalf("register ultra cost failed: %v", err)
	}
	if err := r.RegisterModelCost(ModelCostProfile{ModelID: "fallback-standard", Tier: TierStandard, CostPer1k: 0.005}); err != nil {
		t.Fatalf("register fallback cost failed: %v", err)
	}

	if err := r.RegisterRule(RoutingRule{
		ScenarioID:   "code-synthesis",
		MinPassRate:  0.80,
		MaxVariance:  0.10,
		MaxLatencyMs: 2000.0,
		FallbackTier: TierStandard,
	}); err != nil {
		t.Fatalf("register rule failed: %v", err)
	}

	return r
}

func TestCycle72_AdaptiveRouter_StaticFloorInvariants(t *testing.T) {
	// 1. Invalid budget rejection
	_, err := NewAdaptiveRouter(0, TierStandard, "fallback")
	if !errors.Is(err, ErrInvalidBudget) {
		t.Errorf("expected ErrInvalidBudget for 0 budget, got %v", err)
	}

	_, errNeg := NewAdaptiveRouter(-100, TierStandard, "fallback")
	if !errors.Is(errNeg, ErrInvalidBudget) {
		t.Errorf("expected ErrInvalidBudget for negative budget, got %v", errNeg)
	}

	// 2. Invalid tier rejection
	_, errTier := NewAdaptiveRouter(1000, ModelTier("invalid-tier"), "fallback")
	if !errors.Is(errTier, ErrInvalidTier) {
		t.Errorf("expected ErrInvalidTier, got %v", errTier)
	}

	// 3. Rule validation
	badRule := RoutingRule{ScenarioID: "", MinPassRate: 0.5, MaxLatencyMs: 100}
	if err := badRule.Validate(); !errors.Is(err, ErrInvalidRule) {
		t.Errorf("expected ErrInvalidRule for blank scenario_id, got %v", err)
	}

	badRateRule := RoutingRule{ScenarioID: "sc", MinPassRate: 1.5, MaxLatencyMs: 100}
	if err := badRateRule.Validate(); !errors.Is(err, ErrInvalidRule) {
		t.Errorf("expected ErrInvalidRule for out-of-range pass rate, got %v", err)
	}
}

func TestCycle72_AdaptiveRouter_OperationalDynamicProof(t *testing.T) {
	ctx := context.Background()
	router := setupTestRouter(t, 10000)

	// Populate telemetry: both flash and pro meet constraints, flash is cheaper
	router.UpdateTelemetry(ModelTelemetry{
		ModelID:   "gemini-flash",
		PassRate:  0.85,
		Variance:  0.05,
		LatencyMs: 500.0,
		Degraded:  false,
	})
	router.UpdateTelemetry(ModelTelemetry{
		ModelID:   "gemini-pro",
		PassRate:  0.95,
		Variance:  0.02,
		LatencyMs: 1200.0,
		Degraded:  false,
	})

	resp, err := router.RouteSession(ctx, AdaptiveSessionRequest{
		SessionID:       "CVS-001",
		ScenarioID:      "code-synthesis",
		EstimatedTokens: 2000,
	})
	if err != nil {
		t.Fatalf("unexpected routing error: %v", err)
	}

	// Flash must be selected as cost-optimal
	if resp.SelectedModel != "gemini-flash" {
		t.Errorf("expected gemini-flash selected for cost optimality, got %s", resp.SelectedModel)
	}
	if resp.SelectedTier != TierFast {
		t.Errorf("expected TierFast, got %s", resp.SelectedTier)
	}
	if resp.FallbackUsed {
		t.Error("expected fallback not used")
	}
	if resp.TokensUsed != 2000 {
		t.Errorf("expected 2000 tokens used, got %d", resp.TokensUsed)
	}
	if router.BudgetRemaining() != 8000 {
		t.Errorf("expected 8000 budget remaining, got %d", router.BudgetRemaining())
	}
}

func TestCycle72_AdaptiveRouter_NegativeAdversarialBoundary(t *testing.T) {
	ctx := context.Background()
	router := setupTestRouter(t, 2500)

	// 1. Degradation triggers fallback
	router.UpdateTelemetry(ModelTelemetry{
		ModelID:   "gemini-flash",
		PassRate:  0.50, // Below threshold
		Variance:  0.05,
		LatencyMs: 500.0,
		Degraded:  false,
	})
	router.UpdateTelemetry(ModelTelemetry{
		ModelID:   "gemini-pro",
		PassRate:  0.95,
		Variance:  0.02,
		LatencyMs: 1200.0,
		Degraded:  true, // Degraded
	})

	resp, err := router.RouteSession(ctx, AdaptiveSessionRequest{
		SessionID:       "CVS-002",
		ScenarioID:      "code-synthesis",
		EstimatedTokens: 1000,
	})
	if err != nil {
		t.Fatalf("unexpected routing error on fallback: %v", err)
	}
	if !resp.FallbackUsed {
		t.Error("expected fallback model to be utilized")
	}
	if resp.SelectedModel != "fallback-standard" {
		t.Errorf("expected fallback-standard model, got %s", resp.SelectedModel)
	}

	// 2. Budget breach triggers ErrBudgetExhausted
	// Used so far: 1000. Total budget: 2500. Remaining: 1500.
	// Requesting 2000 tokens must fail closed.
	_, exhaustErr := router.RouteSession(ctx, AdaptiveSessionRequest{
		SessionID:       "CVS-003",
		ScenarioID:      "code-synthesis",
		EstimatedTokens: 2000,
	})
	if !errors.Is(exhaustErr, ErrBudgetExhausted) {
		t.Errorf("expected ErrBudgetExhausted, got %v", exhaustErr)
	}
}
