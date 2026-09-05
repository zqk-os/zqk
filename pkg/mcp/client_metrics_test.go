package mcp

import (
	"context"
	"path/filepath"
	"testing"
)

func TestClientMetricsStore_LifetimeCounters(t *testing.T) {
	tmpDir := t.TempDir()
	metricsFile := filepath.Join(tmpDir, "client_metrics.json")

	store, err := NewClientMetricsStore(metricsFile, context.Background())
	if err != nil {
		t.Fatalf("failed to create ClientMetricsStore: %v", err)
	}

	eBefore, sBefore := store.GetClientMetricsStats()
	if eBefore != 0 || sBefore != 0 {
		t.Fatalf("expected initial stats (0, 0), got events=%d sequences=%d", eBefore, sBefore)
	}

	// Record an event with clientID -> 1 sequence, 1 event
	if err := store.RecordEvent("seq-001", "client-A", "initialize", map[string]any{"tools_count": 5}); err != nil {
		t.Fatalf("failed to record event: %v", err)
	}

	eAfter1, sAfter1 := store.GetClientMetricsStats()
	if sAfter1 != 1 {
		t.Errorf("expected 1 sequence created, got %d", sAfter1)
	}
	if eAfter1 != 1 {
		t.Errorf("expected 1 event recorded, got %d", eAfter1)
	}

	// Record a second event on same sequence -> same 1 sequence, 2 events
	if err := store.RecordEvent("seq-001", "client-A", "tools_list", map[string]any{"tools_count": 5}); err != nil {
		t.Fatalf("failed to record second event: %v", err)
	}

	eAfter2, sAfter2 := store.GetClientMetricsStats()
	if sAfter2 != 1 {
		t.Errorf("expected 1 sequence created, got %d", sAfter2)
	}
	if eAfter2 != 2 {
		t.Errorf("expected 2 events recorded, got %d", eAfter2)
	}

	// Record an event on a new sequence -> 2 sequences, 3 events
	if err := store.RecordEvent("seq-002", "client-B", "initialize", map[string]any{"tools_count": 2}); err != nil {
		t.Fatalf("failed to record third event: %v", err)
	}

	eAfter3, sAfter3 := store.GetClientMetricsStats()
	if sAfter3 != 2 {
		t.Errorf("expected 2 sequences created, got %d", sAfter3)
	}
	if eAfter3 != 3 {
		t.Errorf("expected 3 events recorded, got %d", eAfter3)
	}
}
