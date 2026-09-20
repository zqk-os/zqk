package filecas

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func writeMockCASYAML(t *testing.T, dir, id string) (string, string) {
	t.Helper()
	return writeMockCASYAMLAt(t, dir, id)
}

func writeMockCASYAMLAt(t *testing.T, dir, id string) (string, string) {
	t.Helper()
	body := []byte("id: " + id + "\nkind: goal\n")
	hash := CalculateSHA256Hash(body)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, hash+".yaml")
	if err := fileutil.WriteSecureFile(path, body); err != nil {
		t.Fatal(err)
	}
	return path, hash
}

func TestGetHashForID_EmptyID(t *testing.T) {
	t.Parallel()
	kindDir := t.TempDir()
	cas := NewContentAddressableStorage(kindDir, "goal")

	_, err := cas.GetHashForID("")
	if err == nil {
		t.Fatal("expected error for empty ID, got nil")
	}

	_, err = cas.GetHashForID("   ")
	if err == nil {
		t.Fatal("expected error for whitespace-only ID, got nil")
	}
}

func TestGetHashForID_UnpopulatedIndex_PopulatesInBulk(t *testing.T) {
	t.Parallel()
	kindDir := t.TempDir()

	// Create 5 hash files on disk without creating an index file
	ids := []string{"GOAL-BULK-1", "GOAL-BULK-2", "GOAL-BULK-3", "GOAL-BULK-4", "GOAL-BULK-5"}
	expectedHashes := make(map[string]string)
	for _, id := range ids {
		_, h := writeMockCASYAML(t, kindDir, id)
		expectedHashes[id] = h
	}

	cas := NewContentAddressableStorage(kindDir, "goal")
	if cas.index.Len() != 0 {
		t.Fatalf("expected initial index to have 0 mappings, got %d", cas.index.Len())
	}

	// Looking up one object should trigger bulk population of the unpopulated cache
	hash, err := cas.GetHashForID(ids[0])
	if err != nil {
		t.Fatalf("GetHashForID(%q) error: %v", ids[0], err)
	}
	if hash != expectedHashes[ids[0]] {
		t.Fatalf("got hash %q, want %q", hash, expectedHashes[ids[0]])
	}

	// Index should now contain all 5 objects in memory
	if cas.index.Len() != 5 {
		t.Fatalf("expected index to have 5 mappings populated in bulk, got %d", cas.index.Len())
	}

	// All other objects should now resolve directly from the populated index
	for _, id := range ids[1:] {
		h, err := cas.GetHashForID(id)
		if err != nil {
			t.Fatalf("GetHashForID(%q) error: %v", id, err)
		}
		if h != expectedHashes[id] {
			t.Fatalf("got hash %q, want %q for %s", h, expectedHashes[id], id)
		}
	}
}

func TestEnsureIndexPopulated_EmptyDirectoryOnlyScansOnce(t *testing.T) {
	t.Parallel()
	kindDir := t.TempDir()
	cas := NewContentAddressableStorage(kindDir, "goal")

	if err := cas.EnsureIndexPopulated(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cas.indexPopulated.Load() {
		t.Fatal("expected indexPopulated to be true after EnsureIndexPopulated")
	}

	// Calling again should immediately return without error
	if err := cas.EnsureIndexPopulated(); err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
}

func TestGetHashForID_NegativeCacheAndCooldown(t *testing.T) {
	t.Parallel()
	kindDir := t.TempDir()
	cas := NewContentAddressableStorage(kindDir, "goal")

	// First query for non-existent ID
	start := time.Now()
	_, err := cas.GetHashForID("NONEXISTENT-1")
	if err == nil {
		t.Fatal("expected not found error")
	}
	firstScanTime := cas.lastScanTime.Load()
	if firstScanTime == 0 {
		t.Fatal("expected lastScanTime to be updated after scan")
	}

	// Second query for same non-existent ID should hit negative cache instantly
	start = time.Now()
	_, err = cas.GetHashForID("NONEXISTENT-1")
	if err == nil {
		t.Fatal("expected not found error")
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Fatalf("negative cache lookup took too long: %v", time.Since(start))
	}

	// Query for a different missing ID still errors; it may scan once (kind
	// cooldown must not poison a later Create of a real ID).
	_, err = cas.GetHashForID("NONEXISTENT-2")
	if err == nil {
		t.Fatal("expected not found error")
	}
	if !cas.isNegativeMiss("NONEXISTENT-2") {
		t.Fatal("expected NONEXISTENT-2 to be recorded as a negative miss after its own scan")
	}

	// Creating an object should clear negative miss for that ID
	_, h := writeMockCASYAML(t, kindDir, "NONEXISTENT-1")
	cas.SetIndexMappingInMemory("NONEXISTENT-1", h)

	gotHash, err := cas.GetHashForID("NONEXISTENT-1")
	if err != nil {
		t.Fatalf("expected object to be found after SetIndexMappingInMemory, got err: %v", err)
	}
	if gotHash != h {
		t.Fatalf("got hash %q want %q", gotHash, h)
	}
}

func TestGetHashForID_ConcurrentMisses_Singleflight(t *testing.T) {
	t.Parallel()
	kindDir := t.TempDir()
	cas := NewContentAddressableStorage(kindDir, "goal")

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		goroutinelabels.NewGoroutine("test.concurrent_cas_miss", "test goroutine for concurrent negative miss").StartSimple(func() {
			defer wg.Done()
			_, _ = cas.GetHashForID("CONCURRENT-MISSING")
		})
	}

	wg.Wait()

	// Should be recorded in negative miss cache
	if !cas.isNegativeMiss("CONCURRENT-MISSING") {
		t.Fatal("expected CONCURRENT-MISSING to be in negative miss cache")
	}
}

