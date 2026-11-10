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

type mockIDEPinAdapter struct {
	vendor    string
	configDir string
}

func (m *mockIDEPinAdapter) Vendor() string { return m.vendor }

func (m *mockIDEPinAdapter) IsGlobalConfig(projectRoot, configPath string) bool {
	clean := filepath.Clean(strings.TrimSpace(configPath))
	if clean == "" || clean == "." {
		return false
	}
	if filepath.Base(clean) != "mcp.json" || filepath.Base(filepath.Dir(clean)) != m.configDir {
		return false
	}
	if projectRoot != "" {
		ws := filepath.Clean(filepath.Join(projectRoot, m.configDir, "mcp.json"))
		if clean == ws {
			return false
		}
	}
	return true
}

func (m *mockIDEPinAdapter) ReadPin(configPath string) string {
	return nodesovereignty.ReadGlobalMCPPin(configPath)
}

func (m *mockIDEPinAdapter) GlobalConfigPath() string {
	home, err := fileutil.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, m.configDir, "mcp.json")
}

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
	mockAdapter := &mockIDEPinAdapter{vendor: "test-ide-refuse", configDir: ".mockide"}
	homeConfig := filepath.Join(t.TempDir(), ".mockide", "mcp.json")
	writePinConfig(t, homeConfig, studio)

	err := nodesovereignty.RefuseGlobalMCPPinRewrite(foreign, homeConfig, mockAdapter)
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
	mockAdapter := &mockIDEPinAdapter{vendor: "test-ide-same", configDir: ".mockide"}
	homeConfig := filepath.Join(t.TempDir(), ".mockide", "mcp.json")
	writePinConfig(t, homeConfig, studio)

	if err := nodesovereignty.RefuseGlobalMCPPinRewrite(studio, homeConfig, mockAdapter); err != nil {
		t.Fatalf("refreshing same pin must pass: %v", err)
	}
}

func TestRefuseGlobalMCPPinRewrite_optInAllows(t *testing.T) {
	studio := t.TempDir()
	foreign := t.TempDir()
	mockAdapter := &mockIDEPinAdapter{vendor: "test-ide-optin", configDir: ".mockide"}
	homeConfig := filepath.Join(t.TempDir(), ".mockide", "mcp.json")
	writePinConfig(t, homeConfig, studio)

	t.Setenv(zqkenv.AllowGlobalMCPPinRewrite().Name(), "1")
	if err := nodesovereignty.RefuseGlobalMCPPinRewrite(foreign, homeConfig, mockAdapter); err != nil {
		t.Fatalf("human opt-in must allow rewrite: %v", err)
	}
}

func TestRefuseGlobalMCPPinRewrite_emptyProjectRoot(t *testing.T) {
	t.Parallel()
	studio := t.TempDir()
	mockAdapter := &mockIDEPinAdapter{vendor: "test-ide-empty", configDir: ".mockide"}
	homeConfig := filepath.Join(t.TempDir(), ".mockide", "mcp.json")
	writePinConfig(t, homeConfig, studio)

	err := nodesovereignty.RefuseGlobalMCPPinRewrite("", homeConfig, mockAdapter)
	if err == nil {
		t.Fatal("empty project root must fail closed")
	}
	if !strings.Contains(err.Error(), "empty project root") {
		t.Fatalf("want empty project root error, got: %v", err)
	}
}

func TestIsGlobalMCPConfig(t *testing.T) {
	t.Parallel()
	root := "/tmp/proj"
	if nodesovereignty.IsGlobalMCPConfig(root, filepath.Join(root, ".ide", "mcp.json")) {
		t.Fatal("workspace path must not be global")
	}
	if !nodesovereignty.IsGlobalMCPConfig(root, "/Users/x/.ide/mcp.json") {
		t.Fatal("external mcp.json must be global")
	}
	if nodesovereignty.IsGlobalMCPConfig(root, "") {
		t.Fatal("empty path must not be global")
	}
	if nodesovereignty.IsGlobalMCPConfig(root, ".") {
		t.Fatal("dot path must not be global")
	}

	adapter := &mockIDEPinAdapter{vendor: "test-is-global", configDir: ".special"}
	if !nodesovereignty.IsGlobalMCPConfig(root, "/Users/x/.special/mcp.json", adapter) {
		t.Fatal("delegated adapter global config must report true")
	}
	if nodesovereignty.IsGlobalMCPConfig(root, filepath.Join(root, ".special", "mcp.json"), adapter) {
		t.Fatal("delegated adapter workspace config must report false")
	}
}

func TestReadGlobalMCPPin(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "mcp.json")
	writePinConfig(t, configPath, root)

	got := nodesovereignty.ReadGlobalMCPPin(configPath)
	if got != root {
		t.Fatalf("want pin %s, got %s", root, got)
	}

	if pin := nodesovereignty.ReadGlobalMCPPin(filepath.Join(t.TempDir(), "nonexistent.json")); pin != "" {
		t.Fatalf("want empty pin for missing file, got %s", pin)
	}
}

func TestRegisterIDEPinAdapter(t *testing.T) {
	t.Parallel()
	nodesovereignty.RegisterIDEPinAdapter(nil) // nil-safe
	adapter := &mockIDEPinAdapter{vendor: "test-reg-idempotent", configDir: ".mockreg"}
	nodesovereignty.RegisterIDEPinAdapter(adapter)
	nodesovereignty.RegisterIDEPinAdapter(adapter) // idempotent
}
