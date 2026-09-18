package storage

import (
	"testing"
	"time"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestWarmReverseReferenceIndex(t *testing.T) {
	tmpDir := t.TempDir()

	revIndex := NewReverseReferenceIndex()
	revIndex.AddReference("OBJ-1", "OBJ-TARGET")
	if err := revIndex.SaveCache(tmpDir); err != nil {
		t.Fatalf("failed to save cache: %v", err)
	}

	BindReverseReferenceIndexProjectRoot(tmpDir)
	WarmReverseReferenceIndex(tmpDir)

	// Wait up to 1 second for background warming
	deadline := time.Now().Add(1 * time.Second)
	warmed := false
	for time.Now().Before(deadline) {
		if GetGlobalReverseReferenceIndex().IsReady() {
			warmed = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !warmed {
		t.Fatalf("expected reverse reference index to be warmed within 1s")
	}

	FlushReverseReferenceIndexPersist()
	_ = fileutil.RemoveAll(tmpDir)
}
