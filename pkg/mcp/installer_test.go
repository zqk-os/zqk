package mcp_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Satisfies TST-ECOSYSTEM-1CLICK-MCP-001 and CRIT-ECOSYSTEM-1CLICK-MCP-001:
// Verify One-Click MCP Config Generation does not clobber existing server configurations.

func TestInstallToIDE_PreservesExistingServers(t *testing.T) {
	tmpDir := t.TempDir()
	cursorDir := filepath.Join(tmpDir, ".cursor")
	require.NoError(t, fileutil.MkdirAll(cursorDir, paths.DirPerm755))

	configPath := filepath.Join(cursorDir, "mcp.json")
	initialConfig := map[string]any{
		"mcpServers": map[string]any{
			"custom-ai-tool": map[string]any{
				"command": "python",
				"args":    []string{"-m", "custom_mcp"},
				"env": map[string]string{
					"API_KEY": "secret-1234",
				},
			},
		},
	}
	initialBytes, err := json.MarshalIndent(initialConfig, "", "  ")
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(configPath, initialBytes, paths.FilePerm644))

	logger := logging.GetLoggerFromProfile("")
	fakeExec := filepath.Join(tmpDir, "bin", "zqk")

	// Install to Cursor workspace
	err = mcp.InstallToIDE("Cursor (Workspace)", configPath, fakeExec, tmpDir, logger)
	require.NoError(t, err)

	// Read updated config
	updatedBytes, err := fileutil.ReadFile(configPath)
	require.NoError(t, err)

	var updatedConfig struct {
		MCPServers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
			Type    string            `json:"type"`
		} `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(updatedBytes, &updatedConfig))

	// Verify custom-ai-tool was preserved intact
	customServer, exists := updatedConfig.MCPServers["custom-ai-tool"]
	require.True(t, exists, "pre-existing mcp server must not be clobbered")
	assert.Equal(t, "python", customServer.Command)
	assert.Equal(t, []string{"-m", "custom_mcp"}, customServer.Args)
	assert.Equal(t, "secret-1234", customServer.Env["API_KEY"])

	// Verify zqk entry was added
	zqkServer, zqkExists := updatedConfig.MCPServers["zqk"]
	require.True(t, zqkExists, "zqk server entry must be added")
	assert.Equal(t, "stdio", zqkServer.Type)
	assert.Contains(t, zqkServer.Args, "ide-adapter")
}

func TestInstallToIDE_ClaudeDesktop(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, "Claude")
	require.NoError(t, fileutil.MkdirAll(claudeDir, paths.DirPerm755))

	configPath := filepath.Join(claudeDir, "claude_desktop_config.json")
	logger := logging.GetLoggerFromProfile("")
	fakeExec := filepath.Join(tmpDir, "bin", "zqk")

	err := mcp.InstallToIDE("Claude Desktop", configPath, fakeExec, tmpDir, logger)
	require.NoError(t, err)

	updatedBytes, err := fileutil.ReadFile(configPath)
	require.NoError(t, err)

	var config struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(updatedBytes, &config))

	serverKey := "zqk-" + filepath.Base(tmpDir)
	server, exists := config.MCPServers[serverKey]
	require.True(t, exists, "claude desktop must receive project-specific key: %s", serverKey)
	assert.Equal(t, "zqk", server.Command)
	assert.Equal(t, []string{"mcp", "serve"}, server.Args)
}

func TestInstallToIDE_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	cursorDir := filepath.Join(tmpDir, ".cursor")
	require.NoError(t, fileutil.MkdirAll(cursorDir, paths.DirPerm755))

	configPath := filepath.Join(cursorDir, "mcp.json")
	logger := logging.GetLoggerFromProfile("")
	fakeExec := filepath.Join(tmpDir, "bin", "zqk")

	// First install
	require.NoError(t, mcp.InstallToIDE("Cursor (Workspace)", configPath, fakeExec, tmpDir, logger))
	bytesFirst, err := fileutil.ReadFile(configPath)
	require.NoError(t, err)

	// Second install
	require.NoError(t, mcp.InstallToIDE("Cursor (Workspace)", configPath, fakeExec, tmpDir, logger))
	bytesSecond, err := fileutil.ReadFile(configPath)
	require.NoError(t, err)

	assert.Equal(t, string(bytesFirst), string(bytesSecond), "subsequent install calls must be idempotent")
}

func TestAutoInstall_ProjectWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile("")

	err := mcp.AutoInstall(tmpDir, logger)
	require.NoError(t, err)

	// Both .ide/mcp.json and .cursor/mcp.json should be created
	ideConfig := filepath.Join(tmpDir, ".ide", "mcp.json")
	cursorConfig := filepath.Join(tmpDir, ".cursor", "mcp.json")

	assert.True(t, fileutil.Exists(ideConfig), ".ide/mcp.json should exist")
	assert.True(t, fileutil.Exists(cursorConfig), ".cursor/mcp.json should exist")
}
