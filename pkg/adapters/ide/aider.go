package ide

import (
	"path/filepath"
	"strings"
)

// AiderAdapter implements Adapter for the Aider CLI coding assistant
// (.aider.conf.yml and ~/.aider.conf.yml).
type AiderAdapter struct{}

func init() {
	Register(&AiderAdapter{})
}

func (a *AiderAdapter) Name() string   { return "Aider" }
func (a *AiderAdapter) Vendor() string { return "aider" }

func (a *AiderAdapter) WorkspaceConfig(projectRoot string) (string, string) {
	if projectRoot == "" {
		return "", ""
	}
	return "Aider (Workspace)", filepath.Join(projectRoot, ".aider.conf.yml")
}

func (a *AiderAdapter) GlobalConfigs(homeDir string) map[string]string {
	if homeDir == "" {
		return nil
	}
	return map[string]string{
		"Aider": filepath.Join(homeDir, ".aider.conf.yml"),
	}
}

func (a *AiderAdapter) SearchDirs(homeDir, goos, appData string) []string {
	if homeDir == "" {
		return nil
	}
	return []string{homeDir}
}

func (a *AiderAdapter) KnownFiles() map[string]string {
	return nil
}

// IsForeign returns true for Aider .aider.conf.yml files because Aider uses a
// YAML CLI configuration schema rather than JSON mcpServers.
func (a *AiderAdapter) IsForeign(ideName, configPath string) bool {
	lowerName := strings.ToLower(strings.TrimSpace(ideName))
	normPath := strings.ToLower(filepath.ToSlash(strings.TrimSpace(configPath)))
	return strings.HasPrefix(lowerName, "aider") ||
		strings.HasSuffix(normPath, "/.aider.conf.yml")
}

func (a *AiderAdapter) IsStdioAdapter(ideName, configPath string) bool {
	return false
}
