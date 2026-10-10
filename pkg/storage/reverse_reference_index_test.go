package storage

import (
	stdcontext "context"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

// TestReverseReferenceIndex_AddReference_GetDependents tests AddReference and GetDependents.
func TestReverseReferenceIndex_AddReference_GetDependents(t *testing.T) {
	index := NewReverseReferenceIndex()

	index.AddReference("BLI-003", "BLI-002") // BLI-003 references BLI-002
	index.AddReference("BLI-004", "BLI-002") // BLI-004 references BLI-002

	deps := index.GetDependents("BLI-002")
	if len(deps) != 2 {
		t.Errorf("GetDependents(BLI-002) len = %d, want 2", len(deps))
	}
	sort.Strings(deps)
	if deps[0] != "BLI-003" || deps[1] != "BLI-004" {
		t.Errorf("GetDependents(BLI-002) = %v", deps)
	}

	depsEmpty := index.GetDependents("NONE")
	if len(depsEmpty) != 0 {
		t.Errorf("GetDependents(NONE) = %v, want []", depsEmpty)
	}
}

// TestReverseReferenceIndex_RemoveReference tests RemoveReference.
func TestReverseReferenceIndex_RemoveReference(t *testing.T) {
	index := NewReverseReferenceIndex()

	index.AddReference("BLI-003", "BLI-002")
	index.AddReference("BLI-004", "BLI-002")
	index.RemoveReference("BLI-003", "BLI-002")

	deps := index.GetDependents("BLI-002")
	if len(deps) != 1 || deps[0] != "BLI-004" {
		t.Errorf("After RemoveReference(BLI-003, BLI-002), GetDependents(BLI-002) = %v", deps)
	}
}

// TestReverseReferenceIndex_RemoveObject tests RemoveObject (removes object from all dependent lists).
func TestReverseReferenceIndex_RemoveObject(t *testing.T) {
	index := NewReverseReferenceIndex()

	index.AddReference("BLI-003", "BLI-002")
	index.AddReference("BLI-004", "BLI-002")
	index.RemoveObject("BLI-003") // BLI-003 is a dependent; removing it from the index

	deps := index.GetDependents("BLI-002")
	if len(deps) != 1 || deps[0] != "BLI-004" {
		t.Errorf("After RemoveObject(BLI-003), GetDependents(BLI-002) = %v", deps)
	}
}

// TestReverseReferenceIndex_UpdateReferences tests UpdateReferences (old refs -> new refs).
func TestReverseReferenceIndex_UpdateReferences(t *testing.T) {
	index := NewReverseReferenceIndex()

	index.AddReference("BLI-003", "BLI-002")
	index.UpdateReferences("BLI-003", []string{"BLI-002"}, []string{"BLI-001"}) // now BLI-003 references BLI-001

	depsOld := index.GetDependents("BLI-002")
	if len(depsOld) != 0 {
		t.Errorf("GetDependents(BLI-002) after update = %v, want []", depsOld)
	}
	depsNew := index.GetDependents("BLI-001")
	if len(depsNew) != 1 || depsNew[0] != "BLI-003" {
		t.Errorf("GetDependents(BLI-001) = %v", depsNew)
	}
}

// TestReverseReferenceIndex_Clear tests Clear.
func TestReverseReferenceIndex_Clear(t *testing.T) {
	index := NewReverseReferenceIndex()

	index.AddReference("BLI-003", "BLI-002")
	index.Clear()

	deps := index.GetDependents("BLI-002")
	if len(deps) != 0 {
		t.Errorf("After Clear(), GetDependents(BLI-002) = %v", deps)
	}
}

// TestReverseReferenceIndex_LoadSaveRoundTrip tests SaveCache and LoadCache round-trip.
func TestReverseReferenceIndex_LoadSaveRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	index := NewReverseReferenceIndex()
	index.AddReference("BLI-003", "BLI-002")
	index.AddReference("BLI-004", "BLI-002")

	if err := index.SaveCache(tmpDir); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}

	index2 := NewReverseReferenceIndex()
	loaded, err := index2.LoadCache(tmpDir)
	if err != nil {
		t.Fatalf("LoadCache: %v", err)
	}
	if !loaded {
		t.Fatal("LoadCache returned false")
	}

	deps := index2.GetDependents("BLI-002")
	if len(deps) != 2 {
		t.Errorf("After LoadCache, GetDependents(BLI-002) len = %d, want 2", len(deps))
	}
	sort.Strings(deps)
	if len(deps) == 2 && (deps[0] != "BLI-003" || deps[1] != "BLI-004") {
		t.Errorf("GetDependents(BLI-002) = %v", deps)
	}
}

