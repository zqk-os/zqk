package nodesovereignty_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/nodesovereignty"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func writePinConfig(t *testing.T, path, projectRoot string) {
	t.Helper()
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	doc := map[string]any{
		"mcpServers": map[string]any{
			"zqk": map[string]any{
				"type":    "stdio",
				"command": filepath.Join(projectRoot, "bin", "zqk-mcp-ide-adapter"),
				"args":    []string{"mcp", "ide-adapter", "--tcp", "127.0.0.1:8443"},
				"cwd":     projectRoot,
				"env": map[string]any{
					zqkenv.ProjectRoot().Name(): projectRoot,
				},
			},
		},
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(path, append(b, '\n'), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
}

func TestRefuseGlobalMCPPinRewrite_clobberExistingPinRejected(t *testing.T) {
	t.Parallel()
	studio := t.TempDir()
	foreign := t.TempDir()
	homeCursor := filepath.Join(t.TempDir(), ".cursor", "mcp.json")
	writePinConfig(t, homeCursor, studio)
	err := nodesovereignty.RefuseGlobalMCPPinRewrite(foreign, homeCursor)
	if err == nil {
		t.Fatal("foreign root must not rewrite existing studio pin")
	}
	if !strings.Contains(err.Error(), "refuse global mcp.json") {
		t.Fatalf("want refuse global mcp.json, got: %v", err)
	}
}

func TestRefuseGlobalMCPPinRewrite_samePinAllowed(t *testing.T) {
	t.Parallel()
	studio := t.TempDir()
	homeCursor := filepath.Join(t.TempDir(), ".cursor", "mcp.json")
	writePinConfig(t, homeCursor, studio)
	if err := nodesovereignty.RefuseGlobalMCPPinRewrite(studio, homeCursor); err != nil {
		t.Fatalf("refreshing same pin must pass: %v", err)
	}
}

func TestRefuseGlobalMCPPinRewrite_optInAllows(t *testing.T) {
	studio := t.TempDir()
	foreign := t.TempDir()
	homeCursor := filepath.Join(t.TempDir(), ".cursor", "mcp.json")
	writePinConfig(t, homeCursor, studio)
	t.Setenv(zqkenv.AllowGlobalMCPPinRewrite().Name(), "1")
	if err := nodesovereignty.RefuseGlobalMCPPinRewrite(foreign, homeCursor); err != nil {
		t.Fatalf("human opt-in must allow rewrite: %v", err)
	}
}

func TestIsGlobalCursorMCPConfig(t *testing.T) {
	t.Parallel()
	root := "/tmp/proj"
	if nodesovereignty.IsGlobalCursorMCPConfig(root, filepath.Join(root, ".cursor", "mcp.json")) {
		t.Fatal("workspace path must not be global")
	}
	if !nodesovereignty.IsGlobalCursorMCPConfig(root, "/Users/x/.cursor/mcp.json") {
		t.Fatal("home .cursor/mcp.json must be global")
	}
}
