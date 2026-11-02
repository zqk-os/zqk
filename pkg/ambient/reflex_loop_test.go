package ambient

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestReflexConstants(t *testing.T) {
	if DefaultReflexCapacity != 256 {
		t.Fatalf("expected DefaultReflexCapacity=256, got %d", DefaultReflexCapacity)
	}
	if DefaultReflexHistoryLimit != 256 {
		t.Fatalf("expected DefaultReflexHistoryLimit=256, got %d", DefaultReflexHistoryLimit)
	}
	if DefaultReflexTimeout != 5*time.Second {
		t.Fatalf("expected DefaultReflexTimeout=5s, got %v", DefaultReflexTimeout)
	}
	if DefaultReflexDebounceInterval != 1*time.Second {
		t.Fatalf("expected DefaultReflexDebounceInterval=1s, got %v", DefaultReflexDebounceInterval)
	}

	if ReflexActionSweepBranches != "sweep_branches" {
		t.Fatalf("unexpected ReflexActionSweepBranches: %s", ReflexActionSweepBranches)
	}
	if ReflexActionReplenishIntent != "replenish_intent" {
		t.Fatalf("unexpected ReflexActionReplenishIntent: %s", ReflexActionReplenishIntent)
	}
	if ReflexActionQuarantineHolon != "quarantine_holon" {
		t.Fatalf("unexpected ReflexActionQuarantineHolon: %s", ReflexActionQuarantineHolon)
	}
	if ReflexActionEmitAlert != "emit_alert" {
		t.Fatalf("unexpected ReflexActionEmitAlert: %s", ReflexActionEmitAlert)
	}

	if ReflexTriggerDegradedHealth != "degraded_health" {
		t.Fatalf("unexpected ReflexTriggerDegradedHealth: %s", ReflexTriggerDegradedHealth)
	}
	if ReflexTriggerCriticalHealth != "critical_health" {
		t.Fatalf("unexpected ReflexTriggerCriticalHealth: %s", ReflexTriggerCriticalHealth)
	}
	if ReflexTriggerDivergenceExceeded != "divergence_exceeded" {
		t.Fatalf("unexpected ReflexTriggerDivergenceExceeded: %s", ReflexTriggerDivergenceExceeded)
	}
}

func TestReflexEvent_String(t *testing.T) {
	evt := ReflexEvent{
		ID:      "evt-123",
		Trigger: ReflexTriggerCriticalHealth,
		Action:  ReflexActionQuarantineHolon,
		Reason:  "holon crashed",
	}
	str := evt.String()
	if str == "" {
		t.Fatal("expected non-empty string for ReflexEvent.String()")
	}
}

func TestNewSpinalCordReflexEngine_Defaults(t *testing.T) {
	engine := NewSpinalCordReflexEngine()
	if engine == nil {
		t.Fatal("expected non-nil engine")
	}
	if engine.Capacity() != DefaultReflexCapacity {
		t.Fatalf("expected capacity %d, got %d", DefaultReflexCapacity, engine.Capacity())
	}
	if engine.Timeout() != DefaultReflexTimeout {
		t.Fatalf("expected timeout %v, got %v", DefaultReflexTimeout, engine.Timeout())
	}
	if engine.DebounceInterval() != DefaultReflexDebounceInterval {
		t.Fatalf("expected debounce %v, got %v", DefaultReflexDebounceInterval, engine.DebounceInterval())
	}
	if engine.HistoryLimit() != DefaultReflexHistoryLimit {
		t.Fatalf("expected history limit %d, got %d", DefaultReflexHistoryLimit, engine.HistoryLimit())
	}
	if engine.IsClosed() {
		t.Fatal("expected engine not closed")
	}
	if len(engine.History()) != 0 {
		t.Fatal("expected initial history to be empty")
	}
}