// TestReverseReferenceIndex_LoadCache_ProjectRootEquivalence tests LoadCache with equivalent relative/clean paths.
func TestReverseReferenceIndex_LoadCache_ProjectRootEquivalence(t *testing.T) {
	tmpDir := t.TempDir()
	index := NewReverseReferenceIndex()
	index.AddReference("BLI-101", "PRI-202")

	if err := index.SaveCache(tmpDir); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}

	// Load with equivalent path (trailing slash and dot component)
	equivPath := filepath.Join(tmpDir, "sub", "..") + string(filepath.Separator)
	index2 := NewReverseReferenceIndex()
	loaded, err := index2.LoadCache(equivPath)
	if err != nil {
		t.Fatalf("LoadCache: %v", err)
	}
	if !loaded {
		t.Fatal("LoadCache returned false for equivalent project root path")
	}

	deps := index2.GetDependents("PRI-202")
	if len(deps) != 1 || deps[0] != "BLI-101" {
		t.Errorf("expected [BLI-101], got %v", deps)
	}
}

// TestReverseReferenceIndex_BuildFromScan tests BuildFromScan with YAML files containing _ref fields.
func TestReverseReferenceIndex_BuildFromScan(t *testing.T) {
	tmpDir := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	backlogDir := filepath.Join(processDir, objects.GetDirectoryFromKind(objects.KindBacklogItem))
	if err := fileutil.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Write parent and child YAML (child has parent_ref)
	parent := map[string]any{
		objects.FieldKeyID:            "BLI-002",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Parent",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	parentData, err := yaml.Marshal(parent)
	if err != nil {
		t.Fatalf("Marshal parent: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(backlogDir, "BLI-002.yaml"), parentData, paths.FilePerm644); err != nil {
		t.Fatalf("Write parent: %v", err)
	}

	child := map[string]any{
		objects.FieldKeyID:            "BLI-003",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Child",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		"test_parent_ref":             "BLI-002",
	}
	childData, err := yaml.Marshal(child)
	if err != nil {
		t.Fatalf("Marshal child: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(backlogDir, "BLI-003.yaml"), childData, paths.FilePerm644); err != nil {
		t.Fatalf("Write child: %v", err)
	}

	// BuildFromScan uses global index; clear first and rebuild
	global := GetGlobalReverseReferenceIndex()
	global.Clear()
	defer global.Clear()

	if err := global.BuildFromScan(tmpDir, processDir, []string{"backlog_item"}); err != nil {
		t.Fatalf("BuildFromScan: %v", err)
	}

	deps := global.GetDependents("BLI-002")
	if len(deps) == 0 {
		t.Error("GetDependents(BLI-002) after BuildFromScan: expected at least one dependent (BLI-003)")
	}
	found := false
	for _, id := range deps {
		if id == "BLI-003" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("GetDependents(BLI-002) = %v, expected to contain BLI-003", deps)
	}
}

// TestExtractReferenceIDsFromObject tests extraction of reference IDs from objects.
func TestExtractReferenceIDsFromObject(t *testing.T) {

	tests := []struct {
		name string
		obj  map[string]any
		want []string
	}{
		{"nil", nil, nil},
		{"empty", map[string]any{}, nil},
		{"single_ref", map[string]any{objects.FieldKeyID: "BLI-001", "parent_ref": "BLI-002"}, []string{"BLI-002"}},
		{"kind_id_format", map[string]any{objects.FieldKeyID: "BLI-001", objects.FieldKeyWorkstreamRef: "backlog_item:BLI-002"}, []string{"BLI-002"}},
		{"skips_commit_refs", map[string]any{objects.FieldKeyID: "X", "commit_refs": []any{"abc123"}}, nil},
		{"skips_commit_hashes", map[string]any{objects.FieldKeyID: "X", objects.FieldKeyCommitHashes: []any{"abc123"}}, nil},
		{"slice_refs", map[string]any{objects.FieldKeyID: "X", objects.FieldKeyMilestoneRefs: []any{"MIL-001", "MIL-002"}}, []string{"MIL-001", "MIL-002"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractReferenceIDsFromObject(tt.obj)
			sort.Strings(got)
			sort.Strings(tt.want)
			if len(got) != len(tt.want) {
				t.Errorf("extractReferenceIDsFromObject() len = %d, want %d; got %v", len(got), len(tt.want), got)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("extractReferenceIDsFromObject() = %v, want %v", got, tt.want)
					return
				}
			}
		})
	}
}

// TestExtractObjectIDFromReference tests parsing of reference string formats.
func TestExtractObjectIDFromReference(t *testing.T) {

	tests := []struct {
		ref  string
		want string
	}{
		{"", ""},
		{"BLI-001", "BLI-001"},
		{"backlog_item:BLI-002", "BLI-002"},
		// account:alice may be parsed by ParseNamespace first (short format) and return "alice"
		{"account:alice", "alice"},
		{"domain:process:backlog_item:BLI-003", "BLI-003"},
	}
	for _, tt := range tests {
		got := extractObjectIDFromReference(tt.ref)
		if got != tt.want {
			t.Errorf("extractObjectIDFromReference(%q) = %q, want %q", tt.ref, got, tt.want)
		}
	}
}

// TestReverseReferenceIndex_LoadCache_InvalidOrMissing verifies LoadCache returns false for missing/invalid file.
func TestReverseReferenceIndex_LoadCache_InvalidOrMissing(t *testing.T) {
	tmpDir := t.TempDir()
	index := NewReverseReferenceIndex()

	loaded, err := index.LoadCache(tmpDir)
	if err != nil {
		t.Fatalf("LoadCache (missing): %v", err)
	}
	if loaded {
		t.Error("LoadCache(missing dir) returned true, want false")
	}

	// Invalid/corrupt file
	cacheDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.CacheDir)
	if err := fileutil.MkdirAll(cacheDir, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(cacheDir, "reverse-reference-index.json"), []byte("not json"), paths.FilePerm644); err != nil {
		t.Fatalf("Write corrupt: %v", err)
	}
	loaded2, err2 := index.LoadCache(tmpDir)
	if err2 != nil {
		t.Fatalf("LoadCache (corrupt): %v", err2)
	}
	if loaded2 {
		t.Error("LoadCache(corrupt) returned true, want false")
	}
}

// TestReverseReferenceIndex_SaveCache_CreatesDir verifies SaveCache creates cache directory.
func TestReverseReferenceIndex_SaveCache_CreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	index := NewReverseReferenceIndex()
	index.AddReference("A", "B")

	if err := index.SaveCache(tmpDir); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}
	cachePath := filepath.Join(tmpDir, paths.ProjectDataDir, paths.CacheDir, "reverse-reference-index.json")
	if _, err := fileutil.Stat(cachePath); err != nil {
		t.Errorf("Cache file not created: %v", err)
	}
}

