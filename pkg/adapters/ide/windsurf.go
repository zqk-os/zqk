package ide

import (
	"path/filepath"
	"strings"
)

// WindsurfAdapter implements Adapter for Codeium Windsurf.
type WindsurfAdapter struct{}

func init() {
	Register(&WindsurfAdapter{})
}

func (w *WindsurfAdapter) Name() string   { return "Windsurf" }
func (w *WindsurfAdapter) Vendor() string { return "codeium" }

func (w *WindsurfAdapter) WorkspaceConfig(projectRoot string) (string, string) {
	if projectRoot == "" {
		return "", ""
	}
	return "Windsurf (Workspace)", filepath.Join(projectRoot, ".codeium", "windsurf", "mcp_config.json")
}

func (w *WindsurfAdapter) GlobalConfigs(homeDir string) map[string]string {
	return nil
}

func (w *WindsurfAdapter) SearchDirs(homeDir, goos, appData string) []string {
	var dirs []string
	if homeDir != "" {
		dirs = append(dirs, filepath.Join(homeDir, ".codeium"))
		if goos == "darwin" {
			dirs = append(dirs, filepath.Join(homeDir, "Library", "Application Support", "Windsurf"))
		}
	}
	if goos == "windows" && appData != "" {
		dirs = append(dirs, filepath.Join(appData, "Windsurf"))
	}
	return dirs
}

func (w *WindsurfAdapter) KnownFiles() map[string]string {
	return map[string]string{
		"mcp_config.json": "Global Daemon (AGY/Windsurf)",
	}
}

func (w *WindsurfAdapter) IsForeign(ideName, configPath string) bool {
	p := filepath.ToSlash(configPath)
	if strings.Contains(p, "/.codeium/") {
		return true
	}
	return strings.Contains(ideName, "Windsurf")
}

func (w *WindsurfAdapter) IsStdioAdapter(ideName, configPath string) bool {
	return false
}
