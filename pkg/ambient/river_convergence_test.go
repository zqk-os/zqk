package ambient_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/ambient"
)

// Invariant (Static Floor): Default configuration adheres to bounded resource constants and sane defaults.
func TestRiverConvergence_StaticFloorInvariant(t *testing.T) {
	t.Parallel()

	visionVec := []float64{1.0, 0.5, 0.25}
	cfg := ambient.DefaultRiverConfig("VIS-ROOT-SEED", visionVec)

	if cfg.QueueCapacity != ambient.DefaultIntentQueueCapacity {
		t.Errorf("expected QueueCapacity %d, got %d", ambient.DefaultIntentQueueCapacity, cfg.QueueCapacity)
	}
	if cfg.RiverCapacity != ambient.DefaultRiverCapacity {
		t.Errorf("expected RiverCapacity %d, got %d", ambient.DefaultRiverCapacity, cfg.RiverCapacity)
	}
	if cfg.MinConsensusDrops != ambient.DefaultMinConsensusDrops {
		t.Errorf("expected MinConsensusDrops %d, got %d", ambient.DefaultMinConsensusDrops, cfg.MinConsensusDrops)
	}
	if cfg.AlignmentThreshold != ambient.DefaultAlignmentThreshold {
		t.Errorf("expected AlignmentThreshold %f, got %f", ambient.DefaultAlignmentThreshold, cfg.AlignmentThreshold)
	}

	ctx := context.Background()
	engine, err := ambient.NewRiverConvergenceEngine(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected NewRiverConvergenceEngine error: %v", err)
	}
	defer engine.Close()

	metrics := engine.Metrics()
	if metrics.TotalDropsIngested != 0 {
		t.Errorf("expected 0 drops ingested initially, got %d", metrics.TotalDropsIngested)
	}
	if metrics.TotalRiversFormed != 0 {
		t.Errorf("expected 0 rivers formed initially, got %d", metrics.TotalRiversFormed)
	}
}

