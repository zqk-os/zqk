package ambient

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestAmbientEngine_NegativeBoundaries(t *testing.T) {
	t.Run("ConcurrencyStress_60WorkerBurst", func(t *testing.T) {
		engine, err := DefaultAmbientEngine()
		if err != nil {
			t.Fatalf("failed to create default ambient engine: %v", err)
		}
		if err := engine.Transition(EngineStateStarting); err != nil {
			t.Fatalf("failed transition to starting: %v", err)
		}
		if err := engine.Transition(EngineStateRunning); err != nil {
			t.Fatalf("failed transition to running: %v", err)
		}

		const numWorkers = 60
		const iterationsPerWorker = 100
		var wg sync.WaitGroup
		wg.Add(numWorkers)

		var totalAdmitted int64
		var totalRejected int64

		for w := 0; w < numWorkers; w++ {
			workerID := w
			goroutinelabels.NewGoroutine("test_burst_worker", "concurrent 60-worker event burst stress").StartSimple(func() {
				defer wg.Done()
				for i := 0; i < iterationsPerWorker; i++ {
					// Query state and running status
					st := engine.State()
					_ = st
					_ = engine.IsRunning()

					// Safe execution boundary under concurrent load
					engine.SafeExecute(func() {
						_ = engine.IsRunning()
					})

					// Submit intent event
					evt := IntentEvent{
						ID:        fmt.Sprintf("evt-burst-%d-%d", workerID, i),
						AgentID:   fmt.Sprintf("agent-%d", workerID),
						Intent:    "stress.burst.test",
						Payload:   `{"status":"active"}`,
						Timestamp: time.Now().UTC(),
					}
					err := engine.SubmitIntent(evt)
					if err == nil {
						atomic.AddInt64(&totalAdmitted, 1)
					} else {
						atomic.AddInt64(&totalRejected, 1)
					}
				}
			})
		}

		wg.Wait()

		admitted := atomic.LoadInt64(&totalAdmitted)
		rejected := atomic.LoadInt64(&totalRejected)
		if admitted == 0 {
			t.Errorf("expected at least some events admitted during burst, got %d (rejected: %d)", admitted, rejected)
		}
		if admitted+rejected != numWorkers*iterationsPerWorker {
			t.Errorf("expected %d total attempts, got %d", numWorkers*iterationsPerWorker, admitted+rejected)
		}
	})

	t.Run("PanicContainment", func(t *testing.T) {
		engine, err := DefaultAmbientEngine()
		if err != nil {
			t.Fatalf("failed to create default ambient engine: %v", err)
		}
		if err := engine.Transition(EngineStateStarting); err != nil {
			t.Fatalf("failed transition to starting: %v", err)
		}
		if err := engine.Transition(EngineStateRunning); err != nil {
			t.Fatalf("failed transition to running: %v", err)
		}

		// SafeExecute without panic
		noPanic := engine.SafeExecute(func() {
			// benign operation
		})
		if noPanic != nil {
			t.Fatalf("expected nil recovered, got %v", noPanic)
		}
		if engine.State() != EngineStateRunning {
			t.Fatalf("expected engine state Running, got %s", engine.State())
		}

		// SafeExecute with injected panic
		const panicMsg = "simulated worker goroutine panic"
		recovered := engine.SafeExecute(func() {
			panic(panicMsg)
		})
		if recovered == nil {
			t.Fatal("expected recovered panic to be non-nil")
		}
		if str, ok := recovered.(string); !ok || str != panicMsg {
			t.Fatalf("expected recovered message %q, got %v", panicMsg, recovered)
		}

		// Engine state must flip to Degraded
		if engine.State() != EngineStateDegraded {
			t.Fatalf("expected engine state to flip to Degraded, got %s", engine.State())
		}
		if engine.IsRunning() {
			t.Fatal("expected IsRunning() false when degraded")
		}

		// Submitting intents when degraded fails closed
		validEvt := IntentEvent{
			ID:        "evt-degraded-check",
			AgentID:   "agent-1",
			Intent:    "task.execution",
			Timestamp: time.Now().UTC(),
		}
		if err := engine.SubmitIntent(validEvt); !errors.Is(err, ErrEngineStopped) {
			t.Fatalf("expected ErrEngineStopped when degraded, got %v", err)
		}

		// Recover transitions back to Running
		if err := engine.Recover(); err != nil {
			t.Fatalf("Recover() failed: %v", err)
		}
		if engine.State() != EngineStateRunning {
			t.Fatalf("expected state Running after Recover(), got %s", engine.State())
		}
		if !engine.IsRunning() {
			t.Fatal("expected IsRunning() true after Recover()")
		}

		// Recover when already running is a no-op
		if err := engine.Recover(); err != nil {
			t.Fatalf("Recover() when already running returned error: %v", err)
		}
	})

	t.Run("BoundaryFailureEnforcements", func(t *testing.T) {
		validEvt := IntentEvent{
			ID:        "evt-valid-1",
			AgentID:   "agent-1",
			Intent:    "valid.intent",
			Timestamp: time.Now().UTC(),
		}

		// SubmitIntent on nil engine
		var nilEngine *AmbientEngine
		if err := nilEngine.SubmitIntent(validEvt); !errors.Is(err, ErrNilEngine) {
			t.Fatalf("expected ErrNilEngine on nil engine, got %v", err)
		}

		// SubmitIntent on stopped engine
		engine, err := DefaultAmbientEngine()
		if err != nil {
			t.Fatalf("failed to create default engine: %v", err)
		}
		if err := engine.SubmitIntent(validEvt); !errors.Is(err, ErrEngineStopped) {
			t.Fatalf("expected ErrEngineStopped on stopped engine, got %v", err)
		}

		// Transition engine to running
		if err := engine.Transition(EngineStateStarting); err != nil {
			t.Fatalf("transition starting failed: %v", err)
		}
		if err := engine.Transition(EngineStateRunning); err != nil {
			t.Fatalf("transition running failed: %v", err)
		}

		// Empty event ID validation failure
		invalidIDEvt := IntentEvent{
			ID:        "",
			AgentID:   "agent-1",
			Intent:    "valid.intent",
			Timestamp: time.Now().UTC(),
		}
		if err := engine.SubmitIntent(invalidIDEvt); !errors.Is(err, ErrIntentEventIDRequired) {
			t.Fatalf("expected ErrIntentEventIDRequired, got %v", err)
		}

		// Empty event intent validation failure
		invalidIntentEvt := IntentEvent{
			ID:        "evt-valid-id",
			AgentID:   "agent-1",
			Intent:    "",
			Timestamp: time.Now().UTC(),
		}
		if err := engine.SubmitIntent(invalidIntentEvt); !errors.Is(err, ErrIntentEventIntentRequired) {
			t.Fatalf("expected ErrIntentEventIntentRequired, got %v", err)
		}

		// Fault isolator tripped rejects intents
		if err := engine.FaultIsolator().Trip(); err != nil {
			t.Fatalf("failed to trip fault isolator: %v", err)
		}
		if err := engine.SubmitIntent(validEvt); !errors.Is(err, ErrCircuitTripped) {
			t.Fatalf("expected ErrCircuitTripped when isolator tripped, got %v", err)
		}

		// Reset fault isolator permits operations again
		if err := engine.FaultIsolator().Reset(); err != nil {
			t.Fatalf("failed to reset fault isolator: %v", err)
		}

		// Backpressure regulator closed rejects intents with ErrEventRejected
		if err := engine.BackpressureRegulator().Close(); err != nil {
			t.Fatalf("failed to close backpressure regulator: %v", err)
		}
		if err := engine.SubmitIntent(validEvt); !errors.Is(err, ErrEventRejected) {
			t.Fatalf("expected ErrEventRejected when regulator rejects, got %v", err)
		}
	})

	t.Run("NilReceiverSafety", func(t *testing.T) {
		var nilEngine *AmbientEngine

		// State
		if st := nilEngine.State(); st != EngineStateStopped {
			t.Errorf("expected EngineStateStopped for nil, got %s", st)
		}

		// IsRunning
		if nilEngine.IsRunning() {
			t.Error("expected IsRunning() false for nil")
		}

		// Recover
		if err := nilEngine.Recover(); !errors.Is(err, ErrNilEngine) {
			t.Errorf("expected ErrNilEngine on Recover(), got %v", err)
		}

		// SubmitIntent
		validEvt := IntentEvent{
			ID:        "evt-nil-safety",
			AgentID:   "agent-1",
			Intent:    "valid.intent",
			Timestamp: time.Now().UTC(),
		}
		if err := nilEngine.SubmitIntent(validEvt); !errors.Is(err, ErrNilEngine) {
			t.Errorf("expected ErrNilEngine on SubmitIntent(), got %v", err)
		}

		// SafeExecute nil receiver benign
		rec := nilEngine.SafeExecute(func() {})
		if rec != nil {
			t.Errorf("expected nil recovered, got %v", rec)
		}

		// SafeExecute nil receiver with panic
		recPanic := nilEngine.SafeExecute(func() {
			panic("nil engine isolated panic")
		})
		if recPanic == nil || recPanic != "nil engine isolated panic" {
			t.Errorf("expected caught panic %q, got %v", "nil engine isolated panic", recPanic)
		}

		// SafeExecute nil func
		recNilFunc := nilEngine.SafeExecute(nil)
		if recNilFunc != nil {
			t.Errorf("expected nil recovered for nil func, got %v", recNilFunc)
		}
	})
}
