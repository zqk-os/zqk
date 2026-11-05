package vds

import (
	"path/filepath"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestLoadSpine_FallbackWhenMissing(t *testing.T) {
	tempDir := t.TempDir()

	// 1. LoadSpine with empty relPath when docs/quality/verifiable_decomposition_spine_profile.yaml is absent
	spine, err := LoadSpine(tempDir, "")
	if err != nil {
		t.Fatalf("expected LoadSpine to fall back to embedded default, got err: %v", err)
	}
	if spine == nil {
		t.Fatal("expected non-nil spine profile")
	}
	if spine.ProfileID != "verifiable_decomposition_spine_v1" {
		t.Fatalf("expected profile_id 'verifiable_decomposition_spine_v1', got %q", spine.ProfileID)
	}
	if len(spine.Stages) != 5 {
		t.Fatalf("expected 5 stages, got %d", len(spine.Stages))
	}
	if len(spine.Completion.HardGates) == 0 || spine.Completion.HardGates[0] != "independent_verify" {
		t.Fatalf("expected hard gate 'independent_verify', got %+v", spine.Completion.HardGates)
	}

	// 2. LoadSpine with explicit DefaultSpineProfileRel when file is absent
	spineRel, err := LoadSpine(tempDir, DefaultSpineProfileRel)
	if err != nil {
		t.Fatalf("expected LoadSpine with DefaultSpineProfileRel to fall back, got err: %v", err)
	}
	if spineRel == nil || spineRel.ProfileID != "verifiable_decomposition_spine_v1" {
		t.Fatalf("unexpected spine profile: %+v", spineRel)
	}

	// 3. ResolveProfiles when spine and customization are both absent from disk
	resSpine, resCust, err := ResolveProfiles(tempDir, "", "")
	if err != nil {
		t.Fatalf("expected ResolveProfiles to succeed with fallback spine and empty cust, got err: %v", err)
	}
	if resSpine == nil || resSpine.ProfileID != "verifiable_decomposition_spine_v1" {
		t.Fatalf("unexpected resSpine: %+v", resSpine)
	}
	if resCust == nil {
		t.Fatal("expected non-nil resCust")
	}
}

func TestEnsureSpineProfile_And_InitSpineProfileFile(t *testing.T) {
	tempDir := t.TempDir()

	// Initial call should create file
	path, created, err := EnsureSpineProfile(tempDir)
	if err != nil {
		t.Fatalf("EnsureSpineProfile failed: %v", err)
	}
	if !created {
		t.Fatal("expected created=true for new file")
	}
	expectedPath := filepath.Join(tempDir, DefaultSpineProfileRel)
	if path != expectedPath {
		t.Fatalf("expected path %s, got %s", expectedPath, path)
	}

	data, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read created spine profile: %v", err)
	}
	if string(data) != DefaultEmbeddedSpineProfileYAML {
		t.Fatal("file content does not match DefaultEmbeddedSpineProfileYAML")
	}

	// Second call without force should not overwrite
	path2, created2, err := EnsureSpineProfile(tempDir)
	if err != nil {
		t.Fatalf("second EnsureSpineProfile failed: %v", err)
	}
	if created2 {
		t.Fatal("expected created=false when file already exists")
	}
	if path2 != expectedPath {
		t.Fatalf("expected path %s, got %s", expectedPath, path2)
	}

	// Modify file content to test force overwrite
	customContent := "schema_version: 1\nprofile_id: modified\nstages: []\n"
	if err := fileutil.WriteFile(path, []byte(customContent), 0644); err != nil {
		t.Fatal(err)
	}

	// InitSpineProfileFile with force=false should not overwrite modified file
	_, createdNoForce, err := InitSpineProfileFile(tempDir, false)
	if err != nil {
		t.Fatal(err)
	}
	if createdNoForce {
		t.Fatal("expected created=false with force=false")
	}
	modData, _ := fileutil.ReadFile(path)
	if string(modData) != customContent {
		t.Fatal("file was unexpectedly overwritten with force=false")
	}

	// InitSpineProfileFile with force=true should overwrite
	_, createdForce, err := InitSpineProfileFile(tempDir, true)
	if err != nil {
		t.Fatal(err)
	}
	if !createdForce {
		t.Fatal("expected created=true with force=true")
	}
	restoredData, _ := fileutil.ReadFile(path)
	if string(restoredData) != DefaultEmbeddedSpineProfileYAML {
		t.Fatal("file was not restored to DefaultEmbeddedSpineProfileYAML with force=true")
	}
}
