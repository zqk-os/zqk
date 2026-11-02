package ambient

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/validation/qa"
)

// TestSpinalCordReflex_StaticFloorInvariants satisfies CRIT-1793593447556743000-f02109c7
// (Spinal Cord Reflex Engine Invariant Typing and Action Contracts).
func TestSpinalCordReflex_StaticFloorInvariants(t *testing.T) {
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

	testSpinalCordReflexNilSafety(t)
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

	if err := engine.RegisterRule(nil); err != ErrNilTriggerRule {
		t.Fatalf("expected ErrNilTriggerRule, got %v", err)
	}
	if err := engine.RegisterHandler("", func(context.Context, ReflexAction) error { return nil }); err != ErrEmptyActionType {
		t.Fatalf("expected ErrEmptyActionType, got %v", err)
	}
	if err := engine.RegisterHandler(ReflexActionRealignIntent, nil); err != ErrNilActionHandler {
		t.Fatalf("expected ErrNilActionHandler, got %v", err)
	}

	res, err := engine.Evaluate(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error evaluating nil snapshot: %v", err)
	}
	if res == nil || res.Healthy || res.Reason == "" {
		t.Fatalf("expected unhealthy evaluation for nil snapshot, got %+v", res)
	}

	if err := engine.TriggerAction(ctx, ReflexAction{}); err != ErrEmptyActionType {
		t.Fatalf("expected ErrEmptyActionType on empty action, got %v", err)
	}
}

// TestSpinalCordReflex_OperationalDynamicProof satisfies CRIT-1793593453460187000-1f190376
// by verifying that middle-brain snapshots trigger deterministic compensating reflex actions.
func TestSpinalCordReflex_OperationalDynamicProof(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	verifyHealthyReflex(t, ctx)
	verifyDegradedReflex(t, ctx)
	verifySaturatedReflex(t, ctx)
}

func verifyHealthyReflex(t *testing.T, ctx context.Context) {
	t.Run("healthy_snapshot_produces_zero_action", func(t *testing.T) {
		snap := &HolonTelemetrySnapshot{
			Timestamp:               time.Now(),
			HealthStatus:            TelemetryHealthHealthy,
			ActiveHolonCount:        3,
			StreamHydratorDrops:     0,
			RiverConvergenceLatency: 10 * time.Millisecond,
			AlignmentScore:          0.95,
			DivergenceCount:         0,
		}

		decision, err := EvaluateSnapshot(snap)
		if err != nil {
			t.Fatalf("expected nil error on healthy snapshot, got %v", err)
		}
		if decision.RequiresAction || decision.Action != ActionNoop {
			t.Fatalf("expected ActionNoop without required action, got %s", decision.Action)
		}

		result, err := ExecuteDecision(ctx, decision)
		if err != nil {
			t.Fatalf("expected nil error on executing noop decision, got %v", err)
		}
		if !result.Success || result.Action != ActionNoop {
			t.Fatalf("expected successful ActionNoop result, got %s", result.Action)
		}
	})
}

func verifyDegradedReflex(t *testing.T, ctx context.Context) {
	t.Run("degraded_divergent_snapshot_produces_intent_realign", func(t *testing.T) {
		snap := &HolonTelemetrySnapshot{
			Timestamp:               time.Now(),
			HealthStatus:            TelemetryHealthDegraded,
			ActiveHolonCount:        2,
			StreamHydratorDrops:     2,
			RiverConvergenceLatency: 50 * time.Millisecond,
			AlignmentScore:          0.45,
			DivergenceCount:         6,
		}

		decision, err := EvaluateSnapshot(snap)
		if err != nil {
			t.Fatalf("expected nil error on degraded snapshot, got %v", err)
		}
		if !decision.RequiresAction || decision.Action != ActionIntentRealign {
			t.Fatalf("expected ActionIntentRealign, got %s", decision.Action)
		}

		result, err := ExecuteDecision(ctx, decision)
		if err != nil {
			t.Fatalf("expected nil error on executing realign decision, got %v", err)
		}
		if !result.Success || result.Action != ActionIntentRealign {
			t.Fatalf("expected ActionIntentRealign in result, got %s", result.Action)
		}
		if result.RecoveredState != TelemetryHealthHealthy {
			t.Fatalf("expected recovered state %s, got %s", TelemetryHealthHealthy, result.RecoveredState)
		}
	})
}

