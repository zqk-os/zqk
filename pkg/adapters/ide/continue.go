package ide

import (
	"path/filepath"
	"strings"
)

// ContinueAdapter implements Adapter for the Continue.dev IDE extension
// (.continue/config.yaml and ~/.continue/config.yaml).
type ContinueAdapter struct{}

func init() {
	Register(&ContinueAdapter{})
}

func (c *ContinueAdapter) Name() string   { return "Continue" }
func (c *ContinueAdapter) Vendor() string { return "continue" }

func (c *ContinueAdapter) WorkspaceConfig(projectRoot string) (string, string) {
	if projectRoot == "" {
		return "", ""
	}
	return "Continue (Workspace)", filepath.Join(projectRoot, ".continue", "config.yaml")
}

func (c *ContinueAdapter) GlobalConfigs(homeDir string) map[string]string {
	if homeDir == "" {
		return nil
	}
	return map[string]string{
		"Continue": filepath.Join(homeDir, ".continue", "config.yaml"),
	}
}

func (c *ContinueAdapter) SearchDirs(homeDir, goos, appData string) []string {
	if homeDir == "" {
		return nil
	}
	return []string{filepath.Join(homeDir, ".continue")}
}

func (c *ContinueAdapter) KnownFiles() map[string]string {
	return nil
}

// IsForeign returns true for Continue.dev config.yaml files because Continue stores MCP
// servers in a YAML schema rather than JSON mcpServers.
func (c *ContinueAdapter) IsForeign(ideName, configPath string) bool {
	lowerName := strings.ToLower(strings.TrimSpace(ideName))
	normPath := strings.ToLower(filepath.ToSlash(strings.TrimSpace(configPath)))
	return strings.HasPrefix(lowerName, "continue") ||
		strings.HasSuffix(normPath, "/.continue/config.yaml")
}

func (c *ContinueAdapter) IsStdioAdapter(ideName, configPath string) bool {
	return false
}
