package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

type mcpConfig struct {
	MCPServers map[string]mcpServerConfig `json:"mcpServers"`
}

type mcpServerConfig struct {
	// Type is required by Cursor 2026 Customize / Tools & MCP (stdio | sse | http).
	// missing type → probe without spawn.
	Type     string            `json:"type,omitempty"`
	Command  string            `json:"command"`
	Args     []string          `json:"args"`
	Env      map[string]string `json:"env,omitempty"`
	EnvFile  string            `json:"envFile,omitempty"`
	Cwd      string            `json:"cwd,omitempty"`
	Disabled bool              `json:"disabled,omitempty"`
}

const mcpTransportStdio = "stdio"

func isForeignMCPConfig(ideName, configPath string) bool {
	p := filepath.ToSlash(configPath)
	if strings.Contains(p, "/.gemini/") || strings.Contains(p, "/.codeium/") {
		return true
	}
	return strings.Contains(ideName, "AGY") || strings.Contains(ideName, "Windsurf")
}

// isIDEStdioAdapterConfig is true for Cursor/IDE mcp.json that must spawn
// ide-adapter (stdio → daemon TCP), not fat mcp serve.
func isIDEStdioAdapterConfig(ideName, configPath string) bool {
	switch ideName {
	case "IDE (Workspace)", "Cursor (Workspace)", "Cursor (Global)":
		return true
	}
	parent := filepath.Base(filepath.Dir(configPath))
	return filepath.Base(configPath) == "mcp.json" && (parent == ".cursor" || parent == ".ide")
}

