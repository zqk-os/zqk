package ambient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
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
	if snap.WarningCount != len(snap.Warnings) {
		t.Fatalf("expected warning count %d, got %d", len(snap.Warnings), snap.WarningCount)
	}

	history := nilAggregator.History()
	if history != nil {
		t.Fatalf("expected nil history for nil aggregator, got %v", history)
	}
	if limit := nilAggregator.HistoryLimit(); limit != 0 {
		t.Fatalf("expected 0 history limit for nil aggregator, got %d", limit)
	}

	// 3. MiddleBrainTelemetrySnapshot nil-safety
	var nilSnapshot *MiddleBrainTelemetrySnapshot
	if str := nilSnapshot.String(); !strings.Contains(str, "<nil>") {
		t.Fatalf("expected nil representation in string, got %s", str)
	}
	if nilSnapshot.IsHealthy() {
		t.Fatal("expected IsHealthy() to return false for nil snapshot")
	}
	if nilSnapshot.IsDegraded() {
		t.Fatal("expected IsDegraded() to return false for nil snapshot")
	}
	if nilSnapshot.IsCritical() {
		t.Fatal("expected IsCritical() to return false for nil snapshot")
	}
	if !nilSnapshot.IsQuiescent() {
		t.Fatal("expected IsQuiescent() to return true for nil snapshot")
	}
	if !nilSnapshot.HasWarnings() {
		t.Fatal("expected HasWarnings() to return true for nil snapshot")
	}
	if ids := nilSnapshot.HolonIDs(); ids != nil {
		t.Fatalf("expected nil HolonIDs for nil snapshot, got %v", ids)
	}

	// 4. Aggregator builder with custom limit floor
	aggWithLimit := NewHolonTelemetryAggregatorWithLimit(nil, nil, nil, -10)
	if aggWithLimit.HistoryLimit() != DefaultTelemetryHistoryLimit {
		t.Fatalf("expected default history limit fallback, got %d", aggWithLimit.HistoryLimit())
	}
	aggWithCustom := NewHolonTelemetryAggregatorWithLimit(nil, nil, nil, 42)
	if aggWithCustom.HistoryLimit() != 42 {
		t.Fatalf("expected custom history limit 42, got %d", aggWithCustom.HistoryLimit())
	}

	// 5. Test snapshot receiver predicates on populated snapshots
	healthySnap := &MiddleBrainTelemetrySnapshot{
		HealthStatus: TelemetryHealthHealthy,
		Holons: []HolonRecord{
			{ID: "holon-1", Phase: HolonPhaseRunning},
			{ID: "holon-2", Phase: HolonPhaseRunning},
		},
	}
	if !healthySnap.IsHealthy() || healthySnap.IsDegraded() || healthySnap.IsCritical() || healthySnap.IsQuiescent() {
		t.Fatalf("unexpected predicates for healthy snapshot: %+v", healthySnap)
	}
	if healthySnap.HasWarnings() {
		t.Fatal("expected no warnings on healthy snapshot")
	}
	ids := healthySnap.HolonIDs()
	if len(ids) != 2 || ids[0] != "holon-1" || ids[1] != "holon-2" {
		t.Fatalf("unexpected holon IDs: %v", ids)
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

// TestHolonTelemetry_CorruptAmbientStateFilesOnDisk satisfies CRIT-1793589814624127000-371d4759
// by verifying that missing, empty, truncated, or unparseable ambient state files fail closed
// with actionable error receipts and never trigger panics.
func TestHolonTelemetry_CorruptAmbientStateFilesOnDisk(t *testing.T) {
	tempDir, err := fileutil.MkdirTemp("", "ambient-telemetry-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tempDir)

	// 1. Successful round-trip serialization and loading
	validSnap := &MiddleBrainTelemetrySnapshot{
		Timestamp:             time.Now().UTC().Truncate(time.Millisecond),
		HealthStatus:          TelemetryHealthHealthy,
		ActiveHolonCount:      2,
		Holons:                []HolonRecord{{ID: "holon-a", Phase: HolonPhaseRunning}},
		StreamCount:           1,
		TotalRaindrops:        5,
		RiverCount:            1,
		TotalDropsIngested:    5,
		AverageAlignmentScore: 0.95,
	}
	validFile := filepath.Join(tempDir, "valid-state.json")
	if err := SaveTelemetryStateFile(validFile, validSnap); err != nil {
		t.Fatalf("failed to save valid state file: %v", err)
	}

	loadedSnap, err := LoadTelemetryStateFile(validFile)
	if err != nil {
		t.Fatalf("failed to load valid state file: %v", err)
	}
	if loadedSnap.HealthStatus != validSnap.HealthStatus {
		t.Fatalf("expected health status %s, got %s", validSnap.HealthStatus, loadedSnap.HealthStatus)
	}
	if loadedSnap.ActiveHolonCount != validSnap.ActiveHolonCount {
		t.Fatalf("expected active holons %d, got %d", validSnap.ActiveHolonCount, loadedSnap.ActiveHolonCount)
	}

	// 2. Empty path fails closed
	if err := SaveTelemetryStateFile("", validSnap); !errors.Is(err, ErrStatePathRequired) {
		t.Fatalf("expected ErrStatePathRequired for empty save path, got %v", err)
	}
	if _, err := LoadTelemetryStateFile(""); !errors.Is(err, ErrStatePathRequired) {
		t.Fatalf("expected ErrStatePathRequired for empty load path, got %v", err)
	}

	// 3. Nil snapshot fails closed
	if err := SaveTelemetryStateFile(filepath.Join(tempDir, "nil.json"), nil); !errors.Is(err, ErrNilTelemetrySnapshot) {
		t.Fatalf("expected ErrNilTelemetrySnapshot, got %v", err)
	}

	// 4. Non-existent state file fails closed with actionable error receipt
	nonExistentFile := filepath.Join(tempDir, "does-not-exist.json")
	if _, err := LoadTelemetryStateFile(nonExistentFile); err == nil || !errors.Is(err, ErrCorruptAmbientState) {
		t.Fatalf("expected ErrCorruptAmbientState for missing file, got %v", err)
	}

	// 5. Empty state file (0 bytes) fails closed
	emptyFile := filepath.Join(tempDir, "empty-state.json")
	if err := fileutil.WriteStandardFile(emptyFile, []byte("")); err != nil {
		t.Fatalf("failed to write empty file: %v", err)
	}
	if _, err := LoadTelemetryStateFile(emptyFile); err == nil || !errors.Is(err, ErrEmptyAmbientState) {
		t.Fatalf("expected ErrEmptyAmbientState for empty file, got %v", err)
	}

	// 5b. Whitespace-only state file fails closed
	whitespaceFile := filepath.Join(tempDir, "whitespace-state.json")
	if err := fileutil.WriteStandardFile(whitespaceFile, []byte("   \n\t  \n")); err != nil {
		t.Fatalf("failed to write whitespace file: %v", err)
	}
	if _, err := LoadTelemetryStateFile(whitespaceFile); err == nil || !errors.Is(err, ErrEmptyAmbientState) {
		t.Fatalf("expected ErrEmptyAmbientState for whitespace file, got %v", err)
	}

	// 6. Truncated JSON fails closed with actionable receipt
	truncatedFile := filepath.Join(tempDir, "truncated-state.json")
	if err := fileutil.WriteStandardFile(truncatedFile, []byte(`{"timestamp": "2026-11-01T`)); err != nil {
		t.Fatalf("failed to write truncated file: %v", err)
	}
	if _, err := LoadTelemetryStateFile(truncatedFile); err == nil || !errors.Is(err, ErrCorruptAmbientState) {
		t.Fatalf("expected ErrCorruptAmbientState for truncated JSON, got %v", err)
	}

	// 7. Malformed / syntactically corrupt JSON fails closed
	malformedFile := filepath.Join(tempDir, "malformed-state.json")
	if err := fileutil.WriteStandardFile(malformedFile, []byte(`{"health_status": 12345, "active_holon_count": "not-a-number"`)); err != nil {
		t.Fatalf("failed to write malformed file: %v", err)
	}
	if _, err := LoadTelemetryStateFile(malformedFile); err == nil || !errors.Is(err, ErrCorruptAmbientState) {
		t.Fatalf("expected ErrCorruptAmbientState for malformed JSON, got %v", err)
	}

	// 8. Permission-denied / unreadable file fails closed (non-root Unix environments)
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		unreadableFile := filepath.Join(tempDir, "unreadable-state.json")
		if err := fileutil.WriteFile(unreadableFile, []byte(`{"health_status":"healthy"}`), 0o000); err == nil {
			defer fileutil.Chmod(unreadableFile, 0o600)
			if _, err := LoadTelemetryStateFile(unreadableFile); err == nil || !errors.Is(err, ErrCorruptAmbientState) {
				t.Fatalf("expected ErrCorruptAmbientState for unreadable file, got %v", err)
			}
		}
	}

	// 9. Aggregator PersistState and RestoreState integration
	agg := NewHolonTelemetryAggregator(nil, nil, nil)
	persistedPath := filepath.Join(tempDir, "agg-persisted.json")
	if err := agg.PersistState(context.Background(), persistedPath); err != nil {
		t.Fatalf("failed to persist aggregator state: %v", err)
	}

	restoredSnap, err := agg.RestoreState(persistedPath)
	if err != nil {
		t.Fatalf("failed to restore aggregator state: %v", err)
	}
	if restoredSnap == nil || restoredSnap.HealthStatus != TelemetryHealthQuiescent {
		t.Fatalf("unexpected restored snapshot: %+v", restoredSnap)
	}
	if len(agg.History()) != 2 { // 1 from PersistState(Snapshot) + 1 from RestoreState
		t.Fatalf("expected 2 history entries, got %d", len(agg.History()))
	}

	// 10. Nil aggregator PersistState fails closed safely
	var nilAgg *HolonTelemetryAggregator
	if err := nilAgg.PersistState(context.Background(), persistedPath); err == nil {
		t.Fatal("expected error calling PersistState on nil aggregator")
	}
}

// TestHolonTelemetry_UninitializedRiverStreamsAndNilReceivers satisfies CRIT-1793589814624127000-371d4759
// by proving that uninitialized river streams, nil hydrators, and nil convergence engines
// fail closed with deterministic error receipts and zero panics.
func TestHolonTelemetry_UninitializedRiverStreamsAndNilReceivers(t *testing.T) {
	// 1. ValidateRiverStream boundary checks
	if err := ValidateRiverStream(nil); !errors.Is(err, ErrUninitializedStream) {
		t.Fatalf("expected ErrUninitializedStream for nil river, got %v", err)
	}
	emptyIDRiver := &IntentRiver{ID: "", PooledVector: []float64{1.0}}
	if err := ValidateRiverStream(emptyIDRiver); !errors.Is(err, ErrUninitializedStream) {
		t.Fatalf("expected ErrUninitializedStream for empty river ID, got %v", err)
	}
	nilVectorRiver := &IntentRiver{ID: "river-valid", PooledVector: nil}
	if err := ValidateRiverStream(nilVectorRiver); !errors.Is(err, ErrUninitializedStream) {
		t.Fatalf("expected ErrUninitializedStream for nil vector, got %v", err)
	}
	validRiver := &IntentRiver{ID: "river-valid", PooledVector: []float64{1.0, 0.0}}
	if err := ValidateRiverStream(validRiver); err != nil {
		t.Fatalf("expected nil error for valid river, got %v", err)
	}

	// 2. ValidateIntentStream boundary checks
	if err := ValidateIntentStream(nil); !errors.Is(err, ErrUninitializedStream) {
		t.Fatalf("expected ErrUninitializedStream for nil stream, got %v", err)
	}
	emptyIDStream := &IntentStream{ID: ""}
	if err := ValidateIntentStream(emptyIDStream); !errors.Is(err, ErrUninitializedStream) {
		t.Fatalf("expected ErrUninitializedStream for empty stream ID, got %v", err)
	}
	validStream := &IntentStream{ID: "stream-valid"}
	if err := ValidateIntentStream(validStream); err != nil {
		t.Fatalf("expected nil error for valid stream, got %v", err)
	}

	// 3. Nil receiver resilience on RiverConvergenceEngine
	var nilEngine *RiverConvergenceEngine
	if err := nilEngine.IngestDrop(IntentDrop{MissionID: "mis-1"}); err != ErrEngineClosed {
		t.Fatalf("expected ErrEngineClosed for nil engine ingest, got %v", err)
	}
	if r, ok := nilEngine.GetRiver("any"); r != nil || ok {
		t.Fatalf("expected nil, false from nil engine GetRiver, got %v, %v", r, ok)
	}
	if rivers := nilEngine.ListRivers(); rivers != nil {
		t.Fatalf("expected nil rivers from nil engine ListRivers, got %v", rivers)
	}
	metrics := nilEngine.Metrics()
	if metrics.TotalDropsIngested != 0 || metrics.ActiveRiversCount != 0 {
		t.Fatalf("expected empty metrics from nil engine, got %+v", metrics)
	}
	if err := nilEngine.Close(); err != nil {
		t.Fatalf("expected nil error from nil engine Close, got %v", err)
	}

	// 4. Nil receiver resilience on StreamHydrator
	var nilHydrator *StreamHydrator
	if err := nilHydrator.PoolRaindrop(context.Background(), "stream-1", IntentRaindrop{ID: "drop-1"}); err == nil {
		t.Fatal("expected error from nil hydrator PoolRaindrop")
	}
	if s, ok := nilHydrator.GetStream("stream-1"); s != nil || ok {
		t.Fatalf("expected nil, false from nil hydrator GetStream, got %v, %v", s, ok)
	}
	if streams := nilHydrator.ListStreams(); streams != nil {
		t.Fatalf("expected nil streams from nil hydrator ListStreams, got %v", streams)
	}
	if _, err := nilHydrator.MergeStream("stream-1"); err == nil {
		t.Fatal("expected error from nil hydrator MergeStream")
	}

	// 5. Aggregator telemetry handles uninitialized/nil river streams without panic
	ctx := context.Background()
	engine, err := NewRiverConvergenceEngine(ctx, RiverConfig{
		RootVisionID:      "VIS-ROOT",
		RootVisionVector:  []float64{1.0, 0.0},
		ConvergenceWindow: 5 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer engine.Close()

	// Ingest an initial drop to create an active river
	_ = engine.IngestDrop(IntentDrop{
		ID:           "d1",
		MissionID:    "mis-uninit",
		IntentVector: []float64{1.0, 0.0},
		Weight:       1.0,
	})

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, ok := engine.GetRiver("river:mis-uninit"); ok {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Tamper internally with river to simulate corrupt/uninitialized pooled vector
	engine.mu.Lock()
	if r, exists := engine.rivers["river:mis-uninit"]; exists {
		r.PooledVector = nil // uninitialized
	}
	engine.mu.Unlock()

	agg := NewHolonTelemetryAggregator(nil, nil, engine)
	snap := agg.Snapshot(ctx)
	if snap == nil {
		t.Fatal("expected snapshot, got nil")
	}
	if !snap.HasWarnings() {
		t.Fatal("expected warnings for uninitialized river pooled vector")
	}
	hasUninitWarn := false
	for _, w := range snap.Warnings {
		if strings.Contains(w, "uninitialized pooled vector") {
			hasUninitWarn = true
			break
		}
	}
	if !hasUninitWarn {
		t.Fatalf("expected uninitialized pooled vector warning, got warnings: %v", snap.Warnings)
	}
}

// TestHolonTelemetry_ExhaustedTelemetryChannelsAndNilSubscribers satisfies CRIT-1793589814624127000-371d4759
// by proving that nil subscribers, saturated channel buffers, and rapid emission bursts
// fail safely without daemon panic, deadlocks, or goroutine leaks.
func TestHolonTelemetry_ExhaustedTelemetryChannelsAndNilSubscribers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	agg := NewHolonTelemetryAggregator(nil, nil, nil)

	// 1. Nil subscriber channel registration fails closed
	if err := agg.RegisterSubscriber("nil-sub", nil); !errors.Is(err, ErrNilTelemetryChannel) {
		t.Fatalf("expected ErrNilTelemetryChannel, got %v", err)
	}
	if err := agg.RegisterSubscriber("", make(chan *MiddleBrainTelemetrySnapshot, 1)); err == nil {
		t.Fatal("expected error for empty subscriber id")
	}

	// 2. Nil aggregator receiver registration and broadcast safety
	var nilAgg *HolonTelemetryAggregator
	if err := nilAgg.RegisterSubscriber("sub", make(chan *MiddleBrainTelemetrySnapshot, 1)); err == nil {
		t.Fatal("expected error registering on nil aggregator")
	}
	if nilAgg.UnregisterSubscriber("sub") {
		t.Fatal("expected false unregistering on nil aggregator")
	}
	if count := nilAgg.SubscriberCount(); count != 0 {
		t.Fatalf("expected 0 subscriber count on nil aggregator, got %d", count)
	}
	nilRes := nilAgg.Broadcast(&MiddleBrainTelemetrySnapshot{})
	if len(nilRes.Warnings) == 0 {
		t.Fatal("expected warnings broadcasting on nil aggregator")
	}

	// 3. EmitTelemetryToChannel nil checks and saturation boundary
	snap := &MiddleBrainTelemetrySnapshot{HealthStatus: TelemetryHealthHealthy}
	if err := EmitTelemetryToChannel(ctx, nil, snap); !errors.Is(err, ErrNilTelemetryChannel) {
		t.Fatalf("expected ErrNilTelemetryChannel, got %v", err)
	}
	testCh := make(chan *MiddleBrainTelemetrySnapshot, 1)
	if err := EmitTelemetryToChannel(ctx, testCh, nil); !errors.Is(err, ErrNilTelemetrySnapshot) {
		t.Fatalf("expected ErrNilTelemetrySnapshot, got %v", err)
	}

	// Fill single-capacity channel
	if err := EmitTelemetryToChannel(ctx, testCh, snap); err != nil {
		t.Fatalf("failed to emit initial snapshot: %v", err)
	}

	// Second emit must return ErrTelemetryChannelFull immediately (zero stall, zero leak)
	if err := EmitTelemetryToChannel(ctx, testCh, snap); !errors.Is(err, ErrTelemetryChannelFull) {
		t.Fatalf("expected ErrTelemetryChannelFull on saturated channel, got %v", err)
	}

	// 4. EmitTelemetryWithTimeout bounds
	if err := EmitTelemetryWithTimeout(ctx, testCh, snap, 20*time.Millisecond); !errors.Is(err, ErrTelemetryChannelFull) {
		t.Fatalf("expected ErrTelemetryChannelFull on timeout with saturated channel, got %v", err)
	}

	// Drain channel and verify EmitTelemetryWithTimeout succeeds
	<-testCh
	if err := EmitTelemetryWithTimeout(ctx, testCh, snap, 50*time.Millisecond); err != nil {
		t.Fatalf("expected success emitting with timeout to drained channel, got %v", err)
	}

	// 5. Multi-subscriber broadcast with mixed available, exhausted, and nil channels
	subAvailable := make(chan *MiddleBrainTelemetrySnapshot, 2)
	subExhausted := make(chan *MiddleBrainTelemetrySnapshot, 1)
	// Pre-fill subExhausted
	subExhausted <- snap

	if err := agg.RegisterSubscriber("healthy-sub", subAvailable); err != nil {
		t.Fatalf("failed to register healthy subscriber: %v", err)
	}
	if err := agg.RegisterSubscriber("exhausted-sub", subExhausted); err != nil {
		t.Fatalf("failed to register exhausted subscriber: %v", err)
	}
	if agg.SubscriberCount() != 2 {
		t.Fatalf("expected 2 subscribers, got %d", agg.SubscriberCount())
	}

	// Intentionally inject a nil subscriber in internal map to verify adversarial resilience
	agg.mu.Lock()
	agg.subscribers["nil-corrupt-sub"] = nil
	agg.mu.Unlock()

	// Record goroutine baseline
	goroutinesBefore := runtime.NumGoroutine()

	// Broadcast snapshot:
	// - healthy-sub receives (Delivered: 1)
	// - exhausted-sub drops without blocking (Exhausted: 1)
	// - nil-corrupt-sub skips without panic (Warning: 1)
	broadcastRes := agg.Broadcast(snap)
	if broadcastRes.Delivered != 1 {
		t.Fatalf("expected 1 delivered, got %d", broadcastRes.Delivered)
	}
	if broadcastRes.Exhausted != 1 {
		t.Fatalf("expected 1 exhausted, got %d", broadcastRes.Exhausted)
	}
	if len(broadcastRes.Warnings) < 2 { // 1 for exhausted, 1 for nil
		t.Fatalf("expected at least 2 warnings, got %v", broadcastRes.Warnings)
	}

	// Verify snapshot was actually received by healthy subscriber
	select {
	case received := <-subAvailable:
		if received.HealthStatus != TelemetryHealthHealthy {
			t.Fatalf("unexpected snapshot received: %+v", received)
		}
	default:
		t.Fatal("expected snapshot in healthy subscriber channel")
	}

	// 6. High-throughput saturation burst: verify zero goroutine leaks and zero deadlock
	for i := 0; i < 200; i++ {
		agg.Broadcast(snap)
	}

	// Wait briefly for any asynchronous activity to settle
	time.Sleep(20 * time.Millisecond)
	goroutinesAfter := runtime.NumGoroutine()
	// Goroutine count must not grow uncontrollably
	if delta := goroutinesAfter - goroutinesBefore; delta > 2 {
		t.Fatalf("detected potential goroutine leak: before=%d, after=%d (delta=%d)",
			goroutinesBefore, goroutinesAfter, delta)
	}

	// 7. Clean unregistration
	if !agg.UnregisterSubscriber("healthy-sub") {
		t.Fatal("expected true when unregistering healthy-sub")
	}
	if !agg.UnregisterSubscriber("exhausted-sub") {
		t.Fatal("expected true when unregistering exhausted-sub")
	}
	if agg.UnregisterSubscriber("nonexistent") {
		t.Fatal("expected false when unregistering nonexistent sub")
	}
}
