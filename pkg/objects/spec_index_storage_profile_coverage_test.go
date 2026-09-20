package objects

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"
)

// TestSpecIndex_storageProfileWireCoverage ensures materialized spec index kinds only use declared
// storage_profile wire values, and that cas_entity and stream each appear at least once (today's
// object_specs). light_file is valid but may have no dedicated kind yet — see DATA_CELL_MODEL.md.
func TestSpecIndex_storageProfileWireCoverage(t *testing.T) {
	t.Parallel()
	root := testModuleRoot(t)
	specsDir := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
	idx, err := BuildSpecIndexFromSpecsDir(specsDir)
	if err != nil {
		t.Fatalf("BuildSpecIndexFromSpecsDir: %v", err)
	}
	if idx == nil || idx.Kinds == nil {
		t.Fatal("nil index")
	}

	counts := make(map[string]int)
	for _, kind := range GetAllKindsFromIndex(idx) {
		ks, ok := idx.GetKindSummary(kind)
		if !ok {
			continue
		}
		w := ks.StorageProfile
		if w == "" {
			counts["(empty)"]++
			continue
		}
		if _, err := datacell.ParseStorageProfile(w); err != nil {
			t.Fatalf("kind %q: invalid storage_profile %q: %v", kind, w, err)
		}
		counts[w]++
	}

	if counts[string(datacell.ProfileCASEntity)] == 0 {
		t.Fatal("expected at least one kind with storage_profile cas_entity (auditable chain)")
	}
	if counts[string(datacell.ProfileStream)] == 0 {
		t.Fatal("expected at least one kind with storage_profile stream (high-volume kinds)")
	}
	if n := counts[string(datacell.ProfileLightFile)]; n == 0 {
		t.Log("note: no object_spec uses storage_profile light_file yet — profile reserved (runtime organism / low-ceremony kinds)")
	}
}
