package objects

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
)

// TestSpecIndexStorageProfileMatchesHighVolumeStreamKinds ensures the spec plane (object_specs
// storage_profile → spec_index.json) agrees with high_volume_kinds.yaml for stream-backed kinds.
// This locks the data-cell / spec-stream pattern: stream kinds must declare storage_profile: stream
// so they do not incorrectly inherit cas_entity from auditable.
func TestSpecIndexStorageProfileMatchesHighVolumeStreamKinds(t *testing.T) {
	t.Parallel()
	root := testModuleRoot(t)
	specsDir := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
	idx, err := BuildSpecIndexFromSpecsDir(specsDir)
	if err != nil {
		t.Fatalf("BuildSpecIndexFromSpecsDir: %v", err)
	}
	if err := ValidateHighVolumeStreamKindsMatchSpecIndex(idx, root); err != nil {
		t.Fatal(err)
	}
	// Note: we do not require the inverse (every spec_index stream kind appears in high_volume_kinds).
	// Test-only kinds and metric variants may inherit storage_profile: stream from base_metric without
	// being runtime stream-storage kinds.
}
