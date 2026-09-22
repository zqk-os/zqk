package bootstrap

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestBootstrap_EdgeCasesAndErrors(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Test captureDirectory when directory does not exist
	nonExistentDir := filepath.Join(tmpDir, "non_existent")
	targetMap := make(map[string][]byte)
	if err := captureDirectory(nonExistentDir, targetMap, "test"); err != nil {
		t.Fatalf("expected nil error for non-existent directory, got %v", err)
	}

	// Test captureConfigFiles error on missing sourceDir
	if err := captureConfigFiles(nonExistentDir, targetMap); err == nil {
		t.Fatalf("expected error when reading non-existent source directory")
	}

	// Test captureConfigFiles ignoring non-YAML files and bootstrap.yaml
	sourceDir := filepath.Join(tmpDir, "source")
	if err := fileutil.MkdirAll(sourceDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to mkdir: %v", err)
	}
	_ = fileutil.WriteFile(filepath.Join(sourceDir, "bootstrap.yaml"), []byte("bootstrap"), paths.FilePerm644)
	_ = fileutil.WriteFile(filepath.Join(sourceDir, "ignored.txt"), []byte("txt"), paths.FilePerm644)
	_ = fileutil.WriteFile(filepath.Join(sourceDir, "valid.yaml"), []byte("k: v"), paths.FilePerm644)

	configMap := make(map[string][]byte)
	if err := captureConfigFiles(sourceDir, configMap); err != nil {
		t.Fatalf("unexpected error capturing configs: %v", err)
	}
	if len(configMap) != 1 || string(configMap["valid.yaml"]) != "k: v" {
		t.Errorf("expected only valid.yaml captured, got %v", configMap)
	}

	// Test SaveToFile & LoadFromFile with all file types (profiles, traits, lifecycles, configs)
	capture := &BootstrapCapture{
		Version: BootstrapCaptureFormatVersion,
		ObjectSpecs: map[string][]byte{
			"spec1.yaml": []byte("ontology: spec1"),
		},
		Lifecycles: map[string][]byte{
			"sub/lc1.yaml": []byte("lifecycle: lc1"),
		},
		Profiles: map[string][]byte{
			"prof1.yaml": []byte("profile: prof1"),
		},
		Configs: map[string][]byte{
			"conf1.yaml": []byte("config: conf1"),
		},
		Traits: map[string][]byte{
			"trait1.yaml": []byte("trait: trait1"),
		},
	}

	savePath := filepath.Join(tmpDir, "output", "bootstrap.yaml")
	if err := capture.SaveToFile(savePath); err != nil {
		t.Fatalf("failed to save to file: %v", err)
	}

	loaded, err := LoadFromFile(savePath)
	if err != nil {
		t.Fatalf("failed to load from file: %v", err)
	}
	if len(loaded.ObjectSpecs) != 1 || len(loaded.Lifecycles) != 1 || len(loaded.Profiles) != 1 || len(loaded.Configs) != 1 || len(loaded.Traits) != 1 {
		t.Errorf("unexpected loaded file counts: %+v", loaded)
	}

	// Test LoadFromFile errors
	if _, err := LoadFromFile(filepath.Join(tmpDir, "does_not_exist.yaml")); err == nil {
		t.Errorf("expected error for non-existent file")
	}
	corruptFile := filepath.Join(tmpDir, "corrupt.yaml")
	_ = fileutil.WriteFile(corruptFile, []byte("files:\n  bad: 'not-valid-base64---'"), paths.FilePerm644)
	if _, err := LoadFromFile(corruptFile); err == nil {
		t.Errorf("expected error decoding corrupt base64")
	}

	// Test RestoreState without force when file exists (fail-close)
	restoreDir := filepath.Join(tmpDir, "restore")
	if err := RestoreState(loaded, restoreDir, false); err != nil {
		t.Fatalf("failed initial restore: %v", err)
	}
	// Second restore should fail because files exist and force=false
	if err := RestoreState(loaded, restoreDir, false); err == nil {
		t.Errorf("expected error restoring over existing files without force")
	}
	// Restore with force=true should succeed
	if err := RestoreState(loaded, restoreDir, true); err != nil {
		t.Fatalf("expected restore with force to succeed, got %v", err)
	}

	// Test restoreConfigFiles conflict
	confFiles := map[string][]byte{"conf1.yaml": []byte("test")}
	if err := restoreConfigFiles(confFiles, restoreDir, false); err == nil {
		t.Errorf("expected conflict in restoreConfigFiles when file exists")
	}
}
