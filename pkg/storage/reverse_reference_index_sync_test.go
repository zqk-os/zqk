package storage

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func resetReverseReferenceIndexSyncForTest(t *testing.T) {
	t.Helper()
	if old := revRefPersistTimer.Swap(nil); old != nil {
		_ = old.Stop()
	}
	revRefBoundRoot.Store(emptyValue)
	GetGlobalReverseReferenceIndex().Clear()
}

// TestReverseReferenceIndex_CUDPersistsAcrossProcessRestart proves the bugfix:
// CUD must SaveCache so a fresh process LoadCache sees dependents (membership / promote).
// TRACK: REQ-CEF-R2-REL-REVINDEX-FAILOPEN
func TestReverseReferenceIndex_CriteriaRefsUpdateAddsDependent(t *testing.T) {
	resetReverseReferenceIndexSyncForTest(t)
	t.Cleanup(func() { resetReverseReferenceIndexSyncForTest(t) })
	BindReverseReferenceIndexProjectRoot(t.TempDir())

	bli := "BLI-rev-001"
	crit := "CRIT-rev-001"
	oldObj := map[string]any{
		objects.FieldKeyID:              bli,
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyPriorityPlanRef: "PRI-rev-001",
	}
	updateReverseReferenceIndexOnCreate(bli, oldObj)
	newObj := map[string]any{
		objects.FieldKeyID:              bli,
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyPriorityPlanRef: "PRI-rev-001",
		objects.FieldKeyCriteriaRefs:    []string{crit},
	}
	updateReverseReferenceIndexOnUpdate(bli, oldObj, newObj)
	deps := GetGlobalReverseReferenceIndex().GetDependents(crit)
	found := false
	for _, id := range deps {
		if id == bli {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("dependents of %s = %v, want %s", crit, deps, bli)
	}
}

func TestReverseReferenceIndex_CUDPersistsAcrossProcessRestart(t *testing.T) {
	resetReverseReferenceIndexSyncForTest(t)
	t.Cleanup(func() { resetReverseReferenceIndexSyncForTest(t) })

	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.CacheDir)
	if err := fileutil.EnsureDir(cacheDir); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}

	BindReverseReferenceIndexProjectRoot(tmpDir)

	planID := "PRI-test-plan-001"
	childID := "BLI-test-child-001"
	updateReverseReferenceIndexOnCreate(childID, map[string]any{
		objects.FieldKeyID:              childID,
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyPriorityPlanRef: planID,
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
	})
	FlushReverseReferenceIndexPersist()

	cachePath := GetGlobalReverseReferenceIndex().GetCacheFilePath(tmpDir)
	if _, err := fileutil.Stat(cachePath); err != nil {
		t.Fatalf("expected reverse-ref cache on disk after CUD persist: %v", err)
	}

	// Simulate a new CLI process: memory cleared, then load from disk.
	GetGlobalReverseReferenceIndex().Clear()
	revRefBoundRoot.Store(emptyValue)
	BindReverseReferenceIndexProjectRoot(tmpDir)

	deps := GetGlobalReverseReferenceIndex().GetDependents(planID)
	found := false
	for _, id := range deps {
		if id == childID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("GetDependents(%s) after reload = %v, want %s (stale disk = membership miss)", planID, deps, childID)
	}
}

func reverseReferenceIndexUnreadableCacheRoot(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.CacheDir)
	if err := fileutil.EnsureDir(cacheDir); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	// ReadFile on a directory yields a load error (not missing → false,nil).
	cachePath := filepath.Join(cacheDir, reverseReferenceIndexFile)
	if err := fileutil.EnsureDir(cachePath); err != nil {
		t.Fatalf("mkdir cache path as dir: %v", err)
	}
	return tmpDir
}

func TestEnsureReverseReferenceIndexLoaded_LoadErrorLeavesNotReady(t *testing.T) {
	resetReverseReferenceIndexSyncForTest(t)
	t.Cleanup(func() { resetReverseReferenceIndexSyncForTest(t) })

	tmpDir := reverseReferenceIndexUnreadableCacheRoot(t)
	BindReverseReferenceIndexProjectRoot(tmpDir)
	if GetGlobalReverseReferenceIndex().IsReady() {
		t.Fatal("LoadCache I/O error must leave reverse-ref index not ready (fail-closed)")
	}
}

func TestFindDependents_LoadCacheErrorFailsClosed(t *testing.T) {
	resetReverseReferenceIndexSyncForTest(t)
	t.Cleanup(func() { resetReverseReferenceIndexSyncForTest(t) })

	tmpDir := reverseReferenceIndexUnreadableCacheRoot(t)
	f := &FileObjectStorage{projectRoot: tmpDir}
	deps, err := f.findDependents(t.Context(), "BLI-revindex-failclosed", objects.KindBacklogItem)
	if err == nil {
		t.Fatalf("findDependents must fail closed on LoadCache I/O error, got deps=%v", deps)
	}
	if GetGlobalReverseReferenceIndex().IsReady() {
		t.Fatal("findDependents LoadCache error must not mark the index ready-empty")
	}
}

// TestEnsureReverseReferenceIndexLoaded_MissingCacheLeavesNotReady verifies that when no cache
// file exists on disk, the index remains not ready rather than failing open as ready-empty (BLI-CEF-R16-REVINDEX-FAILOPEN-001).
func TestEnsureReverseReferenceIndexLoaded_MissingCacheLeavesNotReady(t *testing.T) {
	resetReverseReferenceIndexSyncForTest(t)
	t.Cleanup(func() { resetReverseReferenceIndexSyncForTest(t) })

	tmpDir := t.TempDir()
	BindReverseReferenceIndexProjectRoot(tmpDir)
	if GetGlobalReverseReferenceIndex().IsReady() {
		t.Fatal("Missing cache must leave reverse-ref index not ready (must not fail-open as ready-empty)")
	}
}