func TestDiscoverCASFilePathByScanning_FastRejection(t *testing.T) {
	t.Parallel()
	_, _, err := DiscoverCASFilePathByScanning("", "/some/dir")
	if err == nil {
		t.Fatal("expected error for empty ID, got nil")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected error mentioning empty, got: %v", err)
	}
}

func TestGetHashForID_FindsBucketedFileAfterKindScan(t *testing.T) {
	t.Parallel()
	kindDir := t.TempDir()
	cas := NewContentAddressableStorage(kindDir, "scheduler_job")

	_, existingHash := writeMockCASYAML(t, kindDir, "SCH-EXISTING")
	got, err := cas.GetHashForID("SCH-EXISTING")
	if err != nil {
		t.Fatalf("seed GetHashForID: %v", err)
	}
	if got != existingHash {
		t.Fatalf("seed hash %q want %q", got, existingHash)
	}
	if cas.index.Len() == 0 {
		t.Fatal("expected populated index after seed lookup")
	}

	_, missErr := cas.GetHashForID("SCH-MISSING")
	if missErr == nil {
		t.Fatal("expected miss for SCH-MISSING")
	}
	if cas.lastScanTime.Load() == 0 {
		t.Fatal("expected lastScanTime after miss scan")
	}

	id := "SCH-1789281781270652000"
	_, wantHash := writeMockCASYAMLAt(t, filepath.Join(kindDir, "2026-09"), id)
	got, err = cas.GetHashForID(id)
	if err != nil {
		t.Fatalf("GetHashForID after bucketed create: %v", err)
	}
	if got != wantHash {
		t.Fatalf("got hash %q want %q", got, wantHash)
	}
}

func TestCasHashYAMLExists_FindsDateBucketedHash(t *testing.T) {
	t.Parallel()
	kindDir := t.TempDir()
	id := "QAS-BUCKETED"
	_, hash := writeMockCASYAMLAt(t, filepath.Join(kindDir, "2026-09"), id)
	if !CasHashYAMLExists(kindDir, hash) {
		t.Fatalf("expected CasHashYAMLExists to find %s.yaml under 2026-09/", hash)
	}
	if CasHashYAMLExists(kindDir, strings.Repeat("ab", 32)) {
		t.Fatal("expected missing hash to be absent")
	}
}

// TestStreamSegmentCASMembraneFederation verifies CRIT-1789285385225174000-60220498
// for BLI-1789285463641928000-cca4794d:
// After a kernel create persists a CAS YAML blob under a date bucket, object get by id
// succeeds without a kind-root Stat miss or cooldown false-negative. Last-resort discover
// on create miss locates the blob.
func TestStreamSegmentCASMembraneFederation(t *testing.T) {
	t.Parallel()
	kindDir := t.TempDir()
	cas := NewContentAddressableStorage(kindDir, "backlog_item")

	bliID := "BLI-1789285463641928000-cca4794d"
	bucketDir := filepath.Join(kindDir, "2026-09")
	_, wantHash := writeMockCASYAMLAt(t, bucketDir, bliID)

	gotHash, err := cas.GetHashForID(bliID)
	if err != nil {
		t.Fatalf("GetHashForID(%q) failed: %v", bliID, err)
	}
	if gotHash != wantHash {
		t.Fatalf("got hash %q want %q for %s", gotHash, wantHash, bliID)
	}
}
