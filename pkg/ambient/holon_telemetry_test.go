package ambient

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestHolonTelemetry_StaticFloorInvariants satisfies CRIT-1793589814624125000-c28209f6
// (Static Floor Invariant: Named buffer constants, nil-safe telemetry receivers, and zero unhandled panic vectors).
func TestHolonTelemetry_StaticFloorInvariants(t *testing.T) {
	// 1. Verify named constants invariants
	if DefaultTelemetryCapacity <= 0 {
		t.Fatalf("expected DefaultTelemetryCapacity > 0, got %d", DefaultTelemetryCapacity)
	}
	if DefaultTelemetryHistoryLimit <= 0 {
		t.Fatalf("expected DefaultTelemetryHistoryLimit > 0, got %d", DefaultTelemetryHistoryLimit)
	}
	if DefaultTelemetryTimeout <= 0 {
		t.Fatalf("expected DefaultTelemetryTimeout > 0, got %v", DefaultTelemetryTimeout)
	}
	if MaxAllowedDivergences <= 0 {
		t.Fatalf("expected MaxAllowedDivergences > 0, got %d", MaxAllowedDivergences)
	}

	// 2. Verify nil-safe receiver methods (zero panic guarantee)
	var nilAggregator *HolonTelemetryAggregator
	snap := nilAggregator.Snapshot(context.Background())
	if snap == nil {
		t.Fatal("expected non-nil snapshot from nil aggregator")
	}
	if snap.HealthStatus != TelemetryHealthQuiescent {
		t.Fatalf("expected quiescent health status for nil aggregator, got %s", snap.HealthStatus)
	}
	if len(snap.Warnings) == 0 {
		t.Fatal("expected warnings for nil aggregator call")
	}

	history := nilAggregator.History()
	if history != nil {
		t.Fatalf("expected nil history for nil aggregator, got %v", history)
	}

	var nilSnapshot *MiddleBrainTelemetrySnapshot
	if str := nilSnapshot.String(); !strings.Contains(str, "<nil>") {
		t.Fatalf("expected nil representation in string, got %s", str)
	}
}

// TestHolonTelemetry_OperationalDynamicProof satisfies CRIT-1793589814624126000-aa123817
// (Operational Dynamic Proof: Concurrent middle-brain holon telemetry aggregation and CLI status report).
func TestHolonTelemetry_OperationalDynamicProof(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Setup Holon Registry with active middle-brain holons
	reg := NewHolonLifecycleRegistry()
	if err := reg.Register("hydrator-holon-1", "Pool intent raindrops into streams"); err != nil {
		t.Fatalf("failed to register hydrator holon: %v", err)
	}
	if err := reg.Transition("hydrator-holon-1", HolonPhaseStarting); err != nil {
		t.Fatalf("failed to transition to starting: %v", err)
	}
	if err := reg.Transition("hydrator-holon-1", HolonPhaseRunning); err != nil {
		t.Fatalf("failed to transition to running: %v", err)
	}

	// 2. Setup Stream Hydrator and inject raindrops
	hydrator := NewStreamHydrator(DefaultStreamCapacity, DefaultMinWeightFloor)
	err := hydrator.PoolRaindrop(ctx, "stream-telemetry-1", IntentRaindrop{
		ID:        "drop-telemetry-001",
		Source:    "user-chat",
		Kind:      "cli_command",
		Content:   "Build an ambient holon telemetry status command",
		Weight:    1.0,
		Timestamp: time.Now(),
		Tags:      []string{"cli", "ambient"},
	})
	if err != nil {
		t.Fatalf("failed to pool raindrop: %v", err)
	}

	// 3. Setup River Convergence Engine
	rootVision := []float64{1.0, 0.0, 0.0}
	engine, err := NewRiverConvergenceEngine(ctx, RiverConfig{
		RootVisionID:       "VIS-ROOT-MIDDLE-BRAIN",
		RootVisionVector:   rootVision,
		ConvergenceWindow:  DefaultConvergenceWindow,
		MinConsensusDrops:  1,
		AlignmentThreshold: 0.50,
	})
	if err != nil {
		t.Fatalf("failed to construct river convergence engine: %v", err)
	}
	defer engine.Close()

	if err := engine.IngestDrop(IntentDrop{
		ID:           "drop-stream-001",
		Source:       "user-prompt",
		MissionID:    "MIS-HOLON-TELEMETRY",
		IntentVector: []float64{1.0, 0.0, 0.0},
		Timestamp:    time.Now(),
		Weight:       1.0,
	}); err != nil {
		t.Fatalf("failed to ingest drop into river engine: %v", err)
	}

	// Wait briefly for river convergence processing
	time.Sleep(100 * time.Millisecond)

	// 4. Run Telemetry Aggregator Snapshot
	aggregator := NewHolonTelemetryAggregator(reg, hydrator, engine)
	snapshot := aggregator.Snapshot(ctx)

	if snapshot.ActiveHolonCount != 1 {
		t.Fatalf("expected 1 active holon, got %d", snapshot.ActiveHolonCount)
	}
	if snapshot.StreamCount != 1 {
		t.Fatalf("expected 1 active stream, got %d", snapshot.StreamCount)
	}
	if snapshot.TotalRaindrops != 1 {
		t.Fatalf("expected 1 total raindrop, got %d", snapshot.TotalRaindrops)
	}
	if snapshot.TotalDropsIngested != 1 {
		t.Fatalf("expected 1 drop ingested into river, got %d", snapshot.TotalDropsIngested)
	}
	if snapshot.AverageAlignmentScore < 0.99 {
		t.Fatalf("expected alignment score ~1.0, got %f", snapshot.AverageAlignmentScore)
	}

	// Verify history buffer retention
	history := aggregator.History()
	if len(history) != 1 {
		t.Fatalf("expected 1 snapshot in history, got %d", len(history))
	}

	summary := snapshot.String()
	if !strings.Contains(summary, "Active Holons: 1") || !strings.Contains(summary, "Streams: 1") {
		t.Fatalf("unexpected summary format: %s", summary)
	}
}

