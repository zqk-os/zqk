package accumulator

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// SamplePayload models test domain state.
type SamplePayload struct {
	TotalCount   int               `json:"total_count"`
	ActiveItems  []string          `json:"active_items"`
	ItemStatuses map[string]string `json:"item_statuses"`
}

// MockAccumulator implements Accumulator[SamplePayload] for testing.
type MockAccumulator struct {
	mu           sync.RWMutex
	name         string
	itemStatuses map[string]string
	scanCount    uint32
	scanBlockCh  chan struct{}
}

func NewMockAccumulator(name string) *MockAccumulator {
	return &MockAccumulator{
		name:         name,
		itemStatuses: make(map[string]string),
	}
}

func (m *MockAccumulator) Name() string {
	return m.name
}

func (m *MockAccumulator) DefaultPayload() SamplePayload {
	return SamplePayload{
		TotalCount:   0,
		ActiveItems:  make([]string, 0),
		ItemStatuses: make(map[string]string),
	}
}

func (m *MockAccumulator) ApplyEvent(ev *lifecycle.LifecycleEvent) bool {
	if ev == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	targetID := ev.ID
	if targetID == "" {
		return false
	}

	newStatus := ev.ToStatus
	if newStatus == "" {
		newStatus = objects.ObjectStatusActive
	}

	m.itemStatuses[targetID] = newStatus
	return true
}

func (m *MockAccumulator) BuildPayload() SamplePayload {
	m.mu.RLock()
	defer m.mu.RUnlock()

	active := make([]string, 0)
	statuses := make(map[string]string, len(m.itemStatuses))
	for id, status := range m.itemStatuses {
		statuses[id] = status
		if status == objects.ObjectStatusActive {
			active = append(active, id)
		}
	}

	return SamplePayload{
		TotalCount:   len(m.itemStatuses),
		ActiveItems:  active,
		ItemStatuses: statuses,
	}
}

