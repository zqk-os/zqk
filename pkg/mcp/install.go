package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/adapters/cursor"
	"github.com/zqk-os/zqk/pkg/adapters/ide"
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
	return ide.IsForeignConfig(ideName, configPath)
}

func isWorkspaceConfig(ideName, projectRoot, configPath string) bool {
	return ide.IsWorkspaceConfig(ideName, projectRoot, configPath)
}

// isIDEStdioAdapterConfig is true for Cursor/IDE mcp.json that must spawn
// ide-adapter (stdio → daemon TCP), not fat mcp serve.
func isIDEStdioAdapterConfig(ideName, configPath string) bool {
	return ide.IsIDEStdioAdapterConfig(ideName, configPath)
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
	return ide.FindConfigs(homeDir, projectRoot)
}

// InstallToIDE registers the ZQK MCP server in the specific IDE configuration file.
func InstallToIDE(ideName, configPath, execPath, projectRoot string, logger logging.Logger) error {
	dir := filepath.Dir(configPath)
	if isIDEStdioAdapterConfig(ideName, configPath) || isWorkspaceConfig(ideName, projectRoot, configPath) {
		if err := fileutil.EnsureDir(dir); err != nil {
			return fmt.Errorf("failed to create workspace config directory: %w", err)
		}
	} else if !fileutil.Exists(dir) {
		return fmt.Errorf("config directory does not exist: %s", dir)
	}

	if projectRoot == "" {
		projectRoot = paths.ResolveProjectRoot(".")
	}

	rawConfig, config, err := readExistingMCPConfig(ideName, configPath)
	if err != nil {
		return err
	}
	if rawConfig == nil {
		return nil
	}

	serverName, entry := buildMCPServerConfig(ideName, configPath, execPath, projectRoot, config.MCPServers, logger)
	config.MCPServers[serverName] = entry
	rawConfig["mcpServers"] = config.MCPServers

	// Fail-closed: prevent clobbering an existing seated host-global MCP pin
	// without human opt-in or a recent intentional node_hop.
	if err := cursor.RefuseGlobalMCPPinRewrite(projectRoot, configPath); err != nil {
		logging.Fluent(logger).Warn("skipped global mcp.json rewrite (node sovereignty)").
			IDE(ideName).
			Path(configPath).
			WithError(err).
			Log()
		return err
	}

	// Write back
	data, err := json.MarshalIndent(rawConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := fileutil.WriteStandardFile(configPath, data); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	logging.Fluent(logger).Info("Configured MCP server").IDE(ideName).Path(configPath).Log()
	return nil
}

func readExistingMCPConfig(ideName, configPath string) (map[string]any, mcpConfig, error) {
	rawConfig := make(map[string]any)
	var config mcpConfig

	data, err := fileutil.ReadFile(configPath)
	if err != nil {
		config.MCPServers = make(map[string]mcpServerConfig)
		return rawConfig, config, nil
	}

	if err := json.Unmarshal(data, &rawConfig); err != nil {
		if isForeignMCPConfig(ideName, configPath) || len(bytes.TrimSpace(data)) == 0 {
			return nil, config, nil
		}
		return nil, config, fmt.Errorf("failed to parse existing config: %w", err)
	}

	if err := json.Unmarshal(data, &config); err != nil {
		// Tolerated schema variance: typed unmarshal is auxiliary to rawConfig
		config.MCPServers = make(map[string]mcpServerConfig)
	}

	if rawConfig == nil {
		rawConfig = make(map[string]any)
	}
	if config.MCPServers == nil {
		config.MCPServers = make(map[string]mcpServerConfig)
	}
	return rawConfig, config, nil
}

func buildMCPServerConfig(ideName, configPath, execPath, projectRoot string, existingServers map[string]mcpServerConfig, logger logging.Logger) (string, mcpServerConfig) {
	exeName := brand.ExecutableName()
	serverName := exeName
	var env map[string]string
	var cwd string
	var command string
	var args []string

	if projectRoot != "" {
		mcpEnv := map[string]string{
			zqkenv.ProjectRoot().Name():   projectRoot,
			zqkenv.MCPConfigPath().Name(): paths.MCPConfigPath(projectRoot),
		}
		if isIDEStdioAdapterConfig(ideName, configPath) {
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
			roleBin := filepath.Join(projectRoot, "bin", MCPRoleProcessName(MCPRoleIDEAdapter))
			if _, linkErr := EnsureMCPRoleSymlink(projectRoot, MCPRoleIDEAdapter, target); linkErr != nil {
				logging.Fluent(logger).Warn("MCP ide-adapter role symlink unavailable; using target bin in mcp.json").
					WithError(linkErr).
					Path(target).
					Log()
				command = target
			} else {
				command = roleBin
			}
			cwd = projectRoot
			env = mcpEnv
			args = []string{"mcp", "ide-adapter", "--tcp", DefaultDaemonTCP}
		} else {
			if strings.Contains(ideName, "Workspace") || ideName == "Claude Code" {
				serverName = exeName
			} else {
				serverName = exeName + "-" + filepath.Base(projectRoot)
			}
			cwd = projectRoot
			command = filepath.Base(execPath)
			args = []string{"mcp", "serve"}
			env = mcpEnv
		}
	} else {
		command = filepath.Base(execPath)
		args = []string{"mcp", "serve"}
	}

	entry := mcpServerConfig{
		Command: command,
		Args:    args,
		Env:     env,
		Cwd:     cwd,
	}
	if prev, ok := existingServers[serverName]; ok && prev.EnvFile != "" {
		entry.EnvFile = prev.EnvFile
	}
	if isIDEStdioAdapterConfig(ideName, configPath) {
		entry.Type = mcpTransportStdio
		if projectRoot != "" {
			entry.EnvFile = filepath.Join(projectRoot, paths.ProjectDataDir, paths.MCPDir, "cursor-stdio.env")
		}
	}
	return serverName, entry
}
