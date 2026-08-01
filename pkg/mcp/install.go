package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	cliContext "github.com/lanceman/zqk/internal/cli/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

type mcpConfig struct {
	MCPServers map[string]mcpServerConfig `json:"mcpServers"`
}

type mcpServerConfig struct {
	Command  string            `json:"command"`
	Args     []string          `json:"args"`
	Env      map[string]string `json:"env,omitempty"`
	Cwd      string            `json:"cwd,omitempty"`
	Disabled bool              `json:"disabled,omitempty"`
}

// AutoInstall registers the ZQK MCP server with supported IDEs and Agents.
func AutoInstall(projectRoot string, logger logging.Logger) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	configs := findMCPConfigs(homeDir, projectRoot)

	successCount := 0

	for ideName, configPath := range configs {
		if err := InstallToIDE(ideName, configPath, execPath, projectRoot, logger); err != nil {
			logging.Fluent(logger).Warn("Failed to install MCP server").String("ide", ideName).WithError(err).Log()
		} else {
			successCount++
		}
	}

	if successCount == 0 {
		return fmt.Errorf("no supported IDE configurations found or updated")
	}

	logging.Fluent(logger).Info("Successfully installed ZQK MCP server").Int("ides_configured", successCount).Log()
	return nil
}

// findMCPConfigs dynamically searches common directories for known MCP config files
func findMCPConfigs(homeDir, projectRoot string) map[string]string {
	configs := make(map[string]string)

	// Always ensure Cursor workspace is targeted if in a project
	if projectRoot != "" {
		configs["Cursor (Workspace)"] = filepath.Join(projectRoot, ".cursor", "mcp.json")
	}

	knownFiles := map[string]string{
		"mcp_config.json":            "Global Daemon (AGY/Windsurf)",
		"mcp.json":                   "Cursor (Global)",
		"claude_desktop_config.json": "Claude Desktop",
		"cline_mcp_settings.json":    "Cline",
	}

	searchDirs := []string{
		filepath.Join(homeDir, ".codeium"),
		filepath.Join(homeDir, ".gemini"),
		filepath.Join(homeDir, ".cursor"),
		filepath.Join(homeDir, ".config"),
	}

	if runtime.GOOS == "darwin" {
		searchDirs = append(searchDirs, filepath.Join(homeDir, "Library", "Application Support"))
	} else if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			searchDirs = append(searchDirs, appData)
		}
	}

	matches := fileutil.FindFiles(searchDirs, knownFiles)
	for _, match := range matches {
		// Prevent overwriting Cursor workspace config if we stumble upon it
		if projectRoot != "" && filepath.Clean(match.Path) == filepath.Clean(filepath.Join(projectRoot, ".cursor", "mcp.json")) {
			continue
		}
		// Use the path as the key to ensure uniqueness if multiple are found
		configs[fmt.Sprintf("%s (%s)", match.Identifier, match.Path)] = match.Path
	}

	return configs
}

// InstallToIDE registers the ZQK MCP server in the specific IDE configuration file.
func InstallToIDE(ideName, configPath, execPath, projectRoot string, logger logging.Logger) error {
	// Check if directory exists; if not, IDE is likely not installed or not initialized
	dir := filepath.Dir(configPath)
	if ideName == "Cursor (Workspace)" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create workspace config directory: %w", err)
		}
	} else {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return fmt.Errorf("config directory does not exist: %s", dir)
		}
	}

	var config mcpConfig

	// Read existing config if it exists
	if data, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to parse existing config: %w", err)
		}
	}

	if config.MCPServers == nil {
		config.MCPServers = make(map[string]mcpServerConfig)
	}

	// Determine server name and configuration
	serverName := "zqk"
	var env map[string]string
	var cwd string

	// Use only the binary name (e.g. "zqk") so that it resolves dynamically via PATH.
	execPath = filepath.Base(execPath)

	if projectRoot == "" {
		projectRoot = cliContext.ResolveProjectRoot(".")
	}

	if projectRoot != "" {
		if ideName == "Cursor (Workspace)" {
			serverName = "zqk"
			cwd = "${workspaceFolder}"
			env = map[string]string{
				"ZQK_PROJECT_ROOT":    "${workspaceFolder}",
				"ZQK_MCP_CONFIG_PATH": "${workspaceFolder}/.zqk/mcp/config.yaml",
			}
		} else {
			// For global daemons and IDEs (AGY, Claude Desktop, Windsurf), we need a project-specific entry
			// so that they don't overwrite each other. For Cursor (Workspace), "zqk" is fine because it's local.
			serverName = "zqk-" + filepath.Base(projectRoot)
			cwd = projectRoot
			env = map[string]string{
				"ZQK_PROJECT_ROOT":    projectRoot,
				"ZQK_MCP_CONFIG_PATH": filepath.Join(projectRoot, ".zqk", "mcp", "config.yaml"),
			}
		}
	}

	// Add or update ZQK server
	config.MCPServers[serverName] = mcpServerConfig{
		Command: execPath,
		Args:    []string{"mcp", "serve"},
		Env:     env,
		Cwd:     cwd,
	}

	// Write back
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := fileutil.WriteStandardFile(configPath, data); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	logging.Fluent(logger).Info("Configured MCP server").String("ide", ideName).String("path", configPath).Log()
	return nil
}
