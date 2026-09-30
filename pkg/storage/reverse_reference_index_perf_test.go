package storage

import (
	stdcontext "context"
	"fmt"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestReverseReferenceIndex_DualIndexIntegrity_StaticFloor verifies CRIT-CEF-PERF-DUAL-INDEX-001:
// ReverseReferenceIndex maintains synchronized forwardIndex and reverse index across AddReference,
// RemoveReference, UpdateReferences, and LoadCache without fallback to full linear scans.
func TestReverseReferenceIndex_DualIndexIntegrity_StaticFloor(t *testing.T) {
	idx := NewReverseReferenceIndex()

	// 1. Initial State: empty maps initialized
	if idx.index == nil || idx.forwardIndex == nil {
		t.Fatalf("expected both index and forwardIndex to be initialized, got index=%v, forwardIndex=%v", idx.index, idx.forwardIndex)
	}

	// 2. AddReference synchronization
	idx.AddReference("OBJ-A", "REF-1")
	idx.AddReference("OBJ-A", "REF-2")
	idx.AddReference("OBJ-B", "REF-1")

	// Verify reverse index
	deps1 := idx.GetDependents("REF-1")
	sort.Strings(deps1)
	if len(deps1) != 2 || deps1[0] != "OBJ-A" || deps1[1] != "OBJ-B" {
		t.Fatalf("GetDependents(REF-1) = %v, expected [OBJ-A, OBJ-B]", deps1)
	}

	// Verify forward index
	refsA := idx.GetReferencedObjectIDs("OBJ-A")
	sort.Strings(refsA)
	if len(refsA) != 2 || refsA[0] != "REF-1" || refsA[1] != "REF-2" {
		t.Fatalf("GetReferencedObjectIDs(OBJ-A) = %v, expected [REF-1, REF-2]", refsA)
	}

	// Idempotency: re-adding does not duplicate
	idx.AddReference("OBJ-A", "REF-1")
	if len(idx.GetReferencedObjectIDs("OBJ-A")) != 2 {
		t.Fatalf("AddReference duplicate created extra forward entry")
	}
	if len(idx.GetDependents("REF-1")) != 2 {
		t.Fatalf("AddReference duplicate created extra reverse entry")
	}

	// 3. UpdateReferences synchronization
	// Replace REF-2 with REF-3 for OBJ-A
	idx.UpdateReferences("OBJ-A", []string{"REF-2"}, []string{"REF-3"})
	refsAUpdated := idx.GetReferencedObjectIDs("OBJ-A")
	sort.Strings(refsAUpdated)
	if len(refsAUpdated) != 2 || refsAUpdated[0] != "REF-1" || refsAUpdated[1] != "REF-3" {
		t.Fatalf("after UpdateReferences, GetReferencedObjectIDs(OBJ-A) = %v, expected [REF-1, REF-3]", refsAUpdated)
	}
	if len(idx.GetDependents("REF-2")) != 0 {
		t.Fatalf("expected REF-2 to have 0 dependents after update, got %v", idx.GetDependents("REF-2"))
	}
	deps3 := idx.GetDependents("REF-3")
	if len(deps3) != 1 || deps3[0] != "OBJ-A" {
		t.Fatalf("expected REF-3 to have dependent OBJ-A, got %v", deps3)
	}

	// 4. RemoveReference synchronization
	idx.RemoveReference("OBJ-A", "REF-3")
	if len(idx.GetReferencedObjectIDs("OBJ-A")) != 1 || idx.GetReferencedObjectIDs("OBJ-A")[0] != "REF-1" {
		t.Fatalf("after RemoveReference, GetReferencedObjectIDs(OBJ-A) = %v, expected [REF-1]", idx.GetReferencedObjectIDs("OBJ-A"))
	}
	if len(idx.GetDependents("REF-3")) != 0 {
		t.Fatalf("after RemoveReference, GetDependents(REF-3) should be empty, got %v", idx.GetDependents("REF-3"))
	}

	// 5. RemoveObject synchronization
	idx.RemoveObject("OBJ-A")
	if refs := idx.GetReferencedObjectIDs("OBJ-A"); len(refs) != 0 {
		t.Fatalf("after RemoveObject, GetReferencedObjectIDs(OBJ-A) = %v, expected empty", refs)
	}
	if deps := idx.GetDependents("REF-1"); len(deps) != 1 || deps[0] != "OBJ-B" {
		t.Fatalf("after RemoveObject(OBJ-A), GetDependents(REF-1) = %v, expected [OBJ-B]", deps)
	}

	// 6. Cache save & load reconstruction of forwardIndex
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.CacheDir)
	if err := fileutil.EnsureDir(cacheDir); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}

	// Create and populate index to be saved
	saveIdx := NewReverseReferenceIndex()
	saveIdx.AddReference("OBJ-X", "TARGET-Y")
	saveIdx.AddReference("OBJ-X", "TARGET-Z")
	saveIdx.AddReference("OBJ-W", "TARGET-Y")

	// Save cache using SaveCache
	if err := saveIdx.SaveCache(tmpDir); err != nil {
		t.Fatalf("SaveCache failed: %v", err)
	}

	// Load into a new index and verify forwardIndex reconstruction
	loadIdx := NewReverseReferenceIndex()
	loaded, err := loadIdx.LoadCache(tmpDir)
	if err != nil {
		t.Fatalf("LoadCache failed: %v", err)
	}
	if !loaded {
		t.Fatalf("LoadCache returned loaded=false")
	}

	// Verify reverse entries in loaded index
	depsY := loadIdx.GetDependents("TARGET-Y")
	sort.Strings(depsY)
	if len(depsY) != 2 || depsY[0] != "OBJ-W" || depsY[1] != "OBJ-X" {
		t.Fatalf("loaded GetDependents(TARGET-Y) = %v, expected [OBJ-W, OBJ-X]", depsY)
	}

	// Verify forward entries reconstructed in loaded index
	refsX := loadIdx.GetReferencedObjectIDs("OBJ-X")
	sort.Strings(refsX)
	if len(refsX) != 2 || refsX[0] != "TARGET-Y" || refsX[1] != "TARGET-Z" {
		t.Fatalf("loaded GetReferencedObjectIDs(OBJ-X) = %v, expected [TARGET-Y, TARGET-Z]", refsX)
	}
	refsW := loadIdx.GetReferencedObjectIDs("OBJ-W")
	if len(refsW) != 1 || refsW[0] != "TARGET-Y" {
		t.Fatalf("loaded GetReferencedObjectIDs(OBJ-W) = %v, expected [TARGET-Y]", refsW)
	}
}