func verifySaturatedReflex(t *testing.T, ctx context.Context) {
	t.Run("saturated_river_produces_rate_throttle", func(t *testing.T) {
		snap := &HolonTelemetrySnapshot{
			Timestamp:               time.Now(),
			HealthStatus:            TelemetryHealthHealthy,
			ActiveHolonCount:        3,
			StreamHydratorDrops:     15,
			RiverConvergenceLatency: 750 * time.Millisecond,
			AlignmentScore:          0.90,
			DivergenceCount:         0,
		}

		decision, err := EvaluateSnapshot(snap)
		if err != nil {
			t.Fatalf("expected nil error on saturated snapshot, got %v", err)
		}
		if !decision.RequiresAction || decision.Action != ActionRateThrottle {
			t.Fatalf("expected ActionRateThrottle, got %s", decision.Action)
		}

		result, err := ExecuteDecision(ctx, decision)
		if err != nil {
			t.Fatalf("expected nil error on executing throttle decision, got %v", err)
		}
		if !result.Success || result.Action != ActionRateThrottle {
			t.Fatalf("expected ActionRateThrottle in result, got %s", result.Action)
		}
		if result.RecoveredState != TelemetryHealthHealthy {
			t.Fatalf("expected recovered state %s, got %s", TelemetryHealthHealthy, result.RecoveredState)
		}
	})
}

