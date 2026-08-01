# Built-In MCP Tools vs. CLI Bridge Tools

**Version**: 1.0.0  
**Created**: 2026-01-08  
**Status**: Active  
**Purpose**: Explain the difference between built-in MCP tools and CLI bridge tools

## Quick Summary

**Built-In Tools**: Hardcoded in the MCP server, manually registered, always available  
**CLI Bridge Tools**: Auto-discovered from CLI commands, filtered by config/RBAC

## Built-In MCP Tools

### What They Are

Built-in tools are **hardcoded directly in the MCP server code**. They are manually registered in functions like `RegisterCommonTools()` or `RegisterWorkflowTools()`.

### Characteristics

1. **Manual Registration**
   - Registered in code (e.g., `server.RegisterTool(...)`)
   - Handler functions written specifically for the tool
   - Added to switch statement in `server.go`

2. **Always Available**
   - Bypass `exposed_commands` whitelist in config
   - Not filtered by config security settings
   - Available regardless of server configuration
   - Still subject to RBAC (roles/permissions) for write operations

3. **Specialized Functionality**
   - May not map cleanly to a single CLI command
   - May require direct access to internal systems (graph backend, events)
   - Optimized for specific agent workflows
   - Can combine multiple CLI commands or add logic

4. **Examples**
   - `zqk_object_list` - Wraps `object list` but optimized
   - `zqk_get_current_priority_plan` - Workflow-aware (doesn't map to single CLI command)
   - `zqk_graph_traversal` - Requires graph backend access
   - `zqk_test_echo` - Test/debug tool

### Current Built-In Tools

- **Common CLI Tools** (5): `zqk_object_list`, `zqk_object_get`, `zqk_object_count`, `zqk_system_status`, `zqk_system_check`
- **Graph Tools** (3): `zqk_graph_traversal`, `zqk_resolve_references`, `zqk_state_aware_query`
- **Workflow Tools** (4): `zqk_get_current_priority_plan`, `zqk_get_priority_plan_items`, `zqk_get_current_backlog_item`, `zqk_get_next_backlog_item`
- **Test Tools** (1): `zqk_test_echo`

### Registration Code

```go
// In pkg/mcp/tools_common.go
func RegisterCommonTools(server *Server) {
    server.RegisterTool(
        "zqk_object_list",
        "List objects with filtering...",
        map[string]any{...}, // Input schema
        nil, // Handler called via switch statement
    )
}
```

## CLI Bridge Tools

### What They Are

CLI bridge tools are **automatically discovered** from the Cobra command tree and converted to MCP tools. They map directly to CLI commands.

### Characteristics

1. **Automatic Discovery**
   - Discovered from `rootCommand` (Cobra command tree)
   - Converted to MCP tools via `ConvertCommandToMCPTool()`
   - Registered automatically via `RegisterCLIToolsWithRootCommandAndConfig()`

2. **Filtered by Config**
   - Subject to `exposed_commands` whitelist (if configured)
   - Subject to `blocked_commands` blacklist
   - Subject to `write_operations` restrictions
   - Filtered by RBAC (roles and permissions)

3. **Direct CLI Mapping**
   - Map 1:1 to CLI commands
   - Execute via `ExecuteCLICommandViaMCPWithContext()`
   - General-purpose operations
   - No custom logic (just execute CLI command)

4. **Examples**
   - `zqk_object_create` - Maps to `zqk object create`
   - `zqk_object_update` - Maps to `zqk object update`
   - `zqk_object_delete` - Maps to `zqk object delete`
   - `zqk_system_check` - Maps to `zqk system check` (if not built-in)

### Discovery Process

1. **Discover Commands**: Scan Cobra command tree
2. **Filter by Permissions**: Remove commands user can't access
3. **Filter by Config**: Apply `exposed_commands`, `blocked_commands`, `write_operations`
4. **Convert to Tools**: Use `ConvertCommandToMCPTool()` to create tool definition
5. **Register**: Add to server's tool map

### Registration Code

```go
// In pkg/mcp/cli_bridge.go
func RegisterCLIToolsWithRootCommandAndConfig(...) error {
    // Discover all commands
    allCommands := DiscoverCLICommands(rootCmd)
    
    // Filter by permissions and config
    filteredCommands := FilterCommandsByPermissions(allCommands, secCtx)
    filteredCommands = filterCommandsByConfig(filteredCommands, config, secCtx)
    
    // Convert and register
    for _, cmd := range filteredCommands {
        tool := ConvertCommandToMCPTool(cmd)
        server.RegisterTool(tool.Name, tool.Description, tool.InputSchema, nil)
    }
}
```

## Key Differences

| Aspect | Built-In Tools | CLI Bridge Tools |
|--------|----------------|------------------|
| **Registration** | Manual (hardcoded) | Automatic (discovered) |
| **Location** | `pkg/mcp/tools_*.go` | `cmd/zqk/*/*.go` (CLI commands) |
| **Handler** | Custom handler function | Executes CLI command |
| **Config Filtering** | Bypass whitelist | Subject to config |
| **Availability** | Always available (if role permits) | Filtered by config + RBAC |
| **Purpose** | Specialized, optimized | General-purpose |
| **Mapping** | May not map to CLI command | Maps 1:1 to CLI command |
| **Maintenance** | Manual (code changes) | Automatic (add CLI command = new tool) |

## Current Architecture Note

**Important**: The architecture is in transition. Currently, some tools like `zqk_object_list` are built-in but they should probably be CLI bridge tools (they map directly to CLI commands). 

The architecture document (`MCP_BUILT_IN_TOOLS_ARCHITECTURE.md`) proposes:
- **Built-in tools** = Workflow/context-aware tools (don't map to single CLI command)
- **CLI bridge tools** = General operations (map directly to CLI commands)

But the current implementation has some general operations as built-in tools for convenience (they're always available).

## Which Should You Use?

### Create a Built-In Tool When:

✅ The operation doesn't map cleanly to a single CLI command  
✅ You need direct access to internal systems (graph, events)  
✅ You need workflow-aware operations (current priority plan, next item)  
✅ You need optimized, purpose-built operations  
✅ You want the tool to always be available (bypass config)

### Use CLI Bridge Tool When:

✅ The operation maps directly to a CLI command  
✅ It's a general-purpose operation (create, update, delete, list)  
✅ Config-based filtering is acceptable  
✅ You want automatic discovery (add CLI command = new tool)

## Examples

### Built-In Tool Example

```go
// Manual registration
server.RegisterTool(
    "zqk_get_current_priority_plan",
    "Get the current active priority plan...",
    inputSchema,
    nil, // Handler in switch statement
)

// Custom handler (in server_tool_execution.go switch statement)
case "zqk_get_current_priority_plan":
    return HandleGetCurrentPriorityPlan(ctx, s, args)
```

### CLI Bridge Tool Example

```go
// Automatic - just exists because CLI command exists
// CLI command: zqk object create <kind> --file <file>
// Auto-registered as: zqk_object_create

// No custom code needed - just execute CLI command
// Handled by executeCLICommandWithContext()
```

## For Your Write Wrapper Tool

If you create `zqk_write_object_file`:

- **Built-in tool**: ✅ Recommended
  - Doesn't map to a single CLI command (needs file detection, parsing, routing)
  - Needs custom logic (detect object file, parse YAML, route to CLI)
  - Should always be available (bypass config)
  - Optimized for agent workflow (matches `write` tool interface)

- **CLI bridge tool**: ❌ Not suitable
  - Would require a new CLI command (adds complexity)
  - Can't match `write` tool interface exactly
  - Would be filtered by config (defeats purpose)

## Related Documentation

- `MCP_BUILT_IN_TOOLS.md` - List of built-in tools
- `MCP_BUILT_IN_TOOLS_ARCHITECTURE.md` - Architecture and future plans
- `mcp-cli-bridge-v1.0.md` - CLI bridge implementation