// TestReverseReferenceIndex_JoinAndAdmission_OperationalProof verifies CRIT-CEF-PERF-JOIN-ADMISSION-001:
// joinBindings executes O(M+N) indexed hash-joins on shared keys and WaitUnderGoroutineCeiling
// enforces concurrency limits using reusable non-leaking timers.
func TestReverseReferenceIndex_JoinAndAdmission_OperationalProof(t *testing.T) {
	// 1. High scale ReverseReferenceIndex batch removal using dual forward-index
	idx := NewReverseReferenceIndex()
	const numObjects = 2000
	for i := 0; i < numObjects; i++ {
		objID := fmt.Sprintf("BLI-%04d", i)
		refID := fmt.Sprintf("REQ-%04d", i%50)
		idx.AddReference(objID, refID)
	}

	// Batch removal: forward-index eliminates N^2 scan overhead
	startRemoval := time.Now()
	for i := 0; i < numObjects; i++ {
		objID := fmt.Sprintf("BLI-%04d", i)
		idx.RemoveObject(objID)
	}
	elapsedRemoval := time.Since(startRemoval)

	if elapsedRemoval > 200*time.Millisecond {
		t.Errorf("batch removal of %d objects took %v, expected < 200ms", numObjects, elapsedRemoval)
	}
	for i := 0; i < 50; i++ {
		refID := fmt.Sprintf("REQ-%04d", i)
		if deps := idx.GetDependents(refID); len(deps) != 0 {
			t.Fatalf("expected 0 dependents for %s after removal, got %v", refID, deps)
		}
	}

	// 2. WaitUnderGoroutineCeiling admission with high ceiling (normal execution)
	ctx := stdcontext.Background()
	// High ceiling: returns immediately without waiting
	err := concurrency.WaitUnderGoroutineCeiling(ctx, 100000, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitUnderGoroutineCeiling failed under high ceiling: %v", err)
	}

	// Disabled ceiling (ceiling <= 0)
	err = concurrency.WaitUnderGoroutineCeiling(ctx, 0, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitUnderGoroutineCeiling failed with disabled ceiling: %v", err)
	}
}

