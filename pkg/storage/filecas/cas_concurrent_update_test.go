package filecas

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestCASConcurrentUpdate_NoLostIndexOrExtraBlobs verifies that concurrent Update
// calls on the same object ID serialize under WithSweepLock without TOCTOU races,
// lost index mappings, or leftover orphan blobs.
func TestCASConcurrentUpdate_NoLostIndexOrExtraBlobs(t *testing.T) {
	prevMetrics := GetMetrics
	GetMetrics = func() StorageMetrics { return noopStorageMetrics{} }
	prevPending := NoteObjectIDCachePending
	NoteObjectIDCachePending = func(string, string, string, string, string, string) {}
	prevVis := GetCASPendingVisibilityCache
	GetCASPendingVisibilityCache = func(string) PendingVisibilityCache { return noopPendingCache{} }
	prevHV := IsHighVolumeKindForCache
	IsHighVolumeKindForCache = func(string) bool { return false }
	t.Cleanup(func() {
		GetMetrics = prevMetrics
		NoteObjectIDCachePending = prevPending
		GetCASPendingVisibilityCache = prevVis
		IsHighVolumeKindForCache = prevHV
	})

	kindDir := filepath.Join(t.TempDir(), ".zqk", "process", "backlog_item")
	if err := fileutil.MkdirAll(kindDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	cas := NewContentAddressableStorage(kindDir, "backlog_item", immediateMockIndexQueue{})
	cas.SetPostSyncCallback(func(string, string, string, string) error { return nil })

	objectID := "BLI-CONCURRENT-UPDATE-RACE"
	initial := []byte("id: BLI-CONCURRENT-UPDATE-RACE\nstatus: planned\nversion: 0\n")
	if err := cas.Update(objectID, initial); err != nil {
		t.Fatalf("initial Update failed: %v", err)
	}

	const workers = 8
	const iterations = 8
	var wg sync.WaitGroup
	errCh := make(chan error, workers*iterations)

	for w := range workers {
		workerID := w
		wg.Add(1)
		goroutinelabels.NewGoroutine("cas_concurrent_update_test", "concurrent CAS update worker").StartSimple(func() {
			defer wg.Done()
			for i := range iterations {
				payload := fmt.Appendf(nil, "id: %s\nstatus: testing\nworker: %d\niter: %d\n", objectID, workerID, i)
				if err := cas.Update(objectID, payload); err != nil {
					errCh <- fmt.Errorf("worker %d iter %d: %w", workerID, i, err)
				}
			}
		})
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent Update error: %v", err)
	}

	finalHash, err := cas.GetHashForID(objectID)
	if err != nil {
		t.Fatalf("GetHashForID failed after concurrent updates: %v", err)
	}
	finalBlob := filepath.Join(kindDir, finalHash+".yaml")
	if !fileutil.Exists(finalBlob) {
		t.Fatalf("expected live blob %s to exist", finalBlob)
	}

	entries, err := fileutil.ReadDir(kindDir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	var yamlCount int
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
			yamlCount++
		}
	}
	if yamlCount != 1 {
		t.Fatalf("expected exactly 1 live .yaml blob after concurrent updates, got %d", yamlCount)
	}
}
