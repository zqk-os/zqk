package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestChangeJournalIDCache_LifetimeCounters(t *testing.T) {
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, ".zqk", "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	cache := getChangeJournalIDCache(context.Background(), tmpDir)

	alloc, disp := cache.GetIDCacheStats()
	if alloc != 0 || disp != 0 {
		t.Errorf("expected (0, 0), got (%d, %d)", alloc, disp)
	}

	id, err := cache.nextID(context.Background())
	if err != nil {
		t.Fatalf("nextID failed: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty ID")
	}

	alloc, disp = cache.GetIDCacheStats()
	if alloc != changeJournalIDBatchSize || disp != 1 {
		t.Errorf("expected alloc=%d disp=1, got alloc=%d disp=%d", changeJournalIDBatchSize, alloc, disp)
	}

	createdInit, reusedInit := GetGlobalChangeJournalIDCacheStats()

	// Get cache again for same project -> reused counter should increment
	cache2 := getChangeJournalIDCache(context.Background(), tmpDir)
	if cache2 != cache {
		t.Fatalf("expected cached instance")
	}

	createdAfter, reusedAfter := GetGlobalChangeJournalIDCacheStats()
	if createdAfter < createdInit || reusedAfter <= reusedInit {
		t.Fatalf("expected reused counter to increment, got init=(%d,%d) after=(%d,%d)", createdInit, reusedInit, createdAfter, reusedAfter)
	}
}