// TestHolonTelemetry_NegativeAdversarialBoundaries satisfies CRIT-1793589814624127000-371d4759
// (Negative Adversarial Boundary: Corrupt ambient state resilience, uninitialized streams, and exhausted telemetry channels).
func TestHolonTelemetry_NegativeAdversarialBoundaries(t *testing.T) {
	ctx := context.Background()

	// 1. Partial/unconfigured components (registry only with active holon)
	reg := NewHolonLifecycleRegistry()
	_ = reg.Register("standalone-holon", "Isolated worker")
	_ = reg.Transition("standalone-holon", HolonPhaseStarting)
	_ = reg.Transition("standalone-holon", HolonPhaseRunning)
	aggregator := NewHolonTelemetryAggregator(reg, nil, nil)

	snapshot := aggregator.Snapshot(ctx)
	if snapshot.HealthStatus != TelemetryHealthDegraded {
		t.Fatalf("expected degraded health status for partial aggregator, got %s", snapshot.HealthStatus)
	}
	if len(snapshot.Warnings) < 2 {
		t.Fatalf("expected at least 2 warnings for missing hydrator and engine, got %d", len(snapshot.Warnings))
	}

	// 1b. Quiescent state with zero active holons and zero rivers
	emptyReg := NewHolonLifecycleRegistry()
	quiescentAggregator := NewHolonTelemetryAggregator(emptyReg, nil, nil)
	quiescentSnap := quiescentAggregator.Snapshot(ctx)
	if quiescentSnap.HealthStatus != TelemetryHealthQuiescent {
		t.Fatalf("expected quiescent health status for empty aggregator, got %s", quiescentSnap.HealthStatus)
	}

	// 2. High divergence threshold triggers critical health status
	engine, err := NewRiverConvergenceEngine(ctx, RiverConfig{
		RootVisionID:       "VIS-ROOT",
		RootVisionVector:   []float64{1.0, 0.0, 0.0},
		ConvergenceWindow:  10 * time.Millisecond,
		MinConsensusDrops:  1,
		AlignmentThreshold: 0.99,
	})
	if err != nil {
		t.Fatalf("failed to construct river convergence engine: %v", err)
	}
	defer engine.Close()

	// Ingest conflicting orthogonal drops to trigger divergences
	for i := 0; i < MaxAllowedDivergences+2; i++ {
		_ = engine.IngestDrop(IntentDrop{
			ID:           "diverging-drop",
			Source:       "adversarial",
			MissionID:    "MIS-ROGUE",
			IntentVector: []float64{0.0, 1.0, 0.0}, // Orthogonal vector -> 0.0 alignment
			Timestamp:    time.Now(),
			Weight:       1.0,
		})
	}
	time.Sleep(50 * time.Millisecond)

	fullAggregator := NewHolonTelemetryAggregator(reg, nil, engine)
	critSnapshot := fullAggregator.Snapshot(ctx)
	if critSnapshot.HealthStatus != TelemetryHealthCritical {
		t.Fatalf("expected critical health status on excessive divergences, got %s", critSnapshot.HealthStatus)
	}
	if critSnapshot.DivergenceCount <= MaxAllowedDivergences {
		t.Fatalf("expected divergences > %d, got %d", MaxAllowedDivergences, critSnapshot.DivergenceCount)
	}

	// 3. Ring buffer history limit enforcement
	for i := 0; i < DefaultTelemetryHistoryLimit+20; i++ {
		_ = fullAggregator.Snapshot(ctx)
	}
	if len(fullAggregator.History()) != DefaultTelemetryHistoryLimit {
		t.Fatalf("expected history ring buffer capped at %d, got %d", DefaultTelemetryHistoryLimit, len(fullAggregator.History()))
	}
}
