package ide

import (
	"path/filepath"
)

// RooCodeAdapter implements Adapter for the Roo Code IDE extension.
type RooCodeAdapter struct{}

func init() {
	Register(&RooCodeAdapter{})
}

func (r *RooCodeAdapter) Name() string   { return "Roo Code" }
func (r *RooCodeAdapter) Vendor() string { return "roo_code" }

func (r *RooCodeAdapter) WorkspaceConfig(projectRoot string) (string, string) {
	if projectRoot == "" {
		return "", ""
	}
	return "Roo Code (Workspace)", filepath.Join(projectRoot, ".roo", "mcp.json")
}

func (r *RooCodeAdapter) GlobalConfigs(homeDir string) map[string]string {
	return nil
}

func (r *RooCodeAdapter) SearchDirs(homeDir, goos, appData string) []string {
	return nil
}

func (r *RooCodeAdapter) KnownFiles() map[string]string {
	return map[string]string{
		"roo_mcp_settings.json": "Roo Code",
	}
}

func (r *RooCodeAdapter) IsForeign(ideName, configPath string) bool {
	return false
}

func (r *RooCodeAdapter) IsStdioAdapter(ideName, configPath string) bool {
	return false
}
