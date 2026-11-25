package ide

import (
	"path/filepath"
	"strings"
)

// ZedAdapter implements Adapter for the Zed editor (.zed/settings.json and ~/.config/zed/settings.json).
type ZedAdapter struct{}

func init() {
	Register(&ZedAdapter{})
}

func (z *ZedAdapter) Name() string   { return "Zed" }
func (z *ZedAdapter) Vendor() string { return "zed" }

func (z *ZedAdapter) WorkspaceConfig(projectRoot string) (string, string) {
	if projectRoot == "" {
		return "", ""
	}
	return "Zed (Workspace)", filepath.Join(projectRoot, ".zed", "settings.json")
}

func (z *ZedAdapter) GlobalConfigs(homeDir string) map[string]string {
	if homeDir == "" {
		return nil
	}
	return map[string]string{
		"Zed": filepath.Join(homeDir, ".config", "zed", "settings.json"),
	}
}

func (z *ZedAdapter) SearchDirs(homeDir, goos, appData string) []string {
	if homeDir == "" {
		return nil
	}
	return []string{filepath.Join(homeDir, ".config", "zed")}
}

func (z *ZedAdapter) KnownFiles() map[string]string {
	return nil
}

// IsForeign returns true for Zed settings.json files because Zed stores MCP entries under
// `context_servers` rather than `mcpServers`, so native ZQK MCP rewriters must not clobber them.
func (z *ZedAdapter) IsForeign(ideName, configPath string) bool {
	lowerName := strings.ToLower(strings.TrimSpace(ideName))
	normPath := strings.ToLower(filepath.ToSlash(strings.TrimSpace(configPath)))
	return strings.HasPrefix(lowerName, "zed") ||
		strings.HasSuffix(normPath, "/.zed/settings.json") ||
		strings.HasSuffix(normPath, "/zed/settings.json")
}

func (z *ZedAdapter) IsStdioAdapter(ideName, configPath string) bool {
	return false
}
