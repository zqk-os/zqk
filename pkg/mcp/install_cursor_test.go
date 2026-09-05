package mcp

import (
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/brand"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func testMCPLogger() logging.Logger {
	return logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(pkgctx.NewSystemContext()))
}

func TestIsForeignMCPConfig(t *testing.T) {
	t.Parallel()
	if !isForeignMCPConfig("Global Daemon (AGY/Windsurf)", "/Users/x/.gemini/config/mcp_config.json") {
		t.Fatal("gemini AGY config must be foreign")
	}
	if isForeignMCPConfig("Cursor (Workspace)", "/tmp/proj/.cursor/mcp.json") {
		t.Fatal("workspace mcp.json must stay owned")
	}
}

func TestInstallToIDE_skipsEmptyGeminiConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, ".gemini", "config", "mcp_config.json")
	if err := fileutil.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := InstallToIDE("Global Daemon (AGY/Windsurf) ("+path+")", path, "/bin/zqk", t.TempDir(), testMCPLogger()); err != nil {
		t.Fatalf("empty gemini config should skip, not error: %v", err)
	}
}

func TestIsIDEStdioAdapterConfig(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, path string
		want       bool
	}{
		{"Cursor (Workspace)", "/tmp/proj/.cursor/mcp.json", true},
		{"Cursor (Global)", "/Users/x/.cursor/mcp.json", true},
		{"IDE (Workspace)", "/tmp/proj/.ide/mcp.json", true},
		{"other", "/tmp/proj/.cursor/mcp.json", true},
		{"Claude Desktop", "/tmp/claude_desktop_config.json", false},
	}
	for _, tc := range cases {
		if got := isIDEStdioAdapterConfig(tc.name, tc.path); got != tc.want {
			t.Fatalf("%s %s: got %v want %v", tc.name, tc.path, got, tc.want)
		}
	}
}

func TestFindMCPConfigsIncludesCursorPaths(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	root := t.TempDir()
	got := findMCPConfigs(home, root)
	wantCursorWS := filepath.Join(root, ".cursor", "mcp.json")
	wantCursorG := filepath.Join(home, ".cursor", "mcp.json")
	if got["Cursor (Workspace)"] != wantCursorWS {
		t.Fatalf("workspace path: got %q want %q", got["Cursor (Workspace)"], wantCursorWS)
	}
	if got["Cursor (Global)"] != wantCursorG {
		t.Fatalf("global path: got %q want %q", got["Cursor (Global)"], wantCursorG)
	}
}

func TestInstallToIDE_CursorMcpJSONIncludesStdioType(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := fileutil.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(binDir, brand.ExecutableName())
	if err := fileutil.WriteFile(target, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(root, ".cursor", "mcp.json")
	if err := InstallToIDE("Cursor (Workspace)", cfgPath, target, root, testMCPLogger()); err != nil {
		t.Fatalf("InstallToIDE: %v", err)
	}
	raw, err := fileutil.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg mcpConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse: %v\n%s", err, raw)
	}
	entry, ok := cfg.MCPServers[brand.ExecutableName()]
	if !ok {
		t.Fatalf("missing server %q in %s", brand.ExecutableName(), raw)
	}
	if entry.Type != mcpTransportStdio {
		t.Fatalf("type=%q want %q", entry.Type, mcpTransportStdio)
	}
	if !strings.Contains(entry.Command, "${workspaceFolder}/bin/") {
		t.Fatalf("command=%q want workspaceFolder bin interpolation", entry.Command)
	}
	if len(entry.Args) < 2 || entry.Args[0] != "mcp" || entry.Args[1] != "ide-adapter" {
		t.Fatalf("args=%v want mcp ide-adapter …", entry.Args)
	}
}
