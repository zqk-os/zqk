package ide

import (
	"path/filepath"
)

// GenericAdapter implements Adapter for standard IDE (.ide/mcp.json and VS Code ecosystem).
type GenericAdapter struct{}

func init() {
	Register(&GenericAdapter{})
}

func (g *GenericAdapter) Name() string   { return "IDE" }
func (g *GenericAdapter) Vendor() string { return "generic" }

func (g *GenericAdapter) WorkspaceConfig(projectRoot string) (string, string) {
	if projectRoot == "" {
		return "", ""
	}
	return "IDE (Workspace)", filepath.Join(projectRoot, ".ide", "mcp.json")
}

func (g *GenericAdapter) GlobalConfigs(homeDir string) map[string]string {
	return nil
}

func (g *GenericAdapter) SearchDirs(homeDir, goos, appData string) []string {
	var dirs []string
	if homeDir != "" {
		dirs = append(dirs,
			filepath.Join(homeDir, ".ide"),
			filepath.Join(homeDir, ".config"),
		)
		if goos == "darwin" {
			dirs = append(dirs, filepath.Join(homeDir, "Library", "Application Support", "Code"))
		}
	}
	if goos == "windows" && appData != "" {
		dirs = append(dirs, filepath.Join(appData, "Code"))
	}
	return dirs
}

func (g *GenericAdapter) KnownFiles() map[string]string {
	return map[string]string{
		"mcp.json": "IDE (Global)",
	}
}

func (g *GenericAdapter) IsForeign(ideName, configPath string) bool {
	return false
}

func (g *GenericAdapter) IsStdioAdapter(ideName, configPath string) bool {
	if ideName == "IDE (Workspace)" {
		return true
	}
	parent := filepath.Base(filepath.Dir(configPath))
	return filepath.Base(configPath) == "mcp.json" && parent == ".ide"
}
