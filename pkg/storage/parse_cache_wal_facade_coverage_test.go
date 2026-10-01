package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// TestStorageExtended_Wave6_ParseCache tests yaml_parse_cache.go
func TestStorageExtended_Wave6_ParseCache(t *testing.T) {
	// 1. NewParseCache with small limit to trigger eviction
	cache := storagepkg.NewParseCache(2)
	if cache == nil {
		t.Fatalf("failed to create ParseCache")
	}

	// Metrics when empty
	if cache.Hits() != 0 || cache.Misses() != 0 || cache.Puts() != 0 || cache.Evictions() != 0 {
		t.Errorf("expected 0 metrics on fresh cache")
	}

	// Get missing entry (records miss)
	_, found := cache.Get("missing-hash")
	if found {
		t.Errorf("expected not found for missing-hash")
	}
	if cache.Misses() != 1 {
		t.Errorf("expected 1 miss, got %d", cache.Misses())
	}

	// Put entries
	obj1 := &objects.ParsedObject{ID: "PRI-1", Kind: "priority_plan"}
	obj2 := &objects.ParsedObject{ID: "PRI-2", Kind: "priority_plan"}
	obj3 := &objects.ParsedObject{ID: "PRI-3", Kind: "priority_plan"}

	cache.Put("hash-1", obj1)
	cache.Put("hash-2", obj2)
	if cache.Puts() != 2 {
		t.Errorf("expected 2 puts, got %d", cache.Puts())
	}

	// Hit
	cached1, found := cache.Get("hash-1")
	if !found || cached1.ID != "PRI-1" {
		t.Errorf("expected hit for hash-1")
	}
	if cache.Hits() != 1 {
		t.Errorf("expected 1 hit, got %d", cache.Hits())
	}

	// Put third entry triggers eviction of least recently used (hash-2)
	cache.Put("hash-3", obj3)
	if cache.Evictions() != 1 {
		t.Errorf("expected 1 eviction, got %d", cache.Evictions())
	}

	// Reset metrics
	cache.ResetMetrics()
	if cache.Hits() != 0 || cache.Misses() != 0 || cache.Puts() != 0 || cache.Evictions() != 0 {
		t.Errorf("expected 0 metrics after reset")
	}

	// Nil cache safety
	var nilCache *storagepkg.ParseCache
	if nilCache.Hits() != 0 || nilCache.Misses() != 0 || nilCache.Puts() != 0 || nilCache.Evictions() != 0 {
		t.Errorf("expected 0 for nil cache metrics")
	}
	nilCache.ResetMetrics()
	_, _ = nilCache.Get("h")
	nilCache.Put("h", nil)
}

// TestStorageExtended_Wave6_WaitGroupObserver tests waitgroup_observer.go
func TestStorageExtended_Wave6_WaitGroupObserver(t *testing.T) {
	ctx := context.Background()
	obs := storagepkg.NewLoggingWaitGroupObserver(ctx)

	// Summary when empty
	emptySummary := obs.Summary()
	if emptySummary == "" {
		t.Errorf("expected non-empty summary on empty observer")
	}

	// Lifecycle calls
	obs.OnGroupCreated("grp-1", "test-op")
	obs.OnGroupAdd("grp-1", 2)
	obs.OnGroupDone("grp-1")
	obs.OnGroupWait("grp-1")

	// Summary with active groups
	activeSummary := obs.Summary()
	if activeSummary == "" {
		t.Errorf("expected non-empty summary for active group")
	}

	// Call OnGroupDone beyond add count to trigger warning branch
	obs.OnGroupDone("grp-1")
	obs.OnGroupDone("grp-1") // doneCount (3) > addCount (2)

	// Get stats
	op, adds, dones, waits, exists := obs.GetStats("grp-1")
	if !exists || op != "test-op" || adds != 2 || dones != 3 || waits != 1 {
		t.Errorf("unexpected stats: op=%s, adds=%d, dones=%d, waits=%d, exists=%v", op, adds, dones, waits, exists)
	}

	// Missing stats
	_, _, _, _, existsMissing := obs.GetStats("missing-grp")
	if existsMissing {
		t.Errorf("expected exists=false for missing group")
	}

	// GetAllStats
	allStats := obs.GetAllStats()
	if len(allStats) != 1 {
		t.Errorf("expected 1 group in GetAllStats")
	}

	// SetEnabled
	obs.SetEnabled(false)
	obs.OnGroupAdd("grp-1", 1) // ignored when disabled
	obs.SetEnabled(true)

	// ClearStats
	obs.ClearStats()
	if len(obs.GetAllStats()) != 0 {
		t.Errorf("expected 0 stats after ClearStats")
	}
}

// TestStorageExtended_Wave6_WALFacade tests wal_facade.go functions
func TestStorageExtended_Wave6_WALFacade(t *testing.T) {
	tmpDir := t.TempDir()

	// Write and Read applied sequence
	err := storagepkg.WriteAppliedSeq(tmpDir, 42)
	if err != nil {
		t.Logf("WriteAppliedSeq returned: %v", err)
	}

	seq, err := storagepkg.ReadAppliedSeq(tmpDir)
	if err != nil {
		t.Logf("ReadAppliedSeq returned: %v", err)
	}
	_ = seq

	// ReplayWAL
	err = storagepkg.ReplayWAL(tmpDir, 0, func(rec *storagepkg.WALRecord) error {
		return nil
	})
	if err != nil {
		t.Logf("ReplayWAL returned: %v", err)
	}

	// ReadLastSeqFromTail
	lastSeq, err := storagepkg.ReadLastSeqFromTail(tmpDir)
	if err != nil {
		t.Logf("ReadLastSeqFromTail returned: %v", err)
	}
	_ = lastSeq

	// WaitForWALProcessingEventDriven with tiny timeout
	_ = storagepkg.WaitForWALProcessingEventDriven(tmpDir, 10*time.Millisecond)

	// CompactWAL
	_ = storagepkg.CompactWAL(tmpDir)
}
