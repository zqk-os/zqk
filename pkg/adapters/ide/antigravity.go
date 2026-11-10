package ide

import (
	"path/filepath"
	"strings"
)

// AntigravityAdapter implements Adapter for Google Antigravity / Gemini.
type AntigravityAdapter struct{}

func init() {
	Register(&AntigravityAdapter{})
}

func (a *AntigravityAdapter) Name() string   { return "Antigravity" }
func (a *AntigravityAdapter) Vendor() string { return "google" }

func (a *AntigravityAdapter) WorkspaceConfig(projectRoot string) (string, string) {
	return "", ""
}

func (a *AntigravityAdapter) GlobalConfigs(homeDir string) map[string]string {
	return nil
}

func (a *AntigravityAdapter) SearchDirs(homeDir, goos, appData string) []string {
	if homeDir == "" {
		return nil
	}
	return []string{
		filepath.Join(homeDir, ".gemini"),
	}
}

func (a *AntigravityAdapter) KnownFiles() map[string]string {
	return map[string]string{
		"mcp_config.json": "Global Daemon (AGY/Windsurf)",
	}
}

func (a *AntigravityAdapter) IsForeign(ideName, configPath string) bool {
	p := filepath.ToSlash(configPath)
	if strings.Contains(p, "/.gemini/") {
		return true
	}
	return strings.Contains(ideName, "AGY")
}

func (a *AntigravityAdapter) IsStdioAdapter(ideName, configPath string) bool {
	return false
}
