package swarm

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// getRepoRoot finds the root directory of the repository
func getRepoRoot() string {
	cwd, _ := fileutil.Getwd()
	for {
		if _, err := fileutil.Stat(filepath.Join(cwd, "go.mod")); err == nil {
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
	if _, err := fileutil.Stat(mcpPath); fileutil.IsNotExist(err) {
		t.Skipf("configured MCP server %s not found (run: make bin/zqk-mcp)", mcpPath)
	}

	ctx := context.Background()
	t.Setenv(zqkenv.APIKey().Name(), pkgctx.TestHarnessAccountID)

	// Isolate the MCP server execution from the real workspace using testkit
	isoProj := testkit.PrepareIsolatedTempProject(t, nil)
	// We still explicitly set ZQK_TEST_ROOT because zqk-mcp runs out of process, and might not use the test prefix if it's an external binary
	t.Setenv(zqkenv.TestRoot().Name(), isoProj.Root)
	t.Setenv(zqkenv.SessionID().Name(), "ZQK-TEST-SESSION")

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

func TestIsEagerTool_writeCodeIsEager(t *testing.T) {
	t.Parallel()
	if !isEagerTool("zqk_write_code") {
		t.Fatal("zqk_write_code must be eager so AgentX sees path/content instead of fuzzy-matching write_file")
	}
	if !isEagerTool("zqk_observer_search") {
		t.Fatal("zqk_observer_search must be eager so doers see AST search without mcp_list_tools")
	}
	if isEagerTool("zqk_mcp_call_tool") {
		t.Fatal("meta tools are injected, not classified eager")
	}
}

func TestMCPChildEnviron_keepsProjectRootSetsWorktree(t *testing.T) {
	t.Parallel()
	parent := []string{"FOO=1", zqkenv.ProjectRoot().Name() + "=/studio", zqkenv.AgentWorktreeRoot().Name() + "=/old"}
	got := mcpChildEnviron(parent, "/wt")
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, zqkenv.ProjectRoot().Name()+"=/studio") {
		t.Fatalf("studio PROJECT_ROOT must stay: %v", got)
	}
	if !strings.Contains(joined, zqkenv.AgentWorktreeRoot().Name() + "=/wt") {
		t.Fatalf("worktree root missing: %v", got)
	}
	if strings.Contains(joined, zqkenv.AgentWorktreeRoot().Name() + "=/old") {
		t.Fatalf("stale worktree root kept: %v", got)
	}
}

func TestExecuteToolCall_mcpCallToolRejectsShellName(t *testing.T) {
	t.Parallel()
	ex := &MCPExecutor{allTools: map[string]llm.ToolDefinition{"zqk_read_code": {}}}
	_, err := ex.ExecuteToolCall(context.Background(), llm.ToolCall{
		Name:      "zqk_mcp_call_tool",
		Arguments: `{"tool_name":"go mod tidy","arguments":{}}`,
	})
	if err == nil || !strings.Contains(err.Error(), "not a tool name") {
		t.Fatalf("want shell-name reject, got %v", err)
	}
	_, err = ex.ExecuteToolCall(context.Background(), llm.ToolCall{
		Name:      "zqk_mcp_call_tool",
		Arguments: `{"tool_name":"zqk_create_directory","arguments":{}}`,
	})
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("want unregistered exact reject, got %v", err)
	}
}

func TestNormalizeToolName_prefersExactLazyWriteCode(t *testing.T) {
	t.Parallel()
	executor := &MCPExecutor{
		tools: []llm.ToolDefinition{{Name: "zqk_write_file"}},
		allTools: map[string]llm.ToolDefinition{
			"zqk_write_file": {},
			"zqk_write_code": {},
		},
	}
	got, err := executor.normalizeToolName("zqk_write_code")
	if err != nil {
		t.Fatal(err)
	}
	if got != "zqk_write_code" {
		t.Fatalf("normalizeToolName(zqk_write_code)=%q, want exact lazy name not write_file", got)
	}
}
// tdd refresh
