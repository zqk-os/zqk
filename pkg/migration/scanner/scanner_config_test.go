package scanner

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestGetGlobalScannerConfig_readsFileFromCwd(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, paths.ProcessInternalDir, paths.ScannerConfigFile)
	if err := fileutil.EnsureDir(filepath.Dir(configPath)); err != nil {
		t.Fatal(err)
	}
	body := []byte("exclude_directories:\n  - only_this\n")
	if err := fileutil.WriteStandardFile(configPath, body); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	paths.ResetCwdDiscovery()
	ResetGlobalScannerConfig()
	t.Cleanup(func() {
		paths.ResetCwdDiscovery()
		ResetGlobalScannerConfig()
	})

	cfg := GetGlobalScannerConfig()
	if len(cfg.ExcludeDirectories) != 1 || cfg.ExcludeDirectories[0] != "only_this" {
		t.Fatalf("ExcludeDirectories = %#v", cfg.ExcludeDirectories)
	}
}
