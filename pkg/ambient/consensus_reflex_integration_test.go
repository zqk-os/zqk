package ambient

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	qa "github.com/zqk-os/zqk/pkg/validation/qa"
)

// Traceability Verification:
// Backlog Item: BLI-1793625348554336000-538995a1
// Requirement:  REQ-1793625103975205000-90b23076
// Criteria:     CRIT-1793625127194881000-ffd42540 (Negative Boundary: Expired Consensus Leases, Queue Saturation & Panic Isolation)

func setupRunningEngineWithArbiter(t *testing.T, tickInterval time.Duration) (*AmbientEngine, context.CancelFunc) {
	t.Helper()
	cfg := DefaultAmbientEngineConfig()
	cfg.TickInterval = tickInterval
	cfg.GracefulStopTimeout = 2 * time.Second

	engine, err := NewAmbientEngine(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	if startErr := engine.Start(ctx); startErr != nil {
		cancel()
		t.Fatalf("failed to start engine: %v", startErr)
	}
	return engine, cancel
}

func verifyLeaseExpiredAndTickActive(t *testing.T, engine *AmbientEngine, arbiter *AmbientConsensusArbiter, taskID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	expired := false
	for time.Now().Before(deadline) {
		lease, found := arbiter.GetLease(taskID)
		if found && lease.State == ConsensusStateExpired {
			expired = true
			if lease.WorkerID != "" {
				t.Errorf("expected worker id to be cleared upon expiry, got %q", lease.WorkerID)
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !expired {
		t.Fatalf("expected lease for %s to transition to ConsensusStateExpired", taskID)
	}

	initialTicks := engine.Status().TicksExecuted
	time.Sleep(30 * time.Millisecond)
	subsequentTicks := engine.Status().TicksExecuted

	if subsequentTicks <= initialTicks {
		t.Fatalf("engine tick loop stalled: initial=%d, subsequent=%d", initialTicks, subsequentTicks)
	}

	if !engine.IsRunning() {
		t.Fatal("expected engine to remain in running state after lease expiry reaping")
	}
}

// TestAmbientEngine_ConsensusLeaseExpiry verifies that tasks with expired lease
// durations are safely transitioned/evicted by the supervisor tick without stalling the engine.
func TestAmbientEngine_ConsensusLeaseExpiry(t *testing.T) {
	engine, cancel := setupRunningEngineWithArbiter(t, 10*time.Millisecond)
	defer func() {
		cancel()
		if stopErr := engine.Stop(); stopErr != nil {
			t.Errorf("failed to stop engine: %v", stopErr)
		}
	}()

	arbiter := engine.ConsensusArbiter()
	if arbiter == nil {
		t.Fatal("expected non-nil consensus arbiter from engine")
	}

	taskID := "task-expire-boundary-1"
	lease, propErr := arbiter.ProposeTask(taskID, []byte("lease expiry test payload"))
	if propErr != nil {
		t.Fatalf("failed to propose task: %v", propErr)
	}
	if lease.State != ConsensusStatePending {
		t.Fatalf("expected pending state, got %v", lease.State)
	}

	shortLeaseTTL := 25 * time.Millisecond
	acquiredLease, leaseErr := arbiter.LeaseTask(taskID, "worker-alpha", shortLeaseTTL)
	if leaseErr != nil {
		t.Fatalf("failed to lease task: %v", leaseErr)
	}
	if acquiredLease.State != ConsensusStateLeased {
		t.Fatalf("expected leased state, got %v", acquiredLease.State)
	}

	time.Sleep(50 * time.Millisecond)
	verifyLeaseExpiredAndTickActive(t, engine, arbiter, taskID)
}

func saturateReflexQueue(t *testing.T, ctx context.Context, reflex *SpinalCordReflexEngine, capacity int) {
	t.Helper()
	for i := 0; i < capacity; i++ {
		action := ReflexAction{
			ID:        fmt.Sprintf("act-fill-%d", i),
			Type:      ReflexActionRealignIntent,
			Target:    "intent_stream",
			Timestamp: time.Now().UTC(),
		}
		if trigErr := reflex.TriggerAction(ctx, action); trigErr != nil {
			t.Fatalf("unexpected error filling queue at index %d: %v", i, trigErr)
		}
	}

	overflowAction := ReflexAction{
		ID:        "act-overflow",
		Type:      ReflexActionRealignIntent,
		Target:    "intent_stream",
		Timestamp: time.Now().UTC(),
	}

	metricsBefore := reflex.Metrics()
	err := reflex.TriggerAction(ctx, overflowAction)
	if !errors.Is(err, ErrActionQueueFull) {
		t.Fatalf("expected ErrActionQueueFull, got: %v", err)
	}

	metricsAfter := reflex.Metrics()
	if metricsAfter.ActionsFailed <= metricsBefore.ActionsFailed {
		t.Fatalf("expected ActionsFailed to increment, before=%d, after=%d",
			metricsBefore.ActionsFailed, metricsAfter.ActionsFailed)
	}
}

// TestAmbientEngine_ReflexQueueSaturation verifies action queue saturation returns
// ErrActionQueueFull and increments drop counters without engine panic.
func TestAmbientEngine_ReflexQueueSaturation(t *testing.T) {
	t.Run("EngineBoundReflexSaturation", func(t *testing.T) {
		cfg := DefaultAmbientEngineConfig()
		cfg.TickInterval = 10 * time.Millisecond
		cfg.GracefulStopTimeout = 2 * time.Second

		engine, err := NewAmbientEngine(cfg)
		if err != nil {
			t.Fatalf("failed to create engine: %v", err)
		}

		reflex := engine.ReflexLoop()
		if reflex == nil {
			t.Fatal("expected non-nil reflex loop from engine")
		}

		ctx := context.Background()
		saturateReflexQueue(t, ctx, reflex, DefaultReflexActionQueueCapacity)

		if startErr := engine.Start(ctx); startErr != nil {
			t.Fatalf("failed to start engine with saturated reflex queue: %v", startErr)
		}
		defer func() {
			if stopErr := engine.Stop(); stopErr != nil {
				t.Errorf("failed to stop engine: %v", stopErr)
			}
		}()

		if !engine.IsRunning() {
			t.Fatal("expected engine to be running after reflex queue saturation")
		}
	})

	t.Run("BoundedCapacityReflexSaturation", func(t *testing.T) {
		const smallCapacity = 2
		reflex := NewSpinalCordReflexEngine(WithActionQueueCapacity(smallCapacity))
		ctx := context.Background()
		saturateReflexQueue(t, ctx, reflex, smallCapacity)

		if closeErr := reflex.Close(); closeErr != nil {
			t.Fatalf("failed to close reflex engine: %v", closeErr)
		}
	})
}

// TestAmbientEngine_ReflexPanicIsolation verifies that a panicked reflex action handler
// is safely recovered, isolated, and logged while the ambient engine continues ticking healthy.
func TestAmbientEngine_ReflexPanicIsolation(t *testing.T) {
	engine, cancel := setupRunningEngineWithArbiter(t, 10*time.Millisecond)
	defer func() {
		cancel()
		if stopErr := engine.Stop(); stopErr != nil {
			t.Errorf("failed to stop engine: %v", stopErr)
		}
	}()

	reflex := engine.ReflexLoop()
	if reflex == nil {
		t.Fatal("expected non-nil reflex loop from engine")
	}

	ctx := context.Background()
	panickingActionType := ReflexActionType("panicking_action")
	const expectedPanicValue = "deliberate test panic inside reflex handler"

	regErr := reflex.RegisterHandler(panickingActionType, func(ctx context.Context, action ReflexAction) error {
		panic(expectedPanicValue)
	})
	if regErr != nil {
		t.Fatalf("failed to register panicking handler: %v", regErr)
	}

	metricsBefore := reflex.Metrics()
	action := ReflexAction{
		ID:        "act-panic-1",
		Type:      panickingActionType,
		Target:    "fault_isolator",
		Timestamp: time.Now().UTC(),
	}

	err := reflex.TriggerAction(ctx, action)
	if !errors.Is(err, ErrActionHandlerPanicked) {
		t.Fatalf("expected error wrapping ErrActionHandlerPanicked, got: %v", err)
	}
	if !strings.Contains(err.Error(), expectedPanicValue) {
		t.Fatalf("expected panic message %q in error string, got: %v", expectedPanicValue, err)
	}

	metricsAfter := reflex.Metrics()
	if metricsAfter.ActionsFailed <= metricsBefore.ActionsFailed {
		t.Fatalf("expected ActionsFailed to increment after panic, before=%d, after=%d",
			metricsBefore.ActionsFailed, metricsAfter.ActionsFailed)
	}

	initialTicks := engine.Status().TicksExecuted
	time.Sleep(30 * time.Millisecond)
	subsequentTicks := engine.Status().TicksExecuted

	if subsequentTicks <= initialTicks {
		t.Fatalf("engine tick loop stalled after panic recovery: initial=%d, subsequent=%d",
			initialTicks, subsequentTicks)
	}

	if engine.State() != EngineStateRunning {
		t.Fatalf("expected engine state running after isolated panic, got: %s", engine.State())
	}
}

func verifyNilEngineGuards(t *testing.T) {
	t.Helper()
	var nilEngine *AmbientEngine

	if nilEngine.ConsensusArbiter() != nil {
		t.Fatal("expected nil consensus arbiter for nil engine")
	}
	if nilEngine.ReflexLoop() != nil {
		t.Fatal("expected nil reflex loop for nil engine")
	}

	report := nilEngine.TelemetryReport()
	if report == nil {
		t.Fatal("expected non-nil degraded report on nil engine")
	}
	if report.GetStatus() != TelemetryStatusStopped {
		t.Fatalf("expected stopped status on nil engine, got %s", report.GetStatus())
	}
	if report.IsHealthy() {
		t.Fatal("expected nil engine report to be unhealthy")
	}
}

func verifyNilArbiterGuards(t *testing.T) {
	t.Helper()
	var nilArbiter *AmbientConsensusArbiter

	if reaped := nilArbiter.ReapExpiredLeases(); reaped != 0 {
		t.Fatalf("expected 0 reaped on nil arbiter, got %d", reaped)
	}
	if leases := nilArbiter.ListLeases(); len(leases) != 0 {
		t.Fatalf("expected empty leases on nil arbiter, got %+v", leases)
	}
	if lease, found := nilArbiter.GetLease("dummy"); found || lease != nil {
		t.Fatalf("expected nil lease and false on nil arbiter, got %v, %v", lease, found)
	}
	if err := nilArbiter.Stop(); err != nil {
		if !errors.Is(err, ErrNilReceiver) {
			t.Fatalf("expected ErrNilReceiver on nil arbiter Stop, got %v", err)
		}
	}
	if err := nilArbiter.Close(); err != nil {
		if !errors.Is(err, ErrNilReceiver) {
			t.Fatalf("expected ErrNilReceiver on nil arbiter Close, got %v", err)
		}
	}
}

func verifyNilReflexGuards(t *testing.T) {
	t.Helper()
	var nilReflex *SpinalCordReflexEngine
	ctx := context.Background()

	action := ReflexAction{
		ID:   "act-nil",
		Type: ReflexActionRealignIntent,
	}

	if err := nilReflex.TriggerAction(ctx, action); !errors.Is(err, ErrNilReflexEngine) {
		t.Fatalf("expected ErrNilReflexEngine, got %v", err)
	}
	if _, err := nilReflex.Evaluate(ctx, nil); !errors.Is(err, ErrNilReflexEngine) {
		t.Fatalf("expected ErrNilReflexEngine on Evaluate, got %v", err)
	}
	metrics := nilReflex.Metrics()
	if metrics.EvaluationsCount != 0 || metrics.ActionsTriggered != 0 || metrics.ActionsFailed != 0 {
		t.Fatalf("expected zero metrics on nil reflex, got %+v", metrics)
	}
	if history := nilReflex.History(); history != nil {
		t.Fatalf("expected nil history on nil reflex, got %+v", history)
	}
	if err := nilReflex.Close(); err != nil {
		t.Fatalf("expected nil error on nil reflex Close, got %v", err)
	}
}

func verifyPartialEngineTelemetryReport(t *testing.T) {
	t.Helper()
	partialEngine := &AmbientEngine{
		state: EngineStateRunning,
	}
	if partialEngine.ConsensusArbiter() != nil {
		t.Fatal("expected nil consensus arbiter on partial engine")
	}
	if partialEngine.ReflexLoop() != nil {
		t.Fatal("expected nil reflex loop on partial engine")
	}

	report := partialEngine.TelemetryReport()
	if report == nil {
		t.Fatal("expected non-nil telemetry report for partial engine")
	}
	if _, exists := report.GetComponent("consensus_arbiter"); exists {
		t.Fatal("component consensus_arbiter should not be registered when nil")
	}
	if _, exists := report.GetComponent("reflex_loop"); exists {
		t.Fatal("component reflex_loop should not be registered when nil")
	}
}

// TestAmbientEngine_NilConsensusAndReflexGuards verifies nil engine, nil arbiter,
// and nil reflex loop accessors and TelemetryReport calls degrade gracefully.
func TestAmbientEngine_NilConsensusAndReflexGuards(t *testing.T) {
	t.Run("NilEngineAccessorsAndTelemetryReport", func(t *testing.T) {
		verifyNilEngineGuards(t)
	})

	t.Run("NilConsensusArbiterDirectGuards", func(t *testing.T) {
		verifyNilArbiterGuards(t)
	})

	t.Run("NilReflexLoopDirectGuards", func(t *testing.T) {
		verifyNilReflexGuards(t)
	})

	t.Run("EngineWithNilSubcomponentsTelemetryReport", func(t *testing.T) {
		verifyPartialEngineTelemetryReport(t)
	})
}

// TestAmbientEngine_ConcurrentConsensusAndReflexWorkers exercises concurrent
// access across the engine, consensus arbiter, and reflex loop using goroutinelabels.
func TestAmbientEngine_ConcurrentConsensusAndReflexWorkers(t *testing.T) {
	engine, cancel := setupRunningEngineWithArbiter(t, 10*time.Millisecond)
	defer func() {
		cancel()
		if stopErr := engine.Stop(); stopErr != nil {
			t.Errorf("failed to stop engine: %v", stopErr)
		}
	}()

	arbiter := engine.ConsensusArbiter()
	reflex := engine.ReflexLoop()

	const numWorkers = 8
	const numIterations = 20
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for w := 0; w < numWorkers; w++ {
		workerID := fmt.Sprintf("worker-%d", w)
		goroutinelabels.NewGoroutine("test_concurrent_worker", "concurrent arbiter and reflex access").
			StartSimple(func() {
				defer wg.Done()
				ctx := context.Background()
				for i := 0; i < numIterations; i++ {
					taskID := fmt.Sprintf("task-conc-%s-%d", workerID, i)
					_, propErr := arbiter.ProposeTask(taskID, []byte("concurrent payload"))
					if propErr != nil {
						t.Errorf("concurrent ProposeTask failed: %v", propErr)
					}
					_, leaseErr := arbiter.LeaseTask(taskID, workerID, 20*time.Millisecond)
					if leaseErr != nil && !errors.Is(leaseErr, ErrTaskAlreadyLeased) {
						t.Errorf("concurrent LeaseTask unexpected err: %v", leaseErr)
					}

					action := ReflexAction{
						ID:        fmt.Sprintf("act-conc-%s-%d", workerID, i),
						Type:      ReflexActionRealignIntent,
						Target:    "intent_stream",
						Timestamp: time.Now().UTC(),
					}
					trigErr := reflex.TriggerAction(ctx, action)
					if trigErr != nil && !errors.Is(trigErr, ErrActionQueueFull) {
						t.Errorf("concurrent TriggerAction unexpected err: %v", trigErr)
					}
				}
			})
	}

	wg.Wait()

	if !engine.IsRunning() {
		t.Fatal("expected engine to remain running after concurrent worker operations")
	}
}

// TestConsensusReflex_StaticFloorInvariants satisfies CRIT-1793625127194879000-56cf209b.
func TestConsensusReflex_StaticFloorInvariants(t *testing.T) {
	t.Parallel()

	cfg := DefaultAmbientEngineConfig()
	require.NotNil(t, cfg.ConsensusArbiterConfig)
	require.NotNil(t, cfg.ReflexLoopConfig)

	assert.Equal(t, DefaultLeaseDuration, cfg.ConsensusArbiterConfig.DefaultLeaseTTL)
	assert.Equal(t, DefaultReapInterval, cfg.ConsensusArbiterConfig.ReapInterval)
	assert.Equal(t, DefaultMaxConsensusTasks, cfg.ConsensusArbiterConfig.MaxTasks)

	engine, err := NewAmbientEngine(cfg)
	require.NoError(t, err)
	require.NotNil(t, engine)
	assert.NotNil(t, engine.ConsensusArbiter())
	assert.NotNil(t, engine.ReflexLoop())
}

// TestConsensusReflex_OperationalDynamicProof satisfies CRIT-1793625127194880000-6df3a8f2.
func TestConsensusReflex_OperationalDynamicProof(t *testing.T) {
	t.Parallel()

	cfg := DefaultAmbientEngineConfig()
	cfg.TickInterval = 20 * time.Millisecond
	cfg.ConsensusArbiterConfig.ReapInterval = 20 * time.Millisecond

	engine, err := NewAmbientEngine(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, engine.Start(ctx))
	assert.True(t, engine.IsRunning())

	arb := engine.ConsensusArbiter()
	require.NotNil(t, arb)

	lease, err := arb.ProposeTask("task-dyn-proof", []byte("payload"))
	require.NoError(t, err)
	assert.Equal(t, ConsensusStatePending, lease.State)

	claimed, err := arb.LeaseTask("task-dyn-proof", "worker-dyn", 1*time.Second)
	require.NoError(t, err)
	assert.Equal(t, ConsensusStateLeased, claimed.State)

	require.NoError(t, arb.CommitTask("task-dyn-proof", "worker-dyn"))

	report := engine.TelemetryReport()
	require.NotNil(t, report)
	assert.Equal(t, 6, report.GetActiveHolonsCount())

	compArb, okArb := report.GetComponent("consensus_arbiter")
	require.True(t, okArb)
	assert.Equal(t, TelemetryStatusHealthy, compArb.Status)

	compReflex, okReflex := report.GetComponent("reflex_loop")
	require.True(t, okReflex)
	assert.Equal(t, TelemetryStatusHealthy, compReflex.Status)

	require.NoError(t, engine.Stop())
}

// TestConsensusReflex_NegativeAdversarialBoundaries satisfies CRIT-1793625127194881000-ffd42540.
func TestConsensusReflex_NegativeAdversarialBoundaries(t *testing.T) {
	t.Parallel()

	cfg := DefaultAmbientEngineConfig()
	cfg.ConsensusArbiterConfig.DefaultLeaseTTL = 50 * time.Millisecond
	cfg.ConsensusArbiterConfig.ReapInterval = 10 * time.Millisecond

	engine, err := NewAmbientEngine(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, engine.Start(ctx))
	defer func() {
		_ = engine.Stop()
	}()

	arb := engine.ConsensusArbiter()
	require.NotNil(t, arb)

	for i := 0; i < 10; i++ {
		taskID := fmt.Sprintf("neg-task-%d", i)
		_, err := arb.ProposeTask(taskID, []byte("data"))
		require.NoError(t, err)
	}

	const numWorkers = 30
	var wg sync.WaitGroup
	wg.Add(numWorkers)
	var leasesClaimed int64

	for w := 0; w < numWorkers; w++ {
		workerID := fmt.Sprintf("worker-neg-%d", w)
		goroutinelabels.NewGoroutine("test_neg_worker", "negative boundary stress worker").
			StartSimple(func() {
				defer wg.Done()
				for i := 0; i < 10; i++ {
					taskID := fmt.Sprintf("neg-task-%d", i%10)
					lease, lErr := arb.LeaseTask(taskID, workerID, 20*time.Millisecond)
					if lErr == nil && lease != nil {
						atomic.AddInt64(&leasesClaimed, 1)
						_ = arb.CommitTask(taskID, workerID)
					}
				}
			})
	}

	wg.Wait()
	assert.Greater(t, atomic.LoadInt64(&leasesClaimed), int64(0))
}

// TestConsensusReflex_ASTAudit satisfies code craftsmanship and hygiene mandates.
func TestConsensusReflex_ASTAudit(t *testing.T) {
	auditor := qa.NewASTAuditor()
	targetFiles := []string{
		"consensus_arbiter.go",
		"reflex_loop.go",
		"engine.go",
		"engine_lifecycle.go",
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