func (m *MockAccumulator) ScanFromStorage(ctx context.Context, sp storage.ObjectStorageProvider) error {
	if m.scanBlockCh != nil {
		select {
		case <-m.scanBlockCh:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	atomic.AddUint32(&m.scanCount, 1)
	m.mu.Lock()
	defer m.mu.Unlock()

	// Simulate populating state during full scan
	m.itemStatuses["REBUILT-001"] = objects.ObjectStatusActive
	m.itemStatuses["REBUILT-002"] = objects.ObjectStatusComplete
	return nil
}

// MockStorageProvider satisfies storage.ObjectStorageProvider for testing.
type MockStorageProvider struct {
	storage.ObjectStorageProvider
}

// TestAccumulator_BuilderAndEngineConfig verifies fluent builder validation and config.
func TestAccumulator_BuilderAndEngineConfig(t *testing.T) {
	acc := NewMockAccumulator("test_metrics")
	tmpDir := t.TempDir()

	engine, err := NewBuilder[SamplePayload](acc).
		WithProjectRoot(tmpDir).
		WithSchemaVersion("2.1.0").
		WithStalenessTolerance(30 * time.Second).
		WithRebuildTimeout(15 * time.Second).
		WithPollInterval(100 * time.Millisecond).
		Build()

	if err != nil {
		t.Fatalf("unexpected error building engine: %v", err)
	}

	spec := engine.Spec()
	if spec.Name != "test_metrics" {
		t.Errorf("expected name 'test_metrics', got %q", spec.Name)
	}
	if spec.SchemaVersion != "2.1.0" {
		t.Errorf("expected schema '2.1.0', got %q", spec.SchemaVersion)
	}
	if spec.StalenessTolerance != 30*time.Second {
		t.Errorf("expected tolerance 30s, got %v", spec.StalenessTolerance)
	}

	expectedPath := filepath.Join(tmpDir, paths.ProjectDataDir, paths.StateDir, "test_metrics_lite.json")
	if engine.StoragePath() != expectedPath {
		t.Errorf("expected storage path %q, got %q", expectedPath, engine.StoragePath())
	}

	// Error case: nil accumulator
	_, err = NewBuilder[SamplePayload](nil).Build()
	if err == nil {
		t.Error("expected error with nil accumulator")
	}
}

// TestAccumulator_AtomicPersistenceAndHotPathSLA satisfies CRIT-1789551839272714000-27a0ccc0.
func TestAccumulator_AtomicPersistenceAndHotPathSLA(t *testing.T) {
	tmpDir := t.TempDir()
	acc := NewMockAccumulator("latency_test")
	acc.itemStatuses["ITEM-1"] = objects.ObjectStatusActive
	acc.itemStatuses["ITEM-2"] = objects.ObjectStatusActive

	engine, err := NewBuilder[SamplePayload](acc).
		WithProjectRoot(tmpDir).
		Build()
	if err != nil {
		t.Fatalf("build engine: %v", err)
	}

	// 1. Atomic Save
	if err := engine.SaveToLiteFile(); err != nil {
		t.Fatalf("SaveToLiteFile failed: %v", err)
	}

	// 2. Hot-path read SLA verification (< 5ms)
	ctx := context.Background()
	sp := &MockStorageProvider{}

	start := time.Now()
	iterations := 100
	for i := 0; i < iterations; i++ {
		envelope, err := engine.GetOrRecoverPayload(ctx, sp)
		if err != nil {
			t.Fatalf("GetOrRecoverPayload error: %v", err)
		}
		if envelope.Stale || envelope.Recovering {
			t.Fatalf("unexpected degraded flags on fresh file: %+v", envelope)
		}
		if envelope.Payload.TotalCount != 2 {
			t.Fatalf("expected count 2, got %d", envelope.Payload.TotalCount)
		}
	}
	elapsed := time.Since(start)
	avgLatency := elapsed / time.Duration(iterations)

	t.Logf("Hot-path average read latency: %v (total %v for %d reads)", avgLatency, elapsed, iterations)
	if avgLatency > 5*time.Millisecond {
		t.Errorf("hot-path read SLA violated: avg latency %v exceeds 5ms limit", avgLatency)
	}
}

// TestAccumulator_ColdBootAsyncRecovery satisfies CRIT-1789551842110690000-ee7b440f.
func TestAccumulator_ColdBootAsyncRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	acc := NewMockAccumulator("cold_boot_test")

	engine, err := NewBuilder[SamplePayload](acc).
		WithProjectRoot(tmpDir).
		Build()
	if err != nil {
		t.Fatalf("build engine: %v", err)
	}

	ctx := context.Background()
	sp := &MockStorageProvider{}

	// File does not exist yet -> Must return cold boot degraded envelope immediately
	envelope, err := engine.GetOrRecoverPayload(ctx, sp)
	if err != nil {
		t.Fatalf("GetOrRecoverPayload failed: %v", err)
	}

	if !envelope.Stale {
		t.Error("expected Stale=true on missing file")
	}
	if !envelope.Recovering {
		t.Error("expected Recovering=true on missing file")
	}
	if envelope.DegradedReason != "cold_boot_materialized_projection_missing" {
		t.Errorf("unexpected degraded reason: %q", envelope.DegradedReason)
	}

	// Wait for async background rebuild to finish
	deadline := time.Now().Add(3 * time.Second)
	var recovered *Envelope[SamplePayload]
	for time.Now().Before(deadline) {
		env, err := engine.GetOrRecoverPayload(ctx, sp)
		if err == nil && !env.Stale && !env.Recovering {
			recovered = env
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if recovered == nil {
		t.Fatal("timed out waiting for asynchronous recovery to populate fresh projection")
	}
	if recovered.Payload.TotalCount != 2 {
		t.Errorf("expected 2 rebuilt items, got %d", recovered.Payload.TotalCount)
	}
}

// TestAccumulator_StalenessCircuitBreaker verifies non-blocking async recovery when stale.
func TestAccumulator_StalenessCircuitBreaker(t *testing.T) {
	tmpDir := t.TempDir()
	acc := NewMockAccumulator("staleness_test")
	acc.itemStatuses["INIT-1"] = objects.ObjectStatusActive

	engine, err := NewBuilder[SamplePayload](acc).
		WithProjectRoot(tmpDir).
		WithStalenessTolerance(100 * time.Millisecond).
		Build()
	if err != nil {
		t.Fatalf("build engine: %v", err)
	}

	// Persist initial state
	if err := engine.SaveToLiteFile(); err != nil {
		t.Fatalf("save initial lite file: %v", err)
	}

	// Sleep past staleness tolerance
	time.Sleep(150 * time.Millisecond)

	ctx := context.Background()
	sp := &MockStorageProvider{}

	// Read must return stale envelope without blocking
	start := time.Now()
	envelope, err := engine.GetOrRecoverPayload(ctx, sp)
	latency := time.Since(start)

	if err != nil {
		t.Fatalf("GetOrRecoverPayload failed: %v", err)
	}
	if latency > 100*time.Millisecond {
		t.Errorf("read stalled during staleness circuit breaker: %v", latency)
	}
	if !envelope.Stale || !envelope.Recovering {
		t.Errorf("expected stale=true, recovering=true, got %+v", envelope)
	}

	// Wait for background recovery to complete
	deadline := time.Now().Add(3 * time.Second)
	var freshEnv *Envelope[SamplePayload]
	for time.Now().Before(deadline) {
		env, err := engine.GetOrRecoverPayload(ctx, sp)
		if err == nil && !env.Stale {
			freshEnv = env
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if freshEnv == nil {
		t.Fatal("async reconciler failed to refresh stale projection")
	}
}

// TestAccumulator_DebounceConcurrency verifies that only one background rebuild runs.
func TestAccumulator_DebounceConcurrency(t *testing.T) {
	tmpDir := t.TempDir()
	acc := NewMockAccumulator("debounce_test")
	blockCh := make(chan struct{})
	var closeOnce sync.Once
	acc.scanBlockCh = blockCh
	defer closeOnce.Do(func() { close(blockCh) })

	engine, err := NewBuilder[SamplePayload](acc).
		WithProjectRoot(tmpDir).
		Build()
	if err != nil {
		t.Fatalf("build engine: %v", err)
	}

	sp := &MockStorageProvider{}
	first := engine.TriggerAsyncRebuild(sp)
	second := engine.TriggerAsyncRebuild(sp)
	third := engine.TriggerAsyncRebuild(sp)

	if !first {
		t.Error("expected first TriggerAsyncRebuild to return true")
	}
	if second {
		t.Error("expected second TriggerAsyncRebuild to be debounced (return false)")
	}
	if third {
		t.Error("expected third TriggerAsyncRebuild to be debounced (return false)")
	}

	// Unblock the background reconciler now that debounce assertions are complete
	closeOnce.Do(func() { close(blockCh) })

	// Wait for the in-flight background reconciler to finish before test cleanup removes tmpDir
	if !engine.WaitUntilIdle(5 * time.Second) {
		t.Fatal("reconciler did not finish within deadline")
	}
}

// TestAccumulator_WALSubscription satisfies CRIT-1789551839272714000-27a0ccc0 incremental WAL ingestion.
func TestAccumulator_WALSubscription(t *testing.T) {
	tmpDir := t.TempDir()
	walDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.StateDir)
	if err := fileutil.MkdirAll(walDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}

	wal, err := lifecycle.GetOrCreateLifecycleWAL(tmpDir)
	if err != nil {
		t.Fatalf("create wal: %v", err)
	}

	acc := NewMockAccumulator("wal_stream_test")
	engine, err := NewBuilder[SamplePayload](acc).
		WithProjectRoot(tmpDir).
		WithPollInterval(50 * time.Millisecond).
		Build()
	if err != nil {
		t.Fatalf("build engine: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	updateCh := make(chan struct{}, 10)
	engine.StartBackgroundWALSubscriber(ctx, updateCh)

	// Emit lifecycle event to WAL
	event := &lifecycle.LifecycleEvent{
		Ts:        time.Now().UTC(),
		EventType: lifecycle.EventTypeStatusTransition,
		ID:        "TASK-WAL-001",
		Kind:      objects.KindAgentTask,
		ToStatus:  objects.ObjectStatusActive,
	}
	if err := wal.Append(event); err != nil {
		t.Fatalf("append wal event: %v", err)
	}
	if err := wal.Sync(); err != nil {
		t.Fatalf("sync wal event: %v", err)
	}

	// Expect subscriber to notify updateCh
	select {
	case <-updateCh:
		// Success
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for WAL subscriber to process event")
	}

	// Verify lite file updated
	envelope, err := engine.LoadFromLiteFile()
	if err != nil {
		t.Fatalf("load lite file: %v", err)
	}
	if envelope.Payload.TotalCount != 1 {
		t.Errorf("expected 1 item in lite file, got %d", envelope.Payload.TotalCount)
	}
	if envelope.Payload.ItemStatuses["TASK-WAL-001"] != objects.ObjectStatusActive {
		t.Errorf("expected status 'active', got %q", envelope.Payload.ItemStatuses["TASK-WAL-001"])
	}
}

func TestEngine_DualFormatDeserialization(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	spec := AccumulatorSpec{
		Name:               "mock_view",
		SchemaVersion:      "1.0.0",
		ProjectRoot:        tempDir,
		StoragePath:        filepath.Join(tempDir, paths.ProjectDataDir, paths.StateDir, "mock_view_lite.json"),
		StalenessTolerance: 2 * time.Minute,
	}

	acc := NewMockAccumulator("mock_view")
	engine, err := NewEngine[SamplePayload](spec, acc)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	if err := fileutil.MkdirAll(filepath.Dir(spec.StoragePath), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}

	// Test Case 1: Legacy flat JSON format (no "payload" envelope wrapper)
	legacyTimestamp := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	legacyFlat := map[string]any{
		objects.FieldKeySchemaVersion: "1.0.0",
		"materialized_at":             legacyTimestamp.Format(time.RFC3339Nano),
		"total_count":                 42,
		"item_statuses": map[string]string{
			"ITEM-FLAT-001": "completed",
		},
	}
	legacyFlatJSON, err := json.Marshal(legacyFlat)
	if err != nil {
		t.Fatalf("marshal legacy flat: %v", err)
	}

	if err := fileutil.WriteFile(spec.StoragePath, legacyFlatJSON, paths.FilePerm644); err != nil {
		t.Fatalf("write legacy flat file: %v", err)
	}

	env, err := engine.LoadFromLiteFile()
	if err != nil {
		t.Fatalf("LoadFromLiteFile failed on legacy flat JSON: %v", err)
	}
	if env == nil {
		t.Fatal("expected non-nil envelope from legacy flat JSON")
	}
	if env.Payload.TotalCount != 42 {
		t.Errorf("expected TotalCount 42, got %d", env.Payload.TotalCount)
	}
	if env.Payload.ItemStatuses["ITEM-FLAT-001"] != "completed" {
		t.Errorf("expected ITEM-FLAT-001 'completed', got %q", env.Payload.ItemStatuses["ITEM-FLAT-001"])
	}
	if !env.MaterializedAt.Equal(legacyTimestamp) {
		t.Errorf("expected MaterializedAt %v, got %v", legacyTimestamp, env.MaterializedAt)
	}

	// Test Case 2: Standard Envelope[T] wrapped JSON format
	envelopeTimestamp := time.Date(2026, 9, 16, 15, 30, 0, 0, time.UTC)
	envelopeData := map[string]any{
		objects.FieldKeySchemaVersion: "1.0.0",
		"materialized_at":             envelopeTimestamp.Format(time.RFC3339Nano),
		objects.FieldKeyPayload: map[string]any{
			"total_count": 99,
			"item_statuses": map[string]string{
				"ITEM-ENV-001": "in_progress",
			},
		},
	}
	envelopeJSON, err := json.Marshal(envelopeData)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	if err := fileutil.WriteFile(spec.StoragePath, envelopeJSON, paths.FilePerm644); err != nil {
		t.Fatalf("write envelope file: %v", err)
	}

	env2, err := engine.LoadFromLiteFile()
	if err != nil {
		t.Fatalf("LoadFromLiteFile failed on envelope JSON: %v", err)
	}
	if env2 == nil {
		t.Fatal("expected non-nil envelope from envelope JSON")
	}
	if env2.Payload.TotalCount != 99 {
		t.Errorf("expected TotalCount 99, got %d", env2.Payload.TotalCount)
	}
	if env2.Payload.ItemStatuses["ITEM-ENV-001"] != "in_progress" {
		t.Errorf("expected ITEM-ENV-001 'in_progress', got %q", env2.Payload.ItemStatuses["ITEM-ENV-001"])
	}
	if !env2.MaterializedAt.Equal(envelopeTimestamp) {
		t.Errorf("expected MaterializedAt %v, got %v", envelopeTimestamp, env2.MaterializedAt)
	}
}