// TestReverseReferenceIndex_BuildFromScanThenSaveCache verifies that after BuildFromScan,
// SaveCache(projectRoot) creates the cache file at projectRoot/.zqk/cache/ (used by cache build path).
func TestReverseReferenceIndex_BuildFromScanThenSaveCache(t *testing.T) {
	// Uses global index; do not run in parallel to avoid races.
	tmpDir := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	criteriaDir := filepath.Join(processDir, "criteria")
	if err := fileutil.MkdirAll(criteriaDir, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := "id: CRIT-SAVE-001\nkind: criteria\nschema_version: \"" + objects.DefaultSchemaVersion + "\"\ntitle: C1\n"
	if err := fileutil.WriteFile(filepath.Join(criteriaDir, "CRIT-SAVE-001.yaml"), []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	global := GetGlobalReverseReferenceIndex()
	global.Clear()
	defer global.Clear()

	if err := global.BuildFromScan(tmpDir, processDir, []string{"criteria"}); err != nil {
		t.Fatalf("BuildFromScan: %v", err)
	}
	if err := global.SaveCache(tmpDir); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}

	cachePath := filepath.Join(tmpDir, paths.ProjectDataDir, paths.CacheDir, "reverse-reference-index.json")
	if _, err := fileutil.Stat(cachePath); err != nil {
		t.Errorf("Cache file not created after BuildFromScan+SaveCache: %v", err)
	}
}

// TestReverseReferenceIndex_GetDependentsWithError_Contention tests that GetDependentsWithContext
// fails closed with a non-nil error when the index lock cannot be acquired within the timeout.
func TestReverseReferenceIndex_GetDependentsWithError_Contention(t *testing.T) {
	index := NewReverseReferenceIndex()
	index.AddReference("BLI-CHILD", "BLI-PARENT")

	// 1. Success case: dependents returned cleanly
	deps, err := index.GetDependentsWithError("BLI-PARENT")
	if err != nil {
		t.Fatalf("unexpected error getting dependents: %v", err)
	}
	if len(deps) != 1 || deps[0] != "BLI-CHILD" {
		t.Fatalf("expected [BLI-CHILD], got %v", deps)
	}

	// 2. Timeout / contention case: holding exclusive lock causes fail-closed error
	index.mu.Lock()
	defer index.mu.Unlock()

	ctx, cancel := stdcontext.WithTimeout(stdcontext.Background(), 20*time.Millisecond)
	defer cancel()

	blockedDeps, lockErr := index.GetDependentsWithContext(ctx, "BLI-PARENT")
	if lockErr == nil {
		t.Fatalf("expected lock acquisition timeout error, got nil error with deps %v", blockedDeps)
	}
	if blockedDeps != nil {
		t.Fatalf("expected nil dependents on lock failure, got %v", blockedDeps)
	}
}

// TestReverseReferenceIndex_ProjectScopedIsolation verifies that project-scoped indices
// maintain independent state and clearing one does not affect another (TDE-CEF-F-ARCH-003).
func TestReverseReferenceIndex_ProjectScopedIsolation(t *testing.T) {
	projA := "/tmp/project-alpha-" + t.Name()
	projB := "/tmp/project-beta-" + t.Name()

	idxA := GetReverseReferenceIndexForProject(projA)
	idxB := GetReverseReferenceIndexForProject(projB)

	if idxA == idxB {
		t.Fatalf("expected distinct index instances for different projects, got identical pointer %p", idxA)
	}
	if idxA.ProjectRoot() != projA {
		t.Errorf("idxA.ProjectRoot() = %q, want %q", idxA.ProjectRoot(), projA)
	}
	if idxB.ProjectRoot() != projB {
		t.Errorf("idxB.ProjectRoot() = %q, want %q", idxB.ProjectRoot(), projB)
	}

	// Add references to project A
	idxA.AddReference("BLI-ALPHA-1", "REQ-ALPHA-1")
	// Add references to project B
	idxB.AddReference("BLI-BETA-1", "REQ-BETA-1")

	// Verify isolation
	depsA := idxA.GetDependents("REQ-ALPHA-1")
	if len(depsA) != 1 || depsA[0] != "BLI-ALPHA-1" {
		t.Errorf("idxA.GetDependents(REQ-ALPHA-1) = %v, want [BLI-ALPHA-1]", depsA)
	}
	if len(idxA.GetDependents("REQ-BETA-1")) != 0 {
		t.Errorf("idxA unexpectedly contains REQ-BETA-1 dependents")
	}

	depsB := idxB.GetDependents("REQ-BETA-1")
	if len(depsB) != 1 || depsB[0] != "BLI-BETA-1" {
		t.Errorf("idxB.GetDependents(REQ-BETA-1) = %v, want [BLI-BETA-1]", depsB)
	}
	if len(idxB.GetDependents("REQ-ALPHA-1")) != 0 {
		t.Errorf("idxB unexpectedly contains REQ-ALPHA-1 dependents")
	}

	// Clear project A and verify project B is completely unaffected
	idxA.Clear()
	if len(idxA.GetDependents("REQ-ALPHA-1")) != 0 {
		t.Errorf("idxA should be empty after Clear()")
	}

	depsBAfter := idxB.GetDependents("REQ-BETA-1")
	if len(depsBAfter) != 1 || depsBAfter[0] != "BLI-BETA-1" {
		t.Fatalf("destructive cross-project contamination: idxB lost entries after idxA.Clear(): %v", depsBAfter)
	}
}

// TestReverseReferenceIndex_MultiTenantConcurrentAccess verifies thread-safe concurrent
// operations on multiple project-scoped indices without deadlocks or state pollution.
func TestReverseReferenceIndex_MultiTenantConcurrentAccess(t *testing.T) {
	ctx, cancel := stdcontext.WithCancel(stdcontext.Background())
	defer cancel()
	_ = ctx

	const numProjects = 5
	const numOps = 20

	done := make(chan struct{})
	for p := 0; p < numProjects; p++ {
		projPath := filepath.Join("/tmp", "concurrent-proj", string(rune('a'+p)))
		pID := p
		pathCopy := projPath
		goroutinelabels.NewGoroutine("reverse_reference_index_test", "multitenant concurrent worker").StartSimple(func() {
			defer func() { done <- struct{}{} }()
			idx := GetReverseReferenceIndexForProject(pathCopy)
			for i := 0; i < numOps; i++ {
				child := "BLI-" + string(rune('A'+pID))
				parent := "REQ-" + string(rune('A'+pID))
				idx.AddReference(child, parent)
				deps := idx.GetDependents(parent)
				if len(deps) == 0 {
					t.Errorf("expected non-empty dependents in project %s", pathCopy)
				}
				if i%5 == 0 {
					idx.RemoveReference(child, parent)
				}
			}
		})
	}

	for p := 0; p < numProjects; p++ {
		<-done
	}
}

// TestReverseReferenceIndex_SaveCache_LockContention_PropagatesError tests that SaveCache
// fails closed with a non-nil error when the index lock cannot be acquired within the timeout.
func TestReverseReferenceIndex_SaveCache_LockContention_PropagatesError(t *testing.T) {
	projDir := t.TempDir()
	index := NewReverseReferenceIndex()
	index.AddReference("BLI-CHILD", "BLI-PARENT")

	// Hold exclusive write lock so SaveCache's RLock or Lock cannot be acquired
	index.mu.Lock()
	defer index.mu.Unlock()

	err := index.SaveCache(projDir)
	if err == nil {
		t.Fatal("expected error from SaveCache when lock is contended, got nil")
	}
	if !strings.Contains(err.Error(), "lock timeout") {
		t.Errorf("expected error to mention 'lock timeout', got: %v", err)
	}
}
