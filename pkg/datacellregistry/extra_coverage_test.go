package datacellregistry

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestDatacellRegistry_EmptyProjectRootErrors(t *testing.T) {
	t.Parallel()

	if _, err := DescriptorReadModelForProject("", 0); err == nil {
		t.Errorf("expected error on empty project root")
	}
	if _, err := DescriptorReadModelForProject("", 1); err == nil {
		t.Errorf("expected error on empty project root")
	}
	if _, err := LoadDataCellDescriptors(""); err == nil {
		t.Errorf("expected error on empty project root")
	}
	if _, err := LoadDescriptorReadModel(""); err == nil {
		t.Errorf("expected error on empty project root")
	}
	if _, err := HighVolumeStreamSpecProfileMismatches("", nil); err == nil {
		t.Errorf("expected error on empty project root")
	}
	if _, err := StreamKindNamesFromHighVolumeConfig(""); err == nil {
		t.Errorf("expected error on empty project root")
	}
	if _, err := HighVolumeStreamStewardshipDrift(""); err == nil {
		t.Errorf("expected error on empty project root")
	}

	InvalidateDescriptorReadModelCache("")
	if key := normalizeProjectRootKey(""); key != "" {
		t.Errorf("expected empty string for normalizeProjectRootKey(\"\"), got %q", key)
	}
	if c := descriptorCacheForProject(""); c == nil || c.root != "" {
		t.Errorf("expected empty root cache")
	}
}

func TestStreamKindNamesFromHighVolumeConfig_ParseErrorAndMissing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	// Missing config returns nil, nil
	kinds, err := StreamKindNamesFromHighVolumeConfig(root)
	if err != nil || kinds != nil {
		t.Errorf("expected nil, nil for missing config, got %v, %v", kinds, err)
	}

	drift, err := HighVolumeStreamStewardshipDrift(root)
	if err != nil || drift != nil {
		t.Errorf("expected nil, nil for missing config in HighVolumeStreamStewardshipDrift, got %v, %v", drift, err)
	}

	// Corrupt YAML config
	configDir := filepath.Join(root, paths.ProcessInternalConfigsDir)
	if err := fileutil.MkdirAll(configDir, paths.DirPerm750); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	cfgFile := filepath.Join(configDir, paths.HighVolumeKindsConfigFile)
	if err := fileutil.WriteFile(cfgFile, []byte("invalid: yaml: ["), paths.FilePerm600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if _, err := StreamKindNamesFromHighVolumeConfig(root); err == nil {
		t.Errorf("expected parse error on corrupt YAML")
	}
	if _, err := HighVolumeStreamStewardshipDrift(root); err == nil {
		t.Errorf("expected error from HighVolumeStreamStewardshipDrift on corrupt YAML")
	}
}

func TestHighVolumeStreamSpecProfileMismatchesFromIndex_EdgeCases(t *testing.T) {
	t.Parallel()
	if mismatches := HighVolumeStreamSpecProfileMismatchesFromIndex(nil, []string{"audit_event"}); mismatches != nil {
		t.Errorf("expected nil when idx is nil")
	}
	if mismatches := HighVolumeStreamSpecProfileMismatchesFromIndex(nil, nil); mismatches != nil {
		t.Errorf("expected nil when streamEnabledKinds is nil")
	}
}
