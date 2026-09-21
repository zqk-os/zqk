package mcp

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestLoadMCPConfig_reloadsWhenFileAppears(t *testing.T) {
	root := t.TempDir()
	cfg, err := LoadMCPConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MCPServer.Tools.AliasMode == nil || !*cfg.MCPServer.Tools.AliasMode {
		t.Fatal("greenfield default must enable alias_mode")
	}

	configPath := paths.MCPConfigPath(root)
	if err := fileutil.EnsureDir(filepath.Dir(configPath)); err != nil {
		t.Fatal(err)
	}
	body := []byte("mcp_server:\n  idle_timeout: 99m\n")
	if err := fileutil.WriteStandardFile(configPath, body); err != nil {
		t.Fatal(err)
	}

	cfg2, err := LoadMCPConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.MCPServer.IdleTimeout != "99m" {
		t.Fatalf("idle_timeout got %q want 99m", cfg2.MCPServer.IdleTimeout)
	}
}
