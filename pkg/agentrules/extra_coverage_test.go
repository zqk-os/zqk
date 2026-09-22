package agentrules

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestWriteManifestFromDisk_AndLoadErrors(t *testing.T) {
	root := t.TempDir()
	rulesDir := filepath.Join(root, ".cursor", "rules")
	if err := fileutil.EnsureDir(rulesDir); err != nil {
		t.Fatal(err)
	}

	// 1. Create a couple .mdc files and a non-mdc file
	if err := fileutil.WriteSecureFile(filepath.Join(rulesDir, "01-first.mdc"), []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(filepath.Join(rulesDir, "02-second.mdc"), []byte("second")); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(filepath.Join(rulesDir, "ignored.txt"), []byte("ignored")); err != nil {
		t.Fatal(err)
	}

	// 2. WriteManifestFromDisk
	if err := WriteManifestFromDisk(root, ""); err != nil {
		t.Fatalf("WriteManifestFromDisk failed: %v", err)
	}

	// 3. LoadManifest and Validate
	m, err := LoadManifest(root)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}
	if m.Version != 1 {
		t.Errorf("expected version 1, got %d", m.Version)
	}
	if len(m.Rules) != 2 || m.Rules[0] != "01-first.mdc" || m.Rules[1] != "02-second.mdc" {
		t.Errorf("unexpected rules in manifest: %v", m.Rules)
	}

	if err := Validate(root, ""); err != nil {
		t.Errorf("expected Validate to succeed after WriteManifestFromDisk: %v", err)
	}

	// 4. LoadManifest errors: invalid version 0
	cfgDir := filepath.Dir(ManifestPath(root))
	if err := fileutil.WriteSecureFile(filepath.Join(cfgDir, ManifestFileName), []byte("version: 0\nrules: []\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(root); err == nil {
		t.Errorf("expected error for manifest version 0")
	}

	// 5. LoadManifest errors: malformed YAML
	if err := fileutil.WriteSecureFile(filepath.Join(cfgDir, ManifestFileName), []byte(":::invalid yaml:::")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(root); err == nil {
		t.Errorf("expected error for malformed YAML")
	}

	// 6. WriteManifestFromDisk with missing rules dir
	if err := WriteManifestFromDisk(root, "non_existent_rules_dir"); err == nil {
		t.Errorf("expected error writing manifest from non-existent rules dir")
	}

	// 7. ResolveRulesDir precedence
	pFlag := ResolveRulesDir(root, "custom/dir")
	if pFlag != filepath.Join(root, "custom/dir") {
		t.Errorf("unexpected pFlag: %s", pFlag)
	}
	t.Setenv(zqkenv.AgentRulesDir().Name(), "env/dir")
	pEnv := ResolveRulesDir(root, "")
	if pEnv != filepath.Join(root, "env/dir") {
		t.Errorf("unexpected pEnv: %s", pEnv)
	}
	t.Setenv(zqkenv.AgentRulesDir().Name(), "")
	pDefault := ResolveRulesDir(root, "")
	if pDefault != filepath.Join(root, ".cursor/rules") {
		t.Errorf("unexpected pDefault: %s", pDefault)
	}
}

