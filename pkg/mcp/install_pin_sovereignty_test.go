package mcp

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestInstallToIDE_RefuseGlobalPinRewrite_Negative(t *testing.T) {
	studio := t.TempDir()
	foreign := t.TempDir()
	for _, root := range []string{studio, foreign} {
		if err := fileutil.MkdirAll(filepath.Join(root, "bin"), paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(root, "bin", brand.ExecutableName())
		if err := fileutil.WriteFile(target, []byte("#!/bin/sh\n"), paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	global := filepath.Join(home, ".cursor", "mcp.json")
	if err := fileutil.MkdirAll(filepath.Dir(global), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	existing := map[string]any{
		"mcpServers": map[string]any{
			"zqk": map[string]any{
				"type":    "stdio",
				"command": filepath.Join(studio, "bin", "zqk-mcp-ide-adapter"),
				"cwd":     studio,
				"env": map[string]any{
					zqkenv.ProjectRoot().Name(): studio,
				},
			},
		},
	}
	b, _ := json.MarshalIndent(existing, "", "  ")
	if err := fileutil.WriteFile(global, append(b, '\n'), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	before, err := fileutil.ReadFile(global)
	if err != nil {
		t.Fatal(err)
	}
	err = InstallToIDE("Cursor (Global)", global, filepath.Join(foreign, "bin", brand.ExecutableName()), foreign, testMCPLogger())
	if err == nil {
		t.Fatal("foreign InstallToIDE must refuse clobbering studio global pin")
	}
	if !strings.Contains(err.Error(), "refuse global mcp.json") {
		t.Fatalf("want sovereignty refuse, got: %v", err)
	}
	after, err := fileutil.ReadFile(global)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("global mcp.json mutated despite refuse")
	}
}
