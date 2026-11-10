package ide_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/adapters/ide"
)

func TestIDEAdapters_StaticFloorInvariants(t *testing.T) {
	adapters := ide.All()
	require.NotEmpty(t, adapters, "at least one adapter must be registered")

	expectedVendors := map[string]bool{
		"cursor":    false,
		"anthropic": false,
		"cline":     false,
		"codeium":   false,
		"google":    false,
		"generic":   false,
	}

	for _, a := range adapters {
		assert.NotEmpty(t, a.Name(), "adapter name must not be empty")
		assert.NotEmpty(t, a.Vendor(), "adapter vendor must not be empty")
		if _, ok := expectedVendors[a.Vendor()]; ok {
			expectedVendors[a.Vendor()] = true
		}
	}

	for vendor, found := range expectedVendors {
		assert.True(t, found, "expected vendor adapter %s to be registered", vendor)
	}
}

func TestIDEAdapters_WorkspaceAndGlobalConfigs(t *testing.T) {
	tmpDir := t.TempDir()
	configs := ide.FindConfigs(tmpDir, tmpDir)
	require.NotEmpty(t, configs)

	// Workspace configurations should be discovered
	assert.Contains(t, configs, "IDE (Workspace)")
	assert.Contains(t, configs, "Cursor (Workspace)")
	assert.Contains(t, configs, "Claude Code (Workspace)")
	assert.Contains(t, configs, "Windsurf (Workspace)")
	assert.Contains(t, configs, "Cline (Workspace)")

	assert.Equal(t, filepath.Join(tmpDir, ".ide", "mcp.json"), configs["IDE (Workspace)"])
	assert.Equal(t, filepath.Join(tmpDir, ".cursor", "mcp.json"), configs["Cursor (Workspace)"])
	assert.Equal(t, filepath.Join(tmpDir, ".claude.json"), configs["Claude Code (Workspace)"])
	assert.Equal(t, filepath.Join(tmpDir, ".codeium", "windsurf", "mcp_config.json"), configs["Windsurf (Workspace)"])
	assert.Equal(t, filepath.Join(tmpDir, ".cline", "mcp_settings.json"), configs["Cline (Workspace)"])
}

func TestIDEAdapters_IsForeignConfig(t *testing.T) {
	tests := []struct {
		ideName    string
		configPath string
		expected   bool
	}{
		{"Global Daemon (AGY/Windsurf)", "/home/user/.gemini/mcp_config.json", true},
		{"Global Daemon (AGY/Windsurf)", "/home/user/.codeium/mcp_config.json", true},
		{"AGY Host", "/some/path/mcp.json", true},
		{"Windsurf (Workspace)", "/project/.codeium/windsurf/mcp_config.json", true},
		{"Cursor (Workspace)", "/project/.cursor/mcp.json", false},
		{"Claude Code (Workspace)", "/project/.claude.json", false},
		{"IDE (Workspace)", "/project/.ide/mcp.json", false},
	}

	for _, tc := range tests {
		t.Run(tc.ideName+"_"+tc.configPath, func(t *testing.T) {
			actual := ide.IsForeignConfig(tc.ideName, tc.configPath)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestIDEAdapters_IsStdioAdapterConfig(t *testing.T) {
	tests := []struct {
		ideName    string
		configPath string
		expected   bool
	}{
		{"Cursor (Workspace)", "/project/.cursor/mcp.json", true},
		{"Cursor (Global)", "/home/user/.cursor/mcp.json", true},
		{"IDE (Workspace)", "/project/.ide/mcp.json", true},
		{"Custom", "/project/.cursor/mcp.json", true},
		{"Custom", "/project/.ide/mcp.json", true},
		{"Claude Code (Workspace)", "/project/.claude.json", false},
		{"Windsurf (Workspace)", "/project/.codeium/windsurf/mcp_config.json", false},
		{"Cline (Workspace)", "/project/.cline/mcp_settings.json", false},
	}

	for _, tc := range tests {
		t.Run(tc.ideName+"_"+tc.configPath, func(t *testing.T) {
			actual := ide.IsIDEStdioAdapterConfig(tc.ideName, tc.configPath)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestIDEAdapters_IsWorkspaceConfig(t *testing.T) {
	root := "/Users/test/workspace"
	assert.True(t, ide.IsWorkspaceConfig("Cursor (Workspace)", root, filepath.Join(root, ".cursor", "mcp.json")))
	assert.False(t, ide.IsWorkspaceConfig("Cursor (Global)", root, "/Users/test/.cursor/mcp.json"))
	assert.False(t, ide.IsWorkspaceConfig("Claude Desktop", root, "/Users/test/Library/Application Support/Claude/claude_desktop_config.json"))
	assert.False(t, ide.IsWorkspaceConfig("", "", "/some/path"))
}