// AutoInstall registers the ZQK MCP server with supported IDEs and Agents.
func AutoInstall(projectRoot string, logger logging.Logger) error {
	execPath, err := fileutil.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	homeDir, err := fileutil.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	if zqkenv.IsInTest() || zqkenv.TestRoot().Get() != "" {
		homeDir = ""
	}

	configs := findMCPConfigs(homeDir, projectRoot)

	successCount := 0

	for ideName, configPath := range configs {
		if err := InstallToIDE(ideName, configPath, execPath, projectRoot, logger); err != nil {
			logging.Fluent(logger).Warn("Failed to install MCP server").IDE(ideName).WithError(err).Log()
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

	// Always ensure IDE workspace is targeted if in a project
	if projectRoot != "" {
		configs["IDE (Workspace)"] = filepath.Join(projectRoot, ".ide", "mcp.json")
		// Cursor 2026 Customize reads project .cursor/mcp.json (not only .ide/).
		configs["Cursor (Workspace)"] = filepath.Join(projectRoot, ".cursor", "mcp.json")
	}
	if homeDir == "" {
		return configs
	}

	// Relocated MCP controls (Customize sidebar) edit ~/.cursor/mcp.json.
	// Empty global mcpServers leaves the connector red even when project file is valid.
	configs["Cursor (Global)"] = filepath.Join(homeDir, ".cursor", "mcp.json")

	knownFiles := map[string]string{
		"mcp_config.json":            "Global Daemon (AGY/Windsurf)",
		"mcp.json":                   "IDE (Global)",
		"claude_desktop_config.json": "Claude Desktop",
		"cline_mcp_settings.json":    "Cline",
	}

	searchDirs := []string{
		filepath.Join(homeDir, ".codeium"),
		filepath.Join(homeDir, ".gemini"),
		filepath.Join(homeDir, ".ide"),
		filepath.Join(homeDir, ".config"),
	}

	if runtime.GOOS == "darwin" {
		searchDirs = append(searchDirs,
			filepath.Join(homeDir, "Library", "Application Support", "Claude"),
			filepath.Join(homeDir, "Library", "Application Support", "Cursor"),
			filepath.Join(homeDir, "Library", "Application Support", "Code"),
			filepath.Join(homeDir, "Library", "Application Support", "Windsurf"),
		)
	} else if runtime.GOOS == "windows" {
		if appData := zqkenv.OSAppData().Get(); appData != "" {
			searchDirs = append(searchDirs,
				filepath.Join(appData, "Claude"),
				filepath.Join(appData, "Cursor"),
				filepath.Join(appData, "Code"),
				filepath.Join(appData, "Windsurf"),
			)
		}
	}

	already := make(map[string]struct{}, len(configs))
	for _, p := range configs {
		already[filepath.Clean(p)] = struct{}{}
	}
	matches := fileutil.FindFiles(searchDirs, knownFiles)
	for _, match := range matches {
		clean := filepath.Clean(match.Path)
		if _, dup := already[clean]; dup {
			continue
		}
		// Prevent overwriting IDE workspace config if we stumble upon it
		if projectRoot != "" && clean == filepath.Clean(filepath.Join(projectRoot, ".ide", "mcp.json")) {
			continue
		}
		// Use the path as the key to ensure uniqueness if multiple are found
		configs[fmt.Sprintf("%s (%s)", match.Identifier, match.Path)] = match.Path
		already[clean] = struct{}{}
	}

	return configs
}

// InstallToIDE registers the ZQK MCP server in the specific IDE configuration file.
func InstallToIDE(ideName, configPath, execPath, projectRoot string, logger logging.Logger) error {
	// Check if directory exists; if not, IDE is likely not installed or not initialized
	dir := filepath.Dir(configPath)
	if isIDEStdioAdapterConfig(ideName, configPath) {
		if err := fileutil.EnsureDir(dir); err != nil {
			return fmt.Errorf("failed to create workspace config directory: %w", err)
		}
	} else if !fileutil.Exists(dir) {
		return fmt.Errorf("config directory does not exist: %s", dir)
	}

	var config mcpConfig

	// Read existing config if it exists
	if data, err := fileutil.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(data, &config); err != nil {
			// Vendor/global files we do not own (empty Gemini mcp_config.json)
			// must not warn-flood AutoInstall. Skip; do not rewrite.
			// no longer walks ~/.gemini / AGY paths.
			if isForeignMCPConfig(ideName, configPath) || len(bytes.TrimSpace(data)) == 0 {
				return nil
			}
			return fmt.Errorf("failed to parse existing config: %w", err)
		}
	}

	if config.MCPServers == nil {
		config.MCPServers = make(map[string]mcpServerConfig)
	}

	// Determine server name and configuration (brand-generic; no hardcoded product name).
	exeName := brand.ExecutableName()
	serverName := exeName
	var env map[string]string
	var cwd string
	var command string
	var args []string

	if projectRoot == "" {
		projectRoot = paths.ResolveProjectRoot(".")
	}

	if projectRoot != "" {
		mcpEnv := map[string]string{
			zqkenv.ProjectRoot().Name():   projectRoot,
			zqkenv.MCPConfigPath().Name(): paths.MCPConfigPath(projectRoot),
		}
		if isIDEStdioAdapterConfig(ideName, configPath) {
			// IDE entrypoint: ide-adapter (stdio MCP face) → host MCP daemon.
			// Prefer workshop stable so Cursor/MCP share the same inode as scheduler;
			// tip bin/zqk is for rebuilds — promote via scripts/install.sh.
			// TRACK: no tip/stable split-brain.
			serverName = exeName
			target := ""
			for _, candidate := range paths.StableBinaryCandidates(projectRoot) {
				if fileutil.IsRegularFile(candidate) {
					target = candidate
					break
				}
			}
			if target == "" {
				target = paths.RepoBinPath(projectRoot)
			}
			if !fileutil.IsRegularFile(target) {
				target = execPath
			}
			if _, linkErr := EnsureMCPRoleSymlink(projectRoot, MCPRoleIDEAdapter, target); linkErr != nil {
				logging.Fluent(logger).Warn("MCP ide-adapter role symlink unavailable; using target bin in mcp.json").
					WithError(linkErr).
					Path(target).
					Log()
				command = target
			} else {
				// Interpolated paths: Cursor Customize + clones must not bake machine abs paths.
				command = "${workspaceFolder}/bin/" + MCPRoleProcessName(MCPRoleIDEAdapter)
			}
			cwd = "${workspaceFolder}"
			env = map[string]string{
				zqkenv.ProjectRoot().Name():   "${workspaceFolder}",
				zqkenv.MCPConfigPath().Name(): "${workspaceFolder}/" + paths.ProjectDataDir + "/" + paths.MCPDir + "/config.yaml",
			}
			args = []string{"mcp", "ide-adapter", "--tcp", DefaultDaemonTCP}
		} else {
			// For global daemons and IDEs (AGY, Claude Desktop, Windsurf), we need a project-specific entry
			// so that they don't overwrite each other. For IDE (Workspace), the bare brand name is fine.
			serverName = exeName + "-" + filepath.Base(projectRoot)
			cwd = projectRoot
			command = filepath.Base(execPath)
			args = []string{"mcp", "serve"}
			env = mcpEnv
		}
	} else {
		command = filepath.Base(execPath)
		args = []string{"mcp", "serve"}
	}

	// Add or update ZQK server
	entry := mcpServerConfig{
		Command: command,
		Args:    args,
		Env:     env,
		Cwd:     cwd,
	}
	if prev, ok := config.MCPServers[serverName]; ok && prev.EnvFile != "" {
		entry.EnvFile = prev.EnvFile
	}
	if isIDEStdioAdapterConfig(ideName, configPath) {
		entry.Type = mcpTransportStdio
		entry.EnvFile = "${workspaceFolder}/" + paths.ProjectDataDir + "/" + paths.MCPDir + "/cursor-stdio.env"
	}
	config.MCPServers[serverName] = entry

	// Write back
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := fileutil.WriteStandardFile(configPath, data); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	logging.Fluent(logger).Info("Configured MCP server").IDE(ideName).Path(configPath).Log()
	return nil
}
