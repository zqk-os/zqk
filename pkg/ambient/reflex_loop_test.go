package ambient

import (
	"context"
	"testing"
	"time"
)

// TestSpinalCordReflex_StaticFloorInvariants satisfies CRIT-1793593447556743000-f02109c7
// (Spinal Cord Reflex Engine Invariant Typing and Action Contracts).
func TestSpinalCordReflex_StaticFloorInvariants(t *testing.T) {
	// 1. Verify capacity constants invariants
	if DefaultReflexActionQueueCapacity <= 0 {
		t.Fatalf("expected DefaultReflexActionQueueCapacity > 0, got %d", DefaultReflexActionQueueCapacity)
	}
	if DefaultReflexEvaluationTimeout <= 0 {
		t.Fatalf("expected DefaultReflexEvaluationTimeout > 0, got %v", DefaultReflexEvaluationTimeout)
	}
	if DefaultMaxTriggerRules <= 0 {
		t.Fatalf("expected DefaultMaxTriggerRules > 0, got %d", DefaultMaxTriggerRules)
	}
	if DefaultMaxActionHistory <= 0 {
		t.Fatalf("expected DefaultMaxActionHistory > 0, got %d", DefaultMaxActionHistory)
	}

	// 2. Verify typed action contracts
	expectedActions := map[ReflexActionType]string{
		ReflexActionRealignIntent: "realign_intent",
		ReflexActionPruneBranch:   "prune_branch",
		ReflexActionRestartHolon:  "restart_holon",
		ReflexActionDrainStream:   "drain_stream",
		ReflexActionQuiesceEngine: "quiesce_engine",
	}
	for actionType, expectedStr := range expectedActions {
		if string(actionType) != expectedStr {
			t.Fatalf("expected %s, got %s", expectedStr, actionType)
		}
	}

	// 3. Nil-safe receiver methods (zero panic guarantee)
	testSpinalCordReflexNilSafety(t)

	// 4. Engine options and boundary validation
	testSpinalCordReflexOptionsAndBoundaries(t)
}

func testSpinalCordReflexNilSafety(t *testing.T) {
	ctx := context.Background()
	var nilEngine *SpinalCordReflexEngine

	res, err := nilEngine.Evaluate(ctx, nil)
	if err != ErrNilReflexEngine {
		t.Fatalf("expected ErrNilReflexEngine, got %v", err)
	}
	if res == nil || res.Healthy {
		t.Fatalf("expected non-nil unhealthy evaluation from nil engine, got %+v", res)
	}

	dummyAction := ReflexAction{
		ID:        "act-001",
		Type:      ReflexActionRealignIntent,
		Target:    "holon-test",
		Reason:    "testing nil receiver",
		Timestamp: time.Now(),
	}
	if err := nilEngine.TriggerAction(ctx, dummyAction); err != ErrNilReflexEngine {
		t.Fatalf("expected ErrNilReflexEngine, got %v", err)
	}
	if err := nilEngine.RegisterRule(nil); err != ErrNilReflexEngine {
		t.Fatalf("expected ErrNilReflexEngine, got %v", err)
	}
	if err := nilEngine.RegisterHandler(ReflexActionRealignIntent, nil); err != ErrNilReflexEngine {
		t.Fatalf("expected ErrNilReflexEngine, got %v", err)
	}

	metrics := nilEngine.Metrics()
	if metrics.EvaluationsCount != 0 || metrics.ActionsTriggered != 0 {
		t.Fatalf("expected zero metrics for nil engine, got %+v", metrics)
	}
	if hist := nilEngine.History(); hist != nil {
		t.Fatalf("expected nil history for nil engine, got %v", hist)
	}
	if err := nilEngine.Close(); err != nil {
		t.Fatalf("expected nil error on nil Close, got %v", err)
	}
}

func testSpinalCordReflexOptionsAndBoundaries(t *testing.T) {
	ctx := context.Background()

	// Verify fallback when non-positive options are provided
	engine := NewSpinalCordReflexEngine(
		WithActionQueueCapacity(-10),
		WithEvaluationTimeout(-5*time.Second),
		WithTriggerRule(nil),
		WithActionHandler(ReflexActionRealignIntent, nil),
	)
	if engine == nil {
		t.Fatal("expected non-nil engine instance")
	}
	defer engine.Close()

	// Verify parameter validations
	if err := engine.RegisterRule(nil); err != ErrNilTriggerRule {
		t.Fatalf("expected ErrNilTriggerRule, got %v", err)
	}
	if err := engine.RegisterHandler("", func(context.Context, ReflexAction) error { return nil }); err != ErrEmptyActionType {
		t.Fatalf("expected ErrEmptyActionType, got %v", err)
	}
	if err := engine.RegisterHandler(ReflexActionRealignIntent, nil); err != ErrNilActionHandler {
		t.Fatalf("expected ErrNilActionHandler, got %v", err)
	}

	// Verify nil snapshot evaluation returns unhealthy reason without error
	res, err := engine.Evaluate(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error evaluating nil snapshot: %v", err)
	}
	if res == nil || res.Healthy || res.Reason == "" {
		t.Fatalf("expected unhealthy evaluation for nil snapshot, got %+v", res)
	}

	// Verify empty action type rejection
	if err := engine.TriggerAction(ctx, ReflexAction{}); err != ErrEmptyActionType {
		t.Fatalf("expected ErrEmptyActionType on empty action, got %v", err)
	}
}
