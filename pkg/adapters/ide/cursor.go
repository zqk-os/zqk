package ide

import (
	"path/filepath"
)

// CursorAdapter implements Adapter for Cursor IDE.
type CursorAdapter struct{}

func init() {
	Register(&CursorAdapter{})
}

func (c *CursorAdapter) Name() string   { return "Cursor" }
func (c *CursorAdapter) Vendor() string { return "cursor" }

func (c *CursorAdapter) WorkspaceConfig(projectRoot string) (string, string) {
	if projectRoot == "" {
		return "", ""
	}
	return "Cursor (Workspace)", filepath.Join(projectRoot, ".cursor", "mcp.json")
}

func (c *CursorAdapter) GlobalConfigs(homeDir string) map[string]string {
	if homeDir == "" {
		return nil
	}
	return map[string]string{
		"Cursor (Global)": filepath.Join(homeDir, ".cursor", "mcp.json"),
	}
}

func (c *CursorAdapter) SearchDirs(homeDir, goos, appData string) []string {
	var dirs []string
	if goos == "darwin" && homeDir != "" {
		dirs = append(dirs, filepath.Join(homeDir, "Library", "Application Support", "Cursor"))
	} else if goos == "windows" && appData != "" {
		dirs = append(dirs, filepath.Join(appData, "Cursor"))
	}
	return dirs
}

func (c *CursorAdapter) KnownFiles() map[string]string {
	return nil
}

func (c *CursorAdapter) IsForeign(ideName, configPath string) bool {
	return false
}

func (c *CursorAdapter) IsStdioAdapter(ideName, configPath string) bool {
	switch ideName {
	case "Cursor (Workspace)", "Cursor (Global)":
		return true
	}
	parent := filepath.Base(filepath.Dir(configPath))
	return filepath.Base(configPath) == "mcp.json" && parent == ".cursor"
}