func TestNewSpinalCordReflexEngine_Options(t *testing.T) {
	mockHandler := func(ctx context.Context, evt ReflexEvent) error { return nil }

	engine := NewSpinalCordReflexEngine(
		nil,
		WithReflexCapacity(512),
		WithReflexCapacity(-1), // ignored
		WithReflexTimeout(10*time.Second),
		WithReflexTimeout(-1), // ignored
		WithReflexDebounceInterval(2*time.Second),
		WithReflexDebounceInterval(-1), // ignored
		WithReflexHistoryLimit(100),
		WithReflexHistoryLimit(-1), // ignored
		WithReflexHandler("custom_action", mockHandler),
		WithReflexHandler("", mockHandler),    // ignored
		WithReflexHandler("bad_handler", nil), // ignored
	)

	if engine.Capacity() != 512 {
		t.Fatalf("expected capacity 512, got %d", engine.Capacity())
	}
	if engine.Timeout() != 10*time.Second {
		t.Fatalf("expected timeout 10s, got %v", engine.Timeout())
	}
	if engine.DebounceInterval() != 2*time.Second {
		t.Fatalf("expected debounce 2s, got %v", engine.DebounceInterval())
	}
	if engine.HistoryLimit() != 100 {
		t.Fatalf("expected history limit 100, got %d", engine.HistoryLimit())
	}

	handler, exists := engine.GetHandler("custom_action")
	if !exists || handler == nil {
		t.Fatal("expected custom_action handler to be registered")
	}
}

func TestRegisterHandler_Validation(t *testing.T) {
	engine := NewSpinalCordReflexEngine()
	mockHandler := func(ctx context.Context, evt ReflexEvent) error { return nil }

	// Empty action
	if err := engine.RegisterHandler("", mockHandler); !errors.Is(err, ErrEmptyActionType) {
		t.Fatalf("expected ErrEmptyActionType, got %v", err)
	}
	if err := engine.RegisterHandler("   ", mockHandler); !errors.Is(err, ErrEmptyActionType) {
		t.Fatalf("expected ErrEmptyActionType for whitespace, got %v", err)
	}

	// Nil handler
	if err := engine.RegisterHandler("act1", nil); !errors.Is(err, ErrNilActionHandler) {
		t.Fatalf("expected ErrNilActionHandler, got %v", err)
	}

	// Valid registration
	if err := engine.RegisterHandler("act1", mockHandler); err != nil {
		t.Fatalf("unexpected error registering handler: %v", err)
	}

	// Duplicate registration
	if err := engine.RegisterHandler("act1", mockHandler); !errors.Is(err, ErrDuplicateHandler) {
		t.Fatalf("expected ErrDuplicateHandler, got %v", err)
	}

	// Registered actions listing
	actions := engine.RegisteredActions()
	if len(actions) != 1 || actions[0] != "act1" {
		t.Fatalf("expected actions [act1], got %v", actions)
	}

	// Closed engine
	_ = engine.Close()
	if err := engine.RegisterHandler("act2", mockHandler); !errors.Is(err, ErrReflexEngineClosed) {
		t.Fatalf("expected ErrReflexEngineClosed, got %v", err)
	}
}

func TestUnregisterHandler(t *testing.T) {
	engine := NewSpinalCordReflexEngine()
	mockHandler := func(ctx context.Context, evt ReflexEvent) error { return nil }

	_ = engine.RegisterHandler("act1", mockHandler)

	// Empty action
	if err := engine.UnregisterHandler(""); !errors.Is(err, ErrEmptyActionType) {
		t.Fatalf("expected ErrEmptyActionType, got %v", err)
	}

	// Non-existent action
	if err := engine.UnregisterHandler("non_existent"); !errors.Is(err, ErrHandlerNotFound) {
		t.Fatalf("expected ErrHandlerNotFound, got %v", err)
	}

	// Valid unregister
	if err := engine.UnregisterHandler("act1"); err != nil {
		t.Fatalf("unexpected error unregistering: %v", err)
	}
	if _, exists := engine.GetHandler("act1"); exists {
		t.Fatal("expected act1 to be deleted")
	}

	// Closed engine
	_ = engine.Close()
	if err := engine.UnregisterHandler("act1"); !errors.Is(err, ErrReflexEngineClosed) {
		t.Fatalf("expected ErrReflexEngineClosed on closed engine, got %v", err)
	}
}

