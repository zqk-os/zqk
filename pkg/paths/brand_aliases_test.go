package paths

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestBrandAliases_LoadBrandCLIBinaryPath(t *testing.T) {
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, ConfigDir)
	if err := fileutil.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfgFile := filepath.Join(cfgDir, ZqkConfigFileName)
	content := []byte("cli:\n  binary_path: /custom/bin/zqk\npaths:\n  aliases:\n    foo: bar\n")
	if err := fileutil.WriteFile(cfgFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	binPath := LoadBrandCLIBinaryPath(tmp)
	if binPath != "/custom/bin/zqk" {
		t.Fatalf("expected /custom/bin/zqk, got %q", binPath)
	}

	aliases := LoadBrandPathAliases(tmp)
	if aliases == nil || aliases["foo"] != "bar" {
		t.Fatalf("expected alias foo: bar, got %v", aliases)
	}

	brandPath := BrandSettingsPath(tmp)
	if brandPath != cfgFile {
		t.Fatalf("expected brand path %q, got %q", cfgFile, brandPath)
	}

	// Empty root
	if LoadBrandCLIBinaryPath("") != "" {
		t.Fatalf("expected empty binary path for empty root")
	}
	if LoadBrandPathAliases("") != nil {
		t.Fatalf("expected nil aliases for empty root")
	}
}
