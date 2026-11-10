package ide

import (
	"path/filepath"
)

// ClineAdapter implements Adapter for Cline IDE extension.
type ClineAdapter struct{}

func init() {
	Register(&ClineAdapter{})
}

func (c *ClineAdapter) Name() string   { return "Cline" }
func (c *ClineAdapter) Vendor() string { return "cline" }

func (c *ClineAdapter) WorkspaceConfig(projectRoot string) (string, string) {
	if projectRoot == "" {
		return "", ""
	}
	return "Cline (Workspace)", filepath.Join(projectRoot, ".cline", "mcp_settings.json")
}

func (c *ClineAdapter) GlobalConfigs(homeDir string) map[string]string {
	return nil
}

func (c *ClineAdapter) SearchDirs(homeDir, goos, appData string) []string {
	return nil
}

func (c *ClineAdapter) KnownFiles() map[string]string {
	return map[string]string{
		"cline_mcp_settings.json": "Cline",
	}
}

func (c *ClineAdapter) IsForeign(ideName, configPath string) bool {
	return false
}

func (c *ClineAdapter) IsStdioAdapter(ideName, configPath string) bool {
	return false
}