func TestRegisterRule_Validation(t *testing.T) {
	engine := NewSpinalCordReflexEngine()
	mockPredicate := func(s *MiddleBrainTelemetrySnapshot) bool { return true }

	if err := engine.RegisterRule("", "act", "reason", mockPredicate); !errors.Is(err, ErrEmptyTrigger) {
		t.Fatalf("expected ErrEmptyTrigger, got %v", err)
	}
	if err := engine.RegisterRule("trig", "", "reason", mockPredicate); !errors.Is(err, ErrEmptyActionType) {
		t.Fatalf("expected ErrEmptyActionType, got %v", err)
	}
	if err := engine.RegisterRule("trig", "act", "reason", nil); !errors.Is(err, ErrNilPredicate) {
		t.Fatalf("expected ErrNilPredicate, got %v", err)
	}

	if err := engine.RegisterRule("trig", "act", "reason", mockPredicate); err != nil {
		t.Fatalf("unexpected error registering rule: %v", err)
	}

	_ = engine.Close()
	if err := engine.RegisterRule("trig", "act", "reason", mockPredicate); !errors.Is(err, ErrReflexEngineClosed) {
		t.Fatalf("expected ErrReflexEngineClosed, got %v", err)
	}
}

func TestEvaluateSnapshot_ValidationAndContext(t *testing.T) {
	engine := NewSpinalCordReflexEngine()

	// Nil snapshot
	events, err := engine.EvaluateSnapshot(context.Background(), nil)
	if !errors.Is(err, ErrNilSnapshot) || events != nil {
		t.Fatalf("expected ErrNilSnapshot, got events=%v, err=%v", events, err)
	}

	// Canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	snap := &MiddleBrainTelemetrySnapshot{HealthStatus: TelemetryHealthHealthy}
	events, err = engine.EvaluateSnapshot(ctx, snap)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// Closed engine
	_ = engine.Close()
	events, err = engine.EvaluateSnapshot(context.Background(), snap)
	if !errors.Is(err, ErrReflexEngineClosed) {
		t.Fatalf("expected ErrReflexEngineClosed, got %v", err)
	}
}

