package ambient

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/validation/qa"
)

func TestAmbientEngine_OperationalDynamicProof(t *testing.T) {
	t.Run("LifecycleAndSupervisorTicks", func(t *testing.T) {
		cfg := DefaultAmbientEngineConfig()
		cfg.TickInterval = 10 * time.Millisecond
		cfg.GracefulStopTimeout = 2 * time.Second

		engine, err := NewAmbientEngine(cfg)
		if err != nil {
			t.Fatalf("unexpected error creating engine: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// 1. Verify Start(ctx) transitions engine to EngineStateRunning and begins background ticks.
		if err := engine.Start(ctx); err != nil {
			t.Fatalf("failed to start engine: %v", err)
		}

		if engine.State() != EngineStateRunning {
			t.Fatalf("expected state %s, got %s", EngineStateRunning, engine.State())
		}
		if !engine.IsRunning() {
			t.Fatal("expected engine.IsRunning() to be true")
		}

		// Await ticks executed
		var status EngineStatus
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			status = engine.Status()
			if status.TicksExecuted > 0 {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}

		// 2. Verify Status() reports positive uptime and non-zero TicksExecuted.
		if status.TicksExecuted == 0 {
			t.Fatalf("expected non-zero TicksExecuted, got %d", status.TicksExecuted)
		}
		if status.Uptime <= 0 {
			t.Fatalf("expected positive uptime, got %v", status.Uptime)
		}
		if status.State != EngineStateRunning {
			t.Fatalf("expected status state %s, got %s", EngineStateRunning, status.State)
		}
		if status.Subcomponents == nil {
			t.Fatal("expected non-nil Subcomponents map")
		}
		if _, ok := status.Subcomponents["intent_stream"]; !ok {
			t.Fatal("expected subcomponent intent_stream")
		}
		if _, ok := status.Subcomponents["epoch_synchronizer"]; !ok {
			t.Fatal("expected subcomponent epoch_synchronizer")
		}
		if _, ok := status.Subcomponents["backpressure_regulator"]; !ok {
			t.Fatal("expected subcomponent backpressure_regulator")
		}
		if _, ok := status.Subcomponents["fault_isolator"]; !ok {
			t.Fatal("expected subcomponent fault_isolator")
		}

		// 3. Verify Telemetry() reflects valid aggregate holon metrics.
		// Push sample intent to verify pending intents and runway metrics.
		_ = engine.IntentStream().Push(IntentEvent{
			ID:     "intent-test-1",
			Intent: "execute task",
		})

		telem := engine.Telemetry()
		if telem.PendingIntents != 1 {
			t.Fatalf("expected 1 pending intent, got %d", telem.PendingIntents)
		}
		if telem.ActiveAgents < 1 {
			t.Fatalf("expected at least 1 active agent, got %d", telem.ActiveAgents)
		}
		if telem.RunwayDepth <= 0 {
			t.Fatalf("expected positive runway depth, got %f", telem.RunwayDepth)
		}
		if telem.CircuitState != StateClosed.String() {
			t.Fatalf("expected circuit state %s, got %s", StateClosed, telem.CircuitState)
		}
		if telem.FailureRate != 0.0 {
			t.Fatalf("expected zero failure rate, got %f", telem.FailureRate)
		}
		if telem.CurrentCapacity <= 0 {
			t.Fatalf("expected positive capacity, got %d", telem.CurrentCapacity)
		}

		// Verify TelemetryReport() constructs full telemetry report from live engine.
		report := engine.TelemetryReport()
		if report == nil {
			t.Fatal("expected non-nil telemetry report from running engine")
		}
		if report.GetActiveHolonsCount() != 6 {
			t.Fatalf("expected 6 active holons in report, got %d", report.GetActiveHolonsCount())
		}
		if report.GetCircuitBreakerStatus() != StateClosed.String() {
			t.Fatalf("expected circuit breaker %s, got %s", StateClosed, report.GetCircuitBreakerStatus())
		}
		if report.GetRunwayWatermark() != 1 {
			t.Fatalf("expected runway watermark 1, got %d", report.GetRunwayWatermark())
		}
		if !report.IsHealthy() {
			t.Fatal("expected running engine telemetry report to be healthy")
		}

		// 4. Calling Start() on already running engine returns ErrEngineAlreadyRunning.
		if err := engine.Start(ctx); !errors.Is(err, ErrEngineAlreadyRunning) {
			t.Fatalf("expected ErrEngineAlreadyRunning on duplicate Start(), got %v", err)
		}

		// 5. Stop() cancels context, terminates supervisor within graceful timeout, transitions to EngineStateStopped.
		if err := engine.Stop(); err != nil {
			t.Fatalf("failed to stop engine: %v", err)
		}
		if engine.State() != EngineStateStopped {
			t.Fatalf("expected state %s, got %s", EngineStateStopped, engine.State())
		}
		if engine.IsRunning() {
			t.Fatal("expected engine not running after Stop()")
		}

		// 6. Subsequent Stop() returns ErrEngineStopped.
		if err := engine.Stop(); !errors.Is(err, ErrEngineStopped) {
			t.Fatalf("expected ErrEngineStopped on subsequent Stop(), got %v", err)
		}
	})

	t.Run("NilReceiverSafety", func(t *testing.T) {
		var nilEngine *AmbientEngine

		if err := nilEngine.Start(context.Background()); !errors.Is(err, ErrNilEngine) {
			t.Fatalf("expected ErrNilEngine on Start(), got %v", err)
		}
		if err := nilEngine.Stop(); !errors.Is(err, ErrNilEngine) {
			t.Fatalf("expected ErrNilEngine on Stop(), got %v", err)
		}

		status := nilEngine.Status()
		if status.State != EngineStateStopped {
			t.Fatalf("expected StateStopped on nil Status(), got %s", status.State)
		}

		telem := nilEngine.Telemetry()
		if telem != (EngineTelemetry{}) {
			t.Fatalf("expected zero EngineTelemetry on nil receiver, got %+v", telem)
		}

		nilReport := nilEngine.TelemetryReport()
		if nilReport == nil || nilReport.GetStatus() != TelemetryStatusStopped {
			t.Fatalf("expected stopped report on nil TelemetryReport(), got %+v", nilReport)
		}
	})

	t.Run("DynamicStateDegradationAndRecovery", func(t *testing.T) {
		cfg := DefaultAmbientEngineConfig()
		cfg.TickInterval = 10 * time.Millisecond
		cfg.GracefulStopTimeout = 2 * time.Second

		engine, err := NewAmbientEngine(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if err := engine.Start(ctx); err != nil {
			t.Fatalf("failed to start: %v", err)
		}
		defer func() { _ = engine.Stop() }()

		// Trip circuit breaker to Open state
		if err := engine.FaultIsolator().Trip(); err != nil {
			t.Fatalf("failed to trip fault isolator: %v", err)
		}

		// Await supervisor tick reconciling to EngineStateDegraded
		deadline := time.Now().Add(1 * time.Second)
		degraded := false
		for time.Now().Before(deadline) {
			if engine.State() == EngineStateDegraded {
				degraded = true
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		if !degraded {
			t.Fatalf("expected engine state to degrade to %s, got %s", EngineStateDegraded, engine.State())
		}

		telem := engine.Telemetry()
		if telem.CircuitState != StateOpen.String() {
			t.Fatalf("expected circuit state Open in telemetry, got %s", telem.CircuitState)
		}

		// Reset isolator to Closed
		if err := engine.FaultIsolator().Reset(); err != nil {
			t.Fatalf("failed to reset fault isolator: %v", err)
		}

		// Await supervisor tick reconciling back to EngineStateRunning
		deadline = time.Now().Add(1 * time.Second)
		recovered := false
		for time.Now().Before(deadline) {
			if engine.State() == EngineStateRunning {
				recovered = true
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		if !recovered {
			t.Fatalf("expected engine state to recover to %s, got %s", EngineStateRunning, engine.State())
		}
	})

	t.Run("ConcurrentReadAccessUnderTicks", func(t *testing.T) {
		cfg := DefaultAmbientEngineConfig()
		cfg.TickInterval = 5 * time.Millisecond
		cfg.GracefulStopTimeout = 2 * time.Second

		engine, err := NewAmbientEngine(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if err := engine.Start(ctx); err != nil {
			t.Fatalf("failed to start: %v", err)
		}

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			goroutinelabels.NewGoroutine("test_concurrent_reader", "read engine status under ticks").
				WithWaitGroup(&wg).
				StartSimple(func() {
					for j := 0; j < 50; j++ {
						_ = engine.Status()
						_ = engine.Telemetry()
						_ = engine.State()
						_ = engine.IsRunning()
						time.Sleep(1 * time.Millisecond)
					}
				})
		}

		wg.Wait()
		if err := engine.Stop(); err != nil {
			t.Fatalf("failed to stop: %v", err)
		}
	})
}

// TestAmbientEngine_ASTAudit verifies zero swallowed errors or other medium/high AST violations
// in engine core, lifecycle, and resilience implementation files.
func TestAmbientEngine_ASTAudit(t *testing.T) {
	auditor := qa.NewASTAuditor()
	targetFiles := []string{
		"engine.go",
		"engine_lifecycle.go",
		"engine_resilience.go",
	}

	for _, file := range targetFiles {
		violations, err := auditor.AuditFile(file)
		if err != nil {
			t.Fatalf("failed to audit %s: %v", file, err)
		}
		for _, v := range violations {
			if v.Severity == "high" || v.Severity == "medium" || v.Severity == "error" {
				t.Errorf("AST violation [%s] in %s at %s: %s", v.Severity, file, v.Pos, v.Message)
			}
		}
	}
}
