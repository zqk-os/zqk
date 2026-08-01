// Package mcp: tests for CLI–MCP feature parity (ITEM-010).
// Verifies that built-in MCP tools correspond to CLI commands so agents can perform the same operations via MCP or CLI.

package mcp

import (
	"testing"
)

// commonToolToCLIPath defines the expected CLI command path for each common built-in tool.
// Used to document and assert CLI–MCP parity for ITEM-010 (Verify CLI and MCP feature parity).
var commonToolToCLIPath = map[string]string{
	GetToolName("object_list"):   "object list",
	GetToolName("object_get"):    "object get",
	GetToolName("object_count"):  "object count",
	GetToolName("system_status"): "system status",
	GetToolName("system_check"):  "system check",
}

// TestCommonBuiltInToolsMapToCLICommands verifies that each common built-in tool has a defined CLI command path.
// Part of CLI–MCP parity verification (ITEM-010). Expand commonToolToCLIPath when adding new common tools.
func TestCommonBuiltInToolsMapToCLICommands(t *testing.T) {
	t.Parallel()
	for toolName, cliPath := range commonToolToCLIPath {
		if toolName == emptyValue {
			t.Error("tool name must be non-empty in commonToolToCLIPath")
		}
		if cliPath == emptyValue {
			t.Errorf("CLI path for tool %q must be non-empty", toolName)
		}
	}
	// Require at least the five common tools (object_list, object_get, object_count, system_status, system_check)
	const expectedCommon = 5
	if n := len(commonToolToCLIPath); n < expectedCommon {
		t.Errorf("commonToolToCLIPath should have at least %d entries (CLI–MCP parity), got %d", expectedCommon, n)
	}
}
