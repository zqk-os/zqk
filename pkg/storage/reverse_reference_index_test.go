package storage

import (
	"github.com/lanceman/zqk/pkg/datacell"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"path/filepath"
	"sort"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/objects"
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

// TestReverseReferenceIndex_BuildFromScan tests BuildFromScan with YAML files containing _ref fields.
func TestReverseReferenceIndex_BuildFromScan(t *testing.T) {
	tmpDir := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	backlogDir := filepath.Join(processDir, "backlog")
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
	parentData, _ := yaml.Marshal(parent)
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
	childData, _ := yaml.Marshal(child)
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
		{"skips_commit_refs", map[string]any{objects.FieldKeyID: "X", objects.FieldKeyCommitRefs: []any{"abc123"}}, nil},
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
	cacheDir := filepath.Join(tmpDir, paths.ProjectDataDir, "cache")
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
	cachePath := filepath.Join(tmpDir, paths.ProjectDataDir, "cache", "reverse-reference-index.json")
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

	cachePath := filepath.Join(tmpDir, paths.ProjectDataDir, "cache", "reverse-reference-index.json")
	if _, err := fileutil.Stat(cachePath); err != nil {
		t.Errorf("Cache file not created after BuildFromScan+SaveCache: %v", err)
	}
}
