package swarm

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/llm"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// getRepoRoot finds the root directory of the repository
func getRepoRoot() string {
	cwd, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			return cwd
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			return ""
		}
		cwd = parent
	}
}

func TestMCPExecutor(t *testing.T) {
	repoRoot := getRepoRoot()
	if repoRoot == "" {
		t.Fatal("could not find repo root")
	}

	mcpPath := filepath.Join(repoRoot, "bin", "zqk-mcp")
	if _, err := os.Stat(mcpPath); os.IsNotExist(err) {
		t.Skipf("configured MCP server %s not found (run: make bin/zqk-mcp)", mcpPath)
	}

	ctx := context.Background()
	t.Setenv(zqkenv.APIKey(), "account:system")

	// Isolate the MCP server execution from the real workspace using testkit
	isoProj := testkit.PrepareIsolatedTempProject(t, nil)
	// We still explicitly set ZQK_TEST_ROOT because zqk-mcp runs out of process, and might not use the test prefix if it's an external binary
	t.Setenv(zqkenv.TestRoot(), isoProj.Root)
	t.Setenv(zqkenv.SessionID(), "ZQK-TEST-SESSION")

	executor, err := NewMCPExecutor(ctx, mcpPath)
	if err != nil {
		t.Fatalf("failed to create MCP executor: %v", err)
	}
	defer executor.Close()

	tools, err := executor.GetTools(ctx)
	if err != nil {
		t.Fatalf("failed to get tools: %v", err)
	}

	if len(tools) == 0 {
		t.Errorf("expected at least one tool, got 0")
	}

	// Try to find a simple tool to call, like echo or bash execution
	var toolName string
	for _, t := range tools {
		if t.Name == "echo" || t.Name == "zqk_echo" {
			toolName = t.Name
			break
		}
	}

	if toolName == "" {
		for _, t := range tools {
			if t.Name == "zqk_execute_bash" {
				toolName = t.Name
				break
			}
		}
	}

	if toolName == "" {
		t.Skipf("could not find echo or bash tool for testing")
	}

	call := llm.ToolCall{
		Name: toolName,
	}

	if toolName == "zqk_execute_bash" {
		call.Arguments = `{"command": "echo done"}`
	} else {
		call.Arguments = `{"message": "hello from llm"}`
	}

	result, err := executor.ExecuteToolCall(ctx, call)
	if err != nil {
		t.Fatalf("failed to execute tool call %s: %v", toolName, err)
	}

	if result == "" {
		t.Errorf("expected non-empty result from tool call")
	}
	t.Logf("Tool %s output: %s", toolName, result)
}

func TestNormalizeToolName(t *testing.T) {
	executor := &MCPExecutor{
		tools: []llm.ToolDefinition{
			{Name: "zqk_new_object"},
			{Name: "zqk_object_create"},
			{Name: "zqk_object_update"},
			{Name: "zqk_execute_bash"},
		},
	}

	tests := []struct {
		input    string
		expected string
		wantErr  bool
	}{
		{"zqk_new_object", "zqk_new_object", false},       // exact match
		{"zqk new object", "zqk_new_object", false},       // spaces replaced
		{"zqk_new_objct", "zqk_new_object", false},        // fuzzy match (1 typo)
		{"zqk new objet", "zqk_new_object", false},        // fuzzy match (1 typo + spaces)
		{"zqk_object_create", "zqk_object_create", false}, // exact match
		{"zqk object create", "zqk_object_create", false}, // spaces replaced
		{"zqk object upate", "zqk_object_update", false},  // fuzzy match (1 typo + spaces)
		{"totally_wrong", "", true},                       // no match
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := executor.normalizeToolName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("normalizeToolName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("normalizeToolName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
