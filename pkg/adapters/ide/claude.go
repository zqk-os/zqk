package ide

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ClaudeAdapter implements Adapter for Anthropic Claude (Desktop & Code).
type ClaudeAdapter struct{}

func init() {
	Register(&ClaudeAdapter{})
}

func (c *ClaudeAdapter) Name() string   { return "Claude" }
func (c *ClaudeAdapter) Vendor() string { return "anthropic" }

func (c *ClaudeAdapter) WorkspaceConfig(projectRoot string) (string, string) {
	if projectRoot == "" {
		return "", ""
	}
	return "Claude Code (Workspace)", filepath.Join(projectRoot, ".claude.json")
}

func (c *ClaudeAdapter) GlobalConfigs(homeDir string) map[string]string {
	if homeDir == "" {
		return nil
	}
	globalFile := filepath.Join(homeDir, ".claude.json")
	if fileutil.Exists(globalFile) {
		return map[string]string{
			"Claude Code (Global)": globalFile,
		}
	}
	return nil
}

func (c *ClaudeAdapter) SearchDirs(homeDir, goos, appData string) []string {
	var dirs []string
	if goos == "darwin" && homeDir != "" {
		dirs = append(dirs, filepath.Join(homeDir, "Library", "Application Support", "Claude"))
	} else if goos == "windows" && appData != "" {
		dirs = append(dirs, filepath.Join(appData, "Claude"))
	}
	return dirs
}

func (c *ClaudeAdapter) KnownFiles() map[string]string {
	return map[string]string{
		"claude_desktop_config.json": "Claude Desktop",
		".claude.json":               "Claude Code",
	}
}

func (c *ClaudeAdapter) IsForeign(ideName, configPath string) bool {
	return false
}

func (c *ClaudeAdapter) IsStdioAdapter(ideName, configPath string) bool {
	return false
}