func TestEvaluateSnapshot_DefaultRules(t *testing.T) {
	engine := NewSpinalCordReflexEngine(WithReflexDebounceInterval(0))

	// 1. Healthy snapshot -> zero events
	healthySnap := &MiddleBrainTelemetrySnapshot{
		HealthStatus:     TelemetryHealthHealthy,
		ActiveHolonCount: 2,
	}
	events, err := engine.EvaluateSnapshot(nil, healthySnap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events for healthy snapshot, got %d", len(events))
	}

	// 2. Degraded snapshot -> ReflexActionReplenishIntent
	degradedSnap := &MiddleBrainTelemetrySnapshot{
		HealthStatus:     TelemetryHealthDegraded,
		ActiveHolonCount: 1,
		WarningCount:     1,
		Warnings:         []string{"stream lagged"},
	}
	events, err = engine.EvaluateSnapshot(context.Background(), degradedSnap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 || events[0].Action != ReflexActionReplenishIntent {
		t.Fatalf("expected 1 ReflexActionReplenishIntent event, got %v", events)
	}

	// 3. Divergence exceeded snapshot -> ReflexActionSweepBranches
	divergenceSnap := &MiddleBrainTelemetrySnapshot{
		HealthStatus:    TelemetryHealthHealthy,
		DivergenceCount: MaxAllowedDivergences + 2,
	}
	events, err = engine.EvaluateSnapshot(context.Background(), divergenceSnap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 || events[0].Action != ReflexActionSweepBranches {
		t.Fatalf("expected 1 ReflexActionSweepBranches event, got %v", events)
	}

	// 4. Critical snapshot with failed holon -> ReflexActionQuarantineHolon targeting that holon
	criticalSnapFailed := &MiddleBrainTelemetrySnapshot{
		HealthStatus: TelemetryHealthCritical,
		Holons: []HolonRecord{
			{ID: "holon-clean", Purpose: "clean", Phase: HolonPhaseRunning},
			{ID: "holon-failed", Purpose: "flaky", Phase: HolonPhaseFailed},
		},
	}
	events, err = engine.EvaluateSnapshot(context.Background(), criticalSnapFailed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 || events[0].Action != ReflexActionQuarantineHolon {
		t.Fatalf("expected ReflexActionQuarantineHolon, got %v", events)
	}
	if events[0].HolonID != "holon-failed" {
		t.Fatalf("expected HolonID 'holon-failed', got %s", events[0].HolonID)
	}

	// 5. Critical snapshot with stopped holon
	criticalSnapStopped := &MiddleBrainTelemetrySnapshot{
		HealthStatus: TelemetryHealthCritical,
		Holons: []HolonRecord{
			{ID: "holon-stopped", Purpose: "dead", Phase: HolonPhaseStopped},
		},
	}
	events, err = engine.EvaluateSnapshot(context.Background(), criticalSnapStopped)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 || events[0].HolonID != "holon-stopped" {
		t.Fatalf("expected HolonID 'holon-stopped', got %v", events)
	}

	// 6. Critical snapshot without holons
	criticalSnapNoHolons := &MiddleBrainTelemetrySnapshot{
		HealthStatus: TelemetryHealthCritical,
	}
	events, err = engine.EvaluateSnapshot(context.Background(), criticalSnapNoHolons)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 || events[0].HolonID != "" {
		t.Fatalf("expected empty HolonID, got %s", events[0].HolonID)
	}
}

func TestEvaluateSnapshot_DebounceAndReset(t *testing.T) {
	debounce := 500 * time.Millisecond
	engine := NewSpinalCordReflexEngine(WithReflexDebounceInterval(debounce))

	degradedSnap := &MiddleBrainTelemetrySnapshot{
		HealthStatus: TelemetryHealthDegraded,
		WarningCount: 1,
	}

	// First evaluation fires
	events1, err := engine.EvaluateSnapshot(context.Background(), degradedSnap)
	if err != nil || len(events1) != 1 {
		t.Fatalf("expected 1 event, got %d, err=%v", len(events1), err)
	}

	// Immediate evaluation within debounce window skips
	events2, err := engine.EvaluateSnapshot(context.Background(), degradedSnap)
	if err != nil || len(events2) != 0 {
		t.Fatalf("expected 0 events (debounced), got %d, err=%v", len(events2), err)
	}

	// ResetDebounce for specific trigger
	engine.ResetDebounce(ReflexTriggerDegradedHealth)
	events3, err := engine.EvaluateSnapshot(context.Background(), degradedSnap)
	if err != nil || len(events3) != 1 {
		t.Fatalf("expected 1 event after trigger reset, got %d", len(events3))
	}

	// ResetDebounce all
	engine.ResetDebounce("")
	events4, err := engine.EvaluateSnapshot(context.Background(), degradedSnap)
	if err != nil || len(events4) != 1 {
		t.Fatalf("expected 1 event after full reset, got %d", len(events4))
	}
}

func TestExecuteAction(t *testing.T) {
	engine := NewSpinalCordReflexEngine(WithReflexTimeout(50 * time.Millisecond))

	var handledEvt ReflexEvent
	handlerCalled := false
	_ = engine.RegisterHandler(ReflexActionReplenishIntent, func(ctx context.Context, evt ReflexEvent) error {
		handlerCalled = true
		handledEvt = evt
		return nil
	})

	_ = engine.RegisterHandler("failing_action", func(ctx context.Context, evt ReflexEvent) error {
		return errors.New("handler failed intentionally")
	})

	_ = engine.RegisterHandler("slow_action", func(ctx context.Context, evt ReflexEvent) error {
		select {
		case <-time.After(100 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	// Canceled context
	ctxCanceled, cancel := context.WithCancel(context.Background())
	cancel()
	err := engine.ExecuteAction(ctxCanceled, ReflexEvent{Action: ReflexActionReplenishIntent})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// Handler not found
	err = engine.ExecuteAction(context.Background(), ReflexEvent{Action: "unknown_act"})
	if !errors.Is(err, ErrHandlerNotFound) {
		t.Fatalf("expected ErrHandlerNotFound, got %v", err)
	}

	// Successful execution
	err = engine.ExecuteAction(nil, ReflexEvent{Action: ReflexActionReplenishIntent, ID: "test-1"})
	if err != nil || !handlerCalled || handledEvt.ID != "test-1" {
		t.Fatalf("unexpected execution result: err=%v, called=%v, evt=%v", err, handlerCalled, handledEvt)
	}

	// Failing action
	err = engine.ExecuteAction(context.Background(), ReflexEvent{Action: "failing_action"})
	if err == nil || err.Error() != "handler failed intentionally" {
		t.Fatalf("expected intentional error, got %v", err)
	}

	// Slow action timeout
	err = engine.ExecuteAction(context.Background(), ReflexEvent{Action: "slow_action"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}

	// Closed engine
	_ = engine.Close()
	err = engine.ExecuteAction(context.Background(), ReflexEvent{Action: ReflexActionReplenishIntent})
	if !errors.Is(err, ErrReflexEngineClosed) {
		t.Fatalf("expected ErrReflexEngineClosed, got %v", err)
	}
}

func TestEmitAction_And_CapacityExhaustion(t *testing.T) {
	engine := NewSpinalCordReflexEngine(WithReflexCapacity(1))

	evt1 := ReflexEvent{ID: "e1", Action: ReflexActionEmitAlert}
	evt2 := ReflexEvent{ID: "e2", Action: ReflexActionEmitAlert}

	// First emit succeeds
	if err := engine.EmitAction(evt1); err != nil {
		t.Fatalf("unexpected emit error: %v", err)
	}

	// Channel buffer is now full (cap=1). Next emit fails closed with ErrReflexChannelExhausted
	if err := engine.EmitAction(evt2); !errors.Is(err, ErrReflexChannelExhausted) {
		t.Fatalf("expected ErrReflexChannelExhausted, got %v", err)
	}

	// Read from ActionChannel
	ch := engine.ActionChannel()
	readEvt := <-ch
	if readEvt.ID != "e1" {
		t.Fatalf("expected e1, got %s", readEvt.ID)
	}

	// Close engine
	_ = engine.Close()
	if err := engine.EmitAction(evt1); !errors.Is(err, ErrReflexEngineClosed) {
		t.Fatalf("expected ErrReflexEngineClosed, got %v", err)
	}
}

func TestHistory_RingBufferLimit(t *testing.T) {
	engine := NewSpinalCordReflexEngine(
		WithReflexHistoryLimit(2),
		WithReflexDebounceInterval(0),
	)

	// Custom rule to fire repeatedly
	_ = engine.RegisterRule("always", "action", "testing", func(s *MiddleBrainTelemetrySnapshot) bool { return true })

	snap := &MiddleBrainTelemetrySnapshot{}
	_, _ = engine.EvaluateSnapshot(context.Background(), snap)
	_, _ = engine.EvaluateSnapshot(context.Background(), snap)
	_, _ = engine.EvaluateSnapshot(context.Background(), snap)

	history := engine.History()
	if len(history) != 2 {
		t.Fatalf("expected history limited to 2 items, got %d", len(history))
	}
}

func TestSpinalCordReflexEngine_CloseIdempotent(t *testing.T) {
	engine := NewSpinalCordReflexEngine()
	if err := engine.Close(); err != nil {
		t.Fatalf("unexpected error closing engine: %v", err)
	}
	// Second close should be safe and idempotent
	if err := engine.Close(); err != nil {
		t.Fatalf("unexpected error on second close: %v", err)
	}
	if !engine.IsClosed() {
		t.Fatal("expected IsClosed to be true")
	}
}

func TestSpinalCordReflexEngine_Concurrency(t *testing.T) {
	engine := NewSpinalCordReflexEngine(
		WithReflexCapacity(100),
		WithReflexDebounceInterval(0),
	)

	_ = engine.RegisterHandler(ReflexActionReplenishIntent, func(ctx context.Context, evt ReflexEvent) error {
		return nil
	})

	var wg sync.WaitGroup
	workers := 10

	for i := 0; i < workers; i++ {
		wg.Add(1)
		workerID := i
		goroutinelabels.NewGoroutine("test_reflex_worker", "concurrent reflex worker test").StartSimple(func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				snap := &MiddleBrainTelemetrySnapshot{
					HealthStatus: TelemetryHealthDegraded,
				}
				events, err := engine.EvaluateSnapshot(context.Background(), snap)
				if err == nil && len(events) > 0 {
					_ = engine.ExecuteAction(context.Background(), events[0])
				}
				_ = engine.History()
				_ = engine.RegisteredActions()
				_ = workerID
			}
		})
	}

	wg.Wait()
	_ = engine.Close()
}