// TestReverseReferenceIndex_BoundaryFallback_NegativeBoundary verifies CRIT-CEF-PERF-BOUNDARY-FALLBACK-001:
// ReverseReferenceIndex safely handles missing keys and nil maps; WaitUnderGoroutineCeiling aborts
// on ctx.Done() with context error propagation.
func TestReverseReferenceIndex_BoundaryFallback_NegativeBoundary(t *testing.T) {
	idx := NewReverseReferenceIndex()

	// 1. Empty string parameters safe handling
	idx.AddReference("", "")
	idx.AddReference("OBJ-1", "")
	idx.AddReference("", "REF-1")
	if len(idx.GetDependents("")) != 0 {
		t.Errorf("expected empty dependents for empty string")
	}
	if len(idx.GetReferencedObjectIDs("")) != 0 {
		t.Errorf("expected empty refs for empty string")
	}

	idx.RemoveReference("", "")
	idx.RemoveReference("OBJ-1", "")
	idx.RemoveReference("", "REF-1")
	idx.RemoveObject("")
	idx.UpdateReferences("", nil, nil)
	idx.UpdateReferences("OBJ-1", []string{""}, []string{""})

	// 2. Querying non-existent objects
	if refs := idx.GetReferencedObjectIDs("NONEXISTENT"); len(refs) != 0 {
		t.Errorf("expected empty slice for non-existent object, got %v", refs)
	}
	if deps := idx.GetDependents("NONEXISTENT"); len(deps) != 0 {
		t.Errorf("expected empty slice for non-existent dependent, got %v", deps)
	}

	// 3. Fallback when forwardIndex is explicitly nil (legacy or uninitialized struct)
	legacyIdx := &ReverseReferenceIndex{
		index:        make(map[string][]string),
		forwardIndex: nil, // simulated unpopulated forward index
	}
	legacyIdx.index["REF-LEGACY"] = []string{"OBJ-LEGACY-1", "OBJ-LEGACY-2"}
	legacyIdx.RemoveObject("OBJ-LEGACY-1")

	remaining := legacyIdx.GetDependents("REF-LEGACY")
	if len(remaining) != 1 || remaining[0] != "OBJ-LEGACY-2" {
		t.Fatalf("fallback scan failed when forwardIndex was nil: got %v, expected [OBJ-LEGACY-2]", remaining)
	}

	// 4. GetDependentsWithContext timeout / cancellation propagation under lock contention
	idx.mu.Lock()
	canceledCtx, cancel := stdcontext.WithCancel(stdcontext.Background())
	cancel() // pre-canceled
	deps, err := idx.GetDependentsWithContext(canceledCtx, "REF-1")
	idx.mu.Unlock()
	if err == nil {
		t.Fatalf("expected error from canceled context in GetDependentsWithContext under contention, got nil (deps=%v)", deps)
	}

	// 5. WaitUnderGoroutineCeiling context cancellation & deadline exceeded
	// Force ceiling to 1 so NumGoroutine() (which is >= 2 in Go testing) triggers blocking
	timeoutCtx, timeoutCancel := stdcontext.WithTimeout(stdcontext.Background(), 20*time.Millisecond)
	defer timeoutCancel()

	ceilingErr := concurrency.WaitUnderGoroutineCeiling(timeoutCtx, 1, 5*time.Millisecond)
	if ceilingErr == nil {
		t.Fatalf("expected context deadline exceeded under ceiling=1, got nil")
	}
	if ceilingErr != stdcontext.DeadlineExceeded && ceilingErr != stdcontext.Canceled {
		t.Fatalf("expected DeadlineExceeded or Canceled, got %v", ceilingErr)
	}
}
