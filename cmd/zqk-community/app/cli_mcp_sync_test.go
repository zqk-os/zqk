package app

import (
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/mcp"
	"github.com/spf13/pflag"
)

func TestMCPCLISyncParity(t *testing.T) {
	rootCmdMu.Lock()
	defer rootCmdMu.Unlock()
	ensureCommandsRegisteredLocked()

	initCtx := &pkgctx.CliInitializationContext{
		ProjectRoot: "/tmp",
	}
	secCtx := pkgctx.NewSecurityContext("system", []string{"admin"}, []string{"read:*", "write:*", "execute:*"})
	config := &mcp.ServerConfig{} // Use empty config for tests

	tools := mcp.ListExposedTools(initCtx, rootCmd, config, secCtx)

	mcpToolsMap := make(map[string]mcp.Tool)
	for _, tool := range tools {
		t.Logf("Found MCP tool: %s", tool.Name)
		mcpToolsMap[tool.Name] = tool
	}

	// We want to test hand-rolled tools in pkg/mcp/tools_common.go
	tests := []struct {
		mcpToolName string
		cliPath     []string // e.g., ["object", "list"]
	}{
		{mcp.GetToolName("object_list"), []string{"object", "list"}},
		{mcp.GetToolName("object_get"), []string{"object", "get"}},
		{mcp.GetToolName("object_count"), []string{"object", "count"}},
		{mcp.GetToolName("system_status"), []string{"system", "status"}},
		{mcp.GetToolName("system_check"), []string{"system", "check"}},
	}

	for _, tt := range tests {
		t.Run(tt.mcpToolName, func(t *testing.T) {
			tool, exists := mcpToolsMap[tt.mcpToolName]
			if !exists {
				t.Fatalf("MCP tool %s not found", tt.mcpToolName)
			}

			// Find CLI command
			cmd := rootCmd
			for _, p := range tt.cliPath {
				found := false
				for _, c := range cmd.Commands() {
					if c.Name() == p {
						cmd = c
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("CLI command path %v not found at %s", tt.cliPath, p)
				}
			}

			// Get MCP schema properties
			schema, ok := tool.InputSchema.(map[string]any)
			if !ok {
				t.Fatal("MCP tool schema is not a map")
			}
			props, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatal("MCP tool schema properties is not a map")
			}

			// Get CLI flags
			cliFlags := make(map[string]bool)
			cmd.Flags().VisitAll(func(f *pflag.Flag) {
				cliFlags[f.Name] = true
			})

			// Compare MCP properties to CLI flags
			for propName := range props {
				// Internal MCP parameters that don't map to CLI flags directly
				if propName == "_command_path" || propName == "format" {
					continue
				}

				// Normalize propName since MCP uses snake_case and CLI uses kebab-case
				normalizedProp := strings.ReplaceAll(propName, "_", "-")

				// Some properties are positional arguments in CLI, not flags
				if normalizedProp == "id" || normalizedProp == "id..." || normalizedProp == "kind" {
					continue
				}

				if !cliFlags[normalizedProp] {
					t.Errorf("MCP tool %s has property '%s' which has no corresponding CLI flag '--%s' in command '%s'",
						tt.mcpToolName, propName, normalizedProp, strings.Join(tt.cliPath, " "))
				}
			}
		})
	}
}
