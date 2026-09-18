package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAncillaryCacheDurableFsync(t *testing.T) {
	tmpDir, err := fileutil.MkdirTemp("", "ancillary-cache-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tmpDir)

	// Test 1: ReverseReferenceIndex SaveCache durability with fsync
	revIndex := NewReverseReferenceIndex()
	revIndex.AddReference("OBJ-B", "OBJ-A")
	revIndex.AddReference("OBJ-C", "OBJ-A")

	if err := revIndex.SaveCache(tmpDir); err != nil {
		t.Fatalf("ReverseReferenceIndex.SaveCache failed: %v", err)
	}

	cacheFile := revIndex.getCacheFilePath(tmpDir)
	if !fileutil.Exists(cacheFile) {
		t.Fatalf("expected cache file to exist at %s", cacheFile)
	}

	// Verify loaded cache matches
	newRevIndex := NewReverseReferenceIndex()
	ok, err := newRevIndex.LoadCache(tmpDir)
	if err != nil || !ok {
		t.Fatalf("failed to load saved reverse reference cache: ok=%v, err=%v", ok, err)
	}
	dependents := newRevIndex.GetDependents("OBJ-A")
	if len(dependents) != 2 {
		t.Fatalf("expected 2 dependents, got %d", len(dependents))
	}

	// Test 2: HighVolumeEventCache SaveCache durability with fsync
	hveCache := NewHighVolumeEventCache()
	hveCache.Set(&HighVolumeEventCacheEntry{
		ID:        "EVT-001",
		Kind:      "audit_event",
		CreatedAt: time.Now().UTC(),
		EventType: "steering",
		FilePath:  filepath.Join(tmpDir, "evt.yaml"),
		Exists:    true,
	})

	if err := hveCache.SaveCache(tmpDir); err != nil {
		t.Fatalf("HighVolumeEventCache.SaveCache failed: %v", err)
	}

	hveCacheFile := hveCache.getCacheFilePath(tmpDir)
	if !fileutil.Exists(hveCacheFile) {
		t.Fatalf("expected high volume cache file to exist at %s", hveCacheFile)
	}

	newHveCache := NewHighVolumeEventCache()
	if ok, err := newHveCache.LoadCache(tmpDir); err != nil || !ok {
		t.Fatalf("failed to load high volume event cache: ok=%v, err=%v", ok, err)
	}
	entry, found := newHveCache.Get("EVT-001")
	if !found || entry == nil || !entry.Exists {
		t.Fatalf("expected EVT-001 to be found in loaded cache")
	}
}
