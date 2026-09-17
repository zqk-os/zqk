package agentrules

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestValidate_OK(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	rulesDir := filepath.Join(root, ".ide", "rules")
	if err := fileutil.EnsureDir(rulesDir); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(filepath.Join(rulesDir, "a.mdc"), []byte("---\nalwaysApply: true\n---\n")); err != nil {
		t.Fatal(err)
	}
	cfgDir := filepath.Dir(ManifestPath(root))
	if err := fileutil.EnsureDir(cfgDir); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(filepath.Join(cfgDir, ManifestFileName), []byte("version: 1\nrules:\n  - a.mdc\n")); err != nil {
		t.Fatal(err)
	}
	if err := Validate(root, ""); err != nil {
		t.Fatal(err)
	}
}

func TestValidate_CustomRulesDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	custom := filepath.Join(root, ".agent", "rules")
	if err := fileutil.EnsureDir(custom); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(filepath.Join(custom, "x.mdc"), []byte("x")); err != nil {
		t.Fatal(err)
	}
	cfgDir := filepath.Dir(ManifestPath(root))
	if err := fileutil.EnsureDir(cfgDir); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(filepath.Join(cfgDir, ManifestFileName), []byte("version: 1\nrules:\n  - x.mdc\n")); err != nil {
		t.Fatal(err)
	}
	if err := Validate(root, ".agent/rules"); err != nil {
		t.Fatal(err)
	}
}

func TestValidate_Mismatch(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := fileutil.EnsureDir(filepath.Join(root, ".ide", "rules")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.mdc", "orphan.mdc"} {
		if err := fileutil.WriteSecureFile(filepath.Join(root, ".ide", "rules", name), []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	cfgDir := filepath.Dir(ManifestPath(root))
	if err := fileutil.EnsureDir(cfgDir); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(filepath.Join(cfgDir, ManifestFileName), []byte("version: 1\nrules:\n  - a.mdc\n")); err != nil {
		t.Fatal(err)
	}
	if err := Validate(root, ""); err == nil {
		t.Fatal("expected error for orphan rule file")
	}
}