// TestSpinalCordReflex_BoundaryAndErrorHandling verifies nil-safety and cancellation resilience.
func TestSpinalCordReflex_BoundaryAndErrorHandling(t *testing.T) {
	decision, err := EvaluateSnapshot(nil)
	if err == nil || decision != nil {
		t.Fatalf("expected error on nil snapshot, got decision=%v, err=%v", decision, err)
	}

	result, err := ExecuteDecision(context.Background(), nil)
	if err == nil || result != nil {
		t.Fatalf("expected error on nil decision, got result=%v, err=%v", result, err)
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	validDecision := &ReflexDecision{Action: ActionNoop, RequiresAction: false}
	_, err = ExecuteDecision(canceledCtx, validDecision)
	if err == nil {
		t.Fatalf("expected context cancellation error, got nil")
	}

	if snap := FromMiddleBrainSnapshot(nil); snap != nil {
		t.Fatalf("expected nil from nil middle brain snapshot, got %v", snap)
	}
}

// TestSpinalCordReflex_NegativeAdversarialBoundaries satisfies CRIT-1793593457072661000-88195b95
// by verifying panic recovery, channel capacity exhaustion, context timeouts, and bounded history.
func TestSpinalCordReflex_NegativeAdversarialBoundaries(t *testing.T) {
	ctx := context.Background()

	testHandlerPanicRecovery(t, ctx)
	testCapacityExhaustion(t, ctx)
	testContextCancellationAndTimeout(t)
	testHistoryRingLimit(t, ctx)
}

func testHandlerPanicRecovery(t *testing.T, ctx context.Context) {
	engine := NewSpinalCordReflexEngine()
	defer engine.Close()

	panickingHandler := func(ctx context.Context, action ReflexAction) error {
		panic("deliberate action handler panic")
	}

	if err := engine.RegisterHandler(ReflexActionRestartHolon, panickingHandler); err != nil {
		t.Fatalf("unexpected error registering handler: %v", err)
	}

	action := ReflexAction{
		ID:        "act-panic",
		Type:      ReflexActionRestartHolon,
		Target:    "holon-faulty",
		Reason:    "panic recovery test",
		Timestamp: time.Now(),
	}

	err := engine.TriggerAction(ctx, action)
	if err == nil || !errors.Is(err, ErrActionHandlerPanicked) {
		t.Fatalf("expected ErrActionHandlerPanicked, got: %v", err)
	}

	metrics := engine.Metrics()
	if metrics.ActionsFailed < 1 {
		t.Fatalf("expected ActionsFailed >= 1, got %d", metrics.ActionsFailed)
	}
}

func testCapacityExhaustion(t *testing.T, ctx context.Context) {
	engine := NewSpinalCordReflexEngine(WithActionQueueCapacity(1))
	defer engine.Close()

	action1 := ReflexAction{
		ID:        "act-cap-1",
		Type:      ReflexActionPruneBranch,
		Target:    "branch-1",
		Reason:    "queue fill",
		Timestamp: time.Now(),
	}
	action2 := ReflexAction{
		ID:        "act-cap-2",
		Type:      ReflexActionPruneBranch,
		Target:    "branch-2",
		Reason:    "queue overflow",
		Timestamp: time.Now(),
	}

	if err := engine.TriggerAction(ctx, action1); err != nil {
		t.Fatalf("expected first action to succeed, got %v", err)
	}

	err := engine.TriggerAction(ctx, action2)
	if err == nil || !errors.Is(err, ErrActionQueueFull) {
		t.Fatalf("expected ErrActionQueueFull, got: %v", err)
	}
}

func testContextCancellationAndTimeout(t *testing.T) {
	engine := NewSpinalCordReflexEngine()
	defer engine.Close()

	startedCh := make(chan struct{})
	slowHandler := func(ctx context.Context, action ReflexAction) error {
		close(startedCh)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
			return nil
		}
	}

	if err := engine.RegisterHandler(ReflexActionDrainStream, slowHandler); err != nil {
		t.Fatalf("unexpected error registering slow handler: %v", err)
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	action := ReflexAction{
		ID:        "act-cancel",
		Type:      ReflexActionDrainStream,
		Target:    "stream-1",
		Reason:    "cancel test",
		Timestamp: time.Now(),
	}
	if err := engine.TriggerAction(canceledCtx, action); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	liveCtx, cancelLive := context.WithCancel(context.Background())
	goroutinelabels.NewGoroutine("test_mid_flight_cancel", "cancel context on event start").StartSimple(func() {
		<-startedCh
		cancelLive()
	})

	actionLive := ReflexAction{
		ID:        "act-live-cancel",
		Type:      ReflexActionDrainStream,
		Target:    "stream-2",
		Reason:    "mid-flight cancel",
		Timestamp: time.Now(),
	}
	if err := engine.TriggerAction(liveCtx, actionLive); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled on mid-flight cancel, got %v", err)
	}
}

func testHistoryRingLimit(t *testing.T, ctx context.Context) {
	engine := NewSpinalCordReflexEngine(WithActionQueueCapacity(50))
	defer engine.Close()

	for i := 0; i < 600; i++ {
		action := ReflexAction{
			ID:        "act-hist",
			Type:      ReflexActionQuiesceEngine,
			Target:    "engine",
			Reason:    "history ring test",
			Timestamp: time.Now(),
		}
		if err := engine.TriggerAction(ctx, action); err != nil && !errors.Is(err, ErrActionQueueFull) {
			t.Fatalf("unexpected error in history test: %v", err)
		}
	}

	history := engine.History()
	if len(history) > DefaultMaxActionHistory {
		t.Fatalf("expected history <= %d, got %d", DefaultMaxActionHistory, len(history))
	}
}

// TestSpinalCordReflex_ASTAudit verifies zero medium/high AST violations in reflex files.
func TestSpinalCordReflex_ASTAudit(t *testing.T) {
	auditor := qa.NewASTAuditor()
	for _, file := range []string{"reflex_loop.go", "reflex_loop_test.go"} {
		violations, err := auditor.AuditFile(file)
		if err != nil {
			t.Fatalf("failed to audit %s: %v", file, err)
		}

		for _, v := range violations {
			if v.Severity == "high" || v.Severity == "medium" {
				t.Errorf("AST violation [%s] in %s at %s: %s", v.Severity, file, v.Pos, v.Message)
			}
		}
	}
}