// Dynamic Proof (Operational): Raindrops of intent pool into a surging river aligned with root vision.
func TestRiverConvergence_OperationalProof_SurgingRiver(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Root vision vector
	visionVec := []float64{1.0, 1.0, 1.0}
	cfg := ambient.DefaultRiverConfig("VIS-ORIGINAL-SEED", visionVec)
	cfg.ConvergenceWindow = 10 * time.Millisecond
	cfg.MinConsensusDrops = 3

	engine, err := ambient.NewRiverConvergenceEngine(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer engine.Close()

	// Feed 5 drops closely aligned with root vision
	for i := 1; i <= 5; i++ {
		drop := ambient.IntentDrop{
			ID:           "drop-001",
			Source:       "agent-seat-1",
			MissionID:    "MIS-KERNEL-EXPEDITION",
			IntentVector: []float64{0.95, 1.05, 0.98},
			Timestamp:    time.Now(),
			Weight:       1.0,
		}
		if err := engine.IngestDrop(drop); err != nil {
			t.Fatalf("IngestDrop %d failed: %v", i, err)
		}
	}

	// Allow convergence window to flush
	time.Sleep(50 * time.Millisecond)

	river, ok := engine.GetRiver("river:MIS-KERNEL-EXPEDITION")
	if !ok {
		t.Fatal("expected river:MIS-KERNEL-EXPEDITION to exist")
	}

	if river.DropCount != 5 {
		t.Errorf("expected DropCount 5, got %d", river.DropCount)
	}
	if river.Status != "surging" {
		t.Errorf("expected river status 'surging', got %q", river.Status)
	}
	if river.AlignmentScore < 0.95 {
		t.Errorf("expected alignment score > 0.95, got %f", river.AlignmentScore)
	}

	metrics := engine.Metrics()
	if metrics.TotalDropsIngested != 5 {
		t.Errorf("expected 5 drops ingested, got %d", metrics.TotalDropsIngested)
	}
	if metrics.TotalRiversFormed != 1 {
		t.Errorf("expected 1 river formed, got %d", metrics.TotalRiversFormed)
	}
	if metrics.DivergenceWarnings != 0 {
		t.Errorf("expected 0 divergence warnings, got %d", metrics.DivergenceWarnings)
	}
}

// Adversarial Proof (Negative Boundary): Validates error handling for corrupt drops, dimension mismatches, and diverging missions.
func TestRiverConvergence_NegativeBoundary_AdversarialScenarios(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	visionVec := []float64{1.0, 0.0, 0.0}
	var (
		divergenceMu   sync.Mutex
		divergentEvent bool
		divergentScore float64
	)

	cfg := ambient.DefaultRiverConfig("VIS-ROOT-SEED", visionVec)
	cfg.ConvergenceWindow = 10 * time.Millisecond
	cfg.MinConsensusDrops = 2
	cfg.AlignmentThreshold = 0.50
	cfg.OnDivergence = func(riverID, missionID string, alignmentScore float64) {
		divergenceMu.Lock()
		defer divergenceMu.Unlock()
		divergentEvent = true
		divergentScore = alignmentScore
	}

	engine, err := ambient.NewRiverConvergenceEngine(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// 1. Missing Mission ID
	err = engine.IngestDrop(ambient.IntentDrop{
		IntentVector: []float64{1.0, 0.0, 0.0},
	})
	if err != ambient.ErrEmptyMissionID {
		t.Errorf("expected ErrEmptyMissionID, got: %v", err)
	}

	// 2. Vector Dimension Mismatch
	err = engine.IngestDrop(ambient.IntentDrop{
		MissionID:    "MIS-ROGUE",
		IntentVector: []float64{1.0, 0.0}, // length 2 vs length 3
	})
	if err != ambient.ErrDimensionMismatch {
		t.Errorf("expected ErrDimensionMismatch, got: %v", err)
	}

	// 3. Inject Opposing / Divergent Intent Drops (orthogonal or opposing vector)
	for i := 0; i < 3; i++ {
		err = engine.IngestDrop(ambient.IntentDrop{
			MissionID:    "MIS-DIVERGENT",
			IntentVector: []float64{-1.0, 0.0, 0.0}, // completely opposing vector
			Weight:       1.0,
		})
		if err != nil {
			t.Fatalf("failed to ingest divergent drop %d: %v", i, err)
		}
	}

	time.Sleep(50 * time.Millisecond)

	divergenceMu.Lock()
	if !divergentEvent {
		t.Error("expected divergence hook to be triggered for opposing mission")
	}
	if divergentScore > 0.0 {
		t.Errorf("expected negative alignment score for opposing vector, got %f", divergentScore)
	}
	divergenceMu.Unlock()

	divergentRiver, ok := engine.GetRiver("river:MIS-DIVERGENT")
	if !ok {
		t.Fatal("expected divergent river to exist")
	}
	if divergentRiver.Status != "diverging" {
		t.Errorf("expected status 'diverging', got %q", divergentRiver.Status)
	}

	// 4. Ingesting after Engine Close returns ErrEngineClosed
	if err := engine.Close(); err != nil {
		t.Fatalf("failed to close engine: %v", err)
	}

	err = engine.IngestDrop(ambient.IntentDrop{
		MissionID:    "MIS-POST-CLOSE",
		IntentVector: []float64{1.0, 0.0, 0.0},
	})
	if err != ambient.ErrEngineClosed {
		t.Errorf("expected ErrEngineClosed after engine termination, got: %v", err)
	}
}

// Utility Proof: CosineSimilarity edge case coverage
func TestCosineSimilarity_EdgeCases(t *testing.T) {
	t.Parallel()

	// Zero-length vectors
	if sim := ambient.CosineSimilarity(nil, nil); sim != 0.0 {
		t.Errorf("expected 0.0 for nil vectors, got %f", sim)
	}
	if sim := ambient.CosineSimilarity([]float64{1.0}, []float64{1.0, 2.0}); sim != 0.0 {
		t.Errorf("expected 0.0 for mismatched lengths, got %f", sim)
	}
	// Zero-magnitude vectors
	if sim := ambient.CosineSimilarity([]float64{0.0, 0.0}, []float64{1.0, 1.0}); sim != 0.0 {
		t.Errorf("expected 0.0 for zero norm, got %f", sim)
	}
}
