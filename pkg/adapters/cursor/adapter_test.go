package cursor_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/adapters/cursor"
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

func TestIsGlobalMCPConfig(t *testing.T) {
	t.Parallel()
	root := "/tmp/proj"
	if cursor.IsGlobalMCPConfig(root, filepath.Join(root, ".cursor", "mcp.json")) {
		t.Fatal("workspace path must not be global")
	}
	if !cursor.IsGlobalMCPConfig(root, "/Users/x/.cursor/mcp.json") {
		t.Fatal("home .cursor/mcp.json must be global")
	}
	if cursor.IsGlobalMCPConfig(root, "") {
		t.Fatal("empty path must not be global")
	}
	if cursor.IsGlobalMCPConfig(root, ".") {
		t.Fatal("dot path must not be global")
	}
	if cursor.IsGlobalMCPConfig(root, "/Users/x/.cursor/other.json") {
		t.Fatal("non-mcp.json file must not be global MCP config")
	}
	if cursor.IsGlobalMCPConfig(root, "/Users/x/.vscode/mcp.json") {
		t.Fatal("non-.cursor dir must not be cursor global MCP config")
	}
}

func TestReadMCPPin(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath := filepath.Join(t.TempDir(), ".cursor", "mcp.json")
	writePinConfig(t, configPath, root)

	got := cursor.ReadMCPPin(configPath)
	if got != root {
		t.Fatalf("want pin %s, got %s", root, got)
	}

	// Non-existent file
	if pin := cursor.ReadMCPPin(filepath.Join(t.TempDir(), "nonexistent.json")); pin != "" {
		t.Fatalf("expected empty pin for nonexistent file, got %s", pin)
	}

	// Corrupt file
	corruptPath := filepath.Join(t.TempDir(), "corrupt.json")
	if err := fileutil.WriteFile(corruptPath, []byte("invalid json"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if pin := cursor.ReadMCPPin(corruptPath); pin != "" {
		t.Fatalf("expected empty pin for corrupt file, got %s", pin)
	}
}

func TestRefuseGlobalMCPPinRewrite_clobberExistingPinRejected(t *testing.T) {
	t.Parallel()
	studio := t.TempDir()
	foreign := t.TempDir()
	homeCursor := filepath.Join(t.TempDir(), ".cursor", "mcp.json")
	writePinConfig(t, homeCursor, studio)

	err := cursor.RefuseGlobalMCPPinRewrite(foreign, homeCursor)
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

	if err := cursor.RefuseGlobalMCPPinRewrite(studio, homeCursor); err != nil {
		t.Fatalf("refreshing same pin must pass: %v", err)
	}
}

func TestRefuseGlobalMCPPinRewrite_optInAllows(t *testing.T) {
	studio := t.TempDir()
	foreign := t.TempDir()
	homeCursor := filepath.Join(t.TempDir(), ".cursor", "mcp.json")
	writePinConfig(t, homeCursor, studio)

	t.Setenv(zqkenv.AllowGlobalMCPPinRewrite().Name(), "1")
	if err := cursor.RefuseGlobalMCPPinRewrite(foreign, homeCursor); err != nil {
		t.Fatalf("human opt-in must allow rewrite: %v", err)
	}
}

func TestRefuseGlobalMCPPinRewrite_nonGlobalConfigAllowed(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	wsPath := filepath.Join(root, ".cursor", "mcp.json")
	writePinConfig(t, wsPath, root)

	if err := cursor.RefuseGlobalMCPPinRewrite(root, wsPath); err != nil {
		t.Fatalf("workspace config must be allowed: %v", err)
	}
}

func TestAdapter_Compliance(t *testing.T) {
	a := cursor.New()
	var _ nodesovereignty.IDEPinAdapter = a

	if a.Vendor() != cursor.VendorID {
		t.Fatalf("want vendor %s, got %s", cursor.VendorID, a.Vendor())
	}

	root := "/tmp/proj"
	if a.IsGlobalConfig(root, filepath.Join(root, ".cursor", "mcp.json")) {
		t.Fatal("workspace config must not be global")
	}
	if !a.IsGlobalConfig(root, "/Users/test/.cursor/mcp.json") {
		t.Fatal("home config must be global")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	wantPath := filepath.Join(home, ".cursor", "mcp.json")
	if a.GlobalConfigPath() != wantPath {
		t.Fatalf("want global config path %s, got %s", wantPath, a.GlobalConfigPath())
	}
}
